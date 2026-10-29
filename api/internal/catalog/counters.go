package catalog

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/jholhewres/draftmind/api/internal/draft"
)

// LowSampleGames is the record size under which a matchup is weaker: below
// draft.ShrinkGames games the shrinkage keeps less than half of its distance
// from 50 % (ADR-0009).
const LowSampleGames = draft.ShrinkGames

// Counter is a hero's record against one opponent on the current patch:
// the shrunk win rate minus 50, in points, and the games behind it.
type Counter struct {
	Hero
	Delta     float64 `json:"delta"`
	Games     int     `json:"games"`
	LowSample bool    `json:"lowSample"`
}

// Counters are the heroes a hero beats and the heroes that beat it on the
// current patch, strongest first.
type Counters struct {
	Patch          string    `json:"patch"`
	LowSampleGames int       `json:"lowSampleGames"`
	GoodAgainst    []Counter `json:"goodAgainst"`
	BadAgainst     []Counter `json:"badAgainst"`
}

// HeroCounters returns up to n opponents on each side for the hero with
// slug, from the current patch's hero_matchup rows only.
func (s *Store) HeroCounters(ctx context.Context, slug string, n int) (Counters, error) {
	out := Counters{LowSampleGames: LowSampleGames, GoodAgainst: []Counter{}, BadAgainst: []Counter{}}
	var patchID, heroID int
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.name, h.id FROM hero h
		JOIN hero_patch hp ON hp.hero_id = h.id
		JOIN patch p ON p.id = hp.patch_id AND p.is_current
		WHERE h.slug = $1`, slug).Scan(&patchID, &out.Patch, &heroID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("hero %s: %w", slug, err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT h.id, h.slug, h.name, h.primary_attr, h.attack_type, h.roles, h.img, h.icon, m.games, m.wins
		FROM hero_matchup m
		JOIN hero h ON h.id = m.opponent_id
		WHERE m.patch_id = $1 AND m.hero_id = $2 AND m.games > 0`, patchID, heroID)
	if err != nil {
		return out, fmt.Errorf("counters of %s: %w", slug, err)
	}
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Counter, error) {
		var c Counter
		var wins int
		if err := row.Scan(&c.ID, &c.Slug, &c.Name, &c.PrimaryAttr, &c.AttackType, &c.Roles, &c.Img, &c.Icon, &c.Games, &wins); err != nil {
			return c, err
		}
		c.Delta = draft.Delta(wins, c.Games)
		c.LowSample = c.Games < LowSampleGames
		return c, nil
	})
	if err != nil {
		return out, fmt.Errorf("counters of %s: %w", slug, err)
	}
	out.GoodAgainst = strongest(all, n, func(c Counter) float64 { return c.Delta })
	out.BadAgainst = strongest(all, n, func(c Counter) float64 { return -c.Delta })
	return out, nil
}

// strongest returns up to n counters whose edge is above 0, largest edge
// first; ties go to the larger sample, then the lower hero id.
func strongest(all []Counter, n int, edge func(Counter) float64) []Counter {
	out := []Counter{}
	for _, c := range all {
		if edge(c) > 0 {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Counter) int {
		return cmp.Or(cmp.Compare(edge(b), edge(a)), cmp.Compare(b.Games, a.Games), cmp.Compare(a.ID, b.ID))
	})
	return out[:min(n, len(out))]
}
