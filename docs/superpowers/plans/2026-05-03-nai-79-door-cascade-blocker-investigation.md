# NAI-79 — Door cascade-blocker investigation Implementation Plan (Stage 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land permanent `Cfg.NodeDebug`-gated structured logs at four diagnostic choke-points so a smoke at the Tutorial Island RS Guide door produces enough captured-log signal to deterministically route to one of H1/H2/H3/H4 per spec §5.

**Architecture:** Two slog-Debug frames — Frame A at `handleOpLoc` tail (handler-side: click coords + Loc geometry + cache-key string) and Frame B at `processInteraction` tail (interaction-side: pre/post-step branch ids, waypoint count, distance, target-still-set). Branch tracking inside `tryInteract` writes to per-Player fields (`lastInteractBranchPre`/`Post`) keyed by a transient `interactCallSlot` mode flag set by `processInteraction` before each call.

**Tech Stack:** Go 1.26+, `log/slog`, existing `*Server.log *slog.Logger` plumbing (server.go:48).

**Spec:** `docs/superpowers/specs/2026-05-03-nai-79-door-cascade-blocker-investigation-design.md` (HEAD `d41bbad`).

**Scope:** Stage 1 only. Stage 2 (one of bundles H1/H2/H3/H4 per spec §6) materializes as a plan-update after smoke handoff captures the log.

---

## File Structure

**New files:**
- `modules/world/interaction_debug.go` — file-local helpers `chebDist`, `targetKindString`, and the `recordTryInteractBranch` writer.
- `modules/world/interaction_debug_test.go` — capturing-logger test helper (`newCapturingLogger`) + helper tests + branch-tracking + Frame A + Frame B tests.

**Modified files:**
- `modules/world/player.go` — three new int fields (`lastInteractBranchPre`, `lastInteractBranchPost`, `interactCallSlot`) + zero-init in `newPlayer`.
- `modules/world/interaction.go` — `tryInteract` instrumented (5 sites: 4 return statements + fallthrough); `processInteraction` captures pre-step state and emits Frame B at tail.
- `modules/world/handler_oploc.go` — Frame A emit before success `return nil`.

**Justification for `interaction_debug.go`:** Keeps the helpers and the (small) recorder out of `interaction.go`, which is already 494 lines. Tests for the helpers + frames live alongside.

---

## Task 1 — Helpers + capturing-logger test scaffolding

**Files:**
- Create: `modules/world/interaction_debug.go`
- Create: `modules/world/interaction_debug_test.go`

This task lands the two pure helpers (`chebDist`, `targetKindString`) and the test-side capturing-logger scaffolding (`newCapturingLogger`) that subsequent tasks reuse. No production behavior change yet beyond the helpers' addition.

- [ ] **Step 1 — Write failing test for `chebDist` and `targetKindString`.**

Create `modules/world/interaction_debug_test.go`:

```go
package world

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

func TestChebDist(t *testing.T) {
	tests := []struct {
		name                   string
		ax, az, bx, bz, expect int
	}{
		{"same tile", 5, 5, 5, 5, 0},
		{"adjacent N", 5, 5, 5, 4, 1},
		{"diagonal", 5, 5, 6, 6, 1},
		{"two tiles E", 5, 5, 7, 5, 2},
		{"asymmetric 3x1", 5, 5, 8, 6, 3},
		{"negative direction", 10, 10, 7, 7, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chebDist(tc.ax, tc.az, tc.bx, tc.bz)
			if got != tc.expect {
				t.Errorf("chebDist(%d,%d,%d,%d) = %d, want %d",
					tc.ax, tc.az, tc.bx, tc.bz, got, tc.expect)
			}
		})
	}
}

func TestTargetKindString(t *testing.T) {
	loc := entitypkg.NewLoc(0, 1, 1, 1, 1, entitypkg.LifecycleForever, 0, 0, 0)
	npc := &Npc{}
	plr := &Player{}

	tests := []struct {
		name   string
		target entity
		expect string
	}{
		{"loc", loc, "Loc"},
		{"npc", npc, "Npc"},
		{"player", plr, "Player"},
		{"nil", nil, "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := targetKindString(tc.target)
			if got != tc.expect {
				t.Errorf("targetKindString(%T) = %q, want %q",
					tc.target, got, tc.expect)
			}
		})
	}
}

// capturingHandler is a slog.Handler that retains every Record passed to
// Handle so tests can assert on emitted frames.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *capturingHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(h.records))
	copy(out, h.records)
	return out
}

// newCapturingLogger returns a logger and the handler so tests can pull
// records back out. The logger emits at Debug level (matching production
// usage of s.log.Debug for instrumentation).
func newCapturingLogger() (*slog.Logger, *capturingHandler) {
	h := &capturingHandler{}
	return slog.New(h), h
}

// findRecord returns the first record with the given message, or nil.
func findRecord(records []slog.Record, msg string) *slog.Record {
	for i := range records {
		if records[i].Message == msg {
			return &records[i]
		}
	}
	return nil
}

// attrValue extracts the value of attribute `key` from `r`. Returns
// (slog.Value{}, false) if not found.
func attrValue(r slog.Record, key string) (slog.Value, bool) {
	var found slog.Value
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a.Value
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

// requireAttr fails the test if `key` is missing from `r` or its value
// (compared via String()) doesn't match `want`.
func requireAttr(t *testing.T, r slog.Record, key, want string) {
	t.Helper()
	v, ok := attrValue(r, key)
	if !ok {
		t.Fatalf("record %q missing attr %q", r.Message, key)
	}
	if got := v.String(); got != want {
		t.Errorf("record %q attr %q = %q, want %q", r.Message, key, got, want)
	}
}
```

