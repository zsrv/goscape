package world

import (
	"io"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// TestOnReconnect_EmitsResyncSequence pins the opcode order emitted by
// onReconnect: RESET_CLIENT_VARCACHE → 21 × UPDATE_STAT → UPDATE_RUN_ENERGY
// → RESET_ANIMS. NAI-182 B4.
func TestOnReconnect_EmitsResyncSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	onReconnect(s, p)
	p.client.flushWrite()

	got := <-received

	// Walk the byte stream opcode by opcode.
	offset := 0

	// (a) RESET_CLIENT_VARCACHE — 1 byte (opcode only, no payload).
	if len(got) < offset+1 {
		t.Fatalf("stream too short at RESET_CLIENT_VARCACHE: got %d bytes", len(got))
	}
	wantRCV := byte((int(gameserver.OpResetClientVarCache.Opcode) + int(enc.GetNext())) & 0xff)
	if got[offset] != wantRCV {
		t.Errorf("byte[%d] RESET_CLIENT_VARCACHE opcode: got 0x%02x, want 0x%02x", offset, got[offset], wantRCV)
	}
	offset++

	// (b) No varps in newTestServer, so varp loop emits nothing.

	// (c) No shutdown timer (shutdownTick == -1 by default), so no UPDATE_REBOOT_TIMER.

	// (d) closeModal(true) — clears weakQueue then early-returns on
	// modalState==None (default on newTestPlayer); no wire packet.

	// (e) Tabs: newTestPlayer initializes tabs to all -1 (non-zero), so
	// IfSetTab is called for each of the 14 tabs.
	// Each IF_SETTAB packet: opcode(1) + P2(com)(2) + P1(tab)(1) = 4 bytes.
	for tab := range 14 {
		if len(got) < offset+4 {
			t.Fatalf("stream too short at IF_SETTAB tab=%d: got %d bytes, offset %d", tab, len(got), offset)
		}
		wantTabOpcode := byte((int(gameserver.OpIfSetTab.Opcode) + int(enc.GetNext())) & 0xff)
		if got[offset] != wantTabOpcode {
			t.Errorf("byte[%d] IF_SETTAB[%d] opcode: got 0x%02x, want 0x%02x", offset, tab, got[offset], wantTabOpcode)
		}
		offset += 4 // opcode(1) + com P2(2) + tab P1(1)
	}

	// (f) No invListeners in newTestPlayer, so no packets from refreshInvs.

	// (g) 21 × UPDATE_STAT — 7 bytes each: opcode(1) + stat(1) + exp/10 P4(4) + level(1).
	for i := range 21 {
		if len(got) < offset+7 {
			t.Fatalf("stream too short at UPDATE_STAT[%d]: got %d bytes, offset %d", i, len(got), offset)
		}
		wantStatOpcode := byte((int(gameserver.OpUpdateStat.Opcode) + int(enc.GetNext())) & 0xff)
		if got[offset] != wantStatOpcode {
			t.Errorf("byte[%d] UPDATE_STAT[%d] opcode: got 0x%02x, want 0x%02x", offset, i, got[offset], wantStatOpcode)
		}
		offset += 7
	}

	// (h) UPDATE_RUN_ENERGY — 2 bytes: opcode(1) + energy/100 P1(1).
	if len(got) < offset+2 {
		t.Fatalf("stream too short at UPDATE_RUN_ENERGY: got %d bytes, offset %d", len(got), offset)
	}
	wantRunEnergy := byte((int(gameserver.OpUpdateRunEnergy.Opcode) + int(enc.GetNext())) & 0xff)
	if got[offset] != wantRunEnergy {
		t.Errorf("byte[%d] UPDATE_RUN_ENERGY opcode: got 0x%02x, want 0x%02x", offset, got[offset], wantRunEnergy)
	}
	offset += 2

	// (i) RESET_ANIMS — 1 byte: opcode only.
	if len(got) < offset+1 {
		t.Fatalf("stream too short at RESET_ANIMS: got %d bytes, offset %d", len(got), offset)
	}
	wantRA := byte((int(gameserver.OpResetAnims.Opcode) + int(enc.GetNext())) & 0xff)
	if got[offset] != wantRA {
		t.Errorf("byte[%d] RESET_ANIMS opcode: got 0x%02x, want 0x%02x", offset, got[offset], wantRA)
	}
	offset++

	// Verify we consumed exactly the bytes that were sent.
	if len(got) != offset {
		t.Errorf("stream length: got %d bytes, consumed %d — unexpected trailing bytes", len(got), offset)
	}
}

