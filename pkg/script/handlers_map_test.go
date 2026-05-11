package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// MAP_PLAYERCOUNT (opcode 1015) — NAI-35-T2.
// Mirrors TS ServerOps.ts:27-45. Pops two coords (rect bounds) and pushes
// the count of players whose (x, z) falls inside the rect on from.level.

func TestHandleMapPlayerCount_EmptyRect(t *testing.T) {
	sf := newSingleOp("map_playercount_empty", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = &mockPlayerLookup{}
	state.PushInt(coordgrid.PackCoord(0, 100, 100)) // c1
	state.PushInt(coordgrid.PackCoord(0, 110, 110)) // c2 (top of stack)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("count: got %d, want 0", got)
	}
}

func TestHandleMapPlayerCount_SinglePlayerInRect(t *testing.T) {
	p := &mockPlayer{x: 105, z: 105}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, (105 >> 3) << 3, (105 >> 3) << 3}: {p},
		},
	}
	sf := newSingleOp("map_playercount_one", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(coordgrid.PackCoord(0, 100, 100))
	state.PushInt(coordgrid.PackCoord(0, 110, 110))
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 1 {
		t.Errorf("count: got %d, want 1", got)
	}
}

func TestHandleMapPlayerCount_PlayerAtRectBoundary(t *testing.T) {
	// Inclusive boundary at fromX (TS line 36: x >= from.x).
	p := &mockPlayer{x: 100, z: 105}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, (100 >> 3) << 3, (105 >> 3) << 3}: {p},
		},
	}
	sf := newSingleOp("map_playercount_boundary", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(coordgrid.PackCoord(0, 100, 100))
	state.PushInt(coordgrid.PackCoord(0, 110, 110))
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 1 {
		t.Errorf("inclusive-boundary count: got %d, want 1", got)
	}
}

func TestHandleMapPlayerCount_PlayerOutsideRect(t *testing.T) {
	p := &mockPlayer{x: 95, z: 95}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, (95 >> 3) << 3, (95 >> 3) << 3}: {p},
		},
	}
	sf := newSingleOp("map_playercount_outside", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(coordgrid.PackCoord(0, 100, 100))
	state.PushInt(coordgrid.PackCoord(0, 110, 110))
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("count: got %d, want 0", got)
	}
}

func TestHandleMapPlayerCount_PlayerInZoneButOutsideRect(t *testing.T) {
	// Player at (97, 105). Zone (97>>3)<<3 = 96 IS within the iteration
	// window (zones 96..112 cover rect 100..110), so the inner rect filter
	// is reached. But 97 < 100 → inner conjunction's `p.X() >= fromX`
	// rejects. Pins the inner-filter false branch (TestPlayerOutsideRect
	// exercises only the zone-window pre-filter).
	p := &mockPlayer{x: 97, z: 105}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, (97 >> 3) << 3, (105 >> 3) << 3}: {p},
		},
	}
	sf := newSingleOp("map_playercount_in_zone_out_of_rect", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(coordgrid.PackCoord(0, 100, 100))
	state.PushInt(coordgrid.PackCoord(0, 110, 110))
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("in-zone-out-of-rect count: got %d, want 0", got)
	}
}

func TestHandleMapPlayerCount_CrossLevelRectIgnoresToLevel(t *testing.T) {
	// NAI-35-D1: TS uses from.level only; to.level is silently ignored.
	// Player on level 1, from.level=0 → NOT counted (level-0 zones are
	// empty in this fixture).
	p := &mockPlayer{x: 105, z: 105}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{1, (105 >> 3) << 3, (105 >> 3) << 3}: {p},
		},
	}
	sf := newSingleOp("map_playercount_d1", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(coordgrid.PackCoord(0, 100, 100)) // from.level = 0
	state.PushInt(coordgrid.PackCoord(1, 110, 110)) // to.level = 1 (ignored)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("cross-level count (D1): got %d, want 0", got)
	}
}

// MAP_FINDSQUARE (opcode 1009) — NAI-35-T6.
// Mirrors TS ServerOps.ts:254-374. Pops [coord, minRadius, maxRadius, type]
// and pushes a packed coord of a free walkable square near origin (or the
// origin coord on exhaustion).

// blockKey indexes blocked tiles by (level, x, z) for mapFindSquareWorld.
type blockKey struct{ level, x, z int }

// xzKey indexes F2P tiles by (x, z) (level-agnostic, mirroring TS
// gameMap.isFreeToPlay signature).
type xzKey struct{ x, z int }

// mapFindSquareWorld extends mockWorld with the IsMapBlocked / IsFreeToPlay
// surface MAP_FINDSQUARE needs. Embeds *mockWorld so all existing WorldVars
// methods (CurrentTick / PlayerCount / MapLive / VarsInt etc.) are inherited;
// MapMembers is overridden to allow a F2P (members=0) test case.
type mapFindSquareWorld struct {
	*mockWorld
	blockedTiles map[blockKey]bool
	f2pTiles     map[xzKey]bool
	members      int
}

