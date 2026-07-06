package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/audit"
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
	adminAccount := adminRole
	if h.isLoginLocked(ctx, w, clientIP, adminAccount) {
		return
	}

	storedPassword, ok := h.getStoredAdminPassword(ctx, w)
	if !ok {
		return
	}

	if !compareAdminPassword(body.Password, storedPassword) {
		h.handleFailedLogin(ctx, clientIP, adminAccount)
		apierror.Unauthorized("Wrong password").Write(w)
		return
	}

	h.completeAdminLogin(w, r, ctx, clientIP, adminAccount)
}

func (h *AdminHandler) isLoginLocked(ctx context.Context, w http.ResponseWriter, clientIP, account string) bool {
	if h.redis == nil {
		return false
	}
	locked, err := h.redis.IsLoginLocked(ctx, clientIP, account)
	if err != nil {
		slog.Warn("failed to check login lock", "ip", clientIP, "account", account, "error", err)
		apierror.New(http.StatusServiceUnavailable, "Service Unavailable",
			"Login temporarily unavailable, please retry later").Write(w)
		return true
	}
	if !locked {
		return false
	}
	metrics.AdminLoginLockedTotal.Inc()
	apierror.TooManyRequests("too many failed login attempts, try again later").Write(w)
	return true
}

func (h *AdminHandler) completeAdminLogin(w http.ResponseWriter, r *http.Request, ctx context.Context, clientIP, account string) {
	if h.redis != nil {
		if err := h.redis.ResetFailedLogin(ctx, clientIP, account); err != nil {
			slog.Warn("failed to reset failed login", "ip", clientIP, "account", account, "error", err)
		}
	}

	audit.Log(ctx, audit.AuditEntry{
		Action:    "admin.login.success",
		ActorID:   adminRole,
		ActorIP:   clientIP,
		Resource:  "admin/session",
		RequestID: middleware.GetRequestID(ctx),
	})

	token, jti, err := h.signAdminToken()
	if err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	if h.redis != nil && jti != "" {
		if err := h.redis.AddAdminJTI(ctx, jti, config.AdminTokenTTL); err != nil {
			slog.Warn("failed to track admin jti", "jti", jti, "error", err)
		}
	}

	secure := isSecure(r)
	cookie := auth.BuildAuthCookie("admin_token", token, int(config.AdminTokenTTL.Seconds()), secure)
	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Logged in"})
}

// handleFailedLogin increments the failed login counter and logs the failure.
func (h *AdminHandler) handleFailedLogin(ctx context.Context, clientIP, account string) {
	if h.redis != nil {
		ipCount, acctCount, ferr := h.redis.IncrementFailedLogin(ctx, clientIP, account)
		if ferr != nil {
			slog.Warn("failed to increment failed login", "ip", clientIP, "account", account, "error", ferr)
		} else if ipCount >= maxFailedLoginAttempts || acctCount >= maxFailedLoginAttempts {
			if lerr := h.redis.SetLoginLock(ctx, clientIP, account, loginLockDuration); lerr != nil {
				slog.Warn("failed to set login lock", "ip", clientIP, "account", account, "error", lerr)
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
