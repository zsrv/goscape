# NAI-99 Multi-tile Loc Footprint Coverage Investigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stage 1 audit + pinned reproducer test for the NAI-98 fountain-footprint smoke residual; identify root cause for "Lumbridge fountain treated as 1 tile wide; player walks partway in then stuck", or document the diagnosis ceiling and route to NAI-100.

**Architecture:** Hybrid probe-then-diff. Stage 1.1 enumerates locs + FlagMap state in a Lumbridge-fountain bbox via the production loader. Stages 1.2 (H1), 1.3 (H2 Rust cross-check), 1.4 (H4 l-pack decoder read) audit each hypothesis in risk-weighted order; H3 (content-side composition) closes naturally from the 1.1 dump. Coverage-assertion repro test pins observed shape with `t.Skip("NAI-99: …")`. Diagnosis report compiled per-hypothesis. **No production code changes** — only tests under `pkg/gamemap/` and docs under `docs/superpowers/investigations/`.

**Tech Stack:** Go 1.26+. TS reference: `LostCityRS/Engine-TS` (per `ts_source_canonical_path`). Rust pathfinder reference: `2004scape/rsmod-pathfinder` AS HEAD if H2 escalates (per `rust_source_canonical_path` analogue + NAI-94 §risks precedent).

**Spec:** `docs/superpowers/specs/2026-05-05-nai-99-multi-tile-loc-footprint-investigation-design.md`

---

## File Structure

**Created:**
- `pkg/gamemap/nai99_fountain_dump_test.go` — two tests:
  1. `TestNAI99_FountainFootprintDump_Lumbridge` — loads real cache, replays `populateStaticLocsIntoZones` collision-write inside a Lumbridge-fountain bbox, dumps `(x, z, perInstance.Shape, perInstance.Angle, locID, locTypeID, locTypeName, lt.Width, lt.Length, lt.BlockWalk, lt.BlockRange, lt.Active, FlagMap[x,z,level])` per loc instance. No assertions; output captured into the diagnosis report.
  2. `TestNAI99_FountainCoverage_Lumbridge` — for the fountain LocType identified by Stage 1.1 (name match `*fountain*` or hand-supplied ID), asserts every tile in the W×L footprint (rotated by Angle) carries the expected flag. `t.Skip("NAI-99: …")` if reproduces.
- `docs/superpowers/investigations/2026-05-05-nai-99-diagnosis.md` — per-hypothesis verdict + file:line evidence + Stage 2 (NAI-100) handoff.

**Modified:**
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — append "From NAI-99" section.

**Read-only references (audit input):**
- `pkg/gamemap/gamemap.go` (NAI-96-current `ChangeLocCollision` dispatch; lines 61-78)
- `pkg/pathfinder/routefinder/api.go` (`ChangeFloor` lines 74-80, single-tile; `ChangeLoc` lines 82-99, W×L)
- `pkg/objtype/loctype.go` (`PostDecode` lines 162-176; default at line 183)
- `pkg/entity/loc.go` (per-instance `Shape()`/`Angle()`/`Layer()`; lines 49-59)
- `pkg/gamemap/load.go` (`loadLocs` line 137 — l-pack decoder)
- `modules/world/server.go:315-335` (`populateStaticLocsIntoZones` — boot-time collision-write site)
- `pkg/pathfinder/collision/flagmap.go` (`Get`/`Set`/`Add`/`Remove`/`IsZoneAllocated`)
- `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/GameMap.ts` (TS `changeLocCollision`; lines 326-341)
- `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/config/LocType.ts` (TS PostDecode)

---

## Conventions for this plan

