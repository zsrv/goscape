# NAI-12 CheckVis / LoS Unification — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire CheckVis (Line-of-Sight / Line-of-Walk) gating across four hunt variants (`huntPlayers`, `huntNpcs`, `huntObjs`, `huntLocs`) and the NPC-side `inApproachDistance` range check — replacing five pre-existing TODO breadcrumbs with inline calls into `s.gamemap.Pathfinder.LineValidator`.

**Architecture:** Five inline gate insertions; no new types, no new packages. Two TS argument-order quirks preserved exactly with FIDELITY comments (huntPlayers src/dest swap; `inApproachDistance` NPC-backward LoS with `FlagBlockPlayers`). One tracked DEVIATION: Go's scalar `srcSize` collapses TS's full 4-arg size signature — NAI-12 uses `srcSize=destWidth=destLength=1` uniformly, tracked as a follow-up.

**Tech Stack:** Go 1.26+, existing `pkg/pathfinder/routefinder.LineValidator` (`HasLineOfSight` / `HasLineOfWalk`), existing `pkg/pathfinder/collision` (`FlagLocProjBlocker`, `FlagBlockPlayers`, `FlagWallNorthProjBlocker`), existing `pkg/gamemap.GameMap` wrapping `*routefinder.PathFinderAPI`, existing `pkg/objtype.HuntVisOff/HuntVisLineOfSight/HuntVisLineOfWalk` constants.

**Spec:** `docs/superpowers/specs/2026-04-23-nai-12-checkvis-unified-design.md`

---

## File map

| File | Role |
|---|---|
| `modules/world/npc_hunt.go` | Replace TODO at L138 with huntPlayers gate (swapped src/dest) |
| `modules/world/npc_hunt_entities.go` | Replace TODOs at L61, L119, L183 with NPC/OBJ/SCENERY gates |
| `modules/world/npc_interaction.go` | Extend `inApproachDistance` (L482-502) with NPC-backward LoS gate |
| `modules/world/npc_hunt_test.go` | +3 tests (huntPlayers: pass, block, swap quirk) |
| `modules/world/npc_hunt_entities_test.go` | +6 tests + `withBlockingWall` helper |
| `modules/world/npc_interaction_test.go` | +4 tests (approach: pass, block, backward quirk, player-flag) |
| `.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` | Mark NAI-8 CheckVis + NAI-11 inApproachDistance as Resolved |

Task ordering: Task 1 establishes the shared helper and the huntNpcs pattern. Tasks 2–4 replicate the pattern at huntObjs / huntLocs / huntPlayers. Task 5 tackles inApproachDistance's different shape. Task 6 updates memory and closes the loop.

---

## Task 1: Shared helper + `huntNpcs` CheckVis gate

**Files:**
- Modify: `modules/world/npc_hunt_entities.go` (replace TODO at L61)
- Modify: `modules/world/npc_hunt_entities_test.go` (+helper + 2 tests)

**Context:** Spec § Scope-IN item 2 + § Testing strategy tests 4–5. Establishes the `withBlockingWall` helper (reused by Tasks 2–4). Test 5 uses `HuntVisLineOfWalk` to exercise the LoW dispatch branch; Test 4 uses `HuntVisLineOfSight`.

**FIDELITY:** NPC-as-source convention — `HasLineOfSight(n.level, n.x, n.z, other.x, other.z, 1, 1, 1, 0)`. Matches TS `ScriptIterators.ts:113-118`.

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_hunt_entities_test.go`:

```go
// withBlockingWall installs a projectile-blocker flag at (level, x, z)
// on the given Server's gamemap so the straight-line ray traversing
// that tile is blocked by HasLineOfSight/HasLineOfWalk.
//
// Pre-condition: s.gamemap has been constructed via gamemap.New(...).
func withBlockingWall(t *testing.T, s *Server, level, x, z int) {
	t.Helper()
	s.gamemap.Pathfinder.Flags.Add(x, z, level, collision.FlagLocProjBlocker)
}

func TestHuntNpcsCheckVisLineOfSightPasses(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	tIn := addNpcToServerAt(t, s, 10, 1, -1, n.x, n.z+2, n.level) // 2 tiles north, clear path

	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (LoS clear path)", len(hunted))
	}
	if hunted[0].Slot() != tIn.nid {
		t.Errorf("hunted[0]: got nid %d, want %d", hunted[0].Slot(), tIn.nid)
	}
}

