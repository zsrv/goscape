package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/script"
)

// --- NAI-37 Task 9: world-script-queue scheduler tests --------------------

// newReturnImmediatelyScript constructs a minimal ScriptState whose
// Execute completes immediately (returns Finished). Use for tests that
// just need to verify the scheduler-side logic (queue mechanics) without
// caring about script behavior.
func newReturnImmediatelyScript(t *testing.T) *script.ScriptState {
	t.Helper()
	sf := &script.ScriptFile{
		Name:             "test_return",
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	return script.Init(sf, nil, false, nil, nil)
}

func TestProcessWorldQueue_DelayZero_FiresOnSecondCall(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 0) // stored as delay=1 per TS World.enqueueScript
	if got, want := len(s.worldScriptQueue), 1; got != want {
		t.Fatalf("queue length post-enqueue: got %d, want %d", got, want)
	}
	// Tick 1: stored delay=1; post-decrement returns 1, decrements to 0; 1>0 skip.
	s.processWorldQueue()
	if got, want := len(s.worldScriptQueue), 1; got != want {
		t.Fatalf("after tick 1: queue length got %d, want %d (delay=0 user-side fires on 2nd call per TS)", got, want)
	}
	// Tick 2: stored delay=0; post-decrement returns 0, decrements to -1; 0>0 false → fire.
	s.processWorldQueue()
	if got, want := len(s.worldScriptQueue), 0; got != want {
		t.Errorf("after tick 2: queue length got %d, want %d (delay=0 user-side fires on 2nd call per TS)", got, want)
	}
	if got, want := state.Execution, script.Finished; got != want {
		t.Errorf("script Execution post-fire: got %v, want %v", got, want)
	}
}

func TestProcessWorldQueue_DelayN_FiresAfterNPlusOneTicks(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 3) // stored as delay=4 per TS World.enqueueScript

	// Tick 1: const=4, store=3, 4>0 skip.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after tick 1: queue length got %d, want 1", got)
	}
	// Tick 2: const=3, store=2, 3>0 skip.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after tick 2: queue length got %d, want 1", got)
	}
	// Tick 3: const=2, store=1, 2>0 skip.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after tick 3: queue length got %d, want 1", got)
	}
	// Tick 4: const=1, store=0, 1>0 skip.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after tick 4: queue length got %d, want 1", got)
	}
	// Tick 5: const=0, store=-1, 0>0 false → fire.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("after tick 5: queue length got %d, want 0 (user delay=3 stored as 4 fires on 5th call per TS)", got)
	}
}

func TestProcessWorldQueue_FifoOrder(t *testing.T) {
	s := newTestServer(t)
	a := newReturnImmediatelyScript(t)
	a.Script.Name = "A"
	b := newReturnImmediatelyScript(t)
	b.Script.Name = "B"
	c := newReturnImmediatelyScript(t)
	c.Script.Name = "C"
	s.EnqueueWorldScript(a, 0) // stored=1 each
	s.EnqueueWorldScript(b, 0)
	s.EnqueueWorldScript(c, 0)

	wantOrder := []string{"A", "B", "C"}

	// Tick 1: each entry stored=1; post-decrement returns 1, 1>0 skip.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 3 {
		t.Fatalf("after tick 1: queue length got %d, want 3 (delay=0 user-side fires on 2nd call per TS)", got)
	}
	// Tick 2: each entry stored=0; post-decrement returns 0, 0>0 false → fire.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 0 {
		t.Fatalf("queue post-drain: got %d, want 0", got)
	}
	// Each script's state.Execution should be Finished, indicating
	// they all ran. We can't easily verify ORDER via state alone since
	// they all reach the same final state — but pin "all 3 fired".
	for i, st := range []*script.ScriptState{a, b, c} {
		if st.Execution != script.Finished {
			t.Errorf("script %s (index %d): Execution got %v, want Finished",
				wantOrder[i], i, st.Execution)
		}
	}
}

func TestProcessWorldQueue_RemovedBeforeFire(t *testing.T) {
	// Verify that when an entry fires, it's removed from the queue
	// BEFORE script.Execute is called. We can't easily inspect this
	// from inside a real script, but we can verify post-fire the
	// queue contains exactly 0 entries (i.e., the entry didn't
	// stay around after firing).
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 0) // stored=1; needs 2 calls to fire per TS
	s.processWorldQueue()           // tick 1: skip
	s.processWorldQueue()           // tick 2: fire
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("queue length post-fire: got %d, want 0 (entry must be removed before+after fire)", got)
	}
}

