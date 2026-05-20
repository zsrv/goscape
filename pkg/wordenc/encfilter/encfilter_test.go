package encfilter

import (
	"os"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// makeSyntheticJag builds a minimal wordenc jagfile containing one entry in
// each of the 4 sections, mirroring the TS pack format (Engine-TS/src/cache/
// wordenc/WordEnc.ts:190-221 decoders + pkg/pack/wordenc/pack.go encoders).
func makeSyntheticJag(t *testing.T) *jagfile.Jagfile {
	t.Helper()
	jf := jagfile.NewEmptyJagfile(false)

	// badenc.txt: 1 entry "anal" with one combo 3:19.
	bad := packet.Alloc(2)
	bad.P4(1)
	bad.P1(4) // word length
	for _, c := range []byte("anal") {
		bad.P1(c)
	}
	bad.P1(1) // combo count
	bad.P1(3)
	bad.P1(19)
	jf.Write("badenc.txt", bad)

	// fragmentsenc.txt: 1 entry value 42.
	frag := packet.Alloc(2)
	frag.P4(1)
	frag.P2(42)
	jf.Write("fragmentsenc.txt", frag)

	// domainenc.txt: 1 entry "test".
	dom := packet.Alloc(2)
	dom.P4(1)
	dom.P1(4)
	for _, c := range []byte("test") {
		dom.P1(c)
	}
	jf.Write("domainenc.txt", dom)

	// tldlist.txt: 1 entry type=2 tld="com".
	tld := packet.Alloc(2)
	tld.P4(1)
	tld.P1(2) // tld type
	tld.P1(3)
	for _, c := range []byte("com") {
		tld.P1(c)
	}
	jf.Write("tldlist.txt", tld)

	// Round-trip through Save+NewJagfile so .FileQueue lands in .FileHash + .FileSize.
	tmpPath := t.TempDir() + "/wordenc.jag"
	if err := jf.Save(tmpPath); err != nil {
		t.Fatalf("Save synthetic jag: %v", err)
	}
	raw, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read synthetic jag: %v", err)
	}
	out, err := jagfile.NewJagfile(packet.NewPacket(raw))
	if err != nil {
		t.Fatalf("parse synthetic jag: %v", err)
	}
	return out
}

func TestLoadFromJag_DecodesAllFourSections(t *testing.T) {
	jf := makeSyntheticJag(t)
	f, err := LoadFromJag(jf)
	if err != nil {
		t.Fatalf("LoadFromJag: %v", err)
	}
	if got := len(f.bads); got != 1 {
		t.Errorf("bads: got %d, want 1", got)
	}
	if got := string(f.bads[0]); got != "anal" {
		t.Errorf("bads[0]: got %q, want %q", got, "anal")
	}
	if got := len(f.badCombos[0]); got != 1 {
		t.Errorf("badCombos[0]: got %d, want 1", got)
	}
	if f.badCombos[0][0] != [2]int{3, 19} {
		t.Errorf("badCombos[0][0]: got %v, want [3 19]", f.badCombos[0][0])
	}
	if got := f.fragments; len(got) != 1 || got[0] != 42 {
		t.Errorf("fragments: got %v, want [42]", got)
	}
	if got := len(f.domains); got != 1 || string(f.domains[0]) != "test" {
		t.Errorf("domains: got %v, want [test]", f.domains)
	}
	if got := len(f.tlds); got != 1 || string(f.tlds[0]) != "com" || f.tldTypes[0] != 2 {
		t.Errorf("tlds: got %v / types=%v, want [com] / [2]", f.tlds, f.tldTypes)
	}
}

func TestEmpty_FilterIsIdentity(t *testing.T) {
	f := Empty()
	got := f.Filter("hello world")
	if got != "hello world" {
		t.Errorf("Empty().Filter: got %q, want %q", got, "hello world")
	}
}
