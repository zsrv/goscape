package world

import (
	"net"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// wirePacket holds a decrypted opcode and its raw payload bytes as read
// from the wire.
type wirePacket struct {
	opcode  int
	payload []byte
}

// captureCamWire launches a goroutine that reads from cc (with a 1-second
// deadline), then decrypts opcodes using the supplied parallel ISAAC decryptor
// and slices payloads by the fixed PayloadSize for each known cam op.
//
// The returned channel receives the slice of wirePackets once the goroutine
// drains all data. Must be called BEFORE the action that causes the write
// (net.Pipe is synchronous — both sides must be ready concurrently).
//
// dec must already be advanced past any pre-existing bytes that were already
// consumed by writeOut before this goroutine was launched. In practice
// newZoneTestPlayer emits no wire packets, so dec starts at stream offset 0.
func captureCamWire(t *testing.T, cc net.Conn, dec *io2.Isaac) <-chan []wirePacket {
	t.Helper()
	ch := make(chan []wirePacket, 1)
	go func() {
		buf := make([]byte, 4096)
		cc.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := cc.Read(buf)
		buf = buf[:n]

		var pkts []wirePacket
		pos := 0
		for pos < n {
			// Decrypt the opcode byte: same arithmetic as writeOut's encrypt.
			encByte := int(buf[pos])
			key := int(dec.GetNext() & 0xff)
			opcode := (encByte - key) & 0xff
			pos++

			var op gameserver.Op
			switch opcode {
			case int(gameserver.OpCamMoveTo.Opcode):
				op = gameserver.OpCamMoveTo
			case int(gameserver.OpCamLookAt.Opcode):
				op = gameserver.OpCamLookAt
			case int(gameserver.OpCamShake.Opcode):
				op = gameserver.OpCamShake
			case int(gameserver.OpCamReset.Opcode):
				op = gameserver.OpCamReset
			default:
				t.Errorf("captureCamWire: unexpected decrypted opcode %d (raw=0x%02x key=0x%02x) at byte pos %d (total n=%d)",
					opcode, encByte, key, pos-1, n)
				ch <- pkts
				return
			}

			if pos+op.PayloadSize > n {
				t.Errorf("captureCamWire: truncated payload: need %d bytes at pos %d but only %d bytes total",
					op.PayloadSize, pos, n)
				ch <- pkts
				return
			}
			payload := make([]byte, op.PayloadSize)
			copy(payload, buf[pos:pos+op.PayloadSize])
			pos += op.PayloadSize

			pkts = append(pkts, wirePacket{opcode: opcode, payload: payload})
		}
		ch <- pkts
	}()
	return ch
}

// TestUpdateBuildAreaCameraDrain_moveto (T5): one cameraPackets entry with
// kind=0 at (camX=300, camZ=400, height=550, rotationSpeed=80,
// rotationMultiplier=120) and player at (originX=296, originZ=392).
// rotationSpeed/rotationMultiplier use distinct values so a wire-format
// field-swap between adjacent p1 fields is detectable.
//
// After updateBuildArea():
//   - exactly one OpCamMoveTo (opcode 3) arrives on the wire
//   - payload bytes: [localX, localZ, height_hi, height_lo, rotSpeed, rotMul]
//     = [52, 56, 0x02, 0x26, 80, 120]
//   - p.cameraPackets is emptied (len==0)
//
// Wire math:
//
//	ZoneOrigin(296) = (296>>3 - 6)<<3 = (37-6)<<3 = 248; localX = 300-248 = 52
//	ZoneOrigin(392) = (392>>3 - 6)<<3 = (49-6)<<3 = 344; localZ = 400-344 = 56
//	height=550 = 0x0226 → big-endian bytes [0x02, 0x26]
func TestUpdateBuildAreaCameraDrain_moveto(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 296, 392, 0)

	// newZoneTestPlayer does not call writeOut (rebuildScenery/rebuildZones
	// write no wire packets), so dec starts at stream offset 0.
	dec := io2.New([4]uint32{1, 2, 3, 4}) // matches slot=1 encryptor seed

	// Arrange: one cam_moveto entry.
	p.cameraPackets = []cameraInfo{
		{kind: 0, camX: 300, camZ: 400, height: 550, rotationSpeed: 80, rotationMultiplier: 120},
	}

	// Launch reader goroutine BEFORE the write (net.Pipe is synchronous).
	received := captureCamWire(t, cc, dec)

	// Act.
	p.updateBuildArea()
	p.client.flushWrite()

	// Assert wire bytes.
	pkts := <-received
	if len(pkts) != 1 {
		t.Fatalf("expected 1 cam packet; got %d", len(pkts))
	}
	pkt := pkts[0]
	if pkt.opcode != int(gameserver.OpCamMoveTo.Opcode) {
		t.Errorf("opcode: got %d, want %d (OpCamMoveTo)", pkt.opcode, gameserver.OpCamMoveTo.Opcode)
	}

	wantZoneOriginX := coordgrid.ZoneOrigin(296)
	wantZoneOriginZ := coordgrid.ZoneOrigin(392)
	wantLocalX := byte(300 - wantZoneOriginX) // 52
	wantLocalZ := byte(400 - wantZoneOriginZ) // 56
	wantPayload := []byte{
		wantLocalX, wantLocalZ,
		0x02, 0x26, // height=550 big-endian
		80, 120, // rotationSpeed, rotationMultiplier (distinct)
	}
	if len(pkt.payload) != len(wantPayload) {
		t.Fatalf("payload length: got %d, want %d", len(pkt.payload), len(wantPayload))
	}
	for i, b := range wantPayload {
		if pkt.payload[i] != b {
			t.Errorf("payload[%d]: got 0x%02x, want 0x%02x", i, pkt.payload[i], b)
		}
	}

	// Assert accumulator cleared.
	if len(p.cameraPackets) != 0 {
		t.Errorf("cameraPackets len after drain: got %d, want 0", len(p.cameraPackets))
	}
}

