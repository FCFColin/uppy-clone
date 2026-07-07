package handler

import (
	"context"
	"net/http"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

// DefaultAuthService is the production AuthService implementation.
// This thin wrapper layer exists solely to prevent circular imports between the handler
// and auth packages. The handler package defines AuthService (interface) and consumes it;
// the auth package provides the underlying implementations. DefaultAuthService bridges the
// two by composing auth primitives without forcing handler to import auth directly.
type DefaultAuthService struct {
	jwtMgr     *auth.JWTManager
	refreshMgr *auth.RefreshTokenManager
	tokens     auth.TokenStore
	users      UserStore
	magicLink  *auth.MagicLinkService
	resendKey  string
	emailFrom  string
	timeouts   config.TimeoutConfig
}

func NewDefaultAuthService(
	jwtMgr *auth.JWTManager,
	refreshMgr *auth.RefreshTokenManager,
	tokens auth.TokenStore,
	users UserStore,
	resendKey string,
	emailFrom string,
	timeouts config.TimeoutConfig,
) *DefaultAuthService {
	return &DefaultAuthService{
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

func (a *DefaultAuthService) RequestMagicLink(ctx context.Context, email string, r *http.Request) error {
	return a.magicLink.RequestMagicLink(a.tokens, a.users, a.resendKey, a.emailFrom, email, r, a.timeouts)
}

func (a *DefaultAuthService) VerifyMagicLink(ctx context.Context, token string, r *http.Request) (userID, accessToken, refreshToken string, err error) {
	cookie, resp, authErr := auth.VerifyMagicLink(a.tokens, a.users, a.jwtMgr, a.refreshMgr, token, r)
	if authErr != nil {
		return "", "", "", authErr
	}
	if cookie != nil {
		accessToken = cookie.Value
	}
	return resp.UserID, accessToken, resp.RefreshToken, nil
}

func (a *DefaultAuthService) QuickPlay(ctx context.Context, nickname string, r *http.Request) (userID, accessToken, refreshToken string, err error) {
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

func (a *DefaultAuthService) RefreshSession(ctx context.Context, refreshToken string, r *http.Request) (accessToken, newRefreshToken string, cookieMaxAge int, err error) {
	result, authErr := auth.RefreshSession(ctx, a.refreshMgr, a.jwtMgr, a.users, refreshToken)
	if authErr != nil {
		return "", "", 0, authErr
	}
	return result.AccessToken, result.RefreshToken, config.CookieMaxAge, nil
}

func (a *DefaultAuthService) ExportUserData(ctx context.Context, userID string) (*domain.User, []domain.GameResult, error) {
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

func (a *DefaultAuthService) DeleteUserData(ctx context.Context, userID string, r *http.Request) error {
	return auth.DeleteUserData(ctx, a.jwtMgr, a.refreshMgr, a.tokens, a.users, userID, r)
}

func (a *DefaultAuthService) RevokeRefreshToken(ctx context.Context, token string) error {
	return a.refreshMgr.Revoke(ctx, token)
}

func (a *DefaultAuthService) RevokeAllTokens(ctx context.Context, r *http.Request) error {
	auth.RevokeAllTokens(ctx, a.jwtMgr, a.refreshMgr, a.tokens, r)
	return nil
}

func (a *DefaultAuthService) AuthenticatedUserFromRequest(r *http.Request) (userID, nickname string, ok bool) {
	return auth.AuthenticatedUserFromRequestWithRevocation(r, a.jwtMgr, a.tokens)
}

func (a *DefaultAuthService) GetJTI(r *http.Request) string {
	return auth.GetJTI(r)
}

func (a *DefaultAuthService) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	return a.tokens.IsJWTRevoked(ctx, jti)
}
