# NAI-182 — Misc ServerProt Cluster Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port 4 missing TS `ServerGameProt.ts` opcodes (`UPDATE_PID`, `RESET_ANIMS`, `RESET_CLIENT_VARCACHE`, `UPDATE_REBOOT_TIMER`), the `onReconnect` resync lifecycle, the World shutdown consumer, and 3 ServerProt-coupled staff-cheats (`::reboot`, `::slowreboot`, `::serverdrop`).

**Architecture:** Encoder send-functions follow the existing `sendUpdateStat`-style pattern in `modules/world/`. Login wiring branches on `p.reconnecting` at `processLogins`. Reboot infra adds `Server.shutdownTick` + `Server.rebootTimer(duration)` broadcaster + `Server.processShutdown` consumer firing at the top of every tick. World-module graceful exit threads `s.gracefulExit chan struct{}` + `s.shutdownGraceful bool` through `Server.Run` and `world.go runFn`.

**Tech Stack:** Go 1.26+, package `github.com/zsrv/goscape/modules/world`, package `github.com/zsrv/goscape/pkg/io/protocol/game/server`. Tests use `pkg/io/packet`, `pkg/io/isaac`, the existing `isaacPair` / `drainConn` / `newTestPlayer` helpers from `modules/world/*_test.go`.

**Spec:** `docs/superpowers/specs/2026-05-12-nai-182-misc-serverprot-cluster-design.md`

---

## File Structure

| File | Action | Responsibility |
| --- | --- | --- |
| `pkg/io/protocol/game/server/prot.go` | Modify | Append 4 `Op` declarations |
| `modules/world/login_resync.go` | Create | `sendUpdatePid`, `sendResetClientVarCache`, `sendResetAnims`, `onReconnect` |
| `modules/world/login_resync_test.go` | Create | Encoder byte-pins + processLogins fresh-login wiring tests |
| `modules/world/reboot.go` | Create | `sendUpdateRebootTimer`, `Server.rebootTimer`, `isPendingShutdown`, `shutdownTicksRemaining`, `processShutdown` |
| `modules/world/reboot_test.go` | Create | Reboot infra + shutdown consumer tests |
| `modules/world/reconnect_test.go` | Create | `onReconnect` lifecycle tests |
| `modules/world/server.go` | Modify | Add `shutdownTick int` + `shutdownGraceful bool` + `gracefulExit chan struct{}` fields; init in `newServer`; modify `Server.Run()` to select on `gracefulExit` |
| `modules/world/player.go` | Modify | Add `forceRemove bool` field |
| `modules/world/tick.go` | Modify | Insert `processShutdown` at top of `runTickLoopWithRate` for-body; insert `processLogins` reconnect-branch + fresh-login emits; modify `processLogouts` to honor `forceRemove` |
| `modules/world/world.go` | Modify | `runFn` patches `serverDone` branch to check `serv.shutdownGraceful` |
| `modules/world/handlers_game.go` | Modify | Add `math` import; append `reboot` / `slowreboot` / `serverdrop` switch arms |
| `modules/world/handlers_game_test.go` | Modify | Append cheat-dispatch tests |

---

## Task 1 (B0): Append 4 Opcode Declarations

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`

Pure data declarations — no test needed (existing pattern at NAI-181 close `820301c`).

- [ ] **Step 1: Append 4 `Op` entries**

Open `pkg/io/protocol/game/server/prot.go`. Append BEFORE the closing `)` of the `var (...)` block (i.e., after `OpLastLoginInfo` at line 145), the following 4 entries:

```go

	// OpUpdatePid carries the player's server-side slot to the client
	// so the client's localPlayer reference is bound to the correct
	// PlayerInfo slot. Emitted once at onLogin. Fixed 2-byte payload:
	// p2(slot). Mirrors TS ServerGameProt.UPDATE_PID (139, 2) and
	// UpdatePidEncoder.ts (NAI-182).
	OpUpdatePid = Op{Opcode: 139, PayloadSize: 2}

	// OpResetAnims tells the client to clear all animation layers on the
	// local player. Zero-byte payload. Emitted at onLogin (after varp
	// resync) and onReconnect (after per-stat UpdateStat/UpdateRunEnergy).
	// Mirrors TS ServerGameProt.RESET_ANIMS (136, 0) and
	// ResetAnimsEncoder.ts (NAI-182).
	OpResetAnims = Op{Opcode: 136, PayloadSize: 0}

	// OpResetClientVarCache tells the client to drop its cached varp
	// values so the next varp packets become authoritative. Emitted at
	// onLogin and onReconnect immediately before the varp transmit-loop.
	// Zero-byte payload. Mirrors TS ServerGameProt.RESET_CLIENT_VARCACHE
	// (193, 0) and ResetClientVarCacheEncoder.ts (NAI-182).
	OpResetClientVarCache = Op{Opcode: 193, PayloadSize: 0}

	// OpUpdateRebootTimer carries the number of game ticks (600ms each)
	// remaining until the world reboots. Sent broadcast by
	// Server.rebootTimer and to each connecting player at processLogins
	// if a shutdown is pending. Fixed 2-byte payload: p2(ticks). Mirrors
	// TS ServerGameProt.UPDATE_REBOOT_TIMER (43, 2) and
	// UpdateRebootTimerEncoder.ts (NAI-182).
	OpUpdateRebootTimer = Op{Opcode: 43, PayloadSize: 2}
```

- [ ] **Step 2: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/io/protocol/game/server/...`
Expected: exit 0, no output.

- [ ] **Step 3: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(proto): NAI-182 B0 — UpdatePid/ResetAnims/ResetClientVarCache/UpdateRebootTimer Op declarations

Adds the 4 missing TS ServerGameProt opcodes to pkg/io/protocol/game/server/prot.go.
No callers wired yet; encoders + login/reconnect/shutdown wiring follow in
NAI-182 B1-B6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 (B1): Encoder Send-Functions + Byte-Pin Tests

**Files:**
- Create: `modules/world/login_resync.go`
- Create: `modules/world/reboot.go`
- Create: `modules/world/login_resync_test.go`
- Create: `modules/world/reboot_test.go`

Pattern reference: `modules/world/stat_update.go:10-37` (send-function shape) and `modules/world/player_post_decode_test.go:63-95` (byte-pin assertion using isaac sibling stream + `drainConn`).

### Pre-flight controller checks

- [ ] **Step 1: Verify test helpers exist**

```bash
grep -n "func newTestPlayer\|func isaacPair\|func drainConn" modules/world/*_test.go | head
```

Expected output includes `newTestPlayer`, `isaacPair`, `drainConn` definitions. If any is missing, the test code below must import from wherever it lives — pin actual location before writing tests.

### Encoder source

- [ ] **Step 2: Write `modules/world/login_resync.go` (encoders only — `onReconnect` added in B4)**

Create new file `modules/world/login_resync.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdatePid writes one UPDATE_PID packet. Mirrors TS
// UpdatePidEncoder (`buf.p2(message.uid)`); TS passes p.slot at
// Player.ts:495 via `new UpdatePid(this.slot)` — slot is the int
// field, not the composed uid. NAI-182.
func sendUpdatePid(p *Player, slot int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(slot))
	p.writeOut(gameserver.OpUpdatePid, buf.Bytes())
}

// sendResetClientVarCache writes one RESET_CLIENT_VARCACHE packet
// (0-byte payload). NAI-182.
func sendResetClientVarCache(p *Player) {
	p.writeOut(gameserver.OpResetClientVarCache, nil)
}

// sendResetAnims writes one RESET_ANIMS packet (0-byte payload). NAI-182.
func sendResetAnims(p *Player) {
	p.writeOut(gameserver.OpResetAnims, nil)
}
```

- [ ] **Step 3: Write `modules/world/reboot.go` (encoder only — broadcaster + getters + processShutdown added in B2/B5)**

Create new file `modules/world/reboot.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateRebootTimer writes one UPDATE_REBOOT_TIMER packet carrying
// the remaining tick count (NOT seconds). Mirrors TS
// UpdateRebootTimerEncoder (`buf.p2(message.ticks)`). NAI-182.
func sendUpdateRebootTimer(p *Player, ticks int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(int16(ticks)))
	p.writeOut(gameserver.OpUpdateRebootTimer, buf.Bytes())
}
```

- [ ] **Step 4: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: exit 0.

### Encoder byte-pin tests

- [ ] **Step 5: Write failing test — `TestSendUpdatePid_EmitsExactByteSequence`**

Create `modules/world/login_resync_test.go`:

```go
package world

import (
	"bytes"
	"testing"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestSendUpdatePid_EmitsExactByteSequence pins the wire bytes of
// UPDATE_PID: encrypted opcode + p2(slot). NAI-182 B1.
func TestSendUpdatePid_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})
	p.slot = 0x1234

	received := drainConn(t, cc)
	sendUpdatePid(p, p.slot)
	_ = p.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpUpdatePid.Opcode) + int(sibling.GetNext())) & 0xff),
		0x12, 0x34, // p2: slot
	}
	if !bytes.Equal(got, want) {
		t.Errorf("UPDATE_PID wire bytes: got %#x, want %#x", got, want)
	}
}
```

