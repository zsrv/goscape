package script

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildFakeDat writes a minimal script.dat with the given version and one script entry.
func buildFakeDat(version int, entries [][]byte) []byte {
	var dat []byte
	dat = binary.BigEndian.AppendUint32(dat, uint32(len(entries)))
	dat = binary.BigEndian.AppendUint32(dat, uint32(version))
	for _, e := range entries {
		dat = append(dat, e...)
	}
	return dat
}

// buildFakeIdx writes a minimal script.idx for the given entry sizes.
func buildFakeIdx(sizes []int) []byte {
	var idx []byte
	idx = binary.BigEndian.AppendUint32(idx, uint32(len(sizes))) // entryCount header
	for _, s := range sizes {
		idx = binary.BigEndian.AppendUint32(idx, uint32(s))
	}
	return idx
}

// buildTrivialScript builds the smallest decodable script blob.
func buildTrivialScript(name string, lookupKey uint32) []byte {
	instrs := []testInstr{{op: OpReturn, intOp: 0}}
	return buildScript(name, "test.rs2", lookupKey, 0, 0, 0, 0, instrs)
}

func TestProviderRejectsVersionMismatch(t *testing.T) {
	dir := t.TempDir()

	blob := buildTrivialScript("[proc,x]", 0xFFFFFFFF)
	dat := buildFakeDat(19, [][]byte{blob}) // wrong version
	idx := buildFakeIdx([]int{len(blob)})

	if err := os.WriteFile(filepath.Join(dir, "script.dat"), dat, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script.idx"), idx, 0644); err != nil {
		t.Fatal(err)
	}

	p := NewProvider()
	err := p.Load(dir)
	if err == nil {
		t.Fatal("expected error for version mismatch, got nil")
	}
}

func TestProviderLoadRealCache(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack", "server")
	if _, err := os.Stat(filepath.Join(cacheDir, "script.dat")); os.IsNotExist(err) {
		t.Skip("real cache not present; skipping")
	}

	p := NewProvider()
	if err := p.Load(cacheDir); err != nil {
		t.Fatalf("Load real cache: %v", err)
	}
	if p.Count() == 0 {
		t.Fatal("expected non-zero script count")
	}

	// GetByName should return at least one script.
	var found *ScriptFile
	for name := range p.byName {
		found = p.GetByName(name)
		if found != nil {
			break
		}
	}
	if found == nil {
		t.Fatal("GetByName returned nil for all known names")
	}
}

func TestProviderGetByTriggerFallback(t *testing.T) {
	// Build three scripts with specific/category/global lookup keys for trigger=5.
	trigger := ServerTriggerType(5)
	typeID := 10
	catID := 3

	specificKey := uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
	categoryKey := uint32(trigger) | (0x1 << 8) | (uint32(catID) << 10)
	globalKey := uint32(trigger)

	p := NewProvider()
	p.byKey = make(map[uint32]*ScriptFile)
	p.byName = make(map[string]*ScriptFile)

	specific := &ScriptFile{Name: "specific", LookupKey: specificKey}
	category := &ScriptFile{Name: "category", LookupKey: categoryKey}
	global := &ScriptFile{Name: "global", LookupKey: globalKey}

	p.byKey[specificKey] = specific
	p.byKey[categoryKey] = category
	p.byKey[globalKey] = global

	// All three present: specific wins.
	got := p.GetByTrigger(trigger, typeID, catID)
	if got != specific {
		t.Errorf("expected specific, got %v", got)
	}

	// Remove specific: category wins.
	delete(p.byKey, specificKey)
	got = p.GetByTrigger(trigger, typeID, catID)
	if got != category {
		t.Errorf("expected category, got %v", got)
	}

	// Remove category: global wins.
	delete(p.byKey, categoryKey)
	got = p.GetByTrigger(trigger, typeID, catID)
	if got != global {
		t.Errorf("expected global, got %v", got)
	}

	// Remove global: nil.
	delete(p.byKey, globalKey)
	got = p.GetByTrigger(trigger, typeID, catID)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestProviderByNameUnique(t *testing.T) {
	// When two scripts share a name, the last-decoded wins (map overwrite).
	dir := t.TempDir()

	blob1 := buildTrivialScript("[proc,x]", 0xFFFFFFFF)
	blob2 := buildTrivialScript("[proc,x]", 0xFFFFFFFF) // same name, different blob

	dat := buildFakeDat(CompilerVersion, [][]byte{blob1, blob2})
	idx := buildFakeIdx([]int{len(blob1), len(blob2)})

	if err := os.WriteFile(filepath.Join(dir, "script.dat"), dat, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script.idx"), idx, 0644); err != nil {
		t.Fatal(err)
	}

	p := NewProvider()
	if err := p.Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}

	f := p.GetByName("[proc,x]")
	if f == nil {
		t.Fatal("GetByName returned nil for existing name")
	}
	// Both blobs decode to the same content; just assert we got one back.
	if f.Name != "[proc,x]" {
		t.Errorf("name: got %q want %q", f.Name, "[proc,x]")
	}
}
