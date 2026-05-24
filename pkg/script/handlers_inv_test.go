package script

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
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
	testInvMain     = 1 // stack-normal, capacity 28
	testInvBank     = 2 // stack-always, capacity 100
	testObjCoin     = 995
	testObjArr      = 2
	testObjSword    = 3 // non-stackable scratch obj for per-slot semantics tests
	testObjCertNote = 4 // certificate item (UNCERT direction): CertTemplate=self(>=0), CertLink=testObjCoin
	testObjLogs     = 5 // certifiable item (CERT direction): CertTemplate=-1, CertLink=testObjCertNote
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
//   - obj 3   "sword": non-stackable, category 10
//   - param 1: int, default 0
//   - inv 1 "main": size 28, scope TEMP, protect=false
//   - inv 2 "bank": size 100, scope SHARED, protect=false
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

	sword := objtype.NewObjType(testObjSword)
	sword.Name = "Sword"
	sword.DebugName = "sword"
	sword.Stackable = false
	sword.Category = 10
	sword.Params = objtype.ParamMap{1: uint32(5)}
	mc.objs[testObjSword] = sword

	certNote := objtype.NewObjType(testObjCertNote)
	certNote.DebugName = "cert_note"
	certNote.CertTemplate = testObjCertNote // self-referential (>=0) satisfies UNCERT gate
	certNote.CertLink = testObjCoin
	certNote.Stackable = true
	mc.objs[testObjCertNote] = certNote

	logs := objtype.NewObjType(testObjLogs)
	logs.DebugName = "logs"
	logs.CertTemplate = -1          // CertTemplate==-1 satisfies the CERT gate (INVERTED vs UNCERT)
	logs.CertLink = testObjCertNote // certifies TO cert note
	logs.Stackable = true           // "should be a stackable cert already" per TS comment
	mc.objs[testObjLogs] = logs

	mainInv := objtype.NewInvType(testInvMain)
	mainInv.DebugName = "main"
	mainInv.Size = 28
	mainInv.Scope = objtype.InvTypeScopeTemp
	mainInv.Protect = false // override NewInvType default so existing tests don't trip the protect/scope gate
	mc.invs[testInvMain] = mainInv

	bankInv := objtype.NewInvType(testInvBank)
	bankInv.DebugName = "bank"
	bankInv.Size = 100
	bankInv.Scope = objtype.InvTypeScopeShared
	bankInv.Protect = false
	mc.invs[testInvBank] = bankInv

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
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
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

// runInvOpExpectErrAsPlayer is the active-player variant of
// runInvOpExpectErr. Sets up a zero-value mockPlayer + PtrActivePlayer
// so the requireActivePlayer gate is satisfied; tests targeting deeper
// gates (InvTypeValid / ObjTypeValid / ObjStackValid / protect-scope /
// dummyitem) use this helper. PtrProtectedActivePlayer remains unset (Init's third
// arg) — tests that need a protected script set state.Pointers |= PtrProtectedActivePlayer
// before pushing inputs.
func runInvOpExpectErrAsPlayer(t *testing.T, op Opcode, intInputs []int, lookup InvLookup, configs Configs, substr string) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
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

// runInvOpWithWorld is the WorldVars variant of runInvOp; useful for
// handlers that overflow to world via s.World.AddObj (e.g. INV_MOVEITEM_CERT,
// INV_DROPSLOT). The active-player pointer is set unconditionally so
// requireActivePlayer gates pass; callers that need a non-player context
// should use a lower-level helper.
func runInvOpWithWorld(t *testing.T, op Opcode, intInputs []int, lookup InvLookup, configs Configs, world WorldVars) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	state.Inv = lookup
	state.Configs = configs
	state.World = world
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("%s: unexpected error: %v", op.String(), err)
	}
	return state
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

// TS InvOps.ts:622-625 runs check(inv, InvTypeValid) BEFORE the
// obj === -1 short-circuit, so an unregistered InvType id must produce
// the canonical registry-miss error rather than silently pushing 0.
func TestInvTotal_UnknownInv_ObjNeg1_RejectsRegistry(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvTotal, []int{9999, -1}, lookup, mc, "no InvType with value (9999) found")
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

// TestInvSize_NoResolvableInv_L21 pins L21: INV_SIZE is a pure config read
// (TS InvOps.ts:27-31 state.pushInt(invType.size)) and must succeed even when
// no player inventory is resolvable (nil lookup → resolveInv returns nil).
// Pre-L21 the handler resolved a live inv and errored here.
func TestInvSize_NoResolvableInv_L21(t *testing.T) {
	mc := newTestInvConfigs()
	state := runInvOp(t, OpInvSize, []int{testInvMain}, nil, mc)
	if got := state.PopInt(); got != 28 {
		t.Errorf("INV_SIZE(main) with no resolvable inv: got %d, want 28", got)
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

// -- INV_ITEMSPACE / INV_ITEMSPACE2 validator tests (TS InvOps.ts:286-303) --
//
// Mirror TS check(obj, ObjTypeValid) and check(count, ObjStackValid)
// inserted after the InvTypeValid gate. The count==0 fast-path runs
// BEFORE the validators (matches TS:289-292), so 0-count ObjStackValid
// is unreachable; we cover negative + above-StackLimit instead.

func TestInvItemSpace_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// obj id 9999 is not registered in newTestInvConfigs.
	runInvOpExpectErr(t, OpInvItemSpace, []int{testInvMain, 9999, 1, 28}, lookup, mc, "INV_ITEMSPACE: no ObjType with value (9999) found")
}

func TestInvItemSpace_ObjStackValid_CountNegative(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvItemSpace, []int{testInvMain, testObjCoin, -1, 28}, lookup, mc, "INV_ITEMSPACE: invalid count (-1)")
}

func TestInvItemSpace2_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvItemSpace2, []int{testInvMain, 9999, 1, 28}, lookup, mc, "INV_ITEMSPACE2: no ObjType with value (9999) found")
}

func TestInvItemSpace2_ObjStackValid_CountNegative(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvItemSpace2, []int{testInvMain, testObjCoin, -1, 28}, lookup, mc, "INV_ITEMSPACE2: invalid count (-1)")
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

// TestInvMoveItem_OverflowDropsToFloor pins H7: items removed from the source
// but rejected by a full destination drop to the floor (owned by the active
// player, 200t) instead of vanishing. Mirrors TS InvOps.ts:521-530.
func TestInvMoveItem_OverflowDropsToFloor(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	w := newFakeWorldMembers()

	// Destination (main, 28 slots, StackNormal) is full of non-stackable
	// swords so it cannot accept any coins.
	main := lookup.invs[testInvMain]
	for i := range 28 {
		main.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
	}
	// Source (bank) holds 30 coins.
	bank := lookup.invs[testInvBank]
	bank.Items[0] = &inventory.Item{Id: testObjCoin, Count: 30}

	runInvOpWithWorld(t, OpInvMoveItem, []int{testInvBank, testInvMain, testObjCoin, 30}, lookup, mc, w)

	if got := bank.GetItemCount(testObjCoin); got != 0 {
		t.Errorf("bank coin after move: got %d, want 0 (all removed)", got)
	}
	if got := main.GetItemCount(testObjCoin); got != 0 {
		t.Errorf("main coin after move: got %d, want 0 (full, none accepted)", got)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("expected 1 AddObj (stackable overflow), got %d", len(w.addedCalls))
	}
	if got := w.addedCalls[0]; got.typeID != testObjCoin || got.count != 30 || got.duration != 200 {
		t.Errorf("AddObj: got %+v, want {typeID:%d count:30 duration:200}", got, testObjCoin)
	}
}

// TestInvMoveFromSlot_OverflowDropsToFloor pins H8: the whole source slot is
// removed; whatever the destination can't hold drops to the floor. Mirrors TS
// Player.invMoveFromSlot + InvOps.ts:339-347.
func TestInvMoveFromSlot_OverflowDropsToFloor(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	w := newFakeWorldMembers()

	main := lookup.invs[testInvMain]
	for i := range 28 {
		main.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
	}
	bank := lookup.invs[testInvBank]
	bank.Items[4] = &inventory.Item{Id: testObjCoin, Count: 15}

	runInvOpWithWorld(t, OpInvMoveFromSlot, []int{testInvBank, testInvMain, 4}, lookup, mc, w)

	if got := bank.Get(4); got != nil {
		t.Errorf("bank slot 4 after move: got %+v, want nil (slot emptied)", got)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("expected 1 AddObj (stackable overflow), got %d", len(w.addedCalls))
	}
	if got := w.addedCalls[0]; got.typeID != testObjCoin || got.count != 15 || got.duration != 200 {
		t.Errorf("AddObj: got %+v, want {typeID:%d count:15 duration:200}", got, testObjCoin)
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

func TestInvMoveToSlot_NoActivePlayer(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvMoveToSlot, []int{testInvMain, testInvBank, 0, 0}, lookup, mc, "INV_MOVETOSLOT: no active player")
}

func TestInvMoveToSlot_FromInvTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveToSlot, []int{9999, testInvBank, 0, 0}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveToSlot_ToInvTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveToSlot, []int{testInvMain, 9999, 0, 0}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveToSlot_FromProtectedRejectsUnprotected(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	mc.invs[testInvMain].Scope = objtype.InvTypeScopePerm
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveToSlot, []int{testInvMain, testInvBank, 0, 0}, lookup, mc, "INV_MOVETOSLOT: $inv requires protected access")
}

func TestInvMoveToSlot_ToProtectedAsymmetricD1(t *testing.T) {
	// DEVIATION-NAI-131-D1: to-gate evaluates fromInvType.Scope, NOT toInvType.Scope.
	// fromInvType.Scope=Shared but toInvType.Protect=true → to-gate must NOT fire (gated by from's scope).
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Scope = objtype.InvTypeScopeShared
	mc.invs[testInvBank].Protect = true
	mc.invs[testInvBank].Scope = objtype.InvTypeScopePerm
	lookup := newTestInvLookup()
	st := runInvOp(t, OpInvMoveToSlot, []int{testInvMain, testInvBank, 0, 0}, lookup, mc)
	if st == nil {
		t.Fatal("expected handler to complete without error")
	}
}

func TestInvMoveToSlot_ToProtectedSamefromScopePerm(t *testing.T) {
	// Inverse pin: fromInvType.Scope=Perm + toInvType.Protect=true → to-gate fires.
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Scope = objtype.InvTypeScopePerm
	mc.invs[testInvBank].Protect = true
	mc.invs[testInvBank].Scope = objtype.InvTypeScopeShared // ignored — gate uses from's scope
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveToSlot, []int{testInvMain, testInvBank, 0, 0}, lookup, mc, "INV_MOVETOSLOT: $inv requires protected access")
}

func TestInvTotalParam(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// testObjSword is non-stackable, so 5 swords distribute one-per-slot
	// in main (StackNormal): 5 slots × 1 count. INV_TOTALPARAM sums the
	// param value per non-empty slot (no count multiply — that's
	// TOTALPARAM_STACK). params[1]=5 × 5 slots = 25.
	runInvOp(t, OpInvAdd, []int{testInvMain, testObjSword, 5}, lookup, mc)
	state := runInvOp(t, OpInvTotalParam, []int{testInvMain, 1}, lookup, mc)
	if got := state.PopInt(); got != 25 {
		t.Errorf("INV_TOTALPARAM: got %d, want 25", got)
	}
}

func TestInvTotalCat(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// 3 swords (cat=10, non-stackable: 3 slots × 1 count) + 2 arrows
	// (cat=20, stackable: 1 slot — fixture aside; cat-mismatch means
	// count is irrelevant). TOTALCAT for cat=10 sums the count of sword
	// slots: 3 slots × 1 count = 3.
	runInvOp(t, OpInvAdd, []int{testInvMain, testObjSword, 3}, lookup, mc)
	runInvOp(t, OpInvAdd, []int{testInvMain, testObjArr, 2}, lookup, mc)
	state := runInvOp(t, OpInvTotalCat, []int{testInvMain, 10}, lookup, mc)
	if got := state.PopInt(); got != 3 {
		t.Errorf("INV_TOTALCAT(10 swords): got %d, want 3", got)
	}
}

// -- Negative paths --

func TestInvLookupNilReturnsError(t *testing.T) {
	mc := newTestInvConfigs()
	// No lookup: every INV_* mutation / read that needs inv must error.
	runInvOpExpectErr(t, OpInvTotal, []int{testInvMain, testObjCoin}, nil, mc, "no inv for type")
	runInvOpExpectErr(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, nil, mc, "no active player")
	runInvOpExpectErr(t, OpInvClear, []int{testInvMain}, nil, mc, "no active player")
}

