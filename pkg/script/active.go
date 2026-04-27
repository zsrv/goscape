package script

// ActivePlayer is the minimal surface RuneScript needs from a Player.
// Sub-spec S2 wires modules/world.Player to this interface. S4 adds
// suspension + queue methods.
type ActivePlayer interface {
	MessageGame(msg string)
	Username() string

	// SetDelayed marks the active player as suspended for `ticks` more
	// ticks starting next tick. Implementation must compute
	// resumeTick = currentTick + 1 + ticks.
	SetDelayed(ticks int)

	// EnqueueScriptArgs appends a queued fresh-run request with the
	// given queue type and the caller-supplied parallel arg slices
	// (IntArgs + StringArgs — matches TS PlayerQueueRequest.args
	// ScriptArgument[] shape per
	// Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15). Delay=0
	// fires same tick. STRONG-type entries fire even if the player is
	// busy; others wait until idle. nil/nil expresses "no args" — the
	// TS-faithful empty-args default (Player.ts:821 args=[]).
	//
	// Returns a non-nil error when scriptID does not resolve to a
	// registered script (mirrors TS PlayerOps.ts:103-105 throw). The
	// goscape error is `fmt.Errorf("unable to find queue script: %d",
	// scriptID)`.
	//
	// (S5h: renamed from EnqueueScript to carry type. NAI-26 Bundle 1:
	// renamed from EnqueueScriptTyped to carry parallel-slice args and
	// error return.)
	EnqueueScriptArgs(scriptID uint32, delay int, intArgs []int, stringArgs []string, qtype PlayerQueueType) error

	// StoreActiveScript saves a Suspended ScriptState so the tick loop
	// can resume it when the player's delay expires.
	StoreActiveScript(state *ScriptState)

	// ClearActiveScript discards any stored ScriptState. Called after
	// Finished/Aborted runs and on logout/cleanup.
	ClearActiveScript()

	// Playtime returns the number of ticks the player has been online
	// this session, used by the TIMESPENT / GETTIMESPENT opcodes.
	Playtime() int

	// S5b: VARP read/write.

	// Varp returns the player's current value for varp id. Returns 0 on OOB.
	Varp(id int) int32

	// SetVarp writes val to the player's varp storage. If the varp type
	// has transmit=true the write is also sent to the client via
	// VARP_SMALL / VARP_LARGE. OOB writes are dropped silently.
	SetVarp(id int, val int32)

	// S5c: position / facing / teleport.

	// CoordPacked returns the player's current position packed as a single
	// RS2 coord int: (level<<28) | (x<<14) | z. Used by the COORD opcode.
	CoordPacked() int

	// TeleJump instantly teleports the player to (x, z, level) with no
	// interpolation, clearing any pending walk queue. Invalid coordinates
	// are dropped silently.
	TeleJump(x, z, level int)

	// Teleport moves the player to (x, z, level) and flags the client for
	// a smooth teleport transition. Invalid coordinates are dropped silently.
	Teleport(x, z, level int)

	// FaceSquare rotates the player to face the square at absolute
	// (x, z) on the current level.
	FaceSquare(x, z int)

	// S5c: stats.

	// Stat returns the player's current (possibly boosted/drained) level
	// for skill id. Returns 0 on OOB.
	Stat(id int) int

	// StatBase returns the player's base (unboosted) level for skill id,
	// derived from the stored XP. Returns 0 on OOB.
	StatBase(id int) int

	// StatXP returns the player's accumulated XP for skill id as a scaled
	// integer (authentic: XP * 10). Returns 0 on OOB.
	StatXP(id int) int

	// SetCurLevel overrides the player's current level for skill id,
	// clamped to [0, 255]. OOB ids are dropped silently.
	SetCurLevel(id int, level int)

	// AddXP adds xp (scaled * 10) to the player's stored XP for skill id,
	// recomputing base level and clamping at the XP cap. OOB ids are
	// dropped silently.
	AddXP(id int, xp int)

	// S5c: animation.

	// PlayAnim schedules sequence seqID with the given client-side delay
	// on the player's primary animation slot. seqID=-1 clears.
	PlayAnim(seqID, delay int)

	// PlaySpotAnim schedules a graphic (spotanim) on the player at the
	// given height with the given client-side delay. id=-1 clears.
	PlaySpotAnim(id, height, delay int)

	// SetReadyAnim sets the player's idle/stand animation.
	SetReadyAnim(seqID int)

	// SetTurnAnim sets the player's turn-in-place animation.
	SetTurnAnim(seqID int)

	// SetWalkAnim sets the player's forward-walk animation.
	SetWalkAnim(seqID int)

	// SetWalkAnimB sets the player's backward-walk animation.
	SetWalkAnimB(seqID int)

	// SetWalkAnimL sets the player's strafe-left walk animation.
	SetWalkAnimL(seqID int)

	// SetWalkAnimR sets the player's strafe-right walk animation.
	SetWalkAnimR(seqID int)

	// SetRunAnim sets the player's run animation.
	SetRunAnim(seqID int)

	// S5f: interface / modal control.

	// CloseModal closes any currently open main/chat/side interface and
	// flags the client to refresh modal state.
	CloseModal()

	// OpenMain opens the given interface component as the main modal,
	// closing any chat/side modals per authentic TS rules.
	OpenMain(com int)

	// OpenChat opens the given interface component as the chat modal,
	// leaving any main modal open.
	OpenChat(com int)

	// OpenSide opens the given interface component as the side modal,
	// leaving any main modal open.
	OpenSide(com int)

	// OpenMainSide opens mainCom as the main modal and sideCom as the
	// side modal simultaneously.
	OpenMainSide(mainCom, sideCom int)

	// IfSetText emits an IF_SETTEXT wire op setting the text of interface
	// component com. Fire-and-forget; no server-side persistence.
	IfSetText(com int, text string)

	// IfSetModel emits an IF_SETMODEL wire op binding modelID to component
	// com. Fire-and-forget; no server-side persistence.
	IfSetModel(com, modelID int)

	// IfSetNpcHead emits an IF_SETNPCHEAD wire op binding the head of
	// npcID to component com. Fire-and-forget; no server-side persistence.
	IfSetNpcHead(com, npcID int)

	// IfSetPlayerHead emits an IF_SETPLAYERHEAD wire op binding the local
	// player's head to component com. Fire-and-forget; no server-side
	// persistence.
	IfSetPlayerHead(com int)

	// IfSetAnim emits an IF_SETANIM wire op binding sequence seqID to
	// component com. Fire-and-forget; no server-side persistence.
	IfSetAnim(com, seqID int)

	// IfSetHide emits an IF_SETHIDE wire op setting the hide flag on
	// component com. Fire-and-forget; no server-side persistence.
	IfSetHide(com int, hide bool)

	// IfSetTab emits an IF_SETTAB wire op binding component com to tab
	// slot tab. Fire-and-forget; no server-side persistence.
	IfSetTab(com, tab int)

	// IfSetObject emits an IF_SETOBJECT wire op binding objID at the
	// given scale to component com. Fire-and-forget; no server-side
	// persistence.
	IfSetObject(com, objID, scale int)

	// IfSetColour emits an IF_SETCOLOUR wire op setting the text colour
	// of component com. Fire-and-forget; no server-side persistence.
	IfSetColour(com, colour int)

	// IfSetPosition emits an IF_SETPOSITION wire op setting the (x, y)
	// position of component com. Fire-and-forget; no server-side
	// persistence.
	IfSetPosition(com, x, y int)

	// IfSetRecol emits an IF_SETRECOL wire op remapping srcColour to
	// dstColour on component com. Fire-and-forget; no server-side
	// persistence.
	IfSetRecol(com, srcColour, dstColour int)

	// IfSetTabActive emits an IF_SETTABACTIVE wire op making tab the
	// currently-active tab. Fire-and-forget; no server-side persistence.
	IfSetTabActive(tab int)

	// SetResumeButtons stores the 5 resume-button interface ids for
	// later consumption by P_PAUSEBUTTON. No wire op is emitted.
	SetResumeButtons(b1, b2, b3, b4, b5 int)

	// S5g: dialog suspension.

	// LastCom returns the component id most recently clicked on the client.
	// Used by LAST_COM opcode and pause-button resume gating.
	LastCom() int

	// SendCountDialog writes a P_COUNTDIALOG wire packet to the active
	// player's client, prompting an "enter a number" dialog. Called by
	// the P_COUNTDIALOG script opcode before suspension.
	SendCountDialog()

	// S5h: action-clear ops.

	// StopAction clears the current interaction target + pending action.
	// Matches TS Player.stopAction().
	StopAction()

	// ClearPendingAction clears the current interaction + pending action
	// + closes any open modal. Walk queue is preserved.
	ClearPendingAction()

	// S5i: timer ops.

	// SetTimer registers a timer that re-runs the script at scriptID every
	// `interval` ticks with `intArgs`/`stringArgs` as parallel-slice typed
	// args (matching TS PlayerOps.ts:826,834 popScriptArgs convention).
	// Overwrites any existing timer at the same scriptID. type = TimerNormal
	// (waits for idle) or TimerSoft (fires while busy).
	//
	// NAI-27 Bundle 2: returns a non-nil error when the scriptID does not
	// resolve to a registered script (mirrors TS PlayerOps.ts:822-824 +
	// :838-840 throw shape). Engine-dispatch paths with no provider
	// configured are tolerant and return nil unchanged.
	SetTimer(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype PlayerTimerType) error

	// ClearTimer cancels the timer at scriptID, regardless of type.
	// Silent no-op if no such timer.
	ClearTimer(scriptID uint32)

	// GetTimer returns the absolute clock tick at which the timer at
	// scriptID was last set or fired (TS-faithful per Player.ts:910 /
	// PlayerOps.ts:858), or -1 if no such timer is registered.
	//
	// NAI-27 Bundle 2: doc-comment updated alongside the (*Player).GetTimer
	// semantic flip from "(Clock+Interval)-now" remaining-ticks to absolute
	// Clock. Pre-Bundle-2 tests asserting non-Clock arithmetic on this
	// return are broken; the entity-level pin is at
	// modules/world/player_timer_test.go.
	GetTimer(scriptID uint32) int

	// S5m: "last-input" queries. Each pushes the player's stored field
	// captured during the most recent matching client packet — item
	// slots from OPHELD/OPUSE/INV_BUTTOND events. Scripts running
	// outside those triggers can still read these fields; we skip the
	// TS trigger-whitelist gate for MVP.
	LastItem() int
	LastSlot() int
	LastUseItem() int
	LastUseSlot() int
	LastTargetSlot() int

	// CamReset sends a CAM_RESET wire packet to the client, resetting
	// any custom camera state. Called by the CAM_RESET script opcode.
	CamReset()

	// HintNpc directs the client to render a hint arrow pointing at the
	// NPC with the given nid (slot id). Mirrors TS Player.hintNpc at
	// Player.ts:2174-2176, which writes a HintArrow(type=1) packet.
	// Called by the HINT_NPC (opcode 2028) handler.
	HintNpc(nid int)

	// HintCoord directs the client to render a hint arrow at the (x, z) tile
	// with the given offset (2..6, sub-tile arrow position) and height.
	// Mirrors TS Player.hintTile at Player.ts:2178-2180; called by the
	// HINT_COORD (opcode 2027) handler. NAI-39.
	HintCoord(offset, x, z, height int)

	// HintPlayer directs the client to render a hint arrow pointing at the
	// player in slot `slot`. Mirrors TS Player.hintPlayer at
	// Player.ts:2182-2184; called by the HINT_PL (opcode 2029) handler.
	// NAI-39.
	HintPlayer(slot int)

	// HintStop directs the client to clear any active hint arrow. Mirrors
	// TS Player.stopHint at Player.ts:2186-2188; called by the HINT_STOP
	// (opcode 2030) handler. NAI-39.
	HintStop()

	// Slot returns the player's authoritative slot id (the index into the
	// world's player array). Mirrors TS Player.slot. Consumed by HINT_PL,
	// which reads activePlayer2.slot. NAI-39.
	Slot() int

	// StaffModLevel returns the player's staff moderation level.
	// 0 for regular players; >0 for mods/admins. Used by STAFFMODLEVEL
	// opcode to gate mod-only behaviour. Matches the rsbuf.PlayerSource
	// signature so *Player can satisfy both interfaces without a
	// duplicate method.
	StaffModLevel() int32

	// UID returns the player's persistent account uid (from the login
	// RPC). Used by the UID script opcode for mod/account-state checks.
	UID() int

	// X returns the player's current absolute world X coord. Used by
	// MAP_PLAYERCOUNT (NAI-35-T2) for rect-filter checks; will also be
	// used by PlayerIterator.passesFilter (NAI-35-T4).
	X() int

	// Z returns the player's current absolute world Z coord. Used by
	// MAP_PLAYERCOUNT (NAI-35-T2) and PlayerIterator.passesFilter
	// (NAI-35-T4).
	Z() int

	// SetApRange sets the approach-range-in-tiles for the active
	// interaction AND marks apRangeCalled=true. Called by p_aprange
	// script opcode when an APLOC trigger wants to extend the range
	// the player should approach before re-firing. Matches TS
	// PlayerOps.ts:P_APRANGE — both fields are set in a single call
	// (tick-serialized by the engine; no lock needed).
	SetApRange(n int)

	// TargetSubjectCom returns the com-component value stored at click
	// time by OpLocT-style handlers. For OpLocT it's spellCom; for
	// OpLoc1..5 and OpLocU it's -1. Allows APLOCT scripts to read
	// which spell the player cast via future @spellcom-style script
	// variables. S6m: interface method added ahead of the script-opcode
	// consumer that reads it.
	TargetSubjectCom() int

	// S6u: inventory listener registration opcodes (inv_transmit /
	// inv_stoptransmit).

	// InvListenOnCom registers an inventory listener at UI component id
	// `com` tracking inv type `invType`. Callers pass the player's own
	// UID (via ActivePlayer.UID()) or a popped uid for INV_OTHERTRANSMIT
	// scenarios; the implementation rewrites source to -1 internally
	// when invType has SCOPE_SHARED scope (matches TS Player.ts:1456-1459).
	// On the dispatch side, source == -1 routes to the world-shared
	// inventory; source >= 0 routes to the player at that server slot.
	// Replaces any existing listener at com unless the existing entry
	// has the same type (in which case the call is a no-op preserving
	// FirstSeen state). Safe when the implementation's listener map is
	// still nil — it must lazy-init.
	InvListenOnCom(invType, com, source int)

	// InvStopListenOnCom unregisters the listener at UI component id com.
	// No-op when no listener exists there. Must be safe when the listener
	// map is nil.
	InvStopListenOnCom(com int)

	// S6v: p_op* script-queued interaction methods.

	// SetInteractionScriptLoc anchors the player on `loc` with trigger
	// ApLoc<op> as a script-queued interaction (TS Interaction.SCRIPT).
	// op is 1-indexed (1..5). Matches TS PlayerOps.ts:386-402 terminal
	// setInteraction call.
	//
	// Implementations must type-assert the narrow ActiveLoc interface to
	// their concrete loc type. Caller pre-validates op ∈ [1,5].
	SetInteractionScriptLoc(loc ActiveLoc, op int)

	// SetInteractionScriptNpc anchors the player on `npc` with trigger
	// ApNpc<op> as a script-queued interaction. Matches TS
	// PlayerOps.ts:404-415.
	SetInteractionScriptNpc(npc ActiveNpc, op int)

	// S7a: protected-binding gate.

	// CanAccess reports whether the player can be bound as the active
	// player by P_FINDUID. Returns false when delayed, when a modal
	// main/chat is open, or when a suspended protected script is
	// stored on the player. Mirrors TS Player.canAccess
	// (Engine-TS/src/engine/entity/Player.ts:805-812). FINDUID does
	// NOT consult this — only P_FINDUID does.
	CanAccess() bool

	// S7b: anim-protect flag.

	// SetAnimProtect updates the player's anim-protect flag. While nonzero,
	// the engine should suppress in-engine animation requests (gated reader
	// unported, see PAnimProtect handler comment and deviation S7b-D1).
	SetAnimProtect(v int)

	// S7c: appearance refresh.

	// SetAppearanceInv updates the active player's appearanceInv field AND
	// flags MaskAppearance so the next tick regenerates the appearance buffer
	// (tick.go:325-335). Mirrors TS Player.buildAppearance at
	// Engine-TS/src/engine/entity/Player.ts:1836-1839 — both side-effects are
	// required; tests assert both. NAI-21 Bundle 1 closed S7c-D1:
	// generateAppearance reads p.invs[p.appearanceInv]. Callers pre-validate
	// id via checkInvType.
	SetAppearanceInv(id int)

	// S7e: character-design save gate.

	// SetAllowDesign updates the active player's allowDesign flag. When true,
	// the client's IdkSaveDesign inbound packet (character-design recustomise)
	// is permitted to apply. Mirrors TS Player.allowDesign
	// (Engine-TS/src/engine/entity/Player.ts:323). The handler coerces the
	// popped int via v==1 before calling. Reader path (IdkSaveDesignHandler)
	// unported — deviation S7e-D1.
	SetAllowDesign(v bool)

	// LowMemory reports whether the player's client requested low-memory
	// mode at login (carried on the RS2 login request's LowMemory bit).
	// Script opcodes that trigger client audio loads gate on this flag —
	// see handleMidiSong / handleMidiJingle in handlers_player.go.
	LowMemory() bool

	// PlaySong sends a MIDI song by name to the client. Called by the
	// MIDI_SONG script opcode (PlayerOps.ts:796-804). Implementation
	// performs TS name normalization (lowercase + spaces→underscores),
	// looks up the preloaded blob + CRC, and writes MidiSong; silent
	// no-op on empty name or missing PRELOADED entry (mirrors TS guard
	// `if (song && crc)` at Player.ts:1910).
	PlaySong(name string)

	// PlayJingle sends a short MIDI jingle by name to the client. Called
	// by the MIDI_JINGLE script opcode (PlayerOps.ts:806-816).
	// Implementation performs TS name normalization (lowercase +
	// underscores→spaces), looks up the preloaded blob, and writes
	// MidiJingle; silent no-op on empty name or missing PRELOADED entry
	// (mirrors TS guard `if (jingle)` at Player.ts:1923).
	PlayJingle(delay int, name string)
}

