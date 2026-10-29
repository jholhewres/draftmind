// Package httpapi exposes the HTTP API.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/photo"
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
	// DraftDataset loads the current patch's ranking data for these enemies.
	DraftDataset(ctx context.Context, enemies []int) (patch string, ds draft.Dataset, err error)
}

// Server wires the routes to their dependencies.
type Server struct {
	db      Pinger
	catalog Catalog
	jev     draft.Asker
	reader  photo.Reader
	log     *slog.Logger

	suggestLimit, readLimit Limit
	now                     func() time.Time
}

// New builds the server.
func New(db Pinger, cat Catalog, log *slog.Logger) *Server {
	return &Server{db: db, catalog: cat, log: log, suggestLimit: DefaultSuggestLimit, readLimit: DefaultReadLimit, now: time.Now}
}

// WithDecider lets jev choose among the draft suggestions; nil keeps the
// code ranking.
func (s *Server) WithDecider(jev draft.Asker) *Server {
	s.jev = jev
	return s
}

// Handler returns the routed handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/heroes", s.listHeroes)
	mux.HandleFunc("GET /api/heroes/{slug}", s.heroDetail)
	// Both spend money on model calls and need no login.
	mux.Handle("POST /api/draft/suggest", newLimiter(s.suggestLimit, s.now).wrap(http.HandlerFunc(s.suggest)))
	mux.Handle("POST /api/draft/read", newLimiter(s.readLimit, s.now).wrap(http.HandlerFunc(s.readDraft)))
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
