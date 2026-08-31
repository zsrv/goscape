package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// TestInvListenOnComRegistersNewListener verifies a fresh call on a
// Player with no prior listeners creates a map entry with the expected
// Type, Com, Source, and FirstSeen=true.
func TestInvListenOnComRegistersNewListener(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(93, 149, -1)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1", len(p.invListeners))
	}
	got, ok := p.invListeners[149]
	if !ok {
		t.Fatal("listener at com=149 should exist")
	}
	if got.Type != 93 {
		t.Errorf("Type: got %d, want 93", got.Type)
	}
	if got.Com != 149 {
		t.Errorf("Com: got %d, want 149", got.Com)
	}
	if got.Source != -1 {
		t.Errorf("Source: got %d, want -1", got.Source)
	}
	if !got.FirstSeen {
		t.Error("FirstSeen should be true for new listener")
	}
}

// TestInvListenOnComReplacesExisting verifies that a second call with
// the same com but a DIFFERENT type overwrites the first entry and
// resets FirstSeen to true. Matches TS Player.ts:1457-1460 same-com-
// different-type splice (the (β) dedup at TS:1446-1449 does NOT apply
// because the types differ; that's pinned separately by
// TestInvListenOnComDedupsSameTypeSameCom).
func TestInvListenOnComReplacesExisting(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(93, 149, -1)
	// Simulate a first-seen emit flipping FirstSeen to false.
	l := p.invListeners[149]
	l.FirstSeen = false
	p.invListeners[149] = l

	// Re-register with a different Type/Source at the same com.
	p.invListenOnCom(100, 149, 2)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1 (re-register should not add a second entry)", len(p.invListeners))
	}
	got := p.invListeners[149]
	if got.Type != 100 {
		t.Errorf("Type: got %d, want 100 (replace should overwrite)", got.Type)
	}
	if got.Source != 2 {
		t.Errorf("Source: got %d, want 2 (replace should overwrite)", got.Source)
	}
	if !got.FirstSeen {
		t.Error("FirstSeen should reset to true on replace")
	}
}

// TestInvListenOnComLazyInitializesMap verifies that the first call on
// a Player whose invListeners field is nil allocates the map.
func TestInvListenOnComLazyInitializesMap(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.invListeners != nil {
		t.Fatalf("precondition: invListeners should start nil, got %v", p.invListeners)
	}

	p.invListenOnCom(93, 149, -1)

	if p.invListeners == nil {
		t.Fatal("invListenOnCom should allocate the map on first call")
	}
}

// TestInvStopListenOnComRemovesListener verifies that calling stop on
// a registered com deletes the entry and decreases len by 1.
func TestInvStopListenOnComRemovesListener(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListenOnCom(93, 149, -1)
	p.invListenOnCom(100, 200, -1)
	if len(p.invListeners) != 2 {
		t.Fatalf("setup: len should be 2, got %d", len(p.invListeners))
	}

	received := drainConn(t, cc)
	p.invStopListenOnCom(149)
	p.client.flushWrite()

	got := <-received
	if len(got) != 3 {
		t.Errorf("packet bytes: got %d, want 3 (opcode + P2)", len(got))
	}
	if len(p.invListeners) != 1 {
		t.Errorf("len: got %d, want 1", len(p.invListeners))
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener at 149 should be removed")
	}
	if _, ok := p.invListeners[200]; !ok {
		t.Error("listener at 200 should remain")
	}
}

// TestInvStopListenOnComNoopForMissingKey verifies calling stop on a
// com that was never registered is a no-op (does not panic, does not
// mutate map).
func TestInvStopListenOnComNoopForMissingKey(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListenOnCom(93, 149, -1)

	received := drainConn(t, cc)
	p.invStopListenOnCom(999) // never registered
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("missing-key stop should write no packet; got %d bytes", len(got))
	}
	if len(p.invListeners) != 1 {
		t.Errorf("len: got %d, want 1 (unrelated listener should remain)", len(p.invListeners))
	}
}

