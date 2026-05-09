# NAI-142 — `rebuildZones` cached-client coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Retire the misnamed `rebuildScenery` `activeZones` dual-write (R-D2) and port TS `NetworkPlayer.ts:269-271` per-zone-change `rebuildZones()` call site (R-D3) as one TS-fidelity bundle.

**Architecture:** Single-task TDD bundle, ~50–75 LOC. Add `Player.lastZone int` field (sentinel `-1`), introduce `(*Player).updateBuildArea()` method mirroring TS `NetworkPlayer.updateMap`'s lastZone slice, insert it at the top of `processOut` (mirroring TS `processClientsOut → updateMap` ordering at `World.ts:1097`), then drop the hardcoded level-0 `activeZones` write at `player.go:667`. Cleanup-only doc-comment edits at `player.go:691, 694-700, 856-858`.

**Tech Stack:** Go 1.26+ (`go_version.md`); `github.com/zsrv/goscape/pkg/coordgrid` for `PackCoord`.

**Spec:** `docs/superpowers/specs/2026-05-09-nai-142-rebuildzones-cached-client-coverage-design.md`

**TS source:**
- `LostCityRS/Engine-TS/src/engine/entity/NetworkPlayer.ts:269-271` (per-zone-change call site)
- `LostCityRS/Engine-TS/src/engine/entity/Player.ts:380` (`lastZone: number = -1`)
- `LostCityRS/Engine-TS/src/engine/World.ts:1097` (TS processClientsOut → updateMap call slot)

---

## File Structure

**Modify:**
- `modules/world/player.go` — add `lastZone` field + init, add `updateBuildArea` method, edit `processOut`, delete line-667 dual-write, update three doc-comment blocks.
- `modules/world/player_zone_test.go` — add tests T1–T6 below.

**No new files.** Test additions fit into the existing `player_zone_test.go`. The plan below produces five small commits (one per logical step + one for tests + one for the close-trailer); the implementer subagent may collapse to fewer commits if all checks stay green.

---

## Task 1: NAI-142 Bundle (R-D2 + R-D3)

**Files:**
- Modify: `modules/world/player.go:316` (struct field), `:518-521` (init), `:649, :667` (R-D2 surgery), `:691, :694-700` (rebuildZones doc), `:855-867` (processOut + updateBuildArea)
- Test: `modules/world/player_zone_test.go` (append tests T1–T6)

### Step 1 — Read the current rebuildScenery + processOut + Player struct

- [ ] **Step 1: Read these three regions verbatim before editing**

Read:
- `modules/world/player.go:300-322` (Player struct around `activeZones`)
- `modules/world/player.go:496-536` (newPlayer constructor literal)
- `modules/world/player.go:643-722` (rebuildScenery + rebuildZones, including doc-comments)
- `modules/world/player.go:855-867` (processOut)

Why: every Edit in this task targets one of these regions. Reading first lets you reproduce the exact whitespace + surrounding lines for unique-string Edit matches.

### Step 2 — Write the failing tests (T1–T6)

- [ ] **Step 2: Append the test block below to `modules/world/player_zone_test.go`**

The file currently imports:

```go
import (
    "net"
    "testing"

    entitypkg "github.com/zsrv/goscape/pkg/entity"
    io2 "github.com/zsrv/goscape/pkg/io/isaac"
    "github.com/zsrv/goscape/pkg/zone"
)
```

Add `"github.com/zsrv/goscape/pkg/coordgrid"` to the import group (alphabetical placement: between `entitypkg` and `io2`).

Append the following tests verbatim at the end of the file:

```go
// === NAI-142: per-zone-change rebuildZones (R-D3) + rebuildScenery
// activeZones dual-write retirement (R-D2) ===

// T1: first-tick fires (lastZone == -1 sentinel → mismatch on first call).
func TestUpdateBuildArea_FirstTickFires(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 50<<3, 50<<3, 0
	p.originX, p.originZ = p.x, p.z

	if p.lastZone != -1 {
		t.Fatalf("newPlayer should init lastZone=-1; got %d", p.lastZone)
	}

	p.updateBuildArea()

	wantZone := coordgrid.PackCoord(0, 50<<3, 50<<3)
	if p.lastZone != wantZone {
		t.Errorf("lastZone after first updateBuildArea: got %d, want %d", p.lastZone, wantZone)
	}
	if len(p.activeZones) == 0 {
		t.Error("activeZones must be populated by rebuildZones on first updateBuildArea")
	}
}

// T2: same-zone no-fire (lastZone equals currentZone → no-op).
func TestUpdateBuildArea_SameZoneNoFire(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 50<<3, 50<<3, 0
	p.originX, p.originZ = p.x, p.z
	p.updateBuildArea() // first-tick fire establishes lastZone

	// Replace activeZones with a sentinel single-entry map. If
	// updateBuildArea fires again, it will call rebuildZones which clears
	// activeZones and refills with 49 entries — the sentinel disappears.
	sentinelKey := -999999
	p.activeZones = map[int]bool{sentinelKey: true}

	p.updateBuildArea()

	if !p.activeZones[sentinelKey] {
		t.Error("same-zone updateBuildArea must NOT fire rebuildZones; sentinel was cleared")
	}
	if len(p.activeZones) != 1 {
		t.Errorf("same-zone updateBuildArea: activeZones len got %d, want 1", len(p.activeZones))
	}
}

// T3: cross-zone (x boundary) fires.
func TestUpdateBuildArea_CrossZoneFires(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 50<<3, 50<<3, 0
	p.originX, p.originZ = p.x, p.z
	p.updateBuildArea()

	// Cross x-zone boundary: 50<<3 (zone x=50) → 51<<3 (zone x=51).
	p.x = 51 << 3
	p.updateBuildArea()

	wantZone := coordgrid.PackCoord(0, 51<<3, 50<<3)
	if p.lastZone != wantZone {
		t.Errorf("cross-zone lastZone: got %d, want %d", p.lastZone, wantZone)
	}
	// New center (51, 50) must be in activeZones.
	if !p.activeZones[coordgrid.ZoneIndex(51<<3, 50<<3, 0)] {
		t.Error("cross-zone activeZones missing new center (51, 50)")
	}
}

// T4: cross-level fires (e.g., trapdoor descent).
func TestUpdateBuildArea_CrossLevelFires(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 50<<3, 50<<3, 0
	p.originX, p.originZ = p.x, p.z
	p.updateBuildArea()

	// Same x/z but different level — TS encodes level in the packed coord.
	p.level = 1
	p.updateBuildArea()

	wantZone := coordgrid.PackCoord(1, 50<<3, 50<<3)
	if p.lastZone != wantZone {
		t.Errorf("cross-level lastZone: got %d, want %d", p.lastZone, wantZone)
	}
	// activeZones now keyed at level=1; level-0 center key must be ABSENT.
	if p.activeZones[coordgrid.ZoneIndex(50<<3, 50<<3, 0)] {
		t.Error("cross-level activeZones must not contain level-0 center key")
	}
	if !p.activeZones[coordgrid.ZoneIndex(50<<3, 50<<3, 1)] {
		t.Error("cross-level activeZones must contain level-1 center key")
	}
}

// T5: rebuildScenery no longer pre-populates activeZones (R-D2 retirement).
// Pins the dual-write removal: post-rebuildScenery, activeZones is empty
// (line-649 reset still runs; line-667 write is gone).
func TestRebuildScenery_DoesNotPrePopulateActiveZones(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 50<<3, 50<<3, 0
	p.originX, p.originZ = p.x, p.z
	// Pre-set sentinel; rebuildScenery's line-649 reset clears it.
	p.activeZones = map[int]bool{-1: true}

	_ = p.rebuildScenery(0)

	if len(p.activeZones) != 0 {
		t.Errorf("rebuildScenery must not populate activeZones (R-D2); got len=%d", len(p.activeZones))
	}
}

// T6: bundle-value test — cached-client cross-zone delivers fresh activeZones.
// Pre-bundle: rebuildScenery's 13×13 superset masked the staleness; the
// post-rebuildScenery activeZones at center (50,50) was a 169-entry set.
// Post-bundle: updateBuildArea fires rebuildZones each cross, producing the
// canonical 7×7 (49-entry) set centered on the new zone.
func TestUpdateBuildArea_CachedClientCrossZoneFreshActiveZones(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 50<<3, 50<<3, 0
	p.originX, p.originZ = p.x, p.z
	p.updateBuildArea()
	if len(p.activeZones) != 49 {
		t.Fatalf("first-tick 7×7 activeZones: got %d, want 49", len(p.activeZones))
	}
	if !p.activeZones[coordgrid.ZoneIndex(50<<3, 50<<3, 0)] {
		t.Fatal("first-tick activeZones missing center (50,50)")
	}

	// In-window cross-zone: zone window stays anchored to origin (50,50);
	// shouldRebuild() returns false; processOut.updateBuildArea fires.
	p.x = 52 << 3
	p.updateBuildArea()

	if len(p.activeZones) != 49 {
		t.Fatalf("post-cross-zone 7×7 activeZones: got %d, want 49", len(p.activeZones))
	}
	if !p.activeZones[coordgrid.ZoneIndex(52<<3, 50<<3, 0)] {
		t.Error("post-cross-zone activeZones missing new center (52,50)")
	}
	// Old non-overlapping cell must be GONE: center moved from 50→52, so
	// the old 7×7 [47..53] dropped (46,46), (46,47), (47,46) etc. The new
	// 7×7 [49..55] is centered at 52. Cell (47,50) was in the old set, not
	// in the new.
	if p.activeZones[coordgrid.ZoneIndex(47<<3, 50<<3, 0)] {
		t.Error("post-cross-zone activeZones still contains stale cell (47,50)")
	}
}
```

