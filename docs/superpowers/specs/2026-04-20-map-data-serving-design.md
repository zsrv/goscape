# Sub-spec 5b: Map Data Serving — Design

**Status:** Draft → ready for plan
**Scope:** Respond to the client's `REBUILD_GETMAPS` (opcode 150) request by chunking the raw `m{X}_{Z}` / `l{X}_{Z}` bytes retained by sub-spec 5a and shipping them via `DATA_LAND` / `DATA_LOC` / `DATA_LAND_DONE` / `DATA_LOC_DONE`.
**Out of scope:** `writeFullFollows` Respawn-branch replay (5c); `LocType` collision; NPC/obj file delivery.

---

## Goal

Make the client render real landscape after login. When the client receives a `RebuildNormal` packet with CRCs it can't match against its local cache, it fires a `REBUILD_GETMAPS` listing missing mapsquares; the server must stream the raw m/l bytes back chunked.

After this sub-spec, a player logging in sees ground tiles, walls, and static locs rendered from the server's cache directory. Sub-spec 5a's in-memory static-loc state becomes client-visible for the first time.

## Architecture

Self-contained new file with 4 sender helpers + 1 handler. Four additive `Op{}` entries. Handler registration via one line in the existing game-handlers init block. No changes to buildarea or gamemap — 5a prepared both (`BuildArea.LastBuild`, `BuildArea.Mapsquares`, `GameMap.LandBytes`, `GameMap.LocBytes`).

```
pkg/io/protocol/game/server/prot.go     + 4 Op{} entries
modules/world/data_map.go (new)         4 sender helpers + handleRebuildGetMaps + streamLand/streamLoc
modules/world/data_map_test.go (new)
modules/world/handlers_game.go          register gameHandlers[150]
```

## Components

### 1. Outer opcodes — `pkg/io/protocol/game/server/prot.go`

```go
OpDataLand     = Op{Opcode: 132, PayloadSize: -2}
OpDataLoc      = Op{Opcode: 220, PayloadSize: -2}
OpDataLandDone = Op{Opcode: 80,  PayloadSize: 2}
OpDataLocDone  = Op{Opcode: 20,  PayloadSize: 2}
```

Sizes verified against the Java client's `SERVERPROT_SIZES` table:
- opcode 132 → −2 ✓
- opcode 220 → −2 ✓
- opcode 80 → 2 ✓
- opcode 20 → 2 ✓

### 2. Constants + sender helpers — `modules/world/data_map.go`

```go
// rebuildGetMapsChunkSize is the max payload bytes per DATA_LAND/DATA_LOC
// packet. Derived from TS: 1000 (target packet size) - 1 (opcode byte)
// - 2 (length prefix) - 1 (mapX) - 1 (mapZ) - 2 (off) - 2 (totalLen) = 991.
const rebuildGetMapsChunkSize = 991

const (
	rebuildGetMapsLastBuildTicks = 10 // request is stale after 10 ticks
	rebuildGetMapsMapsLimit      = 18 // max 9 mapsquares × 2 file types
)

// sendDataLand writes one chunk of land data for the given mapsquare.
// Wire: p1(mapX) p1(mapZ) p2(off) p2(totalLen) pdata(chunk).
func sendDataLand(p *Player, mapX, mapZ, off, total int, chunk []byte) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	buf.P2(uint16(off))
	buf.P2(uint16(total))
	buf.PData(chunk)
	p.writeOut(gameserver.OpDataLand, buf.Bytes())
}

// sendDataLoc writes one chunk of loc data. Same wire format as sendDataLand.
func sendDataLoc(p *Player, mapX, mapZ, off, total int, chunk []byte) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	buf.P2(uint16(off))
	buf.P2(uint16(total))
	buf.PData(chunk)
	p.writeOut(gameserver.OpDataLoc, buf.Bytes())
}

// sendDataLandDone signals end-of-stream for one mapsquare's land file.
// Wire: p1(mapX) p1(mapZ).
func sendDataLandDone(p *Player, mapX, mapZ int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	p.writeOut(gameserver.OpDataLandDone, buf.Bytes())
}

// sendDataLocDone signals end-of-stream for one mapsquare's loc file.
func sendDataLocDone(p *Player, mapX, mapZ int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	p.writeOut(gameserver.OpDataLocDone, buf.Bytes())
}
```

