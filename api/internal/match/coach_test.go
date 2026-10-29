package match

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/items"
	"github.com/jholhewres/draftmind/api/internal/persona"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// model is an OpenRouter stand-in: a JSON request (the parse) gets parse,
// any other (the answer) gets answer. It counts both.
type model struct {
	parse, answer  string
	mu             sync.Mutex
	parses, writes int
	lastAnswerBody string
}

func (m *model) server(t *testing.T) *writer.Writer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		text := m.answer
		if body["response_format"] != nil {
			m.parses++
			text = m.parse
		} else {
			m.writes++
			b, _ := json.Marshal(body["messages"])
			m.lastAnswerBody = string(b)
		}
		m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "openai/gpt-6-luna-20261001",
			"choices": []any{map[string]any{"message": map[string]any{"content": text}}},
			"usage":   map[string]any{"prompt_tokens": 400, "completion_tokens": 60, "cost": 0.0001},
		})
	}))
	t.Cleanup(srv.Close)
	w := writer.New(srv.URL, "test-key", "openai/gpt-6-luna", nil)
	w.HTTP = srv.Client()
	return w
}

// fakeFacts: Puck (13) mid against Viper (47) and Pudge (14), on the current patch.
type fakeFacts struct{}

var heroes = []catalog.Hero{{ID: 13, Slug: "puck", Name: "Puck"}, {ID: 14, Slug: "pudge", Name: "Pudge"}, {ID: 47, Slug: "viper", Name: "Viper"}}

func (fakeFacts) Heroes(context.Context) (string, []catalog.Hero, error) { return "7.41", heroes, nil }

func (fakeFacts) HeroDetail(_ context.Context, slug, lang string) (catalog.HeroDetail, error) {
	abilities := map[string][]catalog.Ability{
		"puck":  {{Name: "Illusory Orb"}, {Name: "Phase Shift", Description: "Puck briefly shifts out of the world."}},
		"viper": {{Name: "Poison Attack"}, {Name: "Viper Strike", Description: "Slows and poisons a target (" + lang + ")."}},
		"pudge": {{Name: "Meat Hook"}, {Name: "Rot"}},
	}
	return catalog.HeroDetail{Abilities: abilities[slug]}, nil
}

func (fakeFacts) DraftDataset(context.Context, []int) (string, draft.Dataset, error) {
	return "7.41", draft.Dataset{Matchups: map[draft.Pair]draft.Record{{Hero: 13, Opponent: 47}: {Games: 1000, Wins: 440}}}, nil
}

func (fakeFacts) Names(context.Context) (catalog.Names, error) {
	return catalog.Names{
		Heroes:    map[int][]string{13: {"Puck"}, 14: {"Pudge"}, 47: {"Viper"}},
		Abilities: []string{"Illusory Orb", "Phase Shift", "Poison Attack", "Viper Strike", "Meat Hook", "Rot"},
		Items:     []string{"Bottle", "Black King Bar", "Divine Rapier", "Witch Blade"},
		OffPatch:  []string{"Old Hero"},
	}, nil
}

// jev picks the items category at high priority, or fails.
type jev struct{ asked int }

func (j *jev) Ask(context.Context, decision.Request) (decision.Result, error) {
	j.asked++
	return decision.Result{Answers: map[string]decision.Answer{
		"category": {Type: "choice", Choice: "lane", Confidence: 0.8},
		"priority": {Type: "score", Probabilities: map[string]float64{"0": 0.1, "1": 0.2, "2": 0.7}, Confidence: 0.8},
	}}, nil
}

func session() Session {
	return Session{ID: "s", HeroID: 13, Role: 2, Enemies: []int{47, 14}, Allies: []int{}, ItemPlan: items.Build{
		Core:        []items.PhaseItems{{Phase: "mid", Items: []items.ItemCount{{Item: items.Item{ID: 1, Name: "Witch Blade"}}}}},
		Situational: []items.Situational{{Item: items.Item{ID: 116, Name: "Black King Bar"}, EnemyID: 47, Urgency: "soon"}},
	}}
}

