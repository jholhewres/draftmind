#!/usr/bin/env python3
"""Build the synthetic pick-screen photo set for Draftmind's bench-photo.

Each fixture renders a 1920x1080 pick screen whose top bar holds the ten
hero portraits (five per team, some slots still empty), then photographs it:
the screen sits in a darker frame with a perspective tilt, blur, glare, moire,
sensor noise and JPEG compression. Next to each image goes the expected draft.

Usage: python3 -I scripts/fixtures/make_fixtures.py [repo root]
Needs Python 3 with Pillow and numpy, psql, and a synced database (make sync).
The repo root defaults to two levels above this script. Reads the current
patch's heroes with psql (DATABASE_URL from the environment, else from
<repo>/.env), caches portraits in <repo>/api/data/portraits (gitignored,
shared with the API's cache) and writes
<repo>/api/testdata/photos/synthetic-NN.{jpg,json}.
Deterministic: the same heroes table and seed give the same files.
"""
import io
import json
import math
import os
import random
import subprocess
import sys
import urllib.request

import numpy as np
from PIL import Image, ImageDraw, ImageFilter

CDN = "https://cdn.cloudflare.steamstatic.com"
SEED = 20261028
SCREEN_W, SCREEN_H = 1920, 1080
# Top bar geometry on the 1920x1080 screen: five portraits a side of the clock.
PW, PH, GAP, TOP = 118, 66, 6, 10
CLOCK_W = 236


def env_value(repo, key):
    if os.environ.get(key):
        return os.environ[key]
    path = os.path.join(repo, ".env")
    if os.path.exists(path):
        with open(path) as f:
            for line in f:
                if line.startswith(key + "="):
                    return line.split("=", 1)[1].strip()
    raise SystemExit(f"{key} is not set in the environment or {path}")


def heroes(repo):
    url = env_value(repo, "DATABASE_URL")
    out = subprocess.run(
        ["psql", url, "-At", "-F", "\t", "-c",
         "SELECT h.id, h.slug, h.name, h.img FROM hero h JOIN hero_patch hp ON hp.hero_id = h.id "
         "JOIN patch p ON p.id = hp.patch_id AND p.is_current ORDER BY h.id"],
        check=True, capture_output=True, text=True).stdout
    rows = [line.split("\t") for line in out.strip().splitlines()]
    return [{"id": int(r[0]), "slug": r[1], "name": r[2], "img": r[3]} for r in rows]


def portrait(cache, hero):
    path = os.path.join(cache, os.path.basename(hero["img"]))
    if not os.path.exists(path):
        with urllib.request.urlopen(CDN + hero["img"], timeout=30) as r:
            data = r.read()
        with open(path, "wb") as f:
            f.write(data)
    return Image.open(path).convert("RGB")


def slot_boxes():
    """The ten slot rectangles on the screen, radiant left to right then dire."""
    row = 5 * PW + 4 * GAP
    left0 = (SCREEN_W - CLOCK_W) // 2 - row
    right0 = (SCREEN_W + CLOCK_W) // 2
    boxes = []
    for base in (left0, right0):
        for i in range(5):
            x = base + i * (PW + GAP)
            boxes.append((x, TOP, x + PW, TOP + PH))
    return boxes


