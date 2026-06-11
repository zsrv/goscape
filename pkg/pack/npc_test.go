package pack

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// npcTestRegistries returns the four name-map packs + a paramTypes
// fixture used by npc parser/packer tests.
func npcTestRegistries(t *testing.T) (modelPack, categoryPack, seqPack, huntPack *PackFile, paramTypes *objtype.ParamTypeConfigs, lk *paramLookups) {
	t.Helper()
	modelPack = newTestPF("model", map[int]string{
		0: "rat_body",
		1: "rat_head",
	})
	categoryPack = newTestPF("category", map[int]string{
		0: "monster",
	})
	seqPack = newTestPF("seq", map[int]string{
		0: "walk",
		1: "attack",
		2: "idle",
		3: "turn",
	})
	huntPack = newTestPF("hunt", map[int]string{
		0: "default_hunt",
	})
	intParam := &objtype.ParamType{Type: objtype.ScriptVarTypeInt}
	intParam.ID = 4
	intParam.DebugName = "aggression"
	strParam := &objtype.ParamType{Type: objtype.ScriptVarTypeString}
	strParam.ID = 3
	strParam.DebugName = "label"
	paramTypes = &objtype.ParamTypeConfigs{
		ConfigNames: map[string]int{"aggression": 4, "label": 3},
		Configs:     []*objtype.ParamType{nil, nil, nil, strParam, intParam},
	}
	lk = &paramLookups{}
	return
}

// ── Parser tests ────────────────────────────────────────────────────────────

func TestParseNpcConfig_Name(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("name", "Giant Rat")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("name key should be accepted")
	}
	s, ok := val.(string)
	if !ok || s != "Giant Rat" {
		t.Fatalf("got %#v, want string \"Giant Rat\"", val)
	}
}

func TestParseNpcConfig_Huntmode(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("huntmode", "default_hunt")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("huntmode key should be accepted")
	}
	idx, ok := val.(int)
	if !ok || idx != 0 {
		t.Fatalf("got %#v, want int 0", val)
	}
}

func TestParseNpcConfig_Param(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("param", "aggression,2")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("param key should be accepted")
	}
	pv, ok := val.(ParamValue)
	if !ok {
		t.Fatalf("got %#v, want ParamValue", val)
	}
	if pv.ID != 4 {
		t.Fatalf("got ID=%d, want 4", pv.ID)
	}
	if pv.Type != objtype.ScriptVarTypeInt {
		t.Fatalf("got Type=%d, want Int", pv.Type)
	}
	iv, ok := pv.Value.(int)
	if !ok || iv != 2 {
		t.Fatalf("got Value=%#v, want int 2", pv.Value)
	}
}

func TestParseNpcConfig_UnknownKey(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("zzz_unknown", "value")
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("unknown key should NOT be accepted")
	}
	if val != nil {
		t.Fatalf("got %#v, want nil", val)
	}
}

func TestParseNpcConfig_Turnspeed(t *testing.T) {
	// rev-254: turnspeed joined numberKeys (TS NpcConfig.ts:27 @ 2e3bcf43).
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("turnspeed", "16")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("turnspeed key should be accepted")
	}
	if val.(int) != 16 {
		t.Fatalf("got %#v, want 16", val)
	}

	if _, _, err := parse("turnspeed", "abc"); err == nil {
		t.Fatal("want err for non-numeric turnspeed")
	}
}

func TestParseNpcConfig_Moverestrict(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("moverestrict", "passthru")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("moverestrict key should be accepted")
	}
	if val.(int) != objtype.MoveRestrictPassthru {
		t.Fatalf("got %#v, want MoveRestrictPassthru(6)", val)
	}
}

func TestParseNpcConfig_Blockwalk(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("blockwalk", "NPC")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("blockwalk key should be accepted")
	}
	if val.(int) != objtype.BlockWalkNPC {
		t.Fatalf("got %#v, want BlockWalkNPC(1)", val)
	}
}

