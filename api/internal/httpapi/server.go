// Package httpapi exposes the HTTP API.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jholhewres/draftmind/api/internal/auth"
	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/items"
	"github.com/jholhewres/draftmind/api/internal/photo"
	"github.com/jholhewres/draftmind/api/internal/writer"
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
	// Names are the current patch's hero and ability names, for the tip check.
	Names(ctx context.Context) (catalog.Names, error)
	// HeroHistory returns catalog.ErrNotFound for an unknown hero.
	HeroHistory(ctx context.Context, slug, lang string) (catalog.HeroHistory, error)
	Patches(ctx context.Context) ([]catalog.PatchSummary, error)
	// PatchChanges returns catalog.ErrNotFound for a patch not synced.
	PatchChanges(ctx context.Context, name, lang string) (catalog.PatchChanges, error)
	// HeroCounters returns catalog.ErrNotFound for a hero the current patch lacks.
	HeroCounters(ctx context.Context, slug string, n int) (catalog.Counters, error)
	// BuildDataset loads the current patch's item build data for these enemies.
	BuildDataset(ctx context.Context, enemies []int) (patch string, ds items.Dataset, err error)
	// HeroBuild returns catalog.ErrNotFound for a hero the current patch lacks.
	HeroBuild(ctx context.Context, slug string) (catalog.HeroBuild, error)
}

// Server wires the routes to their dependencies.
type Server struct {
	db      Pinger
	catalog Catalog
	jev     draft.Asker
	reader  photo.Reader
	tips    Tipper
	matches Matches
	coach   Replier
	auth    *auth.Service
	log     *slog.Logger

	suggestLimit, readLimit Limit
	mailIPLimit, mailLimit  Limit // sign-in links per client IP and per address
	now                     func() time.Time
	reads                   chan struct{} // a slot per photo read running
}

// New builds the server.
func New(db Pinger, cat Catalog, log *slog.Logger) *Server {
	return &Server{db: db, catalog: cat, log: log, suggestLimit: DefaultSuggestLimit, readLimit: DefaultReadLimit,
		mailIPLimit: DefaultMailIPLimit, mailLimit: DefaultMailLimit, now: time.Now,
		reads: make(chan struct{}, maxConcurrentReads)}
}

// WithDecider lets jev choose among the draft suggestions; nil keeps the
// code ranking.
func (s *Server) WithDecider(jev draft.Asker) *Server {
	s.jev = jev
	return s
}

// Tipper writes a hero's tip; it fails open to a template.
type Tipper interface {
	Tip(ctx context.Context, b writer.Brief) writer.Tip
}

// WithWriter writes the top heroes' tips in the stream; nil sends templates.
func (s *Server) WithWriter(t Tipper) *Server {
	s.tips = t
	return s
}

// WithAuth adds sign-in, sessions and GET /api/me; without it requireUser
// answers 401 and every visitor stays one.
func (s *Server) WithAuth(a *auth.Service) *Server {
	s.auth = a
	return s
}

// requireUser answers 401 unless the request has a live session; next
// reads the user with auth.UserFrom.
func (s *Server) requireUser(next http.Handler) http.Handler {
	if s.auth == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in first"})
		})
	}
	return s.auth.Require(next)
}

// userID is the signed-in user's id, or nil for a visitor.
func (s *Server) userID(r *http.Request) *int64 {
	if s.auth == nil {
		return nil
	}
	if u, ok := s.auth.UserOf(r); ok {
		return &u.ID
	}
	return nil
}

// Handler returns the routed handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/heroes", s.listHeroes)
	mux.HandleFunc("GET /api/heroes/{slug}", s.heroDetail)
	mux.HandleFunc("GET /api/heroes/{slug}/history", s.heroHistory)
	mux.HandleFunc("GET /api/heroes/{slug}/counters", s.heroCounters)
	mux.HandleFunc("GET /api/heroes/{slug}/build", s.heroBuild)
	mux.HandleFunc("GET /api/patches", s.listPatches)
	mux.HandleFunc("GET /api/patches/{name}", s.patchChanges)
	// These spend money on model calls and need no login.
	suggest, read := newLimiter(s.suggestLimit, s.now), newLimiter(s.readLimit, s.now)
	mux.Handle("POST /api/draft/suggest", suggest.wrap(http.HandlerFunc(s.suggest)))
	mux.Handle("POST /api/draft/stream", suggest.wrap(s.stream(read)))
	mux.Handle("POST /api/draft/read", read.wrap(http.HandlerFunc(s.readDraft)))
	mux.Handle("POST /api/draft/build", suggest.wrap(http.HandlerFunc(s.build)))
	mux.Handle("POST /api/match", suggest.wrap(http.HandlerFunc(s.startMatch)))
	mux.HandleFunc("GET /api/match/{id}", s.getMatch)
	mux.Handle("POST /api/match/{id}/messages", suggest.wrap(http.HandlerFunc(s.matchMessage)))
	if s.auth != nil {
		s.auth.Mount(mux, newLimiter(s.mailIPLimit, s.now).wrap, newLimiter(s.mailLimit, s.now).take)
	} else {
		mux.Handle("GET /api/me", s.requireUser(nil))
	}
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
