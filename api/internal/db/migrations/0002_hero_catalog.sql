-- One row per sync run, with what it changed and why it failed.
CREATE TABLE sync_run (
    id          bigserial PRIMARY KEY,
    source      text NOT NULL,
    status      text NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    inserted    integer NOT NULL DEFAULT 0,
    updated     integer NOT NULL DEFAULT 0,
    unchanged   integer NOT NULL DEFAULT 0,
    error       text
);

CREATE INDEX sync_run_source_started ON sync_run (source, started_at DESC);

-- Heroes keyed by the game's own hero id.
CREATE TABLE hero (
    id           integer PRIMARY KEY,
    slug         text NOT NULL UNIQUE,
    name         text NOT NULL,
    primary_attr text NOT NULL,
    attack_type  text NOT NULL,
    roles        text[] NOT NULL DEFAULT '{}',
    img          text NOT NULL,
    icon         text NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now()
);
