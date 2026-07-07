package game

import (
	"math"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/protocol"
	"pgregory.net/rapid"
)

func TestPhysics_ApplyPhysicsGravityAlwaysDownward(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x := rapid.Float64().Draw(t, "x")
		y := rapid.Float64().Draw(t, "y")
		vx := rapid.Float64().Draw(t, "vx")
		vy := rapid.Float64().Draw(t, "vy")
		if y <= 0 || y >= 1 || math.Abs(vy) > 1e6 {
			return
		}
		b := &domain.BalloonState{X: x, Y: y, VX: vx, VY: vy}
		before := b.VY
		ApplyPhysics(b)
		if b.VY >= before {
			t.Fatal("gravity did not pull downward")
		}
	})
}

func TestPhysics_ApplyPhysicsPositionInBounds(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x := rapid.Float64().Draw(t, "x")
		y := rapid.Float64().Draw(t, "y")
		vx := rapid.Float64().Draw(t, "vx")
		vy := rapid.Float64().Draw(t, "vy")
		if math.Abs(x) > 1e6 || math.Abs(y) > 1e6 || math.Abs(vx) > 1e6 || math.Abs(vy) > 1e6 {
			return
		}
		b := &domain.BalloonState{X: x, Y: y, VX: vx, VY: vy}
		gameOver := ApplyPhysics(b)
		if b.X < 0 || b.X > 1 {
			t.Fatal("X out of bounds")
		}
		if !gameOver && b.Y < 0 {
			t.Fatal("Y went negative without game over")
		}
	})
}

func TestPhysics_ApplyPhysicsGameOverConsistent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x := rapid.Float64().Draw(t, "x")
		y := rapid.Float64().Draw(t, "y")
		vx := rapid.Float64().Draw(t, "vx")
		vy := rapid.Float64().Draw(t, "vy")
		b := &domain.BalloonState{X: x, Y: y, VX: vx, VY: vy}
		gameOver := ApplyPhysics(b)
		if math.IsNaN(b.Y) || math.IsInf(b.Y, 0) {
			return
		}
		if gameOver != (b.Y <= 0) {
			t.Fatal("game over inconsistent with Y position")
		}
	})
}

func TestPhysics_ApplyTapForceDirectionMatch(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		balloonX := rapid.Float64().Draw(t, "balloonX")
		balloonY := rapid.Float64().Draw(t, "balloonY")
		tapX := rapid.Float64().Draw(t, "tapX")
		tapY := rapid.Float64().Draw(t, "tapY")
		if balloonY <= 0 || balloonY >= 1 || tapY <= 0 || tapY >= 1 {
			return
		}
		dx := balloonX - tapX
		dy := balloonY - tapY
		if math.IsNaN(dx) || math.IsNaN(dy) || (dx == 0 && dy == 0) {
			return
		}
		b := &domain.BalloonState{X: balloonX, Y: balloonY, VX: 0, VY: 0}
		ok := ApplyTapForce(b, tapX, tapY)
		if !ok {
			return
		}
		sameDirX := math.Signbit(b.VX) == math.Signbit(dx)
		sameDirY := math.Signbit(b.VY) == math.Signbit(dy)
		if !sameDirX || !sameDirY {
			t.Fatal("tap force direction does not match")
		}
	})
}

func TestPhysics_ApplyTapForceVelocityChangesOnApply(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		balloonX := rapid.Float64().Draw(t, "balloonX")
		balloonY := rapid.Float64().Draw(t, "balloonY")
		tapX := rapid.Float64().Draw(t, "tapX")
		tapY := rapid.Float64().Draw(t, "tapY")
		b := &domain.BalloonState{X: balloonX, Y: balloonY, VX: 0, VY: 0}
		ok := ApplyTapForce(b, tapX, tapY)
		if !ok {
			return
		}
		if b.VX == 0 && b.VY == 0 {
			t.Fatal("velocity unchanged after tap")
		}
	})
}

func TestPhysics_UpdateWindBounds(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Int64().Draw(t, "seed")
		rng := newSeededRNG(seed)
		state := NewGameState("PROP", rng)
		for i := 0; i < 1000; i++ {
			UpdateWind(state, rng)
		}
		if state.Wind < -protocol.WindClamp || state.Wind > protocol.WindClamp {
			t.Fatal("wind out of bounds")
		}
	})
}

func TestPhysics_InitialWindDeterministic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Int64().Draw(t, "seed")
		rng1 := newSeededRNG(seed)
		rng2 := newSeededRNG(seed)
		w1, t1 := initialWind(rng1)
		w2, t2 := initialWind(rng2)
		if w1 != w2 || t1 != t2 {
			t.Fatal("initial wind not deterministic")
		}
	})
}

func TestPhysics_InitialWindValidRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Int64().Draw(t, "seed")
		rng := newSeededRNG(seed)
		wind, _ := initialWind(rng)
		if wind < -protocol.WindClamp || wind > protocol.WindClamp {
			t.Fatal("initial wind out of valid range")
		}
	})
}
