package handler

import (
	"encoding/json"
	"net/http"
)

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	ctx := r.Context()

	refreshToken := refreshTokenFromRequest(r)
	if refreshToken == "" {
		refreshToken = body.RefreshToken
	}
	if refreshToken != "" {
		_ = h.auth.RevokeRefreshToken(ctx, refreshToken)
	}

	h.auth.RevokeAllTokens(ctx, r)

	secure := isSecure(r)
	http.SetCookie(w, buildAuthCookie("quickplay", "", -1, secure))
	http.SetCookie(w, buildAuthCookie("session", "", -1, secure))
	http.SetCookie(w, buildRefreshCookie("", secure))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}
