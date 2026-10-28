package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Source fetches one dotaconstants build file, e.g. heroes.json.
type Source interface {
	Fetch(ctx context.Context, file string) ([]byte, error)
}

// Result counts what one sync run did to the rows it touched.
type Result struct {
	Inserted, Updated, Unchanged int
}

// Changed is the number of rows written.
func (r Result) Changed() int { return r.Inserted + r.Updated }

func (r *Result) add(inserted, written bool) {
	switch {
	case !written:
		r.Unchanged++
	case inserted:
		r.Inserted++
	default:
		r.Updated++
	}
}

// Syncer writes synced data into Postgres.
type Syncer struct {
	pool *pgxpool.Pool
	src  Source
	log  *slog.Logger
}

// NewSyncer builds a syncer.
func NewSyncer(pool *pgxpool.Pool, src Source, log *slog.Logger) *Syncer {
	return &Syncer{pool: pool, src: src, log: log}
}

const heroesSource = "dotaconstants:heroes"

// Heroes syncs patches and heroes in one transaction and records the run.
// The latest patch by release date becomes the current one.
func (s *Syncer) Heroes(ctx context.Context) (Result, error) {
	var runID int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO sync_run (source, status) VALUES ($1, 'running') RETURNING id`, heroesSource).Scan(&runID); err != nil {
		return Result{}, fmt.Errorf("start sync run: %w", err)
	}
	res, err := s.heroes(ctx)
	s.finish(runID, res, err)
	if err != nil {
		return res, err
	}
	s.log.Info("heroes synced", "inserted", res.Inserted, "updated", res.Updated, "unchanged", res.Unchanged)
	return res, nil
}

func (s *Syncer) heroes(ctx context.Context) (Result, error) {
	var res Result
	patchJSON, err := s.src.Fetch(ctx, "patch.json")
	if err != nil {
		return res, fmt.Errorf("fetch patch.json: %w", err)
	}
	heroJSON, err := s.src.Fetch(ctx, "heroes.json")
	if err != nil {
		return res, fmt.Errorf("fetch heroes.json: %w", err)
	}
	patches, err := ParsePatches(patchJSON)
	if err != nil {
		return res, err
	}
	if len(patches) == 0 {
		return res, errors.New("patch.json has no patches")
	}
	heroes, err := ParseHeroes(heroJSON)
	if err != nil {
		return res, err
	}

	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current := patches[len(patches)-1].Name
		// Clear the old current patch first: the unique index allows one.
		if _, err := tx.Exec(ctx, `UPDATE patch SET is_current = false WHERE is_current AND name <> $1`, current); err != nil {
			return fmt.Errorf("clear current patch: %w", err)
		}
		for _, p := range patches {
			inserted, written, err := upsertReturning(ctx, tx, `
				INSERT INTO patch (name, released_at, is_current) VALUES ($1, $2, $3)
				ON CONFLICT (name) DO UPDATE SET released_at = EXCLUDED.released_at, is_current = EXCLUDED.is_current
				WHERE (patch.released_at, patch.is_current) IS DISTINCT FROM (EXCLUDED.released_at, EXCLUDED.is_current)
				RETURNING xmax = 0`, p.Name, p.ReleasedAt, p.Name == current)
			if err != nil {
				return fmt.Errorf("upsert patch %s: %w", p.Name, err)
			}
			res.add(inserted, written)
		}
		for _, h := range heroes {
			inserted, written, err := upsertReturning(ctx, tx, `
				INSERT INTO hero (id, slug, name, primary_attr, attack_type, roles, img, icon)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (id) DO UPDATE SET slug = EXCLUDED.slug, name = EXCLUDED.name,
					primary_attr = EXCLUDED.primary_attr, attack_type = EXCLUDED.attack_type,
					roles = EXCLUDED.roles, img = EXCLUDED.img, icon = EXCLUDED.icon, updated_at = now()
				WHERE (hero.slug, hero.name, hero.primary_attr, hero.attack_type, hero.roles, hero.img, hero.icon)
					IS DISTINCT FROM (EXCLUDED.slug, EXCLUDED.name, EXCLUDED.primary_attr, EXCLUDED.attack_type,
					EXCLUDED.roles, EXCLUDED.img, EXCLUDED.icon)
				RETURNING xmax = 0`, h.ID, h.Slug, h.Name, h.PrimaryAttr, h.AttackType, h.Roles, h.Img, h.Icon)
			if err != nil {
				return fmt.Errorf("upsert hero %s: %w", h.Slug, err)
			}
			res.add(inserted, written)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// upsertReturning runs an upsert that returns "xmax = 0" only for rows it
// wrote: no row back means the stored row was already equal.
func upsertReturning(ctx context.Context, tx pgx.Tx, sql string, args ...any) (inserted, written bool, err error) {
	err = tx.QueryRow(ctx, sql, args...).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return inserted, true, nil
}

// finish records the outcome on a fresh context, so a cancelled sync is still logged.
func (s *Syncer) finish(runID int64, res Result, runErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status, errText := "succeeded", (*string)(nil)
	if runErr != nil {
		status = "failed"
		msg := runErr.Error()
		errText = &msg
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE sync_run SET status = $2, finished_at = now(), inserted = $3, updated = $4, unchanged = $5, error = $6
		WHERE id = $1`, runID, status, res.Inserted, res.Updated, res.Unchanged, errText); err != nil {
		s.log.Error("record sync run", "run", runID, "err", err)
	}
}
