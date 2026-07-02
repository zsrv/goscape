package pack

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// objTestRegistries returns the four name-map packs + a paramTypes
// fixture used by obj parser/packer tests.
func objTestRegistries(t *testing.T) (modelPack, categoryPack, seqPack, objPack *PackFile, paramTypes *objtype.ParamTypeConfigs, lk *paramLookups) {
	t.Helper()
	modelPack = newTestPF("model", map[int]string{
		0: "model_zero",
		1: "model_one",
		2: "model_two",
	})
	categoryPack = newTestPF("category", map[int]string{
		0: "weapon",
		1: "armor",
	})
	seqPack = newTestPF("seq", map[int]string{
		0: "swing",
	})
	objPack = newTestPF("obj", map[int]string{
		0: "sword",
		1: "cert_sword",
		2: "shield",
		3: "template_for_cert",
	})
	intParam := &objtype.ParamType{Type: objtype.ScriptVarTypeInt}
	intParam.ID = 3
	intParam.DebugName = "damage"
	strParam := &objtype.ParamType{Type: objtype.ScriptVarTypeString}
	strParam.ID = 2
	strParam.DebugName = "label"
	paramTypes = &objtype.ParamTypeConfigs{
		ConfigNames: map[string]int{"damage": 3, "label": 2},
		Configs:     []*objtype.ParamType{nil, nil, strParam, intParam},
	}
	lk = &paramLookups{}
	return
}

// ── Parser tests ────────────────────────────────────────────────────────────

func TestParseObjConfig_Name(t *testing.T) {
	mp, cp, sp, op, pt, lk := objTestRegistries(t)
	parse := parseObjConfigFor(mp, cp, sp, op, lk, pt)

	val, accepted, err := parse("name", "Sword")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("name key should be accepted")
	}
	s, ok := val.(string)
	if !ok || s != "Sword" {
		t.Fatalf("got %#v, want string \"Sword\"", val)
	}
}

func TestParseObjConfig_Param(t *testing.T) {
	mp, cp, sp, op, pt, lk := objTestRegistries(t)
	parse := parseObjConfigFor(mp, cp, sp, op, lk, pt)

	val, accepted, err := parse("param", "damage,42")
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
	if pv.ID != 3 {
		t.Fatalf("got ID=%d, want 3", pv.ID)
	}
	if pv.Type != objtype.ScriptVarTypeInt {
		t.Fatalf("got Type=%d, want Int", pv.Type)
	}
	iv, ok := pv.Value.(int)
	if !ok || iv != 42 {
		t.Fatalf("got Value=%#v, want int 42", pv.Value)
	}
}

func TestParseObjConfig_UnknownKey(t *testing.T) {
	mp, cp, sp, op, pt, lk := objTestRegistries(t)
	parse := parseObjConfigFor(mp, cp, sp, op, lk, pt)

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

// ── Packer tests ────────────────────────────────────────────────────────────
//
// objPack fixtures use a single slot via objOneSlotPack so each test
// asserts a self-contained per-id frame (2-byte size header + body +
// Next() 0x00 terminator). Tests that need the cert/uncert pairing
// pattern use a multi-slot fixture and indexOffset to slice out one entry.

func objOneSlotPack(name string) *PackFile {
	return newTestPF("obj", map[int]string{0: name})
}

// objServerDebugTrailer returns the expected server bytes for a slot
// with a `name` debugname but no other server-side emits: 2-byte size
// header + opcode 250 + pjstr(name) + Next 0x00.
func objServerDebugTrailer(name string) []byte {
	buf := []byte{0x00, 0x01} // size=1 header
	buf = append(buf, 0xFA)
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0x0A, 0x00)
	return buf
}

// objClientEmptyBody returns the expected client bytes when there are
// no client-side emits: 2-byte size header + Next 0x00.
func objClientEmptyBody() []byte {
	return []byte{0x00, 0x01, 0x00}
}