// TestInvTransmitRegistersListener runs a script pushing (inv, com) then
// OpInvTransmit; asserts the mock player recorded
// InvListenOnCom(invType, com, activePlayer.uid). Matches TS InvOps.ts
// INV_TRANSMIT (`const [inv, com] = state.popInts(2)` — inv pushed first,
// com second). Pins post-NAI-24-Bundle-2 behavior; pre-fix this test
// asserted Source: -1 (S6u porting bug at commit fa57ee4). Push order
// migrated by NAI-113 T9 to match runescript-compiler convention; the
// previous (com, inv) order was hand-tuned to a buggy handler pop order
// that masked a side-panel emission bug in production.
func TestInvTransmitRegistersListener(t *testing.T) {
	mp := &mockPlayer{uidValue: 42}

	sf := &ScriptFile{
		Name: "inv_transmit",
		Opcodes: []Opcode{
			OpPushConstantInt, // inv (bottom — pushed first)
			OpPushConstantInt, // com (top — pushed second)
			OpInvTransmit,
			OpReturn,
		},
		IntOperands:      []int32{testInvMain, 149, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = newTestInvConfigs()
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastInvListenOnCom) != 1 {
		t.Fatalf("expected 1 call to InvListenOnCom, got %d", len(mp.lastInvListenOnCom))
	}
	got := mp.lastInvListenOnCom[0]
	if got.InvType != testInvMain || got.Com != 149 || got.Source != 42 {
		t.Errorf("InvListenOnCom args: got %+v, want {InvType:%d, Com:149, Source:42}", got, testInvMain)
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

// TestInvTransmitSourceTracksActivePlayerUID is a complement to
// TestInvTransmitRegistersListener (line 388) using a non-trivial
// composed-uid sentinel. Pins NAI-113 cascade context: INV_TRANSMIT
// propagates the active player's UID() to the listener Source field
// regardless of how UID() obtains its value. Pre-NAI-113 production
// Player.uid was always -1 (Server.addPlayer never composed it); the
// existing literal-42 test would have masked that bug. Post-fix
// production composes uid via composeUID(username37, slot); this test
// pins the propagation chain end-to-end with a value matching
// production-realistic uid shapes.
func TestInvTransmitSourceTracksActivePlayerUID(t *testing.T) {
	const wantUID = 0xDEADBEE
	mp := &mockPlayer{uidValue: wantUID}

	sf := &ScriptFile{
		Name: "inv_transmit_uid_track",
		Opcodes: []Opcode{
			OpPushConstantInt, // inv (bottom — pushed first per TS popInts order)
			OpPushConstantInt, // com (top — pushed second)
			OpInvTransmit,
			OpReturn,
		},
		IntOperands:      []int32{testInvMain, 149, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = newTestInvConfigs()
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastInvListenOnCom) != 1 {
		t.Fatalf("expected 1 call to InvListenOnCom, got %d", len(mp.lastInvListenOnCom))
	}
	got := mp.lastInvListenOnCom[0]
	if got.Source != wantUID {
		t.Errorf("Source: got %#x, want %#x (must propagate active player UID)", got.Source, wantUID)
	}
	if got.InvType != testInvMain || got.Com != 149 {
		t.Errorf("InvType/Com mismatch: got {%d, %d}, want {%d, 149}", got.InvType, got.Com, testInvMain)
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
		IntOperands:      []int32{42, testInvMain, 149, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = newTestInvConfigs()
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastInvListenOnCom) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mp.lastInvListenOnCom))
	}
	got := mp.lastInvListenOnCom[0]
	if got.InvType != testInvMain || got.Com != 149 || got.Source != 42 {
		t.Errorf("args: got %+v, want {InvType:%d, Com:149, Source:42}", got, testInvMain)
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
// check(com, NumberNotNull)). The InvListenOnCom side-effect must NOT occur.
//
// Push order per TS popInts(2)→[inv,com]: invType first (bottom), com on top.
// Migrated by NAI-113 T9; previous order was hand-tuned to a buggy
// handler pop order.
//
// Uses a registered invType (testInvMain) so com=-1 is the rejection
// trigger (not the new InvTypeValid gate, which is exercised separately
// in TestHandleInvTransmitInvalidInvRejected).
func TestHandleInvTransmitNullComRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "inv_transmit_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // push invType (bottom — pushed first)
			OpPushConstantInt, // push com (-1, top — pushed second)
			OpInvTransmit,
			OpReturn,
		},
		IntOperands: []int32{testInvMain, -1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = newTestInvConfigs()

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
			invType:    testInvMain,
			com:        -1,
			wantSubstr: "INVOTHER_TRANSMIT: input number was null(-1)",
		},
		{
			name:       "null_uid",
			uid:        -1,
			invType:    testInvMain,
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
			state.Configs = newTestInvConfigs()

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

// TestHandleInvTransmitInvalidInvRejected pins INV_TRANSMIT inv validation
// gap closure: TS InvOps.ts:647 calls check(inv, InvTypeValid). An unregistered
// invType id must reject before InvListenOnCom is called.
func TestHandleInvTransmitInvalidInvRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "inv_transmit_invalid_inv",
		Opcodes: []Opcode{
			OpPushConstantInt, // invType (bogus, bottom)
			OpPushConstantInt, // com (top)
			OpInvTransmit,
			OpReturn,
		},
		IntOperands: []int32{9999, 149, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = newTestInvConfigs()

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for unregistered invType=9999, got nil")
	}
	want := "INV_TRANSMIT: no InvType with value (9999) found"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.lastInvListenOnCom) != 0 {
		t.Errorf("lastInvListenOnCom: got %d calls, want 0 (must not register on rejection)", len(mp.lastInvListenOnCom))
	}
}

// TestHandleInvOtherTransmitInvalidInvRejected pins INVOTHER_TRANSMIT inv
// validation gap closure: TS InvOps.ts:658 calls check(inv, InvTypeValid).
// Validation order is uid → inv → com (TS:657-659), so a valid uid plus
// unregistered invType must reject with the InvTypeValid error before the
// com check fires.
func TestHandleInvOtherTransmitInvalidInvRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "invother_transmit_invalid_inv",
		Opcodes: []Opcode{
			OpPushConstantInt, // uid (bottom)
			OpPushConstantInt, // invType (bogus)
			OpPushConstantInt, // com (top)
			OpInvOtherTransmit,
			OpReturn,
		},
		IntOperands: []int32{42, 9999, 149, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = newTestInvConfigs()

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for unregistered invType=9999, got nil")
	}
	want := "INVOTHER_TRANSMIT: no InvType with value (9999) found"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.lastInvListenOnCom) != 0 {
		t.Errorf("lastInvListenOnCom: got %d calls, want 0 (must not register on rejection)", len(mp.lastInvListenOnCom))
	}
}

// -- INV_DROPSLOT tests --

func TestHandleInvDropSlotHappyPath(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	invType.Protect = true
	invType.Scope = objtype.InvTypeScopeTemp
	mc.invs[93] = invType
	logs := objtype.NewObjType(1511)
	logs.Stackable = false
	mc.objs[1511] = logs
	s.Configs = mc

	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Set(2, &inventory.Item{Id: 1511, Count: 1})
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inv}}

	// Push order [inv, coord, slot, duration] — duration on top.
	s.PushInt(93)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(2)
	s.PushInt(100)

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("handleInvDropSlot returned error: %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("expected 1 AddObj call, got %d", len(w.addedCalls))
	}
	if got := w.addedCalls[0].typeID; got != 1511 {
		t.Errorf("AddObj typeID: got %d, want 1511", got)
	}
	if got := w.addedCalls[0].receiverID; got != 12345 {
		t.Errorf("AddObj receiverID: got %d, want 12345 (player UID)", got)
	}
	if got := w.addedCalls[0].count; got != 1 {
		t.Errorf("AddObj count: got %d, want 1", got)
	}
	if it := inv.Get(2); it != nil {
		t.Errorf("expected slot 2 cleared, got %+v", it)
	}
}

func TestHandleInvDropSlotEmptySlotErrors(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.World = newFakeWorldMembers()
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	invType.Protect = true
	invType.Scope = objtype.InvTypeScopeTemp
	mc.invs[93] = invType
	s.Configs = mc

	inv := inventory.New(93, 28, inventory.StackNormal)
	// slot 2 left empty
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inv}}

	s.PushInt(93)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(2)
	s.PushInt(100)

	if err := handleInvDropSlot(s); err == nil {
		t.Errorf("INV_DROPSLOT empty slot: expected error, got nil")
	}
}

func TestHandleInvDropSlotProtectedRequired(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.World = newFakeWorldMembers()
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	// not protected — protect gate must fire (Pointers zero-value lacks PtrProtectedActivePlayer)

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	invType.Protect = true
	invType.Scope = objtype.InvTypeScopeTemp
	mc.invs[93] = invType
	s.Configs = mc

	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Set(2, &inventory.Item{Id: 1511, Count: 1})
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inv}}

	s.PushInt(93)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(2)
	s.PushInt(100)

	if err := handleInvDropSlot(s); err == nil {
		t.Errorf("INV_DROPSLOT protect-required without PtrProtectedActivePlayer: expected error, got nil")
	}
}

func TestHandleInvDropSlotSharedScopeBypassesProtect(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	// not protected — protect gate must fire (Pointers zero-value lacks PtrProtectedActivePlayer)

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	invType.Protect = true
	invType.Scope = objtype.InvTypeScopeShared // SHARED bypasses protect gate
	mc.invs[93] = invType
	logs := objtype.NewObjType(1511)
	logs.Stackable = false
	mc.objs[1511] = logs
	s.Configs = mc

	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Set(2, &inventory.Item{Id: 1511, Count: 1})
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inv}}

	s.PushInt(93)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(2)
	s.PushInt(100)

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("INV_DROPSLOT scope=Shared: expected success, got error %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Errorf("expected 1 AddObj call, got %d", len(w.addedCalls))
	}
}

// TestHandleInvDropSlotSetsActiveObjAndPointer pins NAI-115-D3 closure:
// INV_DROPSLOT must set s.ActiveObj and PtrActiveObj after the drop spawn.
func TestHandleInvDropSlotSetsActiveObjAndPointer(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	invType.Protect = true
	invType.Scope = objtype.InvTypeScopeTemp
	mc.invs[93] = invType
	logs := objtype.NewObjType(1511)
	logs.Stackable = false
	mc.objs[1511] = logs
	s.Configs = mc

	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Set(2, &inventory.Item{Id: 1511, Count: 1})
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inv}}

	s.PushInt(93)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(2)
	s.PushInt(100)

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("handleInvDropSlot returned error: %v", err)
	}
	if s.ActiveObj == nil {
		t.Fatalf("INV_DROPSLOT: expected s.ActiveObj set, got nil")
	}
	if s.Pointers&PtrActiveObj == 0 {
		t.Errorf("INV_DROPSLOT: expected PtrActiveObj set in Pointers")
	}
}

// -- NAI-130 overflow-to-world tests --

// helper: build a fakeWorldAddObj-backed state for INV_ADD overflow tests.
// Sets up: mc with stackable + non-stackable test objs, an InvType=1
// (StackNormal capacity 28), an mockPlayer at level=0, x=3200, z=3200,
// uid=12345. Returns the state, the world recorder, and the inv (so the
// caller can pre-fill it).
func newInvAddOverflowState(t *testing.T) (*ScriptState, *fakeWorldAddObj, *inventory.Inventory) {
	t.Helper()
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(testInvMain)
	invType.Size = 28
	invType.Protect = false // NewInvType defaults Protect=true; clear so NAI-130 overflow tests don't trip the NAI-131 T1 protect gate.
	mc.invs[testInvMain] = invType
	s.Configs = mc

	inv := inventory.New(testInvMain, 28, inventory.StackNormal)
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{testInvMain: inv}}

	s.Self = &mockPlayer{
		uidValue:    12345,
		coordPacked: coordgrid.PackCoord(0, 3200, 3200),
		x:           3200,
		z:           3200,
	}
	s.Pointers |= PtrActivePlayer
	return s, w, inv
}

// (1) No-overflow regression: bag has space; no AddObj calls.
func TestInvAdd_NoOverflow_NoWorldAddObj(t *testing.T) {
	s, w, _ := newInvAddOverflowState(t)
	s.PushInt(testInvMain)
	s.PushInt(testObjCoin)
	s.PushInt(5)
	if err := handleInvAdd(s); err != nil {
		t.Fatalf("handleInvAdd: %v", err)
	}
	if len(w.addedCalls) != 0 {
		t.Errorf("no overflow → no AddObj calls, got %d", len(w.addedCalls))
	}
}

// (2) Stackable overflow > 1: full bag (no existing stack) + stackable
// obj + overflow=5 → 1 AddObj with count=5.
func TestInvAdd_StackableOverflow_GreaterThanOne_SingleDrop(t *testing.T) {
	s, w, inv := newInvAddOverflowState(t)
	// Fill all 28 slots with OTHER (non-stackable) objs so free=0 and
	// the stackable obj has no existing stack.
	for i := range 28 {
		inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
	}
	s.PushInt(testInvMain)
	s.PushInt(testObjCoin) // Stackable=true per newTestInvConfigs
	s.PushInt(5)
	if err := handleInvAdd(s); err != nil {
		t.Fatalf("handleInvAdd: %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("stackable overflow=5: expected 1 AddObj call, got %d", len(w.addedCalls))
	}
	got := w.addedCalls[0]
	want := addObjCall{level: 0, x: 3200, z: 3200, typeID: testObjCoin, count: 5, duration: 200, receiverID: 12345}
	if got != want {
		t.Errorf("AddObj: got %+v, want %+v", got, want)
	}
}

// (3) Stackable overflow == 1: TS line 75 special case — even stackable,
// overflow=1 emits 1 single-count drop.
func TestInvAdd_StackableOverflow_EqualsOne_SingleDrop(t *testing.T) {
	s, w, inv := newInvAddOverflowState(t)
	for i := range 28 {
		inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
	}
	s.PushInt(testInvMain)
	s.PushInt(testObjCoin)
	s.PushInt(1)
	if err := handleInvAdd(s); err != nil {
		t.Fatalf("handleInvAdd: %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("stackable overflow=1: expected 1 AddObj call, got %d", len(w.addedCalls))
	}
	if got := w.addedCalls[0].count; got != 1 {
		t.Errorf("AddObj count: got %d, want 1", got)
	}
}

// (4) Non-stackable overflow loops one-per-call.
func TestInvAdd_NonStackableOverflow_LoopsOnePerCall(t *testing.T) {
	s, w, inv := newInvAddOverflowState(t)
	// Fill 25 of 28 slots so free=3; non-stackable distribution puts 3
	// swords in slots 25-27, then overflow = 6 - 3 = 3.
	for i := range 25 {
		inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
	}
	s.PushInt(testInvMain)
	s.PushInt(testObjSword) // Stackable=false per newTestInvConfigs
	s.PushInt(6)
	if err := handleInvAdd(s); err != nil {
		t.Fatalf("handleInvAdd: %v", err)
	}
	if len(w.addedCalls) != 3 {
		t.Fatalf("non-stack overflow=3: expected 3 AddObj calls, got %d", len(w.addedCalls))
	}
	for i, c := range w.addedCalls {
		if c.count != 1 {
			t.Errorf("AddObj[%d] count: got %d, want 1", i, c.count)
		}
		if c.typeID != testObjSword {
			t.Errorf("AddObj[%d] typeID: got %d, want %d", i, c.typeID, testObjSword)
		}
	}
}

// (5) Overflow drop coords come from player.CoordPacked() / X / Z.
func TestInvAdd_OverflowDropUsesPlayerCoord(t *testing.T) {
	s, w, inv := newInvAddOverflowState(t)
	// Override the default coord to a recognizable level=2, x=2500, z=3000.
	s.Self.(*mockPlayer).coordPacked = coordgrid.PackCoord(2, 2500, 3000)
	s.Self.(*mockPlayer).x = 2500
	s.Self.(*mockPlayer).z = 3000
	for i := range 28 {
		inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
	}
	s.PushInt(testInvMain)
	s.PushInt(testObjCoin)
	s.PushInt(7)
	if err := handleInvAdd(s); err != nil {
		t.Fatalf("handleInvAdd: %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("expected 1 AddObj call, got %d", len(w.addedCalls))
	}
	got := w.addedCalls[0]
	if got.level != 2 || got.x != 2500 || got.z != 3000 {
		t.Errorf("AddObj coord: got level=%d x=%d z=%d, want level=2 x=2500 z=3000", got.level, got.x, got.z)
	}
}

// (6) Overflow drop receiverID is the player's UID.
func TestInvAdd_OverflowDropReceiverIsPlayerUID(t *testing.T) {
	s, w, inv := newInvAddOverflowState(t)
	s.Self.(*mockPlayer).uidValue = 99999
	for i := range 28 {
		inv.Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
	}
	s.PushInt(testInvMain)
	s.PushInt(testObjCoin)
	s.PushInt(3)
	if err := handleInvAdd(s); err != nil {
		t.Fatalf("handleInvAdd: %v", err)
	}
	if len(w.addedCalls) != 1 {
		t.Fatalf("expected 1 AddObj call, got %d", len(w.addedCalls))
	}
	if got := w.addedCalls[0].receiverID; got != 99999 {
		t.Errorf("AddObj receiverID: got %d, want 99999", got)
	}
}

// -- NAI-131 INV_ADD validator tests --

// (T1.1) InvTypeValid: passing an inv id not registered in s.Configs
// triggers checkInvType before any inv lookup, mirroring TS check(inv,
// InvTypeValid). Asserts TS-shaped error literal.
func TestInvAdd_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, lookup, mc, "no InvType with value (1) found")
}

// (T1.2) ObjTypeValid: passing an obj id not registered triggers
// checkObjType. Mirrors TS check(objId, ObjTypeValid).
func TestInvAdd_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.objs, testObjCoin)
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, lookup, mc, "no ObjType with value (995) found")
}

// (T1.3) ObjStackValid: count == 0 (TS: ScriptInputRangeValidator min=1).
func TestInvAdd_ObjStackValid_CountZero(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 0}, lookup, mc, "invalid count (0)")
}

// (T1.4) ObjStackValid: count == -1 (TS rejects below min=1).
func TestInvAdd_ObjStackValid_CountNegative(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, -1}, lookup, mc, "invalid count (-1)")
}

// (T1.5) Protect+TEMP scope rejects unprotected script. The fixture
// invType "main" defaults Scope=TEMP so a Protect=true override here
// triggers the gate.
func TestInvAdd_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true // scope is TEMP from T0 default
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, lookup, mc, "$inv requires protected access: main")
}

// (T1.6) Protect+SHARED scope is the TS escape hatch — no protected
// access required even when Protect=true.
func TestInvAdd_ProtectGate_SharedScopeEscapeHatch(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvBank].Protect = true // bank is SHARED scope per T0
	// No error expected: SHARED scope skips the protect gate.
	runInvOp(t, OpInvAdd, []int{testInvBank, testObjCoin, 1}, lookup, mc)
}

