// Package server hosts the local HTTP API and serves the embedded Svelte viewer.
//
// The server binds only to localhost by default. It exposes:
//
//	GET  /api/schema  -> current schema as JSON
//	POST /api/layout  -> persist per-table layout hints back to the .erd file
//	GET  /*           -> static assets from the embedded web/dist bundle
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/erdlens/erdlens/internal/schema"
)

// Config controls how the local viewer server binds and behaves.
type Config struct {
	// Addr to bind. Empty means "127.0.0.1:0" (OS-assigned free port).
	Addr string
	// Assets is the embedded frontend bundle (web/dist). Required.
	Assets fs.FS
	// Schema is the initial schema served at /api/schema.
	Schema *schema.Schema
	// OnSave is invoked after a successful POST /api/layout, with the mutated
	// schema. Typically writes to the source .erd file.
	OnSave func(*schema.Schema) error
}

// Server is a running local viewer.
type Server struct {
	cfg      Config
	mu       sync.Mutex
	schema   *schema.Schema
	listener net.Listener
	http     *http.Server
}

// Start listens on cfg.Addr and begins serving. It returns immediately once
// the listener is open; use Addr() to discover the assigned port.
func Start(_ context.Context, cfg Config) (*Server, error) {
	if cfg.Schema == nil {
		return nil, errors.New("server: nil schema")
	}
	if cfg.Assets == nil {
		return nil, errors.New("server: nil assets")
	}
	addr := cfg.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	s := &Server{cfg: cfg, schema: cfg.Schema, listener: ln}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/schema", s.handleSchema)
	mux.HandleFunc("/api/layout", s.handleLayout)
	mux.Handle("/", http.FileServer(http.FS(cfg.Assets)))

	s.http = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = s.http.Serve(ln) }()
	return s, nil
}

// Addr returns the URL the server is listening on.
func (s *Server) Addr() string {
	return "http://" + s.listener.Addr().String()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) handleSchema(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	sc := s.schema
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// layoutPayload maps table name -> position.
type layoutPayload map[string]schema.Layout

func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var payload layoutPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	for i := range s.schema.Tables {
		if l, ok := payload[s.schema.Tables[i].Name]; ok {
			ll := l
			s.schema.Tables[i].Layout = &ll
		}
	}
	sc := s.schema
	save := s.cfg.OnSave
	s.mu.Unlock()

	if save != nil {
		if err := save(sc); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