// ActiveNpc is the per-NPC surface that NPC_* opcodes and VARN
// handlers read/write. Set on ScriptState before Execute by callers
// that target a specific NPC (test fixtures, OPNPC routing, etc.).
type ActiveNpc interface {
	NpcType() int // returns NpcType.id
	NpcX() int
	NpcZ() int
	NpcLevel() int
	NpcStat(stat int) int     // current (boosted) level — S6a: only HP (id 0) is real
	NpcBaseStat(stat int) int // base level — S6a: only HP (id 0) is real
	NpcCategory() int
	NpcUID() int // (typeId << 16) | nid
	// Nid returns the NPC slot id (the low 16 bits of NpcUID). Used by
	// NPC-targeted player-bound packets like HintArrow that reference
	// the NPC by slot rather than by packed UID.
	Nid() int
	NpcVarN(id int) int32
	SetNpcVarN(id int, val int32)
	// Say buffers text as the NPC's speech bubble for the current tick,
	// flagging NpcMaskSay so the NPC-info encoder emits it. Empty text is
	// allowed (produces an empty bubble that clears itself next tick via
	// ResetMasks).
	Say(text []byte)

	// Animate schedules sequence `id` with client-side `delay` on the NPC's
	// primary animation slot this tick. id = -1 clears.
	Animate(id, delay int)

	// FaceCoord rotates the NPC to face absolute square (x, z). Wire coords
	// are doubled + 1 (face-center convention).
	FaceCoord(x, z int)

	// ChangeType morphs the NPC to newType and schedules a revert to
	// baseType after `duration` ticks. Resets all 6 stats onto the new
	// type's base values using a boost/drain-preserving formula. Mirrors
	// TS Npc.changeType at Engine-TS/.../Npc.ts:427-449 with reset=true.
	// No-op when duration < 1 OR when the NPC is dead.
	ChangeType(newType, duration int)

	// ChangeTypeKeepAll morphs the NPC to newType and schedules a revert
	// after `duration` ticks, preserving all current stat values (no
	// reset). The revert, when it fires, takes the light path
	// (resetOnRevert=false → typeId + uid + CHANGE_TYPE mask only).
	// Mirrors TS Npc.changeType at Engine-TS/.../Npc.ts:427-449 with
	// reset=false, dispatched from NPC_CHANGETYPE_KEEPALL (opcode 2506,
	// TS NpcOps.ts:465-471). No-op when duration < 1 OR when the NPC is dead.
	ChangeTypeKeepAll(newType, duration int)

	// Damage applies `amount` damage of `dmgType` to the NPC this tick,
	// flagging NpcMaskDamage. Decrements curHP (clamped at 0). Does NOT
	// trigger death handling or auto-retaliate — those belong in a future
	// NPC AI sub-spec.
	Damage(amount, dmgType int)

	// StoreActiveScript saves a NpcSuspended ScriptState so Npc.turn()
	// can resume it when the NPC's delay expires. Mirrors
	// ActivePlayer.StoreActiveScript at active.go:22-24.
	StoreActiveScript(state *ScriptState)

	// ClearActiveScript discards any stored ScriptState. Called after
	// Finished/Aborted runs. Mirrors ActivePlayer.ClearActiveScript.
	ClearActiveScript()

	// SetDelayed marks the NPC as suspended for `ticks` more ticks
	// starting next tick. Implementations compute delayedUntil =
	// currentTick + 1 + ticks. Mirrors ActivePlayer.SetDelayed at
	// active.go:13-14.
	SetDelayed(ticks int)

	// EnqueueScriptForTrigger appends a queued ai_queueN dispatch to
	// the NPC. Matches TS Npc.enqueueScript at Npc.ts:241-245 — the
	// trigger (TriggerAiQueue1..TriggerAiQueue20) identifies which
	// script runs; lookup happens at fire time via
	// scriptProvider.GetByTrigger keyed on the NPC's type + category.
	EnqueueScriptForTrigger(trigger ServerTriggerType, delay int, intArg int)

	// SetTimer sets the tick interval between ai_timer trigger fires
	// on the active NPC. interval == -1 is a silent no-op, matching
	// TS Npc.setTimer at Engine-TS/.../Npc.ts:210-214. Called by the
	// NPC_SETTIMER opcode.
	SetTimer(interval int)

	// SetHuntRange sets the NPC's hunt search radius. Called by
	// the NPC_SETHUNT opcode. Matches TS NpcOps.ts:174-176 — despite
	// the opcode name, this sets RANGE only; mode uses SetHuntMode.
	SetHuntRange(r int)

	// SetHuntMode sets the NPC's HuntType id. -1 clears. Callers
	// do no bounds validation; the hunt processor validates when
	// looking up the HuntType. Mirrors TS NpcOps.ts:178-185.
	SetHuntMode(mode int)

	// SetWalkTrigger sets the deferred AI-queue trigger index for the
	// active NPC. Called by NPC_WALKTRIGGER (opcode 2545). Range
	// validation [1, 20] happens in the handler before -1 transform;
	// this method just writes the field. Mirrors TS Npc.walktrigger
	// at NpcOps.ts:488.
	SetWalkTrigger(queueID int)

	// SetWalkTriggerArg sets the arg passed to the walktrigger script
	// when it eventually fires. Mirrors TS Npc.walktriggerArg at
	// NpcOps.ts:489.
	SetWalkTriggerArg(arg int)

	// Teleport moves the active NPC to (x, z, level) and flags the client
	// for a tele transition. Mirrors (n *Npc).Teleport on the world side
	// (modules/world/npc_script.go). Called by NPC_TELE handler
	// (handlers_npc.go) after checkCoord validates and unpacks the packed
	// coord.
	//
	// DEVIATION NAI-34-D3, D4 (both entities) and NAI-34-D5-NPC vs TS
	// PathingEntity.teleport (PathingEntity.ts:267) — partial closure as
	// of NAI-36-T7 (D1, D2 closed for both entities; D5 closed for Player).
	//
	// RESIDUAL (active deviations):
	//   - D3-Player + D3-NPC: no focus() call (PathingEntity.ts:286).
	//     Player has FaceCoord (player_masks.go:45) and Npc has both
	//     focus() (npc_interaction.go:686) and FaceCoord (npc_masks.go:120),
	//     so neither side is dead-API gated; closure was deferred to a
	//     future "pathing-entity-focus-and-step-tracking" sub-spec because
	//     fine-coord conversion + instant-flag semantics need design.
	//   - D4-Player + D4-NPC: no lastStepX/Z adjust (PathingEntity.ts:289-290).
	//     Player has lastStepX/Z fields (player.go:79) but Npc does NOT;
	//     adding to Npc is dead-API per dead_api_polish.md until an NPC-side
	//     stride-tracking consumer materializes. Player-side closure
	//     deferred to the same future sub-spec for consistency.
	//   - D5-NPC: no `previousLevel != level → moveSpeed=INSTANT + jump=true`
	//     branch on Npc. Npc has no jump field; dead-API foot-gun. Player
	//     half closes in NAI-36-T7. Tracked for the same future sub-spec.
	//
	// CLOSED in NAI-36-T7:
	//   - D1 (level clamp [0, 3]) — both entities.
	//   - D2 (unallocated-zone reject) — both entities.
	//   - D5-Player (level-change INSTANT/jump branch).
	//   - Player.Teleport order divergence (refresh-then-flag).
	//
	// See (n *Npc).Teleport doc comment in modules/world/npc_script.go
	// for the matching world-side tracker.
	Teleport(x, z, level int)

	// QueueWaypoint clears any existing path and sets a single destination
	// (level-implicit by current NPC level). Mirrors TS Npc.queueWaypoint
	// at Engine-TS/.../Npc.ts. Used by NPC_WALK (opcode 2544).
	QueueWaypoint(x, z int)

	// TargetOp returns the NPC's current targetOp/mode value (the field set
	// by NPC_SETMODE / interaction binding). Used by NPC_GETMODE (opcode 2522).
	TargetOp() int

	// ClearInteraction clears the NPC's current interaction binding.
	// Mirrors TS PathingEntity.clearInteraction. Used by NPC_SETMODE
	// clear-target branch (NAI-36).
	ClearInteraction()

	// ResetDefaults reverts the NPC to defaultMode + clears interaction
	// + emits faceEntity reset mask. Mirrors TS Npc.resetDefaults. Used
	// by NPC_SETMODE NULL-mode + no-target-fallthrough branches (NAI-36).
	ResetDefaults()

	// ClearPatrol resets nextPatrolTick to -1. Mirrors TS Npc.clearPatrol
	// at Engine-TS/.../Npc.ts:377-379. Used by NPC_SETMODE PATROL branch
	// (NAI-36).
	ClearPatrol()

	// SetTargetOp sets n.targetOp directly (no interaction binding). Used
	// by NPC_SETMODE clear-target and target-binding branches that assign
	// targetOp before the interaction call. Mirrors TS direct property
	// write `state.activeNpc.targetOp = mode` at NpcOps.ts:196,205.
	SetTargetOp(mode int)

	// SetInteractionScript binds the NPC's interaction to target with mode
	// as the targetOp, using Interaction.SCRIPT. Mirrors TS
	// Npc.setInteraction(Interaction.SCRIPT, target, mode) at NpcOps.ts:225-228.
	// target is one of: ActivePlayer, ActiveNpc, ActiveLoc, ActiveObj
	// (script-side interfaces). Adapter type-switches on the underlying
	// concrete world-side entity. Pass nil to no-op (caller handles
	// null-target as resetDefaults).
	SetInteractionScript(target any, mode int)
}

// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
	LocType() int // returns the LocType ID (from packed Loc.Info bitfield)
}

// ActiveObj is the surface that OBJ_* and AI_APOBJ/AI_OPOBJ handlers
// use to read obj state. Narrow by design — extend as future sub-specs
// wire more obj script opcodes.
type ActiveObj interface {
	ObjType() int              // underlying ObjType id
	Coords() (x, z, level int) // world position
}
