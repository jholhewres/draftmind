-- The items of each patch from dotaconstants items.json, at the revision
-- the hero sync pins. Minimal for now: B1 adds components and tags.
CREATE TABLE item (
    patch_id   integer NOT NULL REFERENCES patch (id) ON DELETE CASCADE,
    id         integer NOT NULL,
    key        text NOT NULL,             -- dotaconstants key, e.g. blink
    dname      text NOT NULL,             -- display name, in English
    cost       integer NOT NULL DEFAULT 0 CHECK (cost >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (patch_id, id),
    UNIQUE (patch_id, key)
);
