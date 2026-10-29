package draft

import (
	"slices"
	"testing"
)

func TestModesRankTheirOwnPool(t *testing.T) {
	ds := sixHeroes()
	// Single and Random Draft rank only the offered heroes, minus taken ones.
	for _, m := range []Mode{SingleDraft, RandomDraft, CaptainsDraft} {
		got := ids(Rank(ds, Draft{Mode: m, Pool: []int{5, 4, 1}, Allies: []int{1}, Enemies: []int{2}, Role: 2}, 10))
		if !slices.Equal(got, []int{4, 5}) {
			t.Errorf("%s ranks %v, want only the offered 4 and 5 (1 is taken)", m, got)
		}
	}
	// All Pick ignores a pool.
	if got := Rank(ds, Draft{Mode: AllPick, Pool: []int{4}, Enemies: []int{2}, Role: 2}, 10); len(got) != 5 {
		t.Errorf("All Pick ranks %d heroes, want every legal one", len(got))
	}
	// All Random suggests nobody.
	if got := Rank(ds, Draft{Mode: AllRandom, Enemies: []int{2}, Role: 2}, 10); len(got) != 0 {
		t.Errorf("All Random ranks %v, want none", ids(got))
	}
}

func TestCaptainsModeSuggestsBansThatBeatOurAllies(t *testing.T) {
	ds := sixHeroes()
	// Hero 4 beats hero 2 60-40; hero 5 loses to it 40-60. With 2 on our
	// side, 4 is the ban to make and 5 the last.
	d := Draft{Mode: CaptainsMode, Allies: []int{2}, Bans: []int{3}, Role: 1}
	got := Bans(ds, d, 3)
	if len(got) != 3 || got[0].HeroID != 4 || got[0].Counter != 5 || slices.Contains(ids(got), 3) || slices.Contains(ids(got), 2) {
		t.Fatalf("bans %+v, want 4 first at +5 against our ally, never 2 or 3", got)
	}
	if b := Bans(ds, Draft{Mode: AllPick, Allies: []int{2}, Role: 1}, 3); b != nil {
		t.Fatalf("All Pick suggests bans %v", ids(b))
	}
}

func TestTurboRanksOnTurboWinRatesWhenTheDataHasThem(t *testing.T) {
	ds := sixHeroes()
	ds.Stats = map[int]Record{1: {Games: 1000, Wins: 500}, 3: {Games: 1000, Wins: 500}}
	ds.TurboStats = map[int]Record{3: {Games: 1000, Wins: 600}}
	got := Rank(ds, Draft{Mode: Turbo, Role: 1}, 2)
	if got[0].HeroID != 3 || StatsFor(ds, Turbo) != StatsTurbo {
		t.Fatalf("turbo ranks %v from %s, want 3 first from turbo stats", ids(got), StatsFor(ds, Turbo))
	}
	if got := Rank(ds, Draft{Mode: AllPick, Role: 1}, 2); got[0].HeroID != 1 {
		t.Fatalf("all pick ranks %v, want the ranked tie broken by id", ids(got))
	}
	ds.TurboStats = nil
	if StatsFor(ds, Turbo) != StatsRanked {
		t.Fatal("turbo without turbo data must say it uses the ranked data")
	}
}

func TestModesParseAndRefuseTheUnsupported(t *testing.T) {
	for _, s := range []string{"", "all_pick", "all_pick_ranked", "turbo", "captains_mode", "captains_draft", "single_draft", "random_draft", "all_random"} {
		if _, err := ParseMode(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"ability_draft", "custom", "x"} {
		if _, err := ParseMode(s); err == nil {
			t.Errorf("%q parsed", s)
		}
	}
	if m, _ := ParseMode(""); m != AllPickRanked {
		t.Errorf("empty mode is %s, want all_pick_ranked", m)
	}
	if TimeScale(Turbo) >= 1 || TimeScale(AllPick) != 1 || GoldScale(Turbo) <= 1 {
		t.Error("Turbo must run faster and richer than All Pick")
	}
}
