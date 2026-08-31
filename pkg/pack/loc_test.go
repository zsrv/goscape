package pack

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// locTestRegistries returns the four name-map packs + a paramTypes
// fixture used by loc parser/packer tests.
func locTestRegistries(t *testing.T) (modelPack, categoryPack, seqPack, texturePack *PackFile, paramTypes *objtype.ParamTypeConfigs, lk *paramLookups) {
	t.Helper()
	modelPack = newTestPF("model", map[int]string{
		0: "table",
		1: "chair",
		2: "table_8",
	})
	categoryPack = newTestPF("category", map[int]string{
		0: "furniture",
	})
	seqPack = newTestPF("seq", map[int]string{
		0: "idle",
	})
	texturePack = newTestPF("texture", map[int]string{
		0: "wood",
	})
	intParam := &objtype.ParamType{Type: objtype.ScriptVarTypeInt}
	intParam.ID = 7
	intParam.DebugName = "flammable"
	strParam := &objtype.ParamType{Type: objtype.ScriptVarTypeString}
	strParam.ID = 6
	strParam.DebugName = "label"
	paramTypes = &objtype.ParamTypeConfigs{
		ConfigNames: map[string]int{"flammable": 7, "label": 6},
		Configs:     []*objtype.ParamType{nil, nil, nil, nil, nil, nil, strParam, intParam},
	}
	lk = &paramLookups{}
	return
}

// ── Parser tests ────────────────────────────────────────────────────────────

func TestParseLocConfig_Name(t *testing.T) {
	_, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(cp, sp, tp, lk, pt)

	val, accepted, err := parse("name", "Table")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("name key should be accepted")
	}
	s, ok := val.(string)
	if !ok || s != "Table" {
		t.Fatalf("got %#v, want string \"Table\"", val)
	}
}

func TestParseLocConfig_Width(t *testing.T) {
	_, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(cp, sp, tp, lk, pt)

	val, accepted, err := parse("width", "3")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("width key should be accepted")
	}
	n, ok := val.(int)
	if !ok || n != 3 {
		t.Fatalf("got %#v, want int 3", val)
	}
}

func TestParseLocConfig_Param(t *testing.T) {
	_, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(cp, sp, tp, lk, pt)

	val, accepted, err := parse("param", "flammable,1")
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
	if pv.ID != 7 {
		t.Fatalf("got ID=%d, want 7", pv.ID)
	}
	if pv.Type != objtype.ScriptVarTypeInt {
		t.Fatalf("got Type=%d, want Int", pv.Type)
	}
	iv, ok := pv.Value.(int)
	if !ok || iv != 1 {
		t.Fatalf("got Value=%#v, want int 1", pv.Value)
	}
}

func TestParseLocConfig_UnknownKey(t *testing.T) {
	_, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(cp, sp, tp, lk, pt)

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
// Per TS LocConfig.ts:170-434, almost every opcode is emitted to the
// `client` PackedData buffer; only opcodes 61 (category), 249 (params),
// and 250 (debugname) go to `server`. The byte-pin tests below assert
// the appropriate buffer per opcode. Each emitted buffer is framed:
//
//   - 2-byte size header (`p2(locPack.Max)`)
//   - per-id body
//   - 1-byte `0x00` Next() terminator after each id
//
// PJStr uses an LF (0x0a) terminator (`PackedData.PJStr` → PJStrLF).

func locOneSlotPack(name string) *PackFile {
	return newTestPF("loc", map[int]string{0: name})
}

// header + (trailing 250+pjstr(debugname)+Next) wrapper for an unnamed
// `debugname` slot in server output.
func locServerDebugTrailer(name string) []byte {
	buf := []byte{0x00, 0x01} // size=1 header
	buf = append(buf, 0xFA)
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0x0A, 0x00)
	return buf
}

// header + Next() terminator (no body) for the client buffer when there
// are no client-side emits.
func locClientEmptyBody() []byte {
	return []byte{0x00, 0x01, 0x00}
}

func TestPackLocConfigs_Name(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("table")
	configs := map[string][]ConfigLine{
		"table": {{Key: "name", Value: "Table"}},
	}
	server, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Client: opcode 2 + pjstr("Table\n") + Next 0x00
	wantClient := []byte{0x00, 0x01, 0x02, 'T', 'a', 'b', 'l', 'e', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, wantClient) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, wantClient)
	}
	// Server: only debugname trailer.
	if !bytes.Equal(server.Dat.Data, locServerDebugTrailer("table")) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, locServerDebugTrailer("table"))
	}
}

