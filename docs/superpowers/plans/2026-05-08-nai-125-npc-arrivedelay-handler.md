# NAI-125 — NPC_ARRIVEDELAY (opcode 2502) handler implementation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `NpcOps.ts:542-555` `NPC_ARRIVEDELAY` (opcode 2502) into goscape. Reserved opcode constant has no dispatch entry; ScriptRunner aborts `[proc,npc_death]` at pc=4 every NPC kill.

**Architecture:** Additive port across 5 files. Two production files (`pkg/script/active.go` interface; `pkg/script/handlers_npc.go` handler; `pkg/script/handlers.go` dispatch entry; `modules/world/npc_script.go` impl on `*Npc`; `modules/world/npc.go` doc-comment retire) + one test file (`pkg/script/handlers_npc_test.go` mockNpc extension + 7 tests). Standard subagent-driven-development: 4 implementer tasks (T1–T4), then controller-orchestrated Sonnet review + user-launched smoke + close.

**Tech Stack:** Go 1.26+. No new dependencies.

---

## Background

The opcode constant `OpNpcArriveDelay = 2502` is reserved at `pkg/script/opcode.go:239` (and stringified at `:877-878`) but has no dispatch entry in `handlers.go:262-468`. Per `pkg/script/runner.go:65-77`, `Execute` returns `script %q: no handler for NPC_ARRIVEDELAY (opcode 2502) at pc=N` and sets `Execution = Aborted` when the runner hits the unhandled opcode. NAI-123 close smoke (`5687f4e`, 2026-05-07) confirmed this aborts `[proc,npc_death]` at pc=4 every NPC kill.

**TS reference** at `Engine-TS/src/engine/script/handlers/NpcOps.ts:542-555`:

```ts
[ScriptOpcode.NPC_ARRIVEDELAY]: checkedHandler(ActiveNpc, state => {
    if (state.activeNpc.lastMovement < World.currentTick - 1) {
        return;
    }
    state.activeNpc.delayed = true;
    if (state.activeNpc.lastMovement === World.currentTick - 1) {
        state.activeNpc.delayedUntil = World.currentTick + 1;
    } else {
        state.activeNpc.delayedUntil = World.currentTick + 2;
    }
    state.execution = ScriptState.NPC_SUSPENDED;
}),
```

**Three-tick acceptance window with recency-dependent suspend:**
- `lastMovement = T+1` (this tick) → `SetDelayed(1)` (TS `delayedUntil = T+2`)
- `lastMovement = T` (last tick) → `SetDelayed(1)` (TS `delayedUntil = T+2`)
- `lastMovement = T-1` (2 ticks ago) → `SetDelayed(0)` (TS `delayedUntil = T+1`) ← **NPC-unique branch**
- `lastMovement = T-2` (3 ticks ago) → no-op
- `lastMovement = 0` (never) → no-op

Mapping to goscape's `SetDelayed(ticks)` (writes `delayedUntil = currentTick + 1 + ticks` per `modules/world/npc.go:323-326`):
- TS `delayedUntil = T+2` ⇒ goscape `SetDelayed(1)`
- TS `delayedUntil = T+1` ⇒ goscape `SetDelayed(0)`

**Producer-side wiring is already complete** per NAI-82: `(*Npc).updateMovement` writes `n.lastMovement = s.currentTick + 1` at `modules/world/npc_interaction.go:334`, pinned by `modules/world/npc_movement_test.go:24-50`. NAI-125 adds the consumer.

## File Structure

| File | Responsibility | Touched in |
|---|---|---|
| `pkg/script/handlers_npc_test.go` | mockNpc fixture extension + 7 NPC_ARRIVEDELAY tests | T1, T3 |
| `pkg/script/active.go` | `ActiveNpc.LastMovement() int` interface method | T2 |
| `modules/world/npc_script.go` | `(*Npc).LastMovement() int` impl | T2 |
| `modules/world/npc.go` | retire stale `:74-76` "deferred" doc-comment | T2 |
| `pkg/script/handlers_npc.go` | `handleNpcArriveDelay` handler function | T4 |
| `pkg/script/handlers.go` | dispatch-table entry `OpNpcArriveDelay: handleNpcArriveDelay` | T4 |

