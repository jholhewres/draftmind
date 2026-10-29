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
