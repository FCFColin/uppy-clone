package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
)

// ExportUserData handles GET /api/v1/user/data
func (h *AuthHandler) ExportUserData(w http.ResponseWriter, r *http.Request) {
	userId, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}
	if !RequireDB(h.db, w) {
		return
	}

	exportData, err := auth.ExportUserData(r.Context(), h.db, userId)
	if err != nil {
		apierror.NotFound("User not found").Write(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(exportData)
}

// DeleteUserData handles DELETE /api/v1/user/data
func (h *AuthHandler) DeleteUserData(w http.ResponseWriter, r *http.Request) {
	userId, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	ctx := r.Context()
	if err := auth.DeleteUserData(ctx, h.jwtMgr, h.refreshMgr, h.redis, h.db, userId, r); err != nil {
		slog.Error("failed to delete user data", "userId", userId, "error", err)
	}

	http.SetCookie(w, auth.BuildAuthCookie("quickplay", "", -1, true))
	http.SetCookie(w, auth.BuildAuthCookie("session", "", -1, true))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "User data deletion scheduled. All sessions have been revoked.",
	})
}
