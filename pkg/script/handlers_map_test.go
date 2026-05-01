package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
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
