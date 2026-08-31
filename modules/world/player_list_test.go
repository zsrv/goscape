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
		if l.count.Load() != 2 {
			t.Fatalf("count = %d, want 2", l.count.Load())
		}
		l.remove(a)
		if l.count.Load() != 1 || l.get(7) != nil || l.get(9) != b {
			t.Fatalf("count/get after remove: count=%d", l.count.Load())
		}
		// removing twice is a no-op
		l.remove(a)
		if l.count.Load() != 1 {
			t.Fatalf("count after double remove = %d, want 1", l.count.Load())
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
		// IPv6 since Engine-TS 8139461a: the whole address is left-packed
		// 16 bits at a time (World.ts:913-925 @1d25566c), so the key is the
		// trailing groups rather than "third hextet mod 256". 2001:db8:a1::1
		// elides five groups, leaving 1 in the low bits; bucket 1&7 = 1.
		{"[2001:db8:a1::1]:5", 1, 1},
		// IPv6 loopback "::1": all but the final group are elided.
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

// TestPlayerLoopKeyZeroStillProcessed pins the documented deviation on
// playerLoopKey: a derived key of 0 collides with TS HashTable's key-0
// sentinel, so TS iteration would never yield the player and would hide later
// bucket-0 logins behind it — an upstream container bug goscape does NOT
// replicate, because the Go port keys buckets by slice rather than by a
// sentinel-terminated list. Both the key-0 player and a later bucket-0 login
// must be iterated.
//
// The IPv6 probe changed with Engine-TS 8139461a: under the old "third hextet
// mod 256" rule a zero third group produced key 0, but the key is now the
// left-packed address, so a trailing "::" is what zeroes the low bits.
func TestPlayerLoopKeyZeroStillProcessed(t *testing.T) {
	if k := playerLoopKey("[2001:db8::]:43594"); k != 0 {
		t.Fatalf("IPv6 trailing-:: key = %#x, want 0", k)
	}
	if k := playerLoopKey("0.0.0.0:1"); k != 0 {
		t.Fatalf("0.0.0.0 key = %#x, want 0", k)
	}

	l := newPlayerList(2048)
	zeroKey := &Player{}
	l.add(1, 0, zeroKey) // bucket 0, key 0 — TS sentinel collision
	later := &Player{}
	l.add(2, 8, later) // bucket 0 (8 & 7), behind the key-0 player

	var got []*Player
	for p := range l.all() {
		got = append(got, p)
	}
	if len(got) != 2 || got[0] != zeroKey || got[1] != later {
		t.Fatalf("key-0 iteration: got %d players (want both, in insertion order)", len(got))
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

// TestIPv6LoopKey pins the full left-packed IPv6 key Engine-TS 8139461a
// introduced (World.ts:913-925 @1d25566c), replacing "third hextet mod 256".
//
// Expected values are computed the same way TS does — shift each present
// hextet in by 16 bits, expanding a "::" run to the number of groups it
// elides — then truncated to 64 bits, which is exact for the low 3 bits the
// bucket index actually consumes.
func TestIPv6LoopKey(t *testing.T) {
	tests := []struct {
		name string
		host string
		want uint64
	}{
		// 8 explicit groups: the low 64 bits are the last four hextets.
		{"full address", "2001:0db8:0000:0000:0000:ff00:0042:8329",
			0x0000_ff00_0042_8329},
		// "::" elides six groups; only ::1 contributes, so the key is 1.
		{"loopback", "::1", 1},
		// 2001:db8::1 -> 2001,0db8 then six elided groups then 1.
		{"compressed middle", "2001:db8::1", 1},
		// A zone suffix must be stripped before parsing.
		{"zone suffix stripped", "fe80::1%eth0", 1},
		// Trailing "::" contributes nothing after the shift, so the low bits
		// are zero.
		{"trailing double colon", "2001:db8::", 0},
		// Unparseable groups coerce to 0, matching JS parseInt -> NaN.
		{"garbage group", "zzzz::1", 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ipv6LoopKey(tc.host); got != tc.want {
				t.Errorf("ipv6LoopKey(%q) = %#x, want %#x", tc.host, got, tc.want)
			}
		})
	}
}

// TestIPv6LoopKeyDistinguishesAddresses pins the point of the rewrite. Under
// the old "third hextet mod 256" rule every address below shared hextet index
// 2, so all three landed in one bucket regardless of how unrelated they were.
func TestIPv6LoopKeyDistinguishesAddresses(t *testing.T) {
	hosts := []string{
		"2001:db8:0:0:0:0:0:1",
		"2001:db8:0:0:0:0:0:2",
		"2001:db8:0:0:0:0:0:3",
	}
	seen := map[uint64]string{}
	for _, h := range hosts {
		k := ipv6LoopKey(h)
		if prev, dup := seen[k]; dup {
			t.Errorf("%q and %q collide on key %#x", prev, h, k)
		}
		seen[k] = h
	}
}

// TestIPv4LoopKeyIsUnsigned pins the >>> 0 half of the same upstream change: a
// leading octet >= 128 must not produce a sign-extended key.
func TestIPv4LoopKeyIsUnsigned(t *testing.T) {
	got := playerLoopKey("255.0.0.1")
	want := uint64(0xff00_0001)
	if got != want {
		t.Errorf("playerLoopKey(255.0.0.1) = %#x, want %#x", got, want)
	}
}
