package gamesync

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

// refSource reads patch.json at the ref the same way it reads it at the revision.
type refSource struct{ fixtureSource }

func (r refSource) FetchAtRef(ctx context.Context, file string) ([]byte, error) {
	return r.Fetch(ctx, file)
}

type fixtureDataFeed struct {
	fixtureFeed
	fixtureNotes
}

type fixtureStatsAPI struct {
	fixtureOpenDota
	fixtureItemPopularity
}

const onePatch = `[{"name": "7.40", "date": "2025-12-16T00:50:40.281Z"}]`

// syncPatch740 makes 7.40 current with Puck and Axe and every source synced on it.
func syncPatch740(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	s := NewSyncer(pool, fixtureSource{t: t, files: map[string]string{"heroes.json": puckAndAxe, "patch.json": onePatch}}, quietLog())
	if _, err := s.Heroes(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Items(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Details(ctx, fixtureFeed{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stats(ctx, fixtureOpenDota{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ItemPopularity(ctx, fixtureItemPopularity{}); err != nil {
		t.Fatal(err)
	}
}

// scheduler sees 7.41 in patch.json, with Puck and Axe, and serves od as OpenDota.
func scheduler(t *testing.T, pool *pgxpool.Pool, od fixtureOpenDota) *Scheduler {
	calls := 0
	src := func() PatchSource {
		return refSource{fixtureSource{t: t, files: map[string]string{"heroes.json": puckAndAxe}}}
	}
	sc := NewScheduler(pool, src, fixtureDataFeed{fixtureNotes: fixtureNotes{calls: &calls}},
		fixtureStatsAPI{fixtureOpenDota: od}, quietLog())
	sc.NotesLast = 2
	return sc
}

func currentPatch(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var name string
	if err := pool.QueryRow(context.Background(), `SELECT name FROM patch WHERE is_current`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

// patchRows counts and versions every row a patch owns, so any write to it shows.
func patchRows(t *testing.T, pool *pgxpool.Pool, patch string) string {
	t.Helper()
	var v string
	err := pool.QueryRow(context.Background(), `
		WITH p AS (SELECT id FROM patch WHERE name = $1)
		SELECT concat_ws(' ',
			(SELECT count(*) || ':' || coalesce(string_agg(xmin::text, ',' ORDER BY hero_id), '') FROM hero_patch WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) || ':' || coalesce(sum(xmin::text::bigint), 0) FROM hero_text WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) || ':' || coalesce(sum(xmin::text::bigint), 0) FROM ability WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) || ':' || coalesce(sum(xmin::text::bigint), 0) FROM item WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) || ':' || coalesce(sum(xmin::text::bigint), 0) FROM hero_stat WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) || ':' || coalesce(sum(xmin::text::bigint), 0) FROM hero_matchup WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) || ':' || coalesce(sum(xmin::text::bigint), 0) FROM hero_item_popularity WHERE patch_id = (SELECT id FROM p)))`,
		patch).Scan(&v)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestScheduleANewPatchTriggersTheFullSync(t *testing.T) {
	pool := dbtest.New(t)
	syncPatch740(t, pool)
	ctx := context.Background()

	if err := scheduler(t, pool, fixtureOpenDota{}).Check(ctx); err != nil {
		t.Fatal(err)
	}
	if got := currentPatch(t, pool); got != "7.41" {
		t.Fatalf("current patch %q, want 7.41", got)
	}
	var heroes, texts, items, stats, matchups, builds, notes int
	err := pool.QueryRow(ctx, `
		WITH p AS (SELECT id FROM patch WHERE name = '7.41')
		SELECT (SELECT count(*) FROM hero_patch WHERE patch_id = (SELECT id FROM p) AND max_health IS NOT NULL),
			(SELECT count(*) FROM hero_text WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) FROM item WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) FROM hero_stat WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) FROM hero_matchup WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(*) FROM hero_item_popularity WHERE patch_id = (SELECT id FROM p)),
			(SELECT count(DISTINCT patch_name) FROM patch_note)`).Scan(&heroes, &texts, &items, &stats, &matchups, &builds, &notes)
	if err != nil {
		t.Fatal(err)
	}
	if heroes != 2 || texts != 4 || items == 0 || stats == 0 || matchups == 0 || builds == 0 || notes != 2 {
		t.Fatalf("7.41 has %d heroes with details, %d texts, %d items, %d stats, %d matchups, %d builds; %d patches of notes",
			heroes, texts, items, stats, matchups, builds, notes)
	}
	var status, revision string
	if err := pool.QueryRow(ctx, `SELECT status, revision FROM sync_run WHERE source = 'schedule:patch'`).Scan(&status, &revision); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || revision != "7.41" {
		t.Fatalf("full sync run %s for %q, want succeeded for 7.41", status, revision)
	}
}

func TestScheduleAFailedSyncKeepsThePreviousPatchCurrent(t *testing.T) {
	pool := dbtest.New(t)
	syncPatch740(t, pool)
	ctx := context.Background()
	before := patchRows(t, pool, "7.40")

	if err := scheduler(t, pool, fixtureOpenDota{failMatchups: 13}).Check(ctx); err == nil {
		t.Fatal("a full sync whose matchups failed reported success")
	}
	if got := currentPatch(t, pool); got != "7.40" {
		t.Fatalf("current patch %q after a failed sync, want 7.40", got)
	}
	if after := patchRows(t, pool, "7.40"); after != before {
		t.Fatalf("7.40's rows changed: %s, were %s", after, before)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM sync_run WHERE source = 'schedule:patch'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("full sync run %s, want failed", status)
	}
}
