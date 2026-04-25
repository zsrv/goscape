package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// newRegisteredNpc constructs a synthetic NPC for tests.
//
// Defaults: nid=1 (placeholder; overwritten by addNpc when register=true),
// typeId=1, coords (3200, 3200, 0).
//
// When register=true, calls s.addNpc(n, -1, true) — equivalent to the
// production first-spawn path. The caller-passed nid=1 is overwritten
// by s.allocNpcSlot inside addNpc; n.nid post-call reflects the
// allocated slot. NB: n.uid is computed at NewNpc time as
// (typeId<<16)|1; addNpc does not recompute it. This matches existing
// production behavior — tests that care about uid invariants must
// recompute or read n.nid after the call.
//
// When register=false, returns a bare *Npc with nid=1, suitable for
// unit tests of constructor / mask / stats behavior that don't need
// the npcs[]/npcLoop[] bookkeeping.
//
// typ must not be nil (callers vary Size, BlockWalk, Stats, HuntMode
// etc per test). s.npcTypes is allocated to a two-element Configs slice
// [nil, typ] when nil so resetEntityForRespawn's lookupType call
// resolves correctly.
func newRegisteredNpc(t *testing.T, s *Server, typ *objtype.NpcType, register bool) *Npc {
	t.Helper()
	if typ == nil {
		t.Fatalf("newRegisteredNpc: typ must not be nil")
	}
	if s.npcTypes == nil {
		s.npcTypes = &objtype.NPCTypeConfigs{
			Configs: []*objtype.NpcType{nil, typ},
		}
	}
	n := NewNpc(1, 1, 3200, 3200, 0, typ)
	if register {
		if err := s.addNpc(n, -1, true); err != nil {
			t.Fatalf("newRegisteredNpc: addNpc: %v", err)
		}
	}
	return n
}
