package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Names are the current patch's entity names, for checking model text
// against the database: each hero's names in every language, by id, and
// every ability's (talents left out).
type Names struct {
	Heroes    map[int][]string
	Abilities []string
}

// Names loads the current patch's hero and ability names.
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
		return nil
	})
	return n, err
}