// (T1.7) Dummy-item gate: non-DummyInv inv + ObjType.DummyItem != 0
// rejects with TS-shaped literal.
func TestInvAdd_DummyItemGate_RejectsDummyItemInRegularInv(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.objs[testObjCoin].DummyItem = 1 // make coins a dummy item; main is not a dummy inv
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, 1}, lookup, mc, "dummyitem in non-dummyinv: coins -> main")
}

// -- NAI-131 INV_SETSLOT validator tests --

func TestInvSetSlot_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}, lookup, mc, "no InvType with value (1) found")
}

func TestInvSetSlot_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.objs, testObjCoin)
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}, lookup, mc, "no ObjType with value (995) found")
}

func TestInvSetSlot_ObjStackValid_CountZero(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 0}, lookup, mc, "invalid count (0)")
}

func TestInvSetSlot_ObjStackValid_CountNegative(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, -1}, lookup, mc, "invalid count (-1)")
}

func TestInvSetSlot_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}, lookup, mc, "$inv requires protected access: main")
}

func TestInvSetSlot_ProtectGate_SharedScopeEscapeHatch(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvBank].Protect = true
	runInvOp(t, OpInvSetSlot, []int{testInvBank, 0, testObjCoin, 1}, lookup, mc)
}

func TestInvSetSlot_DummyItemGate_RejectsDummyItemInRegularInv(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.objs[testObjCoin].DummyItem = 1
	runInvOpExpectErrAsPlayer(t, OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}, lookup, mc, "dummyitem in non-dummyinv: coins -> main")
}

// -- NAI-131 INV_DEL / INV_DELSLOT / INV_CLEAR validator tests --

// (T3.A) INV_DEL — full Obj-gate set without dummyitem.
func TestInvDel_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvDel, []int{testInvMain, testObjCoin, 1}, lookup, mc, "no InvType with value (1) found")
}

func TestInvDel_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.objs, testObjCoin)
	runInvOpExpectErrAsPlayer(t, OpInvDel, []int{testInvMain, testObjCoin, 1}, lookup, mc, "no ObjType with value (995) found")
}

func TestInvDel_ObjStackValid_CountZero(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvDel, []int{testInvMain, testObjCoin, 0}, lookup, mc, "invalid count (0)")
}

func TestInvDel_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	runInvOpExpectErrAsPlayer(t, OpInvDel, []int{testInvMain, testObjCoin, 1}, lookup, mc, "$inv requires protected access: main")
}

func TestInvDel_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvDel, []int{testInvMain, testObjCoin, 1}, nil, mc, "INV_DEL: no active player")
}

// (T3.B) INV_DELSLOT — InvTypeValid + protect/scope only.
func TestInvDelSlot_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvDelSlot, []int{testInvMain, 0}, lookup, mc, "no InvType with value (1) found")
}

func TestInvDelSlot_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	runInvOpExpectErrAsPlayer(t, OpInvDelSlot, []int{testInvMain, 0}, lookup, mc, "$inv requires protected access: main")
}

func TestInvDelSlot_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvDelSlot, []int{testInvMain, 0}, nil, mc, "INV_DELSLOT: no active player")
}

// (T3.C) INV_CLEAR — InvTypeValid + protect/scope only.
func TestInvClear_InvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvClear, []int{testInvMain}, lookup, mc, "no InvType with value (1) found")
}

func TestInvClear_ProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	runInvOpExpectErrAsPlayer(t, OpInvClear, []int{testInvMain}, lookup, mc, "$inv requires protected access: main")
}

func TestInvClear_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvClear, []int{testInvMain}, nil, mc, "INV_CLEAR: no active player")
}

// -- NAI-131 INV_MOVEITEM / INV_MOVEFROMSLOT validator tests --

// (T4.A) INV_MOVEITEM — full Obj-gate set + 2 inv gates.
func TestInvMoveItem_FromInvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "no InvType with value (1) found")
}

func TestInvMoveItem_ToInvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvBank)
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "no InvType with value (2) found")
}

func TestInvMoveItem_ObjTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.objs, testObjCoin)
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "no ObjType with value (995) found")
}

func TestInvMoveItem_ObjStackValid_CountZero(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 0}, lookup, mc, "invalid count (0)")
}

// Protect gate fires on the FROM inv (TS InvOps.ts:507-509).
func TestInvMoveItem_FromProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true // from=main, scope=TEMP
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "$inv requires protected access: main")
}

// Protect gate fires on the TO inv (TS InvOps.ts:511-513) BUT TS
// preserves the asymmetry: the SHARED-escape check uses fromInv's
// scope, not toInv's. So when from=TEMP and to=anything, the TO gate
// still fires regardless of toInv's scope. DEVIATION-NAI-131-D1.
func TestInvMoveItem_ToProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// from=main (TEMP, Protect=false default), to=bank (SHARED, Protect=true override)
	mc.invs[testInvBank].Protect = true
	// from is TEMP (not SHARED) — no TS escape hatch — both gates check fromInv.scope=TEMP.
	// FROM gate's Protect=false skips it; TO gate fires.
	runInvOpExpectErrAsPlayer(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "$inv requires protected access: bank")
}

// TS asymmetry pin (DEVIATION-NAI-131-D1): when fromInv.scope == SHARED,
// BOTH gates' escape hatches fire because both check fromInv.scope.
// toInv's own scope is irrelevant.
func TestInvMoveItem_TSAsymmetry_FromSharedSkipsBothGates(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	// Swap roles: bank is "from" (SHARED), main is "to" (TEMP).
	mc.invs[testInvMain].Protect = true // would normally trigger TO gate
	// But fromInv (bank) is SHARED → TS asymmetry skips both gates.
	runInvOp(t, OpInvMoveItem, []int{testInvBank, testInvMain, testObjCoin, 1}, lookup, mc)
}

func TestInvMoveItem_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}, nil, mc, "INV_MOVEITEM: no active player")
}

// (T4.B) INV_MOVEFROMSLOT — 2 inv gates + 2 protect gates only.
func TestInvMoveFromSlot_FromInvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvMain)
	runInvOpExpectErrAsPlayer(t, OpInvMoveFromSlot, []int{testInvMain, testInvBank, 0}, lookup, mc, "no InvType with value (1) found")
}

func TestInvMoveFromSlot_ToInvTypeValid_UnregisteredID(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	delete(mc.invs, testInvBank)
	runInvOpExpectErrAsPlayer(t, OpInvMoveFromSlot, []int{testInvMain, testInvBank, 0}, lookup, mc, "no InvType with value (2) found")
}

func TestInvMoveFromSlot_FromProtectGate_RejectsUnprotected(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	// Pre-fill source slot so the test reaches the gate before the empty-slot error.
	lookup.invs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 1}
	runInvOpExpectErrAsPlayer(t, OpInvMoveFromSlot, []int{testInvMain, testInvBank, 0}, lookup, mc, "$inv requires protected access: main")
}

func TestInvMoveFromSlot_NoActivePlayerErrors(t *testing.T) {
	mc := newTestInvConfigs()
	runInvOpExpectErr(t, OpInvMoveFromSlot, []int{testInvMain, testInvBank, 0}, nil, mc, "INV_MOVEFROMSLOT: no active player")
}

// (T5) checkObjStack upper-bound: count > Inventory.StackLimit
// (0x7fffffff) is rejected. TS-fidelity per ScriptValidators.ts:121.
// PushInt applies signed-int32 normalisation (toInt32 parity), so
// 0x80000000 wraps to -2147483648 on the stack and fails the lower
// bound check (c < 1) rather than the upper bound — same rejection
// outcome, same error site, post-cast number in the message. TS hits
// this path identically (Numbers.ts:7 `num | 0`).
func TestInvAdd_ObjStackValid_CountAboveStackLimit(t *testing.T) {
	lookup := newTestInvLookup()
	mc := newTestInvConfigs()
	overLimit := int(inventory.StackLimit) + 1 // 0x80000000 = 2147483648
	postCast := int(int32(overLimit))          // -2147483648 (toInt32 parity)
	runInvOpExpectErrAsPlayer(t, OpInvAdd, []int{testInvMain, testObjCoin, overLimit}, lookup, mc, fmt.Sprintf("invalid count (%d)", postCast))
}

func TestInvChangeSlot_NoActivePlayer(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, testObjArr, 1}, lookup, mc, "INV_CHANGESLOT: no active player")
}

func TestInvChangeSlot_InvTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvChangeSlot, []int{9999, testObjCoin, testObjArr, 1}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvChangeSlot_ProtectedRejectsUnprotected(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	mc.invs[testInvMain].Scope = objtype.InvTypeScopePerm
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, testObjArr, 1}, lookup, mc, "INV_CHANGESLOT: $inv requires protected access")
}

func TestInvChangeSlot_FindObjTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvChangeSlot, []int{testInvMain, 9999, testObjArr, 1}, lookup, mc, "no ObjType with value (9999) found")
}

func TestInvChangeSlot_ReplaceObjTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, 9999, 1}, lookup, mc, "no ObjType with value (9999) found")
}

func TestInvChangeSlot_HitOnFirstMatch(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	inv := lookup.Get(nil, testInvMain) // pre-populate
	inv.Set(0, &inventory.Item{Id: testObjCoin, Count: 100})
	inv.Set(1, &inventory.Item{Id: testObjCoin, Count: 50})
	runInvOp(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, testObjArr, 7}, lookup, mc)
	// Slot 0 replaced; slot 1 untouched (early return on first hit).
	if got := inv.Get(0); got == nil || got.Id != testObjArr || got.Count != 7 {
		t.Errorf("slot 0: got %+v, want {Id=%d, Count=7}", got, testObjArr)
	}
	if got := inv.Get(1); got == nil || got.Id != testObjCoin || got.Count != 50 {
		t.Errorf("slot 1 should be unchanged: got %+v", got)
	}
}

func TestInvChangeSlot_NoMatchSilentNoOp(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	inv := lookup.Get(nil, testInvMain)
	inv.Set(0, &inventory.Item{Id: testObjCoin, Count: 100})
	runInvOp(t, OpInvChangeSlot, []int{testInvMain, testObjArr, testObjSword, 1}, lookup, mc)
	if got := inv.Get(0); got == nil || got.Id != testObjCoin || got.Count != 100 {
		t.Errorf("slot 0 should be unchanged: got %+v", got)
	}
}

func TestInvChangeSlot_ReplaceCountZeroAbsencePin(t *testing.T) {
	// Absence-pin: TS does NOT validate replaceCount via ObjStackValid (no `check(count, ObjStackValid)` at InvOps.ts:86-113).
	// replaceCount=0 must be accepted (pop-without-validate); inv.Set writes the zero-count item.
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	inv := lookup.Get(nil, testInvMain)
	inv.Set(0, &inventory.Item{Id: testObjCoin, Count: 100})
	runInvOp(t, OpInvChangeSlot, []int{testInvMain, testObjCoin, testObjArr, 0}, lookup, mc)
	if got := inv.Get(0); got == nil || got.Id != testObjArr || got.Count != 0 {
		t.Errorf("slot 0: got %+v, want {Id=%d, Count=0}", got, testObjArr)
	}
}

// --- INV_MOVEITEM_UNCERT ---

func TestInvMoveItemUncert_NoActivePlayer(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "INV_MOVEITEM_UNCERT: no active player")
}

func TestInvMoveItemUncert_FromInvTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemUncert, []int{9999, testInvBank, testObjCoin, 1}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveItemUncert_ToInvTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemUncert, []int{testInvMain, 9999, testObjCoin, 1}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveItemUncert_ObjTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, 9999, 1}, lookup, mc, "no ObjType with value (9999) found")
}

func TestInvMoveItemUncert_ObjStackInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCoin, 0}, lookup, mc, "INV_MOVEITEM_UNCERT: invalid count (0)")
}

func TestInvMoveItemUncert_NonCertObjMovesAsIs(t *testing.T) {
	// Non-cert obj (CertTemplate=-1 default, CertLink=-1 default) → invAdd uses obj.Id unchanged.
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 50})
	runInvOp(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCoin, 50}, lookup, mc)
	if got := from.Get(0); got != nil {
		t.Errorf("from slot 0 should be empty: got %+v", got)
	}
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjCoin); got != 50 {
		t.Errorf("to inv: got %d coins, want 50", got)
	}
}

func TestInvMoveItemUncert_CertObjUncertifies(t *testing.T) {
	// Certificate item (CertTemplate=self>=0, CertLink=testObjCoin>=0) →
	// gate fires → invAdd uses CertLink (the real underlying obj).
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCertNote, Count: 5})
	runInvOp(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCertNote, 5}, lookup, mc)
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjCertNote); got != 0 {
		t.Errorf("to inv should not contain cert note: got %d", got)
	}
	// CertLink = testObjCoin → 5 coins added.
	if got := to.GetItemCount(testObjCoin); got != 5 {
		t.Errorf("to inv: got %d coins via cert→link, want 5", got)
	}
}

func TestInvMoveItemUncert_RemoveZeroCompletesNoOp(t *testing.T) {
	// from inv empty → tx.Completed=0 → return without invAdd.
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOp(t, OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCoin, 50}, lookup, mc)
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjCoin); got != 0 {
		t.Errorf("to inv: got %d coins, want 0 (Remove returned 0)", got)
	}
}

// -- INV_MOVEITEM_CERT tests --

func TestInvMoveItemCert_NoActivePlayer(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 1}, lookup, mc, "INV_MOVEITEM_CERT: no active player")
}

func TestInvMoveItemCert_FromInvTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemCert, []int{9999, testInvBank, testObjCoin, 1}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvMoveItemCert_ObjStackInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 0}, lookup, mc, "INV_MOVEITEM_CERT: invalid count (0)")
}

func TestInvMoveItemCert_NonCertObjMovesAsIs(t *testing.T) {
	// Non-cert obj (CertTemplate=-1 default, CertLink=-1 default) → finalObj=obj.
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 50})
	runInvOp(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 50}, lookup, mc)
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjCoin); got != 50 {
		t.Errorf("to inv: got %d coins, want 50", got)
	}
}

func TestInvMoveItemCert_CertableObjCertifies(t *testing.T) {
	// Certifiable obj (CertTemplate==-1 && CertLink>=0) → finalObj=CertLink.
	// testObjLogs has CertTemplate=-1, CertLink=testObjCertNote → finalObj=testObjCertNote.
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjLogs, Count: 5})
	runInvOp(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjLogs, 5}, lookup, mc)
	to := lookup.Get(nil, testInvBank)
	if got := to.GetItemCount(testObjLogs); got != 0 {
		t.Errorf("to inv should NOT contain raw logs: got %d", got)
	}
	if got := to.GetItemCount(testObjCertNote); got != 5 {
		t.Errorf("to inv: got %d cert notes via CertLink, want 5", got)
	}
}

func TestInvMoveItemCert_OverflowDropsToWorld(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 10})
	// Cap bank to 1 slot, fill with non-stackable arrow → coin overflow.
	to := lookup.Get(nil, testInvBank)
	to.Capacity = 1
	to.Items = make([]*inventory.Item, 1)
	to.Set(0, &inventory.Item{Id: testObjArr, Count: 1})

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	runInvOpWithWorld(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 10}, lookup, mc, world)
	if len(world.addedCalls) != 1 {
		t.Fatalf("want 1 World.AddObj call (single stacked overflow), got %d", len(world.addedCalls))
	}
	got := world.addedCalls[0]
	if got.typeID != testObjCoin || got.count != 10 || got.duration != 200 {
		t.Errorf("AddObj args: got typeID=%d count=%d duration=%d, want typeID=%d count=10 duration=200",
			got.typeID, got.count, got.duration, testObjCoin)
	}
}