func TestParseNpcConfig_Defaultmode(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("defaultmode", "wander")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("defaultmode key should be accepted")
	}
	if val.(int) != objtype.NPCModeWander {
		t.Fatalf("got %#v, want NPCModeWander(1)", val)
	}
}

func TestParseNpcConfig_VislevelHide(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("vislevel", "hide")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("vislevel key should be accepted")
	}
	if val.(int) != 0 {
		t.Fatalf("got %#v, want int 0", val)
	}
}

func TestParseNpcConfig_WalkanimList(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("walkanim", "walk,attack,idle,turn")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("walkanim key should be accepted")
	}
	arr, ok := val.([]int)
	if !ok {
		t.Fatalf("got %#v, want []int", val)
	}
	want := []int{0, 1, 2, 3}
	if len(arr) != 4 {
		t.Fatalf("got len=%d, want 4", len(arr))
	}
	for i := range want {
		if arr[i] != want[i] {
			t.Fatalf("arr[%d]: got %d, want %d", i, arr[i], want[i])
		}
	}
}

// ── Packer tests ────────────────────────────────────────────────────────────
//
// Each packer test uses a single-slot npcPack fixture; the per-id frame
// is the 2-byte size header + body + Next() 0x00 terminator.
//
// Note: when a config block exists, ALL slots emit:
//   - name trailer (opcode 2 + pjstr(name|debugname))   on client
//   - vislevel default (opcode 95 + p2(1))              on client, if no vislevel was set
//   - debugname trailer (opcode 250 + pjstr(debugname)) on server (when nonempty)
// Tests below include these trailers in the expected byte sequences.

func npcOneSlotPack(name string) *PackFile {
	return newTestPF("npc", map[int]string{0: name})
}

// npcClientDefaultTrailer returns the always-emitted client trailer for a
// configured slot with no explicit name or vislevel: opcode 2 + pjstr(debugname)
// + opcode 95 + p2(1).
func npcClientDefaultTrailer(debugname string) []byte {
	buf := []byte{0x02}
	buf = append(buf, []byte(debugname)...)
	buf = append(buf, 0x0A)
	buf = append(buf, 0x5F, 0x00, 0x01) // 95 + p2(1)
	return buf
}

// npcServerDebugTrailer returns the always-emitted server trailer when
// debugname is nonempty: opcode 250 + pjstr(debugname).
func npcServerDebugTrailer(debugname string) []byte {
	buf := []byte{0xFA}
	buf = append(buf, []byte(debugname)...)
	buf = append(buf, 0x0A)
	return buf
}

// expectClient assembles: 2-byte size header (0x00 0x01) + body + default
// client trailer (name+vislevel-default) + Next 0x00.
func expectClient(debugname string, body ...byte) []byte {
	out := []byte{0x00, 0x01}
	out = append(out, body...)
	out = append(out, npcClientDefaultTrailer(debugname)...)
	out = append(out, 0x00)
	return out
}

// expectServer assembles: 2-byte size header + body + debugname trailer + Next 0x00.
func expectServer(debugname string, body ...byte) []byte {
	out := []byte{0x00, 0x01}
	out = append(out, body...)
	out = append(out, npcServerDebugTrailer(debugname)...)
	out = append(out, 0x00)
	return out
}

