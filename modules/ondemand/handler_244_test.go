package ondemand

// rev-244 B3 — failing pins for the FileStream-backed HTTP asset routes.
// Tests are written against the NEW API (cache field + CachePath config) that
// does not yet exist. They MUST fail before the implementation is added.
//
// Route table verified against Engine-TS 9aadcec4 src/web.ts:63-84:
//   /crc       → CrcBuffer (unchanged)
//   /title     → cache.read(0, 1)
//   /config    → cache.read(0, 2)
//   /interface → cache.read(0, 3)
//   /media     → cache.read(0, 4)
//   /versionlist → cache.read(0, 5)   NEW (replaces /models)
//   /textures  → cache.read(0, 6)
//   /wordenc   → cache.read(0, 7)
//   /sounds    → cache.read(0, 8)
//   /ondemand.zip → REMOVED at 274 (web.ts dee467c8 dropped the route)
//   /build        → REMOVED at 274 (web.ts dee467c8 dropped the route)
//   .mid route → REMOVED (225 web.ts:63-69; midi now TCP OnDemand archive 3)
//   /models    → REMOVED (replaced by /versionlist)
//   /maps/     → KEPT (goscape-specific, no 244 TS analog)
//
// Missing-file posture: 404 (established goscape-ondemand posture; TS would
// panic with a Bun 500 on the non-null assertion — decision row).

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// newTestOnDemandWithCache creates an OnDemand wired with a FileStream whose
// archive 0 has fixtures written at files 1–8. The caller receives the
// per-file fixture data indexed by file number.
func newTestOnDemandWithCache(t *testing.T) (*OnDemand, map[int][]byte) {
	t.Helper()
	dir := t.TempDir()
	fs, err := filestream.New(dir, true, false)
	if err != nil {
		t.Fatalf("filestream.New: %v", err)
	}
	t.Cleanup(func() { fs.Close() })

	fixtures := make(map[int][]byte)
	for file := 1; file <= 8; file++ {
		data := make([]byte, 16)
		for i := range data {
			data[i] = byte(file*16 + i)
		}
		if !fs.Write(0, file, data, 0) {
			t.Fatalf("FileStream.Write(0, %d) failed", file)
		}
		fixtures[file] = data
	}

	a := &OnDemand{
		log:     discardLogger(),
		cache:   fs,
		cacheMu: sync.Mutex{},
	}
	return a, fixtures
}

// --- Archive route pins ---

func TestRootHandler244_Title(t *testing.T) {
	a, fix := newTestOnDemandWithCache(t)
	req := httptest.NewRequest(http.MethodGet, "/title", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, fix[1]) {
		t.Fatalf("body = %v, want %v", got, fix[1])
	}
}

func TestRootHandler244_Config(t *testing.T) {
	a, fix := newTestOnDemandWithCache(t)
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, fix[2]) {
		t.Fatalf("body = %v, want %v", got, fix[2])
	}
}

func TestRootHandler244_Interface(t *testing.T) {
	a, fix := newTestOnDemandWithCache(t)
	req := httptest.NewRequest(http.MethodGet, "/interface", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, fix[3]) {
		t.Fatalf("body = %v, want %v", got, fix[3])
	}
}

func TestRootHandler244_Media(t *testing.T) {
	a, fix := newTestOnDemandWithCache(t)
	req := httptest.NewRequest(http.MethodGet, "/media", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, fix[4]) {
		t.Fatalf("body = %v, want %v", got, fix[4])
	}
}

func TestRootHandler244_VersionlistNew(t *testing.T) {
	a, fix := newTestOnDemandWithCache(t)
	req := httptest.NewRequest(http.MethodGet, "/versionlist", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, fix[5]) {
		t.Fatalf("body = %v, want %v (versionlist must read archive 0, file 5)", got, fix[5])
	}
}

func TestRootHandler244_Textures(t *testing.T) {
	a, fix := newTestOnDemandWithCache(t)
	req := httptest.NewRequest(http.MethodGet, "/textures", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, fix[6]) {
		t.Fatalf("body = %v, want %v", got, fix[6])
	}
}

func TestRootHandler244_Wordenc(t *testing.T) {
	a, fix := newTestOnDemandWithCache(t)
	req := httptest.NewRequest(http.MethodGet, "/wordenc", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, fix[7]) {
		t.Fatalf("body = %v, want %v", got, fix[7])
	}
}