// TestInvStopListenOnComNoopForNilMap verifies calling stop on a Player
// whose map is still nil does not panic (Go's delete-on-nil semantic).
func TestInvStopListenOnComNoopForNilMap(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	if p.invListeners != nil {
		t.Fatalf("precondition: invListeners should start nil")
	}

	received := drainConn(t, cc)
	p.invStopListenOnCom(149) // must not panic
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("nil-map stop should write no packet; got %d bytes", len(got))
	}
	if p.invListeners != nil {
		t.Error("stop on nil map should not cause an allocation")
	}
}

// TestInvListenOnComEarlyOutOnInvalidInvType pins the TS Player.ts:1442-1444
// early-out: invType=-1 means invalid; the listener registration is a no-op
// and the map is not allocated.
func TestInvListenOnComEarlyOutOnInvalidInvType(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(-1, 149, 0)

	if len(p.invListeners) != 0 {
		t.Errorf("len: got %d, want 0 (early-out should not register)", len(p.invListeners))
	}
	if p.invListeners != nil {
		t.Error("invListeners should remain nil — early-out should not allocate the map")
	}
}

// TestInvListenOnComDedupsSameTypeSameCom pins the TS Player.ts:1446-1449
// dedup: a second invListenOnCom call with the same (Type, Com) is a no-op
// — FirstSeen state is preserved across redundant calls so that a redundant
// inv_transmit does not force a re-emit.
func TestInvListenOnComDedupsSameTypeSameCom(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(93, 149, -1)
	// Simulate a first-seen emit flipping FirstSeen to false.
	l := p.invListeners[149]
	l.FirstSeen = false
	p.invListeners[149] = l

	// Re-register with the SAME Type and Com — should be a no-op (preserves FirstSeen=false).
	p.invListenOnCom(93, 149, -1)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1 (dedup should not add a second entry)", len(p.invListeners))
	}
	got := p.invListeners[149]
	if got.FirstSeen {
		t.Error("FirstSeen should remain false after redundant invListenOnCom on same (Type, Com)")
	}
	if got.Type != 93 {
		t.Errorf("Type: got %d, want 93", got.Type)
	}
}

// Test fixture constants for the scope-rewrite tests. The InvTypeID is the
// slot used in the configs slice; testInvTypesLen is comfortably larger so
// indexing is safe and the bounds-check in (γ) is exercised on a populated
// slot rather than out-of-range.
const (
	testInvTypeID   = 42
	testInvTypesLen = 50
)

// TestInvListenOnComRewritesSourceForSharedScope pins the TS Player.ts:1456-1459
// scope-rewrite: when invType has SCOPE_SHARED scope, the registration method
// rewrites source = -1 internally regardless of what the caller passed.
func TestInvListenOnComRewritesSourceForSharedScope(t *testing.T) {
	configs := make([]*objtype.InvType, testInvTypesLen)
	configs[testInvTypeID] = &objtype.InvType{Scope: objtype.InvTypeScopeShared}
	p, _ := newTestPlayerWithInvTypes(t, configs)

	// Caller passes source=99; the SCOPE_SHARED rewrite should override to -1.
	p.invListenOnCom(testInvTypeID, 149, 99)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1", len(p.invListeners))
	}
	got := p.invListeners[149]
	if got.Source != -1 {
		t.Errorf("Source: got %d, want -1 (SCOPE_SHARED should rewrite)", got.Source)
	}
	if got.Type != testInvTypeID {
		t.Errorf("Type: got %d, want %d", got.Type, testInvTypeID)
	}
}

