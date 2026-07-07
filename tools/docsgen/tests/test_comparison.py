from tools.docsgen.__main__ import comparison_lines

SUMMARY = {
    "225": {"items": 2000, "npcs": 800, "locs": 2500},
    "274": {"items": 3894, "npcs": 1359, "locs": 4671},
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
