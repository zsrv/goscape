package world

// login_ratelimit_test.go pins rev-254 A4: the op-14 per-address and
// op-16/18 per-device login rate limits plus the ttlAttemptCache backing
// them.
//
// TS contracts verified against Engine-TS @2e3bcf43:
//
//	World.ts:176-177  — TTLCache windows: address 60s, device 15s.
//	World.ts:2104-2117 — op-14: the 8 zero bytes are sent BEFORE the
//	    address check; gated on NODE_PRODUCTION && threshold > 0;
//	    increment on EVERY attempt; attempts >= threshold → [16] + close.
//	World.ts:2172-2184 — op-16/18: key `${uid}@${remoteAddress}` (uid
//	    read signed via g4s at :2168); same gate/increment/threshold
//	    semantics → [16] + close.

import (
	"bytes"
	"net"
	"testing"
	"time"

	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
)

// op14Attempt feeds one op-14 request through handleLogin and returns the
// raw reply bytes plus handleLogin's error. Each handleLogin write path
// ends in exactly one flush, so a single Read captures the whole reply.
func op14Attempt(t *testing.T, c *client, clientConn net.Conn) ([]byte, error) {
	t.Helper()
	c.bufferData([]byte{14, 0x00})

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		_ = clientConn.(interface{ SetReadDeadline(time.Time) error }).SetReadDeadline(time.Now().Add(time.Second))
		n, _ := clientConn.Read(buf)
		received <- buf[:n]
	}()

	err := c.handleLogin()
	select {
	case got := <-received:
		return got, err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for op-14 reply")
		return nil, err
	}
}

// newRatelimitedClient builds a client wired to a production-mode test
// server with the given thresholds.
func newRatelimitedClient(t *testing.T, addressLimit, deviceLimit int) (*client, net.Conn, *Server) {
	t.Helper()
	c, clientConn := newTestClient(t)
	s := newTestServer(t)
	s.cfg.NodeProduction = true
	s.cfg.NodeRatelimitAddressLogin = addressLimit
	s.cfg.NodeRatelimitDeviceLogin = deviceLimit
	c.server = s
	return c, clientConn, s
}

// TestTTLAttemptCacheSlidingWindow pins the isaacs/ttlcache subset the TS
// attempt pattern exercises: every set re-arms the FULL TTL, so the window
// slides from the LAST attempt; an expired entry restarts at 1.
func TestTTLAttemptCacheSlidingWindow(t *testing.T) {
	var now time.Time
	base := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	cache := &ttlAttemptCache{nowFn: func() time.Time { return now }}
	const ttl = 10 * time.Second

	now = base
	if got := cache.bump("k", ttl); got != 1 {
		t.Fatalf("bump 1: got %d, want 1", got)
	}
	// 9s later — inside the window → 2, and the window re-arms from here.
	now = base.Add(9 * time.Second)
	if got := cache.bump("k", ttl); got != 2 {
		t.Fatalf("bump 2 (+9s): got %d, want 2", got)
	}
	// 18s after base but only 9s after the LAST attempt → still counting.
	now = base.Add(18 * time.Second)
	if got := cache.bump("k", ttl); got != 3 {
		t.Fatalf("bump 3 (+18s): got %d, want 3 (sliding window)", got)
	}
	// 10s exactly after the last attempt — entry expired (now !< expires) → reset.
	now = base.Add(28 * time.Second)
	if got := cache.bump("k", ttl); got != 1 {
		t.Fatalf("bump after expiry: got %d, want 1", got)
	}
	// Independent keys do not interact.
	if got := cache.bump("other", ttl); got != 1 {
		t.Fatalf("bump other key: got %d, want 1", got)
	}
}

