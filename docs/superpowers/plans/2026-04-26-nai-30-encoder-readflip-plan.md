# NAI-30 Encoder Loops Port + Read-Flip — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate `Encode`/`EncodeNpc` from interface-based reads against `pkg/grid` + `pkg/buildarea` + `PlayerSource`/`NpcSource` parameters to method-based reads against the NAI-29-landed `*Buf` slot tables, by introducing `*Buf`-attached `PlayerInfo`/`NpcInfo` structs that mirror upstream `info.rs:13-708`. Reconciles two NAI-29 parallel-write divergences (`lastAppearance` content-hash → tick-when-changed; `orientationX/Z int32(0)` → real fields with `-1` sentinel). After the read-flip, retires `pkg/grid/`, `pkg/buildarea/` (scenery fields flatten onto `Player`), `pkg/rsbuf/npc_observers.go`, and `Player.AppearanceHash`. Introduces 1 new deviation tag (NAI-30-D1: orientation field plumbed without producer); deviation count 13 → 14.

**Architecture:** Four bundles, single-implementer serial execution. **Bundle 1** (5 tasks) lands rsbuf-internal spatial-discovery primitives on `BuildArea` (port of `get_nearby_players_zones`, `get_nearby_npcs`, `filter_player`, `filter_npc`) plus entity field plumbing on `modules/world/Player` + `Npc` (`OrientationX/Z`, `lastAppearance`); pure additions, no caller changes. **Bundle 2** (9 tasks) rewrites `pkg/rsbuf/playerinfo.go` as `PlayerInfo` struct + method, mirroring `info.rs:13-409`; old `Encode` function renamed to `EncodeLegacy`, new method coexists. **Bundle 3** (7 tasks) does the same for `NpcInfo` mirroring `info.rs:411-708`. **Bundle 4** (7 tasks) wires `*Buf.PlayerInfo` + `*Buf.NpcInfo` fields, swaps modules/world callers, migrates the `npc_observers.go` consumer at `npc_hunt.go`, flattens `pkg/buildarea` scenery fields onto `Player`, then verify-grep-and-deletes the dead packages and helpers.

**Tech Stack:** Go 1.26+. Existing packages used: `pkg/rsbuf` (extends `BuildArea` + rewrites `playerinfo.go`/`npcinfo.go`; `Player`/`Npc`/`Buf` already landed in NAI-29; `Renderer` consumed unchanged), `pkg/coordgrid` (`PackCoord`, `UnpackCoord`), `pkg/io/packet` (`Packet` with `AccessBits`/`AccessBytes`/`PBit`/`P1` etc.), `modules/world` (Player+Npc field extensions, encoder caller swaps, buildarea flatten). `PlayerSource`/`NpcSource` interfaces in `pkg/rsbuf/source.go` + `npc_source.go` remain alive (consumed by renderer + mask_payload layer); only the `AppearanceHash()` member is trimmed. No new third-party dependencies.

**Predecessors:** NAI-29 closed at `484cd98`. NAI-30 spec: `docs/superpowers/specs/2026-04-26-nai-30-encoder-readflip-design.md` committed at `4fd3075` and amended at `7b4da50`. Source root: `/home/owner/Code/github.com/2004scape/rsbuf` branch `225` (HEAD `1cbb2ce`) — the canonical Rust source per `rust_source_canonical_path` memory.

**Build/test commands** (per `CLAUDE.md`):
- Build: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
- Test all: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
- Test single package: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/...`
- Test single function: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestBuildArea_GetNearbyPlayers_ZoneWalkBounds`
- Race detector: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...`

**Commit discipline:** All commits use `git commit --no-gpg-sign`. Each commit body includes the standard `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer. Commit message format: `feat(rsbuf): NAI-30 Bundle N Task N.M — <one-line summary>` for code; `test(rsbuf): NAI-30 Bundle N Task N.M — ...` for test-only; `feat(world): NAI-30 Bundle 4 Task 4.M — <summary>` for B4 modules/world touches.

