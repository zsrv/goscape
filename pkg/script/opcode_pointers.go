package script

import "slices"

// pointerGroupFind is the 5-element find_* pointer-name list (unexported
// to prevent caller mutation). Mirrors TS POINTER_GROUP_FIND
// (ScriptOpcodePointers.ts:3). External callers reach it through the
// PointerGroupFind() accessor which returns a fresh copy.
//
// NAI-202-D-POINTER-GROUP-FIND-HARDENED: NAI-201 originally shipped this
// as `var PointerGroupFind = []string{...}` (exported slice). NAI-202
// closes a NAI-201 final-review follow-up by hiding the storage and
// returning copies, defending against accidental mutation of package
// state by callers that grow into existence in NAI-203+.
//
// Order matters: corrupt-slice content is concatenated in this exact
// order on all 6 TS spread sites.
var pointerGroupFind = [5]string{
	"find_player", "find_npc", "find_loc", "find_obj", "find_db",
}

// PointerGroupFind returns a fresh slice copy of the find_* pointer-name
// list. Returning a copy ensures callers cannot mutate package-internal
// state — see NAI-202-D-POINTER-GROUP-FIND-HARDENED.
func PointerGroupFind() []string {
	return slices.Clone(pointerGroupFind[:])
}

// Pointers holds the pointer-gate flags for one script opcode. Mirrors
// the inline TS type at ScriptOpcodePointers.ts:5-14.
//
// Field semantics:
//   - Require / Require2: pointer names that MUST be set when the opcode
//     executes. Variant *2 applies in 2-active-entity contexts.
//   - Set / Set2: pointer names the opcode SETS on success.
//   - Corrupt / Corrupt2: pointer names the opcode invalidates.
//   - Conditional: true if Set takes effect only on a successful branch
//     (e.g., FINDUID conditional on lookup hit).
//
// Nil slice == "no entries" (matches TS optional-field omitted). Map
// miss on ScriptOpcodePointers returns the Pointers zero-value (all-nil
// slices, Conditional=false), which is the goscape equivalent of TS
// `ScriptOpcodePointers[opcode]` returning `undefined` — both mean
// "no constraints".
type Pointers struct {
	Require     []string
	Require2    []string
	Set         []string
	Set2        []string
	Corrupt     []string
	Corrupt2    []string
	Conditional bool
}

// corruptExceptActive returns PointerGroupFind ++ extras as a fresh
// slice. Mirrors TS spread pattern `[...POINTER_GROUP_FIND, ...extras]`
// used in 4 simple-spread entries:
//   - P_ARRIVEDELAY     (ScriptOpcodePointers.ts:286)
//   - P_COUNTDIALOG     (ScriptOpcodePointers.ts:301)
//   - P_DELAY           (ScriptOpcodePointers.ts:314)
//   - P_PAUSEBUTTON     (ScriptOpcodePointers.ts:370)
//
// TWO additional sites use a longer prefix
// (`['p_active_player', 'p_active_player2', ...POINTER_GROUP_FIND,
// 'last_com', ...]`):
//   - NPC_DELAY        (ScriptOpcodePointers.ts:569)
//   - NPC_ARRIVEDELAY  (ScriptOpcodePointers.ts:711)
//
// Those two are ported as literal slice expansions (NOT via
// corruptExceptActive) because the prefix breaks the helper symmetry —
// see deviation NAI-201-D-POINTERS-SPREAD-HELPER.
func corruptExceptActive(extras ...string) []string {
	return slices.Concat(pointerGroupFind[:], extras)
}

