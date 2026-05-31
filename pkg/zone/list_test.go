package zone

import (
	"testing"
)

func TestDoublyLinkList_AddTailIncrementsSize(t *testing.T) {
	var l DoublyLinkList[int]
	if l.Size() != 0 {
		t.Errorf("empty list Size: got %d, want 0", l.Size())
	}
	l.AddTail(10)
	l.AddTail(20)
	l.AddTail(30)
	if l.Size() != 3 {
		t.Errorf("after 3 AddTail, Size: got %d, want 3", l.Size())
	}
}

func TestDoublyLinkList_AddTailReturnsElement(t *testing.T) {
	var l DoublyLinkList[int]
	e := l.AddTail(42)
	if e == nil {
		t.Fatal("AddTail returned nil Element")
	}
	if e.Value != 42 {
		t.Errorf("Element.Value: got %d, want 42", e.Value)
	}
}

func TestDoublyLinkList_UnlinkRemovesAndDecrements(t *testing.T) {
	var l DoublyLinkList[int]
	_ = l.AddTail(10)
	e2 := l.AddTail(20)
	_ = l.AddTail(30)
	e2.Unlink()
	if l.Size() != 2 {
		t.Errorf("after Unlink, Size: got %d, want 2", l.Size())
	}
	got := []int{}
	for v := range l.All(false) {
		got = append(got, v)
	}
	want := []int{10, 30}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("after Unlink, iteration: got %v, want %v", got, want)
	}
}

func TestDoublyLinkList_UnlinkIdempotent(t *testing.T) {
	var l DoublyLinkList[int]
	e := l.AddTail(99)
	e.Unlink()
	if l.Size() != 0 {
		t.Errorf("after first Unlink, Size: got %d, want 0", l.Size())
	}
	// Second call must not panic and must not decrement size.
	e.Unlink()
	if l.Size() != 0 {
		t.Errorf("after second Unlink, Size: got %d, want 0", l.Size())
	}
}

func TestDoublyLinkList_AllForwardOrderMatchesInsertion(t *testing.T) {
	var l DoublyLinkList[int]
	for _, v := range []int{1, 2, 3, 4, 5} {
		l.AddTail(v)
	}
	got := []int{}
	for v := range l.All(false) {
		got = append(got, v)
	}
	want := []int{1, 2, 3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All(false)[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestDoublyLinkList_AllReverseOrderMatchesInsertion(t *testing.T) {
	var l DoublyLinkList[int]
	for _, v := range []int{1, 2, 3, 4, 5} {
		l.AddTail(v)
	}
	got := []int{}
	for v := range l.All(true) {
		got = append(got, v)
	}
	want := []int{5, 4, 3, 2, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All(true)[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestDoublyLinkList_EmptyAllYieldsNothing(t *testing.T) {
	var l DoublyLinkList[int]
	count := 0
	for range l.All(false) {
		count++
	}
	if count != 0 {
		t.Errorf("empty All count: got %d, want 0", count)
	}
}

func TestElement_UnlinkClearsListPointer(t *testing.T) {
	var l DoublyLinkList[int]
	e := l.AddTail(42)
	e.Unlink()
	if e.list != nil {
		t.Error("after Unlink, e.list should be nil for idempotency")
	}
}

func TestDoublyLinkList_UnlinkMiddleRelinksNeighbors(t *testing.T) {
	var l DoublyLinkList[int]
	e1 := l.AddTail(1)
	e2 := l.AddTail(2)
	e3 := l.AddTail(3)
	e2.Unlink()
	if e1.next != e3 {
		t.Error("after Unlink middle, e1.next should be e3")
	}
	if e3.prev != e1 {
		t.Error("after Unlink middle, e3.prev should be e1")
	}
}

func TestDoublyLinkList_UnlinkHeadUpdatesHead(t *testing.T) {
	var l DoublyLinkList[int]
	e1 := l.AddTail(1)
	e2 := l.AddTail(2)
	e1.Unlink()
	if l.head != e2 {
		t.Error("after Unlink head, list.head should be e2")
	}
}

func TestDoublyLinkList_UnlinkTailUpdatesTail(t *testing.T) {
	var l DoublyLinkList[int]
	e1 := l.AddTail(1)
	e2 := l.AddTail(2)
	e2.Unlink()
	if l.tail != e1 {
		t.Error("after Unlink tail, list.tail should be e1")
	}
}

// TestDoublyLinkList_AllForward_SurvivesMidIterationUnlink pins
// datastruct-db-1 (2026-05-28 audit): TS DoublyLinkList.all
// (DoublyLinkList.ts:73-87) captures `this.cursor` BEFORE yielding so
// the iterator survives an Unlink of the yielded node (Unlink clears
// next/prev to nil, which would otherwise strand the loop).
//
// Setup: list of [1, 2, 3, 4, 5]; the yield body Unlinks element with
// Value=2 the first time it sees it. TS-faithful iteration continues
// through 3, 4, 5. Pre-fix goscape stops at 2 (Unlink cleared n.next).
func TestDoublyLinkList_AllForward_SurvivesMidIterationUnlink(t *testing.T) {
	var l DoublyLinkList[int]
	elems := []*Element[int]{}
	for _, v := range []int{1, 2, 3, 4, 5} {
		elems = append(elems, l.AddTail(v))
	}
	target := elems[1] // Value=2
	got := []int{}
	for v := range l.All(false) {
		got = append(got, v)
		if v == 2 {
			target.Unlink()
		}
	}
	want := []int{1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("forward iteration with mid-Unlink: got %v (len=%d), want %v (len=%d) — pre-fix stops at 2 because Unlink clears n.next; TS DoublyLinkList.all captures next before yield",
			got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("forward iteration[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
	if l.Size() != 4 {
		t.Errorf("post-iteration Size: got %d, want 4 (Unlink removed 1 element)", l.Size())
	}
}

// TestDoublyLinkList_AllReverse_SurvivesMidIterationUnlink mirrors
// the above for reverse=true: Unlink the just-yielded element (Value=4
// in a [1,2,3,4,5] reverse walk → yields 5, 4, ...) and assert
// iteration completes 5, 4, 3, 2, 1 instead of stopping at 4.
func TestDoublyLinkList_AllReverse_SurvivesMidIterationUnlink(t *testing.T) {
	var l DoublyLinkList[int]
	elems := []*Element[int]{}
	for _, v := range []int{1, 2, 3, 4, 5} {
		elems = append(elems, l.AddTail(v))
	}
	target := elems[3] // Value=4 (second yield in reverse)
	got := []int{}
	for v := range l.All(true) {
		got = append(got, v)
		if v == 4 {
			target.Unlink()
		}
	}
	want := []int{5, 4, 3, 2, 1}
	if len(got) != len(want) {
		t.Fatalf("reverse iteration with mid-Unlink: got %v (len=%d), want %v (len=%d) — pre-fix stops at 4 because Unlink clears n.prev; TS captures prev before yield",
			got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reverse iteration[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}
