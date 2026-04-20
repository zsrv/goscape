# Sub-spec 4a: Stats, AFK, Inv Listeners — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace three no-op stub functions in `Player.processOut()` with working diff-driven update loops (stats, run energy, inventories), add AFK zone state tracking, and wire UpdateInvStopTransmit into modal-close. The fourth stub (`updateZones`) remains for sub-spec 4b.

**Architecture:** Three new opcodes on `pkg/io/protocol/game/server`, three small sender files in `modules/world/`, rewrites of `updateStats`/`updateInvs`/`updateAfkZones`, one new `afkzone.go` helper module, and a modal-close hook in `encodeOut`. No new packages.

**Tech Stack:** Go 1.26, `pkg/io/packet.Packet` (RS2 binary buffer), existing `inventory.Inventory`, `rsbuf.Renderer` (unchanged). Test-driven; each task commits independently.

**Build prefix:** All `go` commands below use `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` (required by the global CLAUDE.md).

**Commit policy:** All commits use `git commit --no-gpg-sign`. Conventional-commits style (`feat`, `test`, `refactor`, `docs`).

**Spec reference:** `docs/superpowers/specs/2026-04-19-stats-afk-invs-design.md`.

---

## File Structure

**Create:**
- `modules/world/stat_update.go` — UpdateStat + UpdateRunEnergy packet senders
- `modules/world/stat_update_test.go` — sender byte-level tests + updateStats diff loop tests
- `modules/world/inv_stop_transmit.go` — UpdateInvStopTransmit sender
- `modules/world/inv_update_test.go` — updateInvs listener-routing tests
- `modules/world/afkzone.go` — afkZones state + pack/unpack + intersect helpers + updateAfkZones
- `modules/world/afkzone_test.go` — AFK zone state tests
- `modules/world/modal_close_test.go` — encodeOut stop-transmit hook tests

**Modify:**
- `pkg/io/protocol/game/server/prot.go` — 3 new opcodes
- `modules/world/player.go` — extend `InventoryListener`, add `afkZones`/`lastAfkZone` fields, fix `newPlayer` sentinel init, rewrite `updateStats`/`updateInvs`, replace `updateAfkZones` stub, hook `encodeOut` modal-close
- `modules/world/server.go` — add `invs` map field + init
- `modules/world/inv_update.go` — rename `sendUpdateInvFull` → `sendUpdateInvFullCom`, rename param `invId` → `com`, drop stale comment

---

## Task 1: Prerequisites — opcodes, struct fields, sentinel init

Adds everything structural so later tasks compile cleanly. No new logic yet.

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`
- Modify: `modules/world/player.go`
- Modify: `modules/world/server.go`

- [ ] **Step 1.1: Add the three new opcodes**

Edit `pkg/io/protocol/game/server/prot.go`, inside the existing `var (...)` block (after `OpNpcInfo`):

```go
OpUpdateStat            = Op{Opcode: 44, PayloadSize: 6}
OpUpdateRunEnergy       = Op{Opcode: 68, PayloadSize: 1}
OpUpdateInvStopTransmit = Op{Opcode: 15, PayloadSize: 2}
```

- [ ] **Step 1.2: Extend `InventoryListener`**

Edit `modules/world/player.go` lines 14-19 to:

```go
// InventoryListener associates a player-visible UI component with an inventory.
type InventoryListener struct {
	Type      int  // InvType id
	Com       int  // UI component id
	Source    int  // -1 = world-shared inventory, else owning player's slot
	FirstSeen bool // true until the first UpdateInvFull; then false
}
```

- [ ] **Step 1.3: Add `afkZones` and `lastAfkZone` fields to Player**

In the session-flags block of `Player` (around line 122), add:

```go
	// === AFK zones (sub-spec 4a) ===
	afkZones    [2]int32
	lastAfkZone int
```

- [ ] **Step 1.4: Fix sentinel init in `newPlayer`**

In `newPlayer` (around line 302, before the closing brace), insert:

```go
	}
	// Sentinel values so the first tick of updateStats emits all 21 UpdateStat
	// packets. stats[i] is int32 (always >= 0 in gameplay); levels[i] is uint8
	// (max real value 99). -1 and 255 are unreachable legitimate values.
	for i := 0; i < 21; i++ {
		p.lastStats[i] = -1
		p.lastLevels[i] = 255
	}
	return p
