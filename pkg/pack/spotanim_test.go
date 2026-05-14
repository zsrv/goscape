package pack

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/colorconv"
)

func spotanimTestRegistries(t *testing.T) (modelPack, seqPack *PackFile) {
	t.Helper()
	modelPack = newTestPF("model", map[int]string{
		0: "m_zero",
		1: "m_one",
	})
	seqPack = newTestPF("seq", map[int]string{
		0: "anim_zero",
	})
	return
}

func TestParseSpotAnimConfig_Model(t *testing.T) {
	mp, sp := spotanimTestRegistries(t)
	parse := parseSpotAnimConfigFor(mp, sp)
	val, accepted, err := parse("model", "m_one")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("model should be accepted")
	}
	if val.(int) != 1 {
		t.Fatalf("got %#v, want int(1)", val)
	}
}

func TestParseSpotAnimConfig_AngleRange(t *testing.T) {
	mp, sp := spotanimTestRegistries(t)
	parse := parseSpotAnimConfigFor(mp, sp)
	if _, _, err := parse("angle", "361"); err == nil {
		t.Fatal("angle=361 should reject (TS range 0..360)")
	}
}

func TestParseSpotAnimConfig_AmbientNegativeOK(t *testing.T) {
	mp, sp := spotanimTestRegistries(t)
	parse := parseSpotAnimConfigFor(mp, sp)
	val, _, err := parse("ambient", "-100")
	if err != nil {
		t.Fatal(err)
	}
	if val.(int) != -100 {
		t.Fatalf("got %#v, want int(-100)", val)
	}
}

func TestParseSpotAnimConfig_Recol_ColorConvertedAtParser(t *testing.T) {
	mp, sp := spotanimTestRegistries(t)
	parse := parseSpotAnimConfigFor(mp, sp)
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

func spotanimOneSlotPack(name string) *PackFile {
	return newTestPF("spotanim", map[int]string{0: name})
}

func spotanimServerDebugTrailer(name string) []byte {
	buf := []byte{0x00, 0x01}
	buf = append(buf, 0xFA)
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0x0A, 0x00)
	return buf
}

func TestPackSpotAnimConfigs_Model(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	configs := map[string][]ConfigLine{
		"flame": {{Key: "model", Value: 7}},
	}
	_, client := packSpotAnimConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x01, 0x00, 0x07, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSpotAnimConfigs_HasAlpha_TrueEmits(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	configs := map[string][]ConfigLine{
		"flame": {{Key: "hasalpha", Value: true}},
	}
	_, client := packSpotAnimConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSpotAnimConfigs_HasAlpha_FalseNoEmit(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	configs := map[string][]ConfigLine{
		"flame": {{Key: "hasalpha", Value: false}},
	}
	_, client := packSpotAnimConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSpotAnimConfigs_RecolSrcDst(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	configs := map[string][]ConfigLine{
		"flame": {
			{Key: "recol1s", Value: 0x1234}, // → opcode 40
			{Key: "recol1d", Value: 0x5678}, // → opcode 50
		},
	}
	_, client := packSpotAnimConfigs(configs, pf)
	want := []byte{
		0x00, 0x01,
		40, 0x12, 0x34,
		50, 0x56, 0x78,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSpotAnimConfigs_ServerDebugTrailer(t *testing.T) {
	pf := spotanimOneSlotPack("flame")
	server, _ := packSpotAnimConfigs(map[string][]ConfigLine{}, pf)
	if !bytes.Equal(server.Dat.Data, spotanimServerDebugTrailer("flame")) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, spotanimServerDebugTrailer("flame"))
	}
}

func TestParseSpotAnimConfig_UnknownKey(t *testing.T) {
	mp, sp := spotanimTestRegistries(t)
	parse := parseSpotAnimConfigFor(mp, sp)
	_, accepted, err := parse("zzz_unknown", "anything")
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("unknown key should not be claimed")
	}
}

func TestPackSpotAnimConfigs_NoDebugnameNoTrailer(t *testing.T) {
	pf := newTestPF("spotanim", map[int]string{0: ""})
	server, _ := packSpotAnimConfigs(map[string][]ConfigLine{}, pf)
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}
