package versionlist

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/unpack/unpacktest"
)

// TestVersionlistAnimParity is the env-gated parity test for anim_index.
// It requires GOSCAPE_REF254_DIR. Run with:
//
//	GOSCAPE_REF254_DIR=/path/to/Server254-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/versionlist/ -run TestVersionlistAnimParity -v -count=1 -timeout 900s
func TestVersionlistAnimParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	var out bytes.Buffer
	if err := AnimIndex(cacheDir, &out); err != nil {
		t.Fatalf("AnimIndex: %v", err)
	}

	// anim_index emits no content or cache writes — only stdout.
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

	unpacktest.AssertManifest(t, refRoot, "versionlist-anim", r)
}

// TestVersionlistMidiParity is the env-gated parity test for midi_index.
// It requires GOSCAPE_REF254_DIR. Run with:
//
//	GOSCAPE_REF254_DIR=/path/to/Server254-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/versionlist/ -run TestVersionlistMidiParity -v -count=1 -timeout 900s
//
// srcDir uses the scratch content tree (contains midi.pack after midi unpack).
// The MidiPack is loaded read-only; no save is called.
func TestVersionlistMidiParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	var out bytes.Buffer
	if err := MidiIndex(cacheDir, scratch, &out); err != nil {
		t.Fatalf("MidiIndex: %v", err)
	}

	// midi_index emits no content or cache writes — only stdout.
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

	unpacktest.AssertManifest(t, refRoot, "versionlist-midi", r)
}

// TestVersionlistModelParity is the env-gated parity test for model_index.
// It requires GOSCAPE_REF254_DIR. Run with:
//
//	GOSCAPE_REF254_DIR=/path/to/Server254-ref/engine \
//	  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
//	  go test ./pkg/unpack/versionlist/ -run TestVersionlistModelParity -v -count=1 -timeout 900s
//
// ModelIndex writes two files into cacheDir (model_index, model_index.txt).
// The Wrote slice includes "CACHE:"+p for each cache write.
func TestVersionlistModelParity(t *testing.T) {
	refRoot := unpacktest.RefDir(t)
	contentDir := unpacktest.ContentDir(t)
	scratch := unpacktest.Scratch(t, contentDir)
	cacheDir := unpacktest.CacheDir(t)
	marker := unpacktest.Marker(t)

	if err := ModelIndex(cacheDir, scratch, nil); err != nil {
		t.Fatalf("ModelIndex: %v", err)
	}

	// model_index writes to cacheDir only; no content changes.
	content := unpacktest.ChangedSet(t, contentDir, scratch)
	cachePristine := refRoot + "/unpack-ref/cache"
	cache := unpacktest.ChangedSet(t, cachePristine, cacheDir)

	// Content wrote: none.
	contentWrote := unpacktest.WroteSince(t, scratch, marker)
	// Cache wrote: files written into cacheDir after the marker.
	cacheWrote := unpacktest.WroteSince(t, cacheDir, marker)
	wrote := make([]string, 0, len(contentWrote)+len(cacheWrote))
	wrote = append(wrote, contentWrote...)
	for _, p := range cacheWrote {
		wrote = append(wrote, "CACHE:"+p)
	}

	r := unpacktest.Result{
		Content:      content,
		Cache:        cache,
		Wrote:        wrote,
		Stdout:       nil,
		PostDir:      scratch,
		CachePostDir: cacheDir,
	}

	unpacktest.AssertManifest(t, refRoot, "versionlist-model", r)
}
