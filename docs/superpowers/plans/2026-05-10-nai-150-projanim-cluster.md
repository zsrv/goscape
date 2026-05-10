# NAI-150 PROJANIM cluster — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the 3-handler PROJANIM cluster (PROJANIM_MAP/NPC/PL) from TS Engine-TS `ServerOps.ts:171-210`, closing the user-reported "no handler for PROJANIM_NPC" smoke WARN.

**Architecture:** Add 2 methods to `script.WorldVars` (`MapProjAnim` + `LookupNpcBySlot`); production impl delegates to existing `Server.MapProjAnim` (`modules/world/world_zone.go:164`) and `Server.npcs[]` array. Three handlers in a new `pkg/script/handlers_projanim.go` use existing helpers (`checkCoord`, `checkSpotAnimType`) and existing `WorldVars.LookupPlayerByUID`. Dispatch entries in `pkg/script/handlers.go`.

**Tech Stack:** Go 1.26+. No new dependencies. Files: `pkg/script/state.go`, `pkg/script/handlers.go`, `pkg/script/handlers_projanim.go` (new), `pkg/script/handlers_projanim_test.go` (new), `pkg/script/handlers_vars_test.go`, `modules/world/server_varp.go`, `modules/world/server_varp_test.go` (new).

**Spec:** `docs/superpowers/specs/2026-05-10-nai-150-projanim-cluster-design.md` (commit `0756c04`).

**TS source canonical path:** `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/ServerOps.ts:171-210`.

**Cascade-tail:** 39 → 36 unhandled at HEAD `000d974`.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/state.go` | Modify | Add `MapProjAnim` + `LookupNpcBySlot` to `WorldVars` interface |
| `pkg/script/handlers_vars_test.go` | Modify | Add no-op stubs on `mockWorld` so existing tests still compile |
| `modules/world/server_varp.go` | Modify | Add `MapProjAnim` (Server.MapProjAnim passthrough) + `LookupNpcBySlot` (s.npcs[slot] lookup) on `worldVarsView` + conformance assertion |
| `pkg/script/handlers_projanim.go` | Create | 3 handlers: `handleProjAnimMap`, `handleProjAnimNpc`, `handleProjAnimPl` |
| `pkg/script/handlers.go` | Modify | 3 dispatch entries: `OpProjAnimMap`, `OpProjAnimNpc`, `OpProjAnimPl` |
| `pkg/script/handlers_projanim_test.go` | Create | 12 unit tests + recording mock `projAnimWorld` |
| `modules/world/server_varp_test.go` | Create | 2 production-wiring tests |

`spotAnimMapWorld` (handlers_map_test.go:416) embeds `mockWorld` — its inheritance picks up the new `mockWorld` stubs; no edit needed.

---

## Task 1: Foundation — extend WorldVars + stub-fix consumers

**Goal:** Add the two new method signatures to `script.WorldVars`. Add stub no-op impls to `mockWorld` (test infra) and `worldVarsView` (production) so the project still compiles. Production impls remain stubs at this stage — Task 5 fills them in via TDD.

**Files:**
- Modify: `pkg/script/state.go` (add 2 method signatures to `WorldVars` interface, near existing `AnimMap` and `LookupPlayerByUID`)
- Modify: `pkg/script/handlers_vars_test.go` (add 2 stub methods on `mockWorld`)
- Modify: `modules/world/server_varp.go` (add 2 stub methods on `worldVarsView` + conformance assertion)

- [ ] **Step 1.1: Add `MapProjAnim` to WorldVars interface**

In `pkg/script/state.go`, find the existing `AnimMap` method declaration (around line 86) inside the `WorldVars` interface. Append the new method immediately after `AnimMap`'s declaration:

```go
	// MapProjAnim broadcasts a projectile event from (level, srcX, srcZ)
	// to (dstX, dstZ). target encodes the receiver: 0 = none (MAP→MAP),
	// npc.nid+1 = NPC target, -player.slot-1 = player target.
	// srcHeight/dstHeight are pre-scaled by the handler (×4).
	// Mirrors TS World.mapProjAnim. Used by PROJANIM_MAP, PROJANIM_NPC,
	// PROJANIM_PL. NAI-150.
	MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim,
		srcHeight, dstHeight, startDelay, endDelay, peak, arc int)
