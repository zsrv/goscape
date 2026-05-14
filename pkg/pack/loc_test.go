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
	mp, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(mp, cp, sp, tp, lk, pt)

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
	mp, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(mp, cp, sp, tp, lk, pt)

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
	mp, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(mp, cp, sp, tp, lk, pt)

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
	mp, cp, sp, tp, pt, lk := locTestRegistries(t)
	parse := parseLocConfigFor(mp, cp, sp, tp, lk, pt)

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
	server, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x18, 0x00, 0x07, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_HasalphaTrue(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "hasalpha", Value: true}}}
	_, client, err := packLocConfigs(configs, locPack, mp)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x19, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Wallwidth(t *testing.T) {
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("x")
	configs := map[string][]ConfigLine{"x": {{Key: "wallwidth", Value: 5}}}
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	server, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	_, client, err := packLocConfigs(configs, locPack, mp)
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
	// "chair" exists in modelPack and has no shape-suffix variants → forced shape _8.
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("c")
	configs := map[string][]ConfigLine{"c": {{Key: "model1", Value: "chair"}}}
	_, client, err := packLocConfigs(configs, locPack, mp)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 1 + p1(count=1) + p2(modelID=1) + p1(shape=10)
	// + opcode 2 + pjstr("c\n")  (shape _8 forces name-transmit)
	// + Next
	want := []byte{0x00, 0x01, 0x01, 0x01, 0x00, 0x01, 0x0A, 0x02, 'c', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
	}
}

func TestPackLocConfigs_Models_DirectMatchWithShape8Variant(t *testing.T) {
	// "table" matches exactly (id=0) AND "table_8" exists (id=2). TS
	// directReference probe scans shape 0..22 SKIPPING shape 10 (_8),
	// so the "_8" variant alone does NOT flip directReference to false.
	// Result: directReference wins → emit id=0 forced into shape _8.
	mp, _, _, _, _, _ := locTestRegistries(t)
	locPack := locOneSlotPack("t")
	configs := map[string][]ConfigLine{"t": {{Key: "model1", Value: "table"}}}
	_, client, err := packLocConfigs(configs, locPack, mp)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 1 + p1(1) + p2(modelID=0 for "table") + p1(shape=10)
	// + opcode 2 + pjstr("t\n") (shape _8 forces name) + Next
	want := []byte{0x00, 0x01, 0x01, 0x01, 0x00, 0x00, 0x0A, 0x02, 't', 0x0A, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("got % x, want % x", client.Dat.Data, want)
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
	server, _, err := packLocConfigs(configs, locPack, mp)
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
	server, _, err := packLocConfigs(configs, locPack, mp)
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
	server, _, err := packLocConfigs(configs, locPack, mp)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("got % x, want % x", server.Dat.Data, want)
	}
}
