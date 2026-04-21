# OPLOC Routing Implementation Plan (S6j)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire OPLOC1..5 client click opcodes to fire `[oploc1..5,<locType>]` script triggers on the next tick, mirroring the OPNPC routing shape established in S6b.

**Architecture:** Two-phase decomposition. Phase A (Task 2) is the synchronous handler (`handleOpLoc1..5` in `handler_oploc.go`) that decodes the wire payload, runs four validation gates, and mutates player interaction state via `SetInteraction`. Phase B (Task 3) is the tick-deferred trigger fire — `tryFireOpTrigger` in `interaction_trigger.go` extends its existing `*Npc` switch with a parallel `*Loc` case that performs the lifecycle gate and runs the script. Task 1 is pure additive plumbing that both phases depend on (Loc entity-interface methods + `Server.GetLoc` + `ScriptState.ActiveLoc` field).

**Tech Stack:** Go 1.23+, standard library only. Tests use the existing `pkg/io/packet`, `modules/world` test fixtures (`makeOpNpcFixture` is the canonical pattern), and `pkg/objtype` config types.

**Spec reference:** `docs/superpowers/specs/2026-04-21-runescript-s6j-oploc-routing-design.md` (commit `8f96ea5`).

**Build commands (per CLAUDE.md):**
- Build: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- Test all: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
- Test one: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestName -v`
- Vet: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

**Commit policy (per CLAUDE.md):** All commits use `git commit --no-gpg-sign`.

---

## File Structure

| File | Created/Modified | Responsibility | Task |
|---|---|---|---|
| `pkg/entity/loc.go` | Modify | Add `Slot()` and `Coords()` for `entity` interface conformance | 1 |
| `pkg/entity/loc_test.go` | Create | Entity interface tests for Loc | 1 |
| `pkg/script/state.go` | Modify | Add `ActiveLoc` field on `ScriptState` | 1 |
| `modules/world/loc_lookup.go` | Create | `Server.GetLoc(level, x, z, locId)` helper + tests | 1 |
| `modules/world/loc_lookup_test.go` | Create | Tests for `Server.GetLoc` | 1 |
| `modules/world/interaction_trigger_test.go` | Modify | Add compile-time `var _ entity = (*entity.Loc)(nil)` assertion + 6 fire tests | 1, 3 |
| `modules/world/handler_oploc.go` | Create | `handleOpLoc(p, payload, op int)` + 5 thin wrappers `handleOpLoc1..5` | 2 |
| `modules/world/handler_oploc_test.go` | Create | 8 handler validation tests | 2 |
| `modules/world/handlers_game.go` | Modify | Wire 5 client opcode → handler entries | 2 |
| `modules/world/player.go` | Modify | Refactor `targetSubject` field shape | 2 |
| `modules/world/interaction_trigger.go` | Modify | Append `*Loc` switch case to `tryFireOpTrigger` + `locStillValid` helper | 3 |

**Existing infrastructure already in place (no changes needed):**
- `script.PtrActiveLoc` constant — `pkg/script/pointer.go:12`
- `script.ActiveLoc` interface (empty stub) — `pkg/script/active.go:303`
- `script.TriggerOpLoc1..5` (66..70) — `pkg/script/trigger.go:71-75`
- `script.ServerTriggerType` and `script.GetByTrigger` (3-tier fallback) — `pkg/script/provider.go`
- Client opcodes `OPLOC1..5` (245, 172, 96, 97, 116; all 6 bytes) — `pkg/io/protocol/game/client/prot.go:68-72`
- `Player.SetInteraction` already sets `apRange = 10`, `apRangeCalled = false` — `modules/world/interaction.go:24-33`
- `Player.ClearPendingAction` exists — `modules/world/player_script.go:438`
- `Server.zoneMap *zone.ZoneMap` and `zoneMap.Get(level, worldX, worldZ) *Zone` — `modules/world/server.go:90`, `pkg/zone`
- `Server.locTypes *objtype.LocTypeConfigs` with `.Get(id) *LocType` and `LocType.Category` — `modules/world/server.go:73`
- `Zone.Locs []*entity.Loc` — `pkg/zone/zone.go:20`

---

## Task 1: Loc Entity Interface + ScriptState.ActiveLoc + Server.GetLoc

**Goal:** Pure additive plumbing. After this task, `Loc` satisfies `modules/world.entity`, `ScriptState` has an `ActiveLoc` field, and `Server.GetLoc` is callable. No behavior change to existing flows.

**Files:**
- Modify: `pkg/entity/loc.go`
- Create: `pkg/entity/loc_test.go`
- Modify: `pkg/script/state.go`
- Create: `modules/world/loc_lookup.go`
- Create: `modules/world/loc_lookup_test.go`
- Modify: `modules/world/interaction_trigger_test.go` (add compile-time assertion only — fire tests come in Task 3)

### Step-by-step

- [ ] **Step 1.1: Write failing test for `Loc.Slot()` returning -1**

Create `pkg/entity/loc_test.go` with:

```go
package entity

import "testing"

