package protocol

import (
	"bytes"
	"fmt"
	"math"
	"sync"
)

// snapshotBufPool reuses *bytes.Buffer across EncodeSnapshot calls to avoid
// allocating a fresh buffer on every snapshot (15 Hz per room).
var snapshotBufPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// EncodeSnapshot encodes a full game snapshot message.
//
// Binary layout (little-endian):
//
// msgType(1) + tickCount(uint32) + score(uint32) + phaseCode(uint8)
// balloon: x(float32) + y(float32) + vx(float32) + vy(float32)
// bird: active(uint8) + [x(float32)+y(float32) if active]
// ghost: active(uint8) + [x(float32) + y(float32) + repelTimer(uint16) if active]
// playerCount(uint8) + per-player: playerIndex(uint16) + cooldownMs(uint32) + palette(uint32) + scoreContribution(uint32) + nickLen(uint8) + nickname(bytes)
// rippleCount(uint8) + per-ripple: playerIndex(uint16) + x(float32) + y(float32)
// wind(float32)
func EncodeSnapshot(phase GamePhase, tickCount uint32, score uint32, balloon BalloonState, bird BirdState, ghost GhostState, players []PlayerState, ripples []Ripple, wind float64) []byte {
	size := calcSnapshotSize(bird, ghost, players, ripples)

	buf := snapshotBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Grow(size)
	defer snapshotBufPool.Put(buf)

	var b4 [4]byte
	var b2 [2]byte

	buf.WriteByte(MsgSnapshot)
	writeUint32(buf, b4[:], tickCount)
	writeUint32(buf, b4[:], score)
	buf.WriteByte(PhaseToCode(phase))

	encodeBalloon(buf, b4[:], balloon)
	encodeBird(buf, b4[:], bird)
	encodeGhost(buf, b4[:], b2[:], ghost)

	if len(players) > math.MaxUint8 {
		panic(fmt.Sprintf("EncodeSnapshot: players count %d exceeds uint8 limit", len(players)))
	}
	buf.WriteByte(uint8(len(players))) //nolint:gosec // G115: bounded by MaxUint8 check above
	encodePlayers(buf, b2[:], b4[:], players)

	if len(ripples) > math.MaxUint8 {
		panic(fmt.Sprintf("EncodeSnapshot: ripples count %d exceeds uint8 limit", len(ripples)))
	}
	buf.WriteByte(uint8(len(ripples))) //nolint:gosec // G115: bounded by MaxUint8 check above
	encodeRipples(buf, b2[:], b4[:], ripples)

	writeUint32(buf, b4[:], math.Float32bits(float32(wind)))

	// misc-006: Copy out the result because the pool buffer will be reused by subsequent
	// EncodeSnapshot calls. The returned slice outlives this function (it is
	// queued on player Send channels and written later by writePump).
	// The make+copy here is the unavoidable final copy — the sync.Pool already
	// eliminates the buffer allocation. A future optimization could use a
	// reference-counted buffer, but the current approach is sufficient for
	// the expected message volume (~60 snapshots/sec/room).
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result
}

// calcSnapshotSize pre-calculates the total buffer size to avoid reallocations.
func calcSnapshotSize(bird BirdState, ghost GhostState, players []PlayerState, ripples []Ripple) int {
	size := 1 + 4 + 4 + 1 + 16 + 1 // header + balloon + bird active flag
	if bird.Active {
		size += 8
	}
	size += 1 + 1 // ghost flag + playerCount
	if ghost.Active {
		size += 8 + 2 // x + y + repelTimer
	}
	for _, p := range players {
		size += 2 + 4 + 4 + 4 + 1 + len(p.Nickname)
	}
	size++ // rippleCount
	for range ripples {
		size += 2 + 4 + 4
	}
	size += 4 // wind
	return size
}

func writeUint32(buf *bytes.Buffer, b4 []byte, v uint32) {
	le.PutUint32(b4, v)
	buf.Write(b4)
}

