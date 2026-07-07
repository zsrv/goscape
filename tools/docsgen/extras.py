"""Music track list and worldmap place-name index."""
from pathlib import Path

from .render import GENERATED, md_escape


def parse_labels(text: str) -> list[tuple[str, int, int, int]]:
    out = []
    for line in text.splitlines():
        line = line.strip()
        if not line.startswith("="):
            continue
        parts = line[1:].rsplit(",", 3)
        if len(parts) != 4:
            continue
        name = parts[0].replace("/", " ")
        try:
            x, z, kind = int(parts[1]), int(parts[2]), int(parts[3])
        except ValueError:
            continue
        out.append((name, x, z, kind))
    return out


def render_places_page(labels, out: Path) -> int:
    lines = [GENERATED, "", "# Places", "",
             "World-map labels from the game cache.", "",
             "| Name | X | Z |", "|---|---|---|"]
    for name, x, z, _kind in sorted(labels):
        lines.append(f"| {md_escape(name)} | {x} | {z} |")
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text("\n".join(lines) + "\n")
    return len(labels)


def render_music_page(content_dir: Path, out: Path) -> int:
    groups: dict[str, list[str]] = {}
    for p in sorted(content_dir.rglob("*.mid")):
        groups.setdefault(p.parent.name, []).append(p.stem)
    lines = [GENERATED, "", "# Music tracks", ""]
    total = 0
    for group in sorted(groups):
        names = sorted(groups[group])
        total += len(names)
        lines += [f"## {group} ({len(names)})", ""]
        lines += [f"- {md_escape(n)}" for n in names]
        lines.append("")
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text("\n".join(lines))
    return total
