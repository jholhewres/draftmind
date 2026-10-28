package catalog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jholhewres/draftmind/api/internal/draft"
)

// pubBrackets are the hero_stat brackets the patch win rate sums: the ranked
// pub brackets, without the small pro sample.
var pubBrackets = []string{"1", "2", "3", "4", "5", "6", "7", "8"}

// DraftDataset loads what ranking reads for the current patch: its heroes,
// their pub picks and wins, and every hero's record against the enemies.
// The patch is empty until the first sync.
func (s *Store) DraftDataset(ctx context.Context, enemies []int) (string, draft.Dataset, error) {
	ds := draft.Dataset{Matchups: map[draft.Pair]draft.Record{}, Stats: map[int]draft.Record{}}
	var patch string
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.name, h.id, h.roles FROM hero h
			JOIN hero_patch hp ON hp.hero_id = h.id
			JOIN patch p ON p.id = hp.patch_id AND p.is_current
			ORDER BY h.id`)
		if err != nil {
			return fmt.Errorf("heroes: %w", err)
		}
		ds.Heroes, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (draft.Hero, error) {
			var h draft.Hero
			return h, row.Scan(&patch, &h.ID, &h.Roles)
		})
		if err != nil {
			return fmt.Errorf("heroes: %w", err)
		}

		rows, err = tx.Query(ctx, `
			SELECT st.hero_id, sum(st.picks), sum(st.wins) FROM hero_stat st
			JOIN patch p ON p.id = st.patch_id AND p.is_current
			WHERE st.bracket = ANY($1)
			GROUP BY st.hero_id`, pubBrackets)
		if err != nil {
			return fmt.Errorf("hero stats: %w", err)
		}
		var id int
		var r draft.Record
		if _, err := pgx.ForEachRow(rows, []any{&id, &r.Games, &r.Wins}, func() error {
			ds.Stats[id] = r
			return nil
		}); err != nil {
			return fmt.Errorf("hero stats: %w", err)
		}

		rows, err = tx.Query(ctx, `
			SELECT m.hero_id, m.opponent_id, m.games, m.wins FROM hero_matchup m
			JOIN patch p ON p.id = m.patch_id AND p.is_current
			WHERE m.opponent_id = ANY($1::integer[])`, enemies)
		if err != nil {
			return fmt.Errorf("matchups: %w", err)
		}
		var pair draft.Pair
		if _, err := pgx.ForEachRow(rows, []any{&pair.Hero, &pair.Opponent, &r.Games, &r.Wins}, func() error {
			ds.Matchups[pair] = r
			return nil
		}); err != nil {
			return fmt.Errorf("matchups: %w", err)
		}
		return nil
	})
	return patch, ds, err
}
