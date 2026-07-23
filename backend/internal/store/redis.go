package store

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

type baseRedisStore struct {
	rdb  *redis.Client
	cb   *gobreaker.CircuitBreaker[any]
	deps Deps
}

func newBaseRedisStore(rdb *redis.Client, deps Deps) baseRedisStore {
	return baseRedisStore{rdb: rdb, cb: deps.RedisBreakerFactory(), deps: deps}
}

func newBaseRedisStoreWithBreaker(rdb *redis.Client, cb *gobreaker.CircuitBreaker[any], deps Deps) baseRedisStore {
	return baseRedisStore{rdb: rdb, cb: cb, deps: deps}
}

// RedisStore wraps a go-redis client with circuit breaker protection.
// Domain methods are provided by embedded typed stores via method promotion.
type RedisStore struct {
	rdb  *redis.Client
	cb   *gobreaker.CircuitBreaker[any]
	deps Deps

	*SessionStore
	*RateLimitStore
	*RoomRegistryStore
	*MagicLinkStore
	*EmailQueueStore
	*LobbyReadCacheStore
}

func NewRedisStore(redisURL string, timeouts config.TimeoutConfig, deps ...Deps) (*RedisStore, error) {
	d := depsOrZero(deps...)
	conn, err := config.ParseRedisURL(redisURL)
	if err != nil {
		return nil, err
	}
	poolSize := config.GetEnvIntPositive("REDIS_POOL_SIZE", 20)
	minIdleConns := config.GetEnvIntPositive("REDIS_MIN_IDLE_CONNS", 5)

	opts := &redis.Options{
		Addr:            conn.Addr,
		Password:        conn.Password,
		PoolSize:        poolSize,
		MinIdleConns:    minIdleConns,
		PoolTimeout:     4 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,
		ReadTimeout:     timeouts.RedisReadTimeout,
		WriteTimeout:    timeouts.RedisWriteTimeout,
		MaxRetries:      0,
	}
	if strings.HasPrefix(redisURL, "rediss://") {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), timeouts.RedisConnectTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return newRedisStoreFromClient(rdb, d), nil
}

func NewRedisStoreFromClient(rdb *redis.Client, deps ...Deps) *RedisStore {
	return newRedisStoreFromClient(rdb, depsOrZero(deps...))
}

func newRedisStoreFromClient(rdb *redis.Client, deps Deps) *RedisStore {
	cb := deps.RedisBreakerFactory()
	base := newBaseRedisStoreWithBreaker(rdb, cb, deps)
	return &RedisStore{
		rdb:  rdb,
		cb:   cb,
		deps: deps,

		SessionStore:        &SessionStore{baseRedisStore: base},
		RateLimitStore:      &RateLimitStore{baseRedisStore: base},
		RoomRegistryStore:   &RoomRegistryStore{baseRedisStore: base},
		MagicLinkStore:      &MagicLinkStore{baseRedisStore: base},
		EmailQueueStore:     &EmailQueueStore{baseRedisStore: base},
		LobbyReadCacheStore: &LobbyReadCacheStore{baseRedisStore: base},
	}
}

func (s *RedisStore) PoolStats() *redis.PoolStats { return s.rdb.PoolStats() }

func (s *RedisStore) CircuitBreaker() *gobreaker.CircuitBreaker[any] { return s.cb }

func (s *RedisStore) Client() *redis.Client { return s.rdb }

func (s *RedisStore) Close() error { return s.rdb.Close() }

// RedisCluster holds domain-separated Redis stores for fault isolation (ADR-029).
//
// Stateful: room registry, JWT revocation, magic links, refresh tokens,
//
//	email queue, game results, outbox, Pub/Sub.
//	Requires persistence (AOF/RDB), low-latency, HA (Sentinel).
//
// Ephemeral: rate limiting.
//
//	Tolerant of data loss, can fail-open, no persistence needed.
//
// When REDIS_EPHEMERAL_URL is unset, Ephemeral falls back to Stateful
// (single-instance backward compatibility).
type RedisCluster struct {
	Stateful  *RedisStore
	Ephemeral *RedisStore
}

// NewRedisCluster creates a RedisCluster from stateful and ephemeral URLs.
// If ephemeralURL is empty, the stateful store is reused for both domains.
func NewRedisCluster(statefulURL, ephemeralURL string, timeouts config.TimeoutConfig, deps ...Deps) (*RedisCluster, error) {
	stateful, err := NewRedisStore(statefulURL, timeouts, deps...)
	if err != nil {
		return nil, err
	}

	var ephemeral *RedisStore
	if ephemeralURL == "" {
		ephemeral = stateful
	} else {
		ephemeral, err = NewRedisStore(ephemeralURL, timeouts, deps...)
		if err != nil {
			_ = stateful.Close()
			return nil, err
		}
	}

	return &RedisCluster{
		Stateful:  stateful,
		Ephemeral: ephemeral,
	}, nil
}

func (c *RedisCluster) IsSeparated() bool {
	return c.Stateful != c.Ephemeral
}

func (c *RedisCluster) Close() error {
	if c.IsSeparated() {
		_ = c.Ephemeral.Close()
	}
	return c.Stateful.Close()
}

func (c *RedisCluster) CircuitBreakers() []*gobreaker.CircuitBreaker[any] {
	cbs := []*gobreaker.CircuitBreaker[any]{c.Stateful.CircuitBreaker()}
	if c.IsSeparated() {
		cbs = append(cbs, c.Ephemeral.CircuitBreaker())
	}
	return cbs
}

const lobbyListCachePrefix = "lobby:list:"
const lobbyCheckCachePrefix = "lobby:check:"

const LobbyReadCacheTTL = 30 * time.Second

