package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/domain"
)

func TestSaveLobbyState(t *testing.T) {
	tests := []struct {
		name    string
		execErr error
		wantErr bool
	}{
		{"success", nil, false},
		{"error", errors.New("save failed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewLobbyRepository)
			exec := mock.ExpectExec("INSERT INTO lobby_states").
				WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg())
			expectExecResult(exec, tt.execErr, "INSERT 1")
			err := repo.SaveLobbyState(context.Background(), &domain.LobbyState{ID: "l1", Code: "ABCD1", State: "waiting", UpdatedAt: 100, CreatedAt: 50})
			assertWantErr(t, err, tt.wantErr, "SaveLobbyState")
		})
	}
}

func TestLoadLobbyState(t *testing.T) {
	tests := []struct {
		name     string
		queryErr error
		rowErr   error
		rows     *pgxmock.Rows
		wantNil  bool
		wantErr  bool
	}{
		{
			name: "found",
			rows: pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).
				AddRow("l1", "ABCD1", "playing", int64(200), int64(100)),
		},
		{
			name:     "not found",
			queryErr: pgx.ErrNoRows,
			wantNil:  true,
		},
		{
			name:    "scan error",
			rows:    pgxmock.NewRows([]string{"id", "code", "state", "updated_at", "created_at"}).AddRow("l1", "ABCD1", "playing", int64(200), int64(100)),
			rowErr:  errors.New("scan failed"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewLobbyRepository)
			q := mock.ExpectQuery("SELECT id, code, state, updated_at, created_at FROM lobby_states WHERE code").
				WithArgs("ABCD1")
			if tt.queryErr != nil {
				q.WillReturnError(tt.queryErr)
			} else {
				if tt.rowErr != nil {
					tt.rows.RowError(0, tt.rowErr)
				}
				q.WillReturnRows(tt.rows)
			}
			ls, err := repo.LoadLobbyState(context.Background(), "ABCD1")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadLobbyState: %v", err)
			}
			if tt.wantNil && ls != nil {
				t.Fatalf("expected nil, got %+v", ls)
			}
			if !tt.wantNil && ls == nil {
				t.Fatal("expected non-nil lobby state")
			}
		})
	}
}

func TestDeleteLobbyState(t *testing.T) {
	tests := []struct {
		name    string
		execErr error
		wantErr bool
	}{
		{"success", nil, false},
		{"error", errors.New("delete failed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewLobbyRepository)
			exec := mock.ExpectExec("DELETE FROM lobby_states WHERE code").
				WithArgs("ABCD1")
			expectExecResult(exec, tt.execErr, "DELETE 1")
			err := repo.DeleteLobbyState(context.Background(), "ABCD1")
			assertWantErr(t, err, tt.wantErr, "DeleteLobbyState")
		})
	}
}
