# Protocol

These pages document the **RS2 wire protocol as goscape actually implements
it** — the byte layouts, opcodes, and constants a client developer or a
protocol debugger needs to talk to (or trace) a goscape world. Every fact here
was read out of the Go marshal/unmarshal code, not from RS2 community lore: if
a field is documented, it is because the code reads or writes it in that order,
at that size.

!!! info "Revision scope"
    This build documents wire revision **{{ revision }}**. Each goscape branch
    implements exactly one revision (`pkg/io/protocol/revision`), and the world
    server rejects any client whose revision byte does not match with login
    reply `6` ("RuneScape has been updated!"). Where a field's shape is known to
    differ on another revision, that is called out inline.

## What is on these pages

| Page | Covers |
|---|---|
| [Packet framing](framing.md) | The opcode + size model (`Operation`), fixed vs. length-prefixed packets, partial-read handling, and the byte-order / string conventions of the RS2 accessors. |
| [Login handshake](login.md) | The full connect → session-key → RSA login-block → response-code sequence, field-by-field, plus how the ISAAC ciphers are armed. |
| [ISAAC](isaac.md) | The stream cipher that enciphers game opcodes after login: seeding, the encryptor/decryptor offset, and mechanics. |
| [OnDemand](ondemand.md) | The HTTP cache surface (archive routes, `/crc`, `/rs2.cgi`) and the TCP OnDemand request channel, plus the on-disk cache layout they serve. |

## Audience and altitude

The audience is someone implementing or debugging a client, a proxy, or a
packet capture — not someone modifying engine internals. For the server-side
picture (how packets are read/written through `packet.Packet`, the
`G`/`P` accessor naming, bit-level access, the connection state machine, and
where the ISAAC ciphers sit in the connection lifecycle), read the
[contributor architecture guide](../contributor/architecture.md); these pages
link to it rather than repeat the buffer internals.

## Reading the field tables

- **Endianness is big-endian unless a field says otherwise.** `G2`/`G4` read
  most-significant byte first; the little-endian variants (`IG2`/`IG4`) are
  named explicitly where used.
- **Strings are newline-terminated** in the login protocol (`GJStrLF`), i.e.
  the terminator byte is `0x0A` — see [framing](framing.md#strings).
- **Opcodes and sizes are `Operation` constants.** A `PayloadSize` of `-1` or
  `-2` means the packet carries a 1- or 2-byte length prefix; a non-negative
  value is a fixed payload length. See [framing](framing.md#the-operation-model).

## A note on cross-revision differences

The wire format is stable within a revision but shifts between them. One
concrete example, recorded in the docs-site
[`REFERENCES.md`](https://github.com/zsrv/goscape) rev-254 notes: **NPC ids
widen to 14 bits at rev-254** (the info-block terminator moves `8191 → 16383`
and NPC capacity `8192 → 16384`) as a consequence of the `@2004scape/rsbuf`
`254.1.0` bump. This build is rev-{{ revision }}; treat any width or opcode you
read here as revision-specific and re-verify against the code for the revision
you are targeting.
