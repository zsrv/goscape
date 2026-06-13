package world

// handshake_test.go pins rev-244 B3 Task 18: the login handshake re-shape.
//
// TS contracts verified against 9aadcec4:
//
//  (a) Connect-time seed removed (TcpServer.ts:21-26 at 225 had it; GONE at 244):
//      A fresh connection receives NO unsolicited bytes.
//
//  (b) Opcode 14 (World.ts:2143-2156):
//      payload = 1 byte (the _loginServer byte, discarded).
//      reply = 8x00, then 0x00, then 8-byte seed (first word 24-bit-masked,
//      second word full 32-bit random). Total: 17 bytes. Two successive op-14s
//      produce different seeds (randomness smoke).
//
//  (c) Opcode 15 (World.ts:2240-2242):
//      payload = 0 bytes (opcode only). client.state→ClientStateOndemand;
//      reply = 8x00. Subsequent 4-byte frames are routed to s.onDemand.
//
//  (d) login OK reply = [2, min(staffModLevel,2), 1] at 254
//      (TS World.ts:946-950 @43e02957) — the 245.2 18/19 staff fork is gone.
//
//  (e) Op-16/18 regression: existing tests keep passing (framing unchanged).

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// readAllWithTimeout reads up to n bytes from r; returns what arrived in d or
// times out after dur. Used to assert "nothing arrived" (empty) or "exactly N".
func readAllWithTimeout(r io.Reader, n int, dur time.Duration) ([]byte, bool) {
	buf := make([]byte, n)
	ch := make(chan []byte, 1)
	go func() {
		if rd, ok := r.(interface{ SetReadDeadline(time.Time) error }); ok {
			_ = rd.SetReadDeadline(time.Now().Add(dur))
		}
		nread, _ := r.Read(buf)
		ch <- buf[:nread]
	}()
	select {
	case got := <-ch:
		return got, true
	case <-time.After(dur + 50*time.Millisecond):
		return nil, false
	}
}

// ---------------------------------------------------------------------------
// (a) No connect-time seed
// ---------------------------------------------------------------------------

// TestNoConnectTimeSeed pins rev-244 contract: a fresh connection receives
// NO bytes before the client sends any opcode.
// At 225, TcpServer.ts:24-27 sent an 8-byte seed immediately on connect.
// At 244, TcpServer.ts has no such send — the seed is embedded in the
// op-14 reply (World.ts:2151-2155).
func TestNoConnectTimeSeed(t *testing.T) {
	c, clientConn := newTestClient(t)
	_ = c // newClient initialises state; we only care about the wire side

	// Give the server side 100ms to send anything unsolicited.
	data, _ := readAllWithTimeout(clientConn, 64, 100*time.Millisecond)
	if len(data) != 0 {
		t.Errorf("connect-time seed should be absent at 244; got %d bytes: %v", len(data), data)
	}
}

// ---------------------------------------------------------------------------
// (b) Op-14 — checklogin handshake
// ---------------------------------------------------------------------------

// TestOp14ReplyIs17Bytes pins that handleLogin with opcode 14 + 1-byte payload
// returns exactly 17 bytes: 8 zero bytes, 0x00, then an 8-byte seed.
// Ref: World.ts:2147-2155 (244 pin 9aadcec4).
func TestOp14ReplyIs17Bytes(t *testing.T) {
	c, clientConn := newTestClient(t)

	// Wire: opcode(14) + payload(1 byte = _loginServer, discarded by TS).
	c.bufferData([]byte{14, 0x00})

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		_ = clientConn.(interface{ SetReadDeadline(time.Time) error }).SetReadDeadline(time.Now().Add(time.Second))
		n, _ := clientConn.Read(buf)
		received <- buf[:n]
	}()

	if err := c.handleLogin(); err != nil {
		t.Fatalf("handleLogin op14: unexpected err: %v", err)
	}
	_ = c.flushWrite() // flush after handleLogin writes

	select {
	case got := <-received:
		if len(got) != 17 {
			t.Fatalf("op-14 reply: got %d bytes, want 17; bytes=%v", len(got), got)
		}
		// First 8 bytes must be zero (TS: client.send([0,0,0,0,0,0,0,0]))
		if !bytes.Equal(got[:8], make([]byte, 8)) {
			t.Errorf("op-14 reply bytes 0-7: got %v, want 8 zeros", got[:8])
		}
		// Byte 8 must be 0x00 (TS: client.send([0]))
		if got[8] != 0x00 {
			t.Errorf("op-14 reply byte 8: got 0x%02x, want 0x00", got[8])
		}
		// Bytes 9-12 (first seed word): 24-bit mask → high byte must be 0.
		// TS: p4(Math.floor(Math.random() * 0x00ffffff)) — top byte is 0.
		if got[9] != 0x00 {
			t.Errorf("op-14 seed byte 9 (high byte of first word): got 0x%02x, want 0x00 (24-bit mask)", got[9])
		}
		// Bytes 13-16 (second seed word): full 32-bit — no constraint except it's present.
		// (already validated by len==17)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for op-14 reply")
	}
}