// TestInvListenOnComKeepsSourceForNonSharedScope pins the negative case of the
// scope-rewrite: when invType has non-SCOPE_SHARED scope (perm/temp), the
// caller-passed source is preserved.
func TestInvListenOnComKeepsSourceForNonSharedScope(t *testing.T) {
	configs := make([]*objtype.InvType, testInvTypesLen)
	configs[testInvTypeID] = &objtype.InvType{Scope: objtype.InvTypeScopePerm}
	p, _ := newTestPlayerWithInvTypes(t, configs)

	p.invListenOnCom(testInvTypeID, 149, 99)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1", len(p.invListeners))
	}
	got := p.invListeners[149]
	if got.Source != 99 {
		t.Errorf("Source: got %d, want 99 (SCOPE_PERM should preserve caller source)", got.Source)
	}
	if got.Type != testInvTypeID {
		t.Errorf("Type: got %d, want %d (Type should be preserved alongside Source)", got.Type, testInvTypeID)
	}
}

// TestInvStopListenOnComWritesUpdatePacket pins TS Player.ts:1464-1471:
// invStopListenOnCom must remove the listener AND write
// OpUpdateInvStopTransmit(com).
func TestInvStopListenOnComWritesUpdatePacket(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListenOnCom(93, 149, -1)

	received := drainConn(t, cc)
	p.invStopListenOnCom(149)
	p.client.flushWrite()

	got := <-received
	// Wire: 1 opcode byte + 2 payload bytes (P2 com=149) = 3 bytes.
	if len(got) != 3 {
		t.Errorf("got %d bytes, want 3 (opcode + P2 com); bytes=%v", len(got), got)
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener at 149 should be removed")
	}
}

// TestUpdateInvsLazyAllocSeedsStockTemp pins: a SCOPE_TEMP listener for an
// unallocated InvType with StockObj populated causes updateInvs to
// lazy-allocate the per-player inventory (seeding stock items) and emit a
// sendUpdateInvFullCom wire packet. Mirrors TS Player.ts:1400-1438.
func TestUpdateInvsLazyAllocSeedsStockTemp(t *testing.T) {
	// InvType: SCOPE_TEMP, capacity 5, stock=[bronze_dagger=100, bronze_sword=101].
	cfg := &objtype.InvType{
		ID:         testInvTypeID,
		Scope:      objtype.InvTypeScopeTemp,
		Size:       5,
		StockObj:   []uint16{100, 101, 0, 0, 0},
		StockCount: []uint16{1, 1, 0, 0, 0},
	}
	configs := make([]*objtype.InvType, testInvTypesLen)
	configs[testInvTypeID] = cfg

	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s := &Server{
		log:        discardLogger(),
		invTypes:   &objtype.InvTypeConfigs{Configs: configs},
		invs:       make(map[int]*inventory.Inventory),
		playerLoop: []*Player{p}, // for LookupPlayerByUID
	}
	s.invLookup = invLookupView{s: s}
	p.client.server = s
	p.uid = 12345
	p.active = true

	// Register listener at com=149 — SCOPE_TEMP, source=p.uid.
	p.invListenOnCom(testInvTypeID, 149, p.uid)
	if p.invListeners[149].Source != p.uid {
		t.Fatalf("setup: Source got %d, want %d", p.invListeners[149].Source, p.uid)
	}
	if _, ok := p.invs[testInvTypeID]; ok {
		t.Fatal("setup: per-player inv slot must be empty before updateInvs")
	}

	received := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()

	got := <-received
	if len(got) == 0 {
		t.Fatal("updateInvs should emit a wire packet for a SCOPE_TEMP listener with stock items")
	}

	// Lazy-alloc must have populated p.invs[testInvTypeID] with stock items.
	inv, ok := p.invs[testInvTypeID]
	if !ok || inv == nil {
		t.Error("updateInvs should lazy-allocate per-player inv from InvType")
	} else if inv.GetItemCount(100) != 1 || inv.GetItemCount(101) != 1 {
		t.Errorf("lazy-allocated inv missing stock items: items=%v", inv.Items)
	}
}

// ── NAI-136 runweight extension tests (§8.3) ──────────────────────────────────

