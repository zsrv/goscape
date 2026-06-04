# NAI-16 — MIDI_SONG + MIDI_JINGLE encoders + PRELOADED registry port

- **Sub-spec**: NAI-16
- **Date**: 2026-04-24
- **Scope label**: C (cross-package port — `pkg/cache`, `pkg/io/protocol/game/server`, `modules/world`; ~225-380 LOC)
- **Predecessors**: S7i (session-flags plumbing) — last on `main` as `6d99fda`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

S7h ported the MIDI_SONG (2064) and MIDI_JINGLE (2063) **script handlers** with TS-faithful dispatch (StringNotNull validation, active-player gate, lowMemory bail) but deferred the client-packet writes under a single deviation **S7h-D1**. `(*Player).PlaySong` and `(*Player).PlayJingle` perform TS name normalization (lowercase + spaces↔underscores — asymmetric by TS design, pinned by `TestNormalize*Name*`) and early-return on empty, but make no `p.writeOut` call. `OpMidiSong` / `OpMidiJingle` are NOT registered in `pkg/io/protocol/game/server/prot.go` — avoids dead-API wire ops pending the encoder port.

**S7h close note:** the 2026-04-24 Java-client smoke confirmed `[label,music_playbyregion]` ran to completion at HEAD=`327dc6a` with zero further stalls — the script body's music-dispatch does not semantically depend on client-side packet acknowledgment. **S7h-D1 is a safe deviation in production** (script flow completes without client audio); NAI-16 retires the deviation to restore client-side audio playback fidelity.

**Scope-gate finding (2026-04-24, S7i close, drive-by Item C declined inline):** the original S7h estimate of 60-120 LOC was too low. Pre-impl scope-gate at HEAD=`6d99fda` discovered `pkg/cache/` contains only `crctable.go` (46 lines, RS2 archive CRC table) — zero `LoadMidi` / `LoadSound` / `LoadMusic` / `LoadJingle` symbols across the repo. No `PRELOADED` registry equivalent. BUT the data files ARE staged: `data/pack/client/{maps,songs,jingles}` are all populated. This is a code port, not an asset-bundling problem.

