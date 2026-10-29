#!/usr/bin/env python3
"""Draw the synthetic end-of-match screen used as a test fixture.

Radiant wins in 38:12; Juggernaut (the e2e session's hero) is on radiant at
9/3/14. The tests that read it expect exactly these values.

Usage: python3 -I scripts/fixtures/make_endscreen.py [OUT.jpg]
Needs Python 3 with Pillow and the DejaVu Sans fonts. OUT defaults to
api/internal/photo/testdata/endscreen.jpg; copy the result to
web/e2e/fixtures/end-screen.jpg too. FONT_DIR points at another folder that
holds DejaVuSans.ttf and DejaVuSans-Bold.ttf.
"""
import os
import sys

from PIL import Image, ImageDraw, ImageFont

FONT_DIRS = [
    os.environ.get("FONT_DIR", ""),
    "/usr/share/fonts/truetype/dejavu",  # Debian, Ubuntu
    "/usr/share/fonts/dejavu-sans-fonts",  # Fedora
    "/usr/share/fonts/TTF",  # Arch
    "/Library/Fonts",
]


def font(name, size):
    for d in FONT_DIRS:
        path = os.path.join(d, name)
        if d and os.path.exists(path):
            return ImageFont.truetype(path, size)
    raise SystemExit(f"{name} not found; set FONT_DIR to the folder that holds it")


radiant = [("Juggernaut", 9, 3, 14), ("Crystal Maiden", 2, 8, 21), ("Axe", 6, 7, 17), ("Lion", 3, 9, 19), ("Puck", 11, 4, 12)]
dire = [("Drow Ranger", 7, 6, 8), ("Bane", 1, 10, 13), ("Viper", 8, 7, 9), ("Lich", 2, 9, 15), ("Pudge", 5, 9, 11)]


def main():
    root = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..")
    out = sys.argv[1] if len(sys.argv) > 1 else os.path.join(root, "api", "internal", "photo", "testdata", "endscreen.jpg")

    img = Image.new("RGB", (1600, 900), (14, 17, 22))
    d = ImageDraw.Draw(img)
    big, mid = font("DejaVuSans-Bold.ttf", 64), font("DejaVuSans-Bold.ttf", 30)
    small = font("DejaVuSans.ttf", 26)
    d.rectangle([0, 0, 1600, 150], fill=(22, 30, 18))
    d.text((800, 60), "RADIANT VICTORY", font=big, fill=(156, 203, 74), anchor="mm")
    d.text((800, 120), "DURATION 38:12", font=mid, fill=(220, 220, 220), anchor="mm")

    def table(y, title, color, rows):
        d.text((60, y), title, font=mid, fill=color)
        for x, h in ((700, "K"), (800, "D"), (900, "A"), (1050, "NET WORTH")):
            d.text((x, y), h, font=small, fill=(160, 168, 178))
        for i, (name, k, de, a) in enumerate(rows):
            yy = y + 50 + i * 54
            d.rectangle([50, yy - 6, 1550, yy + 40], fill=(24, 28, 35) if i % 2 else (30, 35, 43))
            d.text((70, yy), name, font=small, fill=(237, 239, 242))
            for x, v in ((700, k), (800, de), (900, a)):
                d.text((x, yy), str(v), font=small, fill=(237, 239, 242))
            d.text((1050, yy), f"{(k * 1300 + a * 400 + 9000):,}", font=small, fill=(217, 174, 91))

    table(180, "THE RADIANT", (156, 203, 74), radiant)
    table(520, "THE DIRE", (226, 85, 62), dire)
    img.save(out, "JPEG", quality=85)
    print(out)


if __name__ == "__main__":
    main()
