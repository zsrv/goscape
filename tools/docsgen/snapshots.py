"""Per-revision snapshots of rev-branch files rendered as doc pages."""
import subprocess
from pathlib import Path

from .render import GENERATED

HELM_VALUES = ["single-binary-values.yaml", "management-values.yaml",
               "world-values.yaml"]


def git_show(repo: Path, ref: str, path: str) -> str | None:
    r = subprocess.run(["git", "-C", str(repo), "show", f"{ref}:{path}"],
                       capture_output=True, text=True)
    return r.stdout if r.returncode == 0 else None


def write_snapshots(repo: Path, branch: str, overlay_docs: Path) -> None:
    lang = git_show(repo, branch, "docs/RUNESCRIPT.md")
    if lang is None:
        raise SystemExit(f"{branch}: docs/RUNESCRIPT.md missing")
    note = (f"{GENERATED}\n<!-- snapshot of {branch}:docs/RUNESCRIPT.md — "
            f"edit there, then re-run docsgen -->\n\n")
    p = overlay_docs / "runescript" / "language.md"
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(note + lang)

    ref_yaml = git_show(repo, branch, "examples/full-config-reference.yaml")
    if ref_yaml is None:
        raise SystemExit(f"{branch}: examples/full-config-reference.yaml missing")
    p = overlay_docs / "admin" / "config-reference.md"
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(
        f"{GENERATED}\n\n# Configuration reference\n\n"
        f"Every option at its default, as shipped on `{branch}`.\n\n"
        f"```yaml\n{ref_yaml}```\n"
    )

    lines = [GENERATED, "", "# Helm values examples", "",
             f"Example values files from `{branch}:production/helm/goscape/`.", ""]
    for name in HELM_VALUES:
        text = git_show(repo, branch, f"production/helm/goscape/{name}")
        lines.append(f"## {name}")
        lines.append("")
        if text is None:
            lines.append(f"_Not present on `{branch}`._")
        else:
            lines.append(f"```yaml\n{text}```")
        lines.append("")
    (overlay_docs / "admin" / "helm-values.md").write_text("\n".join(lines))
