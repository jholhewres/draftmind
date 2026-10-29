package gamesync

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// ItemPopularityAPI reads OpenDota's item popularity.
type ItemPopularityAPI interface {
	// ItemPopularity returns the /heroes/{id}/itemPopularity response.
	ItemPopularity(ctx context.Context, heroID int) ([]byte, error)
}

// Phases are the game phases of /heroes/{id}/itemPopularity, in game order,
// with the response field each one is read from.
var Phases = []struct{ Name, Field string }{
	{"start", "start_game_items"},
	{"early", "early_game_items"},
	{"mid", "mid_game_items"},
	{"late", "late_game_items"},
}

// ItemCount is how often a hero's games had an item in one phase.
type ItemCount struct {
	Phase  string
	ItemID int
	Count  int
}

// ParseItemPopularity reads a /heroes/{id}/itemPopularity response: each
// phase maps an item id to a count. Items the patch lacks and zero counts
// are skipped. Sorted by phase order, then item id.
func ParseItemPopularity(b []byte, onPatch map[int]bool) ([]ItemCount, error) {
	var raw map[string]map[string]int
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse itemPopularity: %w", err)
	}
	var out []ItemCount
	for _, ph := range Phases {
		var phase []ItemCount
		for key, n := range raw[ph.Field] {
			id, err := strconv.Atoi(key)
			if err != nil {
				return nil, fmt.Errorf("parse itemPopularity: %s has item %q", ph.Field, key)
			}
			if n < 0 {
				return nil, fmt.Errorf("parse itemPopularity: item %d has count %d", id, n)
			}
			if n > 0 && onPatch[id] {
				phase = append(phase, ItemCount{Phase: ph.Name, ItemID: id, Count: n})
			}
		}
		slices.SortFunc(phase, func(a, b ItemCount) int { return a.ItemID - b.ItemID })
		out = append(out, phase...)
	}
	return out, nil
}

const itemPopularitySource = "opendota:itempopularity"

// ItemPopularity syncs each current-patch hero's item popularity in its own
// transaction, like Stats: it stops at the first failure, the rows saved so
// far stay, and the next run rewrites only what changed.
func (s *Syncer) ItemPopularity(ctx context.Context, od ItemPopularityAPI) (Result, error) {
	var runID int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO sync_run (source, status) VALUES ($1, 'running') RETURNING id`, itemPopularitySource).Scan(&runID); err != nil {
		return Result{}, fmt.Errorf("start sync run: %w", err)
	}
	var res Result
	err := s.itemPopularity(ctx, od, &res)
	s.finish(runID, res, err)
	if err != nil {
		return res, err
	}
	s.log.Info("item popularity synced", "inserted", res.Inserted, "updated", res.Updated,
		"deleted", res.Deleted, "unchanged", res.Unchanged)
	return res, nil
}

func (s *Syncer) itemPopularity(ctx context.Context, od ItemPopularityAPI, res *Result) error {
	patchID, heroes, err := s.patchHeroes(ctx)
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `SELECT id FROM item WHERE patch_id = $1`, patchID)
	if err != nil {
		return fmt.Errorf("current patch items: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return fmt.Errorf("current patch items: %w", err)
	}
	if len(ids) == 0 {
		return fmt.Errorf("the current patch has no items: run sync heroes first")
	}
	onPatch := make(map[int]bool, len(ids))
	for _, id := range ids {
		onPatch[id] = true
	}
	for _, hero := range heroes {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := od.ItemPopularity(ctx, hero)
		if err != nil {
			return fmt.Errorf("hero %d item popularity: %w", hero, err)
		}
		counts, err := ParseItemPopularity(b, onPatch)
		if err != nil {
			return fmt.Errorf("hero %d: %w", hero, err)
		}
		if err := s.inTx(ctx, res, func(tx pgx.Tx, r *Result) error {
			return storeItemPopularity(ctx, tx, patchID, hero, counts, r)
		}); err != nil {
			return fmt.Errorf("hero %d: store item popularity: %w", hero, err)
		}
	}
	return nil
}

// storeItemPopularity makes the hero's rows exactly counts.
func storeItemPopularity(ctx context.Context, tx pgx.Tx, patchID, heroID int, counts []ItemCount, res *Result) error {
	keys := make([]string, 0, len(counts))
	for _, c := range counts {
		keys = append(keys, c.Phase+":"+strconv.Itoa(c.ItemID))
		inserted, written, err := upsertReturning(ctx, tx, `
			INSERT INTO hero_item_popularity (patch_id, hero_id, phase, item_id, count) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (patch_id, hero_id, phase, item_id) DO UPDATE SET count = EXCLUDED.count
			WHERE hero_item_popularity.count IS DISTINCT FROM EXCLUDED.count
			RETURNING xmax = 0`, patchID, heroID, c.Phase, c.ItemID, c.Count)
		if err != nil {
			return fmt.Errorf("%s item %d: %w", c.Phase, c.ItemID, err)
		}
		res.add(inserted, written)
	}
	del, err := tx.Exec(ctx, `
		DELETE FROM hero_item_popularity WHERE patch_id = $1 AND hero_id = $2 AND phase || ':' || item_id <> ALL($3::text[])`,
		patchID, heroID, keys)
	if err != nil {
		return fmt.Errorf("remove old item popularity: %w", err)
	}
	res.Deleted += int(del.RowsAffected())
	return nil
}
