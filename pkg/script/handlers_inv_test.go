package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)

// mockInvLookup implements script.InvLookup with in-memory inventories
// keyed by typeID. Ignores the active player — tests exercise the
// handler dispatch + resolveInv plumbing, not scope routing.
type mockInvLookup struct {
	invs map[int]*inventory.Inventory
}

func (m *mockInvLookup) Get(_ ActivePlayer, typeID int) *inventory.Inventory {
	return m.invs[typeID]
}

const (
	testInvMain = 1 // stack-normal, capacity 28
	testInvBank = 2 // stack-always, capacity 100
	testObjCoin = 995
	testObjArr  = 2
)

// newTestInvLookup seeds a fresh mockInvLookup with the shared fixture:
//   - main (typeID=1): capacity 28, StackNormal
//   - bank (typeID=2): capacity 100, StackAlways
func newTestInvLookup() *mockInvLookup {
	main := inventory.New(testInvMain, 28, inventory.StackNormal)
	bank := inventory.New(testInvBank, 100, inventory.StackAlways)
	return &mockInvLookup{
		invs: map[int]*inventory.Inventory{
			testInvMain: main,
			testInvBank: bank,
		},
	}
}

// newTestInvConfigs builds a mockConfigs seeded with the inventory
// fixture types used across this file:
//   - obj 995 "coins": stackable, category 10, params[1] = 5
//   - obj 2   "arrow": stackable, category 20, params[1] = 0
//   - param 1: int, default 0
func newTestInvConfigs() *mockConfigs {
	mc := &mockConfigs{
		objs:    make(map[int]*objtype.ObjType),
		npcs:    make(map[int]*objtype.NpcType),
		locs:    make(map[int]*objtype.LocType),
		enums:   make(map[int]*objtype.EnumType),
		structs: make(map[int]*objtype.StructType),
		params:  make(map[int]*objtype.ParamType),
		invs:    make(map[int]*objtype.InvType),
	}

	coins := objtype.NewObjType(testObjCoin)
	coins.Name = "Coins"
	coins.DebugName = "coins"
	coins.Stackable = true
	coins.Category = 10
	coins.Params = objtype.ParamMap{1: uint32(5)}
	mc.objs[testObjCoin] = coins

	arrow := objtype.NewObjType(testObjArr)
	arrow.Name = "Arrow"
	arrow.DebugName = "arrow"
	arrow.Stackable = true
	arrow.Category = 20
	arrow.Params = objtype.ParamMap{1: uint32(0)}
	mc.objs[testObjArr] = arrow

	pInt := objtype.NewParamType(1)
	pInt.DebugName = "p_int"
	pInt.Type = objtype.ScriptVarTypeInt
	pInt.DefaultInt = 0
	mc.params[1] = pInt

	return mc
}

// runInvOp executes a single INV_* opcode against the given lookup and
// configs with intInputs pre-pushed onto the int stack (bottom →
// leftmost). Returns the post-execution state.
func runInvOp(t *testing.T, op Opcode, intInputs []int, lookup InvLookup, configs Configs) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Inv = lookup
	state.Configs = configs
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("%s: unexpected error: %v", op.String(), err)
	}
	return state
}

// runInvOpExpectErr runs a single INV_* opcode and asserts that Execute
// returns an error whose message contains substr.
func runInvOpExpectErr(t *testing.T, op Opcode, intInputs []int, lookup InvLookup, configs Configs, substr string) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Inv = lookup
	state.Configs = configs
	for _, v := range intInputs {
		state.PushInt(v)
	}
	err := Execute(state)
	if err == nil {
		t.Fatalf("%s: expected error containing %q, got nil", op.String(), substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("%s: expected error containing %q, got %q", op.String(), substr, err.Error())
	}
}

// -- Reads --

func TestInvAddThenTotal(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// ADD bank, coins, 42.
	runInvOp(t, OpInvAdd, []int{testInvBank, testObjCoin, 42}, lookup, mc)
	// TOTAL bank, coins.
	state := runInvOp(t, OpInvTotal, []int{testInvBank, testObjCoin}, lookup, mc)
	if got := state.PopInt(); got != 42 {
		t.Errorf("INV_TOTAL after ADD 42 coins: got %d, want 42", got)
	}
}

