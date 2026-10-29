<p align="center">
  <img src="docs/images/logo.svg" width="72" alt="Draftmind logo">
</p>

<h1 align="center">Draftmind</h1>

<p align="center">
  A draft guide and in-match coach for Dota 2, in the browser.<br>
  <a href="https://draftmind.jhol.dev">draftmind.jhol.dev</a>
</p>

<p align="center">
  <img src="docs/images/landing.jpg" width="240" alt="Landing page on a phone">
  <img src="docs/images/heroes.jpg" width="240" alt="Heroes tab on a phone">
  <img src="docs/images/hero.jpg" width="240" alt="Hero page on a phone">
</p>

<p align="center">
  <img src="docs/images/hero-desktop.jpg" width="820" alt="Hero page on desktop: counters, attributes and abilities">
</p>

Photograph the pick screen and Draftmind reads the draft, suggests heroes, then items, and keeps coaching until the match ends, by text or push-to-talk voice. It also has a guide page per hero and a patches tab.

> Draftmind is an independent fan tool, not affiliated with or endorsed by Valve Corporation. Dota 2 and its hero names, portraits and game data are trademarks and copyrighted material of Valve. Draftmind runs on a separate device or browser tab and never reads, modifies or overlays the game client: its only inputs are what the player types, says, photographs or, on a PC, the game window they choose to share; nothing touches the game client. See [NOTICE](NOTICE).

## Features

- **Draft read from a photo.** A phone photo or screenshot of the pick screen becomes both teams' picks, with a confidence per slot; any slot can be fixed in one tap. The reader uses TypeSafe's Jev image questions (a beta feature) and falls back to a vision model ([ADR-0006](docs/adr/0006-photo-read-with-jev.md)).
- **Live draft from a shared window.** On a PC, share the game window and each pick is read on its own: the browser checks the portraits every 2 s for free and sends one frame only when a slot changes. A Live bar shows the read count and cost, with a Stop button; no frame is stored ([ADR-0013](docs/adr/0013-live-screen-read.md)).
- **Counter suggestions.** Code ranks every legal hero by matchups, synergy, role fit and patch win rate; the Jev decision model chooses among the top candidates. The ranking shows first and is refined in place.
- **Streaming tips.** A short written reason per suggestion, streamed as it is written, built only from the decided result.
- **Item build** for the locked hero against the enemy lineup.
- **Coach chat and voice.** Tell the coach how the game goes by text, push-to-talk or a screenshot; a custom persona sets its tone, language and focus.
- **Match result and history.** End a match with a tap, a photo of the end screen or a message; it goes into the player's history.
- **Hero guide.** Stats, abilities and talents, counters and victims, item builds and the hero's history of changes, patch by patch.
- **Patches tab.** Every patch with its hero and item changes.
- **Accounts.** E-mail and password, Steam, or an e-mail magic link when SMTP is set. A visitor can try a draft read without an account.
- English and Brazilian Portuguese UI and game text.

## Architecture

