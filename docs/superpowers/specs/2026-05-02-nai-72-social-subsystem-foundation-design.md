# NAI-72 — Social subsystem foundation (ChatSetMode + Friend/Ignore family + ReportAbuse)

**Status:** Spec written 2026-05-02.
**Predecessor:** NAI-71 (HEAD `5298d6a`). Net deviation tally entering: 14.
**Opens:** 5 new deviations:
- `NAI-72-D-FRIENDS-SERVER-BRIDGE`
- `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD`
- `NAI-72-D-LOGGER-BRIDGE`
- `NAI-72-D-STAFFMODLEVEL-DEAD-WRITER`
- `NAI-72-D-INPUT-RECORDING-NOT-PORTED`

Plus one tracker note (not a deviation): `NAI-72-N-RESETENTITY-PARTIAL` — partial port of `Player.resetEntity(false)`; rest of the body belongs to chat-mask / script-protect sub-specs and is already split across other tracker entries. Tracked as a doc-comment audit note, not a numbered deviation.

**Closes:** none.
**Bug-fix surfaced (not a deviation):** `pkg/util/jstring.FromBase37` is missing TS's `value % 37 === 0` invalid-name check (JString.ts:42-44). Folded into T1.
**Tech stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`. Rust source N/A.

## 1. Background

Six game opcodes in `pkg/io/protocol/game/client/prot.go` are accepted at the wire level but have no entry in `gameHandlers[]`, so `(*Player).readPacket` silently discards them at `modules/world/player.go:771-775`:

| Opcode | Name | Payload | TS handler | TS LOC |
|---|---|---|---|---|
| 244 | CHAT_SETMODE | 3 | `ChatSetModeHandler.ts` | 15 |
| 118 | FRIENDLIST_ADD | 8 | `FriendListAddHandler.ts` | 17 |
| 11 | FRIENDLIST_DEL | 8 | `FriendListDelHandler.ts` | 17 |
| 79 | IGNORELIST_ADD | 8 | `IgnoreListAddHandler.ts` | 17 |
| 171 | IGNORELIST_DEL | 8 | `IgnoreListDelHandler.ts` | 17 |
| 190 | REPORT_ABUSE | 10 | `ReportAbuseHandler.ts` | 29 |

Together these form the engine's "social" cluster — they share three structural traits absent from the OPHELD/OPLOC/OPNPC handler families:

1. **Per-tick spam-protect flags.** Five of the six handlers (all except CHAT_SETMODE) early-return when a per-player flag is true and set it true on success, so the client can issue at most one social packet per type per tick. TS resets the flags in `Player.resetEntity(false)` at Player.ts:466-467, called from the World tick path at World.ts:1138.
2. **Out-of-process bridge consumers.** TS routes the actual work to three Worker threads (`friendThread`, `loginThread`, `loggerThread`) via `postMessage(...)` — the engine does not keep friend/ignore lists, ban records, or abuse reports in-engine. Goscape has no analog process model yet, so this sub-spec defines a no-op bridge interface trio (matching the precedent from NAI-71's `addSessionLog` deferral) and tracks each closure path as a separate deviation.
3. **base37-encoded usernames.** All five gated handlers receive a `uint64` username on the wire and decode via `FromBase37`, gating on the `'invalid_name'` sentinel.

## 2. Current state at HEAD

### 2.1 Dependencies (verified present at `5298d6a`)

| Dependency | goscape location | Notes |
|---|---|---|
| Player.publicChat / privateChat / tradeDuel | `player.go:172` | Declared, currently unwritten |
| Player.mutedUntil (`time.Time`) | `player.go:175` | Already present |
| Player.username | `player.go:67` | Set at login |
| (*Player).MessageGame | used in `handler_opheld.go:191`, etc. | Verified |
| `pkg/util/jstring.FromBase37(uint64) string` | `pkg/util/jstring/jstring.go:34` | Returns `"invalid_name"` for upper-bound; missing TS `% 37 == 0` check (see §2.4) |
| `pkg/util/jstring.ToBase37(string) uint64` | `pkg/util/jstring/jstring.go:14` | Already used by `player.go:360` |
| `world.NodeProduction bool` config | `config.go:43` (also `server_varp.go:74`) | **Already exists** — consumed by MAP_LIVE opcode; ReportAbuse moderator-mute branch reuses |
| `world.NodeMembers bool` config | `config.go:42` (used by handler_opheld.go) | Pattern reference |
| `gameHandlers [256]func(*Player, []byte) error` registry | `handlers_game.go:15` | Add 6 entries in `init()` |
| Opcode size table entries | `pkg/io/protocol/game/client/prot.go:104,107,108,110-113` | All 6 already declared |
| `(*Server).processCleanup()` per-tick reset hook | `tick.go:470` | Currently calls `ResetMasks()` only — will extend |
| Server constructor `NewServer(cfg, loginClient, logger)` | `server.go:128` | Add bridge field defaults |

### 2.2 Goscape conventions (confirmed via grep)

- **Default config keys via `f.BoolVar(&c.NodeProduction, "world.node-production", false, ...)`** — established at `config.go:72`. NAI-72 reuses (no new flag).
- **No-op-default subsystem stubs** — established by NAI-71's session-log deferral and the pre-existing `loginClient` interface pattern at `server.go:49`. NAI-72 follows: an interface field on Server, default-initialized to a no-op impl, swappable for tests via a `recordingBridges` capture impl.
- **Per-tick reset of player transient state** — `processCleanup()` at `tick.go:470` is the goscape analog of TS's "reset players" loop at World.ts:1138. NAI-72 adds the new flag-reset lines to the existing `for _, p := range players` block; does not introduce a new pass.
- **`fromBase37` invalid-name sentinel** — both TS (JString.ts:39,43) and goscape (jstring.go:37) return the literal string `"invalid_name"`. Gate condition is `jstring.FromBase37(username) == "invalid_name"`.
- **Test-fixture conn drainage** — established pattern (e.g. `handler_opheld_test.go`); use for ReportAbuse's `MessageGame` ack.

### 2.3 Verified-absent claims (premise grep evidence)

Per `risk_register_premise_grep.md` and `controller_preflight.md`:

```
$ rg -n "socialProtect|reportAbuseProtect|staffModLevel" pkg/ modules/
(no hits — confirms all three Player fields and per-tick reset are absent)