func TestLocSlotReturnsMinusOne(t *testing.T) {
	l := NewLoc(0, 100, 100, 1, 1, LifecycleForever, 0, 10, 0)
	if got := l.Slot(); got != -1 {
		t.Errorf("Loc.Slot(): got %d, want -1", got)
	}
}
```

- [ ] **Step 1.2: Run test to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/... -run TestLocSlotReturnsMinusOne -v`

Expected: FAIL with `l.Slot undefined (type *Loc has no field or method Slot)`.

- [ ] **Step 1.3: Add `Slot()` method to Loc**

Append to `pkg/entity/loc.go` (after the existing `Angle()` method at line 34):

```go
// Slot returns -1 because locs are not slot-indexed (unlike Players and Npcs
// which live in server-wide slot registries). Required for the world.entity
// interface so locs can be assigned to Player.target.
func (l *Loc) Slot() int { return -1 }
```

- [ ] **Step 1.4: Run test to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/... -run TestLocSlotReturnsMinusOne -v`

Expected: PASS.

- [ ] **Step 1.5: Write failing test for `Loc.Coords()`**

Append to `pkg/entity/loc_test.go`:

```go
func TestLocCoordsReturnsXZLevel(t *testing.T) {
	l := NewLoc(2, 3245, 3198, 1, 1, LifecycleForever, 0, 10, 0)
	x, z, level := l.Coords()
	if x != 3245 || z != 3198 || level != 2 {
		t.Errorf("Loc.Coords(): got (%d, %d, %d), want (3245, 3198, 2)", x, z, level)
	}
}
```

- [ ] **Step 1.6: Run test to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/... -run TestLocCoordsReturnsXZLevel -v`

Expected: FAIL with `l.Coords undefined`.

- [ ] **Step 1.7: Add `Coords()` method to Loc**

Append to `pkg/entity/loc.go`:

```go
// Coords returns the loc's tile position. Required for the world.entity
// interface. Reads X/Z/Level from the embedded entity.Entity (see
// entity.go:6-12 for the field layout); no allocation.
func (l *Loc) Coords() (x, z, level int) {
	return l.X, l.Z, l.Level
}
```

- [ ] **Step 1.8: Run test to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/entity/... -run TestLocCoordsReturnsXZLevel -v`

Expected: PASS.

- [ ] **Step 1.9: Add compile-time `entity` interface assertion**

The `entity` interface in `modules/world/movement_consts.go:45-48` is unexported (lowercase `e`). To assert that `*entity.Loc` satisfies it, the assertion must live inside `modules/world`. Open `modules/world/interaction_trigger_test.go` and append (at the bottom of the file):

```go
// Compile-time assertion that *entity.Loc satisfies the package-local
// entity interface (Slot() int + Coords() (x, z, level int)). Required
// for p.target = loc to type-check when handler_oploc sets the target.
var _ entity = (*entity.Loc)(nil)
```

If the test file does not already import `github.com/zsrv/goscape/pkg/entity`, add it.

- [ ] **Step 1.10: Run build to verify the assertion compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: builds successfully (no output on success).

- [ ] **Step 1.11: Add `ActiveLoc` field to `ScriptState`**

In `pkg/script/state.go`, the `ScriptState` struct currently has `ActiveNpc ActiveNpc` at line 99. Add an `ActiveLoc` field directly below it. Find the existing block:

```go
	// ActiveNpc is the NPC that NPC_* and VARN ops target. Nil if no
	// NPC is bound to this script's execution. Set by callers (test
	// fixtures, OPNPC trigger routing in a future sub-spec).
	ActiveNpc ActiveNpc
```

Replace it with:

```go
	// ActiveNpc is the NPC that NPC_* and VARN ops target. Nil if no
	// NPC is bound to this script's execution. Set by callers (test
	// fixtures, OPNPC trigger routing).
	ActiveNpc ActiveNpc

	// ActiveLoc is the Loc that LOC_* ops target. Nil if no Loc is
	// bound to this script's execution. Set by callers (test fixtures,
	// OPLOC trigger routing). Type is the package-local ActiveLoc
	// interface (currently empty — handlers_loc.go will populate
	// methods in a follow-up sub-spec).
	ActiveLoc ActiveLoc
```

- [ ] **Step 1.12: Run build to confirm ScriptState compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`

Expected: builds successfully.

- [ ] **Step 1.13: Write failing test for `Server.GetLoc`**

Create `modules/world/loc_lookup_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
)

// TestServerGetLocReturnsLocWhenPresent places one loc in a zone and
// confirms GetLoc returns it for matching (level, x, z, locId).
func TestServerGetLocReturnsLocWhenPresent(t *testing.T) {
	s := newTestServer(t)
	loc := entity.NewLoc(0, 3200, 3200, 1, 1, entity.LifecycleForever, 42, 10, 0)
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Locs = append(z.Locs, loc)

	got := s.GetLoc(0, 3200, 3200, 42)
	if got != loc {
		t.Errorf("GetLoc: got %v, want %v", got, loc)
	}
}

// TestServerGetLocReturnsNilWhenAbsent confirms GetLoc returns nil
// when the requested locId is not in the target zone.
func TestServerGetLocReturnsNilWhenAbsent(t *testing.T) {
	s := newTestServer(t)
	if got := s.GetLoc(0, 3200, 3200, 42); got != nil {
		t.Errorf("GetLoc: got %v, want nil", got)
	}
}

// TestServerGetLocFiltersByTypeID confirms GetLoc only matches the
// requested locId, not any loc at the coords.
func TestServerGetLocFiltersByTypeID(t *testing.T) {
	s := newTestServer(t)
	otherLoc := entity.NewLoc(0, 3200, 3200, 1, 1, entity.LifecycleForever, 99, 10, 0)
	z := s.zoneMap.Get(0, 3200, 3200)
	z.Locs = append(z.Locs, otherLoc)

	if got := s.GetLoc(0, 3200, 3200, 42); got != nil {
		t.Errorf("GetLoc: got %v, want nil (typeID 42 absent, only 99 present)", got)
	}
}
```

