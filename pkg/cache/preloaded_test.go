package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// resetPreloadedForTest clears Preloaded + PreloadedCRC and registers a
// t.Cleanup to clear them again after the test. Preloaded and PreloadedCRC
// are package-global maps that bleed between tests if not isolated.
func resetPreloadedForTest(t *testing.T) {
	t.Helper()
	for k := range Preloaded {
		delete(Preloaded, k)
	}
	for k := range PreloadedCRC {
		delete(PreloadedCRC, k)
	}
	t.Cleanup(func() {
		for k := range Preloaded {
			delete(Preloaded, k)
		}
		for k := range PreloadedCRC {
			delete(PreloadedCRC, k)
		}
	})
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

	cases := []struct {
		key   string
		bytes []byte
	}{
		{"m0_0", mapBytes},
		{"test.mid", songBytes},
		{"fanfare.mid", jingleBytes},
	}
	for _, c := range cases {
		got, ok := Preloaded[c.key]
		if !ok {
			t.Errorf("Preloaded[%q] missing", c.key)
			continue
		}
		if !bytes.Equal(got, c.bytes) {
			t.Errorf("Preloaded[%q] = %v, want %v", c.key, got, c.bytes)
		}
		gotCRC, ok := PreloadedCRC[c.key]
		if !ok {
			t.Errorf("PreloadedCRC[%q] missing", c.key)
			continue
		}
		wantCRC := packet.GetCRC(c.bytes, 0, len(c.bytes))
		if gotCRC != wantCRC {
			t.Errorf("PreloadedCRC[%q] = 0x%08x, want 0x%08x", c.key, gotCRC, wantCRC)
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
	if len(Preloaded) != 0 {
		t.Errorf("Preloaded has %d entries; want 0", len(Preloaded))
	}
	if len(PreloadedCRC) != 0 {
		t.Errorf("PreloadedCRC has %d entries; want 0", len(PreloadedCRC))
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
	for _, key := range []string{"placeholder", "empty.mid", "blank.mid"} {
		got, ok := Preloaded[key]
		if !ok || len(got) != 0 {
			t.Errorf("Preloaded[%q] = %v, want []byte{}", key, got)
		}
		if PreloadedCRC[key] != 0 {
			t.Errorf("PreloadedCRC[%q] = 0x%08x, want 0 (CRC32 of empty)", key, PreloadedCRC[key])
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
	if _, ok := Preloaded["sub"]; ok {
		t.Errorf("Preloaded[\"sub\"] should not be present (subdir skipped)")
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
	if got := Preloaded["shared.mid"]; !bytes.Equal(got, jingleBytes) {
		t.Errorf("Preloaded[\"shared.mid\"] = %v, want %v (jingles wins, dir-order semantics)", got, jingleBytes)
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
	got, ok := Preloaded["adventure.mid"]
	if !ok {
		t.Fatal("Preloaded[\"adventure.mid\"] missing after load against staged data")
	}
	if len(got) == 0 {
		t.Errorf("Preloaded[\"adventure.mid\"] is empty; want non-empty")
	}
	want, err := os.ReadFile(knownPath)
	if err != nil {
		t.Fatalf("read direct: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Preloaded[\"adventure.mid\"] does not match direct os.ReadFile of %s", knownPath)
	}
	if PreloadedCRC["adventure.mid"] != packet.GetCRC(want, 0, len(want)) {
		t.Errorf("PreloadedCRC[\"adventure.mid\"] does not match direct GetCRC")
	}
}
