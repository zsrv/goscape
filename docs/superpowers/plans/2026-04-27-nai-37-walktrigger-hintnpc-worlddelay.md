# NAI-37 — NPC_WALKTRIGGER + HINT_NPC + WORLD_DELAY (with full world-script-queue infrastructure) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port three smoke-surfaced runscript opcodes (NPC_WALKTRIGGER, HINT_NPC, WORLD_DELAY) and ship the foundational world-script-queue infrastructure that activates the long-staged `WorldSuspended` execution constant declared since NAI-S1.

**Architecture:** Three independent bundles with no inter-bundle dependencies. B1 ports a state-only NPC field-write opcode. B2 ports a fire-and-forget player-bound packet (HintArrow type=1 only). B3 ports a state-machine-transition opcode that requires a new `worldScriptQueue` on `*Server`, a `processWorldQueue` tick step, three producer wirings (player path, npc path, world self-loop), and a `resumeOrFinishWorld` dispatch helper. All three bundles follow strict TS fidelity per `true_to_ts_gate.md`.

**Tech Stack:** Go 1.26+ (per `go_version.md`), TS source canonical path `LostCityRS/Engine-TS` (per `ts_source_canonical_path.md`).

**Spec:** `docs/superpowers/specs/2026-04-27-nai-37-walktrigger-hintnpc-worlddelay-design.md` (commit `75a737d`).

**HEAD baseline:** `82c2173` (NAI-36 close).

---

## Bundle 1: NPC_WALKTRIGGER (opcode 2545)

### Task 1: Add walktrigger fields to *Npc, ActiveNpc setter methods, mockNpc recorder

**Files:**
- Modify: `modules/world/npc.go` (add 2 fields to Npc struct, add `walktrigger: -1` default in NewNpc)
- Modify: `modules/world/npc_script.go` (add SetWalkTrigger + SetWalkTriggerArg methods on *Npc)
- Modify: `pkg/script/active.go` (add 2 methods to ActiveNpc interface)
- Modify: `pkg/script/handlers_npc_test.go` (add 2 recorder fields + 2 method impls to mockNpc)

- [ ] **Step 1: Read the Npc struct body to find the right field-insertion site**

Run: `grep -n "^type Npc struct\|^}" /home/owner/Code/github.com/zsrv/goscape/modules/world/npc.go | head -10`

Expected: locate the type Npc struct boundaries. Insert new fields near other `int` fields (e.g., near `nextPatrolPoint`).

- [ ] **Step 2: Add walktrigger fields to Npc struct**

In `modules/world/npc.go`, inside the `type Npc struct` block, add (placement near other "deferred trigger" / scheduling fields; choose a sensible neighbor like `nextPatrolPoint`):

```go
	// walktrigger queues a deferred AI-queue trigger (0..19, -1 = unset)
	// to fire when this NPC completes a walk step. Written by the
	// NPC_WALKTRIGGER (opcode 2545) handler. NOT YET CONSUMED — the
	// AI-tick walktrigger consumption is tracked deviation
	// NAI-37-D-WALKTRIGGER-NOREADER. Mirrors TS Npc.walktrigger.
	walktrigger    int
	walktriggerArg int
```

- [ ] **Step 3: Default walktrigger to -1 in NewNpc**

In `modules/world/npc.go`, `NewNpc` constructor (currently around line 130-183), add to the `&Npc{...}` initializer (alongside other `-1`-default fields like `walkDir`, `runDir`, `waypointIndex`):

```go
		walktrigger:    -1,
		walktriggerArg: 0,
```

Note: walktriggerArg defaults to 0 because that's a valid runtime value (script can pass arg=0). Only walktrigger needs the -1 sentinel.

Also add a doc-comment noting the partial coverage — existing `&Npc{...}` literals in test files default to `walktrigger=0` which is benign in NAI-37 (no reader). When the AI-tick consumer is ported, every literal must be audited per `plan_enumerate_struct_literals.md`.

- [ ] **Step 4: Add SetWalkTrigger and SetWalkTriggerArg methods on *Npc**

In `modules/world/npc_script.go`, near other simple field-setter methods (e.g., near `SetTimer`, `SetHuntRange`):

```go
// SetWalkTrigger sets the deferred AI-queue trigger index (0..19) that
// fires when this NPC completes a walk step. Called by the
// NPC_WALKTRIGGER handler (handlers_npc.go) after queueID validation.
// Mirrors TS Npc.walktrigger field write at NpcOps.ts:488.
func (n *Npc) SetWalkTrigger(queueID int) { n.walktrigger = queueID }

// SetWalkTriggerArg sets the arg passed to the walktrigger script when
// it eventually fires. Mirrors TS Npc.walktriggerArg field write at
// NpcOps.ts:489.
func (n *Npc) SetWalkTriggerArg(arg int) { n.walktriggerArg = arg }
```

- [ ] **Step 5: Extend ActiveNpc interface**

In `pkg/script/active.go`, inside `type ActiveNpc interface { ... }` (currently around line 410-580), add (alongside other simple setters like `SetTimer`, `SetHuntRange`):

```go
	// SetWalkTrigger sets the deferred AI-queue trigger index for the
	// active NPC. Called by NPC_WALKTRIGGER (opcode 2545). Range
	// validation [1, 20] happens in the handler before -1 transform;
	// this method just writes the field. Mirrors TS Npc.walktrigger
	// at NpcOps.ts:488.
	SetWalkTrigger(queueID int)

	// SetWalkTriggerArg sets the arg passed to the walktrigger script
	// when it eventually fires. Mirrors TS Npc.walktriggerArg at
	// NpcOps.ts:489.
	SetWalkTriggerArg(arg int)
```

- [ ] **Step 6: Extend mockNpc with recorder fields and method implementations**

In `pkg/script/handlers_npc_test.go`, inside `type mockNpc struct` (currently at line 186-214), add to the field list (near `setHuntModeCalls`):

```go
	walkTriggerCalls    []int
	walkTriggerArgCalls []int
```

After the mockNpc method block (which currently ends near `Teleport` at line 299), add:

```go
func (m *mockNpc) SetWalkTrigger(queueID int) {
	m.walkTriggerCalls = append(m.walkTriggerCalls, queueID)
}

func (m *mockNpc) SetWalkTriggerArg(arg int) {
	m.walkTriggerArgCalls = append(m.walkTriggerArgCalls, arg)
}
```

- [ ] **Step 7: Build verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean build. Compile errors here mean either the ActiveNpc interface is now broader than mockNpc covers (some other test file mocks ActiveNpc and needs the same 2 methods added), or the *Npc world-side type doesn't satisfy ActiveNpc post-extension. Both are catchable by the build.

- [ ] **Step 8: Vet verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean.

- [ ] **Step 9: Run full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all 23 packages green. No tests should break — the new fields default to safe values, the new interface methods exist on both implementations, and no existing test exercises walktrigger.

- [ ] **Step 10: Commit**

```bash
git add modules/world/npc.go modules/world/npc_script.go pkg/script/active.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "feat(world,script): NAI-37 T1 — walktrigger fields + ActiveNpc setters

Add walktrigger and walktriggerArg fields to *Npc with -1 sentinel
default in NewNpc. Add SetWalkTrigger and SetWalkTriggerArg setter
methods on *Npc and to the ActiveNpc interface. Extend mockNpc with
matching recorder fields.

Foundation for NAI-37 Bundle 1 (NPC_WALKTRIGGER opcode 2545 handler
in T2). No reader for walktrigger fields at this commit — tracked
deviation NAI-37-D-WALKTRIGGER-NOREADER closes when the AI-tick
walktrigger consumption is ported."
```

---

### Task 2: NPC_WALKTRIGGER handler + registration

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcWalkTrigger`)
- Modify: `pkg/script/handlers.go` (register the dispatch entry)

- [ ] **Step 1: Write the failing test**

In `pkg/script/handlers_npc_test.go`, append after the existing NPC handler tests:

```go
// --- NAI-37 Task 2: NPC_WALKTRIGGER handler unit tests ---------------------

func TestNpcWalkTrigger_NoActiveNpc_Errors(t *testing.T) {
	s := &ScriptState{} // no ActiveNpc set
	s.PushInt(5)        // arg
	s.PushInt(3)        // queueID
	if err := handleNpcWalkTrigger(s); err == nil {
		t.Fatalf("expected error for no active npc")
	}
}

func TestNpcWalkTrigger_QueueIDBelowOne_Errors(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{ActiveNpc: npc}
	s.PushInt(5) // arg
	s.PushInt(0) // queueID = 0 → invalid
	if err := handleNpcWalkTrigger(s); err == nil {
		t.Fatalf("expected error for queueID=0")
	}
	if len(npc.walkTriggerCalls) != 0 {
		t.Errorf("walkTriggerCalls: got %d writes, want 0 on validation failure",
			len(npc.walkTriggerCalls))
	}
}

