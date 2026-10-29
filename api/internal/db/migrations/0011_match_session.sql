-- B2: a match the player coaches through, and its chat. The id is a random
-- UUID the browser keeps; user_id stays null until accounts (D1) exist.
CREATE TABLE match_session (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    bigint,
    patch_id   integer NOT NULL REFERENCES patch (id) ON DELETE CASCADE,
    hero_id    integer NOT NULL,
    role       smallint NOT NULL CHECK (role BETWEEN 1 AND 5),
    allies     integer[] NOT NULL DEFAULT '{}',
    enemies    integer[] NOT NULL DEFAULT '{}',
    bans       integer[] NOT NULL DEFAULT '{}',
    item_plan  jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    ended_at   timestamptz,
    FOREIGN KEY (patch_id, hero_id) REFERENCES hero_patch (patch_id, hero_id) ON DELETE CASCADE
);
CREATE INDEX match_session_user ON match_session (user_id) WHERE user_id IS NOT NULL;

-- One chat line: the player's message, or the coach's answer or follow-up
-- question with the situation and decision behind it.
CREATE TABLE match_message (
    id         bigserial PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES match_session (id) ON DELETE CASCADE,
    author     text NOT NULL CHECK (author IN ('player', 'coach')),
    kind       text NOT NULL CHECK (kind IN ('text', 'answer', 'followup')),
    text       text NOT NULL DEFAULT '',
    situation  jsonb,
    category   text,
    priority   text,
    source     text,   -- the decision's source: jev or parser
    reason     text,   -- why a template stood in for the writer's text
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX match_message_session ON match_message (session_id, id);
