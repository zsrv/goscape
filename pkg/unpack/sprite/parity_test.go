package sprite

import (
	"testing"

	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
)

// TestSpriteMediaParity is the env-gated full sprite-media parity test.
// It requires GOSCAPE_REF274_DIR to point at the engine directory of a
// Server274-ref checkout. Run with:
//
//	GOSCAPE_REF274_DIR=/path/to/Server274-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/sprite/ -run TestSpriteMediaParity -v -count=1 -timeout 900s
func TestSpriteMediaParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	err := Media(Options{
		CacheDir: cacheDir,
		SrcDir:   scratch,
	})
	if err != nil {
		t.Fatalf("Media: %v", err)
	}

	content := unpacktest.ChangedSet(t, contentDir, scratch)
	cachePristine := refRoot + "/unpack-ref/cache"
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)
	wrote := unpacktest.WroteSince(t, scratch, marker)

	r := unpacktest.Result{
		Content:      content,
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       nil,
		PostDir:      scratch,
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "sprite-media", r)
}

// TestSpriteTexturesParity is the env-gated full sprite-textures parity test.
// It requires GOSCAPE_REF274_DIR to point at the engine directory of a
// Server274-ref checkout. Run with:
//
//	GOSCAPE_REF274_DIR=/path/to/Server274-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/sprite/ -run TestSpriteTexturesParity -v -count=1 -timeout 900s
func TestSpriteTexturesParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	err := Textures(Options{
		CacheDir: cacheDir,
		SrcDir:   scratch,
	})
	if err != nil {
		t.Fatalf("Textures: %v", err)
	}

	content := unpacktest.ChangedSet(t, contentDir, scratch)
	cachePristine := refRoot + "/unpack-ref/cache"
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)
	wrote := unpacktest.WroteSince(t, scratch, marker)

	r := unpacktest.Result{
		Content:      content,
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       nil,
		PostDir:      scratch,
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "sprite-textures", r)
}

// TestSpriteTitleParity is the env-gated full sprite-title parity test.
// It requires GOSCAPE_REF274_DIR to point at the engine directory of a
// Server274-ref checkout. Run with:
//
//	GOSCAPE_REF274_DIR=/path/to/Server274-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/sprite/ -run TestSpriteTitleParity -v -count=1 -timeout 900s
func TestSpriteTitleParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	err := Title(Options{
		CacheDir: cacheDir,
		SrcDir:   scratch,
	})
	if err != nil {
		t.Fatalf("Title: %v", err)
	}

	content := unpacktest.ChangedSet(t, contentDir, scratch)
	cachePristine := refRoot + "/unpack-ref/cache"
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)
	wrote := unpacktest.WroteSince(t, scratch, marker)

	r := unpacktest.Result{
		Content:      content,
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       nil,
		PostDir:      scratch,
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "sprite-title", r)
}
