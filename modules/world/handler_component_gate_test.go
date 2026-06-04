package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/zone"
)

// compGateCase drives the 4-scenario gate test for an Op*T/U handler.
//
// payloadOK is a payload that would otherwise pass all gates (entity exists,
// is visible, listener resolves, etc). The helper rewrites or relies on the
// payload's component-id field to drive the gate-failure scenarios.
type compGateCase struct {
	name             string
	handler          func(*Player, []byte) error
	setupOk          func(t *testing.T, s *Server, p *Player) // seeds prerequisite state for happy-path
	payloadOK        []byte
	rootLayer        int  // RootLayer for the test component; placed at p.tabs[0] to satisfy IsComponentVisible
	flagBits         int  // T-variant: ActionTarget bitmask. U-variant: 0.
	isUVariant       bool // U: gate boolean field (Usable or Interactable). T: gate ActionTarget bits.
	usesInteractable bool // U-variant 244: gate Interactable instead of Usable. TS OpNpcUHandler.ts:24.
	comId            int  // component id referenced by payloadOK
}

// setUFlag sets the appropriate U-variant boolean field on ct.
// At 225 the gate used Usable; at 244 some handlers (e.g. OpNpcU) use Interactable.
// TS OpNpcUHandler.ts:24 (244): `!com.interactable`.
func (c *compGateCase) setUFlag(ct *objtype.ComponentType, pass bool) {
	if c.usesInteractable {
		ct.Interactable = pass
	} else {
		ct.Usable = pass
	}
}

// runCompGate exercises 4 scenarios per handler:
//  1. nil component (registry empty for c.comId)
//  2. flag fail (T: ActionTarget=0; U: Usable=false or Interactable=false)
//  3. not visible (component registered but RootLayer not in any tab/modal)
//  4. happy-path (all gates pass)
func runCompGate(t *testing.T, c compGateCase) {
	t.Helper()

	// Scenario 1: nil component (no registry seed).
	t.Run(c.name+"/nil_component_rejects", func(t *testing.T) {
		s := newTestServer(t)
		p, _ := newTestPlayer(t)
		p.client.server = s
		c.setupOk(t, s, p)
		// no seedComponentTypes call → registry has no entry for comId

		err := c.handler(p, c.payloadOK)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		assertGateRejected(t, p, "nil component should reject")
	})

	// Scenario 2: flag fail.
	t.Run(c.name+"/flag_fail_rejects", func(t *testing.T) {
		s := newTestServer(t)
		p, _ := newTestPlayer(t)
		p.client.server = s
		c.setupOk(t, s, p)
		ct := &objtype.ComponentType{RootLayer: c.rootLayer}
		if !c.isUVariant {
			ct.ActionTarget = 0 // no bits set — gate's correct bit absent
		} else {
			c.setUFlag(ct, false)
		}
		seedComponentTypes(t, s, map[int]*objtype.ComponentType{c.comId: ct})
		p.tabs[0] = c.rootLayer

		err := c.handler(p, c.payloadOK)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		assertGateRejected(t, p, "flag fail should reject")
	})

	// Scenario 3: not visible (component exists with passing flag, but root not in any slot).
	t.Run(c.name+"/not_visible_rejects", func(t *testing.T) {
		s := newTestServer(t)
		p, _ := newTestPlayer(t)
		p.client.server = s
		c.setupOk(t, s, p)
		ct := &objtype.ComponentType{RootLayer: c.rootLayer}
		if !c.isUVariant {
			ct.ActionTarget = c.flagBits
		} else {
			c.setUFlag(ct, true)
		}
		seedComponentTypes(t, s, map[int]*objtype.ComponentType{c.comId: ct})
		// note: do NOT set p.tabs[0] — root invisible

		err := c.handler(p, c.payloadOK)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		assertGateRejected(t, p, "not visible should reject")
	})

	// Scenario 4: happy-path.
	t.Run(c.name+"/happy_path_accepts", func(t *testing.T) {
		s := newTestServer(t)
		p, _ := newTestPlayer(t)
		p.client.server = s
		c.setupOk(t, s, p)
		ct := &objtype.ComponentType{RootLayer: c.rootLayer}
		if !c.isUVariant {
			ct.ActionTarget = c.flagBits
		} else {
			c.setUFlag(ct, true)
		}
		seedComponentTypes(t, s, map[int]*objtype.ComponentType{c.comId: ct})
		p.tabs[0] = c.rootLayer

		err := c.handler(p, c.payloadOK)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if !p.opcalled {
			t.Errorf("opcalled: got false, want true (gate should pass)")
		}
		if p.target == nil {
			t.Errorf("target: got nil, want non-nil entity (SetInteraction should fire)")
		}
	})
}