func TestPackLocConfigs_Width(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("block")
	configs := map[string][]ConfigLine{
		"block": {{Key: "width", Value: 3}},
	}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x0E, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Length(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "length", Value: 4}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x0F, 0x04, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_BlockwalkFalse(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "blockwalk", Value: false}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x11, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_BlockrangeFalse(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "blockrange", Value: false}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x12, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Active(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "active", Value: true}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 19 + pbool(true)=0x01 + opcode 2 + pjstr("x\n") (active=1 forces name) + Next
	want := []byte{0x00, 0x01, 0x13, 0x01, 0x02, 'x', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_HillskewTrue(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "hillskew", Value: true}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x15, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_SharelightTrue(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "sharelight", Value: true}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x16, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_OccludeTrue(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "occlude", Value: true}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x17, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Anim(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "anim", Value: 7}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x18, 0x00, 0x07, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

// TestPackLocConfigs_HasalphaRetired pins that hasalpha emits nothing —
// Engine-TS 8139461a dropped the key and the client opcode-25 emission
// (TS LocConfig.ts:57,247 @1d25566c).
func TestPackLocConfigs_HasalphaRetired(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "hasalpha", Value: true}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
	if _, ok := locBooleanKeys["hasalpha"]; ok {
		t.Error("locBooleanKeys still contains hasalpha")
	}
}

func TestPackLocConfigs_Wallwidth(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "wallwidth", Value: 5}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x1C, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Ambient(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "ambient", Value: 9}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x1D, 0x09, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Contrast(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "contrast", Value: 4}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x27, 0x04, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Op1(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "op1", Value: "use"}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// op1 → opcode 30+(1-1)=30 + pjstr("use\n") + Next
	want := []byte{0x00, 0x01, 0x1E, 'u', 's', 'e', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_RecolPair(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "recol1s", Value: 0x1111},
			{Key: "recol1d", Value: 0x2222},
		},
	}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 40 + p1(count=1) + p2(0x1111) + p2(0x2222) + Next
	want := []byte{0x00, 0x01, 0x28, 0x01, 0x11, 0x11, 0x22, 0x22, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_RetexPair(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{
		"x": {
			{Key: "retex1s", Value: 7},
			{Key: "retex1d", Value: 8},
		},
	}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// retex uses opcode 40 (shared recol slot pre-rev-465) + p1(count) + p2 p2.
	want := []byte{0x00, 0x01, 0x28, 0x01, 0x00, 0x07, 0x00, 0x08, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Mapfunction(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "mapfunction", Value: 0x1234}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x3C, 0x12, 0x34, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Category(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "category", Value: 9}}}
	server, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Server: opcode 61 + p2(9) + opcode 250 + pjstr("x\n") + Next
	wantServer := []byte{0x00, 0x01, 0x3D, 0x00, 0x09, 0xFA, 'x', 0x0A, 0x00}
	if !bytes.Equal(server.Dat.Data, wantServer) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, wantServer)
	}
	// Client: empty body, just Next 0x00.
	if !bytes.Equal(client.Dat.Data, locClientEmptyBody()) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, locClientEmptyBody())
	}
}

