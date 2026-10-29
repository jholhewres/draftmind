# ADR-0012: Draft game modes

Status: accepted · 2026-10-29

## Context

Until now every draft was ranked All Pick. Players also draft in Turbo, Captains Mode, Captains Draft, Single Draft, Random Draft and All Random, and the live draft and live coach that follow must know the mode. The data we have is not split evenly by mode: OpenDota gives hero stats per rank bracket plus a separate Turbo pick/win count, while matchups and item popularity mix every mode.

## Decision

- **Supported modes** (`draft.Mode`): `all_pick_ranked` (default), `all_pick`, `turbo`, `captains_mode`, `captains_draft`, `single_draft`, `random_draft`, `all_random`. Ability Draft and custom games are listed in the UI as not supported; the API rejects any other value (400).
- **Stored and remembered.** `match_session.game_mode` (default `all_pick_ranked`) and `app_user.game_mode` (migration 0020). A signed-in player's last mode is the next draft's default (`GET /api/me/mode`); a visitor's is kept in the browser.
- **Stats per mode.** `sync stats` stores OpenDota `turbo_picks`/`turbo_wins` as bracket `turbo`. A Turbo draft ranks on those when present and on the ranked brackets otherwise; the response says which (`stats: turbo|ranked`). Matchups and item popularity stay mixed across modes; a Turbo build is labelled "general item data".
- **Turbo scaling.** Time-based thresholds scale by 0.5 and gold thresholds by 2 (`TimeScale`, `GoldScale`), for the live coach's code triggers.
- **Bans.** Modes with a ban phase (Captains Mode, Captains Draft) also return ban suggestions: the heroes that would rank best against our allies, i.e. the same ranking with the draft flipped. Code only, no model call.
- **Pools.** Single Draft, Random Draft and Captains Draft rank only the heroes the player says are offered (`pool`, up to 50, validated on patch). Without a pool nothing is ranked. A photo carries no pool, so a pooled mode is read and ranked as the default.
- **All Random** ranks nothing: the player sets the hero they got and goes straight to the match and its build.
- **Readers stay honest.** The photo and live readers are calibrated for the All Pick pick screen (ranked, unranked, Turbo). On any other mode every slot is flagged unknown (`layout: uncalibrated`) and the player taps each hero; no guessing.

## Consequences

- Turbo rankings are only as good as Turbo's pick counts; small samples fall back to ranked stats rather than invent.
- Calibrating the Captains Mode layout is future work; until then those reads cost the player a few taps.