```

- [ ] **Step 1.2: Add `LookupNpcBySlot` to WorldVars interface**

In the same file, find the existing `LookupPlayerByUID` method declaration (around line 126) inside the `WorldVars` interface. Append the new method immediately after `LookupPlayerByUID`'s declaration:

```go
	// LookupNpcBySlot resolves the NPC slot to its live ActiveNpc, or
	// nil if the slot is out of range / unoccupied. Slot-only — does
	// NOT verify the high-16 type bits, unlike NpcLookup.FindNpcByUID.
	// Mirrors TS World.getNpc(slot). Used by PROJANIM_NPC. NAI-150.
	LookupNpcBySlot(slot int) ActiveNpc
```

- [ ] **Step 1.3: Run build to confirm compile breaks across the project**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: FAIL on `mockWorld` (in `pkg/script`) and `worldVarsView` (in `modules/world`) — both must implement the new interface methods.

- [ ] **Step 1.4: Add no-op stubs on `mockWorld`**

In `pkg/script/handlers_vars_test.go`, locate the existing `func (m *mockWorld) AnimMap(...)` stub (line 51) and the existing `func (m *mockWorld) LookupPlayerByUID(...)` impl (line 61). Insert the two new stub methods directly after `AnimMap`'s line 51:

```go
// NAI-150: default no-op stub for PROJANIM_* test fixture. Real recording
// is layered on by handler-specific test types (projAnimWorld in
// handlers_projanim_test.go).
func (m *mockWorld) MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim, srcHeight, dstHeight, startDelay, endDelay, peak, arc int) {
}

// NAI-150: default no-op stub for PROJANIM_NPC test fixture. Returns
// nil (slot empty). Tests exercising the lookup override via
// projAnimWorld.
func (m *mockWorld) LookupNpcBySlot(slot int) ActiveNpc { return nil }
```

- [ ] **Step 1.5: Add stub impls on `worldVarsView`**

In `modules/world/server_varp.go`, append the following at the end of the file:

```go
// MapProjAnim implements script.WorldVars.MapProjAnim. Stub at
// NAI-150 T1; real delegation lands in T5.
func (w worldVarsView) MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim, srcHeight, dstHeight, startDelay, endDelay, peak, arc int) {
}

// LookupNpcBySlot implements script.WorldVars.LookupNpcBySlot. Stub
// at NAI-150 T1; real lookup lands in T5.
func (w worldVarsView) LookupNpcBySlot(slot int) script.ActiveNpc { return nil }

// Compile-time conformance assertion for script.WorldVars. Adding any
// new WorldVars method that worldVarsView fails to implement breaks
// the build here. NAI-150 T1.
var _ script.WorldVars = worldVarsView{}
```

- [ ] **Step 1.6: Run build to confirm compile is clean**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean build, no output.

- [ ] **Step 1.7: Run pkg/script + modules/world tests to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...`

Expected: all existing tests still PASS. No new tests yet.

- [ ] **Step 1.8: Commit**

```bash
git add pkg/script/state.go pkg/script/handlers_vars_test.go modules/world/server_varp.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-150 T1 — extend WorldVars with MapProjAnim + LookupNpcBySlot

Adds two method signatures to script.WorldVars; stub impls on mockWorld
and worldVarsView keep build/tests green. Real impls land in T5.
Foundation for the 3-handler PROJANIM cluster (T3).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Handler tests — RED

**Goal:** Write all 12 handler tests + recording mock in a new file. All should fail because `OpProjAnim*` opcodes have no dispatch entry yet (so `Execute` errors with "no handler for ..."). Direct-call negative tests fail because `handleProjAnim*` symbols don't exist yet.

**Files:**
- Create: `pkg/script/handlers_projanim_test.go`

- [ ] **Step 2.1: Write the test file with recording mock + all 12 tests**

Create `pkg/script/handlers_projanim_test.go` with this exact content:

```go
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

// packCoord returns the 32-bit packed (level, x, z) layout used by
// CoordValid/checkCoord. Mirrors the existing handlers_map_test.go
// inline expression `(level << 28) | (x << 14) | z`.
func packCoord(level, x, z int) int {
	return (level << 28) | (x << 14) | z
}

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
```

- [ ] **Step 2.2: Run the new tests to confirm RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestProjAnim -v 2>&1 | head -60`

