package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/store"
)

const adminRole = "admin"

const maskedKey = "••••••••"

// maxFailedLoginAttempts is the threshold at which an IP is locked out.
const maxFailedLoginAttempts = 5

// loginLockDuration is how long an IP remains locked out after too many failures.
const loginLockDuration = 15 * time.Minute

// AdminHandler handles admin endpoints.
type AdminHandler struct {
	db     *store.PostgresStore
	jwtMgr *auth.JWTManager
	redis  *store.RedisStore
}

// NewAdminHandler creates a new AdminHandler.
// redis is used for failed-login lockout tracking; may be nil in tests.
func NewAdminHandler(db *store.PostgresStore, jwtMgr *auth.JWTManager, redis *store.RedisStore) *AdminHandler {
	return &AdminHandler{
		db:     db,
		jwtMgr: jwtMgr,
		redis:  redis,
	}
}

// adminClaims extends jwt.RegisteredClaims with role.
type adminClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

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

	// Check login lockout before any password verification.
	// 企业为何需要：暴力破解防御必须在密码验证前拦截，否则锁定机制形同虚设。
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

	// Compare password using bcrypt
	if !compareAdminPassword(body.Password, storedPassword) {
		h.handleFailedLogin(ctx, clientIP)
		apierror.Unauthorized("Wrong password").Write(w)
		return
	}

	// On success, reset the failed login counter.
	if h.redis != nil {
		if err := h.redis.ResetFailedLogin(ctx, clientIP); err != nil {
			slog.Warn("failed to reset failed login", "ip", clientIP, "error", err)
		}
	}

	// Audit: admin login success
	audit.Log(ctx, audit.AuditEntry{
		Action:    "admin.login.success",
		ActorID:   adminRole,
		ActorIP:   clientIP,
		Resource:  "admin/session",
		RequestID: middleware.GetRequestID(ctx),
	})

	// Sign admin JWT (30min expiry; admin clients should refresh via /api/v1/auth/refresh)
	token, err := h.signAdminToken()
	if err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	// Set admin token as cookie
	secure := auth.IsSecure(r)
	cookie := auth.BuildAuthCookie("admin_token", token, int(config.AdminTokenTTL.Seconds()), secure)
	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
}

// getStoredAdminPassword retrieves the admin password from the app_config DB row.
// Writes the appropriate error response and returns false on failure.
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

// handleFailedLogin increments the failed login counter, sets a lock if the
// threshold is reached, and logs the failure to the audit trail.
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

// GetConfig handles GET /api/admin/config (requires admin JWT)
func (h *AdminHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config, err := h.db.GetConfig(ctx, "global")
	if err != nil || config == nil {
		apierror.NotFound("Config not found").Write(w)
		return
	}

	// Parse and mask sensitive fields
	var storedConfig struct {
		EmailEnabled  bool   `json:"email_enabled"`
		ResendApiKey  string `json:"resend_api_key"`
		EmailFrom     string `json:"email_from"`
		AdminPassword string `json:"admin_password"`
	}
	if err := json.Unmarshal([]byte(config.Config), &storedConfig); err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	// Decrypt API key if present
	resendApiKey := storedConfig.ResendApiKey
	if resendApiKey != "" {
		decrypted, err := crypto.Decrypt(resendApiKey)
		if err != nil {
			// If decryption fails, the value may be plaintext (legacy data)
			resendApiKey = storedConfig.ResendApiKey
		} else {
			resendApiKey = decrypted
		}
	}

	// Mask API key
	maskedApiKey := ""
	if resendApiKey != "" {
		maskedApiKey = maskedKey
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"emailEnabled": storedConfig.EmailEnabled,
		"resendApiKey": maskedApiKey,
		"emailFrom":    storedConfig.EmailFrom,
	})
}

// configUpdates represents the optional fields for a config update request.
type configUpdates struct {
	EmailEnabled  *bool   `json:"emailEnabled"`
	ResendApiKey  *string `json:"resendApiKey"`
	EmailFrom     *string `json:"emailFrom"`
	AdminPassword *string `json:"adminPassword"`
	OldPassword   *string `json:"oldPassword"`
}

