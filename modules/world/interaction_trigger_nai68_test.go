package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// buildPOpNpcScript creates a *script.ScriptFile keyed at (trigger, typeID)
// whose body is [push op, P_OPNPC, RETURN]. The executing ScriptState
// must have ActiveNpc pre-bound (done by fireOpTriggerNpc's state setup).
// op must be in [1,5].
func buildPOpNpcScript(trigger script.ServerTriggerType, typeID int, op int) *script.ScriptFile {
	key := script.LookupKeyForType(trigger, typeID)
	return &script.ScriptFile{
		Name:             "[opnpc,p_op_npc]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpPOpNpc, script.OpReturn},
		IntOperands:      []int32{int32(op), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
}

// buildPOpLocScript creates a *script.ScriptFile keyed at (trigger, typeID)
// whose body is [push op, P_OPLOC, RETURN]. The executing ScriptState
// must have ActiveLoc pre-bound (done by fireOpTriggerLoc's state setup).
// op must be in [1,5].
func buildPOpLocScript(trigger script.ServerTriggerType, typeID int, op int) *script.ScriptFile {
	key := script.LookupKeyForType(trigger, typeID)
	return &script.ScriptFile{
		Name:             "[oploc,p_op_loc]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpPOpLoc, script.OpReturn},
		IntOperands:      []int32{int32(op), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
}

// --- B3 OP-Loc variant ---

// TestFireOpTriggerLocCapturesNextTargetFromScript pins TS Player.ts:1129-1134.
// An OP-Loc trigger script that calls p_op_loc mid-execution must have
// the new target captured into p.nextTarget; p.target must be restored to the
// original loc post-script (the tail's pop applies the nextTarget on the next
// tick).
//
// NAI-68 B3 OP-Loc variant.
func TestFireOpTriggerLocCapturesNextTargetFromScript(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)

	// Register an [oploc1, locType] script whose body calls p_op_loc(loc, 2).
	// state.ActiveLoc is set to loc by fireOpTriggerLoc, so p_op_loc reads it.
	s.scriptProvider.Register(buildPOpLocScript(script.TriggerOpLoc1, loc.Type(), 2))

	fireOpTriggerLoc(p, s, loc)

	// Tail HAS NOT run (testing fire helper in isolation).
	// Expected: nextTarget captured (loc, from p_op_loc), target restored to original loc.
	if p.nextTarget == nil {
		t.Errorf("p.nextTarget: got nil, want loc (script-set target captured via p_op_loc)")
	}
	if p.nextTarget != loc {
		t.Errorf("p.nextTarget: got %v, want loc (script called p_op_loc(loc, 2))", p.nextTarget)
	}
	if p.target != loc {
		t.Errorf("p.target: got %v, want loc (restored after capture)", p.target)
	}
}

// TestFireOpTriggerLocClearsWaypoints pins TS Player.ts:1131. Every
// OP fire MUST clear waypoints before script execution, regardless
// of whether the script sets a nextTarget.
//
// NAI-68 B5 dual-pin: counterpart with no script-side p_op_* runs in
// TestFireOpTriggerLocClearsWaypointsNoNextTarget below.
func TestFireOpTriggerLocClearsWaypoints(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)

	// Pre-state: active waypoint queue.
	p.waypointIndex = 3
	p.waypoints[3] = 0x0EADBEEF

	s.scriptProvider.Register(buildNpcSayScript(script.TriggerOpLoc1, loc.Type(), "any-msg-fixture"))

	fireOpTriggerLoc(p, s, loc)

	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1131)", p.waypointIndex)
	}
}

// TestFireOpTriggerLocClearsWaypointsNoNextTarget — counter-pin: same
// clear semantics even when no nextTarget is set.
//
// NAI-68 B5 absence-pin.
func TestFireOpTriggerLocClearsWaypointsNoNextTarget(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)

	p.waypointIndex = 3
	p.waypoints[3] = 0x0EADBEEF

	// Register a no-op script so we hit the script-execution path
	// without any p_op_* setting a nextTarget.
	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerOpLoc1, loc.Type(), -1))

	fireOpTriggerLoc(p, s, loc)

	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1131 — fires regardless of nextTarget)", p.waypointIndex)
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil (no-op script didn't set nextTarget)", p.nextTarget)
	}
}