Expected: compile FAILS — `handleProjAnimMap`, `handleProjAnimNpc`, `handleProjAnimPl` are undefined symbols. (The test file directly references the handler functions for negative-path tests, so the failure is at build, not at test runtime.)

- [ ] **Step 2.3: Commit (RED)**

```bash
git add pkg/script/handlers_projanim_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(script): NAI-150 T2 — PROJANIM_*  handler tests RED

Adds 12 unit tests + projAnimWorld recording mock for the 3-handler
PROJANIM cluster. Pins TS validation orders (PROJANIM_MAP: spotanim
first; PROJANIM_NPC/PL: srcCoord first), height ×4 scaling, target
encodings (0/-slot-1/+nid+1), and PROJANIM_NPC slot-only lookup
(typeId mismatch does NOT block lookup, mirrors TS comment-out).

Compile fails — handler symbols don't exist yet. Greens in T3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Implement handlers + dispatch — GREEN

**Goal:** Create `pkg/script/handlers_projanim.go` with the 3 handler bodies and add 3 dispatch entries to `pkg/script/handlers.go`. Tests from T2 turn GREEN.

**Files:**
- Create: `pkg/script/handlers_projanim.go`
- Modify: `pkg/script/handlers.go` (3 dispatch entries near line 107 / `OpSpotAnimMap`)

- [ ] **Step 3.1: Write the handler file**

Create `pkg/script/handlers_projanim.go` with this exact content:

```go
package script

import "fmt"

// handleProjAnimMap (PROJANIM_MAP, opcode 1018) queues a tile→tile
// projectile event broadcast to all players in the source zone.
// Mirrors TS Engine-TS/src/engine/script/handlers/ServerOps.ts:202-210.
//
// TS validation order is spotanim → srcCoord → dstCoord (different
// from PROJANIM_NPC/PL which validate srcCoord first). Pinned by
// TestProjAnimMap_ValidationOrder.
func handleProjAnimMap(s *ScriptState) error {
	arc := s.PopInt()
	peak := s.PopInt()
	duration := s.PopInt()
	delay := s.PopInt()
	dstHeight := s.PopInt()
	srcHeight := s.PopInt()
	spotanim := s.PopInt()
	dstCoord := s.PopInt()
	srcCoord := s.PopInt()

	if err := checkSpotAnimType(s, spotanim, "PROJANIM_MAP"); err != nil {
		return err
	}
	srcLevel, srcX, srcZ, err := checkCoord(srcCoord, "PROJANIM_MAP")
	if err != nil {
		return err
	}
	_, dstX, dstZ, err := checkCoord(dstCoord, "PROJANIM_MAP")
	if err != nil {
		return err
	}

	s.World.MapProjAnim(srcLevel, srcX, srcZ, dstX, dstZ, 0,
		spotanim, srcHeight*4, dstHeight*4, delay, duration, peak, arc)
	return nil
}

// handleProjAnimNpc (PROJANIM_NPC, opcode 2546) queues a tile→NPC
// projectile event with the NPC encoded as receiver via npc.Nid()+1.
// Slot-only NPC lookup — does NOT verify the high-16 expectedType
// bits (mirrors TS comment-out at ServerOps.ts:192). Mirrors TS
// ServerOps.ts:185-200.
func handleProjAnimNpc(s *ScriptState) error {
	arc := s.PopInt()
	peak := s.PopInt()
	duration := s.PopInt()
	delay := s.PopInt()
	dstHeight := s.PopInt()
	srcHeight := s.PopInt()
	spotanim := s.PopInt()
	npcUid := s.PopInt()
	srcCoord := s.PopInt()

	srcLevel, srcX, srcZ, err := checkCoord(srcCoord, "PROJANIM_NPC")
	if err != nil {
		return err
	}
	if err := checkSpotAnimType(s, spotanim, "PROJANIM_NPC"); err != nil {
		return err
	}

	slot := npcUid & 0xffff
	npc := s.World.LookupNpcBySlot(slot)
	if npc == nil {
		return fmt.Errorf("PROJANIM_NPC: invalid npc uid: %d", npcUid)
	}

	s.World.MapProjAnim(srcLevel, srcX, srcZ, npc.NpcX(), npc.NpcZ(),
		npc.Nid()+1, spotanim, srcHeight*4, dstHeight*4,
		delay, duration, peak, arc)
	return nil
}

