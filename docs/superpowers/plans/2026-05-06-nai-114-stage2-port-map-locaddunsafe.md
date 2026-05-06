# NAI-114 Stage 2 — port MAP_LOCADDUNSAFE handler — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the `MAP_LOCADDUNSAFE` (opcode 1012) script handler so the firemaking script chain `[opheldu,tinderbox]` → `[label,light_logs_inv]` → `[proc,area_allow_loc_add]` advances past the area-allow check at PC 1 of script id 7358 (currently aborts with "no handler for MAP_LOCADDUNSAFE").

**Architecture:** Single new opcode handler in `pkg/script/handlers_map.go` mirroring TS `Engine-TS/src/engine/script/handlers/ServerOps.ts:212-252` line-by-line. Pops one packed coord, iterates **all locs in the coord's zone** (not just same-tile, unlike `LOC_ADD`'s `LocsAtCoord`), filters by `LocType.Active != 1` and the WALL-only inactive-skip rule, then performs per-layer occupancy checks against the input coord. Pushes 1 if any active loc occupies the tile, else 0. Two infra extensions: (a) extend the script-side `ActiveLoc` interface with an `Active() bool` method (carries `entity.Loc.IsActive`); (b) extend `script.LocOps` with `AllLocsInZone(level, x, z int) []ActiveLoc` (zone-wide, distinct from existing `LocsAtCoord` which filters by tile).

**Tech Stack:** Go 1.26+. Mirrors LostCityRS/Engine-TS canonical TS source (`ts_source_canonical_path`).

**Out of scope (deferred to NAI-115 per spec §5 Shape D escalation):** the 7-handler cascade (`INV_DROPSLOT`, `OBJ_DEL`, `OBJ_COORD`, `OBJ_ADDALL`, `OBJ_ADD`, `LINEOFWALK`, `P_OPOBJ`). Stage-2 smoke verification proves milestone progress: server warn shifts from `MAP_LOCADDUNSAFE` to the next missing handler in chain order (`INV_DROPSLOT` if `[proc,area_allow_loc_add]` returns 1; or content "You can't light a fire here." MES if it returns 0).

---

