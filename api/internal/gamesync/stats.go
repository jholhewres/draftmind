package gamesync

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// OpenDota reads the OpenDota API.
type OpenDota interface {
	// HeroStats returns the /heroStats response.
	HeroStats(ctx context.Context) ([]byte, error)
	// Matchups returns the /heroes/{id}/matchups response.
	Matchups(ctx context.Context, heroID int) ([]byte, error)
}

const statsSource = "opendota:stats"

// Stats syncs the current patch's hero stats, then each hero's matchups in
// its own transaction. It stops at the first failure: the rows saved so far
// stay, the run is marked failed, and the next run rewrites only what changed.
func (s *Syncer) Stats(ctx context.Context, od OpenDota) (Result, error) {
	var runID int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO sync_run (source, status) VALUES ($1, 'running') RETURNING id`, statsSource).Scan(&runID); err != nil {
		return Result{}, fmt.Errorf("start sync run: %w", err)
	}
	var res Result
	err := s.stats(ctx, od, &res)
	s.finish(runID, res, err)
	if err != nil {
		return res, err
	}
	s.log.Info("hero stats and matchups synced", "inserted", res.Inserted, "updated", res.Updated,
		"deleted", res.Deleted, "unchanged", res.Unchanged)
	return res, nil
}

func (s *Syncer) stats(ctx context.Context, od OpenDota, res *Result) error {
	patchID, ids, err := s.currentPatchHeroes(ctx)
	if err != nil {
		return err
	}
	inPatch := make(map[int]bool, len(ids))
	for _, id := range ids {
		inPatch[id] = true
	}

	b, err := od.HeroStats(ctx)
	if err != nil {
		return fmt.Errorf("hero stats: %w", err)
	}
	stats, err := ParseHeroStats(b, inPatch)
	if err != nil {
		return err
	}
	if len(stats) == 0 {
		return errors.New("heroStats has no hero of the current patch")
	}
	if err := s.inTx(ctx, res, func(tx pgx.Tx, r *Result) error { return storeHeroStats(ctx, tx, patchID, stats, r) }); err != nil {
		return fmt.Errorf("store hero stats: %w", err)
	}

	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := od.Matchups(ctx, id)
		if err != nil {
			return fmt.Errorf("hero %d matchups: %w", id, err)
		}
		ms, err := ParseMatchups(b, inPatch)
		if err != nil {
			return fmt.Errorf("hero %d: %w", id, err)
		}
		if err := s.inTx(ctx, res, func(tx pgx.Tx, r *Result) error { return storeMatchups(ctx, tx, patchID, id, ms, r) }); err != nil {
			return fmt.Errorf("hero %d: store matchups: %w", id, err)
		}
	}
	return nil
}

// inTx runs fn in a transaction and adds its counts to res only on commit.
func (s *Syncer) inTx(ctx context.Context, res *Result, fn func(pgx.Tx, *Result) error) error {
	var counts Result
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		counts = Result{}
		return fn(tx, &counts)
	})
	if err != nil {
		return err
	}
	res.Inserted += counts.Inserted
	res.Updated += counts.Updated
	res.Deleted += counts.Deleted
	res.Unchanged += counts.Unchanged
	return nil
}

func storeHeroStats(ctx context.Context, tx pgx.Tx, patchID int, stats []HeroStat, res *Result) error {
	for _, st := range stats {
		inserted, written, err := upsertReturning(ctx, tx, `
			INSERT INTO hero_stat (patch_id, hero_id, bracket, picks, wins) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (patch_id, hero_id, bracket) DO UPDATE SET picks = EXCLUDED.picks, wins = EXCLUDED.wins
			WHERE (hero_stat.picks, hero_stat.wins) IS DISTINCT FROM (EXCLUDED.picks, EXCLUDED.wins)
			RETURNING xmax = 0`, patchID, st.HeroID, st.Bracket, st.Picks, st.Wins)
		if err != nil {
			return fmt.Errorf("hero %d bracket %s: %w", st.HeroID, st.Bracket, err)
		}
		res.add(inserted, written)
	}
	return nil
}

// storeMatchups makes the hero's matchup rows exactly ms.
func storeMatchups(ctx context.Context, tx pgx.Tx, patchID, heroID int, ms []Matchup, res *Result) error {
	opponents := make([]int, 0, len(ms))
	for _, m := range ms {
		if m.OpponentID == heroID {
			continue // a hero has no record against itself
		}
		opponents = append(opponents, m.OpponentID)
		inserted, written, err := upsertReturning(ctx, tx, `
			INSERT INTO hero_matchup (patch_id, hero_id, opponent_id, games, wins) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (patch_id, hero_id, opponent_id) DO UPDATE SET games = EXCLUDED.games, wins = EXCLUDED.wins
			WHERE (hero_matchup.games, hero_matchup.wins) IS DISTINCT FROM (EXCLUDED.games, EXCLUDED.wins)
			RETURNING xmax = 0`, patchID, heroID, m.OpponentID, m.Games, m.Wins)
		if err != nil {
			return fmt.Errorf("opponent %d: %w", m.OpponentID, err)
		}
		res.add(inserted, written)
	}
	del, err := tx.Exec(ctx, `DELETE FROM hero_matchup WHERE patch_id = $1 AND hero_id = $2 AND opponent_id <> ALL($3::integer[])`,
		patchID, heroID, opponents)
	if err != nil {
		return fmt.Errorf("remove old matchups: %w", err)
	}
	res.Deleted += int(del.RowsAffected())
	return nil
}

// currentPatchHeroes returns the current patch and its hero ids in order.
func (s *Syncer) currentPatchHeroes(ctx context.Context) (int, []int, error) {
	var patchID int
	if err := s.pool.QueryRow(ctx, `SELECT id FROM patch WHERE is_current`).Scan(&patchID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil, errors.New("no current patch: run sync heroes first")
		}
		return 0, nil, fmt.Errorf("current patch: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT hero_id FROM hero_patch WHERE patch_id = $1 ORDER BY hero_id`, patchID)
	if err != nil {
		return 0, nil, fmt.Errorf("current patch heroes: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return 0, nil, fmt.Errorf("current patch heroes: %w", err)
	}
	return patchID, ids, nil
}
