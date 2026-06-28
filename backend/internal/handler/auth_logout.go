package handler

import (
	"encoding/json"
	"net/http"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/metrics"
)

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	rec, w := metrics.BeginAuth("logout", w)
	defer rec.End()

	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	ctx := r.Context()

	refreshToken := auth.RefreshTokenFromRequest(r)
	if refreshToken == "" {
		refreshToken = body.RefreshToken
	}
	if refreshToken != "" {
		_ = h.refreshMgr.Revoke(ctx, refreshToken)
	}

	auth.RevokeAllTokens(ctx, h.jwtMgr, h.refreshMgr, h.redis, r)

	secure := auth.IsSecure(r)
	http.SetCookie(w, auth.BuildAuthCookie("quickplay", "", -1, secure))
	http.SetCookie(w, auth.BuildAuthCookie("session", "", -1, secure))
	http.SetCookie(w, auth.BuildRefreshCookie("", secure))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}
