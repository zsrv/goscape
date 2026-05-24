package script

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)

const (
	// StackCapacity is the maximum depth of the int and string stacks.
	// Exceeding it is a compiler bug and panics.
	StackCapacity = 1024

	// OpCountLimit caps opcode execution in a single Execute call to
	// prevent infinite loops from hanging the server. The runner aborts
	// when OpCount exceeds this (strict `>`), so up to 500_001 opcodes
	// execute — matching TS ScriptRunner.ts:144 (`opcount > 500_000`).
	OpCountLimit = 500_000

	// FrameCapacity is the maximum GOSUB nesting depth.
	FrameCapacity = 50
)

// LineValidator is the script-VM bridge for line-of-sight / line-of-walk
// checks during HuntAll-mode passesFilter. Methods mirror
// pkg/pathfinder/routefinder.LineValidator's surface — that struct
// satisfies this interface via Go's structural typing, so production
// wiring needs no adapter. Tests inject stubs. NAI-35-T3.
type LineValidator interface {
	HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool
	HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool
}

// PlayerLookup is the player-resolution surface for FINDUID / P_FINDUID
// (UID-keyed lookup) and for zone-rect player enumeration used by
// MAP_PLAYERCOUNT (NAI-35).
type PlayerLookup interface {
	// LookupPlayerByUID resolves a UID to an ActivePlayer if a player
	// with that UID is currently logged in. Returns nil on miss.
	// Handlers decide whether the result is usable: FINDUID accepts any
	// match; P_FINDUID additionally gates on CanAccess.
	LookupPlayerByUID(uid int) ActivePlayer

	// ZonePlayers returns all players in the zone at (level, zoneX, zoneZ),
	// filtered by IsValid via Zone.PlayersSafe. Mirrors NpcLookup.ZoneNpcs
	// shape (world coords, not zone-index; ZoneMap.Get masks internally).
	// Empty/nil slice on miss. Used by MAP_PLAYERCOUNT (NAI-35).
	ZonePlayers(level, zoneX, zoneZ int) []ActivePlayer
}

