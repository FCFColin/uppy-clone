package game

import (
	"math"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/metrics"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/validate"
)

func (r *Room) handleTap(player *domain.PlayerState, playerID string, payload []byte) {
	now := time.Now().UnixMilli()

	if !r.validateTapRequest(player, now) {
		r.sendToPlayer(playerID, protocol.EncodeTapRejected())
		return
	}
	tapX, tapY, ok := r.decodeTapPayload(payload)
	if !ok || !r.applyTapPhysics(float64(tapX), float64(tapY)) {
		r.sendToPlayer(playerID, protocol.EncodeTapRejected())
		return
	}

	cooldown := r.updatePlayerStats(player, now)
	r.broadcastTapResult(player, cooldown)
}

func (r *Room) validateTapRequest(player *domain.PlayerState, now int64) bool {
	if r.state.Phase != domain.PhasePlaying {
		return false
	}
	if !player.CanTap(now) {
		return false
	}
	return true
}

func (r *Room) decodeTapPayload(payload []byte) (float32, float32, bool) {
	if len(payload) < 8 {
		return 0, 0, false
	}
	tapX, tapY, _ := protocol.DecodeTap(payload)
	if math.IsNaN(float64(tapX)) || math.IsNaN(float64(tapY)) ||
		math.IsInf(float64(tapX), 0) || math.IsInf(float64(tapY), 0) ||
		float64(tapX) < 0 || float64(tapX) > 1 || float64(tapY) < 0 || float64(tapY) > 1 {
		return 0, 0, false
	}
	return tapX, tapY, true
}

func (r *Room) applyTapPhysics(tapX, tapY float64) bool {
	if !ApplyTapForce(&r.state.Balloon, tapX, tapY) {
		return false
	}
	ApplyGhostRepel(r.state, tapX, tapY)
	return true
}

func (r *Room) updatePlayerStats(player *domain.PlayerState, now int64) int64 {
	cooldown := CalculateCooldown(len(r.state.Players))
	player.RecordTap(now, cooldown)
	r.state.Balloon.Score++
	return cooldown
}

func (r *Room) broadcastTapResult(player *domain.PlayerState, cooldown int64) {
	tapMsg := protocol.EncodeTapAccepted(
		uint16(player.PlayerIndex),
		uint32(cooldown),
		float32(r.state.Balloon.X),
		float32(r.state.Balloon.Y),
	)
	r.broadcast(tapMsg, "")
}

func (r *Room) handleSetNicknameMsg(player *domain.PlayerState, payload []byte) {
	nickname, ok := protocol.DecodeNicknamePayload(payload)
	sanitized := ""
	if ok {
		sanitized = validate.Nickname(nickname)
	}
	if sanitized == "" {
		metrics.NicknameConfirmTotal.WithLabelValues("rejected").Inc()
		return
	}

	player.NicknameConfirmed = true
	metrics.NicknameConfirmTotal.WithLabelValues("accepted").Inc()

	if HandleSetNickname(r.state, player, sanitized, r.usedNames) {
		r.requestPersist()
	}
	r.broadcast(r.buildSnapshot(), "")
	r.tryStartWhenAllReady()
}