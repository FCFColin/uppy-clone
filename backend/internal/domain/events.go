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

// DomainEvent 可嵌入到具体事件类型中，提供 OccurredAt 实现。
type DomainEvent struct {
	At time.Time
}

func (e DomainEvent) OccurredAt() time.Time { return e.At }

// PlayerJoined 玩家加入房间事件。
type PlayerJoined struct {
	DomainEvent
	RoomCode string
	UserID   string
	Nickname string
}

func (e PlayerJoined) EventType() string { return "player.joined" }

// PlayerLeft 玩家离开房间事件。
type PlayerLeft struct {
	DomainEvent
	RoomCode string
	UserID   string
}

func (e PlayerLeft) EventType() string { return "player.left" }

// GameEnded 游戏结束事件。
type GameEnded struct {
	DomainEvent
	RoomCode string
}

func (e GameEnded) EventType() string { return "game.ended" }

// PhaseChanged 游戏阶段转换事件。
type PhaseChanged struct {
	DomainEvent
	RoomCode string
	From     string
	To       string
}

func (e PhaseChanged) EventType() string { return "phase.changed" }

// UserHardDeleted 用户 GDPR 硬删除事件。
type UserHardDeleted struct {
	DomainEvent
	UserID string
}

func (e UserHardDeleted) EventType() string { return "user.hard_deleted" }
