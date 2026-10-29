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

## Last result (2026-10-28, 8 synthetic photos, two runs)

| reader | slots right | wrong hero | flagged | p50 | p95 | cost / photo |
|---|---|---|---|---|---|---|
| jev (`typesafe/jev-1.14`) | 77/80 and 73/80 (96 %, 91 %) | 0 | 3, 7 | 1.86 s, 2.64 s | 4.27 s, 3.47 s | $0.00124 |
| vision (`google/gemini-3.1-flash-lite`) | 46/80 (57.5 %) | 32 | 2 | 3.24 s, 2.84 s | 4.39 s, 3.22 s | $0.00098 |

Jev's shortlist held the right hero in 65/66 and 62/66 picked slots. Photo to first suggestion: p50 2.27 s and 2.26 s, p95 3.42 s and 3.92 s. Default reader: Jev.
