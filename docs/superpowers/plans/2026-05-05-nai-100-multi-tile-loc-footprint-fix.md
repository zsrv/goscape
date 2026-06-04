# NAI-100 — Multi-Tile Loc Footprint Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `pkg/gamemap.loadLocs` consume the LocType registry's `Width`/`Length` fields so static-loc entities carry their true multi-tile footprint, fixing the Lumbridge fountain (and ~all multi-tile static locs) collision-write and pathfinder-approach paths.

**Architecture:** Approach 1 setter-variant (per spec §3). Add `gm.SetLocTypes(*objtype.LocTypeConfigs)`; `loadLocs` looks up `lt.Width, lt.Length` from the registry and falls back to `1, 1` when the registry is nil (preserves test fixtures) or the locID is unknown (log-warn, goscape defensive — TS aborts via `printFatalError`). Hoist `objtype.LoadLocTypes` above `gm.Init` in `modules/world/server.go` so the production boot path threads correct W/L into entities at construction time.

**Tech Stack:** Go 1.26+; goscape engine; LostCityRS/Engine-TS canonical reference (`Engine-TS/src/engine/GameMap.ts:248-263`).

**Spec:** `docs/superpowers/specs/2026-05-05-nai-100-multi-tile-loc-footprint-fix-design.md`

**Predecessor:** NAI-99 diagnosis at `docs/superpowers/investigations/2026-05-05-nai-99-diagnosis.md`.

---

## File map

- **Modify** `pkg/gamemap/gamemap.go` — add `locTypes` field + `SetLocTypes` method.
- **Modify** `pkg/gamemap/load.go` — replace hardcoded `1, 1` with `lt.Width, lt.Length` lookup at line 190; drop TODO at line 135-136; update `loadLocs` doc-comment.
- **Modify** `modules/world/server.go` — hoist `objtype.LoadLocTypes(cfg.CachePath)` block (currently lines 235-241) above `gm.Init` (line 178-182); call `gm.SetLocTypes(locTypes)` between them.
- **Modify** `pkg/gamemap/nai99_fountain_dump_test.go` — drop the `t.Skip(...)` block (lines 163-176) and reorder the `LoadLocTypes`/`SetLocTypes` call before `gm.Init` so the production loader path is exercised.
- **Create** `pkg/gamemap/load_test.go` — two new unit tests covering the LocType-aware lookup and the nil-fallback path.

## Pre-flight verification (controller-side, before Task 1 dispatch)

Per `controller_preflight` memory, controller verifies these premises against HEAD before each implementer dispatch:

1. `pkg/gamemap/load.go:190` still reads `entity.NewLoc(actualLevel, absX, absZ, 1, 1, ...)`.
2. `pkg/gamemap/load.go:135-136` still has the `Footprint hardcoded to 1x1 ... TODO(loctype)` doc-comment.
3. `pkg/gamemap/gamemap.go` `GameMap` struct has fields `Pathfinder`, `multimap`, `freemap`, `mData`, `lData`, `landsByMapSquare`, `staticLocs`, `npcSpawns`, `log` (and no `locTypes` field yet).
4. `modules/world/server.go:178-182` calls `gm := gamemap.New(logger); gm.Init(cfg.CachePath)`.
5. `modules/world/server.go:235-241` calls `locTypes, err := objtype.LoadLocTypes(cfg.CachePath); ...; s.locTypes = locTypes`.
6. `pkg/gamemap/nai99_fountain_dump_test.go:163-176` contains the `t.Skip(...)` block.
7. `pkg/gamemap/nai99_fountain_dump_test.go:178-185` order: `gm.Init(cacheDir)` THEN `objtype.LoadLocTypes(cacheDir)`.
8. `pkg/objtype/loctype.go:204` `func LoadLocTypes(dir string)` takes only a directory string (no cross-type deps).
9. `pkg/entity/entity.go` exports `Entity.Width`, `Entity.Length` (int).
10. `pkg/objtype/loctype.go:19-29` `LocType` struct has exported `Width int` (default 1) and `Length int` (default 1).

If any premise fails, halt and reconcile before dispatching Task 1.

---

### Task 1: Add `locTypes` field + `SetLocTypes` setter on `GameMap`

