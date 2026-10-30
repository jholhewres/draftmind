package catalog

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
	"github.com/jholhewres/draftmind/api/internal/items"
)

// seed puts Axe on an old and the current patch, the same item id on both
// with different names, and two current builds plus one old.
func seed(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := dbtest.New(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO patch (name, is_current) VALUES ('7.40', false), ('7.41', true);
		INSERT INTO hero (id, slug, name, primary_attr, attack_type, img, icon) VALUES (2, 'axe', 'Axe', 'str', 'Melee', '/a.png', '/a_i.png');
		INSERT INTO hero_patch (patch_id, hero_id) SELECT id, 2 FROM patch;
		INSERT INTO item (patch_id, id, key, dname, cost, img) SELECT id, 1, 'blink', CASE WHEN is_current THEN 'Blink Dagger' ELSE 'Old Blink' END, 2250, '/blink.png' FROM patch;
		INSERT INTO item (patch_id, id, key, dname, cost, img) SELECT id, 116, 'black_king_bar', 'Black King Bar', 4050, '/bkb.png' FROM patch WHERE is_current;
		INSERT INTO hero_build (patch_id, hero_id, position, source, name, games, win_rate) SELECT id, 2, 3, 'opendota', 'Popular', 500, 0.52 FROM patch;
		INSERT INTO hero_build (patch_id, hero_id, position, source, name) SELECT id, 2, 3, 'curated', 'Blink first' FROM patch WHERE is_current;
		INSERT INTO hero_build_item (patch_id, hero_id, position, source, ord, phase, item_id)
			SELECT id, 2, 3, 'opendota', 0, 'mid', 1 FROM patch UNION ALL
			SELECT id, 2, 3, 'opendota', 1, 'late', 116 FROM patch WHERE is_current UNION ALL
			SELECT id, 2, 3, 'curated', 0, 'early', 1 FROM patch WHERE is_current`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestHeroBuildsReadsTheCurrentPatchBuildsWithTheirItemsInOrder(t *testing.T) {
	s := NewStore(seed(t))
	got, err := s.HeroBuilds(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	games, wr := 500, 0.52
	blink := items.Card{ID: 1, Key: "blink", Name: "Blink Dagger", Cost: 2250, Img: "/blink.png"}
	bkb := items.Card{ID: 116, Key: "black_king_bar", Name: "Black King Bar", Cost: 4050, Img: "/bkb.png"}
	want := []Build{
		{Position: 3, Source: "curated", Name: "Blink first", Items: []BuildItem{{Phase: "early", Card: blink}}},
		{Position: 3, Source: "opendota", Name: "Popular", Games: &games, WinRate: &wr, Items: []BuildItem{{Phase: "mid", Card: blink}, {Phase: "late", Card: bkb}}},
	}
	if fmt.Sprintf("%+v", flat(got)) != fmt.Sprintf("%+v", flat(want)) {
		t.Fatalf("builds\n got %+v\nwant %+v", flat(got), flat(want))
	}
	if none, err := s.HeroBuilds(context.Background(), 99); err != nil || len(none) != 0 || none == nil {
		t.Fatalf("unknown hero: %v, %v; want an empty list", none, err)
	}
}

func TestNamesGiveItemCardsOnlyForTheCurrentPatch(t *testing.T) {
	n, err := NewStore(seed(t)).Names(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []items.Card{
		{ID: 1, Key: "blink", Name: "Blink Dagger", Cost: 2250, Img: "/blink.png"},
		{ID: 116, Key: "black_king_bar", Name: "Black King Bar", Cost: 4050, Img: "/bkb.png"},
	}
	if fmt.Sprint(n.ItemCards) != fmt.Sprint(want) {
		t.Fatalf("item cards %v, want %v", n.ItemCards, want)
	}
}

// flat prints the pointer fields by value so builds compare by content.
func flat(bs []Build) []string {
	out := []string{}
	for _, b := range bs {
		g, w := "-", "-"
		if b.Games != nil {
			g = fmt.Sprint(*b.Games)
		}
		if b.WinRate != nil {
			w = fmt.Sprint(*b.WinRate)
		}
		out = append(out, fmt.Sprintf("%d %s %s %s %s %v", b.Position, b.Source, b.Name, g, w, b.Items))
	}
	return out
}