def screen(rng, cache, all_heroes, slots):
    img = Image.new("RGB", (SCREEN_W, SCREEN_H), (14, 17, 22))
    d = ImageDraw.Draw(img)
    # Background: a vignette and a faint map-like texture.
    arr = np.asarray(img).astype(np.float32)
    yy, xx = np.mgrid[0:SCREEN_H, 0:SCREEN_W]
    vign = 1.0 - 0.55 * (((xx - SCREEN_W / 2) / SCREEN_W) ** 2 + ((yy - SCREEN_H / 2) / SCREEN_H) ** 2)
    tex = 10 * np.sin(xx / 37.0 + np.sin(yy / 53.0)) * np.cos(yy / 41.0)
    arr = arr * vign[..., None] + tex[..., None] * np.array([0.6, 0.7, 0.5])
    img = Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8))
    d = ImageDraw.Draw(img)
    # Top bar band and the clock.
    d.rectangle((0, 0, SCREEN_W, TOP + PH + 18), fill=(9, 11, 14))
    cx = SCREEN_W // 2
    d.rectangle((cx - CLOCK_W // 2 + 20, 6, cx + CLOCK_W // 2 - 20, 70), fill=(24, 27, 33))
    d.text((cx - 18, 26), f"0:{rng.randint(10, 59):02d}", fill=(220, 220, 220))
    for i, (box, hero) in enumerate(zip(slot_boxes(), slots)):
        color = (94, 160, 52) if i < 5 else (190, 60, 44)
        x0, y0, x1, y1 = box
        if hero is None:
            # An empty slot: a dark panel with a faint team tint.
            d.rectangle(box, fill=(22, 25, 30))
            d.rectangle((x0 + 2, y0 + 2, x1 - 2, y1 - 2), outline=(40, 44, 50), width=1)
        else:
            img.paste(portrait(cache, hero).resize((PW, PH), Image.LANCZOS), (x0, y0))
        d.rectangle((x0, y1 + 2, x1, y1 + 6), fill=color)
    # The hero grid below, the same portraits the top bar uses: a distractor.
    pool = rng.sample(all_heroes, 60)
    gw, gh = 96, 54
    gx0, gy0 = 240, 210
    for i, hero in enumerate(pool):
        r, c = divmod(i, 15)
        x, y = gx0 + c * (gw + 6), gy0 + r * (gh + 8)
        p = portrait(cache, hero).resize((gw, gh), Image.LANCZOS)
        if rng.random() < 0.3:  # picked or banned heroes are greyed out
            p = Image.blend(p, p.convert("L").convert("RGB"), 0.85)
        img.paste(p, (x, y))
    d.rectangle((240, 520, 1680, 1040), fill=(18, 21, 27))
    d.text((260, 540), "PICK A HERO", fill=(200, 180, 120))
    return img


def find_coeffs(dst, src):
    """PIL PERSPECTIVE coefficients mapping output points dst to input points src."""
    m = []
    for (x, y), (u, v) in zip(dst, src):
        m.append([x, y, 1, 0, 0, 0, -u * x, -u * y])
        m.append([0, 0, 0, x, y, 1, -v * x, -v * y])
    a = np.array(m, dtype=np.float64)
    b = np.array(src, dtype=np.float64).reshape(8)
    return np.linalg.solve(a, b).tolist()


def photograph(rng, scr, case):
    """Photograph the screen: frame, tilt, blur, glare, moire, noise, JPEG."""
    out_w, out_h = 1600, 1200 if case["crop"] == "full" else 900
    # The screen's corners in the photo; the "top" framing shows only the
    # upper part of the screen, as players are asked to frame the hero bar.
    if case["crop"] == "full":
        sx0, sy0, sx1, sy1 = 120, 160, 1480, 925
    else:
        # Zoomed in on the hero bar: the screen's sides and bottom fall
        # outside the photo; the aspect ratio is kept.
        sx0, sy0, sx1, sy1 = -200, 80, 1800, 1205
    src_h = SCREEN_H
    j = case["tilt"]
    corners = [
        (sx0 + rng.uniform(-j, j), sy0 + rng.uniform(-j, j)),
        (sx1 + rng.uniform(-j, j), sy0 + rng.uniform(-j, j)),
        (sx1 + rng.uniform(-j, j), sy1 + rng.uniform(-j, j)),
        (sx0 + rng.uniform(-j, j), sy1 + rng.uniform(-j, j)),
    ]
    src = [(0, 0), (SCREEN_W, 0), (SCREEN_W, src_h), (0, src_h)]
    warped = scr.transform((out_w, out_h), Image.PERSPECTIVE, find_coeffs(corners, src),
                           Image.BICUBIC, fillcolor=(0, 0, 0))
    mask = Image.new("L", (out_w, out_h), 0)
    ImageDraw.Draw(mask).polygon(corners, fill=255)
    # Room around the monitor: a dim, warm gradient with a bezel.
    yy, xx = np.mgrid[0:out_h, 0:out_w]
    room = np.stack([30 + 25 * yy / out_h, 26 + 18 * yy / out_h, 22 + 10 * xx / out_w], -1)
    photo = Image.fromarray(room.astype(np.uint8))
    bezel = [(x + (x - out_w / 2) * 0.02, y + (y - out_h / 2) * 0.03) for x, y in corners]
    ImageDraw.Draw(photo).polygon(bezel, fill=(8, 8, 9))
    photo.paste(warped, (0, 0), mask)
    arr = np.asarray(photo).astype(np.float32)
    # Screen-door moire: interference of two rotated gratings.
    a = math.radians(case["moire_angle"])
    f1, f2 = 2 * math.pi / 3.1, 2 * math.pi / 3.4
    u = xx * math.cos(a) + yy * math.sin(a)
    moire = np.sin(u * f1) * np.sin((xx * math.cos(a + 0.05) + yy * math.sin(a + 0.05)) * f2)
    arr += case["moire"] * moire[..., None] * np.array([1.0, 0.9, 1.1])
    # Glare: a soft bright blob, sometimes over the hero bar.
    gx, gy = case["glare_at"][0] * out_w, case["glare_at"][1] * out_h
    r2 = ((xx - gx) / (out_w * case["glare_r"])) ** 2 + ((yy - gy) / (out_h * case["glare_r"] * 0.7)) ** 2
    arr += case["glare"] * np.exp(-r2)[..., None] * np.array([1.0, 0.98, 0.92])
    # Exposure, a slight colour cast and sensor noise.
    arr = arr * case["exposure"] + np.array(case["cast"])
    arr += np.random.default_rng(case["seed"]).normal(0, case["noise"], arr.shape)
    photo = Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8))
    photo = photo.filter(ImageFilter.GaussianBlur(case["blur"]))
    buf = io.BytesIO()
    photo.save(buf, "JPEG", quality=case["quality"])
    return buf.getvalue()


