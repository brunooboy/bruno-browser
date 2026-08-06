from __future__ import annotations

from pathlib import Path

from PIL import Image, ImageDraw


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "build" / "appicon.png"

INK = (220, 231, 226)
MUTED = (107, 130, 121)
GREEN = (66, 255, 145)
CYAN = (48, 213, 200)
DARK = (5, 8, 10)
PANEL = (9, 17, 21)
GRID = (14, 30, 34)


GLYPHS = {
    "A": ("01110", "10001", "10001", "11111", "10001", "10001", "10001"),
    "B": ("11110", "10001", "10001", "11110", "10001", "10001", "11110"),
    "C": ("01111", "10000", "10000", "10000", "10000", "10000", "01111"),
    "E": ("11111", "10000", "10000", "11110", "10000", "10000", "11111"),
    "F": ("11111", "10000", "10000", "11110", "10000", "10000", "10000"),
    "I": ("11111", "00100", "00100", "00100", "00100", "00100", "11111"),
    "L": ("10000", "10000", "10000", "10000", "10000", "10000", "11111"),
    "N": ("10001", "11001", "11001", "10101", "10011", "10011", "10001"),
    "O": ("01110", "10001", "10001", "10001", "10001", "10001", "01110"),
    "P": ("11110", "10001", "10001", "11110", "10000", "10000", "10000"),
    "R": ("11110", "10001", "10001", "11110", "10100", "10010", "10001"),
    "S": ("01111", "10000", "10000", "01110", "00001", "00001", "11110"),
    "T": ("11111", "00100", "00100", "00100", "00100", "00100", "00100"),
    "U": ("10001", "10001", "10001", "10001", "10001", "10001", "01110"),
    "W": ("10001", "10001", "10001", "10101", "10101", "10101", "01010"),
}


def text_width(value: str, scale: int) -> int:
    return sum((5 if character != " " else 3) * scale for character in value) + max(0, len(value) - 1) * scale


def pixel_text(draw: ImageDraw.ImageDraw, position: tuple[int, int], value: str, scale: int, fill: tuple[int, int, int]) -> None:
    x, y = position
    for character in value.upper():
        if character == " ":
            x += 4 * scale
            continue
        glyph = GLYPHS.get(character)
        if glyph is None:
            x += 6 * scale
            continue
        for row, cells in enumerate(glyph):
            for column, cell in enumerate(cells):
                if cell == "1":
                    draw.rectangle(
                        (x + column * scale, y + row * scale, x + (column + 1) * scale - 1, y + (row + 1) * scale - 1),
                        fill=fill,
                    )
        x += 6 * scale


def cropped_icon() -> Image.Image:
    image = Image.open(SOURCE).convert("RGBA")
    alpha_box = image.getchannel("A").getbbox()
    if alpha_box is None:
        raise RuntimeError("app icon has no visible pixels")
    return image.crop(alpha_box)


