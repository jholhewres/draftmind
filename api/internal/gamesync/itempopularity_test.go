package gamesync

import (
	"context"
	"fmt"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

// fixtureItemPopularity serves testdata/opendota_itempopularity_<id>.json.
type fixtureItemPopularity struct{}

func (fixtureItemPopularity) ItemPopularity(_ context.Context, heroID int) ([]byte, error) {
	return readFixtureFile(fmt.Sprintf("opendota_itempopularity_%d.json", heroID))
}

func TestItemPopularitySyncIsIdempotentAndKeyedByPatch(t *testing.T) {
	pool := dbtest.New(t)
	syncPuckAndAxe(t, pool)
	ctx := context.Background()
	s := NewSyncer(pool, fixtureSource{t: t}, quietLog())
	if _, err := s.Items(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := s.ItemPopularity(ctx, fixtureItemPopularity{})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `
		SELECT p.name, ip.hero_id, ip.phase, ip.item_id, ip.count FROM hero_item_popularity ip
		JOIN patch p ON p.id = ip.patch_id ORDER BY ip.hero_id, ip.phase, ip.item_id`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var patch, phase string
		var hero, item, count int
		if err := rows.Scan(&patch, &hero, &phase, &item, &count); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s %d %s %d %d", patch, hero, phase, item, count))
	}
	// Items 16 and 9999 are not on the patch and are left out.
	want := []string{
		"7.41 2 early 1 2300", "7.41 2 early 44 40", "7.41 2 late 609 700", "7.41 2 mid 1 120", "7.41 2 mid 145 900", "7.41 2 start 44 5120",
		"7.41 13 early 1 1900", "7.41 13 mid 609 300", "7.41 13 start 44 4000",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rows\n%v\nwant\n%v", got, want)
	}
	if first.Inserted != 9 {
		t.Fatalf("first run %+v, want 9 inserted", first)
	}
	second, err := s.ItemPopularity(ctx, fixtureItemPopularity{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() != 0 || second.Unchanged != 9 {
		t.Fatalf("second run %+v, want 0 changed and 9 unchanged", second)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM sync_run WHERE source = 'opendota:itempopularity' ORDER BY id DESC LIMIT 1`).Scan(&status); err != nil || status != "succeeded" {
		t.Fatalf("sync run %q, %v; want succeeded", status, err)
	}
}
