package game

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/domain"
)

// jsonMarshalGameResultFn is replaceable in unit tests.
var jsonMarshalGameResultFn = json.Marshal

// gameEndedOutboxPayloadFn is replaceable in unit tests.
var gameEndedOutboxPayloadFn = auth.GameEndedOutboxPayload

type gameResultJob struct {
	sessionID string
	roomCode  string
	payload   []byte
	outbox    []byte
}

// enqueueGameResultAsync fires game result + outbox insert without blocking the caller.
func (r *Room) enqueueGameResultAsync() {
	if r.state.SessionID == "" {
		return
	}

	endedAt := time.Now().UnixMilli()
	results := make([]domain.GameResultPlayer, 0, len(r.state.Players))
	for _, p := range r.state.Players {
		results = append(results, domain.GameResultPlayer{
			UserID:            p.ID,
			ScoreContribution: p.ScoreContribution,
			TapsCount:         p.TapsCount,
		})
	}

	finalScore := r.state.Balloon.Score
	sessionID := r.state.SessionID
	roomCode := r.state.LobbyCode

	// 直写 PostgreSQL 作为主路径，不依赖 Redis 队列。
	if r.store != nil {
		r.asyncWg.Add(1)
		go func() {
			defer r.asyncWg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
			defer cancel()
			if err := r.store.RecordGameResult(ctx, sessionID, roomCode, endedAt, finalScore, results); err != nil {
				slog.Error("direct record game result failed", "error", err, "session_id", sessionID)
			}
		}()
	}

	// 同时入队 Redis（供 worker 批量处理/对账），非主路径。
	if r.hub != nil && r.hub.cache != nil {
		payload := map[string]interface{}{
			"game_id":     sessionID,
			"room_code":   roomCode,
			"final_score": finalScore,
			"results":     resultsToMap(results),
			"ended_at":    endedAt,
		}
		payloadJSON, err := jsonMarshalGameResultFn(payload)
		if err != nil {
			r.logger.Error("marshal game result payload", "error", err)
			return
		}

		var outboxPayload []byte
		if r.store != nil {
			outboxPayload, err = gameEndedOutboxPayloadFn(payload)
			if err != nil {
				r.logger.Error("marshal game ended outbox payload", "error", err)
			}
		}

		job := gameResultJob{
			sessionID: sessionID,
			roomCode:  roomCode,
			payload:   payloadJSON,
			outbox:    outboxPayload,
		}
		r.asyncWg.Add(1)
		go func() {
			defer r.asyncWg.Done()
			r.runGameResultJob(job)
		}()
	}
}

func resultsToMap(results []domain.GameResultPlayer) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]interface{}{
			"user_id":            r.UserID,
			"score_contribution": r.ScoreContribution,
			"taps_count":         r.TapsCount,
		})
	}
	return out
}

func (r *Room) runGameResultJob(job gameResultJob) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
	defer cancel()

	if err := r.hub.cache.EnqueueGameResult(ctx, job.payload); err != nil {
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
	r.asyncWg.Add(1)
	go func() {
		defer r.asyncWg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
		defer cancel()
		if err := r.store.CreateGameSession(ctx, session); err != nil {
			r.logger.Warn("create game session failed, will retry",
				"error", err,
				"room_code", session.LobbyCode)
		}
	}()
}
