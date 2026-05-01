package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/script"
)

func TestModalCloseEmitsStopTransmit(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
		150: {Type: 93, Com: 150, Source: -1, FirstSeen: false},
	}
	p.refreshModalClose = true

	received := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	// Expected wire:
	//   1 byte IfClose (opcode, no payload)
	//   + 2 * 3 bytes UpdateInvStopTransmit (1 opcode + 2 payload)
	// Total = 1 + 6 = 7 bytes.
	if len(got) != 7 {
		t.Errorf("got %d bytes, want 7 (IfClose + 2× StopTransmit); bytes=%v", len(got), got)
	}
	if len(p.invListeners) != 0 {
		t.Errorf("invListeners should be cleared; got %d", len(p.invListeners))
	}
}

func TestNoStopTransmitWithoutModalClose(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
	}
	p.refreshModalClose = false

	received := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("no modal close → no stop-transmit; got %d bytes", len(got))
	}
	if len(p.invListeners) != 1 {
		t.Errorf("invListeners should be untouched; got %d", len(p.invListeners))
	}
}

// TestCloseModalClearsWeakQueueWhenTrue pins CloseModal(true) drops weak
// queue entries. Mirrors TS Player.closeModal default arg path
// (Player.ts:742-744).
func TestCloseModalClearsWeakQueueWhenTrue(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	p.CloseModal(true)

	if got, want := len(p.queue), 1; got != want {
		t.Fatalf("queue len: got %d, want %d (weak should be dropped)", got, want)
	}
	if p.queue[0].Type != script.QueueStrong {
		t.Errorf("queue[0].Type: got %v, want QueueStrong", p.queue[0].Type)
	}
}

// TestCloseModalPreservesWeakQueueWhenFalse pins CloseModal(false)
// preserves weak queue entries. Mirrors TS Player.closeModal(false)
// path (Player.ts:2148 caller).
func TestCloseModalPreservesWeakQueueWhenFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	p.CloseModal(false)

	if got, want := len(p.queue), 2; got != want {
		t.Fatalf("queue len: got %d, want %d (weak should be preserved)", got, want)
	}
}

// TestCloseModalClearsActiveScriptProtectWhenNotDelayed pins
// !delayed && activeScript != nil → activeScript.Protect = false.
// Mirrors TS Player.closeModal !delayed → protect=false branch
// (Player.ts:745-747), applied via NAI-52 convergence (TS this.protect ↔
// goscape activeScript.Protect).
func TestCloseModalClearsActiveScriptProtectWhenNotDelayed(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = false
	p.activeScript = &script.ScriptState{
		Script:  &script.ScriptFile{Name: "running"},
		Protect: true,
	}

	p.CloseModal(true)

	if p.activeScript == nil {
		t.Fatal("activeScript: got nil, want preserved (Suspended/Running scripts not nulled)")
	}
	if p.activeScript.Protect {
		t.Errorf("activeScript.Protect: got true, want false (!delayed should clear)")
	}
	if p.protectedScriptActive() {
		t.Errorf("protectedScriptActive(): got true, want false (NAI-52 convergence)")
	}
}

// TestCloseModalPreservesActiveScriptProtectWhenDelayed pins
// delayed → activeScript.Protect preserved.
// Mirrors TS Player.closeModal `if (!this.delayed)` guard.
func TestCloseModalPreservesActiveScriptProtectWhenDelayed(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = true
	p.activeScript = &script.ScriptState{
		Script:  &script.ScriptFile{Name: "running"},
		Protect: true,
	}

	p.CloseModal(true)

	if p.activeScript == nil {
		t.Fatal("activeScript: got nil, want preserved")
	}
	if !p.activeScript.Protect {
		t.Errorf("activeScript.Protect: got false, want true (delayed should preserve)")
	}
}

// TestCloseModalNilActiveScriptNoPanic pins !delayed + nil activeScript
// is a no-op (no panic). Mirrors TS where `this.protect = false` is a
// no-op when no script is suspended.
func TestCloseModalNilActiveScriptNoPanic(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = false
	p.activeScript = nil

	// Should not panic.
	p.CloseModal(true)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil")
	}
}