func TestInvMoveItemCert_RemoveZeroCompletesNoOp(t *testing.T) {
	// from inv empty → tx.Completed=0 → return without invAdd or world drop.
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	runInvOpWithWorld(t, OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 50}, lookup, mc, world)
	if len(world.addedCalls) != 0 {
		t.Errorf("want 0 AddObj calls when from-Remove completes 0; got %d", len(world.addedCalls))
	}
}

// -- NAI-132 T6 INV_DROPITEM tests --

func TestInvDropItem_NoActivePlayer(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErr(t, OpInvDropItem, []int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100}, lookup, mc, "INV_DROPITEM: no active player")
}

func TestInvDropItem_InvTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{9999, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100}, lookup, mc, "no InvType with value (9999) found")
}

func TestInvDropItem_CoordInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{testInvMain, -1, testObjCoin, 1, 100}, lookup, mc, "INV_DROPITEM: coord out of range (-1)")
}

func TestInvDropItem_ObjTypeInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), 9999, 1, 100}, lookup, mc, "no ObjType with value (9999) found")
}

func TestInvDropItem_ObjStackInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 0, 100}, lookup, mc, "INV_DROPITEM: invalid count (0)")
}

func TestInvDropItem_DurationInvalid(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	runInvOpExpectErrAsPlayer(t, OpInvDropItem, []int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 0}, lookup, mc, "INV_DROPITEM: duration out of range")
}

func TestInvDropItem_StackableSpawnsSingleStackedObj(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 100})
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 100, 200}, lookup, mc, world)
	if len(world.addedCalls) != 1 {
		t.Fatalf("stackable: want 1 AddObj call, got %d", len(world.addedCalls))
	}
	if world.addedCalls[0].count != 100 {
		t.Errorf("AddObj count: got %d, want 100 (single stacked)", world.addedCalls[0].count)
	}
}

func TestInvDropItem_NonStackableSpawnsSingleStacked(t *testing.T) {
	// TS InvOps.ts:181-184 spawns ONE floor Obj with the full completed
	// count regardless of stackability — there is no per-item branch.
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjSword, Count: 1})
	from.Set(1, &inventory.Item{Id: testObjSword, Count: 1})
	from.Set(2, &inventory.Item{Id: testObjSword, Count: 1})
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjSword, 3, 200}, lookup, mc, world)
	if len(world.addedCalls) != 1 {
		t.Fatalf("non-stackable count=3 (TS-faithful single-stacked): want 1 AddObj call, got %d", len(world.addedCalls))
	}
	if world.addedCalls[0].count != 3 {
		t.Errorf("AddObj count: got %d, want 3 (single stacked)", world.addedCalls[0].count)
	}
}

func TestInvDropItem_StackableCompletedOneSpawnsSingle(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 1})
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 200}, lookup, mc, world)
	if len(world.addedCalls) != 1 {
		t.Fatalf("want 1 AddObj call, got %d", len(world.addedCalls))
	}
	if world.addedCalls[0].count != 1 {
		t.Errorf("count: got %d, want 1", world.addedCalls[0].count)
	}
}

func TestInvDropItem_RemoveZeroCompletedNoSpawn(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 50, 200}, lookup, mc, world)
	if len(world.addedCalls) != 0 {
		t.Errorf("empty inv: want 0 AddObj, got %d", len(world.addedCalls))
	}
}

func TestInvDropItem_ActiveObjPointerSet(t *testing.T) {
	mc := newTestInvConfigs()
	lookup := newTestInvLookup()
	from := lookup.Get(nil, testInvMain)
	from.Set(0, &inventory.Item{Id: testObjCoin, Count: 5})
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	st := runInvOpWithWorld(t, OpInvDropItem, []int{testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 5, 200}, lookup, mc, world)
	if st.Pointers&PtrActiveObj == 0 {
		t.Error("PtrActiveObj should be set after successful drop")
	}
	if st.ActiveObj == nil {
		t.Error("ActiveObj should be set")
	}
}

// -- NAI-133 T3: BOTH_MOVEINV tests --

// runBothMoveInv executes OpBothMoveInv with the given intOperand against
// a state pre-bound with Self + Self2. intInputs are pushed in order
// (matching the TS popInts(2) order: from on bottom, to on top).
// slot1Protected: if true, also sets PtrProtectedActivePlayer2 on the
// constructed state. Returns the post-execution state.
func runBothMoveInv(t *testing.T, operand int32, intInputs []int, lookup InvLookup, configs Configs, world WorldVars, self, self2 *mockPlayer, slot1Protected bool) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_BOTH_MOVEINV",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{operand, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, self, true, nil, nil) // slot-0 protected
	state.Self2 = self2
	state.Pointers |= PtrActivePlayer2
	if slot1Protected {
		state.Pointers |= PtrProtectedActivePlayer2
	}
	state.Inv = lookup
	state.Configs = configs
	state.World = world
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("BOTH_MOVEINV: unexpected error: %v", err)
	}
	return state
}

// runBothMoveInvExpectErr is the error-path variant of runBothMoveInv.
// slot0Protected / slot1Protected control which protect flags are set on
// the state. self / self2 may be nil to test missing-pointer paths.
func runBothMoveInvExpectErr(t *testing.T, operand int32, intInputs []int, lookup InvLookup, configs Configs, world WorldVars, self, self2 ActivePlayer, slot0Protected, slot1Protected bool, substr string) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_BOTH_MOVEINV",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{operand, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, self, slot0Protected, nil, nil)
	if self2 != nil {
		state.Self2 = self2
		state.Pointers |= PtrActivePlayer2
	}
	if slot1Protected {
		state.Pointers |= PtrProtectedActivePlayer2
	}
	state.Inv = lookup
	state.Configs = configs
	state.World = world
	for _, v := range intInputs {
		state.PushInt(v)
	}
	err := Execute(state)
	if err == nil {
		t.Fatalf("BOTH_MOVEINV: expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("BOTH_MOVEINV: expected error containing %q, got %q", substr, err.Error())
	}
}

// twoPlayerInvLookup routes Get(player, typeID) to one of two per-player
// inventory maps based on the receiver pointer. Tests use this to give
// Self and Self2 distinct main/bank inventories. Other player addresses
// return nil.
type twoPlayerInvLookup struct {
	self      *mockPlayer
	self2     *mockPlayer
	selfInvs  map[int]*inventory.Inventory
	self2Invs map[int]*inventory.Inventory
}

func (m *twoPlayerInvLookup) Get(p ActivePlayer, typeID int) *inventory.Inventory {
	mp, ok := p.(*mockPlayer)
	if !ok {
		return nil
	}
	switch mp {
	case m.self:
		return m.selfInvs[typeID]
	case m.self2:
		return m.self2Invs[typeID]
	}
	return nil
}

// newTwoPlayerInvFixture builds a fixture where Self and Self2 each have
// their own main + bank inventories. Inventories are seeded as fresh
// (capacity 28 main, 100 bank).
func newTwoPlayerInvFixture() (*twoPlayerInvLookup, *mockPlayer, *mockPlayer) {
	self := &mockPlayer{username: "Self", uidValue: 1, x: 100, z: 100}
	self2 := &mockPlayer{username: "Self2", uidValue: 2, x: 200, z: 200}
	selfMain := inventory.New(testInvMain, 28, inventory.StackNormal)
	selfBank := inventory.New(testInvBank, 100, inventory.StackAlways)
	self2Main := inventory.New(testInvMain, 28, inventory.StackNormal)
	self2Bank := inventory.New(testInvBank, 100, inventory.StackAlways)
	return &twoPlayerInvLookup{
		self:      self,
		self2:     self2,
		selfInvs:  map[int]*inventory.Inventory{testInvMain: selfMain, testInvBank: selfBank},
		self2Invs: map[int]*inventory.Inventory{testInvMain: self2Main, testInvBank: self2Bank},
	}, self, self2
}

// TestBothMoveInv_Primary_DrainsFromSelfToSelf2 — operand=0; populate
// Self's main with {coins x 5, sword x 1}; expect Self2's main to hold
// the items post; Self's main empty.
func TestBothMoveInv_Primary_DrainsFromSelfToSelf2(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 5}
	lookup.selfInvs[testInvMain].Items[1] = &inventory.Item{Id: testObjSword, Count: 1}

	st := runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)
	_ = st

	for slot, it := range lookup.selfInvs[testInvMain].Items {
		if it != nil {
			t.Errorf("Self.main slot %d should be nil, got %+v", slot, it)
		}
	}
	if it := lookup.self2Invs[testInvMain].Get(0); it == nil || it.Id != testObjCoin || it.Count != 5 {
		t.Errorf("Self2.main slot 0: got %+v, want {coins, 5}", it)
	}
	if it := lookup.self2Invs[testInvMain].Get(1); it == nil || it.Id != testObjSword || it.Count != 1 {
		t.Errorf("Self2.main slot 1: got %+v, want {sword, 1}", it)
	}
	if len(world.addedCalls) != 0 {
		t.Errorf("expected zero AddObj calls, got %d: %+v", len(world.addedCalls), world.addedCalls)
	}
}

// TestBothMoveInv_Secondary_DrainsFromSelf2ToSelf — operand=1; Self2's
// bank → Self's bank (Scope=Shared, no protect gate).
func TestBothMoveInv_Secondary_DrainsFromSelf2ToSelf(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	lookup.self2Invs[testInvBank].Items[0] = &inventory.Item{Id: testObjArr, Count: 100}

	st := runBothMoveInv(t, 1, []int{testInvBank, testInvBank}, lookup, mc, world, self, self2, false)
	_ = st

	if it := lookup.self2Invs[testInvBank].Get(0); it != nil {
		t.Errorf("Self2.bank slot 0: got %+v, want nil", it)
	}
	if it := lookup.selfInvs[testInvBank].Get(0); it == nil || it.Id != testObjArr || it.Count != 100 {
		t.Errorf("Self.bank slot 0: got %+v, want {arrow, 100}", it)
	}
}

// TestBothMoveInv_Overflow_StackableDropsSingleStack — toInv full (no free
// slot, no merge target) + stackable from-item count=N → AddObj called once
// at toPlayer's tile with full count.
func TestBothMoveInv_Overflow_StackableDropsSingleStack(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 7}
	for i := range lookup.self2Invs[testInvMain].Items {
		lookup.self2Invs[testInvMain].Items[i] = &inventory.Item{Id: testObjSword, Count: 1}
	}

	st := runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)
	_ = st

	if it := lookup.selfInvs[testInvMain].Get(0); it != nil {
		t.Errorf("Self.main slot 0 should be nil after delete, got %+v", it)
	}
	if len(world.addedCalls) != 1 {
		t.Fatalf("expected 1 AddObj call (stackable overflow), got %d: %+v", len(world.addedCalls), world.addedCalls)
	}
	call := world.addedCalls[0]
	if call.typeID != testObjCoin || call.count != 7 {
		t.Errorf("AddObj: got typeID=%d count=%d, want %d / 7", call.typeID, call.count, testObjCoin)
	}
	if call.x != self2.x || call.z != self2.z {
		t.Errorf("AddObj coords: got (%d, %d), want (%d, %d) (toPlayer=Self2)", call.x, call.z, self2.x, self2.z)
	}
	if call.duration != 200 {
		t.Errorf("AddObj duration: got %d, want 200 (TS InvOps.ts:430)", call.duration)
	}
}

// TestBothMoveInv_Overflow_NonStackableDropsPerUnit — non-stackable count=K,
// toInv full → AddObj called K times count=1.
func TestBothMoveInv_Overflow_NonStackableDropsPerUnit(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjSword, Count: 3}
	for i := range lookup.self2Invs[testInvMain].Items {
		lookup.self2Invs[testInvMain].Items[i] = &inventory.Item{Id: testObjArr, Count: 1}
	}

	st := runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)
	_ = st

	if len(world.addedCalls) != 3 {
		t.Fatalf("expected 3 AddObj calls (non-stackable per-unit), got %d: %+v", len(world.addedCalls), world.addedCalls)
	}
	for i, call := range world.addedCalls {
		if call.typeID != testObjSword || call.count != 1 {
			t.Errorf("call %d: got typeID=%d count=%d, want %d / 1", i, call.typeID, call.count, testObjSword)
		}
		if call.duration != 200 {
			t.Errorf("call %d duration: got %d, want 200", i, call.duration)
		}
	}
}

// TestBothMoveInv_FromProtectGate_FiresWhenSlotUnprotected — primary,
// fromInv.Protect=true + Scope=TEMP, slot-0 unprotected → from-gate fires.
func TestBothMoveInv_FromProtectGate_FiresWhenSlotUnprotected(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true
	mc.invs[testInvMain].Scope = objtype.InvTypeScopeTemp
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	sf := &ScriptFile{
		Name:             "from_gate",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, self, false, nil, nil) // slot-0 NOT protected
	state.Self2 = self2
	state.Pointers |= PtrActivePlayer2
	state.Inv = lookup
	state.Configs = mc
	state.World = world
	state.PushInt(testInvMain)
	state.PushInt(testInvMain)

	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "BOTH_MOVEINV: $from_inv requires protected access: main") {
		t.Errorf("expected from-gate error, got %v", err)
	}
}

// TestBothMoveInv_ToProtectGate_UsesFromInvScope_Fires — TS quirk pin
// (InvOps.ts:397). toInv.Protect=true, fromInv.Scope=TEMP → to-gate FIRES
// because it reads fromInv.Scope. Slot-0 protected, slot-1 unprotected.
func TestBothMoveInv_ToProtectGate_UsesFromInvScope_Fires(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Scope = objtype.InvTypeScopeTemp   // from-scope NOT shared
	mc.invs[testInvBank].Protect = true                     // toInv.Protect=true
	mc.invs[testInvBank].Scope = objtype.InvTypeScopeShared // toInv.Scope IS shared (TS quirk: gate ignores this)
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	runBothMoveInvExpectErr(t, 0, []int{testInvMain, testInvBank}, lookup, mc, world,
		self, self2,
		true,  // slot-0 protected
		false, // slot-1 NOT protected
		"BOTH_MOVEINV: $to_inv requires protected access: bank",
	)
}

// TestBothMoveInv_ToProtectGate_UsesFromInvScope_DoesNotFire — inverse pin:
// fromInv.Scope=Shared → gate does NOT fire even though toInv.Protect=true
// and toInv.Scope=TEMP.
func TestBothMoveInv_ToProtectGate_UsesFromInvScope_DoesNotFire(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Scope = objtype.InvTypeScopeShared // from-scope shared → gate skipped
	mc.invs[testInvBank].Protect = true
	mc.invs[testInvBank].Scope = objtype.InvTypeScopeTemp // toInv.Scope NOT shared (TS quirk: ignored)
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	st := runBothMoveInv(t, 0, []int{testInvMain, testInvBank}, lookup, mc, world, self, self2, false)
	if st == nil {
		t.Fatal("expected handler to complete without error")
	}
}

// TestBothMoveInv_NoSelf2_Primary_Errors — operand=0 with PtrActivePlayer2
// unset → Self2 nil → runtime null-check fires (TS InvOps.ts:389 parity).
func TestBothMoveInv_NoSelf2_Primary_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, _ := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	sf := &ScriptFile{
		Name:             "no_self2",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, self, true, nil, nil)
	// Self2 deliberately NOT set; PtrActivePlayer2 NOT set.
	state.Inv = lookup
	state.Configs = mc
	state.World = world
	state.PushInt(testInvMain)
	state.PushInt(testInvMain)

	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "player is null") {
		t.Errorf("expected 'player is null' error, got %v", err)
	}
}

