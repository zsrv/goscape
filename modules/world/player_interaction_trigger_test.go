package world

import (
	"bytes"
	"net"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/script"
)

// TestApPlayerTriggerForOp pins the op→APPLAYER trigger map.
//
// fireOpTriggerPlayer derives OPPLAYER by adding 7 (TS Player.ts:~997
// offset), so the constants here are the AP-side; the +7 derivation is
// covered by the trigger-constant invariant rather than by this map.
func TestApPlayerTriggerForOp(t *testing.T) {
	cases := []struct {
		op     int
		want   script.ServerTriggerType
		wantOk bool
	}{
		{1, script.TriggerApPlayer1, true},
		{2, script.TriggerApPlayer2, true},
		{3, script.TriggerApPlayer3, true},
		{4, script.TriggerApPlayer4, true},
		{targetOpPlayerT, script.TriggerApPlayerT, true},
		{targetOpPlayerU, script.TriggerApPlayerU, true},
		{0, 0, false},
		{5, 0, false}, // no OPPLAYER5 wire packet from the client
		{-1, 0, false},
	}
	for _, c := range cases {
		got, ok := apPlayerTriggerForOp(c.op)
		if ok != c.wantOk {
			t.Errorf("apPlayerTriggerForOp(%d): ok=%v, want %v", c.op, ok, c.wantOk)
		}
		if got != c.want {
			t.Errorf("apPlayerTriggerForOp(%d): trigger=%d, want %d", c.op, got, c.want)
		}
	}
}

// buildOpPlayerHintPlScript produces a tiny `[opplayer<op>,_]` script
// whose body is [HINT_PL, RETURN]. HINT_PL pulls Self2.Slot() directly
// out of ScriptState, so no operand stack push is needed.
//
// trigger should be the OPPLAYER trigger (TriggerOpPlayer1..4 / T / U).
func buildOpPlayerHintPlScript(trigger script.ServerTriggerType) *script.ScriptFile {
	return &script.ScriptFile{
		Name:             "[opplayer1,_]",
		LookupKey:        script.LookupKeyForGlobal(trigger),
		Opcodes:          []script.Opcode{script.OpHintPl, script.OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
}

// newPlayerTriggerFixture sets up two players: clicker (slot=1, no
// drainable conn) and target (slot=2, real net.Conn so we can observe
// outbound HINT_ARROW packets via drainConn). The clicker's
// targetOp/targetSubject/target are anchored at op=1 → Player; the
// fixture leaves it to the caller to register any [opplayer1,_] script.
func newPlayerTriggerFixture(t *testing.T) (s *Server, clicker, target *Player, targetConn net.Conn) {
	t.Helper()
	s = newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty; caller registers

	clicker, _ = newTestPlayer(t)
	clicker.client.server = s
	clicker.slot = 1

	target, targetConn = newTestPlayer(t)
	target.client.server = s
	target.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	target.slot = 2

	clicker.SetInteraction(InteractionEngine, target, 1, -1)
	clicker.interacted = true // simulate reach
	return
}

// TestFireOpTriggerPlayer_BindsSelf2ToClicker is the
// deviation-closure pin for NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER.
//
// Registering an [opplayer1,_] script that runs HINT_PL (which dereferences
// state.Self2.Slot()) and observing the resulting HINT_ARROW packet on the
// target's wire proves:
//   - srv.runScript routed `p` (clicker) into buildPlayerScriptState's
//     case-ActivePlayer arm at script.go:54-58, which set
//     state.Self2 = clicker and OR-d in PtrActivePlayer2 (otherwise the
//     requireActivePlayer2 guard inside handleHintPl would have errored
//     and no wire packet would be emitted).
//   - state.Self = target, since HintPlayer is dispatched on target's
//     *Player and the wire packet lands on target's connection (not the
//     clicker's).
//
// The slot on the wire is the clicker's slot, confirming the Self2 link.
func TestFireOpTriggerPlayer_BindsSelf2ToClicker(t *testing.T) {
	s, clicker, target, targetConn := newPlayerTriggerFixture(t)

	// Compute expected first wire byte using a parallel encryptor seeded
	// identically to target.client.encryptor.
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	received := drainConn(t, targetConn)
	tryFireOpTrigger(clicker)
	target.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(wantEnc.GetNext())) & 0xff),
		0x0A,                                        // p1: type = 10 (player hint)
		byte(clicker.slot >> 8), byte(clicker.slot), // p2: slot
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}
	if !bytes.Equal(got, want) {
		t.Errorf("HINT_ARROW wire bytes: got %#x, want %#x", got, want)
	}
	if !clicker.interactionFired {
		t.Error("interactionFired: got false, want true after fire")
	}
}

