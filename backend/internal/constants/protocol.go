package constants

// Client message types (browser → server)
const (
	MsgTap         = 0x10
	MsgSetNickname = 0x11
	MsgRestartVote = 0x12
	MsgPing        = 0x20
)

// Server message types (server → browser)
const (
	MsgSnapshot        = 0x01
	MsgPlayerJoin      = 0x02
	MsgPlayerLeave     = 0x03
	MsgTapAccepted     = 0x04
	MsgTapRejected     = 0x05
	MsgGameStateChange = 0x06
	MsgRestartStatus   = 0x07
	MsgPong            = 0x21
)
