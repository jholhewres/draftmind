# ADR-0006: Read photos with Jev images, benchmarked against a vision model

Status: accepted · 2026-10-28 · Supersedes [ADR-0004](0004-vision-draft-read.md)

## Context

ADR-0004 assumed Jev could not read images. Jev (TypeSafe System One) now accepts images in the `state`, in a question's `instructions` and in its `criteria`, and a Box question returns where something is in an image. The feature is marked "Dreamer-ready" (beta): it is opt-in per request with the `TypeSafe-Danger-Zone-Images: true` header, and its format, limits and accuracy can change between releases.

The pick phase leaves the player 15–20 s, so a photo must turn into suggestions in about 5 s. Photos also appear later: the end-of-match screen (result, duration, KDA) and screenshots sent to the coach.

## Decision

- Photo reads go through one interface, `PhotoReader`, with two implementations:
  - **Jev:** a Box question finds the hero slots and Choice questions pick each slot's hero, with the hero portraits from the database as options. Only heroes of the current patch can be answered.
  - **Vision model** on OpenRouter (`VISION_MODEL`) with a JSON schema, as ADR-0004 described.
- A benchmark command (`make bench-photo`) scores both on real photos in `api/testdata/photos/`: slot accuracy, p50 and p95 latency, cost. `PHOTO_READER` picks the default from its result.
- The other reader is the fallback when the first fails or misses its deadline.
- The browser downscales to ~1600 px and strips EXIF. The server resolves every name or option against `hero` and flags what it cannot resolve. Images are not stored unless the player opts in to the benchmark set.

## Consequences

- One provider covers both reading and deciding, if Jev wins the benchmark: fewer moving parts and one audit log.
- Beta risk: a Jev release can change the image format or accuracy. The fallback reader and the benchmark in CI catch it.
- The flagged-slot UI from ADR-0004 stays: the read draft shows on the suggestions screen, and each slot is fixed with one tap.

## Benchmark result (2026-10-28)

`make bench-photo` on the 8 synthetic photos of `api/testdata/photos/` (no real photos yet), two runs:

| reader | slots right | wrong hero | flagged | p50 | p95 | cost / photo |
|---|---|---|---|---|---|---|
| Jev (`typesafe/jev-1.14`) | 91–96 % | 0 | 3–7 | 1.9–2.6 s | 3.5–4.3 s | $0.0012 |
| Vision (`google/gemini-3.1-flash-lite`) | 57.5 % | 32 | 2 | 2.8–3.2 s | 3.2–4.4 s | $0.0010 |

Photo to first suggestion through the API (read, then suggest): p50 2.3 s, p95 3.4–3.9 s.

- **Default: Jev** (`PHOTO_READER=jev`); the vision model is its fallback. Jev reads more slots right and never named a wrong hero; the vision model names a wrong hero with 0.95 confidence on 40 % of slots, so its confidence cannot gate a guess.
- How the Jev reader works: a Box question finds the clock between the teams; code walks outward from it, finds each portrait by correlating small normalized thumbnails of the current patch's portraits (cached under `DATA_DIR`, outside git), numbers each one by its distance from the clock and shortlists the 5 best-fitting heroes; one choice question per slot, in parallel, picks among their portraits, with `empty` and `other` options. A slot stands only when Jev's pick has probability ≥ 0.5 and its portrait also fits the slot by the code's measure (correlation ≥ 0.75); anything else is flagged.
- The top bar's shape (slot size and gap relative to the clock) is calibrated on the synthetic layout; recheck it with real photos.
- Jev photo calls go over HTTP/1.1, one connection each, without a hedge: over HTTP/2 the ten slot uploads queue on one connection and miss the 3 s deadline.
