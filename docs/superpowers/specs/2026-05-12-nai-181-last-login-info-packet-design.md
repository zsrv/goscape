# NAI-181 — `LAST_LOGIN_INFO` server packet port

**Date:** 2026-05-12
**Status:** Design (combined spec + plan per `compressed_cadence.md`)
**Tracker:** Retires `NAI-162-D-LASTLOGIN-NO-PACKET` (4 in-code citations)
**Predecessor:** NAI-162 B1 (trivial-handler sweep #4 — exposed `LastLoginInfo()` on the `ActiveProtectedPlayer` interface as a no-op pending ServerProt port)
**HEAD at design:** main (post-NAI-180 close `2cb8a09`)

## 1. Problem

`pkg/script/handlers_player.go::handleLastLoginInfo` (opcode 2054)
delegates to `s.Self.LastLoginInfo()`. The concrete entity body at
`modules/world/player_script.go:1331-1333` is an intentional no-op
pending the `LAST_LOGIN_INFO` server-prot opcode. TS
`Engine-TS/src/engine/entity/Player.ts:2190-2200`:

```ts
lastLoginInfo() {
    const lastDate: bigint = this.lastLoginTime === 0n ? BigInt(Date.now()) : this.lastLoginTime;
    const nextDate: bigint = BigInt(Date.now());

    const lastIp = 2130706433; // 127.0.0.1
    const daysSinceLogin: number = (Number(nextDate - lastDate) / (1000 * 60 * 60 * 24)) | 0;
    const daysSinceRecoveriesChanged = 201; // hide :)

    this.write(new LastLoginInfo(lastIp, daysSinceLogin, daysSinceRecoveriesChanged, this.messageCount));
    this.lastLoginTime = nextDate;
}
```

Encoder (`LastLoginInfoEncoder.ts:9-14`):

```ts
buf.p4(message.lastLoginIp);
buf.p2(message.daysSinceLogin);
buf.p1(message.daysSinceRecoveryChange);
buf.p2(message.unreadMessageCount);
```

Server prot identity: `ServerGameProt.LAST_LOGIN_INFO = new ServerGameProt(140, 9)`.

## 2. Affected sites (pre-flight verified at HEAD `2cb8a09`)

- `pkg/io/protocol/game/server/prot.go` — no `OpLastLoginInfo` entry; opcode 140 unused (verified via `grep -n "140" prot.go` returns zero hits for an `OpCode:` use).
- `modules/world/player.go:239` — `messageCount int` exists at `// === chat state ===`. No `lastLoginTime` field.
- `modules/world/player_script.go:1331-1333` — `LastLoginInfo()` no-op body.
- `pkg/script/handlers_player.go:1837` — interface-side citation.
- `pkg/script/active.go:763` — interface doc-comment citation.
- `modules/world/player_script_test.go:1686` — test-side citation pinning the no-op contract.

`writeOut` (`player.go:452`) requires `p.client != nil`; production
handler `handleLastLoginInfo` gates on `requireActivePlayer` which only
fires for connected players, so nil-client guard inside the method is
not needed (matches `HintNpc` / `HintCoord` / `HintPlayer` / `HintStop`
sibling pattern at `player_script.go:307-364`).

## 3. Architecture

### 3.1 ServerProt entry

Append to `pkg/io/protocol/game/server/prot.go` in the same group as
`OpEnableTracking` / `OpFinishTracking` (NAI-73-adjacent server signals;
LAST_LOGIN_INFO is also a client-info signal):

```go
// OpLastLoginInfo carries previous-login telemetry the client renders
// on the welcome screen: last-login IP (always 127.0.0.1 / 2130706433
// per TS Player.ts:2194), days since previous login, days since
// recovery-questions changed (always 201, hidden), and the unread
// message count. Fixed 9-byte payload: p4(lastIp), p2(daysSinceLogin),
// p1(daysSinceRecoveriesChanged), p2(messageCount). Mirrors TS
// ServerGameProt.LAST_LOGIN_INFO (140, 9) and LastLoginInfoEncoder.ts.
OpLastLoginInfo = Op{Opcode: 140, PayloadSize: 9}
```

### 3.2 `Player.lastLoginTime` field

Add to `modules/world/player.go` adjacent to `messageCount`:

```go
// lastLoginTime is the unix-ms timestamp captured at the most recent
// LAST_LOGIN_INFO emission. Zero on a fresh login — see (*Player).
// LastLoginInfo for the lastDate==0 first-call branch. Mirrors TS
// Player.lastLoginTime (Player.ts:2191, 2199).
lastLoginTime int64
```

Zero value matches TS `0n` sentinel; no constructor change needed.

### 3.3 `(*Player).LastLoginInfo` body

Replace the no-op at `modules/world/player_script.go:1331-1333` with:

```go
// LastLoginInfo emits a LAST_LOGIN_INFO server packet with the
// previous-login timestamp and IP. Mirrors TS Player.lastLoginInfo
// (Player.ts:2190-2200).
//
// First call (lastLoginTime==0): daysSinceLogin computes to 0 because
// lastDate falls back to now. Subsequent calls compute integer days
// between previous lastLoginTime and now. After writing, lastLoginTime
// advances to now.
//
// lastIp is hardcoded to 127.0.0.1 (2130706433) and
// daysSinceRecoveriesChanged to 201 ("hide :)") per TS Player.ts:2194,2196.
func (p *Player) LastLoginInfo() {
    now := time.Now().UnixMilli()
    lastDate := p.lastLoginTime
    if lastDate == 0 {
        lastDate = now
    }
    const lastIp = 2130706433 // 127.0.0.1
    const dayMillis = int64(1000 * 60 * 60 * 24)
    daysSinceLogin := int((now - lastDate) / dayMillis)
    const daysSinceRecoveriesChanged = 201

    payload := []byte{
        byte(lastIp >> 24), byte(lastIp >> 16), byte(lastIp >> 8), byte(lastIp), // p4: lastIp
        byte(daysSinceLogin >> 8), byte(daysSinceLogin),                         // p2: daysSinceLogin
        byte(daysSinceRecoveriesChanged),                                        // p1
        byte(p.messageCount >> 8), byte(p.messageCount),                         // p2: messageCount
    }
    p.writeOut(gameserver.OpLastLoginInfo, payload)
    p.lastLoginTime = now
}
```

`time` import must be added to `modules/world/player_script.go`'s
`import ( ... )` block — verified absent at HEAD `2cb8a09`. (`player.go`
already imports `"time"` for the `mutedUntil time.Time` field at
line 238; that's a different file.)

### 3.4 Memorial-style citation updates

The four in-code references to `NAI-162-D-LASTLOGIN-NO-PACKET` get
retirement notes (memorial style, mirroring NAI-19's
`NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE` convention and
NAI-180's NAI-35-T3-D1 retirement):

- `pkg/script/active.go:761-764` — replace "Current impl is a no-op pending ServerProt port (NAI-162-D-LASTLOGIN-NO-PACKET)." with "Emits LAST_LOGIN_INFO server packet (NAI-181 retires NAI-162-D-LASTLOGIN-NO-PACKET)."
- `pkg/script/handlers_player.go:1836-1838` — strip the "NAI-162-D-LASTLOGIN-NO-PACKET: underlying ... no-op pending ServerProt port." paragraph; the handler itself is unchanged.
- `modules/world/player_script.go:1323-1333` — rewrite doc-comment per §3.3 above.
- `modules/world/player_script_test.go:1686` — retire the "pins method exists and is a no-op" pin; replace with the new byte-pin test (§4 below).

## 4. Test plan

Four tests at `modules/world/player_script_test.go`, following the
`TestHintNpcPayloadBytes` template at line 1131:

### 4.1 `TestLastLoginInfo_FirstCall_EmitsExactByteSequence`

```go
func TestLastLoginInfo_FirstCall_EmitsExactByteSequence(t *testing.T) {
    p, cc := newTestPlayer(t)
    enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    p.messageCount = 0x0007
    p.lastLoginTime = 0 // first-call branch

    want := []byte{
        byte((int(gameserver.OpLastLoginInfo.Opcode) + int(enc.GetNext())) & 0xff),
        0x7F, 0x00, 0x00, 0x01, // p4: lastIp = 2130706433 (127.0.0.1)
        0x00, 0x00,             // p2: daysSinceLogin = 0 (first-call branch)
        0xC9,                   // p1: daysSinceRecoveriesChanged = 201
        0x00, 0x07,             // p2: messageCount = 7
    }
    received := drainConn(t, cc)
    p.LastLoginInfo()
    p.client.flushWrite()
    got := <-received
    if !bytes.Equal(got, want) {
        t.Errorf("LastLoginInfo first-call wire: got %#x, want %#x", got, want)
    }
}
```

Pins all 4 encoder fields' positions + ordering + endianness.

### 4.2 `TestLastLoginInfo_SubsequentCall_DaysSinceLoginAdvances`

Set `p.lastLoginTime` to `time.Now().UnixMilli() - 5*dayMillis - 100`
(5 days + buffer). Call `LastLoginInfo()`. Decrypt the opcode prefix and
parse bytes [5:7] as big-endian uint16. Assert `daysSinceLogin >= 5`
(tolerant for test runtime); upper bound `<= 6` (covers slow CI).

This pins the integer-truncation formula and `lastDate` non-zero branch
without introducing a `time.Now()` injection point.

### 4.3 `TestLastLoginInfo_UpdatesLastLoginTime`

Capture `before := time.Now().UnixMilli()`. Call `LastLoginInfo()`.
Capture `after := time.Now().UnixMilli()`. Assert
`before <= p.lastLoginTime && p.lastLoginTime <= after`.

Pins the `this.lastLoginTime = nextDate` write at TS Player.ts:2199.

### 4.4 `TestLastLoginInfo_MessageCountSerialization`

Set `p.messageCount = 0xABCD`. Run the §4.1 byte-pin against bytes [8:10]
expecting `{0xAB, 0xCD}` (big-endian p2). Disambiguates messageCount
slot from daysSinceRecoveriesChanged endianness regression.

## 5. Build sequence

Single TDD bundle (compressed cadence — ≤~50 production LOC + ~120
test LOC).

1. **T1 (RED)** — add 4 tests at `modules/world/player_script_test.go`
   referencing `gameserver.OpLastLoginInfo` (compile error). Tests fail.
2. **T2 (GREEN)** —
   - Add `OpLastLoginInfo` entry to `pkg/io/protocol/game/server/prot.go`.
   - Add `lastLoginTime int64` field to `Player` (player.go).
   - Replace `LastLoginInfo()` body at `player_script.go:1331-1333`.
   - Update memorial citations at `active.go:761-764`,
     `handlers_player.go:1836-1838`, `player_script.go:1323-1333`.
   - Retire the existing no-op pin at `player_script_test.go:1686-1690`.
3. **T3 (CLOSE)** — close commit with `Closes memory:` trailer and
   `nai_followups.md` NAI-181 section.

## 6. Risk register

- **R1: `time.Now()` non-determinism in test §4.2.** Tolerance window
  (`>=5 && <=6`) absorbs realistic test-runtime jitter. Failure case
  would require the test to take >86 400 000 ms (>1 day) — not
  possible. Risk: ZERO.
- **R2: `messageCount > 0xFFFF` overflow.** Existing field is `int`;
  no production producer caps it. TS truncates the same way (p2 wraps
  on `Number` → 16-bit byte cast). Behavior matches. Risk: ACCEPTED
  (mirrors TS).
- **R3: clock skew causes negative `daysSinceLogin`.** TS truncates
  negative `Number` toward zero (`| 0`); Go `int64` division
  truncates toward zero too. Encoded as 2-byte two's-complement on
  the wire (Java client reads as signed `short`). Behavior matches.
  Risk: ACCEPTED (mirrors TS).
- **R4: persistence — does goscape carry `lastLoginTime` across
  reconnects?** TS persists via Player save. Goscape's player save
  layer (`pkg/save/`) is out of scope here; `lastLoginTime` resets to
  0 on each fresh `*Player` allocation. First-call branch covers
  every fresh login. Future save-format port can pick up the field.
  Risk: ACCEPTED (deferred, matches existing save-coverage scope).

## 7. Deviations introduced

None. The port is verbatim-TS for the engine-side surface. The
persistence gap (R4) is a save-layer scope concern, not a behavioral
divergence at this packet's emission site.

## 8. Net deviation tally

`NAI-162-D-LASTLOGIN-NO-PACKET` retires: tally -1.

## 9. Memory patterns applied

- `compressed_cadence.md` — combined spec+plan, single TDD bundle.
- `controller_preflight.md` — all six affected sites grep-verified
  at HEAD `2cb8a09` (§2).
- `verify_implementer_claims.md` — at impl-close, fresh `go test
  ./modules/world/... -count=1 -run LastLoginInfo` + repo-wide
  `go test ./...` will ground-truth.
- `retire_deviation_grep_all_comments.md` — §3.4 enumerates all 4
  citation sites and prescribes the memorial-style retirement.
- `close_commit_memory_trailer.md` — close commit carries
  `Closes memory: NAI-162-D-LASTLOGIN-NO-PACKET`.
- `rsbuf_roundtrip_tests.md` — §4.1's distinguishable nonzero
  messageCount catches endianness regressions at the byte-pin.