**Files:**
- Modify: `pkg/gamemap/gamemap.go`
- Test: `pkg/gamemap/load_test.go` (created in Task 2 — Task 1 has no dedicated test; verified by Task 2's tests)

**Rationale:** The setter-on-GameMap pattern allows the production boot to pass the LocType registry to `loadLocs` without forcing every `gm.Init(t.TempDir())` test caller to thread a new parameter (preserves ~14 test sites unchanged). The setter is called BEFORE `Init` for footprint correctness; calling it after Init has no effect on already-loaded static locs (NAI-100 doesn't add a re-load path; this matches the production lifecycle where Init runs once at boot).

- [ ] **Step 1.1: Add the import**

Read `pkg/gamemap/gamemap.go` lines 1-13 (current import block). Add `"github.com/zsrv/goscape/pkg/objtype"` to the import block, alphabetized between `"github.com/zsrv/goscape/pkg/entity"` and `"github.com/zsrv/goscape/pkg/pathfinder/collision"`.

After the edit, the import block looks like:

```go
import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)
```

- [ ] **Step 1.2: Add the `locTypes` field to the `GameMap` struct**

Read `pkg/gamemap/gamemap.go:21-32` (the `GameMap` struct). Add `locTypes *objtype.LocTypeConfigs` after the `npcSpawns` field, before `log`. Updated struct:

```go
// GameMap holds collision data for the game world.
type GameMap struct {
	Pathfinder       *routefinder.PathFinderAPI
	multimap         map[int]bool      // packed zone coord -> multi combat
	freemap          map[int]bool      // packed zone coord -> F2P
	mData            map[uint16][]byte // (mapX<<8)|mapZ -> raw m{x}_{z} bytes (sub-spec 5b)
	lData            map[uint16][]byte // (mapX<<8)|mapZ -> raw l{x}_{z} bytes (sub-spec 5b)
	landsByMapSquare map[uint16][]int8 // (mapX<<8)|mapZ -> mapLevels*64*64 land bytes; populated by loadGround, consumed by loadLocs (NAI-96 LINK_BELOW)
	staticLocs       []*entity.Loc     // parsed static locs with absolute world coords
	npcSpawns        []NpcSpawn
	locTypes         *objtype.LocTypeConfigs // optional; when set before Init, loadLocs uses lt.Width/lt.Length per LocType (NAI-100). nil-OK preserves t.TempDir() test fixtures.
	log              *slog.Logger
}
```

- [ ] **Step 1.3: Add `SetLocTypes` method**

Insert the method immediately after `New(...)` (after line 45 in current `pkg/gamemap/gamemap.go`):

```go
// SetLocTypes registers the LocType configs used by loadLocs to thread
// per-instance Width/Length into static *entity.Loc construction. Must be
// called BEFORE Init for static-loc footprint correctness; calling later
// has no effect on already-loaded static locs. nil-OK: when unset,
// loadLocs falls back to 1×1 (preserves test fixtures with empty caches).
// Mirrors TS GameMap.ts:248-263 where loadLocations consults LocType.get().
func (gm *GameMap) SetLocTypes(cfgs *objtype.LocTypeConfigs) {
	gm.locTypes = cfgs
}
```

- [ ] **Step 1.4: Build the package to verify the changes compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/gamemap/...`
Expected: clean build, no output.

- [ ] **Step 1.5: Run existing pkg/gamemap tests to ensure no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/...`
Expected: all existing tests pass; the NAI-99 coverage test still SKIPs (Task 3 lifts).

- [ ] **Step 1.6: Commit**

```bash
git add pkg/gamemap/gamemap.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(gamemap): NAI-100 T1 — add SetLocTypes setter on GameMap

Threads *objtype.LocTypeConfigs into the gamemap so loadLocs (Task 2)
can read lt.Width/lt.Length per static-loc instance. nil-OK to
preserve t.TempDir() test fixtures unchanged. Mirrors TS GameMap.ts
:248-263 (LocType.get() at static-loc construction time).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Update `loadLocs` to consume `lt.Width, lt.Length` (TDD)

**Files:**
- Modify: `pkg/gamemap/load.go:135-196` (loadLocs body + doc comment)
- Test: `pkg/gamemap/load_test.go` (new)

**TDD note:** The test is in-package (`package gamemap`) so it can call the unexported `gm.loadLocs(data, mapSquareX, mapSquareZ)` directly without going through `Init`. This avoids needing real m/l files on disk.

- [ ] **Step 2.1: Write the failing tests**

Create `pkg/gamemap/load_test.go`:

```go
package gamemap

import (
	"io"
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// buildSyntheticLocStream encodes one static-loc placement of locID
// at local-coord (0, 0, 0) of a mapsquare, with shape=10 (LayerGround
// centrepiece) and angle=0. The byte format mirrors loadLocs's reader:
//
//	PSmart(locID + 1)  -- delta brings -1 → locID
//	PSmart(1)          -- coordDelta=1, coord=0 → localX=0, localZ=0, level=0
//	P1(info)           -- info = (shape << 2) | (angle & 0x3)
//	PSmart(0)          -- end inner loop (next coordDelta == 0)
//	PSmart(0)          -- end outer loop (next locDelta == 0)
func buildSyntheticLocStream(locID int, shape, angle int) []byte {
	buf := packet.NewPacket(make([]byte, 0, 16))
	buf.PSmart(int32(locID + 1))
	buf.PSmart(1)
	buf.P1(uint8((shape << 2) | (angle & 0x3)))
	buf.PSmart(0)
	buf.PSmart(0)
	return buf.Data
}

// newDiscardGameMap returns a GameMap with a discarding logger so test
// output stays clean.
func newDiscardGameMap() *GameMap {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestLoadLocs_UsesLocTypeWidthLength pins the post-NAI-100 behavior:
// when SetLocTypes is called before loadLocs, the resulting *entity.Loc
// carries the LocType's W×L (not the legacy hardcoded 1×1).
func TestLoadLocs_UsesLocTypeWidthLength(t *testing.T) {
	gm := newDiscardGameMap()

	cfgs := &objtype.LocTypeConfigs{
		Configs: make([]*objtype.LocType, 100),
	}
	cfgs.Configs[42] = &objtype.LocType{Width: 2, Length: 3}
	gm.SetLocTypes(cfgs)

	data := buildSyntheticLocStream(42, 10, 0)
	gm.loadLocs(data, 50, 50)

	if len(gm.staticLocs) != 1 {
		t.Fatalf("staticLocs: got %d, want 1", len(gm.staticLocs))
	}
	loc := gm.staticLocs[0]
	if loc.Type() != 42 {
		t.Errorf("Type: got %d, want 42", loc.Type())
	}
	if loc.Width != 2 {
		t.Errorf("Width: got %d, want 2 (lt.Width)", loc.Width)
	}
	if loc.Length != 3 {
		t.Errorf("Length: got %d, want 3 (lt.Length)", loc.Length)
	}
	if loc.X != 50*64+0 || loc.Z != 50*64+0 || loc.Level != 0 {
		t.Errorf("coords: got (%d,%d,%d), want (3200,3200,0)", loc.X, loc.Z, loc.Level)
	}
}

// TestLoadLocs_NilLocTypesFallback pins the test-fixture path:
// when SetLocTypes was never called, loadLocs falls back to 1×1 and
// does not log any "LocType" warnings (the warnings only fire when
// gm.locTypes != nil but the entry is missing/out-of-range).
func TestLoadLocs_NilLocTypesFallback(t *testing.T) {
	gm := newDiscardGameMap()
	// Note: no SetLocTypes call.

	data := buildSyntheticLocStream(42, 10, 0)
	gm.loadLocs(data, 50, 50)

	if len(gm.staticLocs) != 1 {
		t.Fatalf("staticLocs: got %d, want 1", len(gm.staticLocs))
	}
	loc := gm.staticLocs[0]
	if loc.Type() != 42 {
		t.Errorf("Type: got %d, want 42", loc.Type())
	}
	if loc.Width != 1 {
		t.Errorf("Width: got %d, want 1 (nil-locTypes fallback)", loc.Width)
	}
	if loc.Length != 1 {
		t.Errorf("Length: got %d, want 1 (nil-locTypes fallback)", loc.Length)
	}
}
```

- [ ] **Step 2.2: Run the new tests to verify they fail (red)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestLoadLocs -v`
Expected:
- `TestLoadLocs_UsesLocTypeWidthLength` FAILS — "Width: got 1, want 2" / "Length: got 1, want 3" (loadLocs still hardcodes 1, 1).
- `TestLoadLocs_NilLocTypesFallback` PASSES (current behavior already returns 1, 1).

If both fail or `TestLoadLocs_NilLocTypesFallback` fails, the test fixture is wrong — debug before proceeding.

- [ ] **Step 2.3: Update the `loadLocs` doc comment**

Read `pkg/gamemap/load.go:112-136` (the doc comment block above `loadLocs`). Replace lines 135-136:

```
// Footprint hardcoded to 1x1 until LocType config loading lands.
// TODO(loctype): use LocType.Width/Length.
```

with:

```
// Per-instance footprint (Width, Length) is read from the LocType registry
// when SetLocTypes was called before Init. When the registry is unset
// (e.g., empty-cache test fixtures) or the locID is missing/out-of-range
// (goscape defensive; TS calls printFatalError — see GameMap.ts:249-252),
// falls back to 1×1 with a log-warn for the missing-LocType branches.
// Mirrors TS GameMap.ts:248-263.
```

- [ ] **Step 2.4: Replace the hardcoded `1, 1` at the `entity.NewLoc` call**

Read `pkg/gamemap/load.go:186-193`. Replace:

```go
				if actualLevel < 0 {
					continue
				}

				loc := entity.NewLoc(actualLevel, absX, absZ, 1, 1,
					entity.LifecycleRespawn,
					locID, shape, angle)
				gm.staticLocs = append(gm.staticLocs, loc)
```

with:

```go
				if actualLevel < 0 {
					continue
				}

				width, length := 1, 1
				if gm.locTypes != nil {
					if locID >= 0 && locID < len(gm.locTypes.Configs) {
						if lt := gm.locTypes.Configs[locID]; lt != nil {
							width, length = lt.Width, lt.Length
						} else {
							// (goscape defensive; TS calls printFatalError on missing LocType — see GameMap.ts:249-252)
							gm.log.Warn("loadLocs: nil LocType for locID; using 1x1 fallback",
								"locID", locID, "mapSquareX", mapSquareX, "mapSquareZ", mapSquareZ)
						}
					} else {
						// (goscape defensive; TS calls printFatalError on missing LocType — see GameMap.ts:249-252)
						gm.log.Warn("loadLocs: locID out of range; using 1x1 fallback",
							"locID", locID, "mapSquareX", mapSquareX, "mapSquareZ", mapSquareZ)
					}
				}

				loc := entity.NewLoc(actualLevel, absX, absZ, width, length,
					entity.LifecycleRespawn,
					locID, shape, angle)
				gm.staticLocs = append(gm.staticLocs, loc)
```

- [ ] **Step 2.5: Run the new tests to verify they pass (green)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestLoadLocs -v`
Expected: both `TestLoadLocs_UsesLocTypeWidthLength` and `TestLoadLocs_NilLocTypesFallback` PASS.

- [ ] **Step 2.6: Run the full pkg/gamemap suite to verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/...`
Expected: all tests pass; NAI-99 coverage test still SKIPs (Task 3 lifts).

- [ ] **Step 2.7: Commit**

```bash
git add pkg/gamemap/load.go pkg/gamemap/load_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(gamemap): NAI-100 T2 — loadLocs consumes lt.Width/lt.Length

Per-instance static-loc footprint now reflects the LocType registry
when SetLocTypes was called before Init. Drops the load.go:135-136
TODO. nil-locTypes / unknown-locID branches fall back to 1×1 with a
log-warn (D1 deviation: TS aborts via printFatalError).

Tests: in-package TestLoadLocs_UsesLocTypeWidthLength pins the
post-fix path; TestLoadLocs_NilLocTypesFallback pins the test-fixture
compat for the ~14 callers of gm.Init(t.TempDir()).

Mirrors TS GameMap.ts:248-263.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Boot reorder + lift skip on NAI-99 coverage test

**Files:**
- Modify: `modules/world/server.go` (hoist LoadLocTypes; call SetLocTypes before Init)
- Modify: `pkg/gamemap/nai99_fountain_dump_test.go` (drop t.Skip; reorder LoadLocTypes/SetLocTypes before Init)

**Why combined:** The skip-lift requires both the boot reorder (production path) AND the test-side reorder (test now exercises the production code path); committing them together avoids a transient red between commits.

- [ ] **Step 3.1: Hoist `LoadLocTypes` in modules/world/server.go**

Read `modules/world/server.go:178-241`. The current shape:

```go
	gm := gamemap.New(logger)
	if err := gm.Init(cfg.CachePath); err != nil {
		return nil, fmt.Errorf("failed to load game map: %w", err)
	}
	s.gamemap = gm

	params, err := objtype.LoadParams(cfg.CachePath)
	... [56 lines: params, objTypes, invTypes, dbTableTypes, dbRowTypes, varpTypes, varsTypes, enumTypes, structTypes loaded] ...
	locTypes, err := objtype.LoadLocTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load loc types: %w", err)
	}
	s.enumTypes = enumTypes
	s.structTypes = structTypes
	s.locTypes = locTypes
```

Make two surgical edits.

**Edit A:** Replace the `gm := gamemap.New(...)` block (lines 178-182):

Old:
```go
	gm := gamemap.New(logger)
	if err := gm.Init(cfg.CachePath); err != nil {
		return nil, fmt.Errorf("failed to load game map: %w", err)
	}
	s.gamemap = gm
```

New:
```go
	locTypes, err := objtype.LoadLocTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load loc types: %w", err)
	}
	s.locTypes = locTypes

	gm := gamemap.New(logger)
	gm.SetLocTypes(locTypes)
	if err := gm.Init(cfg.CachePath); err != nil {
		return nil, fmt.Errorf("failed to load game map: %w", err)
	}
	s.gamemap = gm
```

**Edit B:** Delete the now-duplicated `locTypes` block at lines 235-241.

Old (4 lines starting around line 235):
```go
	locTypes, err := objtype.LoadLocTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load loc types: %w", err)
	}
```

Delete those 4 lines. Also delete the `s.locTypes = locTypes` assignment at line 241 (the assignment now lives at the new earlier site). The `s.enumTypes = enumTypes; s.structTypes = structTypes` lines at 239-240 stay as-is.

After both edits, the assignment block around the original line 239 should look like:
```go
	s.enumTypes = enumTypes
	s.structTypes = structTypes
	s.configsView = serverConfigsView{s: s}
```

(The `s.locTypes = locTypes` assignment moved to the earlier hoisted block; the rest of the assignments are unchanged.)

- [ ] **Step 3.2: Verify the existing `err` shadowing concern**

Note: at the original line 184 we have `params, err := objtype.LoadParams(cfg.CachePath)`. The `:=` here re-declares `err`. After the hoist, `err` is now first introduced higher up at the new `locTypes, err := objtype.LoadLocTypes(...)` line. The subsequent `:= ... err :=` uses are still valid Go (each block uses `:=` to reuse the same `err` slot when at least one LHS variable is new). No code change needed; just verify by building.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: clean build.

- [ ] **Step 3.3: Run modules/world tests to verify boot reorder is non-regressive**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all tests pass.

- [ ] **Step 3.4: Lift skip + reorder on TestNAI99_FountainCoverage_Lumbridge**

Read `pkg/gamemap/nai99_fountain_dump_test.go:152-200`. The current shape:

```go
func TestNAI99_FountainCoverage_Lumbridge(t *testing.T) {
	const fountainTypeID = 879

	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	t.Skip(`NAI-99: fountain footprint coverage divergence reproduces.

Observed (verbatim from Step 6.3 run):
  NAI-99 instance 0: typeID=879 origin=(2556,3113,0) shape=10 angle=3 W=2 L=2 (rotated W=2 L=2) flagged=[(2556,3113)=0x100] unflagged=[(2557,3113)=0x0 (2556,3114)=0x0 (2557,3114)=0x0]
  NAI-99: instance 0 footprint coverage divergence — flagged=[(2556,3113)=0x100] unflagged=[(2557,3113)=0x0 (2556,3114)=0x0 (2557,3114)=0x0] expected all 4 tiles flagged

Expected (post-NAI-100 fix): unflagged=[]; all 4 tiles carry FlagLoc (0x100).

Root cause per NAI-99 diagnosis report: H5 — pkg/gamemap/load.go:190
hardcodes entity.NewLoc(... 1, 1, ...) ignoring lt.Width/lt.Length;
production loc.Length/loc.Width=1,1 passed to ChangeLocCollision so
ChangeLoc loops 1×1=1. TODO at pkg/gamemap/load.go:136 acknowledges.

Stage 2 lifts in NAI-100.`)

	gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}
	cfgs, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}
