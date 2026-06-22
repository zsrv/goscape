# NAI-66 Pathing-Entity Reorient Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `(*Player).reorient` and `(*Npc).reorient` (TS PathingEntity.ts:349-361), wire into `Server.processInfo` before rsbuf compute, and activate `Player.SetInteraction` default arm to write `targetX/Z` for Loc/Obj targets — closing `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ`. Re-frame `NAI-34-D4-NPC` and `NAI-34-D5-NPC` as permanent dead-API skips.

**Architecture:** Single TDD task with four cycles (Player.reorient + Player.SetInteraction default arm + Npc.reorient + processInfo wiring), followed by a CLOSE task for doc-only deviation reframes and the close commit. Each cycle is its own commit; the CLOSE task is the final close commit.

**Tech Stack:** Go 1.26+. TS source canonical path: `$HOME/Code/github.com/LostCityRS/Engine-TS/`.

---

## Pre-flight Verification (Controller)

Before dispatching the implementer, the controller MUST verify these against HEAD per `controller_preflight.md`:

- [ ] `modules/world/interaction.go` line ~92 still hosts the `default:` DEVIATION block in `Player.SetInteraction`. Run: `grep -n "DEVIATION NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ" modules/world/interaction.go` — expect a single hit on line ~92.
- [ ] `targetWidthLength` is at `modules/world/npc_interaction.go:687` (or grep-locate). Run: `grep -n "func targetWidthLength" modules/world/npc_interaction.go`.
- [ ] `Player.targetX, targetZ, stepsTaken, target, focus` all exist. Run: `grep -nE "^\s+(targetX|targetZ|stepsTaken|target|focus)\b" modules/world/player.go modules/world/player_script.go | head`.
- [ ] `Npc.targetX, targetZ, stepsTaken, target, focus` all exist. Run: `grep -nE "^\s+(targetX|targetZ|stepsTaken|target|focus)\b" modules/world/npc.go modules/world/npc_interaction.go | head`.
- [ ] `Server.processInfo` is at `modules/world/tick.go:329`. Run: `grep -n "func (s \*Server) processInfo" modules/world/tick.go`.
- [ ] No existing `(*Player).reorient` or `(*Npc).reorient`. Run: `grep -n "func (p \*Player) reorient\|func (n \*Npc) reorient" modules/world/*.go` — expect empty.
- [ ] `coordgrid.Fine` exists at `pkg/coordgrid/coordgrid.go:127`. Run: `grep -n "^func Fine" pkg/coordgrid/coordgrid.go`.
- [ ] `entity.Coords()` returns `(x, z, level int)`. Run: `grep -n "Coords() (x, z, level int)" pkg/entity/*.go modules/world/*.go | head`.
- [ ] Tests run clean against HEAD. Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/...` — expect PASS.

---

## Task 1 — reorient port + Player.SetInteraction default arm + processInfo wiring

**Files (all touched in this task):**
- Create: `modules/world/player_reorient_test.go`
- Create: `modules/world/npc_reorient_test.go`
- Modify: `modules/world/movement.go` (append `(*Player).reorient`)
- Modify: `modules/world/npc_interaction.go` (append `(*Npc).reorient` near `(*Npc).focus`)
- Modify: `modules/world/interaction.go` (replace DEVIATION block in `Player.SetInteraction` default arm; append 2 SetInteraction tests in `interaction_test.go`)
- Modify: `modules/world/interaction_test.go` (append 2 default-arm tests; rename existing `TestSetInteractionLocTargetDoesNotSetFaceEntity` comment to drop the NAI-41-D claim of deferral)
- Modify: `modules/world/tick.go` (insert reorient loops in `processInfo`)
- Modify: `modules/world/rsbuf_per_tick_test.go` (append 1 wire test)

**Test commands:**
- Targeted: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerReorient -v`
- Full module: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
- Race: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

### Cycle A — `(*Player).reorient` (5 cases) + impl

- [ ] **Step A1: Write the 5 failing tests.** Create `modules/world/player_reorient_test.go` with the following content:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// TestPlayerReorientPathingTargetPlayer pins TS PathingEntity.ts:351-353
// branch: target is *Player → focus on fine(t.x, 1), fine(t.z, 1).
// targetX/Z left untouched.
func TestPlayerReorientPathingTargetPlayer(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 110, 120, 0)
	defer otherWait()

	p.target = other

	p.reorient()

	if p.faceAngleX != coordgrid.Fine(110, 1) {
		t.Errorf("faceAngleX: got %d, want %d", p.faceAngleX, coordgrid.Fine(110, 1))
	}
	if p.faceAngleZ != coordgrid.Fine(120, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", p.faceAngleZ, coordgrid.Fine(120, 1))
	}
	if p.targetX != -1 || p.targetZ != -1 {
		t.Errorf("targetX/Z: got (%d,%d), want (-1,-1) (unchanged for PathingEntity branch)", p.targetX, p.targetZ)
	}
}

