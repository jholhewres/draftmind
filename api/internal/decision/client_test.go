package decision

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "sk-or-test-key-not-real"

// memLog keeps the entries a client records.
type memLog struct {
	mu      sync.Mutex
	entries []Entry
}

func (m *memLog) Record(_ context.Context, e Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e)
}

func (m *memLog) all() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Entry(nil), m.entries...)
}

func newClient(t *testing.T, h http.HandlerFunc) (*Client, *memLog) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	log := &memLog{}
	c := New(srv.URL, testKey, "typesafe/jev-1.13", "typesafe", log)
	c.HTTP = srv.Client()
	return c, log
}

// pickRequest asks one choice among two heroes.
func pickRequest() Request {
	return Request{
		Purpose: "test.pick",
		State:   map[string]any{"enemies": []string{"Axe"}},
		Questions: map[string]Question{
			"pick": Choice("Which hero?", map[string]any{"Juggernaut": "counters Axe", "Anti-Mage": "farms fast"}),
		},
	}
}

const okBody = `{
	"model": "typesafe/jev-1.13.0",
	"answers": {"pick": {"type": "choice", "choice": "Juggernaut", "confidence": 0.6,
		"probabilities": {"Juggernaut": 0.8, "Anti-Mage": 0.2}}},
	"usage": {"input_tokens": 300, "output_tokens": 20, "cost": 0.00042}
}`

// hang holds a request until the client gives up on it. The body is read
// first: the server only notices a closed connection after that.
func hang(r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	select {
	case <-r.Context().Done():
	case <-time.After(5 * time.Second):
	}
}

func reply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func TestDecisionClientSendsTheQuestionsWithThePinnedProvider(t *testing.T) {
	var got map[string]any
	var auth, path string
	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		reply(http.StatusOK, okBody)(w, r)
	})
	res, err := c.Ask(context.Background(), pickRequest())
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/systemone" || auth != "Bearer "+testKey {
		t.Fatalf("path %q, auth set %t", path, auth == "Bearer "+testKey)
	}
	if got["model"] != "typesafe/jev-1.13" || got["state"] == nil {
		t.Fatalf("request %v", got)
	}
	q := got["questions"].(map[string]any)["pick"].(map[string]any)
	if q["type"] != "choice" || q["instructions"] != "Which hero?" || len(q["criteria"].(map[string]any)) != 2 {
		t.Fatalf("question %v", q)
	}
	prov, _ := json.Marshal(got["provider"])
	want := `{"allow_fallbacks":false,"data_collection":"deny","only":["typesafe"],"zdr":true}`
	if string(prov) != want {
		t.Fatalf("provider %s, want %s", prov, want)
	}
	a := res.Answers["pick"]
	if res.Model != "typesafe/jev-1.13.0" || a.Choice != "Juggernaut" || a.Confidence != 0.6 || a.Probabilities["Anti-Mage"] != 0.2 {
		t.Fatalf("result %+v", res)
	}
}

