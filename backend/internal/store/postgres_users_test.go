package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

// --- User create ---

func TestCreateUser(t *testing.T) {
	lastLogin := int64(100)
	tests := []struct {
		name      string
		user      *domain.User
		setup     func(mock pgxmock.PgxPoolIface)
		preHook   func() func() // returns cleanup; runs before repo creation
		wantErr   string        // empty: no error expected
		wantIsErr error
	}{
		{
			name: "success",
			user: &domain.User{ID: "user-1", Email: "create@example.com", Nickname: "Creator", Palette: 1, CreatedAt: 100, LastLogin: &lastLogin},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO users").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
				mock.ExpectExec("INSERT INTO outbox_events").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
				mock.ExpectCommit()
			},
		},
		{
			name: "duplicate user",
			user: &domain.User{ID: "user-dup", Email: "dup@example.com", Nickname: "Dup", CreatedAt: 1},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO users").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnError(&pgconn.PgError{Code: "23505"})
				mock.ExpectRollback()
			},
			wantIsErr: ErrDuplicateUser,
		},
		{
			name: "begin error",
			user: &domain.User{ID: "u1", Email: "a@b.com", Nickname: "n"},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
			},
			wantErr: "begin tx",
		},
		{
			name: "commit error",
			user: &domain.User{ID: "u4", Email: "d@e.com", Nickname: "n", CreatedAt: 1},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO users").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
				mock.ExpectExec("INSERT INTO outbox_events").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
				mock.ExpectRollback()
			},
			wantErr: "commit create user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.preHook != nil {
				cleanup := tt.preHook()
				t.Cleanup(cleanup)
			}
			repo, mock := newMockRepo(t, NewUserRepository)
			tt.setup(mock)
			err := repo.CreateUser(context.Background(), tt.user)
			if tt.wantIsErr != nil {
				if !errors.Is(err, tt.wantIsErr) {
					t.Fatalf("CreateUser = %v, want %v", err, tt.wantIsErr)
				}
				return
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("CreateUser = %v, want %q error", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations: %v", err)
			}
		})
	}
}

// --- Email storage helpers ---

func TestPrepareEmailForStorage(t *testing.T) {
	t.Parallel()

	hash, stored, err := prepareEmailForStorage("user@example.com")
	if err != nil {
		t.Fatalf("prepareEmailForStorage: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty email hash")
	}
	if stored == "" {
		t.Fatal("expected non-empty stored email")
	}
	if hash != crypto.EmailHMAC("user@example.com") {
		t.Error("hash should match EmailHMAC of input")
	}
}

func TestPrepareEmailForStorage_WithEncryptionKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto.InitFromEnv: %v", err)
	}

	hash, stored, err := prepareEmailForStorage("secure@example.com")
	if err != nil {
		t.Fatalf("prepareEmailForStorage: %v", err)
	}
	if hash == "" || stored == "" {
		t.Fatal("expected hash and stored email")
	}
	if stored == "secure@example.com" {
		t.Error("expected encrypted stored email when key is set")
	}
	if !strings.HasPrefix(stored, "v1:") {
		t.Errorf("stored email = %q, want v1: prefix", stored)
	}
}

func TestPrepareEmailForStorage_EncryptError(t *testing.T) {
	orig := encryptEmailForStorageFn
	t.Cleanup(func() { encryptEmailForStorageFn = orig })
	encryptEmailForStorageFn = func(string) (string, error) {
		return "", fmt.Errorf("encrypt failed")
	}

	_, _, err := prepareEmailForStorage("fail@example.com")
	if err == nil {
		t.Fatal("expected encrypt error")
	}
	if !strings.Contains(err.Error(), "encrypt email") {
		t.Errorf("error = %v, want encrypt email wrapper", err)
	}
}