func lobbyListCacheKey(limit int, cursor string) string {
	return fmt.Sprintf("%s%d:%s", lobbyListCachePrefix, limit, cursor)
}

func lobbyCheckCacheKey(code string) string {
	return lobbyCheckCachePrefix + code
}

// LobbyReadCacheStore handles ADR-006 read-through cache for lobby list and
// room-check responses in Redis.
type LobbyReadCacheStore struct {
	baseRedisStore
}

func (s *LobbyReadCacheStore) GetCachedLobbyList(ctx context.Context, limit int, cursor string) ([]byte, bool, error) {
	var data []byte
	var found bool
	_, err := s.cb.Execute(func() (any, error) {
		key := lobbyListCacheKey(limit, cursor)
		val, err := s.rdb.Get(ctx, key).Bytes()
		if err == redis.Nil {
			data, found = nil, false
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get lobby list cache: %w", err)
		}
		data, found = val, true
		return nil, nil
	})
	if err != nil {
		return nil, false, err
	}
	return data, found, nil
}

func (s *LobbyReadCacheStore) SetCachedLobbyList(ctx context.Context, limit int, cursor string, data []byte) error {
	_, err := s.cb.Execute(func() (any, error) {
		key := lobbyListCacheKey(limit, cursor)
		if err := s.rdb.Set(ctx, key, data, LobbyReadCacheTTL).Err(); err != nil {
			return nil, fmt.Errorf("set lobby list cache: %w", err)
		}
		return nil, nil
	})
	return err
}

func (s *LobbyReadCacheStore) GetCachedRoomCheck(ctx context.Context, code string) ([]byte, bool, error) {
	var data []byte
	var found bool
	_, err := s.cb.Execute(func() (any, error) {
		key := lobbyCheckCacheKey(code)
		val, err := s.rdb.Get(ctx, key).Bytes()
		if err == redis.Nil {
			data, found = nil, false
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get room check cache: %w", err)
		}
		data, found = val, true
		return nil, nil
	})
	if err != nil {
		return nil, false, err
	}
	return data, found, nil
}

func (s *LobbyReadCacheStore) SetCachedRoomCheck(ctx context.Context, code string, data []byte) error {
	_, err := s.cb.Execute(func() (any, error) {
		key := lobbyCheckCacheKey(code)
		if err := s.rdb.Set(ctx, key, data, LobbyReadCacheTTL).Err(); err != nil {
			return nil, fmt.Errorf("set room check cache: %w", err)
		}
		return nil, nil
	})
	return err
}

func (s *LobbyReadCacheStore) InvalidateLobbyListCaches(ctx context.Context) error {
	_, err := s.cb.Execute(func() (any, error) {
		var cursor uint64
		for {
			keys, next, err := s.rdb.Scan(ctx, cursor, lobbyListCachePrefix+"*", 100).Result()
			if err != nil {
				return nil, fmt.Errorf("scan lobby list cache keys: %w", err)
			}
			if len(keys) > 0 {
				if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
					return nil, fmt.Errorf("delete lobby list cache keys: %w", err)
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		return nil, nil
	})
	return err
}

func (s *LobbyReadCacheStore) InvalidateRoomCheck(ctx context.Context, code string) error {
	_, err := s.cb.Execute(func() (any, error) {
		if err := s.rdb.Del(ctx, lobbyCheckCacheKey(code)).Err(); err != nil {
			return nil, fmt.Errorf("delete room check cache: %w", err)
		}
		return nil, nil
	})
	return err
}

type RoomRegistryStore struct {
	baseRedisStore
}

func NewRoomRegistryStore(rdb *redis.Client, deps ...Deps) *RoomRegistryStore {
	d := depsOrZero(deps...)
	return &RoomRegistryStore{baseRedisStore: newBaseRedisStore(rdb, d)}
}

func (s *RoomRegistryStore) RegisterRoom(ctx context.Context, code string, data []byte, ttl time.Duration) error {
	ctx, span := s.deps.Tracer.Start(ctx, "room_registry.RegisterRoom",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "SET"),
		),
	)
	defer span.End()

	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		if setErr := s.rdb.Set(ctx, key, data, ttl).Err(); setErr != nil {
			return nil, fmt.Errorf("register room: %w", setErr)
		}
		// Maintain a SET index for O(1) listing instead of SCAN (store-015).
		if sAddErr := s.rdb.SAdd(ctx, roomIndexKey(), code).Err(); sAddErr != nil {
			return nil, fmt.Errorf("register room index: %w", sAddErr)
		}
		return nil, nil
	})
	return err
}

func (s *RoomRegistryStore) UnregisterRoom(ctx context.Context, code string) error {
	key := roomInfoKey(code)
	_, err := s.cb.Execute(func() (any, error) {
		if delErr := s.rdb.Del(ctx, key).Err(); delErr != nil {
			return nil, fmt.Errorf("unregister room: %w", delErr)
		}
		// Remove from the SET index (store-015).
		if sRemErr := s.rdb.SRem(ctx, roomIndexKey(), code).Err(); sRemErr != nil {
			return nil, fmt.Errorf("unregister room index: %w", sRemErr)
		}
		return nil, nil
	})
	return err
}

func (s *RoomRegistryStore) GetRoomRegistry(ctx context.Context, code string) (*domain.RoomRegistryInfo, error) {
	ctx, span := s.deps.Tracer.Start(ctx, "room_registry.GetRoomRegistry",
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "GET"),
		),
	)
	defer span.End()

	raw, err := s.rdb.Get(ctx, roomInfoKey(code)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get room registry: %w", err)
	}
	info, unmarshalErr := domain.UnmarshalRoomRegistryInfo(raw)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}
	return info, nil
}
