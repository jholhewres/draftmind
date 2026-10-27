# ADR-0003: The LLM writes, Jev decides, the code executes

Status: accepted · 2026-10-27

## Context

A chat model asked "what should I pick?" answers fluently and often wrongly: it names heroes that are already picked, quotes old patches, or invents items. Jhol's projects use a split where a calibrated decision model makes the choices and code owns every side effect.

## Decision

Every suggestion goes through four steps:

1. **Parse (LLM, optional).** Free input (chat text, transcript) becomes a typed situation. Photos go through the vision step instead (ADR-0004).
2. **Candidates (code).** Code queries the database and ranks legal options with explicit, numeric reasons (counter delta per enemy, synergy, role fit, patch win rate). Illegal options (picked, banned, wrong patch) never leave this step.
3. **Decide (Jev).** The top N candidates go to Jev (`typesafe/jev-*` on OpenRouter's System One endpoint, model pinned by `DECISION_MODEL`) as a `choice` question whose criteria are the candidates with their reasons, plus `score` questions where a level is needed (e.g. how urgent an item is). Jev returns the choice, probabilities and confidence.
4. **Write (LLM).** A writer model turns the decided, structured result into a short tip in the player's language and coach persona. Code checks the text names only entities from the result; a failing text is replaced by a template.

Client rules (our own implementation in `api/internal/decision`, not a copy of usewoo's):

- Strict parsing: every question asked must come back with the same type and a value, else the whole call fails.
- Provider pinned with no fallbacks and zero data retention; snapshot drift is logged.
- Deadline per call, bounded retries on 429/5xx with `Retry-After`, and a hedged second request after a short delay.
- Every call and answer goes to `decision_log`.

**Fail open:** if Jev errors or times out, the code ranking is shown as is, labelled "ranked from stats".

## Consequences

- Answers stay inside the database's facts; the LLM cannot add a hero or item.
- Three model hops cost latency; the UI streams the code ranking first and refines.
- Jev's thresholds need calibration on our questions; start in shadow (log only) for low-confidence cases.
