# NAI-16 Implementation Plan — NAI-5+8 Deferral Sweep

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close four accumulated NAI-era deferrals in a single sub-spec: wire the `checkNotCombatSelf` hunt filter (Item A), wire `NPC_CHANGETYPE` duration semantics with full TS-parity including typeId/uid/lifecycleTick writes (Item B), add a direct RESPAWN+alive morph-revert test (Item C), and add a `processNpcEventQueue` happy-path fire test (Item D).

**Architecture:** Linear TDD per task — write failing test, verify failure, implement minimal production code, verify pass, commit. Four production/test tasks plus a closing memory-update commit. Task 2 (ChangeType) is the only task with an interface-signature change; it ripples to 5 enumerated call sites which must all update in lockstep to compile.

**Tech Stack:** Go 1.26+. No new packages. Existing `pkg/script` (`ActiveNpc`, `handlers_npc.go`), `pkg/objtype` (`HuntType`), `modules/world` (`Npc`, `Server`, `npc_hunt.go`, `npc_ai.go`, `npc_masks.go`, `npc_script.go`, `npc_event_queue.go`).

**Spec:** `docs/superpowers/specs/2026-04-23-nai-16-deferral-sweep-design.md`

**Go command prefix:** All `go` invocations use `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` per the project's `CLAUDE.md`.

**Commit style:** All commits use `--no-gpg-sign` per user's global `CLAUDE.md`.

---

## Task 1 — Item A: `checkNotCombatSelf` filter in `huntPlayers`

**Files:**
- Modify: `modules/world/npc_hunt.go` (filter body at lines 178-179, doc comment at lines 98-103, filter-coverage comment at lines 87-93)
- Test: `modules/world/npc_hunt_test.go` (add 2 new test functions)

**Rationale:** Ports TS `Npc.ts:946-948` as a direct symmetric extension of the NAI-15 `checkNotCombat` filter. Reads the NPC-side varn via `n.NpcVarN(hunt.CheckNotCombatSelf)` (existing from S6a). Gated by the same outer combat guard as `checkNotCombat`.

### Steps

- [ ] **Step 1.1 — Write the two failing tests**

Append to end of `modules/world/npc_hunt_test.go` (insert after the last existing `TestHuntPlayersCombatGuard*` test — exact position is end of file):

```go
// TestHuntPlayersCheckNotCombatSelf guards the NPC-side 8-tick
// combat-window filter at TS Npc.ts:946-948. Symmetric to
// TestHuntPlayersCheckNotCombat but reads n.NpcVarN instead of p.Varp.
// When the outer guard applies, an NPC whose own combat-tracker varn
// was written within [currentTick-7, currentTick] skips the candidate;
// at currentTick-8 and earlier, the candidate passes.
func TestHuntPlayersCheckNotCombatSelf(t *testing.T) {
	// Helper mirrors TestHuntPlayersCheckNotCombat's setup. varnVal
	// seeds n.varns[0] via SetNpcVarN.
	setup := func(t *testing.T, currentTick int, varnVal int32) (*Server, *Npc, *Player) {
		t.Helper()
		s := newServerForScriptTest(t)
		s.gamemap = gamemap.New(discardLogger())
		s.currentTick = currentTick
		n := newNpcForLifecycleTest(t)
		n.server = s
		n.x, n.z, n.level = 3094, 3106, 0
		n.huntRange = 10
		n.target = nil // guard applies (target != p) → filter fires
		n.SetNpcVarN(0, varnVal)
		p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
		return s, n, p
	}

	t.Run("default-minus-one-disables", func(t *testing.T) {
		_, n, _ := setup(t, 100, 100) // varn written this tick
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (CheckNotCombatSelf=-1 disables filter)", len(hunted))
		}
	})

	t.Run("varn-this-tick-excluded", func(t *testing.T) {
		_, n, _ := setup(t, 100, 100) // 100+8 > 100 → fire
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: 0}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 0 {
			t.Fatalf("got %d, want 0 (varn written this tick → filter fires)", len(hunted))
		}
	})

	t.Run("varn-minus-eight-included", func(t *testing.T) {
		_, n, _ := setup(t, 100, 92) // 92+8 = 100, 100 > 100 is false → pass
		hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: 0}
		hunted := n.huntPlayers(n.server, hunt)
		if len(hunted) != 1 {
			t.Fatalf("got %d, want 1 (varn==currentTick-8 → exclusive boundary, filter passes)", len(hunted))
		}
	})
}

// TestHuntPlayersCheckNotCombatSelfOutsideGuard guards that the filter
// does NOT fire when the outer combat guard is skipped (target == p OR
// multi-combat zone). Mirrors TestHuntPlayersCombatGuard but with the
// self-side filter.
func TestHuntPlayersCheckNotCombatSelfOutsideGuard(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10
	n.SetNpcVarN(0, 100) // varn==currentTick → filter would fire if guard applies
	p := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
	n.target = p // guard SKIPPED (target == p)

	hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: 0}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("got %d, want 1 (target==p → guard skipped, filter does not fire)", len(hunted))
	}
}
```

