package world

import (
	"io"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/script"
)

// TestResumeCountDialogSetsLastIntAndResumes pins the happy path for
// opcode 237 (RESUME_P_COUNTDIALOG): a P_COUNTDIALOG-suspended script
// receives the i32 count via state.LastInt, Execution flips to Running,
// and resumeOrFinish drives the script to completion. Mirrors TS
// ResumePCountDialogHandler.ts semantics.
func TestResumeCountDialogSetsLastIntAndResumes(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Script: push "before", mes, p_countdialog, push "after", mes, return.
	// p_countdialog suspends; RESUME_P_COUNTDIALOG flips to Running and
	// the script falls through to "after".
	sf := &script.ScriptFile{
		Name: "[countdialog,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpPCountDialog,
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0, 0, 0, 0},
		StringOperands:   []string{"before", "", "", "after", "", ""},
		InstructionCount: 6,
	}

	received := drainConn(t, cc)
	s.runScript(sf, p, nil, script.TriggerProc, true, nil, nil)
	p.client.flushWrite()
	<-received

	if p.activeScript == nil {
		t.Fatal("setup: expected activeScript to be set after p_countdialog")
	}
	if p.activeScript.Execution != script.CountDialog {
		t.Fatalf("setup: Execution: got %v, want CountDialog", p.activeScript.Execution)
	}

	// Send RESUME_P_COUNTDIALOG with count=42 as big-endian i32.
	received2 := drainConn(t, cc)
	buf := packet.NewPacket([]byte{0x00, 0x00, 0x00, 0x2A}) // 42
	if err := s.handleResumeCountDialog(p, buf); err != nil {
		t.Fatalf("resume: %v", err)
	}
	p.client.flushWrite()
	second := <-received2

	// Post-resume wire payload bytes 2..6 = "after".
	if string(second[2:7]) != "after" {
		t.Errorf("post-resume payload: got %q, want \"after\"", second[2:])
	}
	if p.activeScript != nil {
		t.Errorf("activeScript after resume-and-finish: got %v, want nil", p.activeScript)
	}
}

// TestResumeCountDialogSignExtendsNegative pins that a wire i32 with the
// sign bit set is sign-extended to int when stored in state.LastInt
// (count := int32(buf.G4()) then state.LastInt = int(count)).
func TestResumeCountDialogSignExtendsNegative(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Script: p_countdialog, last_int, return — leaves LastInt on the
	// int stack so we can inspect what the handler stored.
	sf := &script.ScriptFile{
		Name: "[countdialog,negint]",
		Opcodes: []script.Opcode{
			script.OpPCountDialog,
			script.OpLastInt,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	received := drainConn(t, cc)
	s.runScript(sf, p, nil, script.TriggerProc, true, nil, nil)
	p.client.flushWrite()
	<-received

	if p.activeScript == nil || p.activeScript.Execution != script.CountDialog {
		t.Fatalf("setup: expected CountDialog state, got activeScript=%v", p.activeScript)
	}

	// We need to peek at LastInt AFTER handleResumeCountDialog writes it
	// but BEFORE resumeOrFinish clears activeScript. Track the value
	// indirectly: send -1 (0xFFFFFFFF) and confirm activeScript is
	// cleared (script ran to completion, which requires LAST_INT to
	// have produced a valid int — but easier check is the actual
	// stored value before Execute fires). Use a shim: capture state
	// pointer first then re-check after.
	state := p.activeScript

	buf := packet.NewPacket([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // -1 as int32
	if err := s.handleResumeCountDialog(p, buf); err != nil {
		t.Fatalf("resume: %v", err)
	}

	// state.LastInt was set by the handler before resumeOrFinish drove
	// the script to completion (which leaves state alive but unreferenced
	// from p.activeScript). Confirm the sign-extension.
	if state.LastInt != -1 {
		t.Errorf("LastInt: got %d, want -1 (sign-extended from 0xFFFFFFFF)", state.LastInt)
	}
}

// TestResumeCountDialogNilActiveScriptNoop pins that a RESUME_P_COUNTDIALOG
// packet arriving with no active script is a clean no-op (no panic, no
// state mutation). Mirrors the activeScript == nil early-return at
// resume_dialog.go:36.
func TestResumeCountDialogNilActiveScriptNoop(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.activeScript = nil

	buf := packet.NewPacket([]byte{0x00, 0x00, 0x00, 0x0A}) // count=10
	if err := s.handleResumeCountDialog(p, buf); err != nil {
		t.Fatalf("handler returned err: %v", err)
	}
	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil")
	}
}

// TestResumeCountDialogWrongExecutionStateNoop pins that a
// RESUME_P_COUNTDIALOG packet arriving while the active script is in a
// non-CountDialog state (e.g. PauseButton) is a clean no-op: LastInt is
// NOT overwritten, Execution is NOT flipped, and resumeOrFinish is NOT
// called. Mirrors the Execution != CountDialog early-return at
// resume_dialog.go:36.
func TestResumeCountDialogWrongExecutionStateNoop(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.activeScript = &script.ScriptState{
		Script:    &script.ScriptFile{Name: "wrongstate"},
		Execution: script.PauseButton,
		LastInt:   777, // sentinel — must NOT be overwritten
	}

	buf := packet.NewPacket([]byte{0x00, 0x00, 0x00, 0x0A}) // count=10
	if err := s.handleResumeCountDialog(p, buf); err != nil {
		t.Fatalf("handler returned err: %v", err)
	}
	if p.activeScript == nil {
		t.Fatal("activeScript: got nil, want preserved (PauseButton state must not be cleared)")
	}
	if p.activeScript.Execution != script.PauseButton {
		t.Errorf("Execution: got %v, want PauseButton (state must not flip)", p.activeScript.Execution)
	}
	if p.activeScript.LastInt != 777 {
		t.Errorf("LastInt: got %d, want 777 (sentinel must not be overwritten)", p.activeScript.LastInt)
	}
}

// TestResumeCountDialogShortPayloadPanicsEOF pins the G4 contract: a
// payload shorter than 4 bytes causes G4 to panic with io.EOF (see
// pkg/io/packet/packet.go:193-200). The handler does not pre-validate
// length; it relies on G4's panic. This documents the actual contract
// rather than a defensive error-return.
func TestResumeCountDialogShortPayloadPanicsEOF(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.activeScript = &script.ScriptState{
		Script:    &script.ScriptFile{Name: "shortpayload"},
		Execution: script.CountDialog,
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected G4 to panic on short payload, got no panic")
		}
		if r != io.EOF {
			t.Errorf("panic value: got %v, want io.EOF", r)
		}
	}()

	// Only 3 bytes — G4 needs 4.
	buf := packet.NewPacket([]byte{0x00, 0x00, 0x00})
	_ = s.handleResumeCountDialog(p, buf)
}