func TestHuntNpcsCheckVisLineOfWalkBlocks(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addNpcToServerAt(t, s, 10, 1, -1, n.x, n.z+2, n.level) // 2 tiles north
	withBlockingWall(t, s, 0, 3094, 3107)                      // mid-tile blocker

	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfWalk}
	hunted := n.huntNpcs(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 (LoW blocked by mid-tile)", len(hunted))
	}
}
```

Imports at top of `modules/world/npc_hunt_entities_test.go`, add:

```go
import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/zone"
)
```

(Start from the existing import list at `npc_hunt_entities_test.go:3-10` and add `gamemap` + `collision`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntNpcsCheckVis' -v`

Expected: BOTH PASS (with current code they should return the NPC regardless of CheckVis because the gate is missing). Specifically:
- `TestHuntNpcsCheckVisLineOfSightPasses` — passes trivially (no block → NPC hunted, regardless of gate presence).
- `TestHuntNpcsCheckVisLineOfWalkBlocks` — **FAILS** with "hunted: got 1, want 0 (LoW blocked by mid-tile)" because no gate exists yet.

If `TestHuntNpcsCheckVisLineOfSightPasses` fails, there's a fixture error. The informative failing test for TDD is `TestHuntNpcsCheckVisLineOfWalkBlocks`.

- [ ] **Step 3: Replace the TODO with the gate**

Edit `modules/world/npc_hunt_entities.go`. Find:

```go
		if dx > n.huntRange || dz > n.huntRange {
			continue
		}
		// TODO: CheckVis gate — TS ScriptIterators.ts:113-118.
		// Deferred; see nai_followups.md.
		hunted = append(hunted, other)
```

Replace the TODO lines with:

```go
		if dx > n.huntRange || dz > n.huntRange {
			continue
		}
		// CheckVis gate — TS ScriptIterators.ts:113-118.
		// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
		if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
				n.level, n.x, n.z, other.x, other.z, 1, 1, 1, 0) {
			continue
		}
		if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
				n.level, n.x, n.z, other.x, other.z, 1, 1, 1, 0) {
			continue
		}
		hunted = append(hunted, other)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntNpcsCheckVis' -v`

Expected: both tests PASS.

- [ ] **Step 5: Run whole-package + whole-repo suites**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Expected: PASS (existing NAI-7/8/9 tests stay green because they set `CheckVis = 0` or don't set it, and `HuntVisOff` short-circuits both gate branches).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS on every package. (Per memory "Verify implementer claims with fresh independent runs" — package-scoped green can mask cross-package breakage.)

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_hunt_entities.go modules/world/npc_hunt_entities_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-12 add huntNpcs CheckVis gate + withBlockingWall helper

Replaces the ScriptIterators.ts:113-118 TODO with inline LoS/LoW gates
matching TS NPC-as-source convention. New test helper withBlockingWall
lays groundwork for the remaining hunt-variant tasks.
EOF
)"
```

---

## Task 2: `huntObjs` CheckVis gate

**Files:**
- Modify: `modules/world/npc_hunt_entities.go` (replace TODO at L119)
- Modify: `modules/world/npc_hunt_entities_test.go` (+2 tests)

**Context:** Spec § Scope-IN item 3 + § Testing strategy tests 6–7. Replicates Task 1's pattern for OBJ targets.

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_hunt_entities_test.go`:

```go
// addObjToZoneAt seeds a dynamic Obj in the Server's zone map.
// Mirrors the pattern from existing huntObjs tests; returns the *Obj
// so callers can mutate further.
func addObjToZoneAt(t *testing.T, s *Server, objType, x, z, level int) *entitypkg.Obj {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 100)}
	}
	if objType < len(s.objTypes.Configs) && s.objTypes.Configs[objType] == nil {
		s.objTypes.Configs[objType] = &objtype.ObjType{
			ConfigType: objtype.ConfigType{ID: objType},
			Category:   -1,
		}
	}
	o := &entitypkg.Obj{Type: objType, X: x, Z: z, Level: level, Count: 1, Lifecycle: entitypkg.LifecycleDespawn}
	z2 := s.zoneMap.Get(level, x, z)
	z2.Objs = append(z2.Objs, o)
	return o
}

func TestHuntObjsCheckVisLineOfSightPasses(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZoneAt(t, s, 1, n.x, n.z+2, n.level) // 2 tiles north, clear path

	hunt := &objtype.HuntType{CheckObj: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (LoS clear path)", len(hunted))
	}
}

func TestHuntObjsCheckVisLineOfSightBlocks(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addObjToZoneAt(t, s, 1, n.x, n.z+2, n.level)
	withBlockingWall(t, s, 0, 3094, 3107)

	hunt := &objtype.HuntType{CheckObj: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntObjs(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 (LoS blocked by mid-tile)", len(hunted))
	}
}
```