- **Reproducer disposition:** the coverage assertion test is written as a *real* assertion against expected behavior (every footprint tile flagged). Run it. If it FAILS, wrap the body in a `t.Skip` block immediately above with `// NAI-99: …` and pin the OBSERVED shape (full assertion-failure output: which tiles were flagged, which weren't, lt.Width/lt.Length, perInstance.Shape) so NAI-100 has a precise diff target. If it PASSES, the symptom is *not* footprint coverage — leave it as a passing test and note the elimination in the diagnosis report. Per `skip_pin_full_struct_capture`: skip-pin values come from verbatim test output, not inferred fields.
- **No production code changes.** If a task surfaces a "smoking gun" one-line fix opportunity, document it in the diagnosis report's Stage 2 handoff section — do **not** apply it in NAI-99.
- **Subagent fabrication guard** (`audit_subagent_fabrication`, `verify_implementer_claims`): controller verifies every claimed file:line citation with `git show HEAD -- <file>` / `rg` / `Read` before merging into the diagnosis report. **Highest fabrication risk:** Stage 1.3 Rust rsmod-pathfinder read — controller-side independent verification mandatory; cite exact Rust file path and function signature.
- **`go` invocation prefix** (per global CLAUDE.md): every `go test`/`go build` runs as `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
- **Short-circuit policy:** each substage's verdict appended to the diagnosis doc immediately. If Stage 1.2 surfaces "stuck on adjacent loc, not the fountain" (H1), controller decides at that point whether to continue Stages 1.3/1.4 for completeness or close at H1. H3 verdict ("matches TS, content authoring") is reported regardless from the 1.1 dump.

---

## Task 1: Bundle 0 — controller pre-flight (no commits)

**Purpose:** Verify spec §3 pre-flight observations against HEAD before dispatching audit work. Stale citations cause wasted implementer cycles (`controller_preflight`).

**Files:** read-only.

- [ ] **Step 1.1: Verify NAI-98 close commit is the most recent NAI work**

```bash
git log --oneline -5
```

Expected: `4ed5acc chore(close): NAI-98 …` is at or near the top. The NAI-99 spec commit `4073c1f` should be at HEAD.

- [ ] **Step 1.2: Verify spec citation for `ChangeLocCollision` GroundDecor branch**

```bash
sed -n '52,78p' pkg/gamemap/gamemap.go
```

Expected: line 72 `case loc.LayerGroundDecor:` invokes `gm.Pathfinder.ChangeFloor(x, z, level, add)` only when `active == 1`. If the branch shape has changed (e.g., NAI-98 added a W×L loop), halt and update the spec.

- [ ] **Step 1.3: Verify `ChangeFloor` is single-tile**

```bash
sed -n '74,80p' pkg/pathfinder/routefinder/api.go
```

Expected: single-tile add/remove via `pf.Flags.Add(x, z, level, collision.FlagBlockWalk)`. No `width*length` loop. If a loop has been added, H2 has already been addressed — halt and re-evaluate the spec.

- [ ] **Step 1.4: Verify `ChangeLoc` is W×L for comparison**

```bash
sed -n '82,99p' pkg/pathfinder/routefinder/api.go
```

Expected: `for index := 0; index < width*length; index++` loop iterating with `deltaX := x + (index % width)`, `deltaZ := z + index/width`. This is the LayerGround template the dump test will validate against.

- [ ] **Step 1.5: Verify `populateStaticLocsIntoZones` BlockWalk gate**

```bash
sed -n '315,335p' modules/world/server.go
```

Expected: line 327 gate `if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk` — collision write is gated on `lt.BlockWalk` AT THE CALL SITE. Critical for H1: a LocType with `BlockWalk=false` is silently skipped here regardless of `Active`.

- [ ] **Step 1.6: Verify entity.Loc accessor surface**

```bash
sed -n '49,60p' pkg/entity/loc.go
```

Expected: `Type()` (bits 0..13), `Shape()` (bits 14..18), `Angle()` (bits 19..20), `Layer()` (bits 21..22 of `BaseInfo`). The dump test calls `l.Shape()`, `l.Angle()`, `l.Layer()`.

- [ ] **Step 1.7: Verify FlagMap accessor**

```bash
sed -n '30,55p' pkg/pathfinder/collision/flagmap.go
```

Expected: `Get(absoluteX, absoluteZ, level int) int` at line 30. The dump test calls `gm.Pathfinder.Flags.Get(x, z, level)`.

- [ ] **Step 1.8: Verify real cache fixture availability**

```bash
ls data/pack/server/maps/m48_50 data/pack/server/loc.dat 2>&1 | head -5
```

Expected: both present. If either is missing, the dump test t.Skipf cleanly (precedent in `pkg/gamemap/nai98_realcache_probe_test.go:31-36`).

- [ ] **Step 1.9: Record pre-flight outcome**

Write a 6-line summary in working notes (NOT a commit). Format:

```
NAI-99 Bundle 0 pre-flight at HEAD <git rev-parse --short HEAD>:
- Step 1.2 LayerGroundDecor branch: <match | mismatch — describe>
- Step 1.3 ChangeFloor single-tile: <confirmed | mismatch>
- Step 1.4 ChangeLoc W×L: <confirmed | mismatch>
- Step 1.5 server.go BlockWalk gate: <confirmed | else>
- Step 1.6 Loc accessors: <confirmed | mismatch>
- Step 1.7 FlagMap.Get: <confirmed | mismatch>
- Step 1.8 fixture: <available | unavailable>
```

These notes feed Task 7's diagnosis report §"Audit baseline."

---

## Task 2: Stage 1.1 — Fountain footprint dump test

**Hypothesis:** H1 enumeration + H3 natural close. Produces the input data Stages 1.2, 1.3, 1.4 consume.

**Files:**
- Create: `pkg/gamemap/nai99_fountain_dump_test.go`

- [ ] **Step 2.1: Write the dump test**

```go
package gamemap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestNAI99_FountainFootprintDump_Lumbridge loads real m48_50 (Lumbridge
// mapsquare), replays the production server.go:populateStaticLocsIntoZones
// collision-write loop in-test, and dumps every loc instance in the bbox
// around the Lumbridge fountain (NAI-98 smoke residual coords).
//
// User smoke 2026-05-05: walked NW from spawn (~3221, 3218); fountain is
// "multi-tile but treated as 1 tile wide; player walks partway in then
// stuck."
//
// bbox: x ∈ [3217..3225], z ∈ [3214..3220], level=0.
//
// Output is captured via t.Logf and lands in the NAI-99 diagnosis report
// as Stage 1.1 input. No assertions — this is a probe.
//
// Disposition: always passes; t.Skipf if cache fixture unavailable.
func TestNAI99_FountainFootprintDump_Lumbridge(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}

	cfgs, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}

	const (
		level = 0
		xLo   = 3217
		xHi   = 3225
		zLo   = 3214
		zHi   = 3220
	)

	type seen struct {
		x, z                                 int
		shape, angle, layer                  int
		ltID                                 int
		ltName                               string
		ltWidth, ltLength                    int
		blockWalk, blockRange                bool
		active                               int
	}
	var inBbox []seen

	// Replay populateStaticLocsIntoZones' collision-write gate
	// (modules/world/server.go:324-330) for every static loc — but only
	// inside the bbox, so the FlagMap stays focused and dump output is
	// manageable.
	for _, l := range gm.StaticLocs() {
		if l.Level != level {
			continue
		}
		if l.X < xLo || l.X > xHi || l.Z < zLo || l.Z > zHi {
			continue
		}
		ltID := l.Type()
		if ltID < 0 || ltID >= len(cfgs.Configs) {
			t.Logf("loc at (%d,%d) typeID %d out of range", l.X, l.Z, ltID)
			continue
		}
		lt := cfgs.Configs[ltID]
		if lt == nil {
			t.Logf("loc at (%d,%d) typeID %d nil config", l.X, l.Z, ltID)
			continue
		}
		// Mirror server.go:327 gate.
		if lt.BlockWalk {
			gm.ChangeLocCollision(l.Shape(), l.Angle(), lt.BlockRange,
				l.Length, l.Width, lt.Active, l.X, l.Z, l.Level, true)
		}
		inBbox = append(inBbox, seen{
			x: l.X, z: l.Z,
			shape: l.Shape(), angle: l.Angle(), layer: l.Layer(),
			ltID: ltID, ltName: lt.DebugName,
			ltWidth: lt.Width, ltLength: lt.Length,
			blockWalk: lt.BlockWalk, blockRange: lt.BlockRange,
			active: lt.Active,
		})
	}

	// Dump per-loc info. ltName fountain match is the primary identification key.
	t.Logf("=== NAI-99 Stage 1.1 loc dump: bbox x∈[%d,%d] z∈[%d,%d] level=%d ===", xLo, xHi, zLo, zHi, level)
	t.Logf("loc instances in bbox: %d", len(inBbox))
	for _, s := range inBbox {
		t.Logf("loc x=%d z=%d shape=%d angle=%d layer=%d locTypeID=%d name=%q W=%d L=%d BlockWalk=%v BlockRange=%v Active=%d",
			s.x, s.z, s.shape, s.angle, s.layer, s.ltID, s.ltName,
			s.ltWidth, s.ltLength, s.blockWalk, s.blockRange, s.active)
	}

	// Dump non-zero FlagMap state at every tile in bbox.
	t.Logf("=== NAI-99 Stage 1.1 FlagMap dump (post loc-collision-write replay) ===")
	flaggedCount := 0
	for z := zLo; z <= zHi; z++ {
		for x := xLo; x <= xHi; x++ {
			flag := gm.Pathfinder.Flags.Get(x, z, level)
			if flag == 0 {
				continue
			}
			t.Logf("flag x=%d z=%d level=%d = 0x%x", x, z, level, flag)
			flaggedCount++
		}
	}
	t.Logf("flagged tiles in bbox: %d", flaggedCount)
}
```

- [ ] **Step 2.2: Run the test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI99_FountainFootprintDump_Lumbridge -v ./pkg/gamemap/
```

