package handler

import (
	"encoding/json"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/config"
)

// QuickPlay handles POST /api/v1/auth/quickplay
func (h *AuthHandler) QuickPlay(w http.ResponseWriter, r *http.Request) {
	nickname, apiErr := parseQuickPlayRequest(r)
	if apiErr != nil {
		apiErr.Write(w)
		return
	}

	userID, accessToken, refreshToken, err := h.auth.QuickPlay(r.Context(), nickname, r)
	if err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	secure := isSecure(r)
	writeAuthCookies(w, r, buildAuthCookie("quickplay", accessToken, config.CookieMaxAge, secure), refreshToken)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"userId": userID})
}