- [ ] **Step 1.14: Run tests to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestServerGetLoc -v`

Expected: compile failure — `s.GetLoc undefined`.

- [ ] **Step 1.15: Implement `Server.GetLoc`**

Create `modules/world/loc_lookup.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/entity"
)

// GetLoc returns the loc at (level, x, z) whose type matches locId, or
// nil if no such loc exists in the corresponding zone. Mirrors TS
// World.getLoc(x, z, level, locId) used by OpLocHandler validation.
//
// Iteration is O(zone-loc-count); zones typically hold a handful of
// locs, so a linear scan is fine. If profiling later shows hot zones,
// a coord-keyed map can replace the slice.
func (s *Server) GetLoc(level, x, z, locId int) *entity.Loc {
	zn := s.zoneMap.Get(level, x, z)
	if zn == nil {
		return nil
	}
	for _, l := range zn.Locs {
		if l == nil {
			continue
		}
		if l.Level == level && l.X == x && l.Z == z && l.Type() == locId {
			return l
		}
	}
	return nil
}
```

- [ ] **Step 1.16: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestServerGetLoc -v`

Expected: all 3 tests PASS.

- [ ] **Step 1.17: Run full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all existing tests still pass.

- [ ] **Step 1.18: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 1.19: Commit Task 1**

```bash
git add pkg/entity/loc.go pkg/entity/loc_test.go pkg/script/state.go modules/world/loc_lookup.go modules/world/loc_lookup_test.go modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Loc entity-interface + Server.GetLoc + ScriptState.ActiveLoc plumbing (S6j-1)

Adds the pure-additive plumbing OPLOC routing depends on:
- Loc.Slot() returns -1 (not slot-indexed) and Loc.Coords() returns
  (X, Z, Level) so *entity.Loc satisfies the world.entity interface
  for Player.target assignment.
- ScriptState.ActiveLoc field mirrors ActiveNpc; uses the existing
  pkg/script.ActiveLoc interface stub (handlers_loc.go expansion is
  a follow-up sub-spec).
- Server.GetLoc(level, x, z, locId) helper iterates zoneMap.Get(...)
  .Locs filtered by typeID. O(zone-loc-count); fine for now.

3 entity tests + 3 GetLoc tests. Compile-time assertion ensures Loc
keeps satisfying the entity interface even after future field changes.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6j-oploc-routing-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6j-oploc-routing.md (Task 1)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: handle_oploc.go + targetSubject Refactor + Opcode Wiring

**Goal:** The synchronous click handler. After this task, OPLOC1..5 client clicks decode → validate → mutate `Player.target`/`targetOp`/`targetSubject`, with no script firing yet (that's Task 3).

**Files:**
- Create: `modules/world/handler_oploc.go`
- Create: `modules/world/handler_oploc_test.go`
- Modify: `modules/world/handlers_game.go` (5 new entries)
- Modify: `modules/world/player.go` (refactor `targetSubject` field)

### Step-by-step

- [ ] **Step 2.1: Refactor `Player.targetSubject` field shape**

In `modules/world/player.go` line 86, replace:

```go
	targetSubject    struct{ typ, com int }
```

with:

```go
	// targetSubject snapshots the initial type/coords of the interaction
	// target at click time. tryFireOpTrigger reads (typ, x, z, level) to
	// detect mid-tick mutation/despawn (a tree changing into a stump
	// between click and tick processing). The previous (typ, com) shape
	// was unused; expanding it is safe because no code reads .com.
	targetSubject struct{ typ, x, z, level int }
```

- [ ] **Step 2.2: Run build to confirm refactor compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: builds successfully (no readers of `.com` exist; verified with `grep -rn "targetSubject\." modules/world pkg/`).

- [ ] **Step 2.3: Write failing test for valid OPLOC click sets interaction state**

Create `modules/world/handler_oploc_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/grid"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// makeOpLocFixture creates a server + player + loc adjacent to the player,
// with a LocType registered, ready for handleOpLoc tests.
// Player at (99, 100, 0); loc at (100, 100, 0) — Chebyshev=1 (adjacent).
// Player originX/originZ = (100, 100) so viewport gate accepts coords
// within [-104, +104] of (100, 100).
func makeOpLocFixture(t *testing.T) (*Server, *Player, *entity.Loc) {
	t.Helper()
	s := newTestServer(t)
	s.grid = grid.New()

	locType := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "test_loc"},
		Category:   7,
	}
	s.locTypes.Set(42, locType) // see locTypes.Set helper note below

	loc := entity.NewLoc(0, 100, 100, 1, 1, entity.LifecycleForever, 42, 10, 0)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Locs = append(zn.Locs, loc)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0
	p.originX, p.originZ = 100, 100

	return s, p, loc
}