// TestOnReconnect_FlipsAllInvListenerFirstSeenToTrue verifies that
// onReconnect resets FirstSeen=true on every invListener entry so the next
// updateInvs tick re-emits each as UpdateInvFull. NAI-182 B4.
func TestOnReconnect_FlipsAllInvListenerFirstSeenToTrue(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, cc)

	p.invListeners = map[int]InventoryListener{
		100: {Com: 100, Type: 0, Source: p.uid, FirstSeen: false},
		101: {Com: 101, Type: 1, Source: p.uid, FirstSeen: false},
		102: {Com: 102, Type: -1, Source: -1, FirstSeen: false},
	}

	onReconnect(s, p)

	for com, l := range p.invListeners {
		if !l.FirstSeen {
			t.Errorf("invListeners[%d].FirstSeen: got false, want true", com)
		}
	}
}

// TestOnReconnect_ResetsMoveSpeedToInstant pins that onReconnect forces
// p.moveSpeed back to MoveSpeedInstant regardless of the pre-reconnect
// value. Mirrors TS Player.onReconnect (Player.ts:556 —
// `this.moveSpeed = MoveSpeed.INSTANT`).
func TestOnReconnect_ResetsMoveSpeedToInstant(t *testing.T) {
	for _, pre := range []MoveSpeed{MoveSpeedStationary, MoveSpeedWalk, MoveSpeedRun} {
		p, cc := newTestPlayer(t)
		s := newTestServer(t)
		p.client.server = s
		p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
		go io.Copy(io.Discard, cc)

		p.moveSpeed = pre

		onReconnect(s, p)

		if p.moveSpeed != MoveSpeedInstant {
			t.Errorf("pre=%v: p.moveSpeed: got %v, want MoveSpeedInstant", pre, p.moveSpeed)
		}
	}
}

// TestOnReconnect_OrsEntityMaskIntoMasks verifies that onReconnect ORs
// p.entitymask into p.masks so face_entity resync is triggered on the next
// mask block. Mirrors TS Player.onReconnect (Player.ts:574). NAI-182 B4.
func TestOnReconnect_OrsEntityMaskIntoMasks(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, cc)

	p.entitymask = 0x80
	p.masks = 0x01

	onReconnect(s, p)

	if p.masks&0x80 == 0 {
		t.Errorf("p.masks: entitymask bit 0x80 not set; got 0x%x", p.masks)
	}
}

// TestOnReconnect_OrsAppearanceMaskIntoMasks pins TS-faithful
// Player.onReconnect (Player.ts:555 — `this.masks |=
// PlayerInfoProt.APPEARANCE; // resync appearance`). After a resync the
// next mask-block emit must carry the appearance payload so newly-visible
// observers see the up-to-date appearance. player-net-3.
func TestOnReconnect_OrsAppearanceMaskIntoMasks(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, cc)

	// Pin entitymask to MaskFaceEntity (0x4) so a passing assertion on
	// MaskAppearance (0x1) cannot be satisfied by the existing entitymask
	// OR at block (k).
	p.entitymask = rsbuf.MaskFaceEntity
	p.masks = 0

	onReconnect(s, p)

	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("p.masks: MaskAppearance bit (0x%x) not set after onReconnect; got 0x%x", rsbuf.MaskAppearance, p.masks)
	}
}

// TestOnReconnect_ClearsWeakQueue pins TS-faithful closeModal() at
// Player.ts:543 — onReconnect calls closeModal() with the default
// `clearWeakQueue=true` (Player.ts:741), so any QueueWeak entries
// outstanding from before the disconnect are dropped on resync.
// Strong queue entries are preserved. player-net-2.
func TestOnReconnect_ClearsWeakQueue(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, cc)

	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	onReconnect(s, p)

	if got, want := len(p.queue), 1; got != want {
		t.Fatalf("p.queue len after onReconnect: got %d, want %d (weak should be dropped)", got, want)
	}
	if p.queue[0].Type != script.QueueStrong {
		t.Errorf("p.queue[0].Type: got %v, want QueueStrong", p.queue[0].Type)
	}
}