- [ ] **Step 3: Run the new tests; they MUST fail**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUpdateBuildArea_|TestRebuildScenery_DoesNotPrePopulateActiveZones' -v
```

Expected: ALL six tests FAIL.

- T1, T2, T3, T4, T6: compile error — `p.lastZone` undefined; `p.updateBuildArea` undefined.
- T5: passes-as-test-target FAILS — `len(p.activeZones)` is 169 (the 13×13 dual-write), not 0.

Per `verify_implementer_claims.md`: capture the exact `go test` output to prove the failures are the ones expected, not unrelated breakage. If T5 passes (suggesting the dual-write is already gone) STOP — re-read HEAD; spec premise is wrong.

### Step 3 — Add `lastZone` field to Player struct

- [ ] **Step 4: Edit `modules/world/player.go` — add `lastZone` field below `rebuiltOnce`**

Locate this region (lines 314-318 area):

```go
	lastBuild   int
	loadedZones map[int]bool
	activeZones map[int]bool
	mapsquares  map[uint16]bool
	rebuiltOnce bool
```

Add a new field block immediately after `rebuiltOnce`:

```go
	lastBuild   int
	loadedZones map[int]bool
	activeZones map[int]bool
	mapsquares  map[uint16]bool
	rebuiltOnce bool

	// lastZone is the previously-witnessed packed zone coord
	// (level, zoneX<<3, zoneZ<<3) used by updateBuildArea to detect
	// per-tick zone transitions. Sentinel -1 forces the first
	// updateBuildArea call to fire rebuildZones (matching TS
	// Player.ts:380 `lastZone: number = -1`). Encoded via
	// coordgrid.PackCoord — NOT coordgrid.ZoneIndex — so future ports
	// of triggerZone/triggerZoneExit (NAI-142-D-R-D3) can decode via
	// CoordGrid.unpackCoord parity (NetworkPlayer.ts:281).
	lastZone int