// TestBothMoveInv_NoSelf_Secondary_Errors — operand=1, Self nil. Wrapper
// requireActivePlayer fires per TS InvOps.ts:373 checkedHandler(ActivePlayer).
func TestBothMoveInv_NoSelf_Secondary_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, _, self2 := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	sf := &ScriptFile{
		Name:             "no_self_secondary",
		Opcodes:          []Opcode{OpBothMoveInv, OpReturn},
		IntOperands:      []int32{1, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil) // Self nil
	state.Self2 = self2
	state.Pointers |= PtrActivePlayer2
	state.Inv = lookup
	state.Configs = mc
	state.World = world
	state.PushInt(testInvMain)
	state.PushInt(testInvMain)

	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "no active player") {
		t.Errorf("expected 'no active player' error, got %v", err)
	}
}

// TestBothMoveInv_FromInvNil_Errors — InvLookup returns nil for from →
// "inv is null".
func TestBothMoveInv_FromInvNil_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	delete(lookup.selfInvs, testInvMain)
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	runBothMoveInvExpectErr(t, 0, []int{testInvMain, testInvBank}, lookup, mc, world,
		self, self2, true, false,
		"BOTH_MOVEINV: inv is null",
	)
}

// TestBothMoveInv_ToInvNil_Errors — InvLookup returns nil for to.
func TestBothMoveInv_ToInvNil_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	delete(lookup.self2Invs, testInvBank)
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	runBothMoveInvExpectErr(t, 0, []int{testInvMain, testInvBank}, lookup, mc, world,
		self, self2, true, false,
		"BOTH_MOVEINV: inv is null",
	)
}

// TestBothMoveInv_InvalidOperand_Errors — operand=2 → error.
func TestBothMoveInv_InvalidOperand_Errors(t *testing.T) {
	mc := newTestInvConfigs()
	lookup, self, self2 := newTwoPlayerInvFixture()
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	runBothMoveInvExpectErr(t, 2, []int{testInvMain, testInvMain}, lookup, mc, world,
		self, self2, true, false,
		"BOTH_MOVEINV: invalid intOperand 2",
	)
}

// TestBothMoveInv_StakePositive_EmitsWealthEvent — post-NAI-115-D1-retirement
// positive pin: fromInvType.DebugName="dueloffer", from-inv has 1000 coins,
// primary operand (0). Expects exactly one STAKE WealthEvent on the fromPlayer
// with correct EventType, AccountItems, and AccountValue.
// Mirrors TS InvOps.ts:448-456 gate: `if (fromItems.length > 0)`.
func TestBothMoveInv_StakePositive_EmitsWealthEvent(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].DebugName = "dueloffer" // STAKE branch trigger in TS
	mc.invs[testInvMain].Protect = false
	lookup, self, self2 := newTwoPlayerInvFixture()
	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 1000}
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	_ = runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)

	// Production sanity: items moved.
	if it := lookup.self2Invs[testInvMain].Get(0); it == nil || it.Count != 1000 {
		t.Fatalf("transfer must succeed before emission pin is meaningful: got %+v", it)
	}
	// STAKE event pinned (NAI-162 B2.6.fixup, NAI-115-D1 retirement).
	if len(self.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d calls, want 1 (STAKE)", len(self.addWealthEventCalls))
	}
	evt := self.addWealthEventCalls[0]
	if evt.EventType != WealthEventTypeStake {
		t.Errorf("EventType: got %d, want %d (STAKE)", evt.EventType, WealthEventTypeStake)
	}
	if len(evt.AccountItems) != 1 || evt.AccountItems[0].ID != testObjCoin || evt.AccountItems[0].Count != 1000 {
		t.Errorf("AccountItems: got %+v, want [{ID:%d Count:1000}]", evt.AccountItems, testObjCoin)
	}
	// AccountValue = Cost(1) * Count(1000) = 1000.
	if evt.AccountValue != 1000 {
		t.Errorf("AccountValue: got %d, want 1000", evt.AccountValue)
	}
}

// TestBothMoveInv_TradePositive_EmitsWealthEvent — non-secondary, non-dueloffer,
// from-inv has items. Expects exactly one TRADE WealthEvent on the fromPlayer.
// Covers the primary TRADE emission path (TS InvOps.ts:483-492).
func TestBothMoveInv_TradePositive_EmitsWealthEvent(t *testing.T) {
	mc := newTestInvConfigs()
	// testInvMain.DebugName defaults to "main" (not "dueloffer") → TRADE branch.
	mc.invs[testInvMain].Protect = false
	lookup, self, self2 := newTwoPlayerInvFixture()
	lookup.selfInvs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 500}
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	_ = runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)

	if len(self.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d calls, want 1 (TRADE)", len(self.addWealthEventCalls))
	}
	evt := self.addWealthEventCalls[0]
	if evt.EventType != WealthEventTypeTrade {
		t.Errorf("EventType: got %d, want %d (TRADE)", evt.EventType, WealthEventTypeTrade)
	}
	if len(evt.AccountItems) != 1 || evt.AccountItems[0].ID != testObjCoin || evt.AccountItems[0].Count != 500 {
		t.Errorf("AccountItems: got %+v, want [{ID:%d Count:500}]", evt.AccountItems, testObjCoin)
	}
	// AccountValue = Cost(1) * Count(500) = 500.
	if evt.AccountValue != 500 {
		t.Errorf("AccountValue: got %d, want 500", evt.AccountValue)
	}
}

// TestBothMoveInv_TradePositive_ToInvHasItems_EmitsWealthEvent — regression for
// NAI-162 B2.6.fixup Issue 1: fromInv empty, toInv (self2's matching inv) has
// items. TS InvOps.ts:483 gates on `fromItems.length > 0 || toLogs.size > 0`;
// prior goscape code silently skipped emission when fromLogs was empty even if
// toInv had items.
func TestBothMoveInv_TradePositive_ToInvHasItems_EmitsWealthEvent(t *testing.T) {
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = false
	lookup, self, self2 := newTwoPlayerInvFixture()
	// fromInv (Self.main) is empty — nothing to drain.
	// toPlayer's matching from-type inv (Self2.main) has items → toLogs.size > 0.
	lookup.self2Invs[testInvMain].Items[0] = &inventory.Item{Id: testObjCoin, Count: 250}
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	_ = runBothMoveInv(t, 0, []int{testInvMain, testInvMain}, lookup, mc, world, self, self2, false)

	// fromInv was empty so nothing moved; sanity check.
	_ = self2
	// TRADE event must still fire because toLogs.size > 0.
	if len(self.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d calls, want 1 (TRADE via toInv non-empty)", len(self.addWealthEventCalls))
	}
	evt := self.addWealthEventCalls[0]
	if evt.EventType != WealthEventTypeTrade {
		t.Errorf("EventType: got %d, want %d (TRADE)", evt.EventType, WealthEventTypeTrade)
	}
	// AccountItems comes from fromLogs which is empty.
	if len(evt.AccountItems) != 0 {
		t.Errorf("AccountItems: got %d items, want 0 (fromInv was empty)", len(evt.AccountItems))
	}
}

// -- NAI-134 INV_DROPITEM_DELAYED tests --

// makeDropItemDelayedState builds a direct-call test fixture matching
// the existing newInvAddOverflowState pattern (handlers_inv_test.go:1038).
// Sets up: configs with one InvType (Protect/Scope per args) and one
// stackable ObjType, an inventory pre-loaded with `count` of the obj, an
// mockPlayer at (0,3200,3200) with uid=12345, PtrActivePlayer set, and
// a fakeWorldAddObj recorder wired into state.World.
//
// Returns (state, inv, world). Caller pushes int args in TS pop-order
// before calling handleInvDropItemDelayed directly.
func makeDropItemDelayedState(t *testing.T, protect bool, scope int, count int) (*ScriptState, *inventory.Inventory, *fakeWorldAddObj) {
	t.Helper()
	s := newTestState(minimalScript(OpReturn))

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(testInvMain)
	invType.DebugName = "test_inv"
	invType.Size = 28
	invType.Protect = protect
	invType.Scope = scope
	mc.invs[testInvMain] = invType
	s.Configs = mc

	inv := inventory.New(testInvMain, 28, inventory.StackNormal)
	if count > 0 {
		inv.Items[0] = &inventory.Item{Id: testObjCoin, Count: count}
	}
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{testInvMain: inv}}

	s.Self = &mockPlayer{
		uidValue:    12345,
		coordPacked: coordgrid.PackCoord(0, 3200, 3200),
		x:           3200,
		z:           3200,
	}
	s.Pointers |= PtrActivePlayer

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s.World = world
	return s, inv, world
}

// pushDropItemDelayedArgs pushes the 6 args in TS pop-order
// (invID at bottom, delay on top): handler PopInts in reverse.
func pushDropItemDelayedArgs(s *ScriptState, invID, coord, obj, count, duration, delay int) {
	s.PushInt(invID)
	s.PushInt(coord)
	s.PushInt(obj)
	s.PushInt(count)
	s.PushInt(duration)
	s.PushInt(delay)
}

// TestInvDropItemDelayed_NoActivePlayer_Errors pins the requireActivePlayer
// guard. Without PtrActivePlayer set, handler returns an error.
func TestInvDropItemDelayed_NoActivePlayer_Errors(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)
	s.Pointers &^= PtrActivePlayer

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 5)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no active player") {
		t.Errorf("err: got %q, want substring \"no active player\"", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 0 {
		t.Errorf("error path: expected 0 enqueue calls, got %d", got)
	}
}

// TestInvDropItemDelayed_HappyPath_EnqueueArgs pins the success path:
// validators pass, protect-gate skipped (Protect=false), inv.Remove
// succeeds with completed=count, EnqueueObjDelayed receives every arg
// verbatim including delay.
func TestInvDropItemDelayed_HappyPath_EnqueueArgs(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)
	const wantDelay = 7
	const wantDuration = 100

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 3, wantDuration, wantDelay)
	err := handleInvDropItemDelayed(s)

	if err != nil {
		t.Fatalf("happy path: unexpected error: %v", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 1 {
		t.Fatalf("expected 1 enqueue call, got %d", got)
	}
	c := world.enqueueObjDelayedCalls[0]
	if c.level != 0 || c.x != 3200 || c.z != 3200 {
		t.Errorf("enqueue coord: got level=%d x=%d z=%d, want 0/3200/3200", c.level, c.x, c.z)
	}
	if c.typeID != testObjCoin {
		t.Errorf("enqueue typeID: got %d, want %d", c.typeID, testObjCoin)
	}
	if c.count != 3 {
		t.Errorf("enqueue count: got %d, want 3 (TS uses Remove.completed)", c.count)
	}
	if c.duration != wantDuration {
		t.Errorf("enqueue duration: got %d, want %d", c.duration, wantDuration)
	}
	if c.delay != wantDelay {
		t.Errorf("enqueue delay: got %d, want %d", c.delay, wantDelay)
	}
	if c.receiverID != s.Self.UID() {
		t.Errorf("enqueue receiverID: got %d, want %d (Self.UID)", c.receiverID, s.Self.UID())
	}
	// TS-asymmetry vs INV_DROPITEM: ActiveObj NOT set (obj does not yet exist).
	if s.ActiveObj != nil {
		t.Errorf("DoesNotSetActiveObj: state.ActiveObj got %v, want nil", s.ActiveObj)
	}
	if s.Pointers&PtrActiveObj != 0 {
		t.Errorf("DoesNotSetActiveObj: PtrActiveObj should not be set, pointers=%b", s.Pointers)
	}
}

// TestInvDropItemDelayed_RemoveCompletedZero_NoEnqueue pins TS
// InvOps.ts:203-205: when inv.Remove returns completed=0 (empty inv),
// handler returns nil and does NOT enqueue.
func TestInvDropItemDelayed_RemoveCompletedZero_NoEnqueue(t *testing.T) {
	// Empty inv (count=0 → Items[0] not seeded → Remove returns completed=0).
	s, _, world := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 0)

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err != nil {
		t.Fatalf("completed=0 path: unexpected error: %v", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 0 {
		t.Errorf("completed=0: expected 0 enqueue calls, got %d", got)
	}
}

// TestInvDropItemDelayed_BadInv_Errors pins InvTypeValid: invID=-1 fails.
func TestInvDropItemDelayed_BadInv_Errors(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, -1, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for invID=-1")
	}
	if !strings.Contains(err.Error(), "INV_DROPITEM_DELAYED") {
		t.Errorf("err: got %q, want INV_DROPITEM_DELAYED prefix", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 0 {
		t.Errorf("error path: expected 0 enqueue, got %d", got)
	}
}

// TestInvDropItemDelayed_BadCoord_Errors pins CoordValid: coord=-1 fails.
func TestInvDropItemDelayed_BadCoord_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, testInvMain, -1, testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for coord=-1")
	}
	if !strings.Contains(err.Error(), "coord") {
		t.Errorf("err: got %q, want substring \"coord\"", err)
	}
}

// TestInvDropItemDelayed_BadObj_Errors pins ObjTypeValid: obj=-1 fails.
func TestInvDropItemDelayed_BadObj_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), -1, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for obj=-1")
	}
}

// TestInvDropItemDelayed_BadCount_Errors pins ObjStackValid: count=0 fails.
func TestInvDropItemDelayed_BadCount_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 0, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for count=0")
	}
}

// TestInvDropItemDelayed_BadDuration_Errors pins DurationValid:
// duration=0 fails (DurationValid rejects <=0).
func TestInvDropItemDelayed_BadDuration_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 0, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected error for duration=0")
	}
}

// TestInvDropItemDelayed_NilWorld_DefensiveError pins
// DEVIATION-NAI-130-D2 sibling: nil World surface returns a clean error
// rather than nil-deref. Only fires AFTER all validators + protect gate
// + Remove succeed.
func TestInvDropItemDelayed_NilWorld_DefensiveError(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)
	s.World = nil

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("nil World: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no world surface") {
		t.Errorf("err: got %q, want substring \"no world surface\"", err)
	}
}

