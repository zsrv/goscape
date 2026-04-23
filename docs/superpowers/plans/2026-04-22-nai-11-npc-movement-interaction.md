# NAI-11 NPC Movement-Interaction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `Npc.processMovementInteraction` + full `PathingEntity.setInteraction` into Go, closing NAI-10's seven deferred `setInteraction` fields, unifying NpcMode constants on the TS-space enum, and wiring the full AP/OP trigger matrix across all four target categories (Player/Npc/Loc/Obj) × (AP, OP) × 1..5.

**Architecture:** Two new files (`modules/world/npc_interaction.go`, `modules/world/npc_interaction_trigger.go`) hold the dispatcher, mode functions, AP/OP matrix, and `SetInteraction`. `npc_ai.go` lines 79-102 collapse to a single `n.processMovementInteraction(s)` call. `runNpcScript` signature extends to take a generic `target entity` with internal type-dispatch to `ActivePlayer`/`OtherActiveNpc`/`ActiveLoc`/`ActiveObj` pointers. `entity` interface gains `IsValid()`. `pkg/objtype/npctype.go` adopts the full 68-value TS `NpcMode` enum.

**Tech Stack:** Go 1.26+, existing `pkg/script` (all `TriggerAi{Op,Ap}{Player,Npc,Loc,Obj}{1..5}` and `TriggerAiQueue{1..20}` already defined), existing `pkg/coordgrid`, existing `pkg/entity` (`Loc`, `Obj`), existing `pkg/objtype.NpcType.AttackRange/MaxRange/GiveChase/DefaultMode`.

**Spec reference:** `docs/superpowers/specs/2026-04-22-nai-11-npc-movement-interaction-design.md`.

**Go command prefix for every Go invocation in this plan:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`

**Git commit flag:** every commit uses `--no-gpg-sign`.

---

## File Structure Map

| File                                            | Disposition     | Purpose                                                                                 |
|-------------------------------------------------|-----------------|-----------------------------------------------------------------------------------------|
| `pkg/objtype/npctype.go`                        | modify          | Add full `NPCMode*` enum (48 new constants adjacent to existing NPCModeQueue1..20)      |
| `pkg/objtype/npctype_test.go`                   | modify          | Add enum-boundary tests                                                                 |
| `modules/world/npc.go`                          | modify          | Add 7 new fields; delete Go-ad-hoc `NpcMode*` constants; `NewNpc` calls `defaultMode()` |
| `modules/world/npc_ai.go`                       | modify          | Lines 79-102 collapse to 1 call; delete `advanceWaypoint`/`wanderMode`/`patrolMode`      |
| `modules/world/npc_hunt.go`                     | modify          | Interaction branch calls `SetInteraction`; DEVIATION comment shrinks                    |
| `modules/world/interaction.go`                  | modify          | Add `IsValid() bool` to `entity` interface                                              |
| `modules/world/player.go`                       | modify          | Add `IsValid()` method                                                                  |
| `modules/world/npc_interaction.go`              | **create**      | Dispatcher, modes, setInteraction, validation, updateMovement (~320 LOC)                 |
| `modules/world/npc_interaction_test.go`         | **create**      | 29 test functions                                                                       |
| `modules/world/npc_interaction_trigger.go`      | **create**      | 8 fire helpers + 8 mode→trigger helpers + 2 dispatchers (~220 LOC)                        |
| `modules/world/npc_interaction_trigger_test.go` | **create**      | 24 test functions                                                                       |
| `pkg/entity/loc.go`                             | modify          | Add `IsValid()` method                                                                  |
| `pkg/entity/obj.go`                             | modify          | Add `IsValid()` method                                                                  |
| `pkg/script/active.go`                          | modify          | Expand `ActiveObj` stub; add `OtherActiveNpc` field semantics                           |
| `pkg/script/state.go`                           | modify          | Add `ActiveObj` field + `OtherActiveNpc` field + `PtrActiveObj`/`PtrOtherActiveNpc` flags |
| `pkg/script/npc_script.go`                      | modify          | Extend `runNpcScript` signature + internal type-dispatch                                |
| `pkg/script/npc_script_test.go`                 | modify          | 4 target-dispatch tests; migrate existing signature usages                              |
| Various test files (NAI-2..NAI-10)              | modify          | Migrate `NpcMode*` constant renames + `runNpcScript` signature usages (grep-and-list)    |

---

## Phase 1 — Infrastructure (constants, fields, interface, signature)

### Task 1: Add full `NPCMode*` enum to `pkg/objtype/npctype.go`

**Files:**
- Modify: `pkg/objtype/npctype.go` (existing const block around line 42-72)
- Test: `pkg/objtype/npctype_test.go`

**Context:** TS `NpcMode` at `Engine-TS/src/engine/entity/NpcMode.ts` defines 68 values (NULL=-1 through QUEUE20=66). Go has NPCModeNull=-1, NPCModeNone=0, NPCModeWander=1, and QUEUE1..20=47..66 only. This task adds the missing 45 values (NPCModePatrol through NPCModeApNpc5 — everything between WANDER and QUEUE1). Go naming convention: `NPCModeWander`, `NPCModePatrol`, `NPCModePlayerEscape`, `NPCModeOpPlayer1`, `NPCModeApLoc3`, etc. (match existing `NPCMode` prefix + TS CamelCase).

- [ ] **Step 1: Write the failing test**

Append to `pkg/objtype/npctype_test.go` (after `TestNPCModeQueueConstants` that already exists):

```go
func TestNPCModeFullEnum(t *testing.T) {
	// Mirrors Engine-TS/src/engine/entity/NpcMode.ts:1-96.
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"NPCModeNull", NPCModeNull, -1},
		{"NPCModeNone", NPCModeNone, 0},
		{"NPCModeWander", NPCModeWander, 1},
		{"NPCModePatrol", NPCModePatrol, 2},
		{"NPCModePlayerEscape", NPCModePlayerEscape, 3},
		{"NPCModePlayerFollow", NPCModePlayerFollow, 4},
		{"NPCModePlayerFace", NPCModePlayerFace, 5},
		{"NPCModePlayerFaceClose", NPCModePlayerFaceClose, 6},
		{"NPCModeOpPlayer1", NPCModeOpPlayer1, 7},
		{"NPCModeOpPlayer5", NPCModeOpPlayer5, 11},
		{"NPCModeApPlayer1", NPCModeApPlayer1, 12},
		{"NPCModeApPlayer5", NPCModeApPlayer5, 16},
		{"NPCModeOpLoc1", NPCModeOpLoc1, 17},
		{"NPCModeOpLoc5", NPCModeOpLoc5, 21},
		{"NPCModeApLoc1", NPCModeApLoc1, 22},
		{"NPCModeApLoc5", NPCModeApLoc5, 26},
		{"NPCModeOpObj1", NPCModeOpObj1, 27},
		{"NPCModeOpObj5", NPCModeOpObj5, 31},
		{"NPCModeApObj1", NPCModeApObj1, 32},
		{"NPCModeApObj5", NPCModeApObj5, 36},
		{"NPCModeOpNpc1", NPCModeOpNpc1, 37},
		{"NPCModeOpNpc5", NPCModeOpNpc5, 41},
		{"NPCModeApNpc1", NPCModeApNpc1, 42},
		{"NPCModeApNpc5", NPCModeApNpc5, 46},
		{"NPCModeQueue1", NPCModeQueue1, 47},
		{"NPCModeQueue20", NPCModeQueue20, 66},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s: got %d, want %d", tc.name, tc.got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run TestNPCModeFullEnum -v`

Expected: FAIL with "undefined: NPCModePatrol" (and many more).

- [ ] **Step 3: Add the constants in `pkg/objtype/npctype.go`**

Locate the existing `const (...)` block starting with `NPCModeNull = -1` and ending with `NPCModeQueue20 = 66` (around lines 42-72). Replace it with:

```go
// NPCMode values. Full enum mirroring Engine-TS/src/engine/entity/NpcMode.ts.
// Single source of truth — supersedes the Go-ad-hoc NpcMode* constants
// that previously lived in modules/world/npc.go.
const (
	NPCModeNull            = -1
	NPCModeNone            = 0
	NPCModeWander          = 1
	NPCModePatrol          = 2
	NPCModePlayerEscape    = 3
	NPCModePlayerFollow    = 4
	NPCModePlayerFace      = 5
	NPCModePlayerFaceClose = 6

	// OPPLAYER — [ai_opplayerN,npc]
	NPCModeOpPlayer1 = 7
	NPCModeOpPlayer2 = 8
	NPCModeOpPlayer3 = 9
	NPCModeOpPlayer4 = 10
	NPCModeOpPlayer5 = 11

	// APPLAYER — [ai_applayerN,npc]
	NPCModeApPlayer1 = 12
	NPCModeApPlayer2 = 13
	NPCModeApPlayer3 = 14
	NPCModeApPlayer4 = 15
	NPCModeApPlayer5 = 16

	// OPLOC — [ai_oplocN,npc]
	NPCModeOpLoc1 = 17
	NPCModeOpLoc2 = 18
	NPCModeOpLoc3 = 19
	NPCModeOpLoc4 = 20
	NPCModeOpLoc5 = 21

	// APLOC — [ai_aplocN,npc]
	NPCModeApLoc1 = 22
	NPCModeApLoc2 = 23
	NPCModeApLoc3 = 24
	NPCModeApLoc4 = 25
	NPCModeApLoc5 = 26

	// OPOBJ — [ai_opobjN,npc]
	NPCModeOpObj1 = 27
	NPCModeOpObj2 = 28
	NPCModeOpObj3 = 29
	NPCModeOpObj4 = 30
	NPCModeOpObj5 = 31

	// APOBJ — [ai_apobjN,npc]
	NPCModeApObj1 = 32
	NPCModeApObj2 = 33
	NPCModeApObj3 = 34
	NPCModeApObj4 = 35
	NPCModeApObj5 = 36

	// OPNPC — [ai_opnpcN,npc]
	NPCModeOpNpc1 = 37
	NPCModeOpNpc2 = 38
	NPCModeOpNpc3 = 39
	NPCModeOpNpc4 = 40
	NPCModeOpNpc5 = 41

	// APNPC — [ai_apnpcN,npc]
	NPCModeApNpc1 = 42
	NPCModeApNpc2 = 43
	NPCModeApNpc3 = 44
	NPCModeApNpc4 = 45
	NPCModeApNpc5 = 46

	// QUEUE — [ai_queueN,npc] dispatched by Npc.consumeHuntTarget.
	NPCModeQueue1  = 47
	NPCModeQueue2  = 48
	NPCModeQueue3  = 49
	NPCModeQueue4  = 50
	NPCModeQueue5  = 51
	NPCModeQueue6  = 52
	NPCModeQueue7  = 53
	NPCModeQueue8  = 54
	NPCModeQueue9  = 55
	NPCModeQueue10 = 56
	NPCModeQueue11 = 57
	NPCModeQueue12 = 58
	NPCModeQueue13 = 59
	NPCModeQueue14 = 60
	NPCModeQueue15 = 61
	NPCModeQueue16 = 62
	NPCModeQueue17 = 63
	NPCModeQueue18 = 64
	NPCModeQueue19 = 65
	NPCModeQueue20 = 66
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -v`

Expected: PASS (all tests including new `TestNPCModeFullEnum`).

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/npctype.go pkg/objtype/npctype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-11 add full NPCMode enum

Port TS NpcMode.ts (68 values) into pkg/objtype. Adds 45 new constants
from NPCModePatrol through NPCModeApNpc5; preserves existing
NPCModeNull/None/Wander and NPCModeQueue1..20."
```

---

### Task 2: Migrate Go-ad-hoc `NpcMode*` constants to `objtype.NPCMode*`

**Files:**
- Modify: `modules/world/npc.go:17-21` (delete old constants)
- Modify: `modules/world/npc.go` `NewNpc` (use objtype constants)
- Modify: `modules/world/npc_ai.go` `turn()` switch (use objtype constants)
- Modify: every test file referencing old constants (grep-and-list below)

**Context:** Go's ad-hoc `NpcModeNone=-1`, `NpcModeWander=0`, `NpcModePatrol=1` don't match TS values (which are 0, 1, 2). NAI-10 silently wrote TS-space values into the same field; this task closes the mismatch by deleting the ad-hoc set. CRITICAL semantic shift: targetOp-zero previously meant WANDER; now means NONE. Any test or code that reads targetOp==0 must be re-inspected.

- [ ] **Step 1: Grep every call site**

Run: `grep -rn "NpcModeNone\|NpcModeWander\|NpcModePatrol" modules/ pkg/`

Record the full list — each site needs migration. Expected sites (verify at runtime; list may be incomplete):
- `modules/world/npc.go:17-21` — constant definitions (DELETE).
- `modules/world/npc.go:101-137` — `NewNpc` `mode := NpcModeNone ... NpcModePatrol`.
- `modules/world/npc_ai.go:92-95` — switch cases in turn().
- `modules/world/npc_ai_test.go` — wander/patrol tests.
- `modules/world/npc_test.go` — NewNpc tests.
- `modules/world/npc_event_queue_test.go` — lifecycle tests.
- Any other hits surfaced by the grep.

- [ ] **Step 2: Update `modules/world/npc.go`**

Delete lines 17-21 (the `NpcModeNone/Wander/Patrol` const block).

Update `NewNpc` around line 100-137:
```go
func NewNpc(nid, typeId, x, z, level int, typ *objtype.NpcType) *Npc {
	mode := objtype.NPCModeNone
	if len(typ.PatrolCoord) > 0 {
		mode = objtype.NPCModePatrol
	} else if typ.WanderRange > 0 {
		mode = objtype.NPCModeWander
	}

	return &Npc{
		// ... existing fields ...
		targetOp: mode,
		// ... rest unchanged ...
	}
}
```

- [ ] **Step 3: Update `modules/world/npc_ai.go`**

Lines 91-96 (in `turn()`, movement block):
```go
switch n.targetOp {
case objtype.NPCModeWander:
    n.wanderMode(s)
case objtype.NPCModePatrol:
    n.patrolMode(s)
}
```

Add import if not present: `"github.com/zsrv/goscape/pkg/objtype"`.

- [ ] **Step 4: Migrate every test file from grep results**

Mechanical sed-like substitution. For each file in the grep output:
- Replace `NpcModeNone` → `objtype.NPCModeNone`
- Replace `NpcModeWander` → `objtype.NPCModeWander`
- Replace `NpcModePatrol` → `objtype.NPCModePatrol`
- Add `"github.com/zsrv/goscape/pkg/objtype"` import if missing.

**Semantic check per test file:** tests that assert `targetOp == 0` MUST be reviewed — `0` now means NPCModeNone (TS). If the test intended "wander" it must be updated to `objtype.NPCModeWander` (now 1).

- [ ] **Step 5: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS everywhere. Any failure here means a test made an implicit value assumption that no longer holds — investigate rather than silence.

- [ ] **Step 6: Re-grep to verify clean migration**

Run: `grep -rn "NpcModeNone\|NpcModeWander\|NpcModePatrol" modules/ pkg/`

Expected: ZERO results.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit --no-gpg-sign -m "refactor(world): NAI-11 migrate NpcMode constants to objtype.NPCMode*

Delete Go-ad-hoc NpcModeNone/Wander/Patrol (values -1, 0, 1).
Migrate NewNpc + Npc.turn + every test site to the TS-aligned
objtype.NPCMode* enum (NONE=0, WANDER=1, PATROL=2). Closes the latent
NAI-10 space mismatch where consumeHuntTarget wrote hunt.FindNewMode
(TS space) into a field whose switch used Go-ad-hoc values."
```

---

### Task 3: Add new interaction fields to `*Npc` struct

**Files:**
- Modify: `modules/world/npc.go` (struct + `NewNpc` initialisation)
- Test: `modules/world/npc_test.go`

