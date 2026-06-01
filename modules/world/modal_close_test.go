package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// TestEncodeOutOnRefreshModalCloseWritesIfCloseOnly pins that
// encodeOut emits ONLY OpIfClose when refreshModalClose is set —
// per-listener UpdateInvStopTransmit packets are now written at
// CloseModal time via clearComListeners (NAI-64 atomic switchover).
// Listener map is left intact; CloseModal owns the per-slot removals.
func TestEncodeOutOnRefreshModalCloseWritesIfCloseOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// Pre-load invListeners; encodeOut must NOT touch them.
	p.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
		150: {Type: 93, Com: 150, Source: -1, FirstSeen: false},
	}
	p.refreshModalClose = true

	received := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	// Wire: 1 byte OpIfClose (opcode, no payload). Nothing else.
	if len(got) != 1 {
		t.Errorf("got %d bytes, want 1 (IfClose only); bytes=%v", len(got), got)
	}
	if len(p.invListeners) != 2 {
		t.Errorf("invListeners must be untouched by encodeOut; got %d", len(p.invListeners))
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

// TestCloseModalNilActiveScriptNoPanic pins nil activeScript is a no-op
// (no panic). Defensive coverage for the modalStateNone early-return
// path; CloseModal never derefs activeScript post-NAI-111.
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

// TestCloseModalNoneEarlyReturnStillRunsClearWeakQueue pins the
// weak-queue clearing runs BEFORE the modalState == NONE early-return
// (TS Player.ts:742-744 — clearWeakQueue runs before the modalState
// check).
func TestCloseModalNoneEarlyReturnStillRunsClearWeakQueue(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueWeak},
	}
	p.modalState = modalStateNone

	p.CloseModal(true)

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (weak should be cleared even on NONE early-return)", len(p.queue))
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
	p.modalState = modalStateChat
	p.modalChat = 7
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "suspended"},
		Execution: script.Suspended,
		Pointers:  script.PtrProtectedActivePlayer,
	}
	p.activeScript = state

	p.CloseModal(true)

	if p.activeScript != state {
		t.Errorf("activeScript: got %v, want preserved %v (Suspended must NOT be cleared)", p.activeScript, state)
	}
}

// TestCloseModalPreservesInFlightProtectOnResumedScript pins NAI-111:
// CloseModal must NOT strip PtrProtectedActivePlayer from p.activeScript.
// During a resumed script's in-flight execution (Execution=Running),
// p.activeScript IS the in-flight ScriptState — handlers downstream of
// tut_close/if_close (e.g. p_telejump) read s.Pointers&PAP via
// requireProtectedActivePlayer. TS Player.closeModal (Player.ts:741-794)
// touches no script pointer. Regression for NAI-53 T3's incorrect
// NAI-52-convergence over-clear.
func TestCloseModalPreservesInFlightProtectOnResumedScript(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = false
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "tutorial_complete"},
		Execution: script.Running,
		Pointers:  script.PtrActivePlayer | script.PtrProtectedActivePlayer,
	}
	p.activeScript = state

	p.CloseModal(true)

	if p.activeScript != state {
		t.Fatalf("activeScript: got %v, want preserved %v (Running must NOT be cleared)", p.activeScript, state)
	}
	if p.activeScript.Pointers&script.PtrProtectedActivePlayer == 0 {
		t.Errorf("activeScript.PtrProtectedActivePlayer: got clear, want set (CloseModal must not strip mid-flight protect)")
	}
	if p.activeScript.Pointers&script.PtrActivePlayer == 0 {
		t.Errorf("activeScript.PtrActivePlayer: got clear, want set (CloseModal must not touch any script pointer)")
	}
}

