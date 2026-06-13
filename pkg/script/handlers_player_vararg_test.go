package script

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// TestStrongQueueVarArg_RoundTrip pins NAI-27 Bundle 3:
// STRONGQUEUEVARARG pops popScriptArgs (top), then delay, then scriptID,
// and enqueues a STRONG queue request with the captured args. Mirrors
// TS PlayerOps.ts:110-120 line-by-line.
func TestStrongQueueVarArg_RoundTrip(t *testing.T) {
	sf := newSingleOp("strong_vararg_rt", OpStrongQueueVarArg)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	// Int stack from bottom: [scriptID=101, delay=3, intArgs[0]=10, intArgs[1]=20].
	// String stack from bottom: [tags="ii"].
	state.PushInt(101)
	state.PushInt(3)
	state.PushInt(10)
	state.PushInt(20)
	state.PushString("ii")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 101 || got.Delay != 3 || got.Type != QueueStrong {
		t.Errorf("scalars: got ScriptID=%d Delay=%d Type=%v, want 101 3 QueueStrong",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{10, 20}) {
		t.Errorf("IntArgs: got %v, want [10 20]", got.IntArgs)
	}
	if len(got.StringArgs) != 0 {
		t.Errorf("StringArgs: got %v, want empty/nil", got.StringArgs)
	}
}

// TestStrongQueueVarArg_ScriptMissing pins NAI-27 Bundle 3:
// STRONGQUEUEVARARG returns the entity-layer script-missing error.
// Mirrors TS PlayerOps.ts:115-117.
func TestStrongQueueVarArg_ScriptMissing(t *testing.T) {
	sf := newSingleOp("strong_vararg_missing", OpStrongQueueVarArg)
	mp := &mockPlayer{
		enqueueScriptArgsReturnErr: fmt.Errorf("unable to find queue script: 77"),
	}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77)
	state.PushInt(0)
	state.PushString("")

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: expected error, got nil")
	}
	want := "unable to find queue script: 77"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	// The mock records the call before returning the configured error,
	// matching the existing TestQueueOps* script-missing pattern.
	if len(mp.enqueueCalls) != 1 {
		t.Errorf("enqueueCalls: got %d, want 1 (mock should record before returning)", len(mp.enqueueCalls))
	}
}

// TestStrongQueueVarArg_AcceptsNullDelay pins that STRONGQUEUEVARARG does
// NOT check NumberNotNull on delay (TS popInts(2) destructure, no check).
// Pushing the null sentinel must enqueue successfully. (Since rev-274 the
// fixed-arg STRONGQUEUE also no longer checks — see
// TestStrongQueueAcceptsNullDelay in handlers_test.go.)
func TestStrongQueueVarArg_AcceptsNullDelay(t *testing.T) {
	const nullDelay = -1 // ScriptValidators.ts NumberNotNull sentinel; matches checkNotNull at handlers_player.go:62.
	sf := newSingleOp("strong_vararg_null_delay", OpStrongQueueVarArg)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(101)
	state.PushInt(nullDelay)
	state.PushString("")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v (want nil — STRONGQUEUEVARARG does not check NumberNotNull)", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	if mp.enqueueCalls[0].Delay != nullDelay {
		t.Errorf("Delay: got %d, want %d (null sentinel preserved through enqueue)", mp.enqueueCalls[0].Delay, nullDelay)
	}
}

// TestWeakQueueVarArg_RoundTrip pins TS PlayerOps.ts:134-144.
func TestWeakQueueVarArg_RoundTrip(t *testing.T) {
	sf := newSingleOp("weak_vararg_rt", OpWeakQueueVarArg)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(202)
	state.PushInt(5)
	state.PushInt(11)
	state.PushInt(22)
	state.PushString("ii")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 202 || got.Delay != 5 || got.Type != QueueWeak {
		t.Errorf("scalars: got ScriptID=%d Delay=%d Type=%v, want 202 5 QueueWeak",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{11, 22}) {
		t.Errorf("IntArgs: got %v, want [11 22]", got.IntArgs)
	}
}

// TestWeakQueueVarArg_ScriptMissing pins TS PlayerOps.ts:139-141.
func TestWeakQueueVarArg_ScriptMissing(t *testing.T) {
	sf := newSingleOp("weak_vararg_missing", OpWeakQueueVarArg)
	mp := &mockPlayer{
		enqueueScriptArgsReturnErr: fmt.Errorf("unable to find queue script: 77"),
	}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77)
	state.PushInt(0)
	state.PushString("")

	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "unable to find queue script") {
		t.Errorf("error: got %v, want contains 'unable to find queue script'", err)
	}
}