**Context:** NAI-10 deferred seven fields; NAI-11 adds them. All default to "not set" values matching TS constructor semantics.

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_test.go`:

```go
func TestNewNpcInitialisesInteractionFields(t *testing.T) {
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	if n.apRange != 10 {
		t.Errorf("apRange: got %d, want 10", n.apRange)
	}
	if n.apRangeCalled != false {
		t.Errorf("apRangeCalled: got %t, want false", n.apRangeCalled)
	}
	if n.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1", n.targetSubject.com)
	}
	if n.targetSubject.typ != -1 {
		t.Errorf("targetSubject.typ: got %d, want -1", n.targetSubject.typ)
	}
	if n.targetX != -1 {
		t.Errorf("targetX: got %d, want -1", n.targetX)
	}
	if n.targetZ != -1 {
		t.Errorf("targetZ: got %d, want -1", n.targetZ)
	}
	if n.faceAngleX != -1 {
		t.Errorf("faceAngleX: got %d, want -1", n.faceAngleX)
	}
	if n.faceAngleZ != -1 {
		t.Errorf("faceAngleZ: got %d, want -1", n.faceAngleZ)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNewNpcInitialisesInteractionFields -v`

Expected: FAIL — field `apRange` (etc.) unknown.

- [ ] **Step 3: Add struct fields and type**

In `modules/world/npc.go`, after the existing `interaction` section (around line 82-84), add fields and above the `Npc` struct declaration, add the helper type:

```go
// npcTargetSubject captures the "initial target snapshot" fields TS
// Npc uses in validateTarget to detect mid-interaction changetypes.
// Mirrors TS targetSubject = { com: number, type: number }.
type npcTargetSubject struct {
	com int // -1 when unset (TS truthy-coerced from falsy values)
	typ int // -1 when unset or when target is a Player
}
```

Then extend the `Npc` struct's interaction section:

```go
	// === interaction ===
	target         entity
	faceEntity     int
	apRange        int              // NAI-11: default 10; -1 sentinel = "no AP script"
	apRangeCalled  bool             // NAI-11
	targetSubject  npcTargetSubject // NAI-11
	targetX        int              // NAI-11: fine-grained coord for non-PathingEntity targets
	targetZ        int
	faceAngleX     int              // NAI-11: fine-grained coord, mask-emitted via faceSquare
	faceAngleZ     int
```

- [ ] **Step 4: Initialise the fields in `NewNpc`**

In the `NewNpc` return struct literal, add (adjacent to the other `-1`-defaulted fields):

```go
		apRange:        10,
		apRangeCalled:  false,
		targetSubject:  npcTargetSubject{com: -1, typ: -1},
		targetX:        -1,
		targetZ:        -1,
		faceAngleX:     -1,
		faceAngleZ:     -1,
```

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNewNpcInitialisesInteractionFields -v`

Expected: PASS.

- [ ] **Step 6: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS — adding struct fields should not affect any existing behaviour.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc.go modules/world/npc_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add interaction fields to Npc struct

Adds apRange, apRangeCalled, targetSubject, targetX, targetZ,
faceAngleX, faceAngleZ — the seven fields TS setInteraction writes
that NAI-10 deferred. NewNpc initialises to TS-matching defaults
(apRange=10; others -1 or zero-value)."
```

---

### Task 4: Add `IsValid()` to `entity` interface + concrete implementations

**Files:**
- Modify: `modules/world/interaction.go` (entity interface)
- Modify: `modules/world/npc.go` (add IsValid method)
- Modify: `modules/world/player.go` (add IsValid method)
- Modify: `pkg/entity/loc.go` (add IsValid method)
- Modify: `pkg/entity/obj.go` (add IsValid method)
- Test: `modules/world/npc_test.go`, `modules/world/player_test.go`, `pkg/entity/loc_test.go`, `pkg/entity/obj_test.go`

**Context:** Spec Section 6: layered validity. Intrinsic `IsValid()` lives on each entity type; zone-membership stays as world-module helpers (`locStillValid`/`objStillValid`).

- [ ] **Step 1: Locate the entity interface**

Run: `grep -n "type entity interface" modules/world/`

Expected: one hit, likely in `modules/world/interaction.go` or a related file. Open that file and find the interface definition. It currently has `Coords() (x, z, level int)` and `Slot() int`.

- [ ] **Step 2: Write the failing tests**

Append to `modules/world/npc_test.go`:

```go
func TestNpcIsValid(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	if !n.IsValid() {
		t.Error("fresh npc: IsValid = false, want true")
	}
	n.dead = true
	if n.IsValid() {
		t.Error("dead npc: IsValid = true, want false")
	}
}
```

Append to `modules/world/player_test.go` (if file exists; else create minimal skeleton first):

```go
func TestPlayerIsValid(t *testing.T) {
	p := &Player{online: true, client: &client{}}
	if !p.IsValid() {
		t.Error("online player with client: IsValid = false, want true")
	}
	p.online = false
	if p.IsValid() {
		t.Error("offline player: IsValid = true, want false")
	}
	p.online = true
	p.client = nil
	if p.IsValid() {
		t.Error("online player without client: IsValid = true, want false")
	}
}
```

Append to `pkg/entity/loc_test.go` (if file exists; else create):

```go
func TestLocIsValid(t *testing.T) {
	l := NewLoc(0, 100, 100, LifecycleRespawn, 42, 10, 0, 0)
	if !l.IsValid() {
		t.Error("fresh loc: IsValid = false, want true")
	}
}
```

Append to `pkg/entity/obj_test.go` (if file exists; else create):

```go
func TestObjIsValid(t *testing.T) {
	o := NewObj(0, 100, 100, LifecycleRespawn, 42, 1)
	if !o.IsValid() {
		t.Error("fresh obj: IsValid = false, want true")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -run "TestNpcIsValid|TestPlayerIsValid|TestLocIsValid|TestObjIsValid" -v`

Expected: compile errors — `IsValid` undefined on all four types.

- [ ] **Step 4: Add `IsValid()` to the entity interface**

In the file containing `type entity interface`, add:
```go
type entity interface {
	Coords() (x, z, level int)
	Slot() int
	IsValid() bool // NAI-11 — intrinsic validity; zone-membership is checked separately
}
```

- [ ] **Step 5: Implement `IsValid()` on `*Npc`**

Append to `modules/world/npc.go`:
```go
// IsValid returns whether the NPC is intrinsically alive. Matches the
// "lifecycle" half of TS Npc.isValid. The stronger TS isActive
// additionally checks !delayed; validateTarget handles that separately
// when the target is an *Npc.
func (n *Npc) IsValid() bool {
	return !n.dead
}
```

- [ ] **Step 6: Implement `IsValid()` on `*Player`**

Append to `modules/world/player.go`:
```go
// IsValid returns whether the player's session is live. TS Player.isValid
// takes an optional hash64 for cross-server session checking; Go's
// single-server case collapses to "online with an attached client".
func (p *Player) IsValid() bool {
	return p.online && p.client != nil
}
```

If `online` field doesn't exist on `*Player`, grep for the equivalent ("p.loggedIn"? "p.connected"?) and use that. If no equivalent, just return `p.client != nil`.

- [ ] **Step 7: Implement `IsValid()` on `*entitypkg.Loc`**

Append to `pkg/entity/loc.go`:
```go
// IsValid returns the loc's intrinsic validity. Zone-membership
// (pointer still in zoneMap.Get(level,x,z).Locs) is checked
// separately by world-module helpers at the validateTarget call site,
// because pkg/entity cannot depend on modules/world.
func (l *Loc) IsValid() bool {
	return true // intrinsic lifecycle check; world-module helper does zone lookup
}
```

- [ ] **Step 8: Implement `IsValid()` on `*entitypkg.Obj`**

Append to `pkg/entity/obj.go`:
```go
// IsValid returns the obj's intrinsic validity. Same layering as Loc:
// zone-membership check lives in the world module.
func (o *Obj) IsValid() bool {
	return true
}
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -run "IsValid" -v`

Expected: PASS.

- [ ] **Step 10: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS — adding the interface method should not break any existing consumer because every current `entity` implementer now satisfies it.

- [ ] **Step 11: Commit**

```bash
git add -A
git commit --no-gpg-sign -m "feat(world): NAI-11 add IsValid() to entity interface

Intrinsic-validity method on Player/Npc/Loc/Obj. Zone-membership
checks stay as modules/world helpers (pkg/entity cannot import
modules/world). Layered validity matches TS's conceptual split
between Entity.isValid() and world-side zone tracking."
```

---

### Task 5: Extend `ScriptState` with `ActiveObj`, `OtherActiveNpc`, pointer flags

**Files:**
- Modify: `pkg/script/state.go` (ScriptState fields)
- Modify: `pkg/script/active.go` (ActiveObj interface, pointer flags)

**Context:** Per spec Section 5, `runNpcScript` needs to type-dispatch targets into distinct `ScriptState` pointers. `ActiveLoc` + `PtrActiveLoc` already exist. Missing: `ActiveObj` (currently empty-interface stub), `OtherActiveNpc` (new), and their pointer flags.

- [ ] **Step 1: Inventory existing pointer-flag constants**

Run: `grep -n "^\s*PtrActive\|Pointer \=\|Pointer uint\|type Pointer" pkg/script/`

Record the shape. Typical: `type Pointer uint32` + constants like `PtrActivePlayer = 1 << iota`.

- [ ] **Step 2: Expand `ActiveObj` interface in `pkg/script/active.go`**

Locate `type ActiveObj interface{}` (around line 396) and replace with:

```go
// ActiveObj is the surface that OBJ_* and AI_APOBJ/AI_OPOBJ handlers
// use to read obj state. Narrow by design — extend as future sub-specs
// wire more obj script opcodes.
type ActiveObj interface {
	ObjType() int                        // underlying ObjType id
	Coords() (x, z, level int)           // world position
}
```

- [ ] **Step 3: Add new pointer-flag constants + ScriptState fields**

In `pkg/script/active.go` (or wherever the `Pointer` constants live), add (preserve `iota` ordering by appending at the end):

```go
	// NAI-11 additions.
	PtrActiveObj
	PtrOtherActiveNpc
```

In `pkg/script/state.go`, extend the `ScriptState` struct:

```go
type ScriptState struct {
	// ... existing fields unchanged ...
	Pointers       Pointer
	ActivePlayer   ActivePlayer    // existing
	ActiveNpc      ActiveNpc       // existing
	ActiveLoc      ActiveLoc       // existing
	ActiveObj      ActiveObj       // NAI-11
	OtherActiveNpc ActiveNpc       // NAI-11 — secondary NPC for NPC→NPC AI dispatch
}
```

- [ ] **Step 4: Make `*entitypkg.Obj` satisfy the new `ActiveObj` interface**

The Obj struct already has `Coords()`. Add `ObjType()` as a method alias for the public `Type` field. Append to `pkg/entity/obj.go`:

```go
// ObjType returns the obj's type id. Wrapper around the public Type
// field so *Obj satisfies script.ActiveObj.
func (o *Obj) ObjType() int { return o.Type }
```

(Using `ObjType()` rather than `Type()` to avoid a Go method-name collision with the `Type` field on the same struct — Go disallows both a field and a method with the same name on the same type.)

- [ ] **Step 5: Run compile check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean build.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all existing tests pass — these are additive changes.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit --no-gpg-sign -m "feat(script): NAI-11 expand ScriptState with ActiveObj + OtherActiveNpc

Adds PtrActiveObj + PtrOtherActiveNpc pointer flags. Expands ActiveObj
interface from empty stub to {ObjType() int; Coords() (int,int,int)}.
*entitypkg.Obj satisfies the interface via a new ObjType() method
(field rename is prevented by Go's no-field-method-name-collision
rule). Prep for NAI-11's runNpcScript target type-dispatch."
```

---

### Task 6: Extend `runNpcScript` signature + internal target type-dispatch

**Files:**
- Modify: `pkg/script/npc_script.go`
- Test: `pkg/script/npc_script_test.go`

**Context:** Spec Section 5 — signature expands from `(sf, n, activePlayer *Player, req)` to `(sf, n, target entity, req)`. Internal type-switch wires the appropriate `ScriptState` field + pointer flag. This task DEFINES the new signature but does not yet migrate callers (next task does that).

Because `pkg/script` cannot import `modules/world` (layering), the `target` parameter must be a type already visible from `pkg/script`. The existing helper pattern uses the `ActiveXxx` interfaces. `runNpcScript` is called FROM `modules/world`, so the caller passes an `entity`-typed value — but `pkg/script` needs to accept it as `ActivePlayer | ActiveNpc | ActiveLoc | ActiveObj` or as a concrete-typed nil.

**Resolution:** change `runNpcScript` to accept `target any` (empty interface) and type-switch internally against the `ActiveXxx` interfaces. Each `ActiveXxx` already exists in `pkg/script/active.go`. The caller passes `target entity` — any concrete `entity` value also satisfies its corresponding `Active*` interface.

- [ ] **Step 1: Read existing `runNpcScript`**

Run: `grep -n "func.*runNpcScript" pkg/script/`

Open the file and inspect the current signature + body.

- [ ] **Step 2: Write the failing tests**

Append to `pkg/script/npc_script_test.go`:

```go
func TestRunNpcScriptDispatchesActivePlayerTarget(t *testing.T) {
	s := newServerForNpcScriptTest(t)   // existing helper
	n := newNpcForScriptTest(t)
	p := &mockActivePlayer{slot: 5}     // existing mock

	sf := newCapturingScriptFile(t)
	s.runNpcScript(sf, n, p, nil)

	state := sf.capturedState
	if state.ActivePlayer != p {
		t.Errorf("ActivePlayer: got %v, want %v", state.ActivePlayer, p)
	}
	if state.Pointers&PtrActivePlayer == 0 {
		t.Error("Pointers: PtrActivePlayer flag not set")
	}
}

func TestRunNpcScriptDispatchesActiveLocTarget(t *testing.T) {
	s := newServerForNpcScriptTest(t)
	n := newNpcForScriptTest(t)
	l := &mockActiveLoc{locType: 42}

	sf := newCapturingScriptFile(t)
	s.runNpcScript(sf, n, l, nil)

	state := sf.capturedState
	if state.ActiveLoc != l {
		t.Errorf("ActiveLoc: got %v, want %v", state.ActiveLoc, l)
	}
	if state.Pointers&PtrActiveLoc == 0 {
		t.Error("Pointers: PtrActiveLoc flag not set")
	}
}

func TestRunNpcScriptDispatchesActiveObjTarget(t *testing.T) {
	s := newServerForNpcScriptTest(t)
	n := newNpcForScriptTest(t)
	o := &mockActiveObj{objType: 99}

	sf := newCapturingScriptFile(t)
	s.runNpcScript(sf, n, o, nil)

	state := sf.capturedState
	if state.ActiveObj != o {
		t.Errorf("ActiveObj: got %v, want %v", state.ActiveObj, o)
	}
	if state.Pointers&PtrActiveObj == 0 {
		t.Error("Pointers: PtrActiveObj flag not set")
	}
}

func TestRunNpcScriptDispatchesOtherActiveNpcTarget(t *testing.T) {
	s := newServerForNpcScriptTest(t)
	n := newNpcForScriptTest(t)
	other := newNpcForScriptTest(t) // second NPC as target

	sf := newCapturingScriptFile(t)
	s.runNpcScript(sf, n, other, nil)

	state := sf.capturedState
	if state.OtherActiveNpc == nil {
		t.Error("OtherActiveNpc: nil, want set")
	}
	if state.Pointers&PtrOtherActiveNpc == 0 {
		t.Error("Pointers: PtrOtherActiveNpc flag not set")
	}
}

// mockActiveObj is a test fixture satisfying script.ActiveObj.
type mockActiveObj struct {
	objType int
	x, z, l int
}

func (m *mockActiveObj) ObjType() int               { return m.objType }
func (m *mockActiveObj) Coords() (int, int, int)    { return m.x, m.z, m.l }
```

Add a `newCapturingScriptFile` helper and a fixture hook that captures `state` before execute. If the existing `newNoopScriptFile` helper doesn't expose the state, extend or add a new helper that does.

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestRunNpcScriptDispatches" -v`

Expected: compile error or FAIL — signature doesn't accept the target types yet.

- [ ] **Step 4: Rewrite `runNpcScript`**

Replace the function body in `pkg/script/npc_script.go`:

```go
// runNpcScript initialises a ScriptState anchored on n and (optionally) a
// target entity, then dispatches to resumeOrFinishNpc. Handles a nil sf
// gracefully — a nil script file is a "no-op" fire (matches TS
// ScriptProvider.getByTrigger returning null).
//
// Target type-dispatch (NAI-11):
//   - target == nil: no secondary pointer (AI_TIMER, AI_DESPAWN, AI_QUEUE*)
//   - target satisfies ActivePlayer: state.ActivePlayer + PtrActivePlayer
//   - target satisfies ActiveLoc:    state.ActiveLoc    + PtrActiveLoc
//   - target satisfies ActiveObj:    state.ActiveObj    + PtrActiveObj
//   - target satisfies ActiveNpc:    state.OtherActiveNpc + PtrOtherActiveNpc
//     (the PRIMARY ActiveNpc is `n` itself; target is a secondary)
//
// Check order matters: ActivePlayer first (most specific/common in TS AI
// triggers), ActiveNpc last (so a Player target doesn't accidentally
// dispatch into the OtherActiveNpc branch via interface promotion).
func (s *Server) runNpcScript(sf *ScriptFile, n ActiveNpc, target any, req *NpcQueueRequest) {
	if sf == nil {
		return
	}

	state := Init(sf, nil, true, nil, nil)
	state.ActiveNpc = n
	state.Pointers |= PtrActiveNpc
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup

	switch t := target.(type) {
	case nil:
		// No secondary pointer.
	case ActivePlayer:
		state.ActivePlayer = t
		state.Pointers |= PtrActivePlayer
	case ActiveLoc:
		state.ActiveLoc = t
		state.Pointers |= PtrActiveLoc
	case ActiveObj:
		state.ActiveObj = t
		state.Pointers |= PtrActiveObj
	case ActiveNpc:
		state.OtherActiveNpc = t
		state.Pointers |= PtrOtherActiveNpc
	}

	if req != nil {
		state.QueueIntArg = req.IntArg
	}

	s.resumeOrFinishNpc(state, n)
}
```

Adjust to match the exact surrounding API surface (the existing `runNpcScript` may have slightly different init/wiring — preserve it; only the target-dispatch block is new).

- [ ] **Step 5: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestRunNpcScriptDispatches" -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/npc_script.go pkg/script/npc_script_test.go
git commit --no-gpg-sign -m "feat(script): NAI-11 extend runNpcScript with target type-dispatch

runNpcScript(sf, n, target any, req) — replaces (sf, n, activePlayer, req).
Type-switches target against ActivePlayer/ActiveLoc/ActiveObj/ActiveNpc
interfaces and wires the appropriate ScriptState field + pointer flag.
nil target skips the secondary pointer (AI_TIMER/DESPAWN/QUEUE* paths).
Callers migrate in the next task."
```

---

### Task 7: Migrate existing `runNpcScript` call sites to new signature

**Files:**
- Modify: every call site of `runNpcScript` in `modules/world/` and `pkg/script/`
- Modify: every test file using the old signature

**Context:** Mechanical migration. Old: `runNpcScript(sf, n, activePlayer, req)`. New: `runNpcScript(sf, n, target, req)`. For every existing call site, `activePlayer` becomes `target` — still a `*Player` at runtime, still satisfies `ActivePlayer`, so the internal type-switch routes identically. Except nil callers (currently `activePlayer=nil`): `target=nil` explicit (no change).

- [ ] **Step 1: Grep every call site**

Run: `grep -rn "runNpcScript(" modules/ pkg/`

Expected sites (verify at runtime):
- `modules/world/npc_timer.go` (NAI-4)
- `modules/world/npc_queue.go` (NAI-3)
- `modules/world/npc_event_queue.go` (NAI-5)
- `modules/world/npc_hunt.go:191` (NAI-10 QUEUE branch)
- `pkg/script/npc_script.go` (internal recursion in resumeOrFinishNpc — verify)
- All test files named `*_test.go` invoking the helper.

- [ ] **Step 2: Migrate each site**

For sites currently passing `nil` as the third arg: no change needed — nil is a valid `target any`.

For sites currently passing `activePlayer *Player` or similar concrete player: no change needed — `*Player` implements `ActivePlayer`, so `target any` still routes correctly.

The function-signature declaration itself already changed in Task 6; every caller automatically adopts the new shape.

- [ ] **Step 3: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS. If any test fails, it's because the old signature used a `*Player`-typed parameter and the new `target any` now routes into a different branch. Inspect case-by-case.

- [ ] **Step 4: Commit (even if empty diff)**

If no source changes were needed, just verify and record the migration completeness with a no-op commit:

```bash
git commit --no-gpg-sign --allow-empty -m "chore(world): NAI-11 verify all runNpcScript call sites adopt new signature

Task 6 changed the signature. Task 7 confirms all existing callers
at NAI-2..NAI-10 compile and run under the new signature without
modification (nil → nil route; *Player → ActivePlayer route)."
```

If source changes WERE needed, commit them:

```bash
git add -A
git commit --no-gpg-sign -m "refactor(world): NAI-11 migrate runNpcScript call sites to target-any signature"
```

---

### Task 8: Add `coordgrid.Fine` + `coordgrid.DistanceToSW` (if missing)

**Files:**
- Modify: `pkg/coordgrid/coordgrid.go` (or similar)
- Test: `pkg/coordgrid/coordgrid_test.go`

**Context:** Spec's `SetInteraction` uses `coordgrid.Fine(coord, size)` to convert coarse-grained tile coords to fine-grained centre coords. Spec's `targetWithinMaxRange` uses `distanceToSW`. Verify existence first.

- [ ] **Step 1: Check for existing helpers**

Run: `grep -n "func Fine\|func DistanceToSW" pkg/coordgrid/`

If BOTH exist: skip this task, continue to Task 9.

- [ ] **Step 2: Write the failing test**

If `Fine` is missing, append to `pkg/coordgrid/coordgrid_test.go`:

```go
func TestFine(t *testing.T) {
	// TS CoordGrid.fine(coord, size): coord*64 + (size*64 - 1) / 2.
	// For a 1x1 entity at tile 100: fine = 100*64 + 31 = 6431.
	tests := []struct {
		name       string
		coord, siz int
		want       int
	}{
		{"1x1 at 0", 0, 1, 31},
		{"1x1 at 100", 100, 1, 6431},
		{"2x2 at 0", 0, 2, 63},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Fine(tc.coord, tc.siz); got != tc.want {
				t.Errorf("Fine(%d,%d) = %d, want %d", tc.coord, tc.siz, got, tc.want)
			}
		})
	}
}
```

If `DistanceToSW` is missing, append:

```go
func TestDistanceToSW(t *testing.T) {
	// TS CoordGrid.distanceToSW: Chebyshev max(|ax-bx|, |az-bz|) using
	// both entities' SW corner (x,z) coords directly.
	tests := []struct {
		ax, az, bx, bz int
		want           int
	}{
		{0, 0, 3, 4, 4},
		{0, 0, 0, 0, 0},
		{10, 10, 10, 5, 5},
	}
	for _, tc := range tests {
		got := DistanceToSW(tc.ax, tc.az, tc.bx, tc.bz)
		if got != tc.want {
			t.Errorf("DistanceToSW(%d,%d,%d,%d) = %d, want %d", tc.ax, tc.az, tc.bx, tc.bz, got, tc.want)
		}
	}
}
```

- [ ] **Step 3: Implement the missing helpers**

Append to `pkg/coordgrid/coordgrid.go`:

```go
// Fine converts a coarse-grained tile coord + size to the fine-grained
// centre coord used by the face-angle mask. TS formula:
// coord * 64 + (size * 64 - 1) / 2. Matches CoordGrid.fine.
func Fine(coord, size int) int {
	return coord*64 + (size*64-1)/2
}

// DistanceToSW returns the Chebyshev distance between two SW-corner
// coords. Matches TS CoordGrid.distanceToSW for 1x1 entities; caller
// is responsible for using the correct corner for multi-tile cases.
func DistanceToSW(ax, az, bx, bz int) int {
	dx := ax - bx
	if dx < 0 {
		dx = -dx
	}
	dz := az - bz
	if dz < 0 {
		dz = -dz
	}
	if dx > dz {
		return dx
	}
	return dz
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/coordgrid/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/coordgrid/
git commit --no-gpg-sign -m "feat(coordgrid): NAI-11 add Fine + DistanceToSW helpers

Fine(coord, size) converts tile coord + size to fine-grained centre
coord — used by Npc.focus for face-angle mask. DistanceToSW(ax,az,bx,bz)
is Chebyshev distance between SW corners — used by targetWithinMaxRange."
```

(Skip this commit if both helpers already existed.)

---

### Task 9: Add `objStillValid` helper to `modules/world`

**Files:**
- Modify: `modules/world/interaction_trigger.go` (add next to existing `locStillValid`)
- Test: `modules/world/interaction_trigger_test.go`

**Context:** Mirror of `locStillValid` for Obj targets. Used by NAI-11's `validateTarget` when target is an `*Obj`.

- [ ] **Step 1: Read existing `locStillValid`**

Run: `grep -n "func locStillValid" modules/world/`

Open `modules/world/interaction_trigger.go` at the hit line (~218). Inspect the shape.

- [ ] **Step 2: Write the failing test**

Append to `modules/world/interaction_trigger_test.go`:

```go
func TestObjStillValid(t *testing.T) {
	s := newServerWithZoneMap(t) // existing helper; if unavailable, assemble inline
	o := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleRespawn, 42, 1)
	// Place obj in zone map.
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Objs = append(zn.Objs, o)

	if !objStillValid(s, o, 100, 100, 0) {
		t.Error("present obj: objStillValid = false, want true")
	}

	// Remove and re-check.
	zn.Objs = nil
	if objStillValid(s, o, 100, 100, 0) {
		t.Error("removed obj: objStillValid = true, want false")
	}
}
```

If the zone map doesn't track `Objs` yet on zones, adapt: check whichever zone collection holds objs. If none exists, the simplest fidelity-preserving implementation is to return `o.Lifecycle` != despawned (intrinsic check, deferring real zone-membership) — but that makes `objStillValid` redundant with `IsValid`. Prefer wiring zone-lookup if the zone map already tracks obs.

- [ ] **Step 3: Implement `objStillValid`**

Append to `modules/world/interaction_trigger.go`:

```go
// objStillValid checks whether the held *Obj pointer still represents
// the same obj at the same tile. Zone-membership is authoritative —
// parallels locStillValid for loc targets.
func objStillValid(srv *Server, obj *entitypkg.Obj, wantX, wantZ, wantLevel int) bool {
	zn := srv.zoneMap.Get(wantLevel, wantX, wantZ)
	return slices.Contains(zn.Objs, obj)
}
```

If `zn.Objs` doesn't exist yet, add the field to the zone type (one-line addition in the zone-file) OR use whatever existing collection the zone uses for objs. If the infrastructure is incomplete, implement the helper to return `obj.Lifecycle != entitypkg.LifecycleDespawned` as a graceful fallback and add a tracked-deviation comment.

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestObjStillValid -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add objStillValid helper

Mirror of locStillValid for *Obj targets. Consumed by validateTarget
when an NPC's interaction target is an Obj."
```

---

## Phase 2 — Helpers and small components (TDD by unit)

### Task 10: `checkOpTrigger` / `checkApTrigger` range checks

**Files:**
- Create: `modules/world/npc_interaction.go`
- Create: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 4. The two functions map a targetOp value to "is this in the OP-trigger range" / "is this in the AP-trigger range". TS Npc.ts:1064-1080. Each iterates the 4-category × 5-op bands.

- [ ] **Step 1: Create `npc_interaction.go` with the two functions**

Create `modules/world/npc_interaction.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
)

// checkOpTrigger reports whether targetOp falls in any OP-trigger band
// (OPPLAYER / OPLOC / OPOBJ / OPNPC — 5 values each, four bands).
// Matches TS Npc.checkOpTrigger at Engine-TS/.../Npc.ts:1073-1080.
func checkOpTrigger(op int) bool {
	return (op >= objtype.NPCModeOpPlayer1 && op <= objtype.NPCModeOpPlayer5) ||
		(op >= objtype.NPCModeOpLoc1 && op <= objtype.NPCModeOpLoc5) ||
		(op >= objtype.NPCModeOpObj1 && op <= objtype.NPCModeOpObj5) ||
		(op >= objtype.NPCModeOpNpc1 && op <= objtype.NPCModeOpNpc5)
}

// checkApTrigger reports whether targetOp falls in any AP-trigger band
// (APPLAYER / APLOC / APOBJ / APNPC — 5 values each, four bands).
// Matches TS Npc.checkApTrigger at Engine-TS/.../Npc.ts:1064-1071.
func checkApTrigger(op int) bool {
	return (op >= objtype.NPCModeApPlayer1 && op <= objtype.NPCModeApPlayer5) ||
		(op >= objtype.NPCModeApLoc1 && op <= objtype.NPCModeApLoc5) ||
		(op >= objtype.NPCModeApObj1 && op <= objtype.NPCModeApObj5) ||
		(op >= objtype.NPCModeApNpc1 && op <= objtype.NPCModeApNpc5)
}
```

- [ ] **Step 2: Create test file**

Create `modules/world/npc_interaction_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestCheckOpTrigger(t *testing.T) {
	ops := []struct {
		name string
		op   int
		want bool
	}{
		{"OpPlayer1 (7)", objtype.NPCModeOpPlayer1, true},
		{"OpPlayer5 (11)", objtype.NPCModeOpPlayer5, true},
		{"OpLoc1 (17)", objtype.NPCModeOpLoc1, true},
		{"OpLoc5 (21)", objtype.NPCModeOpLoc5, true},
		{"OpObj1 (27)", objtype.NPCModeOpObj1, true},
		{"OpObj5 (31)", objtype.NPCModeOpObj5, true},
		{"OpNpc1 (37)", objtype.NPCModeOpNpc1, true},
		{"OpNpc5 (41)", objtype.NPCModeOpNpc5, true},
		{"ApPlayer1 (12) — NOT op", objtype.NPCModeApPlayer1, false},
		{"ApNpc5 (46) — NOT op", objtype.NPCModeApNpc5, false},
		{"PatrolMode (2)", objtype.NPCModePatrol, false},
		{"NoneMode (0)", objtype.NPCModeNone, false},
		{"Queue1 (47)", objtype.NPCModeQueue1, false},
		{"Null (-1)", objtype.NPCModeNull, false},
	}
	for _, tc := range ops {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkOpTrigger(tc.op); got != tc.want {
				t.Errorf("checkOpTrigger(%d) = %t, want %t", tc.op, got, tc.want)
			}
		})
	}
}

func TestCheckApTrigger(t *testing.T) {
	ops := []struct {
		name string
		op   int
		want bool
	}{
		{"ApPlayer1 (12)", objtype.NPCModeApPlayer1, true},
		{"ApPlayer5 (16)", objtype.NPCModeApPlayer5, true},
		{"ApLoc1 (22)", objtype.NPCModeApLoc1, true},
		{"ApLoc5 (26)", objtype.NPCModeApLoc5, true},
		{"ApObj1 (32)", objtype.NPCModeApObj1, true},
		{"ApObj5 (36)", objtype.NPCModeApObj5, true},
		{"ApNpc1 (42)", objtype.NPCModeApNpc1, true},
		{"ApNpc5 (46)", objtype.NPCModeApNpc5, true},
		{"OpPlayer1 (7) — NOT ap", objtype.NPCModeOpPlayer1, false},
		{"OpNpc5 (41) — NOT ap", objtype.NPCModeOpNpc5, false},
		{"PatrolMode (2)", objtype.NPCModePatrol, false},
		{"Queue20 (66)", objtype.NPCModeQueue20, false},
	}
	for _, tc := range ops {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkApTrigger(tc.op); got != tc.want {
				t.Errorf("checkApTrigger(%d) = %t, want %t", tc.op, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestCheckOpTrigger|TestCheckApTrigger" -v`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add checkOpTrigger / checkApTrigger range checks

Range-based classifiers over NPCMode targetOp values. Map targetOp to
{OP, AP, neither} based on TS NpcMode 4-band × 5-op layout. Foundation
for the mode→trigger dispatch helpers in npc_interaction_trigger.go."
```

---

### Task 11: 8 mode→trigger map helpers in `npc_interaction_trigger.go`

**Files:**
- Create: `modules/world/npc_interaction_trigger.go`
- Create: `modules/world/npc_interaction_trigger_test.go`

**Context:** Spec Section 4. Eight functions, each mapping a targetOp value to its corresponding `script.ServerTriggerType`, returning `0` for out-of-range.

- [ ] **Step 1: Create the production file with all 8 helpers**

Create `modules/world/npc_interaction_trigger.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// aiApPlayerTriggerForOp maps an APPLAYER targetOp (12..16) to the
// matching TriggerAiApPlayer{1..5}. Returns 0 for out-of-range.
func aiApPlayerTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeApPlayer1 && op <= objtype.NPCModeApPlayer5 {
		return script.TriggerAiApPlayer1 + script.ServerTriggerType(op-objtype.NPCModeApPlayer1)
	}
	return 0
}

