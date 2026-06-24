package service

import (
	"github.com/uppy-clone/backend/internal/game"
)

// LobbyService 封装大厅业务逻辑。
// 企业为何需要：分离业务逻辑使 handler 仅做协议转换，便于测试和复用。
//
// TODO(P3-2.4): handler 可逐步采用本 service，全量迁移为长期重构工作。
type LobbyService struct {
	hub *game.Hub
}

// NewLobbyService 创建 LobbyService。
func NewLobbyService(hub *game.Hub) *LobbyService {
	return &LobbyService{hub: hub}
}

// Hub 返回底层 Hub 引用（过渡期保留，便于 handler 渐进迁移）。
func (s *LobbyService) Hub() *game.Hub {
	return s.hub
}
