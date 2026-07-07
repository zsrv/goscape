"""Extract game commands: Go cheat ladder + RuneScript debugprocs."""
import re
from pathlib import Path

from .render import GENERATED

_CASE = re.compile(r'^\s*case\s+((?:"[^"]+"\s*,?\s*)+):', re.M)
_STR = re.compile(r'"([^"]+)"')
_DEBUGPROC = re.compile(r"^\[debugproc,\s*([A-Za-z0-9_]+)\]", re.M)
_SWITCH_PARTS0 = re.compile(r"switch\s+parts\[0\]\s*\{")


def extract_func(src: str, name: str) -> str:
    marker = f") {name}("
    start = src.find(marker)
    if start == -1:
        # Fallback: a top-level function with no receiver (e.g.
        # `func handleClientCheat(p *Player, payload []byte) error {`)
        # has no ") name(" — it's "func name(" directly. rev-274's
        # handleClientCheat is registered via a gameHandlers map dispatch
        # table rather than a method on a receiver type, so it takes this
        # branch.
        marker = f"\nfunc {name}("
        start = src.index(marker)
    # start + 1: for the bare-function fallback, `marker` itself begins with
    # "\nfunc ", so searching from `start` would immediately re-match the
    # marker's own leading newline and yield an empty slice.
    end = src.find("\nfunc ", start + 1)
    return src[start:end if end != -1 else len(src)]


def _brace_body(src: str, open_brace_idx: int) -> str:
    """Given the index of an opening '{', return the substring through its
    matching closing '}' (brace-depth aware)."""
    depth = 0
    for j in range(open_brace_idx, len(src)):
        if src[j] == "{":
            depth += 1
        elif src[j] == "}":
            depth -= 1
            if depth == 0:
                return src[open_brace_idx:j + 1]
    return src[open_brace_idx:]


def _brace_depth_at(s: str, pos: int) -> int:
    """Net '{'/'}' depth of s[:pos], counting from s[0] (the switch's own
    opening '{', so a case directly inside the switch sits at depth 1)."""
    return s.count("{", 0, pos) - s.count("}", 0, pos)


def parse_go_cheats(src: str) -> list[str]:
    body = extract_func(src, "handleClientCheat")
    # Scope case-scanning to switches keyed on `parts[0]` — the cheat-name
    # dispatch switches (dev block, admin block, ungated `say` switch all
    # use `switch parts[0] {`). Some cheat arms (e.g. "setvis") contain a
    # *nested* switch on a sub-argument (`switch sub[0] { case "0": ... }`)
    # whose case labels are argument values, not command names, so on top
    # of scoping to the parts[0] switches, only cases at depth 1 (direct
    # children of the switch, not nested inside another switch/if/for
    # inside a case arm) count — this excludes the nested sub[0] arms.
    switch_matches = list(_SWITCH_PARTS0.finditer(body))
    scopes = [_brace_body(body, m.end() - 1) for m in switch_matches] or [body]
    names: set[str] = set()
    for scope in scopes:
        for m in _CASE.finditer(scope):
            if _brace_depth_at(scope, m.start()) == 1:
                names.update(_STR.findall(m.group(1)))
    return sorted(names)


def scan_debugprocs(content_dir: Path) -> list[str]:
    names: set[str] = set()
    for p in sorted(content_dir.rglob("*.rs2")):
        names.update(_DEBUGPROC.findall(p.read_text(errors="replace")))
    return sorted(names)


def render_commands_page(cheats: list[str], debugprocs: list[str],
                         out: Path) -> int:
    lines = [
        GENERATED, "",
        "# Commands", "",
        "## Engine cheat commands", "",
        "Built into the server engine (`modules/world/handlers_game.go`).",
        "Gating varies by command: most require staff mod level 2, 3, or 4,",
        "and some additionally require a non-production world; a few (like",
        "`::say`) are ungated. See `modules/world/handlers_game.go` for each",
        "command's exact gate.", "",
        "| Command |", "|---|",
        *[f"| `::{c}` |" for c in cheats],
        "",
        "## Script commands (debugprocs)", "",
        "Defined in RuneScript content as `[debugproc,name]`. Invoked with the",
        "debugproc prefix (default `~`, configurable via",
        "`world.node-debugproc-char`).", "",
        "| Command |", "|---|",
        *[f"| `~{d}` |" for d in debugprocs],
        "",
    ]
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text("\n".join(lines))
    return len(cheats) + len(debugprocs)
