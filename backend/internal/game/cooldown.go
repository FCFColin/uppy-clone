package game

import (
	"math"

	"github.com/uppy-clone/backend/internal/protocol"
)

// CalculateCooldown 对数冷却公式
//
//	cooldown_ms(N) = min(15000, round(1000 + 2032 · log₂(max(1, N))))
func CalculateCooldown(playerCount int) int64 {
	return int64(min(
		int(math.Round(float64(protocol.CooldownBaseMs)+float64(protocol.CooldownLogCoeff)*math.Log2(max(1, float64(playerCount))))),
		protocol.CooldownMaxMs,
	))
}
