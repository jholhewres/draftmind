package gamesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Item is an item of dotaconstants items.json.
type Item struct {
	ID   int
	Key  string
	Name string
	Cost int
	// Qual is dotaconstants' quality, e.g. consumable, component or epic.
	Qual string
	// Components are the keys of the items it is built from.
	Components []string
}

// ParseItems reads items.json, leaving out entries with no id or no
// display name (unnamed recipes); a missing cost is 0. Sorted by id.
func ParseItems(b []byte) ([]Item, error) {
	var raw map[string]struct {
		ID         int      `json:"id"`
		DName      string   `json:"dname"`
		Cost       *int     `json:"cost"`
		Qual       string   `json:"qual"`
		Components []string `json:"components"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse items.json: %w", err)
	}
	out := make([]Item, 0, len(raw))
	for key, r := range raw {
		name := strings.TrimSpace(r.DName)
		if r.ID <= 0 || name == "" {
			continue
		}
		it := Item{ID: r.ID, Key: key, Name: name, Qual: r.Qual, Components: r.Components}
		if it.Components == nil {
			it.Components = []string{}
		}
		if r.Cost != nil && *r.Cost > 0 {
			it.Cost = *r.Cost
		}
		out = append(out, it)
	}
	slices.SortFunc(out, func(a, b Item) int { return a.ID - b.ID })
	return out, nil
}

const itemsSource = "dotaconstants:items"

// Items syncs the current patch's items from items.json at the source's
// revision, in one transaction, and records the run. Items the file no
// longer lists leave the patch.
func (s *Syncer) Items(ctx context.Context) (Result, error) {
	var runID int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO sync_run (source, status) VALUES ($1, 'running') RETURNING id`, itemsSource).Scan(&runID); err != nil {
		return Result{}, fmt.Errorf("start sync run: %w", err)
	}
	res, err := s.items(ctx)
	s.finish(runID, res, err)
	if err != nil {
		return res, err
	}
	s.log.Info("items synced", "revision", res.Revision, "inserted", res.Inserted, "updated", res.Updated, "deleted", res.Deleted,
		"unchanged", res.Unchanged)
	return res, nil
}

func (s *Syncer) items(ctx context.Context) (Result, error) {
	var res Result
	rev, err := s.src.Revision(ctx)
	if err != nil {
		return res, err
	}
	res.Revision = rev
	b, err := s.src.Fetch(ctx, "items.json")
	if err != nil {
		return res, fmt.Errorf("fetch items.json: %w", err)
	}
	items, err := ParseItems(b)
	if err != nil {
		return res, err
	}
	if len(items) == 0 {
		return res, errors.New("items.json has no items")
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		patchID, err := s.targetPatch(ctx, tx)
		if err != nil {
			return err
		}
		ids := make([]int, len(items))
		for i, it := range items {
			ids[i] = it.ID
		}
		del, err := tx.Exec(ctx, `DELETE FROM item WHERE patch_id = $1 AND id <> ALL($2::integer[])`, patchID, ids)
		if err != nil {
			return fmt.Errorf("remove items: %w", err)
		}
		res.Deleted += int(del.RowsAffected())
		for _, it := range items {
			inserted, written, err := upsertReturning(ctx, tx, `
				INSERT INTO item (patch_id, id, key, dname, cost, qual, components) VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (patch_id, id) DO UPDATE SET key = EXCLUDED.key, dname = EXCLUDED.dname, cost = EXCLUDED.cost,
					qual = EXCLUDED.qual, components = EXCLUDED.components, updated_at = now()
				WHERE (item.key, item.dname, item.cost, item.qual, item.components)
					IS DISTINCT FROM (EXCLUDED.key, EXCLUDED.dname, EXCLUDED.cost, EXCLUDED.qual, EXCLUDED.components)
				RETURNING xmax = 0`, patchID, it.ID, it.Key, it.Name, it.Cost, it.Qual, it.Components)
			if err != nil {
				return fmt.Errorf("upsert item %s: %w", it.Key, err)
			}
			res.add(inserted, written)
		}
		return nil
	})
	if err != nil {
		return Result{Revision: rev}, err
	}
	return res, nil
}
