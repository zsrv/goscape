package objtype

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestParseCategoryTypes_RoundTrip synthesises a 3-entry category.dat
// payload (count=3, three named entries) and asserts the parser
// produces matching Configs + ConfigNames. Mirrors TS CategoryType.parse
// (CategoryType.ts:21-37).
func TestParseCategoryTypes_RoundTrip(t *testing.T) {
	// Build payload: g2 count=3, then per-id [code=1, debugname+LF, end=0].
	buf := packet.NewPacket(nil)
	buf.P2(3)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		buf.P1(1)
		buf.PJStrLF(name)
		buf.P1(0)
	}

	cfgs, err := parseCategoryTypes(buf)
	if err != nil {
		t.Fatalf("parseCategoryTypes: %v", err)
	}
	if got := len(cfgs.Configs); got != 3 {
		t.Fatalf("Configs length: got %d, want 3", got)
	}
	wantNames := []string{"alpha", "beta", "gamma"}
	for i, want := range wantNames {
		c := cfgs.Configs[i]
		if c == nil {
			t.Errorf("Configs[%d] nil", i)
			continue
		}
		if c.ID != i {
			t.Errorf("Configs[%d].ID: got %d, want %d", i, c.ID, i)
		}
		if c.DebugName != want {
			t.Errorf("Configs[%d].DebugName: got %q, want %q", i, c.DebugName, want)
		}
		if id, ok := cfgs.ConfigNames[want]; !ok {
			t.Errorf("ConfigNames[%q] missing", want)
		} else if id != i {
			t.Errorf("ConfigNames[%q]: got %d, want %d", want, id, i)
		}
	}
}

// TestParseCategoryTypes_EmptyDebugNameSkipsConfigNames pins TS L33-35:
// only entries with a populated debugname enter the name index.
func TestParseCategoryTypes_EmptyDebugNameSkipsConfigNames(t *testing.T) {
	buf := packet.NewPacket(nil)
	buf.P2(2)
	// id 0: no code 1, just end-of-config.
	buf.P1(0)
	// id 1: code 1 + populated name.
	buf.P1(1)
	buf.PJStrLF("named")
	buf.P1(0)

	cfgs, err := parseCategoryTypes(buf)
	if err != nil {
		t.Fatalf("parseCategoryTypes: %v", err)
	}
	if got := len(cfgs.Configs); got != 2 {
		t.Fatalf("Configs length: got %d, want 2", got)
	}
	if cfgs.Configs[0].DebugName != "" {
		t.Errorf("Configs[0].DebugName: got %q, want empty", cfgs.Configs[0].DebugName)
	}
	if _, ok := cfgs.ConfigNames[""]; ok {
		t.Errorf("ConfigNames should not contain empty string")
	}
	if id, ok := cfgs.ConfigNames["named"]; !ok || id != 1 {
		t.Errorf("ConfigNames[\"named\"]: got %d, %v; want 1, true", id, ok)
	}
}

// TestLoadCategoryTypes_MissingFileReturnsEmpty pins the TS-faithful
// fail-soft on os.ErrNotExist (CategoryType.ts:13 fs.existsSync guard;
// goscape sibling precedent: fonttype.Load).
func TestLoadCategoryTypes_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	// Do NOT create server/category.dat.

	cfgs, err := LoadCategoryTypes(dir)
	if err != nil {
		t.Fatalf("LoadCategoryTypes on missing file: got err %v, want nil", err)
	}
	if cfgs == nil {
		t.Fatal("LoadCategoryTypes: got nil registry, want empty *CategoryTypeConfigs")
	}
	if len(cfgs.Configs) != 0 {
		t.Errorf("Configs length on missing file: got %d, want 0", len(cfgs.Configs))
	}
	if len(cfgs.ConfigNames) != 0 {
		t.Errorf("ConfigNames length on missing file: got %d, want 0", len(cfgs.ConfigNames))
	}
}

// TestLoadCategoryTypes_OtherErrorPropagates pins that non-NotExist I/O
// errors (e.g. permission denied) bubble up.
func TestLoadCategoryTypes_OtherErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	serverDir := filepath.Join(dir, "server")
	if err := os.Mkdir(serverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create the file as a DIRECTORY to force a non-NotExist read error.
	if err := os.Mkdir(filepath.Join(serverDir, "category.dat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCategoryTypes(dir); err == nil {
		t.Fatal("LoadCategoryTypes on bad file: got nil err, want non-nil")
	}
}
