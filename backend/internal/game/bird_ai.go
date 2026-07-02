package game

import (
	"math"
	"math/rand/v2"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

// ─── 鸟 AI ───────────────────────────────────────────────────────────

func setBirdVelocityToward(bird *domain.BirdState, balloon *domain.BalloonState, guardZeroDist bool) {
	dx := balloon.X - bird.X
	dy := balloon.Y - bird.Y
	dist := math.Hypot(dx, dy)
	if guardZeroDist && dist <= 0 {
		return
	}
	bird.VX = (dx / dist) * protocol.BirdSpeed
	bird.VY = (dy / dist) * protocol.BirdSpeed
}

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
			bird.SpawnTimer = RandomSpawnTimer()
		}
	}
}

func RandomSpawnTimer() int {
	lo := protocol.BirdSpawnMin
	hi := protocol.BirdSpawnMax
	return lo + rand.IntN(hi-lo+1)
}

// CheckBirdCollision 检测鸟与气球碰撞
func CheckBirdCollision(bird *domain.BirdState, balloon *domain.BalloonState) bool {
	if !bird.Active {
		return false
	}
	dx := bird.X - balloon.X
	dy := bird.Y - balloon.Y
	dist := math.Hypot(dx, dy)
	return dist < protocol.BirdCollisionRadius
}