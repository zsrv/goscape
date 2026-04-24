# S7a — FINDUID + P_FINDUID Design

> **Sub-spec context:** Twenty-seventh runescript sub-spec; first of S7. Implements the two player-lookup-by-UID opcodes that bind the script's active player to a target identified by uid. Currently blocking `[proc,update_all]` at `pc=61` (P_FINDUID dispatch); FINDUID bundled because the implementations differ by ~5 lines and tests share the same fixture.

> **TS-faithfulness gate:** Matches `LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:60-94`. Single deviation from TS: goscape's collapsed pointer model (single `PtrActivePlayer` + `ScriptState.Protect bool`) means "set ProtectedActivePlayer pointer" reduces to "set Protect=true" — same behavior, fewer pointer flags. Established by S6w; not new here.

> **Scope:** Two opcodes (FINDUID 2019, P_FINDUID 2073), one new host interface, one server-side lookup. ~190 LOC including tests.

## 1. Goal

Implement `OpFindUID` (2019) and `OpPFindUID` (2073) so `[proc,update_all]` and any future script that calls them no longer aborts with `no handler for ...`. Both opcodes pop a UID from the int stack, look up the corresponding logged-in player, atomically rebind `state.Self` to the target on success, and push 1; push 0 on failure.

The two differ in two ways:
1. **P_FINDUID** additionally checks `target.canAccess()` (not delayed, no modal, not in another protected script) — the lookup fails if access is denied.
2. **P_FINDUID** sets `state.Protect = true` on success (TS sets the ProtectedActivePlayer pointer); FINDUID leaves `Protect` unchanged.

P_FINDUID also has a self-reacquire fast-path: if the script already runs on the target with protected access, push 1 with no state change.

## 2. TS reference

- `src/engine/script/handlers/PlayerOps.ts:60-72` — FINDUID. Pop uid → `World.getPlayerByUid(uid)`. Push 0 if nil. Else: `state.activePlayer = player`; `state.pointerAdd(ActivePlayer[state.intOperand])`; push 1. **No canAccess check.**
- `src/engine/script/handlers/PlayerOps.ts:75-94` — P_FINDUID. Pop uid coerced unsigned (`>>> 0`). Self-reacquire fast-path: if `pointerGet(ProtectedActivePlayer) && activePlayer.uid === uid`, push 1 and return. Else lookup. Push 0 if nil OR `!player.canAccess()`. Else: set activePlayer, add ActivePlayer + ProtectedActivePlayer pointers, push 1.
- `src/engine/entity/Player.ts:805-812` — `canAccess() = !this.protect && !this.busy()`.
- `src/engine/entity/Player.ts:801-803` — `busy() = this.delayed || this.containsModalInterface()`.
- `src/engine/entity/Player.ts:796-799` — `containsModalInterface() = (this.modalState & (MAIN | CHAT)) !== NONE`.

## 3. Architecture

### 3.1 New host interface (`pkg/script/state.go`)

```go
// PlayerLookup resolves a UID to an ActivePlayer if a player with that UID
// is currently logged in. The handler decides whether the result is
// usable (P_FINDUID additionally checks ActivePlayer.CanAccess; FINDUID
// accepts any logged-in match).
type PlayerLookup interface {
    LookupPlayerByUID(uid int) ActivePlayer
}
```

`ScriptState` gains one field:

```go
PlayerLookup PlayerLookup
```

### 3.2 New ActivePlayer method (`pkg/script/active.go`)

```go
// CanAccess reports whether the player can be bound as a protected active
// player by P_FINDUID. False if delayed, has a modal main/chat open, or
// is currently inside a suspended protected script. Mirrors TS
// Player.canAccess (Player.ts:805-812). FINDUID does NOT consult this.
CanAccess() bool
```

### 3.3 Handlers (`pkg/script/handlers_player.go`)

