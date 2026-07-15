// Package handler implements HTTP and WebSocket API endpoints.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

// ─── AuthHandler ─────────────────────────────────────────────────────

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	db         UserStore
	redis      TokenStore
	config     *Config
	jwtMgr     *auth.JWTManager
	refreshMgr *auth.RefreshTokenManager
	magicLink  *auth.MagicLinkService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db UserStore, redis TokenStore, jwtMgr *auth.JWTManager, refreshMgr *auth.RefreshTokenManager, config *Config) *AuthHandler {
	return &AuthHandler{
		db:         db,
		redis:      redis,
		jwtMgr:     jwtMgr,
		refreshMgr: refreshMgr,
		magicLink:  auth.NewMagicLinkService(),
		config:     config,
	}
}

// ─── Utilities & Cookie Helpers ──────────────────────────────────────

const defaultMaxBodyBytes = 1 << 20 // 1 MB

func decodeJSONBody(w http.ResponseWriter, r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxBodyBytes)
	return json.NewDecoder(r.Body).Decode(v)
}

func isSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

var refreshCookieName = "refresh"

func buildRefreshCookie(value string, secure bool) *http.Cookie {
	maxAge := int(config.RefreshTokenTTL.Seconds())
	if value == "" {
		maxAge = -1
	}
	return auth.BuildAuthCookie(refreshCookieName, value, maxAge, secure)
}

func getJTI(r *http.Request) string {
	jti, ok := domain.ContextKeyJTI.Value(r.Context())
	if ok {
		return jti
	}
	return ""
}

func clearAuthCookies(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, auth.BuildAuthCookie("quickplay", "", -1, secure))
	http.SetCookie(w, auth.BuildAuthCookie("session", "", -1, secure))
	http.SetCookie(w, buildRefreshCookie("", secure))
}

func writeAuthCookies(w http.ResponseWriter, r *http.Request, accessCookie *http.Cookie, refreshToken string) {
	http.SetCookie(w, accessCookie)
	if refreshToken != "" {
		http.SetCookie(w, buildRefreshCookie(refreshToken, isSecure(r)))
	}
}

func parseQuickPlayRequest(w http.ResponseWriter, r *http.Request) (string, *apierror.ProblemDetails) {
	var body struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		return "", apierror.BadRequest("invalid request body")
	}
	nickname := strings.TrimSpace(body.Nickname)
	if nickname == "" {
		return nickname, nil
	}
	nickRunes := utf8.RuneCountInString(nickname)
	if nickRunes < 2 || nickRunes > 20 {
		return "", apierror.New(http.StatusBadRequest, "Invalid nickname", "nickname must be 2-20 characters")
	}
	for _, r := range nickname {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return "", apierror.New(http.StatusBadRequest, "Invalid nickname", "nickname contains invalid characters")
		}
	}
	return nickname, nil
}

// ─── Session Check & Refresh ─────────────────────────────────────────

func writeAuthCheckResponse(w http.ResponseWriter, userId, nickname, email string, degraded bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	body := map[string]interface{}{
		"authenticated": true,
		"userId":        userId,
		"nickname":      nickname,
	}
	if degraded {
		body["degraded"] = true
	}
	if email != "" {
		body["email"] = email
	}
	_ = json.NewEncoder(w).Encode(body)
}

// CheckAuth handles GET /api/v1/auth/check
func (h *AuthHandler) CheckAuth(w http.ResponseWriter, r *http.Request) {
	userId, nickname, ok := auth.AuthenticatedUserFromRequestWithRevocation(r, h.jwtMgr, h.redis)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	if h.db == nil {
		writeAuthCheckResponse(w, userId, nickname, "", true)
		return
	}

	user, err := h.db.GetUserByID(r.Context(), userId)
	if err != nil {
		slog.Warn("degraded: auth check without DB enrichment", "error", err)
		writeAuthCheckResponse(w, userId, nickname, "", true)
		return
	}

	writeAuthCheckResponse(w, user.ID, user.Nickname, user.Email, false)
}

// RefreshToken handles POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil && err != io.EOF {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	refreshToken := auth.RefreshTokenFromRequest(r)
	if refreshToken == "" {
		refreshToken = body.RefreshToken
	}
	if refreshToken == "" {
		apierror.BadRequest("refresh token is required").Write(w)
		return
	}

	if h.redis == nil {
		slog.Warn("degraded: Redis not available, cannot refresh token")
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]interface{}{"refreshed": false},
			"Token refresh temporarily unavailable, please retry later")
		return
	}

	ctx := r.Context()
	if !RequireDB(h.db, w) || !RequireRedis(h.redis, w) {
		return
	}

	result, err := auth.RefreshSession(ctx, h.refreshMgr, h.jwtMgr, h.db, refreshToken)
	if err != nil {
		apierror.Unauthorized("Invalid or expired refresh token").Write(w)
		return
	}

	cookieName := "session"
	if _, err := r.Cookie("quickplay"); err == nil {
		cookieName = "quickplay"
	}

	secure := isSecure(r)
	writeAuthCookies(w, r, auth.BuildAuthCookie(cookieName, result.AccessToken, config.CookieMaxAge, secure), result.RefreshToken)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"refreshed": true})
}

// ─── QuickPlay ───────────────────────────────────────────────────────