- [ ] **Step 2 — Run test, verify fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestChebDist|TestTargetKindString" -v
```

Expected: FAIL — `undefined: chebDist`, `undefined: targetKindString`.

- [ ] **Step 3 — Implement helpers.**

Create `modules/world/interaction_debug.go`:

```go
package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// chebDist returns the Chebyshev distance between two tile coords.
// Used by NAI-79 Stage 1 instrumentation (interaction tick frame).
func chebDist(ax, az, bx, bz int) int {
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

// targetKindString returns a stable string label for an interaction
// target so the NAI-79 interaction tick frame can name the target type
// without relying on slog's reflection-based formatting. Returns
// "unknown" for nil or unrecognized types.
func targetKindString(t entity) string {
	switch t.(type) {
	case *entitypkg.Loc:
		return "Loc"
	case *entitypkg.Obj:
		return "Obj"
	case *Npc:
		return "Npc"
	case *Player:
		return "Player"
	default:
		return "unknown"
	}
}
```

Note: Verify the actual `entity` interface in this package and the Obj package path. If `entity` is unexported but accessible in the package, the file-local imports above are correct. If `*entitypkg.Obj` doesn't exist (only Locs+ground items live in `pkg/entity`), drop that switch arm and update Step 1's test fixture.

- [ ] **Step 4 — Run test, verify pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestChebDist|TestTargetKindString" -v
```

Expected: PASS for all 10 sub-tests.

- [ ] **Step 5 — Verify capturing-logger scaffolding compiles.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
```

Expected: clean.

- [ ] **Step 6 — Commit.**

```bash
git add modules/world/interaction_debug.go modules/world/interaction_debug_test.go
git commit --no-gpg-sign -m "feat(world): NAI-79 T1 — interaction-debug helpers + capturing-logger test scaffold

Adds chebDist + targetKindString helpers (used by Stage 1 instrumentation
frames in T4/T5). Adds capturingHandler / newCapturingLogger / requireAttr
test scaffolding so subsequent tasks can assert on emitted slog records."
```

---

## Task 2 — Player branch-tracking fields + recordTryInteractBranch helper

**Files:**
- Modify: `modules/world/player.go` (add 3 fields)
- Modify: `modules/world/interaction_debug.go` (add `recordTryInteractBranch`)
- Test: `modules/world/interaction_debug_test.go` (add 1 test)

This task lands the data + helper for branch tracking but doesn't yet wire it into `tryInteract`. Wiring is T3.

- [ ] **Step 1 — Write failing test for `recordTryInteractBranch`.**

Append to `modules/world/interaction_debug_test.go`:

```go
func TestRecordTryInteractBranch(t *testing.T) {
	tests := []struct {
		name              string
		slot, branch      int
		expectPre, expectPost int
	}{
		{"slot 0 writes pre", 0, 1, 1, 0},
		{"slot 1 writes post", 1, 3, 0, 3},
		{"slot 0 branch 4", 0, 4, 4, 0},
		{"slot 1 branch 2", 1, 2, 0, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Player{}
			p.interactCallSlot = tc.slot
			recordTryInteractBranch(p, tc.branch)
			if p.lastInteractBranchPre != tc.expectPre {
				t.Errorf("pre: got %d, want %d", p.lastInteractBranchPre, tc.expectPre)
			}
			if p.lastInteractBranchPost != tc.expectPost {
				t.Errorf("post: got %d, want %d", p.lastInteractBranchPost, tc.expectPost)
			}
		})
	}
}
```

- [ ] **Step 2 — Run test, verify fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestRecordTryInteractBranch -v
```

Expected: FAIL — `p.interactCallSlot undefined`, `recordTryInteractBranch undefined`.

- [ ] **Step 3 — Add Player fields.**

Locate `type Player struct` in `modules/world/player.go`. Add three int fields. The existing struct has many fields — add these in a `// NAI-79 Stage 1 instrumentation` block placed near the existing `target`/`apRange`/`waypointIndex` fields (i.e., grouped with other interaction state). Example placement (Edit at the right block):

```go
	// NAI-79 Stage 1 instrumentation (interaction.go branch tracking).
	// lastInteractBranch{Pre,Post} hold the branch id (0=fallthrough,
	// 1..4) of the most recent tryInteract call from processInteraction's
	// pre-step / post-step arms. interactCallSlot is the transient mode
	// flag (0=pre, 1=post) set by processInteraction before each call.
	lastInteractBranchPre  int
	lastInteractBranchPost int
	interactCallSlot       int
```

Verify `newPlayer` (player.go:~423 per the grep earlier showing `uid: -1`) doesn't need explicit zero-init for these — Go zero-initializes int to 0, which is the desired starting state.

- [ ] **Step 4 — Add `recordTryInteractBranch`.**

Append to `modules/world/interaction_debug.go`:

```go
// recordTryInteractBranch is the side-channel writer used by
// (*Player).tryInteract to surface which of its 4 branches (or
// fallthrough = 0) returned. processInteraction sets p.interactCallSlot
// to 0 before its pre-step call and 1 before its post-step call; this
// helper picks the right Player field based on the slot value.
func recordTryInteractBranch(p *Player, branch int) {
	if p.interactCallSlot == 1 {
		p.lastInteractBranchPost = branch
	} else {
		p.lastInteractBranchPre = branch
	}
}
```

- [ ] **Step 5 — Run test, verify pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestRecordTryInteractBranch -v
```

Expected: PASS for all 4 sub-tests.

- [ ] **Step 6 — Run full package tests, verify no regression from new fields.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. (The new fields are zero-init and untouched by existing code paths; this run just guards against build/import errors.)

- [ ] **Step 7 — Commit.**

```bash
git add modules/world/player.go modules/world/interaction_debug.go modules/world/interaction_debug_test.go
git commit --no-gpg-sign -m "feat(world): NAI-79 T2 — Player branch-tracking fields + recordTryInteractBranch

Adds lastInteractBranchPre/Post + interactCallSlot int fields on Player
plus recordTryInteractBranch helper. No wiring yet — T3 instruments
tryInteract returns to call this helper."
```

---

## Task 3 — Wire branch tracking into `tryInteract`

**Files:**
- Modify: `modules/world/interaction.go:331-400` (5 sites inside tryInteract)
- Test: `modules/world/interaction_debug_test.go` (add 5-case branch-tracking test)

This task instruments each return inside `tryInteract` to call `recordTryInteractBranch` with the branch id, then writes a single test that exercises all 5 outcomes (branches 1, 2, 3, 4, fallthrough) and confirms the right field gets populated for both pre-step (slot=0) and post-step (slot=1).

- [ ] **Step 1 — Write the failing branch-tracking test.**

Append to `modules/world/interaction_debug_test.go`:

```go
func TestTryInteractBranchTrackingPerCallsite(t *testing.T) {
	// Each row drives tryInteract to a specific branch and asserts
	// the recorded branch id for both pre-step (slot=0) and
	// post-step (slot=1) calls. The fixture builds a player with a
	// Loc target and tweaks state per row to force the branch.
	tests := []struct {
		name           string
		setup          func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile)
		allowOpScenery bool
		wantBranch     int
	}{
		{
			name: "branch 1 (op + operable + scenery allowed)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				// player adjacent to loc → operable. opTrigger present
				// via registered script. allowOpScenery=true (so non-
				// PathingEntity Loc target qualifies).
				p.x, p.z = 99, 100
				registerOpLocScript(t, s, loc.Type(), 1, sf)
			},
			allowOpScenery: true,
			wantBranch:     1,
		},
		{
			name: "branch 2 (ap + approach)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				// player 2 tiles away → approach but not operable. ap
				// trigger present, ap_range default 10.
				p.x, p.z = 98, 100
				p.apRange = 10
				registerApLocScript(t, s, loc.Type(), 1, sf)
			},
			wantBranch: 2,
		},
		{
			name: "branch 3 (approach + ap nil)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				p.x, p.z = 98, 100
				p.apRange = 10
				// no scripts registered: ap trigger nil → branch 3.
			},
			wantBranch: 3,
		},
		{
			name: "branch 4 (operable + scenery allowed + op nil)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				p.x, p.z = 99, 100
				// no scripts registered: op trigger nil; allowOpScenery
				// flips on; player is operable → branch 4 (NIH).
			},
			allowOpScenery: true,
			wantBranch:     4,
		},
		{
			name: "fallthrough (operable but allowOpScenery=false, no triggers)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				p.x, p.z = 99, 100
				// allowOpScenery=false; Loc target is non-PathingEntity;
				// branch 1 fails. No ap script. branch 2 fails. Not in
				// approach without ap_range>0 (default in fixture is 0).
				p.apRange = 0
				// branch 3 needs approach=true; with ap_range=0 it's
				// false. branch 4 needs allowOpScenery; false here.
				// Returns false at fallthrough → branch 0.
			},
			allowOpScenery: false,
			wantBranch:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name+" pre-slot", func(t *testing.T) {
			s, p, loc, _ := makeOpLocFixture(t)
			s.scriptProvider = script.NewProvider() // empty; per-row setup re-seeds
			sf := buildPOpLocScript(script.TriggerOpLoc1, loc.Type(), 1)
			tc.setup(s, p, loc, sf)

			// Engage interaction so tryInteract has a target.
			p.SetInteraction(InteractionEngine, loc, 1, -1)

			p.interactCallSlot = 0
			_ = p.tryInteract(tc.allowOpScenery)
			if p.lastInteractBranchPre != tc.wantBranch {
				t.Errorf("pre: got branch %d, want %d", p.lastInteractBranchPre, tc.wantBranch)
			}
			if p.lastInteractBranchPost != 0 {
				t.Errorf("post unexpectedly set: %d", p.lastInteractBranchPost)
			}
		})
		t.Run(tc.name+" post-slot", func(t *testing.T) {
			s, p, loc, _ := makeOpLocFixture(t)
			s.scriptProvider = script.NewProvider()
			sf := buildPOpLocScript(script.TriggerOpLoc1, loc.Type(), 1)
			tc.setup(s, p, loc, sf)
			p.SetInteraction(InteractionEngine, loc, 1, -1)

			p.interactCallSlot = 1
			_ = p.tryInteract(tc.allowOpScenery)
			if p.lastInteractBranchPost != tc.wantBranch {
				t.Errorf("post: got branch %d, want %d", p.lastInteractBranchPost, tc.wantBranch)
			}
			if p.lastInteractBranchPre != 0 {
				t.Errorf("pre unexpectedly set: %d", p.lastInteractBranchPre)
			}
		})
	}
}

