# AGENTS.md — Draftmind

Rules for coding agents working in this repository. Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and the ADRs in [docs/adr/](docs/adr/) before changing behaviour.

## Layout

- `api/` — Go module `github.com/jholhewres/draftmind/api`. Entry point `cmd/draftmind` (`serve`, `migrate`, `sync heroes`, `sync details`, `sync patchnotes`, `sync stats`, `sync builds`). Packages under `internal/`.
- `sync heroes` loads patches and heroes from dotaconstants and is fast; `sync details` then loads each current-patch hero's stats, abilities and talents from the game's data feed (ADR-0007), rate limited to about 2 requests a second, one hero per transaction. `sync patchnotes [N]` loads the notes of every patch the feed lists (the last N with a number) in en and pt-BR, one patch per transaction, fetching again only the newest two patches and those not yet stored. `sync stats` loads OpenDota hero stats and matchups for the current patch at 1 request a second without a key, one hero per transaction, stopping at the first failure (ADR-0009). `sync builds` loads each current-patch hero's OpenDota item popularity by phase the same way. Run `sync heroes` first; `make sync` runs all five.
- `api/internal/db/migrations/` — numbered SQL files, applied once in name order. Never edit an applied migration; add a new one.
- `web/` — React + Vite + TypeScript. `/api` is proxied to the Go server in dev.
- `docs/` — ARCHITECTURE.md and ADRs.
- `scripts/fixtures/` — Python generators for the synthetic test photos (`api/testdata/photos/README.md`).

## Commands

- `make dev`, `make sync`, `make test`, `make lint`, `make build`. Run `make test lint` before calling work done.
- `make e2e` runs the visual tests (Playwright, phone 390x844 and desktop 1280x800) against a mocked API; `make e2e-update` rewrites the baselines after an intended UI change. Browser specs live in `web/e2e/*.spec.ts`; baselines are Linux-only.
- Database tests use a throwaway database per test on the compose Postgres (`TEST_DATABASE_URL` overrides it) and skip when it is down: run `make db-up` first.

## Product rules

- **The database is the source of truth.** Heroes, items, abilities and matchups come from the synced tables. Never hardcode game facts and never let model output introduce an entity that is not in the database for the current patch.
- **The LLM writes, Jev decides, the code executes** (ADR-0003). Code builds and filters candidates; Jev chooses among them; the writer model only produces text from the decided result. Fail open to the code ranking.
- **Trademark:** never use "Dota" in the product name, logo or domain. Keep the Valve trademark and "does not touch the game client" note in the UI footer.
- **No locale assumptions** in logic; UI strings must be translatable.

## Engineering rules

- Secrets only in the local `.env` (gitignored). Never log or commit `OPENROUTER_API_KEY` or any other key.
- Prefer the standard library; add a dependency only with a clear reason.
- Short, objective code comments. Code, docs and commit messages in English.
- Tests next to the code (`*_test.go`, `*.test.tsx`); external HTTP calls tested against `httptest` servers or fixtures, never the live API in unit tests.
- Do not commit or push without the maintainer's explicit ok. No co-author or attribution trailers.
