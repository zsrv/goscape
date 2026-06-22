# NAI-65 Pathing-Entity Focus & Step-Tracking (Partial Closure) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close NAI-34-D3-Player, NAI-34-D3-NPC, and NAI-34-D4-Player by porting the `focus()` call and Player `lastStepX/Z` adjust from TS `PathingEntity.teleport`, while explicitly deferring D4-NPC, D5-NPC, and NAI-41 to a renamed carry-forward sub-spec.

**Architecture:** Three implementation tasks plus a close commit. T1 introduces `(*Player).focus(fx, fz, instant)` mirroring `(*Npc).focus`, then extends `(*Player).Teleport` with the `Face`+`MoveX`+`MoveZ`+`focus` block and the `lastStepX = x-1; lastStepZ = z` writes. T2 extends `(*Npc).Teleport` symmetrically with `coordgrid.Fine(..., n.size)`. T3 trims the now-stale DEVIATION blocks across four files. Each new dead-write site carries the `defensive_gate_doc_comment_label.md`-style "TS-faithful but currently dead" note.

**Tech Stack:** Go 1.26+. Existing helpers: `pkg/coordgrid.Face`, `pkg/coordgrid.MoveX`, `pkg/coordgrid.MoveZ`, `pkg/coordgrid.Fine`. Existing entity helpers: `(*Npc).focus` at `modules/world/npc_interaction.go:706`. Test scaffolding: `newTestServer`, `newTestClient`, `newPlayer` (already in `_test.go` files).

**Spec:** `docs/superpowers/specs/2026-05-01-nai-65-pathing-entity-focus-stride-partial-design.md`.

---

## Pre-flight grep targets

Per `enumerate_all_sites.md`, run these greps at plan-author time, before each task dispatch, and once more post-T3. Each must show the expected sites; any drift means the plan needs re-derivation.

```bash
# Tag references that T3 must trim/re-frame:
rg -n "NAI-34-D3|NAI-34-D4|NAI-34-D5|D3-Player|D3-NPC|D4-Player|D4-NPC|D5-NPC" pkg/ modules/ cmd/
# Free-text follow-up promoted to NAI-65-D-FOCUS-INSTANT-WIRE:
rg -n "face-instant" pkg/ modules/ cmd/
# Verify no other call site of (*Player).Teleport / (*Npc).Teleport that
# might break under the new focus/lastStep ordering:
rg -n "\.Teleport\(" pkg/ modules/ cmd/
```

Expected at HEAD (verified during plan-write):

- DEVIATION blocks at `modules/world/npc_script.go:96-127`, `modules/world/player_script.go:341-350`, `pkg/script/active.go:598-616`.
- Test-doc reference at `modules/world/npc_script_test.go:732`.
- `face-instant` at `modules/world/npc_interaction.go:705` only.
- `(*Npc).Teleport` callers: NPC_TELE handler (`pkg/script/handlers_npc.go`), wanderMode home-tele, patrolMode waypoint-tele (both in `modules/world/npc_interaction.go`).
- `(*Player).Teleport` callers: PLAYER_TELE handler in `pkg/script/handlers_player.go`, plus existing tests.

---

## File Structure

| File | Role | Change |
|---|---|---|
| `modules/world/player_script.go` | Player script-entry helpers including Teleport | Modify Teleport body (D3-Player + D4-Player); add `(*Player).focus` helper next to `FaceSquare` |
| `modules/world/player_script_test.go` | Player Teleport / FaceSquare tests | Add 4 new tests pinning helper + Teleport closure |
| `modules/world/npc_script.go` | Npc script-entry helpers including Teleport | Modify Teleport body (D3-NPC); trim DEVIATION block (T3) |
| `modules/world/npc_script_test.go` | Npc Teleport tests | Add 3 new tests; trim test-doc reference (T3) |
| `pkg/script/active.go` | ActiveNpc adapter doc-comment for Teleport | Trim DEVIATION block (T3) |
| `modules/world/interaction.go` | Player.SetInteraction with NAI-41 deviation comment | Update sub-spec name reference (T3) |

No new files created. All work lands in existing files because the helper (`(*Player).focus`) lives next to `(*Player).FaceSquare` for ergonomic locality (both face-orientation setters).

---

## Task 1 — Player.focus helper + Teleport closure (D3-Player + D4-Player)

**Files:**
- Modify: `modules/world/player_script.go:387` (insert helper after FaceSquare); `modules/world/player_script.go:351-385` (Teleport body)
- Test: `modules/world/player_script_test.go` (append at end)

**Pre-flight grep (run at task start, before any edit):**

```bash
rg -n "p\.faceAngleX|p\.faceAngleZ|func \(p \*Player\) focus" modules/world/
```

Expected: zero hits (no existing Player.focus method; `p.faceAngleX/Z` only initialized at NewPlayer line 391-392).

- [ ] **Step 1.1: Write the helper unit test (failing)**

