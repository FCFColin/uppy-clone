package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

type jwtParseFunc func(string, jwt.Claims, jwt.Keyfunc, ...jwt.ParserOption) (*jwt.Token, error)

// jwtParseWithClaimsFn is injectable for unit tests (e.g. invalid claims paths).
var jwtParseWithClaimsFn jwtParseFunc = jwt.ParseWithClaims

// VerifyToken validates a JWT and returns userId, nickname, jti, and role.
// If the token has no role claim (legacy tokens), role defaults to "user".
func (m *JWTManager) VerifyToken(tokenStr string) (userID, nickname, jti, role string, err error) {
	// auth-002: Verify Issuer and Audience to prevent token confusion across services.
	token, err := jwtParseWithClaimsFn(tokenStr, &customClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodES256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.publicKey, nil
	}, jwt.WithIssuer(config.JWTIssuer), jwt.WithAudience(config.JWTAudience))
	if err != nil {
		return "", "", "", "", fmt.Errorf("verify token: %w", err)
	}

	claims, ok := token.Claims.(*customClaims)
	if !ok || !token.Valid {
		return "", "", "", "", fmt.Errorf("invalid token claims")
	}

	role = claims.Role
	if role == "" {
		role = domain.RoleUser
	}
	return claims.Subject, claims.Nickname, claims.ID, role, nil
}
