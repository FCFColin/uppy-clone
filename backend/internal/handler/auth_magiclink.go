package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/metrics"
)

// RequestMagicLink handles POST /api/v1/auth/request
func (h *AuthHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	rec, w := metrics.BeginAuth("request", w)
	defer rec.End()
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
			slog.Error("magic link request failed", "error", err)
			apierror.InternalError("Internal server error").Write(w)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"message": "Magic link sent"})
}

// VerifyMagicLink handles GET /api/v1/auth/verify
func (h *AuthHandler) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	rec, w := metrics.BeginAuth("verify", w)
	defer rec.End()

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