**Why this order:**
- T1 extends mockNpc first so T3's tests compile cleanly.
- T2 activates the `LastMovement` reader on the production interface + impl. After T2, `go build ./...` still passes (no consumer yet).
- T3 writes the 7 RED tests; they compile (mockNpc has the field+method) and fail at runtime with `no handler for NPC_ARRIVEDELAY` since the dispatch table is unchanged.
- T4 adds the handler + dispatch entry; tests turn GREEN.

This avoids any commit with a broken `go build`. Each task ends in a clean intermediate state.

---

## Task 1: Extend `mockNpc` fixture with `lastMovement` field + `LastMovement()` getter

**Files:**
- Modify: `pkg/script/handlers_npc_test.go` (struct definition at `:199-241`; method block at `:243-249`)

**Why this is task 1:** T3's tests construct `&mockNpc{lastMovement: <value>}`. The field must exist before the test file references it.

- [ ] **Step 1.1: Read current mockNpc state**

Run: `rg -n "^type mockNpc struct \{|\\bsetDelayedCalls\\b" pkg/script/handlers_npc_test.go`

Expected: line 199 (struct), line 222 (`setDelayedCalls []int`), line 344 (`SetDelayed` method).

- [ ] **Step 1.2: Add `lastMovement int` field to mockNpc struct**

Insert in `pkg/script/handlers_npc_test.go` near the other movement-adjacent fields. Add a new field line just before `setDelayedCalls []int` at line 222:

```go
	// NAI-125: mirrors PathingEntity.lastMovement reader for
	// NPC_ARRIVEDELAY. Default 0 = "never moved".
	lastMovement                       int
	setDelayedCalls                    []int
```