// handleProjAnimPl (PROJANIM_PL, opcode 2091) queues a tile→player
// projectile event with the player encoded as receiver via
// -player.Slot()-1. Mirrors TS ServerOps.ts:171-183.
func handleProjAnimPl(s *ScriptState) error {
	arc := s.PopInt()
	peak := s.PopInt()
	duration := s.PopInt()
	delay := s.PopInt()
	dstHeight := s.PopInt()
	srcHeight := s.PopInt()
	spotanim := s.PopInt()
	uid := s.PopInt()
	srcCoord := s.PopInt()

	srcLevel, srcX, srcZ, err := checkCoord(srcCoord, "PROJANIM_PL")
	if err != nil {
		return err
	}
	if err := checkSpotAnimType(s, spotanim, "PROJANIM_PL"); err != nil {
		return err
	}

	pl := s.World.LookupPlayerByUID(uid)
	if pl == nil {
		return fmt.Errorf("PROJANIM_PL: invalid player uid: %d", uid)
	}

	s.World.MapProjAnim(srcLevel, srcX, srcZ, pl.X(), pl.Z(),
		-pl.Slot()-1, spotanim, srcHeight*4, dstHeight*4,
		delay, duration, peak, arc)
	return nil
}
```

- [ ] **Step 3.2: Add dispatch entries to handlers.go**

In `pkg/script/handlers.go`, locate the existing `OpSpotAnimMap: handleSpotAnimMap,` line (currently around line 107). Insert the following block immediately after the existing `// NAI-36-T5: tile-anchored spotanim broadcast.` block (i.e. immediately after the `OpSpotAnimMap` entry and its blank line):

```go
	// NAI-150: server projectile ops — tile→tile / tile→player /
	// tile→npc projectile broadcast. Mirrors TS ServerOps.ts:171-210.
	OpProjAnimMap: handleProjAnimMap,
	OpProjAnimNpc: handleProjAnimNpc,
	OpProjAnimPl:  handleProjAnimPl,

```

- [ ] **Step 3.3: Run the PROJANIM tests to confirm GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestProjAnim -v`

Expected: 12 tests PASS:
- `TestProjAnimMap_HappyPath`
- `TestProjAnimMap_InvalidSrcCoord`
- `TestProjAnimMap_InvalidDstCoord`
- `TestProjAnimMap_UnregisteredSpotanim`
- `TestProjAnimMap_ValidationOrder`
- `TestProjAnimNpc_HappyPath`
- `TestProjAnimNpc_NilNpc`
- `TestProjAnimNpc_ValidationOrder`
- `TestProjAnimPl_HappyPath`
- `TestProjAnimPl_NilPlayer`
- `TestProjAnimPl_TargetEncodingPinSlotZero`
- `TestProjAnimPl_HeightScaling`

- [ ] **Step 3.4: Run full pkg/script test suite for regression check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`

Expected: all PASS.

- [ ] **Step 3.5: Commit**