## File-Structure Map

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/active.go` | Modify (single line in `ActiveLoc` interface block at L721-727) | Add `Active() bool` method to `ActiveLoc` interface |
| `pkg/entity/loc.go` | Modify (append after L96) | Add `Active() bool` method on `*entity.Loc` returning `l.IsActive` (no field rename — Go allows method+field with different names; this method name does not collide with the `IsActive` field) |
| `pkg/script/loc_ops.go` | Modify (append to interface block) | Add `AllLocsInZone(level, x, z int) []ActiveLoc` method to `LocOps` interface |
| `modules/world/script_loc_ops.go` | Modify (append after `LocsAtCoord` impl at L78) | Implement `AllLocsInZone` returning every `*entity.Loc` in the zone (no per-tile filter) |
| `pkg/script/loc_ops_test.go` | Modify (extend `fakeLocOps` struct + add stub method) | Update `fakeLocOps` to satisfy the extended `LocOps` interface |
| `pkg/script/handlers_loc_test.go` | Modify (extend `fakeActiveLoc` struct at L11-23) | Add `active bool` field + `Active() bool` method on `fakeActiveLoc` |
| `pkg/script/handlers_player_test.go` | Modify (extend `mockActiveLoc` struct at L17-29) | Add `active bool` field + `Active() bool` method on `mockActiveLoc` |
| `pkg/script/handlers_map.go` | Modify (append after `handleMapBlocked` at L188-205) | Add `handleMapLocAddUnsafe` function (Shape A new handler) |
| `pkg/script/handlers.go` | Modify (single-line entry near L100 in the map-handlers cluster) | Register `OpMapLocAddUnsafe: handleMapLocAddUnsafe` |
| `pkg/script/handlers_map_test.go` | Modify (append after the `MAP_BLOCKED` test cluster at L321-411) | Add `mapLocAddUnsafeOps` fixture + 8 unit tests covering every TS branch |

No file deletions. No new files.

---

## Reference Material

### TS source (canonical port target)

`Engine-TS/src/engine/script/handlers/ServerOps.ts:212-252`:

```ts
[ScriptOpcode.MAP_LOCADDUNSAFE]: state => {
    const coord: CoordGrid = check(state.popInt(), CoordValid);

    for (const loc of World.gameMap.getZone(coord.x, coord.z, coord.level).getAllLocsUnsafe()) {
        const type = check(loc.type, LocTypeValid);

        if (type.active !== 1) {
            continue;
        }

        const layer = loc.layer;

        if (!loc.isActive && layer === LocLayer.WALL) {
            continue;
        }

        if (layer === LocLayer.WALL) {
            if (loc.x === coord.x && loc.z === coord.z) {
                state.pushInt(1);
                return;
            }
        } else if (layer === LocLayer.GROUND) {
            const width = loc.angle === LocAngle.NORTH || loc.angle === LocAngle.SOUTH ? loc.length : loc.width;
            const length = loc.angle === LocAngle.NORTH || loc.angle === LocAngle.SOUTH ? loc.width : loc.length;
            for (let index = 0; index < width * length; index++) {
                const deltaX = loc.x + (index % width);
                const deltaZ = loc.z + ((index / width) | 0);
                if (deltaX === coord.x && deltaZ === coord.z) {
                    state.pushInt(1);
                    return;
                }
            }
        } else if (layer === LocLayer.GROUND_DECOR) {
            if (loc.x === coord.x && loc.z === coord.z) {
                state.pushInt(1);
                return;
            }
        }
    }
    state.pushInt(0);
},
```

### Goscape sibling-handler patterns (for shape match)

- `pkg/script/handlers_map.go:184-205` — `handleMapBlocked` (single-coord, world-state read, push int).
- `pkg/script/handlers_loc.go:279-322` — `handleLocAdd` (uses `requireConfigs`, `checkLocAngle`, `LocOps.LocsAtCoord` per-tile iteration, `coordgrid.UnpackCoord`).
- `pkg/script/handlers_npc.go:13-19` — `checkCoord(v int, op string) (level, x, z int, err error)`.

### Layer + Angle constants

- Layer values come from `pkg/pathfinder/loc/layer.go:7-12`: `LayerWall=0`, `LayerWallDecor=1`, `LayerGround=2`, `LayerGroundDecor=3`. `entity.Loc.Layer()` returns these as `int`. **TS `LocLayer.WALL` ↔ goscape `LayerWall (0)`; TS `LocLayer.GROUND` ↔ goscape `LayerGround (2)`; TS `LocLayer.GROUND_DECOR` ↔ goscape `LayerGroundDecor (3)`.**
- Angle values come from `pkg/pathfinder/loc/angle.go:3-8`: `AngleWest=0`, `AngleNorth=1`, `AngleEast=2`, `AngleSouth=3`. **TS `LocAngle.NORTH` ↔ goscape `AngleNorth (1)`; TS `LocAngle.SOUTH` ↔ goscape `AngleSouth (3)`.**

### Width/Length lookup

`ActiveLoc` itself does NOT expose width/length (see `active.go:721-727`). Width/length live on `LocType` (`pkg/objtype/loctype.go:27-28`). Look them up via `s.Configs.LocType(loc.LocType()).Width / .Length`. Defaults: 1×1 (set in `NewLocType` at `loctype.go:181-182`).

### LocType.Active gate (TS line 218)

`LocType.Active int` (`pkg/objtype/loctype.go:31`) — values `0`, `1`, or `-1` (the `-1` sentinel is coerced to `0` or `1` by `PostDecode` at `loctype.go:166-176` based on `Shapes`/`Op` presence). **TS check `type.active !== 1` ↔ goscape `lt.Active != 1`.**

### LocType validator (TS `check(loc.type, LocTypeValid)` at line 216)

TS rejects null/missing LocType. Goscape mirror: `s.Configs.LocType(id) == nil` returns nil-pointer; treat as `continue` (the loc is skipped). This matches the existing `handleLocAdd` pattern at `handlers_loc.go:289-291` which errors on nil for the input typ argument; here we silently skip per-loc since the iteration must continue.

### Disabled-handler current behavior (control: how the abort observable today)

`pkg/script/runner.go:68-73` — missing handler returns `fmt.Errorf("script %q: no handler for %s (opcode %d) at pc=%d", …)` with `s.Execution = Aborted`. Logged by `modules/world/script.go:111-122` as a warn; visible in server stdout.

---

## Self-Review Checklist (already applied during plan-write)

1. **Spec coverage** — Stage 2 §5 Shape A "port MAP_LOCADDUNSAFE only"; smoke binding §6; risk R6 (multi-tile loc footprint regression) addressed by Task 3 sub-test 3.6 (NORTH/SOUTH angle width/length swap explicitly tested).
2. **Placeholder scan** — none.
3. **Type consistency** — `Active() bool` is the new method name for both `ActiveLoc` interface and `*entity.Loc` impl; `AllLocsInZone(level, x, z int) []ActiveLoc` is the new `LocOps` method name; `handleMapLocAddUnsafe` is the new handler name. All used identically across tasks.

---

## Task 1: Extend `ActiveLoc` interface with `Active() bool`

**Files:**
- Modify: `pkg/script/active.go:721-727` (interface block)
- Modify: `pkg/entity/loc.go:96` (append after `IsValid` method)
- Modify: `pkg/script/handlers_loc_test.go:11-23` (extend `fakeActiveLoc`)
- Modify: `pkg/script/handlers_player_test.go:17-29` (extend `mockActiveLoc`)
- Modify: `pkg/script/loc_ops_test.go:6-58` (extend `fakeLocOps` only if it needs an `ActiveLoc`-builder helper; check first)

**Why:** TS `MAP_LOCADDUNSAFE` reads `loc.isActive` at `ServerOps.ts:224`. `entity.Loc` has the field (`pkg/entity/loc.go:16`) and `pkg/zone/zone.go` mutates it via `AddStaticLoc`/`AddLoc`/`ChangeLoc`/`RemoveLoc`. The script-side `ActiveLoc` interface (`pkg/script/active.go:721-727`) does not yet expose it. The new method is named `Active()` (not `IsActive()`) because Go forbids a method and field sharing a name within the same struct, and `entity.Loc.IsActive` is an exported field with ~25 reader sites across `pkg/zone`, `modules/world`, and tests — renaming is invasive and out-of-scope.

- [ ] **Step 1.1: Add `Active() bool` to the `ActiveLoc` interface**

Open `pkg/script/active.go`. Locate the `ActiveLoc` interface block (currently L721-727). Add a new method declaration after `Layer()`:

```go
// ActiveLoc is the surface that LOC_* opcodes use to read the loc
// bound to the script's execution. Set by OPLOC trigger routing
// (S6j fireOpTriggerLoc) and LOC_FIND (future).
type ActiveLoc interface {
	LocType() int              // returns the LocType ID (from packed Loc.CurrentInfo bitfield)
	Coords() (x, z, level int) // world position; consumed by LOC_COORD
	Angle() int                // rotation (0=west, 1=north, 2=east, 3=south); consumed by LOC_ANGLE
	Shape() int                // shape (0..22 valid range); consumed by LOC_SHAPE
	Layer() int                // shape's render layer (0..3); consumed by LOC_ADD same-layer search (NAI-86)
	Active() bool              // mirrors entity.Loc.IsActive (zone-managed); consumed by MAP_LOCADDUNSAFE WALL-only inactive-skip (NAI-114)
}
```

- [ ] **Step 1.2: Add `Active()` method on `*entity.Loc`**

Open `pkg/entity/loc.go`. Append after the `IsValid` method (currently the last method in the file at L96):

```go
// Active reports whether the loc is currently zone-active. Mirrors the
// field-backed flag mutated by pkg/zone Zone methods (AddStaticLoc/
// AddLoc/ChangeLoc/RemoveLoc). Method form satisfies the script-side
// ActiveLoc.Active() interface contract; the field name remains
// IsActive for compatibility with the existing ~25 reader sites in
// pkg/zone, modules/world, and tests. NAI-114.
func (l *Loc) Active() bool { return l.IsActive }
```

- [ ] **Step 1.3: Run go build to surface impl-gap compile errors**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: compile errors for every test fake that satisfies `ActiveLoc` but lacks `Active() bool`. Specifically `pkg/script/handlers_loc_test.go` (`fakeActiveLoc`) and `pkg/script/handlers_player_test.go` (`mockActiveLoc`).

- [ ] **Step 1.4: Extend `fakeActiveLoc` in `handlers_loc_test.go`**

Open `pkg/script/handlers_loc_test.go`. Replace L10-23 with:

```go
// fakeActiveLoc is a minimal ActiveLoc implementation for handler tests.
type fakeActiveLoc struct {
	id          int
	x, z, level int
	angle       int
	shape       int
	layer       int
	active      bool
}