// assertGateRejected verifies the handler bailed without setting interaction
// state. opcalled and p.target are the load-bearing post-gate side effects.
func assertGateRejected(t *testing.T, p *Player, msg string) {
	t.Helper()
	if p.opcalled {
		t.Errorf("opcalled: got true, want false (%s)", msg)
	}
	if p.target != nil {
		t.Errorf("target: got non-nil, want nil (%s)", msg)
	}
}

func TestComponentGate_OpNpcT(t *testing.T) {
	const npcSlot = 0
	const spellCom = 4242
	const rootLayer = 4242
	runCompGate(t, compGateCase{
		name:      "OpNpcT",
		handler:   handleOpNpcT,
		comId:     spellCom,
		flagBits:  objtype.ComActionTargetNpc,
		rootLayer: rootLayer,
		payloadOK: []byte{0, npcSlot, spellCom >> 8, spellCom & 0xFF},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedNpcAtSlot(t, s, p, npcSlot)
		},
	})
}

func TestComponentGate_OpObjT(t *testing.T) {
	const x, z = 100, 100
	const objId = 42
	const spellCom = 4243
	const rootLayer = 4243
	runCompGate(t, compGateCase{
		name:      "OpObjT",
		handler:   handleOpObjT,
		comId:     spellCom,
		flagBits:  objtype.ComActionTargetObj,
		rootLayer: rootLayer,
		payloadOK: []byte{
			x >> 8, x & 0xFF,
			z >> 8, z & 0xFF,
			objId >> 8, objId & 0xFF,
			spellCom >> 8, spellCom & 0xFF,
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedObjAt(t, s, p, x, z, objId)
		},
	})
}

func TestComponentGate_OpLocT(t *testing.T) {
	const x, z = 100, 100
	const locId = 42
	const spellCom = 4244
	const rootLayer = 4244
	runCompGate(t, compGateCase{
		name:      "OpLocT",
		handler:   handleOpLocT,
		comId:     spellCom,
		flagBits:  objtype.ComActionTargetLoc,
		rootLayer: rootLayer,
		payloadOK: []byte{
			x >> 8, x & 0xFF,
			z >> 8, z & 0xFF,
			locId >> 8, locId & 0xFF,
			spellCom >> 8, spellCom & 0xFF,
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedLocAt(t, s, p, x, z, locId)
		},
	})
}

func TestComponentGate_OpPlayerT(t *testing.T) {
	const otherSlot = 1
	const spellCom = 4245
	const rootLayer = 4245
	runCompGate(t, compGateCase{
		name:      "OpPlayerT",
		handler:   handleOpPlayerT,
		comId:     spellCom,
		flagBits:  objtype.ComActionTargetPlayer,
		rootLayer: rootLayer,
		payloadOK: []byte{0, otherSlot, spellCom >> 8, spellCom & 0xFF},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedTargetPlayerAtSlot(t, s, p, otherSlot)
		},
	})
}

func TestComponentGate_OpNpcU(t *testing.T) {
	const npcSlot = 0
	const useObj = 1511
	const useSlot = 3
	const useCom = 4246
	const rootLayer = 4246
	runCompGate(t, compGateCase{
		name:             "OpNpcU",
		handler:          handleOpNpcU,
		comId:            useCom,
		isUVariant:       true,
		usesInteractable: true, // 244: gate changed from Usable to Interactable. TS OpNpcUHandler.ts:24.
		rootLayer:        rootLayer,
		payloadOK: []byte{
			0, npcSlot,
			useObj >> 8, useObj & 0xFF,
			useSlot >> 8, useSlot & 0xFF,
			useCom >> 8, useCom & 0xFF,
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedNpcAtSlot(t, s, p, npcSlot)
			seedListenerWithItem(t, s, p, useCom, useSlot, useObj)
		},
	})
}

func TestComponentGate_OpObjU(t *testing.T) {
	const x, z = 100, 100
	const objId = 42
	const useObj = 1511
	const useSlot = 3
	const useCom = 4247
	const rootLayer = 4247
	runCompGate(t, compGateCase{
		name:             "OpObjU",
		handler:          handleOpObjU,
		comId:            useCom,
		isUVariant:       true,
		usesInteractable: true, // 244: gate changed from Usable to Interactable. TS OpObjUHandler.ts:22.
		rootLayer:        rootLayer,
		payloadOK: []byte{
			x >> 8, x & 0xFF,
			z >> 8, z & 0xFF,
			objId >> 8, objId & 0xFF,
			useObj >> 8, useObj & 0xFF,
			useSlot >> 8, useSlot & 0xFF,
			useCom >> 8, useCom & 0xFF,
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedObjAt(t, s, p, x, z, objId)
			seedListenerWithItem(t, s, p, useCom, useSlot, useObj)
		},
	})
}

