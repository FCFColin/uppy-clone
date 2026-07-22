package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

func newTestLobbyHandler() *LobbyHandler {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	return NewLobbyHandler(hub, nil)
}

func testAuthMiddleware(userID, nickname string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithAuthenticatedUser(r.Context(), userID, nickname)
		next(w, r.WithContext(ctx))
	}
}

func newWSTestServer(h *LobbyHandler, userID, nickname string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /lobby/{code}/ws", testAuthMiddleware(userID, nickname, h.WebSocket))
	return httptest.NewServer(mux)
}

func newWSTestServerMultiUser(h *LobbyHandler) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /lobby/{code}/ws", func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-Test-User-ID")
		if userID == "" {
			userID = "default-user"
		}
		ctx := auth.WithAuthenticatedUser(r.Context(), userID, "nick")
		h.WebSocket(w, r.WithContext(ctx))
	})
	return httptest.NewServer(mux)
}

func wsDial(t *testing.T, server *httptest.Server, code, origin string) (*websocket.Conn, int) {
	t.Helper()
	url := "ws" + server.URL[4:] + "/lobby/" + code + "/ws"
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	hdr := http.Header{}
	if origin != "" {
		hdr.Set("Origin", origin)
	}
	conn, resp, err := dialer.Dial(url, hdr)
	if err != nil && resp == nil {
		t.Fatalf("websocket dial failed (no response): %v", err)
	}
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
		_ = resp.Body.Close()
	}
	return conn, statusCode
}

func newTestLobbyHandlerWithOrigins(origins []string) *LobbyHandler {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	return NewLobbyHandler(hub, origins)
}

func waitForConnCount(h *LobbyHandler, target int64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.hub.WSConnCount() == target {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return h.hub.WSConnCount() == target
}

// withPathParam sets a URL path parameter on the request using the Go 1.22+
// standard library mechanism (r.SetPathValue), avoiding chi-specific context
// plumbing in tests. The handler-side URLParam wrapper reads this value.
func withPathParam(r *http.Request, key, val string) *http.Request {
	r.SetPathValue(key, val)
	return r
}

// newTestAdminHandler builds an AdminHandler with a test JWT manager and no
// dependencies. Tests use it as a starting point and inject stores as needed.
func newTestAdminHandler() *AdminHandler {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	return NewAdminHandler(nil, jwtMgr, nil)
}

// newTestAuthHandlerWithRedis builds an AuthHandler backed by miniredis with
// default Config (no DB, no refresh manager). Returns the handler plus the
// redis store and JWT manager so tests can sign tokens or manipulate state.
func newTestAuthHandlerWithRedis(t *testing.T) (*AuthHandler, *store.RedisStore, *auth.JWTManager) {
	t.Helper()
	redisStore := testutil.SetupMiniredisStore(t)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	h := NewAuthHandler(nil, redisStore, jwtMgr, nil, &Config{})
	return h, redisStore, jwtMgr
}

// newTestAuthHandlerWithRefreshMgr builds an AuthHandler backed by miniredis
// with a refresh token manager. db may be nil. Returns handler, redis store,
// JWT manager, and refresh manager so tests can sign tokens or generate
// refresh tokens after setup.
func newTestAuthHandlerWithRefreshMgr(t *testing.T, db auth.UserDB) (*AuthHandler, *store.RedisStore, *auth.JWTManager, *auth.RefreshTokenManager) {
	t.Helper()
	redisStore := testutil.SetupMiniredisStore(t)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())
	h := NewAuthHandler(db, redisStore, jwtMgr, refreshMgr, &Config{})
	return h, redisStore, jwtMgr, refreshMgr
}

// signTestToken signs a token with jwtMgr; fails the test on error.
// E1: consolidates the SignToken + t.Fatalf pattern (6+ call sites).
func signTestToken(t *testing.T, jwtMgr *auth.JWTManager, userID, role string) string {
	t.Helper()
	token, err := jwtMgr.SignToken(userID, role)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	return token
}

// newTestJWTManager creates a JWT manager with the test private key.
// E1: wraps `auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)` (9+ sites).
func newTestJWTManager() *auth.JWTManager {
	return auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
}

// newTestUserRepo creates a pgxmock pool + UserRepository with t.Cleanup.
// E1: consolidates `mock := testutil.NewPgxMock(t); db := store.NewUserRepository(mock)` (10+ sites).
func newTestUserRepo(t *testing.T) (pgxmock.PgxPoolIface, *store.UserRepository) {
	t.Helper()
	mock := testutil.NewPgxMock(t)
	return mock, store.NewUserRepository(mock)
}

// expectGetUserByID sets a successful GetUserByID mock expectation.
// Returned row: id, email, nickname, palette=0, created_at=1, last_login=nil.
// E1: consolidates the 4-line ExpectQuery + NewRows + AddRow pattern.
func expectGetUserByID(mock pgxmock.PgxPoolIface, userID, email, nickname string) {
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
			AddRow(userID, email, nickname, 0, int64(1), nil))
}

// expectGetUserByIDError sets a failing GetUserByID mock expectation.
// E1: error variant of expectGetUserByID.
func expectGetUserByIDError(mock pgxmock.PgxPoolIface, userID string, err error) {
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs(userID).
		WillReturnError(err)
}

// newAuthHandlerWithDB builds an AuthHandler with a pgxmock-backed user repo
// and default Config. Returns handler, mock, and jwtMgr for signing tokens.
// E1: consolidates jwtMgr+NewAuthHandler construction (5+ sites).
func newAuthHandlerWithDB(t *testing.T) (*AuthHandler, pgxmock.PgxPoolIface, *auth.JWTManager) {
	t.Helper()
	mock, db := newTestUserRepo(t)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	h := NewAuthHandler(db, nil, jwtMgr, nil, &Config{})
	return h, mock, jwtMgr
}

// withAuthUser sets the authenticated-user context on r.
// E1: consolidates `r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), ...))` (4+ sites).
func withAuthUser(r *http.Request, userID, nickname string) *http.Request {
	return r.WithContext(auth.WithAuthenticatedUser(r.Context(), userID, nickname))
}

// newJSONRequest creates a ResponseRecorder + Request with Content-Type:
// application/json preset.
// E1: consolidates NewRecorder + NewRequest + Header.Set (9+ sites).
func newJSONRequest(method, path, body string) (*httptest.ResponseRecorder, *http.Request) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return w, r
}

// newStatsHandlerWithDB returns a StatsHandler backed by a pgxmock pool plus
// the mock for setting expectations. E5: consolidates the
// `mock := testutil.NewPgxMock(t); db := store.NewResultRepository(mock); h := NewStatsHandler(db)`
// pattern (5 sites in stats_handler_test.go).
func newStatsHandlerWithDB(t *testing.T) (*StatsHandler, pgxmock.PgxPoolIface) {
	t.Helper()
	mock := testutil.NewPgxMock(t)
	db := store.NewResultRepository(mock)
	return NewStatsHandler(db, nil), mock
}
