package game

import (
	"crypto/rand"
	"math"
	"math/big"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── 数学常量 ────────────────────────────────────────────────────────

const pi = math.Pi

// bigOneShift53 is cached to avoid allocating a new big.Int on every randFloat64 call.
var bigOneShift53 = big.NewInt(1 << 53)

func randFloat64() float64 {
	n, err := rand.Int(rand.Reader, bigOneShift53)
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
	dist := math.Sqrt(dx*dx + dy*dy)

	// 超出有效范围
	if dist > protocol.TapRange {
		if dist < 0.001 {
			// 中心点击给纯向上推力
			balloon.VY += protocol.TapForce
			return true
		}
		return false
	}

	// 推力大小随距离线性衰减
	forceMultiplier := 1 - dist/protocol.TapRange
	force := protocol.TapForce * forceMultiplier

	// 推力方向：从点击位置指向气球方向（推开气球）
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

	if state.Wind > protocol.WindClamp {
		state.Wind = protocol.WindClamp
	}
	if state.Wind < -protocol.WindClamp {
		state.Wind = -protocol.WindClamp
	}

	windScale := 1.0
	edgeDist := state.Balloon.X
	if rightDist := 1 - state.Balloon.X; rightDist < edgeDist {
		edgeDist = rightDist
	}
	if edgeDist < protocol.WindEdgeSoftZone {
		windScale = edgeDist / protocol.WindEdgeSoftZone
	}

	// 风力影响气球水平速度（靠边时减弱，避免被风压在边界）
	state.Balloon.VX += state.Wind * protocol.WindMax * windScale
}

// ─── 鸟 AI ───────────────────────────────────────────────────────────

// UpdateBirdAI 更新鸟 AI
func UpdateBirdAI(bird *domain.BirdState, balloon *domain.BalloonState, tickCount int) {
	if !bird.Active {
		bird.SpawnTimer--
		if bird.SpawnTimer <= 0 {
			fromLeft := randFloat64() > 0.5
			if fromLeft {
				bird.X = -0.1
			} else {
				bird.X = 1.1
			}
			bird.Y = randFloat64()*0.6 + 0.2 // 0.2 到 0.8
			bird.Active = true

			dx := balloon.X - bird.X
			dy := balloon.Y - bird.Y
			dist := math.Sqrt(dx*dx + dy*dy)
			bird.VX = (dx / dist) * protocol.BirdSpeed
			bird.VY = (dy / dist) * protocol.BirdSpeed
		}
	} else {
		bird.X += bird.VX
		bird.Y += bird.VY

		// 每30 ticks 重新校准方向
		if tickCount%30 == 0 {
			dx := balloon.X - bird.X
			dy := balloon.Y - bird.Y
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > 0 {
				bird.VX = (dx / dist) * protocol.BirdSpeed
				bird.VY = (dy / dist) * protocol.BirdSpeed
			}
		}

		// 离开屏幕时销毁
		if bird.X < -0.1 || bird.X > 1.1 || bird.Y < -0.1 || bird.Y > 1.1 {
			bird.Active = false
			bird.SpawnTimer = RandomSpawnTimer()
		}
	}
}

// CheckBirdCollision 检测鸟与气球碰撞
func CheckBirdCollision(bird *domain.BirdState, balloon *domain.BalloonState) bool {
	if !bird.Active {
		return false
	}
	dx := bird.X - balloon.X
	dy := bird.Y - balloon.Y
	distance := math.Sqrt(dx*dx + dy*dy)
	return distance < protocol.BirdCollisionRadius
}

// ─── 幽灵 AI ────────────────────────────────────────────────────────

// UpdateGhostAI 更新幽灵 AI
func UpdateGhostAI(state *domain.GameState) {
	ghost := &state.Ghost

	if !ghost.Active {
		ghost.SpawnTimer--
		if ghost.SpawnTimer <= 0 {
			// 从随机位置生成（对应 TS spawnGhost）
			spawned := spawnGhost()
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
		applyGhostAttractOrWander(ghost, state)
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
		ghost.SpawnTimer = int(protocol.GhostSpawnMin + randFloat64()*float64(protocol.GhostSpawnMax-protocol.GhostSpawnMin))
	}
}

func applyGhostRepel(ghost *domain.GhostState, state *domain.GameState) {
	ghost.RepelTimer--
	dx := ghost.X - state.Balloon.X
	dy := ghost.Y - state.Balloon.Y
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist == 0 {
		dist = 1
	}
	ghost.VX += (dx / dist) * protocol.GhostRepelForce
	ghost.VY += (dy / dist) * protocol.GhostRepelForce
}

func applyGhostAttractOrWander(ghost *domain.GhostState, state *domain.GameState) {
	dx := state.Balloon.X - ghost.X
	dy := state.Balloon.Y - ghost.Y
	dist := math.Sqrt(dx*dx + dy*dy)

	if dist < protocol.GhostAttractRadius {
		attractStrength := 0.5
		ghost.VX += (dx / dist) * protocol.GhostSpeed * attractStrength
		ghost.VY += (dy / dist) * protocol.GhostSpeed * attractStrength
		return
	}

	if state.TickCount%protocol.GhostWanderChangeInterval == 0 {
		angle := randFloat64() * 2 * pi
		ghost.VX = math.Cos(angle) * protocol.GhostSpeed
		ghost.VY = math.Sin(angle) * protocol.GhostSpeed
	}
}

func clampGhostVelocity(ghost *domain.GhostState) {
	maxSpeed := protocol.GhostSpeed * 4
	speed := math.Sqrt(ghost.VX*ghost.VX + ghost.VY*ghost.VY)
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
		r, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			code[i] = roomAlphabet[0]
			continue
		}
		code[i] = roomAlphabet[r.Int64()]
	}
	return string(code)
}