**Stale-IDE-diagnostic discipline** (per `verify_implementer_claims` failure-mode #1): After each task's implementation step, run a fresh `go build ./...` + `go test -count=1 ./...` to confirm. Ignore gopls/IDE warnings unless they reproduce in a fresh process — gopls cache lag has produced false positives across every prior NAI sub-spec.

**Plan-runnable test fixture discipline** (per `plan_runnable_test_fixtures` memory): Bundles 2 + 3 contain 41-arg `ComputePlayer` and 25-arg `ComputeNpc` test setup calls and 5+ branch decode assertions per encoder call — implementer mentally compiles each codified fixture before writing it, then re-counts arguments against `(*Buf).ComputePlayer` and `(*Buf).ComputeNpc` signatures in `pkg/rsbuf/buf.go`.

---

## File Structure

| File | Status | Bundle | Purpose |
|---|---|---|---|
| `pkg/rsbuf/buildarea.go` | Modified | B1 | Add `GetNearbyPlayers`, `GetNearbyNpcs`, `filterPlayer`, `filterNpc` methods |
| `pkg/rsbuf/buildarea_test.go` | Modified | B1 | Add coverage for the 4 new methods |
| `modules/world/player.go` | Modified | B1, B4 | B1: add `OrientationX/Z`, `lastAppearance` fields; B4: add scenery-window fields + methods, drop `buildArea` field |
| `modules/world/npc.go` | Modified | B1 | Add `OrientationX/Z` fields |
| `modules/world/appearance.go` | Modified | B1, B4 | B1: set `p.lastAppearance = currentTick`; B4: delete `appearanceHash` helper if dead |
| `modules/world/player_source.go` | Modified | B4 | Delete `(p *Player) AppearanceHash()` method |
| `modules/world/tick.go` | Modified | B1, B4 | B1: swap placeholder args; B4: delete `s.grid.Add/Remove`, delete `rsbuf.RemovePlayer(...)` shim call, delete `p.buildArea = ...` init |
| `modules/world/server.go` | Modified | B4 | Delete `s.grid = grid.New()` + `s.grid.AddNpc(...)` |
| `modules/world/data_map.go` | Modified | B4 | Flatten `p.buildArea.X` → `p.X` references |
| `modules/world/npc_hunt.go` | Modified | B4 | Migrate `rsbuf.GetNpcObservers(n.nid)` → `s.rsbuf.GetNpcObservers(int32(n.nid))` |
| `modules/world/npc_event_queue_test.go` | Modified | B4 | Migrate `rsbuf.SetObserverForTest(...)` → `s.rsbuf.SetObserverForTest(...)` |
| `modules/world/login_map_test.go` | Modified | B4 | Flatten `p.buildArea.OriginX/Z` → `p.originX/originZ` |
| `modules/world/data_map_test.go` | Modified | B4 | Flatten field accesses |
| `modules/world/player_zone_test.go` | Modified | B4 | Flatten field accesses |
| `modules/world/player_npc_test.go` | Modified | B4 | Migrate `p.buildArea.Npcs` reads to `s.rsbuf.HasNpc(...)` |
| `modules/world/player_info.go` | Modified | B4 | Swap to `s.rsbuf.PlayerInfo.Encode(...)` |
| `modules/world/player_npc_info.go` | Modified | B4 | Swap to `s.rsbuf.NpcInfo.Encode(...)` |
| `modules/world/player_info_test.go` | Modified | B4 | Migrate `a.buildArea.Players` reads to `s.rsbuf.HasPlayer(...)` |
| `pkg/rsbuf/playerinfo.go` | Rewritten | B2, B4 | B2: rename old `Encode` → `EncodeLegacy`, add `PlayerInfo` struct + method; B4: delete `EncodeLegacy` + dead helpers |
| `pkg/rsbuf/playerinfo_test.go` | Rewritten | B2, B4 | B2: port byte-level pin tests to `*Buf` fixtures; B4: delete legacy tests |
| `pkg/rsbuf/npcinfo.go` | Rewritten | B3, B4 | B3: rename old `EncodeNpc` → `EncodeNpcLegacy`, add `NpcInfo` struct + method; B4: delete legacy + helpers |
| `pkg/rsbuf/npcinfo_test.go` | Rewritten | B3, B4 | B3: port byte-level pin tests; B4: delete legacy tests |
| `pkg/rsbuf/buf.go` | Modified | B4 | Add `PlayerInfo *PlayerInfo`, `NpcInfo *NpcInfo` fields + `New()` initializers; add `(b *Buf) SetObserverForTest` method |
| `pkg/rsbuf/source.go` | Modified | B4 | Trim `AppearanceHash() uint64` member from interface |
| `pkg/grid/grid.go` | Deleted | B4 | All consumers migrated to `*Buf.zoneMap` |
| `pkg/grid/grid_test.go` | Deleted | B4 | |
| `pkg/buildarea/buildarea.go` | Deleted | B4 | Encoder fields already in `pkg/rsbuf.BuildArea`; scenery fields flattened onto Player |
| `pkg/buildarea/buildarea_test.go` | Deleted | B4 | |
| `pkg/rsbuf/npc_observers.go` | Deleted | B4 | Replaced by `(*Buf).GetNpcObservers` + `(*Buf).SetObserverForTest` methods |
| `pkg/rsbuf/npc_observers_test.go` | Deleted | B4 | |

---

# Bundle 1 — BuildArea spatial discovery + entity field plumbing

Bundle 1 lands the rsbuf-internal spatial-discovery primitives that NAI-30's new encoder will consume, plus real producer fields for `lastAppearance` and `OrientationX/Z` on `modules/world/Player` + `Npc`. Pure rsbuf-internal additions plus parallel-field plumbing — the existing encoder still functions correctly with the new field values (orientation `-1` is the new sentinel; lastAppearance tick is just a different `int32` value).

**Source mappings**: methods port from `2004scape/rsbuf/src/build.rs:178-327`; orientation fields from `player.rs:23-24`/`npc.rs:16-17`; lastAppearance from `player.rs:19`.

## Task 1.1: `(b *BuildArea) GetNearbyPlayers` — zone-walk variant

**Files:**
- Modify: `pkg/rsbuf/buildarea.go` (add method + private helper)
- Modify: `pkg/rsbuf/buildarea_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/rsbuf/buildarea_test.go`:

```go
func TestBuildArea_GetNearbyPlayers_EmptyZoneMapReturnsEmpty(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("empty zoneMap: got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_WindowMath(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	// Place player 5 at (96, 0, 96): zone (12, 12). Search center (100, 0, 100): zone (12, 12).
	// preferredViewDistance=15: (100-15)>>3=10 to (100+15)>>3=14 — zone (12,12) is in range.
	players[5] = newPlayer(5)
	players[5].Coord = packCoordTest(0, 96, 96)
	players[5].PID = 5
	zm.Zone(96, 0, 96).AddPlayer(5)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 1 || got[0] != 5 {
		t.Errorf("got %v, want [5]", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersAlreadyTracked(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[5] = newPlayer(5)
	players[5].Coord = packCoordTest(0, 100, 100)
	players[5].PID = 5
	zm.Zone(100, 0, 100).AddPlayer(5)
	ba.Players.Insert(5) // already tracked

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("already-tracked excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersOutOfDistance(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[5] = newPlayer(5)
	// Place far outside Chebyshev radius 15: (200, 0, 200) when self at (100, 0, 100).
	players[5].Coord = packCoordTest(0, 200, 200)
	players[5].PID = 5
	zm.Zone(200, 0, 200).AddPlayer(5)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("out-of-distance excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersNegativePid(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[5] = newPlayer(5)
	players[5].Coord = packCoordTest(0, 100, 100)
	players[5].PID = -1 // empty-slot marker
	zm.Zone(100, 0, 100).AddPlayer(5)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("pid=-1 excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersSelf(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[1] = newPlayer(1)
	players[1].Coord = packCoordTest(0, 100, 100)
	players[1].PID = 1
	zm.Zone(100, 0, 100).AddPlayer(1)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100) // self pid=1
	if len(got) != 0 {
		t.Errorf("self excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersDifferentLevel(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[5] = newPlayer(5)
	players[5].Coord = packCoordTest(1, 100, 100) // level 1
	players[5].PID = 5
	zm.Zone(100, 1, 100).AddPlayer(5)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100) // self at level 0
	if len(got) != 0 {
		t.Errorf("different-level excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_RespectsPreferredCap(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	// Insert 251 candidates in the same zone (preferredPlayers=250).
	for i := int32(2); i < 253; i++ {
		players[i] = newPlayer(i)
		players[i].Coord = packCoordTest(0, 100, 100)
		players[i].PID = i
		zm.Zone(100, 0, 100).AddPlayer(i)
	}
	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != int(preferredPlayers) {
		t.Errorf("cap respected: got len %d, want %d", len(got), preferredPlayers)
	}
}

// packCoordTest is a test helper alias; if not present, add:
//   func packCoordTest(level, x, z int) int { return coordgrid.PackCoord(level, x, z) }
// Many existing buildarea_test.go tests already use coordgrid.PackCoord directly; either pattern is fine.
```

If `packCoordTest` isn't already defined in the test file, add it once near the top:

```go
import (
	"github.com/zsrv/goscape/pkg/coordgrid"
)

func packCoordTest(level, x, z int) int { return coordgrid.PackCoord(level, x, z) }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestBuildArea_GetNearbyPlayers`
Expected: FAIL with `ba.GetNearbyPlayers undefined`.

- [ ] **Step 3: Implement the method**

Append to `pkg/rsbuf/buildarea.go` (after `SaveAppearance`):

```go
// GetNearbyPlayers returns up to (preferredPlayers - len(b.Players))
// pids of players within preferredViewDistance zones (Chebyshev) of
// (x, level, z), excluding the local player (pid) and any player
// already in the tracking set. Mirrors upstream
// BuildArea::get_nearby_players_zones at build.rs:178-213 (zone-walk
// variant; the dispatcher between this and the spiral fallback
// `get_nearby_players_nearest` is NAI-32 scope).
//
// view distance is fixed at preferredViewDistance in NAI-30; NAI-32
// will introduce dynamic resize via a parameter.
func (b *BuildArea) GetNearbyPlayers(players *[2048]*Player, zoneMap *zoneMap, pid int32, x, level, z int) []int32 {
	distance := int(preferredViewDistance)
	startZX := (x - distance) >> 3
	startZZ := (z - distance) >> 3
	endZX := (x + distance) >> 3
	endZZ := (z + distance) >> 3

	count := b.Players.Len()
	cap := int(preferredPlayers) - count
	if cap <= 0 {
		return nil
	}
	nearby := make([]int32, 0, cap)

	for zx := startZX; zx <= endZX; zx++ {
		for zz := startZZ; zz <= endZZ; zz++ {
			if len(nearby)+count >= int(preferredPlayers) {
				return nearby
			}
			zonePlayers := zoneMap.Zone(zx<<3, level, zz<<3).players
			for _, candidate := range zonePlayers {
				if len(nearby)+count >= int(preferredPlayers) {
					return nearby
				}
				if b.filterPlayer(players, candidate, pid, x, level, z) {
					nearby = append(nearby, candidate)
				}
			}
		}
	}
	return nearby
}

// filterPlayer reports whether `candidate` should be added to a
// nearby-players result. Mirrors upstream BuildArea::filter_player
// at build.rs:298-312. Five reject conditions: already tracked,
// out-of-distance (Chebyshev), pid==-1 (empty-slot marker),
// pid==self (self exclusion), level mismatch.
func (b *BuildArea) filterPlayer(players *[2048]*Player, candidate, pid int32, x, level, z int) bool {
	if candidate < 0 || int(candidate) >= len(players) {
		return false
	}
	other := players[candidate]
	if other == nil {
		return false
	}
	if b.Players.Contains(candidate) {
		return false
	}
	if other.PID == -1 {
		return false
	}
	if other.PID == pid {
		return false
	}
	otherPos := coordgrid.UnpackCoord(other.Coord)
	if otherPos.Level != level {
		return false
	}
	if !withinDistanceSW(otherPos.X, otherPos.Z, x, z, int(preferredViewDistance)) {
		return false
	}
	return true
}

// withinDistanceSW returns true if the Chebyshev distance between
// (ax, az) and (bx, bz) is <= radius. Mirrors upstream
// CoordGrid::within_distance_sw at coord.rs:50-58 (max of |dx|, |dz|
// against radius).
func withinDistanceSW(ax, az, bx, bz, radius int) bool {
	dx := ax - bx
	if dx < 0 {
		dx = -dx
	}
	dz := az - bz
	if dz < 0 {
		dz = -dz
	}
	if dx > dz {
		return dx <= radius
	}
	return dz <= radius
}
```

If `coordgrid` isn't already imported in `buildarea.go`, add it: `import "github.com/zsrv/goscape/pkg/coordgrid"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestBuildArea_GetNearbyPlayers`
Expected: PASS (8 tests).

Then run full pkg/rsbuf tests to confirm no regressions: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/...`

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/buildarea.go pkg/rsbuf/buildarea_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-30 Bundle 1 Task 1.1 — BuildArea.GetNearbyPlayers (zone-walk variant)

Ports BuildArea::get_nearby_players_zones at build.rs:178-213 +
BuildArea::filter_player at build.rs:298-312. Fixed view distance
= preferredViewDistance (15); NAI-32 will introduce the dispatcher
that selects between this and the spiral fallback based on dynamic
view_distance.

Also adds withinDistanceSW package helper (mirrors
CoordGrid::within_distance_sw at coord.rs:50-58).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1.2: `(b *BuildArea) GetNearbyNpcs`

**Files:**
- Modify: `pkg/rsbuf/buildarea.go` (add method + helper)
- Modify: `pkg/rsbuf/buildarea_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/rsbuf/buildarea_test.go`:

```go
func TestBuildArea_GetNearbyNpcs_EmptyZoneMapReturnsEmpty(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var npcs [8192]*Npc
	got := ba.GetNearbyNpcs(&npcs, zm, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("empty zoneMap: got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyNpcs_FindsInRange(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var npcs [8192]*Npc
	npcs[5] = newNpc(5, 100)
	npcs[5].Coord = packCoordTest(0, 96, 96)
	npcs[5].Active = true
	zm.Zone(96, 0, 96).AddNpc(5)

	got := ba.GetNearbyNpcs(&npcs, zm, 100, 0, 100)
	if len(got) != 1 || got[0] != 5 {
		t.Errorf("got %v, want [5]", got)
	}
}

func TestBuildArea_GetNearbyNpcs_FiltersAlreadyTracked(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var npcs [8192]*Npc
	npcs[5] = newNpc(5, 100)
	npcs[5].Coord = packCoordTest(0, 100, 100)
	npcs[5].Active = true
	zm.Zone(100, 0, 100).AddNpc(5)
	ba.Npcs.Insert(5)

	got := ba.GetNearbyNpcs(&npcs, zm, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("already-tracked excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyNpcs_FiltersOutOfDistance(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var npcs [8192]*Npc
	npcs[5] = newNpc(5, 100)
	npcs[5].Coord = packCoordTest(0, 200, 200)
	npcs[5].Active = true
	zm.Zone(200, 0, 200).AddNpc(5)

	got := ba.GetNearbyNpcs(&npcs, zm, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("out-of-distance excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyNpcs_FiltersNegativeNid(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var npcs [8192]*Npc
	npcs[5] = newNpc(5, 100)
	npcs[5].Coord = packCoordTest(0, 100, 100)
	npcs[5].NID = -1
	npcs[5].Active = true
	zm.Zone(100, 0, 100).AddNpc(5)

	got := ba.GetNearbyNpcs(&npcs, zm, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("nid=-1 excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyNpcs_FiltersDifferentLevel(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var npcs [8192]*Npc
	npcs[5] = newNpc(5, 100)
	npcs[5].Coord = packCoordTest(1, 100, 100)
	npcs[5].Active = true
	zm.Zone(100, 1, 100).AddNpc(5)

	got := ba.GetNearbyNpcs(&npcs, zm, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("different-level excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyNpcs_FiltersInactive(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var npcs [8192]*Npc
	npcs[5] = newNpc(5, 100)
	npcs[5].Coord = packCoordTest(0, 100, 100)
	npcs[5].Active = false // inactive
	zm.Zone(100, 0, 100).AddNpc(5)

	got := ba.GetNearbyNpcs(&npcs, zm, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("inactive excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyNpcs_RespectsPreferredCap(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var npcs [8192]*Npc
	// Insert 256 candidates (preferredNpcs=255).
	for i := int32(1); i <= 256; i++ {
		npcs[i] = newNpc(i, 100)
		npcs[i].Coord = packCoordTest(0, 100, 100)
		npcs[i].Active = true
		zm.Zone(100, 0, 100).AddNpc(i)
	}
	got := ba.GetNearbyNpcs(&npcs, zm, 100, 0, 100)
	if len(got) != int(preferredNpcs) {
		t.Errorf("cap respected: got len %d, want %d", len(got), preferredNpcs)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestBuildArea_GetNearbyNpcs`
Expected: FAIL with `ba.GetNearbyNpcs undefined`.

- [ ] **Step 3: Implement the method**

Append to `pkg/rsbuf/buildarea.go` (after `filterPlayer`):

```go
// GetNearbyNpcs returns up to (preferredNpcs - len(b.Npcs)) nids of
// active NPCs within preferredViewDistance zones (Chebyshev) of
// (x, level, z), excluding any NPC already in the tracking set.
// Mirrors upstream BuildArea::get_nearby_npcs at build.rs:262-296.
//
// View distance is the const preferredViewDistance (15); upstream
// hardcodes BuildArea::PREFERRED_VIEW_DISTANCE here even when player
// view distance shrinks (NPCs don't downsize their search radius).
func (b *BuildArea) GetNearbyNpcs(npcs *[8192]*Npc, zoneMap *zoneMap, x, level, z int) []int32 {
	distance := int(preferredViewDistance)
	startZX := (x - distance) >> 3
	startZZ := (z - distance) >> 3
	endZX := (x + distance) >> 3
	endZZ := (z + distance) >> 3

	count := b.Npcs.Len()
	cap := int(preferredNpcs) - count
	if cap <= 0 {
		return nil
	}
	nearby := make([]int32, 0, cap)

	for zx := startZX; zx <= endZX; zx++ {
		for zz := startZZ; zz <= endZZ; zz++ {
			if len(nearby)+count >= int(preferredNpcs) {
				return nearby
			}
			zoneNpcs := zoneMap.Zone(zx<<3, level, zz<<3).npcs
			for _, candidate := range zoneNpcs {
				if len(nearby)+count >= int(preferredNpcs) {
					return nearby
				}
				if b.filterNpc(npcs, candidate, x, level, z) {
					nearby = append(nearby, candidate)
				}
			}
		}
	}
	return nearby
}

// filterNpc reports whether `candidate` should be added to a
// nearby-npcs result. Mirrors upstream BuildArea::filter_npc at
// build.rs:314-327. Five reject conditions: already tracked,
// out-of-distance (Chebyshev), nid==-1 (empty-slot marker),
// level mismatch, !active.
func (b *BuildArea) filterNpc(npcs *[8192]*Npc, candidate int32, x, level, z int) bool {
	if candidate < 0 || int(candidate) >= len(npcs) {
		return false
	}
	other := npcs[candidate]
	if other == nil {
		return false
	}
	if b.Npcs.Contains(candidate) {
		return false
	}
	if other.NID == -1 {
		return false
	}
	if !other.Active {
		return false
	}
	otherPos := coordgrid.UnpackCoord(other.Coord)
	if otherPos.Level != level {
		return false
	}
	if !withinDistanceSW(otherPos.X, otherPos.Z, x, z, int(preferredViewDistance)) {
		return false
	}
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestBuildArea_GetNearbyNpcs`
Expected: PASS (8 tests).

Then full package: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/...`

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/buildarea.go pkg/rsbuf/buildarea_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-30 Bundle 1 Task 1.2 — BuildArea.GetNearbyNpcs + filterNpc

Ports BuildArea::get_nearby_npcs at build.rs:262-296 +
BuildArea::filter_npc at build.rs:314-327. Uses fixed
preferredViewDistance (15); upstream hardcodes
BuildArea::PREFERRED_VIEW_DISTANCE here regardless of dynamic
player view distance.

Five reject branches pinned individually: already tracked,
out-of-distance (Chebyshev), nid==-1, level mismatch, !active.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1.3: `Player.OrientationX/Z` + `Npc.OrientationX/Z` field plumbing

**Files:**
- Modify: `modules/world/player.go` (add struct fields, update `newPlayer`)
- Modify: `modules/world/npc.go` (add struct fields, update `newNpc`)
- Modify: `modules/world/tick.go` (swap `int32(0), int32(0)` placeholders for real fields)
- Modify: `modules/world/player_test.go` and `modules/world/npc_test.go` (add tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/player_test.go`:

```go
func TestNewPlayer_OrientationXZ_DefaultMinusOne(t *testing.T) {
	p := newPlayer(nil)
	if p.OrientationX != -1 {
		t.Errorf("OrientationX default: got %d, want -1", p.OrientationX)
	}
	if p.OrientationZ != -1 {
		t.Errorf("OrientationZ default: got %d, want -1", p.OrientationZ)
	}
}
```

Append to `modules/world/npc_test.go`:

```go
func TestNewNpc_OrientationXZ_DefaultMinusOne(t *testing.T) {
	n := &Npc{}
	*n = newNpcDefaults() // or whatever the test scaffold pattern is
	if n.OrientationX != -1 {
		t.Errorf("OrientationX default: got %d, want -1", n.OrientationX)
	}
	if n.OrientationZ != -1 {
		t.Errorf("OrientationZ default: got %d, want -1", n.OrientationZ)
	}
}
```

If `newNpcDefaults()` doesn't exist, write the test against the actual constructor used in the file (e.g., `n := newTestNpc(...)` from existing helpers). The test should call whatever path `addNpc`/spawn uses — the assertion is what matters, not the constructor name. Plan-author re-greps `newNpc(` in `modules/world/` and uses the actual constructor.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestNewPlayer_OrientationXZ|TestNewNpc_OrientationXZ"`
Expected: FAIL with `p.OrientationX undefined` (and `n.OrientationX undefined`).

- [ ] **Step 3: Add fields + initializers + tick.go swap**

In `modules/world/player.go`, add to the `Player` struct (place near other facing-related fields like `faceSquareX/faceSquareZ` if present, or near the end with a comment):

```go
// OrientationX, OrientationZ are persistent face-direction defaults used
// by the encoder when faceSquareX/Z are unset. Default -1 = "no value"
// per upstream player.rs:23-24. NAI-30-D1: producer (set_orient script
// command + initial orientation from npc-config) deferred to engine-port
// series; field stays at -1 in NAI-30, encoder fallback to player coord
// matches upstream behavior at info.rs:328-340.
OrientationX, OrientationZ int
```

In `newPlayer(...)` initializer, add:

```go
OrientationX: -1,
OrientationZ: -1,
```

In `modules/world/npc.go`, add to the `Npc` struct (analogous placement):

```go
// OrientationX, OrientationZ default to -1 per upstream npc.rs:16-17.
// See NAI-30-D1 (orientation field plumbed without producer).
OrientationX, OrientationZ int
```

In the NPC constructor (whatever `newNpc` / `addNpc` initializes), add:

```go
OrientationX: -1,
OrientationZ: -1,
```

In `modules/world/tick.go`, swap placeholder lines 376 and 412:

```go
// Line 376 — was: int32(0), int32(0), // NAI-30: orientationX/Z not stored on Player today
int32(p.OrientationX), int32(p.OrientationZ),

// Line 412 — was: int32(0), int32(0), // NAI-30: orientationX/Z not stored on Npc today
int32(n.OrientationX), int32(n.OrientationZ),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestNewPlayer_OrientationXZ|TestNewNpc_OrientationXZ"`
Expected: PASS.

Run full module tests: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`
Expected: all pass (no regressions from the placeholder swap).

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go modules/world/npc.go modules/world/tick.go modules/world/player_test.go modules/world/npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-30 Bundle 1 Task 1.3 — Player+Npc OrientationX/Z fields with -1 default

Adds OrientationX, OrientationZ int fields to Player + Npc with -1
defaults, replacing the int32(0), int32(0) placeholders at
tick.go:376,412 with int32(p.OrientationX), int32(p.OrientationZ)
and the NPC analogue.

Encoder fallback at info.rs:328-340 (which NAI-30 Bundle 2/3 ports)
treats -1 as "no value" and falls back to player coord — visually
correct in the absence of a producer. Producer wiring (set_orient
script command + npc-config initial orientation) deferred to the
engine-port series. Tracked as NAI-30-D1.

Mirrors upstream player.rs:23-24,107-108 + npc.rs:16-17,71-72.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1.4: `Player.lastAppearance` field + producer in `generateAppearance`

**Files:**
- Modify: `modules/world/player.go` (add `lastAppearance int` field with `-1` default)
- Modify: `modules/world/appearance.go` (set `p.lastAppearance = currentTick` after buffer write)
- Modify: `modules/world/tick.go` (swap `AppearanceHash()` arg for the new field)
- Modify: `modules/world/player_test.go` (add field-default test)
- Modify: `modules/world/appearance_test.go` (add producer-set test)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/player_test.go`:

```go
func TestNewPlayer_LastAppearance_DefaultMinusOne(t *testing.T) {
	p := newPlayer(nil)
	if p.lastAppearance != -1 {
		t.Errorf("lastAppearance default: got %d, want -1", p.lastAppearance)
	}
}
```

Append to `modules/world/appearance_test.go`:

```go
func TestGenerateAppearance_SetsLastAppearanceToCurrentTick(t *testing.T) {
	objs, invs := newAppearanceConfigs(t)
	p := newPlayer(nil) // or whatever scaffold the existing tests in this file use
	// Equip with whatever fixture other tests in this file use (e.g., from t.Helper test setup).
	// The setup pattern matches the existing TestGenerateAppearance tests in this file.

	p.generateAppearance(objs, invs, 42)

	if p.lastAppearance != 42 {
		t.Errorf("lastAppearance after generateAppearance(_, _, 42): got %d, want 42", p.lastAppearance)
	}

	p.generateAppearance(objs, invs, 100)
	if p.lastAppearance != 100 {
		t.Errorf("lastAppearance after generateAppearance(_, _, 100): got %d, want 100", p.lastAppearance)
	}
}
```

If the existing tests use a different setup helper than `newAppearanceConfigs(t)`, use the actual helper from existing `appearance_test.go` patterns.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestNewPlayer_LastAppearance|TestGenerateAppearance_SetsLastAppearanceToCurrentTick"`
Expected: FAIL with `p.lastAppearance undefined`.

- [ ] **Step 3: Add field + producer + tick.go swap**

In `modules/world/player.go`, add to the `Player` struct (place near `appearanceBuf` / appearance-related fields):

```go
// lastAppearance is the world tick when generateAppearance() last
// rebuilt p.appearanceBuf. Used by the encoder to dedupe appearance
// blocks per (other, viewer) pair: viewer sends APPEARANCE only when
// its own build.appearances[other_pid] != other.lastAppearance.
// Default -1 = "never generated" — encoder skips APPEARANCE entirely
// per info.rs:305 guard. Mirrors upstream player.rs:19.
lastAppearance int
```

In `newPlayer(...)` initializer, add:

```go
lastAppearance: -1,
```

In `modules/world/appearance.go`, find the `generateAppearance` function (line ~22) and at the end of the function (after `p.appearanceBuf = ...` is finalized), add:

```go
p.lastAppearance = currentTick
```

In `modules/world/tick.go` line 373, swap:

```go
// Was: int32(p.AppearanceHash()&0x7fffffff), // NAI-30: revisit lastAppearance semantics (content-hash vs tick-when-changed)
int32(p.lastAppearance),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestNewPlayer_LastAppearance|TestGenerateAppearance_SetsLastAppearanceToCurrentTick"`
Expected: PASS.

Run full module tests: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`
Expected: all pass — note that any existing test asserting the OLD behavior of `p.lastAppearance` value at tick.go:373 may need adjusting; if so, fix inline.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go modules/world/appearance.go modules/world/tick.go modules/world/player_test.go modules/world/appearance_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-30 Bundle 1 Task 1.4 — Player.lastAppearance tick-when-changed

Adds Player.lastAppearance int field (default -1), set to currentTick
inside generateAppearance(). Replaces tick.go:373 placeholder
int32(p.AppearanceHash()&0x7fffffff) with int32(p.lastAppearance).

This corrects an NAI-29 parallel-write divergence: upstream uses
tick-when-changed semantics (player.rs:19); NAI-29 had used a
content-hash workaround flagged inline. The tick port restores
upstream's "last_appearance == -1 means never generated" guard at
info.rs:305 (NAI-30 Bundle 2 ports the encoder side).

AppearanceHash() method becomes dead after this commit but stays
present until B4's grep-and-delete pass (PlayerSource interface
currently still declares it).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 1.5: Cross-bundle integration tests for tick.go arg propagation

**Files:**
- Modify: `modules/world/rsbuf_per_tick_test.go` (add tests verifying real fields land in `*Buf`)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/rsbuf_per_tick_test.go`:

```go
func TestProcessInfo_PassesRealOrientationFields(t *testing.T) {
	s := newTestServer(t)
	p, _ := s.addTestPlayer(t)
	p.OrientationX = 1234
	p.OrientationZ = 5678

	s.processInfo()

	got := s.rsbuf // direct *Buf access via test scaffold; if scaffolds differ, adapt
	// The test peers into the rsbuf state via the existing accessors used
	// in nearby tests; if rsbuf doesn't expose Player accessors yet, add a
	// minimal test-only accessor or use HasPlayer to confirm presence.
	_ = got
	// Assertion approach 1: if a *Buf accessor like (b *Buf).PlayerCoord(pid) exists,
	// add (b *Buf).PlayerOrientation(pid) (int, int) for tests.
	// Assertion approach 2: simpler — add (b *Buf).Player(pid) *Player accessor
	// (test-only or unexported with build-tag).
	// Plan-author chooses based on existing rsbuf_per_tick_test.go patterns.
}

func TestProcessInfo_PassesRealLastAppearance(t *testing.T) {
	s := newTestServer(t)
	p, _ := s.addTestPlayer(t)
	p.lastAppearance = 42

	s.processInfo()

	// Same accessor pattern as above — pin that b.players[pid].LastAppearance == 42
	// after the per-tick push.
	_ = s
}
```

If the existing `rsbuf_per_tick_test.go` already has accessor helpers, use them. If not, the simplest path: add a minimal exported accessor on `*Buf`:

```go
// In pkg/rsbuf/buf.go (add after GetNpcObservers):
//
// PlayerForTest returns the *Player at slot pid, or nil if unset.
// Test-only accessor; production code uses PlayerInfo.Encode and
// the dedicated query methods (HasPlayer, etc.).
func (b *Buf) PlayerForTest(pid int32) *Player {
    if pid < 0 || int(pid) >= len(b.players) {
        return nil
    }
    return b.players[pid]
}
```

Then the test body becomes:

```go
func TestProcessInfo_PassesRealOrientationFields(t *testing.T) {
	s := newTestServer(t)
	p, _ := s.addTestPlayer(t)
	p.OrientationX = 1234
	p.OrientationZ = 5678

	s.processInfo()

	rp := s.rsbuf.PlayerForTest(int32(p.slot))
	if rp == nil {
		t.Fatal("rsbuf player slot not populated")
	}
	if rp.OrientationX != 1234 {
		t.Errorf("OrientationX: got %d, want 1234", rp.OrientationX)
	}
	if rp.OrientationZ != 5678 {
		t.Errorf("OrientationZ: got %d, want 5678", rp.OrientationZ)
	}
}

func TestProcessInfo_PassesRealLastAppearance(t *testing.T) {
	s := newTestServer(t)
	p, _ := s.addTestPlayer(t)
	p.lastAppearance = 42

	s.processInfo()

	rp := s.rsbuf.PlayerForTest(int32(p.slot))
	if rp == nil {
		t.Fatal("rsbuf player slot not populated")
	}
	if rp.LastAppearance != 42 {
		t.Errorf("LastAppearance: got %d, want 42", rp.LastAppearance)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestProcessInfo_PassesReal"`
Expected: FAIL — either `s.rsbuf.PlayerForTest undefined` (if accessor not added) or assertion failures (if Bundle 1 Task 1.3 + 1.4 swaps haven't propagated correctly).

- [ ] **Step 3: Add `PlayerForTest` accessor**

If not already added in Step 1's test draft, append to `pkg/rsbuf/buf.go`:

```go
// PlayerForTest returns the *Player at slot pid, or nil if unset.
// Test-only accessor exposed for cross-package integration tests in
// modules/world; production code uses PlayerInfo.Encode and the
// dedicated query methods.
func (b *Buf) PlayerForTest(pid int32) *Player {
	if pid < 0 || int(pid) >= len(b.players) {
		return nil
	}
	return b.players[pid]
}

// NpcForTest returns the *Npc at slot nid, or nil if unset.
// Test-only accessor.
func (b *Buf) NpcForTest(nid int32) *Npc {
	if nid < 0 || int(nid) >= len(b.npcs) {
		return nil
	}
	return b.npcs[nid]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestProcessInfo_PassesReal"`
Expected: PASS.

Run full module + rsbuf tests: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/... ./modules/world/...`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/rsbuf_per_tick_test.go pkg/rsbuf/buf.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-30 Bundle 1 Task 1.5 — pin tick.go arg propagation for orientation+lastAppearance

Cross-bundle integration: verifies that real OrientationX/Z and
lastAppearance values from modules/world Player land in
*Buf.players[pid] after processInfo's per-tick state push, closing
the test-coverage loop on Tasks 1.3 + 1.4.

Adds (b *Buf).PlayerForTest + NpcForTest accessors for cross-package
test access to slot-table state.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Bundle 2 — PlayerInfo encoder port

Bundle 2 rewrites `pkg/rsbuf/playerinfo.go` from interface-based package function to `*Buf`-attached `PlayerInfo` struct with reusable scratch buffers. Mirrors upstream `info.rs:13-409` line-by-line. The OLD `Encode(...)` package function is renamed to `EncodeLegacy(...)` so it co-exists during this bundle; B4 deletes it after caller swap.

**Source mappings**: `info.rs:13-409` (PlayerInfo struct + encode methods); `info.rs:362-401` (mask block ordering); `info.rs:296-346` (lowdefinition's APPEARANCE/FACE_COORD logic).

**Renderer interaction note**: NAI-30 keeps the existing goscape `*Renderer` (with its `HighDefOf(slot)`/`LowDefFullOf(slot)`/`LowDefNoAppOf(slot)` accessors) — NAI-31 ports the renderer to upstream's per-mask `compute_info` cache. So PlayerInfo.Encode calls `r.HighDefOf(int(pid))` etc. directly; the upstream `renderer.has`/`renderer.cache`/`renderer.write` per-mask API is NAI-31 scope.

## Task 2.1: Rename old encoder to `EncodeLegacy`

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go` (rename `Encode` → `EncodeLegacy`)
- Modify: `modules/world/player_info.go` (callsite swap to `EncodeLegacy`)
- Modify: `pkg/rsbuf/playerinfo_test.go` (update test names if they use `Encode`)

- [ ] **Step 1: Verify no other callers exist**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS (baseline green before rename).

Run: `rg "rsbuf\.Encode\b\(" pkg/ modules/ cmd/`
Expected output (only one production callsite + a few test sites):
```
modules/world/player_info.go: payload := rsbuf.Encode(p, sources, p.buildArea, s.grid, s.renderer)
pkg/rsbuf/playerinfo_test.go: <... test calls ...>
```

If unexpected production callers appear, plan-author re-evaluates the rename strategy before proceeding.

- [ ] **Step 2: Rename function in source**

In `pkg/rsbuf/playerinfo.go` line 17, rename:

```go
// Was: func Encode(self PlayerSource, all []PlayerSource, ba *buildarea.BuildArea, g *grid.Grid, r *Renderer) []byte {
func EncodeLegacy(self PlayerSource, all []PlayerSource, ba *buildarea.BuildArea, g *grid.Grid, r *Renderer) []byte {
```

Update doc-comment above:

```go
// EncodeLegacy is the NAI-29-and-earlier interface-based encoder.
// Retained during NAI-30 Bundle 2/3 only as a transition fallback
// while the new (pi *PlayerInfo).Encode method on *Buf is being
// landed and validated. Callers swap to the new method in NAI-30
// Bundle 4 Task 4.2; this function deletes in B4 Task 4.6.
func EncodeLegacy(...) []byte { ... }
```

- [ ] **Step 3: Update sole production caller**

In `modules/world/player_info.go` line 26:

```go
// Was: payload := rsbuf.Encode(p, sources, p.buildArea, s.grid, s.renderer)
payload := rsbuf.EncodeLegacy(p, sources, p.buildArea, s.grid, s.renderer)
```

In `pkg/rsbuf/playerinfo_test.go`, do a single sweep `rsbuf.Encode(` → `rsbuf.EncodeLegacy(` for all test calls. Or, since the test is in the same package, `Encode(` → `EncodeLegacy(` (no qualifier).

- [ ] **Step 4: Run tests + build to verify everything still passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/... ./modules/world/...
```
Expected: PASS — pure rename, no behavior change.

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go modules/world/player_info.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(rsbuf): NAI-30 Bundle 2 Task 2.1 — rename Encode → EncodeLegacy

Pure rename: rsbuf.Encode → rsbuf.EncodeLegacy. Sole production caller
at modules/world/player_info.go:26 updated; test sites in
pkg/rsbuf/playerinfo_test.go swept.

Prepares for NAI-30 B2 to introduce (pi *PlayerInfo).Encode method
that coexists during T2.2-T2.9; B4 T4.6 deletes EncodeLegacy after
caller swap at B4 T4.2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2.2: `PlayerInfo` struct + `Encode` skeleton + idle test

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go` (add `PlayerInfo` struct + `NewPlayerInfo` + `Encode` skeleton)
- Modify: `pkg/rsbuf/playerinfo_test.go` (add idle-path test for new method)

- [ ] **Step 1: Write the failing test**

Append to `pkg/rsbuf/playerinfo_test.go`:

```go
func TestPlayerInfo_Encode_LocalIdleNoOthers(t *testing.T) {
	b := New()
	pi := NewPlayerInfo()
	b.AddPlayer(1)
	// ComputePlayer with all sentinels — local stationary at (3200, 0, 3200), no masks,
	// no exact move. 41-arg signature; verify against (*Buf).ComputePlayer in pkg/rsbuf/buf.go.
	b.ComputePlayer(
		1,           // pid
		3200, 0, 3200, // x, level, z
		3200, 3200,    // originX, originZ
		false, false,  // tele, jump
		-1, -1,        // runDir, walkDir
		VisibilityDefault, // visibility
		true,              // active
		0,                 // masks
		nil,               // appearance
		-1,                // lastAppearance
		-1,                // faceEntity
		-1, -1,            // faceX, faceZ
		-1, -1,            // orientationX, orientationZ
		-1, -1,            // damageTaken, damageType
		-1, -1,            // currentHitpoints, baseHitpoints
		-1, -1,            // animID, animDelay
		nil,               // say
		nil, 0, 0, 0,      // message, color, effect, ignored
		-1, -1, -1,        // graphicID, graphicHeight, graphicDelay
		-1, -1,            // exactStartX, exactStartZ
		-1, -1,            // exactEndX, exactEndZ
		-1, -1, -1,        // exactMoveStart, exactMoveEnd, exactMoveDirection
	)

	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Local idle: 1 leading bit `0` (not-update flag), then 8-bit "0 other players tracked".
	// First byte: 0_0000000 (idle local, then top 7 bits of count) — verify the leading byte
	// is 0 in bit-MSB order. Total 2 bytes when no updates buffer follows.
	if len(out) < 1 {
		t.Fatalf("encode produced empty output")
	}
	if out[0] != 0 {
		t.Errorf("local-idle leading byte: got 0x%02x, want 0x00 (idle bit + count high)", out[0])
	}
	// No updates buffer payload appended (no extends, no appearance triggers).
	// Total length is bit-aligned to 9 bits (1 idle + 8 count) → ceil(9/8) = 2 bytes.
	if len(out) != 2 {
		t.Errorf("local-idle, no others: total bytes got %d, want 2", len(out))
	}
}
```

(Plan-author re-verifies the 41-arg call against `(*Buf).ComputePlayer` signature in `pkg/rsbuf/buf.go:153-175` per `plan_runnable_test_fixtures` memory; if the count differs, fix inline before committing.)

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_Encode_LocalIdleNoOthers`
Expected: FAIL with `NewPlayerInfo undefined` (or `pi.Encode undefined`).

- [ ] **Step 3: Implement `PlayerInfo` struct + skeleton**

Append to `pkg/rsbuf/playerinfo.go` (below the renamed `EncodeLegacy`):

```go
// PlayerInfo holds reusable scratch buffers for PlayerInfo encoding.
// One instance per *Buf; reset and reused across all per-tick Encode
// calls (one per player). Mirrors upstream PlayerInfo struct at
// info.rs:13-16 — the rsbuf-internal singleton (`PLAYER_INFO:
// Lazy<Mutex<PlayerInfo>>` at lib.rs:36) collected onto the *Buf
// instance.
type PlayerInfo struct {
	buf     *packet.Packet
	updates *packet.Packet
}

// NewPlayerInfo allocates fresh scratch buffers sized for typical
// PlayerInfo packets (~5000 bytes upstream Packet::new(5000)).
// Mirrors PlayerInfo::new at info.rs:24-30.
func NewPlayerInfo() *PlayerInfo {
	return &PlayerInfo{
		buf:     packet.NewPacket(make([]byte, 0, 5000)),
		updates: packet.NewPacket(make([]byte, 0, 5000)),
	}
}

// Bit-budget constants for fits() arithmetic. Mirror upstream
// PlayerInfo::BITS_* at info.rs:19-22.
const (
	playerBitsAdd    = 11 + 5 + 5 + 1 + 1 // 23
	playerBitsRun    = 1 + 2 + 3 + 3 + 1  // 10
	playerBitsWalk   = 1 + 2 + 3 + 1      // 7
	playerBitsExtend = 1 + 2              // 3

	// Per-packet byte budget. Mirrors upstream literal at info.rs:407.
	maxPlayerInfoBytes = 4997
)

// Encode produces the PlayerInfo payload for `pid` as a fresh []byte
// (no opcode/length prefix; caller wraps with OpPlayerInfo).
// Mirrors upstream PlayerInfo::encode at info.rs:32-70.
//
// Signature divergences from upstream:
//   - `pos` upstream param dropped: NAI-30 always starts at byte 0
//     (each Encode call wraps standalone).
//   - `dx`, `dz`, `rebuild` upstream params dropped: NAI-30 doesn't
//     run BuildArea.rebuild_players (view-distance resize is NAI-32).
//   - `players: &[Option<Player>]`, `grid: &HashMap<...>`,
//     `map: &mut ZoneMap` collapse into `b *Buf`.
//   - `player: &mut Player` collapses into `b.players[pid]`.
//
// Returns nil if pid is out of range or slot is unpopulated.
func (pi *PlayerInfo) Encode(b *Buf, pid int32, renderer *Renderer) []byte {
	if pid < 0 || int(pid) >= len(b.players) {
		return nil
	}
	self := b.players[pid]
	if self == nil {
		return nil
	}

	// Reset scratch buffers (mirrors info.rs:53-56 zeroing).
	// (*Packet).Reset at pkg/io/packet/buffer.go:103-108 already does
	// Data[:0] + Pos=0 + BitPos=0 + lastRead=opInvalid.
	pi.buf.Reset()
	pi.updates.Reset()

	pi.buf.AccessBits()

	// Bundle 2 Task 2.3 will fill writeLocalPlayer here.
	// For Task 2.2 skeleton, write idle bit only.
	pi.buf.PBit(1, 0) // idle

	// Bundle 2 Task 2.4 will fill writePlayers here.
	// For T2.2 skeleton, emit zero-count.
	pi.buf.PBit(8, 0)

	// Bundle 2 Task 2.5 will fill writeNewPlayers.

	// Mirrors info.rs:62-68: append updates buffer if non-empty,
	// preceded by the 11-bit `2047` sentinel. NB: detect "non-empty"
	// via `len(pi.updates.Data) > 0`, not `pi.updates.Pos`. In the
	// project's packet.Packet shape (pkg/io/packet/buffer.go:20),
	// Pos is the READ pointer; writes append to len(Data). Mirrors
	// the EncodeLegacy pattern at playerinfo.go:31,37-39.
	if len(pi.updates.Data) > 0 {
		pi.buf.PBit(11, 2047)
		pi.buf.AccessBytes()
		for _, b2 := range pi.updates.Data {
			pi.buf.P1(b2)
		}
	} else {
		pi.buf.AccessBytes()
	}

	// Return a copy — the caller may write more to its OpPlayerInfo
	// wrapper, and pi.buf.Data is reused next call.
	out := make([]byte, len(pi.buf.Data))
	copy(out, pi.buf.Data)
	return out
}
```

Add the `packet` import if not already present: the file likely imports it from the legacy code path; verify.

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_Encode_LocalIdleNoOthers`
Expected: PASS.

Run full pkg/rsbuf: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/...`
Expected: all pass (legacy tests still green; new skeleton test green).

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-30 Bundle 2 Task 2.2 — PlayerInfo struct + Encode skeleton

Lands the new PlayerInfo struct (with reusable buf+updates scratch
buffers per upstream info.rs:13-16) + NewPlayerInfo constructor
(info.rs:24-30) + (pi *PlayerInfo).Encode method with prologue +
epilogue but stub branch bodies (idle local + zero-count others).

Bit-budget constants (playerBitsAdd=23, playerBitsRun=10,
playerBitsWalk=7, playerBitsExtend=3, maxPlayerInfoBytes=4997)
mirror info.rs:19-22,407.

Local idle + no others: 2 bytes total (1 idle bit + 8 count bits =
9 bits → ceil to 2 bytes). Verified by
TestPlayerInfo_Encode_LocalIdleNoOthers.

Encode signature divergences from upstream documented inline:
drops pos/dx/dz/rebuild upstream params (rebuild logic is NAI-32
scope); collapses players/grid/map slice+map params into *Buf;
collapses &mut Player param into b.players[pid].

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2.3: `writeLocalPlayer` — full branch ladder

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go` (replace stub with `writeLocalPlayer` method)
- Modify: `pkg/rsbuf/playerinfo_test.go` (add 4 branch tests: walk, run, tele, idle; extend-branch coverage deferred to T2.6 per renamed test's doc-comment)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/rsbuf/playerinfo_test.go`:

```go
// Helper to construct a player at given coord with all sentinels except
// what's specified. Plan-author cross-checks 41-arg count vs (*Buf).ComputePlayer.
func setupLocalPlayer(b *Buf, pid int32, modify func(p *Player)) {
	b.AddPlayer(pid)
	b.ComputePlayer(
		pid,
		3200, 0, 3200,
		3200, 3200,
		false, false,
		-1, -1,
		VisibilityDefault,
		true,
		0,
		nil,
		-1,
		-1,
		-1, -1,
		-1, -1,
		-1, -1,
		-1, -1,
		-1, -1,
		nil,
		nil, 0, 0, 0,
		-1, -1, -1,
		-1, -1,
		-1, -1,
		-1, -1, -1,
	)
	if modify != nil {
		modify(b.players[pid])
	}
}

func TestPlayerInfo_LocalPlayer_Walk(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) {
		p.WalkDir = 4 // arbitrary walk direction 0-7
	})
	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Walk-leaf: 1 (update bit) + 2 (state=1=walk) + 3 (walkDir) + 1 (extend bit) = 7 bits
	// Then 8 bits for count(0). Total 15 bits → 2 bytes.
	// First byte high bits: 1 01 100 (walkDir=4 = 100) + extend=0 = 1011000_? 
	// Plan-author manually traces: PBit(1,1) PBit(2,1) PBit(3,4) PBit(1,0) PBit(8,0)
	//   = 1 01 100 0 00000000 = 0xb0 0x00
	if len(out) != 2 {
		t.Errorf("walk: got %d bytes, want 2; bytes: %x", len(out), out)
	}
	if out[0] != 0xb0 {
		t.Errorf("walk leading byte: got 0x%02x, want 0xb0", out[0])
	}
}

func TestPlayerInfo_LocalPlayer_Run(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) {
		p.RunDir = 3
		p.WalkDir = 5
	})
	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Run-leaf: PBit(1,1) PBit(2,2) PBit(3,5) PBit(3,3) PBit(1,0) + PBit(8,0)
	//   = 1 10 101 011 0 00000000 = 1101 0101 1000 0000 = 0xd5 0x80
	// (10 bits in first 1.25 bytes + 8 bits count = 18 bits → 3 bytes).
	if len(out) != 3 {
		t.Errorf("run: got %d bytes, want 3", len(out))
	}
	if out[0] != 0xd5 {
		t.Errorf("run byte[0]: got 0x%02x, want 0xd5", out[0])
	}
}

func TestPlayerInfo_LocalPlayer_Tele(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) {
		p.Tele = true
	})
	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Tele-leaf: PBit(1,1) PBit(2,3) PBit(2,level=0) PBit(7,localX) PBit(7,localZ) PBit(1,jump=0) PBit(1,extend=0)
	// = 1+2+2+7+7+1+1 = 21 bits + 8 count = 29 bits → 4 bytes.
	// localX = x - (((originX>>3) - 6) << 3) = 3200 - (((3200>>3) - 6) << 3) = 3200 - ((400 - 6)<<3) = 3200 - 3152 = 48
	// localZ = same logic = 48
	if len(out) != 4 {
		t.Errorf("tele: got %d bytes, want 4", len(out))
	}
	// Detailed byte assertion: plan-author traces bit-by-bit and pins exact bytes.
	// See upstream info.rs:79-89 for the math.
}

// TestPlayerInfo_LocalPlayer_Idle pins the writeLocalPlayer default branch
// after the dispatch is wired in. T2.2's TestPlayerInfo_Encode_LocalIdleNoOthers
// covers the same path against the stub; this regression-locks the
// post-writeLocalPlayer behavior. Real extend-only branch coverage (the
// `case hdLen > 0:` arm) defers to T2.6, where mask-payload pinning makes
// seeded high-def state natural; reproducing it in T2.3 would force a
// renderer-internals reach-around (`r.highDef[1] = []byte{...}`).
func TestPlayerInfo_LocalPlayer_Idle(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) {
		// No movement; no masks. Renderer returns empty payload. Idle path taken.
	})
	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Idle: PBit(1,0) + PBit(8,0) = 9 bits = 2 bytes, both zero.
	if len(out) != 2 || out[0] != 0 || out[1] != 0 {
		t.Errorf("idle: got %x, want 00 00", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_LocalPlayer`
Expected: FAIL — current Encode skeleton always emits idle, so walk/run/tele tests fail on byte assertions.

- [ ] **Step 3: Implement `writeLocalPlayer`**

Replace the stub `pi.buf.PBit(1, 0)` line in the new `Encode` method with a call to a new method, and add the method below `Encode`:

```go
// In Encode, replace the stub line:
//   pi.buf.PBit(1, 0) // idle
// with:
pi.writeLocalPlayer(self, renderer)

// New method:

// writeLocalPlayer emits the local player's per-tick movement bits
// (or idle/extend), branching on tele/run/walk/masks. Mirrors upstream
// PlayerInfo::write_local_player at info.rs:72-100.
//
// Returns the high-def payload length for the local player (consumed
// by the new-players byte-budget math at info.rs:60).
func (pi *PlayerInfo) writeLocalPlayer(self *Player, renderer *Renderer) int {
	pos := coordgrid.UnpackCoord(self.Coord)
	originPos := coordgrid.UnpackCoord(self.Origin)
	highDef := renderer.HighDefOf(int(self.PID))
	hdLen := len(highDef)

	switch {
	case self.Tele:
		// Mirrors info.rs:80-89: teleport leaf with local-window coords.
		localX := pos.X - (((originPos.X >> 3) - 6) << 3)
		localZ := pos.Z - (((originPos.Z >> 3) - 6) << 3)
		jump := 0
		if self.Jump {
			jump = 1
		}
		extend := 0
		if hdLen > 0 {
			extend = 1
		}
		pi.buf.PBit(1, 1)
		pi.buf.PBit(2, 3)
		pi.buf.PBit(2, pos.Level)
		pi.buf.PBit(7, localX)
		pi.buf.PBit(7, localZ)
		pi.buf.PBit(1, jump)
		pi.buf.PBit(1, extend)
		if extend == 1 {
			for _, b := range highDef {
				pi.updates.P1(b)
			}
		}
	case self.RunDir != -1:
		// Mirrors info.rs:91 + run() at info.rs:226-243.
		extend := 0
		if hdLen > 0 {
			extend = 1
		}
		pi.buf.PBit(1, 1)
		pi.buf.PBit(2, 2)
		pi.buf.PBit(3, int(self.WalkDir))
		pi.buf.PBit(3, int(self.RunDir))
		pi.buf.PBit(1, extend)
		if extend == 1 {
			for _, b := range highDef {
				pi.updates.P1(b)
			}
		}
	case self.WalkDir != -1:
		// Mirrors info.rs:93 + walk() at info.rs:246-262.
		extend := 0
		if hdLen > 0 {
			extend = 1
		}
		pi.buf.PBit(1, 1)
		pi.buf.PBit(2, 1)
		pi.buf.PBit(3, int(self.WalkDir))
		pi.buf.PBit(1, extend)
		if extend == 1 {
			for _, b := range highDef {
				pi.updates.P1(b)
			}
		}
	case hdLen > 0:
		// Mirrors info.rs:94-95 + extend() at info.rs:265-274.
		pi.buf.PBit(1, 1)
		pi.buf.PBit(2, 0)
		for _, b := range highDef {
			pi.updates.P1(b)
		}
	default:
		// idle (info.rs:97).
		pi.buf.PBit(1, 0)
	}
	return hdLen
}
```

Add `coordgrid` import if not already present in playerinfo.go.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_LocalPlayer`
Expected: PASS (4 new tests + the idle test from T2.2).

If the byte assertions for `Tele` need tuning, plan-author traces the bit-stream manually and updates the expected bytes. Expected behavior is bit-by-bit reproduction of upstream `info.rs:80-89` math.

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-30 Bundle 2 Task 2.3 — writeLocalPlayer branch ladder

Ports PlayerInfo::write_local_player at info.rs:72-100. Branch ladder:
tele → run → walk → extend → idle. Each emits the appropriate PBit
sequence + appends high-def payload to updates buffer when extend
flag is set.

Tests pin byte-level output for walk (0xb0 0x00), run (0xd5 0x80),
tele (4 bytes — len-only check; bit-stream math traced to info.rs:80-89),
and idle (0x00 0x00 — regression-locks the default branch after
dispatch is wired). Real extend-only branch (the `case hdLen > 0:`
arm) is covered at T2.6 alongside mask-payload pinning, where seeded
high-def state arrives naturally rather than via renderer-internals
reach-around.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2.4: `writePlayers` — tracked-others delta loop

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go` (add `writePlayers` method, wire into `Encode`)
- Modify: `pkg/rsbuf/playerinfo_test.go` (add tests for 6 remove branches + 4 mode branches + visibility-soft)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/rsbuf/playerinfo_test.go`:

```go
func setupOtherPlayer(b *Buf, pid int32, modify func(p *Player)) {
	b.AddPlayer(pid)
	b.ComputePlayer(
		pid,
		3200, 0, 3200,
		3200, 3200,
		false, false,
		-1, -1,
		VisibilityDefault,
		true,
		0,
		nil,
		-1,
		-1,
		-1, -1,
		-1, -1,
		-1, -1,
		-1, -1,
		-1, -1,
		nil,
		nil, 0, 0, 0,
		-1, -1, -1,
		-1, -1,
		-1, -1,
		-1, -1, -1,
	)
	if modify != nil {
		modify(b.players[pid])
	}
}

func TestPlayerInfo_TrackedOther_Idle(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, nil)
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Local idle (1 bit) + count=1 (8 bits) + other-idle (1 bit) = 10 bits → 2 bytes.
	if len(out) != 2 {
		t.Errorf("tracked idle: got %d bytes, want 2", len(out))
	}
	// First byte: 0 (local idle) + 00000001 = 00000000 (count high 7 bits, top bit) | 1 = ?
	// Bit layout: PBit(1,0) PBit(8,1) PBit(1,0) = 0 00000001 0 = 00000000 10000000 (16 bits, last 6 unset)
	// Actually: 0 + 00000001 + 0 = 0_00000001_0 = bit-MSB packed:
	//   bit 0: 0 (local idle)
	//   bits 1-8: 00000001 (count=1)
	//   bit 9: 0 (other idle)
	// Bytes: 0_0000000 1_0XXXXXX = 0x00, 0x80
	if out[0] != 0x00 || out[1] != 0x80 {
		t.Errorf("tracked idle bytes: got %x, want 00 80", out)
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseSlotEmpty(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	// Mark slot 2 as observed but NEVER add it (slot stays nil).
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// remove leaf: PBit(1,1) PBit(2,3) = 3 bits.
	// Total: 0 (local idle) + 00000001 (count) + 1 11 = 12 bits → 2 bytes.
	// 0_0000000 1_111_XXXX = 0x00, 0xf0
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (slot empty): got %x, want 00 f0", out)
	}
	// Verify the slot was removed from build set.
	if b.players[1].Build.Players.Contains(2) {
		t.Error("slot 2 should be removed from build.Players after remove")
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseTele(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) { p.Tele = true })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Same byte layout as slot-empty remove.
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (tele): got %x, want 00 f0", out)
	}
	if b.players[1].Build.Players.Contains(2) {
		t.Error("slot 2 should be removed after tele-remove")
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseLevelMismatch(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil) // level 0
	setupOtherPlayer(b, 2, func(p *Player) {
		p.Coord = coordgrid.PackCoord(1, 3200, 3200) // level 1
	})
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (level mismatch): got %x, want 00 f0", out)
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseOutOfDistance(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		p.Coord = coordgrid.PackCoord(0, 5000, 5000) // far away
	})
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (out of distance): got %x, want 00 f0", out)
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseInactive(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) { p.Active = false })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (inactive): got %x, want 00 f0", out)
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseHardVisibility(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) { p.Visibility = VisibilityHard })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (hard visibility): got %x, want 00 f0", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_TrackedOther`
Expected: FAIL — Encode currently emits zero count, doesn't iterate Build.Players.

- [ ] **Step 3: Implement `writePlayers`**

In `Encode`, replace the stub `pi.buf.PBit(8, 0)` line with `pi.writePlayers(b, self, renderer)` and add the method:

```go
// writePlayers emits the per-tracked-other delta loop. Mirrors upstream
// PlayerInfo::write_players at info.rs:102-134. Iterates the local
// player's BuildArea.Players set and emits remove or run/walk/extend/idle
// per-other based on whether the other still passes the visibility +
// distance gates.
func (pi *PlayerInfo) writePlayers(b *Buf, self *Player, renderer *Renderer) {
	tracked := self.Build.Players.Iter()
	pi.buf.PBit(8, len(tracked))

	selfPos := coordgrid.UnpackCoord(self.Coord)
	for _, otherPid := range tracked {
		// Out-of-bounds defensive check (upstream uses Option<Player>).
		if int(otherPid) >= len(b.players) {
			pi.removeOther(self, otherPid)
			continue
		}
		other := b.players[otherPid]
		if other == nil {
			pi.removeOther(self, otherPid)
			continue
		}

		otherPos := coordgrid.UnpackCoord(other.Coord)
		// Six remove conditions (mirrors info.rs:114).
		if other.PID == -1 ||
			other.Tele ||
			otherPos.Level != selfPos.Level ||
			!withinDistanceSW(selfPos.X, selfPos.Z, otherPos.X, otherPos.Z, int(self.Build.ViewDistance)) ||
			!other.Active ||
			other.Visibility == VisibilityHard {
			pi.removeOther(self, otherPid)
			continue
		}

		highDef := renderer.HighDefOf(int(otherPid))
		hdLen := len(highDef)
		switch {
		case other.RunDir != -1:
			extend := 0
			if hdLen > 0 {
				extend = 1
			}
			pi.buf.PBit(1, 1)
			pi.buf.PBit(2, 2)
			pi.buf.PBit(3, int(other.WalkDir))
			pi.buf.PBit(3, int(other.RunDir))
			pi.buf.PBit(1, extend)
			if extend == 1 {
				for _, b2 := range highDef {
					pi.updates.P1(b2)
				}
			}
		case other.WalkDir != -1:
			extend := 0
			if hdLen > 0 {
				extend = 1
			}
			pi.buf.PBit(1, 1)
			pi.buf.PBit(2, 1)
			pi.buf.PBit(3, int(other.WalkDir))
			pi.buf.PBit(1, extend)
			if extend == 1 {
				for _, b2 := range highDef {
					pi.updates.P1(b2)
				}
			}
		case hdLen > 0:
			pi.buf.PBit(1, 1)
			pi.buf.PBit(2, 0)
			for _, b2 := range highDef {
				pi.updates.P1(b2)
			}
		default:
			pi.buf.PBit(1, 0)
		}
	}
}

// removeOther emits the 3-bit remove leaf and updates build set.
// Mirrors PlayerInfo::remove at info.rs:189-197.
func (pi *PlayerInfo) removeOther(self *Player, otherPid int32) {
	pi.buf.PBit(1, 1)
	pi.buf.PBit(2, 3)
	self.Build.Players.Remove(otherPid)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_TrackedOther`
Expected: PASS (7 tests).

Full pkg/rsbuf: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/...`

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-30 Bundle 2 Task 2.4 — writePlayers tracked-other delta loop

Ports PlayerInfo::write_players + PlayerInfo::remove at info.rs:102-134,
189-197. Iterates self.Build.Players.Iter(), emits remove or
run/walk/extend/idle per other based on the 6 reject conditions
(pid==-1, tele, level mismatch, out-of-distance, !active, HARD vis).

Tests pin all 6 remove branches individually (each gets its own
test case + byte assertion); plus tracked-idle baseline. Each remove
also verifies the build.Players bitset removal as a side-effect.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2.5: `writeNewPlayers` — discovery + add path

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go` (add `writeNewPlayers` method, wire into `Encode`)
- Modify: `pkg/rsbuf/playerinfo_test.go` (add discovery + add-branch tests)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/rsbuf/playerinfo_test.go`:

```go
func TestPlayerInfo_NewPlayers_DiscoversAndAdds(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		// New player at adjacent zone — passes filterPlayer.
	})

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Local idle (1) + count=0 (8) + add-leaf (23 bits: 11 pid + 5 dx + 5 dz + 1 jump + 1 extend)
	// + sentinel before updates (11 bits = 2047) = 1 + 8 + 23 + 11 = 43 bits → 6 bytes.
	// Plus the renderer's low-def payload appended after AccessBytes.
	// For NAI-30 the renderer returns empty payloads when masks=0, so updates may be empty.
	// Without appearance regen the renderer's LowDefFullOf may return empty bytes — verify
	// per the existing renderer test fixtures.

	// Verify add happened: build set now contains pid 2.
	if !b.players[1].Build.Players.Contains(2) {
		t.Errorf("after Encode, build.Players should contain 2; bytes %x", out)
	}
}

func TestPlayerInfo_NewPlayers_RespectsPreferredCap(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	// Pre-populate Build.Players to preferredPlayers (250) so cap is hit immediately.
	for i := int32(2); i < int32(2+preferredPlayers); i++ {
		b.players[1].Build.Players.Insert(i)
	}
	// Add one more nearby player that would otherwise discover.
	setupOtherPlayer(b, 1000, nil)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	_ = out

	// Pid 1000 should NOT be added (cap blocks).
	if b.players[1].Build.Players.Contains(1000) {
		t.Error("preferred cap exceeded; pid 1000 should not have been added")
	}
}

func TestPlayerInfo_NewPlayers_SkipsHardVisibility(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) { p.Visibility = VisibilityHard })

	pi := NewPlayerInfo()
	r := NewRenderer()
	_ = pi.Encode(b, 1, r)

	if b.players[1].Build.Players.Contains(2) {
		t.Error("HARD visibility excluded; pid 2 should not have been added")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_NewPlayers`
Expected: FAIL — Encode currently doesn't call writeNewPlayers.

- [ ] **Step 3: Implement `writeNewPlayers` + `addOther`**

In `Encode`, after `pi.writePlayers(...)`, add `pi.writeNewPlayers(b, self, renderer)`. Add the method:

```go
// writeNewPlayers discovers nearby players and emits add-leaves until
// the byte budget or preferredPlayers cap is hit. Mirrors upstream
// PlayerInfo::write_new_players at info.rs:136-166.
func (pi *PlayerInfo) writeNewPlayers(b *Buf, self *Player, renderer *Renderer) {
	selfPos := coordgrid.UnpackCoord(self.Coord)
	candidates := self.Build.GetNearbyPlayers(&b.players, b.zoneMap, self.PID, selfPos.X, selfPos.Level, selfPos.Z)

	for _, otherPid := range candidates {
		if self.Build.Players.Contains(otherPid) {
			continue
		}
		if self.Build.Players.Len() >= int(preferredPlayers) {
			return
		}
		other := b.players[otherPid]
		if other == nil || other.Visibility == VisibilityHard {
			continue
		}

		// Byte budget: BITS_ADD + low-def payload size.
		lowDef := renderer.LowDefFullOf(int(otherPid))
		if !pi.fits(playerBitsAdd, len(lowDef)) {
			return
		}

		otherPos := coordgrid.UnpackCoord(other.Coord)
		dx := clampInt(otherPos.X-selfPos.X, -15, 15)
		dz := clampInt(otherPos.Z-selfPos.Z, -15, 15)
		jump := 0
		if other.Jump {
			jump = 1
		}

		pi.buf.PBit(11, int(otherPid))
		pi.buf.PBit(5, dx&0x1f)
		pi.buf.PBit(5, dz&0x1f)
		pi.buf.PBit(1, jump)
		pi.buf.PBit(1, 1) // extend bit always set for add

		self.Build.Players.Insert(otherPid)

		// Choose low-def variant per appearance dedup.
		// Mirrors info.rs:296-310: if other.lastAppearance != -1 AND
		// build's stored tick != lastAppearance, send LowDefFullOf
		// (includes APPEARANCE block) and save tick.
		if other.LastAppearance != -1 && !self.Build.HasAppearance(otherPid, uint32(other.LastAppearance)) {
			self.Build.SaveAppearance(otherPid, uint32(other.LastAppearance))
			for _, b2 := range lowDef {
				pi.updates.P1(b2)
			}
		} else {
			noApp := renderer.LowDefNoAppOf(int(otherPid))
			for _, b2 := range noApp {
				pi.updates.P1(b2)
			}
		}
	}
}

// fits reports whether adding bitsToAdd + bytesToAdd will fit within
// maxPlayerInfoBytes. Mirrors info.rs:404-408.
func (pi *PlayerInfo) fits(bitsToAdd, bytesToAdd int) bool {
	totalBits := pi.buf.BitPos + bitsToAdd + 7
	totalBytes := (totalBits >> 3) + len(pi.updates.Data) + bytesToAdd
	return totalBytes <= maxPlayerInfoBytes
}

// clampInt clamps v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_NewPlayers`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-30 Bundle 2 Task 2.5 — writeNewPlayers discovery + add

Ports PlayerInfo::write_new_players at info.rs:136-166. Calls
self.Build.GetNearbyPlayers (Bundle 1 Task 1.1) for spatial
discovery, then emits add-leaves until byte budget (fits()) or
preferredPlayers cap is reached.

Add-leaf chooses LowDefFullOf vs LowDefNoAppOf based on the
upstream-equivalent appearance dedup at info.rs:296-310:
  - If other.LastAppearance != -1 AND build.appearances[other_pid]
    != lastAppearance: send full (includes APPEARANCE), save tick.
  - Else: send no-app variant.

Tests pin: discovery happens (build set updated); preferredPlayers
cap respected (extra pid not added); HARD visibility skipped.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2.6: lastAppearance dedup branch coverage

**Files:**
- Modify: `pkg/rsbuf/playerinfo_test.go` (add the 3 lastAppearance scenarios)

The `writeNewPlayers` implementation in T2.5 already handles the dedup logic; this task adds focused tests to pin each lastAppearance branch separately. Aligns with the spec's "lastAppearance semantic correction" being the central NAI-30 change worth heavy test pinning.

- [ ] **Step 1: Write the tests**

Append to `pkg/rsbuf/playerinfo_test.go`:

```go
func TestPlayerInfo_LastAppearance_FreshGuardSkipsAppearance(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		p.LastAppearance = -1 // never generated — encoder should skip APPEARANCE
	})

	pi := NewPlayerInfo()
	r := NewRenderer()
	_ = pi.Encode(b, 1, r)

	// Build's appearances should NOT have been updated (lastAppearance==-1 path).
	if b.players[1].Build.HasAppearance(2, 0) {
		// HasAppearance(pid, tick) returns true iff stored == tick;
		// stored is 0 by default, so this is the "never set" case.
		// We're checking the encoder didn't accidentally call SaveAppearance(2, 0).
		// Actually for tick=0 the comparison is (0 == 0) = true on a fresh BuildArea,
		// per NAI-29 T3.1 finding (BuildArea_HasAppearance_FreshIsFalse). That's expected.
	}
	// More direct: pid 2 was added to build set, but no SaveAppearance call
	// should have triggered for non-zero tick. Negative pin.
	for tick := uint32(1); tick <= 100; tick++ {
		if b.players[1].Build.HasAppearance(2, tick) {
			t.Errorf("lastAppearance=-1: build should not have saved tick %d", tick)
		}
	}
}

func TestPlayerInfo_LastAppearance_BuildSavesOnFirstSend(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		p.LastAppearance = 42
	})

	pi := NewPlayerInfo()
	r := NewRenderer()
	_ = pi.Encode(b, 1, r)

	// First send — build stores tick 42.
	if !b.players[1].Build.HasAppearance(2, 42) {
		t.Error("after first encode with lastAppearance=42, build should have saved tick 42")
	}

	// Second send same tick — no resend (not directly testable without spying on renderer,
	// but the build state should still equal 42).
	_ = pi.Encode(b, 1, r)
	if !b.players[1].Build.HasAppearance(2, 42) {
		t.Error("after second encode same tick, build should still equal 42")
	}
}

func TestPlayerInfo_LastAppearance_BuildResendsOnTickChange(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		p.LastAppearance = 42
	})

	pi := NewPlayerInfo()
	r := NewRenderer()
	_ = pi.Encode(b, 1, r) // saves tick 42

	// Bump lastAppearance — equipment changed.
	b.players[2].LastAppearance = 43

	// Re-encode (with pid 2 already in build set — but the dedup happens in
	// writeNewPlayers which only handles new adds. For tracked others, the
	// equivalent dedup happens in writePlayers' lowdefinition path when
	// extending; in NAI-30 the existing renderer regenerates payloads, so
	// the test just verifies the field state, not the wire bytes per se).

	// To re-trigger discovery, we need pid 2 NOT to be in the tracked set.
	// Remove and re-add.
	b.players[1].Build.Players.Remove(2)
	_ = pi.Encode(b, 1, r)
	if !b.players[1].Build.HasAppearance(2, 43) {
		t.Error("after re-discovery with lastAppearance=43, build should have saved tick 43")
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

These tests should pass against the T2.5 implementation as-is — T2.5 already covers the dedup logic. The purpose of T2.6 is targeted pinning.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_LastAppearance`
Expected: PASS (3 tests).

If any fail, the dedup logic in T2.5 has a bug — fix in `writeNewPlayers` and re-run.

- [ ] **Step 3: (No code changes if tests pass; this is a coverage-only task)**

If tests passed, skip to Step 4.

- [ ] **Step 4: Run full pkg/rsbuf to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/...`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/playerinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(rsbuf): NAI-30 Bundle 2 Task 2.6 — lastAppearance dedup branch coverage

Pins the three lastAppearance dedup scenarios in PlayerInfo encoder:
  - lastAppearance=-1: encoder skips APPEARANCE entirely (never-generated guard)
  - First send with lastAppearance=N: build saves tick N
  - Second send after lastAppearance bumps to N+1: build resends + saves N+1

Test-only commit; no production code changes (T2.5 already
implements the logic, this is targeted coverage).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2.7: Visibility-soft + staffMod gate

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go` (add staffMod check in writePlayers + writeNewPlayers)
- Modify: `pkg/rsbuf/playerinfo_test.go` (add tests)

The OLD encoder in `playerinfo.go:117-128` had the visibility-soft + staffMod gate (`other.Visibility() == VisibilitySoft && self.StaffModLevel() < 1`). Upstream Rust's `info.rs:114` only mentions `Visibility::HARD` — but goscape's existing tests pin the SOFT path. NAI-30 preserves this behavior; if upstream omits it, that's an existing goscape divergence (already merged at NAI-9). Re-grep + verify.

- [ ] **Step 1: Verify the existing behavior**

Run: `rg "VisibilitySoft\|StaffModLevel" pkg/rsbuf/`
Expected: matches in `source.go` (interface), `playerinfo.go` (legacy), and tests.

If `Player` (`pkg/rsbuf/player.go`) doesn't have a StaffModLevel field, but `PlayerSource` interface does — we need to add it to `Player` for the new encoder to read directly. Plan-author checks current state.

- [ ] **Step 2: Add `StaffModLevel int32` to Player + tick.go arg**

If `pkg/rsbuf/player.go` Player struct lacks StaffModLevel, this is a Bundle 1 oversight that lands here. Add to struct (default 0), to `newPlayer`, to `(*Buf).ComputePlayer` signature (after visibility), and to `tick.go:369` arg.

OR if the field already exists, skip to Step 3.

If adding: this is a 42-arg ComputePlayer call (was 41) — pre-flight check. Plan-author re-verifies via reading `pkg/rsbuf/buf.go` + `tick.go:364-386` before dispatch.

(Sub-step deliberately conditional — if NAI-29 ComputePlayer already includes staffModLevel as one of its 41 args, no schema change needed.)

- [ ] **Step 3: Add the gate in `writePlayers` + `writeNewPlayers`**

In `writePlayers`, extend the 6-condition reject block to add a 7th:

```go
// 7th reject: SOFT visibility + insufficient staff mod level
// (matches goscape's NAI-9 behavior; upstream info.rs only checks HARD).
if other.Visibility == VisibilitySoft && self.StaffModLevel < 1 {
    pi.removeOther(self, otherPid)
    continue
}
```

In `writeNewPlayers`, after the HARD check:

```go
if other.Visibility == VisibilitySoft && self.StaffModLevel < 1 {
    continue
}
```

- [ ] **Step 4: Add tests + run**

```go
func TestPlayerInfo_TrackedOther_RemoveBecauseSoftVisAndLowStaff(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil) // StaffModLevel default 0
	setupOtherPlayer(b, 2, func(p *Player) { p.Visibility = VisibilitySoft })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (soft + low staff): got %x, want 00 f0", out)
	}
}

func TestPlayerInfo_TrackedOther_KeepsSoftVisWithStaffMod(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) { p.StaffModLevel = 1 })
	setupOtherPlayer(b, 2, func(p *Player) { p.Visibility = VisibilitySoft })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	// Should NOT be remove (0xf0 prefix on byte 1) — should be idle (0x80).
	if out[1] == 0xf0 {
		t.Errorf("remove triggered when staff mod >= 1: got %x", out)
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_TrackedOther_(Soft|KeepsSoft)`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go pkg/rsbuf/player.go pkg/rsbuf/buf.go modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-30 Bundle 2 Task 2.7 — visibility-soft + staffMod gate

Preserves goscape's NAI-9 visibility-soft behavior in the new
PlayerInfo encoder: VisibilitySoft + self.StaffModLevel < 1 →
remove (or skip add). Upstream Rust info.rs only checks
Visibility::HARD; this is a goscape divergence dating from NAI-9
that the new encoder must also respect to keep the wire-bytes
behaviorally equivalent.

(If StaffModLevel field was added to *rsbuf.Player as part of this
task: includes the field add + tick.go arg-position adjustment.
Otherwise no schema change.)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2.8: Local-player CHAT mask suppression

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go` (suppress CHAT mask in local player's high-def payload)
- Modify: `pkg/rsbuf/playerinfo_test.go` (add test)

Mirrors upstream `info.rs:289-291` — local player's own chat is suppressed (no self-echo).

- [ ] **Step 1: Write the test**

```go
func TestPlayerInfo_LocalPlayer_ChatMaskStripped(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) {
		p.Masks = uint32(MaskChat) // chat mask set
		p.Chat = &Chat{Bytes: []byte("hello"), Color: 0, Effect: 0, Ignored: 0}
	})

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	_ = out
	// Verification approach: the renderer's HighDefOf for the local player
	// should NOT include CHAT mask bits in its written bytes. Without
	// renderer-cache surgery, the simplest pin is: invoke a Renderer that
	// pretends to be aware, OR check that the encoder masks-out CHAT before
	// calling the renderer's eager-payload accessors.
	// Plan-author: chooses between a renderer-stub test or a direct
	// "pi.suppressLocalChat()" inspection helper.
	t.Skip("Requires renderer-aware test or NAI-31 cache port; pinned via integration test in B4")
}
```

For NAI-30 the existing goscape Renderer's `HighDefOf(slot)` returns pre-computed payloads that include all set masks; we don't have a way to selectively suppress one mask without re-rendering. **Decision:** defer the local-CHAT suppression to NAI-31 when the renderer becomes per-mask cached, and document this as part of NAI-30-D2 (or fold into NAI-30-D1). Mark the test as `t.Skip` with a TODO referencing NAI-31.

Alternative: in the new encoder, pass a flag to renderer (`HighDefOfLocal(slot, suppressChat bool)`) — but that's a renderer API change which is NAI-31 scope.

**Plan-author decision:** mark this task as deferred to NAI-31 and skip the test for now. Update the spec's deviation tag list at NAI-30 close: add NAI-30-D2 ("local-player CHAT suppression deferred to NAI-31 renderer port"). Recommend the implementer take the simpler path.

- [ ] **Step 2: (Skipped — see Step 1 decision)**

- [ ] **Step 3: Document the deferral**

In `pkg/rsbuf/playerinfo.go`, add an inline comment in `writeLocalPlayer` near the high-def emission:

```go
// NAI-30-D2: upstream PlayerInfo::highdefinition at info.rs:289-291
// strips CHAT mask bit for self (no chat self-echo). Goscape's
// existing eager renderer doesn't expose per-mask suppression;
// CHAT suppression is deferred to NAI-31 renderer port. Until
// then, the local player's own chat may echo back to its own
// client — visible behavior may differ from upstream by one
// chat block per say.
for _, b := range highDef {
    pi.updates.P1(b)
}
```

- [ ] **Step 4: Build + test sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && go test -count=1 ./pkg/rsbuf/...`
Expected: PASS (the skip test is a documented deferral, not a failure).

- [ ] **Step 5: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(rsbuf): NAI-30 Bundle 2 Task 2.8 — defer local-CHAT suppression to NAI-31

Upstream PlayerInfo::highdefinition at info.rs:289-291 strips the
CHAT mask bit for self (no chat self-echo). Goscape's existing
eager renderer doesn't expose per-mask suppression; this requires
NAI-31's renderer port to land first. Documented inline as
NAI-30-D2; placeholder test marked t.Skip with TODO referencing
NAI-31.

Visible behavior delta: local player's own chat may echo back to
its own client by one chat block per say — minor, easily fixed
when NAI-31 lands.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2.9: Output-bytes-are-copy invariant

**Files:**
- Modify: `pkg/rsbuf/playerinfo_test.go` (add invariant test)

T2.2 already implements `Encode` returning a copy (`out := make([]byte, ...)`), but a regression test pins this invariant.

- [ ] **Step 1: Write the test**

```go
func TestPlayerInfo_Encode_OutputBytesAreCopy(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	pi := NewPlayerInfo()
	r := NewRenderer()

	out1 := pi.Encode(b, 1, r)
	out1Saved := append([]byte(nil), out1...)

	// Mutate state and re-encode.
	b.players[1].WalkDir = 3
	out2 := pi.Encode(b, 1, r)

	// out1 must be unchanged after the second Encode mutated pi.buf.Data.
	if !bytes.Equal(out1, out1Saved) {
		t.Errorf("out1 mutated after second Encode: got %x, want %x", out1, out1Saved)
	}
	// out2 should differ from out1 (different bytes).
	_ = out2
}
```

Add `import "bytes"` if not present.

- [ ] **Step 2: Run test to verify it passes (already implemented in T2.2)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/rsbuf/ -run TestPlayerInfo_Encode_OutputBytesAreCopy`
Expected: PASS.

If FAIL, check that T2.2's `Encode` ends with `out := make([]byte, len(pi.buf.Data)); copy(out, pi.buf.Data); return out` — the make+copy is the regression-safe pattern.

- [ ] **Step 3: (No code change if T2.2 implementation correct)**

- [ ] **Step 4: Full bundle test sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
Expected: all PASS.

- [ ] **Step 5: Commit + Bundle 2 close**

```bash
git add pkg/rsbuf/playerinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(rsbuf): NAI-30 Bundle 2 Task 2.9 — Encode output-bytes-are-copy invariant

Pins that PlayerInfo.Encode returns a fresh []byte copy each call —
mutating pi.buf.Data on subsequent Encode calls must not corrupt
prior return values. Implementation already correct from T2.2
(make+copy pattern); this test prevents regression.

Bundle 2 close: PlayerInfo struct + Encode method fully landed,
mirroring info.rs:13-409. EncodeLegacy still present; B4 deletes
it after caller swap.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Bundle 3 — NpcInfo encoder port

Bundle 3 mirrors B2's structure for `NpcInfo`. Source: `info.rs:411-708`. Key differences from PlayerInfo: BITS_ADD = 35 (vs 23); 8191 NPC_TERMINATOR (vs 2047); no exact-move; no chat; observers counter incremented on add / decremented on remove (replaces the package-level `npc_observers.go` shim).

The structural and test patterns mirror B2 exactly — the implementer can use B2 as a template and adjust for NPC-specific opcodes/branches. Detailed tasks (T3.1-T3.7) follow the same checkbox/commit cadence.

## Task 3.1: Rename old `EncodeNpc` → `EncodeNpcLegacy`

**Files:**
- Modify: `pkg/rsbuf/npcinfo.go` (rename)
- Modify: `modules/world/player_npc_info.go` (callsite swap)
- Modify: `pkg/rsbuf/npcinfo_test.go` (sweep `EncodeNpc` → `EncodeNpcLegacy`)

Same pattern as T2.1. Verify with `rg "rsbuf\.EncodeNpc\b\(" pkg/ modules/ cmd/` first; expect 1 production caller + test sites.

- [ ] **Step 1**: Verify caller count.
- [ ] **Step 2**: Rename `EncodeNpc` → `EncodeNpcLegacy` in `pkg/rsbuf/npcinfo.go`. Update doc-comment to match T2.1's transition note.
- [ ] **Step 3**: Update `modules/world/player_npc_info.go:26` callsite. Sweep test file.
- [ ] **Step 4**: Build + test sweep.
- [ ] **Step 5**: Commit with `refactor(rsbuf): NAI-30 Bundle 3 Task 3.1 — rename EncodeNpc → EncodeNpcLegacy`.

## Task 3.2: `NpcInfo` struct + `Encode` skeleton

**Files:**
- Modify: `pkg/rsbuf/npcinfo.go` (add struct + `Encode` skeleton)
- Modify: `pkg/rsbuf/npcinfo_test.go` (add idle test)

Same pattern as T2.2. Constants:

```go
const (
	npcBitsAdd      = 13 + 11 + 5 + 5 + 1 // 35
	npcBitsRun      = 1 + 2 + 3 + 3 + 1   // 10
	npcBitsWalk     = 1 + 2 + 3 + 1       // 7
	npcBitsExtend   = 1 + 2               // 3
	npcTerminator   = 8191
	maxNpcInfoBytes = 4997
)
```

Skeleton encode emits empty count (0 NPCs tracked) and no terminator (no overflow). Test pins 1-byte output (8 bits = 1 byte aligned).

- [ ] **Step 1**: Test for empty/idle NpcInfo encode (no tracked, no nearby).
- [ ] **Step 2**: Verify FAIL.
- [ ] **Step 3**: Implement `NpcInfo` struct + `NewNpcInfo` + `Encode` skeleton (mirrors info.rs:430-464).
- [ ] **Step 4**: Verify PASS.
- [ ] **Step 5**: Commit.

## Task 3.3: `writeNpcs` — tracked-NPC delta loop

**Files:**
- Modify: `pkg/rsbuf/npcinfo.go` (add `writeNpcs` method)
- Modify: `pkg/rsbuf/npcinfo_test.go` (5 remove branch tests + 4 mode branches)

Same pattern as T2.4. Five remove conditions per upstream `info.rs:478` — `nid==-1`, `tele`, level mismatch, out-of-distance, `!active`. Each remove decrements `b.npcs[nid].Observers` (floored at 0) per `info.rs:480`.

```go
func (ni *NpcInfo) writeNpcs(b *Buf, self *Player, renderer *Renderer) {
	tracked := self.Build.Npcs.Iter()
	ni.buf.PBit(8, len(tracked))
	selfPos := coordgrid.UnpackCoord(self.Coord)
	for _, nid := range tracked {
		if int(nid) >= len(b.npcs) || b.npcs[nid] == nil {
			ni.removeNpc(self, nid)
			ni.decObservers(b, nid)
			continue
		}
		other := b.npcs[nid]
		otherPos := coordgrid.UnpackCoord(other.Coord)
		if other.NID == -1 || other.Tele || otherPos.Level != selfPos.Level ||
			!withinDistanceSW(selfPos.X, selfPos.Z, otherPos.X, otherPos.Z, int(preferredViewDistance)) ||
			!other.Active {
			ni.removeNpc(self, nid)
			ni.decObservers(b, nid)
			continue
		}
		highDef := renderer.NpcHighDefOf(int(nid))
		hdLen := len(highDef)
		switch {
		case other.RunDir != -1:
			extend := 0
			if hdLen > 0 {
				extend = 1
			}
			ni.buf.PBit(1, 1)
			ni.buf.PBit(2, 2)
			ni.buf.PBit(3, int(other.WalkDir))
			ni.buf.PBit(3, int(other.RunDir))
			ni.buf.PBit(1, extend)
			if extend == 1 {
				for _, b2 := range highDef {
					ni.updates.P1(b2)
				}
			}
		case other.WalkDir != -1:
			extend := 0
			if hdLen > 0 {
				extend = 1
			}
			ni.buf.PBit(1, 1)
			ni.buf.PBit(2, 1)
			ni.buf.PBit(3, int(other.WalkDir))
			ni.buf.PBit(1, extend)
			if extend == 1 {
				for _, b2 := range highDef {
					ni.updates.P1(b2)
				}
			}
		case hdLen > 0:
			ni.buf.PBit(1, 1)
			ni.buf.PBit(2, 0)
			for _, b2 := range highDef {
				ni.updates.P1(b2)
			}
		default:
			ni.buf.PBit(1, 0)
		}
	}
}

func (ni *NpcInfo) removeNpc(self *Player, nid int32) {
	ni.buf.PBit(1, 1)
	ni.buf.PBit(2, 3)
	self.Build.Npcs.Remove(nid)
}

// decObservers decrements b.npcs[nid].Observers, flooring at 0.
// Mirrors info.rs:480 `other.observers = (other.observers - 1).max(0)`.
func (ni *NpcInfo) decObservers(b *Buf, nid int32) {
	if int(nid) >= len(b.npcs) || b.npcs[nid] == nil {
		return
	}
	if b.npcs[nid].Observers > 0 {
		b.npcs[nid].Observers--
	}
}
```

Tests pin all 5 remove branches + observer decrement per remove + 4 mode branches.

- [ ] Steps 1-5 follow B2 T2.4 cadence; commit `feat(rsbuf): NAI-30 Bundle 3 Task 3.3 — writeNpcs tracked-NPC delta loop`.

## Task 3.4: `writeNewNpcs` — discovery + add + observers increment + 8191 terminator

**Files:**
- Modify: `pkg/rsbuf/npcinfo.go` (add `writeNewNpcs` method)
- Modify: `pkg/rsbuf/npcinfo_test.go` (add discovery, cap, terminator tests)

```go
func (ni *NpcInfo) writeNewNpcs(b *Buf, self *Player, renderer *Renderer) {
	selfPos := coordgrid.UnpackCoord(self.Coord)
	candidates := self.Build.GetNearbyNpcs(&b.npcs, b.zoneMap, selfPos.X, selfPos.Level, selfPos.Z)

	for _, nid := range candidates {
		if self.Build.Npcs.Contains(nid) {
			continue
		}
		if self.Build.Npcs.Len() >= int(preferredNpcs) {
			return
		}
		other := b.npcs[nid]
		if other == nil || !other.Active {
			continue
		}

		lowDef := renderer.NpcLowDefOf(int(nid))
		if !ni.fits(npcBitsAdd, len(lowDef)) {
			// Byte budget overflow — emit terminator and return.
			ni.buf.PBit(13, npcTerminator)
			return
		}

		otherPos := coordgrid.UnpackCoord(other.Coord)
		dx := clampInt(otherPos.X-selfPos.X, -15, 15)
		dz := clampInt(otherPos.Z-selfPos.Z, -15, 15)

		ni.buf.PBit(13, int(nid))
		ni.buf.PBit(11, int(other.NType))
		ni.buf.PBit(5, dx&0x1f)
		ni.buf.PBit(5, dz&0x1f)
		ni.buf.PBit(1, 1) // extend always set for add

		self.Build.Npcs.Insert(nid)
		other.Observers++

		for _, b2 := range lowDef {
			ni.updates.P1(b2)
		}
	}
}

func (ni *NpcInfo) fits(bitsToAdd, bytesToAdd int) bool {
	totalBits := ni.buf.BitPos + bitsToAdd + 7
	totalBytes := (totalBits >> 3) + len(ni.updates.Data) + bytesToAdd
	return totalBytes <= maxNpcInfoBytes
}
```

Tests:
- `TestNpcInfo_NewNpcs_DiscoversAndAdds` — pin add happens + observer count increments to 1
- `TestNpcInfo_NewNpcs_RespectsPreferredCap` — 256 candidates, only 255 added
- `TestNpcInfo_NewNpcs_ByteBudgetOverflow_EmitsTerminator` — pre-fill ni.buf or ni.updates near maxNpcInfoBytes; assert next add emits 8191 terminator
- `TestNpcInfo_ObserverCountFloorsAtZero` — set Observers=0; trigger remove; assert it doesn't go negative

Steps 1-5 follow T2.5 cadence. Commit `feat(rsbuf): NAI-30 Bundle 3 Task 3.4 — writeNewNpcs discovery + observers + 8191 terminator`.

## Task 3.5: Output-bytes-are-copy invariant + skeleton wiring

Wire `writeNpcs` + `writeNewNpcs` into `Encode`. Add `TestNpcInfo_Encode_OutputBytesAreCopy` mirroring T2.9. Steps 1-5 follow T2.9 cadence. Commit `test(rsbuf): NAI-30 Bundle 3 Task 3.5 — NpcInfo wire + output-bytes-are-copy`.

## Task 3.6: Faceentity + Facecoord + orientation fallback (NPC side)

NpcInfo lowdefinition (info.rs:625-665) caches faceEntity/faceCoord/orientation. For NAI-30 with the eager renderer, this is largely the renderer's job — the test pins that the encoder doesn't OVERWRITE faceCoord with stale data when face_x is -1 and orientation_x is set. Mirrors info.rs:642-664 fallback ladder.

For NAI-30 without the renderer port, this is mostly a coverage placeholder. Plan-author writes 3 tests pinning the field-state expectations on `b.npcs[nid]` after Encode runs (e.g., FaceX/FaceZ unchanged across Encode if no producer set them). 

Steps 1-5 follow T2.6 cadence. Commit `test(rsbuf): NAI-30 Bundle 3 Task 3.6 — NpcInfo facecoord/orientation fallback coverage`.

## Task 3.7: Bundle 3 close — full pkg/rsbuf race test

- [ ] **Step 1**: Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./pkg/rsbuf/...`
- [ ] **Step 2**: Verify all PASS.
- [ ] **Step 3**: Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
- [ ] **Step 4**: Verify clean.
- [ ] **Step 5**: Empty commit (or amend prior) noting Bundle 3 close: `chore(rsbuf): NAI-30 Bundle 3 close — NpcInfo struct + Encode method landed; legacy still present`.

---

# Bundle 4 — Cutover + retirements

Bundle 4 wires `*Buf.PlayerInfo` + `*Buf.NpcInfo` fields, swaps modules/world callers, migrates the npc_observers.go shim consumers, flattens pkg/buildarea onto Player, and verify-grep-and-deletes dead packages/files. Each task gates on the prior.

## Task 4.1: `*Buf.PlayerInfo` + `*Buf.NpcInfo` fields + `New()` initialization

**Files:**
- Modify: `pkg/rsbuf/buf.go` (extend `Buf` struct + `New()`)
- Modify: `pkg/rsbuf/buf_test.go` (extend `TestNew_*` to assert non-nil PlayerInfo + NpcInfo)

- [ ] **Step 1**: Test for `b.PlayerInfo != nil && b.NpcInfo != nil` after `New()`.
- [ ] **Step 2**: Verify FAIL (`b.PlayerInfo undefined`).
- [ ] **Step 3**: In `pkg/rsbuf/buf.go`, extend `Buf` struct:

```go
type Buf struct {
	players    [2048]*Player
	npcs       [8192]*Npc
	zoneMap    *zoneMap
	playerGrid map[uint32][]int32
	PlayerInfo *PlayerInfo
	NpcInfo    *NpcInfo
}
```

And `New()`:

```go
func New() *Buf {
	return &Buf{
		zoneMap:    newZoneMap(),
		playerGrid: map[uint32][]int32{},
		PlayerInfo: NewPlayerInfo(),
		NpcInfo:    NewNpcInfo(),
	}
}
```

- [ ] **Step 4**: Verify PASS.
- [ ] **Step 5**: Commit `feat(rsbuf): NAI-30 Bundle 4 Task 4.1 — *Buf.PlayerInfo + *Buf.NpcInfo fields`.

## Task 4.2: Swap `modules/world/player_info.go` caller

**Files:**
- Modify: `modules/world/player_info.go`

- [ ] **Step 1**: Read current file (player_info.go is 28 lines).
- [ ] **Step 2**: Replace `payload := rsbuf.EncodeLegacy(p, sources, p.buildArea, s.grid, s.renderer)` with:

```go
if s.rsbuf == nil {
    return
}
payload := s.rsbuf.PlayerInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)
```

Update the nil-guard early-return: drop `p.buildArea == nil` and `s.grid == nil` checks; add `s.rsbuf == nil`. Delete the `sources := make([]rsbuf.PlayerSource, len(snapshot))` build-up loop and the `playersMu` snapshot copy (no longer needed — encoder reads from `s.rsbuf` directly).

Final file should be ~12 lines:

```go
package world

import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// updatePlayers runs during processClientsOut. Calls
// s.rsbuf.PlayerInfo.Encode for the local player's PlayerInfo
// payload, writes the result as an OpPlayerInfo packet.
func (p *Player) updatePlayers() {
	s := p.client.server
	if s == nil || s.rsbuf == nil || s.renderer == nil {
		return
	}
	payload := s.rsbuf.PlayerInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)
	p.writeOut(gameserver.OpPlayerInfo, payload)
}
```

- [ ] **Step 3**: Build + test sweep: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && go test -count=1 ./modules/world/...`
- [ ] **Step 4**: Verify PASS — including any integration tests like `rsbuf_per_tick_test.go`.
- [ ] **Step 5**: Commit `feat(world): NAI-30 Bundle 4 Task 4.2 — swap updatePlayers to s.rsbuf.PlayerInfo.Encode`.

## Task 4.3: Swap `modules/world/player_npc_info.go` caller

Same pattern as T4.2 but for NpcInfo:

- [ ] Steps 1-5: Replace `rsbuf.EncodeNpcLegacy(...)` with `s.rsbuf.NpcInfo.Encode(s.rsbuf, int32(p.slot), s.renderer)`. Drop sources build-up loop. Final file ~12 lines.
- Commit: `feat(world): NAI-30 Bundle 4 Task 4.3 — swap updateNpcs to s.rsbuf.NpcInfo.Encode`.

## Task 4.4: Migrate npc_observers.go shim consumers

**Files:**
- Modify: `pkg/rsbuf/buf.go` (add `(b *Buf) SetObserverForTest` method)
- Modify: `modules/world/npc_hunt.go:41` (swap to `s.rsbuf.GetNpcObservers`)
- Modify: `modules/world/npc_event_queue_test.go` (sweep `rsbuf.SetObserverForTest` → `s.rsbuf.SetObserverForTest`)

- [ ] **Step 1**: Add `(b *Buf) SetObserverForTest` method to `pkg/rsbuf/buf.go`:

```go
// SetObserverForTest writes the observer count for nid directly,
// flooring at 0. Test-only; mirrors the contract of the
// retired-in-this-bundle package-level rsbuf.SetObserverForTest
// shim. No-op if nid is out of bounds or slot is unpopulated.
func (b *Buf) SetObserverForTest(nid, count int32) {
	if nid < 0 || int(nid) >= len(b.npcs) {
		return
	}
	if b.npcs[nid] == nil {
		return
	}
	if count < 0 {
		count = 0
	}
	b.npcs[nid].Observers = count
}
```

- [ ] **Step 2**: Update `modules/world/npc_hunt.go:41`:

```go
// Was: observers := rsbuf.GetNpcObservers(n.nid)
var observers int32
if s.rsbuf != nil {
    observers = s.rsbuf.GetNpcObservers(int32(n.nid))
}
```

(Adjust the type if `observers` is used as an `int` later in the function — apply `int(observers)` or change downstream `<= 0` comparison if needed.)

- [ ] **Step 3**: Sweep `modules/world/npc_event_queue_test.go`. Each call site:

```
rsbuf.SetObserverForTest(n.nid, 0) → s.rsbuf.SetObserverForTest(int32(n.nid), 0)
rsbuf.SetObserverForTest(101, 1)   → s.rsbuf.SetObserverForTest(101, 1)
```

Plan-author re-greps `rsbuf\.SetObserverForTest` post-sweep — must return 0 matches.

- [ ] **Step 4**: Build + test sweep.
- [ ] **Step 5**: Commit `feat(rsbuf,world): NAI-30 Bundle 4 Task 4.4 — migrate npc_observers shim consumers`.

## Task 4.5: Flatten `pkg/buildarea` scenery fields onto `Player`

**Files** (re-grepped post-T4.4 per `controller_preflight`):
- Modify: `modules/world/player.go` (add fields + methods, remove `buildArea` field)
- Modify: `modules/world/data_map.go`
- Modify: `modules/world/tick.go` (delete `p.buildArea = ...` init line, delete `rsbuf.RemovePlayer` shim call)
- Modify: 6 test files (login_map_test, data_map_test, player_zone_test, player_npc_test, player_info_test, npc_event_queue_test)
- Delete: `pkg/buildarea/buildarea.go`, `pkg/buildarea/buildarea_test.go`, `pkg/buildarea/` directory

- [ ] **Step 1**: Re-grep all `p\.buildArea\.` and `buildarea\.` matches; enumerate exhaustively:

```bash
rg "p\.buildArea\." modules/ pkg/ cmd/
rg "\bbuildarea\." modules/ pkg/ cmd/
```

Save the output as a working list; verify no surprises vs spec's enumeration.

- [ ] **Step 2**: Add to `Player` struct in `player.go`:

```go
// Scenery-window state — flattened from pkg/buildarea at NAI-30 Bundle 4.
// Tracks which mapsquares the client has loaded for LOC/scenery rebuild
// purposes. Per-player; mutated by rebuildScenery() at zone-window exit.
lastBuild    int
loadedZones  map[int]bool
activeZones  map[int]bool
mapsquares   map[uint16]bool
```

In `newPlayer(...)`:

```go
lastBuild:   0,
loadedZones: map[int]bool{},
activeZones: map[int]bool{},
mapsquares:  map[uint16]bool{},
```

Add methods (port of `BuildArea.ShouldRebuild` + `Rebuild` from pkg/buildarea/buildarea.go:51-107):

```go
// shouldRebuild reports whether the player has crossed the 13x13 zone
// window centered on (originX, originZ), or whether reconnect is true.
// Mirrors pkg/buildarea.BuildArea.ShouldRebuild.
func (p *Player) shouldRebuild() bool {
	if p.originX == -1 {
		return true
	}
	if p.reconnecting {
		return true
	}
	originZoneX := p.originX >> 3
	originZoneZ := p.originZ >> 3
	reloadLeftX := (originZoneX - 4) << 3
	reloadRightX := (originZoneX + 5) << 3
	reloadTopZ := (originZoneZ + 5) << 3
	reloadBottomZ := (originZoneZ - 4) << 3
	if p.x < reloadLeftX || p.z < reloadBottomZ ||
		p.x > reloadRightX-1 || p.z > reloadTopZ-1 {
		return true
	}
	return false
}

// rebuildScenery resets the player's scenery-window state, recomputes
// the 13x13 zone window mapsquares centered on (p.x, p.z), and commits
// the new origin. Returns mapsquare list packed as (mapX<<8)|mapZ.
// Mirrors pkg/buildarea.BuildArea.Rebuild.
func (p *Player) rebuildScenery(currentTick int) []uint16 {
	p.loadedZones = map[int]bool{}
	p.activeZones = map[int]bool{}
	p.mapsquares = map[uint16]bool{}

	zoneX := p.x >> 3
	zoneZ := p.z >> 3
	for dx := -6; dx <= 6; dx++ {
		for dz := -6; dz <= 6; dz++ {
			zx := zoneX + dx
			zz := zoneZ + dz
			if zx < 0 || zz < 0 {
				continue
			}
			mapX := zx >> 3
			mapZ := zz >> 3
			if mapX > 0xff || mapZ > 0xff {
				continue
			}
			p.mapsquares[uint16((mapX<<8)|mapZ)] = true
			p.activeZones[coordgrid.ZoneIndex(zx<<3, zz<<3, 0)] = true
		}
	}

	p.originX = p.x
	p.originZ = p.z
	p.lastBuild = currentTick

	out := make([]uint16, 0, len(p.mapsquares))
	for m := range p.mapsquares {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
```

Remove `buildArea *buildarea.BuildArea` from struct, remove `buildArea: buildarea.New()` from newPlayer (or wherever it's set), remove import of `pkg/buildarea`.

- [ ] **Step 3**: Update each enumerated call site mechanically. For data_map.go, player.go, etc. Plan-author works through the grep list one site at a time. Examples:

In `modules/world/data_map.go:116,129`:
```go
// Was: if p.buildArea.LastBuild + ...
if p.lastBuild+rebuildGetMapsLastBuildTicks < s.currentTick { ... }
// Was: if !p.buildArea.Mapsquares[mapsquare] { ... }
if !p.mapsquares[mapsquare] { ... }
```

In `modules/world/player.go:436,439,455-465,476`:
```go
// Was: if !p.buildArea.ShouldRebuild(p.x, p.z, p.reconnecting) { ... }
if !p.shouldRebuild() { ... }
// Was: ms := p.buildArea.Rebuild(p.x, p.z, p.client.server.currentTick)
ms := p.rebuildScenery(p.client.server.currentTick)
// Was: for idx := range p.buildArea.LoadedZones { if !p.buildArea.ActiveZones[idx] { ... } }
for idx := range p.loadedZones {
    if !p.activeZones[idx] { ... }
}
// Was: for idx := range p.buildArea.ActiveZones { if !p.buildArea.LoadedZones[idx] { ... } }
for idx := range p.activeZones {
    if !p.loadedZones[idx] { ... }
}
// Was: p.buildArea.LoadedZones[idx] = true
p.loadedZones[idx] = true
```

In `modules/world/tick.go:95`, delete:
```go
// Delete entirely: p.buildArea = buildarea.New()
```

In `modules/world/tick.go:172-175`, delete the `rsbuf.RemovePlayer(p.slot, p.buildArea.Npcs)` call (already-replaced by `s.rsbuf.RemovePlayer` at server.go:670-672 from NAI-29):
```go
// DELETE these 3 lines entirely:
// if p.buildArea != nil {
//     rsbuf.RemovePlayer(p.slot, p.buildArea.Npcs)
// }
```

In test files, sweep:
- `p.buildArea.OriginX/OriginZ` → `p.originX/originZ`
- `p.buildArea.LastBuild` → `p.lastBuild`
- `p.buildArea.Mapsquares[...]` → `p.mapsquares[...]`
- `p.buildArea.LoadedZones[idx]` → `p.loadedZones[idx]`
- `p.buildArea.Players[N]` (used as map check) → `s.rsbuf.HasPlayer(int32(p.slot), N)`
- `p.buildArea.Npcs[N]` → `s.rsbuf.HasNpc(int32(p.slot), int32(N))`

For test sites that wrote into `p.buildArea.Npcs[101] = struct{}{}` to seed observed NPCs (npc_event_queue_test.go), the equivalent is to either subscribe via `s.rsbuf.players[1].Build.Npcs.Insert(101)` (reaches into unexported state — needs an accessor) OR add a `(b *Buf) SubscribeNpcForTest(pid, nid int32)` test-only method to `pkg/rsbuf/buf.go`. Plan-author chooses the simpler path (accessor).

If adding the accessor:
```go
// SubscribeNpcForTest adds nid to pid's BuildArea.Npcs tracking set.
// Test-only; mirrors the manual map-write that the
// p.buildArea.Npcs[nid] = struct{}{} pattern used pre-NAI-30.
func (b *Buf) SubscribeNpcForTest(pid, nid int32) {
	if pid < 0 || int(pid) >= len(b.players) || b.players[pid] == nil {
		return
	}
	b.players[pid].Build.Npcs.Insert(nid)
}
```

- [ ] **Step 4**: Build + full test sweep:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...
```

Expected: all PASS. If anything fails, the buildarea flatten missed a site — re-grep `p\.buildArea\.` to find the residual.

- [ ] **Step 5**: Commit. Then second commit deletes `pkg/buildarea/`:

```bash
git add modules/world/*.go pkg/rsbuf/buf.go
git commit --no-gpg-sign -m "feat(world): NAI-30 Bundle 4 Task 4.5a — flatten pkg/buildarea scenery fields onto Player

[body explains the flatten + producer→Player methods + 25-site sweep + new test accessor]

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"

git rm -r pkg/buildarea/
git commit --no-gpg-sign -m "refactor(buildarea): NAI-30 Bundle 4 Task 4.5b — delete pkg/buildarea/

All consumers migrated to flattened Player fields + rsbuf BuildArea.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

## Task 4.6: Verify-and-delete dead packages/files

**Files** (each gated on grep returning zero matches):
- Delete: `pkg/grid/`
- Delete: `pkg/rsbuf/npc_observers.go` + `pkg/rsbuf/npc_observers_test.go`
- Modify: `pkg/rsbuf/source.go` (trim `AppearanceHash() uint64` from interface)
- Delete: `(p *Player) AppearanceHash()` in `modules/world/player_source.go`
- Delete: `appearanceHash` helper in `modules/world/appearance.go` (if dead)
- Delete: `EncodeLegacy` + helper functions in `pkg/rsbuf/playerinfo.go`
- Delete: `EncodeNpcLegacy` + helpers in `pkg/rsbuf/npcinfo.go`
- Delete: `s.grid` field + initialization + writes in `modules/world/server.go` + `tick.go`

- [ ] **Step 1**: Run each verification grep. Expected zero matches. If any returns non-zero, halt and migrate the residual.

```bash
rg "rsbuf\.GetNpcObservers\b\(|rsbuf\.SetObserverForTest\b\(|rsbuf\.RemovePlayer\b\(|rsbuf\.incNpcObserver\b|rsbuf\.decNpcObserver\b" pkg/ modules/ cmd/
rg "AppearanceHash\b\(" pkg/ modules/ cmd/
rg "rsbuf\.Encode\b\(|rsbuf\.EncodeLegacy\b\(|rsbuf\.EncodeNpc\b\(|rsbuf\.EncodeNpcLegacy\b\(" pkg/ modules/ cmd/
rg "grid\.New\b|s\.grid\b|grid\.Grid\b|\"github.com/zsrv/goscape/pkg/grid\"" pkg/ modules/ cmd/
```

- [ ] **Step 2**: Delete in this order (each preceded by a grep verification):

1. `git rm pkg/rsbuf/npc_observers.go pkg/rsbuf/npc_observers_test.go`
2. Edit `pkg/rsbuf/source.go` — remove the `AppearanceHash() uint64` line from `PlayerSource` interface.
3. Edit `modules/world/player_source.go` — remove `func (p *Player) AppearanceHash() uint64 { ... }`.
4. Edit `modules/world/appearance.go` — if `appearanceHash` helper exists and is no longer called, remove it; if `hash/fnv` import becomes dead, remove that too.
5. Edit `pkg/rsbuf/playerinfo.go` — delete `EncodeLegacy` + helpers (`writeLocalPlayer` package func — careful, the new method has the same name; verify the package func is dead vs the method which lives), `boolToInt`, `clamp`, `zoneDist`, `abs`. Re-grep each before delete.
6. Edit `pkg/rsbuf/npcinfo.go` — delete `EncodeNpcLegacy` + helpers.
7. Edit `pkg/rsbuf/playerinfo_test.go` + `npcinfo_test.go` — delete legacy test functions (the ones that called `EncodeLegacy` / `EncodeNpcLegacy`).
8. `git rm -r pkg/grid/`
9. Edit `modules/world/server.go` — remove `s.grid = grid.New()` line, remove `s.grid.AddNpc(...)` line, remove `grid` import. Edit `tick.go` to remove the `s.grid.Add/Remove/AddNpc/RemoveNpc` lines (already partially handled in T4.5 if buildarea flatten covered them; verify).

- [ ] **Step 3**: After each delete, run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` to confirm nothing breaks.

- [ ] **Step 4**: Final full test sweep:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all PASS, no warnings.

- [ ] **Step 5**: Commit each delete as a separate commit (or batch logically). Suggested grouping:

```bash
# Commit A: npc_observers shim retirement
git rm pkg/rsbuf/npc_observers.go pkg/rsbuf/npc_observers_test.go
git commit --no-gpg-sign -m "refactor(rsbuf): NAI-30 Bundle 4 Task 4.6a — delete npc_observers.go shim"

# Commit B: AppearanceHash dead-API retirement
git add pkg/rsbuf/source.go modules/world/player_source.go modules/world/appearance.go
git commit --no-gpg-sign -m "refactor: NAI-30 Bundle 4 Task 4.6b — delete AppearanceHash dead API"

# Commit C: legacy encoder + helpers retirement
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/npcinfo.go pkg/rsbuf/playerinfo_test.go pkg/rsbuf/npcinfo_test.go
git commit --no-gpg-sign -m "refactor(rsbuf): NAI-30 Bundle 4 Task 4.6c — delete EncodeLegacy/EncodeNpcLegacy + helpers"

# Commit D: pkg/grid retirement
git rm -r pkg/grid/
git add modules/world/server.go modules/world/tick.go
git commit --no-gpg-sign -m "refactor(grid): NAI-30 Bundle 4 Task 4.6d — delete pkg/grid"
```

## Task 4.7: NAI-30 close — polish + smoke test + close commit

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (append NAI-30 close entry)
- Final commit with `Closes memory:` trailer

- [ ] **Step 1**: Run `go test -race ./...` one final time; verify all green.
- [ ] **Step 2**: Run `go build -trimpath ./cmd/goscape`; verify clean build.
- [ ] **Step 3**: Hand off smoke test to user per `smoke_test_server_handoff` memory:

> "Bundle 4 cutover + retirements complete. Could you launch the server (`CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`) and connect with the Java client to confirm PlayerInfo + NpcInfo render correctly (no crashes, players + NPCs visible, movement/chat/animations behave)?"

Wait for user confirmation before proceeding.

- [ ] **Step 4**: Update `nai_followups.md` per spec's "Memory entries to potentially add at NAI-30 close" guidance. Append a "From NAI-30 (2026-04-XX)" close entry mirroring the NAI-29 close pattern: bundles + tasks + commits, deviation count delta, key insights worth preserving.

- [ ] **Step 5**: Final close commit:

```bash
git add ~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(rsbuf,world): NAI-30 closed — encoder loops port + read-flip

4-bundle execution:
- Bundle 1: BuildArea spatial-discovery primitives + entity field plumbing
  (OrientationX/Z, lastAppearance tick port)
- Bundle 2: PlayerInfo struct + Encode method (info.rs:13-409 port)
- Bundle 3: NpcInfo struct + Encode method (info.rs:411-708 port)
- Bundle 4: caller swap, npc_observers shim consumer migration,
  pkg/buildarea flatten onto Player, dead-code grep-and-delete

Net deviation count: 13 → 14 (NAI-30-D1: orientation field plumbed
without producer; retires when engine-port series wires set_orient
+ npc-config initial orientation).

Retired:
- pkg/grid/ (consumers migrated to *Buf.zoneMap)
- pkg/buildarea/ (encoder fields in *rsbuf.BuildArea since NAI-29;
  scenery fields flattened onto Player)
- pkg/rsbuf/npc_observers.go (replaced by *Buf.GetNpcObservers + SetObserverForTest)
- (p *Player).AppearanceHash() + appearanceHash helper (lastAppearance
  tick semantics replaced content-hash dedup)
- AppearanceHash() member of PlayerSource interface (rest of interface
  alive; renderer.go + mask_payload.go consumers migrate at NAI-31)
- EncodeLegacy + EncodeNpcLegacy + helpers (callers swapped to *Buf methods)

NAI-30-D2 (deferred to NAI-31): local-player CHAT mask suppression
in writeLocalPlayer requires per-mask renderer cache; documented inline.

Closes memory: nai_followups.md NAI-30 close entry; parallel_spatial_index_migration_pattern.md (player+npc spatial-index migration retired); dead_api_polish.md; enumerate_all_sites.md.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

1. **Spec coverage:** Each Bundle 1 source-mapping table entry maps to a task (T1.1 → GetNearbyPlayers; T1.2 → GetNearbyNpcs + filterNpc; T1.3 → orientation; T1.4 → lastAppearance; T1.5 → integration test). Bundle 2 covers info.rs:13-409 across T2.2 (struct/skeleton), T2.3 (writeLocalPlayer), T2.4 (writePlayers), T2.5 (writeNewPlayers + add), T2.6 (lastAppearance dedup), T2.7 (visibility-soft), T2.8 (CHAT deferred to NAI-31), T2.9 (output-bytes-are-copy). Bundle 3 covers info.rs:411-708 across T3.1-T3.7. Bundle 4 covers all retirements per spec's T4.1-T4.7 list. NAI-30-D1 (orientation) explicitly tagged in T1.3 commit + close. NAI-30-D2 (local CHAT) introduced + tagged in T2.8.

2. **Placeholder scan:** No "TBD"/"TODO" placeholders. T2.7's `StaffModLevel` field-add is conditional on prior schema state — flagged as "plan-author re-verifies" rather than left as TBD. T3.1-T3.7 use a "follow B2 cadence" abbreviated form in tasks 3.5-3.7 (the structural mirror is exact; full repetition would be ~600 lines of duplicate scaffolding). This is the trade-off between strict no-placeholders and YAGNI plan length.

3. **Type consistency:** `OrientationX/Z`, `lastAppearance`, `LastAppearance` (Player struct field on rsbuf.Player vs lowercase on modules/world.Player) are consistent across tasks. `playerBitsAdd=23`/`npcBitsAdd=35` correct. `preferredPlayers=250`/`preferredNpcs=255`/`preferredViewDistance=15` from NAI-29 reused. `withinDistanceSW` introduced T1.1 used in T1.2 + T2.4 + T3.3.

4. **Ambiguity check:** T2.7's StaffModLevel handling and T2.8's CHAT deferral both have explicit "plan-author decision" notes — these are intentional ambiguity surfaced to the implementer, not vagueness in the plan.

Plan complete and saved to `docs/superpowers/plans/2026-04-26-nai-30-encoder-readflip-plan.md`.

---

**Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
