package packall

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPackAll_Ref254FullTreeParity is the full-tree byte-parity acceptance
// gate. It runs PackAll over the real 254 content and asserts that the output
// matches the reference cache captured from Engine-TS 2e3bcf43 + Content caee3f2e.
//
// The test is skipped unless GOSCAPE_REF254_DIR is set to the engine directory
// of a Server254-ref checkout, e.g.:
//
//	GOSCAPE_REF254_DIR=/path/to/Server254-ref/engine go test ./pkg/packall/ \
//	  -run Ref254FullTreeParity -v -count=1
//
// Content is derived as $GOSCAPE_REF254_DIR/../content. rawDir is the repo's
// data/raw (contains the wordenc blob; not the engine copy).
//
// data/symbols is NOT part of the gate: upstream deleted the .sym pipeline at
// 2e3bcf43, so the reference cache has no symbols baseline. goscape's .sym
// export is a documented Go-only feature pinned by the synthetic-fixture
// tests in pkg/pack/compiler (see symbols_export_ref_parity_test.go).
//
// Exemptions (not compared byte-for-byte against the manifest):
//   - server/build          — 4-byte wall-clock timestamp; asserted to be exactly 4 bytes.
//   - ondemand.zip          — zip container bytes differ (Go archive/zip vs fflate);
//     entry-level content parity is verified via ref254_ondemand_entries.txt.
//   - server/maps/free2play.csv  — goscape-extra runtime copy; not in TS pack output.
//   - server/maps/multiway.csv   — goscape-extra runtime copy; not in TS pack output.
//     See pkg/pack/maps/pack.go for the deviation rationale.
func TestPackAll_Ref254FullTreeParity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full-tree parity test in short mode (takes ~10-20s)")
	}

	ref := os.Getenv("GOSCAPE_REF254_DIR")
	if ref == "" {
		t.Skip("GOSCAPE_REF254_DIR not set; to run: " +
			"GOSCAPE_REF254_DIR=/path/to/Server254-ref/engine " +
			"go test ./pkg/packall/ -run Ref254FullTreeParity -v -count=1")
	}

	contentDir := filepath.Join(ref, "..", "content")
	if _, err := os.Stat(contentDir); err != nil {
		t.Fatalf("contentDir %q not found (derived from GOSCAPE_REF254_DIR/../content): %v", contentDir, err)
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
	manifest, err := loadRef254Manifest(t)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	// ── 2. Assertion A: every manifest file matches sha256 ──────────────────
	var mismatches []string
	for relPath, wantHex := range manifest {
		// Manifest paths are all "data/pack/..." → outDir/<rest>. (The 245
		// manifest also carried data/symbols/; the 254 baseline has none —
		// upstream deleted the .sym pipeline at 2e3bcf43.)
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
		limit := min(len(mismatches), 10)
		t.Errorf("%d manifest file(s) failed parity (showing first %d):\n%s",
			len(mismatches), limit, strings.Join(mismatches[:limit], "\n"))
	}

	// ── 3. Assertion B: no unexpected extra files ────────────────────────────
	// Walk outDir; every file must either be in the manifest or on the
	// explicit allowlist below. The data/symbols sibling PackAll writes is
	// NOT walked: it is the Go-only .sym export with no upstream baseline.
	//
	// Allowlisted extras (relative to outDir for data/pack paths):
	//   server/build              — wall-clock timestamp, size-checked separately
	//   ondemand.zip              — zip container bytes differ; content-checked separately
	//   server/maps/free2play.csv — goscape runtime copy (pkg/pack/maps/pack.go:72)
	//   server/maps/multiway.csv  — goscape runtime copy (pkg/pack/maps/pack.go:72)
	allowlisted := map[string]bool{
		"data/pack/server/build":              true,
		"data/pack/ondemand.zip":              true,
		"data/pack/server/maps/free2play.csv": true,
		"data/pack/server/maps/multiway.csv":  true,
	}

	var unexpected []string
	walkExtra := func(root, prefix string) {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return walkErr
			}
			rel, _ := filepath.Rel(root, path)
			manifestKey := prefix + "/" + filepath.ToSlash(rel)
			_, inManifest := manifest[manifestKey]
			if !inManifest && !allowlisted[manifestKey] {
				unexpected = append(unexpected, manifestKey)
			}
			return nil
		})
	}
	walkExtra(outDir, "data/pack")
	if len(unexpected) > 0 {
		t.Errorf("%d unexpected extra file(s) in output tree:\n  %s",
			len(unexpected), strings.Join(unexpected, "\n  "))
	}

	// ── 4. Assertion C: server/build is exactly 4 bytes ─────────────────────
	buildPath := filepath.Join(outDir, "server", "build")
	if fi, statErr := os.Stat(buildPath); statErr != nil {
		t.Errorf("server/build missing: %v", statErr)
	} else if fi.Size() != 4 {
		t.Errorf("server/build size = %d, want 4", fi.Size())
	}

	// ── 5. Assertion D: ondemand.zip entry-level content parity ─────────────
	checkOndemandZip(t, filepath.Join(outDir, "ondemand.zip"))
}

