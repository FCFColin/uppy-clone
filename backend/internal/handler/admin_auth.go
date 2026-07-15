package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/requestctx"
)

// getStoredAdminPassword retrieves the admin password from the app_config DB row.
func (h *AdminHandler) getStoredAdminPassword(ctx context.Context, w http.ResponseWriter) (string, bool) {
	dbConfig, err := h.db.GetConfig(ctx, "global")
	if err != nil || dbConfig == nil {
		apierror.Forbidden("Admin not configured").Write(w)
		return "", false
	}

	var storedConfig struct {
		AdminPassword string `json:"admin_password"`
	}
	if err := json.Unmarshal([]byte(dbConfig.Config), &storedConfig); err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return "", false
	}

	if storedConfig.AdminPassword == "" {
		apierror.Forbidden("Admin password not configured").Write(w)
		return "", false
	}
	return storedConfig.AdminPassword, true
}

// Logout handles POST /api/v1/admin/logout.
func (h *AdminHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jti := getJTI(r)
	if jti != "" && h.redis != nil {
		if err := h.redis.RevokeJWT(ctx, jti, config.AdminTokenTTL); err != nil {
			slog.Warn("failed to revoke admin jwt on logout", "jti", jti, "error", err)
		}
		if err := h.redis.RemoveAdminJTI(ctx, jti); err != nil {
			slog.Warn("failed to remove admin jti from active set", "jti", jti, "error", err)
		}
	}

	secure := isSecure(r)
	http.SetCookie(w, auth.BuildAuthCookie("admin_token", "", -1, secure))

	audit.Log(ctx, audit.AuditEntry{
		Action:    "admin.logout",
		ActorType: audit.ActorTypeAdmin,
		ActorID:   adminRole,
		ActorIP:   requestctx.ExtractClientIP(r),
		Resource:  "admin/session",
		RequestID: middleware.GetRequestID(ctx),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}
