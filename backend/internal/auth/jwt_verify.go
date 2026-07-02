package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// VerifyToken validates a JWT and returns userId, nickname, and jti.
func (m *JWTManager) VerifyToken(tokenStr string) (userID, nickname, jti string, err error) {
	userID, nickname, jti, err = m.verifyWithKey(tokenStr, m.primarySecret)
	if err == nil {
		return
	}
	if m.previousSecret != nil {
		return m.verifyWithKey(tokenStr, m.previousSecret)
	}
	return
}

type jwtParseFunc func(string, jwt.Claims, jwt.Keyfunc, ...jwt.ParserOption) (*jwt.Token, error)

// jwtParseWithClaimsFn is injectable for unit tests (e.g. invalid claims paths).
var jwtParseWithClaimsFn jwtParseFunc = jwt.ParseWithClaims

func (m *JWTManager) verifyWithKey(tokenStr string, key []byte) (userID, nickname, jti string, err error) {
	token, err := jwtParseWithClaimsFn(tokenStr, &customClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	})
	if err != nil {
		return "", "", "", fmt.Errorf("verify token: %w", err)
	}

	claims, ok := token.Claims.(*customClaims)
	if !ok || !token.Valid {
		return "", "", "", fmt.Errorf("invalid token claims")
	}

	return claims.Subject, claims.Nickname, claims.ID, nil
}
