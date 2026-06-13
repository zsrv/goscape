package midi

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
)

// TestMidiParity is the env-gated full midi-family parity test.
// It requires GOSCAPE_REF274_DIR to point at the engine directory of a
// Server274-ref checkout. Run with:
//
//	GOSCAPE_REF274_DIR=/path/to/Server274-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/midi/ -run TestMidiParity -v -count=1 -timeout 900s
//
// The test runs Unpack against a temp scratch copy of the reference content tree
// and asserts the result against testdata/ref274/midi.manifest.txt.
func TestMidiParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	var out bytes.Buffer
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

	// Cache changed-set: midi family reads but does not modify the cache.
	cachePristine := refRoot + "/unpack-ref/cache"
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)

	// WROTE: content writes only (midi family does not write to cache).
	wrote := unpacktest.WroteSince(t, scratch, marker)

	r := unpacktest.Result{
		Content:      content,
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       out.Bytes(),
		PostDir:      scratch,
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "midi", r)
}
