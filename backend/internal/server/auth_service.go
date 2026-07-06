package server

import (
	"context"
	"net/http"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

type authServiceAdapter struct {
	jwtMgr     *auth.JWTManager
	refreshMgr *auth.RefreshTokenManager
	tokens     auth.TokenStore
	users      auth.UserDB
	magicLink  *auth.MagicLinkService
	resendKey  string
	emailFrom  string
	timeouts   config.TimeoutConfig
}

func newAuthServiceAdapter(
	jwtMgr *auth.JWTManager,
	refreshMgr *auth.RefreshTokenManager,
	tokens auth.TokenStore,
	users auth.UserDB,
	resendKey string,
	emailFrom string,
	timeouts config.TimeoutConfig,
) *authServiceAdapter {
	return &authServiceAdapter{
		jwtMgr:     jwtMgr,
		refreshMgr: refreshMgr,
		tokens:     tokens,
		users:      users,
		magicLink:  auth.NewMagicLinkService(),
		resendKey:  resendKey,
		emailFrom:  emailFrom,
		timeouts:   timeouts,
	}
}

func (a *authServiceAdapter) RequestMagicLink(ctx context.Context, email string, r *http.Request) error {
	return a.magicLink.RequestMagicLink(a.tokens, a.users, a.resendKey, a.emailFrom, email, r, a.timeouts)
}

func (a *authServiceAdapter) VerifyMagicLink(ctx context.Context, token string, r *http.Request) (userID, accessToken, refreshToken string, err error) {
	cookie, resp, authErr := auth.VerifyMagicLink(a.tokens, a.users, a.jwtMgr, a.refreshMgr, token, r)
	if authErr != nil {
		return "", "", "", authErr
	}
	if cookie != nil {
		accessToken = cookie.Value
	}
	return resp.UserID, accessToken, resp.RefreshToken, nil
}

func (a *authServiceAdapter) QuickPlay(ctx context.Context, nickname string, r *http.Request) (userID, accessToken, refreshToken string, err error) {
	cookie, resp, authErr := auth.QuickPlay(a.users, a.jwtMgr, a.refreshMgr, a.tokens, nickname, r)
	if authErr != nil {
		return "", "", "", authErr
	}
	accessToken = ""
	if cookie != nil {
		accessToken = cookie.Value
	}
	return resp.UserID, accessToken, resp.RefreshToken, nil
}

func (a *authServiceAdapter) RefreshSession(ctx context.Context, refreshToken string, r *http.Request) (accessToken, newRefreshToken string, cookieMaxAge int, err error) {
	result, authErr := auth.RefreshSession(ctx, a.refreshMgr, a.jwtMgr, a.users, refreshToken)
	if authErr != nil {
		return "", "", 0, authErr
	}
	return result.AccessToken, result.RefreshToken, config.CookieMaxAge, nil
}

func (a *authServiceAdapter) ExportUserData(ctx context.Context, userID string) (*domain.User, []domain.GameResult, error) {
	user, err := a.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, nil
	}
	results, err := a.users.GetGameResultsByUserID(ctx, userID)
	if err != nil {
		results = nil
	}
	return user, results, nil
}

func (a *authServiceAdapter) DeleteUserData(ctx context.Context, userID string, r *http.Request) error {
	return auth.DeleteUserData(ctx, a.jwtMgr, a.refreshMgr, a.tokens, a.users, userID, r)
}

func (a *authServiceAdapter) RevokeRefreshToken(ctx context.Context, token string) error {
	return a.refreshMgr.Revoke(ctx, token)
}

func (a *authServiceAdapter) RevokeAllTokens(ctx context.Context, r *http.Request) error {
	auth.RevokeAllTokens(ctx, a.jwtMgr, a.refreshMgr, a.tokens, r)
	return nil
}

func (a *authServiceAdapter) AuthenticatedUserFromRequest(r *http.Request) (userID, nickname string, ok bool) {
	return auth.AuthenticatedUserFromRequestWithRevocation(r, a.jwtMgr, a.tokens)
}

func (a *authServiceAdapter) GetJTI(r *http.Request) string {
	return auth.GetJTI(r)
}

func (a *authServiceAdapter) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	return a.tokens.IsJWTRevoked(ctx, jti)
}