### 3. Stream helpers — `modules/world/data_map.go`

```go
// streamLand chunks the full land file for (mapX, mapZ) into DATA_LAND
// packets followed by exactly one DATA_LAND_DONE. Silently no-ops if the
// mapsquare isn't loaded in the gamemap.
func streamLand(p *Player, gm *gamemap.GameMap, mapX, mapZ int) {
	data := gm.LandBytes(mapX, mapZ)
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

// streamLoc is the symmetric helper for DATA_LOC.
func streamLoc(p *Player, gm *gamemap.GameMap, mapX, mapZ int) {
	data := gm.LocBytes(mapX, mapZ)
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

### 4. Handler — `modules/world/data_map.go`

```go
// handleRebuildGetMaps services the client's request for a batch of m/l
// files. Each 3-byte entry in the payload is a packed (type, mapsquare)
// tuple: bit 16 selects land(0) vs loc(1); bits 0..15 are (mapX<<8)|mapZ.
// Entries that fail rate-limit, entry-count, or buildArea-membership
// checks are silently skipped (matches TS RebuildGetMapsHandler).
func handleRebuildGetMaps(p *Player, payload []byte) error {
	if p.buildArea == nil || p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server
	gm := s.gamemap
	if gm == nil {
		return nil
	}

	// Stale requests (past the 10-tick window after a rebuild) are dropped.
	if p.buildArea.LastBuild+rebuildGetMapsLastBuildTicks < s.currentTick {
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
		if !p.buildArea.Mapsquares[mapsquare] {
			continue
		}
		typ := (packed >> 16) & 0x1
		mapX := int(mapsquare>>8) & 0xFF
		mapZ := int(mapsquare) & 0xFF
		switch typ {
		case 0:
			streamLand(p, gm, mapX, mapZ)
		case 1:
			streamLoc(p, gm, mapX, mapZ)
		}
	}
	return nil
}
```

### 5. Handler registration — `modules/world/handlers_game.go`

In the existing `init()`:

```go
gameHandlers[150] = handleRebuildGetMaps // REBUILD_GETMAPS
```

## Data Flow

```
client sends: opcode 150, 1-byte length prefix N*3, payload = N × 3-byte entries
    │
    ▼
readPacket (dispatches decrypted opcode + stripped length prefix)
    │
    ▼ handleRebuildGetMaps
    ├─ nil-guard buildArea/client/server/gamemap
    ├─ rate-limit check (LastBuild + 10 >= currentTick)
    ├─ entry-count cap (≤ 18)
    └─ foreach entry (g3()):
         ├─ mapsquare = packed & 0xFFFF
         ├─ skip if !buildArea.Mapsquares[mapsquare]
         ├─ type = (packed >> 16) & 0x1
         └─ dispatch:
             type 0: streamLand → N × sendDataLand + 1 × sendDataLandDone
             type 1: streamLoc  → N × sendDataLoc  + 1 × sendDataLocDone
    │
    ▼ (bytes buffered in Player.client.bufw)
