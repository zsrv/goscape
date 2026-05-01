package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// TestCloseModalHandlerSetsRequestModalClose pins that handleCloseModal sets
// requestModalClose and does NOT immediately call CloseModal (TS semantics:
// modal is deferred until processPlayerQueue).
func TestCloseModalHandlerSetsRequestModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalMain = 42

	_ = handleCloseModal(p, nil)

	if !p.requestModalClose {
		t.Error("requestModalClose: want true, got false")
	}
	if p.modalMain != 42 {
		t.Errorf("modalMain changed prematurely: got %d, want 42", p.modalMain)
	}
}

// TestProcessPlayerQueueConsumesRequestModalClose pins that processPlayerQueue
// calls CloseModal before running queued scripts when requestModalClose is set.
func TestProcessPlayerQueueConsumesRequestModalClose(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalMain = 10
	p.modalState = modalStateMain
	p.requestModalClose = true

	s.processPlayerQueue(p)

	if p.requestModalClose {
		t.Error("requestModalClose: want false after processPlayerQueue")
	}
	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1 (CloseModal should have fired)", p.modalMain)
	}
}

// TestProcessPlayerQueueStrongQueueClosesModal pins that a STRONG-typed queue
// entry causes modal close even when requestModalClose was false before the
// tick (TS processQueues lines 854-860).
func TestProcessPlayerQueueStrongQueueClosesModal(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.modalMain = 99
	p.modalState = modalStateMain

	p.queue = append(p.queue, playerQueueRequest{
		Type:  script.QueueStrong,
		Delay: 0,
	})

	s.processPlayerQueue(p)

	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1 (STRONG queue should trigger CloseModal)", p.modalMain)
	}
}

// TestHandleTutClickSideOutOfRange pins that tab values outside [0,13]
// are silently dropped (TS TutClickSideHandler.ts:13-15).
func TestHandleTutClickSideOutOfRange(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s

	for _, tab := range []int{14, 255} {
		if err := s.handleTutClickSide(p, []byte{byte(tab)}); err != nil {
			t.Errorf("tab %d: unexpected error: %v", tab, err)
		}
		if p.activeScript != nil {
			t.Errorf("tab %d: activeScript set unexpectedly", tab)
		}
	}
}

// TestHandleTutClickSideFiresTutorialScript pins that a valid tab fires
// the global [tutorial] script (TS TutClickSideHandler.ts:17-20).
func TestHandleTutClickSideFiresTutorialScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	tutScript := &script.ScriptFile{
		Name:        "[tutorial]",
		LookupKey:   script.LookupKeyForGlobal(script.TriggerTutorial),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(tutScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s

	if err := s.handleTutClickSide(p, []byte{7}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Script returns immediately; activeScript is nil after finish.
	if p.activeScript != nil {
		t.Errorf("activeScript: want nil after RETURN, got %v", p.activeScript)
	}
}

// TestHandleTutClickSideNoScriptNoOp pins that missing [tutorial] script
// is a silent no-op (no panic, no error).
func TestHandleTutClickSideNoScriptNoOp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s

	if err := s.handleTutClickSide(p, []byte{0}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// seedComponentTypes installs an inline component-type registry on s.
// Sparse — id N is placed at index N. The largest id determines slice
// length.
func seedComponentTypes(t *testing.T, s *Server, components map[int]*objtype.ComponentType) {
	t.Helper()
	maxId := 0
	for id := range components {
		if id > maxId {
			maxId = id
		}
	}
	configs := make([]*objtype.ComponentType, maxId+1)
	for id, c := range components {
		configs[id] = c
	}
	s.componentTypes = &objtype.ComponentTypeConfigs{
		ConfigNames: map[string]int{},
		Configs:     configs,
	}
}

// TestHandleIfButtonSetsLastCom pins that lastCom is set after gates pass.
func TestHandleIfButtonSetsLastCom(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	const comId = 42
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		comId: {RootLayer: comId, ButtonType: objtype.Button},
	})
	p.tabs[0] = comId // visible via tab slot

	_ = s.handleIfButton(p, []byte{0, comId})

	if p.lastCom != comId {
		t.Errorf("lastCom: got %d, want %d", p.lastCom, comId)
	}
}

// TestHandleIfButtonResumesPauseButton pins the resume path: comId in
// resumeButtons + activeScript in PauseButton → resumes execution.
func TestHandleIfButtonResumesPauseButton(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Minimal script: RETURN.
	retScript := &script.ScriptFile{
		Name:        "[test_resume]",
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.resumeButtons = [5]int{7, 0, 0, 0, 0}
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7: {RootLayer: 7, ButtonType: objtype.Button},
	})
	p.tabs[0] = 7

	// Build an already-suspended script state.
	st := script.Init(retScript, p, false, nil, nil)
	st.Provider = s.scriptProvider
	st.Configs = s.configsView
	st.Inv = s.invLookup
	st.Npcs = s.npcLookup
	st.PlayerLookup = s
	st.Execution = script.PauseButton
	p.StoreActiveScript(st)

	_ = s.handleIfButton(p, []byte{0, 7}) // com = 7

	// Script finishes (RETURN) → activeScript cleared.
	if p.activeScript != nil {
		t.Errorf("activeScript: want nil after resume+finish, got non-nil")
	}
}