Expected: PASS. **Capture the entire `-v` output** — it's the Stage 1.1 evidence input for Stages 1.2/1.3/1.4. Save to a scratch file (not committed):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI99_FountainFootprintDump_Lumbridge -v ./pkg/gamemap/ > "$TMPDIR/nai99_dump.log" 2>&1
wc -l "$TMPDIR/nai99_dump.log"
```

Expected: non-empty log file; line count > 30 (Lumbridge fountain area should have multiple loc instances).

- [ ] **Step 2.3: Spot-verify the dump quality**

```bash
grep -E "fountain|well_lumbridge|water" "$TMPDIR/nai99_dump.log"
grep -E "loc instances in bbox: " "$TMPDIR/nai99_dump.log"
grep -E "flagged tiles in bbox: " "$TMPDIR/nai99_dump.log"
```

Expected:
- At least one fountain-name match in the loc dump.
- `loc instances in bbox: N` for some N >= 1.
- `flagged tiles in bbox: M` for some M.

If the loc-instances count is 0, the bbox is wrong (off-by-one or wrong square) — **expand bbox to `(3210..3230, 3210..3225)` and re-run** before continuing. If no fountain name matches, dump the full output to the diagnosis report and proceed; the user's smoke coords may need refining.

- [ ] **Step 2.4: Commit the dump test**

```bash
git add pkg/gamemap/nai99_fountain_dump_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(gamemap): NAI-99 T2 — Lumbridge fountain footprint dump

Stage 1.1 audit input. Loads m48_50 via production loader, replays
populateStaticLocsIntoZones gate inside bbox x∈[3217,3225] z∈[3214,3220],
dumps (x, z, shape, angle, layer, locTypeID, name, W, L, BlockWalk,
BlockRange, Active) per loc instance plus non-zero FlagMap flags. No
assertions — feeds NAI-99 diagnosis.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Stage 1.2 — H1 verdict (BlockWalk gating + adjacent-loc identification)