```go
func handleFindUID(s *ScriptState) error {
    uid := s.PopInt()
    if s.PlayerLookup == nil {
        s.PushInt(0)
        return nil
    }
    target := s.PlayerLookup.LookupPlayerByUID(uid)
    if target == nil {
        s.PushInt(0)
        return nil
    }
    s.Self = target
    s.Pointers |= PtrActivePlayer
    s.PushInt(1)
    return nil
}

func handlePFindUID(s *ScriptState) error {
    uid := s.PopInt()
    // Self-reacquire fast-path: already running protected on this player.
    if s.Protect && s.Self != nil && s.Self.UID() == uid {
        s.PushInt(1)
        return nil
    }
    if s.PlayerLookup == nil {
        s.PushInt(0)
        return nil
    }
    target := s.PlayerLookup.LookupPlayerByUID(uid)
    if target == nil || !target.CanAccess() {
        s.PushInt(0)
        return nil
    }
    s.Self = target
    s.Pointers |= PtrActivePlayer
    s.Protect = true
    s.PushInt(1)
    return nil
}
```

Registered in `pkg/script/handlers.go`'s `handlers` map.

### 3.4 Server-side lookup (`modules/world/server.go`)

```go
// LookupPlayerByUID scans s.playerLoop for a logged-in player with the
// given UID. Called from the script tick (already on the tick goroutine,
// so playerLoop access is unguarded). Returns nil if no match.
//
// Does NOT filter on accessibility — handlers (P_FINDUID) consult
// CanAccess separately. Mirrors TS World.getPlayerByUid which is a
// pure lookup; the access gate lives in canAccess().
func (s *Server) LookupPlayerByUID(uid int) script.ActivePlayer {
    for _, p := range s.playerLoop {
        if p == nil || !p.active {
            continue
        }
        if p.uid == uid {
            return p
        }
    }
    return nil
}
```

Wired in `modules/world/script.go:runScript`:

```go
state.PlayerLookup = s
```

### 3.5 CanAccess on Player (`modules/world/player.go` — new method)

```go
// CanAccess reports whether this player can be bound as a protected
// active player. False when delayed, when a modal main/chat is open,
// or when a suspended protected script is stored. Mirrors TS
// Player.canAccess (Player.ts:805-812).
func (p *Player) CanAccess() bool {
    if p.delayed {
        return false
    }
    if p.modalState&(modalStateMain|modalStateChat) != 0 {
        return false
    }
    if p.activeScript != nil && p.activeScript.Protect {
        return false
    }
    return true
}
```

## 4. File map

| File | Action |
|---|---|
| `pkg/script/state.go` | +5 LOC: `PlayerLookup` interface and `ScriptState.PlayerLookup` field |
| `pkg/script/active.go` | +2 LOC: `CanAccess() bool` on `ActivePlayer` interface |
| `pkg/script/handlers_player.go` | +35 LOC: `handleFindUID` + `handlePFindUID` |
| `pkg/script/handlers.go` | +2 LOC: register both handlers in the dispatch map |
| `pkg/script/runner_test.go` | +3 LOC: stub `CanAccess() bool { return true }` on `mockPlayer` |
| `pkg/script/handlers_player_test.go` | +120 LOC: 7 unit tests (see test plan) |
| `modules/world/player.go` | +12 LOC: `CanAccess()` method |
| `modules/world/server.go` | +18 LOC: `LookupPlayerByUID` method |
| `modules/world/script.go` | +1 LOC: `state.PlayerLookup = s` in `runScript` |
| `modules/world/server_test.go` | +60 LOC: 4 lookup unit tests |

## 5. Test plan

**Script-layer tests (`pkg/script/handlers_player_test.go`):**

```go
// Two helper types:
type mockPlayerLookup struct {
    byUID map[int]ActivePlayer
    calls int
}
func (m *mockPlayerLookup) LookupPlayerByUID(uid int) ActivePlayer {
    m.calls++
    return m.byUID[uid]
}

// mockPlayer.canAccessValue bool is added; CanAccess returns it.
```

