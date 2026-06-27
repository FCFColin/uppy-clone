package handler

import (
	"encoding/json"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/metrics"
)

// QuickPlay handles POST /api/v1/auth/quickplay
func (h *AuthHandler) QuickPlay(w http.ResponseWriter, r *http.Request) {
	rec, w := metrics.BeginAuth("quickplay", w)
	defer rec.End()

	if !RequireDB(h.db, w) {
		return
	}

	nickname := auth.ParseQuickPlayRequest(r)

	cookie, resp, err := auth.QuickPlay(h.db, h.jwtMgr, h.refreshMgr, nickname, r)
	if err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
