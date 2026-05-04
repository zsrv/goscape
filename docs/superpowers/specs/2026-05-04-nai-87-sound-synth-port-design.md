# NAI-87 — SOUND_SYNTH port

**Status:** spec
**Date:** 2026-05-04
**Predecessor:** NAI-86 (LOC mutator family port)
**Successors:** TBD pending NAI-87 close re-smoke

## Goal

Port the `SOUND_SYNTH` script opcode (2104) end-to-end so
`[proc,open_and_close_door]` runs past pc=68. Cascade-blocker
surfaced by NAI-86's close door-click smoke at HEAD `26782f0`:

```
script="[proc,open_and_close_door]" err="no handler for SOUND_SYNTH (opcode 2104) at pc=68"
```

Routed to NAI-87 per `smoke_surfaces_adjacent_divergences.md` (>30
LOC stretch threshold) and matches the
`protocol_stub_not_completed.md` pattern: opcode constant declared
in `pkg/script/opcode.go:204` + stringification in the same file's
switch (line 815-816), but no handler, no Player wire, no encoder.

## Reference

- TS handler: `Engine-TS/src/engine/script/handlers/PlayerOps.ts:466-474`
- TS encoder: `Engine-TS/src/network/game/server/codec/SynthSoundEncoder.ts:9-13`
- TS message: `Engine-TS/src/network/game/server/model/SynthSound.ts:3-11`
- TS wire-opcode declaration: `Engine-TS/src/network/game/server/ServerGameProt.ts:80`
  (`SYNTH_SOUND = new ServerGameProt(12, 5)`)
- TS pointer requirement: `Engine-TS/src/engine/script/ScriptOpcodePointers.ts:434-437`
  (`require: ['active_player']`, `require2: ['active_player2']`)
- TS pop semantics: `Engine-TS/src/engine/script/ScriptState.ts:325-331`
  (`popInts(amount)` fills the result slice from `i = amount-1` down
  to `0`, popping top-of-stack first)
- goscape template (NAI-16 retire of S7h-D1 covering MIDI_SONG/MIDI_JINGLE):
  `pkg/script/handlers_player.go:861-909`,
  `modules/world/midi_encoders.go`,
  `modules/world/player_script.go:905-973`,
  `pkg/script/active.go:469-483`,
  `pkg/io/protocol/game/server/prot.go:94-100`.

## Behavior contract (TS-mirror)

```
SOUND_SYNTH(synth, loops, delay):
  delay := pop()                       // top-of-stack first per ScriptState.ts:325-331
  loops := pop()
  synth := pop()
  require active_player                // checkedHandler(ActivePlayer) gate
  if Self.LowMemory(): return          // silent no-op per PlayerOps.ts:470-472
  Self.PlaySynth(synth, loops, delay)
    encode: p2(synth) p1(loops) p2(delay)   // SynthSoundEncoder.ts:10-12
    writeOut(OpSynthSound{Opcode:12, PayloadSize:5}, buf)
```

No `check()` validation — TS has none on this opcode (unlike
MIDI_JINGLE which has `checkNotNull(delay)` and
`checkStringNotNull(name)`). Per `defensive_gate_doc_comment_label.md`,
goscape adds no defensive checks here either.

## Files touched

