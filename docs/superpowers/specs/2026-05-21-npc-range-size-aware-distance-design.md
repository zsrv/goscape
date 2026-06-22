# NPC_RANGE size-aware distance — design

**Date:** 2026-05-21
**Scope:** M-shaped behavior fix at `handleNpcRange` (`pkg/script/handlers_npc.go:1114-1138`) — port TS `CoordGrid.distanceTo` semantics so size>1 NPCs compute Chebyshev distance from their nearest occupied tile rather than from origin.
**Predecessor:** [[npc-param-unknown-param-close]] — closes the prior session arc. Working tree at HEAD `33a8d3cc`.

## 1. Motivation

TS `NpcOps.ts:152-169` for NPC_RANGE delegates to `CoordGrid.distanceTo(npc, target)` where `target = {x, z, width: 1, length: 1}` and `npc` carries its own `width`/`length`. TS `CoordGrid.distanceTo` (`CoordGrid.ts:60-64`) computes Chebyshev distance between two BOXES, using `closest(pos, other)` to find each box's nearest occupied cell relative to the other.

Goscape `handleNpcRange` (`pkg/script/handlers_npc.go:1114-1138`) currently computes `max(|npc.x - target.x|, |npc.z - target.z|)` — Chebyshev distance from the NPC's ORIGIN cell. For size=1 NPCs the two formulas are identical; for size>1 NPCs they diverge whenever the target lies east, north, or northeast of the NPC origin (the NPC occupies cells beyond its origin in those directions).

**Concrete divergence example:** size=3 NPC at origin `(10, 10)` (occupying `(10..12, 10..12)`), target at `(15, 15)`:
- TS: closest npc cell = `(12, 12)`, distance = `max(|12-15|, |12-15|) = 3`
- Goscape: distance from `(10,10)` to `(15,15)` = `max(5, 5) = 5`

Source comment at `handlers_npc.go:1108-1113` documents the deferral:
> *"size>1 audit deferred to a future sub-spec per NAI-120 Bundle 1 audit section 6 dependency note"*

This slice closes that deferral.

## 2. Sibling-handler audit (in-scope per user direction)

Audit motion verified ONE other handler reads NPC origin coordinates for arithmetic:

| Site | Use | Outcome |
|---|---|---|
| `handlers_npc.go:1128-1132` `handleNpcRange` | Chebyshev distance | **In-scope** — port to closest-edge |
| `handlers_npc.go:1000-1002` `handleNpcHunt` | Euclidean-squared distance for closest-NPC tiebreaker | **TS-faithful as-is** — TS `NpcOps.ts:290-325` explicitly uses `CoordGrid.euclideanSquaredDistance(position, npc)` which takes only `{x, z}` (no width/length). Origin-based is intentional in TS for hunt ranking. |

The audit closes cleanly — NPC_HUNT is NOT a sibling-bug. Only `handleNpcRange` needs the port.

## 3. Architecture

Three-layer change touching four files:

```
pkg/script/active.go            (+2 interface methods)
modules/world/npc_script.go     (+2 adapter methods on *Npc)
pkg/script/handlers_npc_test.go (+2 mockNpc fields + 2 accessors + 3 new tests)
pkg/script/handlers_npc.go      (rewrite handleNpcRange body)
```

No new files. No public API outside the script package.

## 4. Interface extension — `ActiveNpc`

Add two methods to the `ActiveNpc` interface (`pkg/script/active.go:801`):

```go
// NpcWidth returns the NPC's tile-footprint width. NPCs are square in
// practice (NpcType.Size populates both width and length per
// modules/world/npc.go:233-239), but the interface keeps them distinct
// to mirror TS Npc.width/length semantics for CoordGrid.distanceTo.
NpcWidth() int

// NpcLength returns the NPC's tile-footprint length. See NpcWidth.
NpcLength() int
```

Placement: directly after `NpcCategory() int` (line 813) — that field is the closest existing "per-NPC type-derived constant" accessor, so width/length sit naturally beside it.

## 5. Production adapter — `*Npc`

Add to `modules/world/npc_script.go` (placed after the existing `NpcCategory` adapter at line 29-34):

```go
// NpcWidth returns the NPC's tile-footprint width. Delegates to the
// existing Width() method (npc.go:233-236) which returns n.size.
func (n *Npc) NpcWidth() int { return n.Width() }

// NpcLength returns the NPC's tile-footprint length. Delegates to the
// existing Length() method (npc.go:238-239) which returns n.size.
func (n *Npc) NpcLength() int { return n.Length() }
```

The compile-time check `var _ script.ActiveNpc = (*Npc)(nil)` at `npc_script.go:11` will catch missing implementations.

