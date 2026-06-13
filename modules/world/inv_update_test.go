package world

import (
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// invUpdateTestInvTypesLen is the configs slice capacity used by
// newInvListenerTestPlayer; large enough to cover both type 0 (world-source
// tests) and type 93 (per-player tests).
const invUpdateTestInvTypesLen = 94

// newInvListenerTestPlayer wires a Player to s with a pre-allocated invs map,
// a composed uid, and encryptor ready for wire-emit assertions. Also ensures
// s.invTypes covers types 0..93 so invLookupView.Get can resolve both world-
// source and per-player listeners created by the update-invs test suite.
func newInvListenerTestPlayer(t *testing.T, s *Server, slot int) (*Player, net.Conn) {
	t.Helper()
	if s.invTypes == nil {
		configs := make([]*objtype.InvType, invUpdateTestInvTypesLen)
		// type 0 — world-source listeners (SCOPE_SHARED); SCOPE_SHARED=2.
		configs[0] = &objtype.InvType{ConfigType: objtype.ConfigType{ID: 0}, Scope: objtype.InvTypeScopeShared, Size: 1}
		// type 93 — per-player listeners (SCOPE_TEMP=0, default).
		configs[93] = &objtype.InvType{ConfigType: objtype.ConfigType{ID: 93}, Scope: objtype.InvTypeScopeTemp, Size: 28}
		s.invTypes = &objtype.InvTypeConfigs{Configs: configs}
		s.invLookup = invLookupView{s: s}
	}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.slot = slot
	p.uid = composeUID(p.username37, p.slot) // NAI-113: matches Server.addPlayer
	p.active = true
	p.invs = map[int]*inventory.Inventory{}
	s.players.set(slot, p)
	return p, cc
}

// TestUpdateInvFull_ClampsSizeToComponentGrid pins the trade-accept crash fix:
// the transmitted slot count is min(inv.capacity, component.width*height), not
// the raw inv capacity. Sending more slots than the target component holds
// overruns the client's invSlotObjId[] array (crash on the smaller
// tradeconfirm grids). Mirrors TS UpdateInvFullEncoder.
func TestUpdateInvFull_ClampsSizeToComponentGrid(t *testing.T) {
	s := newTestServer(t)
	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 5001),
	}
	// Component 5000 is a 2x2 grid (4 slots) — smaller than the 28-slot inv.
	s.componentTypes.Configs[5000] = &objtype.ComponentType{Width: 2, Height: 2}

	p, cc := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)

	received := drainConn(t, cc)
	sendUpdateInvFullCom(p, 5000, inv)
	p.client.flushWrite()
	got := <-received

	// Wire: [enc opcode][len hi][len lo][com hi][com lo][size]...
	if len(got) < 6 {
		t.Fatalf("packet too short: %d bytes", len(got))
	}
	if size := got[5]; size != 4 {
		t.Errorf("UpdateInvFull size byte: got %d, want 4 (clamped to 2x2 component grid, not inv capacity 28)", size)
	}
}

// TestUpdateInvFull_ZeroGridComponent_SendsZeroSize pins inventory-2 /
// gap-server-codec-models-1 (2026-05-28 fresh-audit MED): TS
// UpdateInvFullEncoder.ts:14 computes `size = Math.min(inv.capacity,
// comType.width * comType.height)` UNCONDITIONALLY, so a component
// whose grid is 0 (e.g. width=0 or height=0) yields size=0. Pre-fix
// goscape guarded the clamp on `grid > 0`, so a zero-grid component
// fell through to the full inv-capacity send — the inverse of TS.
func TestUpdateInvFull_ZeroGridComponent_SendsZeroSize(t *testing.T) {
	s := newTestServer(t)
	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 5001),
	}
	// Component 5000 has Width=0 (Height=4 doesn't matter); grid = 0.
	s.componentTypes.Configs[5000] = &objtype.ComponentType{Width: 0, Height: 4}

	p, cc := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)

	received := drainConn(t, cc)
	sendUpdateInvFullCom(p, 5000, inv)
	p.client.flushWrite()
	got := <-received

	// Wire: [enc opcode][len hi][len lo][com hi][com lo][size]...
	if len(got) < 6 {
		t.Fatalf("packet too short: %d bytes", len(got))
	}
	if size := got[5]; size != 0 {
		t.Errorf("UpdateInvFull size byte: got %d, want 0 (TS UpdateInvFullEncoder.ts:14 sends Math.min(capacity, w*h) unconditionally; w=0 yields size=0, NOT inv capacity 28)", size)
	}
}

