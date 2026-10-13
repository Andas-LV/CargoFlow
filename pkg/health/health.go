// Package health serves liveness (/healthz) and readiness (/readyz) HTTP endpoints.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"
)

// Check reports whether a dependency (DB, broker, ...) is usable.
type Check func(ctx context.Context) error

// Checks maps a dependency name to its check.
type Checks map[string]Check

// Register mounts GET /healthz and GET /readyz on mux.
// /healthz always returns 200 while the process is running.
// /readyz runs all checks and returns 503 if any of them fails.
func Register(mux *http.ServeMux, checks Checks) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		code, result := http.StatusOK, map[string]string{}
		for name, check := range checks {
			if err := check(ctx); err != nil {
				code, result[name] = http.StatusServiceUnavailable, err.Error()
				continue
			}
			result[name] = "ok"
		}
		writeJSON(w, code, result)
	})
}

// Server returns an HTTP server exposing only the health endpoints on addr.
func Server(addr string, checks Checks) *http.Server {
	mux := http.NewServeMux()
	Register(mux, checks)
	return &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}

// ListenAndServe runs srv and treats http.ErrServerClosed as a clean exit.
func ListenAndServe(ctx context.Context, srv *http.Server) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return err
	}
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
