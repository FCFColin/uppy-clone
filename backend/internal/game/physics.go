package game

import (
	"math"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ApplyPhysics applies balloon physics and returns whether the game is over.
func ApplyPhysics(balloon *domain.BalloonState) bool {
	// 重力将气球向下拉（vy 减小 = 向下）
	balloon.VY -= protocol.Gravity
	balloon.Y += balloon.VY

	// 水平运动 + 空气阻力
	balloon.X += balloon.VX
	balloon.VX *= protocol.HorizontalDrag

	// 水平边界：反弹（速度减半）
	if balloon.X <= 0 {
		balloon.X = 0
		balloon.VX = math.Abs(balloon.VX) * 0.5
	}
	if balloon.X >= 1 {
		balloon.X = 1
		balloon.VX = -math.Abs(balloon.VX) * 0.5
	}

	if balloon.Y <= 0 {
		return true
	}

	if balloon.Y >= 1 {
		balloon.Y = 1
		if balloon.VY > 0 {
			balloon.VY = 0
		}
	}

	return false
}

// ApplyTapForce calculates the tap push force and returns whether it was effective.
func ApplyTapForce(balloon *domain.BalloonState, tapX, tapY float64) bool {
	dx := balloon.X - tapX
	dy := balloon.Y - tapY
	distSq := dx*dx + dy*dy
	if distSq > protocol.TapRange*protocol.TapRange {
		return false
	}
	dist := math.Hypot(dx, dy)
	if dist < tapDistEpsilon {
		balloon.VY += protocol.TapForce
		return true
	}
	forceMultiplier := 1 - dist/protocol.TapRange
	force := protocol.TapForce * forceMultiplier
	nx := dx / dist
	ny := dy / dist

	balloon.VX += nx * force
	balloon.VY += ny * force

	return true
}

// tapDistEpsilon is the minimum tap distance threshold to avoid division by zero
// and denormalized float performance penalties. Well below any meaningful game
// distance (balloon radius ~0.06 in normalized coords).
const tapDistEpsilon = 1e-15

func tickCountdown(counter *int, resetFn func(), resetVal int) {
	*counter--
	if *counter <= 0 {
		resetFn()
		*counter = resetVal
	}
}

// UpdateWind updates the wind system (three-layer frequency).
func UpdateWind(state *domain.GameState, rng RNGSource) {
	tickCountdown(&state.WindMicroCountdown, func() {
		state.Wind += (rng.Float64() - 0.5) * protocol.WindJitter * 2
	}, protocol.WindMicroInterval)

	tickCountdown(&state.WindMidCountdown, func() {
		state.WindMidOffset = (rng.Float64() - 0.5) * 2 * protocol.WindMidMagnitude
	}, protocol.WindMidInterval)

	tickCountdown(&state.WindChangeCountdown, func() {
		state.WindTarget = (rng.Float64() - 0.5) * protocol.WindTargetSpan
	}, int(float64(protocol.WindChangeInterval)*(0.5+rng.Float64())))

	// 缓慢趋向目标风向 + 中频偏移
	effectiveTarget := state.WindTarget + state.WindMidOffset
	state.Wind += (effectiveTarget - state.Wind) * protocol.WindLerpRate

	state.Wind = max(-protocol.WindClamp, min(protocol.WindClamp, state.Wind))

	windScale := 1.0
	if edgeDist := min(state.Balloon.X, 1-state.Balloon.X); edgeDist < protocol.WindEdgeSoftZone {
		windScale = edgeDist / protocol.WindEdgeSoftZone
	}
	// game-030: Clamp windScale to non-negative — if Balloon.X is outside
	// [0,1], edgeDist can be negative, reversing wind effect.
	windScale = max(0, windScale)

	// 风力影响气球水平速度（靠边时减弱，避免被风压在边界）
	state.Balloon.VX += state.Wind * protocol.WindMax * windScale
}

// CalculateCooldown 对数冷却公式
//
//	cooldown_ms(N) = min(15000, round(1000 + 2032 · log₂(max(1, N))))
func CalculateCooldown(playerCount int) int64 {
	return int64(min(
		int(math.Round(float64(protocol.CooldownBaseMs)+float64(protocol.CooldownLogCoeff)*math.Log2(max(1, float64(playerCount))))),
		protocol.CooldownMaxMs,
	))
}

func setBirdVelocityToward(bird *domain.BirdState, balloon *domain.BalloonState, guardZeroDist bool) {
	dx := balloon.X - bird.X
	dy := balloon.Y - bird.Y
	dist := math.Hypot(dx, dy)
	if dist <= 0 || guardZeroDist && dist <= 0 {
		return
	}
	bird.VX = (dx / dist) * protocol.BirdSpeed
	bird.VY = (dy / dist) * protocol.BirdSpeed
}

// UpdateBirdAI updates the bird AI.
func UpdateBirdAI(bird *domain.BirdState, balloon *domain.BalloonState, tickCount int, rng RNGSource) {
	if !bird.Active {
		// game-028: Clamp SpawnTimer at 0 to prevent indefinite negative
		// values when bird is inactive for many ticks.
		if bird.SpawnTimer > 0 {
			bird.SpawnTimer--
		}
		if bird.SpawnTimer <= 0 {
			fromLeft := rng.Float64() > 0.5
			bird.X = 1.1
			if fromLeft {
				bird.X = -0.1
			}
			bird.Y = rng.Float64()*0.6 + 0.2 // 0.2 到 0.8
			bird.Active = true

			setBirdVelocityToward(bird, balloon, false)
		}
	} else {
		bird.X += bird.VX
		bird.Y += bird.VY

		// 每30 ticks 重新校准方向
		if tickCount%30 == 0 {
			setBirdVelocityToward(bird, balloon, true)
		}

		// 离开屏幕时销毁
		if bird.X < -0.1 || bird.X > 1.1 || bird.Y < -0.1 || bird.Y > 1.1 {
			bird.Active = false
			bird.SpawnTimer = RandomSpawnTimer(rng)
		}
	}
}

// RandomSpawnTimer returns a random spawn timer value between BirdSpawnMin and BirdSpawnMax.
func RandomSpawnTimer(rng RNGSource) int {
	lo := protocol.BirdSpawnMin
	hi := protocol.BirdSpawnMax
	return lo + rng.IntN(hi-lo+1)
}

// CheckBirdCollision checks whether the bird has collided with the balloon.
func CheckBirdCollision(bird *domain.BirdState, balloon *domain.BalloonState) bool {
	if !bird.Active {
		return false
	}
	dx := bird.X - balloon.X
	dy := bird.Y - balloon.Y
	// 鸟椭圆半轴 + 气球碰撞半径，让气球作为"圆"而非"点"参与碰撞
	rx := protocol.BirdCollisionRadiusX + protocol.BalloonCollisionRadius
	ry := protocol.BirdCollisionRadiusY + protocol.BalloonCollisionRadius
	return (dx*dx)/(rx*rx)+(dy*dy)/(ry*ry) < 1
}

// UpdateGhostAI updates the ghost AI.
func UpdateGhostAI(state *domain.GameState, rng RNGSource) {
	ghost := &state.Ghost

	if !ghost.Active {
		ghost.SpawnTimer--
		if ghost.SpawnTimer <= 0 {
			// 从随机位置生成（对应 TS spawnGhost）
			spawned := spawnGhost(rng)
			ghost.X = spawned.X
			ghost.Y = spawned.Y
			ghost.VX = spawned.VX
			ghost.VY = spawned.VY
			ghost.Active = spawned.Active
			ghost.RepelTimer = spawned.RepelTimer
			ghost.SpawnTimer = spawned.SpawnTimer
		}
		return
	}

	// 驱离倒计时
	if ghost.RepelTimer > 0 {
		applyGhostRepel(ghost, state)
	} else {
		applyGhostAttractOrWander(ghost, state, rng)
	}

	clampGhostVelocity(ghost)

	// 移动
	ghost.X += ghost.VX
	ghost.Y += ghost.VY

	// Y 轴边界弹反：保持幽灵始终在可见范围内，避免飘到屏幕下方看不见
	if ghost.Y < 0.02 {
		ghost.Y = 0.02
		ghost.VY = math.Abs(ghost.VY)
	}
	if ghost.Y > 0.98 {
		ghost.Y = 0.98
		ghost.VY = -math.Abs(ghost.VY)
	}

	// 离开屏幕（仅 X 轴）：销毁并等待重生
	if ghost.X < -0.15 || ghost.X > 1.15 {
		ghost.Active = false
		ghost.SpawnTimer = int(protocol.GhostSpawnMin + rng.Float64()*float64(protocol.GhostSpawnMax-protocol.GhostSpawnMin))
	}
}

func applyGhostRepel(ghost *domain.GhostState, state *domain.GameState) {
	ghost.RepelTimer--
	dx := ghost.X - state.Balloon.X
	dy := ghost.Y - state.Balloon.Y
	dist := math.Hypot(dx, dy)
	if dist == 0 {
		dist = 1
	}
	ghost.VX += (dx / dist) * protocol.GhostRepelForce
	ghost.VY += (dy / dist) * protocol.GhostRepelForce
}

func applyGhostAttractOrWander(ghost *domain.GhostState, state *domain.GameState, rng RNGSource) {
	dx := state.Balloon.X - ghost.X
	dy := state.Balloon.Y - ghost.Y
	dist := math.Hypot(dx, dy)
	if dist < protocol.GhostAttractRadius {
		if dist > 0 {
			attractStrength := 0.5
			ghost.VX += (dx / dist) * protocol.GhostSpeed * attractStrength
			ghost.VY += (dy / dist) * protocol.GhostSpeed * attractStrength
		}
		return
	}

	if state.TickCount%protocol.GhostWanderChangeInterval == 0 {
		angle := rng.Float64() * 2 * math.Pi
		ghost.VX = math.Cos(angle) * protocol.GhostSpeed
		ghost.VY = math.Sin(angle) * protocol.GhostSpeed
	}
}

func clampGhostVelocity(ghost *domain.GhostState) {
	maxSpeed := protocol.GhostSpeed * 4
	speed := math.Hypot(ghost.VX, ghost.VY)
	if speed > maxSpeed {
		ghost.VX = (ghost.VX / speed) * maxSpeed
		ghost.VY = (ghost.VY / speed) * maxSpeed
	}
}

// CheckGhostCollision 检查幽灵与气球的碰撞（椭圆碰撞，左右方向更紧）
func CheckGhostCollision(state *domain.GameState) bool {
	if !state.Ghost.Active {
		return false
	}
	dx := state.Balloon.X - state.Ghost.X
	dy := state.Balloon.Y - state.Ghost.Y
	// 鬼椭圆半轴 + 气球碰撞半径，让气球作为"圆"而非"点"参与碰撞
	rx := protocol.GhostCollisionRadiusX + protocol.BalloonCollisionRadius
	ry := protocol.GhostCollisionRadiusY + protocol.BalloonCollisionRadius
	// 椭圆碰撞: (dx/rx)² + (dy/ry)² < 1
	if (dx*dx)/(rx*rx)+(dy*dy)/(ry*ry) < 1 {
		// 气球受到向下速度冲击
		state.Balloon.VY -= protocol.GhostDamage
		// 幽灵弹开
		angle := math.Atan2(dy, dx)
		state.Ghost.VX = -math.Cos(angle) * protocol.GhostSpeed * 3
		state.Ghost.VY = -math.Sin(angle) * protocol.GhostSpeed * 3
		return true
	}
	return false
}

// ApplyGhostRepel 处理点击对幽灵的驱离效果
func ApplyGhostRepel(state *domain.GameState, tapX, tapY float64) {
	if !state.Ghost.Active {
		return
	}
	dx := state.Ghost.X - tapX
	dy := state.Ghost.Y - tapY
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist < protocol.GhostRepelRadius {
		state.Ghost.RepelTimer = protocol.GhostRepelDuration
	}
}