(Use the surrounding tab-aligned width — match the existing fields' alignment.)

- [ ] **Step 1.3: Add `LastMovement()` method to mockNpc**

Insert in `pkg/script/handlers_npc_test.go` immediately after the existing `Nid()` getter (around line 248), preserving the alphabetic-by-grouping convention used by the production `*Npc` impl:

```go
func (m *mockNpc) LastMovement() int { return m.lastMovement }
```

- [ ] **Step 1.4: Verify build passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean exit, no output.

- [ ] **Step 1.5: Verify existing tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/`

Expected: `ok  github.com/zsrv/goscape/pkg/script ...`

- [ ] **Step 1.6: Commit**

```bash
git add pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(nai-125): T1 — extend mockNpc with lastMovement getter

Adds lastMovement int field + LastMovement() int method to the
shared NPC test fixture, ahead of T3's NPC_ARRIVEDELAY test family
that constructs &mockNpc{lastMovement: <value>}. Standalone change;
no consumer yet, build clean.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Activate `Npc.LastMovement` reader on `ActiveNpc` interface + `*Npc` impl + retire stale comment

**Files:**
- Modify: `pkg/script/active.go` (add interface method around line 626, near other NPC reader-getters)
- Modify: `modules/world/npc_script.go` (add `*Npc` impl after the `NpcCategory` block at `:29-37`)
- Modify: `modules/world/npc.go:74-77` (drop stale "deferred" doc-comment framing)

**Why this is task 2:** Adding `LastMovement` to the `ActiveNpc` interface forces every implementer (`*Npc` + `mockNpc`) to provide the method. T1 already added `LastMovement` to `mockNpc`, so the only producer-side gap is `*Npc`. Done atomically with the comment retire.

- [ ] **Step 2.1: Read current ActiveNpc interface neighborhood**

Run: `rg -n "^\t(NpcType|Nid|NpcVarN)\b" pkg/script/active.go | head`

Expected: `:609 NpcType()`, `:625 Nid()`, `:626 NpcVarN(...)`. Insert point: between `Nid()` (`:625`) and `NpcVarN(...)` (`:626`), to maintain reader-getter grouping.

- [ ] **Step 2.2: Add `LastMovement() int` to ActiveNpc interface**

In `pkg/script/active.go`, locate the existing `Nid()` line (currently around `:622-625`):

```go
	// Nid returns the NPC slot id (the low 16 bits of NpcUID). Used by
	// NPC-targeted player-bound packets like HintArrow that reference
	// the NPC by slot rather than by packed UID.
	Nid() int
	NpcVarN(id int) int32
```

Insert the new method between `Nid() int` and `NpcVarN(id int) int32`:

```go
	// Nid returns the NPC slot id (the low 16 bits of NpcUID). Used by
	// NPC-targeted player-bound packets like HintArrow that reference
	// the NPC by slot rather than by packed UID.
	Nid() int

	// LastMovement returns the NPC's TS-PathingEntity.lastMovement value
	// (set to currentTick + 1 at the end of any tick the NPC stepped, else
	// 0). Read by NPC_ARRIVEDELAY (NpcOps.ts:542-555). Mirrors
	// ActivePlayer.LastMovement.
	LastMovement() int

	NpcVarN(id int) int32
```

- [ ] **Step 2.3: Add `(*Npc).LastMovement()` impl in `npc_script.go`**

Locate the existing `NpcCategory` block in `modules/world/npc_script.go` (currently `:29-37`):

```go
func (n *Npc) NpcCategory() int {
	if n.typ == nil {
		return 0
	}
	return n.typ.Category
}

func (n *Npc) NpcStat(stat int) int {
```

Insert the new method between the `NpcCategory` closing brace and the next definition (`NpcStat`):

```go
func (n *Npc) NpcCategory() int {
	if n.typ == nil {
		return 0
	}
	return n.typ.Category
}

// LastMovement returns n.lastMovement, satisfying script.ActiveNpc.
// Used by NPC_ARRIVEDELAY (handlers_npc.go). The field is written by
// (*Npc).updateMovement at npc_interaction.go:334 to currentTick + 1
// after any tick the NPC stepped.
func (n *Npc) LastMovement() int { return n.lastMovement }

func (n *Npc) NpcStat(stat int) int {
```

- [ ] **Step 2.4: Retire stale "deferred" doc-comment in `npc.go`**

Locate `modules/world/npc.go:74-77`:

```go
	// NAI-82: TS PathingEntity.lastMovement (Engine-TS/.../PathingEntity.ts:56).
	// Written to currentTick + 1 at end of updateMovement when position changed;
	// read by AI_ARRIVEDELAY / AI_TARGETMOVED (deferred — see NAI-82 spec §6.1).
	lastMovement int
```

Replace with:

```go
	// NAI-82 (writer) + NAI-125 (reader): TS PathingEntity.lastMovement
	// (Engine-TS/.../PathingEntity.ts:56). Written to currentTick + 1 at
	// end of updateMovement when position changed (npc_interaction.go:334);
	// read by NPC_ARRIVEDELAY via ActiveNpc.LastMovement (handlers_npc.go).
	lastMovement int
```

- [ ] **Step 2.5: Verify build passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean exit, no output. (Both `*Npc` and `mockNpc` now satisfy the extended `ActiveNpc` interface.)

- [ ] **Step 2.6: Verify go vet passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean exit, no output.

- [ ] **Step 2.7: Verify existing tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ ./modules/world/`

Expected: both packages report `ok ... cached` or fresh PASS.

- [ ] **Step 2.8: Commit**

```bash
git add pkg/script/active.go modules/world/npc_script.go modules/world/npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-125): T2 — activate Npc.LastMovement reader

Adds LastMovement() int to ActiveNpc interface (pkg/script/active.go)
and (*Npc).LastMovement implementation (modules/world/npc_script.go).
Retires stale "deferred — see NAI-82 spec §6.1" comment at npc.go:76;
the writer landed in NAI-82 and NAI-125 lands the reader.

No production consumers yet — handler ports in T4. Build clean,
tests pass at this intermediate state.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: RED — write 7 NPC_ARRIVEDELAY tests

**Files:**
- Modify: `pkg/script/handlers_npc_test.go` (append at end of file)

**Why this is task 3:** With T1 + T2 in place, mockNpc satisfies the `ActiveNpc` interface and `lastMovement` is settable. The 7 tests will compile and fail at runtime with `script "<name>": no handler for NPC_ARRIVEDELAY (opcode 2502) at pc=0` because the dispatch table is unchanged.

**Test pattern for OpcodeArr fixture (matching the runNpcOp neighborhood):** each test constructs a single-opcode `ScriptFile{Name: "<case>", Opcodes: []Opcode{OpNpcArriveDelay, OpReturn}}`, sets `state.ActiveNpc = mn` and (where needed) `state.World = w`, then calls `Execute(state)` and asserts on `state.Execution`, `mn.setDelayedCalls`, and `err`.

