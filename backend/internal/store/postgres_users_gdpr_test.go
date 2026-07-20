package store

import (
	"context"
	"errors"
	"testing"

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
