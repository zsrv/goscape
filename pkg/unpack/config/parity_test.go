package config

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
)

// TestConfigParity is the env-gated full config-family parity test.
// It requires GOSCAPE_REF254_DIR to point at the engine directory of a
// Server254-ref checkout. Run with:
//
//	GOSCAPE_REF254_DIR=/path/to/Server254-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/config/ -run TestConfigParity -v -count=1 -timeout 600s
//
// The test runs Unpack against a temp scratch copy of the reference content tree
// and asserts the result against testdata/ref254/config.manifest.txt.
//
// DisplaySrcDir is set to "../unpack-ref/scratch" so that the "Unpacking rev …
// into …/scripts" stdout line matches the TS reference run that was captured with
// the engine running from that relative path. See Options.DisplaySrcDir doc.
func TestConfigParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	var out bytes.Buffer
	// PackDir is intentionally omitted: the reference manifest was captured from
	// the plain (non-merge) unpack path. The merge path (compareIdx != nil) is
	// exercised by TestUnpackConfig_MergeEmission in driver_test.go.
	err := Unpack(Options{
		CacheDir:      cacheDir,
		SrcDir:        scratch,
		Revision:      "254",
		Out:           &out,
		DisplaySrcDir: "../unpack-ref/scratch",
	})
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// Build the result for manifest comparison.
	// Content changed-set: pristine=contentDir (original), post=scratch (after run).
	content := unpacktest.ChangedSet(t, contentDir, scratch)

	// Cache changed-set: config family does not modify the cache, so expect empty.
	// Pristine is the refRoot unpack-ref/cache snapshot that CacheDir was copied from.
	cachePristine := refRoot + "/unpack-ref/cache"
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)

	// WROTE: content writes only (no CACHE: prefix needed — config doesn't touch cache).
	wrote := unpacktest.WroteSince(t, scratch, marker)

	r := unpacktest.Result{
		Content:      content,
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       out.Bytes(),
		PostDir:      scratch,
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "config", r)
}
