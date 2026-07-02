package world

import (
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

// arch-28.4b: buffers may be pool-returned only after BOTH the conn
// goroutine and the tick have dropped their refs; each side's drop is
// idempotent. Run with -race: pre-fix the conn-side release races the
// tick-side flush.
func TestClientTeardownRefcount(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	c := newClient(c2, time.Second, slog.Default())

	if got := c.teardownRefs.Load(); got != 1 {
		t.Fatalf("fresh client refs: got %d, want 1", got)
	}
	c.teardownRefs.Add(1) // simulate successful login (tick becomes co-owner)

	var wg sync.WaitGroup
	wg.Go(func() { c.dropConnRef() })
	wg.Go(func() {
		c.bufw.WriteByte(0) // tick-side write while conn side is tearing down
		c.dropTickRef()
	})
	wg.Wait()

	if got := c.teardownRefs.Load(); got != 0 {
		t.Fatalf("refs after both drops: got %d, want 0", got)
	}
	c.dropTickRef() // double-drop must be a no-op (idle logout + disconnect)
	c.dropConnRef()
	if got := c.teardownRefs.Load(); got != 0 {
		t.Fatalf("refs after redundant drops: got %d, want 0 (no double release)", got)
	}
}
