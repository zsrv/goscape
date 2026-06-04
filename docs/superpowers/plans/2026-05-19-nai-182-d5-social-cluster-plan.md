# Implementation plan — NAI-182-D5 social-cluster ServerGameProt port

**Date:** 2026-05-19
**Spec:** `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md`
**HEAD at plan:** `162fda5f` (spec commit; tree clean for parent `db0c894a`).
**Execution model:** `superpowers:subagent-driven-development`. The controller (parent session) dispatches each task to an implementer subagent, then to per-task spec-and-quality reviewers (per slice-7 / B3 pattern). Each task = a small TDD cycle with a verifiable gate.
**Total tasks:** 8.
**Tags retired on close:** `NAI-S4A-D-NO-INGAME-PACKET-EMIT`, `NAI-S4B-D-NO-INGAME-PM-EMIT`, `DEVIATION-NAI-182-D5-SOCIAL-CLUSTER-PRE-PID-NOT-EMITTED`.
**Tags opened on close:** `DEVIATION-NAI-182-D5-NO-WORDENC-FILTER` (retires-when-wordenc-filter-ports), `DEVIATION-NAI-182-D5-NO-DEFENSIVE-IGNORELIST-LOGIN-EMIT` (permanent), `DEVIATION-NAI-182-D5-CHAT-FILTER-NO-RESTORE` (retires-when-restore-lands).

## Discipline (controller MUST inline verbatim in every implementer prompt)

1. **NEVER `git checkout` / `git restore` tracked files.**
2. **Pre-commit `git status` + post-commit `git show --stat HEAD`.** Concurrent shell edits can sneak into the index between session-start and `git commit`. Recover via `git reset --mixed HEAD~1` — never amend. ([[git-pre-commit-status-check]])
3. **Shell prefix:** `unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"` once per shell session.
4. **Go prefix:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` on EVERY `go` invocation.
5. **Commit flag:** `git commit --no-gpg-sign` on every commit.
6. **Test files use `_test.go` suffix** (build-tag isolation).
7. **`go test -race` always** — the production seam in T3–T5 spans gRPC stream goroutine + tick goroutine via `relayActionQueue`. Race detector catches mis-placed writeOut calls.
8. **Modern Go:** `range N` (not `for i := 0; i < N; i++`), `cmp.Or`, `min`/`max`, `slices.Concat`, `maps.All` where natural. Apply the `use-modern-go` skill before writing any Go code.
9. **Stale-IDE-LSP `go list` "No packages found" diagnostics are environmental** — ignore. `go vet ./...` + `go build ./...` are the authoritative checks.
10. **No proto changes** — D5 is world-side packet emit, not gRPC. If a task seems to need a proto edit, STOP and re-read the spec — the FriendsDispatcher signatures are stable.

## Pre-flight gate (controller, before T1 dispatch)

```bash
cd /home/owner/Code/github.com/zsrv/goscape
unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... ./modules/friends/... -count=1 -timeout 300s
git log --oneline 162fda5f..HEAD                      # expect zero (HEAD == 162fda5f)
```

Tree should be clean (only standing untracked noise). Race-detector clean. Any failure here means the parent state has drifted — investigate before dispatching.

## Task-1: Opcode declarations

**Goal:** Add 4 `Op` entries to `pkg/io/protocol/game/server/prot.go` adjacent to the NAI-182 misc cluster. No callers yet; compiles clean.

**Files:**
- `pkg/io/protocol/game/server/prot.go` (modify)

**TDD cycle:** Data-only — no test. Compile is the gate (precedent: NAI-181 LAST_LOGIN_INFO; NAI-182 misc T0).

**Implementation.** Pre-flight: open `pkg/io/protocol/game/server/prot.go` and locate the NAI-182 misc group anchored on `OpUpdatePid` (verified at line 147 at HEAD). Append the new "social cluster" group immediately AFTER the NAI-182 misc group (after line 174, after `OpUpdateRebootTimer`). Verbatim block:

```go
// OpUpdateFriendList carries one friend-entry update. Fixed 9-byte
// payload: p8(username37) + p1(worldId). worldId == 0 means the friend
// is offline / hidden. Emitted once per entry by the friends-server
// dispatcher (one packet per FriendEntry in the FriendlistUpdate batch).
// Mirrors TS ServerGameProt.UPDATE_FRIENDLIST (152, 9) and
// UpdateFriendListEncoder.ts.
OpUpdateFriendList = Op{Opcode: 152, PayloadSize: 9}

// OpUpdateIgnoreList carries the complete ignorelist snapshot. Variable
// 2-byte-length-prefixed payload: p8(username37) × N. Emitted on every
// ignorelist mutation; the entire list is re-sent rather than a delta.
// Mirrors TS ServerGameProt.UPDATE_IGNORELIST (21, -2) and
// UpdateIgnoreListEncoder.ts.
OpUpdateIgnoreList = Op{Opcode: 21, PayloadSize: -2}

// OpChatFilterSettings carries the player's chat-filter mode triple.
// Fixed 3-byte payload: p1(publicChat) + p1(privateChat) + p1(tradeDuel).
// Emitted once at onLogin (before UpdatePid). Mirrors TS
// ServerGameProt.CHAT_FILTER_SETTINGS (32, 3) and
// ChatFilterSettingsEncoder.ts.
OpChatFilterSettings = Op{Opcode: 32, PayloadSize: 3}

