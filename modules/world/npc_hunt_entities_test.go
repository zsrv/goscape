package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/zone"
)

// addNpcToServerAt seeds s.npcs[nid], registers the NPC's type in
// s.npcTypes.Configs, and subscribes into pkg/zone.
// Returns the *Npc so tests can further mutate fields.
// Slot 0 is reserved; use 1+. nid must be < 8192 (fixed Server.npcs array size).
func addNpcToServerAt(t *testing.T, s *Server, nid, typeId, category, x, z, level int) *Npc {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
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
	zn := s.zoneMap.Get(level, x, z)
	n.zoneListElement = zn.EnterNpc(n)
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
	s.zoneMap = zone.NewZoneMap()
	zn := s.zoneMap.Get(other.level, other.x, other.z)
	other.zoneListElement = zn.EnterNpc(other)

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

	// s.zoneMap is nil per newServerForScriptTest.
	hunted := n.huntNpcs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (zoneMap nil)", hunted)
	}

	s.zoneMap = zone.NewZoneMap()
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
	// Mirror pkg/zone.Zone.AddStaticObj which sets IsActive on append
	// (zone.go:256). Raw-append in this helper would leave IsActive=false,
	// which the huntObjs isValid gate (TS Obj.ts:52-62 → Entity.ts:32-34)
	// would correctly filter out — making every existing huntObjs test
	// spuriously assert zero results.
	o.IsActive = true
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
	o.IsActive = true // mirror AddStaticObj; keep this test honest under the huntObjs isValid gate

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

// addLocToZone seeds an *entity.Loc at (level, x, z) in the
// containing zone, registers its type in s.locTypes.Configs, and
// appends to zone.Locs. Returns the Loc for test assertions.
func addLocToZone(t *testing.T, s *Server, level, x, z, typeId, category int) *entitypkg.Loc {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	if s.locTypes == nil {
		s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 5200)}
	}
	if typeId < len(s.locTypes.Configs) && s.locTypes.Configs[typeId] == nil {
		s.locTypes.Configs[typeId] = &objtype.LocType{
			ConfigType: objtype.ConfigType{ID: typeId},
			Category:   category,
		}
	}
	l := entitypkg.NewLoc(level, x, z, 1, 1, entitypkg.LifecycleForever, typeId, 10, 0)
	zn := s.zoneMap.Get(level, x, z)
	zn.AddStaticLoc(l)
	return l
}

func TestHuntLocsInRangeSameLevel(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	lIn := addLocToZone(t, s, n.level, n.x+3, n.z+3, 1000, -1)
	_ = addLocToZone(t, s, n.level, n.x+20, n.z+20, 1000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})
	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1", len(hunted))
	}
	if hunted[0] != lIn {
		t.Errorf("hunted[0]: got %v, want lIn", hunted[0])
	}
}

func TestHuntLocsDifferentLevelExcluded(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZone(t, s, n.level+1, n.x, n.z, 1000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0", len(hunted))
	}
}

func TestHuntLocsCheckLocFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZone(t, s, n.level, n.x+2, n.z+2, 1000, -1)
	match := addLocToZone(t, s, n.level, n.x+3, n.z+3, 2000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: 2000, CheckCategory: -1})
	if len(hunted) != 1 || hunted[0] != match {
		t.Errorf("hunted: got %v, want [match]", hunted)
	}
}

func TestHuntLocsCheckLocNegativeOneAllowsAll(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZone(t, s, n.level, n.x+2, n.z+2, 1000, -1)
	_ = addLocToZone(t, s, n.level, n.x+3, n.z+3, 2000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})
	if len(hunted) != 2 {
		t.Errorf("hunted: got %d, want 2", len(hunted))
	}
}

func TestHuntLocsCheckCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZone(t, s, n.level, n.x+2, n.z+2, 1000, 42)
	match := addLocToZone(t, s, n.level, n.x+3, n.z+3, 2000, 99)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: 99})
	if len(hunted) != 1 || hunted[0] != match {
		t.Errorf("hunted: got %v, want [match]", hunted)
	}
}

