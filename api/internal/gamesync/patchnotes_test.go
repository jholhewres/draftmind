package gamesync

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

func TestPatchNotesParserKeysNotesByHeroAbilityTalentAndItem(t *testing.T) {
	b, err := readFixtureFile("patchnotes_7.41f_english.json")
	if err != nil {
		t.Fatal(err)
	}
	notes, err := ParsePatchNotes(b, "7.41f")
	if err != nil {
		t.Fatal(err)
	}
	if notes.Name != "7.41f" || !notes.ReleasedAt.Equal(time.Unix(1789455600, 0)) {
		t.Fatalf("patch %q released %v", notes.Name, notes.ReleasedAt)
	}
	want := []PatchNote{
		{Kind: NoteHero, HeroID: 3, Indent: 1, Text: "Base Armor decreased by 1"},
		{Kind: NoteAbility, HeroID: 1, TargetID: 5003, Indent: 1, Text: "Mana Burned as Damage increased from 60% to 65%"},
		{Kind: NoteTalent, HeroID: 11, Indent: 1, Text: "Level 15 Talent Presence Armor Reduction decreased from +2 to +1.5"},
		// The feed's <font> markup is stripped to text.
		{Kind: NoteItem, TargetID: 1593, Indent: 1, Text: "Attack Damage penalty increased from 30/60 to 35/70"},
		{Kind: NoteItem, TargetID: 265, Indent: 1, Text: "Mana Regen bonus decreased from +0.8 to +0.6"},
	}
	for _, w := range want {
		if !slices.Contains(notes.Notes, w) {
			t.Errorf("missing note %+v", w)
		}
	}
	counts := map[NoteKind]int{}
	for _, n := range notes.Notes {
		if n.Text == "" {
			t.Errorf("empty note kept: %+v", n)
		}
		counts[n.Kind]++
	}
	// 26 hero, 42 ability, 12 talent notes; 21 item notes less a bare <br> line.
	if counts[NoteHero] != 26 || counts[NoteAbility] != 42 || counts[NoteTalent] != 12 || counts[NoteItem] != 20 {
		t.Fatalf("notes by kind %v", counts)
	}
}

// fixtureNotes serves testdata/patchnoteslist.json and
// testdata/patchnotes_<patch>_<language>.json, counting requests.
type fixtureNotes struct{ calls *int }

func (f fixtureNotes) PatchList(context.Context) ([]byte, error) {
	*f.calls++
	return readFixtureFile("patchnoteslist.json")
}

func (f fixtureNotes) PatchNotes(_ context.Context, patch, language string) ([]byte, error) {
	*f.calls++
	b, err := readFixtureFile(fmt.Sprintf("patchnotes_%s_%s.json", patch, language))
	if err != nil {
		return nil, fmt.Errorf("feed: no patch %s in %s", patch, language)
	}
	return b, nil
}

func TestPatchNotesSecondSyncChangesZeroRows(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	s := NewSyncer(pool, nil, quietLog())
	calls := 0
	feed := fixtureNotes{calls: &calls}

	// The last two patches of the list: 7.41e and 7.41f.
	first, err := s.PatchNotes(ctx, feed, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.Inserted == 0 || first.Updated != 0 || first.Deleted != 0 {
		t.Fatalf("first run %+v, want only inserts", first)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM patch_note`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows+2 != first.Inserted { // the notes and the two patches
		t.Fatalf("%d notes stored, first run inserted %d", rows, first.Inserted)
	}
	before := noteVersions(t, pool)

	second, err := s.PatchNotes(ctx, feed, 2)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() != 0 || second.Unchanged != first.Inserted {
		t.Fatalf("second run %+v, want 0 changed and %d unchanged", second, first.Inserted)
	}
	if after := noteVersions(t, pool); after != before {
		t.Fatal("second run rewrote rows")
	}
	var status, revision string
	if err := pool.QueryRow(ctx, `SELECT status, revision FROM sync_run WHERE source = 'datafeed:patchnotes' ORDER BY id DESC LIMIT 1`).
		Scan(&status, &revision); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || revision != "7.41f" {
		t.Fatalf("sync_run %s at %s, want succeeded at 7.41f", status, revision)
	}
}

func noteVersions(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var v string
	if err := pool.QueryRow(context.Background(), `
		SELECT concat_ws(' ',
			(SELECT string_agg(xmin::text, ',' ORDER BY name) FROM patch_release),
			(SELECT string_agg(xmin::text, ',' ORDER BY patch_name, lang, seq) FROM patch_note))`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}
