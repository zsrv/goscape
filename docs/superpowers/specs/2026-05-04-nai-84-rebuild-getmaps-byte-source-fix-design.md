## NAI-84: Fix REBUILD_GETMAPS byte source mismatch + port rebuildZones

**Date**: 2026-05-04
**Cadence**: combined spec + plan, single end-of-impl review (per
`compressed_cadence.md`, ≤100 production-LOC band; ~30 production LOC).
**Predecessor**: NAI-83 (HEAD `a0549cf` — LOC_ANGLE wired; cascade still
pending Tutorial Island door-click smoke).
**Pivot trigger**: user cleared Java client cache; client now freezes at
"Map area updated since last visit, so load will take longer this time
only."
**Successor**: TBD (drained by next user-driven cache-clear smoke).

### 1. Problem

Two bugs in goscape's REBUILD_GETMAPS path send the Java 225 client
into a hung "Map area updated since last visit, so load will take longer
this time only" state:

**Bug A — wrong byte source (root cause).**
`sendRebuildNormal` (`modules/world/rebuildmap.go:18-34`) advertises CRCs
from `cache.PreloadedCRC[m{x}_{z}]` / `cache.PreloadedCRC[l{x}_{z}]`,
which are loaded from `data/pack/client/maps/` (the client-pack files —
bzip2-compressed and pre-encrypted for over-the-wire delivery).

`handleRebuildGetMaps` (`modules/world/data_map.go:106-143`) then streams
`gm.LandBytes(mapX, mapZ)` / `gm.LocBytes(mapX, mapZ)`, which are loaded
from `data/pack/server/maps/` via `gamemap.Init` (the **server-pack**
files — uncompressed for in-process collision parsing).

The two directories contain completely different bytes for the same
filenames. Verified via `sha256sum`:

| File | Client-pack size | Server-pack size | Match? |
|---|---|---|---|
| `l29_75` | 441 B | 6598 B | DIFF |
| `l30_75` | 332 B | 321 B | DIFF |

Result: the client receives bytes whose CRC does not match what
RebuildNormal promised, decompression fails, the client repeatedly
re-requests the same map, and the loading screen freezes.

The asymmetry is already documented in
`pkg/gamemap/gamemap_test.go:111-113`:

> "The actual cache layout has n*_* and o*_* files only under
> server/maps/; client/maps/ has only m and l files (and they differ in
> content from server/maps/m,l). Reading from the wrong directory leaves
> npcSpawns empty and NPCs never render in the client."

The same lesson was never applied at the dispatch site for outbound
DATA_LAND / DATA_LOC streaming.

**Bug B — missing `rebuildZones` call (compounding).**
TS `RebuildGetMapsHandler.ts:66` calls `player.buildArea.rebuildZones()`
at the end of the handler. Goscape has no equivalent. In TS,
`rebuildZones` populates `activeZones` to a 7×7-zone window centered on
the player's current zone (intersected with the 13×13-zone build-area
window from origin); `rebuildNormal` deliberately leaves `activeZones`
**empty** so zone deltas don't flow until the client confirms maps are
loaded.

Goscape's `(*Player).rebuildScenery` (`modules/world/player.go:600-635`)
populates `activeZones` simultaneously with `mapsquares` (a 13×13 set).
This means goscape sends zone deltas before the client has finished
downloading map data — a second source of client confusion compounding
Bug A.

### 2. TS reference

`LostCityRS/Engine-TS/src/network/game/client/handler/RebuildGetMapsHandler.ts:43-66`:

```ts
if (type === 0) {
    const land: Uint8Array | undefined = PRELOADED.get(`m${x}_${z}`);
    if (!land) { continue; }
    for (let off: number = 0; off < land.length; off += chunk) {
        player.write(new DataLand(x, z, off, land.length, land.subarray(off, off + chunk)));
    }
    player.write(new DataLandDone(x, z));
} else if (type === 1) {
    const loc: Uint8Array | undefined = PRELOADED.get(`l${x}_${z}`);
    if (!loc) { continue; }
    for (let off: number = 0; off < loc.length; off += chunk) {
        player.write(new DataLoc(x, z, off, loc.length, loc.subarray(off, off + chunk)));
    }
    player.write(new DataLocDone(x, z));
}

player.buildArea.rebuildZones();
```

`LostCityRS/Engine-TS/src/engine/entity/BuildArea.ts:31-55`:

