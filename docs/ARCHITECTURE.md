# Architecture

This page is a map for contributors. Decisions and their reasons are in [adr/](adr/).

## Principles

- **The database is the source of truth.** Heroes, abilities, items, matchups, item popularity and patch notes are synced into Postgres and versioned by patch. Code never hardcodes game facts, and no model output may name an entity that is not in the database for the current patch.
- **The LLM writes, Jev decides, the code executes** ([ADR-0003](adr/0003-decision-pipeline.md)):
  - code builds the candidates from the database;
  - Jev (TypeSafe System One on OpenRouter) chooses and scores among them;
  - a writer model only parses free input and writes short text from the decided result, and an entity check rejects any name outside it.
- **Fast first, refine after.** The deterministic ranking renders first; the Jev decision and the tips refine it in place.
- **Fail open to the code.** When a model call fails or is late, the player gets the code ranking, labelled as such.
- **No locale assumptions in logic.** UI strings come from the en and pt-BR catalogs. Game text is synced in both languages.

## Overview

```
browser (React/Vite) ──HTTP/SSE──▶ api (Go, one binary: serve, migrate, sync)
                                     ├─ Postgres (game data, users, matches, model logs)
                                     ├─ OpenRouter: Jev (decisions, photo read)
                                     ├─ OpenRouter: vision model (photo read fallback, end screen)
                                     ├─ OpenRouter: writer and STT models
                                     └─ sync ◀── dotaconstants, the game's data feed, OpenDota
```

- Suggestions stream over Server-Sent Events: `read` → `ranking` → `decision` → `tip`.
- Every model call is logged in `model_call`, and Jev answers also go to `decision_log`.
- With `SYNC_SCHEDULE=on`, `serve` checks for a new patch every hour. A patch becomes current only when every source synced.

## Code map

| Path | What lives there |
|---|---|
| `api/cmd/draftmind` | Entry point: `serve`, `migrate`, `sync <heroes\|details\|stats\|builds\|patchnotes>`, `bench` (photo benchmark) |
| `api/internal/gamesync` | Sync from dotaconstants, the data feed and OpenDota; the patch scheduler |
| `api/internal/catalog` | Read side of the game data: heroes, counters, builds, patches |
| `api/internal/draft` | Counter-pick ranking ([ADR-0009](adr/0009-counter-pick-ranking.md)) and the Jev decision over it |
| `api/internal/decision` | System One client: strict parsing, deadline, retries, hedge, logging |
| `api/internal/photo` | Photo readers: Jev images and the vision model ([ADR-0006](adr/0006-photo-read-with-jev.md)) |
| `api/internal/writer` | Writer model prompts and the entity check |
| `api/internal/items` | Item build: core items and situational candidates |
| `api/internal/match` | Match session, coach pipeline, voice, result |
| `api/internal/persona` | Coach persona settings (writer prompt only) |
| `api/internal/auth` | Password, magic link and Steam sign-in, server-side sessions ([ADR-0008](adr/0008-accounts.md), [ADR-0010](adr/0010-password-sign-in.md)) |
| `api/internal/httpapi` | HTTP routes, SSE, rate limits, static web serving |
| `api/internal/db` | Pool and embedded migrations (`migrations/`, applied once in name order) |
| `web/src` | React app: `draft/`, `coach/`, `heroes/`, `patches/`, `auth/`, `landing/`, `shell/`, `locales/` |
| `web/e2e` | Playwright visual tests at phone and desktop sizes |

## Data model

| Table | Purpose |
|---|---|
| `patch`, `patch_release` | Game patches (current flag) and every release the data feed lists |
| `hero`, `hero_patch`, `hero_text` | Heroes, their per-patch membership and stats, and localized text |
| `ability`, `ability_text` | Abilities and talents per patch, with localized text |
| `item` | Items of the current patch |
| `hero_stat`, `hero_matchup` | Per-patch picks and wins by bracket, and hero-versus-hero matchups |
| `hero_item_popularity` | Per-patch item popularity by game phase |
| `patch_note` | Patch notes by target (hero, ability, talent, item, general) and language |
| `sync_run` | Each sync: source, revision, counts, error |
| `app_user`, `user_session`, `login_token` | Accounts and sessions |
| `match_session`, `match_message`, `match_result`, `coach_persona` | Match state, chat, result and coach settings |
| `model_call`, `decision_log` | Audit of model calls and Jev answers |

## Budgets

| Step | Budget |
|---|---|
| Photo read | ≤ 3.5 s |
| Ranking | ≤ 100 ms |
| Jev | ≤ 1.5 s (hedged) |
| Writer first token | ≤ 1.5 s |
| Photo to first suggestion | ≤ 5 s p50 |

Images are not stored, and no account data is sent to models.

## Open questions

- **Synergy:** OpenDota has no hero-pair synergy endpoint. The candidates are STRATZ or aggregating public matches.
- **Matchup data:** matchup samples are small. Win rates are shrunk toward 50% by games played; tune this on real drafts.
- **Photo reader:** it is calibrated on synthetic photos and two real screenshots. More real phone photos are welcome in `api/testdata/photos/real/`.
