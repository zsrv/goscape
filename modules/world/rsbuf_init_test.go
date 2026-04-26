package world

import (
	"testing"
)

// TestServer_RsbufInitialized confirms that a freshly-constructed Server
// has a non-nil *rsbuf.Buf field. Bundle 4 wiring; NAI-29 Task 4.1.
func TestServer_RsbufInitialized(t *testing.T) {
	s := newTestServer(t)
	if s.rsbuf == nil {
		t.Fatal("Server.rsbuf is nil after newTestServer; expected initialized *rsbuf.Buf")
	}
}