// TestUpdateBuildAreaCameraDrain_lookatKind (T6): kind=1 → OpCamLookAt
// (opcode 74). Same payload shape as moveto; only the opcode byte differs.
func TestUpdateBuildAreaCameraDrain_lookatKind(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 296, 392, 0)

	dec := io2.New([4]uint32{1, 2, 3, 4})

	p.cameraPackets = []cameraInfo{
		{kind: 1, camX: 300, camZ: 400, height: 550, rotationSpeed: 100, rotationMultiplier: 100},
	}

	received := captureCamWire(t, cc, dec)

	p.updateBuildArea()
	p.client.flushWrite()

	pkts := <-received
	if len(pkts) != 1 {
		t.Fatalf("expected 1 cam packet; got %d", len(pkts))
	}
	if pkts[0].opcode != int(gameserver.OpCamLookAt.Opcode) {
		t.Errorf("opcode: got %d, want %d (OpCamLookAt)", pkts[0].opcode, gameserver.OpCamLookAt.Opcode)
	}
	if len(p.cameraPackets) != 0 {
		t.Errorf("cameraPackets len after drain: got %d, want 0", len(p.cameraPackets))
	}
}

// TestUpdateBuildAreaCameraDrain_originFreshness (T7): origin is mutated
// AFTER appending to cameraPackets, BEFORE calling updateBuildArea. The
// drain must read p.originX/Z at drain-time (not at append-time), so localX/Z
// are computed against the post-mutation origin.
//
//	originX 296→304: ZoneOrigin(304)=(38-6)<<3=256; localX=300-256=44
//	originZ 392→408: ZoneOrigin(408)=(51-6)<<3=360; localZ=400-360=40
func TestUpdateBuildAreaCameraDrain_originFreshness(t *testing.T) {
	s := newZoneTestServer(t)
	// Player initially at (296, 392).
	p, cc := newZoneTestPlayer(t, s, 1, 296, 392, 0)

	dec := io2.New([4]uint32{1, 2, 3, 4})

	// Append with old origin still in effect.
	p.cameraPackets = []cameraInfo{
		{kind: 0, camX: 300, camZ: 400, height: 100, rotationSpeed: 1, rotationMultiplier: 1},
	}

	// Mutate origin to simulate rebuildScenery having run after append.
	p.originX = 304
	p.originZ = 408

	received := captureCamWire(t, cc, dec)

	p.updateBuildArea()
	p.client.flushWrite()

	pkts := <-received
	if len(pkts) != 1 {
		t.Fatalf("expected 1 cam packet; got %d", len(pkts))
	}
	pkt := pkts[0]
	// ZoneOrigin(304) = (38-6)<<3 = 256; localX = 300-256 = 44
	// ZoneOrigin(408) = (51-6)<<3 = 360; localZ = 400-360 = 40
	gotLocalX := pkt.payload[0]
	gotLocalZ := pkt.payload[1]
	if gotLocalX != 44 {
		t.Errorf("localX: got %d, want 44 (origin 304 → ZoneOrigin 256)", gotLocalX)
	}
	if gotLocalZ != 40 {
		t.Errorf("localZ: got %d, want 40 (origin 408 → ZoneOrigin 360)", gotLocalZ)
	}
	if len(p.cameraPackets) != 0 {
		t.Errorf("cameraPackets len after drain: got %d, want 0", len(p.cameraPackets))
	}
}

// TestUpdateBuildAreaCameraThenZone (T8): cameraPackets is populated AND
// lastZone is set to a sentinel value (-1) so updateBuildArea will both drain
// camera packets AND run rebuildZones in the same call. After the call:
//   - p.cameraPackets is empty (drain completed)
//   - p.lastZone != -1 (zone transition recorded)
//
// This tests the ordering guarantee: cam drain fires first, then zone logic.
func TestUpdateBuildAreaCameraThenZone(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 296, 392, 0)

	// Force a zone transition by resetting lastZone to an impossible sentinel.
	// PackCoord(0, (296>>3)<<3, (392>>3)<<3) = actual zone; -1 is never that.
	p.lastZone = -1

	p.cameraPackets = []cameraInfo{
		{kind: 0, camX: 300, camZ: 400, height: 550, rotationSpeed: 100, rotationMultiplier: 100},
	}

	// Launch goroutine to consume write (net.Pipe is synchronous).
	// We don't assert wire bytes here — just state — but we must drain the pipe
	// to avoid blocking flushWrite. cc is needed for writeOut to not block.
	drainDone := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 4096)
		cc.SetReadDeadline(time.Now().Add(time.Second))
		cc.Read(buf)
		drainDone <- struct{}{}
	}()

	p.updateBuildArea()
	p.client.flushWrite()
	<-drainDone

	// Cam drain effect.
	if len(p.cameraPackets) != 0 {
		t.Errorf("cameraPackets len after updateBuildArea: got %d, want 0", len(p.cameraPackets))
	}

	// Zone effect: lastZone was updated away from -1.
	if p.lastZone == -1 {
		t.Error("lastZone still -1 after updateBuildArea; zone transition was not recorded")
	}
}