// --- B3 OP-Npc variant ---

// TestFireOpTriggerNpcCapturesNextTargetFromScript pins TS Player.ts:1129-1134
// for the NPC OP-fire path.
//
// NAI-68 B3 OP-Npc variant.
func TestFireOpTriggerNpcCapturesNextTargetFromScript(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.interacted = true

	// Register [opnpc1, typeID=7] script that calls p_op_npc(npc, 2).
	// state.ActiveNpc = npc is set by fireOpTriggerNpc, so p_op_npc reads it.
	s.scriptProvider.Register(buildPOpNpcScript(script.TriggerOpNpc1, npc.typeId, 2))

	fireOpTriggerNpc(p, s, npc)

	// nextTarget captured from p_op_npc's SetInteraction call.
	// target restored to savedTarget (npc, same pointer).
	if p.nextTarget == nil {
		t.Errorf("p.nextTarget: got nil, want npc (script-set target captured via p_op_npc)")
	}
	if p.nextTarget != npc {
		t.Errorf("p.nextTarget: got %v, want npc", p.nextTarget)
	}
	if p.target != npc {
		t.Errorf("p.target: got %v, want npc (restored after capture)", p.target)
	}
}

// TestFireOpTriggerNpcClearsWaypoints pins TS Player.ts:1131 for the NPC path.
//
// NAI-68 B5 OP-Npc.
func TestFireOpTriggerNpcClearsWaypoints(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.interacted = true

	p.waypointIndex = 5
	p.waypoints[5] = 0x0EADBEEF

	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerOpNpc1, npc.typeId, -1))

	fireOpTriggerNpc(p, s, npc)

	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1131 NPC OP-fire path)", p.waypointIndex)
	}
}

// --- B3 OP-Obj variant ---

// TestFireOpTriggerObjCapturesNextTargetFromScript pins TS Player.ts:1129-1134
// for the Obj OP-fire path. Since no p_op_obj handler is registered, we
// verify the restore-only contract: a noop script does not set nextTarget, and
// p.target is restored to the original obj.
//
// NAI-68 B3 OP-Obj variant.
func TestFireOpTriggerObjCapturesNextTargetFromScript(t *testing.T) {
	s, p, obj, _ := makeOpObjTriggerFixture(t)

	// Noop script — no p_op_obj handler exists yet. Test that target is
	// preserved (restored from savedTarget) and nextTarget is nil (no capture).
	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerOpObj1, obj.Type, -1))

	fireOpTriggerObj(p, s, obj)

	if p.target != obj {
		t.Errorf("p.target: got %v, want obj (restored after OP-Obj fire)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil (noop script set no nextTarget)", p.nextTarget)
	}
}

// TestFireOpTriggerObjClearsWaypoints pins TS Player.ts:1131 for the Obj path.
//
// NAI-68 B5 OP-Obj.
func TestFireOpTriggerObjClearsWaypoints(t *testing.T) {
	s, p, obj, _ := makeOpObjTriggerFixture(t)

	p.waypointIndex = 2
	p.waypoints[2] = 0x0EADBEEF

	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerOpObj1, obj.Type, -1))

	fireOpTriggerObj(p, s, obj)

	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1131 Obj OP-fire path)", p.waypointIndex)
	}
}

// --- B3 OP-Player variant ---

// TestFireOpTriggerPlayerCapturesNextTargetFromScript pins TS Player.ts:1129-1134
// for the Player OP-fire path. Since no p_op_player handler is registered,
// we verify the restore-only contract: a noop script does not set nextTarget,
// and p.target is restored to the original player target.
//
// NAI-68 B3 OP-Player variant.
func TestFireOpTriggerPlayerCapturesNextTargetFromScript(t *testing.T) {
	s, clicker, target, _, _ := newPlayerTriggerFixture(t)

	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	// Make target visible to rsbuf so the HINT_PL dispatch doesn't fail.
	s.players[target.slot] = target
	s.rsbuf.AddPlayer(int32(target.slot))
	s.players[clicker.slot] = clicker
	s.rsbuf.AddPlayer(int32(clicker.slot))

	fireOpTriggerPlayer(clicker, s, target)

	// target restored from savedTarget.
	if clicker.target != target {
		t.Errorf("clicker.target: got %v, want target (restored after OP-Player fire)", clicker.target)
	}
	// nextTarget: HINT_PL doesn't call p_op_player, so no capture expected.
	if clicker.nextTarget != nil {
		t.Errorf("clicker.nextTarget: got %v, want nil (no p_op_player in script)", clicker.nextTarget)
	}
}