// p2x3Payload encodes (x: u16, z: u16, locId: u16) into 6 bytes big-endian.
func p2x3Payload(x, z, locId int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(locId >> 8), byte(locId),
	}
}

// TestHandleOpLoc1SetsInteraction verifies a valid request sets interaction state.
func TestHandleOpLoc1SetsInteraction(t *testing.T) {
	_, p, loc := makeOpLocFixture(t)

	if err := handleOpLoc1(p, p2x3Payload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
	if p.targetOp != 1 {
		t.Errorf("targetOp: got %d, want 1", p.targetOp)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
	if p.targetSubject.typ != 42 {
		t.Errorf("targetSubject.typ: got %d, want 42", p.targetSubject.typ)
	}
	if p.targetSubject.x != 100 || p.targetSubject.z != 100 || p.targetSubject.level != 0 {
		t.Errorf("targetSubject coords: got (%d, %d, %d), want (100, 100, 0)",
			p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)
	}
}
```

> **Note for implementer:** `s.locTypes.Set(...)` may not be the exact API for registering a LocType in tests. Check `pkg/objtype/loctype.go` for the test-registration helper (likely `LocTypeConfigs.Set`, `Register`, or direct map access). If absent, add a small test-only helper or assign via direct field if exposed. The test's intent is "make `s.locTypes.Get(42)` return `locType`."

- [ ] **Step 2.4: Run test to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLoc1SetsInteraction -v`

Expected: compile failure — `handleOpLoc1 undefined`.

- [ ] **Step 2.5: Implement `handleOpLoc` and 5 wrappers**

Create `modules/world/handler_oploc.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// handleOpLoc is the shared implementation for OPLOC1..OPLOC5.
// op is 1..5. Payload = 6 bytes: (x: G2, z: G2, locId: G2).
//
// Validation gates (mirrors TS OpLocHandler.ts:14-42):
//  1. p.delayed → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside player's build-area viewport (104 tiles each axis
//     from p.originX/originZ) → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//
// DEVIATION (S6j-D1): TS gate 6 — `locType.op[op-1] != null && != "hidden"`
// (OpLocHandler.ts:38-42) — is skipped here because LocType.Op []string is
// not yet a field on LocType. Effective behavior: trigger registration
// absence becomes the gate (no trigger → silent no-op on next tick instead
// of TS's UnsetMapFlag at click time). Follow-up: "LocType.Op + loc_op
// script opcode" sub-spec.
//
// On success: ClearPendingAction → SetInteraction(Engine, loc, op) →
// snapshot loc identity into p.targetSubject for tryFireOpTrigger's
// lifecycle gate.
func handleOpLoc(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 6 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	locId := int(r.G2())

	// Viewport gate. Build area is ~13 zones × 8 tiles per side from
	// origin; 104 tiles is the half-extent. Mirrors TS OpLocHandler.ts:20-28
	// scene-bounds rejection.
	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 104 || dz > 104 {
		sendUnsetMapFlag(p)
		return nil
	}

	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.locTypes.Get(locId) == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, loc, op)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return nil
}

func handleOpLoc1(p *Player, payload []byte) error { return handleOpLoc(p, payload, 1) }
func handleOpLoc2(p *Player, payload []byte) error { return handleOpLoc(p, payload, 2) }
func handleOpLoc3(p *Player, payload []byte) error { return handleOpLoc(p, payload, 3) }
func handleOpLoc4(p *Player, payload []byte) error { return handleOpLoc(p, payload, 4) }
func handleOpLoc5(p *Player, payload []byte) error { return handleOpLoc(p, payload, 5) }
```

- [ ] **Step 2.6: Run test to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLoc1SetsInteraction -v`

Expected: PASS. (If the LocType registration helper used in `makeOpLocFixture` doesn't exist, fix that first per the note in Step 2.3.)

- [ ] **Step 2.7: Add 7 more handler validation tests**

Append to `modules/world/handler_oploc_test.go`:

```go
// TestHandleOpLocDelayedPlayerRejected verifies delayed player gets UnsetMapFlag, no state change.
func TestHandleOpLocDelayedPlayerRejected(t *testing.T) {
	s, p, _ := makeOpLocFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	cc := drainConn(t, p.client.conn)
	_ = handleOpLoc1(p, p2x3Payload(100, 100, 42))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
}

// TestHandleOpLocShortPayloadRejected verifies < 6 byte payload emits UnsetMapFlag.
func TestHandleOpLocShortPayloadRejected(t *testing.T) {
	_, p, _ := makeOpLocFixture(t)

	cc := drainConn(t, p.client.conn)
	_ = handleOpLoc1(p, []byte{0x01, 0x02, 0x03}) // only 3 bytes
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpLocOutOfViewportRejected verifies coords > 104 tiles from origin emits UnsetMapFlag.
func TestHandleOpLocOutOfViewportRejected(t *testing.T) {
	_, p, _ := makeOpLocFixture(t) // origin = (100, 100)

	cc := drainConn(t, p.client.conn)
	_ = handleOpLoc1(p, p2x3Payload(250, 100, 42)) // dx = 150 > 104
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport click, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport click")
	}
}

// TestHandleOpLocCoordValidationBoundary verifies exactly 104-tile distance accepted, 105 rejected.
func TestHandleOpLocCoordValidationBoundary(t *testing.T) {
	s, p, _ := makeOpLocFixture(t) // origin = (100, 100)

	// Place a loc at (204, 100, 0), exactly 104 tiles from origin.
	boundaryLoc := entity.NewLoc(0, 204, 100, 1, 1, entity.LifecycleForever, 42, 10, 0)
	zn := s.zoneMap.Get(0, 204, 100)
	zn.Locs = append(zn.Locs, boundaryLoc)

	if err := handleOpLoc1(p, p2x3Payload(204, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1 at boundary: %v", err)
	}
	if p.target != boundaryLoc {
		t.Errorf("dx=104 should be accepted; target = %v, want boundaryLoc", p.target)
	}

	// Reset and try dx = 105 → reject.
	p.ClearInteraction()
	cc := drainConn(t, p.client.conn)
	_ = handleOpLoc1(p, p2x3Payload(205, 100, 42))
	p.client.flushWrite()
	got := <-cc
	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for dx=105")
	}
	if p.target != nil {
		t.Error("target should remain nil for dx=105")
	}
}

// TestHandleOpLocMissingLocRejected verifies Server.GetLoc returning nil emits UnsetMapFlag.
func TestHandleOpLocMissingLocRejected(t *testing.T) {
	_, p, _ := makeOpLocFixture(t)

	cc := drainConn(t, p.client.conn)
	_ = handleOpLoc1(p, p2x3Payload(100, 100, 999)) // wrong locId
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing loc, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing loc")
	}
}

// TestHandleOpLocMissingLocTypeRejected verifies LocType not registered emits UnsetMapFlag.
func TestHandleOpLocMissingLocTypeRejected(t *testing.T) {
	s, p, _ := makeOpLocFixture(t)

	// Place a second loc whose typeID has no LocType registered.
	missingTypeLoc := entity.NewLoc(0, 100, 100, 1, 1, entity.LifecycleForever, 77, 10, 0)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Locs = append(zn.Locs, missingTypeLoc)

	cc := drainConn(t, p.client.conn)
	_ = handleOpLoc1(p, p2x3Payload(100, 100, 77))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing locType, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing locType")
	}
}

