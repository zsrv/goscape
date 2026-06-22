# NAI-97 GroundDecor Over-Blocking + Reach Abandonment Stage 1 Investigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stage 1 audit + pinned reproducer tests for the NAI-96 GroundDecor path-around residual; identify root cause for "NPC 943 path found but abandoned without stepping" and "NPC 3 abandons mid-route at cheb=5", or document the diagnosis ceiling and route to NAI-98.

**Architecture:** Hybrid probe-then-diff. Stage 1.1 enumerates locs + FlagMap state around the smoke coords (input for H1/H2). Stages 1.2–1.6 audit each hypothesis in risk-weighted order, short-circuiting when a smoking gun is found. Diagnosis report compiled per-hypothesis. **No production code changes** — only tests under `pkg/gamemap/` + `pkg/pathfinder/routefinder/` and docs under `docs/superpowers/investigations/`.

**Tech Stack:** Go 1.26+. TS reference: `LostCityRS/Engine-TS` (per `ts_source_canonical_path`). Pathfinder reference: `2004scape/rsmod-pathfinder` AS HEAD if H4 escalates (per NAI-94 §risks).

**Spec:** `docs/superpowers/specs/2026-05-05-nai-97-grounddecor-reach-investigation-design.md`

---

## File Structure

**Created:**
- `pkg/gamemap/nai97_loc_walk_test.go` — loads real m48_50 + replays the production loc→collision pipeline; dumps `(x, z, layer, locID, locTypeID, locTypeName, BlockWalk, Active, FlagMap-flag)` per loc in a small bbox. No assertions; output captured into the diagnosis report.
- `pkg/pathfinder/routefinder/nai97_repro_test.go` — pinned reproducer tests for Repro A (NPC 943 path-around-fountain) and Repro B (NPC 3 mid-route abandonment); `t.Skip` if anomaly reproduces.
- `docs/superpowers/investigations/2026-05-05-nai-97-diagnosis.md` — per-hypothesis verdict + file:line evidence + Stage 2 (NAI-98) handoff.

