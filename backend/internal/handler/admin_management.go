package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// ─── Admin Login ────────────────────────────────────────────────────

// maxFailedLoginAttempts is the threshold at which an IP is locked out.
const maxFailedLoginAttempts = 5

// loginLockDuration is how long an IP/account is locked after max failed attempts.
const loginLockDuration = config.AdminTokenTTL

// Login handles POST /api/admin/login
func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		domain.BadRequest("Invalid request body").Write(w)
		return
	}

	if len(body.Password) > config.BcryptMaxLen {
		domain.BadRequest("password too long (max 72 bytes)").Write(w)
		return
	}

	ctx := r.Context()
	clientIP := middleware.ExtractClientIP(r)
	// handler-025: account-dimension lockout key — the lockout uses BOTH the
	// client IP and the admin account identifier so that a distributed brute
	// force (many IPs, one account) triggers the account-dimension lock.
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
		domain.Unauthorized("Wrong password").Write(w)
		return
	}

	h.completeAdminLogin(w, r, ctx, clientIP, adminAccount)
}

func (h *AdminHandler) isLoginLocked(ctx context.Context, w http.ResponseWriter, clientIP, account string) bool {
	if h.redis == nil {
		slog.Error("admin login: redis not available, denying login")
		domain.New(http.StatusServiceUnavailable, "Service Unavailable",
			"Login temporarily unavailable, please retry later").Write(w)
		return true
	}
	locked, err := h.redis.IsLoginLocked(ctx, clientIP, account)
	if err != nil {
		slog.Warn("failed to check login lock", "ip", clientIP, "account", account, "error", err)
		domain.New(http.StatusServiceUnavailable, "Service Unavailable",
			"Login temporarily unavailable, please retry later").Write(w)
		return true
	}
	if !locked {
		return false
	}
	metrics.AdminLoginLockedTotal.Inc()
	domain.TooManyRequests("too many failed login attempts, try again later").Write(w)
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
		ActorType: audit.ActorTypeAdmin,
		ActorID:   adminRole,
		ActorIP:   clientIP,
		Resource:  "admin/session",
		RequestID: middleware.GetRequestID(ctx),
	})

	token, jti, err := h.signAdminToken()
	if err != nil {
		domain.InternalError("Internal server error").Write(w)
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
	_ = json.NewEncoder(w).Encode(map[string]string{jsonMessage: "Logged in"})
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
		Action:    "admin.login.failed",
		ActorType: audit.ActorTypeAdmin,
		ActorID:   adminRole,
		ActorIP:   clientIP,
		Resource:  "admin/session",
	})
}

// ─── Admin Password Hashing & Audit ─────────────────────────────────

// bcryptGenerate is replaceable in unit tests to simulate hashing failures.
var bcryptGenerate = bcrypt.GenerateFromPassword

// compareAdminPassword compares a plaintext password against a stored hash.
// Only bcrypt hashes are supported — legacy plaintext fallback has been removed
// to prevent timing attacks and enforce strong password storage.
// 企业为何需要：明文密码回退分支允许管理员密码以明文存储在数据库中，一旦数据库泄露即可直接使用。
// 强制 bcrypt 消除了这一攻击面。
func compareAdminPassword(plaintext, stored string) bool {
	if !isBcryptHash(stored) {
		return false // reject non-bcrypt hashes (legacy plaintext no longer supported)
	}
	err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(plaintext))
	return err == nil
}