func TestHuntLocsChebyshevDistanceBoundary(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	in1 := addLocToZone(t, s, n.level, n.x+10, n.z+10, 1000, -1)
	_ = addLocToZone(t, s, n.level, n.x+11, n.z, 1000, -1)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})
	if len(hunted) != 1 || hunted[0] != in1 {
		t.Errorf("boundary: got %v, want [in1]", hunted)
	}
}

func TestHuntLocsMissingTypeConfigSkipsOnCategoryFilter(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	s.zoneMap = zone.NewZoneMap()
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 3)}
	l := entitypkg.NewLoc(n.level, n.x+3, n.z+3, 1, 1, entitypkg.LifecycleForever, 99, 10, 0)
	s.zoneMap.Get(n.level, l.X, l.Z).AddStaticLoc(l)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: 42})
	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0", len(hunted))
	}
}

func TestHuntLocsNilRegistriesReturnsNil(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntRange = 10

	hunted := n.huntLocs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (zoneMap nil)", hunted)
	}

	s.zoneMap = zone.NewZoneMap()
	hunted = n.huntLocs(s, &objtype.HuntType{})
	if hunted != nil {
		t.Errorf("hunted: got %v, want nil (locTypes nil)", hunted)
	}
}

func TestHuntNpcsCheckCategoryNegativeOneAllowsAll(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 80, 1, 42, n.x+2, n.z+2, n.level)
	_ = addNpcToServerAt(t, s, 81, 2, 99, n.x+3, n.z+3, n.level)

	hunted := n.huntNpcs(s, &objtype.HuntType{CheckNpc: -1, CheckCategory: -1})
	if len(hunted) != 2 {
		t.Errorf("hunted: got %d, want 2 (CheckCategory=-1 allows all)", len(hunted))
	}
}

func TestHuntObjsCheckCategoryNegativeOneAllowsAll(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZone(t, s, n.level, n.x+2, n.z+2, 1, 42)
	_ = addObjToZone(t, s, n.level, n.x+3, n.z+3, 2, 99)

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: -1})
	if len(hunted) != 2 {
		t.Errorf("hunted: got %d, want 2 (CheckCategory=-1 allows all)", len(hunted))
	}
}

func TestHuntLocsCheckCategoryNegativeOneAllowsAll(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZone(t, s, n.level, n.x+2, n.z+2, 1000, 42)
	_ = addLocToZone(t, s, n.level, n.x+3, n.z+3, 2000, 99)

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})
	if len(hunted) != 2 {
		t.Errorf("hunted: got %d, want 2 (CheckCategory=-1 allows all)", len(hunted))
	}
}

// TestHuntAllPicksFromVariantResult is the cross-variant integration
// test from the NAI-9 spec: when huntAll dispatches to a variant
// that returns candidates, n.huntTarget is set to one of them.
func TestHuntAllPicksFromVariantResult(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10
	n.huntClock = 0

	// Seed exactly one candidate NPC in range so the rand.IntN pick
	// is deterministic — it must be the only candidate.
	candidate := addNpcToServerAt(t, s, 90, 1, -1, n.x+3, n.z+3, n.level)

	hunt := &objtype.HuntType{
		Type:          objtype.HuntModeNpc,
		Rate:          1,
		CheckNpc:      -1,
		CheckCategory: -1,
	}
	n.huntAll(s, hunt)

	if n.huntTarget == nil {
		t.Fatalf("huntTarget: got nil, want candidate NPC")
	}
	if n.huntTarget.Slot() != candidate.nid {
		t.Errorf("huntTarget: got nid %d, want %d", n.huntTarget.Slot(), candidate.nid)
	}
}

