package checksum

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
)

// ---- synthetic helpers ----

// buildSimpleJag builds a jagfile with the given (name→data) members in order.
// Names must be in jagfile.knownNames for hash resolution to work.
func buildSimpleJag(members []struct {
	name string
	data []byte
}) *jagfile.Jagfile {
	jag := jagfile.NewEmptyJagfile(true)
	for _, m := range members {
		jag.Write(m.name, packet.NewPacket(m.data))
	}
	return jag
}

// writeCacheJag saves a jagfile into a FileStream cache at cacheDir, archive,
// file. When createNew=true the cache files are created from scratch.
func writeCacheJag(t *testing.T, cacheDir string, createNew bool, archive, file int, jag *jagfile.Jagfile) {
	t.Helper()
	dir := t.TempDir()
	tmpPath := filepath.Join(dir, "tmp.jag")
	if err := jag.Save(tmpPath); err != nil {
		t.Fatalf("jag.Save: %v", err)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read tmp jag: %v", err)
	}
	fs2 := filestream.New(cacheDir, createNew, false)
	if !fs2.Write(archive, file, data, 0) {
		t.Fatalf("filestream.Write(%d,%d) failed", archive, file)
	}
	fs2.Close()
}

// ---- unit tests ----

// TestRun_PrintsLinesAndExtractsFiles verifies Run with a tiny 3-jag cache:
// stdout has one line per member in directory order, and each member is
// extracted to <cacheDir>/<jagName>/<name>.
func TestRun_PrintsLinesAndExtractsFiles(t *testing.T) {
	cacheDir := t.TempDir()

	configMembers := []struct {
		name string
		data []byte
	}{
		{"flo.dat", []byte{0x10, 0x20}},
		{"flo.idx", []byte{0x30}},
	}
	ifaceMembers := []struct {
		name string
		data []byte
	}{
		{"data", []byte{0xAA, 0xBB, 0xCC}},
	}
	synthMembers := []struct {
		name string
		data []byte
	}{
		{"sounds.dat", []byte{0xFF}},
	}

	writeCacheJag(t, cacheDir, true, 0, 2, buildSimpleJag(configMembers))
	writeCacheJag(t, cacheDir, false, 0, 3, buildSimpleJag(ifaceMembers))
	writeCacheJag(t, cacheDir, false, 0, 8, buildSimpleJag(synthMembers))

	var out bytes.Buffer
	if err := Run(cacheDir, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	// expect 4 lines: 2 config + 1 interface + 1 synth
	if len(lines) != 4 {
		t.Fatalf("want 4 lines, got %d:\n%s", len(lines), out.String())
	}

	// Verify format: "<jagName> <memberName> <signedInt32>"
	if !strings.HasPrefix(lines[0], "config flo.dat ") {
		t.Errorf("line 0: want prefix 'config flo.dat ', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "config flo.idx ") {
		t.Errorf("line 1: want prefix 'config flo.idx ', got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "interface data ") {
		t.Errorf("line 2: want prefix 'interface data ', got %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], "synth sounds.dat ") {
		t.Errorf("line 3: want prefix 'synth sounds.dat ', got %q", lines[3])
	}

	// Verify extracted files exist with correct bytes.
	for _, m := range configMembers {
		got, err := os.ReadFile(filepath.Join(cacheDir, "config", m.name))
		if err != nil {
			t.Errorf("config/%s: read extracted file: %v", m.name, err)
			continue
		}
		if !bytes.Equal(got, m.data) {
			t.Errorf("config/%s: want %x got %x", m.name, m.data, got)
		}
	}
	got, err := os.ReadFile(filepath.Join(cacheDir, "interface", "data"))
	if err != nil {
		t.Errorf("interface/data: read extracted file: %v", err)
	} else if !bytes.Equal(got, ifaceMembers[0].data) {
		t.Errorf("interface/data: want %x got %x", ifaceMembers[0].data, got)
	}
	got2, err := os.ReadFile(filepath.Join(cacheDir, "synth", "sounds.dat"))
	if err != nil {
		t.Errorf("synth/sounds.dat: read extracted file: %v", err)
	} else if !bytes.Equal(got2, synthMembers[0].data) {
		t.Errorf("synth/sounds.dat: want %x got %x", synthMembers[0].data, got2)
	}
}

// TestRun_SignedInt32CRC verifies that the CRC is printed as a signed int32,
// matching JavaScript's console.log(Packet.getcrc(...)) semantics where
// getcrc returns ~crc (bitwise NOT yielding signed int32).
func TestRun_SignedInt32CRC(t *testing.T) {
	cacheDir := t.TempDir()

	// Use data whose uint32 CRC may have the high bit set (prints negative).
	npcData := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	configMembers := []struct {
		name string
		data []byte
	}{
		{"npc.dat", npcData},
	}
	ifaceMembers := []struct {
		name string
		data []byte
	}{
		{"data", []byte{0x00}},
	}
	synthMembers := []struct {
		name string
		data []byte
	}{
		{"sounds.dat", []byte{0x00}},
	}

	writeCacheJag(t, cacheDir, true, 0, 2, buildSimpleJag(configMembers))
	writeCacheJag(t, cacheDir, false, 0, 3, buildSimpleJag(ifaceMembers))
	writeCacheJag(t, cacheDir, false, 0, 8, buildSimpleJag(synthMembers))

	var out bytes.Buffer
	if err := Run(cacheDir, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	// line 0 is "config npc.dat <crc>"
	var crcVal int32
	if _, err := fmt.Sscanf(lines[0], "config npc.dat %d", &crcVal); err != nil {
		t.Fatalf("parse crc from %q: %v", lines[0], err)
	}

	// Compute expected signed int32 CRC.
	rawU32 := packet.GetCRC(npcData, 0, len(npcData))
	want := int32(rawU32)
	if crcVal != want {
		t.Errorf("CRC: want %d got %d (uint32=%d)", want, crcVal, rawU32)
	}
}

// ---- parity test ----

// TestChecksumParity is the env-gated full parity test for checksum.Run.
// It requires GOSCAPE_REF244_DIR to point at the engine directory of a
// Server244-ref checkout. Run with:
//
//	GOSCAPE_REF244_DIR=/path/to/Server244-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/checksum/ -run TestChecksumParity -v -count=1 -timeout 600s
func TestChecksumParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	var out bytes.Buffer
	if err := Run(cacheDir, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Cache changed-set: pristine vs post-run cacheDir.
	cachePristine := filepath.Join(refRoot, "unpack-ref", "cache")
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)

	// WROTE: files written to cacheDir since marker, with "CACHE:" prefix.
	wroteRaw := unpacktest.WroteSince(t, cacheDir, marker)
	wrote := make([]string, len(wroteRaw))
	for i, p := range wroteRaw {
		wrote[i] = "CACHE:" + p
	}

	r := unpacktest.Result{
		Content:      nil, // checksum does not write content
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       out.Bytes(),
		PostDir:      "",
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "checksum", r)
}
