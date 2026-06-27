package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/store"
)

// StatsHandler serves public leaderboard and optional user stats.
type StatsHandler struct {
	db *store.PostgresStore
}

// NewStatsHandler creates a StatsHandler.
func NewStatsHandler(db *store.PostgresStore) *StatsHandler {
	return &StatsHandler{db: db}
}

// GetLeaderboard handles GET /api/v1/leaderboard?scope=global|weekly&limit=50
func (h *StatsHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "global"
	}
	if scope != "global" && scope != "weekly" {
		http.Error(w, `{"error":"invalid scope"}`, http.StatusBadRequest)
		return
	}

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	entries, err := h.db.GetLeaderboard(r.Context(), scope, limit)
	if err != nil {
		http.Error(w, `{"error":"failed to load leaderboard"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"scope":   scope,
		"entries": entries,
	})
}

// GetUserStats handles GET /api/v1/user/stats (authenticated).
func (h *StatsHandler) GetUserStats(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	userID, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	bestScore, gamesPlayed, err := h.db.GetUserBestScore(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"failed to load stats"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"bestScore":   bestScore,
		"gamesPlayed": gamesPlayed,
		"hasHistory":  gamesPlayed > 0,
	})
}
