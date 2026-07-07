# ISAAC

ISAAC is the stream cipher goscape uses to encipher **game-packet opcodes** once
a client is logged in. This page documents what it protects, how it is seeded
from the login block, the encryptor/decryptor offset, and the cipher mechanics —
all from `pkg/io/isaac/isaac.go` and the opcode read/write paths in
`modules/world/player.go` at wire revision {{ revision }}.

## What ISAAC protects

Only the **opcode byte** of each game packet is enciphered — not the length
prefix, not the payload. After login:

- **Write path** (`player.go`, `writeOut`): the outgoing opcode is
  `(opcode + encryptor.GetNext()) & 0xff`. The length prefix (0/1/2 bytes per
  the [framing](framing.md) model) and payload are written in the clear.
- **Read path** (`player.go`, `readPacket`): the incoming opcode is recovered as
  `(rawByte - decryptor.GetNext()) & 0xff`, then looked up in the opcode table.

Each direction consumes one 32-bit ISAAC output per opcode, truncated to a byte.
Because both ends advance their streams in lockstep, opcode *N* on one side is
deciphered against the same keystream word on the other.

This cipher is armed during the login handshake — see
[login](login.md#step-3-arming-isaac) for where in the sequence it happens and
the [architecture guide](../contributor/architecture.md#connection-lifecycle-and-isaac-placement)
for its place in the connection lifecycle.

## Seeding from the login block

The RSA-encrypted portion of the login block carries **four 32-bit seeds**
(`ISAACSeed[0..3]`; see the [login RSA-block table](login.md#rsa-encrypted-block)).
`isaac.New(seed [4]uint32)` copies those four words into the first four slots of
its 256-word result array and zero-fills the rest, then runs the key schedule
(`init`). Only 4 of the 256 words are seeded from the client — the rest of the
entropy comes from the mixing schedule, matching the client.

### The encryptor/decryptor offset

goscape keys **two** independent ISAAC instances per connection from the same
four seeds, with a fixed offset (`server_login.go`):

| Stream | Seeded from | Used for |
|---|---|---|
| `decryptor` | the four seeds **as received** | deciphering opcodes read from the client |
| `encryptor` | the four seeds **each + 50** | enciphering opcodes sent to the client |

Concretely, the server seeds the decryptor first, then adds `50` to every one of
the four seed words and seeds the encryptor from the adjusted array. The Java
client performs the mirror-image construction (its outbound cipher is seeded
from the unmodified seeds, its inbound cipher from seeds + 50), so the server's
decryptor tracks the client's encryptor and vice-versa.

## Cipher mechanics

`pkg/io/isaac/isaac.go` is a straightforward 256-word ISAAC:

- **State.** A 256-word result array (`rsl`), a 256-word memory array (`mem`),
  three accumulators `a`, `b`, `c`, and a countdown `count`.
- **`New(seed)`** copies the four seed words into `rsl[0..3]` and calls `init`.
- **Key schedule (`init`).** Eight local accumulators start at the golden ratio
  constant `0x9E3779B9` and are scrambled through four mixing rounds; the seed
  array is then folded into `mem` twice (accumulate `rsl`/`mem`, then re-mix);
  finally one generation round (`isaac`) runs and `count` is set to 256.
- **Generation (`isaac`).** Increments `c`, adds it into `b`, then for each of
  the 256 words: applies the ISAAC barrel-shift pattern to `a` — left 13, right
  6, left 2, right 16, selected by `i & 3` — and adds `mem[(i+128) & 0xff]`
  into `a`; computes the new word as `y = mem[(x>>2) & 0xff] + a + b` (where
  `x` is the old `mem[i]`) and stores it to `mem[i]`; then sets
  `b = mem[(y>>10) & 0xff] + x` and writes `b` to `rsl[i]`.
- **`GetNext()`** returns result words in reverse order (`count` counts down from
  255); when the array is exhausted it calls `isaac()` to refill and resets
  `count` to 255.

### Test vectors

The pinned vectors in `isaac_test.go` fix the implementation. After constructing
a stream and drawing `GetNext()` 1,000,001 times:

| Seed | 1,000,001st `GetNext()` |
|---|---|
| `{0, 0, 0, 0}` | `1536048213` |
| `{1, 2, 3, 4}` | `-107094133` (as `int32`; i.e. `& 0xFFFFFFFF`) |
