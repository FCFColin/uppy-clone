// Package testutil provides shared test helpers for backend tests.
package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/pashagolub/pgxmock/v4"
)

// NewPgxMock creates a pgxmock pool registered for t.Cleanup.
//
// RO-049 (aggressive-slim-and-boost-coverage): consolidates the repeated
// `mock, err := pgxmock.NewPool(); t.Fatalf(...); t.Cleanup(mock.Close)`
// pattern that appeared 60+ times across backend unit tests.
//
// pgxmock is the UNIT test double for pgxpool — see postgres.go for the
// pgxmock vs testcontainers boundary (do NOT merge them).
func NewPgxMock(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return mock
}

// NewWSTestUpgraderServer starts an httptest.Server whose handler accepts
// any WebSocket upgrade request using a default websocket.Upgrader. The
// returned server is registered for t.Cleanup.
//
// The handler holds each upgraded connection open until the caller (or test
// teardown) closes it. This is the minimum viable upgrader used by game
// package tests that need a real WS server endpoint to dial.
//
// RO-049: consolidates the repeated `upgrader := websocket.Upgrader{};
// server := httptest.NewServer(...)` pattern that appeared 15+ times in
// internal/game/*_test.go.
func NewWSTestUpgraderServer(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(server.Close)
	return server
}

// DecodeJSONBody decodes the JSON body of an httptest.ResponseRecorder into v.
// On decode error it fails the test with a descriptive message including the
// raw body — replacing the verbose `json.NewDecoder(w.Body).Decode` +
// `t.Fatalf` pattern repeated across handler tests.
func DecodeJSONBody(t *testing.T, w *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("decode response body: %v (body=%q)", err, w.Body.String())
	}
}