**Modified:**
- `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — append "From NAI-97" section.

**Read-only references (audit input):**
- `pkg/gamemap/gamemap.go` (NAI-96-current ChangeLocCollision dispatch)
- `pkg/objtype/loctype.go` (PostDecode at lines 162-176; default at line 183)
- `pkg/pathfinder/routefinder/api.go` (FindPathPlain/FindPathToEntity/FindPathToLoc)
- `modules/world/interaction.go` (lines 571-680 dispatch arms)
- `modules/world/server.go:315-335` (populateStaticLocsIntoZones — boot-time collision-write site)
- `$HOME/Code/github.com/LostCityRS/Engine-TS/src/cache/config/LocType.ts` (TS PostDecode + BlockWalk decode)
- `$HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/GameMap.ts` (TS changeLocCollision; lines 326-341)

---

## Conventions for this plan

- **Reproducer disposition:** Each reproducer test is written as a *real* assertion against expected behavior. Run it. If it FAILS (anomaly reproduces), wrap the body in a `t.Skip` block immediately above with `// NAI-97: ...` and pin the OBSERVED behavior (full `Route` value via `%+v`) in the skip body so NAI-98 has a precise diff target. If it PASSES, leave it as a passing test and note the elimination in the diagnosis report. Per `skip_pin_full_struct_capture`: skip-pin values come from verbatim `%+v` output, not inferred fields.
- **No production code changes.** If a task surfaces a "smoking gun" one-line fix opportunity, document it in the diagnosis report's Stage 2 handoff section — do **not** apply it in NAI-97.
- **Subagent fabrication guard** (`audit_subagent_fabrication`, `verify_implementer_claims`): controller verifies every claimed file:line citation with `git show HEAD -- <file>` / `rg` / `Read` before merging into the diagnosis report. TS PostDecode comparisons (Stage 1.3) and pathfinder dispatch reads (Stage 1.5) are highest fabrication-risk surface.
- **`go` invocation prefix** (per global CLAUDE.md): every `go test`/`go build` runs as `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
- **Short-circuit policy:** each substage's verdict appended to the diagnosis doc immediately. If Stage 1.1+1.2 surface a smoking-gun BlockWalk diff for a LocType blocking the smoke path, controller decides at that point whether to continue Stages 1.3-1.6 for completeness or close at H1.

---

## Task 1: Bundle 0 — controller pre-flight (no commits)

**Purpose:** Verify spec §1 pre-flight observations against HEAD before dispatching audit work. Stale citations cause wasted implementer cycles (`controller_preflight`).

**Files:** read-only.

- [ ] **Step 1.1: Verify the NAI-96 close commit is the most recent NAI work**

```bash
git log --oneline -5
```

Expected: `27dbfbf chore(close): NAI-96 …` is at the top (or close to it; the NAI-97 spec commit `c3aeb6b` may sit above it).

- [ ] **Step 1.2: Verify spec citation for `ChangeLocCollision` signature**

```bash
sed -n '52,78p' pkg/gamemap/gamemap.go
```

Expected: line 61 declares `func (gm *GameMap) ChangeLocCollision(shape, angle int, blocksRange bool, length, width, active, x, z, level int, add bool)`. The `LayerGroundDecor` branch at line 72 invokes `ChangeFloor` only when `active == 1`. If the signature has changed, halt and update the spec.

- [ ] **Step 1.3: Verify spec citation for PostDecode coercion**

```bash
sed -n '162,190p' pkg/objtype/loctype.go
```

Expected: `PostDecode` at line 166 coerces `Active=-1` to `0` by default and to `1` when (Shapes.length==1 && Shapes[0]==10) OR `Op != nil`. Line 183 default `BlockWalk: true`.

- [ ] **Step 1.4: Verify pathfinder API names**

```bash
sed -n '36,72p' pkg/pathfinder/routefinder/api.go
```

Expected: `FindPathPlain` (line 40), `FindPathToEntity` (line 47), `FindPathToLoc` (line 54), `FindNaivePath` (line 62), `FindPath` (line 70). The memory `pathfinder_api_loc_aware` references `FindPathDefault` (renamed to `FindPathPlain`). Note the rename for the diagnosis report's "Risks" section but do NOT update that memory file in NAI-97 (separate followup).

- [ ] **Step 1.5: Verify interaction.go dispatch arms**

```bash
sed -n '615,675p' modules/world/interaction.go
```

Expected: shape-aware dispatch by target class — `*entitypkg.Loc` → `FindPathToLoc` (line 623), PathingEntity → `FindPathToEntity` (line 642) or `FindNaivePath` shortcut (line 639), `*entitypkg.Obj` diff → `FindPathPlain` (line 659/670).

- [ ] **Step 1.6: Verify `populateStaticLocsIntoZones` collision gate**

```bash
sed -n '315,335p' modules/world/server.go
```

Expected: line 327 gate `if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk` — collision write is gated on `lt.BlockWalk` AT THE CALL SITE, with the `lt.Active` param passed through to `ChangeLocCollision`. This is critical for H1: a locType with `BlockWalk=false` is silently skipped here regardless of `Active`.

- [ ] **Step 1.7: Verify real cache fixture availability**

```bash
ls data/pack/server/maps/m48_50 data/pack/server/loc.dat data/pack/client/config 2>&1 | head -10
```

Expected: all three present. If any are missing, the dump test in Task 2 will t.Skipf cleanly (precedent in `pkg/objtype/loctype_realcache_test.go:18-24`).

- [ ] **Step 1.8: Record pre-flight outcome**

Write a 5-line summary in your own working notes (NOT a commit). Format:

```
NAI-97 Bundle 0 pre-flight at HEAD <git rev-parse --short HEAD>:
- Step 1.2 ChangeLocCollision sig: <match | mismatch — describe>
- Step 1.3 PostDecode lines: <match | mismatch — describe>
- Step 1.4 pathfinder API names: <match | mismatch>
- Step 1.5 interaction.go arms: <match | mismatch>
- Step 1.6 server.go BlockWalk gate: <confirmed | else>
- Step 1.7 fixture: <available | unavailable>
```

These notes feed Task 9's diagnosis report §"Audit baseline."

---

## Task 2: Stage 1.1 — Loc-walk + FlagMap dump test

**Hypothesis:** H1 enumeration. Produces the input data Stages 1.2 (TS BlockWalk cross-ref) and 1.3 (PostDecode Active coercion) consume.

**Files:**
- Create: `pkg/gamemap/nai97_loc_walk_test.go`

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

// TestNAI97_LocWalkDump_Lumbridge loads real m48_50 (Lumbridge mapsquare),
// replays the production server.go:populateStaticLocsIntoZones collision-
// write loop in-test, and dumps every loc in the bbox around the NAI-96
// smoke residual coords:
//
//	NPC 943 reach: (3221, 3218) → (3218, 3216)
//	NPC   3 reach: (3218, 3213) → (3223, 3216)
//
// bbox: x ∈ [3215..3225], z ∈ [3211..3220], level=0.
//
// Output is captured via t.Logf and lands in the NAI-97 diagnosis report
// as Stage 1.1 input. No assertions — this is a probe.
//
// Disposition: always passes; t.Skipf if cache fixture unavailable.
func TestNAI97_LocWalkDump_Lumbridge(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	// Discard logger so test output isn't drowned in gamemap.Init INFO lines.
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
		xLo   = 3215
		xHi   = 3225
		zLo   = 3211
		zHi   = 3220
	)

	// Replay populateStaticLocsIntoZones' collision-write gate
	// (modules/world/server.go:324-330) for every static loc — but only
	// inside the bbox, so we don't pollute the FlagMap globally and so
	// the dump output stays manageable.
	type seen struct {
		x, z, layer, locID, ltID int
		ltName                   string
		blockWalk                bool
		active                   int
	}
	var inBbox []seen

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
		// Mirror server.go:327 gate: only write if lt.BlockWalk.
		if lt.BlockWalk {
			gm.ChangeLocCollision(l.Shape(), l.Angle(), lt.BlockRange,
				l.Length, l.Width, lt.Active, l.X, l.Z, l.Level, true)
		}
		inBbox = append(inBbox, seen{
			x: l.X, z: l.Z, layer: l.Layer(), locID: ltID,
			ltID: ltID, ltName: lt.DebugName,
			blockWalk: lt.BlockWalk, active: lt.Active,
		})
	}

	// Dump per-loc info.
	t.Logf("=== NAI-97 Stage 1.1 loc dump: bbox x∈[%d,%d] z∈[%d,%d] level=%d ===", xLo, xHi, zLo, zHi, level)
	t.Logf("locs in bbox: %d", len(inBbox))
	for _, s := range inBbox {
		t.Logf("loc x=%d z=%d layer=%d locTypeID=%d name=%q BlockWalk=%v Active=%d",
			s.x, s.z, s.layer, s.locID, s.ltName, s.blockWalk, s.active)
	}

	// Dump FlagMap state at every tile in bbox.
	t.Logf("=== NAI-97 Stage 1.1 FlagMap dump (post loc-collision-write replay) ===")
	for z := zLo; z <= zHi; z++ {
		for x := xLo; x <= xHi; x++ {
			flag := gm.Pathfinder.Flags.Get(x, z, level)
			if flag == 0 {
				continue // skip clean tiles to keep output focused
			}
			t.Logf("flag x=%d z=%d level=%d = 0x%x", x, z, level, flag)
		}
	}
}
```