// buildRunWeightInvServer builds a minimal Server with invTypes + objTypes for
// player_inv_test NAI-136 tests. invTypeID must be the RunWeight=true InvType;
// objTypeID must be the non-stackable weighted ObjType used for items.
func buildRunWeightInvServer(t *testing.T, p *Player, invTypeID, objTypeID, objWeight int) *Server {
	t.Helper()
	invConfigs := make([]*objtype.InvType, testInvTypesLen)
	invConfigs[invTypeID] = &objtype.InvType{
		ID:        invTypeID,
		Scope:     objtype.InvTypeScopeTemp,
		Size:      1,
		RunWeight: true,
	}
	objConfigs := make([]*objtype.ObjType, objTypeID+1)
	objConfigs[objTypeID] = &objtype.ObjType{
		Stackable: false,
		Weight:    objWeight,
	}
	s := &Server{
		log:        discardLogger(),
		invTypes:   &objtype.InvTypeConfigs{Configs: invConfigs},
		objTypes:   &objtype.ObjTypeConfigs{Configs: objConfigs},
		invs:       make(map[int]*inventory.Inventory),
		playerLoop: []*Player{p},
	}
	s.invLookup = invLookupView{s: s}
	p.client.server = s
	p.uid = 12345
	p.active = true
	return s
}

// Wire sizes for a capacity=1 inv (empty or 1 item, count<255):
//
//	UpdateInvFull wire = 1 opcode + 2 len + (2 com + 1 size + 2 id + 1 count) = 9 bytes.
//	UpdateRunWeight wire = 1 opcode + 2 payload = 3 bytes.
const (
	updateInvFull1SlotBytes = 9 // capacity=1 inv, any fill state
	updateRunWeightBytes    = 3
)

// TestUpdateInvs_RunWeightChangedEmitsPacket — per-player listener on a
// RunWeight=true inv; after first-tick (clears FirstSeen), add a weighted item
// and run a second tick. The second tick emits UpdateInvFull + UpdateRunWeight.
// NAI-136 §8.3.
func TestUpdateInvs_RunWeightChangedEmitsPacket(t *testing.T) {
	const invTypeID = testInvTypeID
	const objTypeID = 3
	const objWeight = 1000 // 1 kg

	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s := buildRunWeightInvServer(t, p, invTypeID, objTypeID, objWeight)

	// Register per-player listener (FirstSeen=true).
	p.invListenOnCom(invTypeID, 149, p.uid)

	// Tick 1 — drains FirstSeen; inv is empty; runweight stays 0 after calculateRunWeight.
	received1 := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	<-received1 // consume to unblock

	// Verify FirstSeen is now false.
	if p.invListeners[149].FirstSeen {
		t.Fatal("setup: FirstSeen must be false after tick 1")
	}

	// Add a weighted non-stackable item to the per-player inv.
	inv := s.invLookup.Get(p, invTypeID)
	if inv == nil {
		t.Fatal("setup: inv must be allocated after tick 1")
	}
	inv.Items[0] = &inventory.Item{Id: objTypeID, Count: 1}
	inv.Update = true // mark dirty so the listener fires

	// Tick 2 — inv.Update=true, RunWeight inv → runWeightChanged=true.
	// calculateRunWeight returns 1000; before=0 → runWeightChanged stays true.
	received2 := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	got2 := <-received2

	wantLen := updateInvFull1SlotBytes + updateRunWeightBytes
	if len(got2) != wantLen {
		t.Errorf("tick 2: got %d bytes, want %d (UpdateInvFull + UpdateRunWeight)", len(got2), wantLen)
	}
}

// TestUpdateInvs_FirstSeenEmitsEvenIfWeightZero — fresh listener (FirstSeen=true)
// on an empty RunWeight inv; even though runWeightChanged=false (weight stays 0),
// firstSeen forces UpdateRunWeight(0) to be emitted. NAI-136 §8.3.
func TestUpdateInvs_FirstSeenEmitsEvenIfWeightZero(t *testing.T) {
	const invTypeID = testInvTypeID
	const objTypeID = 3

	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	buildRunWeightInvServer(t, p, invTypeID, objTypeID, 1000)

	// Register listener with FirstSeen=true (default).
	p.invListenOnCom(invTypeID, 149, p.uid)

	// Tick 1 — empty inv, RunWeight=true, FirstSeen=true.
	// Expect: UpdateInvFull(9) + UpdateRunWeight(3) = 12 bytes.
	received := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	got := <-received

	wantLen := updateInvFull1SlotBytes + updateRunWeightBytes
	if len(got) != wantLen {
		t.Errorf("got %d bytes, want %d (UpdateInvFull + UpdateRunWeight(0))", len(got), wantLen)
	}
}