## 6. Test fixture — `mockNpc`

Add fields after `category int` (line 240):

```go
typeID, x, z, level, uid, category int
width, length                       int
nid                                 int
```

Add accessor methods with default-zero-to-1 fallback near the existing accessors at lines 294-303:

```go
// NpcWidth returns m.width, defaulting to 1 when unset. Preserves
// backward-compat with existing test fixtures that don't set width.
// The default-to-1 contract matches production semantics: NpcType.Size
// is initialized to 1 (npctype.go:310) and never zero in production.
func (m *mockNpc) NpcWidth() int {
    if m.width == 0 {
        return 1
    }
    return m.width
}

// NpcLength returns m.length, defaulting to 1. See NpcWidth.
func (m *mockNpc) NpcLength() int {
    if m.length == 0 {
        return 1
    }
    return m.length
}
```

## 7. Handler rewrite — `handleNpcRange`

Replace lines 1128-1136 (the `dx`/`dz` arithmetic and `max(dx, dz)` push) with:

```go
// Closest-edge Chebyshev per TS CoordGrid.distanceTo + closest
// (CoordGrid.ts:60-72): clamp the target cell into the NPC's
// occupied footprint, then take the max-absolute-axis delta. For
// size=1 NPCs (width=length=1), occupiedX = n.NpcX() and the formula
// collapses to the prior origin-Chebyshev form (byte-identical).
nx := n.NpcX()
nz := n.NpcZ()
occupiedX := nx + n.NpcWidth() - 1
occupiedZ := nz + n.NpcLength() - 1

clampedX := x
if x < nx {
    clampedX = nx
} else if x > occupiedX {
    clampedX = occupiedX
}
clampedZ := z
if z < nz {
    clampedZ = nz
} else if z > occupiedZ {
    clampedZ = occupiedZ
}

dx := clampedX - x
if dx < 0 {
    dx = -dx
}
dz := clampedZ - z
if dz < 0 {
    dz = -dz
}
s.PushInt(max(dx, dz))
return nil
```

Uses the Go 1.21+ `max()` builtin to match the existing handler's final push at line 1136.

Plus update the doc-comment at `handlers_npc.go:1086-1113` to remove the "size>1 audit deferred" framing and replace with a note pointing at this slice's design.

### 7.1 Why the inline clamp instead of a helper

TS `CoordGrid.closest` is reused at three call sites; goscape currently has no `coordgrid.Closest` helper. Adding a helper is **out-of-scope creep**: the only goscape consumer in this slice is `handleNpcRange`. If a future port surfaces another size-aware distance site, a helper extraction becomes worthwhile then.

## 8. Test plan

### 8.1 Preserved tests (no changes expected)

All five existing tests use mockNpc with width/length unset (zero), which the default-zero-to-1 accessor maps to width=length=1. The new formula reduces to origin-Chebyshev for size=1, so:

- `TestNpcRange_SameLevel_Adjacent` — npc (3222, 3218), target (3223, 3218) → dist 1
- `TestNpcRange_SameLevel_Diagonal` — npc (3222, 3218), target (3223, 3219) → dist 1
- `TestNpcRange_DifferentLevel_Sentinel` — diff level → -1
- `TestNpcRange_NoActiveNpc` — no ActiveNpc → error
- `TestNpcRange_InvalidCoord` — invalid coord → error

All pass byte-identically.

### 8.2 New tests (3)

Place after `TestNpcRange_InvalidCoord`:

```go
// TestNpcRange_Size3_TargetInsideFootprint: size-3 NPC at origin (10, 10)
// occupies (10..12, 10..12). Target at (11, 11) is INSIDE the footprint —
// closest cell is (11, 11), distance 0. TS-faithful per
// CoordGrid.distanceTo + closest.
func TestNpcRange_Size3_TargetInsideFootprint(t *testing.T) { ... }

// TestNpcRange_Size3_TargetEastOfFootprint: size-3 NPC at (10, 10), target
// at (15, 11). Closest npc cell = (12, 11), distance = max(|12-15|, |11-11|)
// = 3. Origin-based would erroneously return max(|10-15|, |10-11|) = 5;
// this test pins the divergence fix.
func TestNpcRange_Size3_TargetEastOfFootprint(t *testing.T) { ... }

// TestNpcRange_Size3_TargetSouthwestOfFootprint: size-3 NPC at (10, 10),
// target at (8, 8). Closest npc cell = (10, 10), distance = max(2, 2) = 2.
// This case is byte-identical between origin-based and closest-edge
// formulas (target lies to SW of origin where the NPC's larger footprint
// doesn't shorten the distance); pin for regression safety.
func TestNpcRange_Size3_TargetSouthwestOfFootprint(t *testing.T) { ... }
```