func TestUpdateInvsFirstSeenFires(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Update = false // first-seen listener should override dirty==false.
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: 2, FirstSeen: true},
	}

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("FirstSeen should fire a packet; got none")
	}
	if viewer.invListeners[149].FirstSeen {
		t.Error("FirstSeen should flip to false after first send")
	}
}

func TestUpdateInvsRespectsDirty(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
	}
	inv.Update = false

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	quiet := <-received
	if len(quiet) != 0 {
		t.Errorf("clean listener should emit nothing; got %d bytes", len(quiet))
	}

	inv.Update = true
	received2 := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	loud := <-received2
	if len(loud) == 0 {
		t.Error("dirty inv should fire a packet; got none")
	}
	// updateInvs no longer clears inv.Update — clearing moved to the
	// end-of-tick cleanup pass (Server.processCleanup) so a second player
	// listening to the same inv (trade partner via invother_transmit) isn't
	// starved. The flag stays set until cleanup.
	if !inv.Update {
		t.Error("inv.Update must NOT be cleared by updateInvs (processCleanup clears it)")
	}
}

// TestUpdateInvsCrossPlayerSharedInvNotStarved is the trade-offer regression:
// when ONE inventory is observed by two players (the owner's offer shown to
// the owner via inv_transmit AND to the partner via invother_transmit), both
// players' updateInvs must emit on the same dirty tick. The pre-fix per-player
// clear let whichever player ran first consume inv.Update, so the partner
// (processed later) saw nothing — the "other player's window stays empty" bug.
func TestUpdateInvsCrossPlayerSharedInvNotStarved(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	owner, occ := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)
	owner.invs[93] = inv
	// Owner watches their own offer (inv_transmit); FirstSeen consumed so the
	// only thing that can fire is inv.Update.
	owner.invListeners = map[int]InventoryListener{
		148: {Type: 93, Com: 148, Source: owner.uid, FirstSeen: false},
	}

	partner, pcc := newInvListenerTestPlayer(t, s, 3)
	// Partner watches the SAME inv via invother_transmit (Source = owner.uid).
	partner.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: owner.uid, FirstSeen: false},
	}

	inv.Update = true

	// Owner is processed FIRST (the pre-fix starving order).
	ownerRecv := drainConn(t, occ)
	owner.updateInvs()
	owner.client.flushWrite()
	if got := <-ownerRecv; len(got) == 0 {
		t.Error("owner should emit on dirty inv; got none")
	}

	// Partner processed second must STILL emit — inv.Update survives.
	partnerRecv := drainConn(t, pcc)
	partner.updateInvs()
	partner.client.flushWrite()
	if got := <-partnerRecv; len(got) == 0 {
		t.Error("partner (processed second) must also emit; pre-fix this was starved → empty trade window")
	}
}

func TestUpdateInvsWorldSource(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)
	s.invs[0] = inventory.New(0, 1, inventory.StackAlways)

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = map[int]InventoryListener{
		200: {Type: 0, Com: 200, Source: -1, FirstSeen: true},
	}

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Error("world-source listener should fire on FirstSeen")
	}
}

func TestUpdateInvsSkipsMissingSource(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	// source=99 doesn't exist in s.players.
	viewer.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: 99, FirstSeen: true},
	}

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) != 0 {
		t.Errorf("missing source should be skipped silently; got %d bytes", len(got))
	}
}

// TestUpdateInvsSelfListenerEmitsViaComposedUID is the NAI-113 binding
// test: a player listening on their own inv (Source = self uid, the
// production INV_TRANSMIT shape) must produce an UpdateInvFull packet
// when inv.Update fires. Pre-fix this path was silently broken because
// p.uid stayed at -1, INV_TRANSMIT registered Source=-1, and
// updateInvs routed to the world-shared inv table (which had no entry
// for per-player invtypes).
//
// Asserts the full chain: composed uid + Source=composed uid +
// LookupPlayerByUID-based emit.
func TestUpdateInvsSelfListenerEmitsViaComposedUID(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	p, cc := newInvListenerTestPlayer(t, s, 5)
	if p.uid == -1 {
		t.Fatalf("precondition: helper must compose uid; got -1 (T1/T2 wiring missing)")
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Update = true
	p.invs[93] = inv

	p.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: p.uid, FirstSeen: false},
	}

	received := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("self-listener with composed uid Source should emit UpdateInvFull; got 0 bytes")
	}
}

