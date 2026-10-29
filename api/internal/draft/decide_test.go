package draft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/decision"
)

// jevServer answers System One requests with handle and keeps the last body.
func jevServer(t *testing.T, handle func(w http.ResponseWriter, body map[string]any)) (*decision.Client, *map[string]any) {
	t.Helper()
	var last map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		last = body
		handle(w, body)
	}))
	t.Cleanup(srv.Close)
	c := decision.New(srv.URL, "test-key", "typesafe/jev-1.13", "typesafe", nil)
	c.HTTP = srv.Client()
	return c, &last
}

// answerWith answers the pick question with probs, named by hero, at confidence 0.42.
func answerWith(probs map[string]float64) func(http.ResponseWriter, map[string]any) {
	return answerAt(0.42, probs)
}

// answerAt answers the pick question with probs at confidence conf.
func answerAt(conf float64, probs map[string]float64) func(http.ResponseWriter, map[string]any) {
	return func(w http.ResponseWriter, _ map[string]any) {
		best, top := "", -1.0
		for k, p := range probs {
			if p > top {
				best, top = k, p
			}
		}
		ans, _ := json.Marshal(map[string]any{
			"model":   "typesafe/jev-1.13.0",
			"answers": map[string]any{"pick": map[string]any{"type": "choice", "choice": best, "confidence": conf, "probabilities": probs}},
			"usage":   map[string]any{"input_tokens": 900, "output_tokens": 30},
		})
		_, _ = w.Write(ans)
	}
}

func evenProbs(ds Dataset, top []Suggestion, override map[string]float64) map[string]float64 {
	names := map[int]string{}
	for _, h := range ds.Heroes {
		names[h.ID] = h.Name
	}
	probs := map[string]float64{}
	for _, s := range top {
		probs[names[s.HeroID]] = 0.01
	}
	for k, v := range override {
		probs[k] = v
	}
	return probs
}

func TestDecideReordersTheTop8ByJevsProbabilities(t *testing.T) {
	ds, d := pubMidDraft()
	ranked := Rank(ds, d, -1)
	// Code order of the top 8: 11 SF, 13 Puck, 17 Storm, 25 Lina, 34 Tinker, 39 QoP, 74 Invoker, 106 Ember.
	jev, _ := jevServer(t, answerWith(evenProbs(ds, ranked[:8], map[string]float64{"Invoker": 0.6, "Puck": 0.3})))
	got := Decide(context.Background(), jev, ds, d, ranked)
	if got.Source != SourceJev || got.Reason != "" || got.Confidence == nil || *got.Confidence != 0.42 {
		t.Fatalf("source %q reason %q confidence %v, want Jev's", got.Source, got.Reason, got.Confidence)
	}
	want := []int{74, 13, 11, 17, 25, 34, 39, 106} // Jev's two, then the ties in code order
	if g := ids(got.Suggestions[:8]); !slices.Equal(g, want) {
		t.Fatalf("top 8 %v, want %v", g, want)
	}
	if g := ids(got.Suggestions[8:]); !slices.Equal(g, ids(ranked[8:])) {
		t.Fatalf("after the top 8 %v, want the code order %v", g, ids(ranked[8:]))
	}
	if p := got.Suggestions[0].Probability; p == nil || *p != 0.6 {
		t.Fatalf("Invoker's probability %v, want 0.6", p)
	}
	if got.Suggestions[8].Probability != nil {
		t.Fatal("a hero Jev was not asked about has a probability")
	}
}

func TestDecideAsksOneChoiceWithTheDraftTheRoleAndEachCandidatesReasons(t *testing.T) {
	ds, d := pubMidDraft()
	ranked := Rank(ds, d, -1)
	jev, last := jevServer(t, answerWith(evenProbs(ds, ranked[:8], nil)))
	Decide(context.Background(), jev, ds, d, ranked)
	body := *last
	qs := body["questions"].(map[string]any)
	pick := qs["pick"].(map[string]any)
	if len(qs) != 1 || pick["type"] != "choice" {
		t.Fatalf("questions %v, want one choice", qs)
	}
	opts := pick["criteria"].(map[string]any)
	if len(opts) != 8 || opts["Shadow Fiend"] == nil || opts["Ember Spirit"] == nil || opts["Phoenix"] != nil {
		t.Fatalf("options %v, want the code's top 8 by name", opts)
	}
	sf, _ := json.Marshal(opts["Shadow Fiend"])
	for _, want := range []string{`"enemy":"Axe"`, `"delta":1.7`, `"games":200`, `"role_fit":1`, `"Carry"`, `"patch_win_rate"`} {
		if !strings.Contains(string(sf), want) {
			t.Errorf("Shadow Fiend's reasons %s lack %s", sf, want)
		}
	}
	state, _ := json.Marshal(body["state"])
	for _, want := range []string{`"enemies":["Axe","Crystal Maiden","Juggernaut","Slardar","Sniper"]`, `"number":2`, `"mid lane"`} {
		if !strings.Contains(string(state), want) {
			t.Errorf("state %s lacks %s", state, want)
		}
	}
}