```ts
rebuildZones(): void {
    this.activeZones.clear();

    const centerX = CoordGrid.zone(this.player.x);
    const centerZ = CoordGrid.zone(this.player.z);

    const originX: number = CoordGrid.zone(this.player.originX);
    const originZ: number = CoordGrid.zone(this.player.originZ);

    const leftX = originX - 6;
    const rightX = originX + 6;
    const topZ = originZ + 6;
    const bottomZ = originZ - 6;

    for (let x = centerX - 3; x <= centerX + 3; x++) {
        for (let z = centerZ - 3; z <= centerZ + 3; z++) {
            if (x < leftX || x > rightX || z > topZ || z < bottomZ) { continue; }
            this.activeZones.add(ZoneMap.zoneIndex(x << 3, z << 3, this.player.level));
        }
    }
}
```

### 3. Existing goscape surface

| Concern | Location | Status |
|---|---|---|
| Outbound CRC source | `modules/world/rebuildmap.go:26-27` (`cache.PreloadedCRC`) | ✓ correct (client-pack) |
| `cache.Preloaded` populator | `modules/world/world.go:83` (`PreloadClient("data/pack/client")`) | ✓ wired at startup |
| Outbound byte source | `modules/world/data_map.go:63, :80` (`gm.LandBytes`/`gm.LocBytes`) | **wrong** (server-pack; should be `cache.Preloaded`) |
| Stream helper signature | `modules/world/data_map.go:62, :79` (`streamLand`/`streamLoc(p, gm, ...)`) | needs `gm` removed |
| Handler plumbing | `modules/world/data_map.go:111-114` (`gm := s.gamemap`) | needs removal |
| Post-handler `rebuildZones` | absent | **missing** |
| `activeZones` populator | `(*Player).rebuildScenery` (`player.go:600-635`) | populates 13×13 (TS leaves empty post-rebuildNormal) |
| Test fixture | `modules/world/data_map_test.go` 7 tests use `gamemap.SetLandBytesForTest`/`SetLocBytesForTest` | needs migration to `cache.Preloaded` |
| Existing seed pattern | `seedCachedMidi` (`player_script_test.go:436-444`) | reusable shape |

### 4. Design

#### 4.1 Byte source re-route

`modules/world/data_map.go` — drop the `gm *gamemap.GameMap` parameter
from `streamLand` and `streamLoc`; replace gamemap reads with
`cache.Preloaded[...]` lookups:

```go
import (
    "fmt"

    "github.com/zsrv/goscape/pkg/cache"
    "github.com/zsrv/goscape/pkg/io/packet"
    gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// streamLand chunks the client-pack land file for (mapX, mapZ) into
// DATA_LAND packets followed by exactly one DATA_LAND_DONE. Silent
// no-op if the mapsquare isn't in cache.Preloaded.
//
// Reads from cache.Preloaded (data/pack/client/maps/) — NOT
// gamemap.LandBytes (data/pack/server/maps/). Per TS
// RebuildGetMapsHandler.ts:44 (PRELOADED.get('m${x}_${z}')). The two
// directories hold byte-different files for the same filename: server
// maps are uncompressed for collision parsing; client maps are
// pre-compressed/encrypted for over-the-wire delivery and match the
// CRCs advertised in RebuildNormal.
func streamLand(p *Player, mapX, mapZ int) {
    data := cache.Preloaded[fmt.Sprintf("m%d_%d", mapX, mapZ)]
    if data == nil {
        return
    }
    total := len(data)
    for off := 0; off < total; off += rebuildGetMapsChunkSize {
        end := off + rebuildGetMapsChunkSize
        if end > total {
            end = total
        }
        sendDataLand(p, mapX, mapZ, off, total, data[off:end])
    }
    sendDataLandDone(p, mapX, mapZ)
}

// streamLoc is the symmetric helper for DATA_LOC. Same client-pack
// source, same per-chunk + done-packet structure. Per TS
// RebuildGetMapsHandler.ts:54.
func streamLoc(p *Player, mapX, mapZ int) {
    data := cache.Preloaded[fmt.Sprintf("l%d_%d", mapX, mapZ)]
    if data == nil {
        return
    }
    total := len(data)
    for off := 0; off < total; off += rebuildGetMapsChunkSize {
        end := off + rebuildGetMapsChunkSize
        if end > total {
            end = total
        }
        sendDataLoc(p, mapX, mapZ, off, total, data[off:end])
    }
    sendDataLocDone(p, mapX, mapZ)
}
```

#### 4.2 Handler restructure + rebuildZones call