// TestOp14AddressLimitThirtiethAttemptRejected pins the default-threshold
// (30) contract end-to-end: attempts 1-29 get the full 17-byte handshake
// reply; the 30th gets the 8 zero bytes FOLLOWED by reply byte 16 (TS
// sends the zeros at World.ts:2105 before the check at :2107-2117), and
// the connection closes (errCloseConn).
func TestOp14AddressLimitThirtiethAttemptRejected(t *testing.T) {
	// NOTE: the loop reuses one *client across 30 handleLogin calls.
	// That works because the op-14 path only consumes c.in (refilled per
	// attempt) and writes c.bufw — sendLoginError neither closes the
	// conn nor flips c.state. If handleLogin ever starts doing either on
	// reject, give each attempt a fresh client instead.
	c, clientConn, _ := newRatelimitedClient(t, 30, 5)

	for i := 1; i <= 29; i++ {
		got, err := op14Attempt(t, c, clientConn)
		if err != nil {
			t.Fatalf("attempt %d: unexpected err %v", i, err)
		}
		if len(got) != 17 {
			t.Fatalf("attempt %d: got %d bytes, want 17", i, len(got))
		}
	}

	got, err := op14Attempt(t, c, clientConn)
	if err != errCloseConn {
		t.Errorf("attempt 30: err = %v, want errCloseConn", err)
	}
	// Wire pin: zeros-before-reject — exactly [0]*8 then 16.
	want := append(make([]byte, 8), loginresp.OpTooManyAttempts.Opcode)
	if !bytes.Equal(got, want) {
		t.Errorf("attempt 30 reply: got %v, want %v (8 zeros then byte 16)", got, want)
	}
}

// TestOp14AddressLimitWindowExpiryResets pins the 60s sliding window:
// once the TTL elapses since the LAST attempt, the count restarts and a
// previously-limited address is admitted again.
func TestOp14AddressLimitWindowExpiryResets(t *testing.T) {
	c, clientConn, s := newRatelimitedClient(t, 3, 5)
	var now time.Time
	base := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s.loginAddressAttempts.nowFn = func() time.Time { return now }

	now = base
	for i := 1; i <= 2; i++ {
		if _, err := op14Attempt(t, c, clientConn); err != nil {
			t.Fatalf("attempt %d: unexpected err %v", i, err)
		}
	}
	// Advance past the 60s address TTL — the 3rd attempt counts as 1 again.
	now = base.Add(loginAddressAttemptTTL + time.Second)
	got, err := op14Attempt(t, c, clientConn)
	if err != nil {
		t.Fatalf("post-expiry attempt: unexpected err %v", err)
	}
	if len(got) != 17 {
		t.Fatalf("post-expiry attempt: got %d bytes, want 17", len(got))
	}
	// Control: two more inside the new window hit the threshold (count 3 >= 3).
	if _, err := op14Attempt(t, c, clientConn); err != nil {
		t.Fatalf("attempt 2 of new window: unexpected err %v", err)
	}
	gotReject, err := op14Attempt(t, c, clientConn)
	if err != errCloseConn {
		t.Errorf("attempt 3 of new window: err = %v, want errCloseConn", err)
	}
	want := append(make([]byte, 8), loginresp.OpTooManyAttempts.Opcode)
	if !bytes.Equal(gotReject, want) {
		t.Errorf("attempt 3 of new window: got %v, want %v", gotReject, want)
	}
}

// TestOp14AddressLimitGating pins the TS gate (World.ts:2107): the limit
// only runs when NODE_PRODUCTION && threshold > 0.
func TestOp14AddressLimitGating(t *testing.T) {
	t.Run("non_production", func(t *testing.T) {
		c, clientConn, s := newRatelimitedClient(t, 3, 5)
		s.cfg.NodeProduction = false
		for i := 1; i <= 5; i++ {
			got, err := op14Attempt(t, c, clientConn)
			if err != nil || len(got) != 17 {
				t.Fatalf("attempt %d: err=%v len=%d, want nil/17 (limit disabled off-production)", i, err, len(got))
			}
		}
	})
	t.Run("zero_threshold", func(t *testing.T) {
		c, clientConn, _ := newRatelimitedClient(t, 0, 5)
		for i := 1; i <= 5; i++ {
			got, err := op14Attempt(t, c, clientConn)
			if err != nil || len(got) != 17 {
				t.Fatalf("attempt %d: err=%v len=%d, want nil/17 (threshold 0 disables)", i, err, len(got))
			}
		}
	})
}

