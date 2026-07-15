package store

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// multiIPLoginScript is a Lua script that atomically adds an IP to the user's
// IP set, refreshes the TTL, and returns the cardinality — all in a single
// Redis round-trip (auth-009: was 3 separate calls → 1).
var multiIPLoginScript = redis.NewScript(`
redis.call('SADD', KEYS[1], ARGV[1])
redis.call('EXPIRE', KEYS[1], ARGV[2])
return redis.call('SCARD', KEYS[1])
`)

// TrackUserIPs adds clientIP to the user's IP set with a 1-hour TTL and
// returns the count of distinct IPs seen in the window. This is the pure
// Redis Lua operation extracted from the auth package (RO-046).
func TrackUserIPs(ctx context.Context, rdb redis.Scripter, userID, clientIP string) (int64, error) {
	ipKey := "user:ips:" + userID
	return multiIPLoginScript.Run(ctx, rdb, []string{ipKey}, clientIP, int(time.Hour.Seconds())).Int64()
}