`modules/world/data_map.go` — drop `gm := s.gamemap; if gm == nil` and
the `gm` arg from `streamLand`/`streamLoc` calls; append a
`p.rebuildZones()` call at the end mirroring TS line 66:

```go
func handleRebuildGetMaps(p *Player, payload []byte) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    s := p.client.server

    if p.lastBuild+rebuildGetMapsLastBuildTicks < s.currentTick {
        return nil
    }

    nEntries := len(payload) / 3
    if nEntries > rebuildGetMapsMapsLimit {
        return nil
    }

    r := packet.NewPacket(payload)
    for i := 0; i < nEntries; i++ {
        packed := int(r.G3())
        mapsquare := uint16(packed & 0xFFFF)
        if !p.mapsquares[mapsquare] {
            continue
        }
        typ := (packed >> 16) & 0x1
        mapX := int(mapsquare>>8) & 0xFF
        mapZ := int(mapsquare) & 0xFF
        switch typ {
        case 0:
            streamLand(p, mapX, mapZ)
        case 1:
            streamLoc(p, mapX, mapZ)
        }
    }

    // Mirrors TS RebuildGetMapsHandler.ts:66 — refresh activeZones to
    // the player's current 7×7 zone window now that maps are loaded.
    p.rebuildZones()
    return nil
}
```

The `gamemap` import is removed from `data_map.go`. `s := p.client.server`
is retained because `s.currentTick` is still consulted for the
rate-limit gate.

#### 4.3 `rebuildZones` port