- [ ] **Step 2.2: Run the test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI97_LocWalkDump_Lumbridge -v ./pkg/gamemap/
```

Expected: PASS. The verbose output will contain the loc dump and FlagMap dump. **Capture the entire `-v` output** — it's the Stage 1.1 evidence input for Stages 1.2 and 1.3. Save to a scratch file (not committed):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI97_LocWalkDump_Lumbridge -v ./pkg/gamemap/ > "$TMPDIR/nai97_dump.log" 2>&1
wc -l "$TMPDIR/nai97_dump.log"
```

Expected: non-empty log file; line count >> 50 (Lumbridge area is dense with locs).

- [ ] **Step 2.3: Spot-verify the dump quality**

```bash
grep -E "loc x=321[5-9]|loc x=322[0-5]" "$TMPDIR/nai97_dump.log" | head -20
grep -E "flag x=" "$TMPDIR/nai97_dump.log" | head -20
```

Expected: at least a handful of loc lines and at least a handful of non-zero FlagMap lines. If the loc dump has zero entries, the bbox is wrong or m48_50 is misaligned with the smoke coords — halt and revisit. If the FlagMap dump has zero entries, the collision-write replay didn't fire — halt and re-read Step 2.1 logic.

- [ ] **Step 2.4: Commit the test**

