# Sub-spec 5b: Map Data Serving — Implementation Plan

> **For agentic workers:** Compact 2-task plan. Sub-spec is self-contained and <400 LOC; inline execution is appropriate.

**Goal:** Handler for client opcode 150 that chunks raw map bytes via 4 new outbound opcodes.

**Architecture:** One new file, one new handler registration, 4 new `Op{}` entries.

**Build prefix:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
**Commit flag:** `--no-gpg-sign`.
**Spec reference:** `docs/superpowers/specs/2026-04-20-map-data-serving-design.md`.

---

## Task 1: 4 outer opcodes + all senders + handler + tests

Single bundled task because the file is small and all pieces exercise the same fixture surface.

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`
- Create: `modules/world/data_map.go`
- Create: `modules/world/data_map_test.go`
- Modify: `modules/world/handlers_game.go`

- [ ] **Step 1.1: Add 4 `Op{}` entries to prot.go**

In the existing `var (...)` block in `pkg/io/protocol/game/server/prot.go`, after the most recent opcode additions:

```go
OpDataLand     = Op{Opcode: 132, PayloadSize: -2}
OpDataLoc      = Op{Opcode: 220, PayloadSize: -2}
OpDataLandDone = Op{Opcode: 80, PayloadSize: 2}
OpDataLocDone  = Op{Opcode: 20, PayloadSize: 2}
```

- [ ] **Step 1.2: Run build + existing tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```
Expected: PASS.

- [ ] **Step 1.3: Create `modules/world/data_map.go`**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// rebuildGetMapsChunkSize is the max payload bytes per DATA_LAND/DATA_LOC
// packet. 1000 - opcode(1) - length prefix(2) - mapX(1) - mapZ(1) - off(2)
// - totalLen(2) = 991.
const rebuildGetMapsChunkSize = 991

const (
	rebuildGetMapsLastBuildTicks = 10 // request is stale after 10 ticks
	rebuildGetMapsMapsLimit      = 18 // 9 mapsquares × 2 file types
)

// sendDataLand writes one chunk of land data for (mapX, mapZ).
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

func sendDataLoc(p *Player, mapX, mapZ, off, total int, chunk []byte) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	buf.P2(uint16(off))
	buf.P2(uint16(total))
	buf.PData(chunk)
	p.writeOut(gameserver.OpDataLoc, buf.Bytes())
}

func sendDataLandDone(p *Player, mapX, mapZ int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	p.writeOut(gameserver.OpDataLandDone, buf.Bytes())
}

func sendDataLocDone(p *Player, mapX, mapZ int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	p.writeOut(gameserver.OpDataLocDone, buf.Bytes())
}

// streamLand chunks the land file for (mapX, mapZ) into DATA_LAND packets
// followed by exactly one DATA_LAND_DONE. Silent no-op if unloaded.
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

