"""Where docsgen looks for the checkouts it reads.

docsgen builds per-revision pages out of three things it does not own: the
goscape repository's rev branches, the pinned Lost City content trees, and the
goscape-client worktrees the icon rasterizer runs in. Those live outside this
repository, so revisions.toml names them *relative* to a source root rather
than by absolute path — otherwise the file only works on the machine that
wrote it.

The default assumes the conventional layout, with this repository at
``<src root>/zsrv/goscape-docs`` (or any worktree two levels down), so nothing
needs configuring for a standard checkout. Set ``GOSCAPE_SRC_ROOT`` to point
elsewhere.
"""
import os
from pathlib import Path

# tools/docsgen/paths.py -> tools/docsgen -> tools -> <repo root>
REPO_ROOT = Path(__file__).resolve().parent.parent.parent

SRC_ROOT = Path(os.environ.get("GOSCAPE_SRC_ROOT") or REPO_ROOT.parent.parent)
