package bootstrap

import (
	"context"
	"log/slog"
	"sync"
)

// StartWorker launches fn in a new goroutine tracked by wg. Logs start/stop
// events tagged with name. Used by both server.startWorkers and
// worker.runWorker to launch background consumer goroutines.
//
// Previously duplicated as server.startWorker and worker.startWorkerLoop
// (identical 4-line function bodies).
func StartWorker(ctx context.Context, wg *sync.WaitGroup, name string, fn func(context.Context)) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		fn(ctx)
		slog.Info(name + " worker stopped")
	}()
	slog.Info(name + " worker started")
}
