package main

import (
	"net/http"

	"github.com/google/gops/agent"
)

func main() {
	http.Handle("/path", &agent.Agent{})
	http.ListenAndServe(":4321", nil)
	// this syntax will enable
	// gops trace http://localhost:4321/path
}
