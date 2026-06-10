package graphics

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
)

// TestModelsParity is the env-gated full models-family parity test.
// It requires GOSCAPE_REF245_DIR to point at the engine directory of a
// Server245.2-ref checkout.  Run with:
//
//	GOSCAPE_REF245_DIR=/path/to/Server245.2-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/graphics/ -run TestModelsParity -v -count=1 -timeout 900s
func TestModelsParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	var out bytes.Buffer
	err := Models(Options{
		CacheDir: cacheDir,
		SrcDir:   scratch,
		Out:      &out,
	})
	if err != nil {
		t.Fatalf("Models: %v", err)
	}

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

	unpacktest.AssertManifest(t, refRoot, "models", r)
}

// TestAnimsParity is the env-gated full anims-family parity test.
// It requires GOSCAPE_REF245_DIR to point at the engine directory of a
// Server245.2-ref checkout.  Run with:
//
//	GOSCAPE_REF245_DIR=/path/to/Server245.2-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/graphics/ -run TestAnimsParity -v -count=1 -timeout 900s
func TestAnimsParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	var out bytes.Buffer
	err := Anims(Options{
		CacheDir: cacheDir,
		SrcDir:   scratch,
		Out:      &out,
	})
	if err != nil {
		t.Fatalf("Anims: %v", err)
	}

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

	unpacktest.AssertManifest(t, refRoot, "anims", r)
}
