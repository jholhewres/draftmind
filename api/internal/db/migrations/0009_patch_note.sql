-- Patch notes from the game's data feed (ADR-0007). The feed lists every
-- release, letter patches included, so it has its own table next to patch.
CREATE TABLE patch_release (
    name        text PRIMARY KEY,
    released_at timestamptz NOT NULL
);

CREATE INDEX patch_release_released ON patch_release (released_at DESC);

-- One line of a patch's notes in one language, in feed order (seq).
CREATE TABLE patch_note (
    patch_name text NOT NULL REFERENCES patch_release (name) ON DELETE CASCADE,
    lang       text NOT NULL,
    seq        integer NOT NULL CHECK (seq >= 0),
    kind       text NOT NULL CHECK (kind IN ('hero', 'ability', 'talent', 'item', 'general')),
    hero_id    integer,   -- hero, ability and talent notes
    target_id  integer,   -- the ability of an ability note, the item of an item note
    indent     smallint NOT NULL DEFAULT 0,
    text       text NOT NULL,
    PRIMARY KEY (patch_name, lang, seq)
);

-- A hero's history reads its notes across patches.
CREATE INDEX patch_note_hero ON patch_note (hero_id, lang) WHERE hero_id IS NOT NULL;
