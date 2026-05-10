package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// projAnimCall captures all 13 args of a MapProjAnim invocation for
// assertion. Field names mirror the WorldVars.MapProjAnim signature.
type projAnimCall struct {
	level, srcX, srcZ, dstX, dstZ, target, spotanim                  int
	srcHeight, dstHeight, startDelay, endDelay, peak, arc            int
}

// projAnimWorld is the recording mock for PROJANIM_* handler tests. It
// embeds mockWorld so it inherits all default WorldVars stubs and only
// overrides the surfaces this handler family touches:
// MapProjAnim (capture), LookupNpcBySlot (driven by npcsBySlot map),
// LookupPlayerByUID (driven by mockWorld.playersByUID).
type projAnimWorld struct {
	mockWorld
	mapProjAnimCalls []projAnimCall
	npcsBySlot       map[int]ActiveNpc
}

func (w *projAnimWorld) MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim, srcHeight, dstHeight, startDelay, endDelay, peak, arc int) {
	w.mapProjAnimCalls = append(w.mapProjAnimCalls, projAnimCall{
		level:      level,
		srcX:       srcX,
		srcZ:       srcZ,
		dstX:       dstX,
		dstZ:       dstZ,
		target:     target,
		spotanim:   spotanim,
		srcHeight:  srcHeight,
		dstHeight:  dstHeight,
		startDelay: startDelay,
		endDelay:   endDelay,
		peak:       peak,
		arc:        arc,
	})
}

func (w *projAnimWorld) LookupNpcBySlot(slot int) ActiveNpc {
	if w.npcsBySlot == nil {
		return nil
	}
	return w.npcsBySlot[slot]
}

// packCoord is defined in handlers_player_test.go (same package).

// --- PROJANIM_MAP ----------------------------------------------------

func TestProjAnimMap_HappyPath(t *testing.T) {
	w := &projAnimWorld{}
	m := &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}

	const spotanim = 200
	const srcLevel, srcX, srcZ = 0, 3200, 3300
	const dstX, dstZ = 3210, 3310
	const srcHeight, dstHeight = 5, 7
	const delay, duration, peak, arc = 10, 20, 30, 40

	srcCoord := packCoord(srcLevel, srcX, srcZ)
	dstCoord := packCoord(srcLevel, dstX, dstZ)

	// Push order (deepest first → top last): srcCoord, dstCoord, spotanim,
	// srcHeight, dstHeight, delay, duration, peak, arc. runMapOp Execute()
	// dispatches the opcode; tests RED until T3 wires dispatch+handler.
	state := runMapOp(t, w, m, OpProjAnimMap, []int{
		srcCoord, dstCoord, spotanim,
		srcHeight, dstHeight, delay, duration, peak, arc,
	})
	_ = state

	if len(w.mapProjAnimCalls) != 1 {
		t.Fatalf("mapProjAnimCalls: got %d, want 1", len(w.mapProjAnimCalls))
	}
	got := w.mapProjAnimCalls[0]
	want := projAnimCall{
		level: srcLevel, srcX: srcX, srcZ: srcZ,
		dstX: dstX, dstZ: dstZ,
		target: 0, spotanim: spotanim,
		srcHeight:  srcHeight * 4,
		dstHeight:  dstHeight * 4,
		startDelay: delay,
		endDelay:   duration,
		peak:       peak,
		arc:        arc,
	}
	if got != want {
		t.Errorf("mapProjAnimCalls[0]:\n got %+v\nwant %+v", got, want)
	}
}

func TestProjAnimMap_InvalidSrcCoord(t *testing.T) {
	w := &projAnimWorld{}
	state := &ScriptState{
		World:       w,
		Configs:     &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// PROJANIM_MAP validation order is spotanim → srcCoord → dstCoord
	// (TS ServerOps.ts:205-207). To reach the srcCoord branch, push a
	// VALID spotanim and an invalid srcCoord.
	for _, v := range []int{
		-1,                          // srcCoord (invalid: negative)
		packCoord(0, 3210, 3310),    // dstCoord (valid; never validated due to early-fail)
		200,                         // spotanim (valid: registered)
		5, 7, 10, 20, 30, 40,        // srcHeight, dstHeight, delay, duration, peak, arc
	} {
		state.PushInt(v)
	}

	err := handleProjAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "PROJANIM_MAP") || !strings.Contains(err.Error(), "coord") {
		t.Errorf("invalid srcCoord: got %v, want PROJANIM_MAP coord error", err)
	}
	if len(w.mapProjAnimCalls) != 0 {
		t.Errorf("mapProjAnimCalls on error path: got %d, want 0", len(w.mapProjAnimCalls))
	}
}