NOTE: `newTestPlayer`, `isaacPair`, `drainConn` are existing helpers — exact import / package-local lookup per Step 1.

- [ ] **Step 6: Run the test and verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSendUpdatePid_EmitsExactByteSequence -v`
Expected: PASS (test should pass immediately since the encoder is already written in Step 2). If FAIL, debug the test — the encoder logic itself is trivial.

If you need to verify the test would have failed without the encoder, comment out `sendUpdatePid` in `login_resync.go`, re-run, see compile failure, restore.

- [ ] **Step 7: Add 4 more encoder tests**

Append to `modules/world/login_resync_test.go`:

```go

// TestSendResetClientVarCache_EmitsOpcodeOnly pins the wire bytes:
// single encrypted opcode byte, no payload. NAI-182 B1.
func TestSendResetClientVarCache_EmitsOpcodeOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendResetClientVarCache(p)
	_ = p.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpResetClientVarCache.Opcode) + int(sibling.GetNext())) & 0xff),
	}
	if !bytes.Equal(got, want) {
		t.Errorf("RESET_CLIENT_VARCACHE wire bytes: got %#x, want %#x", got, want)
	}
}

// TestSendResetAnims_EmitsOpcodeOnly pins the wire bytes:
// single encrypted opcode byte, no payload. NAI-182 B1.
func TestSendResetAnims_EmitsOpcodeOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendResetAnims(p)
	_ = p.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpResetAnims.Opcode) + int(sibling.GetNext())) & 0xff),
	}
	if !bytes.Equal(got, want) {
		t.Errorf("RESET_ANIMS wire bytes: got %#x, want %#x", got, want)
	}
}
```

Create `modules/world/reboot_test.go`:

```go
package world

import (
	"bytes"
	"testing"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestSendUpdateRebootTimer_EmitsExactByteSequence pins UPDATE_REBOOT_TIMER
// wire bytes for a representative positive tick count. NAI-182 B1.
func TestSendUpdateRebootTimer_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendUpdateRebootTimer(p, 50)
	_ = p.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(sibling.GetNext())) & 0xff),
		0x00, 0x32, // p2: 50
	}
	if !bytes.Equal(got, want) {
		t.Errorf("UPDATE_REBOOT_TIMER wire bytes: got %#x, want %#x", got, want)
	}
}

// TestSendUpdateRebootTimer_ZeroTicks pins the duration=0 emit
// (e.g., ::reboot immediate). NAI-182 B1.
func TestSendUpdateRebootTimer_ZeroTicks(t *testing.T) {
	p, cc := newTestPlayer(t)
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendUpdateRebootTimer(p, 0)
	_ = p.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(sibling.GetNext())) & 0xff),
		0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("UPDATE_REBOOT_TIMER(0) wire bytes: got %#x, want %#x", got, want)
	}
}
```

- [ ] **Step 8: Run all 5 encoder tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestSendUpdatePid|TestSendResetClientVarCache|TestSendResetAnims|TestSendUpdateRebootTimer" -v`
Expected: 5 PASS.

- [ ] **Step 9: Commit**

```bash
git add modules/world/login_resync.go modules/world/reboot.go modules/world/login_resync_test.go modules/world/reboot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-182 B1 — sendUpdatePid/ResetClientVarCache/ResetAnims/UpdateRebootTimer encoders

Adds 4 send-functions in modules/world/{login_resync,reboot}.go with byte-pin
tests pinning encrypted-opcode + p2(slot|ticks) where applicable. No production
callers yet — wiring follows in NAI-182 B3-B5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 (B2): Reboot Infrastructure (shutdownTick + rebootTimer + getters)

**Files:**
- Modify: `modules/world/server.go` (add field, init)
- Modify: `modules/world/reboot.go` (add broadcaster + getters)
- Modify: `modules/world/reboot_test.go` (add infra tests)

### Pre-flight controller checks

- [ ] **Step 1: Locate `newServer` constructor**

```bash
grep -n "func newServer\|func NewServer" modules/world/server.go
```

Pin the exact constructor name and the line where field initialisers live (look for `currentTick:` or similar default-value assignments — `shutdownTick: -1` goes adjacent).

### Add `shutdownTick` field

- [ ] **Step 2: Modify `modules/world/server.go` — add `shutdownTick int` near `currentTick`**

Find the line `currentTick int` (at line ~60). Insert immediately after it:

```go
	// shutdownTick is the tick on which the world will halt. -1 means
	// no shutdown scheduled. Set by Server.rebootTimer; consumed by
	// Server.processShutdown (called at top of tick body when
	// s.currentTick >= s.shutdownTick && s.shutdownTick != -1).
	// Mirrors TS World.shutdownTick (World.ts:166). NAI-182.
	shutdownTick int
```

- [ ] **Step 3: Initialise `shutdownTick: -1` in `newServer` (or its equivalent constructor)**

In whatever constructor builds the `Server` struct (found in Step 1), add `shutdownTick: -1,` to the struct literal. If the constructor uses `s := &Server{...}` followed by per-field assignments, append `s.shutdownTick = -1` after the struct allocation.

- [ ] **Step 4: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: exit 0.

### Broadcaster + getters

- [ ] **Step 5: Write failing test — `TestNewServer_ShutdownTickDefaultsToMinusOne`**

Append to `modules/world/reboot_test.go`:

```go

// TestNewServer_ShutdownTickDefaultsToMinusOne pins the post-construct
// invariant: no shutdown is pending. NAI-182 B2.
func TestNewServer_ShutdownTickDefaultsToMinusOne(t *testing.T) {
	s := newTestServer(t)
	if s.shutdownTick != -1 {
		t.Errorf("newServer: shutdownTick = %d, want -1", s.shutdownTick)
	}
}
```

Pre-flight Step: confirm `newTestServer` exists. If not, locate the analogous helper and use it (typical pattern: `newTestPlayer(t)` returns `(*Player, conn)` and internally constructs a Server — extract the Server reference via `p.client.server`).

- [ ] **Step 6: Run test, expect PASS (field-init was already done in Step 3)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNewServer_ShutdownTickDefaultsToMinusOne -v`
Expected: PASS.

- [ ] **Step 7: Add broadcaster + getters to `modules/world/reboot.go`**

Append to `modules/world/reboot.go`:

```go

// rebootTimer schedules a world reboot in `duration` ticks and
// broadcasts the new countdown to every connected player in
// s.playerLoop. Mirrors TS World.rebootTimer (World.ts:1787-1793).
// NAI-182.
func (s *Server) rebootTimer(duration int) {
	s.shutdownTick = s.currentTick + duration
	for _, p := range s.playerLoop {
		if p == nil {
			continue
		}
		sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
	}
}

// isPendingShutdown reports whether a shutdown is currently scheduled.
// Mirrors TS World.isPendingShutdown (World.ts:1795-1797). Equivalent
// to s.shutdownTicksRemaining() > -1. NAI-182.
func (s *Server) isPendingShutdown() bool {
	return s.shutdownTicksRemaining() > -1
}

// shutdownTicksRemaining returns shutdownTick - currentTick. Returns a
// negative number when no shutdown is scheduled (shutdownTick == -1).
// Mirrors TS World.shutdownTicksRemaining (World.ts:1799-1801). NAI-182.
func (s *Server) shutdownTicksRemaining() int {
	return s.shutdownTick - s.currentTick
}
```

- [ ] **Step 8: Write broadcaster + getter tests**

Append to `modules/world/reboot_test.go`:

```go

// TestRebootTimer_SetsShutdownTickAndBroadcasts pins the broadcaster
// effect on s.shutdownTick and the per-player wire emit. NAI-182 B2.
func TestRebootTimer_SetsShutdownTickAndBroadcasts(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := p.client.server
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})

	startTick := s.currentTick
	received := drainConn(t, cc)
	s.rebootTimer(50)
	_ = p.client.flushWrite()
	got := <-received

	if s.shutdownTick != startTick+50 {
		t.Errorf("shutdownTick after rebootTimer(50): got %d, want %d", s.shutdownTick, startTick+50)
	}

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(sibling.GetNext())) & 0xff),
		0x00, 0x32,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("broadcast bytes: got %#x, want %#x", got, want)
	}
}

// TestRebootTimer_DurationZero pins immediate-reboot semantics. NAI-182 B2.
func TestRebootTimer_DurationZero(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := p.client.server
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})

	startTick := s.currentTick
	received := drainConn(t, cc)
	s.rebootTimer(0)
	_ = p.client.flushWrite()
	got := <-received

	if s.shutdownTick != startTick {
		t.Errorf("shutdownTick after rebootTimer(0): got %d, want %d", s.shutdownTick, startTick)
	}

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(sibling.GetNext())) & 0xff),
		0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("broadcast bytes: got %#x, want %#x", got, want)
	}
}

// TestIsPendingShutdown_AndTicksRemaining pins the getter return values
// before and after rebootTimer + tick advancement. NAI-182 B2.
func TestIsPendingShutdown_AndTicksRemaining(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := p.client.server

	if s.isPendingShutdown() {
		t.Error("isPendingShutdown: pre-rebootTimer: got true, want false")
	}

	startTick := s.currentTick
	s.rebootTimer(50)

	if !s.isPendingShutdown() {
		t.Error("isPendingShutdown: post-rebootTimer(50): got false, want true")
	}
	if got := s.shutdownTicksRemaining(); got != 50 {
		t.Errorf("shutdownTicksRemaining: got %d, want 50", got)
	}

	// Advance currentTick by 10
	s.currentTick = startTick + 10
	if got := s.shutdownTicksRemaining(); got != 40 {
		t.Errorf("shutdownTicksRemaining after +10 ticks: got %d, want 40", got)
	}
}
```