```

Replace the entire block from `t.Skip(...` (line 163) through the cfgs load (line 185) with the reordered version (no t.Skip; LoadLocTypes/SetLocTypes BEFORE Init):

```go
	cfgs, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}

	gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	gm.SetLocTypes(cfgs)
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}
```

The remainder of the test (the collision-write replay loop and footprint coverage assertion) stays unchanged — `l.Length, l.Width` will now be the correct values from the entity, and the assertion will pass.

- [ ] **Step 3.5: Run the lifted test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestNAI99_FountainCoverage_Lumbridge -v`
Expected: PASS. Test output should include `NAI-99: 39 fountain instance(s) found for typeID=879` (or similar count) and no `unflagged` lines.

If the test FAILS with `unflagged=[…]`, the implementer should investigate before proceeding (likely a missed step in 3.1 or 3.4).

- [ ] **Step 3.6: Run the NAI-99 dump test for sanity**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestNAI99_FountainFootprintDump_Lumbridge -v`
Expected: PASS (this test was passing pre-NAI-100; the dump output will now show entity W/L matching lt.Width/lt.Length, which is the goal — but the test doesn't assert on entity W/L, so it stays green either way).

- [ ] **Step 3.7: Run the entire pkg/gamemap and modules/world suites**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/... ./modules/world/...`
Expected: all green.