// TestFireOpTriggerPlayerClearsWaypoints pins TS Player.ts:1131 for the Player path.
//
// NAI-68 B5 OP-Player.
func TestFireOpTriggerPlayerClearsWaypoints(t *testing.T) {
	s, clicker, target, _, _ := newPlayerTriggerFixture(t)

	clicker.waypointIndex = 4
	clicker.waypoints[4] = 0x0EADBEEF

	s.players[target.slot] = target
	s.rsbuf.AddPlayer(int32(target.slot))
	s.players[clicker.slot] = clicker
	s.rsbuf.AddPlayer(int32(clicker.slot))

	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	fireOpTriggerPlayer(clicker, s, target)

	if clicker.waypointIndex != -1 {
		t.Errorf("clicker.waypointIndex: got %d, want -1 (TS L1131 Player OP-fire path)", clicker.waypointIndex)
	}
}

// --- B3 AP-Loc variant + B6 dual-pin ---

// TestFireApTriggerLocCapturesNextTargetFromScript pins TS Player.ts:1145-1162.
// An AP-Loc trigger script that calls p_op_loc mid-execution must:
//   - capture new target into p.nextTarget,
//   - restore p.target to the original loc,
//   - clear waypoints (TS L1162 — nextTarget != nil branch).
//
// NAI-68 B3 AP-Loc variant + B6 nextTarget-set sub-pin.
func TestFireApTriggerLocCapturesNextTargetFromScript(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Pre-state: active waypoint queue (must be cleared per TS L1162).
	p.waypointIndex = 3
	p.waypoints[3] = 0x0EADBEEF

	// p_op_loc calls SetInteraction(p, state.ActiveLoc, op, -1).
	// fireApTriggerLoc sets state.ActiveLoc = loc, so p.target is set to loc again.
	// nextTarget = loc (non-nil) → waypoints clear branch.
	s.scriptProvider.Register(buildPOpLocScript(script.TriggerApLoc1, loc.Type(), 2))

	fireApTriggerLoc(p, s, loc)

	if p.nextTarget != loc {
		t.Errorf("p.nextTarget: got %v, want loc (script called p_op_loc(loc, 2))", p.nextTarget)
	}
	if p.target != loc {
		t.Errorf("p.target: got %v, want loc (restored)", p.target)
	}
	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1162 nextTarget != nil clears)", p.waypointIndex)
	}
}

// TestFireApTriggerLocClearsWaypointsOnAttackPath pins TS Player.ts:1158-1168.
// When the AP script sets NO nextTarget AND does NOT call p_aprange (the
// "attack path" — the script acted at range and is done), waypoints stay
// CLEARED from the pre-exec clearWaypoints (TS L1149). TS restores waypoints
// ONLY in the `else if (this.apRangeCalled)` branch (L1163-1167); the attack
// path falls through to `return true` with waypoints cleared.
//
// Pre-fix Go restored waypoints whenever nextTarget was nil, conflating the
// apRangeCalled (step-closer) branch with the attack branch. That left a
// ranged/magic attacker carrying its pre-attack path-to-melee, so the player
// walked up to the target after each shot instead of holding range.
func TestFireApTriggerLocClearsWaypointsOnAttackPath(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Pre-state: active waypoint queue (the path toward the target).
	p.waypointIndex = 3
	p.waypoints[3] = 0x0EADBEEF
	p.waypoints[2] = 0x0AFEBABE

	// No-op AP script: no p_op_*, no p_aprange (attack path).
	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerApLoc1, loc.Type(), -1))

	fireApTriggerLoc(p, s, loc)

	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1158-1168 — attack path leaves waypoints cleared)", p.waypointIndex)
	}
	if p.apRangeCalled {
		t.Errorf("p.apRangeCalled: got true, want false (no-op script)")
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil", p.nextTarget)
	}
}