processClientsOut → flushWrite → wire
```

Handler runs in `processClientsIn`; bytes land on the wire in `processClientsOut` of the same tick (before `processCleanup`). One-tick latency is acceptable.

## Error Handling

- Nil-guards on `buildArea`, `client`, `client.server`, `s.gamemap` — any nil = silent drop.
- Missing `mapsquare` in `buildArea.Mapsquares` → skip that entry.
- `gm.LandBytes`/`LocBytes` returns nil → skip streaming (the client will time out and resend).
- Oversized request (> 18 entries) → drop entire request.
- Stale request (past rate-limit window) → drop entire request.

No error-response opcodes. The client's retry logic handles re-requesting if a round trip fails.

## Testing

All tests live in `modules/world/data_map_test.go`. Use the existing `newTestPlayer` + `drainConn` helpers from 4a.

### Wire-format tests

1. `TestSendDataLandWireFormat` — chunk `{0xAA, 0xBB}`, mapX=5, mapZ=6, off=100, total=1000 → expected bytes on the wire: `[encrypted_opcode, len_hi, len_lo, 5, 6, 0, 100, 3, 232, 0xAA, 0xBB]`.
2. `TestSendDataLocWireFormat` — symmetric for opcode 220.
3. `TestSendDataLandDoneFixedSize` — opcode 80 is fixed-2; expect `[encrypted_opcode, mapX, mapZ]` with NO length prefix.
4. `TestSendDataLocDoneFixedSize` — symmetric for opcode 20.

### Handler tests

5. `TestHandleRebuildGetMapsSingleChunk` — seed `gm.mData[key]` via gamemap with a small file (< 991 bytes); payload requests it; verify exactly 1 `DATA_LAND` + 1 `DATA_LAND_DONE` emitted.
6. `TestHandleRebuildGetMapsMultiChunk` — seed a 2500-byte file → expect 3 `DATA_LAND` (sizes 991, 991, 518) + 1 `DATA_LAND_DONE`. Assert offsets: 0, 991, 1982.
7. `TestHandleRebuildGetMapsExactlyChunkBoundary` — seed exactly 991 bytes → expect 1 `DATA_LAND` (size 991, off 0) + 1 `DATA_LAND_DONE`.
8. `TestHandleRebuildGetMapsRoutesToLoc` — payload with type bit = 1 → verify `DATA_LOC` + `DATA_LOC_DONE` emitted, NOT the Land variants.
9. `TestHandleRebuildGetMapsSkipsUnknownMapsquare` — payload requests a mapsquare NOT in `buildArea.Mapsquares` → 0 bytes on wire.
10. `TestHandleRebuildGetMapsSkipsMissingFile` — mapsquare IS in buildArea.Mapsquares but `gm.LandBytes` returns nil → 0 bytes (no done packet either).
11. `TestHandleRebuildGetMapsRateLimitedDropsEntireRequest` — `buildArea.LastBuild=0`, `s.currentTick=100` → 0 bytes even for valid entries.
12. `TestHandleRebuildGetMapsCapsAtEighteenEntries` — 19-entry payload → 0 bytes (whole request dropped).
13. `TestHandleRebuildGetMapsMultipleEntries` — payload with 2 valid entries (one land, one loc for different mapsquares) → both streamed in order, 2 done packets total.

## Acceptance Criteria

1. `go test ./...` passes.
2. `go vet ./...` clean.
3. `go test -race ./...` passes.
4. `gameHandlers[150]` is non-nil and routes to `handleRebuildGetMaps`.
5. All 4 new `Op{}` entries are present in `prot.go` with the verified sizes.
6. No changes outside `pkg/io/protocol/game/server/prot.go` and `modules/world/`.

## LOC Estimate

| File | LOC |
|---|---|
| `pkg/io/protocol/game/server/prot.go` | +6 |
| `modules/world/data_map.go` | ~120 |
| `modules/world/data_map_test.go` | ~240 |
| `modules/world/handlers_game.go` | +1 |
| **Total** | **~370** |

## Dependencies & Risks

- **Sub-spec 5a** — `GameMap.LandBytes`, `GameMap.LocBytes`, `BuildArea.LastBuild`.
- **No new external packages.**
- **No wire-format revision risk** — opcode sizes verified against the Java client's authoritative table (the 4b-2 lesson).
- **Risk: `PData` vs `PDataAlt1`** — chunk bytes go through `PData` (plain). Verified correct against rev-225 client (which uses plain `gdata` for payload bytes).
- **Risk: slow cold-start** — a fresh login triggers ~9 mapsquares × 2 files × ~50 chunks each ≈ 900 outbound packets in one tick. Acceptable for initial scene load; later sub-specs may want to throttle across ticks if profiling shows jank.

## Deferred

- **5c**: `writeFullFollows` Respawn-branch replay once scripts can mutate statics.
- NPC/obj file serving (`n{X}_{Z}`, `o{X}_{Z}`) — not part of RebuildGetMaps.
- LocType config + collision wiring.