CASES = [
    # name, framing, picks per side, tilt (px), blur, glare, moire...
    dict(crop="top", picks=(5, 5), tilt=20, blur=0.8, glare=40, glare_at=(0.8, 0.7), glare_r=0.25, moire=6, moire_angle=12, exposure=1.0, cast=(0, 0, 0), noise=4, quality=88),
    dict(crop="top", picks=(4, 5), tilt=45, blur=1.4, glare=90, glare_at=(0.35, 0.15), glare_r=0.18, moire=10, moire_angle=30, exposure=0.95, cast=(4, 0, -4), noise=6, quality=82),
    dict(crop="full", picks=(5, 5), tilt=35, blur=1.0, glare=70, glare_at=(0.6, 0.3), glare_r=0.3, moire=8, moire_angle=5, exposure=1.05, cast=(0, 3, 6), noise=5, quality=85),
    dict(crop="top", picks=(3, 4), tilt=60, blur=1.8, glare=50, glare_at=(0.2, 0.6), glare_r=0.3, moire=12, moire_angle=48, exposure=0.9, cast=(-3, 0, 5), noise=7, quality=78),
    dict(crop="full", picks=(4, 3), tilt=25, blur=2.2, glare=120, glare_at=(0.72, 0.2), glare_r=0.15, moire=7, moire_angle=20, exposure=1.0, cast=(6, 2, 0), noise=6, quality=80),
    dict(crop="top", picks=(5, 4), tilt=80, blur=1.2, glare=60, glare_at=(0.5, 0.1), glare_r=0.22, moire=14, moire_angle=70, exposure=1.1, cast=(0, 0, 0), noise=5, quality=84),
    dict(crop="top", picks=(2, 2), tilt=30, blur=0.6, glare=30, glare_at=(0.9, 0.9), glare_r=0.3, moire=5, moire_angle=85, exposure=0.85, cast=(2, 2, 2), noise=8, quality=75),
    dict(crop="full", picks=(5, 5), tilt=55, blur=1.6, glare=100, glare_at=(0.3, 0.25), glare_r=0.2, moire=11, moire_angle=38, exposure=0.95, cast=(-4, 2, 6), noise=6, quality=80),
]


def main():
    if len(sys.argv) > 2:
        raise SystemExit(__doc__)
    default = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..")
    repo = os.path.abspath(sys.argv[1] if len(sys.argv) == 2 else default)
    cache = os.path.join(repo, "api", "data", "portraits")
    out = os.path.join(repo, "api", "testdata", "photos")
    os.makedirs(cache, exist_ok=True)
    os.makedirs(out, exist_ok=True)
    all_heroes = heroes(repo)
    rng = random.Random(SEED)
    for n, case in enumerate(CASES, 1):
        case = dict(case, seed=SEED + n)
        picked = rng.sample(all_heroes, 10)
        sides = []
        for side, count in enumerate(case["picks"]):
            team = picked[side * 5: side * 5 + 5]
            # Picks fill slots in order; the rest are still empty.
            sides.append([h if i < count else None for i, h in enumerate(team)])
        slots = sides[0] + sides[1]
        jpg = photograph(rng, screen(rng, cache, all_heroes, slots), case)
        name = f"synthetic-{n:02d}"
        with open(os.path.join(out, name + ".jpg"), "wb") as f:
            f.write(jpg)
        expected = {
            "synthetic": True,
            "radiant": [h["id"] if h else None for h in sides[0]],
            "dire": [h["id"] if h else None for h in sides[1]],
            "names": {"radiant": [h["name"] if h else None for h in sides[0]],
                      "dire": [h["name"] if h else None for h in sides[1]]},
        }
        with open(os.path.join(out, name + ".json"), "w") as f:
            json.dump(expected, f, indent=2)
            f.write("\n")
        print(name, len(jpg), "bytes", case["crop"], case["picks"])


if __name__ == "__main__":
    main()
