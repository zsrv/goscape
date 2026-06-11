package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// --- A9: resumeButtons lifecycle (TS Player.ts @2e3bcf43) -------------------
//
// IF_ADDRESUMEBUTTON (A1) appends to Player.resumeButtons; A9 completes the
// lifecycle: cleared in cleanup (Player.ts:454+456), in closeModal when the
// active script is COUNTDIALOG/PAUSEBUTTON-suspended (Player.ts:776-779), in
// every modal-open method (Player.ts:2012-2016 / :2048-2052 / :2072-2076 /
// :2098-2102 — pinned in TestOpenModalClearsSuspendedDialogAndResumeButtons),
// and in the executeScript Finished/Aborted tail (Player.ts:2224-2226).

// TestCloseModalClearsResumeButtonsOnSuspendedDialog pins TS Player.ts:775-779
// @2e3bcf43:
//
//	// close any input dialogue suspended scripts.
//	if (this.activeScript?.execution === ScriptState.COUNTDIALOG ||
//	    this.activeScript?.execution === ScriptState.PAUSEBUTTON) {
//	    this.activeScript = null;
//	    this.resumeButtons = [];
//	}
func TestCloseModalClearsResumeButtonsOnSuspendedDialog(t *testing.T) {
	for _, exec := range []script.Execution{script.CountDialog, script.PauseButton} {
		p, _ := newTestPlayer(t)
		p.modalState = modalStateChat
		p.modalChat = 50
		p.activeScript = &script.ScriptState{Execution: exec}
		p.resumeButtons = []int{7, 8}

		p.CloseModal(true)

		if p.activeScript != nil {
			t.Errorf("exec=%v: activeScript: got non-nil, want nil", exec)
		}
		if len(p.resumeButtons) != 0 {
			t.Errorf("exec=%v: resumeButtons: got %v, want empty (TS Player.ts:778)", exec, p.resumeButtons)
		}
	}
}

// TestCloseModalKeepsResumeButtonsWhenNoSuspendedDialog pins the guard: the
// clear lives INSIDE the COUNTDIALOG/PAUSEBUTTON branch — a Running (or nil)
// activeScript leaves resumeButtons alone (TS Player.ts:776 condition).
func TestCloseModalKeepsResumeButtonsWhenNoSuspendedDialog(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 50
	p.activeScript = &script.ScriptState{Execution: script.Running}
	p.resumeButtons = []int{7}

	p.CloseModal(true)

	if len(p.resumeButtons) != 1 {
		t.Errorf("resumeButtons: got %v, want [7] (clear only fires for suspended dialog states)", p.resumeButtons)
	}
}

// TestOnScriptFinishedOrAbortedClearsResumeButtons pins TS Player.ts:2224-2226
// @2e3bcf43 (executeScript Finished/Aborted tail):
//
//	} else if (script === this.activeScript) {
//	    this.activeScript = null;
//	    this.resumeButtons = [];
func TestOnScriptFinishedOrAbortedClearsResumeButtons(t *testing.T) {
	p, _ := newTestPlayer(t)
	st := &script.ScriptState{Execution: script.Finished}
	p.activeScript = st
	p.resumeButtons = []int{3}

	p.OnScriptFinishedOrAborted(st)

	if p.activeScript != nil {
		t.Error("activeScript: got non-nil, want nil")
	}
	if len(p.resumeButtons) != 0 {
		t.Errorf("resumeButtons: got %v, want empty (TS Player.ts:2226)", p.resumeButtons)
	}

	// Match-guard: a DIFFERENT state finishing must not clear.
	p.activeScript = &script.ScriptState{Execution: script.PauseButton}
	p.resumeButtons = []int{4}
	p.OnScriptFinishedOrAborted(&script.ScriptState{Execution: script.Finished})
	if len(p.resumeButtons) != 1 {
		t.Errorf("resumeButtons: got %v, want [4] (match-guard — TS `script === this.activeScript`)", p.resumeButtons)
	}
}

// TestRemovePlayerInternalClearsResumeButtons pins the TS Player.cleanup
// clear (Player.ts:450-456 @2e3bcf43, double-cleared at :454 `this.
// resumeButtons = []` and :456 `this.resumeButtons.length = 0` — the
// 2dc4a811 sync added the first without noticing the second).
// removePlayerInternal is goscape's cleanup analog.
func TestRemovePlayerInternalClearsResumeButtons(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	s.playersMu.Lock()
	s.players.set(1, p)
	s.playersMu.Unlock()
	p.resumeButtons = []int{9}

	s.removePlayerInternal(p)

	if len(p.resumeButtons) != 0 {
		t.Errorf("resumeButtons: got %v, want empty (TS Player.cleanup, Player.ts:454+456)", p.resumeButtons)
	}
}

// The restored IF_BUTTON NO_BUTTON gate (TS 7efd4827, IfButtonHandler.ts:16
// @2e3bcf43) is pinned in handler_interface_test.go
// TestHandleIfButton_NoButtonTypeRejectsAt254.