type events struct{ names []string }

func (e *events) emit(name string, _ any) { e.names = append(e.names, name) }

const laneParse = `{"clear": true, "category": "lane", "entities": ["Viper"], "summary": "Viper is winning the mid lane."}`

func TestAnswersNameOnlyHeroesAbilitiesAndItemsInTheDatabaseForTheCurrentPatch(t *testing.T) {
	// The player names an item the patch lacks; the parse passes it on.
	const oldItemParse = `{"clear": true, "category": "lane", "entities": ["Viper", "Necronomicon"], "summary": "Viper wins the lane."}`
	for _, tc := range []struct {
		name, parse, answer, source, reason string
	}{
		{"names from the facts stand", laneParse, "Stay behind your creeps: Viper wins trades until level 6. Keep Phase Shift for Viper Strike and buy Black King Bar soon.", writer.SourceWriter, ""},
		{"an item the facts lack fails", laneParse, "Stay back and farm a Divine Rapier.", writer.SourceTemplate, writer.ReasonRejected},
		{"a name the database lacks fails", laneParse, "Stay back and buy a Glimmer Cape before you fight.", writer.SourceTemplate, writer.ReasonRejected},
		{"an unknown name the player wrote fails at a sentence start", oldItemParse, "Viper is strong. Necronomicon helps you push.", writer.SourceTemplate, writer.ReasonRejected},
		{"an unknown name the player wrote fails in lower case", oldItemParse, "Viper is strong, so a necronomicon helps you push.", writer.SourceTemplate, writer.ReasonRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &model{parse: tc.parse, answer: tc.answer}
			c := &Coach{Parse: m.server(t), Write: nil, Jev: &jev{}, Facts: fakeFacts{}}
			c.Write = c.Parse.(*writer.Writer)
			ev := &events{}
			r, err := c.Reply(context.Background(), session(), "viper is killing me mid", "en", persona.Default(), ev.emit)
			if err != nil {
				t.Fatal(err)
			}
			if r.Kind != "answer" || r.TextSource != tc.source || r.Reason != tc.reason || r.Category != "lane" || r.Priority != "high" || r.Source != SourceJev {
				t.Fatalf("reply %+v, want an answer from %s (%q) at lane/high from Jev", r, tc.source, tc.reason)
			}
			if tc.source == writer.SourceWriter && r.Text != tc.answer {
				t.Fatalf("text %q", r.Text)
			}
			if tc.source == writer.SourceTemplate && r.Text != "" {
				t.Fatalf("a rejected text reached the player: %q", r.Text)
			}
			if strings.Join(ev.names, ",") != "situation,decision,answer" {
				t.Fatalf("events %v", ev.names)
			}
		})
	}
}

func TestAMessageThatCannotBeParsedGetsAFollowUpQuestionInsteadOfAGuess(t *testing.T) {
	for _, parse := range []string{
		`{"clear": false, "category": "other", "entities": [], "summary": ""}`,
		`I think the player is losing lane`,
		`{"clear": true, "category": "jungle", "entities": [], "summary": "x"}`,
	} {
		m := &model{parse: parse, answer: "Buy a Bottle."}
		j := &jev{}
		c := &Coach{Parse: m.server(t), Jev: j, Facts: fakeFacts{}}
		c.Write = c.Parse.(*writer.Writer)
		ev := &events{}
		r, err := c.Reply(context.Background(), session(), "hm", "en", persona.Default(), ev.emit)
		if err != nil {
			t.Fatal(err)
		}
		if r.Kind != "followup" || r.Reason != "unclear" || r.Text != "" {
			t.Fatalf("parse %q: reply %+v, want a follow-up", parse, r)
		}
		if m.writes != 0 || j.asked != 0 || strings.Join(ev.names, ",") != "followup" {
			t.Fatalf("parse %q: %d answers written, Jev asked %d times, events %v; want none but the follow-up", parse, m.writes, j.asked, ev.names)
		}
	}
}