```

### Step 4 — Initialize `lastZone: -1` in newPlayer

- [ ] **Step 5: Edit `modules/world/player.go` — add init in newPlayer literal**

Locate this region (lines 518-521 area):

```go
		lastBuild:      0,
		loadedZones:    map[int]bool{},
		activeZones:    map[int]bool{},
		mapsquares:     map[uint16]bool{},
	}
```

Edit to:

```go
		lastBuild:      0,
		loadedZones:    map[int]bool{},
		activeZones:    map[int]bool{},
		mapsquares:     map[uint16]bool{},
		lastZone:       -1, // NAI-142: sentinel; first updateBuildArea fires rebuildZones
	}
```

### Step 5 — Drop the R-D2 dual-write at line 667

- [ ] **Step 6: Edit `modules/world/player.go` — delete the activeZones write inside rebuildScenery's loop**

Locate this region (lines 665-668 area, inside the `for dx := -6; dx <= 6; dx++` nested loop):

```go
			p.mapsquares[uint16((mapX<<8)|mapZ)] = true
			p.activeZones[coordgrid.ZoneIndex(zx<<3, zz<<3, 0)] = true
		}
	}
```

Edit to (delete the `p.activeZones[...]` line; keep the line above and the closing braces):

```go
			p.mapsquares[uint16((mapX<<8)|mapZ)] = true
		}
	}
```

The line-649 reset (`p.activeZones = map[int]bool{}`) stays — it's defensive state-clearing per spec §2.2.

### Step 6 — Update rebuildZones doc-comment (R-D2 cleanup + R-D3 reference)

- [ ] **Step 7: Edit `modules/world/player.go` — replace the doc-comment block above `func (p *Player) rebuildZones()`**

Locate this exact block (lines ~684-700):

```go
// rebuildZones refreshes activeZones to a 7×7-zone window centered on
// the player's current zone, intersected with the 13×13-zone
// build-area window centered on origin. Mirrors TS
// BuildArea.rebuildZones (BuildArea.ts:31-55).
//
// Called at the end of handleRebuildGetMaps (after the client confirms
// maps loaded). Not called per-zone-change because goscape has not yet
// ported NetworkPlayer.ts:269-271 lastZone-transition tracking;
// deferred follow-up in nai84_rebuildzones_per_zone_change.md.
//
// Note: rebuildScenery (player.go:600-635) currently also writes
// activeZones (with a 13×13 set keyed at level=0). That pre-existing
// divergence is intentionally not touched here — see TS-fidelity
// ledger entry §6 R-D2. rebuildZones runs after rebuildScenery in the
// REBUILD path (rebuildScenery → sendRebuildNormal → client requests
// maps → handleRebuildGetMaps → rebuildZones), so the rebuildScenery
// preset is overwritten before zone deltas flow.
```

Replace with:

```go
// rebuildZones refreshes activeZones to a 7×7-zone window centered on
// the player's current zone, intersected with the 13×13-zone
// build-area window centered on origin. Mirrors TS
// BuildArea.rebuildZones (BuildArea.ts:31-55).
//
// Called from two sites:
//   1. handleRebuildGetMaps (data_map.go:153) — after client confirms
//      maps loaded post-REBUILD_NORMAL.
//   2. updateBuildArea (player.go, top of processOut) — per-tick zone
//      transition (NAI-142, mirroring TS NetworkPlayer.ts:269-271).
//
// Both sites fire on the same tick on a REBUILD path; rebuildZones
// resets activeZones at its top, so the duplication is idempotent.
// Matches TS ordering (TS World.ts:1097 → NetworkPlayer.updateMap also
// calls rebuildZones unconditionally on lastZone change).
```

### Step 7 — Add `updateBuildArea` method

- [ ] **Step 8: Edit `modules/world/player.go` — insert `updateBuildArea` immediately above `processOut`**

Locate this region (lines ~853-867):

```go
}

func (p *Player) processOut() {
	// NAI-93: updateMap moved to Server.processInfo per TS World.ts:996
	// ordering. processOut now starts with PlayerInfo encode against the
	// already-fresh rsbuf state.
	p.updatePlayers()
	p.updateNpcs()
	p.updateZones()
	p.updateInvs()
	p.updateStats()
	p.updateAfkZones()
	p.encodeOut()
	p.client.flushWrite()
}
```

Edit to (insert `updateBuildArea` method above; insert call as first line of processOut; update the comment):

```go
}