// TestUpdateInvs_NoChangeNoEmitOnSecondTick — after tick 1 (clears FirstSeen),
// tick 2 with no inventory mutations emits no packets at all. NAI-136 §8.3.
func TestUpdateInvs_NoChangeNoEmitOnSecondTick(t *testing.T) {
	const invTypeID = testInvTypeID
	const objTypeID = 3

	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	buildRunWeightInvServer(t, p, invTypeID, objTypeID, 1000)
	p.invListenOnCom(invTypeID, 149, p.uid)

	// Tick 1 — clears FirstSeen.
	received1 := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	<-received1

	// Tick 2 — no mutations; inv.Update=false, FirstSeen=false.
	received2 := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	got2 := <-received2

	if len(got2) != 0 {
		t.Errorf("tick 2: got %d bytes, want 0 (no change → no packets)", len(got2))
	}
}

// TestUpdateInvs_SharedInvDoesNotCountToRunWeight — SCOPE_SHARED listener
// (Source==-1) on a RunWeight=true inv; the SCOPE_SHARED branch does NOT
// set runWeightChanged or firstSeen, so no UpdateRunWeight is emitted.
// NAI-136 §8.3; TS NetworkPlayer.ts:354-357.
func TestUpdateInvs_SharedInvDoesNotCountToRunWeight(t *testing.T) {
	const invTypeID = testInvTypeID
	const objTypeID = 3

	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Build server with SCOPE_SHARED inv.
	invConfigs := make([]*objtype.InvType, testInvTypesLen)
	invConfigs[invTypeID] = &objtype.InvType{
		ID:        invTypeID,
		Scope:     objtype.InvTypeScopeShared, // forces Source=-1 in invListenOnCom
		Size:      1,
		RunWeight: true,
	}
	objConfigs := make([]*objtype.ObjType, objTypeID+1)
	objConfigs[objTypeID] = &objtype.ObjType{Stackable: false, Weight: 1000}
	s := &Server{
		log:        discardLogger(),
		invTypes:   &objtype.InvTypeConfigs{Configs: invConfigs},
		objTypes:   &objtype.ObjTypeConfigs{Configs: objConfigs},
		invs:       make(map[int]*inventory.Inventory),
		playerLoop: []*Player{p},
	}
	s.invLookup = invLookupView{s: s}
	p.client.server = s
	p.uid = 12345
	p.active = true

	// SCOPE_SHARED: invListenOnCom rewrites source to -1.
	p.invListenOnCom(invTypeID, 149, p.uid)
	if p.invListeners[149].Source != -1 {
		t.Fatalf("setup: Source must be -1 for SCOPE_SHARED; got %d", p.invListeners[149].Source)
	}

	// Tick — FirstSeen=true, shared inv, RunWeight=true.
	// Expect only UpdateInvFull (9 bytes); no UpdateRunWeight.
	received := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	got := <-received

	wantLen := updateInvFull1SlotBytes // NO UpdateRunWeight
	if len(got) != wantLen {
		t.Errorf("got %d bytes, want %d (UpdateInvFull only; SCOPE_SHARED skips RunWeight)", len(got), wantLen)
	}
}

