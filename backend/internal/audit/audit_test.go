package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ─── Pure logic tests (no DB required, run with -short) ──────────────

// TestComputeHash verifies the HMAC-SHA256 computation matches the expected
// formula: this_hash = HMAC(secret, prev_hash || payload).
func TestComputeHash(t *testing.T) {
	secret := []byte("test-secret")
	prevHash := "abc123"
	payload := []byte(`{"action":"login"}`)

	got := computeHash(secret, prevHash, payload)

	// Recompute independently to verify
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(prevHash))
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Fatalf("computeHash mismatch: got %s, want %s", got, want)
	}
}

// TestComputeHash_Determinism verifies the same inputs always produce the same hash.
func TestComputeHash_Determinism(t *testing.T) {
	secret := []byte("s")
	prevHash := "p"
	payload := []byte("data")

	h1 := computeHash(secret, prevHash, payload)
	h2 := computeHash(secret, prevHash, payload)
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got %s then %s", h1, h2)
	}
}

// TestComputeHash_DifferentInputs verifies different inputs produce different hashes.
func TestComputeHash_DifferentInputs(t *testing.T) {
	secret := []byte("s")

	h1 := computeHash(secret, "prev1", []byte("payload1"))
	h2 := computeHash(secret, "prev2", []byte("payload2"))
	if h1 == h2 {
		t.Fatal("expected different hashes for different inputs")
	}

	// Different secret → different hash
	h3 := computeHash([]byte("other-secret"), "prev1", []byte("payload1"))
	if h1 == h3 {
		t.Fatal("expected different hashes for different secrets")
	}
}

// TestComputeHash_EmptyPrevHash verifies the genesis entry (prev_hash="") computes correctly.
func TestComputeHash_EmptyPrevHash(t *testing.T) {
	secret := []byte("genesis-secret")
	got := computeHash(secret, "", []byte(`{"action":"init"}`))
	if got == "" {
		t.Fatal("expected non-empty hash for genesis entry")
	}
	// Must be 64 hex chars (SHA-256 = 32 bytes = 64 hex chars)
	if len(got) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(got))
	}
}

// ─── Hash chain integrity (pure logic, no DB) ────────────────────────

// buildChain computes a hash chain for N entries in-memory, mimicking writeToDB logic.
// Returns the list of (prevHash, thisHash) pairs for verification.
func buildChain(secret []byte, entries []AuditEntry) []struct{ prev, this string } {
	chain := make([]struct{ prev, this string }, len(entries))
	prevHash := ""
	for i, entry := range entries {
		payload, _ := json.Marshal(entry)
		thisHash := computeHash(secret, prevHash, payload)
		chain[i] = struct{ prev, this string }{prev: prevHash, this: thisHash}
		prevHash = thisHash
	}
	return chain
}

// TestAuditHashChain verifies the HMAC chain is computed correctly:
// each this_hash = HMAC(secret, prev_hash || payload), and prev_hash of
// entry N equals this_hash of entry N-1.
func TestAuditHashChain(t *testing.T) {
	secret := []byte("chain-secret")
	entries := []AuditEntry{
		{Action: "login", ActorID: "user1", ActorIP: "1.1.1.1", Resource: "auth"},
		{Action: "create_room", ActorID: "user1", ActorIP: "1.1.1.1", Resource: "room/123"},
		{Action: "start_game", ActorID: "user2", ActorIP: "2.2.2.2", Resource: "game/456"},
		{Action: "end_game", ActorID: "user2", ActorIP: "2.2.2.2", Resource: "game/456"},
		{Action: "logout", ActorID: "user1", ActorIP: "1.1.1.1", Resource: "auth"},
	}

	chain := buildChain(secret, entries)

	// Verify genesis entry has empty prev_hash
	if chain[0].prev != "" {
		t.Fatalf("genesis entry prev_hash should be empty, got %s", chain[0].prev)
	}

	// Verify chain linkage: prev_hash of entry N == this_hash of entry N-1
	for i := 1; i < len(chain); i++ {
		if chain[i].prev != chain[i-1].this {
			t.Fatalf("chain broken at entry %d: prev_hash=%s, expected=%s",
				i, chain[i].prev, chain[i-1].this)
		}
	}

	// Verify each hash is independently correct
	prevHash := ""
	for i, entry := range entries {
		payload, _ := json.Marshal(entry)
		expected := computeHash(secret, prevHash, payload)
		if chain[i].this != expected {
			t.Fatalf("entry %d hash mismatch: got %s, expected %s",
				i, chain[i].this, expected)
		}
		prevHash = chain[i].this
	}
}