// TestHandleOpLocAllFiveOpsRouteIndependently runs op 1..5 and confirms targetOp matches.
func TestHandleOpLocAllFiveOpsRouteIndependently(t *testing.T) {
	type opCase struct {
		op   int
		fn   func(*Player, []byte) error
		name string
	}
	cases := []opCase{
		{1, handleOpLoc1, "OpLoc1"},
		{2, handleOpLoc2, "OpLoc2"},
		{3, handleOpLoc3, "OpLoc3"},
		{4, handleOpLoc4, "OpLoc4"},
		{5, handleOpLoc5, "OpLoc5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, p, _ := makeOpLocFixture(t)
			if err := c.fn(p, p2x3Payload(100, 100, 42)); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if p.targetOp != c.op {
				t.Errorf("targetOp: got %d, want %d", p.targetOp, c.op)
			}
		})
	}
}

// TestHandleOpLocClearsExistingInteraction verifies any pre-existing interaction is cleared.
func TestHandleOpLocClearsExistingInteraction(t *testing.T) {
	s, p, loc := makeOpLocFixture(t)

	// Pre-set an interaction with a fake npc at slot 1.
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0, DebugName: "test"}}
	npc := NewNpc(1, 0, 100, 100, 0, typ)
	npc.nid = 1
	s.npcs[1] = npc
	p.SetInteraction(InteractionEngine, npc, 3)
	if p.target != npc {
		t.Fatal("setup: pre-existing target should be npc")
	}

	if err := handleOpLoc1(p, p2x3Payload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc (existing npc interaction should be replaced)", p.target)
	}
	if p.targetOp != 1 {
		t.Errorf("targetOp: got %d, want 1", p.targetOp)
	}
}
```

> **Note for implementer:** `drainConn` and `p.client.conn` reference the existing test helpers used in `handler_opnpc_test.go`. If `p.client.conn` is the wrong field name (e.g., `clientConn` is returned separately by `newTestPlayer`), follow the OPNPC test pattern: capture the second return value of `newTestPlayer(t)` as `cc`, then `drainConn(t, cc)`. Adjust `makeOpLocFixture` to return the connection if needed. The simplest fix: have `makeOpLocFixture` return `(*Server, *Player, *entity.Loc, net.Conn)` like OPNPC's fixture does.

- [ ] **Step 2.8: Run all 8 handler tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLoc -v`

Expected: 8 tests pass.

- [ ] **Step 2.9: Wire 5 new opcode entries in handlers_game.go**