// registerOpLocScript registers a [oploc<N>,<typeID>] script via the
// existing TriggerOpLoc<N> + LookupKeyForType convention. The OP-side
// trigger is computed inline as `script.TriggerOpLoc1 + (op-1)` since
// goscape only exposes apLocTriggerForOp; the OP variant is the AP
// variant + 7 (TS offset convention; see interaction_trigger.go:194-198).
func registerOpLocScript(t *testing.T, s *Server, typeID int, op int, sf *script.ScriptFile) {
	t.Helper()
	if s.scriptProvider == nil {
		s.scriptProvider = script.NewProvider()
	}
	trigger := script.TriggerOpLoc1 + script.ServerTriggerType(op-1)
	sf.LookupKey = script.LookupKeyForType(trigger, typeID)
	s.scriptProvider.Register(sf)
}

func registerApLocScript(t *testing.T, s *Server, typeID int, op int, sf *script.ScriptFile) {
	t.Helper()
	if s.scriptProvider == nil {
		s.scriptProvider = script.NewProvider()
	}
	trigger, ok := apLocTriggerForOp(op)
	if !ok {
		t.Fatalf("apLocTriggerForOp(%d) returned ok=false", op)
	}
	sf.LookupKey = script.LookupKeyForType(trigger, typeID)
	s.scriptProvider.Register(sf)
}
```

You'll also need to import `"github.com/zsrv/goscape/pkg/script"` if not already imported. `buildPOpLocScript` is pre-existing in `interaction_trigger_nai68_test.go:29`. `apLocTriggerForOp` lives at `interaction_trigger.go:200`. There is NO `opLocTriggerForOp`; compute the OP trigger inline as shown above.

- [ ] **Step 2 — Run test, verify fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTryInteractBranchTrackingPerCallsite -v
```

Expected: FAIL — branches all return 0 (no recordTryInteractBranch calls inserted yet).

- [ ] **Step 3 — Wire `recordTryInteractBranch` into all 5 sites in `tryInteract`.**

Edit `modules/world/interaction.go`. The current `tryInteract` body (lines 331-400 from the spec) has four `return true` statements (one per branch) plus a trailing `return false`. Insert `recordTryInteractBranch(p, N)` BEFORE each return — note branch 2's NAI-69 same-tick retry has TWO returns (line 377 `return false` and line 379 `return true`), and the same branch number (2) applies to both since they're the same logical branch.

Diff (apply via Edit tool, one Edit per site for safety):

```go
	// Branch 1 — OP fire (TS Player.ts:1123).
	if opTrigger != nil && (isPathing || allowOpScenery) && operable {
		p.interacted = true
		if !p.interactionFired {
			tryFireOpTrigger(p)
		}
		recordTryInteractBranch(p, 1) // NAI-79 Stage 1
		return true
	}

	// Branch 2 — AP fire (TS Player.ts:1139).
	if apTrigger != nil && approach {
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		// NAI-69 same-tick retry (TS Player.ts:1158-1167). Closes
		// NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.
		if p.nextTarget == nil && p.apRangeCalled {
			p.interactionFired = false
			recordTryInteractBranch(p, 2) // NAI-79 Stage 1 (retry-no-op)
			return false
		}
		recordTryInteractBranch(p, 2) // NAI-79 Stage 1
		return true
	}

	// Branch 3 — default-AP no-op (TS Player.ts:1173-1175).
	// ... existing comment block ...
	if approach {
		p.apRange = -1
		recordTryInteractBranch(p, 3) // NAI-79 Stage 1
		return false
	}

	// Branch 4 — default-OP NIH (TS Player.ts:1179-1182).
	if (isPathing || allowOpScenery) && operable {
		defaultOp(p)
		recordTryInteractBranch(p, 4) // NAI-79 Stage 1
		return true
	}

	recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (fallthrough)
	return false
```

Also handle the early-return at `if p.target == nil { return false }` (line 333). This early-return means tryInteract was called with no target — record branch 0 there as well so the test's "no target" code path doesn't accidentally surface as fallthrough on a stale field:

```go
	if p.target == nil {
		recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (no-target early-return)
		return false
	}
```

- [ ] **Step 4 — Run test, verify pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTryInteractBranchTrackingPerCallsite -v
```

Expected: PASS for all 10 sub-tests (5 branches × 2 slots).

- [ ] **Step 5 — Run full package tests; verify no regression.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. (Branch tracking is additive; no behavior change in tryInteract.)

- [ ] **Step 6 — Commit.**

```bash
git add modules/world/interaction.go modules/world/interaction_debug_test.go
git commit --no-gpg-sign -m "feat(world): NAI-79 T3 — branch tracking inside tryInteract