// WorldVars is the minimal surface that pkg/script needs from the
// hosting world to resolve PUSH_VARS / POP_VARS. Decouples the VM
// from concrete server types.
type WorldVars interface {
	VarsInt(id int) int32
	SetVarsInt(id int, val int32)
	VarsString(id int) string
	SetVarsString(id int, val string)

	// S5l: world-state queries used by MAP_CLOCK / PLAYERCOUNT.
	CurrentTick() int
	PlayerCount() int

	// World-config queries: MAP_MEMBERS / MAP_LIVE. Pushed as 0/1.
	MapMembers() int
	MapLive() int

	// IsMapBlocked reports whether the tile at (level, x, z) blocks
	// walking. Used by MAP_FINDSQUARE for candidate-square rejection.
	// Mirrors TS World.gameMap.isMapBlocked. NAI-35-T6.
	IsMapBlocked(level, x, z int) bool

	// IsFreeToPlay reports whether the tile at (x, z) is in an F2P zone.
	// Used by MAP_FINDSQUARE for free-world filtering. Mirrors TS
	// World.gameMap.isFreeToPlay. NAI-35-T6.
	IsFreeToPlay(x, z int) bool

	// IsMulti reports whether the tile at (level, x, z) is in a multi-combat
	// zone. Mirrors TS World.gameMap.isMulti at Engine-TS/.../GameMap.ts.
	// Used by MAP_MULTIWAY (opcode 1014).
	IsMulti(level, x, z int) bool

	// AnimMap broadcasts a tile-anchored spotanim event to every player in
	// the affected zone. Mirrors TS World.animMap at Engine-TS/.../World.ts.
	// Used by SPOTANIM_MAP (opcode 1020).
	AnimMap(level, x, z, spotanim, height, delay int)

	// MapProjAnim broadcasts a projectile event from (level, srcX, srcZ)
	// to (dstX, dstZ). target encodes the receiver: 0 = none (MAP→MAP),
	// npc.nid+1 = NPC target, -player.slot-1 = player target.
	// srcHeight/dstHeight are pre-scaled by the handler (×4).
	// Mirrors TS World.mapProjAnim. Used by PROJANIM_MAP, PROJANIM_NPC,
	// PROJANIM_PL. NAI-150.
	MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim,
		srcHeight, dstHeight, startDelay, endDelay, peak, arc int)

	// RemoveObj despawns / removes the given obj from its zone. Mirrors
	// TS World.removeObj. duration drives the RESPAWN-after-pickup
	// re-spawn timer when obj.IsRespawnLifecycle (else untracks). Used by
	// OBJ_DEL, OBJ_TAKEITEM.
	RemoveObj(obj ActiveObj, duration int)

	// RemoveNpc removes the given NPC from the world. duration is passed
	// through to Server.removeNpc, which scales it by player count and
	// writes lifecycleTick (RESPAWN-lifecycle) or, on DESPAWN-lifecycle,
	// releases the registry slot and runs n.Cleanup() (NAI-19; see
	// modules/world/npc_registry.go). Mirrors TS World.removeNpc at
	// World.ts:1296-1319. Used by NPC_DEL.
	RemoveNpc(npc ActiveNpc, duration int)

	// AddNpcAt spawns a new despawn-lifecycle NPC of `typeID` at (level, x, z)
	// with the given despawn `duration` in ticks. duration=-1 means no
	// scheduled despawn (the caller is responsible for explicit removeNpc).
	// Returns the spawned ActiveNpc on success or an error if the NPC
	// registry is full or typeID is unknown. Mirrors TS NpcOps.ts:42-53
	// (NPC_ADD) + World.addNpc at World.ts:1258-1294. Routes through
	// (*Server).addNpc with firstSpawn=true, hard-setting
	// EntityLifeCycle.DESPAWN. NAI-163 B3.
	AddNpcAt(level, x, z, typeID, duration int) (ActiveNpc, error)

	// AddObj routes a ground-item spawn. receiverID is the owning
	// player's UID for caller-only drops, or zone.PublicReceiver (-1)
	// for broadcast. Returns the just-spawned ActiveObj so handlers can
	// write back to state.ActiveObj and pointerAdd(ActiveObj). Mirrors
	// TS World.addObj which returns the constructed Obj.
	// Used by OBJ_ADD, OBJ_ADDALL, INV_DROPSLOT.
	AddObj(level, x, z, typeID, count, duration, receiverID int, dropperAccountID int64) ActiveObj

	// EnqueueObjDelayed appends an INV_DROPITEM_DELAYED request to the
	// world's per-tick spawn-delay queue. The Obj is constructed at the
	// implementation side (worldVarsView in modules/world). Mirrors TS
	// World.objDelayedQueue.addTail at InvOps.ts:208. Used by INV_DROPITEM_DELAYED.
	EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int, dropperAccountID int64)

	// GetObj returns the first ground obj at (level, x, z) whose type
	// matches objId and is visible to the caller. receiverUID is the
	// player UID gating private-receiver visibility (NAI-153-D2 — goscape
	// uses player UID where TS uses hash64). Returns nil on miss. Mirrors
	// TS World.getObj consumed via OBJ_FIND. NAI-154.
	GetObj(level, x, z, objId, receiverUID int) ActiveObj

	// ZoneObjs returns every obj in the zone owning (level, zoneX, zoneZ),
	// in storage order, without per-tile or per-receiver filtering. The
	// zoneX/zoneZ params are coord-grid coords (not zone indices) — the
	// implementation does the >>3 shift internally. The caller
	// (OBJ_FINDNEXT) applies its own validity gates as needed. Mirrors TS
	// Zone.getAllObjsSafe(true) consumed by ObjIterator.generator
	// (ScriptIterators.ts:400). Empty/nil slice on miss. NAI-154.
	ZoneObjs(level, zoneX, zoneZ int) []ActiveObj

	// LookupPlayerByUID resolves a packed Player UID to the matching
	// ActivePlayer, or nil if no logged-in player has that UID. Used by
	// NPC_FINDHERO, FINDHERO, and DAMAGE. Mirrors TS World.getPlayerByUid
	// (PlayerOps.ts:773) / World.getPlayerByHash64 (PlayerOps.ts:1144,
	// NpcOps.ts:120).
	LookupPlayerByUID(uid int) ActivePlayer

	// LookupNpcBySlot resolves the NPC slot to its live ActiveNpc, or
	// nil if the slot is out of range / unoccupied. Slot-only — does
	// NOT verify the high-16 type bits, unlike NpcLookup.FindNpcByUID.
	// Mirrors TS World.getNpc(slot). Used by PROJANIM_NPC. NAI-150.
	LookupNpcBySlot(slot int) ActiveNpc

	// IsIndoors reports whether the tile at (x, z, level) carries the
	// FlagRoof bit in the global collision FlagMap. Used by MAP_INDOORS
	// (opcode 1010). Mirrors TS isIndoors (GameMap.ts:417-419).
	// NAI-162 B1.
	IsIndoors(x, z, level int) bool

	// MergeLoc routes a multi-tile loc merge to the zone owning the loc.
	// player is the requesting player (used to thread the player slot into
	// the LocMerge zone event). south, east, north, west are the four
	// edge-coord values matching TS Zone.mergeLoc parameter order
	// (se.z=south, se.x=east, nw.z=north, nw.x=west). Used by P_LOCMERGE
	// (opcode 2074). Mirrors TS World.mergeLoc at World.ts:1388-1391.
	// NAI-162 B2.
	MergeLoc(loc ActiveLoc, player ActivePlayer, startCycle, endCycle, south, east, north, west int)

	// NodeID returns the world's configured node ID (cfg.NodeID in the
	// production impl). Used as the `world_id` partition key on telemetry
	// envelopes emitted from script handlers (e.g. WealthEnvelope from
	// INV_DROPITEM). Mirrors the same value threaded through emission sites
	// in modules/world/ that read cfg.NodeID directly.
	NodeID() int
}

