package pack

import (
	"bytes"
	"testing"
)

func seqTestRegistries(t *testing.T) (animPack, objPack *PackFile) {
	t.Helper()
	animPack = newTestPF("anim", map[int]string{
		0: "frame_zero",
		1: "frame_one",
		2: "iframe_zero",
	})
	objPack = newTestPF("obj", map[int]string{
		0: "sword",
		1: "shield",
	})
	return
}

func TestParseSeqConfig_Loops(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)

	val, accepted, err := parse("loops", "5")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("loops should be accepted")
	}
	if val.(int) != 5 {
		t.Fatalf("got %#v, want int(5)", val)
	}
}

func TestParseSeqConfig_LoopsRangeReject(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)
	if _, _, err := parse("loops", "1001"); err == nil {
		t.Fatal("loops=1001 should reject (TS upper bound 1000)")
	}
}

func TestParseSeqConfig_Frame(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)
	val, accepted, err := parse("frame1", "frame_zero")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("frame1 should be accepted")
	}
	if val.(int) != 0 {
		t.Fatalf("got %#v, want int(0)", val)
	}
}

func TestParseSeqConfig_FrameUnknown(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)
	if _, _, err := parse("frame1", "no_such_anim"); err == nil {
		t.Fatal("unknown anim should reject")
	}
}

func TestParseSeqConfig_Walkmerge(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)
	val, accepted, err := parse("walkmerge", "label_3,label_7,label_11")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("walkmerge should be accepted")
	}
	got, ok := val.([]int)
	if !ok {
		t.Fatalf("got %#v, want []int", val)
	}
	want := []int{3, 7, 11}
	if len(got) != len(want) || got[0] != 3 || got[1] != 7 || got[2] != 11 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseSeqConfig_ReplaceHeldLeft_Hide(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)
	val, accepted, err := parse("replaceheldleft", "hide")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("replaceheldleft=hide should be accepted")
	}
	if val.(int) != 0 {
		t.Fatalf("got %#v, want int(0)", val)
	}
}

func TestParseSeqConfig_ReplaceHeldRight_ObjPlus512(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)
	val, accepted, err := parse("replaceheldright", "shield")
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("replaceheldright should be accepted")
	}
	// shield is obj id 1 → 1 + 512 = 513.
	if val.(int) != 513 {
		t.Fatalf("got %#v, want int(513)", val)
	}
}

func TestParseSeqConfig_UnknownKey(t *testing.T) {
	ap, op := seqTestRegistries(t)
	parse := parseSeqConfigFor(ap, op)
	_, accepted, err := parse("zzz_unknown", "anything")
	if err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("unknown key should not be claimed")
	}
}

func seqOneSlotPack(name string) *PackFile {
	return newTestPF("seq", map[int]string{0: name})
}

func seqServerDebugTrailer(name string) []byte {
	buf := []byte{0x00, 0x01}
	buf = append(buf, 0xFA)
	buf = append(buf, []byte(name)...)
	buf = append(buf, 0x0A, 0x00)
	return buf
}

func TestPackSeqConfigs_Loops(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {{Key: "loops", Value: 5}},
	}
	_, client := packSeqConfigs(configs, seqPack)
	want := []byte{0x00, 0x01, 0x02, 0x00, 0x05, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSeqConfigs_ServerDebugTrailer(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{}
	server, _ := packSeqConfigs(configs, seqPack)
	if !bytes.Equal(server.Dat.Data, seqServerDebugTrailer("walk")) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, seqServerDebugTrailer("walk"))
	}
}

func TestPackSeqConfigs_NoDebugnameNoTrailer(t *testing.T) {
	pf := newTestPF("seq", map[int]string{0: ""}) // explicit empty name
	configs := map[string][]ConfigLine{}
	server, _ := packSeqConfigs(configs, pf)
	want := []byte{0x00, 0x01, 0x00}
	if !bytes.Equal(server.Dat.Data, want) {
		t.Fatalf("server:\n got % x\nwant % x", server.Dat.Data, want)
	}
}

func TestPackSeqConfigs_FramesIframesDelays(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {
			{Key: "frame1", Value: 10},
			{Key: "frame2", Value: 11},
			{Key: "iframe1", Value: 20},
			{Key: "delay2", Value: 7},
		},
	}
	_, client := packSeqConfigs(configs, seqPack)
	want := []byte{
		0x00, 0x01,
		0x01, 0x02,
		0x00, 0x0A, 0x00, 0x14, 0x00, 0x00,
		0x00, 0x0B, 0xFF, 0xFF, 0x00, 0x07,
		0x00,
	}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSeqConfigs_ReplaceHeldRight_Hide(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {{Key: "replaceheldright", Value: 0}},
	}
	_, client := packSeqConfigs(configs, seqPack)
	want := []byte{0x00, 0x01, 0x07, 0x00, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSeqConfigs_ReplaceHeldRight_ObjPlus512(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {{Key: "replaceheldright", Value: 513}},
	}
	_, client := packSeqConfigs(configs, seqPack)
	want := []byte{0x00, 0x01, 0x07, 0x02, 0x01, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}

func TestPackSeqConfigs_Walkmerge(t *testing.T) {
	seqPack := seqOneSlotPack("walk")
	configs := map[string][]ConfigLine{
		"walk": {{Key: "walkmerge", Value: []int{3, 7, 11}}},
	}
	_, client := packSeqConfigs(configs, seqPack)
	want := []byte{0x00, 0x01, 0x03, 0x03, 0x03, 0x07, 0x0B, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client:\n got % x\nwant % x", client.Dat.Data, want)
	}
}
