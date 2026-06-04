# S7i — session-flags plumbing (`reconnecting`)

- **Sub-spec**: S7i
- **Date**: 2026-04-24
- **Scope label**: A (compressed cadence — combined spec+plan; ≤25 LOC delta; no formal review gate)
- **Predecessors**: S7h (MIDI_SONG/JINGLE + `lowMemory` plumbing) — last on `main` as `327dc6a`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

S7h plumbed `req.LowMemory` from the wire packet through `client.lowMemory` → `newPlayer{lowMemory:}` → `Player.lowMemory`. Two sibling Player fields remain declared-but-unwritten at `modules/world/player.go:165`:

```go
reconnecting, lowMemory, webClient           bool
```

`p.reconnecting` is **read** at `player.go:391` (`p.buildArea.ShouldRebuild(p.x, p.z, p.reconnecting)`) and **reset** at `:400` (`p.reconnecting = false`), but never assigned a non-default value because no login-flow code writes the field. The build-area rebuild branch is therefore dead in goscape today: actual reconnects look identical to fresh logins. Plumbing closes that latent gap.

`p.webClient` has zero readers in goscape AND zero assignments in TS (Player.ts:312 declares `webClient: boolean = false` and **no other line touches it** in the entire Engine-TS source — verified by `grep -rn "webClient" Engine-TS/src/`). It is a true zombie field on both sides. Per the project's true-to-TS gate, the goscape default-`false` is already correct; inventing a writer here would be a divergence, not a fix. Dropping from S7i scope per the dead_api_polish memory pattern.

## TS evidence

`reconnecting` is opcode-derived in TS, not a wire field. World.ts:2212:

```ts
reconnecting: client.opcode === 18,
```

— exactly the shape goscape already computes at `modules/world/server.go:510`:

```go
reconnecting := opcode[0] == loginreq.OpReqGameReconnect.Opcode
```

(opcode 18 = `OpReqGameReconnect`; opcode 16 = `OpReqInitGameConnection`.) The local var is currently used only for the gRPC `PlayerLoginRequest` and a log line. It must additionally be lifted onto the `client` struct for `newPlayer` to copy.

World.ts:1894 then does `player.reconnecting = reconnecting;`, which is the assignment goscape is missing.

## Tech stack

- Go 1.26+
- Touched packages: `modules/world/`

## Scope

In-scope:
- Add `reconnecting bool` field to `client` struct in `modules/world/client.go` next to `lowMemory`.
- Assign `c.reconnecting = reconnecting` at `modules/world/server.go:511` (immediately after the existing local-var line).
- Add `reconnecting: c.reconnecting,` to the `newPlayer` struct literal in `modules/world/player.go:294-297` block.
- Test coverage parallels `TestNewPlayerCopiesLowMemoryFromClient` (player_test.go:480-506): both branches (`true` and `false`).

Explicitly out of scope:
- `webClient` field — verified zombie in TS source; goscape default-`false` already faithful; nothing to plumb. (See Motivation.)
- `ActivePlayer` interface getter `Reconnecting()` — no script handler currently calls it; YAGNI per dead_api_polish.
- `(*Player).Reconnecting()` Go method — same YAGNI rationale.

## Implementation (single Task)

### Task 1 — plumb `reconnecting` from login opcode through `client` to `Player`

**Files:** `modules/world/client.go`, `modules/world/server.go`, `modules/world/player.go`, `modules/world/player_test.go`.

**`modules/world/client.go`** — add field next to `lowMemory`:

```go
// reconnecting carries whether the client used OpReqGameReconnect (opcode 18)
// vs. OpReqInitGameConnection (opcode 16). Set at server.go's login-opcode
// branch. Copied onto Player at newPlayer(). Read by buildArea.ShouldRebuild
// to skip a full rebuild on actual reconnects.
reconnecting bool
```

**`modules/world/server.go:510-511`** — keep the local for downstream uses (gRPC + log) and assign to client:

```go
reconnecting := opcode[0] == loginreq.OpReqGameReconnect.Opcode
c.reconnecting = reconnecting
```

**`modules/world/player.go:293-297`** — add to `newPlayer` literal alongside `lowMemory`:

```go
p := &Player{
    client:         c,
    reconnecting:   c.reconnecting,
    lowMemory:      c.lowMemory,
    ...
}
```

**`modules/world/player_test.go`** — add `TestNewPlayerCopiesReconnectingFromClient` mirroring `TestNewPlayerCopiesLowMemoryFromClient` (both branches).

### Test plan

- `TestNewPlayerCopiesReconnectingFromClient` — two cases:
  1. `c.reconnecting = true` → `p.reconnecting == true`
  2. default (`false`) → `p.reconnecting == false`

The existing `BuildArea.ShouldRebuild` test surface already exercises the consume side of the field; no new test needed there.

### Verification

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
```

All three must be green before commit.

## Deviations

None expected. No new entries to the deviation count (carry: 16 → 16).

## Cadence call

Compressed cadence per the `compressed_cadence` memory: estimated ~6-8 prod LOC + ~25 LOC test = ~31 LOC. Sub-30-LOC threshold is borderline but the change is purely additive copy-through plumbing in an established pattern (S7h Task 2 template); skipping formal review is justified. If discovery during impl reveals scope creep (e.g., script-side getter need), promote to standard cadence and pause for review.

## Commits

Per the user's brief:

1. `docs(spec): S7i session-flags plumbing (reconnecting + webClient)` — this doc.
2. `feat(script): S7i — plumb reconnecting from login` — code + test (commit message updated to match scope reduction; webClient drop is documented in this spec).
3. `chore(script): S7i closed — session-flags plumbing` (with `Closes memory: nai_followups` trailer).
