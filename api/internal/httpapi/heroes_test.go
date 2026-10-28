package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/dbtest"
	"github.com/jholhewres/draftmind/api/internal/gamesync"
)

func TestListHeroesReturnsEveryHeroOfTheCurrentPatch(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO patch (name, is_current) VALUES ('7.40', false), ('7.41', true);
		INSERT INTO hero (id, slug, name, primary_attr, attack_type, roles, img, icon) VALUES
			(2, 'axe', 'Axe', 'str', 'Melee', '{Initiator}', '/axe.png', '/axe_icon.png'),
			(1, 'antimage', 'Anti-Mage', 'agi', 'Melee', '{Carry,Escape}', '/am.png', '/am_icon.png'),
			(5, 'crystal_maiden', 'Crystal Maiden', 'int', 'Ranged', '{Support}', '/cm.png', '/cm_icon.png');
		INSERT INTO hero_patch (patch_id, hero_id) SELECT p.id, h.id FROM patch p CROSS JOIN hero h WHERE p.name = '7.41'`); err != nil {
		t.Fatal(err)
	}
	srv := New(fakeDB{}, catalog.NewStore(pool), quietLog())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/heroes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Patch  string         `json:"patch"`
		Heroes []catalog.Hero `json:"heroes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Patch != "7.41" || len(body.Heroes) != 3 {
		t.Fatalf("patch %q with %d heroes, want 7.41 with 3", body.Patch, len(body.Heroes))
	}
	if body.Heroes[0].Name != "Anti-Mage" || body.Heroes[2].Slug != "crystal_maiden" ||
		len(body.Heroes[0].Roles) != 2 {
		t.Fatalf("heroes not sorted by name or fields missing: %+v", body.Heroes)
	}
}

// memSource serves dotaconstants files from memory.
type memSource map[string]string

func (m memSource) Revision(context.Context) (string, error) { return "fixture", nil }

func (m memSource) Fetch(_ context.Context, file string) ([]byte, error) {
	b, ok := m[file]
	if !ok {
		return nil, fmt.Errorf("no fixture %s", file)
	}
	return []byte(b), nil
}

const (
	twoPatches = `[{"name": "7.40", "date": "2025-12-16T00:00:00Z"}, {"name": "7.41", "date": "2026-03-24T00:00:00Z"}]`
	antimage   = `"1": {"id": 1, "name": "npc_dota_hero_antimage", "localized_name": "Anti-Mage", "primary_attr": "agi", "attack_type": "Melee", "roles": ["Carry"], "img": "/am.png?", "icon": "/am_icon.png?"}`
	axe        = `"2": {"id": 2, "name": "npc_dota_hero_axe", "localized_name": "Axe", "primary_attr": "str", "attack_type": "Melee", "roles": ["Initiator"], "img": "/axe.png?", "icon": "/axe_icon.png?"}`
)

func TestListHeroesOmitsAHeroMissingFromTheLatestSync(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	full := memSource{"patch.json": twoPatches, "heroes.json": "{" + antimage + "," + axe + "}"}
	if _, err := gamesync.NewSyncer(pool, full, quietLog()).Heroes(ctx); err != nil {
		t.Fatal(err)
	}
	withoutAxe := memSource{"patch.json": twoPatches, "heroes.json": "{" + antimage + "}"}
	if _, err := gamesync.NewSyncer(pool, withoutAxe, quietLog()).Heroes(ctx); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	New(fakeDB{}, catalog.NewStore(pool), quietLog()).Handler().
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/heroes", nil))
	var body struct {
		Heroes []catalog.Hero `json:"heroes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Heroes) != 1 || body.Heroes[0].Slug != "antimage" {
		t.Fatalf("heroes %+v, want only antimage", body.Heroes)
	}
}
