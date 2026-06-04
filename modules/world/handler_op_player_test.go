package world

import (
	"fmt"
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// makeOpPlayerFixture builds a server with two logged-in players (clicker
// at slot 1 and "other" at slot 2). The clicker has a real net.Conn for
// drainConn-based UnsetMapFlag assertions; "other" is wired with the
// minimum needed to satisfy LookupPlayerBySlot + slot indexing.
//
// Returns (server, clicker, otherPlayer, clickerConn).
func makeOpPlayerFixture(t *testing.T) (*Server, *Player, *Player, net.Conn) {
	t.Helper()
	s := newTestServer(t)

	clicker, cc := newTestPlayer(t)
	clicker.client.server = s
	clicker.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	clicker.slot = 1
	s.players[1] = clicker
	s.rsbuf.AddPlayer(int32(clicker.slot))

	other, _ := newTestPlayer(t)
	other.client.server = s
	other.slot = 2
	s.players[2] = other
	s.rsbuf.AddPlayer(int32(other.slot))

	return s, clicker, other, cc
}

// makeOpPlayerFixtureWithBothConns is makeOpPlayerFixture but also returns
// `other`'s conn so tests can drain target-side traffic (e.g. OpMes
// MessageGame writes for trigger-dispatch verification — NAI-62).
func makeOpPlayerFixtureWithBothConns(t *testing.T) (*Server, *Player, *Player, net.Conn, net.Conn) {
	t.Helper()
	s := newTestServer(t)

	clicker, cc := newTestPlayer(t)
	clicker.client.server = s
	clicker.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	clicker.slot = 1
	s.players[1] = clicker
	s.rsbuf.AddPlayer(int32(clicker.slot))

	other, cc2 := newTestPlayer(t)
	other.client.server = s
	other.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	other.slot = 2
	s.players[2] = other
	s.rsbuf.AddPlayer(int32(other.slot))

	return s, clicker, other, cc, cc2
}

// rsbufSeesPlayer makes s.rsbuf.HasPlayer(observer, target) return true
// by inserting target into observer's BuildArea.Players tracking set
// directly (test-only path; production code goes through ComputePlayer).
func rsbufSeesPlayer(t *testing.T, s *Server, observerSlot, targetSlot int) {
	t.Helper()
	bp := s.rsbuf.PlayerForTest(int32(observerSlot))
	if bp == nil {
		t.Fatalf("rsbuf has no player at observer slot %d", observerSlot)
	}
	bp.Build.Players.Insert(int32(targetSlot))
}

// TestHandleOpPlayer_HappyPath_AllOps — for each of op 1..4, the handler
// sets target = other, targetOp = op, targetSubject.com = -1, kind =
// InteractionEngine.
func TestHandleOpPlayer_HappyPath_AllOps(t *testing.T) {
	for op := 1; op <= 4; op++ {
		t.Run(fmt.Sprintf("op=%d", op), func(t *testing.T) {
			s, clicker, other, _ := makeOpPlayerFixture(t)
			rsbufSeesPlayer(t, s, clicker.slot, other.slot)

			if err := handleOpPlayer(clicker, p2Payload(other.slot), op); err != nil {
				t.Fatalf("handleOpPlayer: %v", err)
			}

			if clicker.target != other {
				t.Errorf("target: got %v, want other (%p)", clicker.target, other)
			}
			if clicker.targetOp != op {
				t.Errorf("targetOp: got %d, want %d", clicker.targetOp, op)
			}
			if clicker.targetSubject.com != -1 {
				t.Errorf("targetSubject.com: got %d, want -1", clicker.targetSubject.com)
			}
			if clicker.interactionKind != InteractionEngine {
				t.Errorf("interactionKind: got %v, want InteractionEngine", clicker.interactionKind)
			}
		})
	}
}

// TestHandleOpPlayer_DelayedSendsUnsetMapFlag — when the player is
// delayed, handler skips interaction setup and writes UnsetMapFlag.
// 244: delayed reject must NOT call clearPendingAction (TS OpPlayerHandler.ts:16-19).
func TestHandleOpPlayer_DelayedSendsUnsetMapFlag(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	clicker.delayed = true
	clicker.delayedUntil = 999
	s.currentTick = 0
	clicker.target = clicker // sentinel: clearPendingAction would nil this; must survive

	received := drainConn(t, cc)
	_ = handleOpPlayer(clicker, p2Payload(other.slot), 1)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	// 244: delayed-player reject must NOT call clearPendingAction.
	if clicker.target != clicker {
		t.Error("244: delayed-player reject must NOT call clearPendingAction (target sentinel cleared)")
	}
}

// TestHandleOpPlayer_TargetNotLoggedIn — LookupPlayerBySlot returns nil
// (slot empty) → UnsetMapFlag + clearPendingAction, no interaction set.
// 244: clearPendingAction added on target-not-found reject (TS OpPlayerHandler.ts:21-25).
func TestHandleOpPlayer_TargetNotLoggedIn(t *testing.T) {
	s, clicker, _, cc := makeOpPlayerFixture(t)
	const missingSlot = 99
	s.players[missingSlot] = nil
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayer(clicker, p2Payload(missingSlot), 1)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on target-not-found.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on target-not-found — TS OpPlayerHandler.ts:23)", clicker.targetOp)
	}
}

