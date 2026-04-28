package asset

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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
