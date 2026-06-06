package unpacktest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// hashBytes returns the lowercase hex sha256 of b.
func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TestChangedSet verifies that ChangedSet correctly reports ADDED, MODIFIED,
// and DELETED files, and excludes ".git" entries.
func TestChangedSet(t *testing.T) {
	pristine := t.TempDir()
	post := t.TempDir()

	// pristine: a.txt, b.txt, sub/c.txt, .git (file — must be excluded)
	writeFile(t, filepath.Join(pristine, "a.txt"), []byte("hello"))
	writeFile(t, filepath.Join(pristine, "b.txt"), []byte("original"))
	if err := os.MkdirAll(filepath.Join(pristine, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(pristine, "sub", "c.txt"), []byte("keep"))
	// .git as a plain file — must not appear as DELETED
	writeFile(t, filepath.Join(pristine, ".git"), []byte("gitfile"))

	// post: a.txt same, b.txt changed, sub/c.txt deleted, d.txt added
	writeFile(t, filepath.Join(post, "a.txt"), []byte("hello"))
	writeFile(t, filepath.Join(post, "b.txt"), []byte("changed"))
	writeFile(t, filepath.Join(post, "d.txt"), []byte("new"))

	entries := ChangedSet(t, pristine, post)

	wantAdded := Entry{Kind: "ADDED", Path: "d.txt", Sum: hashBytes([]byte("new"))}
	wantModified := Entry{Kind: "MODIFIED", Path: "b.txt", Sum: hashBytes([]byte("changed"))}
	wantDeleted := Entry{Kind: "DELETED", Path: "sub/c.txt"}

	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d: %v", len(entries), entries)
	}
	if entries[0] != wantAdded {
		t.Errorf("entries[0] ADDED: got %+v, want %+v", entries[0], wantAdded)
	}
	if entries[1] != wantModified {
		t.Errorf("entries[1] MODIFIED: got %+v, want %+v", entries[1], wantModified)
	}
	if entries[2] != wantDeleted {
		t.Errorf("entries[2] DELETED: got %+v, want %+v", entries[2], wantDeleted)
	}

	// Confirm .git was excluded
	for _, e := range entries {
		if strings.HasPrefix(e.Path, ".git") {
			t.Errorf("ChangedSet included .git path: %v", e)
		}
	}
}

// TestChangedSet_GitDirExcluded verifies .git directories are also excluded.
func TestChangedSet_GitDirExcluded(t *testing.T) {
	pristine := t.TempDir()
	post := t.TempDir()

	writeFile(t, filepath.Join(pristine, "a.txt"), []byte("x"))
	writeFile(t, filepath.Join(post, "a.txt"), []byte("x"))

	// .git as a directory in pristine only
	if err := os.MkdirAll(filepath.Join(pristine, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(pristine, ".git", "HEAD"), []byte("ref: refs/heads/main\n"))

	entries := ChangedSet(t, pristine, post)
	if len(entries) != 0 {
		t.Errorf("want 0 entries (no real changes), got %d: %v", len(entries), entries)
	}
}

// TestWroteSince verifies that WroteSince returns exactly the files modified
// after the marker, in sorted order.
func TestWroteSince(t *testing.T) {
	dir := t.TempDir()

	// Create files before taking the marker.
	writeFile(t, filepath.Join(dir, "old.txt"), []byte("old"))
	writeFile(t, filepath.Join(dir, "also_old.txt"), []byte("also old"))

	marker := Marker(t)

	// Rewrite one, create one after the marker.
	writeFile(t, filepath.Join(dir, "old.txt"), []byte("rewritten"))
	writeFile(t, filepath.Join(dir, "new.txt"), []byte("brand new"))

	got := WroteSince(t, dir, marker)
	want := []string{"new.txt", "old.txt"}
	if !slices.Equal(got, want) {
		t.Errorf("WroteSince: got %v, want %v", got, want)
	}
}

// TestWroteSince_Subdirs verifies that WroteSince descends into subdirectories.
func TestWroteSince_Subdirs(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "sub", "old.txt"), []byte("old"))

	marker := Marker(t)

	writeFile(t, filepath.Join(dir, "sub", "old.txt"), []byte("new content"))

	got := WroteSince(t, dir, marker)
	if len(got) != 1 || got[0] != "sub/old.txt" {
		t.Errorf("WroteSince subdirs: got %v, want [sub/old.txt]", got)
	}
}