- [ ] **Step 9: Run all B2 tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNewServer_ShutdownTickDefaults|TestRebootTimer_|TestIsPendingShutdown" -v`
Expected: 4 PASS.

- [ ] **Step 10: Commit**

```bash
git add modules/world/server.go modules/world/reboot.go modules/world/reboot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-182 B2 — Server.shutdownTick + rebootTimer broadcaster + getters

Adds Server.shutdownTick field (init -1), Server.rebootTimer(duration)
broadcaster, and isPendingShutdown / shutdownTicksRemaining getters
mirroring TS World.ts:166,1787-1801. Consumer (processShutdown) lands in
NAI-182 B5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 (B3): processLogins Fresh-Login Wiring

**Files:**
- Modify: `modules/world/tick.go` (processLogins per-player loop)
- Modify: `modules/world/login_resync_test.go` (add wiring tests)

### Risk §5-7 fixture audit (per spec)

- [ ] **Step 1: Enumerate all existing tests that drive `processLogins()`**

```bash
grep -n "s\.processLogins()" modules/world/*_test.go
```

Expected sites (per spec):
- `modules/world/server_test.go:457,492`
- `modules/world/tick_test.go:19`
- `modules/world/tick_logins_test.go:34,49`

For each site, read the test body to see whether it asserts wire-byte equality after processLogins. If any does, those tests need fixture-side adjustment in this task (extend expected bytes with the new UPDATE_PID + RESET_CLIENT_VARCACHE + varp-loop + RESET_ANIMS sequence). If none asserts wire equality, the fresh-login emit is invisible to existing tests and no fixture changes are needed.

Read each callsite. Document findings inline as a comment in the commit message (or, if fixture changes are needed, list each affected test).

- [ ] **Step 2: Verify the varp-table fixture used by `newTestServer` / `newTestPlayer`**

```bash
grep -n "varpTypes\s*=\|varpTypes:" modules/world/*_test.go | head
```

Expected: a test helper somewhere allocates `varpTypes`. If `len(s.varpTypes.Configs)` is zero in the typical test fixture, the new transmit-loop is a no-op for those tests — confirming Step 1's "no fixture changes needed" reading.

### Write failing test — fresh-login emit sequence

- [ ] **Step 3: Write failing test — `TestProcessLogins_FreshLogin_EmitsOpcodeOrder`**

Append to `modules/world/login_resync_test.go`:

```go

// TestProcessLogins_FreshLogin_EmitsOpcodeOrder pins the post-login
// emit sequence: UPDATE_PID → RESET_CLIENT_VARCACHE → transmit-true
// varps → RESET_ANIMS. Mirrors TS Player.onLogin (Player.ts:494-504)
// minus IF_CLOSE / ChatFilterSettings / UpdateIgnoreList (DEVIATION
// NAI-182-D4 / -D5). NAI-182 B3.
func TestProcessLogins_FreshLogin_EmitsOpcodeOrder(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := p.client.server

	// Re-queue this player as a fresh login candidate.
	s.playersMu.Lock()
	s.players[p.slot] = nil // unregister; processLogins will re-add
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	_ = p.client.flushWrite()
	got := <-received

	// Extract opcodes in order. The encrypted opcode byte is
	// (encOpcode + isaac.GetNext()) & 0xff. We accept the raw opcode
	// sequence by tracking the isaac stream and decoding each opcode.
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})
	wantOpcodes := []byte{
		gameserver.OpUpdatePid.Opcode,
		gameserver.OpResetClientVarCache.Opcode,
		gameserver.OpResetAnims.Opcode,
	}
	for i, op := range wantOpcodes {
		if len(got) == 0 {
			t.Fatalf("opcode %d (%d): emit stream truncated", i, op)
		}
		want := byte((int(op) + int(sibling.GetNext())) & 0xff)
		if got[0] != want {
			t.Errorf("opcode %d: got %#x, want %#x (op=%d)", i, got[0], want, op)
		}
		// Skip the opcode byte AND its payload bytes for the next iteration.
		var payloadLen int
		switch op {
		case gameserver.OpUpdatePid.Opcode:
			payloadLen = 2 // p2(slot)
		case gameserver.OpResetClientVarCache.Opcode, gameserver.OpResetAnims.Opcode:
			payloadLen = 0
		}
		got = got[1+payloadLen:]
	}
}
```

This test assumes `len(s.varpTypes.Configs) == 0` in the fixture (so the transmit-loop is a no-op). If §5-7 Step 2 showed a non-empty varp table, extend the `wantOpcodes` list with `OpVarpSmall` / `OpVarpLarge` entries matching the fixture's transmit-true varps.

- [ ] **Step 4: Run test, expect FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLogins_FreshLogin_EmitsOpcodeOrder -v`
Expected: FAIL — the processLogins wiring isn't in place yet.

### Wire processLogins

- [ ] **Step 5: Modify `modules/world/tick.go` — insert fresh-login emits before LOGIN trigger**

Read lines 105-180 of `tick.go` first to confirm current shape.

In the per-player loop body of `processLogins`, locate the existing LOGIN-trigger block at lines 155-158:

```go
		// Fire the LOGIN trigger if the cache has one. Sub-spec RuneScript S3.
		if s.scriptProvider != nil {
			sf := s.scriptProvider.GetByTrigger(script.TriggerLogin, -1, -1)
			s.runScript(sf, p, nil, true, nil, nil)
		}
```

Insert IMMEDIATELY BEFORE that block:

```go
		// NAI-182 B3 — fresh-login emit sequence per TS Player.onLogin
		// (Player.ts:494-504). DEVIATION-NAI-182-D4 omits IF_CLOSE,
		// DEVIATION-NAI-182-D5 omits ChatFilterSettings / UpdateIgnoreList
		// (deferred social cluster).
		//
		// onReconnect branch (NAI-182 B4) lands here in a subsequent task
		// — for now every login is a fresh login.
		sendUpdatePid(p, p.slot)
		sendResetClientVarCache(p)
		if s.varpTypes != nil {
			for i, vt := range s.varpTypes.Configs {
				if vt != nil && vt.Transmit {
					p.writeVarp(i, p.varps[i])
				}
			}
		}
		sendResetAnims(p)

		// NAI-182 B3 — post-onLogin UPDATE_REBOOT_TIMER emit if a
		// shutdown is pending. Mirrors TS World.processLogins
		// (World.ts:944-946).
		if s.shutdownTick != -1 {
			sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
		}
```

- [ ] **Step 6: Compile-check + re-run target test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLogins_FreshLogin_EmitsOpcodeOrder -v`
Expected: PASS.

- [ ] **Step 7: Add reboot-pending fresh-login test**

Append to `modules/world/login_resync_test.go`:

```go

// TestProcessLogins_FreshLogin_WithShutdownPending_EmitsRebootTimer
// pins the post-RESET_ANIMS UPDATE_REBOOT_TIMER emit when a shutdown
// is scheduled. Mirrors TS World.processLogins (World.ts:944-946).
// NAI-182 B3.
func TestProcessLogins_FreshLogin_WithShutdownPending_EmitsRebootTimer(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := p.client.server
	s.shutdownTick = s.currentTick + 25

	s.playersMu.Lock()
	s.players[p.slot] = nil
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	_ = p.client.flushWrite()
	got := <-received

	// Check that UPDATE_REBOOT_TIMER opcode appears in the stream with
	// payload 0x00, 0x19 (= 25). We decode the entire stream by stepping
	// through encrypted opcodes; for simplicity, search for the
	// encrypted UPDATE_REBOOT_TIMER opcode at the EXPECTED position
	// (after PID + RESET_CLIENT_VARCACHE + RESET_ANIMS = 4 bytes).
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})
	for _, op := range []byte{gameserver.OpUpdatePid.Opcode, gameserver.OpResetClientVarCache.Opcode, gameserver.OpResetAnims.Opcode} {
		want := byte((int(op) + int(sibling.GetNext())) & 0xff)
		if len(got) == 0 || got[0] != want {
			t.Fatalf("missing prior opcode %d in stream", op)
		}
		var pl int
		if op == gameserver.OpUpdatePid.Opcode {
			pl = 2
		}
		got = got[1+pl:]
	}
	wantRT := byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(sibling.GetNext())) & 0xff)
	if len(got) < 3 || got[0] != wantRT {
		t.Fatalf("UPDATE_REBOOT_TIMER opcode missing: got %#x, want %#x at pos 0", got, wantRT)
	}
	if got[1] != 0x00 || got[2] != 0x19 {
		t.Errorf("UPDATE_REBOOT_TIMER payload: got %#x %#x, want 0x00 0x19", got[1], got[2])
	}
}

// TestProcessLogins_FreshLogin_NoShutdown_NoRebootTimer pins the
// negative case: no UPDATE_REBOOT_TIMER emit when s.shutdownTick == -1.
// NAI-182 B3.
func TestProcessLogins_FreshLogin_NoShutdown_NoRebootTimer(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := p.client.server
	// shutdownTick defaults to -1 from newServer.

	s.playersMu.Lock()
	s.players[p.slot] = nil
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	_ = p.client.flushWrite()
	got := <-received

	// Walk the stream; ensure no UPDATE_REBOOT_TIMER opcode appears.
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})
	emitted := []byte{}
	for len(got) > 0 {
		// Decrypt the opcode byte for inspection.
		dec := byte((int(got[0]) - int(sibling.GetNext())) & 0xff)
		emitted = append(emitted, dec)
		var pl int
		switch dec {
		case gameserver.OpUpdatePid.Opcode:
			pl = 2
		case gameserver.OpResetClientVarCache.Opcode, gameserver.OpResetAnims.Opcode:
			pl = 0
		default:
			t.Fatalf("unexpected opcode %d in no-shutdown stream", dec)
		}
		if len(got) < 1+pl {
			t.Fatalf("truncated stream after opcode %d", dec)
		}
		got = got[1+pl:]
	}
	for _, op := range emitted {
		if op == gameserver.OpUpdateRebootTimer.Opcode {
			t.Errorf("UPDATE_REBOOT_TIMER emitted when shutdownTick == -1: emitted ops = %v", emitted)
		}
	}
}
```

- [ ] **Step 8: Run all B3 tests + full module suite to catch fixture impact**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLogins_FreshLogin -v`
Expected: 3 PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: full module-world test suite PASS. If any pre-existing test fails due to wire-byte fixture drift, **STOP** — debug per Step 1 enumeration: the failing test asserted byte-equality of a post-processLogins emit stream and needs its expected sequence extended.

- [ ] **Step 9: Commit**

```bash
git add modules/world/tick.go modules/world/login_resync_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-182 B3 — processLogins fresh-login emit sequence

Inserts UPDATE_PID → RESET_CLIENT_VARCACHE → varp transmit-loop → RESET_ANIMS
into processLogins per-player loop, immediately before the LOGIN trigger.
Adds post-onLogin UPDATE_REBOOT_TIMER emit when s.shutdownTick != -1.
Mirrors TS Player.onLogin (Player.ts:494-504) + World.processLogins
(World.ts:944-946). DEVIATION-NAI-182-D4 (IF_CLOSE omitted) and -D5
(social cluster omitted) tracked in spec §6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5 (B4): onReconnect Lifecycle

**Files:**
- Modify: `modules/world/login_resync.go` (add `onReconnect` function)
- Modify: `modules/world/tick.go` (add reconnect branch in processLogins)
- Create: `modules/world/reconnect_test.go`

### Pre-flight controller checks

- [ ] **Step 1: Confirm `p.tabs` zero-vs-nonzero convention**

```bash
grep -n "tabs\s*\[\|p\.tabs\[\|p\.tabs:" modules/world/player.go modules/world/player_interface.go | head
```

Verify: (a) tabs is `[14]int`; (b) initial values are 0 (zero-value); (c) the `IfSetTab` function at `player_interface.go:73-80` is called with `(com, tab int)` and that `com == 0` is the "no tab assigned" sentinel.

- [ ] **Step 2: Confirm `objtype.PlayerStatCount` and `p.stats` / `p.levels` shape**

```bash
grep -n "PlayerStatCount\b" pkg/objtype/*.go modules/world/player.go | head
grep -n "stats\s*\[\|levels\s*\[" modules/world/player.go | head
```

Expected: `objtype.PlayerStatCount == 21`; `p.stats [21]int32`, `p.levels [21]int32` (or similar). The `sendUpdateStat(p, stat, exp, level)` signature at `stat_update.go:10` takes `int` for stat/exp/level — confirm cast site.

- [ ] **Step 3: Confirm `p.invListeners` map shape and `FirstSeen` field**

```bash
grep -n "invListeners\s\+map\|type.*Listener\s\+struct\|FirstSeen\s\+bool" modules/world/player.go
```

Expected: `invListeners map[int]InvListener` (value type, not pointer — hence the read-modify-write idiom).

### Add `onReconnect` function

- [ ] **Step 4: Write failing test — `TestOnReconnect_EmitsResyncSequence`**

Create `modules/world/reconnect_test.go`:

```go
package world

import (
	"bytes"
	"testing"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestOnReconnect_EmitsResyncSequence pins the reconnect resync opcode
// order: RESET_CLIENT_VARCACHE → (varps) → (optional UPDATE_REBOOT_TIMER)
// → (no wire emit for closeModal) → (IF_SETTAB per non-zero tab) →
// per-stat UPDATE_STAT → UPDATE_RUN_ENERGY → RESET_ANIMS. Mirrors TS
// Player.onReconnect (Player.ts:516-574) minus refreshInvs (which
// triggers UPDATE_INV_FULL emits via the FirstSeen flag on the NEXT
// updateInvs tick, NOT inline). NAI-182 B4.
func TestOnReconnect_EmitsResyncSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := p.client.server
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	onReconnect(s, p)
	_ = p.client.flushWrite()
	got := <-received

	// First opcode: RESET_CLIENT_VARCACHE.
	want0 := byte((int(gameserver.OpResetClientVarCache.Opcode) + int(sibling.GetNext())) & 0xff)
	if len(got) == 0 || got[0] != want0 {
		t.Fatalf("opcode[0]: got %#x, want %#x (RESET_CLIENT_VARCACHE)", got, want0)
	}
	got = got[1:]

	// (varp loop skipped — depends on fixture. If non-empty, extend here.)
	// (no UPDATE_REBOOT_TIMER — shutdownTick defaults to -1 in this test.)
	// (no IF_SETTAB — tabs default to all-zero.)
	// 21 per-stat UPDATE_STAT — each is opcode + p1(stat) + p4(exp/10) + p1(level) = 7 bytes.
	for i := 0; i < 21; i++ {
		want := byte((int(gameserver.OpUpdateStat.Opcode) + int(sibling.GetNext())) & 0xff)
		if len(got) < 7 {
			t.Fatalf("stat[%d]: stream truncated", i)
		}
		if got[0] != want {
			t.Errorf("stat[%d] opcode: got %#x, want %#x", i, got[0], want)
		}
		got = got[7:]
	}
	// UPDATE_RUN_ENERGY: opcode + p1 = 2 bytes.
	wantRE := byte((int(gameserver.OpUpdateRunEnergy.Opcode) + int(sibling.GetNext())) & 0xff)
	if len(got) < 2 || got[0] != wantRE {
		t.Errorf("UPDATE_RUN_ENERGY: got %#x at pos 0, want %#x", got, wantRE)
	}
	got = got[2:]
	// RESET_ANIMS: opcode only.
	wantRA := byte((int(gameserver.OpResetAnims.Opcode) + int(sibling.GetNext())) & 0xff)
	if len(got) == 0 || got[0] != wantRA {
		t.Errorf("RESET_ANIMS: got %#x, want %#x", got, wantRA)
	}
	_ = bytes.Equal // keep import; remove if unused
}
```

If `bytes.Equal` is unused, remove the `bytes` import.

- [ ] **Step 5: Run test, expect FAIL (undefined `onReconnect`)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestOnReconnect_EmitsResyncSequence -v`
Expected: compile failure — `onReconnect` undefined.

- [ ] **Step 6: Add `onReconnect` to `modules/world/login_resync.go`**

Append to `modules/world/login_resync.go`:

```go

// onReconnect runs the resync sequence for a reconnecting player.
// Called from processLogins when p.reconnecting == true. Mirrors TS
// Player.onReconnect (Player.ts:516-574).
//
// DEVIATION-NAI-182-D1-RECONNECT-NO-RESTORE — goscape has no save/restore
// subsystem yet. The processLogins fresh-init block runs BEFORE this
// function is called (when p.reconnecting==true, the branch placement
// short-circuits AFTER processLogins's existing init), so resync packets
// carry post-fresh-init defaults rather than restored save state. Wire
// ordering is TS-faithful; data is default-valued. Retires when
// PlayerLoading lands.
func onReconnect(s *Server, p *Player) {
	// (a) RESET_CLIENT_VARCACHE
	sendResetClientVarCache(p)

	// (b) varp transmit-loop
	if s.varpTypes != nil {
		for i, vt := range s.varpTypes.Configs {
			if vt != nil && vt.Transmit {
				p.writeVarp(i, p.varps[i])
			}
		}
	}

	// (c) buildArea clear + rebuild — already handled by
	// p.reconnecting==true → shouldRebuild path at player.go:694-696.
	// No new code; rebuildNormal fires in processInfo this tick.

	// (d) reboot-timer if pending
	if s.shutdownTick != -1 {
		sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
	}

	// (e) closeModal(false) — preserves main modal, drops chat/side.
	// Does NOT emit a wire opcode; flips internal modal slots that
	// processInfo will sync via existing IF_CLOSE wiring.
	p.CloseModal(false)

	// (f) per-tab IF_SETTAB resync. Tabs default to 0 ("no tab
	// assigned"); skip zero entries.
	for tab, com := range p.tabs {
		if com != 0 {
			p.IfSetTab(com, tab)
		}
	}

	// (g) refreshInvs — flip every invListener's FirstSeen back to true
	// so the NEXT updateInvs tick re-emits each as UpdateInvFull.
	// Map-value addressability dance mirrors player.go:884-888.
	for com, l := range p.invListeners {
		l.FirstSeen = true
		p.invListeners[com] = l
	}

	// (h) per-stat UPDATE_STAT for all 21 skills.
	for i := 0; i < int(objtype.PlayerStatCount); i++ {
		sendUpdateStat(p, i, int(p.stats[i]), int(p.levels[i]))
	}

	// (i) UPDATE_RUN_ENERGY.
	sendUpdateRunEnergy(p, p.runenergy)

	// (j) RESET_ANIMS.
	sendResetAnims(p)

	// (k) masks |= entitymask — resync face_entity on the next mask
	// block. Mirrors TS Player.onReconnect (Player.ts:574).
	p.masks |= p.entitymask
}
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to the imports of `login_resync.go` if not already present.

- [ ] **Step 7: Add reconnect branch in `processLogins`**

Modify `modules/world/tick.go` `processLogins`. Find the existing fresh-login emit block added in Task 4 (Step 5), and **wrap** it with an `if p.reconnecting { ... } else { ... }` branch:

Replace the block from Task 4 Step 5 (the `sendUpdatePid` through `if s.shutdownTick != -1` block) with:

```go
		// NAI-182 — reconnect branches to onReconnect's TS-faithful
		// resync path; fresh login runs the standard onLogin emit
		// sequence. p.reconnecting is set by the login codec
		// (server.go:650) based on OpReqGameReconnect.
		if p.reconnecting {
			onReconnect(s, p)
			// rebuildNormal will clear p.reconnecting later in processInfo.
		} else {
			// Fresh-login emit sequence per TS Player.onLogin
			// (Player.ts:494-504). DEVIATION-NAI-182-D4 omits IF_CLOSE,
			// DEVIATION-NAI-182-D5 omits ChatFilterSettings /
			// UpdateIgnoreList (deferred social cluster).
			sendUpdatePid(p, p.slot)
			sendResetClientVarCache(p)
			if s.varpTypes != nil {
				for i, vt := range s.varpTypes.Configs {
					if vt != nil && vt.Transmit {
						p.writeVarp(i, p.varps[i])
					}
				}
			}
			sendResetAnims(p)

			// Post-onLogin UPDATE_REBOOT_TIMER emit if shutdown pending.
			// Mirrors TS World.processLogins (World.ts:944-946).
			if s.shutdownTick != -1 {
				sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
			}
		}
```

- [ ] **Step 8: Compile + re-run target test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestOnReconnect_EmitsResyncSequence -v`
Expected: PASS.

- [ ] **Step 9: Add reconnect-side state-assertion tests**

Append to `modules/world/reconnect_test.go`:

```go

// TestOnReconnect_FlipsAllInvListenerFirstSeenToTrue pins the
// refreshInvs equivalent: every listener's FirstSeen is set to true
// after the call. Map-value addressability tested implicitly. NAI-182 B4.
func TestOnReconnect_FlipsAllInvListenerFirstSeenToTrue(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := p.client.server

	// Seed 3 listeners with FirstSeen=false (post-first-emit state).
	p.invListeners = map[int]InvListener{
		100: {Com: 100, Type: 0, Source: int(p.uid), FirstSeen: false},
		101: {Com: 101, Type: 1, Source: int(p.uid), FirstSeen: false},
		102: {Com: 102, Type: -1, Source: -1, FirstSeen: false},
	}

	onReconnect(s, p)

	for com, l := range p.invListeners {
		if !l.FirstSeen {
			t.Errorf("listener[%d].FirstSeen: got false, want true after onReconnect", com)
		}
	}
}

// TestOnReconnect_OrsEntityMaskIntoMasks pins the
// masks |= entitymask resync. NAI-182 B4.
func TestOnReconnect_OrsEntityMaskIntoMasks(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := p.client.server
	p.entitymask = 0x80
	p.masks = 0x01

	onReconnect(s, p)

	if p.masks&0x80 == 0 {
		t.Errorf("p.masks after onReconnect: got %#x, want 0x80 bit set", p.masks)
	}
}

// TestOnReconnect_WithShutdownPending_EmitsRebootTimerBetweenVarpsAndTabs
// pins the TS order at Player.ts:541-547: UPDATE_REBOOT_TIMER emits
// AFTER the varp loop and BEFORE per-tab IF_SETTAB. NAI-182 B4.
func TestOnReconnect_WithShutdownPending_EmitsRebootTimerBetweenVarpsAndTabs(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := p.client.server
	s.shutdownTick = s.currentTick + 100
	// Seed a non-zero tab so we can pin "REBOOT_TIMER before IF_SETTAB".
	p.tabs[5] = 1234

	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})
	received := drainConn(t, cc)
	onReconnect(s, p)
	_ = p.client.flushWrite()
	got := <-received

	// Walk the stream; assert UPDATE_REBOOT_TIMER appears before IF_SETTAB.
	sawReboot := false
	sawIfSetTab := false
	for len(got) > 0 {
		dec := byte((int(got[0]) - int(sibling.GetNext())) & 0xff)
		if dec == gameserver.OpUpdateRebootTimer.Opcode {
			if sawIfSetTab {
				t.Error("UPDATE_REBOOT_TIMER emitted AFTER IF_SETTAB; TS order violated")
			}
			sawReboot = true
			got = got[3:] // opcode + p2
		} else if dec == gameserver.OpIfSetTab.Opcode {
			sawIfSetTab = true
			got = got[4:] // opcode + 3-byte payload
		} else {
			// Skip unknown — figure payload length from Op declaration.
			var pl int
			switch dec {
			case gameserver.OpResetClientVarCache.Opcode, gameserver.OpResetAnims.Opcode:
				pl = 0
			case gameserver.OpUpdateRunEnergy.Opcode:
				pl = 1
			case gameserver.OpUpdateStat.Opcode:
				pl = 6
			default:
				t.Fatalf("unexpected opcode %d", dec)
			}
			got = got[1+pl:]
		}
	}
	if !sawReboot {
		t.Error("UPDATE_REBOOT_TIMER never emitted despite shutdownTick != -1")
	}
	if !sawIfSetTab {
		t.Error("IF_SETTAB never emitted despite tabs[5] != 0")
	}
}
```

- [ ] **Step 10: Run all B4 tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestOnReconnect_ -v`
Expected: 3 PASS.

- [ ] **Step 11: Run full world-module suite for regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: full PASS. Watch especially for tests touching `p.reconnecting`-true paths.

- [ ] **Step 12: Commit**

```bash
git add modules/world/login_resync.go modules/world/tick.go modules/world/reconnect_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-182 B4 — onReconnect resync lifecycle + processLogins branch

Adds onReconnect(s, p) function emitting the TS-faithful resync sequence
(RESET_CLIENT_VARCACHE, varp loop, optional UPDATE_REBOOT_TIMER, closeModal,
per-tab IF_SETTAB, invListener FirstSeen flip, per-stat UPDATE_STAT × 21,
UPDATE_RUN_ENERGY, RESET_ANIMS, masks |= entitymask). Branches processLogins
on p.reconnecting. Mirrors TS Player.onReconnect (Player.ts:516-574).
DEVIATION-NAI-182-D1-RECONNECT-NO-RESTORE tracked in spec §6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6 (B5): Shutdown Consumer (processShutdown + tick wiring + graceful exit)

**Files:**
- Modify: `modules/world/player.go` (add `forceRemove bool`)
- Modify: `modules/world/server.go` (add `shutdownGraceful bool`, `gracefulExit chan struct{}`, init; modify `Server.Run()`)
- Modify: `modules/world/reboot.go` (add `processShutdown` method)
- Modify: `modules/world/tick.go` (insert `processShutdown` call at top of for-body; honor `forceRemove` in `processLogouts`)
- Modify: `modules/world/world.go` (runFn `serverDone` branch checks `shutdownGraceful`)
- Modify: `modules/world/reboot_test.go` (add shutdown consumer tests)

### Pre-flight controller checks

- [ ] **Step 1: Re-read `Server.Run()` to confirm the errChan select pattern**

Read `modules/world/server.go:395-423`. Confirm: `errChan` is local; populated by handler-loop goroutine, serveTCP goroutine, and tick-loop indirectly. The graceful-exit handshake adds a 4th sender — the tick loop itself when `s.shutdownGraceful` is true.

- [ ] **Step 2: Re-read `processLogouts` to find the force-removal injection point**

Read `modules/world/tick.go:210-260` (or wherever `processLogouts` body lives). Locate the existing `force := false` and the conditions that set `force = true`. Plan adds `if p.forceRemove { force = true }` near the top of the per-player loop.

### Add fields

- [ ] **Step 3: Add `forceRemove bool` to `Player`**

Open `modules/world/player.go`. Find the `Player` struct (line 77). Append after the existing flags section (look for `members bool`, `reconnecting bool`, or similar single-bool fields around line 300-310):

```go
	// forceRemove is set by Server.processShutdown when a player has
	// failed to logout cleanly within 1024 ticks of shutdown initiation
	// (see TS World.processShutdown, World.ts:1207-1213). When true,
	// processLogouts force-removes the player regardless of normal
	// timeout / inflight-action gates. NAI-182.
	forceRemove bool
```

- [ ] **Step 4: Add `shutdownGraceful bool` and `gracefulExit chan struct{}` to `Server`**

Open `modules/world/server.go`. Find the `Server` struct (around line 50-80). Append near `shutdownTick int` from Task 3:

```go
	// shutdownGraceful is set by Server.processShutdown when zero
	// players remain after a reboot. The tick loop returns when set,
	// and world.go runFn distinguishes this from an "unexpected" stop
	// by checking the flag before returning fmt.Errorf. NAI-182.
	shutdownGraceful bool

	// gracefulExit is closed by Server.processShutdown to unblock
	// Server.Run()'s errChan select. Distinct from s.quit (which is
	// closed by Server.Shutdown() via the dskit stoppingFn) to avoid
	// double-close panic. NAI-182.
	gracefulExit chan struct{}
```

Find the `Server` constructor (per Task 3 Step 1 pre-flight — `newServer` or `NewServer`). Add to the struct-literal initialiser:

```go
		gracefulExit: make(chan struct{}),
```

(`shutdownGraceful` defaults to false — no init needed.)

- [ ] **Step 5: Modify `Server.Run()` to select on `gracefulExit`**

In `modules/world/server.go:395-423`, find the existing `Run()` body. Replace:

```go
	return <-errChan
```

with:

```go
	select {
	case err := <-errChan:
		return err
	case <-s.gracefulExit:
		// processShutdown initiated graceful exit. Return nil; world.go
		// runFn checks s.shutdownGraceful to distinguish from
		// "unexpected" stop. NAI-182.
		return nil
	}
```

- [ ] **Step 6: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: exit 0.

### Add processShutdown + tick wiring

- [ ] **Step 7: Write failing test — `TestProcessShutdown_MarksAllConnectedPlayersForLogout`**

Append to `modules/world/reboot_test.go`:

```go

// TestProcessShutdown_MarksAllConnectedPlayersForLogout pins the
// first effect of processShutdown: every connected player has
// loggingOut=true. Mirrors TS World.processShutdown (W.ts:1199-1204).
// NAI-182 B5.
func TestProcessShutdown_MarksAllConnectedPlayersForLogout(t *testing.T) {
	p1, _ := newTestPlayer(t)
	s := p1.client.server
	p2, _ := newTestPlayer(t) // assumes newTestPlayer allocates a new slot
	_ = p2
	s.shutdownTick = s.currentTick

	s.processShutdown()

	for _, p := range s.playerLoop {
		if p == nil {
			continue
		}
		if !p.loggingOut {
			t.Errorf("player slot=%d: loggingOut not set after processShutdown", p.slot)
		}
	}
}
```

If `newTestPlayer` always returns the same player or if the test helper doesn't support multi-player setup, simplify to one player and assert that one player's `loggingOut` is true.

- [ ] **Step 8: Run, expect FAIL (undefined `processShutdown`)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessShutdown_MarksAllConnectedPlayersForLogout -v`
Expected: compile failure.

- [ ] **Step 9: Add `processShutdown` to `modules/world/reboot.go`**

Append to `modules/world/reboot.go`:

```go

// processShutdown runs at the top of s.tick() when s.shutdownTick != -1
// && s.currentTick >= s.shutdownTick. Mirrors TS World.processShutdown
// (World.ts:1198-1226). NAI-182.
func (s *Server) processShutdown() {
	// (a) For every connected player, request logout. TS calls
	// player.logout() + player.client.close() inline; goscape reuses
	// the existing logout machinery (processLogouts drain path) by
	// flagging p.loggingOut. The current tick's processLogouts will
	// then run the standard logout sequence.
	for _, p := range s.playerLoop {
		if p != nil && p.client != nil {
			p.loggingOut = true
		}
	}

	duration := s.currentTick - s.shutdownTick

	// (b) After 1024 ticks (~10 minutes at 600ms/tick), force-remove any
	// player that hasn't completed logout. Mirrors TS World.processShutdown
	// (W.ts:1207-1213). The p.forceRemove flag drives processLogouts'
	// force-branch.
	if duration >= 1024 {
		for _, p := range s.playerLoop {
			if p != nil {
				p.forceRemove = true
			}
		}
	}

	// (c) Graceful exit when zero players remain. TS calls
	// process.exit(0); goscape signals via shutdownGraceful + closes
	// gracefulExit. The tick loop returns; Server.Run() selects on
	// gracefulExit and returns nil; world.go runFn checks
	// shutdownGraceful to distinguish from "unexpected" stop.
	//
	// We deliberately do NOT close(s.quit) — the dskit stoppingFn later
	// calls Server.Shutdown() which closes s.quit; double-close would panic.
	if s.getTotalPlayers() == 0 {
		s.shutdownGraceful = true
		close(s.gracefulExit)
	}
}
```

- [ ] **Step 10: Re-run test, expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessShutdown_MarksAllConnectedPlayersForLogout -v`
Expected: PASS.

### Wire processShutdown into tick loop + processLogouts force-branch

- [ ] **Step 11: Add tick-body wiring at the top of `runTickLoopWithRate`**

Open `modules/world/tick.go`. Find the for-loop body in `runTickLoopWithRate` (line 28). At the **top** of the for-body, BEFORE `start := time.Now()`, insert:

```go
		// NAI-182 — shutdown consumer must run BEFORE any per-tick work
		// so a doomed conn doesn't receive one more tick of activity.
		// Mirrors TS World.cycle (World.ts:419-420 `if (this.shutdown)
		// this.processShutdown();`).
		if s.shutdownTick != -1 && s.currentTick >= s.shutdownTick {
			s.processShutdown()
			if s.shutdownGraceful {
				return // tick loop terminates; Server.Run() returns nil via s.gracefulExit
			}
		}
```

- [ ] **Step 12: Modify `processLogouts` to honor `p.forceRemove`**

In `modules/world/tick.go`, find `processLogouts` (around line 210). Inside the per-player loop body, after the existing `force := false` line, add:

```go
		if p.forceRemove {
			force = true
		}
```

- [ ] **Step 13: Patch `world.go runFn` to check `serv.shutdownGraceful`**

Open `modules/world/world.go`. Find the `runFn` body (line 93). Replace:

```go
		case err := <-serverDone:
			if err != nil {
				return err
			}
			return fmt.Errorf("server stopped unexpectedly")
```

with:

```go
		case err := <-serverDone:
			if err != nil {
				return err
			}
			if serv.shutdownGraceful {
				return nil // NAI-182 — ::reboot / ::slowreboot graceful exit
			}
			return fmt.Errorf("server stopped unexpectedly")
```

- [ ] **Step 14: Compile-check + run targeted test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: exit 0.

### Add shutdown-consumer tests

- [ ] **Step 15: Add 5 more shutdown tests**

Append to `modules/world/reboot_test.go`:

```go

// TestProcessShutdown_ForceRemoveAfter1024Ticks pins the duration-gate
// branch: after 1024 ticks of pending shutdown, remaining players get
// forceRemove=true. NAI-182 B5.
func TestProcessShutdown_ForceRemoveAfter1024Ticks(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := p.client.server
	p.loggingOut = true
	s.shutdownTick = s.currentTick - 1024 // 1024 ticks elapsed since shutdown init

	s.processShutdown()

	if !p.forceRemove {
		t.Errorf("p.forceRemove after 1024-tick processShutdown: got false, want true")
	}
}

// TestProcessShutdown_ForceRemoveNotSetBeforeDuration pins the
// negative-case threshold: at duration=1023, forceRemove stays false.
// NAI-182 B5.
func TestProcessShutdown_ForceRemoveNotSetBeforeDuration(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := p.client.server
	p.loggingOut = true
	s.shutdownTick = s.currentTick - 1023

	s.processShutdown()

	if p.forceRemove {
		t.Errorf("p.forceRemove at duration=1023: got true, want false (threshold is >=1024)")
	}
}

// TestProcessShutdown_ZeroPlayersTriggersGracefulExit pins the
// graceful-exit handshake: with no players, processShutdown sets
// shutdownGraceful=true and closes gracefulExit. NAI-182 B5.
func TestProcessShutdown_ZeroPlayersTriggersGracefulExit(t *testing.T) {
	s := newTestServer(t)
	s.shutdownTick = s.currentTick
	// playerLoop is empty by construction in newTestServer.

	s.processShutdown()

	if !s.shutdownGraceful {
		t.Error("shutdownGraceful: got false, want true after zero-player processShutdown")
	}
	select {
	case <-s.gracefulExit:
		// pass
	default:
		t.Error("gracefulExit channel: not closed after zero-player processShutdown")
	}
}

// TestProcessShutdown_RunsBeforeProcessLogins pins the ordering
// invariant: a player queued in s.newPlayers does NOT graduate during
// a tick where processShutdown fires the graceful-exit path. Mirrors
// TS World.cycle (W.ts:419 → 423 ordering). NAI-182 B5.
func TestProcessShutdown_RunsBeforeProcessLogins(t *testing.T) {
	s := newTestServer(t)
	s.shutdownTick = s.currentTick
	// Pre-load a fresh login candidate.
	pending := newTestPendingPlayer(t, s) // helper that builds a Player and appends to s.newPlayers
	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, pending)
	s.playersMu.Unlock()

	// Drive ONE tick body iteration. Extract a helper or call
	// runTickLoopWithRate-equivalent that yields after one iteration.
	// Simplest: call s.processShutdown() then assert pending is NOT in
	// s.players (because processShutdown's graceful-exit path returned
	// before processLogins ran).
	s.processShutdown()

	// processShutdown returns; in runTickLoopWithRate, the `return`
	// would prevent processLogins from running this tick. Assert pending
	// remains in s.newPlayers (not graduated).
	found := false
	for _, np := range s.newPlayers {
		if np == pending {
			found = true
			break
		}
	}
	if !found {
		t.Error("pending player removed from s.newPlayers despite processShutdown graceful-exit path")
	}
}
```

`newTestPendingPlayer` may not exist. If absent, write it as a local helper in the test file:

```go
// newTestPendingPlayer constructs a Player suitable for s.newPlayers
// without going through the full login codec. NAI-182 B5 test helper.
func newTestPendingPlayer(t *testing.T, s *Server) *Player {
	t.Helper()
	p := newPlayer(/* ... fill per existing newTestPlayer pattern ... */)
	return p
}
```

If you can't construct a Player cleanly, drop the `TestProcessShutdown_RunsBeforeProcessLogins` test and instead pin the ordering by inspection — add a code-comment in `runTickLoopWithRate` noting "processShutdown MUST run before processClientsIn / processLogouts / processLogins" and reference the spec.

- [ ] **Step 16: Run all B5 tests + full world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessShutdown_" -v`
Expected: PASS (4-5 tests).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: full PASS.

