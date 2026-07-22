package game

import (
	"math"
	"regexp"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

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
	// 鬼 X 方向有效碰撞半径 = GhostCollisionRadiusX + BalloonCollisionRadius = 0.035 + 0.06 = 0.095
	state.Balloon.X = 0.5 + (protocol.GhostCollisionRadiusX+protocol.BalloonCollisionRadius)*0.9
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
	// 鬼 X 方向有效碰撞半径 = GhostCollisionRadiusX + BalloonCollisionRadius = 0.035 + 0.06 = 0.095
	state.Balloon.X = 0.5 + (protocol.GhostCollisionRadiusX+protocol.BalloonCollisionRadius)*2
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
	state.Balloon.VY = 0
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

func TestCheckBirdCollision(t *testing.T) {
	cases := []struct {
		name    string
		bird    domain.BirdState
		balloon domain.BalloonState
		want    bool
	}{
		{"Overlap", domain.BirdState{X: 0.5, Y: 0.5, Active: true}, domain.BalloonState{X: 0.5, Y: 0.5}, true},
		{"Inactive", domain.BirdState{X: 0.5, Y: 0.5, Active: false}, domain.BalloonState{X: 0.5, Y: 0.5}, false},
		{"Far", domain.BirdState{X: 0.1, Y: 0.1, Active: true}, domain.BalloonState{X: 0.9, Y: 0.9}, false},
		// 鸟 X 方向有效碰撞半径 = BirdCollisionRadiusX + BalloonCollisionRadius = 0.020 + 0.06 = 0.080
		{"EdgeXInside", domain.BirdState{X: 0.575, Y: 0.5, Active: true}, domain.BalloonState{X: 0.5, Y: 0.5}, true},
		{"EdgeXOutside", domain.BirdState{X: 0.585, Y: 0.5, Active: true}, domain.BalloonState{X: 0.5, Y: 0.5}, false},
		// 鸟 Y 方向有效碰撞半径 = BirdCollisionRadiusY + BalloonCollisionRadius = 0.035 + 0.06 = 0.095
		{"EdgeYInside", domain.BirdState{X: 0.5, Y: 0.59, Active: true}, domain.BalloonState{X: 0.5, Y: 0.5}, true},
		{"EdgeYOutside", domain.BirdState{X: 0.5, Y: 0.60, Active: true}, domain.BalloonState{X: 0.5, Y: 0.5}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CheckBirdCollision(&c.bird, &c.balloon); got != c.want {
				t.Fatalf("CheckBirdCollision = %v, want %v", got, c.want)
			}
		})
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

	UpdateGhostAI(state, testRNG())

	if state.Ghost.X == xBefore && state.Ghost.Y == yBefore {
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

	UpdateGhostAI(state, testRNG())

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
	state.Ghost.Y = 0.01
	state.Ghost.VX = 0
	state.Ghost.VY = -0.005
	state.Ghost.RepelTimer = 0

	UpdateGhostAI(state, testRNG())

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
	state.Ghost.Y = 0.99
	state.Ghost.VX = 0
	state.Ghost.VY = 0.005
	state.Ghost.RepelTimer = 0

	UpdateGhostAI(state, testRNG())

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

	UpdateGhostAI(state, testRNG())

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

	UpdateGhostAI(state, testRNG())

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

	UpdateGhostAI(state, testRNG())

	if state.Ghost.VX <= 0 {
		t.Fatalf("吸引半径内幽灵应朝气球加速（VX 应为正），got VX=%v", state.Ghost.VX)
	}
}

func TestUpdateGhostAI_SpawnWhenInactive(t *testing.T) {
	state := createTestState()
	state.Ghost.Active = false
	state.Ghost.SpawnTimer = 0

	UpdateGhostAI(state, testRNG())

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

	UpdateGhostAI(state, testRNG())

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

	UpdateBirdAI(&state.Bird, &state.Balloon, 0, testRNG())

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

	UpdateBirdAI(&state.Bird, &state.Balloon, 1, testRNG())

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

	UpdateBirdAI(&state.Bird, &state.Balloon, 1, testRNG())

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

	UpdateBirdAI(&state.Bird, &state.Balloon, 30, testRNG())

	if state.Bird.VX <= 0 {
		t.Fatalf("每 30 ticks 重新校准方向，鸟应朝气球加速（VX 应为正），got VX=%v", state.Bird.VX)
	}
}

// ─── 幽灵驱离 ────────────────────────────────────────────────────────

func TestApplyGhostRepel(t *testing.T) {
	cases := []struct {
		name   string
		active bool
		tapX   float64
		tapY   float64
		want   int
	}{
		{"InRange", true, 0.5, 0.5, int(protocol.GhostRepelDuration)},
		{"OutOfRange", true, 0.9, 0.9, 0},
		{"Inactive", false, 0.5, 0.5, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := createTestState()
			state.Ghost.Active = c.active
			state.Ghost.X = 0.5
			state.Ghost.Y = 0.5
			state.Ghost.RepelTimer = 0
			ApplyGhostRepel(state, c.tapX, c.tapY)
			if state.Ghost.RepelTimer != c.want {
				t.Fatalf("RepelTimer = %v, want %v", state.Ghost.RepelTimer, c.want)
			}
		})
	}
}

// ─── 风场 ────────────────────────────────────────────────────────────

func TestUpdateWind_UpdatesCountdowns(t *testing.T) {
	state := createTestState()
	state.WindMicroCountdown = 1
	state.WindMidCountdown = 1
	state.WindChangeCountdown = 1

	UpdateWind(state, testRNG())

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
		UpdateWind(state, testRNG())
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

	UpdateWind(state, testRNG())

	edgeState := createTestState()
	edgeState.Balloon.X = 0.5
	edgeState.Balloon.VX = 0
	edgeState.Wind = 1.0
	edgeState.WindTarget = 1.0
	edgeState.WindMidOffset = 0
	UpdateWind(edgeState, testRNG())

	if math.Abs(state.Balloon.VX) >= math.Abs(edgeState.Balloon.VX) {
		t.Fatalf("edge wind effect should be weaker: edge VX=%v, center VX=%v", state.Balloon.VX, edgeState.Balloon.VX)
	}
}

// ─── 房间码 ──────────────────────────────────────────────────────────
// Note: CalculateCooldown is exhaustively tested in cooldown_contract_test.go.

func TestGenerateRoomCode(t *testing.T) {
	validChars := regexp.MustCompile(`^[A-HJ-NP-Z2-9]+$`)

	for i := 0; i < 50; i++ {
		code := GenerateRoomCode(testRNG())
		if len(code) != 5 {
			t.Fatalf("房间码应为 5 字符，got len=%d, code=%s", len(code), code)
		}
		if !validChars.MatchString(code) {
			t.Fatalf("房间码应只包含大写字母（无 I/O）和数字（无 0/1），got=%s", code)
		}
	}
}