// TestAssertManifest_HappyPath verifies that a matching Result produces no mismatches.
func TestAssertManifest_HappyPath(t *testing.T) {
	dir := t.TempDir()
	refRoot := t.TempDir()
	postDir := t.TempDir()
	cachePostDir := t.TempDir()

	addedContent := []byte("added file")
	modifiedContent := []byte("modified content")
	stdoutBytes := []byte("some stdout output\n")

	addedPath := filepath.Join(postDir, "data", "added.txt")
	if err := os.MkdirAll(filepath.Dir(addedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, addedPath, addedContent)

	manifestContent := fmt.Sprintf(
		"# comment\nADDED data/added.txt %s\nMODIFIED data/modified.txt %s\nDELETED data/gone.txt\nWROTE data/added.txt\nSTDOUT-NORM %s\n",
		hashBytes(addedContent),
		hashBytes(modifiedContent),
		hashBytes(stdoutBytes),
	)
	manifestPath := filepath.Join(dir, "test.manifest.txt")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o644); err != nil {
		t.Fatal(err)
	}

	r := Result{
		Content: []Entry{
			{Kind: "ADDED", Path: "data/added.txt", Sum: hashBytes(addedContent)},
			{Kind: "MODIFIED", Path: "data/modified.txt", Sum: hashBytes(modifiedContent)},
			{Kind: "DELETED", Path: "data/gone.txt"},
		},
		Cache:        nil,
		Wrote:        []string{"data/added.txt"},
		Stdout:       stdoutBytes,
		PostDir:      postDir,
		CachePostDir: cachePostDir,
	}

	mismatches := assertManifestFile(t, manifestPath, refRoot, "test", r)
	if len(mismatches) != 0 {
		t.Errorf("happy path: want 0 mismatches, got %d: %v", len(mismatches), mismatches)
	}
}

