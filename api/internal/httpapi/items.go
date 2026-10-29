package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/items"
)

type buildRequest struct {
	Hero    int    `json:"hero"`
	Enemies []int  `json:"enemies"`
	Mode    string `json:"mode,omitempty"`
}

type buildResponse struct {
	Patch  string `json:"patch"`
	Source string `json:"popularitySource"`
	items.Build
}

// build serves POST /api/draft/build: the locked hero's core items by phase
// and the situational items against the enemies, chosen by Jev among the
// code's candidates, or in the code order when Jev cannot (ADR-0003).
func (s *Server) build(w http.ResponseWriter, r *http.Request) {
	var req buildRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: one object expected"})
		return
	}
	if err := req.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	patch, b, status, err := s.decideBuild(r, req)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, buildResponse{Patch: patch, Source: catalog.BuildSource, Build: b})
}

// decideBuild loads the current patch's data for req and decides the build.
func (s *Server) decideBuild(r *http.Request, req buildRequest) (string, items.Build, int, error) {
	patch, ds, err := s.catalog.BuildDataset(r.Context(), req.Enemies)
	if err != nil {
		s.log.Error("build dataset", "err", err)
		return "", items.Build{}, http.StatusInternalServerError, errors.New("could not load the build data")
	}
	for _, id := range append([]int{req.Hero}, req.Enemies...) {
		if _, ok := ds.Names[id]; !ok {
			return "", items.Build{}, http.StatusBadRequest, fmt.Errorf("unknown hero %d on the current patch", id)
		}
	}
	b := items.ForMode(items.Decide(r.Context(), s.jev, ds, req.Hero, req.Enemies), req.Mode)
	if b.Reason == items.ReasonTimeout || b.Reason == items.ReasonError {
		s.log.Warn("item build: kept the code order", "reason", b.Reason)
	}
	return patch, b, http.StatusOK, nil
}

func (q buildRequest) validate() error {
	if _, err := draft.ParseMode(q.Mode); err != nil {
		return err
	}
	if q.Hero <= 0 || q.Hero > math.MaxInt32 {
		return errors.New("hero is required")
	}
	if len(q.Enemies) > maxEnemies {
		return fmt.Errorf("at most %d enemies", maxEnemies)
	}
	seen := map[int]bool{q.Hero: true}
	for _, id := range q.Enemies {
		if id <= 0 || id > math.MaxInt32 {
			return fmt.Errorf("unknown hero %d on the current patch", id)
		}
		if seen[id] {
			return fmt.Errorf("hero %d is in the lineup twice", id)
		}
		seen[id] = true
	}
	return nil
}

// heroBuild serves GET /api/heroes/{slug}/build: the hero's most bought
// items by phase, from OpenDota item popularity on the current patch.
func (s *Server) heroBuild(w http.ResponseWriter, r *http.Request) {
	b, err := s.catalog.HeroBuild(r.Context(), r.PathValue("slug"))
	if errors.Is(err, catalog.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such hero on the current patch"})
		return
	}
	if err != nil {
		s.log.Error("hero build", "slug", r.PathValue("slug"), "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the hero's build"})
		return
	}
	writeJSON(w, http.StatusOK, b)
}
