package world

import (
	"bytes"
	"net"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
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

// TestSendUpdateRunWeightWireFormat pins the exact 3-byte wire encoding for
// sendUpdateRunWeight(p, 42): encrypted opcode (1 byte) + P2(42) (2 bytes).
// Mirrors TS UpdateRunWeightEncoder (`buf.p2(kg)`). NAI-136.
func TestSendUpdateRunWeightWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdateRunWeight.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x2a, // P2(42)
	}

	received := drainConn(t, cc)
	sendUpdateRunWeight(p, 42)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendUpdateRunWeight_LargeKg pins that kg=64 round-trips through P2 without
// truncation. NAI-136.
func TestSendUpdateRunWeight_LargeKg(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdateRunWeight.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x40, // P2(64)
	}

	received := drainConn(t, cc)
	sendUpdateRunWeight(p, 64)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendUpdateRunWeight_NegativeRoundsTowardZero pins Go integer division
// truncation toward zero for negative values. This is the divisor at the
// sendUpdateRunWeight call site (p.runweight/1000) and mirrors TS Math.trunc.
// In practice calculateRunWeight only sums non-negative weights, but the
// truncation parity with TS is worth an explicit pin. NAI-136 §7 R5.
func TestSendUpdateRunWeight_NegativeRoundsTowardZero(t *testing.T) {
	// Go: int / int truncates toward zero, same as TS Math.trunc.
	if -500/1000 != 0 {
		t.Errorf("-500/1000 = %d, want 0 (Go must truncate toward zero)", -500/1000)
	}
	if -1500/1000 != -1 {
		t.Errorf("-1500/1000 = %d, want -1", -1500/1000)
	}
}

// drainConn reads everything currently in the pipe. Must be called BEFORE flush.
//
// The read deadline only bounds the NO-DATA case: a no-op flush writes nothing,
// so the read waits the whole deadline out. When there IS data it arrives
// synchronously during flushWrite (the in-memory net.Pipe rendezvous is
// sub-millisecond), long before the deadline. Keep it short: ~120 no-data call
// sites × a 1s deadline added ~2 min to the world suite, tripping tight
// -timeout values; 100ms is ample margin and cuts that to ~12s.
func drainConn(t *testing.T, c net.Conn) <-chan []byte {
	t.Helper()
	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
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

// TestUpdateStats_RunEnergy_EmitsOnAnyChange pins TS-faithful fine-grained
// emit per the 2026-05-28 audit row player-net-5. TS NetworkPlayer.ts:330
// gate is `Math.floor(re)/100 !== Math.floor(lre)/100`; for integer re/lre
// that's `re !== lre`, so the packet emits on ANY internal run-energy
// change — even when the wire byte (re/100, see UpdateRunEnergyEncoder.ts)
// is unchanged. goscape pre-fix used `re/100 != lre/100` (int division),
// which suppressed same-wire-byte changes.
func TestUpdateStats_RunEnergy_EmitsOnAnyChange(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// Isolate the per-stat loop so only the run-energy gate can emit.
	for i := 0; i < 21; i++ {
		p.lastStats[i] = p.stats[i]
		p.lastLevels[i] = p.levels[i]
	}
	p.runenergy = 10000
	p.lastRunEnergy = -1 // default sentinel, distinct from runenergy

	// (1) First tick: re=10000 vs lre=-1 differ → emit.
	received := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()
	first := <-received
	if len(first) == 0 {
		t.Fatal("first tick: expected UpdateRunEnergy packet, got nothing")
	}

	// (2) Same-wire-byte bump (re 10000 → 10050; wire byte both = 100):
	// TS-faithful gate STILL emits because internal re changed.
	p.runenergy = 10050
	received2 := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()
	sameByte := <-received2
	if len(sameByte) == 0 {
		t.Error("same-wire-byte bump (10000→10050): expected UpdateRunEnergy packet (TS NetworkPlayer.ts:330 emits on any int re change), got nothing")
	}

	// (3) Cross-wire-byte bump (re 10050 → 10100; wire 100 → 101): emit.
	p.runenergy = 10100
	received3 := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()
	crossByte := <-received3
	if len(crossByte) == 0 {
		t.Error("cross-wire-byte bump (10050→10100): expected UpdateRunEnergy packet, got nothing")
	}

	// (4) No change (re 10100 → 10100): no emit.
	received4 := drainConn(t, cc)
	p.updateStats()
	p.client.flushWrite()
	quiet := <-received4
	if len(quiet) != 0 {
		t.Errorf("no change (10100→10100): expected NO packet, got %d bytes", len(quiet))
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
