package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/draft"
	"github.com/jholhewres/draftmind/api/internal/match"
)

// liveTickOf posts one frame to the session's live coach.
func liveTickOf(t *testing.T, h http.Handler, id string) (int, liveResponse) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("image", "frame.jpg")
	_, _ = part.Write(jpegOf(t, 320, 180))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/match/"+id+"/live", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out liveResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestPastTheSpendCapModelCallsStopAndTheCodeAnswers(t *testing.T) {
	srv, pool := buildPatchesPool(t)
	calls := 0
	jev := jevOn(t, pool, func(w http.ResponseWriter, r *http.Request) {
		calls++
		// Silent, with a probability for each option the question offers.
		var req struct {
			Questions map[string]struct {
				Criteria map[string]any `json:"criteria"`
			} `json:"questions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		probs := map[string]float64{}
		for k := range req.Questions["event"].Criteria {
			probs[k] = 0
		}
		probs["silent"] = 1
		answer, _ := json.Marshal(map[string]any{"model": "typesafe/jev-1.13.0",
			"answers": map[string]any{"event": map[string]any{"type": "choice", "choice": "silent", "confidence": 0.9, "probabilities": probs}},
			"usage":   map[string]any{"input_tokens": 500, "output_tokens": 10, "cost": 0.1}})
		_, _ = w.Write(answer)
	})
	now := srv.now()
	srv.now = func() time.Time { return now }
	h := srv.WithDecider(jev).WithLive(match.NewLive(jev)).WithBudget(0.25, 0.30).Handler()
	id := startTestMatch(t, h)

	// Ticks of $0.10 each until the session's $0.25 cap is passed.
	for i := range 3 {
		now = now.Add(match.LiveMinInterval)
		code, out := liveTickOf(t, h, id)
		if code != http.StatusOK || out.Budget == nil || out.Budget.Cap != 0.25 {
			t.Fatalf("tick %d: %d %+v", i, code, out)
		}
	}
	// Jev was asked three times; the fourth tick asks nobody.
	before := calls
	now = now.Add(match.LiveMinInterval)
	code, out := liveTickOf(t, h, id)
	if code != http.StatusOK || out.Reason != match.LiveReasonBudget || out.Budget.Spent < 0.25 || calls != before {
		t.Fatalf("over the cap: %d %+v after %d calls, want a budget reason and no call", code, out, calls)
	}

	// The session's draft suggestions still come, from the code ranking.
	rec := send(h, http.MethodPost, "/api/draft/suggest", `{"enemies": [2], "role": 1}`)
	var d draft.Decided
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	if rec.Code != http.StatusOK || calls != before {
		t.Fatalf("suggest after the cap: %d %s, %d calls", rec.Code, rec.Body, calls)
	}
	if !strings.Contains(rec.Body.String(), `"reason":"budget"`) || d.Source != draft.SourceRanking {
		t.Fatalf("suggest after the cap: %s, want the code ranking labelled budget", rec.Body)
	}
}