func newMapFindSquareWorld() *mapFindSquareWorld {
	return &mapFindSquareWorld{
		mockWorld:    newMockWorld(),
		blockedTiles: make(map[blockKey]bool),
		f2pTiles:     make(map[xzKey]bool),
	}
}

func (w *mapFindSquareWorld) IsMapBlocked(level, x, z int) bool {
	return w.blockedTiles[blockKey{level, x, z}]
}

func (w *mapFindSquareWorld) IsFreeToPlay(x, z int) bool {
	return w.f2pTiles[xzKey{x, z}]
}

// MapMembers overrides mockWorld.MapMembers to allow tests to flip between
// members (1) and free worlds (0).
func (w *mapFindSquareWorld) MapMembers() int { return w.members }

func TestHandleMapFindSquare_NoneType_FindsFreeSquareWithinRadius(t *testing.T) {
	// members world (members=1) → freeWorld=false → IsFreeToPlay never
	// gates. No blocked tiles → first random candidate within radius
	// always succeeds. Bound check (|dx|, |dz| ≤ 5) is property-deterministic
	// regardless of rand.IntN's output.
	w := newMapFindSquareWorld()
	w.members = 1

	originLevel, originX, originZ := 0, 3200, 3200
	sf := newSingleOp("map_findsquare_none", OpMapFindSquare)
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	state.PushInt(coordgrid.PackCoord(originLevel, originX, originZ)) // coord
	state.PushInt(1)                                                  // minRadius
	state.PushInt(5)                                                  // maxRadius
	state.PushInt(int(MapFindSquareNone))                             // type (top of stack)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := coordgrid.UnpackCoord(state.PopInt())
	if got.Level != originLevel {
		t.Errorf("level: got %d, want %d", got.Level, originLevel)
	}
	dx, dz := got.X-originX, got.Z-originZ
	if dx < 0 {
		dx = -dx
	}
	if dz < 0 {
		dz = -dz
	}
	if dx > 5 || dz > 5 {
		t.Errorf("delta out of maxRadius: dx=%d dz=%d (want both ≤ 5)", dx, dz)
	}
	// minRadius=1 means at least one of |dx|,|dz| ≥ 1 (max(|dx|,|dz|) ≥ 1).
	if max(dx, dz) < 1 {
		t.Errorf("delta inside minRadius: dx=%d dz=%d (want max ≥ 1)", dx, dz)
	}
}

func TestHandleMapFindSquare_AllBlocked_ReturnsOriginCoord(t *testing.T) {
	// Block every tile in the 11×11 region (origin ± 5). Random branch
	// will exhaust 50 attempts → fall through to PushInt(coord). Outcome
	// is deterministic regardless of rand draws.
	w := newMapFindSquareWorld()
	w.members = 1

	originLevel, originX, originZ := 0, 3200, 3200
	for x := originX - 5; x <= originX+5; x++ {
		for z := originZ - 5; z <= originZ+5; z++ {
			w.blockedTiles[blockKey{originLevel, x, z}] = true
		}
	}

	coord := coordgrid.PackCoord(originLevel, originX, originZ)
	sf := newSingleOp("map_findsquare_all_blocked", OpMapFindSquare)
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	state.PushInt(coord)
	state.PushInt(1) // minRadius
	state.PushInt(5) // maxRadius
	state.PushInt(int(MapFindSquareNone))
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != coord {
		t.Errorf("got %d, want origin coord %d (TS line 373 fall-through)", got, coord)
	}
}

func TestHandleMapFindSquare_F2PTileRejectedInFreeWorld(t *testing.T) {
	// members=0 (free world) → freeWorld=true. With f2pTiles empty, every
	// candidate fails the IsFreeToPlay gate → fall through to origin coord.
	// Deterministic regardless of rand draws.
	w := newMapFindSquareWorld()
	w.members = 0

	originLevel, originX, originZ := 0, 3200, 3200
	coord := coordgrid.PackCoord(originLevel, originX, originZ)
	sf := newSingleOp("map_findsquare_f2p_reject", OpMapFindSquare)
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	state.PushInt(coord)
	state.PushInt(1)
	state.PushInt(5)
	state.PushInt(int(MapFindSquareNone))
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != coord {
		t.Errorf("free-world fallthrough: got %d, want origin coord %d", got, coord)
	}
}

func TestHandleMapFindSquare_TypeValidationRejectsInvalid(t *testing.T) {
	// Validation order: checkNumberPositive(min), checkNumberPositive(max),
	// checkFindSquareType(type), checkCoord(coord). Push valid radii, then
	// invalid type → errors on type.
	w := newMapFindSquareWorld()
	w.members = 1

	originLevel, originX, originZ := 0, 3200, 3200
	sf := newSingleOp("map_findsquare_invalid_type", OpMapFindSquare)
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	state.PushInt(coordgrid.PackCoord(originLevel, originX, originZ))
	state.PushInt(1)  // minRadius
	state.PushInt(5)  // maxRadius
	state.PushInt(99) // invalid type (top of stack)
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: expected error for invalid type, got nil")
	}
	if !strings.Contains(err.Error(), "MAP_FINDSQUARE") {
		t.Errorf("error %q does not mention MAP_FINDSQUARE", err.Error())
	}
}

