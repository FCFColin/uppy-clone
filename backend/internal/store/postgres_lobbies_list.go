package store

import (
	"context"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LobbyListResult contains paginated lobby results with metadata.
type LobbyListResult struct {
	Lobbies    []domain.LobbyState
	Total      int
	HasMore    bool
	NextCursor string // format: "updated_at|code"
}

// LoadAllActiveLobbies returns lobby states with cursor-based pagination.
// If limit <= 0, defaults to 50. If limit > 100, caps at 100.
func (s *PostgresStore) LoadAllActiveLobbies(ctx context.Context, limit int, cursor string) (*LobbyListResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	spanAttrs := []attribute.KeyValue{
		attribute.String("db.system", "postgresql"),
		attribute.Int("db.limit", limit),
	}
	if cursor != "" {
		spanAttrs = append(spanAttrs, attribute.String("db.cursor", cursor))
	}

	ctx, span := telemetry.Tracer().Start(ctx, "postgres.LoadAllActiveLobbies",
		trace.WithAttributes(spanAttrs...),
	)
	defer span.End()

	cursorUpdatedAt, cursorCode := parseLobbyCursor(cursor)

	total, err := s.countAllLobbies(ctx)
	if err != nil {
		return nil, err
	}

	lobbies, err := s.fetchLobbiesPage(ctx, limit+1, cursorUpdatedAt, cursorCode)
	if err != nil {
		return nil, err
	}

	return buildLobbyListResult(lobbies, total, limit), nil
}
