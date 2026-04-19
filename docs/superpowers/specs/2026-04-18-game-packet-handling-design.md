# Game Packet Handling Design

**Date:** 2026-04-18
**Scope:** Complete login handshake + implement in-game packet read loop with opcode table dispatch and concrete handlers for movement/keepalive packets.

## Context

The Go world server (`modules/world/`) currently handles login (state 0) via `handleLogin()` in `server.go`. Login validation is complete but the function never sends the login OK byte or transitions to game state. No game-state packet handling exists. This design ports the TypeScript `NetworkPlayer.read()` loop and the `ClientGameProt` opcode table.

## Files

### New

| File | Purpose |
|---|---|
| `pkg/io/protocol/game/client/prot.go` | `[256]Op` lookup table — all ~50 ClientGameProt opcodes from TS |
| `modules/world/client_game.go` | `(*client).handleGame()` — ISAAC decrypt, opcode dispatch |
| `modules/world/handlers_game.go` | `gameHandlers [256]func(*client, []byte) error` registry + concrete handlers |

### Modified

| File | Change |
|---|---|
| `modules/world/client.go` | Add `ClientStateGame ClientState = 1` |
| `modules/world/server.go` | Complete `handleLogin()` post-login path; add `ClientStateGame` case to `handleData()` |
| `modules/world/server_test.go` | New integration tests for login completion and game packet handling |

## Data Flow

### Login completion (`server.go` — `handleLogin()`)

After a successful gRPC `PlayerLogin` response:
1. Send `byte(18)` if `staffModLevel >= 1`, otherwise `byte(2)`
2. Call `c.flushWrite()`
3. Set `c.state = ClientStateGame`

### In-game packet read loop (`client_game.go` — `handleGame()`)

`handleGame()` drains the incoming buffer in a loop:

```
loop:
  peek 1 byte → ISAAC-decrypt opcode byte
  look up gameclient.Ops[opcode]:
    if Name == "": log warn + return errCloseConn  (unknown opcode)
  CheckPacketLength(c.in, Op) → (pLen, ok):
    if !ok: return ErrPayloadTooSmall               (wait for more data)
  consume pLen bytes from c.in
  call gameHandlers[opcode](c, payload[headerSize:])
  repeat
```

`handleData()` gains a `ClientStateGame` case that calls `handleGame()`.

### Handler dispatch (`handlers_game.go`)

`init()` populates `gameHandlers[opcode]` for each known opcode. A nil slot is never reached (unknown opcodes are rejected before dispatch). Handlers receive the raw payload as `[]byte` and parse it with `packet.NewPacket(payload)`.

## Opcode Table (`pkg/io/protocol/game/client/prot.go`)

Mirrors `ClientGameProt.byId[]` from the TS. Each entry is:

```go
type Op struct {
    Name        string
    PayloadSize int  // 0=fixed-zero, N=fixed-N, -1=1-byte-len, -2=2-byte-len
}

var Ops [256]Op
```

All ~50 opcodes from `ClientGameProt.ts` are populated (zero-value `Op{}` means unknown).

## Concrete Handlers

| Packet | Opcode | PayloadSize | Handler behavior |
|---|---|---|---|
| `NO_TIMEOUT` | 108 | 0 | no-op (discard) |
| `IDLE_TIMER` | 70 | 0 | no-op (discard) |
| `MOVE_GAMECLICK` | 181 | -1 | parse coords, log Info |
| `MOVE_OPCLICK` | 93 | -1 | parse coords, log Info |
| `MOVE_MINIMAPCLICK` | 165 | -1 | parse coords, log Info |

Movement payload format (from `MoveClickDecoder.ts`):
- 1 byte: `ctrlHeld`
- 2 bytes: `startX` (G2)
- 2 bytes: `startZ` (G2)
- Remaining pairs: signed-byte delta X + signed-byte delta Z (up to 24 waypoints)
- `MOVE_MINIMAPCLICK` has 14 trailing bytes to skip before counting waypoints

All other opcodes: nil handler slot is never reached (unknown opcode closes connection before dispatch).

## Error Handling

| Condition | Action |
|---|---|
| Unknown ISAAC-decrypted opcode | log Warn + `errCloseConn` |
| `ErrPayloadTooSmall` | return to caller, wait for more TCP data |
| Handler returns error | log Error + close connection |
| `c.decryptor == nil` in game state | log Error + close connection |

## Testing

Extend `modules/world/server_test.go`:

1. **Login completion test:** complete a full login handshake; assert server sends `byte(2)` and connection remains open
2. **NO_TIMEOUT test:** after login, send ISAAC-encrypted opcode 108 (zero payload); assert connection stays alive
3. **MOVE_GAMECLICK test:** after login, send a valid `MOVE_GAMECLICK` packet; assert no error and parsed coords are logged