- [ ] **Step 3.8: Commit**

```bash
git add modules/world/server.go pkg/gamemap/nai99_fountain_dump_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,gamemap): NAI-100 T3 — boot reorder + lift NAI-99 coverage skip

modules/world/server.go: hoist objtype.LoadLocTypes(cfg.CachePath)
above gm.Init so gm.SetLocTypes(locTypes) precedes the static-loc
load and per-instance Width/Length is correct at entity construction
time.

pkg/gamemap/nai99_fountain_dump_test.go: drop t.Skip block from
TestNAI99_FountainCoverage_Lumbridge (NAI-99 Stage 2 success
criterion). Reorder LoadLocTypes/SetLocTypes before Init to mirror
the production boot path.

Mirrors TS GameMap.ts:248-263.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Full-suite verification

- [ ] **Step 4.1: Run the entire test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all packages pass.

If anything fails, the implementer must investigate. Particular concerns:
- A test in `modules/world/` that asserts on hardcoded loc.Width=1 or loc.Length=1 (none expected per spec §6 R4, but verify).
- A test in another package that constructs a `*entity.Loc` and depends on a specific W/L value (none expected).

If pre-existing failures appear that are NOT caused by NAI-100, run `git stash; git checkout HEAD~3` (the pre-NAI-100 baseline), re-run the suite to confirm the failure is pre-existing, then `git checkout main; git stash pop` and document the pre-existing failure in the close commit.

- [ ] **Step 4.2: Run with race detector for catastrophic-regression check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/gamemap/... ./modules/world/...`
Expected: green; no race conditions.

