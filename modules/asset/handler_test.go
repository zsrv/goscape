package asset

import (
	"bytes"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zsrv/goscape/internal/dskit/middleware"
	"github.com/zsrv/goscape/pkg/cache"
)

// TestRootHandlerCrcEndpointServesOnEveryRequest pins the fix for the
// CrcBuffer-as-stateful-reader bug: both the first and second /crc request
// must return the full CrcBytes payload, not an empty body.
func TestRootHandlerCrcEndpointServesOnEveryRequest(t *testing.T) {
	prev := cache.CrcBytes
	t.Cleanup(func() { cache.CrcBytes = prev })
	cache.CrcBytes = []byte{0x00, 0x00, 0x00, 0x00, 0xDE, 0xAD, 0xBE, 0xEF}

	a := &Asset{log: discardLogger()}

	for i := range 2 {
		req := httptest.NewRequest(http.MethodGet, "/crc", nil)
		rr := httptest.NewRecorder()
		a.RootHandler(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, rr.Code)
		}
		got, _ := io.ReadAll(rr.Body)
		if !bytes.Equal(got, cache.CrcBytes) {
			t.Fatalf("request %d: body = %v, want %v", i+1, got, cache.CrcBytes)
		}
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestAssetClientIPExtractsTSParityHeaders pins source-IP wiring: the
// asset's SourceIPExtractor must consult cf-connecting-ip (the TS upstream's
// getIp() primary header in Engine-TS/src/web.ts) and return only the first
// comma-separated address to match TS' .split(',')[0].trim() behaviour.
//
// Note: dskit's SourceIPExtractor is single-source — when LogSourceIPsHeader
// is non-empty, only that header is consulted; the built-in Forwarded /
// X-Real-IP / X-Forwarded-For chain is bypassed. Operators wanting that
// chain (covering x-forwarded-for) must blank the header/regex flags.
func TestAssetClientIPExtractsTSParityHeaders(t *testing.T) {
	// Build the asset with the default flag values so the test covers the
	// production header/regex defaults wired in config.go.
	cfg := Config{Enable: true}
	cfg.RegisterFlagsAndApplyDefaults(flag.NewFlagSet("asset-test", flag.ContinueOnError))

	sourceIPs, err := middleware.NewSourceIPs(cfg.Server.LogSourceIPsHeader, cfg.Server.LogSourceIPsRegex, cfg.Server.LogSourceIPsFull)
	if err != nil {
		t.Fatalf("NewSourceIPs: %v", err)
	}
	a := &Asset{log: discardLogger(), sourceIPs: sourceIPs}

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

// TestAssetClientIPDefaultChainFallback pins the alternative configuration:
// when LogSourceIPsHeader/Regex are blank, the dskit SourceIPExtractor falls
// back to its built-in Forwarded / X-Real-IP / X-Forwarded-For chain.
func TestAssetClientIPDefaultChainFallback(t *testing.T) {
	sourceIPs, err := middleware.NewSourceIPs("", "", false)
	if err != nil {
		t.Fatalf("NewSourceIPs: %v", err)
	}
	a := &Asset{log: discardLogger(), sourceIPs: sourceIPs}

	req := httptest.NewRequest(http.MethodGet, "/title", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.1.1.1")

	if got, want := a.clientIP(req), "198.51.100.9, 10.0.0.1"; got != want {
		t.Fatalf("clientIP() = %q, want %q", got, want)
	}
}

// TestAssetClientIPNilExtractorReturnsEmpty pins the nil-extractor branch:
// if the asset was constructed without a SourceIPExtractor (e.g. tests that
// bypass New), clientIP must not panic.
func TestAssetClientIPNilExtractorReturnsEmpty(t *testing.T) {
	a := &Asset{log: discardLogger()}
	req := httptest.NewRequest(http.MethodGet, "/title", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	req.Header.Set("CF-Connecting-IP", "203.0.113.7")

	if got := a.clientIP(req); got != "" {
		t.Fatalf("clientIP() with nil extractor = %q, want empty", got)
	}
}