// loadRef254Manifest reads testdata/ref254_manifest.txt and returns a map from
// manifest path (e.g. "data/pack/client/config") to its expected sha256 hex.
// The map doubles as a membership set for "in manifest" queries in the
// extra-files walk (presence check via _, ok := manifest[key]).
func loadRef254Manifest(t *testing.T) (map[string]string, error) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "ref254_manifest.txt"))
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

// checkOndemandZip opens outDir/ondemand.zip and verifies that its entry set
// and per-entry sha256 of uncompressed bytes match testdata/ref254_ondemand_entries.txt.
func checkOndemandZip(t *testing.T, zipPath string) {
	t.Helper()

	// Load expected entries: "name size sha256" per line.
	type wantEntry struct {
		size int64
		sha  string
	}
	want := make(map[string]wantEntry)
	ef, err := os.Open(filepath.Join("testdata", "ref254_ondemand_entries.txt"))
	if err != nil {
		t.Fatalf("open ref254_ondemand_entries.txt: %v", err)
	}
	defer ef.Close()

	scanner := bufio.NewScanner(ef)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, shaHex string
		var size int64
		if _, scanErr := fmt.Sscanf(line, "%s %d %s", &name, &size, &shaHex); scanErr != nil {
			t.Fatalf("ref254_ondemand_entries.txt: malformed line %q: %v", line, scanErr)
		}
		want[name] = wantEntry{size: size, sha: shaHex}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		t.Fatalf("scan ref254_ondemand_entries.txt: %v", scanErr)
	}

	// Open the actual zip.
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("ondemand.zip missing: %v", err)
	}
	zr, err := zip.NewReader(readerAtBytes(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("ondemand.zip: zip.NewReader: %v", err)
	}

	got := make(map[string]struct{})
	var errs []string
	for _, f := range zr.File {
		got[f.Name] = struct{}{}
		w, ok := want[f.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("ondemand.zip: unexpected entry %q", f.Name))
			continue
		}
		rc, openErr := f.Open()
		if openErr != nil {
			errs = append(errs, fmt.Sprintf("ondemand.zip: open entry %q: %v", f.Name, openErr))
			continue
		}
		h := sha256.New()
		n, copyErr := io.Copy(h, rc)
		rc.Close()
		if copyErr != nil {
			errs = append(errs, fmt.Sprintf("ondemand.zip: read entry %q: %v", f.Name, copyErr))
			continue
		}
		if n != w.size {
			errs = append(errs, fmt.Sprintf("ondemand.zip: entry %q size %d want %d", f.Name, n, w.size))
		}
		gotSha := hex.EncodeToString(h.Sum(nil))
		if gotSha != w.sha {
			errs = append(errs, fmt.Sprintf("ondemand.zip: entry %q sha mismatch\n  want %s\n  got  %s", f.Name, w.sha, gotSha))
		}
	}

	// Check for missing entries.
	for name := range want {
		if _, ok := got[name]; !ok {
			errs = append(errs, fmt.Sprintf("ondemand.zip: missing entry %q", name))
		}
	}

	if len(errs) > 0 {
		limit := min(len(errs), 10)
		t.Errorf("ondemand.zip parity failures (showing first %d of %d):\n%s",
			limit, len(errs), strings.Join(errs[:limit], "\n"))
	}
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

// readerAtBytes wraps a byte slice as an io.ReaderAt.
type readerAtBytes []byte

func (b readerAtBytes) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
