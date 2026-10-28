# AGENTS.md — Draftmind

Rules for coding agents working in this repository. Read [docs/SPEC.md](docs/SPEC.md), [docs/PLAN.md](docs/PLAN.md) and the ADRs in [docs/adr/](docs/adr/) before changing behaviour.

## Layout

- `api/` — Go module `github.com/jholhewres/draftmind/api`. Entry point `cmd/draftmind` (`serve`, `migrate`, `sync heroes`). Packages under `internal/`.
- `api/internal/db/migrations/` — numbered SQL files, applied once in name order. Never edit an applied migration; add a new one.
- `web/` — React + Vite + TypeScript. `/api` is proxied to the Go server in dev.
- `docs/` — spec, plan, ADRs.

## Commands

- `make dev`, `make sync`, `make test`, `make lint`, `make build`. Run `make test lint` before calling work done.
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
- Do not commit or push without the owner's explicit ok. No co-author or attribution trailers.