func TestDecideFallsBackToTheCodeRankingOnATimeout(t *testing.T) {
	ds, d := pubMidDraft()
	ranked := Rank(ds, d, -1)
	jev, _ := jevServer(t, func(_ http.ResponseWriter, _ map[string]any) {
		time.Sleep(200 * time.Millisecond)
	})
	jev.Deadline, jev.HedgeAfter = 40*time.Millisecond, -1
	got := Decide(context.Background(), jev, ds, d, ranked)
	if got.Source != SourceRanking || got.Reason != ReasonTimeout || got.Confidence != nil {
		t.Fatalf("source %q reason %q, want the ranking labelled timeout", got.Source, got.Reason)
	}
	if !slices.Equal(ids(got.Suggestions), ids(ranked)) {
		t.Fatalf("order %v, want the code ranking %v", ids(got.Suggestions), ids(ranked))
	}
}

func TestDecideFallsBackToTheCodeRankingOnAnError(t *testing.T) {
	ds, d := pubMidDraft()
	ranked := Rank(ds, d, -1)
	jev, _ := jevServer(t, func(w http.ResponseWriter, _ map[string]any) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"model": "m", "answers": {}}`) // the pick is missing
	})
	got := Decide(context.Background(), jev, ds, d, ranked)
	if got.Source != SourceRanking || got.Reason != ReasonError || !slices.Equal(ids(got.Suggestions), ids(ranked)) {
		t.Fatalf("source %q reason %q order %v, want the code ranking labelled error", got.Source, got.Reason, ids(got.Suggestions))
	}
}

func TestDecideWithoutJevKeepsTheRankingLabelledUnavailable(t *testing.T) {
	ds, d := pubMidDraft()
	ranked := Rank(ds, d, -1)
	got := Decide(context.Background(), nil, ds, d, ranked)
	if got.Source != SourceRanking || got.Reason != ReasonUnavailable || !slices.Equal(ids(got.Suggestions), ids(ranked)) {
		t.Fatalf("source %q reason %q, want the ranking labelled unavailable", got.Source, got.Reason)
	}
	one := Decide(context.Background(), nil, ds, d, ranked[:1])
	if one.Source != SourceRanking || one.Reason != ReasonFewCandidates {
		t.Fatalf("one candidate: source %q reason %q, want few_candidates", one.Source, one.Reason)
	}
}

func TestDecideKeepsTheCodeOrderWhenJevIsBelowTheConfidenceFloor(t *testing.T) {
	ds, d := pubMidDraft()
	ranked := Rank(ds, d, -1)
	jev, _ := jevServer(t, answerAt(0.39, evenProbs(ds, ranked[:8], map[string]float64{"Invoker": 0.6})))
	got := Decide(context.Background(), jev, ds, d, ranked)
	if got.Source != SourceRanking || got.Reason != ReasonLowConfidence || got.Confidence == nil || *got.Confidence != 0.39 {
		t.Fatalf("source %q reason %q confidence %v, want the ranking kept for low confidence 0.39", got.Source, got.Reason, got.Confidence)
	}
	if g := ids(got.Suggestions); !slices.Equal(g, ids(ranked)) {
		t.Fatalf("order %v, want the code order %v", g, ids(ranked))
	}
	if got.Shadow != 74 {
		t.Fatalf("shadow pick %d, want Invoker (74), logged only", got.Shadow)
	}
	// At the floor, Jev's order stands.
	jev, _ = jevServer(t, answerAt(0.4, evenProbs(ds, ranked[:8], map[string]float64{"Invoker": 0.6})))
	if got := Decide(context.Background(), jev, ds, d, ranked); got.Source != SourceJev || got.Suggestions[0].HeroID != 74 {
		t.Fatalf("at 0.4: source %q first %d, want Jev's Invoker", got.Source, got.Suggestions[0].HeroID)
	}
}
