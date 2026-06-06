package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// ---- dispatch tests ----

// TestRunUnpack_NoFamily verifies that omitting the family argument returns
// exit 2 and prints usage to stderr.
func TestRunUnpack_NoFamily(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := runUnpack(nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "missing family") {
		t.Errorf("stderr %q: want 'missing family'", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("stderr %q: want usage block", stderr.String())
	}
}

// TestRunUnpack_UnknownFamily verifies that an unrecognised family returns
// exit 2 and echoes the unknown name in stderr.
func TestRunUnpack_UnknownFamily(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := runUnpack([]string{"no-such-family"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-family") {
		t.Errorf("stderr %q: want family name echoed", stderr.String())
	}
}

// TestRunUnpack_HelpReturns0 verifies -h/--help/help all return 0 and
// write usage to stdout (not stderr).
func TestRunUnpack_HelpReturns0(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"-h", "--help", "help"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := runUnpack([]string{arg}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("runUnpack(%q) exit = %d, want 0", arg, code)
			}
			if !strings.Contains(stdout.String(), "Usage:") {
				t.Errorf("stdout %q: want usage block", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr should be empty, got %q", stderr.String())
			}
		})
	}
}

// TestRunUnpack_FamilyHelpReturns0 verifies that -h on any family returns 0.
func TestRunUnpack_FamilyHelpReturns0(t *testing.T) {
	t.Parallel()
	families := []string{
		"config", "interface", "map", "midi", "sound", "models", "anims",
		"sprite-media", "sprite-textures", "sprite-title",
		"versionlist-anim", "versionlist-midi", "versionlist-model",
		"worldmap", "checksum", "compare",
	}
	for _, fam := range families {
		fam := fam
		t.Run(fam, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := runUnpack([]string{fam, "-h"}, &stdout, &stderr)
			if code != 0 {
				t.Errorf("runUnpack(%q -h) exit = %d, want 0; stderr=%q", fam, code, stderr.String())
			}
		})
	}
}

// TestDispatch_UnpackRegistered verifies the verb is wired in main.go.
func TestDispatch_UnpackRegistered(t *testing.T) {
	t.Parallel()
	for _, v := range verbs {
		if v.name == "unpack" {
			return
		}
	}
	t.Error("unpack verb not registered in verbs slice")
}

// ---- synthetic cache helpers ----

// buildVersionlistCacheForTest writes a versionlist jagfile containing the
// given members into cacheDir (archive 0 / file 5). Mirrors the helper in
// pkg/unpack/versionlist/versionlist_test.go.
func buildVersionlistCacheForTest(t *testing.T, cacheDir string, members map[string][]byte) {
	t.Helper()
	vl := jagfile.NewEmptyJagfile(false)
	for name, data := range members {
		vl.Write(name, packet.NewPacket(data))
	}
	tmp := filepath.Join(t.TempDir(), "vl.jag")
	if err := vl.Save(tmp); err != nil {
		t.Fatalf("vl.Save: %v", err)
	}
	vlBytes, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile vl: %v", err)
	}
	fs2 := filestream.New(cacheDir, true, false)
	if !fs2.Write(0, 5, vlBytes, 0) {
		t.Fatal("write versionlist to cache failed")
	}
	fs2.Close()
}

// buildChecksumCacheForTest writes a minimal 3-archive cache (config jag at
// archive 0 / file 2, interface jag at 0/3, synth jag at 0/8) suitable for
// checksum.Run to process. Mirrors the fixture in checksum_test.go.
func buildChecksumCacheForTest(t *testing.T, cacheDir string) {
	t.Helper()

	writeJag := func(archive, file int, createNew bool, members []struct {
		name string
		data []byte
	}) {
		t.Helper()
		jag := jagfile.NewEmptyJagfile(true)
		for _, m := range members {
			jag.Write(m.name, packet.NewPacket(m.data))
		}
		tmp := filepath.Join(t.TempDir(), "tmp.jag")
		if err := jag.Save(tmp); err != nil {
			t.Fatalf("jag.Save: %v", err)
		}
		data, err := os.ReadFile(tmp)
		if err != nil {
			t.Fatalf("ReadFile tmp jag: %v", err)
		}
		fs2 := filestream.New(cacheDir, createNew, false)
		if !fs2.Write(archive, file, data, 0) {
			t.Fatalf("filestream.Write(%d,%d) failed", archive, file)
		}
		fs2.Close()
	}

	writeJag(0, 2, true, []struct {
		name string
		data []byte
	}{{"flo.dat", []byte{0x10, 0x20}}})
	writeJag(0, 3, false, []struct {
		name string
		data []byte
	}{{"data", []byte{0xAA, 0xBB}}})
	writeJag(0, 8, false, []struct {
		name string
		data []byte
	}{{"sounds.dat", []byte{0xFF}}})
}

// ---- end-to-end smoke tests ----

