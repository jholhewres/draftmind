package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

func TestListHeroesReturnsEveryHeroOfTheCurrentPatch(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO patch (name, is_current) VALUES ('7.40', false), ('7.41', true);
		INSERT INTO hero (id, slug, name, primary_attr, attack_type, roles, img, icon) VALUES
			(2, 'axe', 'Axe', 'str', 'Melee', '{Initiator}', '/axe.png', '/axe_icon.png'),
			(1, 'antimage', 'Anti-Mage', 'agi', 'Melee', '{Carry,Escape}', '/am.png', '/am_icon.png'),
			(5, 'crystal_maiden', 'Crystal Maiden', 'int', 'Ranged', '{Support}', '/cm.png', '/cm_icon.png')`); err != nil {
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
