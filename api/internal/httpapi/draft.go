package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"

	"github.com/jholhewres/draftmind/api/internal/draft"
)

// Draft limits: the player's own slot is the one being suggested.
const (
	maxAllies       = 4
	maxEnemies      = 5
	maxBans         = 24
	defaultSuggests = 8
	maxSuggests     = 50
)

type suggestRequest struct {
	Allies  []int `json:"allies"`
	Enemies []int `json:"enemies"`
	Bans    []int `json:"bans"`
	Role    int   `json:"role"`
	Limit   *int  `json:"limit"`
}

type suggestResponse struct {
	Patch       string             `json:"patch"`
	Suggestions []draft.Suggestion `json:"suggestions"`
}

// suggest ranks every legal hero of the current patch for the draft.
func (s *Server) suggest(w http.ResponseWriter, r *http.Request) {
	var req suggestRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
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
	patch, ds, err := s.catalog.DraftDataset(r.Context(), req.Enemies)
	if err != nil {
		s.log.Error("draft dataset", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the draft data"})
		return
	}
	onPatch := make(map[int]bool, len(ds.Heroes))
	for _, h := range ds.Heroes {
		onPatch[h.ID] = true
	}
	for _, list := range [][]int{req.Allies, req.Enemies, req.Bans} {
		for _, id := range list {
			if !onPatch[id] {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown hero %d on the current patch", id)})
				return
			}
		}
	}
	limit := defaultSuggests
	if req.Limit != nil {
		limit = *req.Limit
	}
	d := draft.Draft{Allies: req.Allies, Enemies: req.Enemies, Bans: req.Bans, Role: req.Role}
	writeJSON(w, http.StatusOK, suggestResponse{Patch: patch, Suggestions: draft.Rank(ds, d, limit)})
}

func (q suggestRequest) validate() error {
	switch {
	case q.Role < 1 || q.Role > 5:
		return errors.New("role must be 1 to 5")
	case len(q.Allies) > maxAllies:
		return fmt.Errorf("at most %d allies", maxAllies)
	case len(q.Enemies) > maxEnemies:
		return fmt.Errorf("at most %d enemies", maxEnemies)
	case len(q.Bans) > maxBans:
		return fmt.Errorf("at most %d bans", maxBans)
	case q.Limit != nil && (*q.Limit < 1 || *q.Limit > maxSuggests):
		return fmt.Errorf("limit must be 1 to %d", maxSuggests)
	}
	seen := map[int]bool{}
	for _, list := range [][]int{q.Allies, q.Enemies, q.Bans} {
		for _, id := range list {
			if id <= 0 || id > math.MaxInt32 { // no such hero, and not a database integer
				return fmt.Errorf("unknown hero %d on the current patch", id)
			}
			if seen[id] {
				return fmt.Errorf("hero %d is in the draft twice", id)
			}
			seen[id] = true
		}
	}
	return nil
}
