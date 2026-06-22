# rev-244 Bundle 3: engine core — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the B3 slice of the Engine-TS 225→244 delta — `World.ts`, the entity family, the new in-engine `OnDemand.ts`, `InputTrackingBlob.ts`, the B1-deferred CrcTable/PreloadedPacks rewiring, the `web.ts` delivery delta, the MidiPack registry, and the world-side login rate-limit removal.

**Architecture:** Faithful TS→Go translation per `PORTING-LESSONS.md` (read first: `git show main:PORTING-LESSONS.md` — §3 gotchas, §4 citations, §5 gates). Six slices in dependency order: PlayerList/pid foundation → entity behavior deltas → account_id/tracking → handshake+OnDemand → cache/HTTP delivery → midi/residuals/audit. Each task slices the cross-pin diff to one file group; the TS diff is the contract. All work lands on branch `rev-244`.

**Tech Stack:** Go 1.26 (modern idioms: `iter.Seq`, `for range n`, `min`). Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix. Build: `CGO_ENABLED=0 go build -trimpath ./...`. Race: `-race` on touched packages (CGO_ENABLED=1). Every commit: `--no-gpg-sign` + Claude trailer.

**Spec:** `docs/superpowers/specs/2026-06-04-rev244-b3-engine-core-design.md`

**References:** Engine-TS at the 244 pin: `$HOME/Code/github.com/LostCityRS/Engine-TS` (checkout IS at `9aadcec4`).

**Scope decisions already made (do not relitigate):**
- 244 runtime cache **deferred to B6** (user decision): all FileStream-backed serving is built against synthetic fixtures; the live client smoke + window closures ride B6.
- **`pid` adopted wholesale** (user decision): supersedes B2's HintArrow keep-slot row.
- B2-shipped hunks must **NOT** be double-applied: damage2 entity+feed (`2afa543c`), UpdateUid192 encoder + members derivation (`010ee146`), LastLoginInfo warn flag, IF_OPENOVERLAY table row (`0ef495fb`).
- NOT-PORTED (dead-at-pin): `World.addPlayer`, `Npc.spawnTriggerPending`. NOT-PORTED (platform): `STANDALONE_BUNDLE`/`WorkerFactory`/`typeof self`.
- Deferred: `world_heartbeat` → B5; friends/logger message shapes → B5/private-sibling; `BUILD_STARTUP_UPDATE` + `packAll(modelFlags)` → B6.
- Sandbox gotcha: `git status` shows phantom `??` dotfiles — device-node masks, NOT real files. Never stage them; never `git add -A`. Warn every subagent.

**Bake into every implementer prompt (recurring B2 defects):**
1. Verify every `// TS <File>.ts:<lines>` citation against a numbered listing (`git -C $HOME/Code/github.com/LostCityRS/Engine-TS show 9aadcec4:<file> | cat -n | sed -n '<range>p'`) BEFORE writing.
2. Reject-path tests must seed earlier-gate prerequisites so the gate under test is the discriminating condition.
3. Final-review "missing X" findings can be false positives — verify directly before fixing.

---

## Slice 1 — Foundation: PlayerList + pid

### Task 1: playerList data structure

**Files:**
- Create: `modules/world/player_list.go`
- Create: `modules/world/player_list_test.go`

TS contract: `git -C $HOME/Code/github.com/LostCityRS/Engine-TS show 9aadcec4:src/engine/entity/EntityList.ts | cat -n` — read all 113 lines. Key semantics: `set(id)` records `lastUsedIndex = id` (line 67); base `next()` scans `[lastUsedIndex+1, len)` then wraps `[indexPadding, start)` (lines 22-35); PlayerList has `indexPadding = 1` (line 92: `super(size, 1)`); the priority override (lines 100-112) scans a 100-wide window `[start+init, start+100)` where `init = start == 0 ? 1 : 0`, then falls back to the **default-start** base scan; iteration walks ids **in id order** (lines 37-48).

Representation note (record as in-code comment): TS keeps storage Array + `ids` Int32Array + free Set; the indirection has zero observable effect (get/set/remove keyed by id, iteration in id order, `count = size − free.size`), so Go stores `entities []*Player` indexed by pid directly.

- [ ] **Step 1: Write the failing test.**

```go
package world

import (
	"slices"
	"testing"
)

// TS EntityList.ts:6-113 / PlayerList (244 pin).
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
		if got := l.nextPriority(300); got != 301 { // EntityList.ts:100-112
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
		if got := l.nextPriority(300); got != 8 { // super.next() w/ DEFAULT start (EntityList.ts:111)
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
		{"203.0.113.47:5000", 700},     // IPv4: (47 % 20) * 100
		{"10.0.0.0:1", 1},              // IPv4 start 0 → init quirk → pid 1
		{"[2001:db8:a1::1]:5", 100},    // IPv6: (0xa1 % 20) * 100 = (161%20)*100
		{"", 1},                        // no addr → plain next() (fresh list → 1)
	}
	for _, c := range cases {
		l := newPlayerList(2048) // fresh empty list per case
		if got := getNextPid(l, c.addr); got != c.want {
			t.Errorf("getNextPid(%q) = %d, want %d", c.addr, got, c.want)
		}
	}
}
```

- [ ] **Step 2: FAIL run** — `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlayerList -count=1` → compile FAIL (`playerList` undefined).

- [ ] **Step 3: Implement** `modules/world/player_list.go`:

```go
package world

import (
	"iter"
	"strconv"
	"strings"
)

// playerList ports TS EntityList/PlayerList (EntityList.ts:6-113 at the 244
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
// TS EntityList.ts:15-20,92.
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
// round-robin (TS calls super.next() with no args). TS EntityList.ts:100-112.
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

// getNextPid ports TS World.getNextPid (World.ts:1758-1773): derive the
// preferred pid window from the remote address. TS parses the raw address
// string (split on '.' / ':'), so the Go port does the same rather than
// using net.IP; any parse failure falls back to the plain round-robin.
// remoteAddr may be "host:port", a bare host, or "" (no client).
func getNextPid(l *playerList, remoteAddr string) int {
	host := remoteAddr
	if h, _, err := splitHostPort(remoteAddr); err == nil {
		host = h
	}
	if strings.Contains(host, ".") {
		// IPv4 — first available pid starting from (low octet % 20) * 100.
		octets := strings.Split(host, ".")
		if n, err := strconv.Atoi(octets[len(octets)-1]); err == nil {
			return l.nextPriority((n % 20) * 100)
		}
	} else if strings.Contains(host, ":") {
		// IPv6 — first available pid starting from (site prefix % 20) * 100.
		hextets := strings.Split(host, ":")
		if len(hextets) > 2 {
			if n, err := strconv.ParseInt(hextets[2], 16, 64); err == nil {
				return l.nextPriority((int(n) % 20) * 100)
			}
		}
	}
	return l.next()
}
```

