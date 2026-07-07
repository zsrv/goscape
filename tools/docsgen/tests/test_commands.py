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


def test_scan_debugprocs(tmp_path):
    (tmp_path / "a.rs2").write_text("[debugproc,coords]\nmes(coord);\n")
    sub = tmp_path / "sub"
    sub.mkdir()
    (sub / "b.rs2").write_text("[if_button,x]\n\n[debugproc,fly]\n")
    assert scan_debugprocs(tmp_path) == ["coords", "fly"]
