package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/script"
)

// TestCanAccess_AfterWorldSuspendedClear_ReturnsTrue pins Fix B regression
// baseline: with activeScript nil (the post-Fix-B post-condition after
// WorldSuspended dispatch), protectedScriptActive() returns false and
// CanAccess returns true (assuming no delay and no modal).
// Mirrors TS canAccess (Player.ts:805-812) which checks this.protect
// (boolean), not this.activeScript.
func TestCanAccess_AfterWorldSuspendedClear_ReturnsTrue(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.activeScript = nil
	p.delayed = false
	p.modalState = modalStateNone

	if !p.CanAccess() {
		t.Fatalf("CanAccess after Fix B: got false, want true (activeScript nil → protectedScriptActive false)")
	}
}

// TestProtectedScriptActive_NilActiveScript_ReturnsFalse pins the
// regression guard: protectedScriptActive must return false when
// activeScript is nil, irrespective of any prior PAP-flagged state.
func TestProtectedScriptActive_NilActiveScript_ReturnsFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.activeScript = nil
	if p.protectedScriptActive() {
		t.Fatalf("protectedScriptActive with nil activeScript: got true, want false")
	}
}

// TestResumeOrFinish_WorldSuspended_ClearsActiveScriptUnblocksCanAccess
// is the end-to-end pin for Fix B: drive a PAP-flagged player-bound
// script through resumeOrFinish to the WorldSuspended dispatch arm,
// then verify CanAccess is true (the bug-shape was CanAccess=false here).
//
// Pre-fix: resumeOrFinish's WorldSuspended arm did NOT clear activeScript
// (NAI-44 deliberate retention). With PtrProtectedActivePlayer set,
// protectedScriptActive() returned true → CanAccess returned false.
// Post-fix: ClearActiveScript() is called before enqueue; activeScript
// is nil; CanAccess returns true.
func TestResumeOrFinish_WorldSuspended_ClearsActiveScriptUnblocksCanAccess(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Script: push 5, world_delay, return.
	// WORLD_DELAY sets Execution=WorldSuspended; the wakeup-tick (5)
	// stays on the int stack for resumeOrFinish to consume (PopInt).
	sf := &script.ScriptFile{
		Name: "[worlddelay,nai155t3]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpWorldDelay,
			script.OpReturn,
		},
		IntOperands:      []int32{5, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	// Init with protect=true (PtrProtectedActivePlayer set on state.Pointers).
	// This is the firelighting-path shape that produced the bug.
	state := script.Init(sf, p, true, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()

	// Pre-condition: wire activeScript so the assertion is meaningful.
	p.activeScript = state
	p.delayed = false
	p.modalState = modalStateNone

	s.resumeOrFinish(state, p)

	// Primary assertion: CanAccess must be true post-WorldSuspended.
	// Pre-fix this was false (activeScript non-nil + PAP → CanAccess blocked).
	if !p.CanAccess() {
		t.Fatalf("CanAccess after WorldSuspended dispatch: got false, want true (Fix B clears activeScript)")
	}
	// Structural pin: activeScript must be nil.
	if p.activeScript != nil {
		t.Errorf("p.activeScript after WorldSuspended: got %p, want nil (Fix B must clear)", p.activeScript)
	}
	// Enqueue still happens.
	if got, want := len(s.worldScriptQueue), 1; got != want {
		t.Errorf("worldScriptQueue length: got %d, want %d (state must still be enqueued)", got, want)
	}
}
