// DEVIATION-NAI-C-CONFIGS-ATOMIC-SWAP
//
// Server holds 20 type-config registry fields written by Reload (tick
// goroutine) and concurrently read by the per-connection login goroutine in
// sendLoginOK (client.go: invTypes) and newPlayer (player.go: seqTypes).
// Without synchronisation a reader can observe a partially-swapped pointer
// during Reload. This file introduces serverConfigsSnapshot — an immutable
// bundle of all 20 fields — held in Server.configsPtr (atomic.Pointer).
// Reload and NewServer call storeConfigsSnapshot() after all raw fields are
// assigned; per-connection goroutines read through loginConfigs().
//
// Tick-goroutine readers (server_configs.go, handlers_game.go, tick.go,
// etc.) continue to access the raw Server fields — they share the tick
// goroutine with Reload, so no concurrent access.
//
// Arc 12 precedent: pkg/cache/{crctable,preloaded}.go (commit a1e9da32)
// applied the same atomic.Pointer build-then-swap pattern to the CRC and
// preload snapshots.

package world

import (
	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/objtype"
)

// serverConfigsSnapshot is the immutable point-in-time bundle of all
// type-config registries. Built by storeConfigsSnapshot and atomically
// stored in Server.configsPtr.
type serverConfigsSnapshot struct {
	paramTypes     *objtype.ParamTypeConfigs
	objTypes       *objtype.ObjTypeConfigs
	invTypes       *objtype.InvTypeConfigs
	dbTableTypes   *objtype.DbTableTypeConfigs
	dbRowTypes     *objtype.DbRowTypeConfigs
	dbTableIndex   *objtype.DbTableIndex
	varpTypes      *objtype.VarpTypeConfigs
	varsTypes      *objtype.VarsTypeConfigs
	varnTypes      *objtype.VarnTypeConfigs
	enumTypes      *objtype.EnumTypeConfigs
	structTypes    *objtype.StructTypeConfigs
	locTypes       *objtype.LocTypeConfigs
	npcTypes       *objtype.NPCTypeConfigs
	huntTypes      *objtype.HuntTypeConfigs
	idkTypes       *objtype.IdkTypeConfigs
	mesanimTypes   *objtype.MesanimTypeConfigs
	fontTypes      []*fonttype.FontType
	seqTypes       *objtype.SeqTypeConfigs
	spotanimTypes  *objtype.SpotanimTypeConfigs
	componentTypes *objtype.ComponentTypeConfigs
}

// storeConfigsSnapshot captures all raw config fields from s into a fresh
// serverConfigsSnapshot and atomically swaps it in. Must be called from
// the tick goroutine (or from NewServer before the tick goroutine starts)
// so all raw fields are in a consistent post-assignment state.
func (s *Server) storeConfigsSnapshot() {
	s.configsPtr.Store(&serverConfigsSnapshot{
		paramTypes:     s.paramTypes,
		objTypes:       s.objTypes,
		invTypes:       s.invTypes,
		dbTableTypes:   s.dbTableTypes,
		dbRowTypes:     s.dbRowTypes,
		dbTableIndex:   s.dbTableIndex,
		varpTypes:      s.varpTypes,
		varsTypes:      s.varsTypes,
		varnTypes:      s.varnTypes,
		enumTypes:      s.enumTypes,
		structTypes:    s.structTypes,
		locTypes:       s.locTypes,
		npcTypes:       s.npcTypes,
		huntTypes:      s.huntTypes,
		idkTypes:       s.idkTypes,
		mesanimTypes:   s.mesanimTypes,
		fontTypes:      s.fontTypes,
		seqTypes:       s.seqTypes,
		spotanimTypes:  s.spotanimTypes,
		componentTypes: s.componentTypes,
	})
}

// loginConfigs returns the most recently committed snapshot. Never nil:
// if no snapshot has been stored (pre-init, test-direct-Server construction
// without NewServer), returns an empty snapshot. Callers nil-check
// individual fields before dereferencing.
func (s *Server) loginConfigs() *serverConfigsSnapshot {
	if snap := s.configsPtr.Load(); snap != nil {
		return snap
	}
	return &serverConfigsSnapshot{}
}