```bash
git add pkg/gamemap/nai97_loc_walk_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(gamemap): NAI-97 T2 — Lumbridge loc-walk + FlagMap dump

Stage 1.1 audit input. Loads m48_50 via production loader, replays
populateStaticLocsIntoZones gate inside bbox x∈[3215,3225] z∈[3211,3220],
dumps (x, z, layer, locTypeID, name, BlockWalk, Active) per loc plus
non-zero FlagMap flags. No assertions — feeds NAI-97 diagnosis.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Stage 1.2 — H1 verdict (TS BlockWalk cross-reference)

**Hypothesis:** H1 — TS-divergent over-blocking via `LocType.BlockWalk` default or per-LocType decode.

**Files:** read-only audit. Output recorded inline as audit notes; final form lands in Task 9's diagnosis report.

- [ ] **Step 3.1: Locate TS LocType BlockWalk decode**

```bash
rg -nE "blockwalk|blockWalk|BlockWalk|BLOCKWALK" $HOME/Code/github.com/LostCityRS/Engine-TS/src/cache/config/LocType.ts
```

Record file:line of every match. Specifically: the default value, the opcode that sets it, and the postDecode coercion (if any).

- [ ] **Step 3.2: Read the TS LocType.postDecode**

```bash
sed -n '195,225p' $HOME/Code/github.com/LostCityRS/Engine-TS/src/cache/config/LocType.ts
```

Identify whether TS coerces `blockwalk` based on shapes/op, and how that compares to goscape's `loctype.go:166-176` (`Active` coercion ONLY; `BlockWalk` is decoder-set, not PostDecode-coerced in goscape per spec §1).

- [ ] **Step 3.3: Cross-reference each enumerated LocType**

For every LocType ID enumerated in the Task 2 dump (extract from `$TMPDIR/nai97_dump.log`), record in a markdown table:

| LocTypeID | Name | goscape BlockWalk | goscape Active (post-PostDecode) | TS blockwalk | TS active | Divergent? |
|---|---|---|---|---|---|---|
| ... | ... | ... | ... | ... | ... | match / divergent |

To get the TS values for each ID, you need the TS cache decode output. Two options:
- (a) If the TS server has been run and a structured locType cache dump exists somewhere, use it. Grep for `.locType.json`, `loctypes.json`, or similar test fixtures in `LostCityRS/Engine-TS/data/`.
- (b) Otherwise: read the `LocType.ts` `decodeType` opcodes and trace what the cache bytes would produce. This is high-effort per ID.

If (a) is unavailable and (b) is too expensive for the full enumerated list, narrow the scope: focus on LocTypes that goscape has `BlockWalk=true` AND `Active=1` (the over-block candidates) at tiles between the smoke source and destination coords. That's likely <10 IDs.

- [ ] **Step 3.4: Identify smoking-gun candidates**

For Repro A path (3221, 3218) → (3218, 3216), enumerate the FlagMap-flagged tiles ON the straight line + 1-tile detour candidates between source and destination. For each FlagBlockWalk-flagged tile, name the loc(s) at that tile from the dump. If any loc's TS `blockwalk` is false but goscape's is true (or TS active==0 but goscape active==1) — that's the smoking gun for H1.

Record findings in audit notes. **No commit** — this is a read-only audit; output feeds Task 9's diagnosis report.

- [ ] **Step 3.5: Update controller working notes**

Append to your Bundle 0 notes (Step 1.8):

```
Stage 1.2 verdict: <CONFIRMED for LocTypeID X | ELIMINATED | PARTIAL | UNDETERMINED — reason>
Smoking-gun candidates: <list of LocTypeIDs with explicit goscape vs TS divergence>
```

---

## Task 4: Stage 1.3 — H2 verdict (PostDecode Active coercion)

**Hypothesis:** H2 — `LocType.Active` PostDecode coercion at `loctype.go:166-176` catches decorative GroundDecor that have right-click `Op` set, marking them Active=1 when TS would not.

**Files:** read-only audit.

- [ ] **Step 4.1: Read goscape PostDecode**

```bash
sed -n '162,176p' pkg/objtype/loctype.go
```

Record verbatim.

- [ ] **Step 4.2: Read TS LocType.postDecode for the Active analog**

```bash
sed -n '195,225p' $HOME/Code/github.com/LostCityRS/Engine-TS/src/cache/config/LocType.ts
```

The TS field is named `active` (likely lowercase). Identify the coercion rule.

- [ ] **Step 4.3: Cross-reference per-LocType from Stage 1.1 dump**

For each LocType from the Task 2 dump where goscape produced `Active=1`, trace what TS would produce. Build the table:

| LocTypeID | Name | shapes-set | op-set | goscape Active | TS active | Divergent? |
|---|---|---|---|---|---|---|

Particular focus on **decorative GroundDecor with Op != nil**: these are the canonical H2 candidates per spec §1. Examples likely include water fountains, chairs, statues — anything you can right-click but shouldn't pathblock.

- [ ] **Step 4.4: Identify smoking-gun candidates**

If any LocType in the bbox has goscape `Active=1` (writes collision via `ChangeLocCollision` GroundDecor branch) but TS `active=0`, that's H2 confirmed for that ID.

Record findings in audit notes. **No commit.**

- [ ] **Step 4.5: Update controller working notes**

```
Stage 1.3 verdict: <CONFIRMED for LocTypeID X | ELIMINATED | PARTIAL | UNDETERMINED — reason>
PostDecode divergence summary: <1-2 sentence statement of TS rule vs goscape rule>
```

---

## Task 5: Stage 1.4 — H3 verdict (LayerGroundDecor write-side diff)

**Hypothesis:** H3 — `pkg/gamemap/gamemap.go` `LayerGroundDecor` branch + `ChangeFloor` callees diverge from TS `GameMap.ts changeLocCollision` GroundDecor branch (NAI-96 fixed `LayerGround` width/length angle-swap; LayerGroundDecor is single-tile and untouched).

**Files:** read-only audit.

- [ ] **Step 5.1: Read goscape LayerGroundDecor branch**

```bash
sed -n '52,80p' pkg/gamemap/gamemap.go
sed -n '74,80p' pkg/pathfinder/routefinder/api.go
```

Record: `LayerGroundDecor` writes via `ChangeFloor(x, z, level, add)` (single-tile, no width/length, no angle, no shape).

- [ ] **Step 5.2: Read TS changeLocCollision GroundDecor branch**

```bash
sed -n '320,345p' $HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/GameMap.ts
```

(Lines may shift; search for `changeLocCollision` and read its body.) Record:
- Does TS swap width/length by angle for GroundDecor? (Goscape doesn't.)
- Does TS gate on `active` like goscape? (Goscape: `if active == 1`.)
- What flag does TS write for GroundDecor? (Goscape: `FlagBlockWalk` via `ChangeFloor`.)

- [ ] **Step 5.3: Build the divergence register**

| Aspect | TS GameMap.ts | goscape gamemap.go | Divergent? |
|---|---|---|---|
| GroundDecor active gate | ... | `active == 1` | ... |
| GroundDecor angle swap | ... | none (single-tile) | ... |
| GroundDecor flag written | ... | FlagBlockWalk | ... |
| Tile count written | ... | 1 (single-tile via ChangeFloor) | ... |

- [ ] **Step 5.4: Identify smoking-gun**

If any aspect diverges in a way that would over-block (e.g., goscape writes when TS doesn't, or goscape writes more tiles than TS), record as H3 confirmed.

**No commit.** Update controller working notes:

```
Stage 1.4 verdict: <CONFIRMED | ELIMINATED | PARTIAL | UNDETERMINED — reason>
Divergence summary: <1-2 sentences>
```

---

## Task 6: Stage 1.5 — H4 verdict (interaction.go dispatch routing)

**Hypothesis:** H4 — pathfinder API call-site routing in `modules/world/interaction.go` selects the wrong arm or wrong dimensions for the smoke target classes (NPC reach via `FindPathToEntity`).

**Files:** read-only audit.

- [ ] **Step 6.1: Read goscape interaction dispatch**

```bash
sed -n '565,680p' modules/world/interaction.go
```

Identify, for **player-clicks-NPC** (the Repro A/B target class):
- Which API arm fires (`FindPathToEntity`?)
- Which `srcSize/destWidth/destLength` values are passed
- What `n.Width()` / `n.Length()` return for NPC type 943 / type 3 (likely 1×1)

- [ ] **Step 6.2: Read TS Interaction.ts equivalent**

```bash
rg -n "getInteractionPath|findPathToEntity|findPathToLoc" $HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/Interaction.ts $HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/
```

For player-targets-NPC, identify the TS dispatch arm and its arguments.

- [ ] **Step 6.3: Build dispatch-arm comparison register**

| Target class | TS arm | goscape arm | TS args | goscape args | Divergent? |
|---|---|---|---|---|---|
| player→NPC (Repro A/B class) | ... | `FindPathToEntity` (line 642) or `FindNaivePath` (line 639) | ... | `(p.level, p.x, p.z, tx, tz, p.Width(), tw, tl)` | ... |
| player→Loc | ... | `FindPathToLoc` (line 623) | ... | ... | ... |
| player→Obj | ... | `FindPathPlain` (line 670) | ... | ... | ... |

- [ ] **Step 6.4: Verify NPC dimensions for smoke targets**

```bash
rg -n "Width\(\)|Length\(\)" pkg/entity/npc.go pkg/entity/pathing.go 2>/dev/null | head -10
```

Confirm `n.Width()` / `n.Length()` for a 1×1 NPC return 1. If type 943 / type 3 are non-1×1, the test reproducer in Task 8 must use the actual dims.

- [ ] **Step 6.5: Identify smoking-gun**

If goscape dispatches a different arm than TS for the Repro A/B class, OR passes mismatched dimensions, that's H4 confirmed.

**No commit.** Update controller working notes:

```
Stage 1.5 verdict: <CONFIRMED | ELIMINATED | PARTIAL | UNDETERMINED — reason>
Dispatch divergence summary: <1-2 sentences>
```

---

## Task 7: Stage 1.6 — H5 verdict (post-FindPath waypoint discard / tickloop)

**Hypothesis:** H5 — pathfinder returns a valid waypoint, modules/world tickloop drops it before stepping. Smoke trace: NPC 943 tick N `waypoint_idx=1, steps_taken=0` → tick N+1 `waypoint_idx=-1, target_still_set=false`.

**Files:** read-only audit.

- [ ] **Step 7.1: Locate `waypoint_idx` writers**

```bash
rg -n "waypoint_idx|WaypointIdx|waypointIdx|WaypointIndex" modules/world/ pkg/world/ pkg/entity/ 2>/dev/null | head -30
```

Record every match. Specifically: writes to `-1` (path abandonment) and writes to `0`/`1`/etc. (path-set).

- [ ] **Step 7.2: Locate `target_still_set` analog**

```bash
rg -n "target.*=.*nil|clearTarget|interaction.*=.*nil|target_still_set|clearInteraction" modules/world/ pkg/entity/ 2>/dev/null | head -30
```

Record every match where `target` / `interaction` gets cleared.

- [ ] **Step 7.3: Locate `repathed` flag handling**

```bash
rg -n "repathed|Repathed" modules/world/ pkg/entity/ 2>/dev/null | head -20
```

Record where `repathed` is set, where it's read, and what mutators it gates.

- [ ] **Step 7.4: Trace tickloop ordering**

```bash
rg -n "func.*ProcessNpcs|func.*ProcessPlayers|tickloop|TickLoop" modules/world/ 2>/dev/null | head -10
```

Identify the tick entry point. Read 50 lines around it and trace, for an NPC interaction:
1. Where does pathfinder produce a waypoint?
2. Where does the tickloop consume the waypoint into a step?
3. What state mutators run BETWEEN those two?
4. Is there any path that calls `target = nil` or `waypoint_idx = -1` after the pathfinder produces a valid path?

This is the highest-investigation-cost step. If a test seam isn't reachable from a `_test.go` file in modules/world (no isolated test helper, etc.), H5 is grep+read only.

- [ ] **Step 7.5: Identify smoking-gun**

If a state mutator in the tickloop unconditionally clears `target` or `waypoint_idx` after a path is produced (e.g., on a swallowed error, an arrival check that misfires, a re-entry that re-runs interaction-resolve), that's H5 confirmed.

If no such mutator surfaces from grep+read, document **diagnosis ceiling** — H5 needs runtime instrumentation or a test seam to confirm. Stage 2 (NAI-98) may need to add tickloop-level logging before the fix can be scoped.

**No commit.** Update controller working notes:

```
Stage 1.6 verdict: <CONFIRMED | ELIMINATED | PARTIAL | UNDETERMINED — reason | DIAGNOSIS-CEILING>
Tickloop call-graph notes: <bulleted list of file:line hops between path-produce and step-consume>
```

---

## Task 8: Reproducer tests (Repro A + Repro B)

**Files:**
- Create: `pkg/pathfinder/routefinder/nai97_repro_test.go`

- [ ] **Step 8.1: Write the Repro A test**

```go
package routefinder