| File | Change | Approx LOC |
|---|---|---|
| `pkg/io/protocol/game/server/prot.go` | Add `OpSynthSound = Op{Opcode: 12, PayloadSize: 5}` next to MIDI block, with TS-line comment citing `ServerGameProt.ts:80` | 5 |
| `modules/world/sound_encoders.go` | **New file.** `encodeSynthSound(buf *packet.Packet, synth uint16, loops uint8, delay uint16)` — `p2/p1/p2` per `SynthSoundEncoder.ts:9-13` | 15 |
| `modules/world/sound_encoders_test.go` | **New file.** Bytes-exact (5-byte hex pin) + client-order round-trip + zero-value + max-value tests; follows `midi_encoders_test.go` shape | 50 |
| `modules/world/player_script.go` | Add `(*Player).PlaySynth(synth, loops, delay int)` — buf alloc, `encodeSynthSound`, `writeOut(OpSynthSound, buf.Bytes())` | 10 |
| `modules/world/player_script_test.go` | `TestPlaySynthWritesOut` — pin positive path | 20 |
| `pkg/script/active.go` | Add `PlaySynth(synth, loops, delay int)` to `ActivePlayer` interface with TS-line doc-comment | 8 |
| `pkg/script/handlers_player.go` | `handleSoundSynth` — pop delay/loops/synth top-down, `requireActivePlayer`, `LowMemory` gate, `s.Self.PlaySynth(synth, loops, delay)` | 18 |
| `pkg/script/handlers.go` | Register `OpSoundSynth: handleSoundSynth` in audio block (next to MIDI handlers at line 422-424) | 1 |
| `pkg/script/runner_test.go` mockPlayer | Add `playSynthCalls []struct{ synth, loops, delay int }` capture + `PlaySynth` method (lowMemory mock already exists at line 286) | 10 |
| `pkg/script/handlers_player_test.go` | Three tests: (a) normal pop+dispatch records mock call with correct values; (b) lowMemory=true → silent no-op (no mock call); (c) no-active-player → `requireActivePlayer` error | 70 |

**Production:** ~67 LOC. **Tests:** ~140 LOC. Total ~207 LOC.

## Type widths at the wire

- `synth` is a script int → encoded as `p2` (uint16). Out-of-range
  script values silently truncate per TS encoder behavior. Go cast
  at the encode site: `uint16(synth)`.
- `loops` → `p1` (uint8). Same truncation.
- `delay` → `p2` (uint16). Same.
- `(*Player).PlaySynth` accepts `int` for sig-uniformity with
  `PlaySong` / `PlayJingle`; casts at encode site (mirrors
  `encodeMidiJingle(buf, uint16(delay), jingle)` precedent at
  `modules/world/player_script.go:971`).

## Test strategy

### Encoder (`modules/world/sound_encoders_test.go`)

Per `rsbuf_roundtrip_tests.md` — pin both byte-length and
client-order field decode in every test.

- `TestEncodeSynthSoundFieldsDecodeInClientOrder` — encode
  `(synth=0x1234, loops=0x56, delay=0x789A)`, decode in
  Java-client reader order: G2/G1/G2; assert each field matches.
- `TestEncodeSynthSoundBytesExact` — encode `(0x0102, 0x03, 0x0405)`,
  assert `buf.Bytes()` is exactly `[0x01, 0x02, 0x03, 0x04, 0x05]`
  (5 bytes, big-endian word ordering).
- `TestEncodeSynthSoundZeroValuesValid` — encode `(0, 0, 0)`,
  assert exactly 5 zero bytes.
- `TestEncodeSynthSoundMaxValuesValid` — encode `(0xFFFF, 0xFF, 0xFFFF)`,
  assert exactly `[0xFF×5]`.

### Player wire (`modules/world/player_script_test.go`)

- `TestPlaySynthWritesOut` — synthetic Player wired to a recorder
  (follow `TestPlaySongWritesOut` shape at line 451+); call
  `p.PlaySynth(123, 1, 0)`; assert single recorded writeOut with
  opcode `OpSynthSound.Opcode` (=12) and a 5-byte payload that
  decodes back to `(123, 1, 0)`.

### Handler (`pkg/script/handlers_player_test.go`)

- `TestHandleSoundSynth_DispatchesToActivePlayer` — push
  `synth=42, loops=2, delay=100` (left-to-right; matches TS
  `popInts(3)` evaluation), set `Self`, run handler, assert
  `mockPlayer.playSynthCalls` recorded exactly one entry
  `{synth:42, loops:2, delay:100}` and no error.
