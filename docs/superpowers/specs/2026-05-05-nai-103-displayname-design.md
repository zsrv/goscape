## NAI-103: DISPLAYNAME opcode 2016 + Player.displayName plumbing

**Date**: 2026-05-05
**Cadence**: combined spec + plan, single end-of-impl review (per
`compressed_cadence.md`, ≤15 production-LOC threshold; ~12 production
LOC).
**Predecessor**: NAI-102 (HEAD `d836dac` — TUT_CLOSE handler +
`Player.closeTutorial()` port).
**Trigger**: NAI-95 smoke residual (nai_followups.md "From NAI-95"):
`[proc,chatplayer_page]` script crashes on DISPLAYNAME at pc=24
immediately after the first NPC chat dialog opens, surfacing as a
WARN-level script execute error in `modules/world/script.go:112`.
**Tech stack**: Go 1.26+ (per `go_version.md`).
**Successor**: TBD; no residuals expected (single-handler port + 1-LOC
construction-time field assignment).

### 1. Problem

`OpDisplayName Opcode = 2016` is declared at `pkg/script/opcode.go:116`
and named at `pkg/script/opcode.go:639-640`, but the dispatch table at
`pkg/script/handlers.go` carries no entry — the runner falls through
to the "unhandled opcode" path, surfacing as the WARN observed in
NAI-95 smoke. The matching handler `handleDisplayName` is absent from
`pkg/script/handlers_player.go`.

Compounding: `Player.displayName` is declared as a `string` field at
`modules/world/player.go:73` but **never assigned anywhere**
(`rg "displayName" modules/world/ pkg/` returns only the field decl).
Production reads would push the empty string even after the handler
is wired. TS sets the field at `Player` constructor time
(`Player.ts:417`).

The `pkg/util/jstring.ToDisplayName(s string) string` helper is
already ported (`pkg/util/jstring/jstring.go:66-68`); no new util
work needed.

### 2. TS source (canonical, single read)

**`Engine-TS/src/engine/script/handlers/PlayerOps.ts:235-237`** — handler:

```typescript
[ScriptOpcode.DISPLAYNAME]: checkedHandler(ActivePlayer, state => {
    state.pushString(state.activePlayer.displayName);
}),
```

**`Engine-TS/src/engine/entity/Player.ts:414-417`** — constructor:

```typescript
this.username = username;
this.username37 = username37;
this.hash64 = hash64;
this.displayName = toDisplayName(username);
```

**`Engine-TS/src/engine/script/ScriptOpcodePointers.ts:95-98`** — pointer config:

```typescript
[ScriptOpcode.DISPLAYNAME]: {
    require: ['active_player'],
    require2: ['active_player2']
},
```

**Observations driving the port:**

- `checkedHandler(ActivePlayer, ...)` is a single-pointer guard
  (`ScriptPointer.ts:47-56`); the handler reads `state.activePlayer`
  directly. Goscape precedent: SOUND_SYNTH (NAI-87) at
  `pkg/script/handlers_player.go:950-962` uses `requireActivePlayer`
  + `s.Self.PlaySynth(...)`. **One handler, no `Self2` variant
  needed.** The `require2: ['active_player2']` entry covers the
  `.player2` opcode-prefix dispatch which goscape's runtime already
  swaps via `Pointers + Self/Self2` (no per-handler change).
- `toDisplayName(username)` runs `ToTitleCase(ToSafeName(username))`
  then replaces `_` with ` ` (per `pkg/util/jstring/jstring.go:66-68`);
  e.g. `"alice_smith"` → `"Alice Smith"`. The base37 round-trip in
  `ToSafeName` is the same one already exercised by NAI-32
  Bundle 3's `username37` wiring (`login_username_test.go:25-31`).

### 3. Solution

#### 3.1 Production changes

**(P1)** `modules/world/player.go:433` — populate `displayName` at
construction, sited between `username` and `username37`. The file
already imports `util "github.com/zsrv/goscape/pkg/util/jstring"` at
line 18; use that existing alias:

```go
username:       c.username,
displayName:    util.ToDisplayName(c.username),
username37:     util.ToBase37(c.username),
```

No new import needed.

**(P2)** `modules/world/message_game.go` — add `DisplayName()` getter
adjacent to existing `Username()` at line 22:

```go
// DisplayName returns the player's titlecased, underscore-replaced
// display name. Used by the DISPLAYNAME script opcode. Set once at
// newPlayer construction (TS Player.ts:417).
func (p *Player) DisplayName() string {
    return p.displayName
}
```

**(P3)** `pkg/script/active.go:8` — extend `ActivePlayer` interface,
adding the new method immediately under `Username() string`:

```go
type ActivePlayer interface {
    MessageGame(msg string)
    Username() string
    DisplayName() string
    // ... existing entries ...
```

**(P4)** `pkg/script/handlers_player.go` — add `handleDisplayName`
sited between `handleCoord` (line 522-528) and `handleFaceSquare`
(line 530+). DISPLAYNAME's opcode (2016) sits between OpCoord (2014)
and OpFaceSquare (2017) numerically; OpDamage (2015) has no handler
so this is the natural neighbour:

```go
// handleDisplayName (DISPLAYNAME, opcode 2016) pushes the active
// player's display name. Mirrors TS PlayerOps.ts:235-237.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts
// :95-98 require: ['active_player']).
func handleDisplayName(s *ScriptState) error {
    if err := requireActivePlayer(s, "DISPLAYNAME"); err != nil {
        return err
    }
    s.PushString(s.Self.DisplayName())
    return nil
}
```

**(P5)** `pkg/script/handlers.go:203` — add dispatch entry adjacent
to `OpCoord`:

```go
// Coord / facing / teleport.
OpCoord:       handleCoord,
OpDisplayName: handleDisplayName,
OpFaceSquare:  handleFaceSquare,
```

(The "Coord / facing / teleport" comment block stays — DISPLAYNAME's
locality is by opcode number, not theme; if the implementer prefers
a separate "Identity" comment block above OpCoord, that's an
acceptable cosmetic variation.)

**(P6)** `pkg/script/runner_test.go` — extend `mockPlayer`:

- Add field `displayName string` adjacent to `username` (struct decl
  at line 99).
- Implement getter adjacent to `Username()` at line 332:

```go
func (m *mockPlayer) DisplayName() string { return m.displayName }
```

#### 3.2 Test changes

**(T1)** `pkg/script/handlers_player_test.go` — add two tests adjacent
to the `TestSoundSynth*` family (line 3172-3245), mirroring its shape:

- `TestDisplayNameHappyPath` — `&mockPlayer{displayName: "Alice Smith"}`,
  `Pointers: PtrActivePlayer`; call `handleDisplayName(s)`; assert
  `err == nil` and the top-of-stack string equals `"Alice Smith"`
  (use `s.PopString()` post-call, mirroring the existing string-stack
  read pattern from `TestHandlePushPopStringLocal` at
  `handlers_test.go:79`).
- `TestDisplayNameNoActivePlayerRejects` — `Self: nil, Pointers: 0`;
  expect `err != nil` containing `"DISPLAYNAME: no active player"`,
  mirroring `TestSoundSynthNoActivePlayerRejects` shape exactly.

**(T2)** `modules/world/login_username_test.go` — extend the existing
`TestNewPlayer_PopulatesUsernameFields` test (or add a sibling
`TestNewPlayer_PopulatesDisplayName`) pinning the construction-time
`displayName` assignment. Sibling-test variant (cleaner separation):

```go
// TestNewPlayer_PopulatesDisplayName pins the wiring from the login
// flow to Player.displayName: when a client logs in with a username,
// newPlayer must populate p.displayName with util.ToDisplayName
// (titlecased, underscore-replaced). Used by the DISPLAYNAME script
// opcode (TS Player.ts:417). NAI-103.
func TestNewPlayer_PopulatesDisplayName(t *testing.T) {
    c := &client{username: "alice_smith"}
    p := newPlayer(c)

    want := util.ToDisplayName("alice_smith")
    if p.displayName != want {
        t.Errorf("p.displayName: got %q, want %q", p.displayName, want)
    }
    if p.DisplayName() != want {
        t.Errorf("p.DisplayName(): got %q, want %q", p.DisplayName(), want)
    }
}
```

