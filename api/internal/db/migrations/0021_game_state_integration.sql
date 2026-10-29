-- F7: Game State Integration (ADR-0015). One token per account, stored as
-- its SHA-256, never as sent; revoking it deletes the row. The last state
-- the game posted is kept per account for the live coach.
CREATE TABLE gsi_token (
    user_id      bigint PRIMARY KEY REFERENCES app_user (id) ON DELETE CASCADE,
    token_hash   bytea NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_post_at timestamptz
);

CREATE TABLE gsi_state (
    user_id     bigint PRIMARY KEY REFERENCES app_user (id) ON DELETE CASCADE,
    state       jsonb NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE match_result DROP CONSTRAINT match_result_source_check;
ALTER TABLE match_result ADD CONSTRAINT match_result_source_check CHECK (source IN ('tap', 'message', 'photo', 'gsi'));
