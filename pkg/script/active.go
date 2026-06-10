package script

// WealthEvent captures a single wealth-affecting event for analytics.
// Mirrors TS WealthEvent (WealthEvent.ts:19-23) + WealthEventParams
// (WealthEvent.ts:7-17). Goscape AddWealthEvent appends to an
// in-memory log only per NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY;
// analytics RPC integration deferred.
//
// rev-244 B3 — account_id threading (WealthEvent.ts:21-22,
// Player.ts:640-642, NetworkPlayer.ts:259-261):
// AccountID and AccountSession are stamped by Player.AddWealthEvent
// (not by the caller). RecipientID mirrors WealthEventParams.recipient_id?
// (WealthEvent.ts:13); RecipientSession mirrors recipient_session?
// (WealthEvent.ts:14, previously the sole recipient field).
type WealthEvent struct {
	EventType    int
	AccountItems []WealthItem
	AccountValue int

	// AccountID is the persistent DB account.id of the player who owns
	// this event. Stamped by Player.AddWealthEvent from p.accountID
	// (World.ts:1932 sources it from the login reply).
	// TS WealthEvent.account_id (WealthEvent.ts:21).
	AccountID int64

	// AccountSession is the per-login session correlation key for the
	// account. Stamped by Player.AddWealthEvent:
	//   - client present → p.session (UUID, NetworkPlayer.ts:260)
	//   - no client       → "headless" (Player.ts:641)
	// TS WealthEvent.account_session (WealthEvent.ts:22).
	AccountSession string

	// RecipientID is the optional DB account.id of the counterparty
	// (set by trade/PvP/duel callers that know the other player's ID).
	// TS WealthEventParams.recipient_id? (WealthEvent.ts:13).
	// Zero means absent (TS optional field).
	RecipientID int64

	// RecipientSession is the optional per-login session UUID of the
	// counterparty; set by trade/PvP/duel callers alongside RecipientID.
	// TS WealthEventParams.recipient_session? (WealthEvent.ts:14).
	RecipientSession string
	RecipientItems   []WealthItem
	RecipientValue   int
}

// WealthItem is a single line-item inside a WealthEvent.
type WealthItem struct {
	ID    int
	Name  string
	Count int
}

// WealthEventType* enum mirrors TS WealthEventType (const enum,
// WealthEventType.ts — 0-based ordinals).
const (
	WealthEventTypeTrade     = 0
	WealthEventTypePVP       = 1
	WealthEventTypeStake     = 2
	WealthEventTypeDeath     = 3
	WealthEventTypeDrop      = 4
	WealthEventTypePickup    = 5
	WealthEventTypeShopBuy   = 6
	WealthEventTypeShopSell  = 7
	WealthEventTypeLowAlch   = 8
	WealthEventTypeHighAlch  = 9
	WealthEventTypePartyRoom = 10
)

