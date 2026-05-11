# NAI-158: MessagePrivate handler (opcode 148) — design

## 1. Summary

Port TS `MessagePrivateHandler` (RuneScape /tell-style private chat) to
goscape. Adds the word-pack codec it depends on, extends `FriendsBridge`
with a `PrivateMessage` method, threads a `pmCount` counter through the
Server, and wires the dispatcher for opcode 148.

This is one of the 17 undispatched client opcodes catalogued by the
HEAD-of-`main` missing-handler audit (NAI-157 close-commit memo); the
other 16 are anti-cheat OPLOGIC/CYCLELOGIC and EVENT_CAMERA_POSITION
no-ops in both engines and out of scope.

## 2. Source

- TS handler: `Engine-TS/src/network/game/client/handler/MessagePrivateHandler.ts`
  (36 LOC; lines 10-35 are the handle method).
- TS routing: `Engine-TS/src/engine/World.ts:1631-1643` `sendPrivateMessage`
  (posts to `friendThread.postMessage` with payload
  `{type, username, staffLvl, pmId, target, message, coord}`).
- TS codec: `Engine-TS/src/wordenc/WordPack.ts` (95 LOC).
- Client opcode: 148 (`MESSAGE_PRIVATE`), size `-1` (dynamic 1-byte
  length), declared in goscape `pkg/io/protocol/game/client/prot.go:111`.

Per `ts_source_canonical_path.md`, only `LostCityRS/Engine-TS` is the
porting reference.

## 3. Pre-flight findings (HEAD)

| # | Dep | Status |
|---|---|---|
| 1 | base37 codec | OK — `pkg/util/jstring.{ToBase37,FromBase37}`; "invalid_name" sentinel already in `jstring_test.go` |
| 2 | WordPack codec | absent — ported as part of this sub-spec |
| 3 | Server-side MessagePrivate encoder | absent and not in scope — TS routes via FriendThread, no local-player effect; recipient-side encoder is a FriendServer concern that goscape lacks entirely |
| 4 | `Player.socialProtect` | OK — field + per-tick reset at `modules/world/tick.go:651` |
| 5 | Player-by-base37 lookup | OK — `Server.LookupPlayerByUsername(string)` at `server.go:834`; not needed by this handler since dispatch is bridge-only |
| 6 | Mute infra | partial — `Player.mutedUntil time.Time` field exists at `player.go:238` and is populated from `loginpb.PlayerLoginResponse.MutedUntil`, but no consumer reads it. NAI-158 activates this gate (consumes existing field; not a deviation per `dead_api_polish.md` since the field is *not* dead, it is unwired). |
| 7 | `NodeID` config | OK — `cfg.NodeID int32`, already passed to `LoginClient.WorldStartup` |
| 8 | RNG | OK — `math/rand/v2` is the canonical convention (handlers_player.go, npc_hunt.go); no test seam available (per `no_rng_seam_cascade_probe_bypass.md`) |
| 9 | CoordGrid pack | OK — `coordgrid.PackCoord(level, x, z) → int32`; used at `player.go:979` |
| 10 | `Player.staffModLevel` | OK — `int32`, populated from login proto |
| 11 | `LoginBridgeMod.NotifyPlayerBan` | OK — used by `handler_reportabuse.go:50` with identical signature `("automated", username, time.Now().Add(48*time.Hour))` |
| 12 | `FriendsBridge` + recordingBridges | OK — interface and test recorder at `bridges.go:8` / `bridges_test.go`; extension pattern established |

## 4. Cadence

Full cadence (brainstorm → spec → plan → subagent-driven TDD); compressed
not viable per `compressed_cadence.md` (scope ~95 LOC behavioral + ~200
LOC tests across 4 files including a new package).

Execution per `execution_mode_default.md`: dispatch via
subagent-driven-development.

## 5. Architecture

```
TCP byte stream
    ├─ dispatcher (modules/world/handlers_game.go)
    │     opcode 148 → handleMessagePrivate(p, payload)
    ▼
modules/world/handler_message_private.go
    ├─ defensive nil-check (p.client, p.client.server)
    ├─ G8 → target uint64 (base37)
    ├─ inputLen = len(payload) - 8
    ├─ gates (TS-order; no protect-set on any early return):
    │     1. p.socialProtect || inputLen > 100
    │     2. !p.mutedUntil.IsZero() && time.Now().Before(p.mutedUntil)
    │     3. FromBase37(target) == "invalid_name"
    │           → s.loginBridgeMod.NotifyPlayerBan("automated", p.username, now+48h)
    │             return nil
    ├─ msg := wordpack.Unpack(pk, inputLen)
    ├─ coord := coordgrid.PackCoord(p.level, p.x, p.z)
    ├─ s.friendsBridge.PrivateMessage(p.username, p.staffModLevel,
    │                                  s.nextPmId(), target, msg, coord)
    └─ p.socialProtect = true
```

