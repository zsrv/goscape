package script

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/protocol/revision"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// originProfile is deliberately NOT the "main" default, so a handler that
// hard-codes the default instead of reading WorldVars.NodeProfile fails here.
const originProfile = "beta"

// assertWealthOrigin checks the two origin fields every WealthEnvelope a
// script handler emits must carry: the binary's wire revision and the
// world's configured deployment profile.
func assertWealthOrigin(t *testing.T, env *eventspb.WealthEnvelope) {
	t.Helper()
	if env.Revision != uint32(revision.Expected) {
		t.Errorf("Revision = %d, want %d (revision.Expected)", env.Revision, uint32(revision.Expected))
	}
	if env.Profile != originProfile {
		t.Errorf("Profile = %q, want %q (WorldVars.NodeProfile)", env.Profile, originProfile)
	}
}

// TestObjTakeItemEnvelopeCarriesOrigin covers handlers_obj.go.
func TestObjTakeItemEnvelopeCarriesOrigin(t *testing.T) {
	cap := &capturingWealthEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	s, _, _ := newTakeItemFixture(t)
	w := s.World.(*fakeWorldTakeItem)
	w.nodeID = 7
	w.nodeProfile = originProfile

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	invType.Protect = false
	mc.invs[93] = invType
	mindrune := objtype.NewObjType(558)
	mindrune.Stackable = false
	mindrune.Cost = 100
	mc.objs[558] = mindrune
	s.Configs = mc

	s.ActiveObj = &mockActiveObj{objType: 558, x: 3210, z: 3215, level: 1, count: 2, reveal: -1}
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inventory.New(93, 28, inventory.StackNormal)}}
	s.PushInt(93)

	if err := handleObjTakeItem(s); err != nil {
		t.Fatalf("OBJ_TAKEITEM: %v", err)
	}
	if len(cap.wealthCalls) != 1 {
		t.Fatalf("EmitWealth calls: got %d, want 1", len(cap.wealthCalls))
	}
	assertWealthOrigin(t, cap.wealthCalls[0])
}

// TestInvDropItemEnvelopeCarriesOrigin covers the ItemDropped site in
// handlers_inv.go.
func TestInvDropItemEnvelopeCarriesOrigin(t *testing.T) {
	cap := &capturingWealthEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	mc.invs[testInvMain].Protect = false
	lookup.invs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 5}

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	world.nodeID = 7
	world.nodeProfile = originProfile

	runInvOpWithWorld(t, OpInvDropItem,
		[]int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 5, 100},
		lookup, mc, world)

	if len(cap.wealthCalls) != 1 {
		t.Fatalf("EmitWealth calls: got %d, want 1", len(cap.wealthCalls))
	}
	assertWealthOrigin(t, cap.wealthCalls[0])
}

// TestTradeEnvelopeCarriesOrigin covers the TradeCompleted site in
// handlers_inv.go.
func TestTradeEnvelopeCarriesOrigin(t *testing.T) {
	cap := &capturingWealthEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = false
	mc.invs[testInvBank].Protect = false
	sword := objtype.NewObjType(testObjSword)
	sword.Cost = 100
	mc.objs[testObjSword] = sword
	coin := objtype.NewObjType(testObjCoin)
	coin.Cost = 1
	mc.objs[testObjCoin] = coin

	lookup, self, self2 := newTwoPlayerInvFixture()
	self.accountIDValue = 101
	self2.accountIDValue = 202
	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjSword, Count: 1}
	lookup.self2Invs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 5}

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	world.nodeID = 7
	world.nodeProfile = originProfile

	runBothMoveInv(t, 0, []int{testInvMain, testInvBank}, lookup, mc, world, self, self2, false)

	if len(cap.wealthCalls) != 1 {
		t.Fatalf("EmitWealth calls: got %d, want 1", len(cap.wealthCalls))
	}
	assertWealthOrigin(t, cap.wealthCalls[0])
}

// TestWorldProfileWithoutWorld pins the nil-World degradation: a ScriptState
// with no world wired stamps the empty profile rather than panicking.
func TestWorldProfileWithoutWorld(t *testing.T) {
	s := &ScriptState{}
	if got := s.worldProfile(); got != "" {
		t.Errorf("worldProfile() with nil World = %q, want \"\"", got)
	}
}
