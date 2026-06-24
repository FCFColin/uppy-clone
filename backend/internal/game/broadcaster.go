package game

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

// Broadcaster 抽象跨实例消息广播。
// 企业为何需要：水平扩展后，同一房间的玩家可能连接到不同 Hub 实例。
// Redis Pub/Sub 提供跨实例广播通道，使本地 broadcast() 的消息能到达所有实例。
// nil broadcaster 表示单实例模式（不发布到 Redis，仅本地投递）。
type Broadcaster interface {
	// Publish 发送广播消息到所有订阅了该房间的实例。
	Publish(ctx context.Context, roomCode string, msg BroadcastMessage) error
	// Subscribe 注册指定房间频道的消息处理器，返回取消订阅函数。
	Subscribe(roomCode string, handler func(BroadcastMessage)) (unsubscribe func(), err error)
	// Close 关闭所有订阅与 pubsub 连接。
	Close() error
}

// BroadcastMessage 是通过 Redis Pub/Sub 发布的载荷。
type BroadcastMessage struct {
	RoomCode        string `json:"room_code"`
	ExcludePlayer   string `json:"exclude_player,omitempty"`
	ExcludeInstance string `json:"exclude_instance,omitempty"`
	Payload         []byte `json:"payload"`
	Critical        bool   `json:"critical,omitempty"`
}

// PubSubBroadcaster 基于 Redis Pub/Sub 实现 Broadcaster。
type PubSubBroadcaster struct {
	rdb        *redis.Client
	instanceID string
	connected  atomic.Bool
	subs       sync.Map // roomCode → *redis.PubSub
	logger     *slog.Logger
}

// NewPubSubBroadcaster 创建基于 Redis Pub/Sub 的广播器。
func NewPubSubBroadcaster(rdb *redis.Client) *PubSubBroadcaster {
	b := &PubSubBroadcaster{
		rdb:        rdb,
		instanceID: defaultInstanceID(),
		logger:     slog.Default().With("component", "broadcaster"),
	}
	b.connected.Store(true)
	return b
}

// channelName 返回房间对应的 Redis Pub/Sub 频道名。
func channelName(roomCode string) string {
	return "room:" + roomCode + ":broadcast"
}

// Publish 将消息序列化为 JSON 并发布到房间频道。
// 当 connected 为 false（Redis 不可用）时返回错误，调用方回退到仅本地投递。
func (b *PubSubBroadcaster) Publish(ctx context.Context, roomCode string, msg BroadcastMessage) error {
	if !b.connected.Load() {
		return fmt.Errorf("broadcaster not connected")
	}
	msg.RoomCode = roomCode
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	if err := b.rdb.Publish(ctx, channelName(roomCode), payload).Err(); err != nil {
		return fmt.Errorf("redis publish: %w", err)
	}
	return nil
}

// Subscribe 订阅房间频道，启动 goroutine 读取消息并调用 handler。
// 返回的取消订阅函数会关闭该订阅。
func (b *PubSubBroadcaster) Subscribe(roomCode string, handler func(BroadcastMessage)) (func(), error) {
	ctx := context.Background()
	ps := b.rdb.Subscribe(ctx, channelName(roomCode))
	b.subs.Store(roomCode, ps)

	go func() {
		ch := ps.Channel()
		for msg := range ch {
			var bm BroadcastMessage
			if err := json.Unmarshal([]byte(msg.Payload), &bm); err != nil {
				b.logger.Warn("unmarshal broadcast message", "error", err, "room", roomCode)
				continue
			}
			handler(bm)
		}
	}()

	unsubscribe := func() {
		if v, ok := b.subs.LoadAndDelete(roomCode); ok {
			if sub, ok := v.(*redis.PubSub); ok {
				_ = sub.Close()
			}
		}
	}
	return unsubscribe, nil
}

// Close 关闭所有活跃订阅。
func (b *PubSubBroadcaster) Close() error {
	b.connected.Store(false)
	b.subs.Range(func(key, value any) bool {
		if sub, ok := value.(*redis.PubSub); ok {
			_ = sub.Close()
		}
		b.subs.Delete(key)
		return true
	})
	return nil
}

// defaultInstanceID 返回实例标识：优先读取 INSTANCE_ID 环境变量，
// 否则使用 os.Hostname()。Hub、Room、Broadcaster 共享同一实例标识，
// 用于 ExcludeInstance 字段防止 Pub/Sub 消息回环。
func defaultInstanceID() string {
	if id := os.Getenv("INSTANCE_ID"); id != "" {
		return id
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "unknown"
	}
	return hostname
}