**Hidden coupling:** `PRELOADED` is consumed by three TS sites — `Player.playSong/playJingle` (this sub-spec's target), `RebuildNormalEncoder.ts:18-19`, and `RebuildGetMapsHandler.ts:44,54`. The registry-shape coupling is addressed here (all three dirs populated); the consumer-side encoder/handler ports for `RebuildNormal` / `RebuildGetMaps` are independent follow-up work (§ Out of scope). Porting the registry to feed only music while leaving map/loc keys behind would create a half-state the `true_to_ts_gate` would flag at next-touch.

## Tech stack

- Go 1.26+
- Existing packages: `pkg/cache/` (adds `preloaded.go`), `pkg/io/packet/`, `pkg/io/protocol/game/server/`, `modules/world/`

## Scope (C)

- New file `pkg/cache/preloaded.go`: two exported module-level map vars (`Preloaded`, `PreloadedCRC`) plus one function `PreloadClient(baseDir string) error` that walks `baseDir/{maps,songs,jingles}` and populates both maps keyed by bare filename with CRC32/IEEE of raw bytes. Eager-at-startup; error-returning. Mirrors TS `PreloadedPacks.ts:1-41`.
- Wire `cache.PreloadClient("data/pack/client")` as the first statement in `modules/world/world.go`'s `startingFn` (NewWorldService, :81). Fail-fast: any error propagates out of `startingFn`, failing the world service via the standard dskit lifecycle.
- Register `OpMidiSong = Op{Opcode: 54, PayloadSize: -1}` and `OpMidiJingle = Op{Opcode: 212, PayloadSize: -2}` in `pkg/io/protocol/game/server/prot.go`. Verified TS source: `ServerGameProt.ts:81-82`.
- New file `modules/world/midi_encoders.go`: two unexported byte-level encoder helpers — `encodeMidiSong(buf, name, crc, length)` writing `PJStrLF + P4 + P4` (mirrors `MidiSongEncoder.ts`), and `encodeMidiJingle(buf, delay, data)` writing `P2 + PData` (mirrors `MidiJingleEncoder.ts`).
- Wire PRELOADED lookup + encoder call + `p.writeOut(...)` into `(*Player).PlaySong` / `PlayJingle` at the two `// deferred (S7h-D1): ...` comment sites (`modules/world/player_script.go:577, 606`). Silent no-op on lookup miss (mirrors TS `if (song && crc)` / `if (jingle)` guards).
- New file `pkg/cache/preloaded_test.go`: hybrid fixture tests — synthetic `t.TempDir` unit tests that always run, plus one skip-if-missing integration test against the real `data/pack/client/songs/adventure.mid`.
- New file `modules/world/midi_encoders_test.go`: layered encoder tests per `rsbuf_roundtrip_tests` — field-level decode-in-client-order + byte-exact pin for each encoder.
- Mutate `modules/world/player_script_test.go`: rename `TestPlaySongNoWriteOut` → `TestPlaySongWritesOut`; rename `TestPlayJingleNoWriteOut` → `TestPlayJingleWritesOut`; add three new miss-path tests pinning the silent-no-op branches.
- Retire S7h-D1 from the deviation registry. Close count: **16 − 1 = 15**.

## Explicitly out of scope

- **`RebuildNormalEncoder` port.** `OpRebuildNormal = Op{237, -2}` exists at `prot.go:41` but no goscape encoder consumes `PreloadedCRC["m{x}_{z}"]` / `["l{x}_{z}"]`. Separate sub-spec when the rebuild-region wire pipeline lands end-to-end. This port's registry side is unblocked by NAI-16 at zero cost — consumers can `cache.PreloadedCRC["m30_72"]` directly when they ship.
- **`RebuildGetMapsHandler` port.** `DataLand` / `DataLoc` writes exist in `modules/world/data_map.go` but the map-squares-from-client request-path handler is not wired. Independent follow-up; same registry-ready posture as above.
- **`cache.MakeCRCs()` relocation.** The pre-existing `modules/asset/handler.go:24` misplacement (inside `/crc` HTTP handler, with a `// TEST - belongs in world` comment) is a separate cleanup. Not widened into NAI-16 scope. Appropriate target: a small follow-up sub-spec that moves `MakeCRCs` to the same `world.startingFn` wire-in point this sub-spec establishes for `PreloadClient`.
- **Config-driven data directory.** `data/pack/client` is hardcoded in `PreloadClient`'s caller (matches existing `cache.MakeCRCs` pattern). Future config work can parameterize both in one pass.
- **Partial-success transaction semantics.** If `maps/` loads but `songs/` fails, `Preloaded` already contains map entries when the error returns. Not retried (lifecycle treats `startingFn` as one-shot; failure halts the service). Documented leak; acceptable given the lifecycle guarantee.

## Architecture

### Files created

- `pkg/cache/preloaded.go`
- `pkg/cache/preloaded_test.go`
- `modules/world/midi_encoders.go`
- `modules/world/midi_encoders_test.go`

### Files modified (production)

- `pkg/io/protocol/game/server/prot.go` — two new Op constants.
- `modules/world/world.go` — single-line prefix in `startingFn` calling `cache.PreloadClient`.
- `modules/world/player_script.go` — replace two `// deferred (S7h-D1): ...` comment bodies with PRELOADED lookup + encoder call + `p.writeOut`. Rewrite the five-line S7h-D1 doc-comments on `PlaySong` / `PlayJingle` to reflect NAI-16's closure.

### Files modified (tests)

- `modules/world/player_script_test.go` — rename 2 tests, add 3 miss-path tests, add `seedCachedSong(t, ...)` helper.

### Registry layering

```
pkg/cache/
├── crctable.go       (existing — JAG-archive CRC table for /crc HTTP endpoint)
└── preloaded.go      (NEW — per-file registry for .mid/.dat blobs)
```

These are deliberately **independent** module-level state. `CrcTable []uint32` and `CrcBuffer *Packet` are the 9-slot JAG archive-CRC table used by the asset HTTP server; `Preloaded` and `PreloadedCRC` are per-individual-file maps for MIDI playback + future rebuild-region streaming. Docstrings in `preloaded.go` will call out the distinction to prevent future confusion.

### Touch-site map

```
World startingFn                       Script VM               Player
    │                                     │                      │
    ▼                                     ▼                      ▼
cache.PreloadClient                handleMidiSong          PlaySong(name)
  data/pack/client/maps      ─►    pop/validate/gate  ─►     normalize
  data/pack/client/songs           Self.PlaySong(name)        key = name+".mid"
  data/pack/client/jingles                                    cache.Preloaded[key]?
    │                                                         cache.PreloadedCRC[key]?
    │                                                         encodeMidiSong(buf,...)
    ▼                                                         p.writeOut(OpMidiSong,...)
cache.Preloaded    (name → bytes)
cache.PreloadedCRC (name → uint32)
    ▲
    │  read-many, write-none after startup
```

## Component designs

### `pkg/cache/preloaded.go`

```go
package cache

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/zsrv/goscape/pkg/io/packet"
)

// Preloaded maps bare filenames (e.g. "m30_72", "adventure.mid",
// "advance agility.mid") to their raw bytes. Mirrors TS
// PreloadedPacks.ts's `PRELOADED` Map<string, Uint8Array>.
//
// Write-once at world-module startup via PreloadClient; read-many at
// runtime by (*Player).PlaySong / PlayJingle (and future Rebuild*
// consumers — see TS RebuildNormalEncoder.ts:18-19,
// RebuildGetMapsHandler.ts:44,54).
//
// Distinct from CrcTable / CrcBuffer in crctable.go — those are the
// 9-slot JAG archive-CRC table served by the /crc HTTP endpoint; this
// is per-individual-file state for MIDI playback + map/loc streaming.
var Preloaded = map[string][]byte{}

// PreloadedCRC pairs with Preloaded: bare-filename → CRC32/IEEE of the
// raw bytes. Mirrors TS PRELOADED_CRC. Same write/read posture as
// Preloaded above.
var PreloadedCRC = map[string]uint32{}

// PreloadClient walks baseDir/{maps,songs,jingles} and populates
// Preloaded + PreloadedCRC. Mirrors TS preloadClient() at
// PreloadedPacks.ts:8-41.
//
// Error-returning (vs TS's throw-on-failure) so the caller can fail
// the world startingFn cleanly. Eager: all three dirs read
// synchronously before return. Typical cost <200ms for the current
// data set (~tens of MB total).
//
// Partial-success leak: if, for example, maps/ loads but songs/ fails,
// Preloaded already contains map entries when the error returns. Not
// retried (the services.BasicService lifecycle treats startingFn as
// one-shot; failure halts the service). Documented; acceptable.
func PreloadClient(baseDir string) error {
    for _, sub := range []string{"maps", "songs", "jingles"} {
        dir := filepath.Join(baseDir, sub)
        entries, err := os.ReadDir(dir)
        if err != nil {
            return fmt.Errorf("preload %s: %w", sub, err)
        }
        for _, e := range entries {
            if e.IsDir() {
                continue
            }
            name := e.Name()
            path := filepath.Join(dir, name)
            data, err := os.ReadFile(path)
            if err != nil {
                return fmt.Errorf("preload read %s: %w", path, err)
            }
            Preloaded[name] = data
            PreloadedCRC[name] = packet.GetCRC(data, 0, len(data))
        }
    }
    return nil
}
```

TS reference (`PreloadedPacks.ts:5-41`):

```ts
export const PRELOADED = new Map<string, Uint8Array>();
export const PRELOADED_CRC = new Map<string, number>();

export function preloadClient() {
    const allMaps = fs.readdirSync('data/pack/client/maps');
    for (let i = 0; i < allMaps.length; i++) {
        const name = allMaps[i];
        const map = new Uint8Array(fs.readFileSync(`data/pack/client/maps/${name}`));
        const crc = Packet.getcrc(map, 0, map.length);
        PRELOADED.set(name, map);
        PRELOADED_CRC.set(name, crc);
    }
    // ... songs, jingles symmetric ...
}
```

**Key shape decisions:**
- **Exported vars** (not getters) — symmetry with existing `cache.CrcTable` / `CrcBuffer` / `CrcBuffer32` pattern AND for trivial test seeding (per Shape-B layered test strategy).
- **Bare-filename keys** — exact TS behavior: `os.ReadDir` yields `DirEntry.Name()` which is the bare filename. Consumers add the extension (or not) to match: `PlaySong` does `name+".mid"`; future `RebuildNormalEncoder` does `"m"+x+"_"+z` (no extension).
- **`e.IsDir()` skip** — defensive against stray subdirs. Go's `os.ReadDir` returns both files and dirs; TS `fs.readdirSync` with the default options returns dirs too. TS relies on `readFileSync(path-pointing-at-dir)` to throw; our skip is a gentler guard and reflects the intent.
- **Dir iteration order** — explicit `[]string{"maps", "songs", "jingles"}` mirrors TS line order. Collisions (same filename in two dirs) resolve last-writer-wins, which means jingles-dir wins over songs-dir wins over maps-dir. Pinned by a test.

### `pkg/io/protocol/game/server/prot.go` additions

Two new Op constants grouped with a small audio-family comment block. Placement: **end of the existing `var (...)` block**, after `OpMessageGame` at :84. No reordering of existing opcodes.

```go
// MIDI client-audio packets (verified against TS ServerGameProt.ts:81-82).
// MIDI_SONG streams a song reference (name + crc + length so the client
// can fetch the .mid blob from the asset server); MIDI_JINGLE streams
// an inline jingle payload. Wired from the MIDI_SONG (2064) / MIDI_JINGLE
// (2063) script opcodes via (*Player).PlaySong / PlayJingle.
OpMidiSong   = Op{Opcode: 54, PayloadSize: -1}
OpMidiJingle = Op{Opcode: 212, PayloadSize: -2}
```

### `modules/world/midi_encoders.go`

```go
package world

import "github.com/zsrv/goscape/pkg/io/packet"

// encodeMidiSong writes a MidiSong payload per TS MidiSongEncoder.ts:
//
//   buf.pjstr(message.name);
//   buf.p4(message.crc);
//   buf.p4(message.length);
//
// Byte-aligned. Caller wraps in:
//   p.writeOut(gameserver.OpMidiSong, buf.Bytes())
//
// The string terminator is 0x0A (LF) per TS Packet.pjstr at
// io/Packet.ts:330-337 (universal goscape PJStrLF precedent).
func encodeMidiSong(buf *packet.Packet, name string, crc uint32, length uint32) {
    buf.PJStrLF(name)
    buf.P4(crc)
    buf.P4(length)
}

// encodeMidiJingle writes a MidiJingle payload per TS MidiJingleEncoder.ts:
//
//   buf.p2(message.delay);
//   buf.pdata(message.data, 0, message.data.length);
//
// Byte-aligned. Caller wraps in:
//   p.writeOut(gameserver.OpMidiJingle, buf.Bytes())
//
// goscape's PData(src) takes no offset/length and writes the whole
// slice; TS's pdata(src, 0, src.length) reduces to the same output.
func encodeMidiJingle(buf *packet.Packet, delay uint16, data []byte) {
    buf.P2(delay)
    buf.PData(data)
}
```

TS references (`MidiSongEncoder.ts`, `MidiJingleEncoder.ts`):

```ts
// MidiSongEncoder
encode(buf: Packet, message: MidiSong): void {
    buf.pjstr(message.name);
    buf.p4(message.crc);
    buf.p4(message.length);
}

// MidiJingleEncoder
encode(buf: Packet, message: MidiJingle): void {
    buf.p2(message.delay);
    buf.pdata(message.data, 0, message.data.length);
}
```

**Encoder placement rationale (mirroring goscape precedent, not TS file layout):** goscape's existing byte-packing encoders for per-player wire ops live co-located with their callers in `modules/world/`: `player_interface.go`, `data_map.go`, `stat_update.go`, `message_game.go`, `inv_stop_transmit.go`. `pkg/io/protocol/game/server/prot.go` is **only** Op constants — no encoder functions, no precedent for adding any. Mirroring TS's `src/network/game/server/codec/` directory as a new Go sub-package would introduce one package for two files, diverging from goscape convention. Per the convention, `midi_encoders.go` lives in `modules/world/`.

### `(*Player).PlaySong` and `(*Player).PlayJingle` replacements

Both methods at `modules/world/player_script.go` get their deferred-comment bodies replaced. Doc-comments rewritten to reflect NAI-16 closure.

```go
// PlaySong normalizes the song name per TS Player.playSong
// (Engine-TS/src/engine/entity/Player.ts:1902-1914), looks up the
// preloaded blob + CRC, and writes MidiSong to the client. Silent
// no-op on empty name or missing PRELOADED entry (mirrors TS's
// `if (song && crc)` guard at Player.ts:1910).
//
// NAI-16 retires S7h-D1: the PRELOADED lookup and MidiSong write are
// now wired. TestPlaySongWritesOut is the positive-pin; the miss-path
// pins (TestPlaySong*ReturnsSilently) verify the silent-no-op guards.
func (p *Player) PlaySong(name string) {
    name = normalizeSongName(name)
    if name == "" {
        return
    }
    key := name + ".mid"
    song, okSong := cache.Preloaded[key]
    crc, okCRC := cache.PreloadedCRC[key]
    if !okSong || !okCRC {
        return
    }
    buf := packet.NewPacket(nil)
    encodeMidiSong(buf, name, crc, uint32(len(song)))
    p.writeOut(gameserver.OpMidiSong, buf.Bytes())
}

// PlayJingle normalizes the jingle name per TS Player.playJingle
// (Engine-TS/src/engine/entity/Player.ts:1916-1926), looks up the
// preloaded blob, and writes MidiJingle to the client. Silent no-op
// on empty name or missing PRELOADED entry (mirrors TS's `if (jingle)`
// guard at Player.ts:1923).
//
// NAI-16 retires S7h-D1 (jingle side). TestPlayJingleWritesOut pins
// the positive path; TestPlayJingleMissingFromPreloadedReturnsSilently
// pins the silent-no-op guard.
func (p *Player) PlayJingle(delay int, name string) {
    name = normalizeJingleName(name)
    if name == "" {
        return
    }
    jingle, ok := cache.Preloaded[name+".mid"]
    if !ok {
        return
    }
    buf := packet.NewPacket(nil)
    encodeMidiJingle(buf, uint16(delay), jingle)
    p.writeOut(gameserver.OpMidiJingle, buf.Bytes())
}
```

**Delay cast rationale:** `PlayJingle(delay int, ...)` signature stays `int` to preserve the existing `ActivePlayer` interface shape (script-VM's `popInt` produces `int`). The `uint16` cast at the `encodeMidiJingle` call site mirrors TS's `p2` (which takes `number` and wraps at 16 bits identically). Negative delays are rejected upstream by `checkNotNull` in `handleMidiJingle`; large-positive delays wrap identically in both implementations.

The S7h `_ = delay` stub is removed (the value is now consumed).

**Imports added to `player_script.go`:** `github.com/zsrv/goscape/pkg/cache` and `github.com/zsrv/goscape/pkg/io/packet`. Existing `strings` import stays. The existing `gameserver` alias for `pkg/io/protocol/game/server` (present for `OpCamReset`, `OpPCountDialog`) carries over.

### `modules/world/world.go` startingFn wire-in

Placement: **first statement** in the `startingFn` lambda at `world.go:81+`. Failing cache load must halt startup before the TCP listener goes live.

```go
startingFn := func(ctx context.Context) error {
    if err := cache.PreloadClient("data/pack/client"); err != nil {
        return fmt.Errorf("world: preload client assets: %w", err)
    }
    // ... existing startingFn body ...
}
```

**Import added to `world.go`:** `github.com/zsrv/goscape/pkg/cache`. Existing `fmt` import carries over.

**Hardcoded `"data/pack/client"` path rationale:** matches existing `cache.MakeCRCs()` literal pattern in `crctable.go:36-43` and the `asset/handler.go` `path.Join("data/pack/client", ...)` pattern. Consistency over ergonomics; config-driven path is a cross-cutting concern suited to a separate sub-spec.

## Test strategy

### `pkg/cache/preloaded_test.go` (new file — hybrid fixture shape)

**Synthetic-fixture tests (always run):**

1. `TestPreloadClient3DirWalkPopulatesBothMaps` — create `<tmp>/{maps,songs,jingles}` with 1 file each (`m0_0 = []byte("land")`, `test.mid = []byte{0xFF, 0x00}`, `fanfare.mid = []byte{0x01}`). Call `PreloadClient(<tmp>)`. Assert each `Preloaded[key]` matches input bytes; each `PreloadedCRC[key]` matches `packet.GetCRC(bytes, 0, len(bytes))`. **Pins bare-filename-as-key + extension-preserved + both maps written in lockstep.**
2. `TestPreloadClientEmptyDirsOK` — create dirs but no files. Assert `PreloadClient` returns nil; both maps empty.
3. `TestPreloadClientZeroByteFile` — create `<tmp>/songs/empty.mid` as zero-byte. Assert `Preloaded["empty.mid"] == []byte{}` and `PreloadedCRC["empty.mid"] == 0` (crc32.IEEE of empty is 0).
4. `TestPreloadClientSkipsSubdirs` — create `<tmp>/maps/sub/` subdir. Assert `PreloadClient` returns nil; no `Preloaded["sub"]` key.
5. `TestPreloadClientMissingMapsDirReturnsError` — create `<tmp>/{songs,jingles}` only. Assert error wrapping `"preload maps"`.
6. `TestPreloadClientMissingSongsDirReturnsError` — symmetric for `songs`.
7. `TestPreloadClientMissingJinglesDirReturnsError` — symmetric for `jingles`.
8. `TestPreloadClientKeyCollisionLastWins` — create `<tmp>/songs/shared.mid` with bytes A, `<tmp>/jingles/shared.mid` with bytes B. Assert `Preloaded["shared.mid"] == B` (jingles-dir wins — last in iteration order). **Pins dir-order semantics.**

**Skip-if-missing integration test (fires when real data present):**

9. `TestPreloadClientAgainstStagedDataLoadsAdventure` — if `os.Stat("data/pack/client/songs/adventure.mid")` errors, `t.Skip`. Otherwise: call `PreloadClient("data/pack/client")`, assert nil error; assert `Preloaded["adventure.mid"]` non-empty and equal to direct `os.ReadFile` of that path; assert `PreloadedCRC["adventure.mid"]` matches `packet.GetCRC` on same. **Pins end-to-end wire-up against the exact filename the `[label,music_playbyregion]` smoke requires.**

**Test isolation note:** `Preloaded` and `PreloadedCRC` are package-global. Each test's setup must arrange `t.Cleanup` to delete any keys it seeded (or use a shared `resetPreloaded(t)` helper). The synthetic-fixture tests above all call `PreloadClient` against a fresh `t.TempDir` root — but PreloadClient does not *clear* the maps before populating, it merely inserts. So a test-run isolation helper is required. Defined once in `preloaded_test.go`:

```go
func resetPreloadedForTest(t *testing.T) {
    t.Helper()
    for k := range Preloaded {
        delete(Preloaded, k)
    }
    for k := range PreloadedCRC {
        delete(PreloadedCRC, k)
    }
    t.Cleanup(func() {
        for k := range Preloaded {
            delete(Preloaded, k)
        }
        for k := range PreloadedCRC {
            delete(PreloadedCRC, k)
        }
    })
}
```

Called at the start of every test in this file. The Cleanup registration is belt-and-suspenders against test-order leakage.

### `modules/world/midi_encoders_test.go` (new file — layered rsbuf_roundtrip_tests shape)

**`encodeMidiSong` coverage:**

10. `TestEncodeMidiSongFieldsDecodeInClientOrder` — call `encodeMidiSong(buf, "adventure", 0xDEADBEEF, 2048)`. Then `r := packet.NewPacket(buf.Bytes())`; `r.Pos = 0`; assert `r.GJStrLF() == "adventure"`, `r.G4() == 0xDEADBEEF`, `r.G4() == 2048`, `r.Pos == buf.Len()`. **Pins PJStrLF+P4+P4 ordering matches Java client reader.**
11. `TestEncodeMidiSongBytesExact` — call `encodeMidiSong(buf, "a", 0x01020304, 0x05060708)`. Assert `buf.Bytes() == []byte{0x61, 0x0A, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}`. **Byte-exact regression pin: big-endian P4 + 0x0A LF terminator.**
12. `TestEncodeMidiSongEmptyNameValid` — `encodeMidiSong(buf, "", 0, 0)`. Assert `buf.Bytes() == []byte{0x0A, 0, 0, 0, 0, 0, 0, 0, 0}` (LF terminator then eight zero bytes). **Pins empty-name representability at encoder level (callers guard upstream but the encoder must not panic).**

**`encodeMidiJingle` coverage (symmetric):**

13. `TestEncodeMidiJingleFieldsDecodeInClientOrder` — call `encodeMidiJingle(buf, 500, []byte{0x01, 0x02, 0x03})`. Assert `r.G2() == 500`, remaining bytes `== []byte{0x01, 0x02, 0x03}`. **Pins P2-then-PData ordering.**
14. `TestEncodeMidiJingleBytesExact` — `encodeMidiJingle(buf, 0x0102, []byte{0xFF})`. Assert `buf.Bytes() == []byte{0x01, 0x02, 0xFF}`. **Pins big-endian P2 + raw PData append.**
15. `TestEncodeMidiJingleEmptyDataValid` — `encodeMidiJingle(buf, 0, []byte{})`. Assert `buf.Bytes() == []byte{0x00, 0x00}`. Pins empty-payload edge case.

### `modules/world/player_script_test.go` (mutations to existing file)

**Renames + body rewrites (flip absence-pin → positive-pin):**

16. **RENAME** `TestPlaySongNoWriteOut` → `TestPlaySongWritesOut`:

    ```go
    // TestPlaySongWritesOut pins NAI-16's retirement of S7h-D1:
    // (*Player).PlaySong now issues a writeOut after the PRELOADED
    // lookup. Failure signal = "write-path broken or PRELOADED seeding
    // broken." Replaces the prior absence-pin (which was the S7h-D1
    // escalation signal — now satisfied).
    func TestPlaySongWritesOut(t *testing.T) {
        seedCachedMidi(t, "adventure.mid", []byte{0x01, 0x02, 0x03}, 0xDEADBEEF)
        p, _ := newTestPlayer(t)
        p.PlaySong("adventure")
        if n := p.client.bufw.Buffered(); n == 0 {
            t.Errorf("PlaySong wrote 0 bytes to c.bufw; want >0 (NAI-16 positive pin)")
        }
    }
    ```

17. **RENAME** `TestPlayJingleNoWriteOut` → `TestPlayJingleWritesOut`. Symmetric seeding + assertion (via `seedCachedMidi(t, "fanfare.mid", ...)`; call `p.PlayJingle(3, "fanfare")`; assert `Buffered() > 0`). The PRELOADED_CRC entry the helper seeds is unused by PlayJingle's lookup but costs nothing.

**New miss-path tests (silent-no-op per § Error handling):**

18. `TestPlaySongMissingFromPreloadedReturnsSilently` — do NOT seed `"missing.mid"`. `p.PlaySong("missing")`. Assert `Buffered() == 0`. **Pins TS's `if (song && crc)` guard.**
19. `TestPlayJingleMissingFromPreloadedReturnsSilently` — symmetric.
20. `TestPlaySongSongSeededButCRCMissingReturnsSilently` — seed `Preloaded["orphan.mid"] = bytes` directly, but leave `PreloadedCRC["orphan.mid"]` unset. `p.PlaySong("orphan")`. Assert `Buffered() == 0`. **Pins the `||` conjunction (both maps required) in the guard.**

**Preserved unchanged:** `TestNormalizeSongName*`, `TestNormalizeJingleName*`, `TestPlaySongEmptyNameReturnsSilently`, `TestPlayJingleEmptyNameReturnsSilently`, `TestLowMemoryGetterAndDefault` — all still valid post-NAI-16.

**Shared helper:**

```go
// seedCachedMidi seeds both cache.Preloaded and cache.PreloadedCRC under
// `name` and registers a t.Cleanup to remove both entries after the test.
// Mirrors the production PreloadClient write shape without touching
// the filesystem. Usable for both song and jingle test paths (PlayJingle
// ignores the CRC entry; the wasted write is harmless).
func seedCachedMidi(t *testing.T, name string, data []byte, crc uint32) {
    t.Helper()
    cache.Preloaded[name] = data
    cache.PreloadedCRC[name] = crc
    t.Cleanup(func() {
        delete(cache.Preloaded, name)
        delete(cache.PreloadedCRC, name)
    })
}
```

Lives in `player_script_test.go` (used only by that file's tests). If a future test in another modules/world file needs the same helper, we can move it to a shared `_test.go` at that time — not promoting proactively (YAGNI).

### Coverage crosscheck (per `plan_test_coverage_crosscheck`)

Every test above (1-20) maps 1:1 to a code block in the plan's task list. Plan-write step enforces this crosscheck. Counts:

| File | Test count |
|---|---|
| `pkg/cache/preloaded_test.go` | 9 (1-9, including skip-if-missing) |
| `modules/world/midi_encoders_test.go` | 6 (10-15) |
| `modules/world/player_script_test.go` | +3 new (18-20), 2 renamed + rewritten (16-17) |
| **Total exercises** | **~20** |

## Deviations

| Tag | Description | Rationale | Follow-up |
|---|---|---|---|
| *(no new NAI-16 deviations)* | — | — | — |

**S7h-D1 retired.** `(*Player).PlaySong` and `(*Player).PlayJingle` now issue a `p.writeOut` after PRELOADED lookup, matching TS `Player.playSong` / `Player.playJingle` behavior. `OpMidiSong` / `OpMidiJingle` registered in `prot.go`. Positive-pin tests replace the absence-pins.

**Pre-existing deviations carried forward:** S7a-D1, S7a-D2, S7b-D1, S7c-D1, S7d-D1, S7d-D2, S7d-D3, S7d-D4, S7e-D1, S7f-D1, S7f-D2, S7f-D3, S7g-D1, S7g-D2, S7g-D3.

**Active count after NAI-16 close:** 16 (at HEAD=`6d99fda`) − 1 (S7h-D1 retired) = **15**.

## Adjacent deferred work (not NAI-16)

- **`cache.MakeCRCs()` relocation** — the existing `asset/handler.go:24` misplacement (`cache.MakeCRCs() // TEST - belongs in world`) is a separate cleanup. Natural target: move to the same `world.startingFn` wire-in point NAI-16 establishes. ~5 LOC.
- **`RebuildNormalEncoder` + `RebuildGetMapsHandler` ports** — registry-side unblocked by NAI-16 at zero cost; encoder-side and handler-side are independent follow-ups. See § Out of scope.
- **S7g `dbFind` / `dbFindRefine` dispatch refactor** — closed by drive-by Item A at S7i close (`6021a30`); no longer pending.

## Acceptance gates

Per `verify_implementer_claims`, the combined-review step runs these fresh, independently of any subagent-claimed results.

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` — clean.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — all green (fresh whole-module run; not per-package).
3. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — clean.
4. `rg -n "OpMidiSong|OpMidiJingle" pkg/io/protocol/game/server/prot.go modules/world/` — four hits minimum: two Op declarations + two `p.writeOut(gameserver.OpMidi*, ...)` calls in `player_script.go`.
5. `rg -n "cache.Preloaded|cache.PreloadedCRC" pkg/ modules/` — reads at `modules/world/player_script.go` (2× PlaySong branch), `modules/world/player_script_test.go` (seeds via helper). Writes only in `pkg/cache/preloaded.go` (`PreloadClient`).
6. `rg -n "PreloadClient" modules/ cmd/` — exactly one production call site in `modules/world/world.go` (the startingFn wire-in).
7. `rg -n "deferred \\(S7h-D1\\)" modules/world/` — **zero hits.** The two comment sites at `player_script.go:577, 606` must be replaced.
8. `rg -n "NoWriteOut" modules/world/` — **zero hits.** Both renames complete.
9. **Visual diff check** on `modules/world/player_script.go`'s `PlaySong` and `PlayJingle` doc-comments — must reference NAI-16 closure, not S7h-D1 absence.
10. **Post-commit grep** `git log --oneline -1` — close commit subject line contains "NAI-16" and "MIDI" (or "midi encoders"); `Closes memory: nai_followups` trailer present per `close_commit_memory_trailer`.

## Rollout / smoke-test sequencing

Per `smoke_test_server_handoff`, the Java-client smoke requires a user-launched server. After NAI-16 close commit:

- **User action:** run `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml` and exercise `[label,music_playbyregion]`.
- **Expected primary outcome:** client-side MIDI audio playback now works for regions triggered by the script. No new opcode stalls (the handlers have been wired since S7h).
- **Secondary watch:** startup logs must show `cache.PreloadClient` completed without error. If the `data/pack/client/{maps,songs,jingles}` dirs are missing in the user's environment, startup fails loudly — expected fail-fast behavior per § Error handling.
- **Regression watch:** the next occupied slot for Player-method tests (`BuildAppearance` / `SetAppearanceInv` from S7c) should be unaffected; quick sanity-grep that `p.client.bufw.Buffered()` is not zero after any pre-existing writeOut call (existing regression pins).

## Cadence

- **Size bracket:** ~225-380 LOC production + tests (per S7h scope-gate breakdown). Upper end of the "standard cadence" bracket per `compressed_cadence` memory — not eligible for compression.
- **Cadence:** spec doc + plan doc (separate `docs(spec):` / `docs(plan):` commits), subagent-driven execution per task with per-task review, **two-stage final review** (code-quality pass + whole-impl fidelity pass).
- **Close commit:** conventional `chore(script): NAI-16 closed — MIDI encoders + PRELOADED registry`. Include `Closes memory: nai_followups` trailer per `close_commit_memory_trailer` to make the NAI-16 breadcrumb grep-discoverable from `git log`.
- **Execution mode:** subagent-driven-development per `execution_mode_default` — dispatch fresh subagent per task; no interactive mode menu.
- **Post-close handoff (per `post_task_handoff`):** update `nai_followups.md` NAI-16 entry with closure status; produce paste-ready resume prompt for the next session (smoke-test verification or next-sub-spec brainstorm).
