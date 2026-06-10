package world

import (
	"flag"
	"testing"
	"time"
)

func TestConfigNodeLimitBytesPerTrackingSessionDefault(t *testing.T) {
	var c Config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.NodeLimitBytesPerTrackingSession != 50000 {
		t.Errorf("NodeLimitBytesPerTrackingSession default: got %d, want 50000", c.NodeLimitBytesPerTrackingSession)
	}
}

// NodeSubmitInput defaults false, mirroring TS Environment.ts
// NODE_SUBMIT_INPUT (still defined upstream at 254 with no readers —
// the flag is kept for config compatibility; see config.go).
func TestConfigNodeSubmitInputDefault(t *testing.T) {
	var c Config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.NodeSubmitInput {
		t.Error("NodeSubmitInput default: got true, want false (TS Environment.ts NODE_SUBMIT_INPUT default false)")
	}
}

// logger-transport-3: TS TcpServer.ts:19 sets the idle-socket timeout to
// 30000 ms via `s.setTimeout(30000)`. goscape's TCPServerReadTimeout drives
// the same "kill the connection if no client traffic arrives within N"
// behaviour (server.go:831 `SetReadDeadline(Now + TCPServerReadTimeout)` →
// next Read returns a timeout error → connection terminated). Pre-fix
// default was 5s (six times faster than TS), so idle clients between
// game ticks could be disconnected aggressively where TS would have
// waited. Pin the default at 30s to match TS.
func TestConfigTCPServerReadTimeoutDefault(t *testing.T) {
	var c Config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.TCPServerReadTimeout != 30*time.Second {
		t.Errorf("TCPServerReadTimeout default: got %v, want 30s (TS TcpServer.ts:19 setTimeout(30000))", c.TCPServerReadTimeout)
	}
}
