package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jholhewres/draftmind/api/internal/items"
)

// Names are the entity names model text is checked against: each current
// hero's names in every language, by id; the current patch's abilities
// (talents left out) and items; and every hero off the current patch, so
// naming one is caught too. Items have English names only for now.
type Names struct {
	Heroes    map[int][]string
	Abilities []string
	Items     []string
	// ItemCards are the current patch's items, for the cards an answer shows.
	ItemCards []items.Card
	OffPatch  []string
}

// Names loads the names model text is checked against.
func (s *Store) Names(ctx context.Context) (Names, error) {
	n := Names{Heroes: map[int][]string{}}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT h.id, h.name FROM hero h
			JOIN hero_patch hp ON hp.hero_id = h.id
			JOIN patch p ON p.id = hp.patch_id AND p.is_current
			UNION
			SELECT t.hero_id, t.name FROM hero_text t
			JOIN patch p ON p.id = t.patch_id AND p.is_current
			WHERE t.name <> ''
			ORDER BY 1, 2`)
		if err != nil {
			return fmt.Errorf("hero names: %w", err)
		}
		var id int
		var name string
		if _, err := pgx.ForEachRow(rows, []any{&id, &name}, func() error {
			n.Heroes[id] = append(n.Heroes[id], name)
			return nil
		}); err != nil {
			return fmt.Errorf("hero names: %w", err)
		}
		rows, err = tx.Query(ctx, `
			SELECT DISTINCT t.name FROM ability_text t
			JOIN ability a ON a.patch_id = t.patch_id AND a.hero_id = t.hero_id AND a.id = t.ability_id
			JOIN patch p ON p.id = a.patch_id AND p.is_current
			WHERE a.kind IN ('ability', 'innate') AND t.name <> ''
			ORDER BY 1`)
		if err != nil {
			return fmt.Errorf("ability names: %w", err)
		}
		n.Abilities, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("ability names: %w", err)
		}
		rows, err = tx.Query(ctx, `
			SELECT DISTINCT h.name FROM hero h
			WHERE NOT EXISTS (SELECT 1 FROM hero_patch hp JOIN patch p ON p.id = hp.patch_id AND p.is_current WHERE hp.hero_id = h.id)
			ORDER BY 1`)
		if err != nil {
			return fmt.Errorf("other hero names: %w", err)
		}
		n.OffPatch, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("other hero names: %w", err)
		}
		rows, err = tx.Query(ctx, `
			SELECT DISTINCT i.dname FROM item i JOIN patch p ON p.id = i.patch_id AND p.is_current ORDER BY 1`)
		if err != nil {
			return fmt.Errorf("item names: %w", err)
		}
		n.Items, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("item names: %w", err)
		}
		rows, err = tx.Query(ctx, `
			SELECT i.id, i.key, i.dname, i.cost, i.img FROM item i JOIN patch p ON p.id = i.patch_id AND p.is_current ORDER BY i.id`)
		if err != nil {
			return fmt.Errorf("item cards: %w", err)
		}
		n.ItemCards, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (items.Card, error) {
			var c items.Card
			return c, row.Scan(&c.ID, &c.Key, &c.Name, &c.Cost, &c.Img)
		})
		if err != nil {
			return fmt.Errorf("item cards: %w", err)
		}
		return nil
	})
	return n, err
}
