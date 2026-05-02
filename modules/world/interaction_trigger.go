package world

import (
	"slices"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/script"
)

// tryFireOpTrigger fires the [op<entity><op>,<typeID>] trigger for the
// player's anchored target when the player has just reached interaction
// range. Matches TS Player.tryInteract() for the Engine-kind interaction.
//
// Preconditions (guaranteed by caller — Player.processInteraction):
//   - p.interacted == true
//   - p.interactionKind == InteractionEngine
//   - p.target != nil
//   - p.interactionFired == false
//
// Branches by target concrete type. Common behaviour across branches:
//   - Player became delayed between reach and dispatch: defer; leave
//     interactionFired false so we retry next tick.
//   - Lifecycle gate fail (NPC dead / Loc despawn-or-mutated): silent
//     clear interaction.
//   - targetOp out of [1,5]: silent clear.
//   - No script found (type/category/global): silent clear.
//   - Script suspends (P_DELAY/P_PAUSEBUTTON/P_COUNTDIALOG): keep
//     interaction anchored; resumeOrFinish already stored the state.
//   - Script finishes / aborts: clear interaction.
func tryFireOpTrigger(p *Player) {
	srv := p.client.server

	switch tgt := p.target.(type) {
	case *Npc:
		fireOpTriggerNpc(p, srv, tgt)
	case *entitypkg.Loc:
		fireOpTriggerLoc(p, srv, tgt)
	case *Player:
		// NAI-40 T5: Player→Player engine dispatch. Closes
		// NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER by routing
		// through srv.runScript so buildPlayerScriptState's
		// case-ActivePlayer arm sets state.Self2 = clicker.
		fireOpTriggerPlayer(p, srv, tgt)
	case *entitypkg.Obj:
		fireOpTriggerObj(p, srv, tgt)
	default:
		p.interactionFired = true
	}
}