// handleRebuildGetMaps services the client's request (opcode 150) for a
// batch of m/l files. Each 3-byte entry is a packed (type, mapsquare)
// tuple: bit 16 = type (0=land, 1=loc); bits 0..15 = (mapX<<8)|mapZ.
//
// Validation matches TS RebuildGetMapsHandler:
//   - Reject (silently) if buildArea.LastBuild + 10 < currentTick (stale).
//   - Reject (silently) if entries > MAPS_LIMIT (18).
//   - Skip per-entry if mapsquare not in buildArea.Mapsquares.
//   - Skip per-entry if GameMap has no bytes for that file.
//
// No error-response opcodes are sent — clients retry on their own.
func handleRebuildGetMaps(p *Player, payload []byte) error {
	if p.buildArea == nil || p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server
	gm := s.gamemap
	if gm == nil {
		return nil
	}

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

- [ ] **Step 1.4: Register handler in `modules/world/handlers_game.go`**

In the existing `init()`, add:

```go
gameHandlers[150] = handleRebuildGetMaps // REBUILD_GETMAPS
```

- [ ] **Step 1.5: Build**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```
Expected: exits 0.

- [ ] **Step 1.6: Commit (impl only)**

```bash
git add pkg/io/protocol/game/server/prot.go modules/world/data_map.go modules/world/handlers_game.go
git commit --no-gpg-sign -m "feat(world): serve mapsquare data via REBUILD_GETMAPS

Handler for client opcode 150 that iterates the packed (type, mapsquare)
entries in the payload and chunks the raw m/l bytes from GameMap via 4
new outbound opcodes:

  DATA_LAND     132 (-2)  p1 mapX, p1 mapZ, p2 off, p2 totalLen, pdata chunk
  DATA_LOC      220 (-2)  same format
  DATA_LAND_DONE 80 ( 2)  p1 mapX, p1 mapZ
  DATA_LOC_DONE  20 ( 2)  p1 mapX, p1 mapZ

Chunk size = 991 bytes per DATA_* packet (1000 target - 9 overhead).

Validation lifted from TS RebuildGetMapsHandler: drop stale requests
(beyond 10 ticks past last BuildArea rebuild), cap entries at 18, skip
unknown mapsquares and unloaded files. Silent drops match TS — no
error-response opcodes."
```

---

## Task 2: Tests

**Files:**
- Create: `modules/world/data_map_test.go`

- [ ] **Step 2.1: Write wire-format tests**

Create `modules/world/data_map_test.go`:

```go
package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// newMapDataPlayer wires a Player into a Server with a gamemap and buildArea
// ready to service RebuildGetMaps.
func newMapDataPlayer(t *testing.T) (*Player, net.Conn, *Server) {
	t.Helper()
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.currentTick = 5 // well within rate-limit window

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = isaac.New([4]uint32{1, 2, 3, 4})
	p.buildArea = buildarea.New()
	p.buildArea.LastBuild = 0 // 5 - 0 = 5 ticks < 10 → in-window
	return p, cc, s
}

func TestSendDataLandWireFormat(t *testing.T) {
	p, cc, _ := newMapDataPlayer(t)
	enc := isaac.New([4]uint32{1, 2, 3, 4})

	chunk := []byte{0xAA, 0xBB}
	want := []byte{
		byte((int(gameserver.OpDataLand.Opcode) + int(enc.GetNext())) & 0xff),
		0, 7, // length prefix: 2-byte big-endian = 7
		5, 6, // mapX, mapZ
		0, 100, // off = 100
		3, 232, // total = 1000
		0xAA, 0xBB,
	}

	received := drainConn(t, cc)
	sendDataLand(p, 5, 6, 100, 1000, chunk)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

func TestSendDataLocWireFormat(t *testing.T) {
	p, cc, _ := newMapDataPlayer(t)
	enc := isaac.New([4]uint32{1, 2, 3, 4})
	chunk := []byte{0xCC}
	want := []byte{
		byte((int(gameserver.OpDataLoc.Opcode) + int(enc.GetNext())) & 0xff),
		0, 6, // length prefix
		5, 6,
		0, 50,
		1, 244, // total = 500
		0xCC,
	}
	received := drainConn(t, cc)
	sendDataLoc(p, 5, 6, 50, 500, chunk)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

func TestSendDataLandDoneFixedSize(t *testing.T) {
	p, cc, _ := newMapDataPlayer(t)
	enc := isaac.New([4]uint32{1, 2, 3, 4})
	want := []byte{
		byte((int(gameserver.OpDataLandDone.Opcode) + int(enc.GetNext())) & 0xff),
		7, 8, // mapX, mapZ (no length prefix — fixed 2)
	}
	received := drainConn(t, cc)
	sendDataLandDone(p, 7, 8)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

func TestSendDataLocDoneFixedSize(t *testing.T) {
	p, cc, _ := newMapDataPlayer(t)
	enc := isaac.New([4]uint32{1, 2, 3, 4})
	want := []byte{
		byte((int(gameserver.OpDataLocDone.Opcode) + int(enc.GetNext())) & 0xff),
		7, 8,
	}
	received := drainConn(t, cc)
	sendDataLocDone(p, 7, 8)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// Handler tests — each seeds a small synthetic file in the gamemap and
// issues a REBUILD_GETMAPS payload to verify routing / chunking / validation.

func buildGetMapsPayload(entries ...uint32) []byte {
	buf := make([]byte, 0, len(entries)*3)
	for _, e := range entries {
		// g3 is big-endian, high byte first.
		buf = append(buf, byte(e>>16), byte(e>>8), byte(e))
	}
	return buf
}

func packEntry(typ, mapX, mapZ int) uint32 {
	return uint32((typ << 16) | (mapX << 8) | mapZ)
}

func TestHandleRebuildGetMapsSingleChunk(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	s.gamemap.AddStaticLocRawLandBytesForTest(50, 51, []byte{0x11, 0x22}) // 2-byte file
	p.buildArea.Mapsquares[uint16((50<<8)|51)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 50, 51)))
	p.client.flushWrite()
	got := <-received

	// Expect: 1 × DATA_LAND (opcode + 2-byte len + 8 bytes header+payload) + 1 × DATA_LAND_DONE (opcode + 2 bytes)
	// DATA_LAND size = 8 bytes payload (1+1+2+2+2 = 8). Total on wire per packet:
	//   1 (opcode) + 2 (len prefix) + 8 (payload) = 11 bytes
	// DATA_LAND_DONE: 1 (opcode) + 2 (payload) = 3 bytes
	// Total = 14.
	if len(got) != 14 {
		t.Errorf("got %d bytes, want 14; bytes=%v", len(got), got)
	}
}

func TestHandleRebuildGetMapsMultiChunk(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	file := make([]byte, 2500) // spans 3 chunks: 991 + 991 + 518
	for i := range file {
		file[i] = byte(i)
	}
	s.gamemap.AddStaticLocRawLandBytesForTest(10, 20, file)
	p.buildArea.Mapsquares[uint16((10<<8)|20)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 10, 20)))
	p.client.flushWrite()
	got := <-received

	// 3 × DATA_LAND: each has 1 (op) + 2 (len) + 8 (header) + chunk bytes.
	//   chunk 1: 991 bytes → packet size 1+2+8+991 = 1002
	//   chunk 2: 991 bytes → 1002
	//   chunk 3: 518 bytes → 1+2+8+518 = 529
	// + 1 × DATA_LAND_DONE: 3 bytes
	// Total = 1002 + 1002 + 529 + 3 = 2536.
	if len(got) != 2536 {
		t.Errorf("got %d bytes, want 2536", len(got))
	}
}

func TestHandleRebuildGetMapsExactlyChunkBoundary(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	file := make([]byte, 991)
	s.gamemap.AddStaticLocRawLandBytesForTest(1, 2, file)
	p.buildArea.Mapsquares[uint16((1<<8)|2)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 1, 2)))
	p.client.flushWrite()
	got := <-received

	// 1 × DATA_LAND (1+2+8+991=1002) + 1 × DATA_LAND_DONE (3) = 1005.
	if len(got) != 1005 {
		t.Errorf("got %d bytes, want 1005", len(got))
	}
}

