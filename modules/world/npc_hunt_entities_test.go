package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/zone"
)

// addNpcToServerAt seeds s.npcs[nid], registers the NPC's type in
// s.npcTypes.Configs, and indexes into s.grid. Returns the *Npc
// so tests can further mutate fields. Slot 0 is reserved; use 1+.
// nid must be < 8192 (fixed Server.npcs array size).
func addNpcToServerAt(t *testing.T, s *Server, nid, typeId, category, x, z, level int) *Npc {
	t.Helper()
	if s.grid == nil {
		s.grid = grid.New()
	}
	if s.npcTypes == nil {
		s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 100)}
	}
	if typeId < len(s.npcTypes.Configs) && s.npcTypes.Configs[typeId] == nil {
		s.npcTypes.Configs[typeId] = &objtype.NpcType{
			ConfigType: objtype.ConfigType{ID: typeId},
			Category:   category,
		}
	}
	n := NewNpc(nid, typeId, x, z, level, s.npcTypes.Configs[typeId])
	s.npcs[nid] = n
	s.grid.AddNpc(nid, x, z, level)
	return n
}

func TestHuntNpcsInRangeSameLevel(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	tIn := addNpcToServerAt(t, s, 10, 1, -1, n.x+3, n.z+3, n.level)
	_ = addNpcToServerAt(t, s, 11, 1, -1, n.x+20, n.z+20, n.level) // out of range

	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (in-range only)", len(hunted))
	}
	if hunted[0].Slot() != tIn.nid {
		t.Errorf("hunted[0]: got nid %d, want %d", hunted[0].Slot(), tIn.nid)
	}
}

func TestHuntNpcsDifferentLevelExcluded(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 20, 1, -1, n.x, n.z, n.level+1) // different level

	hunted := n.huntNpcs(s, &objtype.HuntType{CheckNpc: -1, CheckCategory: -1})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (level mismatch)", len(hunted))
	}
}

func TestHuntNpcsCheckNpcFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 30, 5, -1, n.x+2, n.z+2, n.level)      // typeId 5
	match := addNpcToServerAt(t, s, 31, 7, -1, n.x+3, n.z+3, n.level) // typeId 7 (target)

	hunt := &objtype.HuntType{CheckNpc: 7, CheckCategory: -1}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (CheckNpc=7 only)", len(hunted))
	}
	if hunted[0].Slot() != match.nid {
		t.Errorf("hunted[0]: got nid %d, want %d", hunted[0].Slot(), match.nid)
	}
}

func TestHuntNpcsCheckNpcNegativeOneAllowsAll(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 40, 5, -1, n.x+2, n.z+2, n.level)
	_ = addNpcToServerAt(t, s, 41, 7, -1, n.x+3, n.z+3, n.level)

	hunted := n.huntNpcs(s, &objtype.HuntType{CheckNpc: -1, CheckCategory: -1})
	if len(hunted) != 2 {
		t.Errorf("hunted: got %d, want 2 (CheckNpc=-1 allows all)", len(hunted))
	}
}

func TestHuntNpcsCheckCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 50, 1, 42, n.x+2, n.z+2, n.level)      // cat 42
	match := addNpcToServerAt(t, s, 51, 2, 99, n.x+3, n.z+3, n.level) // cat 99 (target)

	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: 99}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1", len(hunted))
	}
	if hunted[0].Slot() != match.nid {
		t.Errorf("hunted[0]: got nid %d, want %d", hunted[0].Slot(), match.nid)
	}
}

func TestHuntNpcsChebyshevDistanceBoundary(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	// At dx = dz = 10: included (|dx|, |dz| both <= 10).
	in1 := addNpcToServerAt(t, s, 60, 1, -1, n.x+10, n.z+10, n.level)
	// At dx = 11, dz = 0: excluded.
	_ = addNpcToServerAt(t, s, 61, 1, -1, n.x+11, n.z, n.level)

	hunted := n.huntNpcs(s, &objtype.HuntType{CheckNpc: -1, CheckCategory: -1})
	if len(hunted) != 1 || hunted[0].Slot() != in1.nid {
		t.Errorf("boundary: got %v, want [nid=%d]", hunted, in1.nid)
	}
}

func TestHuntNpcsMissingTypeConfigSkipsOnCategoryFilter(t *testing.T) {
	// When an NPC has a typeId that's out of bounds of npcTypes.Configs,
	// and CheckCategory is active, the entry should be silently skipped
	// rather than crashing.
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	// Create an NPC whose typeId exceeds the Configs length.
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 3)}
	s.npcTypes.Configs[1] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 1},
		Category:   -1,
	}
	// Create NPC with a type that exists (for NewNpc), but set its typeId
	// to something out of bounds.
	other := NewNpc(70, 1, n.x+3, n.z+3, n.level, s.npcTypes.Configs[1])
	other.typeId = 99 // Now typeId is out of bounds.
	s.npcs[70] = other
	s.grid = grid.New()
	s.grid.AddNpc(70, other.x, other.z, other.level)

	hunted := n.huntNpcs(s, &objtype.HuntType{CheckNpc: -1, CheckCategory: 42})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (oob typeId + category filter → skip)", len(hunted))
	}
}

