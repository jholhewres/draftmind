package sync

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

type fixtureSource struct {
	t   *testing.T
	err error
}

func (f fixtureSource) Fetch(_ context.Context, file string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
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
	if first.Changed() != 5 { // 3 heroes + 2 patches
		t.Fatalf("first run changed %d rows, want 5", first.Changed())
	}
	second, err := s.Heroes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() != 0 || second.Unchanged != 5 {
		t.Fatalf("second run: %+v, want 0 changed and 5 unchanged", second)
	}
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
	if status != "succeeded" || inserted != 5 || updated != 0 || unchanged != 0 || errText != nil {
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
