# NAI-10 consumeHuntTarget + Target Glue — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `Npc.consumeHuntTarget` (`Engine-TS/.../Npc.ts:887-919`) so the NPC tick converts a hunt-phase result into a consumable `target` / `targetOp` (or fires an `ai_queueN` script when `hunt.findNewMode` falls in `[QUEUE1, QUEUE20]`), clearing `huntTarget`/`huntClock` and optionally `huntMode` afterward.

**Architecture:** New unexported method `(*Server).consumeHuntTarget(n *Npc)` added to `modules/world/npc_hunt.go`, called from `Npc.turn()` between `processNpcHunt` and `processNpcRegen` in `modules/world/npc_ai.go`. Two code branches (QUEUE1..20 → direct `runNpcScript` dispatch via the NAI-2 infrastructure; else → write `n.target` + `n.targetOp`) share a common cleanup tail. New constants `NPCModeQueue1..NPCModeQueue20` in `pkg/objtype/npctype.go` to avoid magic numbers. 11 tests in new file `modules/world/npc_hunt_test.go` (10 unit + 1 integration at tick level).

**Tech Stack:** Go 1.26+, existing `pkg/script` (`TriggerAiQueue1`, `ServerTriggerType`, `Provider.GetByTrigger`, `Provider.Register`, `NewProvider`, `LookupKeyForType`), existing `(*Server).runNpcScript` (NAI-2), existing `pkg/objtype.HuntType`.

**Spec:** `docs/superpowers/specs/2026-04-22-nai-10-consume-hunt-target-design.md`

---

## File Structure

**Create:**
- `modules/world/npc_hunt_test.go` — 11 tests (10 unit + 1 integration).

**Modify:**
- `pkg/objtype/npctype.go` — add `NPCModeQueue1..NPCModeQueue20` constants (lines 42-48 block).
- `pkg/objtype/npctype_test.go` — add one constant-value sanity test.
- `modules/world/npc_hunt.go` — add `consumeHuntTarget` method at bottom; add `pkg/script` to import block.
- `modules/world/npc_ai.go` — insert one call-site line in `Npc.turn()` (after `s.processNpcHunt(n)`, before `s.processNpcRegen(n)`).

**Not touched:**
- No changes to `Npc` struct fields (`target`, `targetOp`, `huntTarget`, `huntMode`, `huntClock` all exist).
- No changes to any `ActiveNpc` interface methods.
- No changes to `processNpcHunt`, `huntAll`, or any `huntXxx` variant.

---

## Task 1: Add `NPCModeQueue1..20` constants

**Files:**
- Modify: `pkg/objtype/npctype.go:42-48`
- Test: `pkg/objtype/npctype_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/objtype/npctype_test.go` (or create the file if absent — check first with `Read`):

```go
func TestNPCModeQueueConstants(t *testing.T) {
	// QUEUE1..QUEUE20 mirror TS NpcMode.ts:76-95 (values 47..66).
	// Used by consumeHuntTarget (NAI-10) to detect the direct-dispatch
	// branch via arithmetic: trigger = TriggerAiQueue1 + (mode - QUEUE1).
	if NPCModeQueue1 != 47 {
		t.Errorf("NPCModeQueue1: got %d, want 47", NPCModeQueue1)
	}
	if NPCModeQueue20 != 66 {
		t.Errorf("NPCModeQueue20: got %d, want 66", NPCModeQueue20)
	}
	if NPCModeQueue20-NPCModeQueue1 != 19 {
		t.Errorf("range: got %d, want 19 (20 consecutive values)",
			NPCModeQueue20-NPCModeQueue1)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestNPCModeQueueConstants -v`
Expected: FAIL with `undefined: NPCModeQueue1` (or similar "undefined" error).

- [ ] **Step 3: Add the constants**

Edit `pkg/objtype/npctype.go`. Find the block starting around line 42:

```go
// NPCMode values (subset of rs-server-225/entity.NPCMode constants relevant
// to the current scope of the port).
const (
	NPCModeNull   = -1
	NPCModeNone   = 0
	NPCModeWander = 1
)
```

Replace with:

```go
// NPCMode values (subset of rs-server-225/entity.NPCMode constants relevant
// to the current scope of the port).
const (
	NPCModeNull   = -1
	NPCModeNone   = 0
	NPCModeWander = 1

	// QUEUE1..QUEUE20 are `ai_queueN`-dispatch modes, consumed by
	// consumeHuntTarget (NAI-10) to fire TriggerAiQueueN directly when
	// HuntType.FindNewMode falls in this range. See TS NpcMode.ts:76-95.
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

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestNPCModeQueueConstants -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/npctype.go pkg/objtype/npctype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-10 add NPCModeQueue1..20 constants

Mirrors TS NpcMode.ts:76-95. Consumed by consumeHuntTarget (NAI-10)
to detect the ai_queueN direct-dispatch branch of HuntType.FindNewMode.
EOF
)"
```

---

## Task 2: Scaffold `consumeHuntTarget` with entry guards only

Writes the method body up to (but not including) the branch dispatch — only the three entry guards. Covers the three no-op tests from the spec (#6, #7, #8) plus the unit-test fixture patterns used by later tasks.

**Files:**
- Create: `modules/world/npc_hunt_test.go`
- Modify: `modules/world/npc_hunt.go` (add import + method body)

- [ ] **Step 1: Write the failing tests**

Create `modules/world/npc_hunt_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// newConsumeHuntTargetFixture builds a Server + Npc ready to exercise
// consumeHuntTarget: s.huntTypes is populated with a single hunt config
// at slot 0, n.huntMode = 0, and n.huntTarget is left for the caller to
// set (a nil-target test should leave it nil).
//
// The returned hunt config has Type=HuntModeNpc, Rate=1, FindKeepHunting=true,
// FindNewMode=NPCModeNone (interaction-branch default). Tests mutate these
// fields in place to exercise specific branches.
func newConsumeHuntTargetFixture(t *testing.T) (*Server, *Npc, *objtype.HuntType) {
	t.Helper()
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	hunt := &objtype.HuntType{
		ConfigType:      objtype.ConfigType{ID: 0},
		Type:            objtype.HuntModeNpc,
		Rate:            1,
		FindKeepHunting: true,
		FindNewMode:     objtype.NPCModeNone,
	}
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{hunt},
	}
	n.huntMode = 0
	return s, n, hunt
}

func TestConsumeHuntTargetNilHuntTargetNoOp(t *testing.T) {
	s, n, _ := newConsumeHuntTargetFixture(t)
	n.huntTarget = nil
	n.target = nil
	n.targetOp = 99
	n.huntClock = 7
	n.huntMode = 5

	s.consumeHuntTarget(n)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (no-op expected)", n.target)
	}
	if n.targetOp != 99 {
		t.Errorf("targetOp: got %d, want 99 (unchanged)", n.targetOp)
	}
	if n.huntClock != 7 {
		t.Errorf("huntClock: got %d, want 7 (unchanged)", n.huntClock)
	}
	if n.huntMode != 5 {
		t.Errorf("huntMode: got %d, want 5 (unchanged)", n.huntMode)
	}
}

func TestConsumeHuntTargetHuntModeOffNoOp(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	n.huntClock = 3
	hunt.Type = objtype.HuntModeOff

	s.consumeHuntTarget(n)

	if n.huntTarget != other {
		t.Errorf("huntTarget: got %v, want unchanged (HuntModeOff gate)", n.huntTarget)
	}
	if n.huntClock != 3 {
		t.Errorf("huntClock: got %d, want 3 (unchanged)", n.huntClock)
	}
}

func TestConsumeHuntTargetInvalidHuntModeNoOp(t *testing.T) {
	s, n, _ := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)

	// Case 1: huntMode = -1 (below lower bound).
	n.huntTarget = other
	n.huntMode = -1
	s.consumeHuntTarget(n)
	if n.huntTarget != other {
		t.Errorf("huntMode=-1: huntTarget should be unchanged, got %v", n.huntTarget)
	}

	// Case 2: huntMode = len(Configs) (above upper bound).
	n.huntTarget = other
	n.huntMode = len(s.huntTypes.Configs) // == 1
	s.consumeHuntTarget(n)
	if n.huntTarget != other {
		t.Errorf("huntMode=OOB: huntTarget should be unchanged, got %v", n.huntTarget)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConsumeHuntTarget -v`
Expected: FAIL with `undefined: (*Server).consumeHuntTarget` (compile error).

- [ ] **Step 3: Add the method skeleton**

Edit `modules/world/npc_hunt.go`. **Do not** touch the import block — Task 2 uses only `objtype` (already imported). The `pkg/script` import is added in Task 4 when it's actually referenced.

Append at the end of the file (after `huntPlayers`):

```go
// consumeHuntTarget converts a hunt-phase result (n.huntTarget) into
// interaction state. Matches TS Npc.consumeHuntTarget at
// Engine-TS/.../Npc.ts:887-919.
//
// Control flow (fully implemented across NAI-10 Tasks 2-5):
//   - Entry guards: huntTarget non-nil, huntMode in bounds, hunt config
//     non-nil, hunt.Type != HuntModeOff. Any guard fires → no-op.
//   - Branch on hunt.FindNewMode:
//       QUEUE1..QUEUE20 → fire TriggerAiQueueN directly via runNpcScript.
//       else           → n.target = n.huntTarget; n.targetOp = FindNewMode.
//   - Common tail (both branches): n.huntTarget = nil, n.huntClock = 0.
//   - If !hunt.FindKeepHunting: n.huntMode = -1.
//
// DEVIATION from TS setInteraction: apRange, apRangeCalled, and
// targetSubject fields are not written — NAI-11 scope, not yet on *Npc.
func (s *Server) consumeHuntTarget(n *Npc) {
	if n.huntTarget == nil {
		return
	}
	if s.huntTypes == nil ||
		n.huntMode < 0 ||
		n.huntMode >= len(s.huntTypes.Configs) {
		return
	}
	hunt := s.huntTypes.Configs[n.huntMode]
	if hunt == nil || hunt.Type == objtype.HuntModeOff {
		return
	}
	// Branch dispatch + common tail land in Tasks 3-5.
}
```

`hunt` compiles cleanly without a trailing blank-assign: Go counts the two reads inside the `if` condition (`hunt == nil`, `hunt.Type == ...`) as use, so the "declared and not used" check is already satisfied.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConsumeHuntTarget -v`
Expected: PASS on `TestConsumeHuntTargetNilHuntTargetNoOp`, `TestConsumeHuntTargetHuntModeOffNoOp`, `TestConsumeHuntTargetInvalidHuntModeNoOp`.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-10 scaffold consumeHuntTarget entry guards

Scaffold the method with the four TS entry guards (huntTarget nil,
huntMode OOB, hunt nil, HuntModeOff) and three no-op unit tests.
Branch bodies land in Task 3 (interaction) and Task 4 (QUEUE).
EOF
)"
```

---

## Task 3: Implement interaction branch + common tail

Adds the else-branch target write and the two-field cleanup tail (`huntTarget = nil`, `huntClock = 0`). The `FindKeepHunting` clause lands in Task 5.

**Files:**
- Modify: `modules/world/npc_hunt.go` (replace scaffolding with interaction branch)
- Modify: `modules/world/npc_hunt_test.go` (add 2 tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_hunt_test.go`:

```go
func TestConsumeHuntTargetInteractionBranchSetsTarget(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindNewMode = 4 // PLAYERFOLLOW — not in QUEUE1..20 range

	s.consumeHuntTarget(n)

	if n.target != other {
		t.Errorf("target: got %v, want %v (interaction branch)", n.target, other)
	}
	if n.targetOp != 4 {
		t.Errorf("targetOp: got %d, want 4 (PLAYERFOLLOW)", n.targetOp)
	}
}

func TestConsumeHuntTargetInteractionBranchClearsHuntState(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	n.huntClock = 5
	hunt.FindNewMode = 4

	s.consumeHuntTarget(n)

	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (cleared after consume)", n.huntTarget)
	}
	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (reset after consume)", n.huntClock)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConsumeHuntTargetInteraction -v`
Expected: FAIL — `target: got <nil>, want ...` (scaffolding never assigns target).

- [ ] **Step 3: Replace scaffolding with interaction branch + tail**

Edit `modules/world/npc_hunt.go`. Replace the final line of `consumeHuntTarget`:

```go
	// Branch dispatch + common tail land in Tasks 3-5.
}
```

with:

```go
	// Interaction branch: write target + targetOp for NAI-11 consumption.
	// Task 4 wraps this in an if/else and adds the QUEUE1..QUEUE20 branch.
	//
	// DEVIATION: TS setInteraction also writes apRange, apRangeCalled,
	// targetSubject.com/type. Those fields don't yet exist on *Npc and
	// have zero NAI-10 consumers; NAI-11 adds them.
	n.target = n.huntTarget
	n.targetOp = hunt.FindNewMode

	// Common tail: clear huntTarget and reset huntClock.
	n.huntTarget = nil
	n.huntClock = 0
}
```

No import changes in this task — `pkg/script` is added in Task 4.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConsumeHuntTarget -v`
Expected: PASS on all 5 existing `TestConsumeHuntTarget*` tests.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-10 consumeHuntTarget interaction branch

Writes n.target = n.huntTarget and n.targetOp = hunt.FindNewMode, then
clears huntTarget and resets huntClock. Matches TS Npc.ts:904-911
(else branch + common tail). QUEUE1..20 branch follows in Task 4;
FindKeepHunting clause follows in Task 5.
EOF
)"
```

---

## Task 4: Implement QUEUE1..QUEUE20 branch

Replaces the `TODO(NAI-10 Task 4)` placeholder with the `ai_queueN` direct-dispatch branch. Adds 3 unit tests covering the happy-path dispatch, the "target NOT set in QUEUE branch" guard, and the QUEUE20 boundary.

**Files:**
- Modify: `modules/world/npc_hunt.go` (replace TODO with QUEUE branch + if/else structure)
- Modify: `modules/world/npc_hunt_test.go` (add 3 tests + 1 fixture helper)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_hunt_test.go`:

```go
import "github.com/zsrv/goscape/pkg/script"

// newQueueBranchFixture extends the consumeHuntTarget fixture with
// a scriptProvider wired for AI_QUEUE dispatch. Returns (s, n, hunt,
// provider) so the test can register specific trigger scripts.
//
// The NPC's type (created by newNpcForLifecycleTest) has typeId=0
// and category=-1; scripts must be registered at (trigger, 0)
// specifically for GetByTrigger to find them.
func newQueueBranchFixture(t *testing.T) (
	*Server, *Npc, *objtype.HuntType, *script.Provider,
) {
	t.Helper()
	s, n, hunt := newConsumeHuntTargetFixture(t)
	s.scriptProvider = script.NewProvider()
	return s, n, hunt, s.scriptProvider
}

// buildTimerSetScript creates a script that sets n.timerInterval to a
// specific value — used as a dispatch observer. Post-run, asserting
// n.timerInterval equals `val` proves the script fired (and therefore
// that the correct TriggerAiQueueN was dispatched).
//
// Body: OpPushConstantInt (push val), OpNpcSetTimer (pop, set timer),
// OpReturn. Registered at (trigger, typeID) via LookupKeyForType.
func buildTimerSetScript(t *testing.T, trigger script.ServerTriggerType, typeID int, val int32) *script.ScriptFile {
	t.Helper()
	return &script.ScriptFile{
		Name:             "[queue_branch_observer]",
		LookupKey:        script.LookupKeyForType(trigger, typeID),
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpNpcSetTimer, script.OpReturn},
		IntOperands:      []int32{val, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
}

func TestConsumeHuntTargetQueueBranchFiresScript(t *testing.T) {
	s, n, hunt, provider := newQueueBranchFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindNewMode = objtype.NPCModeQueue3 // 49 → should fire TriggerAiQueue3

	const observerVal int32 = 12345
	provider.Register(buildTimerSetScript(t, script.TriggerAiQueue3, n.typeId, observerVal))

	s.consumeHuntTarget(n)

	if int32(n.timerInterval) != observerVal {
		t.Errorf("timerInterval: got %d, want %d (script should have fired)",
			n.timerInterval, observerVal)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (common-tail cleanup)", n.huntTarget)
	}
}

func TestConsumeHuntTargetQueueBranchDoesNotSetTarget(t *testing.T) {
	s, n, hunt, _ := newQueueBranchFixture(t)
	preexisting := newNpcForLifecycleTest(t)
	other := newNpcForLifecycleTest(t)
	n.target = preexisting
	n.targetOp = 999
	n.huntTarget = other
	hunt.FindNewMode = objtype.NPCModeQueue3

	// Note: no script registered for TriggerAiQueue3 — runNpcScript is
	// a no-op on nil sf. This is the "happy-path with no registered
	// handler" case; huntTarget cleanup still runs.

	s.consumeHuntTarget(n)

	if n.target != preexisting {
		t.Errorf("target: got %v, want %v (QUEUE branch must NOT set target)",
			n.target, preexisting)
	}
	if n.targetOp != 999 {
		t.Errorf("targetOp: got %d, want 999 (unchanged)", n.targetOp)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (common tail)", n.huntTarget)
	}
}

func TestConsumeHuntTargetQueueBranchBoundaryQueue20(t *testing.T) {
	s, n, hunt, provider := newQueueBranchFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindNewMode = objtype.NPCModeQueue20 // 66 → should fire TriggerAiQueue20

	const observerVal int32 = 77777
	provider.Register(buildTimerSetScript(t, script.TriggerAiQueue20, n.typeId, observerVal))

	s.consumeHuntTarget(n)

	if int32(n.timerInterval) != observerVal {
		t.Errorf("timerInterval: got %d, want %d (Queue20 dispatch)",
			n.timerInterval, observerVal)
	}
}
```

Note: if the existing import block in `npc_hunt_test.go` doesn't already include `pkg/script`, merge the new `import` line into the existing block rather than adding a second one. Go accepts only a single import declaration per file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConsumeHuntTargetQueue -v`
Expected: FAIL on `TestConsumeHuntTargetQueueBranchFiresScript` and `TestConsumeHuntTargetQueueBranchBoundaryQueue20` — `timerInterval: got 0, want 12345` (script never fires because the QUEUE branch is still routed through the interaction branch). `TestConsumeHuntTargetQueueBranchDoesNotSetTarget` fails too — interaction branch clobbers `n.target`.

- [ ] **Step 3a: Add `pkg/script` to the import block**

Edit `modules/world/npc_hunt.go`. Update the import block from:

```go
import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

to:

```go
import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)
```

- [ ] **Step 3b: Wrap interaction branch in if/else with QUEUE branch**

In `modules/world/npc_hunt.go`, replace:

```go
	// Interaction branch: write target + targetOp for NAI-11 consumption.
	// Task 4 wraps this in an if/else and adds the QUEUE1..QUEUE20 branch.
	//
	// DEVIATION: TS setInteraction also writes apRange, apRangeCalled,
	// targetSubject.com/type. Those fields don't yet exist on *Npc and
	// have zero NAI-10 consumers; NAI-11 adds them.
	n.target = n.huntTarget
	n.targetOp = hunt.FindNewMode
```

with:

```go
	if hunt.FindNewMode >= objtype.NPCModeQueue1 &&
		hunt.FindNewMode <= objtype.NPCModeQueue20 {
		// QUEUE1..QUEUE20 branch: fire TriggerAiQueueN directly (not
		// enqueued). target/targetOp NOT written — the script owns
		// subsequent state. Matches TS Npc.ts:896-903.
		if n.typ != nil && s.scriptProvider != nil {
			trigger := script.TriggerAiQueue1 +
				script.ServerTriggerType(hunt.FindNewMode-objtype.NPCModeQueue1)
			sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
			s.runNpcScript(sf, n, nil, nil)
		}
	} else {
		// Interaction branch: write target + targetOp for NAI-11 consumption.
		//
		// DEVIATION: TS setInteraction also writes apRange, apRangeCalled,
		// targetSubject.com/type. Those fields don't yet exist on *Npc and
		// have zero NAI-10 consumers; NAI-11 adds them.
		n.target = n.huntTarget
		n.targetOp = hunt.FindNewMode
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConsumeHuntTarget -v`
Expected: PASS on all 8 `TestConsumeHuntTarget*` tests.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-10 consumeHuntTarget QUEUE1..20 branch

Fires TriggerAiQueueN directly via runNpcScript when hunt.FindNewMode
falls in [NPCModeQueue1, NPCModeQueue20]. target/targetOp deliberately
NOT written in this branch — the script owns subsequent state. Matches
TS Npc.ts:896-903.
EOF
)"
```

---

## Task 5: Add `!FindKeepHunting` → `huntMode = -1` clause

Adds the final stop-hunting clause that runs after the common tail. Two new tests cover both sides of the `FindKeepHunting` toggle.

**Files:**
- Modify: `modules/world/npc_hunt.go` (add one `if` at end of method)
- Modify: `modules/world/npc_hunt_test.go` (add 2 tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_hunt_test.go`:

```go
func TestConsumeHuntTargetFindKeepHuntingFalseClearsHuntMode(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindKeepHunting = false
	hunt.FindNewMode = 4
	n.huntMode = 0 // pointing at Configs[0], valid entry

	s.consumeHuntTarget(n)

	if n.huntMode != -1 {
		t.Errorf("huntMode: got %d, want -1 (!FindKeepHunting clears it)", n.huntMode)
	}
}

func TestConsumeHuntTargetFindKeepHuntingTrueKeepsHuntMode(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	hunt.FindKeepHunting = true
	hunt.FindNewMode = 4
	n.huntMode = 0

	s.consumeHuntTarget(n)

	if n.huntMode != 0 {
		t.Errorf("huntMode: got %d, want 0 (FindKeepHunting preserves it)", n.huntMode)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConsumeHuntTargetFindKeep -v`
Expected: FAIL on `TestConsumeHuntTargetFindKeepHuntingFalseClearsHuntMode` — `huntMode: got 0, want -1` (method never clears huntMode). Other test passes by coincidence (huntMode already 0).

- [ ] **Step 3: Add the stop-hunting clause**

Edit `modules/world/npc_hunt.go`. At the end of `consumeHuntTarget` (after `n.huntClock = 0`, before the closing `}`), add:

```go
	// Stop-hunting clause: once an NPC finds a huntTarget, it won't
	// hunt again until its interactions are cleared — unless the hunt
	// config explicitly opts into keep-hunting. Matches TS Npc.ts:913-918.
	if !hunt.FindKeepHunting {
		n.huntMode = -1
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConsumeHuntTarget -v`
Expected: PASS on all 10 `TestConsumeHuntTarget*` tests.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-10 consumeHuntTarget stop-hunting clause

After the common tail: if !hunt.FindKeepHunting, clear n.huntMode to
-1. Matches TS Npc.ts:913-918 — an NPC that found a target won't hunt
again until interactions clear, unless the hunt config opts in.
EOF
)"
```

---

## Task 6: Wire call site in `Npc.turn()` + integration test

Inserts the one-line call in `Npc.turn()` between `processNpcHunt` and `processNpcRegen`. Adds an end-to-end tick test proving consumeHuntTarget runs at the right phase boundary.

**Files:**
- Modify: `modules/world/npc_ai.go` (insert one line in `turn()`)
- Modify: `modules/world/npc_hunt_test.go` (add integration test)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_hunt_test.go`:

```go
func TestNpcTurnHuntAndConsumeSetsTarget(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)

	// Prepare the NPC to run a full turn() and reach consumeHuntTarget:
	//   - Avoid the Events block by setting lifecycle to Forever.
	//   - Place the NPC at a known coord with a configured huntRange.
	n.lifecycle = NpcLifecycleForever
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	// Configure the hunt for NPC-type hunt (HuntModePlayer is skipped by
	// the turn() path per TS Npc.ts:164, so we exercise huntNpcs here).
	hunt.Type = objtype.HuntModeNpc
	hunt.Rate = 1
	hunt.FindNewMode = 4 // PLAYERFOLLOW (interaction branch)
	hunt.FindKeepHunting = true
	hunt.NobodyNear = objtype.HuntNobodyNearKeepHunting // bypass observer gate
	hunt.CheckNpc = -1
	hunt.CheckCategory = -1

	// Seed the grid with a target NPC in range.
	target := addNpcToServerAt(t, s, 10, 1, -1, n.x+3, n.z+3, n.level)

	// Install the hunting NPC into s.npcs so the tick fixture is internally
	// consistent (some helpers expect it); nid=1 reserved by fixture.
	s.npcs[1] = n

	// Run the full tick.
	n.turn(s)

	if n.target == nil {
		t.Fatal("target: got nil, want the hunted NPC (proves consumeHuntTarget ran after processNpcHunt)")
	}
	if n.target.Slot() != target.nid {
		t.Errorf("target.Slot(): got %d, want %d", n.target.Slot(), target.nid)
	}
	if n.targetOp != 4 {
		t.Errorf("targetOp: got %d, want 4 (PLAYERFOLLOW from FindNewMode)", n.targetOp)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (cleared by consumeHuntTarget)", n.huntTarget)
	}
	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (reset by consumeHuntTarget)", n.huntClock)
	}
	if n.huntMode != 0 {
		t.Errorf("huntMode: got %d, want 0 (FindKeepHunting=true preserves it)", n.huntMode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcTurnHuntAndConsumeSetsTarget -v`
Expected: FAIL — `target: got nil, want ...`. processNpcHunt populates `n.huntTarget`, but with no call site in `turn()`, consumeHuntTarget never converts it to `n.target`.

- [ ] **Step 3: Insert the call site**

Edit `modules/world/npc_ai.go`. Find the block:

```go
	// === Hunt + regen + timer + queue (NAI-7, NAI-6, NAI-4, NAI-3) ===
	s.processNpcHunt(n)  // NAI-7 — matches TS Npc.ts:158-171
	s.processNpcRegen(n) // NAI-6 — matches TS Npc.ts:176
	s.processNpcTimer(n)
	s.processNpcQueue(n)
```

Replace with:

```go
	// === Hunt + consume + regen + timer + queue (NAI-7..10, NAI-6, NAI-4, NAI-3) ===
	s.processNpcHunt(n)      // NAI-7 — matches TS Npc.ts:158-171
	s.consumeHuntTarget(n)   // NAI-10 — matches TS Npc.ts:174
	s.processNpcRegen(n)     // NAI-6 — matches TS Npc.ts:176
	s.processNpcTimer(n)
	s.processNpcQueue(n)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcTurnHuntAndConsumeSetsTarget -v`
Expected: PASS.

- [ ] **Step 5: Run the full NAI-10 test suite + whole repo**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConsumeHuntTarget -v`
Expected: PASS on all 10 unit tests.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS on every package. Per memory "Verify implementer claims with fresh independent runs", a package-scoped run can hide cross-package breakage — always end with `./...`.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_ai.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-10 wire consumeHuntTarget into Npc.turn — closes NAI-10

Inserts consumeHuntTarget between processNpcHunt and processNpcRegen
matching TS Npc.ts:174. Integration test TestNpcTurnHuntAndConsumeSetsTarget
proves the phase order end-to-end: huntNpcs populates huntTarget →
consumeHuntTarget writes target/targetOp and clears huntTarget/huntClock.

Closes NAI-10. Deferred-to-NAI-11: apRange, apRangeCalled, targetSubject,
Interaction enum, target.isValid() pre-check.
EOF
)"
```

---

## Post-merge verification

After Task 6 commits, run the memory-prescribed cross-check:

- [ ] **Grep call sites of `consumeHuntTarget`** — expected: exactly one, in `modules/world/npc_ai.go` `Npc.turn()`. Any additional call site is a scope leak.

  ```
  Grep pattern: consumeHuntTarget
  Path: modules/world/
  Expected lines: the declaration (npc_hunt.go), the turn() call (npc_ai.go),
                  and the test file references.
  ```

- [ ] **Grep `NPCModeQueue`** — expected: constants in `pkg/objtype/npctype.go`, consumers in `modules/world/npc_hunt.go` (2 references: `NPCModeQueue1` + `NPCModeQueue20`) and in `modules/world/npc_hunt_test.go` (3 references: `Queue3`, `Queue20`, plus the constants test in `pkg/objtype/npctype_test.go`).

- [ ] **Fresh full-repo test** — `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` from `$HOME/Code/github.com/zsrv/goscape`. Must pass cleanly; no "pre-existing failure" hand-wave.

- [ ] **go vet** — `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`. Must pass cleanly.

---

## Deviations from TS tracked in-code

These three divergences each have a `// DEVIATION:` breadcrumb in `modules/world/npc_hunt.go` pointing back to NAI-11 scope:

1. `apRange=10`, `apRangeCalled=false`, `targetSubject.com/type` — not written in the interaction branch. NAI-11 adds these fields.
2. `target.isValid()` pre-check — not checked. NAI-11's interaction processing gates on target validity.
3. `Interaction.SCRIPT` / `Interaction.ENGINE` enum — not introduced. NAI-11 adds the distinction when its dispatch path needs it.

If any of these three items are still open after NAI-11 lands, move them to `nai_followups.md` with the usual "From NAI-11 (date)" section.
