# NAI-95 — Static-loc collision write at world init

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `populateStaticLocsIntoZones` to write static-loc collision into `FlagMap` at world init, mirroring the runtime `AddLoc` path. Closes the NAI-94 diagnosis ceiling: castle walls and other `BlockWalk` static locs were never reaching the pathfinder, leaving Lumbridge zones unallocated → BFS-from-FlagNull degenerate case → Hans `waypoint_idx=-1` in production smoke.

**Architecture:** TDD. One new test file `modules/world/static_loc_collision_test.go` boots a `Server` against the real `data/pack` cache (skip-if-absent for CI portability), calls `populateStaticLocsIntoZones`, and asserts the post-fix FlagMap state. Test fails at HEAD, passes after the ~8-LOC fix in `modules/world/server.go`. Cascade audit verifies no existing tests in `modules/world/...` break (most existing tests use `t.TempDir()` and won't see static locs at all).

**Tech Stack:** Go 1.26+. Existing helpers reused: `newTestServer` (`server_test.go:311`), `discardLogger`, `gamemap.New` / `gamemap.Init`, `objtype.LoadLocTypes`. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-95-static-loc-collision-fix-design.md`.
**Parent:** NAI-94 diagnosis at `docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md`.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `modules/world/static_loc_collision_test.go` | Create | Stage 1 probe + Stage 2 regression-guard tests against real cache. Single test function with subtests for zone allocation, FindPathPlain shape, and dynamic wall-tile pin. |
| `modules/world/server.go` | Modify (lines 318-323) | Extend `populateStaticLocsIntoZones` to call `s.gamemap.ChangeLocCollision` for each static loc whose LocType has `BlockWalk=true`. Mirrors `world_zone.go::AddLoc` pattern. |
| `modules/world/*_test.go` | Possibly modify | Cascade audit may require fixture updates if any existing test depended on absent static-loc collision. Plan-author triages at Task 4. |

---

## Task 1: Stage 1 probe test (TDD-fails-at-HEAD)

**Files:**
- Create: `modules/world/static_loc_collision_test.go`

**Background:** Static code-trace at spec time confirmed:
- `pkg/gamemap/load.go:55-57` — `loadGround` only writes collision for `level==0 && (land & 0x2 != 0)` tiles.
- `pkg/gamemap/load.go:92-134` — `loadLocs` parses static locs into `gm.staticLocs` but does NOT touch FlagMap.
- `modules/world/server.go:318-323` — `populateStaticLocsIntoZones` calls `z.AddStaticLoc(loc)` only.
- `modules/world/world_zone.go:17-22` — runtime `AddLoc` writes collision via `s.gamemap.ChangeLocCollision`.

The test asserts the **post-fix** correct state and is expected to FAIL at HEAD. The failure output documents the broken state in git history (test-run output captured in commit body).

**Hans path coords (per NAI-94 reproducer):**
- src=(3219, 3224), dst=(3219, 3222), level=0, cheb=2 straight-line.
- Hans's zone base: (3216, 3216) — since 3219>>3=402, 402<<3=3216; 3222>>3=402.
- Player's zone base: (3216, 3224) — since 3224>>3=403, 403<<3=3224.
- Both zones contain Lumbridge castle interior; their static locs (walls / pillars / wall-corners) are the test's collision-write target.

- [ ] **Step 1: Write the failing test**

Create `modules/world/static_loc_collision_test.go`:

```go
package world

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// TestNAI95_StaticLocCollision_HansArea pins NAI-95: populateStaticLocsIntoZones
// must write FlagBlockWalk into FlagMap for each static loc whose LocType has
// BlockWalk=true. Pre-NAI-95, only the runtime AddLoc path wrote collision;
// boot-time static locs (e.g., Lumbridge castle walls around Hans) were skipped.
//
// Smoke symptom (NAI-92 surfaced, NAI-94 diagnosed): player click on Hans
// produced waypoint_idx=-1 because BFS read FlagNull for unallocated zones
// in the castle interior.
//
// Test exercises the real m48_50 / l48_50 cache. Skip-if-absent keeps the
// test CI-portable; pattern mirrors pkg/objtype/loctype_realcache_test.go.
func TestNAI95_StaticLocCollision_HansArea(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}

	locTypes, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}
	s.locTypes = locTypes

	s.populateStaticLocsIntoZones()

	t.Run("ZoneAllocation_HansArea", func(t *testing.T) {
		// Hans NPC zone (covers (3216-3223, 3216-3223) at level 0).
		// Pre-fix: false (no entity collision writes; static-loc walls don't write).
		// Post-fix: true (castle walls in zone write FlagBlockWalk via ChangeLoc).
		if !s.gamemap.Pathfinder.Flags.IsZoneAllocated(3216, 3216, 0) {
			t.Errorf("zone (3216, 3216, 0) [Hans area]: expected allocated post-NAI-95; got unallocated")
		}
		// Player-spawn zone (covers (3216-3223, 3224-3231) at level 0).
		if !s.gamemap.Pathfinder.Flags.IsZoneAllocated(3216, 3224, 0) {
			t.Errorf("zone (3216, 3224, 0) [player walk-in zone]: expected allocated post-NAI-95; got unallocated")
		}
	})

	t.Run("FindPathPlain_HansCheb2", func(t *testing.T) {
		// Mirrors NAI-94's TestNAI94_AllocatedZones_PathfinderWorks/HansCheb2
		// but against the real cache's static-loc collision instead of synthetic
		// internal.BuildCollisionMap. Post-NAI-95 the production cache must
		// produce the same shape: Success=true Alternative=false single waypoint
		// at the dest tile.
		route := s.gamemap.Pathfinder.FindPathPlain(0, 3219, 3224, 3219, 3222)
		if !route.Success {
			t.Errorf("Success: got false, want true; route=%+v", route)
		}
		if route.Alternative {
			t.Errorf("Alternative: got true (moveNear fell back), want false; route=%+v", route)
		}
		if len(route.Waypoints) != 1 {
			t.Fatalf("Waypoints len: got %d, want 1; route=%+v", len(route.Waypoints), route)
		}
		w := route.Waypoints[0]
		if w.X() != 3219 || w.Z() != 3222 || w.Level() != 0 {
			t.Errorf("Waypoints[0]: got (%d, %d, %d), want (3219, 3222, 0)",
				w.X(), w.Z(), w.Level())
		}
	})

	t.Run("WallTileBlocked", func(t *testing.T) {
		// Dynamic positive pin: find the first static loc whose LocType has
		// BlockWalk=true, then assert FlagMap.Get for its tile has FlagBlockWalk
		// set. Don't hardcode a specific castle-wall coord — the cache may
		// shift across builds.
		var found bool
		for _, loc := range s.gamemap.StaticLocs() {
			if loc.Type() < 0 || loc.Type() >= len(s.locTypes.Configs) {
				continue
			}
			lt := s.locTypes.Configs[loc.Type()]
			if lt == nil || !lt.BlockWalk {
				continue
			}
			flag := s.gamemap.Pathfinder.Flags.Get(loc.X, loc.Z, loc.Level)
			if flag == collision.FlagNull {
				t.Errorf("static loc %d at (%d, %d, %d) BlockWalk=true: FlagMap returned FlagNull (zone unallocated post-NAI-95)",
					loc.Type(), loc.X, loc.Z, loc.Level)
				return
			}
			if flag&collision.FlagBlockWalk == 0 {
				t.Errorf("static loc %d at (%d, %d, %d) BlockWalk=true: flag=0x%x missing FlagBlockWalk bit (0x%x)",
					loc.Type(), loc.X, loc.Z, loc.Level, flag, collision.FlagBlockWalk)
				return
			}
			found = true
			break
		}
		if !found {
			t.Skip("no BlockWalk static loc found in cache; cannot pin positive wall-tile collision")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it FAILS at HEAD**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^TestNAI95_StaticLocCollision_HansArea$' -v ./modules/world/...`

Expected output (HEAD = pre-fix):
```
=== RUN   TestNAI95_StaticLocCollision_HansArea
=== RUN   TestNAI95_StaticLocCollision_HansArea/ZoneAllocation_HansArea
    static_loc_collision_test.go:??: zone (3216, 3216, 0) [Hans area]: expected allocated post-NAI-95; got unallocated
    static_loc_collision_test.go:??: zone (3216, 3224, 0) [player walk-in zone]: expected allocated post-NAI-95; got unallocated
=== RUN   TestNAI95_StaticLocCollision_HansArea/FindPathPlain_HansCheb2
    static_loc_collision_test.go:??: Success: ... OR Alternative: got true (moveNear fell back), want false ...
=== RUN   TestNAI95_StaticLocCollision_HansArea/WallTileBlocked
    static_loc_collision_test.go:??: static loc ... BlockWalk=true: FlagMap returned FlagNull (zone unallocated post-NAI-95)
--- FAIL: TestNAI95_StaticLocCollision_HansArea (...)
```

If the test PASSES at HEAD, the static-trace finding is wrong — STOP, do not proceed; escalate to controller.

If the test SKIPS (cache files missing), verify `data/pack/server/maps/m48_50` and `data/pack/server/loc.dat` exist on disk; if missing, escalate to controller.

- [ ] **Step 3: Commit the failing test**

```bash
git add modules/world/static_loc_collision_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-95 — pin post-fix static-loc collision shape against real cache

TDD probe for populateStaticLocsIntoZones gap. Asserts FlagMap zones
around Hans (3216, 3216, 0) and player walk-in zone (3216, 3224, 0)
are allocated, that FindPathPlain produces the NAI-94 cheb=2 reference
shape (Success=true Alternative=false 1-waypoint), and that a known
BlockWalk static loc has FlagBlockWalk bit set. Test FAILS at HEAD
because populateStaticLocsIntoZones omits the collision-write call
that runtime AddLoc performs.

Skip-if-absent gate mirrors pkg/objtype/loctype_realcache_test.go.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Apply Stage 2 fix to `populateStaticLocsIntoZones`

**Files:**
- Modify: `modules/world/server.go:318-323`

**Reference pattern:** `modules/world/world_zone.go:17-22` (runtime `AddLoc`).

- [ ] **Step 1: Edit `populateStaticLocsIntoZones`**

Find the current body at `modules/world/server.go:315-323`:

```go
// populateStaticLocsIntoZones pushes each parsed static loc from the gamemap
// into its owning Zone via Zone.AddStaticLoc. Called once at server startup,
// adjacent to the NPC-spawn pass.
func (s *Server) populateStaticLocsIntoZones() {
	for _, loc := range s.gamemap.StaticLocs() {
		z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
		z.AddStaticLoc(loc)
	}
}
```

Replace with:

```go
// populateStaticLocsIntoZones pushes each parsed static loc from the gamemap
// into its owning Zone via Zone.AddStaticLoc and writes the loc's collision
// into the FlagMap when its LocType has BlockWalk=true. Called once at
// server startup, adjacent to the NPC-spawn pass. Mirrors the runtime
// AddLoc collision-write path at world_zone.go:17-22; the boot-time path
// previously omitted the collision write, leaving zones whose only blockers
// are static locs (e.g., Lumbridge castle interior) unallocated and
// producing FlagNull tile reads in pathfinder BFS expansion.
func (s *Server) populateStaticLocsIntoZones() {
	for _, loc := range s.gamemap.StaticLocs() {
		if s.locTypes != nil {
			if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk {
				s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
					loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
			}
		}
		z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
		z.AddStaticLoc(loc)
	}
}
```

Notes:
- `s.gamemap != nil` gate from `AddLoc` is dropped here (the for-loop iterates `s.gamemap.StaticLocs()`, so non-nil is invariant).
- `s.locTypes != nil` gate kept for parity with `AddLoc`; defensive against tests that build a partial Server without locTypes loaded.
- `loc.Length, loc.Width` arg order matches `AddLoc` (length, width — verified at world_zone.go:21).
- Collision write precedes `z.AddStaticLoc(loc)` for code-shape parity with `AddLoc`. No semantic dependency between the two writes.

- [ ] **Step 2: Run the Stage 1 test, verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^TestNAI95_StaticLocCollision_HansArea$' -v ./modules/world/...`

Expected:
```
=== RUN   TestNAI95_StaticLocCollision_HansArea
=== RUN   TestNAI95_StaticLocCollision_HansArea/ZoneAllocation_HansArea
=== RUN   TestNAI95_StaticLocCollision_HansArea/FindPathPlain_HansCheb2
=== RUN   TestNAI95_StaticLocCollision_HansArea/WallTileBlocked
--- PASS: TestNAI95_StaticLocCollision_HansArea (...)
```

If any subtest fails, controller escalates: re-read the diagnosis, re-derive premises, re-grep cache contents.

- [ ] **Step 3: Commit the fix**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-95 — write static-loc collision at world init

populateStaticLocsIntoZones now mirrors the runtime AddLoc collision-
write path: for each static loc with LocType.BlockWalk=true, call
ChangeLocCollision before zone bookkeeping. Closes the NAI-94 diagnosis
ceiling — castle walls and other boot-time BlockWalk locs were never
reaching FlagMap, leaving Lumbridge zones unallocated and producing
FlagNull tile reads in pathfinder BFS expansion (Hans waypoint_idx=-1
in NAI-92 smoke).

Stage 1 test TestNAI95_StaticLocCollision_HansArea now passes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Cascade audit — run modules/world test suite

**Files:**
- Possibly modify: any `modules/world/*_test.go` that breaks post-fix.

**Why:** Per `latent_bug_at_migration_boundary` memory and spec §5: the fix may surface latent test bugs (fixtures relying on absent static-loc collision) or trigger genuine bugs in NPC wander / line-of-sight code paths.

**Pre-known scope:** Most `modules/world/*_test.go` tests use `t.TempDir()` (no real cache) and won't see static locs at all — fix is a no-op for them. Tests that load real cache or seed synthetic static locs are the candidates.

- [ ] **Step 1: Find candidate tests**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -timeout 120s ./modules/world/... 2>&1 | tee /tmp/nai95-cascade.log
```

If all pass: cascade audit clean, skip to Step 4.

If failures: continue to Step 2.

- [ ] **Step 2: Triage each failure**

For each failing test, read the test source and classify:

| Failure mode | Triage |
|---|---|
| Test seeds a synthetic static loc (`AddStaticLoc` direct call) and asserts NPC walks through that tile. Now the collision write blocks the NPC. | **(b) update fixture** — either remove the synthetic static loc, or change the NPC's path off the now-blocked tile. Document on the test why. |
| Test boots real cache and asserts pathfinding behavior that depended on absent collision. | **(a) test was a pre-existing bug** — fix the assert to match correct post-fix behavior. |
| Test surfaces something unexpected (e.g., NPC wander tile inside a now-allocated wall — content data issue). | **(c) escalate to NAI-96+** — track in `nai_followups.md`, leave failing test or `t.Skip` with NAI-N+1 reference. |

- [ ] **Step 3: Apply fixture / test fixes**

Edit each failing test per triage. Re-run after each fix:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -timeout 120s ./modules/world/...
```

Commit each cascade fix as a separate commit with message form `test(world): NAI-95 cascade — <short description>`.

- [ ] **Step 4: Verify modules/world fully green**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -timeout 120s ./modules/world/...
```

Expected: `ok  github.com/zsrv/goscape/modules/world ...` with no FAIL lines.

---

## Task 4: Cross-package verification

**Files:** None modified (verification only).

- [ ] **Step 1: Full test sweep**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -timeout 300s ./...
```

Expected: All packages PASS. If any `pkg/...` package fails, controller escalates — likely a deeper interaction not foreseen at spec time.

- [ ] **Step 2: Race detector**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 -timeout 600s ./...
```

Expected: All packages PASS, no race warnings. The fix is single-threaded init code (called once from `NewServer`), so no race surface, but verification catches any incidental issues.

- [ ] **Step 3: Build verification**

Run:
```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o /tmp/nai95-goscape ./cmd/goscape
```

Expected: clean build, binary at `/tmp/nai95-goscape`.

---

## Task 5: Smoke handoff to user

**Files:** None modified.

- [ ] **Step 1: Verify clean state on `main`**

```bash
git status
git log --oneline -5
```

Expected: working tree clean, recent commits include `test(world): NAI-95 ...`, `feat(world): NAI-95 ...`, and any cascade-fix commits.

- [ ] **Step 2: Emit smoke prompt for user**

Per `smoke_test_server_handoff` memory, controller cannot smoke directly. Output for the user:

> **NAI-95 Stage 2 fix landed.** Smoke checklist (per spec §6):
>
> 1. Launch server: `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`
> 2. Java client login → Lumbridge spawn.
> 3. **Primary smoke — Hans:** click Hans (NPC near 3219, 3222 in castle courtyard). Expected: player walks to Hans, talk-to interaction completes (chat dialogue or default action triggers). Pre-fix: player stays put / waypoint_idx=-1.
> 4. **Secondary smokes:** click around Lumbridge castle interior, walk through doorway, click Survival Expert (3103, 3095) from outside.
> 5. Paste any unexpected behavior or error output back; controller routes per `smoke_surfaces_adjacent_divergences` (≤30 LOC same-root-cause = in-scope stretch; else NAI-96+).

- [ ] **Step 3: Stop and wait for smoke result**

Do not commit a "close" commit until smoke passes. Smoke is binding (per `cascade_theory_smoke_binding` memory).

---

## Task 6: Close commit (post-smoke)

**Files:** None modified directly. May involve memory updates.

- [ ] **Step 1: Capture any new memory entries from smoke**

If smoke surfaced anything memory-worthy (per the post-task-handoff protocol), write/update memory files in `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/` and update `MEMORY.md`.

If no new memory: skip to Step 2.

- [ ] **Step 2: Final close commit**

Empty commit (no code changes; memorializes close):

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-95 — static-loc collision write at world init

Stage 2 fix to NAI-94 diagnosis. populateStaticLocsIntoZones now writes
FlagMap collision for each static loc with BlockWalk LocType, mirroring
runtime AddLoc. Smoke confirmed Hans interaction completes (NAI-92
waypoint_idx=-1 cleared); secondary smokes around Lumbridge castle
clean.

Closes memory:
  - <list any new/updated memory entries>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If smoke surfaced residuals routed to NAI-96+ per spec §6, list them in the close-commit body and `nai_followups.md`.

---

## Self-Review

**1. Spec coverage:**
- Spec §3 Stage 1 probe — Task 1 (test file with three subtests; ZoneAllocation, FindPathPlain, WallTileBlocked).
- Spec §4 Stage 2 fix — Task 2 (server.go edit + verification).
- Spec §5 Cascade audit — Task 3 (modules/world test sweep + triage matrix).
- Spec §6 Smoke handoff — Task 5 (smoke prompt + wait gate).
- Spec §9 Exit criteria — covered by Tasks 4 (full sweep + race + build) and 6 (close).
- Spec §7 Deviation register — plan-author re-verifies TS source at Task 2 implementation; if TS asymmetry found, surface at Task 6 close. Not a blocking task; spec marks it "no tracking deviation expected."

**2. Placeholder scan:** Plan contains exact file paths, full code blocks, exact commands with expected output. No "TBD" / "TODO" / "implement later" in step bodies. The wall-coord pin in Task 1 step 1 is dynamic (iterates `gm.StaticLocs()`), avoiding a hardcoded coord that could rot.

**3. Type consistency:**
- `s.gamemap.Pathfinder.Flags.IsZoneAllocated(x, z, level)` — verified at `pkg/pathfinder/collision/flagmap.go:142`.
- `s.gamemap.Pathfinder.FindPathPlain(level, srcX, srcZ, destX, destZ)` — verified at `pkg/pathfinder/routefinder/api.go:40`.
- `route.Waypoints[i].X() / Z() / Level()` — `RouteCoordinates` accessor methods (used in `pkg/pathfinder/routefinder/nai94_repro_test.go`).
- `s.gamemap.ChangeLocCollision(shape, angle, blocksRange, length, width, x, z, level, add)` — verified at `pkg/gamemap/gamemap.go:52` and matches `world_zone.go:20-21` call sites (length, width order).
- `s.locTypeOrNil(typeID)` — verified at `world_zone.go:98-100`.
- `lt.BlockWalk`, `lt.BlockRange` — `objtype.LocType` fields (used at `world_zone.go:19-21`).
- `loc.Type()`, `loc.Shape()`, `loc.Angle()`, `loc.X`, `loc.Z`, `loc.Level`, `loc.Length`, `loc.Width` — `*entity.Loc` fields/methods (used at `world_zone.go:20-21`).
- `collision.FlagBlockWalk`, `collision.FlagNull` — verified at `pkg/pathfinder/collision/flag.go`.
- `gamemap.New(logger)` returns `*GameMap` (gamemap.go:33-43).
- `objtype.LoadParams`, `LoadObjTypes`, `LoadLocTypes` — verified at `modules/world/server.go:184-235` (same call signatures).
- `discardLogger()` — used in `modules/world/login_map_test.go:16`.
- `newTestServer(t)` — `server_test.go:311`, returns `*Server` with zoneMap, scriptProvider, etc. seeded.

All identifiers cross-checked against actual definitions. No invented APIs.
