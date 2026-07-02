package game

import (
	"math"
	"regexp"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestRandFloat64_InRange(t *testing.T) {
	for i := 0; i < 200; i++ {
		v := randFloat64()
		if v < 0 || v >= 1 {
			t.Fatalf("randFloat64() = %v, want in [0,1)", v)
		}
	}
}

func TestUpdateWind_ClampAndEdgeZone(t *testing.T) {
	state := createTestState()
	state.Wind = 10
	state.WindTarget = 10
	state.WindMidOffset = 0
	state.WindMicroCountdown = 1
	state.WindMidCountdown = 1
	state.WindChangeCountdown = 1
	state.Balloon.X = 0.01
	UpdateWind(state)
	if state.Wind > protocol.WindClamp {
		t.Fatalf("Wind should be clamped, got %v", state.Wind)
	}

	state.Wind = -10
	state.WindTarget = -10
	state.WindMicroCountdown = 1
	state.WindMidCountdown = 1
	state.WindChangeCountdown = 1
	UpdateWind(state)
	if state.Wind < -protocol.WindClamp {
		t.Fatalf("Wind should be clamped low, got %v", state.Wind)
	}
}

func TestSetBirdVelocityToward_ZeroDistanceGuard(t *testing.T) {
	bird := &domain.BirdState{X: 0.5, Y: 0.5, VX: 0.01, VY: 0.02}
	balloon := &domain.BalloonState{X: 0.5, Y: 0.5}
	setBirdVelocityToward(bird, balloon, true)
	if bird.VX != 0.01 || bird.VY != 0.02 {
		t.Fatalf("velocity should be unchanged when dist=0, got VX=%v VY=%v", bird.VX, bird.VY)
	}
}

func TestApplyPhysics_Gravity(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}
	yBefore := balloon.Y

	gameOver := ApplyPhysics(&balloon)

	if gameOver {
		t.Fatal("不应游戏结束")
	}
	if balloon.VY >= 0 {
		t.Fatalf("重力应使 VY 减小（向下），got VY=%v", balloon.VY)
	}
	if balloon.Y >= yBefore {
		t.Fatalf("重力应使 Y 减小（下落），got Y=%v, before=%v", balloon.Y, yBefore)
	}
}

func TestApplyPhysics_LeftBoundary(t *testing.T) {
	balloon := domain.BalloonState{X: 0, Y: 0.5, VX: -0.01, VY: 0}

	ApplyPhysics(&balloon)

	if balloon.X != 0 {
		t.Fatalf("左边界应钳制 X=0，got X=%v", balloon.X)
	}
	if balloon.VX <= 0 {
		t.Fatalf("左边界反弹后 VX 应为正，got VX=%v", balloon.VX)
	}
}

func TestApplyPhysics_RightBoundary(t *testing.T) {
	balloon := domain.BalloonState{X: 1, Y: 0.5, VX: 0.01, VY: 0}

	ApplyPhysics(&balloon)

	if balloon.X != 1 {
		t.Fatalf("右边界应钳制 X=1，got X=%v", balloon.X)
	}
	if balloon.VX >= 0 {
		t.Fatalf("右边界反弹后 VX 应为负，got VX=%v", balloon.VX)
	}
}

func TestApplyPhysics_HorizontalDrag(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0.1, VY: 0}

	ApplyPhysics(&balloon)

	if math.Abs(balloon.VX) >= 0.1 {
		t.Fatalf("空气阻力应使水平速度衰减，got VX=%v", balloon.VX)
	}
}

func TestApplyPhysics_GameOver(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.001, VX: 0, VY: -0.01}

	gameOver := ApplyPhysics(&balloon)

	if !gameOver {
		t.Fatal("触底应导致游戏结束")
	}
}

func TestApplyPhysics_Ceiling(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.999, VX: 0, VY: 0.05}

	gameOver := ApplyPhysics(&balloon)

	if gameOver {
		t.Fatal("撞天花板不应游戏结束")
	}
	if balloon.VY > 0 {
		t.Fatalf("撞天花板后 VY 应归零或为负，got VY=%v", balloon.VY)
	}
}

// ─── 点击推力 ────────────────────────────────────────────────────────

func TestApplyTapForce_InRange(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}

	ok := ApplyTapForce(&balloon, 0.5, 0.3)

	if !ok {
		t.Fatal("点击在有效范围内应返回 true")
	}
	if balloon.VY <= 0 {
		t.Fatalf("点击在气球下方应获得向上速度，got VY=%v", balloon.VY)
	}
}