- [ ] **Step 17: Commit**

```bash
git add modules/world/player.go modules/world/server.go modules/world/reboot.go modules/world/tick.go modules/world/world.go modules/world/reboot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-182 B5 — processShutdown consumer + graceful exit handshake

Adds Server.processShutdown method, p.forceRemove field driving
processLogouts force-branch, and the world-module graceful-exit
handshake (s.shutdownGraceful + s.gracefulExit channel, Server.Run
select extension, world.go runFn nil-on-graceful patch). Processes
shutdown at the TOP of the tick body, before any per-tick work, so a
doomed conn doesn't receive one more tick of activity. Mirrors TS
World.processShutdown (W.ts:1198-1226) + World.cycle (W.ts:419-420).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7 (B6): Staff-Cheats (::reboot, ::slowreboot, ::serverdrop)

**Files:**
- Modify: `modules/world/handlers_game.go` (add `math` import; append 3 switch arms)
- Modify: `modules/world/handlers_game_test.go` (append cheat-dispatch tests)

### Pre-flight controller checks

- [ ] **Step 1: Confirm TS Player.terminate() body for ::serverdrop semantics**

Read `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts` and grep for `terminate(`:

```bash
grep -n "terminate(" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/NetworkPlayer.ts | head
```

Read the function body. The spec claim is "close TCP conn, leave p.reconnecting=true for next login". If TS also writes a session log or queues a save, document in commit message but skip those for now (out-of-scope subsystems).

- [ ] **Step 2: Confirm parts[1] argument-parsing convention in `handleClientCheat`**

Read `modules/world/handlers_game.go:347-360` (the existing switch arms). Confirm: `parts := strings.SplitN(cheat, " ", 2)` and existing `case "tele":` reads `parts[1]` for args.

### Add cheat arms

- [ ] **Step 3: Write failing test — `TestHandleClientCheat_Reboot_TriggersImmediateBroadcast`**

Append to `modules/world/handlers_game_test.go`:

```go

// TestHandleClientCheat_Reboot_TriggersImmediateBroadcast pins the
// ::reboot cheat: sets s.shutdownTick = s.currentTick (duration=0) and
// broadcasts UPDATE_REBOOT_TIMER(0) to all players in s.playerLoop.
// Mirrors TS ClientCheatHandler.ts:360-364. NAI-182 B6.
func TestHandleClientCheat_Reboot_TriggersImmediateBroadcast(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := p.client.server
	p.staffModLevel = 2 // gate at handlers_game.go:365

	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})
	startTick := s.currentTick

	received := drainConn(t, cc)
	dispatchTeleCheat(t, p, "reboot") // helper dispatches arbitrary cheats; rename if needed
	_ = p.client.flushWrite()
	got := <-received

	if s.shutdownTick != startTick {
		t.Errorf("shutdownTick after ::reboot: got %d, want %d", s.shutdownTick, startTick)
	}

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(sibling.GetNext())) & 0xff),
		0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("broadcast bytes: got %#x, want %#x", got, want)
	}
}
```

Note: the existing `dispatchTeleCheat` helper (`handlers_game_test.go:384-394`) is named for tele but its body dispatches whatever string you pass. Rename to `dispatchCheat` in this task (single global find-replace) OR add an alias. If renaming is risky, just use the existing function name; it's a string parameter.

- [ ] **Step 4: Run, expect FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleClientCheat_Reboot_TriggersImmediateBroadcast -v`
Expected: FAIL — handler doesn't recognize "reboot" yet.

