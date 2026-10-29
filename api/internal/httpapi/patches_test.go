package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/dbtest"
	"github.com/jholhewres/draftmind/api/internal/gamesync"
)

// testdataNotes serves the gamesync patch notes fixtures: 7.41e and 7.41f.
type testdataNotes struct{}

func (testdataNotes) PatchList(context.Context) ([]byte, error) {
	return os.ReadFile("../gamesync/testdata/patchnoteslist.json")
}

func (testdataNotes) PatchNotes(_ context.Context, patch, language string) ([]byte, error) {
	return os.ReadFile(fmt.Sprintf("../gamesync/testdata/patchnotes_%s_%s.json", patch, language))
}

const bane = `"3": {"id": 3, "name": "npc_dota_hero_bane", "localized_name": "Bane", "primary_attr": "all", "attack_type": "Ranged", "roles": ["Support"], "img": "/bane.png?", "icon": "/bane_icon.png?"}`

// syncedPatchNotes syncs 7.41 with Anti-Mage, Axe and Bane, then the notes
// of the feed's last two patches, 7.41e and 7.41f.
func syncedPatchNotes(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := dbtest.New(t)
	ctx := context.Background()
	src := memSource{"patch.json": twoPatches, "heroes.json": "{" + antimage + "," + axe + "," + bane + "}"}
	s := gamesync.NewSyncer(pool, src, quietLog())
	if _, err := s.Heroes(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PatchNotes(ctx, testdataNotes{}, 2); err != nil {
		t.Fatal(err)
	}
	return pool
}

func get[T any](t *testing.T, pool *pgxpool.Pool, target string) (int, T) {
	t.Helper()
	rec := httptest.NewRecorder()
	New(fakeDB{}, catalog.NewStore(pool), quietLog()).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	var body T
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, body
}

func TestHeroHistoryReturnsTheHerosNotesNewestPatchFirst(t *testing.T) {
	pool := syncedPatchNotes(t)

	code, en := get[catalog.HeroHistory](t, pool, "/api/heroes/bane/history?lang=en")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if en.Slug != "bane" || len(en.Patches) != 2 || en.Patches[0].Name != "7.41f" || en.Patches[1].Name != "7.41e" {
		t.Fatalf("history %+v, want 7.41f then 7.41e", en)
	}
	if n := en.Patches[0].Notes; len(n) != 1 || n[0].Kind != "hero" || n[0].Text != "Base Armor decreased by 1" {
		t.Fatalf("7.41f notes %+v", n)
	}
	if n := en.Patches[1].Notes; len(n) != 5 || n[0].Kind != "ability" || n[0].Text != "Max Terrors per hero increased from 5 to 6" {
		t.Fatalf("7.41e notes %+v", n)
	}

	_, pt := get[catalog.HeroHistory](t, pool, "/api/heroes/bane/history?lang=pt-BR")
	if pt.Lang != "pt-BR" || len(pt.Patches) != 2 || pt.Patches[0].Notes[0].Text != "Redução da armadura base em 1" {
		t.Fatalf("pt-BR history %+v", pt)
	}
	if code, _ := get[catalog.HeroHistory](t, pool, "/api/heroes/nobody/history"); code != http.StatusNotFound {
		t.Fatalf("unknown hero: status %d, want 404", code)
	}
}

func TestPatchReturnsItsHeroAndItemChanges(t *testing.T) {
	pool := syncedPatchNotes(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO item (patch_id, id, key, dname) SELECT id, 1593, 'mysterious_hat', 'Fairy''s Trinket' FROM patch WHERE is_current`); err != nil {
		t.Fatal(err)
	}

	code, list := get[struct{ Patches []catalog.PatchSummary }](t, pool, "/api/patches")
	if code != http.StatusOK || len(list.Patches) != 2 || list.Patches[0].Name != "7.41f" {
		t.Fatalf("status %d, patches %+v", code, list.Patches)
	}

	code, p := get[catalog.PatchChanges](t, pool, "/api/patches/7.41f?lang=en")
	if code != http.StatusOK || p.Name != "7.41f" {
		t.Fatalf("status %d, patch %+v", code, p)
	}
	// Of the patch's heroes only those in the database: Anti-Mage and Bane.
	if len(p.Heroes) != 2 || p.Heroes[0].Slug != "antimage" || p.Heroes[1].Slug != "bane" ||
		p.Heroes[1].Notes[0].Text != "Base Armor decreased by 1" {
		t.Fatalf("heroes %+v", p.Heroes)
	}
	// Of its items only those the database names.
	if len(p.Items) != 1 || p.Items[0].Name != "Fairy's Trinket" ||
		p.Items[0].Notes[0].Text != "Attack Damage penalty increased from 30/60 to 35/70" {
		t.Fatalf("items %+v", p.Items)
	}
	if code, _ := get[catalog.PatchChanges](t, pool, "/api/patches/6.00"); code != http.StatusNotFound {
		t.Fatalf("unknown patch: status %d, want 404", code)
	}
}
