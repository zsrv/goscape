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

	// EnqueueScriptTyped appends a queued fresh-run request with the
	// given queue type. Delay=0 fires same tick. STRONG-type entries
	// fire even if the player is busy; others wait until idle.
	// (S5h: renamed from EnqueueScript to carry type.)
	EnqueueScriptTyped(scriptID uint32, delay int, intArg int, qtype PlayerQueueType)

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
	// `interval` ticks with `intArg` as the single int arg. Overwrites any
	// existing timer at the same scriptID. type = TimerNormal (waits for
	// idle) or TimerSoft (fires while busy).
	SetTimer(scriptID uint32, interval int, intArg int, ttype PlayerTimerType)

	// ClearTimer cancels the timer at scriptID, regardless of type.
	// Silent no-op if no such timer.
	ClearTimer(scriptID uint32)

	// GetTimer returns the number of ticks until the timer at scriptID
	// fires next, or -1 if no such timer exists. May be negative if
	// overdue but not yet processed.
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

	// StaffModLevel returns the player's staff moderation level.
	// 0 for regular players; >0 for mods/admins. Used by STAFFMODLEVEL
	// opcode to gate mod-only behaviour. Matches the rsbuf.PlayerSource
	// signature so *Player can satisfy both interfaces without a
	// duplicate method.
	StaffModLevel() int32

	// UID returns the player's persistent account uid (from the login
	// RPC). Used by the UID script opcode for mod/account-state checks.
	UID() int

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

	// ChangeType morphs the NPC to `newType`. The client swaps the model on
	// the next NPC-info flush; server-side fields beyond typeId are not
	// re-initialized (stats, category, etc. still reference the old config).
	// The script op NPC_CHANGETYPE also carries a `duration` parameter for
	// timed revert, but S6c discards it (method takes type only); future
	// AI sub-spec wires a revert timer.
	ChangeType(newType int)

	// Damage applies `amount` damage of `dmgType` to the NPC this tick,
	// flagging NpcMaskDamage. Decrements curHP (clamped at 0). Does NOT
	// trigger death handling or auto-retaliate — those belong in a future
	// NPC AI sub-spec.
	Damage(amount, dmgType int)
}

// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
	LocType() int // returns the LocType ID (from packed Loc.Info bitfield)
}

// ActiveObj is a stub for later sub-specs; defined now to avoid interface churn in S6.
type ActiveObj interface{}
