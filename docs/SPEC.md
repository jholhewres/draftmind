# Draftmind — Specification

Status: draft v1 · 2026-10-27

## 1. What it is

Draftmind is a web app that works as a smart guide and in-match coach for Dota 2 players.

1. **Guide.** A page per hero: abilities, roles, popular items per game phase, counters (who it beats and who beats it) and synergies.
2. **Coach.** During a match the player keeps Draftmind open on a phone or a second screen:
   - **Pre-match:** the player photographs the pick screen. Draftmind reads the draft and, as fast as possible, suggests the heroes the player can pick.
   - **Items:** once the hero is locked, it suggests the item build for the lineup it faces.
   - **Until the end:** the player tells the coach (text or push-to-talk voice) how the game is going and what is hard; the advice adapts.
   - The player can shape their own coach (tone, language, focus), the "own AI agent".

### Trademark and fair-play note

Draftmind is an independent tool. It is not affiliated with or endorsed by Valve Corporation; Dota 2 is a trademark of Valve. The product name and logo never use "Dota". Draftmind runs on a separate device or browser tab: it never reads the game's memory, injects into the client, or draws an overlay over the game. Its only inputs are what the player types, says or photographs. The UI shows this note in the footer and on the about page.

## 2. Users and success

- **Who:** pub players (any rank) who want quick draft and item help without alt-tabbing through several sites. The first user is Jhol.
- **Success for v1:**
  - Photo of the pick screen → hero suggestions on screen in **≤ 8 s p50** (stretch: 5 s), measured from upload to first rendered suggestion.
  - The draft read is right for **≥ 90 % of hero slots** on the fixture set of real photos; every wrong slot can be fixed in one tap.
  - Every hero, counter and item the coach names exists in the database for the current patch (0 hallucinated entities, enforced in code).

## 3. Principles

- **The database is the source of truth.** Heroes, abilities, items, matchups and item popularity are synced from dotaconstants and the OpenDota API and stored in Postgres, versioned by patch. The LLM never invents game facts.
- **The LLM writes, Jev decides, the code executes.** Code builds the candidate set from the database; the Jev decision model (OpenRouter System One) picks and scores among those candidates; an LLM only parses free input and writes the final short text from the structured result. See ADR-0003.
- **Fast first, refine after.** The deterministic ranking is shown as soon as it exists; the Jev decision and the written tip refine it in place.
- **Fail open to the code.** If a model call fails or times out, the player still gets the database ranking, labelled as such.
- **No locale assumptions in logic.** UI strings are translatable (en and pt-BR in v1); the coach answers in the player's language.

## 4. Functional scope

### 4.1 Data sync
- `draftmind sync` pulls dotaconstants (heroes, abilities, hero_abilities, items, patch) and OpenDota (`/heroStats`, `/heroes/{id}/matchups`, `/heroes/{id}/itemPopularity`).
- Idempotent upserts keyed by the current patch; each run recorded in `sync_run` with counts and errors.
- Runs on demand and on patch change (detected from dotaconstants `patch.json`).

### 4.2 Draft read (vision)
- `POST /api/draft/read` takes one image (JPEG/PNG/WebP, ≤ 5 MB, downscaled in the browser to ~1600 px on the long side).
- A vision model on OpenRouter returns structured JSON: both teams' picks (and bans when visible), the player's side when it can tell, and a confidence per slot.
- Names are resolved against the hero table (exact, alias, then fuzzy); unresolved slots come back empty and flagged.
- The player confirms or fixes the draft before or while suggestions render. Images are not stored by default.

### 4.3 Hero suggestions
- Input: allies, enemies, bans, the player's role (pos 1–5) and optionally their hero pool.
- Code ranks every legal hero (not picked, not banned) by: counter advantage against enemy picks, synergy with allies (when data exists), role fit and patch win rate in the player's bracket. Each score keeps its reasons as data.
- Jev chooses among the top N (default 8) and returns probabilities and confidence; the UI shows the top 3–5 with reasons.
- A short tip per hero is written from the structured reasons only.

### 4.4 Items
- After the pick: starting, early, mid and late items from item popularity for the hero, adjusted to the enemy lineup (situational items such as BKB, Silver Edge, Spirit Vessel chosen by Jev from a code-built candidate list).