func TestInvDel(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOp(t, OpInvAdd, []int{testInvBank, testObjCoin, 50}, lookup, mc)
	runInvOp(t, OpInvDel, []int{testInvBank, testObjCoin, 10}, lookup, mc)
	state := runInvOp(t, OpInvTotal, []int{testInvBank, testObjCoin}, lookup, mc)
	if got := state.PopInt(); got != 40 {
		t.Errorf("INV_TOTAL after ADD 50, DEL 10: got %d, want 40", got)
	}
}

func TestInvDelSlot(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// SETSLOT main, slot=0, coins, 5.
	runInvOp(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 5}, lookup, mc)
	// DELSLOT main, 0.
	runInvOp(t, OpInvDelSlot, []int{testInvMain, 0}, lookup, mc)
	// GETNUM main, 0 → 0.
	state := runInvOp(t, OpInvGetNum, []int{testInvMain, 0}, lookup, mc)
	if got := state.PopInt(); got != 0 {
		t.Errorf("INV_GETNUM after DELSLOT: got %d, want 0", got)
	}
}

func TestInvGetObjEmptySlot(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// GETOBJ main, 3 on an empty inv.
	state := runInvOp(t, OpInvGetObj, []int{testInvMain, 3}, lookup, mc)
	if got := state.PopInt(); got != -1 {
		t.Errorf("INV_GETOBJ empty slot: got %d, want -1", got)
	}
}

func TestInvGetObjFilled(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOp(t, OpInvSetSlot, []int{testInvMain, 2, testObjCoin, 7}, lookup, mc)
	state := runInvOp(t, OpInvGetObj, []int{testInvMain, 2}, lookup, mc)
	if got := state.PopInt(); got != testObjCoin {
		t.Errorf("INV_GETOBJ after SETSLOT: got %d, want %d", got, testObjCoin)
	}
}

func TestInvSize(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	state := runInvOp(t, OpInvSize, []int{testInvMain}, lookup, mc)
	if got := state.PopInt(); got != 28 {
		t.Errorf("INV_SIZE(main): got %d, want 28", got)
	}
}

func TestInvClear(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// Fill a handful of slots.
	runInvOp(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}, lookup, mc)
	runInvOp(t, OpInvSetSlot, []int{testInvMain, 1, testObjCoin, 1}, lookup, mc)
	runInvOp(t, OpInvSetSlot, []int{testInvMain, 5, testObjCoin, 1}, lookup, mc)
	// CLEAR main.
	runInvOp(t, OpInvClear, []int{testInvMain}, lookup, mc)
	// FREESPACE main → 28.
	state := runInvOp(t, OpInvFreeSpace, []int{testInvMain}, lookup, mc)
	if got := state.PopInt(); got != 28 {
		t.Errorf("INV_FREESPACE after CLEAR: got %d, want 28", got)
	}
}

func TestInvFreeSpace(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// Empty → 28.
	state := runInvOp(t, OpInvFreeSpace, []int{testInvMain}, lookup, mc)
	if got := state.PopInt(); got != 28 {
		t.Errorf("INV_FREESPACE empty: got %d, want 28", got)
	}
	// Fill 5 slots via SETSLOT.
	for i := 0; i < 5; i++ {
		runInvOp(t, OpInvSetSlot, []int{testInvMain, i, testObjCoin, 1}, lookup, mc)
	}
	state = runInvOp(t, OpInvFreeSpace, []int{testInvMain}, lookup, mc)
	if got := state.PopInt(); got != 23 {
		t.Errorf("INV_FREESPACE after 5 SETSLOT: got %d, want 23", got)
	}
}

func TestInvItemSpace_HasSpace(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// Empty main, non-stackable obj (use a scratch non-stackable id).
	sword := objtype.NewObjType(3)
	sword.DebugName = "sword"
	sword.Stackable = false
	mc.objs[3] = sword
	// ITEMSPACE main, sword, 1, size=28 — empty, fits.
	state := runInvOp(t, OpInvItemSpace, []int{testInvMain, 3, 1, 28}, lookup, mc)
	if got := state.PopInt(); got != 1 {
		t.Errorf("INV_ITEMSPACE empty fits: got %d, want 1", got)
	}
}

func TestInvItemSpace_NoSpace(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	sword := objtype.NewObjType(3)
	sword.DebugName = "sword"
	sword.Stackable = false
	mc.objs[3] = sword
	// Fill all 28 slots of main.
	for i := 0; i < 28; i++ {
		runInvOp(t, OpInvSetSlot, []int{testInvMain, i, 3, 1}, lookup, mc)
	}
	// ITEMSPACE main, sword, 1, size=28 — no slots free.
	state := runInvOp(t, OpInvItemSpace, []int{testInvMain, 3, 1, 28}, lookup, mc)
	if got := state.PopInt(); got != 0 {
		t.Errorf("INV_ITEMSPACE full: got %d, want 0", got)
	}
}