Each tryInteract return now writes its branch id (0=fallthrough/early-
return, 1..4) to lastInteractBranchPre or Post via recordTryInteractBranch,
keyed by p.interactCallSlot. processInteraction sets the slot before each
call (T4)."
```

---

## Task 4 — Frame B emit at `processInteraction` tail

**Files:**
- Modify: `modules/world/interaction.go:151-262` (`processInteraction` body)
- Test: `modules/world/interaction_debug_test.go` (add 2 tests)

This task instruments `processInteraction` to (a) set `interactCallSlot=0` before pre-step `tryInteract`, `=1` before post-step, (b) capture pre-step state into local variables, and (c) emit a single Frame B `slog.Debug` record at the function tail. Frame B emits only when `p.target != nil` was true at function ENTRY and `s.cfg.NodeDebug` is true.

- [ ] **Step 1 — Write failing tests for Frame B.**

Append to `modules/world/interaction_debug_test.go`:

```go
func TestInteractionFrameB_EmittedWhenTargetSetAndNodeDebugTrue(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	logger, h := newCapturingLogger()
	s.log = logger
	s.cfg.NodeDebug = true
	s.scriptProvider = script.NewProvider() // empty; force fallthrough/branch 3

	// Place player 2 tiles away — approach distance with default ap_range=10
	// (set by SetInteraction). Target Loc; no scripts → branch 3 pre-step,
	// then post-step pathToTarget no-op (Loc has no waypoints generated for
	// shape-blind path in this fixture; that's acceptable here — the test
	// only verifies frame emission, not pathing correctness).
	p.x, p.z = 98, 100

	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.uid = 12345

	p.processInteraction()

	rec := findRecord(h.snapshot(), "interaction tick")
	if rec == nil {
		t.Fatal("expected one 'interaction tick' record; got none")
	}
	requireAttr(t, *rec, "player_uid", "12345")
	requireAttr(t, *rec, "target_kind", "Loc")
	if v, ok := attrValue(*rec, "target_x"); !ok || v.Int64() != 100 {
		t.Errorf("target_x: got %v, want 100", v)
	}
	if v, ok := attrValue(*rec, "target_z"); !ok || v.Int64() != 100 {
		t.Errorf("target_z: got %v, want 100", v)
	}
	if _, ok := attrValue(*rec, "cheb_dist"); !ok {
		t.Errorf("cheb_dist missing")
	}
	if _, ok := attrValue(*rec, "branch_pre"); !ok {
		t.Errorf("branch_pre missing")
	}
	if _, ok := attrValue(*rec, "branch_post"); !ok {
		t.Errorf("branch_post missing")
	}
	if _, ok := attrValue(*rec, "waypoint_idx"); !ok {
		t.Errorf("waypoint_idx missing")
	}
	if _, ok := attrValue(*rec, "target_still_set"); !ok {
		t.Errorf("target_still_set missing")
	}
}

func TestInteractionFrameB_SuppressedWhenNoTargetAtEntry(t *testing.T) {
	s, p, _, _ := makeOpLocFixture(t)
	logger, h := newCapturingLogger()
	s.log = logger
	s.cfg.NodeDebug = true

	// p.target is nil (default after newTestPlayer); processInteraction
	// short-circuits at the first guard (interaction.go:170-172). No
	// frame should emit.
	p.processInteraction()

	if rec := findRecord(h.snapshot(), "interaction tick"); rec != nil {
		t.Errorf("unexpected 'interaction tick' record: %v", rec)
	}
}

func TestInteractionFrameB_SuppressedWhenNodeDebugFalse(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	logger, h := newCapturingLogger()
	s.log = logger
	s.cfg.NodeDebug = false
	s.scriptProvider = script.NewProvider()

	p.x, p.z = 98, 100
	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.processInteraction()

	if rec := findRecord(h.snapshot(), "interaction tick"); rec != nil {
		t.Errorf("unexpected 'interaction tick' record: %v", rec)
	}
}
```

- [ ] **Step 2 — Run tests, verify fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestInteractionFrameB -v
```

Expected: FAIL — no records emitted (Frame B not yet implemented).

- [ ] **Step 3 — Implement Frame B in `processInteraction`.**

Edit `modules/world/interaction.go`. Modify `processInteraction` (lines 169-262) to:

1. Capture pre-step state immediately after the early-return guards.
2. Set `p.interactCallSlot = 0` before pre-step `tryInteract` call (line 205).
3. Set `p.interactCallSlot = 1` before post-step `tryInteract` call (line 228).
4. Reset slot to 0 at function start.
5. Reset `lastInteractBranchPre`/`Post` to 0 at function start (so a tick with no calls leaves them clean rather than carrying stale values from the previous tick).
6. Emit Frame B at the function tail when `hadTarget && s.cfg.NodeDebug`.

Apply this Edit (the function body changes substantially; do it as one Edit replacing the whole function from `func (p *Player) processInteraction() {` through its closing `}`):

