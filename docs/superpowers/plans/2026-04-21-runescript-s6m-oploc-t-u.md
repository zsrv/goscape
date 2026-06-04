# OpLocT + OpLocU Handler Implementation Plan (S6m)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close S6j-D5 by wiring OpLocT (spell-on-loc, opcode 9) and OpLocU (item-on-loc, opcode 75) end-to-end with their single-trigger APLOCT/OPLOCT and APLOCU/OPLOCU dispatch.

**Architecture:** Three tasks. Task 1 is foundational: `SetInteraction` gains a `com int` parameter (spellCom storage), `targetSubject.com` is resurrected from S6j, two sentinel constants (`targetOpLocT = 6`, `targetOpLocU = 7`) mark the T/U variants in `p.targetOp`, and 23 existing call sites get a mechanical `, -1` argument. Task 2 adds the two handlers (`handleOpLocT`, `handleOpLocU`) with 4 core TS validation gates plus opcode wiring. Task 3 extends `fireApTriggerLoc` and `fireOpTriggerLoc` through a new `apLocTriggerForOp(op) (trigger, ok)` helper that maps sentinels to the right single-trigger.

**Tech Stack:** Go 1.26 (stdlib only). Tests reuse existing fixtures (`makeOpLocFixture`, `newNoopScriptFile`, `makeApTriggerFixture`) and follow S6j/S6l test patterns.

**Spec reference:** `docs/superpowers/specs/2026-04-21-runescript-s6m-oploc-t-u-design.md` (commit `f01bef4`).

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
| `modules/world/interaction.go` | Modify | `SetInteraction` gains `com int` param; add sentinels `targetOpLocT=6`, `targetOpLocU=7` | 1 |
| `modules/world/player.go` | Modify | `targetSubject` struct regains `com int` field | 1 |
| `modules/world/player_script.go` | Modify | Add `*Player.TargetSubjectCom() int` method | 1 |
| `pkg/script/active.go` | Modify | `ActivePlayer` interface gains `TargetSubjectCom() int` | 1 |
| 23 `SetInteraction` call sites | Modify | Mechanical `, -1` arg addition (Task 1 same-commit) | 1 |
| `modules/world/interaction_test.go` | Modify | 2 new tests for `com` field handling | 1 |
| `modules/world/handler_oploc.go` | Modify | Add `handleOpLocT` + `handleOpLocU` | 2 |
| `modules/world/handler_oploc_test.go` | Modify | 12 validation tests (6 per handler) | 2 |
| `modules/world/handlers_game.go` | Modify | Wire `gameHandlers[9]=handleOpLocT`, `gameHandlers[75]=handleOpLocU` | 2 |
| `modules/world/interaction_trigger.go` | Modify | Add `apLocTriggerForOp` helper; swap inline switches in both fire fns | 3 |
| `modules/world/interaction_trigger_test.go` | Modify | 6 fire-dispatch tests for T/U variants | 3 |