If `addObjToZoneAt` already exists in the test file (from NAI-9 test fixtures), reuse the existing helper instead of redefining it. Check with `grep -n "addObjToZoneAt\|func.*Obj.*ZoneMap" modules/world/npc_hunt_entities_test.go` before appending.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntObjsCheckVis' -v`

Expected: `TestHuntObjsCheckVisLineOfSightBlocks` FAILS (hunted=1 but want 0 — gate missing).

- [ ] **Step 3: Replace the TODO with the gate**

Edit `modules/world/npc_hunt_entities.go`. Find:

```go
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// TODO: CheckVis gate — TS ScriptIterators.ts:137-142.
			hunted = append(hunted, o)
```

Replace with:

```go
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// CheckVis gate — TS ScriptIterators.ts:137-142.
			// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
			if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
					n.level, n.x, n.z, o.X, o.Z, 1, 1, 1, 0) {
				continue
			}
			if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
					n.level, n.x, n.z, o.X, o.Z, 1, 1, 1, 0) {
				continue
			}
			hunted = append(hunted, o)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntObjsCheckVis' -v`
Expected: both PASS.

- [ ] **Step 5: Run whole-package + whole-repo**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_hunt_entities.go modules/world/npc_hunt_entities_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-12 add huntObjs CheckVis gate

Replaces the ScriptIterators.ts:137-142 TODO. Pattern identical to
huntNpcs from Task 1; NPC-as-source with Obj.X/Z as dest.
EOF
)"
```

---

## Task 3: `huntLocs` CheckVis gate

**Files:**
- Modify: `modules/world/npc_hunt_entities.go` (replace TODO at L183)
- Modify: `modules/world/npc_hunt_entities_test.go` (+2 tests)

**Context:** Spec § Scope-IN item 4 + § Testing strategy tests 8–9. Loc's `X`/`Z` are SW corner (TS passes the same; multi-tile locs preserved as 1×1 per TS quirk).

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_hunt_entities_test.go`:

```go
// addLocToZoneAt seeds a static Loc in the Server's zone map. Mirrors
// the pattern from existing huntLocs tests; returns the *Loc so
// callers can mutate further. Category defaults to -1.
func addLocToZoneAt(t *testing.T, s *Server, locType, x, z, level int) *entitypkg.Loc {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	if s.locTypes == nil {
		s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 100)}
	}
	if locType < len(s.locTypes.Configs) && s.locTypes.Configs[locType] == nil {
		s.locTypes.Configs[locType] = &objtype.LocType{
			ConfigType: objtype.ConfigType{ID: locType},
			Category:   -1,
		}
	}
	l := &entitypkg.Loc{X: x, Z: z, Level: level, Lifecycle: entitypkg.LifecycleForever}
	l.SetType(locType)
	z2 := s.zoneMap.Get(level, x, z)
	z2.Locs = append(z2.Locs, l)
	return l
}

func TestHuntLocsCheckVisLineOfSightPasses(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZoneAt(t, s, 1, n.x, n.z+2, n.level)

	hunt := &objtype.HuntType{CheckLoc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntLocs(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (LoS clear path)", len(hunted))
	}
}

func TestHuntLocsCheckVisLineOfSightBlocks(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addLocToZoneAt(t, s, 1, n.x, n.z+2, n.level)
	withBlockingWall(t, s, 0, 3094, 3107)

	hunt := &objtype.HuntType{CheckLoc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntLocs(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 (LoS blocked by mid-tile)", len(hunted))
	}
}
```

If `addLocToZoneAt` already exists (from NAI-9 test fixtures), reuse it. Verify with `grep -n "addLocToZoneAt" modules/world/npc_hunt_entities_test.go` before appending.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntLocsCheckVis' -v`
Expected: `TestHuntLocsCheckVisLineOfSightBlocks` FAILS.

- [ ] **Step 3: Replace the TODO with the gate**

Edit `modules/world/npc_hunt_entities.go`. Find:

```go
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// TODO: CheckVis gate — TS ScriptIterators.ts:160-165.
			hunted = append(hunted, l)
```

Replace with:

```go
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// CheckVis gate — TS ScriptIterators.ts:160-165.
			// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
			// FIDELITY: TS passes {loc.x, loc.z} (1×1), not multi-tile width/length;
			// goscape preserves that quirk.
			if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
					n.level, n.x, n.z, l.X, l.Z, 1, 1, 1, 0) {
				continue
			}
			if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
					n.level, n.x, n.z, l.X, l.Z, 1, 1, 1, 0) {
				continue
			}
			hunted = append(hunted, l)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntLocsCheckVis' -v`
Expected: both PASS.

- [ ] **Step 5: Run whole-package + whole-repo**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_hunt_entities.go modules/world/npc_hunt_entities_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-12 add huntLocs CheckVis gate

