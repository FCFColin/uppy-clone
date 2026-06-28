package game

import "github.com/uppy-clone/backend/internal/protocol"

func reconnectGraceExpired(disconnectedAt, now int64) bool {
	return now-disconnectedAt > protocol.ReconnectGraceMs
}
