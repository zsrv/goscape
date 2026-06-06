// Package unpacktest provides test-support helpers for unpack family parity tests.
// Each family parity test copies the reference content tree and cache dir to a scratch
// location, runs the Go unpack implementation, and asserts the result against the
// committed changed-set manifest produced from the upstream TypeScript reference run.
package unpacktest

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// RefDir returns the Server244-ref ROOT (parent of the directory pointed to by
// GOSCAPE_REF244_DIR), skipping the test when the variable is unset.
func RefDir(t *testing.T) string {
	t.Helper()
	env := os.Getenv("GOSCAPE_REF244_DIR")
	if env == "" {
		t.Skip("GOSCAPE_REF244_DIR not set; set it to the engine directory of a Server244-ref checkout")
	}
	return filepath.Clean(filepath.Join(env, ".."))
}

// ContentDir returns RefDir(t)+"/content".
func ContentDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(RefDir(t), "content")
}

// Scratch copies the full content tree (excluding the top-level ".git" file or
// directory) into a new directory under t.TempDir() and returns its path.
// File modes are preserved; mtimes are NOT preserved so that WroteSince works.
func Scratch(t *testing.T, contentDir string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(contentDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(contentDir, path)
		if relErr != nil {
			return relErr
		}
		// Skip the top-level .git entry (file or dir).
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
		return copyFile(path, dstPath, d)
	})
	if err != nil {
		t.Fatalf("Scratch: walk %q: %v", contentDir, err)
	}
	return dst
}

// CacheDir copies RefDir(t)+"/unpack-ref/cache" into t.TempDir() and returns it.
func CacheDir(t *testing.T) string {
	t.Helper()
	src := filepath.Join(RefDir(t), "unpack-ref", "cache")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			return os.MkdirAll(dstPath, info.Mode())
		}
		return copyFile(path, dstPath, d)
	})
	if err != nil {
		t.Fatalf("CacheDir: walk %q: %v", src, err)
	}
	return dst
}

// Marker returns a time strictly after every file written before the call.
// It sleeps 10ms then writes a probe file into a temporary directory, reads
// back its mtime, and returns (mtime - 1ns). Any file written at or after
// the probe moment will have a mtime strictly after the returned value; any
// file that existed before the 10ms sleep will have a mtime strictly before it.
// (Filesystem mtime resolution on Linux tmpfs is ~1ms; 10ms gives comfortable
// headroom.)
func Marker(t *testing.T) time.Time {
	t.Helper()
	time.Sleep(10 * time.Millisecond)
	probeDir := t.TempDir()
	probe := filepath.Join(probeDir, "marker.probe")
	if err := os.WriteFile(probe, []byte("probe"), 0o644); err != nil {
		t.Fatalf("Marker: write probe: %v", err)
	}
	info, err := os.Stat(probe)
	if err != nil {
		t.Fatalf("Marker: stat probe: %v", err)
	}
	// Return probeTime - 1ns so that WroteSince's strict-after check includes
	// any file written at or after the probe moment, while excluding all files
	// written before the 10ms sleep.
	return info.ModTime().Add(-1)
}

// WroteSince walks dir and returns sorted relative paths of regular files with
// mtime strictly after marker.
func WroteSince(t *testing.T, dir string, marker time.Time) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.ModTime().After(marker) {
			rel, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				return relErr
			}
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WroteSince: walk %q: %v", dir, err)
	}
	slices.Sort(paths)
	return paths
}

// Entry is one changed-set line from a manifest.
type Entry struct {
	Kind string // "ADDED", "MODIFIED", "DELETED"
	Path string
	Sum  string // lowercase hex sha256; empty for DELETED
}

