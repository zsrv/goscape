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
		// case-ActivePlayer arm sets state.Self2 = target (NAI-70).
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
	state.LocOps = srv.locOps
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
//   - In-place Info mutation (tree → stump via Loc.CurrentInfo bitfield change)
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
	state.LocOps = srv.locOps
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
//  3. apRangeCalled mechanism is structurally active per the uniform
//     TS Player.ts:1139-1170 AP block, but behaviorally a no-op for
//     NPC targets. effectiveApRange (interaction.go:393) reads
//     npc.typ.AttackRange (fixed per-type), not p.apRange — so a
//     script calling p_aprange against an NPC target sets
//     p.apRangeCalled=true but doesn't change the in-range check on
//     post-step retry. NAI-69 preserves this preexisting goscape
//     divergence. Closure: future "AP-Npc effectiveApRange parity"
//     audit if upstream TS NPC AP behavior changes.
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

	// Reset apRangeCalled BEFORE exec (TS Player.ts:1141). Each AP fire
	// is a fresh evaluation — script must actively call p_aprange to
	// persist the interaction.
	p.apRangeCalled = false

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.LocOps = srv.locOps
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore. NAI-68.
	// AP-Npc has no apRangeCalled persistence (NPC attackrange is fixed
	// per-type per pre-existing doc-comment at fireApTriggerNpc:293-297).
	savedTarget := p.target
	savedWP := p.waypoints
	savedIdx := p.waypointIndex
	p.target = nil
	p.waypointIndex = -1

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget
	if p.nextTarget != nil {
		p.waypointIndex = -1
	} else {
		p.waypoints = savedWP
		p.waypointIndex = savedIdx
	}

	// Finished/Aborted ClearInteraction dropped — subsumed by
	// processInteraction tail's else-if (TS L1261-1263).
	p.interactionFired = true
}

// fireApTriggerLoc fires the [aploc<op>,<locType>] trigger. Matches
// TS Player.ts:1139-1170. Always sets interactionFired=true at exit;
// the same-tick retry signal is apRangeCalled (set by p_aprange via
// the ActivePlayer.SetApRange interface). tryInteract owns the
// retry-vs-pop decision (see interaction.go AP branch).
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
	state.LocOps = srv.locOps
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore +
	// nextTarget-conditional waypoint clear. NAI-68.
	savedTarget := p.target
	savedWP := p.waypoints
	savedIdx := p.waypointIndex
	p.target = nil
	p.waypointIndex = -1

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget
	if p.nextTarget != nil {
		// TS L1162: clear destination so next-tick interaction starts fresh.
		p.waypointIndex = -1
	} else {
		// No script-set target — restore waypoints (TS L1146 inverse).
		p.waypoints = savedWP
		p.waypointIndex = savedIdx
	}

	// TS L1163-1167 same-tick AP retry: when state.Execution is
	// Finished/Aborted AND apRangeCalled is true, tryInteract sees the
	// flag, restores interactionFired=false, and returns false so
	// processInteraction's walk-arm runs and post-step tryInteract
	// re-fires AP with the new range. Suspended scripts (P_DELAY /
	// P_PAUSEBUTTON / P_COUNTDIALOG) leave apRangeCalled false and
	// keep the anchor across ticks via the suspended ScriptState. NAI-69
	// closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.
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

// apTriggerForTarget dispatches to the per-entity-type apXxxTriggerForOp
// helper. Returns ok=false when targetOp is unsupported for the target's
// concrete type or when the concrete type is unrecognised. Callers gate
// nil-target externally. Internal — used by getOpTrigger and getApTrigger
// to share the type-switch.
func apTriggerForTarget(p *Player) (script.ServerTriggerType, bool) {
	switch p.target.(type) {
	case *Npc:
		return apNpcTriggerForOp(p.targetOp)
	case *entitypkg.Loc:
		return apLocTriggerForOp(p.targetOp)
	case *Player:
		return apPlayerTriggerForOp(p.targetOp)
	case *entitypkg.Obj:
		return apObjTriggerForOp(p.targetOp)
	}
	return 0, false
}

