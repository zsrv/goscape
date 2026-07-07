#!/usr/bin/env python3
"""Assemble per-revision docs trees and deploy them as mike versions.

Usage (from the worktree root, venv active or via .venv/bin/python):
  python tools/build.py assemble --revision 274
  python tools/build.py deploy   --revision 274
  python tools/build.py all          # assemble + deploy every revision, set default
"""
import argparse
import shutil
import string
import subprocess
import sys
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BUILD_DIR = ROOT / ".build"
REVISIONS_TOML = ROOT / "tools" / "docsgen" / "revisions.toml"

CANONICAL_PAGES = [
    ("REFERENCES.md", "contributor/references-pins.md"),
    ("PORTING-LESSONS.md", "contributor/porting-lessons.md"),
]


def load_revisions() -> dict:
    with open(REVISIONS_TOML, "rb") as f:
        return tomllib.load(f)


def assemble(rev: str, cfg: dict) -> Path:
    stage = BUILD_DIR / f"rev-{rev}"
    if stage.exists():
        shutil.rmtree(stage)
    stage.mkdir(parents=True)
    shutil.copytree(ROOT / "docs", stage / "docs")
    shutil.copytree(ROOT / "overrides", stage / "overrides")

    overlay = ROOT / "overlays" / f"rev-{rev}"
    if overlay.exists():
        for src in sorted(overlay.rglob("*")):
            if not src.is_file() or src.name.startswith("_"):
                continue
            dst = stage / "docs" / src.relative_to(overlay)
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src, dst)

    for src_name, dst_rel in CANONICAL_PAGES:
        text = (ROOT / src_name).read_text()
        note = f"<!-- assembled from {src_name} on main; edit that file, not this page -->\n"
        dst = stage / "docs" / dst_rel
        dst.parent.mkdir(parents=True, exist_ok=True)
        dst.write_text(note + text)

    tmpl = string.Template((ROOT / "mkdocs.yml.tmpl").read_text())
    (stage / "mkdocs.yml").write_text(
        tmpl.substitute(revision=rev, revision_branch=cfg["branch"])
    )
    return stage


def deploy(rev: str, aliases: list[str]) -> None:
    cfg_file = BUILD_DIR / f"rev-{rev}" / "mkdocs.yml"
    if not cfg_file.exists():
        sys.exit(f"error: {cfg_file} missing — run assemble first")
    cmd = ["mike", "deploy", "-F", str(cfg_file)]
    if aliases:
        cmd.append("--update-aliases")
    cmd += [f"rev-{rev}", *aliases]
    subprocess.run(cmd, cwd=ROOT, check=True)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("command", choices=["assemble", "deploy", "all"])
    ap.add_argument("--revision")
    args = ap.parse_args()
    data = load_revisions()
    revisions = data["revisions"]

    if args.command == "all":
        for rev in data["order"]:
            assemble(rev, revisions[rev])
            deploy(rev, ["latest"] if rev == data["latest"] else [])
        latest_cfg = BUILD_DIR / f"rev-{data['latest']}" / "mkdocs.yml"
        subprocess.run(
            ["mike", "set-default", "-F", str(latest_cfg), "latest"],
            cwd=ROOT, check=True,
        )
        return

    rev = args.revision
    if rev not in revisions:
        sys.exit(f"error: unknown revision {rev!r}; known: {list(revisions)}")
    if args.command == "assemble":
        print(assemble(rev, revisions[rev]))
    else:
        deploy(rev, ["latest"] if rev == data["latest"] else [])


if __name__ == "__main__":
    main()