// fireOpTriggerNpc fires the [opnpc<op>,<npcType>] trigger. Extracted
// from the original tryFireOpTrigger body verbatim.
func fireOpTriggerNpc(p *Player, srv *Server, npc *Npc) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if npc.dead {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	apTrigger, ok := apNpcTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	trigger := apTrigger + 7 // APNPC→OPNPC offset per TS Player.ts:~997
	category := 0
	if npc.typ != nil {
		category = npc.typ.Category
	}

	// Reads p.targetSubject.com per TS Player.getOpTrigger:993-995 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, npc.typeId), category)
	if sf == nil {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	// TS Player.ts:1129-1134 OP save/clear/exec/capture/restore. NAI-68.
	savedTarget := p.target
	p.target = nil
	p.waypointIndex = -1 // TS L1131

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget

	p.interactionFired = true
}

// fireOpTriggerLoc fires the [oploc<op>,<locType>] trigger.
//
// Lifecycle gate: locStillValid checks BOTH zone membership (loc still
// in zoneMap.Get(level,x,z).Locs by pointer) AND type match (loc.Type()
// == targetSubject.typ). The combined check defends against:
//   - In-place Info mutation (tree → stump via Loc.Info bitfield change)
//   - Removal from zone (loc despawned, axed, etc.)
//
// DEVIATION S6j-D2: TS handler sets targetOp=APLOC1+(op-1) and engine
// fires APLOC at approach range, OPLOC at contact. We fire OPLOC
// directly (no APLOC fallback) — inherits S6b OPNPC convention.
// Follow-up: "approach-vs-operate range gating" sub-spec.
//
// DEVIATION S6j-D4: Loc has no cached typ pointer (only the packed
// Info bitfield); category lookup goes through srv.locTypes.Configs[typeID],
// unlike *Npc which reads npc.typ.Category from a cached pointer.
func fireOpTriggerLoc(p *Player, srv *Server, loc *entitypkg.Loc) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if !locStillValid(srv, loc, p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	apTrigger, ok := apLocTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	trigger := apTrigger + 7 // APLOC→OPLOC offset per TS Player.ts:~997
	// Loc has no cached LocType pointer (only packed Info bitfield);
	// resolve category through the LocType registry. Use direct slice
	// access matching the server_configs.go pattern (LocTypeConfigs has
	// no Get method).
	category := 0
	if locId := loc.Type(); locId >= 0 && locId < len(srv.locTypes.Configs) {
		if lt := srv.locTypes.Configs[locId]; lt != nil {
			category = lt.Category
		}
	}

	// Reads p.targetSubject.com per TS Player.getOpTrigger:993-995 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, loc.Type()), category)
	if sf == nil {
		// S6j-D7 closed in S6k: defaultOp fallback. TS Player.ts:~1095
		// fires this message when the player reaches contact range and
		// no op-trigger is registered for this loc.
		p.MessageGame("Nothing interesting happens.")
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= script.PtrActiveLoc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	// TS Player.ts:1129-1134 OP save/clear/exec/capture/restore.
	// NAI-68 closes NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET.
	savedTarget := p.target
	p.target = nil
	p.waypointIndex = -1 // TS L1131 — this.clearWaypoints()

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget

	// Finished/Aborted ClearInteraction dropped — subsumed by
	// processInteraction tail's else-if at interaction.go (TS L1261-1263).
	p.interactionFired = true
}

// apLocTriggerForOp returns the APLOC trigger for the player's
// targetOp sentinel. Returns ok=false if op is neither 1..5 nor a T/U
// sentinel. fireOpTriggerLoc derives the OPLOC trigger by adding 7 to
// the returned APLOC (TS Player.ts:~997 offset convention):
//
//	APLOC1..5 (59..63) + 7 → OPLOC1..5 (66..70)
//	APLOCT    (65)     + 7 → OPLOCT    (72)
//	APLOCU    (64)     + 7 → OPLOCU    (71)
func apLocTriggerForOp(op int) (script.ServerTriggerType, bool) {
	switch {
	case op >= 1 && op <= 5:
		return script.TriggerApLoc1 + script.ServerTriggerType(op-1), true
	case op == targetOpLocT:
		return script.TriggerApLocT, true
	case op == targetOpLocU:
		return script.TriggerApLocU, true
	default:
		return 0, false
	}
}

// apNpcTriggerForOp returns the APNPC trigger for the player's
// targetOp. fireOpTriggerNpc derives the OPNPC trigger by adding 7
// (TS Player.ts:~997 offset convention):
//
//	APNPC1..5 (3..7) + 7 → OPNPC1..5 (10..14)
//	APNPCT    (9)    + 7 → OPNPCT    (16)
//	APNPCU    (8)    + 7 → OPNPCU    (15)
//
// NPC variant of apLocTriggerForOp. Parallel shape after S6o: 1..5
// ops + T/U sentinels. Returns ok=false for invalid op.
func apNpcTriggerForOp(op int) (script.ServerTriggerType, bool) {
	switch {
	case op >= 1 && op <= 5:
		return script.TriggerApNpc1 + script.ServerTriggerType(op-1), true
	case op == targetOpNpcT:
		return script.TriggerApNpcT, true
	case op == targetOpNpcU:
		return script.TriggerApNpcU, true
	default:
		return 0, false
	}
}

// locStillValid checks whether the held *Loc pointer still represents
// the same loc the player clicked. Two checks combined — both required
// because each defends against a different mutation:
//   - Zone membership: catches loc removal (e.g., axed tree).
//   - Type match: catches in-place Info mutation (e.g., tree → stump
//     via the same *Loc pointer; Loc's docstring explicitly notes
//     "callers can mutate Info in place ... without re-allocating").
func locStillValid(srv *Server, loc *entitypkg.Loc, wantType, wantX, wantZ, wantLevel int) bool {
	if loc.Type() != wantType {
		return false
	}
	zn := srv.zoneMap.Get(wantLevel, wantX, wantZ)
	return slices.Contains(zn.Locs, loc)
}

// objStillValid checks whether the held *Obj pointer still represents
// an obj present in the zone at (wantX, wantZ, wantLevel). Mirrors
// locStillValid for Obj targets. Consumed by NAI-11's validateTarget
// when an NPC's interaction target is an Obj.
func objStillValid(srv *Server, obj *entitypkg.Obj, wantX, wantZ, wantLevel int) bool {
	zn := srv.zoneMap.Get(wantLevel, wantX, wantZ)
	return slices.Contains(zn.Objs, obj)
}

// tryFireApTrigger fires the approach-trigger for the player's anchored
// target when the player has just reached apRange. Dispatches to the
// correct fire helper by concrete target type. Matches TS
// Player.ts:1139-1170 (Loc branch) and Npc.ts:~861-883 (Npc branch).
//
// Preconditions (guaranteed by caller — Player.processInteraction):
//   - p.interacted == true
//   - p.interactionKind == InteractionEngine
//   - p.target != nil
//   - p.interactionFired == false
//   - player is in approach range but NOT operable distance
func tryFireApTrigger(p *Player) {
	srv := p.client.server

	switch tgt := p.target.(type) {
	case *entitypkg.Loc:
		fireApTriggerLoc(p, srv, tgt)
	case *Npc:
		fireApTriggerNpc(p, srv, tgt)
	case *Player:
		// NAI-40 T5: Player→Player approach dispatch. Same Self2
		// substrate as the OP variant.
		fireApTriggerPlayer(p, srv, tgt)
	case *entitypkg.Obj:
		fireApTriggerObj(p, srv, tgt)
	default:
		p.interactionFired = true
	}
}

// fireApTriggerNpc fires the [apnpc<op>,<npcType>] approach-trigger
// for the player's anchored NPC target when the player has reached
// the NPC's per-type attackrange. Matches TS Npc.ts:~861-883
// (checkApTrigger).
//
// Three divergences from fireApTriggerLoc (S6l):
//
//  1. Lifecycle gate is `npc.dead` (not locStillValid). NPCs have a
//     dedicated dead flag — no zone-membership pointer-stale check
//     needed because the *Npc reference itself is authoritative.
//
//  2. Category read from npc.typ.Category directly (the cached
//     pointer). fireApTriggerLoc does a locTypes.Configs[locId]
//     lookup because Loc has no cached LocType pointer, only a
//     packed Info bitfield.
//
//  3. NO apRangeCalled persistence contract. Per TS
//     (Npc.ts:~1064-1080): NPC AP scripts complete and clear
//     interaction unconditionally. The p_aprange persistence is
//     Player-side only; NPC attackrange is fixed per-type so
//     "extend the range" has no meaning. Simpler post-fire logic.
func fireApTriggerNpc(p *Player, srv *Server, npc *Npc) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if npc.dead {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger, ok := apNpcTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	category := 0
	if npc.typ != nil {
		category = npc.typ.Category
	}

	// Reads p.targetSubject.com per TS Player.getApTrigger:1027-1029 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, npc.typeId), category)
	if sf == nil {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}

// fireApTriggerLoc fires the [aploc<op>,<locType>] trigger with the
// persistence contract: apRangeCalled=true keeps the interaction
// anchored across ticks; apRangeCalled=false clears it after a
// terminal Execution. Matches TS Player.ts:1139-1170 + :1261.
//
// Lifecycle gate: locStillValid (same helper from S6j) — catches
// in-place Info mutation and zone removal.
//
// Script lookup: TriggerApLoc1 + (op-1). No APLOC→OPLOC fallthrough
// at approach distance — OPLOC fires only when the player reaches
// contact on a later processInteraction tick.
func fireApTriggerLoc(p *Player, srv *Server, loc *entitypkg.Loc) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if !locStillValid(srv, loc, p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger, ok := apLocTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	category := 0
	if locId := loc.Type(); locId >= 0 && locId < len(srv.locTypes.Configs) {
		if lt := srv.locTypes.Configs[locId]; lt != nil {
			category = lt.Category
		}
	}

	// Reads p.targetSubject.com per TS Player.getApTrigger:1027-1029 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, loc.Type()), category)
	if sf == nil {
		// S6l-D1 closed in S6r: cache "no AP script for this (trigger,
		// locType, category) triple" via the apRange=-1 sentinel so
		// inApproachDistance short-circuits on subsequent ticks.
		// Matches TS Player.ts:~1139-1170 behavior: apRange=-1 means
		// "AP path permanently disabled for this interaction;
		// anchor stays — contact (OP) takes over on a later tick."
		p.apRange = -1
		p.interactionFired = true
		return
	}

	// Reset apRangeCalled BEFORE exec (TS Player.ts:1141). Each AP fire
	// is a fresh evaluation — script must actively call p_aprange to
	// persist the interaction.
	p.apRangeCalled = false

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= script.PtrActiveLoc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		if p.apRangeCalled {
			// Script requested a new approach range. Persist interaction
			// for next-tick re-evaluation at updated apRange.
			p.repathed = false
			// interactionFired stays false → processInteraction re-enters
			// next tick; APLOC re-fires if still in range.
			return
		}
		// apRangeCalled=false → script didn't extend range; TS line 1261
		// clears the interaction.
		p.ClearInteraction()
	}
	// Reached by: (a) Finished/Aborted + !apRangeCalled (after
	// ClearInteraction above), or (b) Suspended/P_DELAY/P_PAUSEBUTTON/
	// P_COUNTDIALOG (anchor intact, resume flow re-enters on resume tick).
	p.interactionFired = true
}

// apObjTriggerForOp returns the APOBJ trigger for p.targetOp. Returns
// ok=false for unrecognised sentinels. fireOpTriggerObj derives OPOBJ
// by adding 7 (TS Player.ts:997 offset convention):
//
//	APOBJ1..5 (31..35) + 7 → OPOBJ1..5 (38..42)
//	APOBJT    (37)     + 7 → OPOBJT    (44)
//	APOBJU    (36)     + 7 → OPOBJU    (43)
func apObjTriggerForOp(op int) (script.ServerTriggerType, bool) {
	switch {
	case op >= 1 && op <= 5:
		return script.TriggerApObj1 + script.ServerTriggerType(op-1), true
	case op == targetOpObjT:
		return script.TriggerApObjT, true
	case op == targetOpObjU:
		return script.TriggerApObjU, true
	default:
		return 0, false
	}
}

// resolveTriggerTypeId mirrors the typeId override in TS Player.getOpTrigger
// (Player.ts:993-995) and Player.getApTrigger (Player.ts:1027-1029): when
// targetSubject.com is set (≠ -1), it overrides the entity's typeId for
// trigger lookup. categoryId is NEVER overridden — the override flips only
// the type slot. Used by every player-side fire helper to thread spellCom
// (T-handlers) and useObj (OpPlayerU) into script-key resolution.
//
// Storage convention: SetInteraction canonicalises com=0 → -1 (matching
// TS PathingEntity.ts:520 truthy), so the != -1 check here behaves
// identically to TS !== -1 even at the com=0 boundary.
func resolveTriggerTypeId(p *Player, defaultTypeId int) int {
	if p.targetSubject.com != -1 {
		return p.targetSubject.com
	}
	return defaultTypeId
}

// fireOpTriggerObj fires the [opobj<op>,<objType>] trigger for the player's
// anchored Obj target when the player has reached operable distance.
// Mirrors fireOpTriggerLoc with three substitutions:
//  1. Lifecycle gate: objStillValid (zone-membership check).
//  2. ScriptState: ActiveObj + PtrActiveObj.
//  3. No-script fallback: "Nothing interesting happens." (TS Player.ts:1095).
func fireOpTriggerObj(p *Player, srv *Server, obj *entitypkg.Obj) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if !objStillValid(srv, obj, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	apTrigger, ok := apObjTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	trigger := apTrigger + 7 // APOBJ→OPOBJ offset per TS Player.ts:997

	category := 0
	if obj.Type >= 0 && obj.Type < len(srv.objTypes.Configs) {
		if ot := srv.objTypes.Configs[obj.Type]; ot != nil {
			category = ot.Category
		}
	}

	// Reads p.targetSubject.com per TS Player.getOpTrigger:993-995 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, obj.Type), category)
	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveObj = obj
	state.Pointers |= script.PtrActiveObj
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	// TS Player.ts:1129-1134 OP save/clear/exec/capture/restore. NAI-68.
	savedTarget := p.target
	p.target = nil
	p.waypointIndex = -1 // TS L1131

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget

	p.interactionFired = true
}

// fireApTriggerObj fires the [apobj<op>,<objType>] approach-trigger for the
// player's anchored Obj target. Mirrors fireApTriggerLoc with three
// substitutions:
//  1. Lifecycle gate: objStillValid.
//  2. ScriptState: ActiveObj + PtrActiveObj.
//  3. No-script path: apRange=-1 sentinel (OP trigger takes over on contact).
func fireApTriggerObj(p *Player, srv *Server, obj *entitypkg.Obj) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if !objStillValid(srv, obj, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger, ok := apObjTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	category := 0
	if obj.Type >= 0 && obj.Type < len(srv.objTypes.Configs) {
		if ot := srv.objTypes.Configs[obj.Type]; ot != nil {
			category = ot.Category
		}
	}

	// Reads p.targetSubject.com per TS Player.getApTrigger:1027-1029 via
	// resolveTriggerTypeId — spellCom override defaultTypeId when set.
	sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, obj.Type), category)
	if sf == nil {
		p.apRange = -1
		p.interactionFired = true
		return
	}

	p.apRangeCalled = false

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveObj = obj
	state.Pointers |= script.PtrActiveObj
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		if p.apRangeCalled {
			p.repathed = false
			return
		}
		p.ClearInteraction()
	}
	p.interactionFired = true
}