func TestComponentGate_OpLocU(t *testing.T) {
	const x, z = 100, 100
	const locId = 42
	const useObj = 1511
	const useSlot = 3
	const useCom = 4248
	const rootLayer = 4248
	runCompGate(t, compGateCase{
		name:       "OpLocU",
		handler:    handleOpLocU,
		comId:      useCom,
		isUVariant: true,
		rootLayer:  rootLayer,
		payloadOK: []byte{
			x >> 8, x & 0xFF,
			z >> 8, z & 0xFF,
			locId >> 8, locId & 0xFF,
			useObj >> 8, useObj & 0xFF,
			useSlot >> 8, useSlot & 0xFF,
			useCom >> 8, useCom & 0xFF,
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedLocAt(t, s, p, x, z, locId)
			seedListenerWithItem(t, s, p, useCom, useSlot, useObj)
		},
	})
}

func TestComponentGate_OpPlayerU(t *testing.T) {
	const otherSlot = 1
	const useObj = 1511
	const useSlot = 3
	const useCom = 4249
	const rootLayer = 4249
	runCompGate(t, compGateCase{
		name:       "OpPlayerU",
		handler:    handleOpPlayerU,
		comId:      useCom,
		isUVariant: true,
		rootLayer:  rootLayer,
		payloadOK: []byte{
			0, otherSlot,
			useObj >> 8, useObj & 0xFF,
			useSlot >> 8, useSlot & 0xFF,
			useCom >> 8, useCom & 0xFF,
		},
		setupOk: func(t *testing.T, s *Server, p *Player) {
			seedTargetPlayerAtSlot(t, s, p, otherSlot)
			seedListenerWithItem(t, s, p, useCom, useSlot, useObj)
		},
	})
}

// seedListenerWithItem registers an inv listener at useCom pointing at world-
// shared invType=93, populates that inv with useObj at useSlot.
func seedListenerWithItem(t *testing.T, s *Server, p *Player, useCom, useSlot, useObj int) {
	t.Helper()
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[useSlot] = &inventory.Item{Id: useObj, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, useCom, -1)
}

// seedNpcAtSlot installs a live NPC at the given slot and subscribes it to
// the player's rsbuf tracking, mirroring the prerequisite-seeding part of
// makeOpNpcFixture. Does NOT seed component types.
func seedNpcAtSlot(t *testing.T, s *Server, p *Player, slot int) {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test"},
		Op:          []string{"Attack", "Talk", "Examine", "Option4", "Option5"},
		WanderRange: 0,
		RespawnRate: 50,
	}
	// nid = slot+1 so nid 0 is never used (nid 0 in rsbuf means absent).
	npcNid := slot + 1
	npc := NewNpc(npcNid, 0, 100, 100, 0, typ)
	npc.nid = npcNid
	s.npcs[slot] = npc
	s.npcLoop = append(s.npcLoop, npc)

	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0

	p.slot = slot + 100 // use an offset so player slot doesn't collide with npc slot
	s.players[p.slot] = p
	s.rsbuf.AddPlayer(int32(p.slot))
	s.rsbuf.SubscribeNpcForTest(int32(p.slot), int32(npcNid))
}

// seedObjAt places an Obj at (x, z) with the given objId in the server's zone
// map, and registers an ObjType entry for objId. Mirrors the prerequisite-
// seeding part of makeOpObjFixture. Does NOT seed component types.
func seedObjAt(t *testing.T, s *Server, p *Player, x, z, objId int) {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	// Ensure objTypes slice is large enough.
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{
			Configs: make([]*objtype.ObjType, objId+1),
		}
	}
	for len(s.objTypes.Configs) <= objId {
		s.objTypes.Configs = append(s.objTypes.Configs, nil)
	}
	s.objTypes.Configs[objId] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: objId, DebugName: "test_obj"},
		Op:         []string{"op1", "op2", "op3", "op4", "op5"},
	}

	obj := entitypkg.NewObj(0, x, z, entitypkg.LifecycleDespawn, objId, 1)
	obj.IsActive = true // L9: GetObj filters !IsActive; production AddObj sets this.
	zn := s.zoneMap.Get(0, x, z)
	zn.Objs = append(zn.Objs, obj)

	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = x-1, z, 0
	p.originX, p.originZ = x, z
}

