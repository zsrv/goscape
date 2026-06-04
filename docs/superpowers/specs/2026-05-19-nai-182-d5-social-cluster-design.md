# NAI-182-D5 — Social-cluster ServerGameProt port (UPDATE_FRIENDLIST / UPDATE_IGNORELIST / MESSAGE_PRIVATE / CHAT_FILTER_SETTINGS)

**Date:** 2026-05-19
**Status:** Design (standard cadence — brainstorm → spec → plan → subagent-driven TDD).
**Predecessor:** NAI-182 misc-serverprot cluster — spec `docs/superpowers/specs/2026-05-12-nai-182-misc-serverprot-cluster-design.md`, plan `docs/superpowers/plans/2026-05-12-nai-182-misc-serverprot-cluster.md`. The misc cluster (`UPDATE_PID` / `RESET_ANIMS` / `RESET_CLIENT_VARCACHE` / `UPDATE_REBOOT_TIMER`) shipped earlier; D5 finishes the carved-off social opcodes that the predecessor explicitly deferred at §6 deviations (`DEVIATION-NAI-182-D5-SOCIAL-CLUSTER-PRE-PID-NOT-EMITTED`).
**HEAD at design:** `66a60a8f` (post-B3 close).
**Tags retired (when shipped):** `NAI-S4A-D-NO-INGAME-PACKET-EMIT`, `NAI-S4B-D-NO-INGAME-PM-EMIT`, `DEVIATION-NAI-182-D5-SOCIAL-CLUSTER-PRE-PID-NOT-EMITTED`.

## 1. Problem

The friends-server bridge arc (slices 1–7) writes friendlist/ignorelist mutations and private-chat messages to SQLite and re-broadcasts them via the gRPC `SubscribeUpdates` stream. World-side, the `friendsSubscriber` consumes that stream and routes each `FriendsUpdate` variant to a `FriendsDispatcher`. The production dispatcher today (`slogFriendsDispatcher` at `modules/world/bridges.go:101-135`) emits a `slog.Debug` line for each variant — the in-game ServerGameProt packet that the Java client decodes (`UPDATE_FRIENDLIST` / `UPDATE_IGNORELIST` / `MESSAGE_PRIVATE`) is NOT written to `p.client.bufw`. From the player's perspective, friendlist updates and inbound private chat are invisible.

Separately, fresh-login `processLogins` at `modules/world/tick.go:248-261` carries a `DEVIATION-NAI-182-D5` comment marking the omitted `ChatFilterSettings` emit — TS `Player.onLogin` (Player.ts:487) writes one `ChatFilterSettings(publicChat, privateChat, tradeDuel)` packet before `UpdatePid`, so the client knows the player's chat-filter mode from tick zero. goscape's fresh-login emit skips this packet.

NAI-182-D5 lands all four social ServerGameProt opcodes (3 dispatcher hops + 1 fresh-login emit), retires the two `NAI-S4A/B` runtime tags plus the `DEVIATION-NAI-182-D5` comment, and closes the social-cluster ServerGameProt port. The TS `UpdateIgnoreList([])` defensive emit at `Player.ts:489-492` is gated on TS `!Environment.NODE_HAS_FRIEND_SERVER` — goscape always runs with a friends server (the bridge arc is the only persistence path), so the no-friend-server branch is permanently inapplicable (deviation tag below).

## 2. Affected sites (pre-flight verified at HEAD `66a60a8f`)