// TestHandleIfButtonPauseButtonNotInResumeButtons pins that a PauseButton
// script is NOT resumed when comId is absent from resumeButtons
// (falls through to trigger lookup).
func TestHandleIfButtonPauseButtonNotInResumeButtons(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.resumeButtons = [5]int{99, 0, 0, 0, 0} // 7 is not in the list
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7: {RootLayer: 7, ButtonType: objtype.Button},
	})
	p.tabs[0] = 7

	st := &script.ScriptState{Execution: script.PauseButton}
	p.StoreActiveScript(st)

	_ = s.handleIfButton(p, []byte{0, 7}) // com = 7

	// activeScript unchanged (not resumed)
	if p.activeScript == nil || p.activeScript.Execution != script.PauseButton {
		t.Errorf("activeScript: want PauseButton state unchanged, got %v", p.activeScript)
	}
}

// TestHandleIfButtonFiresIfButtonScript pins the trigger-lookup path:
// comId not in resumeButtons → [if_button,<com>] script fires.
func TestHandleIfButtonFiresIfButtonScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	ifBtnScript := &script.ScriptFile{
		Name:        "[if_button,42]",
		LookupKey:   script.LookupKeyForType(script.TriggerIfButton, 42),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(ifBtnScript)
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		42: {RootLayer: 42, ButtonType: objtype.Button},
	})
	p.tabs[0] = 42

	_ = s.handleIfButton(p, []byte{0, 42})

	// Script returns immediately; no suspension.
	if p.activeScript != nil {
		t.Errorf("activeScript: want nil after RETURN, got non-nil")
	}
}

// TestHandleIfButtonNoScriptNoOp pins that missing [if_button,<com>]
// is a silent no-op even when gates pass.
func TestHandleIfButtonNoScriptNoOp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7: {RootLayer: 7, ButtonType: objtype.Button},
	})
	p.tabs[0] = 7

	if err := s.handleIfButton(p, []byte{0, 7}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestHandleIfButton_NilComponentRejects: registry is empty → component
// is nil → handler rejects before setting lastCom.
func TestHandleIfButton_NilComponentRejects(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s

	if err := s.handleIfButton(p, []byte{0, 42}); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	if p.lastCom != -1 {
		t.Errorf("lastCom: got %d, want -1 (nil component should reject)", p.lastCom)
	}
}

// TestHandleIfButton_NoButtonTypeRejects: component has ButtonType=NoButton
// → handler rejects before setting lastCom.
func TestHandleIfButton_NoButtonTypeRejects(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		42: {RootLayer: 100, ButtonType: objtype.ButtonNone},
	})
	p.tabs[0] = 100

	if err := s.handleIfButton(p, []byte{0, 42}); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	if p.lastCom != -1 {
		t.Errorf("lastCom: got %d, want -1 (ButtonNone should reject)", p.lastCom)
	}
}