$ rg -n "FriendsBridge|LoginBridgeMod|LoggerBridge|friendsBridge|loggerBridge" modules/
(no hits — confirms bridge interfaces and Server fields are absent)

$ rg -n "ReportAbuseReason" modules/ pkg/
(no hits — confirms enum is absent)

$ rg -n "addFriend|removeFriend|addIgnore|removeIgnore|sendPrivateChatModeToFriendsServer|notifyPlayerBan|notifyPlayerMute|notifyPlayerReport" modules/ pkg/
(no hits — confirms no in-engine friend/ignore/ban/mute/report state)

$ rg -n "world\.NodeProduction|cfg\.NodeProduction" modules/
modules/world/server_varp.go:74:	if w.s == nil || !w.s.cfg.NodeProduction {
(verifies existing consumer — MAP_LIVE opcode — so the field is live)

$ rg -n "gameHandlers\[(11|79|118|148|171|190|244)\]" modules/
(no hits — confirms all 6 opcodes silently discard at HEAD)
```

### 2.4 Outstanding TS-fidelity gap (FOLDED INTO T1, not a deviation)

`pkg/util/jstring.FromBase37` matches TS lines 36-40 (upper-bound + negative check) but is missing the TS `value % 37n === 0n` check at JString.ts:42-44. Effect: usernames whose base37 encoding ends in a `_` character (or trail-zero positions) escape goscape's `"invalid_name"` gate. This is a pre-existing latent narrow divergence; NAI-72 surfaces it because the social handlers are the first consumers to gate on the sentinel. Folded into T1 as a 3-LOC fix at `jstring.go:38`. Not tagged as a deviation — it's a true-to-TS bug fix, not a deferred behavior.

### 2.5 Out-of-scope dependencies (tracked as deviations)

- **`friendThread` worker.** TS spawns a separate Node Worker for `addFriend`/`removeFriend`/`addIgnore`/`removeIgnore`/`sendPrivateChatModeToFriendsServer` (World.ts:1534-1576). Goscape has no friends-server module. Deferred via `NAI-72-D-FRIENDS-SERVER-BRIDGE`.
- **`loginThread` `player_ban` / `player_mute` channels.** TS uses the existing login Worker for moderation messages (World.ts:2275-2294). Goscape's `loginClient` is auth-only (login + logout + save), with no ban/mute IPC. Deferred via `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD`.
- **`loggerThread` worker.** TS posts `'report'` events to a dedicated logger worker (World.ts:2305). Goscape has no logger subsystem. Deferred via `NAI-72-D-LOGGER-BRIDGE`. (Same closure path will activate the deferred EventTracking handler in a future sub-spec.)
- **`Player.input` recording subsystem.** TS `notifyPlayerReport` flips `offenderPlayer.submitInput = true` on MACROING/BUG_ABUSE reasons (World.ts:2298-2304). The `input` and `submitInput` properties are not ported. Deferred via `NAI-72-D-INPUT-RECORDING-NOT-PORTED`. (Same gap blocks the EventTracking handler.)
- **DB-side staff-level loader.** TS reads `staffmodlevel` from the DB at `World.ts:1895` and writes it to `Player.staffModLevel`. Goscape's login DB schema has no `staffmodlevel` column and `LoginClient.UpdatePlayer` does not propagate it. Deferred via `NAI-72-D-STAFFMODLEVEL-DEAD-WRITER`. The field exists post-NAI-72 but defaults to 0 for all players (consume_reserved_constant pattern: opening the consumer with no producer).

## 3. Scope

### 3.1 Out-of-scope (explicitly deferred)

| Item | Why | Defer-tag / closure |
|---|---|---|
| MessagePrivate handler (op 148) | Needs WordPack port + LoggerBridge `private_message` channel | Future "MessagePrivate + WordPack" sub-spec; `socialProtect` reset hook from NAI-72 covers MessagePrivate when ported |
| EventTracking handler (op 81) | Needs `Player.input` recording subsystem | Future "input-recording subsystem" sub-spec; closes `NAI-72-D-INPUT-RECORDING-NOT-PORTED` |
| ChatSetMode `logPublicChat` integration | Belongs to MESSAGE_PUBLIC handler — already wired separately at `handlers_game.go:69`. CHAT_SETMODE itself does not call `logPublicChat`. | n/a |
| `Player.input.hasSeenReport`, `submitInput`, `recordedBlobsSizeTotal`, `record(bytes)` | All input-recording — see above | `NAI-72-D-INPUT-RECORDING-NOT-PORTED` |
| Real friends-server, login-server-mod, logger-server impls | Per-bridge deferral (5 deviations) | Each deviation lists its closure path |
| Full `Player.resetEntity(false)` port | `protect`, `chatColour`, `chatEffect`, `chatRights`, `chatMessage`, `logMessage` resets each belong to other sub-specs (chat-mask / script-protect) | Doc-comment audit note `NAI-72-N-RESETENTITY-PARTIAL` only; not a numbered deviation |
| Wiring `staffModLevel` reads from existing handlers (e.g. `ClientCheatHandler.ts:52,56,189,483`) | Out of scope — would require login DB column + bridge propagation. Field is read-once by ReportAbuse only. | `NAI-72-D-STAFFMODLEVEL-DEAD-WRITER` |

### 3.2 In-scope

#### 3.2.1 New Player fields (`modules/world/player.go`)

Insert immediately after the existing chat-state block (`player.go:172-176`):

```go
// === social spam protection (NAI-72) ===
// socialProtect gates FRIENDLIST_ADD/DEL, IGNORELIST_ADD/DEL, and
// (future) MESSAGE_PRIVATE — at most one such packet per tick per
// player. Reset to false in processCleanup. Set to true at handler-
// success bottom. Mirrors TS Player.socialProtect (Player.ts:386,
// reset Player.ts:466).
socialProtect bool

// reportAbuseProtect gates REPORT_ABUSE — at most one per tick per
// player. Reset/set semantics identical to socialProtect. Mirrors
// TS Player.reportAbuseProtect (Player.ts:387, reset Player.ts:467).
reportAbuseProtect bool

// staffModLevel is the player's moderator level: 0=user, 1+=mod.
// Read by the ReportAbuse moderator-mute branch (handler_reportabuse.go).
// Writer is deferred — see NAI-72-D-STAFFMODLEVEL-DEAD-WRITER.
// Field defaults to 0 for all players at HEAD. Mirrors TS
// Player.staffModLevel (Player.ts:370).
staffModLevel int
```

#### 3.2.2 New per-tick reset (`modules/world/tick.go:processCleanup`)

Extend the existing player loop:

```go
for _, p := range players {
    p.ResetMasks()
    // NAI-72 — TS Player.resetEntity(false) at Player.ts:466-467.
    // Reset social/report spam-protect flags so the next tick admits
    // at most one social/report packet per type per player.
    // (Other resetEntity fields — protect, chatColour/Effect/Rights,
    // chatMessage, logMessage — belong to other sub-specs; tracked
    // as NAI-72-N-RESETENTITY-PARTIAL.)
    p.socialProtect = false
    p.reportAbuseProtect = false
}
```

No changes to NPC loop, zone reset, or rsbuf cleanup.

#### 3.2.3 New file `modules/world/bridges.go` — bridge interfaces + no-op + recording capture

```go
package world

import "time"

// FriendsBridge mirrors TS World.friendThread.postMessage(...) for
// social-list mutations and chat-mode propagation. Real impl is a
// future friends-server module (see NAI-72-D-FRIENDS-SERVER-BRIDGE).
type FriendsBridge interface {
    AddFriend(playerUsername string, target uint64)
    RemoveFriend(playerUsername string, target uint64)
    AddIgnore(playerUsername string, target uint64)
    RemoveIgnore(playerUsername string, target uint64)
    SetChatMode(playerUsername string, privateChat int)
}

// LoginBridgeMod mirrors TS World.loginThread.postMessage('player_ban'/
// 'player_mute', ...). The existing LoginClient is auth-only; this is
// a separate moderation channel. Real impl deferred via
// NAI-72-D-LOGIN-SERVER-BRIDGE-MOD.
type LoginBridgeMod interface {
    NotifyPlayerBan(staff, username string, until time.Time)
    NotifyPlayerMute(staff, username string, until time.Time)
}

// LoggerBridge mirrors TS World.loggerThread.postMessage('report', ...).
// Real impl deferred via NAI-72-D-LOGGER-BRIDGE. The same closure path
// will activate the EventTracking handler.
type LoggerBridge interface {
    // NotifyPlayerReport posts an abuse report. reason is the string
    // label of the ReportAbuseReason enum value (e.g. "MACROING").
    NotifyPlayerReport(player *Player, offender, reason string)
}

// noopBridges is the default impl wired into NewServer. It records
// nothing and performs no I/O. All 11 methods are no-ops.
type noopBridges struct{}

func (noopBridges) AddFriend(string, uint64)              {}
func (noopBridges) RemoveFriend(string, uint64)           {}
func (noopBridges) AddIgnore(string, uint64)              {}
func (noopBridges) RemoveIgnore(string, uint64)           {}
func (noopBridges) SetChatMode(string, int)               {}
func (noopBridges) NotifyPlayerBan(string, string, time.Time)  {}
func (noopBridges) NotifyPlayerMute(string, string, time.Time) {}
func (noopBridges) NotifyPlayerReport(*Player, string, string) {}
```

Plus a test-only `recordingBridges` (in `bridges_test.go`) that captures every call into a slice of typed records, enabling per-handler assertion:

```go
type recordedFriendsCall struct {
    method                 string // "AddFriend" | "RemoveFriend" | "AddIgnore" | "RemoveIgnore" | "SetChatMode"
    playerUsername         string
    targetUsername37       uint64
    privateChatMode        int    // SetChatMode only
}
type recordedLoginModCall struct {
    method   string // "NotifyPlayerBan" | "NotifyPlayerMute"
    staff    string
    username string
    until    time.Time
}
type recordedLoggerCall struct {
    method   string // "NotifyPlayerReport"
    player   *Player
    offender string
    reason   string
}
type recordingBridges struct {
    friends []recordedFriendsCall
    loginMod []recordedLoginModCall
    logger  []recordedLoggerCall
}
// (impls append to slices)
```

#### 3.2.4 New Server fields (`modules/world/server.go:Server`)

```go
// Social/moderation bridges (NAI-72). Default to noopBridges{} in
// NewServer; tests inject recordingBridges. Real impls deferred per
// NAI-72-D-{FRIENDS-SERVER-BRIDGE, LOGIN-SERVER-BRIDGE-MOD, LOGGER-BRIDGE}.
friendsBridge  FriendsBridge
loginBridgeMod LoginBridgeMod
loggerBridge   LoggerBridge
```

`NewServer` initialization (single statement, all three default to the same value):

```go
s.friendsBridge = noopBridges{}
s.loginBridgeMod = noopBridges{}
s.loggerBridge = noopBridges{}
```

#### 3.2.5 New file `modules/world/social.go` — ReportAbuseReason enum

```go
package world

// ReportAbuseReason mirrors TS ReportAbuse.ts:4-17. Values are
// sent over the wire by REPORT_ABUSE (opcode 190); out-of-range
// values trigger an automated ban (per ReportAbuseHandler.ts:14).
type ReportAbuseReason uint8

const (
    ReportAbuseOffensiveLanguage   ReportAbuseReason = 0
    ReportAbuseItemScamming        ReportAbuseReason = 1
    ReportAbusePasswordScamming    ReportAbuseReason = 2
    ReportAbuseBugAbuse            ReportAbuseReason = 3
    ReportAbuseStaffImpersonation  ReportAbuseReason = 4
    ReportAbuseAccountSharing      ReportAbuseReason = 5
    ReportAbuseMacroing            ReportAbuseReason = 6
    ReportAbuseMultiLogging        ReportAbuseReason = 7
    ReportAbuseEncouragingBreakRules ReportAbuseReason = 8
    ReportAbuseMisuseCustomerSupport ReportAbuseReason = 9
    ReportAbuseAdvertisingWebsite  ReportAbuseReason = 10
    ReportAbuseRealWorldTrading    ReportAbuseReason = 11
)

// reasonLabel returns the canonical string label for a ReportAbuseReason
// value, used as the LoggerBridge.NotifyPlayerReport `reason` argument.
// Out-of-range values return "" (caller is responsible for range-checking
// before calling, per the ReportAbuse handler's gate-then-call order).
func reasonLabel(r ReportAbuseReason) string {
    switch r {
    case ReportAbuseOffensiveLanguage:    return "OFFENSIVE_LANGUAGE"
    case ReportAbuseItemScamming:         return "ITEM_SCAMMING"
    case ReportAbusePasswordScamming:     return "PASSWORD_SCAMMING"
    case ReportAbuseBugAbuse:             return "BUG_ABUSE"
    case ReportAbuseStaffImpersonation:   return "STAFF_IMPERSONATION"
    case ReportAbuseAccountSharing:       return "ACCOUNT_SHARING"
    case ReportAbuseMacroing:             return "MACROING"
    case ReportAbuseMultiLogging:         return "MULTI_LOGGING"
    case ReportAbuseEncouragingBreakRules: return "ENCOURAGING_BREAK_RULES"
    case ReportAbuseMisuseCustomerSupport: return "MISUSE_CUSTOMER_SUPPORT"
    case ReportAbuseAdvertisingWebsite:   return "ADVERTISING_WEBSITE"
    case ReportAbuseRealWorldTrading:     return "REAL_WORLD_TRADING"
    }
    return ""
}
```

#### 3.2.6 New handler files

**`modules/world/handler_chatsetmode.go`** — opcode 244, fixed-3:

```go
package world

import "github.com/zsrv/goscape/pkg/io/packet"

// handleChatSetMode handles client opcode 244 (CHAT_SETMODE), payload
// 3 bytes: g1 publicChat, g1 privateChat, g1 tradeDuel.
//
// Mirrors TS ChatSetModeHandler.ts:7-13 — no socialProtect gate (TS
// does not gate this opcode). Activates Player.publicChat /
// .privateChat / .tradeDuel which are declared at player.go:172 but
// were unwritten prior to NAI-72.
func handleChatSetMode(p *Player, payload []byte) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    pk := packet.NewPacket(payload)
    p.publicChat = int(pk.G1())
    p.privateChat = int(pk.G1())
    p.tradeDuel = int(pk.G1())
    p.client.server.friendsBridge.SetChatMode(p.username, p.privateChat)
    return nil
}
```

**`modules/world/handler_social_list.go`** — opcodes 11/79/118/171, fixed-8 each, four handlers sharing one helper:

```go
package world

