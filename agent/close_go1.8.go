// +build go1.8

package agent

import (
	"context"
	"github.com/google/gops/internal"
	"os"
	"time"
)

// Close closes the agent, removing temporary files and closing the http.Server using Shutdown method.
// If no agent is listening, Close does nothing.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if portfile, err := internal.PIDFile(os.Getpid()); err == nil {
		os.Remove(portfile)
	}

	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		server.Shutdown(ctx)
		server.Close()
	}
}
