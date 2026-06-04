# NAI-89 — Loc-revert IsActive fix + NAI-88 probe retire — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the NAI-88-discovered loc-revert bug by consolidating all four `IsActive` writes into the TS-canonical `pkg/zone` Zone methods, then strip the NAI-88 probe scaffold.

**Architecture:** Bundle 1 moves IsActive writes into `pkg/zone/zone.go` (`AddStaticLoc`, `AddLoc`, `ChangeLoc`, `RemoveLoc`), removes the matching writes from `modules/world/world_zone.go`, updates the `pkg/entity/loc.go` doc-comment, and adds a regression-pin in `modules/world` that would have caught the door-revert bug at HEAD. Bundle 2 deletes the 7 NAI-88 probe-emit blocks and 1 struct doc-comment block, reverts the `locObjTracker` constructor signature to no-args, and updates 6 call sites. Bundle 3 hands smoke off to the user (server cannot be launched from the sandbox per `smoke_test_server_handoff` memory).

**Tech Stack:** Go 1.26+. `pkg/zone`, `pkg/entity`, `modules/world`. No new deps. Run all `go` invocations with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. All commits use `git commit --no-gpg-sign`.

**Spec:** [`docs/superpowers/specs/2026-05-04-nai-89-loc-revert-isactive-fix-design.md`](../specs/2026-05-04-nai-89-loc-revert-isactive-fix-design.md)

**Predecessor:** NAI-88 Stage 1, closed at HEAD `0e6d83c`. Spec commit `6170034`.

---

## File Map

**Bundle 1 — IsActive consolidation:**

| File | Change |
|---|---|
| `pkg/zone/zone.go` | Add `loc.IsActive = true` to `AddStaticLoc` (line 150), `AddLoc` (line 157), `ChangeLoc` (line 174); add `loc.IsActive = false` to `RemoveLoc` (line 188). |
| `pkg/zone/zone_test.go` | Append 5 new `TestZone…IsActive…` tests. |
| `modules/world/world_zone.go` | Delete `loc.IsActive = true` line (currently line 25 inside `Server.AddLoc`); delete `loc.IsActive = false` line (currently line 102 inside `Server.RemoveLoc`); update doc-comments to point at the new Zone-method ownership. |
| `pkg/entity/loc.go` | Update `IsActive` field doc-comment (currently line 16). |
| `modules/world/loc_turn_test.go` | Append `TestTurnLocRevertChangedStaticMapLoc` regression-pin. |

**Bundle 2 — NAI-88 probe retire:**

| File | Change |
|---|---|
| `modules/world/loc_turn.go` | Delete 2 probe blocks (lines 16-31 and 56-66). |
| `modules/world/tick.go` | Delete 2 probe blocks (around lines 483 and 493). |
| `modules/world/world_zone.go` | Delete the P4 probe block (around lines 61-79); inline the bare `armRegister` boolean. |
| `modules/world/loc_tracker.go` | Strip the struct doc-comment about probe fields (lines 23-29); drop `log` and `nodeDebug` fields; revert `newLocObjTracker` signature; delete P5/P6 emit blocks (lines 54-61 and 72-85); drop now-unused imports (`fmt`, `log/slog`, `runtime`). |
| `modules/world/server.go` | Update `newLocObjTracker(...)` call (line 167). |
| `modules/world/server_test.go` | Update `newLocObjTracker(...)` call (line 318). |
| `modules/world/loc_tracker_test.go` | Update 4 `newLocObjTracker(nil, false)` calls (lines 10, 23, 37, 51). |

**Bundle 3 — Smoke handoff:** No file changes; the controller emits a paste-ready prompt and stops.

---

## Bundle 1 — IsActive consolidation

### Task 1: Zone.AddStaticLoc — write IsActive

