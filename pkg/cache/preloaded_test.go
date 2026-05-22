package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// resetPreloadedForTest clears the snapshot pointer and registers a
// t.Cleanup to clear it again after the test. The atomic.Pointer is a
// package-global that bleeds between tests if not isolated.
func resetPreloadedForTest(t *testing.T) {
	t.Helper()
	ResetPreloadForTest()
	t.Cleanup(ResetPreloadForTest)
}

// writeFixture writes file with bytes inside <root>/<sub>/<name>, creating
// intermediate dirs. Test helper.
func writeFixture(t *testing.T, root, sub, name string, data []byte) {
	t.Helper()
	dir := filepath.Join(root, sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write %s/%s: %v", dir, name, err)
	}
}

func TestPreloadClient3DirWalkPopulatesBothMaps(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	mapBytes := []byte("land")
	songBytes := []byte{0xFF, 0x00}
	jingleBytes := []byte{0x01}
	writeFixture(t, root, "maps", "m0_0", mapBytes)
	writeFixture(t, root, "songs", "test.mid", songBytes)
	writeFixture(t, root, "jingles", "fanfare.mid", jingleBytes)

	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}

	snap := Preload()
	cases := []struct {
		key   string
		bytes []byte
	}{
		{"m0_0", mapBytes},
		{"test.mid", songBytes},
		{"fanfare.mid", jingleBytes},
	}
	for _, c := range cases {
		got, ok := snap.Data[c.key]
		if !ok {
			t.Errorf("Preload().Data[%q] missing", c.key)
			continue
		}
		if !bytes.Equal(got, c.bytes) {
			t.Errorf("Preload().Data[%q] = %v, want %v", c.key, got, c.bytes)
		}
		gotCRC, ok := snap.CRC[c.key]
		if !ok {
			t.Errorf("Preload().CRC[%q] missing", c.key)
			continue
		}
		wantCRC := packet.GetCRC(c.bytes, 0, len(c.bytes))
		if gotCRC != wantCRC {
			t.Errorf("Preload().CRC[%q] = 0x%08x, want 0x%08x", c.key, gotCRC, wantCRC)
		}
	}
}

func TestPreloadClientEmptyDirsOK(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"maps", "songs", "jingles"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	snap := Preload()
	if len(snap.Data) != 0 {
		t.Errorf("Preload().Data has %d entries; want 0", len(snap.Data))
	}
	if len(snap.CRC) != 0 {
		t.Errorf("Preload().CRC has %d entries; want 0", len(snap.CRC))
	}
}

func TestPreloadClientZeroByteFile(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	writeFixture(t, root, "maps", "placeholder", []byte{})
	writeFixture(t, root, "songs", "empty.mid", []byte{})
	writeFixture(t, root, "jingles", "blank.mid", []byte{})

	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	snap := Preload()
	for _, key := range []string{"placeholder", "empty.mid", "blank.mid"} {
		got, ok := snap.Data[key]
		if !ok || len(got) != 0 {
			t.Errorf("Preload().Data[%q] = %v, want []byte{}", key, got)
		}
		if snap.CRC[key] != 0 {
			t.Errorf("Preload().CRC[%q] = 0x%08x, want 0 (CRC32 of empty)", key, snap.CRC[key])
		}
	}
}