// TestAuditHashChain_Deterministic verifies the same entries always produce the same chain.
func TestAuditHashChain_Deterministic(t *testing.T) {
	secret := []byte("det-secret")
	entries := []AuditEntry{
		{Action: "a", ActorID: "u1"},
		{Action: "b", ActorID: "u2"},
	}

	chain1 := buildChain(secret, entries)
	chain2 := buildChain(secret, entries)

	for i := range chain1 {
		if chain1[i].this != chain2[i].this {
			t.Fatalf("chain not deterministic at entry %d", i)
		}
	}
}

// ─── Tamper detection (pure logic, no DB) ────────────────────────────

// TestAuditTamperDetection verifies that modifying a middle entry's payload
// breaks the hash chain — all subsequent hashes diverge from the original.
func TestAuditTamperDetection(t *testing.T) {
	secret := []byte("tamper-secret")
	entries := []AuditEntry{
		{Action: "login", ActorID: "user1"},
		{Action: "view_profile", ActorID: "user1"},
		{Action: "update_email", ActorID: "user1"},
		{Action: "logout", ActorID: "user1"},
	}

	originalChain := buildChain(secret, entries)

	// Tamper: modify the 2nd entry's payload (index 1)
	tamperedEntries := make([]AuditEntry, len(entries))
	copy(tamperedEntries, entries)
	tamperedEntries[1] = AuditEntry{Action: "view_profile", ActorID: "hacker"} // changed actor

	tamperedChain := buildChain(secret, tamperedEntries)

	// Entry 0 should be unchanged (same payload before tamper point)
	if tamperedChain[0].this != originalChain[0].this {
		t.Fatal("entry 0 should be unchanged — tamper was at index 1")
	}

	// Entry 1 onward should all differ
	tamperDetected := false
	for i := 1; i < len(originalChain); i++ {
		if tamperedChain[i].this != originalChain[i].this {
			tamperDetected = true
			break
		}
	}
	if !tamperDetected {
		t.Fatal("tamper not detected: all hashes after tamper point are identical")
	}

	// Verify ALL entries after the tamper point differ (cascade effect)
	for i := 1; i < len(originalChain); i++ {
		if tamperedChain[i].this == originalChain[i].this {
			t.Fatalf("entry %d hash unchanged after tamper at entry 1 — chain should cascade", i)
		}
	}
}

// TestAuditTamperDetection_FirstEntry verifies tampering the genesis entry breaks the entire chain.
func TestAuditTamperDetection_FirstEntry(t *testing.T) {
	secret := []byte("tamper-first")
	entries := []AuditEntry{
		{Action: "init", ActorID: "admin"},
		{Action: "login", ActorID: "user1"},
		{Action: "logout", ActorID: "user1"},
	}

	originalChain := buildChain(secret, entries)

	// Tamper the genesis entry
	tamperedEntries := make([]AuditEntry, len(entries))
	copy(tamperedEntries, entries)
	tamperedEntries[0] = AuditEntry{Action: "init", ActorID: "attacker"}

	tamperedChain := buildChain(secret, tamperedEntries)

	// Every entry should differ because the prev_hash chain is broken from the start
	for i := range originalChain {
		if tamperedChain[i].this == originalChain[i].this {
			t.Fatalf("entry %d hash unchanged after genesis tamper", i)
		}
	}
}

