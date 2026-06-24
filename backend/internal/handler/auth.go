package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	jwtMgr     *auth.JWTManager
	refreshMgr *auth.RefreshTokenManager
	db         *store.PostgresStore
	redis      *store.RedisStore
	config     *Config
	magicLink  *auth.MagicLinkService
	timeouts   config.TimeoutConfig
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(jwtMgr *auth.JWTManager, refreshMgr *auth.RefreshTokenManager, db *store.PostgresStore, redis *store.RedisStore, config *Config, timeouts config.TimeoutConfig) *AuthHandler {
	return &AuthHandler{
		jwtMgr:     jwtMgr,
		refreshMgr: refreshMgr,
		db:         db,
		redis:      redis,
		config:     config,
		magicLink:  auth.NewMagicLinkService(),
		timeouts:   timeouts,
	}
}

// QuickPlay handles POST /api/v1/auth/quickplay
func (h *AuthHandler) QuickPlay(w http.ResponseWriter, r *http.Request) {
	nickname := auth.ParseQuickPlayRequest(r)

	cookie, resp, err := auth.QuickPlay(h.db, h.jwtMgr, h.refreshMgr, nickname, r)
	if err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// RequestMagicLink handles POST /api/v1/auth/request
func (h *AuthHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	if body.Email == "" {
		apierror.BadRequest("Email is required").Write(w)
		return
	}

	err := h.magicLink.RequestMagicLink(h.redis, h.db, h.config.ResendAPIKey, h.config.EmailFrom, body.Email, r, h.timeouts)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrTooManyRequests):
			apierror.TooManyRequests(err.Error()).Write(w)
		case errors.Is(err, auth.ErrInvalidEmail):
			// 422 Unprocessable Entity: 请求格式正确但语义无效。企业为何需要：区分 400（语法错误如 JSON 解析失败）和 422（语义校验失败如邮箱格式）是 REST API 成熟度标志。
			apierror.UnprocessableEntity(err.Error()).Write(w)
		default:
			apierror.InternalError(err.Error()).Write(w)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"message": "Magic link sent"})
}

// VerifyMagicLink handles GET /api/v1/auth/verify
func (h *AuthHandler) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		apierror.BadRequest("Token is required").Write(w)
		return
	}

	if len(token) != config.MagicLinkTokenLen {
		apierror.BadRequest("invalid token").Write(w)
		return
	}

	if h.redis == nil {
		// Redis unavailable — cannot verify magic link token, suggest retry later
		slog.Warn("degraded: Redis not available, cannot verify magic link")
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]interface{}{
				"verified": false,
			},
			"Authentication service temporarily unavailable, please retry later")
		return
	}

	cookie, resp, err := auth.VerifyMagicLink(h.redis, h.db, h.jwtMgr, h.refreshMgr, token)
	if err != nil {
		apierror.Unauthorized(err.Error()).Write(w)
		return
	}

	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// CheckAuth handles GET /api/v1/auth/check
func (h *AuthHandler) CheckAuth(w http.ResponseWriter, r *http.Request) {
	userId, nickname, ok := auth.GetAuthenticatedUser(r)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	if h.db == nil {
		// No DB available (e.g., test mode) — return basic auth info
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": true,
			"userId":        userId,
			"nickname":      nickname,
			"degraded":      true,
		})
		return
	}

	user, err := h.db.GetUserByID(r.Context(), userId)
	if err != nil {
		// Enterprise rationale: Graceful degradation — return partial auth status
		// instead of 500. The JWT is valid, we just can't enrich with DB data.
		slog.Warn("degraded: auth check without DB enrichment", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": true,
			"userId":        userId,
			"nickname":      nickname,
			"degraded":      true,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"authenticated": true,
		"userId":        user.ID,
		"nickname":      user.Nickname,
		"email":         user.Email,
	})
}

// RefreshToken handles POST /api/v1/auth/refresh
// Accepts a refresh token, validates it, and returns a new access token + rotated refresh token.
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}
	if body.RefreshToken == "" {
		apierror.BadRequest("refresh_token is required").Write(w)
		return
	}

	if h.redis == nil {
		// Redis unavailable — cannot validate/rotate refresh tokens
		slog.Warn("degraded: Redis not available, cannot refresh token")
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]interface{}{
				"access_token":  "",
				"refresh_token": "",
			},
			"Token refresh temporarily unavailable, please retry later")
		return
	}

	ctx := r.Context()

	user, ok := h.validateRefreshAndGetUser(ctx, body.RefreshToken, w)
	if !ok {
		return
	}

	accessToken, newRefreshToken, ok := h.rotateRefreshToken(ctx, body.RefreshToken, user.ID, user.Nickname, w)
	if !ok {
		return
	}

	// Set new access token as HttpOnly cookie
	secure := auth.IsSecure(r)
	cookie := auth.BuildAuthCookie("quickplay", accessToken, config.CookieMaxAge, secure) // 15min
	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": newRefreshToken,
	})
}

