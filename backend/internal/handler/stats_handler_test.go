package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestNewStatsHandler(t *testing.T) {
	t.Parallel()

	var db store.ResultRepository
	h := NewStatsHandler(&db)
	if h == nil || h.db != &db {
		t.Fatal("NewStatsHandler should store db reference")
	}
}

func TestGetUserStats_Unauthorized(t *testing.T) {
	t.Parallel()

	var db store.ResultRepository
	h := NewStatsHandler(&db)
	w := httptest.NewRecorder()
	h.GetUserStats(w, httptest.NewRequest(http.MethodGet, "/api/v1/user/stats", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGetLeaderboard_WithDB(t *testing.T) {
	mock := testutil.NewPgxMock(t)
	db := store.NewResultRepository(mock)
	h := NewStatsHandler(db)

	mock.ExpectQuery("SELECT final_score, lobby_code, ended_at").
		WithArgs(25).
		WillReturnRows(pgxmock.NewRows([]string{"final_score", "lobby_code", "ended_at"}).
			AddRow(100, "ABC12", int64(999)))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard?limit=25", nil)
	h.GetLeaderboard(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetLeaderboard_WeeklyWithDB(t *testing.T) {
	mock := testutil.NewPgxMock(t)
	db := store.NewResultRepository(mock)
	h := NewStatsHandler(db)

	mock.ExpectQuery("SELECT final_score, lobby_code, ended_at").
		WithArgs(pgxmock.AnyArg(), 50).
		WillReturnRows(pgxmock.NewRows([]string{"final_score", "lobby_code", "ended_at"}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard?scope=weekly", nil)
	h.GetLeaderboard(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetLeaderboard_DBError(t *testing.T) {
	mock := testutil.NewPgxMock(t)
	db := store.NewResultRepository(mock)
	h := NewStatsHandler(db)

	mock.ExpectQuery("SELECT final_score, lobby_code, ended_at").
		WithArgs(50).
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	h.GetLeaderboard(w, httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestGetUserStats_DBError(t *testing.T) {
	mock := testutil.NewPgxMock(t)
	db := store.NewResultRepository(mock)
	h := NewStatsHandler(db)

	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(score_contribution\\), 0\\), COUNT\\(\\*\\)").
		WithArgs("user-1").
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/user/stats", nil)
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-1", "nick"))
	h.GetUserStats(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestGetUserStats_WithDB(t *testing.T) {
	mock := testutil.NewPgxMock(t)
	db := store.NewResultRepository(mock)
	h := NewStatsHandler(db)

	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(score_contribution\\), 0\\), COUNT\\(\\*\\)").
		WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"max", "count"}).AddRow(50, 3))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/user/stats", nil)
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-1", "nick"))
	h.GetUserStats(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}