func TestInvItemSpace2_Overflow(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	sword := objtype.NewObjType(3)
	sword.DebugName = "sword"
	sword.Stackable = false
	mc.objs[3] = sword
	// Fill 27 of 28 slots → 1 free.
	for i := 0; i < 27; i++ {
		runInvOp(t, OpInvSetSlot, []int{testInvMain, i, 3, 1}, lookup, mc)
	}
	// ITEMSPACE2 main, sword, 2, size=28 — overflow = 1.
	state := runInvOp(t, OpInvItemSpace2, []int{testInvMain, 3, 2, 28}, lookup, mc)
	if got := state.PopInt(); got != 1 {
		t.Errorf("INV_ITEMSPACE2 overflow: got %d, want 1", got)
	}
}

// -- Mutations --

func TestInvMoveItem(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// Pre-seed main slot 0 with 50 coins directly (SETSLOT bypasses
	// pkg/inventory.Add's per-unit distribution for StackNormal invs,
	// so we can model a stacked coin pile without depending on that
	// path).
	runInvOp(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 50}, lookup, mc)
	// MOVEITEM main → bank, coins, 30.
	runInvOp(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 30}, lookup, mc)

	mainInv := lookup.invs[testInvMain]
	bankInv := lookup.invs[testInvBank]
	if got := mainInv.GetItemCount(testObjCoin); got != 20 {
		t.Errorf("main coin count after MOVEITEM: got %d, want 20", got)
	}
	if got := bankInv.GetItemCount(testObjCoin); got != 30 {
		t.Errorf("bank coin count after MOVEITEM: got %d, want 30", got)
	}
}

func TestInvMoveFromSlot(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOp(t, OpInvSetSlot, []int{testInvMain, 4, testObjCoin, 15}, lookup, mc)
	// MOVEFROMSLOT main → bank, fromSlot=4.
	runInvOp(t, OpInvMoveFromSlot, []int{testInvMain, testInvBank, 4}, lookup, mc)

	mainInv := lookup.invs[testInvMain]
	bankInv := lookup.invs[testInvBank]
	if got := mainInv.Get(4); got != nil {
		t.Errorf("main slot 4 after MOVEFROMSLOT: got %+v, want nil", got)
	}
	if got := bankInv.GetItemCount(testObjCoin); got != 15 {
		t.Errorf("bank coin count after MOVEFROMSLOT: got %d, want 15", got)
	}
}

func TestInvMoveToSlot(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOp(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 11}, lookup, mc)
	runInvOp(t, OpInvSetSlot, []int{testInvMain, 5, testObjArr, 7}, lookup, mc)
	// MOVETOSLOT main → main, from=0, to=5 (swap within same inv).
	runInvOp(t, OpInvMoveToSlot, []int{testInvMain, testInvMain, 0, 5}, lookup, mc)

	mainInv := lookup.invs[testInvMain]
	slot0 := mainInv.Get(0)
	slot5 := mainInv.Get(5)
	if slot0 == nil || slot0.Id != testObjArr || slot0.Count != 7 {
		t.Errorf("main slot 0 after swap: got %+v, want arrow x7", slot0)
	}
	if slot5 == nil || slot5.Id != testObjCoin || slot5.Count != 11 {
		t.Errorf("main slot 5 after swap: got %+v, want coin x11", slot5)
	}
}

func TestInvTotalParam(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// main is StackNormal and pkg/inventory.Add distributes non-stack
	// invs one unit per slot. 5 coins → 5 slots, each with count=1.
	// INV_TOTALPARAM sums the param value per non-empty slot (no count
	// multiply — that's TOTALPARAM_STACK). params[1]=5 × 5 slots = 25.
	runInvOp(t, OpInvAdd, []int{testInvMain, testObjCoin, 5}, lookup, mc)
	state := runInvOp(t, OpInvTotalParam, []int{testInvMain, 1}, lookup, mc)
	if got := state.PopInt(); got != 25 {
		t.Errorf("INV_TOTALPARAM: got %d, want 25", got)
	}
}

