package script

// PointerGroupFind is the 5-element list of find_* pointer names that
// many opcodes spread into their corrupt list. Mirrors TS
// POINTER_GROUP_FIND (ScriptOpcodePointers.ts:3).
//
// Order matters: corrupt-slice content is concatenated in this exact
// order on all 6 TS spread sites.
var PointerGroupFind = []string{
	"find_player", "find_npc", "find_loc", "find_obj", "find_db",
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
// Those two are ported as literal slice expansions (NOT via
// corruptExceptActive) because the prefix breaks the helper symmetry —
// see deviation NAI-201-D-POINTERS-SPREAD-HELPER.
func corruptExceptActive(extras ...string) []string {
	out := make([]string, 0, len(PointerGroupFind)+len(extras))
	out = append(out, PointerGroupFind...)
	out = append(out, extras...)
	return out
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
// 237 entries; verified by TestScriptOpcodePointers_LengthParity in T5.
// Entry order in this literal mirrors TS line ordering to support
// side-by-side review; Go map iteration order itself is randomized but
// unobservable to callers.
var ScriptOpcodePointers = map[Opcode]Pointers{
	// T5 populates this with 237 entries from TS ScriptOpcodePointers.ts:17-981.
}