import (
    "github.com/zsrv/goscape/pkg/io/packet"
    util "github.com/zsrv/goscape/pkg/util/jstring"
)

// socialListAction enumerates the four bridge methods invoked by the
// Friend/Ignore handler family.
type socialListAction int

const (
    socialAddFriend    socialListAction = iota // op 118
    socialRemoveFriend                         // op 11
    socialAddIgnore                            // op 79
    socialRemoveIgnore                         // op 171
)

// handleSocialList is the shared body of FRIENDLIST_ADD/DEL and
// IGNORELIST_ADD/DEL. All four:
//   1. Decode g8 username (uint64 base37).
//   2. Early-return if socialProtect is set OR the username decodes to
//      the "invalid_name" sentinel.
//   3. Dispatch to the appropriate FriendsBridge method.
//   4. Set socialProtect = true.
//
// Mirrors TS {Friend,Ignore}List{Add,Del}Handler.ts:8-15 (all four
// share an identical body shape modulo the World call).
func handleSocialList(p *Player, payload []byte, action socialListAction) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    pk := packet.NewPacket(payload)
    username := pk.G8()

    if p.socialProtect || util.FromBase37(username) == "invalid_name" {
        return nil
    }

    fb := p.client.server.friendsBridge
    switch action {
    case socialAddFriend:
        fb.AddFriend(p.username, username)
    case socialRemoveFriend:
        fb.RemoveFriend(p.username, username)
    case socialAddIgnore:
        fb.AddIgnore(p.username, username)
    case socialRemoveIgnore:
        fb.RemoveIgnore(p.username, username)
    }
    p.socialProtect = true
    return nil
}

