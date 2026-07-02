package handler

import (
	"encoding/json"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
)

// QuickPlay handles POST /api/v1/auth/quickplay
func (h *AuthHandler) QuickPlay(w http.ResponseWriter, r *http.Request) {
	if !RequireDB(h.db, w) {
		return
	}

	nickname := auth.ParseQuickPlayRequest(r)

	var rev auth.JWTRevocationChecker
	if h.redis != nil {
		rev = h.redis
	}
	cookie, resp, err := auth.QuickPlay(h.db, h.jwtMgr, h.refreshMgr, rev, nickname, r)
	if err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	writeAuthCookies(w, r, cookie, resp.RefreshToken)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