func TestHuntNpcsNilRegistriesReturnsNil(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntRange = 10

	// s.grid is nil per newServerForScriptTest.
	hunted := n.huntNpcs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (grid nil)", hunted)
	}

	s.grid = grid.New()
	// s.npcTypes is nil.
	hunted = n.huntNpcs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (npcTypes nil)", hunted)
	}
}

// addObjToZone seeds an *entity.Obj at (level, x, z) in the
// containing zone, registers its type in s.objTypes.Configs, and
// appends to zone.Objs. Returns the Obj for test assertions.
func addObjToZone(t *testing.T, s *Server, level, x, z, typeId, category int) *entitypkg.Obj {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 100)}
	}
	if typeId < len(s.objTypes.Configs) && s.objTypes.Configs[typeId] == nil {
		s.objTypes.Configs[typeId] = &objtype.ObjType{
			ConfigType: objtype.ConfigType{ID: typeId},
			Category:   category,
		}
	}
	o := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, typeId, 1)
	zn := s.zoneMap.Get(level, x, z)
	zn.Objs = append(zn.Objs, o)
	return o
}

func TestHuntObjsInRangeSameLevel(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	oIn := addObjToZone(t, s, n.level, n.x+3, n.z+3, 1, -1)
	_ = addObjToZone(t, s, n.level, n.x+20, n.z+20, 1, -1) // out of range

	hunt := &objtype.HuntType{CheckObj: -1, CheckCategory: -1}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1", len(hunted))
	}
	if hunted[0] != oIn {
		t.Errorf("hunted[0]: got %v, want oIn", hunted[0])
	}
}

func TestHuntObjsDifferentLevelExcluded(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZone(t, s, n.level+1, n.x, n.z, 1, -1) // different level

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: -1})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (level mismatch)", len(hunted))
	}
}

func TestHuntObjsCheckObjFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZone(t, s, n.level, n.x+2, n.z+2, 5, -1)
	match := addObjToZone(t, s, n.level, n.x+3, n.z+3, 7, -1)

	hunt := &objtype.HuntType{CheckObj: 7, CheckCategory: -1}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 1 || hunted[0] != match {
		t.Errorf("hunted: got %v, want [match]", hunted)
	}
}

func TestHuntObjsCheckObjNegativeOneAllowsAll(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZone(t, s, n.level, n.x+2, n.z+2, 5, -1)
	_ = addObjToZone(t, s, n.level, n.x+3, n.z+3, 7, -1)

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: -1})
	if len(hunted) != 2 {
		t.Errorf("hunted: got %d, want 2 (CheckObj=-1 allows all)", len(hunted))
	}
}

func TestHuntObjsCheckCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZone(t, s, n.level, n.x+2, n.z+2, 1, 42)
	match := addObjToZone(t, s, n.level, n.x+3, n.z+3, 2, 99)

	hunt := &objtype.HuntType{CheckObj: -1, CheckCategory: 99}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 1 || hunted[0] != match {
		t.Errorf("hunted: got %v, want [match]", hunted)
	}
}

func TestHuntObjsChebyshevDistanceBoundary(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	in1 := addObjToZone(t, s, n.level, n.x+10, n.z+10, 1, -1)
	_ = addObjToZone(t, s, n.level, n.x+11, n.z, 1, -1)

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: -1})
	if len(hunted) != 1 || hunted[0] != in1 {
		t.Errorf("boundary: got %v, want [in1]", hunted)
	}
}

func TestHuntObjsMissingTypeConfigSkipsOnCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	s.zoneMap = zone.NewZoneMap()
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 3)}
	// Obj with typeId 99 (exceeds Configs length).
	o := entitypkg.NewObj(n.level, n.x+3, n.z+3, entitypkg.LifecycleDespawn, 99, 1)
	zn := s.zoneMap.Get(n.level, o.X, o.Z)
	zn.Objs = append(zn.Objs, o)

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: 42})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (oob typeId + category filter → skip)", len(hunted))
	}
}

func TestHuntObjsNilRegistriesReturnsNil(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntRange = 10

	hunted := n.huntObjs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (zoneMap nil)", hunted)
	}

	s.zoneMap = zone.NewZoneMap()
	hunted = n.huntObjs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (objTypes nil)", hunted)
	}
}
