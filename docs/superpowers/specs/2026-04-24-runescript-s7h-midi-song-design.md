# S7h — MIDI_SONG + MIDI_JINGLE handlers + `lowMemory` plumbing

- **Sub-spec**: S7h
- **Date**: 2026-04-24
- **Scope label**: B (bundle — two sibling opcodes + login-side `lowMemory` plumbing; downstream client-packet writes deferred)
- **Predecessors**: S7g (DB_FIND family) — last on `main` as `7a1ef6a`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

The S7g smoke confirmed the DB_FIND cluster unblocked `[label,music_playbyregion]` past `pc=21`. Script advanced past `pc=74` and now stalls at:

```
script=[label,music_playbyregion]
err="no handler for MIDI_SONG (opcode 2064) at pc=74"
```

This is the same music-region-resolution script S7g just unblocked; next opcode in the body. `MIDI_SONG` (2064) has a script-opcode dispatch in goscape's `opcode.go:163` but no handler wired in `handlers.go`. The TS handler at `PlayerOps.ts:796-804` pops a string, gates on `player.lowMemory`, then calls `player.playSong(name)` which writes a `MidiSong` client packet after a PRELOADED-music lookup.

**Bundle rationale:** The sibling `MIDI_JINGLE` (2063) sits at `PlayerOps.ts:806-816` with the same pattern — pop + validate + lowMemory gate + `player.playJingle(delay, name)`. Both share identical pointer gate (`require: ['active_player']`), identical validator shape (`StringNotNull` + `NumberNotNull`), and identical downstream-deferral posture (no PRELOADED music infra in goscape). Bundling saves an S7i round-trip if the next re-stall is `2063`, and the two handlers review better together than apart (established cluster pattern from S7f / S7g).

## Tech stack

- Go 1.26+
- Existing packages: `pkg/script/`, `modules/world/`, `pkg/io/protocol/login/req/`

## Scope (B)

- Add `checkStringNotNull` validator to `pkg/script/handlers_player.go` (sibling to existing `checkNotNull`).
- Port two script handlers — `handleMidiSong` (2064) and `handleMidiJingle` (2063) — both in `pkg/script/handlers_player.go`, registered in `pkg/script/handlers.go`.
- Extend `ActivePlayer` interface (`pkg/script/active.go`) with `LowMemory()`, `PlaySong(name string)`, `PlayJingle(delay int, name string)`.
- Plumb `req.LowMemory` from the RS2 login request through `client` → `newPlayer` → `player.lowMemory` (three touch points).
- Add `(*Player).LowMemory()`, `(*Player).PlaySong`, `(*Player).PlayJingle` in `modules/world/player_script.go`. Player methods perform TS name-normalization and early-return-on-empty but make **no `writeOut` call** (S7h-D1).
- Mock impls in `pkg/script/runner_test.go` capturing call args + new `lowMemory` flag.
- Tests: validator units, handler dispatch + gate + bail paths, normalization positive-pins, packet-write absence-pins per `ts_asymmetry_dual_pin`.

## Explicitly out of scope

- **PRELOADED music / CRC registry port.** goscape lacks the asset-load pipeline for `.mid` files and their CRC table (zero `rg PRELOADED` hits in-tree at HEAD=7a1ef6a). Separate NAI-scoped sub-spec. Tracking tag in commit trailers: `NAI-16-midi-encoders`.
- **`OpMidiSong` / `OpMidiJingle` registration in `pkg/io/protocol/game/server/prot.go`.** Deliberately deferred with the encoder — declared-but-unused wire ops would trip the `dead_api_polish` review pattern. Landed together with the PRELOADED port.
- **`reconnecting` field plumbing.** Parallel-but-independent session-flag plumbing gap (the field is declared on `player.go:165`, read at `player.go:390/399`, never written in the login flow). Not widened into S7h. Belongs in a dedicated session-flags sub-spec alongside `webClient`.
- **`webClient` field plumbing.** Same posture as `reconnecting`; same deferred-note treatment.
- **Validator consolidation refactor.** `checkNotNull` (int) and new `checkStringNotNull` coexist without consolidation. TS has them as distinct `ScriptValidator<T,R>` implementations (`ScriptInputNumberNotNullValidator` at `ScriptValidators.ts:36`, `ScriptInputStringNotNullValidator` at `ScriptValidators.ts:50`); goscape mirrors that shape.
- **S7g `dbFind` / `dbFindRefine` dispatch refactor.** Reviewer-flagged as standalone polish in S7g close; does not belong in S7h since S7h does not touch `handlers_db.go`.