// TestHandleIfButton_NotVisibleRejects: component RootLayer not in any
// modal/tab slot → handler rejects.
func TestHandleIfButton_NotVisibleRejects(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		42: {RootLayer: 100, ButtonType: objtype.Button},
	})
	// All tabs default -1; no modal open. Component not visible.

	if err := s.handleIfButton(p, []byte{0, 42}); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	if p.lastCom != -1 {
		t.Errorf("lastCom: got %d, want -1 (not visible should reject)", p.lastCom)
	}
}

// runIfButtonProtectScript fires handleIfButton against a Server seeded
// with rootLayer fixture, registers an [if_button,42] script that calls
// P_DELAY (which requires Protect=true), and reports whether the script
// suspended (Protect=true ran P_DELAY OK → Suspended) or aborted
// (Protect=false → requireProtectedActivePlayer rejects → Aborted).
//
// Returns true iff p.activeScript is non-nil and Suspended after the
// handler runs (i.e. handler computed protect=true).
func runIfButtonProtectScript(t *testing.T, rootOverlay bool, includeRoot bool) bool {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	// Script: PUSH_INT 1, P_DELAY, RETURN. P_DELAY requires Protect=true.
	sf := &script.ScriptFile{
		Name:             "[if_button,42]",
		LookupKey:        script.LookupKeyForType(script.TriggerIfButton, 42),
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpPDelay, script.OpReturn},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(sf)

	components := map[int]*objtype.ComponentType{
		42: {RootLayer: 100, ButtonType: objtype.Button},
	}
	if includeRoot {
		components[100] = &objtype.ComponentType{RootLayer: 100, ButtonType: objtype.ButtonNone, Overlay: rootOverlay}
	}
	p, _ := newTestPlayer(t)
	p.client.server = s
	seedComponentTypes(t, s, components)
	p.tabs[0] = 100 // make rootLayer visible

	if err := s.handleIfButton(p, []byte{0, 42}); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	return p.activeScript != nil && p.activeScript.Execution == script.Suspended
}

// TestHandleIfButton_OverlayRootSetsProtectFalse: rootLayer.Overlay=true
// → protect=false → P_DELAY's requireProtectedActivePlayer aborts the
// script → no suspended state.
func TestHandleIfButton_OverlayRootSetsProtectFalse(t *testing.T) {
	if got := runIfButtonProtectScript(t, true, true); got {
		t.Errorf("script suspended: got true, want false (Overlay=true should set protect=false → P_DELAY aborts)")
	}
}

// TestHandleIfButton_NonOverlayRootSetsProtectTrue: rootLayer.Overlay=false
// → protect=true → P_DELAY suspends successfully.
func TestHandleIfButton_NonOverlayRootSetsProtectTrue(t *testing.T) {
	if got := runIfButtonProtectScript(t, false, true); !got {
		t.Errorf("script suspended: got false, want true (Overlay=false should set protect=true → P_DELAY suspends)")
	}
}

// TestHandleIfButton_NilRootSetsProtectTrue: rootLayer not registered
// (root lookup returns nil) → protect=true → P_DELAY suspends.
func TestHandleIfButton_NilRootSetsProtectTrue(t *testing.T) {
	if got := runIfButtonProtectScript(t, false, false); !got {
		t.Errorf("script suspended: got false, want true (nil root should set protect=true → P_DELAY suspends)")
	}
}

// idkPayload builds a 13-byte IDK_SAVEDESIGN payload.
func idkPayload(gender byte, idkit [7]byte, color [5]byte) []byte {
	p := make([]byte, 13)
	p[0] = gender
	for i, v := range idkit {
		p[1+i] = v
	}
	for i, v := range color {
		p[8+i] = v
	}
	return p
}

