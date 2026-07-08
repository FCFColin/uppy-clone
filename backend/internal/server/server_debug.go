package server

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/grafana/pyroscope-go"
)

type pyroscopeLogger struct{}

func (pyroscopeLogger) Infof(msg string, args ...interface{}) {
	slog.Info(fmt.Sprintf(msg, args...))
}

func (pyroscopeLogger) Debugf(msg string, args ...interface{}) {
	slog.Debug(fmt.Sprintf(msg, args...))
}

func (pyroscopeLogger) Errorf(msg string, args ...interface{}) {
	slog.Error(fmt.Sprintf(msg, args...))
}

func initProfiling() {
	if os.Getenv("ENABLE_PYROSCOPE") != "true" {
		return
	}
	address := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	if address == "" {
		slog.Warn("PYROSCOPE_SERVER_ADDRESS not set, skipping profiling")
		return
	}

	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: "balloon-game",
		ServerAddress:   address,
		Logger:          pyroscopeLogger{},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
		},
	})
	if err != nil {
		slog.Error("failed to start pyroscope", "error", err)
		return
	}
	slog.Info("Pyroscope continuous profiling enabled", "address", address)
}

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