Append to `modules/world/player_script_test.go` (after the existing test in the file's footer):

```go
// TestPlayerFocus_HelperWritesFaceAngleOnly pins NAI-65 D3-Player helper
// shape. instant=false sets faceAngleX/Z only — does NOT touch
// faceSquareX/Z or masks. instant=true is currently write-only too,
// matching (*Npc).focus and tracked under NAI-65-D-FOCUS-INSTANT-WIRE.
func TestPlayerFocus_HelperWritesFaceAngleOnly(t *testing.T) {
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.faceAngleX = -1
	p.faceAngleZ = -1
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0

	p.focus(123, 456, false)

	if p.faceAngleX != 123 || p.faceAngleZ != 456 {
		t.Errorf("instant=false faceAngle: got (%d, %d), want (123, 456)", p.faceAngleX, p.faceAngleZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("instant=false faceSquare: got (%d, %d), want (-1, -1) unchanged", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks != 0 {
		t.Errorf("instant=false masks: got %d, want 0 unchanged", p.masks)
	}

	// instant=true: same outcome at HEAD per NAI-65-D-FOCUS-INSTANT-WIRE.
	// Per ts_asymmetry_dual_pin.md, dual-pin both branches so that a future
	// closure of the wire-protocol sub-spec breaks this test loudly.
	p.focus(789, 1011, true)
	if p.faceAngleX != 789 || p.faceAngleZ != 1011 {
		t.Errorf("instant=true faceAngle: got (%d, %d), want (789, 1011)", p.faceAngleX, p.faceAngleZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("instant=true faceSquare: got (%d, %d), want (-1, -1) — flag is currently write-only (NAI-65-D-FOCUS-INSTANT-WIRE)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks != 0 {
		t.Errorf("instant=true masks: got %d, want 0 — flag is currently write-only (NAI-65-D-FOCUS-INSTANT-WIRE)", p.masks)
	}
}
```

- [ ] **Step 1.2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerFocus_HelperWritesFaceAngleOnly -v
```

Expected: compile failure — `p.focus undefined (type *Player has no field or method focus)`.

- [ ] **Step 1.3: Add the `(*Player).focus` helper**

Insert in `modules/world/player_script.go` BEFORE `FaceSquare` (which currently sits at line 387-393). Place the new helper at the location currently occupied by FaceSquare's doc comment, then FaceSquare follows. Exact body:

```go
// focus records the fine-grained face-angle target. Mirrors TS
// PathingEntity.focus (Engine-TS/src/engine/entity/PathingEntity.ts:321-333).
// Called from Teleport (NAI-65 D3-Player closure) and intended for future
// non-Teleport callers (e.g. SetInteraction's Engine-clicked Loc/Obj
// branch when NAI-41 closes).
//
// DEVIATION NAI-65-D-FOCUS-INSTANT-WIRE: TS focus(_, _, client=true) ALSO
// writes faceSquareX/Z and ORs the coord mask into masks. Goscape's wire
// protocol doesn't currently branch on it, so the flag is accepted for
// signature parity but stored write-only. Mirror site: (*Npc).focus
// (npc_interaction.go:706). Closure: future "face-instant wire protocol"
// sub-spec.
func (p *Player) focus(fx, fz int, instant bool) {
	p.faceAngleX = fx
	p.faceAngleZ = fz
	_ = instant
}
```

- [ ] **Step 1.4: Run helper test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerFocus_HelperWritesFaceAngleOnly -v
```

Expected: PASS.

- [ ] **Step 1.5: Write the Teleport-focus + lastStep failing tests**

Append to `modules/world/player_script_test.go`:

```go
// TestPlayerTeleport_FocusFromDirection pins NAI-65 D3-Player. Teleport
// from (3200, 3200, 0) to (3300, 3300, 0): direction is NE, so MoveX/MoveZ
// each return prevDest+1. faceAngleX = Fine(3301, 1) = 3301*64 + (1*64-1)/2
// = 211264 + 31 = 211295. Mirrors TS PathingEntity.ts:286-289.
func TestPlayerTeleport_FocusFromDirection(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.faceAngleX = -1
	p.faceAngleZ = -1

	p.Teleport(3300, 3300, 0)

	wantX := 3301*64 + 31
	wantZ := 3301*64 + 31
	if p.faceAngleX != wantX {
		t.Errorf("faceAngleX after Teleport(NE): got %d, want %d (Fine(3301, 1))", p.faceAngleX, wantX)
	}
	if p.faceAngleZ != wantZ {
		t.Errorf("faceAngleZ after Teleport(NE): got %d, want %d (Fine(3301, 1))", p.faceAngleZ, wantZ)
	}
}

// TestPlayerTeleport_LastStepAdjust pins NAI-65 D4-Player. After Teleport,
// p.lastStepX = p.x - 1 and p.lastStepZ = p.z per TS PathingEntity.ts:291-292.
func TestPlayerTeleport_LastStepAdjust(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.lastStepX = -999
	p.lastStepZ = -999

	p.Teleport(3300, 3300, 0)

	if p.lastStepX != 3299 {
		t.Errorf("lastStepX after Teleport: got %d, want 3299 (x - 1)", p.lastStepX)
	}
	if p.lastStepZ != 3300 {
		t.Errorf("lastStepZ after Teleport: got %d, want 3300 (z)", p.lastStepZ)
	}
}

// TestPlayerTeleport_InPlaceFocusUsesSelfCenter pins the in-place edge case.
// When prev == new, coordgrid.Face returns -1; coordgrid.MoveX/MoveZ no-op
// (DeltaX/Z default-case = 0). focus uses self-center coords:
// Fine(p.x, 1), Fine(p.z, 1). lastStep adjust still applies (x-1, z).
func TestPlayerTeleport_InPlaceFocusUsesSelfCenter(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.faceAngleX = -1
	p.faceAngleZ = -1

	p.Teleport(3200, 3200, 0)

	wantSelf := 3200*64 + 31
	if p.faceAngleX != wantSelf {
		t.Errorf("in-place faceAngleX: got %d, want %d (Fine(3200, 1) self-center)", p.faceAngleX, wantSelf)
	}
	if p.faceAngleZ != wantSelf {
		t.Errorf("in-place faceAngleZ: got %d, want %d (Fine(3200, 1) self-center)", p.faceAngleZ, wantSelf)
	}
	if p.lastStepX != 3199 {
		t.Errorf("in-place lastStepX: got %d, want 3199 (x - 1 still applies)", p.lastStepX)
	}
	if p.lastStepZ != 3200 {
		t.Errorf("in-place lastStepZ: got %d, want 3200", p.lastStepZ)
	}
	if !p.tele {
		t.Error("in-place tele flag: got false, want true")
	}
}
```

- [ ] **Step 1.6: Run new Teleport tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerTeleport_FocusFromDirection|TestPlayerTeleport_LastStepAdjust|TestPlayerTeleport_InPlaceFocusUsesSelfCenter" -v
```

Expected: ALL FAIL — `faceAngleX = -1` (helper never called from Teleport); `lastStepX = -999 / -1` (never written by Teleport).

- [ ] **Step 1.7: Wire focus + lastStep into Player.Teleport**

Edit `modules/world/player_script.go`. The current Teleport body sits at lines 351-385. The new ordered body (replacing existing lines 366-385) — keep lines 351-365 untouched (clamp + zone-allocated reject + comment block):

```go
	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.x = x
	p.z = z
	p.level = level

	// NAI-65 D3-Player: focus call from TS PathingEntity.ts:286-289.
	// Player width=length=1 (no struct field; PathingEntity-default).
	dir := coordgrid.Face(prevX, prevZ, x, z)
	moveX := coordgrid.MoveX(p.x, dir)
	moveZ := coordgrid.MoveZ(p.z, dir)
	p.focus(coordgrid.Fine(moveX, 1), coordgrid.Fine(moveZ, 1), false)

	// Order: refreshPlayerZone runs BEFORE p.tele = true to match TS
	// PathingEntity.ts:290-293. The two writes are functionally
	// commutative (refresh reads only previous coords + current
	// x/z/level; the tele bit is independent), but TS-faithful order is
	// the project's true-to-TS gate default.
	refreshPlayerZone(p, prevX, prevZ, prevLevel)

	// NAI-65 D4-Player: lastStep adjust from TS PathingEntity.ts:291-292.
	// Currently dead-write at HEAD (no production reader of
	// p.lastStepX/Z besides the dead-write of p.followX/Z in
	// processInteraction). Tracked.
	p.lastStepX = p.x - 1
	p.lastStepZ = p.z

	p.tele = true

	// D5: level-change → INSTANT + jump per PathingEntity.ts:295-298.
	if prevLevel != level {
		p.moveSpeed = MoveSpeedInstant
		p.jump = true
	}
}
```

The helper-emitted block above replaces the existing comment-block-plus-body from line 366 down to (but not including) the closing brace at line 385. The closing brace stays where it was.

Add `coordgrid` to the import block at the top of `modules/world/player_script.go`:

```go
import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)
```

Also update the doc-comment block immediately above `func (p *Player) Teleport` at line 341-350. Replace the current "RESIDUAL: D3 (focus orientation), D4 (lastStepX/Z adjust)" line with a CLOSED-by-NAI-65 note. New doc-comment text (exact text replacing lines 341-350):

```go
// Teleport moves the player to (x, z, level) and flags the client for a
// smooth teleport transition (tele without jump in the same-level case;
// tele+jump+INSTANT speed when crossing levels). Mirrors TS
// PathingEntity.teleport at PathingEntity.ts:267-298.
//
// NAI-36-T7 closed D1 (level clamp), D2 (unallocated-zone reject), order
// (refresh BEFORE tele=true), and D5 (level-change INSTANT + jump branch)
// for Player. NAI-65 closed D3-Player (focus call) and D4-Player
// (lastStepX = x-1; lastStepZ = z). See DEVIATION block at npc_script.go
// for the full tracker; D4-NPC, D5-NPC, and NAI-41 remain residual.
```

- [ ] **Step 1.8: Run all four Player Teleport tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestPlayerFocus_HelperWritesFaceAngleOnly|TestPlayerTeleport_FocusFromDirection|TestPlayerTeleport_LastStepAdjust|TestPlayerTeleport_InPlaceFocusUsesSelfCenter" -v
```