func TestPackObjConfigs_Model(t *testing.T) {
	objPack := objOneSlotPack("widget")
	configs := map[string][]ConfigLine{
		"widget": {{Key: "model", Value: 7}},
	}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 1 + p2(7) → and because model is present but no name, the
	// synthesised name is appended ("Widget").
	want := []byte{
		0x00, 0x01,
		0x01, 0x00, 0x07,
		0x02, 'W', 'i', 'd', 'g', 'e', 't', 0x0A,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Name(t *testing.T) {
	objPack := objOneSlotPack("sword")
	configs := map[string][]ConfigLine{
		"sword": {{Key: "name", Value: "Sword"}},
	}
	server, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Client: opcode 2 + pjstr("Sword\n") + Next 0x00
	wantClient := []byte{0x00, 0x01, 0x02, 'S', 'w', 'o', 'r', 'd', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, wantClient) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, wantClient)
	}
	// Server: just debugname trailer.
	if !bytes.Equal(server.Dat.Data, objServerDebugTrailer("sword")) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, objServerDebugTrailer("sword"))
	}
}

func TestPackObjConfigs_Desc(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "desc", Value: "It's a thing."}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x03}
	want = append(want, []byte("It's a thing.")...)
	want = append(want, 0x0A, 0x00)
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Zoom2d(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "2dzoom", Value: 0x1234}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x04, 0x12, 0x34, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Xan2d(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "2dxan", Value: 0x0102}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x05, 0x01, 0x02, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Yan2d(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "2dyan", Value: 0x0203}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x06, 0x02, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Xof2d(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "2dxof", Value: 50}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x07, 0x00, 0x32, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Yof2d(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "2dyof", Value: 60}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x08, 0x00, 0x3C, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Code9True(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "code9", Value: true}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x09, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Code10(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "code10", Value: 5}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x0A, 0x00, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_StackableTrue(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "stackable", Value: true}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x0B, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Cost(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "cost", Value: 0x01020304}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x0C, 0x01, 0x02, 0x03, 0x04, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Wearpos(t *testing.T) {
	objPack := objOneSlotPack("x")
	// wearpos=4 (torso) → opcode 13 + p1(4) on server.
	configs := map[string][]ConfigLine{"x": {{Key: "wearpos", Value: 4}}}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x0D, 0x04, 0xFA, 'x', 0x0A, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackObjConfigs_Wearpos2(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "wearpos2", Value: 6}}}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x0E, 0x06, 0xFA, 'x', 0x0A, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackObjConfigs_TradeableFalse(t *testing.T) {
	objPack := objOneSlotPack("x")
	// tradeable=false → opcode 15 on server.
	configs := map[string][]ConfigLine{"x": {{Key: "tradeable", Value: false}}}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x0F, 0xFA, 'x', 0x0A, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackObjConfigs_MembersTrue(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "members", Value: true}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x10, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Manwear(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{Key: "manwear", Value: objManWomanWearPair{Model: 1, Offset: 7}}},
	}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 23 + p2(1) + p1(7) + Next
	want := []byte{0x00, 0x01, 0x17, 0x00, 0x01, 0x07, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Manwear2(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "manwear2", Value: 3}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x18, 0x00, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Womanwear(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{Key: "womanwear", Value: objManWomanWearPair{Model: 2, Offset: 5}}},
	}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 25 + p2(2) + p1(5) + Next
	want := []byte{0x00, 0x01, 0x19, 0x00, 0x02, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Womanwear2(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "womanwear2", Value: 4}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x1A, 0x00, 0x04, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Wearpos3(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "wearpos3", Value: 8}}}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x1B, 0x08, 0xFA, 'x', 0x0A, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackObjConfigs_Op1(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "op1", Value: "use"}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// op1 → opcode 30 + pjstr("use\n") + Next
	want := []byte{0x00, 0x01, 0x1E, 'u', 's', 'e', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Op5(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "op5", Value: "drop"}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// op5 → opcode 34 + pjstr("drop\n") + Next
	want := []byte{0x00, 0x01, 0x22, 'd', 'r', 'o', 'p', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Iop1(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "iop1", Value: "examine"}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// iop1 → opcode 35 + pjstr("examine\n") + Next
	want := []byte{0x00, 0x01, 0x23, 'e', 'x', 'a', 'm', 'i', 'n', 'e', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Iop5(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "iop5", Value: "trash"}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// iop5 → opcode 39 + pjstr("trash\n") + Next
	want := []byte{0x00, 0x01, 0x27, 't', 'r', 'a', 's', 'h', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_RecolPair(t *testing.T) {
	objPack := objOneSlotPack("x")
	// Both values < 100 → no rgb15→hsl16 conversion.
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "recol1s", Value: 11},
			{Key: "recol1d", Value: 22},
		},
	}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 40 + p1(count=1) + p2(11) + p2(22) + Next
	want := []byte{0x00, 0x01, 0x28, 0x01, 0x00, 0x0B, 0x00, 0x16, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Weight(t *testing.T) {
	objPack := objOneSlotPack("x")
	// weight pre-parsed to -10 grams. Server emits opcode 75 + p2 (signed).
	configs := map[string][]ConfigLine{"x": {{Key: "weight", Value: -10}}}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// p2(int16(-10)) → 0xFF 0xF6
	want := []byte{0x00, 0x01, 0x4B, 0xFF, 0xF6, 0xFA, 'x', 0x0A, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackObjConfigs_Manwear3(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "manwear3", Value: 11}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x4E, 0x00, 0x0B, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Womanwear3(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "womanwear3", Value: 12}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x4F, 0x00, 0x0C, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Manhead(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "manhead", Value: 1}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x5A, 0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Womanhead(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "womanhead", Value: 2}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x5B, 0x00, 0x02, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Manhead2(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "manhead2", Value: 3}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x5C, 0x00, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Womanhead2(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "womanhead2", Value: 4}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x5D, 0x00, 0x04, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Category(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "category", Value: 9}}}
	server, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Server: opcode 94 + p2(9) + opcode 250 + pjstr("x\n") + Next
	wantServer := []byte{0x00, 0x01, 0x5E, 0x00, 0x09, 0xFA, 'x', 0x0A, 0x00}
	if !bytes.Equal(server.Dat.Data, wantServer) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, wantServer)
	}
	if !bytes.Equal(client.Dat.Data, objClientEmptyBody()) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, objClientEmptyBody())
	}
}