// TestAuditTamperDetection_PrevHash verifies that changing a stored prev_hash
// (without changing payload) also breaks the chain.
func TestAuditTamperDetection_PrevHash(t *testing.T) {
	secret := []byte("tamper-prev")
	entries := []AuditEntry{
		{Action: "a", ActorID: "u1"},
		{Action: "b", ActorID: "u2"},
		{Action: "c", ActorID: "u3"},
	}

	originalChain := buildChain(secret, entries)

	// Simulate tampering: recompute entry 2 with a WRONG prev_hash
	payload2, _ := json.Marshal(entries[1])
	wrongPrevHash := "0000000000000000000000000000000000000000000000000000000000000000"
	tamperedHash := computeHash(secret, wrongPrevHash, payload2)

	if tamperedHash == originalChain[1].this {
		t.Fatal("tampered hash should differ from original when prev_hash is changed")
	}

	// Recompute entry 3 using the tampered hash as prev_hash
	payload3, _ := json.Marshal(entries[2])
	cascadedHash := computeHash(secret, tamperedHash, payload3)

	if cascadedHash == originalChain[2].this {
		t.Fatal("entry 3 hash should differ after entry 2 prev_hash tamper")
	}
}

// ─── Integration tests (require testcontainers PG) ───────────────────

// startPostgres starts a PostgreSQL testcontainer and returns a connection pool.
// Skips the test if Docker is unavailable or in short mode.
func startPostgres(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Skipf("skipping: postgres container unavailable (Docker not running?): %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("failed to create pool: %v", err)
	}

	// Run audit_logs migration directly
	migPath := migrationsDir(t)
	migFile := filepath.Join(migPath, "000006_create_audit_logs.up.sql")
	if err := applyMigration(ctx, pool, migFile); err != nil {
		pool.Close()
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("failed to apply migration: %v", err)
	}

	cleanup := func() {
		pool.Close()
		pgContainer.Terminate(ctx)
	}
	return pool, cleanup
}

// applyMigration reads and executes a .sql migration file against the pool.
func applyMigration(ctx context.Context, pool *pgxpool.Pool, path string) error {
	sql, err := os.ReadFile(path) //nolint:gosec // test path is controlled
	if err != nil {
		return fmt.Errorf("read migration %s: %w", path, err)
	}
	_, err = pool.Exec(ctx, string(sql))
	if err != nil {
		return fmt.Errorf("exec migration %s: %w", path, err)
	}
	return nil
}

// migrationsDir resolves the absolute path to backend/migrations.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// This file is at backend/internal/audit/audit_test.go
	// migrations are at backend/migrations/
	dir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve migrations path: %v", err)
	}
	return abs
}

// TestAuditDBHashChain verifies the full DB-backed hash chain:
// insert N entries via Log, read them back, and verify the chain is intact.
func TestAuditDBHashChain(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	secret := "db-chain-secret"
	InitDBLogger(pool, secret)

	ctx := context.Background()
	entries := []AuditEntry{
		{Action: "login", ActorID: "user1", ActorIP: "1.1.1.1", Resource: "auth"},
		{Action: "create_room", ActorID: "user1", ActorIP: "1.1.1.1", Resource: "room/1"},
		{Action: "delete_room", ActorID: "user2", ActorIP: "2.2.2.2", Resource: "room/1"},
	}

	for _, e := range entries {
		Log(ctx, e)
	}

	// Drain the channel — CloseDBLogger blocks until all queued entries are written.
	CloseDBLogger()
	dbLogger = nil // prevent double-close in subsequent tests

	// Read all entries back
	rows, err := pool.Query(ctx,
		`SELECT action, actor_id, actor_ip, resource, prev_hash, this_hash FROM audit_logs ORDER BY id`)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	defer rows.Close()

	type dbRecord struct {
		action, actorID, actorIP, resource, prevHash, thisHash string
	}
	var records []dbRecord
	for rows.Next() {
		var r dbRecord
		if err := rows.Scan(&r.action, &r.actorID, &r.actorIP, &r.resource, &r.prevHash, &r.thisHash); err != nil {
			t.Fatalf("scan: %v", err)
		}
		records = append(records, r)
	}

	if len(records) != len(entries) {
		t.Fatalf("expected %d records, got %d", len(entries), len(records))
	}

	// Verify chain: recompute each hash and compare
	prevHash := ""
	for i, r := range records {
		if r.prevHash != prevHash {
			t.Fatalf("record %d prev_hash mismatch: got %s, expected %s", i, r.prevHash, prevHash)
		}
		entry := AuditEntry{
			Action:   r.action,
			ActorID:  r.actorID,
			ActorIP:  r.actorIP,
			Resource: r.resource,
		}
		payload, _ := json.Marshal(entry)
		expected := computeHash([]byte(secret), prevHash, payload)
		if r.thisHash != expected {
			t.Fatalf("record %d this_hash mismatch: got %s, expected %s", i, r.thisHash, expected)
		}
		prevHash = r.thisHash
	}
}

