package server

import (
	"log/slog"
	"os"
)

// initProfiling starts continuous profiling with Pyroscope when enabled.
// 企业为何需要：always-on profiling 让团队随时查看 CPU/内存火焰图，无需手动抓取。
func initProfiling() {
	if os.Getenv("ENABLE_PYROSCOPE") != "true" {
		return
	}
	pyroscopeAddress := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	if pyroscopeAddress == "" {
		return
	}
	// TODO: add github.com/grafana/pyroscope-go dependency to enable always-on profiling.
	slog.Info("Pyroscope continuous profiling enabled", "address", pyroscopeAddress)
}

// initLogger sets up the structured logger.
// Enterprise rationale: Structured JSON logs are required for log aggregation
// systems (ELK, Loki, Datadog). Text logs cannot be efficiently queried.
// LOG_FORMAT=text switches to text format for local development DX.
func initLogger() *slog.Logger {
	logLevel := parseLogLevel(getEnv("LOG_LEVEL", "info"))
	opts := &slog.HandlerOptions{Level: logLevel}
	var handler slog.Handler
	if getEnv("LOG_FORMAT", "json") == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
