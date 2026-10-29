-- Every call to a model other than Jev's decisions (SPEC: writer, later STT
-- and parse): what it cost, how long it took and how it ended. Never the
-- API key, never the player's photo.
CREATE TABLE model_call (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at     timestamptz NOT NULL DEFAULT now(),
    purpose        text NOT NULL,
    model          text NOT NULL,             -- the model asked for
    answered_model text,                      -- the model that answered
    outcome        text NOT NULL CHECK (outcome IN ('ok', 'rejected', 'timeout', 'error')),
    error          text,
    latency_ms     integer NOT NULL CHECK (latency_ms >= 0),
    input_tokens   integer,
    output_tokens  integer,
    cost_usd       numeric,                   -- from the response usage, when reported
    output         text                       -- the text, kept also when the check rejected it
);

CREATE INDEX model_call_created_at ON model_call (created_at);
