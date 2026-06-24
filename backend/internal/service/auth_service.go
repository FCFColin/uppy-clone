package service

import (
	"context"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// AuthService 封装认证业务逻辑。
// 企业为何需要：分离业务逻辑使 handler 仅做协议转换，便于测试和复用。
//
// TODO(P3-2.4): handler 可逐步采用本 service，全量迁移为长期重构工作。
type AuthService struct {
	db         *store.PostgresStore
	redis      *store.RedisStore
	jwtMgr     *auth.JWTManager
	refreshMgr *auth.RefreshTokenManager
}

// NewAuthService 创建 AuthService。
func NewAuthService(db *store.PostgresStore, redis *store.RedisStore, jwtMgr *auth.JWTManager, refreshMgr *auth.RefreshTokenManager) *AuthService {
	return &AuthService{db: db, redis: redis, jwtMgr: jwtMgr, refreshMgr: refreshMgr}
}

// DeleteUserData 匿名化用户数据并撤销所有令牌（GDPR Article 17）。
func (s *AuthService) DeleteUserData(ctx context.Context, userID string) error {
	// 撤销所有 refresh token
	if s.redis != nil {
		_ = s.refreshMgr.RevokeAllForUser(ctx, userID)
	}
	// 匿名化 PII
	if err := s.db.AnonymizeUser(ctx, userID); err != nil {
		return err
	}
	return nil
}

// ExportUserData 导出用户数据用于 GDPR 合规。
func (s *AuthService) ExportUserData(ctx context.Context, userID string) (*domain.User, error) {
	return s.db.GetUserByID(ctx, userID)
}
