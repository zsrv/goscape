package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
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

// reconcileInvs mirrors TS World.reload L221-236 (the `if (clearInvs)`
// branch). Empties s.invs, rebuilds SCOPE_SHARED slots, and deletes
// SCOPE_TEMP slots from each player's invs map.
//
// SCOPE_PERM invs are persisted to save files and not reconciled (TS
// L222-235 does not touch SCOPE_PERM — only SHARED and TEMP have arms).
//
// Runs on the tick goroutine; no lock acquisition (memory
// plan_race_tag_for_cross_goroutine_test: production world is
// single-goroutine; tick is sole writer to p.invs).
func reconcileInvs(serverInvs map[int]*inventory.Inventory, players []*Player, invTypes *objtype.InvTypeConfigs) map[int]*inventory.Inventory {
	fresh := make(map[int]*inventory.Inventory)
	if invTypes == nil {
		return fresh
	}
	for id := 0; id < len(invTypes.Configs); id++ {
		inv := invTypes.Configs[id]
		if inv == nil {
			continue // goscape-defensive; TS InvType.get(id) returns a sentinel
		}
		switch inv.Scope {
		case objtype.InvTypeScopeShared:
			fresh[id] = inventory.FromType(inv)
		case objtype.InvTypeScopeTemp:
			for _, p := range players {
				if p == nil || p.invs == nil {
					continue
				}
				delete(p.invs, id)
			}
			// SCOPE_PERM: TS does not reconcile (persisted).
		}
	}
	_ = serverInvs // input is the pre-reconcile map; we discard it (TS L222: this.invs.clear())
	return fresh
}
