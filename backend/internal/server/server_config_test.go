package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/handler"
)

func TestParseLogLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"info":    slog.LevelInfo,
		"":        slog.LevelInfo,
	}
	for in, want := range tests {
		if got := parseLogLevel(in); got != want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLoadConfig_MapsEnv(t *testing.T) {
	t.Setenv("JWT_PRIVATE_KEY", "strong-secret-key-at-least-32-bytes-long!!")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("REDIS_URL", "redis:6379")
	t.Setenv("PORT", "9090")
	t.Setenv("RESEND_API_KEY", "re_test")

	cfg := loadConfig()
	if cfg.JWTPrivateKey == "" || cfg.Port != "9090" || cfg.RedisURL != "redis:6379" {
		t.Fatalf("loadConfig: %+v", cfg)
	}
	if serverEnv == nil || serverEnv.Port != "9090" {
		t.Fatal("serverEnv not populated")
	}
}

func TestGetEnv_PrefersServerEnv(t *testing.T) {
	serverEnv = &appConfig.Env{AllowedOrigins: "https://app.example", Port: "3000"}
	t.Cleanup(func() { serverEnv = nil })
	if got := getEnv("ALLOWED_ORIGINS", "default"); got != "https://app.example" {
		t.Errorf("ALLOWED_ORIGINS = %q", got)
	}
	if got := getEnv("PORT", "8080"); got != "3000" {
		t.Errorf("PORT = %q", got)
	}
}

func TestMetricsAuthMiddleware_ForbiddenInProduction(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("METRICS_USER", "")
	t.Setenv("METRICS_PASSWORD", "")
	t.Cleanup(func() {
		os.Unsetenv("ENV")
		os.Unsetenv("METRICS_USER")
		os.Unsetenv("METRICS_PASSWORD")
	})

	handler := metricsAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 when metrics auth not configured in production", rec.Code)
	}
}

func TestMetricsAuthMiddleware_WrongPassword(t *testing.T) {
	t.Setenv("METRICS_USER", "metrics")
	t.Setenv("METRICS_PASSWORD", "secret")
	t.Cleanup(func() {
		os.Unsetenv("METRICS_USER")
		os.Unsetenv("METRICS_PASSWORD")
	})

	handler := metricsAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.SetBasicAuth("metrics", "wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestGetEnvInt_InvalidUsesDefault(t *testing.T) {
	t.Setenv("MAX_WS_CONNECTIONS", "not-int")
	if got := getEnvInt("MAX_WS_CONNECTIONS", 42); got != 42 {
		t.Errorf("got %d", got)
	}
}

func TestMetricsAuthMiddleware_RequiresBasicAuth(t *testing.T) {
	t.Setenv("METRICS_USER", "metrics")
	t.Setenv("METRICS_PASSWORD", "secret")
	t.Cleanup(func() {
		os.Unsetenv("METRICS_USER")
		os.Unsetenv("METRICS_PASSWORD")
	})

	handler := metricsAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("missing credentials", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d", rec.Code)
		}
	})

	t.Run("valid credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.SetBasicAuth("metrics", "secret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d", rec.Code)
		}
	})
}

func TestMetricsAuthMiddleware_DevModeOpen(t *testing.T) {
	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("METRICS_USER", "")
	t.Setenv("METRICS_PASSWORD", "")
	if os.Getenv("METRICS_USER") != "" || os.Getenv("METRICS_PASSWORD") != "" {
		t.Skip("METRICS_* set in environment")
	}
	handler := metricsAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK && rec.Code != http.StatusUnauthorized {
		t.Errorf("unexpected status %d", rec.Code)
	}
	if os.Getenv("METRICS_USER") == "" && os.Getenv("METRICS_PASSWORD") == "" && rec.Code != http.StatusOK {
		t.Errorf("dev mode should allow open metrics, got %d", rec.Code)
	}
}

func TestGetEnvInt_ServerEnvZeroUsesEnvVar(t *testing.T) {
	serverEnv = &appConfig.Env{MaxWSConnections: 0}
	t.Cleanup(func() { serverEnv = nil })
	t.Setenv("MAX_WS_CONNECTIONS", "77")
	if got := getEnvInt("MAX_WS_CONNECTIONS", 42); got != 77 {
		t.Errorf("got %d, want 77 from env when serverEnv value is zero", got)
	}
}

func TestGetEnvInt_InvalidEnvVar(t *testing.T) {
	serverEnv = nil
	t.Cleanup(func() { serverEnv = nil })
	t.Setenv("MAX_WS_CONNECTIONS", "not-a-number")
	if got := getEnvInt("MAX_WS_CONNECTIONS", 42); got != 42 {
		t.Errorf("invalid env got %d, want default 42", got)
	}
}

func TestGetEnvInt_PrefersServerEnv(t *testing.T) {
	serverEnv = &appConfig.Env{MaxWSConnections: 200, MaxPlayersPerRoom: 16}
	t.Cleanup(func() { serverEnv = nil })
	if got := getEnvInt("MAX_WS_CONNECTIONS", 42); got != 200 {
		t.Errorf("MAX_WS_CONNECTIONS = %d", got)
	}
	if got := getEnvInt("MAX_PLAYERS_PER_ROOM", 8); got != 16 {
		t.Errorf("MAX_PLAYERS_PER_ROOM = %d", got)
	}
}

func TestValidateConfig_ExitsOnInvalidEnv(t *testing.T) {
	if os.Getenv("TEST_VALIDATE_CONFIG_SUBPROCESS") == "1" {
		serverEnv = &appConfig.Env{}
		validateConfig(&handler.Config{}, slog.Default())
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestValidateConfig_ExitsOnInvalidEnv$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_VALIDATE_CONFIG_SUBPROCESS=1")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("validateConfig should exit non-zero, got %v", err)
	}
}