**Files:**
- Modify: `pkg/zone/zone.go:150-152`
- Test: `pkg/zone/zone_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append at the end of `pkg/zone/zone_test.go`:

```go
func TestAddStaticLocSetsIsActive(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	if loc.IsActive {
		t.Fatal("setup: fresh loc must default IsActive=false")
	}
	z.AddStaticLoc(loc)
	if !loc.IsActive {
		t.Error("AddStaticLoc must set loc.IsActive=true (mirrors TS Zone.addStaticLoc Zone.ts:208)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... -run TestAddStaticLocSetsIsActive -v`
Expected: FAIL — "AddStaticLoc must set loc.IsActive=true"

- [ ] **Step 3: Add the IsActive write**

Edit `pkg/zone/zone.go` `AddStaticLoc` (currently lines 150-152):

```go
func (z *Zone) AddStaticLoc(loc *entity.Loc) {
	z.Locs = append(z.Locs, loc)
	loc.IsActive = true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... -run TestAddStaticLocSetsIsActive -v`
Expected: PASS.

Also run the full pkg/zone suite to confirm no collateral breakage:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/zone/zone.go pkg/zone/zone_test.go
git commit --no-gpg-sign -m "feat(zone): NAI-89 T1 — Zone.AddStaticLoc sets IsActive=true

Mirrors TS Zone.addStaticLoc (Engine-TS/src/engine/zone/Zone.ts:208).
Static map locs loaded via populateStaticLocsIntoZones at server
start now activate at the canonical TS site instead of remaining
zero-valued (false) until first Server.AddLoc."
```

---

### Task 2: Zone.AddLoc — write IsActive; remove Server.AddLoc write

**Files:**
- Modify: `pkg/zone/zone.go:157-170`
- Modify: `modules/world/world_zone.go:13-28` (drop the `loc.IsActive = true` line and update the comment)
- Test: `pkg/zone/zone_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append at the end of `pkg/zone/zone_test.go`:

```go
func TestAddLocSetsIsActive(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	if loc.IsActive {
		t.Fatal("setup: fresh loc must default IsActive=false")
	}
	z.AddLoc(loc)
	if !loc.IsActive {
		t.Error("Zone.AddLoc must set loc.IsActive=true (mirrors TS Zone.addLoc Zone.ts:226)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... -run TestAddLocSetsIsActive -v`
Expected: FAIL — "Zone.AddLoc must set loc.IsActive=true".

- [ ] **Step 3: Add the IsActive write to Zone.AddLoc**

Edit `pkg/zone/zone.go` `AddLoc` (currently lines 157-170). Place the `IsActive` write between the dynamic-loc append and the `queueEvent` call, mirroring TS line 226:

```go
func (z *Zone) AddLoc(loc *entity.Loc) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocAddChange, func(buf *packet.Packet) {
		rsbuf.EncodeLocAddChange(buf, coord, loc.Shape(), loc.Angle(), loc.Type())
	})
	if loc.Lifecycle == entity.LifecycleDespawn {
		z.Locs = append(z.Locs, loc)
	}
	loc.IsActive = true
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}
```

- [ ] **Step 4: Verify the new test passes; verify the existing Server.AddLoc test still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... -run TestAddLocSetsIsActive -v`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestServerAddLocAddsCollisionWhenBlockwalk -v`
Expected: PASS (Server.AddLoc still passes — IsActive becomes true via the Zone.AddLoc write that happens inside Server.AddLoc).

- [ ] **Step 5: Remove the now-redundant write in Server.AddLoc**

Edit `modules/world/world_zone.go` `Server.AddLoc` (currently lines 16-28). Delete the `loc.IsActive = true` line (line 25) and update the doc-comment to point at the Zone-method ownership:

```go
// AddLoc routes a loc spawn through the world's zone map. Wires
// collision flags via gamemap.ChangeLocCollision when the loc's
// LocType has BlockWalk=true. Mirrors TS World.addLoc
// (Engine-TS/src/engine/World.ts:1337-1348).
//
// IsActive=true is written by the called Zone.AddLoc (pkg/zone/zone.go),
// matching TS Zone.addLoc (Zone.ts:226). duration > 0 schedules
// a despawn-revert via NonPathing.SetLifeCycle, which Registers the
// loc in s.locObjTracker for per-tick processing.
func (s *Server) AddLoc(loc *entitypkg.Loc, duration int) {
	if s.gamemap != nil && s.locTypes != nil {
		if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
		}
	}
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AddLoc(loc)
	s.TrackZone(z)
	loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
}
```

- [ ] **Step 6: Verify all relevant tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... ./modules/world/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/zone/zone.go pkg/zone/zone_test.go modules/world/world_zone.go
git commit --no-gpg-sign -m "feat(zone): NAI-89 T2 — move Zone.AddLoc IsActive write to TS-canonical site

Adds loc.IsActive=true inside Zone.AddLoc (pkg/zone/zone.go),
mirroring TS Zone.addLoc (Engine-TS/src/engine/zone/Zone.ts:226),
and removes the redundant write that lived in Server.AddLoc
(modules/world/world_zone.go:25). Observable behavior unchanged:
the existing Server.AddLoc integration test still passes."
```

---

### Task 3: Zone.ChangeLoc — write IsActive (covers inactive→active force-flip)

**Files:**
- Modify: `pkg/zone/zone.go:174-184`
- Test: `pkg/zone/zone_test.go` (append)

- [ ] **Step 1: Write the failing tests**

Append at the end of `pkg/zone/zone_test.go`:

```go
func TestChangeLocSetsIsActiveWhenInactive(t *testing.T) {
	// Pins TS Zone.ts:231 comment: "If a loc is inactive, it should be
	// set to active when we call a change". This is the smoking-gun
	// branch for NAI-88's door-revert bug — a static map loc, never
	// touched by Server.AddLoc, has IsActive=false at script-time
	// change_loc; without this write the revert tick mis-dispatches.
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	if loc.IsActive {
		t.Fatal("setup: fresh loc must default IsActive=false")
	}
	z.ChangeLoc(loc)
	if !loc.IsActive {
		t.Error("Zone.ChangeLoc on inactive loc must set IsActive=true (mirrors TS Zone.changeLoc Zone.ts:232)")
	}
}

func TestChangeLocPreservesActive(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleRespawn, 100, 0, 0)
	loc.IsActive = true
	z.ChangeLoc(loc)
	if !loc.IsActive {
		t.Error("Zone.ChangeLoc on already-active loc must keep IsActive=true")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... -run "TestChangeLocSetsIsActiveWhenInactive|TestChangeLocPreservesActive" -v`
Expected: `TestChangeLocSetsIsActiveWhenInactive` FAILs ("must set IsActive=true"); `TestChangeLocPreservesActive` PASSes (already-active stays active because nothing touches the field).

- [ ] **Step 3: Add the IsActive write to Zone.ChangeLoc**

Edit `pkg/zone/zone.go` `ChangeLoc` (currently lines 174-184). Place the write at the top, mirroring TS line 232:

```go
func (z *Zone) ChangeLoc(loc *entity.Loc) {
	loc.IsActive = true
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocAddChange, func(buf *packet.Packet) {
		rsbuf.EncodeLocAddChange(buf, coord, loc.Shape(), loc.Angle(), loc.Type())
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}
```

- [ ] **Step 4: Run tests to verify both pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... -run "TestChangeLocSetsIsActiveWhenInactive|TestChangeLocPreservesActive" -v`
Expected: both PASS.

Run the full pkg/zone suite plus modules/world for collateral checks:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... ./modules/world/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/zone/zone.go pkg/zone/zone_test.go
git commit --no-gpg-sign -m "feat(zone): NAI-89 T3 — Zone.ChangeLoc forces IsActive=true

Mirrors TS Zone.changeLoc (Engine-TS/src/engine/zone/Zone.ts:230-232,
including the line 231 comment 'If a loc is inactive, it should be set
to active when we call a change'). This is the smoking-gun missing
write for NAI-88's door-revert bug — a static map loc whose IsActive
defaulted to false through Zone.AddStaticLoc and stayed false through
script-driven change_loc, mis-dispatching the revert tick to AddLoc."
```

---

### Task 4: Zone.RemoveLoc — write IsActive=false; remove Server.RemoveLoc write

**Files:**
- Modify: `pkg/zone/zone.go:188-207`
- Modify: `modules/world/world_zone.go:87-109` (drop the `loc.IsActive = false` line and update the comment)
- Test: `pkg/zone/zone_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append at the end of `pkg/zone/zone_test.go`:

```go
func TestRemoveLocSetsIsActiveFalse(t *testing.T) {
	z := New(0, 0, 0, 0)
	loc := entity.NewLoc(0, 3094, 3106, 1, 1, entity.LifecycleDespawn, 100, 0, 0)
	z.AddLoc(loc) // sets IsActive=true (Task 2)
	if !loc.IsActive {
		t.Fatal("setup: AddLoc should have set IsActive=true")
	}
	z.RemoveLoc(loc)
	if loc.IsActive {
		t.Error("Zone.RemoveLoc must set loc.IsActive=false (mirrors TS Zone.removeLoc Zone.ts:254)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... -run TestRemoveLocSetsIsActiveFalse -v`
Expected: FAIL — "must set loc.IsActive=false".

- [ ] **Step 3: Add the IsActive write to Zone.RemoveLoc**

Edit `pkg/zone/zone.go` `RemoveLoc` (currently lines 188-207). Place the write between `clearQueuedEvents` and the `bytes :=`/`queueEvent` block, mirroring TS Zone.ts:253-256 (after `clearQueuedEvents`, before the LocDel emit):

```go
func (z *Zone) RemoveLoc(loc *entity.Loc) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	if loc.Lifecycle == entity.LifecycleDespawn {
		for i, l := range z.Locs {
			if l == loc {
				z.Locs = append(z.Locs[:i], z.Locs[i+1:]...)
				break
			}
		}
	}
	z.clearQueuedEvents(&loc.NonPathing)
	loc.IsActive = false
	bytes := encodeNested(rsbuf.ZoneOpLocDel, func(buf *packet.Packet) {
		rsbuf.EncodeLocDel(buf, coord, loc.Shape(), loc.Angle())
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}
```

- [ ] **Step 4: Verify the new test passes; verify the existing Server.RemoveLoc test still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... -run TestRemoveLocSetsIsActiveFalse -v`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestServerRemoveLocClearsCollision -v`
Expected: PASS (Server.RemoveLoc still produces IsActive=false via the Zone.RemoveLoc call).

- [ ] **Step 5: Remove the now-redundant write in Server.RemoveLoc**

Edit `modules/world/world_zone.go` `Server.RemoveLoc` (currently lines 87-109). Delete the `loc.IsActive = false` line (line 102) and update the doc-comment:

```go
// RemoveLoc clears collision (if BlockWalk), routes the zone-side
// removal, and reschedules respawn (RESPAWN) or untracks (DESPAWN).
// Mirrors TS World.removeLoc (Engine-TS/src/engine/World.ts:1402-1425).
//
// IsActive=false is written by the called Zone.RemoveLoc (pkg/zone/zone.go),
// matching TS Zone.removeLoc (Zone.ts:254).
func (s *Server) RemoveLoc(loc *entitypkg.Loc, duration int) {
	if !loc.IsActive {
		return
	}
	if s.gamemap != nil && s.locTypes != nil {
		if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, false)
		}
	}
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.RemoveLoc(loc)
	s.TrackZone(z)
	if loc.Lifecycle == entitypkg.LifecycleRespawn {
		loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
	} else {
		loc.SetLifeCycle(-1, s.currentTick, nil)
	}
}
```

- [ ] **Step 6: Verify all relevant tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... ./modules/world/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/zone/zone.go pkg/zone/zone_test.go modules/world/world_zone.go
git commit --no-gpg-sign -m "feat(zone): NAI-89 T4 — move Zone.RemoveLoc IsActive write to TS-canonical site

Adds loc.IsActive=false inside Zone.RemoveLoc (pkg/zone/zone.go),
placed between clearQueuedEvents and the LocDel queueEvent to match
TS Zone.removeLoc (Zone.ts:253-256), and removes the redundant write
that lived in Server.RemoveLoc (modules/world/world_zone.go:102).
Observable behavior unchanged."
```

---

### Task 5: Update IsActive doc-comment

**Files:**
- Modify: `pkg/entity/loc.go:16`

This task has no behavior change; it brings the field doc-comment into agreement with the new ownership rule established by Tasks 1-4.

- [ ] **Step 1: Update the doc-comment**

Edit `pkg/entity/loc.go` line 16. Replace:

```go
	IsActive    bool // true after Server.AddLoc, false after Server.RemoveLoc
```

with:

```go
	IsActive    bool // managed by pkg/zone Zone methods (AddStaticLoc, AddLoc, ChangeLoc, RemoveLoc); mirrors TS Zone.ts isActive writes (Zone.ts:208,226,232,254)
```

- [ ] **Step 2: Verify build and tests still green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: success.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/entity/loc.go
git commit --no-gpg-sign -m "docs(entity): NAI-89 T5 — IsActive doc-comment reflects pkg/zone ownership

Tasks 1-4 moved all IsActive writes to pkg/zone Zone methods, matching
TS Zone.ts. Doc-comment was a pre-NAI-88 artifact that pointed at the
old Server-method ownership convention and silently mis-described the
static-loc init path (which was never written through Server.AddLoc)."
```

---

### Task 6: Regression-pin — TestTurnLocRevertChangedStaticMapLoc

**Files:**
- Test: `modules/world/loc_turn_test.go` (append)

This test would have failed at HEAD `0e6d83c` and passes after Tasks 1-3.

- [ ] **Step 1: Write the regression-pin test**

Append at the end of `modules/world/loc_turn_test.go`:

```go
// TestTurnLocRevertChangedStaticMapLoc is the smoke-equivalent unit
// test for the NAI-88 door-revert bug. Setup mirrors the production
// path: a static map loc (loaded via Zone.AddStaticLoc, never via
// Server.AddLoc) is changed by a script call, then the revert tick
// fires. Pre-NAI-89, this would mis-dispatch to AddLoc because the
// static-loc init path never set IsActive=true and Server.ChangeLoc
// didn't either.
func TestTurnLocRevertChangedStaticMapLoc(t *testing.T) {
	s := newLocTurnTestServer(t)
	s.currentTick = 100

	// Build a static map loc and inject it via Zone.AddStaticLoc to
	// mirror Server.populateStaticLocsIntoZones.
	loc := entitypkg.NewLoc(0, 3094, 3106, 1, 1, entitypkg.LifecycleRespawn, 100, 0, 0)
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AddStaticLoc(loc)
	if !loc.IsActive {
		t.Fatal("setup: AddStaticLoc must set IsActive=true (NAI-89 T1)")
	}

	// Script-driven change_loc with duration=5 schedules revert at tick 105.
	s.ChangeLoc(loc, 101, loc.Shape(), loc.Angle(), 5)
	if !loc.IsChanged() {
		t.Fatal("setup: ChangeLoc should have flipped IsChanged=true")
	}
	if !loc.IsActive {
		t.Fatal("setup: ChangeLoc must keep IsActive=true (NAI-89 T3)")
	}

	// Advance to the scheduled tick and dispatch.
	s.currentTick = 105
	s.turnLoc(loc, 105)

	// Post-revert assertions.
	if loc.IsChanged() {
		t.Error("after turnLoc revert at scheduled tick, IsChanged must be false")
	}
	if loc.Type() != 100 {
		t.Errorf("after turnLoc revert, Type: got %d, want 100 (BaseInfo type)", loc.Type())
	}
	if !loc.IsActive {
		t.Error("after turnLoc revert, IsActive must remain true (TS Zone.changeLoc force-flip held through revert)")
	}
	if loc.LifecycleTick != -1 {
		t.Errorf("after turnLoc revert, LifecycleTick: got %d, want -1 (untracked)", loc.LifecycleTick)
	}
}
```

- [ ] **Step 2: Run the new test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTurnLocRevertChangedStaticMapLoc -v`
Expected: PASS (Tasks 1-4 are already in HEAD).

If you want to confirm the test would have caught the bug, you can temporarily revert the Task 1 + Task 3 writes locally and re-run — the test should FAIL with `IsChanged must be false` (case-3 AddLoc fired). Restore the writes before committing.

- [ ] **Step 3: Run the full suite as a final Bundle 1 check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/loc_turn_test.go
git commit --no-gpg-sign -m "test(world): NAI-89 T6 — regression-pin static-loc revert dispatch

TestTurnLocRevertChangedStaticMapLoc reproduces the production
setup that NAI-88 smoke discriminated: a static map loc loaded via
Zone.AddStaticLoc, mutated via Server.ChangeLoc, then ticked to
the scheduled revert. Pre-NAI-89 this mis-dispatched to AddLoc
(IsActive=false → case-3); post-NAI-89 it dispatches RevertLoc
(case-2) and IsChanged returns to false."
```

---

## Bundle 2 — NAI-88 probe retire

### Task 7: Delete probe blocks and revert constructor signature

**Files:**
- Modify: `modules/world/loc_turn.go` (delete 2 probe blocks)
- Modify: `modules/world/tick.go` (delete 2 probe blocks)
- Modify: `modules/world/world_zone.go` (delete the P4 probe block; inline the bare `armRegister`)
- Modify: `modules/world/loc_tracker.go` (strip struct comment about probe fields; drop fields; revert constructor; delete P5/P6 emits; drop unused imports)
- Modify: `modules/world/server.go:167` (constructor call)
- Modify: `modules/world/server_test.go:318` (constructor call)
- Modify: `modules/world/loc_tracker_test.go:10,23,37,51` (4 constructor calls)

This is a strict revert pass; no tests change. Do all edits, then build+test.

- [ ] **Step 1: Strip probes in `modules/world/loc_turn.go`**

Edit the file. Final state should be:

```go
package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// turnLoc is the per-tick dispatch for a tracked Loc. Called from
// Server.processZones for each NonPathing in s.locObjTracker whose
// Parent() is a *Loc. Mirrors TS Loc.turn (Engine-TS/.../Loc.ts:54-74).
//
// Goscape uses Server.currentTick as the authoritative clock and stores
// the absolute target transition tick in LifecycleTick (set via
// Entity.SetLifecycle). TS decrements lifecycleTick-- per tick; the
// observable behavior is equivalent (deviation D-N86-4 in spec §5).
func (s *Server) turnLoc(l *entitypkg.Loc, now int) {
	if l.LifecycleTick != now {
		return
	}
	switch {
	case l.Lifecycle == entitypkg.LifecycleDespawn && l.IsActive:
		s.RemoveLoc(l, 0)
	case l.Lifecycle == entitypkg.LifecycleRespawn && l.IsChanged() && l.IsActive:
		s.RevertLoc(l)
	case l.Lifecycle == entitypkg.LifecycleRespawn && !l.IsActive:
		s.AddLoc(l, 0)
	default:
		// Mirrors TS console.error fallthrough — should not happen.
		// Unconditionally untrack to prevent unbounded re-iteration.
		s.log.Error("loc tracked but no event matched",
			"type", l.Type(), "x", l.X, "z", l.Z, "lifecycle", l.Lifecycle, "active", l.IsActive)
		l.SetLifeCycle(-1, now, nil)
	}
}

// RevertLoc snaps a RESPAWN loc's CurrentInfo back to BaseInfo, swaps
// collision, emits a zone ChangeLoc event, and untracks the lifecycle.
// Mirrors TS World.revertLoc (Engine-TS/.../World.ts:1427-1448). Called
// from turnLoc for the RESPAWN+IsChanged+IsActive branch.
func (s *Server) RevertLoc(l *entitypkg.Loc) {
	if s.gamemap != nil && s.locTypes != nil {
		if oldLt := s.locTypeOrNil(l.Type()); oldLt != nil && oldLt.BlockWalk {
			s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), oldLt.BlockRange,
				l.Length, l.Width, l.X, l.Z, l.Level, false)
		}
	}
	l.Revert()
	if s.gamemap != nil && s.locTypes != nil {
		if newLt := s.locTypeOrNil(l.Type()); newLt != nil && newLt.BlockWalk {
			s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), newLt.BlockRange,
				l.Length, l.Width, l.X, l.Z, l.Level, true)
		}
	}
	z := s.zoneMap.Get(l.Level, l.X, l.Z)
	z.ChangeLoc(l)
	// TS-faithful tail order (World.ts:1445-1447): SetLifeCycle(-1) BEFORE
	// TrackZone, the inverse of AddLoc/ChangeLoc/RemoveLoc. The two writes
	// touch independent data structures (locObjTracker vs zonesTracking)
	// so the order is observably equivalent — preserving the TS sequence
	// for audit clarity.
	l.SetLifeCycle(-1, s.currentTick, nil)
	s.TrackZone(z)
}
```

- [ ] **Step 2: Strip probes in `modules/world/tick.go`**

Locate the two `// NAI-88 probe; remove at Stage 2 close` blocks (currently around lines 483-491 and 493-502 at HEAD `0e6d83c`). Delete each `if s.cfg.NodeDebug && s.log != nil { s.log.Debug("nai88 process_zones iter", ...) }` block in full, including the preceding NAI-88 comment line. The second block lives inside a `for i, np := range snap` loop where `i` is only referenced by the probe; after deletion, change that loop header to `for _, np := range snap` so the unused variable doesn't cause a compile error.

After both deletions, the relevant `processZones` body reads:

```go
		t := s.locObjTracker.(*locObjTracker)
		snap := make([]*entitypkg.NonPathing, 0, t.list.Size())
		for np := range t.All() {
			snap = append(snap, np)
		}
		for _, np := range snap {
			switch p := np.Parent().(type) {
			case *entitypkg.Loc:
				s.turnLoc(p, s.currentTick)
			case *entitypkg.Obj:
				// TODO(NAI-86 D-N86-3): Obj.Turn ports later.
				_ = p
			}
		}
```

The only `fmt` use in `modules/world/tick.go` was inside the second probe block (verified by `rg "fmt\." modules/world/tick.go`). Drop the `"fmt"` import line from the import block.

- [ ] **Step 3: Strip the P4 probe in `modules/world/world_zone.go ChangeLoc`**

Edit `modules/world/world_zone.go` `Server.ChangeLoc`. Replace the body so the `armRegister` boolean drives the if/else directly without the debug emit block. Final `ChangeLoc` body (post-Bundle 1 + post-probe-retire):

```go
// ChangeLoc rewrites the loc's render fields to (typ, shape, angle)
// and reschedules its lifecycle to despawn/revert at currentTick+duration.
// Mirrors TS World.changeLoc (Engine-TS/src/engine/World.ts:1350-1386).
//
// Order matters per TS: (1) early-return if DESPAWN+!IsActive (don't
// return inactive DESPAWN to game world; goscape uses IsActive where
// TS uses isValid — see spec D-N86-2 — defensive gate, TS-equivalent);
// (2) remove old collision; (3) loc.Change(); (4) add new collision;
// (5) zone.ChangeLoc; (6) trackZone; (7) SetLifeCycle (duration if
// changed-or-DESPAWN, else -1 to untrack a no-op static change).
//
// IsActive=true is written by the called Zone.ChangeLoc (pkg/zone/zone.go),
// matching TS Zone.changeLoc (Zone.ts:232).
func (s *Server) ChangeLoc(loc *entitypkg.Loc, typ, shape, angle, duration int) {
	if loc.Lifecycle == entitypkg.LifecycleDespawn && !loc.IsActive {
		return
	}
	if loc.IsActive && s.gamemap != nil && s.locTypes != nil {
		if oldLt := s.locTypeOrNil(loc.Type()); oldLt != nil && oldLt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), oldLt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, false)
		}
	}
	loc.Change(typ, shape, angle)
	if s.gamemap != nil && s.locTypes != nil {
		if newLt := s.locTypeOrNil(typ); newLt != nil && newLt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), newLt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
		}
	}
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.ChangeLoc(loc)
	s.TrackZone(z)
	if loc.IsChanged() || loc.Lifecycle == entitypkg.LifecycleDespawn {
		loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
	} else {
		loc.SetLifeCycle(-1, s.currentTick, nil)
	}
}
```

- [ ] **Step 4: Revert `modules/world/loc_tracker.go` to no-args constructor**

Replace the file with this final state:

```go
package world

import (
	"iter"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// locObjTracker is the per-Server registry of NonPathing entities with
// pending lifecycle transitions. Iterated each tick by Server.processZones.
// Mirrors TS World.locObjTracker (Engine-TS/.../World.ts:154,964-973).
//
// Backed by pkg/zone.DoublyLinkList for O(1) Add/Unlink and an auxiliary
// map *NonPathing → *Element for O(1) Unregister-by-pointer.
type locObjTracker struct {
	list  *zone.DoublyLinkList[*entitypkg.NonPathing]
	nodes map[*entitypkg.NonPathing]*zone.Element[*entitypkg.NonPathing]
}

// newLocObjTracker constructs an empty tracker. Server.New calls this
// once at server startup.
func newLocObjTracker() *locObjTracker {
	return &locObjTracker{
		list:  &zone.DoublyLinkList[*entitypkg.NonPathing]{},
		nodes: map[*entitypkg.NonPathing]*zone.Element[*entitypkg.NonPathing]{},
	}
}

// Register adds np to the tracker. Idempotent — re-registering an
// already-tracked np unlinks the old node first to keep the list
// duplicate-free, matching TS behavior where setLifeCycle always
// unlinks the previous eventTracker before re-adding.
func (t *locObjTracker) Register(np *entitypkg.NonPathing) {
	if existing, ok := t.nodes[np]; ok {
		existing.Unlink()
		delete(t.nodes, np)
	}
	t.nodes[np] = t.list.AddTail(np)
}

// Unregister removes np from the tracker. No-op if np is not tracked.
func (t *locObjTracker) Unregister(np *entitypkg.NonPathing) {
	if e, ok := t.nodes[np]; ok {
		e.Unlink()
		delete(t.nodes, np)
	}
}

// All returns an iterator over the tracked entries in insertion order.
// Callers that mutate the tracker mid-iteration MUST snapshot first
// (Server.processZones does this).
func (t *locObjTracker) All() iter.Seq[*entitypkg.NonPathing] {
	return t.list.All(false)
}
```

- [ ] **Step 5: Update the production constructor call in `modules/world/server.go`**

Edit line 167. Change:

```go
		locObjTracker: newLocObjTracker(logger, cfg.NodeDebug),
```

to:

```go
		locObjTracker: newLocObjTracker(),
```

- [ ] **Step 6: Update the test fixture constructor call in `modules/world/server_test.go`**

Edit line 318. Change:

```go
		locObjTracker:  newLocObjTracker(nil, false),
```

to:

```go
		locObjTracker:  newLocObjTracker(),
```

- [ ] **Step 7: Update the 4 constructor calls in `modules/world/loc_tracker_test.go`**

Edit each of lines 10, 23, 37, 51. Change every `newLocObjTracker(nil, false)` to `newLocObjTracker()`.

- [ ] **Step 8: Verify no NAI-88 markers or `nai88` strings remain**

Run: `rg "NAI-88 probe; remove at Stage 2 close" modules/`
Expected: 0 matches.

Run: `rg "nai88" modules/`
Expected: 0 matches.

Run: `rg -i "NAI-88" modules/`
Expected: 0 matches (commit messages and docs/ still reference NAI-88; that's fine — only the `modules/` source tree should be clean).

- [ ] **Step 9: Verify build and tests are green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: success.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add modules/world/loc_turn.go modules/world/tick.go modules/world/world_zone.go modules/world/loc_tracker.go modules/world/server.go modules/world/server_test.go modules/world/loc_tracker_test.go
git commit --no-gpg-sign -m "chore(world): NAI-89 T7 — retire NAI-88 probe scaffold

Strict revert: deletes the 7 NAI-88 probe-emit blocks (P1×2, P2, P3,
P4, P5, P6), strips the locObjTracker struct comment about probe
fields, drops the log+nodeDebug fields, reverts newLocObjTracker
to no-args, and updates 6 call sites (1 prod + 5 tests). Drops
now-unused fmt/log/slog/runtime imports. Build and full test
suite green; rg confirms 0 NAI-88 markers in modules/."
```

---

## Bundle 3 — Smoke handoff

### Task 8: Hand off smoke to user

This task does not modify the repo. The controller emits a paste-ready prompt for the user, who runs the server (per `smoke_test_server_handoff` memory — Claude's sandboxed process is unreachable from the host Java client) and observes the door behavior.

- [ ] **Step 1: Emit the smoke prompt to the user**

In your final message before stopping, include this verbatim block for the user to follow:

```
NAI-89 smoke handoff. To verify the loc-revert fix end-to-end:

1. Build: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o ./goscape ./cmd/goscape`
2. Run: `./goscape --config.file config.yaml`  (with Tutorial Island static-loc data loaded)
3. Connect with the Java client, walk to Tutorial Island door at (3098, 3107, level 0).
4. Open the door (script invokes change_loc).
5. Wait through the script-encoded duration (~5 ticks).
6. CONFIRM: door reverts visibly to its closed (baseInfo) state in the client viewport.

Per java_client_coord_chat_suppression memory: the Tutorial Island
coord box overrides chat to suppress messages. Visual confirmation
of the door rendering is the gate; do NOT rely on chat output inside
the suppressed coord box.

If the door reverts: NAI-88 + NAI-89 are closed. Reply `smoke green`
and the controller will land the close commit with `Closes memory:`
trailer per close_commit_memory_trailer memory.

If the door does NOT revert (or reverts incorrectly): paste any
client-side or server-side trace and the controller will route the
residual per smoke_surfaces_adjacent_divergences memory (≤30 LOC
fix → in-scope NAI-89 stretch; else NAI-90).
```

- [ ] **Step 2: Stop and wait for user smoke result**

Do not commit anything. The close commit is deferred until the user replies with the smoke result.

---

## Self-Review (controller)

Before dispatching, the controller should re-verify:

1. **Spec coverage:** Every spec section (§3 Fix strategy, §4 Bundles 1-3, §5 Test strategy, §6 Risks) maps to at least one task. Bundle 1 → T1-T6; Bundle 2 → T7; Bundle 3 → T8.

2. **Placeholder scan:** No TBD/TODO. Vague-on-purpose: `pkg/zone/zone_test.go` ad-hoc IsActive writes (spec §6 risk row) — controller pre-flight greps before T7 and decides keep/drop inline. Accept as-is.

3. **Type/signature consistency:** `newLocObjTracker()` returns `*locObjTracker`; field set is `{list, nodes}` after T7; all 6 call sites pass zero args. Bundle 1 method receivers (`*Zone`, `*Server`) and parameters match HEAD.

4. **Premise re-grep at dispatch:** Per `controller_preflight` memory, before each T1-T8 dispatch the controller re-greps the named line numbers (they will shift across tasks; the file/symbol path is the durable handle).

5. **Subagent worktree-path check:** Per `feedback_subagent_wt_path` memory, controller runs `git status` on main between merges; stash any stray content from main working tree.

---

## Closes (after Bundle 3 smoke green)

Final close commit by the controller, once the user reports `smoke green`:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
close: NAI-88 + NAI-89 — loc-revert IsActive consolidation, smoke green

Bundle 1 moved all four loc IsActive writes into pkg/zone Zone methods,
matching TS Zone.ts (Zone.addStaticLoc, Zone.addLoc, Zone.changeLoc,
Zone.removeLoc). Bundle 2 retired the NAI-88 Stage 1 probe scaffold.
Bundle 3 smoke confirmed the Tutorial Island door reverts to baseInfo
at the scheduled tick.

Closes memory: NAI-88 (Stage 1 probe scaffold).
EOF
)"
```
