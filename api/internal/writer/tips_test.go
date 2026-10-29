package writer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// brief is Juggernaut for a safe lane carry against Axe and Lina.
func brief() Brief {
	return Brief{
		HeroID: 8, Hero: "Juggernaut", Lang: "en", Position: 1, PositionName: "safe lane carry",
		Roles: []string{"Carry", "Pusher"}, RoleFit: 1, WinRate: 51.3,
		Enemies: []Enemy{{Name: "Axe", Delta: 5, Games: 4000}, {Name: "Lina", Delta: -1.2, Games: 3000}},
		Allowed: []string{"Juggernaut", "Axe", "Lina"},
		Known:   []string{"Juggernaut", "Axe", "Lina", "Earthshaker", "Anti-Mage", "Blade Fury", "Mana Break"},
	}
}

// openRouter answers chat completions with text and keeps every body it got.
type openRouter struct {
	mu     sync.Mutex
	bodies []map[string]any
}

func (o *openRouter) serve(t *testing.T, text string, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		o.mu.Lock()
		o.bodies = append(o.bodies, body)
		o.mu.Unlock()
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "openai/gpt-6-luna-20261001",
			"choices": []any{map[string]any{"message": map[string]any{"content": text}}},
			"usage":   map[string]any{"prompt_tokens": 210, "completion_tokens": 30, "cost": 0.00003},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

type callLog struct {
	mu    sync.Mutex
	calls []Call
}

func (l *callLog) Record(_ context.Context, c Call) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, c)
}

func newWriter(t *testing.T, text string, delay time.Duration) (*Writer, *openRouter, *callLog) {
	t.Helper()
	or := &openRouter{}
	srv := or.serve(t, text, delay)
	log := &callLog{}
	w := New(srv.URL, "test-key", "openai/gpt-6-luna", log)
	w.HTTP = srv.Client()
	return w, or, log
}

func TestTipFromTheWriterStandsWhenItNamesOnlyTheResult(t *testing.T) {
	w, or, log := newWriter(t, "Juggernaut wins 5 points more against Axe; Lina is close to even.", 0)
	tip := w.Tip(context.Background(), brief())
	if tip.Source != SourceWriter || tip.Text != "Juggernaut wins 5 points more against Axe; Lina is close to even." || tip.HeroID != 8 {
		t.Fatalf("tip %+v, want the writer's text", tip)
	}
	if len(or.bodies) != 1 || or.bodies[0]["model"] != "openai/gpt-6-luna" {
		t.Fatalf("requests %v", or.bodies)
	}
	prompt, _ := json.Marshal(or.bodies[0]["messages"])
	for _, want := range []string{"Juggernaut", "Axe", "safe lane carry", "BCP 47", "en"} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("prompt lacks %q: %s", want, prompt)
		}
	}
	if len(log.calls) != 1 || log.calls[0].Outcome != OutcomeOK || log.calls[0].Purpose != "draft.tip" ||
		log.calls[0].Cost == nil || *log.calls[0].Cost != 0.00003 || log.calls[0].InputTokens != 210 {
		t.Fatalf("logged %+v", log.calls)
	}
}

func TestTipNamingAnUnknownHeroOrItemIsReplacedByATemplate(t *testing.T) {
	for name, text := range map[string]string{
		"a hero of the patch not in the reasons": "Juggernaut beats Axe, and Earthshaker cannot stop him.",
		"a hero not on the patch":                "Juggernaut beats Axe, much like Mirana does.",
		"an item, not in the database":           "Juggernaut beats Axe; rush a Battle Fury.",
		"an ability, at a sentence's start":      "Blade Fury shreds Axe. Pick Juggernaut.",
		"an ability of the patch":                "Juggernaut beats Axe with Mana Break.",
	} {
		t.Run(name, func(t *testing.T) {
			w, _, log := newWriter(t, text, 0)
			tip := w.Tip(context.Background(), brief())
			if tip.Source != SourceTemplate || tip.Text != "" || tip.Reason != ReasonRejected || tip.HeroID != 8 {
				t.Fatalf("tip %+v, want a template for %q", tip, text)
			}
			if len(log.calls) != 1 || log.calls[0].Outcome != OutcomeRejected || log.calls[0].Output != text {
				t.Fatalf("logged %+v, want the rejected text", log.calls)
			}
		})
	}
}

func TestCheckAllowsTheResultsNamesAndPlainSentences(t *testing.T) {
	b := brief()
	for _, text := range []string{
		"Juggernaut beats Axe by 5 points.",
		"Juggernaut vai bem contra Axe: +5 pontos em 4.000 partidas.",
		"Juggernaut's spin beats Axe, and Lina is close to even.",
	} {
		if err := Check(text, b.Allowed, b.Known); err != nil {
			t.Errorf("%q: %v", text, err)
		}
	}
	for _, text := range []string{
		"",
		strings.Repeat("a", MaxTip+1),
		"Juggernaut is strong in the MMR climb.",
		"Juggernaut. Earthshaker is weak.",
		"Mirana is weaker than Juggernaut here.",
		"Juggernaut beats Axe. Mirana does too.",
		"Juggernaut beats Axe: Battle Fury helps.",
		"Juggernaut beats Axe with mana break.",
	} {
		if err := Check(text, b.Allowed, b.Known); err == nil {
			t.Errorf("%q passed", text)
		}
	}
}

func TestTipFailsOpenToATemplateWhenTheWriterIsLate(t *testing.T) {
	w, _, log := newWriter(t, "Juggernaut beats Axe.", time.Second)
	w.Timeout = 50 * time.Millisecond
	start := time.Now()
	tip := w.Tip(context.Background(), brief())
	if tip.Source != SourceTemplate || tip.Reason != ReasonTimeout {
		t.Fatalf("tip %+v, want a timeout template", tip)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("waited %v past a 50 ms deadline", time.Since(start))
	}
	if len(log.calls) != 1 || log.calls[0].Outcome != OutcomeTimeout {
		t.Fatalf("logged %+v", log.calls)
	}
}

func TestTipFailsOpenToATemplateOnAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"no provider"}}`, http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	log := &callLog{}
	w := New(srv.URL, "test-key", "openai/gpt-6-luna", log)
	w.HTTP = srv.Client()
	if tip := w.Tip(context.Background(), brief()); tip.Source != SourceTemplate || tip.Reason != ReasonError {
		t.Fatalf("tip %+v, want an error template", tip)
	}
	if len(log.calls) != 1 || log.calls[0].Outcome != OutcomeError || strings.Contains(log.calls[0].Error, "test-key") {
		t.Fatalf("logged %+v", log.calls)
	}
}
