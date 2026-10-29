package httpapi

import (
	"context"
	"net/http"
	"slices"

	"github.com/jholhewres/draftmind/api/internal/auth"
	"github.com/jholhewres/draftmind/api/internal/persona"
)

// Personas keeps each signed-in player's coach persona and UI language.
type Personas interface {
	Get(ctx context.Context, userID int64) (persona.Persona, error)
	Save(ctx context.Context, userID int64, p persona.Persona) error
	UILanguage(ctx context.Context, userID int64) (string, error)
	SaveUILanguage(ctx context.Context, userID int64, lang string) error
	GameMode(ctx context.Context, userID int64) (string, error)
	SaveGameMode(ctx context.Context, userID int64, mode string) error
}

// WithPersonas keeps signed-in players' personas; without it every coach
// request uses the persona it carries.
func (s *Server) WithPersonas(p Personas) *Server {
	s.personas = p
	return s
}

// personaOf is the persona a coach request is written with: the signed-in
// player's saved one, else the one in the query (tone, language, verbosity,
// focus; each missing one at its default). A bad value answers 400.
func (s *Server) personaOf(w http.ResponseWriter, r *http.Request) (persona.Persona, bool) {
	if s.personas != nil {
		if me := s.userID(r); me != nil {
			p, err := s.personas.Get(r.Context(), *me)
			if err != nil {
				s.log.Error("coach persona", "err", err)
				return persona.Default(), true // the persona shapes wording only: fail open
			}
			return p, true
		}
	}
	p := persona.Default()
	q := r.URL.Query()
	for key, field := range map[string]*string{"tone": &p.Tone, "language": &p.Language, "verbosity": &p.Verbosity, "focus": &p.Focus} {
		if v := q.Get(key); v != "" {
			*field = v
		}
	}
	if err := p.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return p, false
	}
	return p, true
}

// myPersona serves GET /api/me/persona: the signed-in player's persona, or
// the default before they save one.
func (s *Server) myPersona(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || s.personas == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings are not kept"})
		return
	}
	p, err := s.personas.Get(r.Context(), u.ID)
	if err != nil {
		s.log.Error("coach persona", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the settings"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// savePersona serves PUT /api/me/persona: every setting, each one of its values.
func (s *Server) savePersona(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || s.personas == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings are not kept"})
		return
	}
	var p persona.Persona
	if err := decodeOne(w, r, &p, 1<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := p.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.personas.Save(r.Context(), u.ID, p); err != nil {
		s.log.Error("save coach persona", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save the settings"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type uiLanguage struct {
	Language string `json:"language"`
}

// myLanguage serves GET /api/me/language: the UI language the signed-in
// player picked, or "" before they pick one.
func (s *Server) myLanguage(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || s.personas == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings are not kept"})
		return
	}
	lang, err := s.personas.UILanguage(r.Context(), u.ID)
	if err != nil {
		s.log.Error("ui language", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the settings"})
		return
	}
	writeJSON(w, http.StatusOK, uiLanguage{Language: lang})
}

// saveLanguage serves PUT /api/me/language: one of persona.UILanguages.
func (s *Server) saveLanguage(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || s.personas == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings are not kept"})
		return
	}
	var body uiLanguage
	if err := decodeOne(w, r, &body, 1<<8); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !slices.Contains(persona.UILanguages, body.Language) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "language must be en or pt-BR"})
		return
	}
	if err := s.personas.SaveUILanguage(r.Context(), u.ID, body.Language); err != nil {
		s.log.Error("save ui language", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save the settings"})
		return
	}
	writeJSON(w, http.StatusOK, body)
}

// myMode serves GET /api/me/mode: the game mode the signed-in player last
// coached a match in (ADR-0012), or "" before their first.
func (s *Server) myMode(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok || s.personas == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings are not kept"})
		return
	}
	mode, err := s.personas.GameMode(r.Context(), u.ID)
	if err != nil {
		s.log.Error("game mode", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"mode": mode})
}
