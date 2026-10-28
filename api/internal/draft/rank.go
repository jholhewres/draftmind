// Package draft ranks the heroes a player can pick against a draft. It is
// pure: the caller loads the dataset, and the same input gives the same output.
package draft

import (
	"cmp"
	"slices"
)

// ShrinkGames is k in (wins + k/2) / (games + k): a record of k games keeps
// half of its distance from 50 %. Method of moments on real OpenDota matchups
// (Axe, Puck) gave k between 72 and 192; 100 sits between (ADR-0009).
const ShrinkGames = 100

// ShrunkWinRate is a win rate pulled toward 50 % by the games behind it.
func ShrunkWinRate(wins, games int) float64 {
	return (float64(wins) + ShrinkGames*0.5) / (float64(games) + ShrinkGames)
}

// Record is a count of games and the wins among them.
type Record struct{ Games, Wins int }

// Pair names Hero's record against Opponent.
type Pair struct{ Hero, Opponent int }

// Hero is a hero of the current patch with its dotaconstants roles.
type Hero struct {
	ID    int
	Name  string
	Roles []string
}

// Dataset is what ranking reads: the patch's heroes, their matchups and
// their patch picks (as Games) and wins.
type Dataset struct {
	Heroes   []Hero
	Matchups map[Pair]Record
	Stats    map[int]Record
}

// Draft is the state of a pick: hero ids per side and the player's position.
type Draft struct {
	Allies, Enemies, Bans []int
	Role                  int // 1..5
}

// EnemyDelta is a candidate's shrunk win rate against one enemy, minus 50, in points.
type EnemyDelta struct {
	HeroID int     `json:"heroId"`
	Delta  float64 `json:"delta"`
	Games  int     `json:"games"`
}

// Suggestion is a ranked candidate with the reasons behind its score.
type Suggestion struct {
	HeroID  int          `json:"heroId"`
	Score   float64      `json:"score"`
	Counter float64      `json:"counter"` // sum of the enemy deltas
	WinRate float64      `json:"winRate"` // shrunk patch win rate, in %
	RoleFit float64      `json:"roleFit"` // 0..1
	Enemies []EnemyDelta `json:"enemies"` // in the draft's enemy order
	// Probability is Jev's for this hero, when Jev was asked about it.
	Probability *float64 `json:"probability,omitempty"`
}

// Delta is 100 * ShrunkWinRate - 50, in win-rate points, in a form that
// keeps whole results exact (60 wins in 100 games is +5, not +5.000000000000007).
func Delta(wins, games int) float64 {
	return 50 * float64(2*wins-games) / float64(games+ShrinkGames)
}

// Score weights, in win-rate points (ADR-0009).
const (
	// PatchWeight scales the hero's patch win rate minus 50.
	PatchWeight = 0.5
	// RoleWeight is what a perfect fit for the player's position adds.
	RoleWeight = 4.0
)

// roleFit maps positions 1-5 to dotaconstants roles and how well each fits.
// A hero's fit is its best role's weight; no listed role fits 0.
var roleFit = map[int]map[string]float64{
	1: {"Carry": 1},
	2: {"Nuker": 1, "Carry": 0.5, "Escape": 0.5},
	3: {"Initiator": 1, "Durable": 1, "Disabler": 0.5},
	4: {"Support": 1, "Initiator": 0.5, "Disabler": 0.5, "Nuker": 0.5},
	5: {"Support": 1, "Disabler": 0.5},
}

// RoleFit is how well roles fit position role, from 0 to 1. A hero listed
// as Support but not Carry fits no core position (1-3), whatever else it
// lists: Bane's Nuker or Treant Protector's Durable do not make a mid or an
// offlaner (ADR-0009).
func RoleFit(role int, roles []string) float64 {
	if role <= 3 && slices.Contains(roles, "Support") && !slices.Contains(roles, "Carry") {
		return 0
	}
	fit := 0.0
	for _, r := range roles {
		fit = max(fit, roleFit[role][r])
	}
	return fit
}

// Rank returns the best n legal heroes for the draft: every hero not picked
// or banned, the heroes that fit the position first, each group by score,
// then by hero id. Counter deltas over five enemies outweigh the role term,
// so fitting first is what keeps a support off a mid's list; when fewer
// heroes fit than are asked for, the rest fill in.
func Rank(ds Dataset, d Draft, n int) []Suggestion {
	taken := make(map[int]bool, len(d.Allies)+len(d.Enemies)+len(d.Bans))
	for _, list := range [][]int{d.Allies, d.Enemies, d.Bans} {
		for _, id := range list {
			taken[id] = true
		}
	}
	// Summed in id order, so the score does not depend on the enemy order.
	byID := slices.Clone(d.Enemies)
	slices.Sort(byID)

	out := make([]Suggestion, 0, len(ds.Heroes))
	for _, h := range ds.Heroes {
		if taken[h.ID] {
			continue
		}
		s := Suggestion{HeroID: h.ID, Enemies: make([]EnemyDelta, len(d.Enemies))}
		delta := make(map[int]float64, len(d.Enemies))
		for i, e := range d.Enemies {
			m := ds.Matchups[Pair{Hero: h.ID, Opponent: e}]
			delta[e] = Delta(m.Wins, m.Games)
			s.Enemies[i] = EnemyDelta{HeroID: e, Delta: delta[e], Games: m.Games}
		}
		for _, e := range byID {
			s.Counter += delta[e]
		}
		st := ds.Stats[h.ID]
		s.WinRate = 100 * ShrunkWinRate(st.Wins, st.Games)
		s.RoleFit = RoleFit(d.Role, h.Roles)
		s.Score = s.Counter + PatchWeight*(s.WinRate-50) + RoleWeight*s.RoleFit
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Suggestion) int {
		if fa, fb := a.RoleFit > 0, b.RoleFit > 0; fa != fb {
			if fa {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return cmp.Compare(a.HeroID, b.HeroID)
	})
	if n >= 0 && n < len(out) {
		out = out[:n]
	}
	return out
}