| Path | State at HEAD |
| --- | --- |
| `pkg/io/protocol/game/server/prot.go` | No `OpUpdateFriendList` / `OpUpdateIgnoreList` / `OpMessagePrivate` / `OpChatFilterSettings`. Opcodes 152, 21, 41, 32 unused. NAI-182 misc opcodes (`OpUpdatePid` 139, `OpResetAnims` 136, `OpResetClientVarCache` 193, `OpUpdateRebootTimer` 43) already present at lines 147-174 — adjacent group placement. |
| `modules/world/bridges.go:80-135` | `FriendsDispatcher` interface + `slogFriendsDispatcher` default impl. 3 methods log Debug. Two `NAI-S4A-D-NO-INGAME-PACKET-EMIT` / `NAI-S4B-D-NO-INGAME-PM-EMIT` doc-comments at interface and impl. |
| `modules/world/bridges.go:285-303` | `noopBridges` already implements `OnFriendlistUpdate(uint64, []*friendspb.FriendEntry)`, `OnIgnorelistUpdate(uint64, []uint64)`, `OnPrivateMessage(uint64, uint64, int32, uint32, string)` as no-ops. Signatures must remain stable. |
| `modules/world/server.go:165` | `friendsDispatcher FriendsDispatcher` field on `*Server`. |
| `modules/world/server.go:292` | `s.friendsDispatcher = newSlogFriendsDispatcher(s.log)` — single wire-up site. Replace with emitter constructor that closes over `*Server`. |
| `modules/world/server.go:236-243, 286` | `relayActionQueue chan func()` (buffer 64) + `make(chan func(), 64)` constructor wiring. `drainRelayActions()` runs at top of every tick (`tick.go:54`). Single-writer-on-tick-goroutine invariant for player state. |
| `modules/world/world_state_ops.go:13-42` | `(*Server).enqueueRelayAction(action func())` + `drainRelayActions()` precedent. Drop-newest on full buffer. |
| `modules/world/world_state_ops.go:56-70` | `(*Server).lookupPlayerByUsername37(u37 uint64) *Player` — iterates `s.players` (tick-goroutine-only read; safe inside the enqueued closure). Returns nil if absent. |
| `modules/world/friends_subscriber.go:117-132` | `dispatch(u *friendspb.FriendsUpdate)` routes each variant to `s.dispatcher.On*`. The subscriber's `s.username37` is the recipient/viewer for all three variants. No changes needed in subscriber. |
| `modules/world/tick.go:248-261` | Fresh-login emit sequence inside `processLogins`. Contains the `DEVIATION-NAI-182-D5` comment marking omitted `ChatFilterSettings` + `UpdateIgnoreList`. Order today: `sendUpdatePid` → `sendResetClientVarCache` → varp transmit-loop → `sendResetAnims` → optional `sendUpdateRebootTimer`. TS order at `Player.ts:486-504`: `ChatFilterSettings` → (no-friend-server branch only) `UpdateIgnoreList([])` → `IfClose` (D4) → `UpdatePid` → `ResetClientVarCache` → varp loop → `ResetAnims`. goscape inserts `sendChatFilterSettings` BEFORE `sendUpdatePid`. |
| `modules/world/player.go:238` | `publicChat, privateChat, tradeDuel int` fields exist on `Player`. Defaults are zero (matching TS `Player.publicChat = 0` etc.). |
| `modules/world/player.go:91, 514` | `staffModLevel int32` on `Player`. Copied from `client.staffModLevel` in `newPlayer`. |
| `modules/world/player.go:475-490` | `(*Player).writeOut(op gameserver.Op, payload []byte)` — writes ISAAC-encrypted opcode + optional length prefix + payload to `p.client.bufw`. NOT safe off-tick: touches per-Player ISAAC stream + buffered writer. Production callers all run on the tick goroutine. |
| `pkg/wordenc/wordpack/wordpack.go:68` | `wordpack.Pack(buf *packet.Packet, s string)` — TS `WordPack.pack` port. Truncates to 80 chars. |
| `pkg/wordenc` | No `WordEnc.filter` port exists (TS profanity filter applied at PM-send site). Deviation: goscape passes `chat` through `wordpack.Pack` without TS-equivalent WordEnc filtering. |
| `pkg/io/packet/packet.go` (P-methods) | `P1(uint8)`, `P2(uint16)`, `P4(uint32)`, `P8(uint64)` available. |
| `pkg/friendspb/friends.pb.go` | `FriendEntry.WorldId int32`, `FriendEntry.Username37 uint64` — fields needed for UPDATE_FRIENDLIST encode. `WorldId=0` ⇒ "offline" per slice-3 close note. |

TS source verified at `/home/owner/Code/github.com/LostCityRS/Engine-TS`:

- `src/network/game/server/ServerGameProt.ts:48-51` — opcodes `UPDATE_IGNORELIST` (21, -2), `CHAT_FILTER_SETTINGS` (32, 3), `MESSAGE_PRIVATE` (41, -1), `UPDATE_FRIENDLIST` (152, 9).
- `src/network/game/server/codec/UpdateFriendListEncoder.ts:9-12` — `buf.p8(message.name); buf.p1(message.nodeId);` (one packet per friend entry; 9 fixed bytes).
- `src/network/game/server/codec/UpdateIgnoreListEncoder.ts:9-17` — for-each name: `buf.p8(name);`. `test()` = `8 * names.length`. Variable size with 2-byte length prefix.
- `src/network/game/server/codec/MessagePrivateEncoder.ts:11-25` — `p8(from)` + `p4(messageId)` + `p1(staffLvlAdjusted)` + `WordPack.pack(buf, WordEnc.filter(message.msg))`. Staff-lvl adjustment: `if staffLvl > 0 then staffLvl += 1`. Variable size with 1-byte length prefix.
- `src/network/game/server/codec/ChatFilterSettingsEncoder.ts:9-13` — `p1(publicChat)` + `p1(privateChat)` + `p1(tradeDuel)`. Fixed 3 bytes.
- `src/engine/entity/Player.ts:486-491` — `onLogin` writes `ChatFilterSettings(publicChat, privateChat, tradeDuel)` then (no-friend-server branch only) `UpdateIgnoreList([])`.
- `src/engine/World.ts:1951-2000` — `onFriendMessage` dispatch: per-entry `player.write(new UpdateFriendList(BigInt(friendUsername37), world))` loop for `UPDATE_FRIENDLIST`; single `player.write(new UpdateIgnoreList(ignored))` for `UPDATE_IGNORELIST`; single `player.write(new MessagePrivate(fromPlayer, pmId, fromPlayerStaffLvl, chat))` for `PRIVATE_MESSAGE`.

## 3. Architecture

### 3.1 Opcode declarations

Append to `pkg/io/protocol/game/server/prot.go` in a new "social cluster" group adjacent to the existing NAI-182 misc group:

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
// staffLvl. Emitted by the friends-server dispatcher on PrivateMessageDelivery.
// Mirrors TS ServerGameProt.MESSAGE_PRIVATE (41, -1) and
// MessagePrivateEncoder.ts.
OpMessagePrivate = Op{Opcode: 41, PayloadSize: -1}
```

### 3.2 Encoder send-functions

New file `modules/world/friends_emit.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// sendUpdateFriendList writes one UPDATE_FRIENDLIST packet for a single
// friend-entry update. Callers loop over entries; one packet per entry.
// worldId == 0 conveys offline/hidden per slice-3 friends-server contract.
// Mirrors TS UpdateFriendListEncoder (`p8(name); p1(nodeId)`).
func sendUpdateFriendList(p *Player, username37 uint64, worldId int) {
	buf := packet.NewPacket(nil)
	buf.P8(username37)
	buf.P1(uint8(worldId))
	p.writeOut(gameserver.OpUpdateFriendList, buf.Bytes())
}

// sendUpdateIgnoreList writes one UPDATE_IGNORELIST packet carrying the
// complete ignorelist snapshot. Mirrors TS UpdateIgnoreListEncoder
// (`for name in names: p8(name)`). Empty slice produces a zero-length
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
// (`p1(publicChat); p1(privateChat); p1(tradeDuel)`). All three fields
// are signed in TS but the wire is a single byte each; goscape stores
// them as int and casts to uint8 here.
func sendChatFilterSettings(p *Player, publicChat, privateChat, tradeDuel int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(publicChat))
	buf.P1(uint8(privateChat))
	buf.P1(uint8(tradeDuel))
	p.writeOut(gameserver.OpChatFilterSettings, buf.Bytes())
}

// sendMessagePrivate writes one MESSAGE_PRIVATE packet to the recipient.
// from is the sender's username37. pmId is the friends-server-assigned
// PM correlation id. staffLvl is the SENDER's staff level; the wire
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

### 3.3 Tick-goroutine seam: emitFriendsDispatcher

