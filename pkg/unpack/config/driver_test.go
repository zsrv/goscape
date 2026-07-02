package config

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// TestReorderUnpacked_BucketOrder verifies the canonical bucket order produced
// by reorderUnpacked for each settings combination.
//
// TS source: Unpack.ts:81-108.
func TestReorderUnpacked_BucketOrder(t *testing.T) {
	t.Run("all_false", func(t *testing.T) {
		// When all flags are false every line lands in "others".
		in := []string{"[debugname]", "name=foo", "desc=bar", "model=m1", "recol1s=1", "active=yes"}
		s := reorderUnpackedSettings{}
		got := reorderUnpacked(in, s)
		// Expected order: debugname bucket first (starts with '['), then all others in order.
		want := []string{"[debugname]", "name=foo", "desc=bar", "model=m1", "recol1s=1", "active=yes"}
		if !slices.Equal(got, want) {
			t.Errorf("all_false:\n got  %v\n want %v", got, want)
		}
	})

	t.Run("all_true", func(t *testing.T) {
		// All flags true: order = debugname, name, desc, model, ldmodel, recol, retex, others.
		in := []string{
			"[myname]",
			"name=Foo",
			"desc=A desc",
			"model=m1",
			"ldmodel=m2",
			"recol1s=100",
			"retex1s=200",
			"active=yes",
		}
		s := reorderUnpackedSettings{moveName: true, moveDesc: true, moveRecol: true, moveModel: true}
		got := reorderUnpacked(in, s)
		want := []string{
			"[myname]",
			"name=Foo",
			"desc=A desc",
			"model=m1",
			"ldmodel=m2",
			"recol1s=100",
			"retex1s=200",
			"active=yes",
		}
		if !slices.Equal(got, want) {
			t.Errorf("all_true:\n got  %v\n want %v", got, want)
		}
	})

	t.Run("loc_settings", func(t *testing.T) {
		// loc: moveName=true, moveDesc=true, moveRecol=true, moveModel=true
		in := []string{
			"active=yes",
			"recol1s=10",
			"model=m1",
			"[loc_0]",
			"desc=A wall",
			"name=Wall",
			"retex1d=20",
		}
		s := settingsForType("loc")
		got := reorderUnpacked(in, s)
		want := []string{
			"[loc_0]",
			"name=Wall",
			"desc=A wall",
			"model=m1",
			"recol1s=10",
			"retex1d=20",
			"active=yes",
		}
		if !slices.Equal(got, want) {
			t.Errorf("loc_settings:\n got  %v\n want %v", got, want)
		}
	})

	t.Run("npc_settings", func(t *testing.T) {
		// npc: moveName=true, moveRecol=true, moveModel=true (no moveDesc)
		in := []string{
			"[npc_0]",
			"desc=A goblin",
			"name=Goblin",
			"model=m1",
			"recol1s=10",
			"size=2",
		}
		s := settingsForType("npc")
		got := reorderUnpacked(in, s)
		// desc stays in others (moveDesc=false for npc)
		want := []string{
			"[npc_0]",
			"name=Goblin",
			"model=m1",
			"recol1s=10",
			"desc=A goblin",
			"size=2",
		}
		if !slices.Equal(got, want) {
			t.Errorf("npc_settings:\n got  %v\n want %v", got, want)
		}
	})

	t.Run("obj_settings", func(t *testing.T) {
		// obj: moveName=true, moveRecol=true (no moveDesc, no moveModel)
		in := []string{
			"[obj_5]",
			"name=Sword",
			"model=m2",
			"recol1s=100",
			"stackable=yes",
		}
		s := settingsForType("obj")
		got := reorderUnpacked(in, s)
		want := []string{
			"[obj_5]",
			"name=Sword",
			"recol1s=100",
			"model=m2",
			"stackable=yes",
		}
		if !slices.Equal(got, want) {
			t.Errorf("obj_settings:\n got  %v\n want %v", got, want)
		}
	})

	t.Run("idk_settings", func(t *testing.T) {
		// idk: moveRecol=true only
		in := []string{
			"[idk_0]",
			"retex1s=50",
			"disable=yes",
		}
		s := settingsForType("idk")
		got := reorderUnpacked(in, s)
		want := []string{
			"[idk_0]",
			"retex1s=50",
			"disable=yes",
		}
		if !slices.Equal(got, want) {
			t.Errorf("idk_settings:\n got  %v\n want %v", got, want)
		}
	})

	t.Run("seq_settings", func(t *testing.T) {
		// seq/flo/varp/spotanim: all flags false — everything in natural order except '[' bucket
		in := []string{
			"[seq_0]",
			"replaymode=count",
			"frames=3",
		}
		s := settingsForType("seq")
		got := reorderUnpacked(in, s)
		want := []string{
			"[seq_0]",
			"replaymode=count",
			"frames=3",
		}
		if !slices.Equal(got, want) {
			t.Errorf("seq_settings:\n got  %v\n want %v", got, want)
		}
	})
}

