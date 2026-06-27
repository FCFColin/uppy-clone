package game

import (
	"context"
	"encoding/json"
	"time"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/domain"
)

type gameResultJob struct {
	sessionID string
	roomCode  string
	payload   []byte
	outbox    []byte
}

// enqueueGameResultAsync fires game result + outbox insert without blocking the caller.
func (r *Room) enqueueGameResultAsync() {
	if r.hub == nil || r.hub.redis == nil || r.state.SessionID == "" {
		return
	}

	endedAt := time.Now().UnixMilli()
	results := make([]map[string]interface{}, 0, len(r.state.Players))
	for _, p := range r.state.Players {
		results = append(results, map[string]interface{}{
			"user_id":            p.ID,
			"score_contribution": p.ScoreContribution,
			"taps_count":         p.TapsCount,
		})
	}

	payload := map[string]interface{}{
		"game_id":     r.state.SessionID,
		"room_code":   r.state.LobbyCode,
		"final_score": r.state.Balloon.Score,
		"results":     results,
		"ended_at":    endedAt,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		r.logger.Error("marshal game result payload", "error", err)
		return
	}

	var outboxPayload []byte
	if r.store != nil {
		outboxPayload, err = auth.GameEndedOutboxPayload(payload)
		if err != nil {
			r.logger.Error("marshal game ended outbox payload", "error", err)
		}
	}

	job := gameResultJob{
		sessionID: r.state.SessionID,
		roomCode:  r.state.LobbyCode,
		payload:   payloadJSON,
		outbox:    outboxPayload,
	}

	go r.runGameResultJob(job)
}

func (r *Room) runGameResultJob(job gameResultJob) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
	defer cancel()

	if err := r.hub.redis.EnqueueGameResult(ctx, job.payload); err != nil {
		r.logger.Error("enqueue game result", "error", err)
	}
	if r.store != nil && len(job.outbox) > 0 {
		if err := r.store.InsertOutboxEvent(ctx, "game", job.sessionID, job.outbox); err != nil {
			r.logger.Error("insert game ended outbox event", "error", err)
		}
	}
}

// createGameSessionAsync inserts a game session row without blocking the room lock.
func (r *Room) createGameSessionAsync(session *domain.GameSession) {
	if r.store == nil || session == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
		defer cancel()
		if err := r.store.CreateGameSession(ctx, session); err != nil {
			r.logger.Warn("create game session failed, will retry",
				"error", err,
				"room_code", session.LobbyCode)
		}
	}()
}
