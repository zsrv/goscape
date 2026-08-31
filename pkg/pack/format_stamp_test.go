package pack

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeOlderThanSource creates dest, then back-dates it so a plain mtime
// comparison would consider it FRESH relative to src (dest newer than src).
// Any rebuild the tests then observe must come from the format latch, not
// from mtimes.
func freshOutput(t *testing.T, srcDir, ext, out string) {
	t.Helper()
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "a"+ext)
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(src, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestBeginPackForcesRebuildOnFormatChange is the direct regression pin for
// the bug this guard exists for: the rev-274 sync moved config opcodes, and a
// `make pack` afterwards left 2,909 artifacts unrebuilt because freshness only
// looks at source mtimes.
//
// The output here is deliberately NEWER than its source, so mtime alone says
// "fresh". Only the format mismatch can make ShouldBuild return true.
func TestBeginPackForcesRebuildOnFormatChange(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	out := filepath.Join(dir, "out", "npc.dat")
	freshOutput(t, srcDir, ".npc", out)

	// A stamp from a DIFFERENT packer format.
	stampDir := filepath.Join(dir, "out", ".stamps")
	if err := os.MkdirAll(stampDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stampDir, formatStampName), []byte("format=999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	forced, restore := BeginPack(filepath.Join(dir, "out"))
	defer restore()

	if !forced {
		t.Fatal("BeginPack: got forced=false for a mismatched format, want true")
	}
	if !ShouldBuild(srcDir, ".npc", out) {
		t.Error("ShouldBuild: got false after a format change; the stale output would survive")
	}
	if !ShouldBuildFile(filepath.Join(srcDir, "a.npc"), out) {
		t.Error("ShouldBuildFile: got false after a format change")
	}
	if !ShouldBuildFileAny(srcDir, out) {
		t.Error("ShouldBuildFileAny: got false after a format change")
	}
	if !ForceRebuild() {
		t.Error("ForceRebuild: got false; presence-only gates (worldmap) would not rebuild")
	}
}

// TestBeginPackDoesNotForceOnMatchingFormat is the other half, and the one
// that actually matters for the guard being useful rather than merely safe.
//
// A guard that always fires would pass the test above while permanently
// destroying incrementality — which is precisely the failure mode the design
// rejected for the hash-the-executable option, since `make pack` stamps a
// fresh BuildDate into every binary. This asserts the latch stays OFF when
// the format is unchanged, so a fresh output is still skipped.
func TestBeginPackDoesNotForceOnMatchingFormat(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	out := filepath.Join(outDir, "npc.dat")
	freshOutput(t, srcDir, ".npc", out)

	if err := WriteFormatStamp(outDir); err != nil {
		t.Fatalf("WriteFormatStamp: %v", err)
	}

	forced, restore := BeginPack(outDir)
	defer restore()

	if forced {
		t.Fatal("BeginPack: got forced=true for a matching format, want false")
	}
	if ForceRebuild() {
		t.Fatal("ForceRebuild: latched on despite a matching format")
	}
	if ShouldBuild(srcDir, ".npc", out) {
		t.Error("ShouldBuild: got true for an up-to-date output; incrementality is broken")
	}
}

// TestBeginPackTreatsMissingStampAsStale pins the first-run behaviour. A tree
// packed before this guard shipped carries no stamp, so nothing is known about
// which packer built it — the only safe reading is "stale".
func TestBeginPackTreatsMissingStampAsStale(t *testing.T) {
	dir := t.TempDir()
	forced, restore := BeginPack(dir)
	defer restore()

	if !forced {
		t.Error("BeginPack: got forced=false for an unstamped tree, want true")
	}
}

// TestReadFormatStampRejectsMalformed pins that an unreadable stamp fails
// toward a rebuild rather than toward trusting the tree.
func TestReadFormatStampRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"no separator", "1\n"},
		{"non-numeric", "format=abc\n"},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, ".stamps"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".stamps", formatStampName), []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, found := ReadFormatStamp(dir); found {
				t.Errorf("ReadFormatStamp(%q): got found=true, want false", tc.content)
			}
		})
	}
}

// TestWriteFormatStampRoundTrips pins that what we write is what we read, so
// a correct pack does not force a rebuild on its immediate successor.
func TestWriteFormatStampRoundTrips(t *testing.T) {
	dir := t.TempDir()
	if err := WriteFormatStamp(dir); err != nil {
		t.Fatalf("WriteFormatStamp: %v", err)
	}
	version, found := ReadFormatStamp(dir)
	if !found {
		t.Fatal("ReadFormatStamp: got found=false right after writing")
	}
	if version != FormatVersion {
		t.Errorf("version: got %d, want %d", version, FormatVersion)
	}
}

// TestFormatStampPathMirrorsTSLayout pins the location. TS writes
// data/pack/.stamps/*.txt (FsCache.ts:165 @1d25566c); keeping the same shape
// means the two output trees can be diffed side by side without the stamp
// directory itself showing up as a structural difference.
func TestFormatStampPathMirrorsTSLayout(t *testing.T) {
	got := FormatStampPath("data/pack")
	want := filepath.Join("data", "pack", ".stamps", "pack-format.txt")
	if got != want {
		t.Errorf("FormatStampPath: got %q, want %q", got, want)
	}
}
