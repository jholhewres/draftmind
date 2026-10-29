-- ADR-0012: game modes. A match session records its mode; a signed-in
-- player's last mode is remembered. OpenDota's Turbo picks and wins are kept
-- as hero_stat bracket 'turbo'.
ALTER TABLE match_session ADD COLUMN game_mode text NOT NULL DEFAULT 'all_pick_ranked'
    CHECK (game_mode IN ('all_pick_ranked', 'all_pick', 'turbo', 'captains_mode', 'captains_draft', 'single_draft', 'random_draft', 'all_random'));
ALTER TABLE app_user ADD COLUMN game_mode text
    CHECK (game_mode IN ('all_pick_ranked', 'all_pick', 'turbo', 'captains_mode', 'captains_draft', 'single_draft', 'random_draft', 'all_random'));
