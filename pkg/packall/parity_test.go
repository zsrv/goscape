package packall

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// TestPackAll_Ref274FullTreeParity is the full-tree byte-parity acceptance
// gate. It runs PackAll over the real 274 content and asserts that the output
// matches the reference cache captured from Engine-TS dee467c8 + Content
// 7f97b0a5 (Node 24 toolchain).
//
// The test is skipped unless GOSCAPE_REF274_DIR is set to the engine directory
// of a Server274-ref checkout, e.g.:
//
//	GOSCAPE_REF274_DIR=/path/to/Server274-ref/engine go test ./pkg/packall/ \
//	  -run Ref274FullTreeParity -v -count=1
//
// Content is derived as $GOSCAPE_REF274_DIR/../content. rawDir is the repo's
// data/raw (contains the wordenc blob; not the engine copy).
//
// data/symbols is NOT part of the gate: upstream has no .sym pipeline (symbols
// live in-memory in @lostcityrs/runescript), so the reference cache has no
// symbols baseline. goscape's .sym export is a documented Go-only feature
// pinned by the self-consistency test (TestWriteCompilerSymbols_SelfConsistency).
//
// ── Structural note (274 vs prior pins) ─────────────────────────────────────
// At the 274 pin (TS map Pack.js @dee467c8) the map packer NO LONGER writes a
// loose maps tree. Land/loc land in the Jag store (cache.write(4, …)); the
// raw server map data (m/l/n/o) and gzip client map data (m/l) are cached in
// ArtifactCache zips (data/pack/.cache/maps-{server,client}.zip). goscape
// keeps its own loose maps layout (client/maps/*, server/maps/*) AND the Jag
// store. So the comparable surface against the 274 reference is:
//
//   - main_file_cache.dat + idx0-4   (the Jag store — maps live here too)
//   - client/{config,interface,media,sounds,textures,title,versionlist}
//   - server/*.dat / *.idx           (loose config)
//   - mapview/worldmap.jag
//
// These 56 files are in testdata/ref274_manifest.txt; all match byte-for-byte.
//
// The reference's data/pack/.cache/* (ArtifactCache stores) and .stamps/*
// (FsCache snapshots) are NOT-PORTED incremental-build infra (goscape does not
// emit them). The maps BYTES those zips cache are still covered: client land/
// loc live in the Jag store (manifest), and assertion C below cross-checks
// goscape's loose maps tree against the .cache/maps-{server,client}.zip
// entries — giving full byte coverage of every map file.
//
// Exemptions (not compared byte-for-byte against the manifest):
//   - server/maps/free2play.csv  — goscape-extra runtime copy; not in TS pack output.
//   - server/maps/multiway.csv   — goscape-extra runtime copy; not in TS pack output.
//     See pkg/pack/maps/pack.go for the deviation rationale.
//   - client/maps/*, server/maps/* — goscape's own loose layout (no per-file
//     274 reference; validated against the .cache zips in assertion C).
func TestPackAll_Ref274FullTreeParity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full-tree parity test in short mode (takes ~10-20s)")
	}

	ref := os.Getenv("GOSCAPE_REF274_DIR")
	if ref == "" {
		t.Skip("GOSCAPE_REF274_DIR not set; to run: " +
			"GOSCAPE_REF274_DIR=/path/to/Server274-ref/engine " +
			"go test ./pkg/packall/ -run Ref274FullTreeParity -v -count=1")
	}

	contentDir := filepath.Join(ref, "..", "content")
	if _, err := os.Stat(contentDir); err != nil {
		t.Fatalf("contentDir %q not found (derived from GOSCAPE_REF274_DIR/../content): %v", contentDir, err)
	}

	// rawDir is the goscape repo's data/raw, not the engine's copy. Tests run
	// with cwd = this package directory, so resolve it relatively.
	rawDir, err := filepath.Abs(filepath.Join("..", "..", "data", "raw"))
	if err != nil {
		t.Fatalf("resolve rawDir: %v", err)
	}
	if _, err := os.Stat(rawDir); err != nil {
		t.Fatalf("rawDir %q not found: %v (rawDir must be the goscape repo's data/raw)", rawDir, err)
	}

	outDir := t.TempDir()
	// dataPackDir: RunServerCompiler reads back the cache PackConfigs just wrote,
	// so pass outDir (same as the smoke test).
	if err := PackAll(contentDir, outDir, outDir, rawDir); err != nil {
		t.Fatalf("PackAll: %v", err)
	}

	// ── 1. Load manifest ────────────────────────────────────────────────────
	manifest, err := loadRef274Manifest(t)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	// ── 2. Assertion A: every manifest file matches sha256 ──────────────────
	var mismatches []string
	for relPath, wantHex := range manifest {
		// Manifest paths are all "data/pack/..." → outDir/<rest>.
		if !strings.HasPrefix(relPath, "data/pack/") {
			t.Errorf("manifest path %q has unexpected prefix (not data/pack/)", relPath)
			continue
		}
		absPath := filepath.Join(outDir, strings.TrimPrefix(relPath, "data/pack/"))

		gotHex, hashErr := sha256File(absPath)
		if hashErr != nil {
			mismatches = append(mismatches, fmt.Sprintf("MISSING  %s: %v", relPath, hashErr))
			continue
		}
		if gotHex != wantHex {
			mismatches = append(mismatches, fmt.Sprintf("SHA_MISMATCH %s\n  want %s\n  got  %s", relPath, wantHex, gotHex))
		}
	}
	if len(mismatches) > 0 {
		limit := 10
		if len(mismatches) < limit {
			limit = len(mismatches)
		}
		t.Errorf("%d manifest file(s) failed parity (showing first %d):\n%s",
			len(mismatches), limit, strings.Join(mismatches[:limit], "\n"))
	}

	// ── 3. Assertion B: no unexpected extra files ────────────────────────────
	// Walk outDir; every file must either be in the manifest, on the explicit
	// allowlist, or under goscape's own loose maps layout (client/maps/,
	// server/maps/ — validated in assertion C). The data/symbols sibling
	// PackAll writes is NOT walked: it is the Go-only .sym export.
	allowlisted := map[string]bool{
		"data/pack/server/maps/free2play.csv": true,
		"data/pack/server/maps/multiway.csv":  true,
	}
	// Prefixes for goscape's own loose maps layout (no per-file 274 reference;
	// byte-validated against the .cache artifact zips in assertion C).
	looseMapPrefixes := []string{
		"data/pack/client/maps/",
		"data/pack/server/maps/",
	}

	var unexpected []string
	_ = filepath.Walk(outDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		rel, _ := filepath.Rel(outDir, path)
		manifestKey := "data/pack/" + filepath.ToSlash(rel)
		if _, inManifest := manifest[manifestKey]; inManifest {
			return nil
		}
		if allowlisted[manifestKey] {
			return nil
		}
		for _, p := range looseMapPrefixes {
			if strings.HasPrefix(manifestKey, p) {
				return nil
			}
		}
		unexpected = append(unexpected, manifestKey)
		return nil
	})
	if len(unexpected) > 0 {
		t.Errorf("%d unexpected extra file(s) in output tree:\n  %s",
			len(unexpected), strings.Join(unexpected, "\n  "))
	}

	// ── 4. Assertion C: loose maps tree vs the reference .cache artifact zips ─
	// At 274 the reference packs maps into ArtifactCache zips, not a loose tree.
	// goscape emits a loose tree; assert every entry is byte-identical to the
	// corresponding zip member so the maps get full byte coverage.
	//   maps-client.zip: m<XZ>, l<XZ>  (gzip)  → client/maps/<key>
	//   maps-server.zip: m/l/n/o<XZ>   (raw)   → server/maps/<key>
	assertLooseMapsMatchZip(t,
		filepath.Join(ref, "data", "pack", ".cache", "maps-client.zip"),
		filepath.Join(outDir, "client", "maps"), "client/maps")
	assertLooseMapsMatchZip(t,
		filepath.Join(ref, "data", "pack", ".cache", "maps-server.zip"),
		filepath.Join(outDir, "server", "maps"), "server/maps")
}

