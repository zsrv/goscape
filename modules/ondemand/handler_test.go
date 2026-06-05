package ondemand

import (
	"bytes"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/dskit/middleware"
)

// TestRootHandlerCrcEndpointServesOnEveryRequest pins the fix for the
// CrcBuffer-as-stateful-reader bug: both the first and second /crc request
// must return the full CRC bytes payload, not an empty body.
func TestRootHandlerCrcEndpointServesOnEveryRequest(t *testing.T) {
	prev := cache.CRC()
	t.Cleanup(func() { cache.SetCRCForTest(prev) })
	want := []byte{0x00, 0x00, 0x00, 0x00, 0xDE, 0xAD, 0xBE, 0xEF}
	cache.SetCRCForTest(&cache.CRCSnapshot{Bytes: want})

	a := &OnDemand{log: discardLogger()}

	for i := range 2 {
		req := httptest.NewRequest(http.MethodGet, "/crc", nil)
		rr := httptest.NewRecorder()
		a.RootHandler(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, rr.Code)
		}
		got, _ := io.ReadAll(rr.Body)
		if !bytes.Equal(got, want) {
			t.Fatalf("request %d: body = %v, want %v", i+1, got, want)
		}
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestOnDemandClientIPExtractsTSParityHeaders pins source-IP wiring: the
// OnDemand server's SourceIPExtractor must consult cf-connecting-ip (the TS upstream's
// getIp() primary header in Engine-TS/src/web.ts) and return only the first
// comma-separated address to match TS' .split(',')[0].trim() behaviour.
//
// Note: dskit's SourceIPExtractor is single-source — when LogSourceIPsHeader
// is non-empty, only that header is consulted; the built-in Forwarded /
// X-Real-IP / X-Forwarded-For chain is bypassed. Operators wanting that
// chain (covering x-forwarded-for) must blank the header/regex flags.
func TestOnDemandClientIPExtractsTSParityHeaders(t *testing.T) {
	// Build the OnDemand server with the default flag values so the test covers the
	// production header/regex defaults wired in config.go.
	cfg := Config{Enable: true}
	cfg.RegisterFlagsAndApplyDefaults(flag.NewFlagSet("ondemand-test", flag.ContinueOnError))

	sourceIPs, err := middleware.NewSourceIPs(cfg.Server.LogSourceIPsHeader, cfg.Server.LogSourceIPsRegex, cfg.Server.LogSourceIPsFull)
	if err != nil {
		t.Fatalf("NewSourceIPs: %v", err)
	}
	a := &OnDemand{log: discardLogger(), sourceIPs: sourceIPs}

	tests := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{
			name:    "cf-connecting-ip extracted with remote suffix",
			headers: map[string]string{"CF-Connecting-IP": "203.0.113.7"},
			remote:  "10.0.0.1:5555",
			want:    "203.0.113.7, 10.0.0.1",
		},
		{
			name:    "cf-connecting-ip comma list returns only first",
			headers: map[string]string{"CF-Connecting-IP": "203.0.113.7, 192.0.2.1"},
			remote:  "10.0.0.1:5555",
			want:    "203.0.113.7, 10.0.0.1",
		},
		{
			name:    "cf-connecting-ip strips surrounding whitespace",
			headers: map[string]string{"CF-Connecting-IP": "  203.0.113.7  ,  192.0.2.1  "},
			remote:  "10.0.0.1:5555",
			want:    "203.0.113.7, 10.0.0.1",
		},
		{
			name:    "falls back to remote addr when header absent",
			headers: nil,
			remote:  "10.0.0.1:5555",
			want:    "10.0.0.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/title", nil)
			req.RemoteAddr = tc.remote
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			got := a.clientIP(req)
			if got != tc.want {
				t.Fatalf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestOnDemandClientIPDefaultChainFallback pins the alternative configuration:
// when LogSourceIPsHeader/Regex are blank, the dskit SourceIPExtractor falls
// back to its built-in Forwarded / X-Real-IP / X-Forwarded-For chain.
func TestOnDemandClientIPDefaultChainFallback(t *testing.T) {
	sourceIPs, err := middleware.NewSourceIPs("", "", false)
	if err != nil {
		t.Fatalf("NewSourceIPs: %v", err)
	}
	a := &OnDemand{log: discardLogger(), sourceIPs: sourceIPs}

	req := httptest.NewRequest(http.MethodGet, "/title", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.1.1.1")

	if got, want := a.clientIP(req), "198.51.100.9, 10.0.0.1"; got != want {
		t.Fatalf("clientIP() = %q, want %q", got, want)
	}
}

// TestOnDemandClientIPNilExtractorReturnsEmpty pins the nil-extractor branch:
// if the OnDemand server was constructed without a SourceIPExtractor (e.g. tests that
// bypass New), clientIP must not panic.
func TestOnDemandClientIPNilExtractorReturnsEmpty(t *testing.T) {
	a := &OnDemand{log: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/title", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	req.Header.Set("CF-Connecting-IP", "203.0.113.7")

	if got := a.clientIP(req); got != "" {
		t.Fatalf("clientIP() with nil extractor = %q, want empty", got)
	}
}

// TestRootHandlerPublicFallbackServesKnownMime mirrors web.ts:114-119: after
// every named route misses, a file inside public/ is served with the
// project-specific Content-Type (here: application/javascript for .js).
func TestRootHandlerPublicFallbackServesKnownMime(t *testing.T) {
	dir := t.TempDir()
	body := []byte("console.log('hi');\n")
	if err := os.WriteFile(filepath.Join(dir, "foo.js"), body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	a := &OnDemand{log: discardLogger(), cfg: Config{PublicDir: dir}}

	req := httptest.NewRequest(http.MethodGet, "/foo.js", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/javascript" {
		t.Fatalf("Content-Type = %q, want application/javascript", got)
	}
	if got, _ := io.ReadAll(rr.Body); !bytes.Equal(got, body) {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

// TestRootHandlerPublicFallbackUnknownExtUsesTextPlain pins the TS fallback
// Content-Type "text/plain" for extensions not in MIME_TYPES (web.ts:117).
func TestRootHandlerPublicFallbackUnknownExtUsesTextPlain(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.bin"), []byte("anything"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	a := &OnDemand{log: discardLogger(), cfg: Config{PublicDir: dir}}

	req := httptest.NewRequest(http.MethodGet, "/data.bin", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
}

// TestRootHandlerPublicFallbackMissingFile404s asserts the TS else-branch
// behavior (web.ts:121): missing files surface as 404, not a stale named-route
// response or an empty 200.
func TestRootHandlerPublicFallbackMissingFile404s(t *testing.T) {
	dir := t.TempDir()
	a := &OnDemand{log: discardLogger(), cfg: Config{PublicDir: dir}}

	req := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// TestRootHandlerPublicFallbackBlocksPathTraversal verifies that http.Dir
// rejects ".." escape attempts so a sibling file outside PublicDir cannot
// be served via the public fallback.
func TestRootHandlerPublicFallbackBlocksPathTraversal(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	publicDir := filepath.Join(parent, "public")
	if err := os.Mkdir(publicDir, 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}

	a := &OnDemand{log: discardLogger(), cfg: Config{PublicDir: publicDir}}

	// http.ServeFile / http.Dir reject ".." path elements. We also test that
	// even if the URL parser collapses the path, the file outside the root is
	// not reachable.
	for _, p := range []string{"/../secret.txt", "/..%2Fsecret.txt"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		a.RootHandler(rr, req)

		if rr.Code == http.StatusOK {
			body, _ := io.ReadAll(rr.Body)
			if bytes.Contains(body, []byte("nope")) {
				t.Fatalf("path %q: served secret outside PublicDir; body=%q", p, body)
			}
		}
	}
}

// TestRootHandlerPublicFallbackDisabledWhenUnset asserts that an empty
// PublicDir disables the fallback entirely — requests fall through to the
// stdlib 404 instead of probing the working directory.
func TestRootHandlerPublicFallbackDisabledWhenUnset(t *testing.T) {
	a := &OnDemand{log: discardLogger(), cfg: Config{PublicDir: ""}}

	req := httptest.NewRequest(http.MethodGet, "/foo.js", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// TestRootHandlerPublicFallbackDirectory404s asserts that requesting a
// directory under public/ returns 404 rather than rendering an index listing —
// the TS Bun.file path serves files only.
func TestRootHandlerPublicFallbackDirectory404s(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}

	a := &OnDemand{log: discardLogger(), cfg: Config{PublicDir: dir}}

	req := httptest.NewRequest(http.MethodGet, "/sub", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// TestRootHandler_MidWithoutUnderscore404s pins that a ".mid" request returns
// 404. At rev-244, the .mid route is removed entirely (midi now flows over TCP
// OnDemand archive 3 per web.ts 9aadcec4). The Arc-31 M28 no-underscore
// panic guard is also gone since the whole branch is deleted.
func TestRootHandler_MidWithoutUnderscore404s(t *testing.T) {
	a := &OnDemand{log: discardLogger()}

	for _, p := range []string{"/x.mid", "/.mid", "/nounderscore.mid"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()

		// Must not panic, must 404 (route is gone at rev-244).
		a.RootHandler(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", p, rr.Code)
		}
	}
}

// TestRootHandler_MidBeatsArchivePrefix was testing the L47 dispatch order
// (the .mid branch preceding archive startsWith branches in TS web.ts:62-86).
// At rev-244, the .mid route is removed entirely from web.ts; the dispatch
// concern no longer applies. A /config_123.mid request now 404s (the .mid
// branch is gone; /config prefix only matches when the path starts with
// "/config" but ends in .mid — however the archive route for /config fires
// first without a FileStream; result is 404 regardless since no cache is
// configured here).
//
// This test is kept as a regression guard: the request must not accidentally
// serve the config archive as if it were a song.
func TestRootHandler_MidBeatsArchivePrefix(t *testing.T) {
	// No cache wired — /config archive route will 404. The test confirms the
	// old "serve a song file" path is gone regardless.
	t.Chdir(t.TempDir())

	songDir := filepath.Join("data", "pack", "client", "songs")
	if err := os.MkdirAll(songDir, 0o755); err != nil {
		t.Fatalf("mkdir songs: %v", err)
	}
	// The song file that the old handler would have served.
	if err := os.WriteFile(filepath.Join(songDir, "config.mid"), []byte("THE-SONG"), 0o644); err != nil {
		t.Fatalf("write song: %v", err)
	}
	// The decoy archive file (would be served by /config if cache were present).
	if err := os.WriteFile(filepath.Join("data", "pack", "client", "config"), []byte("THE-ARCHIVE"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	a := &OnDemand{log: discardLogger()} // no cache
	req := httptest.NewRequest(http.MethodGet, "/config_123.mid", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	// .mid route is gone; /config archive route fires but cache is nil → 404.
	// Must never serve "THE-SONG" (old .mid path is dead) or "THE-ARCHIVE"
	// (cache is nil so archive route also 404s).
	if rr.Code == http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("status = 200, want 404; body = %q (old .mid or archive path still active?)", body)
	}
}