// TestHandleOpPlayer_NotVisibleViaRsbuf — target exists but not visible
// to local player per rsbuf.HasPlayer → UnsetMapFlag + clearPendingAction,
// no interaction set.
// 244: clearPendingAction added on rsbuf-not-visible reject (TS OpPlayerHandler.ts:27-31).
func TestHandleOpPlayer_NotVisibleViaRsbuf(t *testing.T) {
	_, clicker, other, cc := makeOpPlayerFixture(t)
	// Deliberately do NOT call rsbufSeesPlayer.
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayer(clicker, p2Payload(other.slot), 1)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for non-visible target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on rsbuf-not-visible.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on rsbuf-not-visible — TS OpPlayerHandler.ts:29)", clicker.targetOp)
	}
}

// TestHandleOpPlayer_TruncatedPayload — payload < 2 bytes → UnsetMapFlag.
func TestHandleOpPlayer_TruncatedPayload(t *testing.T) {
	_, clicker, _, cc := makeOpPlayerFixture(t)

	received := drainConn(t, cc)
	_ = handleOpPlayer(clicker, []byte{0x01}, 1)
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for truncated payload, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// p2x2Payload encodes (a: u16, b: u16) into 4 bytes big-endian.
// Used by OpPlayerT payload construction: slot + spellCom.
func opPlayerTPayload(slot, spellCom int) []byte {
	return []byte{
		byte(slot >> 8), byte(slot),
		byte(spellCom >> 8), byte(spellCom),
	}
}

// TestHandleOpPlayerT_HappyPath — valid OPPLAYERT request sets target,
// targetOp = targetOpPlayerT, targetSubject.com = spellCom, kind = Engine.
func TestHandleOpPlayerT_HappyPath(t *testing.T) {
	s, clicker, other, _ := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const spellCom = 7777

	// gate satisfaction: register spellCom with passing ActionTarget bit and visibility.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		spellCom: {RootLayer: spellCom, ActionTarget: objtype.ComActionTargetPlayer},
	})
	clicker.tabs[0] = spellCom

	if err := handleOpPlayerT(clicker, opPlayerTPayload(other.slot, spellCom)); err != nil {
		t.Fatalf("handleOpPlayerT: %v", err)
	}

	if clicker.target != other {
		t.Errorf("target: got %v, want other (%p)", clicker.target, other)
	}
	if clicker.targetOp != targetOpPlayerT {
		t.Errorf("targetOp: got %d, want targetOpPlayerT (%d)", clicker.targetOp, targetOpPlayerT)
	}
	if clicker.targetSubject.com != spellCom {
		t.Errorf("targetSubject.com: got %d, want %d (spellCom)", clicker.targetSubject.com, spellCom)
	}
	if clicker.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", clicker.interactionKind)
	}
}

// TestHandleOpPlayerT_DelayedSendsUnsetMapFlag — delayed clicker →
// UnsetMapFlag, no interaction set.
// 244: delayed reject must NOT call clearPendingAction (TS OpPlayerTHandler.ts:16-19).
func TestHandleOpPlayerT_DelayedSendsUnsetMapFlag(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	clicker.delayed = true
	clicker.delayedUntil = 999
	s.currentTick = 0
	clicker.target = clicker // sentinel: clearPendingAction would nil this; must survive

	received := drainConn(t, cc)
	_ = handleOpPlayerT(clicker, opPlayerTPayload(other.slot, 7777))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	// 244: delayed-player reject must NOT call clearPendingAction.
	if clicker.target != clicker {
		t.Error("244: delayed-player reject must NOT call clearPendingAction (target sentinel cleared)")
	}
}

