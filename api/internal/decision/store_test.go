package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

func TestDecisionLogKeepsEachCallInTheDatabase(t *testing.T) {
	pool := dbtest.New(t)
	var logs bytes.Buffer
	store := NewPGLog(pool, slog.New(slog.NewTextHandler(&logs, nil)))
	cost := 0.00042
	store.Record(context.Background(), Entry{
		Purpose: "draft.suggest", Model: "typesafe/jev-1.13", AnsweredModel: "typesafe/jev-1.13.0",
		Outcome: OutcomeOK, Latency: 412 * time.Millisecond, Attempts: 2, Hedged: true,
		InputTokens: 300, OutputTokens: 20, Cost: &cost,
		Request: json.RawMessage(`{"questions": {"pick": {"type": "choice"}}}`),
		Answer:  json.RawMessage(`{"answers": {"pick": {"choice": "Juggernaut"}}}`),
	})
	store.Record(context.Background(), Entry{
		Purpose: "draft.suggest", Model: "typesafe/jev-1.13", Outcome: OutcomeTimeout, Error: "no answer within 1.5s",
		Latency: 1500 * time.Millisecond, Attempts: 1, Request: json.RawMessage(`{}`),
	})
	if logs.Len() > 0 {
		t.Fatalf("the store logged a failure: %s", logs.String())
	}

	rows, err := pool.Query(context.Background(), `
		SELECT purpose, model, coalesce(answered_model, ''), outcome, coalesce(error, ''), latency_ms, attempts, hedged,
			input_tokens, output_tokens, cost_usd::float8, request->'questions'->'pick'->>'type', answer->'answers'->'pick'->>'choice'
		FROM decision_log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		purpose, model, answered, outcome, err string
		latency, attempts                      int
		hedged                                 bool
		in, out                                *int
		cost                                   *float64
		qtype, choice                          *string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.purpose, &r.model, &r.answered, &r.outcome, &r.err, &r.latency, &r.attempts, &r.hedged,
			&r.in, &r.out, &r.cost, &r.qtype, &r.choice); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("%d rows, want 2", len(got))
	}
	ok, late := got[0], got[1]
	if ok.purpose != "draft.suggest" || ok.model != "typesafe/jev-1.13" || ok.answered != "typesafe/jev-1.13.0" ||
		ok.outcome != "ok" || ok.latency != 412 || ok.attempts != 2 || !ok.hedged || *ok.in != 300 || *ok.out != 20 ||
		ok.cost == nil || *ok.cost != 0.00042 || *ok.qtype != "choice" || *ok.choice != "Juggernaut" {
		t.Fatalf("ok row %+v", ok)
	}
	if late.outcome != "timeout" || late.err != "no answer within 1.5s" || late.cost != nil || late.in != nil || late.choice != nil {
		t.Fatalf("timeout row %+v", late)
	}
}
