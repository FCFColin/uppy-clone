// Package seed inserts test data for development environments.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/idgen"
	"github.com/uppy-clone/backend/internal/store"
)

func main() {
	status, err := runSeed(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	log.Println(status)
}

// newPostgresStoreFn is replaceable in unit tests.
var newPostgresStoreFn = store.NewPostgresStore

// newSeedReposFn creates user/result repos from the store. Replaceable in unit
// tests to inject mock-backed repos (db.Pool() returns nil for pgxmock).
var newSeedReposFn = newSeedRepos

func newSeedRepos(db *store.PostgresStore) (*store.UserRepository, *store.ResultRepository) {
	return store.NewUserRepository(db.Pool()), store.NewResultRepository(db.Pool())
}

// seedStats tracks successful insert counts per category for accurate reporting.
type seedStats struct {
	usersCreated    int
	sessionsCreated int
	resultsCreated  int
}

func runSeed(dbURL string) (string, error) {
	if dbURL == "" {
		return "", fmt.Errorf("DATABASE_URL required")
	}
	if err := validateDevDatabaseURL(dbURL); err != nil {
		return "", err
	}

	timeouts := config.DefaultTimeoutConfig()
	db, err := newPostgresStoreFn(dbURL, timeouts)
	if err != nil {
		return "", fmt.Errorf("connect DB: %w", err)
	}
	defer db.Close()

	userRepo, resultRepo := newSeedReposFn(db)

	ctx := context.Background()
	now := time.Now().Unix()
	stats := seedStats{}

	users, userErr := seedUsers(ctx, userRepo, now, &stats)
	if userErr != nil {
		return "", fmt.Errorf("seed users: %w", userErr)
	}

	sessionIDs, sessionErr := seedSessions(ctx, resultRepo, now, &stats)
	if sessionErr != nil {
		return "", fmt.Errorf("seed sessions: %w", sessionErr)
	}

	if err := seedResults(ctx, resultRepo, now, users, sessionIDs, &stats); err != nil {
		return "", fmt.Errorf("seed results: %w", err)
	}

	return fmt.Sprintf("Seed completed: %d users, %d game sessions, %d game results",
		stats.usersCreated, stats.sessionsCreated, stats.resultsCreated), nil
}

// validateDevDatabaseURL ensures the DATABASE_URL is safe for dev seeding.
// It parses the URL properly (not substring matching) and checks the final
// sslmode value — preventing bypass via ?sslmode=disable&sslmode=require
// where libpq takes the last value.
func validateDevDatabaseURL(dbURL string) error {
	u, err := url.Parse(dbURL)
	if err != nil {
		return fmt.Errorf("SEED ABORTED: invalid DATABASE_URL: %w", err)
	}
	q := u.Query()
	// libpq/pgx takes the LAST value for duplicate query keys. url.Values.Get()
	// returns the first, so we check all values and use the last one.
	sslmodes := q["sslmode"]
	if len(sslmodes) == 0 {
		return fmt.Errorf("SEED ABORTED: DATABASE_URL must contain sslmode=disable (dev only)")
	}
	finalSSLMode := sslmodes[len(sslmodes)-1]
	if finalSSLMode != "disable" {
		return fmt.Errorf("SEED ABORTED: DATABASE_URL sslmode must be disable (dev only), got %q", finalSSLMode)
	}
	// Reject if duplicate sslmode keys present (defensive — avoids ambiguity).
	if len(sslmodes) > 1 {
		return fmt.Errorf("SEED ABORTED: DATABASE_URL has multiple sslmode parameters (ambiguous)")
	}
	return nil
}

// isDuplicateError returns true for unique constraint violations (SQLSTATE 23505)
// or domain.ErrDuplicateUser, which indicate idempotent re-runs rather than
// real failures.
func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrDuplicateUser) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	return false
}

// seedUsers inserts 3 test users and returns them for use by other seed helpers.
// Duplicate-key errors are treated as non-fatal (idempotent re-runs); other
// errors are returned. The stats counter is incremented only on success.
func seedUsers(ctx context.Context, userRepo *store.UserRepository, now int64, stats *seedStats) ([]*domain.User, error) {
	users := []*domain.User{
		{ID: idgen.UUID(), Email: "alice@test.com", Nickname: "Alice", Palette: 0, CreatedAt: now},
		{ID: idgen.UUID(), Email: "bob@test.com", Nickname: "Bob", Palette: 1, CreatedAt: now},
		{ID: idgen.UUID(), Email: "charlie@test.com", Nickname: "Charlie", Palette: 2, CreatedAt: now},
	}
	for _, u := range users {
		if err := userRepo.CreateUser(ctx, u); err != nil {
			if isDuplicateError(err) {
				log.Printf("create user %s: %v (may already exist)", u.Nickname, err)
				continue
			}
			return users, fmt.Errorf("create user %s: %w", u.Nickname, err)
		}
		stats.usersCreated++
	}
	return users, nil
}

// seedSessions inserts 5 completed game sessions and returns their IDs.
// Duplicate-key errors are non-fatal; other errors are returned.
func seedSessions(ctx context.Context, resultRepo *store.ResultRepository, now int64, stats *seedStats) ([]string, error) {
	sessionIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		code := fmt.Sprintf("SEED%d", i)
		endedAt := now - int64(i*3600)
		startedAt := endedAt - 600
		session := &domain.GameSession{
			ID:         idgen.UUID(),
			LobbyCode:  code,
			Status:     "completed",
			StartedAt:  &startedAt,
			EndedAt:    &endedAt,
			FinalScore: 1000 - i*100,
		}
		sessionIDs[i] = session.ID
		if err := resultRepo.CreateGameSession(ctx, session); err != nil {
			if isDuplicateError(err) {
				log.Printf("create game session %s: %v (may already exist)", code, err)
				continue
			}
			return sessionIDs, fmt.Errorf("create game session %s: %w", code, err)
		}
		stats.sessionsCreated++
	}
	return sessionIDs, nil
}

// seedResults inserts 10 game results (2 per session, cycling through users).
// InsertSeedGameResult uses ON CONFLICT DO NOTHING, so duplicates are silently
// skipped — we count only successful inserts.
func seedResults(ctx context.Context, resultRepo *store.ResultRepository, now int64, users []*domain.User, sessionIDs []string, stats *seedStats) error {
	for i := 0; i < 10; i++ {
		sessionIdx := i / 2
		userIdx := i % len(users)
		if sessionIdx >= len(sessionIDs) {
			sessionIdx = 0
		}
		if userIdx >= len(users) {
			userIdx = 0
		}
		result := &domain.GameResult{
			ID:                idgen.UUID(),
			SessionID:         sessionIDs[sessionIdx],
			UserID:            users[userIdx].ID,
			ScoreContribution: 500 - i*30,
			TapsCount:         20 + i*3,
			CreatedAt:         now - int64(i*60),
		}
		if err := resultRepo.InsertSeedGameResult(ctx, result); err != nil {
			if isDuplicateError(err) {
				continue
			}
			return fmt.Errorf("create game result %d: %w", i, err)
		}
		stats.resultsCreated++
	}
	return nil
}