`FriendsDispatcher` methods are called from the `friendsSubscriber` Recv goroutine — `s.dispatch(u)` runs on the gRPC stream goroutine, NOT the tick goroutine. `writeOut` is unsafe off-tick (mutates per-Player ISAAC stream + bufw). The production impl must marshal the call back to the tick goroutine via the existing `relayActionQueue` (the slice-5b precedent).

Replace `slogFriendsDispatcher` as the production wire-up. New impl in `modules/world/bridges.go` (replacing the slog impl; slog impl stays callable for null-server / test paths):

```go
// emitFriendsDispatcher is the production FriendsDispatcher. Each
// method enqueues a closure on s.relayActionQueue so the writeOut on
// the resolved Player runs on the tick goroutine (the only goroutine
// allowed to touch Player.client.bufw + ISAAC stream). The recipient
// is resolved inside the closure (not at enqueue time) so a player who
// logs out between event arrival and tick-drain is correctly skipped.
//
// slogFriendsDispatcher remains the default fallback when s == nil
// (testing seam) or when friends-server is disabled — that path never
// reaches a real Player.
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

var _ FriendsDispatcher = (*emitFriendsDispatcher)(nil)
```

Wire-up change at `modules/world/server.go:292`:

```go
// Before
s.friendsDispatcher = newSlogFriendsDispatcher(s.log)

// After
s.friendsDispatcher = newEmitFriendsDispatcher(s, s.log)
```

`slogFriendsDispatcher` stays in the file as a testing/null fallback (no removal — referenced by existing tests and serves as the doc-comment carrier for the architectural intent).

### 3.4 Fresh-login ChatFilterSettings emit

Modify `modules/world/tick.go:248-261`. Insert `sendChatFilterSettings` BEFORE `sendUpdatePid`. Remove the `DEVIATION-NAI-182-D5` line of the existing comment (the deviation is now closed). Final block:

```go
} else {
	// Fresh-login emit sequence per TS Player.onLogin
	// (Player.ts:486-504). DEVIATION-NAI-182-D4 omits IF_CLOSE.
	sendChatFilterSettings(p, p.publicChat, p.privateChat, p.tradeDuel)
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

**No `UpdateIgnoreList([])` defensive emit** — TS gates this on `!Environment.NODE_HAS_FRIEND_SERVER` (`Player.ts:489-492`), and goscape permanently runs WITH a friends server. Captured as a permanent deviation tag in §6.

### 3.5 Doc-comment retirements

`modules/world/bridges.go:80-91, 100, 121-129` carry the two `NAI-S4A-D-NO-INGAME-PACKET-EMIT` / `NAI-S4B-D-NO-INGAME-PM-EMIT` blocks. After D5 ships, the comments still describe the slog dispatcher (which now coexists with the emit dispatcher as a null-fallback), so the runtime tag is retired but the architectural note stays — re-word the doc-comment to "retired by NAI-182-D5" rather than deleting.

The `DEVIATION-NAI-182-D5` line in the `tick.go` comment is deleted outright (the deviation is closed, no doc residue needed).

### 3.6 noopBridges signatures

Confirmed at `bridges.go:301-303` — signatures already match. No change. (If the dispatcher interface had needed an extra method or arg, all noopBridges/recordingBridges sites would need updating. This slice ADDS no new dispatcher methods — it adds NEW non-dispatcher send-functions and wires them through the existing 3 dispatcher methods.)

## 4. Test plan

Pattern reference: existing byte-pin tests in `modules/world/login_resync_test.go` (NAI-182 misc) and `modules/world/reboot_test.go`. They use the same `drainConn` channel + sibling ISAAC stream pattern as `player_script_test.go:TestHintNpcPayloadBytes`.

### 4.1 Encoder byte-pins (T2)

`modules/world/friends_emit_test.go`:

- `TestSendUpdateFriendList_EmitsExactByteSequence` — `sendUpdateFriendList(p, 0x0102030405060708, 7)` expects encrypted-opcode byte + `01 02 03 04 05 06 07 08 07` (8-byte username37 + 1-byte worldId = 9 fixed bytes).
- `TestSendUpdateIgnoreList_EmitsExactByteSequence` — `sendUpdateIgnoreList(p, []uint64{0x0102030405060708, 0xAABBCCDDEEFF0011})` expects encrypted-opcode + 2-byte BE length prefix `00 10` + 16 payload bytes.
- `TestSendUpdateIgnoreList_EmptyListEmitsZeroLengthPayload` — `sendUpdateIgnoreList(p, nil)` expects encrypted-opcode + `00 00` length prefix + zero payload bytes. Mirrors TS `player.write(new UpdateIgnoreList([]))`.
- `TestSendChatFilterSettings_EmitsExactByteSequence` — `sendChatFilterSettings(p, 2, 1, 3)` expects encrypted-opcode + `02 01 03`.
- `TestSendMessagePrivate_EmitsExactByteSequence` — `sendMessagePrivate(p, 0x0102030405060708, 0xDEADBEEF, 0, "hi")` expects: encrypted-opcode + 1-byte length prefix (computed from buf.Len after wordpack) + payload `01..08 DE AD BE EF 00 <wordpacked-bytes-for-"hi">`. Compute the wordpack output via `wordpack.Pack(testBuf, "hi")` in the test to derive the expected length and tail bytes.
- `TestSendMessagePrivate_StaffLvlAdjustmentZero` — `staffLvl=0` ⇒ wire byte `00`.
- `TestSendMessagePrivate_StaffLvlAdjustmentPositive` — `staffLvl=2` ⇒ wire byte `03` (TS `staffLvl + 1`).
- `TestSendMessagePrivate_StaffLvlAdjustmentNegative` — `staffLvl=-1` ⇒ wire byte `0xFF` (no adjustment when not > 0).

### 4.2 emitFriendsDispatcher (T3-T5)

`modules/world/friends_dispatcher_emit_test.go`:

- `TestEmitFriendsDispatcher_OnFriendlistUpdate_EnqueuesOnePacketPerEntry` — seed `s` with one Player whose username37 matches `viewer`; call `d.OnFriendlistUpdate(viewer, []*friendspb.FriendEntry{{Username37: 1, WorldId: 1}, {Username37: 2, WorldId: 0}})`; assert `s.relayActionQueue` length 1 (single closure); drain via `s.drainRelayActions()`; assert player conn received TWO `OpUpdateFriendList` packets in order (`01..08`, then `02..08` with worldId byte). Also asserts the slog Debug line fires (capture via sync buffer logger).
- `TestEmitFriendsDispatcher_OnFriendlistUpdate_MissingPlayerNoEmit` — `viewer` does NOT match any player; assert closure runs, no packet written, no error.
- `TestEmitFriendsDispatcher_OnIgnorelistUpdate_EmitsSnapshot` — seed one player; `d.OnIgnorelistUpdate(viewer, []uint64{1, 2, 3})`; drain; assert one `OpUpdateIgnoreList` packet with 3 × 8-byte payload.
- `TestEmitFriendsDispatcher_OnPrivateMessage_EmitsPacket` — seed one player; `d.OnPrivateMessage(target, 0x0102030405060708, 0, 0xDEADBEEF, "hi")`; drain; assert one `OpMessagePrivate` packet matching the byte-pin from T2 (compose expected bytes via direct encoder call).
- `TestEmitFriendsDispatcher_OnPrivateMessage_MissingTargetNoEmit` — target absent; assert closure no-ops cleanly.
- `TestEmitFriendsDispatcher_OffTickEnqueue_DropsOnFullQueue` — fill `s.relayActionQueue` to capacity (64); call `d.OnFriendlistUpdate(...)`; assert no panic, no block, dropped silently (matches slice-5b drop-newest semantics — verify by counting drained closures after).
- `TestEmitFriendsDispatcher_LogoutBetweenEnqueueAndDrain` — seed player; enqueue an emit; remove player from `s.players`; drain; assert no packet written, no panic.

### 4.3 Fresh-login ChatFilterSettings emit (T6)

`modules/world/login_resync_test.go` — extend existing fresh-login byte-pin tests. The new packet emits BEFORE `OpUpdatePid`, so every fresh-login byte-pin test must prepend the `OpChatFilterSettings` 3-byte payload.

- `TestProcessLogins_FreshLogin_EmitsChatFilterSettingsFirst` — new test. Seed `p.publicChat = 1, p.privateChat = 2, p.tradeDuel = 0`; run `s.processLogins()`; assert opcode order opens with `OpChatFilterSettings(1, 2, 0)` THEN `OpUpdatePid` etc.
- `TestProcessLogins_FreshLogin_ChatFilterDefaults` — defaults all 0; emit byte `00 00 00`.
- Extend `TestProcessLogins_FreshLogin_EmitsOpcodeOrder` (or whatever the existing NAI-182-misc test is named — see Risk §5-1) to prepend `OpChatFilterSettings` to the asserted opcode sequence.

### 4.4 noopBridges + interface compile-checks (T7)

No new test code (signatures unchanged). T7 task verifies `go build ./...` clean across the world module after T2-T6 land.

### 4.5 Whole-slice gate (T8)

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s` — zero FAIL across all packages.
- `cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content` — 12 OK / 0 ERR (baseline holds).
- Optional e2e: extend `TestFriendsClient_E2E_SubscribeUpdatesStream` (or a sibling) to assert that an inbound `FriendsUpdate_PrivateMessage` results in a byte-pinned `OpMessagePrivate` write to the recipient's buffered conn. Whether to add this is left to T8 judgment per the slice-5b precedent (it caught real bugs there; the cost is one ~30-LOC test).