- [ ] **Step 1.2 — Run tests, confirm they FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntPlayersCheckNotCombatSelf' -v`

Expected: `varn-this-tick-excluded` sub-case FAILS with `got 1, want 0` (the filter is not yet wired, so the candidate is accepted). `default-minus-one-disables` and `varn-minus-eight-included` and `TestHuntPlayersCheckNotCombatSelfOutsideGuard` PASS vacuously (candidate accepted in all cases matches the "filter not wired" state).

- [ ] **Step 1.3 — Wire the filter in production code**

In `modules/world/npc_hunt.go`, replace lines 178-179 (the DEFERRED comment) with:

```go
			// checkNotCombatSelf (TS:946-948): skip candidate if this NPC's
			// own combat-tracker varn was written within the past 8 ticks.
			// Symmetric to checkNotCombat above, but reads the NPC side
			// (n.NpcVarN) instead of the player side (p.Varp).
			if hunt.CheckNotCombatSelf != -1 &&
				int(n.NpcVarN(hunt.CheckNotCombatSelf))+8 > s.currentTick {
				continue
			}
```

- [ ] **Step 1.4 — Update the deferred-filter doc comment block**

In `modules/world/npc_hunt.go`, replace lines 98-103 (the "Filters DEFERRED" block) with:

```go
// Filters DEFERRED (infra missing; each TS line cited):
//   - checkNotBusy             (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong        (TS:939-941)       — wilderness + combat-level
//   - checkInv                 (TS:959-969)       — inventory queries
```

Also update the "Filter coverage" list at lines 87-93. Add one line after the existing `checkNotCombat` line:

```go
//   - checkNotCombatSelf       (NAI-16, TS:946-948)
```

- [ ] **Step 1.5 — Run tests, confirm they PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntPlayers' -v`

Expected: all `TestHuntPlayersCheckNotCombatSelf*` sub-cases PASS, plus all existing `TestHuntPlayers*` tests still PASS (regression check).

- [ ] **Step 1.6 — Run full-package tests as regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: all tests PASS.

- [ ] **Step 1.7 — Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-16 Task 1 checkNotCombatSelf filter in huntPlayers

Ports TS Npc.ts:946-948. Symmetric to the NAI-15 checkNotCombat
filter but reads n.NpcVarN instead of p.Varp. Gated by the same
outer combat guard at TS Npc.ts:942.

The S6a varns infrastructure (Npc.varns, NpcVarN, SetNpcVarN) was
already at HEAD; the nai_followups.md "no NPC-vars infra" blocker
claim was stale — captured in the NAI-16 close memory update.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — Item B: `NPC_CHANGETYPE` duration wiring

**Files:**
- Modify: `pkg/script/active.go` (interface decl at line 341)
- Modify: `pkg/script/handlers_npc.go` (handler at lines 173-184)
- Modify: `modules/world/npc_masks.go` (impl at lines 16-19)
- Modify: `pkg/script/handlers_npc_test.go` (mockNpc at lines 79-81 + `changeTypeCalls` field)
- Modify: `pkg/script/handlers_player_test.go` (mockActiveNpc at line 31)
- Modify: `modules/world/npc_test.go` (update `TestNpcChangeTypeSetsMask` at lines 41-49)
- Test: Add `TestNpcChangeTypeDurationZeroNoOp` and `TestNpcChangeTypeDeadNoOp` to `modules/world/npc_test.go`
- Test: Add `TestHandleNpcChangeTypePassesDuration` to `pkg/script/handlers_npc_test.go`

**Rationale:** Current `*Npc.ChangeType` only writes `changeTypeID` (mask payload) and raises the mask bit; it does not update `n.typeId`, which means NPCs never actually morph server-side. This is a latent correctness bug. Port TS `Npc.changeType(type, duration)` at `Engine-TS/.../Npc.ts:427-449` minus the DEFERRED stats-reset and KEEPALL branches.

