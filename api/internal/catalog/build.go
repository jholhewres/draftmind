package catalog

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/items"
)

// BuildSource names where a build's counts come from, for the UI's label.
const BuildSource = "opendota_item_popularity"

// HeroBuild is a hero's most bought items by phase on the current patch.
type HeroBuild struct {
	Patch  string             `json:"patch"`
	Source string             `json:"source"`
	Core   []items.PhaseItems `json:"core"`
	// Builds are the stored builds per position (ADR-0011).
	Builds []Build `json:"builds"`
}

// BuildDataset loads the current patch's items, every hero's item
// popularity and, for each enemy, the heroes that beat it. A popularity row
// whose item is not on the current patch is never read.
func (s *Store) BuildDataset(ctx context.Context, enemies []int) (string, items.Dataset, error) {
	return s.buildDataset(ctx, enemies, 0)
}

// buildDataset loads the build data; only > 0 reads that hero's popularity alone.
func (s *Store) buildDataset(ctx context.Context, enemies []int, only int) (string, items.Dataset, error) {
	ds := items.Dataset{Items: map[int]items.Item{}, Popularity: map[int]map[string][]items.Count{}, Counters: map[int][]int{}, Names: map[int]string{},
		ItemTags: map[int][]string{}, HeroTags: map[int][]string{}}
	var patch string
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var patchID int
		if err := tx.QueryRow(ctx, `SELECT id, name FROM patch WHERE is_current`).Scan(&patchID, &patch); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("current patch: %w", err)
		}
		rows, err := tx.Query(ctx, `SELECT id, key, dname, cost, img, qual, components FROM item WHERE patch_id = $1`, patchID)
		if err != nil {
			return fmt.Errorf("items: %w", err)
		}
		var it items.Item
		if _, err := pgx.ForEachRow(rows, []any{&it.ID, &it.Key, &it.Name, &it.Cost, &it.Img, &it.Qual, &it.Components}, func() error {
			ds.Items[it.ID] = it
			it.Components = nil
			return nil
		}); err != nil {
			return fmt.Errorf("items: %w", err)
		}
		rows, err = tx.Query(ctx, `
			SELECT ip.hero_id, ip.phase, ip.item_id, ip.count FROM hero_item_popularity ip
			JOIN item i ON i.patch_id = ip.patch_id AND i.id = ip.item_id
			WHERE ip.patch_id = $1 AND ($2 = 0 OR ip.hero_id = $2)`, patchID, only)
		if err != nil {
			return fmt.Errorf("item popularity: %w", err)
		}
		var hero int
		var phase string
		var c items.Count
		if _, err := pgx.ForEachRow(rows, []any{&hero, &phase, &c.ItemID, &c.Count}, func() error {
			if ds.Popularity[hero] == nil {
				ds.Popularity[hero] = map[string][]items.Count{}
			}
			ds.Popularity[hero][phase] = append(ds.Popularity[hero][phase], c)
			return nil
		}); err != nil {
			return fmt.Errorf("item popularity: %w", err)
		}
		rows, err = tx.Query(ctx, `
			SELECT h.id, h.name FROM hero h JOIN hero_patch hp ON hp.hero_id = h.id AND hp.patch_id = $1`, patchID)
		if err != nil {
			return fmt.Errorf("hero names: %w", err)
		}
		var name string
		if _, err := pgx.ForEachRow(rows, []any{&hero, &name}, func() error {
			ds.Names[hero] = name
			return nil
		}); err != nil {
			return fmt.Errorf("hero names: %w", err)
		}
		// Counter tags (ADR-0011).
		rows, err = tx.Query(ctx, `SELECT item_id, tag FROM item_tag WHERE patch_id = $1 ORDER BY 1, 2`, patchID)
		if err != nil {
			return fmt.Errorf("item tags: %w", err)
		}
		var id int
		var tag string
		if _, err := pgx.ForEachRow(rows, []any{&id, &tag}, func() error {
			ds.ItemTags[id] = append(ds.ItemTags[id], tag)
			return nil
		}); err != nil {
			return fmt.Errorf("item tags: %w", err)
		}
		rows, err = tx.Query(ctx, `SELECT hero_id, tag FROM hero_tag WHERE patch_id = $1 AND hero_id = ANY($2::integer[]) ORDER BY 1, 2`, patchID, enemies)
		if err != nil {
			return fmt.Errorf("hero tags: %w", err)
		}
		if _, err := pgx.ForEachRow(rows, []any{&id, &tag}, func() error {
			ds.HeroTags[id] = append(ds.HeroTags[id], tag)
			return nil
		}); err != nil {
			return fmt.Errorf("hero tags: %w", err)
		}
		// The heroes that beat each enemy: their record against it.
		rows, err = tx.Query(ctx, `
			SELECT opponent_id, hero_id, games, wins FROM hero_matchup
			WHERE patch_id = $1 AND opponent_id = ANY($2::integer[]) AND games > 0`, patchID, enemies)
		if err != nil {
			return fmt.Errorf("enemy counters: %w", err)
		}
		type beater struct {
			id    int
			delta float64
		}
		beaters := map[int][]beater{}
		var enemy, games, wins int
		if _, err := pgx.ForEachRow(rows, []any{&enemy, &hero, &games, &wins}, func() error {
			if d := draft.Delta(wins, games); d > 0 {
				beaters[enemy] = append(beaters[enemy], beater{hero, d})
			}
			return nil
		}); err != nil {
			return fmt.Errorf("enemy counters: %w", err)
		}
		for e, bs := range beaters {
			slices.SortFunc(bs, func(a, b beater) int { return cmp.Or(cmp.Compare(b.delta, a.delta), cmp.Compare(a.id, b.id)) })
			for _, b := range bs {
				ds.Counters[e] = append(ds.Counters[e], b.id)
			}
		}
		return nil
	})
	return patch, ds, err
}

// HeroBuild returns the hero's most bought items by phase on the current
// patch; ErrNotFound for a hero the patch lacks.
func (s *Store) HeroBuild(ctx context.Context, slug string) (HeroBuild, error) {
	var heroID int
	err := s.pool.QueryRow(ctx, `
		SELECT h.id FROM hero h
		JOIN hero_patch hp ON hp.hero_id = h.id
		JOIN patch p ON p.id = hp.patch_id AND p.is_current
		WHERE h.slug = $1`, slug).Scan(&heroID)
	if errors.Is(err, pgx.ErrNoRows) {
		return HeroBuild{}, ErrNotFound
	}
	if err != nil {
		return HeroBuild{}, fmt.Errorf("hero %s: %w", slug, err)
	}
	patch, ds, err := s.buildDataset(ctx, nil, heroID)
	if err != nil {
		return HeroBuild{}, err
	}
	builds, err := s.HeroBuilds(ctx, heroID)
	if err != nil {
		return HeroBuild{}, err
	}
	return HeroBuild{Patch: patch, Source: BuildSource, Core: items.Core(ds, heroID), Builds: builds}, nil
}