func TestHandleMapFindSquare_NumberPositiveValidation(t *testing.T) {
	// minRadius=-1 is the FIRST value validated → checkNumberPositive fires.
	// (TS NumberPositive accepts zero — see ScriptValidators.ts:43-48.)
	w := newMapFindSquareWorld()
	w.members = 1

	originLevel, originX, originZ := 0, 3200, 3200
	sf := newSingleOp("map_findsquare_neg_min", OpMapFindSquare)
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	state.PushInt(coordgrid.PackCoord(originLevel, originX, originZ))
	state.PushInt(-1) // minRadius (invalid: negative)
	state.PushInt(5)  // maxRadius
	state.PushInt(int(MapFindSquareNone))
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: expected error for minRadius=-1, got nil")
	}
	if !strings.Contains(err.Error(), "MAP_FINDSQUARE") {
		t.Errorf("error %q does not mention MAP_FINDSQUARE", err.Error())
	}
}

// --- NAI-36 Task 4: MAP_BLOCKED Layer 1 unit tests -----------------------

// runMapOp executes a single map opcode against the given world fixture
// and returns the post-execution state. Pass c=nil for tests that don't
// exercise the Configs lookup.
func runMapOp(t *testing.T, w WorldVars, c Configs, op Opcode, intInputs []int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := &ScriptState{
		Script:      sf,
		World:       w,
		Configs:     c,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return state
}

// mapBlockedWorld extends mockWorld with controllable IsMapBlocked +
// IsFreeToPlay return values for the 4-branch coverage.
type mapBlockedWorld struct {
	mockWorld
	mapBlocked bool
	freeToPlay bool
}

func (w *mapBlockedWorld) IsMapBlocked(level, x, z int) bool { return w.mapBlocked }
func (w *mapBlockedWorld) IsFreeToPlay(x, z int) bool        { return w.freeToPlay }

func TestMapBlocked_MembersWorldClearTilePushes0(t *testing.T) {
	w := &mapBlockedWorld{mockWorld: mockWorld{mapMembers: 1}, mapBlocked: false}
	state := runMapOp(t, w, nil, OpMapBlocked, []int{(0 << 28) | (3200 << 14) | 3300})

	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("members-world clear tile: got top=%d ISP=%d, want top=0 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

func TestMapBlocked_MembersWorldBlockedTilePushes1(t *testing.T) {
	w := &mapBlockedWorld{mockWorld: mockWorld{mapMembers: 1}, mapBlocked: true}
	state := runMapOp(t, w, nil, OpMapBlocked, []int{(0 << 28) | (3200 << 14) | 3300})

	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("members-world blocked tile: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// F2P-world non-F2P tile: short-circuits to push 1 BEFORE the IsMapBlocked
// check. Tests the early-return per TS ServerOps.ts:132-135.
func TestMapBlocked_F2PWorldNonF2PTilePushes1(t *testing.T) {
	w := &mapBlockedWorld{
		mockWorld:  mockWorld{mapMembers: 0}, // F2P world
		mapBlocked: false,                    // would push 0 if reached
		freeToPlay: false,                    // tile is NOT F2P
	}
	state := runMapOp(t, w, nil, OpMapBlocked, []int{(0 << 28) | (3200 << 14) | 3300})

	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("F2P-world non-F2P tile: got top=%d ISP=%d, want top=1 ISP=1 (short-circuit)",
			state.IntStack[0], state.ISP)
	}
}

// F2P-world F2P tile: passes the gate; falls through to IsMapBlocked.
func TestMapBlocked_F2PWorldF2PTilePushesIsBlocked(t *testing.T) {
	w := &mapBlockedWorld{
		mockWorld:  mockWorld{mapMembers: 0}, // F2P world
		mapBlocked: true,
		freeToPlay: true, // tile IS F2P
	}
	state := runMapOp(t, w, nil, OpMapBlocked, []int{(0 << 28) | (3200 << 14) | 3300})

	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("F2P-world F2P-blocked tile: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// --- NAI-36 Task 5: SPOTANIM_MAP Layer 1 unit tests ----------------------

type spotAnimMapWorld struct {
	mockWorld
	animMapCalls []struct {
		level, x, z, spotanim, height, delay int
	}
}

func (w *spotAnimMapWorld) AnimMap(level, x, z, spotanim, height, delay int) {
	w.animMapCalls = append(w.animMapCalls, struct {
		level, x, z, spotanim, height, delay int
	}{level, x, z, spotanim, height, delay})
}

func TestSpotAnimMap_PopsValidatesAndDelegates(t *testing.T) {
	w := &spotAnimMapWorld{}
	m := &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}

	const spotanim, height, delay = 200, 50, 5
	const level, x, z = 0, 3200, 3300
	coord := (level << 28) | (x << 14) | z

	// Push order: spotanim first (deepest), then coord, then height, then delay (top).
	// Pop order in handler: delay (top), height, coord, spotanim (deepest).
	state := runMapOp(t, w, m, OpSpotAnimMap, []int{spotanim, coord, height, delay})
	_ = state

	if len(w.animMapCalls) != 1 {
		t.Fatalf("animMapCalls: got %d, want 1", len(w.animMapCalls))
	}
	got := w.animMapCalls[0]
	want := struct {
		level, x, z, spotanim, height, delay int
	}{level, x, z, spotanim, height, delay}
	if got != want {
		t.Errorf("animMapCalls[0]: got %+v, want %+v", got, want)
	}
}

func TestSpotAnimMap_InvalidCoordErrors(t *testing.T) {
	w := &spotAnimMapWorld{}
	state := &ScriptState{
		World:       w,
		Configs:     &mockConfigs{},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Push 4 ints with an out-of-range coord (-1).
	state.PushInt(200)
	state.PushInt(-1) // invalid coord
	state.PushInt(50)
	state.PushInt(5)

	err := handleSpotAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "SPOTANIM_MAP") {
		t.Errorf("invalid coord: got %v, want SPOTANIM_MAP error", err)
	}
	if len(w.animMapCalls) != 0 {
		t.Errorf("animMapCalls on error path: got %d, want 0", len(w.animMapCalls))
	}
}

// Pins post-NAI-58 negative-id rejection: checkSpotAnimType errors on
// id < 0 before any Configs lookup.
func TestSpotAnimMap_NegativeSpotanimIDErrors(t *testing.T) {
	w := &spotAnimMapWorld{}
	state := &ScriptState{
		World:       w,
		Configs:     &mockConfigs{},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(-1) // invalid spotanim id
	state.PushInt((0 << 28) | (3200 << 14) | 3300)
	state.PushInt(50)
	state.PushInt(5)

	err := handleSpotAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "SPOTANIM_MAP") {
		t.Errorf("negative spotanim id: got %v, want SPOTANIM_MAP error", err)
	}
}

func TestSpotAnimMap_ZeroDelayPassesThrough(t *testing.T) {
	w := &spotAnimMapWorld{}
	m := &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}

	const spotanim, height, delay = 200, 0, 0
	coord := (0 << 28) | (3200 << 14) | 3300

	_ = runMapOp(t, w, m, OpSpotAnimMap, []int{spotanim, coord, height, delay})

	if len(w.animMapCalls) != 1 {
		t.Fatalf("animMapCalls: got %d, want 1", len(w.animMapCalls))
	}
	got := w.animMapCalls[0]
	if got.height != 0 || got.delay != 0 {
		t.Errorf("zero height/delay: got height=%d delay=%d, want 0/0",
			got.height, got.delay)
	}
}

// TestSpotAnimMap_RegisteredIdPasses pins the positive arm of the
// post-NAI-58 SpotAnimTypeValid mirror: a registered id reaches
// World.AnimMap with the spotanim untouched.
func TestSpotAnimMap_RegisteredIdPasses(t *testing.T) {
	w := &spotAnimMapWorld{}
	m := &mockConfigs{
		spotAnimTypes: map[int]*objtype.SpotanimType{
			7: objtype.NewSpotanimType(7),
		},
	}

	const spotanim, height, delay = 7, 50, 5
	const level, x, z = 0, 3200, 3300
	coord := (level << 28) | (x << 14) | z

	_ = runMapOp(t, w, m, OpSpotAnimMap, []int{spotanim, coord, height, delay})

	if len(w.animMapCalls) != 1 {
		t.Fatalf("animMapCalls: got %d, want 1", len(w.animMapCalls))
	}
	got := w.animMapCalls[0]
	if got.spotanim != spotanim {
		t.Errorf("spotanim: got %d, want %d", got.spotanim, spotanim)
	}
}

// TestSpotAnimMap_UnregisteredIdRejects pins the post-NAI-58
// SpotAnimTypeValid mirror: an id that's non-negative but absent
// from the registry is rejected.
func TestSpotAnimMap_UnregisteredIdRejects(t *testing.T) {
	w := &spotAnimMapWorld{}
	m := &mockConfigs{
		spotAnimTypes: map[int]*objtype.SpotanimType{
			7: objtype.NewSpotanimType(7),
		},
	}
	state := &ScriptState{
		World:       w,
		Configs:     m,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(8) // unregistered spotanim id
	state.PushInt((0 << 28) | (3200 << 14) | 3300)
	state.PushInt(50)
	state.PushInt(5)

	err := handleSpotAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "SPOTANIM_MAP") {
		t.Errorf("unregistered spotanim id: got %v, want SPOTANIM_MAP error", err)
	}
	if len(w.animMapCalls) != 0 {
		t.Errorf("animMapCalls on error path: got %d, want 0", len(w.animMapCalls))
	}
}

// TestSpotAnimMap_NilEntryRejects covers the registry-has-key-but-nil-value
// edge: mockConfigs.spotAnimTypes[7] = nil → SpotAnimType(7) returns nil
// → validation rejects.
func TestSpotAnimMap_NilEntryRejects(t *testing.T) {
	w := &spotAnimMapWorld{}
	m := &mockConfigs{
		spotAnimTypes: map[int]*objtype.SpotanimType{
			7: nil,
		},
	}
	state := &ScriptState{
		World:       w,
		Configs:     m,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(7) // key present but value is nil
	state.PushInt((0 << 28) | (3200 << 14) | 3300)
	state.PushInt(50)
	state.PushInt(5)

	err := handleSpotAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "SPOTANIM_MAP") {
		t.Errorf("nil-value spotanim id: got %v, want SPOTANIM_MAP error", err)
	}
}

// --- NAI-114 Stage 2: MAP_LOCADDUNSAFE Layer 1 unit tests --------------

// mapLocAddUnsafeOps is a minimal LocOps fixture for MAP_LOCADDUNSAFE
// tests. Records nothing; provides controllable AllLocsInZone return.
// The other LocOps methods are not called by MAP_LOCADDUNSAFE; they
// satisfy the interface so a test-state can mount this as state.LocOps.
type mapLocAddUnsafeOps struct {
	zoneLocs []ActiveLoc
}

func (m *mapLocAddUnsafeOps) ChangeLoc(loc ActiveLoc, typ, shape, angle, duration int) error {
	return nil
}
func (m *mapLocAddUnsafeOps) AddLoc(level, x, z, typ, shape, angle, duration int) (ActiveLoc, error) {
	return nil, nil
}
func (m *mapLocAddUnsafeOps) RemoveLoc(loc ActiveLoc, duration int) error { return nil }
func (m *mapLocAddUnsafeOps) AnimLoc(loc ActiveLoc, seq int) error        { return nil }
func (m *mapLocAddUnsafeOps) LocsAtCoord(level, x, z int) []ActiveLoc     { return nil }
func (m *mapLocAddUnsafeOps) AllLocsInZone(level, x, z int) []ActiveLoc   { return m.zoneLocs }
func (m *mapLocAddUnsafeOps) GetLoc(level, x, z, typ int) ActiveLoc       { return nil }

// runMapLocAddUnsafe is the standard test harness: pushes the packed
// coord and dispatches the opcode through Execute (so registration is
// also exercised).
func runMapLocAddUnsafe(t *testing.T, locs []ActiveLoc, configs Configs, packedCoord int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_MAP_LOCADDUNSAFE",
		Opcodes:          []Opcode{OpMapLocAddUnsafe, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := &ScriptState{
		Script:      sf,
		LocOps:      &mapLocAddUnsafeOps{zoneLocs: locs},
		Configs:     configs,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(packedCoord)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return state
}

func TestMapLocAddUnsafe_EmptyZonePushes0(t *testing.T) {
	state := runMapLocAddUnsafe(t, nil, &fakeConfigs{}, coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("empty zone: got top=%d ISP=%d, want top=0 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// TS ServerOps.ts:218 — type.active !== 1 → continue.
func TestMapLocAddUnsafe_LocTypeActiveZeroSkipped(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 0 // explicit; default is -1 then PostDecode coerces, but we set directly
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	wallAtCoord := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  0, // LayerWall
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{wallAtCoord}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("LocType.Active=0 wall at coord: got top=%d ISP=%d, want top=0 ISP=1 (TS line 218 skip)",
			state.IntStack[0], state.ISP)
	}
}

// TS ServerOps.ts:224 — !loc.isActive && layer === LocLayer.WALL → continue.
// Distinguishes from TS line 218: this loc HAS LocType.Active=1, but its
// runtime IsActive flag (Loc.IsActive, zone-managed) is false.
func TestMapLocAddUnsafe_InactiveWallSkipped(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	inactiveWallAtCoord := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  0, // LayerWall
		active: false,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{inactiveWallAtCoord}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("inactive wall at coord: got top=%d ISP=%d, want top=0 ISP=1 (TS line 224 skip)",
			state.IntStack[0], state.ISP)
	}
}

// TS ServerOps.ts:228-232 — active WALL at coord → push 1.
func TestMapLocAddUnsafe_ActiveWallAtCoordPushes1(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeWallAtCoord := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  0, // LayerWall
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeWallAtCoord}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("active wall at coord: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// TS ServerOps.ts:228-232 inverse — active WALL not at coord → continue → push 0.
func TestMapLocAddUnsafe_ActiveWallNotAtCoordPushes0(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeWallElsewhere := fakeActiveLoc{
		id: 42, x: 105, z: 100, level: 0, // 5 tiles east of probe coord
		layer:  0, // LayerWall
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeWallElsewhere}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("active wall not at coord: got top=%d ISP=%d, want top=0 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// TS ServerOps.ts:233-243 — 1×1 GROUND at coord → push 1 (single iteration
// of the footprint loop, deltaX/deltaZ = anchor).
func TestMapLocAddUnsafe_GroundLayer1x1AtCoordPushes1(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	lt.Width = 1
	lt.Length = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeGround := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  2, // LayerGround
		angle:  0, // AngleWest (no width/length swap)
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeGround}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("1x1 ground at coord: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// TS ServerOps.ts:236-243 — 2×1 GROUND anchored at (100,100), AngleWest:
// width=2, length=1. Footprint covers (100,100) and (101,100). Probing
// (101,100) → push 1 (second iteration of the footprint loop).
func TestMapLocAddUnsafe_GroundLayer2x1FootprintCoversCoord(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	lt.Width = 2
	lt.Length = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeGround := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  2, // LayerGround
		angle:  0, // AngleWest
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeGround}, configs,
		coordgrid.PackCoord(0, 101, 100)) // probe one tile east
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("2x1 ground footprint covers (101,100): got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// TS ServerOps.ts:234-235 — AngleNorth/AngleSouth swap width and length.
// Anchor (100,100), Width=1, Length=2, AngleNorth → effective width=2,
// length=1; footprint covers (100,100) and (101,100). The original
// (101,100) probe must hit; the (100,101) probe (the unswapped axis)
// must miss.
func TestMapLocAddUnsafe_GroundLayerNorthAngleSwapsWidthLength(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	lt.Width = 1
	lt.Length = 2
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeGround := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  2,                   // LayerGround
		angle:  int(loc.AngleNorth), // 1
		active: true,
	}
	// Hit case: (101, 100) is covered by the swapped 2×1 footprint.
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeGround}, configs,
		coordgrid.PackCoord(0, 101, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("AngleNorth swap hit: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}

	// Miss case: (100, 101) — the unswapped Length axis — is NOT covered.
	state2 := runMapLocAddUnsafe(t, []ActiveLoc{activeGround}, configs,
		coordgrid.PackCoord(0, 100, 101))
	if state2.ISP != 1 || state2.IntStack[0] != 0 {
		t.Errorf("AngleNorth swap miss: got top=%d ISP=%d, want top=0 ISP=1",
			state2.IntStack[0], state2.ISP)
	}
}

// TS ServerOps.ts:244-249 — GROUND_DECOR at coord → push 1 (no footprint;
// exact tile match like WALL).
func TestMapLocAddUnsafe_GroundDecorAtCoordPushes1(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeGroundDecor := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  3, // LayerGroundDecor
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeGroundDecor}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("ground-decor at coord: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// TS ServerOps.ts:224 inverse — inactive ground/ground-decor locs are
// STILL checked (the WALL-only inactive-skip rule does not extend to
// other layers). Probes an inactive ground-decor at coord; expects push 1.
func TestMapLocAddUnsafe_InactiveGroundDecorStillChecked(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	inactiveGroundDecor := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  3, // LayerGroundDecor
		active: false,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{inactiveGroundDecor}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("inactive ground-decor at coord (TS line 224 inverse): got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// Coord validation (checkCoord) errors before the zone iteration begins.
// No push occurs; Execute returns the error tagged "MAP_LOCADDUNSAFE".
func TestMapLocAddUnsafe_NegativeCoordErrors(t *testing.T) {
	sf := &ScriptFile{
		Name:             "test_MAP_LOCADDUNSAFE",
		Opcodes:          []Opcode{OpMapLocAddUnsafe, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := &ScriptState{
		Script:      sf,
		LocOps:      &mapLocAddUnsafeOps{},
		Configs:     &fakeConfigs{},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(-1) // invalid coord
	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "MAP_LOCADDUNSAFE") {
		t.Errorf("negative coord: got err=%v, want error containing MAP_LOCADDUNSAFE", err)
	}
}

// Configs nil (defensive — the firemaking chain should not crash if a
// later state-builder forgets to wire Configs). Per the doc comment:
// nil Configs → all per-loc LocType lookups silently skip → push 0.
func TestMapLocAddUnsafe_ConfigsNilSkipsAllLocsPushes0(t *testing.T) {
	wallAtCoord := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  0, // LayerWall
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{wallAtCoord}, nil,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("nil Configs: got top=%d ISP=%d, want top=0 ISP=1",
			state.IntStack[0], state.ISP)
	}
}

// --- NAI-115 Task 4: LINEOFWALK (opcode 1006) unit tests ------------------

func TestHandleLineOfWalkSameLevelTrue(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1 // members world; F2P gate inert
	s.World = mw
	s.LineValidator = &stubLineValidator{lowReturn: true}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200)) // c1 (from)
	s.PushInt(coordgrid.PackCoord(0, 3201, 3200)) // c2 (to)

	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("LINEOFWALK same-level true: got %d, want 1", got)
	}
}

func TestHandleLineOfWalkDifferentLevels(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = &stubLineValidator{lowReturn: true}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(coordgrid.PackCoord(1, 3200, 3200)) // different level

	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("LINEOFWALK different-level: got %d, want 0", got)
	}
}

func TestHandleLineOfWalkF2PShortCircuit(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 0 // F2P world; default IsFreeToPlay returns false → blocked
	s.World = mw
	s.LineValidator = &stubLineValidator{lowReturn: true}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(coordgrid.PackCoord(0, 3201, 3200))

	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("LINEOFWALK F2P-blocked: got %d, want 0", got)
	}
}

func TestHandleLineOfWalkNilValidator(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = nil

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(coordgrid.PackCoord(0, 3201, 3200))

	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk returned error: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("LINEOFWALK nil validator: got %d, want 0", got)
	}
}

// --- NAI-163 B1 T0: isLineOfSight wrapper arg-shape regression ---------------
// NAI-163-D-LOS-ARG-SHAPE-FIX: widens isLineOfSight from (1,0,0,0) to
// (1,1,1,0) to match TS GameMap.ts:429-431.

// stubLineValidatorArgs records every (Has)LineOfSight and (Has)LineOfWalk
// call's full arg tuple for the NAI-163-D-LOS-ARG-SHAPE-FIX and
// NAI-165-D-LOW-ARG-SHAPE-FIX regressions. Distinct from the existing
// npc_iterator_test.go recordingLineValidator (which only captures level +
// src/dest, not srcSize/destWidth/destLength/extraFlag).
type stubLineValidatorArgs struct {
	losCalls  []losCall
	losReturn bool
	lowCalls  []losCall
	lowReturn bool
}

type losCall struct {
	level, srcX, srcZ, destX, destZ           int
	srcSize, destWidth, destLength, extraFlag int
}

func (st *stubLineValidatorArgs) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	st.losCalls = append(st.losCalls, losCall{level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag})
	return st.losReturn
}

func (st *stubLineValidatorArgs) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	st.lowCalls = append(st.lowCalls, losCall{level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag})
	return st.lowReturn
}

func TestIsLineOfSightWrapper_PassesTSFaithfulArgShape(t *testing.T) {
	// Regression pin: TS GameMap.ts:430 calls
	//   rsmod.hasLineOfSight(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0)
	// goscape's srcSize expands to srcWidth=srcLength=1 inside RayCast
	// (linevalidator.go:21), so the TS-faithful arg tuple at the wrapper
	// level is srcSize=1, destWidth=1, destLength=1, extraFlag=0.
	// Pre-NAI-163-D-LOS-ARG-SHAPE-FIX the wrapper was (1, 0, 0, 0).
	st := &stubLineValidatorArgs{losReturn: true}
	s := &ScriptState{LineValidator: st}
	_ = isLineOfSight(s, 0, 3200, 3300, 3210, 3305)
	if len(st.losCalls) != 1 {
		t.Fatalf("expected 1 LineValidator call, got %d", len(st.losCalls))
	}
	got := st.losCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("isLineOfSight arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

// --- NAI-163 B1 T1: handleLineOfSight (LINEOFSIGHT, opcode 1005) tests ------
// Mirrors TS ServerOps.ts:144-162. Pop order (top first): c2 (to), c1 (from).
// Gate order: level-mismatch → F2P gate → LineValidator.

func TestHandleLineOfSight_LevelMismatch_PushZero(t *testing.T) {
	st := &stubLineValidatorArgs{losReturn: true}
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = st
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(1, 3200, 3300)) // to (c2) — different level
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("expected push 0 on level mismatch, got %d", got)
	}
	if len(st.losCalls) != 0 {
		t.Fatalf("LineValidator must not be called on level mismatch; got %d calls", len(st.losCalls))
	}
}

func TestHandleLineOfSight_F2PGate_NonMembersWorld_PushZero(t *testing.T) {
	st := &stubLineValidatorArgs{losReturn: true}
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 0 // F2P world; default IsFreeToPlay returns false → blocked
	s.World = mw
	s.LineValidator = st
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(0, 3210, 3305)) // to (c2) — not F2P
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("expected push 0 on F2P-gate block, got %d", got)
	}
	if len(st.losCalls) != 0 {
		t.Fatalf("LineValidator must not be called when F2P gate fires; got %d calls", len(st.losCalls))
	}
}

func TestHandleLineOfSight_F2PGate_MembersWorld_Bypasses(t *testing.T) {
	st := &stubLineValidatorArgs{losReturn: true}
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1 // members world; F2P gate inert
	s.World = mw
	s.LineValidator = st
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(0, 3210, 3305)) // to (c2)
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("expected push 1 (LV returns true, members world), got %d", got)
	}
	if len(st.losCalls) != 1 {
		t.Fatalf("LineValidator must be called once in members world; got %d calls", len(st.losCalls))
	}
}

func TestHandleLineOfSight_RayClear_PushOne(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = &stubLineValidator{losReturn: true}
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(0, 3210, 3305)) // to (c2)
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Fatalf("ray clear: expected 1, got %d", got)
	}
}

func TestHandleLineOfSight_RayBlocked_PushZero(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = &stubLineValidator{losReturn: false}
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(0, 3210, 3305)) // to (c2)
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Fatalf("ray blocked: expected 0, got %d", got)
	}
}

