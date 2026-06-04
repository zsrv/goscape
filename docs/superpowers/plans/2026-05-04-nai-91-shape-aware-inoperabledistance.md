# NAI-91: Shape-aware inOperableDistance — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `pkg/pathfinder/reach.Reached` into both player-side and NPC-side `inOperableDistance` for Loc targets, retiring the same-tile-excluding Chebyshev predicate that's currently failing the Tutorial Island RS Guide door re-click. End with a user-launched smoke confirming door enter+exit.

**Architecture:** Replace the 4-int free function `inOperableDistance(px, pz, tx, tz)` at `interaction.go:460` with a target-aware dispatch `inOperableDistance(p *Player, target entity)` that switches on target type — `*entitypkg.Loc` → `reach.Reached(...)`, else → `inOperableDistanceCheb(...)` (the legacy predicate, renamed). Mirror on `(*Npc).inOperableDistance`. New deviation tag `NAI-91-D-OPERABLE-CHEB-FALLBACK` scopes the still-pending entity-shape and Obj-shape work.

**Tech Stack:** Go 1.26+, Go test (table-driven matrix). TDD per `superpowers:test-driven-development`.

**Spec:** `docs/superpowers/specs/2026-05-04-nai-91-shape-aware-inoperabledistance-design.md` (committed at 2581927).

---

## File Structure

**Modified:**
- `modules/world/interaction.go` — replace `inOperableDistance` body (lines 456-473) with target-aware dispatch + retain Chebyshev predicate as `inOperableDistanceCheb`. Update the single caller at line 381.
- `modules/world/interaction_test.go` — migrate the existing `TestInOperableDistance` (lines ~265-289) to test `inOperableDistanceCheb`; append the new matrix + on-tile pin tests at end of file.
- `modules/world/npc_interaction.go` — replace `(*Npc).inOperableDistance` body (lines 521-546) with target-aware dispatch; rewrite the doc-comment block at lines 525-528 to drop the (now-fixed) Loc-side claim and scope the residual under `NAI-91-D-OPERABLE-CHEB-FALLBACK`.
- `modules/world/npc_interaction_test.go` — append matrix tests at end of file (existing `TestNpcInOperableDistance` at line 724 stays green: it tests pathing-entity targets which hit the Chebyshev default arm).

**Created:** None.

---

## Task 1 — Player-side `inOperableDistance` Loc dispatch

**Files:**
- Modify: `modules/world/interaction.go` (function at lines 456-473; caller at line 381)
- Modify: `modules/world/interaction_test.go` (existing test at lines 265-289; append new tests at EOF)

This is a TDD task. Write the failing matrix first; run RED; implement; run GREEN.

- [ ] **Step 1: Re-verify HEAD shapes**

Run these greps and confirm before editing — line numbers may have drifted:

```bash
rg -n "func inOperableDistance|inOperableDistance\(" modules/world/interaction.go modules/world/interaction_test.go
rg -n "func.*Server.*\) locTypeOrNil" modules/world/world_zone.go
rg -n "Pathfinder\s+\*?\w+|Flags\s+collision\.FlagMap" pkg/gamemap/gamemap.go pkg/pathfinder/routefinder/api.go
```

Expected:
- `interaction.go:460` — `func inOperableDistance(px, pz, tx, tz int) bool`
- `interaction.go:381` — single caller `operable := inOperableDistance(p.x, p.z, tx, tz)`
- `interaction_test.go:284` — `got := inOperableDistance(0, 0, tc.dx, tc.dz)`
- `world_zone.go:98` — `func (s *Server) locTypeOrNil(id int) *objtype.LocType`
- `gamemap.go:23` — `Pathfinder *routefinder.PathFinderAPI`
- `api.go:16` — `Flags collision.FlagMap` (value, not pointer)

If any line number drifted, use the new line; if a signature drifted, **stop and report** before continuing.

- [ ] **Step 2: Migrate the existing free-function test to `inOperableDistanceCheb`**

This step is preparatory: the existing test at `interaction_test.go:265-289` calls the 4-int free function. The new dispatch replaces that signature. Migrate this test FIRST so it pins the Chebyshev fallback under its new name.

Read the full test (open `modules/world/interaction_test.go` and locate `TestInOperableDistance`). Rename the function and update the call. Replace the existing test body with:

```go
// TestInOperableDistanceCheb_PathingEntityFallback pins the Chebyshev≤1
// excluding-same-tile predicate for non-Loc targets (PathingEntity / Obj).
// Lives under NAI-91-D-OPERABLE-CHEB-FALLBACK pending entity-shape /
// reachedObj port. Renamed from TestInOperableDistance at NAI-91 T1.
func TestInOperableDistanceCheb_PathingEntityFallback(t *testing.T) {
	cases := []struct {
		dx, dz int
		want   bool
	}{
		{0, 0, false}, // same tile
		{1, 0, true},  // N/S/E/W adjacent
		{0, 1, true},
		{-1, 0, true},
		{0, -1, true},
		{1, 1, true},   // diagonal adjacent
		{-1, -1, true}, // diagonal adjacent
		{2, 0, false},  // 2 away
		{0, 2, false},
		{2, 1, false},
	}
	for _, tc := range cases {
		got := inOperableDistanceCheb(0, 0, tc.dx, tc.dz)
		if got != tc.want {
			t.Errorf("inOperableDistanceCheb(0,0,%d,%d) = %v, want %v", tc.dx, tc.dz, got, tc.want)
		}
	}
}
```

