package draft

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestRankShrinksMatchupWinRateTowardHalfByGamesPlayed(t *testing.T) {
	cases := []struct {
		wins, games int
		want        float64
	}{
		{0, 0, 0.5},               // no games: a coin flip
		{6, 10, 56.0 / 110},       // 60% on 10 games barely moves
		{60, 100, 0.55},           // 60% on 100 games keeps half the edge
		{600, 1000, 650.0 / 1100}, // 60% on 1000 games keeps most of it
	}
	for _, c := range cases {
		if got := ShrunkWinRate(c.wins, c.games); !near(got, c.want) {
			t.Errorf("ShrunkWinRate(%d, %d) = %v, want %v", c.wins, c.games, got, c.want)
		}
	}
}

// sixHeroes is a worked example: hero 4 beats hero 2 60-40 over 100 games
// (shrunk to 55 %, +5.0), hero 5 loses 40-60 (-5.0), hero 6 has no games.
func sixHeroes() Dataset {
	return Dataset{
		Heroes: []Hero{
			{ID: 1, Roles: []string{"Carry"}},
			{ID: 2, Roles: []string{"Support"}},
			{ID: 3, Roles: []string{"Carry"}},
			{ID: 4, Roles: []string{"Nuker"}},
			{ID: 5, Roles: []string{"Initiator"}},
			{ID: 6, Roles: []string{"Carry"}},
		},
		Matchups: map[Pair]Record{
			{Hero: 4, Opponent: 2}: {Games: 100, Wins: 60},
			{Hero: 5, Opponent: 2}: {Games: 100, Wins: 40},
		},
	}
}

func ids(s []Suggestion) []int {
	out := make([]int, len(s))
	for i, x := range s {
		out[i] = x.HeroID
	}
	return out
}

func TestRankNeverSuggestsAPickedOrBannedHero(t *testing.T) {
	got := Rank(sixHeroes(), Draft{Allies: []int{1}, Enemies: []int{2}, Bans: []int{3}, Role: 3}, 10)
	seen := map[int]bool{}
	for _, id := range ids(got) {
		seen[id] = true
	}
	if len(got) != 3 || !seen[4] || !seen[5] || !seen[6] {
		t.Fatalf("suggested %v, want exactly heroes 4, 5 and 6", ids(got))
	}
}

func TestRankListsTheCounterDeltaPerEnemyInTheDraftOrder(t *testing.T) {
	ds := sixHeroes()
	ds.Matchups[Pair{Hero: 4, Opponent: 1}] = Record{Games: 10, Wins: 4} // (4+50)/110 = 49.09 %
	got := Rank(ds, Draft{Enemies: []int{2, 1}, Role: 2}, 10)
	var four Suggestion
	for _, s := range got {
		if s.HeroID == 4 {
			four = s
		}
	}
	want := []EnemyDelta{{HeroID: 2, Delta: 5, Games: 100}, {HeroID: 1, Delta: 5400.0/110 - 50, Games: 10}}
	if len(four.Enemies) != 2 || four.Enemies[0].HeroID != 2 || four.Enemies[1].HeroID != 1 {
		t.Fatalf("hero 4 enemies %+v, want %+v", four.Enemies, want)
	}
	for i, w := range want {
		if g := four.Enemies[i]; !near(g.Delta, w.Delta) || g.Games != w.Games {
			t.Errorf("enemy %d: %+v, want %+v", i, g, w)
		}
	}
	if !near(four.Counter, 5+5400.0/110-50) {
		t.Errorf("counter %v, want the sum of the deltas", four.Counter)
	}
}

func TestRankAHeroWithNoGamesAgainstAnEnemyHasAZeroDelta(t *testing.T) {
	got := Rank(sixHeroes(), Draft{Enemies: []int{2}, Role: 1}, 10)
	for _, s := range got {
		if s.HeroID == 6 && (s.Enemies[0] != EnemyDelta{HeroID: 2, Delta: 0, Games: 0}) {
			t.Fatalf("hero 6 vs 2: %+v, want a zero delta on zero games", s.Enemies[0])
		}
	}
}