// TestUpdateInvsCrossPlayerListenerEmitsViaComposedUID exercises the
// INVOTHER_TRANSMIT shape: viewer's listener at Source=owner.uid must
// resolve the owner via LookupPlayerByUID and emit owner.invs[Type].
func TestUpdateInvsCrossPlayerListenerEmitsViaComposedUID(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Update = false // FirstSeen should fire emit regardless of Update.
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	if owner.uid == -1 || viewer.uid == -1 {
		t.Fatalf("precondition: helper must compose uids; owner.uid=%d, viewer.uid=%d", owner.uid, viewer.uid)
	}

	viewer.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: owner.uid, FirstSeen: true},
	}

	received := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("cross-player FirstSeen listener should emit; got 0 bytes")
	}
	if viewer.invListeners[149].FirstSeen {
		t.Error("FirstSeen should flip to false post-emit")
	}
}

// TestUpdateInvsCrossPlayerNonSlotUID forces the LookupPlayerByUID
// path to be exercised for real: the owner is placed at slot 2 but
// has uid manually set to a value far above any valid slot index, so
// players[uid] would index out of bounds (or a wrong slot) under the
// pre-fix slot-indexed lookup. Under the LookupPlayerByUID fix the
// emit succeeds.
func TestUpdateInvsCrossPlayerNonSlotUID(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	owner.uid = 0xABCDEF // far above len(s.players); pre-fix `players[0xABCDEF]` panics
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Update = true
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: owner.uid, FirstSeen: false},
	}

	received := drainConn(t, vcc)
	// Under pre-fix code this panics with "index out of range".
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("updateInvs panicked under slot-indexed lookup: %v", r)
		}
	}()
	viewer.updateInvs()
	viewer.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("cross-player listener with non-slot uid should emit via LookupPlayerByUID; got 0 bytes")
	}
}

// --- 274 partial-update wire format + full/partial fork ---

// TestSendUpdateInvPartial_WireFormat pins the per-slot wire layout of
// sendUpdateInvPartial byte-for-byte against TS UpdateInvPartialEncoder
// (@dee467c8): p2(component), then per slot p1(slot); a populated slot writes
// p2(id+1) and p1(count) (or p1(255)+p4(count) for count>=255); an empty slot
// writes p2(0)+p1(0).
func TestSendUpdateInvPartial_WireFormat(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 2)

	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[1] = &inventory.Item{Id: 1000, Count: 7}   // small count
	inv.Items[5] = &inventory.Item{Id: 2000, Count: 300} // big count (>=255)
	// slot 9 left empty

	received := drainConn(t, cc)
	sendUpdateInvPartial(p, 0x1234, inv, 1, 5, 9)
	p.client.flushWrite()
	got := <-received

	// Strip [enc opcode][len hi][len lo] header.
	if len(got) < 3 {
		t.Fatalf("packet too short: %d bytes", len(got))
	}
	payloadLen := int(got[1])<<8 | int(got[2])
	body := got[3:]
	if len(body) != payloadLen {
		t.Fatalf("payload length mismatch: header says %d, body is %d", payloadLen, len(body))
	}

	// Expected body, mirroring the TS encoder slot-by-slot.
	want := []byte{
		0x12, 0x34, // p2(component)
		// slot 1: p1(1), p2(1000+1=1001), p1(7)
		1, 0x03, 0xE9, 7,
		// slot 5: p1(5), p2(2000+1=2001), p1(255), p4(300)
		5, 0x07, 0xD1, 255, 0x00, 0x00, 0x01, 0x2C,
		// slot 9: p1(9), p2(0), p1(0)
		9, 0x00, 0x00, 0,
	}
	if len(body) != len(want) {
		t.Fatalf("body length: got %d, want %d\n got=%v\nwant=%v", len(body), len(want), body, want)
	}
	for i := range want {
		if body[i] != want[i] {
			t.Fatalf("body byte %d: got 0x%02X, want 0x%02X\n got=%v\nwant=%v", i, body[i], want[i], body, want)
		}
	}
}

