"""Build a rev branch's goscape-cli and run `unpack config` against its cache."""
import os
import subprocess
import sys
from pathlib import Path

REPO = Path("/home/owner/Code/github.com/zsrv/goscape")


def _go_env() -> dict:
    tmp = os.environ.get("TMPDIR", "/tmp")
    return os.environ | {
        "CGO_ENABLED": "0",
        "GOPATH": f"{tmp}/go",
        "GOCACHE": f"{tmp}/go-cache",
    }


def add_worktree(repo: Path, branch: str, wt: Path) -> None:
    subprocess.run(
        ["git", "-C", str(repo), "worktree", "add", "--force",
         str(wt), branch],
        check=True,
    )


def remove_worktree(repo: Path, wt: Path) -> None:
    result = subprocess.run(
        ["git", "-C", str(repo), "worktree", "remove", "--force", str(wt)],
        check=False,
    )
    if result.returncode != 0:
        print(
            f"docsgen: warning: failed to remove worktree at {wt} "
            f"(git exited {result.returncode}); remove it manually with "
            f"'git -C {repo} worktree remove --force {wt}' "
            "or 'git worktree prune'.",
            file=sys.stderr,
        )


def run_unpack(cfg: dict, workdir: Path, wt: Path) -> Path:
    """Build goscape-cli from an already-checked-out worktree `wt` and run
    `unpack config` against the revision's cache. Worktree lifecycle
    (add/remove) is owned by the caller (run_revision) so the checkout can
    outlive this call for later steps (e.g. `commands`) that also need to
    read files out of it.
    """
    src = workdir / "src"
    src.mkdir(parents=True, exist_ok=True)
    cli = workdir / "goscape-cli"
    subprocess.run(
        ["go", "build", "-trimpath", "-o", str(cli), "./cmd/goscape-cli"],
        cwd=wt, env=_go_env(), check=True,
    )
    subprocess.run(
        [str(cli), "unpack", "config",
         "-cache-dir", cfg["cache_dir"],
         "-src-dir", str(src),
         "-revision", str(cfg["unpack_revision"])],
        check=True,
    )
    out = src / "scripts" / "_unpack" / str(cfg["unpack_revision"])
    if not (out / "all.obj").exists():
        raise SystemExit(f"unpack produced no all.obj under {out}")
    return out
