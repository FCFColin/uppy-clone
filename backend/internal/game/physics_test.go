package game

import (
	"math"
	"regexp"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CheckBirdCollision(&c.bird, &c.balloon); got != c.want {
				t.Fatalf("CheckBirdCollision = %v, want %v", got, c.want)
			}
		})
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
	for i := 0; i < 10; i++ {
		state := createTestState()
		state.Ghost.Active = false
		state.Ghost.SpawnTimer = 0

		UpdateGhostAI(state, newSeededRNG(int64(i)))

		if !state.Ghost.Active {
			t.Fatal("未激活且倒计时到 0 时应生成新幽灵")
		}
		if state.Ghost.X != -0.1 && state.Ghost.X != 1.1 {
			t.Fatalf("新生成的幽灵 X 应为边缘值 -0.1 或 1.1，got X=%v", state.Ghost.X)
		}
		if state.Ghost.Y < 0.2 || state.Ghost.Y > 0.8 {
			t.Fatalf("新生成的幽灵 Y 应在 0.2-0.8，got Y=%v", state.Ghost.Y)
		}
		if state.Ghost.X == -0.1 && state.Ghost.VX <= 0 {
			t.Fatalf("从左边缘生成的幽灵 VX 应为正（向右移动），got VX=%v", state.Ghost.VX)
		}
		if state.Ghost.X == 1.1 && state.Ghost.VX >= 0 {
			t.Fatalf("从右边缘生成的幽灵 VX 应为负（向左移动），got VX=%v", state.Ghost.VX)
		}
	}
}

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


// Note: CalculateCooldown is exhaustively tested in cooldown_contract_test.go.

func TestGenerateRoomCode(t *testing.T) {
	validChars := regexp.MustCompile(`^[A-HJ-NP-Z2-9]+$`)

	for i := 0; i < 20; i++ {
		code := GenerateRoomCode(testRNG())
		if len(code) != 5 {
			t.Fatalf("房间码应为 5 字符，got len=%d, code=%s", len(code), code)
		}
		if !validChars.MatchString(code) {
			t.Fatalf("房间码应只包含大写字母（无 I/O）和数字（无 0/1），got=%s", code)
		}
	}
}
