package store

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

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

func TestGetUserByID_DecryptError(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto.InitFromEnv: %v", err)
	}

	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()

	lastLogin := int64(2)
	rows := pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
		AddRow("id-42", "v1:00112233445566778899aabbccddeeff", "Bad", 0, int64(1), &lastLogin)
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs("id-42").
		WillReturnRows(rows)

	_, err := repo.GetUserByID(ctx, "id-42")
	if err == nil || !strings.Contains(err.Error(), "decrypt email") {
		t.Fatalf("GetUserByID = %v, want decrypt error", err)
	}
}