No new dispatcher infrastructure. No new packages other than
`pkg/wordenc/wordpack`.

## 6. Components

### 6.1 `pkg/wordenc/wordpack` (new package)

Package path preserves TS namespace `wordenc/WordPack.ts`, leaving room
for a future `pkg/wordenc/wordenc` (BadWords/Domains/Tlds filter chain).
Package name `wordpack`.

Public surface, verbatim parity with TS:

```go
// Unpack decodes length bytes of word-packed input from pk starting at
// pk.Pos, returning the sentence-cased plain text. Mirrors TS
// wordenc/WordPack.ts:14-41.
func Unpack(pk *packet.Packet, length int) string

// Pack encodes input as word-packed bytes appended to pk. Inputs longer
// than 80 chars are truncated; output is lowercased before lookup.
// Mirrors TS wordenc/WordPack.ts:43-78.
func Pack(pk *packet.Packet, input string)
```

Internal lookup table: package-level `var charLookup = [...]string{...}`
(60 entries, verbatim from TS lines 5-12). Stored as `[]string` not
`[]byte` because the TS table includes the multi-byte UTF-8 char `£`
(plus `$`, `%`, `"`, `[`, `]`); per-entry length-1 substrings preserve
exact TS semantics.

Internal helper `toSentenceCase(string) string` mirrors TS lines 80-94.

No external imports outside `pkg/io/packet`.

### 6.2 `modules/world/bridges.go` — `FriendsBridge.PrivateMessage`

Extend the interface:

```go
type FriendsBridge interface {
    AddFriend(playerUsername string, target uint64)
    RemoveFriend(playerUsername string, target uint64)
    AddIgnore(playerUsername string, target uint64)
    RemoveIgnore(playerUsername string, target uint64)
    SetChatMode(playerUsername string, privateChat int)
    PrivateMessage(playerUsername string, staffLvl int32, pmId uint32,
                   target uint64, message string, coord int32)
}
```

`coord` typed `int32` matching `coordgrid.PackCoord` return.
`pmId uint32` fits TS bit layout `(NodeID<<24) | (rand<<16) | counter`.

`noopBridges` gains a no-op `PrivateMessage` impl.

### 6.3 `modules/world/server.go` — `pmCount` field

Add field on `Server` struct:

```go
pmCount uint32 // monotonic counter for FriendThread pmId low-16 bits;
               // mirrors TS World.pmCount.
```

Zero-initialized via existing `NewServer` (no constructor change
required; struct-literal zero is correct).

### 6.4 `modules/world/server.go` — `nextPmId` helper

Lives alongside the `pmCount` field on `Server` (not in the handler
file) so unit-testing the helper does not require constructing a
`Player`:

```go
// nextPmId mirrors the pmId computation inside TS
// World.sendPrivateMessage (World.ts:1641):
//
//   (Environment.NODE_ID << 24) + ((Math.random() * 0xff) << 16)
//     + this.pmCount++
//
// Note: TS uses Math.random()*0xff which is [0, 255), so rand.IntN(0xff)
// (range [0, 254]) is the faithful port — NOT rand.IntN(256).
func (s *Server) nextPmId() uint32 {
    randByte := uint32(rand.IntN(0xff))
    pm := uint32(s.cfg.NodeID&0xff)<<24 | randByte<<16 | s.pmCount
    s.pmCount++
    return pm
}
```

### 6.5 `modules/world/handler_message_private.go` — handler

