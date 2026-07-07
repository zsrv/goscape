"""Item-icon rasterizer integration, all five revisions.

Two inputs feed this step:

  * icondump (goscape-client `cmd/icondump`, one branch per revision) renders
    every item-inventory icon in a rev's game cache to a 32x32 PNG, id-keyed
    (`<out>/<id>.png`). Built fresh per run from that revision's persistent
    sibling client worktree — nothing here mutates it. Every revision but
    225 loads a classic on-disk cache; 225 predates that cache format
    (jag-era pack pipeline) and loads raw jag archives instead — see
    `_icondump_args`.
  * `<content_dir>/pack/obj.pack` — the git-tracked `id=debugname` registry
    in the revision's content tree, the source of truth for real symbolic
    obj debugnames (goscape's pack pipeline emits data/symbols/obj.sym as a
    pure reformat of it; see pkg/pack/compiler/symbols_export.go:97-105).
    goscape-cli `unpack config` (used for the family tables) decodes binary
    configs that carry no debug name and synthesizes placeholder `[obj_N]`
    headers instead, so obj.pack is where the real names — the ones icon
    files and the golden rasterizer fixtures are keyed by — come from. One
    revision (225) has no unpack tooling at all and sources its records
    straight from the content tree instead, where the header IS already the
    real debugname but carries no obj id — see `map_records_by_debugname`.
"""
import os
import re
import shutil
import subprocess
from pathlib import Path

FLOOR = 0.95

# Content-tree revisions (rev-225: no cache/unpack tooling exists — see
# map_records_by_debugname) gate on the fraction of records that resolve to
# a rendered icon, not on icondump's own rendered/total (that stays FLOOR
# above and is met independently — 225 renders every obj id it has).
NAME_MATCH_FLOOR = 0.90

_SUMMARY_RE = re.compile(r"rendered=(\d+) skipped=(\d+) total=(\d+)")


def _go_env(tmp: str) -> dict:
    return os.environ | {
        "CGO_ENABLED": "0",
        "GOPATH": f"{tmp}/go",
        "GOCACHE": f"{tmp}/go-cache",
    }


def assert_branch(client_worktree: Path, expected_branch: str) -> None:
    """SystemExit if client_worktree isn't currently on expected_branch.

    client_branch is declarative for now — icondump is built directly from
    a persistent sibling checkout (like the server rev worktrees), not a
    docsgen-managed worktree, so this is the only guard against silently
    rendering icons from the wrong revision's client if that checkout ever
    gets repointed.
    """
    result = subprocess.run(
        ["git", "-C", str(client_worktree), "branch", "--show-current"],
        capture_output=True, text=True, check=True,
    )
    actual = result.stdout.strip()
    if actual != expected_branch:
        raise SystemExit(
            f"icons: {client_worktree} is on branch {actual!r}, expected "
            f"{expected_branch!r} (revisions.toml icons.client_branch)"
        )


def build_icondump(client_worktree: Path, out: Path, tmp: str) -> Path:
    subprocess.run(
        ["go", "build", "-o", str(out), "./cmd/icondump"],
        cwd=client_worktree, env=_go_env(tmp), check=True,
    )
    return out


def parse_summary(text: str) -> tuple[int, int, int]:
    m = _SUMMARY_RE.search(text)
    if not m:
        raise SystemExit(f"icons: icondump printed no summary line: {text!r}")
    return int(m.group(1)), int(m.group(2)), int(m.group(3))


def _icondump_args(icondump_bin: Path, out_dir: Path, *,
                   cache_dir: str | None = None, jag_dir: str | None = None) -> list[str]:
    """Build the icondump argv. Every revision but rev-225 loads a classic
    on-disk cache (`-cache <dir>`); rev-225 predates that cache format
    entirely (its era shipped raw jag archives) so its icondump build takes
    `-jag-dir <dir>` instead — same tool contract otherwise (summary line,
    index.tsv, `<id>.png` outputs). Exactly one of cache_dir/jag_dir must be
    given; this is a config error (revisions.toml), not a runtime one, so it
    raises the same SystemExit style as the rest of this module.
    """
    if bool(cache_dir) == bool(jag_dir):
        raise SystemExit(
            "icons: exactly one of icons.cache_dir/icons.jag_dir must be "
            "configured (revisions.toml)"
        )
    flag, value = ("-jag-dir", jag_dir) if jag_dir else ("-cache", cache_dir)
    return [str(icondump_bin), flag, value, "-out", str(out_dir)]


def render_icons(icondump_bin: Path, out_dir: Path, *,
                 cache_dir: str | None = None, jag_dir: str | None = None) -> tuple[int, int, int]:
    """Run icondump once (single-shot per process — see its doc comment) and
    apply the rendered/total floor. Returns (rendered, skipped, total).
    """
    out_dir.mkdir(parents=True, exist_ok=True)
    args = _icondump_args(icondump_bin, out_dir, cache_dir=cache_dir, jag_dir=jag_dir)
    result = subprocess.run(args, capture_output=True, text=True, check=True)
    rendered, skipped, total = parse_summary(result.stdout)
    if total == 0 or rendered / total < FLOOR:
        raise SystemExit(
            f"icons: rendered {rendered}/{total} — floor is {FLOOR:.0%}"
        )
    return rendered, skipped, total