1. **TestFindUIDFound** — Lookup returns target. Stack: `[1]`. `Self == target`. `Pointers & PtrActivePlayer != 0`. `Protect` unchanged from initial false.
2. **TestFindUIDNotFound** — Lookup returns nil. Stack: `[0]`. `Self` unchanged.
3. **TestFindUIDNoLookupConfigured** — `PlayerLookup == nil`. Stack: `[0]`. `Self` unchanged.
4. **TestPFindUIDSelfReacquire** — `Self.uidValue=42`, `Protect=true`, push 42. Stack: `[1]`. `Self` unchanged. `lookup.calls == 0` (fast-path skipped lookup).
5. **TestPFindUIDFoundCanAccess** — Target `canAccessValue=true`. Stack: `[1]`. `Self == target`. `Protect == true`. `Pointers & PtrActivePlayer != 0`.
6. **TestPFindUIDFoundCannotAccess** — Target `canAccessValue=false`. Stack: `[0]`. `Self` unchanged. `Protect` unchanged.
7. **TestPFindUIDNotFound** — Lookup returns nil. Stack: `[0]`. `Self` unchanged.

**Server-layer tests (`modules/world/server_test.go`):**

8. **TestLookupPlayerByUIDFound** — Server with one player at `uid=99` returns that player.
9. **TestLookupPlayerByUIDNotFound** — Returns nil for an unknown UID.
10. **TestLookupPlayerByUIDIgnoresInactive** — A player with `active=false` is skipped.
11. **TestPlayerCanAccess** — Table-driven across (delayed, modalState, activeScript-with-Protect) combinations. Asserts the four-case truth table.

## 6. Task split

**Single task.** ~190 LOC across 9 files. No external dependencies, no migrations, no fixture churn (existing tests stay green because `mockPlayer.CanAccess` defaults to `true` and the new field on `ScriptState` defaults to nil — handlers degrade gracefully).

Commit: `feat(script): FINDUID + P_FINDUID handlers with player-lookup interface (S7a)`

## 7. Deviations

| ID | Status |
|---|---|
| **S7a-D1** | **NEW (carried from S6w)** — TS `pointerAdd(ProtectedActivePlayer)` collapses to `state.Protect = true` because goscape doesn't separate ActivePlayer from ProtectedActivePlayer pointers. Same observable behavior; gate helpers (`requireProtectedActivePlayer`) already check `Protect`. Splitting becomes mechanical if a future sub-spec needs the distinction. |
| **S7a-D2** | **NEW** — `Player.uid` is currently never assigned (always -1) — see follow-up below. The handler ships functional but `LookupPlayerByUID` will fail to match any non-(-1) UID until the source-of-truth for `p.uid` is wired. Scripts handle `FINDUID == 0` already (the legitimate "target logged out" case) so this degrades gracefully rather than crashing. |

No closures.

## 8. Follow-ups

- **`p.uid` source wiring** — needs a sub-spec to decide between (a) `req.UID` from the client (machine hash, not stable per account), (b) the gRPC PlayerLogin response account_id field, or (c) a deterministic hash of the username (matches TS `getUid(username)`). Likely (c). Until this lands, P_FINDUID for non-self targets always pushes 0.
- **Other UID-keyed opcodes** — `OpUID` (push self uid), `OpName` already implemented; check whether the `STAFFMODSELF`-style ops or any AI handler also reference `p.uid` and would benefit from the same source decision.

## 9. Self-review notes

- No placeholders / TBDs.
- Internal consistency: handlers in §3.3 match test expectations in §5; file map in §4 sums correctly to ~190 LOC; deviations in §7 cross-reference S6w and the §8 follow-up.
- Scope: single task, no decomposition needed.
- Ambiguity: the "set Protect=true" reduction is explicit; the "p.uid always -1" risk is called out twice (architecture + deviation table) so an implementer cannot miss it.
