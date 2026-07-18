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
	"github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/requestctx"
	"github.com/uppy-clone/backend/internal/store"
)

const adminRole = "admin"

const maskedKey = "••••••••"

const (
	adminPasswordKey     = "admin_password"
	resendAPIKey         = "resend_api_key" //nolint:gosec // false positive: JSON config field name, not a credential
	adminSessionResource = "admin/session"
	jsonMessage          = "message"
	jsonUserID           = "userId"
	jsonNickname         = "nickname"
	sessionCookie        = "session"
	quickplayCookie      = "quickplay"
	degradedKey          = "degraded"
	globalScope          = "global"
	codeKey              = "code"
	jwtRoleClaim         = "role"
	jwtSubClaim          = "sub"
	jwtIatClaim          = "iat"
	jwtExpClaim          = "exp"
)

// TokenSigner creates admin JWTs. Replaceable in tests.
type TokenSigner interface {
	SignToken() (token, jti string, err error)
}

// AdminHandler handles admin endpoints.
type AdminHandler struct {
	db          *store.ConfigRepository
	adminJwtMgr *auth.JWTManager
	redis       *store.RedisStore
	tokenSigner TokenSigner
}

// NewAdminHandler creates a new AdminHandler.
// redis is used for failed-login lockout tracking; may be nil in tests.
func NewAdminHandler(db *store.ConfigRepository, adminJwtMgr *auth.JWTManager, redis *store.RedisStore) *AdminHandler {
	return &AdminHandler{
		db:          db,
		adminJwtMgr: adminJwtMgr,
		redis:       redis,
	}
}

// adminClaims extends jwt.RegisteredClaims with role.
type adminClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// configUpdates represents the optional fields for a config update request.
type configUpdates struct {
	EmailEnabled  *bool   `json:"emailEnabled"`
	ResendApiKey  *string `json:"resendApiKey"`
	EmailFrom     *string `json:"emailFrom"`
	AdminPassword *string `json:"adminPassword"`
	OldPassword   *string `json:"oldPassword"`
}

// parseConfigUpdates decodes the config update request body.
func (h *AdminHandler) parseConfigUpdates(w http.ResponseWriter, r *http.Request) (*configUpdates, error) {
	var updates configUpdates
	if err := decodeJSONBody(w, r, &updates); err != nil {
		return nil, err
	}
	return &updates, nil
}

// auditConfigChange logs the config change with before/after states (sensitive fields masked).
func (h *AdminHandler) auditConfigChange(ctx context.Context, r *http.Request, beforeConfig map[string]interface{}, storedConfig map[string]interface{}) {
	afterConfig := maskSensitiveFields(storedConfig)
	audit.Log(ctx, audit.AuditEntry{
		Action:    "admin.config.update",
		ActorType: audit.ActorTypeAdmin,
		ActorID:   adminRole,
		ActorIP:   requestctx.ExtractClientIP(r),
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
		masked[k] = v
		if k == adminPasswordKey || k == resendAPIKey {
			masked[k] = maskedKey
		}
	}
	return masked
}

// signAdminToken creates an admin JWT with 30-minute expiry.
// Returns the signed token string and its jti for session tracking (H5).
func (h *AdminHandler) signAdminToken() (string, string, error) {
	if h.tokenSigner != nil {
		return h.tokenSigner.SignToken()
	}
	return h.signAdminTokenDefault()
}

func (h *AdminHandler) signAdminTokenDefault() (string, string, error) {
	now := time.Now()
	jti := uuid.NewString()
	claims := map[string]any{
		jwtRoleClaim: adminRole,
		"jti":        jti,
		jwtSubClaim:  adminRole,
		jwtIatClaim:  now.Unix(),
		jwtExpClaim:  now.Add(config.AdminTokenTTL).Unix(),
	}
	signed, err := h.adminJwtMgr.SignWithClaims(claims)
	if err != nil {
		return "", "", err
	}
	return signed, jti, nil
}

// revokeAllAdminSessions revokes all active admin JWTs by iterating the
// tracked jtis in Redis. Called on password change to force re-login (H5).
func (h *AdminHandler) revokeAllAdminSessions(ctx context.Context) {
	if h.redis == nil {
		return
	}
	jtis, err := h.redis.GetAllAdminJTIs(ctx)
	if err != nil {
		slog.Warn("failed to get admin jtis for revocation", "error", err)
		return
	}
	for _, jti := range jtis {
		if err := h.redis.RevokeJWT(ctx, jti, config.AdminTokenTTL); err != nil {
			slog.Warn("failed to revoke admin jti", "jti", jti, "error", err)
		}
		if err := h.redis.RemoveAdminJTI(ctx, jti); err != nil {
			slog.Warn("failed to remove admin jti from active set", "jti", jti, "error", err)
		}
	}
}

// VerifyAdminToken checks if the request carries a valid admin JWT.
func (h *AdminHandler) VerifyAdminToken(r *http.Request) bool {
	_, ok := h.VerifyAdminTokenClaims(r)
	return ok
}

// VerifyAdminTokenClaims parses and verifies the admin JWT from the admin_token cookie.
func (h *AdminHandler) VerifyAdminTokenClaims(r *http.Request) (*adminClaims, bool) {
	cookie, err := r.Cookie("admin_token")
	if err != nil {
		return nil, false
	}

	token, err := jwt.ParseWithClaims(cookie.Value, &adminClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return h.adminJwtMgr.PublicKey(), nil
	})
	if err != nil {
		return nil, false
	}

	claims, ok := token.Claims.(*adminClaims)
	if !ok || !token.Valid || claims.Role != adminRole {
		return nil, false
	}

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

// ─── Admin Auth: Password Retrieval & Logout ───────────────────────

// getStoredAdminPassword retrieves the admin password from the app_config DB row.
func (h *AdminHandler) getStoredAdminPassword(ctx context.Context, w http.ResponseWriter) (string, bool) {
	dbConfig, err := h.db.GetConfig(ctx, globalScope)
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
		Resource:  adminSessionResource,
		RequestID: middleware.GetRequestID(ctx),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{jsonMessage: "Logged out"})
}
