package world

import (
	"bytes"
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/gamemap"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// newMapDataPlayer wires a Player into a Server with a gamemap and
// buildArea ready to service RebuildGetMaps. Returns the player, the
// client-side pipe, and the server.
func newMapDataPlayer(t *testing.T) (*Player, net.Conn, *Server) {
	t.Helper()
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.currentTick = 5 // well within rate-limit window

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.buildArea = buildarea.New()
	p.buildArea.LastBuild = 0 // 5 - 0 = 5 < 10 -> in-window
	return p, cc, s
}

func TestSendDataLandWireFormat(t *testing.T) {
	p, cc, _ := newMapDataPlayer(t)
	enc := io2.New([4]uint32{1, 2, 3, 4})

	chunk := []byte{0xAA, 0xBB}
	// DATA_LAND payload: p1(mapX=5), p1(mapZ=6), p2(off=100), p2(total=1000),
	// pdata([0xAA, 0xBB]) = 2+2+2+2 = 8 bytes total payload.
	// Wire: encrypted_opcode, len_hi=0, len_lo=8, then payload bytes.
	want := []byte{
		byte((int(gameserver.OpDataLand.Opcode) + int(enc.GetNext())) & 0xff),
		0, 8,
		5, 6,
		0, 100,
		3, 232, // 1000 big-endian
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
	enc := io2.New([4]uint32{1, 2, 3, 4})
	chunk := []byte{0xCC}
	// Payload = 1+1+2+2+1 = 7 bytes.
	want := []byte{
		byte((int(gameserver.OpDataLoc.Opcode) + int(enc.GetNext())) & 0xff),
		0, 7,
		5, 6,
		0, 50,
		1, 244, // 500 big-endian
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
	enc := io2.New([4]uint32{1, 2, 3, 4})
	want := []byte{
		byte((int(gameserver.OpDataLandDone.Opcode) + int(enc.GetNext())) & 0xff),
		7, 8, // mapX, mapZ (no length prefix - fixed 2)
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
	enc := io2.New([4]uint32{1, 2, 3, 4})
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

func buildGetMapsPayload(entries ...uint32) []byte {
	buf := make([]byte, 0, len(entries)*3)
	for _, e := range entries {
		buf = append(buf, byte(e>>16), byte(e>>8), byte(e))
	}
	return buf
}

func packEntry(typ, mapX, mapZ int) uint32 {
	return uint32((typ << 16) | (mapX << 8) | mapZ)
}

func TestHandleRebuildGetMapsSingleChunk(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	s.gamemap.SetLandBytesForTest(50, 51, []byte{0x11, 0x22})
	p.buildArea.Mapsquares[uint16((50<<8)|51)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 50, 51)))
	p.client.flushWrite()
	got := <-received

	// Expected:
	//   DATA_LAND: opcode(1) + len_prefix(2) + payload(1+1+2+2+2=8) = 11 bytes
	//   DATA_LAND_DONE: opcode(1) + payload(2) = 3 bytes (fixed, no len prefix)
	// Total = 14.
	if len(got) != 14 {
		t.Errorf("got %d bytes, want 14; bytes=%v", len(got), got)
	}
}

func TestHandleRebuildGetMapsMultiChunk(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	file := make([]byte, 2500) // 991 + 991 + 518
	for i := range file {
		file[i] = byte(i)
	}
	s.gamemap.SetLandBytesForTest(10, 20, file)
	p.buildArea.Mapsquares[uint16((10<<8)|20)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 10, 20)))
	p.client.flushWrite()
	got := <-received

	// 3 DATA_LAND packets:
	//   chunk 1: opcode(1) + len(2) + header(6) + 991 = 1000
	//   chunk 2: same = 1000
	//   chunk 3: opcode(1) + len(2) + header(6) + 518 = 527
	// + DATA_LAND_DONE: opcode(1) + 2 = 3
	// Total = 1000 + 1000 + 527 + 3 = 2530.
	if len(got) != 2530 {
		t.Errorf("got %d bytes, want 2530", len(got))
	}
}

func TestHandleRebuildGetMapsExactlyChunkBoundary(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	file := make([]byte, 991)
	s.gamemap.SetLandBytesForTest(1, 2, file)
	p.buildArea.Mapsquares[uint16((1<<8)|2)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 1, 2)))
	p.client.flushWrite()
	got := <-received

	// 1 DATA_LAND (1+2+6+991 = 1000) + 1 DATA_LAND_DONE (3) = 1003.
	if len(got) != 1003 {
		t.Errorf("got %d bytes, want 1003", len(got))
	}
}

func TestHandleRebuildGetMapsRoutesToLoc(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	s.gamemap.SetLocBytesForTest(3, 4, []byte{0xEE})
	p.buildArea.Mapsquares[uint16((3<<8)|4)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(1, 3, 4)))
	p.client.flushWrite()
	got := <-received

	// 1 DATA_LOC (1+2+6+1 = 10) + 1 DATA_LOC_DONE (3) = 13.
	if len(got) != 13 {
		t.Errorf("got %d bytes, want 13", len(got))
	}
}

func TestHandleRebuildGetMapsSkipsUnknownMapsquare(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	s.gamemap.SetLandBytesForTest(50, 51, []byte{0x11})
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
	p, cc, _ := newMapDataPlayer(t)
	p.buildArea.Mapsquares[uint16((99<<8)|99)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(packEntry(0, 99, 99)))
	p.client.flushWrite()
	got := <-received

	if len(got) != 0 {
		t.Errorf("missing file should produce 0 bytes; got %d", len(got))
	}
}

func TestHandleRebuildGetMapsRateLimitedDropsEntireRequest(t *testing.T) {
	p, cc, s := newMapDataPlayer(t)
	s.currentTick = 100
	p.buildArea.LastBuild = 0 // 100 > 10 -> stale
	s.gamemap.SetLandBytesForTest(50, 51, []byte{0x11})
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
	s.gamemap.SetLandBytesForTest(1, 1, []byte{0xAA})
	s.gamemap.SetLocBytesForTest(2, 2, []byte{0xBB})
	p.buildArea.Mapsquares[uint16((1<<8)|1)] = true
	p.buildArea.Mapsquares[uint16((2<<8)|2)] = true

	received := drainConn(t, cc)
	_ = handleRebuildGetMaps(p, buildGetMapsPayload(
		packEntry(0, 1, 1), // land
		packEntry(1, 2, 2), // loc
	))
	p.client.flushWrite()
	got := <-received

	// 1 DATA_LAND (10) + 1 DATA_LAND_DONE (3) + 1 DATA_LOC (10) + 1 DATA_LOC_DONE (3) = 26.
	if len(got) != 26 {
		t.Errorf("2 entries should produce 26 bytes; got %d", len(got))
	}
}
