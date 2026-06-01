package world

import (
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
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
// branch). Returns a fresh map containing SCOPE_SHARED slots
// rebuilt from type; deletes SCOPE_TEMP slots from each player's
// invs map. Caller assigns the returned map to s.invs (replacing
// the prior one, matching TS L222 this.invs.clear()).
//
// SCOPE_PERM invs are persisted to save files and not reconciled (TS
// L222-235 does not touch SCOPE_PERM — only SHARED and TEMP have arms).
//
// Runs on the tick goroutine; no lock acquisition (memory
// plan_race_tag_for_cross_goroutine_test: production world is
// single-goroutine; tick is sole writer to p.invs).
func reconcileInvs(players []*Player, invTypes *objtype.InvTypeConfigs) map[int]*inventory.Inventory {
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
	return fresh
}

// broadcast routes through the optional capture hook (test-injected)
// or falls back to Server.BroadcastMes (production).
func (s *Server) broadcast(msg string) {
	if s.broadcastMesFunc != nil {
		s.broadcastMesFunc(msg)
		return
	}
	s.BroadcastMes(msg)
}

// Reload re-loads all type-configs, scripts, CRCs, and preloaded client
// assets from cfg.CachePath. Mirrors TS World.reload at World.ts:206-292.
//
// Callers: (1) handleClientCheat ::reload (always clearInvs=true);
// (2) future friends-server inbound RELAY_RELOAD relay (clearInvs=false;
// TS World.ts:2036 — caller absent at NAI-190; signature preserves the
// parameter for the eventual wire-up).
//
// Runs synchronously on the tick goroutine (memory
// plan_race_tag_for_cross_goroutine_test); no locks acquired. Tick
// spike during cache reload matches TS's blocking-main-thread posture.
//
// DEVIATIONs:
//   - D1-GAMEMAP-RE-INJECT: step 11 re-injects loc/obj types into the
//     GameMap (TS reads package singletons; goscape goes through setters).
//   - D2-HALF-SWAP: post-step-3 mid-pipeline errors leave s.* partially
//     mutated. TS-parity (TS does not roll back). No rollback path.
//   - D3-CANDIDATE-VARSHARED-CLOBBER: see resizeVarShared.
//
// NAI-190.
func (s *Server) Reload(clearInvs bool) error {
	cachePath := s.cfg.CachePath

	// ─── Step 1: load pre-inv registries into locals ───
	varpTypes_, err := objtype.LoadVarpTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: varp types: %w", err)
	}
	params_, err := objtype.LoadParams(cachePath)
	if err != nil {
		return fmt.Errorf("reload: params: %w", err)
	}
	objTypes_, err := objtype.LoadObjTypes(cachePath, params_)
	if err != nil {
		return fmt.Errorf("reload: obj types: %w", err)
	}
	locTypes_, err := objtype.LoadLocTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: loc types: %w", err)
	}
	npcTypes_, err := objtype.LoadNPCTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: npc types: %w", err)
	}
	idkTypes_, err := objtype.LoadIdkTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: idk types: %w", err)
	}
	seqFrames_, err := objtype.LoadSeqFrames(cachePath)
	if err != nil {
		return fmt.Errorf("reload: seq frames: %w", err)
	}
	seqTypes_, err := objtype.LoadSeqTypes(cachePath, seqFrames_)
	if err != nil {
		return fmt.Errorf("reload: seq types: %w", err)
	}
	spotanim_, err := objtype.LoadSpotanimTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: spotanim types: %w", err)
	}
	categoryTypes_, err := objtype.LoadCategoryTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: category types: %w", err)
	}
	enumTypes_, err := objtype.LoadEnumTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: enum types: %w", err)
	}
	structTypes_, err := objtype.LoadStructTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: struct types: %w", err)
	}

	// ─── Step 2: load InvType ───
	invTypes_, err := objtype.LoadInvTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: inv types: %w", err)
	}

	// ─── Step 3: atomic swap of pre-inv registries ───
	s.varpTypes = varpTypes_
	s.paramTypes = params_
	s.objTypes = objTypes_
	s.locTypes = locTypes_
	s.npcTypes = npcTypes_
	s.idkTypes = idkTypes_
	s.seqTypes = seqTypes_
	s.spotanimTypes = spotanim_
	s.categoryTypes = categoryTypes_
	s.enumTypes = enumTypes_
	s.structTypes = structTypes_
	s.invTypes = invTypes_

	// ─── Step 4: clearInvs reconcile ───
	if clearInvs {
		s.invs = reconcileInvs(s.players[:], s.invTypes)
	}

	// ─── Step 5: load post-inv configs ───
	mesanim_, err := objtype.LoadMesanimTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: mesanim types: %w", err)
	}
	dbTable_, err := objtype.LoadDbTableTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: dbtable types: %w", err)
	}
	dbRow_, err := objtype.LoadDbRowTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: dbrow types: %w", err)
	}
	huntTypes_, err := objtype.LoadHuntTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: hunt types: %w", err)
	}
	varnTypes_, err := objtype.LoadVarnTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: varn types: %w", err)
	}
	varsTypes_, err := objtype.LoadVarsTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: vars types: %w", err)
	}

	// ─── Step 6: swap post-inv registries ───
	s.mesanimTypes = mesanim_
	s.dbTableTypes = dbTable_
	s.dbRowTypes = dbRow_
	s.dbTableIndex = objtype.BuildDbTableIndex(dbTable_, dbRow_)
	s.huntTypes = huntTypes_
	s.varnTypes = varnTypes_
	s.varsTypes = varsTypes_

	// ─── Step 7: VarShared resize ───
	s.vars, s.varsStrings = resizeVarShared(s.vars, s.varsStrings, s.varsTypes.Configs)

	// ─── Step 8: load + swap Component ───
	componentTypes_, err := objtype.LoadComponentTypes(cachePath)
	if err != nil {
		return fmt.Errorf("reload: component types: %w", err)
	}
	s.componentTypes = componentTypes_

	// ─── Step 9: reload scripts + broadcast result (TS L272-285) ───
	serverDir := filepath.Join(cachePath, "server")
	if s.scriptProvider == nil {
		s.scriptProvider = script.NewProvider()
	}
	count, scriptErr := s.scriptProvider.Load(serverDir)
	if s.cfg.NodeDebug {
		if scriptErr != nil {
			s.broadcast("There was an issue while reloading scripts.")
		} else {
			s.broadcast(fmt.Sprintf("Loaded %d scripts.", count))
		}
	} else {
		if scriptErr != nil {
			s.log.Error("script reload failed", "err", scriptErr)
		} else {
			s.log.Debug("scripts reloaded", "count", count)
		}
	}

	// ─── Step 10: CRC regen + client preload (TS L288, L291) ───
	cache.MakeCRCs(cachePath)
	clientDir := filepath.Join(cachePath, "client")
	if err := cache.PreloadClient(clientDir); err != nil {
		// TS preloadClient throws on error; goscape returns. Per
		// DEVIATION-NAI-190-D2-HALF-SWAP, the post-step-3 swap is
		// already committed.
		return fmt.Errorf("reload: preload client: %w", err)
	}

	// ─── Step 11: GameMap re-injection (DEVIATION-NAI-190-D1) ───
	if s.gamemap != nil {
		s.gamemap.SetLocTypes(s.locTypes)
		s.gamemap.SetObjTypes(s.objTypes)
		s.gamemap.SetMembers(s.cfg.NodeMembers)
	}

	// Build-then-swap the concurrent-reader snapshot. All raw fields are now
	// in their final Reload state; per-connection login goroutines will pick
	// this up on their next atomic load. DEVIATION-NAI-C-CONFIGS-ATOMIC-SWAP.
	s.storeConfigsSnapshot()

	return nil
}