The `requireActiveNpc` gate at `pkg/script/handlers_npc.go:98-103` checks `s.ActiveNpc == nil` only (not `s.Pointers & PtrActiveNpc`). For the no-active-npc case it suffices to leave `state.ActiveNpc` unset.

- [ ] **Step 3.1: Locate end-of-file insertion point**

Run: `wc -l pkg/script/handlers_npc_test.go`

Expected: a number around 2100. Append the new test family at the end of the file.

- [ ] **Step 3.2: Add the 7-test family**

Append to `pkg/script/handlers_npc_test.go`:

```go

// -- NPC_ARRIVEDELAY tests (NAI-125) ---------------------------------------
//
// TS NpcOps.ts:542-555: 3-tick acceptance window with recency-dependent
// suspend duration. Asymmetric vs P_ARRIVEDELAY (which has a 2-tick
// window and always SetDelayed(0)).
//
// lastMovement is written by (*Npc).updateMovement to currentTick + 1
// after any tick the NPC stepped (npc_interaction.go:334). The
// SetDelayed(ticks) primitive at npc.go:323-326 writes both
// n.delayed = true and n.delayedUntil = currentTick + 1 + ticks, so:
//   TS delayedUntil = T+2 ⇒ goscape SetDelayed(1)
//   TS delayedUntil = T+1 ⇒ goscape SetDelayed(0)

// TestNpcArriveDelaySuspendsWhenMovedThisTick: lastMovement = currentTick + 1.
// Gate condition: 101 < 99 is false ⇒ continue. Branch: 101 == 99 is false
// ⇒ SetDelayed(1) (delayedUntil = T+2).
func TestNpcArriveDelaySuspendsWhenMovedThisTick(t *testing.T) {
	mn := &mockNpc{lastMovement: 101}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_moved_this_tick",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", state.Execution)
	}
	if len(mn.setDelayedCalls) != 1 || mn.setDelayedCalls[0] != 1 {
		t.Errorf("setDelayedCalls: got %v, want [1]", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelaySuspendsWhenMovedLastTick: lastMovement = currentTick (the
// boundary case — moved on tick T-1 means lastMovement was set to T-1+1 = T).
// Gate condition: 100 < 99 is false ⇒ continue. Branch: 100 == 99 is false
// ⇒ SetDelayed(1) (delayedUntil = T+2). Mid-of-window.
func TestNpcArriveDelaySuspendsWhenMovedLastTick(t *testing.T) {
	mn := &mockNpc{lastMovement: 100}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_moved_last_tick",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", state.Execution)
	}
	if len(mn.setDelayedCalls) != 1 || mn.setDelayedCalls[0] != 1 {
		t.Errorf("setDelayedCalls: got %v, want [1]", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelaySuspendsWhenMovedTwoTicksAgo: lastMovement = currentTick - 1.
// Gate condition: 99 < 99 is false ⇒ continue. Branch: 99 == 99 is true
// ⇒ SetDelayed(0) (delayedUntil = T+1). NPC-unique branch (no equivalent
// in P_ARRIVEDELAY which always SetDelayed(0) regardless).
func TestNpcArriveDelaySuspendsWhenMovedTwoTicksAgo(t *testing.T) {
	mn := &mockNpc{lastMovement: 99}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_moved_two_ticks_ago",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", state.Execution)
	}
	if len(mn.setDelayedCalls) != 1 || mn.setDelayedCalls[0] != 0 {
		t.Errorf("setDelayedCalls: got %v, want [0]", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelayNoOpWhenMovedThreeTicksAgo: lastMovement = currentTick - 2
// (the first tick on which the gate becomes a no-op).
// Gate condition: 98 < 99 is true ⇒ return early.
func TestNpcArriveDelayNoOpWhenMovedThreeTicksAgo(t *testing.T) {
	mn := &mockNpc{lastMovement: 98}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_moved_three_ticks_ago",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished (no-op should let OpReturn complete)", state.Execution)
	}
	if len(mn.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (no-op must not call SetDelayed)", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelayNoOpWhenNeverMoved: lastMovement = 0 (zero-value, never
// moved). Gate condition: 0 < 99 is true ⇒ return early. Pins zero-value.
func TestNpcArriveDelayNoOpWhenNeverMoved(t *testing.T) {
	mn := &mockNpc{lastMovement: 0}
	w := &mockWorld{tick: 100}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_never_moved",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished", state.Execution)
	}
	if len(mn.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want []", mn.setDelayedCalls)
	}
}

// TestNpcArriveDelayRequiresActiveNpc: handler must reject when no
// ActiveNpc bound. Mirrors requireActiveNpc semantics shared by every
// NPC_* reader handler.
func TestNpcArriveDelayRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_no_npc",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally left nil.
	state.World = &mockWorld{tick: 100}

	err := Execute(state)
	if err == nil || err.Error() != "NPC_ARRIVEDELAY: no active npc" {
		t.Errorf("expected 'NPC_ARRIVEDELAY: no active npc', got %v", err)
	}
}

// TestNpcArriveDelayRequiresWorld: handler reads s.World.CurrentTick() to
// evaluate its gate; missing world must return a clean error rather than
// nil-deref. Defensive guard mirrors P_ARRIVEDELAY's sibling-handler
// convention (DEVIATION-NAI-125-D1; goscape defensive; TS skips this check).
func TestNpcArriveDelayRequiresWorld(t *testing.T) {
	mn := &mockNpc{lastMovement: 101}
	sf := &ScriptFile{
		Name:    "npc_arrivedelay_no_world",
		Opcodes: []Opcode{OpNpcArriveDelay, OpReturn},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = mn
	// state.World intentionally left nil.

	err := Execute(state)
	if err == nil || err.Error() != "NPC_ARRIVEDELAY: no world" {
		t.Errorf("expected 'NPC_ARRIVEDELAY: no world', got %v", err)
	}
	if len(mn.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (rejection must not mutate)", mn.setDelayedCalls)
	}
}
```

