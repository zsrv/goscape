from tools.docsgen.configtext import parse_config_text

FIXTURE = """\
[knife]
name=Knife
desc=A dangerous looking knife.
cost=6
model=model_2559_obj
2dzoom=580
manwear=model_4045,0

[fur]
name=Fur
desc=This would make warm clothing.
cost=10
members=yes
op1=Take
op2=Examine
"""


def test_records_in_order_with_debugname_and_index():
    recs = parse_config_text(FIXTURE)
    assert [(r["_debugname"], r["_index"]) for r in recs] == [("knife", 0), ("fur", 1)]


def test_key_values():
    recs = parse_config_text(FIXTURE)
    assert recs[0]["name"] == "Knife"
    assert recs[0]["cost"] == "6"
    assert recs[1]["members"] == "yes"
    assert recs[1]["op1"] == "Take"


def test_repeated_keys_become_lists():
    recs = parse_config_text("[x]\nrecol1s=1\nrecol1s=2\n")
    assert recs[0]["recol1s"] == ["1", "2"]


def test_blank_lines_and_comments_ignored():
    recs = parse_config_text("// header comment\n\n[a]\nname=A\n\n// tail\n")
    assert len(recs) == 1 and recs[0]["name"] == "A"