// seedLocAt places a Loc at (x, z) with the given locId in the server's zone
// map, and registers a LocType entry for locId. Mirrors the prerequisite-
// seeding part of makeOpLocFixture. Does NOT seed component types.
func seedLocAt(t *testing.T, s *Server, p *Player, x, z, locId int) {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	// Ensure locTypes slice is large enough.
	if s.locTypes == nil {
		s.locTypes = &objtype.LocTypeConfigs{
			Configs: make([]*objtype.LocType, locId+1),
		}
	}
	for len(s.locTypes.Configs) <= locId {
		s.locTypes.Configs = append(s.locTypes.Configs, nil)
	}
	s.locTypes.Configs[locId] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: locId, DebugName: "test_loc"},
		Category:   7,
		Op:         []string{"op1", "op2", "op3", "op4", "op5"},
	}

	loc := entitypkg.NewLoc(0, x, z, 1, 1, entitypkg.LifecycleForever, locId, 10, 0)
	zn := s.zoneMap.Get(0, x, z)
	zn.Locs = append(zn.Locs, loc)
	// Mirror pkg/zone.Zone.AddStaticLoc — raw-append leaves IsActive=false,
	// which the new Server.GetLoc isActive filter (TS Zone.ts:471-477) would
	// strip. Tests using this helper exercise success-path OpLoc / component
	// flows that need GetLoc to return a non-nil loc.
	loc.IsActive = true

	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = x-1, z, 0
	p.originX, p.originZ = x, z
}

// seedTargetPlayerAtSlot installs a second player at the given slot and makes
// it visible to p via rsbuf.HasPlayer. Mirrors the prerequisite-seeding part
// of makeOpPlayerFixture. Does NOT seed component types.
func seedTargetPlayerAtSlot(t *testing.T, s *Server, p *Player, slot int) {
	t.Helper()
	other, _ := newTestPlayer(t)
	other.client.server = s
	other.slot = slot
	s.players[slot] = other
	s.rsbuf.AddPlayer(int32(slot))

	// Wire the clicker player into the server.
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.slot = slot + 100 // avoid collision with target slot
	s.players[p.slot] = p
	s.rsbuf.AddPlayer(int32(p.slot))

	// Make the target visible to the clicker.
	bp := s.rsbuf.PlayerForTest(int32(p.slot))
	if bp == nil {
		t.Fatalf("rsbuf has no player at observer slot %d", p.slot)
	}
	bp.Build.Players.Insert(int32(slot))
}

// runProtectScript registers a script for (trigger, comId) that runs
// P_DELAY (which requires Protect=true via requireProtectedActivePlayer),
// invokes handlerFn against a Server seeded with the rootLayer fixture,
// and reports whether the script suspended (handler computed protect=true)
// or aborted (handler computed protect=false).
//
// rootOverlay sets Overlay on the rootLayer component (when includeRoot).
// includeRoot=false omits the root component entirely → lookupComponent
// returns nil → protect should default to true.
func runProtectScript(
	t *testing.T,
	trigger script.ServerTriggerType,
	comId int,
	rootLayer int,
	rootOverlay bool,
	includeRoot bool,
	registerExtra func(*Server, *Player), // e.g., listener + inv setup
	invokeHandler func(*Server, *Player) error,
	componentExtras *objtype.ComponentType, // additional fields like InventoryOptions, Draggable
) bool {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}

	sf := &script.ScriptFile{
		Name:             "[trigger,com]",
		LookupKey:        script.LookupKeyForType(trigger, comId),
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpPDelay, script.OpReturn},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(sf)

	com := &objtype.ComponentType{RootLayer: rootLayer}
	if componentExtras != nil {
		com.InventoryOptions = componentExtras.InventoryOptions
		com.Draggable = componentExtras.Draggable
	}
	components := map[int]*objtype.ComponentType{comId: com}
	if includeRoot {
		components[rootLayer] = &objtype.ComponentType{RootLayer: rootLayer, Overlay: rootOverlay}
	}
	p, _ := newTestPlayer(t)
	p.client.server = s
	// script-core-1: the new player-anchored script-error reporter writes
	// MessageGame frames to the wire on Execute error (P_DELAY's protect
	// reject path now spams 4+ frames). Wire an encryptor so the error
	// branch can encode opcodes without nil-deref. Unconsumed bytes are
	// fine — the test asserts on activeScript state, not wire content.
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	seedComponentTypes(t, s, components)
	p.tabs[0] = rootLayer

	if registerExtra != nil {
		registerExtra(s, p)
	}

	if err := invokeHandler(s, p); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return p.activeScript != nil && p.activeScript.Execution == script.Suspended
}