func TestPackObjConfigs_Zan2d(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "2dzan", Value: 0x0506}}}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x5F, 0x05, 0x06, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Dummyitem(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "dummyitem", Value: 1}}}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Server: opcode 96 + p1(1) + 250 + pjstr("x\n") + Next
	want := []byte{0x00, 0x01, 0x60, 0x01, 0xFA, 'x', 0x0A, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackObjConfigs_Count1(t *testing.T) {
	objPack := objOneSlotPack("x")
	// count1=obj,N → opcode 100 + p2(obj) + p2(count)
	configs := map[string][]ConfigLine{
		"x": {{Key: "count1", Value: objCountPair{Obj: 42, Count: 100}}},
	}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x64, 0x00, 0x2A, 0x00, 0x64, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Count10(t *testing.T) {
	objPack := objOneSlotPack("x")
	// count10 → opcode 100 + (10-1) = 109
	configs := map[string][]ConfigLine{
		"x": {{Key: "count10", Value: objCountPair{Obj: 1, Count: 5}}},
	}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x6D, 0x00, 0x01, 0x00, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackObjConfigs_Respawnrate(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "respawnrate", Value: 600}}}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Server: opcode 201 + p2(600=0x0258) + 250 + pjstr("x\n") + Next
	want := []byte{0x00, 0x01, 0xC9, 0x02, 0x58, 0xFA, 'x', 0x0A, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackObjConfigs_Param(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{
			Key:   "param",
			Value: ParamValue{ID: 3, Type: objtype.ScriptVarTypeInt, Value: 42},
		}},
	}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Server: opcode 249 + p1(1) + p3(3) + pbool(false) + p4(42)
	// + opcode 250 + pjstr("x\n") + Next
	want := []byte{
		0x00, 0x01,
		0xF9,
		0x01,
		0x00, 0x00, 0x03,
		0x00,
		0x00, 0x00, 0x00, 0x2A,
		0xFA, 'x', 0x0A,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackObjConfigs_ParamString(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {{
			Key:   "param",
			Value: ParamValue{ID: 2, Type: objtype.ScriptVarTypeString, Value: "hi"},
		}},
	}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 249 + p1(1) + p3(2) + pbool(true) + pjstr("hi\n")
	// + opcode 250 + pjstr("x\n") + Next
	want := []byte{
		0x00, 0x01,
		0xF9,
		0x01,
		0x00, 0x00, 0x02,
		0x01,
		'h', 'i', 0x0A,
		0xFA, 'x', 0x0A,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

// TestPackObjConfigs_CertUncertPairing pins the dual asymmetry:
//
//   - id=0 "sword": NON-cert with a config block. Server emits the
//     reverse-lookup opcode 97 (cert_sword exists at id=1) followed by
//     the 250 + pjstr trailer. Client emits members=true (opcode 16).
//   - id=1 "cert_sword": IS-cert. cfg replaced with [{certlink, 0},
//     {certtemplate, 3}] regardless of any user-supplied [cert_sword]
//     config. Client emits opcodes 97 (p2(0)) and 98 (p2(3)). Server
//     emits ONLY 250 + pjstr — cert names never reverse-lookup
//     themselves (TS guard: `!isCert`).
//   - id=2 "shield": NON-cert with NO config block (configs map miss).
//     TS's `if (config)` skips the entire trailer block, including
//     reverse-lookup. Server emits ONLY 250 + pjstr; client is empty.
//   - id=3 "template_for_cert": NON-cert with no config and no
//     "cert_template_for_cert". Same shape as id=2.
//
// TS source: tools/pack/config/ObjConfig.ts:209-218 (replacement) and
// 389-394 (reverse lookup gated by `if (config)`).
func TestPackObjConfigs_CertUncertPairing(t *testing.T) {
	// All four obj ids must be present per docstring.
	objPack := newTestPF("obj", map[int]string{
		0: "sword",
		1: "cert_sword",
		2: "shield",
		3: "template_for_cert",
	})
	configs := map[string][]ConfigLine{
		"sword": {{Key: "members", Value: true}},
		// "cert_sword" intentionally omitted: replacement runs regardless.
		// "shield" and "template_for_cert" omitted: tests the "no config →
		// no reverse-lookup, no trailers" arm.
	}

	server, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Server expected stream:
	//   2-byte size header (0x00 0x04)
	//   id=0 "sword": opcode 97 + p2(1=cert_sword id) + 250 + pjstr("sword\n") + Next 0x00
	//   id=1 "cert_sword": 250 + pjstr("cert_sword\n") + Next 0x00
	//   id=2 "shield": 250 + pjstr("shield\n") + Next 0x00
	//   id=3 "template_for_cert": 250 + pjstr("template_for_cert\n") + Next 0x00
	var wantServer []byte
	wantServer = append(wantServer, 0x00, 0x04)
	// id=0
	wantServer = append(wantServer, 0x61, 0x00, 0x01)
	wantServer = append(wantServer, 0xFA)
	wantServer = append(wantServer, []byte("sword")...)
	wantServer = append(wantServer, 0x0A, 0x00)
	// id=1
	wantServer = append(wantServer, 0xFA)
	wantServer = append(wantServer, []byte("cert_sword")...)
	wantServer = append(wantServer, 0x0A, 0x00)
	// id=2
	wantServer = append(wantServer, 0xFA)
	wantServer = append(wantServer, []byte("shield")...)
	wantServer = append(wantServer, 0x0A, 0x00)
	// id=3
	wantServer = append(wantServer, 0xFA)
	wantServer = append(wantServer, []byte("template_for_cert")...)
	wantServer = append(wantServer, 0x0A, 0x00)

	if !bytes.Equal(server.Dat.Data, wantServer) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, wantServer)
	}

	// Client expected stream:
	//   2-byte size header (0x00 0x04)
	//   id=0 "sword": opcode 16 (members=true) + Next 0x00
	//   id=1 "cert_sword": 97 + p2(0=sword uncert) + 98 + p2(3=template_for_cert) + Next 0x00
	//   id=2 "shield": empty + Next 0x00 (no config block)
	//   id=3 "template_for_cert": empty + Next 0x00
	var wantClient []byte
	wantClient = append(wantClient, 0x00, 0x04)
	wantClient = append(wantClient, 0x10, 0x00)
	wantClient = append(wantClient, 0x61, 0x00, 0x00, 0x62, 0x00, 0x03, 0x00)
	wantClient = append(wantClient, 0x00)
	wantClient = append(wantClient, 0x00)

	if !bytes.Equal(client.Dat.Data, wantClient) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, wantClient)
	}
}

