package game

import (
	"context"

	"github.com/uppy-clone/backend/internal/domain"
)

// RoomRepository 是房间持久化的接口（依赖倒置）。
// 企业为何需要：依赖倒置使 game 包不依赖具体的存储实现，便于测试和替换。
//
// P3-3.2: store.PostgresStore 已实现本接口（SaveLobbyState/LoadLobbyState/DeleteLobbyState
// 签名与本接口一致）。
//
// TODO(P3-3.4): 当前 Hub/Room 仍直接依赖 *store.PostgresStore；
// 未来重构应通过依赖注入传入 RoomRepository 接口，使 game 包可独立测试。
type RoomRepository interface {
	SaveLobbyState(ctx context.Context, ls *domain.LobbyState) error
	LoadLobbyState(ctx context.Context, code string) (*domain.LobbyState, error)
	DeleteLobbyState(ctx context.Context, code string) error
}

// SnapshotEncoder 是快照编码的接口（依赖倒置）。
//
// P3-3.3: protocol.EncodeSnapshot 当前签名接收多个参数而非 *domain.GameState，
// 未来需提供一个 adapter 包装 EncodeSnapshot 以实现本接口。
//
// TODO(P3-3.4): 当前 Room.buildSnapshot 直接调用 protocol.EncodeSnapshot；
// 未来重构应通过依赖注入传入 SnapshotEncoder 接口。
type SnapshotEncoder interface {
	Encode(state *domain.GameState) ([]byte, error)
}