// OpMessagePrivate carries one inbound private-chat delivery to the
// recipient. Variable 1-byte-length-prefixed payload:
// p8(fromUsername37) + p4(pmId) + p1(staffLvlAdjusted) +
// WordPack.pack(chat). staffLvlAdjusted = staffLvl > 0 ? staffLvl + 1 :
// staffLvl. Emitted by the friends-server dispatcher on
// PrivateMessageDelivery. Mirrors TS ServerGameProt.MESSAGE_PRIVATE
// (41, -1) and MessagePrivateEncoder.ts.
OpMessagePrivate = Op{Opcode: 41, PayloadSize: -1}
```

**Gates:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Both zero output. No tests to run; data-only.

**Commit:** `feat(world): NAI-182-D5 T1 — social-cluster opcode declarations`

Spec: `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md`.

## Task-2: Encoder send-functions (RED → GREEN)

**Goal:** Add 4 top-level send-functions in a new file `modules/world/friends_emit.go`. Add 8 byte-pin tests in `modules/world/friends_emit_test.go`. Each test asserts the exact wire bytes the encoder produces.

**Files:**
- `modules/world/friends_emit.go` (new)
- `modules/world/friends_emit_test.go` (new)

**TDD cycle:** Bundle red → green for all 4 encoders (each is trivial — single-function file mirrors the `login_resync.go` / `reboot.go` pattern). Implementer writes ALL 8 tests first (will fail to compile since functions don't exist), then writes the 4 send-functions, then runs tests to green.

**Test fixtures (verbatim — runnable as-is).** Pattern matches `modules/world/login_resync_test.go:11-71`. Helper imports: `gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"`, `io2 "github.com/zsrv/goscape/pkg/io/isaac"`, `"github.com/zsrv/goscape/pkg/io/packet"`, `"github.com/zsrv/goscape/pkg/wordenc/wordpack"`, `"testing"`.

```go
package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// TestSendUpdateFriendList_EmitsExactByteSequence pins the wire bytes of
// sendUpdateFriendList. Fixed 9-byte payload: p8(username37) + p1(worldId).
func TestSendUpdateFriendList_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendUpdateFriendList(p, 0x0102030405060708, 7)
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpUpdateFriendList.Opcode) + int(enc.GetNext())) & 0xff),
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x07,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendUpdateIgnoreList_EmitsExactByteSequence pins UPDATE_IGNORELIST
// with a 2-entry snapshot. Variable 2-byte-length-prefixed payload.
func TestSendUpdateIgnoreList_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendUpdateIgnoreList(p, []uint64{0x0102030405060708, 0xAABBCCDDEEFF0011})
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpUpdateIgnoreList.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x10, // 2-byte BE length = 16 bytes (2 entries × 8)
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendUpdateIgnoreList_EmptyListEmitsZeroLengthPayload pins the
// no-entries case: opcode + `00 00` length prefix + zero payload.
// Mirrors TS `player.write(new UpdateIgnoreList([]))`.
func TestSendUpdateIgnoreList_EmptyListEmitsZeroLengthPayload(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendUpdateIgnoreList(p, nil)
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpUpdateIgnoreList.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x00,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendChatFilterSettings_EmitsExactByteSequence pins CHAT_FILTER_SETTINGS.
// Fixed 3-byte payload: p1(public) + p1(private) + p1(trade).
func TestSendChatFilterSettings_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendChatFilterSettings(p, 2, 1, 3)
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
		0x02, 0x01, 0x03,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendMessagePrivate_EmitsExactByteSequence pins MESSAGE_PRIVATE.
// Variable 1-byte-length-prefixed payload:
//   p8(from) + p4(pmId) + p1(staffLvl) + WordPack.pack(chat).
// staffLvl=0 ⇒ wire byte 00.
func TestSendMessagePrivate_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Compute the wordpacked bytes for "hi" exactly the way the encoder will.
	wpBuf := packet.NewPacket(nil)
	wordpack.Pack(wpBuf, "hi")
	wpBytes := wpBuf.Bytes()

	received := drainConn(t, cc)
	sendMessagePrivate(p, 0x0102030405060708, 0xDEADBEEF, 0, "hi")
	p.client.flushWrite()

	got := <-received
	header := []byte{
		byte((int(gameserver.OpMessagePrivate.Opcode) + int(enc.GetNext())) & 0xff),
		// 1-byte length prefix: 8 (from) + 4 (pmId) + 1 (staffLvl) + len(wpBytes).
		byte(8 + 4 + 1 + len(wpBytes)),
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0xDE, 0xAD, 0xBE, 0xEF,
		0x00, // staffLvl=0, no adjustment
	}
	want := append(header, wpBytes...)

	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendMessagePrivate_StaffLvlAdjustmentPositive pins the TS-faithful
// `staffLvl > 0 ⇒ +1` adjustment. staffLvl=2 ⇒ wire byte 03.
func TestSendMessagePrivate_StaffLvlAdjustmentPositive(t *testing.T) {
	p, cc := newTestPlayer(t)
	_, _ = isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendMessagePrivate(p, 1, 2, 2, "x")
	p.client.flushWrite()

	got := <-received
	// got = [encrypted-opcode, length, 8-byte-from, 4-byte-pmId, staffLvlByte, wpBytes...].
	// Offset 1+1+8+4 = 14 is the staffLvl byte.
	if got[14] != 0x03 {
		t.Fatalf("staffLvl byte: got 0x%02x, want 0x03 (2 + 1 adjustment)", got[14])
	}
}

// TestSendMessagePrivate_StaffLvlAdjustmentNegative pins that the
// adjustment ONLY applies when staffLvl > 0. staffLvl=-1 ⇒ wire 0xFF.
func TestSendMessagePrivate_StaffLvlAdjustmentNegative(t *testing.T) {
	p, cc := newTestPlayer(t)
	_, _ = isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendMessagePrivate(p, 1, 2, -1, "x")
	p.client.flushWrite()

	got := <-received
	if got[14] != 0xFF {
		t.Fatalf("staffLvl byte: got 0x%02x, want 0xFF (-1, no adjustment)", got[14])
	}
}
```