// TestInvDropItemDelayed_ProtectGate_Operand0_Errors pins operand=0 +
// Protect=true + Scope!=Shared + no PtrProtectedActivePlayer → error.
func TestInvDropItemDelayed_ProtectGate_Operand0_Errors(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, true, objtype.InvTypeScopeTemp, 5)
	// Pointers does NOT include PtrProtectedActivePlayer.

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("expected protect-gate error")
	}
	if !strings.Contains(err.Error(), "protected access") {
		t.Errorf("err: got %q, want substring \"protected access\"", err)
	}
	if !strings.Contains(err.Error(), "test_inv") {
		t.Errorf("err: got %q, want substring \"test_inv\" (debugname)", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 0 {
		t.Errorf("protect-error path: expected 0 enqueue, got %d", got)
	}
}

// TestInvDropItemDelayed_ProtectGate_Operand0_PassesWithFlag pins
// operand=0 + Protect=true + PtrProtectedActivePlayer set → success.
func TestInvDropItemDelayed_ProtectGate_Operand0_PassesWithFlag(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, true, objtype.InvTypeScopeTemp, 5)
	s.Pointers |= PtrProtectedActivePlayer

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err != nil {
		t.Fatalf("protect-flag set: unexpected error: %v", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 1 {
		t.Errorf("protect-flag set: expected 1 enqueue, got %d", got)
	}
}

// TestInvDropItemDelayed_ProtectGate_Operand1_RequiresPtr2 pins NAI-133
// slot routing: operand=1 must check PtrProtectedActivePlayer2, not
// PtrProtectedActivePlayer.
//
// Sub-case A: only PtrProtectedActivePlayer set (not …2) → error.
// Sub-case B: PtrProtectedActivePlayer2 set → success.
func TestInvDropItemDelayed_ProtectGate_Operand1_RequiresPtr2(t *testing.T) {
	t.Run("PtrProtectedActivePlayer_only_errors", func(t *testing.T) {
		s, _, _ := makeDropItemDelayedState(t, true, objtype.InvTypeScopeTemp, 5)
		s.Script.IntOperands[s.PC] = 1
		s.Pointers |= PtrProtectedActivePlayer // wrong slot

		pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
		err := handleInvDropItemDelayed(s)

		if err == nil {
			t.Fatalf("operand=1 with only PtrProtectedActivePlayer (not …2): expected error")
		}
		if !strings.Contains(err.Error(), "protected access") {
			t.Errorf("err: got %q, want substring \"protected access\"", err)
		}
	})

	t.Run("PtrProtectedActivePlayer2_passes", func(t *testing.T) {
		s, _, world := makeDropItemDelayedState(t, true, objtype.InvTypeScopeTemp, 5)
		s.Script.IntOperands[s.PC] = 1
		s.Pointers |= PtrProtectedActivePlayer2
		// operand=1 drops as the SECONDARY player (TS INV_DROPITEM_DELAYED
		// uses state.activePlayer); bind Self2 so the operand-aware
		// s.activePlayer() resolves to a real player.
		s.Self2 = &mockPlayer{}
		s.Pointers |= PtrActivePlayer2

		pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
		err := handleInvDropItemDelayed(s)

		if err != nil {
			t.Fatalf("operand=1 with PtrProtectedActivePlayer2: unexpected error: %v", err)
		}
		if got := len(world.enqueueObjDelayedCalls); got != 1 {
			t.Errorf("expected 1 enqueue, got %d", got)
		}
	})
}

// TestInvDropItemDelayed_BadOperand_Errors pins operand=2 → "invalid
// intOperand". Mirrors handleBothMoveInv at handlers_inv.go:1230-1233.
func TestInvDropItemDelayed_BadOperand_Errors(t *testing.T) {
	s, _, _ := makeDropItemDelayedState(t, false, objtype.InvTypeScopeTemp, 5)
	s.Script.IntOperands[s.PC] = 2

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err == nil {
		t.Fatalf("operand=2: expected error")
	}
	if !strings.Contains(err.Error(), "invalid intOperand") {
		t.Errorf("err: got %q, want substring \"invalid intOperand\"", err)
	}
}

// TestInvDropItemDelayed_SharedScopeBypassesProtect pins TS InvOps.ts:197:
// when invType.Scope == InvTypeScopeShared, the protect gate is skipped
// even if invType.Protect=true and no PtrProtectedActivePlayer flag.
func TestInvDropItemDelayed_SharedScopeBypassesProtect(t *testing.T) {
	s, _, world := makeDropItemDelayedState(t, true, objtype.InvTypeScopeShared, 5)
	// Pointers does NOT include PtrProtectedActivePlayer.

	pushDropItemDelayedArgs(s, testInvMain, coordgrid.PackCoord(0, 3200, 3200), testObjCoin, 1, 100, 0)
	err := handleInvDropItemDelayed(s)

	if err != nil {
		t.Fatalf("Scope=Shared: unexpected error: %v", err)
	}
	if got := len(world.enqueueObjDelayedCalls); got != 1 {
		t.Errorf("Scope=Shared: expected 1 enqueue, got %d", got)
	}
}

// -- INV_STOCKBASE --

// TestHandleInvStockBase_NilStockObjReturnsMinusOne pins TS
// InvOps.ts:46-49 — `if (!invType.stockobj || !invType.stockcount) return -1`.
// Pop order: obj popped first (top of stack), inv popped second.
// Push order: inv pushed first (deeper), obj pushed second (top).
func TestHandleInvStockBase_NilStockObjReturnsMinusOne(t *testing.T) {
	invType := objtype.NewInvType(testInvMain)
	invType.DebugName = "main"
	obj := objtype.NewObjType(testObjCoin)
	mc := &mockConfigs{
		invs: map[int]*objtype.InvType{testInvMain: invType},
		objs: map[int]*objtype.ObjType{testObjCoin: obj},
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     mc,
	}
	s.PushInt(testInvMain) // deeper: inv popped second
	s.PushInt(testObjCoin) // top: obj popped first

	if err := handleInvStockBase(s); err != nil {
		t.Fatalf("handleInvStockBase: %v", err)
	}
	if got := s.IntStack[s.ISP-1]; got != -1 {
		t.Errorf("top: got %d, want -1 (nil StockObj)", got)
	}
}

// TestHandleInvStockBase_ObjNotInListReturnsMinusOne pins TS InvOps.ts:51-52.
// Pop order: obj popped first (top of stack), inv popped second.
func TestHandleInvStockBase_ObjNotInListReturnsMinusOne(t *testing.T) {
	invType := objtype.NewInvType(testInvMain)
	invType.DebugName = "main"
	invType.StockObj = []uint16{1, 2, 3}
	invType.StockCount = []uint16{10, 20, 30}
	obj := objtype.NewObjType(99) // not in stock
	mc := &mockConfigs{
		invs: map[int]*objtype.InvType{testInvMain: invType},
		objs: map[int]*objtype.ObjType{99: obj},
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     mc,
	}
	s.PushInt(testInvMain) // deeper: inv popped second
	s.PushInt(99)          // top: obj popped first

	if err := handleInvStockBase(s); err != nil {
		t.Fatalf("handleInvStockBase: %v", err)
	}
	if got := s.IntStack[s.ISP-1]; got != -1 {
		t.Errorf("top: got %d, want -1 (obj not in list)", got)
	}
}

// TestHandleInvStockBase_ObjInListReturnsCount pins TS InvOps.ts:52
// — push stockcount[index].
// Pop order: obj popped first (top of stack), inv popped second.
func TestHandleInvStockBase_ObjInListReturnsCount(t *testing.T) {
	invType := objtype.NewInvType(testInvMain)
	invType.DebugName = "main"
	invType.StockObj = []uint16{10, 20, 30}
	invType.StockCount = []uint16{100, 200, 300}
	obj := objtype.NewObjType(20) // index=1
	mc := &mockConfigs{
		invs: map[int]*objtype.InvType{testInvMain: invType},
		objs: map[int]*objtype.ObjType{20: obj},
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     mc,
	}
	s.PushInt(testInvMain) // deeper: inv popped second
	s.PushInt(20)          // top: obj popped first

	if err := handleInvStockBase(s); err != nil {
		t.Fatalf("handleInvStockBase: %v", err)
	}
	if got := s.IntStack[s.ISP-1]; got != 200 {
		t.Errorf("top: got %d, want 200 (stockcount[1])", got)
	}
}

// TestHandleInvDebugName_PushesName pins TS InvOps.ts:34-38 happy path.
func TestHandleInvDebugName_PushesName(t *testing.T) {
	invType := objtype.NewInvType(testInvMain)
	invType.DebugName = "main"
	mc := &mockConfigs{
		invs: map[int]*objtype.InvType{testInvMain: invType},
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     mc,
	}
	s.PushInt(testInvMain)
	if err := handleInvDebugName(s); err != nil {
		t.Fatalf("handleInvDebugName: %v", err)
	}
	if got := s.StringStack[0]; got != "main" {
		t.Errorf("top of string stack: got %q, want %q", got, "main")
	}
}

// TestHandleInvDebugName_EmptyFallsBackToNullLiteral pins TS InvOps.ts:37
// — `invType.debugname ?? 'null'`.
func TestHandleInvDebugName_EmptyFallsBackToNullLiteral(t *testing.T) {
	invType := objtype.NewInvType(testInvMain)
	invType.DebugName = "" // simulate the TS undefined → ?? 'null' arm
	mc := &mockConfigs{
		invs: map[int]*objtype.InvType{testInvMain: invType},
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     mc,
	}
	s.PushInt(testInvMain)
	if err := handleInvDebugName(s); err != nil {
		t.Fatalf("handleInvDebugName: %v", err)
	}
	if got := s.StringStack[0]; got != "null" {
		t.Errorf("top of string stack: got %q, want %q", got, "null")
	}
}

// TestInvAllStock pins OpInvAllStock's body: pop typeID, checkInvType,
// push 1 if InvType.AllStock else 0. Mirrors TS InvOps.ts:20-24.
// NAI-160 T5.
func TestInvAllStock(t *testing.T) {
	mp := &mockPlayer{}
	mc := &mockConfigs{invs: map[int]*objtype.InvType{42: {AllStock: true}}}
	sf := &ScriptFile{
		Name:             "[inv_allstock_true,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpInvAllStock, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = mc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 1 {
		t.Errorf("INV_ALLSTOCK(AllStock=true): got %d, want 1", got)
	}
}

// TestInvAllStockFalseDefault pins the AllStock=false path. NAI-160 T5.
func TestInvAllStockFalseDefault(t *testing.T) {
	mp := &mockPlayer{}
	mc := &mockConfigs{invs: map[int]*objtype.InvType{42: {AllStock: false}}}
	sf := &ScriptFile{
		Name:             "[inv_allstock_false,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpInvAllStock, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = mc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("INV_ALLSTOCK(AllStock=false): got %d, want 0", got)
	}
}

// TestInvAllStockInvalidType pins checkInvType rejection. NAI-160 T5.
func TestInvAllStockInvalidType(t *testing.T) {
	mp := &mockPlayer{}
	mc := &mockConfigs{invs: map[int]*objtype.InvType{}}
	sf := &ScriptFile{
		Name:             "[inv_allstock_invalid,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpInvAllStock, OpReturn},
		IntOperands:      []int32{99, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = mc
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want INV_ALLSTOCK <type-error>")
	}
	if got := err.Error(); !strings.Contains(got, "INV_ALLSTOCK") {
		t.Errorf("err: got %q, want 'INV_ALLSTOCK' substring", got)
	}
}

// TestPerformInvAdd_DirectCall pins the contract that performInvAdd
// can be called with already-typed args, bypassing the PopInt-driven
// handleInvAdd wrapper. Mirrors the same validation chain + Inventory.Add
// path that handleInvAdd takes; this test focuses on the direct-call
// happy path so OBJ_TAKEITEM (NAI-153 T4) can rely on the shared impl.
func TestPerformInvAdd_DirectCall(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	s.Self = &mockPlayer{uidValue: 12345}
	s.Pointers |= PtrActivePlayer

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(93)
	invType.Size = 28
	invType.Protect = false // NewInvType defaults Protect=true; clear it so unprotected call succeeds
	mc.invs[93] = invType
	mindrune := objtype.NewObjType(558)
	mindrune.Stackable = false
	mc.objs[558] = mindrune
	s.Configs = mc

	inv := inventory.New(93, 28, inventory.StackNormal)
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{93: inv}}

	if err := performInvAdd(s, 93, 558, 1, "TEST"); err != nil {
		t.Fatalf("performInvAdd returned error: %v", err)
	}

	got := inv.Get(0)
	if got == nil || got.Id != 558 || got.Count != 1 {
		t.Errorf("performInvAdd: inv slot 0 got %+v, want {Id:558 Count:1}", got)
	}
}

// TestHandleInvTotalParamStack pins the pop-order [param, inv] and
// the pushInt of the delegated sum. Mirrors TS InvOps.ts:792-796.
// TS popInts(2) → [inv, param] means param is on top of stack (popped
// first), then inv.
func TestHandleInvTotalParamStack(t *testing.T) {
	mp := &mockPlayer{invTotalParamStackReturn: 42}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        mp,
		Pointers:    PtrActivePlayer,
	}
	// Push order: inv first (deeper), param on top (popped first).
	s.PushInt(5) // inv
	s.PushInt(7) // param (top of stack)

	if err := handleInvTotalParamStack(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mp.invTotalParamStackArgs) != 1 {
		t.Fatalf("delegate: got %d calls, want 1", len(mp.invTotalParamStackArgs))
	}
	got := mp.invTotalParamStackArgs[0]
	if got.InvID != 5 || got.ParamID != 7 {
		t.Errorf("delegate args: got %+v, want {InvID:5, ParamID:7}", got)
	}
	if v := s.PopInt(); v != 42 {
		t.Errorf("pushed: got %d, want 42", v)
	}
}

// TestNAI115D1Retirement_InvDropSlotScopePermEmitsWealthEvent pins the
// behaviour flip: pre-NAI-162, INV_DROPSLOT on a SCOPE_PERM inv skipped
// AddWealthEvent (NAI-115-D1 deviation). Post-NAI-162 B2.6, the path emits
// a DROP event. Mirrors TS InvOps.ts:231-238.
func TestNAI115D1Retirement_InvDropSlotScopePermEmitsWealthEvent(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	mp := &mockPlayer{uidValue: 99}
	s.Self = mp
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer

	mc := newTestInvConfigs()
	// SCOPE_PERM inv
	invType := objtype.NewInvType(7)
	invType.Size = 28
	invType.Protect = true
	invType.Scope = objtype.InvTypeScopePerm
	mc.invs[7] = invType
	// Obj with cost
	sword := objtype.NewObjType(1277)
	sword.Name = "Rune Sword"
	sword.DebugName = "rune_sword"
	sword.Cost = 20000
	mc.objs[1277] = sword
	s.Configs = mc

	inv := inventory.New(7, 28, inventory.StackNormal)
	inv.Set(0, &inventory.Item{Id: 1277, Count: 1})
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{7: inv}}

	s.PushInt(7)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mp.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d, want 1 (retirement)", len(mp.addWealthEventCalls))
	}
	evt := mp.addWealthEventCalls[0]
	if evt.EventType != WealthEventTypeDrop {
		t.Errorf("EventType: got %d, want %d (DROP)", evt.EventType, WealthEventTypeDrop)
	}
	if evt.AccountValue != 20000 {
		t.Errorf("AccountValue: got %d, want 20000 (1*cost)", evt.AccountValue)
	}
	if len(evt.AccountItems) != 1 || evt.AccountItems[0].ID != 1277 {
		t.Errorf("AccountItems: got %+v, want [{ID:1277 ...}]", evt.AccountItems)
	}
}

// TestNAI115D1Retirement_InvDropSlotScopeTempNoWealthEvent pins that
// non-SCOPE_PERM drops do NOT emit wealth events (ammo/TEMP bypass).
func TestNAI115D1Retirement_InvDropSlotScopeTempNoWealthEvent(t *testing.T) {
	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	mp := &mockPlayer{uidValue: 99}
	s.Self = mp
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(9)
	invType.Size = 28
	invType.Protect = true
	invType.Scope = objtype.InvTypeScopeTemp // NOT SCOPE_PERM
	mc.invs[9] = invType
	arrow := objtype.NewObjType(882)
	arrow.Name = "Arrow"
	arrow.DebugName = "arrow_heads"
	arrow.Cost = 3
	mc.objs[882] = arrow
	s.Configs = mc

	inv := inventory.New(9, 28, inventory.StackNormal)
	inv.Set(0, &inventory.Item{Id: 882, Count: 10})
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{9: inv}}

	s.PushInt(9)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mp.addWealthEventCalls) != 0 {
		t.Errorf("AddWealthEvent: got %d, want 0 (TEMP scope = no event)", len(mp.addWealthEventCalls))
	}
}

// --- NAI-162 B3.1 BOTH_DROPSLOT (opcode 4300) tests ---
//
// Helpers shared across B3 tests.

// newBothDropSlotConfigs builds a mockConfigs with two obj types and two inv
// types for BOTH_DROPSLOT / INV_DROPALL tests.
//
//   - obj 10 (untradeable, cost=100, debugname="rune")
//   - obj 20 (tradeable, cost=50,  debugname="gold")
//   - inv 5 (scope=ScopeNormal,  protect=false)
//   - inv 6 (scope=InvTypeScopePerm, protect=true)
func newBothDropSlotConfigs() *mockConfigs {
	mc := &mockConfigs{
		objs:    make(map[int]*objtype.ObjType),
		npcs:    make(map[int]*objtype.NpcType),
		locs:    make(map[int]*objtype.LocType),
		enums:   make(map[int]*objtype.EnumType),
		structs: make(map[int]*objtype.StructType),
		params:  make(map[int]*objtype.ParamType),
		invs:    make(map[int]*objtype.InvType),
	}

	rune := objtype.NewObjType(10)
	rune.DebugName = "rune"
	rune.Cost = 100
	rune.Tradeable = false
	mc.objs[10] = rune

	gold := objtype.NewObjType(20)
	gold.DebugName = "gold"
	gold.Cost = 50
	gold.Tradeable = true
	mc.objs[20] = gold

	normalInv := objtype.NewInvType(5)
	normalInv.DebugName = "normal"
	normalInv.Size = 28
	normalInv.Scope = objtype.InvTypeScopeTemp
	normalInv.Protect = false
	mc.invs[5] = normalInv

	permInv := objtype.NewInvType(6)
	permInv.DebugName = "perm"
	permInv.Size = 28
	permInv.Scope = objtype.InvTypeScopePerm
	permInv.Protect = true
	mc.invs[6] = permInv

	return mc
}