- [ ] **Step 5: Modify `modules/world/handlers_game.go` — add `math` import**

In the `import` block (line 1-15ish), add `"math"`. Resulting imports include:

```go
import (
	"math"
	"strconv"
	"strings"
	// ... existing imports ...
)
```

- [ ] **Step 6: Append 3 switch arms in `handleClientCheat`**

Find the existing switch at `handlers_game.go:360` (`case "say":`). After the existing `case "tele":` arm (line ~395 area — verify exact line at pre-flight), append:

```go
	case "reboot":
		// Mirrors TS ClientCheatHandler.ts:360-364. duration=0 means
		// immediate shutdown (shutdownTick = currentTick). NAI-182.
		// DEVIATION-NAI-182-D2 — TS gates this on Environment.NODE_PRODUCTION;
		// goscape uses staffModLevel>=2 which is enforced at the top of
		// handleClientCheat.
		s := p.client.server
		s.rebootTimer(0)

	case "slowreboot":
		// Mirrors TS ClientCheatHandler.ts:365-373. Default 30 seconds
		// when args[0] is missing or unparseable (TS tryParseInt
		// semantics). Formula: ticks = ceil(seconds * 1000 / 600).
		// NAI-182.
		seconds := 30
		if len(parts) >= 2 {
			if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				seconds = v
			}
		}
		ticks := int(math.Ceil(float64(seconds) * 1000.0 / 600.0))
		s := p.client.server
		s.rebootTimer(ticks)

	case "serverdrop":
		// Mirrors TS ClientCheatHandler.ts:374-376 player.terminate().
		// Closes the TCP conn without removing the player from
		// s.players; the next reconnect (OpReqGameReconnect) hits this
		// player's slot and runs the onReconnect path. NAI-182.
		if p.client != nil && p.client.conn != nil {
			_ = p.client.conn.Close()
		}
```

