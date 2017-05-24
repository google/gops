package agent

import (
	"net/http"

	"github.com/google/gops/signal"
)

// HandlerFunc returns a function that handles gops requests over HTTP.
func HandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sig, ok := signal.FromParam(r.URL.Query().Get("action"))
		if !ok {
			w.WriteHeader(400)
			_, _ = w.Write([]byte("Unknown action!"))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		handle(w, sig)
	}
}