import (
	"testing"
)

// TestNAI97_NPC943_PathAroundFountain is the Repro A reproducer for NAI-97.
// Smoke shape (2026-05-05 NAI-96 close-day): player at (3221, 3218), NPC type
// 943 at (3218, 3216), level=0, cheb=3. Pathing AROUND the Lumbridge fountain
// GroundDecor between source and destination should reach an adjacent tile
// (NPC reach within 1 tile).
//
// This unit test runs against an EMPTY FlagMap to isolate pathfinder behavior
// from collision-write state. If it passes here, the bug is upstream of the
// pathfinder API on a clean grid (i.e., requires the actual fountain
// FlagBlockWalk write to surface). If it fails, H4 escalates: pathfinder
// itself can't handle the geometry.
//
// Disposition (per NAI-97 plan §"Conventions"):
//   - PASS → H4 eliminated at unit level for empty-grid case; record in diagnosis.
//   - FAIL → wrap in t.Skip with `%+v Route` pinned, route to NAI-98.
func TestNAI97_NPC943_PathAroundFountain(t *testing.T) {
	pf := NewPathFinderAPI()

	const (
		level       = 0
		srcX        = 3221
		srcZ        = 3218
		dstX        = 3218
		dstZ        = 3216
		srcSize     = 1
		destWidth   = 1 // NPC type 943 dimensions; verified in Task 6 Step 6.4
		destLength  = 1
	)

	route := pf.FindPathToEntity(level, srcX, srcZ, dstX, dstZ, srcSize, destWidth, destLength)

	if !route.Success {
		t.Fatalf("Route.Success=false on empty FlagMap; cheb=3 unobstructed must succeed. Route=%+v", route)
	}
	if len(route.Waypoints) == 0 {
		t.Fatalf("Route.Waypoints empty on empty FlagMap")
	}
	last := route.Waypoints[len(route.Waypoints)-1]
	// Reach within 1 tile of dest (entity dispatch sentinel shape=-2 expects
	// occupy-adjacent, not stand-on-dest).
	dx := last.X() - dstX
	dz := last.Z() - dstZ
	if dx < -1 || dx > 1 || dz < -1 || dz > 1 {
		t.Fatalf("last waypoint = (%d, %d); want within cheb=1 of (%d, %d). Route=%+v",
			last.X(), last.Z(), dstX, dstZ, route)
	}
}
```

- [ ] **Step 8.2: Write the Repro B test**

```go
// TestNAI97_NPC3_MidRouteAbandonment is the Repro B reproducer for NAI-97.
// Smoke shape: player abandons at (3218, 3213) trying to reach NPC type 3
// at (3223, 3216), cheb=5. Same shape as Repro A but different geometry —
// runs against the same empty-FlagMap baseline.
//
// Disposition: same as Repro A.
func TestNAI97_NPC3_MidRouteAbandonment(t *testing.T) {
	pf := NewPathFinderAPI()

	const (
		level      = 0
		srcX       = 3218
		srcZ       = 3213
		dstX       = 3223
		dstZ       = 3216
		srcSize    = 1
		destWidth  = 1
		destLength = 1
	)

	route := pf.FindPathToEntity(level, srcX, srcZ, dstX, dstZ, srcSize, destWidth, destLength)

	if !route.Success {
		t.Fatalf("Route.Success=false on empty FlagMap; cheb=5 unobstructed must succeed. Route=%+v", route)
	}
	if len(route.Waypoints) == 0 {
		t.Fatalf("Route.Waypoints empty on empty FlagMap")
	}
	last := route.Waypoints[len(route.Waypoints)-1]
	dx := last.X() - dstX
	dz := last.Z() - dstZ
	if dx < -1 || dx > 1 || dz < -1 || dz > 1 {
		t.Fatalf("last waypoint = (%d, %d); want within cheb=1 of (%d, %d). Route=%+v",
			last.X(), last.Z(), dstX, dstZ, route)
	}
}
```

- [ ] **Step 8.3: Run both reproducers**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI97_NPC943_PathAroundFountain|TestNAI97_NPC3_MidRouteAbandonment' -v ./pkg/pathfinder/routefinder/
```