**Call-site enumeration** (per `enumerate_all_sites.md` memory — grep-verified at spec time):
1. `pkg/script/active.go:341` — interface
2. `pkg/script/handlers_npc.go:182` — handler call
3. `modules/world/npc_masks.go:16` — concrete impl
4. `pkg/script/handlers_npc_test.go:79` — mockNpc
5. `pkg/script/handlers_player_test.go:31` — mockActiveNpc
6. `modules/world/npc_test.go:43` — existing test call

The `modules/world/npc_event_queue_test.go:37` site writes `n.typeId = 99` directly (not via `ChangeType`) and is unaffected.

### Steps

- [ ] **Step 2.1 — Write failing tests (will not compile yet)**

This task breaks TDD-ordering slightly because a signature change means tests don't compile until the interface and ALL call sites update. We write the tests first as failing (pre-compile), then update all sites atomically.

**Add/update in `modules/world/npc_test.go`** — replace existing `TestNpcChangeTypeSetsMask` at lines 41-49:

```go
func TestNpcChangeTypeSetsMask(t *testing.T) {
	n := newTestNpc(1)
	n.ChangeType(42, 100)
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Error("NpcMaskChangeType should be set")
	}
	if n.changeTypeID != 42 {
		t.Errorf("changeTypeID: got %d, want 42", n.changeTypeID)
	}
	if n.typeId != 42 {
		t.Errorf("typeId: got %d, want 42 (NAI-16 — ChangeType now writes typeId)", n.typeId)
	}
	wantUID := (42 << 16) | n.nid
	if n.uid != wantUID {
		t.Errorf("uid: got %d, want %d (recomputed from new typeId)", n.uid, wantUID)
	}
	if n.lifecycleTick != 100 {
		t.Errorf("lifecycleTick: got %d, want 100 (schedules revert)", n.lifecycleTick)
	}
}

func TestNpcChangeTypeDurationZeroNoOp(t *testing.T) {
	n := newTestNpc(1)
	// Seed known state so "no-op" is observable.
	origTypeID := n.typeId
	origUID := n.uid
	origLifecycleTick := n.lifecycleTick
	origMasks := n.masks

	n.ChangeType(42, 0) // TS guard: duration < 1 → total no-op

	if n.typeId != origTypeID {
		t.Errorf("typeId: got %d, want %d (duration=0 should not write)", n.typeId, origTypeID)
	}
	if n.uid != origUID {
		t.Errorf("uid: got %d, want %d (duration=0 should not recompute)", n.uid, origUID)
	}
	if n.lifecycleTick != origLifecycleTick {
		t.Errorf("lifecycleTick: got %d, want %d (duration=0 should not write)", n.lifecycleTick, origLifecycleTick)
	}
	if n.masks != origMasks {
		t.Errorf("masks: got %d, want %d (duration=0 should not raise mask)", n.masks, origMasks)
	}
}

func TestNpcChangeTypeDeadNoOp(t *testing.T) {
	n := newTestNpc(1)
	n.dead = true
	origTypeID := n.typeId
	origMasks := n.masks

	n.ChangeType(42, 100) // TS guard: !isActive → total no-op

	if n.typeId != origTypeID {
		t.Errorf("typeId: got %d, want %d (dead NPC should not morph)", n.typeId, origTypeID)
	}
	if n.masks != origMasks {
		t.Errorf("masks: got %d, want %d (dead NPC should not raise mask)", n.masks, origMasks)
	}
}
```

**Add in `pkg/script/handlers_npc_test.go`** — update the existing `mockNpc.ChangeType` at lines 79-81 and the `changeTypeCalls` field declaration (grep the same file for `changeTypeCalls` to find the field), then add a new test.

Update the `changeTypeCalls` field (grep earlier in the same file for `changeTypeCalls` — search for the field declaration on `mockNpc`):

```go
changeTypeCalls []struct{ newType, duration int }
```

Update the `ChangeType` method at lines 79-81:

```go
func (m *mockNpc) ChangeType(newType, duration int) {
	m.changeTypeCalls = append(m.changeTypeCalls, struct{ newType, duration int }{newType, duration})
}
```

Add new test at end of `pkg/script/handlers_npc_test.go`:

