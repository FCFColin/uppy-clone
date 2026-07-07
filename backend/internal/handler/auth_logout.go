package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
)

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil && err != io.EOF {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	ctx := r.Context()

	refreshToken := auth.RefreshTokenFromRequest(r)
	if refreshToken == "" {
		refreshToken = body.RefreshToken
	}
	if refreshToken != "" {
		_ = h.auth.RevokeRefreshToken(ctx, refreshToken)
	}

	h.auth.RevokeAllTokens(ctx, r)

	clearAuthCookies(w, isSecure(r))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}
