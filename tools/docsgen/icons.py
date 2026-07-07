"""Item-icon rasterizer integration (rev-274 only so far).

Two independent binaries feed this step:

  * icondump (goscape-client `cmd/icondump`) renders every item-inventory
    icon in a rev's game cache to a 32x32 PNG, id-keyed (`<out>/<id>.png`).
  * goscape-cli `pack` (goscape server) packs a revision's content tree and,
    as a side effect, writes `data/symbols/*.sym` — real symbolic debugnames
    (see pkg/pack/compiler/symbols_export.go). goscape-cli `unpack config`
    (used for the family tables) has no such source and synthesizes
    placeholder `[obj_N]` headers instead, so obj.sym is the only place a
    fresh docsgen worktree can recover the real names icon files (and the
    golden rasterizer fixtures) are keyed by.

Both binaries are built fresh per run from persistent sibling worktrees —
nothing here mutates them.
"""
import os
import re
import shutil
import subprocess
from pathlib import Path

FLOOR = 0.95

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


def render_icons(icondump_bin: Path, cache_dir: str, out_dir: Path) -> tuple[int, int, int]:
    """Run icondump once (single-shot per process — see its doc comment) and
    apply the rendered/total floor. Returns (rendered, skipped, total).
    """
    out_dir.mkdir(parents=True, exist_ok=True)
    result = subprocess.run(
        [str(icondump_bin), "-cache", cache_dir, "-out", str(out_dir)],
        capture_output=True, text=True, check=True,
    )
    rendered, skipped, total = parse_summary(result.stdout)
    if total == 0 or rendered / total < FLOOR:
        raise SystemExit(
            f"icons: rendered {rendered}/{total} — floor is {FLOOR:.0%}"
        )
    return rendered, skipped, total


def build_symbols(server_cli: Path, content_dir: str, raw_dir: Path, workdir: Path) -> Path:
    """Run `goscape-cli pack` against content_dir to (re)generate real
    debugnames. data/symbols/*.sym is a pack-pipeline output artifact — a
    fresh docsgen worktree starts from a clean checkout with no such
    directory, so it must be rebuilt every run. Returns the symbols dir
    (packall writes it as a sibling of the pack -out-dir).
    """
    pack_out = workdir / "icon-symbols-pack"
    subprocess.run(
        [str(server_cli), "pack",
         "-src-dir", content_dir,
         "-out-dir", str(pack_out),
         "-raw-dir", str(raw_dir)],
        check=True,
    )
    return pack_out.parent / "symbols"


def load_obj_sym(path: Path) -> dict[int, str]:
    table: dict[int, str] = {}
    for line in path.read_text(errors="replace").splitlines():
        if not line.strip():
            continue
        id_str, _, name = line.partition("\t")
        table[int(id_str)] = name
    return table


def patch_debugnames(all_obj_path: Path, symtab: dict[int, str]) -> None:
    """Rewrite all.obj's `[obj_N]` placeholder headers with real symbolic
    debugnames from symtab, keyed by header position (0-based == obj id;
    see configtext.parse_config_text's `_index`). A record with no symtab
    entry keeps its placeholder header.
    """
    lines = all_obj_path.read_text(errors="replace").splitlines()
    index = 0
    for i, line in enumerate(lines):
        if line.startswith("[") and line.endswith("]"):
            name = symtab.get(index)
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
