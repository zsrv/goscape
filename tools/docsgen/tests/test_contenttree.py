from tools.docsgen.contenttree import synthesize_all_dir


def _write(path, text):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


def _fixture_tree(root):
    c = root / "content"
    _write(c / "scripts" / "b" / "late.obj", "[zebra]\nname=Zebra\n")
    _write(c / "scripts" / "a" / "early.obj", "[axe]\nname=Axe\n\n")
    _write(c / "scripts" / "_unpack" / "all.obj", "[misc]\nname=Misc\n")
    _write(c / "scripts" / "a" / "vars.varp", "[varp_1]\nscope=perm\n")
    return c


def test_concatenation_sorted_by_full_path_joined_with_blank_line(tmp_path):
    out = synthesize_all_dir(_fixture_tree(tmp_path), tmp_path / "all_dir")
    # "_unpack" < "a" < "b" by path; trailing newlines normalized to exactly
    # one blank line between files and a single trailing newline.
    assert (out / "all.obj").read_text() == (
        "[misc]\nname=Misc\n\n[axe]\nname=Axe\n\n[zebra]\nname=Zebra\n"
    )
    assert (out / "all.varp").read_text() == "[varp_1]\nscope=perm\n"


def test_deterministic_across_runs(tmp_path):
    c = _fixture_tree(tmp_path)
    first = synthesize_all_dir(c, tmp_path / "run1")
    second = synthesize_all_dir(c, tmp_path / "run2")
    for name in ("all.obj", "all.npc", "all.loc", "all.varp"):
        assert (first / name).read_text() == (second / name).read_text()


def test_family_with_no_source_files_yields_empty_input(tmp_path):
    out = synthesize_all_dir(_fixture_tree(tmp_path), tmp_path / "all_dir")
    # No *.npc files in the fixture: all.npc exists but parses to no records.
    assert (out / "all.npc").read_text() == "\n"