## 5. Risks & open premises (controller pre-flight)

Per memory `controller_preflight.md`, the controller does a 30-second grep+Read pass against HEAD before dispatching each task. The premises below are flagged for verification:

1. **Existing fresh-login byte-pin tests enumerated.** The NAI-182 misc cluster shipped fresh-login byte-pin tests at `modules/world/login_resync_test.go`. Inserting `sendChatFilterSettings` before `sendUpdatePid` shifts every byte-pin assertion that pins the opening of the fresh-login sequence. Pre-flight greps `processLogins\|sendUpdatePid\|sendResetClientVarCache` under `modules/world/*_test.go` and enumerates EVERY test that asserts opening-byte sequence. Per memory `enumerate_all_sites.md`, the plan lists each callsite explicitly. **Plan-author task:** complete the enumeration before T6 dispatch.

2. **`s.relayActionQueue` capacity vs. burst friends-server events.** Slice-5b sized it at 64 for RELAY_* admin events (rare). Friends-server PRIVATE_MESSAGE deliveries can burst (e.g., spam target — though TS ratelimits at sender side). Drop-newest may silently swallow a few PMs under burst; matches slice-4a's `subscriberBufferSize = 64` drop-newest contract. Document, don't change. **If burst loss is a concern**, raise the buffer in a follow-up — not in scope for D5.

3. **`lookupPlayerByUsername37` complexity.** Linear scan of `s.players` (slice-5b T3 close note). For ~2000 players × per-PM lookup, this is O(N) per event. Slice-5b accepted this; D5 inherits. Document as a "future optimization" in close memo if it surfaces in smoke.

4. **WordPack semantics inside the closure.** `wordpack.Pack` allocates a fresh buffer; runs entirely from the captured `chat` string; no shared state. Safe inside the enqueued closure (tick-goroutine-side after drain). The closure captures `chat string` by value (immutable in Go) — no race.

5. **`FriendEntry.WorldId` range.** Wire is `p1(nodeId)` — single byte, 0-255. goscape's `WorldId int32` could legally exceed 255; production deploys are 1-65 historically. Spec uses `uint8(worldId)` which truncates silently. **Pre-flight:** if goscape's worldid range ever exceeds 255, this truncates without diagnostic. Document as a deviation if needed (out of scope today; track if smoke reveals a fixture with WorldId > 255).

6. **`s.players` field access from inside the closure.** Verified at HEAD: `lookupPlayerByUsername37` reads `s.players` directly. The closure runs on the tick goroutine via `drainRelayActions` (called at top of tick — `tick.go:54`). `s.players` is a tick-goroutine-only structure (no external mutation between drains). Safe. ✅ Verified.