// TestHandleOpPlayerT_TargetNotLoggedIn — LookupPlayerBySlot returns nil →
// UnsetMapFlag + clearPendingAction, no interaction set.
// 244: clearPendingAction added on target-not-found reject (TS OpPlayerTHandler.ts:29-33).
// Prerequisites: component gate seeded so the target-not-logged-in gate is the
// discriminating condition under test (not the component gate).
func TestHandleOpPlayerT_TargetNotLoggedIn(t *testing.T) {
	s, clicker, _, cc := makeOpPlayerFixture(t)
	const (
		missingSlot = 99
		spellCom    = 7777
	)
	s.players[missingSlot] = nil
	// Seed component so the component gate passes; target-not-found gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		spellCom: {RootLayer: spellCom, ActionTarget: objtype.ComActionTargetPlayer},
	})
	clicker.tabs[0] = spellCom
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayerT(clicker, opPlayerTPayload(missingSlot, spellCom))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on target-not-found.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on target-not-found — TS OpPlayerTHandler.ts:31)", clicker.targetOp)
	}
}

// TestHandleOpPlayerT_TargetNotVisible — target exists but rsbuf.HasPlayer
// is false → UnsetMapFlag + clearPendingAction, no interaction set.
// 244: clearPendingAction added on rsbuf-not-visible reject (TS OpPlayerTHandler.ts:35-39).
// Prerequisites: component gate seeded so the rsbuf-visibility gate is the
// discriminating condition under test (not the component gate).
func TestHandleOpPlayerT_TargetNotVisible(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	// Deliberately do NOT call rsbufSeesPlayer.
	const spellCom = 7777
	// Seed component so the component gate passes; rsbuf-visibility gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		spellCom: {RootLayer: spellCom, ActionTarget: objtype.ComActionTargetPlayer},
	})
	clicker.tabs[0] = spellCom
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayerT(clicker, opPlayerTPayload(other.slot, spellCom))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for non-visible target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on rsbuf-not-visible.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on rsbuf-not-visible — TS OpPlayerTHandler.ts:37)", clicker.targetOp)
	}
}

// TestHandleOpPlayerT_TruncatedPayload — payload < 4 bytes → UnsetMapFlag.
func TestHandleOpPlayerT_TruncatedPayload(t *testing.T) {
	_, clicker, _, cc := makeOpPlayerFixture(t)

	received := drainConn(t, cc)
	_ = handleOpPlayerT(clicker, []byte{0x00, 0x02, 0x01})
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for truncated payload, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerT_ComponentGate_ClearsPendingAction — nil/!visible/!actionTarget
// combined component check fires clearPendingAction (244 delta).
// 244: TS OpPlayerTHandler.ts:21-26 combined check now calls clearPendingAction.
func TestHandleOpPlayerT_ComponentGate_ClearsPendingAction(t *testing.T) {
	_, clicker, other, cc := makeOpPlayerFixture(t)
	// spellCom=8888 is NOT registered → nil component → component gate fires.
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayerT(clicker, opPlayerTPayload(other.slot, 8888))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for component gate reject, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on component gate reject.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on component gate — TS OpPlayerTHandler.ts:23)", clicker.targetOp)
	}
}

// opPlayerUPayload encodes (slot:u16, useObj:u16, useSlot:u16, useCom:u16)
// into 8 bytes big-endian. Used by OpPlayerU payload construction.
func opPlayerUPayload(slot, useObj, useSlot, useCom int) []byte {
	return []byte{
		byte(slot >> 8), byte(slot),
		byte(useObj >> 8), byte(useObj),
		byte(useSlot >> 8), byte(useSlot),
		byte(useCom >> 8), byte(useCom),
	}
}

// seedOpPlayerUInv populates s.invs[invType] with `useObj` at slot
// `useSlot`, and registers a player-local inv listener at component
// `useCom` pointing at world-shared inv (Source = -1). Mirrors the
// fixture pattern from handler_opnpc_test.go's TestHandleOpNpcUSetsInteraction.
func seedOpPlayerUInv(t *testing.T, s *Server, p *Player, invType, useCom, useObj, useSlot int) {
	t.Helper()
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(invType, 28, inventory.StackNormal)
	inv.Items[useSlot] = &inventory.Item{Id: useObj, Count: 1}
	s.invs[invType] = inv
	p.invListenOnCom(invType, useCom, -1)
}

