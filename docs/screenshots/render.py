"""Render the demo screens written by `go test -run TestDemoScreens` into
window-framed PNGs. Needs Pillow and a monospace font (JetBrains Mono if present).

    python3 docs/screenshots/render.py INPUT_DIR OUTPUT_DIR
"""
import glob
import os
import re
import subprocess
import sys

from PIL import Image, ImageDraw, ImageFilter, ImageFont

SCALE = 2  # drawn at 2x, then scaled down, for smooth text
BG = (11, 14, 20)
TEXT = (230, 233, 239)
SGR = re.compile(r"(\x1b\[[0-9;:]*m)")


def find_font(names):
    listing = subprocess.run(["fc-list"], capture_output=True, text=True).stdout.splitlines()
    for name in names:
        for line in listing:
            if name in line:
                return line.split(":")[0]
    return None


def load_fonts():
    fallback = "/usr/share/fonts/dejavu-sans-mono-fonts/DejaVuSansMono.ttf"
    regular = find_font(["JetBrainsMono-Regular", "JetBrains Mono"]) or fallback
    bold = find_font(["JetBrainsMono-Bold.", "JetBrainsMono-ExtraBold"]) or regular
    size = 15 * SCALE
    return (ImageFont.truetype(regular, size), ImageFont.truetype(bold, size),
            ImageFont.truetype(regular, 13 * SCALE))


def apply_sgr(codes, state):
    """Update (fg, bg, bold) for one escape sequence."""
    fg, bg, bold = state
    c = [int(v or 0) for v in re.split("[;:]", codes or "0")]
    i = 0
    while i < len(c):
        if c[i] == 0:
            fg, bg, bold = TEXT, None, False
        elif c[i] == 1:
            bold = True
        elif c[i] == 22:
            bold = False
        elif c[i] == 39:
            fg = TEXT
        elif c[i] == 49:
            bg = None
        elif c[i] in (38, 48) and i + 1 < len(c) and c[i + 1] == 2:
            rgb = tuple(c[i + 2:i + 5])
            if c[i] == 38:
                fg = rgb
            else:
                bg = rgb
            i += 4
        i += 1
    return fg, bg, bold


def render_file(src, dst, title="oryn"):
    regular, bold, small = load_fonts()
    cell_w = round(regular.getlength("M"))
    cell_h = int(15 * SCALE * 1.42)
    lines = open(src, encoding="utf8").read().rstrip("\n").split("\n")
    cols = max(len(SGR.sub("", line)) for line in lines)

    pad, bar, margin = 22 * SCALE, 44 * SCALE, 56 * SCALE
    text_w, text_h = cols * cell_w, len(lines) * cell_h
    width, height = text_w + 2 * pad + 2 * margin, text_h + 2 * pad + bar + 2 * margin

    # backdrop: a soft vertical gradient with a blurred shadow under the window
    backdrop = Image.new("RGB", (width, height))
    draw = ImageDraw.Draw(backdrop)
    for y in range(height):
        t = y / height
        draw.line([(0, y), (width, y)], fill=(int(30 + 30 * t), int(24 + 16 * t), int(58 + 40 * t)))
    shadow = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    ImageDraw.Draw(shadow).rounded_rectangle(
        [margin, margin + 14 * SCALE, width - margin, height - margin + 14 * SCALE], radius=14 * SCALE, fill=(0, 0, 0, 150))
    canvas = Image.alpha_composite(backdrop.convert("RGBA"), shadow.filter(ImageFilter.GaussianBlur(26 * SCALE)))

    win = Image.new("RGBA", (width - 2 * margin, height - 2 * margin), (0, 0, 0, 0))
    wd = ImageDraw.Draw(win)
    ww, wh = win.size
    wd.rounded_rectangle([0, 0, ww - 1, wh - 1], radius=14 * SCALE, fill=BG, outline=(52, 58, 76), width=SCALE)
    wd.rounded_rectangle([0, 0, ww - 1, bar + 14 * SCALE], radius=14 * SCALE, fill=(21, 25, 35))
    wd.rectangle([0, bar - 2 * SCALE, ww - 1, bar + 14 * SCALE], fill=BG)
    wd.line([(0, bar), (ww, bar)], fill=(40, 46, 62), width=SCALE)
    for i, colour in enumerate([(255, 95, 87), (254, 188, 46), (40, 200, 64)]):
        x = 22 * SCALE + i * 22 * SCALE
        wd.ellipse([x, bar / 2 - 6 * SCALE, x + 12 * SCALE, bar / 2 + 6 * SCALE], fill=colour)
    wd.text((ww / 2 - small.getlength(title) / 2, bar / 2 - 8 * SCALE), title, font=small, fill=(125, 135, 150))

    state = (TEXT, None, False)
    for row, line in enumerate(lines):
        x = 0
        for part in SGR.split(line):
            m = re.fullmatch(r"\x1b\[([0-9;:]*)m", part)
            if m:
                state = apply_sgr(m[1], state)
                continue
            fg, bg, is_bold = state
            for ch in part:
                px, py = pad + x * cell_w, bar + pad + row * cell_h
                if bg and bg != BG:
                    wd.rectangle([px, py, px + cell_w, py + cell_h], fill=bg)
                if ch != " ":
                    wd.text((px, py + 2 * SCALE), ch, font=bold if is_bold else regular, fill=fg)
                x += 1
        fg, bg, _ = state
        if bg and bg != BG:  # a selected row's band runs to the edge
            wd.rectangle([pad + x * cell_w, bar + pad + row * cell_h, ww - pad, bar + pad + (row + 1) * cell_h], fill=bg)

    canvas.alpha_composite(win, (margin, margin))
    canvas.convert("RGB").resize((width // 2, height // 2), Image.LANCZOS).save(dst, optimize=True)


if __name__ == "__main__":
    src_dir, dst_dir = sys.argv[1:3]
    os.makedirs(dst_dir, exist_ok=True)
    for path in sorted(glob.glob(os.path.join(src_dir, "*.ansi"))):
        name = os.path.basename(path)[: -len(".ansi")]
        render_file(path, os.path.join(dst_dir, name + ".png"))
        print(name)