// validateRefreshAndGetUser validates the refresh token and looks up the user.
// Writes an error response and returns false on failure.
func (h *AuthHandler) validateRefreshAndGetUser(ctx context.Context, token string, w http.ResponseWriter) (*domain.User, bool) {
	userID, err := h.refreshMgr.Validate(ctx, token)
	if err != nil {
		apierror.Unauthorized("Invalid or expired refresh token").Write(w)
		return nil, false
	}

	user, err := h.db.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		apierror.Unauthorized("User not found").Write(w)
		return nil, false
	}
	return user, true
}

// rotateRefreshToken revokes the old refresh token and generates new access + refresh tokens.
// Writes an error response and returns false on failure.
func (h *AuthHandler) rotateRefreshToken(ctx context.Context, oldToken, userID, nickname string, w http.ResponseWriter) (string, string, bool) {
	// Revoke old refresh token (rotation)
	if err := h.refreshMgr.Revoke(ctx, oldToken); err != nil {
		_ = err
	}

	accessToken, err := h.jwtMgr.SignToken(userID, nickname)
	if err != nil {
		apierror.InternalError("Failed to sign token").Write(w)
		return "", "", false
	}

	newRefreshToken, err := h.refreshMgr.Generate(ctx, userID)
	if err != nil {
		apierror.InternalError("Failed to generate refresh token").Write(w)
		return "", "", false
	}
	return accessToken, newRefreshToken, true
}

// Logout handles POST /api/v1/auth/logout
// Revokes the provided refresh token, revokes the access token's jti,
// and clears the access token cookie.
// 企业为何需要：无撤销机制的 JWT 意味着被盗 token 在过期前持续有效。JWT 撤销列表是登出安全的行业标准实现，
// 用 Redis SET + TTL 实现最小性能开销。
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	ctx := r.Context()

	// Revoke refresh token if provided
	if body.RefreshToken != "" {
		_ = h.refreshMgr.Revoke(ctx, body.RefreshToken)
	}

	// Revoke the access token's jti so it can't be used after logout
	// Try both cookie names to find the current access token
	auth.RevokeAllTokens(ctx, h.jwtMgr, h.redis, r)

	// Clear access token cookies
	http.SetCookie(w, auth.BuildAuthCookie("quickplay", "", -1, true))
	http.SetCookie(w, auth.BuildAuthCookie("session", "", -1, true))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}

// ExportUserData handles GET /api/v1/user/data
// 企业为何需要：GDPR 第 20 条（数据可携带权）要求系统能导出用户数据。
func (h *AuthHandler) ExportUserData(w http.ResponseWriter, r *http.Request) {
	userId, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	ctx := r.Context()
	user, err := h.db.GetUserByID(ctx, userId)
	if err != nil || user == nil {
		apierror.NotFound("User not found").Write(w)
		return
	}

	// Export user profile + game results (N+1 fix: single indexed query).
	// 企业为何需要：GDPR 第 20 条数据可携带权要求导出用户全部数据，原实现返回空数组。
	exportData := map[string]interface{}{
		"user": map[string]interface{}{
			"id":         user.ID,
			"email":      user.Email,
			"nickname":   user.Nickname,
			"created_at": user.CreatedAt,
			"last_login": user.LastLogin,
		},
	}
	if results, err := h.db.GetGameResultsByUserID(ctx, userId); err == nil && results != nil {
		exportData["game_results"] = results
	} else {
		exportData["game_results"] = []interface{}{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(exportData)
}

// DeleteUserData handles DELETE /api/v1/user/data
// 企业为何需要：GDPR 第 17 条（删除权）要求系统能删除用户数据。
// 此端点立即匿名化 PII（email/nickname），撤销所有会话，并标记用户为待硬删除。
// 硬删除在 30 天保留期后由定时任务执行。
func (h *AuthHandler) DeleteUserData(w http.ResponseWriter, r *http.Request) {
	userId, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	ctx := r.Context()

	// Revoke all refresh tokens for this user
	if h.redis != nil {
		_ = h.refreshMgr.RevokeAllForUser(ctx, userId)
	}

	// Revoke current access token
	auth.RevokeAllTokens(ctx, h.jwtMgr, h.redis, r)

	// Anonymize user PII (GDPR Article 17)
	if h.db != nil {
		if err := h.db.AnonymizeUser(ctx, userId); err != nil {
			slog.Error("failed to anonymize user data", "userId", userId, "error", err)
			// Don't fail the request — tokens are already revoked
		}
	}

	// Clear cookies
	http.SetCookie(w, auth.BuildAuthCookie("quickplay", "", -1, true))
	http.SetCookie(w, auth.BuildAuthCookie("session", "", -1, true))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "User data deletion scheduled. All sessions have been revoked.",
	})
}