// TestHandleOpPlayerU_HappyPath — valid OPPLAYERU request sets target,
// targetOp = targetOpPlayerU, targetSubject.com = useObj (NAI-62: useObj
// threaded through SetInteraction for trigger-lookup override per TS
// OpPlayerUHandler.ts:77 + Player.ts:993-995), lastUseItem = useObj,
// lastUseSlot = useSlot, kind = Engine.
func TestHandleOpPlayerU_HappyPath(t *testing.T) {
	s, clicker, other, _ := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const (
		invType = 93
		useCom  = 149
		useObj  = 1511
		useSlot = 3
	)

	// Seed component so the component gate passes.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		useCom: {RootLayer: useCom, Interactable: true},
	})
	clicker.tabs[0] = useCom

	seedOpPlayerUInv(t, s, clicker, invType, useCom, useObj, useSlot)

	if err := handleOpPlayerU(clicker, opPlayerUPayload(other.slot, useObj, useSlot, useCom)); err != nil {
		t.Fatalf("handleOpPlayerU: %v", err)
	}

	if clicker.target != other {
		t.Errorf("target: got %v, want other (%p)", clicker.target, other)
	}
	if clicker.targetOp != targetOpPlayerU {
		t.Errorf("targetOp: got %d, want targetOpPlayerU (%d)", clicker.targetOp, targetOpPlayerU)
	}
	if clicker.targetSubject.com != useObj {
		t.Errorf("targetSubject.com: got %d, want %d (useObj — NAI-62 producer fix per TS OpPlayerUHandler.ts:67)", clicker.targetSubject.com, useObj)
	}
	if clicker.lastUseItem != useObj {
		t.Errorf("lastUseItem: got %d, want %d (useObj)", clicker.lastUseItem, useObj)
	}
	if clicker.lastUseSlot != useSlot {
		t.Errorf("lastUseSlot: got %d, want %d", clicker.lastUseSlot, useSlot)
	}
	if clicker.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", clicker.interactionKind)
	}
}

// TestHandleOpPlayerU_UseObjZeroCanonicalisation pins the TS truthy quirk
// (PathingEntity.ts:520) end-to-end: when useObj=0 from the wire, the
// producer threads it through SetInteraction, which canonicalises 0 → -1.
// NAI-62 — verifies §3.1 + §3.2 compose correctly.
func TestHandleOpPlayerU_UseObjZeroCanonicalisation(t *testing.T) {
	s, clicker, other, _ := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	const (
		invType = 93
		useCom  = 149
		useObj  = 0 // <-- TS truthy boundary
		useSlot = 3
	)

	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		useCom: {RootLayer: useCom, Interactable: true},
	})
	clicker.tabs[0] = useCom

	seedOpPlayerUInv(t, s, clicker, invType, useCom, useObj, useSlot)

	if err := handleOpPlayerU(clicker, opPlayerUPayload(other.slot, useObj, useSlot, useCom)); err != nil {
		t.Fatalf("handleOpPlayerU: %v", err)
	}

	if clicker.target != other {
		t.Errorf("target: got %v, want other (%p)", clicker.target, other)
	}
	if clicker.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (useObj=0 canonicalised per TS PathingEntity.ts:520)", clicker.targetSubject.com)
	}
	if clicker.lastUseItem != 0 {
		t.Errorf("lastUseItem: got %d, want 0 (useObj is preserved on lastUseItem; only com is canonicalised)", clicker.lastUseItem)
	}
}

// TestHandleOpPlayerU_DelayedSendsUnsetMapFlag — delayed clicker →
// UnsetMapFlag, no interaction set, lastUseItem unmodified.
// 244: delayed reject must NOT call clearPendingAction (TS OpPlayerUHandler.ts:18-20).
func TestHandleOpPlayerU_DelayedSendsUnsetMapFlag(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)
	clicker.delayed = true
	clicker.delayedUntil = 999
	s.currentTick = 0
	clicker.lastUseItem = 42 // sentinel: must stay unchanged on rejection
	clicker.target = clicker // sentinel: clearPendingAction would nil this; must survive

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	// 244: delayed-player reject must NOT call clearPendingAction.
	if clicker.target != clicker {
		t.Error("244: delayed-player reject must NOT call clearPendingAction (target sentinel cleared)")
	}
	if clicker.lastUseItem != 42 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 42", clicker.lastUseItem)
	}
}

