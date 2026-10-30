package draft

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"
	"strconv"

	"github.com/jholhewres/draftmind/api/internal/budget"
	"github.com/jholhewres/draftmind/api/internal/decision"
)

// JevCandidates is how many of the ranking's top heroes Jev chooses among.
const JevCandidates = 8

// Where a list's order came from, and why the ranking was kept.
const (
	SourceJev     = "jev"
	SourceRanking = "ranking"

	ReasonTimeout       = "timeout"        // Jev did not answer within the deadline
	ReasonError         = "error"          // Jev failed or answered badly
	ReasonUnavailable   = "unavailable"    // no decision model is configured
	ReasonFewCandidates = "few_candidates" // nothing to choose between
	ReasonLowConfidence = "low_confidence" // Jev answered below MinConfidence; its answer is only logged
	ReasonBudget        = "budget"         // the model spend cap is used up
)

// MinConfidence is the least confidence Jev's choice needs to reorder the
// list; below it the code order stands and Jev's answer is shadow (ADR-0003).
const MinConfidence = 0.4

// Asker asks the decision model.
type Asker interface {
	Ask(ctx context.Context, req decision.Request) (decision.Result, error)
}

// Decided is the ranked list in Jev's order, or the code ranking with the
// reason it was kept (ADR-0003: fail open).
type Decided struct {
	Source      string       `json:"source"`
	Reason      string       `json:"reason,omitempty"`
	Confidence  *float64     `json:"confidence,omitempty"`
	Suggestions []Suggestion `json:"suggestions"`
	// Shadow is the hero Jev would have put first when its answer was below
	// MinConfidence, for the log only.
	Shadow int `json:"-"`
}

// positions name the player's position for Jev.
var positions = map[int]string{1: "safe lane carry", 2: "mid lane", 3: "off lane", 4: "soft support", 5: "hard support"}

// PositionName names position role (1-5), or is empty.
func PositionName(role int) string { return positions[role] }

const pickInstructions = "Which candidate should the player pick for their position in this draft? " +
	"Each candidate lists the reasons the stats give: its win-rate delta in points against each enemy " +
	"with the games behind it, its patch win rate, its roles and how well they fit the position (0 to 1), " +
	"and the stats score that ranked it."

// Decide asks Jev to choose among the top JevCandidates of ranked, in one
// choice question, and orders them by Jev's probabilities; the rest keep
// the code order. Any failure keeps the code ranking, labelled.
func Decide(ctx context.Context, jev Asker, ds Dataset, d Draft, ranked []Suggestion) Decided {
	fallback := func(reason string) Decided {
		return Decided{Source: SourceRanking, Reason: reason, Suggestions: ranked}
	}
	if len(ranked) < 2 {
		return fallback(ReasonFewCandidates)
	}
	if jev == nil {
		return fallback(ReasonUnavailable)
	}
	heroes := make(map[int]Hero, len(ds.Heroes))
	for _, h := range ds.Heroes {
		heroes[h.ID] = h
	}
	name := func(id int) string {
		if h := heroes[id]; h.Name != "" {
			return h.Name
		}
		return "#" + strconv.Itoa(id)
	}
	top := ranked[:min(JevCandidates, len(ranked))]
	options := make(map[string]any, len(top))
	byOption := make(map[string]int, len(top))
	for i, s := range top {
		key := name(s.HeroID)
		if _, dup := options[key]; dup {
			return fallback(ReasonError)
		}
		byOption[key] = i
		vs := make([]map[string]any, len(s.Enemies))
		for j, e := range s.Enemies {
			vs[j] = map[string]any{"enemy": name(e.HeroID), "delta": round1(e.Delta), "games": e.Games}
		}
		options[key] = map[string]any{
			"roles":           heroes[s.HeroID].Roles,
			"role_fit":        s.RoleFit,
			"against_enemies": vs,
			"patch_win_rate":  round1(s.WinRate),
			"stats_score":     round1(s.Score),
		}
	}
	names := func(ids []int) []string {
		out := make([]string, len(ids))
		for i, id := range ids {
			out[i] = name(id)
		}
		return out
	}
	state := map[string]any{
		"task":     "Choose the hero the player should pick in a 5v5 hero draft.",
		"position": map[string]any{"number": d.Role, "name": positions[d.Role]},
		"allies":   names(d.Allies),
		"enemies":  names(d.Enemies),
		"bans":     names(d.Bans),
	}
	res, err := jev.Ask(ctx, decision.Request{
		Purpose:   "draft.suggest",
		State:     state,
		Questions: map[string]decision.Question{"pick": decision.Choice(pickInstructions, options)},
	})
	if errors.Is(err, budget.ErrOverCap) {
		return fallback(ReasonBudget)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fallback(ReasonTimeout)
	}
	if err != nil {
		return fallback(ReasonError)
	}
	pick := res.Answers["pick"]
	out := slices.Clone(ranked)
	for key, i := range byOption {
		p := pick.Probabilities[key]
		out[i].Probability = &p
	}
	// Stable: equal probabilities keep the code order.
	slices.SortStableFunc(out[:len(top)], func(a, b Suggestion) int { return cmp.Compare(*b.Probability, *a.Probability) })
	conf := pick.Confidence
	if conf < MinConfidence {
		return Decided{Source: SourceRanking, Reason: ReasonLowConfidence, Confidence: &conf, Suggestions: ranked, Shadow: out[0].HeroID}
	}
	return Decided{Source: SourceJev, Confidence: &conf, Suggestions: out}
}

func round1(x float64) float64 { return math.Round(x*10) / 10 }
