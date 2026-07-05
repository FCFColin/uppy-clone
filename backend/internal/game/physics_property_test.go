package game

import (
	"math"
	"testing"
	"testing/quick"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
)

func TestApplyPhysics_GravityAlwaysDownward(t *testing.T) {
	f := func(x, y, vx, vy float64) bool {
		if y <= 0 || y >= 1 || math.Abs(vy) > 1e6 {
			return true
		}
		b := &domain.BalloonState{X: x, Y: y, VX: vx, VY: vy}
		before := b.VY
		ApplyPhysics(b)
		return b.VY < before
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestApplyPhysics_PositionInBounds(t *testing.T) {
	f := func(x, y, vx, vy float64) bool {
		if math.Abs(x) > 1e6 || math.Abs(y) > 1e6 || math.Abs(vx) > 1e6 || math.Abs(vy) > 1e6 {
			return true
		}
		b := &domain.BalloonState{X: x, Y: y, VX: vx, VY: vy}
		ApplyPhysics(b)
		return b.X >= 0 && b.X <= 1 && b.Y >= 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestApplyPhysics_GameOverConsistent(t *testing.T) {
	f := func(x, y, vx, vy float64) bool {
		b := &domain.BalloonState{X: x, Y: y, VX: vx, VY: vy}
		gameOver := ApplyPhysics(b)
		if math.IsNaN(b.Y) || math.IsInf(b.Y, 0) {
			return true
		}
		return gameOver == (b.Y <= 0)
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestApplyTapForce_DirectionMatch(t *testing.T) {
	f := func(balloonX, balloonY, tapX, tapY float64) bool {
		if balloonY <= 0 || balloonY >= 1 || tapY <= 0 || tapY >= 1 {
			return true
		}
		dx := balloonX - tapX
		dy := balloonY - tapY
		if math.IsNaN(dx) || math.IsNaN(dy) || (dx == 0 && dy == 0) {
			return true
		}
		b := &domain.BalloonState{X: balloonX, Y: balloonY, VX: 0, VY: 0}
		ok := ApplyTapForce(b, tapX, tapY)
		if !ok {
			return true
		}
		sameDirX := math.Signbit(b.VX) == math.Signbit(dx)
		sameDirY := math.Signbit(b.VY) == math.Signbit(dy)
		return sameDirX && sameDirY
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestApplyTapForce_VelocityChangesOnApply(t *testing.T) {
	f := func(balloonX, balloonY, tapX, tapY float64) bool {
		b := &domain.BalloonState{X: balloonX, Y: balloonY, VX: 0, VY: 0}
		ok := ApplyTapForce(b, tapX, tapY)
		if !ok {
			return true
		}
		return b.VX != 0 || b.VY != 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestUpdateWind_Bounds(t *testing.T) {
	f := func(seed int64) bool {
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", rng)
		for i := 0; i < 1000; i++ {
			UpdateWind(state, rng)
		}
		return state.Wind >= -protocol.WindClamp && state.Wind <= protocol.WindClamp
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestInitialWind_Deterministic(t *testing.T) {
	f := func(seed int64) bool {
		rng1 := newSeededRNG(seed)
		rng2 := newSeededRNG(seed)
		w1, t1 := initialWind(rng1)
		w2, t2 := initialWind(rng2)
		return w1 == w2 && t1 == t2
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestInitialWind_ValidRange(t *testing.T) {
	f := func(seed int64) bool {
		rng := newSeededRNG(seed)
		wind, _ := initialWind(rng)
		return wind >= -protocol.WindClamp && wind <= protocol.WindClamp
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
