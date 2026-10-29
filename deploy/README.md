# Deploy

Draftmind runs in production as one static Go binary that serves both the API
and the web build, kept alive by [pm2](https://pm2.keymetrics.io/) behind
nginx, with PostgreSQL on the same host. This page describes one working
layout; adapt the paths and names to your host. `draftmind.example.com`
stands for your domain throughout.

## Layout on the server

```
/var/www/<domain>/
  releases/<version>/draftmind   # static linux/amd64 binary (-X main.version)
  releases/<version>/web/        # web/dist from pnpm build
  current -> releases/<version>  # switched atomically on release
  .env                           # mode 600, owned by the app user; never in git
  ecosystem.config.js            # pm2 app definition, reads .env
/etc/nginx/sites-available/<domain> (+ sites-enabled link)
```

- **Process:** pm2 runs `current/draftmind serve` as an unprivileged user,
  with the environment read from `.env`, and restarts it on exit. Run
  `pm2 startup` and `pm2 save` once so it comes back after a reboot.
- **Port:** `HTTP_ADDR=127.0.0.1:<port>`, a random free loopback port, so
  only nginx can reach the binary.
- **Proxy:** nginx terminates TLS (certbot) for the domain and proxies
  everything to that port: `client_max_body_size 6m` for photos,
  `proxy_buffering off` for the draft and coach streams (SSE).
- **Web:** `WEB_DIR=/var/www/<domain>/current/web`. The binary serves the
  build at `/`, falls back to `index.html` for the app's routes, caches
  `assets/` for a year and leaves `/api` to the API.
- **Client address:** `TRUSTED_PROXY=127.0.0.1` so rate limits key on nginx's
  `X-Real-IP`, not on the proxy.
- **Cookies:** `PUBLIC_URL=https://<domain>`, so session cookies are `Secure`
  (ADR-0008).
- **Data:** a `draftmind` database and role on the local PostgreSQL (16 or
  newer). `SYNC_SCHEDULE=on` keeps the game data current after the first sync.

## pm2

A minimal `ecosystem.config.js` next to `.env` (Node 20.12 or newer for
`util.parseEnv`):

```js
const fs = require('node:fs')
const path = require('node:path')
const { parseEnv } = require('node:util')

const root = __dirname
const env = parseEnv(fs.readFileSync(path.join(root, '.env'), 'utf8'))

module.exports = {
  apps: [{
    name: 'draftmind',
    cwd: root,
    script: path.join(root, 'current/draftmind'),
    args: 'serve',
    interpreter: 'none',
    env,
    max_restarts: 10,
    restart_delay: 2000,
  }],
}
```

`pm2 start ecosystem.config.js` the first time, `pm2 reload draftmind` (or
`pm2 restart draftmind --update-env` after changing `.env`) afterwards.

## nginx

```nginx
server {
    listen 443 ssl;
    server_name draftmind.example.com;
    # ssl_certificate lines managed by certbot

    client_max_body_size 6m;

    location / {
        proxy_pass http://127.0.0.1:<port>;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_read_timeout 120s;
    }
}
```

`certbot --nginx -d draftmind.example.com` issues the certificate and adds
the HTTP-to-HTTPS redirect.

## Release

1. Build: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=<v>" -o draftmind ./cmd/draftmind`
   in `api/`, and `pnpm build` in `web/`.
2. Upload both to a new `releases/<version>/`.
3. Run `draftmind migrate` with the `.env` loaded, switch `current`, then
   `pm2 reload draftmind`.
4. `curl -fsS https://<domain>/api/health`; on failure point `current` back
   at the previous release and reload again.

First run only: `draftmind sync heroes`, then `details`, `patchnotes`,
`stats` and `builds`.

Keep release scripts, host addresses and credentials outside this
repository.
