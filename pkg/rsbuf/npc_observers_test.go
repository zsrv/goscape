package rsbuf

import "testing"

// resetObserversForTest clears observer state between tests. All
// tests in this file must call this at the start.
func resetObserversForTest() {
	clear(npcObservers)
}

func TestGetNpcObserversDefaultZero(t *testing.T) {
	resetObserversForTest()
	if got := GetNpcObservers(42); got != 0 {
		t.Errorf("GetNpcObservers(42) fresh: got %d, want 0", got)
	}
}

func TestIncNpcObserverIncrements(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(7)
	incNpcObserver(7)
	if got := GetNpcObservers(7); got != 2 {
		t.Errorf("GetNpcObservers(7) after 2 inc: got %d, want 2", got)
	}
}

func TestDecNpcObserverFloorsAtZero(t *testing.T) {
	resetObserversForTest()
	decNpcObserver(9) // dec with no prior inc
	if got := GetNpcObservers(9); got != 0 {
		t.Errorf("GetNpcObservers(9) after dec-from-zero: got %d, want 0", got)
	}
}

func TestDecNpcObserverDecrementsPositive(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(3)
	incNpcObserver(3)
	decNpcObserver(3)
	if got := GetNpcObservers(3); got != 1 {
		t.Errorf("GetNpcObservers(3) after inc+inc+dec: got %d, want 1", got)
	}
}

func TestRemovePlayerDecrementsEachSubscribedNid(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(10)
	incNpcObserver(10)
	incNpcObserver(20)
	subs := map[int]struct{}{10: {}, 20: {}}
	RemovePlayer(1, subs)
	if got := GetNpcObservers(10); got != 1 {
		t.Errorf("GetNpcObservers(10) after RemovePlayer: got %d, want 1", got)
	}
	if got := GetNpcObservers(20); got != 0 {
		t.Errorf("GetNpcObservers(20) after RemovePlayer: got %d, want 0", got)
	}
}

func TestRemovePlayerEmptySetIsNoOp(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(5)
	RemovePlayer(1, map[int]struct{}{})
	if got := GetNpcObservers(5); got != 1 {
		t.Errorf("GetNpcObservers(5) after empty RemovePlayer: got %d, want 1", got)
	}
}

func TestRemovePlayerNilSetIsNoOp(t *testing.T) {
	resetObserversForTest()
	incNpcObserver(6)
	RemovePlayer(1, nil) // must not panic
	if got := GetNpcObservers(6); got != 1 {
		t.Errorf("GetNpcObservers(6) after nil RemovePlayer: got %d, want 1", got)
	}
}

func TestSetObserverForTestOverridesCount(t *testing.T) {
	resetObserversForTest()
	SetObserverForTest(42, 5)
	if got := GetNpcObservers(42); got != 5 {
		t.Errorf("GetNpcObservers(42) after SetObserverForTest(5): got %d, want 5", got)
	}
	SetObserverForTest(42, 0)
	if got := GetNpcObservers(42); got != 0 {
		t.Errorf("GetNpcObservers(42) after SetObserverForTest(0): got %d, want 0", got)
	}
}