// TestOp14SeedVariesAcrossRequests pins that two successive op-14s produce
// different seed bytes. This is a randomness smoke test (not cryptographically
// rigorous, but op-14 must not return a constant value).
func TestOp14SeedVariesAcrossRequests(t *testing.T) {
	seeds := make([][]byte, 2)
	for i := range seeds {
		c, clientConn := newTestClient(t)
		c.bufferData([]byte{14, 0x00})

		received := make(chan []byte, 1)
		go func() {
			buf := make([]byte, 64)
			_ = clientConn.(interface{ SetReadDeadline(time.Time) error }).SetReadDeadline(time.Now().Add(time.Second))
			n, _ := clientConn.Read(buf)
			received <- buf[:n]
		}()

		if err := c.handleLogin(); err != nil {
			t.Fatalf("handleLogin op14 run %d: %v", i, err)
		}
		_ = c.flushWrite()

		select {
		case got := <-received:
			if len(got) != 17 {
				t.Fatalf("run %d: got %d bytes, want 17", i, len(got))
			}
			seeds[i] = got[9:] // 8-byte seed portion
		case <-time.After(time.Second):
			t.Fatalf("run %d: timed out", i)
		}
	}

	// It's astronomically unlikely (p≈2^-64) for two random 8-byte values to match.
	if bytes.Equal(seeds[0], seeds[1]) {
		t.Errorf("op-14 seeds identical across two requests: %v — seed must be random", seeds[0])
	}
}

// ---------------------------------------------------------------------------
// (c) Op-15 — OnDemand entry
// ---------------------------------------------------------------------------