Expected: all PASS.

- [ ] **Step 1.9: Run the full modules/world package to ensure no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. Pay particular attention to `TestPlayerTeleport_LevelChangeSetsInstantAndJump` and `TestPlayerTeleport_OrderRefreshThenFlag` — they should still pass unchanged.

- [ ] **Step 1.10: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 1.11: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-65 T1 — Player.focus helper + Teleport D3/D4 closure

Adds (*Player).focus(fx, fz, instant) mirroring (*Npc).focus, then
extends (*Player).Teleport with the Face+MoveX+MoveZ+focus block and
lastStepX = x-1; lastStepZ = z writes per TS PathingEntity.ts:286-292.
Closes NAI-34-D3-Player and NAI-34-D4-Player. Both writes are
TS-faithful but currently dead at HEAD (no production reader of
faceAngleX/Z; lastStepX/Z reads only via the also-dead followX/Z
chain in processInteraction). Tracked.

Opens NAI-65-D-FOCUS-INSTANT-WIRE: the new (*Player).focus and the
existing (*Npc).focus both store the `instant` parameter write-only;
TS focus(_, _, client=true) would also write faceSquareX/Z and OR
the coord mask into masks. T3 will surface this in (*Npc).focus's
doc-comment too.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — Npc.Teleport closure (D3-NPC)

