package world

import (
	"bytes"
	"net"
	"testing"
	"time"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func TestSendUpdateStatWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Expected wire:
	//   encrypted opcode = (44 + enc.GetNext()) & 0xff
	//   payload = p1(3) p4(100/10=10) p1(10) = [3, 0, 0, 0, 10, 10]
	want := []byte{
		byte((int(gameserver.OpUpdateStat.Opcode) + int(enc.GetNext())) & 0xff),
		3,
		0, 0, 0, 10,
		10,
	}

	received := drainConn(t, cc)
	sendUpdateStat(p, 3, 100, 10)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

func TestSendUpdateRunEnergyWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdateRunEnergy.Opcode) + int(enc.GetNext())) & 0xff),
		100, // 10000 / 100
	}

	received := drainConn(t, cc)
	sendUpdateRunEnergy(p, 10000)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// drainConn reads everything currently in the pipe. Must be called BEFORE flush.
func drainConn(t *testing.T, c net.Conn) <-chan []byte {
	t.Helper()
	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		c.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := c.Read(buf)
		received <- buf[:n]
	}()
	return received
}


func TestUpdateStatsFiresOnChange(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// Match all stats so only the target index diverges from sentinel.
	for i := 0; i < 21; i++ {
		p.lastStats[i] = p.stats[i]
		p.lastLevels[i] = p.levels[i]
	}
	p.lastRunEnergy = p.runenergy // isolate the stat loop from run-energy emission
	p.stats[3] = 100
	p.levels[3] = 10

	received := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()

	got := <-received
	if len(got) == 0 {
		t.Fatal("expected UpdateStat packet, got nothing")
	}

	// Second call: lastStats/lastLevels now match; should emit nothing.
	received2 := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()
	after := <-received2
	if len(after) != 0 {
		t.Errorf("second call should emit nothing; got %d bytes", len(after))
	}
}

func TestUpdateStatsRunEnergyCoarseGrain(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// All stats already match (isolate runenergy).
	for i := 0; i < 21; i++ {
		p.lastStats[i] = p.stats[i]
		p.lastLevels[i] = p.levels[i]
	}
	p.runenergy = 10000
	p.lastRunEnergy = -1 // default from newPlayer

	received := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()
	first := <-received
	if len(first) == 0 {
		t.Fatal("expected UpdateRunEnergy packet on first tick")
	}

	// Bump by 50: wire value (100) unchanged.
	p.runenergy = 10050
	received2 := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()
	quiet := <-received2
	if len(quiet) != 0 {
		t.Errorf("wire value unchanged; expected no packet, got %d bytes", len(quiet))
	}

	// Bump across boundary: wire value changes from 100 → 101.
	p.runenergy = 10100
	received3 := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()
	loud := <-received3
	if len(loud) == 0 {
		t.Error("wire value crossed boundary; expected packet, got nothing")
	}
}

func TestUpdateStatsFirstTickEmitsAll21(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// Fresh player: stats/levels all zero; last* sentinel -1/255.
	// runenergy = 10000, lastRunEnergy = -1.

	received := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()
	got := <-received
	// Each UpdateStat is 7 bytes (1 opcode + 6 payload). Plus 1 UpdateRunEnergy
	// (1 + 1 = 2 bytes). Total: 21*7 + 2 = 149.
	if len(got) != 149 {
		t.Errorf("first tick: got %d bytes, want 149 (21 stats + 1 runenergy)", len(got))
	}
}