```go
func (p *Player) processInteraction() {
	if p.target == nil {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	// DEVIATION NAI-44-D-CANACCESS-NO-STUN-CHECK: TS canAccess() also tests
	// stun/freeze; goscape has no stun system, so the !p.delayed subset is
	// the in-tree approximation.
	if p.delayed && s.currentTick < p.delayedUntil {
		return
	}

	// NAI-79 Stage 1 — pre-step state capture for Frame B emit at tail.
	// All target-coord fields refer to the INITIAL target; target_still_set
	// separately signals whether p.target was nulled during the tick.
	hadTarget := true
	initialTarget := p.target
	initialTargetX, initialTargetZ, _ := p.target.Coords()
	opTriggerPresent := getOpTrigger(p, s) != nil
	apTriggerPresent := getApTrigger(p, s) != nil
	p.lastInteractBranchPre = 0
	p.lastInteractBranchPost = 0
	p.interactCallSlot = 0

	// TS L1201-1202.
	p.followX = p.lastStepX
	p.followZ = p.lastStepZ
	// TS L1203.
	p.nextTarget = nil

	followOp := isFollowOp(p)

	_, _, tlevel := p.target.Coords()
	if tlevel != p.level {
		p.ClearInteraction()
		sendUnsetMapFlag(p)
		// NAI-79 Stage 1 — emit Frame B even on level-mismatch clear so
		// the captured log shows the cross-level ClearInteraction case.
		emitInteractionTickFrame(s, p, hadTarget, initialTarget,
			initialTargetX, initialTargetZ, opTriggerPresent,
			apTriggerPresent, false /*interactedFinal*/)
		return
	}

	interacted := false

	// Pre-step interact arm (TS L1209-1224).
	if !followOp {
		p.processWalktrigger()
	}
	p.interactCallSlot = 0
	interacted = p.tryInteract(false)

	// Post-step arm (TS L1227-1252). Skipped when pre-step interacted.
	if !interacted {
		// Recalc path (TS L1228-1229).
		if !p.repathed {
			tx, tz, _ := p.target.Coords()
			p.pathToTarget(tx, tz)
			p.repathed = true
		}

		if p.hasWaypoints() {
			p.processWalktrigger()
		}

		// followOp + waypoint exhaustion → clear (TS L1237-1239).
		if !p.hasWaypoints() && followOp {
			p.ClearInteraction()
		}

		// Post-step interact (TS L1244-1252). Skipped when followOp
		// (the chase keeps interaction anchored across steps).
		if p.target != nil && !followOp {
			p.interactCallSlot = 1
			interacted = p.tryInteract(p.stepsTaken == 0)
			if !interacted && !p.hasWaypoints() && p.stepsTaken == 0 {
				p.MessageGame("I can't reach that!")
				p.ClearInteraction()
			}
		}
	}

	// nextTarget pop + auto-clear (TS L1255-1263). When an OP/AP
	// trigger script called p_op_* mid-trigger, the fire helpers
	// captured the script-set target into p.nextTarget; pop it here.
	// Otherwise, auto-clear the interaction. followOp paths can still
	// reach the else-if when tryInteract returned true at the pre-step
	// arm (contact range with target=*Player op=3); TS does the same —
	// followOp gates SKIP post-step-interact, not the auto-clear
	// itself. NAI-68 closed NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET via
	// this reshape; NAI-69 closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED
	// by routing the same-tick retry signal through tryInteract.
	if p.nextTarget != nil {
		p.target = p.nextTarget
	} else if interacted && !p.apRangeCalled {
		p.ClearInteraction()
	}

	// Tail mapflag clear (TS L1266-1268). When the player has consumed
	// at least one step this tick and no waypoints remain, clear the
	// client's pending map-click indicator. Without this, a player who
	// walks a full path without reaching the target (path blocked, target
	// moved out of reach) leaves the yellow X on screen until the next
	// click. Idempotent against the auto-clear above (which also nulls
	// waypoints via ClearInteraction).
	if !p.hasWaypoints() && p.stepsTaken > 0 {
		sendUnsetMapFlag(p)
	}

	// NAI-79 Stage 1 — Frame B emit at tail. Gated on hadTarget (a
	// tick with no target at entry should never emit) and NodeDebug.
	emitInteractionTickFrame(s, p, hadTarget, initialTarget,
		initialTargetX, initialTargetZ, opTriggerPresent,
		apTriggerPresent, interacted)
}
```

Then add `emitInteractionTickFrame` to `modules/world/interaction_debug.go`:

```go
// emitInteractionTickFrame writes the NAI-79 Stage 1 "interaction tick"
// Frame B record. Caller (processInteraction) gates on hadTarget; this
// helper additionally gates on s.cfg.NodeDebug. All target-coord fields
// refer to the INITIAL target (snapshotted by the caller at function
// entry); target_still_set separately signals whether p.target was
// nulled during the tick.
func emitInteractionTickFrame(
	s *Server,
	p *Player,
	hadTarget bool,
	initialTarget entity,
	initialTargetX, initialTargetZ int,
	opTriggerPresent, apTriggerPresent bool,
	interactedFinal bool,
) {
	if !hadTarget || !s.cfg.NodeDebug || s.log == nil {
		return
	}
	s.log.Debug("interaction tick",
		"tick", s.currentTick,
		"player_uid", p.uid,
		"target_kind", targetKindString(initialTarget),
		"target_type_id", initialTarget.Type(),
		"target_x", initialTargetX,
		"target_z", initialTargetZ,
		"player_x", p.x,
		"player_z", p.z,
		"cheb_dist", chebDist(p.x, p.z, initialTargetX, initialTargetZ),
		"op_trigger", opTriggerPresent,
		"ap_trigger", apTriggerPresent,
		"ap_range", p.apRange,
		"waypoint_idx", p.waypointIndex,
		"branch_pre", p.lastInteractBranchPre,
		"branch_post", p.lastInteractBranchPost,
		"interacted", interactedFinal,
		"interaction_fired", p.interactionFired,
		"steps_taken", p.stepsTaken,
		"repathed", p.repathed,
		"target_still_set", p.target != nil,
	)
}
```