In `modules/world/handlers_game.go`, after the OPNPC block (line 27-31), append:

```go
	gameHandlers[245] = handleOpLoc1 // OPLOC1
	gameHandlers[172] = handleOpLoc2 // OPLOC2
	gameHandlers[96] = handleOpLoc3  // OPLOC3
	gameHandlers[97] = handleOpLoc4  // OPLOC4
	gameHandlers[116] = handleOpLoc5 // OPLOC5
```

(Opcodes verified against `pkg/io/protocol/game/client/prot.go:68-72`.)

- [ ] **Step 2.10: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass (no regressions; new tests pass).

- [ ] **Step 2.11: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 2.12: Commit Task 2**

```bash
git add modules/world/handler_oploc.go modules/world/handler_oploc_test.go modules/world/handlers_game.go modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): handleOpLoc1..5 + 8 validation tests (S6j-2)

Phase A of OPLOC routing — synchronous click handler. Mirrors
handler_opnpc.go shape: shared handleOpLoc(p, payload, op) +
5 thin wrappers handleOpLoc1..5.

Four validation gates per TS OpLocHandler.ts:14-42:
  delayed → payload-length → viewport(104t) → loc-exists → locType-exists
DEVIATION S6j-D1: skips per-op gate (LocType.Op []string deferred).

Refactors Player.targetSubject from {typ, com int} to
{typ, x, z, level int} for tryFireOpTrigger's lifecycle gate
(Task 3). The .com field had zero readers; safe expansion.

Wires OPLOC1=245, OPLOC2=172, OPLOC3=96, OPLOC4=97, OPLOC5=116
in handlers_game.go.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6j-oploc-routing-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6j-oploc-routing.md (Task 2)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: tryFireOpTrigger Loc Branch + locStillValid

**Goal:** Phase B of OPLOC routing — tick-deferred trigger fire. After this task, `[oploc1..5,<locType>]` scripts execute end-to-end on player click.

**Files:**
- Modify: `modules/world/interaction_trigger.go` (append `*Loc` switch case + `locStillValid` helper)
- Modify: `modules/world/interaction_trigger_test.go` (append 6 fire tests)

### Step-by-step

- [ ] **Step 3.1: Write failing test for "no script registered → silent ClearInteraction"**

Append to `modules/world/interaction_trigger_test.go`:

```go
// makeOpLocTriggerFixture creates a fixture for tryFireOpTrigger Loc-branch
// tests: server + player anchored on a loc with valid targetSubject.
// Returns (server, player, loc).
func makeOpLocTriggerFixture(t *testing.T) (*Server, *Player, *entity.Loc) {
	t.Helper()
	s, p, loc := makeOpLocFixture(t)
	p.SetInteraction(InteractionEngine, loc, 1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return s, p, loc
}

// TestTryFireOpTriggerLocNoScript verifies a Loc target with no registered
// trigger silently clears the interaction.
func TestTryFireOpTriggerLocNoScript(t *testing.T) {
	_, p, _ := makeOpLocTriggerFixture(t)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after silent clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after no-script clear")
	}
}
```

- [ ] **Step 3.2: Run test to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTryFireOpTriggerLocNoScript -v`

Expected: FAIL — currently the default branch in `tryFireOpTrigger` (the `if !ok` for `*Npc`) sets `interactionFired=true` but does NOT call `ClearInteraction`. So `p.target` will still be non-nil after the call.

- [ ] **Step 3.3: Extend `tryFireOpTrigger` with `*Loc` switch case**

Open `modules/world/interaction_trigger.go`. The current shape has the `*Npc` type-assert at line 31 with a default at lines 32-35. Restructure to a multi-branch switch.

Replace the entire function (lines 28-81) with:

```go
// tryFireOpTrigger fires the [op<entity><op>,<typeID>] trigger for the
// player's anchored target when the player has just reached interaction
// range. Matches TS Player.tryInteract() for the Engine-kind interaction.
//
// Preconditions (guaranteed by caller — Player.processInteraction):
//   - p.interacted == true
//   - p.interactionKind == InteractionEngine
//   - p.target != nil
//   - p.interactionFired == false
//
// Branches by target concrete type. Common behaviour across branches:
//   - Player became delayed between reach and dispatch: defer; leave
//     interactionFired false so we retry next tick.
//   - Lifecycle gate fail (NPC dead / Loc despawn-or-mutated): silent
//     clear interaction.
//   - targetOp out of [1,5]: silent clear.
//   - No script found (type/category/global): silent clear.
//   - Script suspends (P_DELAY/P_PAUSEBUTTON/P_COUNTDIALOG): keep
//     interaction anchored; resumeOrFinish already stored the state.
//   - Script finishes / aborts: clear interaction.
//
// (OPOBJ branch will extend this switch in a later sub-spec.)
func tryFireOpTrigger(p *Player) {
	srv := p.client.server

	switch tgt := p.target.(type) {
	case *Npc:
		fireOpTriggerNpc(p, srv, tgt)
	case *entity.Loc:
		fireOpTriggerLoc(p, srv, tgt)
	default:
		// Target type not handled by any branch: skip; mark fired so we
		// don't retry every tick.
		p.interactionFired = true
	}
}

// fireOpTriggerNpc fires the [opnpc<op>,<npcType>] trigger. Extracted
// from the original tryFireOpTrigger body verbatim.
func fireOpTriggerNpc(p *Player, srv *Server, npc *Npc) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if npc.dead {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	op := p.targetOp
	if op < 1 || op > 5 {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger := script.TriggerOpNpc1 + script.ServerTriggerType(op-1)
	category := 0
	if npc.typ != nil {
		category = npc.typ.Category
	}

	sf := srv.scriptProvider.GetByTrigger(trigger, npc.typeId, category)
	if sf == nil {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}

// fireOpTriggerLoc fires the [oploc<op>,<locType>] trigger.
//
// Lifecycle gate: locStillValid checks BOTH zone membership (loc still
// in zoneMap.Get(level,x,z).Locs by pointer) AND type match (loc.Type()
// == targetSubject.typ). The combined check defends against:
//   - In-place Info mutation (tree → stump via Loc.Info bitfield change)
//   - Removal from zone (loc despawned, axed, etc.)
//
// DEVIATION S6j-D2: TS handler sets targetOp=APLOC1+(op-1) and engine
// fires APLOC at approach range, OPLOC at contact. We fire OPLOC
// directly (no APLOC fallback) — inherits S6b OPNPC convention.
// Follow-up: "approach-vs-operate range gating" sub-spec.
//
// DEVIATION S6j-D4: Loc has no cached typ pointer (only the packed
// Info bitfield); category lookup goes through srv.locTypes.Get(typeID),
// unlike *Npc which reads npc.typ.Category from a cached pointer.
func fireOpTriggerLoc(p *Player, srv *Server, loc *entity.Loc) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if !locStillValid(srv, loc, p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	op := p.targetOp
	if op < 1 || op > 5 {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger := script.TriggerOpLoc1 + script.ServerTriggerType(op-1)
	category := 0
	if lt := srv.locTypes.Get(loc.Type()); lt != nil {
		category = lt.Category
	}

	sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
	if sf == nil {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	state := script.Init(sf, p, false, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= script.PtrActiveLoc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}

// locStillValid checks whether the held *Loc pointer still represents
// the same loc the player clicked. Two checks combined — both required
// because each defends against a different mutation:
//   - Zone membership: catches loc removal (e.g., axed tree).
//   - Type match: catches in-place Info mutation (e.g., tree → stump
//     via the same *Loc pointer; Loc's docstring explicitly notes
//     "callers can mutate Info in place ... without re-allocating").
func locStillValid(srv *Server, loc *entity.Loc, wantType, wantX, wantZ, wantLevel int) bool {
	if loc.Type() != wantType {
		return false
	}
	zn := srv.zoneMap.Get(wantLevel, wantX, wantZ)
	if zn == nil {
		return false
	}
	for _, l := range zn.Locs {
		if l == loc {
			return true
		}
	}
	return false
}
```

Update the imports at the top of `interaction_trigger.go` to add `github.com/zsrv/goscape/pkg/entity` if not already present.

- [ ] **Step 3.4: Run the no-script test to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTryFireOpTriggerLocNoScript -v`

Expected: PASS.

- [ ] **Step 3.5: Run all existing interaction_trigger tests to confirm no Npc-branch regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTryFireOpTrigger -v`

Expected: all existing Npc-branch tests still pass (refactor extracted them into `fireOpTriggerNpc` verbatim).

- [ ] **Step 3.6: Add 5 more fire tests**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestTryFireOpTriggerLocScriptFires verifies a registered [oploc1,<typeID>]
// script fires, ActiveLoc is set, and ClearInteraction runs after Finished.
func TestTryFireOpTriggerLocScriptFires(t *testing.T) {
	s, p, loc := makeOpLocTriggerFixture(t)

	// Register a no-op script for [oploc1, locType=42].
	sf := newNoopScriptFile(t, script.TriggerOpLoc1, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after script fire")
	}
}

// TestTryFireOpTriggerLocDeferredOnDelay verifies a delayed player defers
// fire (no state change, interactionFired stays false).
func TestTryFireOpTriggerLocDeferredOnDelay(t *testing.T) {
	s, p, loc := makeOpLocTriggerFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	tryFireOpTrigger(p)

	if p.target != loc {
		t.Errorf("target: got %v, want loc (deferred)", p.target)
	}
	if p.interactionFired {
		t.Error("interactionFired: want false (deferred)")
	}
}

// TestTryFireOpTriggerLocTypeChanged verifies in-place type mutation
// (loc.Info changed via packLocInfo) clears interaction silently.
func TestTryFireOpTriggerLocTypeChanged(t *testing.T) {
	_, p, loc := makeOpLocTriggerFixture(t)

	// Mutate the loc's type in-place by overwriting Info. New type 99
	// differs from p.targetSubject.typ (42).
	loc.Info = (99 & 0x3FFF) | (10&0x1F)<<14 | (0&0x3)<<19

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (type changed)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after type-change clear")
	}
}

