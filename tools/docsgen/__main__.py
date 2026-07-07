"""docsgen — generate per-revision overlay pages. Run from the worktree root."""
import argparse
import sys
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

# Golden facts (id: debugname) per revision, gating icons.map_records's
# id-density mapping for the four cache-backed revisions (see each rev's
# goscape-client cmd/icondump/testdata/icons<rev>/sample.json). Verified
# directly against each revision's content tree pack/obj.pack: knife and
# bronze_dagger happen to hold the SAME ids (946/1205) on every revision
# 244-274 — pinned per revision anyway, not shared, since a future revision
# could renumber either.
ICON_SPOT_CHECKS = {
    "244": {1205: "bronze_dagger", 946: "knife"},
    "245.2": {1205: "bronze_dagger", 946: "knife"},
    "254": {1205: "bronze_dagger", 946: "knife"},
    "274": {1205: "bronze_dagger", 946: "knife"},
}

# rev-225 (config_source = content-tree): records have no obj id, so the
# equivalent golden fact is checked the other way around — debugname ->
# expected id, against the inverted pack/obj.pack (icons.map_records_by_debugname).
# Same ids as every other revision (verified against
# Server225_2/content/pack/obj.pack directly).
ICON_SPOT_CHECKS_BY_NAME = {
    "225": {"bronze_dagger": 1205, "knife": 946},
}


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
            # generate_config_families renders item rows. The mapping and
            # the overlay copy live in the configs step, so `--steps icons`
            # alone renders into the temp dir and discards the result.
            icon_render = None  # (out_dir, total) once icondump has run
            if "icons" in steps and icons_enabled:
                if "configs" not in steps:
                    print(
                        f"rev-{rev}: warning: icons step output is discarded "
                        "without the configs step",
                        file=sys.stderr,
                    )
                client_wt = Path(icons_cfg["client_worktree"])
                icons.assert_branch(client_wt, icons_cfg["client_branch"])
                icondump = icons.build_icondump(
                    client_wt, workdir / "icondump", td,
                )
                icons_out = workdir / "icons_rendered"
                # rev-225 has no client cache (jag-era pack pipeline) and
                # takes -jag-dir instead of -cache; every other revision's
                # icons config sets cache_dir. icons.render_icons requires
                # exactly one of the two.
                _rendered, _skipped, total = icons.render_icons(
                    icondump, icons_out,
                    cache_dir=icons_cfg.get("cache_dir"),
                    jag_dir=icons_cfg.get("jag_dir"),
                )
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
                    names = icons.load_obj_pack(
                        Path(cfg["content_dir"]) / "pack" / "obj.pack"
                    )
                    if content_tree:
                        # rev-225: content-tree records already carry real
                        # debugnames as their header (no [obj_N] placeholder
                        # to patch), but no obj id — record order isn't id
                        # order, so the id-density mapping below doesn't
                        # apply. Resolve id via the inverted obj.pack instead
                        # (icons.map_records_by_debugname).
                        records = configtext.parse_config_text(
                            (all_dir / "all.obj").read_text(errors="replace")
                        )
                        icons_set, matched, total_records = icons.map_records_by_debugname(
                            records, names, icons_out,
                            overlay / "player" / "items" / "icons",
                            spot_checks=ICON_SPOT_CHECKS_BY_NAME.get(rev),
                        )
                        print(
                            f"rev-{rev}: icons matched {matched}/{total_records} "
                            f"content-tree records ({matched / total_records:.1%})",
                            file=sys.stderr,
                        )
                    else:
                        # Real debugnames: goscape-cli unpack config (above)
                        # only ever synthesizes `[obj_N]` placeholder headers
                        # (binary configs carry no debug name). The content
                        # tree's git-tracked pack/obj.pack is the
                        # id=debugname source of truth (see
                        # icons.load_obj_pack), so patch those names into
                        # all.obj before parsing.
                        icons.patch_debugnames(all_dir / "all.obj", names)
                        records = configtext.parse_config_text(
                            (all_dir / "all.obj").read_text(errors="replace")
                        )
                        icons_set = icons.map_records(
                            records, total, icons_out,
                            overlay / "player" / "items" / "icons",
                            spot_checks=ICON_SPOT_CHECKS.get(rev),
                        )
                    # "Files copied" — may be < icondump's own rendered count
                    # for content-tree revisions (unmatched records get no
                    # icon); equal to it for cache-backed revisions (density
                    # check above already enforces 1:1 record<->id coverage).
                    counts["icons"] = len(icons_set)

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
        "| Revision | Items | Icons | NPCs | Locations | Varps | Commands | Music | Places |",
        "|---|---|---|---|---|---|---|---|---|",
    ]
    for rev, c in summary.items():
        lines.append(
            f"| rev-{rev} | {c.get('items', '')} | {c.get('icons', '')} "
            f"| {c.get('npcs', '')} | {c.get('locs', '')} | {c.get('varps', '')} "
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
