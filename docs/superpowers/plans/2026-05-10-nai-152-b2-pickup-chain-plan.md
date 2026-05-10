# NAI-152 B2 Pickup Chain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unblock the mindrune pickup smoke surfaced after NAI-152 B1 by registering the missing OBJ_TYPE script handler and porting TS-faithful Obj reach-check into Player and Npc `inOperableDistance` methods.

**Architecture:** Three independent T-tasks. T1 ports TS `ObjOps.ts:132-134` (1-line handler + dispatch wiring). T2 ports TS `Player.ts:1110` (Obj branch using `reachedEntity || reachedObj` via `pkg/pathfinder/reach.Reached` with locShape=-2 and -1). T3 ports TS `PathingEntity.ts:389` (Obj branch using `reachedObj` only — Npc inherits the base method, not Player's override). Both reach-check fixes retire the Obj clause of `NAI-91-D-OPERABLE-CHEB-FALLBACK`.

**Tech Stack:** Go 1.26+. Packages touched: `pkg/script`, `modules/world`. Pre-existing port `pkg/pathfinder/reach.Reached` (already imported in both target files for the Loc branch). `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` for all `go` invocations.

**Reference spec:** `docs/superpowers/specs/2026-05-10-nai-152-b2-pickup-chain-design.md`.

---

## File Structure

**T1 (OBJ_TYPE handler):**
- Modify: `pkg/script/handlers_obj.go` — add `handleObjType`.
- Modify: `pkg/script/handlers.go:120-127` — wire `OpObjType: handleObjType` into the OBJ family dispatch block.
- Modify: `pkg/script/handlers_obj_test.go` — add two test cases.

**T2 (Player Obj reach):**
- Modify: `modules/world/interaction.go:606-628` — add Obj branch to `inOperableDistance`; trim NAI-91 deviation doc-comment.
- Modify: `modules/world/interaction_test.go` — add 4 test cases under a new "NAI-152 B2 Obj reach" section.

**T3 (Npc Obj reach):**
- Modify: `modules/world/npc_interaction.go:664-696` — add Obj branch to `(*Npc).inOperableDistance`; trim NAI-91 deviation doc-comment.
- Modify: `modules/world/npc_interaction_test.go` — add 3 test cases under a new "NAI-152 B2 Obj reach" section.

---

## Pre-flight Verification

Before T1: Confirm `pkg/script/handlers_obj.go` still defines `handleObjAdd`, `handleObjDel`, `handleObjCoord`, `handleObjAddAll` and `requireActiveObj`. Confirm `pkg/script/handlers.go:120-127` still contains the OBJ family block with `OpObjCoord/OpObjDel/OpObjAdd/OpObjAddAll` registered. Confirm `pkg/script/opcode.go:319` still declares `OpObjType = 3511` and L1020 has the `"OBJ_TYPE"` String() entry.

```bash
rg -n "OpObjType\b|requireActiveObj|handleObjCoord" pkg/script/
```

Expected: hits at `opcode.go:319`, `opcode.go:1020`, `handlers_obj.go:11-15`, `handlers_obj.go:147-157`, `handlers.go:120`.

Before T2: Confirm `modules/world/interaction.go:606-628` still defines `inOperableDistance(p *Player, target entity)` with the Loc branch using `reach.Reached` and the trailing `return inOperableDistanceCheb(p.x, p.z, tx, tz)`. Confirm `(*Player).Width() int` at `modules/world/player.go:595` returns 1. Confirm `reach.Reached` is already imported (transitively via the Loc branch).

```bash
rg -n "func inOperableDistance\b|func \(p \*Player\) Width\b" modules/world/
rg -n "\"github.com/zsrv/goscape/pkg/pathfinder/reach\"" modules/world/interaction.go
```

Before T3: Confirm `modules/world/npc_interaction.go:675` still defines `func (n *Npc) inOperableDistance(target entity) bool`. Confirm `reach` package imported.

```bash
rg -n "func \(n \*Npc\) inOperableDistance" modules/world/npc_interaction.go
rg -n "\"github.com/zsrv/goscape/pkg/pathfinder/reach\"" modules/world/npc_interaction.go
```

**Cross-task fixture audit (run once before any task):** Enumerate existing tests that pin same-tile-rejects semantics on Obj targets under the current Cheb path. None expected (Obj reach is rarely tested directly), but verify per `enumerate_all_sites.md`.

```bash
rg -n "inOperableDistance.*Obj\|inOperableDistance.*obj\|Obj.*inOperableDistance" modules/world/
```

Expected: no hits, or only doc-comment references — no test assertions that would regress.

---

## Task 1: OBJ_TYPE script handler

**Files:**
- Modify: `pkg/script/handlers_obj.go` (append after `handleObjCoord`, ~L158)
- Modify: `pkg/script/handlers.go` (OBJ family block at L120-127)
- Test: `pkg/script/handlers_obj_test.go` (append)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/script/handlers_obj_test.go` (after `TestHandleObjDelNilActive` at L62):

```go
// --- NAI-152 B2 T1: OBJ_TYPE handler ------------------------------------

func TestHandleObjType(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.ActiveObj = &mockActiveObj{objType: 558, x: 3200, z: 3200, level: 0}

	if err := handleObjType(s); err != nil {
		t.Fatalf("handleObjType returned error: %v", err)
	}
	got := s.PopInt()
	if got != 558 {
		t.Errorf("OBJ_TYPE: got %d, want 558 (mindrune id)", got)
	}
}

func TestHandleObjTypeNilActive(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	if err := handleObjType(s); err == nil {
		t.Errorf("OBJ_TYPE: expected error on nil ActiveObj, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleObjType$|TestHandleObjTypeNilActive' -v
```

Expected: compile error `undefined: handleObjType`.

- [ ] **Step 3: Add the handler implementation**

Append to `pkg/script/handlers_obj.go` after `handleObjCoord` (file currently ends near L157):

```go
// handleObjType (OBJ_TYPE, opcode 3511) pushes the active obj's type id.
// Mirrors TS ObjOps.ts:132-134:
//
//	[ScriptOpcode.OBJ_TYPE]: state => {
//	    state.pushInt(check(state.activeObj.type, ObjTypeValid).id);
//	},
//
// TS validates the type id via ObjTypeValid. In goscape the active obj is
// pre-validated at the wire handler (handler_opobj.go:62-70 looks up
// ObjType.Configs[objId] before constructing the obj), so the id is
// round-trip-clean. (goscape defensive guard upstream; TS re-validates here.)
func handleObjType(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_TYPE"); err != nil {
		return err
	}
	s.PushInt(s.ActiveObj.ObjType())
	return nil
}
```

- [ ] **Step 4: Register the handler in the dispatch table**

Modify `pkg/script/handlers.go:120-127` (the NAI-115 OBJ block). Add `OpObjType` line:

```go
	// NAI-115 Bundle 1+2: firemaking-cascade Obj/Inv/Server/Player ports.
	OpObjCoord:    handleObjCoord,
	OpObjDel:      handleObjDel,
	OpObjAdd:      handleObjAdd,
	OpObjAddAll:   handleObjAddAll,
	OpObjType:     handleObjType, // NAI-152 B2 T1
	OpLineOfWalk:  handleLineOfWalk,
	OpInvDropSlot: handleInvDropSlot,
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHandleObjType$|TestHandleObjTypeNilActive' -v
```

Expected: PASS for both tests.

- [ ] **Step 6: Run full pkg/script test suite (no regression)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS (no regressions).

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers.go pkg/script/handlers_obj_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-152 B2 T1 — register OBJ_TYPE handler

Port TS ObjOps.ts:132-134 — pushes the active obj's type id.
Closes the "no handler for OBJ_TYPE (opcode 3511) at pc=0" crash
in [label,pickup_obj_table] surfaced by the B1 mindrune smoke.

EOF
)"
```

---

## Task 2: Player.inOperableDistance Obj branch

**Files:**
- Modify: `modules/world/interaction.go:591-628` (`inOperableDistance` method + doc-comment)
- Test: `modules/world/interaction_test.go` (append after the NAI-91 Loc tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/interaction_test.go` (after the existing NAI-91 player-side Loc test block, around the end of the file or the next clearly-separated section):

```go
// -- NAI-152 B2 T2 Obj-target reach tests ---------------------------------
//
// Ports TS Player.ts:1110 — reachedEntity || reachedObj. Retires the Obj
// clause of NAI-91-D-OPERABLE-CHEB-FALLBACK. Same-tile pickup succeeds via
// reach.Reached's srcX==destX && srcZ==destZ early-out on the locShape=-1
// arm.

// newObjReachTestServer constructs a minimal *Server with a gamemap so
// inOperableDistance's new Obj branch can read collision flags. No
// locTypes needed — Obj targets don't dispatch via locTypeOrNil.
func newObjReachTestServer(t *testing.T) *Server {
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
	return s
}

// TestPlayer_InOperableDistance_Obj_SameTile pins the mindrune pickup
// reach-check. Pre-B2 returned false via inOperableDistanceCheb (excludes
// same-tile); post-B2 returns true via reach.Reached locShape=-1
// short-circuit (strategy.go:37). This is the B1-smoke binding case.
func TestPlayer_InOperableDistance_Obj_SameTile(t *testing.T) {
	s := newObjReachTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if !inOperableDistance(p, obj) {
		t.Fatalf("expected inOperableDistance true on same-tile Obj (mindrune pickup)")
	}
}

// TestPlayer_InOperableDistance_Obj_Adjacent pins the table-pickup case
// (player one tile away from the obj). reachedEntity (locShape=-2) enters
// ReachExclusiveRectangle which returns true for the 4 orthogonal
// neighbors of a 1×1 dest (reachRectangle1 perimeter check, all flags
// default zero).
func TestPlayer_InOperableDistance_Obj_Adjacent(t *testing.T) {
	s := newObjReachTestServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3201, 3200, 0)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3201, 3200, 0

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if !inOperableDistance(p, obj) {
		t.Fatalf("expected inOperableDistance true on adjacent (east) Obj")
	}
}

// TestPlayer_InOperableDistance_Obj_OutOfReach pins the no-reach case
// (distance > 1). Both reachedEntity and reachedObj arms return false:
// reachedEntity's reachRectangle1 perimeter check rejects non-adjacent
// src; reachedObj falls through the noStrategy switch default to false.
func TestPlayer_InOperableDistance_Obj_OutOfReach(t *testing.T) {
	s := newObjReachTestServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3210, 3200, 0)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3210, 3200, 0

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if inOperableDistance(p, obj) {
		t.Fatalf("expected inOperableDistance false at distance 10")
	}
}

// TestPlayer_InOperableDistance_Obj_CrossLevel preserves the existing
// top-level guard (target.level != p.level → false).
func TestPlayer_InOperableDistance_Obj_CrossLevel(t *testing.T) {
	s := newObjReachTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0

	obj := entitypkg.NewObj(1 /*level=1*/, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if inOperableDistance(p, obj) {
		t.Fatalf("expected inOperableDistance false on cross-level Obj")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlayer_InOperableDistance_Obj_' -v
```

Expected: `TestPlayer_InOperableDistance_Obj_SameTile` fails ("expected ... true on same-tile"). Adjacent may also fail (Cheb returns true at dx=1, but routes through Cheb; mode of failure differs). CrossLevel passes (existing guard). OutOfReach passes (Cheb rejects dx=10).

If all 4 pass before the production edit, the tests don't exercise the new code path — investigate before proceeding.

- [ ] **Step 3: Edit the production code — add the Obj branch and trim the doc-comment**

Modify `modules/world/interaction.go:591-628`. Replace the entire `inOperableDistance` function and its doc-comment.

Find this block:

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
		// goscape defensive: gamemap is always initialised by Server.Init in
		// production but may be nil in narrow unit tests that don't load map
		// data. Fall back to Chebyshev when absent so pre-NAI-91 tests that
		// don't exercise the shape-aware path continue to compile and run.
		if srv.gamemap == nil {
			return inOperableDistanceCheb(p.x, p.z, tx, tz)
		}
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
```

Replace with:

```go
// inOperableDistance reports whether p is in contact range of target.
// Mirrors TS Player.inOperableDistance (Player.ts:1099-1111):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached for shape /
//     angle / forceapproach-aware reach (NAI-91).
//   - Obj targets dispatch to reach.Reached twice — locShape=-2
//     (reachedEntity) OR locShape=-1 (reachedObj). Same-tile pickup
//     succeeds via the locShape=-1 short-circuit at strategy.go:37
//     (NAI-152 B2). 1×1 Obj invariant: NewObj sets Width=Length=1
//     unconditionally (pkg/entity/obj.go:39).
//   - PathingEntity (Player, Npc) targets fall through to
//     inOperableDistanceCheb (Chebyshev≤1, excludes same tile) pending
//     entity-shape port (DEVIATION NAI-91-D-OPERABLE-CHEB-FALLBACK).
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
		// goscape defensive: gamemap is always initialised by Server.Init in
		// production but may be nil in narrow unit tests that don't load map
		// data. Fall back to Chebyshev when absent so pre-NAI-91 tests that
		// don't exercise the shape-aware path continue to compile and run.
		if srv.gamemap == nil {
			return inOperableDistanceCheb(p.x, p.z, tx, tz)
		}
		flags := srv.gamemap.Pathfinder.Flags
		var fap int
		if cfg := srv.locTypeOrNil(loc.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
			loc.Width, loc.Length, 1, loc.Angle(), loc.Shape(), fap)
	}
	if obj, ok := target.(*entitypkg.Obj); ok {
		// TS Player.ts:1110 — reachedEntity || reachedObj. Same-tile
		// pickup relies on the locShape=-1 short-circuit; reachedEntity
		// (locShape=-2) returns false on 1×1 same-tile because
		// ReachExclusiveRectangle's Collides() detects the src/dest
		// overlap and rejects (TS rsmod has identical semantics).
		srv := p.client.server
		if srv.gamemap == nil {
			return inOperableDistanceCheb(p.x, p.z, tx, tz)
		}
		flags := srv.gamemap.Pathfinder.Flags
		if reach.Reached(flags, p.level, p.x, p.z, tx, tz,
			obj.Width, obj.Length, p.Width(), 0, -2, 0) {
			return true
		}
		return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
			obj.Width, obj.Length, p.Width(), 0, -1, 0)
	}
	return inOperableDistanceCheb(p.x, p.z, tx, tz)
}
```

- [ ] **Step 4: Run the new tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlayer_InOperableDistance_Obj_' -v
```

Expected: PASS for all 4 tests.

- [ ] **Step 5: Run full modules/world test suite (no regression)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS (no regressions). If any previously-passing test now fails, inspect — it likely pinned the same-tile Cheb-rejects semantic for an Obj target, in which case retire the assertion per `latent_bug_at_migration_boundary.md`.

- [ ] **Step 6: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-152 B2 T2 — Player Obj reach via reachedEntity || reachedObj

Port TS Player.ts:1110 Obj branch into inOperableDistance.
Same-tile pickup now succeeds via reach.Reached locShape=-1
short-circuit; adjacent pickup via locShape=-2 ReachExclusiveRectangle.
Retires the Obj clause of NAI-91-D-OPERABLE-CHEB-FALLBACK for Player.
Closes the "I can't reach that!" symptom on the B1 mindrune smoke.

EOF
)"
```

---

## Task 3: Npc.inOperableDistance Obj branch

**Files:**
- Modify: `modules/world/npc_interaction.go:664-696` (`(*Npc).inOperableDistance` method + doc-comment)
- Test: `modules/world/npc_interaction_test.go` (append)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_interaction_test.go` after the existing NAI-91 NPC-side block (after `TestNpc_InOperableDistance_WallStraightMatrix` and any other NAI-91 tests):

```go
// -- NAI-152 B2 T3 NPC Obj-target reach tests -----------------------------
//
// Ports TS PathingEntity.ts:389 (base class — Npc inherits). Single
// reach.Reached call with locShape=-1 (reachedObj), no OR-chain.
// Asymmetric with Player.ts:1110 which overrides to OR reachedEntity.

// newNpcObjReachTestServer constructs a minimal *Server with a gamemap
// (no locTypes needed for Obj targets).
func newNpcObjReachTestServer(t *testing.T) *Server {
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
	return s
}

// TestNpc_InOperableDistance_Obj_SameTile pins on-tile Obj reach for an
// NPC (size=1).
func TestNpc_InOperableDistance_Obj_SameTile(t *testing.T) {
	s := newNpcObjReachTestServer(t)
	typ := &objtype.NpcType{Size: 1}
	n := NewNpc(1, 42, 3200, 3200, 0, typ)
	n.server = s

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if !n.inOperableDistance(obj) {
		t.Fatalf("expected NPC inOperableDistance true on same-tile Obj")
	}
}

// TestNpc_InOperableDistance_Obj_Adjacent — reachedObj only (no OR-chain),
// so adjacency relies on the noStrategy default. reach.Reached(...,
// locShape=-1) falls to the default switch case (strategy.go:50-52) and
// returns false for non-same-tile coords. Pin that TS-faithful semantic.
func TestNpc_InOperableDistance_Obj_Adjacent(t *testing.T) {
	s := newNpcObjReachTestServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3201, 3200, 0)

	typ := &objtype.NpcType{Size: 1}
	n := NewNpc(1, 42, 3201, 3200, 0, typ)
	n.server = s

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if n.inOperableDistance(obj) {
		t.Fatalf("expected NPC inOperableDistance false on adjacent Obj " +
			"(TS PathingEntity.ts:389 base — reachedObj only; no Player " +
			"OR-chain)")
	}
}

// TestNpc_InOperableDistance_Obj_OutOfReach pins distance>1.
func TestNpc_InOperableDistance_Obj_OutOfReach(t *testing.T) {
	s := newNpcObjReachTestServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3210, 3200, 0)

	typ := &objtype.NpcType{Size: 1}
	n := NewNpc(1, 42, 3210, 3200, 0, typ)
	n.server = s

	obj := entitypkg.NewObj(0, 3200, 3200, entitypkg.LifecycleDespawn, 558, 1)
	if n.inOperableDistance(obj) {
		t.Fatalf("expected NPC inOperableDistance false at distance 10")
	}
}
```

**Note on adjacent test expectation:** TS `PathingEntity.inOperableDistance` for Obj targets is `reachedObj` only (`PathingEntity.ts:389`), which maps to `rsmod.reached(..., 0, -1, 0)`. goscape `reach.Reached` with `locShape=-1` resolves to `noStrategy` and short-circuits true only on same-tile (`strategy.go:37`) — adjacent returns false. This is TS-faithful per the base class. (Player.ts:1110 overrides with reachedEntity OR to handle adjacency; NPCs don't get that override.)

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpc_InOperableDistance_Obj_' -v
```

Expected: `TestNpc_InOperableDistance_Obj_SameTile` fails (Cheb rejects same-tile). Adjacent currently passes via Cheb (dx=1 returns true) — this is a behavior change the test pins to TS-faithful. So adjacent will fail too (current Cheb returns true; new code returns false → test wants false → wait, that means the test passes pre-fix and fails post-fix...). Let me re-read: `if n.inOperableDistance(obj) { t.Fatalf("expected false") }`. Pre-fix: Cheb returns true → fatal fires → FAIL. Post-fix: reachedObj returns false → no fatal → PASS. Correct — test fails pre-fix and passes post-fix. OutOfReach passes pre-fix (Cheb rejects dx=10) and post-fix.

- [ ] **Step 3: Edit the production code — add the Obj branch and trim the doc-comment**

Modify `modules/world/npc_interaction.go:664-696`. Replace the entire `(*Npc).inOperableDistance` method.

Find this block:

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
// constructing minimal *Npc without a server keep working
// (goscape defensive; production Server.Init always sets gamemap).
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
	// Chebyshev fallback (NAI-91-D-OPERABLE-CHEB-FALLBACK); shared with
	// player-side via interaction.go (same package).
	return inOperableDistanceCheb(n.x, n.z, tx, tz)
}
```

Replace with:

```go
// inOperableDistance reports whether n is in contact range of target.
// Mirrors TS PathingEntity.inOperableDistance (PathingEntity.ts:378-389):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached (shape /
//     angle / forceapproach-aware) with srcSize=n.size (NAI-91).
//   - Obj targets dispatch to reach.Reached with locShape=-1
//     (reachedObj). No OR-chain — TS base class uses reachedObj only;
//     Player.ts:1110 overrides to OR reachedEntity but Npc inherits the
//     base (NAI-152 B2 T3). Same-tile pickup succeeds via the
//     strategy.go:37 short-circuit.
//   - PathingEntity (Player, Npc) targets fall through to Chebyshev≤1
//     excluding same-tile, pending entity-shape port (DEVIATION
//     NAI-91-D-OPERABLE-CHEB-FALLBACK).
//
// Defensive: nil n.server falls through to Chebyshev so test fixtures
// constructing minimal *Npc without a server keep working
// (goscape defensive; production Server.Init always sets gamemap).
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
	if obj, ok := target.(*entitypkg.Obj); ok && n.server != nil && n.server.gamemap != nil {
		// TS PathingEntity.ts:389 (base class) — reachedObj only. Asymmetric
		// with Player.ts:1110's reachedEntity || reachedObj override; Npc
		// inherits the base. Per audit_full_method_against_ts.md +
		// ts_base_class_read_for_inherited_behavior.md.
		flags := n.server.gamemap.Pathfinder.Flags
		srcSize := n.size
		if srcSize <= 0 {
			srcSize = 1
		}
		return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
			obj.Width, obj.Length, srcSize, 0, -1, 0)
	}
	// Chebyshev fallback (NAI-91-D-OPERABLE-CHEB-FALLBACK); shared with
	// player-side via interaction.go (same package).
	return inOperableDistanceCheb(n.x, n.z, tx, tz)
}
```

- [ ] **Step 4: Run the new tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpc_InOperableDistance_Obj_' -v
```