- [ ] **Step 4.3: Build the binary**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o $TMPDIR/goscape ./cmd/goscape`
Expected: clean build, binary written.

---

### Task 5: Close commit + memory update

- [ ] **Step 5.1: Update memory for NAI-99 fountain residual**

Per `nai_98_fountain_footprint_residual` memory entry, NAI-99 closed with the residual routed to NAI-100. Update the memory file to mark NAI-100 as the closer:

Read `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_98_fountain_footprint_residual.md` and update the description and body to indicate NAI-100 closed the residual. Also update `MEMORY.md` index line accordingly.

- [ ] **Step 5.2: Add NAI-100 follow-ups memory entry (if any surfaced)**

If Task 4's full-suite run surfaced ANY pre-existing failures or adjacent untracked divergences, add them to `nai_followups.md`. Otherwise skip this step.

- [ ] **Step 5.3: Close commit**

```bash
git add -A
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-100 — multi-tile Loc footprint coverage fix (Stage 2 of NAI-99)

Closes the H5 root cause from NAI-99 diagnosis: pkg/gamemap/load.go
:190 now consumes lt.Width/lt.Length per static-loc instance via the
gm.SetLocTypes setter. Boot reorder in modules/world/server.go
hoists objtype.LoadLocTypes above gm.Init so the production path
threads correct W×L into *entity.Loc at construction time.
Downstream (populateStaticLocsIntoZones, world_zone.go runtime
ChangeLoc paths, interaction.go/npc_interaction.go findApproachPoint
sites) auto-fix because they read loc.Width/loc.Length from the
now-correct entity.

