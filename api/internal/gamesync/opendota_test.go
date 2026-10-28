package gamesync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixtureHeroIDs are the hero ids a fixture names, plus extra.
func fixtureHeroIDs(t *testing.T, name string, extra ...int) map[int]bool {
	t.Helper()
	var rows []struct {
		HeroID int `json:"hero_id"`
	}
	if err := json.Unmarshal(readFixture(t, name), &rows); err != nil {
		t.Fatal(err)
	}
	ids := map[int]bool{}
	for _, r := range rows {
		ids[r.HeroID] = true
	}
	for _, id := range extra {
		ids[id] = true
	}
	return ids
}

func TestMatchupParseReadsTheOpenDotaFixture(t *testing.T) {
	inPatch := fixtureHeroIDs(t, "opendota_matchups_2.json", 2)
	got, err := ParseMatchups(readFixture(t, "opendota_matchups_2.json"), inPatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 126 {
		t.Fatalf("%d matchups, want 126", len(got))
	}
	if got[0] != (Matchup{OpponentID: 11, Games: 355, Wins: 177}) {
		t.Fatalf("first matchup %+v, want Axe vs 11: 355 games, 177 wins", got[0])
	}
}

func TestMatchupParseSkipsAnOpponentThePatchDoesNotHave(t *testing.T) {
	got, err := ParseMatchups([]byte(`[{"hero_id": 13, "games_played": 10, "wins": 4}, {"hero_id": 999, "games_played": 5, "wins": 1}]`),
		map[int]bool{2: true, 13: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (Matchup{OpponentID: 13, Games: 10, Wins: 4}) {
		t.Fatalf("got %+v, want only the opponent on the patch", got)
	}
}

func TestMatchupParseRejectsMoreWinsThanGames(t *testing.T) {
	_, err := ParseMatchups([]byte(`[{"hero_id": 13, "games_played": 3, "wins": 4}]`), map[int]bool{13: true})
	if err == nil {
		t.Fatal("want an error for 4 wins in 3 games")
	}
}

func TestMatchupParseHeroStatsReadsEveryBracket(t *testing.T) {
	got, err := ParseHeroStats(readFixture(t, "opendota_herostats.json"), map[int]bool{1: true, 2: true})
	if err != nil {
		t.Fatal(err)
	}
	var am []HeroStat
	for _, s := range got {
		if s.HeroID == 13 {
			t.Fatalf("hero 13 is not on the patch: %+v", s)
		}
		if s.HeroID == 1 {
			am = append(am, s)
		}
	}
	want := []HeroStat{
		{1, "1", 13045, 6667}, {1, "2", 46721, 23872}, {1, "3", 92990, 47507}, {1, "4", 155037, 79930},
		{1, "5", 178789, 91448}, {1, "6", 101055, 51530}, {1, "7", 70607, 35810}, {1, "8", 0, 0}, {1, "pro", 14, 7},
	}
	if len(am) != len(want) {
		t.Fatalf("Anti-Mage stats %+v, want %+v", am, want)
	}
	for i := range want {
		if am[i] != want[i] {
			t.Errorf("bracket %d: %+v, want %+v", i, am[i], want[i])
		}
	}
}

// openDotaStub records each request's path, query and arrival time and
// answers with the statuses given, then 200 with "[]".
type openDotaStub struct {
	mu       sync.Mutex
	statuses []int
	asked    []string
	at       []time.Time
}

func (o *openDotaStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.asked = append(o.asked, r.URL.RequestURI())
	o.at = append(o.at, time.Now())
	if len(o.statuses) > 0 {
		status := o.statuses[0]
		o.statuses = o.statuses[1:]
		if status != http.StatusOK {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(status)
			return
		}
	}
	_, _ = w.Write([]byte("[]"))
}

func TestMatchupHTTPOpenDotaKeepsOneRequestPerSecondWithoutAKey(t *testing.T) {
	stub := &openDotaStub{}
	srv := httptest.NewServer(stub)
	defer srv.Close()
	od := NewHTTPOpenDota(srv.URL, "", srv.Client())
	ctx := context.Background()
	if _, err := od.HeroStats(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := od.Matchups(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := od.Matchups(ctx, 13); err != nil {
		t.Fatal(err)
	}
	want := []string{"/heroStats", "/heroes/2/matchups", "/heroes/13/matchups"}
	if strings.Join(stub.asked, " ") != strings.Join(want, " ") {
		t.Fatalf("asked %v, want %v", stub.asked, want)
	}
	for i := 1; i < len(stub.at); i++ {
		if gap := stub.at[i].Sub(stub.at[i-1]); gap < time.Second {
			t.Fatalf("request %d came %v after the one before, want at least 1 s", i, gap)
		}
	}
}

func TestMatchupHTTPOpenDotaSendsTheKeyAndKeepsItOutOfErrors(t *testing.T) {
	stub := &openDotaStub{statuses: []int{http.StatusOK, http.StatusNotFound}}
	srv := httptest.NewServer(stub)
	defer srv.Close()
	od := NewHTTPOpenDota(srv.URL, "secret-key", srv.Client())
	if _, err := od.Matchups(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	_, err := od.Matchups(context.Background(), 13)
	if err == nil || strings.Contains(err.Error(), "secret-key") || !strings.Contains(err.Error(), "/heroes/13/matchups") {
		t.Fatalf("error %v, want the path and status without the key", err)
	}
	if stub.asked[0] != "/heroes/2/matchups?api_key=secret-key" {
		t.Fatalf("asked %q, want the key as api_key", stub.asked[0])
	}
	if gap := stub.at[1].Sub(stub.at[0]); gap >= time.Second {
		t.Fatalf("with a key, requests came %v apart, want less than the keyless 1 s", gap)
	}
}

func TestMatchupHTTPOpenDotaRetriesARateLimitedRequest(t *testing.T) {
	stub := &openDotaStub{statuses: []int{http.StatusTooManyRequests, http.StatusBadGateway}}
	srv := httptest.NewServer(stub)
	defer srv.Close()
	od := NewHTTPOpenDota(srv.URL, "k", srv.Client())
	b, err := od.Matchups(context.Background(), 2)
	if err != nil || string(b) != "[]" || len(stub.asked) != 3 {
		t.Fatalf("got %q, %v after %d requests, want the third answer", b, err, len(stub.asked))
	}
}
