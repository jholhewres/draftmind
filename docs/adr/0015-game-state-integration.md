# ADR-0015: Exact match state from Game State Integration

Status: accepted · 2026-10-29 · posting to the HTTPS server is unverified until the owner plays one match

## Context

The live coach (ADR-0014) reads the HUD from a shared window, which costs a Jev call per look and guesses gold and items. The game has an official feed, Game State Integration (GSI): with a config file in its `cfg/gamestate_integration/` folder and the `-gamestateintegration` launch option, it posts the player's own state as JSON to a URI: clock, game state, gold, items, level, KDA, hero alive and buyback, and the winner at the end. It is exact, free and only ever the player's own data.

## Decision

- **Endpoint.** `POST /api/gsi` takes the game's JSON. The account is the token in `auth.token`, stored as its SHA-256 in `gsi_token` (migration 0021), never as sent. Bodies over 64 KB are refused (413), unknown tokens 401, and posts are rate limited per client IP. The game ignores answers, so every refusal is only a status.
- **Settings ("Game integration").** A signed-in player makes the file `gamestate_integration_draftmind.cfg`, with their token and the server's `PUBLIC_URL` + `/api/gsi`, shown once with the install steps (copy to the cfg folder, add `-gamestateintegration`, start a match). Making a new file replaces the token; Revoke deletes it and the stored state. The screen shows "Receiving data" while the last post is under a minute old. Visitors and a server without it see nothing: everything works as before.
- **Code triggers first, no model call.** Each post is compared with the last one (`gsi.Triggers`): gold reaches the cost of the next item in the session's plan that the player does not own; a death within 3 minutes of the last; dead with buyback ready and affordable; the laning stage ends at 10:00. Turbo halves the clock thresholds (ADR-0012). A trigger speaks through the live coach's cooldown and cap as a template line with the item card (ADR-0014), with no Jev or writer call. The match it applies to is the player's latest open session from the last six hours.
- **Live tick on facts.** With a post under a minute old, the 30 s live tick sends Jev the game's state as `game` facts and no image: exact gold, items and clock instead of reading the HUD.
- **Result by itself.** On `POST_GAME` with a winner, the result is recorded with source `gsi`: win or loss from the player's team, kills, deaths, assists and the clock as duration. The Coach tab, polling while the game posts, closes the match.
- **Game mode.** When the post names a mode (`map.game_mode`), it wins over the one picked at the draft for the triggers.

## Pending

- **HTTPS posting is unverified.** The game's GSI is documented and widely used with `http://localhost` receivers; whether it posts directly to a public `https://` URI could not be checked without the game. The owner installs the file after deploy and plays one match: the settings screen turns to "Receiving data" if it works. If the game will not post to HTTPS, the fallback is a small local bridge (localhost receiver forwarding to the server), to be documented then.
- The fixtures in `api/internal/gsi/testdata` follow the documented payload shape; replace them with recorded posts after that first match.