```bash
git add pkg/script/handlers_projanim.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-150 T3 — port PROJANIM_MAP/NPC/PL (opcodes 1018/2546/2091)

Three handlers in pkg/script/handlers_projanim.go mirror TS Engine-TS
ServerOps.ts:171-210. PROJANIM_MAP validates spotanim before coords;
PROJANIM_NPC/PL validate srcCoord first. PROJANIM_NPC uses slot-only
lookup (TS comment-out of expectedType check). Target encoding:
0 (MAP) / npc.Nid()+1 (NPC) / -player.Slot()-1 (PL).

Dispatch entries in pkg/script/handlers.go under "NAI-150" comment.
12 unit tests from T2 GREEN.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Production-wiring tests — RED

**Goal:** Write tests pinning that `worldVarsView.MapProjAnim` delegates to `Server.MapProjAnim` and `worldVarsView.LookupNpcBySlot` looks up `s.npcs[slot]`. Tests fail because T1 stubs are no-ops.

**Files:**
- Create: `modules/world/server_varp_test.go`

- [ ] **Step 4.1: Write the test file**

Create `modules/world/server_varp_test.go` with this exact content:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/zone"
)

// TestWorldVarsView_MapProjAnim_Delegates pins that the WorldVars
// MapProjAnim method routes through Server.MapProjAnim →
// Zone.MapProjAnim, producing an enclosed ZoneOpMapProjAnim event in
// the source-coord zone. NAI-150 T4.
func TestWorldVarsView_MapProjAnim_Delegates(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	// Args mirror pkg/zone TestMapProjAnimEnclosed (zone_test.go:307)
	// 13-arg signature on top of (level=0).
	w.MapProjAnim(0, 3, 4, 5, 7, 0, 100, 10, 0, 0, 50, 40, 30)

	if len(s.zonesTracking) != 1 {
		t.Fatalf("zonesTracking: got %d, want 1", len(s.zonesTracking))
	}
	var z *zone.Zone
	for k := range s.zonesTracking {
		z = k
	}
	events := z.Events()
	if len(events) == 0 {
		t.Fatalf("zone events: got 0, want 1 (MapProjAnim)")
	}
	e := events[0]
	if e.Type != zone.ZoneEventEnclosed {
		t.Errorf("event type: got %v, want Enclosed", e.Type)
	}
	if e.Bytes[0] != rsbuf.ZoneOpMapProjAnim {
		t.Errorf("opcode: got %d, want ZoneOpMapProjAnim=%d", e.Bytes[0], rsbuf.ZoneOpMapProjAnim)
	}
}

// TestWorldVarsView_LookupNpcBySlot table-pins slot resolution against
// Server.npcs. NAI-150 T4.
func TestWorldVarsView_LookupNpcBySlot(t *testing.T) {
	s := newZoneTestServer(t)
	w := worldVarsView{s: s}

	// Empty server: any slot returns nil.
	if got := w.LookupNpcBySlot(0); got != nil {
		t.Errorf("empty server slot 0: got %v, want nil", got)
	}
	if got := w.LookupNpcBySlot(100); got != nil {
		t.Errorf("empty server slot 100: got %v, want nil", got)
	}

	// OOB negative.
	if got := w.LookupNpcBySlot(-1); got != nil {
		t.Errorf("OOB slot -1: got %v, want nil", got)
	}

	// OOB positive (Server.npcs is fixed-size [8192]*Npc — server.go:93).
	if got := w.LookupNpcBySlot(8192); got != nil {
		t.Errorf("OOB slot 8192: got %v, want nil", got)
	}
	if got := w.LookupNpcBySlot(99999); got != nil {
		t.Errorf("OOB slot 99999: got %v, want nil", got)
	}

	// Populated slot returns the registered NPC.
	n := newTestNpc(7)
	s.npcs[7] = n
	got := w.LookupNpcBySlot(7)
	if got == nil {
		t.Fatalf("populated slot 7: got nil, want non-nil")
	}
	if got.Nid() != 7 {
		t.Errorf("populated slot 7: got Nid=%d, want 7", got.Nid())
	}

	// Adjacent unpopulated slot still returns nil.
	if got := w.LookupNpcBySlot(8); got != nil {
		t.Errorf("unpopulated slot 8 adjacent to populated 7: got %v, want nil", got)
	}

	// Nil-server defensive (sanity).
	wn := worldVarsView{}
	if got := wn.LookupNpcBySlot(7); got != nil {
		t.Errorf("nil-server: got %v, want nil", got)
	}
}
```

- [ ] **Step 4.2: Run the new wiring tests to confirm RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestWorldVarsView_MapProjAnim_Delegates|TestWorldVarsView_LookupNpcBySlot" -v`

Expected: both FAIL.
- `TestWorldVarsView_MapProjAnim_Delegates`: FAIL with `zonesTracking: got 0, want 1` (T1 stub is a no-op).
- `TestWorldVarsView_LookupNpcBySlot`: FAIL on the populated-slot assertion `populated slot 7: got nil, want non-nil` (T1 stub returns nil).

- [ ] **Step 4.3: Commit (RED)**

