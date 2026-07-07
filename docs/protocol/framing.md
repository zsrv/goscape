# Packet framing

This page describes how a goscape packet is delimited on the wire: the opcode +
size model, how a reader knows when a whole packet has arrived, and the
byte-order / string conventions the accessors use. It documents wire revision
{{ revision }} as implemented in `pkg/io/protocol` and `pkg/io/packet`.

The server-side accessor naming (`G` = get/read, `P` = put/write, the digit is
the byte width, bit-level access via `AccessBits`/`AccessBytes`) is covered in
the [contributor architecture guide](../contributor/architecture.md#packet-buffer-conventions-pkgiopacket)
— this page only states the wire-level facts those methods produce.

## The `Operation` model

Every packet type is described by an `Operation`
(`pkg/io/protocol/protocol.go`):

```go
type Operation struct {
    Name        string
    PayloadSize int
    Opcode      uint8
}
```

- **`Opcode`** — the single unsigned byte that identifies the packet. For game
  packets this byte is ISAAC-enciphered (see [ISAAC](isaac.md)); during the
  login handshake it is plaintext.
- **`PayloadSize`** — the shape of the payload that follows the opcode:

| `PayloadSize` | Meaning | On the wire after the opcode |
|---|---|---|
| `≥ 0` | Fixed-length payload of exactly that many bytes | *N* payload bytes |
| `-1` | Dynamic, **1-byte** length prefix | 1 length byte, then that many payload bytes |
| `-2` | Dynamic, **2-byte** length prefix (big-endian) | 2 length bytes, then that many payload bytes |

So a `-1` packet frames as `[opcode][len:1][payload…]` and a `-2` packet as
`[opcode][len:2 BE][payload…]`. A fixed packet is just `[opcode][payload…]` with
the length known from the `Operation`.

The write side mirrors this exactly: when a game frame is emitted, the opcode is
written first, then for `PayloadSize == -1` a single length byte, for
`PayloadSize == -2` a two-byte big-endian length, then the payload
(`modules/world/player.go`, `writeOut`).

## Partial reads: `CheckPacketLength`

TCP delivers a byte stream, not framed messages, so a reader may hold only part
of a packet. `CheckPacketLength(p, o)` (`protocol.go`) reports whether the
buffer `p` already holds a complete packet for operation `o`, returning
`(size, complete)`:

1. **Header not yet buffered.** For a `-2` packet with fewer than 3 bytes
   buffered (opcode + 2 length bytes), or a `-1` packet with fewer than 2 bytes
   (opcode + 1 length byte), it returns `(p.Len(), false)` — the declared size
   cannot be computed until the length-prefix bytes themselves arrive, so the
   first value is "bytes available", not "bytes needed".
2. **Size known but payload incomplete.** Once the length prefix is readable
   (or the size is static), it computes `packetSize = headerSize + payloadSize`
   and returns `(packetSize, false)` if fewer than `packetSize` bytes are
   buffered.
3. **Complete.** When at least `packetSize` bytes are buffered it returns
   `(packetSize, true)`.

!!! warning "The int is only meaningful when the bool is true"
    The first return value does **not** carry one uniform meaning across the
    `false` paths (available-bytes on the "no header yet" path, needed-bytes
    otherwise). Callers branch on the bool; only when it is `true` is the int
    guaranteed to equal the packet's full header+payload size. This is spelled
    out in the function's own doc comment.

The length prefix is read big-endian: for `-2`, `payloadSize =
tl[1]<<8 | tl[2]` (skipping the opcode at `tl[0]`); for `-1`, `payloadSize =
tl[1]`.

### Framing errors

`protocol.go` defines the sentinel errors a framing/length check can raise:

| Error | Raised when |
|---|---|
| `ErrIncorrectOpcode` | The opcode byte is not one the handler expects. |
| `ErrIncorrectDataLength` | Too few bytes are present to even read the header. |
| `ErrPayloadTooSmall` | The buffered payload is shorter than the declared size. |
| `ErrPayloadTooLarge` | The buffered payload is longer than the declared size. |

## Byte-order and accessor conventions

The RS2 accessors in `pkg/io/packet/packet.go` fix the wire encoding of each
scalar. The ones that appear in the protocol pages:

| Method | Reads/writes | Encoding |
|---|---|---|
| `G1` / `P1` | 1 byte, unsigned | — |
| `G2` / `P2` | 2 bytes, unsigned | **big-endian** (`data[0]<<8 \| data[1]`) |
| `IG2` / `IP2` | 2 bytes, unsigned | little-endian |
| `G3` / `P3` | 3 bytes, unsigned | big-endian |
| `G4` / `P4` | 4 bytes, unsigned | **big-endian** |
| `IG4` / `IP4` | 4 bytes, unsigned | little-endian |
| `G8` / `P8` | 8 bytes, unsigned | big-endian (two big-endian `G4`s, high word first) |
| `GBool` / `PBool` | 1 byte | `true` ⇔ byte `1`; write emits `1`/`0` |

`GSmart`/`PSmart` (1-or-2-byte "smart" ints) and `GVarInt`/`PVarInt`
(base-128 varints) exist for game-body fields but do not appear in the login
handshake; their ranges are documented in the accessor source.

### Strings

Strings on the RS2 wire are **byte-per-character, terminator-delimited** — there
is no length prefix. The terminator is baked into the method name:

| Method | Terminator byte |
|---|---|
| `GJStrLF` / `PJStrLF` | `0x0A` (newline, `\n`) |
| `GJStrNUL` / `PJStrNUL` | `0x00` (NUL) |
| `GJStr(t)` / `PJStr(s, t)` | caller-supplied byte `t` |

The login username and password use `GJStrLF` / `PJStrLF`, so they are
**newline-terminated** on the wire — the reader consumes bytes up to and
including the `0x0A`, and the writer appends `0x0A` after the characters
(`packet.go`, `PJStr`/`GJStr`). Each character is written as one byte
(`rune & 0xff`), matching the client's per-UTF-16-code-unit encoding rather than
Go's multi-byte UTF-8.

!!! note "Missing-terminator edge case"
    `GJStr` mirrors a TS quirk: if the terminator is never found before the
    buffer ends, the final byte is consumed but dropped from the returned
    string. This is a faithful port, not a goscape bug — see the `GJStr` doc
    comment in `packet.go`.