Record output verbatim. **Both are highly likely to PASS** because they run against an empty FlagMap — the bug requires real collision flags. The pass/fail signals what the reproducer tells us:

- **Both PASS:** H4 (pathfinder shape) is eliminated for the empty-grid case. The bug is collision-write or post-FindPath only — confirms H5 / H1 / H2 / H3 axis. Record elimination in diagnosis.
- **Either FAILS:** H4 partial — pathfinder itself can't reach this geometry without obstacles. Wrap the failing test in `t.Skip("NAI-97: …")` with the verbatim `%+v` Route pinned (per `skip_pin_full_struct_capture`).

- [ ] **Step 8.4: Disposition**

For each test that PASSED in 8.3: leave as-is.

For each test that FAILED in 8.3: edit to wrap the body in:

```go
t.Skip("NAI-97: pathfinder fails on empty-grid clean reach; observed Route at NAI-97 audit time: <PASTE EXACT %+v from 8.3 output>. Lift in NAI-98 once fix lands.")
```

…inserted as the first statement of the test body, **above** the existing assertions (which serve as the diff target for NAI-98).

- [ ] **Step 8.5: Run final state**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI97_NPC943_PathAroundFountain|TestNAI97_NPC3_MidRouteAbandonment' -v ./pkg/pathfinder/routefinder/
```

Expected: green (passing or skipping cleanly).

- [ ] **Step 8.6: Commit**

```bash
git add pkg/pathfinder/routefinder/nai97_repro_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(routefinder): NAI-97 T8 — Repro A/B clean-grid reproducers

Pins NAI-96 smoke shape: NPC 943 at (3218,3216) reach from (3221,3218),
NPC 3 at (3223,3216) reach from (3218,3213). Empty FlagMap baseline —
disposition recorded inline (PASS = H4 eliminated for clean grid; FAIL =
H4 partial, t.Skip with %+v Route pin).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Compile diagnosis report

**Files:**
- Create: `docs/superpowers/investigations/2026-05-05-nai-97-diagnosis.md`

- [ ] **Step 9.1: Verify the directory exists**

```bash
ls docs/superpowers/investigations/ 2>/dev/null && echo "dir exists" || mkdir -p docs/superpowers/investigations
```

- [ ] **Step 9.2: Write the diagnosis report**

Create `docs/superpowers/investigations/2026-05-05-nai-97-diagnosis.md`. Fill every cell from the audit findings collected in Tasks 1-8; do NOT leave any section empty — write "ELIMINATED — see §X for evidence" or "UNDETERMINED — diagnosis ceiling: …" if the audit didn't produce a verdict.

