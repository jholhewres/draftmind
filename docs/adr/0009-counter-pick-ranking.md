# ADR-0009: Counter-pick ranking from OpenDota matchups

Status: accepted · 2026-10-28

## Context

The Draft tab ranks every legal hero against a typed draft (ADR-0003, step 2). OpenDota's `/heroes/{id}/matchups` samples are small: on the real data captured for Axe and Puck, a pair has 9 to 355 games, about 105 on average. Raw win rates over so few games swing by several points on noise alone.

## Decision

- **Sync.** `draftmind sync stats` reads `/heroStats` into `hero_stat` (picks and wins per bracket `1`..`8` and `pro`) and each current-patch hero's `/heroes/{id}/matchups` into `hero_matchup`, keyed by the current patch. One request per second without `OPENDOTA_API_KEY`, one per 100 ms with it; a 429 or 5xx is retried up to 3 times after `Retry-After` or a doubling delay. Each hero's matchups commit in their own transaction and the run stops at the first failure: saved rows stay, `sync_run` is marked failed, and the next run rewrites only rows that changed. The key never appears in errors or logs.
- **Shrinkage.** A matchup win rate is `(wins + k/2) / (games + k)` with **k = 100**: a record of 100 games keeps half of its distance from 50 %. A method-of-moments estimate on the captured matchups gave k = 72 (Puck) and 192 (Axe). The per-enemy delta is that rate minus 50, in points; a pair with no games is 0.
- **Score** (in win-rate points) = sum of the deltas against each enemy + 0.5 × (patch win rate − 50) + 4 × role fit. The patch win rate sums the pub brackets 1–8 of `hero_stat` (the pro sample is too small), shrunk the same way.
- **Role fit** is the best weight among the hero's dotaconstants roles for the player's position:

  | Position | Fit 1 | Fit 0.5 |
  |---|---|---|
  | 1 (safe lane) | Carry | — |
  | 2 (mid) | Nuker | Carry, Escape |
  | 3 (off lane) | Initiator, Durable | Disabler |
  | 4 (soft support) | Support | Initiator, Disabler, Nuker |
  | 5 (hard support) | Support | Disabler |

- Picked and banned heroes never enter the ranking; ties break by hero id. `POST /api/draft/suggest` returns the top 8 by default with every reason as data.

## Consequences

- A strong counter on a thin sample ranks below a moderate one on a large sample; that is intended.
- The deltas are against 50 %, so a hero that wins a lot in general also shows positive deltas; the patch term is kept small for that reason.
- k, the weights and the role table are constants in `api/internal/draft`; revisit them once real drafts are validated (SPEC open question 3).
