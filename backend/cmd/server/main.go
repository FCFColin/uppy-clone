// Command server is the HTTP/WebSocket entrypoint for the balloon game backend.
package main

import (
	"os"

	"github.com/uppy-clone/backend/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		os.Exit(1)
	}
}