Test coverage:
- pkg/gamemap/nai99_fountain_dump_test.go::TestNAI99_FountainCoverage_Lumbridge
  skip lifted; passes with unflagged=[] for fountain instance 0.
- pkg/gamemap/load_test.go (new): TestLoadLocs_UsesLocTypeWidthLength
  + TestLoadLocs_NilLocTypesFallback pin the lookup and the
  test-fixture compat path.

D1 deviation: TS GameMap.ts:249-252 calls printFatalError on missing
LocType; goscape log-warns and falls back to 1×1 (no fatal-error
infra; soft-degrade matches existing loadLocs tolerance for missing
l-files).

Smoke handoff: user runs server + Java client; walks NW from
Lumbridge spawn (3221, 3218) toward fountain (3221..3222,
3226..3227); expects to be unable to walk onto the 4 footprint
tiles and pathfinder routes around. Cascade-binds residuals per
cascade_theory_smoke_binding.

Closes memory: nai_98_fountain_footprint_residual

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5.4: Verify final tree state**

Run: `git log --oneline -6`
Expected: 4 NAI-100 commits (T1, T2, T3, close) above the NAI-99 close commit `b009ea9`.

Run: `git status`
Expected: clean working tree.

---

## Self-review checks performed by plan author

Spec coverage:
- §3.1 GameMap.SetLocTypes → Task 1 ✓
- §3.2 loadLocs lookup → Task 2 ✓
- §3.3 boot reorder → Task 3 (Step 3.1) ✓
- §5.1 lift skip on NAI-99 coverage test → Task 3 (Step 3.4) ✓
- §5.2 TestLoadLocs_UsesLocTypeWidthLength → Task 2 (Step 2.1) ✓
- §5.3 TestLoadLocs_NilLocTypesFallback → Task 2 (Step 2.1) ✓
- §5.4 smoke handoff → close-commit body (Step 5.3) ✓
- §7 D1 deviation labeled → Task 2 (Step 2.4 doc-comment + production code comments) ✓
- §8 success criteria → Task 4 (full-suite) ✓

Type consistency: `*objtype.LocTypeConfigs`, `gm.SetLocTypes`, `gm.locTypes`, `lt.Width`, `lt.Length` — names consistent across all 5 tasks.

Placeholder scan: no TBD/TODO/etc in plan body.

## Smoke handoff (post-merge, user-driven)

Per `smoke_test_server_handoff` memory: this implementer does NOT run the smoke. After Task 5 close commit lands, the controller asks the user to:

1. Build the binary: `CGO_ENABLED=0 go build -trimpath -o /tmp/goscape ./cmd/goscape` (with the `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix).
2. Run the server with the project's `config.yaml`.
3. Connect the Java client (`/home/owner/Code/github.com/LostCityRS/Client-Java`) and log in to a fresh character. Spawn coords default to Lumbridge.
4. From spawn `(3221, 3218)`, click NW toward the fountain `(3221..3222, 3226..3227)`.
5. **Expected:** player cannot walk onto any of the 4 footprint tiles; pathfinder routes around.
6. Bind result. Per `cascade_theory_smoke_binding`: if multi-tile loc collision works for the fountain, root cause is closed. If other multi-tile features still misbehave (path-around shape-blindness, etc.), residuals route to NAI-101+.
