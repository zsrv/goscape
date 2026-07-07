"""docsgen — generate per-revision overlay pages. Run from the worktree root."""
import argparse
import tempfile
import tomllib
from pathlib import Path

from . import commands, contenttree, families, snapshots, unpack
from .extras import parse_labels, render_music_page, render_places_page
from .render import GENERATED

ROOT = Path(__file__).resolve().parent.parent.parent
ALL_STEPS = ["unpack", "configs", "commands", "extras", "snapshots"]
WORKTREE_STEPS = {"unpack", "configs", "commands"}

FLOOR_ITEMS = 1000
FLOOR_COMMANDS = 20


def load() -> dict:
    with open(ROOT / "tools" / "docsgen" / "revisions.toml", "rb") as f:
        return tomllib.load(f)


def run_revision(rev: str, cfg: dict, steps: list[str]) -> dict[str, int]:
    overlay = ROOT / "overlays" / f"rev-{rev}"
    overlay.mkdir(parents=True, exist_ok=True)
    counts: dict[str, int] = {}
    with tempfile.TemporaryDirectory(prefix=f"docsgen-{rev}-") as td:
        workdir = Path(td)
        wt = workdir / "worktree"
        needs_worktree = bool(WORKTREE_STEPS & set(steps))
        # A revision may source its config data from the content tree instead
        # of a cache unpack (rev-225: no client cache exists and the unpack
        # tooling postdates it; see revisions.toml). In that mode no CLI build
        # is needed for the unpack/configs steps.
        content_tree = cfg.get("config_source") == "content-tree"
        if needs_worktree:
            unpack.add_worktree(unpack.REPO, cfg["branch"], wt)
        try:
            if "unpack" in steps or "configs" in steps:
                if content_tree:
                    all_dir = contenttree.synthesize_all_dir(
                        Path(cfg["content_dir"]), workdir / "all_dir"
                    )
                else:
                    all_dir = unpack.run_unpack(cfg, workdir, wt)
                counts |= families.generate_config_families(all_dir, overlay)
                if counts["items"] < FLOOR_ITEMS:
                    raise SystemExit(
                        f"rev-{rev}: only {counts['items']} items — floor is {FLOOR_ITEMS}"
                    )
            if "commands" in steps:
                go_src = (wt / "modules" / "world"
                          / "handlers_game.go").read_text()
                cheats = commands.parse_go_cheats(go_src)
                debugprocs = commands.scan_debugprocs(Path(cfg["content_dir"]))
                counts["commands"] = commands.render_commands_page(
                    cheats, debugprocs, overlay / "player" / "commands.md",
                )
                if counts["commands"] < FLOOR_COMMANDS:
                    raise SystemExit(
                        f"rev-{rev}: only {counts['commands']} commands — floor is {FLOOR_COMMANDS}"
                    )
            if "extras" in steps:
                counts["music"] = render_music_page(
                    Path(cfg["content_dir"]), overlay / "player" / "music.md"
                )
                labels_path = Path(cfg["content_dir"]) / "maps" / "labels.txt"
                if not labels_path.is_file():
                    raise SystemExit(f"rev-{rev}: {labels_path} missing")
                counts["places"] = render_places_page(
                    parse_labels(labels_path.read_text(errors="replace")),
                    overlay / "player" / "places.md",
                )
            if "snapshots" in steps:
                snapshots.write_snapshots(unpack.REPO, cfg["branch"], overlay)
        finally:
            if needs_worktree:
                unpack.remove_worktree(unpack.REPO, wt)
    return counts


def comparison_lines(summary: dict, notes: dict[str, str]) -> list[str]:
    lines = [
        GENERATED,
        "", "# Revision comparison", "",
        "| Revision | Items | NPCs | Locations | Varps | Commands | Music | Places |",
        "|---|---|---|---|---|---|---|---|",
    ]
    for rev, c in summary.items():
        lines.append(
            f"| rev-{rev} | {c.get('items', '')} | {c.get('npcs', '')} "
            f"| {c.get('locs', '')} | {c.get('varps', '')} "
            f"| {c.get('commands', '')} | {c.get('music', '')} "
            f"| {c.get('places', '')} |"
        )
    noted = [rev for rev in summary if notes.get(rev)]
    if noted:
        lines.append("")
        for rev in noted:
            lines.append(f"- **rev-{rev}:** {notes[rev]}")
    return lines


def write_comparison(summary: dict, notes: dict[str, str], docs_dir: Path) -> None:
    (docs_dir / "player" / "revision-comparison.md").write_text(
        "\n".join(comparison_lines(summary, notes)) + "\n"
    )


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--revision", default="all")
    ap.add_argument("--steps", default=",".join(ALL_STEPS))
    args = ap.parse_args()
    data = load()
    steps = args.steps.split(",")
    revs = data["order"] if args.revision == "all" else [args.revision]
    summary = {}
    for rev in revs:
        summary[rev] = run_revision(rev, data["revisions"][rev], steps)
        print(f"rev-{rev}: {summary[rev]}")
    print("summary:", summary)
    if args.revision == "all" and set(steps) == set(ALL_STEPS):
        notes = {
            rev: rcfg["note"]
            for rev, rcfg in data["revisions"].items() if "note" in rcfg
        }
        write_comparison(summary, notes, ROOT / "docs")


if __name__ == "__main__":
    main()
