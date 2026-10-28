package decision

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGLog writes calls to decision_log. A failed write is logged, never
// returned: the log must not fail the decision it records.
type PGLog struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewPGLog builds a store on pool.
func NewPGLog(pool *pgxpool.Pool, log *slog.Logger) *PGLog {
	return &PGLog{pool: pool, log: log}
}

// Record inserts the entry.
func (s *PGLog) Record(ctx context.Context, e Entry) {
	nullInt := func(n int) *int { // 0 tokens means the usage was not reported
		if n == 0 {
			return nil
		}
		return &n
	}
	nullText := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	var answer *string
	if len(e.Answer) > 0 {
		a := string(e.Answer)
		answer = &a
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO decision_log (purpose, model, answered_model, outcome, error, latency_ms, attempts, hedged,
			input_tokens, output_tokens, cost_usd, request, answer)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb)`,
		e.Purpose, e.Model, nullText(e.AnsweredModel), e.Outcome, nullText(e.Error), e.Latency.Milliseconds(), e.Attempts,
		e.Hedged, nullInt(e.InputTokens), nullInt(e.OutputTokens), e.Cost, string(e.Request), answer)
	if err != nil {
		s.log.Error("decision log", "purpose", e.Purpose, "err", err)
	}
}
