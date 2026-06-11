package clientinterface

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
)

// TestInterfaceParity is the env-gated full interface-family parity test.
// It requires GOSCAPE_REF254_DIR to point at the engine directory of a
// Server254-ref checkout. Run with:
//
//	GOSCAPE_REF254_DIR=/path/to/Server254-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/clientinterface/ -run TestInterfaceParity -v -count=1 -timeout 900s
//
// The test runs Unpack against a temp scratch copy of the reference content
// tree and asserts the result against testdata/ref254/interface.manifest.txt.
//
// STDOUT-NORM is sha256 of the empty string (interface family emits nothing
// to Out; console.error goes to Errorf, not Out).
func TestInterfaceParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	var out bytes.Buffer
	var errLines []string
	errorf := func(format string, args ...any) {
		// Capture errorf output but do not write to out (matches TS console.error).
		_ = format
		_ = args
		errLines = append(errLines, format)
	}

	err := Unpack(Options{
		CacheDir: cacheDir,
		SrcDir:   scratch,
		Out:      &out,
		Errorf:   errorf,
	})
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// Build the result for manifest comparison.
	content := unpacktest.ChangedSet(t, contentDir, scratch)

	cachePristine := refRoot + "/unpack-ref/cache"
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)

	wrote := unpacktest.WroteSince(t, scratch, marker)

	r := unpacktest.Result{
		Content:      content,
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       out.Bytes(),
		PostDir:      scratch,
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "interface", r)
}
