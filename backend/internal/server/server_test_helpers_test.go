package server

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	appConfig "github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

// setupRunServerEnv sets the env vars + serverEnv required by runServer-based
// tests. The DatabaseURL and RedisURL are taken from the caller (mock or real).
// serverEnv is restored on t.Cleanup.
//
// E5: consolidates the 10-line `t.Setenv(...)*5 + prevEnv + serverEnv = Load()
// + EnableHSTS/MigrationsDir/AllowedOrigins + t.Cleanup` block that appeared
// 5+ times in server_lifecycle_test.go.
func setupRunServerEnv(t *testing.T, dbURL, redisAddr string) {
	t.Helper()
	t.Setenv("ENABLE_HSTS", "false")
	t.Setenv("JWT_PRIVATE_KEY", testsecrets.TestJWTPrivateKeyPEM)
	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("REDIS_URL", redisAddr)
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)

	prevEnv := serverEnv
	serverEnv = appConfig.Load()
	serverEnv.EnableHSTS = false
	serverEnv.MigrationsDir = "migrations"
	serverEnv.AllowedOrigins = "http://localhost"
	t.Cleanup(func() { serverEnv = prevEnv })
}

// bindFreePort finds a free TCP port, sets PORT=<port>, and returns the port.
// E5: consolidates the 6-line `net.Listen + t.Fatalf + .Addr().(*net.TCPAddr).Port
// + ln.Close + t.Setenv("PORT", ...)` block that appeared 3 times.
func bindFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	t.Setenv("PORT", strconv.Itoa(port))
	return port
}

// injectShutdownSignal replaces shutdownSignals with a channel the test controls.
// Returns the channel and restores the original on t.Cleanup.
// E5: consolidates the 4-line `sigCh + prev + shutdownSignals = ... + t.Cleanup`
// block that appeared 5 times in server_lifecycle_test.go.
func injectShutdownSignal(t *testing.T) chan os.Signal {
	t.Helper()
	sigCh := make(chan os.Signal, 1)
	prev := shutdownSignals
	shutdownSignals = func() <-chan os.Signal { return sigCh }
	t.Cleanup(func() { shutdownSignals = prev })
	return sigCh
}

// waitForHealthLive polls /health/live on 127.0.0.1:port until it responds or
// the deadline (5s) elapses. Failures are non-fatal — callers assert via the
// outer runServer result.
// E5: consolidates the 8-line polling loop that appeared 4 times.
func waitForHealthLive(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health/live")
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
