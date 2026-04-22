package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

func newNpcForScriptTest(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
	}
	return NewNpc(1, 0, 3094, 3106, 0, typ)
}

func TestNpcStoreAndClearActiveScript(t *testing.T) {
	n := newNpcForScriptTest(t)
	state := &script.ScriptState{}

	n.StoreActiveScript(state)
	if n.activeScript != state {
		t.Errorf("StoreActiveScript: got %v, want %v", n.activeScript, state)
	}

	n.ClearActiveScript()
	if n.activeScript != nil {
		t.Errorf("ClearActiveScript: got %v, want nil", n.activeScript)
	}
}

func TestNpcSetDelayed(t *testing.T) {
	n := newNpcForScriptTest(t)
	s := &Server{}
	s.currentTick = 100
	n.server = s

	n.SetDelayed(5)

	if !n.delayed {
		t.Errorf("delayed: got false, want true")
	}
	want := 100 + 1 + 5
	if n.delayedUntil != want {
		t.Errorf("delayedUntil: got %d, want %d", n.delayedUntil, want)
	}
}

// newServerForScriptTest builds a minimal *Server wired for running
// NPC-anchored scripts. Reuses the pattern from script_test.go:939-952.
func newServerForScriptTest(t *testing.T) *Server {
	t.Helper()
	return &Server{
		log: discardLogger(),
	}
}

func TestRunNpcScriptFiresAndFinishes(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:    "trivial_return",
		Opcodes: []script.Opcode{script.OpReturn},
	}

	s.runNpcScript(sf, n, nil, nil)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (script finished)", n.activeScript)
	}
	if n.delayed {
		t.Errorf("delayed: got true, want false")
	}
}

func TestRunNpcScriptSuspendsOnNpcDelay(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int32{3},
	}

	s.runNpcScript(sf, n, nil, nil)

	if n.activeScript == nil {
		t.Fatalf("activeScript: got nil, want stored state")
	}
	if n.activeScript.Execution != script.NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", n.activeScript.Execution)
	}
	if !n.delayed {
		t.Errorf("delayed: got false, want true")
	}
	want := 100 + 1 + 3
	if n.delayedUntil != want {
		t.Errorf("delayedUntil: got %d, want %d", n.delayedUntil, want)
	}
}

func TestNpcTurnResumesSuspendedScriptAfterDelay(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int32{3},
	}

	// Suspend: after this, delayedUntil = 104.
	s.runNpcScript(sf, n, nil, nil)
	if n.activeScript == nil || !n.delayed {
		t.Fatalf("setup: expected suspended state")
	}

	// Advance to delayedUntil and call turn.
	s.currentTick = 104
	n.turn(s)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (resumed and finished)", n.activeScript)
	}
	if n.delayed {
		t.Errorf("delayed: got true, want false (delay expired)")
	}
}

func TestNpcTurnDoesNotResumeWhileDelayed(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int32{3},
	}

	// Suspend: delayedUntil = 104.
	s.runNpcScript(sf, n, nil, nil)

	// Advance to one tick BEFORE delayedUntil.
	s.currentTick = 103
	n.turn(s)

	if n.activeScript == nil {
		t.Errorf("activeScript: got nil, want still-suspended state")
	}
	if n.activeScript != nil && n.activeScript.Execution != script.NpcSuspended {
		t.Errorf("Execution: got %v, want still NpcSuspended", n.activeScript.Execution)
	}
	if !n.delayed {
		t.Errorf("delayed: got false, want true (still within delay window)")
	}
}

func TestNpcEnqueueScriptForTrigger(t *testing.T) {
	n := newNpcForScriptTest(t)

	n.EnqueueScriptForTrigger(script.TriggerAiQueue3, 5, 42)

	if len(n.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1", len(n.queue))
	}
	req := n.queue[0]
	if req.Trigger != script.TriggerAiQueue3 {
		t.Errorf("Trigger: got %v, want TriggerAiQueue3", req.Trigger)
	}
	if req.Delay != 5 {
		t.Errorf("Delay: got %d, want 5", req.Delay)
	}
	if req.IntArg != 42 {
		t.Errorf("IntArg: got %d, want 42", req.IntArg)
	}
}

func TestNpcTurnDeadNpcDoesNotResumeScript(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int32{3},
	}

	// Suspend the script: delayedUntil = 104.
	s.runNpcScript(sf, n, nil, nil)
	if n.activeScript == nil || !n.delayed {
		t.Fatalf("setup: expected suspended state")
	}

	// NPC dies before the delay expires.
	n.dead = true

	// Advance past delayedUntil and call turn.
	s.currentTick = 105
	n.turn(s)

	// Script must NOT have resumed — it stays suspended on the dead
	// NPC per TS Npc.ts:112 isActive guard.
	if n.activeScript == nil {
		t.Errorf("activeScript: got nil, want stored (dead NPC should not resume)")
	}
	if n.activeScript != nil && n.activeScript.Execution != script.NpcSuspended {
		t.Errorf("Execution: got %v, want still NpcSuspended", n.activeScript.Execution)
	}
}

