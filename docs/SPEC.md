# Draftmind — Specification

Status: draft v2 · 2026-10-28

## 1. What it is

Draftmind is a web app that works as a smart guide and in-match coach for Dota 2 players.

1. **Guide.** Every hero with current stats and abilities, its counters, the builds used in game and its history of changes, patch by patch. A patches page lists what each patch changed.
2. **Coach.** During a match the player keeps Draftmind open on a phone or a second screen:
   - **Pick:** the player photographs the pick screen. The pick leaves 15–20 s, so Draftmind shows the read draft and the heroes to pick on one screen in about 5 s.
   - **Items:** once the hero is locked, it suggests the item build for the lineup it faces.
   - **Until the end:** the player tells the coach (text, push-to-talk voice or a screenshot) how the game is going; the advice adapts.
   - **Result:** at the end the player says whether they won or lost, with a tap, a photo of the end screen or a message. The match goes into their history.
   - The player can shape their own coach (tone, language, focus), the "own AI agent".
3. **Accounts and landing page.** A short landing page explains the app; sign-in by e-mail link or Steam keeps history and settings.

### Trademark and fair-play note

Draftmind is an independent tool. It has no relation to Valve Corporation or the game, and is not endorsed by them; Dota 2 is a trademark of Valve. The product name and logo never use "Dota". Draftmind runs on a separate device or browser tab: it never reads the game's memory, injects into the client, or draws an overlay over the game. Its only inputs are what the player types, says or photographs. The UI shows this note in the footer of the landing page, the sign-in page and the main tabs. Hero names and images belong to their owners and are credited as such.

## 2. Users and success

- **Who:** pub players (any rank) who want quick draft and item help without alt-tabbing through several sites. The first user is Jhol.
- **Success for v1:**
  - Photo of the pick screen → first suggestion on screen in **≤ 5 s p50 and ≤ 8 s p95**, measured from upload to first rendered suggestion.
  - The draft read is right for **≥ 90 % of hero slots** on the fixture set of real photos; every wrong slot can be fixed in one tap.
  - Every hero, ability, counter and item the app names exists in the database for the current patch (0 hallucinated entities, enforced in code).
  - A new patch is in the database, with its notes, within 24 h of release.

## 3. Principles

- **The database is the source of truth.** Heroes, abilities, items, matchups, item popularity, guides and patch notes are synced from dotaconstants, the game's public data feed and the OpenDota API and stored in Postgres, versioned by patch. The LLM never invents game facts.
- **The LLM writes, Jev decides, the code executes.** Code builds the candidate set from the database; Jev picks and scores among those candidates; an LLM only parses free input and writes the final short text from the structured result. See ADR-0003.
- **Fast first, refine after.** The deterministic ranking is shown as soon as it exists; the Jev decision and the written tip refine it in place.
- **Fail open to the code.** If a model call fails or times out, the player still gets the database ranking, labelled as such.
- **No locale assumptions in logic.** UI strings come from message catalogs (en and pt-BR in v1); game text is synced in both languages; the coach answers in the player's language.

## 4. Functional scope

### 4.1 Data sync
- `draftmind sync` pulls:
  - dotaconstants: hero ids, slugs, images, items, patches;
  - the game's data feed: hero details (stats, abilities, talents, lore) and patch notes, in English and Brazilian Portuguese (ADR-0007);
  - OpenDota: `/heroStats`, `/heroes/{id}/matchups`, `/heroes/{id}/itemPopularity`.
- Idempotent upserts keyed by patch; each run is recorded in `sync_run` with counts, errors and the source revision it read.
- Runs on demand and on patch change. A new patch becomes current only when its sync succeeds.