// newBothDropState builds a ScriptState for BOTH_DROPSLOT tests with
// the given intOperand (0=primary, 1=secondary). Both Self and Self2 are
// set; slot-0 protected by default (Init third arg=true). Callers may add
// PtrProtectedActivePlayer2 for secondary protect tests.
func newBothDropState(operand int32, self, self2 *mockPlayer, mc *mockConfigs, lookup InvLookup, world WorldVars) *ScriptState {
	sf := &ScriptFile{
		Name:             "test_BOTH_DROPSLOT",
		Opcodes:          []Opcode{OpBothDropSlot, OpReturn},
		IntOperands:      []int32{operand, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	s := Init(sf, self, true, nil, nil) // slot-0 protected
	s.Self2 = self2
	s.Pointers |= PtrActivePlayer2
	s.Configs = mc
	s.Inv = lookup
	s.World = world
	return s
}

// TestHandleBothDropSlot_PrimaryFromSelf_Untradeable: secondary=0,
// inv.protect=false, untradeable obj → addObj.receiverID = self.UID().
func TestHandleBothDropSlot_PrimaryFromSelf_Untradeable(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.self2Invs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.selfInvs[5].Set(0, &inventory.Item{Id: 10, Count: 3})

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s := newBothDropState(0, self, self2, mc, lookup, world)
	// push: inv, coord, slot, duration (bottom→top; handler pops LIFO: duration, slot, coord, inv)
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleBothDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.addedCalls) != 1 {
		t.Fatalf("addObj: got %d, want 1", len(world.addedCalls))
	}
	// Untradeable: receiver = fromPlayer (self) UID
	if world.addedCalls[0].receiverID != 11 {
		t.Errorf("untradeable receiver: got %d, want 11 (self.UID)", world.addedCalls[0].receiverID)
	}
}

// TestHandleBothDropSlot_PrimaryFromSelf_Tradeable: tradeable obj →
// addObj.receiverID = toPlayer (self2) UID.
func TestHandleBothDropSlot_PrimaryFromSelf_Tradeable(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.self2Invs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.selfInvs[5].Set(0, &inventory.Item{Id: 20, Count: 1}) // tradeable

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s := newBothDropState(0, self, self2, mc, lookup, world)
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleBothDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.addedCalls) != 1 {
		t.Fatalf("addObj: got %d, want 1", len(world.addedCalls))
	}
	// Tradeable: receiver = toPlayer (self2) UID
	if world.addedCalls[0].receiverID != 22 {
		t.Errorf("tradeable receiver: got %d, want 22 (self2.UID)", world.addedCalls[0].receiverID)
	}
}

// TestHandleBothDropSlot_SecondaryFromSelf2_Untradeable: secondary=1 →
// fromPlayer = Self2, toPlayer = Self. Untradeable → receiver = self2.UID.
func TestHandleBothDropSlot_SecondaryFromSelf2_Untradeable(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.self2Invs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.self2Invs[5].Set(0, &inventory.Item{Id: 10, Count: 2}) // self2 holds the item

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s := newBothDropState(1, self, self2, mc, lookup, world) // operand=1 (secondary)
	s.Pointers |= PtrProtectedActivePlayer2
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleBothDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.addedCalls) != 1 {
		t.Fatalf("addObj: got %d, want 1", len(world.addedCalls))
	}
	// untradeable: receiver = fromPlayer (self2) UID
	if world.addedCalls[0].receiverID != 22 {
		t.Errorf("secondary untradeable receiver: got %d, want 22 (self2.UID)", world.addedCalls[0].receiverID)
	}
}

// TestHandleBothDropSlot_SecondaryFromSelf2_Tradeable: secondary=1,
// tradeable → receiver = toPlayer (self) UID.
func TestHandleBothDropSlot_SecondaryFromSelf2_Tradeable(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.self2Invs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.self2Invs[5].Set(0, &inventory.Item{Id: 20, Count: 1}) // tradeable

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s := newBothDropState(1, self, self2, mc, lookup, world)
	s.Pointers |= PtrProtectedActivePlayer2
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleBothDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.addedCalls) != 1 {
		t.Fatalf("addObj: got %d, want 1", len(world.addedCalls))
	}
	// tradeable: receiver = toPlayer (self) UID
	if world.addedCalls[0].receiverID != 11 {
		t.Errorf("secondary tradeable receiver: got %d, want 11 (self.UID)", world.addedCalls[0].receiverID)
	}
}

// TestHandleBothDropSlot_ScopePerm_EmitsPVPWealthEvent: SCOPE_PERM →
// AddWealthEvent(PVP) on state.activePlayer (Self). RecipientSession=""
// per NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY (Session() not on interface).
func TestHandleBothDropSlot_ScopePerm_EmitsPVPWealthEvent(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[6] = inventory.New(6, 28, inventory.StackNormal)
	lookup.self2Invs[6] = inventory.New(6, 28, inventory.StackNormal)
	lookup.selfInvs[6].Set(0, &inventory.Item{Id: 10, Count: 5}) // cost=100 → value=500

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s := newBothDropState(0, self, self2, mc, lookup, world)
	s.PushInt(6) // perm inv
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleBothDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(self.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d calls, want 1 (PVP)", len(self.addWealthEventCalls))
	}
	evt := self.addWealthEventCalls[0]
	if evt.EventType != WealthEventTypePVP {
		t.Errorf("EventType: got %d, want %d (PVP)", evt.EventType, WealthEventTypePVP)
	}
	if evt.AccountValue != 500 {
		t.Errorf("AccountValue: got %d, want 500 (5*100)", evt.AccountValue)
	}
	if len(evt.AccountItems) != 1 || evt.AccountItems[0].ID != 10 || evt.AccountItems[0].Count != 5 {
		t.Errorf("AccountItems: got %+v, want [{ID:10 Count:5}]", evt.AccountItems)
	}
}

// TestHandleBothDropSlot_ScopeNormal_NoWealthEvent: non-SCOPE_PERM →
// no wealth event emitted.
func TestHandleBothDropSlot_ScopeNormal_NoWealthEvent(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.self2Invs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.selfInvs[5].Set(0, &inventory.Item{Id: 10, Count: 1})

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s := newBothDropState(0, self, self2, mc, lookup, world)
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleBothDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(self.addWealthEventCalls) != 0 {
		t.Errorf("AddWealthEvent: got %d calls, want 0 (non-SCOPE_PERM)", len(self.addWealthEventCalls))
	}
}

