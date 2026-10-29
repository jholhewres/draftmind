package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/catalog"
	"github.com/jholhewres/draftmind/api/internal/decision"
	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/writer"
)

type sseEvent struct {
	name string
	data json.RawMessage
}

// events reads a text/event-stream body one event at a time.
type events struct {
	t  *testing.T
	br *bufio.Reader
}

// next reads the next event, failing the test when none comes within wait.
func (e events) next(wait time.Duration) sseEvent {
	e.t.Helper()
	got := make(chan sseEvent, 1)
	errs := make(chan error, 1)
	go func() {
		var ev sseEvent
		for {
			line, err := e.br.ReadString('\n')
			if err != nil {
				errs <- err
				return
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case line == "":
				if ev.name != "" {
					got <- ev
					return
				}
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.data = json.RawMessage(strings.TrimPrefix(line, "data: "))
			}
		}
	}()
	select {
	case ev := <-got:
		return ev
	case err := <-errs:
		e.t.Fatalf("stream ended: %v", err)
	case <-time.After(wait):
		e.t.Fatalf("no event within %v", wait)
	}
	return sseEvent{}
}

// rest reads every event up to and including done.
func (e events) rest() []sseEvent {
	e.t.Helper()
	var out []sseEvent
	for {
		ev := e.next(5 * time.Second)
		out = append(out, ev)
		if ev.name == "done" {
			return out
		}
	}
}

// opened is a stream's status and headers; its body is read through events.
type opened struct {
	StatusCode int
	Header     http.Header
}

func openStream(t *testing.T, srv *Server, query, contentType string, body []byte) (opened, events) {
	t.Helper()
	api := httptest.NewServer(srv.Handler())
	t.Cleanup(api.Close)
	resp, err := http.Post(api.URL+"/api/draft/stream"+query, contentType, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return opened{StatusCode: resp.StatusCode, Header: resp.Header}, events{t: t, br: bufio.NewReader(resp.Body)}
}

// heldJev answers the pick once released, preferring Anti-Mage at conf.
type heldJev struct {
	release chan struct{}
	conf    float64
}

func (j *heldJev) Ask(ctx context.Context, _ decision.Request) (decision.Result, error) {
	select {
	case <-j.release:
	case <-ctx.Done():
		return decision.Result{}, ctx.Err()
	}
	return decision.Result{Answers: map[string]decision.Answer{"pick": {
		Type: "choice", Choice: "Anti-Mage", Confidence: j.conf,
		Probabilities: map[string]float64{"Anti-Mage": 0.7, "Juggernaut": 0.3},
	}}}, nil
}

// heldTipper writes "tip <hero>" once released and keeps the briefs.
type heldTipper struct {
	release chan struct{}
	mu      sync.Mutex
	briefs  []writer.Brief
}

func (h *heldTipper) Tip(ctx context.Context, b writer.Brief) writer.Tip {
	h.mu.Lock()
	h.briefs = append(h.briefs, b)
	h.mu.Unlock()
	select {
	case <-h.release:
	case <-ctx.Done():
		return writer.Template(b.HeroID, writer.ReasonTimeout)
	}
	return writer.Tip{HeroID: b.HeroID, Text: "tip " + b.Hero, Source: writer.SourceWriter}
}

func heroIDs(ss []draft.Suggestion) []int {
	out := make([]int, len(ss))
	for i, s := range ss {
		out[i] = s.HeroID
	}
	return out
}

func decode[T any](t *testing.T, ev sseEvent) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(ev.data, &v); err != nil {
		t.Fatalf("%s event %s: %v", ev.name, ev.data, err)
	}
	return v
}

