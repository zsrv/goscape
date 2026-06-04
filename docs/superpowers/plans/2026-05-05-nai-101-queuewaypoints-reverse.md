# NAI-101 — `queueWaypoints` TS-required input-reversal

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the TS-required reverse-copy semantic into goscape's `(*Player).queueWaypoints` and `(*Npc).queueWaypoints`. The functions currently store `packed` in natural src→dst order; TS `PathingEntity.queueWaypoints` (PathingEntity.ts:248-254) reverses to dst→src. Without the reverse, `stepOnce` reads `waypoints[n-1] = dest` and `Face` heads straight at the destination, ignoring intermediate detour direction-change points. Closes the NAI-100 path-around residual.

**Architecture:** TDD. Two tiny production edits (~10 LOC total) at the storage-orientation boundary. Red-first unit tests pin the reverse-copy semantic and the truncation direction. A regression-style stepOnce-iteration test pins that detour direction-change-points are now consumed correctly. A real-cache full-stack Lumbridge fountain test (new file) verifies post-fix path-around behavior end-to-end.

**Tech Stack:** Go 1.26+; goscape engine; `LostCityRS/Engine-TS` canonical reference. Existing helpers reused: `newTestPlayer`, `newTestNpc`, `newTestServer`, `discardLogger`, `gamemap.New/Init`, `objtype.LoadLocTypes`, `populateStaticLocsIntoZones`, `packTestCoord`.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-101-queuewaypoints-reverse-design.md`.
**Predecessor:** NAI-100 (commit `a45c123`) shipped fountain footprint coverage; smoke 2026-05-05 surfaced the path-around residual.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `modules/world/movement.go` | Modify (lines 14-29) | Replace `(*Player).queueWaypoints` natural-order copy with TS-faithful reverse-copy; update doc-comment with TS citation. |
| `modules/world/npc_ai.go` | Modify (lines 90-107) | Replace `(*Npc).queueWaypoints` natural-order copy with TS-faithful reverse-copy; update doc-comment, cross-reference Player. |
| `modules/world/movement_test.go` | Modify (append) | Add 3 unit tests: reverse-order pin, truncation-direction pin, multi-waypoint stepOnce regression. |
| `modules/world/npc_movement_test.go` | Modify (append) | Add 2 unit tests: Npc reverse-order pin, Npc multi-waypoint stepOnce regression. |
| `modules/world/nai101_fountain_test.go` | Create | Real-cache Lumbridge fountain regression test (skip-if-absent for CI portability). |

---

## Conventions

- All `go` invocations use the project's required prefix: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
- All commits use `git commit --no-gpg-sign`.
- Follow modern Go guidelines (project uses Go 1.26+; `for range N` is fine; `any` for empty interface; `min`/`max` builtins available).
- Tests use existing test helpers; do not add new ones.
- Doc-comment style: cite `Engine-TS PathingEntity.ts:LINE-LINE` per project convention.

---

## Task 1: Player `queueWaypoints` reverse-copy + unit tests (TDD red→green)

**Files:**
- Modify: `modules/world/movement.go:14-29`
- Modify: `modules/world/movement_test.go` (append three new tests)

**Background:** Per spec §2 + §6, `(*Player).queueWaypoints` currently stores `packed[i] → waypoints[i]` (natural order); TS `PathingEntity.queueWaypoints` reverses on copy so that `waypoints[0] = packed[length-1]`. With reversed storage, `stepOnce`'s read of `waypoints[waypointIndex=n-1]` correctly returns the **first step** (closest to source), not the destination.

Pre-fix audit (per spec §9): only `stepOnce` reads `p.waypoints[*]` in production. Existing tests are single-element (reverse is identity for n=1) or sentinel-preservation (order-orthogonal). Zero collateral expected.

- [ ] **Step 1.1: Write failing test — reverse-order pin**

Append to `modules/world/movement_test.go`:

```go
// TestQueueWaypointsReversesInputOrder pins TS PathingEntity.queueWaypoints
// (Engine-TS/src/engine/entity/PathingEntity.ts:248-254): packed arrives in
// src→dst order ([first_step, …, dest]); queueWaypoints reverses on copy so
// internal storage is [dest, …, first_step]. stepOnce's read of
// waypoints[waypointIndex=n-1] then returns first_step.
func TestQueueWaypointsReversesInputOrder(t *testing.T) {
	p, _ := newTestPlayer(t)

	a := packTestCoord(0, 3100, 3100) // first_step
	b := packTestCoord(0, 3105, 3105) // mid
	c := packTestCoord(0, 3110, 3110) // dest
	packed := []int{a, b, c}

	p.queueWaypoints(packed)

	if p.waypointIndex != 2 {
		t.Errorf("waypointIndex: got %d, want 2 (n-1)", p.waypointIndex)
	}
	if p.waypoints[0] != c {
		t.Errorf("waypoints[0]: got 0x%X, want 0x%X (= packed[2] = dest)", p.waypoints[0], c)
	}
	if p.waypoints[1] != b {
		t.Errorf("waypoints[1]: got 0x%X, want 0x%X (= packed[1] = mid)", p.waypoints[1], b)
	}
	if p.waypoints[2] != a {
		t.Errorf("waypoints[2]: got 0x%X, want 0x%X (= packed[0] = first_step)", p.waypoints[2], a)
	}
}
```

- [ ] **Step 1.2: Run test — verify it FAILS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestQueueWaypointsReversesInputOrder -v`

Expected: FAIL. Pre-fix output should show `waypoints[0]: got 0x...A..., want 0x...C...` (natural-order storage).

- [ ] **Step 1.3: Write failing test — truncation-direction pin**

Append to `modules/world/movement_test.go`:

```go
// TestQueueWaypointsTruncatesFarEntries pins TS PathingEntity.queueWaypoints
// truncation behavior (PathingEntity.ts:248-254 inner condition output <
// this.waypoints.length): when packed exceeds the waypoints buffer length,
// the entries closest to dest are preserved and far-from-dest entries are
// dropped. This matches TS because TS iterates input from length-1 down to
// 0 while output is bounded above by waypoints.length.
//
// Goscape's Player.waypoints is a fixed-size [25]int. With 30-element
// packed input, the 5 entries at packed[0..4] (closest to source) are
// dropped; packed[5..29] reversed are stored at waypoints[0..24].
func TestQueueWaypointsTruncatesFarEntries(t *testing.T) {
	p, _ := newTestPlayer(t)

	const inLen = 30
	if inLen <= len(p.waypoints) {
		t.Fatalf("test fixture broken: inLen=%d must exceed len(p.waypoints)=%d", inLen, len(p.waypoints))
	}
	packed := make([]int, inLen)
	for i := range packed {
		packed[i] = packTestCoord(0, 3000+i, 3000)
	}

	p.queueWaypoints(packed)

	bufLen := len(p.waypoints)
	if p.waypointIndex != bufLen-1 {
		t.Errorf("waypointIndex: got %d, want %d (buffer cap)", p.waypointIndex, bufLen-1)
	}
	// Storage[i] = packed[inLen-1-i] for i in [0, bufLen). The last
	// bufLen entries of packed (the dest-end) are preserved; packed[0..4]
	// (source-end) are dropped.
	for i := range bufLen {
		want := packed[inLen-1-i]
		if p.waypoints[i] != want {
			t.Errorf("waypoints[%d]: got 0x%X, want 0x%X (= packed[%d])", i, p.waypoints[i], want, inLen-1-i)
		}
	}
}
```

