# Draftmind

An AI-powered draft guide and in-match coach. Photograph the pick screen and Draftmind suggests heroes, then items, then keeps coaching until the end of the match, by text or push-to-talk voice. It also has a guide page per hero: abilities, items, counters and synergies.

Game data (heroes, items, matchups) lives in Postgres, synced per patch from [dotaconstants](https://github.com/odota/dotaconstants) and the [OpenDota API](https://docs.opendota.com/). Choices are made by the Jev decision model among candidates the code builds from that data; an LLM only reads input and writes the final text. See [docs/SPEC.md](docs/SPEC.md), [docs/PLAN.md](docs/PLAN.md) and [docs/adr/](docs/adr/).

> Draftmind is an independent tool, not affiliated with or endorsed by Valve Corporation. Dota 2 is a trademark of Valve. Draftmind runs on a separate device or browser tab and never reads, modifies or overlays the game client.

## Stack

- `api/`: Go, `net/http`, pgx, embedded SQL migrations
- `web/`: React, Vite, TypeScript, Vitest, oxlint
- Postgres 17 via `docker-compose.yml`
- OpenRouter for the vision model, the Jev decision model (System One) and the writer model

## Getting started

Requirements: Go 1.27+, Node 22+, pnpm, Docker, golangci-lint v2.

```sh
cp .env.example .env   # then set OPENROUTER_API_KEY
make setup
make sync              # load heroes and patches, hero details, patch notes, then OpenDota stats and matchups (several minutes)
make dev               # Postgres on 127.0.0.1:55433, API on :8080, web on :5173
```

## Make targets

| Target | What it does |
|---|---|
| `make dev` | Starts Postgres, applies migrations, runs the API and the web dev server |
| `make test` | Go tests and Vitest |
| `make lint` | `go vet`, golangci-lint, oxlint and `tsc` |
| `make migrate` | Applies pending migrations |
| `make sync` | Syncs heroes and patches from dotaconstants (`draftmind sync heroes`), then each current-patch hero's stats, abilities and talents in en and pt-BR from the game's data feed (`draftmind sync details`, about 2 requests a second), then every patch's notes in en and pt-BR from the same feed (`draftmind sync patchnotes`), then OpenDota hero stats and matchups (`draftmind sync stats`, 1 request a second without `OPENDOTA_API_KEY`), then each hero's item popularity by game phase from OpenDota (`draftmind sync builds`, same rate) |
| `make build` | Builds `api/bin/draftmind` and `web/dist` |
| `make db-up` / `make db-down` | Starts or stops Postgres |

## Configuration

All settings come from the environment (see [.env.example](.env.example)). `.env` is gitignored; secrets never go into the repository.
