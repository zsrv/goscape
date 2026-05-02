package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestClearComListenersFiltersByRootLayer pins TS Player.ts:733-737:
// only listeners whose Component.RootLayer matches the arg are removed.
func TestClearComListenersFiltersByRootLayer(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 100},
		200: {RootLayer: 999},
	})
	p.invListenOnCom(93, 149, -1)
	p.invListenOnCom(94, 200, -1)

	received := drainConn(t, cc)
	p.clearComListeners(100)
	p.client.flushWrite()

	got := <-received
	// One UpdateInvStopTransmit packet = 1 opcode + 2 payload = 3 bytes.
	if len(got) != 3 {
		t.Errorf("packet bytes: got %d, want 3 (one StopTransmit)", len(got))
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener at 149 (RootLayer 100) should be removed")
	}
	if _, ok := p.invListeners[200]; !ok {
		t.Error("listener at 200 (RootLayer 999) should be retained")
	}
}

// TestClearComListenersRootMinusOneNoOp pins TS L729-731 early-return.
func TestClearComListenersRootMinusOneNoOp(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 100},
	})
	p.invListenOnCom(93, 149, -1)

	received := drainConn(t, cc)
	p.clearComListeners(-1)
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("rootCom=-1 should be a no-op; got %d bytes", len(got))
	}
	if _, ok := p.invListeners[149]; !ok {
		t.Error("listener at 149 should remain")
	}
}

// TestClearComListenersUnknownComponentSkipped pins the goscape-
// defensive nil-Component skip: a listener whose com is not in the
// registry must NOT be removed (defensive; TS assumes non-nil).
func TestClearComListenersUnknownComponentSkipped(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 1)
	// Seed a different component to ensure registry is non-nil.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		1: {RootLayer: 1},
	})
	p.invListenOnCom(93, 9999, -1) // 9999 not in registry

	received := drainConn(t, cc)
	p.clearComListeners(9999)
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("unknown com should be skipped; got %d bytes", len(got))
	}
	if _, ok := p.invListeners[9999]; !ok {
		t.Error("listener at 9999 (unknown com) should be retained")
	}
}

// TestClearComListenersRemovesMultipleSiblings pins multi-removal
// correctness + iteration safety under concurrent delete.
func TestClearComListenersRemovesMultipleSiblings(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		10: {RootLayer: 50},
		11: {RootLayer: 50},
		12: {RootLayer: 50},
		20: {RootLayer: 60},
	})
	p.invListenOnCom(93, 10, -1)
	p.invListenOnCom(93, 11, -1)
	p.invListenOnCom(93, 12, -1)
	p.invListenOnCom(93, 20, -1)

	received := drainConn(t, cc)
	p.clearComListeners(50)
	p.client.flushWrite()

	got := <-received
	// Three matched listeners → 3 × 3-byte packets = 9 bytes.
	if len(got) != 9 {
		t.Errorf("packet bytes: got %d, want 9 (3× StopTransmit)", len(got))
	}
	for _, com := range []int{10, 11, 12} {
		if _, ok := p.invListeners[com]; ok {
			t.Errorf("listener at %d should be removed", com)
		}
	}
	if _, ok := p.invListeners[20]; !ok {
		t.Error("listener at 20 (RootLayer 60) should be retained")
	}
}
