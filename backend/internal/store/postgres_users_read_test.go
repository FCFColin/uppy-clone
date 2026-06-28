package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestGetUserByEmail_Found(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	lastLogin := int64(200)
	rows := pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
		AddRow("user-1", "found@example.com", "Found", 2, int64(100), &lastLogin)
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "found@example.com").
		WillReturnRows(rows)

	user, err := s.GetUserByEmail(ctx, "found@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID != "user-1" || user.Email != "found@example.com" || user.Nickname != "Found" {
		t.Errorf("user = %+v", user)
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "missing@example.com").
		WillReturnError(pgx.ErrNoRows)

	user, err := s.GetUserByEmail(ctx, "missing@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user != nil {
		t.Fatalf("expected nil user, got %+v", user)
	}
}

func TestGetUserByEmail_ScanError(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
		AddRow("user-1", 123, "Bad", 0, int64(1), int64(2))
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WillReturnRows(rows)

	_, err := s.GetUserByEmail(ctx, "bad@example.com")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestGetUserByID_Found(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	lastLogin := int64(60)
	rows := pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
		AddRow("id-42", "byid@example.com", "ByID", 3, int64(50), &lastLogin)
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs("id-42").
		WillReturnRows(rows)

	user, err := s.GetUserByID(ctx, "id-42")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user == nil || user.ID != "id-42" {
		t.Fatalf("user = %+v", user)
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	s, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs("missing-id").
		WillReturnError(pgx.ErrNoRows)

	user, err := s.GetUserByID(ctx, "missing-id")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user != nil {
		t.Fatalf("expected nil, got %+v", user)
	}
}