func (f fakeActiveLoc) LocType() int              { return f.id }
func (f fakeActiveLoc) Coords() (x, z, level int) { return f.x, f.z, f.level }
func (f fakeActiveLoc) Angle() int                { return f.angle }
func (f fakeActiveLoc) Shape() int                { return f.shape }
func (f fakeActiveLoc) Layer() int                { return f.layer }
func (f fakeActiveLoc) Active() bool              { return f.active }
```

Existing `fakeActiveLoc{id: 42, …}` constructions in the rest of the file leave `active` as `false` (zero-value). That's fine for the existing tests — none of them exercise `MAP_LOCADDUNSAFE` and the LOC_* handlers under test do not consume `Active()`.

- [ ] **Step 1.5: Extend `mockActiveLoc` in `handlers_player_test.go`**

Open `pkg/script/handlers_player_test.go`. Replace L17-29 with:

```go
type mockActiveLoc struct {
	locType int
	x, z    int
	level   int
	angle   int
	shape   int
	layer   int
	active  bool
}

func (m *mockActiveLoc) LocType() int              { return m.locType }
func (m *mockActiveLoc) Coords() (x, z, level int) { return m.x, m.z, m.level }
func (m *mockActiveLoc) Angle() int                { return m.angle }
func (m *mockActiveLoc) Shape() int                { return m.shape }
func (m *mockActiveLoc) Layer() int                { return m.layer }
func (m *mockActiveLoc) Active() bool              { return m.active }
```

- [ ] **Step 1.6: Re-run go build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean build (zero errors).

- [ ] **Step 1.7: Run full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests PASS (existing tests do not consume `Active()` so the zero-value default is irrelevant to their assertions).

- [ ] **Step 1.8: Commit**

```bash
git add pkg/script/active.go pkg/entity/loc.go pkg/script/handlers_loc_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-114 — add Active() to ActiveLoc interface

Extends the script-side ActiveLoc interface with Active() bool, sourced
from entity.Loc.IsActive. The new method is required by the upcoming
MAP_LOCADDUNSAFE handler (NAI-114 Stage 2) for the WALL-only
inactive-skip rule per TS ServerOps.ts:224.

Method name disambiguates from the entity.Loc.IsActive field (Go forbids
method/field name overlap on the same type); the field is preserved
because ~25 reader sites consume it directly across pkg/zone,
modules/world, and tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Extend `LocOps` with `AllLocsInZone`

**Files:**
- Modify: `pkg/script/loc_ops.go` (interface block)
- Modify: `modules/world/script_loc_ops.go:78` (append after `LocsAtCoord` impl)
- Modify: `pkg/script/loc_ops_test.go` (extend `fakeLocOps`)

**Why:** `MAP_LOCADDUNSAFE` iterates `World.gameMap.getZone(coord).getAllLocsUnsafe()` — every loc in the zone, not just locs at the target tile. The existing `LocsAtCoord` filters by `(x, z, level)` exact match (`script_loc_ops.go:73`), which would drop GROUND-layer locs whose footprint covers the target tile from a different anchor. The new method returns the full zone slice; the handler does the per-loc footprint check itself.

- [ ] **Step 2.1: Write a failing test exercising `AllLocsInZone`**

Open `pkg/script/loc_ops_test.go`. Append at the end of the file:

```go
// TestLocOpsInterfaceHasAllLocsInZone confirms LocOps surfaces the
// zone-wide loc enumeration MAP_LOCADDUNSAFE needs (distinct from
// LocsAtCoord which filters by exact tile). NAI-114.
func TestLocOpsInterfaceHasAllLocsInZone(t *testing.T) {
	var ops LocOps = &fakeLocOps{}
	got := ops.AllLocsInZone(0, 100, 200)
	if got != nil {
		t.Errorf("fakeLocOps.AllLocsInZone(empty): got %v, want nil", got)
	}
}
```

(If `loc_ops_test.go` does not already import `"testing"`, add it.)

- [ ] **Step 2.2: Run the test to confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestLocOpsInterfaceHasAllLocsInZone ./pkg/script/`

Expected: FAIL — compile error `*fakeLocOps does not implement LocOps (missing method AllLocsInZone)`.

- [ ] **Step 2.3: Add `AllLocsInZone` to the `LocOps` interface**

Open `pkg/script/loc_ops.go`. Replace the entire file with:

```go
package script

