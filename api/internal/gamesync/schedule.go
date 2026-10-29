package gamesync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PatchSource is dotaconstants, which can also read a file at the ref
// itself, without spending a GitHub API request on resolving it.
type PatchSource interface {
	Source
	FetchAtRef(ctx context.Context, file string) ([]byte, error)
}

// DataFeed is the game's data feed: hero details and patch notes.
type DataFeed interface {
	Feed
	NotesFeed
}

// StatsAPI is OpenDota: stats, matchups and item popularity.
type StatsAPI interface {
	OpenDota
	ItemPopularityAPI
}

// Schedule defaults: look for a new patch every hour, refresh stats and
// builds every day, and wait at most a day before retrying a failed sync.
const (
	DefaultCheckEvery = time.Hour
	DefaultStatsEvery = 24 * time.Hour
	maxRetryWait      = 24 * time.Hour
)

const schedulePatchSource = "schedule:patch"

// Scheduler keeps the synced data current from inside the server. A new
// patch is synced from every source into its own rows while the previous
// patch stays current, and is promoted only when every source succeeded.
//
// GitHub's unauthenticated API allows 60 requests an hour: the hourly check
// reads patch.json at the ref from raw files, outside the API, and only a
// full sync resolves the ref through it, once. Not safe for concurrent use.
type Scheduler struct {
	pool   *pgxpool.Pool
	log    *slog.Logger
	source func() PatchSource // a fresh source per check, so each resolves the ref again
	feed   DataFeed
	od     StatsAPI

	// NotesLast is how many of the feed's newest patches the notes sync
	// reads; 0 reads them all, already stored ones from the cache.
	NotesLast              int
	CheckEvery, StatsEvery time.Duration

	now      func() time.Time
	failures int       // full syncs failed in a row
	retryAt  time.Time // no full sync before this after a failure
}

// NewScheduler builds a scheduler with the default intervals.
func NewScheduler(pool *pgxpool.Pool, source func() PatchSource, feed DataFeed, od StatsAPI, log *slog.Logger) *Scheduler {
	return &Scheduler{pool: pool, log: log, source: source, feed: feed, od: od,
		CheckEvery: DefaultCheckEvery, StatsEvery: DefaultStatsEvery, now: time.Now}
}

