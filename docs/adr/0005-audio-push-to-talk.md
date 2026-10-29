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

## Amendment · 2026-10-29 (B3)

- **STT model:** `google/gemini-3.1-flash-lite` by default (`STT_MODEL`). It takes audio input on OpenRouter with zero data retention, is already the vision model, and costs about $0.50 per million audio tokens (~32 tokens a second), so a 10 s clip is about $0.0002. Cheaper audio models on the list (Voxtral, Gemini 2.5 Flash Lite) were not worth a second provider for v1.
- **Clip format:** the browser converts what `MediaRecorder` records (WebM/Opus, Ogg or MP4, by browser) to 16 kHz mono 16-bit WAV before upload, so the server checks one format and every audio-input model accepts it. Clips are at most 30 s (~1 MB) and never stored; the transcript is stored as the player's message (kind `voice`).
- `POST /api/match/{id}/voice` takes the WAV body, transcribes it with the player's UI language as a hint, then runs the same coach turn as a typed message. It has its own rate limit (10 clips, then one per 15 s per client).
- The `Speaker` is the browser's `speechSynthesis`, speaking the answer in the UI language.
