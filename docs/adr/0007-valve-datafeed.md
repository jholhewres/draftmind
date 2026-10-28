# ADR-0007: Hero details and patch notes from the game's public data feed

Status: accepted · 2026-10-28 · Amends [ADR-0002](0002-data-source-and-sync.md)

## Context

Players want current details for every hero (stats, abilities, lore) and the changes each patch made, kept as a per-hero history. dotaconstants carries static data but trails the live game. The official site publishes public JSON feeds that its own hero and patch pages use:

- `www.dota2.com/datafeed/herolist` and `/datafeed/herodata?hero_id=<id>&language=<lang>`: stats, attributes, abilities with values, talents, lore, localized.
- `www.dota2.com/datafeed/patchnoteslist` and `/datafeed/patchnotes?version=<patch>&language=<lang>`: every patch with its timestamp, and its notes grouped by hero, ability, talent and item, keyed by `hero_id` and `ability_id`.

## Decision

- dotaconstants stays the source of hero ids, slugs and image paths (ADR-0002). The data feed adds details and patch notes on top, keyed by the same `hero_id`.
- Heroes are scoped to a patch: `hero_patch` records which heroes, stats and abilities each patch has, and the API reads the current patch only.
- `patch_note` stores every note with its patch, target (hero, ability, talent, item, general) and text, in `en` and `brazilian`. A hero's history is its notes ordered by patch.
- Each sync records the dotaconstants commit it read (resolved from the ref) in `sync_run`, so a run can be reproduced.
- Requests are rate limited and cached by patch; runtime requests never call the feed.

## Consequences

- The feed is undocumented and can change without notice. Parsing is strict, a failed parse fails the run, and the previous patch stays current.
- Localized text comes from the source, so `pt-BR` hero pages need no translation step.
- The terms of the official site apply. We store facts and short notes for the app's own pages, credit the source, and keep the trademark note in the footer.