Replaces the ScriptIterators.ts:160-165 TODO. Uses {loc.x, loc.z} (SW
corner, 1×1) per TS — multi-tile locs NOT treated as multi-tile here.
Quirk preserved with inline FIDELITY comment.
EOF
)"
```

---

## Task 4: `huntPlayers` CheckVis gate + swap-quirk guard

**Files:**
- Modify: `modules/world/npc_hunt.go` (replace TODO at L138)
- Modify: `modules/world/npc_hunt_test.go` (+3 tests)

**Context:** Spec § Scope-IN item 1 + § Testing strategy tests 1–3. Only hunt variant with **player-as-source** (TS `ScriptIterators.ts:88-94`). Test 3 guards against accidental un-swap.

**FIDELITY:** `HasLineOfSight(n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0)` — player-as-source. TS comment doesn't explain why; preserve verbatim.

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_hunt_test.go`:

```go
// addPlayerToServerAt seeds s.players[slot] + indexes into s.grid.
// Returns the *Player so callers can mutate further. Slot 0 is reserved;
// use 1+.
func addPlayerToServerAt(t *testing.T, s *Server, slot, x, z, level int) *Player {
	t.Helper()
	if s.grid == nil {
		s.grid = grid.New()
	}
	p := &Player{slot: slot, x: x, z: z, level: level, faceEntity: -1, faceSquareX: -1, faceSquareZ: -1}
	s.players[slot] = p
	s.grid.AddPlayer(slot, x, z, level)
	return p
}

func TestHuntPlayersCheckVisLineOfSightPasses(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addPlayerToServerAt(t, s, 1, n.x, n.z+2, n.level)

	hunt := &objtype.HuntType{CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (LoS clear path)", len(hunted))
	}
}

func TestHuntPlayersCheckVisLineOfSightBlocks(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addPlayerToServerAt(t, s, 1, n.x, n.z+2, n.level)
	s.gamemap.Pathfinder.Flags.Add(3094, 3107, 0, collision.FlagLocProjBlocker)

	hunt := &objtype.HuntType{CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 (LoS blocked by mid-tile)", len(hunted))
	}
}

// TestHuntPlayersCheckVisArgumentOrderSwapQuirk guards the TS src/dest
// swap at ScriptIterators.ts:88-94. TS huntPlayers uses player-as-source;
// the other three variants use NPC-as-source. Test installs an asymmetric
// directional-wall flag that blocks player→NPC direction but would pass
// NPC→player — proving the Go code calls HasLineOfSight(p.x, p.z, n.x, n.z)
// (TS order) rather than the swapped NPC-as-source order.
//
// Asymmetric fixture rationale:
//   NPC at (3094, 3106), player at (3094, 3108). Ray goes 2 tiles along +Z.
//   Player→NPC direction: travelSouth — ray checks FlagWallNorth-bit when
//   entering each new tile from the north side. Place FlagWallNorthProjBlocker
//   at mid-tile (3094, 3107): the player→NPC ray entering (3094, 3107) from
//   the north is blocked.
//   NPC→player direction (the un-swap): travelNorth — ray checks
//   FlagWallSouth-bit. The FlagWallNorthProjBlocker we set is not in the
//   south-direction mask, so this un-swap direction would PASS.
//
//   If implementer reverts to NPC-as-source, the ray passes and the player
//   is hunted; this test flips red.
func TestHuntPlayersCheckVisArgumentOrderSwapQuirk(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	_ = addPlayerToServerAt(t, s, 1, n.x, n.z+2, n.level)
	// Asymmetric wall: blocks player→NPC direction only.
	s.gamemap.Pathfinder.Flags.Add(3094, 3107, 0, collision.FlagWallNorthProjBlocker)

	hunt := &objtype.HuntType{CheckVis: objtype.HuntVisLineOfSight}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Fatalf("hunted: got %d, want 0 — player-as-source LoS blocked; "+
			"if 1, the src/dest swap is reverted (bug)", len(hunted))
	}
}
```

Imports at top of `modules/world/npc_hunt_test.go` — add `gamemap`, `grid`, `collision`:

```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/script"
)
```

