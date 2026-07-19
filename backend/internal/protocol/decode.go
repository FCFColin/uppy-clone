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

// WSMessageTypeName maps a protocol message type byte to a human-readable
// label (e.g. for metrics or tracing). It returns "unknown" for
// unrecognized types.
func WSMessageTypeName(msgType byte) string {
	switch msgType {
	case MsgTap:
		return "tap"
	case MsgSetNickname:
		return "set_nickname" //nolint:goconst // protocol message-type label
	case MsgRestartVote:
		return "restart_vote"
	case MsgPing:
		return "ping"
	default:
		return "unknown" //nolint:goconst // protocol message-type label
	}
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

// DecodeNicknamePayload decodes a set-nickname payload (without msgType prefix).
func DecodeNicknamePayload(data []byte) (nickname string, ok bool) {
	if len(data) < 1 {
		return "", false
	}
	nickLen := int(data[0])
	if nickLen <= 0 {
		return "", false
	}
	if len(data) < 1+nickLen {
		return "", false
	}
	return string(data[1 : 1+nickLen]), true
}