func TestHandleRebuildGetMapsRoutesToLoc(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	s.gamemap.AddStaticLocRawLocBytesForTest(3, 4, []byte{0xEE})
	p.buildArea.Mapsquares[uint16((3<<8)|4)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(1, 3, 4)))
	p.client.flushWrite()
	got := <-received

	// Expect opcodes 220 + 20, NOT 132 + 80.
	// Parsing encrypted opcode back is brittle; instead assert byte length:
	// 1 DATA_LOC (1+2+8+1=12) + 1 DATA_LOC_DONE (3) = 15.
	if len(got) != 15 {
		t.Errorf("got %d bytes, want 15; bytes=%v", len(got), got)
	}
}

func TestHandleRebuildGetMapsSkipsUnknownMapsquare(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	s.gamemap.AddStaticLocRawLandBytesForTest(50, 51, []byte{0x11})
	// buildArea does NOT include this mapsquare.

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 50, 51)))
	p.client.flushWrite()
	got := <-received

	if len(got) != 0 {
		t.Errorf("unknown mapsquare should produce 0 bytes; got %d", len(got))
	}
}

func TestHandleRebuildGetMapsSkipsMissingFile(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	// gamemap has NO file for this mapsquare.
	p.buildArea.Mapsquares[uint16((99<<8)|99)] = true
	_ = s

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 99, 99)))
	p.client.flushWrite()
	got := <-received

	if len(got) != 0 {
		t.Errorf("missing file should produce 0 bytes (no done packet either); got %d", len(got))
	}
}

func TestHandleRebuildGetMapsRateLimitedDropsEntireRequest(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	s.currentTick = 100
	p.buildArea.LastBuild = 0 // 100 - 0 = 100 > 10 → stale
	s.gamemap.AddStaticLocRawLandBytesForTest(50, 51, []byte{0x11})
	p.buildArea.Mapsquares[uint16((50<<8)|51)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 50, 51)))
	p.client.flushWrite()
	got := <-received

	if len(got) != 0 {
		t.Errorf("stale request should produce 0 bytes; got %d", len(got))
	}
}

