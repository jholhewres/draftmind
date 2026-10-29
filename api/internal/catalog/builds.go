package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jholhewres/draftmind/api/internal/items"
)

// Build is a stored build for a hero at a position (ADR-0011).
type Build struct {
	Position int         `json:"position"`
	Source   string      `json:"source"` // opendota, curated or coach
	Name     string      `json:"name"`
	Games    *int        `json:"games,omitempty"`
	WinRate  *float64    `json:"winRate,omitempty"`
	Items    []BuildItem `json:"items"`
}

// BuildItem is a build's item in order, with its phase.
type BuildItem struct {
	Phase string `json:"phase"`
	items.Card
}

// HeroBuilds are the hero's stored builds on the current patch, by position
// then source, each with its items in order.
func (s *Store) HeroBuilds(ctx context.Context, hero int) ([]Build, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.position, b.source, b.name, b.games, b.win_rate, bi.phase, i.id, i.key, i.dname, i.cost, i.img
		FROM hero_build b JOIN patch p ON p.id = b.patch_id AND p.is_current
		JOIN hero_build_item bi USING (patch_id, hero_id, position, source)
		JOIN item i ON i.patch_id = bi.patch_id AND i.id = bi.item_id
		WHERE b.hero_id = $1 ORDER BY b.position, b.source, bi.ord`, hero)
	if err != nil {
		return nil, fmt.Errorf("hero builds: %w", err)
	}
	out := []Build{}
	var b Build
	var it BuildItem
	if _, err := pgx.ForEachRow(rows, []any{&b.Position, &b.Source, &b.Name, &b.Games, &b.WinRate, &it.Phase, &it.ID, &it.Key, &it.Name,
		&it.Cost, &it.Img}, func() error {
		if n := len(out); n == 0 || out[n-1].Position != b.Position || out[n-1].Source != b.Source {
			out = append(out, Build{Position: b.Position, Source: b.Source, Name: b.Name, Games: b.Games, WinRate: b.WinRate})
		}
		out[len(out)-1].Items = append(out[len(out)-1].Items, it)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("hero builds: %w", err)
	}
	return out, nil
}
