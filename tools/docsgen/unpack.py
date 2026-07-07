"""Build a rev branch's goscape-cli and run `unpack config` against its cache."""
import os
import subprocess
from pathlib import Path

REPO = Path("/home/owner/Code/github.com/zsrv/goscape")


def _go_env() -> dict:
    tmp = os.environ.get("TMPDIR", "/tmp")
    return os.environ | {
        "CGO_ENABLED": "0",
        "GOPATH": f"{tmp}/go",
        "GOCACHE": f"{tmp}/go-cache",
    }


def run_unpack(repo: Path, cfg: dict, workdir: Path) -> Path:
    wt = workdir / "worktree"
    src = workdir / "src"
    src.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        ["git", "-C", str(repo), "worktree", "add", "--force",
         str(wt), cfg["branch"]],
        check=True,
    )
    try:
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
    finally:
        subprocess.run(
            ["git", "-C", str(repo), "worktree", "remove", "--force", str(wt)],
            check=False,
        )
    out = src / "scripts" / "_unpack" / str(cfg["unpack_revision"])
    if not (out / "all.obj").exists():
        raise SystemExit(f"unpack produced no all.obj under {out}")
    return out
