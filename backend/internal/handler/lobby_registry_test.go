package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/store"
)

type stubLobbyRepo struct {
	result *store.LobbyListResult
	err    error
}

func (s *stubLobbyRepo) SaveLobbyState(context.Context, *domain.LobbyState) error { return nil }
func (s *stubLobbyRepo) LoadLobbyState(context.Context, string) (*domain.LobbyState, error) {
	return nil, nil
}
func (s *stubLobbyRepo) DeleteLobbyState(context.Context, string) error { return nil }
func (s *stubLobbyRepo) LoadAllActiveLobbies(context.Context, int, string) (*store.LobbyListResult, error) {
	return s.result, s.err
}
func (s *stubLobbyRepo) CreateGameSession(context.Context, *domain.GameSession) error { return nil }
func (s *stubLobbyRepo) InsertOutboxEvent(context.Context, string, string, []byte) error {
	return nil
}
func (s *stubLobbyRepo) RecordGameResult(context.Context, string, string, int64, int, []domain.GameResultPlayer) error {
	return nil
}

func TestWriteDegradedLobbyList(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	writeDegradedLobbyList(w)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["degraded"] != true {
		t.Errorf("degraded = %v, want true", body["degraded"])
	}
	if body["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", body["total"])
	}
}

func TestListLobbies_DegradedWhenStoreUnavailable(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	h.ListLobbies(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 degraded", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["degraded"] != true {
		t.Errorf("degraded = %v, want true", body["degraded"])
	}
}

func TestListLobbies_SuccessWithETag(t *testing.T) {
	t.Parallel()

	repo := &stubLobbyRepo{
		result: &store.LobbyListResult{
			Lobbies: []domain.LobbyState{{Code: "ABCDE", State: "waiting"}},
			Total:   1,
		},
	}
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	hub := game.NewHub(repo, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h := NewLobbyHandler(hub, jwtMgr, nil)

	w := httptest.NewRecorder()
	h.ListLobbies(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header")
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil)
	r2.Header.Set("If-None-Match", etag)
	h.ListLobbies(w2, r2)
	if w2.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", w2.Code)
	}
}

func TestListLobbies_RespectsLimitQuery(t *testing.T) {
	t.Parallel()

	repo := &stubLobbyRepo{result: &store.LobbyListResult{Total: 0}}
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	hub := game.NewHub(repo, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h := NewLobbyHandler(hub, jwtMgr, nil)

	w := httptest.NewRecorder()
	h.ListLobbies(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies?limit=5", nil))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestListLobbies_InvalidLimitUsesDefault(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	h.ListLobbies(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies?limit=99999", nil))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 degraded", w.Code)
	}
}

func TestCreateRoom_Success(t *testing.T) {
	t.Parallel()

	repo := &stubLobbyRepo{result: &store.LobbyListResult{Total: 0}}
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	hub := game.NewHub(repo, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h := NewLobbyHandler(hub, jwtMgr, nil)

	w := httptest.NewRecorder()
	h.CreateRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"] == "" {
		t.Fatalf("body = %+v", body)
	}
}

func TestCreateRoom_HubUnavailable(t *testing.T) {
	t.Parallel()

	h := &LobbyHandler{hub: nil}
	w := httptest.NewRecorder()
	h.CreateRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestCreateRoom_CodeConflict(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h := NewLobbyHandler(hub, jwtMgr, nil)

	orig := game.SetGenerateRoomCodeHook(func() string { return "CONFL" })
	t.Cleanup(orig)
	hub.CreateRoom(context.Background()) // occupies CONFL

	w := httptest.NewRecorder()
	h.CreateRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

func TestCheckRoom_InvalidCode(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/BAD", nil), "code", "BAD")
	h.CheckRoom(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCheckRoom_Success(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/"+code, nil), "code", code)
	h.CheckRoom(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestRegistryCheckRoom_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ABCDE", nil), "code", "ABCDE")
	h.CheckRoom(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestMatchRoom_Success(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	h.MatchRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/match", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["lobbyCode"] == "" {
		t.Fatalf("body = %+v", body)
	}
}

func TestCheckRoom_HubUnavailable(t *testing.T) {
	t.Parallel()

	h := &LobbyHandler{hub: nil}
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ABCDE", nil), "code", "ABCDE")
	h.CheckRoom(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 degraded", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["degraded"] != true {
		t.Errorf("degraded = %v, want true", body["degraded"])
	}
}

func TestCheckRoom_CacheReadError(t *testing.T) {
	t.Parallel()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = redisStore.Close() })

	hub := game.NewHub(nil, redisStore, config.DefaultTimeoutConfig(), 0, 0, nil)
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	h := NewLobbyHandler(hub, jwtMgr, nil)

	mr.SetError("redis down")
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ABCDE", nil), "code", "ABCDE")
	h.CheckRoom(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 degraded", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["degraded"] != true {
		t.Errorf("degraded = %v, want true", body["degraded"])
	}
}

func TestListLobbies_MarshalError(t *testing.T) {
	t.Parallel()

	prev := jsonMarshalFn
	jsonMarshalFn = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	t.Cleanup(func() { jsonMarshalFn = prev })

	repo := &stubLobbyRepo{
		result: &store.LobbyListResult{
			Lobbies: []domain.LobbyState{{Code: "ABCDE", State: "waiting"}},
			Total:   1,
		},
	}
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	hub := game.NewHub(repo, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h := NewLobbyHandler(hub, jwtMgr, nil)

	w := httptest.NewRecorder()
	h.ListLobbies(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 degraded", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["degraded"] != true {
		t.Errorf("degraded = %v, want true", body["degraded"])
	}
}

type errResponseWriter struct {
	header     http.Header
	statusCode int
	failWrite  bool
}

func (e *errResponseWriter) Header() http.Header {
	if e.header == nil {
		e.header = make(http.Header)
	}
	return e.header
}

func (e *errResponseWriter) Write([]byte) (int, error) {
	if e.failWrite {
		return 0, context.Canceled
	}
	return len([]byte(`{"lobbies":[]}`)), nil
}

func (e *errResponseWriter) WriteHeader(statusCode int) {
	e.statusCode = statusCode
}

func TestListLobbies_WriteError(t *testing.T) {
	t.Parallel()

	repo := &stubLobbyRepo{
		result: &store.LobbyListResult{
			Lobbies: []domain.LobbyState{{Code: "ABCDE", State: "waiting"}},
			Total:   1,
		},
	}
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	hub := game.NewHub(repo, nil, config.DefaultTimeoutConfig(), 0, 0, nil)
	h := NewLobbyHandler(hub, jwtMgr, nil)

	w := &errResponseWriter{failWrite: true}
	h.ListLobbies(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil))
}

func TestHandleRegistryRoom_OpError(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	h.handleRegistryRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil), registryRoomParams{
		emptyKey:      "code",
		emptyVal:      "",
		unavailMsg:    "unavailable",
		unavailLog:    "hub unavailable",
		failLog:       "op failed",
		degradedMsg:   "degraded",
		responseField: "code",
	}, func(context.Context) (string, error) {
		return "", context.Canceled
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