func TestEmailFromStorage(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto.InitFromEnv: %v", err)
	}

	// Round-trip: prepare then read back.
	_, stored, err := prepareEmailForStorage("roundtrip@example.com")
	if err != nil {
		t.Fatalf("prepareEmailForStorage: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "plaintext legacy", input: "legacy@example.com", want: "legacy@example.com"},
		{name: "encrypted round trip", input: stored, want: "roundtrip@example.com"},
		{name: "corrupted ciphertext", input: "v1:00112233445566778899aabbccddeeff", wantErr: "decrypt email"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := emailFromStorage(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("emailFromStorage = %v, want %q error", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("emailFromStorage: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- GDPR lifecycle ---

func TestUpdateUserLastLogin(t *testing.T) {
	tests := []struct {
		name    string
		execErr error
		wantErr bool
	}{
		{"success", nil, false},
		{"error", errors.New("update failed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewUserRepository)
			exec := mock.ExpectExec("UPDATE users SET last_login").WithArgs("user-1")
			expectExecResult(exec, tt.execErr, "UPDATE 1")
			err := repo.UpdateUserLastLogin(context.Background(), "user-1")
			assertWantErr(t, err, tt.wantErr, "UpdateUserLastLogin")
		})
	}
}

func TestAnonymizeUser(t *testing.T) {
	tests := []struct {
		name      string
		userErr   error
		outboxErr error
		wantErr   bool
	}{
		{"success", nil, nil, false},
		{"user update error", errors.New("update failed"), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewUserRepository)
			exec := mock.ExpectExec("UPDATE users SET email").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "user-gdpr")
			expectExecResult(exec, tt.userErr, "UPDATE 1")
			if tt.userErr == nil {
				outboxExec := mock.ExpectExec("UPDATE outbox_events").WithArgs("user-gdpr")
				expectExecResult(outboxExec, tt.outboxErr, "UPDATE 0")
			}
			err := repo.AnonymizeUser(context.Background(), "user-gdpr")
			assertWantErr(t, err, tt.wantErr, "AnonymizeUser")
		})
	}
}

func TestHardDeleteExpiredUsers(t *testing.T) {
	tests := []struct {
		name      string
		retention int
		execErr   error
		wantErr   bool
	}{
		{"success", 30, nil, false},
		{"error", 30, errors.New("delete failed"), true},
		{"default retention", 0, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewUserRepository)
			exec := mock.ExpectExec("DELETE FROM users").WithArgs(pgxmock.AnyArg())
			expectExecResult(exec, tt.execErr, "DELETE 5")
			_, err := repo.HardDeleteExpiredUsers(context.Background(), tt.retention)
			assertWantErr(t, err, tt.wantErr, "HardDeleteExpiredUsers")
		})
	}
}

// --- User reads ---

func TestGetUserByEmail(t *testing.T) {
	lastLogin := int64(200)
	tests := []struct {
		name     string
		email    string
		queryErr error
		rows     *pgxmock.Rows
		wantNil  bool
	}{
		{
			name:  "found",
			email: "found@example.com",
			rows: pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
				AddRow("user-1", "found@example.com", "Found", 2, int64(100), &lastLogin),
		},
		{
			name:     "not found",
			email:    "missing@example.com",
			queryErr: pgx.ErrNoRows,
			wantNil:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewUserRepository)
			q := mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
				WithArgs(pgxmock.AnyArg(), tt.email)
			if tt.queryErr != nil {
				q.WillReturnError(tt.queryErr)
			} else {
				q.WillReturnRows(tt.rows)
			}
			user, err := repo.GetUserByEmail(context.Background(), tt.email)
			if err != nil {
				t.Fatalf("GetUserByEmail: %v", err)
			}
			if tt.wantNil && user != nil {
				t.Fatalf("expected nil, got %+v", user)
			}
			if !tt.wantNil && user == nil {
				t.Fatal("expected non-nil user")
			}
		})
	}
}

// TestGetUserByEmail_SoftDeletedFilterParenthesized verifies store-020: the OR
// branch is parenthesized so deleted_at IS NULL filters the entire WHERE.
func TestGetUserByEmail_SoftDeletedFilterParenthesized(t *testing.T) {
	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()

	lastLogin := int64(300)
	rows := pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
		AddRow("user-1", "active@example.com", "Active", 1, int64(100), &lastLogin)
	mock.ExpectQuery(`WHERE \(email_hash = \$1 OR`).
		WithArgs(pgxmock.AnyArg(), "active@example.com").
		WillReturnRows(rows)

	user, err := repo.GetUserByEmail(ctx, "active@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user == nil || user.ID != "user-1" {
		t.Fatalf("user = %+v", user)
	}
}

func TestGetUserByEmail_DecryptError(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto.InitFromEnv: %v", err)
	}

	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()

	lastLogin := int64(2)
	rows := pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
		AddRow("user-1", "v1:00112233445566778899aabbccddeeff", "Bad", 0, int64(1), &lastLogin)
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "bad@example.com").
		WillReturnRows(rows)

	_, err := repo.GetUserByEmail(ctx, "bad@example.com")
	if err == nil || !strings.Contains(err.Error(), "decrypt email") {
		t.Fatalf("GetUserByEmail = %v, want decrypt error", err)
	}
}

func TestGetUserByID(t *testing.T) {
	lastLogin := int64(60)
	tests := []struct {
		name     string
		id       string
		queryErr error
		rows     *pgxmock.Rows
		wantNil  bool
	}{
		{
			name: "found",
			id:   "id-42",
			rows: pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
				AddRow("id-42", "byid@example.com", "ByID", 3, int64(50), &lastLogin),
		},
		{
			name:     "not found",
			id:       "missing-id",
			queryErr: pgx.ErrNoRows,
			wantNil:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewUserRepository)
			q := mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
				WithArgs(tt.id)
			if tt.queryErr != nil {
				q.WillReturnError(tt.queryErr)
			} else {
				q.WillReturnRows(tt.rows)
			}
			user, err := repo.GetUserByID(context.Background(), tt.id)
			if err != nil {
				t.Fatalf("GetUserByID: %v", err)
			}
			if tt.wantNil && user != nil {
				t.Fatalf("expected nil, got %+v", user)
			}
			if !tt.wantNil && user == nil {
				t.Fatal("expected non-nil user")
			}
		})
	}
}