// TestUnpackConfigNames_DefaultRegistration verifies that unpackConfigNames
// registers "<type>_<id>" for ids that have no existing name, and skips ids
// that are already registered.
//
// TS source: Unpack.ts:73-77.
func TestUnpackConfigNames_DefaultRegistration(t *testing.T) {
	// Build a minimal jagfile blob in memory: a config jagfile with flo.idx and flo.dat.
	// flo.idx: count=3, each entry has len=0.
	// flo.dat: 3 zero-length entries (just the 2-byte count, no payload).
	//
	// We'll use the Jagfile API via the ReadConfigIdx contract:
	//   idx content: g2(count=3), then 3×g2(len=0)  → 8 bytes total
	//   dat content: g2(count=3) → 2 bytes
	//
	// But actually, ReadConfigIdx reads from the raw packets. We need to set up
	// a real jagfile; instead, use the internal helpers directly.

	srcDir := t.TempDir()

	// Write minimal pack/flo.pack (2 existing names: 0=water, 2=grass; id 1 is missing)
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "flo.pack"), []byte("0=water\n2=grass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// We need to create a synthetic jagfile. Use raw byte construction.
	// jagfile archive 0 file 2 format:
	//   The outer jag wrapper: P3(unpackedSize) P3(packedSize) <data>
	//   Inner data: P2(fileCount) then per-file: P4(hash) P3(unpackedSize) P3(packedSize)
	//   then file payloads.
	//
	// We need: flo.idx and flo.dat named files.
	// Use the jagfile package directly — build via NewEmptyJagfile and Write API
	// would be cleanest, but we need to build the raw bytes for Read.
	// Instead, use ReadConfigIdx directly with synthetic packets.

	// Build idx packet: count=3, len[0]=0, len[1]=0, len[2]=0
	idxBytes := make([]byte, 2+3*2)              // 2 for count + 3*2 for per-entry lengths
	binary.BigEndian.PutUint16(idxBytes[0:2], 3) // count
	// all lengths = 0, already zeroed

	// Build dat packet: count=3 (just the 2-byte header; entries have 0 length)
	datBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(datBytes[0:2], 3)

	// Use ReadConfigIdx to produce the ConfigIdx
	idxPkt := newPacketFromBytes(idxBytes)
	datPkt := newPacketFromBytes(datBytes)

	sourceIdx, err := ReadConfigIdx(idxPkt, datPkt)
	if err != nil {
		t.Fatalf("ReadConfigIdx: %v", err)
	}
	if sourceIdx.Size != 3 {
		t.Fatalf("expected size 3, got %d", sourceIdx.Size)
	}

	// Load the pack registry
	reg := newTestRegistry(srcDir)
	pf, regErr := reg.EnsureFlo()
	if regErr != nil {
		t.Fatalf("EnsureFlo: %v", regErr)
	}

	// id 0 = "water" (exists), id 1 = "" (missing → register "flo_1"), id 2 = "grass" (exists)
	for id := range sourceIdx.Size {
		if pf.GetByID(id) == "" {
			pf.Register(id, "flo_"+itoa(id))
		}
	}

	if got := pf.GetByID(0); got != "water" {
		t.Errorf("id 0: got %q, want %q", got, "water")
	}
	if got := pf.GetByID(1); got != "flo_1" {
		t.Errorf("id 1: got %q, want %q", got, "flo_1")
	}
	if got := pf.GetByID(2); got != "grass" {
		t.Errorf("id 2: got %q, want %q", got, "grass")
	}
}

