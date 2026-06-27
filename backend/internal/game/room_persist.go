package game

import (
	"context"
	"fmt"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/idgen"
)

// saveStateWithError persists state to PostgreSQL and returns any error.
// P4-6.1: 暴露 error 供 Saga 补偿模式判断是否回滚。
func (r *Room) saveStateWithError() error {
	if r.store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeouts.PGQueryTimeout)
	defer cancel()

	data, err := SerializeState(r.state)
	if err != nil {
		return fmt.Errorf("serialize state: %w", err)
	}

	ls := &domain.LobbyState{
		ID:        idgen.UUID(),
		Code:      r.state.LobbyCode,
		State:     string(data),
		UpdatedAt: time.Now().UnixMilli(),
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := r.store.SaveLobbyState(ctx, ls); err != nil {
		return fmt.Errorf("save lobby state: %w", err)
	}
	return nil
}

// saveState 持久化到 PostgreSQL
func (r *Room) saveState() {
	if err := r.saveStateWithError(); err != nil {
		r.logger.Error("save state", "error", err)
	}
}
