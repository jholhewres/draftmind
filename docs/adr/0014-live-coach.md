# ADR-0014: Live coach from the shared game window

Status: accepted · 2026-10-29 · default off (`LIVE_COACH=off`) until benchmarked

## Context

With the game window shared (ADR-0013), the coach can speak up during the match without being asked. The owner's cost rule: use the language model as little as possible. Jev is cheap (about $0.0012 a look); the writer is not, and its free text is what the entity check has to police.

## Decision

**Templates first, the model last.** The pipeline still follows ADR-0003 (the LLM writes, Jev decides, the code executes), with every step that can be code made code:

1. **Free browser gate.** Every tick (30 s, `draftmind.liveTick` overrides it, 10–300 s) the browser samples the window at 64×36. It sends nothing when the frame did not change (mean 8×8 brightness move under 6) or does not look like a match in progress (a black screen, or no dark HUD band under a brighter world). Capture stops when the share stops, the player leaves the Coach tab, or the server reports the match ended.
2. **Code gate before Jev** (`match.Live.Tick`): a finished match or a tick under 10 s after the last one never reaches Jev.
3. **One closed Jev question.** `event`: one of `silent`, `item_timing`, `deaths`, `objective`, `phase`, `map`, with its probability as the confidence. No free text comes back. Under 0.6 counts as silent.
4. **Cooldown and dedupe in code.** The same event type is not spoken within 4 minutes (2 in Turbo, ADR-0012's time scale), and at most one event is spoken in any minute.
5. **The line is a template.** A spoken event is stored as a coach message tagged `live` (`source: live`, `reason: live:<event>`); the UI words it per event in en and pt-BR. An item event carries the next item of the session's item plan (the plan's urgent situational items, then its mid and late core) as its card; with no item left it stays silent. The writer is called only when `LIVE_WRITER=on`, with the event and that item as its only facts, and its text passes the entity check or the template stands.
6. **Fail silent.** A failed or late Jev call (8 s), a failed store or a refused frame posts nothing.

**Cost is visible.** Each tick's Jev cost comes back to the page, which shows looks, spoken events and the session's cost in the live bar; Jev calls are in `decision_log`, writer calls in `model_call`, and every tick is logged with its reason. Budget: about $0.10 for a 40-minute match at one look every 30 s, less with the change gate.

**Fair play and privacy** as ADR-0013: only the player's own screen, nothing stored, no overlay, no automation. The chat keeps working for manual questions; the "Items now?" chip stays code-only (ADR-0011).

## Pending

- **Benchmark before the default turns on.** The repo has pick- and ban-phase screenshots only (`api/testdata/photos/real`), none taken during a match, so Jev's event accuracy and p50 latency on in-match frames are not measured. Accuracy is not estimated or faked: `LIVE_COACH` stays off until a set of real in-match screenshots, labelled by event, is benchmarked.
- With Game State Integration (card f7), code triggers on the game's own state run before any Jev call and replace reading the HUD.