```markdown
# NAI-97 — GroundDecor Over-Blocking + Reach Abandonment Stage 1 Diagnosis

**Spec:** `docs/superpowers/specs/2026-05-05-nai-97-grounddecor-reach-investigation-design.md`
**Plan:** `docs/superpowers/plans/2026-05-05-nai-97-grounddecor-reach-investigation.md`
**Audit date:** 2026-05-05
**HEAD at audit start:** <git rev-parse --short HEAD from Bundle 0>

## Summary

[Single paragraph: which hypothesis fired (if any), root cause if identified, OR diagnosis ceiling.]

## Audit baseline (Bundle 0 controller pre-flight)

[Paste the 5-line summary from Task 1 Step 1.8 verbatim.]

## Reproducer test results

| Test | Result | Disposition |
|---|---|---|
| TestNAI97_LocWalkDump_Lumbridge | PASS (dump-only) | always passes; output captured at Stage 1.1 |
| TestNAI97_NPC943_PathAroundFountain | [PASS / FAIL+pinned] | [skipped or passing] |
| TestNAI97_NPC3_MidRouteAbandonment | [PASS / FAIL+pinned] | [skipped or passing] |

## Per-hypothesis verdicts

### H1 — TS-divergent over-blocking via LocType.BlockWalk

**Verdict:** [CONFIRMED for LocTypeID X | ELIMINATED | PARTIAL | UNDETERMINED]

**Cross-reference table (from Task 3 Step 3.3):**

| LocTypeID | Name | goscape BlockWalk | goscape Active | TS blockwalk | TS active | Divergent? |
|---|---|---|---|---|---|---|
| ... | ... | ... | ... | ... | ... | ... |

**Smoking-gun candidates (Task 3 Step 3.4):** [list of LocTypeIDs at smoke-path tiles]

**Evidence:**
- ...

### H2 — LocType.Active PostDecode coercion

**Verdict:** [CONFIRMED | ELIMINATED | PARTIAL | UNDETERMINED]

**Coercion rule comparison (from Task 4 Step 4.2):**

- TS rule: ...
- goscape rule (`pkg/objtype/loctype.go:166-176`): ...

**Per-LocType table (from Task 4 Step 4.3):**

| LocTypeID | Name | shapes-set | op-set | goscape Active | TS active | Divergent? |
|---|---|---|---|---|---|---|
| ... | ... | ... | ... | ... | ... | ... |

**Evidence:**
- ...

### H3 — LayerGroundDecor write-side divergence

**Verdict:** [CONFIRMED | ELIMINATED | PARTIAL | UNDETERMINED]

**Divergence register (from Task 5 Step 5.3):**

| Aspect | TS GameMap.ts | goscape gamemap.go | Divergent? |
|---|---|---|---|
| GroundDecor active gate | ... | `active == 1` | ... |
| GroundDecor angle swap | ... | none (single-tile) | ... |
| GroundDecor flag written | ... | FlagBlockWalk | ... |
| Tile count written | ... | 1 | ... |

**Evidence:**
- ...

### H4 — Pathfinder API call-site dispatch routing

**Verdict:** [CONFIRMED | ELIMINATED | PARTIAL | UNDETERMINED]

**Dispatch-arm register (from Task 6 Step 6.3):**

| Target class | TS arm | goscape arm | TS args | goscape args | Divergent? |
|---|---|---|---|---|---|
| player→NPC | ... | `FindPathToEntity` (interaction.go:642) | ... | ... | ... |
| player→Loc | ... | `FindPathToLoc` (interaction.go:623) | ... | ... | ... |
| player→Obj | ... | `FindPathPlain` (interaction.go:670) | ... | ... | ... |

**Empty-grid reproducer outcome (from Task 8 Step 8.3):** [both PASS / one FAILED + which]

**Evidence:**
- ...

### H5 — Post-FindPath waypoint discard / interaction-state reset

**Verdict:** [CONFIRMED | ELIMINATED | PARTIAL | UNDETERMINED | DIAGNOSIS-CEILING]

**Tickloop call-graph (from Task 7 Step 7.4):**

[Bulleted list of file:line hops between pathfinder-produce and step-consume.]

**Smoking-gun mutators identified (Task 7 Step 7.5):**

- ...

**Evidence:**
- ...

## Root cause

[Single paragraph naming the bug with file:line evidence, OR "Diagnosis ceiling: NAI-98 needs <X> to break through. Specifically: ..."]

## Stage 2 (NAI-98) handoff

- **Root cause:** [file:line + 1-2 sentence summary]
- **Repro tests to lift skip on:** [list of t.Skip-wrapped tests + expected post-fix behavior]
- **Files NAI-98 will touch:** [exact list]
- **Estimated LOC for fix:** [ballpark]
- **Residual hypotheses for NAI-99+:** [any divergences not in NAI-98 scope]
- **Smoke spec:** post-fix smoke must confirm both Repro A (player at (3221,3218) reaches adjacent to NPC 943 at (3218,3216)) and Repro B (player at (3218,3213) reaches NPC 3 at (3223,3216)) without "I can't reach that" abandonment.
```

- [ ] **Step 9.3: Verify report has no template placeholders**

```bash
rg -nE "\[CONFIRMED|\[file:line|\[exact list|\[ballpark|\[paste|\[Single paragraph|\| \.\.\. \|" docs/superpowers/investigations/2026-05-05-nai-97-diagnosis.md
```

Expected: zero matches against literal template tokens (square-bracket placeholders, `| ... |` table cells). Every `[...]` placeholder and every `| ... |` cell in the tables must be filled with real evidence. Fix inline if any surface.

- [ ] **Step 9.4: Commit**

```bash
git add docs/superpowers/investigations/2026-05-05-nai-97-diagnosis.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-97 diagnosis report

Per-hypothesis verdicts H1-H5; reproducer test results;
loc-walk + FlagMap dump baseline; root cause / diagnosis
ceiling; Stage 2 (NAI-98) handoff.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Followups update + close commit

**Files:**
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

- [ ] **Step 10.1: Append "From NAI-97" section to followups**

Read the current tail:

```bash
tail -50 $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
```

Append a new section using Edit at the end-of-file marker:

```markdown

---

## From NAI-97

**Why:** NAI-97 was a Stage 1 audit of the GroundDecor path-around residual NAI-96 surfaced in 2026-05-05 smoke. Stage 2 fix is NAI-98.
**How to apply:** When opening NAI-98, read `docs/superpowers/investigations/2026-05-05-nai-97-diagnosis.md` first; the Stage 2 handoff section there is the spec input.