// withBlockingWall installs a blocking wall at (x, z, level) on the
// given Server's gamemap so the straight-line ray traversing that tile
// is blocked by both HasLineOfSight and HasLineOfWalk. Per FlagMap.Add,
// this also implicitly allocates the zone so adjacent path tiles read
// FlagOpen instead of FlagNull.
//
// Pre-condition: s.gamemap has been constructed via gamemap.New(...).
//
// Install both bits: FlagLocProjBlocker blocks LoS (via LineSightBlocked* masks);
// FlagLoc blocks LoW (via LineWalkBlocked* / FlagWalkBlocked). A real wall-loc
// would set both, so the helper mirrors that reality and is universal across
// Tasks 2-5's LoS and LoW block tests.
func withBlockingWall(t *testing.T, s *Server, x, z, level int) {
	t.Helper()
	s.gamemap.Pathfinder.Flags.Add(x, z, level, collision.FlagLoc|collision.FlagLocProjBlocker)
}

func TestHuntNpcsCheckVisLineOfSightPasses(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	tIn := addNpcToServerAt(t, s, 10, 1, -1, n.x, n.z+2, n.level) // 2 tiles north, clear path
	// Allocate the path's zone so empty tiles read FlagOpen instead of
	// FlagNull (0x7FFFFFFF). FlagMap.IsFlagged treats unallocated zones as fully
	// blocked; in production all map-loaded zones are allocated.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(n.x, n.z, n.level)

	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (LoS clear path)", len(hunted))
	}
	if hunted[0].Slot() != tIn.nid {
		t.Errorf("hunted[0]: got nid %d, want %d", hunted[0].Slot(), tIn.nid)
	}
}

func TestHuntNpcsCheckVisLineOfWalkBlocks(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 10, 1, -1, n.x, n.z+2, n.level) // 2 tiles north
	withBlockingWall(t, s, 3094, 3107, 0)                      // mid-tile blocker

	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfWalk}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 (LoW blocked by mid-tile)", len(hunted))
	}
}

// addObjToZoneAt seeds a dynamic Obj in the Server's zone map.
// Returns the *Obj so callers can mutate further.
func addObjToZoneAt(t *testing.T, s *Server, objType, x, z, level int) *entitypkg.Obj {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 100)}
	}
	if objType < len(s.objTypes.Configs) && s.objTypes.Configs[objType] == nil {
		s.objTypes.Configs[objType] = &objtype.ObjType{
			ConfigType: objtype.ConfigType{ID: objType},
			Category:   -1,
		}
	}
	o := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, objType, 1)
	z2 := s.zoneMap.Get(level, x, z)
	z2.Objs = append(z2.Objs, o)
	// Mirror pkg/zone.Zone.AddStaticObj (zone.go:256) — raw-append leaves
	// IsActive=false, which the huntObjs isValid gate would filter out.
	o.IsActive = true
	return o
}

func TestHuntObjsCheckVisLineOfSightPasses(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	// Allocate the source zone — unallocated zones return FlagNull=0x7FFFFFFF which
	// IsFlagged treats as blocked. See Task 1 LoS-pass test for full context.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(n.x, n.z, n.level)

	_ = addObjToZoneAt(t, s, 1, n.x, n.z+2, n.level) // 2 tiles north, clear path

	hunt := &objtype.HuntType{CheckObj: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (LoS clear path)", len(hunted))
	}
}

func TestHuntObjsCheckVisLineOfSightBlocks(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZoneAt(t, s, 1, n.x, n.z+2, n.level)
	withBlockingWall(t, s, 3094, 3107, 0) // mid-tile blocker; (x, z, level)

	hunt := &objtype.HuntType{CheckObj: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 (LoS blocked by mid-tile)", len(hunted))
	}
}