```go
func handleMessagePrivate(p *Player, payload []byte) error {
    if p.client == nil || p.client.server == nil {
        // goscape defensive; TS reaches via static accessor.
        return nil
    }
    pk := packet.NewPacket(payload)
    target := pk.G8()
    inputLen := len(payload) - 8
    if p.socialProtect || inputLen > 100 {
        return nil
    }
    if !p.mutedUntil.IsZero() && time.Now().Before(p.mutedUntil) {
        return nil
    }
    s := p.client.server
    if util.FromBase37(target) == "invalid_name" {
        s.loginBridgeMod.NotifyPlayerBan("automated", p.username,
            time.Now().Add(48*time.Hour))
        return nil
    }
    msg := wordpack.Unpack(pk, inputLen)
    coord := coordgrid.PackCoord(p.level, p.x, p.z)
    s.friendsBridge.PrivateMessage(p.username, p.staffModLevel,
        s.nextPmId(), target, msg, coord)
    p.socialProtect = true
    return nil
}
```

### 6.6 `modules/world/handlers_game.go` — dispatcher

Add a single entry for opcode 148 (shape grepped at plan-author time and
matched to the existing dispatch convention).

## 7. Data flow

1. Client sends opcode 148 with dynamic 1-byte length prefix.
2. Dispatcher reads N bytes payload (first 8 = target base37, remainder
   = word-packed input).
3. Handler reads `target` via `G8()`, computes `inputLen =
   len(payload) - 8` for the gate check, and (on the happy path)
   advances the same `*packet.Packet` through `wordpack.Unpack` which
   continues reading from `pk.Pos` for `inputLen` bytes.
4. Bridge call posts the decoded text + metadata to the friends-server
   stub. No local-player effect; recipient delivery is FriendServer's
   responsibility (deferred).
5. `socialProtect` flips true; reset to false next tick at
   `tick.go:651`.

## 8. Error handling

- Defensive `nil` check on `p.client` / `p.client.server` matches the
  established pattern in `handler_social_list.go:32-35` and
  `handler_reportabuse.go:32-35`. Labeled "goscape defensive; TS reaches
  via static accessor" per `defensive_gate_doc_comment_label.md`.
- All gate failures return `nil` (not an error). TS returns `false`
  meaning "do not advance protocol state"; goscape's handler signature
  is `error`, where `nil` is the success/no-op signal. Matches sibling
  handlers.
- Malformed payload (`len(payload) < 8`): `packet.Packet.G8()` panics
  on short reads per `packet_rw_pointer_gotcha.md`. This is acceptable
  per the convention used elsewhere — the dispatcher's outer recovery
  catches it; a short payload here is a protocol violation and is not
  expected from a well-behaved client. No defensive length check added.

## 9. Testing

### 9.1 `pkg/wordenc/wordpack/wordpack_test.go`

Table-driven tests:

1. **Round-trip:** `Pack(pk, "hello world") → Unpack(pk', n) ==
   "Hello world"` (sentence-case applied on decode but not on encode;
   equality holds for already-sentence-cased input).
2. **Carry encoding:** chars at indexes 13-59 require 12-bit encoding;
   verify `Pack("a!b")` then `Unpack` yields `"A!b"`.
3. **Length cap (encode):** `Pack(s)` truncates input to 80 chars (TS
   line 44-46).
4. **Length cap (decode):** `Unpack` caps decoded output at 80 chars
   (TS line 19: `pos < 80`).
5. **Sentence-case rules:** `"hello. world! foo"` →
   `"Hello. World! Foo"` (capitalize after `.` and `!`; no other
   punctuation triggers; matches TS lines 80-94 verbatim).
6. **`£` round-trip:** explicit case that the multi-byte UTF-8 char
   survives Pack/Unpack (justifies the `[]string` lookup table choice).
7. **Empty input:** `Pack(pk, "")` writes nothing; `Unpack(pk, 0)`
   returns `""`.

### 9.2 `modules/world/bridges_test.go` (extension)

1. **recordingBridges captures PrivateMessage** — add
   `privateMsgs []recordedPrivateMessage` field with all six args.
   Assert capture parity with sibling methods (matches
   `helper_as_oracle_test_anti_pattern.md` — pin call-site AND output
   shape, not the helper as its own oracle).
2. Interface conformance for noopBridges already covered by the
   existing `_ FriendsBridge = noopBridges{}` assertion — no new test.

### 9.3 `modules/world/server_pmid_test.go`

Standalone tests for `Server.nextPmId`:

1. **pmCount monotonicity** — `s.nextPmId()` called 3× yields counter
   bytes 0, 1, 2 in bits 0-15. Random bits 16-23 masked
   (`pmId & 0xff00ffff`) before assertion per
   `no_rng_seam_cascade_probe_bypass.md`.
2. **NodeID byte placement** — set `s.cfg.NodeID = 0x42`, call
   `nextPmId()`, assert `(pmId >> 24) & 0xff == 0x42`.
