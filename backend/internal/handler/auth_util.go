package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

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
