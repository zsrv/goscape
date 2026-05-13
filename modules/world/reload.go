package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
)

// resizeVarShared mirrors TS World.reload's VarSharedType resize block
// at World.ts:246-268. When the new VarSharedType count differs from
// the old, allocates fresh slices of the new size, copies the overlap
// from old, then re-initializes EVERY slot per type (DEVIATION-NAI-190-
// D3-CANDIDATE-VARSHARED-CLOBBER — TS clobbers copied values; mirrored
// verbatim per the true-to-TS gate).
//
// When the counts match, returns the input slices unchanged (TS L246's
// `if` guard).
func resizeVarShared(oldVars []int32, oldStrs []string, newConfigs []*objtype.VarSharedType) (newVars []int32, newStrs []string) {
	if len(oldVars) == len(newConfigs) {
		return oldVars, oldStrs
	}
	newVars = make([]int32, len(newConfigs))
	newStrs = make([]string, len(newConfigs))
	n := min(len(oldVars), len(newVars))
	copy(newVars, oldVars[:n])
	copy(newStrs, oldStrs[:n])
	// TS L259-267: iterates ALL indices unconditionally, clobbering
	// copied non-string slots. Mirrored verbatim.
	for i := 0; i < len(newVars); i++ {
		varsh := newConfigs[i]
		if varsh == nil {
			continue // goscape-defensive; TS VarSharedType.get(id) returns a sentinel
		}
		if varsh.Type == objtype.ScriptVarTypeString {
			continue
		}
		if varsh.Type == objtype.ScriptVarTypeInt {
			newVars[i] = 0
		} else {
			newVars[i] = -1
		}
	}
	return newVars, newStrs
}
