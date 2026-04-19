package world

import (
	"io"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/gamemap"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestLoginSendsRebuildNormal verifies that updateMap() sends a RebuildNormal
// packet on first call (when buildArea.OriginX is -1).
func TestLoginSendsRebuildNormal(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	p, clientConn := newTestPlayer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.buildArea = buildarea.New()

	received := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf[0]
		}
	}()

	p.updateMap()
	p.client.flushWrite()

	wantOp := byte((int(gameserver.OpRebuildNormal.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got != wantOp {
			t.Errorf("first byte: got %d, want %d (encrypted RebuildNormal opcode)", got, wantOp)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for RebuildNormal")
	}
}