- **Root cause / diagnosis ceiling (verbatim from diagnosis report §"Root cause"):** [paste]
- **Reproducer tests awaiting lift:** [paste from §"Stage 2 handoff"]
- **Files NAI-98 will touch:** [paste]
- **Residuals for NAI-99+:** [paste]
- **Stale memory to update:** `pathfinder_api_loc_aware.md` references `FindPathDefault` which was renamed to `FindPathPlain` (per Bundle 0 Step 1.4). Update during NAI-98 close or as a separate cleanup.
```

Replace each `[paste]` with the actual content from the diagnosis report.

- [ ] **Step 10.2: Verify the followups entry has no `[paste]` placeholders left**

```bash
rg -n "\[paste\]" $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
```

Expected: zero matches.

- [ ] **Step 10.3: Pre-close verification — full pathfinder + gamemap test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pathfinder/... ./pkg/gamemap/... -v 2>&1 | tail -50
```

Expected: all tests PASS or SKIP cleanly. No FAIL.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: green at the repo level.

- [ ] **Step 10.4: Verify `git status` clean of stray artifacts (per `feedback_subagent_wt_path`)**

```bash
git status
```

Expected: only intended changes (the new test files, diagnosis report; `nai_followups.md` lives outside the repo). Any unstaged dotfiles shown at session start are pre-existing — leave untouched.

- [ ] **Step 10.5: Close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-97 — GroundDecor over-blocking + reach Stage 1

Stage 1 audit complete. Hypotheses H1-H5 verdicted in diagnosis
report; reproducer tests pinned; Stage 2 (NAI-98) handoff in
nai_followups.md.

[Add 1-2 line summary of root cause / diagnosis ceiling here.]

Closes memory: nai_96_grounddecor_path_around_residual.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Replace the bracketed summary line with the actual root cause / ceiling. Keep the `Closes memory:` trailer per `close_commit_memory_trailer` — NAI-97 closes the residual entry that motivated it.

- [ ] **Step 10.6: Verify clean close**

```bash
git log -1 --stat
git status
```

Expected: `git status` clean; latest commit message contains the summary; the close commit's `--stat` is empty (the close commit is `--allow-empty` because all content commits already shipped under Tasks 2/8/9).

---

## Self-Review Checklist (controller, before task dispatch)

- [ ] **Spec coverage:**
  - §1 Context & motivation → Task 1 (Bundle 0 verifies premises) + §"Goal"
  - §2 In/out of scope → Task 0 conventions ("no production change") + Task 8 reproducer disposition + Task 7 H5 ceiling clause
  - §3 Hypothesis register H1-H5 → Tasks 3, 4, 5, 6, 7 (one per hypothesis)
  - §4 Probe / reproducer matrix → Tasks 2 (loc-walk dump), 8 (Repro A/B). H5 reproducer optional clause honored at Task 7 Step 7.4.
  - §5 Methodology hybrid probe-then-diff → Task ordering 1→2→3→4→5→6→7→8→9; short-circuit via "controller updates working notes" pattern after each substage
  - §6 Deliverables → Tasks 2, 8, 9, 10
  - §7 Exit criteria → Task 10 Step 10.3 + Task 9 §"Stage 2 handoff"
  - §8 Risks → Task 1 fixture-availability handling (Step 1.7) + Task 7 H5 diagnosis-ceiling clause + Task 3 narrowing-of-scope clause + audit-fabrication-guard in conventions
  - §9 Cadence references → Task 10 close-commit `Closes memory:` trailer + conventions §subagent-fabrication-guard
  - §10 Stage 2 handoff template → Task 9 Step 9.2 inline template
- [ ] **No placeholders:** every step has actual command / code. Template `[paste]` and `[...]` markers in Tasks 9 and 10 are intentional fill-in-from-audit-output sites, gated by explicit "expected: zero matches" verification (9.3, 10.2).
- [ ] **Type consistency:**
  - Test names referenced consistently across Tasks 2/8/9/10
  - `Route.Success`, `Route.Waypoints`, `RouteCoordinates.X()/.Z()` match `route.go:3-7` + `routecoordinates.go:9-19` verified at plan-author time
  - `gm.Pathfinder.Flags.Get(x, z, level)` matches `flagmap.go:30` + `api.go:16` (verified)
  - `gm.StaticLocs()` returns `[]*entity.Loc` with `.X`, `.Z`, `.Level`, `.Width`, `.Length` direct fields + `.Type()`/`.Shape()`/`.Angle()`/`.Layer()` methods (verified `entity/loc.go:12-59`, `entity/nonpathing.go` for `.X`/`.Z`/`.Level`/`.Width`/`.Length`)
  - `objtype.LoadLocTypes(dir)` returns `*LocTypeConfigs{Configs []*LocType}` (verified `loctype.go:204`)
  - `lt.BlockWalk` / `lt.Active` / `lt.DebugName` / `lt.BlockRange` are direct `*LocType` fields (verified `loctype.go:29-31`)
- [ ] **Conditional task gating:** Task 7 explicitly notes H5 may close as DIAGNOSIS-CEILING if no test seam reachable; Task 8 disposition branches on PASS/FAIL.
- [ ] **Audit-fabrication guard wired:** Tasks 3, 4, 5, 6, 7 are read-only / no-commit; their outputs feed Task 9, where the controller verifies every file:line citation independently before merging into the diagnosis report.
- [ ] **Bundle 0 pre-flight wired:** Task 1 verifies all spec §1 premises (signature, line numbers, gate logic, fixture availability) before any audit dispatch.