Note: the `entity` parameter type must match whatever the existing interaction system uses. Verify by reading the type of `Player.target`. If it's `entity` (file-local interface), the parameter type is correct as written.

- [ ] **Step 4 — Run tests, verify pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestInteractionFrameB -v
```

Expected: PASS for all 3 sub-tests.

- [ ] **Step 5 — Run full package tests; verify no regression.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. (Frame B is additive; the only behavior change in `processInteraction` is the slot-flag write before each tryInteract call, which is semantically a no-op on the un-instrumented path.)

- [ ] **Step 6 — Run full repo tests + race detector.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...
```

Expected: both PASS.

- [ ] **Step 7 — Commit.**

```bash
git add modules/world/interaction.go modules/world/interaction_debug.go modules/world/interaction_debug_test.go
git commit --no-gpg-sign -m "feat(world): NAI-79 T4 — Frame B emit at processInteraction tail

processInteraction now captures pre-step state (initial target/coords +
op/ap trigger presence) and emits a single 'interaction tick' slog.Debug
record at the function tail under Cfg.NodeDebug. interactCallSlot is
set to 0/1 before pre-step/post-step tryInteract calls so the branch
tracker (T3) writes to the right field. Frame B is the H1/H3 evidence
channel per NAI-79 spec §5."
```

---

## Task 5 — Frame A emit at `handleOpLoc` tail

**Files:**
- Modify: `modules/world/handler_oploc.go:25-96` (`handleOpLoc`)
- Test: `modules/world/interaction_debug_test.go` (add 2 tests)

This task adds the handler-side frame so the captured log shows H2 cache-key evidence (`loc_name`, `op_slot`) plus the Loc geometry inputs (`loc_shape`, `loc_angle`, `lt_width`, `lt_length`) that drive H1's `FindPathDefault` shape-blindness. Emitted on the success path only (after `targetSubject` snapshot, before the final `return nil`).

- [ ] **Step 1 — Write failing tests for Frame A.**

Append to `modules/world/interaction_debug_test.go`:

```go
func TestInteractionFrameA_EmittedWhenNodeDebugTrue(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	logger, h := newCapturingLogger()
	s.log = logger
	s.cfg.NodeDebug = true
	s.currentTick = 42
	p.uid = 99

	if err := handleOpLoc1(p, p2x3Payload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1: %v", err)
	}

	rec := findRecord(h.snapshot(), "oploc handler")
	if rec == nil {
		t.Fatal("expected one 'oploc handler' record; got none")
	}
	requireAttr(t, *rec, "player_uid", "99")
	requireAttr(t, *rec, "tick", "42")
	requireAttr(t, *rec, "op", "1")
	requireAttr(t, *rec, "loc_id", "42")
	requireAttr(t, *rec, "loc_name", "test_loc") // from makeOpLocFixture LocType.DebugName
	requireAttr(t, *rec, "op_slot", "op1")       // from fixture's Op[0]
	if v, ok := attrValue(*rec, "lt_width"); !ok || v.Int64() != 1 {
		t.Errorf("lt_width: got %v, want 1", v)
	}
	if v, ok := attrValue(*rec, "lt_length"); !ok || v.Int64() != 1 {
		t.Errorf("lt_length: got %v, want 1", v)
	}
	if v, ok := attrValue(*rec, "loc_shape"); !ok || v.Int64() != int64(loc.Shape()) {
		t.Errorf("loc_shape: got %v, want %d", v, loc.Shape())
	}
	if v, ok := attrValue(*rec, "loc_angle"); !ok || v.Int64() != int64(loc.Angle()) {
		t.Errorf("loc_angle: got %v, want %d", v, loc.Angle())
	}
}

func TestInteractionFrameA_SuppressedWhenNodeDebugFalse(t *testing.T) {
	s, p, _, _ := makeOpLocFixture(t)
	logger, h := newCapturingLogger()
	s.log = logger
	s.cfg.NodeDebug = false

	if err := handleOpLoc1(p, p2x3Payload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpLoc1: %v", err)
	}

	if rec := findRecord(h.snapshot(), "oploc handler"); rec != nil {
		t.Errorf("unexpected 'oploc handler' record: %v", rec)
	}
}

func TestInteractionFrameA_NotEmittedOnFailedHandler(t *testing.T) {
	s, p, _, _ := makeOpLocFixture(t)
	logger, h := newCapturingLogger()
	s.log = logger
	s.cfg.NodeDebug = true

	// Send a coord outside the viewport (originX=100, max delta 52) →
	// handler returns early at the viewport gate without setting
	// interaction. Frame A should not emit because we only emit on
	// the success path.
	if err := handleOpLoc1(p, p2x3Payload(200, 200, 42)); err != nil {
		t.Fatalf("handleOpLoc1: %v", err)
	}

	if rec := findRecord(h.snapshot(), "oploc handler"); rec != nil {
		t.Errorf("unexpected 'oploc handler' record on failed handler: %v", rec)
	}
}
```