- [ ] **Step 1.4: Run test — verify it FAILS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestQueueWaypointsTruncatesFarEntries -v`

Expected: FAIL. Pre-fix output should show `waypoints[0]` matching `packed[0]` (natural order with truncation at the end).

- [ ] **Step 1.5: Write failing test — multi-waypoint stepOnce regression pin**

Append to `modules/world/movement_test.go`:

```go
// TestStepOnceFollowsDirectionChangePoints is the regression pin for the
// NAI-101 root cause. Pre-fix, with packed=[first_step, mid, dest] stored
// natural-order, stepOnce reads waypoints[n-1] = dest and uses Face to head
// straight at dest, ignoring the routed mid waypoint. Post-fix, reversed
// storage means waypoints[n-1] = first_step; stepOnce iterates through
// each direction-change point in turn.
//
// Scenario: player at (3094, 3106). Route N to (3094, 3110), then E to
// (3097, 3110). Pre-fix Face from (3094, 3106) to dest (3097, 3110) returns
// DirectionNortheast (heads NE diagonally), bypassing the routed N→E shape.
// Post-fix Face from (3094, 3106) to first_step (3094, 3107) returns
// DirectionNorth (correct first step).
func TestStepOnceFollowsDirectionChangePoints(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedWalk

	firstStep := packTestCoord(0, 3094, 3107)
	mid := packTestCoord(0, 3094, 3110)
	dest := packTestCoord(0, 3097, 3110)
	p.queueWaypoints([]int{firstStep, mid, dest})

	// Tick 1: should step N (toward first_step), not NE (toward dest).
	p.resolveMovement()
	if p.x != 3094 || p.z != 3107 {
		t.Fatalf("tick 1: got (%d,%d), want (3094,3107) [N step toward first_step]; "+
			"pre-fix bug heads NE toward dest", p.x, p.z)
	}
}
```

- [ ] **Step 1.6: Run test — verify it FAILS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestStepOnceFollowsDirectionChangePoints -v`

Expected: FAIL. Pre-fix output should show `got (3095,3107)` (NE step toward dest, not N step toward first_step).

- [ ] **Step 1.7: Apply the fix — replace `(*Player).queueWaypoints`**

Edit `modules/world/movement.go:14-29` to:

```go
// queueWaypoints replaces the current path with the given packed coords.
// Mirrors TS PathingEntity.queueWaypoints (Engine-TS PathingEntity.ts:248-254):
// reverses the input on copy so that internal storage is [dest, …, first_step].
// stepOnce reads waypoints[waypointIndex] starting at n-1 (= first_step) and
// decrements toward 0 (= dest).
//
// Truncation: when len(packed) exceeds len(p.waypoints), entries closest to
// dest are preserved (input iterates from length-1 down; output bounded above
// by waypoints buffer cap). TS-faithful: TS truncates the same way via
// output < this.waypoints.length.
func (p *Player) queueWaypoints(packed []int) {
	if len(packed) == 0 {
		p.waypointIndex = -1
		return
	}
	index := -1
	for input, output := len(packed)-1, 0; input >= 0 && output < len(p.waypoints); input, output = input-1, output+1 {
		p.waypoints[output] = packed[input]
		index++
	}
	p.waypointIndex = index
}
```

- [ ] **Step 1.8: Run all three new tests — verify they PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestQueueWaypointsReversesInputOrder|TestQueueWaypointsTruncatesFarEntries|TestStepOnceFollowsDirectionChangePoints' -v`

Expected: all 3 PASS.

