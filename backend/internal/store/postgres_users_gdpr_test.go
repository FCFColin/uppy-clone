package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

func TestUpdateUserLastLogin_Success(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	if err := s.UpdateUserLastLogin(ctx, "user-1"); err != nil {
		t.Fatalf("UpdateUserLastLogin: %v", err)
	}
}

func TestUpdateUserLastLogin_Error(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectExec("UPDATE users SET last_login").
		WillReturnError(errors.New("update failed"))

	if err := s.UpdateUserLastLogin(ctx, "user-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestAnonymizeUser_Success(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("UPDATE users SET email").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "user-gdpr").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	mock.ExpectCommit()

	if err := s.AnonymizeUser(ctx, "user-gdpr"); err != nil {
		t.Fatalf("AnonymizeUser: %v", err)
	}
}

func TestAnonymizeUser_Error(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("UPDATE users SET email").
		WillReturnError(errors.New("anonymize failed"))

	if err := s.AnonymizeUser(ctx, "user-gdpr"); err == nil {
		t.Fatal("expected error")
	}
}

func TestAnonymizeUser_EncryptError(t *testing.T) {
	orig := encryptEmailForStorageFn
	t.Cleanup(func() { encryptEmailForStorageFn = orig })
	encryptEmailForStorageFn = func(string) (string, error) {
		return "", fmt.Errorf("encrypt failed")
	}

	s, _ := newMockPostgresStore(t)
	if err := s.AnonymizeUser(context.Background(), "user-gdpr"); err == nil {
		t.Fatal("expected encrypt error")
	}
}

func TestHardDeleteExpiredUsers_DefaultRetention(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM users WHERE deleted_at IS NOT NULL").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("DELETE 2"))

	deleted, err := s.HardDeleteExpiredUsers(ctx, 0)
	if err != nil {
		t.Fatalf("HardDeleteExpiredUsers: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
}

func TestHardDeleteExpiredUsers_Error(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM users WHERE deleted_at IS NOT NULL").
		WillReturnError(errors.New("delete failed"))

	_, err := s.HardDeleteExpiredUsers(ctx, 30)
	if err == nil {
		t.Fatal("expected error")
	}
}