Each constructs `mockNpc{x: 10, z: 10, level: 0, width: 3, length: 3}` and follows the existing test idiom (PushInt of packed coord, call handler, assert PopInt).

Total new test count: **3**. Total NPC_RANGE test count post-slice: **8** (5 preserved + 3 new).

### 8.3 Coverage of NpcWidth/NpcLength accessors

No dedicated unit tests for the accessors themselves — they're trivial getters and the 3 new size>1 NPC_RANGE tests exercise them as integration. Adding standalone accessor tests would be redundant.

## 9. Gates

Standard validation gates (CLAUDE.md):

```
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go test -race ./... -count=1
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go test ./pkg/packall/ -run TestPackAll_TwelveStageSmoke -count=1
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/gofmt -l <touched files>
```

Audit-greps:

- `grep -c "NpcWidth()\b" pkg/script/active.go modules/world/npc_script.go pkg/script/handlers_npc_test.go pkg/script/handlers_npc.go` — expect ≥ 4 (interface decl + adapter + mock accessor + handler use)
- `grep -c "NpcLength()\b" ` (same files) — expect ≥ 4
- `grep -c "occupiedX\|occupiedZ" pkg/script/handlers_npc.go` — expect 2 each (handler body only)
- `grep -c "size>1 audit deferred" pkg/script/handlers_npc.go` — expect 0 (carry-forward retired from doc-comment)

Compile-time guarantee: `var _ script.ActiveNpc = (*Npc)(nil)` at `modules/world/npc_script.go:11` will fail to compile if the adapter methods are missing.

## 10. Risk mitigations

- **Risk:** Other tests using `mockNpc` may accidentally exercise NpcWidth/NpcLength through unrelated handlers. **Mitigation:** Only `handleNpcRange` in this slice reads NpcWidth/NpcLength. Audit-grep `NpcWidth\|NpcLength` across `pkg/script/handlers_*.go` confirms zero other call sites pre-slice. Default-zero-to-1 accessor preserves correctness for any future inadvertent use.
- **Risk:** A test was relying on the buggy origin-based behavior for size>1. **Mitigation:** Grep `mockNpc{[^}]*width\s*:\s*[2-9]\|mockNpc{[^}]*size\s*:\s*[2-9]` returns zero hits — no existing test constructs size>1 mockNpc.
- **Risk:** Performance regression from extra clamp branches. **Mitigation:** Two int comparisons + assignments per axis; negligible vs the script-execution overhead. Not worth measuring.
- **Risk:** Doc-comment update at `handlers_npc.go:1108-1113` loses important context. **Mitigation:** Replace the "deferred" framing with explicit "Size-aware Chebyshev per TS CoordGrid.distanceTo" and cite this design doc.

## 11. Out of scope

- `coordgrid.DistanceTo` / `coordgrid.Closest` helper extraction — only one consumer in this slice; revisit if a future port adds a second.
- `NPC_HUNT` euclideanSquaredDistance audit — verified TS-faithful in §2.
- Player-side `P_*RANGE`-style opcodes — separate audit, not part of this slice.
- Modifying `*Npc.Width()` / `Length()` signature — already returns `int`; the new ActiveNpc methods delegate without altering production.
- Adding mockNpc fields beyond width/length — out-of-scope creep.

## 12. Commit plan

Single commit, in-thread:

```
refactor(script): size-aware NPC_RANGE Chebyshev distance per TS

Port TS CoordGrid.distanceTo + closest semantics into handleNpcRange
so size>1 NPCs compute Chebyshev distance from their nearest occupied
tile rather than from origin. Size=1 NPCs are byte-identical; size>1
diverged east/north of origin by (size-1) per axis.

Adds NpcWidth/NpcLength to the ActiveNpc interface (one method per
field, matching the existing NpcX/NpcZ pattern) with delegating
adapters on *Npc (Width/Length already exist) and default-zero-to-1
accessors on mockNpc (preserves the &ScriptState{} test-fixture
convention from state.go:277). NPC_HUNT's origin-Euclidean ranking is
TS-faithful per CoordGrid.euclideanSquaredDistance and stays unchanged.

Closes the size>1 deferral at handlers_npc.go:1108-1113 noted in
NAI-120 Bundle 1 audit section 6.
```

## 13. Cadence

XS scope post-audit, in-thread main-thread, no subagent dispatch — matches the session's six-slice clean-cadence streak. Spec → impl → gates → commit → close-memo + MEMORY.md.