// TestCloseModalNoneEarlyReturnPreservesRefreshModalClose pins
// modalState == NONE early-return. When no modal is open, CloseModal
// must NOT touch refreshModalClose (avoids redundant wire IF_CLOSE).
// Mirrors TS Player.closeModal `if (modalState === NONE) return`
// (Player.ts:749-751).
func TestCloseModalNoneEarlyReturnPreservesRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateNone
	p.modalMain = -1
	p.modalChat = -1
	p.modalSide = -1
	p.refreshModalClose = false

	p.CloseModal(true)

	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (NONE state must early-return)")
	}
}

// TestCloseModalNonNoneResetsAllSlots pins that with any modal open,
// all three slots are reset to -1, modalState becomes NONE, and
// refreshModalClose is set true.
func TestCloseModalNonNoneResetsAllSlots(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain
	p.modalMain = 42
	p.modalChat = 88
	p.modalSide = 99
	p.refreshModalClose = false

	p.CloseModal(true)

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1", p.modalMain)
	}
	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1", p.modalChat)
	}
	if p.modalSide != -1 {
		t.Errorf("modalSide: got %d, want -1", p.modalSide)
	}
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %#x, want %#x (NONE)", p.modalState, modalStateNone)
	}
	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (modal was open)")
	}
}

// TestCloseModalNoneEarlyReturnStillRunsClearWeakQueueAndProtect pins
// the early-return is positioned AFTER weak-queue clearing and the
// !delayed protect-clear (TS Player.ts:742-748 — both run before the
// modalState check).
func TestCloseModalNoneEarlyReturnStillRunsClearWeakQueueAndProtect(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueWeak},
	}
	p.delayed = false
	p.activeScript = &script.ScriptState{
		Script:  &script.ScriptFile{Name: "running"},
		Protect: true,
	}
	p.modalState = modalStateNone

	p.CloseModal(true)

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (weak should be cleared even on NONE early-return)", len(p.queue))
	}
	if p.activeScript == nil || p.activeScript.Protect {
		t.Errorf("activeScript.Protect should be cleared even on NONE early-return")
	}
}

// TestCloseModalNullsActiveScriptOnCountDialog pins COUNTDIALOG-suspended
// activeScript is nulled on CloseModal. Closes NAI-52-F1.
// Mirrors TS Player.closeModal Player.ts:756-758.
func TestCloseModalNullsActiveScriptOnCountDialog(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 7
	p.activeScript = &script.ScriptState{
		Script:    &script.ScriptFile{Name: "dialog"},
		Execution: script.CountDialog,
	}

	p.CloseModal(true)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (COUNTDIALOG must be cleared)")
	}
}

// TestCloseModalNullsActiveScriptOnPauseButton pins PAUSEBUTTON-suspended
// activeScript is nulled on CloseModal. Closes NAI-52-F1.
func TestCloseModalNullsActiveScriptOnPauseButton(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 7
	p.activeScript = &script.ScriptState{
		Script:    &script.ScriptFile{Name: "pause"},
		Execution: script.PauseButton,
	}

	p.CloseModal(true)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (PAUSEBUTTON must be cleared)")
	}
}

// TestCloseModalPreservesActiveScriptOnSuspended pins Suspended (non-dialog)
// activeScript is preserved on CloseModal. Mirrors TS exclusion of
// non-COUNTDIALOG/PAUSEBUTTON execution states from the null branch.
func TestCloseModalPreservesActiveScriptOnSuspended(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = true // delayed so the protect-clear block doesn't fire
	p.modalState = modalStateChat
	p.modalChat = 7
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "suspended"},
		Execution: script.Suspended,
		Protect:   true,
	}
	p.activeScript = state

	p.CloseModal(true)

	if p.activeScript != state {
		t.Errorf("activeScript: got %v, want preserved %v (Suspended must NOT be cleared)", p.activeScript, state)
	}
}