def fitted_icon(icon: Image.Image, size: int, padding: int = 0) -> Image.Image:
    target = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    available = max(1, size - 2 * padding)
    resized = icon.copy()
    resized.thumbnail((available, available), Image.Resampling.NEAREST)
    target.alpha_composite(resized, ((size - resized.width) // 2, (size - resized.height) // 2))
    return target


def add_grid(draw: ImageDraw.ImageDraw, width: int, height: int, spacing: int) -> None:
    for x in range(0, width, spacing):
        draw.line((x, 0, x, height), fill=GRID)
    for y in range(0, height, spacing):
        draw.line((0, y, width, y), fill=GRID)


def build_installer_left(icon: Image.Image) -> Image.Image:
    image = Image.new("RGB", (164, 314), DARK)
    draw = ImageDraw.Draw(image)
    add_grid(draw, image.width, image.height, 16)
    draw.rectangle((0, 0, 163, 8), fill=GREEN)
    draw.rectangle((0, 9, 104, 11), fill=CYAN)
    draw.rectangle((12, 22, 151, 170), fill=PANEL)
    draw.rectangle((12, 22, 151, 170), outline=(24, 58, 54), width=2)
    mark = fitted_icon(icon, 128, 4)
    image.paste(mark, (18, 31), mark)

    first = "BRUNO"
    second = "BROWSER"
    pixel_text(draw, ((164 - text_width(first, 3)) // 2, 190), first, 3, GREEN)
    pixel_text(draw, ((164 - text_width(second, 3)) // 2, 220), second, 3, INK)
    draw.rectangle((20, 254, 143, 256), fill=CYAN)
    caption = "LOCAL OPS"
    pixel_text(draw, ((164 - text_width(caption, 1)) // 2, 270), caption, 1, MUTED)
    draw.rectangle((16, 296, 20, 300), fill=GREEN)
    draw.rectangle((24, 296, 107, 300), fill=(18, 44, 38))
    draw.rectangle((24, 296, 81, 300), fill=GREEN)
    return image


def build_installer_header(icon: Image.Image) -> Image.Image:
    image = Image.new("RGB", (150, 57), DARK)
    draw = ImageDraw.Draw(image)
    add_grid(draw, image.width, image.height, 12)
    mark = fitted_icon(icon, 50, 2)
    image.paste(mark, (4, 3), mark)
    pixel_text(draw, (60, 9), "BRUNO", 2, GREEN)
    pixel_text(draw, (60, 31), "BROWSER", 1, INK)
    draw.rectangle((60, 48, 143, 50), fill=CYAN)
    return image


def build_banner(icon: Image.Image) -> Image.Image:
    image = Image.new("RGB", (1200, 320), DARK)
    draw = ImageDraw.Draw(image)
    add_grid(draw, image.width, image.height, 32)
    draw.rectangle((0, 0, 1199, 9), fill=GREEN)
    draw.rectangle((0, 10, 760, 13), fill=CYAN)
    draw.rectangle((34, 30, 286, 290), fill=PANEL)
    draw.rectangle((34, 30, 286, 290), outline=(24, 58, 54), width=3)
    mark = fitted_icon(icon, 238, 8)
    image.paste(mark, (42, 40), mark)
    pixel_text(draw, (340, 62), "BRUNO", 9, GREEN)
    pixel_text(draw, (340, 148), "BROWSER", 7, INK)
    pixel_text(draw, (344, 230), "LOCAL FIRST PROFILE OPS", 3, CYAN)
    draw.rectangle((344, 282, 1120, 286), fill=(18, 44, 38))
    draw.rectangle((344, 282, 866, 286), fill=GREEN)
    draw.rectangle((1135, 276, 1145, 286), fill=(255, 83, 98))
    return image


def save_assets() -> None:
    icon = cropped_icon()
    targets = {
        ROOT / "frontend" / "src" / "assets" / "bruno-browser-icon.png": fitted_icon(icon, 256, 10),
        ROOT / "docs" / "assets" / "bruno-browser-icon.png": fitted_icon(icon, 512, 18),
    }
    for path, image in targets.items():
        path.parent.mkdir(parents=True, exist_ok=True)
        image.save(path, optimize=True)

    windows_icon = ROOT / "build" / "windows" / "icon.ico"
    windows_icon.parent.mkdir(parents=True, exist_ok=True)
    fitted_icon(icon, 256, 12).save(
        windows_icon,
        format="ICO",
        sizes=((16, 16), (20, 20), (24, 24), (32, 32), (40, 40), (48, 48), (64, 64), (128, 128), (256, 256)),
    )

    installer = ROOT / "build" / "windows" / "installer" / "resources"
    installer.mkdir(parents=True, exist_ok=True)
    build_installer_left(icon).save(installer / "leftimage.bmp")
    build_installer_header(icon).save(installer / "headerimage.bmp")
    build_banner(icon).save(ROOT / "docs" / "assets" / "bruno-browser-banner.png", optimize=True)


if __name__ == "__main__":
    save_assets()