// TestPlayerReorientPathingTargetNpc — symmetric to above, *Npc target.
func TestPlayerReorientPathingTargetNpc(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 105, 108, 0)

	p.target = npc

	p.reorient()

	if p.faceAngleX != coordgrid.Fine(105, 1) {
		t.Errorf("faceAngleX: got %d, want %d", p.faceAngleX, coordgrid.Fine(105, 1))
	}
	if p.faceAngleZ != coordgrid.Fine(108, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", p.faceAngleZ, coordgrid.Fine(108, 1))
	}
}

// TestPlayerReorientLocTargetStepsZero pins the default-arm focus + clear:
// Loc target with stepsTaken==0 and targetX != -1 → focus on cached
// targetX/Z, then clear to -1.
func TestPlayerReorientLocTargetStepsZero(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	p.target = loc
	p.targetX = 999
	p.targetZ = 1001
	p.stepsTaken = 0

	p.reorient()

	if p.faceAngleX != 999 {
		t.Errorf("faceAngleX: got %d, want 999", p.faceAngleX)
	}
	if p.faceAngleZ != 1001 {
		t.Errorf("faceAngleZ: got %d, want 1001", p.faceAngleZ)
	}
	if p.targetX != -1 {
		t.Errorf("targetX: got %d, want -1 (cleared after focus)", p.targetX)
	}
	if p.targetZ != -1 {
		t.Errorf("targetZ: got %d, want -1 (cleared after focus)", p.targetZ)
	}
}

// TestPlayerReorientLocTargetStepsNonzero pins the early-out:
// Loc target with stepsTaken > 0 → no focus, no clear.
func TestPlayerReorientLocTargetStepsNonzero(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	p.target = loc
	p.targetX = 999
	p.targetZ = 1001
	p.stepsTaken = 3
	preFaceX := p.faceAngleX
	preFaceZ := p.faceAngleZ

	p.reorient()

	if p.faceAngleX != preFaceX {
		t.Errorf("faceAngleX: got %d, want %d (unchanged; stepsTaken>0 early-out)", p.faceAngleX, preFaceX)
	}
	if p.faceAngleZ != preFaceZ {
		t.Errorf("faceAngleZ: got %d, want %d", p.faceAngleZ, preFaceZ)
	}
	if p.targetX != 999 || p.targetZ != 1001 {
		t.Errorf("targetX/Z: got (%d,%d), want (999,1001) (unchanged)", p.targetX, p.targetZ)
	}
}

// TestPlayerReorientNilTarget pins the no-op when target is nil.
func TestPlayerReorientNilTarget(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	p.target = nil
	p.targetX = -1
	p.targetZ = -1
	preFaceX := p.faceAngleX
	preFaceZ := p.faceAngleZ

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("reorient panicked on nil target: %v", r)
		}
	}()

	p.reorient()

	if p.faceAngleX != preFaceX || p.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle changed on nil target: got (%d,%d), want (%d,%d)", p.faceAngleX, p.faceAngleZ, preFaceX, preFaceZ)
	}
}
```

- [ ] **Step A2: Run tests, verify FAIL.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerReorient -v`. Expected: FAIL with `p.reorient undefined (type *Player has no field or method reorient)`.

- [ ] **Step A3: Implement `(*Player).reorient`.** Append to `modules/world/movement.go` (file already imports `coordgrid`):

```go
// reorient is the per-tick refocus invoked from Server.processInfo
// before rsbuf compute. Mirrors TS PathingEntity.reorient at
// Engine-TS/src/engine/entity/PathingEntity.ts:349-361.
//
// PathingEntity targets (Player/Npc) are refocused on the target's
// current position (target may have moved this tick). Non-pathing
// targets (Loc/Obj) trigger one-shot focus + clear of the cached
// fine-coord (targetX/Z) iff the player took zero steps this tick —
// semantically "the entity moved off while we were trying to reach it."
func (p *Player) reorient() {
	switch t := p.target.(type) {
	case *Player:
		p.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	case *Npc:
		p.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	default:
		_ = t
		if p.targetX != -1 && p.stepsTaken == 0 {
			p.focus(p.targetX, p.targetZ, false)
			p.targetX = -1
			p.targetZ = -1
		}
	}
}
```

