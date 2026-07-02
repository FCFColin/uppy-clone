package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
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
	_ = json.NewEncoder(w).Encode(body)
}

// CheckAuth handles GET /api/v1/auth/check
func (h *AuthHandler) CheckAuth(w http.ResponseWriter, r *http.Request) {
	var rev auth.JWTRevocationChecker
	if h.redis != nil {
		rev = h.redis
	}
	userId, nickname, ok := auth.AuthenticatedUserFromRequestWithRevocation(r, h.jwtMgr, rev)
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
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	refreshToken := auth.RefreshTokenFromRequest(r)
	if refreshToken == "" {
		refreshToken = body.RefreshToken
	}
	if refreshToken == "" {
		apierror.BadRequest("refresh token is required").Write(w)
		return
	}

	if h.redis == nil {
		slog.Warn("degraded: Redis not available, cannot refresh token")
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]interface{}{"refreshed": false},
			"Token refresh temporarily unavailable, please retry later")
		return
	}

	ctx := r.Context()
	if !RequireDB(h.db, w) || !RequireRedis(h.redis, w) {
		return
	}

	result, err := auth.RefreshSession(ctx, h.refreshMgr, h.jwtMgr, h.db, refreshToken)
	if err != nil {
		apierror.Unauthorized("Invalid or expired refresh token").Write(w)
		return
	}

	// Preserve the original cookie name ("session" for magic-link users, "quickplay" for quick-play).
	// Previously hardcoded "quickplay", causing magic-link users to get a "quickplay" cookie after
	// their first refresh (C6).
	cookieName := "session"
	if _, err := r.Cookie("quickplay"); err == nil {
		cookieName = "quickplay"
	}

	secure := auth.IsSecure(r)
	writeAuthCookies(w, r, auth.BuildAuthCookie(cookieName, result.AccessToken, config.CookieMaxAge, secure), result.RefreshToken)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"refreshed": true})
}