func TestPackObjConfigs_DebugnameEmpty(t *testing.T) {
	// Empty debugname slot → no opcode 250, just Next() terminator.
	objPack := newTestPF("obj", map[int]string{0: ""})
	configs := map[string][]ConfigLine{}
	server, _, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

// ── Task 8: resize/ambient/contrast + modelFlags ─────────────────────────────

// TestParseObjConfig_ResizeAmbientContrast verifies that the five new
// number-keyed fields are accepted by the parser.
// TS source: ObjConfig.ts:18-23 (244).
func TestParseObjConfig_ResizeAmbientContrast(t *testing.T) {
	mp, cp, sp, op, pt, lk := objTestRegistries(t)
	parse := parseObjConfigFor(mp, cp, sp, op, lk, pt)

	cases := []struct {
		key   string
		input string
		want  int
	}{
		{"resizex", "100", 100},
		{"resizey", "200", 200},
		{"resizez", "300", 300},
		{"ambient", "10", 10},
		{"contrast", "20", 20},
	}
	for _, tc := range cases {
		val, accepted, err := parse(tc.key, tc.input)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.key, err)
		}
		if !accepted {
			t.Fatalf("%s: key should be accepted", tc.key)
		}
		n, ok := val.(int)
		if !ok || n != tc.want {
			t.Fatalf("%s: got %#v, want int %d", tc.key, val, tc.want)
		}
	}
}