- [ ] **Step 7: Re-run target test, expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleClientCheat_Reboot_TriggersImmediateBroadcast -v`
Expected: PASS.

- [ ] **Step 8: Append 5 more cheat tests**

Append to `modules/world/handlers_game_test.go`:

```go

// TestHandleClientCheat_SlowReboot_NoArgsDefaultsTo30Seconds pins the
// missing-arg fallback: 30 seconds → ceil(30000/600) = 50 ticks.
// Mirrors TS tryParseInt(args[0], 30) semantics. NAI-182 B6.
func TestHandleClientCheat_SlowReboot_NoArgsDefaultsTo30Seconds(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := p.client.server
	p.staffModLevel = 2
	_, sibling := isaacPair([4]uint32{1, 2, 3, 4})

	startTick := s.currentTick
	received := drainConn(t, cc)
	dispatchTeleCheat(t, p, "slowreboot")
	_ = p.client.flushWrite()
	got := <-received

	if s.shutdownTick != startTick+50 {
		t.Errorf("shutdownTick after ::slowreboot (no args): got %d, want %d", s.shutdownTick, startTick+50)
	}

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(sibling.GetNext())) & 0xff),
		0x00, 0x32,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("broadcast bytes: got %#x, want %#x", got, want)
	}
}

// TestHandleClientCheat_SlowReboot_WithSecondsArg pins arg-parsing:
// ::slowreboot 60 → ceil(60000/600) = 100 ticks. NAI-182 B6.
func TestHandleClientCheat_SlowReboot_WithSecondsArg(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := p.client.server
	p.staffModLevel = 2

	startTick := s.currentTick
	dispatchTeleCheat(t, p, "slowreboot 60")

	if s.shutdownTick != startTick+100 {
		t.Errorf("shutdownTick after ::slowreboot 60: got %d, want %d", s.shutdownTick, startTick+100)
	}
}

