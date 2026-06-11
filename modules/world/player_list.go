package world

import (
	"iter"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

// playerList ports the rev-254 World player containers (TS World.ts:145-148
// @2e3bcf43, introduced upstream in a8186b95 "refactor: Replaced PlayerList
// with HashTable<Player>"):
//
//		// the server processes players in the underlying bucket-order (key fragment + insertion order)
//		readonly playerLoop: HashTable<Player> = new HashTable(8);
//		// the client and server communicate via player "slots," separate from processing
//		readonly players: Player[] = new Array(2048);
//
//	  - entities is the protocol slot array (TS `players`). "Slot" is the
//	    wire identity — player info, UPDATE_PID, hint arrows, OP_PLAYER
//	    input — and says nothing about processing order.
//	  - buckets is the processing loop (TS `playerLoop`), keyed by client
//	    IP. Per-tick passes iterate bucket 0..7 in ascending order, each
//	    bucket in insertion (login) order, so processing order is
//	    IP-influenced and independent of slot numbers (TS HashTable.ts).
//
// Go representation: TS chains players through the intrusive Linkable
// next/prev fields with one sentinel per bucket; the Go port keeps a
// slice per bucket (append = TS tail-insert, slices.Delete = unlink).
// Membership is tracked via Player.loopBucket instead of Linkable.prev.
// Observable contract — bucket selection, in-bucket order, removal and
// re-add semantics — is identical.
type playerList struct {
	entities []*Player                    // slot → player; TS World.players
	buckets  [playerLoopBuckets][]*Player // TS World.playerLoop (HashTable(8))
	// count caches the occupied-slot total. TS getTotalPlayers
	// (World.ts:1691-1702) recounts slots 1..2046 on every call, with a
	// "todo: could cache this, or increment/decrement on add/remove"
	// note; the cached value is identical. Atomic because connection
	// goroutines read it (handleLogin's NodeMaxConnected gate) while
	// the tick goroutine mutates, and tick-side readers can sit inside
	// playersMu critical sections (an RLock there would self-deadlock).
	count atomic.Int32
}

// playerLoopBuckets mirrors `new HashTable(8)` (TS World.ts:146). The
// bucket index is key & (playerLoopBuckets - 1) (TS HashTable.ts:35).
const playerLoopBuckets = 8

// playerLoopHeadlessKey is the loop key for logins with no attached
// client socket: 2130706433 = 127.0.0.1 (TS World.ts:914-917).
const playerLoopHeadlessKey uint64 = 2130706433

func newPlayerList(size int) *playerList {
	return &playerList{entities: make([]*Player, size)}
}

func (l *playerList) get(slot int) *Player {
	if slot < 0 || slot >= len(l.entities) {
		return nil
	}
	return l.entities[slot]
}

// nextSlot ports TS World.getNextPlayerSlot (World.ts:1634-1642): linear
// scan for the lowest free slot in [1, 2046]. Slot 0 is never allocated
// and neither is the final slot — TS scans `1 <= i < 2047` against the
// 2048-entry array. Returns -1 when the world is full (TS returns -1;
// the caller rejects the login with the world-full reply). Unlike the
// rev-244 PlayerList there is no round-robin resume and no IP-derived
// priority window: a freed low slot is reused by the very next login.
func (l *playerList) nextSlot() int {
	for slot := 1; slot < len(l.entities)-1; slot++ {
		if l.entities[slot] == nil {
			return slot
		}
	}
	return -1
}

// add inserts p at slot and appends it to the playerLoop bucket derived
// from key. Mirrors the TS login sequence (World.ts:902-921):
// playerLoop.add(key, player), players[slot] = player, player.slot =
// slot (the World.ts:921 assignment is folded in here so the
// remove-by-player slot lookup is always coherent). TS HashTable.add
// unlinks an already-linked value before appending it at the bucket
// tail (HashTable.ts:30-43); add replicates both behaviors.
func (l *playerList) add(slot int, key uint64, p *Player) {
	if l.entities[slot] == nil {
		l.count.Add(1)
	}
	l.entities[slot] = p
	p.slot = slot
	l.loopUnlink(p)
	b := int(key & (playerLoopBuckets - 1))
	l.buckets[b] = append(l.buckets[b], p)
	p.loopBucket = b + 1
}

// set is add with the headless 127.0.0.1 loop key (TS World.ts:914-917).
// Production logins go through Server.addPlayer → add with the
// client-derived key; set is the shorthand used by tests and matches
// the no-client login path exactly.
func (l *playerList) set(slot int, p *Player) {
	l.add(slot, playerLoopHeadlessKey, p)
}

// remove clears p's slot and unlinks it from its playerLoop bucket.
// TS World.removePlayer (World.ts:1586-1589): `delete this.players[slot]`
// followed by `player.unlink()`.
func (l *playerList) remove(p *Player) {
	if p == nil {
		return
	}
	if p.slot >= 0 && p.slot < len(l.entities) && l.entities[p.slot] == p {
		l.entities[p.slot] = nil
		l.count.Add(-1)
	}
	l.loopUnlink(p)
}

// loopUnlink removes p from its playerLoop bucket, if linked. Ports TS
// Linkable.unlink (Linkable.ts:6-15); no-op when not linked, like the
// TS prev == null guard.
func (l *playerList) loopUnlink(p *Player) {
	if p.loopBucket == 0 {
		return
	}
	b := p.loopBucket - 1
	if i := slices.Index(l.buckets[b], p); i >= 0 {
		l.buckets[b] = slices.Delete(l.buckets[b], i, i+1)
	}
	p.loopBucket = 0
}

// all iterates the playerLoop: buckets in ascending index order, each
// bucket in insertion (login) order. This is the per-tick processing
// order at the 254 pin (TS HashTable.all, HashTable.ts:49-60) — NOT
// slot order. Callers that remove players mid-pass must snapshot first
// (snapshotPlayers), the same invariant as before; the TS iterator
// instead pre-reads node.next before yielding.
func (l *playerList) all() iter.Seq[*Player] {
	return func(yield func(*Player) bool) {
		for b := range l.buckets {
			for _, p := range l.buckets[b] {
				if !yield(p) {
					return
				}
			}
		}
	}
}

// splitHostPort wraps net.SplitHostPort and tolerates a missing port by
// returning the input as the host unchanged. Parse problems are absorbed
// (never surfaced) — playerLoopKey treats any unparseable address as the
// headless 127.0.0.1 bucket, so there is nothing to signal.
func splitHostPort(addr string) (host, port string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, ""
	}
	return host, port
}

