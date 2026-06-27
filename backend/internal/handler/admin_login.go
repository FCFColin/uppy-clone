package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/middleware"
)

// maxFailedLoginAttempts is the threshold at which an IP is locked out.
const maxFailedLoginAttempts = 5

// loginLockDuration is how long an IP remains locked out after too many failures.
const loginLockDuration = 15 * time.Minute

// Login handles POST /api/admin/login
func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	if len(body.Password) > config.BcryptMaxLen {
		apierror.BadRequest("password too long (max 72 bytes)").Write(w)
		return
	}

	ctx := r.Context()
	clientIP := middleware.ExtractClientIP(r)

	if h.redis != nil {
		locked, err := h.redis.IsLoginLocked(ctx, clientIP)
		if err != nil {
			slog.Warn("failed to check login lock", "ip", clientIP, "error", err)
		} else if locked {
			metrics.AdminLoginLockedTotal.Inc()
			apierror.TooManyRequests("too many failed login attempts, try again later").Write(w)
			return
		}
	}

	storedPassword, ok := h.getStoredAdminPassword(ctx, w)
	if !ok {
		return
	}

	if !compareAdminPassword(body.Password, storedPassword) {
		h.handleFailedLogin(ctx, clientIP)
		apierror.Unauthorized("Wrong password").Write(w)
		return
	}

	if h.redis != nil {
		if err := h.redis.ResetFailedLogin(ctx, clientIP); err != nil {
			slog.Warn("failed to reset failed login", "ip", clientIP, "error", err)
		}
	}

	audit.Log(ctx, audit.AuditEntry{
		Action:    "admin.login.success",
		ActorID:   adminRole,
		ActorIP:   clientIP,
		Resource:  "admin/session",
		RequestID: middleware.GetRequestID(ctx),
	})

	token, err := h.signAdminToken()
	if err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	secure := auth.IsSecure(r)
	cookie := auth.BuildAuthCookie("admin_token", token, int(config.AdminTokenTTL.Seconds()), secure)
	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
}

// handleFailedLogin increments the failed login counter and logs the failure.
func (h *AdminHandler) handleFailedLogin(ctx context.Context, clientIP string) {
	if h.redis != nil {
		count, ferr := h.redis.IncrementFailedLogin(ctx, clientIP)
		if ferr != nil {
			slog.Warn("failed to increment failed login", "ip", clientIP, "error", ferr)
		} else if count >= maxFailedLoginAttempts {
			if lerr := h.redis.SetLoginLock(ctx, clientIP, loginLockDuration); lerr != nil {
				slog.Warn("failed to set login lock", "ip", clientIP, "error", lerr)
			}
		}
	}
	audit.Log(ctx, audit.AuditEntry{
		Action:   "admin.login.failed",
		ActorID:  adminRole,
		ActorIP:  clientIP,
		Resource: "admin/session",
	})
}