func TestInvTotalCat(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// 3 coins (cat=10, 3 slots × 1 count) + 2 arrows (cat=20, 2 slots
	// × 1 count). TOTALCAT for cat=10 sums the count of coin slots.
	runInvOp(t, OpInvAdd, []int{testInvMain, testObjCoin, 3}, lookup, mc)
	runInvOp(t, OpInvAdd, []int{testInvMain, testObjArr, 2}, lookup, mc)
	state := runInvOp(t, OpInvTotalCat, []int{testInvMain, 10}, lookup, mc)
	if got := state.PopInt(); got != 3 {
		t.Errorf("INV_TOTALCAT(10 coins): got %d, want 3", got)
	}
}

// -- Negative paths --

func TestInvLookupNilReturnsError(t *testing.T) {
	mc := newTestInvConfigs()
	// No lookup: every INV_* mutation / read that needs inv must error.
	runInvOpExpectErr(t, OpInvTotal, []int{testInvMain, testObjCoin}, nil, mc, "no inv for type")
	runInvOpExpectErr(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, nil, mc, "no inv for type")
	runInvOpExpectErr(t, OpInvClear, []int{testInvMain}, nil, mc, "no inv for type")
}

// TestInvTransmitRegistersListener runs a script pushing (com, inv) then
// OpInvTransmit; asserts the mock player recorded
// InvListenOnCom(invType, com, -1). Matches TS InvOps.ts INV_TRANSMIT.
func TestInvTransmitRegistersListener(t *testing.T) {
	mp := &mockPlayer{}

	sf := &ScriptFile{
		Name: "inv_transmit",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // inv (top)
			OpInvTransmit,
			OpReturn,
		},
		IntOperands:      []int32{149, 93, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastInvListenOnCom) != 1 {
		t.Fatalf("expected 1 call to InvListenOnCom, got %d", len(mp.lastInvListenOnCom))
	}
	got := mp.lastInvListenOnCom[0]
	if got.InvType != 93 || got.Com != 149 || got.Source != -1 {
		t.Errorf("InvListenOnCom args: got %+v, want {InvType:93, Com:149, Source:-1}", got)
	}
}

// TestInvTransmitNoActivePlayerErrors verifies INV_TRANSMIT returns
// an error when PtrActivePlayer is not set.
func TestInvTransmitNoActivePlayerErrors(t *testing.T) {
	sf := newSingleOp("inv_transmit_no_player", OpInvTransmit)
	state := Init(sf, nil, false, nil, nil)
	state.PushInt(93)
	state.PushInt(149)

	err := Execute(state)
	if err == nil || err.Error() != "INV_TRANSMIT: no active player" {
		t.Errorf("expected 'INV_TRANSMIT: no active player' error, got %v", err)
	}
}

// TestInvStopTransmitUnregistersListener runs a script pushing com then
// OpInvStopTransmit; asserts mockPlayer recorded InvStopListenOnCom(com).
func TestInvStopTransmitUnregistersListener(t *testing.T) {
	mp := &mockPlayer{}

	sf := &ScriptFile{
		Name: "inv_stoptransmit",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpInvStopTransmit,
			OpReturn,
		},
		IntOperands:      []int32{149, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastInvStopListenOnCom) != 1 || mp.lastInvStopListenOnCom[0] != 149 {
		t.Errorf("InvStopListenOnCom: got %v, want [149]", mp.lastInvStopListenOnCom)
	}
}

// TestInvStopTransmitNoActivePlayerErrors verifies INV_STOPTRANSMIT
// returns an error when PtrActivePlayer is not set.
func TestInvStopTransmitNoActivePlayerErrors(t *testing.T) {
	sf := newSingleOp("inv_stoptransmit_no_player", OpInvStopTransmit)
	state := Init(sf, nil, false, nil, nil)
	state.PushInt(149)

	err := Execute(state)
	if err == nil || err.Error() != "INV_STOPTRANSMIT: no active player" {
		t.Errorf("expected 'INV_STOPTRANSMIT: no active player' error, got %v", err)
	}
}

// TestInvOtherTransmitRegistersListenerWithUid — happy path for
// OpInvOtherTransmit. Closes S6u-SB1.
func TestInvOtherTransmitRegistersListenerWithUid(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "invother_transmit",
		Opcodes: []Opcode{
			OpPushConstantInt, // uid
			OpPushConstantInt, // inv
			OpPushConstantInt, // com (top)
			OpInvOtherTransmit,
			OpReturn,
		},
		IntOperands:      []int32{42, 93, 149, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastInvListenOnCom) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mp.lastInvListenOnCom))
	}
	got := mp.lastInvListenOnCom[0]
	if got.InvType != 93 || got.Com != 149 || got.Source != 42 {
		t.Errorf("args: got %+v, want {InvType:93, Com:149, Source:42}", got)
	}
}