**Hypothesis:** H1 — fountain LocType has `BlockWalk=false` (or `Active!=1`), so the collision write is skipped at `server.go:327`; the actual stuck tile belongs to an adjacent loc (basin, plinth, etc.).

**Files:** read-only audit. Output recorded inline as audit notes; final form lands in Task 7's diagnosis report.

- [ ] **Step 3.1: Identify the fountain LocType from the dump**

Open `$TMPDIR/nai99_dump.log` and grep for fountain-related names:

```bash
grep -i "fountain\|well_\|wishing" "$TMPDIR/nai99_dump.log"
```

Record the matching `locTypeID`, `name`, `W`, `L`, `BlockWalk`, `Active`, and the tile coords `(x, z)` of each instance. There may be one instance per W×L footprint (single multi-tile placement) OR multiple instances at adjacent tiles (content-side composition — H3).

If no fountain-name match, fall back to: identify all locs with `W>1 || L>1` in the dump (multi-tile candidates):

```bash
grep -E "W=[2-9]|L=[2-9]" "$TMPDIR/nai99_dump.log"
```

- [ ] **Step 3.2: Cross-reference each fountain LocType against the smoke "stuck tile"**

The user's smoke trace ("walks partway in") doesn't pin the exact stuck tile. To bound it, identify which tiles in the bbox are FlagBlockWalk-flagged and adjacent to an unflagged tile inside or near a fountain footprint:

```bash
grep "flag x=" "$TMPDIR/nai99_dump.log"
```

Cross-reference each flagged tile against the loc-dump entries at the same `(x, z)`. Record:
- Which loc instance(s) produced each flag.
- Whether the flag-producing loc is the fountain itself or an adjacent loc.
- Whether the fountain LocType has `BlockWalk=true` AND `Active==1` (would have written) or `BlockWalk=false` / `Active!=1` (silently skipped at the gate).

- [ ] **Step 3.3: Record H1 verdict**

Append to controller working notes (NOT yet committed):

```
H1 verdict: <CONFIRMED | ELIMINATED | PARTIAL>
- Fountain LocType: id=<N>, name=<X>, W=<W>, L=<L>, BlockWalk=<v>, Active=<a>
- Was server.go:327 gate skipped for fountain? <yes | no>
- Stuck-tile candidate(s) (flagged tiles inside/near fountain footprint): <list>
- Producing loc(s) for each candidate: <list>
- File:line evidence: <pkg/objtype/loctype.go:<N> for default; loc.dat decode for this ID>
```

If H1 confirms (e.g., fountain has BlockWalk=false → gate skips → stuck tile belongs to adjacent loc), short-circuit: skip Tasks 4 and 5 unless controller wants completeness. Document the decision in the diagnosis report.

---

## Task 4: Stage 1.3 — H2 verdict (Rust rsmod-pathfinder cross-check)

**Hypothesis:** H2 — fountain is a single multi-tile GroundDecor LocType; `ChangeFloor` writes only the origin tile in both TS and goscape; the canonical Rust rsmod-pathfinder may handle W×L internally, in which case goscape's `ChangeFloor` is the divergence.

**Run only if Stage 1.2 surfaces a single fountain LocType with `W>1 || L>1` AND `BlockWalk=true` AND `Active==1`. Skip otherwise.**

**Files:** read-only audit.

- [ ] **Step 4.1: Locate the rsmod-pathfinder Rust source**

Probable path: `/home/owner/Code/github.com/2004scape/rsmod-pathfinder/` or sibling. Search:

```bash
find /home/owner/Code/github.com/2004scape -maxdepth 3 -type d -name "*rsmod*" 2>/dev/null
find /home/owner/Code/github.com/2004scape -maxdepth 4 -type f -name "*.rs" 2>/dev/null | head -10
```

If not present locally, **halt** and record in the diagnosis report: "H2 Rust cross-check: rsmod-pathfinder source unavailable locally; verdict UNDETERMINED-BY-LOCAL-ABSENCE; NAI-100 must clone or use 2004scape/rsmod-pathfinder HEAD via web." Per `audit_subagent_fabrication`, never fabricate Rust citations.

- [ ] **Step 4.2: Read `change_floor` in rsmod-pathfinder**

```bash
grep -rn "change_floor\|fn change_floor" /home/owner/Code/github.com/2004scape/rsmod-pathfinder/ 2>/dev/null | head -10
```

Read the function body. Record:
- Does it iterate W×L (signature includes `width`/`length`)?
- Or is it strictly single-tile (signature `(x, z, level, add)`)?

- [ ] **Step 4.3: Read `change_loc_collision` (or analogue) in rsmod-pathfinder**

```bash
grep -rn "change_loc_collision\|fn changeLocCollision\|GROUND_DECOR\|GroundDecor" /home/owner/Code/github.com/2004scape/rsmod-pathfinder/ 2>/dev/null | head -20
```

Read the GroundDecor branch. Record whether it loops W×L itself before calling `change_floor`, or calls `change_floor(x, z, level, add)` once.

- [ ] **Step 4.4: Cross-reference TS to confirm equivalence**

```bash
grep -n "GROUND_DECOR\|changeFloor" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/GameMap.ts
```