// InvLookup is the inventory resolution surface for INV_* handlers.
// Implementations route between player-owned and world-shared
// inventories based on InvType.Scope.
type InvLookup interface {
	// Get returns the inventory at typeID for the given active player,
	// or nil if the type is invalid or the player has no such inv.
	Get(self ActivePlayer, typeID int) *inventory.Inventory
}

// NpcLookup is the script→world bridge for NPC_FIND family opcodes. All
// methods return the matching NPC as script.ActiveNpc or nil when no
// match. Implementations iterate the world NPC registry; see
// serverNpcLookup (modules/world/npc_script_lookup.go) for the
// production impl.
//
// huntvis accepts HuntVisOff/LineOfSight/LineOfWalk (pkg/objtype.HuntVis*).
// The production impl (serverNpcLookup) applies LoS/LoW filtering per TS
// Distance-mode semantics (ScriptIterators.ts:348-352); callers validate
// via checkHuntVis upstream.
type NpcLookup interface {
	// FindClosestNpcByType: NPC_FIND semantics. Square-bounded by dist
	// from (level, x, z); filter by typeID; closest by euclidean-squared
	// with later-match-wins on ties.
	FindClosestNpcByType(level, x, z, dist, typeID, huntvis int) ActiveNpc

	// FindClosestNpcByCategory: NPC_FINDCAT semantics. Same shape as
	// FindClosestNpcByType but filter via NpcType.Category == cat.
	FindClosestNpcByCategory(level, x, z, dist, cat, huntvis int) ActiveNpc

	// FindNpcAtExactCoord: NPC_FINDEXACT semantics. Returns the first
	// NPC at exactly (level, x, z) whose type matches typeID, or nil.
	FindNpcAtExactCoord(level, x, z, typeID int) ActiveNpc

	// ZoneNpcs returns all NPCs subscribed to the zone at (level, zoneX, zoneZ),
	// filtered by IsValid. Mirrors TS Zone.getAllNpcsSafe(true) consumed by
	// NpcIterator.generator (ScriptIterators.ts:330,341). zoneX/zoneZ are
	// coord-grid coords (not zone indices); the impl converts via
	// ZoneMap.Get which masks internally. Empty/nil slice on miss.
	// No error path. Used by NPC_FINDALL/FINDALLANY/FINDALLZONE iterators.
	ZoneNpcs(level, zoneX, zoneZ int) []ActiveNpc

	// FindNpcByUID resolves a packed NPC UID `(typeId<<16)|nid` to the
	// matching ActiveNpc. The lookup verifies BOTH the slot has a live
	// NPC AND the NPC's typeId equals the high-16-bit type. Returns nil
	// on miss. Mirrors TS NpcOps.ts:26-40 (NPC_FINDUID). NAI-120 Bundle 2A.
	FindNpcByUID(uid int) ActiveNpc
}