// TestHandleBothDropSlot_NullToPlayer: Self2 absent → error.
func TestHandleBothDropSlot_NullToPlayer(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	sf := &ScriptFile{
		Name:             "test_BOTH_DROPSLOT",
		Opcodes:          []Opcode{OpBothDropSlot, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	s := Init(sf, self, true, nil, nil)
	// Self2 absent: Pointers has no PtrActivePlayer2
	s.Configs = mc
	s.World = &fakeWorldAddObj{mockWorld: newMockWorld()}
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{}}
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	err := Execute(s)
	if err == nil {
		t.Fatal("expected error for null toPlayer, got nil")
	}
	if !strings.Contains(err.Error(), "player is null") {
		t.Errorf("expected 'player is null', got: %v", err)
	}
}

// TestHandleBothDropSlot_EmptySlot: slot has no item → error "$slot is empty".
func TestHandleBothDropSlot_EmptySlot(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.self2Invs[5] = inventory.New(5, 28, inventory.StackNormal)
	// No item in slot 0.

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s := newBothDropState(0, self, self2, mc, lookup, world)
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	err := Execute(s)
	if err == nil {
		t.Fatal("expected error for empty slot, got nil")
	}
	if !strings.Contains(err.Error(), "$slot is empty") {
		t.Errorf("expected '$slot is empty', got: %v", err)
	}
}

// TestHandleBothDropSlot_ProtectedGate_PrimaryMissing: inv.protect=true,
// scope!=SHARED, no PtrProtectedActivePlayer → error.
func TestHandleBothDropSlot_ProtectedGate_PrimaryMissing(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[6] = inventory.New(6, 28, inventory.StackNormal)
	lookup.self2Invs[6] = inventory.New(6, 28, inventory.StackNormal)
	lookup.selfInvs[6].Set(0, &inventory.Item{Id: 10, Count: 1})

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	sf := &ScriptFile{
		Name:             "test_BOTH_DROPSLOT",
		Opcodes:          []Opcode{OpBothDropSlot, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	s := Init(sf, self, false, nil, nil) // NOT protected
	s.Self2 = self2
	s.Pointers |= PtrActivePlayer2
	s.Configs = mc
	s.Inv = lookup
	s.World = world
	s.PushInt(6) // perm+protect inv
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	err := Execute(s)
	if err == nil {
		t.Fatal("expected protect error, got nil")
	}
	if !strings.Contains(err.Error(), "inv requires protected access") {
		t.Errorf("expected 'inv requires protected access', got: %v", err)
	}
}

// TestHandleBothDropSlot_ProtectedGate_SecondaryMissing: secondary=1,
// inv.protect=true, no PtrProtectedActivePlayer2 → error.
func TestHandleBothDropSlot_ProtectedGate_SecondaryMissing(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[6] = inventory.New(6, 28, inventory.StackNormal)
	lookup.self2Invs[6] = inventory.New(6, 28, inventory.StackNormal)
	lookup.self2Invs[6].Set(0, &inventory.Item{Id: 10, Count: 1})

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	sf := &ScriptFile{
		Name:             "test_BOTH_DROPSLOT",
		Opcodes:          []Opcode{OpBothDropSlot, OpReturn},
		IntOperands:      []int32{1, 0}, // secondary
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	s := Init(sf, self, true, nil, nil) // slot-0 protected, NOT slot-1
	s.Self2 = self2
	s.Pointers |= PtrActivePlayer2
	// No PtrProtectedActivePlayer2
	s.Configs = mc
	s.Inv = lookup
	s.World = world
	s.PushInt(6)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	err := Execute(s)
	if err == nil {
		t.Fatal("expected slot-1 protect error, got nil")
	}
	if !strings.Contains(err.Error(), "inv requires protected access") {
		t.Errorf("expected 'inv requires protected access', got: %v", err)
	}
}

// TestHandleBothDropSlot_NoActivePlayer: Self absent → error.
func TestHandleBothDropSlot_NoActivePlayer(t *testing.T) {
	mc := newBothDropSlotConfigs()
	sf := &ScriptFile{
		Name:             "test_BOTH_DROPSLOT",
		Opcodes:          []Opcode{OpBothDropSlot, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	s := Init(sf, nil, false, nil, nil)
	s.Configs = mc
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	err := Execute(s)
	if err == nil {
		t.Fatal("expected no-active-player error, got nil")
	}
	if !strings.Contains(err.Error(), "no active player") {
		t.Errorf("expected 'no active player', got: %v", err)
	}
}

// TestHandleBothDropSlot_InvDelZero: when slot lookup returns a non-nil item
// but its Count is 0 (completed==0 path), the handler returns nil with no
// AddObj call. Pins spec §5.4 #10.
func TestHandleBothDropSlot_InvDelZero(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	self2 := &mockPlayer{uidValue: 22}
	lookup, _, _ := newTwoPlayerInvFixture()
	lookup.self = self
	lookup.self2 = self2
	lookup.selfInvs[5] = inventory.New(5, 28, inventory.StackNormal)
	lookup.self2Invs[5] = inventory.New(5, 28, inventory.StackNormal)
	// Slot 0 has a non-nil item but Count=0; simulates slot vacated mid-handler.
	lookup.selfInvs[5].Set(0, &inventory.Item{Id: 10, Count: 0})

	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	s := newBothDropState(0, self, self2, mc, lookup, world)
	s.PushInt(5) // inv
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0) // slot
	s.PushInt(50)

	if err := handleBothDropSlot(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.addedCalls) != 0 {
		t.Errorf("AddObj: got %d calls, want 0 (completed==0 early-return)", len(world.addedCalls))
	}
}

// --- NAI-162 B3.2 INV_DROPALL (opcode 4309) tests ---

// newInvDropAllState builds a ScriptState for INV_DROPALL with the given
// intOperand (0=slot-0 protect, 1=slot-1 protect). Self is always set.
func newInvDropAllState(operand int32, self *mockPlayer, mc *mockConfigs, inv *inventory.Inventory, invID int, world WorldVars) *ScriptState {
	sf := &ScriptFile{
		Name:             "test_INV_DROPALL",
		Opcodes:          []Opcode{OpInvDropAll, OpReturn},
		IntOperands:      []int32{operand, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	s := Init(sf, self, true, nil, nil) // slot-0 protected
	s.Configs = mc
	s.World = world
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{invID: inv}}
	return s
}

// TestHandleInvDropAll_EmptyInv: all slots nil → no addObj, no wealth event.
func TestHandleInvDropAll_EmptyInv(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	inv := inventory.New(5, 28, inventory.StackNormal) // all nil slots
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	s := newInvDropAllState(0, self, mc, inv, 5, world)
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(50)

	if err := Execute(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.addedCalls) != 0 {
		t.Errorf("addObj: got %d, want 0 (empty inv)", len(world.addedCalls))
	}
	if len(self.addWealthEventCalls) != 0 {
		t.Errorf("AddWealthEvent: got %d, want 0 (empty inv)", len(self.addWealthEventCalls))
	}
}

// TestHandleInvDropAll_MixedSlots_ScopeNormal: 3 non-empty slots, SCOPE_NORMAL
// → 3 addObj calls, no wealth event.
func TestHandleInvDropAll_MixedSlots_ScopeNormal(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	inv := inventory.New(5, 28, inventory.StackNormal)
	inv.Set(0, &inventory.Item{Id: 10, Count: 2}) // untradeable
	inv.Set(1, &inventory.Item{Id: 20, Count: 3}) // tradeable
	inv.Set(2, &inventory.Item{Id: 10, Count: 1}) // untradeable
	// slots 3..27 remain nil
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	s := newInvDropAllState(0, self, mc, inv, 5, world)
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(50)

	if err := Execute(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.addedCalls) != 3 {
		t.Fatalf("addObj: got %d, want 3", len(world.addedCalls))
	}
	if len(self.addWealthEventCalls) != 0 {
		t.Errorf("AddWealthEvent: got %d, want 0 (non-SCOPE_PERM)", len(self.addWealthEventCalls))
	}
}

// TestHandleInvDropAll_TradeableSplit: tradeable obj → addObj.receiverID=-1
// (PublicReceiver). Untradeable → addObj.receiverID=self.UID(). Mirrors TS
// InvOps.ts:773-778.
func TestHandleInvDropAll_TradeableSplit(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	inv := inventory.New(5, 28, inventory.StackNormal)
	inv.Set(0, &inventory.Item{Id: 10, Count: 1}) // untradeable
	inv.Set(1, &inventory.Item{Id: 20, Count: 1}) // tradeable
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	s := newInvDropAllState(0, self, mc, inv, 5, world)
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(50)

	if err := Execute(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.addedCalls) != 2 {
		t.Fatalf("addObj: got %d, want 2", len(world.addedCalls))
	}
	// slot 0 (untradeable): receiverID = self.UID
	if world.addedCalls[0].receiverID != 11 {
		t.Errorf("untradeable receiver: got %d, want 11 (self.UID)", world.addedCalls[0].receiverID)
	}
	// slot 1 (tradeable): receiverID = -1 (PublicReceiver / Obj.NO_RECEIVER)
	if world.addedCalls[1].receiverID != -1 {
		t.Errorf("tradeable receiver: got %d, want -1 (PublicReceiver)", world.addedCalls[1].receiverID)
	}
}

// TestHandleInvDropAll_ScopePerm_AccumulatesWealthLog: SCOPE_PERM with
// 3 non-empty slots (ids 10, 20, 10) → 3 addObj + 1 wealth event with
// 2 line items (id=10 has accumulated count) and summed AccountValue.
// Mirrors TS InvOps.ts:750-763 Map accumulation (R8: keyed by objID).
func TestHandleInvDropAll_ScopePerm_AccumulatesWealthLog(t *testing.T) {
	mc := newBothDropSlotConfigs()
	self := &mockPlayer{uidValue: 11}
	inv := inventory.New(6, 28, inventory.StackNormal)
	inv.Set(0, &inventory.Item{Id: 10, Count: 3}) // cost=100 → 300
	inv.Set(1, &inventory.Item{Id: 20, Count: 2}) // cost=50  → 100
	inv.Set(2, &inventory.Item{Id: 10, Count: 5}) // cost=100 → 500; merges into id=10 → total count=8
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}

	s := newInvDropAllState(0, self, mc, inv, 6, world)
	s.PushInt(6)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(50)

	if err := Execute(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.addedCalls) != 3 {
		t.Fatalf("addObj: got %d, want 3", len(world.addedCalls))
	}
	if len(self.addWealthEventCalls) != 1 {
		t.Fatalf("AddWealthEvent: got %d, want 1 (SCOPE_PERM death)", len(self.addWealthEventCalls))
	}
	evt := self.addWealthEventCalls[0]
	if evt.EventType != WealthEventTypeDeath {
		t.Errorf("EventType: got %d, want %d (DEATH)", evt.EventType, WealthEventTypeDeath)
	}
	// AccountValue = (3+5)*100 + 2*50 = 800 + 100 = 900
	if evt.AccountValue != 900 {
		t.Errorf("AccountValue: got %d, want 900", evt.AccountValue)
	}
	// 2 distinct IDs: id=10 (count=8) and id=20 (count=2).
	if len(evt.AccountItems) != 2 {
		t.Fatalf("AccountItems: got %d items, want 2", len(evt.AccountItems))
	}
	// Validate the item with ID=10 has accumulated count=8.
	var got10, got20 *WealthItem
	for i := range evt.AccountItems {
		switch evt.AccountItems[i].ID {
		case 10:
			got10 = &evt.AccountItems[i]
		case 20:
			got20 = &evt.AccountItems[i]
		}
	}
	if got10 == nil || got10.Count != 8 {
		t.Errorf("id=10 item: got %+v, want Count=8", got10)
	}
	if got20 == nil || got20.Count != 2 {
		t.Errorf("id=20 item: got %+v, want Count=2", got20)
	}
}

// TestHandleInvDropAll_NoActivePlayer: no Self → error.
func TestHandleInvDropAll_NoActivePlayer(t *testing.T) {
	mc := newBothDropSlotConfigs()
	sf := &ScriptFile{
		Name:             "test_INV_DROPALL",
		Opcodes:          []Opcode{OpInvDropAll, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	s := Init(sf, nil, false, nil, nil)
	s.Configs = mc
	s.PushInt(5)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(50)

	err := Execute(s)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no active player") {
		t.Errorf("expected 'no active player', got: %v", err)
	}
}

// TestNAI162Probe_InvDropSlot_FiresUnderNodeDebug pins that the
// nai162.wealth.invdropslot gateway emits exactly one record when
// NodeDebug=true and the SCOPE_PERM path fires.
func TestNAI162Probe_InvDropSlot_FiresUnderNodeDebug(t *testing.T) {
	rec, lg := captureLogger()

	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	mp := &mockPlayer{uidValue: 99}
	s.Self = mp
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer
	s.NodeDebug = true
	s.Log = lg

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(7)
	invType.Size = 28
	invType.Protect = true
	invType.Scope = objtype.InvTypeScopePerm
	mc.invs[7] = invType
	sword := objtype.NewObjType(1277)
	sword.Name = "Rune Sword"
	sword.DebugName = "rune_sword"
	sword.Cost = 20000
	mc.objs[1277] = sword
	s.Configs = mc

	inv := inventory.New(7, 28, inventory.StackNormal)
	inv.Set(0, &inventory.Item{Id: 1277, Count: 1})
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{7: inv}}

	s.PushInt(7)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("handleInvDropSlot: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("probe records: got %d, want 1", len(rec.records))
	}
	r := rec.records[0]
	if r.Message != "nai162.wealth.invdropslot" {
		t.Errorf("message: got %q, want %q", r.Message, "nai162.wealth.invdropslot")
	}
	got := recordAttrs(r)
	if got["event_type"] != int64(WealthEventTypeDrop) {
		t.Errorf(`attr "event_type": got %v, want %d`, got["event_type"], WealthEventTypeDrop)
	}
	if got["value"] != int64(20000) {
		t.Errorf(`attr "value": got %v, want 20000`, got["value"])
	}
	if got["inv"] != int64(7) {
		t.Errorf(`attr "inv": got %v, want 7`, got["inv"])
	}
	if got["count"] != int64(1) {
		t.Errorf(`attr "count": got %v, want 1`, got["count"])
	}
}

// TestNAI162Probe_InvDropSlot_SuppressedWhenNodeDebugFalse pins that the
// nai162.wealth.invdropslot gateway is suppressed when NodeDebug=false.
func TestNAI162Probe_InvDropSlot_SuppressedWhenNodeDebugFalse(t *testing.T) {
	rec, lg := captureLogger()

	s := newTestState(minimalScript(OpReturn))
	w := newFakeWorldMembers()
	s.World = w
	mp := &mockPlayer{uidValue: 99}
	s.Self = mp
	s.Pointers |= PtrActivePlayer
	s.Pointers |= PtrProtectedActivePlayer
	// NodeDebug zero-value = false
	s.Log = lg

	mc := newTestInvConfigs()
	invType := objtype.NewInvType(7)
	invType.Size = 28
	invType.Protect = true
	invType.Scope = objtype.InvTypeScopePerm
	mc.invs[7] = invType
	sword := objtype.NewObjType(1277)
	sword.Name = "Rune Sword"
	sword.DebugName = "rune_sword"
	sword.Cost = 20000
	mc.objs[1277] = sword
	s.Configs = mc

	inv := inventory.New(7, 28, inventory.StackNormal)
	inv.Set(0, &inventory.Item{Id: 1277, Count: 1})
	s.Inv = &mockInvLookup{invs: map[int]*inventory.Inventory{7: inv}}

	s.PushInt(7)
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0)
	s.PushInt(50)

	if err := handleInvDropSlot(s); err != nil {
		t.Fatalf("handleInvDropSlot: %v", err)
	}

	if len(rec.records) != 0 {
		t.Errorf("probe records under NodeDebug=false: got %d, want 0", len(rec.records))
	}
}

// -- NAI-133 slot routing for single-player INV-write opcodes ----------------
//
// These tests pin the operand=1 → PtrProtectedActivePlayer2 routing for
// every single-player INV-write opcode that consults the protect/scope
// gate. Pre-fix all 13 opcodes hardcoded slot-0 (PtrProtectedActivePlayer),
// silently dropping the TS `ProtectedActivePlayer[state.intOperand]`
// indexing (InvOps.ts:64, 91, 119, 136, 149, 172, 220, 329, 333, 359,
// 363, 507, 511, 543, 547, 578, 582, 607).
//
// Strategy: for each opcode, push the TS popInts() order with the protect
// inv as the source/target, run via a fresh Init() (so PC=0 lines up with
// IntOperands[0]), and assert error/success on the slot-1 protect flag.
// Sub-tests:
//   - operand=1 + only PtrProtectedActivePlayer (slot-0) set → error
//   - operand=1 + PtrProtectedActivePlayer2 (slot-1) set → no error
//
// runInvOpProtectSlot1 builds the state, sets s.Script.IntOperands[0]=1,
// applies the requested protect-flag mask, pushes inputs, and runs Execute.
// Returns (err, state) so the caller can inspect post-execution state.
func runInvOpProtectSlot1(t *testing.T, op Opcode, intInputs []int, protectFlags Pointer) (error, *ScriptState) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{1, 0}, // operand=1 at PC=0 → slot-1 routing
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{
		uidValue:    12345,
		coordPacked: coordgrid.PackCoord(0, 3200, 3200),
		x:           3200,
		z:           3200,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | protectFlags
	state.Inv = newTestInvLookup()
	mc := newTestInvConfigs()
	mc.invs[testInvMain].Protect = true // gate must fire on slot routing
	state.Configs = mc
	world := &fakeWorldAddObj{mockWorld: newMockWorld()}
	state.World = world
	for _, v := range intInputs {
		state.PushInt(v)
	}
	return Execute(state), state
}

// invWriteOpcodeProtectSlot1Cases enumerates each single-player INV-write
// opcode plus the TS pop-order inputs that route through the protect gate.
// Pre-loaded items in the inventory ensure the protect gate is reached
// (otherwise Remove/Get would short-circuit before the gate fires) — but
// since the gate fires AFTER validators and BEFORE inv mutations, this is
// only required for op-shapes whose validators depend on slot contents
// (none in this set: every gate is reached purely by validator chains over
// pushed inputs).
//
// INV_DROPSLOT is included even though its gate uses
// requireProtectedActivePlayer{,2} (not the inline pattern) — the routing
// branch is the same shape.
func invWriteOpcodeProtectSlot1Cases() []struct {
	name   string
	op     Opcode
	inputs []int
} {
	coord := coordgrid.PackCoord(0, 3200, 3200)
	return []struct {
		name   string
		op     Opcode
		inputs []int
	}{
		// TS popInts(3) [inv, obj, count]
		{"INV_ADD", OpInvAdd, []int{testInvMain, testObjCoin, 1}},
		// TS popInts(4) [inv, find, replace, replaceCount]
		{"INV_CHANGESLOT", OpInvChangeSlot, []int{testInvMain, testObjCoin, testObjCoin, 1}},
		// TS popInt() — inv only
		{"INV_CLEAR", OpInvClear, []int{testInvMain}},
		// TS popInts(3) [inv, obj, count]
		{"INV_DEL", OpInvDel, []int{testInvMain, testObjCoin, 1}},
		// TS popInts(2) [inv, slot]
		{"INV_DELSLOT", OpInvDelSlot, []int{testInvMain, 0}},
		// TS popInts(5) [inv, coord, obj, count, duration]
		{"INV_DROPITEM", OpInvDropItem, []int{testInvMain, coord, testObjCoin, 1, 100}},
		// TS popInts(4) [inv, coord, slot, duration]; slot 0 must exist
		// for the post-gate inv.Get to succeed in success-path tests.
		{"INV_DROPSLOT", OpInvDropSlot, []int{testInvMain, coord, 0, 100}},
		// TS popInts(3) [fromInv, toInv, fromSlot] — fromSlot must exist
		// for post-gate path; but gate fires first, so any slot suffices
		// for slot-1-error case. For success case, we pre-seed below.
		{"INV_MOVEFROMSLOT", OpInvMoveFromSlot, []int{testInvMain, testInvBank, 0}},
		// TS popInts(4) [fromInv, toInv, obj, count]
		{"INV_MOVEITEM", OpInvMoveItem, []int{testInvMain, testInvBank, testObjCoin, 1}},
		// TS popInts(4) [fromInv, toInv, obj, count]
		{"INV_MOVEITEM_CERT", OpInvMoveItemCert, []int{testInvMain, testInvBank, testObjCoin, 1}},
		// TS popInts(4) [fromInv, toInv, obj, count]
		{"INV_MOVEITEM_UNCERT", OpInvMoveItemUncert, []int{testInvMain, testInvBank, testObjCoin, 1}},
		// TS popInts(4) [fromInv, toInv, fromSlot, toSlot]
		{"INV_MOVETOSLOT", OpInvMoveToSlot, []int{testInvMain, testInvBank, 0, 0}},
		// TS popInts(4) [inv, slot, obj, count]
		{"INV_SETSLOT", OpInvSetSlot, []int{testInvMain, 0, testObjCoin, 1}},
	}
}

// TestInvWriteOpcodes_ProtectGate_Operand1_RequiresSlot1Pointer pins the
// negative branch: operand=1 + Protect=true + only slot-0 pointer set →
// error mentioning "protected access". Pre-fix every opcode would have
// silently passed (slot-0 set is enough under the hardcoded check).
func TestInvWriteOpcodes_ProtectGate_Operand1_RequiresSlot1Pointer(t *testing.T) {
	for _, tc := range invWriteOpcodeProtectSlot1Cases() {
		t.Run(tc.name, func(t *testing.T) {
			err, _ := runInvOpProtectSlot1(t, tc.op, tc.inputs, PtrProtectedActivePlayer)
			if err == nil {
				t.Fatalf("operand=1 with only slot-0 protect flag: expected error, got nil")
			}
			if !strings.Contains(err.Error(), "protected access") {
				t.Fatalf("err: got %q, want substring \"protected access\"", err)
			}
		})
	}
}

// TestInvWriteOpcodes_ProtectGate_Operand1_PassesWithSlot1Pointer pins the
// positive branch: operand=1 + Protect=true + slot-1 pointer set passes
// the gate. (Some opcodes may return a downstream non-protect error — e.g.
// INV_MOVEFROMSLOT errors when the source slot is empty. We accept any
// outcome that is NOT a "protected access" error: success or downstream
// failure both prove the gate was passed.)
func TestInvWriteOpcodes_ProtectGate_Operand1_PassesWithSlot1Pointer(t *testing.T) {
	for _, tc := range invWriteOpcodeProtectSlot1Cases() {
		t.Run(tc.name, func(t *testing.T) {
			err, _ := runInvOpProtectSlot1(t, tc.op, tc.inputs, PtrProtectedActivePlayer2)
			if err != nil && strings.Contains(err.Error(), "protected access") {
				t.Fatalf("operand=1 with slot-1 protect flag: gate should pass, got %q", err)
			}
		})
	}
}

// TestInvWriteOpcodes_ProtectGate_BadOperand_Errors pins the
// out-of-range operand path: operand=2 → "invalid intOperand %d" error
// per parity with handleInvDropItemDelayed / handleBothMoveInv.
func TestInvWriteOpcodes_ProtectGate_BadOperand_Errors(t *testing.T) {
	for _, tc := range invWriteOpcodeProtectSlot1Cases() {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name:             "test_" + tc.op.String(),
				Opcodes:          []Opcode{tc.op, OpReturn},
				IntOperands:      []int32{2, 0}, // out of range
				StringOperands:   []string{"", ""},
				InstructionCount: 2,
			}
			mp := &mockPlayer{
				uidValue:    12345,
				coordPacked: coordgrid.PackCoord(0, 3200, 3200),
				x:           3200,
				z:           3200,
			}
			state := Init(sf, mp, false, nil, nil)
			state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer | PtrProtectedActivePlayer2
			state.Inv = newTestInvLookup()
			mc := newTestInvConfigs()
			mc.invs[testInvMain].Protect = true
			state.Configs = mc
			world := &fakeWorldAddObj{mockWorld: newMockWorld()}
			state.World = world
			for _, v := range tc.inputs {
				state.PushInt(v)
			}
			err := Execute(state)
			if err == nil {
				t.Fatalf("operand=2: expected error, got nil")
			}
			if !strings.Contains(err.Error(), "invalid intOperand") {
				t.Fatalf("err: got %q, want substring \"invalid intOperand\"", err)
			}
		})
	}
}