func writeUint16(buf *bytes.Buffer, b2 []byte, v uint16) {
	le.PutUint16(b2, v)
	buf.Write(b2)
}

// encodeBalloon writes the balloon state to the buffer.
// Layout: x(float32) + y(float32) + vx(float32) + vy(float32)
func encodeBalloon(buf *bytes.Buffer, b4 []byte, balloon BalloonState) {
	writeUint32(buf, b4, math.Float32bits(balloon.X))
	writeUint32(buf, b4, math.Float32bits(balloon.Y))
	writeUint32(buf, b4, math.Float32bits(balloon.Vx))
	writeUint32(buf, b4, math.Float32bits(balloon.Vy))
}

// encodeBird writes the bird state to the buffer.
func encodeBird(buf *bytes.Buffer, b4 []byte, bird BirdState) {
	if bird.Active {
		buf.WriteByte(1)
		writeUint32(buf, b4, math.Float32bits(bird.X))
		writeUint32(buf, b4, math.Float32bits(bird.Y))
	} else {
		buf.WriteByte(0)
	}
}

// encodeGhost writes the ghost state to the buffer.
func encodeGhost(buf *bytes.Buffer, b4 []byte, b2 []byte, ghost GhostState) {
	if ghost.Active {
		buf.WriteByte(1)
		writeUint32(buf, b4, math.Float32bits(ghost.X))
		writeUint32(buf, b4, math.Float32bits(ghost.Y))
		writeUint16(buf, b2, ghost.RepelTimer)
	} else {
		buf.WriteByte(0)
	}
}

// encodePlayers writes all player states to the buffer.
func encodePlayers(buf *bytes.Buffer, b2 []byte, b4 []byte, players []PlayerState) {
	for _, p := range players {
		writeUint16(buf, b2, p.PlayerIndex)
		writeUint32(buf, b4, p.CooldownMs)
		writeUint32(buf, b4, p.Palette)
		writeUint32(buf, b4, p.ScoreContribution)
		nickBytes := []byte(p.Nickname)
		if len(nickBytes) > math.MaxUint8 {
			panic(fmt.Sprintf("encodePlayers: nickname byte length %d exceeds uint8 limit", len(nickBytes)))
		}
		buf.WriteByte(uint8(len(nickBytes))) //nolint:gosec // G115: bounded by MaxUint8 check above
		buf.Write(nickBytes)
	}
}

// encodeRipples writes all ripple states to the buffer.
func encodeRipples(buf *bytes.Buffer, b2 []byte, b4 []byte, ripples []Ripple) {
	for _, r := range ripples {
		writeUint16(buf, b2, r.PlayerIndex)
		writeUint32(buf, b4, math.Float32bits(r.X))
		writeUint32(buf, b4, math.Float32bits(r.Y))
	}
}

// EncodeTapAccepted encodes a tap-accepted acknowledgement.
//
// Binary layout: msgType(1) + playerIndex(uint16) + cooldownMs(uint32) + x(float32) + y(float32)
func EncodeTapAccepted(playerIndex uint16, cooldownMs uint32, balloonX float32, balloonY float32) []byte {
	buf := new(bytes.Buffer)
	buf.Grow(1 + 2 + 4 + 4 + 4)
	buf.WriteByte(MsgTapAccepted)
	var b2 [2]byte
	le.PutUint16(b2[:], playerIndex)
	buf.Write(b2[:])
	var b4 [4]byte
	le.PutUint32(b4[:], cooldownMs)
	buf.Write(b4[:])
	le.PutUint32(b4[:], math.Float32bits(balloonX))
	buf.Write(b4[:])
	le.PutUint32(b4[:], math.Float32bits(balloonY))
	buf.Write(b4[:])
	return buf.Bytes()
}