// LocOps is the script→world mutator surface for LOC_CHANGE / LOC_ADD /
// LOC_DEL / LOC_ANIM / MAP_LOCADDUNSAFE. Implementations live in
// modules/world (see script_loc_ops.go). Decouples pkg/script from
// world-side entity types; handlers pass the script-side ActiveLoc
// interface, the adapter type-asserts to the concrete *entity.Loc.
//
// LocsAtCoord returns the slice of locs at (level, x, z) for the
// LOC_ADD same-layer search branch. Returning a slice (not iter.Seq)
// keeps the interface simple — the call site iterates synchronously
// and the slice is small (≤4 layers per tile in practice).
//
// AllLocsInZone returns every loc in the zone owning (level, x, z),
// without any per-tile filtering. NAI-114 MAP_LOCADDUNSAFE consumes
// this for footprint-overlap probing; the handler does the per-loc
// (x, z, layer, footprint) checks itself per TS
// ServerOps.ts:212-252.
type LocOps interface {
	ChangeLoc(loc ActiveLoc, typ, shape, angle, duration int) error
	AddLoc(level, x, z, typ, shape, angle, duration int) (ActiveLoc, error)
	RemoveLoc(loc ActiveLoc, duration int) error
	AnimLoc(loc ActiveLoc, seq int) error
	LocsAtCoord(level, x, z int) []ActiveLoc
	AllLocsInZone(level, x, z int) []ActiveLoc
}
```

- [ ] **Step 2.4: Stub `AllLocsInZone` on `fakeLocOps`**

Open `pkg/script/loc_ops_test.go`. The current `fakeLocOps` struct (L6-13) is:

```go
type fakeLocOps struct {
	changeCalls []changeLocCall
	addCalls    []addLocCall
	removeCalls []removeLocCall
	animCalls   []animLocCall
	atCoord     []ActiveLoc
	addReturn   ActiveLoc // returned from AddLoc
}
```

Add a new field `inZone []ActiveLoc` after `atCoord` so the struct becomes:

```go
type fakeLocOps struct {
	changeCalls []changeLocCall
	addCalls    []addLocCall
	removeCalls []removeLocCall
	animCalls   []animLocCall
	atCoord     []ActiveLoc
	inZone      []ActiveLoc
	addReturn   ActiveLoc // returned from AddLoc
}
```

Append a new method after `LocsAtCoord` (currently L54-56):

```go
func (f *fakeLocOps) AllLocsInZone(level, x, z int) []ActiveLoc {
	return f.inZone
}
```

- [ ] **Step 2.5: Re-run the test to confirm it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestLocOpsInterfaceHasAllLocsInZone ./pkg/script/`

Expected: PASS.

- [ ] **Step 2.6: Implement `AllLocsInZone` in the modules/world adapter**

Open `modules/world/script_loc_ops.go`. Append after the `LocsAtCoord` impl (currently ends at L78):

```go
// AllLocsInZone returns the script-side ActiveLoc slice for every loc
// in the zone owning (level, x, zc), without any per-tile filter.
// MAP_LOCADDUNSAFE (NAI-114) consumes this for footprint-overlap
// probing; the handler does the per-loc (x, z, layer, footprint)
// checks itself.
func (o *serverLocOps) AllLocsInZone(level, x, zc int) []script.ActiveLoc {
	z := o.s.zoneMap.Get(level, x, zc)
	out := make([]script.ActiveLoc, 0, len(z.Locs))
	for _, l := range z.Locs {
		out = append(out, l)
	}
	return out
}
```

- [ ] **Step 2.7: Run full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests PASS.

- [ ] **Step 2.8: Commit**

```bash
git add pkg/script/loc_ops.go pkg/script/loc_ops_test.go modules/world/script_loc_ops.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-114 — add LocOps.AllLocsInZone for zone-wide enumeration

Adds AllLocsInZone(level, x, z int) []ActiveLoc to the script.LocOps
interface and implements it in modules/world's serverLocOps adapter.
Returns every loc in the zone owning the input coord, without the
per-tile filter LocsAtCoord applies.

Required by the upcoming MAP_LOCADDUNSAFE handler (NAI-114 Stage 2)
which mirrors TS World.gameMap.getZone(coord).getAllLocsUnsafe() at
Engine-TS/src/engine/script/handlers/ServerOps.ts:215. The handler
performs per-loc footprint-overlap checks itself, so the adapter does
no filtering.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: TDD `handleMapLocAddUnsafe`

**Files:**
- Modify: `pkg/script/handlers_map.go` (append after `handleMapBlocked` at L188-205)
- Modify: `pkg/script/handlers_map_test.go` (append after the `MAP_BLOCKED` test cluster, currently ends at L411)
- Modify: `pkg/script/handlers.go` (single-line registration entry — Task 4 does this; deferred to keep Task 3 unit-test-only)

**Why:** Mirror TS `ServerOps.ts:212-252` line-by-line. Per `true_to_ts_gate`, every behavioral branch is pinned by a unit test before any production code is written. The branch matrix is:

| # | Branch | Test |
|---|---|---|
| 1 | Empty zone → push 0 | 3.1 |
| 2 | Single loc, `LocType.Active != 1` → skip → push 0 | 3.2 |
| 3 | Single inactive WALL → skip → push 0 (TS line 224) | 3.3 |
| 4 | Single active WALL at coord → push 1 | 3.4 |
| 5 | Single active WALL **not** at coord → continue → push 0 | 3.5 |
| 6 | Single active GROUND, 1×1, at coord → push 1 | 3.6a |
| 7 | Single active GROUND, 2×1, anchor=(100,100), coord=(101,100) → push 1 (footprint covers) | 3.6b |
| 8 | Single active GROUND, 1×2 with `AngleNorth`, anchor=(100,100), coord=(100,101) → push 1 (NORTH/SOUTH angle swaps width/length) | 3.6c |
| 9 | Single active GROUND_DECOR at coord → push 1 | 3.7 |
| 10 | Multiple inactive non-WALL locs (still checked per TS line 224 inverse), one matches → push 1 | 3.8 |
| 11 | Coord validation error (negative coord) → return error, no push | 3.9 |
| 12 | Configs nil (LocType lookup degrades) → silently skip the loc → push 0 | 3.10 |

- [ ] **Step 3.1: Write the empty-zone test (RED — empty/missing handler)**

Open `pkg/script/handlers_map_test.go`. Append at the end of the file:

```go
// --- NAI-114 Stage 2: MAP_LOCADDUNSAFE Layer 1 unit tests --------------

// mapLocAddUnsafeOps is a minimal LocOps fixture for MAP_LOCADDUNSAFE
// tests. Records nothing; provides controllable AllLocsInZone return.
// The other LocOps methods are not called by MAP_LOCADDUNSAFE; they
// satisfy the interface so a test-state can mount this as state.LocOps.
type mapLocAddUnsafeOps struct {
	zoneLocs []ActiveLoc
}