func TestHandleRebuildGetMapsCapsAtEighteenEntries(t *testing.T) {
	p, cc, _ := newMapDataPlayer(t)
	// 19 entries → reject. No need to seed files.
	entries := make([]uint32, 19)
	for i := range entries {
		entries[i] = packEntry(0, 50, 51)
	}

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(entries...))
	p.client.flushWrite()
	got := <-received

	if len(got) != 0 {
		t.Errorf("oversized request should produce 0 bytes; got %d", len(got))
	}
}

func TestHandleRebuildGetMapsMultipleEntries(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	s.gamemap.AddStaticLocRawLandBytesForTest(1, 1, []byte{0xAA})
	s.gamemap.AddStaticLocRawLocBytesForTest(2, 2, []byte{0xBB})
	p.buildArea.Mapsquares[uint16((1<<8)|1)] = true
	p.buildArea.Mapsquares[uint16((2<<8)|2)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(
		packEntry(0, 1, 1), // land
		packEntry(1, 2, 2), // loc
	))
	p.client.flushWrite()
	got := <-received

	// 1 DATA_LAND (1+2+8+1=12) + 1 DATA_LAND_DONE (3) +
	// 1 DATA_LOC (1+2+8+1=12) + 1 DATA_LOC_DONE (3) = 30.
	if len(got) != 30 {
		t.Errorf("2 entries should produce 30 bytes; got %d", len(got))
	}
}
```

Tests require `net` import too (for `net.Conn` in helper return type). The helper also uses `AddStaticLocRawLandBytesForTest` / `AddStaticLocRawLocBytesForTest` — we'll need to add those tiny test-seeder methods on `GameMap`.

- [ ] **Step 2.2: Add test-seed helpers on `GameMap`**

In `pkg/gamemap/gamemap.go`, add:

```go
// AddStaticLocRawLandBytesForTest seeds raw m{mapX}_{mapZ} bytes for
// tests that want to exercise serving without real cache files.
func (gm *GameMap) AddStaticLocRawLandBytesForTest(mapX, mapZ int, b []byte) {
	gm.mData[uint16((mapX<<8)|mapZ)] = b
}

// AddStaticLocRawLocBytesForTest seeds raw l{mapX}_{mapZ} bytes for tests.
func (gm *GameMap) AddStaticLocRawLocBytesForTest(mapX, mapZ int, b []byte) {
	gm.lData[uint16((mapX<<8)|mapZ)] = b
}
```

(Exported despite the `ForTest` suffix because they cross package boundaries — test-only usage documented via comment.)

- [ ] **Step 2.3: Import fix for the test file**

Ensure `modules/world/data_map_test.go` imports include `"net"`.

- [ ] **Step 2.4: Run tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run '^(TestSend|TestHandle)Data|^(TestSend|TestHandle)RebuildGetMaps' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```
Expected: all green.

- [ ] **Step 2.5: Commit**

```bash
git add modules/world/data_map_test.go pkg/gamemap/gamemap.go
git commit --no-gpg-sign -m "test(world): cover REBUILD_GETMAPS handler + data senders

13 tests: 4 wire-format assertions for the 4 new senders, plus handler
coverage for single-chunk/multi-chunk/exact-boundary routing, land-vs-loc
dispatch, unknown-mapsquare skip, missing-file skip, rate-limit drop,
entry-count cap, and multi-entry ordering.

Adds AddStaticLocRawLandBytesForTest / LocBytesForTest on GameMap so
tests can seed raw mapsquare bytes without real cache files."
```

---

## Final Verification

- [ ] `go test -race ./...` — PASS
- [ ] `go vet ./...` — clean
- [ ] `grep 'gameHandlers\[150\]' modules/world/handlers_game.go` — non-empty
- [ ] `grep 'OpDataLand\b' pkg/io/protocol/game/server/prot.go` — non-empty

## Spec Coverage

| Spec item | Task |
|---|---|
| 4 `Op{}` entries | Task 1 |
| 4 sender helpers | Task 1 |
| `streamLand` / `streamLoc` | Task 1 |
| `handleRebuildGetMaps` | Task 1 |
| Handler registration | Task 1 |
| `rebuildGetMapsChunkSize`, limits | Task 1 |
| Wire-format tests | Task 2 |
| Handler tests (8 scenarios) | Task 2 |
| Test-seed helpers on GameMap | Task 2 |
| Acceptance (tests + vet + race) | Final |