**Existing infrastructure already in place (verify, don't modify):**
- Opcodes `OPLOCT=9` (8 bytes), `OPLOCU=75` (12 bytes) — `pkg/io/protocol/game/client/prot.go:73-74`
- Triggers `TriggerApLocT=65`, `TriggerOpLocT=72`, `TriggerApLocU=64`, `TriggerOpLocU=71` — `pkg/script/trigger.go:65-76`
- `Player.lastUseItem int`, `Player.lastUseSlot int` — `modules/world/player.go:173`
- `Player.LastUseItem()` / `Player.LastUseSlot()` accessors — `modules/world/player_script.go:92-93`
- `Server.GetLoc`, `Server.locTypes.Configs[]` — established S6j
- `locStillValid` — S6j lifecycle helper
- `inApproachDistance` / `inOperableDistance` — S6l / S6j
- `fireApTriggerLoc` + `apRangeCalled` persistence — S6l
- Handler map literal — `modules/world/handlers_game.go`

---

## Task 1: SetInteraction Signature + targetSubject.com + Sentinels + Call-Site Migration

**Goal:** Foundational signature change. After this task, `SetInteraction(kind, target, op, com int)` carries the spellCom slot and the 23 existing callers pass `-1`. `TargetSubjectCom()` is wired through the `ActivePlayer` interface for future script reads.

**Files:**
- Modify: `modules/world/interaction.go`
- Modify: `modules/world/player.go`
- Modify: `modules/world/player_script.go`
- Modify: `pkg/script/active.go`
- Modify: `modules/world/handler_opnpc.go`
- Modify: `modules/world/handler_oploc.go`
- Modify: `modules/world/handler_oploc_test.go`
- Modify: `modules/world/tick_interactions_test.go`
- Modify: `modules/world/interaction_test.go`
- Modify: `modules/world/interaction_trigger_test.go`

### Step-by-step

- [ ] **Step 1.1: Re-expand `targetSubject` struct with `com`**

In `modules/world/player.go`, find the current struct (set in S6l):

```go
targetSubject struct{ typ, x, z, level int }
```

Replace with:

```go
// targetSubject snapshots the identity of the interaction target at
// click time. Components:
//   typ, x, z, level — loc identity for tryFireXxxTriggerLoc's
//     lifecycle gate (set by OpLoc handlers after SetInteraction).
//   com — spell-component ID for OpLocT; -1 for OpLoc1..5 and OpLocU.
//     Scripts read via ActivePlayer.TargetSubjectCom() (S6m).
// S6m: com field resurrected from S6j shrink to carry spellCom.
targetSubject struct{ typ, x, z, level, com int }
```

- [ ] **Step 1.2: Run build — expect pass (field is additive, no reader yet)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: PASS.

- [ ] **Step 1.3: Write failing test for `SetInteraction.com` field storage**

In `modules/world/interaction_test.go`, append:

```go
// TestSetInteractionStoresComField verifies that SetInteraction's
// new com parameter writes through to p.targetSubject.com.
// S6m: proves the spellCom slot is carried end-to-end.
func TestSetInteractionStoresComField(t *testing.T) {
	p, _ := newTestPlayer(t)

	// Use a fake target that satisfies the entity interface; we only
	// care about the com write, not the target.
	fake := fakeEntity{x: 100, z: 100, level: 0}
	p.SetInteraction(InteractionEngine, fake, 6, 12345)

	if p.targetSubject.com != 12345 {
		t.Errorf("targetSubject.com: got %d, want 12345", p.targetSubject.com)
	}
	if p.targetOp != 6 {
		t.Errorf("targetOp: got %d, want 6", p.targetOp)
	}
}

// TestSetInteractionPassesMinusOneForNonComOps verifies backwards-compat
// behavior: the S6j/S6k/S6l call sites that pass -1 correctly clear any
// prior com state.
func TestSetInteractionPassesMinusOneForNonComOps(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.targetSubject.com = 999 // simulate stale prior value

	fake := fakeEntity{x: 100, z: 100, level: 0}
	p.SetInteraction(InteractionEngine, fake, 1, -1)

	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (S6j-era callers pass -1)", p.targetSubject.com)
	}
}

// fakeEntity is a minimal entity implementation for tests that need a
// non-nil, non-specific target.
type fakeEntity struct{ x, z, level int }

func (f fakeEntity) Slot() int                      { return -1 }
func (f fakeEntity) Coords() (x, z, level int)      { return f.x, f.z, f.level }
```

Note: if `fakeEntity` or an equivalent already exists in the file, skip redefining it. Adjust the test to use whatever fake is canonical.

- [ ] **Step 1.4: Run test — expect COMPILE FAILURE (signature mismatch)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestSetInteractionStoresComField -v`

Expected: FAIL — `too many arguments in call to p.SetInteraction` because the current signature is `(kind, target, op)` not `(kind, target, op, com)`.

- [ ] **Step 1.5: Change `SetInteraction` signature**

In `modules/world/interaction.go`, replace the existing function:

```go
// SetInteraction anchors the interaction state machine on a target entity.
func (p *Player) SetInteraction(kind InteractionKind, target entity, op int) {
	p.target = target
	p.targetOp = op
	p.interactionKind = kind
	p.apRange = 10
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
	p.interactionFired = false
}
```

With:

```go
// SetInteraction anchors the interaction state machine on a target entity.
// For OpLocT the com parameter carries the spell-component ID; for OpLocU
// pass -1 (item tracking uses lastUseItem/lastUseSlot instead). For
// OpLoc1..5 and OpNpc1..5, callers pass -1.
func (p *Player) SetInteraction(kind InteractionKind, target entity, op, com int) {
	p.target = target
	p.targetOp = op
	p.targetSubject.com = com
	p.interactionKind = kind
	p.apRange = 10
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
	p.interactionFired = false
}
```

- [ ] **Step 1.6: Add sentinel constants**

In `modules/world/interaction.go`, append near the existing `InteractionEngine` / `InteractionScript` constants:

```go
// Sentinel targetOp values for non-op-numbered Loc interaction types.
// OpLoc1..5 use op = 1..5 (the op slot clicked); T and U variants use
// these sentinels so fireXxxTriggerLoc can dispatch to the correct
// single-trigger (APLOCT/OPLOCT or APLOCU/OPLOCU). Matches TS's model
// where setInteraction stores the APLOC trigger value directly;
// goscape uses sentinel integers instead (see S6j-D3 convention).
const (
	targetOpLocT = 6 // APLOCT / OPLOCT dispatch marker
	targetOpLocU = 7 // APLOCU / OPLOCU dispatch marker
)
```

- [ ] **Step 1.7: Run build — expect FAIL at all 23 existing call sites**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: FAIL with many `not enough arguments in call to p.SetInteraction` errors across test files and handlers.

- [ ] **Step 1.8: Mechanically update all 23 call sites with `, -1`**

Enumerated sites (add `, -1` as the 4th argument):

- `modules/world/handler_opnpc.go:47` — `p.SetInteraction(InteractionEngine, npc, op, -1)`
- `modules/world/handler_oploc.go:88` — `p.SetInteraction(InteractionEngine, loc, op, -1)`
- `modules/world/tick_interactions_test.go:34, 49` — 2 sites
- `modules/world/handler_oploc_test.go:289` — 1 site
- `modules/world/interaction_test.go:50, 83, 135, 167, 198, 229, 251, 394` — 8 sites
- `modules/world/interaction_trigger_test.go:42, 68, 102, 139, 174, 198, 227, 243, 277, 411` — 10 sites

Also update the docstring in `modules/world/handler_oploc.go:21` that says:
```go
// On success: ClearPendingAction → SetInteraction(Engine, loc, op) →
```
Change to:
```go
// On success: ClearPendingAction → SetInteraction(Engine, loc, op, -1) →
```

Verify coverage by re-grep after editing:
```
grep -rn "SetInteraction(" modules/world/ pkg/ | grep -v "\.md:" | grep -vE "func \(p \*Player\) SetInteraction|// " | grep -v ", -1)"
```
Expected: empty output (every call site should now end with `, -1)`).

- [ ] **Step 1.9: Run build — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: PASS.

- [ ] **Step 1.10: Run the two TestSetInteraction tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestSetInteraction -v`