// updateBuildArea fires rebuildZones() on per-tick zone transitions.
// Mirrors the lastZone slice of TS NetworkPlayer.updateMap
// (NetworkPlayer.ts:269-271):
//
//	const zone = CoordGrid.packCoord(this.level, (this.x >> 3) << 3, (this.z >> 3) << 3);
//	if (this.lastZone !== zone) {
//	    this.buildArea.rebuildZones();
//	    // ... triggerZone/triggerZoneExit/SetMultiway (NAI-142-D-R-D3)
//	    this.lastZone = zone;
//	}
//
// Camera packets (NetworkPlayer.ts:243-253), lastMapZone +
// triggerMapzone (NetworkPlayer.ts:256-266), and triggerZone +
// triggerZoneExit + SetMultiway (NetworkPlayer.ts:274-285) are
// deferred follow-ups; see nai_followups.md
// NAI-142-D-R-D{1,2,3}.
func (p *Player) updateBuildArea() {
	zone := coordgrid.PackCoord(p.level, (p.x>>3)<<3, (p.z>>3)<<3)
	if p.lastZone != zone {
		p.rebuildZones()
		p.lastZone = zone
	}
}

func (p *Player) processOut() {
	// NAI-93: goscape's updateMap (=TS BuildArea.rebuildNormal slot) was
	// moved to Server.processInfo per TS World.ts:996 ordering, so
	// processOut starts with PlayerInfo encode against already-fresh
	// rsbuf state.
	//
	// NAI-142: updateBuildArea is the TS NetworkPlayer.updateMap slot
	// (TS World.ts:1097) — it must run before updateZones so any
	// per-tick zone transition is reflected in activeZones before zone
	// deltas flow.
	p.updateBuildArea()
	p.updatePlayers()
	p.updateNpcs()
	p.updateZones()
	p.updateInvs()
	p.updateStats()
	p.updateAfkZones()
	p.encodeOut()
	p.client.flushWrite()
}
```

### Step 8 — Run all six new tests; they MUST pass

- [ ] **Step 9: Run targeted tests**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUpdateBuildArea_|TestRebuildScenery_DoesNotPrePopulateActiveZones' -v
```

Expected: ALL six tests PASS.

If any FAIL: do not "fix forward" with extra logic. Re-read the spec §2.4–§2.5 and the failing assertion; the most likely bug is a typo in `coordgrid.PackCoord` argument order (TS: `packCoord(level, x, z)` → goscape: `PackCoord(level, x, z)` — see `pkg/coordgrid/coordgrid.go:158`).

### Step 9 — Run the full world package test suite; no regressions

- [ ] **Step 10: Run full package**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: PASS (no failures).

If any pre-existing test fails, capture the failure verbatim. Pre-existing failures at HEAD~1 should be diff'd against `git stash && go test ./modules/world/` to verify they're not caused by this bundle. Per `verify_implementer_claims.md` — never attribute a failure to "pre-existing" without that diff.

### Step 10 — Run full repo test suite + vet

- [ ] **Step 11: Run full module + vet**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: PASS.

### Step 11 — Commit

- [ ] **Step 12: Commit the bundle**

Stage exactly the two files; no `git add -A`:

```bash
git add modules/world/player.go modules/world/player_zone_test.go
```

Confirm staged diff:

```bash
git diff --cached --stat
```

Expected:

```
 modules/world/player.go            | ~30 +-
 modules/world/player_zone_test.go  | ~140 +
 2 files changed, ~165 insertions(+), ~5 deletions(-)
```

Commit:

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-142 — retire rebuildScenery activeZones dual-write + port per-zone-change rebuildZones

