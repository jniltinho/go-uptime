# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
"""Builds .github/assets/logo-with-{dark,light}-text.png: the icon of the project and its name, in the Inter of the interface."""
import sys
from fontTools.ttLib import TTFont
from PIL import Image, ImageDraw, ImageFont

repo, out = sys.argv[1], sys.argv[2]
font_path = out + "/inter.ttf"
font = TTFont(repo + "/web/app/src/assets/fonts/inter-4-1-latin.woff2")
font.flavor = None
font.save(font_path)

HEIGHT, GAP, SIZE, TEXT = 663, 120, 430, "Go Uptime"
icon = Image.open(repo + "/.github/assets/logo.png").convert("RGBA").resize((HEIGHT, HEIGHT), Image.LANCZOS)
face = ImageFont.truetype(font_path, SIZE)
try:
    face.set_variation_by_axes([650, 32])  # weight, optical size
except Exception:
    face.set_variation_by_axes([650])
probe = ImageDraw.Draw(Image.new("RGBA", (10, 10)))
left, top, right, bottom = probe.textbbox((0, 0), TEXT, font=face)
width = HEIGHT + GAP + (right - left) + 20
for name, colour in (("dark", (29, 60, 85, 255)), ("light", (255, 255, 255, 255))):
    canvas = Image.new("RGBA", (width, HEIGHT), (0, 0, 0, 0))
    canvas.alpha_composite(icon, (0, 0))
    ImageDraw.Draw(canvas).text((HEIGHT + GAP - left, (HEIGHT - (bottom - top)) // 2 - top), TEXT, font=face, fill=colour)
    canvas.save(f"{out}/logo-with-{name}-text.png", optimize=True)
    print(name, canvas.size)
