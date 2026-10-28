package gamesync

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

// fixtureFeed serves testdata/herodata_<id>_<language>.json; a hero without
// a fixture fails like an unreachable feed.
type fixtureFeed struct{}

func (f fixtureFeed) HeroData(_ context.Context, heroID int, language string) ([]byte, error) {
	b, err := readFixtureFile(fmt.Sprintf("herodata_%d_%s.json", heroID, language))
	if err != nil {
		return nil, fmt.Errorf("feed: no hero %d in %s", heroID, language)
	}
	return b, nil
}

const puckAndAxe = `{
	"2": {"id": 2, "name": "npc_dota_hero_axe", "primary_attr": "str", "attack_type": "Melee", "roles": ["Initiator"], "img": "/axe.png?", "icon": "/axe_icon.png?", "localized_name": "Axe"},
	"13": {"id": 13, "name": "npc_dota_hero_puck", "primary_attr": "int", "attack_type": "Ranged", "roles": ["Initiator", "Escape"], "img": "/puck.png?", "icon": "/puck_icon.png?", "localized_name": "Puck"}}`

// syncPuckAndAxe makes 7.41 current with Puck and Axe on it.
func syncPuckAndAxe(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	src := fixtureSource{t: t, files: map[string]string{"heroes.json": puckAndAxe}}
	if _, err := NewSyncer(pool, src, quietLog()).Heroes(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDatafeedSecondSyncChangesZeroRows(t *testing.T) {
	pool := dbtest.New(t)
	syncPuckAndAxe(t, pool)
	ctx := context.Background()
	s := NewSyncer(pool, nil, quietLog())

	first, err := s.Details(ctx, fixtureFeed{})
	if err != nil {
		t.Fatal(err)
	}
	// 2 hero_patch rows get stats; 2 heroes x 2 languages of hero_text;
	// 26 abilities and talents; each in 2 languages.
	if first.Updated != 2 || first.Inserted != 4+26+52 || first.Deleted != 0 {
		t.Fatalf("first run %+v, want 2 updated and 82 inserted", first)
	}
	before := detailVersions(t, pool)
	second, err := s.Details(ctx, fixtureFeed{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() != 0 || second.Unchanged != 84 {
		t.Fatalf("second run %+v, want 0 changed and 84 unchanged", second)
	}
	if after := detailVersions(t, pool); after != before {
		t.Fatalf("second run rewrote rows: xmin %s, was %s", after, before)
	}
}

// detailVersions lists the xmin of every row the details sync writes.
func detailVersions(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var v string
	err := pool.QueryRow(context.Background(), `
		SELECT concat_ws(' ',
			(SELECT string_agg(xmin::text, ',' ORDER BY patch_id, hero_id) FROM hero_patch),
			(SELECT string_agg(xmin::text, ',' ORDER BY patch_id, hero_id, lang) FROM hero_text),
			(SELECT string_agg(xmin::text, ',' ORDER BY patch_id, hero_id, id) FROM ability),
			(SELECT string_agg(xmin::text, ',' ORDER BY patch_id, hero_id, ability_id, lang) FROM ability_text))`).Scan(&v)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDatafeedSyncStoresStatsAndTextPerLanguage(t *testing.T) {
	pool := dbtest.New(t)
	syncPuckAndAxe(t, pool)
	ctx := context.Background()
	if _, err := NewSyncer(pool, nil, quietLog()).Details(ctx, fixtureFeed{}); err != nil {
		t.Fatal(err)
	}
	var health, attackRange int
	if err := pool.QueryRow(ctx, `SELECT max_health, attack_range FROM hero_patch WHERE hero_id = 13`).Scan(&health, &attackRange); err != nil {
		t.Fatal(err)
	}
	if health != 494 || attackRange != 550 {
		t.Fatalf("puck health %d range %d, want 494 and 550", health, attackRange)
	}
	var name string
	if err := pool.QueryRow(ctx, `
		SELECT t.name FROM ability_text t JOIN ability a ON (a.patch_id, a.hero_id, a.id) = (t.patch_id, t.hero_id, t.ability_id)
		WHERE a.name = 'puck_dream_coil' AND t.lang = 'pt-BR'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Espiral dos Sonhos" {
		t.Fatalf("dream coil in pt-BR is %q", name)
	}
}

func TestDatafeedSyncKeepsGoingPastAHeroThatFails(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	// Anti-Mage (1), Axe (2) and Crystal Maiden (5); the feed only has Axe.
	if _, err := NewSyncer(pool, fixtureSource{t: t}, quietLog()).Heroes(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := NewSyncer(pool, nil, quietLog()).Details(ctx, fixtureFeed{})
	if err == nil || !strings.Contains(err.Error(), "hero 1") || !strings.Contains(err.Error(), "hero 5") {
		t.Fatalf("err = %v, want one naming heroes 1 and 5", err)
	}
	var abilities int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ability WHERE hero_id = 2`).Scan(&abilities); err != nil {
		t.Fatal(err)
	}
	if abilities != 13 {
		t.Fatalf("axe has %d abilities and talents, want 13", abilities)
	}
	var status string
	var errText *string
	if err := pool.QueryRow(ctx, `SELECT status, error FROM sync_run WHERE source = 'datafeed:herodata'`).Scan(&status, &errText); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || errText == nil || !strings.Contains(*errText, "hero 5") {
		t.Fatalf("sync_run %s %v, want failed naming hero 5", status, errText)
	}
}

func TestDatafeedSyncRejectsAResponseForAnotherHero(t *testing.T) {
	pool := dbtest.New(t)
	syncPuckAndAxe(t, pool)
	_, err := NewSyncer(pool, nil, quietLog()).Details(context.Background(), swappedFeed{fixtureFeed{}})
	if err == nil || !strings.Contains(err.Error(), "asked for hero 2") {
		t.Fatalf("err = %v, want a rejection of the swapped response", err)
	}
}

// swappedFeed answers Puck's data when asked for Axe.
type swappedFeed struct{ fixtureFeed }

func (f swappedFeed) HeroData(ctx context.Context, heroID int, language string) ([]byte, error) {
	if heroID == 2 {
		heroID = 13
	}
	return f.fixtureFeed.HeroData(ctx, heroID, language)
}