func TestRankAddsASmallPatchWinRateTermAndTheRoleFit(t *testing.T) {
	ds := Dataset{
		Heroes: []Hero{{ID: 1, Roles: []string{"Support"}}, {ID: 2, Roles: []string{"Carry"}}},
		Stats:  map[int]Record{1: {Games: 9900, Wins: 5445}}, // (5445+50)/10000 = 54.95 %
	}
	got := Rank(ds, Draft{Role: 5}, 10)
	// Hero 1: 0.5 * 4.95 + 4 * 1 (Support fits pos 5). Hero 2: no stats, no fit.
	if len(got) != 2 || got[0].HeroID != 1 || !near(got[0].Score, 0.5*4.95+4) || !near(got[0].WinRate, 54.95) ||
		got[0].RoleFit != 1 || got[1].Score != 0 || got[1].RoleFit != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestRankRoleFitMapsEachPositionToItsRoles(t *testing.T) {
	cases := []struct {
		role  int
		roles []string
		want  float64
	}{
		{1, []string{"Carry", "Escape"}, 1},
		{1, []string{"Support"}, 0},
		{2, []string{"Nuker"}, 1},
		{2, []string{"Support", "Nuker", "Escape"}, 0},      // a support is no mid
		{2, []string{"Support", "Carry", "Nuker"}, 1},       // unless also a carry (Lina)
		{1, []string{"Support", "Carry"}, 1},                // a carry that can support
		{3, []string{"Support", "Initiator", "Durable"}, 0}, // a support is no offlaner
		{4, []string{"Support", "Nuker"}, 1},                // supports fit the support positions
		{5, []string{"Support", "Disabler", "Initiator"}, 1},
		{2, []string{"Carry", "Pusher"}, 0.5},
		{3, []string{"Durable"}, 1},
		{3, []string{"Disabler"}, 0.5},
		{4, []string{"Support", "Nuker"}, 1},
		{4, []string{"Initiator"}, 0.5},
		{5, []string{"Support"}, 1},
		{5, []string{"Carry"}, 0},
	}
	for _, c := range cases {
		if got := RoleFit(c.role, c.roles); got != c.want {
			t.Errorf("RoleFit(%d, %v) = %v, want %v", c.role, c.roles, got, c.want)
		}
	}
}

func TestRankBreaksTiesByHeroID(t *testing.T) {
	ds := Dataset{Heroes: []Hero{{ID: 9}, {ID: 3}, {ID: 7}}}
	if got := ids(Rank(ds, Draft{Role: 1}, 10)); !slices.Equal(got, []int{3, 7, 9}) {
		t.Fatalf("tied heroes in order %v, want 3, 7, 9", got)
	}
}

func TestRankIsDeterministicForAFixedDataset(t *testing.T) {
	ds := fullDataset(127, 1)
	d := Draft{Allies: []int{1, 2, 3, 4}, Enemies: []int{10, 20, 30, 40, 50}, Bans: []int{5, 6, 7}, Role: 2}
	first := Rank(ds, d, -1)
	// The same draft with each list in another order, over the heroes reversed.
	shuffled := Dataset{Heroes: slices.Clone(ds.Heroes), Matchups: ds.Matchups, Stats: ds.Stats}
	slices.Reverse(shuffled.Heroes)
	d2 := Draft{Allies: []int{4, 3, 2, 1}, Enemies: []int{50, 40, 30, 20, 10}, Bans: []int{7, 6, 5}, Role: 2}
	for range 20 {
		again := Rank(shuffled, d2, -1)
		if !slices.Equal(ids(first), ids(again)) {
			t.Fatalf("order changed:\n%v\n%v", ids(first), ids(again))
		}
		for i := range first {
			if first[i].Score != again[i].Score {
				t.Fatalf("hero %d score %v then %v", first[i].HeroID, first[i].Score, again[i].Score)
			}
		}
	}
	if len(first) != 127-12 {
		t.Fatalf("%d suggestions, want every hero but the 12 taken", len(first))
	}
}

func TestRankReturnsTheTopN(t *testing.T) {
	ds := fullDataset(127, 1)
	all := Rank(ds, Draft{Enemies: []int{1}, Role: 1}, -1)
	top := Rank(ds, Draft{Enemies: []int{1}, Role: 1}, 8)
	if len(top) != 8 || !slices.Equal(ids(top), ids(all[:8])) {
		t.Fatalf("top 8 %v, want the first 8 of %v", ids(top), ids(all[:8]))
	}
}

// fullDataset is n heroes with a record against every other hero and patch
// stats, generated from seed.
func fullDataset(n int, seed uint64) Dataset {
	r := rand.New(rand.NewPCG(seed, seed))
	roles := []string{"Carry", "Support", "Nuker", "Disabler", "Jungler", "Durable", "Escape", "Pusher", "Initiator"}
	ds := Dataset{Matchups: map[Pair]Record{}, Stats: map[int]Record{}}
	for id := 1; id <= n; id++ {
		ds.Heroes = append(ds.Heroes, Hero{ID: id, Roles: []string{roles[r.IntN(len(roles))], roles[r.IntN(len(roles))]}})
		games := 1000 + r.IntN(100000)
		ds.Stats[id] = Record{Games: games, Wins: games/2 + r.IntN(games/10) - games/20}
	}
	for a := 1; a <= n; a++ {
		for b := 1; b <= n; b++ {
			if a != b {
				games := r.IntN(400)
				ds.Matchups[Pair{Hero: a, Opponent: b}] = Record{Games: games, Wins: r.IntN(games + 1)}
			}
		}
	}
	return ds
}

func TestRankTheFullHeroSetTakesUnder100msP95(t *testing.T) {
	ds := fullDataset(127, 7)
	d := Draft{Allies: []int{1, 2, 3, 4}, Enemies: []int{10, 20, 30, 40, 50}, Bans: []int{5, 6, 7, 8, 9, 11, 12, 13, 14, 15}, Role: 3}
	const runs = 200
	took := make([]time.Duration, runs)
	for i := range took {
		start := time.Now()
		Rank(ds, d, 8)
		took[i] = time.Since(start)
	}
	slices.Sort(took)
	p95 := took[runs*95/100]
	t.Logf("rank of 127 heroes: p50 %v, p95 %v", took[runs/2], p95)
	if p95 >= 100*time.Millisecond {
		t.Fatalf("p95 %v, want under 100 ms", p95)
	}
}

func BenchmarkRank(b *testing.B) {
	ds := fullDataset(127, 7)
	d := Draft{Allies: []int{1, 2, 3, 4}, Enemies: []int{10, 20, 30, 40, 50}, Bans: []int{5, 6, 7}, Role: 3}
	for b.Loop() {
		Rank(ds, d, 8)
	}
}

// pubMidDraft is a real-draft-like fixture with the heroes' dotaconstants
// roles: five supports that each beat the five enemies by about 3 points
// and ten mid heroes that beat them by about 1.7, as in the A3 run where
// Phoenix, Treant Protector and Bane topped a Pos 2 list.
func pubMidDraft() (Dataset, Draft) {
	supports := []Hero{
		{ID: 3, Name: "Bane", Roles: []string{"Support", "Disabler", "Nuker", "Durable"}},
		{ID: 7, Name: "Earthshaker", Roles: []string{"Support", "Initiator", "Disabler", "Nuker"}},
		{ID: 26, Name: "Lion", Roles: []string{"Support", "Disabler", "Nuker", "Initiator"}},
		{ID: 83, Name: "Treant Protector", Roles: []string{"Support", "Initiator", "Durable", "Disabler", "Escape"}},
		{ID: 110, Name: "Phoenix", Roles: []string{"Support", "Nuker", "Initiator", "Escape", "Disabler"}},
	}
	mids := []Hero{
		{ID: 11, Name: "Shadow Fiend", Roles: []string{"Carry", "Nuker"}},
		{ID: 13, Name: "Puck", Roles: []string{"Initiator", "Disabler", "Escape", "Nuker"}},
		{ID: 17, Name: "Storm Spirit", Roles: []string{"Carry", "Escape", "Nuker", "Initiator", "Disabler"}},
		{ID: 25, Name: "Lina", Roles: []string{"Support", "Carry", "Nuker", "Disabler"}},
		{ID: 34, Name: "Tinker", Roles: []string{"Carry", "Nuker", "Pusher"}},
		{ID: 39, Name: "Queen of Pain", Roles: []string{"Carry", "Nuker", "Escape"}},
		{ID: 46, Name: "Templar Assassin", Roles: []string{"Carry", "Escape"}},
		{ID: 74, Name: "Invoker", Roles: []string{"Carry", "Nuker", "Disabler", "Escape", "Pusher"}},
		{ID: 106, Name: "Ember Spirit", Roles: []string{"Carry", "Escape", "Nuker", "Disabler", "Initiator"}},
		{ID: 126, Name: "Void Spirit", Roles: []string{"Carry", "Escape", "Nuker", "Disabler"}},
	}
	enemies := []Hero{
		{ID: 2, Name: "Axe", Roles: []string{"Initiator", "Durable", "Disabler", "Carry"}},
		{ID: 5, Name: "Crystal Maiden", Roles: []string{"Support", "Disabler", "Nuker"}},
		{ID: 8, Name: "Juggernaut", Roles: []string{"Carry", "Pusher", "Escape"}},
		{ID: 28, Name: "Slardar", Roles: []string{"Carry", "Durable", "Initiator", "Disabler", "Escape"}},
		{ID: 35, Name: "Sniper", Roles: []string{"Carry", "Nuker"}},
	}
	ds := Dataset{Matchups: map[Pair]Record{}, Stats: map[int]Record{}}
	ds.Heroes = append(append(append(ds.Heroes, supports...), mids...), enemies...)
	d := Draft{Role: 2}
	for _, e := range enemies {
		d.Enemies = append(d.Enemies, e.ID)
		for _, s := range supports {
			ds.Matchups[Pair{Hero: s.ID, Opponent: e.ID}] = Record{Games: 200, Wins: 109} // +3.0
		}
		for _, m := range mids {
			ds.Matchups[Pair{Hero: m.ID, Opponent: e.ID}] = Record{Games: 200, Wins: 105} // +1.7
		}
	}
	return ds, d
}

func TestRankAPos2ListOnARealDraftHasNoSupportInItsTop8(t *testing.T) {
	ds, d := pubMidDraft()
	top := Rank(ds, d, 8)
	names := map[int]string{}
	for _, h := range ds.Heroes {
		names[h.ID] = h.Name
	}
	for _, s := range top {
		switch names[s.HeroID] {
		case "Bane", "Earthshaker", "Lion", "Treant Protector", "Phoenix":
			t.Fatalf("%s is in the Pos 2 top 8: %v", names[s.HeroID], ids(top))
		}
	}
	if len(top) != 8 {
		t.Fatalf("%d suggestions, want 8", len(top))
	}
	// Templar Assassin fits Pos 2 at 0.5 (Carry, Escape): last of the mids.
	if all := Rank(ds, d, -1); all[9].HeroID != 46 || all[10].HeroID != 3 {
		t.Fatalf("order %v, want Templar Assassin 10th and the supports after", ids(all))
	}
}

func TestRankFillsFromTheRestWhenFewerThan8HeroesFitThePosition(t *testing.T) {
	ds, d := pubMidDraft()
	var kept []Hero
	for _, h := range ds.Heroes {
		switch h.ID {
		case 11, 13, 17, 25, 34, 39, 46: // drop seven mids: three still fit
		default:
			kept = append(kept, h)
		}
	}
	ds.Heroes = kept
	got := ids(Rank(ds, d, 8))
	// Invoker, Ember Spirit and Void Spirit fit, tied, by id; then the
	// supports by score (tied, by id).
	if want := []int{74, 106, 126, 3, 7, 26, 83, 110}; !slices.Equal(got, want) {
		t.Fatalf("top 8 %v, want %v", got, want)
	}
}