**Files:**
- Modify: `modules/world/npc_script.go:128-144` (Teleport body)
- Test: `modules/world/npc_script_test.go` (append after existing Teleport tests, before the NAI-36 commented block at line 729)

**Pre-flight grep (run at task start):**

```bash
rg -n "n\.faceAngleX|n\.faceAngleZ" modules/world/npc_script.go modules/world/npc.go
```

Expected: only `npc.go:108-109` (field decl) and `npc.go:182-183` (NewNpc init). Npc.Teleport must currently NOT touch faceAngle.

- [ ] **Step 2.1: Write the failing tests**

Append to `modules/world/npc_script_test.go` immediately AFTER `TestNpcTeleport_NilServerNoOp` (line 727) and BEFORE the `// --- NAI-36 Task 7: ...` comment block at line 729:

```go
// TestNpcTeleport_FocusFromDirection pins NAI-65 D3-NPC. Teleport from
// (3200, 3200, 0) to (3300, 3300, 0) for a size=1 NPC: dir=NE, moveX=3301,
// moveZ=3301. faceAngleX = Fine(3301, 1) = 3301*64 + 31 = 211295.
// Mirrors TS PathingEntity.ts:286-289.
func TestNpcTeleport_FocusFromDirection(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, size: 1, startX: 3200, startZ: 3200, startLevel: 0, faceAngleX: -1, faceAngleZ: -1}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	n.Teleport(3300, 3300, 0)

	wantX := 3301*64 + 31
	wantZ := 3301*64 + 31
	if n.faceAngleX != wantX {
		t.Errorf("faceAngleX after Teleport(NE): got %d, want %d (Fine(3301, 1))", n.faceAngleX, wantX)
	}
	if n.faceAngleZ != wantZ {
		t.Errorf("faceAngleZ after Teleport(NE): got %d, want %d (Fine(3301, 1))", n.faceAngleZ, wantZ)
	}
}

// TestNpcTeleport_FocusSize2 pins the size>1 path so a refactor that drops
// `n.size` to a literal `1` regresses. Fine(3301, 2) = 3301*64 + (2*64-1)/2
// = 211264 + 63 = 211327.
func TestNpcTeleport_FocusSize2(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, size: 2, startX: 3200, startZ: 3200, startLevel: 0, faceAngleX: -1, faceAngleZ: -1}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	n.Teleport(3300, 3300, 0)

	wantX := 3301*64 + 63
	wantZ := 3301*64 + 63
	if n.faceAngleX != wantX {
		t.Errorf("size=2 faceAngleX: got %d, want %d (Fine(3301, 2))", n.faceAngleX, wantX)
	}
	if n.faceAngleZ != wantZ {
		t.Errorf("size=2 faceAngleZ: got %d, want %d (Fine(3301, 2))", n.faceAngleZ, wantZ)
	}
}

// TestNpcTeleport_InPlaceFocusUsesSelfCenter pins the in-place edge case.
// prev == new → Face returns -1 → MoveX/MoveZ no-op → focus uses
// self-center coords. tele still flags true.
func TestNpcTeleport_InPlaceFocusUsesSelfCenter(t *testing.T) {
	s := newTestServer(t)
	n := &Npc{nid: 0, typeId: 0, x: 3200, z: 3200, level: 0, size: 1, startX: 3200, startZ: 3200, startLevel: 0, faceAngleX: -1, faceAngleZ: -1}
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	n.tele = false

	n.Teleport(3200, 3200, 0)

	wantSelf := 3200*64 + 31
	if n.faceAngleX != wantSelf {
		t.Errorf("in-place faceAngleX: got %d, want %d (Fine(3200, 1) self-center)", n.faceAngleX, wantSelf)
	}
	if n.faceAngleZ != wantSelf {
		t.Errorf("in-place faceAngleZ: got %d, want %d (Fine(3200, 1) self-center)", n.faceAngleZ, wantSelf)
	}
	if !n.tele {
		t.Error("in-place tele flag: got false, want true")
	}
}
```

- [ ] **Step 2.2: Run new tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNpcTeleport_FocusFromDirection|TestNpcTeleport_FocusSize2|TestNpcTeleport_InPlaceFocusUsesSelfCenter" -v
```

Expected: all FAIL — `faceAngleX = -1` (Teleport never calls focus).

- [ ] **Step 2.3: Wire focus into Npc.Teleport**

Edit `modules/world/npc_script.go`. Current body (lines 128-144) replaced as:

```go
func (n *Npc) Teleport(x, z, level int) {
	// D1: clamp level to [0, 3] per PathingEntity.ts:268-271.
	if level < 0 {
		level = 0
	} else if level > 3 {
		level = 3
	}
	// D2: reject teleports to unallocated zones per PathingEntity.ts:273-278.
	if n.server != nil && !n.server.IsZoneAllocated(level, x, z) {
		return
	}

	prevX, prevZ, prevLevel := n.x, n.z, n.level
	n.x, n.z, n.level = x, z, level

	// NAI-65 D3-NPC: focus call from TS PathingEntity.ts:286-289.
	// Npc width=length=size (square; from typ.Size at NewNpc).
	dir := coordgrid.Face(prevX, prevZ, x, z)
	moveX := coordgrid.MoveX(n.x, dir)
	moveZ := coordgrid.MoveZ(n.z, dir)
	n.focus(coordgrid.Fine(moveX, n.size), coordgrid.Fine(moveZ, n.size), false)

	refreshNpcZone(n.server, n, prevX, prevZ, prevLevel)
	n.tele = true
}
```

Add `coordgrid` to the import block at the top of `modules/world/npc_script.go`:

```go
import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)
```

(T3 handles the doc-comment trim above `func (n *Npc) Teleport`. Don't touch lines 87-127 in this task — those are the DEVIATION block, retired in T3.)

- [ ] **Step 2.4: Run the new tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNpcTeleport_FocusFromDirection|TestNpcTeleport_FocusSize2|TestNpcTeleport_InPlaceFocusUsesSelfCenter" -v
```