Expected: `TestSetInteractionStoresComField` PASS, `TestSetInteractionPassesMinusOneForNonComOps` PASS.

- [ ] **Step 1.11: Run full test suite — expect PASS with no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass. If any existing test fails, inspect the new `com` write: S6l's `TestClearInteractionResetsApRange` and similar should continue to pass because `ClearInteraction` does NOT touch `com` (correct — the next `SetInteraction` overwrites it).

- [ ] **Step 1.12: Add `ActivePlayer.TargetSubjectCom` interface method**

In `pkg/script/active.go`, append to the `ActivePlayer` interface (near the existing methods):

```go
	// TargetSubjectCom returns the com-component value stored at click
	// time by OpLocT-style handlers. For OpLocT it's spellCom; for
	// OpLoc1..5 and OpLocU it's -1. Allows APLOCT scripts to read
	// which spell the player cast via future @spellcom-style script
	// variables. S6m: interface method added ahead of the script-opcode
	// consumer that reads it.
	TargetSubjectCom() int
```

- [ ] **Step 1.13: Run build — expect FAIL (`*Player` no longer satisfies interface)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: FAIL with `*Player does not implement ActivePlayer (missing method TargetSubjectCom)`.

- [ ] **Step 1.14: Add `*Player.TargetSubjectCom()` method**

In `modules/world/player_script.go`, append near other short accessor methods:

```go
// TargetSubjectCom implements script.ActivePlayer.TargetSubjectCom.
// Returns p.targetSubject.com which was set by OpLocT's SetInteraction
// call (spellCom) or -1 for non-com callers.
func (p *Player) TargetSubjectCom() int { return p.targetSubject.com }
```

- [ ] **Step 1.15: Run build to confirm interface now satisfied**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: PASS.

- [ ] **Step 1.16: Extend any `fakeActivePlayer`/`mockPlayer` test fake with `TargetSubjectCom`**

The `ActivePlayer` interface has a test-fake in `pkg/script/runner_test.go` (`mockPlayer`). Search for it:

```
grep -n "mockPlayer\|fakeActivePlayer" pkg/script/*.go
```

Find the struct definition and method list. Add a `TargetSubjectCom` method (returning a capture field or a static value for tests that don't care):

```go
// Add field to the existing struct (if tests want to assert captures):
type mockPlayer struct {
	// ... existing fields ...
	lastTargetSubjectCom int
}

// Method:
func (m *mockPlayer) TargetSubjectCom() int { return m.lastTargetSubjectCom }
```

If the fake's convention is to return zero-value from unimplemented methods (just satisfy the interface), keep it minimal:

```go
func (m *mockPlayer) TargetSubjectCom() int { return -1 }
```

Either works; choose based on whether any Task 1 test needs to observe `TargetSubjectCom` (none required at this task — the script-opcode consumer comes later).

- [ ] **Step 1.17: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass.

- [ ] **Step 1.18: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 1.19: Commit Task 1**

```bash
git add modules/world/interaction.go modules/world/player.go modules/world/player_script.go modules/world/handler_oploc.go modules/world/handler_opnpc.go modules/world/handler_oploc_test.go modules/world/interaction_test.go modules/world/interaction_trigger_test.go modules/world/tick_interactions_test.go pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): SetInteraction com param + targetSubject.com + sentinels (S6m-1)

Foundational signature change for S6m OpLocT/OpLocU handlers:

- SetInteraction(kind, target, op) becomes
  SetInteraction(kind, target, op, com int). For OpLocT com is
  spellCom; for all other callers it's -1.
- targetSubject struct re-expands from {typ, x, z, level int} to
  {typ, x, z, level, com int}. Reverses the S6j shrink that was
  safe then ("no code reads .com") and is newly necessary now.
- Sentinel constants targetOpLocT=6, targetOpLocU=7 added to
  distinguish T/U variants in p.targetOp (OpLoc1..5 keep 1..5).
- ActivePlayer interface gains TargetSubjectCom() int; *Player
  impl returns p.targetSubject.com.
- 23 existing SetInteraction call sites across production code
  and tests mechanically updated with `, -1` argument.

No behavior change yet — Tasks 2 and 3 wire handlers and fire
dispatch that USE the com field. Build green at every commit.

2 new tests for SetInteraction.com storage round-trip.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6m-oploc-t-u-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6m-oploc-t-u.md (Task 1)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: handleOpLocT + handleOpLocU Handlers + 12 Validation Tests + Opcode Wiring

**Goal:** Add both click handlers. After this task, OpLocT / OpLocU packets decode, validate (4 gates), and mutate player state (`targetSubject`, `lastUseItem` for U). No trigger fires yet — that's Task 3.

**Files:**
- Modify: `modules/world/handler_oploc.go`
- Modify: `modules/world/handler_oploc_test.go`
- Modify: `modules/world/handlers_game.go`

### Step-by-step

- [ ] **Step 2.1: Write failing test for `handleOpLocT` happy path**

In `modules/world/handler_oploc_test.go`, append:

```go
// p2x4Payload encodes (x: u16, z: u16, locId: u16, com: u16) into 8 bytes big-endian.
// Used by OpLocT payload construction.
func p2x4Payload(x, z, locId, com int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(locId >> 8), byte(locId),
		byte(com >> 8), byte(com),
	}
}