Expected (already confirmed in spec §3): TS `changeLocCollision` GroundDecor branch calls `rsmod.changeFloor(x, z, level, add)` once — single-tile invocation. So if Rust `change_floor` is multi-tile-aware internally, TS gets multi-tile coverage *for free* via the rsmod call; goscape's port-equivalent `ChangeFloor` (also single-tile invocation, no internal W×L loop) is the divergence.

- [ ] **Step 4.5: Record H2 verdict**

Append to controller working notes:

```
H2 verdict: <CONFIRMED | ELIMINATED | UNDETERMINED-BY-LOCAL-ABSENCE>
- Rust rsmod change_floor signature: <(x,z,level,add) | (x,z,level,width,length,add) | other>
- Rust rsmod GroundDecor branch loops W×L externally? <yes | no | n/a>
- TS GameMap.ts:336-340 invokes rsmod.changeFloor with W/L? <no — single-tile invocation per HEAD read>
- File:line evidence: <Rust path:line; TS GameMap.ts:336-340>
- If CONFIRMED: NAI-100 must add W×L loop in goscape — likely at pkg/pathfinder/routefinder/api.go:74-80 ChangeFloor (signature change) OR pkg/gamemap/gamemap.go:73-75 LayerGroundDecor branch (caller-side loop).
```

---

## Task 5: Stage 1.4 — H4 verdict (l-pack per-instance Shape decode)

**Hypothesis:** H4 — fountain is a `LayerGround` centrepiece (shape 10/11) with `W=2,L=2`, but per-instance Shape decoded from the l-pack is wrong (e.g., decoded as 22 GroundDecor) → routes through `LayerGroundDecor` single-tile branch instead of `LayerGround` W×L `ChangeLoc`.

**Run only if Stage 1.2 dump shows the fountain instance with `shape == 22` (GroundDecor) AND H1/H2 don't already explain the smoke. Skip otherwise.**

**Files:** read-only audit.

- [ ] **Step 5.1: Read goscape's per-instance shape decoder**

```bash
sed -n '112,180p' pkg/gamemap/load.go
```

Record the bit layout used for `CurrentInfo` / `BaseInfo` writes: which bits the per-instance shape is packed into; how the loader extracts shape from the l-pack stream (e.g., `attribute >> 2` per JagFile decode convention); whether shape is masked to 5 bits (`& 0x1F`) at decode-time.

- [ ] **Step 5.2: Read TS l-pack decoder**

```bash
find /home/owner/Code/github.com/LostCityRS/Engine-TS -name "*.ts" | xargs grep -ln "loadLocs\|decodeLocs\|LocGrid\|class World" 2>/dev/null | head -5
```

Locate TS's loc-pack decoder. Most likely `src/engine/GameMap.ts` `decodeLocs` or `src/cache/Map.ts`. Read the per-instance shape extraction. Record the bit layout TS uses.

- [ ] **Step 5.3: Diff goscape vs TS bit layout**

Compare:
- Where in the per-loc l-pack stream is shape encoded?
- What bit shifts / masks does TS apply?
- What does goscape apply?

A divergence at this layer would silently misclassify centrepiece (10/11) as ground-decor (22) for some instances.

- [ ] **Step 5.4: Sanity-check fountain instance against expected shape**

If TS code references the fountain LocType by name, find the `shape` field on its LocType definition in the cache:

```bash
grep -rn "fountain" /home/owner/Code/github.com/LostCityRS/Engine-TS/data/src/scripts/ 2>/dev/null | head -10
```

Identify the `.loc` content config. Note: LocType `shape` and per-instance `shape` are different — the per-instance shape comes from the l-pack stream, the LocType `shape`/`shapes` is a model-list selector. The user-facing W×L coverage depends on per-instance shape (which selects layer).

- [ ] **Step 5.5: Record H4 verdict**

Append to controller working notes:

```
H4 verdict: <CONFIRMED | ELIMINATED | PARTIAL>
- Fountain instance per-Shape from dump: <N>
- TS expected per-Shape (if discoverable): <N | unknown>
- goscape l-pack decoder bit layout: <description; file:line>
- TS l-pack decoder bit layout: <description; file:line>
- Divergence: <described | none>
- File:line evidence: <pkg/gamemap/load.go:<N>; TS path:<N>>
```

---

## Task 6: Stage 2 reproducer — fountain coverage assertion

**Purpose:** Pin the observed-coverage shape with `t.Skip("NAI-99: …")` so NAI-100 has a precise success criterion (lift the skip, footprint coverage matches expected).

**Files:**
- Modify: `pkg/gamemap/nai99_fountain_dump_test.go` (append second test)

- [ ] **Step 6.1: Identify the fountain LocType ID and footprint origin from Task 3**

From Task 3.1's output, record:
- `fountainTypeID` — the LocType ID matching `*fountain*` (or hand-supplied per controller decision).
- `fountainX, fountainZ` — the (x, z) of the fountain instance with `W>1 || L>1`. If multiple instances (H3 content-composition), record all and note the test will assert on the first one.
- `fountainAngle` — the per-instance angle.
- `fountainW, fountainL` — `lt.Width, lt.Length`.

