from tools.docsgen.commands import extract_func, parse_go_cheats, scan_debugprocs

GO_SRC = '''
package world

func (c *client) handlePacket(op int) {
\tswitch op {
\tcase 1:
\t\tc.doThing("not_a_cheat")
\t}
}

func (c *client) handleClientCheat(text string) {
\tswitch parts[0] {
\tcase "tele":
\t\tc.tele()
\tcase "give", "givemany":
\t\tc.give()
\tcase "setstat":
\t\tc.setStat()
\t}
}

func (c *client) other() {
\tswitch x {
\tcase "not_in_cheat_func":
\t}
}
'''


def test_extract_func_bounds():
    body = extract_func(GO_SRC, "handleClientCheat")
    assert "tele" in body
    assert "not_in_cheat_func" not in body
    assert "not_a_cheat" not in body


def test_parse_go_cheats():
    assert parse_go_cheats(GO_SRC) == ["give", "givemany", "setstat", "tele"]


# rev-274's real handleClientCheat is a bare package-level function (no
# receiver), so extract_func must take its "\nfunc name(" fallback path.
# It also nests a `switch sub[0] { case "0": ... }` inside the "setvis"
# cheat arm — those numeric labels are argument values, not commands.
GO_SRC_BARE = '''
package world

func handlePacket(p *Player, op int) {
\tswitch op {
\tcase 1:
\t\tp.doThing("not_a_cheat")
\t}
}

func handleClientCheat(p *Player, payload []byte) error {
\tif p.staffModLevel >= 4 {
\t\tswitch parts[0] {
\t\tcase "tele":
\t\t\tp.tele()
\t\tcase "setvis":
\t\t\tswitch sub[0] {
\t\t\tcase "0":
\t\t\t\tp.setVis(0)
\t\t\tcase "1", "2":
\t\t\t\tp.setVis(1)
\t\t\t}
\t\tcase "after_nested":
\t\t\tp.afterNested()
\t\t}
\t}
\tswitch parts[0] {
\tcase "say":
\t\tp.say()
\t}
\treturn nil
}

func other(p *Player) {
\tswitch x {
\tcase "not_in_cheat_func":
\t}
}
'''


def test_extract_func_bare_function_fallback():
    body = extract_func(GO_SRC_BARE, "handleClientCheat")
    assert "tele" in body
    assert "not_a_cheat" not in body
    assert "not_in_cheat_func" not in body


def test_extract_func_bare_function_at_eof():
    src = GO_SRC_BARE[:GO_SRC_BARE.index("\nfunc other")] + "\n"
    body = extract_func(src, "handleClientCheat")
    assert "tele" in body
    assert "return nil" in body


def test_parse_go_cheats_excludes_nested_switch_labels():
    cheats = parse_go_cheats(GO_SRC_BARE)
    # Numeric labels from the nested `switch sub[0]` must not leak in...
    assert "0" not in cheats
    assert "1" not in cheats
    assert "2" not in cheats
    # ...and a case arm AFTER the nested switch must still be captured
    # (pins the brace-depth bookkeeping across the nested block).
    assert cheats == ["after_nested", "say", "setvis", "tele"]


def test_scan_debugprocs(tmp_path):
    (tmp_path / "a.rs2").write_text("[debugproc,coords]\nmes(coord);\n")
    sub = tmp_path / "sub"
    sub.mkdir()
    (sub / "b.rs2").write_text("[if_button,x]\n\n[debugproc,fly]\n")
    assert scan_debugprocs(tmp_path) == ["coords", "fly"]