(Existing imports at `npc_hunt_test.go:3-8` already include `objtype` and `script`. Check whether `addPlayerToServerAt` already exists from a prior sub-spec before appending — grep `grep -n "func addPlayerToServerAt" modules/world/`. If it exists with this signature, reuse.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntPlayersCheckVis' -v`

Expected:
- `TestHuntPlayersCheckVisLineOfSightPasses` — PASSES (no block → player hunted).
- `TestHuntPlayersCheckVisLineOfSightBlocks` — **FAILS** (hunted=1, want 0).
- `TestHuntPlayersCheckVisArgumentOrderSwapQuirk` — **FAILS** (hunted=1, want 0).

- [ ] **Step 3: Replace the TODO with the gate**

Edit `modules/world/npc_hunt.go`. Find the TODO at L138:

```go
		// checkAfk (TS:935-937): filter players who've gone AFK
		// (1000-tick same-zone threshold).
		if hunt.CheckAfk && p.IsZonesAfk() {
			continue
		}
		// TODO: CheckVis gate — TS ScriptIterators.ts:88-94.
		// Deferred; see nai_followups.md.
		hunted = append(hunted, p)
```

Replace the TODO lines with:

```go
		// checkAfk (TS:935-937): filter players who've gone AFK
		// (1000-tick same-zone threshold).
		if hunt.CheckAfk && p.IsZonesAfk() {
			continue
		}
		// CheckVis gate — TS ScriptIterators.ts:88-94.
		// FIDELITY: TS huntPlayers swaps src/dest vs other three variants —
		// player-as-source (p.x, p.z) → NPC-as-dest (n.x, n.z). Preserve
		// the asymmetry verbatim. See NAI-12 spec § Architecture.
		// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
		if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
				n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0) {
			continue
		}
		if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
				n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0) {
			continue
		}
		hunted = append(hunted, p)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHuntPlayersCheckVis' -v`
Expected: all 3 PASS.

- [ ] **Step 5: Run whole-package + whole-repo**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-12 add huntPlayers CheckVis gate + swap-quirk guard

Replaces the ScriptIterators.ts:88-94 TODO. Player-as-source src/dest
order matches TS verbatim — asymmetric vs the three NPC-as-source
variants. TestHuntPlayersCheckVisArgumentOrderSwapQuirk uses a
directional FlagWallNorthProjBlocker fixture to guard against accidental
un-swap.
EOF
)"
```

---

## Task 5: `inApproachDistance` LoS gate + NPC-backward + player-flag guards

**Files:**
- Modify: `modules/world/npc_interaction.go` (extend `inApproachDistance` at L482-502)
- Modify: `modules/world/npc_interaction_test.go` (+4 tests)

**Context:** Spec § Scope-IN item 5 + § Testing strategy tests 10–13. Two quirks guarded: (a) NPC-backward LoS — TS passes target-as-source, self-as-dest; (b) `CollisionFlag.PLAYER` extraFlag — Go `collision.FlagBlockPlayers`.

**FIDELITY:** `HasLineOfSight(n.level, tx, tz, n.x, n.z, 1, 1, 1, collision.FlagBlockPlayers)` — target-as-source + player blocker. TS `PathingEntity.ts:402-405`.

**DEVIATION tracked (§ Scope OUT #7):** Go's scalar `srcSize` loses TS's 4-arg full sizes. NAI-12 uses 1,1,1 uniformly.

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_interaction_test.go`:

