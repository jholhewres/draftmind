-- B4: how a match ended, as the player confirmed it, with the coach's notes.
-- One per session; confirming again replaces it.
CREATE TABLE match_result (
    session_id uuid PRIMARY KEY REFERENCES match_session (id) ON DELETE CASCADE,
    result     text NOT NULL CHECK (result IN ('win', 'loss')),
    duration_s integer CHECK (duration_s BETWEEN 1 AND 14400),
    kills      integer CHECK (kills BETWEEN 0 AND 999),
    deaths     integer CHECK (deaths BETWEEN 0 AND 999),
    assists    integer CHECK (assists BETWEEN 0 AND 999),
    source     text NOT NULL CHECK (source IN ('tap', 'message', 'photo')),
    notes      jsonb NOT NULL DEFAULT '[]',
    created_at timestamptz NOT NULL DEFAULT now()
);