func TestPackNpcConfigs_Name(t *testing.T) {
	npcPack := npcOneSlotPack("rat")
	configs := map[string][]ConfigLine{
		"rat": {{Key: "name", Value: "Giant Rat"}},
	}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Body is empty; trailer emits opcode 2 + pjstr("Giant Rat\n") + 95 + p2(1).
	want := []byte{0x00, 0x01,
		0x02, 'G', 'i', 'a', 'n', 't', ' ', 'R', 'a', 't', 0x0A,
		0x5F, 0x00, 0x01,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Desc(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "desc", Value: "It bites."}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte{0x03}
	body = append(body, []byte("It bites.")...)
	body = append(body, 0x0A)
	want := expectClient("x", body...)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Size(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "size", Value: 3}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 12 + p1(3)
	want := expectClient("x", 0x0C, 0x03)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Readyanim(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "readyanim", Value: 7}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 13 + p2(7)
	want := expectClient("x", 0x0D, 0x00, 0x07)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Walkanim(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "walkanim", Value: 5}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Single id → opcode 14 + p2(5)
	want := expectClient("x", 0x0E, 0x00, 0x05)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_WalkanimList(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "walkanim", Value: []int{0, 1, 2, 3}}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 4-element array → opcode 17 + 4 × p2.
	want := expectClient("x",
		0x11,
		0x00, 0x00,
		0x00, 0x01,
		0x00, 0x02,
		0x00, 0x03,
	)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_HasalphaTrue(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "hasalpha", Value: true}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 16 only.
	want := expectClient("x", 0x10)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Category(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "category", Value: 5}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// server: opcode 18 + p2(5) + 250 + pjstr("x\n")
	want := expectServer("x", 0x12, 0x00, 0x05)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Op1(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "op1", Value: "attack"}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// op1 → opcode 30 + pjstr("attack\n")
	body := []byte{0x1E}
	body = append(body, []byte("attack")...)
	body = append(body, 0x0A)
	want := expectClient("x", body...)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Op5(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "op5", Value: "examine"}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// op5 → opcode 34 + pjstr("examine\n")
	body := []byte{0x22}
	body = append(body, []byte("examine")...)
	body = append(body, 0x0A)
	want := expectClient("x", body...)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Turnspeed(t *testing.T) {
	// rev-254: client opcode 103 + p2 (TS NpcConfig.ts:394-396 @ 2e3bcf43).
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "turnspeed", Value: 16}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := expectClient("x", 0x67, 0x00, 0x10)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Attack(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "attack", Value: 100}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 74 + p2(100)
	want := expectServer("x", 0x4A, 0x00, 0x64)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Defence(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "defence", Value: 50}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := expectServer("x", 0x4B, 0x00, 0x32)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Strength(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "strength", Value: 25}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := expectServer("x", 0x4C, 0x00, 0x19)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Hitpoints(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "hitpoints", Value: 500}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := expectServer("x", 0x4D, 0x01, 0xF4)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Ranged(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "ranged", Value: 7}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := expectServer("x", 0x4E, 0x00, 0x07)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Magic(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "magic", Value: 8}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := expectServer("x", 0x4F, 0x00, 0x08)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Resizex(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "resizex", Value: 100}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 90 + p2(100)
	want := expectClient("x", 0x5A, 0x00, 0x64)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Resizey(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "resizey", Value: 100}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 91 + p2(100)
	want := expectClient("x", 0x5B, 0x00, 0x64)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Resizez(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "resizez", Value: 100}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 92 + p2(100)
	want := expectClient("x", 0x5C, 0x00, 0x64)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_MinimapFalse(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "minimap", Value: false}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 93 only.
	want := expectClient("x", 0x5D)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Vislevel(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "vislevel", Value: 42}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 95 + p2(42); since vislevel was set, the default 95+p2(1)
	// is NOT emitted in the trailer. Trailer is just opcode 2 + pjstr(x\n).
	want := []byte{0x00, 0x01,
		0x5F, 0x00, 0x2A,
		0x02, 'x', 0x0A,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Resizeh(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "resizeh", Value: 200}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 97 + p2(200)
	want := expectClient("x", 0x61, 0x00, 0xC8)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Resizev(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "resizev", Value: 300}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 98 + p2(300)
	want := expectClient("x", 0x62, 0x01, 0x2C)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Wanderrange(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "wanderrange", Value: 5}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 200 + p2(5)
	want := expectServer("x", 0xC8, 0x00, 0x05)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Maxrange(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "maxrange", Value: 10}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 201 + p2(10)
	want := expectServer("x", 0xC9, 0x00, 0x0A)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Huntrange(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "huntrange", Value: 15}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 202 + p1(15) — mirrors TS L386 p1 (likely TS bug, byte-exact).
	want := expectServer("x", 0xCA, 0x0F)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Timer(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "timer", Value: 60}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 203 + p2(60)
	want := expectServer("x", 0xCB, 0x00, 0x3C)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Respawnrate(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "respawnrate", Value: 100}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 204 + p2(100)
	want := expectServer("x", 0xCC, 0x00, 0x64)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Moverestrict(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{Key: "moverestrict", Value: objtype.MoveRestrictBlocked}},
	}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 206 + p1(1)
	want := expectServer("x", 0xCE, 0x01)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Attackrange(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "attackrange", Value: 7}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 207 + p2(7)
	want := expectServer("x", 0xCF, 0x00, 0x07)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Blockwalk(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{Key: "blockwalk", Value: objtype.BlockWalkAll}},
	}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 208 + p1(2)
	want := expectServer("x", 0xD0, 0x02)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Huntmode(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "huntmode", Value: 3}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 209 + p1(3)
	want := expectServer("x", 0xD1, 0x03)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Defaultmode(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{Key: "defaultmode", Value: objtype.NPCModeWander}},
	}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 210 + p1(1)
	want := expectServer("x", 0xD2, 0x01)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_MembersTrue(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "members", Value: true}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 211 only
	want := expectServer("x", 0xD3)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_GivechaseFalse(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "givechase", Value: false}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 213 only
	want := expectServer("x", 0xD5)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Regenrate(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "regenrate", Value: 100}}}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 214 + p2(100)
	want := expectServer("x", 0xD6, 0x00, 0x64)
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_RecolPair(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	// Both < 100 → no rgb15→hsl16 conversion.
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "recol1s", Value: 11},
			{Key: "recol1d", Value: 22},
		},
	}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 40 + p1(1) + p2(11) + p2(22)
	want := expectClient("x",
		0x28, 0x01,
		0x00, 0x0B,
		0x00, 0x16,
	)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Models(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	// Two models; collected by first-pass, emitted in trailer as opcode 1.
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "model1", Value: 5},
			{Key: "model2", Value: 7},
		},
	}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// trailer order: name (opcode 2 + pjstr(x\n)) → models (opcode 1 + p1(2) + p2 p2)
	// → vislevel default (opcode 95 + p2(1))
	want := []byte{0x00, 0x01,
		0x02, 'x', 0x0A,
		0x01, 0x02, 0x00, 0x05, 0x00, 0x07,
		0x5F, 0x00, 0x01,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Heads(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "head1", Value: 3},
			{Key: "head2", Value: 9},
		},
	}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// trailer: name → heads (opcode 60 + p1(2) + p2 p2) → vislevel default
	want := []byte{0x00, 0x01,
		0x02, 'x', 0x0A,
		0x3C, 0x02, 0x00, 0x03, 0x00, 0x09,
		0x5F, 0x00, 0x01,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Patrol(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{Key: "patrol1", Value: npcPatrolEntry{Coord: 0x01020304, Delay: 7}}},
	}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// trailer: opcode 212 + p1(1) + p4(0x01020304) + p1(7) + 250 + pjstr(x\n) + Next 0x00
	want := []byte{0x00, 0x01,
		0xD4, 0x01,
		0x01, 0x02, 0x03, 0x04,
		0x07,
		0xFA, 'x', 0x0A,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_Param(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{
			Key:   "param",
			Value: ParamValue{ID: 4, Type: objtype.ScriptVarTypeInt, Value: 42},
		}},
	}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 249 + p1(1) + p3(4) + pbool(false) + p4(42) + 250 + pjstr(x\n) + Next
	want := []byte{0x00, 0x01,
		0xF9,
		0x01,
		0x00, 0x00, 0x04,
		0x00,
		0x00, 0x00, 0x00, 0x2A,
		0xFA, 'x', 0x0A,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackNpcConfigs_ParamString(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{
			Key:   "param",
			Value: ParamValue{ID: 3, Type: objtype.ScriptVarTypeString, Value: "hi"},
		}},
	}
	server, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 249 + p1(1) + p3(3) + pbool(true) + pjstr(hi\n) + 250 + pjstr(x\n) + Next
	want := []byte{0x00, 0x01,
		0xF9,
		0x01,
		0x00, 0x00, 0x03,
		0x01,
		'h', 'i', 0x0A,
		0xFA, 'x', 0x0A,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

// TestPackNpcConfigs_NoConfig pins the no-config arm: client emits only
// the Next 0x00 terminator (TS L271 `if (config)` block is skipped, so
// no name/95-default emit); server emits opcode 250 + pjstr(debugname) +
// Next 0x00.
func TestPackNpcConfigs_NoConfig(t *testing.T) {
	npcPack := npcOneSlotPack("rat")
	configs := map[string][]ConfigLine{}
	server, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantClient := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, wantClient) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, wantClient)
	}
	wantServer := []byte{0x00, 0x01,
		0xFA, 'r', 'a', 't', 0x0A,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, wantServer) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, wantServer)
	}
}

