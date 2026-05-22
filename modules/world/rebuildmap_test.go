package world

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/cache"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// seedCachedMapCRC writes m{mx}_{mz} and l{mx}_{mz} CRCs into the
// preload snapshot for the duration of the test. Mirrors NAI-16's
// seedCachedMidi pattern.
//
// Uses build-then-swap (read-copy-update) to extend the atomic.Pointer
// snapshot. Test-only; not safe for concurrent use.
func seedCachedMapCRC(t *testing.T, mx, mz int, mCRC, lCRC uint32) {
	t.Helper()
	mKey := fmt.Sprintf("m%d_%d", mx, mz)
	lKey := fmt.Sprintf("l%d_%d", mx, mz)
	prior := cache.Preload()
	next := &cache.PreloadSnapshot{
		Data: map[string][]byte{},
		CRC:  map[string]uint32{},
	}
	for k, v := range prior.Data {
		next.Data[k] = v
	}
	for k, v := range prior.CRC {
		next.CRC[k] = v
	}
	next.CRC[mKey] = mCRC
	next.CRC[lKey] = lCRC
	cache.SetPreloadForTest(next)
	t.Cleanup(cache.ResetPreloadForTest)
}

func TestSendRebuildNormalWireFormat(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.client.server = newTestServer(t)
	p.x = 3094
	p.z = 3106

	mapsquares := []uint16{uint16((48 << 8) | 48)}

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 17)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	sendRebuildNormal(p, mapsquares)
	p.client.flushWrite()

	expectedOpcode := byte((int(gameserver.OpRebuildNormal.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpcode {
			t.Errorf("opcode byte: got %d, want %d", got[0], expectedOpcode)
		}
		if got[1] != 0 || got[2] != 14 {
			t.Errorf("len prefix: got %v, want [0 14]", got[1:3])
		}
		if got[3] != 0x01 || got[4] != 0x82 {
			t.Errorf("zoneX: got %v, want [0x01 0x82]", got[3:5])
		}
		if got[5] != 0x01 || got[6] != 0x84 {
			t.Errorf("zoneZ: got %v, want [0x01 0x84]", got[5:7])
		}
		if got[7] != 48 || got[8] != 48 {
			t.Errorf("mapsquare: got (%d,%d), want (48,48)", got[7], got[8])
		}
	case <-time.After(time.Second):
		t.Error("timed out")
	}
}

// TestSendRebuildNormalReadsCacheCRC pins the positive-witness path:
// CRCs seeded into cache.PreloadedCRC appear in the encoded packet at
// the right offsets. Per TS RebuildNormalEncoder.ts:18-19.
func TestSendRebuildNormalReadsCacheCRC(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.client.server = newTestServer(t)
	p.x = 3094
	p.z = 3106

	seedCachedMapCRC(t, 48, 48, 0xDEADBEEF, 0xCAFEBABE)
	mapsquares := []uint16{uint16((48 << 8) | 48)}

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 17)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	sendRebuildNormal(p, mapsquares)
	p.client.flushWrite()

	expectedOpcode := byte((int(gameserver.OpRebuildNormal.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpcode {
			t.Errorf("opcode byte: got %d, want %d", got[0], expectedOpcode)
		}
		// Bytes 9-12: mCRC big-endian (0xDEADBEEF)
		if got[9] != 0xDE || got[10] != 0xAD || got[11] != 0xBE || got[12] != 0xEF {
			t.Errorf("mCRC: got %v, want [0xDE 0xAD 0xBE 0xEF]", got[9:13])
		}
		// Bytes 13-16: lCRC big-endian (0xCAFEBABE)
		if got[13] != 0xCA || got[14] != 0xFE || got[15] != 0xBA || got[16] != 0xBE {
			t.Errorf("lCRC: got %v, want [0xCA 0xFE 0xBA 0xBE]", got[13:17])
		}
	case <-time.After(time.Second):
		t.Error("timed out")
	}
}
