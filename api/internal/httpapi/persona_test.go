package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jholhewres/draftmind/api/internal/auth"
	"github.com/jholhewres/draftmind/api/internal/match"
	"github.com/jholhewres/draftmind/api/internal/persona"
)

// accounts serves srv with e-mail sign-in in development; signIn returns
// a fresh session cookie for email, as a new device would get.
func accounts(t *testing.T, srv *Server, pool *pgxpool.Pool) (http.Handler, func(email string) *http.Cookie) {
	t.Helper()
	logs := &bytes.Buffer{}
	a, err := auth.New(pool, "http://localhost:5173", slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.WithLogMailer(); err != nil {
		t.Fatal(err)
	}
	h := srv.WithAuth(a).Handler()
	return h, func(email string) *http.Cookie {
		logs.Reset()
		if rec := send(h, http.MethodPost, "/api/auth/email", `{"email":"`+email+`"}`); rec.Code != http.StatusAccepted {
			t.Fatalf("link: %d %s", rec.Code, rec.Body)
		}
		m := devLink.FindStringSubmatch(logs.String())
		for _, c := range send(h, http.MethodPost, "/api/auth/email/verify", `{"token":"`+m[1]+`"}`).Result().Cookies() {
			if c.Name == auth.CookieName {
				return c
			}
		}
		t.Fatal("no session")
		return nil
	}
}

const (
	blunt = `{"tone":"direct","language":"en","verbosity":"short","focus":"laning"}`
	warm  = `{"tone":"encouraging","language":"pt-BR","verbosity":"normal","focus":"macro"}`
)

func TestSettingsBelongToTheSignedInPlayerAndSurviveANewDevice(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	h, signIn := accounts(t, srv, pool)
	phone := signIn("player@example.com")

	if rec := send(h, http.MethodGet, "/api/me/persona", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("visitor: %d, want 401", rec.Code)
	}
	var got persona.Persona
	rec := send(h, http.MethodGet, "/api/me/persona", "", phone)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got != persona.Default() {
		t.Fatalf("before saving: %d %s, want the default", rec.Code, rec.Body)
	}
	if rec := send(h, http.MethodPut, "/api/me/persona", warm, phone); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{`{"tone":"rude","language":"en","verbosity":"short","focus":"laning"}`, `{"tone":"direct"}`} {
		if rec := send(h, http.MethodPut, "/api/me/persona", bad, phone); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", bad, rec.Code)
		}
	}

	// A new device signs in again and finds the same settings.
	laptop := signIn("player@example.com")
	rec = send(h, http.MethodGet, "/api/me/persona", "", laptop)
	got = persona.Persona{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil ||
		got != (persona.Persona{Tone: "encouraging", Language: "pt-BR", Verbosity: "normal", Focus: "macro"}) {
		t.Fatalf("new device: %d %s, want the saved persona", rec.Code, rec.Body)
	}
	// Another player has their own.
	rec = send(h, http.MethodGet, "/api/me/persona", "", signIn("other@example.com"))
	got = persona.Persona{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got != persona.Default() {
		t.Fatalf("other player: %s, want the default", rec.Body)
	}
}

func TestTheCoachAnswersInThePersonasLanguage(t *testing.T) {
	coach := &recordingCoach{}
	srv, pool := buildPatchesPool(t)
	h, signIn := accounts(t, srv.WithCoach(coach), pool)
	me := signIn("player@example.com")
	if rec := send(h, http.MethodPut, "/api/me/persona", warm, me); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	id := startTestMatch(t, h)

	// The UI is in English; the signed-in player's persona speaks pt-BR, and
	// the persona the request carries does not override the saved one.
	send(h, http.MethodPost, "/api/match/"+id+"/messages?lang=en&tone=direct&language=en", `{"text":"pudge is roaming"}`, me)
	// A visitor's persona comes with the request; auto follows the UI.
	send(h, http.MethodPost, "/api/match/"+id+"/messages?lang=en&tone=analytic&language=pt-BR&verbosity=short&focus=items", `{"text":"pudge is roaming"}`)
	send(h, http.MethodPost, "/api/match/"+id+"/messages?lang=en&language=auto", `{"text":"pudge is roaming"}`)

	answered := []string{}
	for i, l := range coach.langs {
		answered = append(answered, coach.personas[i].LangFor(l))
	}
	if strings.Join(answered, ",") != "pt-BR,pt-BR,en" {
		t.Fatalf("answered in %v, want pt-BR, pt-BR, en", answered)
	}
	if coach.personas[0].Tone != "encouraging" || coach.personas[1] != (persona.Persona{Tone: "analytic", Language: "pt-BR", Verbosity: "short", Focus: "items"}) ||
		coach.personas[2] != persona.Default() {
		t.Fatalf("personas %+v", coach.personas)
	}
	if rec := send(h, http.MethodPost, "/api/match/"+id+"/messages?tone=rude", `{"text":"hi"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a bad persona: %d, want 400", rec.Code)
	}
}

func TestTwoPersonasGivenTheSameMatchGetTheSamePicksAndItems(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	h, signIn := accounts(t, srv, pool)
	a, b := signIn("a@example.com"), signIn("b@example.com")
	send(h, http.MethodPut, "/api/me/persona", blunt, a)
	send(h, http.MethodPut, "/api/me/persona", warm, b)

	draftBody := `{"allies":[],"enemies":[2],"bans":[],"role":1}`
	pa := send(h, http.MethodPost, "/api/draft/suggest?lang=en", draftBody, a)
	pb := send(h, http.MethodPost, "/api/draft/suggest?lang=pt-BR", draftBody, b)
	if pa.Code != http.StatusOK || pa.Body.String() != pb.Body.String() {
		t.Fatalf("picks differ by persona:\n%s\n%s", pa.Body, pb.Body)
	}
	start := `{"hero":3,"role":1,"allies":[],"enemies":[2],"bans":[]}`
	var ma, mb match.Session
	if err := json.Unmarshal(send(h, http.MethodPost, "/api/match", start, a).Body.Bytes(), &ma); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(send(h, http.MethodPost, "/api/match", start, b).Body.Bytes(), &mb); err != nil {
		t.Fatal(err)
	}
	ia, _ := json.Marshal(ma.ItemPlan)
	ib, _ := json.Marshal(mb.ItemPlan)
	if len(ma.ItemPlan.Core) == 0 || string(ia) != string(ib) {
		t.Fatalf("items differ by persona:\n%s\n%s", ia, ib)
	}
}

func TestTheUILanguageIsKeptForTheSignedInPlayer(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	h, signIn := accounts(t, srv, pool)
	phone := signIn("player@example.com")

	if rec := send(h, http.MethodPut, "/api/me/language", `{"language":"en"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("visitor: %d, want 401", rec.Code)
	}
	if rec := send(h, http.MethodGet, "/api/me/language", "", phone); rec.Body.String() != "{\"language\":\"\"}\n" {
		t.Fatalf("before saving: %d %s, want no language", rec.Code, rec.Body)
	}
	if rec := send(h, http.MethodPut, "/api/me/language", `{"language":"en"}`, phone); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{`{"language":"fr"}`, `{"language":"auto"}`, `{}`} {
		if rec := send(h, http.MethodPut, "/api/me/language", bad, phone); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", bad, rec.Code)
		}
	}
	// A new device finds it; another player has none.
	if rec := send(h, http.MethodGet, "/api/me/language", "", signIn("player@example.com")); rec.Body.String() != "{\"language\":\"en\"}\n" {
		t.Fatalf("new device: %d %s, want en", rec.Code, rec.Body)
	}
	if rec := send(h, http.MethodGet, "/api/me/language", "", signIn("other@example.com")); rec.Body.String() != "{\"language\":\"\"}\n" {
		t.Fatalf("other player: %s, want no language", rec.Body)
	}
}