// ChangedSet diffs post against pristine (both directory trees, ".git" excluded)
// and returns entries sorted by Path within Kind order ADDED, MODIFIED, DELETED.
func ChangedSet(t *testing.T, pristine, post string) []Entry {
	t.Helper()

	pristineFiles, err := walkTree(pristine)
	if err != nil {
		t.Fatalf("ChangedSet: walk pristine %q: %v", pristine, err)
	}
	postFiles, err := walkTree(post)
	if err != nil {
		t.Fatalf("ChangedSet: walk post %q: %v", post, err)
	}

	var added, modified, deleted []Entry

	// Check for ADDED and MODIFIED.
	for rel, postSum := range postFiles {
		if pristineSum, ok := pristineFiles[rel]; ok {
			if pristineSum != postSum {
				modified = append(modified, Entry{Kind: "MODIFIED", Path: rel, Sum: postSum})
			}
		} else {
			added = append(added, Entry{Kind: "ADDED", Path: rel, Sum: postSum})
		}
	}

	// Check for DELETED.
	for rel := range pristineFiles {
		if _, ok := postFiles[rel]; !ok {
			deleted = append(deleted, Entry{Kind: "DELETED", Path: rel})
		}
	}

	slices.SortFunc(added, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(modified, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(deleted, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })

	return slices.Concat(added, modified, deleted)
}

// Result bundles everything a family parity test produced.
type Result struct {
	Content      []Entry  // ChangedSet of the content scratch
	Cache        []Entry  // ChangedSet of the cache dir (vs its pristine copy)
	Wrote        []string // WroteSince(content scratch), plus "CACHE:"+p for cache-dir writes
	Stdout       []byte   // raw captured stdout of the Go tool
	PostDir      string   // the content scratch dir (for PNG pixel lookups)
	CachePostDir string   // the cache dir
}

// AssertManifest compares r against testdata/ref244/<family>.manifest.txt.
// It resolves the manifest path relative to this package's source location.
// All mismatches are reported with t.Errorf (not just the first).
func AssertManifest(t *testing.T, refRoot, family string, r Result) {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	manifestPath := filepath.Join(filepath.Dir(thisFile), "..", "testdata", "ref244", family+".manifest.txt")
	mismatches := assertManifestFile(t, manifestPath, refRoot, family, r)
	for _, m := range mismatches {
		t.Errorf("manifest mismatch: %s", m)
	}
}

// assertManifestFile is the core implementation. It returns all mismatch
// descriptions so tests can inspect them directly without depending on t.Errorf.
func assertManifestFile(t *testing.T, manifestPath, refRoot, family string, r Result) []string {
	t.Helper()
	manifest, err := parseManifest(manifestPath)
	if err != nil {
		t.Fatalf("assertManifestFile: parse %q: %v", manifestPath, err)
	}

	var mismatches []string

	// --- Changed-set comparison (ADDED/MODIFIED/DELETED for content) ---
	contentByPath := make(map[string]Entry, len(r.Content))
	for _, e := range r.Content {
		contentByPath[e.Kind+"|"+e.Path] = e
	}

	manifestContentByPath := make(map[string]manifestEntry, len(manifest.content))
	for _, me := range manifest.content {
		manifestContentByPath[me.kind+"|"+me.path] = me
	}

	// Every manifest content entry must be matched.
	for key, me := range manifestContentByPath {
		got, ok := contentByPath[key]
		if !ok {
			mismatches = append(mismatches, fmt.Sprintf("content %s %s: in manifest but missing from result", me.kind, me.path))
			continue
		}
		if me.kind != "DELETED" {
			compareSha(me, got.Sum, r.PostDir, refRoot, family+".post", "content", &mismatches)
		}
	}

	// Every result content entry must appear in the manifest.
	// Exception: result-side ADDED or MODIFIED entries whose path ends in ".png"
	// are exempt from the "missing from manifest" failure when the decoded pixels
	// of the Go-produced file match the reference post-tree snapshot
	// (<refRoot>/unpack-ref/<family>.post/<path>).  This covers the case where TS
	// emitted PNG bytes byte-identically with the committed content tree (so no
	// changed-set entry was generated), but Go's image/png encoder produces
	// different bytes for the same pixel data.
	for key, got := range contentByPath {
		if _, ok := manifestContentByPath[key]; !ok {
			if (got.Kind == "ADDED" || got.Kind == "MODIFIED") && strings.HasSuffix(got.Path, ".png") {
				// Apply pixel-equality exemption: compare against the reference post snapshot.
				goFile := filepath.Join(r.PostDir, got.Path)
				refFile := filepath.Join(refRoot, "unpack-ref", family+".post", got.Path)
				var pixMismatch []string
				compareSha(manifestEntry{kind: got.Kind, path: got.Path, sum: got.Sum}, got.Sum, r.PostDir, refRoot, family+".post", "content", &pixMismatch)
				// compareSha with matching sums will not add a mismatch for non-PNG.
				// For PNG we need to do the pixel check directly.
				goImg, goErr := decodeImageFile(goFile)
				refImg, refErr := decodeImageFile(refFile)
				if goErr != nil || refErr != nil {
					// Cannot decode → treat as real mismatch.
					mismatches = append(mismatches, fmt.Sprintf("content %s %s: in result but missing from manifest (PNG decode error: go=%v ref=%v)", got.Kind, got.Path, goErr, refErr))
					continue
				}
				goBounds := goImg.Bounds()
				refBounds := refImg.Bounds()
				if goBounds != refBounds {
					mismatches = append(mismatches, fmt.Sprintf("content %s %s: in result but missing from manifest (PNG bounds differ: go=%v ref=%v)", got.Kind, got.Path, goBounds, refBounds))
					continue
				}
				pixelMatch := true
				for y := goBounds.Min.Y; y < goBounds.Max.Y; y++ {
					for x := goBounds.Min.X; x < goBounds.Max.X; x++ {
						gr, gg, gb, ga := goImg.At(x, y).RGBA()
						rr, rg, rb, ra := refImg.At(x, y).RGBA()
						if gr != rr || gg != rg || gb != rb || ga != ra {
							mismatches = append(mismatches, fmt.Sprintf("content %s %s: in result but missing from manifest (pixel (%d,%d) differs: go=(%d,%d,%d,%d) ref=(%d,%d,%d,%d))",
								got.Kind, got.Path, x, y, gr, gg, gb, ga, rr, rg, rb, ra))
							pixelMatch = false
							break
						}
					}
					if !pixelMatch {
						break
					}
				}
				// If pixels match, the PNG byte-divergence is expected — no mismatch.
				continue
			}
			mismatches = append(mismatches, fmt.Sprintf("content %s %s: in result but missing from manifest", got.Kind, got.Path))
		}
	}

	// --- Changed-set comparison for cache ---
	cacheByPath := make(map[string]Entry, len(r.Cache))
	for _, e := range r.Cache {
		cacheByPath[e.Kind+"|"+e.Path] = e
	}

	manifestCacheByPath := make(map[string]manifestEntry, len(manifest.cache))
	for _, me := range manifest.cache {
		manifestCacheByPath[me.kind+"|"+me.path] = me
	}

	for key, me := range manifestCacheByPath {
		got, ok := cacheByPath[key]
		if !ok {
			mismatches = append(mismatches, fmt.Sprintf("cache %s %s: in manifest but missing from result", me.kind, me.path))
			continue
		}
		if me.kind != "DELETED" {
			compareSha(me, got.Sum, r.CachePostDir, refRoot, family+".cachepost", "cache", &mismatches)
		}
	}

	for key, got := range cacheByPath {
		if _, ok := manifestCacheByPath[key]; !ok {
			mismatches = append(mismatches, fmt.Sprintf("cache %s %s: in result but missing from manifest", got.Kind, got.Path))
		}
	}

	// --- WROTE set comparison ---
	// Manifest WROTE entries were captured from the TS reference runs with the
	// PackFile module-load registry rewrites subtracted (importing
	// tools/pack/PackFile.js rewrites 20 pack/*.pack files byte-identically in
	// EVERY family, even print-only ones — see the manifest header). That
	// subtraction makes genuine registry saves un-attributable when they land
	// on a noise-listed path (e.g. config's ModelPack.save touches
	// pack/model.pack, midi's MidiPack.save rewrites pack/midi.pack with
	// identical bytes). So the WROTE check is asymmetric: every manifest WROTE
	// must be present in the result, and result extras are failures UNLESS
	// they are registry .pack files under pack/ (content correctness for those
	// is still pinned by the ADDED/MODIFIED changed-set when bytes change).
	wroteMani := make(map[string]bool, len(manifest.wrote))
	for _, p := range manifest.wrote {
		wroteMani[p] = true
	}
	wroteResult := make(map[string]bool, len(r.Wrote))
	for _, p := range r.Wrote {
		wroteResult[p] = true
	}

	for p := range wroteMani {
		if !wroteResult[p] {
			mismatches = append(mismatches, fmt.Sprintf("WROTE %s: in manifest but missing from result", p))
		}
	}
	for p := range wroteResult {
		if !wroteMani[p] && !isRegistryPack(p) {
			mismatches = append(mismatches, fmt.Sprintf("WROTE %s: in result but missing from manifest", p))
		}
	}

	// --- STDOUT-NORM comparison ---
	if manifest.stdoutNorm != "" {
		gotSum := sha256Hex(r.Stdout)
		if gotSum != manifest.stdoutNorm {
			mismatches = append(mismatches, fmt.Sprintf("STDOUT-NORM: want %s got %s", manifest.stdoutNorm, gotSum))
		}
	}

	slices.Sort(mismatches)
	return mismatches
}

// compareSha checks sha equality for a single manifest entry, applying the PNG
// pixel-equality exception for paths ending in ".png".
// section is the human-readable section label ("content" or "cache") included
// in every mismatch string.
// postDir is the directory containing the Go-produced file for this entry.
// refSubdir is the subdirectory name under "<refRoot>/unpack-ref/" that holds the
// reference snapshot (e.g. "test.post" for content, "test.cachepost" for cache).
func compareSha(me manifestEntry, gotSum, postDir, refRoot, refSubdir, section string, mismatches *[]string) {
	if !strings.HasSuffix(me.path, ".png") {
		// Exact hex sha match.
		if me.sum != gotSum {
			*mismatches = append(*mismatches, fmt.Sprintf("%s %s %s: sha want %s got %s", section, me.kind, me.path, me.sum, gotSum))
		}
		return
	}

	// PNG: pixel equality.
	goFile := filepath.Join(postDir, me.path)
	refFile := filepath.Join(refRoot, "unpack-ref", refSubdir, me.path)

	goImg, err := decodeImageFile(goFile)
	if err != nil {
		*mismatches = append(*mismatches, fmt.Sprintf("%s %s %s: decode go PNG %q: %v", section, me.kind, me.path, goFile, err))
		return
	}
	refImg, err := decodeImageFile(refFile)
	if err != nil {
		*mismatches = append(*mismatches, fmt.Sprintf("%s %s %s: decode ref PNG %q: %v", section, me.kind, me.path, refFile, err))
		return
	}

	goBounds := goImg.Bounds()
	refBounds := refImg.Bounds()
	if goBounds != refBounds {
		*mismatches = append(*mismatches, fmt.Sprintf("%s %s %s: PNG bounds differ: go=%v ref=%v", section, me.kind, me.path, goBounds, refBounds))
		return
	}

	for y := goBounds.Min.Y; y < goBounds.Max.Y; y++ {
		for x := goBounds.Min.X; x < goBounds.Max.X; x++ {
			gr, gg, gb, ga := goImg.At(x, y).RGBA()
			rr, rg, rb, ra := refImg.At(x, y).RGBA()
			if gr != rr || gg != rg || gb != rb || ga != ra {
				*mismatches = append(*mismatches, fmt.Sprintf("%s %s %s: pixel (%d,%d) differs: go=(%d,%d,%d,%d) ref=(%d,%d,%d,%d)",
					section, me.kind, me.path, x, y, gr, gg, gb, ga, rr, rg, rb, ra))
				return // report first differing pixel only
			}
		}
	}
}

// --- Manifest parsing ---

type manifestEntry struct {
	kind string // "ADDED", "MODIFIED", "DELETED"
	path string
	sum  string // empty for DELETED
}

type parsedManifest struct {
	content    []manifestEntry
	cache      []manifestEntry
	wrote      []string
	stdoutNorm string
}

func parseManifest(path string) (*parsedManifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := &parsedManifest{}
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
		case "ADDED", "MODIFIED":
			if len(parts) < 3 {
				return nil, fmt.Errorf("bad line: %q", line)
			}
			// Format: KIND <path-may-have-spaces> <sha256>
			// sha256 is always the last field; path is parts[1:len-1] joined.
			sum := parts[len(parts)-1]
			p := strings.Join(parts[1:len(parts)-1], " ")
			m.content = append(m.content, manifestEntry{kind: parts[0], path: p, sum: sum})
		case "DELETED":
			if len(parts) < 2 {
				return nil, fmt.Errorf("bad line: %q", line)
			}
			// Format: DELETED <path-may-have-spaces>
			m.content = append(m.content, manifestEntry{kind: "DELETED", path: strings.Join(parts[1:], " ")})
		case "CACHE-ADDED", "CACHE-MODIFIED":
			if len(parts) < 3 {
				return nil, fmt.Errorf("bad line: %q", line)
			}
			kind := strings.TrimPrefix(parts[0], "CACHE-")
			sum := parts[len(parts)-1]
			p := strings.Join(parts[1:len(parts)-1], " ")
			m.cache = append(m.cache, manifestEntry{kind: kind, path: p, sum: sum})
		case "CACHE-DELETED":
			if len(parts) < 2 {
				return nil, fmt.Errorf("bad line: %q", line)
			}
			m.cache = append(m.cache, manifestEntry{kind: "DELETED", path: strings.Join(parts[1:], " ")})
		case "WROTE":
			if len(parts) < 2 {
				return nil, fmt.Errorf("bad line: %q", line)
			}
			// WROTE <relpath> or WROTE CACHE:<relpath>
			// Path may contain spaces; join all parts after the keyword.
			m.wrote = append(m.wrote, strings.Join(parts[1:], " "))
		case "STDOUT-NORM":
			if len(parts) < 2 {
				return nil, fmt.Errorf("bad line: %q", line)
			}
			m.stdoutNorm = parts[1]
		}
	}
	return m, scanner.Err()
}

// --- Internal utilities ---

// walkTree returns a map of relative path → sha256 hex for all regular files
// in root, skipping the top-level ".git" entry.
func walkTree(root string) (map[string]string, error) {
	result := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		// Skip top-level .git (file or dir).
		if rel == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		sum, sumErr := sha256File(path)
		if sumErr != nil {
			return sumErr
		}
		result[rel] = sum
		return nil
	})
	return result, err
}

// copyFile copies src to dst, preserving the file mode.
func copyFile(src, dst string, d fs.DirEntry) error {
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
	_, copyErr := io.Copy(w, r)
	closeErr := w.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// sha256File returns the lowercase hex sha256 of the file at path.
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

// isRegistryPack reports whether p is a name-registry file (pack/<type>.pack,
// optionally CACHE:-prefixed) — the only paths exempt from the
// extra-WROTE-entry check (see the WROTE comparison comment).
func isRegistryPack(p string) bool {
	p = strings.TrimPrefix(p, "CACHE:")
	dir, file := path.Split(p)
	return dir == "pack/" && strings.HasSuffix(file, ".pack")
}

// sha256Hex returns the lowercase hex sha256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// decodeImageFile opens path and decodes it as an image.
func decodeImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