// Frame holds a suspended call frame for GOSUB / RETURN.
type Frame struct {
	Script       *ScriptFile
	PC           int
	IntLocals    []int
	StringLocals []string
}

// ScriptState is the mutable execution context for one running script.
type ScriptState struct {
	Script   *ScriptFile
	Provider *Provider // for GOSUB target lookup by LookupKey
	World    WorldVars // for PUSH_VARS / POP_VARS; nil if the script uses no VARS

	// Configs is the config lookup surface. Callers set this after Init
	// if the script uses config-read opcodes (OC_*, NC_*, LC_*, ENUM,
	// STRUCT_PARAM).
	Configs Configs

	// Inv is the inventory resolution surface. Callers set this after
	// Init if the script uses INV_* opcodes.
	Inv InvLookup

	// PlayerLookup is the player-resolution surface for FINDUID / P_FINDUID.
	// Callers set this after Init if the script uses UID-keyed player
	// ops. Nil disables the lookup (handlers degrade to "not found").
	PlayerLookup PlayerLookup

	// Npcs is the NPC-lookup surface for NPC_FIND family opcodes.
	// Callers set this after Init if the script uses find opcodes.
	// Nil disables (handlers treat a nil surface as "no match", push 0).
	Npcs NpcLookup

	// LocOps is the script→world mutator surface for LOC_CHANGE / LOC_ADD /
	// LOC_DEL / LOC_ANIM (NAI-86). Callers set this after Init if the
	// script uses loc mutator opcodes. Nil disables (handlers return an
	// explicit error).
	LocOps LocOps

	// LineValidator is the LoS/LoW bridge for Distance + HuntAll iterator
	// passesFilter (NAI-35-T3, extended to Distance at NAI-33-D1 retire
	// per TS ScriptIterators.ts:348-352). Nil = no validator wired (both
	// modes pessimistically allow). Production sets this from
	// gamemap.Pathfinder.LineValidator via modules/world/script.go.
	LineValidator LineValidator

	// NodeDebug is the per-state instrumentation gate. Production wiring
	// reads from cfg.NodeDebug at script construction sites; pkg/script
	// itself never reads cfg. Zero-value (false) preserves silence in
	// every existing &ScriptState{} test fixture without modification.
	// NAI-90: gates the handlePTeleport frame T (handlers_player.go).
	NodeDebug bool

	// Log is an optional logger for diagnostic instrumentation (e.g.
	// gateway probes). Wired by goscape state-builders from Server.log;
	// may be nil for pkg/script-internal tests. Always nil-check before
	// use. NAI-128 Stage 3.
	Log *slog.Logger

	PC      int
	OpCount int

	// Trigger is the ServerTriggerType that fired this script. Set by
	// the script-construction sites in modules/world (buildPlayerScriptState,
	// buildNpcScriptState, and the in-line fire* helpers in
	// interaction_trigger.go / player_interaction_trigger.go); consumed by
	// the LAST_* opcode allowlists at handlers_dialog.go to enforce the
	// per-opcode "is not safe to use in this trigger" gate. Zero-value
	// (TriggerProc=0) is NOT in any LAST_* allowlist, so test fixtures
	// that omit Trigger fall through to "throw" cleanly. Mirrors TS
	// ScriptState.trigger (ScriptState.ts) — populated by
	// ScriptRunner.init's `state.trigger = trigger` and read at
	// PlayerOps.ts:259-340,1026-1033.
	Trigger ServerTriggerType

	// LastInt is the int value injected by a resume event (e.g. the count
	// from RESUME_P_COUNTDIALOG). Scripts read it via LAST_INT opcode.
	LastInt int

	// Timespent is the start of a script-side stopwatch set by the
	// TIMESPENT opcode and read by GETTIMESPENT to compute elapsed time.
	// Mirrors TS ScriptState.timespent (DebugOps.ts:13-27) which stores
	// `performance.now()` and subtracts on read. Zero value (unset) makes
	// GETTIMESPENT report the elapsed since Unix epoch, matching TS's
	// behavior when timespent is never assigned (undefined - now ~= NaN
	// in JS, but goscape returns a deterministic large number).
	Timespent time.Time

	Execution Execution

	IntStack    []int
	StringStack []string
	ISP         int // int stack pointer: next free slot
	SSP         int // string stack pointer: next free slot

	IntLocals    []int
	StringLocals []string

	Frames  []Frame
	FrameSP int // frame stack pointer: next free slot

	Pointers Pointer
	Self     ActivePlayer
	Target   ActivePlayer
	// Self2 is the secondary active-player slot consumed by HINT_PL and
	// the player→player OPPLAYER<N>/APPLAYER<N> trigger family.
	// Mirrors TS ScriptState._activePlayer2 (ScriptState.ts:80).
	// Production producer: fireOpTriggerPlayer / fireApTriggerPlayer
	// (modules/world/player_interaction_trigger.go) wired by NAI-40.
	Self2 ActivePlayer

	// ActiveNpc is the NPC that NPC_* and VARN ops target. Nil if no
	// NPC is bound to this script's execution. Set by callers (test
	// fixtures, OPNPC trigger routing).
	ActiveNpc ActiveNpc

	// ActiveLoc is the Loc that LOC_* ops target. Nil if no Loc is
	// bound to this script's execution. Set by callers (test fixtures,
	// OPLOC trigger routing). Type is the package-local ActiveLoc
	// interface (currently empty — handlers_loc.go will populate
	// methods in a follow-up sub-spec).
	ActiveLoc ActiveLoc

	// OtherActiveLoc is the secondary Loc slot, parallel to OtherActiveNpc.
	// Set by LOC_FINDNEXT (and any future LOC_FIND family handler) when
	// the bytecode IntOperand is 1 (.loc2 syntax). NAI-119.
	//
	// NAI-119-D CLOSED: LOC_* read/mutate handlers (and PlayerOps P_OPLOC/
	// P_LOCMERGE) now resolve `.loc`/`.loc2` operand-aware via s.activeLoc()
	// (mirrors TS ScriptState.activeLoc getter, ScriptState.ts:269-279), so
	// this slot is read by operand=1 invocations. Write-side stays in
	// setActiveLocSlot. (NPC_SETMODE's OPLOC target still reads the primary
	// directly — matching TS NpcOps.ts:216 `state._activeLoc`.)
	OtherActiveLoc ActiveLoc

	// ActiveObj is the Obj that OBJ_* and AI_*OBJ handlers target. Nil
	// if no Obj is bound. NAI-11.
	ActiveObj ActiveObj

	// OtherActiveObj is the secondary Obj slot, parallel to OtherActiveLoc
	// (NAI-119) and OtherActiveNpc (NAI-11). Written via setActiveObjSlot by
	// every operand-aware OBJ writeback when the bytecode IntOperand is 1
	// (.obj2 syntax): OBJ_FIND / OBJ_FINDNEXT, and (since L20) the spawn
	// handlers OBJ_ADD / OBJ_ADDALL / INV_DROPSLOT / INV_DROPITEM. NAI-154.
	//
	// NAI-154-D CLOSED: OBJ_* read/mutate handlers (and PlayerOps P_OPOBJ)
	// now resolve `.obj`/`.obj2` operand-aware via s.activeObj() (mirrors TS
	// ScriptState.activeObj getter, ScriptState.ts:289-299), so this slot is
	// read by operand=1 invocations. Write-side stays in setActiveObjSlot.
	// (NPC_SETMODE's OPOBJ target still reads the primary directly — matching
	// TS NpcOps.ts:214 `state._activeObj`.)
	OtherActiveObj ActiveObj

	// OtherActiveNpc is the secondary NPC slot used by AI_*NPC handlers
	// when `Self` (an NPC) targets another NPC. NAI-11.
	OtherActiveNpc ActiveNpc

	// npcIterator holds the active NPC_FIND iterator state. Set by
	// FINDALL/FINDALLANY/FINDALLZONE; consumed by FINDNEXT. Lifetime is
	// single-tick — Stale() check enforced at FINDNEXT against
	// s.World.CurrentTick(). Nil = no active iterator. Mirrors TS
	// ScriptState.npcIterator (ScriptState.ts:125). Lowercase = package-
	// private; handlers in pkg/script access directly. NAI-33.
	npcIterator *NpcIterator

	// locIterator holds the active LOC_FIND iterator state. Set by
	// LOC_FINDALLZONE; consumed by LOC_FINDNEXT. Lifetime is single-tick
	// — Stale() check enforced at FINDNEXT against s.World.CurrentTick().
	// Nil = no active iterator. Mirrors TS ScriptState.locIterator. NAI-119.
	locIterator *LocIterator

	// objIterator holds the active OBJ_FIND iterator state. Set by
	// OBJ_FINDALLZONE; consumed by OBJ_FINDNEXT. Lifetime is single-tick —
	// Stale() check enforced at FINDNEXT against s.World.CurrentTick().
	// Nil = no active iterator. Mirrors TS ScriptState.objIterator. NAI-154.
	objIterator *ObjIterator

	// playerIterator holds the active player-iterator state. Set by
	// HUNTALL; consumed by HUNTNEXT (T5). Single-tick lifetime — Stale()
	// check at HUNTNEXT against s.World.CurrentTick(). Nil = no active
	// iterator. NAI-35-T4.
	playerIterator *PlayerIterator

	// DB cursor state — populated by DB_LISTALL* (and DB_FIND*, deferred to a
	// later sub-spec); consumed by DB_FINDNEXT, DB_FINDBYINDEX. DbTable == nil
	// means no LISTALL/FIND has selected a table yet; DbRow is the cursor index
	// into DbRowQuery (-1 after LISTALL before the first FINDNEXT advance).
	DbTable    *objtype.DbTableType
	DbRow      int
	DbRowQuery []int

	// Arrays holds script-local int[] arrays defined via DEFINE_ARRAY.
	// Index = array slot (0..4); length set at DEFINE_ARRAY, fixed
	// thereafter. A nil slice at a slot means "undefined"; OOB reads
	// return 0 and OOB writes are dropped.
	Arrays [5][]int32

	// SplitPages holds the per-page, per-line wrapped chat-dialog text
	// produced by SPLIT_INIT and consumed by SPLIT_GET / SPLIT_PAGECOUNT
	// / SPLIT_LINECOUNT. Nil before any SPLIT_INIT call. Each call to
	// SPLIT_INIT replaces (not appends) the slice. Mirrors TS
	// ScriptState.splitPages (StringOps.ts:91). NAI-75.
	SplitPages [][]string

	// SplitMesanim is the MesanimType id parsed from a leading <p,name>
	// prefix on a SPLIT_INIT text argument. -1 when no prefix or when
	// the name does not resolve to a known MesanimType (NAI-179 retired
	// the unconditional -1 deviation; MesanimType resolution now live).
	SplitMesanim int32
}

