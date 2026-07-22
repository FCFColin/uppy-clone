package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// PlayerCounter returns live player/room counts (implemented by *game.Hub).
// Declared as an interface here to avoid importing the game package, which
// would create an import cycle (game depends on handler for other helpers).
type PlayerCounter interface {
	PlayerCount() int
	RoomCount() int
}

// StatsHandler serves public leaderboard and optional user stats.
type StatsHandler struct {
	db  *store.ResultRepository
	hub PlayerCounter
}

// NewStatsHandler creates a StatsHandler backed by the given leaderboard store
// and an optional live PlayerCounter (typically *game.Hub) for online counts.
func NewStatsHandler(db *store.ResultRepository, hub PlayerCounter) *StatsHandler {
	return &StatsHandler{db: db, hub: hub}
}

// GetLeaderboard handles GET /api/v1/leaderboard?scope=global|weekly&limit=50
func (h *StatsHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	if !RequireDB(h.db, w) {
		return
	}

	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = globalScope
	}
	if scope != globalScope && scope != "weekly" {
		domain.BadRequest("invalid scope").Write(w)
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
		domain.InternalError("failed to load leaderboard").Write(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"scope":   scope,
		"entries": entries,
	})
}

// GetUserStats handles GET /api/v1/user/stats (authenticated).
func (h *StatsHandler) GetUserStats(w http.ResponseWriter, r *http.Request) {
	if !RequireDB(h.db, w) {
		return
	}

	userID, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userID == "" {
		domain.Unauthorized("").Write(w)
		return
	}

	bestScore, gamesPlayed, err := h.db.GetUserBestScore(r.Context(), userID)
	if err != nil {
		// handler-023: Log the actual error for debugging.
		slog.Error("failed to load user stats", "error", err, "user_id", userID)
		domain.InternalError("failed to load stats").Write(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"bestScore":   bestScore,
		"gamesPlayed": gamesPlayed,
		"hasHistory":  gamesPlayed > 0,
	})
}

// GetPublicStats handles GET /api/v1/stats/public — returns live, non-authenticated stats.
func (h *StatsHandler) GetPublicStats(w http.ResponseWriter, r *http.Request) {
	if !RequireDB(h.db, w) {
		return
	}

	type publicStats struct {
		OnlinePlayers int `json:"onlinePlayers"`
		GamesToday    int `json:"gamesToday"`
		BestScore     int `json:"bestScore"`
		ActiveRooms   int `json:"activeRooms"`
	}

	var stats publicStats
	if h.hub != nil {
		stats.OnlinePlayers = h.hub.PlayerCount()
		stats.ActiveRooms = h.hub.RoomCount()
	}

	gamesToday, err := h.db.GetGamesTodayCount(r.Context())
	if err != nil {
		slog.Error("failed to load games today count", "error", err)
		domain.InternalError("failed to load stats").Write(w)
		return
	}
	stats.GamesToday = gamesToday

	best, err := h.db.GetBestScore(r.Context())
	if err != nil {
		slog.Error("failed to load best score", "error", err)
		domain.InternalError("failed to load stats").Write(w)
		return
	}
	stats.BestScore = best

	writeJSON(w, http.StatusOK, stats)
}