// TestFireOpTriggerPlayer_NoScriptRegistered — empty provider + Player
// target → silent clear (no panic, target nil-ed, fired=true).
func TestFireOpTriggerPlayer_NoScriptRegistered(t *testing.T) {
	_, clicker, _, _ := newPlayerTriggerFixture(t)

	tryFireOpTrigger(clicker)

	if clicker.target != nil {
		t.Errorf("target: expected cleared on no-script-found; got %v", clicker.target)
	}
	if !clicker.interactionFired {
		t.Error("interactionFired: got false, want true")
	}
}

// TestFireApTriggerPlayer_NoScriptSetsApRangeMinusOne — empty provider +
// Player target on the AP path → apRange=-1 sentinel (matches Loc behavior
// at S6r). interaction NOT cleared (anchor stays — contact path takes
// over on a later tick).
func TestFireApTriggerPlayer_NoScriptSetsApRangeMinusOne(t *testing.T) {
	_, clicker, _, _ := newPlayerTriggerFixture(t)
	clicker.apRange = 10

	tryFireApTrigger(clicker)

	if clicker.apRange != -1 {
		t.Errorf("apRange: got %d, want -1 (no-script-found marker)", clicker.apRange)
	}
	if clicker.target == nil {
		t.Error("target: expected anchor preserved on AP no-script; got nil")
	}
}

// TestTryFireOpTrigger_PlayerArm pins the type-switch dispatch arm:
// when p.target is *Player, tryFireOpTrigger calls fireOpTriggerPlayer
// (the new T5 arm), not the default skip. Verified indirectly via the
// HINT_ARROW side-effect — if the *Player arm were missing, the default
// arm would mark fired=true without invoking any script and no
// HINT_ARROW would arrive.
func TestTryFireOpTrigger_PlayerArm(t *testing.T) {
	s, clicker, target, targetConn := newPlayerTriggerFixture(t)
	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	received := drainConn(t, targetConn)
	tryFireOpTrigger(clicker)
	target.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("no wire packet on target — Player arm did not fire")
	}
	// First byte is the encrypted HINT_ARROW opcode; we don't pin the
	// exact value here (covered by BindsSelf2 above) — just confirm a
	// packet arrived at all.
}

// TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Player-target script's Self == target, so MES opcode emits MessageGame
// on target's conn. Pre-fix lookup uses (trigger, -1, -1); override
// registers at (trigger, K, -1) which is unreachable → no MessageGame
// on target's conn. Post-fix → script runs → marker appears.
func TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, clicker, other, _, cc2 := makeOpPlayerFixtureWithBothConns(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const overrideTypeId = 7783
	const marker = "opplayer1-override-fired"

	clicker.target = other
	clicker.targetOp = 1
	clicker.targetSubject.com = overrideTypeId

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildPlayerMesScript(script.TriggerOpPlayer1, overrideTypeId, marker))

	received := drainConn(t, cc2)
	fireOpTriggerPlayer(clicker, s, other)
	other.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(marker)) {
		t.Errorf("drained bytes from target conn: missing %q substring; override should have run override-keyed script for targetSubject.com=%d (default Player-target lookup typeId=-1), got %x",
			marker, overrideTypeId, got)
	}
}

// TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Same OpMes marker strategy as Task 2.7; AP variant. Also asserts
// p.apRange != -1 as a secondary signal (no-script path sets apRange = -1
// per fireApTriggerPlayer:88).
func TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, clicker, other, _, cc2 := makeOpPlayerFixtureWithBothConns(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const overrideTypeId = 7784
	const marker = "applayer1-override-fired"

	clicker.target = other
	clicker.targetOp = 1
	clicker.targetSubject.com = overrideTypeId

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildPlayerMesScript(script.TriggerApPlayer1, overrideTypeId, marker))

	received := drainConn(t, cc2)
	fireApTriggerPlayer(clicker, s, other)
	other.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(marker)) {
		t.Errorf("drained bytes from target conn: missing %q substring; override should have run override-keyed script for targetSubject.com=%d, got %x",
			marker, overrideTypeId, got)
	}
	if clicker.apRange == -1 {
		t.Errorf("apRange: got -1 (no-script sentinel), want >0; override should have prevented the no-script path")
	}
}
