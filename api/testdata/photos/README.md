# Photo read bench set

`make bench-photo` reads every photo here with both photo readers (ADR-0006) and reports slot accuracy, p50/p95 latency and cost per reader, then the time from photo to first suggestion through the API. It is the only command that calls the models on photos; unit tests use recorded responses.

## The synthetic set (`synthetic-NN`)

**These photos are synthetic, not real photos of the game.** Each one renders a 1920x1080 pick screen whose top bar holds ten hero portraits from the image CDN (five per team either side of a clock, some slots still empty), with the hero grid below as a distractor, then "photographs" it: a perspective tilt, a dark bezel, blur, glare, screen-door moiré, a colour cast, sensor noise and JPEG compression. Some are framed on the hero bar, some show the whole monitor.

They were built by `~/jhol/scripts/draftmind/a4-photos/make_fixtures.py` (seed 20261028) from the current patch's heroes:

```sh
python3 -I ~/jhol/scripts/draftmind/a4-photos/make_fixtures.py <repo>
```

The layout of the top bar is invented, so the Jev reader's layout constants (`api/internal/photo/jev.go`) are calibrated on it. Real photos will tell whether they hold.

## Adding a real photo

1. Photograph the pick screen with the phone, as a player would.
2. Downscale it to about 1600 px on the long side and save it here as JPEG or PNG, e.g. `real-01.jpg`. Strip EXIF (the app's upload does both).
3. Next to it, write `real-01.json` with the draft it shows: hero ids from the `hero` table, left to right, `null` for a slot nobody has picked yet.

```json
{
  "synthetic": false,
  "radiant": [1, 2, null, null, null],
  "dire": [8, 14, 22, null, null]
}
```

4. Run `make bench-photo` and compare with the last result below. If the Jev reader slips on real photos, recheck the layout constants in `jev.go`.

## Last result (2026-10-28, 8 synthetic photos, three runs)

| reader | slots right | wrong hero | flagged | p50 | p95 | cost / photo |
|---|---|---|---|---|---|---|
| jev (`typesafe/jev-1.14`) | 77, 73, 74 of 80 (91–96 %) | 0 | 3, 7, 6 | 1.86–2.64 s | 2.98–4.27 s | $0.00124 |
| vision (`google/gemini-3.1-flash-lite`), its names trusted | 46 of 80 (57.5 %) | 32 | 2 | 2.84–3.24 s | 3.22–4.39 s | $0.00098 |

Jev's shortlist held the right hero in 62 to 65 of 66 picked slots. Photo to first suggestion through the API: p50 2.1–2.3 s, p95 3.4–3.9 s, no failure; the bench fails when p50 is over 5 s.

Default reader: Jev. The vision model is the fallback, and as served it flags every hero it names: its 0.95 was as likely wrong as right.
