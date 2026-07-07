# Login handshake

This page documents the connect-to-in-game handshake for wire revision
{{ revision }}, exactly as goscape implements it in
`modules/world/server_login.go`, `modules/world/client.go`, and
`pkg/io/protocol/login/`. Every field below is taken from the marshal/unmarshal
code; the RSA login-block table is in the precise order the bytes appear.

The world server is a raw TCP listener. A fresh connection starts in the
**login** state and dispatches on the first byte (the opcode):

| Opcode | Meaning | Where documented |
|---|---|---|
| `14` | Check-login / server session-key exchange | Below |
| `15` | OnDemand cache-connection entry | [OnDemand](ondemand.md) |
| `16` | New game connection (`OpReqInitGameConnection`) | Below |
| `18` | Game reconnect (`OpReqGameReconnect`) | Below |

Opcodes `16` and `18` carry the same login-block layout; they differ only in
whether the server treats the session as a reconnect.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant C as Client
    participant W as World server (TCP)
    participant L as Login service (gRPC)

    C->>W: op 14 (checklogin): opcode + 1 discard byte
    W->>C: 8 zero bytes
    Note over W: per-address rate-limit check
    W->>C: 0x00 + 8-byte server session seed
    Note over C,W: 17 bytes total on the happy path

    C->>W: op 16/18 login block<br/>(cleartext header + RSA block)
    Note over W: validate revision, then archive CRCs<br/>(cleartext — before any RSA work)
    W->>W: RSA-decrypt block → 4 ISAAC seeds, uid, user, pass
    W->>W: arm ISAAC: decryptor = seeds,<br/>encryptor = seeds + 50
    W->>L: PlayerLogin(username, password, uid, …)
    L->>W: LoginResult (+ save, staff level, members)

    alt accepted (OK / reconnect OK)
        W->>C: [2, min(staffModLevel,2), 1]
        Note over C,W: connection enters game state;<br/>opcodes now ISAAC-enciphered
    else rejected
        W->>C: single reject byte (or [21, seconds])
        W--xC: close connection
    end