func TestPackNpcConfigs_DebugnameEmpty(t *testing.T) {
	// Empty debugname slot → no opcode 250 emit, just Next() terminator.
	// Also: configs map miss for "" → no `if config` block → no client
	// trailers either.
	npcPack := newTestPF("npc", map[int]string{0: ""})
	configs := map[string][]ConfigLine{}
	server, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// ── New keys: ambient / contrast / headicon / alwaysontop + modelFlags ──────

// TestParseNpcConfig_Ambient checks that "ambient" is accepted as a number key.
func TestParseNpcConfig_Ambient(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("ambient", "20")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("ambient key should be accepted")
	}
	n, ok := val.(int)
	if !ok || n != 20 {
		t.Fatalf("got %#v, want int 20", val)
	}
}

// TestParseNpcConfig_Contrast checks that "contrast" is accepted as a number key.
func TestParseNpcConfig_Contrast(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("contrast", "45")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("contrast key should be accepted")
	}
	n, ok := val.(int)
	if !ok || n != 45 {
		t.Fatalf("got %#v, want int 45", val)
	}
}

// TestParseNpcConfig_Headicon checks that "headicon" is accepted as a number key.
func TestParseNpcConfig_Headicon(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("headicon", "512")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("headicon key should be accepted")
	}
	n, ok := val.(int)
	if !ok || n != 512 {
		t.Fatalf("got %#v, want int 512", val)
	}
}

