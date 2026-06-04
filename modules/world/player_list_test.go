package world

import (
	"slices"
	"testing"
)

// TS EntityList.ts:6-115 / PlayerList (244 pin).
func TestPlayerListAllocation(t *testing.T) {
	mk := func() *playerList { return newPlayerList(2048) }
	pl := func() *Player { return &Player{} }

	t.Run("round-robin resumes after lastUsedIndex", func(t *testing.T) {
		l := mk()
		l.set(5, pl())
		if got := l.next(); got != 6 { // EntityList.ts:22-28
			t.Fatalf("next() = %d, want 6", got)
		}
	})
	t.Run("wraparound floors at indexPadding 1, never pid 0", func(t *testing.T) {
		l := mk()
		for pid := 1; pid < 2048; pid++ {
			l.set(pid, pl())
		}
		l.remove(3)
		l.set(2047, pl()) // lastUsedIndex = 2047 → forward scan empty
		if got := l.next(); got != 3 { // EntityList.ts:29-33 wrap from indexPadding
			t.Fatalf("next() = %d, want 3", got)
		}
	})
	t.Run("full list returns -1", func(t *testing.T) {
		l := mk()
		for pid := range 2048 {
			l.set(pid, pl())
		}
		if got := l.next(); got != -1 { // TS throws (EntityList.ts:34); Go -1
			t.Fatalf("next() = %d, want -1", got)
		}
	})
	t.Run("priority window scans [start, start+100)", func(t *testing.T) {
		l := mk()
		l.set(300, pl())
		if got := l.nextPriority(300); got != 301 { // EntityList.ts:100-114
			t.Fatalf("nextPriority(300) = %d, want 301", got)
		}
	})
	t.Run("priority start 0 skips pid 0 (init quirk)", func(t *testing.T) {
		l := mk()
		if got := l.nextPriority(0); got != 1 { // EntityList.ts:103-105
			t.Fatalf("nextPriority(0) = %d, want 1", got)
		}
	})
	t.Run("priority window exhausted falls back to round-robin default start", func(t *testing.T) {
		l := mk()
		for pid := 300; pid < 400; pid++ {
			l.set(pid, pl())
		}
		l.set(7, pl()) // lastUsedIndex = 7
		if got := l.nextPriority(300); got != 8 { // super.next() w/ DEFAULT start (EntityList.ts:113)
			t.Fatalf("nextPriority(300) = %d, want 8", got)
		}
	})
	t.Run("iteration in pid order; count tracks set/remove", func(t *testing.T) {
		l := mk()
		a, b, c := pl(), pl(), pl()
		l.set(900, a)
		l.set(4, b)
		l.set(2000, c)
		var got []*Player
		for p := range l.all() { // EntityList.ts:37-48 — id order, not insertion
			got = append(got, p)
		}
		if !slices.Equal(got, []*Player{b, a, c}) {
			t.Fatalf("iteration order wrong: %v", got)
		}
		l.remove(4)
		if l.count != 2 || l.get(4) != nil || l.get(900) != a {
			t.Fatalf("count/get after remove: count=%d", l.count)
		}
	})
}

// TS World.getNextPid (World.ts:1758-1773): IP-derived priority start.
func TestGetNextPidStartDerivation(t *testing.T) {
	cases := []struct {
		addr string
		want int
	}{
		{"203.0.113.47:5000", 700},  // IPv4: (47 % 20) * 100
		{"10.0.0.0:1", 1},           // IPv4 start 0 → init quirk → pid 1
		{"[2001:db8:a1::1]:5", 100}, // IPv6: (0xa1 % 20) * 100 = (161%20)*100
		{"", 1},                     // no addr → plain next() (fresh list → 1)
	}
	for _, c := range cases {
		l := newPlayerList(2048) // fresh empty list per case
		if got := getNextPid(l, c.addr); got != c.want {
			t.Errorf("getNextPid(%q) = %d, want %d", c.addr, got, c.want)
		}
	}
}
