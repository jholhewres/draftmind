# ADR-0001: Stack — Go API, React/Vite web, Postgres

Status: accepted · 2026-10-27

## Context

Draftmind needs a low-latency API that fans out to several model calls, a small sync job, and a mobile-friendly web UI. Jhol's other projects (usewoo, cs2mix-engine) run Go + Postgres, and the operational pattern is one binary on the personal server.

## Decision

- **API:** Go (module `github.com/jholhewres/draftmind/api`), standard library `net/http` routing, `pgx/v5` for Postgres, SQL migrations embedded in the binary and applied by a small runner (`draftmind migrate`). One binary with subcommands: `serve`, `migrate`, `sync`.
- **Web:** React + Vite + TypeScript, pnpm, Vitest, oxlint. Served by Vite in dev (proxying `/api`) and as static files behind the API in production.
- **Database:** Postgres 17, `docker-compose.yml` for local dev on `127.0.0.1:55433`.
- **Streaming:** Server-Sent Events for suggestions and coach answers (one-way, works through proxies, no extra dependency).
- **Tooling:** `Makefile` with `dev`, `test`, `lint`; `golangci-lint` v2, oxlint, `tsc --noEmit`.

## Consequences

- No ORM or framework: more SQL by hand, fewer moving parts.
- Single deployable plus Postgres; no queue or cache until a measured need.
- SSE means the client cannot push mid-stream; voice uploads use plain POST.
