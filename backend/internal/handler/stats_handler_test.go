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

func TestGetUserStats_Unauthorized(t *testing.T) {
	t.Parallel()

	var db store.ResultRepository
	h := NewStatsHandler(&db, nil)
	w := httptest.NewRecorder()
	h.GetUserStats(w, httptest.NewRequest(http.MethodGet, "/api/v1/user/stats", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGetLeaderboard_WithDB(t *testing.T) {
	h, mock := newStatsHandlerWithDB(t)

	mock.ExpectQuery(`SELECT MAX\(gr\.score_contribution\)`).
		WithArgs(25).
		WillReturnRows(pgxmock.NewRows([]string{"best_score", "display_name", "best_at"}).
			AddRow(100, "ABC12", int64(999)))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard?limit=25", nil)
	h.GetLeaderboard(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetLeaderboard_WeeklyWithDB(t *testing.T) {
	h, mock := newStatsHandlerWithDB(t)

	mock.ExpectQuery(`SELECT MAX\(gr\.score_contribution\)`).
		WithArgs(pgxmock.AnyArg(), 50).
		WillReturnRows(pgxmock.NewRows([]string{"best_score", "display_name", "best_at"}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard?scope=weekly", nil)
	h.GetLeaderboard(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetLeaderboard_DBError(t *testing.T) {
	h, mock := newStatsHandlerWithDB(t)

	mock.ExpectQuery(`SELECT MAX\(gr\.score_contribution\)`).
		WithArgs(50).
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	h.GetLeaderboard(w, httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestGetUserStats_DBError(t *testing.T) {
	h, mock := newStatsHandlerWithDB(t)

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
	h, mock := newStatsHandlerWithDB(t)

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

// fakePlayerCounter is a test-only PlayerCounter that returns fixed counts.
type fakePlayerCounter struct {
	players int
	rooms   int
}

func (f fakePlayerCounter) PlayerCount() int { return f.players }
func (f fakePlayerCounter) RoomCount() int   { return f.rooms }

func TestGetPublicStats_Success(t *testing.T) {
	h, mock := newStatsHandlerWithDB(t)
	h.hub = fakePlayerCounter{players: 10, rooms: 2}

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM game_sessions").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(score_contribution\\), 0\\) FROM game_results").
		WillReturnRows(pgxmock.NewRows([]string{"best"}).AddRow(100))

	w := httptest.NewRecorder()
	h.GetPublicStats(w, httptest.NewRequest(http.MethodGet, "/api/v1/stats/public", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		OnlinePlayers int `json:"onlinePlayers"`
		GamesToday    int `json:"gamesToday"`
		BestScore     int `json:"bestScore"`
		ActiveRooms   int `json:"activeRooms"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.OnlinePlayers != 10 {
		t.Errorf("onlinePlayers = %d, want 10", resp.OnlinePlayers)
	}
	if resp.ActiveRooms != 2 {
		t.Errorf("activeRooms = %d, want 2", resp.ActiveRooms)
	}
	if resp.GamesToday != 5 {
		t.Errorf("gamesToday = %d, want 5", resp.GamesToday)
	}
	if resp.BestScore != 100 {
		t.Errorf("bestScore = %d, want 100", resp.BestScore)
	}
}

func TestGetPublicStats_NilDB(t *testing.T) {
	h := NewStatsHandler(nil, nil)

	w := httptest.NewRecorder()
	h.GetPublicStats(w, httptest.NewRequest(http.MethodGet, "/api/v1/stats/public", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Degraded bool   `json:"degraded"`
		Message  string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !resp.Degraded {
		t.Errorf("degraded = false, want true")
	}
}

func TestGetPublicStats_DBError(t *testing.T) {
	h, mock := newStatsHandlerWithDB(t)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM game_sessions").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	h.GetPublicStats(w, httptest.NewRequest(http.MethodGet, "/api/v1/stats/public", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
}