- [ ] **Step 3.3: Verify build passes (tests compile)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/`

Expected: clean exit, no output. The 7 new tests must compile against the T1+T2 mockNpc + interface state.

- [ ] **Step 3.4: Run the new tests — verify all 7 RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcArriveDelay -v`

Expected: all 7 tests FAIL. The error in each will be `script "<name>": no handler for NPC_ARRIVEDELAY (opcode 2502) at pc=0` (because the dispatch table doesn't include the handler yet). Specifically:
- The 5 happy-path / no-op tests fail at the `Execute` call (`t.Fatalf("Execute: <err>")`).
- `TestNpcArriveDelayRequiresActiveNpc` fails because the error message doesn't match `"NPC_ARRIVEDELAY: no active npc"` — it's the unknown-opcode error instead.
- `TestNpcArriveDelayRequiresWorld` fails the same way.

Confirm at least one test output includes the literal string `no handler for NPC_ARRIVEDELAY (opcode 2502) at pc=0`. If a different failure mode appears, stop and investigate before T4.

- [ ] **Step 3.5: Verify other tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run '^Test[^N]|^TestN[^p]|^TestNp[^c]|^TestNpc[^A]'`

Expected: PASS. (Crude regex meaning "any test name not starting with `TestNpcA`"; ensures the 7 new tests are the only failures.)

If that regex is awkward, alternatively:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ 2>&1 | grep -E '^(--- FAIL|FAIL\s|PASS|ok\s)' | head -20`

Expected: only the 7 `TestNpcArriveDelay*` lines under `--- FAIL`; package overall reports `FAIL`.

- [ ] **Step 3.6: Commit**

```bash
git add pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(nai-125): T3 — pin NPC_ARRIVEDELAY behavior (RED)

7-test family covering the TS NpcOps.ts:542-555 contract:
- 3 suspend cases pinning the recency-dependent SetDelayed mapping
  (this tick / last tick → SetDelayed(1); 2 ticks ago → SetDelayed(0))
- 2 no-op cases pinning gate-rejection (3 ticks ago, never moved)
- 2 gate-error cases (no ActiveNpc, no World) pinning the defensive
  guards from DEVIATION-NAI-125-D1 (goscape defensive; TS skips)

All 7 RED with "no handler for NPC_ARRIVEDELAY (opcode 2502)";
T4 lands the handler.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: GREEN — port `handleNpcArriveDelay` + register dispatch entry

**Files:**
- Modify: `pkg/script/handlers_npc.go` (append handler near `handleNpcDelay` at `:313-330`)
- Modify: `pkg/script/handlers.go` (insert dispatch entry near `OpNpcDelay` at `:407`)

**Note on imports:** `pkg/script/handlers_npc.go:4` already imports `"errors"` (existing site at `:154`: `errors.New("NPC_NAME: no configs")`). No import-block edit needed.

- [ ] **Step 4.1: Add `handleNpcArriveDelay` function**

Locate the end of `handleNpcDelay` in `pkg/script/handlers_npc.go` (currently line 330):

```go
func handleNpcDelay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DELAY"); err != nil {
		return err
	}
	ticks := s.PopInt()
	if err := checkNotNull(ticks, "NPC_DELAY"); err != nil {
		return err
	}
	s.ActiveNpc.SetDelayed(ticks)
	s.Execution = NpcSuspended
	return nil
}
```

Insert immediately after (before `handleNpcQueue`):

```go
// handleNpcArriveDelay implements NPC_ARRIVEDELAY (opcode 2502): if the
// active NPC has moved within the past 3 ticks (this tick, last tick, or
// 2 ticks ago), suspend the script with a delay computed from the
// movement recency; otherwise no-op. TS NpcOps.ts:542-555.
//
// The 3-tick window arises from the TS lastMovement contract (written
// to currentTick + 1 after a moving tick): the gate accepts moves from
// this tick (lastMovement = T+1), last tick (lastMovement = T), and
// 2 ticks ago (lastMovement = T-1) but rejects moves from 3+ ticks ago
// (lastMovement <= T-2; T-2 < T-1 ⇒ return).
//
// Inner branch: if NPC moved 2 ticks ago (lastMovement = T-1), suspend
// for 1 tick (TS delayedUntil = T+1 ⇒ goscape SetDelayed(0)). Otherwise
// (this tick or last tick), suspend for 2 ticks (TS delayedUntil = T+2
// ⇒ goscape SetDelayed(1)). The +1 offset comes from goscape's
// SetDelayed(ticks) writing delayedUntil = currentTick + 1 + ticks
// (npc.go:323-326).
//
// Vs P_ARRIVEDELAY (handlers.go:739): NPC variant has a 3-tick window
// (vs 2) and a recency-dependent suspend duration (vs always 1 tick),
// per TS NpcOps.ts asymmetry vs PlayerOps.ts:357-366.
//
// DEVIATION-NAI-125-D1: s.World == nil defensive guard (goscape
// defensive; TS skips this check). Mirrors handlePArriveDelay /
// handleMapClock / handlePlayerCount sibling-handler convention.
func handleNpcArriveDelay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_ARRIVEDELAY"); err != nil {
		return err
	}
	if s.World == nil {
		return errors.New("NPC_ARRIVEDELAY: no world")
	}
	last := s.ActiveNpc.LastMovement()
	tick := s.World.CurrentTick()
	if last < tick-1 {
		return nil
	}
	if last == tick-1 {
		s.ActiveNpc.SetDelayed(0) // delayedUntil = T+1
	} else {
		s.ActiveNpc.SetDelayed(1) // delayedUntil = T+2
	}
	s.Execution = NpcSuspended
	return nil
}
```

- [ ] **Step 4.2: Register dispatch entry**

Locate the alphabetic-by-name NPC mutating-ops block in `pkg/script/handlers.go:402-414` (verified at HEAD `48e95ab`):

```go
	// S6c: NPC mutating ops batch.
	OpNpcAnim:              handleNpcAnim,
	OpNpcChangeType:        handleNpcChangeType,
	OpNpcChangeTypeKeepAll: handleNpcChangeTypeKeepAll,
	OpNpcDamage:            handleNpcDamage,
	OpNpcDelay:             handleNpcDelay,
	OpNpcFaceSquare:        handleNpcFaceSquare,