(Note: the 8th test `TestSendMessagePrivate_StaffLvlAdjustmentZero` is folded into `TestSendMessagePrivate_EmitsExactByteSequence` — the staffLvl=0 case is the main byte-pin, no need for a separate test.)

**Implementation file (verbatim).**

```go
// modules/world/friends_emit.go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// sendUpdateFriendList writes one UPDATE_FRIENDLIST packet for a single
// friend-entry update. Callers loop over entries; one packet per entry.
// worldId == 0 conveys offline/hidden per slice-3 friends-server contract.
// Mirrors TS UpdateFriendListEncoder (p8(name); p1(nodeId)).
func sendUpdateFriendList(p *Player, username37 uint64, worldId int) {
	buf := packet.NewPacket(nil)
	buf.P8(username37)
	buf.P1(uint8(worldId))
	p.writeOut(gameserver.OpUpdateFriendList, buf.Bytes())
}

// sendUpdateIgnoreList writes one UPDATE_IGNORELIST packet carrying the
// complete ignorelist snapshot. Mirrors TS UpdateIgnoreListEncoder
// (for name in names: p8(name)). Empty slice produces a zero-length
// payload (still emitted; matches TS `player.write(new UpdateIgnoreList([]))`).
func sendUpdateIgnoreList(p *Player, ignored []uint64) {
	buf := packet.NewPacket(nil)
	for _, name := range ignored {
		buf.P8(name)
	}
	p.writeOut(gameserver.OpUpdateIgnoreList, buf.Bytes())
}

// sendChatFilterSettings writes one CHAT_FILTER_SETTINGS packet carrying
// the chat-mode triple. Mirrors TS ChatFilterSettingsEncoder
// (p1(publicChat); p1(privateChat); p1(tradeDuel)).
func sendChatFilterSettings(p *Player, publicChat, privateChat, tradeDuel int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(publicChat))
	buf.P1(uint8(privateChat))
	buf.P1(uint8(tradeDuel))
	p.writeOut(gameserver.OpChatFilterSettings, buf.Bytes())
}

// sendMessagePrivate writes one MESSAGE_PRIVATE packet to the recipient.
// from is the sender's username37. pmId is the friends-server-assigned
// PM correlation id. staffLvl is the sender's staff level; the wire
// applies the TS-faithful `+1 if > 0` adjustment so the client renders
// the correct staff icon. chat is the unpacked text; goscape
// WordPack.Pack's it here for the wire.
//
// DEVIATION-NAI-182-D5-NO-WORDENC-FILTER — TS calls
// `WordPack.pack(buf, WordEnc.filter(message.msg))`; goscape has no
// WordEnc.filter port yet (only WordPack). The chat is packed verbatim.
// Retires when wordenc filter is ported.
func sendMessagePrivate(p *Player, from uint64, pmId uint32, staffLvl int32, chat string) {
	adjusted := staffLvl
	if adjusted > 0 {
		adjusted += 1
	}
	buf := packet.NewPacket(nil)
	buf.P8(from)
	buf.P4(uint32(pmId))
	buf.P1(uint8(adjusted))
	wordpack.Pack(buf, chat)
	p.writeOut(gameserver.OpMessagePrivate, buf.Bytes())
}
```