// PushInt pushes v onto the int stack, normalised through signed-int32
// (mirroring TS Numbers.ts:7 `toInt32(num) = num | 0`). RuneScript ints
// are 32-bit; both varp/varn storage (`[]int32`) and the wire protocol
// use signed-int32 representation, so any value pushed must share that
// representation to round-trip through storage cleanly.
//
// Without this cast, high-bit-set values (e.g. `composeUID` results
// >=0x80000000, which apply to ~50% of usernames) would compare unequal
// after a write+read cycle: the fresh push stays as Go int 2147483649
// while the varn read returns int(int32(stored)) = -2147483647. That
// mismatch broke `%npc_aggressive_player == uid` in the combat-check
// proc and surfaced as "Someone else is fighting that." after one hit.
//
// Panics if the stack is full (programming error / compiler bug).
func (s *ScriptState) PushInt(v int) {
	if s.ISP >= StackCapacity {
		panic(fmt.Sprintf("script: int stack overflow at pc=%d in %q", s.PC, s.Script.Name))
	}
	s.IntStack[s.ISP] = int(int32(v))
	s.ISP++
}

// PopInt pops and returns the top of the int stack.
// Returns 0 on underflow (matches TS toInt32(null) === 0 behaviour).
func (s *ScriptState) PopInt() int {
	if s.ISP <= 0 {
		return 0
	}
	s.ISP--
	return s.IntStack[s.ISP]
}