// TestHandleClientCheat_SlowReboot_NonIntegerArgFallsBackToDefault
// pins the parse-failure fallback. NAI-182 B6.
func TestHandleClientCheat_SlowReboot_NonIntegerArgFallsBackToDefault(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := p.client.server
	p.staffModLevel = 2

	startTick := s.currentTick
	dispatchTeleCheat(t, p, "slowreboot abc")

	if s.shutdownTick != startTick+50 {
		t.Errorf("shutdownTick after ::slowreboot abc: got %d, want %d (default 30s → 50 ticks)", s.shutdownTick, startTick+50)
	}
}

// TestHandleClientCheat_ServerDrop_ClosesConn pins ::serverdrop:
// TCP conn is closed; player stays in s.players. Mirrors TS
// ClientCheatHandler.ts:374-376 player.terminate(). NAI-182 B6.
func TestHandleClientCheat_ServerDrop_ClosesConn(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := p.client.server
	p.staffModLevel = 2
	slotBefore := p.slot

	dispatchTeleCheat(t, p, "serverdrop")

	// Assert player still occupies slot.
	if s.players[slotBefore] != p {
		t.Errorf("player removed from slot %d after ::serverdrop; should remain for reconnect", slotBefore)
	}
	// Conn-close assertion: write to conn should fail. Test setup uses
	// a pipe-like conn; closing it makes subsequent Write return an error.
	if _, err := p.client.conn.Write([]byte{0}); err == nil {
		t.Error("p.client.conn.Write succeeded after ::serverdrop; expected closed-conn error")
	}
}