func TestProjAnimMap_InvalidDstCoord(t *testing.T) {
	w := &projAnimWorld{}
	state := &ScriptState{
		World:       w,
		Configs:     &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	for _, v := range []int{
		packCoord(0, 3200, 3300),    // srcCoord (valid)
		-1,                          // dstCoord (invalid)
		200,                         // spotanim
		5, 7, 10, 20, 30, 40,
	} {
		state.PushInt(v)
	}

	err := handleProjAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "PROJANIM_MAP") || !strings.Contains(err.Error(), "coord") {
		t.Errorf("invalid dstCoord: got %v, want PROJANIM_MAP coord error", err)
	}
	if len(w.mapProjAnimCalls) != 0 {
		t.Errorf("mapProjAnimCalls on error path: got %d, want 0", len(w.mapProjAnimCalls))
	}
}

func TestProjAnimMap_UnregisteredSpotanim(t *testing.T) {
	w := &projAnimWorld{}
	state := &ScriptState{
		World:       w,
		Configs:     &mockConfigs{}, // empty: any id is unregistered
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	for _, v := range []int{
		packCoord(0, 3200, 3300), // srcCoord
		packCoord(0, 3210, 3310), // dstCoord
		7,                        // spotanim (unregistered)
		5, 7, 10, 20, 30, 40,
	} {
		state.PushInt(v)
	}

	err := handleProjAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "PROJANIM_MAP") {
		t.Errorf("unregistered spotanim: got %v, want PROJANIM_MAP error", err)
	}
	if len(w.mapProjAnimCalls) != 0 {
		t.Errorf("mapProjAnimCalls on error path: got %d, want 0", len(w.mapProjAnimCalls))
	}
}

// Pins TS validation order: spotanim is checked BEFORE srcCoord
// (ServerOps.ts:205-207). With both invalid, the error must mention
// the spotanim path (the first check), not the coord path.
func TestProjAnimMap_ValidationOrder(t *testing.T) {
	w := &projAnimWorld{}
	state := &ScriptState{
		World:       w,
		Configs:     &mockConfigs{}, // empty: any id is unregistered
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	for _, v := range []int{
		-1, // srcCoord (invalid)
		-1, // dstCoord (invalid)
		7,  // spotanim (unregistered)
		5, 7, 10, 20, 30, 40,
	} {
		state.PushInt(v)
	}

	err := handleProjAnimMap(state)
	if err == nil || !strings.Contains(err.Error(), "PROJANIM_MAP") {
		t.Fatalf("validation order: got %v, want PROJANIM_MAP error", err)
	}
	// Spotanim error message comes from checkSpotAnimType
	// (handlers_map.go:213). Coord error mentions "coord". Validation-
	// order pin: with spotanim checked first per TS, the message must
	// NOT contain "coord".
	if strings.Contains(err.Error(), "coord") {
		t.Errorf("validation order: error mentions \"coord\" — TS PROJANIM_MAP checks spotanim first; got %q", err.Error())
	}
}

// --- PROJANIM_NPC ----------------------------------------------------

func TestProjAnimNpc_HappyPath(t *testing.T) {
	const slot = 7
	const npcType = 99
	npcUid := (npcType << 16) | slot

	npc := &mockNpc{typeID: 42, x: 300, z: 400, level: 0, nid: slot}
	// Note typeID=42 NOT 99 — pin TS comment-out of expectedType check:
	// lookup returns the slot's npc even with type mismatch.
	w := &projAnimWorld{
		npcsBySlot: map[int]ActiveNpc{slot: npc},
	}
	m := &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}

	const spotanim = 200
	const srcLevel, srcX, srcZ = 0, 3200, 3300
	const srcHeight, dstHeight = 5, 7
	const delay, duration, peak, arc = 10, 20, 30, 40

	srcCoord := packCoord(srcLevel, srcX, srcZ)

	state := runMapOp(t, w, m, OpProjAnimNpc, []int{
		srcCoord, npcUid, spotanim,
		srcHeight, dstHeight, delay, duration, peak, arc,
	})
	_ = state

	if len(w.mapProjAnimCalls) != 1 {
		t.Fatalf("mapProjAnimCalls: got %d, want 1", len(w.mapProjAnimCalls))
	}
	got := w.mapProjAnimCalls[0]
	want := projAnimCall{
		level: srcLevel, srcX: srcX, srcZ: srcZ,
		dstX: 300, dstZ: 400, // from lookup-resolved npc, NOT popped src
		target: slot + 1, // nid+1 encoding
		spotanim:   spotanim,
		srcHeight:  srcHeight * 4,
		dstHeight:  dstHeight * 4,
		startDelay: delay,
		endDelay:   duration,
		peak:       peak,
		arc:        arc,
	}
	if got != want {
		t.Errorf("mapProjAnimCalls[0]:\n got %+v\nwant %+v", got, want)
	}
}

