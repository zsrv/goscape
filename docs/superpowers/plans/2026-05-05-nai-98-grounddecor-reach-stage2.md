# NAI-98 GroundDecor Reach Stage 2 Narrow-Then-Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Narrow NAI-97's diagnosis ceiling (sub-H6 BFS / sub-H7 StepValidator vs BFS / sub-H8 tickloop) via a real-cache integration test, then surgically fix the surfaced sub-H so the NAI-96 close-day smoke residuals (Repro A: NPC 943 path-around-fountain; Repro B: NPC 3 mid-route abandonment) close.

**Architecture:** Single sub-spec, two phases with a controller-driven plan-amendment checkpoint between them. Phase 1 ships a real-cache three-signal probe in `pkg/gamemap` that pins which sub-H fires. Controller reads the Phase 1 commit output, re-greps premises against HEAD, diffs the surfaced predicate against upstream source (TS for H8; Rust `rsmod-pathfinder` branch 225 for H6/H7), drafts Phase 2 task block, dispatches. Phase 2 is sub-H-conditional surgical fix. User-launched smoke binds the close.

**Tech Stack:** Go 1.26+. Upstream sources: `LostCityRS/Engine-TS` (per `ts_source_canonical_path`) and `2004scape/rsmod-pathfinder` branch 225 (per `rust_source_canonical_path`).

**Spec:** `docs/superpowers/specs/2026-05-05-nai-98-grounddecor-reach-stage2-design.md`

---

## File Structure

**Created (Phase 1, this plan):**
- `pkg/gamemap/nai98_realcache_probe_test.go` — real-cache three-signal H6/H7/H8 probe; two test functions for Repro A + Repro B sharing a `runRealCacheReachProbe` helper.

**Created (Phase 2, plan-amendment):**
- Sub-H-conditional regression test (file path determined by surfaced sub-H — see spec §5.1/5.2/5.3).

**Modified (Phase 2, plan-amendment):**
- Sub-H-conditional production code (one of: `pkg/pathfinder/routefinder/routefinder.go` + maybe `pkg/pathfinder/reach/`; OR `pkg/gamemap/gamemap.go` + `pkg/pathfinder/routefinder/routefinder.go`; OR `modules/world/interaction.go` + maybe `modules/world/movement.go`).

**Deleted (Phase 2 close):**
- `pkg/pathfinder/routefinder/nai97_repro_test.go` — empty-grid degenerate per `empty_flagmap_degenerate_routefinder`; superseded by the real-cache probe.

**Modified (close):**
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` — close & supersede `nai_96_grounddecor_path_around_residual.md` entry; update `pathfinder_api_loc_aware.md` rename note.
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/pathfinder_api_loc_aware.md` — `FindPathDefault` → `FindPathPlain`.
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — append "From NAI-98" section.
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_96_grounddecor_path_around_residual.md` — close status, OR rename to a closed-marker file.

**Read-only references (Phase 1):**
- `pkg/gamemap/gamemap.go` (`CanTravel` at :97-100; `ChangeLocCollision` at :61-78; `StaticLocs()` at :167-169)
- `pkg/gamemap/nai97_loc_walk_test.go` (existing pattern for real-cache test setup)
- `pkg/objtype/loctype.go` (`LoadLocTypes`, `LocType.BlockWalk` field)
- `pkg/pathfinder/routefinder/api.go` (`FindPathToEntity` at :47)
- `pkg/pathfinder/routefinder/route.go` (`Route` struct)
- `pkg/pathfinder/routefinder/routecoordinates.go` (`RouteCoordinates.X()` / `.Z()` accessors)
- `pkg/pathfinder/routefinder/routefinder.go:130-153` (waypoint-construction loop — direction-change-point semantics)

---

## Conventions for this plan

- **`go` invocation prefix** (per global CLAUDE.md): every `go test`/`go build` runs as `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
- **Subagent fabrication guard** (`audit_subagent_fabrication`, `verify_implementer_claims`): controller verifies every claimed file:line citation by independently rerunning `go test -v` and reading the output, NOT trusting the subagent's commit message.
- **Plan-amendment, not replace_all** (`plan_doc_replaceall_timeline`): when controller fills in Phase 2 task block after Phase 1 commit, use Edit per task with task-section context, never `replace_all`.
- **Phase 1 commit may fail CI.** If H6/H7 fires, `t.Fatalf` paths fire. Phase 1 commit message explicitly notes "Phase 1 narrowing test — failure is the diagnostic signal for plan-amendment; do not revert." Controller does NOT gate Phase 2 dispatch on green CI for the Phase 1 commit.
- **`/clear` between plan and Phase 1 implementer dispatch** (`superpowers_clear_between_spec_and_impl`): after this plan lands, controller emits resume prompt and stops; user `/clear`s before Phase 1 dispatch.

---

## Task 1: Bundle 0 — controller pre-flight (no commits)

**Purpose:** Verify spec §1 + §3.2 + §4 premises against HEAD before dispatching Phase 1. Stale citations cause wasted implementer cycles (`controller_preflight`).

**Files:** read-only.

- [ ] **Step 1.1: Confirm HEAD baseline**

```bash
git log --oneline -5
```

Expected: `dc7eb69 docs(spec): NAI-98 — correct H7 probe algorithm` and `dd77fa2 docs(spec): NAI-98 — GroundDecor reach Stage 2 narrow-then-fix` near the top, above `2a99116 chore(close): NAI-97`.

- [ ] **Step 1.2: Verify `gm.CanTravel` signature**

