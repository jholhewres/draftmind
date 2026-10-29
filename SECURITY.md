# Security policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub: open the repository's **Security** tab and choose **Report a vulnerability** (private vulnerability reporting). Please do not open a public issue or pull request for a security problem.

Include what is affected, how to reproduce it and the impact you see. You will get an answer as soon as the maintainer can look at it; a fix is released before the details are made public.

## Keys and secrets

- Never put an API key, password, session cookie or `.env` content in an issue, pull request, log excerpt or screenshot. Redact them before you paste anything.
- If you think a key was exposed, rotate it at its provider first, then report it.
- Secrets belong only in the local, gitignored `.env`.

## Scope

The code in this repository: the Go API in `api/` and the web app in `web/`. Problems in OpenRouter, OpenDota, the game's data feed or other third-party services go to their owners.