// TestHandleOpLocTSetsInteraction verifies OpLocT decodes a valid payload
// and routes through SetInteraction with targetOp=targetOpLocT and
// targetSubject.com=spellCom.
func TestHandleOpLocTSetsInteraction(t *testing.T) {
	_, p, loc, _ := makeOpLocFixture(t)

	if err := handleOpLocT(p, p2x4Payload(100, 100, 42, 7777)); err != nil {
		t.Fatalf("handleOpLocT: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
	if p.targetOp != targetOpLocT {
		t.Errorf("targetOp: got %d, want targetOpLocT (%d)", p.targetOp, targetOpLocT)
	}
	if p.targetSubject.com != 7777 {
		t.Errorf("targetSubject.com: got %d, want 7777 (spellCom)", p.targetSubject.com)
	}
	if p.targetSubject.typ != 42 || p.targetSubject.x != 100 || p.targetSubject.z != 100 || p.targetSubject.level != 0 {
		t.Errorf("targetSubject snapshot: got (typ=%d,x=%d,z=%d,level=%d), want (42,100,100,0)",
			p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
}
```

- [ ] **Step 2.2: Run test — expect compile failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLocTSetsInteraction -v`

Expected: compile failure — `handleOpLocT undefined`.

- [ ] **Step 2.3: Implement `handleOpLocT`**

Append to `modules/world/handler_oploc.go`:

```go
// handleOpLocT is the handler for OPLOCT (opcode 9, 8-byte payload).
// Spell-on-loc: player drags a spell icon from the magic-book interface
// onto a loc. Payload = (x:G2, z:G2, locId:G2, spellCom:G2).
//
// Validation gates (mirrors TS OpLocTHandler.ts:~49):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside viewport (52-tile half-extent) → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//
// DEVIATION (S6m-D1): TS also validates spellCom references a component
// with ComActionTarget.LOC flag AND that the component is visible in the
// player's interface stack (OpLocTHandler.ts:~25-35). Skipped here
// because goscape has no component registry yet. Effective risk: client
// can forge spellCom values; scripts reading p.TargetSubjectCom() get
// raw wire values. Follow-up: "component registry + ComActionTarget
// validation" sub-spec.
//
// On success: ClearPendingAction → SetInteraction(Engine, loc,
// targetOpLocT, spellCom) → targetSubject snapshot.
func handleOpLocT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 8 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	locId := int(r.G2())
	spellCom := int(r.G2())

	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		sendUnsetMapFlag(p)
		return nil
	}

	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.locTypes.Configs[locId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, loc, targetOpLocT, spellCom)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return nil
}
```

- [ ] **Step 2.4: Run test — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLocTSetsInteraction -v`

Expected: PASS.

- [ ] **Step 2.5: Add the 5 remaining OpLocT validation tests**

Append to `modules/world/handler_oploc_test.go`:

```go
// TestHandleOpLocTDelayedPlayerRejected verifies delayed → UnsetMapFlag.
func TestHandleOpLocTDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	_ = handleOpLocT(p, p2x4Payload(100, 100, 42, 7777))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
}

// TestHandleOpLocTShortPayloadRejected verifies <8 bytes → UnsetMapFlag.
func TestHandleOpLocTShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)

	_ = handleOpLocT(p, []byte{0x00, 0x64, 0x00, 0x64}) // only 4 bytes
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpLocTOutOfViewportRejected verifies dx > 52 → UnsetMapFlag.
func TestHandleOpLocTOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)
	// origin is (100, 100); dx = 250-100 = 150 > 52.
	_ = handleOpLocT(p, p2x4Payload(250, 100, 42, 7777))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport")
	}
}

// TestHandleOpLocTMissingLocRejected verifies Server.GetLoc nil → UnsetMapFlag.
func TestHandleOpLocTMissingLocRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)
	// locId 999 is not registered in the fixture zone.
	_ = handleOpLocT(p, p2x4Payload(100, 100, 999, 7777))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing loc, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing loc")
	}
}

// TestHandleOpLocTMissingLocTypeRejected verifies missing LocType → UnsetMapFlag.
// Mirrors S6j's approach: register a loc whose typeID has no LocType entry.
func TestHandleOpLocTMissingLocTypeRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)

	// Place a second loc at (100, 100) with typeID 77, but don't register
	// LocType 77 — Configs[77] stays nil.
	extraLoc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 77, 10, 0)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Locs = append(zn.Locs, extraLoc)

	_ = handleOpLocT(p, p2x4Payload(100, 100, 77, 7777))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing locType, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing locType")
	}
}
```

- [ ] **Step 2.6: Run all OpLocT tests — expect all 6 PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLocT -v`

Expected: 6 tests PASS.