// TestHandleClientCheat_RebootCheats_StaffGate pins the staff-mod
// gate: staffModLevel<2 means ::reboot is silently ignored. NAI-182 B6.
func TestHandleClientCheat_RebootCheats_StaffGate(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := p.client.server
	p.staffModLevel = 1 // below the gate (gate is >=2)

	startTick := s.currentTick
	dispatchTeleCheat(t, p, "reboot")

	if s.shutdownTick != -1 {
		t.Errorf("shutdownTick after ::reboot with staffModLevel=1: got %d, want -1 (gate blocked)", s.shutdownTick)
	}
	_ = startTick
}
```

- [ ] **Step 9: Run all B6 tests + full world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleClientCheat_ -v`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: full PASS.

- [ ] **Step 10: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-182 B6 — ::reboot / ::slowreboot / ::serverdrop staff cheats

Wires 3 ServerProt-coupled cheats into handleClientCheat:
- ::reboot → s.rebootTimer(0) immediate shutdown (TS CCH:360-364)
- ::slowreboot <s> → s.rebootTimer(ceil(s*1000/600)) (TS CCH:365-373)
- ::serverdrop → close TCP conn, retain player slot (TS CCH:374-376)
Existing staffModLevel>=2 gate covers all three; DEVIATION-NAI-182-D2
tracks the divergence from TS Environment.NODE_PRODUCTION gate.
DEVIATION-NAI-182-D3 lists the other 25 unported ClientCheatHandler
cheats.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: NAI-182 close + memory entries + tracker hygiene

**Files:**
- Create: appropriate memory files under `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/`
- Update: `MEMORY.md`

### Save non-derivable learnings

- [ ] **Step 1: Save memory entry — login-byte-pin tests must enumerate varp emits**

Per spec §8 close-time memory entries, create a `feedback`-type memory:

```bash
ls /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/
```

Then write a file like `login_byte_pin_varp_enumeration.md` with frontmatter (type=feedback) capturing: "tests that drive processLogins and assert byte equality must enumerate transmit-true varps in the expected output". Add a one-line entry to MEMORY.md.

- [ ] **Step 2: Save memory entry — processShutdown ordering invariant**

Write `processshutdown_runs_before_logins.md` (type=feedback) — "Server.processShutdown MUST run BEFORE processClientsIn / processLogouts / processLogins in the tick body; reversing leaks one tick of activity to doomed conns". Add MEMORY.md line.

- [ ] **Step 3: Save memory entry — shutdownGraceful handshake**

Write `world_runfn_graceful_exit_handshake.md` (type=feedback) — "world.go runFn treats nil-error from Run() as 'unexpected stop' by default. Self-initiated graceful exit requires both s.shutdownGraceful=true AND closing s.gracefulExit (NOT s.quit — that's owned by Shutdown() and would double-close)". Add MEMORY.md line.

### Tracker / commit hygiene

- [ ] **Step 4: Verify all 5 deviation tags are searchable from production code**

```bash
rg "DEVIATION-NAI-182" modules/ pkg/
```

Expected: D1, D2, D3, D4, D5 each have at least one in-code citation. If any is missing from code (only in spec/plan), add a doc-comment to a relevant production site.

- [ ] **Step 5: Final full-suite run with race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: full PASS.

- [ ] **Step 6: NAI-182 close commit**

If memory files were added:

```bash
git add /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-182 — misc ServerProt cluster + onReconnect + shutdown consumer + reboot cheats

Closes NAI-182. Memory entries:
- login_byte_pin_varp_enumeration.md
- processshutdown_runs_before_logins.md
- world_runfn_graceful_exit_handshake.md

Deviations tracked in spec §6:
- D1 RECONNECT-NO-RESTORE (clears when PlayerLoading lands)
- D2 CHEAT-NODE-PRODUCTION-GATE
- D3 25 unported ClientCheatHandler cheats
- D4 IFCLOSE-LOGIN-NOT-EMITTED
- D5 SOCIAL-CLUSTER-PRE-PID-NOT-EMITTED

Sibling cluster (UPDATE_FRIENDLIST / UPDATE_IGNORELIST / MESSAGE_PRIVATE
server-bound / CHAT_FILTER_SETTINGS) deferred to a future sub-spec.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Checklist (run after writing the plan)

- **Spec coverage:**
  - §3.1 opcode declarations → Task 1 ✓
  - §3.2 encoders → Task 2 ✓
  - §3.3 reboot infra → Task 3 ✓
  - §3.4 processShutdown + D2 → Task 6 ✓
  - §3.4.1 world.go runFn handshake → Task 6 Step 13 ✓
  - §3.5 processLogins fresh-login wiring → Task 4 ✓
  - §3.6 onReconnect lifecycle → Task 5 ✓
  - §3.7 D3-narrowed cheats → Task 7 ✓
  - §5-7 fixture audit risk → Task 4 Steps 1-2 ✓
  - §8 close-time memory entries → Task 8 ✓
- **Placeholder scan:** No TBD / TODO / "implement later". One conditional helper-write fallback in Task 6 Step 15 (`newTestPendingPlayer`) is acceptable — it provides a written alternative path if the helper turns out to be unwritable.
- **Type consistency:**
  - `sendUpdateRebootTimer(p, ticks int)` — used identically in Task 2, 3, 4, 7. ✓
  - `s.shutdownTick int` — referenced identically across Tasks 3-6. ✓
  - `s.shutdownGraceful bool` and `s.gracefulExit chan struct{}` — declared in Task 6 Step 4, used in Steps 5, 9. ✓
  - `p.forceRemove bool` — declared Task 6 Step 3, used Step 12. ✓
  - `onReconnect(s, p)` signature — declared Task 5 Step 6, used Task 5 Step 7. ✓
