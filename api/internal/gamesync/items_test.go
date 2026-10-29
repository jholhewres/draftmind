package gamesync

import (
	"context"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

type itemRow struct {
	ID   int
	Key  string
	Name string
	Cost int
}

func patchItems(t *testing.T, s *Syncer) []itemRow {
	t.Helper()
	rows, err := s.pool.Query(context.Background(), `
		SELECT i.id, i.key, i.dname, i.cost FROM item i JOIN patch p ON p.id = i.patch_id AND p.is_current ORDER BY i.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []itemRow
	for rows.Next() {
		var r itemRow
		if err := rows.Scan(&r.ID, &r.Key, &r.Name, &r.Cost); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestSyncItemsWritesTheCurrentPatchItemsOnceAndDropsRemovedOnes(t *testing.T) {
	pool := dbtest.New(t)
	s := NewSyncer(pool, fixtureSource{t: t}, quietLog())
	ctx := context.Background()
	if _, err := s.Heroes(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := s.Items(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The unnamed recipe is left out; a missing cost is 0.
	want := []itemRow{{1, "blink", "Blink Dagger", 2250}, {44, "tango", "Tango", 90}, {145, "bfury", "Battle Fury", 4100}, {609, "aghanims_shard", "Aghanim's Shard", 0}}
	if got := patchItems(t, s); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("items %+v, want %+v", got, want)
	}
	if first.Inserted != 4 || first.Revision != "fixture" {
		t.Fatalf("first run %+v, want 4 inserted at the fixture revision", first)
	}
	second, err := s.Items(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() != 0 || second.Unchanged != 4 {
		t.Fatalf("second run %+v, want nothing changed", second)
	}
	s.src = fixtureSource{t: t, files: map[string]string{"items.json": `{"tango": {"id": 44, "dname": "Tango", "cost": 95}}`}}
	third, err := s.Items(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := patchItems(t, s); len(got) != 1 || got[0] != (itemRow{44, "tango", "Tango", 95}) || third.Deleted != 3 || third.Updated != 1 {
		t.Fatalf("after a patch: items %+v, run %+v", got, third)
	}
}
