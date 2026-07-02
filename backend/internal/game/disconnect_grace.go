package game

import "github.com/uppy-clone/backend/internal/domain"

func reconnectGraceExpired(disconnectedAt, now int64) bool {
	return now-disconnectedAt > domain.ReconnectGraceMs
}
