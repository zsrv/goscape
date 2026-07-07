"""docsgen — generate per-revision overlay pages. Run from the worktree root."""
import argparse
import tempfile
import tomllib
from pathlib import Path

from . import commands, configtext, contenttree, families, icons, snapshots, unpack
from .extras import parse_labels, render_music_page, render_places_page
from .render import GENERATED

ROOT = Path(__file__).resolve().parent.parent.parent
ALL_STEPS = ["icons", "unpack", "configs", "commands", "extras", "snapshots"]
WORKTREE_STEPS = {"icons", "unpack", "configs", "commands"}

FLOOR_ITEMS = 1000
FLOOR_COMMANDS = 20

# rev-274 golden facts (see cmd/icondump/testdata/icons274/sample.json in the
# goscape-client worktree) — gates the icons step's id-density mapping.
ICON_SPOT_CHECKS = {1205: "bronze_dagger", 946: "knife"}


def load() -> dict:
    with open(ROOT / "tools" / "docsgen" / "revisions.toml", "rb") as f:
        return tomllib.load(f)


def run_revision(rev: str, cfg: dict, steps: list[str]) -> dict[str, int]:
    overlay = ROOT / "overlays" / f"rev-{rev}"
    overlay.mkdir(parents=True, exist_ok=True)
    counts: dict[str, int] = {}
    icons_cfg = cfg.get("icons")
    icons_enabled = bool(icons_cfg and icons_cfg.get("enabled"))
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
            # icons runs BEFORE configs so the rendered icon set (and the
            # real debugnames patched into all.obj below) exist by the time
            # generate_config_families renders item rows.
            icon_render = None  # (out_dir, total) once icondump has run
            if "icons" in steps and icons_enabled:
                client_wt = Path(icons_cfg["client_worktree"])
                icons.assert_branch(client_wt, icons_cfg["client_branch"])
                icondump = icons.build_icondump(
                    client_wt, workdir / "icondump", td,
                )
                icons_out = workdir / "icons_rendered"
                rendered, _skipped, total = icons.render_icons(
                    icondump, icons_cfg["cache_dir"], icons_out,
                )
                counts["icons"] = rendered
                icon_render = (icons_out, total)

            if "unpack" in steps or "configs" in steps:
                if content_tree:
                    all_dir = contenttree.synthesize_all_dir(
                        Path(cfg["content_dir"]), workdir / "all_dir"
                    )
                else:
                    all_dir = unpack.run_unpack(cfg, workdir, wt)

                icons_set = None
                if icon_render is not None:
                    icons_out, total = icon_render
                    # Real debugnames: goscape-cli unpack config (above) only
                    # ever synthesizes `[obj_N]` placeholder headers (binary
                    # configs carry no debug name) — data/symbols/obj.sym is
                    # a pack-pipeline output artifact, so it must be rebuilt
                    # fresh from this same worktree + the revision's content
                    # tree. See icons.build_symbols.
                    server_cli = unpack.build_cli(wt, workdir / "goscape-cli-pack")
                    sym_dir = icons.build_symbols(
                        server_cli, cfg["content_dir"], wt / "data" / "raw", workdir,
                    )
                    symtab = icons.load_obj_sym(sym_dir / "obj.sym")
                    icons.patch_debugnames(all_dir / "all.obj", symtab)
                    records = configtext.parse_config_text(
                        (all_dir / "all.obj").read_text(errors="replace")
                    )
                    icons_set = icons.map_records(
                        records, total, icons_out,
                        overlay / "player" / "items" / "icons",
                        spot_checks=ICON_SPOT_CHECKS,
                    )

                counts |= families.generate_config_families(all_dir, overlay, icons_set)
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