**Gates:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestSendUpdateFriendList|TestSendUpdateIgnoreList|TestSendChatFilterSettings|TestSendMessagePrivate' -count=1 -timeout 60s
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
```

All 7 tests pass. `go vet` zero output.

**Commit:** `feat(world): NAI-182-D5 T2 — social-cluster encoders + byte-pins`

Spec: `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md`.

## Task-3: emitFriendsDispatcher.OnFriendlistUpdate (RED → GREEN)

**Goal:** Add `emitFriendsDispatcher` struct + constructor + `OnFriendlistUpdate` method in `modules/world/bridges.go`. Replace `newSlogFriendsDispatcher` wire-up at `server.go:292` with `newEmitFriendsDispatcher`. Add 3 tests.

**Files:**
- `modules/world/bridges.go` (modify — add struct + method; keep slogFriendsDispatcher)
- `modules/world/server.go` (modify — one line at :292)
- `modules/world/friends_dispatcher_emit_test.go` (new)

**Test fixtures.** Pattern: each test seeds a `*Server` via `newTestServer(t)`, attaches a player via `newTestPlayer(t)` with a known username37, calls the dispatcher method, drains the relay action queue via `s.drainRelayActions()` (which runs on the test goroutine — simulating the tick), then asserts wire bytes.

```go
package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/friendspb"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestEmitFriendsDispatcher_OnFriendlistUpdate_EnqueuesOnePacketPerEntry
// pins that the dispatcher closes over a single closure that emits one
// UPDATE_FRIENDLIST packet per FriendEntry — matching TS World.ts:1964-1966
// (for-loop over data.friends).
func TestEmitFriendsDispatcher_OnFriendlistUpdate_EnqueuesOnePacketPerEntry(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Register the player so lookupPlayerByUsername37 finds it.
	const viewer uint64 = 0x1111222233334444
	p.username37 = viewer
	s.playersMu.Lock()
	s.players = append(s.players, p)
	s.playersMu.Unlock()

	d := newEmitFriendsDispatcher(s, s.log)
	received := drainConn(t, cc)

	d.OnFriendlistUpdate(viewer, []*friendspb.FriendEntry{
		{Username37: 0x0102030405060708, WorldId: 1},
		{Username37: 0xAABBCCDDEEFF0011, WorldId: 0},
	})
	s.drainRelayActions()
	p.client.flushWrite()

	got := <-received
	// Expect two UPDATE_FRIENDLIST packets back-to-back: each is
	// 1 opcode byte + 8 username37 + 1 worldId = 10 bytes.
	want := []byte{
		byte((int(gameserver.OpUpdateFriendList.Opcode) + int(enc.GetNext())) & 0xff),
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x01,
		byte((int(gameserver.OpUpdateFriendList.Opcode) + int(enc.GetNext())) & 0xff),
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11, 0x00,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestEmitFriendsDispatcher_OnFriendlistUpdate_MissingPlayerNoEmit pins
// that the dispatcher silently drops events for a viewer not in s.players.
func TestEmitFriendsDispatcher_OnFriendlistUpdate_MissingPlayerNoEmit(t *testing.T) {
	s := newTestServer(t)
	d := newEmitFriendsDispatcher(s, s.log)

	// No players registered.
	d.OnFriendlistUpdate(0xDEADBEEF, []*friendspb.FriendEntry{{Username37: 1, WorldId: 1}})
	s.drainRelayActions() // closure runs, lookup returns nil, no-op.

	// No panic, no error — just verify the queue is now empty.
	select {
	case <-s.relayActionQueue:
		t.Fatal("relayActionQueue should be drained")
	default:
	}
}

// TestEmitFriendsDispatcher_OnFriendlistUpdate_LogoutBetweenEnqueueAndDrain
// pins that a player who logs out between event arrival and drain is
// silently skipped (no panic, no orphan write).
func TestEmitFriendsDispatcher_OnFriendlistUpdate_LogoutBetweenEnqueueAndDrain(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s

	const viewer uint64 = 0xCAFE
	p.username37 = viewer
	s.playersMu.Lock()
	s.players = append(s.players, p)
	s.playersMu.Unlock()

	d := newEmitFriendsDispatcher(s, s.log)
	d.OnFriendlistUpdate(viewer, []*friendspb.FriendEntry{{Username37: 1, WorldId: 1}})

	// Player logs out before drain.
	s.playersMu.Lock()
	s.players = s.players[:0]
	s.playersMu.Unlock()

	// Drain — closure runs, lookup returns nil, no-op.
	s.drainRelayActions()
}
```

**Implementation.** In `modules/world/bridges.go`, immediately AFTER the existing `slogFriendsDispatcher` block (after line 135), append the new struct + constructor + first method:

```go
// emitFriendsDispatcher is the production FriendsDispatcher. Each
// method enqueues a closure on s.relayActionQueue so the writeOut on
// the resolved Player runs on the tick goroutine (the only goroutine
// allowed to touch Player.client.bufw + ISAAC stream). The recipient
// is resolved inside the closure (not at enqueue time) so a player who
// logs out between event arrival and tick-drain is correctly skipped.
//
// slogFriendsDispatcher remains the default fallback for tests and
// when friends-server is disabled — that path never reaches a real
// Player.
//
// Retires NAI-S4A-D-NO-INGAME-PACKET-EMIT / NAI-S4B-D-NO-INGAME-PM-EMIT
// (NAI-182-D5, 2026-05-19).
type emitFriendsDispatcher struct {
	s   *Server
	log *slog.Logger
}

func newEmitFriendsDispatcher(s *Server, log *slog.Logger) FriendsDispatcher {
	return &emitFriendsDispatcher{s: s, log: log}
}

func (d *emitFriendsDispatcher) OnFriendlistUpdate(viewer uint64, entries []*friendspb.FriendEntry) {
	d.log.Debug("friends dispatch: friendlist update",
		slog.Uint64("viewer", viewer),
		slog.Int("entries", len(entries)))
	d.s.enqueueRelayAction(func() {
		p := d.s.lookupPlayerByUsername37(viewer)
		if p == nil {
			return
		}
		for _, e := range entries {
			sendUpdateFriendList(p, e.Username37, int(e.WorldId))
		}
	})
}

// OnIgnorelistUpdate and OnPrivateMessage land in T4 and T5 — they share
// the same enqueue+lookup+emit shape. Defining the interface methods
// in stages keeps each TDD task self-contained.

var _ FriendsDispatcher = (*emitFriendsDispatcher)(nil)
```

**However** — Go interfaces require all methods to be present for the `var _ FriendsDispatcher = ...` compile-check to pass. The cleanest cadence: T3 adds ALL THREE methods as STUBS first (so the var-assert compiles), with T4 and T5 each replacing one stub with the real implementation and adding tests. Adjust:

- T3 lands: struct + constructor + `OnFriendlistUpdate` (real) + `OnIgnorelistUpdate` and `OnPrivateMessage` stubs that **call through to slog only** (mirroring `slogFriendsDispatcher`).
- T4 replaces `OnIgnorelistUpdate` stub with the real impl.
- T5 replaces `OnPrivateMessage` stub with the real impl.

Stub bodies for T3 (these become the T4/T5 real impls):

```go
func (d *emitFriendsDispatcher) OnIgnorelistUpdate(viewer uint64, ignored []uint64) {
	// Stub — replaced in T4. For T3 we log-only to keep the interface
	// satisfied without dispatching enqueue side-effects.
	d.log.Debug("friends dispatch: ignorelist update (T3 stub)",
		slog.Uint64("viewer", viewer),
		slog.Int("ignored", len(ignored)))
}

func (d *emitFriendsDispatcher) OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string) {
	// Stub — replaced in T5.
	d.log.Debug("friends dispatch: private message (T3 stub)",
		slog.Uint64("target", target),
		slog.Uint64("from", from),
		slog.Uint64("pm_id", uint64(pmId)))
}
```

**`server.go:292` wire-up flip.** Open `modules/world/server.go`, locate `s.friendsDispatcher = newSlogFriendsDispatcher(s.log)` at line 292, replace with:

```go
s.friendsDispatcher = newEmitFriendsDispatcher(s, s.log)
```

**Gates:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestEmitFriendsDispatcher_OnFriendlistUpdate' -count=1 -timeout 60s
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
```

3 new tests pass. Full world-package suite still green (the wire-up flip means every subscriber-using test now hits emitFriendsDispatcher — confirm no regression).

**Commit:** `feat(world): NAI-182-D5 T3 — emitFriendsDispatcher.OnFriendlistUpdate`

Spec: `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md`.

## Task-4: emitFriendsDispatcher.OnIgnorelistUpdate (RED → GREEN)

**Goal:** Replace the T3 stub for `OnIgnorelistUpdate` with the real impl. Add 1 byte-pin test.

**Files:**
- `modules/world/bridges.go` (modify — replace one stub)
- `modules/world/friends_dispatcher_emit_test.go` (modify — add test)

**Test fixture.**

```go
// TestEmitFriendsDispatcher_OnIgnorelistUpdate_EmitsSnapshot pins that
// the dispatcher emits one UPDATE_IGNORELIST packet carrying the full
// snapshot to the viewer's wire.
func TestEmitFriendsDispatcher_OnIgnorelistUpdate_EmitsSnapshot(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	const viewer uint64 = 0x1111
	p.username37 = viewer
	s.playersMu.Lock()
	s.players = append(s.players, p)
	s.playersMu.Unlock()

	d := newEmitFriendsDispatcher(s, s.log)
	received := drainConn(t, cc)

	d.OnIgnorelistUpdate(viewer, []uint64{0x0102030405060708, 0xAABBCCDDEEFF0011})
	s.drainRelayActions()
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpUpdateIgnoreList.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x10,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}
```

**Implementation.** Replace the T3 stub body verbatim:

```go
func (d *emitFriendsDispatcher) OnIgnorelistUpdate(viewer uint64, ignored []uint64) {
	d.log.Debug("friends dispatch: ignorelist update",
		slog.Uint64("viewer", viewer),
		slog.Int("ignored", len(ignored)))
	d.s.enqueueRelayAction(func() {
		p := d.s.lookupPlayerByUsername37(viewer)
		if p == nil {
			return
		}
		sendUpdateIgnoreList(p, ignored)
	})
}
```

**Gates:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestEmitFriendsDispatcher_OnIgnorelistUpdate' -count=1 -timeout 60s
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

**Commit:** `feat(world): NAI-182-D5 T4 — emitFriendsDispatcher.OnIgnorelistUpdate`

Spec: `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md`.

## Task-5: emitFriendsDispatcher.OnPrivateMessage (RED → GREEN)

**Goal:** Replace the T3 stub for `OnPrivateMessage` with the real impl. Add 2 tests.

**Files:**
- `modules/world/bridges.go` (modify — replace one stub)
- `modules/world/friends_dispatcher_emit_test.go` (modify — add tests)

**Test fixtures.**

```go
// TestEmitFriendsDispatcher_OnPrivateMessage_EmitsPacket pins that the
// dispatcher emits one MESSAGE_PRIVATE packet to the recipient's wire
// matching the T2 encoder byte-pin.
func TestEmitFriendsDispatcher_OnPrivateMessage_EmitsPacket(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	const target uint64 = 0x4444
	p.username37 = target
	s.playersMu.Lock()
	s.players = append(s.players, p)
	s.playersMu.Unlock()

	// Compute the wordpacked bytes for "hi".
	wpBuf := packet.NewPacket(nil)
	wordpack.Pack(wpBuf, "hi")
	wpBytes := wpBuf.Bytes()

	d := newEmitFriendsDispatcher(s, s.log)
	received := drainConn(t, cc)

	d.OnPrivateMessage(target, 0x0102030405060708, 0, 0xDEADBEEF, "hi")
	s.drainRelayActions()
	p.client.flushWrite()

	got := <-received
	header := []byte{
		byte((int(gameserver.OpMessagePrivate.Opcode) + int(enc.GetNext())) & 0xff),
		byte(8 + 4 + 1 + len(wpBytes)),
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0xDE, 0xAD, 0xBE, 0xEF,
		0x00,
	}
	want := append(header, wpBytes...)
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestEmitFriendsDispatcher_OnPrivateMessage_MissingTargetNoEmit pins
// that the dispatcher silently drops PMs for a target not in s.players
// (e.g., player logged out between sender's send and recipient's tick).
func TestEmitFriendsDispatcher_OnPrivateMessage_MissingTargetNoEmit(t *testing.T) {
	s := newTestServer(t)
	d := newEmitFriendsDispatcher(s, s.log)

	d.OnPrivateMessage(0xDEAD, 0xBEEF, 0, 0, "hi")
	s.drainRelayActions()

	select {
	case <-s.relayActionQueue:
		t.Fatal("relayActionQueue should be drained")
	default:
	}
}
```

**Implementation.** Replace the T3 stub body verbatim:

```go
func (d *emitFriendsDispatcher) OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string) {
	d.log.Debug("friends dispatch: private message",
		slog.Uint64("target", target),
		slog.Uint64("from", from),
		slog.Uint64("pm_id", uint64(pmId)))
	d.s.enqueueRelayAction(func() {
		p := d.s.lookupPlayerByUsername37(target)
		if p == nil {
			return
		}
		sendMessagePrivate(p, from, pmId, staffLvl, chat)
	})
}
```

**Gates:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestEmitFriendsDispatcher_OnPrivateMessage' -count=1 -timeout 60s
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

**Commit:** `feat(world): NAI-182-D5 T5 — emitFriendsDispatcher.OnPrivateMessage`

Spec: `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md`.

## Task-6: Fresh-login ChatFilterSettings emit + existing test updates (RED → GREEN)

**Goal:** Insert `sendChatFilterSettings` BEFORE `sendUpdatePid` in `processLogins`. Delete the `DEVIATION-NAI-182-D5` line from the existing comment. Update the 3 existing fresh-login byte-pin tests in `modules/world/login_resync_test.go` to prepend the new opcode. Add 2 new tests.

**Files:**
- `modules/world/tick.go` (modify — `processLogins` body at lines 248-261)
- `modules/world/login_resync_test.go` (modify — update 3 tests + add 2 tests)

**Pre-flight enumerate (controller, before T6 dispatch).** Confirm the following 3 fresh-login byte-pin tests exist at HEAD and verify their structure has not drifted from spec §5-1 enumeration:

- `login_resync_test.go:76` `TestProcessLogins_FreshLogin_EmitsOpcodeOrder`
- `login_resync_test.go:123` `TestProcessLogins_FreshLogin_WithShutdownPending_EmitsRebootTimer`
- `login_resync_test.go:167` `TestProcessLogins_FreshLogin_NoShutdown_NoRebootTimer`

NO other fresh-login byte-pin tests touch `OpUpdatePid` opcode-key consumption. The `tick_test.go:14` `TestProcessLoginsAllocatesInputTracking` does NOT inspect bytes; safe. The `tick_logins_test.go:18, 48` tests do NOT inspect bytes; safe. The `server_test.go:456, 486` tests do NOT inspect bytes; safe.

**Implementation — tick.go edit.** Open `modules/world/tick.go`, locate lines 247-268 (the `else` branch after `if p.reconnecting`). Replace:

```go
// Fresh-login emit sequence per TS Player.onLogin
// (Player.ts:494-504). DEVIATION-NAI-182-D4 omits IF_CLOSE,
// DEVIATION-NAI-182-D5 omits ChatFilterSettings /
// UpdateIgnoreList (deferred social cluster).
sendUpdatePid(p, p.slot)
```

With:

```go
// Fresh-login emit sequence per TS Player.onLogin
// (Player.ts:486-504). DEVIATION-NAI-182-D4 omits IF_CLOSE.
// UpdateIgnoreList([]) defensive emit is permanently skipped
// (DEVIATION-NAI-182-D5-NO-DEFENSIVE-IGNORELIST-LOGIN-EMIT —
// goscape always runs with a friends server).
sendChatFilterSettings(p, p.publicChat, p.privateChat, p.tradeDuel)
sendUpdatePid(p, p.slot)
```

**Implementation — test updates.** For each of the 3 existing fresh-login byte-pin tests, prepend the new opcode key consumption and the 3 payload bytes to the expected sequence.

For `TestProcessLogins_FreshLogin_EmitsOpcodeOrder` (line 76): the `want` slice currently opens with `OpUpdatePid` opcode-key then `0x00, byte(p.slot)`. Prepend (in order):

```go
want := []byte{
    byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
    0x00, 0x00, 0x00, // p.publicChat, p.privateChat, p.tradeDuel all 0 at fresh login
    byte((int(gameserver.OpUpdatePid.Opcode) + int(enc.GetNext())) & 0xff),
}
// ... rest unchanged
```

For `TestProcessLogins_FreshLogin_WithShutdownPending_EmitsRebootTimer` (line 123): the `enc.GetNext()` consumption block consumes 3 opcode keys then computes UPDATE_REBOOT_TIMER as the 4th. After T6, it must consume 4 keys then compute UPDATE_REBOOT_TIMER as the 5th. And the byte offset increases by 4 (1 opcode + 3 payload). Replace:

```go
// Consume the first 3 packets: UPDATE_PID (3 bytes), RESET_CLIENT_VARCACHE
// (1 byte), RESET_ANIMS (1 byte) = 5 bytes total.
enc.GetNext() // UPDATE_PID opcode key
enc.GetNext() // RESET_CLIENT_VARCACHE opcode key
enc.GetNext() // RESET_ANIMS opcode key
offset := 3 + 1 + 1
```

With:

```go
// Consume the first 4 packets: CHAT_FILTER_SETTINGS (1+3 bytes),
// UPDATE_PID (1+2 bytes), RESET_CLIENT_VARCACHE (1 byte), RESET_ANIMS
// (1 byte) = 9 bytes total.
enc.GetNext() // CHAT_FILTER_SETTINGS opcode key
enc.GetNext() // UPDATE_PID opcode key
enc.GetNext() // RESET_CLIENT_VARCACHE opcode key
enc.GetNext() // RESET_ANIMS opcode key
offset := 4 + 3 + 1 + 1
```

For `TestProcessLogins_FreshLogin_NoShutdown_NoRebootTimer` (line 167): the test consumes 3 keys for the "what UPDATE_REBOOT_TIMER opcode-byte WOULD be" computation. After T6, it must consume 4 keys. Also the "wire should be exactly 5 bytes" assertion shifts to "wire should be exactly 9 bytes" (CFS=4 + PID=3 + RCV=1 + RA=1). Update the consumed-keys block and the length assertion at line 192 (`// The stream should be exactly 5 bytes`).

**New tests in `login_resync_test.go`.** Append:

```go
// TestProcessLogins_FreshLogin_EmitsChatFilterSettingsFirst pins that
// CHAT_FILTER_SETTINGS is the first packet on a fresh-login wire,
// carrying the player's publicChat/privateChat/tradeDuel triple.
// NAI-182-D5 T6.
func TestProcessLogins_FreshLogin_EmitsChatFilterSettingsFirst(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Direct field writes pre-processLogins are clobbered by fresh-init
	// for skills/invs/varps. publicChat/privateChat/tradeDuel are NOT
	// reset by initPlayerVarps; setting here is safe.
	p.publicChat = 1
	p.privateChat = 2
	p.tradeDuel = 0

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
		0x01, 0x02, 0x00,
	}
	if len(got) < len(want) {
		t.Fatalf("wire too short: got %d bytes, want at least %d", len(got), len(want))
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x", i, got[i], b)
		}
	}
}

// TestProcessLogins_FreshLogin_ChatFilterDefaults pins that
// publicChat/privateChat/tradeDuel default to 0 emit `00 00 00` on the wire.
// NAI-182-D5 T6.
func TestProcessLogins_FreshLogin_ChatFilterDefaults(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x00, 0x00,
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x", i, got[i], b)
		}
	}
}
```

**Gates:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestProcessLogins_FreshLogin' -count=1 -timeout 60s
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

The 3 modified tests + 2 new tests all pass. Full world suite green.

**Commit:** `feat(world): NAI-182-D5 T6 — fresh-login ChatFilterSettings emit`

Spec: `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md`.

## Task-7: Doc-comment retirements + slogFriendsDispatcher status

**Goal:** Update the `NAI-S4A` / `NAI-S4B` doc-comments on `FriendsDispatcher` and `slogFriendsDispatcher` to mark the tags retired (preserving the architectural-intent docs). Verify `slogFriendsDispatcher` still has no other callers; if it does, document them. No code-behavior change.

**Files:**
- `modules/world/bridges.go` (modify — edit 3 doc-comment blocks at lines 80-91, 100, 121-129)

**Implementation.** At `bridges.go:80-91` (`FriendsDispatcher` interface comment), replace:

```go
// FriendsDispatcher is the world-side sink for server -> world friends
// updates received over the SubscribeUpdates stream. Production impl
// (slogFriendsDispatcher, below) logs each event at Debug; the
// in-game ServerGameProt packet emit (UPDATE_FRIENDLIST /
// UPDATE_IGNORELIST / MESSAGE_PRIVATE writes to the player's client
// connection) is gated on NAI-182-D5 (the "social cluster"
// ServerGameProt deferral noted at tick.go:226).
//
// NAI-S4A-D-NO-INGAME-PACKET-EMIT — friendlist/ignorelist methods;
//   retires when NAI-182-D5 retires and OnFriendlistUpdate /
//   OnIgnorelistUpdate are wired through to player.write(...).
// NAI-S4B-D-NO-INGAME-PM-EMIT — OnPrivateMessage; retires when
//   NAI-182-D5 retires and the dispatcher is wired through to
//   player.write(MessagePrivate{...}) per TS World.ts:2000.
```

With:

```go
// FriendsDispatcher is the world-side sink for server -> world friends
// updates received over the SubscribeUpdates stream. Production impl
// is emitFriendsDispatcher (NAI-182-D5, 2026-05-19) which enqueues the
// real ServerGameProt packet emit on the tick-goroutine via
// s.relayActionQueue. slogFriendsDispatcher remains as a debug-only
// fallback for null-friends-server / test paths.
//
// Retired tags:
//   NAI-S4A-D-NO-INGAME-PACKET-EMIT — RETIRED 2026-05-19 (NAI-182-D5).
//     OnFriendlistUpdate / OnIgnorelistUpdate now emit UPDATE_FRIENDLIST /
//     UPDATE_IGNORELIST to the recipient's wire.
//   NAI-S4B-D-NO-INGAME-PM-EMIT — RETIRED 2026-05-19 (NAI-182-D5).
//     OnPrivateMessage now emits MESSAGE_PRIVATE to the recipient's wire.
```

At `bridges.go:98-100` (`slogFriendsDispatcher` struct comment), replace:

```go
// slogFriendsDispatcher is the default FriendsDispatcher. Logs each
// event at Debug; does NOT emit ServerGameProt packets to the player.
// See NAI-S4A-D-NO-INGAME-PACKET-EMIT above.
```

With:

```go
// slogFriendsDispatcher is the debug-only fallback FriendsDispatcher.
// Logs each event at Debug; does NOT emit ServerGameProt packets to
// the player. Production binds emitFriendsDispatcher instead — see
// FriendsDispatcher interface doc-comment above.
```

At `bridges.go:121-129` (`OnPrivateMessage` impl comment on slogFriendsDispatcher), replace:

```go
// OnPrivateMessage logs the inbound PM at Debug. The MESSAGE_PRIVATE
// ServerGameProt packet write to player.client (mirroring TS
// World.ts:2000 `player.write(new MessagePrivate(...))`) is gated on
// NAI-182-D5 (social-cluster ServerGameProt port) — see
// NAI-S4A-D-NO-INGAME-PACKET-EMIT on the interface for the parallel
// friendlist/ignorelist gating.
//
// NAI-S4B-D-NO-INGAME-PM-EMIT — retires when NAI-182-D5 retires and
// the dispatcher is wired through to player.write(MessagePrivate{...}).
```

With:

```go
// OnPrivateMessage logs the inbound PM at Debug. This is the fallback
// impl; production binds emitFriendsDispatcher (see FriendsDispatcher
// interface doc-comment) which writes the real MESSAGE_PRIVATE packet
// to the recipient via the tick-goroutine relayActionQueue.
```

**Gates:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

Build clean. Full world suite green.

**Commit:** `docs(world): NAI-182-D5 T7 — retire NAI-S4A/B doc-comments`

Spec: `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md`.

## Task-8: Whole-slice gate + memory close

**Goal:** Run the full `-race` suite + smoke-pack baseline. Author the memory close-memo + index entry. Decide whether to land the optional e2e extension (spec §4.5).

**Files:**
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (modify — prepend entry above the existing B3 close line)
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_182_d5_social_cluster_close.md` (new — full close memo)

**Gates:**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s
go run ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```

The race suite MUST show zero FAIL across all 56 testable packages. The smoke-pack baseline MUST hold at 12 OK / 0 ERR / 0 SKIP.

**Optional e2e extension (T8 judgment call).** The slice-5b precedent showed that a real-stream e2e catches dispatch-routing bugs that unit tests miss. Pattern: extend `TestFriendsClient_E2E_SubscribeUpdatesStream` (in `modules/world/friends_smoke_test.go` per slice-4a wiring) to push a `FriendsUpdate_PrivateMessage` through the real stream and assert that the recipient's wire receives the byte-pinned `OpMessagePrivate` packet. The cost is ~30 LOC + one `s.drainRelayActions()` call inserted into the test's polling loop. **Recommended yes** — closes the loop from stream Recv → dispatcher → enqueue → drain → writeOut. Implementer chooses to add or document why not.

**Close memo content (`nai_182_d5_social_cluster_close.md`).** Frontmatter + body. Required content:

- Date: 2026-05-19.
- Commits: `162fda5f..<final-HEAD>` (count commits inclusive of T1..T8).
- Files touched: `pkg/io/protocol/game/server/prot.go`, `modules/world/friends_emit.go` (new), `modules/world/friends_emit_test.go` (new), `modules/world/bridges.go`, `modules/world/server.go`, `modules/world/tick.go`, `modules/world/login_resync_test.go`, `modules/world/friends_dispatcher_emit_test.go` (new), optionally `modules/world/friends_smoke_test.go`.
- Retired tags: `NAI-S4A-D-NO-INGAME-PACKET-EMIT`, `NAI-S4B-D-NO-INGAME-PM-EMIT`, `DEVIATION-NAI-182-D5-SOCIAL-CLUSTER-PRE-PID-NOT-EMITTED`.
- Opened tags: `DEVIATION-NAI-182-D5-NO-WORDENC-FILTER`, `DEVIATION-NAI-182-D5-NO-DEFENSIVE-IGNORELIST-LOGIN-EMIT`, `DEVIATION-NAI-182-D5-CHAT-FILTER-NO-RESTORE`.
- Test counts: 7 new encoder byte-pins + 6 new dispatcher tests + 2 new fresh-login tests + 3 modified fresh-login tests + (optional) 1 e2e.
- Gate results: `-race` runtime in seconds, package count; smoke-pack runtime + OK/ERR counts.
- Surprises / deviations from plan: any in-execution adaptation, recorded per the slice-7 pattern.
- Next pivot: with the friends-server arc fully closed, the natural next is general world / runescript engine work (or NAI-183 if cluster-style work resurfaces).

**MEMORY.md index entry (prepend above the existing B3 close line; ~150 chars).** Example:

```
- [NAI-182-D5 social-cluster ServerGameProt close](nai_182_d5_social_cluster_close.md) — UPDATE_FRIENDLIST/IGNORELIST/MESSAGE_PRIVATE emit + CHAT_FILTER_SETTINGS login shipped 2026-05-19 across N commits ...
```

**Commit:** `chore: NAI-182-D5 close memo`

Spec: `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md`.

## Per-task review cadence (controller protocol)

Mirroring the B3 / slice-7 pattern. After each implementer subagent reports task complete, the controller dispatches:

1. **Spec-and-quality reviewer** (Opus) — receives the spec section + the commit diff + the gate output. Confidence-rated SHIP/HOLD verdict. Hold = controller patches in a follow-up commit before next task; ship = next task dispatches.
2. **(Whole-slice only, T8)** — one final review pass over the full delta `162fda5f..HEAD` with explicit "is this ready to merge to main and call the social-cluster ServerGameProt port complete?" framing.

## Risk register (controller carries through every dispatch)

Per spec §5 — re-confirm before each task dispatches:

1. T6 fresh-login byte-pin sites enumerated and verified at HEAD (spec §5-1). Re-grep before T6 dispatch.
2. `s.relayActionQueue` capacity (64) — drop-newest is documented. Burst loss acceptable for D5.
3. `lookupPlayerByUsername37` O(N) — accepted, document if smoke surfaces tick budget regression.
4. WordPack inside closure — confirmed safe (immutable string capture).
5. `FriendEntry.WorldId` > 255 truncation — document only if smoke fixture hits.
6. `s.players` tick-goroutine-only invariant — confirmed via slice-5b precedent.
7. `slogFriendsDispatcher` keep-or-delete — keep, doc-comment retired (T7).
8. `p.publicChat` zero-default at fresh login — confirmed acceptable.
9. `sendUpdateFriendList` one-per-entry emit cost — accepted.
10. `tick.go:250` comment edit shape — re-Read before T6 dispatch.
11. `friendsDispatcher` callers — confirmed only `friendsSubscriber.dispatch` calls it.

## File summary (controller's grep checklist)

```bash
# All write sites in the slice — confirm each is touched by exactly the tasks that should touch it.
grep -rn "OpUpdateFriendList\|OpUpdateIgnoreList\|OpChatFilterSettings\|OpMessagePrivate" pkg/io/protocol/game/server/prot.go
grep -rn "sendUpdateFriendList\|sendUpdateIgnoreList\|sendChatFilterSettings\|sendMessagePrivate" modules/world/
grep -rn "emitFriendsDispatcher\|newEmitFriendsDispatcher" modules/world/
grep -rn "NAI-S4A-D-NO-INGAME\|NAI-S4B-D-NO-INGAME\|DEVIATION-NAI-182-D5" modules/world/
```

Pre-dispatch (any task that modifies these files): re-read the relevant block immediately before issuing the edit (per `controller_preflight.md`).
