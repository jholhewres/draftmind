# ADR-0005: Push-to-talk voice in v1; Realtime later

Status: accepted · 2026-10-27

## Context

Mid-game the player's hands are busy, so voice matters. The OpenAI Realtime API gives the best experience but needs an OpenAI key, which is not set up; OpenRouter is the only provider key available.

## Decision

- **v1 is push-to-talk:** hold a button, the browser records with `MediaRecorder` (Opus/WebM), release to upload.
- **STT** on the server through an OpenRouter model that accepts audio input (`STT_MODEL`); the transcript enters the normal coach pipeline (ADR-0003).
- **TTS** in the browser with the Web Speech API (`speechSynthesis`) first: free, no key, no extra latency. A server TTS provider can replace it behind the same `Speaker` interface if voice quality is not good enough.
- STT and TTS sit behind small interfaces (`Transcriber`, `Speaker`) so a Realtime session can be added later without changing the coach.

## Consequences

- Round trip is record → upload → STT → coach → speak, a few seconds; fine between fights, not for split-second calls.
- Browser TTS voice quality varies by OS.
- **Later:** OpenAI Realtime behind a feature flag once an `OPENAI_API_KEY` exists, reusing the coach session and its tools.
