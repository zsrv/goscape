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

	// EnqueueScript appends a queued fresh-run request with one int arg.
	// delay=0 fires same tick (authentic TS behavior).
	EnqueueScript(scriptID uint32, delay int, intArg int)

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
}

// Stubs for later sub-specs; defined now to avoid interface churn in S6.
type ActiveNpc interface{}
type ActiveLoc interface{}
type ActiveObj interface{}
