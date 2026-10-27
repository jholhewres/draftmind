-- Game patches; every synced table is keyed by one.
CREATE TABLE patch (
    id          serial PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    released_at timestamptz,
    is_current  boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX patch_one_current ON patch (is_current) WHERE is_current;