// TestHandleOpPlayerU_TargetNotLoggedIn — LookupPlayerBySlot returns nil →
// UnsetMapFlag, no interaction set.
func TestHandleOpPlayerU_TargetNotLoggedIn(t *testing.T) {
	s, clicker, _, cc := makeOpPlayerFixture(t)
	const missingSlot = 99
	s.players[missingSlot] = nil
	// Seed component so the component gate passes; target-not-logged-in gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(missingSlot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerU_TargetNotVisible — target exists but rsbuf.HasPlayer
// is false → UnsetMapFlag, no interaction set.
func TestHandleOpPlayerU_TargetNotVisible(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	// Deliberately do NOT call rsbufSeesPlayer.
	// Seed component so the component gate passes; rsbuf-visibility gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for non-visible target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerU_TruncatedPayload — payload < 8 bytes → UnsetMapFlag.
func TestHandleOpPlayerU_TruncatedPayload(t *testing.T) {
	_, clicker, _, cc := makeOpPlayerFixture(t)

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, []byte{0x00, 0x02, 0x05, 0xE7}) // only 4 bytes
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for truncated payload, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
}

// TestHandleOpPlayerU_InvListenerMissing — no invListener registered for
// useCom → UnsetMapFlag, lastUseItem unmodified.
func TestHandleOpPlayerU_InvListenerMissing(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	// Seed component so the component gate passes; listener-missing gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal)
	// NO invListenOnCom.
	clicker.lastUseItem = 77 // sentinel

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing listener, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	if clicker.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 77", clicker.lastUseItem)
	}
}

// TestHandleOpPlayerU_ItemNotInSlot — registered listener resolves an
// inv where the claimed slot does NOT hold the claimed useObj →
// UnsetMapFlag, lastUseItem unmodified.
func TestHandleOpPlayerU_ItemNotInSlot(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	// Seed component so the component gate passes; item-mismatch gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 9999, Count: 1} // NOT 1511
	s.invs[93] = inv
	clicker.invListenOnCom(93, 149, -1)
	clicker.lastUseItem = 77 // sentinel

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for item mismatch, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	if clicker.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 77", clicker.lastUseItem)
	}
}

// TestHandleOpPlayerU_ComponentGate_ClearsPendingAction — nil/!visible/!interactable
// combined component check fires clearPendingAction (244 delta).
// 244: TS OpPlayerUHandler.ts:23-27 combined check now calls clearPendingAction.
func TestHandleOpPlayerU_ComponentGate_ClearsPendingAction(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	// useCom=9876 is NOT registered → nil component → component gate fires.
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 9876))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for component gate reject, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on component gate reject.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on component gate — TS OpPlayerUHandler.ts:25)", clicker.targetOp)
	}
}

// TestHandleOpPlayerU_InvListenerMissing_ClearsPendingAction — listener-missing gate
// fires clearPendingAction (244 delta).
// 244: TS OpPlayerUHandler.ts:31-35 listener check now calls clearPendingAction.
func TestHandleOpPlayerU_InvListenerMissing_ClearsPendingAction(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	// NO invListenOnCom — listener-missing gate is the discriminating condition.
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing listener, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on listener-missing gate.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on listener-missing — TS OpPlayerUHandler.ts:33)", clicker.targetOp)
	}
}

// TestHandleOpPlayerU_ItemNotInSlot_ClearsPendingAction — inv/slot/item combined
// check fires clearPendingAction (244 delta).
// 244: TS OpPlayerUHandler.ts:37-42 combined inv check now calls clearPendingAction.
func TestHandleOpPlayerU_ItemNotInSlot_ClearsPendingAction(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	// Register listener but put wrong item in slot — item-mismatch gate is the
	// discriminating condition.
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 9999, Count: 1} // NOT 1511
	s.invs[93] = inv
	clicker.invListenOnCom(93, 149, -1)
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for item mismatch, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on inv/item mismatch gate.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on item mismatch — TS OpPlayerUHandler.ts:40)", clicker.targetOp)
	}
}