func TestRootHandler244_Sounds(t *testing.T) {
	a, fix := newTestOnDemandWithCache(t)
	req := httptest.NewRequest(http.MethodGet, "/sounds", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, fix[8]) {
		t.Fatalf("body = %v, want %v", got, fix[8])
	}
}

// --- /versionlist replaces /models ---

// TestRootHandler244_ModelsGone pins that /models is no longer routed — the
// 244 web.ts removes the /models branch entirely (it is replaced by
// /versionlist). Without a cache or loose-file match, the handler should
// fall through to 404.
func TestRootHandler244_ModelsGone(t *testing.T) {
	// No cache wired, no public dir — only named routes could match.
	a := &OnDemand{log: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (/models must be gone at rev-244)", rr.Code)
	}
}

// --- .mid route removed ---

// TestRootHandler244_MidRouteGone pins that the .mid route is removed at 244.
// TS web.ts removes the .mid branch entirely (midi now flows over TCP OnDemand
// archive 3). A .mid request must fall through to 404, not be served.
// Arc-31 M28 404-handling rides along: the no-underscore panic guard is also
// gone since the whole branch is removed.
func TestRootHandler244_MidRouteGone(t *testing.T) {
	// Create the songs dir with a file so the old handler would have found it.
	t.Chdir(t.TempDir())
	songDir := filepath.Join("data", "pack", "client", "songs")
	if err := os.MkdirAll(songDir, 0o755); err != nil {
		t.Fatalf("mkdir songs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(songDir, "gielinor.mid"), []byte("SONG"), 0o644); err != nil {
		t.Fatalf("write song: %v", err)
	}

	a := &OnDemand{log: discardLogger()}
	for _, p := range []string{"/gielinor_12345.mid", "/x.mid", "/.mid"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		a.RootHandler(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404 (.mid must be gone at rev-244)", p, rr.Code)
		}
	}
}

// rev-274: the /ondemand.zip and /build static-route tests were retired —
// TS web.ts (dee467c8) dropped both routes and PackAll.ts no longer emits the
// backing artifacts. The routes were removed from handler.go; a request now
// falls through to the public/404 path (covered by the unmatched-path test).

// --- Missing archive file → 404 ---

// TestRootHandler244_ArchiveMissing404 pins the missing-file posture for
// archive routes: when cache.Read returns nil (archive file not written),
// the handler must return 404, not 200/empty or panic.
// Decision row: TS would 500 (Bun non-null assertion); goscape uses 404.
func TestRootHandler244_ArchiveMissing404(t *testing.T) {
	dir := t.TempDir()
	// Create an empty FileStream — file 1 has never been written.
	fs, err := filestream.New(dir, true, false)
	if err != nil {
		t.Fatalf("filestream.New: %v", err)
	}
	defer fs.Close()

	a := &OnDemand{
		log:     discardLogger(),
		cache:   fs,
		cacheMu: sync.Mutex{},
	}

	for _, path := range []string{"/title", "/config", "/interface", "/media", "/versionlist", "/textures", "/wordenc", "/sounds"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		a.RootHandler(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s with no cache data: status = %d, want 404", path, rr.Code)
		}
	}
}

// TestRootHandler244_NilCacheFallsThrough404 pins that when the OnDemand
// struct has no FileStream (nil cache, e.g. cache-path not configured) archive
// routes return 404, not panic.
func TestRootHandler244_NilCacheFallsThrough404(t *testing.T) {
	a := &OnDemand{log: discardLogger()} // cache == nil
	for _, path := range []string{"/title", "/config", "/versionlist"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		a.RootHandler(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s with nil cache: status = %d, want 404", path, rr.Code)
		}
	}
}

// --- Content-Type pin ---

// TestRootHandler244_ArchiveContentType pins that archive routes use
// application/octet-stream, matching the existing posture in handler.go.
func TestRootHandler244_ArchiveContentType(t *testing.T) {
	a, _ := newTestOnDemandWithCache(t)
	req := httptest.NewRequest(http.MethodGet, "/title", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if got := rr.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
}

// --- /maps/ route unchanged ---

// TestRootHandler244_MapsRouteUnchanged pins that the goscape-specific /maps/
// route is preserved at rev-244 (no TS analog; revisit at B6).
func TestRootHandler244_MapsRouteUnchanged(t *testing.T) {
	t.Chdir(t.TempDir())
	mapsDir := filepath.Join("data", "pack", "client", "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := []byte("MAPDATA")
	if err := os.WriteFile(filepath.Join(mapsDir, "m48_50"), want, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := &OnDemand{log: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/maps/m48_50", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
}