// TestDeviceLoginLimited pins the per-device limit helper feeding the
// op-16/18 path (World.ts:2172-2184): keyed `${uid}@${remoteAddress}`
// with the uid rendered SIGNED (TS g4s), threshold compare is >=, the
// 15s window slides, and the gate honors production/threshold.
func TestDeviceLoginLimited(t *testing.T) {
	t.Run("fifth_attempt_limited_and_uid_keyed", func(t *testing.T) {
		c, _, s := newRatelimitedClient(t, 30, 5)
		for i := 1; i <= 4; i++ {
			if c.deviceLoginLimited(1234) {
				t.Fatalf("uid 1234 attempt %d: limited early (want attempts < 5 admitted)", i)
			}
		}
		if !c.deviceLoginLimited(1234) {
			t.Error("uid 1234 attempt 5: want limited (attempts >= 5)")
		}
		// A different uid from the same address counts independently.
		for i := 1; i <= 4; i++ {
			if c.deviceLoginLimited(99) {
				t.Fatalf("uid 99 attempt %d: limited early — key must include the uid", i)
			}
		}
		// Rejected attempts kept incrementing uid 1234's count.
		if !c.deviceLoginLimited(1234) {
			t.Error("uid 1234 attempt 6: want still limited")
		}
		// Signed key rendering: TS reads the uid via g4s, so 0xFFFFFFFF
		// renders as "-1" in the key (World.ts:2168+2173).
		c.deviceLoginLimited(0xFFFFFFFF)
		s.loginDeviceAttempts.mu.Lock()
		_, negKey := s.loginDeviceAttempts.entries["-1@pipe"]
		s.loginDeviceAttempts.mu.Unlock()
		if !negKey {
			t.Error(`uid 0xFFFFFFFF: want cache key "-1@pipe" (signed g4s rendering)`)
		}
	})
	t.Run("window_expiry_resets", func(t *testing.T) {
		c, _, s := newRatelimitedClient(t, 30, 3)
		var now time.Time
		base := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
		s.loginDeviceAttempts.nowFn = func() time.Time { return now }

		now = base
		c.deviceLoginLimited(7)
		c.deviceLoginLimited(7)
		now = base.Add(loginDeviceAttemptTTL + time.Second)
		if c.deviceLoginLimited(7) {
			t.Error("post-expiry attempt: want admitted (count reset to 1)")
		}
	})
	t.Run("gating", func(t *testing.T) {
		c, _, s := newRatelimitedClient(t, 30, 1)
		s.cfg.NodeProduction = false
		if c.deviceLoginLimited(1) {
			t.Error("non-production: want limit disabled")
		}
		s.cfg.NodeProduction = true
		s.cfg.NodeRatelimitDeviceLogin = 0
		if c.deviceLoginLimited(1) {
			t.Error("threshold 0: want limit disabled")
		}
		// No server at all (unit-test clients): disabled.
		c.server = nil
		if c.deviceLoginLimited(1) {
			t.Error("nil server: want limit disabled")
		}
	})
}

// TestSendLoginHopTimerWire pins the hop-timer reject wire shape
// [21, min(255, remainingMs/1000)] (TS World.ts:1861-1866 @2e3bcf43;
// JS Uint8Array.from truncates the float toward zero) and that the
// helper returns errCloseConn.
func TestSendLoginHopTimerWire(t *testing.T) {
	cases := []struct {
		remainingMs int64
		wantSecs    byte
		label       string
	}{
		{44_999, 44, "44999ms→44 (truncation, not rounding)"},
		{1_000, 1, "1000ms→1"},
		{999, 0, "999ms→0 (sub-second truncates)"},
		{255_000, 255, "255000ms→255"},
		{300_000, 255, "300000ms→255 (clamp)"},
		{0, 0, "0ms→0 (defensive; LoginServer only emits remaining>0)"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			c, clientConn := newTestClient(t)

			received := make(chan []byte, 1)
			go func() {
				buf := make([]byte, 8)
				_ = clientConn.(interface{ SetReadDeadline(time.Time) error }).SetReadDeadline(time.Now().Add(time.Second))
				n, _ := clientConn.Read(buf)
				received <- buf[:n]
			}()

			err := c.sendLoginHopTimer(tc.remainingMs)
			if err != errCloseConn {
				t.Errorf("err = %v, want errCloseConn", err)
			}
			select {
			case got := <-received:
				want := []byte{loginresp.OpHopTimer.Opcode, tc.wantSecs}
				if !bytes.Equal(got, want) {
					t.Errorf("wire: got %v, want %v", got, want)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for hop-timer reply")
			}
		})
	}
}