```go
// approachDistanceFixture builds a *Server + *Npc + target *Player
// positioned 2 tiles apart on level 0, ready to exercise inApproachDistance.
// NPC at (3094, 3106); target at (3094, 3108). s.gamemap is wired via
// gamemap.New(...). Returns everything the caller needs.
func approachDistanceFixture(t *testing.T) (*Server, *Npc, *Player) {
	t.Helper()
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	p := &Player{slot: 1, x: 3094, z: 3108, level: 0, faceEntity: -1, faceSquareX: -1, faceSquareZ: -1}
	return s, n, p
}

func TestNpcInApproachDistanceLosPasses(t *testing.T) {
	s, n, p := approachDistanceFixture(t)

	if !n.inApproachDistance(5, p) {
		t.Error("inApproachDistance: got false, want true (range ok + LoS clear)")
	}
	_ = s
}

func TestNpcInApproachDistanceLosBlocks(t *testing.T) {
	s, n, p := approachDistanceFixture(t)
	// Loc-projectile-blocker mid-tile between target (3094,3108) and NPC (3094,3106).
	s.gamemap.Pathfinder.Flags.Add(3094, 3107, 0, collision.FlagLocProjBlocker)

	if n.inApproachDistance(5, p) {
		t.Error("inApproachDistance: got true, want false (LoS blocked by mid-tile)")
	}
}

// TestNpcInApproachDistanceNpcBackwardArgsQuirk guards the TS
// target-as-source + self-as-dest ordering at PathingEntity.ts:402-405.
// Uses an asymmetric directional-wall flag that blocks target→NPC
// direction but would pass NPC→target.
//
// Fixture rationale (target north of NPC at +2 z):
//   Target→NPC direction: travelSouth — ray checks FlagWallNorth-bit when
//   entering each new tile from the north. FlagWallNorthProjBlocker at
//   mid-tile (3094, 3107) blocks this direction.
//   NPC→target direction (the un-swap): travelNorth — checks
//   FlagWallSouth-bit. FlagWallNorthProjBlocker is not in the south mask,
//   so the un-swap would pass.
//
//   If implementer reverses to self-as-source (forward LoS), the ray
//   passes and inApproachDistance returns true; this test flips red.
func TestNpcInApproachDistanceNpcBackwardArgsQuirk(t *testing.T) {
	s, n, p := approachDistanceFixture(t)
	s.gamemap.Pathfinder.Flags.Add(3094, 3107, 0, collision.FlagWallNorthProjBlocker)

	if n.inApproachDistance(5, p) {
		t.Error("inApproachDistance: got true, want false — target-as-source LoS " +
			"blocked; if true, the TS NPC-backward arg order is reverted (bug)")
	}
}

// TestNpcInApproachDistancePlayerFlagIsRespected guards the
// CollisionFlag.PLAYER extraFlag wiring at GameMap.ts:433-435. Places
// only FlagBlockPlayers at a mid-tile (no wall, no proj-blocker). The
// ray would PASS if extraFlag=0, but BLOCK if extraFlag=FlagBlockPlayers.
// Proves inApproachDistance actually passes FlagBlockPlayers through.
func TestNpcInApproachDistancePlayerFlagIsRespected(t *testing.T) {
	s, n, p := approachDistanceFixture(t)
	s.gamemap.Pathfinder.Flags.Add(3094, 3107, 0, collision.FlagBlockPlayers)

	if n.inApproachDistance(5, p) {
		t.Error("inApproachDistance: got true, want false — FlagBlockPlayers " +
			"mid-tile; if true, extraFlag=FlagBlockPlayers is not wired (bug)")
	}
}
```

Imports at top of `modules/world/npc_interaction_test.go` — add `gamemap` and `collision` (the file already has `testing`; check existing import list and only add missing):

```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	// ...existing imports
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcInApproachDistance' -v`

Expected:
- `TestNpcInApproachDistanceLosPasses` — PASSES (no block → true).
- `TestNpcInApproachDistanceLosBlocks` — **FAILS** (got true, want false).
- `TestNpcInApproachDistanceNpcBackwardArgsQuirk` — **FAILS** (got true, want false).
- `TestNpcInApproachDistancePlayerFlagIsRespected` — **FAILS** (got true, want false).

- [ ] **Step 3: Extend `inApproachDistance` with the LoS gate**

Edit `modules/world/npc_interaction.go`. Find the current function at L475-502:

```go
// inApproachDistance checks whether target is within rng tiles
// (Chebyshev, excluding same tile). Mirrors the player-side shape at
// interaction.go:148-164.
//
// DEVIATION from TS (PathingEntity.ts:392-406): no LoS gating. TS's
// isApproached walks the collision map; NAI-11 inherits player-side's
// S6l-D4 no-LoS posture. Tracked follow-up.
func (n *Npc) inApproachDistance(rng int, target entity) bool {
	if rng <= 0 {
		return false
	}
	tx, tz, tlevel := target.Coords()
	if tlevel != n.level {
		return false
	}
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > rng || dz > rng {
		return false
	}
	return !(dx == 0 && dz == 0)
}
```

Replace with:

```go
// inApproachDistance checks whether target is within rng tiles
// (Chebyshev, excluding same tile) AND within TS-style line-of-sight.
// Mirrors TS PathingEntity.inApproachDistance at
// PathingEntity.ts:392-406 (NPC branch at :402-403).
//
// NAI-12 closes the NAI-11 "no LoS gating" deferral.
//
// FIDELITY: "Los for Npcs is always calculated backwards for all Entity
// types" — source is target, dest is self. TS's isApproached
// (GameMap.ts:433-435) dispatches to hasLineOfSight with
// CollisionFlag.PLAYER as extraFlag — Go equivalent
// collision.FlagBlockPlayers.
//
// DEVIATION: TS passes target.width+target.length and this.width+this.length
// (four size args). Go's HasLineOfSight collapses src to scalar srcSize;
// NAI-12 approximates with srcSize=1, destWidth=1, destLength=1 matching
// the hunt-variant convention. Tracked as size-aware follow-up in
// nai_followups.md.
func (n *Npc) inApproachDistance(rng int, target entity) bool {
	if rng <= 0 {
		return false
	}
	tx, tz, tlevel := target.Coords()
	if tlevel != n.level {
		return false
	}
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > rng || dz > rng {
		return false
	}
	// LoS gate — TS PathingEntity.ts:402-405.
	// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
	if n.server != nil && n.server.gamemap != nil &&
		!n.server.gamemap.Pathfinder.LineValidator.HasLineOfSight(
			n.level, tx, tz, n.x, n.z, 1, 1, 1, collision.FlagBlockPlayers) {
		return false
	}
	return !(dx == 0 && dz == 0)
}
```

