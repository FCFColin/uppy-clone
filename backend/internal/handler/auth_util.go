package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

var (
	ErrTooManyRequests = errors.New("too many requests")
	ErrInvalidEmail    = errors.New("invalid email")
)

func isSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func buildAuthCookie(name, value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

var refreshCookieName = "refresh"

func buildRefreshCookie(value string, secure bool) *http.Cookie {
	maxAge := int(config.RefreshTokenTTL.Seconds())
	if value == "" {
		maxAge = -1
	}
	return buildAuthCookie(refreshCookieName, value, maxAge, secure)
}

func refreshTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func getAuthenticatedUser(r *http.Request) (userID, nickname string, ok bool) {
	uid, ok1 := domain.ContextKeyUserID.Value(r.Context())
	nick, ok2 := domain.ContextKeyNickname.Value(r.Context())
	if !ok1 || !ok2 || uid == "" {
		return "", "", false
	}
	return uid, nick, true
}

func getJTI(r *http.Request) string {
	jti, ok := domain.ContextKeyJTI.Value(r.Context())
	if ok {
		return jti
	}
	return ""
}

func parseQuickPlayRequest(r *http.Request) (string, *apierror.ProblemDetails) {
	var body struct {
		Nickname string `json:"nickname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", apierror.BadRequest("invalid request body")
	}
	nickname := strings.TrimSpace(body.Nickname)
	if len(nickname) < 2 || len(nickname) > 20 {
		return "", apierror.New(http.StatusBadRequest, "Invalid nickname", "nickname must be 2-20 characters")
	}
	for _, r := range nickname {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return "", apierror.New(http.StatusBadRequest, "Invalid nickname", "nickname contains invalid characters")
		}
	}
	return nickname, nil
}
