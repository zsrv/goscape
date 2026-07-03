package world

import (
	"testing"
	"time"
)

// arch-29.2: pins the keepalive contract. Time must be low enough to
// detect a dead NAT flow before players notice frozen friends state;
// PermitWithoutStream keeps the probe alive between RPCs.
func TestClientKeepaliveParams(t *testing.T) {
	p := clientKeepaliveParams()
	if p.Time != 30*time.Second {
		t.Errorf("Time: got %v, want 30s", p.Time)
	}
	if p.Timeout != 10*time.Second {
		t.Errorf("Timeout: got %v, want 10s", p.Timeout)
	}
	if !p.PermitWithoutStream {
		t.Error("PermitWithoutStream must be true (subscriber streams are idle-heavy)")
	}
}