func TestNpcWalkTrigger_QueueIDAboveTwenty_Errors(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{ActiveNpc: npc}
	s.PushInt(5)  // arg
	s.PushInt(21) // queueID = 21 → invalid
	if err := handleNpcWalkTrigger(s); err == nil {
		t.Fatalf("expected error for queueID=21")
	}
}

func TestNpcWalkTrigger_PopOrderAndTransform(t *testing.T) {
	npc := &mockNpc{}
	s := &ScriptState{ActiveNpc: npc}
	s.PushInt(7)  // queueID (pushed first → bottom of stack)
	s.PushInt(42) // arg (pushed second → top of stack)
	if err := handleNpcWalkTrigger(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Pop order: arg (top) first, queueID (next) second.
	// Then queueID-1 transform: 7 → 6.
	if want := []int{6}; !equalIntSlice(npc.walkTriggerCalls, want) {
		t.Errorf("walkTriggerCalls: got %v, want %v", npc.walkTriggerCalls, want)
	}
	if want := []int{42}; !equalIntSlice(npc.walkTriggerArgCalls, want) {
		t.Errorf("walkTriggerArgCalls: got %v, want %v", npc.walkTriggerArgCalls, want)
	}
}

func TestNpcWalkTrigger_BoundaryQueueIDs(t *testing.T) {
	t.Run("queueID=1", func(t *testing.T) {
		npc := &mockNpc{}
		s := &ScriptState{ActiveNpc: npc}
		s.PushInt(1) // queueID
		s.PushInt(0) // arg
		if err := handleNpcWalkTrigger(s); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []int{0}; !equalIntSlice(npc.walkTriggerCalls, want) {
			t.Errorf("queueID=1 → walktrigger=0 (queueID-1); got %v", npc.walkTriggerCalls)
		}
	})
	t.Run("queueID=20", func(t *testing.T) {
		npc := &mockNpc{}
		s := &ScriptState{ActiveNpc: npc}
		s.PushInt(20) // queueID
		s.PushInt(0)  // arg
		if err := handleNpcWalkTrigger(s); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []int{19}; !equalIntSlice(npc.walkTriggerCalls, want) {
			t.Errorf("queueID=20 → walktrigger=19 (queueID-1); got %v", npc.walkTriggerCalls)
		}
	})
}
```

The test uses an `equalIntSlice` helper. Verify it already exists in `pkg/script/handlers_npc_test.go` or a sibling test file:

Run: `grep -n "func equalIntSlice" /home/owner/Code/github.com/zsrv/goscape/pkg/script/`

Expected: locate the existing helper. If it doesn't exist, add it to the test file:

```go
func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcWalkTrigger -v`

Expected: FAIL with "undefined: handleNpcWalkTrigger" (the handler doesn't exist yet).

- [ ] **Step 3: Write the handler**

In `pkg/script/handlers_npc.go`, append after `handleNpcWalk` (which is currently around line 379-394):

```go
// handleNpcWalkTrigger (NPC_WALKTRIGGER, opcode 2545) sets a deferred
// AI-queue trigger and arg on the active NPC; the trigger fires when
// the NPC completes a walk step. Pop order: arg (top), queueID
// (bottom). queueID ∈ [1, 20] mirrors TS QueueValid range, transformed
// to [0, 19] via queueID-1 to match TS NpcOps.ts:488 storage. Mirrors
// TS NpcOps.ts:483-490.
//
// NOT YET CONSUMED at NAI-37: the AI-tick walktrigger consumption that
// reads these fields and fires the queued script when the NPC completes
// a walk step is tracked deviation NAI-37-D-WALKTRIGGER-NOREADER.
func handleNpcWalkTrigger(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_WALKTRIGGER"); err != nil {
		return err
	}
	arg := s.PopInt()
	queueID := s.PopInt()
	if queueID < 1 || queueID > 20 {
		return fmt.Errorf("NPC_WALKTRIGGER: invalid queueId %d (want 1..20)", queueID)
	}
	s.ActiveNpc.SetWalkTrigger(queueID - 1)
	s.ActiveNpc.SetWalkTriggerArg(arg)
	return nil
}
```

- [ ] **Step 4: Register the handler**

In `pkg/script/handlers.go`, locate the dispatch table (a map keyed on `Opcode`). Find the existing `OpNpcWalk: handleNpcWalk` registration and add nearby (in opcode-numerical order to match the file's existing convention):

```go
	OpNpcWalkTrigger:     handleNpcWalkTrigger,
```

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcWalkTrigger -v`

Expected: PASS for all 5 test cases.

- [ ] **Step 6: Run full package test to catch regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ ./modules/world/`

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "feat(script): NAI-37 T2 — NPC_WALKTRIGGER handler (opcode 2545)

Pop order: arg (top), queueID (bottom). queueID ∈ [1, 20] inline
range-checked then transformed to [0, 19] via queueID-1 before
ActiveNpc.SetWalkTrigger/SetWalkTriggerArg writes. Mirrors TS
NpcOps.ts:483-490.

5 unit tests pinning: no-active-npc rejection, queueID below/above
range, pop-order + queueID-1 transform together, boundary IDs 1+20.

Bundle 1 (NPC_WALKTRIGGER) complete."
```

---

### Task 3: Entity-side round-trip test for SetWalkTrigger

**Files:**
- Modify: `modules/world/npc_script_test.go` (add round-trip test)

- [ ] **Step 1: Write the failing test**

In `modules/world/npc_script_test.go`, append:

```go
// --- NAI-37 Task 3: walktrigger field-write round-trip ---------------------

func TestNpcSetWalkTriggerFieldWrites(t *testing.T) {
	n := NewNpc(0, 0, 3200, 3200, 0, &objtype.NpcType{})
	if got := n.walktrigger; got != -1 {
		t.Errorf("NewNpc walktrigger default: got %d, want -1 (unset sentinel)", got)
	}
	n.SetWalkTrigger(7)
	if got := n.walktrigger; got != 7 {
		t.Errorf("SetWalkTrigger(7): got walktrigger=%d, want 7", got)
	}
	n.SetWalkTriggerArg(42)
	if got := n.walktriggerArg; got != 42 {
		t.Errorf("SetWalkTriggerArg(42): got walktriggerArg=%d, want 42", got)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcSetWalkTriggerFieldWrites -v`

Expected: PASS. The test pins the NewNpc default sentinel AND both setter methods in one round-trip.

- [ ] **Step 3: Commit**

```bash
git add modules/world/npc_script_test.go
git commit --no-gpg-sign -m "test(world): NAI-37 T3 — walktrigger field-write round-trip pin

Pin NewNpc walktrigger default = -1 (unset sentinel) and both
SetWalkTrigger/SetWalkTriggerArg writes. Closes Bundle 1 entity-side
test surface."
```

---

## Bundle 2: HINT_NPC (opcode 2028)

### Task 4: OpHintArrow protocol declaration + Nid() accessor on ActiveNpc

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go` (add OpHintArrow)
- Modify: `pkg/script/active.go` (add Nid() to ActiveNpc interface)
- Modify: `modules/world/npc_script.go` (add (n *Npc).Nid() impl)
- Modify: `pkg/script/handlers_npc_test.go` (add Nid() to mockNpc + nid field)

- [ ] **Step 1: Add OpHintArrow to prot.go**

In `pkg/io/protocol/game/server/prot.go`, locate a sensible insertion point (near other player-bound modal/UI ops). Add:

```go
	// HINT_ARROW — directs the client to render a hint indicator pointing
	// at an NPC, player, tile, or to clear (one of 5 type variants in
	// TS HintArrowEncoder; goscape ships only the type=1 NPC variant
	// at NAI-37 — tracked deviation NAI-37-D-HINTARROW-PARTIAL-ENCODER).
	// TS ServerGameProt.HINT_ARROW = (25, 6).
	OpHintArrow = Op{Opcode: 25, PayloadSize: 6}
```

- [ ] **Step 2: Add Nid() to ActiveNpc interface**

In `pkg/script/active.go`, inside `type ActiveNpc interface { ... }`, add (alongside other simple read accessors like `NpcUID`):

```go
	// Nid returns the NPC slot id (the low 16 bits of NpcUID). Used by
	// NPC-targeted player-bound packets like HintArrow that reference
	// the NPC by slot rather than by packed UID.
	Nid() int
```

- [ ] **Step 3: Add (n *Npc).Nid() impl on world-side Npc**

In `modules/world/npc_script.go`, near `NpcUID` (currently around line 28-29):

```go
// Nid implements script.ActiveNpc.Nid by returning n.nid (the slot id,
// low 16 bits of UID). Used by NPC-targeted player-bound packets.
func (n *Npc) Nid() int { return n.nid }
```

- [ ] **Step 4: Add Nid() and nid field to mockNpc**

In `pkg/script/handlers_npc_test.go`, inside `type mockNpc struct`:

```go
	nid int