- `TestHandleSoundSynth_LowMemorySilent` — set
  `mockPlayer.lowMemoryValue = true`, push the same three ints,
  set `Self`, run handler, assert no entries in `playSynthCalls`
  and no error returned (silent no-op).
- `TestHandleSoundSynth_NoActivePlayerReturnsError` — push three
  ints, leave `Self = nil`, run handler, assert error matches
  `requireActivePlayer` shape (mirror existing
  `TestHandleMidiSong_NoActivePlayerReturnsError` if present).

### Re-smoke (out-of-band, post-merge)

User runs the door-click smoke from NAI-86 at NAI-87 HEAD:

1. Start server (Claude cannot — see `smoke_test_server_handoff.md`).
2. Login Tutorial Island starting position.
3. Click the closed door at the spawn-room exit.
4. Observe:
   - **(3) walkability:** can the player path through the
     LOC_CHANGE'd inviswall after the close?
   - **(4) auto-revert:** does the open variant disappear after
     `duration=3` ticks (lifecycle revert path) — and does the
     original closed door reappear afterwards?

Routing decision (per `cascade_theory_smoke_binding.md`):
- Both verify → close NAI-86 carry-forward items 3+4 in the
  `Closes memory:` trailer of the NAI-87 close commit.
- Either diverges → file new tracker entry; route to NAI-88.

## Cadence

Single-bundle subagent-driven TDD (per `runescript_cadence.md`).
Template is fully prescriptive — no architectural decisions left
to surface during implementation.

- Bundle 1 (one task, ten files): land all production + test code
  in TDD order (encoder → Player wire → ActivePlayer interface →
  handler + dispatch → mockPlayer capture → handler tests).
- Two-stage Sonnet review at bundle close (per
  `superpowers_code_reviewer_model.md` model cap).
- Pre-dispatch controller pre-flight (per `controller_preflight.md`):
  re-grep + Read each cited file path, line number, and helper
  state at HEAD before writing the implementer prompt.

## Deviations expected

None. Template-mirror port; fidelity is straightforward and the
TS source has no quirks worth tracking. If anything surfaces
during implementation that requires a deviation, file under
`NAI-87-D-<TAG>` and surface in the close commit.

## Risk register

- **R1 — pop order.** Three-int pops are easy to mis-mirror.
  Mitigated: explicit `delay/loops/synth` top-down comment in
  handler citing `ScriptState.ts:325-331` + handler test asserting
  correct mapping (`synth=42, loops=2, delay=100` distinct values
  catch any swap).
- **R2 — PayloadSize=5 fixed.** Both MIDI ops are dynamic (-1/-2);
  SynthSound is fixed 5. Encoder must produce exactly 5 bytes.
  Mitigated: bytes-exact test pins length; `Op{PayloadSize: 5}`
  declared explicitly.
- **R3 — Mock signature mismatch in handler tests** (per
  `mock_recorder_field_naming_check.md`). Mitigated: plan-author
  greps `mockPlayer` struct in `pkg/script/runner_test.go` and
  reproduces field-naming convention before codifying tests.
- **R4 — Encoder file isolation.** New `sound_encoders.go` could
  drift from `midi_encoders.go` test conventions. Mitigated:
  `sound_encoders_test.go` mirrors `midi_encoders_test.go` test
  function naming + assertion style verbatim.

## Out of scope

- Other `SOUND_*` opcodes — none currently surfaced; no scope
  creep.
- Renaming `midi_encoders.go` to `audio_encoders.go` — explicitly
  rejected during brainstorming; new `sound_encoders.go` keeps the
  diff focused.
- TS `Player.write(...)` queue infrastructure — already in place
  via existing `(*Player).writeOut`; no refactor needed.
- The NAI-86 carry-forward items 3+4 verification is a re-smoke
  observation only; if behavior diverges, the divergence ports
  in NAI-88, not here.