```

Replace the current `return &Player{ ... }` literal with a named `p := &Player{ ... }` and append the loop + `return p` as shown. Exact surgery:

Before (ends like):
```go
		faceSquareX:    -1,
		faceSquareZ:    -1,
	}
}
```

After:
```go
		faceSquareX:    -1,
		faceSquareZ:    -1,
	}
	for i := 0; i < 21; i++ {
		p.lastStats[i] = -1
		p.lastLevels[i] = 255
	}
	return p
}
```

And change the line at the top of the function from `return &Player{` to `p := &Player{`.

- [ ] **Step 1.5: Add `invs` map to Server**

Edit `modules/world/server.go` to add a field to the Server struct:

```go
	// invs is world-shared inventories (banks, shops) keyed by InvType id.
	// Empty until populated by non-4a code. Listeners with Source==-1 read from here.
	invs map[int]*inventory.Inventory
```

And in the constructor/initialiser (wherever `players` is initialised, or in `NewServer`), ensure:

```go
invs: make(map[int]*inventory.Inventory),
```

Add the import `"github.com/zsrv/goscape/pkg/inventory"` if not already present.

- [ ] **Step 1.6: Build to verify compilation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: exits 0 with no output.

- [ ] **Step 1.7: Run existing tests to verify nothing regressed**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS across all packages.

- [ ] **Step 1.8: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go modules/world/player.go modules/world/server.go
git commit --no-gpg-sign -m "feat(world): scaffold sub-spec 4a — opcodes, struct fields, sentinel init

- Add OpUpdateStat (44/6), OpUpdateRunEnergy (68/1), OpUpdateInvStopTransmit (15/2)
- Add FirstSeen to InventoryListener
- Add afkZones[2] + lastAfkZone to Player
- Add Server.invs for world-shared inventories
- Fix lastStats/lastLevels sentinel init so first tick emits all 21 stats"
```

---

## Task 2: UpdateStat + UpdateRunEnergy senders

**Files:**
- Create: `modules/world/stat_update.go`
- Create: `modules/world/stat_update_test.go`

- [ ] **Step 2.1: Write the failing tests**

Create `modules/world/stat_update_test.go`:

```go
package world

import (
	"bytes"
	"testing"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func TestSendUpdateStatWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	sendUpdateStat(p, 3, 100, 10)
	p.client.flushWrite()

	// Expected wire:
	//   encrypted opcode = (44 + enc.GetNext()) & 0xff
	//   payload = p1(3) p4(100/10=10) p1(10) = [3, 0, 0, 0, 10, 10]
	want := []byte{
		byte((int(gameserver.OpUpdateStat.Opcode) + int(enc.GetNext())) & 0xff),
		3,
		0, 0, 0, 10,
		10,
	}
	got := drainConn(t, cc)
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

func TestSendUpdateRunEnergyWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	sendUpdateRunEnergy(p, 10000)
	p.client.flushWrite()

	want := []byte{
		byte((int(gameserver.OpUpdateRunEnergy.Opcode) + int(enc.GetNext())) & 0xff),
		100, // 10000 / 100
	}
	got := drainConn(t, cc)
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}
```

Add a helper at the end of the same file (if `drainConn` doesn't already exist in the package — check with `grep -n 'func drainConn' modules/world/`. If missing, add it here):

```go
// drainConn reads everything currently in the pipe. Assumes the pipe was
// flushed before this call.
func drainConn(t *testing.T, c net.Conn) []byte {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 4096)
	n, _ := c.Read(buf)
	return buf[:n]
}
```

Add corresponding imports: `"net"`, `"time"`.

- [ ] **Step 2.2: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestSendUpdateStat|TestSendUpdateRunEnergy' -v`
Expected: FAIL with `undefined: sendUpdateStat` / `undefined: sendUpdateRunEnergy`.

- [ ] **Step 2.3: Implement the senders**

Create `modules/world/stat_update.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateStat writes one UpdateStat packet for the given skill slot.
// Wire: p1(stat) p4(exp/10) p1(level). XP is lossy on the wire (divided by 10).
func sendUpdateStat(p *Player, stat, exp, level int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(stat))
	buf.P4(uint32(exp / 10))
	buf.P1(uint8(level))
	p.writeOut(gameserver.OpUpdateStat, buf.Bytes())
}

// sendUpdateRunEnergy writes one UpdateRunEnergy packet.
// Internal energy is 0-10000; the wire value is energy/100 (0-100 byte).
func sendUpdateRunEnergy(p *Player, energy int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(energy / 100))
	p.writeOut(gameserver.OpUpdateRunEnergy, buf.Bytes())
}
```

- [ ] **Step 2.4: Run the tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestSendUpdateStat|TestSendUpdateRunEnergy' -v`
Expected: PASS.

- [ ] **Step 2.5: Commit**

```bash
git add modules/world/stat_update.go modules/world/stat_update_test.go
git commit --no-gpg-sign -m "feat(world): add sendUpdateStat and sendUpdateRunEnergy

UpdateStat: p1(stat) p4(exp/10) p1(level) — XP lossy on wire.
UpdateRunEnergy: p1(energy/100) — internal 0-10000, wire 0-100."
```

---

## Task 3: `updateStats()` diff loop

**Files:**
- Modify: `modules/world/player.go` (the `updateStats` stub)
- Append: `modules/world/stat_update_test.go` (add diff-loop tests)

- [ ] **Step 3.1: Write failing tests**

Append to `modules/world/stat_update_test.go`:

```go
func TestUpdateStatsFiresOnChange(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// Match all stats so only the target index diverges from sentinel.
	for i := 0; i < 21; i++ {
		p.lastStats[i] = p.stats[i]
		p.lastLevels[i] = p.levels[i]
	}
	p.lastRunEnergy = p.runenergy // isolate the stat loop from run-energy emission
	p.stats[3] = 100
	p.levels[3] = 10

	p.updateStats()
	p.client.flushWrite()

	got := drainConn(t, cc)
	if len(got) == 0 {
		t.Fatal("expected UpdateStat packet, got nothing")
	}
	// Second call: lastStats/lastLevels now match; should emit nothing.
	p.updateStats()
	p.client.flushWrite()
	after := drainConn(t, cc)
	if len(after) != 0 {
		t.Errorf("second call should emit nothing; got %d bytes", len(after))
	}
}

func TestUpdateStatsRunEnergyCoarseGrain(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// All stats already match (isolate runenergy).
	for i := 0; i < 21; i++ {
		p.lastStats[i] = p.stats[i]
		p.lastLevels[i] = p.levels[i]
	}
	p.runenergy = 10000
	p.lastRunEnergy = -1 // default from newPlayer

	p.updateStats()
	p.client.flushWrite()
	first := drainConn(t, cc)
	if len(first) == 0 {
		t.Fatal("expected UpdateRunEnergy packet on first tick")
	}

	// Bump by 50: wire value (100) unchanged.
	p.runenergy = 10050
	p.updateStats()
	p.client.flushWrite()
	quiet := drainConn(t, cc)
	if len(quiet) != 0 {
		t.Errorf("wire value unchanged; expected no packet, got %d bytes", len(quiet))
	}

	// Bump across boundary: wire value changes from 100 → 101.
	p.runenergy = 10100
	p.updateStats()
	p.client.flushWrite()
	loud := drainConn(t, cc)
	if len(loud) == 0 {
		t.Error("wire value crossed boundary; expected packet, got nothing")
	}
}

func TestUpdateStatsFirstTickEmitsAll21(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// Fresh player: stats/levels all zero; last* sentinel -1/255.
	// runenergy = 10000, lastRunEnergy = -1.
	p.updateStats()
	p.client.flushWrite()
	got := drainConn(t, cc)
	// Each UpdateStat is 7 bytes (1 opcode + 6 payload). Plus 1 UpdateRunEnergy
	// (1 + 1 = 2 bytes). Total: 21*7 + 2 = 149.
	if len(got) != 149 {
		t.Errorf("first tick: got %d bytes, want 149 (21 stats + 1 runenergy)", len(got))
	}
}
```

- [ ] **Step 3.2: Run the tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestUpdateStats -v`
Expected: FAIL — `updateStats` is currently a no-op stub that sends nothing.

- [ ] **Step 3.3: Rewrite `updateStats`**

In `modules/world/player.go`, replace the existing `func (p *Player) updateStats()    {}` stub with:

```go
func (p *Player) updateStats() {
	for i := 0; i < 21; i++ {
		if p.stats[i] != p.lastStats[i] || p.levels[i] != p.lastLevels[i] {
			sendUpdateStat(p, i, int(p.stats[i]), int(p.levels[i]))
			p.lastStats[i] = p.stats[i]
			p.lastLevels[i] = p.levels[i]
		}
	}
	if p.runenergy/100 != p.lastRunEnergy/100 {
		sendUpdateRunEnergy(p, p.runenergy)
		p.lastRunEnergy = p.runenergy
	}
}
```

- [ ] **Step 3.4: Run the tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestUpdateStats -v`
Expected: PASS.

- [ ] **Step 3.5: Run full test suite — make sure nothing regressed**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 3.6: Commit**

```bash
git add modules/world/player.go modules/world/stat_update_test.go
git commit --no-gpg-sign -m "feat(world): implement Player.updateStats diff loop

Iterates 21 skills; emits UpdateStat on stat or level change.
Emits UpdateRunEnergy only when floor(energy/100) crosses a boundary."
```

---

## Task 4: UpdateInvStopTransmit sender

Small sender — deliberately a separate task from the hook so the latter can land with tests.

**Files:**
- Create: `modules/world/inv_stop_transmit.go`

- [ ] **Step 4.1: Implement the sender**

Create `modules/world/inv_stop_transmit.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateInvStopTransmit tells the client to stop receiving updates for
// the given UI component. Fired when a modal containing the component closes.
// Wire: p2(component).
func sendUpdateInvStopTransmit(p *Player, com int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	p.writeOut(gameserver.OpUpdateInvStopTransmit, buf.Bytes())
}
```

- [ ] **Step 4.2: Build to verify compilation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: exits 0.

- [ ] **Step 4.3: Commit**

```bash
git add modules/world/inv_stop_transmit.go
git commit --no-gpg-sign -m "feat(world): add sendUpdateInvStopTransmit

p2(component). Fired when a modal holding the component closes."
```

---

## Task 5: Rename `sendUpdateInvFull` → `sendUpdateInvFullCom` + rewrite `updateInvs`

**Files:**
- Modify: `modules/world/inv_update.go`
- Modify: `modules/world/player.go` (the `updateInvs` stub)
- Create: `modules/world/inv_update_test.go`

- [ ] **Step 5.1: Write the failing tests**

Create `modules/world/inv_update_test.go`:

```go
package world

import (
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func newInvListenerTestPlayer(t *testing.T, s *Server, slot int) (*Player, net.Conn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.slot = slot
	p.invs = map[int]*inventory.Inventory{}
	s.players[slot] = p
	return p, cc
}

func TestUpdateInvsFirstSeenFires(t *testing.T) {
	s := newTestServer(t)
	s.players = make(map[int]*Player)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Update = false // first-seen listener should override dirty==false.
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 2, FirstSeen: true},
	}

	viewer.updateInvs()
	viewer.client.flushWrite()
	got := drainConn(t, vcc)
	if len(got) == 0 {
		t.Fatal("FirstSeen should fire a packet; got none")
	}
	if viewer.invListeners[0].FirstSeen {
		t.Error("FirstSeen should flip to false after first send")
	}
}

func TestUpdateInvsRespectsDirty(t *testing.T) {
	s := newTestServer(t)
	s.players = make(map[int]*Player)
	s.invs = make(map[int]*inventory.Inventory)

	owner, _ := newInvListenerTestPlayer(t, s, 2)
	inv := inventory.New(93, 28, inventory.StackNormal)
	owner.invs[93] = inv

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 2, FirstSeen: false},
	}
	inv.Update = false

	viewer.updateInvs()
	viewer.client.flushWrite()
	quiet := drainConn(t, vcc)
	if len(quiet) != 0 {
		t.Errorf("clean listener should emit nothing; got %d bytes", len(quiet))
	}

	inv.Update = true
	viewer.updateInvs()
	viewer.client.flushWrite()
	loud := drainConn(t, vcc)
	if len(loud) == 0 {
		t.Error("dirty inv should fire a packet; got none")
	}
	if inv.Update {
		t.Error("inv.Update should be cleared after the tick")
	}
}

func TestUpdateInvsWorldSource(t *testing.T) {
	s := newTestServer(t)
	s.players = make(map[int]*Player)
	s.invs = make(map[int]*inventory.Inventory)
	s.invs[0] = inventory.New(0, 1, inventory.StackAlways)

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	viewer.invListeners = []InventoryListener{
		{Type: 0, Com: 200, Source: -1, FirstSeen: true},
	}

	viewer.updateInvs()
	viewer.client.flushWrite()
	got := drainConn(t, vcc)
	if len(got) == 0 {
		t.Error("world-source listener should fire on FirstSeen")
	}
}

func TestUpdateInvsSkipsMissingSource(t *testing.T) {
	s := newTestServer(t)
	s.players = make(map[int]*Player)
	s.invs = make(map[int]*inventory.Inventory)

	viewer, vcc := newInvListenerTestPlayer(t, s, 3)
	// source=99 doesn't exist in s.players.
	viewer.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 99, FirstSeen: true},
	}

	viewer.updateInvs()
	viewer.client.flushWrite()
	got := drainConn(t, vcc)
	if len(got) != 0 {
		t.Errorf("missing source should be skipped silently; got %d bytes", len(got))
	}
}
```

- [ ] **Step 5.2: Run the tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestUpdateInvs -v`
Expected: FAIL — current stub iterates `p.invs`, not `p.invListeners`, and doesn't consult `Server.invs`.

- [ ] **Step 5.3: Rename `sendUpdateInvFull` → `sendUpdateInvFullCom`**

Edit `modules/world/inv_update.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateInvFullCom writes an UpdateInvFull packet for a single inventory
// routed to the given UI component. Matches TS UpdateInvFullEncoder:
//   p2(com) p1(size)
//   per slot: p2(id+1) p1(count) OR p1(255)+p4(count) for count >= 255,
//             or p2(0)+p1(0) for empty slots.
func sendUpdateInvFullCom(p *Player, com int, inv *inventory.Inventory) {
	buf := packet.NewPacket(nil)

	buf.P2(uint16(com))
	size := inv.Capacity
	if size > 0xff {
		size = 0xff
	}
	buf.P1(uint8(size))
	for slot := 0; slot < size; slot++ {
		item := inv.Get(slot)
		if item == nil {
			buf.P2(0)
			buf.P1(0)
			continue
		}
		buf.P2(uint16(item.Id + 1))
		if item.Count >= 255 {
			buf.P1(255)
			buf.P4(uint32(item.Count))
		} else {
			buf.P1(uint8(item.Count))
		}
	}

	p.writeOut(gameserver.OpUpdateInvFull, buf.Bytes())
}
```

- [ ] **Step 5.4: Rewrite `updateInvs`**

In `modules/world/player.go`, replace the existing `updateInvs()` body (lines 324-332 approximately) with:

```go
func (p *Player) updateInvs() {
	if p.client == nil || p.client.server == nil {
		return
	}
	for i := range p.invListeners {
		l := &p.invListeners[i]

		var inv *inventory.Inventory
		if l.Source == -1 {
			inv = p.client.server.invs[l.Type]
		} else {
			other := p.client.server.players[l.Source]
			if other == nil {
				continue
			}
			inv = other.invs[l.Type]
		}
		if inv == nil {
			continue
		}

		if inv.Update || l.FirstSeen {
			sendUpdateInvFullCom(p, l.Com, inv)
			l.FirstSeen = false
		}
	}
	// Clear inv.Update AFTER all listeners (multiple listeners can share an inv).
	for _, inv := range p.invs {
		inv.Update = false
	}
	for _, inv := range p.client.server.invs {
		inv.Update = false
	}
}
```

Ensure `"github.com/zsrv/goscape/pkg/inventory"` is imported in `player.go` (it already is, per line 8).

- [ ] **Step 5.5: Run the tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestUpdateInvs -v`
Expected: PASS (all four new tests).

- [ ] **Step 5.6: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 5.7: Commit**

```bash
git add modules/world/inv_update.go modules/world/inv_update_test.go modules/world/player.go
git commit --no-gpg-sign -m "feat(world): listener-routed updateInvs with FirstSeen support

- Rename sendUpdateInvFull -> sendUpdateInvFullCom (param is component, not invId)
- updateInvs iterates Player.invListeners and routes to each listener's Com
- Source -1 resolves to Server.invs (world-shared), else to another player
- Missing sources/invs silently skipped
- inv.Update cleared after all listeners observe it

Deferred: UpdateRunWeight emission (sub-spec 4b)."
```

---

## Task 6: AFK zones — `afkzone.go` + `updateAfkZones`

**Files:**
- Create: `modules/world/afkzone.go`
- Create: `modules/world/afkzone_test.go`
- Modify: `modules/world/player.go` (the `updateAfkZones` stub)

- [ ] **Step 6.1: Write the failing tests**

Create `modules/world/afkzone_test.go`:

```go
package world

import "testing"

func TestPackUnpackAfkCoord(t *testing.T) {
	got := packAfkCoord(0, 3084, 3096)
	x, z := unpackAfkCoord(got)
	if x != 3084 || z != 3096 {
		t.Errorf("roundtrip: got (%d,%d), want (3084,3096)", x, z)
	}
}

func TestRectsIntersect(t *testing.T) {
	if !rectsIntersect(100, 100, 1, 1, 95, 95, 21, 21) {
		t.Error("point (100,100) should intersect 21×21 rect at (95,95)")
	}
	if rectsIntersect(200, 200, 1, 1, 95, 95, 21, 21) {
		t.Error("point (200,200) should NOT intersect 21×21 rect at (95,95)")
	}
}

func TestAfkZoneIncrementsWhileStill(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	for i := 0; i < 5; i++ {
		p.updateAfkZones()
	}
	if p.lastAfkZone != 5 {
		t.Errorf("lastAfkZone: got %d, want 5", p.lastAfkZone)
	}
}

func TestAfkZoneRecentersOnLeave(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.updateAfkZones()
	oldZone0 := p.afkZones[0]
	// Move 25 tiles — outside the 21×21 window.
	p.x = 3094 + 25
	p.updateAfkZones()
	if p.afkZones[0] == oldZone0 {
		t.Error("afkZones[0] should have been recentered after moving out")
	}
	if p.afkZones[1] != oldZone0 {
		t.Errorf("afkZones[1] should have received the old zone[0]; got %d want %d", p.afkZones[1], oldZone0)
	}
	if p.lastAfkZone != 0 {
		t.Errorf("lastAfkZone should reset to 0 on recenter; got %d", p.lastAfkZone)
	}
}

func TestAfkZoneSaturatesAt1000(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	for i := 0; i < 1500; i++ {
		p.updateAfkZones()
	}
	if p.lastAfkZone != 1000 {
		t.Errorf("lastAfkZone: got %d, want 1000", p.lastAfkZone)
	}
	if !p.IsZonesAfk() {
		t.Error("IsZonesAfk() should return true at 1000")
	}
}

func TestAfkZoneInstantJumpUsesNewCoord(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.updateAfkZones()
	// Move out.
	p.x = 3094 + 25
	p.moveSpeed = MoveSpeedInstant
	p.jump = true
	p.updateAfkZones()
	if p.afkZones[0] != p.afkZones[1] {
		t.Errorf("instant+jump should put the same new coord in both slots; got [%d, %d]", p.afkZones[0], p.afkZones[1])
	}
}
```

- [ ] **Step 6.2: Run the tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPackUnpackAfkCoord|TestRectsIntersect|TestAfkZone' -v`
Expected: FAIL — `packAfkCoord`, `rectsIntersect`, `IsZonesAfk` undefined.

- [ ] **Step 6.3: Implement the AFK zone module**

Create `modules/world/afkzone.go`:

```go
package world

// packAfkCoord packs (level, x, z) into a single int32 using the standard
// RS coord layout: (level<<28) | ((x&0x3FFF)<<14) | (z&0x3FFF).
func packAfkCoord(level, x, z int) int32 {
	return int32((level&0x3)<<28 | (x&0x3FFF)<<14 | (z & 0x3FFF))
}

// unpackAfkCoord reverses packAfkCoord, returning (x, z). Level is unused by
// AFK logic (the checks are 2D) so we don't bother returning it.
func unpackAfkCoord(packed int32) (x, z int) {
	x = int((packed >> 14) & 0x3FFF)
	z = int(packed & 0x3FFF)
	return
}

// rectsIntersect returns whether two axis-aligned rectangles overlap.
// Coords are the south-west corner; sizes extend north/east.
func rectsIntersect(ax, az, aw, ah, bx, bz, bw, bh int) bool {
	return ax < bx+bw && ax+aw > bx && az < bz+bh && az+ah > bz
}

// updateAfkZones advances the AFK-zone state machine. Pure server-side — no
// packet is ever sent. See TS Player.ts::updateAfkZones for the reference.
func (p *Player) updateAfkZones() {
	if p.lastAfkZone < 1000 {
		p.lastAfkZone++
	}
	if p.withinAfkZone() {
		return
	}
	coord := packAfkCoord(0, p.x-10, p.z-10)
	if p.moveSpeed == MoveSpeedInstant && p.jump {
		p.afkZones[1] = coord
	} else {
		p.afkZones[1] = p.afkZones[0]
	}
	p.afkZones[0] = coord
	p.lastAfkZone = 0
}

// withinAfkZone returns true if the player's 1×1 footprint still overlaps
// either of the two tracked 21×21 AFK windows.
func (p *Player) withinAfkZone() bool {
	const size = 21
	for i := 0; i < len(p.afkZones); i++ {
		zx, zz := unpackAfkCoord(p.afkZones[i])
		if rectsIntersect(p.x, p.z, 1, 1, zx, zz, size, size) {
			return true
		}
	}
	return false
}

// IsZonesAfk returns true once lastAfkZone saturates at 1000 ticks (the
// player has not left either zone for that long).
func (p *Player) IsZonesAfk() bool { return p.lastAfkZone == 1000 }
```

- [ ] **Step 6.4: Remove the `updateAfkZones` stub from `player.go`**

Find and delete the line `func (p *Player) updateAfkZones() {}` in `modules/world/player.go`. (The real implementation now lives in `afkzone.go`.)

- [ ] **Step 6.5: Run the tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPackUnpackAfkCoord|TestRectsIntersect|TestAfkZone' -v`
Expected: PASS (all 6 tests).

- [ ] **Step 6.6: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6.7: Commit**

```bash
git add modules/world/afkzone.go modules/world/afkzone_test.go modules/world/player.go
git commit --no-gpg-sign -m "feat(world): AFK zone tracking

Port of TS Player.updateAfkZones. Two 21×21 windows tracked in afkZones[0,1];
lastAfkZone saturates at 1000 ticks. Pure server-side state — no packet.
IsZonesAfk() exposes the saturation flag for future code (idle logout, etc.)."
```

---

## Task 7: Modal-close → UpdateInvStopTransmit hook

**Files:**
- Modify: `modules/world/player.go::encodeOut`
- Create: `modules/world/modal_close_test.go`

- [ ] **Step 7.1: Write the failing tests**

Create `modules/world/modal_close_test.go`:

```go
package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func TestModalCloseEmitsStopTransmit(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 2, FirstSeen: false},
		{Type: 93, Com: 150, Source: -1, FirstSeen: false},
	}
	p.refreshModalClose = true

	p.encodeOut()
	p.client.flushWrite()

	got := drainConn(t, cc)
	// Expected wire:
	//   1 byte IfClose (opcode, no payload)
	//   + 2 * 3 bytes UpdateInvStopTransmit (1 opcode + 2 payload)
	// Total = 1 + 6 = 7 bytes.
	if len(got) != 7 {
		t.Errorf("got %d bytes, want 7 (IfClose + 2× StopTransmit); bytes=%v", len(got), got)
	}
	if len(p.invListeners) != 0 {
		t.Errorf("invListeners should be cleared; got %d", len(p.invListeners))
	}
}

func TestNoStopTransmitWithoutModalClose(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListeners = []InventoryListener{
		{Type: 93, Com: 149, Source: 2, FirstSeen: false},
	}
	p.refreshModalClose = false

	p.encodeOut()
	p.client.flushWrite()

	got := drainConn(t, cc)
	if len(got) != 0 {
		t.Errorf("no modal close → no stop-transmit; got %d bytes", len(got))
	}
	if len(p.invListeners) != 1 {
		t.Errorf("invListeners should be untouched; got %d", len(p.invListeners))
	}
}
```

- [ ] **Step 7.2: Run the tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestModalClose -v` and `...-run TestNoStopTransmitWithoutModalClose -v`
Expected: FAIL — current `encodeOut` doesn't emit stop-transmit.

- [ ] **Step 7.3: Hook stop-transmit into `encodeOut`**

In `modules/world/player.go::encodeOut`, replace the existing `if p.refreshModalClose { p.writeOut(gameserver.OpIfClose, nil) }` block so that the stop-transmit + listener purge happens before the flag is cleared:

Before (the relevant excerpt from around lines 181-188):
```go
	if modalChanged {
		if p.refreshModalClose {
			p.writeOut(gameserver.OpIfClose, nil)
		}
		p.refreshModalClose = false
		p.lastModalMain = p.modalMain
		p.lastModalChat = p.modalChat
		p.lastModalSide = p.modalSide
	}
```

After:
```go
	if modalChanged {
		if p.refreshModalClose {
			p.writeOut(gameserver.OpIfClose, nil)
			// Stop transmitting every currently-registered inv.
			// Approximation: TS only stops listeners bound to the closing
			// modal's components; we don't yet have a component-to-modal
			// mapping, so clear all. Re-registered on next modal open.
			for _, l := range p.invListeners {
				sendUpdateInvStopTransmit(p, l.Com)
			}
			p.invListeners = p.invListeners[:0]
		}
		p.refreshModalClose = false
		p.lastModalMain = p.modalMain
		p.lastModalChat = p.modalChat
		p.lastModalSide = p.modalSide
	}
```

- [ ] **Step 7.4: Run the tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestModalClose|TestNoStopTransmit' -v`
Expected: PASS.

- [ ] **Step 7.5: Run full suite + vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no output (clean).

- [ ] **Step 7.6: Commit**

```bash
git add modules/world/player.go modules/world/modal_close_test.go
git commit --no-gpg-sign -m "feat(world): emit UpdateInvStopTransmit on modal close

When refreshModalClose triggers, iterate invListeners and send
UpdateInvStopTransmit for each, then clear the listener slice. Approximates
TS behaviour (which only clears listeners bound to the closed modal) — we
conservatively clear all since we don't yet have a component→modal mapping."
```

---

## Final Verification

- [ ] **Step F.1: Full test suite, including race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS.

- [ ] **Step F.2: `go vet`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no output.

- [ ] **Step F.3: Confirm the four `Player.update*` stubs are gone or explicit**

Run: `grep -n 'func (p \*Player) update' modules/world/player.go modules/world/afkzone.go`

Expected: `updateMap`, `updatePlayers`, `updateNpcs`, `updateZones`, `updateInvs`, `updateStats` in `player.go`; `updateAfkZones` in `afkzone.go`. Only `updateZones` should remain a `{}` stub, with a `// TODO(sub-spec 4b)` comment if desired.

- [ ] **Step F.4: (optional) Document completion**

No spec-doc edits required — the spec already marks what's done. If the session chooses, append a “Status: completed 2026-04-20” line; otherwise skip.

---

## Spec coverage map (self-review)

| Spec requirement | Task |
|---|---|
| `OpUpdateStat` / `OpUpdateRunEnergy` / `OpUpdateInvStopTransmit` opcodes | Task 1 |
| `sendUpdateStat` / `sendUpdateRunEnergy` senders | Task 2 |
| `sendUpdateInvStopTransmit` sender | Task 4 |
| `InventoryListener.FirstSeen` field | Task 1 |
| `Server.invs` world-inventory map | Task 1 |
| `updateInvs` listener-routing rewrite + sendUpdateInvFull rename | Task 5 |
| `updateStats` diff loop | Task 3 |
| `lastStats`/`lastLevels` sentinel init | Task 1 |
| `afkZones` / `lastAfkZone` fields | Task 1 |
| `updateAfkZones` + `withinAfkZone` + helpers | Task 6 |
| `IsZonesAfk` accessor | Task 6 |
| Modal-close stop-transmit hook | Task 7 |
| All seven acceptance criteria verifiable via `go test ./...` | Task F |

No gaps. Every spec bullet maps to a specific task.