### 4.5 Coach session
- A match session holds: draft, player hero and role, item plan, elapsed time (manual), and the chat history.
- Each player message is parsed by an LLM into a structured situation (e.g. "losing lane vs Viper", "enemy has 3 stuns"); code fetches the relevant facts; Jev decides the advice category and priority; the LLM writes the answer.
- Custom coach: per-user settings for tone, language, verbosity and focus, applied to the writer prompt only (they never change what is decided).

### 4.6 Voice (v1)
- Push-to-talk: hold to record in the browser, release to send. Speech-to-text on the server, the normal coach pipeline, text-to-speech back. See ADR-0005.

### 4.7 Hero guide
- Hero list with search and role filter; hero page with abilities, item popularity by phase, top counters, top victims and synergies, all for the current patch.

## 5. Out of scope (v1)

- Reading the game client in any way (memory, overlays, injected UI). Dota Game State Integration is a later, separate option and still never an overlay.
- Replay (`.dem`) parsing and post-match analysis.
- OpenAI Realtime voice (needs an OpenAI key; later, behind the same session API).
- Payments, public sign-up at scale, native mobile apps.

## 6. Architecture

```
browser (React/Vite) ──HTTP/SSE──▶ api (Go)
                                     ├─ Postgres (heroes, items, matchups, sessions, decision log)
                                     ├─ OpenRouter: vision model (draft read)
                                     ├─ OpenRouter: System One / Jev (decisions)
                                     ├─ OpenRouter: writer LLM (parse input, write tips)
                                     └─ sync job ◀── dotaconstants, OpenDota
```

- Monorepo: `api/` (Go module `github.com/jholhewres/draftmind/api`), `web/` (React + Vite + TypeScript), `docker-compose.yml` (Postgres), `Makefile`.
- Suggestions stream over Server-Sent Events: `ranking` → `decision` → `tip` events, so the first list renders before the models finish.
- Every model call (vision, Jev, writer, STT) is logged in `model_call` with model, latency, tokens, cost and outcome; Jev answers also in `decision_log`.

### Data model (initial)

| Table | Purpose |
|---|---|
| `patch` | Game patch (name, release date, current flag) |
| `hero` | id, slug, localized name, aliases, attributes, roles, attack type, image paths |
| `ability`, `hero_ability` | Abilities and their order per hero |
| `item` | Items with cost, components, tags |
| `hero_stat` | Per patch and bracket: picks, wins |
| `hero_matchup` | Per patch: hero vs hero, games, wins |
| `hero_synergy` | Per patch: hero with hero, games, wins (source TBD, see PLAN) |
| `hero_item_popularity` | Per patch: hero, phase, item, count |
| `sync_run` | Each sync: source, started, finished, counts, error |
| `coach_session`, `coach_message` | Match session state and chat |
| `draft_read` | Extracted draft JSON, confidence, corrections (no image) |
| `model_call`, `decision_log` | Audit of model calls and Jev answers |

## 7. Non-functional

- **Latency budgets:** vision ≤ 4 s, ranking ≤ 100 ms, Jev ≤ 1.5 s (hedged), writer first token ≤ 1.5 s.
- **Cost:** tracked per call in `model_call`; budget per coach session reported in the UI later.
- **Secrets:** `OPENROUTER_API_KEY` lives only in the local, gitignored `.env`; never logged.
- **Privacy:** no images stored by default; no account data sent to models; OpenRouter calls request zero data retention where the provider supports it.
- **Ops:** one Go binary plus Postgres; deploy follows the personal-server pattern (app under `/var/www/<domain>/`, bound to a random port on 127.0.0.1).

## 8. Open questions

1. **Synergy source.** OpenDota has no hero-pair synergy endpoint. Options: STRATZ GraphQL (needs a token) or aggregating our own sample from OpenDota public matches. Decide in the synergy slice.
2. **Matchup sample size.** OpenDota `/heroes/{id}/matchups` comes from a limited match sample; shrink win rates toward 50 % by games played. Validate on real drafts.
3. **Vision model.** Pick the model by a benchmark on real photos (accuracy, p50 latency, cost) in the vision slice.
4. **Accounts.** Anonymous sessions first; when to add login (needed for the custom coach and history).