```bash
sed -n '95,102p' pkg/gamemap/gamemap.go
```

Expected: `func (gm *GameMap) CanTravel(level, x, z, offsetX, offsetZ int) bool` at line 97; body delegates to `gm.Pathfinder.StepValidator.CanTravel(level, x, z, offsetX, offsetZ, 1, 0, collision.TypeNormal)`. **If signature has changed, halt and update spec §3.2 Step 4.**

- [ ] **Step 1.3: Verify `gm.Pathfinder.FindPathToEntity` signature**

```bash
sed -n '40,55p' pkg/pathfinder/routefinder/api.go
```

Expected: `FindPathToEntity(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength int) Route` at line 47. **If renamed/reshaped, halt and update spec.**

- [ ] **Step 1.4: Verify `gm.StaticLocs()` accessor**

```bash
sed -n '165,172p' pkg/gamemap/gamemap.go
```

Expected: `func (gm *GameMap) StaticLocs() []*entity.Loc` at line 169. **If absent, the Phase 1 test cannot replay collision writes; halt.**

- [ ] **Step 1.5: Verify Stage 1.1 dump test pattern available for reference**

```bash
test -f pkg/gamemap/nai97_loc_walk_test.go && echo "OK: nai97 dump test present" || echo "MISSING: cannot reuse pattern"
```

Expected: `OK: nai97 dump test present`. The Phase 1 test mirrors this file's setup boilerplate (cache load, populateStaticLocsIntoZones replay).

- [ ] **Step 1.6: Verify cache fixture availability**

```bash
ls data/pack/server/maps/m48_50 data/pack/server/loc.dat 2>&1 | head -5
```