```bash
git add modules/world/server_varp_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-150 T4 — worldVarsView.MapProjAnim/LookupNpcBySlot RED

Pins production wiring: MapProjAnim must delegate through
Server.MapProjAnim → Zone.MapProjAnim producing ZoneOpMapProjAnim
event; LookupNpcBySlot must read s.npcs[slot] with full bounds
+ nil-slot defensive coverage. Greens in T5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Production-wiring real impl — GREEN

**Goal:** Replace the T1 stubs in `worldVarsView` with real delegation. T4 tests turn GREEN.

**Files:**
- Modify: `modules/world/server_varp.go`

- [ ] **Step 5.1: Replace the stub bodies**

In `modules/world/server_varp.go`, find the two stub methods added in T1 (`MapProjAnim` and `LookupNpcBySlot` at the end of the file). Replace their stub bodies with real impls. The final block (preserving the conformance assertion) should look like this:

```go
// MapProjAnim implements script.WorldVars.MapProjAnim. Delegates to
// Server.MapProjAnim (modules/world/world_zone.go:164) which routes
// the event by source-coord zone and tracks the zone for end-of-tick
// flush. NAI-150.
func (w worldVarsView) MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim, srcHeight, dstHeight, startDelay, endDelay, peak, arc int) {
	if w.s == nil {
		return
	}
	w.s.MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim,
		srcHeight, dstHeight, startDelay, endDelay, peak, arc)
}

// LookupNpcBySlot implements script.WorldVars.LookupNpcBySlot.
// Returns s.npcs[slot] cast to script.ActiveNpc, or nil for OOB slot
// or empty slot. Slot-only — does NOT verify the high-16 type bits,
// unlike NpcLookup.FindNpcByUID. Mirrors TS World.getNpc(slot).
// NAI-150.
func (w worldVarsView) LookupNpcBySlot(slot int) script.ActiveNpc {
	if w.s == nil {
		return nil
	}
	if slot < 0 || slot >= len(w.s.npcs) {
		return nil
	}
	n := w.s.npcs[slot]
	if n == nil {
		return nil
	}
	return n
}

// Compile-time conformance assertion for script.WorldVars. Adding any
// new WorldVars method that worldVarsView fails to implement breaks
// the build here. NAI-150 T1.
var _ script.WorldVars = worldVarsView{}
```

- [ ] **Step 5.2: Run wiring tests to confirm GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestWorldVarsView_MapProjAnim_Delegates|TestWorldVarsView_LookupNpcBySlot" -v`

Expected: both PASS.

- [ ] **Step 5.3: Run full modules/world test suite for regression check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: all PASS.

- [ ] **Step 5.4: Commit**

```bash
git add modules/world/server_varp.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-150 T5 — wire worldVarsView.MapProjAnim/LookupNpcBySlot

Replaces T1 stubs with real impls. MapProjAnim passes through to
Server.MapProjAnim (world_zone.go:164). LookupNpcBySlot reads
Server.npcs[slot] with bounds + nil-slot defensives. T4 tests GREEN.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Repo verification

**Goal:** Confirm the full repo is clean: vet passes, all tests pass, dispatch count grew by 3.

- [ ] **Step 6.1: go vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean, no output.

- [ ] **Step 6.2: full repo test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all PASS.

- [ ] **Step 6.3: confirm dispatch growth**

Run: `grep -cE '^\tOp[A-Z][a-zA-Z]+:\s+handle' pkg/script/handlers.go`

Expected: count of dispatch entries equals (count at HEAD `000d974`) + 3. The exact pre-count depends on HEAD, but the delta must be +3 vs `git show 000d974:pkg/script/handlers.go | grep -cE '^\tOp[A-Z][a-zA-Z]+:\s+handle'`.

- [ ] **Step 6.4: confirm 3 PROJANIM symbols are dispatched**

Run: `grep -E 'OpProjAnim(Map|Npc|Pl):' pkg/script/handlers.go`

Expected: exactly 3 lines:

```
	OpProjAnimMap: handleProjAnimMap,
	OpProjAnimNpc: handleProjAnimNpc,
	OpProjAnimPl:  handleProjAnimPl,