3. **Random byte in range** — call `nextPmId()` 32×, assert all
   `(pmId >> 16) & 0xff` values fall in `[0, 254]` (the TS off-by-one
   from `Math.random() * 0xff`).

### 9.4 `modules/world/handler_message_private_test.go`

5 cases, all using `newTestPlayer` + `recordingBridges`:

| # | Case | Setup | Assert |
|---|---|---|---|
| 1 | Happy path | target = ToBase37("alice"); 2-byte word-packed "hi" tail | `rec.privateMsgs[0]` has playerUsername=p.username, target=ToBase37("alice"), message="Hi" (sentence-cased), staffLvl=p.staffModLevel, pmId structure (NodeID byte + counter 0), coord=PackCoord(p.level,p.x,p.z); `p.socialProtect == true`; `s.pmCount == 1` |
| 2 | socialProtect gate | `p.socialProtect = true`; same payload as #1 | `len(rec.privateMsgs) == 0`; `len(rec.loginMod) == 0`; `s.pmCount == 0` |
| 3 | length>100 gate | 101-byte word-packed tail (any content) | `len(rec.privateMsgs) == 0`; `s.pmCount == 0`; `p.socialProtect == false` |
| 4 | mutedUntil gate | `p.mutedUntil = time.Now().Add(time.Hour)` | `len(rec.privateMsgs) == 0`; `len(rec.loginMod) == 0`; `p.socialProtect == false` |
| 5 | invalid_name → ban | target = 6582952005840035281 (sentinel from jstring_test.go) | `rec.loginMod[0]` = NotifyPlayerBan with staff="automated", username=p.username, until ~= now+48h (window-tolerant); `len(rec.privateMsgs) == 0`; `p.socialProtect == false`; `s.pmCount == 0` |

## 10. Deviations

**No active deviations.** Every TS branch maps to existing goscape
infrastructure. Two stub-tag references inherited (not new):

- `NAI-72-D-FRIENDS-SERVER-BRIDGE` — `friendsBridge.PrivateMessage` is a
  stub; real FriendServer impl is a separate future module.
- `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` — `loginBridgeMod.NotifyPlayerBan`
  is a stub; real moderation channel is a separate future module.

## 11. Task split (subagent-driven-TDD)

**T1: WordPack codec** — `pkg/wordenc/wordpack/wordpack.go` + tests.
Standalone; no imports from `modules/world`.

**T2: FriendsBridge.PrivateMessage + Server.pmCount + nextPmId** —
extend `bridges.go` interface; add `noopBridges.PrivateMessage`; add
`Server.pmCount` field and `Server.nextPmId` method in `server.go`;
extend `bridges_test.go` recorder; add `server_pmid_test.go`.

**T3: Handler wire-up** — `handler_message_private.go` + dispatcher
entry in `handlers_game.go`. Imports T1 and T2 outputs. Adds the 5
handler tests.

Each task is independently compilable and shippable in its own commit.

## 12. Memory hits

Cited in close-commit `Closes memory:` trailer per
`close_commit_memory_trailer.md`:

- `runescript_cadence.md` — full cadence routing
- `controller_preflight.md` — preflight performed before brainstorm
- `true_to_ts_gate.md` — every TS branch mapped to live goscape infra
- `dead_api_polish.md` — `mutedUntil` activated (was unwired)
- `no_rng_seam_cascade_probe_bypass.md` — pmId tests use bit-masking
- `audit_full_method_against_ts.md` — verbatim TS line read drove design
- `defensive_gate_doc_comment_label.md` — labeled nil-checks
- `helper_as_oracle_test_anti_pattern.md` — bridge tests pin output shape
- `spec_ts_source_read.md` — design reads TS source verbatim, no analogy

## 13. Out of scope

- WordEnc filter chain (BadWords/Domains/Tlds) — separate larger port,
  not needed for opcode 148.
- Server-side MessagePrivate encoder (recipient delivery) — FriendServer
  responsibility; goscape has no FriendServer impl.
- Friends-server real implementation — deferred via
  `NAI-72-D-FRIENDS-SERVER-BRIDGE`.
- The other 16 undispatched opcodes (15 anti-cheat OPLOGIC/CYCLELOGIC +
  EVENT_CAMERA_POSITION 189) — silent no-ops in TS as well; not on the
  porting roadmap.
