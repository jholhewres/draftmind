package gamesync

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

// relationItems: Battle Fury is built from Blink Dagger, Tango and an item
// the patch lacks.
const relationItems = `{
	"blink": {"id": 1, "img": "/blink.png", "dname": "Blink Dagger", "cost": 2250},
	"tango": {"id": 44, "img": "/tango.png", "dname": "Tango", "cost": 90},
	"bfury": {"id": 145, "img": "/bfury.png", "dname": "Battle Fury", "cost": 4100, "components": ["blink", "tango", "quarterstaff"]},
	"aghanims_shard": {"id": 609, "dname": "Aghanim's Shard", "cost": 1400}}`

// relationCounters names an item and a hero the patch lacks; both are skipped.
const relationCounters = `{
	"version": 1,
	"items": {"illusions": ["bfury"], "mobility": ["blink", "no_such_item"]},
	"heroes": {"axe": ["healing"], "no_such_hero": ["evasion"]},
	"roles": {"Escape": ["mobility"]}}`

func queryStrings(t *testing.T, s *Syncer, sql string) []string {
	t.Helper()
	rows, err := s.pool.Query(context.Background(), sql)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.TrimSuffix(fmt.Sprintln(vals...), "\n"))
	}
	return out
}

func TestRelationsSyncStoresComponentsTagsAndBuildsKeyedByPatchAndChangesNothingOnASecondRun(t *testing.T) {
	pool := dbtest.New(t)
	syncPuckAndAxe(t, pool)
	ctx := context.Background()
	s := NewSyncer(pool, fixtureSource{t: t, files: map[string]string{"items.json": relationItems}}, quietLog())
	if _, err := s.Items(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stats(ctx, fixtureOpenDota{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ItemPopularity(ctx, fixtureItemPopularity{}); err != nil {
		t.Fatal(err)
	}
	first, err := s.Relations(ctx, []byte(relationCounters))
	if err != nil {
		t.Fatal(err)
	}

	check := func(name, sql string, want ...string) {
		t.Helper()
		if got := queryStrings(t, s, sql); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s\n got %q\nwant %q", name, got, want)
		}
	}
	check("components", `SELECT i.key, c.key FROM item_component ic
		JOIN item i ON i.patch_id = ic.patch_id AND i.id = ic.item_id JOIN item c ON c.patch_id = ic.patch_id AND c.id = ic.component_id
		ORDER BY 1, 2`, "bfury blink", "bfury tango")
	check("upgrades of Blink Dagger", `SELECT u.key FROM item_component ic JOIN item u ON u.patch_id = ic.patch_id AND u.id = ic.item_id
		WHERE ic.component_id = 1`, "bfury")
	check("item tags", `SELECT i.key, t.tag, t.source FROM item_tag t JOIN item i ON i.patch_id = t.patch_id AND i.id = t.item_id ORDER BY 1, 2`,
		"bfury illusions curated", "blink mobility curated")
	check("hero tags", `SELECT h.slug, t.tag, t.source FROM hero_tag t JOIN hero h ON h.id = t.hero_id ORDER BY 1, 2`,
		"axe healing curated", "puck mobility roles")
	// Axe (Initiator) and Puck (Initiator, Escape) fit position 3 best.
	check("builds", `SELECT p.name, b.hero_id, b.position, b.source, b.games > 0, b.win_rate BETWEEN 0 AND 1 FROM hero_build b
		JOIN patch p ON p.id = b.patch_id ORDER BY 2`, "7.41 2 3 opendota true true", "7.41 13 3 opendota true true")
	check("build items", `SELECT bi.hero_id, bi.ord, bi.phase, i.key FROM hero_build_item bi
		JOIN item i ON i.patch_id = bi.patch_id AND i.id = bi.item_id ORDER BY 1, 2`,
		"2 0 start tango", "2 1 early blink", "2 2 mid bfury", "2 3 late aghanims_shard",
		"13 0 start tango", "13 1 early blink", "13 2 mid aghanims_shard")
	if first.Inserted != 15 {
		t.Fatalf("first run %+v, want 15 rows inserted", first)
	}

	second, err := s.Relations(ctx, []byte(relationCounters))
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() != 0 || second.Unchanged != 15 {
		t.Fatalf("second run %+v, want nothing changed", second)
	}

	// A tag dropped from the file leaves the database.
	third, err := s.Relations(ctx, []byte(`{"version": 2, "items": {"illusions": ["bfury"]}, "heroes": {}, "roles": {}}`))
	if err != nil {
		t.Fatal(err)
	}
	if third.Deleted != 3 {
		t.Fatalf("third run %+v, want the 3 dropped tags deleted", third)
	}
}