- [ ] **Step 2.7: Write failing test for `handleOpLocU` happy path**

Append to `modules/world/handler_oploc_test.go`:

```go
// p2x6Payload encodes (x, z, locId, useObj, useSlot, useCom) into 12 bytes big-endian.
// Used by OpLocU payload construction.
func p2x6Payload(x, z, locId, useObj, useSlot, useCom int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(locId >> 8), byte(locId),
		byte(useObj >> 8), byte(useObj),
		byte(useSlot >> 8), byte(useSlot),
		byte(useCom >> 8), byte(useCom),
	}
}

// TestHandleOpLocUSetsInteraction verifies OpLocU decodes a valid payload
// and routes through SetInteraction with targetOp=targetOpLocU. useObj
// and useSlot land on p.lastUseItem/lastUseSlot; useCom is discarded
// (S6m-D2/D3).
func TestHandleOpLocUSetsInteraction(t *testing.T) {
	_, p, loc, _ := makeOpLocFixture(t)

	if err := handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpLocU: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
	if p.targetOp != targetOpLocU {
		t.Errorf("targetOp: got %d, want targetOpLocU (%d)", p.targetOp, targetOpLocU)
	}
	if p.lastUseItem != 1511 {
		t.Errorf("lastUseItem: got %d, want 1511 (useObj)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot: got %d, want 3", p.lastUseSlot)
	}
	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (OpLocU passes -1)", p.targetSubject.com)
	}
	if p.targetSubject.typ != 42 || p.targetSubject.x != 100 || p.targetSubject.z != 100 || p.targetSubject.level != 0 {
		t.Errorf("targetSubject snapshot: got (typ=%d,x=%d,z=%d,level=%d), want (42,100,100,0)",
			p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)
	}
}
```

- [ ] **Step 2.8: Run test — expect compile failure `handleOpLocU undefined`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLocUSetsInteraction -v`

Expected: FAIL.

- [ ] **Step 2.9: Implement `handleOpLocU`**

Append to `modules/world/handler_oploc.go`:

```go
// handleOpLocU is the handler for OPLOCU (opcode 75, 12-byte payload).
// Item-on-loc: player drags an inventory item onto a loc (e.g., axe on
// tree, tinderbox on logs, seed on patch).
// Payload = (x:G2, z:G2, locId:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// Validation gates (subset of TS OpLocUHandler.ts:~79):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside viewport → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//
// DEVIATION (S6m-D2): TS validates useCom references a usable, visible
// interface component (OpLocUHandler.ts:~25-35). Skipped — no component
// registry yet.
//
// DEVIATION (S6m-D3): TS does an inventory-listener lookup by useCom to
// verify the player has an inv listening at that interface, plus
// slot-bounds + item-at-slot-matches-useObj validation
// (OpLocUHandler.ts:~45-70). Goscape's invListeners is a slice, not a
// keyed map, so this lookup shape doesn't translate directly. Skip;
// scripts reading p.LastUseItem()/p.LastUseSlot() get raw wire values.
// Security risk: client can claim any item/slot. Real scripts
// defensively re-check via inv_getobj-style opcodes. Follow-up:
// "InvListener keyed-map refactor + OpLocU item validation" sub-spec.
//
// DEVIATION (S6m-D4): TS checks members-only items against NODE_MEMBERS
// server config (OpLocUHandler.ts:~71-77). Skipped because goscape has
// no members-config surface yet. Follow-up: "members-config + item-
// gating" sub-spec.
//
// On success: set p.lastUseItem = useObj, p.lastUseSlot = useSlot →
// ClearPendingAction → SetInteraction(Engine, loc, targetOpLocU, -1) →
// targetSubject snapshot.
func handleOpLocU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 12 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	locId := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	_ = int(r.G2()) // useCom — deliberately discarded (S6m-D2/D3)

	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		sendUnsetMapFlag(p)
		return nil
	}

	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.locTypes.Configs[locId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, loc, targetOpLocU, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return nil
}
```

- [ ] **Step 2.10: Run test — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestHandleOpLocUSetsInteraction -v`

Expected: PASS.

- [ ] **Step 2.11: Add the 5 remaining OpLocU validation tests**

Append to `modules/world/handler_oploc_test.go`:

```go
// TestHandleOpLocUDelayedPlayerRejected verifies delayed → UnsetMapFlag,
// and that lastUseItem is NOT clobbered (defensive: delayed rejection
// happens before any player-state mutation).
func TestHandleOpLocUDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0
	p.lastUseItem = 42 // sentinel: must stay unchanged

	_ = handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
	if p.lastUseItem != 42 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 42", p.lastUseItem)
	}
}

// TestHandleOpLocUShortPayloadRejected verifies <12 bytes → UnsetMapFlag.
func TestHandleOpLocUShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)

	_ = handleOpLocU(p, []byte{0x00, 0x64, 0x00, 0x64, 0x00, 0x2a, 0x05, 0xe7}) // 8 bytes
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpLocUOutOfViewportRejected verifies dx > 52 → UnsetMapFlag.
func TestHandleOpLocUOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)
	// origin (100,100); dx = 250-100 = 150 > 52.
	_ = handleOpLocU(p, p2x6Payload(250, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport")
	}
}

// TestHandleOpLocUMissingLocRejected verifies Server.GetLoc nil → UnsetMapFlag.
func TestHandleOpLocUMissingLocRejected(t *testing.T) {
	_, p, _, cc := makeOpLocFixture(t)
	// locId 999 is not registered in fixture zone.
	_ = handleOpLocU(p, p2x6Payload(100, 100, 999, 1511, 3, 149))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing loc, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing loc")
	}
}

// TestHandleOpLocUMissingLocTypeRejected verifies missing LocType → UnsetMapFlag.
func TestHandleOpLocUMissingLocTypeRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)

	// Place a loc with typeID 77 but no registered LocType.
	extraLoc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 77, 10, 0)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Locs = append(zn.Locs, extraLoc)

	_ = handleOpLocU(p, p2x6Payload(100, 100, 77, 1511, 3, 149))
	p.client.flushWrite()
	got := <-cc

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing locType, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing locType")
	}
}
```

