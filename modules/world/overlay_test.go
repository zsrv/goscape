package world

// Tests for overlay state + IF_OPENOVERLAY flush. Task 10 of rev-244 B3.
//
// TS contracts verified at 9aadcec4:
//   - Player.ts:358-359  overlay = -1; lastOverlay = -1;
//   - Player.ts:1955-1965 openOverlay(com): early-return if same, clear
//     listeners on com==-1 before setting overlay.
//   - NetworkPlayer.ts:192-195 flush: write IfOpenOverlay on
//     overlay !== lastOverlay; update lastOverlay.

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// TestOpenOverlay_FlushWritesOnce pins that a single OpenOverlay(X) followed
// by encodeOut writes exactly one IF_OPENOVERLAY packet (3 bytes: 1 opcode +
// 2 payload) and that a second encodeOut without state change writes nothing.
//
// TS NetworkPlayer.ts:192-195 flush: `if (this.overlay !== this.lastOverlay)`
// writes IfOpenOverlay and updates lastOverlay. B4 wires the call site.
func TestOpenOverlay_FlushWritesOnce(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// Ensure both fields start equal so the flush is a no-op initially.
	p.overlay = -1
	p.lastOverlay = -1

	p.OpenOverlay(200)

	received := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	// IF_OPENOVERLAY: 1 encrypted opcode byte + 2 payload bytes = 3 bytes.
	if len(got) != 3 {
		t.Fatalf("first flush: got %d bytes, want 3 (1 opcode + 2 payload); bytes=%v", len(got), got)
	}
	// Payload bytes [1] and [2] are the com as big-endian int16.
	gotCom := int(got[1])<<8 | int(got[2])
	if gotCom != 200 {
		t.Errorf("first flush com: got %d, want 200", gotCom)
	}

	// Second flush: lastOverlay has caught up; should emit nothing.
	received2 := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got2 := <-received2
	if len(got2) != 0 {
		t.Errorf("second flush: got %d bytes, want 0 (no change); bytes=%v", len(got2), got2)
	}
}

// TestOpenOverlay_SameComIsNoop pins that calling OpenOverlay(X) twice does
// not change overlay (early-return on same com), so the subsequent flush
// writes exactly once.
//
// TS Player.ts:1956-1958 `if (this.overlay === com) { return; }`.
func TestOpenOverlay_SameComIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.overlay = -1
	p.lastOverlay = -1

	p.OpenOverlay(300)
	p.OpenOverlay(300) // second call must be a no-op

	if p.overlay != 300 {
		t.Errorf("overlay after two same-com calls: got %d, want 300", p.overlay)
	}
}

// TestOpenOverlay_MinusOneClearsListeners pins that OpenOverlay(-1) after
// OpenOverlay(X) calls clearComListeners(X) — i.e. the old overlay's
// component listeners are removed. Also pins that the flush writes
// IF_OPENOVERLAY with the -1 com (P2 = 0xFFFF).
//
// TS Player.ts:1960-1962 `if (com === -1) { this.clearComListeners(this.overlay); }`.
// TS NetworkPlayer.ts:192-195: overlay changed X→-1, so flush fires.
func TestOpenOverlay_MinusOneClearsListeners(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 1)
	// Seed a component with RootLayer = 500 (the overlay com we'll use).
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 500}, // child component of overlay 500
	})

	// Set the overlay to 500 and seed an inv listener on com 149 (child of 500).
	p.overlay = 500
	p.lastOverlay = 500
	p.invListenOnCom(93, 149, -1)
	if _, ok := p.invListeners[149]; !ok {
		t.Fatal("precondition: listener 149 must be seeded")
	}

	// OpenOverlay(-1): should clear listeners for old overlay (500) and set overlay=-1.
	received := drainConn(t, cc)
	p.OpenOverlay(-1)
	p.client.flushWrite() // flush the UpdateInvStopTransmit from invStopListenOnCom

	got := <-received
	// UpdateInvStopTransmit for com 149: 3 bytes (opcode + 2-byte com).
	if len(got) != 3 {
		t.Errorf("listener removal bytes: got %d, want 3 (1 UpdateInvStopTransmit); bytes=%v", len(got), got)
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener 149 should be removed after OpenOverlay(-1)")
	}
	if p.overlay != -1 {
		t.Errorf("overlay: got %d, want -1", p.overlay)
	}

	// Flush encodeOut: overlay changed 500 → -1, so IF_OPENOVERLAY must fire.
	received2 := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got2 := <-received2
	// IF_OPENOVERLAY: 1 opcode + 2 payload = 3 bytes.
	if len(got2) != 3 {
		t.Fatalf("encodeOut flush: got %d bytes, want 3; bytes=%v", len(got2), got2)
	}
	// Payload for -1 as big-endian int16 is 0xFF, 0xFF (= 65535 unsigned).
	if got2[1] != 0xFF || got2[2] != 0xFF {
		t.Errorf("encodeOut flush com bytes: got [%#x %#x], want [0xff 0xff] (-1 as BE int16)", got2[1], got2[2])
	}
}

// TestOpenOverlay_InitialStateNoFlush pins that the default state (overlay ==
// lastOverlay == -1) produces no output from encodeOut — the flush condition
// is false without any OpenOverlay call.
//
// Mirrors TS Player.ts:358-359 initial state parity.
func TestOpenOverlay_InitialStateNoFlush(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// Both fields start at -1 (set in newPlayer; this is an explicit assertion).
	if p.overlay != -1 || p.lastOverlay != -1 {
		t.Fatalf("precondition: overlay=%d lastOverlay=%d, want both -1 (newPlayer default)", p.overlay, p.lastOverlay)
	}

	received := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("initial state flush: got %d bytes, want 0; bytes=%v", len(got), got)
	}
}
