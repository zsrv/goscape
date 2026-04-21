# APNPC Approach-Range Gating Implementation Plan (S6n)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close S6l-D2 by wiring the NPC-side parallel to S6l's APLOC gating: when the player approaches an NPC within the NPC's per-type `AttackRange`, fire `[apnpc<op>,<npcType>]` trigger scripts.

**Architecture:** Two tasks. Task 1 introduces `effectiveApRange(p)` in `modules/world/interaction.go` — a tiny helper that returns `npc.typ.AttackRange` for `*Npc` targets and `p.apRange` for `*Loc`/other. `processInteraction` swaps its inline `p.apRange` for this helper. Task 2 adds `apNpcTriggerForOp` (mirrors S6m's `apLocTriggerForOp` for 1..5 only), `fireApTriggerNpc` (mirrors S6l's `fireApTriggerLoc` with three documented NPC divergences: `npc.dead` gate, cached `npc.typ.Category`, no `apRangeCalled` persistence), and wires the `*Npc` case in `tryFireApTrigger`.

**Tech Stack:** Go 1.26 (stdlib only). Tests reuse existing `newTestServer`, `newTestPlayer`, `NewNpc`, `newTriggerFixture` (from S6j-era `interaction_trigger_test.go`), and `newNoopScriptFile` helpers.

**Spec reference:** `docs/superpowers/specs/2026-04-21-runescript-s6n-apnpc-range-gating-design.md` (commit `53812a6`).

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
| `modules/world/interaction.go` | Modify | Add `effectiveApRange(p *Player) int`; swap `p.apRange` → `effectiveApRange(p)` in AP branch | 1 |
| `modules/world/interaction_test.go` | Modify | 4 tests (3 helper unit + 1 integration) | 1 |
| `modules/world/interaction_trigger.go` | Modify | Add `apNpcTriggerForOp` + `fireApTriggerNpc`; wire `*Npc` case in `tryFireApTrigger` | 2 |
| `modules/world/interaction_trigger_test.go` | Modify | 2 helper unit tests + 5 fire tests | 2 |

