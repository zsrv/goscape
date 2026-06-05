package pack

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/colorconv"
)

func idkTestRegistries(t *testing.T) (modelPack *PackFile) {
	t.Helper()
	modelPack = newTestPF("model", map[int]string{
		0: "m_zero",
		1: "m_one",
	})
	return
}

func TestParseIdkConfig_Type(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)
	cases := map[string]int{
		"man_hair":    0,
		"man_jaw":     1,
		"woman_torso": 9,
		"woman_feet":  13,
	}
	for name, want := range cases {
		val, accepted, err := parse("type", name)
		if err != nil {
			t.Errorf("type=%s: %v", name, err)
			continue
		}
		if !accepted {
			t.Errorf("type=%s should be accepted", name)
			continue
		}
		if val.(int) != want {
			t.Errorf("type=%s: got %#v, want int(%d)", name, val, want)
		}
	}
}

func TestParseIdkConfig_TypeUnknown(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)
	if _, _, err := parse("type", "no_such_part"); err == nil {
		t.Fatal("unknown type should reject")
	}
}

func TestParseIdkConfig_Model(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)
	val, accepted, err := parse("model1", "m_one")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("model1 should be accepted")
	}
	if val.(int) != 1 {
		t.Fatalf("got %#v, want int(1)", val)
	}
}

func TestParseIdkConfig_Head(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)
	val, accepted, err := parse("head1", "m_zero")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("head1 should be accepted")
	}
	if val.(int) != 0 {
		t.Fatalf("got %#v, want int(0)", val)
	}
}

func TestParseIdkConfig_RecolColorConvertedAtParser(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)
	val, accepted, err := parse("recol1s", "0x1234")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("recol1s should be accepted")
	}
	want := colorconv.Rgb15toHsl16(0x1234)
	if val.(int) != want {
		t.Fatalf("got %#v, want Rgb15toHsl16(0x1234)=%d", val, want)
	}
}

func TestParseIdkConfig_Disable(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)
	val, accepted, err := parse("disable", "yes")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("disable should be accepted")
	}
	if val.(bool) != true {
		t.Fatalf("got %#v, want true", val)
	}
}

func TestParseIdkConfig_UnknownKey(t *testing.T) {
	mp := idkTestRegistries(t)
	parse := parseIdkConfigFor(mp)
	_, accepted, err := parse("zzz_unknown", "anything")
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("unknown key should not be claimed")
	}
}

func idkOneSlotPack(name string) *PackFile {
	return newTestPF("idk", map[int]string{0: name})
}

func idkServerDebugTrailer(name string) []byte {
	buf := []byte{0x00, 0x01}
	buf = append(buf, 0xFA)
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0x0A, 0x00)
	return buf
}

func TestPackIdkConfigs_Type(t *testing.T) {
	pf := idkOneSlotPack("man_hair_0")
	configs := map[string][]ConfigLine{
		"man_hair_0": {{Key: "type", Value: 0}},
	}
	_, client := packIdkConfigs(configs, pf, nil)
	want := []byte{0x00, 0x01, 0x01, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_DisableTrueEmits(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {{Key: "disable", Value: true}},
	}
	_, client := packIdkConfigs(configs, pf, nil)
	want := []byte{0x00, 0x01, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_Models(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {
			{Key: "model1", Value: 7},
			{Key: "model2", Value: 9},
		},
	}
	_, client := packIdkConfigs(configs, pf, nil)
	want := []byte{0x00, 0x01, 0x02, 0x02, 0x00, 0x07, 0x00, 0x09, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_Heads(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {
			{Key: "head1", Value: 4},
			{Key: "head2", Value: 5},
		},
	}
	_, client := packIdkConfigs(configs, pf, nil)
	want := []byte{0x00, 0x01, 60, 0x00, 0x04, 61, 0x00, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_RecolSrcDst(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {
			{Key: "recol1s", Value: 0x1234},
			{Key: "recol1d", Value: 0x5678},
		},
	}
	_, client := packIdkConfigs(configs, pf, nil)
	want := []byte{0x00, 0x01, 40, 0x12, 0x34, 50, 0x56, 0x78, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackIdkConfigs_ServerDebugTrailer(t *testing.T) {
	pf := idkOneSlotPack("man_hair_0")
	server, _ := packIdkConfigs(map[string][]ConfigLine{}, pf, nil)
	if !bytes.Equal(server.Dat.Data, idkServerDebugTrailer("man_hair_0")) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, idkServerDebugTrailer("man_hair_0"))
	}
}

func TestPackIdkConfigs_NoDebugnameNoTrailer(t *testing.T) {
	pf := newTestPF("idk", map[int]string{0: ""})
	server, _ := packIdkConfigs(map[string][]ConfigLine{}, pf, nil)
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackIdkConfigs_DisableFalseNoEmit(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {{Key: "disable", Value: false}},
	}
	_, client := packIdkConfigs(configs, pf, nil)
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// TestPackIdkConfigs_ModelFlags_0x80 pins that packIdkConfigs writes 0x80
// into modelFlags for each model and head model id, per TS
// IdkConfig.ts:146,150 @ 9aadcec4.
func TestPackIdkConfigs_ModelFlags_0x80(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {
			{Key: "model1", Value: 3},
			{Key: "head1", Value: 7},
		},
	}
	modelFlags := make([]int, 10)
	packIdkConfigs(configs, pf, modelFlags)
	if modelFlags[3] != 0x80 {
		t.Errorf("modelFlags[3] = 0x%x, want 0x80 (model1 id=3)", modelFlags[3])
	}
	if modelFlags[7] != 0x80 {
		t.Errorf("modelFlags[7] = 0x%x, want 0x80 (head1 id=7)", modelFlags[7])
	}
	// Unrelated slots must be zero.
	for _, idx := range []int{0, 1, 2, 4, 5, 6, 8, 9} {
		if modelFlags[idx] != 0 {
			t.Errorf("modelFlags[%d] = 0x%x, want 0 (untouched)", idx, modelFlags[idx])
		}
	}
}

// TestPackIdkConfigs_ModelFlags_NilSafe pins that passing nil for modelFlags
// does not panic (nil guard required for backward-compat callers).
func TestPackIdkConfigs_ModelFlags_NilSafe(t *testing.T) {
	pf := idkOneSlotPack("idk0")
	configs := map[string][]ConfigLine{
		"idk0": {
			{Key: "model1", Value: 3},
			{Key: "head1", Value: 7},
		},
	}
	// Must not panic.
	packIdkConfigs(configs, pf, nil)
}
