package httpapi

import (
	"errors"
	"net/http"

	"github.com/jholhewres/draftmind/api/internal/catalog"
)

func (s *Server) heroHistory(w http.ResponseWriter, r *http.Request) {
	lang := pickLang(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
	h, err := s.catalog.HeroHistory(r.Context(), r.PathValue("slug"), lang)
	if errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such hero"})
		return
	}
	if err != nil {
		s.log.Error("hero history", "slug", r.PathValue("slug"), "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the hero's history"})
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) listPatches(w http.ResponseWriter, r *http.Request) {
	ps, err := s.catalog.Patches(r.Context())
	if err != nil {
		s.log.Error("list patches", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not list the patches"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"patches": ps})
}

func (s *Server) patchChanges(w http.ResponseWriter, r *http.Request) {
	lang := pickLang(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
	p, err := s.catalog.PatchChanges(r.Context(), r.PathValue("name"), lang)
	if errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such patch"})
		return
	}
	if err != nil {
		s.log.Error("patch changes", "patch", r.PathValue("name"), "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the patch"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}
