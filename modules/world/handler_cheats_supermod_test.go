package world

import (
	"bytes"
	"net"
	"testing"
	"time"

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
	pkt.PJStrLF(cheat) // payload is the cheat string only; no leading byte (TS gjstr-only)
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

// TestBanDispatchHappy pins TS L569-581 ::ban <username> <minutes>.
// Asserts recordingBridges.loginMod[0] = {method:NotifyPlayerBan,
// staff:p.username, username:<arg>, until:≈now+minutes}. Note: TS
// lowercases args (L42), so "bob" stays "bob".
func TestBanDispatchHappy(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob 30")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d calls, want 1", len(rec.loginMod))
	}
	got := rec.loginMod[0]
	if got.method != "NotifyPlayerBan" {
		t.Errorf("method: got %q, want NotifyPlayerBan", got.method)
	}
	if got.staff != "alice" {
		t.Errorf("staff: got %q, want alice (the calling moderator)", got.staff)
	}
	if got.username != "bob" {
		t.Errorf("username: got %q, want bob", got.username)
	}
	wantUntil := before.Add(30 * time.Minute)
	if diff := got.until.Sub(wantUntil); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now+30m; want within 5s", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been banned for 30 minutes.")) {
		t.Errorf("MessageGame: missing ack; got %q", out)
	}
}

// TestBanDispatchUsage pins TS L571-574: args.length < 2 → usage message,
// no bridge call.
func TestBanDispatchUsage(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d calls, want 0 on missing minutes arg", len(rec.loginMod))
	}
	if !bytes.Contains(out, []byte("Usage: ::ban <username> <minutes>")) {
		t.Errorf("MessageGame: missing usage; got %q", out)
	}
}

// TestBanDispatchUnparseableMinutes pins TS L578: tryParseInt default 60
// applied when minutes arg fails to parse.
func TestBanDispatchUnparseableMinutes(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob abc")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	wantUntil := before.Add(60 * time.Minute)
	if diff := rec.loginMod[0].until.Sub(wantUntil); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now+60m default", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been banned for 60 minutes.")) {
		t.Errorf("MessageGame: missing 60-minute ack; got %q", out)
	}
}

// TestBanDispatchNegativeClamp pins TS L578 Math.max(0, ...) — negative
// minutes clamps to 0.
func TestBanDispatchNegativeClamp(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob -5")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	// 0 minutes → until ≈ now.
	if diff := rec.loginMod[0].until.Sub(before); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now (0-min clamp)", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been banned for 0 minutes.")) {
		t.Errorf("MessageGame: missing 0-minute ack; got %q", out)
	}
}

// TestBanDispatchNodeProductionGate pins TS L569 && NODE_PRODUCTION.
// NodeProduction=false → arm inert (no bridge call, no message).
func TestBanDispatchNodeProductionGate(t *testing.T) {
	p, cc, s, rec := supermodSetup(t)
	s.cfg.NodeProduction = false
	received := drainConn(t, cc)
	dispatchCheat(t, p, "ban bob 30")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d, want 0 (NodeProduction=false)", len(rec.loginMod))
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected; got %q", out)
	}
}

// TestMuteDispatchHappy pins TS L582-594 ::mute <username> <minutes>.
func TestMuteDispatchHappy(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob 30")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d calls, want 1", len(rec.loginMod))
	}
	got := rec.loginMod[0]
	if got.method != "NotifyPlayerMute" {
		t.Errorf("method: got %q, want NotifyPlayerMute", got.method)
	}
	if got.staff != "alice" {
		t.Errorf("staff: got %q, want alice", got.staff)
	}
	if got.username != "bob" {
		t.Errorf("username: got %q, want bob", got.username)
	}
	wantUntil := before.Add(30 * time.Minute)
	if diff := got.until.Sub(wantUntil); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now+30m", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been muted for 30 minutes.")) {
		t.Errorf("MessageGame: missing ack; got %q", out)
	}
}

func TestMuteDispatchUsage(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d calls, want 0", len(rec.loginMod))
	}
	if !bytes.Contains(out, []byte("Usage: ::mute <username> <minutes>")) {
		t.Errorf("MessageGame: missing usage; got %q", out)
	}
}

func TestMuteDispatchUnparseableMinutes(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob abc")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	wantUntil := before.Add(60 * time.Minute)
	if diff := rec.loginMod[0].until.Sub(wantUntil); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now+60m default", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been muted for 60 minutes.")) {
		t.Errorf("MessageGame: missing 60-minute ack; got %q", out)
	}
}

