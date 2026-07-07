from pathlib import Path

from tools.docsgen.render import (
    GENERATED, md_escape, render_chunked_family, render_nav_fragment,
)


def _recs(n):
    return [
        {"_debugname": f"obj_{i}", "_index": i, "name": f"Item {i}"}
        for i in range(n)
    ]


def _row(rec):
    return ["", rec["name"], str(rec["_index"])]


def test_md_escape():
    assert md_escape("a|b") == "a\\|b"
    assert md_escape("a\nb") == "a b"


def test_chunking_and_index(tmp_path):
    entries = render_chunked_family(
        _recs(503), "items", "Items", ["Icon", "Name", "Id"], _row,
        tmp_path / "player" / "items", chunk=500,
    )
    files = sorted(p.name for p in (tmp_path / "player" / "items").iterdir())
    assert files == ["index.md", "page-001.md", "page-002.md"]
    assert len(entries) == 2
    assert entries[0][1] == "player/items/page-001.md"
    page1 = (tmp_path / "player" / "items" / "page-001.md").read_text()
    assert page1.startswith(GENERATED)
    assert "| Icon | Name | Id |" in page1
    assert page1.count("| Item ") == 500
    index = (tmp_path / "player" / "items" / "index.md").read_text()
    assert "Item 0" in index and "Item 502" in index


def test_nav_fragment_shape(tmp_path):
    frag = render_nav_fragment([
        ("Items", "player/items/index.md",
         [("page 1 (A – B)", "player/items/page-001.md")]),
    ])
    assert frag == (
        "      - Items:\n"
        "          - player/items/index.md\n"
        '          - "page 1 (A – B)": player/items/page-001.md'
    )


def test_deterministic(tmp_path):
    a, b = tmp_path / "a", tmp_path / "b"
    render_chunked_family(_recs(10), "items", "Items",
                          ["Icon", "Name", "Id"], _row, a)
    render_chunked_family(_recs(10), "items", "Items",
                          ["Icon", "Name", "Id"], _row, b)
    assert (a / "page-001.md").read_bytes() == (b / "page-001.md").read_bytes()
