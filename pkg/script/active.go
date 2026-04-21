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
}

// Stubs for later sub-specs; defined now to avoid interface churn in S6.
type ActiveNpc interface{}
type ActiveLoc interface{}
type ActiveObj interface{}
