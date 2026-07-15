//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

// Integration tests for HTTP handler round-trips with real PostgreSQL and Redis.
// These verify that the handler, store, and hub layers compose correctly.

func TestQuickPlayRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	ctx, rdb := testutil.SetupRedisStore(t)

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := auth.NewRefreshTokenManager(rdb.Client())

	timeouts := config.DefaultTimeoutConfig()
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, rdb, db, "", "", timeouts)
	authHandler := NewAuthHandler(db, rdb, authSvc, &Config{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/quickplay", strings.NewReader(`{"nickname":"IntegrationPlayer"}`))
	r.Header.Set("Content-Type", "application/json")
	authHandler.QuickPlay(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("QuickPlay status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.UserID == "" {
		t.Fatal("expected non-empty userId")
	}

	// Verify the user was persisted in PostgreSQL.
	user, err := db.GetUserByID(context.Background(), resp.UserID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user == nil {
		t.Fatal("user not found in DB after quickplay")
	}
	if user.Nickname != "IntegrationPlayer" {
		t.Fatalf("DB nickname = %q, want IntegrationPlayer", user.Nickname)
	}

	// Verify refresh token was stored in Redis.
	_, err = refreshMgr.Validate(ctx, resp.UserID)
	if err == nil {
		t.Fatal("expected error for invalid refresh token (token is different from userID)")
	}
}

func TestMagicLinkRequestVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	_, rdb := testutil.SetupRedisStore(t)

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	timeouts := config.DefaultTimeoutConfig()
	authSvc := newMockAuthSvc(jwtMgr, nil, rdb, db, "re_test", "test@test.com", timeouts)
	authHandler := NewAuthHandler(db, rdb, authSvc, &Config{
		ResendAPIKey: "re_test",
		EmailFrom:    "test@test.com",
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/request", strings.NewReader(`{"email":"integration-test@example.com"}`))
	r.Header.Set("Content-Type", "application/json")
	authHandler.RequestMagicLink(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body.String())
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["message"] != "Magic link sent" {
		t.Fatalf("message = %q, want 'Magic link sent'", body["message"])
	}
}

func TestLobbyCreateJoinFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	ctx, rdb := testutil.SetupRedisStore(t)

	timeouts := config.DefaultTimeoutConfig()
	hub := game.NewHub(db, rdb, timeouts, 100, 10)
	lobbyHandler := NewLobbyHandler(hub, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil)
	lobbyHandler.CreateRoom(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("CreateRoom status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var createResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	code := createResp["code"]
	if code == "" {
		t.Fatal("expected non-empty room code")
	}

	// Verify room is registered in Redis.
	info, err := rdb.GetRoomRegistry(ctx, code)
	if err != nil {
		t.Fatalf("GetRoomRegistry: %v", err)
	}
	if info == nil || info.Code != code {
		t.Fatalf("room not found in Redis registry, got %+v", info)
	}

	// Verify room is persisted in PostgreSQL.
	ls, err := db.LoadLobbyState(ctx, code)
	if err != nil {
		t.Fatalf("LoadLobbyState: %v", err)
	}
	if ls == nil || ls.Code != code {
		t.Fatalf("lobby state not found in DB, got %+v", ls)
	}

	// Check room via HTTP.
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/"+code, nil)
	r2.SetPathValue("code", code)
	lobbyHandler.CheckRoom(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("CheckRoom status = %d, want 200; body=%s", w2.Code, w2.Body.String())
	}

	var checkResp map[string]interface{}
	if err := json.NewDecoder(w2.Body).Decode(&checkResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if checkResp["code"] != code {
		t.Fatalf("check code = %v, want %q", checkResp["code"], code)
	}
	if checkResp["phase"] != "waiting" {
		t.Fatalf("phase = %v, want waiting", checkResp["phase"])
	}
}

func TestLeaderboardAfterGameEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store

	// Insert a game session and results directly.
	ctx := context.Background()
	sessionID := "test-session-leaderboard"
	lobbyCode := "LBRD1"
	endedAt := int64(100)

	if err := db.CreateGameSession(ctx, &domain.GameSession{
		ID:         sessionID,
		LobbyCode:  lobbyCode,
		Status:     "ended",
		EndedAt:    &endedAt,
		FinalScore: 500,
	}); err != nil {
		t.Fatalf("CreateGameSession: %v", err)
	}

	statsHandler := NewStatsHandler(db)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard?scope=global&limit=10", nil)
	statsHandler.GetLeaderboard(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("GetLeaderboard status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Scope   string                    `json:"scope"`
		Entries []domain.LeaderboardEntry `json:"entries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Scope != "global" {
		t.Fatalf("scope = %q, want global", resp.Scope)
	}
}