Use `"alice_smith"` rather than `"alice"` so the underscore→space
substitution is exercised (rather than a no-op identity case).

### 4. Out of scope

- Re-running the chatplayer_page proc end-to-end as an automated
  integration test. The handler unit test + entity wiring test cover
  the engine-side port; in-content `chatplayer_page` is content-side
  data and re-running it is the user's opportunistic Tutorial Island
  smoke (see §8 below).
- DAMAGE opcode 2015 (also unhandled per `rg "OpDamage:"
  pkg/script/handlers.go` empty); separate sub-spec when surfaced.
- Refreshing `displayName` mid-session (e.g. on a future rename
  feature). TS only sets at construction; goscape mirrors. No
  rename infrastructure exists.
- Other deferred stubs (SPLIT_* font-aware wrap at
  `pkg/script/handlers_string.go:97`) — depend on FontType /
  MesanimType cache config loaders; separate sub-specs.

### 5. Deviations introduced

**None.** Full TS-faithful port.

### 6. Deviations retired

- **NAI-95 carry-forward "DISPLAYNAME opcode 2016 missing handler"**
  — retired by P4 (handler) + P5 (dispatch) + P1 (field assignment).
  Re-grep at impl time: `rg "OpDisplayName" pkg/script/handlers.go`
  → 1 match (the new dispatch entry). `rg "displayName" modules/world/`
  → 2+ matches (field decl + new construction-time write + new
  getter; pre-fix sentinel of "field decl only" cleared).

### 7. Implementation plan (subagent-driven, single bundle)

Single subagent dispatch covers all changes; compressed cadence skips
formal review.

**Bundle 1: DISPLAYNAME port (single dispatch)**

Tasks for the implementer (TDD per `superpowers:test-driven-development`):

1. **T1 (TDD, fail-compile)**: Write `TestDisplayNameHappyPath` +
   `TestDisplayNameNoActivePlayerRejects` in
   `pkg/script/handlers_player_test.go`. Both fail-compile
   (`mockPlayer.DisplayName` undefined; `handleDisplayName`
   undefined).

2. **T2 (RED→GREEN, mock + interface + handler)**: Add
   `displayName string` field to `mockPlayer` struct + `DisplayName()`
   method in `pkg/script/runner_test.go`. Add `DisplayName() string`
   to `ActivePlayer` interface in `pkg/script/active.go`. Add
   `handleDisplayName` in `pkg/script/handlers_player.go` (between
   `handleCoord` and `handleFaceSquare`). Register `OpDisplayName:
   handleDisplayName` in `pkg/script/handlers.go` (between `OpCoord`
   and `OpFaceSquare`). T1 tests turn green. Compile-check the entire
   `pkg/script/...` package — adding to the interface forces every
   `ActivePlayer` implementation to provide `DisplayName()`. The only
   non-mock implementation is `*Player` (modules/world); that
   compile failure is expected and is fixed in T4.

3. **T3 (TDD, fail-compile)**: Write `TestNewPlayer_PopulatesDisplayName`
   in `modules/world/login_username_test.go` (sibling of
   `TestNewPlayer_PopulatesUsernameFields`). Fails to compile
   (`p.DisplayName` undefined; `p.displayName` field write absent
   from newPlayer means `p.displayName == ""`).

4. **T4 (RED→GREEN, Player getter + newPlayer wiring)**: Add
   `(*Player).DisplayName()` getter in `modules/world/message_game.go`
   adjacent to `Username()`. Add `displayName: util.ToDisplayName(
   c.username),` in `newPlayer` at `modules/world/player.go:433`
   (between `username` and `username37`). The `util` alias is
   already imported at `player.go:18` (verified at spec-write —
   see R1 below). T3 turns green; T2's modules/world compile failure
   clears.

