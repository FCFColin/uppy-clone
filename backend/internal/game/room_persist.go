package game

import (
	"context"
	"fmt"
	"time"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/idgen"
)

func newLobbyState(code string, stateJSON []byte) *domain.LobbyState {
	now := time.Now().UnixMilli()
	return &domain.LobbyState{
		ID:        idgen.UUID(),
		Code:      code,
		State:     string(stateJSON),
		UpdatedAt: now,
		CreatedAt: now,
	}
}

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

	ls := newLobbyState(r.state.LobbyCode, data)
	if err := r.store.SaveLobbyState(ctx, ls); err != nil {
		return fmt.Errorf("save lobby state: %w", err)
	}
	return nil
}

// saveState 持久化到 PostgreSQL（异步 debounced）。
func (r *Room) saveState() {
	r.requestPersist()
}
