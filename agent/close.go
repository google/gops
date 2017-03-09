// +build !go1.8

package agent

import (
	"github.com/google/gops/internal"
	"os"
)

// Close closes the agent, removing temporary files and closing the http.Server.
// If no agent is listening, Close does nothing.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if portfile, err := internal.PIDFile(os.Getpid()); err == nil {
		os.Remove(portfile)
	}

	if listener != nil {
		listener.Close()
	}
}