Expected: both present. (If absent on the implementer's machine, the test `t.Skipf`s rather than failing.)

- [ ] **Step 1.7: Verify waypoint-construction direction-change semantics**

```bash
sed -n '125,160p' pkg/pathfinder/routefinder/routefinder.go
```

Expected: line 130-153 loop appends a waypoint only when `currDir != nextDir`; waypoints are prepended to the slice (closest-to-source first). **Confirms spec §3.2 Step 4 H7 algorithm.** If this loop has been refactored, halt and re-derive H7 stepping.

**Bundle 0 exit:** all 7 sub-steps return expected values. No commits. If any halt fires, escalate to user before Phase 1 dispatch.

---

## Task 2: Phase 1 — real-cache three-signal probe

**Purpose:** Land the real-cache integration test that pins which sub-H (H6/H7/H8) fires on the smoke geometry. Phase 1 commit output is the input to the plan-amendment checkpoint.

**Files:**
- Create: `pkg/gamemap/nai98_realcache_probe_test.go`

**Reference pattern:** `pkg/gamemap/nai97_loc_walk_test.go` (cache load + populateStaticLocsIntoZones replay).

- [ ] **Step 2.1: Write the failing test**

Create `pkg/gamemap/nai98_realcache_probe_test.go` with:

```go
package gamemap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestNAI98_RealCacheReachProbe_NPC943 — Repro A: player (3221, 3218),
// NPC 943 at (3218, 3216). Three-signal probe pins which sub-hypothesis
// (H6 BFS / H7 StepValidator vs BFS / H8 tickloop) fires. See spec
// §3.2 in docs/superpowers/specs/2026-05-05-nai-98-grounddecor-reach-stage2-design.md
func TestNAI98_RealCacheReachProbe_NPC943(t *testing.T) {
	runRealCacheReachProbe(t, 3221, 3218, 3218, 3216)
}

// TestNAI98_RealCacheReachProbe_NPC3 — Repro B: player (3218, 3213),
// NPC 3 at (3223, 3216). Same probe shape, different geometry.
func TestNAI98_RealCacheReachProbe_NPC3(t *testing.T) {
	runRealCacheReachProbe(t, 3218, 3213, 3223, 3216)
}

func runRealCacheReachProbe(t *testing.T, srcX, srcZ, dstX, dstZ int) {
	t.Helper()

	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	// Setup: real cache load + LocTypes + production populateStaticLocsIntoZones
	// replay (mirrors modules/world/server.go:315-330).
	gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}
	cfgs, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}
	for _, l := range gm.StaticLocs() {
		ltID := l.Type()
		if ltID < 0 || ltID >= len(cfgs.Configs) {
			continue
		}
		lt := cfgs.Configs[ltID]
		if lt == nil || !lt.BlockWalk {
			continue
		}
		gm.ChangeLocCollision(l.Shape(), l.Angle(), lt.BlockRange,
			l.Length, l.Width, lt.Active, l.X, l.Z, l.Level, true)
	}

	const (
		level      = 0
		srcSize    = 1
		destWidth  = 1
		destLength = 1
	)

	// Signal H6 — BFS / reach predicate.
	route := gm.Pathfinder.FindPathToEntity(level, srcX, srcZ, dstX, dstZ, srcSize, destWidth, destLength)
	if !route.Success {
		t.Fatalf("H6 FIRES: FindPathToEntity Success=false on real-cache geometry. Route=%+v", route)
	}
	if len(route.Waypoints) == 0 {
		t.Fatalf("H6 FIRES: FindPathToEntity returned zero waypoints on real-cache geometry. Route=%+v", route)
	}
	last := route.Waypoints[len(route.Waypoints)-1]
	if dx := last.X() - dstX; dx < -1 || dx > 1 {
		t.Fatalf("H6 FIRES: last waypoint=(%d,%d) cheb-X=%d > 1 from dst=(%d,%d). Route=%+v",
			last.X(), last.Z(), dx, dstX, dstZ, route)
	}
	if dz := last.Z() - dstZ; dz < -1 || dz > 1 {
		t.Fatalf("H6 FIRES: last waypoint=(%d,%d) cheb-Z=%d > 1 from dst=(%d,%d). Route=%+v",
			last.X(), last.Z(), dz, dstX, dstZ, route)
	}

	// Signal H7 — StepValidator vs BFS-CanMove divergence.
	// BFS waypoints are direction-change points (per spec §3.2; routefinder.go:130-153);
	// walk single tiles along each waypoint→waypoint straight segment.
	x, z := srcX, srcZ
	for segIdx, wp := range route.Waypoints {
		dx, dz := wp.X()-x, wp.Z()-z
		sx := sgn(dx)
		sz := sgn(dz)
		if sx == 0 && sz == 0 {
			t.Skipf("Phase 1 surfaces unexpected route shape (degenerate same-tile waypoint at segment %d). Route=%+v", segIdx, route)
		}
		for x != wp.X() || z != wp.Z() {
			if !gm.CanTravel(level, x, z, sx, sz) {
				t.Fatalf("H7 FIRES at sub-step (%d,%d)→(%d,%d) inside segment %d/%d (waypoint (%d,%d)→(%d,%d)) step=(%d,%d) but CanTravel=false. Route=%+v",
					x, z, x+sx, z+sz, segIdx+1, len(route.Waypoints), x, z, wp.X(), wp.Z(), sx, sz, route)
			}
			x += sx
			z += sz
		}
	}

	// Signal H8 — by elimination.
	t.Logf("H8 FIRES by elimination on (%d,%d)→(%d,%d): BFS path internally consistent (%d waypoints, last=(%d,%d)) and StepValidator-walkable. Phase 2 must investigate tickloop-level state mutation in modules/world/.",
		srcX, srcZ, dstX, dstZ, len(route.Waypoints), last.X(), last.Z())
}

// sgn returns the sign of x in {-1, 0, 1}.
func sgn(x int) int {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}
```

- [ ] **Step 2.2: Run the test to capture which signal fires**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -v ./pkg/gamemap/... -run TestNAI98_RealCacheReachProbe
```

Expected: ONE of the following four outcomes per Repro:

- `H6 FIRES: …` — sub-H6 (BFS / reach) is the surfaced hypothesis.
- `H7 FIRES at sub-step …` — sub-H7 (StepValidator vs BFS) is the surfaced hypothesis.
- `H8 FIRES by elimination …` (via `t.Logf`, with test passing) — sub-H8 (tickloop) is the surfaced hypothesis; Phase 2 must continue at modules/world tickloop layer.
- `t.Skipf("Phase 1 surfaces unexpected route shape …")` — outside {H6, H7, H8}; halt Phase 2, escalate to user (per spec §3.3).

Capture verbatim test output. Both Repro A and Repro B may surface different sub-Hs; controller handles divergent surfacing in plan-amendment.

- [ ] **Step 2.3: Verify the rest of the test suite is unaffected**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/... ./pkg/pathfinder/...
```

Expected: pre-existing tests unchanged. Only the two `TestNAI98_RealCacheReachProbe_*` tests may fail (per Step 2.2). If any *other* test fails, halt — the Phase 1 test's setup is leaking state.

- [ ] **Step 2.4: Commit**

```bash
git add pkg/gamemap/nai98_realcache_probe_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(gamemap): NAI-98 Phase 1 — real-cache 3-signal H6/H7/H8 probe

Phase 1 narrowing test for NAI-97 diagnosis ceiling. Pins which
sub-hypothesis fires on the smoke geometry:
  - H6: FindPathToEntity returns no path / out-of-cheb waypoint.
  - H7: BFS waypoint produces a sub-step where CanTravel disagrees.
  - H8: by elimination if H6+H7 pass.

Phase 1 narrowing test — failure is the diagnostic signal for
plan-amendment; do not revert.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Phase 1 exit:** test commits with one of the four outcomes pinned. Controller takes over for the plan-amendment checkpoint (Task 3).

---

## Task 3: PHASE 1 → PHASE 2 PLAN-AMENDMENT CHECKPOINT (controller-only)

**STATUS: PLACEHOLDER — controller fills in after Task 2 commit per spec §4 plan-amendment checkpoint.**

This task does NOT execute via subagent dispatch. The controller (main session) executes it directly.

**Controller checklist (spec §4 verbatim):**

- [ ] **Step 3.1: Read Phase 1 test output verbatim**

Re-run Phase 1 at HEAD locally — do not trust Task 2 implementer's commit message:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -v ./pkg/gamemap/... -run TestNAI98_RealCacheReachProbe 2>&1 | tee $TMPDIR/nai98_phase1_output.log
```

Pin the verbatim output for §3.5 below.

- [ ] **Step 3.2: Confirm sub-H surfaced ∈ {H6, H7, H8}**

If output contains `t.Skipf("Phase 1 surfaces unexpected route shape …")`, halt and escalate to user. Do NOT proceed to Phase 2 plan-amendment.

If both Repro A and Repro B surface the SAME sub-H, single-track plan-amendment.

If they surface DIFFERENT sub-Hs, plan-amendment must address both (Phase 2 may need two task blocks, dispatched independently).

- [ ] **Step 3.3: Re-grep premises against HEAD per surfaced sub-H**

For sub-H6: re-Read `pkg/pathfinder/routefinder/routefinder.go:107, 117-153, 165-180`. Confirm symbol shapes match spec §5.1.

For sub-H7: re-Read `pkg/gamemap/gamemap.go:97-101` `CanTravel` + `pkg/pathfinder/routefinder/routefinder.go:187, 197` BFS clip-flag. Confirm symbol shapes match spec §5.2.

For sub-H8: re-Read `modules/world/interaction.go:87, 135, 171-294, 236-239, 255-258, 454` + `modules/world/movement.go:34-115`. Confirm symbol shapes match spec §5.3.

If any premise is stale, update spec inline before Phase 2 dispatch (per `plan_doc_replaceall_timeline`: per-section Edit, not replace_all).

- [ ] **Step 3.4: Diff against upstream source**

For sub-H6: clone or open `2004scape/rsmod-pathfinder` branch 225; diff goscape's `routeFindSize1` + `findClosestApproachPoint` against the Rust equivalents. Pin divergence shape verbatim.

For sub-H7: same upstream; diff goscape `CanTravel` + BFS clip-flag selection against Rust `StepValidator`-equivalent + BFS predicate.

For sub-H8: open `LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts` repathed-equivalent + tickloop ordering in `LostCityRS/Engine-TS/src/engine/entity/Player.ts` and `Npc.ts`. Per `ts_base_class_read_for_inherited_behavior`: read both leaf and base.

- [ ] **Step 3.5: Draft Phase 2 task block**

Open `docs/superpowers/plans/2026-05-05-nai-98-grounddecor-reach-stage2.md` (this file). Replace the §"Task 4: Phase 2 — sub-H-conditional surgical fix (PLACEHOLDER)" block with concrete tasks: one task per identified divergence; per-task code blocks include verbatim before/after snippets sourced from `Read` at HEAD. Use Edit per section, not `replace_all` (per `plan_doc_replaceall_timeline`).

Each Phase 2 task follows the standard plan task structure (TDD: write failing test → run → minimal impl → run → commit). Sub-H-specific patterns:

- **Sub-H6:** test fixture allocated via `internal.BuildCollisionMap` (per `empty_flagmap_degenerate_routefinder`); per-predicate unit test in `pkg/pathfinder/routefinder/`.
- **Sub-H7:** test constructs known FlagMap via `internal.BuildCollisionMap`; runs BFS over it; walks result through `CanTravel`; asserts agreement.
- **Sub-H8:** test in `modules/world/`; mocks Player + Interaction; drives ticks; asserts `waypointIndex` and `target` lifecycle.

- [ ] **Step 3.6: Pre-flight Phase 2 task premises**

Per `controller_preflight`: every file path, line number, and signature in the drafted Phase 2 tasks must be verified at HEAD before dispatch. Per `plan_runnable_test_fixtures`: mentally execute each test fixture to catch self-catches.

- [ ] **Step 3.7: Commit the plan amendment**

```bash
git add docs/superpowers/plans/2026-05-05-nai-98-grounddecor-reach-stage2.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(plan): NAI-98 Phase 2 task block — sub-H<N> drafted post-Phase-1

Phase 1 commit <SHA> surfaced sub-H<N> via <signal>. Plan-amendment
drafts Phase 2 fix tasks per spec §<5.N>.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.8: Dispatch Phase 2 implementer(s)**

Per `execution_mode_default`: subagent-driven-development. One subagent per Phase 2 task. Per `superpowers_code_reviewer_model`: post-implementer code-reviewer agent runs on Sonnet, not Opus.

**Checkpoint exit:** plan amendment committed; Phase 2 tasks dispatched.

---

## Task 4: Phase 2 — sub-H8 fix: port TS Player.pathToPathingTarget gate

**Sub-H surfaced:** H8 (tickloop-level state mutation). Phase 1 commit `daf1e28` confirmed both repros via `t.Logf("H8 FIRES by elimination")`:
- Repro A (NPC 943, src=(3221,3218)→dst=(3218,3216)): `BFS path internally consistent (2 waypoints, last=(3219,3216))` — last waypoint cheb-1 of dst.
- Repro B (NPC 3, src=(3218,3213)→dst=(3223,3216)): `BFS path internally consistent (2 waypoints, last=(3222,3216))` — last waypoint cheb-1 of dst.

H6 + H7 do not fire on either geometry. BFS is internally consistent and StepValidator-walkable. Reach abandonment is at tickloop-level.

**Root cause (verbatim from upstream-source diff):**

TS `Player.processInteraction` at `LostCityRS/Engine-TS/src/engine/entity/Player.ts:1228-1229` calls `this.pathToPathingTarget()` **UNCONDITIONALLY** each tick when `!interacted`:

```ts
if (!interacted) {
    // Recalc path
    this.pathToPathingTarget();
    ...
}
```

TS `Player.pathToPathingTarget` (Player.ts:1034-1055) gates SMART repath internally on `isLastOrNoWaypoint()` (PathingEntity.ts:374-376: `waypointIndex <= 0`):

```ts
pathToPathingTarget(): void {
    if (!(this.target instanceof PathingEntity)) {
        return;
    }
    if (this.isLastOrNoWaypoint() && (this.targetOp === ServerTriggerType.APPLAYER3 || this.targetOp === ServerTriggerType.OPPLAYER3)) {
        this.queueWaypoint(this.target.followX, this.target.followZ);
        return;
    }
    if (!this.canAccess()) {
        return;
    }
    if (Environment.NODE_CLIENT_ROUTEFINDER && CoordGrid.intersects(...)) {
        this.queueWaypoints(findNaivePath(...));
        return;
    }
    if (this.isLastOrNoWaypoint()) {
        this.pathToTarget();
    }
}
```

TS `repathed` field at `PathingEntity.ts:64` is declared and reset in `Player.resetEntity` (Player.ts:459) but is **NEVER READ** elsewhere in TS — verified via `grep -rn "repathed" Engine-TS/src/`. It is a vestigial dead field.

goscape `interaction.go:236-239` misinterprets the dead-field-presence as live-gate-semantics:

```go
if !p.repathed {
    p.pathToTarget()
    p.repathed = true
}
```

This is a **once-per-interaction-lifecycle gate** (only reset by `SetInteraction` at :87 and `ClearInteraction` at :135). After the first repath fires, it is OFF for the remainder of the interaction. Path-exhaustion mid-interaction never re-paths → `!hasWaypoints && stepsTaken==0 && !interacted` → "I can't reach that" + `ClearInteraction` at interaction.go:255-258. This is the smoke shape on both repros.

**Files:**
- Create: `modules/world/interaction_h8_test.go` — TDD regression test (red → fix → green).
- Modify: `modules/world/interaction.go` — add `pathToPathingTarget` method; replace gate at :236-239.
- Modify: `modules/world/interaction_test.go` — rewrite `repathed`-as-pathing-fired assertions at :184 and :560-561 to assert directly on `p.waypointIndex`.

**Files NOT modified (deviation rationale):**
- `repathed` field declaration on Player struct, resets in `SetInteraction`/`ClearInteraction`, debug emit at `interaction_debug.go:67` — kept as TS-vestigial (TS retains the field declared + reset). Field becomes inert (no production reader) post-fix; tests at :76, :91, :108, :985 that exercise reset-to-false or set-as-state stay valid.
- `modules/world/movement.go:34-115` (resolveMovement → stepOnce ordering) — DEVIATION (documented at interaction.go:168-170: "Goscape's updateMovement runs in processPathing BEFORE processInteractions; TS embeds it inline at L1241"). Out of H8 fix scope; preserved.

### Step 4.1: Write failing regression test

Create `modules/world/interaction_h8_test.go`:

```go
package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// TestProcessInteractionRepathsAfterPathExhaustion — NAI-98 sub-H8 regression.
// Pre-NAI-98: interaction.go:236-239 gates pathToTarget() on `!p.repathed`,
// a once-per-interaction-lifecycle boolean. Post-fix: pathToPathingTarget
// gates SMART repath on isLastOrNoWaypoint (TS Player.ts:1034-1055,
// PathingEntity.ts:374-376).
//
// Repro shape: player anchors target NPC at cheb=15 (out of apRange=10);
// first processInteraction queues path; we manually exhaust the path
// (waypointIndex=-1 with target still anchored); second processInteraction
// must re-queue path. Pre-fix: !p.repathed gate is false on tick 2 →
// pathToTarget skipped → !hasWaypoints && stepsTaken==0 → "I can't reach
// that" + ClearInteraction (target=nil).
func TestProcessInteractionRepathsAfterPathExhaustion(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeClientRoutefinder = true
	npc := makeInteractionNpc(t, s, 1, 115, 100, 0) // cheb=15

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	// Tick 1: initial pathToPathingTarget queues path.
	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	<-received

	if p.waypointIndex < 0 {
		t.Fatalf("tick 1: waypointIndex=%d, want >= 0 (initial repath)", p.waypointIndex)
	}
	if p.target == nil {
		t.Fatal("tick 1: target cleared unexpectedly")
	}

	// Simulate path exhaustion mid-interaction.
	p.waypointIndex = -1

	// Tick 2: pathToPathingTarget MUST re-queue path. Pre-fix would skip
	// because !p.repathed=false, then "I can't reach that" + Clear fires.
	received = drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	<-received

	if p.target == nil {
		t.Errorf("tick 2: target cleared (interaction abandoned). Pre-fix !p.repathed gate prevented repath; expected pathToPathingTarget to re-queue path on isLastOrNoWaypoint.")
	}
	if p.waypointIndex < 0 {
		t.Errorf("tick 2: waypointIndex=%d after path exhaustion. pathToPathingTarget did not re-queue waypoints. This is the H8 bug: TS Player.ts:1052-1054 gates on isLastOrNoWaypoint; goscape gated on !p.repathed once-per-interaction.", p.waypointIndex)
	}
}
```

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -v ./modules/world/... -run TestProcessInteractionRepathsAfterPathExhaustion
```

Expected: RED. Tick 2's `target==nil` and `waypointIndex<0` fire.

### Step 4.2: Add `pathToPathingTarget` method on Player

Insert in `modules/world/interaction.go` directly before `pathToTarget` (around current line 566). Body mirrors TS `Player.pathToPathingTarget` (Player.ts:1034-1055), with documented divergences:

```go
// pathToPathingTarget mirrors TS Player.pathToPathingTarget
// (Engine-TS/src/engine/entity/Player.ts:1034-1055). Called once per tick
// from processInteraction's post-step branch when !interacted (TS L1228-1229).
//
// Dispatch:
//   - Loc/Obj target: no-op (TS L1035-1037). In TS, Loc/Obj targets get
//     their initial path from MoveClick/scripts; tickloop never repaths.
//     Pre-NAI-98 goscape ran pathToTarget once per interaction for these
//     targets too (legacy `!p.repathed` gate). DEVIATION
//     NAI-98-D-LOC-OBJ-NO-OP-ALIGNED-TO-TS: aligned to TS no-op as part of
//     this fix; smoke targets are *Npc, but the gate retirement is the
//     same code path so Loc/Obj alignment is a free byproduct. If a
//     downstream Loc/Obj smoke surfaces a residual, revisit.
//   - PathingEntity + isLastOrNoWaypoint + followOp (APPLAYER3/OPPLAYER3):
//     queueWaypoint to target's followX/followZ (TS L1039-1042).
//     Player-on-player chase fast-path. Goscape's *Player has followX/Z;
//     *Npc does not (DEVIATION NAI-98-D-NPC-NO-FOLLOWXY: ports of TS
//     PathingEntity.ts:1201-1202 base behavior limited to *Player today;
//     followOp branch fires only when target is *Player anyway).
//   - !canAccess: no-op (TS L1044-1046). Goscape canAccess approximation:
//     !p.delayed && !p.protectedScriptActive() (per CanAccess doc-comment
//     in player_script.go; DEVIATION NAI-44-D-CANACCESS-NO-STUN-CHECK).
//   - NODE_CLIENT_ROUTEFINDER + intersects: queueWaypoints via
//     FindNaivePath (TS L1048-1051). Mirrors the same shortcut at
//     pathToTarget Smart/PathingEntity arm (interaction.go:638-644).
//   - PathingEntity + isLastOrNoWaypoint (no followOp, no intersects):
//     pathToTarget (TS L1052-1054).
//
// isLastOrNoWaypoint mirrors TS PathingEntity.ts:374-376 (waypointIndex <= 0).
//
// Retires the goscape divergent `!p.repathed` once-per-interaction gate
// at interaction.go:236-239 (pre-NAI-98). The `repathed` field stays
// declared + reset in SetInteraction/ClearInteraction as TS-vestigial
// (TS PathingEntity.ts:64 declares it + Player.ts:459 resets it but
// nothing reads it).
func (p *Player) pathToPathingTarget() {
	if p.target == nil {
		return
	}
	if _, ok := p.target.(pathingEntity); !ok {
		// Loc/Obj target — TS no-op.
		return
	}
	if p.isLastOrNoWaypoint() && isFollowOp(p) {
		// Player-on-player chase: queue waypoint to target's last-step coord.
		// followOp implies target is *Player (per isFollowOp at :145-151);
		// goscape's *Player has followX/followZ (player.go:104).
		if t, ok := p.target.(*Player); ok {
			p.queueWaypoint(t.followX, t.followZ)
		}
		return
	}
	if p.delayed || p.protectedScriptActive() {
		// canAccess gate (TS L1044-1046). DEVIATION
		// NAI-44-D-CANACCESS-NO-STUN-CHECK: stun/freeze unmodelled.
		return
	}
	srv := p.client.server
	if srv == nil {
		return
	}
	tx, tz, _ := p.target.Coords()
	if t, ok := p.target.(pathingEntity); ok {
		tw, tl := t.Width(), t.Length()
		if srv.cfg.NodeClientRoutefinder && coordgrid.Intersects(p.x, p.z, p.Width(), p.Length(), tx, tz, tw, tl) {
			pf := srv.pathfinder()
			if pf == nil {
				p.queueWaypoint(tx, tz)
				return
			}
			route := pf.FindNaivePath(p.level, p.x, p.z, tx, tz, p.Width(), p.Length(), tw, tl, 0, collision.TypeNormal)
			p.queueWaypoints(routeToPacked(route))
			return
		}
	}
	if p.isLastOrNoWaypoint() {
		p.pathToTarget()
	}
}

// isLastOrNoWaypoint mirrors TS PathingEntity.isLastOrNoWaypoint
// (PathingEntity.ts:374-376): true when the player has consumed all but
// the final waypoint or has none queued.
func (p *Player) isLastOrNoWaypoint() bool {
	return p.waypointIndex <= 0
}
```

Verify the `pathingEntity` interface, `coordgrid.Intersects`, `collision.TypeNormal`, and `srv.pathfinder()` symbols at HEAD before commit:

```bash
grep -n "pathingEntity interface\|func Intersects\|TypeNormal\|func .* pathfinder()" modules/world/pathing.go pkg/coordgrid/*.go pkg/pathfinder/collision/*.go modules/world/server.go
```

### Step 4.3: Replace the gate at interaction.go:236-239

Replace:

```go
		// Recalc path (TS L1228-1229).
		if !p.repathed {
			p.pathToTarget()
			p.repathed = true
		}
```

with:

```go
		// Recalc path (TS L1228-1229).
		p.pathToPathingTarget()
```

### Step 4.4: Run regression test → green

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -v ./modules/world/... -run TestProcessInteractionRepathsAfterPathExhaustion
```

Expected: PASS. Tick 2 re-queues path because `isLastOrNoWaypoint() == true`.

### Step 4.5: Update existing tests asserting `repathed` as pathing-fired signal

`modules/world/interaction_test.go` lines 184 and 560-561 use `if !p.repathed` as a proxy for "did the pathing branch fire this tick". After the fix, `repathed` is no longer set by the post-step branch (it stays at its `SetInteraction` reset value of `false`), so the assertion is invalid. Replace each with a direct `waypointIndex` assertion.

At interaction_test.go:184 (inside TestProcessInteractionOutOfRangePaths):

```go
	if !p.repathed {
		t.Error("repathed should be true after first out-of-range tick")
	}
```

→ delete. The preceding `if p.waypointIndex < 0` assertion at :181-183 already covers "pathing branch fired".

At interaction_test.go:560-561 (inside TestProcessInteractionNpcUsesAttackrange):

```go
	if !p.repathed {
		t.Error("p.repathed: got false, want true — pathing branch should fire when out of AP range")
	}
```

→ replace with:

```go
	if p.waypointIndex < 0 {
		t.Error("p.waypointIndex < 0 — pathing branch should fire when out of AP range")
	}
```

Other `p.repathed` references stay as-is (state-setup at :91, :985; reset-to-false assertions at :76, :108).

### Step 4.6: Run the full module test suite → green

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. Verify no other test relies on the old `repathed`-as-gate semantics.

### Step 4.7: Run full repo test suite → green

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

### Step 4.8: Verify Phase 1 probe still PASSES post-fix

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -v ./pkg/gamemap/... -run TestNAI98_RealCacheReachProbe
```

Expected: BOTH repros emit `H8 FIRES by elimination …` via `t.Logf` (passing). The probe operates on the BFS+StepValidator layer, not the tickloop; H6/H7 paths remain green pre/post-fix.

### Step 4.9: Commit

```bash
git add modules/world/interaction.go modules/world/interaction_h8_test.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(world): NAI-98 Phase 2 — port TS Player.pathToPathingTarget gate

Sub-H8 fix. Phase 1 probe (commit daf1e28) surfaced H8 by elimination
on both Repro A (NPC 943) and Repro B (NPC 3): BFS path internally
consistent and StepValidator-walkable; reach abandonment is at
tickloop level.

Root cause: goscape interaction.go:236-239 gated pathToTarget on
`!p.repathed` (once-per-interaction-lifecycle boolean). TS
Player.processInteraction (Player.ts:1228-1229) calls pathToPathingTarget
unconditionally each tick; pathToPathingTarget (Player.ts:1034-1055)
gates SMART repath internally on isLastOrNoWaypoint (PathingEntity.ts
:374-376: waypointIndex <= 0). TS `repathed` (PathingEntity.ts:64) is
declared but never read — vestigial.

Smoke trace fits: tick N path consumed to last waypoint cheb-1 of
target; tick N+1 target moves; pre-fix gate prevents repath →
"I can't reach that" + ClearInteraction.

Replaces gate with TS-faithful pathToPathingTarget port. Retires
behavioral coupling on `repathed`; field stays declared as TS-vestigial.

Test: modules/world/interaction_h8_test.go drives 2 ticks of
processInteraction with manual mid-interaction path-exhaustion;
asserts target stays anchored and waypointIndex re-queues.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Task 4 exit:** Phase 2 fix committed; full test suite green; Phase 1 probe still PASSES (H8 logf path remains, asserting BFS+StepValidator unchanged). Smoke gate (Task 6) pending.

---

## Task 5: Repro test cleanup

**Purpose:** Delete the empty-grid skip-pinned reproducers in `pkg/pathfinder/routefinder/`. They're documented degenerate per `empty_flagmap_degenerate_routefinder` and superseded by `pkg/gamemap/nai98_realcache_probe_test.go`.

**Files:**
- Delete: `pkg/pathfinder/routefinder/nai97_repro_test.go`

- [ ] **Step 5.1: Confirm post-fix the real-cache probe behaves as expected**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -v ./pkg/gamemap/... -run TestNAI98_RealCacheReachProbe
```

Expected post-fix output depends on the surfaced sub-H — controller specifies in plan-amendment Step 3.5. Default expectation: both `H6 FIRES` and `H7 FIRES` paths NO LONGER fire; if `H8 FIRES by elimination` was the surfaced sub-H, the test passes with a `t.Logf` reflecting the post-fix path shape (controller may amend the `t.Logf` line to a regression assertion; specifics in plan-amendment).

- [ ] **Step 5.2: Delete the empty-grid repro file**

```bash
rm pkg/pathfinder/routefinder/nai97_repro_test.go
```

- [ ] **Step 5.3: Verify package builds + tests still green**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pathfinder/...
```

Expected: PASS, with no NAI-97 reproducer reference.

- [ ] **Step 5.4: Commit**

```bash
git add -A pkg/pathfinder/routefinder/nai97_repro_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(routefinder): delete NAI-97 empty-grid repros (NAI-98 cleanup)

Empty-grid degenerate per memory empty_flagmap_degenerate_routefinder;
superseded by pkg/gamemap/nai98_realcache_probe_test.go which exercises
the real production loader + populateStaticLocsIntoZones replay.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: User-launched smoke gate

**Purpose:** Bind the close per `cascade_theory_smoke_binding`. Smoke is user-launched (per `smoke_test_server_handoff` — Claude's sandboxed server is unreachable from the Java client).

**Files:** none modified by controller; user runs server + client.

- [ ] **Step 6.1: Verify all tests green at HEAD**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 6.2: Emit smoke handoff prompt to user**

Controller posts to user (paste-ready):

> NAI-98 Phase 2 fix landed. Please launch the server with `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml` and connect with the Java client to verify:
>
> **Repro A (NPC 943 path-around-fountain):** spawn at (3221, 3218) level 0 in Lumbridge. Click NPC type 943 at (3218, 3216). Expected: character walks the path AROUND the fountain GroundDecor and contacts the NPC. No "I can't reach that". No mid-route abandonment.
>
> **Repro B (NPC 3 mid-route reach):** spawn at (3218, 3213) level 0. Click NPC type 3 at (3223, 3216). Expected: character walks the cheb=5 path and contacts the NPC. No mid-route abandonment.
>
> Reply with PASS / FAIL per Repro.

- [ ] **Step 6.3: Route the smoke result per spec §6**

- Both pass → proceed to Task 7 close.
- One pass, one fail (same shape as pre-fix on the failure) → reopen as NAI-99 with refined sub-H taxonomy (per `smoke_unchanged_means_multiple_blockers`). Do NOT close NAI-98.
- One pass, one fail (different shape) → cascade-adjacent failure; route per `smoke_surfaces_adjacent_divergences` (in-scope-stretch if ≤30 LOC, else NAI-99).
- Both fail with same shape as pre-fix → spec amendment within NAI-98 (re-run Phase 1 with new evidence to surface a fourth sub-H), or NAI-99 reopen.

---

## Task 7: Memory updates + close commit

**Purpose:** Carry-forward stale memory updates from NAI-97; record NAI-98 cascade-attribution; emit the close commit with `Closes memory:` trailer.

**Files (memory):**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/pathfinder_api_loc_aware.md`
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (if status of `nai_96_grounddecor_path_around_residual` changes)
- Modify or supersede: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_96_grounddecor_path_around_residual.md`

**Files (repo):** close commit only.

- [ ] **Step 7.1: Rename `FindPathDefault` → `FindPathPlain` in `pathfinder_api_loc_aware.md`**

Edit the memory file to update any `FindPathDefault` reference to `FindPathPlain`. Per memory body structure for reference type: lead with the rule, then file:line citations. Confirm `pkg/pathfinder/routefinder/api.go:40` is the current home of `FindPathPlain`.

- [ ] **Step 7.2: Close `nai_96_grounddecor_path_around_residual` entry**

Update the memory entry's body to mark the residual closed by NAI-98 with the smoke confirmation date (today). Update `MEMORY.md` index line to reflect the closed status (e.g. `— CLOSED by NAI-98 [smoke-confirmed YYYY-MM-DD]`).

- [ ] **Step 7.3: Append "From NAI-98" section to `nai_followups.md`**

Mirror the NAI-97 "From NAI-N" template:

```markdown
## From NAI-98

**Why:** NAI-98 was the Stage 2 fix for NAI-97's diagnosis ceiling on the
NAI-96 close-day GroundDecor reach abandonment smoke residuals.
**How to apply:** When opening a future GroundDecor / pathfinder reach
investigation, reference the closed sub-H<N> root cause and the durable
real-cache probe at `pkg/gamemap/nai98_realcache_probe_test.go`.

- **Surfaced sub-H:** sub-H<N> (per Phase 1 commit <SHA>).
- **Root cause (verbatim from Phase 2 commit body):** <controller fills>.
- **Files NAI-98 touched:** <controller fills per Phase 2>.
- **Residuals routed to NAI-99+:** none (or list, controller fills).
- **Lessons confirmed / new memory entries:** <controller fills>.
```

Controller fills the placeholders from Phase 1 commit + Phase 2 commit + smoke result.

- [ ] **Step 7.4: Save any new memory entries surfaced during NAI-98**

Per `post_task_handoff`: if NAI-98 surfaced a non-derivable lesson (e.g. predicate bit-flag mismatch class for sub-H6, tickloop-ordering invariant for sub-H8), save it as a new memory entry with the standard frontmatter. Add the entry to `MEMORY.md` index. Do not duplicate existing entries; check `MEMORY.md` first.

- [ ] **Step 7.5: Final test run**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS across the entire repo.

- [ ] **Step 7.6: Stage the close commit**

```bash
git status
git diff --stat HEAD
```

Expected: only the planned files modified (Phase 2 production change + Phase 2 regression test + Phase 1 probe + `nai97_repro_test.go` deletion). No stray changes.

- [ ] **Step 7.7: Commit the close**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-98 — GroundDecor reach Stage 2 fix (sub-H<N>)

Phase 1 narrowing test in pkg/gamemap surfaced sub-H<N>. Phase 2
applied <surgical-fix-summary>; smoke confirms Repro A (NPC 943
path-around-fountain) and Repro B (NPC 3 mid-route reach) both pass.

Closes memory: nai_96_grounddecor_path_around_residual
Closes memory: <new-entry-name-if-any>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If the close adds no new file changes (Phase 1+2+5 already committed; only memory updates left, which live outside the repo), use `--allow-empty` per the example. Otherwise stage the relevant files.

- [ ] **Step 7.8: Emit post-task handoff to user**

Per `post_task_handoff`: paste-ready resume prompt for the next task.

---

## Risks (carry-forward from spec §9)

- Phase 1 surfaces a hypothesis outside {H6, H7, H8}: §3.3 `t.Skipf` halt + escalate.
- Mid-spec plan-amendment introduces stale premises: per-task pre-flight at Task 3 Step 3.6.
- Real-cache fixture flakiness: mirror against `nai97_loc_walk_test.go`'s known-passing pattern.
- Phase 2 fix lands but smoke unchanged: §6 routing rules; NAI-99 reopen.
- Audit subagent fabrication: controller-side independent verification (Task 3 Step 3.1 re-run).
- CI red on Phase 1 commit: explicitly expected per Conventions; controller does not gate.

---

## Self-review note

This plan is intentionally PHASED. Tasks 1, 2, 5, 6, 7 are pre-authored at plan-write time. Tasks 3, 4 are placeholders that the controller fills in after Phase 1 lands per spec §11. The placeholder shape IS the plan-write deliverable for those tasks; do not flag as TBD.