// TestFireApTriggerLocRestoresWaypointsWhenApRangeCalled pins the surviving
// restore branch (TS L1163-1167): when the AP script calls p_aprange (the
// step-closer path), the pre-exec waypoint clear is reverted so the player
// keeps walking toward the new range.
func TestFireApTriggerLocRestoresWaypointsWhenApRangeCalled(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	p.waypointIndex = 3
	p.waypoints[3] = 0x0EADBEEF
	p.waypoints[2] = 0x0AFEBABE

	// AP script calls p_aprange(2) — step-closer, no attack this tick.
	s.scriptProvider.Register(scriptFileWithApRangeCall(t, script.TriggerApLoc1, loc.Type(), 2))

	fireApTriggerLoc(p, s, loc)

	if !p.apRangeCalled {
		t.Fatal("p.apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.waypointIndex != 3 {
		t.Errorf("p.waypointIndex: got %d, want 3 (TS L1163-1167 — apRangeCalled restores path)", p.waypointIndex)
	}
	if p.waypoints[3] != 0x0EADBEEF {
		t.Errorf("p.waypoints[3]: got 0x%X, want 0x0EADBEEF", p.waypoints[3])
	}
	if p.waypoints[2] != 0x0AFEBABE {
		t.Errorf("p.waypoints[2]: got 0x%X, want 0x0AFEBABE", p.waypoints[2])
	}
}

// --- B3 AP-Npc variant ---

// TestFireApTriggerNpcCapturesNextTargetFromScript pins TS Player.ts:1145-1162
// for the AP-Npc path. A script that calls p_op_npc re-anchors on ActiveNpc
// (same npc), setting p.target during execution. Post-fire:
//   - p.nextTarget = npc (captured),
//   - p.target = npc (restored from savedTarget),
//   - p.waypointIndex = -1 (nextTarget != nil → clear branch).
//
// NAI-68 B3 AP-Npc variant.
func TestFireApTriggerNpcCapturesNextTargetFromScript(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	// Pre-state: active waypoint queue (must be cleared when nextTarget set).
	p.waypointIndex = 5
	p.waypoints[5] = 0x0EADBEEF

	// p_op_npc reads state.ActiveNpc (= npc) and calls SetInteraction(p, npc, 2, -1).
	// After exec: p.target = npc; capture: p.nextTarget = npc.
	s.scriptProvider.Register(buildPOpNpcScript(script.TriggerApNpc1, npc.typeId, 2))

	fireApTriggerNpc(p, s, npc)

	if p.nextTarget != npc {
		t.Errorf("p.nextTarget: got %v, want npc (captured from p_op_npc)", p.nextTarget)
	}
	if p.target != npc {
		t.Errorf("p.target: got %v, want npc (restored from savedTarget)", p.target)
	}
	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (nextTarget != nil clears)", p.waypointIndex)
	}
}

// TestFireApTriggerNpcClearsWaypointsOnAttackPath pins TS Player.ts:1158-1168
// for AP-Npc — the path the live ranged/magic combat bug took. A no-op AP
// script (no p_op_*, no p_aprange) is the "attack happened at range" case:
// waypoints stay cleared so the player holds position. This is the
// player_combat_start_ap attack branch (npc_range(coord) <= attackrange),
// where the combat script attacks and does NOT call p_aprange.
//
// Pre-fix Go restored the pre-attack path here, so a ranged/magic attacker
// kept its path-to-melee and walked all the way up to the NPC after firing —
// the user-reported "I run up to the NPC with ranged/magic" symptom. Melee
// was unaffected because melee attacks via the OP branch at contact, not via
// this AP branch.
func TestFireApTriggerNpcClearsWaypointsOnAttackPath(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	// Pre-state: a queued path toward the NPC (as pathToTarget would build).
	p.waypointIndex = 4
	p.waypoints[4] = 0x0EADBEEF

	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerApNpc1, npc.typeId, -1))

	fireApTriggerNpc(p, s, npc)

	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1158-1168 — attack path leaves waypoints cleared; pre-fix restored them and the player walked up)", p.waypointIndex)
	}
	if p.apRangeCalled {
		t.Errorf("p.apRangeCalled: got true, want false (no-op script)")
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil (noop script set no target)", p.nextTarget)
	}
}

