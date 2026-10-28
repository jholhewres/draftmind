package httpapi

import (
	"net/http"

	"github.com/jholhewres/draftmind/api/internal/catalog"
)

type heroesResponse struct {
	Patch  string         `json:"patch"`
	Heroes []catalog.Hero `json:"heroes"`
}

func (s *Server) listHeroes(w http.ResponseWriter, r *http.Request) {
	patch, heroes, err := s.catalog.Heroes(r.Context())
	if err != nil {
		s.log.Error("list heroes", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not list heroes"})
		return
	}
	if heroes == nil {
		heroes = []catalog.Hero{}
	}
	writeJSON(w, http.StatusOK, heroesResponse{Patch: patch, Heroes: heroes})
}