If Task 3 found NO fountain LocType in the bbox, **halt Task 6** and document in the diagnosis report under "diagnosis ceiling: no fountain LocType identifiable from bbox dump." This may indicate the user's coord estimate is off; NAI-100 may need a wider bbox or coord-from-client capture.

- [ ] **Step 6.2: Append the coverage assertion test**

Append to `pkg/gamemap/nai99_fountain_dump_test.go`:

```go
// TestNAI99_FountainCoverage_Lumbridge asserts every tile in the
// W×L footprint of the Lumbridge fountain LocType (rotated by Angle
// per the LayerGround swap convention at gamemap.go:67-71) carries
// the expected flag — FlagBlockWalk for GroundDecor active=1, FlagLoc
// for LayerGround.
//
// User smoke 2026-05-05: fountain "treated like 1 tile wide; player
// walks partway in then stuck." This test pins which footprint tiles
// are flagged vs unflagged after collision-write replay.
//
// Disposition: if reproduces (some footprint tiles unflagged), add a
// t.Skip wrapper above the body with full assertion-failure output
// per skip_pin_full_struct_capture; lifting the skip is NAI-100's
// success criterion.
func TestNAI99_FountainCoverage_Lumbridge(t *testing.T) {
	// Fountain identification — populated from Task 3.1 dump output.
	// If multiple instances, the first multi-tile instance is asserted;
	// remaining instances are dumped via t.Logf for completeness.
	const (
		fountainTypeID = -1 // <<< FILL IN from Task 3.1 dump
		// Set fountainTypeID = -1 to skip; otherwise plug the real ID.
	)
	if fountainTypeID < 0 {
		t.Skip("NAI-99: fountain LocType ID not yet identified from Stage 1.1 dump; populate the const after Task 3.1")
	}

	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}
	cfgs, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}

	// Replay collision-write globally (not bbox-limited) so we don't
	// miss adjacent-zone allocations that the W×L footprint may span.
	for _, l := range gm.StaticLocs() {
		ltID := l.Type()
		if ltID < 0 || ltID >= len(cfgs.Configs) {
			continue
		}
		lt := cfgs.Configs[ltID]
		if lt == nil || !lt.BlockWalk {
			continue
		}
		gm.ChangeLocCollision(l.Shape(), l.Angle(), lt.BlockRange,
			l.Length, l.Width, lt.Active, l.X, l.Z, l.Level, true)
	}

	// Find every static-loc instance of fountainTypeID.
	var fountains []*entity.Loc
	for _, l := range gm.StaticLocs() {
		if l.Type() == fountainTypeID {
			fountains = append(fountains, l)
		}
	}
	if len(fountains) == 0 {
		t.Fatalf("NAI-99: no fountain instance with typeID=%d found in StaticLocs", fountainTypeID)
	}
	t.Logf("NAI-99: %d fountain instance(s) found for typeID=%d", len(fountains), fountainTypeID)

	// Assert footprint coverage on the first multi-tile instance.
	lt := cfgs.Configs[fountainTypeID]
	for idx, f := range fountains {
		// Apply the same length/width swap as gamemap.go:67-71 for LayerGround
		// (NORTH/SOUTH = identity; EAST/WEST = swap). For LayerGroundDecor,
		// rotation does not swap — single-tile origin.
		w, l := lt.Width, lt.Length
		if loc.LayerOf(loc.Shape(f.Shape())) == loc.LayerGround {
			if f.Angle() != loc.AngleNorth && f.Angle() != loc.AngleSouth {
				w, l = l, w
			}
		}

		var unflagged []string
		var flagged []string
		for dz := 0; dz < l; dz++ {
			for dx := 0; dx < w; dx++ {
				tx, tz := f.X+dx, f.Z+dz
				flag := gm.Pathfinder.Flags.Get(tx, tz, f.Level)
				cell := fmt.Sprintf("(%d,%d)=0x%x", tx, tz, flag)
				if flag == 0 {
					unflagged = append(unflagged, cell)
				} else {
					flagged = append(flagged, cell)
				}
			}
		}
		t.Logf("NAI-99 instance %d: typeID=%d origin=(%d,%d,%d) shape=%d angle=%d W=%d L=%d (rotated W=%d L=%d) flagged=%v unflagged=%v",
			idx, fountainTypeID, f.X, f.Z, f.Level, f.Shape(), f.Angle(), lt.Width, lt.Length, w, l, flagged, unflagged)
		if idx == 0 {
			if len(unflagged) > 0 {
				t.Errorf("NAI-99: instance 0 footprint coverage divergence — flagged=%v unflagged=%v expected all %d tiles flagged",
					flagged, unflagged, w*l)
			}
		}
	}
}
```

Add the imports `"fmt"`, `"github.com/zsrv/goscape/pkg/entity"`, `"github.com/zsrv/goscape/pkg/pathfinder/loc"` to the file's import block.

- [ ] **Step 6.3: Plug the fountain typeID and run the test**

