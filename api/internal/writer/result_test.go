package writer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAMessageIsParsedIntoATypedResult(t *testing.T) {
	for _, c := range []struct {
		answer string
		want   string
		err    error
	}{
		{`{"result": "loss"}`, "loss", nil},
		{"```json\n{\"result\": \"win\"}\n```", "win", nil},
		{`{"result": "unclear"}`, "", ErrUnclear},
		{`{"result": "draw"}`, "", ErrUnclear},
		{`not json`, "", ErrUnclear},
	} {
		w, or, log := newWriter(t, c.answer, 0)
		got, err := w.ParseResult(context.Background(), "perdemos")
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("answer %q: %q, %v; want %q, %v", c.answer, got, err, c.want, c.err)
		}
		if prompt, _ := json.Marshal(or.bodies[0]["messages"]); !strings.Contains(string(prompt), "perdemos") {
			t.Errorf("prompt lacks the message: %s", prompt)
		}
		if len(log.calls) != 1 || log.calls[0].Purpose != "match.result" {
			t.Errorf("logged %+v", log.calls)
		}
	}
}

func TestNotesComeOnlyFromTheSessionAndPassTheEntityCheck(t *testing.T) {
	answer := `{"notes": [
		"Axe won the lane until you stayed behind your creeps.",
		"Black King Bar turned the fights against Axe.",
		"Next time buy Divine Rapier early.",
		"Earthshaker caught you twice in the river."
	]}`
	w, or, log := newWriter(t, answer, 0)
	b := NotesBrief{
		Lang: "en", Result: "win",
		Messages: []string{"player: axe keeps calling me in lane", "coach: Stay behind your creeps."},
		Facts:    map[string]any{"you": map[string]any{"hero": "Juggernaut"}, "enemies": []string{"Axe"}},
		Allowed:  []string{"Juggernaut", "Axe", "Black King Bar"},
		Known:    []string{"Juggernaut", "Axe", "Earthshaker", "Black King Bar", "Divine Rapier"},
	}
	n := w.Notes(context.Background(), b)
	want := []string{"Axe won the lane until you stayed behind your creeps.", "Black King Bar turned the fights against Axe."}
	if n.Source != SourceWriter || strings.Join(n.Items, "|") != strings.Join(want, "|") {
		t.Fatalf("notes %+v, want the two that name only the session's facts", n)
	}
	prompt, _ := json.Marshal(or.bodies[0]["messages"])
	for _, s := range []string{"axe keeps calling me in lane", "Stay behind your creeps.", "win", "Juggernaut"} {
		if !strings.Contains(string(prompt), s) {
			t.Errorf("prompt lacks %q", s)
		}
	}
	if len(log.calls) != 1 || log.calls[0].Purpose != "match.notes" || log.calls[0].Outcome != OutcomeOK {
		t.Fatalf("logged %+v", log.calls)
	}

	// Every note failing the check leaves none, labelled as such.
	w, _, _ = newWriter(t, `{"notes": ["Buy Divine Rapier."]}`, 0)
	if n := w.Notes(context.Background(), b); n.Source != SourceTemplate || len(n.Items) != 0 || n.Reason != ReasonRejected {
		t.Fatalf("notes %+v, want none, rejected", n)
	}
}