R-D2: drop the hardcoded level-0 13×13 activeZones write inside
rebuildScenery's loop (player.go formerly :667). TS BuildArea.Rebuild
does not touch activeZones; goscape's preset was a benign-broader
superset that masked the missing per-zone-change call site.

R-D3: port TS NetworkPlayer.ts:269-271 by adding `Player.lastZone`
(sentinel -1) and a new `updateBuildArea` method called at the top of
processOut, mirroring TS World.ts:1097 → NetworkPlayer.updateMap. On
zone transition (cross-x, cross-z, or cross-level), rebuildZones runs
before updateZones streams deltas, so cached-client tele/walk paths
now use the canonical 7×7 level-aware activeZones set instead of the
stale 13×13 level-0 preset.

Bundle is mandatory: R-D2 alone breaks cached-client delta delivery;
R-D3 alone is redundant until R-D2 retires.

Tests: T1 first-tick fires; T2 same-zone no-fire; T3 cross-zone fires;
T4 cross-level fires; T5 rebuildScenery no longer pre-populates;
T6 cached-client cross-zone produces fresh 7×7.

Closes memory: nai_followups.md NAI-84-D-R-D2 NAI-84-D-R-D3
EOF
)"
```

Verify commit:

```bash
git log -1 --stat
```

Expected: 2 files changed, body matches the message above.

---

## Task 2: Smoke handoff + memory update

### Step 12 — Hand off the smoke

- [ ] **Step 13: Print the smoke instructions for the user**

Per `smoke_test_server_handoff.md`: the implementer subagent CANNOT run the Java client from the sandbox. Print the following instructions for the user verbatim and STOP:

```
NAI-142 ready for smoke at HEAD <commit-sha>.

Boot: `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`
Then connect with the Java client and run:

  1. Login at any populated tutorial/varrock area.
  2. Walk continuously through ≥3 zone boundaries.
     PIN: scenery (doors, npcs, ground objs) renders without
          duplication, dropout, or stale-state in destination zones.
  3. Tele across the 13×13 build-area window (e.g. ::tele to a far
     coord).
     PIN: map loads, no client-side AIOOBE, locs/objs render correctly.
  4. NAI-141 regression-fence: open Lumbridge kitchen door, descend
     trapdoor, ascend.
     PIN: door renders in exactly one state (NAI-141 §1 criterion 1).

Report observed shape verbatim.
```

Wait for the user to run the smoke.

### Step 13 — On smoke green: update follow-up memory

- [ ] **Step 14: Edit `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`**

Two edits, in this order:

**Edit A — Retire NAI-84-D-R-D2 and NAI-84-D-R-D3 (lines 89-140 area).**

Replace the H3 sections `### NAI-84-D-R-D2 — ...` (line 89) through the closing of `### NAI-84-D-R-D3 — ...` (~line 140) with a single retirement note:

```markdown
### NAI-84-D-R-D2 / NAI-84-D-R-D3 — RETIRED (NAI-142, 2026-05-09)

Both follow-ups closed by NAI-142 bundle. R-D2 (rebuildScenery
activeZones dual-write) retired by deleting the line-667 write;
R-D3 (per-zone-change rebuildZones call site) ported by adding
`Player.lastZone` + `updateBuildArea` method called at top of
processOut.

See close commit (search `git log` for `NAI-142`) and spec
`docs/superpowers/specs/2026-05-09-nai-142-rebuildzones-cached-client-coverage-design.md`.
```

**Edit B — Add four new follow-up entries.** Append the following H3 sections to the appropriate place in the file (most recent at top of "open" section):