// TestCloseModalIfCloseDispatchMain pins per-slot IF_CLOSE dispatch
// for modalMain. Mirrors TS Player.closeModal:761-769.
func TestCloseModalIfCloseDispatchMain(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	ifCloseScript := &script.ScriptFile{
		Name:        "[if_close,42]",
		LookupKey:   script.LookupKeyForType(script.TriggerIfClose, 42),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(ifCloseScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateMain
	p.modalMain = 42

	p.CloseModal(true)

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1", p.modalMain)
	}
	// Script is registered, so dispatch path was taken; OpReturn finishes
	// immediately so activeScript is nil.
	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (IF_CLOSE script returned)")
	}
}

// TestCloseModalIfCloseDispatchChat pins per-slot IF_CLOSE dispatch
// for modalChat (slot lookup uses modalChat com ID).
func TestCloseModalIfCloseDispatchChat(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	ifCloseScript := &script.ScriptFile{
		Name:        "[if_close,88]",
		LookupKey:   script.LookupKeyForType(script.TriggerIfClose, 88),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(ifCloseScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateChat
	p.modalChat = 88

	p.CloseModal(true)

	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1", p.modalChat)
	}
}

// TestCloseModalIfCloseDispatchSide pins per-slot IF_CLOSE dispatch
// for modalSide.
func TestCloseModalIfCloseDispatchSide(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	ifCloseScript := &script.ScriptFile{
		Name:        "[if_close,99]",
		LookupKey:   script.LookupKeyForType(script.TriggerIfClose, 99),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(ifCloseScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateSide
	p.modalSide = 99

	p.CloseModal(true)

	if p.modalSide != -1 {
		t.Errorf("modalSide: got %d, want -1", p.modalSide)
	}
}

// TestCloseModalIfCloseMissingScriptNoOp pins that an open slot with no
// registered IF_CLOSE script is a silent no-op (slot still resets, no
// panic). Mirrors TS where `if (closeTrigger)` guards the executeScript.
func TestCloseModalIfCloseMissingScriptNoOp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateMain
	p.modalMain = 42

	// Should not panic.
	p.CloseModal(true)

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1", p.modalMain)
	}
}

// TestCloseModalNilScriptProviderNoOp pins that nil scriptProvider is
// a silent no-op (slots still reset). Defensive — covers test paths
// that don't seed scriptProvider.
func TestCloseModalNilScriptProviderNoOp(t *testing.T) {
	s := newTestServer(t)
	// s.scriptProvider intentionally nil.
	s.scriptProvider = nil
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateMain
	p.modalMain = 42

	// Should not panic.
	p.CloseModal(true)

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1", p.modalMain)
	}
}

// TestCloseModalCombinedPauseButtonNullAndPerSlotDispatch pins the
// interaction of two NAI-53 T5 branches in a single fixture:
//
//	(a) PAUSEBUTTON-suspended activeScript is nulled (NAI-52-F1 closure
//	    branch, modal_close_test.go:257-271 covers in isolation).
//	(b) Per-slot IF_CLOSE dispatch fires for the open chat slot (T5
//	    per-slot trigger-script port, modal_close_test.go:329-352
//	    covers in isolation).
//
// NAI-53 T5's quality review surfaced this as a coverage gap: the null
// tests use newTestPlayer without a server, and the dispatch tests use
// fresh ScriptStates left at zero-value Running execution. This test
// puts them in the same fixture. NAI-54 T4 (closes NAI-53-F2).
func TestCloseModalCombinedPauseButtonNullAndPerSlotDispatch(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	ifCloseScript := &script.ScriptFile{
		Name:        "[if_close,77]",
		LookupKey:   script.LookupKeyForType(script.TriggerIfClose, 77),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(ifCloseScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalState = modalStateChat
	p.modalChat = 77
	pausedState := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "paused-dialog"},
		Execution: script.PauseButton,
	}
	p.activeScript = pausedState

	p.CloseModal(true)

	// (a) PauseButton-state activeScript was nulled.
	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (PauseButton must be cleared)")
	}
	// (b) Per-slot dispatch fired (slot reset, modalState cleared,
	// refreshModalClose set; the dispatched IF_CLOSE script is OpReturn
	// so it Finishes immediately and does not re-store activeScript).
	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1 (slot reset)", p.modalChat)
	}
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %#x, want %#x (NONE)", p.modalState, modalStateNone)
	}
	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true")
	}
}
