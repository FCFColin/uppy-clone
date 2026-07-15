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

func TestLogUserCreateAudit_DoesNotPanic(_ *testing.T) {
	logUserCreateAudit(context.Background(), &domain.User{
		ID:       "user-1",
		Nickname: "TestPlayer",
	})
}

func TestCreateUser_Success(t *testing.T) {
	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()
	lastLogin := int64(100)
	user := &domain.User{
		ID:        "user-1",
		Email:     "create@example.com",
		Nickname:  "Creator",
		Palette:   1,
		CreatedAt: 100,
		LastLogin: &lastLogin,
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(user.ID, pgxmock.AnyArg(), pgxmock.AnyArg(), user.Nickname, user.Palette, user.CreatedAt, user.LastLogin).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs("user", user.ID, pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreateUser_DuplicateUser(t *testing.T) {
	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()
	user := &domain.User{
		ID:        "user-dup",
		Email:     "dup@example.com",
		Nickname:  "Dup",
		CreatedAt: 1,
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(&pgconn.PgError{Code: "23505"})
	mock.ExpectRollback()

	err := repo.CreateUser(ctx, user)
	if !errors.Is(err, ErrDuplicateUser) {
		t.Fatalf("CreateUser = %v, want ErrDuplicateUser", err)
	}
}

func TestCreateUser_BeginError(t *testing.T) {
	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

	err := repo.CreateUser(ctx, &domain.User{ID: "u1", Email: "a@b.com", Nickname: "n"})
	if err == nil || !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("CreateUser = %v, want begin error", err)
	}
}

func TestCreateUser_OutboxInsertError(t *testing.T) {
	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()
	user := &domain.User{ID: "u2", Email: "b@c.com", Nickname: "n", CreatedAt: 1}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("outbox insert failed"))
	mock.ExpectRollback()

	err := repo.CreateUser(ctx, user)
	if err == nil || !strings.Contains(err.Error(), "insert outbox event") {
		t.Fatalf("CreateUser = %v, want outbox insert error", err)
	}
}

func TestCreateUser_PrepareEmailError(t *testing.T) {
	orig := encryptEmailForStorageFn
	t.Cleanup(func() { encryptEmailForStorageFn = orig })
	encryptEmailForStorageFn = func(string) (string, error) {
		return "", fmt.Errorf("encrypt failed")
	}

	repo, _ := newMockRepo(t, NewUserRepository)
	err := repo.CreateUser(context.Background(), &domain.User{ID: "u5", Email: "e@f.com", Nickname: "n"})
	if err == nil || !strings.Contains(err.Error(), "encrypt email") {
		t.Fatalf("CreateUser = %v, want encrypt email error", err)
	}
}

func TestCreateUser_InsertError(t *testing.T) {
	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()
	user := &domain.User{ID: "u3", Email: "c@d.com", Nickname: "n", CreatedAt: 1}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	err := repo.CreateUser(ctx, user)
	if err == nil || !strings.Contains(err.Error(), "create user") {
		t.Fatalf("CreateUser = %v, want create user error", err)
	}
}

func TestCreateUser_CommitError(t *testing.T) {
	repo, mock := newMockRepo(t, NewUserRepository)
	ctx := context.Background()
	user := &domain.User{ID: "u4", Email: "d@e.com", Nickname: "n", CreatedAt: 1}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
	mock.ExpectRollback()

	err := repo.CreateUser(ctx, user)
	if err == nil || !strings.Contains(err.Error(), "commit create user") {
		t.Fatalf("CreateUser = %v, want commit error", err)
	}
}