Expected: all PASS.

- [ ] **Step 2.5: Run all NPC Teleport tests to ensure no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNpcTeleport" -v
```

Expected: PASS. Pay attention to existing tests `TestNpcTeleport_SetsFieldsAndTeleFlag`, `TestNpcTeleport_CrossZoneRefreshSubscription`, `TestNpcTeleport_SameZoneNoRefresh`, `TestNpcTeleport_NilServerNoOp`, `TestNpcTeleport_LevelClampNegative`, `TestNpcTeleport_LevelClampHigh`, `TestNpcTeleport_UnallocatedZoneRejects` — they should not have been perturbed.

- [ ] **Step 2.6: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 2.7: Commit**

```bash
git add modules/world/npc_script.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-65 T2 — Npc.Teleport D3-NPC closure

Extends (*Npc).Teleport with the Face+MoveX+MoveZ+focus block per TS
PathingEntity.ts:286-289. Width=length=n.size (square Npc; from
typ.Size at NewNpc). Closes NAI-34-D3-NPC. Currently dead-write at
HEAD (no rsbuf reader of n.faceAngleX/Z), tracked.

D4-NPC (no lastStepX/Z field) and D5-NPC (no jump field) remain
residual; T3 reframes them in the DEVIATION block. NAI-41 remains
deferred to a future reorient port.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — DEVIATION-block trim across reference sites

**Files:**
- Modify: `modules/world/npc_script.go:87-127` (Teleport doc-block)
- Modify: `modules/world/player_script.go:341-350` (Teleport doc-block — already trimmed in T1; verify only)
- Modify: `pkg/script/active.go:598-616` (Teleport adapter doc-block)
- Modify: `modules/world/npc_script_test.go:729-733` (test-doc reference block)
- Modify: `modules/world/interaction.go:92-98` (NAI-41 deviation comment — sub-spec name reference only)

**Pre-flight grep (run at task start):**

```bash
rg -n "D3-Player|D3-NPC|D4-Player" pkg/ modules/ cmd/
rg -n "pathing-entity-focus-and-step-tracking|focus/step-tracking|focus/\\s*step-tracking" pkg/ modules/ cmd/
```

Expected: matches the four DEVIATION-block sites listed above plus the NAI-41 site at `interaction.go:92-98`.

- [ ] **Step 3.1: Trim `modules/world/npc_script.go:87-127`**

Replace the entire DEVIATION-block-plus-doc spanning lines 87-127 with:

```go
// Teleport moves the NPC to (x, z, level), refreshes its zone
// subscription if the zone changed, and flags the client for a tele
// transition (no walk-anim interpolation). Mirrors TS
// PathingEntity.teleport at PathingEntity.ts:267-298.
//
// Used by NPC_TELE script handler (pkg/script/handlers_npc.go) and by
// AI teleport sites — wanderMode home-tele (npc_interaction.go ~:95)
// and patrolMode waypoint-tele (~:121).
//
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
// RESIDUAL:
//   - D4-NPC: no lastStepX/Z fields on Npc. Adding is dead-API per
//     dead_api_polish.md until an NPC stride-tracking consumer ports.
//     Blocked on: NPC stride-tracking consumer (e.g. NPC_LASTSTEP-style
//     opcode or AI movement code that reads stride state).
//   - D5-NPC: no jump field on Npc; pkg/rsbuf/npc.go:15-33 Npc struct
//     has no Jump field either, mirroring upstream Rust npc.rs:3-29.
//     Blocked on: rsbuf.Npc.Jump field + npcinfo encoder branch
//     (would diverge from upstream rsbuf parity).
//
// Both residual items are tracked for the future
// "pathing-entity-reorient-and-stride-tracking" sub-spec.
//
// Body order (focus, refresh, tele=true) matches TS
// PathingEntity.ts:286-293.
```

- [ ] **Step 3.2: Verify Player Teleport doc-block was already trimmed in T1**

Read `modules/world/player_script.go` lines 341-350. Confirm the block reads:

```
// Teleport moves the player to (x, z, level) and flags the client for a
// smooth teleport transition (tele without jump in the same-level case;
// tele+jump+INSTANT speed when crossing levels). Mirrors TS
// PathingEntity.teleport at PathingEntity.ts:267-298.
//
// NAI-36-T7 closed D1 (level clamp), D2 (unallocated-zone reject), order
// (refresh BEFORE tele=true), and D5 (level-change INSTANT + jump branch)
// for Player. NAI-65 closed D3-Player (focus call) and D4-Player
// (lastStepX = x-1; lastStepZ = z). See DEVIATION block at npc_script.go
// for the full tracker; D4-NPC, D5-NPC, and NAI-41 remain residual.
```

