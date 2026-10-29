# Deploy

Draftmind runs in production as one static Go binary behind nginx, with
PostgreSQL on the same host. The deploy scripts live outside this repository
and run from the operator's machine over SSH; nothing here holds a secret or
a host address.

## Layout on the server

```
/var/www/draftmind.jhol.dev/
  releases/<version>/draftmind   # static linux/amd64 binary (-X main.version)
  releases/<version>/web/        # web/dist from pnpm build
  current -> releases/<version>  # switched atomically on release
  .env                           # mode 600, owned by the service user; never in git
/etc/systemd/system/draftmind.service
/etc/nginx/sites-available/draftmind.jhol.dev (+ sites-enabled link)
```

- **Service:** `draftmind.service` runs `current/draftmind serve` as the
  `draftmind` system user (no login shell), with `EnvironmentFile=.env`, and
  listens on a loopback port only.
- **Proxy:** nginx terminates TLS (certbot) for `draftmind.jhol.dev` and
  proxies everything to that port: `client_max_body_size 6m` for photos,
  `proxy_buffering off` for the draft and coach streams (SSE).
- **Web:** `WEB_DIR=<app>/current/web`. The binary serves the build at `/`,
  falls back to `index.html` for the app's routes, caches `assets/` for a year
  and leaves `/api` to the API.
- **Client address:** `TRUSTED_PROXY=127.0.0.1` so rate limits key on nginx's
  `X-Real-IP`, not on the proxy.
- **Cookies:** `PUBLIC_URL=https://draftmind.jhol.dev`, so session cookies are
  `Secure` (ADR-0008).
- **Data:** a `draftmind` database and role on the local PostgreSQL (16 or
  newer). `SYNC_SCHEDULE=on` keeps the game data current after the first sync.

## Release

1. Build: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=<v>"`
   and `pnpm build`.
2. Upload both to a new `releases/<version>/`.
3. `draftmind migrate`, switch `current`, restart the unit.
4. `curl -fsS https://draftmind.jhol.dev/api/health`; on failure `current`
   goes back to the previous release and the unit restarts.

First run only: `draftmind sync heroes`, then `details`, `stats`, `builds`
and `patchnotes`.