```

(no commit; verification only)

---

## Task 7: End-of-impl reviewer subagent

**Goal:** Dispatch one Sonnet-model `superpowers:code-reviewer` subagent to audit the full sub-spec. Per `superpowers_code_reviewer_model.md` the reviewer must NOT be on Opus. Per `audit_subagent_fabrication.md`, the controller spot-checks any code-citing claim before acting on it.

- [ ] **Step 7.1: Dispatch the reviewer subagent on Sonnet**

Use the `Agent` tool with:
- `subagent_type`: `feature-dev:code-reviewer` (NOT the `superpowers:code-reviewer` agent — see footnote on agent availability; use whichever code-reviewer agent the harness offers)
- `model`: `sonnet`
- `description`: "NAI-150 PROJANIM cluster review"
- `prompt`: Self-contained brief that includes:
  - Spec path: `docs/superpowers/specs/2026-05-10-nai-150-projanim-cluster-design.md`
  - Plan path: `docs/superpowers/plans/2026-05-10-nai-150-projanim-cluster.md`
  - TS source: `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/ServerOps.ts:171-210`
  - Range to review: T1 commit through T5 commit (the 5 NAI-150 commits since `000d974`).
  - Specific audit targets:
    - PROJANIM_MAP validation order (spotanim → src → dst, NOT src first)
    - PROJANIM_NPC slot-only lookup (typeID NOT verified — pin TS comment-out at ServerOps.ts:192)
    - Target encodings: `0 / npc.Nid()+1 / -pl.Slot()-1`
    - Height ×4 scaling on both src and dst
    - Pop order matches push order across all 9 ints per handler
    - TS-fidelity of error messages
    - WorldVars interface placement (comments + neighbors)
    - Dispatch grouping under NAI-150 comment in handlers.go
    - Production-wiring delegation in worldVarsView
  - Format: report Critical / Important / Nit findings. Cite file:line for every claim.

- [ ] **Step 7.2: Read the review and triage**

If Critical or Important findings — fix in a NEW commit (NOT --amend), then re-run T6 verification, then re-dispatch reviewer if the changes were substantive (otherwise close as-is). If Nit-only — proceed to close.

- [ ] **Step 7.3: (Conditional) Apply fixes**

Per `verify_implementer_claims.md`: before changing code based on a reviewer claim, grep / Read the cited file:line to verify the claim against HEAD. If the reviewer fabricated, document the false-positive in the close-commit body and skip the change.

```bash
# example, if a real fix is needed:
git add <files>
git commit --no-gpg-sign -m "fix(script): NAI-150 — <reviewer-finding-summary>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Close commit + memory update

**Goal:** Single `chore(close)` commit per `close_commit_memory_trailer.md` that records the sub-spec close, retires the NAI-150 follow-up entry, and adds a new "From NAI-150" close section.

- [ ] **Step 8.1: Update nai_followups.md memory**

Edit `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`:

1. Find the existing `### NAI-150 candidate — PROJANIM_NPC + PROJANIM_MAP + PROJANIM_PL cluster` section under "From NAI-149". Mark it `RETIRED 2026-05-10 by NAI-150` (preserve the rationale block for provenance).

2. Add a new section at the top (just under the file header, before the existing "From NAI-149"), formatted as:

```markdown
## From NAI-150 (2026-05-10) — PROJANIM cluster port

NAI-150 spec at `docs/superpowers/specs/2026-05-10-nai-150-projanim-cluster-design.md` (commit `0756c04`); plan at `docs/superpowers/plans/2026-05-10-nai-150-projanim-cluster.md` (commit `<plan-commit-sha>`). No deferred items.

---

## NAI-150 — CLOSED 2026-05-10

**Scope:** Port PROJANIM_MAP (opcode 1018), PROJANIM_NPC (opcode 2546), PROJANIM_PL (opcode 2091) from TS Engine-TS ServerOps.ts:171-210. Adds 2 methods to script.WorldVars (MapProjAnim, LookupNpcBySlot); production impl in worldVarsView delegates to Server.MapProjAnim and Server.npcs.

**Cadence:** Mid-band (~260 LOC) per runescript_cadence — separate spec + plan, subagent-driven TDD (T1 foundation → T2 RED tests → T3 GREEN handlers → T4 RED wiring → T5 GREEN wiring → T6 verify → T7 reviewer).

**Spec:** `docs/superpowers/specs/2026-05-10-nai-150-projanim-cluster-design.md` (commit `0756c04`).
**Plan:** `docs/superpowers/plans/2026-05-10-nai-150-projanim-cluster.md` (commit `<plan-commit-sha>`).

**Commits (chronological):** T1 `<sha>`, T2 `<sha>`, T3 `<sha>`, T4 `<sha>`, T5 `<sha>`. Close: this commit.

**Cascade:** Closes user-reported "no handler for PROJANIM_NPC (opcode 2546)" WARN from 2026-05-09/10 smoke logs. Cascade-tail: 39 → 36 unhandled (post-NAI-149 baseline).

**Deviations opened:** None. PROJANIM_NPC `_expectedType` skip and PROJANIM_MAP validation-order quirk are TS-correct, not deviations — both pinned by tests and labeled in handler doc-comments.

**Reviewer (Sonnet) verdict:** <fill in from T7 result>.

**Smoke handoff:** User runs the goscape server + Java client smoke; PROJANIM_NPC "no handler" WARN should disappear. If the smoke surfaces an adjacent unhandled-opcode issue ≤30 LOC (smoke_surfaces_adjacent_divergences), route into NAI-151 in-scope-stretch; else open NAI-151 separately.
```

