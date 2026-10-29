# ADR-0010: Sign-in with an e-mail and a password

Status: accepted · 2026-10-29 · amends ADR-0008

## Context

ADR-0008 chose a magic link and Steam, with no passwords. Production has no SMTP yet, so the magic link cannot reach anyone, and Steam alone leaves out players who do not want to link it.

## Decision

- A third way in: **e-mail and password**. `POST /api/auth/register` creates the account, `POST /api/auth/login` opens the same server-side session as the other two.
- The password is kept only as an **argon2id** hash in PHC form (`$argon2id$v=19$m=19456,t=2,p=1$salt$key`, OWASP's minimum), in `app_user.password_hash`. The parameters travel with the hash, so raising them later still checks old hashes. 8 to 200 characters.
- Register answers the same whether the address is new or taken, and never sets a password on an existing account: without e-mail there is no proof the address is yours. An account from a magic link or Steam has no password and cannot gain one this way.
- Login answers one generic error for a wrong password, an unknown address or an account without a password, and always runs one hash compare (against a dummy hash when there is no account) so the timing matches.
- Both routes are rate limited per client IP and per address, in memory, like the link route. At most 4 hashes run at once.
- `GET /api/auth/methods` says which ways in are on. The web app shows the magic link only when the server has a mailer (SMTP, or the log in development).

## Consequences

- Passwords to store: a breach of the database exposes slow hashes, not passwords.
- No reset flow until SMTP exists. A forgotten password means Steam, or a link once SMTP is set.
- Register then login tells a caller whether its own password worked, so it can learn an address was taken. Without e-mail verification this cannot be closed; the rate limits slow it.