// UpdateConfig handles PATCH /api/v1/admin/config (requires admin JWT)
func (h *AdminHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	updates, err := h.parseConfigUpdates(r)
	if err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	ctx := r.Context()
	cfg, err := h.db.GetConfig(ctx, "global")
	if err != nil || cfg == nil {
		apierror.NotFound("Config not found").Write(w)
		return
	}

	var storedConfig map[string]interface{}
	if err := json.Unmarshal([]byte(cfg.Config), &storedConfig); err != nil {
		storedConfig = make(map[string]interface{})
	}

	beforeConfig := maskSensitiveFields(storedConfig)

	if !h.applyConfigUpdates(ctx, w, r, storedConfig, updates) {
		return
	}

	if err := h.saveConfig(ctx, cfg, storedConfig); err != nil {
		apierror.InternalError("Failed to save config").Write(w)
		return
	}

	h.auditConfigChange(ctx, r, beforeConfig, storedConfig)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Config updated"})
}

// parseConfigUpdates decodes the config update request body.
func (h *AdminHandler) parseConfigUpdates(r *http.Request) (*configUpdates, error) {
	var updates configUpdates
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		return nil, err
	}
	return &updates, nil
}

// applyConfigUpdates applies the requested updates to storedConfig.
// Returns false if the request was aborted (error response already written).
// 企业为何需要：不验证旧密码意味着任何能拿到 admin JWT 的攻击者（如 XSS 窃取 cookie）
// 即可修改密码长期接管账户。验证旧密码是密码修改流程的行业标准。
func (h *AdminHandler) applyConfigUpdates(ctx context.Context, w http.ResponseWriter, r *http.Request, storedConfig map[string]interface{}, updates *configUpdates) bool {
	if updates.EmailEnabled != nil {
		storedConfig["email_enabled"] = *updates.EmailEnabled
	}
	if updates.ResendApiKey != nil && *updates.ResendApiKey != maskedKey {
		encrypted, err := crypto.Encrypt(*updates.ResendApiKey)
		if err != nil {
			apierror.InternalError("Failed to encrypt API key").Write(w)
			return false
		}
		storedConfig["resend_api_key"] = encrypted
	}
	if updates.EmailFrom != nil {
		storedConfig["email_from"] = *updates.EmailFrom
	}
	if updates.AdminPassword != nil {
		if updates.OldPassword == nil {
			apierror.BadRequest("oldPassword required to change adminPassword").Write(w)
			return false
		}
		currentPwd, _ := storedConfig["admin_password"].(string)
		if !compareAdminPassword(*updates.OldPassword, currentPwd) {
			apierror.Unauthorized("wrong old password").Write(w)
			return false
		}
		hashed, err := hashAdminPassword(*updates.AdminPassword)
		if err != nil {
			apierror.InternalError("Failed to hash password").Write(w)
			return false
		}
		storedConfig["admin_password"] = hashed
		AuditPasswordChange(ctx, middleware.ExtractClientIP(r))
		// Revoke the current admin token so the admin must re-login with the
		// new password. 企业为何需要：改密后旧 token 仍有效等于改密无效——攻击者窃取的
		// 旧 cookie 仍可登录。撤销当前 jti 强制重新认证，是改密流程的安全闭环。
		if jti := auth.GetJTI(r); jti != "" && h.redis != nil {
			if err := h.redis.RevokeJWT(ctx, jti, config.AdminTokenTTL); err != nil {
				slog.Warn("failed to revoke admin jwt after password change", "jti", jti, "error", err)
			}
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

// auditConfigChange logs the config change with before/after states (sensitive fields masked).
func (h *AdminHandler) auditConfigChange(ctx context.Context, r *http.Request, beforeConfig map[string]interface{}, storedConfig map[string]interface{}) {
	afterConfig := maskSensitiveFields(storedConfig)
	audit.Log(ctx, audit.AuditEntry{
		Action:    "admin.config.update",
		ActorID:   adminRole,
		ActorIP:   middleware.ExtractClientIP(r),
		Resource:  "admin/config/global",
		Before:    beforeConfig,
		After:     afterConfig,
		RequestID: middleware.GetRequestID(ctx),
	})
}

// maskSensitiveFields returns a copy of the config map with sensitive fields masked.
func maskSensitiveFields(cfg map[string]interface{}) map[string]interface{} {
	masked := make(map[string]interface{})
	for k, v := range cfg {
		if k == "admin_password" || k == "resend_api_key" {
			masked[k] = maskedKey
		} else {
			masked[k] = v
		}
	}
	return masked
}

// signAdminToken creates an admin JWT with 30-minute expiry.
// Admin clients should call /api/v1/auth/refresh before the 30-minute expiry
// to obtain a new access token via the existing refresh token mechanism.
// 企业为何需要：jti (JWT ID) 是 RFC 7519 标准字段，用于唯一标识每个 token，
// 是 token 撤销机制（登出/改密后立即失效）的前提。无 jti 的 token 无法被精确撤销。
func (h *AdminHandler) signAdminToken() (string, error) {
	now := time.Now()
	jti := uuid.NewString()
	claims := adminClaims{
		Role: adminRole,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   adminRole,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(config.AdminTokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtMgr.Secret())
}

// VerifyAdminToken checks if the request carries a valid admin JWT.
// When Redis is configured, also rejects tokens whose jti has been revoked.
func (h *AdminHandler) VerifyAdminToken(r *http.Request) bool {
	_, ok := h.VerifyAdminTokenClaims(r)
	return ok
}

// VerifyAdminTokenClaims parses and verifies the admin JWT from the
// admin_token cookie, checks the revocation list (when Redis is available),
// and returns the claims on success. The claims contain the jti needed by
// the auth middleware, Logout, and password-change revocation flows.
// 企业为何需要：admin token 必须支持撤销（登出/改密后立即失效）。复用用户 JWT 的
// Redis 撤销列表（RevokeJWT/IsJWTRevoked）避免重复造轮子。redis == nil 时跳过
// 撤销检查（测试场景），生产环境始终配置 Redis。
func (h *AdminHandler) VerifyAdminTokenClaims(r *http.Request) (*adminClaims, bool) {
	cookie, err := r.Cookie("admin_token")
	if err != nil {
		return nil, false
	}

	token, err := jwt.ParseWithClaims(cookie.Value, &adminClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return h.jwtMgr.Secret(), nil
	})
	if err != nil {
		return nil, false
	}

	claims, ok := token.Claims.(*adminClaims)
	if !ok || !token.Valid || claims.Role != adminRole {
		return nil, false
	}

	// Check revocation list when Redis is available.
	if h.redis != nil && claims.ID != "" {
		revoked, revErr := h.redis.IsJWTRevoked(r.Context(), claims.ID)
		if revErr != nil {
			slog.Warn("admin jwt revocation check failed", "jti", claims.ID, "error", revErr)
			return nil, false
		}
		if revoked {
			slog.Info("revoked admin jwt used", "jti", claims.ID)
			return nil, false
		}
	}

	return claims, true
}

// Logout handles POST /api/v1/admin/logout.
// Revokes the current admin token's jti and clears the admin_token cookie.
// 企业为何需要：登出必须使 token 立即失效，否则被盗 cookie 在过期前持续有效。
// 复用用户 JWT 的 Redis 撤销列表，TTL 设为 AdminTokenTTL 使撤销条目随 token 自然过期清理。
func (h *AdminHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jti := auth.GetJTI(r)
	if jti != "" && h.redis != nil {
		if err := h.redis.RevokeJWT(ctx, jti, config.AdminTokenTTL); err != nil {
			slog.Warn("failed to revoke admin jwt on logout", "jti", jti, "error", err)
		}
	}

	// Clear admin_token cookie
	secure := auth.IsSecure(r)
	http.SetCookie(w, auth.BuildAuthCookie("admin_token", "", -1, secure))

	audit.Log(ctx, audit.AuditEntry{
		Action:    "admin.logout",
		ActorID:   adminRole,
		ActorIP:   middleware.ExtractClientIP(r),
		Resource:  "admin/session",
		RequestID: middleware.GetRequestID(ctx),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}