// TestWeakQueueVarArg_AcceptsNullDelay pins TS PlayerOps.ts:134-144 (no NumberNotNull).
func TestWeakQueueVarArg_AcceptsNullDelay(t *testing.T) {
	const nullDelay = -1
	sf := newSingleOp("weak_vararg_null", OpWeakQueueVarArg)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(202)
	state.PushInt(nullDelay)
	state.PushString("")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 || mp.enqueueCalls[0].Delay != nullDelay {
		t.Errorf("Delay: got %d (calls=%d), want %d (1 call)",
			mp.enqueueCalls[0].Delay, len(mp.enqueueCalls), nullDelay)
	}
}

// TestQueueVarArg_RoundTrip pins TS PlayerOps.ts:159-169.
func TestQueueVarArg_RoundTrip(t *testing.T) {
	sf := newSingleOp("queue_vararg_rt", OpQueueVarArg)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(303)
	state.PushInt(7)
	state.PushInt(13)
	state.PushInt(26)
	state.PushString("ii")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 303 || got.Delay != 7 || got.Type != QueueNormal {
		t.Errorf("scalars: got ScriptID=%d Delay=%d Type=%v, want 303 7 QueueNormal",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{13, 26}) {
		t.Errorf("IntArgs: got %v, want [13 26]", got.IntArgs)
	}
}

// TestQueueVarArg_ScriptMissing pins TS PlayerOps.ts:163-165.
func TestQueueVarArg_ScriptMissing(t *testing.T) {
	sf := newSingleOp("queue_vararg_missing", OpQueueVarArg)
	mp := &mockPlayer{
		enqueueScriptArgsReturnErr: fmt.Errorf("unable to find queue script: 77"),
	}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77)
	state.PushInt(0)
	state.PushString("")

	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "unable to find queue script") {
		t.Errorf("error: got %v, want contains 'unable to find queue script'", err)
	}
}

// TestQueueVarArg_AcceptsNullDelay pins TS PlayerOps.ts:159-169 (no NumberNotNull).
// Note: differs from fixed-arg QUEUE (PlayerOps.ts:148-157), which also lacks
// NumberNotNull. As of rev-274 the fixed-arg STRONGQUEUE no longer checks
// NumberNotNull either (TS @dee467c8), so none of the queue ops gate delay.
func TestQueueVarArg_AcceptsNullDelay(t *testing.T) {
	const nullDelay = -1
	sf := newSingleOp("queue_vararg_null", OpQueueVarArg)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(303)
	state.PushInt(nullDelay)
	state.PushString("")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 || mp.enqueueCalls[0].Delay != nullDelay {
		t.Errorf("Delay: got %d (calls=%d), want %d (1 call)",
			mp.enqueueCalls[0].Delay, len(mp.enqueueCalls), nullDelay)
	}
}