**The LLM writes, Jev decides, the code executes** ([ADR-0003](docs/adr/0003-decision-pipeline.md)). Game data lives in Postgres, synced per patch from [dotaconstants](https://github.com/odota/dotaconstants), the game's public data feed and the [OpenDota API](https://docs.opendota.com/). Code builds and filters the candidates from that data; Jev chooses among them; a writer model only turns the decided result into text. When a model call fails, the player still gets the code ranking. No hero, item or ability reaches the UI unless it is in the database for the current patch.

- `api/`: Go, `net/http`, pgx, embedded SQL migrations. One binary: `serve`, `migrate`, `sync`, `bench`.
- `web/`: React, Vite, TypeScript, Vitest, oxlint, Playwright.
- Postgres 17 via `docker-compose.yml`.
- Models through [OpenRouter](https://openrouter.ai/): Jev (System One) for decisions and photo reads, a vision model, a writer model and a speech-to-text model.

More in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and the ADRs in [docs/adr/](docs/adr/).

## Quick start

Requirements: Go 1.27+, Node 22+, pnpm 10, Docker (for Postgres), golangci-lint v2, and an OpenRouter API key for the model features.

```sh
cp .env.example .env   # then set OPENROUTER_API_KEY
make setup             # pnpm install and go mod download
make sync              # first data load, several minutes
make dev               # Postgres on 127.0.0.1:55433, API on :8080, web on :5173
```

Open http://localhost:5173. Without `OPENROUTER_API_KEY` the app still runs: suggestions keep the code ranking, tips and coach answers are templates, photos cannot be read and voice is off. Without SMTP, sign-in links are written to the API log in development.

## Configuration

Every setting comes from the environment; `make` loads `.env` (gitignored). See [.env.example](.env.example).

| Variable | Required | What it does |
|---|---|---|
| `DATABASE_URL` | yes | Postgres connection string. The compose database is `postgres://draftmind:draftmind@127.0.0.1:55433/draftmind?sslmode=disable`. |
| `HTTP_ADDR` | no | Address the API listens on. Default `127.0.0.1:8080`. |
| `PUBLIC_URL` | in production | Where the web app is served; sign-in links and Steam return here. A localhost URL is development (plain HTTP cookies); anything else must be `https`. Default `http://localhost:5173`. |
| `WEB_DIR` | in production | The web build (`web/dist`) served at `/` with the API. Empty serves the API alone. |
| `TRUSTED_PROXY` | behind a proxy | Comma-separated proxy IPs whose `X-Real-IP` / `X-Forwarded-For` is trusted for rate limits and logs. Empty trusts no header. |
| `OPENROUTER_API_KEY` | for model features | Key for every model call (Jev, vision, writer, speech-to-text). |
| `OPENROUTER_BASE_URL` | no | Default `https://openrouter.ai/api`. |
| `DECISION_MODEL` | no | Jev model for decisions. Default `typesafe/jev-1.13`. |
| `DECISION_PROVIDER` | no | OpenRouter provider for Jev. Default `typesafe`. |
| `PHOTO_READER` | no | Default photo reader, `jev` or `vision`; the other is the fallback. Default `jev`. |
| `PHOTO_JEV_MODEL` | no | Jev model for photo reads (image questions need 1.14). Default `typesafe/jev-1.14`. |
| `VISION_MODEL` | no | Vision model with JSON-schema output, for the fallback reader and the end screen. Default `google/gemini-3.1-flash-lite`. |
| `DATA_DIR` | no | Local data outside git (the hero portrait cache). Default `data`. |
| `WRITER_MODEL` | no | Text model that writes the tips and coach answers. Default `openai/gpt-6-luna`. |
| `STT_MODEL` | no | Audio-input model that transcribes push-to-talk clips. Default `google/gemini-3.1-flash-lite`. |
| `OPENDOTA_API_KEY` | no | Raises the OpenDota rate limit (1 request a second without it). |
| `DOTACONSTANTS_REF` | no | dotaconstants branch, tag or commit to sync from. Default `master`. |
| `SYNC_SCHEDULE` | no | `on`: `serve` checks for a new patch every hour, syncs it from every source and makes it current only when all succeeded, and refreshes stats daily. Default `off`. |
| `SMTP_HOST` | for e-mail links | SMTP server for sign-in links (STARTTLS when offered). Without it, development logs the link and production turns e-mail links off. |
| `SMTP_PORT` | no | Default `587`. |
| `SMTP_USER`, `SMTP_PASS` | if the server needs them | SMTP credentials. |
| `MAIL_FROM` | with `SMTP_HOST` | Sender, e.g. `Draftmind <signin@example.com>`. |

Tests also read `TEST_DATABASE_URL` (default: the compose database).

## Data sync

`make sync` runs, in order:

| Command | Source | Loads |
|---|---|---|
| `draftmind sync heroes` | dotaconstants | patches, heroes, items |
| `draftmind sync details` | the game's data feed | each hero's stats, abilities and talents in en and pt-BR (about 2 requests a second) |
| `draftmind sync patchnotes [N]` | the game's data feed | patch notes in en and pt-BR |
| `draftmind sync stats` | OpenDota | hero stats and matchups |
| `draftmind sync builds` | OpenDota | item popularity by game phase |

Each run is recorded in `sync_run`. With `SYNC_SCHEDULE=on` the server keeps the data current by itself; `GET /api/sync/status` reports the current patch, each source's last sync and the newest failure.

## Tests

```sh
make db-up             # database tests skip when Postgres is down
make test lint         # Go tests, Vitest, go vet, golangci-lint, oxlint and tsc
make e2e               # Playwright visual tests at phone and desktop sizes, mocked API
make bench-photo       # live: reads every photo in api/testdata/photos with both readers
```

`make bench-photo` needs `OPENROUTER_API_KEY` and makes paid model calls; unit tests never call a live API. The e2e baselines are Linux screenshots; see [CONTRIBUTING.md](CONTRIBUTING.md).

## Deploy

One static binary serving the API and the web build, behind nginx. See [deploy/README.md](deploy/README.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md). Coding agents read [AGENTS.md](AGENTS.md).

## License

[MIT](LICENSE). Third-party material, data sources and fonts are listed in [NOTICE](NOTICE).
