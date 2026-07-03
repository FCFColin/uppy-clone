package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
)

// ExportUserData handles GET /api/v1/user/data
func (h *AuthHandler) ExportUserData(w http.ResponseWriter, r *http.Request) {
	userId, _, ok := h.auth.AuthenticatedUserFromRequest(r)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	user, results, err := h.auth.ExportUserData(r.Context(), userId)
	if err != nil {
		apierror.NotFound("User not found").Write(w)
		return
	}

	exportData := map[string]interface{}{
		"user":         user,
		"game_results": results,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(exportData)
}

// DeleteUserData handles DELETE /api/v1/user/data
func (h *AuthHandler) DeleteUserData(w http.ResponseWriter, r *http.Request) {
	userId, _, ok := h.auth.AuthenticatedUserFromRequest(r)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	ctx := r.Context()
	if err := h.auth.DeleteUserData(ctx, userId, r); err != nil {
		slog.Error("failed to delete user data", "userId", userId, "error", err)
		apierror.InternalError("Failed to delete user data").Write(w)
		return
	}

	secure := isSecure(r)
	http.SetCookie(w, buildAuthCookie("quickplay", "", -1, secure))
	http.SetCookie(w, buildAuthCookie("session", "", -1, secure))
	http.SetCookie(w, buildRefreshCookie("", secure))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "User data deletion scheduled. All sessions have been revoked.",
	})
}
