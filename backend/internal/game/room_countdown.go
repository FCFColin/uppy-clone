package game

import (
	"time"

	"github.com/uppy-clone/backend/internal/protocol"
)

func countdownDurationMs() int64 {
	return int64(protocol.CountdownTicks) * 1000 / int64(protocol.TickRate)
}

func countdownDurationMsU32() uint32 {
	return uint32(protocol.CountdownTicks) * 1000 / uint32(protocol.TickRate)
}

func (r *Room) scheduleCountdownFromNow() {
	ms := countdownDurationMs()
	r.setEndGameAlarm(time.Now().Add(time.Duration(ms) * time.Millisecond))
}

func (r *Room) broadcastCountdownPhase() {
	msU32 := countdownDurationMsU32()
	r.broadcastCritical(protocol.EncodeGameStateChange(protocol.PhaseCountdown, msU32))
	r.broadcast(r.buildSnapshot(), "")
}

func remainingCountdownMs(countdownStart int64) int64 {
	elapsed := time.Now().UnixMilli() - countdownStart
	remaining := countdownDurationMs() - elapsed
	if remaining < 100 {
		return 100
	}
	return remaining
}