func TestDecisionClientFailsOnAMissingOrMistypedAnswer(t *testing.T) {
	cases := map[string]string{
		"no answer for the question": `{"model": "m", "answers": {}, "usage": {"input_tokens": 1, "output_tokens": 1}}`,
		"no answers at all":          `{"model": "m", "usage": {"input_tokens": 1, "output_tokens": 1}}`,
		"answer of another type": `{"model": "m", "answers": {"pick": {"type": "noul", "noul": 0.9}},
			"usage": {"input_tokens": 1, "output_tokens": 1}}`,
		"choice that is a number": `{"model": "m", "answers": {"pick": {"type": "choice", "choice": 3, "confidence": 0.5,
			"probabilities": {"Juggernaut": 0.5, "Anti-Mage": 0.5}}}}`,
		"choice not among the options": `{"model": "m", "answers": {"pick": {"type": "choice", "choice": "Pudge", "confidence": 0.5,
			"probabilities": {"Juggernaut": 0.5, "Anti-Mage": 0.5}}}}`,
		"no confidence": `{"model": "m", "answers": {"pick": {"type": "choice", "choice": "Juggernaut",
			"probabilities": {"Juggernaut": 0.5, "Anti-Mage": 0.5}}}}`,
		"an option without a probability": `{"model": "m", "answers": {"pick": {"type": "choice", "choice": "Juggernaut",
			"confidence": 0.5, "probabilities": {"Juggernaut": 1}}}}`,
		"a probability above 1": `{"model": "m", "answers": {"pick": {"type": "choice", "choice": "Juggernaut",
			"confidence": 0.5, "probabilities": {"Juggernaut": 1.5, "Anti-Mage": 0}}}}`,
		"not JSON": `<html>bad gateway</html>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			c, log := newClient(t, reply(http.StatusOK, body))
			_, err := c.Ask(context.Background(), pickRequest())
			if err == nil {
				t.Fatal("call succeeded, want it to fail")
			}
			if e := log.all(); len(e) != 1 || e[0].Outcome != OutcomeError {
				t.Fatalf("log %+v, want one error entry", e)
			}
		})
	}
}

func TestDecisionClientParsesNoulAndScoreAnswersStrictly(t *testing.T) {
	req := Request{Purpose: "test", State: "s", Questions: map[string]Question{
		"yes":   Noul("Is it late?"),
		"level": Score("How urgent?", "low", "high"),
	}}
	good := `{"model": "m", "answers": {
		"yes": {"type": "noul", "noul": 0.7},
		"level": {"type": "score", "score": 0.9, "confidence": 0.4, "legend": {"0": "low", "1": "high"},
			"probabilities": {"0": 0.1, "1": 0.9}}}, "usage": {"input_tokens": 1, "output_tokens": 1}}`
	c, _ := newClient(t, reply(http.StatusOK, good))
	res, err := c.Ask(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Answers["yes"].Noul != 0.7 || res.Answers["level"].Score != 0.9 || res.Answers["level"].Probabilities["1"] != 0.9 {
		t.Fatalf("answers %+v", res.Answers)
	}
	bad := strings.Replace(good, `"noul": 0.7`, `"noul": "yes"`, 1)
	c, _ = newClient(t, reply(http.StatusOK, bad))
	if _, err := c.Ask(context.Background(), req); err == nil {
		t.Fatal("a string noul passed, want a failure")
	}
	bad = strings.Replace(good, `"score": 0.9,`, ``, 1)
	c, _ = newClient(t, reply(http.StatusOK, bad))
	if _, err := c.Ask(context.Background(), req); err == nil {
		t.Fatal("a score answer without its score passed, want a failure")
	}
}

func TestDecisionClientRetriesA5xxOrA429InsideTheDeadline(t *testing.T) {
	var calls atomic.Int32
	c, log := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			reply(http.StatusServiceUnavailable, `{"error": "busy"}`)(w, r)
		case 2:
			w.Header().Set("Retry-After", "0")
			reply(http.StatusTooManyRequests, `{"error": "slow down"}`)(w, r)
		default:
			reply(http.StatusOK, okBody)(w, r)
		}
	})
	c.HedgeAfter = -1
	c.Backoff = time.Millisecond
	if _, err := c.Ask(context.Background(), pickRequest()); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 3 {
		t.Fatalf("%d calls, want 3", n)
	}
	if e := log.all(); len(e) != 1 || e[0].Attempts != 3 || e[0].Outcome != OutcomeOK {
		t.Fatalf("log %+v, want one ok entry after 3 attempts", e)
	}
}

func TestDecisionClientDoesNotRetryABadRequest(t *testing.T) {
	var calls atomic.Int32
	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		reply(http.StatusUnprocessableEntity, `{"detail": [{"loc": ["body", "state"], "msg": "Field required"}]}`)(w, r)
	})
	c.HedgeAfter = -1
	_, err := c.Ask(context.Background(), pickRequest())
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "Field required") {
		t.Fatalf("error %v, want the 422 with its detail", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("%d calls, want 1", calls.Load())
	}
}

func TestDecisionClientTimesOutAtTheDeadline(t *testing.T) {
	c, log := newClient(t, func(_ http.ResponseWriter, r *http.Request) {
		hang(r)
	})
	c.Deadline = 50 * time.Millisecond
	c.HedgeAfter = 20 * time.Millisecond
	start := time.Now()
	_, err := c.Ask(context.Background(), pickRequest())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error %v, want the deadline", err)
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Fatalf("took %v, want about the 50 ms deadline", took)
	}
	if e := log.all(); len(e) != 1 || e[0].Outcome != OutcomeTimeout || !e[0].Hedged {
		t.Fatalf("log %+v, want one hedged timeout entry", e)
	}
}

func TestDecisionClientHedgesASlowRequestAndTheFirstAnswerWins(t *testing.T) {
	var calls atomic.Int32
	c, log := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 { // the first request hangs until it is abandoned
			hang(r)
			return
		}
		reply(http.StatusOK, okBody)(w, r)
	})
	c.HedgeAfter = 30 * time.Millisecond
	start := time.Now()
	res, err := c.Ask(context.Background(), pickRequest())
	if err != nil || res.Answers["pick"].Choice != "Juggernaut" {
		t.Fatalf("result %+v, err %v", res, err)
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("took %v, want the hedge to answer well before the deadline", took)
	}
	if e := log.all(); len(e) != 1 || !e[0].Hedged || e[0].Outcome != OutcomeOK {
		t.Fatalf("log %+v, want one hedged ok entry", e)
	}
}

func TestDecisionClientLogsEveryCallWithModelLatencyAndCost(t *testing.T) {
	c, log := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		reply(http.StatusOK, okBody)(w, r)
	})
	if _, err := c.Ask(context.Background(), pickRequest()); err != nil {
		t.Fatal(err)
	}
	e := log.all()
	if len(e) != 1 {
		t.Fatalf("%d entries, want 1", len(e))
	}
	got := e[0]
	if got.Purpose != "test.pick" || got.Model != "typesafe/jev-1.13" || got.AnsweredModel != "typesafe/jev-1.13.0" ||
		got.Latency < 5*time.Millisecond || got.Cost == nil || *got.Cost != 0.00042 ||
		got.InputTokens != 300 || got.OutputTokens != 20 || got.Outcome != OutcomeOK || got.Attempts != 1 {
		t.Fatalf("entry %+v", got)
	}
	if !strings.Contains(string(got.Request), `"Which hero?"`) || !strings.Contains(string(got.Answer), `"Juggernaut"`) {
		t.Fatalf("request %s, answer %s: want the question and the answer JSON", got.Request, got.Answer)
	}
	for _, s := range []string{string(got.Request), string(got.Answer), got.Error} {
		if strings.Contains(s, testKey) {
			t.Fatal("the API key leaked into the log")
		}
	}
}

func TestDecisionClientLogsAMissingCostAsNull(t *testing.T) {
	c, log := newClient(t, reply(http.StatusOK, strings.Replace(okBody, `, "cost": 0.00042`, ``, 1)))
	if _, err := c.Ask(context.Background(), pickRequest()); err != nil {
		t.Fatal(err)
	}
	if e := log.all(); len(e) != 1 || e[0].Cost != nil || e[0].InputTokens != 300 {
		t.Fatalf("log %+v, want no cost and the tokens", e)
	}
}

func TestDecisionClientNeverPutsTheKeyInAnError(t *testing.T) {
	c, log := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		// A server that echoes the auth header back must not leak it through us.
		reply(http.StatusBadRequest, `{"detail": "bad key `+r.Header.Get("Authorization")+`"}`)(w, r)
	})
	_, err := c.Ask(context.Background(), pickRequest())
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Fatalf("error %v, want a failure without the key", err)
	}
	if e := log.all(); len(e) != 1 || strings.Contains(e[0].Error, testKey) || strings.Contains(string(e[0].Answer), testKey) {
		t.Fatalf("the key leaked into the log entry")
	}
}

// openrouter-choice.json is a live OpenRouter answer (2026-10-28) to the
// draft question for a Pos 2 pick, with its generation id replaced.
func TestDecisionClientParsesTheAnswerShapeOpenRouterSends(t *testing.T) {
	raw, err := os.ReadFile("testdata/openrouter-choice.json")
	if err != nil {
		t.Fatal(err)
	}
	c, log := newClient(t, reply(http.StatusOK, string(raw)))
	options := map[string]any{}
	for _, n := range []string{"Ember Spirit", "Leshrac", "Nyx Assassin", "Mirana", "Broodmother", "Earth Spirit", "Bounty Hunter", "Brewmaster"} {
		options[n] = nil
	}
	res, err := c.Ask(context.Background(), Request{Purpose: "test", State: "draft", Questions: map[string]Question{"pick": Choice("Which?", options)}})
	if err != nil {
		t.Fatal(err)
	}
	if a := res.Answers["pick"]; a.Choice != "Ember Spirit" || a.Confidence != 0.52 || a.Probabilities["Leshrac"] != 0.39 {
		t.Fatalf("answer %+v", a)
	}
	if e := log.all()[0]; e.AnsweredModel != "typesafe/jev-1.13-20260917" || e.Cost == nil || *e.Cost != 0.00009072 || e.InputTokens != 2160 {
		t.Fatalf("entry %+v", e)
	}
}