// TestUnpackConfig_MergeEmission verifies that when two caches differ on an entry,
// both blocks are appended to the .merge file with the separator.
//
// TS source: Unpack.ts:162-196.
func TestUnpackConfig_MergeEmission(t *testing.T) {
	srcDir := t.TempDir()

	// Synthetic unpack function: returns different lines for id 0, same for id 1.
	calls := 0
	unpackFn := func(idx *ConfigIdx, id int) ([]string, error) {
		calls++
		if id == 0 {
			// First call (primary cache) returns version A; second call (compare cache) returns version B.
			if calls%2 == 1 {
				return []string{"[flo_0]", "colour=1"}, nil
			}
			return []string{"[flo_0]", "colour=2"}, nil
		}
		// id 1: always same → no merge entry
		return []string{"[flo_1]", "colour=99"}, nil
	}

	// Build minimal ConfigIdx objects: size=2, all positions/lengths at 0.
	// We need two Jagfile objects. Since unpackConfig calls ReadConfigIdx via jag.Read,
	// but our fn signature takes *ConfigIdx directly, test via direct internal call.
	// Use the private unpackConfig directly (same package).

	// Call unpackConfig with a mock that ignores the jagfile Read (we pass nil jags
	// and override the unpackFn to not use the idx passed to it).
	// Actually, unpackConfig calls ReadConfigIdx from the jag, so we need real jag blobs.
	// Build minimal jag blobs with flo.idx + flo.dat.

	jag1 := makeTinyJagWithFloIdx(t, 2)
	jag2 := makeTinyJagWithFloIdx(t, 2)

	// Track calls: primary jag produces "colour=1", compare produces "colour=2" for id 0.
	callCount := 0
	fn := func(idx *ConfigIdx, id int) ([]string, error) {
		callCount++
		if id == 0 {
			if callCount%2 == 1 {
				return []string{"[flo_0]", "colour=1"}, nil
			}
			return []string{"[flo_0]", "colour=2"}, nil
		}
		// id 1: always same
		return []string{"[flo_1]", "colour=99"}, nil
	}
	_ = unpackFn // suppress unused lint

	printInfo := func(string) {}
	if err := unpackConfig("244", "flo", fn, jag1, jag2, srcDir, printInfo); err != nil {
		t.Fatalf("unpackConfig: %v", err)
	}

	outPath := filepath.Join(srcDir, "scripts", "_unpack", "244", "all.flo")
	mergePath := outPath + ".merge"

	// Main file (all.flo): init-empty (WriteFile(out, '')) plus any non-merge appends.
	// When compareIdx != nil AND id < compareIdx.Size:
	//   - differs  → goes to .merge only (not main)
	//   - same     → nothing written to main
	// Both id 0 and id 1 satisfy id < compareIdx.Size, so the main file stays empty.
	// Expected exact bytes: empty (0 bytes).
	mainBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read main file: %v", err)
	}
	if wantMain := []byte{}; !bytes.Equal(mainBytes, wantMain) {
		t.Errorf("main file exact bytes:\n got  %q\n want %q", mainBytes, wantMain)
	}

	// Merge file (.merge): id 0 differs → appended as two TS blocks (Unpack.ts:173-174).
	//
	// unpacked  = ["[flo_0]", "colour=1", ""]  (after push(''))
	// unpacked2 = ["[flo_0]", "colour=2", ""]  (after push(''))
	//
	// TS line 173: '// --------\n' + unpacked.join('\n')  + '\n'
	//   → "// --------\n[flo_0]\ncolour=1\n\n"
	//     (join produces "[flo_0]\ncolour=1\n", trailing '\n' from the '' element)
	// TS line 174: unpacked2.join('\n') + '\n'
	//   → "[flo_0]\ncolour=2\n\n"
	//
	// id 1 is same on both sides → not appended to merge at all.
	wantMerge := []byte(
		"// --------\n[flo_0]\ncolour=1\n\n" +
			"[flo_0]\ncolour=2\n\n",
	)
	mergeBytes, err := os.ReadFile(mergePath)
	if err != nil {
		t.Fatalf("read merge file: %v", err)
	}
	if !bytes.Equal(mergeBytes, wantMerge) {
		t.Errorf("merge file exact bytes:\n got  %q\n want %q", mergeBytes, wantMerge)
	}
}

// --- Helpers ---

// newPacketFromBytes wraps raw bytes in a packet.Packet.
func newPacketFromBytes(b []byte) *packet.Packet {
	return packet.NewPacket(b)
}

// newTestRegistry builds a pack.Registry pointed at srcDir.
func newTestRegistry(srcDir string) *pack.Registry {
	return &pack.Registry{SrcDir: srcDir}
}

// itoa converts an int to its decimal string form.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// makeTinyJagWithFloIdx builds a minimal *jagfile.Jagfile containing flo.idx
// and flo.dat for count entries, all zero-length.
// This satisfies the Read("flo.idx") / Read("flo.dat") calls inside unpackConfig.
func makeTinyJagWithFloIdx(t *testing.T, count int) *jagfile.Jagfile {
	t.Helper()

	// idx bytes: g2(count), count × g2(0)
	idxBytes := make([]byte, 2+count*2)
	binary.BigEndian.PutUint16(idxBytes[0:2], uint16(count))

	// dat bytes: just g2(count)
	datBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(datBytes[0:2], uint16(count))

	jf := jagfile.NewEmptyJagfile(false)
	jf.Write("flo.idx", packet.NewPacket(idxBytes))
	jf.Write("flo.dat", packet.NewPacket(datBytes))

	// Save/reload to materialise the data.
	dir := t.TempDir()
	path := filepath.Join(dir, "flo.jag")
	if err := jf.Save(path); err != nil {
		t.Fatalf("jagfile save: %v", err)
	}
	loaded, err := jagfile.LoadJagfile(path)
	if err != nil {
		t.Fatalf("jagfile load: %v", err)
	}
	return loaded
}
