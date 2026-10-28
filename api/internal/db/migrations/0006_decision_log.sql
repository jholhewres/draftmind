-- Every call to the decision model (ADR-0003): what was asked, what came
-- back, how long it took and what it cost. Never the API key.
CREATE TABLE decision_log (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at     timestamptz NOT NULL DEFAULT now(),
    purpose        text NOT NULL,
    model          text NOT NULL,             -- the model asked for
    answered_model text,                      -- the snapshot that answered
    outcome        text NOT NULL CHECK (outcome IN ('ok', 'timeout', 'error')),
    error          text,
    latency_ms     integer NOT NULL CHECK (latency_ms >= 0),
    attempts       integer NOT NULL CHECK (attempts >= 0),
    hedged         boolean NOT NULL,
    input_tokens   integer,
    output_tokens  integer,
    cost_usd       numeric,                   -- from the response usage, when reported
    request        jsonb NOT NULL,
    answer         jsonb
);

CREATE INDEX decision_log_created_at ON decision_log (created_at);