// aiOpPlayerTriggerForOp: OPPLAYER targetOp (7..11) → TriggerAiOpPlayer{1..5}.
func aiOpPlayerTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeOpPlayer1 && op <= objtype.NPCModeOpPlayer5 {
		return script.TriggerAiOpPlayer1 + script.ServerTriggerType(op-objtype.NPCModeOpPlayer1)
	}
	return 0
}

// aiApLocTriggerForOp: APLOC targetOp (22..26) → TriggerAiApLoc{1..5}.
func aiApLocTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeApLoc1 && op <= objtype.NPCModeApLoc5 {
		return script.TriggerAiApLoc1 + script.ServerTriggerType(op-objtype.NPCModeApLoc1)
	}
	return 0
}

// aiOpLocTriggerForOp: OPLOC targetOp (17..21) → TriggerAiOpLoc{1..5}.
func aiOpLocTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeOpLoc1 && op <= objtype.NPCModeOpLoc5 {
		return script.TriggerAiOpLoc1 + script.ServerTriggerType(op-objtype.NPCModeOpLoc1)
	}
	return 0
}

// aiApObjTriggerForOp: APOBJ targetOp (32..36) → TriggerAiApObj{1..5}.
func aiApObjTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeApObj1 && op <= objtype.NPCModeApObj5 {
		return script.TriggerAiApObj1 + script.ServerTriggerType(op-objtype.NPCModeApObj1)
	}
	return 0
}

// aiOpObjTriggerForOp: OPOBJ targetOp (27..31) → TriggerAiOpObj{1..5}.
func aiOpObjTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeOpObj1 && op <= objtype.NPCModeOpObj5 {
		return script.TriggerAiOpObj1 + script.ServerTriggerType(op-objtype.NPCModeOpObj1)
	}
	return 0
}

// aiApNpcTriggerForOp: APNPC targetOp (42..46) → TriggerAiApNpc{1..5}.
func aiApNpcTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeApNpc1 && op <= objtype.NPCModeApNpc5 {
		return script.TriggerAiApNpc1 + script.ServerTriggerType(op-objtype.NPCModeApNpc1)
	}
	return 0
}

// aiOpNpcTriggerForOp: OPNPC targetOp (37..41) → TriggerAiOpNpc{1..5}.
func aiOpNpcTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeOpNpc1 && op <= objtype.NPCModeOpNpc5 {
		return script.TriggerAiOpNpc1 + script.ServerTriggerType(op-objtype.NPCModeOpNpc1)
	}
	return 0
}
```

- [ ] **Step 2: Create the test file**

Create `modules/world/npc_interaction_trigger_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

type triggerMapCase struct {
	op   int
	want script.ServerTriggerType
}

func runTriggerMapTest(t *testing.T, name string, fn func(int) script.ServerTriggerType, cases []triggerMapCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := fn(tc.op); got != tc.want {
				t.Errorf("%s(%d) = %d, want %d", name, tc.op, got, tc.want)
			}
		})
	}
}

func TestAiApNpcTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApNpc1, script.TriggerAiApNpc1},
		{objtype.NPCModeApNpc2, script.TriggerAiApNpc2},
		{objtype.NPCModeApNpc3, script.TriggerAiApNpc3},
		{objtype.NPCModeApNpc4, script.TriggerAiApNpc4},
		{objtype.NPCModeApNpc5, script.TriggerAiApNpc5},
		{objtype.NPCModeApNpc5 + 1, 0}, // out-of-range
		{objtype.NPCModeApNpc1 - 1, 0}, // out-of-range
		{objtype.NPCModeApPlayer1, 0},  // wrong category
	}
	runTriggerMapTest(t, "aiApNpcTriggerForOp", aiApNpcTriggerForOp, cases)
}

func TestAiOpNpcTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpNpc1, script.TriggerAiOpNpc1},
		{objtype.NPCModeOpNpc5, script.TriggerAiOpNpc5},
		{objtype.NPCModeOpNpc5 + 1, 0},
		{objtype.NPCModeApNpc1, 0},
	}
	runTriggerMapTest(t, "aiOpNpcTriggerForOp", aiOpNpcTriggerForOp, cases)
}

func TestAiApPlayerTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApPlayer1, script.TriggerAiApPlayer1},
		{objtype.NPCModeApPlayer5, script.TriggerAiApPlayer5},
		{objtype.NPCModeApPlayer5 + 1, 0},
		{objtype.NPCModeOpPlayer1, 0},
	}
	runTriggerMapTest(t, "aiApPlayerTriggerForOp", aiApPlayerTriggerForOp, cases)
}

func TestAiOpPlayerTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpPlayer1, script.TriggerAiOpPlayer1},
		{objtype.NPCModeOpPlayer5, script.TriggerAiOpPlayer5},
		{objtype.NPCModeOpPlayer5 + 1, 0},
	}
	runTriggerMapTest(t, "aiOpPlayerTriggerForOp", aiOpPlayerTriggerForOp, cases)
}

func TestAiApLocTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApLoc1, script.TriggerAiApLoc1},
		{objtype.NPCModeApLoc5, script.TriggerAiApLoc5},
		{objtype.NPCModeApLoc5 + 1, 0},
	}
	runTriggerMapTest(t, "aiApLocTriggerForOp", aiApLocTriggerForOp, cases)
}

func TestAiOpLocTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpLoc1, script.TriggerAiOpLoc1},
		{objtype.NPCModeOpLoc5, script.TriggerAiOpLoc5},
		{objtype.NPCModeOpLoc5 + 1, 0},
	}
	runTriggerMapTest(t, "aiOpLocTriggerForOp", aiOpLocTriggerForOp, cases)
}

func TestAiApObjTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApObj1, script.TriggerAiApObj1},
		{objtype.NPCModeApObj5, script.TriggerAiApObj5},
		{objtype.NPCModeApObj5 + 1, 0},
	}
	runTriggerMapTest(t, "aiApObjTriggerForOp", aiApObjTriggerForOp, cases)
}

func TestAiOpObjTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpObj1, script.TriggerAiOpObj1},
		{objtype.NPCModeOpObj5, script.TriggerAiOpObj5},
		{objtype.NPCModeOpObj5 + 1, 0},
	}
	runTriggerMapTest(t, "aiOpObjTriggerForOp", aiOpObjTriggerForOp, cases)
}
```

- [ ] **Step 3: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestAiA[pO]" -v`

Expected: PASS (8 test functions, each with 3-8 sub-tests).

- [ ] **Step 4: Commit**

```bash
git add modules/world/npc_interaction_trigger.go modules/world/npc_interaction_trigger_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add 8 mode→trigger map helpers

Per-category × per-AP/OP helpers mapping NPCMode targetOp → matching
script.ServerTriggerType. Out-of-range returns 0 so callers can
use 'if trigger == 0 { return }' as a validity guard."
```

---

### Task 12: `(*Npc).defaultMode()` + migrate `NewNpc` to call it

