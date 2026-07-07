package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

func TestUpdateUserLastLogin_Success(t *testing.T) {
	repo, mock := newMockUserRepository(t)
	ctx := context.Background()

	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	if err := repo.UpdateUserLastLogin(ctx, "user-1"); err != nil {
		t.Fatalf("UpdateUserLastLogin: %v", err)
	}
}

func TestUpdateUserLastLogin_Error(t *testing.T) {
	repo, mock := newMockUserRepository(t)
	ctx := context.Background()

	mock.ExpectExec("UPDATE users SET last_login").
		WillReturnError(errors.New("update failed"))

	if err := repo.UpdateUserLastLogin(ctx, "user-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestAnonymizeUser_Success(t *testing.T) {
	repo, mock := newMockUserRepository(t)
	ctx := context.Background()

	mock.ExpectExec("UPDATE users SET email").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "user-gdpr").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	if err := repo.AnonymizeUser(ctx, "user-gdpr"); err != nil {
		t.Fatalf("AnonymizeUser: %v", err)
	}
}

func TestAnonymizeUser_Error(t *testing.T) {
	repo, mock := newMockUserRepository(t)
	ctx := context.Background()

	mock.ExpectExec("UPDATE users SET email").
		WillReturnError(errors.New("update failed"))

	if err := repo.AnonymizeUser(ctx, "user-gdpr"); err == nil {
		t.Fatal("expected error")
	}
}

func TestHardDeleteExpiredUsers_Success(t *testing.T) {
	repo, mock := newMockUserRepository(t)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM users").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("DELETE 5"))

	deleted, err := repo.HardDeleteExpiredUsers(ctx, 30)
	if err != nil {
		t.Fatalf("HardDeleteExpiredUsers: %v", err)
	}
	if deleted != 5 {
		t.Fatalf("expected 5 deleted, got %d", deleted)
	}
}

func TestHardDeleteExpiredUsers_Error(t *testing.T) {
	repo, mock := newMockUserRepository(t)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM users").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("delete failed"))

	_, err := repo.HardDeleteExpiredUsers(ctx, 30)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHardDeleteExpiredUsers_DefaultRetention(t *testing.T) {
	repo, mock := newMockUserRepository(t)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM users").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

	if _, err := repo.HardDeleteExpiredUsers(ctx, 0); err != nil {
		t.Fatalf("HardDeleteExpiredUsers: %v", err)
	}
}