Update `fountainTypeID` to the value from Task 3.1. Then:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI99_FountainCoverage_Lumbridge -v ./pkg/gamemap/
```

Expected behaviors:
- **PASS** (all footprint tiles flagged): the symptom is *not* footprint coverage — H1/H2/H3/H4 must explain the user's "partway in" via a different mechanism (e.g., adjacent loc, BFS step-validator). Document elimination in the diagnosis report; leave the test as a passing regression test.
- **FAIL** with `unflagged=[…]`: the symptom IS footprint coverage. Capture the full t.Errorf output verbatim. Proceed to Step 6.4.

- [ ] **Step 6.4: If FAIL, wrap the test body in a t.Skip pin**

Wrap the test body (after the `fountainTypeID < 0` guard) in:

```go
	t.Skip(`NAI-99: fountain footprint coverage divergence reproduces.

Observed (from Step 6.3 run; TODO replace with verbatim t.Errorf output):
  instance 0: typeID=<N> origin=(<X>,<Z>,0) shape=<S> angle=<A> W=<W> L=<L> rotated W=<rW> L=<rL>
  flagged=[<list>]
  unflagged=[<list>]

Expected (post-NAI-100 fix): unflagged=[]; all <rW*rL> tiles carry FlagBlockWalk.

Stage 2 lifts in NAI-100. Root cause per diagnosis report: <H1|H2|H3|H4>.`)
```

Replace the placeholders with the actual verbatim output from Step 6.3 (per `skip_pin_full_struct_capture`).

- [ ] **Step 6.5: Re-run the test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI99_FountainCoverage_Lumbridge -v ./pkg/gamemap/
```

Expected: `--- SKIP: TestNAI99_FountainCoverage_Lumbridge` with the pinned skip message.

- [ ] **Step 6.6: Commit the coverage test**

```bash
git add pkg/gamemap/nai99_fountain_dump_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(gamemap): NAI-99 T6 — fountain coverage assertion repro

Stage 2 reproducer pin: asserts every tile in the Lumbridge fountain
W×L footprint (rotated by per-instance angle) carries the expected
flag. <PASS|SKIP> per Step 6.3 outcome. Skip body pins observed shape
verbatim per skip_pin_full_struct_capture; lifting is NAI-100's
success criterion.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Compile diagnosis report

**Purpose:** Per-hypothesis verdict + file:line evidence + Stage 2 (NAI-100) handoff.

**Files:**
- Create: `docs/superpowers/investigations/2026-05-05-nai-99-diagnosis.md`

- [ ] **Step 7.1: Write the diagnosis report**

Template:

```markdown
# NAI-99 Diagnosis — Multi-tile Loc Footprint Coverage Investigation

**Stage 1 of `investigation_subspec_cadence`.** Stage 2 routes to NAI-100.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-99-multi-tile-loc-footprint-investigation-design.md`
**Plan:** `docs/superpowers/plans/2026-05-05-nai-99-multi-tile-loc-footprint-investigation.md`

---

## Summary

<1-2 sentence summary of root cause OR diagnosis ceiling>

---

## Audit baseline (Bundle 0 controller pre-flight)

<Verbatim copy of Task 1 Step 1.9 working notes summary>

---

## Reproducer test results

| Test | Path | Disposition | Notes |
|---|---|---|---|
| `TestNAI99_FountainFootprintDump_Lumbridge` | `pkg/gamemap/nai99_fountain_dump_test.go` | <PASS — dump captured> | <line count of $TMPDIR/nai99_dump.log; brief shape summary> |
| `TestNAI99_FountainCoverage_Lumbridge` | same | <PASS | SKIP-pinned> | <if SKIP, paste skip message; if PASS, note coverage matches> |

---

## Per-hypothesis verdicts

### H1 — adjacent-loc / BlockWalk-gating

**Verdict:** <CONFIRMED | ELIMINATED | PARTIAL>

**Evidence (file:line):**
- <pkg/objtype/loctype.go:N — fountain LocType BlockWalk/Active>
- <modules/world/server.go:327 — gate behavior>
- Dump line(s): <quoted from $TMPDIR/nai99_dump.log>

**Implication:** <NAI-100 fix target OR closed>

### H2 — single multi-tile GroundDecor + ChangeFloor single-tile

**Verdict:** <CONFIRMED | ELIMINATED | UNDETERMINED-BY-LOCAL-ABSENCE | NOT-RUN>

**Evidence:**
- <Rust rsmod-pathfinder path:N or "source unavailable locally">
- TS Engine-TS/src/engine/GameMap.ts:326-341 — single-tile rsmod.changeFloor invocation
- goscape pkg/pathfinder/routefinder/api.go:74-80 — single-tile ChangeFloor
- goscape pkg/gamemap/gamemap.go:72-75 — LayerGroundDecor branch

**Implication:** <NAI-100 fix target OR closed>

### H3 — N adjacent single-tile loc placements

**Verdict:** <CONFIRMED | ELIMINATED>

**Evidence:**
- Dump line(s): <list of fountain-name instances at adjacent (x,z) tiles>

**Implication:** <"matches TS, no fix" OR closed>

### H4 — l-pack per-instance Shape decode divergence

**Verdict:** <CONFIRMED | ELIMINATED | NOT-RUN>

**Evidence:**
- pkg/gamemap/load.go:<N> — goscape l-pack shape extraction
- TS path:<N> — TS l-pack shape extraction
- Diff: <described>

**Implication:** <NAI-100 fix target OR closed>

---

## Root cause

<file:line + 1-2 sentence summary; OR "diagnosis ceiling: NAI-100 needs X to break through.">

---

## Stage 2 (NAI-100) handoff

- **Root cause:** <as above>
- **Repro tests to lift skip on:** `TestNAI99_FountainCoverage_Lumbridge` <— expected post-fix behavior: `unflagged=[]`>
- **Files NAI-100 will touch:** <exact list>
- **Estimated LOC for fix:** <ballpark>
- **Residual hypotheses for NAI-101+:** <any divergences not in NAI-100 scope>
- **Smoke spec:** walk NW from Lumbridge spawn (3221, 3218) into the fountain footprint; verify all expected footprint tiles block; verify path-around routes correctly to NPCs on the far side.
```

