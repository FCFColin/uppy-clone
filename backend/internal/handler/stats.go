package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// StatsHandler serves public leaderboard and optional user stats.
type StatsHandler struct {
	db *store.ResultRepository
}

// NewStatsHandler creates a StatsHandler backed by the given leaderboard store.
func NewStatsHandler(db *store.ResultRepository) *StatsHandler {
	return &StatsHandler{db: db}
}

// GetLeaderboard handles GET /api/v1/leaderboard?scope=global|weekly&limit=50
func (h *StatsHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		apierror.New(http.StatusServiceUnavailable, "Service Unavailable", "service unavailable").Write(w)
		return
	}

	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = globalScope
	}
	if scope != globalScope && scope != "weekly" {
		apierror.BadRequest("invalid scope").Write(w)
		return
	}

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	// handler-022: Cap limit to prevent excessively large DB queries.
	if limit > config.MaxPageSize {
		limit = config.MaxPageSize
	}

	entries, err := h.db.GetLeaderboard(r.Context(), scope, limit)
	if err != nil {
		// handler-023: Log the actual error for debugging.
		slog.Error("failed to load leaderboard", "error", err, "scope", scope)
		apierror.InternalError("failed to load leaderboard").Write(w)
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
		apierror.New(http.StatusServiceUnavailable, "Service Unavailable", "service unavailable").Write(w)
		return
	}

	userID, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userID == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	bestScore, gamesPlayed, err := h.db.GetUserBestScore(r.Context(), userID)
	if err != nil {
		// handler-023: Log the actual error for debugging.
		slog.Error("failed to load user stats", "error", err, "user_id", userID)
		apierror.InternalError("failed to load stats").Write(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"bestScore":   bestScore,
		"gamesPlayed": gamesPlayed,
		"hasHistory":  gamesPlayed > 0,
	})
}
