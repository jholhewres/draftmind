-- OpenDota stats per patch (ADR-0002): picks and wins by bracket, and each
-- hero's record against every other hero on the patch.
CREATE TABLE hero_stat (
    patch_id integer NOT NULL,
    hero_id  integer NOT NULL,
    bracket  text NOT NULL,
    picks    integer NOT NULL CHECK (picks >= 0),
    wins     integer NOT NULL CHECK (wins >= 0 AND wins <= picks),
    PRIMARY KEY (patch_id, hero_id, bracket),
    FOREIGN KEY (patch_id, hero_id) REFERENCES hero_patch (patch_id, hero_id) ON DELETE CASCADE
);

-- hero_id's games and wins against opponent_id.
CREATE TABLE hero_matchup (
    patch_id    integer NOT NULL,
    hero_id     integer NOT NULL,
    opponent_id integer NOT NULL CHECK (opponent_id <> hero_id),
    games       integer NOT NULL CHECK (games >= 0),
    wins        integer NOT NULL CHECK (wins >= 0 AND wins <= games),
    PRIMARY KEY (patch_id, hero_id, opponent_id),
    FOREIGN KEY (patch_id, hero_id) REFERENCES hero_patch (patch_id, hero_id) ON DELETE CASCADE,
    FOREIGN KEY (patch_id, opponent_id) REFERENCES hero_patch (patch_id, hero_id) ON DELETE CASCADE
);

-- The ranking reads every candidate's record against a few enemies.
CREATE INDEX hero_matchup_opponent ON hero_matchup (patch_id, opponent_id);