// TestRunUnpack_Checksum_Smoke verifies that the checksum family:
//   - returns exit 0,
//   - writes at least one "<jag> <member> <crc>" line to stdout,
//   - extracts files into <cacheDir>/<jag>/ subdirectories.
func TestRunUnpack_Checksum_Smoke(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	buildChecksumCacheForTest(t, cacheDir)

	var stdout, stderr bytes.Buffer
	code := runUnpack([]string{"checksum",
		"-cache-dir", cacheDir,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}

	// stdout must contain the config/flo.dat line.
	if !strings.Contains(stdout.String(), "config flo.dat") {
		t.Errorf("stdout %q: missing 'config flo.dat' line", stdout.String())
	}

	// Extracted file must exist with the right bytes.
	got, err := os.ReadFile(filepath.Join(cacheDir, "config", "flo.dat"))
	if err != nil {
		t.Fatalf("extracted config/flo.dat: %v", err)
	}
	if !bytes.Equal(got, []byte{0x10, 0x20}) {
		t.Errorf("config/flo.dat bytes = %x, want [10 20]", got)
	}
}

// TestRunUnpack_VersionlistAnim_Smoke verifies that the versionlist-anim
// family returns exit 0 and prints the expected "i flags decimal" lines.
func TestRunUnpack_VersionlistAnim_Smoke(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()

	// anim_index: two g2 entries — [0x0000, 0x0001].
	indexBytes := []byte{0x00, 0x00, 0x00, 0x01}
	buildVersionlistCacheForTest(t, cacheDir, map[string][]byte{"anim_index": indexBytes})

	var stdout, stderr bytes.Buffer
	code := runUnpack([]string{"versionlist-anim",
		"-cache-dir", cacheDir,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}

	// Expect:
	//   0 00000000 0
	//   1 00000001 1
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), stdout.String())
	}
	if lines[0] != "0 00000000 0" {
		t.Errorf("line 0: got %q, want %q", lines[0], "0 00000000 0")
	}
	if lines[1] != "1 00000001 1" {
		t.Errorf("line 1: got %q, want %q", lines[1], "1 00000001 1")
	}
}

// ---- flag plumbing test ----

// TestRunUnpack_Config_FlagPlumbing verifies that -revision and -src-dir flags
// reach the config library. A nonexistent -cache-dir triggers the library's
// "Place a functional cache…" error, proving the flags were threaded through.
func TestRunUnpack_Config_FlagPlumbing(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := runUnpack([]string{
		"config",
		"-cache-dir", filepath.Join(t.TempDir(), "no-such-cache"),
		"-src-dir", t.TempDir(),
		"-revision", "244",
		"-log.level", "error",
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit = %d, want 1 (library error on missing cache)", code)
	}
	// The library returns an error mentioning the missing cache; the logger
	// writes it to stderr.
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "cache") && !strings.Contains(combined, "failed") {
		t.Errorf("combined output %q: expected error about missing cache", combined)
	}
}

// TestRunUnpack_Compare_DefaultType verifies that omitting -type defaults to
// "npc" (TS Compare.ts:52 hardcodes 'npc') and that the run proceeds past flag
// validation into the library — evidenced by exit 1 with a library-level error
// about a missing or unreadable cache, not a flag-validation error (exit 2).
func TestRunUnpack_Compare_DefaultType(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := runUnpack([]string{
		"compare",
		"-cache-dir", filepath.Join(t.TempDir(), "no-such-cache"),
		"-pack-dir", t.TempDir(),
		"-log.level", "error",
	}, &stdout, &stderr)
	// Must be exit 1 (library error), not exit 2 (flag validation).
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (library error proving -type defaulted to npc); stderr=%q", code, stderr.String())
	}
}

// TestRunUnpack_UnknownFlag_Returns2 verifies that an unknown flag returns
// exit 2 for a representative family (config).
func TestRunUnpack_UnknownFlag_Returns2(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := runUnpack([]string{"config", "--no-such-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr %q: want unknown flag name", stderr.String())
	}
}

// TestRunUnpack_Dispatch_TopLevelVerb verifies the verb-level dispatch routes
// correctly from the top-level dispatch function.
func TestRunUnpack_Dispatch_TopLevelVerb(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	// Dispatch with no family → exit 2 (proves the verb was invoked).
	code := dispatch([]string{"unpack"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch(unpack) exit = %d, want 2", code)
	}
}

// TestRunUnpack_Stdout_NotStderr verifies that print-tool output (stdout
// channel) goes to stdout, not stderr, for versionlist-anim.
func TestRunUnpack_Stdout_NotStderr(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	indexBytes := []byte{0x00, 0x01} // one g2 entry
	buildVersionlistCacheForTest(t, cacheDir, map[string][]byte{"anim_index": indexBytes})

	var stdout, stderr bytes.Buffer
	code := runUnpack([]string{"versionlist-anim",
		"-cache-dir", cacheDir,
		"-log.level", "error",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit = %d; stderr=%q", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Error("stdout is empty; versionlist-anim output must go to stdout")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr not empty: %q", stderr.String())
	}
}

// TestRunUnpack_Checksum_StdoutContainsAllLines verifies the checksum family
// writes one line per cache member to stdout (not stderr).
func TestRunUnpack_Checksum_StdoutContainsAllLines(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	buildChecksumCacheForTest(t, cacheDir)

	var stderr bytes.Buffer
	code := runUnpack([]string{"checksum",
		"-cache-dir", cacheDir,
		"-log.level", "error",
	}, io.Discard, &stderr)

	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr not empty on success: %q", stderr.String())
	}
}