## Architecture

### Files created

*(None — all changes land in existing files.)*

### Files modified (production)

- `pkg/script/handlers_player.go` — new `checkStringNotNull`; new `handleMidiSong`; new `handleMidiJingle`.
- `pkg/script/handlers.go` — two dispatch rows (`OpMidiSong`, `OpMidiJingle`).
- `pkg/script/active.go` — three new methods on `ActivePlayer`: `LowMemory()`, `PlaySong`, `PlayJingle`.
- `modules/world/player_script.go` — three new `(*Player)` methods: `LowMemory`, `PlaySong`, `PlayJingle`.
- `modules/world/client.go` — new `lowMemory bool` field on `client` struct near the existing `staffModLevel` / `members` session flags (client.go:50-51).
- `modules/world/server.go` — single-line copy `c.lowMemory = req.LowMemory` right after the `req.UnmarshalBinary(b)` call at `server.go:470`.
- `modules/world/player.go` — single-line addition `lowMemory: c.lowMemory` inside the `newPlayer(c)` struct literal at `player.go:293`.

### Files modified (tests)

- `pkg/script/handlers_player_test.go` — new handler + validator tests.
- `pkg/script/runner_test.go` — extend `mockPlayer` with `lowMemory bool` field, `playSongCalls`, `playJingleCalls` slices; impl `LowMemory`, `PlaySong`, `PlayJingle`.
- `modules/world/player_script_test.go` — new `PlaySong`, `PlayJingle` normalization + absence-pin tests. (File exists or is created alongside `player_script.go` per existing convention.)
- `modules/world/player_test.go` — new `TestNewPlayerCopiesLowMemoryFromClient`.

### Touch-site map

```
req.LowMemory  ──►  c.lowMemory  ──►  p.lowMemory  ──►  (*Player).LowMemory()
  (login wire)      (client struct)   (newPlayer)        (ActivePlayer iface)
                                                                │
                                                                ▼
                            ┌──────────────────────────────────┐
                            │ handleMidiSong / handleMidiJingle │
                            │   pop + StringNotNull + PtrAP    │
                            │   gate + call Play{Song,Jingle}   │
                            └──────────────────────────────────┘
                                            │
                                            ▼
                                (*Player).PlaySong / .PlayJingle
                                  normalize name + early-return
                                  [deferred: writeOut(OpMidi*)]
```

## Validator: `checkStringNotNull`

New helper in `pkg/script/handlers_player.go`, placed immediately after `checkNotNull` at :61:

```go
// checkStringNotNull mirrors TS StringNotNull
// (ScriptInputStringNotNullValidator at ScriptValidators.ts:50-55) — rejects
// empty strings, accepts any non-empty string. Used by handlers wrapping a
// popString result with TS check(..., StringNotNull). TS error literal:
// "An input string was null(-1)."
func checkStringNotNull(v, op string) error {
    if v == "" {
        return fmt.Errorf("%s: input string was null", op)
    }
    return nil
}
```

TS reference (`ScriptValidators.ts:50-55`):

```ts
class ScriptInputStringNotNullValidator implements ScriptValidator<string, string> {
    validate(input: string): string {
        if (input.length > 0) return input;
        throw Error('An input string was null(-1).');
    }
}
```