func TestPreloadClientSkipsSubdirs(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"maps", "songs", "jingles"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	// Create a stray subdir under maps/.
	if err := os.MkdirAll(filepath.Join(root, "maps", "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	if _, ok := Preload().Data["sub"]; ok {
		t.Errorf("Preload().Data[\"sub\"] should not be present (subdir skipped)")
	}
}

func TestPreloadClientMissingMapsDirReturnsError(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"songs", "jingles"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	err := PreloadClient(root)
	if err == nil {
		t.Fatal("PreloadClient: want error, got nil")
	}
	if !strings.Contains(err.Error(), "preload maps") {
		t.Errorf("error %q does not contain \"preload maps\"", err.Error())
	}
}

func TestPreloadClientMissingSongsDirReturnsError(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"maps", "jingles"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	err := PreloadClient(root)
	if err == nil {
		t.Fatal("PreloadClient: want error, got nil")
	}
	if !strings.Contains(err.Error(), "preload songs") {
		t.Errorf("error %q does not contain \"preload songs\"", err.Error())
	}
}

func TestPreloadClientMissingJinglesDirReturnsError(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"maps", "songs"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	err := PreloadClient(root)
	if err == nil {
		t.Fatal("PreloadClient: want error, got nil")
	}
	if !strings.Contains(err.Error(), "preload jingles") {
		t.Errorf("error %q does not contain \"preload jingles\"", err.Error())
	}
}

func TestPreloadClientKeyCollisionLastWins(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	mapBytes := []byte{0xAA}
	songBytes := []byte{0xBB}
	jingleBytes := []byte{0xCC}
	writeFixture(t, root, "maps", "shared.mid", mapBytes)
	writeFixture(t, root, "songs", "shared.mid", songBytes)
	writeFixture(t, root, "jingles", "shared.mid", jingleBytes)

	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	// Iteration order is maps → songs → jingles; last writer wins.
	if got := Preload().Data["shared.mid"]; !bytes.Equal(got, jingleBytes) {
		t.Errorf("Preload().Data[\"shared.mid\"] = %v, want %v (jingles wins, dir-order semantics)", got, jingleBytes)
	}
}

func TestPreloadClientAgainstStagedDataLoadsAdventure(t *testing.T) {
	resetPreloadedForTest(t)
	const knownPath = "../../data/pack/client/songs/adventure.mid"
	if _, err := os.Stat(knownPath); err != nil {
		t.Skipf("staged data not present at %s; skipping integration test", knownPath)
	}
	if err := PreloadClient("../../data/pack/client"); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	snap := Preload()
	got, ok := snap.Data["adventure.mid"]
	if !ok {
		t.Fatal("Preload().Data[\"adventure.mid\"] missing after load against staged data")
	}
	if len(got) == 0 {
		t.Errorf("Preload().Data[\"adventure.mid\"] is empty; want non-empty")
	}
	want, err := os.ReadFile(knownPath)
	if err != nil {
		t.Fatalf("read direct: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Preload().Data[\"adventure.mid\"] does not match direct os.ReadFile of %s", knownPath)
	}
	if snap.CRC["adventure.mid"] != packet.GetCRC(want, 0, len(want)) {
		t.Errorf("Preload().CRC[\"adventure.mid\"] does not match direct GetCRC")
	}
}

// TestPreloadClientSwapPreservesPriorOnError pins build-then-swap
// semantics: when PreloadClient fails partway, the previously-published
// snapshot remains intact (no leaked partial map into live state).
func TestPreloadClientSwapPreservesPriorOnError(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	writeFixture(t, root, "maps", "m0_0", []byte{0x11})
	writeFixture(t, root, "songs", "song.mid", []byte{0x22})
	writeFixture(t, root, "jingles", "jingle.mid", []byte{0x33})

	if err := PreloadClient(root); err != nil {
		t.Fatalf("first PreloadClient: %v", err)
	}
	priorMapBytes := Preload().Data["m0_0"]
	if len(priorMapBytes) == 0 {
		t.Fatal("setup: m0_0 not loaded")
	}

	// Build a second root with maps populated but jingles missing.
	bad := t.TempDir()
	writeFixture(t, bad, "maps", "m1_1", []byte{0xFF})
	writeFixture(t, bad, "songs", "s.mid", []byte{0xEE})
	// no jingles dir → error

	if err := PreloadClient(bad); err == nil {
		t.Fatal("PreloadClient: expected error from missing jingles dir, got nil")
	}

	// Prior snapshot must be unaffected.
	snap := Preload()
	if !bytes.Equal(snap.Data["m0_0"], []byte{0x11}) {
		t.Errorf("prior snapshot was clobbered: m0_0 = %v, want [0x11]", snap.Data["m0_0"])
	}
	if _, ok := snap.Data["m1_1"]; ok {
		t.Errorf("partial-build leaked: m1_1 should not be visible after failed PreloadClient")
	}
}