If correct, no edit needed. Otherwise: edit it to match.

- [ ] **Step 3.3: Trim `pkg/script/active.go:598-624`**

Replace the existing DEVIATION block (lines 598-624) with:

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
	// RESIDUAL:
	//   - D4-NPC: no lastStepX/Z fields on Npc; dead-API until an NPC
	//     stride-tracking consumer ports.
	//   - D5-NPC: no jump field on Npc; rsbuf upstream parity blocks
	//     until an npcinfo encoder branch ports.
	//
	// Tracked under "pathing-entity-reorient-and-stride-tracking"
	// sub-spec (also bundles NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ).
	//
	// See (n *Npc).Teleport doc comment in modules/world/npc_script.go
```

(Verify the closing line "See (n *Npc).Teleport doc comment ..." matches what was there before. Lines 623-624 in HEAD already contain that closer; preserve it.)

- [ ] **Step 3.4: Trim `modules/world/npc_script_test.go:729-733`**

Replace the existing test-doc block at lines 729-733 with:

```go
// --- NAI-36 Task 7 + NAI-65: Npc.Teleport parity status -----------------
//
// Closed: D1 (level clamp), D2 (unallocated-zone reject) — NAI-36-T7.
// Closed: D3-NPC (focus call) — NAI-65.
// Residual: D4-NPC (no lastStepX/Z fields), D5-NPC (no jump field).
// See DEVIATION block in npc_script.go for full tracker.
```

- [ ] **Step 3.5: Update the NAI-41 sub-spec name reference at `modules/world/interaction.go:92-98`**

Replace the existing comment block at lines 92-98 with:

```go
		// DEVIATION NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ: TS L542-545 sets
		// targetX = CoordGrid.fine(target.x, target.width) and targetZ
		// analogously for *Loc/*Obj targets. Player has no targetX/Z
		// consumer at HEAD (the only TS reader is reorient(), unported).
		// Deferred to the future
		// "pathing-entity-reorient-and-stride-tracking" sub-spec.
```

- [ ] **Step 3.6: Verification grep — no RESIDUAL D3 or D4-Player references remain**

```bash
rg -n "D3-Player|D3-NPC|D4-Player" pkg/ modules/ cmd/
```

Expected hits: ALL must be inside CLOSED-block listings or test-name strings (e.g. `TestNpcTeleport_FocusFromDirection` may incidentally match if it referenced D3-NPC in a comment — verify each hit is allowed). Any RESIDUAL framing is a regression.

```bash
rg -n "pathing-entity-focus-and-step-tracking" pkg/ modules/ cmd/
```

Expected: zero hits. The old sub-spec name has been fully renamed.

```bash
rg -n "RESIDUAL.*D3|RESIDUAL.*D4-Player" pkg/ modules/ cmd/
```

Expected: zero hits.

- [ ] **Step 3.7: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. (T3 is comment-only; no behavior change; nothing should break.)

- [ ] **Step 3.8: Commit**

```bash
git add modules/world/npc_script.go modules/world/player_script.go pkg/script/active.go modules/world/npc_script_test.go modules/world/interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(world): NAI-65 T3 — DEVIATION-block trim across all sites

Per-instance Edit (no replace_all per plan_doc_replaceall_timeline.md)
on each commenting site: trims D3-Player, D3-NPC, D4-Player from
RESIDUAL to CLOSED-by-NAI-65; reframes D4-NPC + D5-NPC with sharper
"blocked-on-X" wording; renames the carry-forward sub-spec from
"pathing-entity-focus-and-step-tracking" to
"pathing-entity-reorient-and-stride-tracking" (NAI-41 also bundles
into the renamed carry-forward).

Verified post-trim with `rg "D3-Player|D3-NPC|D4-Player|RESIDUAL.*D3|
pathing-entity-focus-and-step-tracking"` — zero RESIDUAL matches.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — CLOSE: close commit + memory updates

**Files:**
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — append `## NAI-65 — CLOSED <date>` block; update the carry-forward list.

- [ ] **Step 4.1: Append NAI-65 close block to `nai_followups.md`**

Append (after the existing NAI-64 block, before the carry-forward list at line 3177-3186):

