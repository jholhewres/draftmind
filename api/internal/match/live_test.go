package match

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/items"
)

// stubJev answers the live question with event at conf, or fails.
type stubJev struct {
	event string
	conf  float64
	err   error
	asked int
}

func (j *stubJev) Ask(_ context.Context, req decision.Request) (decision.Result, error) {
	j.asked++
	if j.err != nil {
		return decision.Result{}, j.err
	}
	if _, ok := req.Questions["event"]; !ok {
		return decision.Result{}, errors.New("no event question")
	}
	cost := 0.0012
	return decision.Result{Answers: map[string]decision.Answer{
		"event": {Type: "choice", Choice: j.event, Confidence: j.conf, Probabilities: map[string]float64{j.event: j.conf}},
	}, Usage: decision.Usage{Cost: &cost}}, nil
}

func frame() LiveFrame { return LiveFrame{Format: "jpg", Data: []byte{0xff, 0xd8, 0xff, 0xd9}} }

func TestJevDecidesSilentOrSpeakWithAnEventTypeAndConfidence(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, tc := range []struct {
		name   string
		jev    *stubJev
		speak  bool
		event  string
		reason string
	}{
		{"silent posts nothing", &stubJev{event: EventSilent, conf: 0.9}, false, "", LiveReasonSilent},
		{"a failed call posts nothing", &stubJev{err: errors.New("boom")}, false, "", LiveReasonError},
		{"a late call posts nothing", &stubJev{err: context.DeadlineExceeded}, false, "", LiveReasonError},
		{"a low-confidence speak posts nothing", &stubJev{event: EventItemTiming, conf: 0.3}, false, "", LiveReasonUnsure},
		{"a confident speak names its event", &stubJev{event: EventItemTiming, conf: 0.8}, true, EventItemTiming, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := NewLive(tc.jev)
			d := l.Tick(context.Background(), session(), frame(), now)
			if d.Speak != tc.speak || d.Event != tc.event || d.Reason != tc.reason {
				t.Fatalf("got speak=%v event=%q reason=%q, want %v %q %q", d.Speak, d.Event, d.Reason, tc.speak, tc.event, tc.reason)
			}
			if tc.jev.asked != 1 {
				t.Fatalf("jev asked %d times, want 1", tc.jev.asked)
			}
			if tc.speak && (d.Confidence != 0.8 || d.Cost == nil) {
				t.Fatalf("speak without its confidence or cost: %+v", d)
			}
		})
	}
}

func TestASpokenEventIsAWrittenTemplateFromTheItemPlanWithItsCard(t *testing.T) {
	l := NewLive(&stubJev{event: EventItemTiming, conf: 0.9})
	d := l.Tick(context.Background(), session(), frame(), time.Unix(1_800_000_000, 0))
	r := d.Reply
	if r.Kind != "answer" || r.Source != SourceLive || r.Category != "items" || r.Reason != LiveReasonTemplate+":"+EventItemTiming {
		t.Fatalf("reply %+v, want a live items template", r)
	}
	// The next item comes from the session's plan: the soon situational item
	// first, never one the plan lacks.
	if len(r.Items) != 1 || r.Items[0].Name != "Black King Bar" {
		t.Fatalf("items %+v, want Black King Bar from the plan", r.Items)
	}
	// A fight event with no plan item names none.
	s := session()
	s.ItemPlan = items.Build{}
	d = NewLive(&stubJev{event: EventItemTiming, conf: 0.9}).Tick(context.Background(), s, frame(), time.Unix(1_800_000_000, 0))
	if d.Speak {
		t.Fatalf("an item event with no item in the plan spoke: %+v", d)
	}
}

func TestCooldownDedupeAndThePerMinuteCapStopRepeatsWithoutAskingJev(t *testing.T) {
	jev := &stubJev{event: EventItemTiming, conf: 0.9}
	l := NewLive(jev)
	t0 := time.Unix(1_800_000_000, 0)
	s := session()
	if d := l.Tick(context.Background(), s, frame(), t0); !d.Speak {
		t.Fatalf("first tick did not speak: %+v", d)
	}
	// The same event within the cooldown: Jev answers it again, the code drops it.
	if d := l.Tick(context.Background(), s, frame(), t0.Add(30*time.Second)); d.Speak || d.Reason != LiveReasonCooldown {
		t.Fatalf("repeat within cooldown: %+v", d)
	}
	// Another event type may speak, but the per-minute cap holds it.
	jev.event = EventDeaths
	if d := l.Tick(context.Background(), s, frame(), t0.Add(40*time.Second)); d.Speak || d.Reason != LiveReasonCap {
		t.Fatalf("second event inside the minute: %+v", d)
	}
	// Ticks closer than the interval are skipped before any Jev call.
	asked := jev.asked
	if d := l.Tick(context.Background(), s, frame(), t0.Add(45*time.Second)); d.Reason != LiveReasonTooSoon || jev.asked != asked {
		t.Fatalf("a tick 5 s after the last asked Jev: %+v asked %d→%d", d, asked, jev.asked)
	}
	if d := l.Tick(context.Background(), s, frame(), t0.Add(2*time.Minute)); !d.Speak || d.Event != EventDeaths {
		t.Fatalf("a new event after the minute: %+v", d)
	}
	// The first event speaks again once its cooldown is over.
	jev.event = EventItemTiming
	if d := l.Tick(context.Background(), s, frame(), t0.Add(LiveCooldown+time.Second)); !d.Speak {
		t.Fatalf("after cooldown: %+v", d)
	}
	// A finished match never asks.
	s.Result = &Result{Outcome: "win"}
	asked = jev.asked
	if d := l.Tick(context.Background(), s, frame(), t0.Add(time.Hour)); d.Reason != LiveReasonEnded || jev.asked != asked {
		t.Fatalf("finished match: %+v", d)
	}
}

func TestTurboHalvesTheCooldown(t *testing.T) {
	jev := &stubJev{event: EventItemTiming, conf: 0.9}
	l := NewLive(jev)
	s := session()
	s.Mode = "turbo"
	t0 := time.Unix(1_800_000_000, 0)
	l.Tick(context.Background(), s, frame(), t0)
	if d := l.Tick(context.Background(), s, frame(), t0.Add(LiveCooldown/2+time.Second)); !d.Speak {
		t.Fatalf("turbo after half the cooldown: %+v", d)
	}
}
