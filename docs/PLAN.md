# Draftmind — Plan

Status: draft v1 · 2026-10-27 · Spec: [SPEC.md](SPEC.md) · Decisions: [adr/](adr/)

Small vertical slices, each shippable and testable on its own. The order takes the fastest path to **"photo of the draft → hero suggestions"** (S1–S3), then makes the decision real (S4–S5), then grows into guide, items, coach and voice.

S0 (scaffold: Go API, React web, Postgres, Makefile, docs) is done in the bootstrap session and has no card.

## Milestone A — Photo → suggestions

### S1. Hero catalog sync
- **Build:** `draftmind sync heroes` loads dotaconstants heroes and patch into `patch` and `hero` (slug, name, aliases, roles, attributes, image). `GET /api/heroes` lists them; the web shows a searchable hero grid.
- **Criteria:** sync is idempotent (second run changes 0 rows); a `sync_run` row records counts; `GET /api/heroes` returns every hero of the current patch; unit tests parse a fixture of dotaconstants JSON.
- **Verify:** `make test` and `make sync && curl -s localhost:8080/api/heroes | jq length`.

### S2. Counter-pick suggestions from a typed draft
- **Build:** `draftmind sync stats` pulls OpenDota `/heroStats` and `/heroes/{id}/matchups` into `hero_stat` and `hero_matchup` (rate limited, resumable). `POST /api/draft/suggest` takes allies, enemies, bans and role and returns the ranked legal heroes with numeric reasons. The web has a draft board where heroes are tapped into slots and shows the top 5.
- **Criteria:** rate limiter keeps ≤ 1 req/s without a key; a partial sync failure keeps saved heroes and marks the run failed; matchup win rates are shrunk by games played (tested); picked and banned heroes never appear; ranking is deterministic for a fixed dataset; each suggestion lists its counter delta per enemy; p95 under 100 ms on the full hero set.
- **Verify:** `make test`.

### S3. Draft read from a photo
- **Build:** `POST /api/draft/read` sends the image to the vision model (`VISION_MODEL`) and returns a structured draft resolved to hero ids; the web uploads a photo (camera on mobile), shows the read draft for one-tap fixes, then calls S2. A benchmark command scores models on `api/testdata/drafts/`.
- **Criteria:** unknown names are flagged, never guessed; image downscaled and EXIF stripped client side; no image stored; benchmark reports slot accuracy, p50 latency and cost per model; photo → first suggestion ≤ 8 s p50 on the fixture set.
- **Verify:** `make test` and `make bench-vision` (needs `OPENROUTER_API_KEY`).

## Milestone B — Jev decides, LLM writes

### S4. Jev picks among the suggestions
- **Build:** `api/internal/decision`: System One client (strict parse, pinned provider, deadline, retries, hedge) and `decision_log`. Suggestions send the top 8 candidates as a `choice` question; results reorder by Jev probabilities; fail open to the code ranking.
- **Criteria:** a missing or mistyped answer fails the call; timeout falls back to code ranking with a label; every call logged with model, latency and cost; tests against an `httptest` server.
- **Verify:** `make test`.

### S5. Streaming suggestions with written tips
- **Build:** suggestions stream over SSE (`ranking` → `decision` → `tip`); a writer model writes one short tip per hero from the structured reasons; an entity check rejects tips that name anything outside the result.
- **Criteria:** first `ranking` event before any model call returns; a tip naming an unknown hero or item is replaced by a template; UI updates in place.
- **Verify:** `make test`.

## Milestone C — Guide and items

### S6. Hero guide pages
- **Build:** sync abilities, items and `itemPopularity`; hero page with abilities, items by phase, top counters and victims.
- **Criteria:** every number on the page comes from the current patch tables; page works on a phone width.
- **Verify:** `make test`.

### S7. Synergy in suggestions and hero pages
- **Build:** decide the synergy source (STRATZ or self-aggregated from OpenDota public matches), sync `hero_synergy`, and add synergy to the S2 ranking and the hero page.
- **Criteria:** the source decision recorded as an ADR; ranking reasons include synergy with each ally; low-sample pairs weighted down.
- **Verify:** `make test`.

### S8. Item build for the locked hero
- **Build:** after the pick, code builds core and situational item candidates from item popularity and the enemy lineup; Jev picks situational items and urgency (`score`); the writer explains each in one line.
- **Criteria:** only items that exist in the patch; situational choices cite the enemy hero that triggers them.
- **Verify:** `make test`.

## Milestone D — Coach

### S9. Coach session and chat
- **Build:** `coach_session` with draft, hero, role, item plan and chat; each message is parsed into a typed situation, facts are fetched, Jev decides advice category and priority, the writer answers over SSE.
- **Criteria:** session survives a page reload; answers name only DB entities; situation parse failures ask the player a short follow-up instead of guessing.
- **Verify:** `make test`.

### S10. Push-to-talk voice
- **Build:** hold-to-record in the browser, `POST /api/coach/{id}/voice` → STT (`STT_MODEL`) → coach pipeline → reply spoken with `speechSynthesis`.
- **Criteria:** works on mobile Chrome and Safari; transcript shown with the answer; microphone permission denial handled.
- **Verify:** `make test` plus a manual check on a phone.

### S11. Custom coach persona
- **Build:** settings for tone, language, verbosity and focus, applied to the writer prompt only.
- **Criteria:** persona never changes the decided picks or items (test with two personas, same decision).
- **Verify:** `make test`.

## Milestone E — Run it for real

### S12. Patch-aware sync schedule
- **Build:** detect a new patch from dotaconstants, run the full sync, mark it current only when it succeeds; periodic stats refresh; the web footer shows the current patch and last sync time.
- **Criteria:** a failed sync keeps the previous patch current; status visible at `GET /api/sync/status` and in the footer.
- **Verify:** `make test`.

### S13. Accounts and deploy
- **Build:** simple login (magic link) so sessions and personas belong to a user; deploy to the personal server under `/var/www/<domain>/` on a random 127.0.0.1 port behind the proxy, with the trademark note in the footer.
- **Criteria:** health check green in production; `.env` only on the server, never in the repo.
- **Verify:** `curl -fsS https://<domain>/api/health`.

## Dependencies

```
S1 → S2 → S3 ─┐
      S2 → S4 ┴→ S5
S2 → S6 → S7 (→ S2 ranking)
S6 + S4 → S8
S5 + S8 → S9 → S10, S11, S13
S2 → S12
```