```go
func TestHandleNpcChangeTypePassesDuration(t *testing.T) {
	s, m := newNpcHandlerStateForTest()
	s.PushInt(42)  // newType (pushed first, popped second per TS order)
	s.PushInt(100) // duration (pushed second, popped first — TS: duration on top)

	if err := handleNpcChangeType(s); err != nil {
		t.Fatalf("handleNpcChangeType: %v", err)
	}

	if len(m.changeTypeCalls) != 1 {
		t.Fatalf("changeTypeCalls: got %d, want 1", len(m.changeTypeCalls))
	}
	if got := m.changeTypeCalls[0]; got.newType != 42 || got.duration != 100 {
		t.Errorf("changeTypeCalls[0]: got (newType=%d, duration=%d), want (42, 100)",
			got.newType, got.duration)
	}
}
```

**Note on `newNpcHandlerStateForTest`:** If this helper doesn't exist in the test file, use whatever existing helper pattern `handleNpcChangeType`'s sibling tests use (grep for `func handle` pattern usages in `handlers_npc_test.go`). If no such helper exists, construct the `ScriptState` inline using the same pattern as the nearest existing handler test (expected to follow a `state := &ScriptState{ActiveNpc: &mockNpc{...}}` shape).

- [ ] **Step 2.2 — Update the `ActiveNpc` interface**

In `pkg/script/active.go`, replace line 341 and its doc comment (lines 339-341):

```go
	// ChangeType morphs the NPC to newType and schedules a revert to
	// baseType after `duration` ticks. No-op when duration < 1 OR when
	// the NPC is dead. Mirrors TS Npc.changeType at
	// Engine-TS/.../Npc.ts:427-449.
	//
	// DEFERRED: the optional `reset=false` variant (NPC_CHANGETYPE_KEEPALL
	// opcode 2506) and the stats-reset branch at TS:436-443 require
	// baseLevels/levels arrays not yet on *Npc. See the NAI-16 spec's
	// Out-of-scope section.
	ChangeType(newType, duration int)
```

- [ ] **Step 2.3 — Update the concrete `*Npc.ChangeType` impl**

In `modules/world/npc_masks.go`, replace the entire `ChangeType` function at lines 16-19:

```go
// ChangeType morphs the NPC to newType and schedules a revert to
// baseType after `duration` ticks. Mirrors TS Npc.changeType at
// Engine-TS/.../Npc.ts:427-449.
//
// Semantics:
//   - No-op when duration < 1 (TS guard; rejects 0 and negatives in
//     one check) OR when the NPC is dead (TS `!this.isActive`).
//   - On success: writes typeId, recomputes uid, writes lifecycleTick
//     (consumed by the Events block at npc_ai.go:27-43 to fire
//     revertType when it hits 0 on RESPAWN+alive), writes the mask
//     payload field changeTypeID, raises NpcMaskChangeType.
//
// DEFERRED (TS parity gaps, left for a follow-up sub-spec):
//   - Stats-reset branch (TS:436-443) — requires baseLevels/levels
//     arrays on *Npc which don't exist yet. Current engine has only
//     curHP/baseHP; a full 6-stat array port is a separate concern.
//   - The optional `reset=false` flag and its NPC_CHANGETYPE_KEEPALL
//     opcode (opcode 2506 is a reserved constant at
//     pkg/script/opcode.go:243 with no handler). Wiring KEEPALL
//     requires the stats-array infra above, so both land together.
//   - The `type === baseType && RESPAWN → setLifeCycle(-1)` fast-path
//     (TS:444-445) — minor corner case; current behavior writes
//     lifecycleTick=duration unconditionally, which fires a harmless
//     no-op revert at tick 0 (revertType is idempotent when
//     typeId == baseType).
func (n *Npc) ChangeType(newType, duration int) {
	if duration < 1 || n.dead {
		return
	}
	n.typeId = newType
	n.uid = (n.typeId << 16) | n.nid
	n.lifecycleTick = duration
	n.changeTypeID = newType
	n.masks |= rsbuf.NpcMaskChangeType
}
```

- [ ] **Step 2.4 — Update the handler**

In `pkg/script/handlers_npc.go`, replace lines 173-184:

```go
// handleNpcChangeType pops (newType, duration) in TS order (duration
// on top) and morphs the NPC. Matches TS NpcOps.ts:457-462.
//
// DEFERRED: NPC_CHANGETYPE_KEEPALL (opcode 2506) has a reserved
// constant at pkg/script/opcode.go:243 but no handler yet — requires
// the `reset=false` variant of ChangeType, see active.go.
func handleNpcChangeType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	duration := s.PopInt()
	newType := s.PopInt()
	s.ActiveNpc.ChangeType(newType, duration)
	return nil
}
```

- [ ] **Step 2.5 — Update `mockActiveNpc` in `handlers_player_test.go`**