```

And after the existing simple accessors (e.g., `NpcUID`):

```go
func (m *mockNpc) Nid() int { return m.nid }
```

- [ ] **Step 5: Build and vet verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean. If any other ActiveNpc mock exists in the codebase (sibling test file), it'll surface as a compile error here. Search for additional mocks:

Run: `grep -rln "func .*) NpcUID() int" /home/owner/Code/github.com/zsrv/goscape/`

Expected: list every mock that implements ActiveNpc — each needs `Nid() int` added. If any are missing, add them following the same `func (m *mockX) Nid() int { return m.nid }` shape.

- [ ] **Step 6: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go pkg/script/active.go modules/world/npc_script.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "feat(io,world,script): NAI-37 T4 — OpHintArrow + ActiveNpc.Nid

Add OpHintArrow (opcode 25, payload 6) to server protocol matching
TS ServerGameProt.HINT_ARROW. Add Nid() accessor to ActiveNpc
interface and (n *Npc) implementation; extend mockNpc.

Foundation for HINT_NPC handler (opcode 2028) in T6 — handler will
call s.ActiveNpc.Nid() to extract the slot id for the type=1
HintArrow encoder branch."
```

---

### Task 5: (*Player).HintNpc method + byte-pin test + ActivePlayer interface extension

**Files:**
- Modify: `pkg/script/active.go` (add HintNpc to ActivePlayer interface)
- Modify: `modules/world/player_script.go` (add HintNpc method)
- Modify: `modules/world/player_script_test.go` (add byte-pin test)
- Modify: `pkg/script/runner_test.go` (extend mockPlayer with hintNpcCalls)

- [ ] **Step 1: Add HintNpc to ActivePlayer interface**

In `pkg/script/active.go`, inside `type ActivePlayer interface { ... }` (around line 6+), add (alongside other player-bound packet writers like `CamReset`):

```go
	// HintNpc directs the client to render a hint arrow pointing at the
	// NPC with the given nid (slot id). Mirrors TS Player.hintNpc at
	// Player.ts:2174-2176, which writes a HintArrow(type=1) packet.
	// Called by the HINT_NPC (opcode 2028) handler.
	HintNpc(nid int)
```

- [ ] **Step 2: Extend mockPlayer with hintNpcCalls field**

In `pkg/script/runner_test.go`, inside `type mockPlayer struct` (currently at line 95+), add to the field list (near other recorder fields like `lastReadyAnim`, `messages`):

```go
	hintNpcCalls []int
```