(`splitHostPort` = thin wrapper over `net.SplitHostPort` that tolerates a missing port — implement alongside; for `[v6]:port` it strips brackets, matching how TS sees the raw socket address. Check how goscape's conn handler renders `RemoteAddr()` — `conn.RemoteAddr().String()` is `host:port` — and normalize there.)

- [ ] **Step 4: PASS run** — same command → PASS. Then `go vet ./modules/world/`.
- [ ] **Step 5: Commit** — `feat(world): playerList — 244 EntityList/PlayerList port with pid allocation [rev-244 B3]`.

### Task 2: Replace players array + playerLoop with playerList

**Files:**
- Modify: `modules/world/server.go` (fields :100 `players [2048]*Player`, :117 `playerLoop []*Player`; login slot-scan ~:1240-1262; removal ~:1387-1420; save flush ~:1521-1552)
- Modify: `modules/world/tick.go`, `modules/world/rebuild_worker.go` (:169-184), every other `playerLoop`/`s.players[` site
- Test: `modules/world/server_test.go` (or nearest existing fixture file) — iteration-order pin

TS contract: every `playerLoop`→`players` hunk in `git -C $HOME/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4 -- src/engine/World.ts` (the `for (const player of this.players)` rewrites, `getNextPid` at World.ts:1758-1773, the login insert at World.ts:940-961, `removePlayer` at World.ts:1643-1648, `getTotalPlayers` = `players.count` at World.ts:1737-1739).

- [ ] **Step 1: Write the failing pin test** — processing order is now pid order:

```go
// TS World.ts: 244 replaces the IP-bucketed playerLoop HashTable with
// PlayerList; per-tick processing iterates in pid order (EntityList.ts:37-48).
// Closes PORTING-EXCEPTION (gap-db-datastruct-4).
func TestTickIterationPidOrder(t *testing.T) {
	s := newTestServer(t) // use the existing world-test fixture helper (grep for the constructor used by neighboring tick tests)
	pA := loginTestPlayer(t, s, "alice") // helper that runs the production login insert path
	pB := loginTestPlayer(t, s, "bob")
	// force non-monotone insertion: remove+relogin alice into a HIGHER pid
	s.removePlayerInternal(pA)
	pA2 := loginTestPlayer(t, s, "alice")
	if pA2.pid <= pB.pid {
		t.Skipf("allocation did not produce out-of-order pids (pA2=%d pB=%d)", pA2.pid, pB.pid)
	}
	var seen []int
	for p := range s.players.all() {
		seen = append(seen, p.pid)
	}
	if !slices.IsSorted(seen) {
		t.Fatalf("iteration not in pid order: %v", seen)
	}
}
```

(Adapt the helper names to the file's existing fixtures — `grep -n "func newTestServer\|func.*TestServer" modules/world/*_test.go` first; do NOT invent a parallel fixture. The world-full path: assert the login path replies WORLD_FULL when `next()` returns −1 — extend the nearest existing login-flow test.)

- [ ] **Step 2: FAIL run** — `go test ./modules/world/ -run TestTickIterationPidOrder -count=1` → compile FAIL (`s.players.all` undefined).

- [ ] **Step 3: Implement.**
  - Replace the two Server fields with `players *playerList` (init `newPlayerList(2048)` in the constructor near server.go:414).
  - Login insert (server.go ~:1240-1262): replace the linear `for i` scan with `pid := getNextPid(s.players, <remote addr or "">)`; `pid == -1` → existing world-full reply path (TS World.ts:920-936: session log "Tried to log in - world full" + WORLD_FULL). Then `s.players.set(pid, p)`, `s.rsbuf.AddPlayer(int32(pid))`, `p.slot = pid` (field renamed in Task 3), uid composition unchanged (`composeUID` — TS World.ts:957).
  - Removal (server.go ~:1387-1420): `s.players.remove(p.slot)` replaces both the array nil-out and the playerLoop splice.
  - Every walk: `grep -rn "playerLoop" modules/world/ | grep -v _test` — rewrite each to `for p := range s.players.all()`. Where the old code copied the slice for off-tick use (rebuild_worker.go:169-184, save flush :1521-1552), build the copy from the iterator.
  - `getTotalPlayers()` (server.go ~:1063 consumer, reboot.go:127) → `s.players.count`.
  - The remote address for `getNextPid`: TS passes the client only when `isClientConnected` (World.ts:921-923) — pass `""` for headless/bot logins.
- [ ] **Step 4: PASS + sweep** — `grep -rn "playerLoop" modules/world/` → **zero hits** (tests included — update them to the new API, verifying each pinned contract against TS first); `go test ./modules/world/ -count=1` (≈2.5 min — not hung); whole-tree build.
- [ ] **Step 5: Commit** — `feat(world): PlayerList replaces playerLoop+slot array — pid-order ticks, IP-window allocation [rev-244 B3]` (body: closes gap-db-datastruct-4; cite World.ts:940-961/1758-1773).

### Task 3: slot→pid mechanical rename

**Files:**
- Modify: `modules/world/player.go` (:93 `slot` field, :717-718 `Slot()`), `modules/world/interaction.go` (:148), `modules/world/npc_interaction.go` (:1021), `modules/world/player_script.go` (HintPlayer :343), `modules/world/tick.go` (:344, :913 rsbuf feeds), zone-event emitters (`grep -rn "\.slot" modules/world pkg/zone pkg/rsbuf`), comments.

TS contract: the rename is wholesale at the pin (Player.ts:309 `pid`, PathingEntity.ts:531-540, Zone.ts:268/321, NetworkPlayer.ts:311-318, World.ts passim). Adopt-244-names rule.

- [ ] **Step 1:** `grep -rn "\bslot\b\|Slot()" modules/world pkg/rsbuf pkg/zone --include=*.go | grep -vi "hitmarkSlot\|lastSlot\|lastUseSlot\|lastTargetSlot\|interactCallSlot\|inv slot\|component slot" > $TMPDIR/slot-sweep.txt` — review every hit; classify player-identity uses (rename) vs inventory/component-slot uses (keep — they are genuine RS2 "slots").
- [ ] **Step 2:** Rename: `Player.slot` → `Player.pid`; `(p *Player) Slot()` → `Pid()`; update all player-identity call sites + the `+32768` faceEntity compositions + `HintPlayer(pid int)`. **Entity-interface caveat:** `(n *Npc) Slot()` (npc.go:236) returns the **nid** for a shared interface — if Player and Npc share an interface method, keep the interface method name unchanged (goscape-internal polymorphism over pid/nid; TS never unified these) and add a doc comment; rename only Player's own field/accessor and player-specific uses. Comments citing `player.slot` adopt `player.pid`.
- [ ] **Step 3:** Whole-tree build + full `go test ./modules/world/ ./pkg/rsbuf/ ./pkg/zone/ -count=1`. No behavior change expected — any test failure is a missed/over-eager rename.
- [ ] **Step 4: Commit** — `refactor(world): adopt 244 pid naming for player identity (was slot) [rev-244 B3]` (body: supersedes B2 HintArrow keep-slot row — PORTING.md note lands in Task 25).

## Slice 2 — Entity behavior deltas

### Task 4: setAnim priority `>=` — BOTH forks

**Files:**
- Modify: `modules/world/player_script.go` (:992-994), `modules/world/npc_masks.go` (:23-25) + both test files.

TS contract: `git … show 9aadcec4:src/engine/entity/Player.ts | sed -n '1850,1865p'` and `…:src/engine/entity/Npc.ts | sed -n '455,470p'`. 244: `anim == -1 || animId == -1 || SeqType.get(anim).priority >= SeqType.get(animId).priority`. The 225 form (`>` … `|| current.priority === 0`) differs for equal nonzero priorities (244 overwrites, 225 didn't) and for new-priority 0 vs current 0 (identical outcome) — the discriminating case is **equal nonzero priority**.

- [ ] **Step 1:** Failing pins, one per fork: seed an entity with an active anim of priority N>0, play another anim of priority N → 244 expects overwrite. Seed prerequisites per the existing anim tests (`grep -rn "TestPlayAnimation\|animID" modules/world/*_test.go | head`).
- [ ] **Step 2:** FAIL run (both packages’ tests).
- [ ] **Step 3:** Replace both gates with `>=` (drop the `priority == 0` arm); update citations to Player.ts:1857 / Npc.ts:461.
- [ ] **Step 4:** PASS + `go test ./modules/world/ -run 'Anim' -count=1`.
- [ ] **Step 5: Commit** — `feat(world): 244 setAnim priority >= in both entity forks [rev-244 B3]`.

### Task 5: player misc — run-energy /6, combat-level WORN rebuild, cleanup appearanceInv

**Files:**
- Modify: `modules/world/player_run.go` (:35), `modules/world/player_script.go` (combat-level rebuild ~:739-790), the Player cleanup func (`grep -n "func (p \*Player) cleanup\|Cleanup" modules/world/player.go`) + tests.

TS contracts: Player.ts:692 (`(baseLevels[AGILITY] / 6 | 0) + 8`); Player.ts:1820-1824 + 1838-1843 (`buildAppearance(InvType.WORN)` — Go analog `s.invTypes.Worn`, see handler_interface.go:141); Player.ts:471 (`this.appearanceInv = -1` added to cleanup).

- [ ] **Step 1:** Failing pins: (a) energy recovery for a known agility level (e.g. base 60: 225 gives 6+8=14, 244 gives 10+8=18 per step-rest tick); (b) combat-level-change rebuild passes the worn inv id even when `appearanceInv` was bound to something else (seed via SetAppearanceInv); (c) cleanup resets appearanceInv to −1.
- [ ] **Step 2:** FAIL → **Step 3:** implement the three line-level changes → **Step 4:** PASS + suite slice.
- [ ] **Step 5: Commit** — `feat(world): 244 run-energy /6, WORN appearance rebuild, cleanup appearanceInv reset [rev-244 B3]`.

### Task 6: Npc regen rework

**Files:**
- Modify: `modules/world/npc_script.go` (:488-510), `modules/world/npc.go` (:52 — delete `regenInterval`) + tests.

TS contract: `git … show 9aadcec4:src/engine/entity/Npc.ts | sed -n '510,535p'`:

```ts
const type = NpcType.get(this.type);
if (type.regenrate !== 0 && --this.regenClock <= 0) {
    this.regenClock = type.regenrate;
    /* levels move 1 toward baseLevels, both directions */
}
```

Countdown clock; `regenrate == 0` disables entirely; clock init 0 → **regen procs on the NPC's first turn alive**, then every `regenrate` ticks (OSRS-accurate per the TS comment). The 225 `regenInterval` snapshot field is gone — rate changes via `changeType` take effect on the next proc naturally.

- [ ] **Step 1:** Failing pins: first-turn proc (fresh NPC with a damaged level regens on turn 1); steady-state cadence (next proc exactly `regenrate` turns later); `regenrate == 0` never procs. Seed damaged levels before the turn under test.
- [ ] **Step 2:** FAIL → **Step 3:** rewrite `processNpcRegen` per the TS shape; delete `npc.go:52 regenInterval` and its initializer; keep the level-step body unchanged → **Step 4:** PASS + `go test ./modules/world/ -run 'Regen|Npc' -count=1`.
- [ ] **Step 5: Commit** — `feat(world): 244 npc regen countdown — first-turn proc, regenrate=0 disable [rev-244 B3]`.

### Task 7: GameMap nid-allocation hoist

**Files:**
- Modify: `modules/world/server.go` (spawn loop :608-621) + test.

TS contract: GameMap.ts:127-133 — at 244 the `new Npc(..., World.getNextNid(), ...)` construction is **above** the members gate, so nids advance even for skipped members-only NPCs on F2P worlds. (The obj hoist at :149-155 constructs-then-discards — no allocation side effect → NO-OP note in the commit.) First check where goscape assigns nids (`grep -n "nid" modules/world/npc_registry.go modules/world/server.go | head -20`) — the hoist must move the **nid consumption**, not necessarily the whole construction.

- [ ] **Step 1:** Failing pin: F2P world (NodeMembers=false) with a members-only NpcType between two F2P NPCs in spawn order → the third NPC's nid must reflect the skipped allocation (gap in the sequence).
- [ ] **Step 2:** FAIL → **Step 3:** hoist nid allocation above `shouldSpawnNpc` in the spawn loop (keep the gate for the add itself) → **Step 4:** PASS + suite slice.
- [ ] **Step 5: Commit** — `feat(world): 244 nid allocation precedes members gate in map spawns [rev-244 B3]`.

### Task 8: World misc — AFK chances, huntAll signature

**Files:**
- Modify: `modules/world/player.go` (:47-48 constants), `modules/world/npc_hunt.go` (:104 `huntAll`), its World-side callers (`grep -rn "huntAll" modules/world | grep -v _test`).

TS contracts: World.ts:631-636 — literals `0.0833` (normal) / `0.1666` (zonesAfk), replacing 1/24 and 1/12 (**both doubled**; pin the literals, not fractions); Npc.ts:249-252 — `huntAll()` takes no arg and derives `const hunt = HuntType.get(this.huntMode)` internally; World.ts:610-614 caller passes nothing.

- [ ] **Step 1:** Failing pins: constants equal the exact literals (`afkChance1 == 0.0833`, `afkChance2 == 0.1666`); huntAll derives from `huntMode` (call with a stale external hunt arg removed — signature change is compile-pinned).
- [ ] **Step 2:** FAIL → **Step 3:** set the literals (update the player.go:1101-1106 comment's TS citation to World.ts:631-636); change `huntAll(s *Server, hunt *objtype.HuntType)` → `huntAll(s *Server)` deriving hunt from `n.huntMode` at top (keep the existing nil/rate guards — TS Npc.ts:252-258) → **Step 4:** PASS + `go test ./modules/world/ -run 'Hunt|Afk' -count=1`.
- [ ] **Step 5: Commit** — `feat(world): 244 afk-chance literals + huntAll() derives hunt from huntMode [rev-244 B3]`.

### Task 9: Modal re-shape — rename + suspended-script survival

**Files:**
- Modify: `modules/world/player_script.go` (:1187-1281 — OpenMain/OpenChat/OpenSide/OpenMainSide) + tests.

TS contract: `git … show 9aadcec4:src/engine/entity/Player.ts | sed -n '1940,2030p'`. 244 deletes the `if (this.activeScript?.execution === COUNTDIALOG || PAUSEBUTTON) activeScript = null` block from **all four** modal-open methods, and renames `openChatModal→openChat` (Go is already `OpenChat` — NO-OP note), `openMainSideModal→openMainModalSide` (Go `OpenMainSide` → rename `OpenMainModalSide`).

- [ ] **Step 1:** Failing pin: a player with an activeScript suspended in COUNTDIALOG state opens a main modal → 244 expects the suspended script to SURVIVE (`p.activeScript != nil` after OpenMain). Find the Go suspended-clear blocks first: `grep -n "COUNTDIALOG\|CountDialog\|PauseButton" modules/world/player_script.go` — verify all four sites exist before testing (a missing site = a pre-B3 divergence; record it).
- [ ] **Step 2:** FAIL → **Step 3:** delete the four clear blocks; rename `OpenMainSide` → `OpenMainModalSide` (+ call sites; compile-checked) → **Step 4:** PASS + `go test ./modules/world/ -run 'Modal|Open' -count=1` (existing tests pinning the 225 clear behavior get updated — verify each against TS first, per the test-can-pin-a-bug rule).
- [ ] **Step 5: Commit** — `feat(world): 244 modal opens keep suspended scripts; OpenMainModalSide rename [rev-244 B3]`.

### Task 10: Overlay plumbing

**Files:**
- Modify: `modules/world/player.go` (fields near :398 modal block), `modules/world/player_script.go` (new `OpenOverlay`), the modal flush in `modules/world/player.go` (~:505-530 `refreshModal` region) + tests.

TS contract: Player.ts:358-359 (`overlay = -1; lastOverlay = -1`), Player.ts:1954-1964 (`openOverlay`: early-return on same com; `com === -1` → `clearComListeners(this.overlay)` — note: clears listeners for the **old** overlay; then `this.overlay = com`), NetworkPlayer.ts:192-195 (flush: `if (overlay !== lastOverlay) { write IfOpenOverlay(overlay); lastOverlay = overlay }`). Wire op: B2's `OpIfOpenOverlay` (158/2). The B4 script op will call `OpenOverlay`; until then it is engine-internal (exported, with a `// call site lands with B4's IF_OPENOVERLAY script op` note — same posture as B2's encoder row).

- [ ] **Step 1:** Failing pins: (a) OpenOverlay(X) then flush writes IF_OPENOVERLAY once and not again on the next flush; (b) OpenOverlay(-1) after X clears X's com listeners (seed an inv listener on X via the existing listener test helpers — see player.go:1383 `clearComListeners`); (c) OpenOverlay(X) twice → single state change.
- [ ] **Step 2:** FAIL → **Step 3:** add fields + method + flush branch (flush sits with the other modal flushes in the refreshModal region; mirror their writeOut pattern using `gameserver.OpIfOpenOverlay`) → **Step 4:** PASS + `-race ./modules/world/ -run Overlay`.
- [ ] **Step 5: Commit** — `feat(world): 244 overlay state + IF_OPENOVERLAY flush (script op lands B4) [rev-244 B3]`.

### Task 11: onLogin/onReconnect deltas

**Files:**
- Modify: `modules/world/login_resync.go` (:99-107 masks-resync block; onReconnect), reconnect handling in `modules/world/server.go`/`tick.go` (`grep -rn "reconnecting" modules/world/tick.go modules/world/server.go | grep -v _test`) + tests.

TS contracts: 244 deletes onLogin's `masks |= entitymask; masks |= APPEARANCE` resync (225 Player.ts:560-561 — gone at the pin: verify absence via `git … show 9aadcec4:src/engine/entity/Player.ts | sed -n '550,575p'`); onReconnect writes `UpdateUid192(this.pid, this.members)` (Player.ts:501); World reconnect drops `other.session = other.client.uuid` and calls `rsbuf.cleanupPlayerBuildArea(other.pid)` (World.ts:874-880).

- [ ] **Step 1:** Verify-first (B2 mandate): does login_resync.go:99-107 belong to onLogin or onReconnect in goscape? Read the function; TS deleted the block from **onLogin** (`login`/first-login path). Check whether B2's `010ee146` already writes UpdateUid192 on the reconnect path: `grep -rn "sendUpdatePid" modules/world/ | grep -v _test` (tick.go:344 is the login path — confirm whether onReconnect also reaches it).
- [ ] **Step 2:** Failing pins for whichever deltas are real after Step 1 (masks-resync absence on fresh login; UpdateUid192 bytes on reconnect resync; goscape has no `session` field on Player — the TS `other.session` deletion is expected NO-OP, record it).
- [ ] **Step 3:** Implement; update the login_resync.go "(k)" comment block (it cites the deleted TS lines).
- [ ] **Step 4:** PASS + `go test ./modules/world/ -run 'Login|Reconnect' -count=1`.
- [ ] **Step 5: Commit** — `feat(world): 244 login resync drops mask re-OR; reconnect UpdateUid192 verified [rev-244 B3]`.

### Task 12: Queue-cursor semantics — verify, then port or row

**Files:**
- Read: `modules/world/player.go` (:235 `queue []playerQueueRequest`), the queue-processing funcs (`grep -n "processQueues\|processWeakQueue\|fire" modules/world/player*.go | grep -iv test`)
- Possibly modify: those funcs + tests.

TS contract: Player.ts:892-906/910-919 — `const save = this.queue.cursor; … executeScript(...); this.queue.cursor = save;`. TS LinkList carries ONE shared cursor; a queued script running getqueue/clearqueue (B4 ops) re-enters the same list and clobbers the outer loop's position. goscape's queue is a **slice** — if processing iterates by index over a snapshot or by local index with explicit re-entry semantics, the bug cannot occur (NO-OP row); if it iterates shared mutable state, port an equivalent guard.

- [ ] **Step 1:** Read the Go iteration; write a characterization test: enqueue script A (which, when run, clears the queue — simulate by calling the same mutation getqueue/clearqueue will use) followed by script B; assert B's handling matches TS-244 (B is gone after a clear — TS cursor restore does not resurrect unlinked entries; the guard only preserves *position*).
- [ ] **Step 2:** If Go semantics already match: record decision row "queue-cursor save/restore — NO-OP, Go slice iteration is cursor-free" with the test as the pin. If not: port the positional guard.
- [ ] **Step 3:** PASS + commit — `test(world): pin 244 queue re-entry semantics (cursor guard NO-OP|ported) [rev-244 B3]`.

## Slice 3 — account_id / tracking

### Task 13: account_id threading

**Files:**
- Modify: `modules/world/player.go` (new field), `modules/world/client.go` (:87-88 — accountID already arrives via `resp.GetAccountId()`; wire into the Player at login), `modules/world/session_log.go` (SessionLog struct), `modules/world/server.go` (world-side addSessionLog + wealth dedup ~grep `sessionLogs`), `modules/world/player_script.go` (:1719 AddWealthEvent) + tests.

TS contracts: Player.ts:306 (`account_id = -1`), World.ts:1929-1934 (assignment from the login reply), Player.ts:633-642 + NetworkPlayer.ts:252-263 (addSessionLog/addWealthEvent: account_id + `client.uuid` / `'headless'` / `'disconnected'` session strings), SessionLog.ts:1-2, WealthEvent.ts:10-22 (account_id/account_session/recipient_id), World.ts:2250-2261 (signature), World.ts:2276-2284 (dedup key `{type, id: account_id, recipient: recipient_id, coord, tick}`).

- [ ] **Step 1:** Failing pins: session-log entries carry account_id (and the headless vs connected session-uuid split); wealth dedup keys collide on same account_id+tick (two events same player+tick dedup) and not across accounts.
- [ ] **Step 2:** FAIL → **Step 3:** add `Player.accountID int` (default −1); wire from the login response where the client's accountID already lands (client.go:87-88 — follow the existing flow into sendLoginOK/newPlayer); re-key the structs/signatures. The **logger bridge seam stays dormant**: `modules/world/logger_bridge.go` adapters keep emitting the existing `proto/events/v1` shapes — convert at the seam; do NOT touch proto files (B5/private-sibling; tracker row in Task 25).
- [ ] **Step 4:** PASS + `go test ./modules/world/ -run 'SessionLog|Wealth' -count=1`.
- [ ] **Step 5: Commit** — `feat(world): 244 account_id threading — session logs + wealth events re-keyed [rev-244 B3]`.

### Task 14: InputTrackingBlob + submit re-shape

**Files:**
- Modify: `modules/world/input_tracking.go` + its test; `modules/world/server.go` (submitInputTracking analog — `grep -rn "submitInputTracking\|InputTrack" modules/world | grep -v _test`); `modules/world/logger_bridge.go` (adapter only).

TS contracts: InputTrackingBlob.ts:1-11 (`{seq, data: base64, coord}` — seq starts at 1), InputTracking.ts:33-36 (`recordedBlobs: InputTrackingBlob[]`), :132-136 (`record` wraps `(rawData, recordedBlobs.length + 1, player.coord)`), :141-149 (submit passes username + session uuid + ALL blobs; 225 passed only `recordedBlobs[0]`), World.ts:2343-2352.

- [ ] **Step 1:** Failing pins: record() wraps with seq 1,2,3… + current coord + base64 payload; submit passes the full blob slice + correct identity strings.
- [ ] **Step 2:** FAIL → **Step 3:** add the struct; re-shape record/submit; adapter at the logger seam keeps the proto shape (base64-join or first-blob — match whatever the current seam sends so the dormant proto is untouched; document the adapter with the tracker-row reference).
- [ ] **Step 4:** PASS → **Step 5: Commit** — `feat(world): 244 InputTrackingBlob — seq/coord blobs, submit-all [rev-244 B3]`.

## Slice 4 — handshake + OnDemand

### Task 15: Rate-limit removal

**Files:**
- Modify: `modules/world/server.go` (fields :91-98, init :414-415, the address gate in handleLogin ~:996, the device gate further down — `grep -n "RateLimit\|LoginCache\|loginCache" modules/world/server.go`), `modules/world/config.go` (the `node_ratelimit_*` fields + flags + validation)
- Delete: `modules/world/login_ratelimit_test.go`, `pkg/util/ttlcache/` (sole consumer is server.go — re-verify: `grep -rln ttlcache --include=*.go .`)

TS contract: 244 deletes `loginAddressAttempts`/`loginDeviceAttempts`, both enforcement blocks, and the `NODE_RATELIMIT_*` envs (the World.ts diff hunks at old :2106-2117 and :2160-2173).

- [ ] **Step 1:** Delete gates+fields+config+tests+package. **Step 2:** whole-tree build + `go vet` + full modules/world suite. **Step 3:** Commit — `feat(world): remove 225 world-side login rate limiting (244; replacement lands B5) [rev-244 B3]` (body: tracker row in Task 25 — protection gap explicit until B5's login-server 3-in-5s + hop timer).

### Task 16: OnDemand component — queues + request parsing

**Files:**
- Create: `modules/world/ondemand.go`, `modules/world/ondemand_test.go`

TS contract: `git … show 9aadcec4:src/engine/OnDemand.ts | cat -n` (123 lines, read in full). Requests are 4-byte frames: `archive g1, file g2, priority g1`; `archive > 3 || priority > 2` → close; priority 2→urgent, 1→extra, else ingame (OnDemand.ts:42-85).

- [ ] **Step 1:** Failing tests:

```go
func TestOnDemandRequestParsing(t *testing.T) {
	od := newOnDemand(nil) // cache nil-able for parse tests
	closed := false
	// feed two packed requests in one buffer + a partial third
	buf := []byte{0, 0, 1, 2 /*urgent*/, 3, 0, 5, 0 /*ingame*/, 1}
	od.onClientData(testODClient(&closed), buf)
	od.mu.Lock()
	defer od.mu.Unlock()
	if len(od.urgent) != 1 || len(od.ingame) != 1 || len(od.extra) != 0 {
		t.Fatalf("queues: urgent=%d extra=%d ingame=%d", len(od.urgent), len(od.extra), len(od.ingame))
	}
	// partial trailing byte must remain buffered, not consumed
}

func TestOnDemandRejectsBadRequest(t *testing.T) {
	for _, bad := range [][]byte{{4, 0, 0, 0}, {0, 0, 0, 3}} { // archive>3, priority>2
		closed := false
		od := newOnDemand(nil)
		od.onClientData(testODClient(&closed), bad)
		if !closed {
			t.Errorf("bad request %v did not close the connection", bad)
		}
	}
}
```

(Define the writer/closer seam as a small interface — `type odClient interface { send([]byte) error; close() }` — so tests don't need real sockets; the production adapter wraps `*client` in Task 18.)

- [ ] **Step 2:** FAIL → **Step 3:** implement the struct (three `[]odRequest` queues + `sync.Mutex` + `*filestream.FileStream` + its own mutex), `onClientData` consuming whole 4-byte frames from the per-connection buffer (loop `for available >= 4` — OnDemand.ts:52-84) → **Step 4:** PASS + `-race -run OnDemand`.
- [ ] **Step 5: Commit** — `feat(world): 244 OnDemand request queues + 4-byte frame parsing [rev-244 B3]`.

### Task 17: OnDemand cycle + send + lifecycle

**Files:**
- Modify: `modules/world/ondemand.go` (+test); `modules/world/server.go` (service start/stop wiring — alongside the tick goroutine startup)

TS contract: OnDemand.ts:18-40 (cycle drains urgent→extra→ingame FIFO, re-arms every 50ms), :87-120 (send: `cache.read(archive + 1, file)`; chunk loop `remaining = min(500, left)`; 6-byte header `p1 archive, p2 file, p2 totalLen, p1 part`; missing file → single 6-byte frame with len 0 part 0).

- [ ] **Step 1:** Failing chunking pins — payload sizes 0(reject)/1/500/501/1000 against a synthetic FileStream fixture (write archives via `filestream.New(t.TempDir(), true, false)` + `Write(1, 7, data, version)`; B1 round-trip tests show the API):

```go
// 501 bytes → two frames: hdr(0,7,501,0)+500B, hdr(0,7,501,1)+1B. OnDemand.ts:94-110.
```

Assert exact header bytes and chunk boundaries on a recording odClient.

- [ ] **Step 2:** FAIL → **Step 3:** implement `cycle()` (drain under the queue mutex — pop-all then send outside the lock to keep enqueue latency flat; FileStream reads under its own mutex) + `send()`; wire a 50ms `time.Tick` goroutine into the world service Start/stop path (follow the tick-goroutine's lifecycle pattern; stop via the server's existing shutdown signal) → **Step 4:** PASS + `-race -run OnDemand` + whole-tree build.
- [ ] **Step 5: Commit** — `feat(world): 244 OnDemand 50ms cycle + 500-byte chunked sends [rev-244 B3]`.

### Task 18: Login handshake re-shape + OnDemand routing

**Files:**
- Modify: `modules/world/server.go` (delete seed send :883-896; conn-loop state switch :935-961 gains the Ondemand case), `modules/world/client.go` (state consts :29-34 + sendLoginOK :152-178), handleLogin dispatch (server.go :973+) + tests.

TS contracts (verify each against `git … show 9aadcec4:src/engine/World.ts | sed -n '2110,2250p' | cat -n`):
- op **14**: 1-byte payload (loginServer, discarded) → reply `8×0x00`, then `0x00`, then 8-byte seed (`p4(rand & 0xffffff)`, `p4(rand)`) — World.ts:2143-2156. One 17-byte write in Go (TS's three sends are one TCP sequence).
- op **15**: 0-byte payload → `client.state = 2` + reply `8×0x00` — World.ts:2240-2242; conn loop routes state≠login/game data to `onDemand.onClientData` (TcpServer.ts:30-37).
- op **16/18**: NO framing change; goscape's plaintext-revision→reply-6 already matches — verify, expect no diff.
- Login-OK: `staffModLevel >= 2` → byte **19**; `>= 1` → 18; else 2 — World.ts:943-949 (client.go:170 currently has only the `>= 1` split).

- [ ] **Step 1:** Failing byte-exact pins: (a) connect produces NO unsolicited bytes (the 225 seed-on-accept is gone — assert first server bytes come only after op 14); (b) op-14 reply = `00×8, 00, s0,s1,s2…s7` with `s0 == 0` high byte masked (assert `seed[0] == 0` per the 24-bit mask — bytes 9..16 pattern); (c) op-15 → 8 zero bytes + subsequent 4-byte frames land in the OnDemand queues; (d) supermod login (staffModLevel 2) → reply byte 19. Seed prerequisites: reuse the existing login-flow test harness (`grep -rn "handleLogin\|loginTest" modules/world/*_test.go | head`); reject-path tests must pass the earlier gates (valid revision/CRC) so the byte under test discriminates.
- [ ] **Step 2:** FAIL → **Step 3:** implement; existing tests pinning the connect-time seed get updated (they pinned 225 — verify against TS first) → **Step 4:** PASS + full `go test ./modules/world/ -count=1` + `-race -run 'Login|OnDemand|Handshake'`.
- [ ] **Step 5: Commit** — `feat(world): 244 login handshake — seed in op-14 reply, op-15 OnDemand entry, supermod byte 19 [rev-244 B3]`.

## Slice 5 — cache + HTTP delivery

### Task 19: CrcTable from FileStream

**Files:**
- Modify: `pkg/cache/crctable.go` (`MakeCRCs`) + `pkg/cache/crctable_test.go`

TS contract: `git … show 9aadcec4:src/cache/CrcTable.ts | cat -n` (33 lines): `count = OnDemand.cache.count(0)`; for each, `read(0, i)` → `p4(getcrc(jag))`, missing → `p4(0)`; `CrcBuffer32 = getcrc(buffer)` (goscape dropped CrcBuffer32 as unused — keep dropped, note in commit). Module-init guard → Go's existing world-start + `::reload` call sites (no new call sites).

- [ ] **Step 1:** Failing fixture test: build a temp FileStream with 3 archive-0 files (one gap), `MakeCRCs(dir)` → snapshot Table = [crc0, 0, crc2], Bytes = the 12-byte buffer. Keep the existing atomic-swap concurrency test green.
- [ ] **Step 2:** FAIL → **Step 3:** rewrite `MakeCRCs` internals to `filestream.New(cachePath, false, true)` + Count/Read loop (open-read-close per call, matching the call cadence; or accept an injected FileStream if the world module shares — keep the signature `MakeCRCs(cachePath string)` so callers don't change) → **Step 4:** PASS + `go test ./pkg/cache/ ./modules/world/ -count=1` (world login CRC tests consume the snapshot — the B1-format-window skips may extend here; record any new skip with the same `rev244-b1-format-window` tag).
- [ ] **Step 5: Commit** — `feat(cache): 244 CRC table built from FileStream archive 0 [rev-244 B3]`.

### Task 20: PreloadedPacks deletion + consumer survey

**Files:**
- Delete: `pkg/cache/preloaded.go`, `pkg/cache/preloaded_test.go`
- Modify: every consumer — `grep -rn "Preload\|PRELOADED" modules pkg cmd --include=*.go | grep -v _test` first.

TS contract: PreloadedPacks.ts deleted upstream (−41); `preloadClient()` call removed from World.reload (World.ts diff @310-314).

- [ ] **Step 1:** Survey consumers. Expected: world-start/reload `PreloadClient` calls (delete); ondemand maps/`.mid` serving (the `.mid` HTTP route dies in Task 21 — if the maps route reads the preload snapshot, re-verify the route's original 225 citation: 225 web.ts had NO maps route, so a goscape-specific addition gets its own decision row — check `modules/ondemand/handler.go` + `maps_test.go` provenance comments before touching).
- [ ] **Step 2:** Delete + rewire/remove consumers per survey → **Step 3:** whole-tree build + full suite → **Step 4:** Commit — `feat(cache): delete PreloadedPacks (244 removes preload path) [rev-244 B3]` (body: per-consumer disposition list).

### Task 21: HTTP asset routes from FileStream

**Files:**
- Modify: `modules/ondemand/handler.go`, `modules/ondemand/config.go` (cache-path field if absent), `modules/ondemand/ondemand.go` (own FileStream instance) + tests.

TS contract: web.ts:63-84 — `/title`→read(0,1), `/config`→(0,2), `/interface`→(0,3), `/media`→(0,4), **`/versionlist`→(0,5)** (replaces `/models`), `/textures`→(0,6), `/wordenc`→(0,7), `/sounds`→(0,8); `.mid` route REMOVED; `/ondemand.zip` → file `data/pack/ondemand.zip`, `/build` → file `data/pack/server/build` (web.ts:78-81).

- [ ] **Step 1:** Failing route tests against a temp FileStream fixture (httptest against the module's mux; 404 for absent archive files mirrors TS's `!` panic→500? — NO: TS `OnDemand.cache.read(0,1)!` with a missing file throws → Bun 500; pin goscape to 404-on-missing as a deliberate divergence ONLY if the existing handler already 404s — otherwise mirror with 500; decide by reading the current handler's missing-file posture and keep it consistent; record the row).
- [ ] **Step 2:** FAIL → **Step 3:** the module opens its own read-only FileStream (+mutex) at Start; routes read under it; `/models` + `.mid` routes deleted; `/versionlist`, `/ondemand.zip`, `/build` added → **Step 4:** PASS + `-race ./modules/ondemand/`.
- [ ] **Step 5: Commit** — `feat(ondemand): 244 asset routes read FileStream cache; versionlist replaces models; .mid removed [rev-244 B3]`.

### Task 22: rs2.cgi token + WS OnDemand gate + origin-check exception

**Files:**
- Modify: `modules/ondemand/rs2cgi.go` + `templates/`, `modules/ondemand/websocket.go`, `modules/ondemand/config.go` + tests.

TS contract: web.ts:101-104 (`per_deployment_token: WEB_SOCKET_TOKEN_PROTECTION ? getPublicPerDeploymentToken() : ''` — B1 ported the primitive as `pkg/util/pemtoken`); web.ts:165-176 (WS message routing: `state === 2` → OnDemand only when `NODE_WS_ONDEMAND`, else terminate); web.ts:152-158 (WS connect-time seed send deleted); web.ts:125-152 (origin check commented out upstream — **goscape keeps its origin check**: add `PORTING-EXCEPTION (rev244-b3-ws-origin, keep origin check vs upstream TODO-comment-out)` marker + row).

- [ ] **Step 1:** Failing pins: rs2.cgi template renders the token when the config gate is on, empty when off; WS state-2 frames reach the OnDemand queues only under the new config; WS open sends no seed.
- [ ] **Step 2:** FAIL → **Step 3:** implement (config fields: `web_socket_token_protection`, `ws_ondemand` — follow the module's existing flag-registration pattern in config.go) → **Step 4:** PASS.
- [ ] **Step 5: Commit** — `feat(ondemand): 244 per-deployment token + WS ondemand gate; origin check kept (PORTING-EXCEPTION) [rev-244 B3]`.

## Slice 6 — midi, residuals, audit

### Task 23: MidiPack registry + producers

**Files:**
- Create: `modules/world/midi_pack.go` (+test)
- Modify: `modules/world/midi_encoders.go` (:29-35 stub), `modules/world/player_script.go` (PlaySong :1599+, PlayJingle nearby), `modules/world/config.go`/server start (load call)

TS contract: PackFile.ts:206 (`MidiPack = new PackFile('midi', …)` reading `${BUILD_SRC_DIR}/pack/midi.pack` — `id=name` lines); Player.ts:1919-1933:

```ts
playSong: id = MidiPack.getByName(name.toLowerCase().replaceAll(' ', '_').replace(/[^a-z0-9_-]/g, ''));
          if (id !== -1) write(MidiSong(id))
playJingle: id = MidiPack.getByName(name.toLowerCase()); if (id !== -1) write(MidiJingle(id, delay))
```

- [ ] **Step 1:** Failing pins: registry parses `0=scape_main\n1=ghost_town` fixture → `midiIDByName("scape_main") == 0`; absent file → every lookup −1; PlaySong("Scape Main!") normalizes to `scape_main` and writes MIDI_SONG with id 0 via the B2 encoder; PlayJingle keeps spaces (lowercase only). Note the asymmetry is TS-faithful (songs strip/underscore, jingles don't).
- [ ] **Step 2:** FAIL → **Step 3:** load `<cfg.ContentPath>/pack/midi.pack` at server start into a `map[string]int` (the `.pack` line format: `<id>=<name>` — verify against the Content checkout's `pack/midi.pack` head); replace the stub; **remove the `PORTING-EXCEPTION (rev244-b2-midi-window)` marker** (closure: code-side; live verification B6 — Task 25 records it) → **Step 4:** PASS + grep marker count check.
- [ ] **Step 5: Commit** — `feat(world): 244 MidiPack name→id registry closes midi window code-side [rev-244 B3]`.

### Task 24: buildArea.clear wiring + residual NO-OP batch

**Files:**
- Modify: Player cleanup (call `p.buildArea.clear(false)` — build_area.go:59), `modules/world/login_resync.go` (:54-57 comment)
- Read-only verification: NetworkPlayer writeInner analog, getInventoryFromListener analog, validateDistanceWalked analog, processHuntFollow guard, pkg/zone lists, GameMap CSV loader (pkg/gamemap), shop `item?.id`, LinkList style.

TS contracts: Player.ts:452 (`cleanup` → `clear(false)`) — identical at both pins (pre-existing 225 gap; the handoff flag is the citation); Player.ts:541 `clear(true)` is a TS no-op (BuildArea.ts:23-29) — login_resync.go's "(c)" comment is consistent, fix its citation only.

- [ ] **Step 1:** Wire `clear(false)` into the Go cleanup path + pin (cleanup empties activeZones/loadedZones/mapsquares).
- [ ] **Step 2:** Run the residual verifications; record each NO-OP verdict (with the checked Go site) in a scratch list for Task 25's rows. Any verification that finds a REAL divergence becomes its own follow-up fix (stop, report, decide — don't silently fix unrelated gaps).
- [ ] **Step 3:** PASS + Commit — `feat(world): wire buildArea.clear(false) into player cleanup; B3 residual NO-OP verifications [rev-244 B3]`.

### Task 25: Gates + PORTING.md §B3 audit trail

**Files:**
- Modify: `PORTING.md` (new `### B3 — engine core` subsection under the audit trail)

- [ ] **Step 1: Full gates** — `CGO_ENABLED=0 go build -trimpath ./...`; `go vet ./...`; `go test ./... -count=1` (capture exit code); `CGO_ENABLED=1 go test -race ./modules/world/ ./modules/ondemand/ ./pkg/cache/ -count=1`.
- [ ] **Step 2: Write the audit subsection** — mirror the B1/B2 shape: decision rows (the spec's taxonomy: dead-at-pin ×2, platform NOT-PORTED, NO-OP batch with verified Go sites, B2-no-double-apply confirmations, ws-origin PORTING-EXCEPTION, queue-cursor verdict, HTTP missing-file posture) + the **correspondence table** mapping every file of the scope diff (`git -C ../Engine-TS diff --numstat e1dea19f..9aadcec4 -- src/engine src/server/tcp/TcpServer.ts src/web.ts src/app.ts src/cache/CrcTable.ts src/cache/PreloadedPacks.ts ':!src/engine/script'`) to a commit or decision row — no unmapped hunks. Tracker rows: (1) rate-limit gap → B5; (2) world_heartbeat → B5; (3) map-delivery+midi+format windows close at **B6** + umbrella smoke-gate amendment (user decision); (4) 244 reference cache = B6 prerequisite; (5) logger/friends shapes → B5/private. Supersede note on B2's HintArrow keep-slot row. Close `gap-db-datastruct-4` (PlayerList port) — move the closed row per Tracking conventions.
- [ ] **Step 3: Marker audit** — `grep -rn "PORTING-EXCEPTION" modules pkg cmd | wc -l` (−1 midi-window, +1 ws-origin, −1 gap-db-datastruct-4 if marker-carried — verify which carry markers vs rows only).
- [ ] **Step 4: Commit** — `docs(porting): rev-244 B3 audit trail — engine-core correspondence + window/tracker rows [rev-244 B3]`.

---

## Self-review checklist (run after Task 25)

- Every spec §Design item maps to a task: §1→T1-3, §2→T4-12, §3→T2/8/11/13, §4→T15/18, §5→T16-17, §6→T19-20, §7→T21-22, §8→T23, §9→T13-14, §10→T24, taxonomy/trackers→T25.
- Whole-bundle integration review (subagent) before declaring done; then update the resume handoff for B4.
