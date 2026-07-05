package game

import (
	"math"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── 幽灵 AI ────────────────────────────────────────────────────────

// UpdateGhostAI 更新幽灵 AI
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
		attractStrength := 0.5
		ghost.VX += (dx / dist) * protocol.GhostSpeed * attractStrength
		ghost.VY += (dy / dist) * protocol.GhostSpeed * attractStrength
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
	rx := protocol.GhostCollisionRadiusX
	ry := protocol.GhostCollisionRadiusY
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