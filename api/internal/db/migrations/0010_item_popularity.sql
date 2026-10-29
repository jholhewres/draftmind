-- B1: what an item is (dotaconstants qual, e.g. consumable, and the keys of
-- its components), and how often each hero buys it, by game phase, from
-- OpenDota /heroes/{id}/itemPopularity, keyed to the patch.
ALTER TABLE item
    ADD COLUMN qual       text NOT NULL DEFAULT '',
    ADD COLUMN components text[] NOT NULL DEFAULT '{}';

CREATE TABLE hero_item_popularity (
    patch_id integer NOT NULL,
    hero_id  integer NOT NULL,
    phase    text NOT NULL CHECK (phase IN ('start', 'early', 'mid', 'late')),
    item_id  integer NOT NULL,
    count    integer NOT NULL CHECK (count > 0),
    PRIMARY KEY (patch_id, hero_id, phase, item_id),
    FOREIGN KEY (patch_id, hero_id) REFERENCES hero_patch (patch_id, hero_id) ON DELETE CASCADE,
    -- An item the patch drops takes its counts with it.
    FOREIGN KEY (patch_id, item_id) REFERENCES item (patch_id, id) ON DELETE CASCADE
);