// decodeFirstOpcode replays the ISAAC stream from the given seed to recover the
// plaintext opcode of the first packet (writeOut encrypts the opcode as
// (op + encryptor.GetNext()) & 0xff).
func decodeFirstOpcode(seed [4]uint32, packet []byte) byte {
	is := io2.New(seed)
	return byte((int(packet[0]) - int(is.GetNext())) & 0xff)
}

// TestUpdateInvs_FirstSeen_EmitsFull_SeenDirty_EmitsPartial pins the 274
// full/partial fork (TS NetworkPlayer.ts:341-393): a first-seen listener emits
// UPDATE_INV_FULL; once seen, a dirty-slot change emits UPDATE_INV_PARTIAL
// containing exactly the dirty slots. A SCOPE_SHARED (world-source) listener is
// used so each tick emits exactly one packet (the world branch never appends a
// run-weight packet), keeping the ISAAC opcode replay unambiguous.
func TestUpdateInvs_FirstSeen_EmitsFull_SeenDirty_EmitsPartial(t *testing.T) {
	s := newTestServer(t)
	s.invs = make(map[int]*inventory.Inventory)
	// World-shared inv (type 0, capacity 28 to make the full update wide).
	worldInv := inventory.New(0, 28, inventory.StackNormal)
	worldInv.Items[0] = &inventory.Item{Id: 1000, Count: 1}
	worldInv.Items[1] = &inventory.Item{Id: 1001, Count: 1}
	s.invs[0] = worldInv

	seed := [4]uint32{1, 2, 3, 4}
	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.client.encryptor = io2.New(seed)
	viewer.invListeners = map[int]InventoryListener{
		200: {Type: 0, Com: 200, Source: -1, FirstSeen: true},
	}

	// Tick 1 — first-seen → UPDATE_INV_FULL (opcode 106).
	received1 := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got1 := <-received1
	if op := decodeFirstOpcode(seed, got1); op != 106 {
		t.Fatalf("first-seen listener: opcode got %d, want 106 (UPDATE_INV_FULL)", op)
	}
	if viewer.invListeners[200].FirstSeen {
		t.Fatal("FirstSeen should flip to false after the full update")
	}
	fullLen := len(got1)

	// Mutate one slot through Set so it is dirtied. Clear the dirty set from the
	// initial direct Items writes first, then dirty exactly slot 5.
	worldInv.ResetTracking()
	worldInv.Set(5, &inventory.Item{Id: 2000, Count: 3})

	// Tick 2 — seen + dirty → UPDATE_INV_PARTIAL (opcode 172) of just slot 5.
	// The world branch emitted exactly one packet in tick 1, so the ISAAC
	// keystream has advanced exactly once.
	op2Decoder := io2.New(seed)
	op2Decoder.GetNext() // consume tick-1 opcode keystream
	received2 := drainConn(t, vcc)
	viewer.updateInvs()
	viewer.client.flushWrite()
	got2 := <-received2
	op2 := byte((int(got2[0]) - int(op2Decoder.GetNext())) & 0xff)
	if op2 != 172 {
		t.Fatalf("seen+dirty listener: opcode got %d, want 172 (UPDATE_INV_PARTIAL)", op2)
	}
	if len(got2) >= fullLen {
		t.Errorf("partial update (%d bytes) should be shorter than the full update (%d bytes)", len(got2), fullLen)
	}
	// Partial payload = com(2) + 1 slot × (slot1 + id2 + count1) = 6 bytes;
	// packet = opcode(1) + len2(2) + 6 = 9 bytes.
	if len(got2) != 9 {
		t.Errorf("1-dirty-slot partial: got %d bytes, want 9", len(got2))
	}
	// Confirm the partial carries slot 5 (the only dirty slot): after p2(com)
	// the body's 3rd byte is the per-slot id.
	body := got2[3:]
	if body[2] != 5 {
		t.Errorf("partial slot id: got %d, want 5 (the only dirty slot)", body[2])
	}
}
