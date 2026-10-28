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