func handleFriendListAdd(p *Player, payload []byte) error {
    return handleSocialList(p, payload, socialAddFriend)
}
func handleFriendListDel(p *Player, payload []byte) error {
    return handleSocialList(p, payload, socialRemoveFriend)
}
func handleIgnoreListAdd(p *Player, payload []byte) error {
    return handleSocialList(p, payload, socialAddIgnore)
}
func handleIgnoreListDel(p *Player, payload []byte) error {
    return handleSocialList(p, payload, socialRemoveIgnore)
}
```

**`modules/world/handler_reportabuse.go`** — opcode 190, fixed-10:

```go
package world

import (
    "time"

    "github.com/zsrv/goscape/pkg/io/packet"
    util "github.com/zsrv/goscape/pkg/util/jstring"
)

// handleReportAbuse handles client opcode 190 (REPORT_ABUSE), payload
// 10 bytes: g8 offender, g1 reason, g1 moderatorMute(bool).
//
// Mirrors TS ReportAbuseHandler.ts:9-26. Branch order:
//   1. reportAbuseProtect early-return (no protect-set on this branch).
//   2. Out-of-range reason → automated 48h ban + early-return
//      (no protect-set).
//   3. Optional moderator-mute branch (gated 3-way: moderatorMute &&
//      staffModLevel > 0 && cfg.NodeProduction).
//   4. Logger bridge call.
//   5. MessageGame ack.
//   6. reportAbuseProtect = true.
//
// The MACROING/BUG_ABUSE submitInput=true branch (TS World.ts:2298-2304)
// is intentionally omitted — input-recording subsystem not ported
// (NAI-72-D-INPUT-RECORDING-NOT-PORTED).
func handleReportAbuse(p *Player, payload []byte) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    if p.reportAbuseProtect {
        return nil
    }
    pk := packet.NewPacket(payload)
    offender := pk.G8()
    reason := ReportAbuseReason(pk.G1())
    moderatorMute := pk.G1() != 0

    s := p.client.server

    // Range-gate: TS sets a 48h automated ban for out-of-range values
    // (anti-tamper guard against modified clients).
    if reason < ReportAbuseOffensiveLanguage || reason > ReportAbuseRealWorldTrading {
        s.loginBridgeMod.NotifyPlayerBan("automated", p.username, time.Now().Add(48*time.Hour))
        return nil
    }

    // Moderator-mute branch: only fires if the reporting player has
    // staff level > 0 AND the bool flag is set AND we're in production.
    // Mutes the offender for 48h.
    if moderatorMute && p.staffModLevel > 0 && s.cfg.NodeProduction {
        s.loginBridgeMod.NotifyPlayerMute(p.username, util.FromBase37(offender), time.Now().Add(48*time.Hour))
    }

    s.loggerBridge.NotifyPlayerReport(p, util.FromBase37(offender), reasonLabel(reason))
    p.MessageGame("Thank-you, your abuse report has been received")
    p.reportAbuseProtect = true
    return nil
}
```

#### 3.2.7 Handler registry (`modules/world/handlers_game.go init()`)

Insert after the OPHELD block (line 67):

```go
gameHandlers[244] = handleChatSetMode    // CHAT_SETMODE
gameHandlers[118] = handleFriendListAdd  // FRIENDLIST_ADD
gameHandlers[11]  = handleFriendListDel  // FRIENDLIST_DEL
gameHandlers[79]  = handleIgnoreListAdd  // IGNORELIST_ADD
gameHandlers[171] = handleIgnoreListDel  // IGNORELIST_DEL
gameHandlers[190] = handleReportAbuse    // REPORT_ABUSE
```

#### 3.2.8 `pkg/util/jstring.FromBase37` bug fix (true-to-TS)

Insert after the existing upper-bound check (line 38):

```go
if v%37 == 0 {
    return "invalid_name"
}
```

Add a unit test pinning both branches.

## 4. Files diff summary

| File | Change |
|---|---|
| `pkg/util/jstring/jstring.go` | +3 LOC (TS-fidelity bug fix) |
| `pkg/util/jstring/jstring_test.go` | +N LOC (`TestFromBase37InvalidNameMod37`, `TestFromBase37InvalidNameUpperBound`) |
| `modules/world/player.go` | +~18 LOC (3 fields + comments) |
| `modules/world/tick.go` | +~6 LOC in `processCleanup` (2 assignments + comment) |
| `modules/world/server.go` | +~5 LOC (3 Server fields + 3 default-init lines in NewServer) |
| `modules/world/bridges.go` | NEW ~70 LOC (3 interfaces + noopBridges) |
| `modules/world/bridges_test.go` | NEW ~80 LOC (recordingBridges + helpers) |
| `modules/world/social.go` | NEW ~40 LOC (enum + reasonLabel) |
| `modules/world/handler_chatsetmode.go` | NEW ~25 LOC |
| `modules/world/handler_chatsetmode_test.go` | NEW ~70 LOC |
| `modules/world/handler_social_list.go` | NEW ~75 LOC (4 handlers + shared core) |
| `modules/world/handler_social_list_test.go` | NEW ~200 LOC (gate / no-gate / per-action / invalid_name) |
| `modules/world/handler_reportabuse.go` | NEW ~70 LOC |
| `modules/world/handler_reportabuse_test.go` | NEW ~250 LOC (5 branches × ~3 cases) |
| `modules/world/handlers_game.go` | +6 lines in `init()` |
| `modules/world/processCleanup_test.go` (or extend `tick_test.go`) | +~50 LOC (per-tick reset coverage for all 3 new flags + the existing ResetMasks pin) |
| `modules/world/handlers_game_test.go` | +~80 LOC (integration smoke: drive all 6 opcodes through one recordingBridges, assert per-op effect) |

Approximate total: ~1100 LOC including tests; ~370 LOC production. Test density is the standard cadence ratio (~3:1 per recent NAI sub-specs).

## 5. Cadence

Full sub-spec, 4 implementation tasks + close. Two-stage review per `runescript_cadence.md` and `superpowers_code_reviewer_model.md` (Sonnet only).

| Task | Scope | Approx LOC | Reviewer |
|---|---|---|---|
| **T1: Foundation** | jstring.FromBase37 fix + test; 3 Player fields; processCleanup hook + test; bridges.go + bridges_test.go (interfaces, noop, recording capture); social.go (enum + reasonLabel); Server fields + NewServer init | ~250 LOC (impl + tests) | **Stage 1 review (Sonnet)** — pattern lock: bridge interface shape, recordingBridges idiom, processCleanup hook minimal-change, deviation tag completeness, jstring fix correctness |
| **T2: ChatSetMode** | handler_chatsetmode.go + tests; opcode 244 binding | ~95 LOC | (no review) |
| **T3: Friend/Ignore family** | handler_social_list.go + tests; 4 opcode bindings | ~275 LOC | (no review) |
| **T4: ReportAbuse** | handler_reportabuse.go + tests; opcode 190 binding | ~320 LOC | **Stage 2 review (Sonnet)** — TS-fidelity gate: every handler line-by-line vs TS; verify `spec_unconditional_swap_in_arm_block` not-applicable claim; verify `submitInput` omission is doc-tagged; verify ReportAbuse range-check direction; verify moderator-mute 3-way gate order |
| **Close** | integration smoke test (all 6 opcodes via one recordingBridges), deviation tally, memory + close-commit trailer | ~80 LOC | (no review) |

Dispatch via `superpowers:subagent-driven-development` per `execution_mode_default.md`. Controller pre-flight gate before each task per `controller_preflight.md`.

## 6. Risk register (premise verification at spec-write)

Per `risk_register_premise_grep.md`. Each premise is grep-evidenced in §2.3 above.

| # | Premise | Evidence | Risk |
|---|---|---|---|
| R1 | All 6 opcodes are accepted at the wire but unhandled at HEAD. | `pkg/io/protocol/game/client/prot.go:104,107,108,110-113` declare; no `gameHandlers[N]` assignment for any of N∈{11,79,118,148,171,190,244}. | **Verified low.** Existing OPHELD pattern (NAI-71) ports the same shape. |
| R2 | `processCleanup` runs every tick on every player; modifying the loop body is the correct hook for per-tick flag reset. | `tick.go:470` calls `processCleanup`; the player loop at `tick.go:475` iterates `s.playerLoop` (full set). TS analog at World.ts:1138 calls `player.resetEntity(false)` in same position. | **Verified low.** No goscape consumer treats `socialProtect`/`reportAbuseProtect` as carried across ticks. |
| R3 | `world.NodeProduction` config is the correct gate for the moderator-mute branch. | `cfg.NodeProduction` already gates MAP_LIVE at `server_varp.go:74`; declared at `config.go:43`; matches TS `Environment.NODE_PRODUCTION`. | **Verified low.** Same semantics, no new config key. |
| R4 | `FromBase37` returns the literal `"invalid_name"` sentinel that the TS handlers gate on. | `jstring.go:37` returns the literal (matching TS JString.ts:39,43). One missing branch — folded into T1 fix. | **Verified low.** |
| R5 | `socialProtect`/`reportAbuseProtect` flags are NOT consumed by any code path besides the new handlers + the new reset. | Per §2.3 grep — no hits anywhere. | **Verified low.** Brand-new fields. |
| R6 | `staffModLevel` field has no producer at HEAD. | Per §2.3 grep — no hits. Tracked as `NAI-72-D-STAFFMODLEVEL-DEAD-WRITER`. | **Acknowledged.** All players have `staffModLevel == 0`; ReportAbuse moderator-mute branch never fires until login-side wiring lands. ReportAbuse tests use `p.staffModLevel = 1` directly to exercise the branch. |
| R7 | `spec_unconditional_swap_in_arm_block` lesson does NOT apply to any handler in this bundle. | All five "set protect = true" sites are at the BOTTOM of the function, OUTSIDE the gate-check. ReportAbuse moderator-mute branch is independently gated, not nested inside the protect-check. No arm-fallback `if (!script)` patterns in any of these handlers. | **Verified not applicable.** Stage 2 reviewer will re-confirm. |
| R8 | `plan_geometry_premise_pretrace` lesson does NOT apply. | No geometry/coord/distance arithmetic in any handler in this bundle. | **Verified not applicable.** |
| R9 | `(*Player).MessageGame` accepts a string and is the correct ack channel. | Used in `handler_opheld.go:191`, `handler_opnpc.go:254`, etc. — Stage 1 reviewer cross-checks signature. | **Verified low.** |
| R10 | `(*Server).cfg` is the correct field for reading `NodeProduction` from the handler. | `server_varp.go:74` reads `w.s.cfg.NodeProduction` — same access pattern. | **Verified low.** |
| R11 | The 4 social-list bridge calls (Add/Remove × Friend/Ignore) all take `(playerUsername string, target uint64)` — same signature. | TS World.ts:1534-1568 — all four accept `(player: Player, targetUsername37: bigint)` and post identical `{ username, target }` payloads. | **Verified low.** |
| R12 | ReportAbuse `MessageGame` text matches client assertion. | Java client expects exact string `"Thank-you, your abuse report has been received"` — TS quotes verbatim. Pin in test. | **Verified low.** |
| R13 | `gbool()` decode of `moderatorMute` reads 1 byte, true if non-zero. | TS Packet.gbool() reads 1 byte (`return this.g1() === 1`). Goscape pattern: `pk.G1() != 0` (used elsewhere). | **Verified low.** |
| R14 | The handler files belong in the `world` package and follow the `handler_<opname>.go` naming convention. | Established precedent: `handler_opheld.go`, `handler_oploc.go`, `handler_opnpc.go`, `handler_opobj.go`, `handler_inv_button.go`. NAI-72 follows this convention: `handler_chatsetmode.go`, `handler_social_list.go`, `handler_reportabuse.go`. | **Verified low.** |

## 7. Out of scope

See §3.1.

## 8. Memory / lessons applied

| Memory | Application |
|---|---|
| `runescript_cadence.md` | Full 4-task cadence + two-stage review |
| `true_to_ts_gate.md` | Every behavioral decision cited against TS source line |
| `controller_preflight.md` | Premise grep evidence in §2.3; pre-flight before every implementer dispatch |
| `risk_register_premise_grep.md` | §6 register with grep evidence per premise |
| `consume_reserved_constant.md` | `staffModLevel` field opens consumer with no producer; closure path documented |
| `spec_unconditional_swap_in_arm_block.md` | Risk register R7 explicitly verifies not-applicable; Stage 2 reviewer re-confirms |
| `plan_geometry_premise_pretrace.md` | Risk register R8 explicitly verifies not-applicable |
| `defensive_gate_doc_comment_label.md` | `nil` checks on `p.client`/`p.client.server` will be labeled "(goscape defensive; TS reaches via static accessor)" in handler doc-comments |
| `enumerate_all_sites.md` | All 6 opcode bindings enumerated in §3.2.7 |
| `plan_grep_helper_patterns.md` | Reuses `MessageGame`, `FromBase37`, `packet.NewPacket`, `cfg.NodeProduction` instead of inlining |
| `superpowers_code_reviewer_model.md` | Both review dispatches on Sonnet only |
| `execution_mode_default.md` | Dispatch via subagent-driven-development |
| `close_commit_memory_trailer.md` | Close commit will carry `Opens memory:` trailer for all 5 deviations |
| `superpowers_clear_between_spec_and_impl.md` | Plan written next; resume prompt emitted; user `/clear` before T1 dispatch |

## 9. Close-commit deviations summary (template)

```
chore(close): NAI-72 — social subsystem foundation
              (ChatSetMode + Friend/Ignore + ReportAbuse — 6 opcodes)

