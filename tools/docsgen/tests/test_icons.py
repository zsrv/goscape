from pathlib import Path

import pytest

from tools.docsgen import icons
from tools.docsgen.configtext import parse_config_text
from tools.docsgen.families import _obj_row

OBJ_FIXTURE = """\
[obj_0]
name=Knife
cost=6

[obj_1]
name=Fur
cost=10
"""


def test_obj_row_icon_cell():
    recs = parse_config_text(OBJ_FIXTURE)
    recs[0]["_debugname"] = "knife"  # patched real debugname (id 0)
    icons_set = {"knife"}
    assert _obj_row(recs[0], icons_set)[0] == "![Knife](icons/knife.png)"
    assert _obj_row(recs[1], icons_set)[0] == ""


def test_icon_mapping_density_mismatch():
    records = [{"_debugname": "a", "_index": 0}, {"_debugname": "b", "_index": 1}]
    with pytest.raises(SystemExit):
        icons.map_records(records, total=3, icons_dir=Path("/nonexistent"),
                          dest_dir=Path("/nonexistent"))


def test_icon_mapping_spot_check_mismatch(tmp_path):
    records = [{"_debugname": "wrong_name", "_index": 0}]
    with pytest.raises(SystemExit):
        icons.map_records(records, total=1, icons_dir=tmp_path, dest_dir=tmp_path,
                          spot_checks={0: "knife"})


def test_icon_mapping_maps_by_index(tmp_path):
    icons_dir = tmp_path / "rendered"
    icons_dir.mkdir()
    (icons_dir / "0.png").write_bytes(b"PNG0")
    (icons_dir / "1.png").write_bytes(b"PNG1")
    # id 2 has no rendered icon (icondump skip) — must not appear in output.
    records = [
        {"_debugname": "knife", "_index": 0},
        {"_debugname": "bronze_dagger", "_index": 1},
        {"_debugname": "obj_2", "_index": 2},
    ]
    dest = tmp_path / "dest"
    result = icons.map_records(records, total=3, icons_dir=icons_dir, dest_dir=dest,
                               spot_checks={1: "bronze_dagger"})
    assert result == {"knife", "bronze_dagger"}
    assert (dest / "knife.png").read_bytes() == b"PNG0"
    assert (dest / "bronze_dagger.png").read_bytes() == b"PNG1"
    assert not (dest / "obj_2.png").exists()


def test_patch_debugnames_by_explicit_id(tmp_path):
    all_obj = tmp_path / "all.obj"
    all_obj.write_text("[obj_0]\nname=Knife\n\n[obj_1]\nname=Fur\n")
    icons.patch_debugnames(all_obj, {0: "knife"})
    text = all_obj.read_text()
    assert "[knife]" in text
    assert "[obj_1]" in text  # untouched — no obj.pack entry for id 1


def test_patch_debugnames_id_position_invariant(tmp_path):
    all_obj = tmp_path / "all.obj"
    # Placeholder id 5 at record position 0 — unpack config never emits
    # this (one header per id, in order), so it must hard-fail rather than
    # silently mis-key the icon mapping.
    all_obj.write_text("[obj_5]\nname=X\n")
    with pytest.raises(SystemExit):
        icons.patch_debugnames(all_obj, {5: "x"})


def test_load_obj_pack(tmp_path):
    p = tmp_path / "obj.pack"
    p.write_text("0=knife\n1205=bronze_dagger\n")
    assert icons.load_obj_pack(p) == {0: "knife", 1205: "bronze_dagger"}


def test_obj_row_name_falls_back_to_patched_debugname():
    # Pins the APPROVED page-wide side effect of debugname patching: a
    # record with no name= field shows its real (patched) debugname in the
    # Name column instead of an obj_N placeholder.
    recs = parse_config_text("[bronze_dagger]\ncost=10\n")
    row = _obj_row(recs[0], {"bronze_dagger"})
    assert row[1] == "bronze_dagger"
    assert row[0] == "![bronze_dagger](icons/bronze_dagger.png)"


def test_parse_summary():
    assert icons.parse_summary("rendered=3894 skipped=0 total=3894\n") == (3894, 0, 3894)


def test_parse_summary_missing_line():
    with pytest.raises(SystemExit):
        icons.parse_summary("some other output\n")


def test_icondump_args_cache_dir():
    args = icons._icondump_args(Path("/bin/icondump"), Path("/out"), cache_dir="/cache")
    assert args == ["/bin/icondump", "-cache", "/cache", "-out", "/out"]


def test_icondump_args_jag_dir():
    # rev-225: no client cache exists (jag-era pack pipeline); icondump
    # takes the raw jag archive dir instead of -cache.
    args = icons._icondump_args(Path("/bin/icondump"), Path("/out"), jag_dir="/jags")
    assert args == ["/bin/icondump", "-jag-dir", "/jags", "-out", "/out"]


def test_icondump_args_requires_exactly_one():
    with pytest.raises(SystemExit):
        icons._icondump_args(Path("/bin/icondump"), Path("/out"))
    with pytest.raises(SystemExit):
        icons._icondump_args(Path("/bin/icondump"), Path("/out"),
                             cache_dir="/c", jag_dir="/j")


def test_map_by_debugname_unmatched_record_gets_no_icon_and_is_counted(tmp_path):
    # rev-225 (config_source = content-tree): records carry no obj id — the
    # header IS already the real debugname. Resolve id via the content
    # tree's pack/obj.pack (debugname -> id, inverted); a record whose
    # debugname isn't in obj.pack gets no icon and is counted in `total`
    # (lowering the match rate) but not in `matched`.
    icons_dir = tmp_path / "rendered"
    icons_dir.mkdir()
    (icons_dir / "946.png").write_bytes(b"PNG-KNIFE")
    records = [
        {"_debugname": "knife"},
        {"_debugname": "no_such_item"},  # absent from obj.pack
    ]
    names = {946: "knife"}  # id -> debugname, same shape as load_obj_pack()
    dest = tmp_path / "dest"
    copied, matched, total = icons.map_records_by_debugname(
        records, names, icons_dir, dest, floor=0.0,
    )
    assert copied == {"knife"}
    assert matched == 1
    assert total == 2
    assert (dest / "knife.png").read_bytes() == b"PNG-KNIFE"
    assert not (dest / "no_such_item.png").exists()


def test_map_by_debugname_floor_gate(tmp_path):
    icons_dir = tmp_path / "rendered"
    icons_dir.mkdir()
    (icons_dir / "1.png").write_bytes(b"PNG")
    records = [
        {"_debugname": "matched"},
        {"_debugname": "a"}, {"_debugname": "b"}, {"_debugname": "c"},
    ]
    names = {1: "matched"}
    # matched/total = 1/4 = 25% < the 90% floor -> SystemExit.
    with pytest.raises(SystemExit):
        icons.map_records_by_debugname(records, names, icons_dir, tmp_path / "dest")


def test_map_by_debugname_spot_check_mismatch(tmp_path):
    with pytest.raises(SystemExit):
        icons.map_records_by_debugname(
            [{"_debugname": "knife"}], {946: "knife"}, tmp_path, tmp_path,
            spot_checks={"knife": 999},
        )
