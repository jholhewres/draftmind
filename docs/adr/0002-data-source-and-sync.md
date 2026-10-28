# ADR-0002: Game data from dotaconstants and OpenDota, synced per patch

Status: accepted · 2026-10-27 · amended by [ADR-0007](0007-valve-datafeed.md)

## Context

The coach must never invent heroes, items or matchups. The data has to be current for the live patch and cheap to refresh. Sources considered: dotaconstants (static game data as JSON, maintained by OpenDota), the OpenDota API (stats from parsed matches), STRATZ GraphQL (richer, needs a token), and scraping guide sites (fragile, terms risk).

## Decision

- **Static data** (heroes, abilities, hero_abilities, items, patch) from **dotaconstants** JSON at a pinned version or commit.
- **Stats** from the **OpenDota API**: `/heroStats` (picks and wins per bracket), `/heroes/{id}/matchups` (hero vs hero), `/heroes/{id}/itemPopularity` (items by phase).
- `draftmind sync` writes everything into Postgres keyed by `patch`, with idempotent upserts and a `sync_run` row per run. Requests are rate limited under OpenDota's free tier (≈ 1 req/s); an optional `OPENDOTA_API_KEY` raises it.
- Re-sync when dotaconstants reports a new patch, plus a periodic stats refresh.
- Matchup win rates are shrunk toward 50 % by games played before ranking, because the matchup sample is small.
- **STRATZ** stays optional, first candidate for hero-pair synergy (open question in the SPEC).

## Consequences

- The database is the single source of truth; runtime requests never call OpenDota.
- A bad sync can be rolled back by marking the previous patch current.
- Quality of counters depends on OpenDota's sample; we log games played and show low-sample reasons as weaker.
