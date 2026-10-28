// Package httpapi exposes the HTTP API.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jholhewres/draftmind/api/internal/catalog"
)

// Pinger reports whether the database answers.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Catalog reads the synced game data.
type Catalog interface {
	Heroes(ctx context.Context) (patch string, heroes []catalog.Hero, err error)
	// HeroDetail returns catalog.ErrNotFound for a hero the current patch lacks.
	HeroDetail(ctx context.Context, slug, lang string) (catalog.HeroDetail, error)
}

// Server wires the routes to their dependencies.
type Server struct {
	db      Pinger
	catalog Catalog
	log     *slog.Logger
}

// New builds the server.
func New(db Pinger, cat Catalog, log *slog.Logger) *Server {
	return &Server{db: db, catalog: cat, log: log}
}

// Handler returns the routed handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/heroes", s.listHeroes)
	mux.HandleFunc("GET /api/heroes/{slug}", s.heroDetail)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		s.log.Warn("health: database ping failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "database": "down"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "up"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