// assertLooseMapsMatchZip compares every entry of a 274 ArtifactCache map zip
// against the corresponding loose file goscape emits under looseDir. Each zip
// entry name (e.g. "m40_55") maps to looseDir/<name>. Reports byte mismatches,
// missing loose files, and loose files with no zip counterpart.
func assertLooseMapsMatchZip(t *testing.T, zipPath, looseDir, label string) {
	t.Helper()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Errorf("%s: open artifact zip %q: %v", label, zipPath, err)
		return
	}
	defer zr.Close()

	zipKeys := make(map[string]bool)
	var mismatch, missing int
	var samples []string
	for _, ze := range zr.File {
		key := ze.Name
		zipKeys[key] = true
		rc, err := ze.Open()
		if err != nil {
			t.Errorf("%s: open zip entry %q: %v", label, key, err)
			continue
		}
		want, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Errorf("%s: read zip entry %q: %v", label, key, err)
			continue
		}
		got, err := os.ReadFile(filepath.Join(looseDir, key))
		if err != nil {
			missing++
			if len(samples) < 10 {
				samples = append(samples, fmt.Sprintf("MISSING %s/%s", label, key))
			}
			continue
		}
		if !bytes.Equal(got, want) {
			mismatch++
			if len(samples) < 10 {
				samples = append(samples, fmt.Sprintf("BYTE_DIFF %s/%s (zip=%d loose=%d)", label, key, len(want), len(got)))
			}
		}
	}
	t.Logf("%s: %d zip entries, %d mismatch, %d missing", label, len(zr.File), mismatch, missing)
	if mismatch > 0 || missing > 0 {
		t.Errorf("%s: %d byte-mismatch + %d missing loose map files vs reference zip:\n  %s",
			label, mismatch, missing, strings.Join(samples, "\n  "))
	}

	// Loose files with no zip counterpart (excluding goscape's csv extras).
	entries, _ := os.ReadDir(looseDir)
	var orphan []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "free2play.csv" || name == "multiway.csv" {
			continue
		}
		if !zipKeys[name] {
			orphan = append(orphan, name)
		}
	}
	if len(orphan) > 0 {
		sort.Strings(orphan)
		t.Errorf("%s: %d loose map file(s) with no reference zip entry: %v", label, len(orphan), orphan)
	}
}

