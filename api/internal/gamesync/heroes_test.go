package gamesync

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

// fixtureSource serves testdata files; files overrides some of them.
type fixtureSource struct {
	t     *testing.T
	err   error
	files map[string]string
}

func (f fixtureSource) Revision(context.Context) (string, error) { return "fixture", nil }

func (f fixtureSource) Fetch(_ context.Context, file string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if b, ok := f.files[file]; ok {
		return []byte(b), nil
	}
	return readFixture(f.t, file), nil
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSyncHeroesSecondRunChangesZeroRows(t *testing.T) {
	pool := dbtest.New(t)
	s := NewSyncer(pool, fixtureSource{t: t}, quietLog())
	ctx := context.Background()

	first, err := s.Heroes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Changed() != 8 { // 2 patches + 3 heroes + 3 heroes on the current patch
		t.Fatalf("first run changed %d rows, want 8", first.Changed())
	}
	before := rowVersions(t, pool)
	second, err := s.Heroes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() != 0 || second.Unchanged != 8 {
		t.Fatalf("second run: %+v, want 0 changed and 8 unchanged", second)
	}
	if after := rowVersions(t, pool); after != before {
		t.Fatalf("second run rewrote rows: xmin %s, was %s", after, before)
	}
}

// rowVersions lists every synced row's xmin, which changes whenever a row is written.
func rowVersions(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var v string
	err := pool.QueryRow(context.Background(), `
		SELECT concat_ws(' ',
			(SELECT string_agg(xmin::text, ',' ORDER BY id) FROM patch),
			(SELECT string_agg(xmin::text, ',' ORDER BY id) FROM hero),
			(SELECT string_agg(xmin::text, ',' ORDER BY patch_id, hero_id) FROM hero_patch))`).Scan(&v)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSyncHeroesRecordsRunCounts(t *testing.T) {
	pool := dbtest.New(t)
	s := NewSyncer(pool, fixtureSource{t: t}, quietLog())
	if _, err := s.Heroes(context.Background()); err != nil {
		t.Fatal(err)
	}
	var status string
	var inserted, updated, unchanged int
	var errText *string
	err := pool.QueryRow(context.Background(),
		`SELECT status, inserted, updated, unchanged, error FROM sync_run WHERE source = 'dotaconstants:heroes'`).
		Scan(&status, &inserted, &updated, &unchanged, &errText)
	if err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || inserted != 8 || updated != 0 || unchanged != 0 || errText != nil {
		t.Fatalf("sync_run = %s %d/%d/%d err=%v", status, inserted, updated, unchanged, errText)
	}
}

func TestSyncHeroesRecordsErrorWhenSourceFails(t *testing.T) {
	pool := dbtest.New(t)
	s := NewSyncer(pool, fixtureSource{err: errors.New("github returned 503")}, quietLog())
	if _, err := s.Heroes(context.Background()); err == nil {
		t.Fatal("want error when the source fails")
	}
	var status, errText string
	err := pool.QueryRow(context.Background(),
		`SELECT status, error FROM sync_run WHERE source = 'dotaconstants:heroes'`).Scan(&status, &errText)
	if err != nil {
		t.Fatal(err)
	}
	if status != "failed" || errText == "" {
		t.Fatalf("sync_run = %s %q, want failed with the error", status, errText)
	}
}

func TestSyncHeroesMarksLatestPatchCurrent(t *testing.T) {
	pool := dbtest.New(t)
	if _, err := NewSyncer(pool, fixtureSource{t: t}, quietLog()).Heroes(context.Background()); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := pool.QueryRow(context.Background(), `SELECT name FROM patch WHERE is_current`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "7.41" {
		t.Fatalf("current patch %q, want 7.41", name)
	}
}

func TestSyncHeroesCountsTheOldCurrentPatchAsUpdated(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := NewSyncer(pool, fixtureSource{t: t}, quietLog()).Heroes(ctx); err != nil {
		t.Fatal(err)
	}
	threePatches := fixtureSource{t: t, files: map[string]string{"patch.json": `[
		{"name": "7.40", "date": "2025-12-16T00:50:40.281Z"},
		{"name": "7.41", "date": "2026-03-24T00:50:59.580Z"},
		{"name": "7.42", "date": "2026-09-01T00:00:00Z"}]`}}
	if _, err := NewSyncer(pool, threePatches, quietLog()).Heroes(ctx); err != nil {
		t.Fatal(err)
	}
	var inserted, updated, deleted, unchanged int
	err := pool.QueryRow(ctx, `
		SELECT inserted, updated, deleted, unchanged FROM sync_run ORDER BY id DESC LIMIT 1`).
		Scan(&inserted, &updated, &deleted, &unchanged)
	if err != nil {
		t.Fatal(err)
	}
	// 7.42 and its 3 heroes are new; 7.41 stopped being current; 7.40 and the heroes did not change.
	if inserted != 4 || updated != 1 || deleted != 0 || unchanged != 4 {
		t.Fatalf("sync_run counts %d/%d/%d/%d, want inserted 4, updated 1, deleted 0, unchanged 4",
			inserted, updated, deleted, unchanged)
	}
}

func TestSyncHeroesStoresTheCommitResolvedFromTheRef(t *testing.T) {
	pool := dbtest.New(t)
	srv := githubStub(t, "master", map[string]string{
		"patch.json":  string(readFixture(t, "patch.json")),
		"heroes.json": string(readFixture(t, "heroes.json")),
	})
	src := NewHTTPSource(srv.URL, srv.URL, "master", srv.Client())
	if _, err := NewSyncer(pool, src, quietLog()).Heroes(context.Background()); err != nil {
		t.Fatal(err)
	}
	var revision *string
	if err := pool.QueryRow(context.Background(), `SELECT revision FROM sync_run`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision == nil || *revision != testSHA {
		t.Fatalf("sync_run.revision = %v, want %s", revision, testSHA)
	}
}

func TestSyncHeroesCountsAHeroDroppedFromTheCurrentPatchAsDeleted(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := NewSyncer(pool, fixtureSource{t: t}, quietLog()).Heroes(ctx); err != nil {
		t.Fatal(err)
	}
	withoutCM := fixtureSource{t: t, files: map[string]string{"heroes.json": `{
		"1": {"id": 1, "name": "npc_dota_hero_antimage", "primary_attr": "agi", "attack_type": "Melee", "roles": ["Carry", "Escape", "Nuker"], "img": "/apps/dota2/images/dota_react/heroes/antimage.png?", "icon": "/apps/dota2/images/dota_react/heroes/icons/antimage.png?", "localized_name": "Anti-Mage"},
		"2": {"id": 2, "name": "npc_dota_hero_axe", "primary_attr": "str", "attack_type": "Melee", "roles": ["Initiator", "Durable", "Disabler", "Carry"], "img": "/apps/dota2/images/dota_react/heroes/axe.png?", "icon": "/apps/dota2/images/dota_react/heroes/icons/axe.png?", "localized_name": "Axe"}}`}}
	res, err := NewSyncer(pool, withoutCM, quietLog()).Heroes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 || res.Inserted != 0 || res.Updated != 0 || res.Unchanged != 6 {
		t.Fatalf("second run %+v, want 1 deleted and 6 unchanged", res)
	}
}