// Run checks at once, then every CheckEvery until ctx ends.
func (sc *Scheduler) Run(ctx context.Context) {
	tick := time.NewTicker(sc.CheckEvery)
	defer tick.Stop()
	for {
		if err := sc.safeCheck(ctx); err != nil && ctx.Err() == nil {
			sc.log.Warn("scheduled sync failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// safeCheck runs Check, turning a panic on odd upstream data into an error
// so the server keeps serving.
func (sc *Scheduler) safeCheck(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("scheduled sync panicked: %v", r)
		}
	}()
	return sc.Check(ctx)
}

// Check is one tick: a full sync when either source lists a patch the
// database lacks, else a stats and builds refresh when they are older than
// StatsEvery.
func (sc *Scheduler) Check(ctx context.Context) error {
	src := sc.source()
	target, fresh, err := sc.newPatch(ctx, src)
	if err != nil {
		return fmt.Errorf("look for a new patch: %w", err)
	}
	if fresh {
		if sc.now().Before(sc.retryAt) {
			return nil
		}
		if err := sc.fullSync(ctx, src, target); err != nil {
			sc.failures++
			sc.retryAt = sc.now().Add(min(sc.CheckEvery<<min(sc.failures, 10), maxRetryWait))
			return err
		}
		sc.failures, sc.retryAt = 0, time.Time{}
		return nil
	}
	return sc.refreshStats(ctx)
}

// newPatch returns dotaconstants' newest patch, and whether it is not the
// current one or the feed lists a release the database has no notes for.
func (sc *Scheduler) newPatch(ctx context.Context, src PatchSource) (string, bool, error) {
	b, err := src.FetchAtRef(ctx, "patch.json")
	if err != nil {
		return "", false, fmt.Errorf("fetch patch.json: %w", err)
	}
	patches, err := ParsePatches(b)
	if err != nil {
		return "", false, err
	}
	if len(patches) == 0 {
		return "", false, errors.New("patch.json has no patches")
	}
	target := patches[len(patches)-1].Name
	var current string
	if err := sc.pool.QueryRow(ctx, `SELECT name FROM patch WHERE is_current`).Scan(&current); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("current patch: %w", err)
	}
	if target != current {
		return target, true, nil
	}
	b, err = sc.feed.PatchList(ctx)
	if err != nil {
		return "", false, fmt.Errorf("patch list: %w", err)
	}
	releases, err := ParsePatchList(b)
	if err != nil {
		return "", false, err
	}
	var known bool
	if err := sc.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM patch_release WHERE name = $1)`,
		releases[len(releases)-1].Name).Scan(&known); err != nil {
		return "", false, fmt.Errorf("known releases: %w", err)
	}
	return target, !known, nil
}

// fullSync runs every source in order on target's rows, then promotes it.
// A failure leaves the current patch and its per-patch rows as they were;
// the shared hero rows (names, images) may already carry the new values.
// When target is already current (a new feed release only), the sync
// updates its rows in place. The rows staged so far stay for the next
// attempt to update.
func (sc *Scheduler) fullSync(ctx context.Context, src PatchSource, target string) error {
	var runID int64
	if err := sc.pool.QueryRow(ctx,
		`INSERT INTO sync_run (source, status) VALUES ($1, 'running') RETURNING id`, schedulePatchSource).Scan(&runID); err != nil {
		return fmt.Errorf("start sync run: %w", err)
	}
	s := NewSyncer(sc.pool, src, sc.log).ForPatch(target)
	sc.log.Info("new patch: full sync", "patch", target)
	err := sc.stage(ctx, s)
	if err == nil {
		err = s.Promote(ctx, target)
	}
	s.finish(runID, Result{Revision: target}, err)
	if err != nil {
		return fmt.Errorf("full sync of %s: %w", target, err)
	}
	sc.log.Info("patch synced and current", "patch", target)
	return nil
}

func (sc *Scheduler) stage(ctx context.Context, s *Syncer) error {
	steps := []func() (Result, error){
		func() (Result, error) { return s.Heroes(ctx) },
		func() (Result, error) { return s.Items(ctx) },
		func() (Result, error) { return s.Details(ctx, sc.feed) },
		func() (Result, error) { return s.Stats(ctx, sc.od) },
		func() (Result, error) { return s.ItemPopularity(ctx, sc.od) },
		func() (Result, error) { return s.PatchNotes(ctx, sc.feed, sc.NotesLast) },
	}
	for _, step := range steps {
		if _, err := step(); err != nil {
			return err
		}
	}
	return nil
}

// refreshStats syncs the current patch's stats and builds when the last
// successful run of either is older than StatsEvery.
func (sc *Scheduler) refreshStats(ctx context.Context) error {
	var oldest *time.Time
	if err := sc.pool.QueryRow(ctx, `
		SELECT CASE WHEN bool_or(last IS NULL) THEN NULL ELSE min(last) END FROM (
			SELECT (SELECT max(finished_at) FROM sync_run WHERE source = s AND status = 'succeeded') AS last
			FROM unnest($1::text[]) AS s) runs`, []string{statsSource, itemPopularitySource}).Scan(&oldest); err != nil {
		return fmt.Errorf("last stats sync: %w", err)
	}
	if oldest != nil && sc.now().Sub(*oldest) < sc.StatsEvery {
		return nil
	}
	s := NewSyncer(sc.pool, nil, sc.log)
	if _, err := s.Stats(ctx, sc.od); err != nil {
		return err
	}
	_, err := s.ItemPopularity(ctx, sc.od)
	return err
}
