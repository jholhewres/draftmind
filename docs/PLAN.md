# Draftmind — Plan

Status: draft v2 · 2026-10-28 · Spec: [SPEC.md](SPEC.md) · Decisions: [adr/](adr/)

Small vertical slices, each shippable and testable on its own. The order takes the fastest path to **"photo of the pick → suggestions in ~5 s"** (milestone A). Then come the match (B), the guide (C) and running it for real (D). Each slice is a card on the devpit board with its criteria, files and verify command; this file is the overview.

Done: S0 (scaffold) and S1 (hero catalog sync: `draftmind sync heroes`, `GET /api/heroes`, searchable hero grid).

## Foundation

- **P0. UI text comes from en and pt-BR catalogs.** Message catalogs and a `t()` helper; the browser language picks the catalog, English is the fallback; App and the hero grid move over.
- **P1. App shell with the game look and four tabs.** The approved prototype's dark theme, team colors and gold accent; tab bar for Draft, Coach, Heroes and Patches; hero portraits and an attribute filter in the Heroes tab; the trademark note on every main tab.

## Milestone A — Pick: photo → suggestions

- **A1. Hero list shows only the heroes of the current patch.** `hero_patch` membership; `sync_run` counts every write and records the dotaconstants commit it read. Fixes the S1 review findings.
- **A2. Hero page shows stats and abilities from the game's data feed.** Sync hero details in English and Portuguese (ADR-0007); `GET /api/heroes/{slug}`; hero page with banner, stats and abilities.
- **A3. Counter-pick suggestions from a typed draft.** OpenDota matchups (rate limited, resumable, shrunk win rates); `POST /api/draft/suggest` with counter deltas per enemy; Draft tab where heroes are tapped into slots.
- **A5. Jev picks among the suggestions.** System One client (strict parse, pinned provider, deadline, retries, hedge) and `decision_log`; fail open to the code ranking.
- **A4. Draft read from a photo on the suggestions screen.** `PhotoReader` with Jev images and a vision model, chosen by `make bench-photo` (ADR-0006); read draft and ranking on one screen; flagged slots fixed with one tap; ≤ 5 s p50.
- **A6. Streaming suggestions with written tips.** SSE `read` → `ranking` → `decision` → `tip`; entity check on every tip.

## Milestone B — Match

- **B1. Item build for the locked hero.** Item popularity sync (`draftmind sync builds`); core items by phase; situational candidates are mid and late items, not in the core, that this hero or the heroes beating an enemy buy more than heroes overall; Jev picks them with an urgency and the enemy each one answers; the popular build on the hero page.
- **B2. Coach chat during the match.** Match session with draft, hero, role and item plan; typed situation → facts → Jev → writer over SSE.
- **B3. Push-to-talk voice.** Hold to record → STT → coach → `speechSynthesis` (ADR-0005).
- **B4. Match result by tap, photo or message.** End-screen photo fills result, duration and KDA to confirm; coach notes; match history.
- **B5. Custom coach persona.** Tone, language, verbosity and focus, applied to the writer prompt only.

## Milestone C — Guide

- **C1. Patch notes stored as each hero's history.** Every patch's notes from the data feed in `patch_note`; Patches tab; hero history on the hero page.
- **C2. Counters on the hero page.** Top counters and victims from the current patch's matchups.
- **C3. Game guides on the hero page.** Merged into B1: the hero page's popular build by phase comes from OpenDota item popularity, labelled as such.
- **C4. Synergy in suggestions and hero pages.** Decide the synergy source in an ADR; `hero_synergy`; synergy reasons in the ranking.

## Milestone D — Run it for real

- **D1. Sign in with an e-mail link or Steam.** Magic link and Steam OpenID; server-side sessions (ADR-0008).
- **D2. Landing page with three sections.** What it does, how it works, always on the current patch; sign-in call to action; trademark note.
- **D3. Patch-aware sync schedule.** Detect a new patch, run the full sync, mark it current only on success; `GET /api/sync/status`; the footer shows the patch and last sync.
- **D4. Deploy to the personal server.** Under `/var/www/<domain>/` on a random 127.0.0.1 port behind the proxy; `.env` only on the server.

## Dependencies

```
S1 → P0 → P1
S1 → A1 → A2 (+P1) → C1 → D3
     A1 + P1 → A3 → A5 → A4 → A6 → B2
     A2 + A5 → B1 → B2 → B3
     B2 + D1 → B4, B5
     A2 + A3 → C2 → C3 (+B1), C4
     P1 → D1, D2 → D4 (+D1)
     A3 + C1 → D3
```
