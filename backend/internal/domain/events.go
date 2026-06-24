package domain

import "time"

// Event 是领域事件的基接口。
// 企业为何需要：领域事件解耦聚合间的通信，支持事件溯源与最终一致性。
//
// P3-6.3: 事件通过 Transactional Outbox（P1-10）持久化，
// 由 outbox/publisher.go 独立 goroutine 轮询发布到 Redis Stream。
type Event interface {
	EventType() string
	OccurredAt() time.Time
}

// PlayerJoined 玩家加入房间事件。
type PlayerJoined struct {
	RoomCode string
	UserID   string
	Nickname string
	At       time.Time
}

// EventType 返回事件类型标识。
func (e PlayerJoined) EventType() string { return "player.joined" }

// OccurredAt 返回事件发生时间。
func (e PlayerJoined) OccurredAt() time.Time { return e.At }

// PlayerLeft 玩家离开房间事件。
type PlayerLeft struct {
	RoomCode string
	UserID   string
	At       time.Time
}

// EventType 返回事件类型标识。
func (e PlayerLeft) EventType() string { return "player.left" }

// OccurredAt 返回事件发生时间。
func (e PlayerLeft) OccurredAt() time.Time { return e.At }

// GameEnded 游戏结束事件。
type GameEnded struct {
	RoomCode string
	At       time.Time
}

// EventType 返回事件类型标识。
func (e GameEnded) EventType() string { return "game.ended" }

// OccurredAt 返回事件发生时间。
func (e GameEnded) OccurredAt() time.Time { return e.At }

// PhaseChanged 游戏阶段转换事件。
type PhaseChanged struct {
	RoomCode string
	From     string
	To       string
	At       time.Time
}

// EventType 返回事件类型标识。
func (e PhaseChanged) EventType() string { return "phase.changed" }

// OccurredAt 返回事件发生时间。
func (e PhaseChanged) OccurredAt() time.Time { return e.At }