// triggerTypeAndCategory derives (typeId, categoryId) from the target's
// type registry, applying the targetSubject.com override per TS
// Player.getOpTrigger:993-995 / Player.getApTrigger:1027-1029.
//
// Player target: typeId stays -1 (TS Player.ts:971-972 default — Player
// branch doesn't set type) and categoryId stays -1 (provider falls
// through LookupKeyForType / LookupKeyForCategory to LookupKeyForGlobal).
//
// DEVIATION NAI-78-D-NULL-TYPE-GUARD-OMITTED: TS getOpTrigger:983-985 /
// getApTrigger:1015-1017 has a `if (!type) return null` guard that fires
// when NpcType.get / LocType.get / ObjType.get returns null (entity has
// an unknown type ID). Goscape's per-arm fallback to categoryId=0 lets
// execution continue to GetByTrigger, which returns nil from the 3-tier
// fallback in practice (no scripts registered at the unknown key).
// Matches existing fire-helper convention at interaction_trigger.go (Npc
// branch reads `if npc.typ != nil { … }`). Production cache always
// registers types for spawned entities.
//
// Internal — used by getOpTrigger and getApTrigger.
func triggerTypeAndCategory(p *Player, srv *Server) (typeId, categoryId int) {
	typeId = -1
	categoryId = -1

	switch tgt := p.target.(type) {
	case *Npc:
		typeId = tgt.typeId
		if tgt.typ != nil {
			categoryId = tgt.typ.Category
		} else {
			categoryId = 0
		}
	case *entitypkg.Loc:
		typeId = tgt.Type()
		categoryId = 0
		if locId := tgt.Type(); srv.locTypes != nil && locId >= 0 && locId < len(srv.locTypes.Configs) {
			if lt := srv.locTypes.Configs[locId]; lt != nil {
				categoryId = lt.Category
			}
		}
	case *entitypkg.Obj:
		typeId = tgt.Type
		categoryId = 0
		if srv.objTypes != nil && tgt.Type >= 0 && tgt.Type < len(srv.objTypes.Configs) {
			if ot := srv.objTypes.Configs[tgt.Type]; ot != nil {
				categoryId = ot.Category
			}
		}
	case *Player:
		// typeId, categoryId stay -1.
	}

	typeId = resolveTriggerTypeId(p, typeId)
	return typeId, categoryId
}

// getOpTrigger resolves the [op<entity><op>,<typeId>] script for the
// player's anchored target. Mirrors LostCityRS/Engine-TS
// Player.ts:966-998. Returns nil if target is nil, op is unsupported,
// or no script registered. Used by tryInteract (interaction.go) to gate
// branch 1 (OP fire).
//
// The +7 offset converts an APXXX trigger into the matching OPXXX trigger
// per TS Player.ts:997 ScriptProvider.getByTrigger(this.targetOp + 7, …).
func getOpTrigger(p *Player, srv *Server) *script.ScriptFile {
	if p.target == nil {
		return nil
	}
	apTrigger, ok := apTriggerForTarget(p)
	if !ok {
		return nil
	}
	typeId, categoryId := triggerTypeAndCategory(p, srv)
	return srv.scriptProvider.GetByTrigger(apTrigger+7, typeId, categoryId)
}

// getApTrigger resolves the [ap<entity><op>,<typeId>] script. Mirror of
// getOpTrigger without the +7 offset. Mirrors LostCityRS/Engine-TS
// Player.ts:1000-1032. Used by tryInteract to gate branch 2 (AP fire).
func getApTrigger(p *Player, srv *Server) *script.ScriptFile {
	if p.target == nil {
		return nil
	}
	apTrigger, ok := apTriggerForTarget(p)
	if !ok {
		return nil
	}
	typeId, categoryId := triggerTypeAndCategory(p, srv)
	return srv.scriptProvider.GetByTrigger(apTrigger, typeId, categoryId)
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
	state.LocOps = srv.locOps
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
	state.LocOps = srv.locOps
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore +
	// nextTarget-conditional waypoint clear. NAI-68.
	savedTarget := p.target
	savedWP := p.waypoints
	savedIdx := p.waypointIndex
	p.target = nil
	p.waypointIndex = -1

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget
	if p.nextTarget != nil {
		// TS L1162: clear destination so next-tick interaction starts fresh.
		p.waypointIndex = -1
	} else {
		// No script-set target — restore waypoints (TS L1146 inverse).
		p.waypoints = savedWP
		p.waypointIndex = savedIdx
	}

	// TS L1163-1167 same-tick AP retry: when state.Execution is
	// Finished/Aborted AND apRangeCalled is true, tryInteract sees the
	// flag, restores interactionFired=false, and returns false so
	// processInteraction's walk-arm runs and post-step tryInteract
	// re-fires AP with the new range. Suspended scripts leave
	// apRangeCalled false and keep the anchor across ticks via the
	// suspended ScriptState. NAI-69 closes
	// NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.
	p.interactionFired = true
}