// QuickPlay handles POST /api/v1/auth/quickplay
func (h *AuthHandler) QuickPlay(w http.ResponseWriter, r *http.Request) {
	nickname, apiErr := parseQuickPlayRequest(w, r)
	if apiErr != nil {
		apiErr.Write(w)
		return
	}

	if h.db == nil || h.jwtMgr == nil || h.refreshMgr == nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	cookie, resp, err := auth.QuickPlay(h.db, h.jwtMgr, h.refreshMgr, h.redis, nickname, r)
	if err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	accessToken := ""
	if cookie != nil {
		accessToken = cookie.Value
	}
	secure := isSecure(r)
	writeAuthCookies(w, r, auth.BuildAuthCookie("quickplay", accessToken, config.CookieMaxAge, secure), resp.RefreshToken)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"userId": resp.UserID})
}

// ─── Logout ──────────────────────────────────────────────────────────

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil && err != io.EOF {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	ctx := r.Context()

	refreshToken := auth.RefreshTokenFromRequest(r)
	if refreshToken == "" {
		refreshToken = body.RefreshToken
	}
	if refreshToken != "" && h.refreshMgr != nil {
		if err := h.refreshMgr.Revoke(ctx, refreshToken); err != nil {
			slog.Error("failed to revoke refresh token", "error", err)
		}
	}

	if err := auth.RevokeAllTokens(ctx, h.jwtMgr, h.refreshMgr, h.redis, r); err != nil {
		slog.Error("failed to revoke all tokens", "error", err)
	}

	clearAuthCookies(w, isSecure(r))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}

// ─── Magic Link ──────────────────────────────────────────────────────

// RequestMagicLink handles POST /api/v1/auth/request
func (h *AuthHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	if body.Email == "" {
		apierror.BadRequest("Email is required").Write(w)
		return
	}

	err := h.magicLink.RequestMagicLink(h.redis, body.Email, r)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrTooManyRequests):
			apierror.TooManyRequests(err.Error()).Write(w)
		case errors.Is(err, auth.ErrInvalidEmail):
			apierror.UnprocessableEntity(err.Error()).Write(w)
		default:
			slog.Error("magic link request failed", "error", err)
			apierror.InternalError("Internal server error").Write(w)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Magic link sent"})
}

// VerifyMagicLink handles GET /api/v1/auth/verify?token=...
func (h *AuthHandler) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	h.verifyMagicLinkToken(w, r, token)
}

// VerifyMagicLinkPost handles POST /api/v1/auth/verify with JSON body.
func (h *AuthHandler) VerifyMagicLinkPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}
	h.verifyMagicLinkToken(w, r, body.Token)
}

func (h *AuthHandler) verifyMagicLinkToken(w http.ResponseWriter, r *http.Request, token string) {
	if token == "" {
		apierror.BadRequest("Token is required").Write(w)
		return
	}

	if len(token) != config.MagicLinkTokenLen {
		apierror.BadRequest("invalid token").Write(w)
		return
	}

	if h.redis == nil {
		slog.Warn("degraded: Redis not available, cannot verify magic link")
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]interface{}{"verified": false},
			"Authentication service temporarily unavailable, please retry later")
		return
	}

	cookie, resp, err := auth.VerifyMagicLink(h.redis, h.db, h.jwtMgr, h.refreshMgr, token, r)
	if err != nil {
		slog.Error("magic link verification failed", "error", err)
		apierror.Unauthorized("Invalid or expired token").Write(w)
		return
	}

	accessToken := ""
	if cookie != nil {
		accessToken = cookie.Value
	}
	secure := isSecure(r)
	writeAuthCookies(w, r, auth.BuildAuthCookie("session", accessToken, config.CookieMaxAge, secure), resp.RefreshToken)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"userId": resp.UserID})
}

// ─── GDPR Data Export & Delete ───────────────────────────────────────

// ExportUserData handles GET /api/v1/user/data
func (h *AuthHandler) ExportUserData(w http.ResponseWriter, r *http.Request) {
	userId, _, ok := auth.AuthenticatedUserFromRequestWithRevocation(r, h.jwtMgr, h.redis)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	if h.db == nil {
		apierror.InternalError("Failed to export user data").Write(w)
		return
	}

	user, err := h.db.GetUserByID(r.Context(), userId)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			apierror.NotFound("User not found").Write(w)
		} else {
			apierror.InternalError("Failed to export user data").Write(w)
		}
		return
	}

	var results []domain.GameResult
	if user != nil {
		results, err = h.db.GetGameResultsByUserID(r.Context(), userId)
		if err != nil {
			results = nil
		}
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
	userId, _, ok := auth.AuthenticatedUserFromRequestWithRevocation(r, h.jwtMgr, h.redis)
	if !ok || userId == "" {
		apierror.Unauthorized("").Write(w)
		return
	}

	ctx := r.Context()
	if err := auth.DeleteUserData(ctx, h.jwtMgr, h.refreshMgr, h.redis, h.db, userId, r); err != nil {
		slog.Error("failed to delete user data", "userId", userId, "error", err)
		apierror.InternalError("Failed to delete user data").Write(w)
		return
	}

	clearAuthCookies(w, isSecure(r))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "User data deletion scheduled. All sessions have been revoked.",
	})
}