func TestHandleLineOfSight_ArgShape(t *testing.T) {
	// Pins the TS-faithful arg tuple passed to HasLineOfSight by handleLineOfSight
	// via the isLineOfSight wrapper. NAI-163-D-LOS-ARG-SHAPE-FIX.
	st := &stubLineValidatorArgs{losReturn: true}
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = st
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(0, 3210, 3305)) // to (c2)
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	_ = s.PopInt()
	if len(st.losCalls) != 1 {
		t.Fatalf("expected 1 LV call, got %d", len(st.losCalls))
	}
	got := st.losCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("handleLineOfSight arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

// --- NAI-165: isLineOfWalk wrapper + handleLineOfWalk arg-shape regression ---
// NAI-165-D-LOW-ARG-SHAPE-FIX: widens isLineOfWalk from (1, 0, 0, 0) to
// (1, 1, 1, 0) to match TS GameMap.ts:425-427. Symmetric mirror of
// NAI-163-D-LOS-ARG-SHAPE-FIX.

func TestIsLineOfWalkWrapper_PassesTSFaithfulArgShape(t *testing.T) {
	// Regression pin: TS GameMap.ts:426 calls
	//   rsmod.hasLineOfWalk(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0)
	// goscape's srcSize expands to srcWidth=srcLength=1 inside RayCast
	// (linevalidator.go:21), so the TS-faithful arg tuple at the wrapper
	// level is srcSize=1, destWidth=1, destLength=1, extraFlag=0.
	// Pre-NAI-165-D-LOW-ARG-SHAPE-FIX the wrapper was (1, 0, 0, 0).
	st := &stubLineValidatorArgs{lowReturn: true}
	s := &ScriptState{LineValidator: st}
	_ = isLineOfWalk(s, 0, 3200, 3300, 3210, 3305)
	if len(st.lowCalls) != 1 {
		t.Fatalf("expected 1 LineValidator call, got %d", len(st.lowCalls))
	}
	got := st.lowCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("isLineOfWalk arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestHandleLineOfWalk_ArgShape(t *testing.T) {
	// Pins the TS-faithful arg tuple passed to HasLineOfWalk by handleLineOfWalk
	// at the opcode 1006 dispatch site (direct call, NOT via the wrapper —
	// see handlers_map.go:423). NAI-165-D-LOW-ARG-SHAPE-FIX.
	st := &stubLineValidatorArgs{lowReturn: true}
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = st
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(0, 3210, 3305)) // to (c2)
	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk: %v", err)
	}
	_ = s.PopInt()
	if len(st.lowCalls) != 1 {
		t.Fatalf("expected 1 LV call, got %d", len(st.lowCalls))
	}
	got := st.lowCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("handleLineOfWalk arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

// MAP_MULTIWAY (opcode 1014) — NAI-120 Bundle 2A.
// Mirrors TS ServerOps.ts:376-380.

// multiWorld extends mockWorld with a coord→bool map for IsMulti so MAP_MULTIWAY
// tests can pin per-tile multi-zone results. NAI-120 Bundle 2A.
type multiWorld struct {
	*mockWorld
	multiTiles map[[3]int]bool // key: [level, x, z]
}

func (m *multiWorld) IsMulti(level, x, z int) bool {
	return m.multiTiles[[3]int{level, x, z}]
}

func TestMapMultiway_MultiTile(t *testing.T) {
	w := &multiWorld{mockWorld: newMockWorld(), multiTiles: map[[3]int]bool{
		{0, 3222, 3218}: true,
	}}
	s := &ScriptState{
		World:       w,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Push packed coord (level<<28) | (x<<14) | z = 3222 << 14 | 3218.
	s.PushInt((0 << 28) | (3222 << 14) | 3218)
	if err := handleMapMultiway(s); err != nil {
		t.Fatalf("MAP_MULTIWAY multi tile: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("MAP_MULTIWAY multi tile: got %d, want 1", got)
	}
}

func TestMapMultiway_NonMultiTile(t *testing.T) {
	w := &multiWorld{mockWorld: newMockWorld(), multiTiles: nil}
	s := &ScriptState{
		World:       w,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt((0 << 28) | (3000 << 14) | 3000)
	if err := handleMapMultiway(s); err != nil {
		t.Fatalf("MAP_MULTIWAY non-multi: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("MAP_MULTIWAY non-multi: got %d, want 0", got)
	}
}

func TestMapMultiway_NoWorld(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(0)
	if err := handleMapMultiway(s); err == nil {
		t.Error("MAP_MULTIWAY with nil World: want error")
	}
}
