//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/uppy-clone/backend/internal/testutil"
)

func TestOutbox_InsertEvent(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

	payload := []byte(`{"event":"test","value":42}`)
	if err := db.InsertOutboxEvent(ctx, "room", "room-abc", payload); err != nil {
		t.Fatalf("InsertOutboxEvent: %v", err)
	}
}

func TestOutbox_InsertMultipleEvents(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

	events := []struct {
		aggregateType string
		aggregateID   string
		payload       []byte
	}{
		{"room", "room-1", []byte(`{"type":"created"}`)},
		{"room", "room-1", []byte(`{"type":"player_joined"}`)},
		{"room", "room-2", []byte(`{"type":"created"}`)},
		{"user", "user-1", []byte(`{"type":"registered"}`)},
	}

	for _, ev := range events {
		if err := db.InsertOutboxEvent(ctx, ev.aggregateType, ev.aggregateID, ev.payload); err != nil {
			t.Fatalf("InsertOutboxEvent(%s, %s): %v", ev.aggregateType, ev.aggregateID, err)
		}
	}
}

func TestOutbox_InsertEmptyPayload(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

	if err := db.InsertOutboxEvent(ctx, "room", "room-empty", []byte(`{}`)); err != nil {
		t.Fatalf("InsertOutboxEvent with empty payload: %v", err)
	}
}

func TestOutbox_InsertLargePayload(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

	data := make([]byte, 5000)
	for i := range data {
		data[i] = 'x'
	}
	payload := append(append([]byte(`{"data":"`), data...), []byte(`"}`)...)

	if err := db.InsertOutboxEvent(ctx, "room", "room-large", payload); err != nil {
		t.Fatalf("InsertOutboxEvent with large payload: %v", err)
	}
}

func TestOutbox_InsertSpecialChars(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

	payload := []byte(`{"name":"test's data with special chars: \"'\n\t"}`)
	if err := db.InsertOutboxEvent(ctx, "room", "room-special", payload); err != nil {
		t.Fatalf("InsertOutboxEvent with special chars: %v", err)
	}
}

func TestOutbox_InsertLongAggregateID(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

	longID := ""
	for i := 0; i < 255; i++ {
		longID += "a"
	}

	if err := db.InsertOutboxEvent(ctx, "room", longID, []byte(`{"event":"test"}`)); err != nil {
		t.Fatalf("InsertOutboxEvent with long aggregateID: %v", err)
	}
}

func TestOutbox_ConcurrentInserts(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

	done := make(chan error, 10)
	for i := 0; i < 5; i++ {
		go func(idx int) {
			payload := []byte(`{"concurrent":true,"index":` + string(rune('0'+idx)) + `}`)
			done <- db.InsertOutboxEvent(ctx, "room", "room-concurrent", payload)
		}(i)
	}

	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent InsertOutboxEvent %d: %v", i, err)
		}
	}
}
