package writer

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGLog writes calls to model_call. A failed write is logged, never
// returned: the log must not fail the tip it records.
type PGLog struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewPGLog builds a store on pool.
func NewPGLog(pool *pgxpool.Pool, log *slog.Logger) *PGLog {
	return &PGLog{pool: pool, log: log}
}

// Record inserts the call.
func (s *PGLog) Record(ctx context.Context, c Call) {
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
	_, err := s.pool.Exec(ctx, `
		INSERT INTO model_call (purpose, model, answered_model, outcome, error, latency_ms, input_tokens, output_tokens, cost_usd, output)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		c.Purpose, c.Model, nullText(c.AnsweredModel), c.Outcome, nullText(c.Error), c.Latency.Milliseconds(),
		nullInt(c.InputTokens), nullInt(c.OutputTokens), c.Cost, nullText(c.Output))
	if err != nil {
		s.log.Error("model call log", "purpose", c.Purpose, "err", err)
	}
}