**Important:** the existing function doesn't take `s *Server` — it uses `n.server` (set at NPC construction). Verify this reference matches the field name in `modules/world/npc.go` with `grep -n "server" modules/world/npc.go`; adjust if the field is named differently.

Also add `collision` to the file's imports if it isn't already there:

```go
import (
	// ...existing imports
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcInApproachDistance' -v`
Expected: all 4 PASS.

- [ ] **Step 5: Run whole-package + whole-repo**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS on every package. Per memory "Verify implementer claims with fresh independent runs": if any test fails outside this task's scope, investigate — do not chalk it up to "pre-existing" without checking HEAD~1.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-12 add inApproachDistance LoS gate + backward-quirk guards

Replaces NAI-11's "no LoS gating" deferral. Mirrors TS PathingEntity.ts:
402-405 NPC branch: target-as-source, self-as-dest, with
CollisionFlag.PLAYER (Go: FlagBlockPlayers) as the extraFlag.

Two quirk-guard tests — TestNpcInApproachDistanceNpcBackwardArgsQuirk
(target-as-source swap) and TestNpcInApproachDistancePlayerFlagIsRespected
(FlagBlockPlayers wiring). A size-args deviation (Go scalar srcSize vs
TS 4 size args) is tracked as a follow-up.
EOF
)"
```

---

## Task 6: Memory updates + NAI-12 close

**Files:**
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

**Context:** Per memory "NAI series deferred follow-ups" the file is the cross-session tracker. Mark the two resolved deferrals and add the new size-aware `inApproachDistance` follow-up tracked in the spec.

**No code changes.** No tests. Just the memory file update + a final close commit in the repo documenting NAI-12's completion.

- [ ] **Step 1: Read current `nai_followups.md`**

Read the file so you know exactly what to edit:

Path: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

Locate:
- **"## From NAI-8"** section → **"### Deferred: CheckVis (LoS/LoW) gate on all four hunt variants"** subsection (around L180).
- **"## From NAI-11"** section → **"### Deferred: LoS gating in inApproachDistance"** subsection (around L307).

- [ ] **Step 2: Mark NAI-8 CheckVis deferral as Resolved**

Edit the NAI-8 subsection. Replace its first paragraph with a "Resolved 2026-04-23 (NAI-12)" marker. Keep the remediation-body intact (it's now historical record) but prepend:

```markdown
### Deferred: CheckVis (LoS/LoW) gate on all four hunt variants

**Resolved 2026-04-23 (NAI-12)** in commits for Tasks 1–4 of
`docs/superpowers/plans/2026-04-23-nai-12-checkvis-unified.md`. All four
TS ScriptIterators.ts:88-94 / 113-118 / 137-142 / 160-165 branches now
have inline LoS/LoW gates via `s.gamemap.Pathfinder.LineValidator`.
huntPlayers preserves the TS src/dest swap with a dedicated guard test.
See `docs/superpowers/specs/2026-04-23-nai-12-checkvis-unified-design.md`.

---

_Original deferral body (preserved for historical context):_

TS `HuntIterator` applies `HuntVis.LINEOFSIGHT` / `HuntVis.LINEOFWALK`
gates in each per-mode branch:
[...rest unchanged...]
```

(Use Read+Edit to locate the exact existing text and substitute. Don't rewrite the whole file.)

- [ ] **Step 3: Mark NAI-11 inApproachDistance deferral as Resolved**

Same pattern for the NAI-11 subsection. Prepend:

```markdown
### Deferred: LoS gating in inApproachDistance

**Resolved 2026-04-23 (NAI-12)** in the Task 5 commit of
`docs/superpowers/plans/2026-04-23-nai-12-checkvis-unified.md`.
`(*Npc).inApproachDistance` now calls HasLineOfSight with target-as-source
(TS NPC-backward arg order) and `collision.FlagBlockPlayers` as extraFlag,
mirroring TS PathingEntity.ts:402-405. Two quirk-guard tests:
TestNpcInApproachDistanceNpcBackwardArgsQuirk +
TestNpcInApproachDistancePlayerFlagIsRespected.

---

_Original deferral body (preserved for historical context):_

