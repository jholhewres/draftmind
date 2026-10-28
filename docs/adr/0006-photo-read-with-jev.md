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
