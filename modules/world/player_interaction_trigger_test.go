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

// newPlayerTriggerFixture sets up two players: clicker (slot=1, real
// net.Conn with ISAAC seed {5,6,7,8}) and target (slot=2, real net.Conn
// with ISAAC seed {1,2,3,4}). Distinct seeds let wire-byte assertions
// pin which side a packet was emitted on. The clicker's
// targetOp/targetSubject/target are anchored at op=1 → Player; the
// fixture leaves it to the caller to register any [opplayer1,_] script.
func newPlayerTriggerFixture(t *testing.T) (s *Server, clicker, target *Player, clickerConn, targetConn net.Conn) {
	t.Helper()
	s = newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty; caller registers

	clicker, clickerConn = newTestPlayer(t)
	clicker.client.server = s
	clicker.client.encryptor = io2.New([4]uint32{5, 6, 7, 8})
	clicker.slot = 1

	target, targetConn = newTestPlayer(t)
	target.client.server = s
	target.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	target.slot = 2

	clicker.SetInteraction(InteractionEngine, target, 1, -1)
	clicker.interacted = true // simulate reach
	return
}

// TestFireOpTriggerPlayer_BindsSelf2ToTarget pins the TS-true binding
// for the OPPLAYER trigger family (NAI-70).
//
// Registering an [opplayer1,_] script that runs HINT_PL (which dereferences
// state.Self2.Slot()) and observing the resulting HINT_ARROW packet on the
// CLICKER's wire proves:
//   - srv.runScript routed `target` (clicked player) into
//     buildPlayerScriptState's case-ActivePlayer arm at script.go:54-59,
//     which set state.Self2 = target and OR-d in PtrActivePlayer2.
//   - state.Self = clicker (`p`), since HintPlayer is dispatched on
//     state.Self's *Player and the wire packet lands on clicker's conn.
//
// The slot on the wire is the target's slot, confirming the Self2 link.
//
// Mirrors TS Player.ts:1129 + ScriptRunner.ts:84-87 (self=clicker,
// target=target → _activePlayer=clicker, _activePlayer2=target).
func TestFireOpTriggerPlayer_BindsSelf2ToTarget(t *testing.T) {
	s, clicker, target, clickerConn, _ := newPlayerTriggerFixture(t)

	// Compute expected first wire byte using a parallel encryptor seeded
	// identically to clicker.client.encryptor (NAI-70 fixture seed).
	wantEnc, _ := isaacPair([4]uint32{5, 6, 7, 8})

	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	received := drainConn(t, clickerConn)
	tryFireOpTrigger(clicker)
	clicker.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(wantEnc.GetNext())) & 0xff),
		0x0A,                                      // p1: type = 10 (player hint)
		byte(target.slot >> 8), byte(target.slot), // p2: slot (target's)
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
	_, clicker, _, _, _ := newPlayerTriggerFixture(t)

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
	_, clicker, _, _, _ := newPlayerTriggerFixture(t)
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
// when p.target is *Player, tryFireOpTrigger calls fireOpTriggerPlayer,
// not the default skip. Verified indirectly via the HINT_ARROW
// side-effect on clicker's conn (state.Self=clicker per NAI-70) — if
// the *Player arm were missing, the default arm would mark fired=true
// without invoking any script and no HINT_ARROW would arrive.
func TestTryFireOpTrigger_PlayerArm(t *testing.T) {
	s, clicker, _, clickerConn, _ := newPlayerTriggerFixture(t)
	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	received := drainConn(t, clickerConn)
	tryFireOpTrigger(clicker)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("no wire packet on clicker — Player arm did not fire")
	}
	// First byte is the encrypted HINT_ARROW opcode; we don't pin the
	// exact value here (covered by BindsSelf2ToTarget above) — just
	// confirm a packet arrived.
}

// TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom — NAI-62.
// Player-target script's Self == clicker (NAI-70 binding), so MES opcode
// emits MessageGame on clicker's conn. Pre-fix lookup uses (trigger, -1, -1);
// override registers at (trigger, K, -1) which is unreachable → no
// MessageGame on clicker's conn. Post-fix → script runs → marker appears.
func TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
	s, clicker, other, cc1, _ := makeOpPlayerFixtureWithBothConns(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const overrideTypeId = 7783
	const marker = "opplayer1-override-fired"

	clicker.target = other
	clicker.targetOp = 1
	clicker.targetSubject.com = overrideTypeId

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildPlayerMesScript(script.TriggerOpPlayer1, overrideTypeId, marker))

	received := drainConn(t, cc1)
	fireOpTriggerPlayer(clicker, s, other)
	clicker.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(marker)) {
		t.Errorf("drained bytes from clicker conn: missing %q substring; override should have run override-keyed script for targetSubject.com=%d (default Player-target lookup typeId=-1), got %x",
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

// --- B3 AP-Player variant ---

// TestFireApTriggerPlayerRestoresTargetAndWaypoints pins TS Player.ts:1145-1162
// for the AP-Player path. Since runScript's self=target (the target player),
// no p_op_player handler exists, and p_op_npc would act on target's state —
// the test pins the restore-only contract: noop script → p.target restored,
// p.nextTarget nil, waypoints restored.
//
// NAI-68 B3 AP-Player variant.
func TestFireApTriggerPlayerRestoresTargetAndWaypoints(t *testing.T) {
	s, clicker, target, _, _ := newPlayerTriggerFixture(t)

	// Pre-state: active waypoint queue (must be restored when no nextTarget).
	clicker.waypointIndex = 3
	clicker.waypoints[3] = 0x0EADBEEF

	// Register a noop [applayer1,_] script.
	s.scriptProvider.Register(&script.ScriptFile{
		Name:             "[applayer1,_]",
		LookupKey:        script.LookupKeyForGlobal(script.TriggerApPlayer1),
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	})

	s.players[target.slot] = target
	s.players[clicker.slot] = clicker

	fireApTriggerPlayer(clicker, s, target)

	// p.target restored to original target; nextTarget nil (noop script set nothing).
	if clicker.target != target {
		t.Errorf("clicker.target: got %v, want target (restored — NAI-68)", clicker.target)
	}
	if clicker.nextTarget != nil {
		t.Errorf("clicker.nextTarget: got %v, want nil (noop script set no target)", clicker.nextTarget)
	}
	// Waypoints restored (nextTarget == nil branch).
	if clicker.waypointIndex != 3 {
		t.Errorf("clicker.waypointIndex: got %d, want 3 (restored when no nextTarget)", clicker.waypointIndex)
	}
	if clicker.waypoints[3] != 0x0EADBEEF {
		t.Errorf("clicker.waypoints[3]: got 0x%X, want 0x0EADBEEF", clicker.waypoints[3])
	}
	if !clicker.interactionFired {
		t.Error("interactionFired: want true after AP-Player fire")
	}
}

// --- NAI-69 T3 (reframed): AP-Player Self2-reversed binding pin ---

// TestFireApTriggerPlayer_ApRangeCalled_BindsToTargetNotClicker pins
// the preexisting NAI-39 Self/Self2 binding divergence: APPLAYER scripts
// run with state.Self = target (the clicked player), state.Self2 = p
// (the clicker). When the script calls p_aprange, handlePApRange
// (pkg/script/handlers_player.go:695) invokes s.Self.SetApRange(n) — so
// target.apRange and target.apRangeCalled are mutated, NOT the clicker's.
//
// Consequence: the NAI-69 T1 same-tick retry guard
// `if p.nextTarget == nil && p.apRangeCalled` checks p (clicker) but
// clicker.apRangeCalled is never set via APPLAYER scripts. AP-Player
// is a structural no-op for same-tick retry until the NAI-39 binding
// is reworked (tracked: NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY).
//
// TS Player.ts:1151 has ScriptRunner.init(apTrigger, this, target) where
// `this` is the clicker uniformly across AP-Loc/Obj/Npc/Player — so TS
// AP-Player's apRangeCalled IS the clicker's. AP-Loc/AP-Obj/AP-Npc match
// TS in goscape (Self=p); AP-Player is the goscape-specific reversal.
func TestFireApTriggerPlayer_ApRangeCalled_BindsToTargetNotClicker(t *testing.T) {
	s, clicker, target, _, _ := newPlayerTriggerFixture(t)

	// Register an APPLAYER1 script that calls p_aprange(2).
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[applayer1,_]_aprange",
		LookupKey: script.LookupKeyForGlobal(script.TriggerApPlayer1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPApRange,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	})

	s.players[target.slot] = target
	s.players[clicker.slot] = clicker

	fireApTriggerPlayer(clicker, s, target)

	// Uniform-exit contract from NAI-69 T1+T2 (works for AP-Player too):
	if !clicker.interactionFired {
		t.Error("clicker.interactionFired: got false, want true (NAI-69: fire helper uniform exit)")
	}
	if clicker.target != target {
		t.Errorf("clicker.target: got %v, want target (restored after fire)", clicker.target)
	}

	// Self2-reversed-binding pin: p_aprange routed to target.SetApRange,
	// NOT clicker.SetApRange. Tracked: NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY.
	if clicker.apRangeCalled {
		t.Error("clicker.apRangeCalled: got true, want false (script ran on Self=target, not clicker)")
	}
	if clicker.apRange != 10 {
		t.Errorf("clicker.apRange: got %d, want 10 (default unchanged — script mutated target's apRange)", clicker.apRange)
	}
	if !target.apRangeCalled {
		t.Error("target.apRangeCalled: got false, want true (Self=target; SetApRange ran on target)")
	}
	if target.apRange != 2 {
		t.Errorf("target.apRange: got %d, want 2 (script set new range on Self=target)", target.apRange)
	}
}

// TestTryInteract_ApPlayer_NoSameTickRetry_DueToReversedSelf — end-to-end
// pin: tryInteract returns true (guard not tripped) because clicker's
// apRangeCalled stays false. Confirms the NAI-69 T1 guard is structurally
// inert for AP-Player under the current NAI-39 Self/Self2 binding. When
// NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY is closed, this
// test should be inverted to pin the activated retry path.
func TestTryInteract_ApPlayer_NoSameTickRetry_DueToReversedSelf(t *testing.T) {
	s, clicker, target, _, _ := newPlayerTriggerFixture(t)

	// Register the p_aprange(2) script.
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[applayer1,_]_aprange",
		LookupKey: script.LookupKeyForGlobal(script.TriggerApPlayer1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPApRange,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	})

	s.players[target.slot] = target
	s.players[clicker.slot] = clicker

	// Place clicker within AP range (5 tiles) but outside operable range (>1).
	// Default apRange=10; 5 tiles satisfies inApproachDistance but not
	// inOperableDistance — AP arm is taken, not OP arm.
	clicker.x = 3094
	clicker.z = 3106
	target.x = 3094
	target.z = 3111 // 5 tiles away on z-axis

	result := clicker.tryInteract(false)

	// Guard does NOT trigger: clicker.apRangeCalled stays false because
	// p_aprange ran on Self=target.
	if !result {
		t.Error("tryInteract: got false, want true (NAI-69 T1 guard inert for AP-Player; clicker.apRangeCalled=false)")
	}
	if !clicker.interactionFired {
		t.Error("clicker.interactionFired: got false, want true (uniform-exit preserved; no reset because guard didn't fire)")
	}
	if clicker.apRangeCalled {
		t.Error("clicker.apRangeCalled: got true, want false (Self=target divergence)")
	}
	if !target.apRangeCalled {
		t.Error("target.apRangeCalled: got false, want true (script ran on Self=target)")
	}
}
