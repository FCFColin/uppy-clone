package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestInsertOutboxEvent_Success(t *testing.T) {
	_, mock := newMockPostgresStore(t)
	ctx := context.Background()
	payload := []byte(`{"event":"test"}`)

	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs("room", "room-1", payload).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))

	orepo := NewOutboxRepository(mock)
	if err := orepo.InsertOutboxEvent(ctx, "room", "room-1", payload); err != nil {
		t.Fatalf("InsertOutboxEvent: %v", err)
	}
}

func TestInsertOutboxEvent_Error(t *testing.T) {
	_, mock := newMockPostgresStore(t)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO outbox_events").
		WillReturnError(errors.New("insert failed"))

	orepo := NewOutboxRepository(mock)
	if err := orepo.InsertOutboxEvent(ctx, "room", "room-1", []byte(`{}`)); err == nil {
		t.Fatal("expected error")
	}
}