// playerLoopKey derives the playerLoop key from the client's remote
// address, porting the TS login bucketing (World.ts:902-917 @2e3bcf43):
//
//   - IPv4 ("last octet determines the bucket"): all four octets packed
//     big-endian — bucket = key & 7 = the last octet's low 3 bits. JS
//     packs through signed int32 then BigInt; two's complement preserves
//     the low bits, so the Go uint32 pack selects identical buckets.
//     Octets that fail to parse contribute 0, matching JS parseInt → NaN
//     coercing to 0 under <</| (relevant for v4-mapped forms like
//     "::ffff:1.2.3.4", whose first "octet" is "::ffff:1").
//   - IPv6 ("site prefix determines the bucket"): the third hextet
//     parsed as hex, mod 256. A missing/unparseable hextet contributes 0
//     (TS would throw on BigInt(NaN); unreachable for net.Conn-sourced
//     addresses, which are always well-formed).
//   - no address (headless login, no client socket): 2130706433 =
//     127.0.0.1 (TS World.ts:914-917) → bucket 1.
//
// DEVIATION (unreachable-path hardening): a connected client whose
// address contains neither '.' nor ':' is never playerLoop.add-ed in TS
// at all — the player would hold a slot but never be processed. Real
// net.Conn addresses always contain one of the two; goscape routes the
// impossible remainder (and net.Pipe's "pipe" test address) into the
// headless 127.0.0.1 bucket instead of silently never processing the
// player.
//
// DEVIATION (key-0 sentinel skip NOT replicated): TS HashTable seeds
// every bucket with a key-0 sentinel Linkable and its iteration stops
// at the first key-0 node, so a player whose derived key is 0 — e.g.
// IPv6 with third hextet 0 ("2001:db8:0:…" → 0 % 256 = 0) or IPv4
// 0.0.0.0 — is never yielded AND hides every later login in bucket 0
// from per-tick processing (TS HashTable.ts sentinel scan). That is an
// upstream container bug, not intended behavior; goscape's slice
// buckets iterate key-0 players normally. Pinned by
// TestPlayerLoopKeyZeroStillProcessed.
//
// TS parses the raw address string (split on '.' / ':'), so the Go port
// does the same rather than using net.IP. remoteAddr may be
// "host:port", a bare host, or "" (no client).
func playerLoopKey(remoteAddr string) uint64 {
	host, _ := splitHostPort(remoteAddr)
	if strings.Contains(host, ".") {
		octets := strings.Split(host, ".")
		octet := func(i int) uint32 {
			if i >= len(octets) {
				return 0 // JS: octets[i] undefined → parseInt → NaN → 0
			}
			n, _ := strconv.Atoi(octets[i]) // parse failure → 0 (JS NaN)
			return uint32(int32(n))
		}
		// TS: (o0 << 24) | (o1 << 16) | (o2 << 8) | o3, through signed
		// int32 — uint32 arithmetic yields the same low 32 bits.
		key := octet(0)<<24 | octet(1)<<16 | octet(2)<<8 | octet(3)
		return uint64(key)
	}
	if strings.Contains(host, ":") {
		if hextets := strings.Split(host, ":"); len(hextets) > 2 {
			if v, err := strconv.ParseUint(hextets[2], 16, 64); err == nil {
				return v % 256
			}
		}
		return 0
	}
	return playerLoopHeadlessKey
}