```markdown
## NAI-65 — CLOSED <today>

**Scope:** `(*Player).focus` helper + Player.Teleport D3-Player/D4-Player closure + Npc.Teleport D3-NPC closure. Closes NAI-34-D3 (both entities) + NAI-34-D4-Player. Opens NAI-65-D-FOCUS-INSTANT-WIRE (formalizes the existing free-text "face-instant wire protocol" follow-up at npc_interaction.go:705 across both Player.focus and Npc.focus).

**Cadence:** Full sub-spec, single bundle, 3 implementation tasks + 1 close. ~21 production LOC + ~75 test LOC + ~30 LOC of comment churn across 5 files (modules/world/player_script.go, npc_script.go, player_script_test.go, npc_script_test.go, pkg/script/active.go, modules/world/interaction.go).

**Spec:** `docs/superpowers/specs/2026-05-01-nai-65-pathing-entity-focus-stride-partial-design.md`.
**Plan:** `docs/superpowers/plans/2026-05-01-nai-65-pathing-entity-focus-stride-partial.md`.

**Close commit:** (this commit). T1: `<T1-SHA>`. T2: `<T2-SHA>`. T3: `<T3-SHA>`.

**Follow-ups closed:**
- NAI-34-D3-Player — `(*Player).Teleport` now calls `focus(Fine(moveX, 1), Fine(moveZ, 1), false)` per TS PathingEntity.ts:286-289.
- NAI-34-D3-NPC — `(*Npc).Teleport` now calls `focus(Fine(moveX, n.size), Fine(moveZ, n.size), false)` per same TS lines.
- NAI-34-D4-Player — `(*Player).Teleport` now writes `lastStepX = p.x - 1; lastStepZ = p.z` per TS PathingEntity.ts:291-292.

**Deviations opened:** `NAI-65-D-FOCUS-INSTANT-WIRE` — both `(*Player).focus` and `(*Npc).focus` store the `instant` parameter write-only; TS `focus(_, _, client=true)` would also write `faceSquareX/Z` and OR the coord mask into masks. Two sites, both doc-comment-tagged. Closure: future "face-instant wire protocol" sub-spec when a non-Teleport caller (e.g. SetInteraction's Engine-clicked Loc/Obj branch) passes `instant=true`.

**Deviations closed:** NAI-34-D3-Player, NAI-34-D3-NPC, NAI-34-D4-Player.

**Deviation tally:** -3 closures, +1 open = net -2 from NAI-64 close.

**Wire-behaviour delta:** None at HEAD. Both `focus()` sites and the new Player.lastStepX/Z writes target fields with no production reader. The closure is purely TS-shape correctness work; future ports of `reorient()`, rsbuf.Npc.Jump, and wire-side faceSquare-from-focus will read the now-correct state without a migration step.

**Memory entries reinforced (no edits needed):**
- `runescript_cadence.md` — full cadence, 3-task TDD bundle.
- `true_to_ts_gate.md` — every behavioural change cited against TS source.
- `dead_api_polish.md` — D4-NPC + D5-NPC + NAI-41 remain deferred (no consumer); D3 (both) + D4-Player closed because target fields exist on the entity at HEAD.
- `defensive_gate_doc_comment_label.md` — new dead-write sites doc-labeled.
- `enumerate_all_sites.md` — pre-flight grep targets enumerated and re-greped post-T3.
- `retire_deviation_grep_all_comments.md` — T3 ended with the verifying grep; zero RESIDUAL hits.
- `plan_doc_replaceall_timeline.md` — T3 used per-instance Edit, not replace_all.
- `plan_var_name_collision.md` — `dir`/`moveX`/`moveZ` locals checked against enclosing scope.
- `plan_test_coverage_crosscheck.md` — applied at plan-write time.
- `ts_asymmetry_dual_pin.md` — TestPlayerFocus_HelperWritesFaceAngleOnly pins both `instant=false` and `instant=true` to assert the write-only flag.
- `close_commit_memory_trailer.md` — close commit carries `Closes memory:` trailer.

**Carry-forwards (still open after NAI-65):**
- `pathing-entity-reorient-and-stride-tracking` sub-spec — bundles:
  - `NAI-34-D4-NPC` — blocked on NPC stride-tracking consumer (no lastStepX/Z field).
  - `NAI-34-D5-NPC` — blocked on `rsbuf.Npc.Jump` field + npcinfo encoder branch (upstream Rust `npc.rs:3-29` parity).
  - `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ` — blocked on `(*Player).reorient` port (only TS reader of Player.targetX/Z).
- `NAI-65-D-FOCUS-INSTANT-WIRE` — future "face-instant wire protocol" sub-spec.
- NAI-35-T3-D1 op[1] operability gate audit (conditional).
- AI-tick walktrigger consumption (NAI-37-D-WALKTRIGGER-NOREADER + NAI-44-D-PLAYER-WALKTRIGGER-NOOP).
- NAI-40-SB1 OPCALLED convergence (blocked on World.ts:613-642 port).
- NAI-40-SB2 FINDHERO + BOTH_HEROPOINTS (blocked on HeroPoints + hash64 infra).
- NAI-40-SB4 slot-reuse / target-logout detection (defensive-only).
- NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET (blocked on `p_op*` reshape).
- NAI-44-D-CANACCESS-NO-STUN-CHECK (blocked on stun system port).
- NAI-59-D-MODALTUTORIAL-NO-PRODUCER (conditional on tutorial-content driver).
```

Replace `<today>` with the actual date (e.g. `2026-05-01`); replace the `<T1-SHA>`/`<T2-SHA>`/`<T3-SHA>` placeholders with the actual commit SHAs from `git log --oneline -4` at this point.

Also update the carry-forwards list at the bottom of `nai_followups.md` (the ten-bullet block starting at the existing line 3177): rename the first bullet from "pathing-entity-focus-and-step-tracking" to "pathing-entity-reorient-and-stride-tracking", with bundled deviations listed as `NAI-34-D4-NPC`, `NAI-34-D5-NPC`, `NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ`. Add a new bullet for `NAI-65-D-FOCUS-INSTANT-WIRE`.

- [ ] **Step 4.2: Run the full test suite one final time**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 4.3: Final pre-flight grep verification**

```bash
rg -n "D3-Player|D3-NPC|D4-Player" pkg/ modules/ cmd/
rg -n "pathing-entity-focus-and-step-tracking" pkg/ modules/ cmd/
rg -n "NAI-65-D-FOCUS-INSTANT-WIRE" pkg/ modules/ cmd/
```

