package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/match"
)

func TestTheMatchSessionSurvivesAPageReload(t *testing.T) {
	srv := buildPatches(t)
	h := srv.Handler()
	do := func(method, target, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
		return rec
	}

	rec := do(http.MethodPost, "/api/match", `{"hero":1,"role":2,"allies":[],"enemies":[2],"bans":[3]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	var started match.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if !match.ValidID(started.ID) || started.Patch != "7.41" {
		t.Fatalf("started %+v, want a random id on 7.41", started)
	}
	if rec := do(http.MethodPost, "/api/match/"+started.ID+"/messages", `{"text":"viper is killing me mid"}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), "event: answer") || !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("message: %d %s", rec.Code, rec.Body)
	}

	// The reloaded page asks for the session by the id it kept.
	rec = do(http.MethodGet, "/api/match/"+started.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reload: %d %s", rec.Code, rec.Body)
	}
	var got match.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != started.ID || got.HeroID != 1 || got.Role != 2 || len(got.Enemies) != 1 || got.Enemies[0] != 2 ||
		len(got.Bans) != 1 || len(got.ItemPlan.Situational) != 1 || got.ItemPlan.Situational[0].EnemyID != 2 {
		t.Fatalf("reloaded %+v, want the started session with its item plan", got)
	}
	if len(got.Messages) != 2 || got.Messages[0].Author != "player" || got.Messages[0].Text != "viper is killing me mid" ||
		got.Messages[1].Author != "coach" || got.Messages[1].Kind != "answer" || got.Messages[1].Reason != "unavailable" {
		t.Fatalf("messages %+v, want the player's and the coach's", got.Messages)
	}
	for _, id := range []string{"00000000-0000-4000-8000-000000000000", "not-a-uuid"} {
		if rec := do(http.MethodGet, "/api/match/"+id, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s: %d, want 404", id, rec.Code)
		}
	}
}