- [ ] **Step 1.9: Run full `modules/world` test suite — verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: PASS. Per spec §9 audit, existing tests are either single-element (n=1 makes reverse identity) or sentinel-preservation (don't go through queueWaypoints with multi-element packed). If any test fails, STOP and report — investigate before proceeding.

- [ ] **Step 1.10: Commit**

```bash
git add modules/world/movement.go modules/world/movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-101 T1 — Player queueWaypoints TS-faithful reverse-copy

Mirrors PathingEntity.queueWaypoints (Engine-TS PathingEntity.ts:248-254):
input arrives in [first_step, …, dest] order (BFS-natural and protocol-
natural); reverse-copy on store yields [dest, …, first_step] internal
storage so stepOnce's read of waypoints[waypointIndex=n-1] returns the
first step. Pre-fix natural-order storage caused stepOnce to read the
destination and Face-walk straight at it, bypassing intermediate
direction-change points produced by routefinder BFS for detour paths.

Three unit tests pin the reverse-order semantic, the truncation direction
(closest-to-dest preserved), and the multi-waypoint stepOnce iteration.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Npc `queueWaypoints` reverse-copy + symmetric tests

**Files:**
- Modify: `modules/world/npc_ai.go:90-107`
- Modify: `modules/world/npc_movement_test.go` (append two new tests)

**Background:** `(*Npc).queueWaypoints` has the identical bug as `(*Player).queueWaypoints` (per spec §9 audit: same `n.waypoints[i] = packed[i]` natural-order copy at `npc_ai.go:104`). NPC-side `stepOnce` at `npc_interaction.go:348` reads `n.waypoints[n.waypointIndex]` symmetrically. Fix is symmetric.

- [ ] **Step 2.1: Write failing test — Npc reverse-order pin**

Append to `modules/world/npc_movement_test.go`:

```go
// TestNpcQueueWaypointsReversesInputOrder is the Npc-side analogue of
// TestQueueWaypointsReversesInputOrder. Per Engine-TS PathingEntity.ts:248-254,
// the reverse-copy semantic is shared between Player and Npc (both inherit
// queueWaypoints from PathingEntity in TS).
func TestNpcQueueWaypointsReversesInputOrder(t *testing.T) {
	n := newTestNpc(1)

	a := packTestCoord(0, 3100, 3100) // first_step
	b := packTestCoord(0, 3105, 3105) // mid
	c := packTestCoord(0, 3110, 3110) // dest
	n.queueWaypoints([]int{a, b, c})

	if n.waypointIndex != 2 {
		t.Errorf("waypointIndex: got %d, want 2 (n-1)", n.waypointIndex)
	}
	if n.waypoints[0] != c {
		t.Errorf("waypoints[0]: got 0x%X, want 0x%X (= packed[2] = dest)", n.waypoints[0], c)
	}
	if n.waypoints[1] != b {
		t.Errorf("waypoints[1]: got 0x%X, want 0x%X (= packed[1] = mid)", n.waypoints[1], b)
	}
	if n.waypoints[2] != a {
		t.Errorf("waypoints[2]: got 0x%X, want 0x%X (= packed[0] = first_step)", n.waypoints[2], a)
	}
}
```

- [ ] **Step 2.2: Run test — verify it FAILS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcQueueWaypointsReversesInputOrder -v`

Expected: FAIL.

- [ ] **Step 2.3: Write failing test — Npc multi-waypoint stepOnce regression pin**

Append to `modules/world/npc_movement_test.go`:

```go
// TestNpcStepOnceFollowsDirectionChangePoints is the Npc-side regression
// pin for NAI-101. Mirror of TestStepOnceFollowsDirectionChangePoints.
//
// NPC at (3094, 3106). Route N to (3094, 3110), then E to (3097, 3110).
// Pre-fix Face from (3094, 3106) to dest (3097, 3110) returns NE diagonal,
// skipping the mid waypoint. Post-fix iterates first_step → mid → dest.
func TestNpcStepOnceFollowsDirectionChangePoints(t *testing.T) {
	s := newTestServer(t)

	n := newTestNpc(1)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.lastTickX, n.lastTickZ = n.x, n.z

	firstStep := packTestCoord(0, 3094, 3107)
	mid := packTestCoord(0, 3094, 3110)
	dest := packTestCoord(0, 3097, 3110)
	n.queueWaypoints([]int{firstStep, mid, dest})

	// One updateMovement tick should step N (toward first_step), not NE.
	moved := n.updateMovement(s)
	if !moved {
		t.Fatalf("updateMovement: got false, want true (one step queued)")
	}
	if n.x != 3094 || n.z != 3107 {
		t.Fatalf("tick 1: got (%d,%d), want (3094,3107) [N step toward first_step]; "+
			"pre-fix bug heads NE toward dest", n.x, n.z)
	}
}
```

- [ ] **Step 2.4: Run test — verify it FAILS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStepOnceFollowsDirectionChangePoints -v`

Expected: FAIL. Pre-fix `got (3095,3107)`.

- [ ] **Step 2.5: Apply the fix — replace `(*Npc).queueWaypoints`**

Edit `modules/world/npc_ai.go:90-107` to:

```go
// queueWaypoints replaces the current path with the given packed coords.
// Mirrors TS PathingEntity.queueWaypoints (Engine-TS PathingEntity.ts:248-254);
// cross-reference (*Player).queueWaypoints (modules/world/movement.go).
//
// Reverses the input on copy so that internal storage is [dest, …, first_step].
// stepOnce reads waypoints[waypointIndex] starting at n-1 (= first_step) and
// decrements toward 0 (= dest). Truncation drops far-from-dest entries when
// input exceeds the waypoint buffer cap (TS-faithful).
//
// Unexported because external script-VM callers use QueueWaypoint
// (single-step) only.
func (n *Npc) queueWaypoints(packed []int) {
	if len(packed) == 0 {
		n.waypointIndex = -1
		return
	}
	index := -1
	for input, output := len(packed)-1, 0; input >= 0 && output < len(n.waypoints); input, output = input-1, output+1 {
		n.waypoints[output] = packed[input]
		index++
	}
	n.waypointIndex = index
}
```

- [ ] **Step 2.6: Run both new Npc tests — verify they PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcQueueWaypointsReversesInputOrder|TestNpcStepOnceFollowsDirectionChangePoints' -v`

Expected: both PASS.

- [ ] **Step 2.7: Run full `modules/world` test suite — verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: PASS. If any existing Npc test fails (notably any test that pre-loads `n.waypoints[*]` with multi-element data via `n.queueWaypoints` and checks subsequent step direction), STOP and report.

- [ ] **Step 2.8: Commit**

```bash
git add modules/world/npc_ai.go modules/world/npc_movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-101 T2 — Npc queueWaypoints TS-faithful reverse-copy

Symmetric to T1 (Player queueWaypoints). Mirrors PathingEntity.ts:248-254;
cross-reference (*Player).queueWaypoints. Two unit tests pin the reverse-
order semantic and Npc-side stepOnce iteration through direction-change
points.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Real-cache Lumbridge fountain regression test

**Files:**
- Create: `modules/world/nai101_fountain_test.go`

**Background:** Per spec §7.2, the full-stack regression test exercises the entire pathfinder→queueWaypoints→stepOnce pipeline against the real Lumbridge cache with NAI-100's fountain footprint coverage. This pins the smoke target (player at (3222, 3225) reaching adjacent S of NPC at (3219, 3230)).

Skip-if-absent guard mirrors `static_loc_collision_test.go:28-34` for CI portability.

- [ ] **Step 3.1: Write the test file**

Create `modules/world/nai101_fountain_test.go`:

```go
package world

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// TestNAI101_FountainPathAround_RealCache pins the full-stack
// pathfinder → queueWaypoints → stepOnce path-around behavior against the
// real Lumbridge cache with NAI-100's fountain footprint coverage.
//
// Scenario: player at (3222, 3225) requests a route to NPC tile (3219, 3230)
// past the 4-tile fountain footprint (3221..3222, 3226..3227). Pre-NAI-101,
// queueWaypoints stored route in natural src→dst order; stepOnce read
// waypoints[n-1]=dest and Face headed straight NW into the FlagLoc-blocked
// (3221, 3226). Post-NAI-101, reversed storage means stepOnce reads first
// direction-change point (3220, 3225), walks W around the fountain, then N,
// then NW to reach (3219, 3229) (entity-reach adjacent S of NPC).
//
// Skip-if-absent guard keeps the test CI-portable; pattern mirrors
// TestNAI95_StaticLocCollision_HansArea.
func TestNAI101_FountainPathAround_RealCache(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	locTypes, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}
	s.locTypes = locTypes
	s.gamemap.SetLocTypes(locTypes)

	if err := s.gamemap.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}

	s.populateStaticLocsIntoZones()

	// Sanity-check NAI-100 footprint coverage still holds at HEAD.
	t.Run("FountainFootprintFlagged", func(t *testing.T) {
		want := [][2]int{{3221, 3226}, {3221, 3227}, {3222, 3226}, {3222, 3227}}
		for _, c := range want {
			flag := s.gamemap.Pathfinder.Flags.Get(c[0], c[1], 0)
			if flag&collision.FlagLoc == 0 {
				t.Errorf("(%d, %d, 0): flag=0x%x missing FlagLoc bit (NAI-100 regression?)", c[0], c[1], flag)
			}
		}
	})

	// Pin the routefinder output shape (3 direction-change points) the
	// stepOnce iteration must traverse. Bundle 0 probe captured this exact
	// shape at HEAD `a45c123` (post-NAI-100):
	//   [0] (3220, 3225, 0)
	//   [1] (3220, 3229, 0)
	//   [2] (3219, 3230, 0)
	t.Run("FindPathPlain_ProducesDetour", func(t *testing.T) {
		route := s.gamemap.Pathfinder.FindPathPlain(0, 3222, 3225, 3219, 3230)
		if !route.Success {
			t.Fatalf("Success=false; route=%+v", route)
		}
		if route.Alternative {
			t.Fatalf("Alternative=true; want false (full reach, not closest-approach); route=%+v", route)
		}
		if len(route.Waypoints) < 2 {
			t.Fatalf("len(Waypoints)=%d; want ≥2 (detour around fountain); route=%+v", len(route.Waypoints), route)
		}
		// Smoke-evidence pin: last waypoint must be the destination tile.
		last := route.Waypoints[len(route.Waypoints)-1]
		if last.X() != 3219 || last.Z() != 3230 {
			t.Errorf("last waypoint: got (%d, %d), want (3219, 3230)", last.X(), last.Z())
		}
	})

	// Full-stack regression: queue the routefinder's output, tick movement,
	// observe player ends adjacent to dest and stepsTaken > 0.
	t.Run("StepThroughDetour", func(t *testing.T) {
		p, _ := newTestPlayer(t)
		p.client.server = s
		p.x, p.z, p.level = 3222, 3225, 0
		p.moveSpeed = MoveSpeedRun
		p.runenergy = 10000

		route := s.gamemap.Pathfinder.FindPathPlain(0, 3222, 3225, 3219, 3230)
		if !route.Success {
			t.Fatalf("FindPathPlain failed: %+v", route)
		}

		packed := make([]int, 0, len(route.Waypoints))
		for _, wp := range route.Waypoints {
			packed = append(packed, coordgrid.PackCoord(wp.Level(), wp.X(), wp.Z()))
		}
		p.queueWaypoints(packed)

		// Tick up to 12 times. Run-step covers ≤2 tiles per tick; the path
		// is ~5-7 tiles around the fountain, so 12 is generous.
		const maxTicks = 12
		stepsTotal := 0
		for tick := 0; tick < maxTicks; tick++ {
			p.resolveMovement()
			stepsTotal += p.stepsTaken
			if p.waypointIndex < 0 {
				break
			}
		}

		if stepsTotal == 0 {
			t.Fatalf("stepsTotal=0; player never moved (path lost on tick 1 — NAI-101 bug not fixed)")
		}
		// Final position: dest tile (3219, 3230). FindPathPlain
		// (vs. FindPathToEntity) reaches dest exactly.
		if p.x != 3219 || p.z != 3230 {
			t.Errorf("final position: got (%d, %d), want (3219, 3230); stepsTotal=%d, waypointIndex=%d",
				p.x, p.z, stepsTotal, p.waypointIndex)
		}
		// Player must NOT have stepped onto a fountain tile.
		// (Stronger: per-step audit. Lighter: just verify final and stepsTotal.)
		if p.x == 3221 || p.x == 3222 {
			if p.z == 3226 || p.z == 3227 {
				t.Errorf("player ended on fountain tile (%d, %d) — collision check bypassed", p.x, p.z)
			}
		}
	})
}
```

- [ ] **Step 3.2: Run the test — verify it PASSES at HEAD-with-T1+T2-fix**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNAI101_FountainPathAround_RealCache -v`

