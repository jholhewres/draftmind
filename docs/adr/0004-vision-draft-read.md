# ADR-0004: Read the draft from a photo with a vision model

Status: superseded by [ADR-0006](0006-photo-read-with-jev.md) · 2026-10-28

## Context

Jev does not read images. The player photographs the pick screen with a phone (glare, angle, moiré), so the image is noisy. The top bar shows hero portraits without names. Options:

- **Vision LLM** on OpenRouter with structured JSON output: handles noisy photos and layout changes, costs a few cents per image, 2–4 s.
- **Template matching** of portraits against hero images: fast and nearly free, but breaks with photo angle and each UI update.
- **Game State Integration**: official, but does not expose the enemy draft to a player, and runs on the game PC.

## Decision

- Use a **vision model via OpenRouter**, configured by `VISION_MODEL`, with a JSON schema: `radiant[]`, `dire[]`, `bans[]`, `player_side`, each slot with `name` and `confidence`.
- The browser downscales to ~1600 px and strips EXIF before upload.
- The server resolves names against `hero` (exact, alias, fuzzy) and never trusts an unknown name; unresolved slots are flagged for the player.
- The player confirms or fixes the draft with one tap per slot; corrections are stored in `draft_read` to build a benchmark set.
- The model is chosen by a benchmark on real photos (`api/testdata/drafts/`, with expected drafts): accuracy per slot, p50 latency and cost.
- Images are not stored unless the player opts in to help the benchmark.

## Consequences

- Accuracy depends on the model recognising small portraits; the confirm step is mandatory, not optional polish.
- Template matching stays a fallback to evaluate if the benchmark misses the 90 % slot target or the latency budget.
