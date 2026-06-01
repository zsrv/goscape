package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// These tests pin the protected-access guard ported from TS
// Player.runScript (Player.ts:2094):
//
//	if (!force && protect && (this.protect || this.delayed)) return -1;
//
// A fresh protected script must NOT acquire protected access while the
// player already holds it (a protected script suspended on a dialogue)
// or is delayed. Resumes bypass runScript (they call resumeOrFinish
// directly, the force=true path) and are therefore unaffected.
//
// Without the guard, an opheld/opheldu/opheldt/if_button/inv_button
// fired mid-dialogue executes, which combined with the universal
// "consume-after-yield" content pattern (inv_total check -> chat yield
// -> inv_del/inv_add, inv_del return ignored) enables item/coin dupes:
// drop the input during the dialogue, the post-resume inv_del removes
// nothing, the reward is still granted.

// suspendingScript returns a ScriptFile that, if executed, suspends on
// P_PAUSEBUTTON — an observable side effect (it gets stored as the
// player's activeScript via resumeOrFinish). If the guard blocks the
// run, the script never executes and activeScript is left untouched.
func suspendingScript(name string, trigger script.ServerTriggerType, subject int) *script.ScriptFile {
	return &script.ScriptFile{
		Name:             name,
		LookupKey:        script.LookupKeyForType(trigger, subject),
		Opcodes:          []script.Opcode{script.OpPPauseButton, script.OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
}

// TestRunScript_Blocked_WhenProtectedScriptSuspended pins the core dupe
// fix: an opheld DROP fired while a PROTECTED script is suspended on a
// chat dialogue must be rejected (the drop never runs).
func TestRunScript_Blocked_WhenProtectedScriptSuspended(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = []string{"op1", "", "", "", "drop"} // op5 valid

	// A protected script suspended on a chat dialogue (mid-conversation).
	// NAI-111-D1: in production the StoreActiveScript dispatch path
	// re-preserves p.protect=true (resumeOrFinish Suspended/PauseButton/
	// CountDialog arm in modules/world/script.go) when state.Pointers&PAP
	// is set; this fixture plants the activeScript directly, so we set
	// p.protect explicitly to simulate the post-suspend state TS would
	// have via Player.ts:2141 preserve.
	chatState := script.Init(suspendingScript("[opnpc1,chatter]", script.TriggerOpNpc1, 1), p, true /*protect*/, nil, nil)
	chatState.Execution = script.PauseButton
	p.StoreActiveScript(chatState)
	p.protect = true
	p.modalState = modalStateChat
	if !p.protectedScriptActive() {
		t.Fatal("setup invalid: protected script should be active")
	}

	s.scriptProvider.Register(suspendingScript("[opheld5,555]", script.TriggerOpHeld5, 555))

	if err := handleOpHeld5(p, opHeldPayload(555, 3, 149)); err != nil {
		t.Fatalf("handleOpHeld5: %v", err)
	}

	if p.activeScript != chatState {
		t.Errorf("opheld DROP executed while a protected script was suspended; "+
			"activeScript = %p, want the suspended chat script %p", p.activeScript, chatState)
	}
}

// TestRunScript_Blocked_WhenDelayed pins the delayed arm of the guard at
// the runScript chokepoint: a fresh protected script cannot run while the
// player is delayed.
func TestRunScript_Blocked_WhenDelayed(t *testing.T) {
	s, p := setupOpHeldServer(t)
	p.delayed = true

	s.runScript(suspendingScript("[opheld5,555]", script.TriggerOpHeld5, 555),
		p, nil, script.TriggerOpHeld5, true /*protect*/, nil, nil)

	if p.activeScript != nil {
		t.Errorf("protected script ran while delayed; activeScript = %p, want nil", p.activeScript)
	}
}

// TestRunScript_Runs_WhenUnprotectedAndNotDelayed is the control: with no
// protected script active and not delayed, the guard must NOT over-block
// — a fresh protected opheld runs normally.
func TestRunScript_Runs_WhenUnprotectedAndNotDelayed(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = []string{"op1", "", "", "", "drop"}
	if p.protectedScriptActive() || p.delayed {
		t.Fatal("setup invalid: player must be unprotected and not delayed")
	}

	s.scriptProvider.Register(suspendingScript("[opheld5,555]", script.TriggerOpHeld5, 555))

	if err := handleOpHeld5(p, opHeldPayload(555, 3, 149)); err != nil {
		t.Fatalf("handleOpHeld5: %v", err)
	}

	if p.activeScript == nil {
		t.Error("opheld DROP did not run for an unprotected, non-delayed player; guard over-blocked")
	}
}