Closes 6 silent-discard slots in gameHandlers[]:
  244 CHAT_SETMODE
  118 FRIENDLIST_ADD
   11 FRIENDLIST_DEL
   79 IGNORELIST_ADD
  171 IGNORELIST_DEL
  190 REPORT_ABUSE

T1 Foundation (jstring fix + 3 Player fields + processCleanup +
   bridges + ReportAbuseReason)            (TBD-SHA)
T2 ChatSetMode handler                     (TBD-SHA)
T3 Friend/Ignore family (4 handlers)       (TBD-SHA)
T4 ReportAbuse handler                     (TBD-SHA)

Wire-behaviour delta:
- Chat-set-mode requests now write Player.publicChat/privateChat/
  tradeDuel and propagate to friends-server (stub).
- Friend/Ignore add/remove now invoke the friends-server bridge
  (stub) and gate by socialProtect.
- Report-abuse requests now invoke the logger bridge (stub),
  acknowledge the player, and gate by reportAbuseProtect; out-of-
  range reasons trigger an automated ban via the login-mod bridge.
- pkg/util/jstring.FromBase37 now matches TS — % 37 == 0 returns
  "invalid_name" (true-to-TS bug fix surfaced by the social gate).

Opens memory: NAI-72-D-FRIENDS-SERVER-BRIDGE
Opens memory: NAI-72-D-LOGIN-SERVER-BRIDGE-MOD
Opens memory: NAI-72-D-LOGGER-BRIDGE
Opens memory: NAI-72-D-STAFFMODLEVEL-DEAD-WRITER
Opens memory: NAI-72-D-INPUT-RECORDING-NOT-PORTED
```

**Net deviation tally projection:** -0 closures, +5 opens = 14 → 19.