func (m *mapLocAddUnsafeOps) ChangeLoc(loc ActiveLoc, typ, shape, angle, duration int) error {
	return nil
}
func (m *mapLocAddUnsafeOps) AddLoc(level, x, z, typ, shape, angle, duration int) (ActiveLoc, error) {
	return nil, nil
}
func (m *mapLocAddUnsafeOps) RemoveLoc(loc ActiveLoc, duration int) error { return nil }
func (m *mapLocAddUnsafeOps) AnimLoc(loc ActiveLoc, seq int) error        { return nil }
func (m *mapLocAddUnsafeOps) LocsAtCoord(level, x, z int) []ActiveLoc     { return nil }
func (m *mapLocAddUnsafeOps) AllLocsInZone(level, x, z int) []ActiveLoc   { return m.zoneLocs }

// runMapLocAddUnsafe is the standard test harness: pushes the packed
// coord and dispatches the opcode through Execute (so registration is
// also exercised).
func runMapLocAddUnsafe(t *testing.T, locs []ActiveLoc, configs Configs, packedCoord int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_MAP_LOCADDUNSAFE",
		Opcodes:          []Opcode{OpMapLocAddUnsafe, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := &ScriptState{
		Script:      sf,
		LocOps:      &mapLocAddUnsafeOps{zoneLocs: locs},
		Configs:     configs,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(packedCoord)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return state
}

func TestMapLocAddUnsafe_EmptyZonePushes0(t *testing.T) {
	state := runMapLocAddUnsafe(t, nil, &fakeConfigs{}, coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("empty zone: got top=%d ISP=%d, want top=0 ISP=1",
			state.IntStack[0], state.ISP)
	}
}
```

- [ ] **Step 3.2: Run the empty-zone test to confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_EmptyZonePushes0 ./pkg/script/`

Expected: FAIL — `Execute: script "test_MAP_LOCADDUNSAFE": no handler for MAP_LOCADDUNSAFE (opcode 1012) at pc=0`. (This is the abort that the production fix will eliminate.)

- [ ] **Step 3.3: Implement minimal `handleMapLocAddUnsafe` to make the empty-zone test pass**

Open `pkg/script/handlers_map.go`. Append at the end of the file:

```go
// handleMapLocAddUnsafe (MAP_LOCADDUNSAFE, opcode 1012) reports whether
// the input coord is occupied by an active loc that would block a new
// loc-add at that tile. Pops one packed coord; pushes 1 if any qualifying
// loc occupies the tile, else 0. Mirrors TS ServerOps.ts:212-252.
//
// Per-loc filter (TS line 218 + 224):
//
//   - LocType.Active != 1 → skip the loc entirely (no occupancy check).
//   - !loc.Active() && layer == LayerWall → skip the loc entirely
//     (goscape defensive note: TS skips inactive walls only; inactive
//     ground / ground-decor locs ARE checked).
//
// Per-layer occupancy check (TS lines 228-249):
//
//   - LayerWall (TS LocLayer.WALL): exact (x, z) match.
//   - LayerGround (TS LocLayer.GROUND): footprint covers (coord.x, coord.z),
//     where width/length are LocType.Width/Length swapped if Angle is
//     AngleNorth or AngleSouth.
//   - LayerGroundDecor (TS LocLayer.GROUND_DECOR): exact (x, z) match.
//   - LayerWallDecor: not enumerated by TS; falls through to push 0.
//
// Configs nil-handling: a nil LocType lookup silently skips the loc
// (mirrors TS check(loc.type, LocTypeValid) which would throw, but
// goscape defensive — script execution continues with the next loc to
// avoid aborting the firemaking chain on a malformed cache entry;
// goscape defensive; TS throws).
func handleMapLocAddUnsafe(s *ScriptState) error {
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "MAP_LOCADDUNSAFE")
	if err != nil {
		return err
	}
	if s.LocOps == nil {
		s.PushInt(0)
		return nil
	}

	for _, l := range s.LocOps.AllLocsInZone(level, x, z) {
		var lt *objtype.LocType
		if s.Configs != nil {
			lt = s.Configs.LocType(l.LocType())
		}
		if lt == nil || lt.Active != 1 {
			continue
		}

		layer := l.Layer()
		if !l.Active() && layer == int(loc.LayerWall) {
			continue
		}

		lx, lz, _ := l.Coords()
		switch layer {
		case int(loc.LayerWall):
			if lx == x && lz == z {
				s.PushInt(1)
				return nil
			}
		case int(loc.LayerGround):
			width, length := lt.Width, lt.Length
			if l.Angle() == loc.AngleNorth || l.Angle() == loc.AngleSouth {
				width, length = lt.Length, lt.Width
			}
			for index := range width * length {
				deltaX := lx + (index % width)
				deltaZ := lz + (index / width)
				if deltaX == x && deltaZ == z {
					s.PushInt(1)
					return nil
				}
			}
		case int(loc.LayerGroundDecor):
			if lx == x && lz == z {
				s.PushInt(1)
				return nil
			}
		}
	}
	s.PushInt(0)
	return nil
}
```

This requires two new imports in `handlers_map.go`. Update the import block at the top of the file:

```go
import (
	"fmt"
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)
```

(`fmt` and `math/rand/v2` and `coordgrid` are already imported per the existing file at L5-10.)

Also register the handler — open `pkg/script/handlers.go` and add a single line in the map-handlers cluster near L100 (after `OpSpotAnimMap`):

```go
	// NAI-114 Stage 2: zone-wide active-loc occupancy probe for the
	// firemaking-chain area-allow check.
	OpMapLocAddUnsafe: handleMapLocAddUnsafe,
```

- [ ] **Step 3.4: Re-run the empty-zone test to confirm it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_EmptyZonePushes0 ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.5: Add the LocType.Active gate test (RED → GREEN already-passing)**

Append to `pkg/script/handlers_map_test.go`:

```go
// TS ServerOps.ts:218 — type.active !== 1 → continue.
func TestMapLocAddUnsafe_LocTypeActiveZeroSkipped(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 0 // explicit; default is -1 then PostDecode coerces, but we set directly
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	wallAtCoord := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  0, // LayerWall
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{wallAtCoord}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("LocType.Active=0 wall at coord: got top=%d ISP=%d, want top=0 ISP=1 (TS line 218 skip)",
			state.IntStack[0], state.ISP)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_LocTypeActiveZeroSkipped ./pkg/script/`

Expected: PASS (already covered by Step 3.3 implementation).

- [ ] **Step 3.6: Add the inactive-WALL skip test**

Append to `pkg/script/handlers_map_test.go`:

```go
// TS ServerOps.ts:224 — !loc.isActive && layer === LocLayer.WALL → continue.
// Distinguishes from TS line 218: this loc HAS LocType.Active=1, but its
// runtime IsActive flag (Loc.IsActive, zone-managed) is false.
func TestMapLocAddUnsafe_InactiveWallSkipped(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	inactiveWallAtCoord := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  0, // LayerWall
		active: false,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{inactiveWallAtCoord}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("inactive wall at coord: got top=%d ISP=%d, want top=0 ISP=1 (TS line 224 skip)",
			state.IntStack[0], state.ISP)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_InactiveWallSkipped ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.7: Add the active-WALL hit test**

Append to `pkg/script/handlers_map_test.go`:

```go
// TS ServerOps.ts:228-232 — active WALL at coord → push 1.
func TestMapLocAddUnsafe_ActiveWallAtCoordPushes1(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeWallAtCoord := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  0, // LayerWall
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeWallAtCoord}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("active wall at coord: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_ActiveWallAtCoordPushes1 ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.8: Add the active-WALL miss test**

Append to `pkg/script/handlers_map_test.go`:

```go
// TS ServerOps.ts:228-232 inverse — active WALL not at coord → continue → push 0.
func TestMapLocAddUnsafe_ActiveWallNotAtCoordPushes0(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeWallElsewhere := fakeActiveLoc{
		id: 42, x: 105, z: 100, level: 0, // 5 tiles east of probe coord
		layer:  0, // LayerWall
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeWallElsewhere}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("active wall not at coord: got top=%d ISP=%d, want top=0 ISP=1",
			state.IntStack[0], state.ISP)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_ActiveWallNotAtCoordPushes0 ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.9: Add the GROUND-layer 1×1 at-coord test**

Append to `pkg/script/handlers_map_test.go`:

```go
// TS ServerOps.ts:233-243 — 1×1 GROUND at coord → push 1 (single iteration
// of the footprint loop, deltaX/deltaZ = anchor).
func TestMapLocAddUnsafe_GroundLayer1x1AtCoordPushes1(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	lt.Width = 1
	lt.Length = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeGround := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  2, // LayerGround
		angle:  0, // AngleWest (no width/length swap)
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeGround}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("1x1 ground at coord: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_GroundLayer1x1AtCoordPushes1 ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.10: Add the GROUND-layer 2×1 footprint-overlap test**

Append to `pkg/script/handlers_map_test.go`:

```go
// TS ServerOps.ts:236-243 — 2×1 GROUND anchored at (100,100), AngleWest:
// width=2, length=1. Footprint covers (100,100) and (101,100). Probing
// (101,100) → push 1 (second iteration of the footprint loop).
func TestMapLocAddUnsafe_GroundLayer2x1FootprintCoversCoord(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	lt.Width = 2
	lt.Length = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeGround := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  2, // LayerGround
		angle:  0, // AngleWest
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeGround}, configs,
		coordgrid.PackCoord(0, 101, 100)) // probe one tile east
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("2x1 ground footprint covers (101,100): got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_GroundLayer2x1FootprintCoversCoord ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.11: Add the GROUND-layer NORTH-angle width/length swap test**

Append to `pkg/script/handlers_map_test.go`:

```go
// TS ServerOps.ts:234-235 — AngleNorth/AngleSouth swap width and length.
// Anchor (100,100), Width=1, Length=2, AngleNorth → effective width=2,
// length=1; footprint covers (100,100) and (101,100). The original
// (101,100) probe must hit; the (100,101) probe (the unswapped axis)
// must miss.
func TestMapLocAddUnsafe_GroundLayerNorthAngleSwapsWidthLength(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	lt.Width = 1
	lt.Length = 2
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeGround := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  2,                  // LayerGround
		angle:  loc.AngleNorth,     // 1
		active: true,
	}
	// Hit case: (101, 100) is covered by the swapped 2×1 footprint.
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeGround}, configs,
		coordgrid.PackCoord(0, 101, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("AngleNorth swap hit: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}

	// Miss case: (100, 101) — the unswapped Length axis — is NOT covered.
	state2 := runMapLocAddUnsafe(t, []ActiveLoc{activeGround}, configs,
		coordgrid.PackCoord(0, 100, 101))
	if state2.ISP != 1 || state2.IntStack[0] != 0 {
		t.Errorf("AngleNorth swap miss: got top=%d ISP=%d, want top=0 ISP=1",
			state2.IntStack[0], state2.ISP)
	}
}
```

The test imports `loc` (`github.com/zsrv/goscape/pkg/pathfinder/loc`). Update the import block at the top of `handlers_map_test.go`:

```go
import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_GroundLayerNorthAngleSwapsWidthLength ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.12: Add the GROUND_DECOR-layer at-coord test**

Append to `pkg/script/handlers_map_test.go`:

```go
// TS ServerOps.ts:244-249 — GROUND_DECOR at coord → push 1 (no footprint;
// exact tile match like WALL).
func TestMapLocAddUnsafe_GroundDecorAtCoordPushes1(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	activeGroundDecor := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  3, // LayerGroundDecor
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{activeGroundDecor}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("ground-decor at coord: got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_GroundDecorAtCoordPushes1 ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.13: Add the inactive-non-WALL-still-checked test**

Append to `pkg/script/handlers_map_test.go`:

```go
// TS ServerOps.ts:224 inverse — inactive ground/ground-decor locs are
// STILL checked (the WALL-only inactive-skip rule does not extend to
// other layers). Probes an inactive ground-decor at coord; expects push 1.
func TestMapLocAddUnsafe_InactiveGroundDecorStillChecked(t *testing.T) {
	lt := objtype.NewLocType(42)
	lt.Active = 1
	configs := &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}}
	inactiveGroundDecor := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  3, // LayerGroundDecor
		active: false,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{inactiveGroundDecor}, configs,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("inactive ground-decor at coord (TS line 224 inverse): got top=%d ISP=%d, want top=1 ISP=1",
			state.IntStack[0], state.ISP)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_InactiveGroundDecorStillChecked ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.14: Add the coord validation error test**

Append to `pkg/script/handlers_map_test.go`:

```go
// Coord validation (checkCoord) errors before the zone iteration begins.
// No push occurs; Execute returns the error tagged "MAP_LOCADDUNSAFE".
func TestMapLocAddUnsafe_NegativeCoordErrors(t *testing.T) {
	sf := &ScriptFile{
		Name:             "test_MAP_LOCADDUNSAFE",
		Opcodes:          []Opcode{OpMapLocAddUnsafe, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := &ScriptState{
		Script:      sf,
		LocOps:      &mapLocAddUnsafeOps{},
		Configs:     &fakeConfigs{},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	state.PushInt(-1) // invalid coord
	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "MAP_LOCADDUNSAFE") {
		t.Errorf("negative coord: got err=%v, want error containing MAP_LOCADDUNSAFE", err)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_NegativeCoordErrors ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.15: Add the Configs-nil graceful-degradation test**

Append to `pkg/script/handlers_map_test.go`:

```go
// Configs nil (defensive — the firemaking chain should not crash if a
// later state-builder forgets to wire Configs). Per the doc comment:
// nil Configs → all per-loc LocType lookups silently skip → push 0.
func TestMapLocAddUnsafe_ConfigsNilSkipsAllLocsPushes0(t *testing.T) {
	wallAtCoord := fakeActiveLoc{
		id: 42, x: 100, z: 100, level: 0,
		layer:  0, // LayerWall
		active: true,
	}
	state := runMapLocAddUnsafe(t, []ActiveLoc{wallAtCoord}, nil,
		coordgrid.PackCoord(0, 100, 100))
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("nil Configs: got top=%d ISP=%d, want top=0 ISP=1",
			state.IntStack[0], state.ISP)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe_ConfigsNilSkipsAllLocsPushes0 ./pkg/script/`

Expected: PASS.

- [ ] **Step 3.16: Run the full MAP_LOCADDUNSAFE test cluster**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -run TestMapLocAddUnsafe ./pkg/script/`

Expected: 12 PASS (Empty / LocTypeActiveZero / InactiveWall / ActiveWallAtCoord / ActiveWallNotAtCoord / GroundLayer1x1 / GroundLayer2x1 / GroundLayerNorthAngle / GroundDecor / InactiveGroundDecor / NegativeCoord / ConfigsNil).

- [ ] **Step 3.17: Run full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all tests PASS.

- [ ] **Step 3.18: Commit**

```bash
git add pkg/script/handlers_map.go pkg/script/handlers_map_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-114 Stage 2 — port MAP_LOCADDUNSAFE handler

Ports script opcode MAP_LOCADDUNSAFE (1012) per TS
Engine-TS/src/engine/script/handlers/ServerOps.ts:212-252. Pops one
packed coord; iterates every loc in the coord's zone via the new
LocOps.AllLocsInZone surface; applies the TS per-loc filter
(LocType.Active != 1 → skip; !loc.Active && WALL → skip); performs
per-layer occupancy probing (WALL/GROUND_DECOR exact-tile match;
GROUND footprint-overlap with NORTH/SOUTH width-length swap).
Pushes 1 if any qualifying loc occupies the input tile, else 0.

Unblocks the firemaking script chain
[opheldu,tinderbox] → [label,light_logs_inv] → [proc,area_allow_loc_add]
which previously aborted at the GOSUB target's PC 1 with
"no handler for MAP_LOCADDUNSAFE". Per-Stage-2 milestone proof, the
next abort in the cascade is INV_DROPSLOT (deferred to NAI-115 with the
6 other secondary-cascade handlers per scope_gate_prerequisite_chain).

Tests pin all 12 TS branches: empty zone, LocType.Active gate, WALL-only
inactive-skip, WALL hit/miss, GROUND 1×1 hit, GROUND 2×1 footprint-cover,
GROUND NORTH-angle width/length swap, GROUND_DECOR hit, inactive
non-WALL still-checked, coord validation error, Configs nil
graceful-degradation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: User-launched smoke handoff

**Files:** none (out-of-tree verification).

**Why:** Per `smoke_test_server_handoff` — the Java client must drive a real OPHELDU on tinderbox+logs against a live goscape server in the user's environment. Claude's sandboxed processes are unreachable from the host client.

- [ ] **Step 4.1: Confirm post-fix grep proves no other MAP_LOCADDUNSAFE references regressed**

Run: `rg -n "OpMapLocAddUnsafe" pkg/ modules/ cmd/`

Expected: now appears in 4 sites (was 2):
1. `pkg/script/opcode.go:86` (constant) — unchanged.
2. `pkg/script/opcode.go:587-588` (String case) — unchanged.
3. `pkg/script/handlers.go:~100` (registration entry) — NEW.
4. `pkg/script/handlers_map.go` (handler body + signature) — NEW.

If any production-code site outside this list appears, investigate.

- [ ] **Step 4.2: Hand off to user for smoke launch**

Reply to user with this paste-ready handoff:

> NAI-114 Stage 2 implementation complete. Please launch the goscape server for smoke verification:
>
> `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`
>
> Then connect via the Java client rev-225 and walk the Tutorial Island fire-making step (use tinderbox on logs in inventory).
>
> **Expected smoke outcome (one of two valid milestone-proof shapes):**
>
> 1. **Cascade advance**: server stdout warn shifts from `no handler for MAP_LOCADDUNSAFE` to `no handler for INV_DROPSLOT` (or another opcode further in the chain). This proves `[proc,area_allow_loc_add]` executed, returned 1 (allowed), and execution advanced past PC 25 of `[label,light_logs_inv]`.
> 2. **Content gate hit**: chatbox/server-log shows `You can't light a fire here.` (TS LocOps.ts:53-style content MES). This proves `[proc,area_allow_loc_add]` executed and returned 0 (blocked) — content/placement issue on Tutorial Island, not a handler issue.
>
> **Either outcome is a Stage-2 success** — the H3 binding is confirmed and the next cascade step is unmasked. Please report which outcome you observe + the relevant server stdout line + a screenshot of the chatbox if the content path fires.
>
> **Failure mode** (un-expected): warn still shows `no handler for MAP_LOCADDUNSAFE` after restart → Stage 2 implementation gap; do not close NAI-114, re-investigate handler registration.
>
> **Per `cascade_theory_smoke_binding`**: residual issues route per the spec §6 fail-routing decision tree. Visible no-effect with no advance → Stage 3 re-investigation.

- [ ] **Step 4.3: Wait for user smoke result before closing NAI-114**

Do not write the close commit until user reports a Stage-2 success outcome (cascade-advance or content-gate-hit).

---

## Task 5: Close NAI-114 (after smoke confirmation)

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (append cascade-attribution entry per `nai_followups`)
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (only if a NEW memory entry is being added — most likely no new entries)

**Why:** Per `close_commit_memory_trailer` and `nai_followups`. Cascade attribution must be recorded so NAI-115 brainstorming can grep for the prerequisite link.

- [ ] **Step 5.1: Append the cascade-attribution entry to `nai_followups.md`**

Append a new entry under the active section:

```markdown
- **NAI-114 Stage 2** [closed YYYY-MM-DD via close commit]: ported MAP_LOCADDUNSAFE handler. Smoke confirmed `<cascade-advance | content-gate>` outcome. **Cascade**: 7 secondary missing handlers (INV_DROPSLOT, OBJ_DEL, OBJ_COORD, OBJ_ADDALL, OBJ_ADD, LINEOFWALK, P_OPOBJ) routed to NAI-115 per `scope_gate_prerequisite_chain` (Stage 1.2 audit §5 escalation; ~225-275 prod LOC). NAI-115 entry brainstorm: enumerate all 7 handlers' TS source line ranges; consider whether content-gate outcomes change priority order.
```

- [ ] **Step 5.2: Audit any existing memory entries that need updating**

Per `post_task_handoff` — re-read these memories for currency:

- `disasm_reframes_inferred_binding.md` — was Bundle 0 disasm sufficient to bind H3? (Yes — Stage 1.1 + 1.2 confirmed.)
- `bundle0_short_circuits_stage1_audit.md` — does this NAI's pattern fit? (Bundle 0 narrowed but did NOT short-circuit; Stage 1.2 audit was still required.)
- `audit_subagent_fabrication.md` — did the controller HEAD-verification surface fabricated claims? (No — audit was clean per Stage 1.2 §7.)

If any entry has new instances to record, update the body or add a one-line index entry to `MEMORY.md` (under 200 chars per the warning at MEMORY.md tail). **Skip if no surprising/non-obvious updates.**

- [ ] **Step 5.3: Write the close commit**

```bash
git add ~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
# (Plus any other updated memory files)
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-114 — port MAP_LOCADDUNSAFE; cascade routes to NAI-115

Stage 2 ports MAP_LOCADDUNSAFE handler (TS ServerOps.ts:212-252) and
unblocks the firemaking-script chain past PC 25 of [label,light_logs_inv].
Smoke 2026-05-XX confirms <cascade-advance|content-gate> outcome.

Secondary cascade (INV_DROPSLOT, OBJ_DEL, OBJ_COORD, OBJ_ADDALL,
OBJ_ADD, LINEOFWALK, P_OPOBJ) escalated to NAI-115 per
scope_gate_prerequisite_chain. ~225-275 prod LOC across 7 handlers.

Closes memory: nai_followups
EOF
)"
```

- [ ] **Step 5.4: Emit the resume prompt for NAI-115**

Per `post_task_handoff` — prepare a paste-ready handoff for the user's next session:

> NAI-114 closed (commit `<sha>`). Smoke confirmed `<outcome>`.
>
> **Next: NAI-115 brainstorm** — port the 7-handler cascade unblocked by NAI-114 Stage 2:
>   - INV_DROPSLOT (OpInvDropSlot=4312, TS InvOps.ts:213; ~40-60 prod LOC)
>   - OBJ_DEL (OpObjDel=3504, TS ObjOps.ts:112; ~15-20 prod LOC)
>   - OBJ_COORD (OpObjCoord=3502, TS ObjOps.ts:163; ~10-15 prod LOC)
>   - OBJ_ADDALL (OpObjAddAll=3501, TS ObjOps.ts:58; ~40-60 prod LOC)
>   - OBJ_ADD (OpObjAdd=3500, TS ObjOps.ts:20; ~40-60 prod LOC)
>   - LINEOFWALK (OpLineOfWalk=1006, TS ServerOps.ts:65; ~20-30 prod LOC)
>   - P_OPOBJ (OpPOpObj=2080, TS PlayerOps.ts:990; ~20-30 prod LOC)
>
> Apply `superpowers:brainstorming`. Per `runescript_cadence`: brainstorm → spec → plan → subagent-driven TDD. Bundle decision is open — 7 sibling-shape handlers may bundle as one Stage 2 (single TDD pass per handler), or split if any individual handler's TS reference exceeds typical Shape A scope (e.g., OBJ_ADD's RNG-stack interactions).
>
> Smoke binding for NAI-115 close: full firemaking ignition produces fire loc + xp grant + ash drop on Tutorial Island.