func TestMuteDispatchNegativeClamp(t *testing.T) {
	p, cc, _, rec := supermodSetup(t)
	before := time.Now()
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob -5")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	if diff := rec.loginMod[0].until.Sub(before); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until off by %v from now (0-min clamp)", diff)
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been muted for 0 minutes.")) {
		t.Errorf("MessageGame: missing 0-minute ack; got %q", out)
	}
}

func TestMuteDispatchNodeProductionGate(t *testing.T) {
	p, cc, s, rec := supermodSetup(t)
	s.cfg.NodeProduction = false
	received := drainConn(t, cc)
	dispatchCheat(t, p, "mute bob 30")
	p.client.flushWrite()
	out := <-received

	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d, want 0", len(rec.loginMod))
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected; got %q", out)
	}
}

// kickAttachTarget wires a second Player as `targetName` into s.players
// (active=true) so s.LookupPlayerByUsername(targetName) returns it.
// Mirrors handler_reportabuse_test.go:315 reportAbuseSetupWithOnlineOffender.
func kickAttachTarget(t *testing.T, s *Server, targetName string) *Player {
	t.Helper()
	target, _ := newTestPlayer(t)
	target.client.server = s
	target.username = targetName
	target.active = true
	slot := s.players.next()
	if slot == -1 {
		t.Fatal("kickAttachTarget: world full")
	}
	target.pid = slot
	s.players.set(slot, target)
	return target
}

// TestKickDispatchHappy pins TS L605-612: lookup hit → loggingOut=true +
// ack message. DEVIATION-NAI-186-D1: TS calls inline logout()+close();
// goscape defers teardown to processLogouts. Test pins loggingOut=true
// (the precondition for processLogouts to fire next tick).
func TestKickDispatchHappy(t *testing.T) {
	p, cc, s, _ := supermodSetup(t)
	target := kickAttachTarget(t, s, "bob")

	received := drainConn(t, cc)
	dispatchCheat(t, p, "kick bob")
	p.client.flushWrite()
	out := <-received

	if !target.loggingOut {
		t.Error("target.loggingOut: must be true after ::kick (DEVIATION-NAI-186-D1: defers to processLogouts)")
	}
	if !bytes.Contains(out, []byte("Player 'bob' has been kicked from the game.")) {
		t.Errorf("MessageGame: missing ack; got %q", out)
	}
}

// TestKickDispatchLookupMiss pins TS L613-615: target not online → "does
// not exist or is not logged in" message; no state mutation elsewhere.
func TestKickDispatchLookupMiss(t *testing.T) {
	p, cc, _, _ := supermodSetup(t)
	// Do NOT call kickAttachTarget — s.players has only the caller.

	received := drainConn(t, cc)
	dispatchCheat(t, p, "kick ghost")
	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Player 'ghost' does not exist or is not logged in.")) {
		t.Errorf("MessageGame: missing lookup-miss ack; got %q", out)
	}
}

// TestKickDispatchUsage pins TS L597-600 args.length < 1 → usage message.
func TestKickDispatchUsage(t *testing.T) {
	p, cc, s, _ := supermodSetup(t)
	target := kickAttachTarget(t, s, "bob")

	received := drainConn(t, cc)
	dispatchCheat(t, p, "kick")
	p.client.flushWrite()
	out := <-received

	if target.loggingOut {
		t.Error("target.loggingOut: must remain false on empty arg")
	}
	if !bytes.Contains(out, []byte("Usage: ::kick <username>")) {
		t.Errorf("MessageGame: missing usage; got %q", out)
	}
}

// TestKickDispatchNodeProductionGate pins TS L595 && NODE_PRODUCTION.
func TestKickDispatchNodeProductionGate(t *testing.T) {
	p, cc, s, _ := supermodSetup(t)
	s.cfg.NodeProduction = false
	target := kickAttachTarget(t, s, "bob")

	received := drainConn(t, cc)
	dispatchCheat(t, p, "kick bob")
	p.client.flushWrite()
	out := <-received

	if target.loggingOut {
		t.Error("target.loggingOut: must remain false when NodeProduction=false")
	}
	if len(out) != 0 {
		t.Errorf("MessageGame: no message expected; got %q", out)
	}
}
