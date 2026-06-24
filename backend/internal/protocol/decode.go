package protocol

import (
	"math"
)

// DecodeMessage splits a raw binary message into its type byte and payload.
func DecodeMessage(data []byte) (msgType byte, payload []byte) {
	if len(data) == 0 {
		return 0, nil
	}
	return data[0], data[1:]
}

// DecodeTap decodes a tap payload (without msgType prefix) from the client.
//
// Expected layout: tapX(float32) + tapY(float32) = 8 bytes minimum.
// The msgType byte has already been stripped by DecodeMessage, so callers
// pass the payload slice directly.
func DecodeTap(data []byte) (tapX float32, tapY float32, ok bool) {
	if len(data) < 8 {
		return 0, 0, false
	}
	tapX = math.Float32frombits(le.Uint32(data[0:4]))
	tapY = math.Float32frombits(le.Uint32(data[4:8]))
	return tapX, tapY, true
}

// DecodeSetNickname decodes a set-nickname message from the client.
//
// Expected layout: msgType(1) + nickLen(uint8) + nickname(bytes) = 2+ bytes.
func DecodeSetNickname(data []byte) (nickname string, ok bool) {
	if len(data) < 2 {
		return "", false
	}
	if data[0] != MsgSetNickname {
		return "", false
	}
	nickLen := int(data[1])
	if len(data) < 2+nickLen {
		return "", false
	}
	return string(data[2 : 2+nickLen]), true
}

// DecodeRestartVote decodes a restart-vote message from the client.
//
// Expected layout: msgType(1) = 1 byte.
func DecodeRestartVote(data []byte) bool {
	return len(data) >= 1 && data[0] == MsgRestartVote
}

// DecodePing decodes a ping heartbeat message from the client.
//
// Expected layout: msgType(1) = 1 byte.
func DecodePing(data []byte) bool {
	return len(data) >= 1 && data[0] == MsgPing
}

