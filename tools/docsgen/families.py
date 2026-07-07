"""Turn unpacked all.* config text into reference pages for one revision."""
from pathlib import Path

from .configtext import parse_config_text
from .render import GENERATED, md_escape, render_chunked_family, render_nav_fragment


def _s(rec, key, default=""):
    v = rec.get(key, default)
    return v if isinstance(v, str) else ", ".join(v)


def _ops(rec, prefix):
    return ", ".join(
        _s(rec, f"{prefix}{i}") for i in range(1, 6) if rec.get(f"{prefix}{i}")
    )


def _obj_row(rec, icons=None):
    debugname = rec["_debugname"]
    name = _s(rec, "name") or debugname
    icon_cell = f"![{name}](icons/{debugname}.png)" if icons and debugname in icons else ""
    return [
        icon_cell,
        name,
        _s(rec, "desc"),
        _s(rec, "cost", "0"),
        "yes" if _s(rec, "members") == "yes" else "",
        "yes" if _s(rec, "stackable") == "yes" else "",
        _ops(rec, "op") or _ops(rec, "iop"),
    ]


def _npc_row(rec):
    return [
        _s(rec, "name") or rec["_debugname"],
        _s(rec, "desc"),
        _s(rec, "vislevel"),
        _ops(rec, "op"),
    ]


def _loc_row(rec):
    return [
        _s(rec, "name") or rec["_debugname"],
        _s(rec, "desc"),
        _ops(rec, "op"),
    ]


def generate_config_families(all_dir: Path, overlay_docs: Path,
                             icons: set[str] | None = None) -> dict[str, int]:
    counts: dict[str, int] = {}
    sections = []
    plans = [
        ("items", "all.obj", "Items",
         ["Icon", "Name", "Description", "Cost", "Members", "Stackable", "Options"],
         lambda rec: _obj_row(rec, icons)),
        ("npcs", "all.npc", "NPCs",
         ["Name", "Description", "Level", "Options"], _npc_row),
        ("locs", "all.loc", "Locations",
         ["Name", "Description", "Options"], _loc_row),
    ]
    for family, fname, title, columns, row_fn in plans:
        records = parse_config_text((all_dir / fname).read_text(errors="replace"))
        counts[family] = len(records)
        entries = render_chunked_family(
            records, family, title, columns, row_fn,
            overlay_docs / "player" / family,
        )
        sections.append((title, f"player/{family}/index.md", entries))

    varps = parse_config_text((all_dir / "all.varp").read_text(errors="replace"))
    varbits_path = all_dir / "all.varbit"
    varbits = (
        parse_config_text(varbits_path.read_text(errors="replace"))
        if varbits_path.exists() else []
    )
    counts["varps"], counts["varbits"] = len(varps), len(varbits)
    lines = [GENERATED, "", "# Varps & varbits", ""]
    lines += [f"{len(varps)} varps, {len(varbits)} varbits.", ""]
    for title2, recs in (("Varps", varps), ("Varbits", varbits)):
        if not recs:
            continue
        lines += [f"## {title2}", "", "| Id | Debug name | Config |", "|---|---|---|"]
        for r in recs:
            extras = ", ".join(
                f"{k}={md_escape(str(v))}" for k, v in sorted(r.items())
                if not k.startswith("_")
            )
            lines.append(f"| {r['_index']} | {md_escape(r['_debugname'])} | {extras} |")
        lines.append("")
    rs_dir = overlay_docs / "runescript"
    rs_dir.mkdir(parents=True, exist_ok=True)
    (rs_dir / "varps.md").write_text("\n".join(lines))

    (overlay_docs / "_nav_generated.yml").write_text(
        render_nav_fragment(sections) + "\n"
    )
    return counts