5. **T5 (verification)**: Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache
   go test ./...`. Run `rg "OpDisplayName" pkg/script/handlers.go`
   → exactly 1 match (the dispatch entry). Run
   `rg "displayName" modules/world/` → expect 3 matches: field decl
   in `player.go`, write in `newPlayer`, getter in `message_game.go`.
   Run `rg "DisplayName\(\)" pkg/script/active.go modules/world/
   pkg/script/runner_test.go` → expect interface decl + Player
   getter + mockPlayer getter (3 matches).

6. **T6 (close commit)**: Single chore(close) commit with body listing
   retired deviation (NAI-95 carry-forward) and `Closes memory:`
   trailer. Per `close_commit_memory_trailer.md`, no NAI-103-specific
   memory entry exists to retire (this is a one-shot port, not a
   tracker-bound cascade); the trailer can read `(none)` or be
   omitted.

### 8. Risk register

- **R1 — `jstring` import already in `modules/world/player.go`?**
  [GREEN, pre-flighted at spec-write]. `rg "pkg/util/jstring"
  modules/world/player.go` confirms `util "github.com/zsrv/goscape/pkg/util/jstring"`
  at line 18. P1 reuses this `util` alias (matching the existing
  `util.ToBase37(c.username)` call site one line below). No new
  import needed.

- **R2 — `ActivePlayer` interface has implementations beyond
  `*Player` and `*mockPlayer`?** [GREEN, pre-flighted at spec-write].
  `rg "func \(\w+ \*\w+\) Username\(\) string" pkg/ modules/`
  returns exactly two matches: `pkg/script/runner_test.go:332`
  (`*mockPlayer`) and `modules/world/message_game.go:22` (`*Player`).
  No third implementer. If one surfaces during T2 compile, add
  `DisplayName() string` to it immediately — failure mode is a
  compile error, not a silent bug.

- **R3 — Wire-side regression on existing username paths?** [GREEN].
  This sub-spec only adds a new field assignment in `newPlayer`; it
  does not touch any of the existing `c.username`, `username37`,
  `safeName`, or `displayName-emitting` wire packets (`displayName`
  is not currently encoded into any wire output — `rg "p\.displayName"`
  returns only this sub-spec's new sites post-impl). Zero
  blast-radius outside the script-engine path.

- **R4 — `chatplayer_page` script binding** [INFO]. Current smoke
  matrix doesn't run automated content scripts; the
  `[proc,chatplayer_page]` WARN was a passive log observation in
  NAI-95's manual smoke. NAI-103 close gates on `go test ./...` +
  unit-level coverage. User opportunistically re-smokes Tutorial
  Island at next sit-down to confirm the chatplayer_page WARN at
  pc=24 no longer surfaces.

### 9. Verification before close

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean.
- `rg "OpDisplayName" pkg/script/handlers.go` → 1 match (dispatch).
- `rg "displayName" modules/world/` → 3 matches (field, write,
  getter).
- `rg "DisplayName" pkg/script/ modules/world/` → interface decl
  (active.go), `*Player` getter (message_game.go), `*mockPlayer`
  getter (runner_test.go), handler (handlers_player.go), 2 tests
  (handlers_player_test.go), 1 test (login_username_test.go).
- `git show HEAD --stat` matches stated bundle scope; no stray
  worktree writes (per `feedback_subagent_wt_path.md`).

### 10. Notes

This is a textbook compressed-cadence sub-spec: ~12 production LOC,
no novel infrastructure, no new content surfaces, no smoke gate
(opportunistic Tutorial Island re-smoke binds the production-side
chatplayer_page disappearance). The TS source is 4 lines (1 handler
+ 1 constructor write + 4 lines of pointer config) and the goscape
port is ~12 lines (1 handler + 1 newPlayer field + 1 interface
method + 1 Player getter + 1 dispatch entry + 1 mockPlayer field +
1 mockPlayer getter). Fidelity is verified by the
`Self.Username()` / `Self.DisplayName()` interface parity (each
identity getter has its TS counterpart; DISPLAYNAME finally joins).