// TestHandleOpPlayerU_TargetNotLoggedIn_ClearsPendingAction — target-not-found gate
// fires clearPendingAction (244 delta).
// 244: TS OpPlayerUHandler.ts:44-48 target-not-found check now calls clearPendingAction.
func TestHandleOpPlayerU_TargetNotLoggedIn_ClearsPendingAction(t *testing.T) {
	s, clicker, _, cc := makeOpPlayerFixture(t)
	const missingSlot = 99
	s.players[missingSlot] = nil
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(missingSlot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on target-not-found gate.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on target-not-found — TS OpPlayerUHandler.ts:46)", clicker.targetOp)
	}
}

// TestHandleOpPlayerU_TargetNotVisible_ClearsPendingAction — rsbuf-not-visible gate
// fires clearPendingAction (244 delta).
// 244: TS OpPlayerUHandler.ts:50-54 rsbuf check now calls clearPendingAction.
func TestHandleOpPlayerU_TargetNotVisible_ClearsPendingAction(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	// Deliberately do NOT call rsbufSeesPlayer.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)
	clicker.targetOp = 99 // sentinel: cleared by ClearPendingAction

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for non-visible target, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil; got %v", clicker.target)
	}
	// 244 ordering pin: clearPendingAction fires on rsbuf-not-visible gate.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (clearPendingAction on rsbuf-not-visible — TS OpPlayerUHandler.ts:52)", clicker.targetOp)
	}
}

// TestHandleOpPlayer1SetsOpcalled verifies success path sets p.opcalled=true.
func TestHandleOpPlayer1SetsOpcalled(t *testing.T) {
	s, clicker, other, _ := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)

	if err := handleOpPlayer1(clicker, p2Payload(other.slot)); err != nil {
		t.Fatalf("handleOpPlayer1: %v", err)
	}

	if !clicker.opcalled {
		t.Error("opcalled: want true after successful handleOpPlayer1, got false")
	}
}

// TestHandleOpPlayerU_MembersOnNonMembersServer — useObj is members-only
// and NodeMembers is false → MessageGame + UnsetMapFlag, no interaction.
// Mirrors TestHandleOpNpcUMembersOnFreeWorldRejected fixture.
func TestHandleOpPlayerU_MembersOnNonMembersServer(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	// Seed component so the component gate passes; members-free-world gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	s.cfg.NodeMembers = false
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 2000)}
	}
	s.objTypes.Configs[1511] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 1511, DebugName: "members_item"},
		Members:    true,
	}
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected MessageGame + UnsetMapFlag for members-on-free, got nothing")
	}
	if clicker.target != nil {
		t.Errorf("target should remain nil for members-on-free rejection; got %v", clicker.target)
	}
}

// TestHandleOpPlayerU_MembersOnNonMembersServerClearsPendingAction — ordering pin:
// ClearPendingAction must fire BEFORE the members-only check (after rsbuf.HasPlayer
// reject), so a stale pending action is cleared even when the members reject path fires.
// Matches TS OpPlayerUHandler.ts:66 (clearPendingAction before members check).
func TestHandleOpPlayerU_MembersOnNonMembersServerClearsPendingAction(t *testing.T) {
	s, clicker, other, cc := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, other.slot)
	// Seed component so the component gate passes; members-free-world gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	clicker.tabs[0] = 149
	s.cfg.NodeMembers = false
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 2000)}
	}
	s.objTypes.Configs[1511] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 1511, DebugName: "members_item"},
		Members:    true,
	}
	seedOpPlayerUInv(t, s, clicker, 93, 149, 1511, 3)

	// Pre-seed stale pending action — proves members reject clears it.
	clicker.targetOp = 99
	clicker.target = other

	received := drainConn(t, cc)
	_ = handleOpPlayerU(clicker, opPlayerUPayload(other.slot, 1511, 3, 149))
	clicker.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected MessageGame + UnsetMapFlag for members-on-free, got nothing")
	}
	// Ordering pin: ClearPendingAction must have run before members reject.
	if clicker.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (cleared by ClearPendingAction before members reject)", clicker.targetOp)
	}
	if clicker.target != nil {
		t.Errorf("target: got %v, want nil (cleared by ClearPendingAction before members reject)", clicker.target)
	}
}
