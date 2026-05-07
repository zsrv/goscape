package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/inventory"
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
		ConfigType: objtype.ConfigType{ID: testInvTypeID},
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

// TestUpdateInvsLazyAllocSeedsStockShared pins: a SCOPE_SHARED listener
// (source rewritten to -1 by invListenOnCom) causes updateInvs to
// lazy-allocate the world-shared inventory and emit a sendUpdateInvFullCom
// wire packet. Mirrors TS Player.ts:1400-1438 (World.getInventory path).
func TestUpdateInvsLazyAllocSeedsStockShared(t *testing.T) {
	// InvType: SCOPE_SHARED, capacity 5, stock=[bronze_dagger=100, bronze_sword=101].
	cfg := &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: testInvTypeID},
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
