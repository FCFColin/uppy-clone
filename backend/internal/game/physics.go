package game

import (
	"math"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

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
func UpdateWind(state *domain.GameState, rng RNGSource) {
	// === 高频微扰动 ===
	state.WindMicroCountdown--
	if state.WindMicroCountdown <= 0 {
		state.Wind += (rng.Float64() - 0.5) * protocol.WindJitter * 2
		state.WindMicroCountdown = protocol.WindMicroInterval
	}

	// === 中频变化 ===
	state.WindMidCountdown--
	if state.WindMidCountdown <= 0 {
		state.WindMidOffset = (rng.Float64() - 0.5) * 2 * protocol.WindMidMagnitude
		state.WindMidCountdown = protocol.WindMidInterval
	}

	// === 大变化 ===
	state.WindChangeCountdown--
	if state.WindChangeCountdown <= 0 {
		state.WindTarget = (rng.Float64() - 0.5) * protocol.WindTargetSpan
		state.WindChangeCountdown = int(float64(protocol.WindChangeInterval) * (0.5 + rng.Float64()))
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




