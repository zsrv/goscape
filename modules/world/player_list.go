package world

import (
	"iter"
	"net"
	"strconv"
	"strings"
)

// playerList ports TS EntityList/PlayerList (EntityList.ts:6-115 at the 244
// pin): the pid-keyed player registry with round-robin allocation.
//
// Representation: TS keeps a storage Array + ids Int32Array + free Set; the
// indirection has no observable effect (get/set/remove keyed by id,
// iteration in id order, count = size - free.size), so the Go port stores
// entities directly by pid. Observable contract — allocation order,
// iteration order, count, full-world sentinel — is identical.
type playerList struct {
	entities      []*Player
	count         int
	lastUsedIndex int // last pid passed to set(); next() resumes after it. TS EntityList.ts:67
}

// playerListIndexPadding mirrors PlayerList's super(size, 1): the
// wraparound floor of the round-robin scan — pid 0 is never allocated.
// TS EntityList.ts:15-20,97.
const playerListIndexPadding = 1

func newPlayerList(size int) *playerList {
	return &playerList{entities: make([]*Player, size)}
}

func (l *playerList) get(pid int) *Player {
	if pid < 0 || pid >= len(l.entities) {
		return nil
	}
	return l.entities[pid]
}

func (l *playerList) set(pid int, p *Player) { // TS EntityList.ts:59-68
	if l.entities[pid] == nil {
		l.count++
	}
	l.entities[pid] = p
	l.lastUsedIndex = pid
}

func (l *playerList) remove(pid int) { // TS EntityList.ts:70-77
	if l.entities[pid] != nil {
		l.entities[pid] = nil
		l.count--
	}
}

// next is the round-robin scan: forward from lastUsedIndex+1, wrapping at
// indexPadding. Returns -1 when full (TS throws; the only caller maps it
// to the WORLD_FULL login reply). TS EntityList.ts:22-35.
func (l *playerList) next() int {
	start := l.lastUsedIndex + 1
	for pid := start; pid < len(l.entities); pid++ {
		if l.entities[pid] == nil {
			return pid
		}
	}
	for pid := playerListIndexPadding; pid < start && pid < len(l.entities); pid++ {
		if l.entities[pid] == nil {
			return pid
		}
	}
	return -1
}

// nextPriority scans the 100-wide preferred window [start, start+100),
// skipping pid 0 via the init quirk, then falls back to the DEFAULT-start
// round-robin (TS calls super.next() with no args). TS EntityList.ts:100-114.
func (l *playerList) nextPriority(start int) int {
	init := 0
	if start == 0 {
		init = 1
	}
	for i := init; i < 100; i++ {
		pid := start + i
		if pid < len(l.entities) && l.entities[pid] == nil {
			return pid
		}
	}
	return l.next()
}

// all iterates players in pid order. TS EntityList.ts:37-48.
func (l *playerList) all() iter.Seq[*Player] {
	return func(yield func(*Player) bool) {
		for _, p := range l.entities {
			if p != nil && !yield(p) {
				return
			}
		}
	}
}

// splitHostPort wraps net.SplitHostPort and tolerates a missing port by
// returning the input as the host unchanged. Parse problems are absorbed
// (never surfaced) — getNextPid treats any unparseable address as the
// plain round-robin path, so there is nothing to signal.
func splitHostPort(addr string) (host, port string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, ""
	}
	return host, port
}

// getNextPid ports TS World.getNextPid (World.ts:1758-1773): derive the
// preferred pid window from the remote address. TS parses the raw address
// string (split on '.' / ':'), so the Go port does the same rather than
// using net.IP; any parse failure falls back to the plain round-robin.
// remoteAddr may be "host:port", a bare host, or "" (no client).
func getNextPid(l *playerList, remoteAddr string) int {
	host, _ := splitHostPort(remoteAddr)
	if strings.Contains(host, ".") {
		// IPv4 — first available pid starting from (low ip octet % 20) * 100.
		octets := strings.Split(host, ".")
		if n, err := strconv.Atoi(octets[len(octets)-1]); err == nil {
			return l.nextPriority((n % 20) * 100)
		}
	} else if strings.Contains(host, ":") {
		// IPv6 — first available pid starting from (low site prefix % 20) * 100.
		hextets := strings.Split(host, ":")
		if len(hextets) > 2 {
			if n, err := strconv.ParseInt(hextets[2], 16, 64); err == nil {
				return l.nextPriority((int(n) % 20) * 100)
			}
		}
	}
	return l.next()
}
