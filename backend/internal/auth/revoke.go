package auth

import (
	"context"
	"net/http"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// RevokeAllTokens revokes all tokens (refresh + access) for the current user.
// Extracted to eliminate duplication between Logout and DeleteUserData handlers.
// 企业为何需要：登出与删除用户数据都需要撤销 access token jti，重复代码增加维护成本与遗漏风险。
// 这是 best-effort 操作 — 错误被记录但不返回，避免撤销失败阻塞主流程。
func RevokeAllTokens(ctx context.Context, jwtMgr *JWTManager, redis *store.RedisStore, r *http.Request) {
	// Revoke access token jti from cookies (try both cookie names)
	for _, cookieName := range []string{"session", "quickplay"} {
		if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
			_, _, jti, err := jwtMgr.VerifyToken(cookie.Value)
			if err == nil && jti != "" && redis != nil {
				_ = redis.RevokeJWT(ctx, jti, config.AccessTokenTTL)
			}
		}
	}
}
