"""docsgen — generate per-revision overlay pages. Run from the worktree root."""
import argparse
import tempfile
import tomllib
from pathlib import Path

from . import families, unpack

ROOT = Path(__file__).resolve().parent.parent.parent
ALL_STEPS = ["unpack", "configs", "commands", "extras", "snapshots"]

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
        if "unpack" in steps or "configs" in steps:
            all_dir = unpack.run_unpack(unpack.REPO, cfg, workdir)
            counts |= families.generate_config_families(all_dir, overlay)
            if counts["items"] < FLOOR_ITEMS:
                raise SystemExit(
                    f"rev-{rev}: only {counts['items']} items — floor is {FLOOR_ITEMS}"
                )
        # Tasks 7-8 add: commands, extras, snapshots
    return counts


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


if __name__ == "__main__":
    main()