// EncodeTapRejected encodes a tap-rejected message.
//
// Binary layout: msgType(1)
func EncodeTapRejected() []byte {
	return []byte{MsgTapRejected}
}

// EncodeGameStateChange encodes a game state change notification.
//
// Binary layout:
//   - msgType(1) + phaseCode(uint8)
//   - when phase=countdown: + countdownRemainingMs(uint32 LE)
//
// For PhaseEnded, use EncodeGameStateChangeEnded to include endReason.
// Variadic countdownRemainingMs is only used when phase=PhaseCountdown.
func EncodeGameStateChange(phase GamePhase, countdownRemainingMs ...uint32) []byte {
	if phase == PhaseCountdown {
		if len(countdownRemainingMs) == 0 {
			panic("EncodeGameStateChange: PhaseCountdown requires countdownRemainingMs argument")
		}
		buf := new(bytes.Buffer)
		buf.Grow(1 + 1 + 4)
		buf.WriteByte(MsgGameStateChange)
		buf.WriteByte(PhaseToCode(phase))
		var b4 [4]byte
		le.PutUint32(b4[:], countdownRemainingMs[0])
		buf.Write(b4[:])
		return buf.Bytes()
	}
	return []byte{MsgGameStateChange, PhaseToCode(phase)}
}

// EncodeGameStateChangeEnded encodes ended phase with death reason.
func EncodeGameStateChangeEnded(endReason uint8) []byte {
	return []byte{MsgGameStateChange, PhaseToCode(PhaseEnded), endReason}
}

// EncodeRestartStatus encodes a restart vote status update.
//
// Binary layout: msgType(1) + yesVotes(uint8) + totalPlayers(uint8) + countdownMs(uint32)
func EncodeRestartStatus(yesVotes uint8, totalPlayers uint8, countdownMs uint32) []byte {
	buf := new(bytes.Buffer)
	buf.Grow(1 + 1 + 1 + 4)
	buf.WriteByte(MsgRestartStatus)
	buf.WriteByte(yesVotes)
	buf.WriteByte(totalPlayers)
	var b4 [4]byte
	le.PutUint32(b4[:], countdownMs)
	buf.Write(b4[:])
	return buf.Bytes()
}

// EncodePlayerJoin encodes a player-join notification.
//
// Binary layout: msgType(1) + playerIndex(uint16) + nickLen(uint8) + nickname(bytes) + palette(uint32)
func EncodePlayerJoin(playerIndex uint16, nickname string, palette uint32) []byte {
	nickBytes := []byte(nickname)
	buf := new(bytes.Buffer)
	buf.Grow(1 + 2 + 1 + len(nickBytes) + 4)
	buf.WriteByte(MsgPlayerJoin)
	var b2 [2]byte
	le.PutUint16(b2[:], playerIndex)
	buf.Write(b2[:])
	if len(nickBytes) > math.MaxUint8 {
		panic(fmt.Sprintf("EncodePlayerJoin: nickname byte length %d exceeds uint8 limit", len(nickBytes)))
	}
	buf.WriteByte(uint8(len(nickBytes))) //nolint:gosec // G115: bounded by MaxUint8 check above
	buf.Write(nickBytes)
	var b4 [4]byte
	le.PutUint32(b4[:], palette)
	buf.Write(b4[:])
	return buf.Bytes()
}

// EncodePlayerLeave encodes a player-leave notification.
//
// Binary layout: msgType(1) + playerIndex(uint16)
func EncodePlayerLeave(playerIndex uint16) []byte {
	buf := new(bytes.Buffer)
	buf.Grow(1 + 2)
	buf.WriteByte(MsgPlayerLeave)
	var b2 [2]byte
	le.PutUint16(b2[:], playerIndex)
	buf.Write(b2[:])
	return buf.Bytes()
}

// EncodePong encodes a pong heartbeat response.
//
// Binary layout: msgType(1)
func EncodePong() []byte {
	return []byte{MsgPong}
}
