package world

import (
	"testing"

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
		Name:      "[tutorial]",
		LookupKey: script.LookupKeyForGlobal(script.TriggerTutorial),
		Opcodes:   []script.Opcode{script.OpReturn},
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