// TestPackObjConfigs_ResizeAmbientContrast verifies that the five new fields
// emit the correct opcodes and wire widths:
//
//	resizex  → p1(0x6E) p2(v)  (opcode 110)
//	resizey  → p1(0x6F) p2(v)  (opcode 111)
//	resizez  → p1(0x70) p2(v)  (opcode 112)
//	ambient  → p1(0x71) p1(v)  (opcode 113)
//	contrast → p1(0x72) p1(v)  (opcode 114)
//
// TS source: ObjConfig.ts:400-414 (244).
func TestPackObjConfigs_ResizeAmbientContrast(t *testing.T) {
	objPack := objOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "resizex", Value: 0x0102},
			{Key: "resizey", Value: 0x0304},
			{Key: "resizez", Value: 0x0506},
			{Key: "ambient", Value: 7},
			{Key: "contrast", Value: 8},
		},
	}
	_, client, err := packObjConfigs(configs, objPack, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01,
		0x6E, 0x01, 0x02, // resizex p2
		0x6F, 0x03, 0x04, // resizey p2
		0x70, 0x05, 0x06, // resizez p2
		0x71, 0x07, // ambient p1
		0x72, 0x08, // contrast p1
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// TestPackObjConfigs_ModelFlags_Members verifies that after the per-key loop
// a members obj sets modelFlags[modelID] |= 0x40 and modelFlags[wornID] |= 0x10.
// TS source: ObjConfig.ts:421-428 (244).
func TestPackObjConfigs_ModelFlags_Members(t *testing.T) {
	objPack := objOneSlotPack("ring")
	// model id=5, manwear first-tuple id=7, members=true
	configs := map[string][]ConfigLine{
		"ring": {
			{Key: "model", Value: 5},
			{Key: "manwear", Value: objManWomanWearPair{Model: 7, Offset: 0}},
			{Key: "members", Value: true},
		},
	}
	// modelFlags needs enough slots for the highest id referenced.
	modelFlags := make([]int, 10)
	_, _, err := packObjConfigs(configs, objPack, modelFlags)
	if err != nil {
		t.Fatal(err)
	}
	// model id=5 → members → 0x40
	if modelFlags[5] != 0x40 {
		t.Errorf("modelFlags[5] = 0x%02X, want 0x40", modelFlags[5])
	}
	// worn id=7 → members worn → 0x10
	if modelFlags[7] != 0x10 {
		t.Errorf("modelFlags[7] = 0x%02X, want 0x10", modelFlags[7])
	}
}

// TestPackObjConfigs_ModelFlags_NonMembers verifies that a non-members obj
// sets modelFlags[modelID] |= 0x20 and modelFlags[wornID] |= 0x08.
// TS source: ObjConfig.ts:429-437 (244).
func TestPackObjConfigs_ModelFlags_NonMembers(t *testing.T) {
	objPack := objOneSlotPack("dagger")
	configs := map[string][]ConfigLine{
		"dagger": {
			{Key: "model", Value: 3},
			{Key: "manwear", Value: objManWomanWearPair{Model: 4, Offset: 0}},
			// members omitted → false
		},
	}
	modelFlags := make([]int, 10)
	_, _, err := packObjConfigs(configs, objPack, modelFlags)
	if err != nil {
		t.Fatal(err)
	}
	// model id=3 → non-members → 0x20
	if modelFlags[3] != 0x20 {
		t.Errorf("modelFlags[3] = 0x%02X, want 0x20", modelFlags[3])
	}
	// worn id=4 → non-members worn → 0x08
	if modelFlags[4] != 0x08 {
		t.Errorf("modelFlags[4] = 0x%02X, want 0x08", modelFlags[4])
	}
}

// TestPackObjConfigs_ModelFlags_ManheadInline verifies that manhead/womanhead/
// manhead2/womanhead2 set modelFlags[id] |= 0x80 INLINE during key emit.
// TS source: ObjConfig.ts:363-377 (244).
func TestPackObjConfigs_ModelFlags_ManheadInline(t *testing.T) {
	objPack := objOneSlotPack("helm")
	configs := map[string][]ConfigLine{
		"helm": {
			{Key: "manhead", Value: 2},
			{Key: "womanhead", Value: 3},
			{Key: "manhead2", Value: 4},
			{Key: "womanhead2", Value: 5},
		},
	}
	modelFlags := make([]int, 10)
	_, _, err := packObjConfigs(configs, objPack, modelFlags)
	if err != nil {
		t.Fatal(err)
	}
	for id, flag := range []int{2, 3, 4, 5} {
		if modelFlags[flag] != 0x80 {
			t.Errorf("modelFlags[%d] = 0x%02X, want 0x80", id+2, modelFlags[flag])
		}
	}
}

// TestPackObjConfigs_ModelFlags_ManwearTupleFirstElement verifies that for
// manwear (a tuple), the FIRST element (the model id) is tracked for worn[],
// not the offset. TS source: ObjConfig.ts:325 (244) worn.push(values[0]).
func TestPackObjConfigs_ModelFlags_ManwearTupleFirstElement(t *testing.T) {
	objPack := objOneSlotPack("cape")
	// manwear pair: Model=6, Offset=2. Only id=6 should appear in worn[].
	configs := map[string][]ConfigLine{
		"cape": {
			{Key: "manwear", Value: objManWomanWearPair{Model: 6, Offset: 2}},
			{Key: "members", Value: true},
		},
	}
	modelFlags := make([]int, 10)
	_, _, err := packObjConfigs(configs, objPack, modelFlags)
	if err != nil {
		t.Fatal(err)
	}
	// Only id=6 (the model, not the offset=2) gets 0x10.
	if modelFlags[6] != 0x10 {
		t.Errorf("modelFlags[6] = 0x%02X, want 0x10", modelFlags[6])
	}
	// id=2 (offset) must NOT be set.
	if modelFlags[2] != 0 {
		t.Errorf("modelFlags[2] = 0x%02X, want 0x00 (offset must not be tracked)", modelFlags[2])
	}
}