In `pkg/script/handlers_player_test.go` at line 31, replace:

```go
func (m *mockActiveNpc) ChangeType(newType, duration int)     {}
```

- [ ] **Step 2.6 — Confirm `mockNpc` update from Step 2.1 is in place**

Re-verify that `pkg/script/handlers_npc_test.go`'s `mockNpc.ChangeType` uses the new `(newType, duration int)` signature and that `changeTypeCalls` field is `[]struct{ newType, duration int }` (both set in Step 2.1).

- [ ] **Step 2.7 — Run the full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: build succeeds (if it fails, a call-site update was missed — grep `ChangeType(` to find the unconverted site).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests PASS, specifically:
- `TestNpcChangeTypeSetsMask` — PASS with new assertions
- `TestNpcChangeTypeDurationZeroNoOp` — PASS
- `TestNpcChangeTypeDeadNoOp` — PASS
- `TestHandleNpcChangeTypePassesDuration` — PASS
- All existing `TestNpc*` and `TestHandleNpc*` tests — PASS (regression)

- [ ] **Step 2.8 — Commit**

```bash
git add pkg/script/active.go pkg/script/handlers_npc.go modules/world/npc_masks.go \
  pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go modules/world/npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-16 Task 2 ChangeType duration wiring

Extends ActiveNpc.ChangeType to (newType, duration int) and ports
TS Npc.changeType (Engine-TS/.../Npc.ts:427-449) minus the DEFERRED
stats-reset and KEEPALL branches.

*Npc.ChangeType now writes typeId + recomputes uid + schedules the
lifecycleTick revert, closing a latent correctness bug where
changetype'd NPCs never actually morphed server-side (NPC_TYPE reads
returned the pre-changetype value).

TS `duration < 1` and `!isActive` guards are preserved: passing
duration=0 OR calling on a dead NPC is a total no-op.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Item C: RESPAWN+alive morph-revert direct test

**Files:**
- Test: `modules/world/npc_ai_test.go` (add 1 test; note: test file may need to be created if it doesn't exist, but lifecycle-adjacent tests currently live in `npc_event_queue_test.go`)

**Rationale:** Closes the NAI-5 test-gap #2 flagged by the NAI-5 final reviewer — the `lifecycle=Respawn && !dead` branch at `npc_ai.go:37-40` (alive-morph revert) is only exercised indirectly by `TestNpcTurnEventsRespawnPathAfterKill`. A direct unit test pins the revert code path to its own assertions.

**Critical note:** The test MUST use manual state setup (do NOT route through `ChangeType` from Task 2). The memory's "simulate a changetype" phrasing isolates the test from Item B so branch coverage stays clean. If the test went through `ChangeType`, a regression in `ChangeType` could mask a regression in `revertType` and vice versa.

### Steps

- [ ] **Step 3.1 — Locate or create `npc_ai_test.go`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ls -la modules/world/npc_ai_test.go`

Or use the file-system tool. If the file exists, append to it. If it does not exist, create it with this header:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

If the file already exists with a different import set, merge imports into the existing block rather than adding a duplicate import statement.

- [ ] **Step 3.2 — Write the failing test**

Append to `modules/world/npc_ai_test.go`:

```go
// TestNpcTurnRespawnAliveMorphReverts directly exercises the
// `lifecycle=Respawn && !dead` branch at npc_ai.go:37-40: when an
// alive morphed NPC's lifecycleTick hits 0, revertType() fires and
// typeId is restored to baseType.
//
// NAI-5 originally covered this branch only indirectly through
// TestNpcTurnEventsRespawnPathAfterKill (which tests the dead-npc
// respawn path). This direct test isolates the alive-morph branch
// and its revertType() invocation.
//
// DELIBERATELY does NOT use (*Npc).ChangeType to set up the morph —
// the point is to assert revertType()'s post-condition without
// depending on ChangeType's semantics. See NAI-16 spec § 4.
func TestNpcTurnRespawnAliveMorphReverts(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	// Simulate post-changetype state: typeId mutated, uid recomputed,
	// lifecycleTick scheduling a revert in 3 ticks.
	n.typeId = 99
	n.uid = (99 << 16) | n.nid
	n.lifecycle = NpcLifecycleRespawn
	n.dead = false
	n.lifecycleTick = 3
	n.masks = 0 // clear mask so we can assert revertType raises it

	for range 3 {
		n.turn(s)
	}

	if n.typeId != n.baseType {
		t.Errorf("typeId: got %d, want baseType %d (revertType should restore)", n.typeId, n.baseType)
	}
	wantUID := (n.baseType << 16) | n.nid
	if n.uid != wantUID {
		t.Errorf("uid: got %d, want %d (recomputed from baseType)", n.uid, wantUID)
	}
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Error("masks: NpcMaskChangeType bit not set (revertType should raise it)")
	}
	if !n.tele {
		t.Error("tele: got false, want true (revertType raises it)")
	}
}
```