**Existing infrastructure already in place (verify, don't modify):**
- `TriggerApNpc1 = 3` ... `TriggerApNpc5 = 7`; `TriggerOpNpc1 = 10` ... `TriggerOpNpc5 = 14` — `pkg/script/trigger.go:12+19` (`+7` offset verified)
- `NpcType.AttackRange uint16` at `pkg/objtype/npctype.go:85`; decoder opcode 207 at line 190
- `Npc` satisfies `entity` interface (Slot, Coords) — established in S6j
- `handleOpNpc` passes `, -1` for com (S6m signature) — unchanged
- `inApproachDistance(px, pz, tx, tz, apRange int) bool` at `interaction.go:140` — generic over range; accepts any int
- `apLocTriggerForOp(op int) (script.ServerTriggerType, bool)` (S6m) — template for Task 2's `apNpcTriggerForOp`
- `fireApTriggerLoc` at `interaction_trigger.go:~244` (S6l) — template for Task 2's `fireApTriggerNpc`
- `tryFireApTrigger` at `interaction_trigger.go:215` — `*Loc` case wired; default branch for non-Loc. Task 2 wires `*Npc`.
- `fireOpTriggerNpc` at `interaction_trigger.go:49` — **UNTOUCHED in S6n** (scope-defer per §5.6 of spec)

---

## Task 1: effectiveApRange Helper + processInteraction Branch Swap

**Goal:** After this task, `processInteraction` uses the NPC's `AttackRange` as the approach-distance threshold when the target is an NPC, falling back to `p.apRange` for Loc/other targets. No APNPC script fires yet (Task 2 wires that); this task is a pure engine-side state-machine correctness change.

**Files:**
- Modify: `modules/world/interaction.go`
- Modify: `modules/world/interaction_test.go`

### Step-by-step

- [ ] **Step 1.1: Write failing test for `effectiveApRange` with NPC target**

In `modules/world/interaction_test.go`, append:

```go
// TestEffectiveApRangeNpcUsesTypeAttackrange verifies that when the
// player's target is an *Npc, effectiveApRange returns the NPC's
// per-type AttackRange — NOT the Player's mutable apRange field. This
// is the core TS divergence S6n wires: TS Npc.checkApTrigger
// (Npc.ts:~876) reads type.attackrange for NPC approach checks.
func TestEffectiveApRangeNpcUsesTypeAttackrange(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRange = 10 // Player-side mutable default — should be IGNORED for NPC

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 5,
	}
	npc := NewNpc(0, 7, 100, 100, 0, npcType)
	p.target = npc

	if got := effectiveApRange(p); got != 5 {
		t.Errorf("effectiveApRange: got %d, want 5 (npc.typ.AttackRange)", got)
	}
}
```

NOTE: If `objtype` is not yet imported in `interaction_test.go`, add it.

- [ ] **Step 1.2: Run test to verify compile failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestEffectiveApRangeNpcUsesTypeAttackrange -v`

Expected: compile failure — `undefined: effectiveApRange`.

- [ ] **Step 1.3: Implement `effectiveApRange`**

In `modules/world/interaction.go`, append after the existing `inApproachDistance` function (near the end of the file):

```go
// effectiveApRange returns the approach-range in tiles the player's
// current target should be checked against by inApproachDistance.
// For *Npc targets: the NPC's NpcType.AttackRange (fixed per-type,
// never mutated). For *Loc and all other targets: p.apRange (the
// mutable Player field, defaulted to 10 in SetInteraction and
// settable via p_aprange per S6l).
//
// Matches TS Npc.checkApTrigger (Npc.ts:~876) which reads
// type.attackrange, diverging from Player.tryInteract (Player.ts:~1139)
// which reads player.apRange.
//
// Returns 0 (which inApproachDistance rejects) if the target is an
// NPC with a nil NpcType — defensive guard; production cache always
// registers NpcType for any spawned NPC. Edge case: NpcType with
// AttackRange == 0 (uninitialized) will also yield 0 here, meaning
// APNPC never fires for that NPC. Intentional — production cache
// always sets attackrange for NPCs that have AP scripts.
func effectiveApRange(p *Player) int {
	if npc, ok := p.target.(*Npc); ok {
		if npc.typ == nil {
			return 0
		}
		return int(npc.typ.AttackRange)
	}
	return p.apRange
}
```

- [ ] **Step 1.4: Run test to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestEffectiveApRangeNpcUsesTypeAttackrange -v`

Expected: PASS.

- [ ] **Step 1.5: Add the remaining 3 tests**

Append to `modules/world/interaction_test.go`:

```go
// TestEffectiveApRangeLocUsesPlayerApRange verifies that for non-NPC
// targets (e.g. *Loc), effectiveApRange falls back to p.apRange — the
// mutable Player field that S6l's p_aprange opcode writes to.
func TestEffectiveApRangeLocUsesPlayerApRange(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRange = 7 // custom, simulating a p_aprange call

	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	p.target = loc

	if got := effectiveApRange(p); got != 7 {
		t.Errorf("effectiveApRange: got %d, want 7 (p.apRange for Loc target)", got)
	}
}

// TestEffectiveApRangeNilNpcTypeReturnsZero verifies the defensive
// guard: an NPC with a nil typ pointer returns 0 (which
// inApproachDistance rejects), preventing APNPC from firing against
// a malformed NPC that lacks a registered NpcType.
func TestEffectiveApRangeNilNpcTypeReturnsZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.apRange = 10

	// NPC with typ == nil (defensive — production NPCs always have typ set).
	npc := NewNpc(0, 7, 100, 100, 0, nil)
	p.target = npc

	if got := effectiveApRange(p); got != 0 {
		t.Errorf("effectiveApRange: got %d, want 0 (nil typ defensive)", got)
	}
}

// TestProcessInteractionNpcUsesAttackrange is an integration test:
// place an NPC with AttackRange=5 at dx=6 from the player, with
// p.apRange=10. Without the S6n change, the old code would see dx=6
// <= p.apRange=10 and take the AP branch. With S6n, dx=6 > AttackRange=5
// so the pathing branch is taken — proving the helper routes correctly.
func TestProcessInteractionNpcUsesAttackrange(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0
	p.apRange = 10

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 5,
	}
	npc := NewNpc(0, 7, 106, 100, 0, npcType) // dx=6: within p.apRange=10 but past AttackRange=5
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	p.processInteraction()

	// AP branch would set p.interacted = true. Pathing branch does NOT.
	// If effectiveApRange returned p.apRange=10, AP branch fires and
	// p.interacted=true. If it returned AttackRange=5, pathing branch
	// fires and p.interacted stays false (or pathing sets repathed=true).
	if p.interacted {
		t.Error("p.interacted: got true, want false — AP branch should NOT fire (dx=6 > AttackRange=5)")
	}
	if !p.repathed {
		t.Error("p.repathed: got false, want true — pathing branch should fire when out of AP range")
	}
}
```

NOTE: If `entitypkg` is not yet imported, add `entitypkg "github.com/zsrv/goscape/pkg/entity"`.

- [ ] **Step 1.6: Run the 3 new tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestEffectiveApRange|TestProcessInteractionNpcUsesAttackrange" -v`

Expected: 4 tests PASS.

Note: `TestProcessInteractionNpcUsesAttackrange` passes right now even before Step 1.7 runs, because... wait. Let me re-reason: the helper is defined, but `processInteraction` still reads `p.apRange` directly (we haven't swapped it yet in Step 1.7). So with p.apRange=10 and dx=6, the AP branch WOULD fire, meaning `p.interacted` becomes `true`. The test SHOULD FAIL before the swap. Re-run and observe.

**Actually run** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessInteractionNpcUsesAttackrange -v` and confirm it FAILS with `p.interacted: got true, want false`. If it passes now, something is off (verify by inspecting the test output).

The other 3 tests should PASS.

- [ ] **Step 1.7: Swap `processInteraction`'s AP branch to use `effectiveApRange`**

In `modules/world/interaction.go`, find the AP branch at line ~99:

```go
	if inApproachDistance(p.x, p.z, tx, tz, p.apRange) {
```

Replace with:

```go
	if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
```

No other changes.

- [ ] **Step 1.8: Run the integration test to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessInteractionNpcUsesAttackrange -v`

Expected: PASS now that the swap is in place.

- [ ] **Step 1.9: Run the full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass. Particularly verify:
- S6l's `TestProcessInteractionRoutesToApBranch` still passes (uses `*Loc` target; `effectiveApRange` correctly falls back to `p.apRange` for Loc)
- S6l's `TestProcessInteractionOutOfRangePaths` still passes (same — Loc target path)

- [ ] **Step 1.10: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 1.11: Commit Task 1**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): effectiveApRange helper + NPC-attackrange routing in processInteraction (S6n-1)

processInteraction's AP branch previously hardcoded p.apRange for
approach-distance checks — correct for *Loc targets, WRONG for *Npc
targets which should use the NPC's per-type attackrange (fixed,
immutable) per TS Npc.checkApTrigger (Npc.ts:~876).

Adds effectiveApRange(p *Player) int helper: for *Npc targets
returns npc.typ.AttackRange; for *Loc and all other targets falls
back to p.apRange. Nil-guarded: NPC with nil typ returns 0
(inApproachDistance rejects).

processInteraction swaps its inline p.apRange to call
effectiveApRange(p). No APNPC script fires yet — tryFireApTrigger's
default-for-Npc branch (from S6l) still short-circuits. Task 2
(S6n-2) wires fireApTriggerNpc to complete the path.

4 tests: 3 helper-unit (NPC-AttackRange, Loc-apRange, nil-typ-zero)
+ 1 integration (NPC at dx=6 with AttackRange=5 takes pathing branch
instead of AP, proving attackrange is the gate, not p.apRange=10).

Spec: docs/superpowers/specs/2026-04-21-runescript-s6n-apnpc-range-gating-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6n-apnpc-range-gating.md (Task 1)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: apNpcTriggerForOp + fireApTriggerNpc + tryFireApTrigger *Npc Case

**Goal:** Wire the APNPC trigger fire path. After this task, `[apnpc<op>,<npcType>]` scripts execute when the player reaches the NPC's `AttackRange`. S6l-D2 closes.

**Files:**
- Modify: `modules/world/interaction_trigger.go`
- Modify: `modules/world/interaction_trigger_test.go`

### Step-by-step

- [ ] **Step 2.1: Write failing unit tests for `apNpcTriggerForOp`**

In `modules/world/interaction_trigger_test.go`, append:

```go
// TestApNpcTriggerForOpValidValues table-tests the 1..5 op mapping:
//   1..5 → TriggerApNpc1..5 (3..7)
// fireOpTriggerNpc derives OPNPC triggers by adding 7 (10..14).
func TestApNpcTriggerForOpValidValues(t *testing.T) {
	cases := []struct {
		op   int
		want script.ServerTriggerType
		name string
	}{
		{1, script.TriggerApNpc1, "OpNpc1"},
		{2, script.TriggerApNpc2, "OpNpc2"},
		{3, script.TriggerApNpc3, "OpNpc3"},
		{4, script.TriggerApNpc4, "OpNpc4"},
		{5, script.TriggerApNpc5, "OpNpc5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := apNpcTriggerForOp(c.op)
			if !ok {
				t.Fatalf("op=%d: ok=false, want true", c.op)
			}
			if got != c.want {
				t.Errorf("op=%d: got %d, want %d", c.op, got, c.want)
			}
		})
	}
}

// TestApNpcTriggerForOpInvalidValues verifies out-of-range op values
// return ok=false. DEVIATION S6n-D1: APNPC T/U sentinels (6, 7 if
// eventually added) not wired — returns false for those too.
func TestApNpcTriggerForOpInvalidValues(t *testing.T) {
	invalid := []int{0, 6, 7, 8, -1, 100, -100}
	for _, op := range invalid {
		t.Run(fmt.Sprintf("op_%d", op), func(t *testing.T) {
			_, ok := apNpcTriggerForOp(op)
			if ok {
				t.Errorf("op=%d: ok=true, want false", op)
			}
		})
	}
}
```

NOTE: If `"fmt"` is not yet imported in this test file, add it.

- [ ] **Step 2.2: Run tests to verify compile failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestApNpcTriggerForOp -v`

Expected: compile failure — `undefined: apNpcTriggerForOp`.

- [ ] **Step 2.3: Implement `apNpcTriggerForOp`**

In `modules/world/interaction_trigger.go`, append near `apLocTriggerForOp` (search for that name to locate):

```go
// apNpcTriggerForOp returns the APNPC trigger for the player's
// targetOp. Returns ok=false if op is outside [1, 5]. fireOpTriggerNpc
// derives the OPNPC trigger by adding 7 to the returned APNPC (TS
// Player.ts:~997 offset convention):
//
//	APNPC1..5 (3..7) + 7 → OPNPC1..5 (10..14)
//
// NPC variant of apLocTriggerForOp. Does NOT handle T/U sentinels
// (DEVIATION S6n-D1) because OpNpcT/OpNpcU handlers are not wired
// in goscape yet — if those land, this helper's switch extends with
// matching cases.
func apNpcTriggerForOp(op int) (script.ServerTriggerType, bool) {
	if op >= 1 && op <= 5 {
		return script.TriggerApNpc1 + script.ServerTriggerType(op-1), true
	}
	return 0, false
}
```

- [ ] **Step 2.4: Run unit tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestApNpcTriggerForOp -v`

Expected: 5 subtests of `TestApNpcTriggerForOpValidValues` + 7 subtests of `TestApNpcTriggerForOpInvalidValues` PASS.

- [ ] **Step 2.5: Write failing test for APNPC no-script path**

Append to `modules/world/interaction_trigger_test.go`:

```go
// newApTriggerNpcFixture creates a fixture for fireApTriggerNpc tests:
// Server + Player + live Npc with typeID=7, AttackRange=5, categorized
// as Category=0. Player position is (100, 100); NPC is placed at
// (105, 100) — within AttackRange=5 but NOT at contact. targetOp=1.
// No APNPC script pre-registered; callers register one per-test.
func newApTriggerNpcFixture(t *testing.T) (*Server, *Player, *Npc) {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	npcType := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 7, DebugName: "rat"},
		AttackRange: 5,
		Category:    0,
	}
	npc := NewNpc(0, 7, 105, 100, 0, npcType) // dx=5, within AttackRange
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.interacted = true // simulate reach (as processInteraction would set)
	return s, p, npc
}

// TestFireApTriggerNpcNoScript verifies that when no APNPC script is
// registered for the NPC's (typeID, category, 1), fireApTriggerNpc
// clears the interaction silently and marks interactionFired=true.
func TestFireApTriggerNpcNoScript(t *testing.T) {
	s, p, _ := newApTriggerNpcFixture(t)

	fireApTriggerNpc(p, s, p.target.(*Npc))

	if p.target != nil {
		t.Error("target: expected cleared after no-script path")
	}
	if !p.interactionFired {
		t.Error("interactionFired: expected true")
	}
}
```

- [ ] **Step 2.6: Run test to verify compile failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestFireApTriggerNpcNoScript -v`

Expected: compile failure — `undefined: fireApTriggerNpc`.

- [ ] **Step 2.7: Implement `fireApTriggerNpc`**

In `modules/world/interaction_trigger.go`, append after `fireApTriggerLoc` (search for that name to locate the right position):

```go
// fireApTriggerNpc fires the [apnpc<op>,<npcType>] approach-trigger
// for the player's anchored NPC target when the player has reached
// the NPC's per-type attackrange. Matches TS Npc.ts:~861-883
// (checkApTrigger).
//
// Three divergences from fireApTriggerLoc (S6l):
//
//  1. Lifecycle gate is `npc.dead` (not locStillValid). NPCs have a
//     dedicated dead flag — no zone-membership pointer-stale check
//     needed because the *Npc reference itself is authoritative.
//
//  2. Category read from npc.typ.Category directly (the cached
//     pointer). fireApTriggerLoc does a locTypes.Configs[locId]
//     lookup because Loc has no cached LocType pointer, only a
//     packed Info bitfield.
//
//  3. NO apRangeCalled persistence contract. Per TS
//     (Npc.ts:~1064-1080): NPC AP scripts complete and clear
//     interaction unconditionally. The p_aprange persistence is
//     Player-side only; NPC attackrange is fixed per-type so
//     "extend the range" has no meaning. Simpler post-fire logic.
//
// DEVIATION S6n-D1: APNPC T/U sentinels not wired. OpNpcT/OpNpcU
// handlers don't exist in goscape yet; when they land,
// apNpcTriggerForOp gains matching cases and this fire function
// needs a sentinel-aware op-range gate update.
func fireApTriggerNpc(p *Player, srv *Server, npc *Npc) {
	if p.delayed && srv.currentTick < p.delayedUntil {
		return
	}

	if npc.dead {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger, ok := apNpcTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

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
```

- [ ] **Step 2.8: Run test to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestFireApTriggerNpcNoScript -v`

Expected: PASS.

- [ ] **Step 2.9: Add the remaining 4 fire tests**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireApTriggerNpcScriptFires verifies that with an APNPC1 script
// registered at (TriggerApNpc1, typeID=7, categoryID=-1), fireApTriggerNpc
// runs the script, binds ActiveNpc, and clears the interaction after
// Finished (no apRangeCalled persistence — TS divergence #3).
func TestFireApTriggerNpcScriptFires(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	sf := newNoopScriptFile(t, script.TriggerApNpc1, 7, -1)
	s.scriptProvider.Register(sf)

	fireApTriggerNpc(p, s, npc)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after script fire")
	}
}

// TestFireApTriggerNpcDeadNpc verifies the lifecycle gate: a dead NPC
// clears interaction silently (no script runs). Mirrors the S6j
// TestTryFireOpTrigger_DeadNpc pattern but on the AP path.
func TestFireApTriggerNpcDeadNpc(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	npc.dead = true

	fireApTriggerNpc(p, s, npc)

	if p.target != nil {
		t.Error("target: expected cleared for dead npc")
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after dead-clear")
	}
}

// TestFireApTriggerNpcDeferredOnDelay verifies that a delayed player
// short-circuits before any state change (no clear, no fire).
// Matches S6l's TestTryFireApTriggerLocDeferredOnDelay pattern.
func TestFireApTriggerNpcDeferredOnDelay(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	p.delayed = true
	p.delayedUntil = s.currentTick + 3

	fireApTriggerNpc(p, s, npc)

	if p.target == nil {
		t.Error("target: expected preserved while delayed")
	}
	if p.interactionFired {
		t.Error("interactionFired: expected false so next tick retries")
	}
}

// TestFireApTriggerNpcOpOutOfRange verifies that an invalid targetOp
// (e.g., 0 or 99) causes a silent interaction clear via the
// apNpcTriggerForOp gate.
func TestFireApTriggerNpcOpOutOfRange(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)
	p.targetOp = 99 // out of [1, 5]

	fireApTriggerNpc(p, s, npc)

	if p.target != nil {
		t.Error("target: expected cleared for out-of-range op")
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after silent clear")
	}
}
```

- [ ] **Step 2.10: Run all 5 fire tests to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireApTriggerNpc" -v`

Expected: 5 tests PASS.

- [ ] **Step 2.11: Wire the `*Npc` case in `tryFireApTrigger`**

In `modules/world/interaction_trigger.go`, find `tryFireApTrigger` (line ~215). Current shape:

```go
func tryFireApTrigger(p *Player) {
	srv := p.client.server

	switch tgt := p.target.(type) {
	case *entitypkg.Loc:
		fireApTriggerLoc(p, srv, tgt)
	default:
		// *Npc, *Obj, etc. — AP branch not yet wired. Mark fired to
		// prevent same-tick retry; processInteraction's branch ordering
		// ensures OP still fires if player reaches contact next tick.
		p.interactionFired = true
	}
}
```

Becomes:

```go
func tryFireApTrigger(p *Player) {
	srv := p.client.server

	switch tgt := p.target.(type) {
	case *entitypkg.Loc:
		fireApTriggerLoc(p, srv, tgt)
	case *Npc:
		fireApTriggerNpc(p, srv, tgt)
	default:
		// *Obj, etc. — AP branch not yet wired. Mark fired to prevent
		// same-tick retry. Follow-up: APOBJ sub-spec.
		p.interactionFired = true
	}
}
```

- [ ] **Step 2.12: Run the full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass. Particularly verify:
- S6l's `TestTryFireApTriggerLocNoScript` and other Loc-AP tests still pass (Loc case unchanged)
- S6j/S6m's `TestTryFireOpTrigger*` tests still pass (OP dispatcher untouched)

- [ ] **Step 2.13: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 2.14: Run race detector on modules/world**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

Expected: no races.

- [ ] **Step 2.15: Commit Task 2**

```bash
git add modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): fireApTriggerNpc + APNPC trigger dispatch (S6n-2)

Completes S6n end-to-end wiring. APNPC scripts now fire when the
player approaches within the NPC's per-type AttackRange (via S6n-1's
effectiveApRange + processInteraction's AP branch).

Mechanism:
- apNpcTriggerForOp(op) helper maps targetOp 1..5 to TriggerApNpc1..5.
  No T/U sentinels (DEVIATION S6n-D1 — OpNpcT/OpNpcU handlers don't
  exist yet).
- fireApTriggerNpc mirrors fireApTriggerLoc with 3 documented NPC
  divergences: (1) npc.dead lifecycle gate (not locStillValid),
  (2) cached npc.typ.Category (not Configs lookup), (3) no
  apRangeCalled persistence (NPC AP scripts clear unconditionally
  per TS Npc.ts:~1064-1080).
- tryFireApTrigger's *Npc case replaces S6l's default-branch stub.

fireOpTriggerNpc UNTOUCHED — still uses inline TriggerOpNpc1 + (op-1)
arithmetic. Byte-equivalent to apNpcTriggerForOp + 7 but scope-defer
of the refactor is intentional (bundle with OpNpcT/OpNpcU sub-spec
when that lands).

7 new tests:
- TestApNpcTriggerForOpValidValues (5 subtests, 1..5 mappings)
- TestApNpcTriggerForOpInvalidValues (7 subtests, out-of-range)
- TestFireApTriggerNpcNoScript (silent clear)
- TestFireApTriggerNpcScriptFires (script runs, ActiveNpc bound)
- TestFireApTriggerNpcDeadNpc (lifecycle gate)
- TestFireApTriggerNpcDeferredOnDelay (delayed short-circuit)
- TestFireApTriggerNpcOpOutOfRange (invalid op silent clear)

Milestone: S6l-D2 CLOSED. After S6n, S6l has 4 open deviations
(D1/D3/D4/D5 — all documented with scope-defer rationale).

Spec: docs/superpowers/specs/2026-04-21-runescript-s6n-apnpc-range-gating-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6n-apnpc-range-gating.md (Task 2)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes (for plan-author use)

**1. Spec coverage:**
- §1 Goal — Tasks 1+2 collectively wire APNPC end-to-end. ✅
- §2 Architecture — Task 1 (helper + branch swap) / Task 2 (fire path). ✅
- §3 File map — all 4 files appear in task headers. ✅
- §5.1 effectiveApRange — Task 1 Step 1.3. ✅
- §5.2 processInteraction AP branch swap — Task 1 Step 1.7. ✅
- §5.3 apNpcTriggerForOp — Task 2 Step 2.3. ✅
- §5.4 fireApTriggerNpc with 3 divergences — Task 2 Step 2.7. ✅
- §5.5 tryFireApTrigger *Npc case — Task 2 Step 2.11. ✅
- §5.6 fireOpTriggerNpc untouched — explicitly documented in Task 2 commit message. ✅
- §6 Test plan — 4 (Task 1) + 7 (Task 2) = 11 tests. ✅

**2. Type consistency:**
- `effectiveApRange(p *Player) int` signature consistent across Task 1 implementation + all test call sites. ✅
- `apNpcTriggerForOp(op int) (script.ServerTriggerType, bool)` signature consistent across Task 2 implementation + tests. ✅
- `fireApTriggerNpc(p *Player, srv *Server, npc *Npc)` signature consistent across implementation + tests + `tryFireApTrigger` dispatch call. ✅
- `script.TriggerApNpc1` + `script.ServerTriggerType(op-1)` arithmetic matches the S6l/S6m precedent. ✅

**3. Placeholder scan:** No TBD/TODO/"fill in later" patterns. Step 1.6's note about Step 1.7 being the swap that turns the failing integration test green is explicitly written out with the expected observation order — not a placeholder, a deliberate TDD-flow narration.

**4. Scope:** 2 tasks. Task 1 is small (~30 LOC impl + ~80 test LOC). Task 2 is the larger one (~130 LOC impl + ~180 test LOC). Both commit atomically; build green at every commit (Task 1's helper is additive with the AP-branch swap landing in the same commit — no intermediate broken state).