`modules/world/player.go` — append after `rebuildScenery` (line 635):

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
func (p *Player) rebuildZones() {
    p.activeZones = map[int]bool{}
    centerX := p.x >> 3
    centerZ := p.z >> 3
    originZoneX := p.originX >> 3
    originZoneZ := p.originZ >> 3
    leftX := originZoneX - 6
    rightX := originZoneX + 6
    bottomZ := originZoneZ - 6
    topZ := originZoneZ + 6
    for x := centerX - 3; x <= centerX+3; x++ {
        for z := centerZ - 3; z <= centerZ+3; z++ {
            if x < leftX || x > rightX || z < bottomZ || z > topZ {
                continue
            }
            if x < 0 || z < 0 {
                continue
            }
            p.activeZones[coordgrid.ZoneIndex(x<<3, z<<3, p.level)] = true
        }
    }
}
```

`coordgrid` is already imported in `player.go`. No import changes.

The `if x < 0 || z < 0 { continue }` clause is goscape-defensive (mirrors
the existing guard at `rebuildScenery:611`) — TS bypasses it via JS
number coercion + the build-area intersection generally clipping
negatives. Documented as a goscape-defensive gate per
`defensive_gate_doc_comment_label.md`.

`p.level` (TS-faithful per `BuildArea.ts:52`) — distinct from
`rebuildScenery`'s hardcoded `0` at `player.go:620`. The existing
`rebuildScenery` divergence is out of scope here (§6 R-D2).

#### 4.4 Test fixture migration

`modules/world/data_map_test.go` — 7 tests currently seed the wrong
source. Pattern:

Add a helper near the top of the file (after the existing
`newMapDataPlayer`):

```go
// seedClientMap seeds cache.Preloaded under the client-pack m{x}_{z}
// or l{x}_{z} key and registers a t.Cleanup to remove the entry after
// the test. Mirrors seedCachedMidi (player_script_test.go:436) for the
// map-streaming path.
func seedClientMap(t *testing.T, prefix byte, mapX, mapZ int, data []byte) {
    t.Helper()
    name := fmt.Sprintf("%c%d_%d", prefix, mapX, mapZ)
    cache.Preloaded[name] = data
    t.Cleanup(func() {
        delete(cache.Preloaded, name)
    })
}
```

Imports gain `"fmt"` and `"github.com/zsrv/goscape/pkg/cache"`. The
existing `gamemap` import is removed (no production test reads gamemap
after this change; `s.gamemap = gamemap.New(...)` plumbing in
`newMapDataPlayer` is dropped — handler no longer reads it).

Each `s.gamemap.SetLandBytesForTest(X, Y, b)` becomes
`seedClientMap(t, 'm', X, Y, b)`; each `SetLocBytesForTest(X, Y, b)`
becomes `seedClientMap(t, 'l', X, Y, b)`.

### 5. Tests

#### 5.1 Migration of existing tests (no semantic change)

Functional behaviour unchanged — same byte counts, same skip semantics
— only the seeding path differs:

| Test | Old | New |
|---|---|---|
| `TestHandleRebuildGetMapsSingleChunk` | `s.gamemap.SetLandBytesForTest(50, 51, …)` | `seedClientMap(t, 'm', 50, 51, …)` |
| `TestHandleRebuildGetMapsMultiChunk` | `SetLandBytesForTest(10, 20, file)` | `seedClientMap(t, 'm', 10, 20, file)` |
| `TestHandleRebuildGetMapsExactlyChunkBoundary` | `SetLandBytesForTest(1, 2, file)` | `seedClientMap(t, 'm', 1, 2, file)` |
| `TestHandleRebuildGetMapsRoutesToLoc` | `SetLocBytesForTest(3, 4, …)` | `seedClientMap(t, 'l', 3, 4, …)` |
| `TestHandleRebuildGetMapsSkipsUnknownMapsquare` | `SetLandBytesForTest(50, 51, …)` (player has no mapsquare entry) | `seedClientMap(t, 'm', 50, 51, …)` |
| `TestHandleRebuildGetMapsRateLimitedDropsEntireRequest` | `SetLandBytesForTest(50, 51, …)` | `seedClientMap(t, 'm', 50, 51, …)` |
| `TestHandleRebuildGetMapsMultipleEntries` | `SetLandBytesForTest + SetLocBytesForTest` | two `seedClientMap` calls |

`TestHandleRebuildGetMapsSkipsMissingFile` already pre-populates
`mapsquares` without seeding bytes; no migration needed (the empty
`cache.Preloaded["m99_99"]` lookup returns nil → silent skip, same
shape).

`TestSendDataLand…` / `TestSendDataLoc…` / `TestSendDataLandDone…` /
`TestSendDataLocDone…` test the wire helpers in isolation and are
unchanged.

#### 5.2 New: `TestHandleRebuildGetMapsCallsRebuildZones`

Pre-seed `p.activeZones` with a stale entry that lies outside the
expected 7×7 window. Configure `p.x=50<<3, p.z=50<<3, p.originX=50<<3,
p.originZ=50<<3, p.level=0` (centerZone=(50,50), originZone=(50,50);
expected activeZones = the 7×7 set centered on (50,50)). Run handler
with no map entries (nEntries=0 — passes rate-limit + entries-cap, hits
the `rebuildZones()` call path). Assert:

- The stale entry is removed from `p.activeZones`.
- `p.activeZones` contains exactly 49 entries (7×7 = 49 zones, none
  clipped because center==origin).
- A spot-check entry at `coordgrid.ZoneIndex(50<<3, 50<<3, 0)` is
  present.

#### 5.3 New: `TestRebuildZonesIntersectsBuildArea`

Direct unit test on `(*Player).rebuildZones`. Configure a player whose
current center is offset toward an edge of the build area so the 7×7
spills outside the 13×13 origin window. Specifically:

- `p.originX=50<<3, p.originZ=50<<3` → originZone=(50,50); build-area =
  `[44..56] × [44..56]`.
- `p.x=53<<3, p.z=53<<3` → centerZone=(53,53); raw 7×7 = `[50..56] × [50..56]`.
- All raw cells are within the build-area window → no clipping; expect
  49 entries.

Then push the center further: `p.x=56<<3, p.z=56<<3` → centerZone=(56,56);
raw 7×7 = `[53..59] × [53..59]`. Build-area still `[44..56] × [44..56]`,
so cells with x>56 or z>56 are clipped: kept cells = `[53..56] × [53..56]`
= 16 entries. Pin that count + the highest preserved cell at (56,56) is
present + the clipped cell at (57,57) is absent.

#### 5.4 New: `TestRebuildZonesHonorsPlayerLevel`

Configure two players at the same (x, z) on different levels (0 and 1).
Assert the resulting `activeZones` keys differ — pins that the port uses
`p.level` rather than the hardcoded `0` from `rebuildScenery`. One-shot
asymmetry pin per `ts_asymmetry_dual_pin.md` (this is the divergence
side; the absence-of-fix side is covered by the §6 R-D2 ledger entry).

### 6. TS-fidelity ledger

| TS construct | goscape mapping | Divergence? |
|---|---|---|
| `PRELOADED.get('m${x}_${z}')` | `cache.Preloaded[fmt.Sprintf("m%d_%d", x, z)]` | No — equivalent client-pack source |
| `PRELOADED.get('l${x}_${z}')` | `cache.Preloaded[fmt.Sprintf("l%d_%d", x, z)]` | No |
| `player.write(new DataLand(...))` chunk loop | `sendDataLand(p, ...)` chunk loop | No — same wire shape |
| `player.buildArea.rebuildZones()` post-stream | `p.rebuildZones()` post-stream | No — TS-faithful |
| `BuildArea.activeZones.clear()` then 7×7 fill | `p.activeZones = map[int]bool{}` then 7×7 fill | No — equivalent reset semantics |
| `this.player.level` in zoneIndex | `p.level` in `coordgrid.ZoneIndex` | No |
| TS `if (x < 0)` check absent | goscape `if x < 0 || z < 0 { continue }` | Goscape-defensive (`R-D1`); TS skips because Set#add tolerates negatives, but goscape's bounds-check matches the established `rebuildScenery:611` convention |
| Per-zone-change `rebuildZones()` (`NetworkPlayer.ts:271`) | absent | **Tracked deviation `R-D3`** — depends on lastZone tracking that hasn't been ported |

#### Tracked deviations

- **R-D1** (goscape-defensive): negative x/z skip in `rebuildZones`. No
  symptom; matches the local convention. Documented in the doc comment
  per `defensive_gate_doc_comment_label.md`.

- **R-D2** (pre-existing): `(*Player).rebuildScenery:620` populates
  `activeZones` at hardcoded `level=0` and overlaps the role TS leaves
  to `rebuildZones`. NAI-84 leaves this untouched — the dual-write is
  safe because `rebuildZones` clears `activeZones` before refilling, and
  the call order in REBUILD path (rebuildScenery → sendRebuildNormal →
  client requests maps → handleRebuildGetMaps → rebuildZones) means the
  rebuildScenery preset is replaced before any zone delta flows. Future
  cleanup: drop the `rebuildScenery` activeZones write and rely solely
  on `rebuildZones`. Tracked in
  `nai84_rebuildscenery_activezones_dedup.md` (deferred follow-up).

- **R-D3** (deferred port): TS calls `rebuildZones` on every zone
  transition (`NetworkPlayer.ts:271`). Goscape does not yet track
  `lastZone` per-tick. Until that surface lands, players who never
  trigger REBUILD_GETMAPS (cached client) have stale `activeZones` from
  `rebuildScenery` — goscape's pre-NAI-84 behaviour is preserved for
  this case via R-D2. Tracked in
  `nai84_rebuildzones_per_zone_change.md`.

### 7. Out of scope

- **`rebuildScenery` activeZones dual-write retirement** — see R-D2.
  Touching it now would require auditing every consumer of `activeZones`
  to confirm they tolerate empty-until-rebuildZones; out of scope for a
  cache-clear freeze fix.
- **Per-zone-change `rebuildZones` call site** — see R-D3. Depends on
  `lastZone` tracking that hasn't been ported.
- **Multiple `level` semantics for `rebuildScenery`** — separate latent
  divergence noted only in passing.
- **`gamemap.LandBytes` / `LocBytes` retirement** — these accessors
  remain reachable for any future consumer (e.g. tests, debug). They
  are no longer wired to REBUILD_GETMAPS; that's the only consumer
  currently. Document but don't delete (no orphan helpers — they're
  accessed by tests in `pkg/gamemap/gamemap_test.go:165-172`).

### 8. Smoke / cascade routing

Investigation cadence per `investigation_subspec_cadence.md`:

- Stage 1 (audit) — completed in this brainstorm; root cause found via
  byte-diff of `data/pack/client/maps/` vs `data/pack/server/maps/`.
- Stage 2 (fix) — this spec + plan + impl.
- Stage 3 (smoke) — user runs Java client with cache cleared; expected
  outcome is the loading screen progresses past "Map area updated since
  last visit" within seconds and the player spawns into Tutorial Island
  normally. Cascade attribution closes at this smoke.
- If the freeze persists or shifts: route per the decision tree in
  `cascade_theory_smoke_binding.md` — likely surfaces an adjacent issue
  (e.g. zone-delta encoding, or PlayerInfo init ordering) for NAI-85.

### 9. Tech stack

Go 1.26+ (per `go_version.md`).

### 10. Implementation plan

Execution mode: subagent-driven-development (per
`execution_mode_default.md`). Compressed cadence — single bundled
implementer for T1+T2+T3, single Sonnet reviewer at end (per
`compressed_cadence.md`, `superpowers_code_reviewer_model.md`).

**File manifest:**

| File | Action | Responsibility |
|---|---|---|
| `modules/world/data_map.go` | Modify | drop `gm` from streamLand/streamLoc; replace gamemap reads with `cache.Preloaded`; drop gamemap plumbing in handler; add `p.rebuildZones()` call; replace `gamemap` import with `cache` import |
| `modules/world/player.go` | Modify | append `(*Player).rebuildZones` method after `rebuildScenery` |
| `modules/world/data_map_test.go` | Modify | drop `gamemap` import + `s.gamemap = gamemap.New(...)` in `newMapDataPlayer`; add `cache` + `fmt` imports + `seedClientMap` helper; migrate 7 tests; add 3 new tests |

#### Task 1 — Red

Land everything that fails the build / fails new tests. Single commit.

##### Step 1.1 — Migrate `newMapDataPlayer` and existing tests

In `modules/world/data_map_test.go`:

- Remove `"github.com/zsrv/goscape/pkg/gamemap"` from imports. Add
  `"fmt"` and `"github.com/zsrv/goscape/pkg/cache"`.
- In `newMapDataPlayer`, delete the line `s.gamemap = gamemap.New(discardLogger())`.
  (`s.currentTick = 5` and the rest of the helper stay.)
- Insert the `seedClientMap` helper from §4.4 right after `newMapDataPlayer`.
- For each of the 7 listed tests, replace `s.gamemap.SetLandBytesForTest(X, Y, b)`
  with `seedClientMap(t, 'm', X, Y, b)` and `s.gamemap.SetLocBytesForTest(X, Y, b)`
  with `seedClientMap(t, 'l', X, Y, b)`. Remove now-unused `s` from
  `newMapDataPlayer` returns where possible (note: `s` is still needed
  by `TestHandleRebuildGetMapsRateLimitedDropsEntireRequest` for
  `s.currentTick = 100`; keep that one). For tests that no longer use
  `s`, change the destructure to `p, cc, _ := newMapDataPlayer(t)`.

##### Step 1.2 — Append the three new tests

After the last existing test (`TestHandleRebuildGetMapsMultipleEntries`
at line 280), append:

```go
func TestHandleRebuildGetMapsCallsRebuildZones(t *testing.T) {
    p, _, _ := newMapDataPlayer(t)
    p.x = 50 << 3
    p.z = 50 << 3
    p.originX = 50 << 3
    p.originZ = 50 << 3
    p.level = 0
    // Stale entry that should be cleared by rebuildZones.
    staleIdx := coordgrid.ZoneIndex(99<<3, 99<<3, 0)
    p.activeZones[staleIdx] = true

    if err := handleRebuildGetMaps(p, nil); err != nil {
        t.Fatalf("handleRebuildGetMaps: %v", err)
    }

    if p.activeZones[staleIdx] {
        t.Errorf("stale activeZones entry not cleared")
    }
    if want := 49; len(p.activeZones) != want {
        t.Errorf("activeZones size: got %d, want %d", len(p.activeZones), want)
    }
    if !p.activeZones[coordgrid.ZoneIndex(50<<3, 50<<3, 0)] {
        t.Errorf("center zone (50,50) missing from activeZones")
    }
}

