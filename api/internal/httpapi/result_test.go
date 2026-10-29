package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jholhewres/draftmind/api/internal/auth"
	"github.com/jholhewres/draftmind/api/internal/match"
	"github.com/jholhewres/draftmind/api/internal/persona"
	"github.com/jholhewres/draftmind/api/internal/photo"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

type fakeResultParser map[string]string

func (f fakeResultParser) ParseResult(_ context.Context, text string) (string, error) {
	if r, ok := f[text]; ok {
		return r, nil
	}
	return "", writer.ErrUnclear
}

type fakeEndReader struct {
	read photo.EndRead
	hero string
}

func (f *fakeEndReader) ReadEnd(_ context.Context, _ image.Image, hero string) (photo.EndRead, error) {
	f.hero = hero
	return f.read, nil
}

type fakeNoter struct{ got []match.Result }

func (f *fakeNoter) Notes(_ context.Context, _ match.Session, r match.Result, _ string, _ persona.Persona) writer.Notes {
	f.got = append(f.got, r)
	return writer.Notes{Items: []string{"Axe won the lane.", "Black King Bar turned the fights."}, Source: writer.SourceWriter}
}

func send(h http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func storedResult(t *testing.T, h http.Handler, id string) *match.Result {
	t.Helper()
	var s match.Session
	rec := send(h, http.MethodGet, "/api/match/"+id, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	return s.Result
}

func TestATapOrAMessageRecordsTheResult(t *testing.T) {
	noter := &fakeNoter{}
	srv := buildPatches(t).WithResultParser(fakeResultParser{"perdemos": "loss"}).WithNoter(noter)
	h := srv.Handler()

	// A message is read into a result; nothing is saved yet.
	id := startTestMatch(t, h)
	rec := send(h, http.MethodPost, "/api/match/"+id+"/end/parse", `{"text":"perdemos"}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"result":"loss"}` {
		t.Fatalf("parse: %d %s", rec.Code, rec.Body)
	}
	if rec := send(h, http.MethodPost, "/api/match/"+id+"/end/parse", `{"text":"gg"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unclear parse: %d %s", rec.Code, rec.Body)
	}
	if r := storedResult(t, h, id); r != nil {
		t.Fatalf("saved %+v before the player confirmed", r)
	}
	rec = send(h, http.MethodPost, "/api/match/"+id+"/result?lang=pt-BR", `{"result":"loss","source":"message"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm: %d %s", rec.Code, rec.Body)
	}
	r := storedResult(t, h, id)
	if r == nil || r.Outcome != "loss" || r.Source != "message" || len(r.Notes) != 2 || r.DurationS != nil {
		t.Fatalf("stored %+v, want the loss with the coach's notes", r)
	}

	// A tap is the result as it stands.
	id = startTestMatch(t, h)
	if rec := send(h, http.MethodPost, "/api/match/"+id+"/result", `{"result":"win","source":"tap"}`); rec.Code != http.StatusOK {
		t.Fatalf("tap: %d %s", rec.Code, rec.Body)
	}
	if r := storedResult(t, h, id); r == nil || r.Outcome != "win" || r.Source != "tap" {
		t.Fatalf("stored %+v, want the win", r)
	}
	if len(noter.got) != 2 {
		t.Fatalf("notes written %d times, want once per confirmed result", len(noter.got))
	}

	for _, bad := range []string{`{"result":"draw","source":"tap"}`, `{"result":"win","source":"tap","kills":-1}`, `{"result":"win","source":"guess"}`} {
		if rec := send(h, http.MethodPost, "/api/match/"+id+"/result", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", bad, rec.Code)
		}
	}
}

func TestAnEndScreenPhotoFillsTheResultAndNothingIsSavedUntilConfirmed(t *testing.T) {
	two, nine, three, fourteen := 2292, 9, 3, 14
	reader := &fakeEndReader{read: photo.EndRead{Result: "win", DurationS: &two, Kills: &nine, Deaths: &three, Assists: &fourteen}}
	srv := buildPatches(t).WithEndReader(reader).WithNoter(&fakeNoter{})
	h := srv.Handler()
	id := startTestMatch(t, h)

	img, err := os.ReadFile("../photo/testdata/endscreen.jpg")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("image", "end.jpg")
	_, _ = part.Write(img)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/match/"+id+"/end/read", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read: %d %s", rec.Code, rec.Body)
	}
	if want := `{"result":"win","durationS":2292,"kills":9,"deaths":3,"assists":14}`; strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("read %s, want %s", rec.Body, want)
	}
	if reader.hero != "Hero 1" {
		t.Fatalf("reader looked for %q, want the session's hero", reader.hero)
	}
	if r := storedResult(t, h, id); r != nil {
		t.Fatalf("saved %+v before the player confirmed", r)
	}

	// The player fixes the deaths and confirms.
	rec = send(h, http.MethodPost, "/api/match/"+id+"/result",
		`{"result":"win","source":"photo","durationS":2292,"kills":9,"deaths":4,"assists":14}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm: %d %s", rec.Code, rec.Body)
	}
	r := storedResult(t, h, id)
	if r == nil || r.Outcome != "win" || r.Source != "photo" || r.DurationS == nil || *r.DurationS != 2292 ||
		r.Kills == nil || *r.Kills != 9 || *r.Deaths != 4 || *r.Assists != 14 {
		t.Fatalf("stored %+v, want the confirmed read", r)
	}
}

func TestTheHistoryListsThePlayersMatchesNewestFirst(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	logs := &bytes.Buffer{}
	a, err := auth.New(pool, "http://localhost:5173", slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.WithLogMailer(); err != nil {
		t.Fatal(err)
	}
	h := srv.WithAuth(a).WithNoter(&fakeNoter{}).Handler()
	signIn := func(email string) *http.Cookie {
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
	me, other := signIn("player@example.com"), signIn("other@example.com")
	play := func(hero int, result string, who ...*http.Cookie) string {
		rec := send(h, http.MethodPost, "/api/match", `{"hero":`+string(rune('0'+hero))+`,"role":1,"enemies":[]}`, who...)
		var s match.Session
		if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
			t.Fatalf("start: %s", rec.Body)
		}
		if rec := send(h, http.MethodPost, "/api/match/"+s.ID+"/result", `{"result":"`+result+`","source":"tap"}`, who...); rec.Code != http.StatusOK {
			t.Fatalf("result: %d %s", rec.Code, rec.Body)
		}
		return s.ID
	}
	first := play(1, "loss", me)
	play(2, "win", other)
	play(3, "win") // a visitor's
	second := play(2, "win", me)
	// A match started before signing in is the player's once they end it signed in.
	rec := send(h, http.MethodPost, "/api/match", `{"hero":3,"role":1,"enemies":[]}`)
	var early match.Session
	_ = json.Unmarshal(rec.Body.Bytes(), &early)
	if rec := send(h, http.MethodPost, "/api/match/"+early.ID+"/result", `{"result":"loss","source":"tap"}`, me); rec.Code != http.StatusOK {
		t.Fatalf("claim: %d %s", rec.Code, rec.Body)
	}
	// The player started a third match and has not ended it.
	send(h, http.MethodPost, "/api/match", `{"hero":3,"role":1,"enemies":[]}`, me)

	if rec := send(h, http.MethodGet, "/api/me/matches", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("visitor: %d, want 401", rec.Code)
	}
	rec = send(h, http.MethodGet, "/api/me/matches", "", me)
	if rec.Code != http.StatusOK {
		t.Fatalf("history: %d %s", rec.Code, rec.Body)
	}
	var got []match.Past
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != early.ID || got[1].ID != second || got[1].HeroID != 2 || got[1].Result != "win" ||
		got[2].ID != first || got[2].HeroID != 1 || got[2].Result != "loss" || !got[1].EndedAt.After(got[2].EndedAt) {
		t.Fatalf("history %+v, want the player's three ended matches, newest first", got)
	}

	// Another player cannot end a match that is not theirs.
	if rec := send(h, http.MethodPost, "/api/match/"+first+"/result", `{"result":"win","source":"tap"}`, other); rec.Code != http.StatusNotFound {
		t.Fatalf("someone else's match: %d, want 404", rec.Code)
	}
}