// ActivePlayer is the minimal surface RuneScript needs from a Player.
// Sub-spec S2 wires modules/world.Player to this interface. S4 adds
// suspension + queue methods.
type ActivePlayer interface {
	MessageGame(msg string)
	Username() string
	DisplayName() string

	// SetDelayed marks the active player as suspended for `ticks` more
	// ticks starting next tick. Implementation must compute
	// resumeTick = currentTick + 1 + ticks.
	SetDelayed(ticks int)

	// LastMovement returns the absolute tick value stored on the player's
	// lastMovement field. The field is written to currentTick + 1 at the
	// end of any tick in which the player actually advanced (stepsTaken > 0),
	// matching TS Player.processMovement at Engine-TS/.../Player.ts:675-677.
	//
	// Consumed by P_ARRIVEDELAY (PlayerOps.ts:359), which suspends the
	// active script when the player moved within the past 2 ticks
	// (lastMovement >= currentTick) and is a no-op otherwise.
	//
	// Returns 0 when the player has never moved (zero-value of the field).
	LastMovement() int

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

	// OnScriptFinishedOrAborted is the post-Execute tail for the
	// Finished or Aborted execution states. If state matches the
	// player's currently stored activeScript, nulls activeScript;
	// additionally calls CloseModal(false) when no MAIN modal is
	// open. Mirrors TS Player.executeScript tail (Player.ts:2143-2148).
	// Player-only modal clause; the symmetric ActiveNpc method has no
	// modal handling.
	//
	// NAI-54 closure of NAI-53-F1.
	OnScriptFinishedOrAborted(state *ScriptState)

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

	// VarpString reads the per-player STRING-typed var at id. Returns
	// "" defensively for OOB or never-written ids. Mirrors TS
	// Player.getVar dispatched on STRING type.
	VarpString(id int) string

	// SetVarpString writes the per-player STRING-typed var at id. OOB
	// silently dropped. No wire-send (this protocol revision has no
	// varp_string opcode); server-side state only.
	SetVarpString(id int, val string)

	// GetVarBit reads the varbit's bit-range out of its base varp.
	// Unconfigured/garbage varbit ranges read 0. Mirrors TS
	// Player.getVarBit (Player.ts:1750-1760 @43e02957). Used by the
	// PUSH_VARBIT opcode. rev-254.
	GetVarBit(id int) int32

	// SetVarBit writes value into the varbit's bit-range of its base
	// varp, preserving the other bits; out-of-range values write 0.
	// Routes through SetVarp so transmit varps resync. Mirrors TS
	// Player.setVarBit (Player.ts:1762-1777 @43e02957). Used by the
	// POP_VARBIT opcode. rev-254.
	SetVarBit(id int, value int32)

	// RunVarpID returns the varp id discovered at config-load time as the
	// engine-level run-mode varp (the config with ClientCode==7). Mirrors TS
	// VarPlayerType.RUN dynamic discovery at Engine-TS/src/cache/config/VarPlayerType.ts:50-53.
	// Returns 0 as a TS-faithful placeholder default when no clientcode-7
	// config exists in the loaded cache.
	RunVarpID() int

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

	// ExactMove schedules an exact-movement animation: the player follows
	// a straight line from (sX, sZ) to (eX, eZ) between client ticks
	// `begin` and `finish`, facing direction `dir`. Mirrors TS
	// Player.exactMove (sets 7 fields + MaskExactMove). Backing impl at
	// modules/world/player_masks.go:28. NAI-160 T4.
	ExactMove(sX, sZ, eX, eZ, begin, finish, dir int)

	// Walk queues a path from the player's current (level, x, z) to the
	// destination (destX, destZ) at the player's level. Production impl
	// runs the server pathfinder (FindPathPlain) and replaces the player's
	// waypoint queue. Empty/failed routes leave the player stationary.
	// Mirrors TS PlayerOps.P_WALK → player.queueWaypoints(findPath(
	// player.level, player.x, player.z, coord.x, coord.z)).
	Walk(destX, destZ int)

	// UnsetMapFlag clears the player's map-click destination by sending
	// the matching client packet. Mirrors TS Player.unsetMapFlag — called
	// by P_EXACTMOVE (PlayerOps.ts:888) and adjacent server-script paths
	// that override a queued waypoint. NAI-160 T4.
	UnsetMapFlag()

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
	// dropped silently. allowMulti applies the configured node_xp_rate
	// multiplier (TS addXp's allowMulti arg; pass false for exact-level
	// grants like the setlevel cheat).
	AddXP(id int, xp int, allowMulti bool)

	// Say buffers `text` as the player's speech bubble for the current
	// tick, flagging MaskSay so the player-info encoder emits it. Empty
	// text is allowed (produces an empty bubble that clears itself next
	// tick via ResetMasks). Mirrors TS Player.say at Player.ts:1893-1896
	// (this.sayMessage = message; this.masks |= PlayerInfoProt.SAY).
	// NAI-160 T1.
	Say(text []byte)

	// HeadIcons returns the player's current head-icon bitmask. Mirrors
	// TS Player.headicons (default 0) at Player.ts:314 / PlayerOps.ts:981.
	// NAI-160 T2.
	HeadIcons() int

	// SetHeadIcons writes `v` into the head-icon bitmask. Caller is
	// responsible for NumberNotNull validation (handler calls checkNotNull
	// before invoking). Mirrors TS direct assignment at PlayerOps.ts:985.
	// NAI-160 T3.
	SetHeadIcons(v int)

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

	// SetRun writes the run-mode toggle (0=walk, 1=run) to the player.
	// Mirrors TS field write `state.activePlayer.run = state.popInt()`
	// at Engine-TS PlayerOps.ts:1205. The varp-mirror side-effect
	// (setVar(VarPlayerType.RUN, run)) remains explicit at the handler
	// call site (handlePRun), per ts_helper_method_bundles memory.
	// NAI-117.
	SetRun(value int)

	// RunEnergy returns the player's current run-energy value as an
	// int (range [0, 10000]). Mirrors TS `state.pushInt(player.runenergy)`
	// at Engine-TS PlayerOps.ts:1177. NAI-117.
	RunEnergy() int

	// RunWeight returns the player's tracked carry weight in grams (TS
	// stores 1/1000 of a kg in `runweight`; production wiring at
	// modules/world/player.go:880 mirrors TS update site). Consumed by
	// PlayerOps.ts:1181 WEIGHT.
	RunWeight() int

	// AfkEventReady returns the per-player AFK-event ready flag. Set true
	// by the random tick gate at modules/world/player.go:1050 (rand <
	// 0.0167 every 500 ticks). Consumed by PlayerOps.ts:1058 AFK_EVENT.
	AfkEventReady() bool

	// SetAfkEventReady writes the AFK-event ready flag. AFK_EVENT clears
	// it to false after dispatching (TS PlayerOps.ts:1060).
	SetAfkEventReady(v bool)

	// SetRunEnergy writes the player's current run-energy value (range
	// [0, 10000]). Caller is responsible for clamping; HEAL_ENERGY clamps
	// in the handler before calling this. Mirrors TS PlayerOps.ts:1054.
	SetRunEnergy(v int)

	// S5f: interface / modal control.

	// CloseModal closes any currently open main/chat/side interface and
	// flags the client to refresh modal state. clearWeakQueue=true (TS
	// default) drops weak-queue entries before processing; false
	// preserves them. Mirrors TS Player.closeModal(clearWeakQueue).
	CloseModal(clearWeakQueue bool)

	// OpenOverlay opens the given component as the full-screen overlay
	// (com == -1 clears it). State-setting only — the per-tick modal
	// flush emits IF_OPENOVERLAY on change (B3 ebce9706). Mirrors TS
	// Player.openOverlay (Player.ts:1955-1965).
	OpenOverlay(com int)

	// OpenMain opens the given interface component as the main modal,
	// closing any chat/side modals per authentic TS rules.
	OpenMain(com int)

	// OpenChat opens the given interface component as the chat modal,
	// leaving any main modal open.
	OpenChat(com int)

	// OpenSide opens the given interface component as the side modal,
	// leaving any main modal open.
	OpenSide(com int)

	// OpenMainModalSide opens mainCom as the main modal and sideCom as the
	// side modal simultaneously. Renamed from OpenMainSide at 244 to
	// match TS openMainSideModal→openMainModalSide rename (9aadcec4).
	OpenMainModalSide(mainCom, sideCom int)

	// OpenTutorial opens com as the tutorial-overlay component. Per TS,
	// opening the tutorial does NOT close any other modal — the TUT bit
	// is OR'd into modalState. Mirrors LostCityRS/Engine-TS
	// Player.ts:1999-2003 (openTutorial).
	OpenTutorial(com int)

	// CloseTutorial closes any currently-open tutorial overlay. Per TS,
	// this is a no-op when no tutorial is open; otherwise it dispatches
	// the matching IF_CLOSE trigger script (if registered) and resets
	// the tutorial slot. Mirrors LostCityRS/Engine-TS Player.closeTutorial
	// (Player.ts:716-726).
	CloseTutorial()

	// FlashTutorial directs the client to flash the named tab to draw
	// the player's attention to it. Fire-and-forget: writes a single
	// TUT_FLASH server packet (opcode 126, 1-byte tab payload) and
	// returns. Mirrors LostCityRS/Engine-TS PlayerOps.ts:694-696 +
	// TutFlashEncoder.ts.
	FlashTutorial(tab int)

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

	// IfSetScrollPos emits an IF_SETSCROLLPOS wire op setting the vertical
	// scroll position of layer component com. Fire-and-forget; no
	// server-side persistence. New in 245.2 (TS PlayerOps.ts:751-757).
	IfSetScrollPos(com, y int)

	// SetPlayerOp emits a SET_PLAYER_OP wire op setting right-click
	// player-menu entry index (1-8) to text with primary state (0-7).
	// Fire-and-forget; no server-side persistence. New in 254
	// (TS PlayerOps.ts:1230-1239 @43e02957).
	SetPlayerOp(index int, text string, primary int)

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

	// RequestLogout flags the player for tick-loop logout processing.
	// Mirrors TS PlayerOps.ts:622-624 (P_LOGOUT) which sets
	// activePlayer.requestLogout = true. The processLogouts loop
	// (modules/world/tick.go) consumes the flag and tears the session
	// down at the next tick boundary.
	RequestLogout()

	// StopAction clears the current interaction target + pending action.
	// Matches TS Player.stopAction().
	StopAction()

	// HasInteraction reports whether the player has a current interaction
	// target (i.e., `target != nil`). Used by BUSY2 (opcode 2119). Mirrors
	// TS Player.hasInteraction at Engine-TS/.../PathingEntity.ts. NAI-120
	// Bundle 2B.
	HasInteraction() bool

	// HasWaypoints reports whether the player has waypoints queued
	// (waypointIndex >= 0). Used by BUSY2 (opcode 2119). Mirrors TS
	// Player.hasWaypoints. NAI-120 Bundle 2B.
	HasWaypoints() bool

	// Busy reports whether the player cannot accept new interactions —
	// either delayed (suspended by script delay) or has a main/chat modal
	// open. Mirrors TS Player.busy() at Engine-TS/.../Player.ts:801-803.
	// Used by BUSY (PlayerOps.ts:893-895). NAI-163 B0.
	Busy() bool

	// LoggingOut reports whether the player is in the logout-in-progress
	// state (TS Player.loggingOut field). Distinct from delayed/modal/
	// interaction state — set by the logout pipeline before final cleanup.
	// Used by BUSY (PlayerOps.ts:893-895). NAI-163 B0.
	LoggingOut() bool

	// QueueWaypoint clears any existing path and sets a single
	// destination on the active player. Mirrors TS Player.queueWaypoint
	// (Engine-TS PathingEntity.queueWaypoint). NAI-115 T7: used by
	// P_OPOBJ to walk the player toward the active obj's tile before
	// SCRIPT-anchoring the interaction.
	QueueWaypoint(x, z int)

	// InOperableDistance reports whether the active player is currently
	// within operable reach of the given active loc. Mirrors TS
	// Player.inOperableDistance (Engine-TS Player.ts) consumed by
	// P_OPLOC (PlayerOps.ts:396-398) to gate the queueWaypoint dispatch
	// when the player has not yet reached the loc. Production impl in
	// modules/world delegates to reach.Reached with the loc's
	// Width/Length/Angle/Shape/ForceApproach; non-*entity.Loc inputs
	// (test doubles) return true so the queueWaypoint dispatch is
	// suppressed.
	InOperableDistance(loc ActiveLoc) bool

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

	// CamShake sends a CAM_SHAKE wire packet to the client. Direct-write
	// (no accumulator); siblings the existing CamReset shape. Called by
	// the CAM_SHAKE script opcode for cutscene camera shake.
	CamShake(axis, random, amplitude, rate int)

	// CamMoveTo and CamLookAt buffer a deferred zone-relative camera packet
	// onto Player.cameraPackets. The packet is drained at the top of
	// updateBuildArea, where (camX, camZ) is converted to (localX, localZ)
	// against the player's freshly-rebuilt originX/originZ. kind is
	// 0 (moveto) or 1 (lookat). Mirrors TS PlayerOps.ts:206-218 +
	// NetworkPlayer.ts:244-253.
	CamMoveTo(camX, camZ, height, rate, rate2 int)
	CamLookAt(camX, camZ, height, rate, rate2 int)

	// HintNpc directs the client to render a hint arrow pointing at the
	// NPC with the given nid (slot id). Mirrors TS Player.hintNpc at
	// Player.ts:2174-2176, which writes a HintArrow(type=1) packet.
	// Called by the HINT_NPC (opcode 2032) handler.
	HintNpc(nid int)

	// HintCoord directs the client to render a hint arrow at the (x, z) tile
	// with the given offset (2..6, sub-tile arrow position) and height.
	// Mirrors TS Player.hintTile at Player.ts:2178-2180; called by the
	// HINT_COORD (opcode 2031) handler. NAI-39.
	HintCoord(offset, x, z, height int)

	// HintPlayer directs the client to render a hint arrow pointing at the
	// player with the given pid. Mirrors TS Player.hintPlayer at
	// Player.ts:2181-2183 (244); called by the HINT_PLAYER (opcode 2033) handler.
	// NAI-39.
	HintPlayer(pid int)

	// HintStop directs the client to clear any active hint arrow. Mirrors
	// TS Player.stopHint at Player.ts:2186-2188; called by the HINT_STOP
	// (opcode 2034) handler. NAI-39.
	HintStop()

	// Slot returns the player's pid (protocol identity). Name kept for the
	// shared entity-interface shape (Npc's Slot() returns nid). Mirrors TS
	// Player.pid at the 244 pin. Consumed by HINT_PL, which reads
	// activePlayer2.pid. NAI-39.
	Slot() int

	// StaffModLevel returns the player's staff moderation level.
	// 0 for regular players; >0 for mods/admins. Used by STAFFMODLEVEL
	// opcode to gate mod-only behaviour. Matches the rsbuf.PlayerSource
	// signature so *Player can satisfy both interfaces without a
	// duplicate method.
	StaffModLevel() int32

	// UID returns the player's per-session composeUID(username37, slot)
	// hash, computed locally at login — NOT the DB account id (that is
	// AccountID(), below). Used by the UID script opcode and p_finduid to
	// re-acquire / identify a specific player instance (TS getPlayerByUid).
	UID() int

	// AccountID returns the persistent DB account.id (int64) captured
	// from the login RPC. Distinct from UID() (which is the per-session
	// composeUID(username37, slot) hash). Used as the partition key on
	// telemetry envelopes (e.g. WealthEnvelope.AccountId). Returns 0 for
	// players whose session bypassed the login bridge. NAI-Phase2.
	AccountID() int64

	// RecipientSession returns this player's per-login session UUID when a
	// client is attached, else "disconnected". Used when this player is
	// the COUNTERPARTY of a wealth event. Mirrors TS InvOps.ts:446
	// `isClientConnected(toPlayer) ? toPlayer.client.uuid : 'disconnected'`
	// (single-Player-type adaptation documented at the rev-244 B3
	// account_id row; seam added in B4).
	RecipientSession() string

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

	// SetInteractionScriptNpcT anchors the player on `npc` with trigger
	// ApNpcT as a script-queued interaction (TS Interaction.SCRIPT) and
	// stores `spellCom` as the targetSubject.com (the UI component id of
	// the spell being cast). Matches TS PlayerOps.ts:417-421 (P_OPNPCT)
	// terminal setInteraction call. NAI-120 Bundle 2B.
	SetInteractionScriptNpcT(npc ActiveNpc, spellCom int)

	// SetInteractionScriptPlayer anchors the player on `player2` (a secondary
	// active player) with trigger ApPlayer<op> as a script-queued interaction
	// (TS Interaction.SCRIPT). op is 1-indexed (1..5; engine fire-path
	// supports 1..4 — see modules/world/player_interaction_trigger.go's
	// apPlayerTriggerForOp). Matches TS PlayerOps.ts:1009-1020 (P_OPPLAYER)
	// terminal setInteraction. NAI-120 Bundle 2B.
	SetInteractionScriptPlayer(player2 ActivePlayer, op int)

	// SetInteractionScriptObj anchors the player on `obj` with trigger
	// APOBJ<op> as a script-queued interaction (TS Interaction.SCRIPT).
	// Mirrors TS Player.setInteraction
	// (Interaction.SCRIPT, obj, ServerTriggerType.APOBJ1 + (op-1))
	// at PlayerOps.ts:990-1006. NAI-115 T7.
	SetInteractionScriptObj(obj ActiveObj, op int)

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
	// (*Player).PlayAnim suppresses in-engine animation requests per TS
	// Player.ts:1842 (NAI-56).
	SetAnimProtect(v int)

	// WalkTrigger returns the active player's queued walktrigger script
	// id, or -1 if none. Read by GETWALKTRIGGER (opcode 2118) and by
	// (*Player).processWalktrigger before firing. Mirrors TS
	// Player.walktrigger getter at Player.ts:1057-1070.
	WalkTrigger() int

	// SetWalkTrigger writes the queued walktrigger script id. -1 clears.
	// Written by WALKTRIGGER (opcode 2095); also written by
	// (*Player).processWalktrigger to -1 immediately before script
	// dispatch (TS clear-before-check semantics). Mirrors TS
	// Player.walktrigger setter at PlayerOps.ts:1035-1037.
	SetWalkTrigger(scriptID int)

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
	// the client's IfPlayerDesign inbound packet (character-design recustomise)
	// is permitted to apply. Mirrors TS Player.allowDesign
	// (Engine-TS/src/engine/entity/Player.ts:323). The handler coerces the
	// popped int via v==1 before calling. Reader path (IfPlayerDesignHandler)
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

	// PlaySynth sends a synthesized sound effect to the client. Called
	// by the SOUND_SYNTH script opcode (PlayerOps.ts:466-474). No name
	// normalization, no PRELOADED lookup, no validation — TS handler
	// gates only on lowMemory; the script-handler layer applies that
	// gate. Implementation encodes p2(synth) p1(loops) p2(delay) and
	// writes OpSynthSound.
	PlaySynth(synth, loops, delay int)

	// NAI-47: SETIDKIT appearance mutation.

	// Gender returns the player's gender (0=male, 1=female). Used by SETIDKIT
	// to determine the body-part slot offset (female slots = type − 7).
	// Mirrors TS state.activePlayer.gender at PlayerOps.ts:1073.
	Gender() int

	// Members returns whether the player has a members account. Backed by
	// the per-player members field set from the login RPC. Mirrors TS
	// Player.members consumed by PlayerOps.ts:1212 PLAYERMEMBER.
	Members() bool

	// AddHeroPoints credits `amount` to `playerUID` on the player's
	// hero-point ledger. Mirrors TS Player.heroPoints.addHero(...) at
	// PlayerOps.ts:1167 (BOTH_HEROPOINTS recipient). Parallel to
	// ActiveNpc.AddHeroPoints. NAI-127 Bundle 1.
	AddHeroPoints(playerUID, amount int)

	// TopContributor returns the playerUID with the largest HeroPoints
	// credit on this player's ledger, or 0 if the ledger is empty. Used
	// by FINDHERO (PlayerOps.ts:1138-1154). NAI-127 Bundle 1.
	TopContributor() int

	// HeroPointsClear resets the player's hero-point contributor ledger.
	// Called by STAT_ADD / STAT_BOOST / STAT_HEAL on the HP-full branch
	// (PlayerOps.ts:513-515, :552-554, :609-611). NAI-120 Bundle 2D follow-up.
	HeroPointsClear()

	// ChangeStat fires the [changestat,<skill>] trigger for the given
	// stat slot when a cache script is registered for that exact stat
	// (or its category, or globally via the 3-level fallback in
	// GetByTrigger). Silent no-op if no script is registered.
	//
	// Called by STAT_ADD / STAT_SUB / STAT_BOOST / STAT_DRAIN / STAT_HEAL
	// after SetCurLevel when the PRE-CLAMP computed value differs from
	// the prior current level — matches TS PlayerOps.ts:516-518, :534-536,
	// :555-557, :572-574, :613-615 `if (added !== current) player.changeStat(stat)`.
	// The pre-clamp predicate means a 255→255 capped boost still fires
	// if the unclamped value differs.
	//
	// Also called from AddXP's level-up branch (mirrors TS Player.ts:1772);
	// that call site stays inside the *Player impl, not via this interface.
	ChangeStat(id int)

	// SetPreventLogout records an anti-log message and absolute-tick
	// deadline. Used by P_PREVENTLOGOUT (PlayerOps.ts:626-630). The
	// caller computes `untilTick = currentTick + popped-ticks` —
	// matches TS `World.currentTick + check(popInt(), NumberNotNull)`.
	// NAI-127 Bundle 2.
	SetPreventLogout(message string, untilTick int)

	// ApplyDamage applies `amount` damage of `dmgType` to this player.
	// Used by DAMAGE (PlayerOps.ts:768-779). NAI-127 Bundle 2.
	ApplyDamage(amount, dmgType int)

	// SetBodyPart writes body[slot] = idkit. Called by SETIDKIT after slot
	// computation. Does NOT flip MaskAppearance — the script must call
	// BUILDAPPEARANCE separately (TS pattern: SETIDKIT then BUILDAPPEARANCE).
	// Mirrors TS state.activePlayer.body[slot] = idkType.id at PlayerOps.ts:1079.
	SetBodyPart(slot, idkit int)

	// SetColorPart writes colors[slot] = color. Called by SETIDKIT for the
	// color slot that corresponds to the body-part type (0/1→0, 2/3→1, 5→2,
	// 6→3; type 4 has no color write). Mirrors TS state.activePlayer.colors
	// at PlayerOps.ts:1102.
	SetColorPart(slot, color int)

	// SetGender rewrites the player's 7-slot body[] idkit array via the
	// MALE_FEMALE / FEMALE_MALE lookup maps and writes the gender field.
	// Called by SETGENDER after checkGender pre-validates v ∈ [0, 1].
	// Does NOT flip MaskAppearance — TS pattern requires a subsequent
	// BUILDAPPEARANCE for the change to reach the client (mirrors
	// SETIDKIT/SETSKINCOLOUR deferred-rebuild precedent).
	// Mirrors TS PlayerOps.ts:1104-1118.
	SetGender(gender int)

	// AddSessionLog pushes a session-log entry onto the server-level
	// per-tick buffer. Mirrors TS Player.addSessionLog (Player.ts:629-631).
	// eventType is the LoggerEventType numeric value (0=ENGINE, 1=WEALTH,
	// 2=MODERATOR, 3=ADVENTURE — see modules/world/session_log.go for
	// the typed constants). Variadic args are space-joined per TS quirk.
	// Wired by NAI-74.
	AddSessionLog(eventType int, message string, args ...string)

	// UnlinkQueuedScript drops queued fresh-run requests whose script
	// resolves to scriptID. Mirrors TS Player.unlinkQueuedScript with
	// the default NORMAL arm (walks queue + weakQueue; engineQueue
	// untouched). Backing impl at modules/world/player_script.go.
	// NAI-161 T3 — wired by CLEARQUEUE (OpClearQueue, PlayerOps.ts:1060-1063
	// at pin 9aadcec4).
	UnlinkQueuedScript(scriptID int)

	// QueueCount returns the count of queued requests whose script
	// resolves to scriptID. Mirrors TS GETQUEUE iteration over BOTH
	// queue and weakQueue (PlayerOps.ts:919-928 at pin 9aadcec4); the
	// unified p.queue holds both, so the single loop covers them.
	// Backing impl at modules/world/player_script.go. NAI-161 T3 —
	// wired by GETQUEUE (OpGetQueue).
	QueueCount(scriptID int) int

	// NAI-162 B1: trivial-handler sweep #4 widenings.

	// LastLoginInfo emits the LAST_LOGIN_INFO server packet with the
	// previous-login timestamp and IP.
	LastLoginInfo()

	// InvTotalParamStack sums slot.count × objType.Params[paramID]
	// across non-empty inv slots. Mirrors TS Player.invTotalParamStack.
	InvTotalParamStack(invID, paramID int) int

	// AddWealthEvent appends a wealth-affecting event to the server-side
	// log. Concrete body lands in B2.2 (NAI-162).
	AddWealthEvent(evt WealthEvent)
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
	// SetNpcStat writes `level` into the NPC's current (boosted) stat slot
	// `stat`. OOB stats are dropped silently (impl bounds-checks against
	// objtype.NpcStatCount=6). Used by NPC_STATADD / NPC_STATSUB. Mirrors TS
	// `npc.levels[stat] = ...` in NpcOps.ts:492-518. NAI-120 Bundle 2C.
	SetNpcStat(stat, level int)
	NpcCategory() int

	// NpcWidth returns the NPC's tile-footprint width. NPCs are square
	// in practice (NpcType.Size populates both width and length per
	// modules/world/npc.go:233-239), but the interface keeps them
	// distinct to mirror TS Npc.width/length semantics for
	// CoordGrid.distanceTo (read by NPC_RANGE per handlers_npc.go).
	NpcWidth() int

	// NpcLength returns the NPC's tile-footprint length. See NpcWidth.
	NpcLength() int

	NpcUID() int // (typeId << 16) | nid
	// Nid returns the NPC slot id (the low 16 bits of NpcUID). Used by
	// NPC-targeted player-bound packets like HintArrow that reference
	// the NPC by slot rather than by packed UID.
	Nid() int

	// LastMovement returns the NPC's TS-PathingEntity.lastMovement value
	// (set to currentTick + 1 at the end of any tick the NPC stepped, else
	// 0). Read by NPC_ARRIVEDELAY (NpcOps.ts:542-555). Mirrors
	// ActivePlayer.LastMovement.
	LastMovement() int

	// Respawnrate returns the active NPC type's respawnrate config field
	// (objtype.NpcType.RespawnRate, uint16 widened to int). Read by NPC_DEL
	// — passed as the duration arg to script.WorldVars.RemoveNpc. Mirrors
	// TS check(state.activeNpc.type, NpcTypeValid).respawnrate at
	// NpcOps.ts:79.
	Respawnrate() int

	NpcVarN(id int) int32
	SetNpcVarN(id int, val int32)
	// NpcVarNString reads the per-NPC STRING-typed var at id. Returns
	// "" defensively for OOB or never-written ids. Mirrors TS
	// Npc.getVar dispatched on STRING type.
	NpcVarNString(id int) string

	// SetNpcVarNString writes the per-NPC STRING-typed var at id. OOB
	// silently dropped (slice sized to varnTypes.Configs at spawn).
	SetNpcVarNString(id int, val string)
	// Say buffers text as the NPC's speech bubble for the current tick,
	// flagging NpcMaskSay so the NPC-info encoder emits it. Empty text is
	// allowed (produces an empty bubble that clears itself next tick via
	// ResetMasks).
	Say(text []byte)

	// Animate schedules sequence `id` with client-side `delay` on the NPC's
	// primary animation slot this tick. id = -1 clears.
	Animate(id, delay int)

	// PlaySpotAnim schedules a spotanim graphic on the NPC for this tick
	// at the given height with the given client-side delay. Used by
	// SPOTANIM_NPC (opcode 2542). Mirrors TS Npc.spotanim. NAI-120 Bundle 2C.
	PlaySpotAnim(id, height, delay int)

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
	// reset=false, dispatched from NPC_CHANGETYPE_KEEPALL (opcode 2505,
	// TS NpcOps.ts:465-471). No-op when duration < 1 OR when the NPC is dead.
	ChangeTypeKeepAll(newType, duration int)

	// Damage applies `amount` damage of `dmgType` to the NPC this tick,
	// flagging NpcMaskDamage. Decrements curHP (clamped at 0). Does NOT
	// trigger death handling or auto-retaliate, mirroring TS Npc.applyDamage
	// (Npc.ts:472-485) — death is content-script driven in TS too. Content
	// scripts check NPC_STAT(0)<=0 and dispatch npc_del to wake the engine
	// lifecycle path (modules/world/npc_ai.go).
	Damage(amount, dmgType int)

	// StoreActiveScript saves a NpcSuspended ScriptState so Npc.turn()
	// can resume it when the NPC's delay expires. Mirrors
	// ActivePlayer.StoreActiveScript at active.go:22-24.
	StoreActiveScript(state *ScriptState)

	// ClearActiveScript discards any stored ScriptState. Called after
	// Finished/Aborted runs. Mirrors ActivePlayer.ClearActiveScript.
	ClearActiveScript()

	// OnScriptFinishedOrAborted is the post-Execute tail for the
	// Finished or Aborted execution states. Nulls activeScript only
	// if state matches the npc's currently stored value. Mirrors TS
	// Npc.executeScript tail (Npc.ts:226-228). NPCs have no modals.
	//
	// NAI-54.
	OnScriptFinishedOrAborted(state *ScriptState)

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
	// lastIntArg is stored on the queue entry and copied into
	// state.LastInt at fire time (TS Npc.ts:554-555).
	EnqueueScriptForTrigger(trigger ServerTriggerType, delay int, lastIntArg int)

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
	// active NPC. Called by NPC_WALKTRIGGER (opcode 2533). Range
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
	// DEVIATION NAI-34 vs TS PathingEntity.teleport — closure status:
	//
	// CLOSED:
	//   - D1 (level clamp [0, 3]) — NAI-36-T7, both entities.
	//   - D2 (unallocated-zone reject) — NAI-36-T7, both entities.
	//   - D5-Player (level-change INSTANT/jump branch) — NAI-36-T7.
	//   - Player.Teleport order divergence (refresh-then-flag) — NAI-36-T7.
	//   - D3-Player + D3-NPC (focus call) — NAI-65.
	//   - D4-Player (lastStepX = x-1; lastStepZ = z) — NAI-65.
	//
	// RESIDUAL (permanent dead-API skips per NAI-66):
	//   - D4-NPC: no lastStepX/Z fields on Npc; TS itself has no
	//     NPC reader, so adding fields would be dead-API. Closure
	//     requires upstream-TS NPC consumer.
	//   - D5-NPC: no jump field on Npc; TS NPC encoders don't read
	//     it, and rsbuf upstream parity confirms (Rust npc.rs has no
	//     jump field). Closure requires upstream-TS NPC encoder
	//     consumer.
	//
	// NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ closed in NAI-66.
	//
	// See (n *Npc).Teleport doc comment in modules/world/npc_script.go
	Teleport(x, z, level int)

	// QueueWaypoint clears any existing path and sets a single destination
	// (level-implicit by current NPC level). Mirrors TS Npc.queueWaypoint
	// at Engine-TS/.../Npc.ts. Used by NPC_WALK (opcode 2543).
	QueueWaypoint(x, z int)

	// TargetOp returns the NPC's current targetOp/mode value (the field set
	// by NPC_SETMODE / interaction binding). Used by NPC_GETMODE (opcode 2520).
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

	// AddHeroPoints credits `amount` to `playerUID` on the NPC's hero-point
	// ledger. Used by NPC_HEROPOINTS (opcode 2521) to track damage
	// contributions for loot routing. amount < 1 is a no-op (TS short-circuit).
	// Mirrors TS Npc.heroPoints.addHero(...) at NpcOps.ts:479. NAI-120 Bundle 2D.
	AddHeroPoints(playerUID, amount int)

	// TopContributor returns the playerUID with the largest HeroPoints
	// credit on this NPC's ledger, or 0 if the ledger is empty. Used by
	// NPC_FINDHERO (NpcOps.ts:114-130) — TS uses hash64; goscape uses
	// int playerUID. The 0-empty-sentinel mirrors HeroPoints.TopContributor.
	TopContributor() int

	// TargetWithinMaxRange returns true if the NPC's current target is
	// inside the per-mode maxrange envelope (HUNT-distance + corner-quirk
	// adjustments). Mirrors TS Npc.targetWithinMaxRange — read by
	// NPC_INRANGE (NpcOps.ts:556-558). Backing impl at
	// modules/world/npc_interaction.go:591. Returns false defensively when
	// the NPC has no target (TS-equivalent). NAI-160 T7.
	TargetWithinMaxRange() bool

	// HeroPointsClear resets the NPC's hero-point contributor ledger.
	// NOTE: NPC_STATHEAL no longer calls this (244: branch deleted,
	// NpcOps.ts:240-252). Seam retained for forward-compat. NAI-162 B1.
	HeroPointsClear()
}

// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
	LocType() int              // returns the LocType ID (from packed Loc.CurrentInfo bitfield)
	Coords() (x, z, level int) // world position; consumed by LOC_COORD
	Angle() int                // rotation (0=west, 1=north, 2=east, 3=south); consumed by LOC_ANGLE
	Shape() int                // shape (0..22 valid range); consumed by LOC_SHAPE
	Layer() int                // shape's render layer (0..3); consumed by LOC_ADD same-layer search (NAI-86)
	Active() bool              // mirrors entity.Loc.IsActive (zone-managed); consumed by MAP_LOCADDUNSAFE WALL-only inactive-skip (NAI-114)
}

// ActiveObj is the surface that OBJ_* and AI_APOBJ/AI_OPOBJ handlers
// use to read obj state. Narrow by design — extend as future sub-specs
// wire more obj script opcodes.
type ActiveObj interface {
	ObjType() int                  // underlying ObjType id
	Coords() (x, z, level int)     // world position
	ObjCount() int                 // current stack size; consumed by OBJ_COUNT, OBJ_TAKEITEM (NAI-153)
	IsValidFor(playerUID int) bool // private-receiver + count>0 (NAI-153); see *entity.Obj.IsValidFor
	// IsRespawnLifecycle reports whether the obj is RESPAWN-lifecycle
	// (engine-spawned, comes back after a timer). Used by OBJ_TAKEITEM
	// to gate the respawn-duration arg passed to WorldVars.RemoveObj
	// per TS ObjOps.ts:156-160. NAI-178.
	IsRespawnLifecycle() bool

	// DropperAccountID returns the persistent account_id of the human
	// dropper, or 0 if the obj was NPC-spawned, world-spawned, or
	// respawned. Distinct from the visibility-window receiver UID
	// (ReceiverID). Required for cross-account drop+pickup attribution.
	DropperAccountID() int64
}