This test will FAIL TO COMPILE until Step 5 (no `inOperableDistanceCheb` symbol yet). That's expected — this is the RED phase of the migration.

- [ ] **Step 3: Append the failing matrix tests at EOF of `interaction_test.go`**

Add this block at the end of the file (read the file's last few lines first to confirm formatting). Use the existing imports — no new ones needed beyond what's already in the file (`entitypkg`, `objtype`, `script`, `coordgrid`, `gamemap`, `collision`, `zone`, `rsbuf`).

If any of `gamemap`, `collision`, `rsbuf` are NOT yet imported, add them to the import block:

```go
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/rsbuf"
```

(Verify with `rg -n "gamemap|collision|rsbuf" modules/world/interaction_test.go | head` before adding.)

Then append:

```go
// -- NAI-91 player-side shape-aware inOperableDistance tests --------------

// newInOperableTestServer builds a minimal *Server with locTypes + gamemap
// populated so inOperableDistance's Loc dispatch can resolve forceapproach
// and read collision flags. The returned LocType is the only configured one
// (ID 100); callers may set custom ForceApproach via the returned pointer.
func newInOperableTestServer(t *testing.T) (*Server, *objtype.LocType) {
	t.Helper()
	s := &Server{
		quit:           make(chan interface{}),
		log:            discardLogger(),
		scriptProvider: defaultTestProvider(),
		zoneMap:        zone.NewZoneMap(),
		locObjTracker:  newLocObjTracker(),
		rsbuf:          rsbuf.New(),
	}
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = noopBridges{}
	s.locOps = &serverLocOps{s: s}
	s.gamemap = gamemap.New(discardLogger())
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100, DebugName: "wall_test"}}
	s.locTypes.Configs[100] = lt
	return s, lt
}

// makeWallLoc constructs a 1×1 *entitypkg.Loc at (level, x, z) with the given
// shape/angle, type ID 100 (matching newInOperableTestServer's configured
// LocType). Lifecycle is Despawn — non-load-bearing for these tests.
func makeWallLoc(t *testing.T, level, x, z, shape, angle int) *entitypkg.Loc {
	t.Helper()
	return entitypkg.NewLoc(level, x, z, 1, 1, entitypkg.LifecycleDespawn, 100, shape, angle)
}

// TestPlayer_InOperableDistance_DoorTile_AllowsReClick pins the Tutorial
// Island RS Guide door re-click case (NAI-91 root symptom). Player on the
// door tile clicking the door (wall_straight, angle=west, 1×1 footprint).
// Pre-NAI-91 returned false (excluded same-tile); post-NAI-91 returns true
// because reach.ReachWall1 short-circuits srcSize==1 && srcX==destX &&
// srcZ==destZ.
func TestPlayer_InOperableDistance_DoorTile_AllowsReClick(t *testing.T) {
	s, _ := newInOperableTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3098, 3107, 0

	loc := makeWallLoc(t, 0, 3098, 3107, 0 /*wall_straight*/, 0 /*loc_west*/)

	if !inOperableDistance(p, loc) {
		t.Fatalf("expected inOperableDistance true on the door tile (NAI-91 binding)")
	}
}

// TestPlayer_InOperableDistance_WallStraightMatrix exercises the four
// wall_straight angles across on-tile, all 4 orthogonal neighbors, and 4
// diagonals. Reaches that depend on collision flags (e.g. north/south
// neighbors gated by FlagBlock*) get explicit flag wiring; the rest use
// the empty-flagmap default.
func TestPlayer_InOperableDistance_WallStraightMatrix(t *testing.T) {
	type tile struct {
		dx, dz int
		want   bool
		// preFlags is OR-applied to the player's tile (srcX, srcZ) before
		// the call. Empty for cases that don't depend on flags.
		preFlags int
	}
	type angleCase struct {
		angle int
		name  string
		tiles []tile
	}
	cases := []angleCase{
		{
			angle: 0 /*loc_west*/, name: "west",
			tiles: []tile{
				{0, 0, true, 0},   // on-tile
				{-1, 0, true, 0},  // west-adjacent (in front of wall)
				{0, 1, true, collision.FlagBlockNorth},
				{0, -1, true, collision.FlagBlockSouth},
				{1, 0, false, 0}, // east-adjacent (behind wall, no gate)
				{1, 1, false, 0}, // diagonals false
				{-1, -1, false, 0},
				{1, -1, false, 0},
				{-1, 1, false, 0},
			},
		},
		{
			angle: 1 /*loc_north*/, name: "north",
			tiles: []tile{
				{0, 0, true, 0},
				{0, 1, true, 0}, // north-adjacent
				{-1, 0, true, collision.FlagBlockWest},
				{1, 0, true, collision.FlagBlockEast},
				{0, -1, false, 0},
			},
		},
		{
			angle: 2 /*loc_east*/, name: "east",
			tiles: []tile{
				{0, 0, true, 0},
				{1, 0, true, 0}, // east-adjacent
				{0, 1, true, collision.FlagBlockNorth},
				{0, -1, true, collision.FlagBlockSouth},
				{-1, 0, false, 0},
			},
		},
		{
			angle: 3 /*loc_south*/, name: "south",
			tiles: []tile{
				{0, 0, true, 0},
				{0, -1, true, 0}, // south-adjacent
				{-1, 0, true, collision.FlagBlockWest},
				{1, 0, true, collision.FlagBlockEast},
				{0, 1, false, 0},
			},
		},
	}

	const lx, lz = 3098, 3107

	for _, ac := range cases {
		ac := ac
		t.Run(ac.name, func(t *testing.T) {
			for _, tt := range ac.tiles {
				tt := tt
				t.Run(fmt.Sprintf("dx=%+d_dz=%+d", tt.dx, tt.dz), func(t *testing.T) {
					s, _ := newInOperableTestServer(t)
					p, _ := newTestPlayer(t)
					p.client.server = s
					p.x, p.z, p.level = lx+tt.dx, lz+tt.dz, 0
					if tt.preFlags != 0 {
						s.gamemap.Pathfinder.Flags.Add(p.x, p.z, p.level, tt.preFlags)
					}
					loc := makeWallLoc(t, 0, lx, lz, 0 /*wall_straight*/, ac.angle)
					got := inOperableDistance(p, loc)
					if got != tt.want {
						t.Errorf("angle=%s tile dx=%+d dz=%+d preFlags=0x%x: got %v want %v",
							ac.name, tt.dx, tt.dz, tt.preFlags, got, tt.want)
					}
				})
			}
		})
	}
}

// TestPlayer_InOperableDistance_LevelMismatchFalse pins the level-guard
// from TS PathingEntity.ts:379-381.
func TestPlayer_InOperableDistance_LevelMismatchFalse(t *testing.T) {
	s, _ := newInOperableTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3098, 3107, 0
	loc := entitypkg.NewLoc(1 /*level=1*/, 3098, 3107, 1, 1, entitypkg.LifecycleDespawn, 100, 0, 0)
	if inOperableDistance(p, loc) {
		t.Errorf("expected false when target.level != p.level")
	}
}

// TestPlayer_InOperableDistance_NilLocTypeFallback pins forceapproach=0
// behavior when LocType lookup returns nil (out-of-range type id).
func TestPlayer_InOperableDistance_NilLocTypeFallback(t *testing.T) {
	s, _ := newInOperableTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3098, 3107, 0
	// Type id 199 is not configured in newInOperableTestServer.
	loc := entitypkg.NewLoc(0, 3098, 3107, 1, 1, entitypkg.LifecycleDespawn, 199, 0, 0)
	if !inOperableDistance(p, loc) {
		t.Errorf("on-tile reach should still resolve true with nil LocType (forceapproach=0)")
	}
}

// TestPlayer_InOperableDistance_NpcTarget_UsesCheb pins that *Npc targets
// hit the default Chebyshev arm (excludes same-tile).
func TestPlayer_InOperableDistance_NpcTarget_UsesCheb(t *testing.T) {
	s, _ := newInOperableTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 100, 100, 0

	cases := []struct {
		name   string
		tx, tz int
		want   bool
	}{
		{"same tile", 100, 100, false},
		{"adjacent", 100, 101, true},
		{"2 away", 100, 102, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			n := &Npc{x: tc.tx, z: tc.tz, level: 0}
			if got := inOperableDistance(p, n); got != tc.want {
				t.Errorf("npc target: got %v want %v", got, tc.want)
			}
		})
	}
}
```

Add `"fmt"` to the imports if not already present.

- [ ] **Step 4: Run tests to verify RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestInOperableDistanceCheb_PathingEntityFallback|TestPlayer_InOperableDistance' -count=1`

Expected: compile failure on `inOperableDistanceCheb` (undefined) AND on `inOperableDistance(p, loc)` (signature mismatch) — the impl doesn't exist yet. Confirm the failure is "undefined: inOperableDistanceCheb" and/or "too many arguments in call to inOperableDistance" — NOT some unrelated parse error. If the failure is unrelated, fix the test fixture before proceeding.

- [ ] **Step 5: Implement the new dispatch + Chebyshev predicate**

Edit `modules/world/interaction.go`. Replace the function block at lines 456-473 (the current `inOperableDistance(px, pz, tx, tz int) bool`) with this body. Verify the imports already include `pkg/pathfinder/reach` — if not, add it.

```go
// inOperableDistance reports whether p is in contact range of target.
// Mirrors TS Player.inOperableDistance (Player.ts:1099-1111):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached for shape /
//     angle / forceapproach-aware reach (NAI-91).
//   - PathingEntity (Player, Npc) and Obj targets fall through to
//     inOperableDistanceCheb (Chebyshev≤1, excludes same tile) pending
//     entity-shape / reachedObj port (DEVIATION
//     NAI-91-D-OPERABLE-CHEB-FALLBACK).
//
// target.level mismatch returns false (TS guard preserved at all arms).
//
// INVARIANT: pkg/entity/Loc.Width / Loc.Length store ABSOLUTE (un-rotated)
// dimensions — verified at modules/world/script_loc_ops.go:35-43 and
// pkg/gamemap/load.go:128. reach.Reached rotates internally via
// rotation.Rotate(locAngle, destWidth, destLength); no double-rotation.
func inOperableDistance(p *Player, target entity) bool {
	tx, tz, tlevel := target.Coords()
	if tlevel != p.level {
		return false
	}
	if loc, ok := target.(*entitypkg.Loc); ok {
		srv := p.client.server
		flags := srv.gamemap.Pathfinder.Flags
		var fap int
		if cfg := srv.locTypeOrNil(loc.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
			loc.Width, loc.Length, 1, loc.Angle(), loc.Shape(), fap)
	}
	return inOperableDistanceCheb(p.x, p.z, tx, tz)
}

// inOperableDistanceCheb is the Chebyshev≤1 predicate (excludes same tile)
// retained for PathingEntity (Player, Npc) and Obj targets pending the
// TS reachedEntity / reachedObj ports. Lives under DEVIATION
// NAI-91-D-OPERABLE-CHEB-FALLBACK.
func inOperableDistanceCheb(px, pz, tx, tz int) bool {
	dx := px - tx
	if dx < 0 {
		dx = -dx
	}
	dz := pz - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > 1 || dz > 1 {
		return false
	}
	return !(dx == 0 && dz == 0)
}
```

Now update the caller at `interaction.go:381` (inside `tryInteract`):

Before:
```go
operable := inOperableDistance(p.x, p.z, tx, tz)
```

After:
```go
operable := inOperableDistance(p, p.target)
```

The local `tx, tz, _ := p.target.Coords()` line above can stay — `inApproachDistance` at line 382 still consumes `tx, tz`.

Add the `reach` import if not yet present:

```go
	"github.com/zsrv/goscape/pkg/pathfinder/reach"
```

Verify: `rg -n '"github.com/zsrv/goscape/pkg/pathfinder/reach"' modules/world/interaction.go`. Add to the import block alphabetically.

- [ ] **Step 6: Run player-side tests — expect GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestInOperableDistanceCheb_PathingEntityFallback|TestPlayer_InOperableDistance' -count=1 -v`

Expected: all subtests PASS. If a wall_straight matrix subtest fails (e.g. `west/dx=0_dz=+1`), inspect the FlagMap behavior at `pkg/pathfinder/collision/flagmap.go:30-48` and the reach predicate at `pkg/pathfinder/reach/strategy.go:105-120` — the matrix expectations are derived from those impls, so a mismatch indicates a typo in the test fixture, not a bug in the impl.

- [ ] **Step 7: Run the full modules/world suite — expect GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`

Expected: all green. Particular attention to existing tests that called `inOperableDistance(int, int, int, int)` directly — there should be exactly one such site (the `TestInOperableDistance` we migrated in Step 2). If any other call site fails to compile, grep `rg -n "inOperableDistance\(" modules/world/` and resolve before continuing.

- [ ] **Step 8: Race build sanity check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run 'TestPlayer_InOperableDistance' -count=1`

Expected: PASS, no race output.

- [ ] **Step 9: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-91 T1 — player-side shape-aware inOperableDistance for Loc

Dispatch *entitypkg.Loc targets through pkg/pathfinder/reach.Reached
(shape / angle / forceapproach-aware); preserve Chebyshev≤1 fallback
for PathingEntity / Obj as inOperableDistanceCheb under deviation
NAI-91-D-OPERABLE-CHEB-FALLBACK. Closes the Tutorial Island RS Guide
door re-click failure bound at NAI-90 H2 reframe.

Existing TestInOperableDistance migrated to
TestInOperableDistanceCheb_PathingEntityFallback (signature swap).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — NPC-side `(*Npc).inOperableDistance` Loc dispatch

**Files:**
- Modify: `modules/world/npc_interaction.go` (function at lines 521-546)
- Modify: `modules/world/npc_interaction_test.go` (append matrix at EOF; existing `TestNpcInOperableDistance` at line 724 stays as-is — it tests pathing-entity targets).

TDD task.

- [ ] **Step 1: Re-verify HEAD shapes**

```bash
rg -n "func \(n \*Npc\) inOperableDistance" modules/world/npc_interaction.go
rg -n "size\s+int" modules/world/npc.go
rg -n "n\.server\b" modules/world/npc.go modules/world/npc_interaction.go | head
```

Expected:
- `npc_interaction.go:529` — `func (n *Npc) inOperableDistance(target entity) bool`
- `npc.go:121` — `size int` (unexported field on Npc)
- `n.server` is the established access path (e.g. `npc.go:252` `n.server.currentTick`); `n.server.gamemap.Pathfinder...` is used at `npc_interaction.go:614-615`.

If the receiver name on `inOperableDistance` differs (e.g. `(s *Npc)` instead of `(n *Npc)`), use the actual name in the codebase.

- [ ] **Step 2: Append failing matrix tests**

Read the last few lines of `modules/world/npc_interaction_test.go` to confirm formatting. Append:

```go
// -- NAI-91 NPC-side shape-aware inOperableDistance tests -----------------

// newNpcInOperableTestServer mirrors newInOperableTestServer
// (interaction_test.go) for NPC-side fixtures.
func newNpcInOperableTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		quit:           make(chan interface{}),
		log:            discardLogger(),
		scriptProvider: defaultTestProvider(),
		zoneMap:        zone.NewZoneMap(),
		locObjTracker:  newLocObjTracker(),
		rsbuf:          rsbuf.New(),
	}
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = noopBridges{}
	s.locOps = &serverLocOps{s: s}
	s.gamemap = gamemap.New(discardLogger())
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 200)}
	s.locTypes.Configs[100] = &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100, DebugName: "wall_test"}}
	return s
}

// makeNpcWallLoc constructs a 1×1 *entitypkg.Loc at (level, x, z) with the
// given shape/angle, type ID 100.
func makeNpcWallLoc(t *testing.T, level, x, z, shape, angle int) *entitypkg.Loc {
	t.Helper()
	return entitypkg.NewLoc(level, x, z, 1, 1, entitypkg.LifecycleDespawn, 100, shape, angle)
}

// TestNpc_InOperableDistance_WallStraight_OnTile pins the on-tile case
// for an NPC standing on a wall_straight loc (size=1).
func TestNpc_InOperableDistance_WallStraight_OnTile(t *testing.T) {
	s := newNpcInOperableTestServer(t)
	typ := &objtype.NpcType{Size: 1}
	n := NewNpc(1, 42, 3098, 3107, 0, typ)
	n.server = s
	loc := makeNpcWallLoc(t, 0, 3098, 3107, 0, 0)
	if !n.inOperableDistance(loc) {
		t.Fatalf("expected on-tile NPC reach to a wall_straight loc to be true (NAI-91)")
	}
}

// TestNpc_InOperableDistance_WallStraightMatrix mirrors the player-side
// matrix at four wall_straight angles, srcSize=1.
func TestNpc_InOperableDistance_WallStraightMatrix(t *testing.T) {
	type tile struct {
		dx, dz   int
		want     bool
		preFlags int
	}
	type angleCase struct {
		angle int
		name  string
		tiles []tile
	}
	cases := []angleCase{
		{angle: 0, name: "west", tiles: []tile{
			{0, 0, true, 0},
			{-1, 0, true, 0},
			{0, 1, true, collision.FlagBlockNorth},
			{0, -1, true, collision.FlagBlockSouth},
			{1, 0, false, 0},
		}},
		{angle: 1, name: "north", tiles: []tile{
			{0, 0, true, 0},
			{0, 1, true, 0},
			{-1, 0, true, collision.FlagBlockWest},
			{1, 0, true, collision.FlagBlockEast},
			{0, -1, false, 0},
		}},
		{angle: 2, name: "east", tiles: []tile{
			{0, 0, true, 0},
			{1, 0, true, 0},
			{0, 1, true, collision.FlagBlockNorth},
			{0, -1, true, collision.FlagBlockSouth},
			{-1, 0, false, 0},
		}},
		{angle: 3, name: "south", tiles: []tile{
			{0, 0, true, 0},
			{0, -1, true, 0},
			{-1, 0, true, collision.FlagBlockWest},
			{1, 0, true, collision.FlagBlockEast},
			{0, 1, false, 0},
		}},
	}
	const lx, lz = 3098, 3107
	for _, ac := range cases {
		ac := ac
		t.Run(ac.name, func(t *testing.T) {
			for _, tt := range ac.tiles {
				tt := tt
				t.Run(fmt.Sprintf("dx=%+d_dz=%+d", tt.dx, tt.dz), func(t *testing.T) {
					s := newNpcInOperableTestServer(t)
					typ := &objtype.NpcType{Size: 1}
					n := NewNpc(1, 42, lx+tt.dx, lz+tt.dz, 0, typ)
					n.server = s
					if tt.preFlags != 0 {
						s.gamemap.Pathfinder.Flags.Add(n.x, n.z, n.level, tt.preFlags)
					}
					loc := makeNpcWallLoc(t, 0, lx, lz, 0, ac.angle)
					got := n.inOperableDistance(loc)
					if got != tt.want {
						t.Errorf("angle=%s dx=%+d dz=%+d preFlags=0x%x: got %v want %v",
							ac.name, tt.dx, tt.dz, tt.preFlags, got, tt.want)
					}
				})
			}
		})
	}
}

// TestNpc_InOperableDistance_NilServer_FallsBackSafely pins the
// defensive nil-server path. Goscape historically constructs *Npc
// fixtures without a server in some unit tests; preserve safety.
func TestNpc_InOperableDistance_NilServer_FallsBackSafely(t *testing.T) {
	typ := &objtype.NpcType{Size: 1}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	// n.server is nil by default in this minimal fixture.
	target := &Npc{x: 101, z: 100, level: 0}
	if !n.inOperableDistance(target) {
		t.Errorf("nil-server pathing-entity target: expected Chebyshev fallback to succeed")
	}
}
```

Add `"fmt"` to the imports if not present.

- [ ] **Step 3: Run tests — expect RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpc_InOperableDistance_WallStraight' -count=1 -v`

Expected: failures on the on-tile and matrix on-tile cases (current impl excludes same-tile). Other cases (orthogonal neighbors, diagonals) pass coincidentally because Chebyshev≤1 covers them. The on-tile failure is the binding RED signal.

- [ ] **Step 4: Implement the new NPC-side dispatch**

Edit `modules/world/npc_interaction.go`. Replace the function block at lines 521-546 with:

```go
// inOperableDistance reports whether n is in contact range of target.
// Mirrors TS PathingEntity.inOperableDistance (PathingEntity.ts:378-389):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached (shape /
//     angle / forceapproach-aware) with srcSize=n.size (NAI-91).
//   - PathingEntity (Player, Npc) and Obj targets fall through to
//     Chebyshev≤1 excluding same-tile, pending entity-shape /
//     reachedObj port (DEVIATION NAI-91-D-OPERABLE-CHEB-FALLBACK).
//
// Defensive: nil n.server falls through to Chebyshev so test fixtures
// constructing minimal *Npc without a server keep working.
func (n *Npc) inOperableDistance(target entity) bool {
	tx, tz, tlevel := target.Coords()
	if tlevel != n.level {
		return false
	}
	if loc, ok := target.(*entitypkg.Loc); ok && n.server != nil && n.server.gamemap != nil {
		flags := n.server.gamemap.Pathfinder.Flags
		var fap int
		if cfg := n.server.locTypeOrNil(loc.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		srcSize := n.size
		if srcSize <= 0 {
			srcSize = 1
		}
		return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
			loc.Width, loc.Length, srcSize, loc.Angle(), loc.Shape(), fap)
	}
	// Chebyshev fallback (NAI-91-D-OPERABLE-CHEB-FALLBACK).
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > 1 || dz > 1 {
		return false
	}
	return !(dx == 0 && dz == 0)
}
```

Add the `reach` import if not present. Verify with `rg -n '"github.com/zsrv/goscape/pkg/pathfinder/reach"' modules/world/npc_interaction.go`.

- [ ] **Step 5: Run NPC-side tests — expect GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpc_InOperableDistance' -count=1 -v`

Expected: all subtests PASS. Existing `TestNpcInOperableDistance` (line 724) also stays GREEN (its targets are `*Npc`, hitting the Chebyshev arm).

- [ ] **Step 6: Run the full modules/world suite — expect GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`

Expected: all green.

- [ ] **Step 7: Race build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1`

Expected: all green, no race output.

- [ ] **Step 8: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-91 T2 — NPC-side shape-aware inOperableDistance for Loc

Dispatch *entitypkg.Loc targets through pkg/pathfinder/reach.Reached
with srcSize=n.size (multi-tile NPCs route through reachWallN). Preserve
Chebyshev≤1 fallback for PathingEntity / Obj under deviation
NAI-91-D-OPERABLE-CHEB-FALLBACK. Mirror of T1 player-side fix.

Defensive nil-server fallthrough preserved so minimal *Npc test
fixtures continue to work.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Doc-comment rewrite + S6l-D4 cross-grep

**Files:**
- Modify: `modules/world/npc_interaction.go` (only the doc-comment block at lines 521-528, already partially updated by T2's function-body replacement; this task confirms the surrounding narrative).

The T2 function-body Edit included a fresh doc-comment for the `inOperableDistance` method itself. T3 cleans up any **adjacent** comment lines that still claim "S6l-D4 posture" for the Loc path, and runs a cross-grep to confirm no other site mistags the shape gap.

- [ ] **Step 1: Cross-grep S6l-D4**

Run: `rg -n "S6l-D4" pkg/ modules/ cmd/`

Expected sites (per spec §6 expected-set):
- `modules/world/interaction.go:477` — `inApproachDistance` LOS deviation (LOS-related, NOT touched by NAI-91; comment stays as-is).
- `modules/world/npc_interaction.go` — should be GONE after T2's body replacement. If the rewrite preserved a "S6l-D4 posture" reference anywhere in the function's comment block or above it, this task removes it.

If grep surfaces any **other** site claiming S6l-D4 covers a *shape* gap, audit that site:
- If the claim is about LOS-in-approach: leave it.
- If the claim is about Loc-shape reach: rewrite to reference `NAI-91-D-OPERABLE-CHEB-FALLBACK` if applicable (Obj/entity-shape residual) or remove the S6l-D4 reference entirely (now-fixed Loc path).

- [ ] **Step 2: Audit npc_interaction.go's surroundings for stale references**

Read `modules/world/npc_interaction.go` lines 515-560 (the area above and below the new `inOperableDistance`). The pre-T2 comment block at lines 521-528 ("Mirrors the player-side shape at interaction.go:128-141. DEVIATION from TS ... inherits player-side's S6l-D4 posture. Tracked follow-up.") was replaced by T2's new doc-comment. **Confirm no orphan lines remain.** If T2's replace-body kept those comment lines, surgically delete them now.

The expected post-T3 shape of the comment immediately above the function:

```go
// inOperableDistance reports whether n is in contact range of target.
// Mirrors TS PathingEntity.inOperableDistance (PathingEntity.ts:378-389):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached (shape /
//     angle / forceapproach-aware) with srcSize=n.size (NAI-91).
//   - PathingEntity (Player, Npc) and Obj targets fall through to
//     Chebyshev≤1 excluding same-tile, pending entity-shape /
//     reachedObj port (DEVIATION NAI-91-D-OPERABLE-CHEB-FALLBACK).
//
// Defensive: nil n.server falls through to Chebyshev so test fixtures
// constructing minimal *Npc without a server keep working.
func (n *Npc) inOperableDistance(target entity) bool {
```

Nothing else.

- [ ] **Step 3: Build clean**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean build.

- [ ] **Step 4: Run full test suite once more**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`

Expected: all green.

- [ ] **Step 5: Confirm S6l-D4 grep stabilized**

Run again: `rg -n "S6l-D4" pkg/ modules/ cmd/`

Expected: only `modules/world/interaction.go:477` remains in source. (Plan/spec doc references in `docs/superpowers/plans/*.md` and `docs/superpowers/specs/*.md` are expected — those are historical and untouched.)

- [ ] **Step 6: Commit (only if Step 2 surgically deleted orphan lines)**

If T2's doc-comment replacement was clean and Step 2 found no orphans, **skip this commit** — there's nothing to commit. If Step 2 deleted lines:

```bash
git add modules/world/npc_interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world): NAI-91 T3 — drop stale S6l-D4 framing from npc_interaction.go

The pre-NAI-91 comment block at npc_interaction.go:521-528 grouped the
Loc-shape gap under S6l-D4's umbrella ("inherits player-side's S6l-D4
posture"). S6l-D4's actual scope is the inApproachDistance LOS
deviation at interaction.go:477; the shape gap was a separate untagged
deviation. T2 fixed the Loc path; this commit removes the residual
S6l-D4 reference from the inOperableDistance comment block.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — Smoke handoff prep

This task does no code work; it prepares the user-launched smoke. **Stop after this task and emit the smoke handoff prompt. Do not proceed to a close commit until the user attaches smoke evidence.**

- [ ] **Step 1: Final race build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1`

Expected: all green.

- [ ] **Step 2: Binary build**

Run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o $TMPDIR/goscape-nai91 ./cmd/goscape`

Expected: clean build.

- [ ] **Step 3: Emit smoke handoff prompt**

Output to user (verbatim):

```
NAI-91 T1–T3 landed. Please run the smoke per spec §7:

1. Start server with default config:
   CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run \
     -trimpath ./cmd/goscape --config.file config.yaml

2. Java client login at default Tutorial Island spawn.

3. Walk to RS Guide door (loc_3014, ~3098,3107,0); click once.
   EXPECTED: player ends on door tile (3098,3107). Same as before
   NAI-91 — this is the designed first-click.

4. From door tile (3098,3107), click door AGAIN.
   EXPECTED (NAI-91 binding): throughwalk fires; player ends at
   (3097,3107) west of wall. Pre-NAI-91 this gave "I can't reach
   that!".

5. From (3097,3107), click door once more — re-enter to confirm
   no regression on the original path.

6. (Optional) Click a Tutorial Island ladder if accessible —
   cross-shape smoke.

7. (Adjacent surface, NOT expected to work) Re-attempt Survival
   Expert NPC. If unexpectedly reachable, note in evidence;
   otherwise NAI-92 stays open unchanged.

8. Capture goscape.log for the click ticks. Attach + paste your
   click-by-click position observations.

Per cascade_theory_smoke_binding: smoke is binding. If step 4
still fails, return to brainstorm — do not patch around.
```

Stop here. Do **not** write the close commit until the user provides smoke evidence.

---

## Close commit (after user smoke)

This section runs **only after** the user attaches smoke evidence and step 4 confirms the door re-click works. Do not run prematurely.

- [ ] **Step 1: Verify smoke binding**

Read the user's pasted log + observations. Confirm:
- Door first-click: player ends on (3098,3107). ✓
- Door re-click from (3098,3107): player ends on (3097,3107). ✓ ← the NAI-91 binding
- Door re-enter from (3097,3107): player ends on (3098,3107). ✓ (regression check)

If any of these fails, **stop**: route per `cascade_theory_smoke_binding`. Do NOT close.

- [ ] **Step 2: Update memory**

Append to `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`:

```markdown
## From NAI-91

**Closes:** door_throughwalk_gap (re-framed engine-side from H2 proc/branch
divergence to actual root cause: inOperableDistance Loc-shape gap; closed
by reach.Reached dispatch on both player + npc paths).

**Untouched:** Survival Expert NPC pathing across cabin wall (different
mechanism — pathfinder shape-blindness, not reach-gate). Routes to
NAI-92 if/when smoke surfaces a fix is needed.

**New deviation:** NAI-91-D-OPERABLE-CHEB-FALLBACK — Chebyshev≤1
fallback retained for *Player / *Npc / *Obj targets in
inOperableDistance pending TS reachedEntity / reachedObj ports.

**Lessons confirmed:**
- `controller_preflight` — pre-flight grep verified line numbers,
  receiver names (n.server access path), and locTypeOrNil signature
  before plan-author dispatch; zero stale premises.
- `cascade_theory_smoke_binding` — smoke binding the door re-click fix.
- `compressed_cadence` — appropriate for ~55 LOC production fix; no
  Stage 1 instrumentation needed because diagnosis was
  static-confirmed at brainstorm-time (disasm + reach.Reached probe).
- `audit_subagent_fabrication` / `verify_implementer_claims` — N/A
  (single controller).
```

Update `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`: edit the existing `door_throughwalk_gap` line to reflect that NAI-91 closed it (per the carry-forward convention in the file).

- [ ] **Step 3: Empty close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-91 — shape-aware inOperableDistance for Loc targets

Fix landed across T1 (player) + T2 (npc) + T3 (doc-comment cleanup);
user smoke 2026-MM-DD confirmed:
- Door first-click: player at (3097,3107) → ends on (3098,3107) ✓
- Door re-click from (3098,3107) → throughwalk → (3097,3107) ✓ (NAI-91)
- Door re-enter from (3097,3107) → ends on (3098,3107) ✓

Reframes NAI-90's H2 binding (inferred proc/branch divergence) as an
engine-side reach-gate gap. The proc executes correctly per its
content; check_axis returning false at the smoke geometry was
content-correct; the actual bug was inOperableDistance excluding
same-tile, blocking branch 1/4 in tryInteract on the door re-click.

New deviation tag NAI-91-D-OPERABLE-CHEB-FALLBACK scopes the
remaining entity-shape and Obj-shape work.

Closes memory: door_throughwalk_gap (re-framed root cause + closed)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Substitute the actual smoke date in `2026-MM-DD`.

- [ ] **Step 4: Verify HEAD**

Run: `git log --oneline -5`

Expected: NAI-91 close commit at HEAD.

---

## Self-Review

**Spec coverage:**
- Spec §1 goal — wire reach.Reached into inOperableDistance: T1 (player) + T2 (NPC). ✓
- Spec §2 diagnosis — referenced in commit messages; binding test in T1 Step 3 (`TestPlayer_InOperableDistance_DoorTile_AllowsReClick`). ✓
- Spec §3 cadence (single bundle, three tasks): T1, T2, T3 + smoke handoff in T4. ✓
- Spec §4.1 player-side dispatch: T1 Step 5 implementation. ✓
- Spec §4.2 NPC-side dispatch: T2 Step 4 implementation. ✓
- Spec §4.3 width/length invariant: documented in T1 Step 5 doc-comment. ✓
- Spec §4.4 LocType lookup: T1 + T2 use `srv.locTypeOrNil(loc.Type())`; nil-safe (`fap=0` fallback). ✓
- Spec §5.1 player-side test matrix: T1 Step 3 covers wall_straight × 4 angles + on-tile pin + level-mismatch + nil-LocType + Npc-target fallback. ✓ (Wall_l + wall_diagonal coverage trimmed: spec §5.1 invited but the matrix is mechanical and `pkg/pathfinder/reach/strategy_test.go` is canonical for the underlying predicate. Documented as a YAGNI trim — if smoke surfaces a wall_l regression, NAI-91+1 can extend.)
- Spec §5.2 NPC-side test matrix: T2 Step 2. Same trim as §5.1.
- Spec §6 file map: matches T1/T2/T3 file lists. ✓
- Spec §7 smoke protocol: T4 Step 3 emits the handoff prompt. ✓
- Spec §8 deviations (new NAI-91-D-OPERABLE-CHEB-FALLBACK, S6l-D4 untouched): T1+T2 doc-comments + T3 cross-grep. ✓
- Spec §9 risks R1 (rotation), R2 (LocType lookup), R3 (NPC reachWallN), R4 (smoke regression), R5 (NPC server access path), R6 (hidden callers): all addressed in T1 step 1, T2 step 1, the smoke protocol, and the test passes. ✓
- Spec §10 out-of-scope deferrals (Obj reach, entity reach, Survival Expert NPC, pathToTarget shape-port): noted in close-commit body (Survival Expert) and via NAI-91-D-OPERABLE-CHEB-FALLBACK residual.
- Spec §11 test strategy: matrix-driven, race build at T1 step 8 + T2 step 7 + T4 step 1.

**Identified spec-vs-plan trim:** Spec §5.1 invited wall_l + wall_diagonal coverage as "smoke insurance against shape-routing typos." The plan trims this — `pkg/pathfinder/reach/strategy.go:ReachWall1` is canonical and unit-tested at `pkg/pathfinder/reach/strategy_test.go`. The matrix-driven wall_straight coverage is sufficient to pin the dispatch wiring for the user-symptom; broader shape coverage would duplicate strategy_test.go. If user prefers the wider matrix, plan-author re-extends T1 Step 3 / T2 Step 2 with the wall_l angle=west case + wall_diagonal case before dispatching.

**Placeholder scan:** No "TBD" / "TODO" / "implement later". The smoke close commit at "Close commit Step 3" has a `2026-MM-DD` date placeholder — that's expected (filled at smoke time, not pre-committed). The smoke evidence in nai_followups.md ("From NAI-91" section) is templated similarly.

**Type consistency:**
- `inOperableDistance(p *Player, target entity) bool` — used identically in T1 Step 5 (impl) and T1 Step 3 (test calls).
- `inOperableDistanceCheb(px, pz, tx, tz int) bool` — defined in T1 Step 5; used in T1 Step 5 (Player default arm) and T1 Step 2 (migrated test).
- `(n *Npc) inOperableDistance(target entity) bool` — same signature pre/post NAI-91; impl swap only. ✓
- Field references: `loc.Width`, `loc.Length`, `loc.Type()`, `loc.Angle()`, `loc.Shape()` — all match `pkg/entity/loc.go:48-58` accessors. ✓
- `srv.gamemap.Pathfinder.Flags` — verified at gamemap.go:23 + api.go:16. ✓
- `srv.locTypeOrNil(int)` — verified at world_zone.go:98. ✓
- `n.server` access path — verified at npc.go:252 + npc_interaction.go:614. ✓
- `n.size` — verified at npc.go:121. ✓
- `objtype.LocType.ForceApproach` — verified at loctype.go:50. ✓
- `collision.FlagBlockNorth`/`South`/`East`/`West` — verified at flag.go:65-68. ✓
- `loc.AngleWest=0`/`AngleNorth=1`/`AngleEast=2`/`AngleSouth=3` — verified at angle.go:4-7. ✓
- `entitypkg.NewLoc(level, x, z, w, l, lifecycle, typ, shape, angle)` — verified at loc.go:23. ✓

**Ambiguity check:** T2 Step 4's defensive `n.server == nil || n.server.gamemap == nil` fallthrough is documented inline; the matching test (`TestNpc_InOperableDistance_NilServer_FallsBackSafely`) pins the behavior. No ambiguity.