```

Insert `OpNpcArriveDelay: handleNpcArriveDelay,` between `OpNpcAnim` and `OpNpcChangeType` (alphabetic placement: An < Ar < Ch). Match the existing column alignment of the value column (the table uses tab-aligned `:` followed by space-padding to the value column):

```go
	// S6c: NPC mutating ops batch.
	OpNpcAnim:              handleNpcAnim,
	OpNpcArriveDelay:       handleNpcArriveDelay,
	OpNpcChangeType:        handleNpcChangeType,
	OpNpcChangeTypeKeepAll: handleNpcChangeTypeKeepAll,
	OpNpcDamage:            handleNpcDamage,
	OpNpcDelay:             handleNpcDelay,
	OpNpcFaceSquare:        handleNpcFaceSquare,
```

If `gofmt` re-aligns the columns after `go build`, that's fine — the build step rewrites alignment automatically. Do not hand-tweak.

- [ ] **Step 4.3: Verify build passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean exit, no output.

- [ ] **Step 4.4: Verify go vet passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean exit, no output.

- [ ] **Step 4.5: Run the 7 NPC_ARRIVEDELAY tests — verify GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcArriveDelay -v`

Expected: all 7 tests PASS:

```
=== RUN   TestNpcArriveDelaySuspendsWhenMovedThisTick
--- PASS: TestNpcArriveDelaySuspendsWhenMovedThisTick (0.00s)
=== RUN   TestNpcArriveDelaySuspendsWhenMovedLastTick
--- PASS: TestNpcArriveDelaySuspendsWhenMovedLastTick (0.00s)
=== RUN   TestNpcArriveDelaySuspendsWhenMovedTwoTicksAgo
--- PASS: TestNpcArriveDelaySuspendsWhenMovedTwoTicksAgo (0.00s)
=== RUN   TestNpcArriveDelayNoOpWhenMovedThreeTicksAgo
--- PASS: TestNpcArriveDelayNoOpWhenMovedThreeTicksAgo (0.00s)
=== RUN   TestNpcArriveDelayNoOpWhenNeverMoved
--- PASS: TestNpcArriveDelayNoOpWhenNeverMoved (0.00s)
=== RUN   TestNpcArriveDelayRequiresActiveNpc
--- PASS: TestNpcArriveDelayRequiresActiveNpc (0.00s)
=== RUN   TestNpcArriveDelayRequiresWorld
--- PASS: TestNpcArriveDelayRequiresWorld (0.00s)
PASS
ok  	github.com/zsrv/goscape/pkg/script ...
```