**Files:**
- Modify: `modules/world/npc_interaction.go` (add method)
- Modify: `modules/world/npc.go` (use method in `NewNpc`)
- Test: `modules/world/npc_interaction_test.go`

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_interaction_test.go`:

```go
func TestNpcDefaultMode(t *testing.T) {
	tests := []struct {
		name string
		typ  *objtype.NpcType
		want int
	}{
		{"patrol config", &objtype.NpcType{PatrolCoord: []uint32{100}}, objtype.NPCModePatrol},
		{"wander config", &objtype.NpcType{WanderRange: 5}, objtype.NPCModeWander},
		{"neither", &objtype.NpcType{}, objtype.NPCModeNone},
		{"both patrol+wander — patrol wins", &objtype.NpcType{PatrolCoord: []uint32{100}, WanderRange: 5}, objtype.NPCModePatrol},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := &Npc{typ: tc.typ}
			if got := n.defaultMode(); got != tc.want {
				t.Errorf("defaultMode: got %d, want %d", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcDefaultMode -v`

Expected: FAIL — `defaultMode` undefined.

- [ ] **Step 3: Implement `defaultMode`**

Append to `modules/world/npc_interaction.go`:

```go
// defaultMode returns the NPC's baseline mode based on its NpcType config.
// Patrol if PatrolCoord is set; else Wander if WanderRange > 0; else None.
// Used by NewNpc (initial targetOp) and resetDefaults (revert targetOp).
// Matches TS NpcType.defaultmode.
func (n *Npc) defaultMode() int {
	if n.typ == nil {
		return objtype.NPCModeNone
	}
	if len(n.typ.PatrolCoord) > 0 {
		return objtype.NPCModePatrol
	}
	if n.typ.WanderRange > 0 {
		return objtype.NPCModeWander
	}
	return objtype.NPCModeNone
}
```

- [ ] **Step 4: Migrate `NewNpc` to call `defaultMode()`**

In `modules/world/npc.go`, `NewNpc` replaces the inline mode-selection block with a call to `defaultMode`. Since `defaultMode` is a method, it needs an `*Npc` — restructure to construct the NPC, call `defaultMode` on a temp, then set:

```go
func NewNpc(nid, typeId, x, z, level int, typ *objtype.NpcType) *Npc {
	n := &Npc{
		nid:             nid,
		typeId:          typeId,
		baseType:        typeId,
		typ:             typ,
		uid:             (typeId << 16) | nid,
		// ... existing fields unchanged ...
		apRange:        10,
		apRangeCalled:  false,
		targetSubject:  npcTargetSubject{com: -1, typ: -1},
		targetX:        -1,
		targetZ:        -1,
		faceAngleX:     -1,
		faceAngleZ:     -1,
	}
	n.targetOp = n.defaultMode()
	return n
}
```

- [ ] **Step 5: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestNpcDefaultMode|TestNewNpc" -v`

Expected: PASS.

Run full suite: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc.go modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 extract defaultMode; NewNpc uses it

Single source of truth for 'what mode does this NPC start in'.
Same helper will be called by resetDefaults (Task 13)."
```

---

### Task 13: `(*Npc).resetDefaults()` + `(*Npc).clearInteraction()` + tests

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** `resetDefaults` clears target + writes defaultMode. Does NOT touch apRange/apRangeCalled/faceEntity/masks (matches TS). `clearInteraction` is stricter — it DOES clear apRange/apRangeCalled (for use before a fresh `SetInteraction`).

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_interaction_test.go`:

```go
func TestNpcResetDefaultsClearsTargetKeepsOtherState(t *testing.T) {
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = &Npc{nid: 99}
	n.targetOp = objtype.NPCModeOpNpc1
	n.faceEntity = 99
	n.masks = 0xff
	n.apRange = 5
	n.apRangeCalled = true

	n.resetDefaults()

	if n.target != nil {
		t.Error("target: not nil")
	}
	if n.targetOp != objtype.NPCModeWander {
		t.Errorf("targetOp: got %d, want NPCModeWander", n.targetOp)
	}
	// These stay untouched — next SetInteraction call will overwrite.
	if n.faceEntity != 99 {
		t.Errorf("faceEntity: got %d, want 99 (resetDefaults must not clear)", n.faceEntity)
	}
	if n.masks != 0xff {
		t.Errorf("masks: got 0x%x, want 0xff (resetDefaults must not clear)", n.masks)
	}
	if n.apRange != 5 {
		t.Errorf("apRange: got %d, want 5 (resetDefaults must not clear)", n.apRange)
	}
	if n.apRangeCalled != true {
		t.Error("apRangeCalled: not preserved")
	}
}

func TestNpcClearInteractionResetsState(t *testing.T) {
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = &Npc{nid: 99}
	n.targetOp = objtype.NPCModeOpNpc1
	n.apRange = 5
	n.apRangeCalled = true

	n.clearInteraction()

	if n.target != nil {
		t.Error("target: not nil")
	}
	if n.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1", n.targetOp)
	}
	if n.apRange != 10 {
		t.Errorf("apRange: got %d, want 10 (reset to default)", n.apRange)
	}
	if n.apRangeCalled != false {
		t.Error("apRangeCalled: got true, want false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestNpcResetDefaults|TestNpcClearInteraction" -v`

Expected: FAIL.

- [ ] **Step 3: Implement the methods**

Append to `modules/world/npc_interaction.go`:

```go
// resetDefaults clears target/targetOp to defaultMode baseline. Matches
// TS Npc.resetDefaults — INTENTIONALLY does NOT clear apRange,
// apRangeCalled, faceEntity, or masks. Those are overwritten only by
// the next SetInteraction call.
func (n *Npc) resetDefaults() {
	n.target = nil
	n.targetOp = n.defaultMode()
}

// clearInteraction resets interaction state to idle, including apRange
// fields. Matches TS PathingEntity.clearInteraction.
// Does NOT touch faceEntity/masks — those are cleared by the masks
// frame-pass, not here.
func (n *Npc) clearInteraction() {
	n.target = nil
	n.targetOp = -1
	n.apRange = 10
	n.apRangeCalled = false
	n.targetSubject = npcTargetSubject{com: -1, typ: -1}
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestNpcResetDefaults|TestNpcClearInteraction" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add resetDefaults + clearInteraction

resetDefaults clears target/targetOp to defaultMode baseline but
preserves apRange/faceEntity/masks (TS semantics — they're overwritten
by next SetInteraction). clearInteraction is stricter and resets
apRange to 10, apRangeCalled to false."
```

---

### Task 14: `(*Npc).focus()` helper + tests

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 3. Stores fine-grained coords on the NPC. `instant` flag is stored write-only pending wire-protocol follow-up.

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestNpcFocusSetsFaceAngleCoords(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	n.focus(6431, 6431, false)
	if n.faceAngleX != 6431 {
		t.Errorf("faceAngleX: got %d, want 6431", n.faceAngleX)
	}
	if n.faceAngleZ != 6431 {
		t.Errorf("faceAngleZ: got %d, want 6431", n.faceAngleZ)
	}

	// instant flag is write-only — test merely confirms no panic.
	n.focus(1000, 2000, true)
	if n.faceAngleX != 1000 || n.faceAngleZ != 2000 {
		t.Error("focus did not update coords on instant=true call")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcFocus -v`

Expected: FAIL — `focus` undefined.

- [ ] **Step 3: Implement `focus`**

Append to `modules/world/npc_interaction.go`:

```go
// focus records the fine-grained face-angle target. Called from
// SetInteraction with CoordGrid.fine(target.x, target.width),
// CoordGrid.fine(target.z, target.length). The `instant` flag
// distinguishes ENGINE-face from SCRIPT-face; Go's current wire
// protocol doesn't branch on it, so it's stored write-only pending
// a future follow-up.
// Matches TS PathingEntity.focus (Engine-TS/.../PathingEntity.ts, near setInteraction).
func (n *Npc) focus(fx, fz int, instant bool) {
	n.faceAngleX = fx
	n.faceAngleZ = fz
	_ = instant // DEVIATION: write-only pending wire-protocol follow-up.
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcFocus -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.focus helper

Writes fine-grained face-angle target coords. instant flag is stored
write-only pending wire-protocol divergence work."
```

---

### Task 15: `(*Npc).SetInteraction()` + table-driven test

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 3. Closes NAI-10 deferrals #1-7. Single method with internal type-switch on target.

- [ ] **Step 1: Write the failing table-driven test**

Append:

```go
func TestNpcSetInteraction(t *testing.T) {
	// Test fixtures.
	typ := &objtype.NpcType{}
	targetNpc := &Npc{nid: 7, typeId: 99, x: 105, z: 105, level: 0}
	targetPlayer := &Player{slot: 3, online: true, client: &client{}, x: 105, z: 105, level: 0}
	targetLoc := entitypkg.NewLoc(0, 105, 105, entitypkg.LifecycleRespawn, 42, 10, 0, 0)
	targetObj := entitypkg.NewObj(0, 105, 105, entitypkg.LifecycleRespawn, 88, 1)

	type row struct {
		name       string
		target     entity
		kind       InteractionKind
		op         int
		com        int
		wantOK     bool
		wantFace   int // faceEntity; -1 if not applicable
		wantTX     int // targetX; -1 if not applicable
		wantTZ     int
		wantSubCom int
		wantSubTyp int
	}

	rows := []row{
		{
			name: "Player target", target: targetPlayer, kind: InteractionScript,
			op: objtype.NPCModeOpPlayer1, com: -1, wantOK: true,
			wantFace: 3 + 32768, wantTX: -1, wantTZ: -1,
			wantSubCom: -1, wantSubTyp: -1,
		},
		{
			name: "Npc target", target: targetNpc, kind: InteractionScript,
			op: objtype.NPCModeOpNpc1, com: -1, wantOK: true,
			wantFace: 7, wantTX: -1, wantTZ: -1,
			wantSubCom: -1, wantSubTyp: 99,
		},
		{
			name: "Loc target", target: targetLoc, kind: InteractionEngine,
			op: objtype.NPCModeOpLoc1, com: 5, wantOK: true,
			wantFace: -1,
			wantTX:   coordgrid.Fine(105, 1),
			wantTZ:   coordgrid.Fine(105, 1),
			wantSubCom: 5, wantSubTyp: 42,
		},
		{
			name: "Obj target", target: targetObj, kind: InteractionEngine,
			op: objtype.NPCModeOpObj1, com: -1, wantOK: true,
			wantFace: -1,
			wantTX:   coordgrid.Fine(105, 1),
			wantTZ:   coordgrid.Fine(105, 1),
			wantSubCom: -1, wantSubTyp: 88,
		},
		{
			name: "com==0 → subject.com==-1 (TS quirk)", target: targetNpc, kind: InteractionScript,
			op: objtype.NPCModeOpNpc1, com: 0, wantOK: true,
			wantFace: 7, wantTX: -1, wantTZ: -1,
			wantSubCom: -1, wantSubTyp: 99, // com 0 coerces to -1
		},
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			n := NewNpc(1, 42, 100, 100, 0, typ)
			ok := n.SetInteraction(r.kind, r.target, r.op, r.com)
			if ok != r.wantOK {
				t.Errorf("return: got %t, want %t", ok, r.wantOK)
			}
			if n.target != r.target {
				t.Error("target not set")
			}
			if n.targetOp != r.op {
				t.Errorf("targetOp: got %d, want %d", n.targetOp, r.op)
			}
			if n.apRange != 10 {
				t.Errorf("apRange: got %d, want 10", n.apRange)
			}
			if n.apRangeCalled != false {
				t.Error("apRangeCalled: got true, want false")
			}
			if n.targetSubject.com != r.wantSubCom {
				t.Errorf("subject.com: got %d, want %d", n.targetSubject.com, r.wantSubCom)
			}
			if n.targetSubject.typ != r.wantSubTyp {
				t.Errorf("subject.typ: got %d, want %d", n.targetSubject.typ, r.wantSubTyp)
			}
			if r.wantFace != -1 && n.faceEntity != r.wantFace {
				t.Errorf("faceEntity: got %d, want %d", n.faceEntity, r.wantFace)
			}
			if r.wantTX != -1 && n.targetX != r.wantTX {
				t.Errorf("targetX: got %d, want %d", n.targetX, r.wantTX)
			}
			if r.wantTZ != -1 && n.targetZ != r.wantTZ {
				t.Errorf("targetZ: got %d, want %d", n.targetZ, r.wantTZ)
			}
		})
	}
}

func TestNpcSetInteractionTargetInvalidReturnsFalse(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	deadNpc := &Npc{nid: 7, typeId: 99, dead: true}
	originalTarget := n.target

	ok := n.SetInteraction(InteractionScript, deadNpc, objtype.NPCModeOpNpc1, -1)

	if ok != false {
		t.Error("return: got true, want false")
	}
	if n.target != originalTarget {
		t.Error("target changed despite IsValid()==false")
	}
	if n.targetOp != n.defaultMode() { // stays at NewNpc's default
		t.Errorf("targetOp changed: got %d, want %d", n.targetOp, n.defaultMode())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcSetInteraction -v`

Expected: FAIL — `SetInteraction` undefined.

- [ ] **Step 3: Implement `SetInteraction`**

Append to `modules/world/npc_interaction.go`:

```go
// nonPathingEntity marks targets that are NOT PathingEntity (Player/Npc).
// Implemented by *entitypkg.Loc and *entitypkg.Obj. Used by
// SetInteraction to pass the correct `instant` flag into focus().
type nonPathingEntity interface {
	nonPathing()
}

// SetInteraction anchors the NPC's interaction on target. Mirrors TS
// PathingEntity.setInteraction at PathingEntity.ts:510-548. Closes
// NAI-10's seven deferred setInteraction fields:
//   1. apRange = 10
//   2. apRangeCalled = false
//   3. targetSubject.com/typ
//   4. focus() → faceAngleX/Z
//   5. faceEntity + masks |= entitymask (for Player/Npc targets)
//   6. targetX/targetZ (for Loc/Obj targets)
//   7. target.IsValid() pre-check
//
// TS quirk preserved: `com ? com : -1` coerces 0 → -1.
//
// DEVIATION: n.entitymask is never written by the current codebase
// (field initialises to 0), so `n.masks |= n.entitymask` is a harmless
// no-op. Mask plumbing wires entitymask in a later sub-spec.
func (n *Npc) SetInteraction(kind InteractionKind, target entity, op, com int) bool {
	if !target.IsValid() {
		return false
	}

	n.target = target
	n.targetOp = op
	n.apRange = 10
	n.apRangeCalled = false

	// TS "com ? com : -1": 0 coerces to -1.
	if com == 0 {
		n.targetSubject.com = -1
	} else {
		n.targetSubject.com = com
	}

	// targetSubject.typ snapshot for changetype-detection in validateTarget.
	switch t := target.(type) {
	case *Npc:
		n.targetSubject.typ = t.typeId
	case *entitypkg.Loc:
		n.targetSubject.typ = t.Type()
	case *entitypkg.Obj:
		n.targetSubject.typ = t.Type
	default:
		n.targetSubject.typ = -1
	}

	// focus — fine-grained face-angle coord.
	tx, tz, _ := target.Coords()
	tw, tl := targetWidthLength(target)
	fx := coordgrid.Fine(tx, tw)
	fz := coordgrid.Fine(tz, tl)
	_, isNonPathing := target.(nonPathingEntity)
	n.focus(fx, fz, isNonPathing && kind == InteractionEngine)

	// faceEntity / targetX-Z dispatch.
	switch t := target.(type) {
	case *Player:
		slot := t.slot + 32768
		if n.faceEntity != slot {
			n.faceEntity = slot
			n.masks |= n.entitymask
		}
	case *Npc:
		if n.faceEntity != t.nid {
			n.faceEntity = t.nid
			n.masks |= n.entitymask
		}
	default:
		n.targetX = coordgrid.Fine(tx, tw)
		n.targetZ = coordgrid.Fine(tz, tl)
	}

	return true
}

// targetWidthLength returns the target's (width, length) for fine-grained
// coord math. 1x1 for PathingEntity; LocType-derived for Loc; 1x1 for Obj.
func targetWidthLength(target entity) (width, length int) {
	switch t := target.(type) {
	case *entitypkg.Loc:
		// Loc has .Width / .Length on embedded entity.Entity.
		return t.Width, t.Length
	default:
		return 1, 1
	}
}
```

Additionally, make `*Loc` and `*Obj` satisfy `nonPathingEntity`. Append to `pkg/entity/loc.go`:

```go
// nonPathing marks Loc as a non-pathing entity for the world-module
// interaction code. Empty-body marker method — the package layering
// keeps modules/world from adding methods to pkg/entity types.
func (l *Loc) NonPathing() {}
```

Wait: the marker method must be named such that pkg/entity is the DEFINER and modules/world's interface consumes it. The `nonPathing()` interface method in modules/world is unexported, so types in OTHER packages can't implement it.

**Resolution:** define the marker interface with an exported method name that modules/world can call on pkg/entity types. Or: do the nonPathing check via type-assertion directly (`_, ok := target.(*entitypkg.Loc); if ok || isObj { ... }`).

Simplest: replace the `nonPathingEntity` type-switch with direct concrete type checks:

```go
func (n *Npc) SetInteraction(...) bool {
	// ...
	isNonPathing := false
	switch target.(type) {
	case *entitypkg.Loc, *entitypkg.Obj:
		isNonPathing = true
	}
	n.focus(fx, fz, isNonPathing && kind == InteractionEngine)
	// ...
}
```

This is more direct. Remove the `nonPathingEntity` interface entirely.

- [ ] **Step 4: Rewrite SetInteraction with direct type-switch for nonPathing**

Replace the SetInteraction body in `npc_interaction.go` with the simplified version (no `nonPathingEntity` interface). Remove the interface definition.

- [ ] **Step 5: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcSetInteraction -v`

Expected: PASS (5 row tests + invalid-target test).

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.SetInteraction — closes NAI-10 deferrals

Full port of TS PathingEntity.setInteraction. Writes apRange=10,
apRangeCalled=false, targetSubject.com/typ, faceAngleX/Z via focus(),
faceEntity+masks for Player/Npc targets, targetX/Z for Loc/Obj.
IsValid() pre-check short-circuits on stale targets. TS com==0→-1
quirk preserved. n.entitymask remains 0 until mask-plumbing sub-spec
wires it — the OR is harmless meanwhile (tracked deviation)."
```

---

### Task 16: `(*Npc).inOperableDistance()` + `(*Npc).inApproachDistance()` + tests

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 4. Range-only checks (LoS + reach-helpers deferred). Mirror player-side `inOperableDistance`/`inApproachDistance` shapes.

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestNpcInOperableDistance(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	tests := []struct {
		name   string
		tx, tz int
		want   bool
	}{
		{"same tile", 100, 100, false},
		{"adjacent N", 100, 101, true},
		{"adjacent NE (diagonal)", 101, 101, true},
		{"two tiles away", 102, 100, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := &Npc{x: tc.tx, z: tc.tz, level: 0}
			if got := n.inOperableDistance(target); got != tc.want {
				t.Errorf("got %t, want %t", got, tc.want)
			}
		})
	}

	t.Run("different level", func(t *testing.T) {
		target := &Npc{x: 101, z: 100, level: 1}
		if n.inOperableDistance(target) {
			t.Error("different level should return false")
		}
	})
}

func TestNpcInApproachDistance(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	tests := []struct {
		name   string
		rng    int
		tx, tz int
		want   bool
	}{
		{"range 5, at 103", 5, 103, 100, true},
		{"range 5, at 106", 5, 106, 100, false},
		{"range 0 — always false", 0, 101, 100, false},
		{"same tile — false", 5, 100, 100, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := &Npc{x: tc.tx, z: tc.tz, level: 0}
			if got := n.inApproachDistance(tc.rng, target); got != tc.want {
				t.Errorf("got %t, want %t", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestNpcInOperableDistance|TestNpcInApproachDistance" -v`

Expected: FAIL.

- [ ] **Step 3: Implement both methods**

Append to `modules/world/npc_interaction.go`:

```go
// inOperableDistance checks whether target is in contact range
// (Chebyshev <= 1, excluding same tile). Mirrors player-side
// inOperableDistance at interaction.go:128-141.
//
// DEVIATION from TS (PathingEntity.ts:378-389): does not dispatch to
// reachedEntity / reachedLoc / reachedObj — uses Chebyshev for all
// target types. Loc shape/angle/forceapproach and Obj width/length
// reach logic is deferred; inherits player-side's posture. Tracked
// follow-up.
func (n *Npc) inOperableDistance(target entity) bool {
	tx, tz, tlevel := target.Coords()
	if tlevel != n.level {
		return false
	}
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > 1 || dz > 1 {
		return false
	}
	return !(dx == 0 && dz == 0)
}

// inApproachDistance checks whether target is within range tiles
// (Chebyshev, excluding same tile). Mirrors player-side at
// interaction.go:148-164.
//
// DEVIATION from TS (PathingEntity.ts:392-406): no LoS gating. TS's
// isApproached walks the collision map; NAI-11 inherits player-side's
// S6l-D4 no-LoS posture. Tracked follow-up.
func (n *Npc) inApproachDistance(rng int, target entity) bool {
	if rng <= 0 {
		return false
	}
	tx, tz, tlevel := target.Coords()
	if tlevel != n.level {
		return false
	}
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > rng || dz > rng {
		return false
	}
	return !(dx == 0 && dz == 0)
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestNpcInOperableDistance|TestNpcInApproachDistance" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.inOperableDistance / inApproachDistance

Chebyshev range checks for NPC interaction. Mirrors player-side shape
(interaction.go:128-164). LoS gating + reachedLoc/reachedObj helpers
are tracked deviations, matching player-side posture."
```

---

### Task 17: `(*Npc).targetWithinMaxRange()` + table-driven test

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 2. Three branches (OP / AP / default); PLAYERESCAPE branch dropped per scope.

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestNpcTargetWithinMaxRange(t *testing.T) {
	// Npc at origin (start 100,100) with maxrange=5, attackrange=2.
	typ := &objtype.NpcType{MaxRange: 5, AttackRange: 2}

	tests := []struct {
		name     string
		targetOp int
		tx, tz   int
		want     bool
	}{
		// OP branch (maxrange+1 = 6, with corner-removal)
		{"OP within +1", objtype.NPCModeOpNpc1, 106, 100, true},
		{"OP at +2", objtype.NPCModeOpNpc1, 107, 100, false},
		{"OP corner at (+1,+1)", objtype.NPCModeOpNpc1, 106, 106, false}, // corner removal
		{"OP non-corner edge (+1,0)", objtype.NPCModeOpNpc1, 106, 100, true},

		// AP branch (maxrange + attackrange = 7)
		{"AP at +7", objtype.NPCModeApNpc1, 107, 100, true},
		{"AP at +8", objtype.NPCModeApNpc1, 108, 100, false},

		// Default branch (targetless targeted mode, e.g. PLAYERFOLLOW — maxrange+1)
		{"Default at +6", objtype.NPCModePlayerFollow, 106, 100, true},
		{"Default at +7", objtype.NPCModePlayerFollow, 107, 100, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := NewNpc(1, 42, 100, 100, 0, typ)
			n.targetOp = tc.targetOp
			n.target = &Npc{x: tc.tx, z: tc.tz, level: 0}
			if got := n.targetWithinMaxRange(); got != tc.want {
				t.Errorf("got %t, want %t", got, tc.want)
			}
		})
	}

	t.Run("nil target returns true", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.target = nil
		if !n.targetWithinMaxRange() {
			t.Error("nil target: got false, want true")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcTargetWithinMaxRange -v`

Expected: FAIL.

- [ ] **Step 3: Implement `targetWithinMaxRange`**

Append to `modules/world/npc_interaction.go`:

```go
// targetWithinMaxRange enforces the per-mode maxrange rules on n.target.
// Three branches: OP (maxrange+1 with corner-removal quirk), AP
// (maxrange + attackrange), default (maxrange+1 SW-distance).
// Matches TS Npc.targetWithinMaxRange at Npc.ts:629-680.
//
// DEVIATION: PLAYERESCAPE branch (TS :657-673) dropped with the other
// PLAYER* modes. Tracked follow-up.
func (n *Npc) targetWithinMaxRange() bool {
	if n.target == nil {
		return true
	}
	if n.typ == nil {
		return false
	}
	maxrng := int(n.typ.MaxRange)
	attackrng := int(n.typ.AttackRange)

	tx, tz, _ := n.target.Coords()
	dx := tx - n.startX
	if dx < 0 {
		dx = -dx
	}
	dz := tz - n.startZ
	if dz < 0 {
		dz = -dz
	}

	switch {
	case checkOpTrigger(n.targetOp):
		// TS :640-648 — maxrange+1 with corner-removal quirk.
		maxAxis := dx
		if dz > maxAxis {
			maxAxis = dz
		}
		if maxAxis > maxrng+1 {
			return false
		}
		if dx == maxrng+1 && dz == maxrng+1 {
			return false
		}
		return true

	case checkApTrigger(n.targetOp):
		// TS :651-654 — SW-distance up to maxrange + attackrange.
		d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
		return d <= maxrng+attackrng

	default:
		// TS :676 — SW-distance up to maxrange + 1.
		d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
		return d <= maxrng+1
	}
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcTargetWithinMaxRange -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.targetWithinMaxRange

Three-branch range gate: OP uses maxrange+1 with corner-removal quirk;
AP uses maxrange + attackrange SW-distance; default uses maxrange+1
SW-distance. PLAYERESCAPE branch dropped (deferred). Mirrors TS
Npc.ts:629-680 minus the deferred branch."
```

---

### Task 18: `(*Npc).validateTarget()` + tests

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 2. Four gates: level, maxrange, type-changed, concrete lifecycle.

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestNpcValidateTarget(t *testing.T) {
	typ := &objtype.NpcType{MaxRange: 10, AttackRange: 2}

	t.Run("different level", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		n.target = &Npc{x: 101, z: 100, level: 1}
		n.targetSubject.typ = n.target.(*Npc).typeId
		if n.validateTarget() {
			t.Error("different level: got true, want false")
		}
	})

	t.Run("out of maxrange", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		n.target = &Npc{x: 200, z: 100, level: 0}
		n.targetSubject.typ = n.target.(*Npc).typeId
		if n.validateTarget() {
			t.Error("far target: got true, want false")
		}
	})

	t.Run("npc typeId changed (changetype mid-interaction)", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		target := &Npc{nid: 7, typeId: 99, x: 105, z: 100, level: 0}
		n.target = target
		n.targetSubject.typ = 99

		// Simulate changetype.
		target.typeId = 100

		if n.validateTarget() {
			t.Error("changetyped target: got true, want false")
		}
	})

	t.Run("dead npc target", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		target := &Npc{nid: 7, typeId: 99, x: 105, z: 100, level: 0, dead: true}
		n.target = target
		n.targetSubject.typ = 99
		if n.validateTarget() {
			t.Error("dead target: got true, want false")
		}
	})

	t.Run("delayed npc target", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		target := &Npc{nid: 7, typeId: 99, x: 105, z: 100, level: 0, delayed: true}
		n.target = target
		n.targetSubject.typ = 99
		if n.validateTarget() {
			t.Error("delayed target: got true, want false (TS isActive = !dead && !delayed)")
		}
	})

	t.Run("valid npc target", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		target := &Npc{nid: 7, typeId: 99, x: 105, z: 100, level: 0}
		n.target = target
		n.targetSubject.typ = 99
		if !n.validateTarget() {
			t.Error("valid target: got false, want true")
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcValidateTarget -v`

Expected: FAIL.

- [ ] **Step 3: Implement `validateTarget`**

Append to `modules/world/npc_interaction.go`:

```go
// validateTarget enforces per-tick target validity. Four gates matching
// TS Npc.validateTarget at Npc.ts:606-627:
//  1. Same-level.
//  2. targetWithinMaxRange (per-mode maxrange math).
//  3. targetSubject.typ equality (catches changetype mid-interaction)
//     for Npc/Loc targets.
//  4. Concrete lifecycle: *Npc → isActive (!dead && !delayed);
//     *Loc/*Obj → intrinsic + zone-membership; *Player → IsValid.
func (n *Npc) validateTarget() bool {
	if n.target == nil {
		return false
	}

	// Gate 1: level.
	_, _, tlevel := n.target.Coords()
	if tlevel != n.level {
		return false
	}

	// Gate 2: maxrange.
	if !n.targetWithinMaxRange() {
		return false
	}

	// Gate 3: type-changed check for Npc/Loc (TS :618).
	switch t := n.target.(type) {
	case *Npc:
		if n.targetSubject.typ != t.typeId {
			return false
		}
	case *entitypkg.Loc:
		if n.targetSubject.typ != t.Type() {
			return false
		}
	}

	// Gate 4: concrete lifecycle.
	switch t := n.target.(type) {
	case *Npc:
		// TS Npc.isActive = !dead && !delayed. See Npc.ts:623-625.
		return !t.dead && !t.delayed
	case *entitypkg.Loc:
		tx, tz, lvl := t.Coords()
		return t.IsValid() && locStillValid(n.server, t, n.targetSubject.typ, tx, tz, lvl)
	case *entitypkg.Obj:
		tx, tz, lvl := t.Coords()
		return t.IsValid() && objStillValid(n.server, t, tx, tz, lvl)
	case *Player:
		return t.IsValid()
	default:
		return n.target.IsValid()
	}
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcValidateTarget -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.validateTarget

Four-gate validity check: level, maxrange, type-changed (Npc/Loc),
concrete lifecycle (Npc isActive, Loc/Obj zone-membership, Player
IsValid). Matches TS Npc.validateTarget at Npc.ts:606-627."
```

---

### Task 19: `(*Npc).pathToTarget()` naive-only + tests

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 2. Naive-only pathing (Q3). SMART branch deferred with tracked deviation.

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestNpcPathToTarget(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = &Npc{x: 105, z: 108, level: 0}

	n.pathToTarget()

	if n.waypointIndex < 0 {
		t.Fatal("waypointIndex: got < 0, want >= 0 after path set")
	}
	gotPacked := n.waypoints[n.waypointIndex]
	got := coordgrid.UnpackCoord(gotPacked)
	if got.X != 105 || got.Z != 108 {
		t.Errorf("waypoint: got (%d,%d), want (105,108)", got.X, got.Z)
	}
}

func TestNpcPathToTargetNilTargetNoOp(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = nil

	n.pathToTarget()

	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (no-op)", n.waypointIndex)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcPathToTarget -v`

Expected: FAIL.

- [ ] **Step 3: Implement `pathToTarget`**

Append to `modules/world/npc_interaction.go`:

```go
// pathToTarget queues a single waypoint at the target's current tile.
// Naive-only port — TS's pathToTarget at PathingEntity.ts:457-508 has a
// full SMART branch using findPath/findPathToEntity/findPathToLoc,
// which is deferred. Tracked follow-up.
func (n *Npc) pathToTarget() {
	if n.target == nil {
		return
	}
	tx, tz, _ := n.target.Coords()
	queueWaypoint(n, tx, tz)
}
```

If `queueWaypoint` (existing in `npc_ai.go`) takes `(n *Npc, x, z int)`, use it. If it's a method on `*Npc`, call `n.queueWaypoint(tx, tz)` instead. Verify by reading existing `npc_ai.go`.

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcPathToTarget -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.pathToTarget (naive-only)

Queues a single waypoint at target's tile. SMART branch deferred —
tracked deviation. Full route-finder port is a separate sub-spec."
```

---

### Task 20: `(*Npc).updateMovement()` walk + run + tests

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 2. Walk = 1 step; Run = 2 steps. Consumes waypoints. Returns `moved bool`.

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestNpcUpdateMovementWalk(t *testing.T) {
	s := newServerForNpcTest(t) // existing helper from npc_ai_test.go
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if n.x != 101 {
		t.Errorf("x: got %d, want 101 (one step east)", n.x)
	}
	if n.walkDir < 0 {
		t.Errorf("walkDir: got %d, want set", n.walkDir)
	}
	if n.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (walk mode)", n.runDir)
	}
}

func TestNpcUpdateMovementRunConsumesTwoSteps(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedRun
	n.waypoints[0] = coordgrid.PackCoord(0, 105, 100)
	n.waypointIndex = 0

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if n.x != 102 {
		t.Errorf("x: got %d, want 102 (two steps east in run mode)", n.x)
	}
	if n.walkDir < 0 {
		t.Error("walkDir: not set")
	}
	if n.runDir < 0 {
		t.Error("runDir: not set (run mode with multi-step waypoint)")
	}
}

func TestNpcUpdateMovementRunWithOneWaypointStep(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedRun
	n.waypoints[0] = coordgrid.PackCoord(0, 101, 100) // ONE tile away — arrives after 1 step
	n.waypointIndex = 0

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if n.x != 101 {
		t.Errorf("x: got %d, want 101", n.x)
	}
	if n.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (no second step available)", n.runDir)
	}
}

func TestNpcUpdateMovementNoMoveRestrict(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveRestrict = MoveRestrictNoMove
	n.waypoints[0] = coordgrid.PackCoord(0, 105, 100)
	n.waypointIndex = 0

	moved := n.updateMovement(s)

	if moved {
		t.Error("moved: true, want false (NoMove restrict)")
	}
	if n.x != 100 {
		t.Errorf("x: got %d, want 100 (no step)", n.x)
	}
	if n.walkDir != -1 || n.runDir != -1 {
		t.Errorf("dirs: walkDir=%d runDir=%d, want both -1", n.walkDir, n.runDir)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcUpdateMovement -v`

Expected: FAIL.

- [ ] **Step 3: Implement `updateMovement`**

Append to `modules/world/npc_interaction.go`:

```go
// updateMovement consumes up to 1 waypoint step (walk) or 2 (run) per
// tick. Returns true if the NPC moved. Writes walkDir (step 1) and
// runDir (step 2 when running). Replaces npc_ai.go:advanceWaypoint
// (migrated into this method).
func (n *Npc) updateMovement(s *Server) bool {
	if n.moveRestrict == MoveRestrictNoMove {
		n.walkDir = -1
		n.runDir = -1
		return false
	}
	if n.waypointIndex < 0 {
		n.walkDir = -1
		n.runDir = -1
		return false
	}

	advanced1, dir1 := n.stepOnce(s)
	if !advanced1 {
		n.walkDir = -1
		n.runDir = -1
		return false
	}
	n.walkDir = dir1

	if n.moveSpeed == MoveSpeedRun && n.waypointIndex >= 0 {
		advanced2, dir2 := n.stepOnce(s)
		if advanced2 {
			n.runDir = dir2
		} else {
			n.runDir = -1
		}
	} else {
		n.runDir = -1
	}
	return true
}

// stepOnce walks one tile toward the current waypoint. Returns
// (advanced, dir). Extracted from old advanceWaypoint logic at
// npc_ai.go:145-175. Pops the waypoint index when arrived.
func (n *Npc) stepOnce(s *Server) (bool, int) {
	if n.waypointIndex < 0 {
		return false, -1
	}
	dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
	dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
	if dir == -1 {
		n.waypointIndex--
		return false, -1
	}
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz) {
		n.waypointIndex = -1
		return false, -1
	}
	n.x += dx
	n.z += dz
	n.stepsTaken++
	if n.x == dest.X && n.z == dest.Z {
		n.waypointIndex--
	}
	return true, int(dir)
}
```

Verify `MoveSpeedWalk` and `MoveSpeedRun` constants exist in `modules/world/movement_consts.go`. If `MoveSpeedRun` is missing, add it alongside `MoveSpeedWalk`/`MoveSpeedInstant`:

```go
const (
	MoveSpeedInstant MoveSpeed = iota
	MoveSpeedWalk
	MoveSpeedRun
)
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcUpdateMovement -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go modules/world/movement_consts.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.updateMovement (walk + run)

Walk consumes 1 waypoint step; run consumes 2. Returns moved bool
for aiMode's givechase clause. Extracts stepOnce from the old
advanceWaypoint logic. MoveSpeedRun constant added if missing."
```

---

## Phase 3 — Fire helpers (AP/OP dispatch by target category)

### Task 21: Player-target fire helpers (AP + OP) + tests

**Files:**
- Modify: `modules/world/npc_interaction_trigger.go`
- Test: `modules/world/npc_interaction_trigger_test.go`

**Context:** Spec Section 4. Player-target fire helpers. Each helper: guards lifecycle → looks up trigger → fires via `runNpcScript`.

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_interaction_trigger_test.go`:

```go
func TestFireAiOpTriggerPlayerHappy(t *testing.T) {
	s, n, p, sf := setupPlayerFireTest(t,
		script.TriggerAiOpPlayer1, objtype.NPCModeOpPlayer1)
	_ = sf

	n.target = p
	n.targetOp = objtype.NPCModeOpPlayer1
	n.fireAiOpTriggerPlayer(s, p)

	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

func TestFireAiOpTriggerPlayerNoScript(t *testing.T) {
	s, n, p, _ := setupPlayerFireTest(t, 0, objtype.NPCModeOpPlayer1)
	n.target = p
	n.targetOp = objtype.NPCModeOpPlayer1

	n.fireAiOpTriggerPlayer(s, p)

	if n.target != nil {
		t.Error("clearInteraction not called on no-script")
	}
}

func TestFireAiOpTriggerPlayerLifecycleInvalid(t *testing.T) {
	s, n, p, _ := setupPlayerFireTest(t, script.TriggerAiOpPlayer1, objtype.NPCModeOpPlayer1)
	n.target = p
	n.targetOp = objtype.NPCModeOpPlayer1
	p.online = false // invalid

	n.fireAiOpTriggerPlayer(s, p)

	if n.target != nil {
		t.Error("clearInteraction not called on lifecycle-invalid")
	}
}

func TestFireAiApTriggerPlayerHappy(t *testing.T) {
	s, n, p, sf := setupPlayerFireTest(t, script.TriggerAiApPlayer1, objtype.NPCModeApPlayer1)
	n.target = p
	n.targetOp = objtype.NPCModeApPlayer1

	n.fireAiApTriggerPlayer(s, p)

	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

// setupPlayerFireTest builds a Server + Npc + Player + registered script
// file for the given trigger; returns (s, n, p, sf). If trigger==0,
// no script is registered (no-script-found path).
func setupPlayerFireTest(t *testing.T, trigger script.ServerTriggerType, targetOp int) (*Server, *Npc, *Player, *capturingScriptFile) {
	// Existing helpers; adapt if your fixtures differ.
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t, 42)
	n.server = s
	n.x, n.z, n.level = 100, 100, 0

	p := &Player{slot: 3, online: true, client: &client{server: s}, x: 101, z: 100, level: 0}
	// Register the player in the slot so faceEntity slot-math lines up.
	if s.players == nil {
		s.players = make([]*Player, 16)
	}
	s.players[3] = p

	var sf *capturingScriptFile
	if trigger != 0 {
		sf = newCapturingScriptFile(t, trigger, n.typeId, -1)
		s.scriptProvider.Register(sf.scriptFile)
	}
	return s, n, p, sf
}
```

Adapt `newCapturingScriptFile` + `capturingScriptFile.wasRun()` if those helpers don't exist. Write minimal versions mirroring existing `newNoopScriptFile` pattern at `modules/world/interaction_trigger_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireAi(Op|Ap)TriggerPlayer" -v`

Expected: FAIL.

- [ ] **Step 3: Implement both fire helpers**

Append to `modules/world/npc_interaction_trigger.go`:

```go
// fireAiOpTriggerPlayer fires AI_OPPLAYER1..5 for a Player target.
// Called by tryInteract when targetOp is OPPLAYER + in operable range.
// Mirrors TS Npc.tryInteract → executeScript branch.
func (n *Npc) fireAiOpTriggerPlayer(s *Server, target *Player) {
	if !target.IsValid() {
		n.clearInteraction()
		return
	}
	trigger := aiOpPlayerTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	sf := s.scriptProvider.GetByTrigger(trigger, 0, 0) // Player has no typeId/category
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil)
}

// fireAiApTriggerPlayer: same as Op variant but for AP range.
func (n *Npc) fireAiApTriggerPlayer(s *Server, target *Player) {
	if !target.IsValid() {
		n.clearInteraction()
		return
	}
	trigger := aiApPlayerTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	sf := s.scriptProvider.GetByTrigger(trigger, 0, 0)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil)
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireAi(Op|Ap)TriggerPlayer" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction_trigger.go modules/world/npc_interaction_trigger_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Player-target AP/OP fire helpers

fireAiOpTriggerPlayer + fireAiApTriggerPlayer. Lifecycle gate via
target.IsValid(); trigger lookup via aiOp/ApPlayerTriggerForOp.
Dispatches through runNpcScript. Clear-interaction on invalid target
or no-script-found."
```

---

### Task 22: NPC-target fire helpers (AP + OP) + tests

**Files:**
- Modify: `modules/world/npc_interaction_trigger.go`
- Test: `modules/world/npc_interaction_trigger_test.go`

**Context:** Same shape as Task 21 but for NPC targets. Lifecycle gate is `target.dead`. Category read from `target.typ.Category`.

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestFireAiOpTriggerNpcHappy(t *testing.T) {
	s, n, target, sf := setupNpcFireTest(t, script.TriggerAiOpNpc1, objtype.NPCModeOpNpc1)
	n.target = target
	n.targetOp = objtype.NPCModeOpNpc1

	n.fireAiOpTriggerNpc(s, target)

	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

func TestFireAiOpTriggerNpcNoScript(t *testing.T) {
	s, n, target, _ := setupNpcFireTest(t, 0, objtype.NPCModeOpNpc1)
	n.target = target
	n.targetOp = objtype.NPCModeOpNpc1

	n.fireAiOpTriggerNpc(s, target)

	if n.target != nil {
		t.Error("clearInteraction not called")
	}
}

func TestFireAiOpTriggerNpcLifecycleInvalid(t *testing.T) {
	s, n, target, _ := setupNpcFireTest(t, script.TriggerAiOpNpc1, objtype.NPCModeOpNpc1)
	n.target = target
	n.targetOp = objtype.NPCModeOpNpc1
	target.dead = true

	n.fireAiOpTriggerNpc(s, target)

	if n.target != nil {
		t.Error("clearInteraction not called on dead target")
	}
}

func TestFireAiApTriggerNpcHappy(t *testing.T) {
	s, n, target, sf := setupNpcFireTest(t, script.TriggerAiApNpc1, objtype.NPCModeApNpc1)
	n.target = target
	n.targetOp = objtype.NPCModeApNpc1

	n.fireAiApTriggerNpc(s, target)

	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

func setupNpcFireTest(t *testing.T, trigger script.ServerTriggerType, targetOp int) (*Server, *Npc, *Npc, *capturingScriptFile) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t, 42)
	n.server = s
	n.x, n.z, n.level = 100, 100, 0
	target := newNpcForLifecycleTest(t, 99)
	target.server = s
	target.x, target.z, target.level = 101, 100, 0

	var sf *capturingScriptFile
	if trigger != 0 {
		sf = newCapturingScriptFile(t, trigger, target.typeId, -1)
		s.scriptProvider.Register(sf.scriptFile)
	}
	return s, n, target, sf
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireAi(Op|Ap)TriggerNpc" -v`

Expected: FAIL.

- [ ] **Step 3: Implement both fire helpers**

Append to `modules/world/npc_interaction_trigger.go`:

```go
// fireAiOpTriggerNpc fires AI_OPNPC1..5 for an NPC target.
func (n *Npc) fireAiOpTriggerNpc(s *Server, target *Npc) {
	if target.dead {
		n.clearInteraction()
		return
	}
	trigger := aiOpNpcTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	category := 0
	if target.typ != nil {
		category = target.typ.Category
	}
	sf := s.scriptProvider.GetByTrigger(trigger, target.typeId, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil)
}

// fireAiApTriggerNpc: same as Op variant but for AP range.
func (n *Npc) fireAiApTriggerNpc(s *Server, target *Npc) {
	if target.dead {
		n.clearInteraction()
		return
	}
	trigger := aiApNpcTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	category := 0
	if target.typ != nil {
		category = target.typ.Category
	}
	sf := s.scriptProvider.GetByTrigger(trigger, target.typeId, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil)
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireAi(Op|Ap)TriggerNpc" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction_trigger.go modules/world/npc_interaction_trigger_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc-target AP/OP fire helpers

fireAiOpTriggerNpc + fireAiApTriggerNpc. Lifecycle via target.dead.
Category read from target.typ.Category. Same dispatch shape as
Player variant."
```

---

### Task 23: Loc-target fire helpers (AP + OP) + tests

**Files:**
- Modify: `modules/world/npc_interaction_trigger.go`
- Test: `modules/world/npc_interaction_trigger_test.go`

**Context:** Lifecycle via `locStillValid`. Category read from `locTypes.Configs[loc.Type()]`.

- [ ] **Step 1: Write the failing tests**

Append (condensed for brevity — same 4-case shape as Player/Npc):

```go
func TestFireAiOpTriggerLocHappy(t *testing.T) {
	s, n, loc, sf := setupLocFireTest(t, script.TriggerAiOpLoc1, objtype.NPCModeOpLoc1)
	n.target = loc
	n.targetOp = objtype.NPCModeOpLoc1
	n.targetSubject.typ = loc.Type()
	n.targetSubject.com = -1

	n.fireAiOpTriggerLoc(s, loc)

	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

func TestFireAiOpTriggerLocNoScript(t *testing.T) {
	s, n, loc, _ := setupLocFireTest(t, 0, objtype.NPCModeOpLoc1)
	n.target = loc
	n.targetOp = objtype.NPCModeOpLoc1
	n.targetSubject.typ = loc.Type()

	n.fireAiOpTriggerLoc(s, loc)

	if n.target != nil {
		t.Error("clearInteraction not called")
	}
}

func TestFireAiOpTriggerLocLifecycleInvalid(t *testing.T) {
	s, n, loc, _ := setupLocFireTest(t, script.TriggerAiOpLoc1, objtype.NPCModeOpLoc1)
	n.target = loc
	n.targetOp = objtype.NPCModeOpLoc1
	n.targetSubject.typ = loc.Type()

	// Remove loc from zone to invalidate.
	zn := s.zoneMap.Get(0, loc.X, loc.Z)
	zn.Locs = nil

	n.fireAiOpTriggerLoc(s, loc)

	if n.target != nil {
		t.Error("clearInteraction not called on zone-stale loc")
	}
}

func TestFireAiApTriggerLocHappy(t *testing.T) {
	s, n, loc, sf := setupLocFireTest(t, script.TriggerAiApLoc1, objtype.NPCModeApLoc1)
	n.target = loc
	n.targetOp = objtype.NPCModeApLoc1
	n.targetSubject.typ = loc.Type()

	n.fireAiApTriggerLoc(s, loc)

	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

func setupLocFireTest(t *testing.T, trigger script.ServerTriggerType, targetOp int) (*Server, *Npc, *entitypkg.Loc, *capturingScriptFile) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t, 42)
	n.server = s
	n.x, n.z, n.level = 100, 100, 0
	loc := entitypkg.NewLoc(0, 101, 100, entitypkg.LifecycleRespawn, 77, 10, 0, 0)

	// Place loc in zone map.
	zn := s.zoneMap.Get(0, 101, 100)
	zn.Locs = append(zn.Locs, loc)

	var sf *capturingScriptFile
	if trigger != 0 {
		sf = newCapturingScriptFile(t, trigger, 77, -1)
		s.scriptProvider.Register(sf.scriptFile)
	}
	return s, n, loc, sf
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireAi(Op|Ap)TriggerLoc" -v`

Expected: FAIL.

- [ ] **Step 3: Implement both fire helpers**

Append:

```go
// fireAiOpTriggerLoc fires AI_OPLOC1..5 for a Loc target.
func (n *Npc) fireAiOpTriggerLoc(s *Server, target *entitypkg.Loc) {
	tx, tz, tlevel := target.Coords()
	if !locStillValid(s, target, n.targetSubject.typ, tx, tz, tlevel) {
		n.clearInteraction()
		return
	}
	trigger := aiOpLocTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	category := 0
	locId := target.Type()
	if locId >= 0 && locId < len(s.locTypes.Configs) {
		if lt := s.locTypes.Configs[locId]; lt != nil {
			category = lt.Category
		}
	}
	sf := s.scriptProvider.GetByTrigger(trigger, locId, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil)
}

// fireAiApTriggerLoc: same as Op variant but for AP range.
func (n *Npc) fireAiApTriggerLoc(s *Server, target *entitypkg.Loc) {
	tx, tz, tlevel := target.Coords()
	if !locStillValid(s, target, n.targetSubject.typ, tx, tz, tlevel) {
		n.clearInteraction()
		return
	}
	trigger := aiApLocTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	category := 0
	locId := target.Type()
	if locId >= 0 && locId < len(s.locTypes.Configs) {
		if lt := s.locTypes.Configs[locId]; lt != nil {
			category = lt.Category
		}
	}
	sf := s.scriptProvider.GetByTrigger(trigger, locId, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil)
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireAi(Op|Ap)TriggerLoc" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction_trigger.go modules/world/npc_interaction_trigger_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Loc-target AP/OP fire helpers

fireAiOpTriggerLoc + fireAiApTriggerLoc. Lifecycle via locStillValid
(zone-membership + type). Category via locTypes.Configs lookup."
```

---

### Task 24: Obj-target fire helpers (AP + OP) + tests

**Files:**
- Modify: `modules/world/npc_interaction_trigger.go`
- Test: `modules/world/npc_interaction_trigger_test.go`

**Context:** Lifecycle via `objStillValid`. Category lookup via `objTypes.Configs[obj.Type]` (if ObjType registry exists; else default to 0).

- [ ] **Step 1: Write the failing tests**

Append (same shape as Loc fire tests):

```go
func TestFireAiOpTriggerObjHappy(t *testing.T) {
	s, n, obj, sf := setupObjFireTest(t, script.TriggerAiOpObj1, objtype.NPCModeOpObj1)
	n.target = obj
	n.targetOp = objtype.NPCModeOpObj1
	n.targetSubject.typ = obj.Type

	n.fireAiOpTriggerObj(s, obj)

	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

func TestFireAiOpTriggerObjNoScript(t *testing.T) {
	s, n, obj, _ := setupObjFireTest(t, 0, objtype.NPCModeOpObj1)
	n.target = obj
	n.targetOp = objtype.NPCModeOpObj1

	n.fireAiOpTriggerObj(s, obj)

	if n.target != nil {
		t.Error("clearInteraction not called")
	}
}

func TestFireAiOpTriggerObjLifecycleInvalid(t *testing.T) {
	s, n, obj, _ := setupObjFireTest(t, script.TriggerAiOpObj1, objtype.NPCModeOpObj1)
	n.target = obj
	n.targetOp = objtype.NPCModeOpObj1

	// Remove from zone.
	zn := s.zoneMap.Get(0, obj.X, obj.Z)
	zn.Objs = nil

	n.fireAiOpTriggerObj(s, obj)

	if n.target != nil {
		t.Error("clearInteraction not called on stale obj")
	}
}

func TestFireAiApTriggerObjHappy(t *testing.T) {
	s, n, obj, sf := setupObjFireTest(t, script.TriggerAiApObj1, objtype.NPCModeApObj1)
	n.target = obj
	n.targetOp = objtype.NPCModeApObj1

	n.fireAiApTriggerObj(s, obj)

	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

func setupObjFireTest(t *testing.T, trigger script.ServerTriggerType, targetOp int) (*Server, *Npc, *entitypkg.Obj, *capturingScriptFile) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t, 42)
	n.server = s
	n.x, n.z, n.level = 100, 100, 0
	obj := entitypkg.NewObj(0, 101, 100, entitypkg.LifecycleRespawn, 88, 1)

	zn := s.zoneMap.Get(0, 101, 100)
	zn.Objs = append(zn.Objs, obj)

	var sf *capturingScriptFile
	if trigger != 0 {
		sf = newCapturingScriptFile(t, trigger, 88, -1)
		s.scriptProvider.Register(sf.scriptFile)
	}
	return s, n, obj, sf
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireAi(Op|Ap)TriggerObj" -v`

Expected: FAIL.

- [ ] **Step 3: Implement both fire helpers**

Append:

```go
// fireAiOpTriggerObj fires AI_OPOBJ1..5 for an Obj target.
// Category resolution via objTypes.Configs if available; defaults to 0.
func (n *Npc) fireAiOpTriggerObj(s *Server, target *entitypkg.Obj) {
	tx, tz, tlevel := target.Coords()
	if !objStillValid(s, target, tx, tz, tlevel) {
		n.clearInteraction()
		return
	}
	trigger := aiOpObjTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	category := 0
	if s.objTypes != nil {
		if target.Type >= 0 && target.Type < len(s.objTypes.Configs) {
			if ot := s.objTypes.Configs[target.Type]; ot != nil {
				category = ot.Category
			}
		}
	}
	sf := s.scriptProvider.GetByTrigger(trigger, target.Type, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil)
}

// fireAiApTriggerObj: same as Op variant but for AP range.
func (n *Npc) fireAiApTriggerObj(s *Server, target *entitypkg.Obj) {
	tx, tz, tlevel := target.Coords()
	if !objStillValid(s, target, tx, tz, tlevel) {
		n.clearInteraction()
		return
	}
	trigger := aiApObjTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	category := 0
	if s.objTypes != nil {
		if target.Type >= 0 && target.Type < len(s.objTypes.Configs) {
			if ot := s.objTypes.Configs[target.Type]; ot != nil {
				category = ot.Category
			}
		}
	}
	sf := s.scriptProvider.GetByTrigger(trigger, target.Type, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil)
}
```

If `s.objTypes` doesn't exist, inline `category := 0` without the lookup (and note as a tracked deviation).

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireAi(Op|Ap)TriggerObj" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction_trigger.go modules/world/npc_interaction_trigger_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Obj-target AP/OP fire helpers

fireAiOpTriggerObj + fireAiApTriggerObj. Lifecycle via objStillValid
(zone-membership). Category via objTypes.Configs when available."
```

---

### Task 25: Top-level `fireAiOpTrigger` / `fireAiApTrigger` dispatchers + tests

**Files:**
- Modify: `modules/world/npc_interaction_trigger.go`
- Test: `modules/world/npc_interaction_trigger_test.go`

**Context:** Two dispatcher functions type-switch on `n.target` and route to the correct per-category fire helper.

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestFireAiOpTriggerDispatchesPlayer(t *testing.T) {
	s, n, p, sf := setupPlayerFireTest(t, script.TriggerAiOpPlayer1, objtype.NPCModeOpPlayer1)
	n.target = p
	n.targetOp = objtype.NPCModeOpPlayer1

	n.fireAiOpTrigger(s)

	if !sf.wasRun() {
		t.Error("fireAiOpTrigger did not dispatch to Player variant")
	}
}

func TestFireAiApTriggerDispatchesNpc(t *testing.T) {
	s, n, target, sf := setupNpcFireTest(t, script.TriggerAiApNpc1, objtype.NPCModeApNpc1)
	n.target = target
	n.targetOp = objtype.NPCModeApNpc1

	n.fireAiApTrigger(s)

	if !sf.wasRun() {
		t.Error("fireAiApTrigger did not dispatch to Npc variant")
	}
}

func TestFireAiOpTriggerUnknownTargetType(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t, 42)
	n.server = s
	n.target = &dummyEntity{x: 101, z: 100, level: 0, valid: true}
	n.targetOp = objtype.NPCModeOpNpc1

	n.fireAiOpTrigger(s) // should no-op, not panic

	// No specific assertion — just ensure no crash/panic.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireAiO?pTriggerDispatches|TestFireAiOpTriggerUnknownTargetType" -v`

Expected: FAIL.

- [ ] **Step 3: Implement both dispatchers**

Append:

```go
// fireAiOpTrigger dispatches to the per-target-category OP fire helper
// based on n.target's concrete type. Called by tryInteract when
// targetOp is in an OP band and target is in operable distance.
func (n *Npc) fireAiOpTrigger(s *Server) {
	switch t := n.target.(type) {
	case *Player:
		n.fireAiOpTriggerPlayer(s, t)
	case *Npc:
		n.fireAiOpTriggerNpc(s, t)
	case *entitypkg.Loc:
		n.fireAiOpTriggerLoc(s, t)
	case *entitypkg.Obj:
		n.fireAiOpTriggerObj(s, t)
	}
}

// fireAiApTrigger — same shape as fireAiOpTrigger for AP band.
func (n *Npc) fireAiApTrigger(s *Server) {
	switch t := n.target.(type) {
	case *Player:
		n.fireAiApTriggerPlayer(s, t)
	case *Npc:
		n.fireAiApTriggerNpc(s, t)
	case *entitypkg.Loc:
		n.fireAiApTriggerLoc(s, t)
	case *entitypkg.Obj:
		n.fireAiApTriggerObj(s, t)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -v`

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction_trigger.go modules/world/npc_interaction_trigger_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add fireAiOpTrigger / fireAiApTrigger dispatchers

Type-switch on n.target (Player/Npc/Loc/Obj) routing to the correct
per-category fire helper. Unknown target types silently no-op —
matches TS Npc.tryInteract flow where the type-check is implicit
in the script lookup."
```

---

### Task 26: `(*Npc).tryInteract()` + tests

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 4. OP-before-AP dispatcher. TS Npc.ts:861-883.

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestNpcTryInteractOpBranchPlayer(t *testing.T) {
	s, n, p, sf := setupPlayerFireTest(t, script.TriggerAiOpPlayer1, objtype.NPCModeOpPlayer1)
	n.target = p
	n.targetOp = objtype.NPCModeOpPlayer1
	n.typ = &objtype.NpcType{AttackRange: 5}
	// Contact range.
	p.x, p.z = 101, 100

	fired := n.tryInteract(s, true)

	if !fired {
		t.Error("tryInteract: false, want true (OP should fire in contact range)")
	}
	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

func TestNpcTryInteractApBranchPlayer(t *testing.T) {
	s, n, p, sf := setupPlayerFireTest(t, script.TriggerAiApPlayer1, objtype.NPCModeApPlayer1)
	n.target = p
	n.targetOp = objtype.NPCModeApPlayer1
	n.typ = &objtype.NpcType{AttackRange: 5}
	// AP range (not contact).
	p.x, p.z = 103, 100

	fired := n.tryInteract(s, false)

	if !fired {
		t.Error("tryInteract: false, want true (AP should fire in AP range)")
	}
	if !sf.wasRun() {
		t.Error("script not executed")
	}
}

func TestNpcTryInteractOutOfRange(t *testing.T) {
	s, n, p, _ := setupPlayerFireTest(t, script.TriggerAiOpPlayer1, objtype.NPCModeOpPlayer1)
	n.target = p
	n.targetOp = objtype.NPCModeOpPlayer1
	n.typ = &objtype.NpcType{AttackRange: 5}
	p.x, p.z = 200, 100 // way out

	fired := n.tryInteract(s, true)

	if fired {
		t.Error("tryInteract: true, want false (target out of range)")
	}
}

func TestNpcTryInteractOpLocRequiresAllowOpScenery(t *testing.T) {
	s, n, loc, sf := setupLocFireTest(t, script.TriggerAiOpLoc1, objtype.NPCModeOpLoc1)
	n.target = loc
	n.targetOp = objtype.NPCModeOpLoc1
	n.typ = &objtype.NpcType{AttackRange: 5}
	n.targetSubject.typ = loc.Type()

	// With allowOpScenery=false, Loc OP should NOT fire (TS tryInteract(false) at post-move)
	fired := n.tryInteract(s, false)
	if fired {
		t.Error("Loc OP fired with allowOpScenery=false")
	}
	if sf.wasRun() {
		t.Error("Loc script ran despite allowOpScenery=false")
	}

	// With allowOpScenery=true, Loc OP should fire.
	fired = n.tryInteract(s, true)
	if !fired {
		t.Error("Loc OP did not fire with allowOpScenery=true")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcTryInteract -v`

Expected: FAIL.

- [ ] **Step 3: Implement `tryInteract`**

Append to `modules/world/npc_interaction.go`:

```go
// tryInteract evaluates whether an AP or OP trigger should fire this tick.
// OP is checked first (contact range); AP second (approach range).
// allowOpScenery gates whether Loc/Obj OP fires — set true on the
// pre-move call, false on the post-move call. Mirrors TS
// Npc.tryInteract at Npc.ts:861-883.
func (n *Npc) tryInteract(s *Server, allowOpScenery bool) bool {
	if n.target == nil || n.typ == nil {
		return false
	}

	// OP branch — contact range.
	if checkOpTrigger(n.targetOp) && n.inOperableDistance(n.target) {
		_, isPlayer := n.target.(*Player)
		_, isNpc := n.target.(*Npc)
		isPathing := isPlayer || isNpc
		if isPathing || allowOpScenery {
			n.fireAiOpTrigger(s)
			return true
		}
		return false
	}

	// AP branch — approach range.
	if checkApTrigger(n.targetOp) && n.inApproachDistance(int(n.typ.AttackRange), n.target) {
		n.fireAiApTrigger(s)
		return true
	}

	return false
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcTryInteract -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.tryInteract (OP-first, AP-second)

Dispatches to fireAiOpTrigger on contact range (with allowOpScenery
gate for Loc/Obj targets), else fireAiApTrigger on approach range.
Matches TS Npc.tryInteract at Npc.ts:861-883."
```

---

## Phase 4 — Mode implementations

### Task 27: Migrate `noMode` + `wanderMode` + `patrolMode` into `npc_interaction.go`

**Files:**
- Modify: `modules/world/npc_interaction.go` (add three methods)
- Modify: `modules/world/npc_ai.go` (DELETE the existing wanderMode + patrolMode; keep queueWaypoint for now — will be used by tests)
- Test: `modules/world/npc_interaction_test.go` (new tests for noMode + ported patrol's delayedPatrol behaviour)

**Context:** Spec Section 2. Port existing `wanderMode`/`patrolMode` from `npc_ai.go` and extend patrol with `delayedPatrol` + 30-tick-stuck teleport (TS :717-744). Add `noMode` (new — just calls updateMovement).

- [ ] **Step 1: Add `delayedPatrol` + `nextPatrolTick` init to NewNpc if missing**

Grep for `delayedPatrol`:

Run: `grep -n "delayedPatrol\|nextPatrolTick" modules/world/npc.go`

If `delayedPatrol bool` is already in the struct and initialised, skip this step. If `nextPatrolTick int` is present, note it. If `delayedPatrol` is missing, add the field to the struct and initialise it in NewNpc to `false`.

- [ ] **Step 2: Write the failing test for noMode**

Append:

```go
func TestNpcNoModeCallsUpdateMovement(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0

	n.noMode(s)

	// noMode just calls updateMovement — npc should advance.
	if n.x != 101 {
		t.Errorf("noMode did not advance: x=%d, want 101", n.x)
	}
}
```

- [ ] **Step 3: Run tests to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcNoMode -v`

Expected: FAIL.

- [ ] **Step 4: Add the three mode methods to `npc_interaction.go`**

Append:

```go
// noMode is the NpcMode.NONE branch — just walks the existing path if
// any. Matches TS noMode at Npc.ts:693-695.
func (n *Npc) noMode(s *Server) {
	n.updateMovement(s)
}

// wanderMode is the NpcMode.WANDER branch — 1/8-tick random-walk within
// WanderRange + updateMovement + 500-tick teleport-to-spawn counter.
// Matches TS wanderMode at Npc.ts:697-715.
func (n *Npc) wanderMode(s *Server) {
	if n.typ == nil {
		return
	}
	if n.moveRestrict != MoveRestrictNoMove && n.typ.WanderRange > 0 && rand.IntN(8) == 0 {
		rng := int(n.typ.WanderRange)
		dx := rand.IntN(rng*2+1) - rng
		dz := rand.IntN(rng*2+1) - rng
		if n.startX+dx != n.x || n.startZ+dz != n.z {
			queueWaypoint(n, n.startX+dx, n.startZ+dz)
		}
	}
	n.updateMovement(s)
	onSpawn := n.x == n.startX && n.z == n.startZ && n.level == n.startLevel
	n.wanderCounter++
	if n.wanderCounter >= 500 {
		if !onSpawn {
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
			n.tele = true
		}
		n.wanderCounter = 0
	}
}

// patrolMode is the NpcMode.PATROL branch — advance through PatrolCoord
// with PatrolDelay, stuck-teleport at 30 ticks, delayedPatrol latch.
// Matches TS patrolMode at Npc.ts:717-744.
func (n *Npc) patrolMode(s *Server) {
	if n.typ == nil || len(n.typ.PatrolCoord) == 0 {
		return
	}
	patrolDelay := 0
	if n.nextPatrolPoint < len(n.typ.PatrolDelay) {
		patrolDelay = int(n.typ.PatrolDelay[n.nextPatrolPoint])
	}
	dest := coordgrid.UnpackCoord(int(n.typ.PatrolCoord[n.nextPatrolPoint]))

	n.updateMovement(s)

	if n.waypointIndex < 0 && n.target == nil {
		queueWaypoint(n, dest.X, dest.Z)
	}
	if (n.x != dest.X || n.z != dest.Z) && n.nextPatrolTick > -1 && s.currentTick >= n.nextPatrolTick {
		// Stuck-teleport to destination.
		n.x, n.z, n.level = dest.X, dest.Z, 0
		n.tele = true
	}
	if n.x == dest.X && n.z == dest.Z && !n.delayedPatrol {
		n.nextPatrolTick = s.currentTick + patrolDelay
		n.delayedPatrol = true
	}
	if n.nextPatrolTick > s.currentTick {
		return
	}

	n.nextPatrolPoint = (n.nextPatrolPoint + 1) % len(n.typ.PatrolCoord)
	n.nextPatrolTick = s.currentTick + 30 // stuck-teleport horizon
	n.delayedPatrol = false
	dest = coordgrid.UnpackCoord(int(n.typ.PatrolCoord[n.nextPatrolPoint]))
	queueWaypoint(n, dest.X, dest.Z)
}
```

If `queueWaypoint` is a method (not a package function), adjust calls to `n.queueWaypoint(...)`. If it's a free function, keep as shown.

Add import of `math/rand/v2` to the file if not present.

- [ ] **Step 5: DELETE old wanderMode/patrolMode in `npc_ai.go`**

Remove lines in `modules/world/npc_ai.go`:
- `func (n *Npc) wanderMode(s *Server)` and its body.
- `func (n *Npc) patrolMode(s *Server)` and its body.

Keep `queueWaypoint` in place (still used by new wander/patrol and by aiMode's `pathToTarget`).

- [ ] **Step 6: Run all tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -v`

Expected: PASS (any existing wander/patrol tests still work because implementations are behaviour-compatible).

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_ai.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "refactor(world): NAI-11 migrate noMode/wanderMode/patrolMode into npc_interaction

Three mode funcs now live adjacent to processMovementInteraction in
npc_interaction.go. patrolMode gains delayedPatrol latch + 30-tick
stuck-teleport (TS :717-744). noMode is new — just calls updateMovement."
```

---

### Task 28: `(*Npc).aiMode()` try-twice pattern + tests

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 2. Try-twice pattern + givechase clause. TS Npc.ts:832-858.

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestNpcAiModeFiresOpBeforeMoveWhenInRange(t *testing.T) {
	s, n, target, sf := setupNpcFireTest(t, script.TriggerAiOpNpc1, objtype.NPCModeOpNpc1)
	n.target = target
	n.targetOp = objtype.NPCModeOpNpc1
	n.typ = &objtype.NpcType{AttackRange: 5, GiveChase: true}
	// Contact range immediately.
	target.x, target.z = 101, 100

	n.aiMode(s)

	if !sf.wasRun() {
		t.Error("aiMode did not fire OP immediately (in range)")
	}
}

func TestNpcAiModeGivechaseFalseClearsTargetAfterMove(t *testing.T) {
	s, n, target, _ := setupNpcFireTest(t, 0, objtype.NPCModeOpNpc1)
	n.target = target
	n.targetOp = objtype.NPCModeOpNpc1
	n.typ = &objtype.NpcType{AttackRange: 5, GiveChase: false}
	target.x, target.z = 110, 100 // far — won't be in range pre-move
	n.moveSpeed = MoveSpeedWalk
	n.server = s

	n.aiMode(s)

	if n.target != nil {
		t.Error("givechase=false + moved: target not cleared")
	}
}

func TestNpcAiModeGivechaseTrueKeepsTarget(t *testing.T) {
	s, n, target, _ := setupNpcFireTest(t, 0, objtype.NPCModeOpNpc1)
	n.target = target
	n.targetOp = objtype.NPCModeOpNpc1
	n.typ = &objtype.NpcType{AttackRange: 5, GiveChase: true}
	target.x, target.z = 110, 100
	n.moveSpeed = MoveSpeedWalk
	n.server = s

	n.aiMode(s)

	if n.target == nil {
		t.Error("givechase=true + moved: target cleared (should persist)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcAiMode -v`

Expected: FAIL.

- [ ] **Step 3: Implement `aiMode`**

Append:

```go
// aiMode is the AP/OP-dispatching branch of processMovementInteraction.
// Try-twice pattern: tryInteract before AND after updateMovement, so
// stepping INTO range fires the trigger same-tick. Also handles the
// givechase clause. Matches TS aiMode at Npc.ts:832-858.
func (n *Npc) aiMode(s *Server) {
	if n.typ == nil {
		return
	}
	n.wanderCounter = 0

	// Pre-move interaction attempt (allowOpScenery=true).
	if n.tryInteract(s, true) {
		return
	}

	// Not in range — path toward target and step.
	n.pathToTarget()
	moved := n.updateMovement(s)

	if moved && !n.typ.GiveChase {
		n.resetDefaults()
		return
	}

	// Post-move interaction attempt (allowOpScenery=false — per TS,
	// the 2nd call guards against OP firing against scenery after
	// walking into contact range, which is the engine's response to
	// an out-of-range click).
	if n.target != nil {
		n.tryInteract(s, false)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNpcAiMode -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.aiMode try-twice + givechase clause

Fires tryInteract before AND after updateMovement (try-twice pattern)
so stepping into range fires triggers same-tick. Givechase=false +
moved clears the interaction (TS :849-853)."
```

---

### Task 29: `(*Npc).processMovementInteraction()` entry + dispatcher tests

**Files:**
- Modify: `modules/world/npc_interaction.go`
- Test: `modules/world/npc_interaction_test.go`

**Context:** Spec Section 2. The top-level dispatcher.

- [ ] **Step 1: Write the failing tests (dispatcher branches)**

Append:

```go
func TestProcessMovementInteractionDelayedBails(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.delayed = true

	n.processMovementInteraction(s)

	// Nothing observable changed — ensure no panic.
	if n.x != 100 {
		t.Error("delayed bail: npc moved")
	}
}

func TestProcessMovementInteractionDeadBails(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.dead = true

	n.processMovementInteraction(s)

	if n.x != 100 {
		t.Error("dead bail: npc moved")
	}
}

func TestProcessMovementInteractionNullFailsafeFallsToDefault(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.targetOp = objtype.NPCModeNull

	n.processMovementInteraction(s)

	if n.targetOp != objtype.NPCModeWander {
		t.Errorf("Null failsafe: targetOp %d, want NPCModeWander", n.targetOp)
	}
}

func TestProcessMovementInteractionWanderInvokesWanderMode(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.targetOp = objtype.NPCModeWander

	initialCounter := n.wanderCounter
	n.processMovementInteraction(s)
	// wanderMode increments wanderCounter.
	if n.wanderCounter != initialCounter+1 {
		t.Errorf("wanderCounter: got %d, want %d", n.wanderCounter, initialCounter+1)
	}
}

func TestProcessMovementInteractionPlayerModesResetToDefault(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.target = &Npc{x: 101, z: 100, level: 0}
	n.targetOp = objtype.NPCModePlayerFollow
	// set targetSubject for validateTarget to pass the type-changed gate
	n.targetSubject.typ = n.target.(*Npc).typeId

	n.processMovementInteraction(s)

	if n.targetOp != objtype.NPCModeWander { // defaults back via resetDefaults
		t.Errorf("PLAYER* mode: targetOp=%d, want NPCModeWander (resetDefaults)", n.targetOp)
	}
	if n.target != nil {
		t.Error("PLAYER* mode: target not cleared")
	}
}

func TestProcessMovementInteractionNilTargetResetsDefaults(t *testing.T) {
	s := newServerForNpcTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.target = nil
	n.targetOp = objtype.NPCModeOpNpc1

	n.processMovementInteraction(s)

	if n.targetOp != objtype.NPCModeWander {
		t.Errorf("nil target with targeted mode: targetOp=%d, want NPCModeWander", n.targetOp)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessMovementInteraction -v`

Expected: FAIL.

- [ ] **Step 3: Implement `processMovementInteraction`**

Append to `modules/world/npc_interaction.go`:

```go
// processMovementInteraction is the NPC's per-tick movement + interaction
// dispatcher. Replaces npc_ai.go's old wander/patrol/advanceWaypoint
// block. Matches TS Npc.processMovementInteraction at Npc.ts:562-603.
//
// DEVIATION: PLAYERESCAPE / PLAYERFOLLOW / PLAYERFACE / PLAYERFACECLOSE
// modes are deferred — they fall through to resetDefaults (scope
// decision Q1). Tracked follow-up.
func (n *Npc) processMovementInteraction(s *Server) {
	if n.delayed || n.dead {
		return
	}

	// Last-tick bookkeeping (from old npc_ai.go:83-84).
	n.lastTickX, n.lastTickZ, n.lastLevel = n.x, n.z, n.level
	n.tele = false

	// Failsafe: NPCModeNull → defaultMode (TS :568-571).
	if n.targetOp == objtype.NPCModeNull {
		n.targetOp = n.defaultMode()
	}

	// Targetless modes (TS :574-582).
	switch n.targetOp {
	case objtype.NPCModeNone:
		n.noMode(s)
		return
	case objtype.NPCModeWander:
		n.wanderMode(s)
		return
	case objtype.NPCModePatrol:
		n.patrolMode(s)
		return
	}

	// Targeted-mode prelude (TS :585-589).
	if n.target == nil || !n.validateTarget() {
		n.resetDefaults()
		return
	}

	// Targeted-mode dispatch (TS :591-602).
	switch n.targetOp {
	case objtype.NPCModePlayerEscape,
		objtype.NPCModePlayerFollow,
		objtype.NPCModePlayerFace,
		objtype.NPCModePlayerFaceClose:
		// DEVIATION: PLAYER* modes deferred to a future sub-spec.
		n.resetDefaults()
	default:
		n.aiMode(s)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessMovementInteraction -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-11 add Npc.processMovementInteraction dispatcher

Top-level entry replacing old npc_ai.go wander/patrol block. Dispatches
None/Wander/Patrol/aiMode by targetOp. PLAYER* modes reset to default
(deferred). Matches TS Npc.processMovementInteraction."
```

---

## Phase 5 — Integration

### Task 30: Replace `npc_ai.go:79-102` with single call + delete old advanceWaypoint

**Files:**
- Modify: `modules/world/npc_ai.go`

**Context:** Final integration step. Collapse the old movement block and delete the no-longer-used `advanceWaypoint`.

- [ ] **Step 1: Replace the old block in `turn()`**

In `modules/world/npc_ai.go`, locate the section starting around line 79 (`// === Movement / wander / patrol ===`) and going through line 102. Replace with:

```go
	// === Movement / interaction (NAI-11) ===
	n.processMovementInteraction(s)
```

- [ ] **Step 2: Delete `advanceWaypoint`**

In `modules/world/npc_ai.go`, remove the entire `func (n *Npc) advanceWaypoint(s *Server)` body (was at lines ~145-175).

- [ ] **Step 3: Ensure `queueWaypoint` stays**

Verify `func queueWaypoint(n *Npc, x, z int)` (or `func (n *Npc) queueWaypoint(x, z int)`) is still present. It's called from `wanderMode`/`patrolMode`/`pathToTarget`.

- [ ] **Step 4: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS. Every movement-related test now exercises the new dispatcher.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_ai.go
git commit --no-gpg-sign -m "refactor(world): NAI-11 collapse Npc.turn movement block

Lines 79-102 (waypoint/wander/patrol switch + teleport-to-spawn)
collapse to a single processMovementInteraction(s) call. Old
advanceWaypoint deleted (logic migrated to stepOnce inside
updateMovement). queueWaypoint stays — still used by wander/patrol/path."
```

---

### Task 31: Migrate `consumeHuntTarget` interaction branch to `SetInteraction`

**Files:**
- Modify: `modules/world/npc_hunt.go`
- Test: `modules/world/npc_hunt_test.go`

**Context:** Close NAI-10 DEVIATION. Replace the two-line target/targetOp write with a single SetInteraction call.

- [ ] **Step 1: Write the failing handoff test**

Append to `modules/world/npc_hunt_test.go`:

```go
func TestConsumeHuntTargetInteractionBranchCallsSetInteraction(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t, 42)
	n.server = s

	target := newNpcForLifecycleTest(t, 99)
	target.server = s
	target.x, target.z, target.level = 105, 105, 0

	// Set up a hunt that will dispatch to interaction branch
	// (FindNewMode = OpNpc1, not QUEUE).
	huntType := &objtype.HuntType{
		Type:            objtype.HuntModeNpc,
		FindNewMode:     objtype.NPCModeOpNpc1,
		FindKeepHunting: false,
	}
	s.huntTypes = &objtype.HuntTypeConfigs{Configs: []*objtype.HuntType{huntType}}

	n.huntTarget = target
	n.huntMode = 0

	s.consumeHuntTarget(n)

	if n.target != target {
		t.Error("target not set via SetInteraction")
	}
	if n.targetOp != objtype.NPCModeOpNpc1 {
		t.Errorf("targetOp: got %d, want NPCModeOpNpc1", n.targetOp)
	}
	// NAI-11 closures:
	if n.apRange != 10 {
		t.Errorf("apRange: got %d, want 10 (deferral #1 closed)", n.apRange)
	}
	if n.apRangeCalled != false {
		t.Error("apRangeCalled: got true, want false (deferral #2 closed)")
	}
	if n.targetSubject.typ != target.typeId {
		t.Errorf("targetSubject.typ: got %d, want %d (deferral #2 closed)",
			n.targetSubject.typ, target.typeId)
	}
	if n.faceEntity != target.nid {
		t.Errorf("faceEntity: got %d, want %d (deferral #4 closed)",
			n.faceEntity, target.nid)
	}
}

func TestConsumeHuntTargetQueueBranchUnchanged(t *testing.T) {
	// Reuse existing TestConsumeHuntTargetQueueBranchFiresScript-style
	// fixture. Verify runNpcScript still fires via nil secondary target.
	// ...abbreviated — mirror an existing NAI-10 test case.
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestConsumeHuntTargetInteractionBranchCallsSetInteraction -v`

Expected: FAIL — apRange stays at 10 but faceEntity is NOT set (old behavior).

- [ ] **Step 3: Migrate the interaction branch in `consumeHuntTarget`**

In `modules/world/npc_hunt.go`, locate `consumeHuntTarget` (around line 169). Replace the interaction-branch body:

```go
	} else {
		// Interaction branch: full SetInteraction port (NAI-11) closes
		// NAI-10 deferrals #1–#7 atomically.
		n.SetInteraction(InteractionScript, n.huntTarget, hunt.FindNewMode, -1)
	}
```

Also shrink the method-level DEVIATION doc comment:

```go
// consumeHuntTarget converts a hunt-phase result (n.huntTarget) into
// interaction state. Matches TS Npc.consumeHuntTarget at
// Engine-TS/.../Npc.ts:887-919.
//
// Control flow:
//   - Entry guards: huntTarget non-nil, huntMode in bounds, hunt config
//     non-nil, hunt.Type != HuntModeOff. Any guard fires → no-op.
//   - Branch on hunt.FindNewMode:
//       QUEUE1..QUEUE20 → fire TriggerAiQueueN directly via runNpcScript.
//       else           → n.SetInteraction(InteractionScript, huntTarget,
//                         FindNewMode, -1). Closes the full setInteraction
//                         side-effect set.
//   - Common tail (both branches): n.huntTarget = nil, n.huntClock = 0.
//   - If !hunt.FindKeepHunting: n.huntMode = -1.
//
// Previously deferred setInteraction side effects (apRange, apRangeCalled,
// targetSubject, focus, faceEntity+masks, targetX/Z, isValid pre-check)
// are now handled by SetInteraction. See Npc.SetInteraction for details.
func (s *Server) consumeHuntTarget(n *Npc) { ... }
```

- [ ] **Step 4: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestConsumeHuntTargetInteractionBranchCallsSetInteraction -v`

Expected: PASS.

Run the full suite:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "refactor(world): NAI-11 consumeHuntTarget uses SetInteraction — closes NAI-10

Interaction branch migrates from 2-field direct write to
SetInteraction(InteractionScript, huntTarget, FindNewMode, -1). All
seven NAI-10 deferrals close atomically. DEVIATION block in the doc
comment shrinks to a reference pointing at Npc.SetInteraction."
```

---

### Task 32: Post-commit verification + final grep pass

**Files:**
- None (verification only)

**Context:** Per memory *"Enumerate ALL call sites when propagating through a shared file"* and *"Verify implementer claims with fresh independent runs"*.

- [ ] **Step 1: Greps**

Run each and inspect:
```bash
grep -rn "NpcModeNone\|NpcModeWander\|NpcModePatrol" modules/ pkg/
# Expected: ZERO matches.

grep -rn "runNpcScript(" modules/ pkg/
# Expected: every call uses the new 4-arg signature (sf, n, target, req).

grep -rn "processMovementInteraction" modules/
# Expected: ONE production call site in Npc.turn; implementation in npc_interaction.go;
#           test sites in *_test.go.

grep -n "DEVIATION" modules/world/npc_hunt.go
# Expected: the old 7-item DEVIATION block in consumeHuntTarget's doc
#           has shrunk to a reference pointing at SetInteraction.

grep -rn "checkOpTrigger\|checkApTrigger" modules/
# Expected: defined in npc_interaction.go, used by tryInteract + targetWithinMaxRange.
#           Player-side does NOT define these (different range contract).

grep -rn "advanceWaypoint" modules/
# Expected: ZERO matches in production code (logic migrated into stepOnce).
#           Test files may still reference it — investigate any hit.
```

- [ ] **Step 2: Full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS across every package, not just modules/world/.

- [ ] **Step 3: Race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... ./pkg/script/...`

Expected: PASS — no data-race reports.

- [ ] **Step 4: Build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o $TMPDIR/goscape ./cmd/goscape`

Expected: clean build, binary produced.

- [ ] **Step 5: Commit (final NAI-11 closure marker)**

```bash
git commit --no-gpg-sign --allow-empty -m "chore(nai): NAI-11 closed — NPC movement-interaction ported

All seven NAI-10 deferrals resolved. Full TS NpcMode enum unified on
objtype.NPCMode*. AP/OP trigger matrix wired for all four target
categories (Player/Npc/Loc/Obj) × (AP, OP) × 1..5. Run-step support
in updateMovement. PLAYERESCAPE/FOLLOW/FACE/FACECLOSE modes deferred
(tracked). SMART pathfinding deferred (tracked). LoS gating deferred
(tracked). Spec: docs/superpowers/specs/2026-04-22-nai-11-npc-movement-interaction-design.md"
```

---

## Self-review checklist (run after plan is complete)

- [ ] Every spec section maps to at least one task.
- [ ] No TBD/TODO/placeholder in code blocks (verified via `grep -n "TBD\|XXX\|FIXME" plan.md` — should only hit the explicit tracked DEVIATION comments).
- [ ] Type consistency: `runNpcScript(sf, n, target any, req)` uniform across every call site in the plan.
- [ ] Every task ends with a commit command using `--no-gpg-sign`.
- [ ] Every Go invocation uses `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go`.
- [ ] NAI-10 DEVIATION closure tracked in Task 31 with explicit regression test covering all 7 deferrals.
- [ ] Test coverage totals ~59 functions (matching spec's "~59 test functions" figure).

---

## Tracked deviations (carry forward to nai_followups.md on NAI-11 close)

1. PLAYERESCAPE / PLAYERFOLLOW / PLAYERFACE / PLAYERFACECLOSE modes deferred.
2. SMART pathfinding branch deferred.
3. reachedEntity / reachedLoc / reachedObj reach helpers deferred (Chebyshev-only).
4. Line-of-sight gating deferred.
5. `interacted` field on `*Npc` deferred.
6. `Interaction.ENGINE` vs `Interaction.SCRIPT` `instant` flag write-only.
7. `n.entitymask` always 0 (mask-plumbing sub-spec will wire).