// buildNpcForIntegration builds an NPC wired to a server, with typ
// set so processNpcQueue can read n.typ.Category.
func buildNpcForIntegration(t *testing.T) (*Server, *Npc) {
	t.Helper()
	s := newServerForScriptTest(t)
	s.currentTick = 100
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Category:   -1,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)
	n.server = s
	return s, n
}

// TestNpcTurnFiresQueuedEntryWhenDelayZero — enqueue at delay=1,
// advance one tick. TS Npc.ts:544-549: decrement THEN fire if delay<=0.
// So at delay=1, the same tick that decrements also fires. Queue empty.
func TestNpcTurnFiresQueuedEntryWhenDelayZero(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 1, 0)
	if len(n.queue) != 1 {
		t.Fatalf("setup: queue should have 1 entry, got %d", len(n.queue))
	}

	n.turn(s)

	if len(n.queue) != 0 {
		t.Fatalf("after first turn: queue should be empty (delay went 1→0 and fired), got %d", len(n.queue))
	}
}

// TestNpcTurnDoesNotDecrementQueueWhileDelayed — NPC delayed; queue
// delay must not decrement. Matches TS Npc.ts:544-547.
func TestNpcTurnDoesNotDecrementQueueWhileDelayed(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 3, 0)
	n.delayed = true
	n.delayedUntil = s.currentTick + 100 // far future

	n.turn(s)
	n.turn(s)
	n.turn(s)

	if len(n.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1 (no fire while delayed)", len(n.queue))
	}
	if n.queue[0].Delay != 3 {
		t.Errorf("queue[0].Delay: got %d, want 3 (no decrement while delayed)", n.queue[0].Delay)
	}
}

// TestNpcTurnReentryQueueAppendDuringIteration — multiple ready
// entries (delay=0) fire in one processNpcQueue pass.
// Weaker form of the "speedup quirk" test — doesn't prove mid-fire
// append, only multi-entry same-pass drain.
func TestNpcTurnReentryQueueAppendDuringIteration(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// Two entries, both ready (delay=0). The iteration should
	// process both in one turn() call.
	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 0, 0)
	n.EnqueueScriptForTrigger(script.TriggerAiQueue2, 0, 0)

	n.turn(s)

	if len(n.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (both entries should fire in one pass)", len(n.queue))
	}
}

// TestResumeOrFinishNpcErrorPathClearsScript — NAI-2 follow-up.
// When script.Execute returns an error, resumeOrFinishNpc must
// clear n.activeScript (matching the player-side resumeOrFinish
// error-path at modules/world/script.go:31-35).
func TestResumeOrFinishNpcErrorPathClearsScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// Pre-store a dummy script to prove it gets cleared.
	n.activeScript = &script.ScriptState{}

	// Build a state whose Execute will error. Opcode 0xFFFF has no
	// registered handler; Execute returns "no handler for ..." error.
	sf := &script.ScriptFile{
		Name:    "err_script",
		Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
	}
	errState := script.Init(sf, nil, false, nil, nil)
	errState.ActiveNpc = n

	s.resumeOrFinishNpc(errState, n)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (cleared on Execute error)", n.activeScript)
	}
}

// TestResumeOrFinishNpcDefaultBranchClearsScript — NAI-2 follow-up.
// Synthetic: pre-set Execution to a value that hits the default:
// branch (not Finished/Aborted/NpcSuspended). Execute's hot loop
// exits immediately when Execution != Running, so the pre-set value
// survives untouched.
//
// This path is unreachable from authentic content (all non-
// NpcSuspended non-terminal Execution values require an ActivePlayer,
// and runNpcScript passes nil Self), but the test proves the
// defensive clear fires if future code accidentally drives an NPC-
// anchored script into one of these states.
func TestResumeOrFinishNpcDefaultBranchClearsScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	n.activeScript = &script.ScriptState{}

	sf := &script.ScriptFile{
		Name:    "default_branch_script",
		Opcodes: []script.Opcode{script.OpReturn},
	}
	state := script.Init(sf, nil, false, nil, nil)
	state.ActiveNpc = n
	state.Execution = script.CountDialog // synthetic non-Running, non-terminal state

	s.resumeOrFinishNpc(state, n)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (cleared on default branch)", n.activeScript)
	}
}

func TestNpcSetTimer(t *testing.T) {
	n := newNpcForScriptTest(t)

	n.SetTimer(5)
	if n.timerInterval != 5 {
		t.Errorf("timerInterval after SetTimer(5): got %d, want 5", n.timerInterval)
	}

	// -1 is a TS-faithful no-op: must leave timerInterval at 5.
	n.SetTimer(-1)
	if n.timerInterval != 5 {
		t.Errorf("timerInterval after SetTimer(-1): got %d, want 5 (no-op expected)", n.timerInterval)
	}
}

func TestNewNpcSeedsTimerIntervalFromType(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Timer:      7,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	if n.timerInterval != 7 {
		t.Errorf("timerInterval from NewNpc: got %d, want 7 (seeded from typ.Timer)", n.timerInterval)
	}
}
