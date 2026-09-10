package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// Content that keeps an interaction alive across ticks does so by calling
// p_oploc/p_opobj from inside the OP trigger it is already running under —
// woodcutting is the canonical case:
//
//	[oploc1,_tree] @attempt_cut_tree   // ... p_oploc(1)
//	[oploc3,_tree] @cut_tree           // ... p_oploc(3)  <- every tick
//
// p_oploc routes through StopAction() → ClearInteraction(), which resets
// targetSubject.x/z/level to -1, and then SetInteraction(). The engine-side
// click handlers (handler_oploc.go / handler_opobj.go) re-snapshot those
// three fields immediately afterwards; the script path had no equivalent, so
// the re-armed interaction carried (-1,-1,-1). fireOpTriggerLoc's next-tick
// locStillValid() then looked the loc up in zone (-1,-1,-1), missed, and
// cleared the interaction with no message to the player.

// TestPOpLocReArmPreservesTargetSubjectCoords pins the subject snapshot
// across a script-initiated re-arm.
func TestPOpLocReArmPreservesTargetSubjectCoords(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	s.scriptProvider.Register(buildPOpLocScript(script.TriggerOpLoc1, loc.Type(), 3))

	fireOpTriggerLoc(p, s, loc) // [oploc1] body is p_oploc(3)

	if p.targetSubject.x != loc.X || p.targetSubject.z != loc.Z || p.targetSubject.level != loc.Level {
		t.Errorf("targetSubject after p_oploc re-arm: got (x=%d z=%d level=%d), want (x=%d z=%d level=%d)",
			p.targetSubject.x, p.targetSubject.z, p.targetSubject.level, loc.X, loc.Z, loc.Level)
	}
	if !locStillValid(s, loc, p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
		t.Error("locStillValid false after re-arm: the next tick would clear the interaction silently")
	}
}

// TestScriptReArmedLocInteractionSurvivesAcrossTicks drives the real
// pre/post-move passes and pins that a self-re-arming loc interaction keeps
// running, which is what makes continuous woodcutting work.
func TestScriptReArmedLocInteractionSurvivesAcrossTicks(t *testing.T) {
	s, p, loc, _ := makeOpLocTriggerFixture(t)
	s.scriptProvider.Register(buildPOpLocScript(script.TriggerOpLoc1, loc.Type(), 3))
	s.scriptProvider.Register(buildPOpLocScript(script.TriggerOpLoc3, loc.Type(), 3))

	const ticks = 8
	for i := 1; i <= ticks; i++ {
		p.interacted = false // production: ResetMasks (TS PathingEntity.ts:587)
		p.stepsTaken = 0

		p.processInteraction()

		if p.target == nil {
			t.Fatalf("interaction cleared after %d tick(s); a self-re-arming loc op must persist", i)
		}
		if p.targetOp != 3 {
			t.Fatalf("tick %d: targetOp = %d, want 3 (re-armed by p_oploc(3))", i, p.targetOp)
		}
	}
}