```

## Step 1 — op 14: server session-key exchange

The client opens with opcode `14` followed by one payload byte (a
`_loginServer` discriminator that the server reads and discards). Total input:
**2 bytes**.

The server replies in three writes (`server_login.go`, case `14`):

| Order | Bytes | Value |
|---|---|---|
| 1 | 8 | all zero (`P4(0)` twice) |
| 2 | 1 | `0x00` separator |
| 3 | 4 | first seed word — `rand & 0x00ffffff` (high byte always `0x00`) |
| 4 | 4 | second seed word — full 32-bit random |

That is **17 bytes** on the happy path. The 8 zero bytes are sent *before* the
per-address login rate-limit check, so even a rate-limited client receives them,
followed by a single reject byte `16` ("Login attempts exceeded") and a close.

!!! note "The server seed is not stored"
    goscape generates this 8-byte seed fresh per op-14 call and does **not**
    retain it. The ISAAC seeds that actually key the session are the ones the
    client echoes back inside the RSA block at op 16/18 — see step 3.

## Step 2 — op 16/18: the login block

The login packet is a `-1` (1-byte length prefix) operation. It has a
**cleartext header** followed by a **1-byte-length-prefixed RSA-encrypted
block**. The server decodes and validates the cleartext header first
(`UnmarshalHeader`), and only if revision and archive CRCs check out does it
spend RSA CPU decrypting the tail (`UnmarshalRSA`).

### Cleartext header

In wire order (`req.go`, `MarshalBinary` / `UnmarshalHeader`):

| # | Field | Size | Accessor | Notes |
|---|---|---|---|---|
| 1 | Opcode | 1 | `P1`/`G1` | `16` (new) or `18` (reconnect) |
| 2 | Payload size | 1 | `PSize1`/`G1` | length of everything after this byte |
| 3 | Revision | 1 **or** 3 | `G1`, `0xff` → `G2` | see below |
| 4 | Low-memory flag | 1 | `PBool` / `G1 & 0x1` | server takes the **low bit only** |
| 5 | Archive checksums `[0..8]` | 36 | `P4`/`G4` ×9 | nine big-endian CRC-32 values |

**Revision encoding (rev-{{ revision }}).** Revision `274` no longer fits one
byte. The client writes `0xff` as an escape marker followed by a 2-byte
big-endian revision (`P1(0xff)` + `P2(274)`, i.e. `FF 01 12`); the server reads
`G1`, and if it sees `0xff` reads a further `G2`. Revisions below `0xff` use the
historical single byte. A revision that does not equal the compiled expected
revision (`274`) is rejected with reply `6` ("RuneScape has been updated!").

**Archive CRC check.** The nine `G4` checksums are compared against the server's
own archive CRC table (`slices.Equal`). A mismatch is rejected with reply `6`.
An empty/unbuilt cache yields an empty table, so *every* login is rejected as
out-of-date until a real cache exists.

### RSA-encrypted block

After the header comes a 1-byte length prefix giving the RSA ciphertext length,
then the ciphertext itself. Decrypting it (`RSADec`, with the world's private
key) yields this plaintext, in order (`req.go`, `MarshalBinary` /
`UnmarshalRSA`):

| # | Field | Size | Accessor | Notes |
|---|---|---|---|---|
| 1 | RSA magic | 1 | `P1(10)` / `G1` | must equal `10`, else the block is rejected |
| 2 | ISAAC seed `[0..3]` | 16 | `P4`/`G4` ×4 | the four session seeds (big-endian) |
| 3 | UID | 4 | `P4`/`G4` | client machine/session UID |
| 4 | Username | *n+1* | `PJStrLF`/`GJStrLF` | newline-terminated (`0x0A`) |
| 5 | Password | *m+1* | `PJStrLF`/`GJStrLF` | newline-terminated (`0x0A`) |

The default build ships a 512-bit RSA key (`pkg/io/protocol/rsakey.go`); a world
can override the private key via `world.rsa_private_key_path`. The matching
public key is baked into the Java client.

After decryption the server validates the username length (1–12) and password
length (1–20), rejecting out-of-range values with reply `3` ("Invalid username
or password").

## Step 3 — arming ISAAC

Immediately after the RSA block decrypts, the server keys its two ISAAC streams
from the four seeds (`server_login.go`):

- **decryptor** ← the seeds as received.
- **encryptor** ← the same seeds with **`+50` added to each of the four
  words**.

The client does the mirror image (its out-cipher = the server's in-cipher), so
the two ends stay in lockstep. From this point every game opcode is
ISAAC-enciphered. See [ISAAC](isaac.md) for the seeding and cipher mechanics.

## Step 4 — login result and response byte

The world calls the login service's `PlayerLogin` RPC and maps the result to an
RS2 response byte (`loginResultToRS2`). On an accepting result the connection
enters the game state; on any other it sends the reject byte and closes.

### Accepted reply

goscape sends a **3-byte** accepted reply for both new logins and reconnects
(`client.go`, `sendLoginOK`):

```
[ 2, min(staffModLevel, 2), 1 ]
```

- byte 0 — response code `2` (OK).
- byte 1 — staff/mod tier, clamped to `2`.
- byte 2 — constant `1` (mouse-tracking-enabled flag; only settable at login).

!!! note "goscape never emits wire byte 15 (reconnect OK)"
    A reconnect result is mapped internally to `RECONNECT_OK`, but the accepted
    path always writes opcode `2`. Byte `15` is never put on the wire. Likewise,
    replies `18`/`19` (the pre-254 staff-tier login opcodes) were removed at
    revision 254 — the staff tier now rides in byte 1 of the opcode-`2` reply.

### Response codes

Every login response opcode goscape can send (`pkg/io/protocol/login/resp/`):

| Opcode | Name | Client-facing meaning | Payload |
|---|---|---|---|
| `1` | TryAgain | Client waits ~2 s and retries | 0 |
| `2` | OK | Login accepted (followed by 2 more bytes, above) | 0 (+2 on accept) |
| `3` | INVALID_USERNAME_OR_PASSWORD | Invalid username or password | 0 |
| `4` | BANNED | Account disabled | 0 |
| `5` | DUPLICATE | Already logged in; retry in ~60 s | 0 |
| `6` | CLIENT_OUT_OF_DATE | "RuneScape has been updated!" (revision/CRC mismatch) | 0 |
| `7` | SERVER_FULL | World is full | 0 |
| `8` | LOGINSERVER_OFFLINE | Login server unreachable / world not yet registered | 0 |
| `9` | IP_LIMIT | Too many connections from this address | 0 |
| `10` | BadSessionID | Bad session id | 0 |
| `11` | LoginServerRejected | Login server rejected session (e.g. corrupt save) | 0 |
| `12` | NEED_MEMBERS_ACCOUNT | Members-only world | 0 |
| `13` | INVALID_SAVE | Could not complete login | 0 |
| `14` | UPDATE_IN_PROGRESS | Server updating; sent during the pre-shutdown window | 0 |
| `15` | RECONNECT_OK | (internal only — not emitted, see note above) | 0 |
| `16` | TOO_MANY_ATTEMPTS | Login attempts exceeded (rate limit) | 0 |
| `17` | MembersOnlyArea | Standing in a members-only area | 0 |
| `21` | HOP_TIMER | World-hop cooldown; the only reject with a payload | **1** |

**Hop timer (`21`).** This is the sole reject that carries a payload: a single
byte of `min(255, remainingMs / 1000)` seconds (`client.go`,
`sendLoginHopTimer`). The client counts it down on the title screen before
auto-retrying.

All non-accepting replies are followed by a connection close.
