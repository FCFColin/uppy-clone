package service

import (
	"context"

	"github.com/uppy-clone/backend/internal/store"
)

// AdminService 封装管理员业务逻辑。
// 企业为何需要：分离业务逻辑使 handler 仅做协议转换，便于测试和复用。
//
// TODO(P3-2.4): handler 可逐步采用本 service，全量迁移为长期重构工作。
type AdminService struct {
	db    *store.PostgresStore
	redis *store.RedisStore
}

// NewAdminService 创建 AdminService。
func NewAdminService(db *store.PostgresStore, redis *store.RedisStore) *AdminService {
	return &AdminService{db: db, redis: redis}
}

// VerifyLogin 校验管理员凭据并处理锁定逻辑。
// 返回 true 表示登录通过，false 表示凭据错误或被锁定。
//
// TODO(P3-2.4): 密码校验逻辑需从 handler/admin.go 迁移至此；当前为骨架。
func (s *AdminService) VerifyLogin(ctx context.Context, clientIP string) (bool, error) {
	// 检查锁定状态
	if s.redis != nil {
		locked, err := s.redis.IsLoginLocked(ctx, clientIP)
		if err == nil && locked {
			return false, nil
		}
	}

	// 获取配置（含管理员密码哈希）
	config, err := s.db.GetConfig(ctx, "global")
	if err != nil || config == nil {
		return false, nil
	}
	// 密码校验逻辑待从 handler 迁移
	_ = config
	return true, nil
}