def load_obj_pack(path: Path) -> dict[int, str]:
    """Parse a content tree's pack/obj.pack: `id=debugname` lines, one per
    registered obj id. This is the git-tracked source of truth for symbolic
    debugnames (data/symbols/obj.sym is a pure reformat of it — goscape
    pkg/pack/compiler/symbols_export.go:97-105).
    """
    table: dict[int, str] = {}
    for line in path.read_text(errors="replace").splitlines():
        if not line.strip():
            continue
        id_str, _, name = line.partition("=")
        table[int(id_str)] = name
    return table


_PLACEHOLDER = re.compile(r"^\[obj_(\d+)\]$")


def patch_debugnames(all_obj_path: Path, names: dict[int, str]) -> None:
    """Rewrite all.obj's `[obj_N]` placeholder headers with real symbolic
    debugnames from `names`, keyed by the EXPLICIT id N in the placeholder
    (goscape-cli unpack config writes exactly one `[obj_N]` header per obj
    id, in order — so N also equals configtext.parse_config_text's
    `_index`; that invariant is asserted here rather than assumed). An id
    with no `names` entry keeps its placeholder header.
    """
    lines = all_obj_path.read_text(errors="replace").splitlines()
    index = 0
    for i, line in enumerate(lines):
        if not (line.startswith("[") and line.endswith("]")):
            continue
        m = _PLACEHOLDER.match(line)
        if m:
            oid = int(m.group(1))
            if oid != index:
                raise SystemExit(
                    f"icons: {all_obj_path} header {line} at record "
                    f"position {index} — id/position invariant broken"
                )
            name = names.get(oid)
            if name:
                lines[i] = f"[{name}]"
        index += 1
    all_obj_path.write_text("\n".join(lines) + "\n")


def map_records(records: list[dict], total: int, icons_dir: Path, dest_dir: Path,
                spot_checks: dict[int, str] | None = None) -> set[str]:
    """Verify the id-density mapping between icondump's summary and the
    unpacked obj records (both 0..total-1, 1:1 by obj id), spot-check a few
    known ids, then copy every rendered icon to dest_dir keyed by its
    record's debugname. Returns the set of copied debugnames for
    families._obj_row's icon-cell lookup.
    """
    if len(records) != total:
        raise SystemExit(
            f"icons: {len(records)} obj records != icondump total {total}"
        )
    for index, expected in (spot_checks or {}).items():
        actual = records[index]["_debugname"]
        if actual != expected:
            raise SystemExit(
                f"icons: record _index {index} has debugname {actual!r}, "
                f"expected {expected!r}"
            )
    dest_dir.mkdir(parents=True, exist_ok=True)
    debugnames: set[str] = set()
    for rec in records:
        src = icons_dir / f"{rec['_index']}.png"
        if not src.is_file():
            continue
        debugname = rec["_debugname"]
        shutil.copyfile(src, dest_dir / f"{debugname}.png")
        debugnames.add(debugname)
    return debugnames


def map_records_by_debugname(records: list[dict], names: dict[int, str], icons_dir: Path,
                             dest_dir: Path, spot_checks: dict[str, int] | None = None,
                             floor: float = NAME_MATCH_FLOOR) -> tuple[set[str], int, int]:
    """Content-tree revisions (rev-225: no client cache exists and the
    unpack tooling postdates it — see contenttree.py) have no obj ids on
    their records at all: each record's `_debugname` is already the real
    symbolic name (the content stanza's own `[name]` header), but record
    order/`_index` carries no id meaning, so `map_records`'s
    position-keyed density check doesn't apply here.

    Instead, resolve each record's id via `names` (the content tree's
    pack/obj.pack — same `id -> debugname` shape `load_obj_pack` returns,
    inverted here), then copy `<id>.png` from icons_dir if icondump
    rendered one. A record whose debugname has no obj.pack entry (or whose
    id has no rendered icon) gets no icon; it's still counted in `total` so
    the floor reflects the true match rate. obj.pack's debugnames are
    unique per id (verified: rev-225's pack/obj.pack has zero duplicate
    values), so the inversion below is unambiguous.

    Returns (debugnames copied, matched count, total record count).
    """
    name_to_id = {name: oid for oid, name in names.items()}
    for name, expected_id in (spot_checks or {}).items():
        actual = name_to_id.get(name)
        if actual != expected_id:
            raise SystemExit(
                f"icons: obj.pack debugname {name!r} has id {actual!r}, "
                f"expected {expected_id!r}"
            )
    dest_dir.mkdir(parents=True, exist_ok=True)
    debugnames: set[str] = set()
    matched = 0
    for rec in records:
        name = rec["_debugname"]
        oid = name_to_id.get(name)
        if oid is None:
            continue
        src = icons_dir / f"{oid}.png"
        if not src.is_file():
            continue
        shutil.copyfile(src, dest_dir / f"{name}.png")
        debugnames.add(name)
        matched += 1
    total = len(records)
    if total == 0 or matched / total < floor:
        raise SystemExit(
            f"icons: matched {matched}/{total} content-tree records to "
            f"rendered icons — floor is {floor:.0%}"
        )
    return debugnames, matched, total
