package script

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
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
	_, err := p.Load(dir)
	if err == nil {
		t.Fatal("expected error for version mismatch, got nil")
	}
}

func TestProviderLoadRealCache(t *testing.T) {
	// Pinned to the rev-274 reference cache (Engine-TS dee467c8, compiler
	// version 27) — the repo's own data/pack is revision-specific generated
	// output that refreshes later in the migration (Task T24); per-branch
	// reference pinning avoids the cross-revision-cache hazard.
	cacheDir := "/home/owner/Code/github.com/LostCityRS/Server274-ref/engine/data/pack/server"
	if _, err := os.Stat(filepath.Join(cacheDir, "script.dat")); os.IsNotExist(err) {
		t.Skip("Server274-ref cache not present; skipping")
	}

	p := NewProvider()
	if _, err := p.Load(cacheDir); err != nil {
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

	specificKey := LookupKeyForType(trigger, typeID)
	categoryKey := LookupKeyForCategory(trigger, catID)
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
	if _, err := p.Load(dir); err != nil {
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

func TestGetByLookupKey(t *testing.T) {
	p := NewProvider()
	f := &ScriptFile{Name: "[test,key]", LookupKey: 0x1234}
	p.Register(f)

	if got := p.GetByLookupKey(0x1234); got != f {
		t.Errorf("GetByLookupKey(0x1234): got %v, want %v", got, f)
	}
	if got := p.GetByLookupKey(0x9999); got != nil {
		t.Errorf("GetByLookupKey(missing): got %v, want nil", got)
	}
}

func TestGetByTriggerSpecificTypeOnly(t *testing.T) {
	p := NewProvider()
	typeKey := LookupKeyForType(TriggerAdvanceStat, 0) // stat 0 = Attack
	sf := &ScriptFile{Name: "[advancestat,attack]", LookupKey: typeKey}
	p.Register(sf)

	got := p.GetByTriggerSpecific(TriggerAdvanceStat, 0, -1)
	if got != sf {
		t.Errorf("type-specific lookup: got %v, want %v", got, sf)
	}
}

func TestGetByTriggerSpecificCategoryOnly(t *testing.T) {
	p := NewProvider()
	catKey := LookupKeyForCategory(TriggerChangeStat, 7) // category 7
	sf := &ScriptFile{Name: "[changestat,_cat7]", LookupKey: catKey}
	p.Register(sf)

	got := p.GetByTriggerSpecific(TriggerChangeStat, -1, 7)
	if got != sf {
		t.Errorf("category-only lookup: got %v, want %v", got, sf)
	}
}

func TestGetByTriggerSpecificGlobalOnly(t *testing.T) {
	p := NewProvider()
	globalKey := uint32(TriggerChangeStat)
	sf := &ScriptFile{Name: "[changestat,_]", LookupKey: globalKey}
	p.Register(sf)

	got := p.GetByTriggerSpecific(TriggerChangeStat, -1, -1)
	if got != sf {
		t.Errorf("global-only lookup: got %v, want %v", got, sf)
	}
}

func TestGetByTriggerSpecificNoFallback(t *testing.T) {
	// Register ONLY the global tier; specific lookup must NOT fall through.
	p := NewProvider()
	globalKey := uint32(TriggerAdvanceStat)
	p.Register(&ScriptFile{Name: "[advancestat,_]", LookupKey: globalKey})

	got := p.GetByTriggerSpecific(TriggerAdvanceStat, 0, -1)
	if got != nil {
		t.Errorf("type-specific lookup with only-global registered: got %v, want nil (no fallback)", got)
	}
}

func TestGetByTriggerSpecificTypeShortCircuitsCategory(t *testing.T) {
	// type=5, cat=3 — only category script registered. Specific must return nil
	// because typeID != -1 picks the type tier and ignores the cat tier.
	p := NewProvider()
	catKey := LookupKeyForCategory(TriggerChangeStat, 3)
	p.Register(&ScriptFile{Name: "[changestat,_cat3]", LookupKey: catKey})

	got := p.GetByTriggerSpecific(TriggerChangeStat, 5, 3)
	if got != nil {
		t.Errorf("type-tier short-circuit: got %v, want nil (cat ignored when type set)", got)
	}
}

func TestGetByTriggerSpecificMissingReturnsNil(t *testing.T) {
	p := NewProvider() // empty
	if got := p.GetByTriggerSpecific(TriggerChangeStat, 0, -1); got != nil {
		t.Errorf("empty provider: got %v, want nil", got)
	}
}

func TestProviderLoad_ReturnsCountOfDecodedScripts(t *testing.T) {
	dir := t.TempDir()

	blob1 := buildTrivialScript("[proc,a]", 0xFFFFFFFF)
	blob2 := buildTrivialScript("[proc,b]", 0xFFFFFFFF)
	blob3 := buildTrivialScript("[proc,c]", 0xFFFFFFFF)

	dat := buildFakeDat(CompilerVersion, [][]byte{blob1, blob2, blob3})
	idx := buildFakeIdx([]int{len(blob1), len(blob2), len(blob3)})

	if err := os.WriteFile(filepath.Join(dir, "script.dat"), dat, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script.idx"), idx, 0644); err != nil {
		t.Fatal(err)
	}

	p := NewProvider()
	count, err := p.Load(dir)
	if err != nil {
		t.Fatalf("Load returned err: %v", err)
	}
	if count != 3 {
		t.Errorf("count: got %d, want 3", count)
	}
}

func TestProviderLoad_ReturnsMinusOneOnReadError(t *testing.T) {
	p := NewProvider()
	count, err := p.Load(t.TempDir()) // empty dir → script.dat ReadFile fails
	if err == nil {
		t.Fatal("Load expected to return error on missing dat")
	}
	if count != -1 {
		t.Errorf("count: got %d, want -1 on top-level error", count)
	}
}

func TestProviderNames(t *testing.T) {
	p := NewProvider()
	p.Register(&ScriptFile{Name: "[opheldu,tinderbox]", LookupKey: 0xFFFFFFFF})
	p.Register(&ScriptFile{Name: "[opheldu,logs]", LookupKey: 0xFFFFFFFF})
	p.Register(&ScriptFile{Name: "[proc,unrelated]", LookupKey: 0xFFFFFFFF})

	got := p.Names()
	slices.Sort(got)
	want := []string{"[opheldu,logs]", "[opheldu,tinderbox]", "[proc,unrelated]"}
	if !slices.Equal(got, want) {
		t.Errorf("Names(): got %v, want %v", got, want)
	}
}