func TestApplyTapForce_OutOfRange(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}
	vyBefore := balloon.VY

	ok := ApplyTapForce(&balloon, 0.1, 0.1)

	if ok {
		t.Fatal("点击超出有效范围应返回 false")
	}
	if balloon.VY != vyBefore {
		t.Fatalf("超出范围时 VY 不应变，got VY=%v, before=%v", balloon.VY, vyBefore)
	}
}

func TestApplyTapForce_Center(t *testing.T) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}
	vyBefore := balloon.VY

	ok := ApplyTapForce(&balloon, 0.5, 0.5)

	if !ok {
		t.Fatal("点击气球中心应返回 true")
	}
	if math.Abs((balloon.VY-vyBefore)-protocol.TapForce) > 1e-9 {
		t.Fatalf("中心点击应给纯向上推力 TAP_FORCE，got VY diff=%v, want=%v",
			balloon.VY-vyBefore, protocol.TapForce)
	}
}

// ─── 幽灵碰撞 ────────────────────────────────────────────────────────

func TestCheckGhostCollision_Overlap(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Balloon.X = 0.5
	state.Balloon.Y = 0.5

	if !CheckGhostCollision(state) {
		t.Fatal("幽灵与气球重叠时应返回 true")
	}
}

func TestCheckGhostCollision_Edge(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Balloon.X = 0.5 + protocol.GhostCollisionRadiusX*0.9
	state.Balloon.Y = 0.5

	if !CheckGhostCollision(state) {
		t.Fatal("幽灵在碰撞半径边缘应返回 true")
	}
}

func TestCheckGhostCollision_Outside(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Balloon.X = 0.5 + protocol.GhostCollisionRadiusX*2
	state.Balloon.Y = 0.5

	if CheckGhostCollision(state) {
		t.Fatal("幽灵在碰撞半径外应返回 false")
	}
}

func TestCheckGhostCollision_Inactive(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false
	state.Ghost.X = state.Balloon.X
	state.Ghost.Y = state.Balloon.Y

	if CheckGhostCollision(state) {
		t.Fatal("幽灵未激活时应返回 false")
	}
}

func TestCheckGhostCollision_Damage(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = state.Balloon.X
	state.Ghost.Y = state.Balloon.Y
	vyBefore := state.Balloon.VY

	CheckGhostCollision(state)

	if state.Balloon.VY != vyBefore-protocol.GhostDamage {
		t.Fatalf("碰撞后气球 VY 应减少 GHOST_DAMAGE，got VY=%v, want=%v",
			state.Balloon.VY, vyBefore-protocol.GhostDamage)
	}
}

func TestCheckGhostCollision_GhostBounce(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Balloon.X = 0.5
	state.Balloon.Y = 0.5

	CheckGhostCollision(state)

	speed := math.Sqrt(state.Ghost.VX*state.Ghost.VX + state.Ghost.VY*state.Ghost.VY)
	expectedSpeed := protocol.GhostSpeed * 3
	if math.Abs(speed-expectedSpeed) > 1e-9 {
		t.Fatalf("碰撞后幽灵弹开速度应为 GHOST_SPEED*3=%v，got=%v", expectedSpeed, speed)
	}
}

// ─── 鸟碰撞 ──────────────────────────────────────────────────────────

func TestCheckBirdCollision_Overlap(t *testing.T) {
	bird := domain.BirdState{X: 0.5, Y: 0.5, Active: true}
	balloon := domain.BalloonState{X: 0.5, Y: 0.5}

	if !CheckBirdCollision(&bird, &balloon) {
		t.Fatal("鸟与气球重叠时应返回 true")
	}
}

func TestCheckBirdCollision_Inactive(t *testing.T) {
	bird := domain.BirdState{X: 0.5, Y: 0.5, Active: false}
	balloon := domain.BalloonState{X: 0.5, Y: 0.5}

	if CheckBirdCollision(&bird, &balloon) {
		t.Fatal("鸟未激活时应返回 false")
	}
}

func TestCheckBirdCollision_Far(t *testing.T) {
	bird := domain.BirdState{X: 0.1, Y: 0.1, Active: true}
	balloon := domain.BalloonState{X: 0.9, Y: 0.9}

	if CheckBirdCollision(&bird, &balloon) {
		t.Fatal("鸟远离气球时应返回 false")
	}
}

// ─── 幽灵 AI ─────────────────────────────────────────────────────────

func TestUpdateGhostAI_Movement(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.VX = protocol.GhostSpeed
	state.Ghost.VY = 0
	state.Ghost.RepelTimer = 0
	xBefore := state.Ghost.X
	yBefore := state.Ghost.Y

	UpdateGhostAI(state)

	moved := state.Ghost.X != xBefore || state.Ghost.Y != yBefore
	if !moved {
		t.Fatal("幽灵每 tick 位置应发生变化")
	}
}