// TestLongQueueVarArg_RoundTrip pins TS PlayerOps.ts:182-192. LONG
// diverges from the other VARARG variants by popping an extra
// logoutAction and prepending it to the args slice (TS line 191
// `[logoutAction, ...args]`).
func TestLongQueueVarArg_RoundTrip(t *testing.T) {
	sf := newSingleOp("long_vararg_rt", OpLongQueueVarArg)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	// Int stack from bottom: [scriptID=404, delay=2, logoutAction=99, intArgs[0]=55].
	// String stack from bottom: [tags="i"].
	state.PushInt(404)
	state.PushInt(2)
	state.PushInt(99)
	state.PushInt(55)
	state.PushString("i")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 404 || got.Delay != 2 || got.Type != QueueLong {
		t.Errorf("scalars: got ScriptID=%d Delay=%d Type=%v, want 404 2 QueueLong",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{99, 55}) {
		t.Errorf("IntArgs: got %v, want [99 55] (logoutAction prepended per TS PlayerOps.ts:191)", got.IntArgs)
	}
}

// TestLongQueueVarArg_LogoutActionPrepended pins the prepend ordering
// explicitly with multiple intArgs from popScriptArgs to distinguish
// from "wrap as single-element [logoutAction]" or "append" failure modes.
func TestLongQueueVarArg_LogoutActionPrepended(t *testing.T) {
	sf := newSingleOp("long_vararg_prepend", OpLongQueueVarArg)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	// popScriptArgs returns intArgs=[1, 2, 3]; logoutAction=99.
	// Expected entity-call IntArgs: [99, 1, 2, 3].
	// Int stack from bottom: [scriptID=505, delay=0, logoutAction=99, 1, 2, 3].
	// String stack from bottom: [tags="iii"].
	state.PushInt(505)
	state.PushInt(0)
	state.PushInt(99)
	state.PushInt(1)
	state.PushInt(2)
	state.PushInt(3)
	state.PushString("iii")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if !slices.Equal(got.IntArgs, []int{99, 1, 2, 3}) {
		t.Errorf("IntArgs: got %v, want [99 1 2 3] — logoutAction MUST prepend to popScriptArgs intArgs (TS line 191 `[logoutAction, ...args]`); got != [99] (single-wrap failure mode); got != [1 2 3 99] (append failure mode)", got.IntArgs)
	}
}

// TestLongQueueVarArg_ScriptMissing pins TS PlayerOps.ts:187-189.
func TestLongQueueVarArg_ScriptMissing(t *testing.T) {
	sf := newSingleOp("long_vararg_missing", OpLongQueueVarArg)
	mp := &mockPlayer{
		enqueueScriptArgsReturnErr: fmt.Errorf("unable to find queue script: 77"),
	}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77)
	state.PushInt(0)
	state.PushInt(0) // logoutAction
	state.PushString("")

	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "unable to find queue script") {
		t.Errorf("error: got %v, want contains 'unable to find queue script'", err)
	}
}

// TestLongQueueVarArg_AcceptsNullDelay pins TS PlayerOps.ts:182-192 (no NumberNotNull).
func TestLongQueueVarArg_AcceptsNullDelay(t *testing.T) {
	const nullDelay = -1
	sf := newSingleOp("long_vararg_null", OpLongQueueVarArg)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(404)
	state.PushInt(nullDelay)
	state.PushInt(0) // logoutAction
	state.PushString("")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 || mp.enqueueCalls[0].Delay != nullDelay {
		t.Errorf("Delay: got %d (calls=%d), want %d (1 call)",
			mp.enqueueCalls[0].Delay, len(mp.enqueueCalls), nullDelay)
	}
}

// TestVarArgOpsRequireActivePlayer parameterizes the per-opcode
// active-player gate over the 4 new VARARG opcodes — matches the
// TestTimerOpsRequireActivePlayer precedent at handlers_timer_test.go:220.
func TestVarArgOpsRequireActivePlayer(t *testing.T) {
	for _, op := range []Opcode{OpStrongQueueVarArg, OpWeakQueueVarArg, OpQueueVarArg, OpLongQueueVarArg} {
		t.Run(op.String(), func(t *testing.T) {
			sf := newSingleOp("no_self_"+op.String(), op)
			state := Init(sf, nil, false, nil, nil)
			// Pre-load enough items so the handler reaches the gate
			// (the gate fires before any popInt/popString).
			state.PushInt(0)
			state.PushInt(0)
			if op == OpLongQueueVarArg {
				state.PushInt(0) // extra logoutAction
			}
			state.PushString("")

			if err := Execute(state); err == nil {
				t.Errorf("%v: want error with nil Self", op)
			}
		})
	}
}