- [ ] **Step 2.12: Run all 12 handler tests — expect all PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleOpLocT|TestHandleOpLocU" -v`

Expected: 12 tests PASS.

- [ ] **Step 2.13: Wire the two new opcodes in handlers_game.go**

In `modules/world/handlers_game.go`, find the existing block with OpLoc1..5 handlers (`gameHandlers[245] = handleOpLoc1` etc.). Append near it:

```go
gameHandlers[9] = handleOpLocT  // OPLOCT
gameHandlers[75] = handleOpLocU // OPLOCU
```

- [ ] **Step 2.14: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass.

- [ ] **Step 2.15: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 2.16: Commit Task 2**

```bash
git add modules/world/handler_oploc.go modules/world/handler_oploc_test.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): handleOpLocT + handleOpLocU + 12 validation tests (S6m-2)

Adds the two sibling OPLOC click handlers:

- handleOpLocT (opcode 9, 8-byte payload: x, z, locId, spellCom).
  Spell-on-loc. Stores spellCom via SetInteraction(..., spellCom)
  into p.targetSubject.com. Fires APLOCT (single trigger, not
  per-op) via Task 3's dispatch.
- handleOpLocU (opcode 75, 12-byte payload: +useObj, useSlot,
  useCom). Item-on-loc. Stores useObj/useSlot in p.lastUseItem/
  p.lastUseSlot (existing fields); useCom is deliberately
  discarded per DEVIATION S6m-D2/D3.

Both handlers run the 4 core TS validation gates (delayed,
payload-length, viewport 52-tile, loc-exists, locType-exists).
Complex validation (component-visibility check, item-in-slot
match, members-only gate) is explicitly deferred via S6m-D1..D4
with follow-up sub-spec pointers.

12 tests total (6 per handler): happy path, delayed, short-
payload, out-of-viewport, missing-loc, missing-locType. OpLocU
happy path verifies lastUseItem/lastUseSlot capture; OpLocU
delayed-rejection test verifies lastUseItem is NOT clobbered
before validation passes.

Opcodes wired in handlers_game.go: OPLOCT=9, OPLOCU=75.

Triggers fire in Task 3 (S6m-3) — this task only routes state.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6m-oploc-t-u-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6m-oploc-t-u.md (Task 2)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: apLocTriggerForOp Helper + Fire Dispatch Extension

**Goal:** Extend `fireApTriggerLoc` and `fireOpTriggerLoc` to fire single triggers (APLOCT/OPLOCT and APLOCU/OPLOCU) when `p.targetOp` holds the T/U sentinel values. After this task, APLOCT/OPLOCT/APLOCU/OPLOCU scripts fire end-to-end.

**Files:**
- Modify: `modules/world/interaction_trigger.go`
- Modify: `modules/world/interaction_trigger_test.go`

### Step-by-step

- [ ] **Step 3.1: Write failing unit tests for `apLocTriggerForOp` helper**

In `modules/world/interaction_trigger_test.go`, append:

```go
// TestApLocTriggerForOpValidValues table-tests all valid targetOp
// mappings:
//   1..5 → TriggerApLoc1..5 (existing OpLoc1..5 behavior)
//   6 (targetOpLocT) → TriggerApLocT (single)
//   7 (targetOpLocU) → TriggerApLocU (single)
func TestApLocTriggerForOpValidValues(t *testing.T) {
	cases := []struct {
		op   int
		want script.ServerTriggerType
		name string
	}{
		{1, script.TriggerApLoc1, "OpLoc1"},
		{2, script.TriggerApLoc2, "OpLoc2"},
		{3, script.TriggerApLoc3, "OpLoc3"},
		{4, script.TriggerApLoc4, "OpLoc4"},
		{5, script.TriggerApLoc5, "OpLoc5"},
		{targetOpLocT, script.TriggerApLocT, "OpLocT"},
		{targetOpLocU, script.TriggerApLocU, "OpLocU"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := apLocTriggerForOp(c.op)
			if !ok {
				t.Fatalf("op=%d: ok=false, want true", c.op)
			}
			if got != c.want {
				t.Errorf("op=%d: got %d, want %d", c.op, got, c.want)
			}
		})
	}
}

