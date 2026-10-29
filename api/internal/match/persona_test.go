package match

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/persona"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

// askLog keeps every request Jev is asked, as JSON.
type askLog struct {
	jev
	asked []string
}

func (a *askLog) Ask(ctx context.Context, req decision.Request) (decision.Result, error) {
	b, _ := json.Marshal(req)
	a.asked = append(a.asked, string(b))
	return a.jev.Ask(ctx, req)
}

func TestTwoPersonasGivenTheSameMatchDecideTheSameAdvice(t *testing.T) {
	blunt := persona.Persona{Tone: "direct", Language: "en", Verbosity: "short", Focus: "laning"}
	warm := persona.Persona{Tone: "encouraging", Language: "pt-BR", Verbosity: "normal", Focus: "macro"}
	var replies []Reply
	var prompts []string
	j := &askLog{}
	for _, p := range []persona.Persona{blunt, warm} {
		m := &model{parse: laneParse, answer: "Stay behind your creeps."}
		c := &Coach{Parse: m.server(t), Jev: j, Facts: fakeFacts{}}
		c.Write = c.Parse.(*writer.Writer)
		r, err := c.Reply(context.Background(), session(), "viper is killing me mid", "en", p, (&events{}).emit)
		if err != nil {
			t.Fatal(err)
		}
		replies, prompts = append(replies, r), append(prompts, m.lastAnswerBody)
	}

	// The decision is the same: Jev was asked the same question and answered alike.
	if len(j.asked) != 2 {
		t.Fatalf("Jev asked %d times, want 2", len(j.asked))
	}
	if j.asked[0] != j.asked[1] {
		t.Fatalf("Jev was asked differently by persona:\n%s\n%s", j.asked[0], j.asked[1])
	}
	a, b := replies[0], replies[1]
	if a.Category != b.Category || a.Priority != b.Priority || a.Source != b.Source || a.Category != "lane" {
		t.Fatalf("decided %s/%s by %s and %s/%s by %s; want the same", a.Category, a.Priority, a.Source, b.Category, b.Priority, b.Source)
	}
	// Only the writer's prompt carries the persona.
	for i, want := range []string{"Be direct", "Be encouraging"} {
		if !strings.Contains(prompts[i], want) {
			t.Errorf("writer prompt %d lacks %q: %s", i, want, prompts[i])
		}
	}
	if !strings.Contains(prompts[0], "One or two short sentences") || !strings.Contains(prompts[1], "map play") ||
		!strings.Contains(prompts[0], "BCP 47 tag is en.") || !strings.Contains(prompts[1], "BCP 47 tag is pt-BR.") ||
		!strings.Contains(prompts[1], "(pt-BR)") {
		t.Errorf("writer prompts lack the verbosity, focus or language:\n%s\n%s", prompts[0], prompts[1])
	}
}
