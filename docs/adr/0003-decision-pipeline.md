# ADR-0003: The LLM writes, Jev decides, the code executes

Status: accepted · 2026-10-27

## Context

A chat model asked "what should I pick?" answers fluently and often wrongly: it names heroes that are already picked, quotes old patches, or invents items. A better split is one where a calibrated decision model makes the choices and code owns every side effect.

## Decision

Every suggestion goes through four steps:

1. **Parse (LLM, optional).** Free input (chat text, transcript) becomes a typed situation. Photos go through the vision step instead (ADR-0004).
2. **Candidates (code).** Code queries the database and ranks legal options with explicit, numeric reasons (counter delta per enemy, synergy, role fit, patch win rate). Illegal options (picked, banned, wrong patch) never leave this step.
3. **Decide (Jev).** The top N candidates go to Jev (`typesafe/jev-*` on OpenRouter's System One endpoint, model pinned by `DECISION_MODEL`) as a `choice` question whose criteria are the candidates with their reasons, plus `score` questions where a level is needed (e.g. how urgent an item is). Jev returns the choice, probabilities and confidence.
4. **Write (LLM).** A writer model turns the decided, structured result into a short tip in the player's language and coach persona. Code checks the text names only entities from the result; a failing text is replaced by a template.

Client rules (implemented in `api/internal/decision`):

- Strict parsing: every question asked must come back with the same type and a value, else the whole call fails.
- Provider pinned with no fallbacks and zero data retention; snapshot drift is logged.
- Deadline per call, bounded retries on 429/5xx with `Retry-After`, and a hedged second request after a short delay.
- Every call and answer goes to `decision_log`.
- Defaults (2026-10-28): a 1.5 s deadline per call, up to 3 attempts per request with a 100 ms doubling backoff (or `Retry-After`) while the deadline leaves room, and a hedged second request after 600 ms; the first answer wins. The draft sends the ranking's top 8 (ADR-0009) as one `choice` keyed by hero name, with the draft, the position and each candidate's reasons in the state and criteria. `POST /api/draft/suggest` returns `source` (`jev` or `ranking`) and, for the ranking, a `reason` (`timeout`, `error`, `unavailable`, `few_candidates`).

**Fail open:** if Jev errors or times out, the code ranking is shown as is, labelled "ranked from stats".

## Consequences

- Answers stay inside the database's facts; the LLM cannot add a hero or item.
- Three model hops cost latency; the UI streams the code ranking first and refines.
- Jev's thresholds need calibration on our questions; start in shadow (log only) for low-confidence cases.
- Amended 2026-10-29 (B1, B2): the 0.4 confidence floor applies to `choice` answers (the item's enemy, the coach's advice category). A `score` over ordered levels (an item's urgency, the advice priority) came back at 0.1 to 0.4 confidence on every live call, so its most likely level stands without a floor.
