package game

import (
	"crypto/rand"
	"math"
	"math/big"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── 数学常量 ────────────────────────────────────────────────────────

// bigOneShift53 is cached to avoid allocating a new big.Int on every randFloat64 call.
var bigOneShift53 = big.NewInt(1 << 53)

func randFloat64() float64 {
	n, err := randIntFn(rand.Reader, bigOneShift53)
	if err != nil {
		return 0
	}
	return float64(n.Int64()) / float64(1<<53)
}

// ApplyPhysics 应用气球物理，返回 gameOver 标志
func ApplyPhysics(balloon *domain.BalloonState) bool {
	// 重力将气球向下拉（vy 减小 = 向下）
	balloon.VY -= protocol.Gravity
	balloon.Y += balloon.VY

	// 水平运动 + 空气阻力
	balloon.X += balloon.VX
	balloon.VX *= protocol.HorizontalDrag

	// 触底检测：y ≤ 0 → 游戏结束
	if balloon.Y <= 0 {
		return true
	}

	// 天花板检测：y ≥ 1 → 垂直速度归零
	if balloon.Y >= 1 {
		balloon.Y = 1
		if balloon.VY > 0 {
			balloon.VY = 0
		}
		return false
	}

	// 水平边界：反弹（速度减半）
	if balloon.X <= 0 {
		balloon.X = 0
		balloon.VX = math.Abs(balloon.VX) * 0.5
	}
	if balloon.X >= 1 {
		balloon.X = 1
		balloon.VX = -math.Abs(balloon.VX) * 0.5
	}

	return false
}

// ApplyTapForce 计算点击推力，返回推力是否有效
func ApplyTapForce(balloon *domain.BalloonState, tapX, tapY float64) bool {
	dx := balloon.X - tapX
	dy := balloon.Y - tapY
	dist := math.Hypot(dx, dy)
	if dist > protocol.TapRange {
		return false
	}
	forceMultiplier := 1 - dist/protocol.TapRange
	force := protocol.TapForce * forceMultiplier
	nx := dx / dist
	ny := dy / dist

	balloon.VX += nx * force
	balloon.VY += ny * force

	return true
}

// ─── 风场系统 ────────────────────────────────────────────────────────

// UpdateWind 更新风场（三层频率系统）
func UpdateWind(state *domain.GameState) {
	// === 高频微扰动 ===
	state.WindMicroCountdown--
	if state.WindMicroCountdown <= 0 {
		state.Wind += (randFloat64() - 0.5) * protocol.WindJitter * 2
		state.WindMicroCountdown = protocol.WindMicroInterval
	}

	// === 中频变化 ===
	state.WindMidCountdown--
	if state.WindMidCountdown <= 0 {
		state.WindMidOffset = (randFloat64() - 0.5) * 2 * protocol.WindMidMagnitude
		state.WindMidCountdown = protocol.WindMidInterval
	}

	// === 大变化 ===
	state.WindChangeCountdown--
	if state.WindChangeCountdown <= 0 {
		state.WindTarget = (randFloat64() - 0.5) * protocol.WindTargetSpan
		state.WindChangeCountdown = int(float64(protocol.WindChangeInterval) * (0.5 + randFloat64()))
	}

	// 缓慢趋向目标风向 + 中频偏移
	effectiveTarget := state.WindTarget + state.WindMidOffset
	state.Wind += (effectiveTarget - state.Wind) * protocol.WindLerpRate

	state.Wind = max(-protocol.WindClamp, min(protocol.WindClamp, state.Wind))

	windScale := 1.0
	if edgeDist := min(state.Balloon.X, 1-state.Balloon.X); edgeDist < protocol.WindEdgeSoftZone {
		windScale = edgeDist / protocol.WindEdgeSoftZone
	}

	// 风力影响气球水平速度（靠边时减弱，避免被风压在边界）
	state.Balloon.VX += state.Wind * protocol.WindMax * windScale
}

// ─── 工具函数 ────────────────────────────────────────────────────────

// RandomSpawnTimer 生成随机鸟生成倒计时
func RandomSpawnTimer() int {
	lo := protocol.BirdSpawnMin
	hi := protocol.BirdSpawnMax
	return min(int(randFloat64()*float64(hi-lo+1))+lo, hi)
}

// CalculateCooldown 对数冷却公式
//
//	cooldown_ms(N) = min(15000, round(1500 + 2032 · log₂(max(1, N))))
func CalculateCooldown(playerCount int) int64 {
	return int64(min(
		int(math.Round(float64(protocol.CooldownBaseMs)+float64(protocol.CooldownLogCoeff)*math.Log2(max(1, float64(playerCount))))),
		protocol.CooldownMaxMs,
	))
}

// roomAlphabet is the character set used for room codes.
const roomAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// alphabetLen is cached to avoid allocating a new big.Int on every GenerateRoomCode call.
var alphabetLen = big.NewInt(int64(len(roomAlphabet)))

// GenerateRoomCode 生成 5 字符房间码
func GenerateRoomCode() string {
	code := make([]byte, 5)
	for i := 0; i < 5; i++ {
		r, err := randIntFn(rand.Reader, alphabetLen)
		if err != nil {
			code[i] = roomAlphabet[0]
			continue
		}
		code[i] = roomAlphabet[r.Int64()]
	}
	return string(code)
}
