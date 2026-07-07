package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/store"
)

func TestNewStatsHandler(t *testing.T) {
	t.Parallel()

	var db store.PostgresStore
	h := NewStatsHandler(&db)
	if h == nil || h.db != &db {
		t.Fatal("NewStatsHandler should store db reference")
	}
}

func TestGetLeaderboard_NilDB(t *testing.T) {
	t.Parallel()

	h := NewStatsHandler(nil)
	w := httptest.NewRecorder()
	h.GetLeaderboard(w, httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestGetLeaderboard_InvalidScope(t *testing.T) {
	t.Parallel()

	var db store.PostgresStore
	h := NewStatsHandler(&db)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard?scope=daily", nil)
	h.GetLeaderboard(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetLeaderboard_InvalidLimitIgnored(t *testing.T) {
	t.Parallel()

	h := NewStatsHandler(nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard?limit=abc", nil)
	h.GetLeaderboard(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 with nil db", w.Code)
	}
}

func TestGetLeaderboard_WeeklyScopeNilDB(t *testing.T) {
	t.Parallel()

	h := NewStatsHandler(nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard?scope=weekly", nil)
	h.GetLeaderboard(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestGetUserStats_NilDB(t *testing.T) {
	t.Parallel()

	h := NewStatsHandler(nil)
	w := httptest.NewRecorder()
	h.GetUserStats(w, httptest.NewRequest(http.MethodGet, "/api/v1/user/stats", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestGetUserStats_Unauthorized(t *testing.T) {
	t.Parallel()

	var db store.PostgresStore
	h := NewStatsHandler(&db)
	w := httptest.NewRecorder()
	h.GetUserStats(w, httptest.NewRequest(http.MethodGet, "/api/v1/user/stats", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGetLeaderboard_ResponseShape(t *testing.T) {
	t.Parallel()

	h := NewStatsHandler(nil)
	w := httptest.NewRecorder()
	h.GetLeaderboard(w, httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard", nil))

	var body map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["title"]; !ok {
		t.Errorf("expected title field in degraded response, got %s", w.Body.String())
	}
}

func TestGetLeaderboard_WithDB(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
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
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
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
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
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
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
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
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
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
