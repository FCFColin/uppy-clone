package handler

import (
	"encoding/json"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	ctx := r.Context()

	if body.RefreshToken != "" {
		_ = h.refreshMgr.Revoke(ctx, body.RefreshToken)
	}

	auth.RevokeAllTokens(ctx, h.jwtMgr, h.refreshMgr, h.redis, r)

	http.SetCookie(w, auth.BuildAuthCookie("quickplay", "", -1, true))
	http.SetCookie(w, auth.BuildAuthCookie("session", "", -1, true))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}