```markdown
### NAI-142-D-R-D1 — Camera packet processing in updateMap

TS `NetworkPlayer.ts:243-253` processes accumulated `cameraPackets`
inside `updateMap`, emitting CamMoveTo (type 0) or CamLookAt (type 1)
zone-relative against `(originX, originZ)`. Goscape has no
`cameraPackets` accumulator and no CamMoveTo/CamLookAt opcode
implementation; the entire camera surface is unported.

**Future port:** add `cameraPackets` linked-list (or slice) on Player,
script handlers `cam_moveto`/`cam_lookat`/`cam_reset` populate it,
updateBuildArea (or a new sibling) drains it at the top of processOut.
Standalone sub-spec.

**How to apply:** When auditing or porting any camera-related script
opcode (cam_moveto, cam_lookat, cam_reset, cam_shake, cam_smoothreset),
schedule this sub-spec first or alongside.

### NAI-142-D-R-D2 — lastMapZone tracking + triggerMapzone

TS `NetworkPlayer.ts:256-266` tracks `lastMapZone` (64-tile-grid
packed coord) and fires `triggerMapzoneExit(x,z)` + `triggerMapzone(x,z)`
script events on transition. Goscape has no mapzone trigger surface and
no `lastMapZone` field.

**Future port:** add `Player.lastMapZone int = -1`, fire `[mapzone,...]`
and `[mapzone_exit,...]` script triggers on transition. Requires
content-side `[mapzone,...]` trigger declarations (LostCityRS/Content
script grammar) — verify those exist before scheduling. Standalone
sub-spec.

**How to apply:** Co-schedule with NAI-142-D-R-D3 (triggerZone) — same
script-trigger infrastructure.

### NAI-142-D-R-D3 — triggerZone / triggerZoneExit + SetMultiway

TS `NetworkPlayer.ts:274-285` fires `triggerZone(level,x,z)` +
`triggerZoneExit(level,x,z)` script events on lastZone transition,
AND emits a `SetMultiway` packet when the destination zone's multi-combat
flag differs from the source. Goscape:
- `Player.lastZone` (added by NAI-142) carries the source coord; consumer
  must `coordgrid.UnpackCoord(p.lastZone)` to extract `(level,x,z)`.
- No `World.gameMap.isMulti(zoneCoord)` API — needs porting from TS
  `World.ts` gamemap.
- No `SetMultiway` outbound packet encoder.
- No `[zone,...]` / `[zone_exit,...]` script trigger surface.

**Future port:** large surface — likely a multi-task sub-spec (multi
flag map + SetMultiway encoder + zone trigger registry + dispatch in
updateBuildArea).

**How to apply:** After porting, the `if (p.lastZone !== -1)` guard at
TS `NetworkPlayer.ts:280` MUST be preserved (skip exit-trigger on
first tick).

### NAI-142-D-R-D4 — Rename goscape's `updateMap` → `rebuildNormal`

Goscape's `(*Player).updateMap` (player.go:724) is misnamed — it ports
TS `BuildArea.rebuildNormal`, not TS `NetworkPlayer.updateMap`. The
real TS `NetworkPlayer.updateMap` slot is now `(*Player).updateBuildArea`
(NAI-142). Cosmetic rename improves grep-discoverability and frees the
`updateMap` name for a future full port (camera + lastMapZone + lastZone
+ triggerZone consolidated into one method matching TS).

**Future cleanup:** rename `updateMap` → `rebuildNormal` in player.go;
update all callers + comments. Defer until R-D1/R-D2/R-D3 cumulatively
justify it (a one-shot rename in isolation is churn).

**How to apply:** Schedule alongside or after NAI-142-D-R-D{1,2,3}.
```

**Edit C — Update cross-reference at line 6577.**

Locate this line in the file (search for `NAI-84-D-R-D2 / NAI-84-D-R-D3 (open)`):

```markdown
- **NAI-84-D-R-D2 / NAI-84-D-R-D3 (open):** orthogonal but adjacent — they govern *when* `writeFullFollows` fires (rebuildScenery activeZones dual-write; per-zone-change rebuildZones call site). NAI-141 governs *what* it emits. No interaction expected; if a future smoke shows door-state correct after trapdoor round-trip but stale on plain-walking zone transitions, escalate as separate sub-spec on NAI-84-D-R-D3.
```

Edit to:

```markdown
- **NAI-84-D-R-D2 / NAI-84-D-R-D3 (closed in NAI-142, 2026-05-09):** retired the rebuildScenery activeZones dual-write (R-D2) and ported per-zone-change rebuildZones via Player.lastZone + updateBuildArea (R-D3). NAI-141 governs *what* writeFullFollows emits; NAI-142 governs *when* activeZones (its driver) refreshes. No regression observed in NAI-141 smoke at NAI-142 close.
```

### Step 14 — Update MEMORY.md index (no-op if no new memory entries)

- [ ] **Step 15: Verify no MEMORY.md update needed**

NAI-142 introduces no new top-level memory entries (the four new follow-ups live inside `nai_followups.md` H3 sections, which is already indexed). Skip this step unless a new top-level memory file was created.

If audit-mode or implementer flags a new lesson worth a memory file (e.g. unexpected R-D3 first-tick gotcha that should be cross-referenced from MEMORY.md), append a one-line entry per that protocol.

### Step 15 — Final commit + close

- [ ] **Step 16: Commit memory updates**

```bash
git add ~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-142 — retire R-D2 + R-D3; bundle smoke green

Memory: nai_followups.md
- retire NAI-84-D-R-D2 and NAI-84-D-R-D3 entries
- add NAI-142-D-R-D{1,2,3,4} follow-ups for camera /
  lastMapZone+triggerMapzone / triggerZone+SetMultiway / updateMap rename
- update NAI-141 cross-reference at line 6577 to mark closed
EOF
)"
```

Note: the memory file lives outside the repo (`~/.claude/projects/...`), so the `git add` above will be silently ignored in a normal `git commit` of the repo. Memory file edits are NOT committed to the repo — they live in user-global memory. The "memory update" step is the in-place file edit; no git commit is required for the memory file itself.

If the project does keep a session ledger inside the repo (e.g., `docs/superpowers/handoffs/...`), produce a one-page close handoff at `docs/superpowers/handoffs/2026-05-09-nai-142-smoke.md` and commit IT (not the memory file). Skip if no handoff convention is established for this sub-spec.

---

## Self-Review Checklist (post-write)

**Spec coverage** (each spec section maps to a task or step):
- §1.1 PRIMARY smoke (3 criteria) → Step 13 (smoke handoff verbatim)
- §1.2 SECONDARY unit pins (T1–T6) → Steps 2 (write), 9 (run+pass)
- §2.1 Current state → Step 1 (read regions before edit)
- §2.2 R-D2 changes (delete line 667; keep line 649; doc edits at 691, 694-700) → Steps 6 (line 667), 7 (doc 691/694-700)
- §2.2 R-D3 changes (lastZone field; init -1; updateBuildArea; processOut insertion; doc 856-858) → Steps 4 (field), 5 (init), 8 (method + processOut + doc)
- §2.3 Bundle rationale → covered by Task 1 atomic commit (Step 12)
- §2.4–§2.5 call-order verification → enforced by §2.4-citing risk #1, smoke step 13 #2 (REBUILD path)
- §2.6 deviations → Step 14 Edit B (4 follow-up entries)
- §3 test plan T1–T6 → Step 2
- §4 sequencing (single commit) → Step 12
- §5 risk register → enforced by smoke step 13 + memory updates step 14
- §6 memory retire/update → Step 14 (Edit A, B, C)
- §7 close-commit memory trailer → embedded in Step 12 commit body (`Closes memory: nai_followups.md NAI-84-D-R-D2 NAI-84-D-R-D3`)

**Placeholder scan:** No TBDs / "implement later" / "similar to" / "appropriate error handling". Every code block is verbatim-runnable.

**Type consistency:** `lastZone int` (Step 4) ↔ init `-1` (Step 5) ↔ `coordgrid.PackCoord(...) int` return (Step 8 method body) ↔ test assertions `coordgrid.PackCoord(...)` (Step 2). All match.

**Method-name consistency:** `updateBuildArea` used in Step 2 (test calls), Step 8 (definition + processOut insertion), Step 14 (memory entries). No drift.

**Edit-target precision:** Each Edit step quotes the exact pre-image with surrounding context (line numbers approximate; unique-string match precise).
