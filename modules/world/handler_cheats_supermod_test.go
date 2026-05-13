package world

import (
	"bytes"
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// supermodSetup wires a Player into a Server at staffModLevel=2 (super-mod
// gate), with cfg.NodeProduction=true, recordingBridges installed, and a
// fixed-seed ISAAC encryptor so cheat-dispatched MessageGame writes go
// through writeOut without panic.
func supermodSetup(t *testing.T) (*Player, net.Conn, *Server, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.cfg.NodeProduction = true
	rec := installRecordingBridges(s)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.username = "alice"
	p.staffModLevel = 2
	p.x, p.z, p.level = 3200, 3200, 0
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(p.x, p.z, p.level)
	return p, cc, s, rec
}

// dispatchCheat sends `<cmd> <args>` (without the `::` prefix per TS L42)
// through handleClientCheat. Distinct from dispatchTeleCheat in
// handlers_game_test.go which is named after the original ::tele use case.
func dispatchCheat(t *testing.T, p *Player, cheat string) {
	t.Helper()
	pkt := packet.NewPacket(nil)
	pkt.P1(0) // ctrlHeld byte (unused)
	pkt.PJStrLF(cheat)
	if err := handleClientCheat(p, pkt.Data); err != nil {
		t.Fatalf("handleClientCheat: %v", err)
	}
}

// TestSetvisDispatchDefault pins TS ClientCheatHandler.ts:557-558 case '0'
// → Player.setVisibility(DEFAULT). Full state assertion lives in
// TestPlayerSetVisibilityDefault; here we pin only that dispatch reaches
// SetVisibility with the right enum (proxy: post-call visibility==Default).
func TestSetvisDispatchDefault(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	// Start from Hard so we can observe the transition.
	p.visibility = rsbuf.VisibilityHard

	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 0")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility after ::setvis 0: got %d, want Default", p.visibility)
	}
	if !bytes.Contains(out, []byte("vis: 0")) {
		t.Errorf("MessageGame: missing 'vis: 0'; got %q", out)
	}
}

// TestSetvisDispatchSoftStub pins TS L560-562 case '1' → SOFT stub.
// SOFT path: message emitted, state unchanged.
func TestSetvisDispatchSoftStub(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	// Initial visibility=Default per newPlayer.
	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 1")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged (Default) — SOFT is a stub", p.visibility)
	}
	if !bytes.Contains(out, []byte("vis: 1 (not implemented - you are still on vis: 0)")) {
		t.Errorf("MessageGame: missing TS-faithful SOFT stub; got %q", out)
	}
}

// TestSetvisDispatchHard pins TS L563-565 case '2' → HARD.
func TestSetvisDispatchHard(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 2")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityHard {
		t.Errorf("visibility after ::setvis 2: got %d, want Hard", p.visibility)
	}
	if !bytes.Contains(out, []byte("vis: 2")) {
		t.Errorf("MessageGame: missing 'vis: 2'; got %q", out)
	}
}

// TestSetvisDispatchBadArg pins TS L566-567 default: return false — no
// state change, no message.
func TestSetvisDispatchBadArg(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 5")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged (Default)", p.visibility)
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected on bad arg; got %q", out)
	}
}

// TestSetvisDispatchNoArg pins TS L551-554 args.length < 1 → return false.
func TestSetvisDispatchNoArg(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis")
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged", p.visibility)
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected; got %q", out)
	}
}

// TestSetvisDispatchNodeProductionGate pins the && Environment.NODE_PRODUCTION
// arm selector at TS L549. NodeProduction=false → arm inert.
func TestSetvisDispatchNodeProductionGate(t *testing.T) {
	p, cc, s, _ := supermodSetup(t)
	s.cfg.NodeProduction = false
	p.visibility = rsbuf.VisibilityDefault

	received := drainConn(t, cc)
	dispatchCheat(t, p, "setvis 2") // Hard request
	p.client.flushWrite()
	out := <-received

	if p.visibility != rsbuf.VisibilityDefault {
		t.Errorf("visibility: got %d, want unchanged (NodeProduction=false → inert)", p.visibility)
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected when NodeProduction=false; got %q", out)
	}
}