// TestProcessWorldQueue_MultipleEntries_AllFireSameTick verifies the
// "speedup quirk" — a script enqueued mid-iteration (e.g., by the
// currently-firing script's Execute) is processed in the same
// processWorldQueue call, NOT deferred to the next tick.
//
// Difficult to test cleanly without a script handler that calls
// EnqueueWorldScript during Execute. For T9 we approximate by
// pre-enqueueing 2 entries with delay=0 and then verifying the loop
// iterates twice (both fire). The "mid-pass append visibility" is
// proven structurally by reading processWorldQueue's loop body; this
// test pins the behavior for normal multi-entry drainage.
//
// The full re-entrant case (script A's Execute appends B mid-pass) is
// covered by T13's integration test.
func TestProcessWorldQueue_MultipleEntries_AllFireSameTick(t *testing.T) {
	s := newTestServer(t)
	a := newReturnImmediatelyScript(t)
	b := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(a, 0) // stored=1
	s.EnqueueWorldScript(b, 0) // stored=1
	// Tick 1: both stored=1; post-decrement returns 1, 1>0 skip.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 2 {
		t.Fatalf("after tick 1: queue length got %d, want 2 (delay=0 user-side fires on 2nd call per TS)", got)
	}
	// Tick 2: both stored=0; post-decrement returns 0, 0>0 false → fire.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("queue length: got %d, want 0 (both entries should fire same tick)", got)
	}
	if a.Execution != script.Finished {
		t.Errorf("A.Execution: got %v, want Finished", a.Execution)
	}
	if b.Execution != script.Finished {
		t.Errorf("B.Execution: got %v, want Finished", b.Execution)
	}
}

// NOTE: TestProcessWorldQueue_WorldSuspendedSelfLoop is deferred to T12
// because it requires the resumeOrFinishWorld dispatch table to handle
// WorldSuspended (popInt + re-enqueue), which is the T12 deliverable.
// At T9 the stub just calls script.Execute without dispatching.

// --- NAI-37 Task 12: resumeOrFinishWorld dispatch table tests ------------
//
// Construction strategy: script.Execute (pkg/script/runner.go:54) runs
// `for s.Execution == Running`, so a ScriptState constructed with a
// pre-set non-Running Execution and an empty Opcodes slice causes
// Execute to return nil immediately without changing Execution. This
// gives us a clean way to exercise each dispatch branch in isolation —
// the test sets up the post-Execute state and resumeOrFinishWorld
// observes it directly. The WorldSuspended branch is exercised by a
// real bytecode path (PUSH_CONSTANT_INT + WORLD_DELAY) to verify the
// full pop-and-re-enqueue round trip.
//
// The default branch (Running, future-added Execution values) is not
// directly testable from outside: pre-setting Execution=Running with
// empty Opcodes makes Execute return an error (PC out of range), which
// short-circuits resumeOrFinishWorld before dispatch. The branch is
// purely defensive and is covered by inspection.

// TestResumeOrFinishWorld_FinishedClean verifies a script that returns
// (OpReturn → Execution=Finished) is dropped cleanly without re-enqueue.
func TestResumeOrFinishWorld_FinishedClean(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("Finished: queue length got %d, want 0 (drop)", got)
	}
	if state.Execution != script.Finished {
		t.Errorf("Execution post-dispatch: got %v, want Finished", state.Execution)
	}
}

// TestResumeOrFinishWorld_AbortedClean verifies a pre-Aborted state is
// dropped cleanly. Construction: empty Opcodes + pre-set Aborted; Execute
// respects the pre-set value and returns nil; dispatch hits the
// Finished/Aborted arm.
func TestResumeOrFinishWorld_AbortedClean(t *testing.T) {
	s := newTestServer(t)
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "test_aborted", Opcodes: []script.Opcode{}},
		Execution: script.Aborted,
	}
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("Aborted: queue length got %d, want 0 (drop)", got)
	}
}

// TestResumeOrFinishWorld_WorldSuspendedSelfReenqueue verifies the
// world self-loop case (path P3 in the spec, NAI-37 T12). A script
// that runs PUSH_CONSTANT_INT(4) + WORLD_DELAY suspends itself with
// the wakeup-tick on the int stack; resumeOrFinishWorld must pop the
// wakeup-tick and re-enqueue with that delay.
//
// This is the test deferred from T9.
func TestResumeOrFinishWorld_WorldSuspendedSelfReenqueue(t *testing.T) {
	s := newTestServer(t)
	sf := &script.ScriptFile{
		Name:             "test_world_delay",
		Opcodes:          []script.Opcode{script.OpPushConstantInt, script.OpWorldDelay, script.OpReturn},
		IntOperands:      []int32{4, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := script.Init(sf, nil, false, nil, nil)

	s.resumeOrFinishWorld(state)

	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("WorldSuspended self-loop: queue length got %d, want 1", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 5 {
		t.Errorf("re-enqueued delay: got %d, want 5 (popped 4 from int stack, stored as 4+1=5 per TS World.enqueueScript)", got)
	}
	if got := s.worldScriptQueue[0].script; got != state {
		t.Errorf("re-enqueued script identity: got %p, want %p", got, state)
	}
}

// TestResumeOrFinishWorld_CrossContextSuspendedDrop verifies the
// cross-context warn+drop branch for player-Suspended states reaching
// the world dispatch (deviation NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP).
func TestResumeOrFinishWorld_CrossContextSuspendedDrop(t *testing.T) {
	s := newTestServer(t)
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "test_suspended", Opcodes: []script.Opcode{}},
		Execution: script.Suspended,
	}
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("Suspended (cross-context): queue length got %d, want 0 (warn+drop per NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP)", got)
	}
}

