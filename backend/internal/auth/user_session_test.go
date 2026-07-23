package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestFindOrCreateUserByEmail_CreatesUser(t *testing.T) {
	mock := testutil.NewPgxMock(t)

	db := store.NewUserRepository(mock)
	ctx := context.Background()

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "new@example.com").
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "new", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs("user", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	user, err := findOrCreateUserByEmail(ctx, db, "new@example.com")
	if err != nil {
		t.Fatalf("findOrCreateUserByEmail: %v", err)
	}
	if user == nil || user.Email != "new@example.com" || user.Nickname != "new" {
		t.Fatalf("user = %+v", user)
	}
}

func TestIssueMagicLinkSession(t *testing.T) {
	mock := testutil.NewPgxMock(t)

	db := store.NewUserRepository(mock)
	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	jwtMgr, refreshMgr := setupRefreshEnv(t)
	user := &domain.User{ID: "user-1", Nickname: "Magic", Email: "magic@example.com"}

	cookie, resp, err := issueMagicLinkSession(context.Background(), db, jwtMgr, refreshMgr, user, httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("issueMagicLinkSession: %v", err)
	}
	if cookie == nil || resp.RefreshToken == "" {
		t.Fatalf("cookie=%+v resp=%+v", cookie, resp)
	}
}

func TestIssueMagicLinkSession_LastLoginErrorIgnored(t *testing.T) {
	mock := testutil.NewPgxMock(t)

	db := store.NewUserRepository(mock)
	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-1").
		WillReturnError(errors.New("update failed"))

	jwtMgr, refreshMgr := setupRefreshEnv(t)
	user := &domain.User{ID: "user-1", Nickname: "Magic", Email: "magic@example.com"}

	_, resp, err := issueMagicLinkSession(context.Background(), db, jwtMgr, refreshMgr, user, httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("issueMagicLinkSession should continue when last login update fails: %v", err)
	}
	if resp == nil || resp.RefreshToken == "" {
		t.Fatalf("resp = %+v", resp)
	}
}