func TestUpdateGhostAI_MaxSpeed(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.VX = protocol.GhostSpeed * 10
	state.Ghost.VY = protocol.GhostSpeed * 10
	state.Ghost.RepelTimer = 0

	UpdateGhostAI(state)

	speed := math.Sqrt(state.Ghost.VX*state.Ghost.VX + state.Ghost.VY*state.Ghost.VY)
	maxSpeed := protocol.GhostSpeed * 4
	if speed > maxSpeed+0.0001 {
		t.Fatalf("幽灵速度不应超过最大速度 %v，got %v", maxSpeed, speed)
	}
}

func TestUpdateGhostAI_YBoundaryBounce(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.01 // 低于 0.02 下界
	state.Ghost.VX = 0
	state.Ghost.VY = -0.005 // 向下移动
	state.Ghost.RepelTimer = 0

	UpdateGhostAI(state)

	if state.Ghost.Y < 0.02 {
		t.Fatalf("幽灵 Y 应被弹反至 >= 0.02，got Y=%v", state.Ghost.Y)
	}
	if state.Ghost.VY <= 0 {
		t.Fatalf("弹反后 VY 应为正（向上），got VY=%v", state.Ghost.VY)
	}
}

func TestUpdateGhostAI_YBoundaryTopBounce(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.99 // 高于 0.98 上界
	state.Ghost.VX = 0
	state.Ghost.VY = 0.005 // 向上移动
	state.Ghost.RepelTimer = 0

	UpdateGhostAI(state)

	if state.Ghost.Y > 0.98 {
		t.Fatalf("幽灵 Y 应被弹反至 <= 0.98，got Y=%v", state.Ghost.Y)
	}
	if state.Ghost.VY >= 0 {
		t.Fatalf("弹反后 VY 应为负（向下），got VY=%v", state.Ghost.VY)
	}
}

func TestUpdateGhostAI_Offscreen(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 1.2
	state.Ghost.Y = 0.5
	state.Ghost.VX = 0.01
	state.Ghost.VY = 0
	state.Ghost.RepelTimer = 0

	UpdateGhostAI(state)

	if state.Ghost.Active {
		t.Fatal("幽灵离开屏幕时应被销毁")
	}
	if state.Ghost.SpawnTimer <= 0 {
		t.Fatal("销毁后 SpawnTimer 应为正值")
	}
}

func TestUpdateGhostAI_Repel(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.VX = 0
	state.Ghost.VY = 0
	state.Ghost.RepelTimer = 10
	state.Balloon.X = 0.6
	state.Balloon.Y = 0.5

	UpdateGhostAI(state)

	if state.Ghost.VX >= 0 {
		t.Fatalf("被驱离时幽灵应远离气球（VX 应为负），got VX=%v", state.Ghost.VX)
	}
}

func TestUpdateGhostAI_Attract(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.4
	state.Ghost.Y = 0.5
	state.Ghost.VX = 0
	state.Ghost.VY = 0
	state.Ghost.RepelTimer = 0
	state.Balloon.X = 0.5
	state.Balloon.Y = 0.5
	state.TickCount = 1

	UpdateGhostAI(state)

	if state.Ghost.VX <= 0 {
		t.Fatalf("吸引半径内幽灵应朝气球加速（VX 应为正），got VX=%v", state.Ghost.VX)
	}
}

func TestUpdateGhostAI_SpawnWhenInactive(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false
	state.Ghost.SpawnTimer = 0

	UpdateGhostAI(state)

	if !state.Ghost.Active {
		t.Fatal("未激活且倒计时到 0 时应生成新幽灵")
	}
	if state.Ghost.X < 0.15 || state.Ghost.X > 0.85 {
		t.Fatalf("新生成的幽灵 X 应在 0.15-0.85，got X=%v", state.Ghost.X)
	}
}

func TestUpdateGhostAI_CountdownWhenInactive(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false
	state.Ghost.SpawnTimer = 10

	UpdateGhostAI(state)

	if state.Ghost.Active {
		t.Fatal("倒计时未到不应生成幽灵")
	}
	if state.Ghost.SpawnTimer != 9 {
		t.Fatalf("倒计时应递减，got SpawnTimer=%v, want=9", state.Ghost.SpawnTimer)
	}
}

// ─── 鸟 AI ───────────────────────────────────────────────────────────

