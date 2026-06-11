package world

import (
	"slices"
	"testing"
)

// TS World.getNextPlayerSlot (World.ts:1634-1642 @2e3bcf43): linear scan
// for the lowest free slot in [1, 2046]; -1 when full. Upstream a8186b95
// replaced the rev-244 round-robin/IP-windowed PlayerList allocation —
// there is no lastUsedIndex resume and no priority window anymore.
func TestGetNextPlayerSlot(t *testing.T) {
	mk := func() *playerList { return newPlayerList(2048) }
	pl := func() *Player { return &Player{} }

	t.Run("fresh list allocates slot 1 (slot 0 never used)", func(t *testing.T) {
		l := mk()
		if got := l.nextSlot(); got != 1 {
			t.Fatalf("nextSlot() = %d, want 1", got)
		}
	})
	t.Run("linear lowest-free, no round-robin resume", func(t *testing.T) {
		l := mk()
		l.set(1, pl())
		l.set(2, pl())
		l.set(3, pl())
		// rev-244 PlayerList would resume after lastUsedIndex (=3) → 4.
		// rev-254 getNextPlayerSlot always rescans from 1 → freed slot 2.
		two := l.get(2)
		l.remove(two)
		if got := l.nextSlot(); got != 2 {
			t.Fatalf("nextSlot() = %d, want 2 (lowest free, not round-robin)", got)
		}
	})
	t.Run("skips occupied slots", func(t *testing.T) {
		l := mk()
		for slot := 1; slot <= 5; slot++ {
			l.set(slot, pl())
		}
		if got := l.nextSlot(); got != 6 {
			t.Fatalf("nextSlot() = %d, want 6", got)
		}
	})
	t.Run("full world (1..2046 occupied) returns -1; 0 and 2047 never allocated", func(t *testing.T) {
		l := mk()
		for slot := 1; slot < 2047; slot++ { // TS: for (let i = 1; i < 2047; i++)
			l.set(slot, pl())
		}
		// Slots 0 and 2047 are free, yet allocation must fail: the TS scan
		// bounds are 1 <= i < 2047 against the 2048-entry array.
		if l.get(0) != nil || l.get(2047) != nil {
			t.Fatal("slots 0/2047 unexpectedly occupied")
		}
		if got := l.nextSlot(); got != -1 {
			t.Fatalf("nextSlot() = %d, want -1 (world full)", got)
		}
	})
	t.Run("count tracks add/remove", func(t *testing.T) {
		l := mk()
		a, b := pl(), pl()
		l.set(7, a)
		l.set(9, b)
		if l.count != 2 {
			t.Fatalf("count = %d, want 2", l.count)
		}
		l.remove(a)
		if l.count != 1 || l.get(7) != nil || l.get(9) != b {
			t.Fatalf("count/get after remove: count=%d", l.count)
		}
		// removing twice is a no-op
		l.remove(a)
		if l.count != 1 {
			t.Fatalf("count after double remove = %d, want 1", l.count)
		}
	})
}

// TS World.processLogins bucketing (World.ts:902-917 @2e3bcf43): the
// playerLoop key is the full packed IPv4 address (bucket = key & 7, i.e.
// the last octet's low 3 bits), the third hextet mod 256 for IPv6, and
// 2130706433 (127.0.0.1) for headless logins with no client socket.
func TestPlayerLoopKeyDerivation(t *testing.T) {
	cases := []struct {
		addr       string
		wantKey    uint64
		wantBucket int
	}{
		// IPv4: (203<<24)|(0<<16)|(113<<8)|47; bucket = 47 & 7 = 7.
		{"203.0.113.47:5000", 0xCB00712F, 7},
		// IPv4 high first octet: JS packs via signed int32 then BigInt;
		// the low 3 bits (the bucket) are unaffected. Go packs uint32.
		{"255.255.255.255:1", 0xFFFFFFFF, 7},
		{"10.0.0.0:1", 0x0A000000, 0},
		// IPv4-mapped form: "::ffff:127" fails to parse → 0, matching JS
		// parseInt → NaN coercing to 0 under <<. Key = last octet.
		{"::ffff:127.0.0.1", 0x00000001, 1},
		// IPv6: hextets[2] = "a1" = 161; 161 % 256 = 161; bucket 161&7 = 1.
		{"[2001:db8:a1::1]:5", 161, 1},
		// IPv6 loopback "::1": split(":") = ["", "", "1"] → hextets[2]="1".
		{"[::1]:43594", 1, 1},
		// headless (no client): 127.0.0.1 → bucket 1. TS World.ts:914-917.
		{"", 2130706433, 1},
		// DEVIATION (unreachable hardening): no '.' or ':' — TS would
		// never playerLoop.add the player at all; goscape routes to the
		// headless 127.0.0.1 bucket. net.Pipe test conns hit this path.
		{"pipe", 2130706433, 1},
	}
	for _, c := range cases {
		key := playerLoopKey(c.addr)
		if key != c.wantKey {
			t.Errorf("playerLoopKey(%q) = %#x, want %#x", c.addr, key, c.wantKey)
		}
		if b := int(key & (playerLoopBuckets - 1)); b != c.wantBucket {
			t.Errorf("bucket(%q) = %d, want %d", c.addr, b, c.wantBucket)
		}
	}
}

// TS HashTable.all (HashTable.ts:49-60 @2e3bcf43): per-tick processing
// iterates buckets 0..7 in ascending index order, each bucket in
// insertion (login) order. Processing order is therefore IP-influenced
// and independent of slot numbers.
func TestPlayerLoopIterationOrder(t *testing.T) {
	l := newPlayerList(2048)
	pl := func() *Player { return &Player{} }

	// bucket = key & 7. Insert across buckets out of bucket order.
	b7a := pl()
	l.add(10, 0xCB00712F, b7a) // bucket 7
	b1a := pl()
	l.add(11, 2130706433, b1a) // bucket 1 (127.0.0.1)
	b0a := pl()
	l.add(12, 0x0A000000, b0a) // bucket 0
	b1b := pl()
	l.add(13, 0x09, b1b) // bucket 1, second insertion
	b7b := pl()
	l.add(14, 0x07, b7b) // bucket 7, second insertion

	collect := func() []*Player {
		var got []*Player
		for p := range l.all() {
			got = append(got, p)
		}
		return got
	}
	want := []*Player{b0a, b1a, b1b, b7a, b7b}
	if got := collect(); !slices.Equal(got, want) {
		t.Fatalf("iteration order: got %v, want bucket-ascending then insertion order %v", got, want)
	}

	// Removal unlinks from the bucket without disturbing sibling order.
	l.remove(b1a)
	want = []*Player{b0a, b1b, b7a, b7b}
	if got := collect(); !slices.Equal(got, want) {
		t.Fatalf("iteration after remove: got %v, want %v", got, want)
	}

	// Re-adding an already-linked player moves it to the new bucket tail
	// (TS HashTable.add unlinks first, HashTable.ts:30-34).
	l.add(12, 0x0F, b0a) // bucket 0 → bucket 7 tail
	want = []*Player{b1b, b7a, b7b, b0a}
	if got := collect(); !slices.Equal(got, want) {
		t.Fatalf("iteration after re-add: got %v, want %v", got, want)
	}
}