func TestRebuildZonesIntersectsBuildArea(t *testing.T) {
    p, _ := newTestPlayer(t)

    // Case 1: center == origin → full 7×7, no clipping.
    p.originX = 50 << 3
    p.originZ = 50 << 3
    p.x = 50 << 3
    p.z = 50 << 3
    p.level = 0
    p.rebuildZones()
    if len(p.activeZones) != 49 {
        t.Errorf("center==origin: got %d zones, want 49", len(p.activeZones))
    }

    // Case 2: center pushed toward NE corner; build-area clips.
    // origin=(50,50); build-area window [44..56] × [44..56].
    // center=(56,56); raw 7×7 = [53..59] × [53..59]; clipped to
    // [53..56] × [53..56] = 16 entries.
    p.x = 56 << 3
    p.z = 56 << 3
    p.rebuildZones()
    if len(p.activeZones) != 16 {
        t.Errorf("clipped: got %d zones, want 16", len(p.activeZones))
    }
    if !p.activeZones[coordgrid.ZoneIndex(56<<3, 56<<3, 0)] {
        t.Errorf("kept cell (56,56) missing")
    }
    if p.activeZones[coordgrid.ZoneIndex(57<<3, 57<<3, 0)] {
        t.Errorf("clipped cell (57,57) present")
    }
}

