package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

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
			if tt.execErr != nil {
				exec.WillReturnError(tt.execErr)
			} else {
				exec.WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
			}
			err := repo.UpdateUserLastLogin(context.Background(), "user-1")
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("UpdateUserLastLogin: %v", err)
			}
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
			if tt.userErr != nil {
				exec.WillReturnError(tt.userErr)
			} else {
				exec.WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
				outboxExec := mock.ExpectExec("UPDATE outbox_events").WithArgs("user-gdpr")
				if tt.outboxErr != nil {
					outboxExec.WillReturnError(tt.outboxErr)
				} else {
					outboxExec.WillReturnResult(pgconn.NewCommandTag("UPDATE 0"))
				}
			}
			err := repo.AnonymizeUser(context.Background(), "user-gdpr")
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("AnonymizeUser: %v", err)
			}
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
			if tt.execErr != nil {
				exec.WillReturnError(tt.execErr)
			} else {
				exec.WillReturnResult(pgconn.NewCommandTag("DELETE 5"))
			}
			_, err := repo.HardDeleteExpiredUsers(context.Background(), tt.retention)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("HardDeleteExpiredUsers: %v", err)
			}
		})
	}
}
