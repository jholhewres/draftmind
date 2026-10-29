-- D1: accounts (ADR-0008). A person signs in with an e-mail link or Steam and
-- holds a server-side session. Tokens and session ids are stored as their
-- SHA-256, never as sent.
CREATE TABLE app_user (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    email      text UNIQUE,                      -- lower case
    steam_id   text UNIQUE CHECK (steam_id ~ '^[0-9]{17}$'),
    CHECK (email IS NOT NULL OR steam_id IS NOT NULL)
);

-- A magic link: works once, until expires_at.
CREATE TABLE login_token (
    token_hash bytea PRIMARY KEY,
    email      text NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);

CREATE INDEX login_token_expires_at ON login_token (expires_at);

CREATE TABLE user_session (
    id_hash    bytea PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES app_user (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE INDEX user_session_user_id ON user_session (user_id);
CREATE INDEX user_session_expires_at ON user_session (expires_at);

-- A signed-in player's match belongs to them; a visitor's stays null.
ALTER TABLE match_session
    ADD CONSTRAINT match_session_user_id_fkey FOREIGN KEY (user_id) REFERENCES app_user (id) ON DELETE SET NULL;
