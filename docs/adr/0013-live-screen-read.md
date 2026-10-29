# ADR-0013: Draft read live from a shared game window

Status: accepted · 2026-10-29

## Context

On a PC the player often has the site open beside the game or on a second monitor. Photographing the pick screen with a phone is the slow part of the draft read. Browsers can share one window with a page through `getDisplayMedia`. Until now the README and footer promised that Draftmind's only inputs are what the player types, says or photographs; reading a shared window changes that promise, so it needs a decision.

## Decision

- **Input.** "Live from screen" is offered next to the photo and the typed board, only on desktop browsers with `getDisplayMedia` (a coarse pointer hides it). The share asks for a window (`displaySurface: "window"`, the browser's own tab and whole monitors excluded); a whole screen or a tab is refused and stopped at once.
- **Free change detection in the browser.** Every ~2 s the page samples the shared window at 160×90 and compares the mean colour of the ten portrait slots of the top bar (`web/src/draft/frameDiff.ts`), leaving the clock out. Only a frame where a slot changed, and held for two samples, is sent: one JPEG at 1600 px on its longest side, the same as a photo. An unchanged window costs nothing.
- **Same pipeline.** A sent frame goes through the existing photo stream: read (Jev reader, ADR-0006) → code ranking → Jev decision → tips, over the same SSE. The draft and the suggestions update in place; an unresolved slot is flagged for one tap. A frame read while the player has edited the board does not overwrite the edit until the next frame. Mode rules apply as for photos (ADR-0012).
- **Visible and stoppable.** A "Live" bar with the shared window's name, the session's read count and cost (the read's reported USD cost) and a Stop button stays on top of the screen the whole time. Stopping, or ending the share from the browser, stops every track.
- **Privacy.** No frame is stored, on the server or in the browser: each lives in memory until read and is dropped, as photos are. The server logs the read's reader, latency and cost, never the image.
- **Fair play.** Draftmind only looks at what the player already sees in their own window. It never reads the game's memory, injects into the client, clicks or draws over the game; it gives advice, never automation. The README and the footer now say the inputs are what the player types, says, photographs or shares, and that nothing touches the game client.

## Consequences

- A read costs the same as a photo read (about $0.002); a typical draft sends 6 to 10 frames.
- The detector is tuned for the All Pick top bar. On other layouts it still sends on any change and the reader flags what it cannot place.
- The live coach during the match reuses the share and adds its own triggers (its own ADR).

## Fair-play and Valve policy

Reviewed 2026-10-29 against Valve's public statements; only what the sources say is recorded here.

What the sources say:

- **Steam Subscriber Agreement, section on Cheating (revision dated 2026-09-10).** Defines "Cheats" as software or hardware that may give a player an unfair competitive advantage in multiplayer games, forbids creating or using them, forbids interfering with anti-cheat software, and forbids tampering with the execution of Steam or its games, or using unauthorized third-party software to interact with or control their processes or user interface, unless Valve authorizes it. <https://store.steampowered.com/subscriber_agreement/>
- **"Cheaters Will Never Be Welcome in Dota", Dota 2 announcement, 2023-02-21.** Valve banned over 40,000 accounts whose software read information inside the Dota client that was not visible during normal play, and stated: "If you are running any application that reads data from the Dota client as you're playing games, your account can be permanently banned". <https://store.steampowered.com/news/app/570/view/5059268099256054054>
- **"Gameplay Update", Dota 2 announcement, 2023-02-28.** Under "Disabled some functionality used by third-party tools": the `record` command and many client-introspection console commands no longer work in matchmaking, and player profiles cannot be opened during the pregame phase, only after picking ends. <https://store.steampowered.com/news/app/570/view/5059268543373304374>
- **"Gameplay Patch 7.35d And Matchmaking Features", 2024-03-21.** Valve describes "visible actions against users of third-party cheats", says many cheats targeted the pre-game hero ban phase, and replaced that phase with account-stored ban preferences. <https://store.steampowered.com/news/app/570/view/5686430301719384803>
- **Game State Integration.** Valve's GSI is the game's own documented way to post the local player's state to a configured URI from a `cfg/gamestate_integration/` file (ADR-0015). Valve's developer wiki page for it could not be read on this date (it sits behind a bot check), so no wording from it is quoted here.
- No Valve statement found names Overwolf, overlays or screen-reading tools specifically, or authorizes or forbids a tool that only looks at a window the player shares. That question stays open; the press coverage of the 2023 changes is secondary and not used as policy.

What Draftmind does and does not do, measured against those sources:

- It does **not** read the game's memory or files, inject into or hook the client, run console commands, modify the game, draw over it, or send any input to it. Nothing runs on the player's machine besides the browser tab.
- It does **not** read anything the player cannot see: the live read sees only pixels of the window the player chose to share; GSI gives only the local player's own state, the data the game itself chooses to post. Pre-game player profiles and other players' hidden data are never read.
- It does **not** act for the player. It suggests picks, items and timing as text and speech; the player decides and plays.
- It **does** read the screen the player shares and the GSI posts the player configures, both opt-in, visible ("Live" bar, settings status) and stoppable at any time.

Residual risk: Valve's rule targets reading client data; a shared window is what the player already sees, but Valve has not said whether it considers draft or in-match advice from such a tool acceptable. If Valve publishes a rule against it, the live read and live coach are switched off (`LIVE_COACH=off`, and the "Live from screen" entry removed) and the typed and photo flows remain.