// TestUpdateInvs_SkipOnNoNetWeightChange — RunWeight listener fires
// (inv.Update=true) but the recomputed runweight equals p.runweight (same
// total); skip-on-no-change suppresses UpdateRunWeight. NAI-136 §8.3.
func TestUpdateInvs_SkipOnNoNetWeightChange(t *testing.T) {
	const invTypeID = testInvTypeID
	const objTypeID = 3
	const objWeight = 2275 // 2.275 kg; 1 item → runweight=2275

	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s := buildRunWeightInvServer(t, p, invTypeID, objTypeID, objWeight)
	p.invListenOnCom(invTypeID, 149, p.uid)

	// Tick 1 — clears FirstSeen; empty inv → runweight stays 0.
	received1 := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	<-received1

	// Pre-set p.runweight to match what calculateRunWeight will compute (1 item × 2275g).
	inv := s.invLookup.Get(p, invTypeID)
	if inv == nil {
		t.Fatal("setup: inv must be allocated after tick 1")
	}
	inv.Items[0] = &inventory.Item{Id: objTypeID, Count: 1}
	inv.Update = true
	p.runweight = objWeight // pre-set; calculateRunWeight will return the same value

	// Tick 2 — runWeightChanged starts true (RunWeight inv, inv.Update=true),
	// but after calculateRunWeight: before(2275)==p.runweight(2275) → false.
	// firstSeen=false. → no UpdateRunWeight.
	received2 := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()
	got2 := <-received2

	wantLen := updateInvFull1SlotBytes // UpdateInvFull only; no UpdateRunWeight
	if len(got2) != wantLen {
		t.Errorf("skip-on-no-change: got %d bytes, want %d (UpdateInvFull only)", len(got2), wantLen)
	}
}

// TestUpdateInvsLazyAllocSeedsStockShared pins: a SCOPE_SHARED listener
// (source rewritten to -1 by invListenOnCom) causes updateInvs to
// lazy-allocate the world-shared inventory and emit a sendUpdateInvFullCom
// wire packet. Mirrors TS Player.ts:1400-1438 (World.getInventory path).
func TestUpdateInvsLazyAllocSeedsStockShared(t *testing.T) {
	// InvType: SCOPE_SHARED, capacity 5, stock=[bronze_dagger=100, bronze_sword=101].
	cfg := &objtype.InvType{
		ID:         testInvTypeID,
		Scope:      objtype.InvTypeScopeShared,
		Size:       5,
		StockObj:   []uint16{100, 101, 0, 0, 0},
		StockCount: []uint16{1, 1, 0, 0, 0},
	}
	configs := make([]*objtype.InvType, testInvTypesLen)
	configs[testInvTypeID] = cfg

	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s := &Server{
		log:        discardLogger(),
		invTypes:   &objtype.InvTypeConfigs{Configs: configs},
		invs:       make(map[int]*inventory.Inventory),
		playerLoop: []*Player{p},
	}
	s.invLookup = invLookupView{s: s}
	p.client.server = s
	p.uid = 12345
	p.active = true

	// caller source=99; SCOPE_SHARED rewrite inside invListenOnCom flips to -1.
	p.invListenOnCom(testInvTypeID, 149, 99)
	if p.invListeners[149].Source != -1 {
		t.Fatalf("setup: Source got %d, want -1 (SCOPE_SHARED rewrite)", p.invListeners[149].Source)
	}
	if _, ok := s.invs[testInvTypeID]; ok {
		t.Fatal("setup: world-shared inv slot must be empty before updateInvs")
	}

	received := drainConn(t, cc)
	p.updateInvs()
	p.client.flushWrite()

	got := <-received
	if len(got) == 0 {
		t.Fatal("updateInvs should emit a wire packet for a SCOPE_SHARED listener with stock items")
	}

	// Lazy-alloc must have populated s.invs[testInvTypeID] with stock items.
	inv, ok := s.invs[testInvTypeID]
	if !ok || inv == nil {
		t.Error("updateInvs should lazy-allocate world-shared inv from InvType")
	} else if inv.GetItemCount(100) != 1 || inv.GetItemCount(101) != 1 {
		t.Errorf("lazy-allocated shared inv missing stock items: items=%v", inv.Items)
	}
}