func TestUpdateBirdAI_Spawn(t *testing.T) {
	state := createTestState()
	state.Bird.Active = false
	state.Bird.SpawnTimer = 1
	state.Balloon.X = 0.5
	state.Balloon.Y = 0.5

	UpdateBirdAI(&state.Bird, &state.Balloon, 0)

	if !state.Bird.Active {
		t.Fatal("倒计时到 0 时鸟应激活")
	}
}

func TestUpdateBirdAI_Move(t *testing.T) {
	state := createTestState()
	state.Bird.Active = true
	state.Bird.X = 0.5
	state.Bird.Y = 0.5
	state.Bird.VX = 0.01
	state.Bird.VY = 0
	xBefore := state.Bird.X

	UpdateBirdAI(&state.Bird, &state.Balloon, 1)

	if state.Bird.X == xBefore {
		t.Fatal("激活的鸟应移动")
	}
}

func TestUpdateBirdAI_Offscreen(t *testing.T) {
	state := createTestState()
	state.Bird.Active = true
	state.Bird.X = 1.2
	state.Bird.Y = 0.5
	state.Bird.VX = 0.01
	state.Bird.VY = 0

	UpdateBirdAI(&state.Bird, &state.Balloon, 1)

	if state.Bird.Active {
		t.Fatal("鸟离开屏幕时应被销毁")
	}
	if state.Bird.SpawnTimer <= 0 {
		t.Fatal("销毁后 SpawnTimer 应为正值")
	}
}

func TestUpdateBirdAI_Recalibrate(t *testing.T) {
	state := createTestState()
	state.Bird.Active = true
	state.Bird.X = 0.3
	state.Bird.Y = 0.5
	state.Bird.VX = 0
	state.Bird.VY = 0
	state.Balloon.X = 0.8
	state.Balloon.Y = 0.5

	UpdateBirdAI(&state.Bird, &state.Balloon, 30)

	if state.Bird.VX <= 0 {
		t.Fatalf("每 30 ticks 重新校准方向，鸟应朝气球加速（VX 应为正），got VX=%v", state.Bird.VX)
	}
}

// ─── 幽灵驱离 ────────────────────────────────────────────────────────

func TestApplyGhostRepel_InRange(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.RepelTimer = 0

	ApplyGhostRepel(state, 0.5, 0.5)

	if state.Ghost.RepelTimer != protocol.GhostRepelDuration {
		t.Fatalf("驱离半径内应设置 RepelTimer=GHOST_REPEL_DURATION=%v，got=%v",
			protocol.GhostRepelDuration, state.Ghost.RepelTimer)
	}
}

func TestApplyGhostRepel_OutOfRange(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.RepelTimer = 0

	ApplyGhostRepel(state, 0.9, 0.9)

	if state.Ghost.RepelTimer != 0 {
		t.Fatalf("驱离半径外 RepelTimer 应保持 0，got=%v", state.Ghost.RepelTimer)
	}
}

func TestApplyGhostRepel_Inactive(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false

	ApplyGhostRepel(state, 0.5, 0.5)

	if state.Ghost.RepelTimer != 0 {
		t.Fatalf("幽灵未激活时驱离无效，RepelTimer 应为 0，got=%v", state.Ghost.RepelTimer)
	}
}

// ─── 风场 ────────────────────────────────────────────────────────────

func TestUpdateWind_UpdatesCountdowns(t *testing.T) {
	state := createTestState()
	state.WindMicroCountdown = 1
	state.WindMidCountdown = 1
	state.WindChangeCountdown = 1

	UpdateWind(state)

	if state.WindMicroCountdown != protocol.WindMicroInterval {
		t.Fatalf("WindMicroCountdown = %d, want %d", state.WindMicroCountdown, protocol.WindMicroInterval)
	}
	if state.WindMidCountdown != protocol.WindMidInterval {
		t.Fatalf("WindMidCountdown = %d, want %d", state.WindMidCountdown, protocol.WindMidInterval)
	}
	if state.WindChangeCountdown <= 0 {
		t.Fatal("WindChangeCountdown should be reset")
	}
}

func TestUpdateWind_Clamp(t *testing.T) {
	state := createTestState()
	state.Wind = protocol.WindClamp * 2
	state.WindTarget = protocol.WindClamp * 2
	state.WindMidOffset = protocol.WindClamp

	for i := 0; i < 30; i++ {
		UpdateWind(state)
	}

	if state.Wind > protocol.WindClamp {
		t.Fatalf("Wind = %v, want <= %v", state.Wind, protocol.WindClamp)
	}
}