// TestApLocTriggerForOpInvalidValues verifies out-of-range op values
// return ok=false (caller silent-clears). Covers the gap between 5 and
// targetOpLocT (none currently) and below 1 / above 7.
func TestApLocTriggerForOpInvalidValues(t *testing.T) {
	invalid := []int{0, -1, 8, 100, -100}
	for _, op := range invalid {
		t.Run(fmt.Sprintf("op_%d", op), func(t *testing.T) {
			_, ok := apLocTriggerForOp(op)
			if ok {
				t.Errorf("op=%d: ok=true, want false", op)
			}
		})
	}
}
```

Note: `fmt` import may need adding to the test file. If it's already there, no action needed.

- [ ] **Step 3.2: Run tests — expect compile failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestApLocTriggerForOp -v`

Expected: compile failure — `apLocTriggerForOp undefined`.

- [ ] **Step 3.3: Implement `apLocTriggerForOp` helper**

In `modules/world/interaction_trigger.go`, append near the end (after `locStillValid`):

```go
// apLocTriggerForOp returns the APLOC trigger for the player's
// targetOp sentinel. Returns ok=false if op is neither 1..5 nor a T/U
// sentinel. fireOpTriggerLoc derives the OPLOC trigger by adding 7 to
// the returned APLOC (TS Player.ts:~997 offset convention):
//   APLOC1..5 (59..63) + 7 → OPLOC1..5 (66..70)
//   APLOCT    (65)     + 7 → OPLOCT    (72)
//   APLOCU    (64)     + 7 → OPLOCU    (71)
func apLocTriggerForOp(op int) (script.ServerTriggerType, bool) {
	switch {
	case op >= 1 && op <= 5:
		return script.TriggerApLoc1 + script.ServerTriggerType(op-1), true
	case op == targetOpLocT:
		return script.TriggerApLocT, true
	case op == targetOpLocU:
		return script.TriggerApLocU, true
	default:
		return 0, false
	}
}
```

- [ ] **Step 3.4: Run unit tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestApLocTriggerForOp -v`

Expected: all 7 subtests of `TestApLocTriggerForOpValidValues` + 5 subtests of `TestApLocTriggerForOpInvalidValues` PASS.

- [ ] **Step 3.5: Swap `fireApTriggerLoc`'s inline switch to use the helper**

In `modules/world/interaction_trigger.go`, find `fireApTriggerLoc`. Locate this block (approximately in the middle of the function):

```go
	op := p.targetOp
	if op < 1 || op > 5 {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger := script.TriggerApLoc1 + script.ServerTriggerType(op-1)
```

Replace with:

```go
	trigger, ok := apLocTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
```

Everything downstream (lifecycle gate, category lookup, script init, apRangeCalled persistence contract) is unchanged.

- [ ] **Step 3.6: Swap `fireOpTriggerLoc`'s inline switch to use the helper**

In `modules/world/interaction_trigger.go`, find `fireOpTriggerLoc`. Locate this block:

```go
	op := p.targetOp
	if op < 1 || op > 5 {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}

	trigger := script.TriggerOpLoc1 + script.ServerTriggerType(op-1)
```

Replace with:

```go
	apTrigger, ok := apLocTriggerForOp(p.targetOp)
	if !ok {
		p.ClearInteraction()
		p.interactionFired = true
		return
	}
	trigger := apTrigger + 7 // APLOC→OPLOC offset per TS Player.ts:~997
```

- [ ] **Step 3.7: Run existing fire tests to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireOpTrigger|TestTryFireOpTrigger|TestTryFireApTrigger" -v`

Expected: all existing S6j/S6l fire tests still pass. If any fail, the signature change in Task 1 likely left a stale `, -1` argument pattern — re-check.

- [ ] **Step 3.8: Write failing test for OpLocT-trigger fire (OPLOCT at contact)**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireOpTriggerLocFiresOpLocTTrigger verifies that when p.targetOp
// is targetOpLocT (6) and an OPLOCT script is registered, fireOpTriggerLoc
// dispatches to it. Player positioned at contact distance.
func TestFireOpTriggerLocFiresOpLocTTrigger(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	p.SetInteraction(InteractionEngine, loc, targetOpLocT, 7777)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level

	sf := newNoopScriptFile(t, script.TriggerOpLocT, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPLOCT fire")
	}
}
```

- [ ] **Step 3.9: Run test — expect PASS (the helper routes correctly now)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestFireOpTriggerLocFiresOpLocTTrigger -v`

Expected: PASS.

- [ ] **Step 3.10: Add the remaining 3 fire tests**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireOpTriggerLocFiresOpLocUTrigger verifies targetOpLocU (7) →
// OPLOCU dispatch at contact.
func TestFireOpTriggerLocFiresOpLocUTrigger(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	p.SetInteraction(InteractionEngine, loc, targetOpLocU, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level

	sf := newNoopScriptFile(t, script.TriggerOpLocU, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPLOCU fire")
	}
}

// TestFireApTriggerLocFiresApLocTTrigger verifies targetOpLocT (6) →
// APLOCT dispatch at approach distance.
func TestFireApTriggerLocFiresApLocTTrigger(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)
	// Override targetOp from fixture's default 1 → targetOpLocT.
	p.targetOp = targetOpLocT

	sf := newNoopScriptFile(t, script.TriggerApLocT, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after no-p_aprange clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after APLOCT fire")
	}
}

// TestFireApTriggerLocFiresApLocUTrigger verifies targetOpLocU (7) →
// APLOCU dispatch at approach distance.
func TestFireApTriggerLocFiresApLocUTrigger(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)
	p.targetOp = targetOpLocU

	sf := newNoopScriptFile(t, script.TriggerApLocU, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after no-p_aprange clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after APLOCU fire")
	}
}
```

- [ ] **Step 3.11: Run all 4 fire tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestFireOpTriggerLocFires|TestFireApTriggerLocFires" -v`