func TestRebuildZonesHonorsPlayerLevel(t *testing.T) {
    p0, _ := newTestPlayer(t)
    p0.originX, p0.originZ = 50<<3, 50<<3
    p0.x, p0.z = 50<<3, 50<<3
    p0.level = 0
    p0.rebuildZones()

    p1, _ := newTestPlayer(t)
    p1.originX, p1.originZ = 50<<3, 50<<3
    p1.x, p1.z = 50<<3, 50<<3
    p1.level = 1
    p1.rebuildZones()

    // Pin: level-0 keys differ from level-1 keys.
    sameKey := coordgrid.ZoneIndex(50<<3, 50<<3, 0)
    if !p0.activeZones[sameKey] {
        t.Fatalf("p0 missing level-0 center key")
    }
    if p1.activeZones[sameKey] {
        t.Errorf("p1 (level=1) should not have level-0 center key — port honors p.level")
    }
}
```

Imports: add `"github.com/zsrv/goscape/pkg/coordgrid"` to the test file
imports if not already present (likely not — verify at write time).

##### Step 1.3 — Verify red

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: build passes (the migration in 1.1 doesn't break compilation;
the old SetLandBytesForTest references are gone). Then run the
new tests:

`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleRebuildGetMapsCallsRebuildZones -v`

Expected: FAIL — `(*Player).rebuildZones` undefined.

`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRebuildZones -v`

Expected: FAIL — `(*Player).rebuildZones` undefined.

The migrated existing tests will FAIL on byte-source mismatch
(`cache.Preloaded` lookup returns nil because the test seeded `m`/`l`
keys but the production handler still reads `gm.LandBytes`). That's the
expected red signal for Bug A.

If any of the migrated tests pass at this stage, STOP — investigate.

##### Step 1.4 — Commit T1

```bash
git add modules/world/data_map_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-84 T1 — REBUILD_GETMAPS byte-source + rebuildZones red