7. **The `slogFriendsDispatcher` post-D5 role.** After D5 swap, `slogFriendsDispatcher` is unreferenced by `server.go:292`. Tests + the `newSlogFriendsDispatcher` constructor may have stale references. Pre-flight: `grep -rn "newSlogFriendsDispatcher\|slogFriendsDispatcher" modules/world` and either (a) keep both impls with a clear "default = emit; fallback for test = slog" comment, or (b) delete the slog impl and replace any test reference with `noopBridges{}`. Plan picks (a) — minimal-blast-radius approach matching slice-5b's pattern of keeping `slogWorldEventsDispatcher` alongside `actionWorldEventsDispatcher`.

8. **`p.publicChat` etc. zero-default at fresh login.** `Player.publicChat` defaults to 0 in goscape (verified at `player.go:238` — implicit zero-value). TS `Player.publicChat` defaults to 0 (`Player.ts:185` or similar). Match. No save/restore for these fields today (no PlayerLoading restore subsystem populates them). Fresh-login emit will always send `0 0 0` until restore lands. Acceptable — TS-faithful for the no-save case.

9. **`sendUpdateFriendList` one-per-entry emit cost.** A friendlist of 200 entries triggers 200 separate `writeOut` calls inside ONE enqueued closure. ISAAC stream advances 200 opcodes; bufw absorbs ~1.8KB. Single tick. Should not jitter the tick budget; pre-flight check confirms `writeOut` itself is O(1). Document only if smoke surfaces a tick-budget regression.

10. **`tick.go:250` comment edit.** The comment carries both `DEVIATION-NAI-182-D4` (IfClose, NOT closed by D5) and `DEVIATION-NAI-182-D5` (this slice). T6 surgically removes ONLY the `NAI-182-D5` line, keeping the D4 line. Pre-flight: `Read tick.go:248-261` immediately before T6 to confirm comment shape hasn't drifted; the edit is text-precise.

11. **`emitFriendsDispatcher` not used when friendsClient is nil.** `server.go:292` runs unconditionally today (`newSlogFriendsDispatcher`). When `s.friendsClient == nil`, no subscriber ever spins up, so the dispatcher is never called — but `s.friendsDispatcher` still needs to be non-nil (used by helper paths if any). Spec wires `newEmitFriendsDispatcher(s, s.log)` unconditionally. If `s.friendsClient == nil`, the dispatcher will simply never see a call. Safe. Verify no other site in the world module calls `friendsDispatcher` directly (subscriber is the only caller). Pre-flight: `grep -rn "friendsDispatcher" modules/world`.

## 6. Deviations

- **DEVIATION-NAI-182-D5-NO-WORDENC-FILTER** — RETIRED 2026-05-20 (NAI-WORDENC-FILTER slice). `pkg/wordenc/encfilter` ported in 11 commits; `sendMessagePrivate` and `handleMessagePublic` now apply `s.wordenc.Filter(...)`. Pinned by `TestSendMessagePrivate_AppliesWordEncFilter` and `TestHandleMessagePublic_AppliesWordEncFilterToChatBytes`.
- **DEVIATION-NAI-182-D5-NO-DEFENSIVE-IGNORELIST-LOGIN-EMIT** — TS `Player.onLogin` emits `UpdateIgnoreList([])` at `Player.ts:489-492` ONLY when `!Environment.NODE_HAS_FRIEND_SERVER`. goscape permanently runs WITH a friends server (the bridge arc is the only persistence path), so this branch is never taken. Permanent.
- **DEVIATION-NAI-182-D5-CHAT-FILTER-NO-RESTORE** — RETIRED 2026-05-19 (post-D5 cleanup). The marker was authored assuming PlayerLoading did not yet restore these fields, but NAI-PLAYERLOADING (closed at 09e467b5, predates D5) already wired SAV v4+ round-trip for publicChat/privateChat/tradeDuel — see `player_save.go:104` (pack) and `player_load.go:225-229` (unpack). `processLogins` invokes `LoadSave` at `tick.go:222` before `sendChatFilterSettings` at line 253, so the fresh-login emit already reflects saved state for any returning player. Pinned by `TestProcessLogins_FreshLogin_ChatFilterEmitReflectsSAV` in `login_resync_test.go`.

