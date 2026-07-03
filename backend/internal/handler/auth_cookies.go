package handler

import (
	"net/http"
)

// writeAuthCookies sets the access cookie and optional refresh cookie on the response.
func writeAuthCookies(w http.ResponseWriter, r *http.Request, accessCookie *http.Cookie, refreshToken string) {
	http.SetCookie(w, accessCookie)
	if refreshToken != "" {
		http.SetCookie(w, buildRefreshCookie(refreshToken, isSecure(r)))
	}
}