// TestInvOtherTransmitNoActivePlayerErrors verifies requireActivePlayer gate.
func TestInvOtherTransmitNoActivePlayerErrors(t *testing.T) {
	sf := newSingleOp("invother_transmit_no_player", OpInvOtherTransmit)
	state := Init(sf, nil, false, nil, nil)
	state.PushInt(42)  // uid
	state.PushInt(93)  // inv
	state.PushInt(149) // com

	err := Execute(state)
	if err == nil || err.Error() != "INVOTHER_TRANSMIT: no active player" {
		t.Errorf("expected 'INVOTHER_TRANSMIT: no active player', got %v", err)
	}
}

// -- NAI-23 Bundle 4b: NumberNotNull null-pin tests --------------------------

// TestHandleInvTransmitNullComRejected pins NAI-23 Bundle 4b: INV_TRANSMIT
// rejects com=-1 via checkNotNull (TS InvOps.ts INV_TRANSMIT:
// check(com, NumberNotNull)). invType is wrapped with InvTypeValid (not
// NumberNotNull) and stays raw. The InvListenOnCom side-effect must NOT occur.
//
// Pop order: invType first (top of stack), com second (bottom of stack).
// Push com=-1 first (bottom), then invType=93 on top.
func TestHandleInvTransmitNullComRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "inv_transmit_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // push com (-1, bottom)
			OpPushConstantInt, // push invType (93, top)
			OpInvTransmit,
			OpReturn,
		},
		IntOperands: []int32{-1, 93, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "INV_TRANSMIT: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.lastInvListenOnCom) != 0 {
		t.Errorf("lastInvListenOnCom: got %d calls, want 0 (must not register on rejection)", len(mp.lastInvListenOnCom))
	}
}

// TestHandleInvStopTransmitNullComRejected pins NAI-23 Bundle 4b:
// INV_STOPTRANSMIT rejects com=-1 via checkNotNull (TS InvOps.ts
// INV_STOPTRANSMIT: check(state.popInt(), NumberNotNull)). The
// InvStopListenOnCom side-effect must NOT occur.
func TestHandleInvStopTransmitNullComRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "inv_stoptransmit_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // push com (-1)
			OpInvStopTransmit,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "INV_STOPTRANSMIT: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.lastInvStopListenOnCom) != 0 {
		t.Errorf("lastInvStopListenOnCom: got %d calls, want 0 (must not unregister on rejection)", len(mp.lastInvStopListenOnCom))
	}
}

// TestHandleInvOtherTransmitNullRejected pins NAI-23 Bundle 4b: INVOTHER_TRANSMIT
// rejects com=-1 and uid=-1 via checkNotNull (TS InvOps.ts INVOTHER_TRANSMIT:
// check(uid, NumberNotNull) and check(com, NumberNotNull)).
// invType is wrapped with InvTypeValid (not NumberNotNull) and stays raw.
//
// Pop order: com (top, first pop), invType (second pop), uid (bottom, third pop).
// Table-driven: one sub-test per null slot.
func TestHandleInvOtherTransmitNullRejected(t *testing.T) {
	tests := []struct {
		name string
		// Push order: bottom → top == uid, invType, com.
		uid, invType, com int
		wantSubstr        string
	}{
		{
			name:       "null_com",
			uid:        42,
			invType:    93,
			com:        -1,
			wantSubstr: "INVOTHER_TRANSMIT: input number was null(-1)",
		},
		{
			name:       "null_uid",
			uid:        -1,
			invType:    93,
			com:        149,
			wantSubstr: "INVOTHER_TRANSMIT: input number was null(-1)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{}
			sf := &ScriptFile{
				Name: "invother_transmit_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt, // push uid (bottom)
					OpPushConstantInt, // push invType
					OpPushConstantInt, // push com (top)
					OpInvOtherTransmit,
					OpReturn,
				},
				IntOperands: []int32{int32(tc.uid), int32(tc.invType), int32(tc.com), 0, 0},
			}
			state := Init(sf, mp, false, nil, nil)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if len(mp.lastInvListenOnCom) != 0 {
				t.Errorf("lastInvListenOnCom: got %d calls, want 0", len(mp.lastInvListenOnCom))
			}
		})
	}
}
