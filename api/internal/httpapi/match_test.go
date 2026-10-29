package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/auth"
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

	rec := do(http.MethodPost, "/api/match", `{"hero":1,"role":2,"allies":[],"enemies":[2],"bans":[3],"mode":"turbo"}`)
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
	if got.Mode != "turbo" {
		t.Fatalf("reloaded mode %q, want turbo", got.Mode)
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

func TestAPlayersMatchAnswers404ToEveryoneElse(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	logs := &bytes.Buffer{}
	a, err := auth.New(pool, "http://localhost:5173", slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.WithLogMailer(); err != nil {
		t.Fatal(err)
	}
	h := srv.WithAuth(a).Handler()
	signIn := func(email string) *http.Cookie {
		logs.Reset()
		send(h, http.MethodPost, "/api/auth/email", `{"email":"`+email+`"}`)
		m := devLink.FindStringSubmatch(logs.String())
		for _, c := range send(h, http.MethodPost, "/api/auth/email/verify", `{"token":"`+m[1]+`"}`).Result().Cookies() {
			if c.Name == auth.CookieName {
				return c
			}
		}
		t.Fatal("no session")
		return nil
	}
	start := func(who ...*http.Cookie) string {
		var s match.Session
		_ = json.Unmarshal(send(h, http.MethodPost, "/api/match", `{"hero":1,"role":2,"enemies":[2]}`, who...).Body.Bytes(), &s)
		return s.ID
	}
	me, other := signIn("player@example.com"), signIn("other@example.com")
	mine, visitors := start(me), start()
	msg := `{"text":"viper is killing me mid"}`

	for _, who := range [][]*http.Cookie{{other}, nil} {
		if rec := send(h, http.MethodGet, "/api/match/"+mine, "", who...); rec.Code != http.StatusNotFound {
			t.Fatalf("someone else read the player's match: %d", rec.Code)
		}
		if rec := send(h, http.MethodPost, "/api/match/"+mine+"/messages", msg, who...); rec.Code != http.StatusNotFound {
			t.Fatalf("someone else posted to the player's match: %d", rec.Code)
		}
	}
	if rec := send(h, http.MethodGet, "/api/match/"+mine, "", me); rec.Code != http.StatusOK {
		t.Fatalf("the player read their match: %d", rec.Code)
	}
	if rec := send(h, http.MethodPost, "/api/match/"+mine+"/messages", msg, me); rec.Code != http.StatusOK {
		t.Fatalf("the player posted to their match: %d", rec.Code)
	}
	if rec := send(h, http.MethodGet, "/api/match/"+visitors, "", other); rec.Code != http.StatusOK {
		t.Fatalf("a visitor's match by its id: %d", rec.Code)
	}
}

func TestTheItemsQuickActionAnswersWithoutTheCoach(t *testing.T) {
	srv := buildPatches(t)
	coach := &recordingCoach{}
	h := srv.WithCoach(coach).Handler()
	id := startTestMatch(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/match/"+id+"/messages", strings.NewReader(`{"text":"Items now?","quick":"items"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"reason":"quick"`) || !strings.Contains(rec.Body.String(), `"items":[{`) {
		t.Fatalf("quick: %d %s", rec.Code, rec.Body)
	}
	if len(coach.texts) != 0 {
		t.Fatalf("the coach was asked %q", coach.texts)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/match/"+id+"/messages", strings.NewReader(`{"text":"x","quick":"lane"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown quick action: %d", rec.Code)
	}
}