// TestParseNpcConfig_Alwaysontop checks that "alwaysontop" is accepted as a
// boolean key. TS source: NpcConfig.ts:31-33.
func TestParseNpcConfig_Alwaysontop(t *testing.T) {
	mp, cp, sp, hp, pt, lk := npcTestRegistries(t)
	parse := parseNpcConfigFor(mp, cp, sp, hp, lk, pt)

	val, accepted, err := parse("alwaysontop", "yes")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("alwaysontop key should be accepted")
	}
	b, ok := val.(bool)
	if !ok || !b {
		t.Fatalf("got %#v, want bool true", val)
	}
}

// TestPackNpcConfigs_Alwaysontop pins opcode 99 as presence-only (no value
// byte). TS NpcConfig.ts:379-381: `client.p1(99)` — no p1(value) follows.
// Only emitted when value is true (boolean key).
func TestPackNpcConfigs_Alwaysontop(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "alwaysontop", Value: true}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 99 (0x63) only — no value byte.
	want := expectClient("x", 0x63)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// TestPackNpcConfigs_AlwaysontopFalse pins the TS contract: alwaysontop=no
// DOES emit opcode 99, unconditionally. TS NpcConfig.ts:382-383 contains only
// `client.p1(99)` with no value check — unlike hasalpha/members/minimap which
// ARE guarded by `if (value)`. Presence in the config file is sufficient.
func TestPackNpcConfigs_AlwaysontopFalse(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "alwaysontop", Value: false}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 99 (0x63) only — no value byte — even when value is false.
	want := expectClient("x", 0x63)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// TestPackNpcConfigs_Ambient pins opcode 100 + p1(value).