- [ ] **Step 3.3 — Run test, confirm it PASSES**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcTurnRespawnAliveMorphReverts' -v`

Expected: PASS. (Unlike Tasks 1 and 2, Task 3 is a pure test backfill — no production code changes. The branch already works; we're adding direct assertions.)

If the test FAILS, that is a surprise regression in `revertType` or the Events block and MUST be investigated before continuing — it would mean existing indirect tests were not covering what they appeared to.

- [ ] **Step 3.4 — Run full-package regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: all tests PASS.

- [ ] **Step 3.5 — Commit**

```bash
git add modules/world/npc_ai_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-16 Task 3 direct RESPAWN+alive morph-revert test

Closes NAI-5 test-gap #2. The `lifecycle=Respawn && !dead` branch at
npc_ai.go:37-40 was previously covered only indirectly via
TestNpcTurnEventsRespawnPathAfterKill. The new test uses manual
state setup (not *Npc.ChangeType) to isolate the revert path from
Task 2's ChangeType semantics.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — Item D: `processNpcEventQueue` happy-path fire test

**Files:**
- Test: `modules/world/npc_event_queue_test.go` (add 1 test)

**Rationale:** Closes the NAI-5 test-gap #1. `TestProcessNpcEventQueueSkipsDelayedNpcs` covers the skip branch; `TestNpcTurnEventsDespawnEnqueuesEvent` covers enqueue without firing (no script registered). The fire+remove path has no direct test. Observability relies on `resumeOrFinishNpc` calling `npc.ClearActiveScript()` on `Finished` execution — asserted as `n.activeScript == nil` plus empty queue.

### Steps

- [ ] **Step 4.1 — Write the failing test**

Append to `modules/world/npc_event_queue_test.go`:

```go
// TestProcessNpcEventQueueHappyPathFire guards the fire+remove path
// of processNpcEventQueue at modules/world/npc_event_queue.go:36-48.
// A non-delayed NPC with a queued event runs through runNpcScript →
// resumeOrFinishNpc → (on Finished) ClearActiveScript. Observability:
// queue drained + activeScript cleared.
//
// Closes NAI-5 test-gap #1. Complement to
// TestProcessNpcEventQueueSkipsDelayedNpcs (skip branch) and
// TestNpcTurnEventsDespawnEnqueuesEvent (enqueue-no-fire).
func TestProcessNpcEventQueueHappyPathFire(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:    "ai_despawn_stub",
		Opcodes: []script.Opcode{script.OpReturn},
	}
	s.npcEventQueue = append(s.npcEventQueue, NpcEventRequest{
		Type:   NpcEventDespawn,
		Script: sf,
		Npc:    n,
	})

	s.processNpcEventQueue()

	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (queue drained after fire)", len(s.npcEventQueue))
	}
	if n.activeScript != nil {
		t.Error("activeScript: got non-nil, want nil (Finished execution should ClearActiveScript)")
	}
}
```

- [ ] **Step 4.2 — Run test, confirm it PASSES**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessNpcEventQueueHappyPathFire' -v`

Expected: PASS. (Task 4 is pure test backfill like Task 3 — no production code changes. The fire path already works; we're adding direct assertions.)

If the test FAILS with `activeScript: got non-nil`, investigate whether `ScriptFile{Opcodes: [OpReturn]}` produces the `Finished` execution state. If Execute's path for a bare `OpReturn` does NOT route through `resumeOrFinishNpc`'s Finished branch (e.g., if OpReturn needs a stack frame that's not set up), swap the observable to a counter on `s.scriptProvider` via a custom fake — use the same shape as `rsbuf.SetObserverForTest` (a package-level helper that installs a test-only replacement).

- [ ] **Step 4.3 — Run full-package regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: all tests PASS.

- [ ] **Step 4.4 — Commit**

```bash
git add modules/world/npc_event_queue_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-16 Task 4 processNpcEventQueue happy-path fire test

Closes NAI-5 test-gap #1. Complements TestProcessNpcEventQueueSkipsDelayedNpcs
(skip branch) and TestNpcTurnEventsDespawnEnqueuesEvent (enqueue-no-fire)
by covering the fire-and-remove path. Uses ScriptFile{OpReturn} fixture
with Finished-execution → ClearActiveScript as the observability hook.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5 — NAI-16 close: memory updates + follow-ups + close commit

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
- Possible new memory entry (see Step 5.2)

**Rationale:** Per `runescript_cadence.md`, every NAI-N closes with a `chore(nai)` commit and memory cleanup. Per `close_commit_memory_trailer.md`, the close commit uses a `Closes memory:` trailer when a new memory entry lands. Per `post_task_handoff.md`, the task ends with a paste-ready resume prompt for the user.

### Steps

- [ ] **Step 5.1 — Update `nai_followups.md`**

Add resolution preambles (not deletions) to existing entries. Open `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` and update in-place:

**(a)** Under "## From NAI-5 (2026-04-22)" → "### Unassigned (small fix): wire `npc_changetype` duration into new Events block", insert at top of entry (above the original description):

```markdown
**Resolved 2026-04-23 (NAI-16 Task 2).** `ActiveNpc.ChangeType` now
takes `(newType, duration int)`; `*Npc.ChangeType` writes `typeId`,
recomputes `uid`, sets `lifecycleTick = duration`, and raises the
mask. TS guards preserved: `duration < 1` OR `n.dead` → total no-op.
Stats-reset and `NPC_CHANGETYPE_KEEPALL` (opcode 2506) remain
deferred — see Out-of-scope below for the bundling plan.

---

_Original entry body:_
```

**(b)** Under "### Test gaps flagged by NAI-5 final reviewer", insert at top:

```markdown
**Resolved 2026-04-23 (NAI-16 Tasks 3 & 4).** Both gaps closed:
`TestNpcTurnRespawnAliveMorphReverts` (Task 3) directly exercises
the `lifecycle=Respawn && !dead` morph-revert branch;
`TestProcessNpcEventQueueHappyPathFire` (Task 4) directly exercises
the fire+remove path of `processNpcEventQueue`.

---

_Original entry body:_
```

**(c)** Under "## From NAI-8 (2026-04-22)" → "### Deferred filters in huntPlayers (future audit)" → item 4 `checkNotCombatSelf`, update the existing "remaining deferrals" subsection. Find the text:

```markdown
4. **checkNotCombatSelf (TS:946-948)** — NAI-15 wired the shared outer
   guard but left this filter as an inline DEFERRED comment inside the
   guarded block. **Blocker:** no NPC-vars infrastructure yet.
```

Replace it with:

```markdown
4. **checkNotCombatSelf (TS:946-948)** — **Resolved 2026-04-23
   (NAI-16 Task 1).** The "no NPC-vars infrastructure yet" blocker
   claim was stale: `Npc.varns`, `NpcVarN`, and `SetNpcVarN` all
   landed in S6a. NAI-16 wires the filter at the DEFERRED site using
   the existing infrastructure. No new VarNpcType config registry
   was needed — consistent with the existing Go pattern that does
   not use `VarPlayerType` indirection on the player-side `Varp`
   reader either.
```

Also remove the "Required scope for a future sub-spec: `VarNpcType` config registry..." block that follows (it is now obsolete).

**(d)** Add a new entry to the file for NAI-16's Out-of-scope items (under a new "## From NAI-16 (2026-04-23)" section at the end):

```markdown
## From NAI-16 (2026-04-23)

### Deferred: NPC stats-array + KEEPALL variant

NAI-16 ported TS `Npc.changeType` minus the stats-reset branch
(TS:436-443) and minus the `reset=false` path consumed by opcode
`NPC_CHANGETYPE_KEEPALL` (opcode 2506, a reserved constant at
`pkg/script/opcode.go:243` with no handler). Both require a full
6-stat array on `*Npc` (`baseLevels[]` + `levels[]`) that doesn't
exist yet — current engine has only `curHP`/`baseHP`.

Suggested future sub-spec: "NPC stat arrays + KEEPALL". Scope:
`Npc.levels [6]int` + `Npc.baseLevels [6]int` fields, seed from
`NpcType.Stats` at `NewNpc` time, extend `*Npc.NpcStat` /
`*Npc.NpcBaseStat` to read from these arrays, port the TS stats-reset
loop into `*Npc.ChangeType`, add KEEPALL handler. Estimated
~100-150 LOC.

### Minor deviation (low priority): changeType baseType fast-path

TS `Npc.changeType` has a `type === baseType && lifecycle === RESPAWN
→ setLifeCycle(-1)` fast-path at TS:444-445 that skips scheduling a
revert when morphing back to base. Go's current implementation writes
`lifecycleTick = duration` unconditionally, which fires a harmless
no-op `revertType` at tick 0 (revertType is idempotent when
`typeId == baseType`). Low priority — correctness is identical,
only perf cost of one extra branch-take + one extra mask emission.
Fold into a future polish pass.
```

- [ ] **Step 5.2 — (Optional) Add a new memory entry about stale-blocker verification**

Consider adding a standalone memory entry capturing this session's finding: "When a memory record says `X infra is missing`, grep for plausible existing names before starting the brainstorm". The `verify_implementer_claims.md` memory covers this at a general level; a NAI-specific entry is likely redundant. **Skip unless user explicitly requests during review.**

- [ ] **Step 5.3 — Run the final full-package test pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests PASS across the entire module tree.

- [ ] **Step 5.4 — Final fresh-verification grep (per `verify_implementer_claims.md`)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no findings.

Grep for leftover DEFERRED/TODO markers that shouldn't survive NAI-16:

Run: `grep -rn "checkNotCombatSelf.*DEFERRED\|NPC-vars infra" modules/ pkg/`

Expected: NO matches. If any match, the doc-comment update in Task 1 Step 1.4 missed a site — fix before closing.

- [ ] **Step 5.5 — Close commit**

```bash
git add -A  # includes memory file via absolute path; verify via git status first
git status  # sanity-check what will be committed
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(nai): NAI-16 closed — deferral sweep (4 items)

Bundles closures for four accumulated NAI-5 and NAI-8 deferrals:

Task 1 (Item A) — checkNotCombatSelf filter in huntPlayers.
  The "no NPC-vars infra" blocker in nai_followups.md was stale —
  S6a had already shipped Npc.varns + NpcVarN + SetNpcVarN.

Task 2 (Item B) — NPC_CHANGETYPE duration wiring. Ports TS
  Npc.changeType's typeId/uid/lifecycleTick writes and duration
  guard. Closes a latent correctness bug where NPCs never morphed
  server-side.

Task 3 (Item C) — Direct RESPAWN+alive morph-revert test.
Task 4 (Item D) — Direct processNpcEventQueue happy-path fire test.

Deferrals tracked forward: NPC stats-array + KEEPALL variant;
changeType baseType fast-path (minor).

Closes memory: plan_enumerate_struct_literals.md note confirmed —
VarNpcType registry was not needed (S6a had already landed the
equivalent infra under the `varns` name).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Safety note on `git add -A`:** Before Step 5.5, run `git status` FIRST to confirm only expected files are staged. The memory file lives outside the repo (`~/.claude/projects/.../memory/nai_followups.md`) and is NOT tracked by this repo's git — so the commit only captures repo-internal files. The memory update is a plain filesystem change, not git-tracked.

---

## Self-Review (already run inline during plan authoring)

**1. Spec coverage:** Every spec Goal item maps to a task:
- Goal §1 (filter wired) → Task 1
- Goal §2 (ChangeType duration) → Task 2
- Goal §3 (morph-revert test) → Task 3
- Goal §4 (event-queue fire test) → Task 4
- Goal §5 (memory updates) → Task 5
- Goal §6 (doc comment rewrite) → Task 1 Step 1.4
- Goal §7 (DEFERRED comment at ChangeType) → Task 2 Step 2.3

**2. Placeholder scan:** No TBDs. No "add appropriate error handling". No "similar to Task N" (each task's code is spelled out). One intentional fallback branch (Task 4 Step 4.2 "if it FAILS" clause) — this is a diagnostic hint, not a placeholder.

**3. Type consistency:**
- `ChangeType(newType, duration int)` — signature consistent across interface (2.2), impl (2.3), handler (2.4), both mocks (2.1, 2.5), all test sites
- `changeTypeCalls []struct{ newType, duration int }` — field shape consistent between mockNpc declaration and test assertion
- `NpcVarN(id int) int32` — existing symbol, used only in Task 1
- `SetNpcVarN(id int, val int32)` — existing symbol, used only in Task 1

---

## Post-implementation handoff (per `post_task_handoff.md` memory)

After Task 5 closes, provide the user with:

1. A summary of the 5 commits on `main`.
2. Any new memory entries created or updated.
3. A paste-ready resume prompt for the next NAI session, enumerating remaining deferred items.
