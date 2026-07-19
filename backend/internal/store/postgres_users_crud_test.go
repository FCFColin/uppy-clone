package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/domain"
)

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
			name: "outbox insert error",
			user: &domain.User{ID: "u2", Email: "b@c.com", Nickname: "n", CreatedAt: 1},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO users").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
				mock.ExpectExec("INSERT INTO outbox_events").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnError(errors.New("outbox insert failed"))
				mock.ExpectRollback()
			},
			wantErr: "insert outbox event",
		},
		{
			name:  "prepare email error",
			user:  &domain.User{ID: "u5", Email: "e@f.com", Nickname: "n"},
			setup: func(_ pgxmock.PgxPoolIface) {},
			preHook: func() func() {
				orig := encryptEmailForStorageFn
				encryptEmailForStorageFn = func(string) (string, error) {
					return "", fmt.Errorf("encrypt failed")
				}
				return func() { encryptEmailForStorageFn = orig }
			},
			wantErr: "encrypt email",
		},
		{
			name: "insert error",
			user: &domain.User{ID: "u3", Email: "c@d.com", Nickname: "n", CreatedAt: 1},
			setup: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO users").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnError(errors.New("insert failed"))
				mock.ExpectRollback()
			},
			wantErr: "create user",
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