// hashAdminPassword hashes a password using bcrypt.
func hashAdminPassword(password string) (string, error) {
	bytes, err := bcryptGenerate([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// isBcryptHash checks if a string looks like a bcrypt hash.
func isBcryptHash(s string) bool {
	return len(s) == 60 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}

// AuditPasswordChange records a password change in the audit log.
// Called from UpdateConfig when adminPassword is updated.
func AuditPasswordChange(ctx context.Context, actorIP string) {
	audit.Log(ctx, audit.AuditEntry{
		Action:    "admin.password.change",
		ActorType: audit.ActorTypeAdmin,
		ActorID:   adminRole,
		ActorIP:   actorIP,
		Resource:  "admin/config/global:admin_password",
		Before:    maskedKey,
		After:     maskedKey,
		RequestID: middleware.GetRequestID(ctx),
	})
}

// ─── Admin Config Get / Update ──────────────────────────────────────

// GetConfig handles GET /api/admin/config (requires admin JWT)
func (h *AdminHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	// handler-027: DB errors return 500 (InternalError); nil config returns
	// 404 (NotFound). Do NOT conflate DB failures with "not found".
	ctx := r.Context()
	cfg, err := h.db.GetConfig(ctx, globalScope)
	if err != nil {
		domain.InternalError("Failed to load config").Write(w)
		return
	}
	if cfg == nil {
		domain.NotFound("Config not found").Write(w)
		return
	}

	var storedConfig struct {
		EmailEnabled  bool   `json:"email_enabled"`
		ResendApiKey  string `json:"resend_api_key"`
		EmailFrom     string `json:"email_from"`
		AdminPassword string `json:"admin_password"`
	}
	if err := store.UnmarshalConfig(cfg.Config, &storedConfig); err != nil {
		domain.InternalError("Internal server error").Write(w)
		return
	}

	maskedApiKey := ""
	if storedConfig.ResendApiKey != "" {
		maskedApiKey = maskedKey // pragma: allowlist secret
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"emailEnabled": storedConfig.EmailEnabled,
		"resendApiKey": maskedApiKey,
		"emailFrom":    storedConfig.EmailFrom,
	})
}

// UpdateConfig handles PATCH /api/v1/admin/config (requires admin JWT)
func (h *AdminHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	updates, err := h.parseConfigUpdates(w, r)
	if err != nil {
		domain.BadRequest("Invalid request body").Write(w)
		return
	}

	// handler-027: DB errors return 500; nil config returns 404 (same as GetConfig).
	ctx := r.Context()
	cfg, err := h.db.GetConfig(ctx, globalScope)
	if err != nil {
		domain.InternalError("Failed to load config").Write(w)
		return
	}
	if cfg == nil {
		domain.NotFound("Config not found").Write(w)
		return
	}

	var storedConfig map[string]interface{}
	if err := store.UnmarshalConfig(cfg.Config, &storedConfig); err != nil {
		storedConfig = make(map[string]interface{})
	}

	beforeConfig := maskSensitiveFields(storedConfig)

	if !h.applyConfigUpdates(ctx, w, r, storedConfig, updates) {
		return
	}

	if err := h.saveConfig(ctx, cfg, storedConfig); err != nil {
		domain.InternalError("Failed to save config").Write(w)
		return
	}

	h.auditConfigChange(ctx, r, beforeConfig, storedConfig)

	writeJSON(w, http.StatusOK, map[string]string{jsonMessage: "Config updated"})
}

// applyConfigUpdates applies the requested updates to storedConfig.
func (h *AdminHandler) applyConfigUpdates(ctx context.Context, w http.ResponseWriter, r *http.Request, storedConfig map[string]interface{}, updates *configUpdates) bool {
	if updates.EmailEnabled != nil {
		storedConfig["email_enabled"] = *updates.EmailEnabled
	}
	if updates.ResendApiKey != nil && *updates.ResendApiKey != maskedKey { // pragma: allowlist secret
		encrypted, err := crypto.Encrypt(*updates.ResendApiKey)
		if err != nil {
			domain.InternalError("Failed to encrypt API key").Write(w)
			return false
		}
		storedConfig[resendAPIKey] = encrypted // pragma: allowlist secret
	}
	if updates.EmailFrom != nil {
		storedConfig["email_from"] = *updates.EmailFrom
	}
	if updates.AdminPassword != nil { // pragma: allowlist secret
		if updates.OldPassword == nil { // pragma: allowlist secret
			domain.BadRequest("oldPassword required to change adminPassword").Write(w)
			return false
		}
		currentPwd, _ := storedConfig["admin_password"].(string)
		if !compareAdminPassword(*updates.OldPassword, currentPwd) {
			domain.Unauthorized("wrong old password").Write(w)
			return false
		}
		hashed, err := hashAdminPassword(*updates.AdminPassword)
		if err != nil {
			domain.InternalError("Failed to hash password").Write(w)
			return false
		}
		storedConfig["admin_password"] = hashed // pragma: allowlist secret
		AuditPasswordChange(ctx, middleware.ExtractClientIP(r))

		// Revoke ALL admin sessions, not just the current one (H5).
		if h.redis != nil {
			h.revokeAllAdminSessions(ctx)
		}
	}
	return true
}

// saveConfig marshals storedConfig and persists it to the database.
func (h *AdminHandler) saveConfig(ctx context.Context, cfg *domain.AppConfig, storedConfig map[string]interface{}) error {
	configBytes, err := json.Marshal(storedConfig)
	if err != nil {
		return err
	}
	cfg.Config = string(configBytes)
	cfg.UpdatedAt = time.Now().UnixMilli()
	return h.db.SaveConfig(ctx, cfg)
}