- [ ] **Step 4.6: Run full test suite — verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all packages PASS. Pre-existing modernization warnings (S1001 / minmax / rangeint per NAI-124 close memo at `nai_followups.md:6313`) are NOT introduced by this bundle — confirm any failures pre-date `48e95ab` via `git stash && go test ./... && git stash pop` if uncertain.

- [ ] **Step 4.7: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(nai-125): T4 — port NPC_ARRIVEDELAY handler per NpcOps.ts:542-555

Implements opcode 2502 with the TS 3-tick window and recency-dependent
suspend mapping:
- lastMovement = currentTick + 1 (this tick) → SetDelayed(1)
- lastMovement = currentTick (last tick) → SetDelayed(1)
- lastMovement = currentTick - 1 (2 ticks ago) → SetDelayed(0)
- lastMovement <= currentTick - 2 → no-op

DEVIATION-NAI-125-D1 declared in handler doc-comment: nil-World
defensive guard (goscape defensive; TS skips this check), mirroring
handlePArriveDelay sibling-handler convention.

Closes the [proc,npc_death] WARN at pc=4 surfaced in NAI-123 close
smoke (5687f4e).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Implementer-side close

After T4 commits cleanly, the implementer's job is done. The controller will:

1. Run a Sonnet code-reviewer pass per `superpowers_code_reviewer_model`.
2. Apply any reviewer-fix sub-commit if needed.
3. Hand off to the user for smoke per `smoke_test_server_handoff` (Tutorial Island fresh-char-vs-giant-rat: confirm `WARN: no handler for NPC_ARRIVEDELAY` is gone).
4. Apply NAI-125 close commit with `Closes memory:` trailer per `close_commit_memory_trailer`.
5. Route any newly-surfaced cascade symptoms per `smoke_surfaces_adjacent_divergences` (≤30 LOC = in-scope-stretch; >30 LOC = NAI-126 candidate).

These steps are NOT part of the implementer's task list — they are controller-orchestrated.

---

## Verification commands cheat sheet

Single-suite run during iteration:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcArriveDelay -v
```

Full suite + vet + build (controller-preflight per task close):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Worktree-stray check (per `feedback_subagent_wt_path`):

```bash
git status
```
