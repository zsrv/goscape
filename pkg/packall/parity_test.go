package packall

import (
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
//   - server/maps/free2play.csv  — goscape-extra runtime copy; not in TS pack output.
//   - server/maps/multiway.csv   — goscape-extra runtime copy; not in TS pack output.
//     See pkg/pack/maps/pack.go for the deviation rationale.
//
// rev-274: the server/build and ondemand.zip exemptions were removed — TS
// PackAll.ts (dee467c8) no longer emits either artifact, so there is nothing
// to exempt. The ref manifest is regenerated against the 274 baseline in T20.
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
		limit := 10
		if len(mismatches) < limit {
			limit = len(mismatches)
		}
		t.Errorf("%d manifest file(s) failed parity (showing first %d):\n%s",
			len(mismatches), limit, strings.Join(mismatches[:limit], "\n"))
	}

	// ── 3. Assertion B: no unexpected extra files ────────────────────────────
	// Walk outDir; every file must either be in the manifest or on the
	// explicit allowlist below. The data/symbols sibling PackAll writes is
	// NOT walked: it is the Go-only .sym export with no upstream baseline.
	//
	// Allowlisted extras (relative to outDir for data/pack paths):
	//   server/maps/free2play.csv — goscape runtime copy (pkg/pack/maps/pack.go:72)
	//   server/maps/multiway.csv  — goscape runtime copy (pkg/pack/maps/pack.go:72)
	//
	// rev-274: server/build + ondemand.zip removed from the allowlist — they
	// are no longer emitted (TS PackAll.ts dee467c8).
	allowlisted := map[string]bool{
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

	// rev-274: assertions C (server/build size) and D (ondemand.zip entry
	// parity) were removed — neither artifact is emitted at the 274 pin.
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