### 4.2 Photo read
- `POST /api/draft/read` takes one image (JPEG/PNG/WebP, ≤ 5 MB, downscaled in the browser to ~1600 px on the long side).
- The reader (Jev images or a vision model, chosen by benchmark, ADR-0006) returns both teams' picks, bans when visible, the player's side when it can tell, and a confidence per slot.
- Every slot is resolved against the current patch's heroes; unresolved slots come back empty and flagged.
- The end-of-match screen (result, duration, the player's KDA) is read by the vision model (ADR-0006, amended in B4). Images are not stored by default.

### 4.3 Hero suggestions
- Input: allies, enemies, bans, the player's role (pos 1–5) and optionally their hero pool.
- Code ranks every legal hero (not picked, not banned) by counter advantage against enemy picks, synergy with allies (when data exists), role fit and patch win rate. Each score keeps its reasons as data.
- Jev chooses among the top N (default 8) and returns probabilities and confidence; the UI shows the top 3–5 with reasons.
- The read draft and the suggestions share one screen: the ranking renders as soon as the read returns, and fixing a slot re-ranks in place.
- A short tip per hero is written from the structured reasons only.

### 4.4 Items
- After the pick: starting, early, mid and late items from item popularity and the game's guides for the hero, adjusted to the enemy lineup (situational items chosen by Jev from a code-built candidate list).

### 4.5 Match session
- A match session holds: draft, player hero and role, item plan, elapsed time, the chat history and the result.
- Each player message (text, transcript or screenshot) is parsed into a structured situation; code fetches the relevant facts; Jev decides the advice category and priority; the LLM writes the answer.
- **Result:** the player ends the session with a tap (win or loss), a photo of the end screen or a message. A photo fills the result, duration and KDA for the player to confirm. The coach adds 2–3 notes from the session, and the match goes into the player's history.
- Custom coach: per-user settings for tone, language, verbosity and focus, applied to the writer prompt only (they never change what is decided).

### 4.6 Voice (v1)
- Push-to-talk: hold to record in the browser, release to send. Speech-to-text on the server, the normal coach pipeline, text-to-speech back. See ADR-0005.

### 4.7 Hero guide and patches
- **Heroes tab:** every hero of the current patch with its portrait, search, and attribute and role filters.
- **Hero page:**
  - stats, abilities and talents;
  - top counters and top victims;
  - item popularity and the most used build from the game's guides;
  - the hero's history of changes, newest patch first.
- **Patches tab:** the patches in order; for each one, the changes to heroes and items, with a filter for the heroes the player uses.

### 4.8 Accounts and landing page
- **Landing page:** one page with three sections (what it does, how it works, always on the current patch) and the trademark note.
- **Sign-in:** e-mail and password, Steam, or an e-mail magic link when SMTP is set (ADR-0008, ADR-0010).
- A visitor can try a draft read without an account. Saving history and coach settings needs one.

## 5. Out of scope (v1)

- Reading the game client in any way (memory, overlays, injected UI). Dota Game State Integration is a later, separate option and still never an overlay.
- Replay (`.dem`) parsing and deep post-match analysis.
- OpenAI Realtime voice (needs an OpenAI key; later, behind the same session API).
- Payments, public sign-up at scale, native mobile apps.

## 6. Architecture

```
browser (React/Vite) ──HTTP/SSE──▶ api (Go)
                                     ├─ Postgres (game data, users, matches, decision log)
                                     ├─ OpenRouter: System One / Jev (decisions, photo read)
                                     ├─ OpenRouter: vision model (photo read fallback)
                                     ├─ OpenRouter: writer LLM (parse input, write tips)
                                     └─ sync job ◀── dotaconstants, game data feed, OpenDota
```

- Monorepo: `api/` (Go module `github.com/jholhewres/draftmind/api`), `web/` (React + Vite + TypeScript), `docker-compose.yml` (Postgres), `Makefile`.
- Suggestions stream over Server-Sent Events: `read` → `ranking` → `decision` → `tip` events, so the first list renders before the models finish.
- Every model call (photo read, Jev, writer, STT) is logged in `model_call` with model, latency, tokens, cost and outcome; Jev answers also go in `decision_log`.
- **Look:**
  - dark theme close to the game's, with the green/red team colors and a gold accent;
  - hero portraits from the image paths in the database;
  - four tabs: Draft, Coach, Heroes, Patches;
  - mobile first.

### Data model

| Table | Purpose |
|---|---|
| `patch` | Game patch (name, release date, current flag) |
| `hero` | id, slug, names, aliases, image paths |
| `hero_patch` | Per patch: the hero's attributes, roles, stats and lore |
| `ability`, `hero_ability` | Abilities with values and their order per hero, per patch |
| `item` | Items with cost, components, tags |
| `patch_release` | Every release the data feed lists, letter patches included (name, release date) |
| `patch_note` | Per patch: note text by target (hero, ability, talent, item, general), per language |
| `hero_stat` | Per patch and bracket: picks, wins |
| `hero_matchup` | Per patch: hero vs hero, games, wins |
| `hero_synergy` | Per patch: hero with hero, games, wins (source TBD) |
| `hero_item_popularity` | Per patch: hero, phase, item, count |
| `hero_guide` | Per patch: the game's most used builds per hero (source TBD) |
| `sync_run` | Each sync: source, revision, started, finished, counts, error |
| `app_user`, `user_session`, `login_token` | Accounts and sessions |
| `match_session`, `match_message`, `match_result` | Match state, chat and result |
| `draft_read` | Extracted draft JSON, confidence, corrections (no image) |
| `model_call`, `decision_log` | Audit of model calls and Jev answers |

## 7. Non-functional

- **Latency budgets:** photo read ≤ 3.5 s, ranking ≤ 100 ms, Jev ≤ 1.5 s (hedged), writer first token ≤ 1.5 s.
- **Cost:** tracked per call in `model_call`; budget per match session reported in the UI later.
- **Secrets:** `OPENROUTER_API_KEY`, SMTP and session secrets live only in the local, gitignored `.env`; never logged.
- **Privacy:** no images stored by default; no account data sent to models; OpenRouter calls request zero data retention where the provider supports it.
- **Ops:** one Go binary plus Postgres; deploy follows the personal-server pattern (app under `/var/www/<domain>/`, bound to a random port on 127.0.0.1).

## 8. Open questions

1. **Synergy source.** OpenDota has no hero-pair synergy endpoint. Options: STRATZ GraphQL (needs a token) or aggregating our own sample from OpenDota public matches. Decide in the synergy slice.
2. **Game guides source.** The in-game guides live in the Steam Workshop. Decided in B1 (2026-10-29), which absorbed C3: the hero page and the coach show OpenDota item popularity alone, labelled as such; the Workshop guides stay out until a source for their builds exists.
3. **Matchup sample size.** OpenDota matchups come from a limited sample; shrink win rates toward 50 % by games played. Validate on real drafts.
4. **Photo reader.** Jev images (beta) or a vision model, by benchmark (ADR-0006).
5. **Image hosting.** Hero portraits load from the image CDN the synced paths point to. Revisit if the host blocks hotlinking.