// TS NpcConfig.ts:382-384: `client.p1(100); client.p1(value)`.
func TestPackNpcConfigs_Ambient(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "ambient", Value: 20}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 100 (0x64) + p1(20 = 0x14)
	want := expectClient("x", 0x64, 0x14)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// TestPackNpcConfigs_Contrast pins opcode 101 + p1(value).
// TS NpcConfig.ts:385-387: `client.p1(101); client.p1(value)`.
func TestPackNpcConfigs_Contrast(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "contrast", Value: 45}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 101 (0x65) + p1(45 = 0x2D)
	want := expectClient("x", 0x65, 0x2D)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// TestPackNpcConfigs_Headicon pins opcode 102 + p2(value).
// TS NpcConfig.ts:388-390: `client.p1(102); client.p2(value)`.
func TestPackNpcConfigs_Headicon(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "headicon", Value: 512}}}
	_, client, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 102 (0x66) + p2(512 = 0x02 0x00)
	want := expectClient("x", 0x66, 0x02, 0x00)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// TestPackNpcConfigs_ModelFlags_Model pins that model1= sets modelFlags[id]|=0x2.
// TS NpcConfig.ts:293: `modelFlags[value] |= 0x2; // todo: use context from script compiler`
func TestPackNpcConfigs_ModelFlags_Model(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "model1", Value: 5},
			{Key: "model2", Value: 7},
		},
	}
	// modelFlags slice must be large enough to cover model ids 5 and 7.
	modelFlags := make([]int, 10)
	_, _, err := packNpcConfigs(configs, npcPack, modelFlags)
	if err != nil {
		t.Fatal(err)
	}
	if modelFlags[5]&0x2 == 0 {
		t.Fatalf("modelFlags[5]: got 0x%X, expected bit 0x2 set", modelFlags[5])
	}
	if modelFlags[7]&0x2 == 0 {
		t.Fatalf("modelFlags[7]: got 0x%X, expected bit 0x2 set", modelFlags[7])
	}
	// Unrelated slot should be untouched.
	if modelFlags[0] != 0 {
		t.Fatalf("modelFlags[0]: got 0x%X, expected 0", modelFlags[0])
	}
}

// TestPackNpcConfigs_ModelFlags_Head pins that head1= sets modelFlags[id]|=0x2.
// TS NpcConfig.ts:297: `modelFlags[value] |= 0x2; // todo: use context from script compiler`
func TestPackNpcConfigs_ModelFlags_Head(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "head1", Value: 3},
			{Key: "head2", Value: 9},
		},
	}
	modelFlags := make([]int, 15)
	_, _, err := packNpcConfigs(configs, npcPack, modelFlags)
	if err != nil {
		t.Fatal(err)
	}
	if modelFlags[3]&0x2 == 0 {
		t.Fatalf("modelFlags[3]: got 0x%X, expected bit 0x2 set", modelFlags[3])
	}
	if modelFlags[9]&0x2 == 0 {
		t.Fatalf("modelFlags[9]: got 0x%X, expected bit 0x2 set", modelFlags[9])
	}
}

// TestPackNpcConfigs_ModelFlags_NilSafe pins that nil modelFlags does not panic
// (existing call sites pass nil; this covers the B5 no-op plumbing path).
func TestPackNpcConfigs_ModelFlags_NilSafe(t *testing.T) {
	npcPack := npcOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "model1", Value: 2},
			{Key: "head1", Value: 4},
		},
	}
	// nil must not panic — existing callers pass nil.
	_, _, err := packNpcConfigs(configs, npcPack, nil)
	if err != nil {
		t.Fatalf("nil modelFlags should not cause error: %v", err)
	}
}
