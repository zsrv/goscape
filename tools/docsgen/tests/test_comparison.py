from tools.docsgen.__main__ import comparison_lines

SUMMARY = {
    # Deliberately asymmetric: 225 has no "icons" key (pins the blank-cell
    # path), 274 has one (pins the populated-cell path) — same as real
    # per-revision counts before/after a revision's icons step is enabled.
    "225": {"items": 2000, "npcs": 800, "locs": 2500},
    "274": {"items": 3894, "icons": 3894, "npcs": 1359, "locs": 4671},
}


def test_note_renders_as_footnote():
    lines = comparison_lines(SUMMARY, {"225": "decoded with the rev-244 CLI"})
    assert lines[-1] == "- **rev-225:** decoded with the rev-244 CLI"
    assert lines[-2] == ""  # blank line separates footnotes from the table


def test_no_notes_no_footnotes():
    lines = comparison_lines(SUMMARY, {})
    assert lines[-1].startswith("| rev-274 |")
    assert all(not line.startswith("- **") for line in lines)


def test_note_for_unsummarized_revision_ignored():
    lines = comparison_lines(SUMMARY, {"999": "not in summary"})
    assert all("999" not in line for line in lines)


def test_icons_column_header():
    lines = comparison_lines(SUMMARY, {})
    header = next(l for l in lines if l.startswith("| Revision"))
    assert "Icons" in header


def test_icons_column_populated_and_blank_cells():
    lines = comparison_lines(SUMMARY, {})
    row_274 = next(l for l in lines if l.startswith("| rev-274"))
    row_225 = next(l for l in lines if l.startswith("| rev-225"))
    cells_274 = [c.strip() for c in row_274.split("|")]
    cells_225 = [c.strip() for c in row_225.split("|")]
    assert cells_274[2:4] == ["3894", "3894"]  # Items, Icons
    assert cells_225[2:4] == ["2000", ""]  # Items, Icons (blank)