// TestFireApTriggerNpcRestoresWaypointsWhenApRangeCalled pins the surviving
// restore branch (TS L1163-1167) for AP-Npc: a script that calls p_aprange
// (e.g. melee's p_aprange(1) or ranged's step-closer p_aprange(N)) keeps its
// path so the player walks to the new range. Guards the fix from
// over-correcting (melee/step-closer must still path in).
func TestFireApTriggerNpcRestoresWaypointsWhenApRangeCalled(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	p.waypointIndex = 4
	p.waypoints[4] = 0x0EADBEEF

	s.scriptProvider.Register(scriptFileWithApRangeCall(t, script.TriggerApNpc1, npc.typeId, 2))

	fireApTriggerNpc(p, s, npc)

	if !p.apRangeCalled {
		t.Fatal("p.apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.waypointIndex != 4 {
		t.Errorf("p.waypointIndex: got %d, want 4 (TS L1163-1167 — apRangeCalled restores path)", p.waypointIndex)
	}
	if p.waypoints[4] != 0x0EADBEEF {
		t.Errorf("p.waypoints[4]: got 0x%X, want 0x0EADBEEF", p.waypoints[4])
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil (no p_op_*)", p.nextTarget)
	}
}

// --- B3 AP-Obj variant ---

// TestFireApTriggerObjCapturesNextTargetFromScript pins TS Player.ts:1145-1162
// for the AP-Obj path. No p_op_obj handler exists yet; test pins the
// restore-only contract: noop script → p.target restored to obj, p.nextTarget nil.
//
// NAI-68 B3 AP-Obj variant.
func TestFireApTriggerObjCapturesNextTargetFromScript(t *testing.T) {
	s, p, obj, _ := makeApObjTriggerFixture(t)

	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerApObj1, obj.Type, -1))

	fireApTriggerObj(p, s, obj)

	if p.target != obj {
		t.Errorf("p.target: got %v, want obj (restored after AP-Obj fire)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil (noop script set no target)", p.nextTarget)
	}
}

// TestFireApTriggerObjClearsWaypointsOnAttackPath pins TS Player.ts:1158-1168
// for AP-Obj: no nextTarget + no p_aprange (attack path) → waypoints cleared.
func TestFireApTriggerObjClearsWaypointsOnAttackPath(t *testing.T) {
	s, p, obj, _ := makeApObjTriggerFixture(t)

	p.waypointIndex = 2
	p.waypoints[2] = 0x0EADBEEF

	s.scriptProvider.Register(newNoopScriptFile(t, script.TriggerApObj1, obj.Type, -1))

	fireApTriggerObj(p, s, obj)

	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1158-1168 — attack path leaves waypoints cleared)", p.waypointIndex)
	}
	if p.apRangeCalled {
		t.Errorf("p.apRangeCalled: got true, want false (no-op script)")
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil", p.nextTarget)
	}
}

// TestFireApTriggerObjRestoresWaypointsWhenApRangeCalled pins the surviving
// restore branch (TS L1163-1167) for AP-Obj: p_aprange → path restored.
func TestFireApTriggerObjRestoresWaypointsWhenApRangeCalled(t *testing.T) {
	s, p, obj, _ := makeApObjTriggerFixture(t)

	p.waypointIndex = 2
	p.waypoints[2] = 0x0EADBEEF

	s.scriptProvider.Register(scriptFileWithApRangeCall(t, script.TriggerApObj1, obj.Type, 2))

	fireApTriggerObj(p, s, obj)

	if !p.apRangeCalled {
		t.Fatal("p.apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.waypointIndex != 2 {
		t.Errorf("p.waypointIndex: got %d, want 2 (TS L1163-1167 — apRangeCalled restores path)", p.waypointIndex)
	}
	if p.waypoints[2] != 0x0EADBEEF {
		t.Errorf("p.waypoints[2]: got 0x%X, want 0x0EADBEEF", p.waypoints[2])
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil", p.nextTarget)
	}
}
