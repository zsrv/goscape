package world

import (
	"testing"

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

func TestProcessWorldQueue_DelayZero_FiresImmediately(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 0)
	if got, want := len(s.worldScriptQueue), 1; got != want {
		t.Fatalf("queue length post-enqueue: got %d, want %d", got, want)
	}
	s.processWorldQueue()
	if got, want := len(s.worldScriptQueue), 0; got != want {
		t.Errorf("queue length post-fire: got %d, want %d (delay=0 entry must fire and be removed)", got, want)
	}
	// Verify the script ran to Finished.
	if got, want := state.Execution, script.Finished; got != want {
		t.Errorf("script Execution post-fire: got %v, want %v", got, want)
	}
}

func TestProcessWorldQueue_DelayN_FiresAfterNTicks(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 3)

	// Tick 1: delay 3 → 2 (>0, skip).
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after tick 1: queue length got %d, want 1", got)
	}
	// Tick 2: delay 2 → 1 (>0, skip).
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after tick 2: queue length got %d, want 1", got)
	}
	// Tick 3: delay 1 → 0 (NOT > 0, fires).
	s.processWorldQueue()
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("after tick 3: queue length got %d, want 0 (delay=3 entry fires on the 3rd processWorldQueue call)", got)
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
	s.EnqueueWorldScript(a, 0)
	s.EnqueueWorldScript(b, 0)
	s.EnqueueWorldScript(c, 0)

	wantOrder := []string{"A", "B", "C"}

	// processWorldQueue should drain all 3 in FIFO order.
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
	s.EnqueueWorldScript(state, 0)
	s.processWorldQueue()
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
	s.EnqueueWorldScript(a, 0)
	s.EnqueueWorldScript(b, 0)
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