// TestTryFireOpTriggerLocRemoved verifies removing the loc from its zone
// (axed-tree case) clears interaction silently.
func TestTryFireOpTriggerLocRemoved(t *testing.T) {
	s, p, loc := makeOpLocTriggerFixture(t)

	// Remove the loc from its zone.
	zn := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	zn.Locs = nil

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (loc removed)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after removal clear")
	}
}

// TestTryFireOpTriggerLocOpOutOfRange verifies targetOp=0 silently clears.
func TestTryFireOpTriggerLocOpOutOfRange(t *testing.T) {
	_, p, _ := makeOpLocTriggerFixture(t)
	p.targetOp = 0 // invalid

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (invalid op)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after invalid-op clear")
	}
}
```

> **Note for implementer:** `newNoopScriptFile(t, trigger, typeID, categoryID)` is a test helper that creates a `*script.ScriptFile` with the given trigger and a single `RETURN` opcode. If a similar helper already exists in `pkg/script/` test utilities, use it. If not, look at how `interaction_trigger_test.go`'s existing Npc fire tests construct the test script (e.g., `TestTryFireOpTriggerScriptFires` for `*Npc`) and mirror that pattern.

- [ ] **Step 3.7: Run all 6 Loc fire tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTryFireOpTriggerLoc -v`

Expected: 6 tests pass.

- [ ] **Step 3.8: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass.

- [ ] **Step 3.9: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 3.10: Run race detector on world tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

Expected: no races.

- [ ] **Step 3.11: Commit Task 3**

```bash
git add modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): tryFireOpTrigger Loc branch + 6 fire tests (S6j-3)

Phase B of OPLOC routing — tick-deferred trigger fire. After this
commit, [oploc1..5,<locType>] scripts execute end-to-end on player
click.

Refactors tryFireOpTrigger from a single Npc-only body into a
type-switch dispatcher with per-branch helpers (fireOpTriggerNpc,
fireOpTriggerLoc). Npc branch is unchanged behaviorally.

New locStillValid helper guards against two lifecycle mutations:
  (1) zone-membership lookup catches loc removal (axed tree)
  (2) type match catches in-place Info bitfield mutation
      (tree → stump via the same *Loc pointer)

DEVIATION S6j-D2: fires OPLOC directly (no APLOC fallback);
inherits S6b OPNPC simplification. Follow-up: range-gating sub-spec.

DEVIATION S6j-D4: category lookup via srv.locTypes.Get(typeID) since
Loc has no cached typ pointer (only packed Info bitfield).

Spec: docs/superpowers/specs/2026-04-21-runescript-s6j-oploc-routing-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6j-oploc-routing.md (Task 3)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes (for plan-author use)

**1. Spec coverage:**
- §1 Goal — Tasks 1+2+3 collectively achieve "OPLOC click → trigger fire on next tick." ✅
- §2 Architecture — Tasks 2 (Phase A) + 3 (Phase B) match the two-phase decomposition. ✅
- §3 File map — every modified/created file appears in plan task headers. ✅
- §5 Validation gates — Task 2 Step 2.5 implements all 4 gates (5th is documented as deviation S6j-D1). ✅
- §6 Tick-deferred fire — Task 3 Step 3.3 implements with full code. ✅
- §6.2 locStillValid — Task 3 Step 3.3 includes the helper inline. ✅
- §6.3 default-branch comment update — encompassed by the larger refactor in Step 3.3 (the new switch comment supersedes the old). ✅
- §7 Loc entity interface — Task 1 Steps 1.1-1.10. ✅
- §8 ScriptState.ActiveLoc — Task 1 Step 1.11; verified `PtrActiveLoc` and `ActiveLoc` interface stub already exist. ✅
- §9 Test plan — 3 entity tests (Steps 1.1-1.8), 3 GetLoc tests (Steps 1.13-1.16), 8 handler tests (Steps 2.3-2.7), 6 fire tests (Steps 3.1-3.7). Total 20 (spec said ~18; close enough — extra came from splitting `TestServerGetLoc` into 3 cases vs the spec's 1). ✅
- §11 Deviations — referenced in commit messages and code comments throughout. ✅

**2. Type consistency:**
- `Loc.Type()` returns `int` consistently throughout (used in Steps 1.15, 2.5, 3.3). ✅
- `Loc.X/Z/Level` accessed as direct fields (not method calls) consistently (Steps 1.7, 1.15, 2.5, 3.3, 3.6). ✅
- `targetSubject` field shape `{typ, x, z, level int}` consistent across Steps 2.1, 2.3, 2.5, 3.1, 3.3. ✅
- `script.TriggerOpLoc1 + script.ServerTriggerType(op-1)` matches existing Npc branch arithmetic. ✅
- `srv.locTypes.Get(loc.Type())` returns `*objtype.LocType` with `.Category` field — confirmed by spec §3 and codebase grep. ✅

**3. Placeholder scan:** No "TBD" / "TODO" / "implement later" / "fill in details" / "add appropriate error handling" found. The two implementer-notes ("Note for implementer") in Steps 2.3 and 3.6 flag specific verifiable conditions (helper API exists or not), not vague placeholders.

**4. Scope:** Three tasks, each independently committable, each ships with build + tests green. End-to-end OPLOC click → trigger fire works after Task 3.