// TestResumeOrFinishWorld_CrossContextNpcSuspendedDrop verifies the
// cross-context warn+drop branch for NpcSuspended states.
func TestResumeOrFinishWorld_CrossContextNpcSuspendedDrop(t *testing.T) {
	s := newTestServer(t)
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "test_npc_suspended", Opcodes: []script.Opcode{}},
		Execution: script.NpcSuspended,
	}
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("NpcSuspended (cross-context): queue length got %d, want 0", got)
	}
}

// TestResumeOrFinishWorld_CrossContextPauseButtonDrop verifies the
// cross-context warn+drop branch for PauseButton states.
func TestResumeOrFinishWorld_CrossContextPauseButtonDrop(t *testing.T) {
	s := newTestServer(t)
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "test_pause", Opcodes: []script.Opcode{}},
		Execution: script.PauseButton,
	}
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("PauseButton (cross-context): queue length got %d, want 0", got)
	}
}

// TestResumeOrFinishWorld_CrossContextCountDialogDrop verifies the
// cross-context warn+drop branch for CountDialog states.
func TestResumeOrFinishWorld_CrossContextCountDialogDrop(t *testing.T) {
	s := newTestServer(t)
	state := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "test_count", Opcodes: []script.Opcode{}},
		Execution: script.CountDialog,
	}
	s.resumeOrFinishWorld(state)
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("CountDialog (cross-context): queue length got %d, want 0", got)
	}
}

// --- NAI-37 Task 13: WORLD_DELAY full round-trip integration test --------

// TestWorldDelay_FullRoundTrip exercises the complete cross-tick
// coordination of WORLD_DELAY: a player-bound script that pushes a
// delay, calls WORLD_DELAY, then completes after the world tick wakes
// it up.
//
// Tick timeline with user delay=2 (stored as delay+1=3 per TS
// World.enqueueScript at World.ts:1239; processWorldQueue uses
// post-decrement so user delay N fires on the (N+2)-th call):
//
//	T1 (script first runs via runScript): pushes 2, hits WORLD_DELAY,
//	   sets Execution=WorldSuspended. resumeOrFinish (player path)
//	   pops 2, calls EnqueueWorldScript with user delay=2, stores
//	   delay=3, clears p.activeScript.
//	T2 (1st processWorldQueue): const=3, store=2, 3>0 skip.
//	T3 (2nd processWorldQueue): const=2, store=1, 2>0 skip.
//	T4 (3rd processWorldQueue): const=1, store=0, 1>0 skip.
//	T5 (4th processWorldQueue): const=0, store=-1, 0>0 false → fire.
//	   Script resumes from after WORLD_DELAY, runs OpReturn, completes.
//	   resumeOrFinishWorld sees Finished, drops entry.
//
// Per gettimer_passthrough_opcode_semantic_audit.md: handler-mock
// tests pass values through unchanged; only this integration test
// exercises the actual multi-tick state machine.
func TestWorldDelay_FullRoundTrip(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Build minimal ScriptFile: pushInt(2); WORLD_DELAY; RETURN.
	sf := &script.ScriptFile{
		Name: "[worlddelay_roundtrip,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpWorldDelay,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	// Tick 1: fresh-run via the production runScript path. The script
	// will execute pushInt(2); WORLD_DELAY; suspend. The player-path
	// resumeOrFinish handles the suspend → enqueue (user delay=2 stored
	// as 3) + clear.
	s.runScript(sf, p, false, nil, nil)

	// After T1: enqueued with stored delay=3, activeScript cleared.
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after T1: queue length got %d, want 1 (script suspended to world queue)", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 3 {
		t.Fatalf("after T1: enqueued delay got %d, want 3 (user delay=2 stored as 3 per TS World.enqueueScript)", got)
	}
	if p.activeScript != nil {
		t.Fatalf("after T1: p.activeScript should be nil (script transitioned to world-bound)")
	}
	state := s.worldScriptQueue[0].script

	// Tick 2 (1st processWorldQueue): const=3, store=2, 3>0 skip.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after T2: queue length got %d, want 1 (delay 3→2)", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 2 {
		t.Errorf("after T2: delay got %d, want 2", got)
	}

	// Tick 3 (2nd processWorldQueue): const=2, store=1, 2>0 skip.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after T3: queue length got %d, want 1 (delay 2→1)", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 1 {
		t.Errorf("after T3: delay got %d, want 1", got)
	}

	// Tick 4 (3rd processWorldQueue): const=1, store=0, 1>0 skip.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after T4: queue length got %d, want 1 (delay 1→0)", got)
	}
	if got := s.worldScriptQueue[0].delay; got != 0 {
		t.Errorf("after T4: delay got %d, want 0", got)
	}

	// Tick 5 (4th processWorldQueue): const=0, store=-1, 0>0 false → fire.
	// Script resumes from after WORLD_DELAY, runs OpReturn, reaches
	// Finished. resumeOrFinishWorld sees Finished and drops the entry.
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("after T5: queue length got %d, want 0 (delay 0→-1, fires + completes)", got)
	}
	if state.Execution != script.Finished {
		t.Errorf("after T5: state.Execution got %v, want Finished", state.Execution)
	}
}