- [ ] **Step 7.2: Fill in every section from Tasks 1–6 working notes**

Replace every `<...>` placeholder with concrete content. Verbatim citations only — controller verifies each `path:N` claim with `git show HEAD -- <file>` or `Read` before committing the report (per `audit_subagent_fabrication`).

- [ ] **Step 7.3: Commit the diagnosis report**

```bash
git add docs/superpowers/investigations/2026-05-05-nai-99-diagnosis.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-99 diagnosis report

Stage 1 diagnosis. Per-hypothesis verdicts (H1/H2/H3/H4), file:line
evidence, root cause <summary>, Stage 2 handoff for NAI-100.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Followups update + close commit

**Purpose:** Memory + close commit per `close_commit_memory_trailer`.

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`
- Possibly modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (if any new memory entries warranted)

- [ ] **Step 8.1: Read current nai_followups.md**

```bash
cat /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md | tail -30
```

Locate the existing structure (likely `## From NAI-N` sections in chronological order).

- [ ] **Step 8.2: Append "From NAI-99" section**

Append to the file:

```markdown

## From NAI-99

**Stage 2 (NAI-100) handoff:**
- **Root cause:** <as in diagnosis report>
- **Repro tests to lift skip on:** `pkg/gamemap/nai99_fountain_dump_test.go::TestNAI99_FountainCoverage_Lumbridge` <— expected post-fix `unflagged=[]`>
- **Files NAI-100 will touch:** <exact list>
- **Estimated LOC for fix:** <ballpark>
- **Smoke spec:** walk NW from Lumbridge spawn (3221, 3218) into the fountain footprint; verify all expected footprint tiles block; verify path-around routes correctly to NPCs on the far side.
- **Residual hypotheses for NAI-101+:** <any>
```

Replace placeholders with the diagnosis report's content verbatim.

- [ ] **Step 8.3: Verify nai_98_fountain_footprint_residual.md is the memory entry being closed**

```bash
cat /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_98_fountain_footprint_residual.md
```

Confirm this is the residual memory NAI-99 closes (per spec §9 close trailer).

- [ ] **Step 8.4: Add new memory entries if surfaced**

If Stages 1.2/1.3/1.4 surfaced new lessons (e.g., "Rust rsmod handles W×L internally; ChangeFloor port must too"), create a new memory file under `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/` and add a one-line index entry to `MEMORY.md`. Per global memory conventions: only save non-derivable lessons; the diagnosis report and code itself are the canonical source for derivable info.

- [ ] **Step 8.5: Close commit**

```bash
git status
```

Verify only memory files (untracked at `.claude/`) and possibly nothing in the goscape working tree (the spec/plan/test/diagnosis are already committed). If memory edits exist:

```bash
# memory files live outside the goscape working tree, so they're not staged via git add here.
# Just produce the close commit on the goscape side:
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-99 — multi-tile Loc footprint coverage investigation

Stage 1 closed. Root cause: <summary>. Stage 2 (NAI-100) takes over per
diagnosis report.

Closes memory: nai_98_fountain_footprint_residual.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If the close commit would be empty (all NAI-99 work already in prior commits), the `--allow-empty` flag is required. The trailer is the load-bearing artifact for `close_commit_memory_trailer` grep.

- [ ] **Step 8.6: Final verification**

```bash
git log --oneline -10
```

Expected: NAI-99 commit chain visible — spec, dump test (T2), coverage test (T6), diagnosis (T7), close (T8). All carry `Co-Authored-By: Claude Opus 4.7 (1M context)`.

---

## Self-Review Checklist (controller, before task dispatch)

- [ ] Spec §3 hypothesis register has a corresponding Task in this plan: H1→T3, H2→T4, H3→natural close in T2 dump, H4→T5. ✓
- [ ] Every code block compiles mentally: imports listed, identifiers exist (verified `entity.Loc.Shape()/Angle()/Layer()`, `objtype.LoadLocTypes`, `gm.Pathfinder.Flags.Get`, `loc.LayerOf`, `loc.AngleNorth/South`).
- [ ] No "TODO"/"TBD" placeholders in step-bodies (the `<...>` placeholders in template fillings are deliberate and tracked in step text).
- [ ] Every command uses the `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` prefix per global CLAUDE.md.
- [ ] Every commit uses `--no-gpg-sign` per global CLAUDE.md.
- [ ] Subagent fabrication risk called out at Tasks 4 (Rust read) and 5 (TS l-pack decoder read).
- [ ] Close commit carries `Closes memory: nai_98_fountain_footprint_residual.md` trailer per `close_commit_memory_trailer`.
- [ ] No production code changes anywhere in the plan.