// TestAssertManifest_Mismatches verifies individual mismatch cases.
func TestAssertManifest_Mismatches(t *testing.T) {
	dir := t.TempDir()
	refRoot := t.TempDir()
	postDir := t.TempDir()
	cachePostDir := t.TempDir()

	addedContent := []byte("content")
	stdoutBytes := []byte("stdout\n")

	baseManifest := fmt.Sprintf(
		"ADDED data/a.txt %s\nWROTE data/a.txt\nSTDOUT-NORM %s\n",
		hashBytes(addedContent),
		hashBytes(stdoutBytes),
	)

	writeManifest := func(content string) string {
		p := filepath.Join(dir, fmt.Sprintf("manifest_%d.txt", time.Now().UnixNano()))
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// Ensure postDir has the added file for lookups.
	if err := os.MkdirAll(filepath.Join(postDir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(postDir, "data", "a.txt"), addedContent)

	t.Run("missing ADDED in result", func(t *testing.T) {
		p := writeManifest(baseManifest)
		r := Result{
			Content:      nil, // missing the ADDED
			Wrote:        []string{"data/a.txt"},
			Stdout:       stdoutBytes,
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}
		mismatches := assertManifestFile(t, p, refRoot, "test", r)
		if !anyContains(mismatches, "data/a.txt") {
			t.Errorf("expected mismatch mentioning data/a.txt, got: %v", mismatches)
		}
	})

	t.Run("extra changed-set entry in result", func(t *testing.T) {
		p := writeManifest(baseManifest)
		r := Result{
			Content: []Entry{
				{Kind: "ADDED", Path: "data/a.txt", Sum: hashBytes(addedContent)},
				{Kind: "ADDED", Path: "data/extra.txt", Sum: hashBytes([]byte("x"))}, // not in manifest
			},
			Wrote:        []string{"data/a.txt"},
			Stdout:       stdoutBytes,
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}
		mismatches := assertManifestFile(t, p, refRoot, "test", r)
		if !anyContains(mismatches, "data/extra.txt") {
			t.Errorf("expected mismatch mentioning data/extra.txt, got: %v", mismatches)
		}
	})

	t.Run("wrong sha", func(t *testing.T) {
		p := writeManifest(baseManifest)
		r := Result{
			Content: []Entry{
				{Kind: "ADDED", Path: "data/a.txt", Sum: hashBytes([]byte("wrong content"))},
			},
			Wrote:        []string{"data/a.txt"},
			Stdout:       stdoutBytes,
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}
		mismatches := assertManifestFile(t, p, refRoot, "test", r)
		if !anyContains(mismatches, "data/a.txt") {
			t.Errorf("expected mismatch mentioning data/a.txt sha, got: %v", mismatches)
		}
	})

	t.Run("missing WROTE in result", func(t *testing.T) {
		p := writeManifest(baseManifest)
		r := Result{
			Content: []Entry{
				{Kind: "ADDED", Path: "data/a.txt", Sum: hashBytes(addedContent)},
			},
			Wrote:        nil, // missing "data/a.txt"
			Stdout:       stdoutBytes,
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}
		mismatches := assertManifestFile(t, p, refRoot, "test", r)
		if !anyContains(mismatches, "data/a.txt") || !anyContains(mismatches, "WROTE") {
			t.Errorf("expected WROTE mismatch mentioning data/a.txt, got: %v", mismatches)
		}
	})

	t.Run("spaced paths happy path", func(t *testing.T) {
		// Pin that the manifest parser handles paths containing spaces.
		// Commit dd618b46 fixed parseManifest to use parts[1:len-1] join for
		// ADDED/MODIFIED and parts[1:] join for DELETED/WROTE, so a path like
		// "data/with space.txt" is reconstructed correctly instead of being split.
		spacedContent := []byte("spaced file content")
		spacedBinContent := []byte("binary content")

		spacedManifest := fmt.Sprintf(
			"ADDED data/with space.txt %s\nDELETED old file.txt\nWROTE data/with space.txt\nWROTE CACHE:pack dir/file two.bin\nSTDOUT-NORM %s\n",
			hashBytes(spacedContent),
			hashBytes(stdoutBytes),
		)
		p := writeManifest(spacedManifest)

		// postDir already has data/a.txt; create the spaced file.
		writeFile(t, filepath.Join(postDir, "data", "with space.txt"), spacedContent)
		_ = spacedBinContent // not checked by WROTE (registry-pack exempt path)

		r := Result{
			Content: []Entry{
				{Kind: "ADDED", Path: "data/with space.txt", Sum: hashBytes(spacedContent)},
				{Kind: "DELETED", Path: "old file.txt"},
			},
			Wrote:        []string{"data/with space.txt", "CACHE:pack dir/file two.bin"},
			Stdout:       stdoutBytes,
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}
		mismatches := assertManifestFile(t, p, refRoot, "test", r)
		if len(mismatches) != 0 {
			t.Errorf("spaced paths happy path: want 0 mismatches, got %d: %v", len(mismatches), mismatches)
		}
	})

	t.Run("spaced path wrong sha", func(t *testing.T) {
		// Ensure a wrong sha is still detected when the path contains spaces.
		spacedContent := []byte("spaced file content")
		spacedManifest := fmt.Sprintf(
			"ADDED data/with space.txt %s\nSTDOUT-NORM %s\n",
			hashBytes(spacedContent),
			hashBytes(stdoutBytes),
		)
		p := writeManifest(spacedManifest)

		r := Result{
			Content: []Entry{
				{Kind: "ADDED", Path: "data/with space.txt", Sum: hashBytes([]byte("wrong content"))},
			},
			Wrote:        nil,
			Stdout:       stdoutBytes,
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}
		mismatches := assertManifestFile(t, p, refRoot, "test", r)
		if !anyContains(mismatches, "data/with space.txt") {
			t.Errorf("spaced path wrong sha: expected mismatch mentioning \"data/with space.txt\", got: %v", mismatches)
		}
	})

	t.Run("extra WROTE in result", func(t *testing.T) {
		p := writeManifest(baseManifest)
		r := Result{
			Content: []Entry{
				{Kind: "ADDED", Path: "data/a.txt", Sum: hashBytes(addedContent)},
			},
			Wrote:        []string{"data/a.txt", "data/extra.txt"}, // extra
			Stdout:       stdoutBytes,
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}
		mismatches := assertManifestFile(t, p, refRoot, "test", r)
		if !anyContains(mismatches, "data/extra.txt") || !anyContains(mismatches, "WROTE") {
			t.Errorf("expected extra WROTE mismatch, got: %v", mismatches)
		}
	})

	t.Run("extra WROTE registry .pack is exempt", func(t *testing.T) {
		// TS PackFile module-load rewrites made registry saves un-attributable
		// in the manifests, so result-side extra pack/*.pack writes (plain or
		// CACHE:-prefixed) are allowed; anything else stays a mismatch.
		p := writeManifest(baseManifest)
		r := Result{
			Content: []Entry{
				{Kind: "ADDED", Path: "data/a.txt", Sum: hashBytes(addedContent)},
			},
			Wrote: []string{
				"data/a.txt",
				"pack/model.pack",      // exempt
				"CACHE:pack/midi.pack", // exempt
				"pack/sub/deep.pack",   // NOT exempt (not directly under pack/)
				"scripts/all.pack.bak", // NOT exempt (wrong extension shape)
			},
			Stdout:       stdoutBytes,
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}
		mismatches := assertManifestFile(t, p, refRoot, "test", r)
		if anyContains(mismatches, "pack/model.pack") || anyContains(mismatches, "pack/midi.pack") {
			t.Errorf("registry .pack writes must be exempt, got: %v", mismatches)
		}
		if !anyContains(mismatches, "pack/sub/deep.pack") || !anyContains(mismatches, "scripts/all.pack.bak") {
			t.Errorf("non-registry extras must still mismatch, got: %v", mismatches)
		}
	})

	t.Run("wrong stdout sha", func(t *testing.T) {
		p := writeManifest(baseManifest)
		r := Result{
			Content: []Entry{
				{Kind: "ADDED", Path: "data/a.txt", Sum: hashBytes(addedContent)},
			},
			Wrote:        []string{"data/a.txt"},
			Stdout:       []byte("wrong stdout\n"), // different
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}
		mismatches := assertManifestFile(t, p, refRoot, "test", r)
		if !anyContains(mismatches, "STDOUT") {
			t.Errorf("expected STDOUT mismatch, got: %v", mismatches)
		}
	})
}

// TestAssertManifest_PNGPixelEquality verifies the PNG pixel comparison path.
func TestAssertManifest_PNGPixelEquality(t *testing.T) {
	dir := t.TempDir()
	refRoot := t.TempDir()
	postDir := t.TempDir()
	cachePostDir := t.TempDir()

	const pngPath = "sprites/icon.png"
	if err := os.MkdirAll(filepath.Join(postDir, "sprites"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(refRoot, "unpack-ref", "test.post", "sprites"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Build a small 2x2 RGBA image.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	img.SetRGBA(1, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	img.SetRGBA(0, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	img.SetRGBA(1, 1, color.RGBA{R: 128, G: 128, B: 128, A: 255})

	// Encode the same image with default compression to postDir.
	postPNGPath := filepath.Join(postDir, "sprites", "icon.png")
	encodePNG(t, postPNGPath, img, png.DefaultCompression)

	// Encode the same pixels with BestCompression to refRoot (different bytes, same pixels).
	refPNGPath := filepath.Join(refRoot, "unpack-ref", "test.post", "sprites", "icon.png")
	encodePNG(t, refPNGPath, img, png.BestCompression)

	// The two files must have different bytes.
	postBytes, _ := os.ReadFile(postPNGPath)
	refBytes, _ := os.ReadFile(refPNGPath)
	if string(postBytes) == string(refBytes) {
		// If by chance they're equal, just skip the byte-difference assertion.
		t.Log("note: PNG bytes happened to be equal; pixel path still exercised")
	}

	// Build a manifest using postDir's sha (would fail sha match, but pixel path passes).
	manifestContent := fmt.Sprintf(
		"ADDED %s %s\nWROTE %s\nSTDOUT-NORM %s\n",
		pngPath, hashBytes(postBytes),
		pngPath,
		hashBytes([]byte{}),
	)
	manifestPath := filepath.Join(dir, "test.manifest.txt")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o644); err != nil {
		t.Fatal(err)
	}

	r := Result{
		Content: []Entry{
			{Kind: "ADDED", Path: pngPath, Sum: hashBytes(postBytes)},
		},
		Wrote:        []string{pngPath},
		Stdout:       []byte{},
		PostDir:      postDir,
		CachePostDir: cachePostDir,
	}

	mismatches := assertManifestFile(t, manifestPath, refRoot, "test", r)
	if len(mismatches) != 0 {
		t.Errorf("same-pixel PNGs: want 0 mismatches, got: %v", mismatches)
	}

	t.Run("differing pixel", func(t *testing.T) {
		// Change one pixel.
		imgB := image.NewRGBA(image.Rect(0, 0, 2, 2))
		imgB.SetRGBA(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		imgB.SetRGBA(1, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
		imgB.SetRGBA(0, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		imgB.SetRGBA(1, 1, color.RGBA{R: 99, G: 128, B: 128, A: 255}) // different from img

		postPNGPathB := filepath.Join(postDir, "sprites", "icon_b.png")
		encodePNG(t, postPNGPathB, imgB, png.DefaultCompression)
		postBytesB, _ := os.ReadFile(postPNGPathB)

		// ref still has original img pixels.
		refPNGPathB := filepath.Join(refRoot, "unpack-ref", "test.post", "sprites", "icon_b.png")
		encodePNG(t, refPNGPathB, img, png.BestCompression)

		manifestB := fmt.Sprintf(
			"ADDED sprites/icon_b.png %s\nWROTE sprites/icon_b.png\nSTDOUT-NORM %s\n",
			hashBytes(postBytesB),
			hashBytes([]byte{}),
		)
		manifestPathB := filepath.Join(dir, "test_b.manifest.txt")
		if err := os.WriteFile(manifestPathB, []byte(manifestB), 0o644); err != nil {
			t.Fatal(err)
		}

		rB := Result{
			Content: []Entry{
				{Kind: "ADDED", Path: "sprites/icon_b.png", Sum: hashBytes(postBytesB)},
			},
			Wrote:        []string{"sprites/icon_b.png"},
			Stdout:       []byte{},
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}

		mismatchesB := assertManifestFile(t, manifestPathB, refRoot, "test", rB)
		if !anyContains(mismatchesB, "sprites/icon_b.png") {
			t.Errorf("differing pixel: expected pixel mismatch for sprites/icon_b.png, got: %v", mismatchesB)
		}
	})
}

// TestAssertManifest_CachePNGPixelEquality verifies that the PNG pixel-equality
// exception is applied in the cache changed-set section (CACHE-ADDED / CACHE-MODIFIED)
// and that CachePostDir is used to resolve the Go-produced file while
// "<refRoot>/unpack-ref/<family>.cachepost/<path>" is used for the reference file.
func TestAssertManifest_CachePNGPixelEquality(t *testing.T) {
	dir := t.TempDir()
	refRoot := t.TempDir()
	postDir := t.TempDir()      // content post dir — not touched by this test
	cachePostDir := t.TempDir() // cache post dir — holds Go-produced cache PNGs

	const pngPath = "textures/tile.png"

	// Create directory trees in cachePostDir and the cachepost ref snapshot.
	if err := os.MkdirAll(filepath.Join(cachePostDir, "textures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(refRoot, "unpack-ref", "test.cachepost", "textures"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Build a small 2x2 RGBA image.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	img.SetRGBA(1, 0, color.RGBA{R: 40, G: 50, B: 60, A: 255})
	img.SetRGBA(0, 1, color.RGBA{R: 70, G: 80, B: 90, A: 255})
	img.SetRGBA(1, 1, color.RGBA{R: 100, G: 110, B: 120, A: 255})

	// Encode Go cache file with DefaultCompression, reference with BestCompression
	// so the raw bytes differ while pixels are identical.
	cachePNGPath := filepath.Join(cachePostDir, "textures", "tile.png")
	encodePNG(t, cachePNGPath, img, png.DefaultCompression)

	refCachePNGPath := filepath.Join(refRoot, "unpack-ref", "test.cachepost", "textures", "tile.png")
	encodePNG(t, refCachePNGPath, img, png.BestCompression)

	cacheBytes, _ := os.ReadFile(cachePNGPath)
	refBytes, _ := os.ReadFile(refCachePNGPath)
	if string(cacheBytes) == string(refBytes) {
		t.Log("note: cache PNG bytes happened to be equal; pixel path still exercised")
	}

	// Manifest uses the sha of the Go file (which differs from the ref sha).
	manifestContent := fmt.Sprintf(
		"CACHE-ADDED %s %s\nSTDOUT-NORM %s\n",
		pngPath, hashBytes(cacheBytes),
		hashBytes([]byte{}),
	)
	manifestPath := filepath.Join(dir, "test.manifest.txt")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o644); err != nil {
		t.Fatal(err)
	}

	r := Result{
		Content:      nil,
		Cache:        []Entry{{Kind: "ADDED", Path: pngPath, Sum: hashBytes(cacheBytes)}},
		Wrote:        nil,
		Stdout:       []byte{},
		PostDir:      postDir,
		CachePostDir: cachePostDir,
	}

	// Same pixels — must produce no mismatches.
	mismatches := assertManifestFile(t, manifestPath, refRoot, "test", r)
	if len(mismatches) != 0 {
		t.Errorf("same-pixel cache PNGs: want 0 mismatches, got: %v", mismatches)
	}

	t.Run("differing pixel", func(t *testing.T) {
		// Build an image with one pixel changed.
		imgB := image.NewRGBA(image.Rect(0, 0, 2, 2))
		imgB.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		imgB.SetRGBA(1, 0, color.RGBA{R: 40, G: 50, B: 60, A: 255})
		imgB.SetRGBA(0, 1, color.RGBA{R: 70, G: 80, B: 90, A: 255})
		imgB.SetRGBA(1, 1, color.RGBA{R: 1, G: 2, B: 3, A: 255}) // differs from img

		cachePNGPathB := filepath.Join(cachePostDir, "textures", "tile_b.png")
		encodePNG(t, cachePNGPathB, imgB, png.DefaultCompression)
		cacheBytesB, _ := os.ReadFile(cachePNGPathB)

		// Reference still has original img pixels.
		refCachePNGPathB := filepath.Join(refRoot, "unpack-ref", "test.cachepost", "textures", "tile_b.png")
		encodePNG(t, refCachePNGPathB, img, png.BestCompression)

		const pngPathB = "textures/tile_b.png"
		manifestB := fmt.Sprintf(
			"CACHE-ADDED %s %s\nSTDOUT-NORM %s\n",
			pngPathB, hashBytes(cacheBytesB),
			hashBytes([]byte{}),
		)
		manifestPathB := filepath.Join(dir, "test_cache_b.manifest.txt")
		if err := os.WriteFile(manifestPathB, []byte(manifestB), 0o644); err != nil {
			t.Fatal(err)
		}

		rB := Result{
			Content:      nil,
			Cache:        []Entry{{Kind: "ADDED", Path: pngPathB, Sum: hashBytes(cacheBytesB)}},
			Wrote:        nil,
			Stdout:       []byte{},
			PostDir:      postDir,
			CachePostDir: cachePostDir,
		}

		mismatchesB := assertManifestFile(t, manifestPathB, refRoot, "test", rB)
		if !anyContains(mismatchesB, pngPathB) {
			t.Errorf("differing pixel in cache PNG: expected mismatch mentioning %s, got: %v", pngPathB, mismatchesB)
		}
		if !anyContains(mismatchesB, "cache ") {
			t.Errorf("differing pixel in cache PNG: expected mismatch to contain \"cache \", got: %v", mismatchesB)
		}
	})
}

// writeFile is a test helper to write content to path.
func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// encodePNG writes img to path using the given compression level.
func encodePNG(t *testing.T, path string, img image.Image, level png.CompressionLevel) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := png.Encoder{CompressionLevel: level}
	if err := enc.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// anyContains returns true if any mismatch string contains sub.
func anyContains(mismatches []string, sub string) bool {
	for _, m := range mismatches {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}
