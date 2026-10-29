package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"

	"github.com/jholhewres/draftmind/api/internal/draft"
)

// Draft limits: the player's own slot is the one being suggested.
const (
	maxAllies       = 4
	maxEnemies      = 5
	maxBans         = 24
	defaultSuggests = 8
	maxSuggests     = 50
	maxPool         = 50 // Captains Draft offers 27, Single Draft 3, Random Draft 24
)

type suggestRequest struct {
	Allies  []int `json:"allies"`
	Enemies []int `json:"enemies"`
	Bans    []int `json:"bans"`
	Role    int   `json:"role"`
	Limit   *int  `json:"limit"`
	// Mode is the game mode (ADR-0012); empty is ranked All Pick.
	Mode string `json:"mode,omitempty"`
	// Pool is the heroes offered in Single, Random or Captains Draft.
	Pool []int `json:"pool,omitempty"`
}

type suggestResponse struct {
	Patch string `json:"patch"`
	draft.Decided
	modeInfo
}

// modeInfo says which mode a ranking followed, which stats it read, and the
// bans to make when the mode has a ban phase.
type modeInfo struct {
	Mode  draft.Mode         `json:"mode"`
	Stats string             `json:"stats"`
	Bans  []draft.Suggestion `json:"bans,omitempty"`
}

// maxBanSuggests is how many bans a Captains draft suggests.
const maxBanSuggests = 5

func modeOf(ds draft.Dataset, d draft.Draft) modeInfo {
	return modeInfo{Mode: d.Mode, Stats: draft.StatsFor(ds, d.Mode), Bans: draft.Bans(ds, d, maxBanSuggests)}
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
	writeJSON(w, http.StatusOK, suggestResponse{Patch: patch, Decided: decided, modeInfo: modeOf(ds, d)})
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
	// A ban is ranked against our own side, so it needs the allies' matchups.
	against := req.Enemies
	if m, _ := draft.ParseMode(req.Mode); m.HasBans() {
		against = append(slices.Clone(req.Enemies), req.Allies...)
	}
	patch, ds, err := s.catalog.DraftDataset(ctx, against)
	if err != nil {
		s.log.Error("draft dataset", "err", err)
		return "", ds, http.StatusInternalServerError, errors.New("could not load the draft data")
	}
	onPatch := make(map[int]bool, len(ds.Heroes))
	for _, h := range ds.Heroes {
		onPatch[h.ID] = true
	}
	for _, list := range [][]int{req.Allies, req.Enemies, req.Bans, req.Pool} {
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
	m, _ := draft.ParseMode(q.Mode) // validated
	return draft.Draft{Allies: q.Allies, Enemies: q.Enemies, Bans: q.Bans, Role: q.Role, Mode: m, Pool: q.Pool}
}

func (q suggestRequest) validate() error {
	m, err := draft.ParseMode(q.Mode)
	if err != nil {
		return err
	}
	switch {
	case m.Pooled() && len(q.Pool) == 0:
		return fmt.Errorf("%s needs the heroes offered in pool", m)
	case len(q.Pool) > maxPool:
		return fmt.Errorf("at most %d heroes in the pool", maxPool)
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