Expected: all subtests PASS. (T1+T2 already landed before T3 runs, so the test passes on first green.)

If `t.Skipf` fires for the cache guard (data files absent), that's acceptable. If subtests FAIL with the cache present, STOP and report — the integration shape may differ from Bundle 0 probe expectations.

- [ ] **Step 3.3: Sanity check — full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS or skip. Project-wide check ensures no other package depends on the natural-order storage convention (audit per spec §9 said no, but cross-package check is cheap).

- [ ] **Step 3.4: Commit**

```bash
git add modules/world/nai101_fountain_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-101 T3 — full-stack fountain path-around regression

Real-cache Lumbridge regression test for the NAI-101 fix. Player at
(3222, 3225) queues a 3-waypoint detour route past the fountain footprint
flagged by NAI-100, ticks movement, asserts arrival at (3219, 3230) with
stepsTotal > 0. Pre-NAI-101 fix this would FAIL on tick 1 because
queueWaypoints stored natural-order, stepOnce read dest, Face headed NW
into the FlagLoc-blocked fountain, waypointIndex cleared to -1.

Skip-if-absent guard for data/pack mirrors TestNAI95_StaticLocCollision_HansArea.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Rollup close commit

**Files:**
- (No code changes; commit metadata only.)

**Background:** Per `close_commit_memory_trailer` memory + project convention, sub-spec close commits include a `Closes memory:` trailer for grep-discoverable provenance and bind the PRIMARY/SECONDARY split.

PRIMARY (TS-faithful port) closes here. SECONDARY (smoke target) is bound at user smoke; if smoke surfaces a residual, it routes per spec §8 decision tree to NAI-102 or Bundle 3.

- [ ] **Step 4.1: Verify no uncommitted changes from prior tasks**

Run: `git status --short`

Expected: clean (any output is from `.claude/`, `.bashrc`, etc. — not from `modules/world/` or `docs/superpowers/`).

- [ ] **Step 4.2: Verify commit graph is what we expect**

Run: `git --no-pager log --oneline -5`

Expected first 4 entries (newest to oldest):
- `<sha> test(world): NAI-101 T3 — full-stack fountain path-around regression`
- `<sha> feat(world): NAI-101 T2 — Npc queueWaypoints TS-faithful reverse-copy`
- `<sha> feat(world): NAI-101 T1 — Player queueWaypoints TS-faithful reverse-copy`
- `<sha> docs(spec): NAI-101 — queueWaypoints missing TS-required input-reversal`

If unexpected, STOP and report.

- [ ] **Step 4.3: Run full test suite one more time as a final pre-close gate**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS.

- [ ] **Step 4.4: Create empty rollup commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-101 — queueWaypoints TS-faithful reverse-copy

PRIMARY (TS-faithful port) closes: (*Player).queueWaypoints and
(*Npc).queueWaypoints now reverse-copy input on store, mirroring
PathingEntity.ts:248-254. stepOnce's waypoints[n-1] read returns the
first direction-change point (closest to source) instead of the
destination; multi-waypoint detour paths around obstacles now consume
intermediate waypoints correctly.

SECONDARY (smoke target — player walks W around Lumbridge fountain
to reach NPC at (3219, 3230)) is bound at user smoke; if a residual
surfaces, route per the spec §8 decision tree.

Bundle 0 controller pre-flight done in spec session; Stage 1 audit
short-circuited (line-of-code TS-source diff was binding); single
Stage 2 fix bundle (T1+T2+T3); ~12 production LOC + 6 unit/integration
tests.

Closes memory: nai_100_path_around_residual

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4.5: Verify the close commit**

Run: `git --no-pager log -1 --pretty=fuller`

Expected: shows the close commit with the `Closes memory:` trailer.

---

## Smoke handoff

After Task 4 commits, the controller should pause and ask the user to launch the server with the post-fix binary, walk the Lumbridge fountain scenario, and report results. The smoke is the binding test for the SECONDARY commitment.

**User script:**
1. Build: `CGO_ENABLED=0 go build -trimpath -o /go/bin/goscape ./cmd/goscape`
2. Launch with `--config.file config.yaml`.
3. Connect with the Java client; spawn at Lumbridge (3221, 3218).
4. Walk NW past (3222, 3225); right-click NPC at (3219, 3230) and select an OP.
5. Observe whether the player walks W around the fountain to reach the NPC.

**Decision tree (per spec §8):**
- **Symptom resolved:** PRIMARY + SECONDARY both close. Update `nai_100_path_around_residual.md` memory entry to `CLOSED`. Done.
- **Symptom-shape changes:** PRIMARY closes; SECONDARY routes to NAI-102 with new evidence captured.
- **Symptom-shape unchanged:** PRIMARY blocked; Bundle 3 investigates (binary built with fix? re-queue source? nil-guard skip?).
- **Different symptom:** stepOnce iteration regression; Bundle 3 re-audits.