Error message in goscape drops the `(-1)` suffix (the `-1` literal is a number-sentinel artifact; strings don't have a `-1` sentinel — the sentinel is the empty string) while keeping the `"input string was null"` phrase for grepability parity with `checkNotNull`'s error string.

## Handlers

Both land in `pkg/script/handlers_player.go` adjacent to the existing `handlePAnimProtect`.

### `handleMidiSong` (opcode 2064)

```go
// handleMidiSong (MIDI_SONG, opcode 2064) plays a MIDI song by name to the
// active player. Silent no-op if the player has lowMemory set. Mirrors TS
// PlayerOps.ts:796-804.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts:272
// require: ['active_player']).
//
// S7h-D1: downstream (*Player).PlaySong currently performs TS name
// normalization + early-return only; no MidiSong client packet is sent.
func handleMidiSong(s *ScriptState) error {
    name := s.PopString()
    if err := checkStringNotNull(name, "MIDI_SONG"); err != nil {
        return err
    }
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("MIDI_SONG: no active player")
    }
    if s.Self.LowMemory() {
        return nil
    }
    s.Self.PlaySong(name)
    return nil
}
```

TS reference (`PlayerOps.ts:796-804`):

```ts
[ScriptOpcode.MIDI_SONG]: state => {
    const name = check(state.popString(), StringNotNull);
    const player = state.activePlayer;
    if (player.lowMemory) {
        return;
    }
    player.playSong(name);
},
```

### `handleMidiJingle` (opcode 2063)

```go
// handleMidiJingle (MIDI_JINGLE, opcode 2063) plays a short MIDI jingle by
// name and delay to the active player. Silent no-op if the player has
// lowMemory set. Mirrors TS PlayerOps.ts:806-816.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts:269
// require: ['active_player']).
//
// Pop order (top-of-stack first): delay (NumberNotNull), then name
// (StringNotNull). Matches TS `check(state.popInt(), NumberNotNull)` /
// `check(state.popString(), StringNotNull)` evaluation order.
//
// S7h-D1: downstream (*Player).PlayJingle currently performs TS name
// normalization + early-return only; no MidiJingle client packet is sent.
func handleMidiJingle(s *ScriptState) error {
    delay := s.PopInt()
    if err := checkNotNull(delay, "MIDI_JINGLE"); err != nil {
        return err
    }
    name := s.PopString()
    if err := checkStringNotNull(name, "MIDI_JINGLE"); err != nil {
        return err
    }
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("MIDI_JINGLE: no active player")
    }
    if s.Self.LowMemory() {
        return nil
    }
    s.Self.PlayJingle(delay, name)
    return nil
}
```

TS reference (`PlayerOps.ts:806-816`):

```ts
[ScriptOpcode.MIDI_JINGLE]: state => {
    const delay = check(state.popInt(), NumberNotNull);
    const name = check(state.popString(), StringNotNull);
    const player = state.activePlayer;
    if (player.lowMemory) {
        return;
    }
    player.playJingle(delay, name);
},
```

### Dispatch registration (`pkg/script/handlers.go`)

Two new rows added to the handler map (sorted by opcode numeric value):

```go
OpMidiJingle: handleMidiJingle,
OpMidiSong:   handleMidiSong,
```

## `ActivePlayer` interface extensions (`pkg/script/active.go`)

Three new methods:

```go
// LowMemory reports whether the player's client requested low-memory
// mode at login (carried on the RS2 login request's LowMemory bit).
// Script opcodes that trigger client audio loads gate on this flag —
// see handleMidiSong / handleMidiJingle in handlers_player.go.
LowMemory() bool

// PlaySong sends a MIDI song by name to the client. Called by the
// MIDI_SONG script opcode (PlayerOps.ts:796-804).
//
// S7h-D1: actual MidiSong client packet is deferred pending PRELOADED
// music + CRC infrastructure; current impl performs TS name
// normalization (lowercase + spaces→underscores) and early-returns
// on empty without writing.
PlaySong(name string)

// PlayJingle sends a short MIDI jingle by name to the client. Called
// by the MIDI_JINGLE script opcode (PlayerOps.ts:806-816).
//
// S7h-D1: actual MidiJingle client packet is deferred pending
// PRELOADED music infrastructure; current impl performs TS name
// normalization (lowercase + underscores→spaces) and early-returns
// on empty without writing.
PlayJingle(delay int, name string)
```

## Player methods (`modules/world/player_script.go`)

### `LowMemory`

```go
// LowMemory returns the player's low-memory flag as plumbed from the
// RS2 login request (req.LowMemory) through client.lowMemory and
// copied onto the Player at newPlayer().
func (p *Player) LowMemory() bool { return p.lowMemory }
```

### `PlaySong`

**Normalization extracted.** Because `PlaySong` has no observable output in S7h (no `writeOut` call per S7h-D1), the name-normalization step is extracted into an unexported package-level helper so positive direction-pin tests have a concrete observation point. When NAI-16 lands, the helper stays and the PRELOADED+encoder path layers on top of it unchanged.

```go
// normalizeSongName mirrors TS Player.playSong's normalization step
// (Engine-TS/src/engine/entity/Player.ts:1903) — lowercase + spaces
// replaced by underscores. Extracted for direct testability given
// PlaySong's current no-op write body (S7h-D1). Asymmetric with
// normalizeJingleName (spaces→underscores vs. underscores→spaces);
// the asymmetry is TS-intentional — see package-level preamble.
func normalizeSongName(name string) string {
    return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

// PlaySong normalizes the song name per TS Player.playSong
// (Engine-TS/src/engine/entity/Player.ts:1902-1914) and early-returns
// on empty.
//
// S7h-D1: the subsequent TS PRELOADED + PRELOADED_CRC lookup and
// MidiSong(name, crc, length) write from TS is not yet ported.
// goscape lacks the PRELOADED music registry (zero rg hits at
// HEAD=7a1ef6a). No client packet is sent. The
// TestPlaySongNoWriteOut absence-pin (player_script_test.go) escalates
// this deviation when the write path is wired; retirement tracked as
// NAI-16-midi-encoders.
func (p *Player) PlaySong(name string) {
    name = normalizeSongName(name)
    if name == "" {
        return
    }
    // deferred (S7h-D1): PRELOADED lookup + p.writeOut(gameserver.OpMidiSong, ...)
}
```

TS reference (`Player.ts:1902-1914`):

```ts
playSong(name: string) {
    name = name.toLowerCase().replaceAll(' ', '_');
    if (!name) {
        return;
    }

    const song = PRELOADED.get(name + '.mid');
    const crc = PRELOADED_CRC.get(name + '.mid');
    if (song && crc) {
        const length = song.length;
        this.write(new MidiSong(name, crc, length));
    }
}
```

### `PlayJingle`

Normalization extracted symmetrically with `normalizeSongName`:

```go
// normalizeJingleName mirrors TS Player.playJingle's normalization step
// (Engine-TS/src/engine/entity/Player.ts:1917) — lowercase + underscores
// replaced by spaces. Extracted for direct testability given
// PlayJingle's current no-op write body (S7h-D1). Asymmetric with
// normalizeSongName (underscores→spaces vs. spaces→underscores);
// the asymmetry is TS-intentional — see package-level preamble.
func normalizeJingleName(name string) string {
    return strings.ReplaceAll(strings.ToLower(name), "_", " ")
}

// PlayJingle normalizes the jingle name per TS Player.playJingle
// (Engine-TS/src/engine/entity/Player.ts:1916-1926) and early-returns
// on empty.
//
// S7h-D1: the subsequent TS PRELOADED lookup and MidiJingle(delay, data)
// write from TS is not yet ported. No client packet is sent. The
// TestPlayJingleNoWriteOut absence-pin (player_script_test.go)
// escalates this deviation when the write path is wired; retirement
// tracked as NAI-16-midi-encoders.
func (p *Player) PlayJingle(delay int, name string) {
    _ = delay // preserved for future MidiJingle encoder wiring
    name = normalizeJingleName(name)
    if name == "" {
        return
    }
    // deferred (S7h-D1): PRELOADED lookup + p.writeOut(gameserver.OpMidiJingle, ...)
}
```

TS reference (`Player.ts:1916-1926`):

```ts
playJingle(delay: number, name: string): void {
    name = name.toLowerCase().replaceAll('_', ' ');
    if (!name) {
        return;
    }

    const jingle = PRELOADED.get(name + '.mid');
    if (jingle) {
        this.write(new MidiJingle(delay, jingle));
    }
}
```

**Dual normalization asymmetry:** TS song-path replaces `' '` → `'_'`; jingle-path replaces `'_'` → `' '`. The asymmetry is upstream-intentional (songs key into disk with underscore filenames; jingles key into a space-separated title map). Preserved exactly in the Go port per `true_to_ts_gate`. Positive-pin tests (see below) fail if either direction flips.

## `lowMemory` plumbing

Three touch points, all single-line additions.

### `modules/world/client.go` (near :50-51, alongside `staffModLevel` and `members`)

```go
// lowMemory carries the client's low-memory capability bit from the
// RS2 login packet (LoginRequest.LowMemory, parsed at server.go's
// req.UnmarshalBinary). Copied onto Player at newPlayer(). Read by
// script opcodes that trigger client audio loads (MIDI_SONG, MIDI_JINGLE).
lowMemory bool
```

### `modules/world/server.go` (right after :470 `req.UnmarshalBinary(b)`)

```go
c.lowMemory = req.LowMemory
```

Placement rationale: `lowMemory` is a client-capability flag carried on every login request regardless of accept/reject outcome, so set it alongside the earliest login-parse step — not alongside the conditional `c.staffModLevel` / `c.members` assignments at :541-542 (those come from the RPC *response* after a successful RPC).

### `modules/world/player.go` (in `newPlayer(c)` struct literal at :293+)

```go
p := &Player{
    client: c,
    // ... existing fields ...
    lowMemory: c.lowMemory,
    // ... existing fields ...
}
```

Placement: with the other bool-valued session flags (`reconnecting`, `webClient`, etc.) in the struct-literal ordering. Concrete line picked by the implementer subagent based on existing literal ordering at write time.

## Test strategy

### Handler tests (`pkg/script/handlers_player_test.go`)

Validator units:
- `TestCheckStringNotNull_Empty` — returns error wrapping `"<op>: input string was null"`.
- `TestCheckStringNotNull_NonEmpty` — returns nil.

`MIDI_SONG` dispatch:
- `TestMidiSongHappyPath` — mock player with `lowMemory=false`, stack has `"harmony1"`. After dispatch: `mockPlayer.playSongCalls == [{name:"harmony1"}]`, no error.
- `TestMidiSongLowMemoryBails` — mock player with `lowMemory=true`, stack has `"harmony1"`. After dispatch: `len(mockPlayer.playSongCalls) == 0`, no error.
- `TestMidiSongNullStringRejects` — stack has `""`. Error matches `MIDI_SONG: input string was null`.
- `TestMidiSongNoActivePlayerRejects` — `PtrActivePlayer` cleared OR `s.Self == nil`. Error matches `MIDI_SONG: no active player`.

`MIDI_JINGLE` dispatch (symmetric + one extra):
- `TestMidiJingleHappyPath` — stack `[delay=3, name="fanfare"]`. After dispatch: `mockPlayer.playJingleCalls == [{delay:3, name:"fanfare"}]`, no error.
- `TestMidiJingleLowMemoryBails` — `lowMemory=true`. Zero call, no error.
- `TestMidiJingleNullStringRejects` — stack `[delay=3, name=""]`. Error wraps `MIDI_JINGLE: input string was null`.
- `TestMidiJingleNullDelayRejects` — stack `[delay=-1, name="fanfare"]`. Error wraps `MIDI_JINGLE: input number was null(-1)` (from existing `checkNotNull`).
- `TestMidiJingleNoActivePlayerRejects` — `PtrActivePlayer` cleared. Error matches `MIDI_JINGLE: no active player`.

### Player-method tests (`modules/world/player_script_test.go`)

Positive pins (TS normalization direction — tests target the extracted helpers directly, not the enclosing methods):
- `TestNormalizeSongName_LowercaseAndSpacesToUnderscores` — `normalizeSongName("Harmony 1") == "harmony_1"`; plus `"already_lower"` → `"already_lower"` (idempotency on already-normalized input); plus `"ALLCAPS"` → `"allcaps"`.
- `TestNormalizeJingleName_LowercaseAndUnderscoresToSpaces` — `normalizeJingleName("a_quick_jingle") == "a quick jingle"`; plus `"Space Already"` → `"space already"` (confirms spaces left untouched).
- `TestNormalizeSongName_EmptyReturnsEmpty` / `TestNormalizeJingleName_EmptyReturnsEmpty` — feed `""`, expect `""` (documents the sentinel that feeds `PlaySong`/`PlayJingle`'s empty-check).

Early-return pins:
- `TestPlaySongEmptyNameReturnsSilently` — input `""`. No panic; zero post-normalize effect. Paired with absence-pin below.
- `TestPlayJingleEmptyNameReturnsSilently` — input `""`. No panic.

**Absence pins** (per `ts_asymmetry_dual_pin`):
- `TestPlaySongNoWriteOut` — call `p.PlaySong("harmony1")` on a player with a `writeOut` recorder. Assert `len(writeOutCalls) == 0`. Preamble comment: *"Fails when PRELOADED music encoder ports and wires writeOut — signal to retire S7h-D1; replace with positive write-pin at that time."*
- `TestPlayJingleNoWriteOut` — parallel absence-pin for jingle.

### Plumbing test (`modules/world/player_test.go`)

- `TestNewPlayerCopiesLowMemoryFromClient` — construct a `client` with `lowMemory=true`, call `newPlayer(c)`, assert `p.lowMemory == true`. Mirror test with `lowMemory=false` to confirm default path.

### Mock extension (`pkg/script/runner_test.go`)

`mockPlayer` gains:

```go
lowMemory       bool
playSongCalls   []struct{ name string }
playJingleCalls []struct{ delay int; name string }
```

```go
func (m *mockPlayer) LowMemory() bool { return m.lowMemory }
func (m *mockPlayer) PlaySong(name string) {
    m.playSongCalls = append(m.playSongCalls, struct{ name string }{name})
}
func (m *mockPlayer) PlayJingle(delay int, name string) {
    m.playJingleCalls = append(m.playJingleCalls, struct{ delay int; name string }{delay, name})
}
```

### Coverage crosscheck (per `plan_test_coverage_crosscheck` memory)

Every test named above maps 1:1 to a code block in the plan doc's tasks. Plan-write step will diff this list against the per-task checklists and reject any implementer-side omissions.

## Deviations

| Tag | Description | Rationale | Follow-up |
|---|---|---|---|
| **S7h-D1** | `(*Player).PlaySong` and `(*Player).PlayJingle` perform TS name-normalization and early-return-on-empty but make **no `writeOut` call**. Script handlers remain fully TS-faithful (dispatch, pointer gate, `StringNotNull`/`NumberNotNull` validation, `lowMemory` bail, method invocation). | goscape lacks the PRELOADED music / CRC registry required for TS's `MidiSong(name, crc, length)` and `MidiJingle(delay, data)` packet bodies. Porting the registry is a separate, substantial asset-pipeline effort out of S7h scope. Placeholder crc/length would send malformed packets. `OpMidiSong`/`OpMidiJingle` deliberately not registered in `prot.go` to avoid dead-API wire ops. | Retire when PRELOADED port lands — tracked as **NAI-16-midi-encoders**. `TestPlaySongNoWriteOut` / `TestPlayJingleNoWriteOut` absence-pins escalate loudly when the write path is wired. |

**Pre-existing deviations carried forward:** S7a-D1, S7a-D2, S7b-D1, S7c-D1, S7d-D1, S7d-D2, S7d-D3, S7d-D4, S7e-D1, S7f-D1, S7f-D2, S7f-D3, S7g-D1, S7g-D2, S7g-D3.

**Active count after S7h close:** 15 (carried) + 1 (new) = **16**.

## Adjacent deferred work (not S7h)

- `reconnecting` field plumbing (`player.go:165`, read at :390/:399, never written in login). Session-flags sub-spec.
- `webClient` field plumbing (`player.go:165`). Same session-flags sub-spec.
- `MidiSongEncoder` / `MidiJingleEncoder` port + `OpMidiSong`/`OpMidiJingle` in `prot.go`. NAI-16-midi-encoders.
- S7g `dbFind`/`dbFindRefine` dispatch refactor (flag pair over string-compare dispatch) — reviewer polish from S7g close; standalone commit when a contributor is in `handlers_db.go` again.

## Acceptance gates

Per `verify_implementer_claims`, the combined-review step runs these fresh, independently of any subagent-claimed results.

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` — clean.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — all green (fresh run, whole-module scope; not relying on implementer's per-package claim).
3. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — clean.
4. `rg -n "OpMidiSong|OpMidiJingle" pkg/script/handlers.go` — both opcode dispatches wired.
5. `rg -n "lowMemory" modules/world/ pkg/script/` — plumbing sites are exactly `client.go` (field), `server.go` (assignment after UnmarshalBinary), `player.go` (newPlayer literal), `player_script.go` (`LowMemory()` method), `active.go` (interface decl), `runner_test.go` (mock), `handlers_player.go` (handler usage), `player_test.go` (new plumbing test), + handler tests. No stragglers.
6. **Absence-pin sanity:** no `p.writeOut` call inside the bodies of `(*Player).PlaySong` or `(*Player).PlayJingle`. Verified by visual diff inspection of `modules/world/player_script.go` (the two new method bodies). `rg -n "writeOut.*OpMidiSong|writeOut.*OpMidiJingle" modules/` must additionally return **zero** — also catches accidental opcode-adjacent write in any sibling file.
7. **prot.go untouched:** `git diff main...HEAD -- pkg/io/protocol/game/server/prot.go` must be empty.
8. **Struct-literal enumeration** per `plan_enumerate_struct_literals`: plan-write step greps every `Player{...}` struct literal in tests (currently two minimal sites at `server_test.go:359,449` from probe data) and confirms they do not need parallel `lowMemory:` additions (both exist only for `slot` testing; `lowMemory` default-zero is fine). Implementer revalidates at task close.

## Rollout / smoke-test sequencing

Per `smoke_test_server_handoff`, the Java-client smoke requires a user-launched server. After S7h close commit:

- User runs `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml` and exercises `[label,music_playbyregion]`.
- **Expected primary outcome:** script advances past `pc=74` (MIDI_SONG) and past any MIDI_JINGLE opcode hit; logs do not contain `no handler for MIDI_SONG` or `no handler for MIDI_JINGLE`.
- **Secondary watch (carry-forward from S7d):** whether `combat_get_damagetype` finally exercises `DB_GETFIELD` cleanly now that the DB_FIND family (S7g) and MIDI cluster (S7h) are unblocked.
- **Next-stall handoff:** if a new opcode stalls the script, it becomes S7i. If the script completes without further stall, S7h closes the `[label,music_playbyregion]` thread opened in S7f.

## Cadence

- **Size bracket:** ~35-50 LOC production code + ~80-120 LOC tests → 15-100 LOC bracket per `compressed_cadence`.
- **Cadence:** spec doc + plan doc (separate `docs(spec):` / `docs(plan):` commits), subagent-driven execution per task, **single combined review** after all implementation tasks land (not per-task two-stage).
- **Close commit:** conventional `chore(script): S7h closed — MIDI_SONG + MIDI_JINGLE handlers + lowMemory plumbing`. Include `Closes memory: nai_followups` trailer per `close_commit_memory_trailer` to make the NAI-16 breadcrumb grep-discoverable from `git log`.
- **Execution mode:** subagent-driven-development per `execution_mode_default` — dispatch fresh subagent per task; no interactive menu.