// addLocToZoneAt seeds a static Loc in the Server's zone map with
// category -1. Returns the *Loc so callers can mutate further.
func addLocToZoneAt(t *testing.T, s *Server, locType, x, z, level int) *entitypkg.Loc {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	if s.locTypes == nil {
		s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 100)}
	}
	if locType < len(s.locTypes.Configs) && s.locTypes.Configs[locType] == nil {
		s.locTypes.Configs[locType] = &objtype.LocType{
			ConfigType: objtype.ConfigType{ID: locType},
			Category:   -1,
		}
	}
	l := entitypkg.NewLoc(level, x, z, 1, 1, entitypkg.LifecycleForever, locType, 0, 0)
	z2 := s.zoneMap.Get(level, x, z)
	z2.Locs = append(z2.Locs, l)
	// Mirror pkg/zone.Zone.AddStaticLoc (zone.go:152) — raw-append leaves
	// IsActive=false, which the huntLocs isValid gate would filter out.
	l.IsActive = true
	return l
}

func TestHuntLocsCheckVisLineOfSightPasses(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZoneAt(t, s, 1, n.x, n.z+2, n.level) // 2 tiles north, clear path

	// Task 1 fixture-ordering convention: seed entity, THEN AllocateIfAbsent.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(n.x, n.z, n.level)

	hunt := &objtype.HuntType{CheckLoc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntLocs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (LoS clear path)", len(hunted))
	}
}

func TestHuntLocsCheckVisLineOfSightBlocks(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZoneAt(t, s, 1, n.x, n.z+2, n.level)
	withBlockingWall(t, s, 3094, 3107, 0) // mid-tile blocker

	hunt := &objtype.HuntType{CheckLoc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntLocs(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 (LoS blocked by mid-tile)", len(hunted))
	}
}

func TestHuntNpcsUsesZoneSubscriptionExclusive(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone, Category: -1}
	hunter := newRegisteredNpc(t, s, typ, true)
	hunter.huntRange = 5
	// Seed s.npcs[99] only — do NOT subscribe to Zone. huntNpcs reads from
	// Zone, so this NPC must NOT be returned.
	if s.npcTypes == nil {
		s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 100)}
	}
	s.npcs[99] = &Npc{nid: 99, x: hunter.x + 1, z: hunter.z + 1, level: hunter.level, typ: typ}
	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisOff}
	got := hunter.huntNpcs(s, hunt)
	for _, e := range got {
		if other, ok := e.(*Npc); ok && other.nid == 99 {
			t.Error("huntNpcs returned non-Zone-subscribed NPC; should be Zone-exclusive")
		}
	}
}

// TestHuntNpcsRespectsIsValidFilter verifies that NpcsSafe's IsValid()
// gate propagates to huntNpcs results — a Zone-subscribed NPC marked
// dead must NOT appear in hunt results. Mirrors TS huntNpcs's reliance
// on Zone.getAllNpcsSafe (which yields only IsValid()==true entities).
func TestHuntNpcsRespectsIsValidFilter(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone, Category: -1}
	hunter := newRegisteredNpc(t, s, typ, true)
	hunter.huntRange = 5
	// Spawn a Zone-subscribed neighbor NPC. newRegisteredNpc places it at
	// the same default tile as hunter, which is fine for an in-range hunt.
	target := newRegisteredNpc(t, s, typ, true)
	// Mark target dead — IsValid() returns false; NpcsSafe must skip.
	target.dead = true
	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisOff}
	got := hunter.huntNpcs(s, hunt)
	for _, e := range got {
		if other, ok := e.(*Npc); ok && other.nid == target.nid {
			t.Errorf("huntNpcs returned dead NPC nid=%d; IsValid filter should skip it", target.nid)
		}
	}
}

