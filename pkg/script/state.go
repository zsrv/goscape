package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)

const (
	// StackCapacity is the maximum depth of the int and string stacks.
	// Exceeding it is a compiler bug and panics.
	StackCapacity = 1024

	// OpCountLimit is the maximum number of opcodes that may execute in a
	// single Execute call. Prevents infinite loops from hanging the server.
	OpCountLimit = 500_000

	// FrameCapacity is the maximum GOSUB nesting depth.
	FrameCapacity = 50
)

// PlayerLookup resolves a UID to an ActivePlayer if a player with that UID
// is currently logged in. Handlers decide whether the result is usable:
// FINDUID accepts any match; P_FINDUID additionally gates on CanAccess.
// Returns nil if no logged-in player has that UID.
type PlayerLookup interface {
	LookupPlayerByUID(uid int) ActivePlayer
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
// huntvis accepts HuntVisOff/LineOfSight/LineOfWalk (pkg/objtype.HuntVis*)
// but the current impl does not filter on it (deviation S7f-D1).
// Callers must still validate via checkHuntVis.
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

	PC      int
	OpCount int

	// LastInt is the int value injected by a resume event (e.g. the count
	// from RESUME_P_COUNTDIALOG). Scripts read it via LAST_INT opcode.
	LastInt int

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

	// ActiveObj is the Obj that OBJ_* and AI_*OBJ handlers target. Nil
	// if no Obj is bound. NAI-11.
	ActiveObj ActiveObj

	// OtherActiveNpc is the secondary NPC slot used by AI_*NPC handlers
	// when `Self` (an NPC) targets another NPC. NAI-11.
	OtherActiveNpc ActiveNpc

	// DB cursor state — populated by DB_LISTALL* (and DB_FIND*, deferred to a
	// later sub-spec); consumed by DB_FINDNEXT, DB_FINDBYINDEX. DbTable == nil
	// means no LISTALL/FIND has selected a table yet; DbRow is the cursor index
	// into DbRowQuery (-1 after LISTALL before the first FINDNEXT advance).
	DbTable    *objtype.DbTableType
	DbRow      int
	DbRowQuery []int

	Protect bool

	// Arrays holds script-local int[] arrays defined via DEFINE_ARRAY.
	// Index = array slot (0..4); length set at DEFINE_ARRAY, fixed
	// thereafter. A nil slice at a slot means "undefined"; OOB reads
	// return 0 and OOB writes are dropped.
	Arrays [5][]int32
}

// PushInt pushes v onto the int stack.
// Panics if the stack is full (programming error / compiler bug).
func (s *ScriptState) PushInt(v int) {
	if s.ISP >= StackCapacity {
		panic(fmt.Sprintf("script: int stack overflow at pc=%d in %q", s.PC, s.Script.Name))
	}
	s.IntStack[s.ISP] = v
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
// Returns "" on underflow (matches TS popString returning '' on null).
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
	for i, v := range intArgs {
		intLocals[i] = v
	}

	stringLocals := make([]string, max(int(target.StringLocalCount), len(stringArgs)))
	for i, v := range stringArgs {
		stringLocals[i] = v
	}

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
	for i, v := range intArgs {
		intLocals[i] = v
	}
	stringLocals := make([]string, max(int(target.StringLocalCount), len(stringArgs)))
	for i, v := range stringArgs {
		stringLocals[i] = v
	}

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
