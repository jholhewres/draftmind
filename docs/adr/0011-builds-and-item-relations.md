# ADR-0011: Builds, item relations and counter tags in the database

Status: accepted · 2026-10-29

## Context

The coach and the build card must never invent a build or an item (ADR-0003). Until now the database held items and their popularity per hero and phase (`hero_item_popularity`); the situational items came from popularity alone, and a stored build, an item's upgrades or what an item answers lived nowhere. The owner also asked to keep model calls to a minimum: code and templates first, the writer last.

## Decision

- **Tables, all keyed by patch** (migration 0019). A new patch starts empty and the sync refills it; nothing carries over silently.
  - `item_component (patch_id, item_id, component_id)` from dotaconstants `components`. Upgrades are the same rows read from the component side; there is no second table. A component the patch lacks is skipped.
  - `hero_build (patch_id, hero_id, position, source, name, games, win_rate)` and `hero_build_item (…, ord, phase, item_id)`: a named, ordered item list per hero and position (1–5). `source` is `opendota` (derived from popularity), `curated` or `coach`; `games` and `win_rate` are set when the data has them (for `opendota`, the hero's pub record summed over brackets 1–8). Skill order is left out: no cheap source.
  - `item_tag (patch_id, item_id, tag, source)` and `hero_tag (patch_id, hero_id, tag, source)`: what an item answers (`magic_damage`, `illusions`, `healing`…) and what a hero brings.
- **Curated tags live in `api/data/item_counters.json`**, versioned in the repo and embedded in the binary; never in Go code. It lists, per tag, the item keys that answer it; per hero slug, the tags the hero brings; and per dotaconstants role, tags every hero with that role brings (source `roles`). A curated hero tag wins over a role tag. Keys the current patch lacks are skipped. Bump `version` on every change; the sync logs it.
- **Sync.** `draftmind sync builds` loads item popularity, then `Relations`: components, tags and the `opendota` builds in one transaction, removing rows its inputs no longer give. A second run changes 0 rows. An `opendota` build is the popularity core (`items.Core`) at each position the hero's roles fit best (ADR-0009's role fit).
- **Situational candidates.** An item the hero buys at all that shares a tag with an enemy gets `TagLift` (0.05, a 5-point popularity lift) against that enemy, on top of the matchup lift. Candidates still come only from items on the patch; Jev still picks among them (ADR-0003).
- **Coach.** The writer's facts carry the stored builds for the player's position; its allowed names are those builds, the session's item plan and the names in the message. Any other item name fails the check and the template stands.
- **Templates first, model last.** The "Items now?" quick action is answered by code from the session's plan (`match.QuickItems`): no Jev and no writer call, a template the UI words, and item cards. The live coach (ADR for live mode) follows the same rule.

## Consequences

- Tag lists are opinion and go stale; the version and git history show who changed what. A tag that mismatches the game only biases candidates, it never adds an item off the patch.
- Builds per position are as good as the role fit: a flex hero gets builds at several positions with the same items until positional popularity exists.
- `curated` and `coach` builds have tables but no writer yet.