Expected: 4 tests PASS.

- [ ] **Step 3.12: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests pass. Particularly, S6j's `TestTryFireOpTriggerLocScriptFires` and S6l's `TestTryFireApTriggerLocNoScript` etc. should all still pass — the helper refactor preserved their code paths exactly (1..5 → TriggerApLoc1..5 arithmetic is byte-equivalent to the old inline form).

- [ ] **Step 3.13: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no warnings.

- [ ] **Step 3.14: Run race detector on modules/world**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

Expected: no races.

- [ ] **Step 3.15: Commit Task 3**

```bash
git add modules/world/interaction_trigger.go modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): fireXxxTriggerLoc dispatch for T/U variants (S6m-3)

Completes S6m end-to-end wiring. APLOCT/OPLOCT/APLOCU/OPLOCU scripts
now fire when the player clicks a loc with a spell (OpLocT) or an
item (OpLocU).

Mechanism: apLocTriggerForOp(op) helper maps p.targetOp to an APLOC
trigger value. 1..5 → TriggerApLoc1..5 (existing arithmetic);
targetOpLocT (6) → TriggerApLocT (single); targetOpLocU (7) →
TriggerApLocU (single). fireOpTriggerLoc derives OPLOC triggers by
adding 7 per TS Player.ts:~997 convention — verified numerically:
APLOCT(65)+7=OPLOCT(72); APLOCU(64)+7=OPLOCU(71).

fireApTriggerLoc and fireOpTriggerLoc both refactored to use the
helper. Previous inline `op < 1 || op > 5` gates replaced with
`!ok` returns. All pre-existing S6j/S6l fire behavior preserved
byte-equivalent for the 1..5 path.

6 new tests:
- TestApLocTriggerForOpValidValues: 7 subtests covering 1..5 + T/U
- TestApLocTriggerForOpInvalidValues: 5 subtests for out-of-range
- TestFireOpTriggerLocFiresOpLocT/UTrigger: end-to-end OP dispatch
- TestFireApTriggerLocFiresApLocT/UTrigger: end-to-end AP dispatch

Milestone: S6j-D5 CLOSED. After S6m, all 7 S6j deviations are either
closed, storage-convention defensives, or documented-with-infra-
dependency follow-ups. OPLOC click surface is TS-faithful end-to-end.

Spec: docs/superpowers/specs/2026-04-21-runescript-s6m-oploc-t-u-design.md
Plan: docs/superpowers/plans/2026-04-21-runescript-s6m-oploc-t-u.md (Task 3)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes (for plan-author use)

**1. Spec coverage:**
- §1 Goal — Tasks 1+2+3 collectively wire OpLocT + OpLocU end-to-end. ✅
- §2 Architecture — Task 1 (foundation), Task 2 (handlers), Task 3 (fire dispatch). ✅
- §3 File map — every modified file appears in task headers. ✅
- §5.1 SetInteraction signature — Task 1 Step 1.5. ✅
- §5.2 targetSubject.com field — Task 1 Step 1.1. ✅
- §5.3 handleOpLocT — Task 2 Step 2.3. ✅
- §5.4 handleOpLocU — Task 2 Step 2.9. ✅
- §5.5 handlers_game.go wiring — Task 2 Step 2.13. ✅
- §5.6 ActivePlayer.TargetSubjectCom — Task 1 Steps 1.12, 1.14. ✅
- §5.7 apLocTriggerForOp — Task 3 Step 3.3. ✅
- §5.8 fire dispatch swap — Task 3 Steps 3.5, 3.6. ✅
- §6 Test plan — 2 (Task 1) + 12 (Task 2) + 6 (Task 3) = 20 tests (matches spec). ✅

**2. Type consistency:**
- `SetInteraction(kind, target, op, com int)` signature consistent across Task 1 change + all Task 2/3 call sites. ✅
- `targetOpLocT = 6`, `targetOpLocU = 7` consistent. ✅
- `apLocTriggerForOp(op) (script.ServerTriggerType, bool)` signature consistent. ✅
- Handler names `handleOpLocT` / `handleOpLocU` consistent across all references. ✅
- Payload helpers `p2x4Payload` (8-byte for T) / `p2x6Payload` (12-byte for U) named to match existing `p2x3Payload` (6-byte for OpLoc1..5) convention.

**3. Placeholder scan:** No TBD/TODO. Step 1.16 on mockPlayer extension has a conditional note ("if tests want to assert captures, use a capture field; otherwise return -1") — this is a legitimate "implementer choice with both paths specified in full" rather than a placeholder. Step 2.13 points the implementer to find the existing OpLoc1..5 block — straightforward.

**4. Scope:** 3 tasks. Task 1 is the largest in file-touch breadth (10 files including 23 call-site migrations) but each migration is a mechanical 4-char addition. Task 2 is the largest in new code (~180 LOC). Task 3 is the smallest (~30 LOC + 6 tests). Build green at every commit (Task 1 batches all call-site updates; Task 3 preserves byte-equivalent behavior for 1..5).