Expected:
- First grep: zero RESIDUAL framings; only CLOSED-tagged or test-name hits.
- Second grep: zero hits.
- Third grep: at least 2 hits (Player.focus and Npc.focus doc-comments).

- [ ] **Step 4.4: Compose and commit the close commit**

`nai_followups.md` is a memory file (outside the repo working tree) — it doesn't go into git. The close commit captures the same provenance in its message body.

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-65 — pathing-entity focus & step-tracking partial closure

Closes NAI-34-D3 (both entities) + NAI-34-D4-Player. Opens
NAI-65-D-FOCUS-INSTANT-WIRE to formalize the existing free-text
"face-instant wire protocol" follow-up across both (*Player).focus
and (*Npc).focus.

Re-frames the residual carry-forward as
"pathing-entity-reorient-and-stride-tracking" (bundles NAI-34-D4-NPC,
NAI-34-D5-NPC, NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ).

Wire-behaviour delta: none at HEAD. Both new focus() sites and the
Player.lastStepX/Z writes target fields with no production reader.
The closure is purely TS-shape correctness work; future ports of
reorient(), rsbuf.Npc.Jump, and wire-side faceSquare-from-focus will
read the now-correct state without a migration step.

T1: <T1-SHA> — Player.focus helper + Teleport D3/D4 closure.
T2: <T2-SHA> — Npc.Teleport D3-NPC closure.
T3: <T3-SHA> — DEVIATION-block trim across all sites.

Closes memory: nai_followups.md (NAI-65 entry).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Replace the SHA placeholders with the actual commit hashes from `git log --oneline -4`.

---

## Self-review (plan-author check, run before dispatch)

1. **Spec coverage** — every spec section has a task:
   - § Goal: T1 + T2 + T3 cover all three closures.
   - § Non-goals: explicitly preserved (T3 trim keeps D4-NPC, D5-NPC, NAI-41 as residual).
   - § TS reference: T1 + T2 code blocks port the body verbatim.
   - § Edge cases: TestPlayerTeleport_InPlaceFocusUsesSelfCenter + TestNpcTeleport_InPlaceFocusUsesSelfCenter pin in-place; TestNpcTeleport_FocusSize2 pins width/length.
   - § Per-deviation closure plan: 1:1 mapping to T1/T2/T3.
   - § Test strategy: 4 Player tests + 3 NPC tests, all covered in T1/T2 step blocks.
   - § Deviation tags: opened (NAI-65-D-FOCUS-INSTANT-WIRE) — covered in T1 (helper doc-comment) + T2 (no helper change but the n.focus call site references it via the existing comment); closed (D3-Player, D3-NPC, D4-Player) — covered in T1+T2; reframed (D4-NPC, D5-NPC, NAI-41) — covered in T3.
   - § Wire-behaviour delta: documented in T1's commit message + T4's close block.
   - § Risk register: pre-flight greps in each task.
   - § Cadence: 3+1 confirmed.

2. **Placeholder scan** — `<today>`, `<T1-SHA>`, `<T2-SHA>`, `<T3-SHA>` are intentional placeholders to be filled at execution time (T4 instructs the implementer to replace them). No "TBD"/"TODO" left in code blocks.

3. **Type consistency** — verified `(*Player).focus(fx, fz int, instant bool)` matches `(*Npc).focus(fx, fz int, instant bool)` from `npc_interaction.go:706`. `coordgrid.Direction` typed return from `Face` accepted by `MoveX`/`MoveZ` (both take `Direction`). `n.size` is `int` (not `int32`) per `npc.go:117` — no cast needed.

4. **Plan-test-coverage crosscheck (`plan_test_coverage_crosscheck.md`)**:
   - T1 production writes: `p.faceAngleX/Z` (focus helper), `p.lastStepX/Z` (Teleport body) → pinned by TestPlayerFocus_HelperWritesFaceAngleOnly + TestPlayerTeleport_FocusFromDirection + TestPlayerTeleport_LastStepAdjust + TestPlayerTeleport_InPlaceFocusUsesSelfCenter.
   - T2 production writes: `n.faceAngleX/Z` via `n.focus` from Teleport → pinned by TestNpcTeleport_FocusFromDirection + TestNpcTeleport_FocusSize2 + TestNpcTeleport_InPlaceFocusUsesSelfCenter.
   - T3: comment-only; no test pin needed; verification grep stands in.

5. **Plan-author Go variable-name collisions (`plan_var_name_collision.md`)**: `dir`, `moveX`, `moveZ` locals introduced in T1 + T2 don't shadow any existing parameter or scope variable in `(*Player).Teleport`/`(*Npc).Teleport` — existing locals are `prevX`, `prevZ`, `prevLevel` only. No `:=` re-declaration risk.

6. **Spec sibling-site guard audit (`plan_sibling_site_guard_audit.md`)**: T1 imports `coordgrid` into `player_script.go`. Verified that `npc_script.go` (which T2 also imports `coordgrid` into) follows the same import pattern — both are leaf packages already in `modules/world/`. No new sibling guard needed; the helpers are infallible.

7. **Hex literal overflow (`int32_hex_literal_overflow.md`)**: not applicable — no `0xDEADBEEF`-style fixtures in this plan.

8. **Mock recorder field naming check (`mock_recorder_field_naming_check.md`)**: not applicable — no mocks introduced; existing `mockEntity` and `mockActiveNpc` patterns are not touched.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-01-nai-65-pathing-entity-focus-stride-partial.md`.