// TestHuntNpcsExcludesDelayedNpc pins npc-hunt-1: TS Npc.isValid
// (Npc.ts:370-375) overrides the base predicate to return false for
// delayed NPCs, propagating through Zone.getAllNpcsSafe (Zone.ts:399-405)
// to exclude delayed NPCs from huntNpcs results. goscape's Npc.IsValid
// must match — a Zone-subscribed NPC with delayed=true must NOT be
// returned by huntNpcs.
func TestHuntNpcsExcludesDelayedNpc(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone, Category: -1}
	hunter := newRegisteredNpc(t, s, typ, true)
	hunter.huntRange = 5
	target := newRegisteredNpc(t, s, typ, true)
	target.delayed = true
	target.delayedUntil = s.currentTick + 5
	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisOff}
	got := hunter.huntNpcs(s, hunt)
	for _, e := range got {
		if other, ok := e.(*Npc); ok && other.nid == target.nid {
			t.Errorf("huntNpcs returned delayed NPC nid=%d; IsValid filter should skip it (TS Npc.ts:370-375)", target.nid)
		}
	}
}

// TestHuntObjsExcludesInactiveOrDepletedObj pins npc-hunt-2: TS
// Zone.getAllObjsSafe (Zone.ts:411-417) gates yielded objs on
// obj.isValid(); Obj.isValid (Obj.ts:52-62) returns false when count<1
// or when the base !isActive check (Entity.ts:32-34) fails. (The
// reveal/hash64 arm is not exercised — huntObjs passes no hash64.)
// goscape's huntObjs must mirror that gate — inactive or depleted
// (count=0) objs must NOT be returned, while a healthy seed must.
func TestHuntObjsExcludesInactiveOrDepletedObj(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	healthy := addObjToZone(t, s, n.level, n.x+1, n.z+1, 1, -1)
	inactive := addObjToZone(t, s, n.level, n.x+2, n.z+2, 1, -1)
	inactive.IsActive = false
	depleted := addObjToZone(t, s, n.level, n.x+3, n.z+3, 1, -1)
	depleted.Count = 0

	hunted := n.huntObjs(s, &objtype.HuntType{CheckObj: -1, CheckCategory: -1})

	var sawHealthy, sawInactive, sawDepleted bool
	for _, e := range hunted {
		switch o := e.(type) {
		case *entitypkg.Obj:
			switch o {
			case healthy:
				sawHealthy = true
			case inactive:
				sawInactive = true
			case depleted:
				sawDepleted = true
			}
		}
	}
	if sawInactive {
		t.Errorf("huntObjs returned inactive obj; isValid filter should skip it (TS Obj.ts:52-62 → Entity.ts:32-34)")
	}
	if sawDepleted {
		t.Errorf("huntObjs returned depleted (count=0) obj; isValid filter should skip it (TS Obj.ts:57-59)")
	}
	if !sawHealthy {
		t.Errorf("huntObjs did not return healthy obj; expected the healthy seed in results")
	}
}

// TestHuntLocsExcludesInactiveLoc pins npc-hunt-3: TS
// Zone.getAllLocsSafe (Zone.ts:459-465) gates yielded locs on
// loc.isValid(); Loc inherits the base Entity.isValid predicate
// (Entity.ts:32-34) which returns isActive. goscape's huntLocs must
// mirror that gate — an inactive loc must NOT be returned, while a
// healthy seed must.
func TestHuntLocsExcludesInactiveLoc(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	healthy := addLocToZone(t, s, n.level, n.x+1, n.z+1, 1000, -1)
	inactive := addLocToZone(t, s, n.level, n.x+2, n.z+2, 1000, -1)
	inactive.IsActive = false

	hunted := n.huntLocs(s, &objtype.HuntType{CheckLoc: -1, CheckCategory: -1})

	var sawHealthy, sawInactive bool
	for _, e := range hunted {
		if l, ok := e.(*entitypkg.Loc); ok {
			switch l {
			case healthy:
				sawHealthy = true
			case inactive:
				sawInactive = true
			}
		}
	}
	if sawInactive {
		t.Errorf("huntLocs returned inactive loc; isValid filter should skip it (TS Entity.ts:32-34)")
	}
	if !sawHealthy {
		t.Errorf("huntLocs did not return healthy loc; expected the healthy seed in results")
	}
}