3. Replace the placeholder `<plan-commit-sha>` and `<sha>` markers with the actual SHAs from `git log --oneline | head -10`.

- [ ] **Step 8.2: Save the close-commit memory entry**

The plan-controller may also want to save a short memory entry for any cascade insight that surfaced. If nothing notable, skip — `nai_followups.md` is sufficient.

- [ ] **Step 8.3: Final close commit**

```bash
git add /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-150 — PROJANIM cluster port (3 ops)

Closes 1 of 4 user-reported "no handler" WARN classes from the
2026-05-09/10 smoke logs (PROJANIM_NPC, opcode 2546). Bundle also
retires PROJANIM_MAP (1018) and PROJANIM_PL (2091) from the
unhandled-opcode tail (39 → 36).

Adds 2 methods to script.WorldVars (MapProjAnim, LookupNpcBySlot);
worldVarsView delegates to existing Server.MapProjAnim and
Server.npcs. 3 handlers in pkg/script/handlers_projanim.go mirror
TS Engine-TS ServerOps.ts:171-210 with validation-order asymmetry
preserved (PROJANIM_MAP: spotanim first; PROJANIM_NPC/PL: srcCoord
first) and PROJANIM_NPC slot-only lookup matching TS comment-out
of expectedType.

Closes memory: nai_followups.md NAI-149-FOLLOWUP-NAI-150-PROJANIM.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Checklist (run before dispatching T1)

- [x] **Spec coverage:**
  - Spec §5.1 WorldVars additions → T1 (steps 1.1-1.2).
  - Spec §5.2 worldVarsView impls → T1 stubs (1.5) + T5 real (5.1).
  - Spec §5.3 mock consumers → T1 (1.4) covers `mockWorld`; `spotAnimMapWorld` inheritance noted in File Structure table.
  - Spec §5.4 new file `handlers_projanim.go` → T3 (3.1).
  - Spec §5.5 dispatch entries → T3 (3.2).
  - Spec §6 handler bodies → T3 (3.1) verbatim.
  - Spec §7.1 12 unit tests → T2 (2.1) verbatim.
  - Spec §7.2 2 production-wiring tests → T4 (4.1) verbatim.
  - Spec §7.3 conformance assertion → T1 (1.5).
  - Spec §8 cadence (subagent-driven TDD + Sonnet reviewer) → T7 (7.1).
  - Spec §10 cascade attribution → T8 close commit (8.3).
  - Spec §11 R1/R2 risks → covered by T4 (4.1) wiring tests; R3 cascade-attribution covered by T8 smoke handoff text.
- [x] **Placeholder scan:** No "TBD" / "fill in" / "similar to" patterns in code blocks. Reviewer verdict is intentionally `<fill in from T7 result>` — that is filled at close-commit time, not plan-write time.
- [x] **Type consistency:** Method names `MapProjAnim` / `LookupNpcBySlot` consistent across spec, T1, T2, T3, T4, T5. Mock names `projAnimWorld`, `mapProjAnimCalls`, `npcsBySlot`, `playersByUID` consistent T2 → T3 references. Handler names `handleProjAnimMap` / `handleProjAnimNpc` / `handleProjAnimPl` consistent T2 (test calls) → T3 (impl).
