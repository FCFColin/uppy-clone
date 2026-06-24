package service

import (
	"context"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// CommandService 处理写操作（CQRS 写路径）。
// 企业为何需要：CQRS 分离读写路径，写路径可独立优化事务与一致性策略。
//
// P3-7.3: 未来重构可为写路径使用独立的主库连接，与读路径物理隔离。
type CommandService struct {
	db *store.PostgresStore
}

// NewCommandService 创建 CommandService。
func NewCommandService(db *store.PostgresStore) *CommandService {
	return &CommandService{db: db}
}

// CreateUser 创建用户（写操作）。
func (c *CommandService) CreateUser(ctx context.Context, user *domain.User) error {
	return c.db.CreateUser(ctx, user)
}

// AnonymizeUser 匿名化用户数据（写操作，GDPR Article 17）。
func (c *CommandService) AnonymizeUser(ctx context.Context, userID string) error {
	return c.db.AnonymizeUser(ctx, userID)
}
