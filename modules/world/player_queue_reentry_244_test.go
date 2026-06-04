package world

import (
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestProcessPlayerQueue_ClearDuringExecute_244 pins TS 244 cursor-save/restore
// semantics for goscape's slice-based iteration.
//
// TS context (Engine-TS/src/engine/entity/Player.ts:892-900 at pin 9aadcec4):
//
//	processQueue() {
//	    for (let request = this.queue.head(); request !== null; request = this.queue.next()) {
//	        ...
//	        const save = this.queue.cursor; // <-- 244 addition
//	        ...
//	        this.executeScript(script, true);
//	        this.queue.cursor = save;        // <-- 244 addition
//	    }
//	}
//
// TS's LinkList carries ONE shared cursor field. When script A fires and calls
// clearqueue/getqueue inside executeScript, those ops call unlink() on list nodes,
// which corrupts the outer iteration cursor. TS 244 saves cursor before and
// restores it after executeScript so the outer `for` loop can resume correctly.
//
// goscape uses `for i < len(p.queue)` index-based iteration over the LIVE slice.
// There is NO shared cursor field — the loop variable `i` is a plain Go int on the
// stack. Re-entrant mutation of p.queue (via UnlinkQueuedScript, clearWeakQueue,
// or append) cannot corrupt the loop counter because:
//   - The current entry is removed BEFORE firing (line tick.go:675), so i points
//     to the next element after the splice.
//   - UnlinkQueuedScript rebuilds p.queue via a filter copy (player_script.go:136-142)
//     and re-assigns p.queue. The loop re-reads len(p.queue) on every iteration
//     via the for-condition, so the new length is immediately visible.
//   - The index `i` is not stored in any shared field; inner calls cannot clobber it.
//
// Therefore, TS 244's cursor save/restore is a NO-OP for goscape.
//
// This test characterises both observable behaviors that the cursor guard protects:
//
//  1. Clear-during-execute: script A fires and removes B from the queue (simulating
//     CLEARQUEUE). TS contract: B must NOT fire (it was unlinked). The cursor restore
//     ensures the outer loop reaches the now-empty sentinel and stops cleanly.
//     Go result: SAME — after UnlinkQueuedScript rebuilds p.queue without B, the
//     loop condition `i < len(p.queue)` fails immediately (len==0), so B never fires.
//
//  2. Append-during-execute: script A fires and enqueues a new NORMAL entry C.
//     TS contract: C IS visible to the same pass because the cursor was saved
//     pointing into the middle of the list, and the list grew at the tail.
//     TS comment (Player.ts:874-878): "inconsistent queue timing (authentic)" — a
//     script before the end of the list can be processed this tick and enqueue
//     another script that also fires this tick.
//     Go result: SAME — `len(p.queue)` grows as the append happens, and the
//     for-condition re-evaluates each iteration, so C is reached in the same pass.
func TestProcessPlayerQueue_ClearDuringExecute_244(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sfA := &script.ScriptFile{Name: "[A-clears-B]", LookupKey: 0xA}
	sfB := &script.ScriptFile{Name: "[B-should-not-fire]", LookupKey: 0xB}
	s.scriptProvider.Register(sfA) // index 0
	s.scriptProvider.Register(sfB) // index 1

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.pid = 1
	s.players.set(1, p)

	var fired []uint32
	s.runScriptFn = func(f *script.ScriptFile, _ script.ActivePlayer, _ any, _ script.ServerTriggerType, _ bool, _ []int, _ []string) {
		fired = append(fired, f.LookupKey)
		if f.LookupKey == 0xA {
			// CLEARQUEUE(B) via the REAL production path: UnlinkQueuedScript
			// resolves script id 1 (sfB's Register slot) through
			// scriptProvider.GetByID and filter-rebuilds p.queue — exactly
			// what the clearqueue script op does from inside executeScript.
			// In TS, this would also clobber this.queue.cursor; TS 244
			// restores it. In Go, there is no cursor — the loop's i variable
			// is untouched by this.
			p.UnlinkQueuedScript(1)
		}
	}

	// Enqueue A then B, both ready at Delay=0 (NORMAL, non-weak).
	p.queue = []playerQueueRequest{
		{Script: sfA, Delay: 0, Type: script.QueueNormal},
		{Script: sfB, Delay: 0, Type: script.QueueNormal},
	}

	s.processPlayerQueue(p)

	// A fires; A removes B; loop condition `i < len(p.queue)` now fails (len==0);
	// B does NOT fire. TS-244 contract: clear-during-execute suppresses remaining entries.
	want := []uint32{0xA}
	if !slices.Equal(fired, want) {
		t.Fatalf("clear-during-execute: got %v, want %v\n"+
			"(after A removes B, B must not fire — TS Player.ts:892-900 cursor guard ensures "+
			"outer loop resumes at the now-empty list position; Go index iteration achieves "+
			"the same result without a cursor — TS 244 cursor save/restore is a NO-OP for goscape)",
			fired, want)
	}
	// Verify queue is empty: both A (consumed by the loop) and B (removed by A) are gone.
	if len(p.queue) != 0 {
		t.Errorf("p.queue len after: got %d, want 0", len(p.queue))
	}
}

// TestProcessPlayerQueue_AppendDuringExecute_244 characterises the TS-authentic
// "speedup quirk" (Player.ts:874-878) for goscape's slice iteration.
//
// TS LinkList.head() caches cursor = node.next at the first call; LinkList.next()
// reads cursor and advances it. After A fires and appends C to the tail, the list
// grows, and the cursor — after being restored by TS 244 — advances into C on the
// next iteration. So C IS visible in the SAME pass (authentic inconsistency).
//
// Go's `for i < len(p.queue)` re-evaluates len each iteration. A is spliced out
// BEFORE it fires and i is NOT incremented after the splice, so when A's body
// appends C the queue goes [] → [C] and the loop re-reads len at the SAME index
// i==0, which is now C. C fires in the SAME pass.
//
// Result: GO MATCHES TS. The append-visibility behavior is identical.
func TestProcessPlayerQueue_AppendDuringExecute_244(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sfA := &script.ScriptFile{Name: "[A-appends-C]", LookupKey: 0xA}
	sfC := &script.ScriptFile{Name: "[C-appended-by-A]", LookupKey: 0xC}
	s.scriptProvider.Register(sfA) // index 0
	s.scriptProvider.Register(sfC) // index 1

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.pid = 1
	s.players.set(1, p)

	var fired []uint32
	s.runScriptFn = func(f *script.ScriptFile, _ script.ActivePlayer, _ any, _ script.ServerTriggerType, _ bool, _ []int, _ []string) {
		fired = append(fired, f.LookupKey)
		if f.LookupKey == 0xA {
			// Simulate QUEUE(C, 0): enqueue a new NORMAL entry mid-pass.
			// TS: this.queue.addTail(C); cursor was restored to point just past A,
			// so the next iteration reaches B (if present), then C.
			// Go: append grows the slice; next loop iteration reads len(p.queue) and
			// reaches C at the new tail.
			p.queue = append(p.queue, playerQueueRequest{Script: sfC, Delay: 0, Type: script.QueueNormal})
		}
	}

	// Only A is enqueued initially.
	p.queue = []playerQueueRequest{
		{Script: sfA, Delay: 0, Type: script.QueueNormal},
	}

	s.processPlayerQueue(p)

	// A fires and appends C; C is visible in the SAME non-weak pass → fires same tick.
	// Matches TS "inconsistent queue timing (authentic)" (Player.ts:874-878).
	want := []uint32{0xA, 0xC}
	if !slices.Equal(fired, want) {
		t.Fatalf("append-during-execute: got %v, want %v\n"+
			"(C appended by A must fire in the SAME pass — TS 'inconsistent queue timing "+
			"(authentic)' Player.ts:874-878; Go len re-eval achieves the same result; "+
			"TS 244 cursor save/restore is a NO-OP for goscape)",
			fired, want)
	}
	// Queue should be empty: A consumed before firing, C consumed before firing.
	if len(p.queue) != 0 {
		t.Errorf("p.queue len after: got %d, want 0", len(p.queue))
	}
}