Expected: PASS for all 3 tests.

- [ ] **Step 5: Run full modules/world test suite (no regression)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS (no regressions).

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-152 B2 T3 — Npc Obj reach via reachedObj (no OR-chain)

Port TS PathingEntity.ts:389 (base class — Npc inherits). reachedObj only,
no reachedEntity OR. Asymmetric with Player.ts:1110's override (T2);
asymmetry tracks TS exactly. Retires the Obj clause of
NAI-91-D-OPERABLE-CHEB-FALLBACK for Npc.

EOF
)"
```

---

## Task 4: Cross-task regression + race-detector sweep

**Files:** none modified.

- [ ] **Step 1: Run full test suite with race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: PASS across all packages, no race detections, no flakes.

- [ ] **Step 2: Build the binary to confirm compile cleanliness**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o "$TMPDIR/goscape" ./cmd/goscape
```

Expected: clean build, no warnings.

- [ ] **Step 3: Grep for residual deviation references**

Per `retire_deviation_grep_all_comments.md`, audit the deviation tag's references after the Obj-clause retirement:

```bash
rg -n "NAI-91-D-OPERABLE-CHEB-FALLBACK" pkg/ modules/
```

Expected: hits only at the two `inOperableDistance` methods (`modules/world/interaction.go` and `modules/world/npc_interaction.go`) and possibly the corresponding test files. Verify each remaining doc-comment now references only "PathingEntity (Player, Npc)" — no "and Obj" residue. If any comment still mentions "Obj" in the deviation context, fix it.

