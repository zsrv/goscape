package maps

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMapParity is the env-gated full map-family parity test.
// It requires GOSCAPE_REF244_DIR to point at the engine directory of a
// Server244-ref checkout. Run with:
//
//	GOSCAPE_REF244_DIR=/path/to/Server244-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/maps/ -run TestMapParity -v -count=1 -timeout 900s
//
// The test runs Unpack against a temp scratch copy of the reference content tree
// and asserts the result against testdata/ref244/map.manifest.txt.
func TestMapParity(t *testing.T) {
	refRoot := parityRefDir(t)
	contentDir := filepath.Join(refRoot, "content")
	scratch := parityScratch(t, contentDir)
	cacheDir := parityCacheDir(t, refRoot)
	marker := parityMarker(t)

	// Map unpack emits no stdout (printInfo is commented out in TS source).
	err := Unpack(Options{
		CacheDir: cacheDir,
		SrcDir:   scratch,
		Out:      nil, // no output expected; manifest STDOUT-NORM = sha256("")
	})
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	wrote := parityWroteSince(t, scratch, marker)

	parityAssertManifest(t, refRoot, wrote, nil)
}

// --- parity helpers ---

func parityRefDir(t *testing.T) string {
	t.Helper()
	env := os.Getenv("GOSCAPE_REF244_DIR")
	if env == "" {
		t.Skip("GOSCAPE_REF244_DIR not set; set it to the engine directory of a Server244-ref checkout")
	}
	return filepath.Clean(filepath.Join(env, ".."))
}

func parityScratch(t *testing.T, contentDir string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(contentDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(contentDir, path)
		if rel == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			return os.MkdirAll(dstPath, info.Mode())
		}
		return parityFileCopy(path, dstPath, d)
	})
	if err != nil {
		t.Fatalf("parityScratch: %v", err)
	}
	return dst
}

func parityCacheDir(t *testing.T, refRoot string) string {
	t.Helper()
	src := filepath.Join(refRoot, "unpack-ref", "cache")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			return os.MkdirAll(dstPath, info.Mode())
		}
		return parityFileCopy(path, dstPath, d)
	})
	if err != nil {
		t.Fatalf("parityCacheDir: %v", err)
	}
	return dst
}

func parityFileCopy(src, dst string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	var buf [32 * 1024]byte
	_, copyErr := copyWithBuf(r, w, buf[:])
	closeErr := w.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func copyWithBuf(r, w *os.File, buf []byte) (int64, error) {
	var total int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			wn, werr := w.Write(buf[:n])
			total += int64(wn)
			if werr != nil {
				return total, werr
			}
		}
		if err != nil {
			if err.Error() == "EOF" || strings.Contains(err.Error(), "EOF") {
				return total, nil
			}
			return total, err
		}
	}
}

func parityMarker(t *testing.T) time.Time {
	t.Helper()
	time.Sleep(10 * time.Millisecond)
	probe := filepath.Join(t.TempDir(), "marker.probe")
	if err := os.WriteFile(probe, []byte("probe"), 0o644); err != nil {
		t.Fatalf("parityMarker write: %v", err)
	}
	info, err := os.Stat(probe)
	if err != nil {
		t.Fatalf("parityMarker stat: %v", err)
	}
	return info.ModTime().Add(-1)
}

func parityWroteSince(t *testing.T, dir string, marker time.Time) []string {
	t.Helper()
	var paths []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if info.ModTime().After(marker) {
			rel, _ := filepath.Rel(dir, path)
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(paths)
	return paths
}

// parityAssertManifest compares wrote and stdout against testdata/ref244/map.manifest.txt.
func parityAssertManifest(t *testing.T, refRoot string, wrote []string, stdout []byte) {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	manifestPath := filepath.Join(
		filepath.Dir(thisFile), "..", "testdata", "ref244", "map.manifest.txt",
	)

	f, err := os.Open(manifestPath)
	if err != nil {
		t.Fatalf("open manifest %q: %v", manifestPath, err)
	}
	defer f.Close()

	var manifestWrote []string
	var stdoutNorm string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		switch parts[0] {
		case "WROTE":
			if len(parts) >= 2 {
				manifestWrote = append(manifestWrote, parts[1])
			}
		case "STDOUT-NORM":
			if len(parts) >= 2 {
				stdoutNorm = parts[1]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan manifest: %v", err)
	}

	wroteSet := make(map[string]bool, len(wrote))
	for _, p := range wrote {
		wroteSet[p] = true
	}
	manifestSet := make(map[string]bool, len(manifestWrote))
	for _, p := range manifestWrote {
		manifestSet[p] = true
	}

	var mismatches []string
	for p := range manifestSet {
		if !wroteSet[p] {
			mismatches = append(mismatches, "WROTE "+p+": in manifest but missing from result")
		}
	}
	for p := range wroteSet {
		if !manifestSet[p] && !parityIsPackFile(p) {
			mismatches = append(mismatches, "WROTE "+p+": in result but missing from manifest")
		}
	}

	if stdoutNorm != "" {
		h := sha256.Sum256(stdout)
		gotSum := hex.EncodeToString(h[:])
		if gotSum != stdoutNorm {
			mismatches = append(mismatches, "STDOUT-NORM: want "+stdoutNorm+" got "+gotSum)
		}
	}

	slices.Sort(mismatches)
	for _, m := range mismatches {
		t.Errorf("manifest mismatch: %s", m)
	}
}

func parityIsPackFile(p string) bool {
	p = strings.TrimPrefix(p, "CACHE:")
	dir, file := filepath.Split(p)
	dir = filepath.ToSlash(dir)
	return dir == "pack/" && strings.HasSuffix(file, ".pack")
}
