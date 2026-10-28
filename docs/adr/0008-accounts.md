# ADR-0008: Accounts with a magic link or Steam sign-in

Status: accepted · 2026-10-28

## Context

Match history, results and the custom coach belong to a person and must survive a new phone. Players already have a Steam account. Passwords add reset flows and breach risk for little gain.

## Decision

- Two ways in: a **magic link** sent by e-mail, and **Steam OpenID 2.0** sign-in, which returns a SteamID and no password.
- Sessions are server-side rows (`user_session`) referenced by an HttpOnly, Secure, SameSite=Lax cookie. No JWT.
- Match history, results and persona settings reference `app_user`. A visitor can try the draft read without an account. Saving history asks them to sign in.
- E-mail goes out through one SMTP setting in `.env`. The links expire after 15 minutes and work once.

## Consequences

- No passwords to store or reset.
- Steam sign-in links the SteamID now, which opens later features (match import) without touching the game client.
- Outbound e-mail needs a sender domain and SPF/DKIM before production.
