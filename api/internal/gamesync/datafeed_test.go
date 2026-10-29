package gamesync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

var patchWithPuckAndAxe = map[int]bool{2: true, 13: true}

func TestDatafeedParsesStatsAbilitiesAndTalents(t *testing.T) {
	d, err := ParseHeroData(readFixture(t, "herodata_13_english.json"), patchWithPuckAndAxe)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != 13 || d.Name != "Puck" || d.Complexity != 2 {
		t.Fatalf("hero %d %q complexity %d, want 13 Puck 2", d.ID, d.Name, d.Complexity)
	}
	s := d.Stats
	if s.StrBase != 17 || s.StrGain != 2.4 || s.AgiBase != 14 || s.IntBase != 23 || s.IntGain != 3.8 ||
		s.DamageMin != 45 || s.DamageMax != 53 || s.AttackRate != 1.7 || s.AttackRange != 550 ||
		s.MovementSpeed != 290 || s.MaxHealth != 494 || s.MaxMana != 351 || s.Armor != 2.3333335 {
		t.Fatalf("unexpected stats: %+v", s)
	}
	if !strings.HasPrefix(d.Hype, "Puck has a talent for confounding its enemies with playfully elusive mischief.") {
		t.Fatalf("hype keeps its markup: %q", d.Hype)
	}

	var names []string
	for _, a := range d.Abilities {
		names = append(names, a.Name+":"+string(a.Kind))
	}
	want := []string{"puck_illusory_orb:ability", "puck_waning_rift:ability", "puck_phase_shift:ability",
		"puck_puckish:innate", "puck_dream_coil:ability"}
	if !slices.Equal(names, want) {
		t.Fatalf("abilities %v, want %v", names, want)
	}
	orb := d.Abilities[0]
	if orb.ID != 5069 || orb.Text.Name != "Illusory Orb" || !slices.Equal(orb.Cooldowns, []float64{12, 11, 10, 9}) ||
		!slices.Equal(orb.ManaCosts, []float64{90, 100, 110, 120}) {
		t.Fatalf("unexpected orb: %+v", orb)
	}
	// Placeholders take the ability's values; markup becomes plain text.
	if !strings.Contains(orb.Text.Description, "an additional 3% of its impact damage every 0.5s") ||
		!strings.Contains(orb.Text.Description, "Ethereal Jaunt.\n\n Can be put on alt-cast") {
		t.Fatalf("orb description: %q", orb.Text.Description)
	}
	if puckish := d.Abilities[3]; len(puckish.Cooldowns) != 0 || len(puckish.ManaCosts) != 0 {
		t.Fatalf("an all-zero cooldown or cost means none: %+v", puckish)
	}

	if len(d.Talents) != 8 {
		t.Fatalf("got %d talents, want 8", len(d.Talents))
	}
	// {s:bonus_X} is the talent's bonus on the ability value X.
	if tl := d.Talents[0]; tl.ID != 1057 || tl.Name != "special_bonus_unique_puck_orb_damage" || tl.Text.Name != "+35 Illusory Orb Damage" {
		t.Fatalf("first talent %+v", tl)
	}
	if tl := d.Talents[6]; tl.Text.Name != "-30s Dream Coil Cooldown" {
		t.Fatalf("seventh talent %q", tl.Text.Name)
	}
}

func TestDatafeedResolvesATalentsOwnValue(t *testing.T) {
	d, err := ParseHeroData(readFixture(t, "herodata_2_english.json"), patchWithPuckAndAxe)
	if err != nil {
		t.Fatal(err)
	}
	if tl := d.Talents[4]; tl.Text.Name != "+15 Strength" {
		t.Fatalf("talent %q, want +15 Strength", tl.Text.Name)
	}
}

func TestDatafeedReadsTheLocalizedText(t *testing.T) {
	d, err := ParseHeroData(readFixture(t, "herodata_13_brazilian.json"), patchWithPuckAndAxe)
	if err != nil {
		t.Fatal(err)
	}
	if d.Abilities[0].Text.Name != "Orbe Ilusório" || d.Abilities[3].Text.Name != "Puckz Grila" {
		t.Fatalf("abilities not in Portuguese: %q, %q", d.Abilities[0].Text.Name, d.Abilities[3].Text.Name)
	}
	if tl := d.Talents[0]; tl.Text.Name != "+35 de dano para Orbe Ilusório" {
		t.Fatalf("talent %q", tl.Text.Name)
	}
}

