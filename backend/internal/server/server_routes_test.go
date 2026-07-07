package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v4"
	"golang.org/x/crypto/bcrypt"
	"github.com/uppy-clone/backend/internal/auth"
	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/handler"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/rbac"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

func newTestRouter(t *testing.T) *chi.Mux {
	t.Helper()

	prevEnv := serverEnv
	serverEnv = &appConfig.Env{
		TrustedProxyCIDRs: "127.0.0.1/32",
	}
	t.Cleanup(func() { serverEnv = prevEnv })

	jwtSecret := testsecrets.TestJWTPrivateKeyPEM
	jwtMgr := auth.NewJWTManager(jwtSecret)
	adminJwtMgr := auth.NewJWTManager(jwtSecret)
	timeouts := appConfig.DefaultTimeoutConfig()
	hub := game.NewHub(nil, nil, timeouts, 10, 50, nil)

	cfg := &handler.Config{FrontendDir: ""}
	authSvc := handler.NewDefaultAuthService(jwtMgr, nil, nil, nil, "", "", timeouts)
	authHandler := handler.NewAuthHandler(nil, nil, authSvc, cfg)
	lobbyHandler := handler.NewLobbyHandler(hub, nil)
	adminHandler := handler.NewAdminHandler(nil, adminJwtMgr, nil)
	statsHandler := handler.NewStatsHandler(nil)
	rbacEnforcer := rbac.NewEnforcer()
	redisStore := testutil.SetupMiniredisStore(t)
	cluster := store.NewRedisClusterFromStores(redisStore, nil)

	r := chi.NewRouter()
	setupRoutes(r, authHandler, lobbyHandler, adminHandler, statsHandler, jwtMgr, nil, cluster, rbacEnforcer, cfg, hub)
	return r
}

func TestSetupRoutes_HealthLive(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSetupRoutes_HealthReady(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 200 or 503", rec.Code)
	}
}

