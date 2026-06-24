package service

import (
	"context"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// QueryService 处理读操作（CQRS 读路径）。
// 企业为何需要：CQRS 分离读写路径，读路径可使用缓存/只读副本提升性能。
//
// P3-7.3: 未来重构可为读路径使用独立的只读数据库连接（read replica），
// 进一步提升查询吞吐并减轻主库压力。
type QueryService struct {
	db *store.PostgresStore
}

// NewQueryService 创建 QueryService。
func NewQueryService(db *store.PostgresStore) *QueryService {
	return &QueryService{db: db}
}

// GetUserByID 按 ID 查询用户。
func (q *QueryService) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return q.db.GetUserByID(ctx, id)
}

// GetGameResultsByUserID 查询指定用户的游戏结果列表。
func (q *QueryService) GetGameResultsByUserID(ctx context.Context, userID string) ([]domain.GameResult, error) {
	return q.db.GetGameResultsByUserID(ctx, userID)
}