After the mockPlayer method block (find a sibling like `CamReset`'s mock impl), add:

```go
func (m *mockPlayer) HintNpc(nid int) { m.hintNpcCalls = append(m.hintNpcCalls, nid) }
```

If the existing mockPlayer in runner_test.go doesn't already have a `CamReset() { m.camResetCalls++ }`, locate where simple void methods are defined and add HintNpc with the same style.

- [ ] **Step 3: Write the failing byte-pin test**

In `modules/world/player_script_test.go`, append:

```go
// --- NAI-37 Task 5: HintNpc payload byte-pin test --------------------------

// TestHintNpc_PayloadBytes pins the type=1 HintArrow encoder branch
// byte-for-byte. nid=0x1234 chosen so each byte position is
// distinguishable from the zero-fill (catches type-byte regression,
// nid endianness, and field misordering — see rsbuf_roundtrip_tests.md).
func TestHintNpc_PayloadBytes(t *testing.T) {
	p := newTestPlayer(t)
	p.HintNpc(0x1234)
	got := drainPlayerOutputForOp(t, p, gameserver.OpHintArrow)
	want := []byte{0x01, 0x12, 0x34, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("HintNpc(0x1234) payload: got %#x, want %#x", got, want)
	}
}
```

The test references `newTestPlayer` and `drainPlayerOutputForOp`. Verify these exist:

Run: `grep -n "func newTestPlayer\|func drainPlayerOutputForOp" /home/owner/Code/github.com/zsrv/goscape/modules/world/`

Expected: locate existing test helpers used by sibling byte-pin tests. If only one or the other exists, follow the established pattern. If neither exists, the test must be reframed to use the actual existing test infrastructure for capturing player wire output. Search for existing examples:

Run: `grep -rn "OpCamReset.*payload\|TestCamReset" /home/owner/Code/github.com/zsrv/goscape/modules/world/`

Expected: locate sibling tests (e.g., for CamReset) and mirror their setup pattern. The test must end with a 6-byte assertion against the type=1 HintArrow encoder shape `[0x01, p2(nid), p2(0), p1(0)]`.

If no sibling pattern exists, the test setup must:
1. Construct a *Player with a working `bufw` write buffer (mirror NewPlayer-style construction in tests).
2. Call `p.HintNpc(0x1234)`.
3. Read back from the Player's output buffer the opcode byte (`25` = OpHintArrow.Opcode) followed by the 6-byte payload.
4. Assert payload equals `[0x01, 0x12, 0x34, 0x00, 0x00, 0x00]`.

- [ ] **Step 4: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHintNpc_PayloadBytes -v`

Expected: FAIL with "undefined: HintNpc" or similar (method not yet implemented on *Player).

- [ ] **Step 5: Write the (*Player).HintNpc method**

In `modules/world/player_script.go`, near `CamReset` (currently around line 143-147), add:

```go
// HintNpc sends a HINT_ARROW (type=1, NPC variant) wire packet to the
// client. Encodes 6 bytes matching TS HintArrowEncoder type=1 branch:
// p1(type=1), p2(nid), p2(0), p1(0). Called by the HINT_NPC (opcode
// 2028) script handler. Mirrors TS Player.hintNpc at Player.ts:2174-2176.
//
// DEVIATION NAI-37-D-HINTARROW-PARTIAL-ENCODER: only the type=1 branch
// of TS HintArrowEncoder is implemented. Closure when HINT_PL,
// HINT_COORD, HINT_STOP handlers and their respective encoder branches
// land.
func (p *Player) HintNpc(nid int) {
	payload := []byte{
		0x01,                  // p1: type = 1 (NPC hint)
		byte(nid >> 8), byte(nid), // p2: nid (big-endian)
		0x00, 0x00,            // p2: 0 (unused playerSlot for type=1)
		0x00,                  // p1: 0 (unused y for type=1)
	}
	p.writeOut(gameserver.OpHintArrow, payload)
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHintNpc_PayloadBytes -v`

Expected: PASS. The 6-byte payload matches `[0x01, 0x12, 0x34, 0x00, 0x00, 0x00]`.

- [ ] **Step 7: Build and vet to confirm no other ActivePlayer mock breaks**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean. If any sibling mockPlayer exists outside runner_test.go that implements ActivePlayer, it'll surface here as a missing-method compile error and needs `HintNpc(nid int)` added similarly.

- [ ] **Step 8: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green.

- [ ] **Step 9: Commit**

```bash
git add pkg/script/active.go modules/world/player_script.go modules/world/player_script_test.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "feat(world,script): NAI-37 T5 — (*Player).HintNpc + byte-pin test

Implement (p *Player).HintNpc(nid int) emitting the 6-byte type=1
HintArrow encoder branch [0x01, p2(nid), p2(0), p1(0)] via
p.writeOut(gameserver.OpHintArrow, payload). Mirrors TS
Player.hintNpc at Player.ts:2174-2176.

Extend ActivePlayer interface with HintNpc(nid int). Extend mockPlayer
with hintNpcCalls recorder.

Byte-pin test uses nid=0x1234 (every byte distinguishable from
zero-fill per rsbuf_roundtrip_tests.md) — pins type byte, BE
endianness, and zero-fill positions in one assertion."
```

---

### Task 6: HINT_NPC handler + registration + handler tests

**Files:**
- Modify: `pkg/script/handlers_player.go` (add `handleHintNpc`)
- Modify: `pkg/script/handlers.go` (register the dispatch entry)
- Modify: `pkg/script/handlers_player_test.go` (add 3 handler unit tests)

- [ ] **Step 1: Write the failing tests**

In `pkg/script/handlers_player_test.go`, append:

```go
// --- NAI-37 Task 6: HINT_NPC handler unit tests ---------------------------

func TestHintNpc_NoActivePlayer_Errors(t *testing.T) {
	npc := &mockNpc{nid: 42}
	s := &ScriptState{ActiveNpc: npc} // no Self
	if err := handleHintNpc(s); err == nil {
		t.Fatalf("expected error for no active player")
	}
}

func TestHintNpc_NoActiveNpc_Errors(t *testing.T) {
	pl := &mockPlayer{}
	s := &ScriptState{Self: pl} // no ActiveNpc
	if err := handleHintNpc(s); err == nil {
		t.Fatalf("expected error for no active npc")
	}
	if len(pl.hintNpcCalls) != 0 {
		t.Errorf("hintNpcCalls: got %d, want 0 on validation failure",
			len(pl.hintNpcCalls))
	}
}

func TestHintNpc_Success_RecordsNid(t *testing.T) {
	pl := &mockPlayer{}
	npc := &mockNpc{nid: 4242}
	s := &ScriptState{Self: pl, ActiveNpc: npc}
	if err := handleHintNpc(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []int{4242}; !equalIntSlice(pl.hintNpcCalls, want) {
		t.Errorf("hintNpcCalls: got %v, want %v", pl.hintNpcCalls, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHintNpc -v`

Expected: FAIL with "undefined: handleHintNpc".

- [ ] **Step 3: Write the handler**

In `pkg/script/handlers_player.go`, append (location: near other simple ActivePlayer + ActiveNpc dual-pointer handlers; if none exists, place at end of file):

```go
// handleHintNpc (HINT_NPC, opcode 2028) sends a HintArrow type=1 wire
// packet to the active player, pointing at the active NPC. Mirrors TS
// PlayerOps.ts:972-974:
//
//	state.activePlayer.hintNpc(state.activeNpc.nid)
//
// DEVIATION NAI-37-D-HINTARROW-PARTIAL-ENCODER: only the type=1 (NPC)
// hint variant is wired. HINT_PL, HINT_COORD, HINT_STOP handlers
// land in a future sub-spec.
func handleHintNpc(s *ScriptState) error {
	if err := requireActivePlayer(s, "HINT_NPC"); err != nil {
		return err
	}
	if err := requireActiveNpc(s, "HINT_NPC"); err != nil {
		return err
	}
	s.Self.HintNpc(s.ActiveNpc.Nid())
	return nil
}
```

- [ ] **Step 4: Register the handler**

In `pkg/script/handlers.go`, locate `OpHintNpc` placement (numerical neighbor of opcode 2028; sibling with other HINT_* future entries near opcode 2027/2029/2030 if those exist). Add:

```go
	OpHintNpc:        handleHintNpc,
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHintNpc -v`

Expected: PASS for all 3 cases.

- [ ] **Step 6: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(script): NAI-37 T6 — HINT_NPC handler (opcode 2028)

requireActivePlayer + requireActiveNpc + s.Self.HintNpc(s.ActiveNpc.Nid()).
Mirrors TS PlayerOps.ts:972-974 verbatim.

3 unit tests pinning: no-active-player rejection, no-active-npc
rejection, success records nid.

Bundle 2 (HINT_NPC) complete."
```

---

## Bundle 3: WORLD_DELAY infrastructure (opcode 1021)

### Task 7: handleWorldDelay handler + registration + handler test

**Files:**
- Create or modify: `pkg/script/handlers_server.go` (add `handleWorldDelay`; if file doesn't exist, create it)
- Modify: `pkg/script/handlers.go` (register)
- Modify: `pkg/script/handlers_server_test.go` (add handler unit test; create file if needed)

- [ ] **Step 1: Verify whether handlers_server.go exists**

Run: `ls /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_server*.go`

Expected: list any existing server-grouped handler files. If `handlers_server.go` exists, append to it. If not, the closest sibling (e.g., `handlers_map.go` for ServerOps-derived handlers) is the right home.

- [ ] **Step 2: Write the failing test**

Determine the test file: if `handlers_server_test.go` exists, use it. Otherwise pick the sibling matching the production file from Step 1.

Append:

```go
// --- NAI-37 Task 7: WORLD_DELAY handler unit test --------------------------

func TestWorldDelay_SetsExecutionWorldSuspendedAndDoesNotPop(t *testing.T) {
	s := &ScriptState{}
	s.PushInt(999)
	s.PushInt(42)

	startStackLen := len(s.IntStack)
	if err := handleWorldDelay(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := s.Execution, WorldSuspended; got != want {
		t.Errorf("Execution: got %v, want %v", got, want)
	}
	if got, want := len(s.IntStack), startStackLen; got != want {
		t.Errorf("IntStack length: got %d, want %d (handler must not pop)", got, want)
	}
	// Verify exact stack contents (top-of-stack is at the highest index).
	if want := 42; s.IntStack[len(s.IntStack)-1] != want {
		t.Errorf("top-of-stack: got %d, want %d", s.IntStack[len(s.IntStack)-1], want)
	}
	if want := 999; s.IntStack[len(s.IntStack)-2] != want {
		t.Errorf("bottom int: got %d, want %d", s.IntStack[len(s.IntStack)-2], want)
	}
}
```

The test references `s.IntStack`. Verify the field name on ScriptState:

Run: `grep -n "IntStack\|intStack" /home/owner/Code/github.com/zsrv/goscape/pkg/script/state.go | head -5`

Expected: locate the actual field name (likely `IntStack` or `IntStackTop` or similar). Update the test to use the actual field. If the stack is internal-only (no public field), use a `PopInt` round-trip instead:

```go
// Alternative: if IntStack is not a public field, verify length via
// repeated PopInt (then re-push to leave stack unchanged for any
// subsequent assertions). Simpler: just verify the top two PopInt
// calls return the expected values:
top := s.PopInt()
if top != 42 {
	t.Errorf("top PopInt after WORLD_DELAY: got %d, want 42 (handler must not pop)", top)
}
next := s.PopInt()
if next != 999 {
	t.Errorf("next PopInt: got %d, want 999", next)
}
```

The plan-author or implementer picks the form that matches the actual ScriptState surface.

- [ ] **Step 3: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestWorldDelay -v`

Expected: FAIL with "undefined: handleWorldDelay".

- [ ] **Step 4: Write the handler**

In the file from Step 1 (handlers_server.go or sibling), append:

```go
// handleWorldDelay (WORLD_DELAY, opcode 1021) suspends the active
// script to the world-script queue. The wakeup-tick value is NOT
// popped here — it remains on the script's int stack and is popped by
// the suspending caller (resumeOrFinish for player path, resumeOrFinishNpc
// for npc path, processWorldQueue for world-self-loop) at suspend
// time before re-enqueueing. Mirrors TS ServerOps.ts:166-169 verbatim:
//
//	[ScriptOpcode.WORLD_DELAY]: state => {
//	    // arg is popped elsewhere
//	    state.execution = ScriptState.WORLD_SUSPENDED;
//	}
//
// The "arg popped elsewhere" semantics are load-bearing: the script
// bytecode pushes the wakeup-tick before WORLD_DELAY and expects
// the resumer to consume it. Adding a pop here would break the
// bytecode contract.
func handleWorldDelay(s *ScriptState) error {
	s.Execution = WorldSuspended
	return nil
}
```

- [ ] **Step 5: Register the handler**

In `pkg/script/handlers.go`, add:

```go
	OpWorldDelay:     handleWorldDelay,
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestWorldDelay -v`

Expected: PASS.

- [ ] **Step 7: Run full package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/`

Expected: all green. The handler in isolation does not yet activate the world-queue infrastructure — `resumeOrFinish` and `resumeOrFinishNpc` still log+drop on `WorldSuspended`. T8-T12 wire the consumer side.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_server.go pkg/script/handlers.go pkg/script/handlers_server_test.go
git commit --no-gpg-sign -m "feat(script): NAI-37 T7 — WORLD_DELAY handler (opcode 1021)

One-line TS-faithful port: s.Execution = WorldSuspended; return nil.
Does NOT pop the wakeup-tick — that's the resumer's responsibility
(T10-T12 wire all three producer paths).

At this commit the consumer side is not yet activated:
resumeOrFinish/resumeOrFinishNpc still log+drop on WorldSuspended.
T8-T12 ship the worldScriptQueue + processWorldQueue + dispatch
helper, completing the round-trip."
```

---

### Task 8: worldScriptQueue type + EnqueueWorldScript API

**Files:**
- Create: `modules/world/world_script_queue.go` (queue type + EnqueueWorldScript method + worldScriptQueue field on Server)
- Modify: `modules/world/server.go` (add worldScriptQueue field to Server struct)

- [ ] **Step 1: Locate Server struct**

Run: `grep -n "^type Server struct" /home/owner/Code/github.com/zsrv/goscape/modules/world/server.go`

Expected: locate the Server struct. Note the placement of existing tick-wide queue/state fields (e.g., `npcs`, `playerLoop`, `newPlayers`).

- [ ] **Step 2: Add worldScriptQueue field to Server**

In `modules/world/server.go`, inside `type Server struct`, add (near other tick-wide queue fields):

```go
	// worldScriptQueue holds scripts suspended to the world tick. Each
	// entry awaits its delay countdown, then re-enters ScriptRunner
	// at the start of the next tick where it reaches delay==0. Drained
	// by processWorldQueue. Producer call sites: resumeOrFinish (player
	// path), resumeOrFinishNpc (npc path), processWorldQueue itself
	// (world self-loop). Mirrors TS World.queue.
	//
	// Single-tick goroutine ownership; no mutex required.
	worldScriptQueue []worldScriptQueueEntry
```

- [ ] **Step 3: Create world_script_queue.go**

Create `modules/world/world_script_queue.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// worldScriptQueueEntry is one suspended script awaiting its
// world-tick wakeup. delay decrements each tick by processWorldQueue;
// when it reaches 0 the script is removed and ScriptRunner.Execute
// is called, then resumeOrFinishWorld dispatches the post-execute
// state.
type worldScriptQueueEntry struct {
	script *script.ScriptState
	delay  int
}

// EnqueueWorldScript appends a script to the world-script queue with
// the given wakeup-tick countdown. Called by:
//   - resumeOrFinish (player path) when a player-bound script returned
//     WorldSuspended; the caller pops the wakeup-tick from the script's
//     int stack and passes it as delay.
//   - resumeOrFinishNpc (npc path) — symmetric.
//   - processWorldQueue (world self-loop) when a world-queued script
//     re-suspends with WorldSuspended.
//
// Mirrors TS World.enqueueScript at World.ts:1238.
//
// Note: delay parameter is the wakeup-tick value popped by the caller,
// not the queue-internal "ticks remaining" counter. processWorldQueue
// decrements the entry's delay each tick; when it would go below or
// reach zero, the entry fires.
func (s *Server) EnqueueWorldScript(state *script.ScriptState, delay int) {
	s.worldScriptQueue = append(s.worldScriptQueue, worldScriptQueueEntry{
		script: state,
		delay:  delay,
	})
}
```

- [ ] **Step 4: Build verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean.

- [ ] **Step 5: Vet verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean.

- [ ] **Step 6: Run tests to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green (no behavior change yet — the queue field is declared but nothing reads or writes it).

- [ ] **Step 7: Commit**

```bash
git add modules/world/world_script_queue.go modules/world/server.go
git commit --no-gpg-sign -m "feat(world): NAI-37 T8 — worldScriptQueue + EnqueueWorldScript API

New file modules/world/world_script_queue.go declares
worldScriptQueueEntry { script, delay } and (s *Server).EnqueueWorldScript
appending to the s.worldScriptQueue slice on Server.

No producers or consumers wired yet — T9 ships processWorldQueue,
T10/T11 ship the player/npc producer paths, T12 ships the
resumeOrFinishWorld dispatch helper."
```

---

### Task 9: processWorldQueue tick step + tick-loop integration + scheduler tests

**Files:**
- Modify: `modules/world/world_script_queue.go` (add processWorldQueue method)
- Modify: `modules/world/tick.go` (wire processWorldQueue into runTickLoopWithRate)
- Create: `modules/world/world_script_queue_test.go` (scheduler tests)

- [ ] **Step 1: Write failing scheduler tests**

Create `modules/world/world_script_queue_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// --- NAI-37 Task 9: world-script-queue scheduler tests --------------------

// makeWorldQueueTestServer constructs a minimal *Server suitable for
// scheduler-level testing. Uses the same construction pattern as
// existing modules/world tests (mirror an existing test helper if one
// exists; if not, construct directly).
func makeWorldQueueTestServer(t *testing.T) *Server {
	t.Helper()
	// Mirror existing test-server construction in modules/world. Adjust
	// to match whatever helper or constructor pattern existing tests
	// use (e.g., NewServer with test config). The returned *Server
	// must have a logger configured (s.log) — processWorldQueue
	// uses s.log.Warn for error paths.
	s := newTestServer(t) // replace with actual existing test-server helper
	return s
}

// scriptedExecuteFn lets tests intercept script.Execute. We control
// what each "execution" returns by stubbing. processWorldQueue calls
// script.Execute on each entry; tests stub this via the test-only
// hook (or a counter mock-based ScriptState).
//
// Approach: build a real script.ScriptState whose script body is
// deterministic and observable. The simplest path is to use a
// pre-compiled minimal ScriptFile that just returns immediately —
// the test asserts the queue state pre/post-fire rather than
// the script's effect.

func TestProcessWorldQueue_DelayZero_FiresImmediately(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	state := newReturnImmediatelyScriptState(t) // helper that produces a script that runs once and returns Finished
	s.EnqueueWorldScript(state, 0)
	if got, want := len(s.worldScriptQueue), 1; got != want {
		t.Fatalf("queue length post-enqueue: got %d, want %d", got, want)
	}
	s.processWorldQueue()
	if got, want := len(s.worldScriptQueue), 0; got != want {
		t.Errorf("queue length post-fire: got %d, want %d (delay=0 entry must fire and be removed)", got, want)
	}
}

func TestProcessWorldQueue_DelayN_FiresAfterNTicks(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	state := newReturnImmediatelyScriptState(t)
	s.EnqueueWorldScript(state, 3)

	// Tick 1: delay 3 → 2 (>0, skip).
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after tick 1: queue length got %d, want 1", got)
	}
	// Tick 2: delay 2 → 1 (>0, skip).
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after tick 2: queue length got %d, want 1", got)
	}
	// Tick 3: delay 1 → 0 (NOT > 0, fires).
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("after tick 3: queue length got %d, want 0 (delay=3 entry fires on the 3rd processWorldQueue call)", got)
	}
}

func TestProcessWorldQueue_FifoOrder(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	a := newRecordingScriptState(t, "A")
	b := newRecordingScriptState(t, "B")
	c := newRecordingScriptState(t, "C")
	s.EnqueueWorldScript(a, 0)
	s.EnqueueWorldScript(b, 0)
	s.EnqueueWorldScript(c, 0)

	s.processWorldQueue()

	executed := drainScriptExecutionRecord(t)
	want := []string{"A", "B", "C"}
	if !equalStringSlice(executed, want) {
		t.Errorf("execution order: got %v, want %v", executed, want)
	}
}

func TestProcessWorldQueue_RemovedBeforeFire(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	// Use a script that, when Executed, inspects s.worldScriptQueue
	// and records its length. The test asserts the recorded length
	// is 0 — proving the entry was removed BEFORE Execute fired.
	state := newQueueLengthSnapshotScriptState(t, s)
	s.EnqueueWorldScript(state, 0)

	s.processWorldQueue()

	if got := snapshotQueueLength(t); got != 0 {
		t.Errorf("queue length seen by script during Execute: got %d, want 0 (entry must be removed before fire)", got)
	}
}

func TestProcessWorldQueue_ReentrantEnqueueVisibleSameTick(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	// Script A's Execute calls s.EnqueueWorldScript(B, 0); test asserts
	// both A and B fired in the same processWorldQueue call.
	b := newRecordingScriptState(t, "B")
	a := newReentrantEnqueueScriptState(t, s, b)
	s.EnqueueWorldScript(a, 0)

	s.processWorldQueue()

	executed := drainScriptExecutionRecord(t)
	want := []string{"A", "B"} // B was appended mid-iteration; loop must see it
	if !equalStringSlice(executed, want) {
		t.Errorf("execution order with re-entrant enqueue: got %v, want %v (mid-pass append must be visible — speedup quirk)", executed, want)
	}
}

func TestProcessWorldQueue_WorldSuspendedSelfLoop(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	// First Execute: pushes 2 onto stack, returns WorldSuspended.
	// resumeOrFinishWorld pops 2, re-enqueues with delay=2.
	state := newSuspendingScriptState(t, 2)
	s.EnqueueWorldScript(state, 0)

	s.processWorldQueue()

	if got := len(s.worldScriptQueue); got != 1 {
		t.Errorf("after self-loop suspend: queue length got %d, want 1 (re-enqueued)", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 2 {
		t.Errorf("re-enqueued delay: got %d, want 2 (popped from script stack)", got)
	}
}
```

The above test file references several test-helper functions that may or may not exist in goscape's test infrastructure:
- `newTestServer(t)` — minimal *Server constructor
- `newReturnImmediatelyScriptState(t)` — a ScriptState that runs once and returns Finished
- `newRecordingScriptState(t, name)` — a ScriptState that appends `name` to a global execution record
- `drainScriptExecutionRecord(t)` — read+clear the execution record
- `newQueueLengthSnapshotScriptState(t, s)` — script that snapshots queue length when fired
- `snapshotQueueLength(t)` — read the snapshotted length
- `newReentrantEnqueueScriptState(t, s, b)` — script that calls s.EnqueueWorldScript(b, 0) when fired
- `newSuspendingScriptState(t, delay)` — script that pushes `delay` and returns WorldSuspended
- `equalStringSlice(a, b)` — slice equality helper

**Plan-author or implementer must verify these helpers exist OR build them.** Run:

```bash
grep -rn "func newTestServer\|func newReturnImmediatelyScriptState\|func equalStringSlice" /home/owner/Code/github.com/zsrv/goscape/modules/world/ /home/owner/Code/github.com/zsrv/goscape/pkg/script/
```

Per `plan_runnable_test_fixtures.md`: if helpers are missing, the implementer either (a) creates them in `world_script_queue_test.go` as test-local helpers using the actual goscape ScriptState/ScriptFile construction patterns, OR (b) reframes each test to inline the helper logic. The integration test (Task 13) covers the full round-trip; these scheduler tests are unit tests for the queue mechanics — keeping the script state simple is preferable.

If creating test-local helpers, mirror the construction pattern from existing `pkg/script/runner_test.go` ScriptFile construction (the `runScriptFile`/`testScriptFile` patterns). Each test-local helper must compile and produce a real, runnable ScriptState that the actual `script.Execute` can process.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessWorldQueue -v`

Expected: FAIL with "undefined: processWorldQueue" or compile errors for missing helpers.

- [ ] **Step 3: Implement processWorldQueue**

Append to `modules/world/world_script_queue.go`:

```go
// processWorldQueue drains ready entries from s.worldScriptQueue,
// firing each by calling script.Execute and dispatching the
// post-execute state via resumeOrFinishWorld.
//
// Iteration uses index-based slice walk with mid-pass append visibility
// (re-reads len(s.worldScriptQueue) each loop iteration) — this
// preserves the same TS-authentic "speedup quirk" already present
// in processPlayerQueue (tick.go:222) where a script that re-enqueues
// itself or another script during Execute will see the new entry
// processed in the same tick.
//
// Removal happens BEFORE firing (matching processPlayerQueue:243-249)
// so a re-entrant Execute that calls EnqueueWorldScript doesn't
// collide with the index pointer.
//
// Mirrors TS World.processWorld world-queue iteration at World.ts:534-559.
func (s *Server) processWorldQueue() {
	i := 0
	for i < len(s.worldScriptQueue) {
		entry := &s.worldScriptQueue[i]
		entry.delay--
		if entry.delay > 0 {
			i++
			continue
		}
		state := entry.script
		s.worldScriptQueue = append(s.worldScriptQueue[:i], s.worldScriptQueue[i+1:]...)
		s.resumeOrFinishWorld(state)
		// Don't advance i: we just removed the current element, so i
		// now points to what was the next element (or past end).
	}
}
```

The implementation references `s.resumeOrFinishWorld(state)` — that helper is added in Task 12. For T9 to compile, write a stub now and replace in T12:

In `modules/world/script.go`, add a stub:

```go
// resumeOrFinishWorld dispatches the post-Execute state for a script
// run from the world queue. STUB — full dispatch table arrives in T12.
func (s *Server) resumeOrFinishWorld(state *script.ScriptState) {
	if err := script.Execute(state); err != nil {
		s.log.Warn("world script execute error",
			"script", state.Script.Name, "err", err)
		return
	}
	// T12 will switch on state.Execution.
}
```

- [ ] **Step 4: Wire processWorldQueue into runTickLoopWithRate**

In `modules/world/tick.go`, line 34-35 area (between `s.processClientsIn()` and `s.processActiveScripts()`), insert:

```go
		s.processClientsIn()
		s.processWorldQueue() // NAI-37: matches TS World.processWorld start-of-cycle ordering
		s.processActiveScripts()
```

- [ ] **Step 5: Run scheduler tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessWorldQueue -v`

Expected: PASS for all 6 scheduler tests. If any test still fails, check the helper-construction Step 1 produced and adjust.

- [ ] **Step 6: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add modules/world/world_script_queue.go modules/world/world_script_queue_test.go modules/world/tick.go modules/world/script.go
git commit --no-gpg-sign -m "feat(world): NAI-37 T9 — processWorldQueue + tick-loop wiring

Add (s *Server).processWorldQueue draining the world-script queue
with index-iteration and mid-pass append visibility (mirrors
processPlayerQueue's speedup quirk + remove-before-fire pattern).
Wire processWorldQueue into runTickLoopWithRate between processClientsIn
and processActiveScripts, matching TS World.processWorld start-of-cycle
ordering.

Add stub (s *Server).resumeOrFinishWorld in script.go — T12 fills
the dispatch table.

6 scheduler tests pinning: delay=0 fires immediately, delay=N fires
after N ticks, FIFO order, remove-before-fire, re-entrant enqueue
visibility (speedup quirk), WorldSuspended self-loop."
```

---

### Task 10: Player-path producer (resumeOrFinish WorldSuspended branch)

**Files:**
- Modify: `modules/world/script.go` (add WorldSuspended case to resumeOrFinish)
- Modify: `modules/world/script_test.go` (add producer test; create file if absent)

- [ ] **Step 1: Write the failing test**

Determine the test file: `ls /home/owner/Code/github.com/zsrv/goscape/modules/world/script_test.go`. If absent, create. If present, append.

Append:

```go
// --- NAI-37 Task 10: player-path WorldSuspended producer test --------------

func TestResumeOrFinish_WorldSuspended_EnqueuesAndClearsActiveScript(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	p := makeTestPlayer(t, s) // existing test-player helper or a new one matching modules/world conventions

	// Build a ScriptState that, after Execute, will have Execution=WorldSuspended
	// and an int (the delay value to be popped) on its stack. The simplest
	// approach is to construct the ScriptState directly with these post-conditions
	// and skip Execute by using a pre-set state — OR use a real ScriptFile
	// containing pushInt(N); WORLD_DELAY; ...

	state := newSuspendingScriptState(t, 5) // pushes 5 then WORLD_DELAY → leaves int=5 on stack, sets Execution=WorldSuspended

	s.resumeOrFinish(state, p)

	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("worldScriptQueue length post-resumeOrFinish: got %d, want 1", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 5 {
		t.Errorf("enqueued delay: got %d, want 5 (popped from script stack)", got)
	}
	if got := p.activeScript; got != nil {
		t.Errorf("player.activeScript: got %v, want nil (script transitioned to world-bound)", got)
	}
}
```

Verify `makeTestPlayer` and `p.activeScript` accessibility:

Run: `grep -n "func makeTestPlayer\|activeScript " /home/owner/Code/github.com/zsrv/goscape/modules/world/`

Adjust the assertion accordingly — if `activeScript` is unexported, use the goscape ClearActiveScript-test-pattern (e.g., a getter or a sentinel-recorder).

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResumeOrFinish_WorldSuspended -v`

Expected: FAIL — the WorldSuspended branch in resumeOrFinish currently falls through to the default and clears+warns instead of enqueueing.

- [ ] **Step 3: Add WorldSuspended case to resumeOrFinish**

In `modules/world/script.go`, locate `resumeOrFinish` (currently around line 46-64). The current switch is:

```go
	switch state.Execution {
	case script.Finished, script.Aborted:
		self.ClearActiveScript()
	case script.Suspended, script.PauseButton, script.CountDialog:
		self.StoreActiveScript(state)
	default:
		// NpcSuspended / WorldSuspended — future sub-specs.
		s.log.Warn("script in unsupported execution state",
			"script", state.Script.Name, "execution", state.Execution)
		self.ClearActiveScript()
	}
```

Add a new case BEFORE the default branch:

```go
	switch state.Execution {
	case script.Finished, script.Aborted:
		self.ClearActiveScript()
	case script.Suspended, script.PauseButton, script.CountDialog:
		self.StoreActiveScript(state)
	case script.WorldSuspended:
		// NAI-37: player-bound script suspended to world queue. Pop
		// the wakeup-tick (which the script's bytecode pushed before
		// WORLD_DELAY) and enqueue. Player no longer owns this script;
		// it now belongs to the world queue. Mirrors TS Player.ts:2135-2136.
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
		self.ClearActiveScript()
	default:
		// NpcSuspended — future sub-spec.
		s.log.Warn("script in unsupported execution state",
			"script", state.Script.Name, "execution", state.Execution)
		self.ClearActiveScript()
	}
```

The default-branch comment is updated: WorldSuspended is removed from the "future sub-specs" list; only NpcSuspended remains.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResumeOrFinish -v`

Expected: PASS. The new WorldSuspended branch enqueues and clears.

- [ ] **Step 5: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green. Existing resumeOrFinish tests continue to pass — only the previously-default-branch behavior for WorldSuspended changes.

- [ ] **Step 6: Commit**

```bash
git add modules/world/script.go modules/world/script_test.go
git commit --no-gpg-sign -m "feat(world): NAI-37 T10 — player-path WorldSuspended producer

Add explicit case script.WorldSuspended branch to resumeOrFinish:
pops the wakeup-tick from the script's int stack and enqueues to
worldScriptQueue, then ClearActiveScript on the player. Mirrors TS
Player.ts:2135-2136 (and Player.ts:2143-2150 cleanup path).

Default-branch comment updated: WorldSuspended removed from the
future-sub-specs list; NpcSuspended remains (T11)."
```

---

### Task 11: Npc-path producer (resumeOrFinishNpc WorldSuspended branch)

**Files:**
- Modify: `modules/world/npc_script.go` (add WorldSuspended case to resumeOrFinishNpc)
- Modify: `modules/world/npc_script_test.go` (add producer test)

- [ ] **Step 1: Write the failing test**

In `modules/world/npc_script_test.go`, append:

```go
// --- NAI-37 Task 11: npc-path WorldSuspended producer test ----------------

func TestResumeOrFinishNpc_WorldSuspended_EnqueuesAndClears(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	n := NewNpc(0, 0, 3200, 3200, 0, &objtype.NpcType{})

	state := newSuspendingScriptState(t, 7) // pushes 7, WORLD_DELAY → Execution=WorldSuspended

	s.resumeOrFinishNpc(state, n)

	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("worldScriptQueue length post-resumeOrFinishNpc: got %d, want 1", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 7 {
		t.Errorf("enqueued delay: got %d, want 7 (popped from script stack)", got)
	}
	if got := n.activeScript; got != nil {
		t.Errorf("npc.activeScript: got %v, want nil (script transitioned to world-bound)", got)
	}
}
```

Adjust `n.activeScript` access if the field is unexported and not test-accessible — use a sentinel-recorder pattern (e.g., a setter that bumps a counter).

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResumeOrFinishNpc_WorldSuspended -v`

Expected: FAIL — currently the default branch logs warn and ClearActiveScripts.

- [ ] **Step 3: Add WorldSuspended case to resumeOrFinishNpc**

In `modules/world/npc_script.go`, locate `resumeOrFinishNpc` (currently lines 296-315). The current switch:

```go
	switch state.Execution {
	case script.Finished, script.Aborted:
		npc.ClearActiveScript()
	case script.NpcSuspended:
		npc.StoreActiveScript(state)
	default:
		// Suspended / PauseButton / CountDialog / WorldSuspended —
		// not reachable via npc_delay alone, but defensively clear.
		s.log.Warn("npc script in unexpected execution state",
			"script", state.Script.Name, "execution", state.Execution)
		npc.ClearActiveScript()
	}
```

Add a new case BEFORE the default:

```go
	switch state.Execution {
	case script.Finished, script.Aborted:
		npc.ClearActiveScript()
	case script.NpcSuspended:
		npc.StoreActiveScript(state)
	case script.WorldSuspended:
		// NAI-37: npc-bound script suspended to world queue. Symmetric
		// to resumeOrFinish (player path). Mirrors TS Npc.ts:219-220.
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
		npc.ClearActiveScript()
	default:
		// Suspended / PauseButton / CountDialog —
		// not reachable via npc_delay alone, but defensively clear.
		s.log.Warn("npc script in unexpected execution state",
			"script", state.Script.Name, "execution", state.Execution)
		npc.ClearActiveScript()
	}
```

The default-branch comment is updated: WorldSuspended is removed.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResumeOrFinishNpc -v`

Expected: PASS.

- [ ] **Step 5: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_script.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "feat(world): NAI-37 T11 — npc-path WorldSuspended producer

Symmetric to T10 player path: case script.WorldSuspended branch in
resumeOrFinishNpc pops wakeup-tick + enqueues to worldScriptQueue +
ClearActiveScript on the npc. Mirrors TS Npc.ts:219-220.

All three TS producer paths now wired: player (T10), npc (T11),
world self-loop (handled in T9's processWorldQueue → T12's
resumeOrFinishWorld dispatch).

Default-branch comment updated: WorldSuspended removed from
future-sub-specs list."
```

---

### Task 12: resumeOrFinishWorld dispatch table + dispatch tests

**Files:**
- Modify: `modules/world/script.go` (replace stub resumeOrFinishWorld with full dispatch table)
- Modify: `modules/world/script_test.go` or `modules/world/world_script_queue_test.go` (add dispatch table tests)

- [ ] **Step 1: Write failing dispatch tests**

Append to `modules/world/world_script_queue_test.go` (closer location to scheduler tests it shares helpers with):

```go
// --- NAI-37 Task 12: resumeOrFinishWorld dispatch table tests ------------

func TestResumeOrFinishWorld_FinishedClean(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	state := newReturnImmediatelyScriptState(t) // returns Finished
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("Finished: queue length got %d, want 0 (drop)", got)
	}
}

func TestResumeOrFinishWorld_WorldSuspendedSelfReenqueue(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	state := newSuspendingScriptState(t, 4) // pushes 4, returns WorldSuspended
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("WorldSuspended self-loop: queue length got %d, want 1", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 4 {
		t.Errorf("re-enqueued delay: got %d, want 4 (popped)", got)
	}
}

func TestResumeOrFinishWorld_CrossContextSuspendedDrop(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	state := newReturningExecutionScriptState(t, script.Suspended)
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("Suspended (cross-context): queue length got %d, want 0 (warn+drop per NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP)", got)
	}
	// Verify the warn was emitted — capture logger output and assert
	// it contains "cross-context" or similar key phrase. Mirror existing
	// log-capture test patterns in modules/world.
}

func TestResumeOrFinishWorld_CrossContextNpcSuspendedDrop(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	state := newReturningExecutionScriptState(t, script.NpcSuspended)
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("NpcSuspended (cross-context): queue length got %d, want 0", got)
	}
}

func TestResumeOrFinishWorld_CrossContextPauseButtonDrop(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	state := newReturningExecutionScriptState(t, script.PauseButton)
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("PauseButton (cross-context): queue length got %d, want 0", got)
	}
}

func TestResumeOrFinishWorld_CrossContextCountDialogDrop(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	state := newReturningExecutionScriptState(t, script.CountDialog)
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("CountDialog (cross-context): queue length got %d, want 0", got)
	}
}

func TestResumeOrFinishWorld_RunningWarnDrop(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	// Running shouldn't happen post-Execute, but if it does, must drop+warn.
	state := newReturningExecutionScriptState(t, script.Running)
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("Running: queue length got %d, want 0 (drop)", got)
	}
}
```

Verify the helper `newReturningExecutionScriptState(t, exec)` exists or build it. The helper produces a ScriptState whose Execute returns with Execution=exec — the simplest implementation manipulates the returned state directly (since real Execute on a no-op script returns Finished, the test must use a stub or mock).

- [ ] **Step 2: Run dispatch tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResumeOrFinishWorld -v`

Expected: FAIL — the stub from T9 doesn't dispatch on Execution, so most of these tests will fall through with no observable behavior change matching the expected branch.

- [ ] **Step 3: Replace the resumeOrFinishWorld stub with the full dispatch table**

In `modules/world/script.go`, replace the T9 stub:

```go
// resumeOrFinishWorld dispatches the post-Execute state for a script
// run from the world-script queue (called by processWorldQueue after
// removing the entry but before/instead of the next loop iteration).
//
// Dispatch table:
//   - Finished, Aborted: drop entry; clean exit (Aborted may already
//     be logged at script.Execute error level).
//   - WorldSuspended: re-enqueue (self-loop case from path P3 in the
//     spec). Pops the wakeup-tick from the script's int stack and
//     re-appends to worldScriptQueue. Mirrors TS World.ts:553-555.
//   - Suspended, NpcSuspended, PauseButton, CountDialog: warn+drop.
//     Tracked deviation NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP — TS
//     handles these implicitly by re-binding to the corresponding
//     entity's activeScript (Player.ts:2137-2141, Npc.ts:221-225);
//     goscape's narrower handling is intentional pending a broader
//     player-script-lifecycle alignment.
//   - Running: should not occur post-Execute; warn loudly and drop.
func (s *Server) resumeOrFinishWorld(state *script.ScriptState) {
	if err := script.Execute(state); err != nil {
		s.log.Warn("world script execute error",
			"script", state.Script.Name, "err", err)
		return
	}
	switch state.Execution {
	case script.Finished, script.Aborted:
		// Clean exit; nothing to do (entry already removed by caller).
	case script.WorldSuspended:
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
	case script.Suspended, script.NpcSuspended, script.PauseButton, script.CountDialog:
		// DEVIATION NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP: cross-context
		// resume from a world-queued script is not supported. TS would
		// re-bind to the corresponding entity's activeScript; goscape
		// drops with a warn until broader script-lifecycle alignment.
		s.log.Warn("world-queue script transitioned to cross-context state; resume unsupported",
			"script", state.Script.Name, "execution", state.Execution)
	default:
		// Running, or any future-added Execution value.
		s.log.Warn("world-queue script in unexpected execution state",
			"script", state.Script.Name, "execution", state.Execution)
	}
}
```

- [ ] **Step 4: Run dispatch tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResumeOrFinishWorld -v`

Expected: PASS for all 7 dispatch-table cases.

- [ ] **Step 5: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add modules/world/script.go modules/world/world_script_queue_test.go
git commit --no-gpg-sign -m "feat(world): NAI-37 T12 — resumeOrFinishWorld dispatch table

Replace T9's stub with the full dispatch:
  - Finished, Aborted → drop (clean exit).
  - WorldSuspended → re-enqueue with popped wakeup-tick (self-loop,
    path P3, mirrors TS World.ts:553-555).
  - Suspended, NpcSuspended, PauseButton, CountDialog → warn+drop.
    Tracked deviation NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP.
  - default (Running, future) → warn+drop.

7 dispatch tests covering each branch. World-script-queue
infrastructure complete; T13 ships the integration smoke test."
```

---

### Task 13: Full round-trip integration test

**Files:**
- Modify: `modules/world/world_script_queue_test.go` (add integration test)

- [ ] **Step 1: Write the integration test**

Append to `modules/world/world_script_queue_test.go`:

```go
// --- NAI-37 Task 13: full WORLD_DELAY round-trip integration test --------

// TestWorldDelay_FullRoundTrip exercises the complete cross-tick
// coordination of WORLD_DELAY: a player-bound script that pushes a
// delay, calls WORLD_DELAY, then completes after the world tick wakes
// it up.
//
// The test bytecode is deliberately minimal — pushInt(2); WORLD_DELAY;
// RETURN. After T1 (where the script first runs and suspends), the
// script lives in worldScriptQueue with delay=2. After T2 (delay 2→1),
// T3 (delay 1→0), T4 (delay 0→-1, fires), the script resumes,
// runs to RETURN, and the queue is empty.
//
// Per gettimer_passthrough_opcode_semantic_audit.md: handler-mock
// tests pass values through unchanged; this integration test pins
// the actual cross-tick coordination that's the whole point of
// WORLD_DELAY.
//
// Per plan_runnable_test_fixtures.md: the bytecode uses only opcodes
// supported by goscape's existing test ScriptFile-construction
// infrastructure. Plan-author confirms PUSH_INT_LITERAL (or similar)
// + OpWorldDelay + OpReturn are all supported; if any aren't, the
// test reframes to use only supported ops.
func TestWorldDelay_FullRoundTrip(t *testing.T) {
	s := makeWorldQueueTestServer(t)
	p := makeTestPlayer(t, s)

	// Build minimal ScriptFile: pushInt(2); WORLD_DELAY; (script returns
	// after RETURN). Use the existing test-script-builder API in
	// pkg/script/runner_test.go or modules/world test infrastructure.
	sf := buildTestScriptFile(t, []script.Opcode{
		script.OpPushIntConstant, // or whatever push-int-from-operand opcode goscape uses
		script.OpWorldDelay,
		script.OpReturn,
	}, []int32{2}) // intOperands — the 2 to push

	// Run the script as a fresh player-bound run.
	s.runScript(sf, p, false, nil, nil) // signature should match the existing runScript helper

	// After the script first runs:
	//   - It pushes 2, hits WORLD_DELAY, sets Execution=WorldSuspended.
	//   - resumeOrFinish pops 2, enqueues to worldScriptQueue with delay=2.
	//   - Player.activeScript is cleared.
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after T1 (initial run): queue length got %d, want 1", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 2 {
		t.Fatalf("after T1: enqueued delay got %d, want 2", got)
	}

	// T2: processWorldQueue decrements 2→1, doesn't fire.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after T2: queue length got %d, want 1", got)
	}

	// T3: processWorldQueue decrements 1→0, doesn't fire (>0 check).
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after T3: queue length got %d, want 1", got)
	}

	// T4: processWorldQueue decrements 0→-1, fires. Script resumes
	// from after WORLD_DELAY, hits RETURN, completes.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("after T4: queue length got %d, want 0 (script should fire and complete)", got)
	}
}
```

The plan-author or implementer must verify each opcode + helper used:

Run:
```bash
grep -n "OpPushIntConstant\|OpReturn\|OpWorldDelay\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/opcode.go
grep -n "func buildTestScriptFile\|func.*ScriptFile\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/runner_test.go /home/owner/Code/github.com/zsrv/goscape/modules/world/
grep -n "func .* runScript\b" /home/owner/Code/github.com/zsrv/goscape/modules/world/
```

Expected: confirm all references resolve. If any opcode name differs (e.g., `OpPushConstantInt` vs `OpPushIntConstant`), use the actual name. If `buildTestScriptFile` doesn't exist, mirror the construction pattern from existing ScriptFile-using tests.

- [ ] **Step 2: Run the integration test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestWorldDelay_FullRoundTrip -v`

Expected: PASS. If the test reveals an end-to-end bug (e.g., the popInt happens at the wrong stage, the integration shows misalignment between handler/resumer/processor), this is the value of the integration test — fix the underlying bug, not the test.

- [ ] **Step 3: Run full test suite as final verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all 23 packages green; no regressions.

- [ ] **Step 4: Run go vet for completeness**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add modules/world/world_script_queue_test.go
git commit --no-gpg-sign -m "test(world): NAI-37 T13 — WORLD_DELAY full round-trip integration

Pins the cross-tick coordination that's WORLD_DELAY's whole purpose:
  T1: pushInt(2); WORLD_DELAY → suspends, enqueues with delay=2,
      Player.activeScript cleared.
  T2: delay 2→1, doesn't fire.
  T3: delay 1→0, doesn't fire (>0 check).
  T4: delay 0→-1, fires. RETURN runs, queue empty.

Per gettimer_passthrough_opcode_semantic_audit.md: handler-mock
tests pass values through unchanged; only this integration test
exercises the actual multi-tick state machine.

Bundle 3 (WORLD_DELAY full infrastructure) complete.
NAI-37 implementation complete; ready for whole-impl review."
```

---

## Self-review checklist

After all 13 tasks complete, run the spec → plan coverage cross-check before declaring NAI-37 done.

### Spec coverage check (per `plan_test_coverage_crosscheck.md`)

The spec calls for **21 test cases**. Verify the plan delivers:

| Spec test (Section 5) | Plan task | File |
|---|---|---|
| B1 #1 NoActiveNpc errors | T2 | handlers_npc_test.go |
| B1 #2 QueueIDBelowOne errors | T2 | handlers_npc_test.go |
| B1 #3 QueueIDAboveTwenty errors | T2 | handlers_npc_test.go |
| B1 #4 PopOrder + queueID-1 transform | T2 | handlers_npc_test.go |
| B1 #5 BoundaryQueueIDs (1+20) | T2 | handlers_npc_test.go |
| B1 #6 SetWalkTrigger field-write | T3 | npc_script_test.go |
| B2 #1 NoActivePlayer errors | T6 | handlers_player_test.go |
| B2 #2 NoActiveNpc errors | T6 | handlers_player_test.go |
| B2 #3 Success records nid | T6 | handlers_player_test.go |
| B2 #4 PayloadBytes byte-pin | T5 | player_script_test.go |
| B3 #1 SetsExecutionWorldSuspendedAndDoesNotPop | T7 | handlers_server_test.go |
| B3 #2 DelayZero fires | T9 | world_script_queue_test.go |
| B3 #3 DelayN fires after N ticks | T9 | world_script_queue_test.go |
| B3 #4 RemovedBeforeFire | T9 | world_script_queue_test.go |
| B3 #5 FifoOrder | T9 | world_script_queue_test.go |
| B3 #6 ReentrantEnqueueVisibleSameTick | T9 | world_script_queue_test.go |
| B3 #7 WorldSuspendedSelfLoop (scheduler) | T9 | world_script_queue_test.go |
| B3 #8 ResumeOrFinish_WorldSuspended (player) | T10 | script_test.go |
| B3 #9 ResumeOrFinishNpc_WorldSuspended | T11 | npc_script_test.go |
| B3 #10 ResumeOrFinishWorld dispatch table | T12 | world_script_queue_test.go |
| B3 #11 FullRoundTrip integration | T13 | world_script_queue_test.go |

**Total: 21 — matches spec.** ✓

### Deviation tracking (per spec Section 6)

After T13 commits, the deviation tracker (typically `nai_followups.md` or a dedicated tracker doc the user maintains) should list the 4 new tracked deviations:

- `NAI-37-D-WALKTRIGGER-NOREADER`
- `NAI-37-D-HINTARROW-PARTIAL-ENCODER`
- `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP`
- `NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY`

And the 2 implicit retirements should be enumerated in the close-out commit body:

- WorldSuspended declared-but-no-producer (since NAI-S1) — closed by handleWorldDelay (T7).
- WorldSuspended declared-but-no-consumer (since NAI-S1) — closed by processWorldQueue + resumeOrFinishWorld (T9 + T12).

### Smoke handoff

After the whole-impl review, the close-out smoke run (user-driven per `smoke_test_server_handoff.md`) confirms:
1. No `no handler for NPC_WALKTRIGGER (opcode 2545)` warns.
2. No `no handler for HINT_NPC (opcode 2028)` warns.
3. No `no handler for WORLD_DELAY (opcode 1021)` warns.
4. No new `world-queue script transitioned to cross-context state` warns under happy-path content (a new warn here means the cross-context-drop deviation surfaced a real script case to track).
5. WORLD_DELAY-using scripts run to completion (pre-NAI-37 they aborted; post-NAI-37 they should complete after the wakeup tick).