(`_ = t` silences the unused-variable error in the default branch — `t` is bound for type-switch even when the body doesn't use it.)

- [ ] **Step A4: Run tests, verify PASS.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerReorient -v`. Expected: 5 PASS.

- [ ] **Step A5: Commit.**

```bash
git add modules/world/player_reorient_test.go modules/world/movement.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-66 — (*Player).reorient port (TS PathingEntity.ts:349-361)

Per-tick refocus method invoked from Server.processInfo before rsbuf
compute. PathingEntity targets refocus on the target's current
position; non-pathing (Loc/Obj) targets trigger one-shot focus +
clear of cached targetX/Z iff stepsTaken == 0.

Wiring into processInfo and the (*Player).SetInteraction default-arm
write that materialises this consumer come in subsequent cycles of
the same NAI-66 task.

5 unit tests cover all 4 reorient branches plus the nil-target no-op.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Cycle B — Player.SetInteraction default arm + 2 tests

- [ ] **Step B1: Write the 2 failing tests.** Append to `modules/world/interaction_test.go` (existing file; verify imports already include `entitypkg "github.com/zsrv/goscape/pkg/entity"` and `"github.com/zsrv/goscape/pkg/coordgrid"` — add if missing). The first test REPLACES the obsolete contract test `TestSetInteractionLocTargetDoesNotSetFaceEntity` (lines ~603-624) with the active-write contract; the second adds the Obj-target case.

First, RENAME and REWRITE the existing test (replace the body but keep the test function name to preserve git history continuity, OR replace the function entirely — either is acceptable; the snippet below shows full replacement of the old test function):

```go
// TestSetInteractionLocTargetWritesTargetXZ pins NAI-66 closure of
// NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ: *Loc target writes
// targetX = fine(loc.X, loc.Width), targetZ = fine(loc.Z, loc.Length)
// per TS PathingEntity.ts:542-545. Replaces the previous
// TestSetInteractionLocTargetDoesNotSetFaceEntity contract (which
// pinned the now-closed deferral).
func TestSetInteractionLocTargetWritesTargetXZ(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	// 3x2 Loc at (50, 60).
	loc := entitypkg.NewLoc(0, 50, 60, 3, 2, entitypkg.LifecycleForever, 0, 10, 0)

	p.SetInteraction(InteractionEngine, loc, 1, -1)

	// faceEntity must remain unwritten (Loc branch never sets it).
	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Loc branch must not write)", p.faceEntity)
	}
	if p.masks&MaskFaceEntity != 0 {
		t.Error("MaskFaceEntity bit must NOT be set after SetInteraction with *Loc target")
	}
	// targetX/Z now written per NAI-66.
	wantTX := coordgrid.Fine(50, 3)
	wantTZ := coordgrid.Fine(60, 2)
	if p.targetX != wantTX {
		t.Errorf("targetX: got %d, want %d (fine(50, width=3))", p.targetX, wantTX)
	}
	if p.targetZ != wantTZ {
		t.Errorf("targetZ: got %d, want %d (fine(60, length=2))", p.targetZ, wantTZ)
	}
}

// TestSetInteractionObjTargetWritesTargetXZ pins the Obj-target case:
// always 1x1, so fine(obj.X, 1) and fine(obj.Z, 1).
func TestSetInteractionObjTargetWritesTargetXZ(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	obj := entitypkg.NewObj(0, 50, 60, entitypkg.LifecycleForever, 42, 1)

	p.SetInteraction(InteractionEngine, obj, 1, -1)

	wantTX := coordgrid.Fine(50, 1)
	wantTZ := coordgrid.Fine(60, 1)
	if p.targetX != wantTX {
		t.Errorf("targetX: got %d, want %d (fine(50, 1))", p.targetX, wantTX)
	}
	if p.targetZ != wantTZ {
		t.Errorf("targetZ: got %d, want %d (fine(60, 1))", p.targetZ, wantTZ)
	}
}
```

DELETE the old `TestSetInteractionLocTargetDoesNotSetFaceEntity` function in `interaction_test.go` (lines ~600-625). Its claim that "deviation is intentional, not a partial port" is now obsolete — the deviation is closed.

- [ ] **Step B2: Run tests, verify FAIL.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestSetInteraction(Loc|Obj)TargetWritesTargetXZ" -v`. Expected: FAIL — current `Player.SetInteraction` default arm is empty (DEVIATION block); `targetX` and `targetZ` remain at their init -1.

- [ ] **Step B3: Implement default arm activation.** Replace the DEVIATION block in `modules/world/interaction.go` lines 91-98 (the `default:` case in `Player.SetInteraction`) with:

```go
	default:
		// Loc/Obj target — cache fine-coord for reorient consumption.
		// TS PathingEntity.ts:542-545. Closes
		// NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ in NAI-66 (consumer is
		// (*Player).reorient at modules/world/movement.go).
		tx, tz, _ := t.Coords()
		tw, tl := targetWidthLength(t)
		p.targetX = coordgrid.Fine(tx, tw)
		p.targetZ = coordgrid.Fine(tz, tl)
```

`targetWidthLength` is already in the same package (see `modules/world/npc_interaction.go:687`).

- [ ] **Step B4: Run tests, verify PASS.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestSetInteraction(Loc|Obj)TargetWritesTargetXZ|TestPlayerReorient" -v`. Expected: PASS for all (the previous Cycle A tests should still pass; the 2 new tests should now pass). Also run the full module to catch regressions: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`. Expected: PASS.

- [ ] **Step B5: Commit.**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-66 — Player.SetInteraction Loc/Obj targetX/Z write

Activates the default arm of (*Player).SetInteraction's type-switch
to write targetX = fine(target.x, width), targetZ = fine(target.z,
length) per TS PathingEntity.ts:542-545. Reuses the existing
targetWidthLength helper (npc_interaction.go:687).

This closes NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ — (*Player).reorient
(committed in the previous cycle) is the consumer that materialises
the cached fine-coords.

Replaces the obsolete TestSetInteractionLocTargetDoesNotSetFaceEntity
contract test with TestSetInteractionLocTargetWritesTargetXZ +
TestSetInteractionObjTargetWritesTargetXZ.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Cycle C — `(*Npc).reorient` (5 cases) + impl

- [ ] **Step C1: Write the 5 failing tests.** Create `modules/world/npc_reorient_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// TestNpcReorientPathingTargetPlayer pins TS PathingEntity.ts:351-353
// branch on the Npc side: target is *Player → focus on fine.
func TestNpcReorientPathingTargetPlayer(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	other, wait := makeInteractionPlayer(t, s, 110, 120, 0)
	defer wait()

	npc.target = other

	npc.reorient()

	if npc.faceAngleX != coordgrid.Fine(110, 1) {
		t.Errorf("faceAngleX: got %d, want %d", npc.faceAngleX, coordgrid.Fine(110, 1))
	}
	if npc.faceAngleZ != coordgrid.Fine(120, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", npc.faceAngleZ, coordgrid.Fine(120, 1))
	}
	if npc.targetX != -1 || npc.targetZ != -1 {
		t.Errorf("targetX/Z: got (%d,%d), want (-1,-1) (unchanged for PathingEntity branch)", npc.targetX, npc.targetZ)
	}
}

// TestNpcReorientPathingTargetNpc — symmetric, *Npc target.
func TestNpcReorientPathingTargetNpc(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	other := makeInteractionNpc(t, s, 2, 105, 108, 0)

	npc.target = other

	npc.reorient()

	if npc.faceAngleX != coordgrid.Fine(105, 1) {
		t.Errorf("faceAngleX: got %d, want %d", npc.faceAngleX, coordgrid.Fine(105, 1))
	}
	if npc.faceAngleZ != coordgrid.Fine(108, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", npc.faceAngleZ, coordgrid.Fine(108, 1))
	}
}

// TestNpcReorientLocTargetStepsZero pins focus + clear on the default
// branch when stepsTaken == 0 and targetX != -1.
func TestNpcReorientLocTargetStepsZero(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	npc.target = loc
	npc.targetX = 999
	npc.targetZ = 1001
	npc.stepsTaken = 0

	npc.reorient()

	if npc.faceAngleX != 999 {
		t.Errorf("faceAngleX: got %d, want 999", npc.faceAngleX)
	}
	if npc.faceAngleZ != 1001 {
		t.Errorf("faceAngleZ: got %d, want 1001", npc.faceAngleZ)
	}
	if npc.targetX != -1 {
		t.Errorf("targetX: got %d, want -1 (cleared after focus)", npc.targetX)
	}
	if npc.targetZ != -1 {
		t.Errorf("targetZ: got %d, want -1 (cleared after focus)", npc.targetZ)
	}
}

// TestNpcReorientLocTargetStepsNonzero pins the early-out.
func TestNpcReorientLocTargetStepsNonzero(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	npc.target = loc
	npc.targetX = 999
	npc.targetZ = 1001
	npc.stepsTaken = 3
	preFaceX := npc.faceAngleX
	preFaceZ := npc.faceAngleZ

	npc.reorient()

	if npc.faceAngleX != preFaceX {
		t.Errorf("faceAngleX: got %d, want %d (unchanged; stepsTaken>0 early-out)", npc.faceAngleX, preFaceX)
	}
	if npc.faceAngleZ != preFaceZ {
		t.Errorf("faceAngleZ: got %d, want %d", npc.faceAngleZ, preFaceZ)
	}
	if npc.targetX != 999 || npc.targetZ != 1001 {
		t.Errorf("targetX/Z: got (%d,%d), want (999,1001) (unchanged)", npc.targetX, npc.targetZ)
	}
}

// TestNpcReorientNilTarget pins the no-op.
func TestNpcReorientNilTarget(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	npc.target = nil
	npc.targetX = -1
	npc.targetZ = -1
	preFaceX := npc.faceAngleX
	preFaceZ := npc.faceAngleZ

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("reorient panicked on nil target: %v", r)
		}
	}()

	npc.reorient()

	if npc.faceAngleX != preFaceX || npc.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle changed on nil target: got (%d,%d), want (%d,%d)", npc.faceAngleX, npc.faceAngleZ, preFaceX, preFaceZ)
	}
}
```

- [ ] **Step C2: Run tests, verify FAIL.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcReorient -v`. Expected: FAIL with `npc.reorient undefined`.

- [ ] **Step C3: Implement `(*Npc).reorient`.** Append to `modules/world/npc_interaction.go` (file already imports `coordgrid` and `entitypkg`), placed adjacent to the existing `(*Npc).focus` method (look for `func (n *Npc) focus(`):

```go
// reorient is the Npc-side per-tick refocus invoked from
// Server.processInfo before rsbuf compute. Mirrors TS
// PathingEntity.reorient at PathingEntity.ts:349-361. Same shape as
// (*Player).reorient.
func (n *Npc) reorient() {
	switch t := n.target.(type) {
	case *Player:
		n.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	case *Npc:
		n.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	default:
		_ = t
		if n.targetX != -1 && n.stepsTaken == 0 {
			n.focus(n.targetX, n.targetZ, false)
			n.targetX = -1
			n.targetZ = -1
		}
	}
}
```

- [ ] **Step C4: Run tests, verify PASS.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcReorient -v`. Expected: 5 PASS. Also run full module: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...` — expect PASS.

- [ ] **Step C5: Commit.**

```bash
git add modules/world/npc_reorient_test.go modules/world/npc_interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-66 — (*Npc).reorient port (TS PathingEntity.ts:349-361)

Per-tick refocus method on Npc, mirroring (*Player).reorient
shape-for-shape. Same 4 branches: Player target / Npc target /
Loc-default with stepsTaken == 0 (focus + clear) / Loc-default with
stepsTaken > 0 (no-op).

5 unit tests cover all branches plus the nil-target no-op.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Cycle D — `processInfo` wiring + 1 wire test

- [ ] **Step D1: Write the failing wire test.** Append to `modules/world/rsbuf_per_tick_test.go`:

```go
// TestProcessInfoInvokesReorient pins NAI-66's per-tick wire-up: a
// player with a *Loc target and cached targetX/Z + stepsTaken == 0
// must have targetX/Z cleared and faceAngleX/Z written by the time
// processInfo's rsbuf compute pass runs. This pins both ordering
// (reorient runs before ComputePlayers) and the per-tick invocation.
func TestProcessInfoInvokesReorient(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	p.target = loc
	p.targetX = 999
	p.targetZ = 1001
	p.stepsTaken = 0

	s.processInfo()

	if p.targetX != -1 {
		t.Errorf("targetX: got %d, want -1 (reorient must clear)", p.targetX)
	}
	if p.targetZ != -1 {
		t.Errorf("targetZ: got %d, want -1 (reorient must clear)", p.targetZ)
	}
	if p.faceAngleX != 999 {
		t.Errorf("faceAngleX: got %d, want 999 (reorient must focus)", p.faceAngleX)
	}
	if p.faceAngleZ != 1001 {
		t.Errorf("faceAngleZ: got %d, want 1001 (reorient must focus)", p.faceAngleZ)
	}
}
```

(Verify `entitypkg` is already imported in `rsbuf_per_tick_test.go`. If not, add `entitypkg "github.com/zsrv/goscape/pkg/entity"` to its imports.)

- [ ] **Step D2: Run test, verify FAIL.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessInfoInvokesReorient -v`. Expected: FAIL — `processInfo` does not yet invoke `reorient`.

- [ ] **Step D3: Wire reorient into `processInfo`.** Modify `modules/world/tick.go` `processInfo` body. After the players-snapshot copy (existing lines ~330-333) and BEFORE the appearance-regen loop, insert:

```go
	// NAI-66: TS World.ts:995 — per-tick refocus before rsbuf compute.
	// Refocuses on a moved PathingEntity target or clears the cached
	// Loc/Obj targetX/Z when the player took zero steps this tick.
	for _, p := range players {
		p.reorient()
	}
```

Then, BEFORE the line `s.renderer.ComputeNpcs(npcSources)` (existing ~line 354), insert the symmetric Npc loop:

```go
	// NAI-66: TS World.ts:1046 — npc-side per-tick refocus.
	for _, n := range s.npcLoop {
		n.reorient()
	}
```

Both loops must run BEFORE the corresponding `ComputePlayers`/`ComputeNpcs` call so the rsbuf state push observes the cleared `targetX/Z` and updated `faceAngleX/Z`.

Final `processInfo` head should look like:

```go
func (s *Server) processInfo() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	// NAI-66: TS World.ts:995 — per-tick refocus before rsbuf compute.
	for _, p := range players {
		p.reorient()
	}

	// (existing) Regenerate appearance buffer ...
	if s.objTypes != nil && s.invTypes != nil {
		for _, p := range players {
			if p.masks&MaskAppearance != 0 {
				p.generateAppearance(s.objTypes, s.invTypes, s.currentTick)
			}
		}
	}

	sources := make([]rsbuf.PlayerSource, len(players))
	for i, p := range players {
		sources[i] = p
	}
	s.renderer.ComputePlayers(sources)

	// NAI-66: TS World.ts:1046 — npc-side per-tick refocus.
	for _, n := range s.npcLoop {
		n.reorient()
	}

	npcSources := make([]rsbuf.NpcSource, len(s.npcLoop))
	for i, n := range s.npcLoop {
		npcSources[i] = n
	}
	s.renderer.ComputeNpcs(npcSources)

	// (existing) parallel-write to rsbuf state (NAI-29 Bundle 4)
	if s.rsbuf != nil {
		// ... unchanged ...
```

- [ ] **Step D4: Run wire test + full module.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessInfoInvokesReorient -v`. Expected: PASS. Then run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`. Expected: PASS for all. Then race: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`. Expected: PASS, no races.

- [ ] **Step D5: Commit.**

```bash
git add modules/world/tick.go modules/world/rsbuf_per_tick_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-66 — wire reorient into Server.processInfo

Inserts player-loop reorient before ComputePlayers and npc-loop
reorient before ComputeNpcs in processInfo. Mirrors TS World.ts:995
(player.reorient) and World.ts:1046 (npc.reorient) — both run before
the corresponding info-compute pass so rsbuf state observes cleared
targetX/Z and updated faceAngleX/Z from the per-tick refocus.

1 wire test (TestProcessInfoInvokesReorient) pins ordering: player
with Loc target + targetX=999, targetZ=1001, stepsTaken=0 ends the
tick with targetX/Z == -1 and faceAngleX/Z == 999/1001.

Closes NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ end-to-end (consumer is
now wired into the per-tick path).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 1 final verification

- [ ] **Step T1.V1: Full module test, race-mode.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`. Expected: PASS, no races.
- [ ] **Step T1.V2: Cross-package test.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`. Expected: PASS for all packages (no regressions in `pkg/script`, `pkg/rsbuf`, etc.).
- [ ] **Step T1.V3: Build clean.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`. Expected: clean exit.
- [ ] **Step T1.V4: vet.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`. Expected: no warnings.

---

## Task 2 — CLOSE: deviation reframe + memory + close commit

This task does NO production code changes — only doc-comment text updates at deviation tag sites and the memory-file carry-forward update, plus the final close commit.

**Files:**
- Modify: `modules/world/npc_script.go` (DEVIATION block at lines ~95-122 — reframe)
- Modify: `pkg/script/active.go` (DEVIATION block at lines ~600-625 — reframe)
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (add NAI-66 close entry; update D4-NPC + D5-NPC carry-forward framing)

**Verification commands:**
- Grep all tag-site mentions stayed in sync: `grep -rn "NAI-34-D4-NPC\|NAI-34-D5-NPC\|pathing-entity-reorient-and-stride-tracking\|pathing-entity-focus-and-step-tracking" --include="*.go" --include="*.md"` — every hit should reflect the new "permanent dead-API skip" framing.
- Confirm `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ` no longer appears in any `DEVIATION` block: `grep -rn "DEVIATION NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ" --include="*.go"` — expect empty.

### Steps

- [ ] **Step 2.1: Update DEVIATION block in `modules/world/npc_script.go`.** Locate the comment block at lines ~95-122 (immediately above `func (n *Npc) Teleport`). Replace the current RESIDUAL section with the new framing:

```go
// DEVIATION NAI-34 vs TS PathingEntity.teleport — closure status:
//
// CLOSED:
//   - D1 (level clamp to [0, 3]) — NAI-36-T7, both entities.
//   - D2 (unallocated-zone reject via IsZoneAllocated) — NAI-36-T7,
//     both entities.
//   - D5-Player (level-change → moveSpeed=INSTANT + jump=true) —
//     NAI-36-T7.
//   - D3-Player + D3-NPC (focus call from PathingEntity.ts:286-289) —
//     NAI-65, both entities.
//   - D4-Player (lastStepX = x-1; lastStepZ = z from
//     PathingEntity.ts:291-292) — NAI-65.
//
// RESIDUAL (permanent dead-API skips per NAI-66):
//   - D4-NPC: no lastStepX/Z fields on Npc. TS PathingEntity sets
//     these on Npc via inheritance, but no TS reader consumes them
//     on NPC instances (Player.ts:1201-1202 is the only TS reader,
//     scoped to Player only). Closure: requires upstream-TS NPC
//     consumer to materialise — until then, dead-API per
//     dead_api_polish.md.
//   - D5-NPC: no jump field on Npc. TS PathingEntity sets npc.jump
//     on level-change, but no TS NPC encoder reads it; Rust upstream
//     rsbuf::Npc (2004scape/rsbuf branch 225, src/npc.rs:3-29) has
//     no jump field either. Closure: requires upstream-TS NPC
//     encoder consumer to materialise.
//
// Both residuals are documented permanent dead-API skips, not
// blocked-on-future-work items.
//
// Body order (focus, refresh, tele=true) matches TS
// PathingEntity.ts:286-293.
```

- [ ] **Step 2.2: Update DEVIATION block in `pkg/script/active.go`.** Locate the comment block at lines ~600-625 (above the `Teleport(x, z, level int)` interface method). Replace the RESIDUAL + tracker line with:

```go
	// DEVIATION NAI-34 vs TS PathingEntity.teleport — closure status:
	//
	// CLOSED:
	//   - D1 (level clamp [0, 3]) — NAI-36-T7, both entities.
	//   - D2 (unallocated-zone reject) — NAI-36-T7, both entities.
	//   - D5-Player (level-change INSTANT/jump branch) — NAI-36-T7.
	//   - Player.Teleport order divergence (refresh-then-flag) — NAI-36-T7.
	//   - D3-Player + D3-NPC (focus call) — NAI-65.
	//   - D4-Player (lastStepX = x-1; lastStepZ = z) — NAI-65.
	//
	// RESIDUAL (permanent dead-API skips per NAI-66):
	//   - D4-NPC: no lastStepX/Z fields on Npc; TS itself has no
	//     NPC reader, so adding fields would be dead-API. Closure
	//     requires upstream-TS NPC consumer.
	//   - D5-NPC: no jump field on Npc; TS NPC encoders don't read
	//     it, and rsbuf upstream parity confirms (Rust npc.rs has no
	//     jump field). Closure requires upstream-TS NPC encoder
	//     consumer.
	//
	// NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ closed in NAI-66.
	//
	// See (n *Npc).Teleport doc comment in modules/world/npc_script.go
	Teleport(x, z, level int)
```

- [ ] **Step 2.3: Update memory `nai_followups.md`.** Append at the end of the file (or in the appropriate chronological position — match the existing pattern of "## NAI-NN — CLOSED <date>" sections at the end of the file):

```markdown
## NAI-66 — CLOSED 2026-05-02

**Scope:** `(*Player).reorient` + `(*Npc).reorient` port (TS PathingEntity.ts:349-361) wired into `Server.processInfo` before rsbuf compute, plus activation of `Player.SetInteraction` default arm (Loc/Obj targetX/Z fine-coord cache).

**Deviations closed:** `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ` — Player.SetInteraction default arm now writes `targetX = fine(target.x, width)`, `targetZ = fine(target.z, length)`; `(*Player).reorient` is the consumer. DEVIATION block at `modules/world/interaction.go:91-98` removed.

**Deviations reframed (no code change):**
- `NAI-34-D4-NPC` — From "blocked on consumer" → "permanent dead-API skip." TS PathingEntity sets `npc.lastStepX/Z` via inheritance, but no TS reader consumes them on NPC instances (only Player.ts:1201-1202 reads `lastStepX/Z`). Closure: requires upstream-TS NPC consumer.
- `NAI-34-D5-NPC` — From "blocked on rsbuf.Npc.Jump field" → "permanent dead-API skip." TS PathingEntity sets `npc.jump = true`, but no TS NPC encoder reads it; Rust upstream `rsbuf::Npc` (branch 225, npc.rs:3-29) has no jump field. Closure: requires upstream-TS NPC encoder consumer.

**Net deviation tally:** 14 (post-NAI-65) → **13** (post-NAI-66).

**Implementation shape:** Single TDD task with four cycles (Player.reorient + Player.SetInteraction default arm + Npc.reorient + processInfo wiring) + a CLOSE task. ~33 production LOC + ~110 test LOC across 13 new test cases:
- 5 cases for `(*Player).reorient` (player_reorient_test.go).
- 5 cases for `(*Npc).reorient` (npc_reorient_test.go).
- 2 cases for `Player.SetInteraction` Loc + Obj target writes (interaction_test.go; replaces obsolete `TestSetInteractionLocTargetDoesNotSetFaceEntity`).
- 1 wire test for `processInfo` ordering (rsbuf_per_tick_test.go).

**Memory entries applied (no changes needed, just citing for provenance):**
- `runescript_cadence.md` — full cadence followed.
- `dead_api_polish.md` — drove the D4-NPC + D5-NPC dead-API reframe (no field add).
- `plan_grep_helper_patterns.md` — `targetWidthLength` reused (not inlined).
- `ts_source_canonical_path.md` — only `LostCityRS/Engine-TS` cited.
- `flat_arg_signature_for_cross_lang_parity.md` — duplicated method on Player/Npc preferred over interface abstraction.
- `plan_test_coverage_crosscheck.md` — every reorient branch tested.
- `controller_preflight.md` — § "Pre-flight Verification" in plan.
- `retire_deviation_grep_all_comments.md` — tag-site doc-comments updated at every NAI-34-D4/D5-NPC mention (`npc_script.go`, `pkg/script/active.go`).
- `close_commit_memory_trailer.md` — close commit carries `Closes memory:` trailer.
- `ts_asymmetry_dual_pin.md` — implicit: stepsTaken==0 vs >0 dual-pin in reorient tests.

**Carry-forwards (still open after NAI-66):**
- `NAI-65-D-FOCUS-INSTANT-WIRE` — face-instant wire protocol; no driver yet.
- `NAI-34-D4-NPC` + `NAI-34-D5-NPC` — permanent dead-API skip; closure requires upstream-TS change.
- All other NAI-65 carry-forwards remain unchanged.
```

- [ ] **Step 2.4: Final cross-package test.** Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`. Expected: PASS.

- [ ] **Step 2.5: Verify no stale tag references.** Run: `grep -rn "DEVIATION NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ" --include="*.go"`. Expected: empty (DEVIATION block removed in Cycle B). Run: `grep -rn "blocked on" modules/world/npc_script.go pkg/script/active.go | grep -i "lastStep\|jump\|stride\|consumer"`. Expected: empty (replaced with "permanent dead-API skip" framing).

- [ ] **Step 2.6: Close commit.** Stage all files modified in Task 2:

```bash
git add modules/world/npc_script.go pkg/script/active.go
# Note: nai_followups.md lives outside the repo; commit it separately to the
# memory directory if your environment manages it as a separate file. For
# this codebase commit, the in-repo files are sufficient.

git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-66 — pathing-entity reorient port + NPC stride/jump dead-API closure

Scope: ports (*Player).reorient and (*Npc).reorient (TS
PathingEntity.ts:349-361), wires both into Server.processInfo before
rsbuf compute, and activates Player.SetInteraction default arm to
write targetX/Z for Loc/Obj targets.

Deviations closed:
  - NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ (Player.SetInteraction Loc/Obj
    default arm; consumer materialised by (*Player).reorient).

Deviations reframed (no code change):
  - NAI-34-D4-NPC: from "blocked on consumer" to "permanent dead-API
    skip." TS itself has no NPC reader for lastStepX/Z (only
    Player.ts:1201-1202 reads them, scoped to Player).
  - NAI-34-D5-NPC: from "blocked on rsbuf.Npc.Jump field" to
    "permanent dead-API skip." TS NPC encoders don't read npc.jump,
    and Rust upstream rsbuf::Npc has no jump field either
    (2004scape/rsbuf branch 225, npc.rs:3-29).

DEVIATION-block doc-comments updated at npc_script.go and
pkg/script/active.go to reflect the new "permanent dead-API skip"
framing.

Net deviation tally: 14 → 13.

Closes memory: nai_followups.md (NAI-66 close entry),
dead_api_polish.md, plan_grep_helper_patterns.md,
retire_deviation_grep_all_comments.md, controller_preflight.md,
ts_asymmetry_dual_pin.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2.7: Final state verification.** Run:
  - `git log --oneline -8` — expect 5 NAI-66 commits (Cycles A/B/C/D + close).
  - `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — PASS.
  - `git status` — clean.

---

## Self-Review (controller, post-plan)

Before dispatching the implementer:

**Spec coverage check:**
- Spec § 2.1.1 (Player.reorient port) → Cycle A ✓
- Spec § 2.1.2 (Npc.reorient port) → Cycle C ✓
- Spec § 2.1.3 (Player.SetInteraction default arm) → Cycle B ✓
- Spec § 2.1.4 (processInfo wiring) → Cycle D ✓
- Spec § 2.2 (doc-only reframe) → Task 2 Steps 2.1-2.3 ✓
- Spec § 6.1 (Player.reorient 5 tests) → Cycle A Step 1 ✓
- Spec § 6.2 (Npc.reorient 5 tests) → Cycle C Step 1 ✓
- Spec § 6.3 (Player.SetInteraction 2 tests) → Cycle B Step 1 ✓
- Spec § 6.4 (processInfo wire test) → Cycle D Step 1 ✓
- Spec § 7.2 (deviation tag-site reframes) → Task 2 Steps 2.1, 2.2 ✓

**Type/signature consistency check:**
- `coordgrid.Fine(int, int) int` — used identically in all 4 cycles ✓
- `targetWidthLength(entity) (int, int)` — used in Cycle B ✓
- `entity.Coords() (x, z, level int)` — used in Cycle B ✓
- `(*Player).focus(int, int, bool)` — exists, used in Cycle A ✓
- `(*Npc).focus(int, int, bool)` — exists, used in Cycle C ✓
- `(*Player).reorient` and `(*Npc).reorient` defined in Cycles A and C, called in Cycle D ✓

**Cross-cycle naming consistency:**
- Test file naming: `player_reorient_test.go`, `npc_reorient_test.go` (new). SetInteraction tests appended to existing `interaction_test.go`. Wire test appended to `rsbuf_per_tick_test.go` ✓
- Method names: `reorient()` (lowercase; package-private) — matches TS naming and the NAI-65 precedent for `focus()` ✓
- Comment-tag pattern `NAI-66:` used consistently ✓

**Helper patterns:**
- `plan_grep_helper_patterns.md` — `targetWidthLength` reused at Cycle B Step 3, not inlined ✓
- `enumerate_all_sites.md` — Task 2 Steps 2.1, 2.2 enumerate all DEVIATION-block tag sites (npc_script.go, active.go) ✓
- `controller_preflight.md` — § "Pre-flight Verification" runs grep+Read against HEAD before dispatch ✓

No gaps. Plan is dispatch-ready.