// TestPackAll_OrigCacheParity asserts goscape's packed Jag store is byte-
// identical to the ORIGINAL shipped r274 cache — the real acceptance target.
//
// The original cache is a Jag store only (main_file_cache.dat + idx0-4). It is
// gated on GOSCAPE_ORIG_CACHE_DIR (default /home/owner/Code/_runescape/r274/
// original-cache, per T17b); skipped cleanly when absent.
//
// Compression archives (idx1=models, idx2=anims, idx3=midi, idx4=client maps)
// are compared with the 2-byte version trailer stripped (the version NUMBER is
// build metadata that differs — original uses real revisions, goscape/Node
// stamp version=1; the gzip BODY is the byte-parity target, proven 6201/6201
// at T17b). idx0 (the client jags: config/interface/media/…) is NOT compared:
// the original's client archives were built by a different toolchain than the
// pinned 274 Content rebuild — goscape matches the Node reference (the full-
// tree gate), which itself diverges from the original for idx0. That is an
// out-of-scope original-cache/client-toolchain divergence, not a packer bug.
//
// EXPECTED RESULT: full body parity for idx1-4 EXCEPT exactly two arch4
// (client-maps) slots — files 704 and 994 — which the original leaves empty
// (idx size 0) but Content packs real map data into. Documented content
// divergence (the Content map inputs include 2 maps the original omits). If
// MORE than those two differ, that is a real regression.
func TestPackAll_OrigCacheParity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping orig-cache parity test in short mode (runs PackAll, ~10-20s)")
	}

	orig := os.Getenv("GOSCAPE_ORIG_CACHE_DIR")
	if orig == "" {
		orig = "/home/owner/Code/_runescape/r274/original-cache"
	}
	if _, err := os.Stat(filepath.Join(orig, "main_file_cache.dat")); err != nil {
		t.Skipf("original cache absent (%s): %v", orig, err)
	}

	ref := os.Getenv("GOSCAPE_REF274_DIR")
	if ref == "" {
		t.Skip("GOSCAPE_REF274_DIR not set (needed for 274 content); to run set " +
			"both GOSCAPE_REF274_DIR and GOSCAPE_ORIG_CACHE_DIR")
	}
	contentDir := filepath.Join(ref, "..", "content")
	rawDir, err := filepath.Abs(filepath.Join("..", "..", "data", "raw"))
	if err != nil {
		t.Fatalf("resolve rawDir: %v", err)
	}

	outDir := t.TempDir()
	if err := PackAll(contentDir, outDir, outDir, rawDir); err != nil {
		t.Fatalf("PackAll: %v", err)
	}

	of := filestream.New(orig, false, true)
	defer of.Close()
	gf := filestream.New(outDir, false, true)
	defer gf.Close()

	stripVer := func(b []byte) []byte {
		if len(b) >= 2 {
			return b[:len(b)-2]
		}
		return b
	}

	// Compression archives (version trailer stripped → gzip body comparison).
	type slot struct{ arch, file int }
	expectedDiffs := map[slot]bool{
		{4, 704}: true, // empty in original, packed by Content
		{4, 994}: true, // empty in original, packed by Content
	}

	var unexpected []string
	matched := 0
	for arch := 1; arch <= 4; arch++ {
		oc := of.Count(arch)
		gc := gf.Count(arch)
		maxc := oc
		if gc > maxc {
			maxc = gc
		}
		for f := 0; f < maxc; f++ {
			ob := stripVer(of.Read(arch, f, false))
			gb := stripVer(gf.Read(arch, f, false))
			if bytes.Equal(ob, gb) {
				matched++
				continue
			}
			if expectedDiffs[slot{arch, f}] {
				continue
			}
			if len(unexpected) < 30 {
				unexpected = append(unexpected,
					fmt.Sprintf("arch%d/f%d (orig=%d goscape=%d)", arch, f, len(ob), len(gb)))
			}
		}
	}

	t.Logf("orig-cache parity: %d members body-matched (idx1-4, version trailer stripped); "+
		"%d expected map-slot diffs (arch4 f704, f994)", matched, len(expectedDiffs))

	if len(unexpected) > 0 {
		t.Errorf("orig-cache parity: %d UNEXPECTED member diff(s) beyond the 2 known empty "+
			"map slots:\n  %s", len(unexpected), strings.Join(unexpected, "\n  "))
	}

	// Positively confirm the 2 expected slots ARE empty in original and
	// non-empty in goscape (guards against a silent format change that would
	// otherwise let them pass as "matching").
	for s := range expectedDiffs {
		ob := of.Read(s.arch, s.file, false)
		gb := gf.Read(s.arch, s.file, false)
		if len(ob) != 0 {
			t.Errorf("arch%d/f%d: expected original to be EMPTY, got %d bytes (deviation no longer holds)", s.arch, s.file, len(ob))
		}
		if len(gb) == 0 {
			t.Errorf("arch%d/f%d: expected goscape to pack non-empty map data, got 0 bytes", s.arch, s.file)
		}
	}
}

// loadRef274Manifest reads testdata/ref274_manifest.txt and returns a map from
// manifest path (e.g. "data/pack/client/config") to its expected sha256 hex.
// The map doubles as a membership set for "in manifest" queries in the
// extra-files walk (presence check via _, ok := manifest[key]).
func loadRef274Manifest(t *testing.T) (map[string]string, error) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "ref274_manifest.txt"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("manifest: malformed line %q", line)
		}
		m[fields[1]] = fields[0]
	}
	return m, scanner.Err()
}

// sha256File returns the lowercase hex sha256 of the named file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