// ScriptOpcodePointers maps Opcode → Pointers describing the
// pointer-gate flags consumed by the bytecode compiler's typechecker
// (NAI-203+ arc). Mirrors TS ScriptOpcodePointers
// (ScriptOpcodePointers.ts:1-984).
//
// Opcodes not listed here have an absent / empty Pointers (TS:
// `ScriptOpcodePointers[opcode]` returns undefined, treated as "no
// constraints"). Mirrored in goscape via map miss (zero-value
// Pointers{}).
//
// 242 entries at the rev-254 pin; verified by
// TestScriptOpcodePointers_LengthParity, which documents the
// per-revision count history.
// Entry order in this literal mirrors TS line ordering to support
// side-by-side review; Go map iteration order itself is randomized but
// unobservable to callers.
var ScriptOpcodePointers = map[Opcode]Pointers{
	// Player ops
	OpAllowDesign: {Require: []string{"active_player"}},
	OpAnim: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpBasReadyAnim:    {Require: []string{"active_player"}},
	OpBasRunning:      {Require: []string{"active_player"}},
	OpBasTurnOnSpot:   {Require: []string{"active_player"}},
	OpBasWalkB:        {Require: []string{"active_player"}},
	OpBasWalkF:        {Require: []string{"active_player"}},
	OpBasWalkL:        {Require: []string{"active_player"}},
	OpBasWalkR:        {Require: []string{"active_player"}},
	OpBuildAppearance: {Require: []string{"active_player"}},
	OpBusy: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpBusy2: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpCamLookAt: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpCamMoveTo: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpCamReset: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpCamShake: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpClearQueue: {Require: []string{"active_player"}},
	OpClearSoftTimer: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpClearTimer: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpGetTimer: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpCoord: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpDamage: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpDisplayName: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpFaceSquare: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpFindUID: {
		Set:         []string{"active_player"},
		Set2:        []string{"active_player2"},
		Conditional: true,
	},
	OpGender: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpGetQueue: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStatAdvance:  {Require: []string{"active_player"}},
	OpHeadIconsGet: {Require: []string{"active_player"}},
	OpHeadIconsSet: {Require: []string{"active_player"}},
	OpHealEnergy:   {Require: []string{"active_player"}},
	OpHintCoord:    {Require: []string{"active_player"}},
	OpHintNpc:      {Require: []string{"active_player", "active_npc"}},
	OpHintPlayer:   {Require: []string{"active_player", "active_player2"}},
	OpHintStop:     {Require: []string{"active_player"}},
	OpHuntAll:      {Set: []string{"find_player"}},
	OpHuntNext: {
		Require:     []string{"find_player"},
		Require2:    []string{"find_player"},
		Set:         []string{"active_player"},
		Set2:        []string{"active_player2"},
		Conditional: true,
	},
	OpNpcHunt: {
		Set:  []string{"active_npc"},
		Set2: []string{"active_npc2"},
	},
	OpNpcHuntAll: {Set: []string{"find_npc"}},
	OpNpcHasOp: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpIfClose: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpTutClose:   {Require: []string{"active_player"}},
	OpIfOpenChat: {Require: []string{"active_player"}},
	OpTutOpen:    {Require: []string{"active_player"}},
	OpIfOpenMain: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpIfOpenMainSide: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpIfOpenSide:  {Require: []string{"active_player"}},
	OpIfSetAnim:   {Require: []string{"active_player"}},
	OpIfSetColour: {Require: []string{"active_player"}},
	OpIfSetHide: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpIfSetModel:      {Require: []string{"active_player"}},
	OpIfSetNpcHead:    {Require: []string{"active_player"}},
	OpIfSetObject:     {Require: []string{"active_player"}},
	OpIfSetPlayerHead: {Require: []string{"active_player"}},
	OpIfSetPosition:   {Require: []string{"active_player"}},
	OpIfSetScrollPos:  {Require: []string{"active_player"}},
	// SET_PLAYER_OP new in 254 (TS ScriptOpcodePointers.ts @43e02957):
	// require active_player only — no require2.
	OpSetPlayerOp: {Require: []string{"active_player"}},
	// OpIfSetRecol deleted in 244 (ScriptOpcode.ts); row removed.
	OpIfSetResumeButtons: {Require: []string{"active_player"}},
	OpIfSetTab:           {Require: []string{"active_player"}},
	OpIfSetTabActive: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpTutFlash: {Require: []string{"active_player"}},
	OpIfSetText: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpLastLoginInfo:  {Require: []string{"active_player"}},
	OpLastCom:        {Require: []string{"last_com"}},
	OpLastInt:        {Require: []string{"last_int"}},
	OpLastItem:       {Require: []string{"last_item"}},
	OpLastSlot:       {Require: []string{"last_slot"}},
	OpLastTargetSlot: {Require: []string{"last_targetslot"}},
	OpLastUseItem:    {Require: []string{"last_useitem"}},
	OpLastUseSlot:    {Require: []string{"last_useslot"}},
	OpLongQueue: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpLongQueueVarArg: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpLowMemory: {Require: []string{"active_player"}},
	OpMes: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpMidiJingle: {Require: []string{"active_player"}},
	OpMidiSong:   {Require: []string{"active_player"}},
	OpName: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpPApRange: {Require: []string{"p_active_player"}},
	OpPArriveDelay: {
		Require: []string{"p_active_player"},
		Corrupt: corruptExceptActive(
			"last_com", "last_int", "last_item", "last_slot",
			"last_targetslot", "last_useitem", "last_useslot",
		),
	},
	OpPCountDialog: {
		Require: []string{"p_active_player"},
		Set:     []string{"last_int"},
		Corrupt: corruptExceptActive(
			"last_com", "last_item", "last_slot",
			"last_targetslot", "last_useitem", "last_useslot",
		),
	},
	OpPDelay: {
		Require: []string{"p_active_player"},
		Corrupt: corruptExceptActive(
			"last_com", "last_int", "last_item", "last_slot",
			"last_targetslot", "last_useitem", "last_useslot",
		),
	},
	OpPExactMove: {Require: []string{"p_active_player"}},
	OpPFindUID: {
		Set:         []string{"p_active_player", "active_player"},
		Set2:        []string{"p_active_player2", "active_player2"},
		Conditional: true,
	},
	OpPLocMerge: {Require: []string{"p_active_player"}},
	OpPLogout:   {Require: []string{"p_active_player"}},
	OpPPreventLogout: {
		Require:  []string{"p_active_player"},
		Require2: []string{"p_active_player2"},
	},
	OpPOpHeld: {Require: []string{"p_active_player"}},
	OpPOpLoc:  {Require: []string{"p_active_player", "active_loc"}},
	OpPOpNpc:  {Require: []string{"p_active_player", "active_npc"}},
	OpPOpNpcT: {Require: []string{"p_active_player", "active_npc"}},
	OpPOpObj:  {Require: []string{"p_active_player", "active_obj"}},
	OpPOpPlayer: {
		Require:  []string{"p_active_player", "active_player2"},
		Require2: []string{"p_active_player2", "active_player"},
	},
	OpPOpPlayerT: {
		Require:  []string{"p_active_player", "active_player2"},
		Require2: []string{"p_active_player2", "active_player"},
	},
	OpPPauseButton: {
		Require: []string{"p_active_player"},
		Set:     []string{"last_com"},
		Corrupt: corruptExceptActive(
			"last_int", "last_item", "last_slot",
			"last_targetslot", "last_useitem", "last_useslot",
		),
	},
	OpPStopAction: {
		Require:  []string{"p_active_player"},
		Require2: []string{"p_active_player2"},
	},
	OpPClearPendingAction: {
		Require:  []string{"p_active_player"},
		Require2: []string{"p_active_player2"},
	},
	OpPTeleJump: {
		Require:  []string{"p_active_player"},
		Require2: []string{"p_active_player2"},
	},
	OpPTeleport: {
		Require:  []string{"p_active_player"},
		Require2: []string{"p_active_player2"},
	},
	OpPWalk: {
		Require:  []string{"p_active_player"},
		Require2: []string{"p_active_player2"},
	},
	OpProjAnimPl: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpQueue: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpQueueVarArg: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpSay: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpSetIdKit: {Require: []string{"p_active_player"}},
	OpWalkTrigger: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpGetWalkTrigger: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpSetTimer: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpSoftTimer: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpSoundSynth: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpSpotAnimPl: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStaffModLevel: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStat: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStatAdd: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStatBase: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStatHeal: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStatSub: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStatBoost: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStatDrain: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStatRandom: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpUID: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpWeakQueue: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpWeakQueueVarArg: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpFindHero: {
		Set:         []string{"active_player2"},
		Set2:        []string{"active_player"},
		Conditional: true,
	},
	OpBothHeroPoints: {
		Require:  []string{"active_player", "active_player2"},
		Require2: []string{"active_player2", "active_player"},
	},
	OpSetGender:     {Require: []string{"p_active_player"}},
	OpSetSkinColour: {Require: []string{"p_active_player"}},
	OpPAnimProtect: {
		Require:  []string{"p_active_player"},
		Require2: []string{"p_active_player2"},
	},
	OpRunEnergy: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpWeight: {
		Require:  []string{"p_active_player"},
		Require2: []string{"p_active_player2"},
	},
	OpPRun: {
		Require:  []string{"p_active_player"},
		Require2: []string{"p_active_player2"},
	},
	OpStrongQueue: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpStrongQueueVarArg: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	// New in 244: BUFFER_FULL, NPC_HUNTNEXT, IF_OPENOVERLAY, LAST_COORD.
	// ScriptOpcodePointers.ts lines cited from TS pin 9aadcec4.
	OpBufferFull: {Require: []string{"active_player"}},
	OpNpcHuntNext: {
		Require:     []string{"find_npc"},
		Require2:    []string{"find_npc"},
		Set:         []string{"active_npc"},
		Set2:        []string{"active_npc2"},
		Conditional: true,
	},
	OpIfOpenOverlay: {Require: []string{"active_player"}, Require2: []string{"active_player2"}},
	OpLastCoord:     {Require: []string{"active_player"}, Require2: []string{"active_player2"}},

	// Npc ops
	OpNpcAdd: {
		Set:  []string{"active_npc"},
		Set2: []string{"active_npc2"},
	},
	OpNpcAnim: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcBaseStat: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcCategory: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcChangeType: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcChangeTypeKeepAll: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcCoord: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcDamage: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcDel: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcDelay: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
		Corrupt: []string{
			"p_active_player", "p_active_player2",
			"find_player", "find_npc", "find_loc", "find_obj", "find_db",
			"last_com", "last_int", "last_item", "last_slot",
			"last_targetslot", "last_useitem", "last_useslot",
		},
	},
	OpNpcFaceSquare: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcFind: {
		Set:         []string{"active_npc"},
		Set2:        []string{"active_npc2"},
		Conditional: true,
	},
	OpNpcFindCat: {
		Set:         []string{"active_npc"},
		Set2:        []string{"active_npc2"},
		Conditional: true,
	},
	OpNpcFindAllAny:  {Set: []string{"find_npc"}},
	OpNpcFindAll:     {Set: []string{"find_npc"}},
	OpNpcFindAllZone: {Set: []string{"find_npc"}},
	OpNpcFindNext: {
		Require:     []string{"find_npc"},
		Set:         []string{"active_npc"},
		Set2:        []string{"active_npc2"},
		Conditional: true,
	},
	OpNpcFindExact: {
		Set:         []string{"active_npc"},
		Set2:        []string{"active_npc2"},
		Conditional: true,
	},
	// 254 (TS ScriptOpcodePointers.ts @43e02957): require2 removed;
	// set2 active_player2 → active_player (both set and set2 are
	// active_player).
	OpNpcFindHero: {
		Require:     []string{"active_npc"},
		Set:         []string{"active_player"},
		Set2:        []string{"active_player"},
		Conditional: true,
	},
	OpNpcFindUID: {
		Set:         []string{"active_npc"},
		Set2:        []string{"active_npc2"},
		Conditional: true,
	},
	OpNpcGetMode: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcHeroPoints: {Require: []string{"active_npc", "active_player"}},
	OpNpcName: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcParam: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcQueue: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcRange: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcSay: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcSetHunt: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcSetHuntMode: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcSetMode: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcWalkTrigger: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcSetTimer: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcStat: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcStatAdd: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcStatHeal: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcStatSub: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcTele: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcType: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcUID: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpProjAnimNpc: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpSpotAnimNpc: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcWalk: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcAttackRange: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},
	OpNpcArriveDelay: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
		Corrupt: []string{
			"p_active_player", "p_active_player2",
			"find_player", "find_npc", "find_loc", "find_obj", "find_db",
			"last_com", "last_int", "last_item", "last_slot",
			"last_targetslot", "last_useitem", "last_useslot",
		},
	},
	OpNpcInRange: {
		Require:  []string{"active_npc"},
		Require2: []string{"active_npc2"},
	},

	// Loc ops
	OpLocAdd: {
		Set:  []string{"active_loc"},
		Set2: []string{"active_loc2"},
	},
	OpLocAngle: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},
	OpLocAnim: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},
	OpLocCategory: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},
	OpLocChange: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},
	OpLocCoord: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},
	OpLocDel: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},
	OpLocFind: {
		Set:         []string{"active_loc"},
		Set2:        []string{"active_loc2"},
		Conditional: true,
	},
	OpLocFindAllZone: {
		Set:  []string{"find_loc"},
		Set2: []string{"find_loc"},
	},
	OpLocFindNext: {
		Require:     []string{"find_loc"},
		Require2:    []string{"find_loc"},
		Set:         []string{"active_loc"},
		Set2:        []string{"active_loc2"},
		Conditional: true,
	},
	OpLocName: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},
	OpLocParam: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},
	OpLocShape: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},
	OpLocType: {
		Require:  []string{"active_loc"},
		Require2: []string{"active_loc2"},
	},

	// Obj ops
	// 254 (TS ScriptOpcodePointers.ts @43e02957): require2
	// active_player2 → active_player.
	OpObjAdd: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player"},
		Set:      []string{"active_obj"},
		Set2:     []string{"active_obj2"},
	},
	OpObjAddAll: {
		Set:  []string{"active_obj"},
		Set2: []string{"active_obj2"},
	},
	OpObjCoord: {
		Require:  []string{"active_obj"},
		Require2: []string{"active_obj2"},
	},
	OpObjCount: {
		Require:  []string{"active_obj"},
		Require2: []string{"active_obj2"},
	},
	OpObjDel: {
		Require:  []string{"active_obj"},
		Require2: []string{"active_obj2"},
	},
	OpObjName: {
		Require:  []string{"active_obj"},
		Require2: []string{"active_obj2"},
	},
	OpObjParam: {
		Require:  []string{"active_obj"},
		Require2: []string{"active_obj2"},
	},
	// 254 (TS ScriptOpcodePointers.ts @43e02957): require2
	// active_obj2 → active_obj (active_player2 unchanged).
	OpObjTakeItem: {
		Require:  []string{"active_obj", "active_player"},
		Require2: []string{"active_obj", "active_player2"},
	},
	OpObjType: {
		Require:  []string{"active_obj"},
		Require2: []string{"active_obj2"},
	},
	OpObjFind: {
		Set:  []string{"active_obj"},
		Set2: []string{"active_obj2"},
	},
	OpObjFindAllZone: {
		Set:  []string{"find_obj"},
		Set2: []string{"find_obj"},
	},
	OpObjFindNext: {
		Require:     []string{"find_obj"},
		Require2:    []string{"find_obj"},
		Set:         []string{"active_obj"},
		Set2:        []string{"active_obj2"},
		Conditional: true,
	},

	// Inventory ops
	OpInvAdd: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvChangeSlot: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvClear: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvDel: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvDelSlot: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvDropItem: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
		Set:      []string{"active_obj"},
		Set2:     []string{"active_obj2"},
	},
	OpInvDropItemDelayed: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
		Set:      []string{"active_obj"},
		Set2:     []string{"active_obj2"},
	},
	OpInvDropSlot: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
		Set:      []string{"active_obj"},
		Set2:     []string{"active_obj2"},
	},
	OpInvFreeSpace: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvGetNum: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvGetObj: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvItemSpace: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvItemSpace2: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvMoveFromSlot: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvMoveToSlot: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpBothMoveInv: {
		Require:  []string{"active_player", "active_player2"},
		Require2: []string{"active_player2", "active_player"},
	},
	OpInvMoveItem: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvMoveItemCert: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvMoveItemUncert: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvSetSlot: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvTotal: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvTotalCat: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvTransmit: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvOtherTransmit: {
		Require:  []string{"active_player", "active_player2"},
		Require2: []string{"active_player2", "active_player"},
	},
	OpInvStopTransmit: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpBothDropSlot: {
		Require:  []string{"active_player", "active_player2"},
		Require2: []string{"active_player2", "active_player"},
	},
	OpInvDropAll: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvTotalParam: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},
	OpInvTotalParamStack: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},

	// String ops
	OpTextGender: {
		Require:  []string{"active_player"},
		Require2: []string{"active_player2"},
	},

	// DB ops
	OpDbFindNext:         {Require: []string{"find_db"}},
	OpDbFind:             {Set: []string{"find_db"}},
	OpDbFindRefine:       {Require: []string{"find_db"}},
	OpDbListAll:          {Set: []string{"find_db"}},
	OpDbListAllWithCount: {Set: []string{"find_db"}},
}