// TestOp15ReplyIs8ZerosAndStateTransition pins that handleLogin with opcode 15
// (no payload) writes 8 zero bytes and transitions state to ClientStateOndemand.
// Ref: World.ts:2240-2242 (244 pin 9aadcec4).
func TestOp15ReplyIs8ZerosAndStateTransition(t *testing.T) {
	c, clientConn := newTestClient(t)

	// Wire: opcode 15 only (no payload per TS: client.waiting stays 0).
	c.bufferData([]byte{15})

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		_ = clientConn.(interface{ SetReadDeadline(time.Time) error }).SetReadDeadline(time.Now().Add(time.Second))
		n, _ := clientConn.Read(buf)
		received <- buf[:n]
	}()

	if err := c.handleLogin(); err != nil {
		t.Fatalf("handleLogin op15: unexpected err: %v", err)
	}
	_ = c.flushWrite()

	select {
	case got := <-received:
		if len(got) != 8 {
			t.Fatalf("op-15 reply: got %d bytes, want 8; bytes=%v", len(got), got)
		}
		if !bytes.Equal(got, make([]byte, 8)) {
			t.Errorf("op-15 reply: got %v, want 8 zero bytes", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for op-15 reply")
	}

	if c.state != ClientStateOndemand {
		t.Errorf("state after op-15: got %v, want ClientStateOndemand (%v)", c.state, ClientStateOndemand)
	}
}

// TestOp15OnDemandRoutingIntegration pins that after op-15 transitions to
// ClientStateOndemand, subsequent 4-byte frames are routed to s.onDemand
// via the *clientODAdapter. Feed one valid urgent request and verify it
// lands in the client's priority-2 queue (rev-274 per-client model).
func TestOp15OnDemandRoutingIntegration(t *testing.T) {
	c, clientConn := newTestClient(t)
	s := newTestServer(t)
	s.onDemand = newOnDemand(nil)
	c.server = s

	// Drain the clientConn side (don't care about the reply bytes here).
	go io.Copy(io.Discard, clientConn)

	// Transition to OnDemand via op-15.
	c.bufferData([]byte{15})
	if err := c.handleLogin(); err != nil {
		t.Fatalf("handleLogin op15: %v", err)
	}
	_ = c.flushWrite()

	if c.state != ClientStateOndemand {
		t.Fatalf("state must be ClientStateOndemand after op-15, got %v", c.state)
	}

	// Now simulate the conn read-loop routing: feed a 4-byte OnDemand frame
	// (archive=0, file=1, priority=2 → urgent) through the connection handler's
	// ondemand state branch, which should call s.onDemand.onClientData.
	odFrame := []byte{0, 0, 1, 2} // archive=0, file=0x0001, priority=2 → urgent
	c.bufferData(odFrame)

	// Manually invoke the ondemand routing path (mirrors what the read loop does
	// in state ClientStateOndemand after the implementation is in place).
	adapter := &clientODAdapter{c: c}
	consumed := s.onDemand.onClientData(adapter, odFrame)
	if consumed != 4 {
		t.Fatalf("onClientData consumed %d bytes, want 4", consumed)
	}

	s.onDemand.mu.Lock()
	defer s.onDemand.mu.Unlock()
	cq, ok := s.onDemand.clients[adapter.id()]
	if !ok {
		t.Fatal("no clientQueue created for the connection")
	}
	if len(cq.queues[2]) != 1 {
		t.Fatalf("priority-2 queue: got %d entries, want 1", len(cq.queues[2]))
	}
	if cq.queues[2][0].archive != 0 || cq.queues[2][0].file != 1 {
		t.Errorf("queues[2][0]: got archive=%d file=%d, want 0 1", cq.queues[2][0].archive, cq.queues[2][0].file)
	}
}

// ---------------------------------------------------------------------------
// (d) staffModLevel — capped staff byte inside the 3-byte login OK reply
// ---------------------------------------------------------------------------

// TestSendLoginOKStaffTierMatrix pins the 254 login-OK reply for all staff
// tiers. TS World.ts:946-950 @43e02957: always [2, min(staffModLevel, 2), 1]
// — the 245.2 opcode-18/19 fork is gone; the trailing 1 enables client-side
// mouse tracking (Client.java:2451-2452 @2e629784 reads staffmodlevel then
// mouseTracked after response code 2).
// The state transition to ClientStateGame is asserted per row.
func TestSendLoginOKStaffTierMatrix(t *testing.T) {
	cases := []struct {
		staffModLevel int32
		wantBytes     [3]byte
		label         string
	}{
		{0, [3]byte{2, 0, 1}, "normal(0)→[2,0,1]"},
		{1, [3]byte{2, 1, 1}, "mod(1)→[2,1,1]"},
		{2, [3]byte{2, 2, 1}, "supermod(2)→[2,2,1]"},
		{3, [3]byte{2, 2, 1}, "admin(3)→[2,2,1] (min cap)"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			c, clientConn := newTestClient(t)
			c.staffModLevel = tc.staffModLevel

			received := make(chan [3]byte, 1)
			go func() {
				var buf [3]byte
				_ = clientConn.(interface{ SetReadDeadline(time.Time) error }).SetReadDeadline(time.Now().Add(time.Second))
				if _, err := io.ReadFull(clientConn, buf[:]); err == nil {
					received <- buf
				}
			}()

			if err := c.sendLoginOK(); err != nil {
				t.Fatalf("sendLoginOK: %v", err)
			}

			select {
			case got := <-received:
				if got != tc.wantBytes {
					t.Errorf("got bytes %v, want %v", got, tc.wantBytes)
				}
			case <-time.After(time.Second):
				t.Fatal("timeout")
			}

			if c.state != ClientStateGame {
				t.Errorf("state after sendLoginOK: got %v, want ClientStateGame", c.state)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// (e) Regression: op-16 unknown-opcode still closes
// ---------------------------------------------------------------------------

// TestHandleLoginUnknownOpcodeClosesConn pins TS World.ts:2243-2244:
// unknown opcodes result in client.terminate() (≈ errCloseConn in Go).
// This existed before B3; confirm it still holds after adding 14/15 cases.
func TestHandleLoginUnknownOpcodeClosesConn(t *testing.T) {
	c, clientConn := newTestClient(t)
	go io.Copy(io.Discard, clientConn)

	c.bufferData([]byte{99}) // opcode 99 — no TS handler at any rev

	err := c.handleLogin()
	if err == nil {
		t.Fatal("handleLogin: want error for unknown opcode, got nil")
	}
}