// PushString pushes v onto the string stack.
// Panics if the stack is full.
func (s *ScriptState) PushString(v string) {
	if s.SSP >= StackCapacity {
		panic(fmt.Sprintf("script: string stack overflow at pc=%d in %q", s.PC, s.Script.Name))
	}
	s.StringStack[s.SSP] = v
	s.SSP++
}

// PopString pops and returns the top of the string stack.
// Returns "" on underflow (matches TS popString returning ” on null).
func (s *ScriptState) PopString() string {
	if s.SSP <= 0 {
		return ""
	}
	s.SSP--
	return s.StringStack[s.SSP]
}

// GosubCall saves the current frame onto the frame stack and sets up execution
// of target. intArgs and stringArgs are pre-popped by the caller in reverse
// order so that intArgs[0] is the first argument.
//
// The new frame's PC is set to -1 so that the runner's post-handler PC++
// lands at 0 (the first instruction of the callee). This mirrors TS
// ScriptState.setupNewScript setting pc = -1 before the loop's ++pc.
func (s *ScriptState) GosubCall(target *ScriptFile, intArgs []int, stringArgs []string) {
	if s.FrameSP >= FrameCapacity {
		panic(fmt.Sprintf("script: frame stack overflow in %q", s.Script.Name))
	}

	// Save current frame.
	s.Frames[s.FrameSP] = Frame{
		Script:       s.Script,
		PC:           s.PC,
		IntLocals:    s.IntLocals,
		StringLocals: s.StringLocals,
	}
	s.FrameSP++

	// Allocate new locals for the callee.
	intLocals := make([]int, max(int(target.IntLocalCount), len(intArgs)))
	copy(intLocals, intArgs)

	stringLocals := make([]string, max(int(target.StringLocalCount), len(stringArgs)))
	copy(stringLocals, stringArgs)

	// Switch to callee context. PC = -1 so runner's PC++ lands at 0.
	s.Script = target
	s.PC = -1
	s.IntLocals = intLocals
	s.StringLocals = stringLocals
}

// JumpCall performs a tail-call to target, discarding all saved frames.
// Distinct from GosubCall which saves the caller frame. TS reference:
// ScriptState.gotoFrame → setupNewScript.
func (s *ScriptState) JumpCall(target *ScriptFile, intArgs []int, stringArgs []string) {
	s.FrameSP = 0

	intLocals := make([]int, max(int(target.IntLocalCount), len(intArgs)))
	copy(intLocals, intArgs)
	stringLocals := make([]string, max(int(target.StringLocalCount), len(stringArgs)))
	copy(stringLocals, stringArgs)

	s.Script = target
	s.PC = -1
	s.IntLocals = intLocals
	s.StringLocals = stringLocals
}

// Return pops the most recent call frame and restores execution context.
// If the frame stack is empty, sets Execution = Finished.
func (s *ScriptState) Return() error {
	if s.FrameSP == 0 {
		s.Execution = Finished
		return nil
	}

	s.FrameSP--
	frame := s.Frames[s.FrameSP]
	s.Script = frame.Script
	s.PC = frame.PC
	s.IntLocals = frame.IntLocals
	s.StringLocals = frame.StringLocals
	return nil
}