- [ ] **Step 4: Commit if any doc-comment trims were needed**

```bash
# Only if step 3 surfaced residual "and Obj" wording in deviation comments.
git add <files>
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world): NAI-152 B2 — trim Obj refs from NAI-91 deviation comments

Per retire_deviation_grep_all_comments.md, deviation tag now scopes only
to PathingEntity (Player, Npc) targets after T2/T3 retired the Obj clause.

EOF
)"
```

---

## Task 5: Java-client smoke gate

**Files:** none modified.

Per `smoke_test_server_handoff.md` — server runs in the user's host environment; Claude cannot launch it from the sandbox.

- [ ] **Step 1: Hand off to user with launch command**

Output to the user verbatim:

> The B2 commits are landed. Please run the server (`CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml` or whatever your local launcher is) and the Java client. Repro:
>
> 1. Log in.
> 2. Stand on (or near) the mindrune ground item (id=558). If none is present nearby, spawn one via `::give 558 1` then drop it (or use whatever spawn flow you prefer).
> 3. Right-click the ground item → "Take mindrune".
>
> Expected pass:
> - No `"no handler for OBJ_TYPE (opcode 3511)"` in server log.
> - No `"I can't reach that!"` in client chat.
> - Mindrune appears in inventory; ground item disappears.
> - Off-tile pickup (stand one tile away first, then click) produces the same result.
>
> Paste any server log lines that look unrelated-but-new — adjacent surprises route per `smoke_surfaces_adjacent_divergences.md` (≤30 LOC stretch-in here, larger to NAI-153).

