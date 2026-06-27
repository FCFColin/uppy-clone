package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/metrics"
)

func writeAuthCheckResponse(w http.ResponseWriter, userId, nickname, email string, degraded bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	body := map[string]interface{}{
		"authenticated": true,
		"userId":        userId,
		"nickname":      nickname,
	}
	if degraded {
		body["degraded"] = true
	}
	if email != "" {
		body["email"] = email
	}
	json.NewEncoder(w).Encode(body)
}

// CheckAuth handles GET /api/v1/auth/check
func (h *AuthHandler) CheckAuth(w http.ResponseWriter, r *http.Request) {
	rec, w := metrics.BeginAuth("check", w)
	defer rec.End()

	userId, nickname, ok := auth.AuthenticatedUserFromRequest(r, h.jwtMgr)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	if h.db == nil {
		writeAuthCheckResponse(w, userId, nickname, "", true)
		return
	}

	user, err := h.db.GetUserByID(r.Context(), userId)
	if err != nil {
		slog.Warn("degraded: auth check without DB enrichment", "error", err)
		writeAuthCheckResponse(w, userId, nickname, "", true)
		return
	}

	writeAuthCheckResponse(w, user.ID, user.Nickname, user.Email, false)
}

// RefreshToken handles POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	rec, w := metrics.BeginAuth("refresh", w)
	defer rec.End()

	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}
	if body.RefreshToken == "" {
		apierror.BadRequest("refresh_token is required").Write(w)
		return
	}

	if h.redis == nil {
		slog.Warn("degraded: Redis not available, cannot refresh token")
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]interface{}{
				"access_token":  "",
				"refresh_token": "",
			},
			"Token refresh temporarily unavailable, please retry later")
		return
	}

	ctx := r.Context()
	if !RequireDB(h.db, w) || !RequireRedis(h.redis, w) {
		return
	}

	result, err := auth.RefreshSession(ctx, h.refreshMgr, h.jwtMgr, h.db, body.RefreshToken)
	if err != nil {
		apierror.Unauthorized("Invalid or expired refresh token").Write(w)
		return
	}

	secure := auth.IsSecure(r)
	cookie := auth.BuildAuthCookie("quickplay", result.AccessToken, config.CookieMaxAge, secure)
	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
	})
}