Migrates 7 existing tests from gamemap.SetLandBytesForTest /
SetLocBytesForTest to seedClientMap (cache.Preloaded). Adds three new
tests pinning the rebuildZones port + build-area intersection +
per-level keying. Tests fail until T2: byte-source migration produces
nil Preloaded lookups against the unported handler, and rebuildZones
is undefined.
EOF
)"
```

#### Task 2 — Green

##### Step 2.1 — Re-route `streamLand` / `streamLoc` + handler

In `modules/world/data_map.go`:

- Replace the `gamemap` import with `"github.com/zsrv/goscape/pkg/cache"`.
- Replace the bodies of `streamLand` and `streamLoc` per §4.1 (drop
  `gm *gamemap.GameMap` parameter, read from `cache.Preloaded`).
- Replace `handleRebuildGetMaps` body per §4.2 (drop `gm := s.gamemap;
  if gm == nil { return nil }`; drop `gm` arg from streamLand/streamLoc
  calls; append `p.rebuildZones()` before `return nil`).

Verify imports — final imports block should be `fmt`,
`github.com/zsrv/goscape/pkg/cache`,
`github.com/zsrv/goscape/pkg/io/packet`, and
`gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"`. The
`gamemap` import is removed.

##### Step 2.2 — Append `rebuildZones` to player.go

Append the method body from §4.3 immediately after `rebuildScenery`'s
closing brace at `modules/world/player.go:635`. No new imports.

##### Step 2.3 — Verify green

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestHandleRebuildGetMaps|TestRebuildZones" -v`

Expected: all pass.

##### Step 2.4 — Commit T2

```bash
git add modules/world/data_map.go modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(world): NAI-84 T2 — REBUILD_GETMAPS reads client-pack + rebuildZones port

Bug A: streamLand/streamLoc now read cache.Preloaded[m{x}_{z} / l{x}_{z}]
(client-pack from data/pack/client/maps/) instead of gm.LandBytes /
gm.LocBytes (server-pack from data/pack/server/maps/). The CRCs
advertised in RebuildNormal already came from cache.PreloadedCRC; the
byte stream now matches.

Bug B: handleRebuildGetMaps calls (*Player).rebuildZones at end,
mirroring TS RebuildGetMapsHandler.ts:66. New rebuildZones method
populates activeZones to the 7×7-zone window centered on the player's
current zone, intersected with the 13×13-zone build-area window from
origin, keyed at p.level (TS-faithful).

Closes the cache-clear freeze surfaced by clearing the Java 225 client
cache after long absence ("Map area updated since last visit, so load
will take longer this time only").
EOF
)"
```

#### Task 3 — Verify

##### Step 3.1 — Full test suite