func TestSetupRoutes_LeaderboardNilDB(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestSetupRoutes_UserStatsUnauthorized(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/user/stats", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestSetupRoutes_AuthCheckUnauthorized(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestSetupRoutes_ListLobbiesDegraded(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 degraded", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"degraded":true`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestSetupRoutes_AdminPutConfigUnauthorized(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/admin/config", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestSetupAdminRoutes_DeprecatedPutConfigHeaders(t *testing.T) {
	jwtSecret := testsecrets.TestJWTPrivateKeyPEM
	jwtMgr := auth.NewJWTManager(jwtSecret)

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

	password := "secret"
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	loginCfg, _ := json.Marshal(map[string]string{"admin_password": string(hashed)})

	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnRows(pgxmock.NewRows([]string{"id", "config", "updated_at"}).
			AddRow("global", string(loginCfg), int64(1000)))

	db := store.NewPostgresStoreWithPool(mock)
	redisStore := testutil.SetupMiniredisStore(t)
	cluster := store.NewRedisClusterFromStores(redisStore, nil)
	adminHandler := handler.NewAdminHandler(db, jwtMgr, redisStore)

	r := chi.NewRouter()
	setupAdminRoutes(r, adminHandler, cluster, jwtMgr, rbac.NewEnforcer())

	loginRec := httptest.NewRecorder()
	r.ServeHTTP(loginRec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/login",
		strings.NewReader(`{"password":"`+password+`"}`)))
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", loginRec.Code, loginRec.Body.String())
	}

	var adminCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "admin_token" {
			adminCookie = c
			break
		}
	}
	if adminCookie == nil {
		t.Fatal("missing admin_token cookie after login")
	}

	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config", strings.NewReader("{bad"))
	putReq.AddCookie(adminCookie)
	putRec := httptest.NewRecorder()
	r.ServeHTTP(putRec, putReq)

	if putRec.Header().Get("Deprecation") != "true" {
		t.Errorf("Deprecation header = %q, want true", putRec.Header().Get("Deprecation"))
	}
	if putRec.Header().Get("Sunset") == "" {
		t.Error("expected Sunset header on deprecated PUT /config")
	}
	if putRec.Header().Get("Link") == "" {
		t.Error("expected Link header on deprecated PUT /config")
	}
}

func TestSetupRoutes_AdminLoginBadRequest(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader("{bad")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestSetupRoutes_AdminConfigUnauthorized(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestSetupRoutes_MagicLinkRequestBadRequest(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/request", strings.NewReader(`{"email":""}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAuthMiddlewareWrapper_RejectsMissingAuth(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	mw := authMiddlewareWrapper(jwtMgr, nil)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/registry/create", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAdminAuthMiddleware_RejectsMissingToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	adminHandler := handler.NewAdminHandler(nil, jwtMgr, nil)
	mw := adminAuthMiddleware(adminHandler)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/logout", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestSetupMiddleware_RegistersRequestID(t *testing.T) {
	prevEnv := serverEnv
	serverEnv = &appConfig.Env{TrustedProxyCIDRs: "127.0.0.1/32"}
	t.Cleanup(func() { serverEnv = prevEnv })

	r := chi.NewRouter()
	setupMiddleware(r)
	var requestID string
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		requestID = appMiddleware.GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if requestID == "" {
		t.Error("expected request ID in context from middleware chain")
	}
}

func TestInitHandlers(t *testing.T) {
	prevEnv := serverEnv
	serverEnv = &appConfig.Env{AllowedOrigins: "http://localhost"}
	t.Cleanup(func() { serverEnv = prevEnv })

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	cfg := &handler.Config{}
	hub := game.NewHub(nil, nil, appConfig.DefaultTimeoutConfig(), 0, 0, nil)
	redisStore := testutil.SetupMiniredisStore(t)

	authH, lobbyH, adminH, statsH := initHandlers(jwtMgr, jwtMgr, nil, redisStore, cfg, appConfig.DefaultTimeoutConfig(), hub)
	if authH == nil || lobbyH == nil || adminH == nil || statsH == nil {
		t.Fatal("initHandlers returned nil handler")
	}
}

func TestInitRBAC(t *testing.T) {
	if initRBAC() == nil {
		t.Fatal("initRBAC returned nil")
	}
}

func TestValidateConfig_ValidEnv(t *testing.T) {
	serverEnv = &appConfig.Env{
		JWTPrivateKey:     testsecrets.TestJWTPrivateKeyPEM,
		DatabaseURL:       "postgres://localhost/test",
		EncryptionKey:     testsecrets.TestEncryptionKeyHex,
		TrustedProxyCIDRs: "127.0.0.1/32",
	}
	t.Cleanup(func() { serverEnv = nil })
	t.Setenv("ENABLE_HSTS", "true")

	cfg := &handler.Config{}
	validateConfig(cfg, slog.Default())
}

func TestStartServer_StartsAndShutsDown(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := startServer(r, &handler.Config{Port: "0"})
	if srv == nil {
		t.Fatal("startServer returned nil")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestSetupStatsRoutes_NilHandlerSkipsRoutes(t *testing.T) {
	r := chi.NewRouter()
	setupStatsRoutes(r, nil, nil, auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM), rbac.NewEnforcer())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/leaderboard", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when stats handler nil", rec.Code)
	}
}

func TestSetupStaticRoutes_ServesFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}

	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/ status = %d, want 200", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("/app.js status = %d, want 200", rec2.Code)
	}
	if !strings.Contains(rec2.Header().Get("Cache-Control"), "max-age=") {
		t.Fatalf("Cache-Control = %q, want max-age for static asset", rec2.Header().Get("Cache-Control"))
	}
}

func TestSetupStaticRoutes_PathTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	r := chi.NewRouter()
	setupStaticRoutes(r, &handler.Config{FrontendDir: dir})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/../../etc/passwd", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for traversal", rec.Code)
	}
}

func TestAuthMiddlewareWrapper_AcceptsValidToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	redisStore := testutil.SetupMiniredisStore(t)
	token, err := jwtMgr.SignToken("user-wrap", "Nick")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	mw := authMiddlewareWrapper(jwtMgr, redisStore)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/registry/create", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusOK {
		t.Fatalf("called=%v status=%d", called, rec.Code)
	}
}
