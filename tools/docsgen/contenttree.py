"""Synthesize unpack-style all.* inputs from a pinned content tree.

rev-225 predates goscape-cli's unpack tooling and no rev-225 client cache
exists (that era's pack pipeline shipped .jag archives, not
main_file_cache.*), so its config data is parsed straight from the content
repo's scripts tree. This includes the upstream `scripts/_unpack` staging
dumps, which hold configs never migrated into named source files — the two
sets are disjoint and together cover the full config space.
"""
from pathlib import Path

FAMILY_EXTS = ("obj", "npc", "loc", "varp")


def synthesize_all_dir(content_dir: Path, out_dir: Path) -> Path:
    """Concatenate every ``*.<ext>`` under ``<content_dir>/scripts`` into
    ``out_dir/all.<ext>`` for each config family, sorted by full path for
    determinism, file contents joined with a blank line."""
    scripts = content_dir / "scripts"
    out_dir.mkdir(parents=True, exist_ok=True)
    for ext in FAMILY_EXTS:
        files = sorted(scripts.rglob(f"*.{ext}"), key=str)
        text = "\n\n".join(
            f.read_text(errors="replace").rstrip("\n") for f in files
        )
        (out_dir / f"all.{ext}").write_text(text + "\n")
    return out_dir
