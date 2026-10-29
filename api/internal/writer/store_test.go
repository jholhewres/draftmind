package writer

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jholhewres/draftmind/api/internal/dbtest"
)

func TestModelCallKeepsEachWriterCallInTheDatabase(t *testing.T) {
	pool := dbtest.New(t)
	var logs bytes.Buffer
	store := NewPGLog(pool, slog.New(slog.NewTextHandler(&logs, nil)))
	cost := 0.00003
	store.Record(context.Background(), Call{
		Purpose: "draft.tip", Model: "openai/gpt-6-luna", AnsweredModel: "openai/gpt-6-luna-20261001", Outcome: OutcomeRejected,
		Error: `names "Earthshaker", which is not in the result`, Latency: 912 * time.Millisecond,
		InputTokens: 210, OutputTokens: 30, Cost: &cost, Output: "Earthshaker cannot stop him.",
	})
	store.Record(context.Background(), Call{Purpose: "draft.tip", Model: "openai/gpt-6-luna", Outcome: OutcomeTimeout, Latency: 4 * time.Second})
	if logs.Len() > 0 {
		t.Fatalf("the store logged a failure: %s", logs.String())
	}
	rows, err := pool.Query(context.Background(), `
		SELECT purpose, model, coalesce(answered_model, ''), outcome, coalesce(error, ''), latency_ms,
			input_tokens, output_tokens, cost_usd::float8, output
		FROM model_call ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		purpose, model, answered, outcome, err string
		latency                                int
		in, out                                *int
		cost                                   *float64
		output                                 *string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.purpose, &r.model, &r.answered, &r.outcome, &r.err, &r.latency, &r.in, &r.out, &r.cost, &r.output); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("%d rows, want 2", len(got))
	}
	rej, late := got[0], got[1]
	if rej.purpose != "draft.tip" || rej.answered != "openai/gpt-6-luna-20261001" || rej.outcome != "rejected" || rej.latency != 912 ||
		*rej.in != 210 || *rej.out != 30 || rej.cost == nil || *rej.cost != 0.00003 || rej.output == nil || *rej.output != "Earthshaker cannot stop him." {
		t.Fatalf("rejected row %+v", rej)
	}
	if late.outcome != "timeout" || late.in != nil || late.cost != nil || late.output != nil {
		t.Fatalf("timeout row %+v", late)
	}
}