// TestAuditConcurrentLog verifies concurrent safety: multiple goroutines
// calling Log simultaneously must not cause data races or corrupt the chain.
// Run with: go test -race ./internal/audit/...
func TestAuditConcurrentLog(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	secret := "concurrent-secret" //nolint:gosec // test secret
	InitDBLogger(pool, secret)

	ctx := context.Background()
	const goroutines = 10
	const perGoroutine = 20

	runConcurrentAuditLogs(ctx, goroutines, perGoroutine)

	// Drain the channel
	CloseDBLogger()
	dbLogger = nil // prevent double-close

	// Verify all entries were written
	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&count)
	if err != nil {
		t.Fatalf("count audit_logs: %v", err)
	}
	expected := goroutines * perGoroutine
	if count != expected {
		t.Fatalf("expected %d audit logs, got %d", expected, count)
	}

	verifyAuditChainIntegrity(t, ctx, pool, secret)
}

// runConcurrentAuditLogs spawns goroutines that each log perGoroutine entries.
func runConcurrentAuditLogs(ctx context.Context, goroutines, perGoroutine int) {
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				Log(ctx, AuditEntry{
					Action:   fmt.Sprintf("action-%d-%d", gid, i),
					ActorID:  fmt.Sprintf("user-%d", gid),
					ActorIP:  "10.0.0.1",
					Resource: fmt.Sprintf("resource/%d/%d", gid, i),
				})
			}
		}(g)
	}
	wg.Wait()
}

// verifyAuditChainIntegrity reads all audit_logs rows and verifies the HMAC chain is intact.
func verifyAuditChainIntegrity(t *testing.T, ctx context.Context, pool *pgxpool.Pool, secret string) {
	t.Helper()
	rows, err := pool.Query(ctx,
		`SELECT action, actor_id, actor_ip, resource, prev_hash, this_hash FROM audit_logs ORDER BY id`)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	defer rows.Close()

	prevHash := ""
	for rows.Next() {
		var action, actorID, actorIP, resource, storedPrev, storedThis string
		if err := rows.Scan(&action, &actorID, &actorIP, &resource, &storedPrev, &storedThis); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if storedPrev != prevHash {
			t.Fatalf("chain broken: prev_hash=%s, expected=%s", storedPrev, prevHash)
		}
		entry := AuditEntry{
			Action:   action,
			ActorID:  actorID,
			ActorIP:  actorIP,
			Resource: resource,
		}
		payload, _ := json.Marshal(entry)
		expectedHash := computeHash([]byte(secret), prevHash, payload)
		if storedThis != expectedHash {
			t.Fatalf("hash mismatch for action=%s: got %s, expected %s", action, storedThis, expectedHash)
		}
		prevHash = storedThis
	}
}

// TestInitDBLogger_NilPool verifies InitDBLogger is a no-op when pool is nil.
func TestInitDBLogger_NilPool(t *testing.T) {
	// Reset global state
	dbLogger = nil
	InitDBLogger(nil, "secret")
	if dbLogger != nil {
		t.Fatal("expected dbLogger to remain nil when pool is nil")
	}
}

// TestInitDBLogger_EmptySecret verifies InitDBLogger is a no-op when secret is empty.
func TestInitDBLogger_EmptySecret(t *testing.T) {
	dbLogger = nil
	InitDBLogger(nil, "")
	if dbLogger != nil {
		t.Fatal("expected dbLogger to remain nil when secret is empty")
	}
}

// TestCloseDBLogger_NotInitialized verifies CloseDBLogger is safe to call when not initialized.
func TestCloseDBLogger_NotInitialized(_ *testing.T) {
	dbLogger = nil
	// Should not panic
	CloseDBLogger()
}

// TestLog_NoDBLogger verifies Log works without a DB logger (stdout only).
func TestLog_NoDBLogger(_ *testing.T) {
	dbLogger = nil
	// Should not panic — just logs to stdout
	Log(context.Background(), AuditEntry{
		Action:  "test",
		ActorID: "user1",
	})
}