`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

All packages green. Most-likely failure points if anything breaks:

- `modules/world` other tests that depended on `s.gamemap` being non-nil
  via `newMapDataPlayer` (no longer set after Step 1.1). If any test
  outside `data_map_test.go` reuses `newMapDataPlayer`, audit the call
  sites first.

##### Step 3.2 — Vet clean

`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

No output expected.

##### Step 3.3 — No commit

Verification only.

#### Combined review

Single Sonnet reviewer at end (per `compressed_cadence.md` and
`superpowers_code_reviewer_model.md`). Cumulative diff: `<spec-SHA>..HEAD`
(spec → impl range). Review checklist:

1. **TS-fidelity** — byte source matches `PRELOADED.get(...)`;
   `rebuildZones` matches `BuildArea.rebuildZones` line-by-line including
   the `p.level` use.
2. **No dead imports** — `gamemap` removed from `data_map.go`; `cache`
   added.
3. **R-D1 doc-comment** — negative-axis skip is labeled
   "(goscape defensive; TS skips this check)" per
   `defensive_gate_doc_comment_label.md`.
4. **R-D2 doc-comment** — `rebuildZones` doc explains the dual-write
   relationship with `rebuildScenery` and references R-D2.
5. **Test migration** — all 7 migrated tests use `seedClientMap`; no
   stragglers using `SetLandBytesForTest` / `SetLocBytesForTest` for
   handler tests (`pkg/gamemap/gamemap_test.go` retains its own use of
   those fixtures and is not affected).
6. **`go test ./...`** + **`go vet ./...`** clean.
7. **Commit content matches stated diff** — `git show <T1> --stat` and
   `git show <T2> --stat` per `implementer_commit_content_verify.md`.

#### Close commit

After reviewer approves:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
close: NAI-84 — REBUILD_GETMAPS byte-source fix + rebuildZones port

Cascade: Java 225 client cache-clear freeze at "Map area updated since
last visit, so load will take longer this time only". Two bugs:
(1) handler streamed server-pack bytes against client-pack CRCs;
(2) activeZones never refreshed to 7×7 post-handler. Both fixed.
Cascade attribution closes at the next user-driven cache-clear smoke.

Closes memory: nai84_seed_rebuild_getmaps.md
EOF
)"
```

(Empty close commit per `close_commit_memory_trailer.md`.)

### 11. Self-review

**Spec coverage:**
- §4.1 byte-source re-route → T2 Step 2.1 ✓
- §4.2 handler restructure + rebuildZones call → T2 Step 2.1 ✓
- §4.3 rebuildZones port → T2 Step 2.2 ✓
- §4.4 test fixture migration → T1 Steps 1.1, 1.2 ✓
- §5.1 7 migrated tests → T1 Step 1.1 ✓
- §5.2 rebuildZones-call test → T1 Step 1.2 ✓
- §5.3 build-area intersection test → T1 Step 1.2 ✓
- §5.4 level-honoring test → T1 Step 1.2 ✓
- §6 ledger + R-D1/R-D2/R-D3 doc-comments and tracker entries → T2 Step 2.1, 2.2 ✓
- §8 smoke routing → close commit body ✓

**Placeholder scan:** no TBD, no "appropriate", no inline TODO.

**Type consistency:** `rebuildZones()` no args, no return — matches both
the TS shape (`rebuildZones(): void`) and the new call site signature.
`coordgrid.ZoneIndex(x, z, level int) int` matches existing usage at
`player.go:620`.

**Premise-freshness reverification at HEAD `88a6d1c`** (per
`controller_preflight.md`):
- `data_map.go:62, :79` — `streamLand(p, gm, ...)` / `streamLoc(p, gm, ...)` confirmed.
- `data_map.go:111-114` — `gm := s.gamemap; if gm == nil { return nil }` confirmed.
- `rebuildmap.go:18-34` — CRC source confirmed.
- `world.go:83` — `cache.PreloadClient("data/pack/client")` confirmed.
- `player.go:600-635` — `rebuildScenery` confirmed; `level=0` hardcode at line 620 confirmed.
- `player_script_test.go:436-444` — `seedCachedMidi` shape confirmed.
- `data_map_test.go:9, :19` — `gamemap` import + `s.gamemap = gamemap.New(...)` confirmed.
- 7 tests using `SetLandBytesForTest`/`SetLocBytesForTest` confirmed at lines 123, 146, 168, 184, 200, 231, 263, 264.
- `(*Player).newTestPlayer` is reusable for direct unit tests on `rebuildZones` (verified — used elsewhere in `data_map_test.go:22`).

**Spec → impl exit clean.**