func TestStreamSendsTheRankingBeforeAnyModelCallReturns(t *testing.T) {
	pool := seedDraft(t)
	// An ability and a talent of Axe's, and his pt-BR name.
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO ability (patch_id, hero_id, id, slot, kind, name) SELECT id, 2, 10, 0, 'ability', 'axe_berserkers_call' FROM patch WHERE is_current;
		INSERT INTO ability (patch_id, hero_id, id, slot, kind, name) SELECT id, 2, 11, 9, 'talent', 'special_bonus_armor' FROM patch WHERE is_current;
		INSERT INTO ability_text (patch_id, hero_id, ability_id, lang, name, description, lore)
			SELECT id, 2, 10, 'en', 'Berserker''s Call', '', '' FROM patch WHERE is_current UNION ALL
			SELECT id, 2, 11, 'en', '+6 Armor', '', '' FROM patch WHERE is_current;
		INSERT INTO hero_text (patch_id, hero_id, lang, name, hype, bio) SELECT id, 2, 'pt-BR', 'Machado', '', '' FROM patch WHERE is_current`); err != nil {
		t.Fatal(err)
	}
	jev := &heldJev{release: make(chan struct{}), conf: 0.6}
	tips := &heldTipper{release: make(chan struct{})}
	srv := New(fakeDB{}, catalog.NewStore(pool), quietLog()).WithDecider(jev).WithWriter(tips)
	resp, evs := openStream(t, srv, "?lang=pt-BR", "application/json", []byte(`{"enemies": [2], "bans": [5, 7], "role": 1}`))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d, content type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// Jev and the writer are both held: the ranking must already be out.
	first := evs.next(2 * time.Second)
	if first.name != "ranking" {
		t.Fatalf("first event %q, want ranking", first.name)
	}
	if r := decode[rankingEvent](t, first); r.Patch != "7.41" || !slices.Equal(heroIDs(r.Suggestions), []int{8, 1}) {
		t.Fatalf("ranking %+v, want Juggernaut then Anti-Mage on 7.41", r)
	}
	close(jev.release)
	decided := evs.next(2 * time.Second)
	if d := decode[draft.Decided](t, decided); decided.name != "decision" || d.Source != draft.SourceJev ||
		!slices.Equal(heroIDs(d.Suggestions), []int{1, 8}) {
		t.Fatalf("event %q %s, want Jev's decision with Anti-Mage first", decided.name, decided.data)
	}
	close(tips.release)
	var names []string
	tipped := map[int]writer.Tip{}
	for _, ev := range evs.rest() {
		names = append(names, ev.name)
		if ev.name == "tip" {
			tip := decode[writer.Tip](t, ev)
			tipped[tip.HeroID] = tip
		}
	}
	if !slices.Equal(names, []string{"tip", "tip", "done"}) || tipped[8].Text != "tip Juggernaut" || tipped[1].Text != "tip Anti-Mage" {
		t.Fatalf("events %v, tips %+v", names, tipped)
	}
	tips.mu.Lock()
	defer tips.mu.Unlock()
	for _, b := range tips.briefs {
		if b.HeroID == 8 {
			if b.Lang != "pt-BR" || b.Position != 1 || len(b.Enemies) != 1 || b.Enemies[0] != (writer.Enemy{Name: "Axe", Delta: 5, Games: 100}) ||
				!slices.Contains(b.Allowed, "Axe") || !slices.Contains(b.Allowed, "Juggernaut") || slices.Contains(b.Allowed, "Earthshaker") ||
				!slices.Contains(b.Allowed, "Machado") || !slices.Contains(b.Known, "Earthshaker") ||
				!slices.Contains(b.Known, "Berserker's Call") || slices.Contains(b.Known, "+6 Armor") || !slices.Contains(b.Known, "Mirana") {
				t.Fatalf("brief %+v", b)
			}
		}
	}
}

// writerOn is a real writer whose model answers each hero with text[hero].
func writerOn(t *testing.T, text map[string]string) *writer.Writer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var facts struct{ Hero string }
		_ = json.Unmarshal([]byte(body.Messages[len(body.Messages)-1].Content), &facts)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": text[facts.Hero]}}}})
	}))
	t.Cleanup(srv.Close)
	w := writer.New(srv.URL, "test-key", "openai/gpt-6-luna", nil)
	w.HTTP = srv.Client()
	return w
}

func TestStreamTipNamingAHeroOrItemOutsideTheResultComesAsATemplate(t *testing.T) {
	pool := seedDraft(t)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO item (patch_id, id, key, dname, cost) SELECT id, 1, 'blink', 'Blink Dagger', 2250 FROM patch WHERE is_current`); err != nil {
		t.Fatal(err)
	}
	w := writerOn(t, map[string]string{
		"Juggernaut":  "Juggernaut wins 5 points more against Axe.",
		"Anti-Mage":   "Anti-Mage struggles into Axe, but Earthshaker would not.",
		"Earthshaker": "Earthshaker beats Axe, then rush a blink dagger.", // an item in lower case
	})
	srv := New(fakeDB{}, catalog.NewStore(pool), quietLog()).WithWriter(w)
	_, evs := openStream(t, srv, "", "application/json", []byte(`{"enemies": [2], "bans": [5], "role": 1}`))
	tipped := map[int]writer.Tip{}
	for _, ev := range evs.rest() {
		if ev.name == "tip" {
			tip := decode[writer.Tip](t, ev)
			tipped[tip.HeroID] = tip
		}
	}
	// Top 3 without Jev: Juggernaut, Anti-Mage, then Earthshaker.
	want := map[int]writer.Tip{
		8: {HeroID: 8, Text: "Juggernaut wins 5 points more against Axe.", Source: writer.SourceWriter},
		1: {HeroID: 1, Source: writer.SourceTemplate, Reason: writer.ReasonRejected},
		7: {HeroID: 7, Source: writer.SourceTemplate, Reason: writer.ReasonRejected},
	}
	if len(tipped) != 3 || tipped[8] != want[8] || tipped[1] != want[1] || tipped[7] != want[7] {
		t.Fatalf("tips %+v, want %+v", tipped, want)
	}
}