func TestUpdateWind_EdgeSoftZone(t *testing.T) {
	state := createTestState()
	state.Balloon.X = protocol.WindEdgeSoftZone / 2
	state.Balloon.VX = 0
	state.Wind = 1.0
	state.WindTarget = 1.0
	state.WindMidOffset = 0

	UpdateWind(state)

	edgeState := createTestState()
	edgeState.Balloon.X = 0.5
	edgeState.Balloon.VX = 0
	edgeState.Wind = 1.0
	edgeState.WindTarget = 1.0
	edgeState.WindMidOffset = 0
	UpdateWind(edgeState)

	if math.Abs(state.Balloon.VX) >= math.Abs(edgeState.Balloon.VX) {
		t.Fatalf("edge wind effect should be weaker: edge VX=%v, center VX=%v", state.Balloon.VX, edgeState.Balloon.VX)
	}
}

func TestCalculateCooldown(t *testing.T) {
	// playerCount=1: cooldown = 1000 + 2032*log2(1) = 1000
	result1 := CalculateCooldown(1)
	if result1 != int64(protocol.CooldownBaseMs) {
		t.Errorf("playerCount=1: got %d, want %d", result1, protocol.CooldownBaseMs)
	}

	// playerCount=2: cooldown = 1000 + 2032*log2(2) = 3032
	result2 := CalculateCooldown(2)
	expected2 := int64(math.Round(float64(protocol.CooldownBaseMs) + float64(protocol.CooldownLogCoeff)*math.Log2(2)))
	if result2 != expected2 {
		t.Errorf("playerCount=2: got %d, want %d", result2, expected2)
	}

	// 结果不应超过上限
	result100 := CalculateCooldown(100)
	if result100 > int64(protocol.CooldownMaxMs) {
		t.Errorf("playerCount=100: 冷却时间不应超过 %d，got %d", protocol.CooldownMaxMs, result100)
	}

	// 极大 playerCount 应被上限截断
	resultBig := CalculateCooldown(10000)
	if resultBig != int64(protocol.CooldownMaxMs) {
		t.Errorf("playerCount=10000: 应达到上限 %d，got %d", protocol.CooldownMaxMs, resultBig)
	}
}

// ─── 房间码 ──────────────────────────────────────────────────────────

func TestGenerateRoomCode(t *testing.T) {
	validChars := regexp.MustCompile(`^[A-HJ-NP-Z2-9]+$`)

	for i := 0; i < 50; i++ {
		code := GenerateRoomCode()
		if len(code) != 5 {
			t.Fatalf("房间码应为 5 字符，got len=%d, code=%s", len(code), code)
		}
		if !validChars.MatchString(code) {
			t.Fatalf("房间码应只包含大写字母（无 I/O）和数字（无 0/1），got=%s", code)
		}
	}
}

// ─── Benchmarks ──────────────────────────────────────────────────────

func BenchmarkApplyPhysics(b *testing.B) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0.01, VY: 0.01}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balloon.Y = 0.5
		balloon.VY = 0.01
		ApplyPhysics(&balloon)
	}
}

func BenchmarkApplyTapForce(b *testing.B) {
	balloon := domain.BalloonState{X: 0.5, Y: 0.5, VX: 0, VY: 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balloon.VX = 0
		balloon.VY = 0
		ApplyTapForce(&balloon, 0.5, 0.3)
	}
}

func BenchmarkUpdateGhostAI(b *testing.B) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	state.Ghost.VX = protocol.GhostSpeed
	state.Ghost.VY = 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		UpdateGhostAI(state)
	}
}

func BenchmarkUpdateBirdAI(b *testing.B) {
	state := createTestState()
	state.Bird.Active = true
	state.Bird.X = 0.3
	state.Bird.Y = 0.5
	state.Bird.VX = protocol.BirdSpeed
	state.Bird.VY = 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		UpdateBirdAI(&state.Bird, &state.Balloon, state.TickCount)
		state.TickCount++
	}
}

func BenchmarkCheckGhostCollision(b *testing.B) {
	state := createTestState()
	state.Ghost.Active = true
	state.Ghost.X = 0.5
	state.Ghost.Y = 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CheckGhostCollision(state)
	}
}

func BenchmarkCheckBirdCollision(b *testing.B) {
	bird := domain.BirdState{X: 0.5, Y: 0.5, Active: true}
	balloon := domain.BalloonState{X: 0.5, Y: 0.5}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CheckBirdCollision(&bird, &balloon)
	}
}

func BenchmarkCalculateCooldown(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateCooldown(10)
	}
}

func BenchmarkGenerateRoomCode(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GenerateRoomCode()
	}
}
