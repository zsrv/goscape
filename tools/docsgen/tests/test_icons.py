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


def test_patch_debugnames_by_position(tmp_path):
    all_obj = tmp_path / "all.obj"
    all_obj.write_text("[obj_0]\nname=Knife\n\n[obj_1]\nname=Fur\n")
    icons.patch_debugnames(all_obj, {0: "knife"})
    text = all_obj.read_text()
    assert "[knife]" in text
    assert "[obj_1]" in text  # untouched — no symtab entry for id 1


def test_parse_summary():
    assert icons.parse_summary("rendered=3894 skipped=0 total=3894\n") == (3894, 0, 3894)


def test_parse_summary_missing_line():
    with pytest.raises(SystemExit):
        icons.parse_summary("some other output\n")
