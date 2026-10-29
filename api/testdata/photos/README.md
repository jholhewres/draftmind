# Photo read bench set

`make bench-photo` reads every photo here with both photo readers (ADR-0006) and reports slot accuracy, p50/p95 latency and cost per reader, then the time from photo to first suggestion through the API. It is the only command that calls the models on photos; unit tests use recorded responses.

## The synthetic set (`synthetic-NN`)

**These photos are synthetic, not real photos of the game.** Each one renders a 1920x1080 pick screen whose top bar holds ten hero portraits from the image CDN (five per team either side of a clock, some slots still empty), with the hero grid below as a distractor, then "photographs" it: a perspective tilt, a dark bezel, blur, glare, screen-door moiré, a colour cast, sensor noise and JPEG compression. Some are framed on the hero bar, some show the whole monitor.

They are built by [`scripts/fixtures/make_fixtures.py`](../../../scripts/fixtures/make_fixtures.py) (seed 20261028) from the current patch's heroes. It needs Python 3 with Pillow and numpy, `psql` and a synced database (`make sync`); the same heroes table gives the same files:

```sh
python3 -I scripts/fixtures/make_fixtures.py
```

The end-screen fixture (`api/internal/photo/testdata/endscreen.jpg`, also `web/e2e/fixtures/end-screen.jpg`) is drawn by [`scripts/fixtures/make_endscreen.py`](../../../scripts/fixtures/make_endscreen.py).

The layout of the top bar is invented, so the Jev reader's layout constants (`api/internal/photo/jev.go`) are calibrated on it. Real photos will tell whether they hold.

## The real set (`real/`)

Screenshots and photos of the game's own pick screen. **Only their expected drafts (`real/*.json`) are committed**; the images stay out of git (`.gitignore`), so copy them in by hand. The bench reports them apart from the synthetic set and skips an expected draft whose image is not there.

- `pick-phase` (1920x1080 PNG screenshot): radiant an empty slot, Enigma, a hovered (greyed, not locked) portrait, Faceless Void and Rubick; dire Puck, empty, Crystal Maiden, empty, Beastmaster. The hover counts as empty: the reader must flag it or call it empty, never name it.
- `ban-phase` (the banning phase, a 2560x1440 screenshot downscaled to a 1600x900 JPEG, since the API refuses photos over 2048 px): no hero picked.

The game's bar differs from the synthetic one: the slot nearest the clock is centred 240 px from the clock's centre for 66 px tall slots (177 px on the synthetic bar), and its clock is big white digits rather than a dim panel. The Jev reader tells the two apart by the clock (`gameClock` in `api/internal/photo/jev.go`).

## Adding a real photo

1. Photograph or screenshot the pick screen, as a player would.
2. Downscale it to at most 2048 px on the long side and save it in `real/` as JPEG or PNG, e.g. `real/real-01.jpg`. Strip EXIF (the app's upload does both).
3. Next to it, write `real/real-01.json` with the draft it shows: hero ids from the `hero` table, left to right, `null` for a slot nobody has locked yet.

```json
{
  "synthetic": false,
  "radiant": [1, 2, null, null, null],
  "dire": [8, 14, 22, null, null]
}
```

4. Run `make bench-photo` and compare with the last result below.

## Last result (2026-10-28, 8 synthetic and 2 real photos, after the real-screenshot fix)

| set | reader | slots right | wrong hero | flagged | p50 | p95 | cost / photo |
|---|---|---|---|---|---|---|---|
| synthetic | jev (`typesafe/jev-1.14`) | 78 of 80 (97.5 %) | 0 | 2 | 2.49 s | 3.21 s | $0.00124 |
| synthetic | vision (`google/gemini-3.1-flash-lite`), its names trusted | 46 of 80 (57.5 %) | 32 | 2 | 4.52 s | 5.20 s | $0.00098 |
| real | jev | 17 of 20 (85 %) | 0 | 3 | 2.43 s | 3.44 s | $0.00111 |
| real | vision, its names trusted | 13 of 20 (65 %) | 5 | 2 | 3.52 s | 4.23 s | $0.00096 |

Jev's shortlist held the right hero in 66 of 66 picked synthetic slots and 6 of 6 real ones. On `pick-phase` every locked hero is read and the hover is flagged; on `ban-phase` no hero is guessed (two empty slots flagged). Photo to first suggestion through the API: p50 2.8 s, p95 3.55 s, no failure; the bench fails when p50 is over 5 s.

Before the fix, on `pick-phase` the radiant slots came back shifted by one, Beastmaster read as empty and Crystal Maiden unknown. A first fix that aimed between the two bars' geometries misread synthetic-04 (4 wrong heroes) when the Box answer's centre was 0.4 slot off; the bar's symmetry now places the slots instead.

Earlier, synthetic only (three runs): jev 73 to 77 of 80, 0 wrong heroes.

Default reader: Jev. The vision model is the fallback, and as served it flags every hero it names: its 0.95 was as likely wrong as right.
