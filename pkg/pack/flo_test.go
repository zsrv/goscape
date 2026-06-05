package pack

import (
	"bytes"
	"testing"
)

func floTestRegistries(t *testing.T) (texturePack *PackFile) {
	t.Helper()
	texturePack = newTestPF("texture", map[int]string{
		0: "wood",
		1: "stone",
	})
	return
}

func TestParseFloConfig_Colour(t *testing.T) {
	tp := floTestRegistries(t)
	parse := parseFloConfigFor(tp)
	val, accepted, err := parse("colour", "0xFF00AA")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("colour should be accepted")
	}
	if val.(int) != 0xFF00AA {
		t.Fatalf("got %#v, want int(0xFF00AA)", val)
	}
}

func TestParseFloConfig_Overlay(t *testing.T) {
	tp := floTestRegistries(t)
	parse := parseFloConfigFor(tp)
	val, accepted, err := parse("overlay", "yes")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("overlay should be accepted")
	}
	if val.(bool) != true {
		t.Fatalf("got %#v, want true", val)
	}
}

func TestParseFloConfig_Texture(t *testing.T) {
	tp := floTestRegistries(t)
	parse := parseFloConfigFor(tp)
	val, accepted, err := parse("texture", "stone")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("texture should be accepted")
	}
	if val.(int) != 1 {
		t.Fatalf("got %#v, want int(1)", val)
	}
}

func TestParseFloConfig_UnknownKey(t *testing.T) {
	tp := floTestRegistries(t)
	parse := parseFloConfigFor(tp)
	_, accepted, err := parse("zzz", "value")
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("unknown key should not be claimed")
	}
}

func floOneSlotPack(name string) *PackFile {
	return newTestPF("flo", map[int]string{0: name})
}

func TestPackFloConfigs_Colour(t *testing.T) {
	floPack := floOneSlotPack("red")
	configs := map[string][]ConfigLine{
		"red": {{Key: "colour", Value: 0xFF0000}},
	}
	_, client := packFloConfigs(configs, floPack, nil)
	want := []byte{
		0x00, 0x01,
		0x01, 0xFF, 0x00, 0x00,
		0x06, 'r', 'e', 'd', 0x0A,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_Texture(t *testing.T) {
	floPack := floOneSlotPack("flo_dirt")
	configs := map[string][]ConfigLine{
		"flo_dirt": {{Key: "texture", Value: 3}},
	}
	_, client := packFloConfigs(configs, floPack, nil)
	want := []byte{0x00, 0x01, 0x02, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_OverlayTrueEmits(t *testing.T) {
	floPack := floOneSlotPack("flo_x")
	configs := map[string][]ConfigLine{
		"flo_x": {{Key: "overlay", Value: true}},
	}
	_, client := packFloConfigs(configs, floPack, nil)
	want := []byte{0x00, 0x01, 0x03, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_OverlayFalseNoEmit(t *testing.T) {
	floPack := floOneSlotPack("flo_x")
	configs := map[string][]ConfigLine{
		"flo_x": {{Key: "overlay", Value: false}},
	}
	_, client := packFloConfigs(configs, floPack, nil)
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_OccludeFalseEmits(t *testing.T) {
	floPack := floOneSlotPack("flo_x")
	configs := map[string][]ConfigLine{
		"flo_x": {{Key: "occlude", Value: false}},
	}
	_, client := packFloConfigs(configs, floPack, nil)
	want := []byte{0x00, 0x01, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackFloConfigs_OccludeTrueNoEmit(t *testing.T) {
	floPack := floOneSlotPack("flo_x")
	configs := map[string][]ConfigLine{
		"flo_x": {{Key: "occlude", Value: true}},
	}
	_, client := packFloConfigs(configs, floPack, nil)
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

// Dual-asymmetry pin for opcode 6.
func TestPackFloConfigs_Opcode6_Asymmetry(t *testing.T) {
	cases := []struct {
		name string
		want []byte
	}{
		{"red", []byte{0x00, 0x01, 0x06, 'r', 'e', 'd', 0x0A, 0x00}},
		{"flo_dirt", []byte{0x00, 0x01, 0x00}},
	}
	for _, tc := range cases {
		floPack := floOneSlotPack(tc.name)
		_, client := packFloConfigs(map[string][]ConfigLine{}, floPack, nil)
		if !bytes.Equal(client.Dat.Data, tc.want) {
			t.Errorf("client[%s]:\n got % x\nwant % x", tc.name, client.Dat.Data, tc.want)
		}
	}
}

// Spec §9 R2: .flo server PackedData NEVER emits opcode bytes.
func TestPackFloConfigs_EmptyServerBytes(t *testing.T) {
	floPack := newTestPF("flo", map[int]string{
		0: "red",
		1: "blue",
		2: "green",
	})
	server, _ := packFloConfigs(map[string][]ConfigLine{}, floPack, nil)
	want := []byte{0x00, 0x03, 0x00, 0x00, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server (empty-bytes invariant):\n got % x\nwant % x", server.Dat.Data, want)
	}
}