- [ ] **Step 2 — Run tests, verify fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestInteractionFrameA -v
```

Expected: FAIL — no records emitted.

- [ ] **Step 3 — Implement Frame A in `handleOpLoc`.**

Edit `modules/world/handler_oploc.go`. Insert the Frame A emit immediately before the success `return nil` (current line 95, after `p.targetSubject.level = loc.Level`):

```go
	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, loc, op, -1)
	p.opcalled = true
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level

	// NAI-79 Stage 1 — handler frame for H1/H2 evidence channel.
	if s.cfg.NodeDebug && s.log != nil {
		s.log.Debug("oploc handler",
			"tick", s.currentTick,
			"player_uid", p.uid,
			"op", op,
			"click_x", x,
			"click_z", z,
			"loc_id", locId,
			"loc_name", locType.DebugName,
			"loc_shape", loc.Shape(),
			"loc_angle", loc.Angle(),
			"lt_width", locType.Width,
			"lt_length", locType.Length,
			"op_slot", locType.Op[op-1],
		)
	}
	return nil
}
```

(Apply this Edit only to the `handleOpLoc` function — the OPLOCT/OPLOCU handlers are out of scope for NAI-79 Stage 1; the door symptom is OPLOC1.)

- [ ] **Step 4 — Run tests, verify pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestInteractionFrameA -v
```

Expected: PASS for all 3 sub-tests.

- [ ] **Step 5 — Run full package tests; verify no regression.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS.

- [ ] **Step 6 — Run full repo tests.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 7 — Build the binary; verify clean.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o /tmp/goscape-nai79-t5 ./cmd/goscape
```

Expected: clean build; binary at `/tmp/goscape-nai79-t5`.

- [ ] **Step 8 — Commit.**

```bash
git add modules/world/handler_oploc.go modules/world/interaction_debug_test.go
git commit --no-gpg-sign -m "feat(world): NAI-79 T5 — Frame A emit at handleOpLoc tail

handleOpLoc now emits an 'oploc handler' slog.Debug record on the
success path under Cfg.NodeDebug, carrying click coords + locId +
LocType DebugName/Width/Length + Loc Shape/Angle + Op[op-1] trigger
key. This is the H1 (geometry) + H2 (cache-key) evidence channel
per NAI-79 spec §5. OPLOCT / OPLOCU are out of scope for the door
symptom; instrumentation lives only on handleOpLoc."
```

---

## Stage 1 close (no further task needed yet)

After T5 commits, controller emits a smoke-handoff resume prompt to the user (per spec §7), then waits for captured-log paste. The Stage 2 plan-update materializes after routing (one of bundles H1/H2/H3/H4 per spec §6); that plan-update is authored in a fresh session.

---

## Self-Review (run after writing all tasks)

**1. Spec coverage:**
- §4.1 Frame A → T5. ✓
- §4.2 Frame B → T4. ✓
- §4.3 Branch tracking → T3 (instrumentation) + T2 (data + helper). ✓
- §4.4 Helpers → T1. ✓
- §4.5 Pre-step state capture → T4 (inside processInteraction). ✓
- §8 Tests:
  - TestInteractionFrameA_EmittedWhenNodeDebugTrue → T5. ✓
  - TestInteractionFrameA_SuppressedWhenNodeDebugFalse → T5. ✓
  - TestInteractionFrameB_EmittedWhenTargetSetAndNodeDebugTrue → T4. ✓
  - TestInteractionFrameB_SuppressedWhenNoTargetAtEntry → T4. ✓
  - TestBranchTracking_Branch1Through4_PerCallsite → T3. ✓
  - TestChebDistAndTargetKindString → T1. ✓
- Bonus tests beyond spec: T4 adds NodeDebug=false suppression (parallels Frame A), T5 adds failed-handler suppression. Both are confidence-elevators, not scope creep.

**2. Placeholder scan:** No TBD/TODO/"appropriate error handling" in any task. Every code-step has the actual code.

**3. Type consistency:**
- `lastInteractBranchPre`/`Post` int (T2) — referenced as int in T3 test + T3 instrumentation + T4 emit. ✓
- `interactCallSlot` int (T2) — set to 0/1 in T4 processInteraction; read in T2 helper. ✓
- `recordTryInteractBranch(p *Player, branch int)` (T2) — called with `recordTryInteractBranch(p, N)` in T3. ✓
- `chebDist(ax, az, bx, bz int) int` (T1) — called in T4 emitInteractionTickFrame. ✓
- `targetKindString(t entity) string` (T1) — called in T4 emitInteractionTickFrame. ✓
- `emitInteractionTickFrame(...)` (T4) — single declaration site; signature consistent with caller in processInteraction. ✓
- Frame B field `interaction_fired` reads `p.interactionFired` (existing field). Frame A field `op_slot` reads `locType.Op[op-1]` (existing slice).

**4. Risk surface:**
- Mode flag `interactCallSlot` is per-Player; concurrent Player calls don't cross. ✓
- `processInteraction` is called sequentially per player from `tick.go`'s loop; no race on `lastInteractBranchPre`/`Post`. ✓
- Frame B emits even when the level-mismatch ClearInteraction path runs — this is intentional (captures cross-level ClearInteraction in the log) and not a regression of the existing tests (since they don't currently assert on log records).
- Branch 0 ("fallthrough") collides with the `p.target == nil` early-return record. Both record `branch=0`, which is correct: in either case `tryInteract` did NOT take a documented branch. The test in T3 covers the standard fallthrough; the no-target case is rare in practice.

**5. Build/test commands consistent:** All steps use `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` per global CLAUDE.md. ✓
