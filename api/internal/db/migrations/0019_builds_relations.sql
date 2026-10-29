-- ADR-0011: builds, item relations and counter tags, all keyed by patch so
-- a new patch starts empty and the sync refills it.

-- An item and each item it is built from, from dotaconstants components.
-- Upgrades are the same rows read the other way.
CREATE TABLE item_component (
    patch_id     integer NOT NULL,
    item_id      integer NOT NULL,
    component_id integer NOT NULL CHECK (component_id <> item_id),
    PRIMARY KEY (patch_id, item_id, component_id),
    FOREIGN KEY (patch_id, item_id) REFERENCES item (patch_id, id) ON DELETE CASCADE,
    FOREIGN KEY (patch_id, component_id) REFERENCES item (patch_id, id) ON DELETE CASCADE
);
CREATE INDEX item_component_upgrade ON item_component (patch_id, component_id);

-- A named, ordered item list for a hero at a position (1-5).
CREATE TABLE hero_build (
    patch_id integer NOT NULL,
    hero_id  integer NOT NULL,
    position smallint NOT NULL CHECK (position BETWEEN 1 AND 5),
    source   text NOT NULL CHECK (source IN ('opendota', 'curated', 'coach')),
    name     text NOT NULL,
    games    integer CHECK (games >= 0),        -- when the data has a pick count
    win_rate double precision CHECK (win_rate BETWEEN 0 AND 1),
    PRIMARY KEY (patch_id, hero_id, position, source),
    FOREIGN KEY (patch_id, hero_id) REFERENCES hero_patch (patch_id, hero_id) ON DELETE CASCADE
);

CREATE TABLE hero_build_item (
    patch_id integer NOT NULL,
    hero_id  integer NOT NULL,
    position smallint NOT NULL,
    source   text NOT NULL,
    ord      smallint NOT NULL CHECK (ord >= 0),
    phase    text NOT NULL CHECK (phase IN ('start', 'early', 'mid', 'late')),
    item_id  integer NOT NULL,
    PRIMARY KEY (patch_id, hero_id, position, source, ord),
    FOREIGN KEY (patch_id, hero_id, position, source) REFERENCES hero_build (patch_id, hero_id, position, source) ON DELETE CASCADE,
    FOREIGN KEY (patch_id, item_id) REFERENCES item (patch_id, id) ON DELETE CASCADE
);

-- What an item answers (magic damage, illusions...), and what a hero brings,
-- from api/data/item_counters.json (curated) or the hero's roles (roles).
CREATE TABLE item_tag (
    patch_id integer NOT NULL,
    item_id  integer NOT NULL,
    tag      text NOT NULL,
    source   text NOT NULL CHECK (source IN ('curated')),
    PRIMARY KEY (patch_id, item_id, tag),
    FOREIGN KEY (patch_id, item_id) REFERENCES item (patch_id, id) ON DELETE CASCADE
);

CREATE TABLE hero_tag (
    patch_id integer NOT NULL,
    hero_id  integer NOT NULL,
    tag      text NOT NULL,
    source   text NOT NULL CHECK (source IN ('curated', 'roles')),
    PRIMARY KEY (patch_id, hero_id, tag),
    FOREIGN KEY (patch_id, hero_id) REFERENCES hero_patch (patch_id, hero_id) ON DELETE CASCADE
);