func TestDatafeedRejectsAHeroThePatchDoesNotHave(t *testing.T) {
	_, err := ParseHeroData(readFixture(t, "herodata_13_english.json"), map[int]bool{1: true, 2: true, 5: true})
	if err == nil || !strings.Contains(err.Error(), "13") {
		t.Fatalf("err = %v, want a rejection naming hero 13", err)
	}
}

func TestDatafeedRejectsAResponseWithoutAHero(t *testing.T) {
	if _, err := ParseHeroData([]byte(`{"result": {"data": {"heroes": []}, "status": 1}}`), patchWithPuckAndAxe); err == nil {
		t.Fatal("want an error for a response without a hero")
	}
}

func TestDatafeedHTTPFeedAsksForTheHeroInTheLanguage(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Path+"?"+r.URL.Query().Encode())
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	feed := NewHTTPFeed(srv.URL, 0, srv.Client())
	ctx := context.Background()
	if _, err := feed.HeroData(ctx, 13, "brazilian"); err != nil {
		t.Fatal(err)
	}
	if _, err := feed.PatchList(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := feed.PatchNotes(ctx, "7.41f", "brazilian"); err != nil {
		t.Fatal(err)
	}
	want := []string{"/datafeed/herodata?hero_id=13&language=brazilian", "/datafeed/patchnoteslist?language=english",
		"/datafeed/patchnotes?language=brazilian&version=7.41f"}
	if !slices.Equal(got, want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
}

func TestDatafeedHTTPFeedSpacesRequestsByTheInterval(t *testing.T) {
	var at []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		at = append(at, time.Now())
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	const interval = 40 * time.Millisecond
	feed := NewHTTPFeed(srv.URL, interval, srv.Client())
	for range 3 {
		if _, err := feed.HeroData(context.Background(), 2, "english"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i < len(at); i++ {
		if gap := at[i].Sub(at[i-1]); gap < interval {
			t.Fatalf("request %d came %s after the previous one, want at least %s", i, gap, interval)
		}
	}
}

func TestDatafeedHTTPFeedFailsOnAnErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	if _, err := NewHTTPFeed(srv.URL, 0, srv.Client()).HeroData(context.Background(), 2, "english"); err == nil ||
		!strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v, want HTTP 429", err)
	}
}

func TestDatafeedFillsDurationChannelAndCastPointFromTheAbility(t *testing.T) {
	b := []byte(`{"result": {"data": {"heroes": [{"id": 2, "name_loc": "Axe", "max_health": 600, "max_mana": 200, "movement_speed": 300, "attack_rate": 1.7, "abilities": [{
		"id": 1, "name": "x_spell", "name_loc": "Spell",
		"desc_loc": "Lasts %abilityduration%s, channels %AbilityChannelTime%s, casts in %abilitycastpoint%s, hits %Damage%.",
		"durations": [2, 3], "channel_times": [4], "cast_points": [0.3],
		"special_values": [{"name": "damage", "values_float": [100]}]}]}]}}}`)
	d, err := ParseHeroData(b, patchWithPuckAndAxe)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := d.Abilities[0].Text.Description, "Lasts 2 / 3s, channels 4s, casts in 0.3s, hits 100."; got != want {
		t.Fatalf("description %q, want %q", got, want)
	}
}

func TestDatafeedRejectsAHeroWithoutStats(t *testing.T) {
	b := []byte(`{"result": {"data": {"heroes": [{"id": 2, "name_loc": "Axe", "abilities": []}]}}}`)
	if _, err := ParseHeroData(b, patchWithPuckAndAxe); err == nil || !strings.Contains(err.Error(), "stats") {
		t.Fatalf("err = %v, want a rejection for missing stats", err)
	}
}

func TestDatafeedHTTPFeedRetriesA502AndANullBody(t *testing.T) {
	answers := []func(http.ResponseWriter){
		func(w http.ResponseWriter) { http.Error(w, "bad gateway", http.StatusBadGateway) },
		func(w http.ResponseWriter) { _, _ = w.Write([]byte("null")) },
		func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"result":{}}`)) },
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		answers[min(calls, len(answers)-1)](w)
		calls++
	}))
	t.Cleanup(srv.Close)
	b, err := NewHTTPFeed(srv.URL, 0, srv.Client()).HeroData(context.Background(), 2, "english")
	if err != nil || string(b) != `{"result":{}}` || calls != 3 {
		t.Fatalf("got %q, %v after %d requests; want the third answer", b, err, calls)
	}
}