`inApproachDistance` uses Chebyshev range only. TS `isApproached` at
`PathingEntity.ts:392-406` walks the collision map via the
line-of-walk routines.
[...rest unchanged...]
```

- [ ] **Step 4: Add new follow-up entry for size-aware inApproachDistance**

Under the existing NAI-11 section, append a new subsection documenting the deviation NAI-12 introduced:

```markdown
## From NAI-12 (2026-04-23)

### Deferred: size-aware inApproachDistance LoS

NAI-12 wires LoS into `inApproachDistance` but approximates sizes with
`srcSize=destWidth=destLength=1` (matching hunt-variant convention). TS
passes real sizes at `PathingEntity.ts:403`:
`target.width, target.length, this.width, this.length`. Go's
`LineValidator.HasLineOfSight` signature collapses src to scalar
`srcSize`, so a strict port needs either:

1. A new `Width()` / `Length()` pair on the `entity` interface, OR
2. A helper `approachTargetSize(target entity) int` with type-switch
   (NPC → `typ.Size`; Player → 1; Loc → width; Obj → 1).

Impact is low because the upstream Chebyshev range check already filters
most mismatches, and multi-tile NPCs are rare in this era. Remediation
is a size-aware port in a future sub-spec — likely folded into a broader
"entity-geometry-aware LoS" pass that also upgrades the hunt variants
off the 1,1,1 approximation where TS implicitly defaults to 1s.
```

- [ ] **Step 5: Verify the memory file is well-formed**

Run: `wc -l $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
Expected: line count increased by ~25-40 lines (two Resolved markers + one new follow-up).

Also verify no mid-section corruption by searching for the original "Deferred: CheckVis" header and confirming it now starts with the "**Resolved 2026-04-23 (NAI-12)**" line.

- [ ] **Step 6: Final whole-repo suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS on every package. Confirms all 13 NAI-12 tests land green alongside the existing suite.

- [ ] **Step 7: NAI-12 close commit**

```bash
git add docs/superpowers/plans/2026-04-23-nai-12-checkvis-unified.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(nai): NAI-12 closed — CheckVis / LoS unification

All 5 sites wired: 4 hunt variants + inApproachDistance. 13 new tests
(pass + block per site + 2 argument-order quirk guards). Closes two
deferrals: NAI-8 hunt-CheckVis + NAI-11 inApproachDistance-LoS. One
new tracked deviation: size-aware inApproachDistance (future sub-spec).

Memory updated: ~/.claude/.../nai_followups.md reflects the two
resolutions + the new deviation.
EOF
)"
```

Memory file is not in-repo, but this close commit references the plan. After the commit, confirm the plan is staged and the memory file is saved out-of-repo via `git status`.

---

## Appendix: Expected diff summary

After all 6 tasks commit, the final diff should show:

- `modules/world/npc_hunt.go`: +~15 LOC (huntPlayers gate with FIDELITY comment)
- `modules/world/npc_hunt_entities.go`: +~40 LOC (3 gates × ~13 LOC each)
- `modules/world/npc_interaction.go`: +~15 LOC (inApproachDistance gate + expanded docstring)
- `modules/world/npc_hunt_test.go`: +~90 LOC (3 tests + addPlayerToServerAt helper)
- `modules/world/npc_hunt_entities_test.go`: +~130 LOC (6 tests + withBlockingWall + possibly 2 entity helpers)
- `modules/world/npc_interaction_test.go`: +~80 LOC (4 tests + approachDistanceFixture helper)

Net source: ~70 LOC added. Net test: ~300 LOC added. Reasonable ratio for strict-fidelity porting (~4× test:source).

## Appendix: Plan-test-coverage crosscheck

Per memory "Plan-test-coverage crosscheck": every one of the 13 tests from the spec § Testing strategy is coded in this plan:

| Spec test # | Plan task | Plan step |
|---|---|---|
| 1 (HuntPlayers LoS pass) | Task 4 | Step 1 |
| 2 (HuntPlayers LoS block) | Task 4 | Step 1 |
| 3 (HuntPlayers swap quirk) | Task 4 | Step 1 |
| 4 (HuntNpcs LoS pass) | Task 1 | Step 1 |
| 5 (HuntNpcs LoW block) | Task 1 | Step 1 |
| 6 (HuntObjs LoS pass) | Task 2 | Step 1 |
| 7 (HuntObjs LoS block) | Task 2 | Step 1 |
| 8 (HuntLocs LoS pass) | Task 3 | Step 1 |
| 9 (HuntLocs LoS block) | Task 3 | Step 1 |
| 10 (Approach LoS pass) | Task 5 | Step 1 |
| 11 (Approach LoS block) | Task 5 | Step 1 |
| 12 (Approach NPC-backward quirk) | Task 5 | Step 1 |
| 13 (Approach player-flag wiring) | Task 5 | Step 1 |

13 tests, all coded.
