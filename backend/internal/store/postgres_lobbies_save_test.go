package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestSaveLobbyState_Success(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()
	ls := &domain.LobbyState{ID: "l1", Code: "ABCD1", State: "waiting", UpdatedAt: 100, CreatedAt: 50}

	mock.ExpectExec("INSERT INTO lobby_states").
		WithArgs(ls.ID, ls.Code, ls.State, ls.UpdatedAt, ls.CreatedAt).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))

	if err := repo.SaveLobbyState(ctx, ls); err != nil {
		t.Fatalf("SaveLobbyState: %v", err)
	}
}

func TestSaveLobbyState_Error(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO lobby_states").
		WillReturnError(errors.New("save failed"))

	err := repo.SaveLobbyState(ctx, &domain.LobbyState{ID: "l1", Code: "X"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadLobbyState_Found(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
		AddRow("l1", "ABCD1", "playing", int64(200), int64(100))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
		WithArgs("ABCD1").
		WillReturnRows(rows)

	ls, err := repo.LoadLobbyState(ctx, "ABCD1")
	if err != nil {
		t.Fatalf("LoadLobbyState: %v", err)
	}
	if ls == nil || ls.Code != "ABCD1" || ls.State != "playing" {
		t.Fatalf("lobby = %+v", ls)
	}
}

func TestLoadLobbyState_NotFound(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
		WithArgs("MISSING").
		WillReturnError(pgx.ErrNoRows)

	ls, err := repo.LoadLobbyState(ctx, "MISSING")
	if err != nil {
		t.Fatalf("LoadLobbyState: %v", err)
	}
	if ls != nil {
		t.Fatalf("expected nil, got %+v", ls)
	}
}

func TestDeleteLobbyState_Success(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM lobby_states WHERE code").
		WithArgs("ABCD1").
		WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

	if err := repo.DeleteLobbyState(ctx, "ABCD1"); err != nil {
		t.Fatalf("DeleteLobbyState: %v", err)
	}
}

func TestDeleteLobbyState_Error(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM lobby_states WHERE code").
		WillReturnError(errors.New("delete failed"))

	if err := repo.DeleteLobbyState(ctx, "ABCD1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadLobbyState_ScanError(t *testing.T) {
	repo, mock := newMockLobbyRepository(t)
	ctx := context.Background()

	rows := pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
		AddRow("l1", "ABCD1", "playing", int64(200), int64(100)).
		RowError(0, errors.New("scan failed"))
	mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
		WithArgs("ABCD1").
		WillReturnRows(rows)

	_, err := repo.LoadLobbyState(ctx, "ABCD1")
	if err == nil {
		t.Fatal("expected scan error")
	}
}