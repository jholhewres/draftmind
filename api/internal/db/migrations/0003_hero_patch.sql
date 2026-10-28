-- Which heroes each patch has; the API reads the current patch only (ADR-0007).
CREATE TABLE hero_patch (
    patch_id integer NOT NULL REFERENCES patch (id) ON DELETE CASCADE,
    hero_id  integer NOT NULL REFERENCES hero (id) ON DELETE CASCADE,
    PRIMARY KEY (patch_id, hero_id)
);

CREATE INDEX hero_patch_hero ON hero_patch (hero_id);

-- Databases synced before this table existed keep their heroes on the current patch.
INSERT INTO hero_patch (patch_id, hero_id)
SELECT p.id, h.id FROM patch p CROSS JOIN hero h WHERE p.is_current;

-- Other names players use for a hero (SPEC data model).
ALTER TABLE hero ADD COLUMN aliases text[] NOT NULL DEFAULT '{}';

-- The source commit a run read, and the rows it removed.
ALTER TABLE sync_run
    ADD COLUMN revision text,
    ADD COLUMN deleted  integer NOT NULL DEFAULT 0;
