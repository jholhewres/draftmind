package gsi

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func load(t *testing.T, name string) State {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	s, token, err := Parse(b)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if token != "TOKEN" {
		t.Fatalf("%s: token %q", name, token)
	}
	return s
}

func TestParseReadsThePlayersOwnStateFromRecordedPayloads(t *testing.T) {
	s := load(t, "in_progress.json")
	if s.Phase != PhaseGame || s.Clock != 845 || s.Gold != 2310 || s.Level != 11 || s.Kills != 3 || s.Deaths != 2 || s.Assists != 5 ||
		s.Team != "radiant" || s.MatchID != "8123456789" || !s.Alive || s.BuybackCost != 790 || s.Mode != "" {
		t.Fatalf("in progress: %+v", s)
	}
	// Item keys without the item_ prefix, inventory slots only, no empties.
	if !slices.Equal(s.Items, []string{"power_treads", "magic_wand"}) {
		t.Fatalf("items %v", s.Items)
	}
	end := load(t, "post_game.json")
	if end.Phase != PhaseEnded || end.Winner != "radiant" || end.Kills != 12 || end.DurationS != 2410 {
		t.Fatalf("post game: %+v", end)
	}
	if out, ok := end.Outcome(); !ok || out != "win" {
		t.Fatalf("outcome %q %v, want win", out, ok)
	}
	if out, ok := s.Outcome(); ok {
		t.Fatalf("a game in progress has no outcome, got %q", out)
	}
	turbo := load(t, "turbo_dead.json")
	if turbo.Mode != "turbo" || turbo.Alive || turbo.Team != "dire" {
		t.Fatalf("turbo: %+v", turbo)
	}
	if pick := load(t, "hero_selection.json"); pick.Phase != PhaseDraft {
		t.Fatalf("hero selection: %+v", pick)
	}
	if _, _, err := Parse([]byte(`{"map": {}}`)); err == nil {
		t.Fatal("a payload without auth parsed")
	}
	if _, _, err := Parse([]byte(`not json`)); err == nil {
		t.Fatal("garbage parsed")
	}
}

func TestTriggersFireOnGoldForTheNextItemDeathsPhaseAndBuyback(t *testing.T) {
	plan := Plan{Next: []PlanItem{{Key: "bfury", Name: "Battle Fury", Cost: 4100}, {Key: "black_king_bar", Name: "Black King Bar", Cost: 4050}}}
	cur := load(t, "in_progress.json")

	// Not enough gold for Battle Fury yet: nothing.
	if got := Triggers(nil, cur, plan); len(got) != 0 {
		t.Fatalf("2310 gold fired %v", got)
	}
	// Gold crosses the next item's cost: one item trigger, naming it.
	rich := cur
	rich.Gold = 4200
	got := Triggers(&cur, rich, plan)
	if len(got) != 1 || got[0].Event != EventItem || got[0].Item == nil || got[0].Item.Key != "bfury" {
		t.Fatalf("gold for the next item: %+v", got)
	}
	// An item already owned is skipped for the next one.
	owned := rich
	owned.Items = append(owned.Items, "bfury")
	if got := Triggers(&cur, owned, plan); len(got) != 1 || got[0].Item.Key != "black_king_bar" {
		t.Fatalf("owned item not skipped: %+v", got)
	}
	// Staying rich does not fire again.
	if got := Triggers(&rich, rich, plan); len(got) != 0 {
		t.Fatalf("still rich fired %v", got)
	}

	// Two deaths within the window: a deaths trigger; buyback is up while dead.
	dead := cur
	dead.Deaths, dead.Alive, dead.Clock, dead.Gold = 4, false, 900, 1000
	prev := cur
	prev.Deaths, prev.Clock = 3, 860
	events := map[string]bool{}
	for _, tr := range Triggers(&prev, dead, plan) {
		events[tr.Event] = true
	}
	if !events[EventDeaths] || !events[EventBuyback] {
		t.Fatalf("death with buyback gold: %v", events)
	}
	// A single death long after the last one is not "repeated".
	once := cur
	once.Deaths, once.Clock = 3, 1800
	early := cur
	early.Deaths, early.Clock, early.LastDeathClock = 2, 1790, 300
	for _, tr := range Triggers(&early, once, plan) {
		if tr.Event == EventDeaths {
			t.Fatalf("a lone death fired deaths")
		}
	}

	// Phase: the laning stage ends at 10 min, 5 min in Turbo.
	lane := cur
	lane.Clock = 590
	later := cur
	later.Clock = 610
	if !hasEvent(Triggers(&lane, later, Plan{}), EventPhase) {
		t.Fatal("laning end not fired at 10:00")
	}
	tl, tn := lane, later
	tl.Mode, tn.Mode, tl.Clock, tn.Clock = "turbo", "turbo", 290, 310
	if !hasEvent(Triggers(&tl, tn, Plan{}), EventPhase) {
		t.Fatal("turbo laning end not fired at 5:00")
	}
}

func hasEvent(ts []Trigger, e string) bool {
	return slices.ContainsFunc(ts, func(t Trigger) bool { return t.Event == e })
}