func TestProjAnimNpc_NilNpc(t *testing.T) {
	const slot = 7
	npcUid := (99 << 16) | slot
	w := &projAnimWorld{
		npcsBySlot: map[int]ActiveNpc{}, // slot empty
	}
	state := &ScriptState{
		World:       w,
		Configs:     &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	for _, v := range []int{
		packCoord(0, 3200, 3300), // srcCoord
		npcUid,                   // npcUid (slot empty)
		200,                      // spotanim
		5, 7, 10, 20, 30, 40,
	} {
		state.PushInt(v)
	}

	err := handleProjAnimNpc(state)
	if err == nil || !strings.Contains(err.Error(), "PROJANIM_NPC") || !strings.Contains(err.Error(), "invalid npc uid") {
		t.Errorf("nil npc: got %v, want PROJANIM_NPC invalid-uid error", err)
	}
	if len(w.mapProjAnimCalls) != 0 {
		t.Errorf("mapProjAnimCalls on error path: got %d, want 0", len(w.mapProjAnimCalls))
	}
}

// Pins TS validation order: srcCoord is checked BEFORE spotanim
// (ServerOps.ts:188-189). With both invalid, the error must mention
// "coord", not the spotanim error.
func TestProjAnimNpc_ValidationOrder(t *testing.T) {
	w := &projAnimWorld{}
	state := &ScriptState{
		World:       w,
		Configs:     &mockConfigs{}, // empty: any id is unregistered
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	for _, v := range []int{
		-1,           // srcCoord (invalid)
		(99 << 16),   // npcUid (slot=0; would also fail lookup but never reached)
		7,            // spotanim (unregistered)
		5, 7, 10, 20, 30, 40,
	} {
		state.PushInt(v)
	}

	err := handleProjAnimNpc(state)
	if err == nil || !strings.Contains(err.Error(), "PROJANIM_NPC") {
		t.Fatalf("validation order: got %v, want PROJANIM_NPC error", err)
	}
	if !strings.Contains(err.Error(), "coord") {
		t.Errorf("validation order: error does not mention \"coord\" — TS PROJANIM_NPC checks srcCoord first; got %q", err.Error())
	}
}

// --- PROJANIM_PL -----------------------------------------------------

func TestProjAnimPl_HappyPath(t *testing.T) {
	const uid = 12345
	const slot = 4
	pl := &mockPlayer{slot: slot, x: 500, z: 600}
	w := &projAnimWorld{
		mockWorld: mockWorld{
			playersByUID: map[int]ActivePlayer{uid: pl},
		},
	}
	m := &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}

	const spotanim = 200
	const srcLevel, srcX, srcZ = 0, 3200, 3300
	const srcHeight, dstHeight = 5, 7
	const delay, duration, peak, arc = 10, 20, 30, 40

	srcCoord := packCoord(srcLevel, srcX, srcZ)

	state := runMapOp(t, w, m, OpProjAnimPl, []int{
		srcCoord, uid, spotanim,
		srcHeight, dstHeight, delay, duration, peak, arc,
	})
	_ = state

	if len(w.mapProjAnimCalls) != 1 {
		t.Fatalf("mapProjAnimCalls: got %d, want 1", len(w.mapProjAnimCalls))
	}
	got := w.mapProjAnimCalls[0]
	want := projAnimCall{
		level: srcLevel, srcX: srcX, srcZ: srcZ,
		dstX: 500, dstZ: 600, // from lookup-resolved player
		target: -slot - 1, // -4-1 = -5
		spotanim:   spotanim,
		srcHeight:  srcHeight * 4,
		dstHeight:  dstHeight * 4,
		startDelay: delay,
		endDelay:   duration,
		peak:       peak,
		arc:        arc,
	}
	if got != want {
		t.Errorf("mapProjAnimCalls[0]:\n got %+v\nwant %+v", got, want)
	}
}

func TestProjAnimPl_NilPlayer(t *testing.T) {
	const uid = 12345
	w := &projAnimWorld{} // no playersByUID seeded
	state := &ScriptState{
		World:       w,
		Configs:     &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	for _, v := range []int{
		packCoord(0, 3200, 3300), // srcCoord
		uid,                      // uid (not registered)
		200,                      // spotanim
		5, 7, 10, 20, 30, 40,
	} {
		state.PushInt(v)
	}

	err := handleProjAnimPl(state)
	if err == nil || !strings.Contains(err.Error(), "PROJANIM_PL") || !strings.Contains(err.Error(), "invalid player uid") {
		t.Errorf("nil player: got %v, want PROJANIM_PL invalid-uid error", err)
	}
	if len(w.mapProjAnimCalls) != 0 {
		t.Errorf("mapProjAnimCalls on error path: got %d, want 0", len(w.mapProjAnimCalls))
	}
}

// Pins -player.Slot()-1 off-by-one with the smallest valid slot value.
func TestProjAnimPl_TargetEncodingPinSlotZero(t *testing.T) {
	const uid = 1
	pl := &mockPlayer{slot: 0, x: 100, z: 200}
	w := &projAnimWorld{
		mockWorld: mockWorld{
			playersByUID: map[int]ActivePlayer{uid: pl},
		},
	}
	m := &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}

	state := runMapOp(t, w, m, OpProjAnimPl, []int{
		packCoord(0, 3200, 3300),
		uid, 200,
		0, 0, 0, 0, 0, 0,
	})
	_ = state

	if len(w.mapProjAnimCalls) != 1 {
		t.Fatalf("mapProjAnimCalls: got %d, want 1", len(w.mapProjAnimCalls))
	}
	if got := w.mapProjAnimCalls[0].target; got != -1 {
		t.Errorf("target encoding: got %d, want -1 (slot=0 → -slot-1 = -1)", got)
	}
}

// Pins srcHeight*4 / dstHeight*4 scaling. Independent of other coverage
// to keep the regression signal narrow if the multiplier ever changes.
func TestProjAnimPl_HeightScaling(t *testing.T) {
	const uid = 1
	pl := &mockPlayer{slot: 0, x: 100, z: 200}
	w := &projAnimWorld{
		mockWorld: mockWorld{
			playersByUID: map[int]ActivePlayer{uid: pl},
		},
	}
	m := &mockConfigs{spotAnimTypes: map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}

	const srcH, dstH = 2, 3
	state := runMapOp(t, w, m, OpProjAnimPl, []int{
		packCoord(0, 3200, 3300),
		uid, 200,
		srcH, dstH, 0, 0, 0, 0,
	})
	_ = state

	if len(w.mapProjAnimCalls) != 1 {
		t.Fatalf("mapProjAnimCalls: got %d, want 1", len(w.mapProjAnimCalls))
	}
	got := w.mapProjAnimCalls[0]
	if got.srcHeight != srcH*4 || got.dstHeight != dstH*4 {
		t.Errorf("height scaling: got src=%d dst=%d, want src=%d dst=%d (×4)", got.srcHeight, got.dstHeight, srcH*4, dstH*4)
	}
}
