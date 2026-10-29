package httpapi

import (
	"context"
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
	Patch string `json:"patch"`
	draft.Decided
}

// suggest ranks every legal hero of the current patch for the draft and
// lets Jev order the top of the ranking (ADR-0003); the code ranking stands,
// labelled, when Jev fails, is late or is unsure.
func (s *Server) suggest(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDraft(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	patch, ds, status, err := s.loadDraft(r.Context(), req)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	limit := req.limit()
	d := req.draft()
	decided := draft.Decide(r.Context(), s.jev, ds, d, draft.Rank(ds, d, max(limit, draft.JevCandidates)))
	s.logDecided(decided)
	decided.Suggestions = decided.Suggestions[:min(limit, len(decided.Suggestions))]
	writeJSON(w, http.StatusOK, suggestResponse{Patch: patch, Decided: decided})
}

// decodeDraft reads exactly one valid draft from r's JSON body.
func decodeDraft(w http.ResponseWriter, r *http.Request) (suggestRequest, error) {
	var req suggestRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, errors.New("invalid JSON: " + err.Error())
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return req, errors.New("invalid JSON: one object expected")
	}
	return req, req.validate()
}

// loadDraft loads the current patch's ranking data for req, refusing a
// hero the patch lacks.
func (s *Server) loadDraft(ctx context.Context, req suggestRequest) (string, draft.Dataset, int, error) {
	patch, ds, err := s.catalog.DraftDataset(ctx, req.Enemies)
	if err != nil {
		s.log.Error("draft dataset", "err", err)
		return "", ds, http.StatusInternalServerError, errors.New("could not load the draft data")
	}
	onPatch := make(map[int]bool, len(ds.Heroes))
	for _, h := range ds.Heroes {
		onPatch[h.ID] = true
	}
	for _, list := range [][]int{req.Allies, req.Enemies, req.Bans} {
		for _, id := range list {
			if !onPatch[id] {
				return "", ds, http.StatusBadRequest, fmt.Errorf("unknown hero %d on the current patch", id)
			}
		}
	}
	return patch, ds, http.StatusOK, nil
}

// logDecided logs why the code ranking stood, and Jev's shadow answer.
func (s *Server) logDecided(d draft.Decided) {
	switch d.Reason {
	case draft.ReasonTimeout, draft.ReasonError:
		s.log.Warn("draft suggest: kept the code ranking", "reason", d.Reason)
	case draft.ReasonLowConfidence:
		s.log.Info("draft suggest: Jev below the confidence floor, kept the code ranking (shadow)",
			"confidence", *d.Confidence, "floor", draft.MinConfidence, "jev_first", d.Shadow)
	}
}

func (q suggestRequest) limit() int {
	if q.Limit != nil {
		return *q.Limit
	}
	return defaultSuggests
}

func (q suggestRequest) draft() draft.Draft {
	return draft.Draft{Allies: q.Allies, Enemies: q.Enemies, Bans: q.Bans, Role: q.Role}
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
