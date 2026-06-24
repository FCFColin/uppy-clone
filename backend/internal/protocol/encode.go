package protocol

import (
	"bytes"
	"encoding/binary"
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
// balloon: x(float32) + y(float32) + vy(float32) + vx(float32)
// bird: active(uint8) + [x(float32)+y(float32) if active]
// ghost: active(uint8) + x(float32) + y(float32) + repelTimer(uint16)
// playerCount(uint8) + per-player: playerIndex(uint16) + cooldownMs(uint32) + palette(uint32) + scoreContribution(uint32) + nickLen(uint8) + nickname(bytes)
// rippleCount(uint8) + per-ripple: playerIndex(uint16) + x(float32) + y(float32)
// wind(float32)
func EncodeSnapshot(phase GamePhase, tickCount uint32, score uint32, balloon BalloonState, bird BirdState, ghost GhostState, players []PlayerState, ripples []Ripple, wind float64) []byte {
	size := calcSnapshotSize(bird, players, ripples)

	buf := snapshotBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Grow(size)
	defer snapshotBufPool.Put(buf)

	var b4 [4]byte
	var b2 [2]byte

	buf.WriteByte(MsgSnapshot)
	le.PutUint32(b4[:], tickCount)
	buf.Write(b4[:])
	le.PutUint32(b4[:], score)
	buf.Write(b4[:])
	buf.WriteByte(PhaseToCode(phase))

	encodeBalloon(buf, b4[:], balloon)
	encodeBird(buf, b4[:], bird)
	encodeGhost(buf, b4[:], b2[:], ghost)

	buf.WriteByte(uint8(len(players))) //nolint:gosec // players count bounded by MaxPlayersPerRoom(50) < 256
	encodePlayers(buf, b2[:], b4[:], players)

	buf.WriteByte(uint8(len(ripples)))
	encodeRipples(buf, b2[:], b4[:], ripples)

	le.PutUint32(b4[:], math.Float32bits(float32(wind)))
	buf.Write(b4[:])

	// Copy out the result because the pool buffer will be reused by subsequent
	// EncodeSnapshot calls. The returned slice outlives this function (it is
	// queued on player Send channels and written later by writePump).
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result
}

// calcSnapshotSize pre-calculates the total buffer size to avoid reallocations.
func calcSnapshotSize(bird BirdState, players []PlayerState, ripples []Ripple) int {
	size := 1 + 4 + 4 + 1 + 16 + 1 // header + balloon + bird active flag
	if bird.Active {
		size += 8
	}
	size += 1 + 8 + 2 + 1 // ghost flag + x + y + repelTimer + playerCount
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

// encodeBalloon writes the balloon state to the buffer.
func encodeBalloon(buf *bytes.Buffer, b4 []byte, balloon BalloonState) {
	le.PutUint32(b4, math.Float32bits(balloon.X))
	buf.Write(b4)
	le.PutUint32(b4, math.Float32bits(balloon.Y))
	buf.Write(b4)
	le.PutUint32(b4, math.Float32bits(balloon.Vy))
	buf.Write(b4)
	le.PutUint32(b4, math.Float32bits(balloon.Vx))
	buf.Write(b4)
}

// encodeBird writes the bird state to the buffer.
func encodeBird(buf *bytes.Buffer, b4 []byte, bird BirdState) {
	if bird.Active {
		buf.WriteByte(1)
		le.PutUint32(b4, math.Float32bits(bird.X))
		buf.Write(b4)
		le.PutUint32(b4, math.Float32bits(bird.Y))
		buf.Write(b4)
	} else {
		buf.WriteByte(0)
	}
}

// encodeGhost writes the ghost state to the buffer.
func encodeGhost(buf *bytes.Buffer, b4 []byte, b2 []byte, ghost GhostState) {
	if ghost.Active {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	le.PutUint32(b4, math.Float32bits(ghost.X))
	buf.Write(b4)
	le.PutUint32(b4, math.Float32bits(ghost.Y))
	buf.Write(b4)
	le.PutUint16(b2, ghost.RepelTimer)
	buf.Write(b2)
}

// encodePlayers writes all player states to the buffer.
func encodePlayers(buf *bytes.Buffer, b2 []byte, b4 []byte, players []PlayerState) {
	for _, p := range players {
		le.PutUint16(b2, p.PlayerIndex)
		buf.Write(b2)
		le.PutUint32(b4, p.CooldownMs)
		buf.Write(b4)
		le.PutUint32(b4, p.Palette)
		buf.Write(b4)
		le.PutUint32(b4, p.ScoreContribution)
		buf.Write(b4)
		nickBytes := []byte(p.Nickname)
		buf.WriteByte(uint8(len(nickBytes)))
		buf.Write(nickBytes)
	}
}

// encodeRipples writes all ripple states to the buffer.
func encodeRipples(buf *bytes.Buffer, b2 []byte, b4 []byte, ripples []Ripple) {
	for _, r := range ripples {
		le.PutUint16(b2, r.PlayerIndex)
		buf.Write(b2)
		le.PutUint32(b4, math.Float32bits(r.X))
		buf.Write(b4)
		le.PutUint32(b4, math.Float32bits(r.Y))
		buf.Write(b4)
	}
}

// EncodeTapAccepted encodes a tap-accepted acknowledgement.
//
// Binary layout: msgType(1) + playerIndex(uint16) + cooldownMs(uint32) + x(float32) + y(float32)
func EncodeTapAccepted(playerIndex uint16, cooldownMs uint32, balloonX float32, balloonY float32) []byte {
	var buf bytes.Buffer
	buf.WriteByte(MsgTapAccepted)
	_ = binary.Write(&buf, le, playerIndex)
	_ = binary.Write(&buf, le, cooldownMs)
	_ = binary.Write(&buf, le, balloonX)
	_ = binary.Write(&buf, le, balloonY)
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
// Binary layout: msgType(1) + phaseCode(uint8)
func EncodeGameStateChange(phase GamePhase) []byte {
	return []byte{MsgGameStateChange, PhaseToCode(phase)}
}

// EncodeRestartStatus encodes a restart vote status update.
//
// Binary layout: msgType(1) + yesVotes(uint8) + totalPlayers(uint8) + countdownMs(uint32)
func EncodeRestartStatus(yesVotes uint8, totalPlayers uint8, countdownMs uint32) []byte {
	var buf bytes.Buffer
	buf.WriteByte(MsgRestartStatus)
	buf.WriteByte(yesVotes)
	buf.WriteByte(totalPlayers)
	_ = binary.Write(&buf, le, countdownMs)
	return buf.Bytes()
}

// EncodePlayerJoin encodes a player-join notification.
//
// Binary layout: msgType(1) + playerIndex(uint16) + nickLen(uint8) + nickname(bytes) + palette(uint32)
func EncodePlayerJoin(playerIndex uint16, nickname string, palette uint32) []byte {
	nickBytes := []byte(nickname)
	var buf bytes.Buffer
	buf.WriteByte(MsgPlayerJoin)
	_ = binary.Write(&buf, le, playerIndex)
	buf.WriteByte(uint8(len(nickBytes))) //nolint:gosec // nickname length bounded by MaxNicknameLen
	buf.Write(nickBytes)
	_ = binary.Write(&buf, le, palette)
	return buf.Bytes()
}

// EncodePlayerLeave encodes a player-leave notification.
//
// Binary layout: msgType(1) + playerIndex(uint16)
func EncodePlayerLeave(playerIndex uint16) []byte {
	var buf bytes.Buffer
	buf.WriteByte(MsgPlayerLeave)
	_ = binary.Write(&buf, le, playerIndex)
	return buf.Bytes()
}

// EncodePong encodes a pong heartbeat response.
//
// Binary layout: msgType(1)
func EncodePong() []byte {
	return []byte{MsgPong}
}

