package draft

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

// Mode is the game mode of a draft (ADR-0012).
type Mode string

// The supported modes. Ability Draft and custom games are not a hero draft.
const (
	AllPickRanked Mode = "all_pick_ranked"
	AllPick       Mode = "all_pick" // unranked, casual
	Turbo         Mode = "turbo"
	CaptainsMode  Mode = "captains_mode"
	CaptainsDraft Mode = "captains_draft"
	SingleDraft   Mode = "single_draft"
	RandomDraft   Mode = "random_draft"
	AllRandom     Mode = "all_random"
)

// Modes lists the supported modes in the order the UI offers them.
var Modes = []Mode{AllPickRanked, AllPick, Turbo, CaptainsMode, CaptainsDraft, SingleDraft, RandomDraft, AllRandom}

// ErrUnsupportedMode is a mode that is not a hero draft.
var ErrUnsupportedMode = errors.New("game mode not supported")

// ParseMode reads a mode; empty is ranked All Pick.
func ParseMode(s string) (Mode, error) {
	if s == "" {
		return AllPickRanked, nil
	}
	if m := Mode(s); slices.Contains(Modes, m) {
		return m, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnsupportedMode, s)
}

// Pooled says the mode offers a pool of heroes to pick from.
func (m Mode) Pooled() bool { return m == SingleDraft || m == RandomDraft || m == CaptainsDraft }

// Suggests says the player picks a hero at all.
func (m Mode) Suggests() bool { return m != AllRandom }

// HasBans says the ban phase is part of the draft.
func (m Mode) HasBans() bool { return m == CaptainsMode || m == CaptainsDraft }

// Which stats a ranking read.
const (
	StatsRanked = "ranked" // the pub brackets 1-8, every mode but Turbo
	StatsTurbo  = "turbo"  // OpenDota's turbo picks and wins
)

// StatsFor names the stats a ranking for m reads: Turbo's own when the
// dataset has them, else the ranked ones, which the UI labels as such.
// OpenDota splits hero stats by bracket and Turbo only; matchups and item
// popularity are not split by mode (ADR-0012).
func StatsFor(ds Dataset, m Mode) string {
	if m == Turbo && len(ds.TurboStats) > 0 {
		return StatsTurbo
	}
	return StatsRanked
}

// TimeScale is how long the mode's game clock takes for the same moment of a
// normal game: Turbo runs about twice as fast.
func TimeScale(m Mode) float64 {
	if m == Turbo {
		return 0.5
	}
	return 1
}

// GoldScale is how much faster gold comes: Turbo roughly doubles it.
func GoldScale(m Mode) float64 {
	if m == Turbo {
		return 2
	}
	return 1
}

// Bans are the best n heroes to ban in a mode with a ban phase: those that
// beat the allies already picked the most, the patch win rate breaking ties.
// Nil for a mode without bans.
func Bans(ds Dataset, d Draft, n int) []Suggestion {
	if !d.Mode.HasBans() {
		return nil
	}
	// A ban is ranked as a pick against our own side.
	flip := Draft{Mode: d.Mode, Pool: d.Pool, Allies: d.Enemies, Enemies: d.Allies, Bans: d.Bans}
	out := rankAll(ds, flip)
	slices.SortFunc(out, func(a, b Suggestion) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(a.HeroID, b.HeroID))
	})
	return out[:min(n, len(out))]
}