// TestHandleIdkSaveDesignAllowDesignFalse pins that the packet is dropped
// when allowDesign is false.
func TestHandleIdkSaveDesignAllowDesignFalse(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = false

	_ = s.handleIdkSaveDesign(p, idkPayload(0, [7]byte{0, 1, 2, 3, 4, 5, 6}, [5]byte{0, 0, 0, 0, 0}))

	if p.gender != 0 || p.body != [7]int{0, 10, 18, 26, 33, 36, 42} {
		t.Error("player state changed despite allowDesign=false")
	}
}

// TestHandleIdkSaveDesignInvalidGender pins that gender > 1 is rejected.
func TestHandleIdkSaveDesignInvalidGender(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true

	_ = s.handleIdkSaveDesign(p, idkPayload(2, [7]byte{}, [5]byte{}))

	if p.gender != 0 {
		t.Errorf("gender changed: got %d, want 0", p.gender)
	}
}

// TestHandleIdkSaveDesignColorOutOfBounds pins that a color value >=
// designBodyColorCount[i] is rejected.
func TestHandleIdkSaveDesignColorOutOfBounds(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true

	// color[0] max is 11 (count=12); send 12 → out of bounds.
	// s.idkTypes is nil so the idk loop is skipped; color check still fires.
	_ = s.handleIdkSaveDesign(p, idkPayload(0, [7]byte{}, [5]byte{12, 0, 0, 0, 0}))

	if p.gender != 0 {
		t.Errorf("state changed despite invalid color: gender=%d", p.gender)
	}
}

// buildIdkTypes returns an IdkTypeConfigs with count entries.
// Entry i has Type=i and Disable=false.
// Covers male slots 0..6 (types 0..6) and female slots 0..6 (types 7..13)
// when count=14.
func buildIdkTypes(count int) *objtype.IdkTypeConfigs {
	configs := make([]*objtype.IdkType, count)
	for i := range count {
		c := objtype.NewIdkType(i)
		c.Type = i
		configs[i] = c
	}
	return &objtype.IdkTypeConfigs{
		ConfigNames: map[string]int{},
		Configs:     configs,
	}
}

// TestHandleIdkSaveDesignSuccess pins the happy path: valid inputs update
// gender/body/colors and flag MaskAppearance.
func TestHandleIdkSaveDesignSuccess(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	p.appearanceInv = 0

	// Seed a minimal registry: 14 entries, entry[i].Type = i.
	s.idkTypes = buildIdkTypes(14)

	// gender=1 (female): expected types per slot = i+7 ∈ [7..13].
	// idkit values must match registry IDs whose Type == i+7.
	body := [7]byte{7, 8, 9, 10, 11, 12, 13}
	colors := [5]byte{0, 1, 2, 0, 0}
	_ = s.handleIdkSaveDesign(p, idkPayload(1, body, colors))

	if p.gender != 1 {
		t.Errorf("gender: got %d, want 1", p.gender)
	}
	for i, v := range body {
		if p.body[i] != int(v) {
			t.Errorf("body[%d]: got %d, want %d", i, p.body[i], v)
		}
	}
	for i, v := range colors {
		if p.colors[i] != int(v) {
			t.Errorf("colors[%d]: got %d, want %d", i, p.colors[i], v)
		}
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Error("MaskAppearance: want set, got unset")
	}
}

// TestHandleIdkSaveDesignFemaleJaw255Accepted pins that wire value 255 is
// decoded to -1 AND accepted for the female jaw slot (gender=1, i=1, type=8).
// With idk validation active, idkit=-1 is only allowed at this slot.
func TestHandleIdkSaveDesignFemaleJaw255Accepted(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true

	// Seed registry: 14 entries (IDs 0..13, Type=0..13).
	s.idkTypes = buildIdkTypes(14)

	// gender=1, idkit[1]=255 → -1 (female jaw exception, type=8 skipped).
	// Other slots use valid female IDs (type = i+7).
	body := [7]byte{7, 255, 9, 10, 11, 12, 13} // slot 1 = 255 → -1
	colors := [5]byte{0, 0, 0, 0, 0}
	_ = s.handleIdkSaveDesign(p, idkPayload(1, body, colors))

	if p.body[1] != -1 {
		t.Errorf("body[1]: got %d, want -1 (female jaw 255→-1)", p.body[1])
	}
	if p.gender != 1 {
		t.Errorf("gender: got %d, want 1", p.gender)
	}
}

