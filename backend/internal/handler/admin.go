package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/store"
)

const adminRole = "admin"

const maskedKey = "••••••••"

// AdminHandler handles admin endpoints.
type AdminHandler struct {
	db          *store.PostgresStore
	adminJwtMgr *auth.JWTManager
	redis       *store.RedisStore
}

// NewAdminHandler creates a new AdminHandler.
// redis is used for failed-login lockout tracking; may be nil in tests.
func NewAdminHandler(db *store.PostgresStore, adminJwtMgr *auth.JWTManager, redis *store.RedisStore) *AdminHandler {
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
func (h *AdminHandler) parseConfigUpdates(r *http.Request) (*configUpdates, error) {
	var updates configUpdates
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		return nil, err
	}
	return &updates, nil
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

// signAdminTokenFn is replaceable in unit tests to simulate signing failures.
var signAdminTokenFn = (*AdminHandler).signAdminTokenImpl

// signAdminToken creates an admin JWT with 30-minute expiry.
func (h *AdminHandler) signAdminToken() (string, error) {
	return signAdminTokenFn(h)
}

func (h *AdminHandler) signAdminTokenImpl() (string, error) {
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
	return token.SignedString(h.adminJwtMgr.Secret())
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
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return h.adminJwtMgr.Secret(), nil
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