## 7. Tags retired by this slice

- **`NAI-S4A-D-NO-INGAME-PACKET-EMIT`** — `bridges.go:80-91, 100` (`FriendsDispatcher` interface + `slogFriendsDispatcher`). Replaced by `emitFriendsDispatcher` which writes the actual packets. Doc-comment edited to "retired by NAI-182-D5 (2026-05-19)".
- **`NAI-S4B-D-NO-INGAME-PM-EMIT`** — `bridges.go:89-90, 121-129` (`OnPrivateMessage` slog impl). Same edit.
- **`DEVIATION-NAI-182-D5-SOCIAL-CLUSTER-PRE-PID-NOT-EMITTED`** — declared in the NAI-182 misc spec §6 deviations and in `tick.go:250`. ChatFilterSettings is now emitted at the spec-faithful slot (before UpdatePid). The `tick.go:250` deviation line is deleted; the misc spec doc-deviation stays in the doc as historical record but its tracker text adds "Retired by NAI-182-D5".

## 8. Task ordering (for plan-author)

Plan-author drafts a per-task plan doc following the memory pattern at `plan_runnable_test_fixtures.md` (every code-block runnable as-is). Order:

- **T1 — Opcode declarations** (§3.1). 4 `Op` entries. No callers; compiles clean. No test (data-only).
- **T2 — Encoder send-functions + byte-pins** (§3.2, §4.1). 4 send-functions in new file `modules/world/friends_emit.go`. 8 byte-pin tests in `modules/world/friends_emit_test.go`. TDD: each red-test → green per encoder, or bundled red→green for all 4 if straightforward.
- **T3 — emitFriendsDispatcher.OnFriendlistUpdate** (§3.3, §4.2). Add `emitFriendsDispatcher` struct + constructor + first method. Wire-up at `server.go:292`. Three new tests (per §4.2).
- **T4 — emitFriendsDispatcher.OnIgnorelistUpdate** (§3.3, §4.2). Second method. One test (snapshot emit).
- **T5 — emitFriendsDispatcher.OnPrivateMessage** (§3.3, §4.2). Third method. Two tests (emit + missing-target).
- **T6 — Fresh-login ChatFilterSettings emit + comment retirement** (§3.4, §4.3). Surgical edit to `tick.go:248-261`. Update existing fresh-login byte-pin tests to prepend `OpChatFilterSettings`. Add 2 new tests for the new emit.
- **T7 — Doc-comment retirements + interface compile check** (§3.5). Edit the two `NAI-S4A/B` doc-comments on `FriendsDispatcher` and `slogFriendsDispatcher`. `go build ./...`. No new test.
- **T8 — Whole-slice gate + close memo** (§4.5). Full `-race` suite. Smoke-pack 12 OK. Optional e2e extension. Memory close per `nai_followups.md` + `post_task_handoff.md` patterns.

Each task should commit independently with `feat(world): NAI-182-D5 T<N> — <title>` per recent NAI-182 commit convention.

## 9. Close-time memory entries

To save at NAI-182-D5 close (per memory `nai_followups.md`, `post_task_handoff.md`):

- **D5 close memo** — full slice summary with retired tags, opened deviation tags, file/line counts, gate results. Index entry above the existing B3 line in `MEMORY.md`.
- **emitFriendsDispatcher pattern** — first instance of a non-slog dispatcher that enqueues on `relayActionQueue` for tick-goroutine writeOut. Pattern reusable for future server→client emit hops. Worth a dedicated entry if the pattern is non-obvious to a future reader.
- **`sendMessagePrivate` staff-lvl adjustment** — `if staffLvl > 0 then +1` is TS-faithful but non-obvious (staff-icon mapping). Worth a 1-line note in the close memo for future readers of the encoder.

If smoke or implementer steps surface adjacent TS-fidelity divergences, route in-scope-stretch (≤30 LOC) per memory `smoke_surfaces_adjacent_divergences.md`, else open NAI-183.