- [ ] **Step 2: On smoke pass, write the close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-152 B2 — pickup chain unblocked (handler + reach)

Mindrune Java-client smoke passes: no OBJ_TYPE handler crash, no
"can't reach that" on same-tile pickup, inventory receives the
item, ground item clears. Closes NAI-152 master spec §6.3 (B2-β
with reach-check addition); retires Obj clause of
NAI-91-D-OPERABLE-CHEB-FALLBACK on both Player and Npc.

Closes memory: ts_base_class_read_for_inherited_behavior,
audit_full_method_against_ts.
EOF
)"
```

- [ ] **Step 3: Memory and follow-up sweep**

Per `post_task_handoff.md`:
- If smoke surfaced any non-obvious finding (e.g. cache-loader quirk for mindrune Op overrides, content-side template surprises), save to memory.
- If any adjacent divergence was routed to NAI-153, add a memory entry per `nai_followups.md`.
- Emit a paste-ready resume prompt for the next session (NAI-153 brainstorm or NAI-152 stretch).

---

## Self-Review (post-write)

**Spec coverage:** Each spec section maps to a task:
- Spec §5.1 T1 (OBJ_TYPE handler) → Plan Task 1 ✓
- Spec §5.2 T2 (Player Obj reach) → Plan Task 2 ✓
- Spec §5.3 T3 (Npc Obj reach) → Plan Task 3 ✓
- Spec §6 test strategy → Plan tasks 1-3 inline tests + Task 4 regression sweep ✓
- Spec §10 acceptance gate (Java smoke) → Plan Task 5 ✓
- Spec §8 deviation retirement → Plan Task 4 step 3 grep + trim ✓

**Placeholder scan:** No "TBD", "TODO", "implement later". Every step has either exact code blocks or exact shell commands with expected output.

**Type consistency:** `handleObjType(s *ScriptState) error` (T1) matches the existing OBJ family handler signature (`handleObjCoord(s *ScriptState) error`). `inOperableDistance(p *Player, target entity) bool` (T2) and `(n *Npc) inOperableDistance(target entity) bool` (T3) preserve existing signatures. `reach.Reached` parameter order matches `strategy.go:35`. `entitypkg.NewObj(level, x, z, lifecycle, typ, count)` matches `pkg/entity/obj.go:34`.

**Risk on Task 3 adjacent test framing:** The test asserts `false` for adjacent, which is a behavior change (current Cheb returns true at dx=1). This is intentional and TS-faithful per `PathingEntity.ts:389` (`reachedObj` only). If the implementer reads the spec carefully, this is clear. The plan's "Note on adjacent test expectation" block calls it out explicitly.

**Risk on `reach.Reached(..., -1, ...)` adjacency for Npc:** Returns false. NPCs picking up adjacent items will fail this check — by TS design. If a smoke surfaces that NPC AI needs adjacent-Obj reach, that's an NAI-153 candidate (TS would behave identically, so it's not a goscape bug).