func TestPackLocConfigs_MirrorTrue(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "mirror", Value: true}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x3E, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_ShadowFalse(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "shadow", Value: false}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x40, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Resizex(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "resizex", Value: 100}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x41, 0x00, 0x64, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Resizey(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "resizey", Value: 100}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x42, 0x00, 0x64, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Resizez(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "resizez", Value: 100}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x43, 0x00, 0x64, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Mapscene(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "mapscene", Value: 4}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x44, 0x00, 0x04, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Forceapproach(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "forceapproach", Value: 0b1110}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x45, 0x0E, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Offsetx(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "offsetx", Value: 50}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x46, 0x00, 0x32, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Offsety(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "offsety", Value: 50}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x47, 0x00, 0x32, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Offsetz(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "offsetz", Value: 50}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x48, 0x00, 0x32, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_ForcedecorTrue(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "forcedecor", Value: true}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x49, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Desc(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "desc", Value: "A thing"}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 3 + pjstr("A thing\n") + Next
	want := []byte{0x00, 0x01, 0x03, 'A', ' ', 't', 'h', 'i', 'n', 'g', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Models_ForceShape8(t *testing.T) {
	// "chair" exists in modelPack as the raw (unsuffixed) name → resolves to
	// shape _8 (centrepiece_straight) via the raw-name probe (TS
	// LocConfig.ts:329-336 @4c95f87e; upstream 3b653372). All-centrepiece
	// list → compact opcode-5 form at rev-254 (TS LocConfig.ts:386-392
	// @ 2e3bcf43).
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("c")
	configs := map[string][]ConfigLine{"c": {{Key: "model", Value: "chair"}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 5 + p1(count=1) + p2(modelID=1) — NO shape byte
	// + opcode 2 + pjstr("c\n")  (shape _8 forces name-transmit)
	// + Next
	want := []byte{0x00, 0x01, 0x05, 0x01, 0x00, 0x01, 0x02, 'c', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Models_MixedShapesKeepCode1(t *testing.T) {
	// "wall" resolves via the raw-name centrepiece probe (id=1, post-rename
	// naming — TS LocConfig.ts:329-336 @4c95f87e; upstream 3b653372) AND the
	// wall_1 shape-suffix variant (wall_straight shape 0, id=0). Mixed
	// shapes → centrepieceOnly false → the opcode-1 (model, shape) pair
	// form is retained (TS LocConfig.ts:393-401 @ 2e3bcf43).
	mp := newTestPF("model", map[int]string{
		0: "wall_1", // LocShapeSuffix[0] = "_1" (wall_straight)
		1: "wall",   // centrepiece_straight (raw name, no _8 suffix)
	})
	locPack := locOneSlotPack("mywall")
	configs := map[string][]ConfigLine{
		"mywall": {{Key: "model", Value: "wall"}},
	}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 1 + p1(count=2) + (p2(1)+p1(10)) [raw "wall" probed first] +
	// (p2(0)+p1(0)) + opcode 2 + pjstr (centrepiece presence forces name)
	// + Next
	want := []byte{0x00, 0x01,
		0x01, 0x02,
		0x00, 0x01, 0x0A,
		0x00, 0x00, 0x00,
		0x02, 'm', 'y', 'w', 'a', 'l', 'l', 0x0A,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestParseLocConfig_Model2to5Accepted_Model1Rejected(t *testing.T) {
	// rev-254: stringKeys enumerate model, model2..model5
	// (TS LocConfig.ts:37-42 @ 2e3bcf43). model1 is NOT in the list →
	// invalid property KEY.
	_, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(cp, sp, tp, lk, pt)

	for _, key := range []string{"model", "model2", "model3", "model4", "model5"} {
		v, accepted, err := parse(key, "chair")
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if !accepted {
			t.Fatalf("%s: should be accepted", key)
		}
		if v.(string) != "chair" {
			t.Fatalf("%s: got %#v, want raw string passthrough", key, v)
		}
	}

	_, accepted, err := parse("model1", "chair")
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("model1: accepted; want invalid key (TS stringKeys exclude model1)")
	}
}

func TestParseLocConfig_Raiseobject(t *testing.T) {
	// rev-254: raiseobject joined booleanKeys (TS LocConfig.ts:60 @ 2e3bcf43).
	_, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(cp, sp, tp, lk, pt)

	v, accepted, err := parse("raiseobject", "yes")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("raiseobject should be accepted")
	}
	if v.(bool) != true {
		t.Fatalf("got %#v, want true", v)
	}

	if _, _, err := parse("raiseobject", "maybe"); err == nil {
		t.Fatal("want err for non-boolean raiseobject")
	}
}

func TestPackLocConfigs_Raiseobject(t *testing.T) {
	// rev-254: opcode 75 + pbool, emitted for BOTH true and false (no
	// asymmetry, unlike forcedecor 73 / breakroutefinding 74).
	// TS LocConfig.ts:311-314 @ 2e3bcf43.
	mp, _, _, _, _, _ := locTestRegistries(t)

	for _, tc := range []struct {
		val  bool
		want []byte
	}{
		{true, []byte{0x00, 0x01, 0x4B, 0x01, 0x00}},
		{false, []byte{0x00, 0x01, 0x4B, 0x00, 0x00}},
	} {
		locPack := locOneSlotPack("x")
		configs := map[string][]ConfigLine{"x": {{Key: "raiseobject", Value: tc.val}}}
		_, client, err := packLocConfigs(configs, locPack, mp, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(client.Dat.Data, tc.want) {
			t.Fatalf("raiseobject=%v: got % x, want % x", tc.val, client.Dat.Data, tc.want)
		}
	}
}

func TestPackLocConfigs_Param(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("fire")
	configs := map[string][]ConfigLine{
		"fire": {{
			Key:   "param",
			Value: ParamValue{ID: 7, Type: objtype.ScriptVarTypeInt, Value: 1},
		}},
	}
	server, _, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Server: opcode 249 + p1(1) + p3(7) + pbool(false) + p4(1)
	// + opcode 250 + pjstr("fire\n") + Next
	want := []byte{
		0x00, 0x01,
		0xF9,
		0x01,
		0x00, 0x00, 0x07,
		0x00,
		0x00, 0x00, 0x00, 0x01,
		0xFA, 'f', 'i', 'r', 'e', 0x0A,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackLocConfigs_ParamString(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("s")
	configs := map[string][]ConfigLine{
		"s": {{
			Key:   "param",
			Value: ParamValue{ID: 6, Type: objtype.ScriptVarTypeString, Value: "hi"},
		}},
	}
	server, _, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 249 + p1(1) + p3(6) + pbool(true) + pjstr("hi\n")
	// + opcode 250 + pjstr("s\n") + Next
	want := []byte{
		0x00, 0x01,
		0xF9,
		0x01,
		0x00, 0x00, 0x06,
		0x01,
		'h', 'i', 0x0A,
		0xFA, 's', 0x0A,
		0x00,
	}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

func TestPackLocConfigs_DebugnameEmpty(t *testing.T) {
	// Empty debugname slot → no opcode 250, just Next() terminator.
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := newTestPF("loc", map[int]string{0: ""})
	configs := map[string][]ConfigLine{}
	server, _, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}

// TestPackLocConfigs_ModelFlags_0x4_ShapeSuffix pins that resolveLocModels
// writes 0x4 into modelFlags for BOTH the raw-name centrepiece branch and
// the per-shape suffix branch (TS LocConfig.ts:333,345 @4c95f87e; upstream
// 3b653372 — the old directReference logic that forced raw-only resolution
// when a shape variant existed is deleted).
//
// Setup: raw="wall" has a non-_8 shape variant "wall_0" (LocShapeSuffix[0]).
// The raw-name probe finds "wall" (id=0) → centrepiece shape, modelFlags[0]
// |= 0x4. The per-shape loop finds "wall_0" (id=1) → shape 0, modelFlags[1]
// |= 0x4. Both are emitted now — the raw name is never dropped.
func TestPackLocConfigs_ModelFlags_0x4_ShapeSuffix(t *testing.T) {
	// LocShapeSuffix[0] is the suffix for shape 0.
	suffix0 := LocShapeSuffix[0]
	mp := newTestPF("model", map[int]string{
		0: "wall",
		1: "wall" + suffix0,
	})
	locPack := newTestPF("loc", map[int]string{0: "mywall"})
	configs := map[string][]ConfigLine{
		"mywall": {{Key: "model1", Value: "wall"}},
	}
	modelFlags := make([]int, 2)
	_, _, err := packLocConfigs(configs, locPack, mp, modelFlags)
	if err != nil {
		t.Fatalf("packLocConfigs: %v", err)
	}
	// "wall"+suffix0 (id=1) resolved via per-shape branch → 0x4.
	if modelFlags[1] != 0x4 {
		t.Errorf("modelFlags[1] = 0x%x, want 0x4 (per-shape branch)", modelFlags[1])
	}
	// "wall" (id=0) ALSO resolved via the raw-name centrepiece branch → 0x4
	// (no longer suppressed by directReference when a shape variant exists).
	if modelFlags[0] != 0x4 {
		t.Errorf("modelFlags[0] = 0x%x, want 0x4 (raw-name centrepiece branch)", modelFlags[0])
	}
}

// TestResolveLocModels_RawNameHit pins pin (a): a raw-name hit alone (no
// shape-suffix variants) resolves to the centrepiece shape and sets
// modelFlags. TS LocConfig.ts:329-336 @4c95f87e (upstream 3b653372) — the
// old directReference machinery required "door_8" for this; the simplified
// probe reads the raw name directly since Content dropped the `_8` filename
// suffix in the paired rename.
func TestResolveLocModels_RawNameHit(t *testing.T) {
	mp := newTestPF("model", map[int]string{
		0: "door", // raw name only, no shape suffixes in pack
	})
	modelFlags := make([]int, 1)
	models, err := resolveLocModels([]string{"door"}, mp, modelFlags, "test")
	if err != nil {
		t.Fatalf("resolveLocModels: %v", err)
	}
	if len(models) != 1 || models[0].model != 0 || models[0].shape != locShapeCentrepieceStraight {
		t.Fatalf("expected [{model:0 shape:10}], got %v", models)
	}
	if modelFlags[0] != 0x4 {
		t.Errorf("modelFlags[0] = 0x%x, want 0x4", modelFlags[0])
	}
}

// TestResolveLocModels_RawPlusShapedVariant pins pin (b): when BOTH the raw
// name and a shape-suffixed variant exist in modelPack, BOTH are resolved
// and emitted — the raw name as centrepiece (shape 10), the suffixed
// variant at its own shape. TS LocConfig.ts:329-346 @4c95f87e (upstream
// 3b653372). Under the old directReference logic, the presence of any
// shape-suffix variant flipped directReference to false and caused the raw
// name to be dropped entirely (only the suffixed variant was emitted); the
// simplified probe treats the two checks independently.
func TestResolveLocModels_RawPlusShapedVariant(t *testing.T) {
	// LocShapeSuffix[0] == "_1" (shape value 0).
	mp := newTestPF("model", map[int]string{
		0: "door",
		1: "door" + LocShapeSuffix[0],
	})
	modelFlags := make([]int, 2)
	models, err := resolveLocModels([]string{"door"}, mp, modelFlags, "test")
	if err != nil {
		t.Fatalf("resolveLocModels: %v", err)
	}
	want := []locModelShape{
		{model: 0, shape: locShapeCentrepieceStraight},
		{model: 1, shape: 0},
	}
	if len(models) != len(want) || models[0] != want[0] || models[1] != want[1] {
		t.Fatalf("got %v, want %v", models, want)
	}
	if modelFlags[0] != 0x4 {
		t.Errorf("modelFlags[0] = 0x%x, want 0x4 (raw-name centrepiece)", modelFlags[0])
	}
	if modelFlags[1] != 0x4 {
		t.Errorf("modelFlags[1] = 0x%x, want 0x4 (shape-suffix variant)", modelFlags[1])
	}
}

// TestResolveLocModels_NoMatches pins pin (c): no raw or suffixed match at
// all for a non-empty srcModels → error "failed to find suitable loc
// models" (unchanged from before this task).
func TestResolveLocModels_NoMatches(t *testing.T) {
	mp := newTestPF("model", map[int]string{0: "unrelated"})
	_, err := resolveLocModels([]string{"ghost"}, mp, nil, "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := err.Error(), "test: failed to find suitable loc models"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestPackLocConfigs_BreakroutefindingTrue pins that breakroutefinding=yes
// (parsed as bool true) emits client opcode 74 (0x4A) and nothing server-side.
// TS source: tools/pack/config/LocConfig.ts:60 (booleanKeys) + :307-310 (emission)
// @ 3c16994c.
func TestPackLocConfigs_BreakroutefindingTrue(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "breakroutefinding", Value: true}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 74 (0x4A) + Next 0x00 (no payload — same shape as forcedecor/73)
	want := []byte{0x00, 0x01, 0x4A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

// TestPackLocConfigs_BreakroutefindingFalse pins that breakroutefinding=no
// (parsed as bool false) emits nothing to the client buffer.
// TS source: LocConfig.ts:307-310 @ 3c16994c — emission fires only on value===true.
func TestPackLocConfigs_BreakroutefindingFalse(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "breakroutefinding", Value: false}}}
	_, client, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	// false → no opcode 74 emitted; client body is empty (just Next terminator).
	if !bytes.Equal(client.Dat.Data, locClientEmptyBody()) {
		t.Fatalf("got % x, want % x", client.Dat.Data, locClientEmptyBody())
	}
}

// TestPackLocConfigs_ModelFlags_NilSafe pins that nil modelFlags does not
// panic in resolveLocModels.
func TestPackLocConfigs_ModelFlags_NilSafe(t *testing.T) {
	mp := newTestPF("model", map[int]string{0: "pillar"})
	locPack := newTestPF("loc", map[int]string{0: "myobj"})
	configs := map[string][]ConfigLine{
		"myobj": {{Key: "model1", Value: "pillar"}},
	}
	// Must not panic.
	_, _, err := packLocConfigs(configs, locPack, mp, nil)
	if err != nil {
		t.Fatalf("packLocConfigs with nil modelFlags: %v", err)
	}
}