func TestStreamLabelsALowConfidenceDecisionAndKeepsTheCodeOrder(t *testing.T) {
	pool := seedDraft(t)
	jev := &heldJev{release: make(chan struct{}), conf: 0.3}
	close(jev.release)
	srv := New(fakeDB{}, catalog.NewStore(pool), quietLog()).WithDecider(jev)
	_, evs := openStream(t, srv, "", "application/json", []byte(`{"enemies": [2], "bans": [5, 7], "role": 1}`))
	var d *draft.Decided
	tips := 0
	for _, ev := range evs.rest() {
		switch ev.name {
		case "decision":
			v := decode[draft.Decided](t, ev)
			d = &v
		case "tip":
			// No writer: each top hero still gets the template.
			if tip := decode[writer.Tip](t, ev); tip.Source != writer.SourceTemplate || tip.Reason != writer.ReasonUnavailable {
				t.Fatalf("tip %+v, want an unavailable template", tip)
			}
			tips++
		}
	}
	if d == nil || d.Source != draft.SourceRanking || d.Reason != draft.ReasonLowConfidence || d.Confidence == nil || *d.Confidence != 0.3 ||
		!slices.Equal(heroIDs(d.Suggestions), []int{8, 1}) {
		t.Fatalf("decision %+v, want the code order labelled low_confidence", d)
	}
	if tips != 2 {
		t.Fatalf("%d tips, want 2", tips)
	}
}

func TestStreamFromAPhotoSendsTheReadThenRanksTheDraftItShows(t *testing.T) {
	pool := seedDraft(t)
	srv := New(fakeDB{}, catalog.NewStore(pool), quietLog()).WithPhotoReader(&stubReader{})
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("image", "pick.jpg")
	_, _ = part.Write(jpegOf(t, 600, 400))
	_ = mw.Close()
	// The stub reads Anti-Mage in the first radiant slot; the player is dire.
	resp, evs := openStream(t, srv, "?role=1&team=dire", mw.FormDataContentType(), body.Bytes())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	first := evs.next(5 * time.Second)
	type readEvent struct {
		Patch   string `json:"patch"`
		Reader  string `json:"reader"`
		Radiant []struct {
			HeroID int    `json:"heroId"`
			Status string `json:"status"`
		} `json:"radiant"`
		Draft suggestRequest `json:"draft"`
	}
	r := decode[readEvent](t, first)
	if first.name != "read" || r.Patch != "7.41" || r.Reader != "vision" || r.Radiant[0].HeroID != 1 ||
		len(r.Draft.Allies) != 0 || !slices.Equal(r.Draft.Enemies, []int{1}) || r.Draft.Role != 1 {
		t.Fatalf("first event %q %s", first.name, first.data)
	}
	ranking := evs.next(5 * time.Second)
	if ranking.name != "ranking" {
		t.Fatalf("second event %q, want ranking", ranking.name)
	}
	if got := heroIDs(decode[rankingEvent](t, ranking).Suggestions); slices.Contains(got, 1) || got[0] != 8 {
		t.Fatalf("ranking %v, want Anti-Mage taken and Juggernaut first", got)
	}
}

func TestStreamRefusesABadDraftOrLanguageBeforeStreaming(t *testing.T) {
	pool := seedDraft(t)
	srv := New(fakeDB{}, catalog.NewStore(pool), quietLog())
	for name, tc := range map[string]struct{ query, body string }{
		"role":      {"", `{"role": 9}`},
		"off patch": {"", `{"enemies": [9], "role": 1}`},
		"language":  {"?lang=" + strings.Repeat("x", 40), `{"role": 1}`},
	} {
		resp, _ := openStream(t, srv, tc.query, "application/json", []byte(tc.body))
		if resp.StatusCode != http.StatusBadRequest || strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
			t.Errorf("%s: status %d, content type %q", name, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
}