// TestHandleIdkSaveDesignValidMale pins the male happy path with a seeded registry.
func TestHandleIdkSaveDesignValidMale(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14)

	// gender=0: types 0..6, use registry IDs 0..6.
	body := [7]byte{0, 1, 2, 3, 4, 5, 6}
	colors := [5]byte{0, 1, 2, 0, 0}
	_ = s.handleIdkSaveDesign(p, idkPayload(0, body, colors))

	if p.gender != 0 {
		t.Errorf("gender: got %d, want 0", p.gender)
	}
	for i, v := range body {
		if p.body[i] != int(v) {
			t.Errorf("body[%d]: got %d, want %d", i, p.body[i], v)
		}
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Error("MaskAppearance: want set, got unset")
	}
}

// TestHandleIdkSaveDesignValidFemaleWithJaw pins the female happy path
// where all 7 slots including jaw are valid IDs.
func TestHandleIdkSaveDesignValidFemaleWithJaw(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14)

	// gender=1: types 7..13, use registry IDs 7..13.
	body := [7]byte{7, 8, 9, 10, 11, 12, 13}
	colors := [5]byte{0, 0, 0, 0, 0}
	_ = s.handleIdkSaveDesign(p, idkPayload(1, body, colors))

	if p.gender != 1 {
		t.Errorf("gender: got %d, want 1", p.gender)
	}
	for i, v := range body {
		if p.body[i] != int(v) {
			t.Errorf("body[%d]: got %d, want %d", i, p.body[i], v)
		}
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Error("MaskAppearance: want set, got unset")
	}
}

// TestHandleIdkSaveDesignDisabledIdk pins that a disabled IdkType rejects
// the whole packet (TS IdkSaveDesignHandler.ts:30: idk.disable check).
func TestHandleIdkSaveDesignDisabledIdk(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14)
	s.idkTypes.Configs[0].Disable = true // slot 0 disabled

	// gender=0, idkit[0]=0 → disabled → rejected.
	body := [7]byte{0, 1, 2, 3, 4, 5, 6}
	_ = s.handleIdkSaveDesign(p, idkPayload(0, body, [5]byte{}))

	if p.gender != 0 || p.masks&rsbuf.MaskAppearance != 0 {
		t.Error("state changed despite disabled idk: expected rejection")
	}
}

// TestHandleIdkSaveDesignWrongType pins that a type mismatch rejects the packet
// (TS IdkSaveDesignHandler.ts:30: idk.type != type check).
func TestHandleIdkSaveDesignWrongType(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14)
	s.idkTypes.Configs[0].Type = 99 // wrong type for slot 0 (expected 0)

	body := [7]byte{0, 1, 2, 3, 4, 5, 6}
	_ = s.handleIdkSaveDesign(p, idkPayload(0, body, [5]byte{}))

	if p.gender != 0 || p.masks&rsbuf.MaskAppearance != 0 {
		t.Error("state changed despite wrong idk type: expected rejection")
	}
}

// TestHandleIdkSaveDesignOutOfRangeIdkit pins that an idkit value >= registry
// length is rejected (bounds check before Configs[idkit[i]] dereference).
func TestHandleIdkSaveDesignOutOfRangeIdkit(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.allowDesign = true
	s.idkTypes = buildIdkTypes(14) // IDs 0..13

	// idkit[0] = 14 → out of range (len=14).
	body := [7]byte{14, 1, 2, 3, 4, 5, 6}
	_ = s.handleIdkSaveDesign(p, idkPayload(0, body, [5]byte{}))

	if p.gender != 0 || p.masks&rsbuf.MaskAppearance != 0 {
		t.Error("state changed despite out-of-range idkit: expected rejection")
	}
}