// TestCloseModal_NotDelayed_ProtectClearedTSFaithful pins NAI-111-D1
// closure: TS Player.closeModal at Player.ts:746 unconditionally sets
// this.protect=false even when activeScript is mid-flight with PAP
// still set on its Pointers. Goscape's CloseModal mirrors that clear
// against the new Player.protect field. The script-state PAP pointer
// (activeScript.Pointers&PtrProtectedActivePlayer) is NOT touched —
// downstream handler pointerCheck (p_telejump etc.) still works,
// preserving the NAI-53 T3 regression fix where clearing PAP-on-state
// broke in-flight resumed scripts like tut_close inside
// [label,tutorial_complete] aborting P_TELEJUMP.
//
// The companion test TestCloseModalPreservesInFlightProtectOnResumedScript
// pins the activeScript.Pointers preservation; this test pins the
// Player.protect clear at the gate layer.
func TestCloseModal_NotDelayed_ProtectClearedTSFaithful(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.delayed = false
	p.protect = true
	p.activeScript = &script.ScriptState{
		Script:    &script.ScriptFile{Name: "tutorial_complete"},
		Execution: script.Running,
		Pointers:  script.PtrActivePlayer | script.PtrProtectedActivePlayer,
	}
	p.modalState = modalStateMain // ensure CloseModal does work (early-returns when None)

	p.CloseModal(true)

	if p.protect {
		t.Error("p.protect: got true, want false (CloseModal must clear gate, matching TS Player.ts:746)")
	}
	if p.protectedScriptActive() {
		t.Error("protectedScriptActive(): got true, want false (gate cleared)")
	}
	// Companion preservation: activeScript + PAP-on-state untouched.
	if p.activeScript == nil {
		t.Fatal("activeScript: got nil, want preserved (mid-flight script protected via state.Pointers)")
	}
	if p.activeScript.Pointers&script.PtrProtectedActivePlayer == 0 {
		t.Error("activeScript.PtrProtectedActivePlayer: got clear, want preserved (handler pointerCheck source)")
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

// -- NAI-102: CloseTutorial tests -----------------------------------------

// TestCloseTutorial_EarlyReturnsWhenNoTutorialOpen pins the TS-faithful
// no-op when modalTutorial == -1 (Player.ts:716-726 early-returns).
// Mirrors TestCloseModalIfCloseDispatchMain fixture shape.
func TestCloseTutorial_EarlyReturnsWhenNoTutorialOpen(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	// Fresh player: modalTutorial defaults to -1, modalState == modalStateNone.

	p.CloseTutorial()

	if p.modalTutorial != -1 {
		t.Errorf("modalTutorial: got %d, want -1", p.modalTutorial)
	}
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %#x, want modalStateNone (%#x)", p.modalState, modalStateNone)
	}
}

// TestCloseTutorial_DispatchesIfCloseTriggerAndResets pins TS Player.ts:716-726:
// when modalTutorial != -1, dispatch the matching IF_CLOSE trigger script
// (if registered) and reset the tutorial slot + clear modalStateTut.
func TestCloseTutorial_DispatchesIfCloseTriggerAndResets(t *testing.T) {
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
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = enc
	p.modalTutorial = 42
	p.modalState |= modalStateTut

	p.CloseTutorial()

	if p.modalTutorial != -1 {
		t.Errorf("modalTutorial: got %d, want -1", p.modalTutorial)
	}
	if p.modalState&modalStateTut != 0 {
		t.Errorf("modalState&modalStateTut: got %#x, want 0", p.modalState&modalStateTut)
	}
	// Script is registered, so dispatch path was taken; OpReturn finishes
	// immediately so activeScript is nil.
	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (IF_CLOSE script returned)")
	}
}

// TestCloseTutorial_NoIfCloseTriggerStillResets pins that a missing
// registered IF_CLOSE script is a silent no-op for dispatch but still
// resets the tutorial slot. Mirrors TS `if (closeTrigger)` guard.
func TestCloseTutorial_NoIfCloseTriggerStillResets(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = enc
	p.modalTutorial = 42
	p.modalState |= modalStateTut

	// Should not panic.
	p.CloseTutorial()

	if p.modalTutorial != -1 {
		t.Errorf("modalTutorial: got %d, want -1", p.modalTutorial)
	}
	if p.modalState&modalStateTut != 0 {
		t.Errorf("modalState&modalStateTut: got %#x, want 0", p.modalState&modalStateTut)
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

// TestCloseModalClearsOnlyListenersForClosingSlots pins TS
// Player.ts:761-791: clearComListeners is called per slot with the
// slot's modal id; only listeners whose Component.RootLayer matches a
// closing slot are removed. Closes NAI-53-D-CLEARCOMLISTENERS-PER-SLOT.
func TestCloseModalClearsOnlyListenersForClosingSlots(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 100}, // matches modalMain
		250: {RootLayer: 200}, // matches modalSide
		300: {RootLayer: 999}, // unrelated
	})
	p.invListenOnCom(93, 149, -1)
	p.invListenOnCom(93, 250, -1)
	p.invListenOnCom(93, 300, -1)
	p.modalState = modalStateMain | modalStateSide
	p.modalMain = 100
	p.modalChat = -1
	p.modalSide = 200

	received := drainConn(t, cc)
	p.CloseModal(true)
	p.client.flushWrite()

	got := <-received
	// Two UpdateInvStopTransmit packets (149 + 250) = 6 bytes;
	// IF_CLOSE wire packet is emitted later by encodeOut, not here.
	if len(got) != 6 {
		t.Errorf("packet bytes: got %d, want 6 (2× StopTransmit); bytes=%v", len(got), got)
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener 149 (RootLayer 100 == modalMain) should be removed")
	}
	if _, ok := p.invListeners[250]; ok {
		t.Error("listener 250 (RootLayer 200 == modalSide) should be removed")
	}
	if _, ok := p.invListeners[300]; !ok {
		t.Error("listener 300 (RootLayer 999) should be retained")
	}
	if !p.refreshModalClose {
		t.Error("CloseModal should set refreshModalClose=true")
	}
}

// TestCloseModalNoListenersStillClosesAndWritesIfClose pins that
// CloseModal with empty invListeners produces zero per-listener
// packets but still flags refreshModalClose so the next encodeOut
// writes OpIfClose.
func TestCloseModalNoListenersStillClosesAndWritesIfClose(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		100: {RootLayer: 100},
	})
	p.modalState = modalStateMain
	p.modalMain = 100
	// invListeners deliberately left nil.

	received := drainConn(t, cc)
	p.CloseModal(true)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	// Wire: 1 byte OpIfClose only.
	if len(got) != 1 {
		t.Errorf("got %d bytes, want 1 (IfClose only); bytes=%v", len(got), got)
	}
}
