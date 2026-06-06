package maps

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
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
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	var out bytes.Buffer
	// Map unpack emits no stdout (printInfo is commented out in TS source).
	// STDOUT-NORM in the manifest is sha256 of empty string.
	err := Unpack(Options{
		CacheDir: cacheDir,
		SrcDir:   scratch,
		Out:      &out,
	})
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// Content changed-set: pristine=contentDir (original), post=scratch (after run).
	content := unpacktest.ChangedSet(t, contentDir, scratch)

	// Cache changed-set: map family reads but does not modify the cache.
	// Pristine is the refRoot unpack-ref/cache snapshot that CacheDir was copied from.
	cachePristine := refRoot + "/unpack-ref/cache"
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)

	// WROTE: content writes only (map family does not write to cache).
	wrote := unpacktest.WroteSince(t, scratch, marker)

	r := unpacktest.Result{
		Content:      content,
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       out.Bytes(),
		PostDir:      scratch,
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "map", r)
}
