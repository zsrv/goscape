# NAI-185 — Port admin-block non-spawn cheat cohort (7 cheats)

**Status:** Spec
**Date:** 2026-05-12
**TS source:** `LostCityRS/Engine-TS` `src/network/game/client/handler/ClientCheatHandler.ts`
**Tech stack:** Go 1.26+

## 1. Goal

Port 7 of the 17 cheats remaining in `DEVIATION-NAI-184-D2-D3-CARRYFORWARD` (`handlers_game.go:366-377`): the admin-block (`staffModLevel >= 3`) cheats that don't require dynamic Loc/Npc spawn or interface routing. After this sub-spec, the admin block's only unported arms are the dynamic-spawn trio (`locadd`, `npcadd`, `openmain`), cleanly partitioning the carryforward by real-infra prerequisite.

Ports:
- `setvar`, `setvarother (&& NP)`, `getvar`, `getvarother (&& NP)` — VarPlayer-quartet.
- `giveother (&& NP)` — cross-player inventory grant.
- `givecrap` — fill inventory with 28 random valid items.
- `broadcast (&& NP)` — fan-out MessageGame to all logged-in players.

## 2. Background

### 2.1 Why these 7

The NAI-184 close commit (0bcfae7) left the carryforward block enumerating 17 unported cheats across three tiers (dev, admin, super-mod). Infra survey at NAI-185 brainstorm confirmed that the carryforward labels overstate the blocker for the VarPlayer quartet: the named blocker `VarPlayerType.GetByName` is **not** missing data — `VarpTypeConfigs.ConfigNames` already populates a `debugname → id` map at load time (`varptype.go:99-101`). Only an exported accessor is missing.

Concretely, every dependency these 7 cheats reach into already exists at HEAD `0bcfae7`:

| Dependency | Site | Provided by |
|---|---|---|
| `VarpTypeConfigs.ConfigNames` (debugname index) | `varptype.go:99-101` | Existing — loaded by `parseVarpTypes` |
| `Player.Varp`, `Player.SetVarp` | `player_script.go:418-434` | Existing — script.ActivePlayer impl |
| `Player.CanAccess`, `Player.CloseModal`, `Player.ClearInteraction`, `Player.UnsetMapFlag` | `player_script.go:391`, `player_script.go:880`, `interaction.go:147`, `player_masks.go:54` | Existing |
| `Server.LookupPlayerByUsername` | `server.go:891` | Existing — NAI-184 teleother consumer |
| `ObjTypeConfigs.ByName` | `objtype.go:76` | Existing — NAI-184 give/givemany consumer |
| `Player.InvAdd` | `player_inv_cheat.go:19` | Existing — NAI-184 give consumer |
| `Server.cfg.NodeProduction`, `Server.cfg.NodeMembers` | `config.go:43`, `config.go:36` | Existing |
| `Player.MessageGame` | `message_game.go:15` | Existing |
| `ObjType.Members`, `ObjType.DummyItem`, `ObjType.CertTemplate` | `objtype.go:138/156/164` | Existing |
| `math/rand/v2 rand.IntN` | package-level (NAI convention) | Existing — `input_tracking.go:216`, `npc_hunt.go:82` |

The only new code is three small helpers (§4) plus the seven handler arms.

### 2.2 What's left after NAI-185

The rewritten carryforward (§6.4) partitions the remaining 10 cheats:
- **Dynamic-spawn cohort (admin >=3, 3 cheats):** `locadd`, `npcadd`, `openmain` — needs dynamic Loc/Npc registration + interface routing.
- **Dev block (!NP && >=4, 3 cheats):** `reload`, `rebuild`, `speed` — needs cache/script reload + runtime tick-rate mutation.
- **Super-mod block (>=2 && NP, 4 cheats):** `setvis`, `ban`, `mute`, `kick` — `setvis` is trivial (1-line `SetVisibility` setter); `ban`/`mute`/`kick` need login-moderation callbacks.

Three follow-up cohorts, each with a coherent real-infra prerequisite.

## 3. Approach (selected: C — all admin-reachable non-spawn)

Considered three bundling tiers:

- **A — VarPlayer quartet only (4 cheats):** smallest. Adds one helper (`VarpTypeConfigs.ByName`). Clean.
- **B — VarPlayer + cross-player inv (6 cheats):** adds `giveother`, `givecrap`. Reuses NAI-184 patterns (LookupPlayerByUsername, ObjType.ByName, InvAdd).
- **C — All admin-reachable non-spawn (7 cheats):** adds `broadcast`. New helper `Server.BroadcastMes` (RLock + range). Cleanly closes the admin block except for the dynamic-spawn trio.

**Selected: C.** Cleanest post-state for follow-up sub-spec planning. Diff size (~110 LOC handler + ~30 LOC helpers + ~700 LOC tests) is comparable to NAI-184. No new entity/world subsystems introduced.

Rejected D (C + setvis): setvis lives in the super-mod `>=2` block, which goscape doesn't instantiate yet. Activating that block for one dead-under-default arm mixes "admin-block close" with "super-mod-block open" and weakens the sub-spec framing.

## 4. New infra

### 4.1 `(*VarpTypeConfigs).ByName(name string) *VarPlayerType`

**File:** `pkg/objtype/varptype.go` (append after `parseVarpTypes`).

Direct mirror of `ObjTypeConfigs.ByName` at `objtype.go:76-92`. ConfigNames-indexed lookup with linear-scan fallback for test fixtures that don't populate the index. nil receiver returns nil.

```go
// ByName returns the VarPlayerType matching the given debugname, or nil
// if no match exists. Mirrors TS VarPlayerType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) only if ConfigNames is unpopulated (test fixtures).
func (vtc *VarpTypeConfigs) ByName(name string) *VarPlayerType {
    if vtc == nil {
        return nil
    }
    if id, ok := vtc.ConfigNames[name]; ok {
        if id >= 0 && id < len(vtc.Configs) {
            return vtc.Configs[id]
        }
    }
    for _, c := range vtc.Configs {
        if c != nil && c.DebugName == name {
            return c
        }
    }
    return nil
}
```

### 4.2 `(*Server).BroadcastMes(msg string)`

**File:** `modules/world/server_broadcast.go` (new file).

Iterates `s.players` under `playersMu.RLock` and calls `MessageGame` on each non-nil entry. Lock pattern matches `tick.go:97-100`. Mirrors TS `World.broadcastMes`.

```go
package world

// BroadcastMes sends a MESSAGE_GAME packet to every logged-in player.
// Mirrors TS World.broadcastMes. Holds Server.playersMu.RLock for the
// duration of the fan-out — callers must NOT hold playersMu.
func (s *Server) BroadcastMes(msg string) {
    s.playersMu.RLock()
    defer s.playersMu.RUnlock()
    for _, p := range s.players {
        if p == nil {
            continue
        }
        p.MessageGame(msg)
    }
}
```

### 4.3 `givecrap` inline RNG loop

**No helper extraction.** Loop is 12 lines, single call site, branches testable via post-conditions. Inlining matches the existing `give`/`givemany` arm style at `handlers_game.go:493-526`.

```go
case "givecrap":
    // TS L323-338. Fills inventory with 28 random items, retrying
    // each slot until one passes the filter (non-members under
    // !NodeMembers, non-dummy, non-certificate-template).
    for i := 0; i < 28; i++ {
        for {
            id := rand.IntN(len(p.client.server.objTypes.Configs))
            obj := p.client.server.objTypes.Configs[id]
            if obj == nil {
                continue
            }
            if !p.client.server.cfg.NodeMembers && obj.Members {
                continue
            }
            if obj.DummyItem != 0 {
                continue
            }
            if obj.CertTemplate != -1 {
                continue
            }
            p.InvAdd(p.client.server.invTypes.Inv, id, 1, false)
            break
        }
    }
```

## 5. Cheat semantics (per-arm port spec)

Each arm slots into the existing `if p.staffModLevel >= 3` admin block (`handlers_game.go:427+`) in TS source order. NP-gated arms use the inline `if !cfg.NodeProduction { break }` pattern (DEVIATION-NAI-185-D2 — §6.2). All TS line refs against `ClientCheatHandler.ts`.

### 5.1 `setvar` (TS L192-219) — not NP-gated

```
::setvar <name> <value>
```

1. Tokenize: `sub := strings.SplitN(args, " ", 2)`; reject if `len(sub) < 2` (TS L194: `args.length < 2`).
2. `cfg := s.varpTypes.ByName(sub[0])`; nil → reject (TS L201-203; silent, no MessageGame).
3. If `cfg.Protect`:
   - `p.CloseModal(true)` (TS L206; clearWeakQueue defaults true at Player.ts:741).
   - If `!p.CanAccess()`: `p.MessageGame("Please finish what you are doing first.")`; reject (TS L208-211). **Do not** clear interaction / map flag in this branch.
   - `p.ClearInteraction()`; `p.UnsetMapFlag()` (TS L213-214).
4. `value := clampInt32(parseIntOr(sub[1], 0))` where `clampInt32(v)` clamps to `[-0x80000000, 0x7fffffff]` (TS L217 `Math.max(-0x80000000, Math.min(tryParseInt(args[1], 0), 0x7fffffff))`).
5. `p.SetVarp(cfg.ID, int32(value))`.
6. `p.MessageGame(fmt.Sprintf("set %s: to %d", cfg.DebugName, value))` (TS L219).

### 5.2 `setvarother` (TS L220-252) — NP-gated

```
::setvarother <username> <name> <value>
```

1. `if !p.client.server.cfg.NodeProduction { break }` (DEVIATION-NAI-185-D2).
2. `sub := strings.SplitN(args, " ", 3)`; reject if `len(sub) < 3`.
3. `other := s.LookupPlayerByUsername(sub[0])`; nil → `p.MessageGame(fmt.Sprintf("%s is not logged in.", sub[0]))` + reject.
4. `cfg := s.varpTypes.ByName(sub[1])`; nil → reject (silent).
5. If `cfg.Protect`:
   - `other.CloseModal(true)`.
   - If `!other.CanAccess()`: **caller** gets `p.MessageGame(fmt.Sprintf("%s is busy right now.", sub[0]))`; reject (TS L242 — caller-vs-target asymmetry; DEVIATION-NAI-185-D3).
   - `other.ClearInteraction()`; `other.UnsetMapFlag()`.
6. `value := clampInt32(parseIntOr(sub[2], 0))`.
7. `other.SetVarp(cfg.ID, int32(value))`.
8. `p.MessageGame(fmt.Sprintf("set %s: to %d on %s", sub[1], value, other.username))`.

### 5.3 `getvar` (TS L253-267) — not NP-gated

```
::getvar <name>
```

1. `name := args`; reject if `name == ""` (TS L255: `args.length < 1`).
2. `cfg := s.varpTypes.ByName(name)`; nil → reject (silent).
3. `p.MessageGame(fmt.Sprintf("get %s: %d", cfg.DebugName, p.Varp(cfg.ID)))`.

### 5.4 `getvarother` (TS L268-287) — NP-gated

```
::getvarother <username> <name>
```

1. NP gate.
2. `sub := strings.SplitN(args, " ", 2)`; reject if `len(sub) < 2`.
3. `other := s.LookupPlayerByUsername(sub[0])`; nil → `p.MessageGame(fmt.Sprintf("%s is not logged in.", sub[0]))` + reject.
4. `cfg := s.varpTypes.ByName(sub[1])`; nil → reject (silent).
5. `p.MessageGame(fmt.Sprintf("get %s: %d on %s", cfg.DebugName, other.Varp(cfg.ID), other.username))`.

### 5.5 `giveother` (TS L303-322) — NP-gated

```
::giveother <username> <obj> [count]
```

1. NP gate.
2. `sub := strings.SplitN(args, " ", 3)`; reject if `len(sub) < 2`.
3. `other := s.LookupPlayerByUsername(sub[0])`; nil → `p.MessageGame(fmt.Sprintf("%s is not logged in.", sub[0]))` + reject.
4. `obj := s.objTypes.ByName(sub[1])`; nil → reject (silent).
5. Count: default 1. If `len(sub) > 2`: `count = parseIntOr(sub[2], 1)`. Clamp `[1, 0x7fffffff]`.
6. `other.InvAdd(s.invTypes.Inv, obj.ID, count, false)`.

### 5.6 `givecrap` (TS L323-338) — not NP-gated

See §4.3 inline implementation.

### 5.7 `broadcast` (TS L353-359) — NP-gated

```
::broadcast <message>
```

1. NP gate.
2. TS guard `args.length < 0` is unreachable (DEVIATION-NAI-185-D1). Skipped.
3. `s.BroadcastMes(args)`.

TS uses `cheat.substring(cmd.length + 1)` against the raw lowercased input. Goscape's `args` (computed at `handlers_game.go:362-365`) is semantically identical for any cmd of the matching first-token length. Documented inline (no code deviation).

## 6. Deviations

### 6.1 `DEVIATION-NAI-185-D1-DEAD-GUARD`

TS L355 `if (args.length < 0) return false;` — unreachable (array length is non-negative). Not ported as code; documented inline at the broadcast arm.

### 6.2 `DEVIATION-NAI-185-D2-NP-INLINE-GATE`

TS achieves NP-gating via the dispatcher fall-through (`else if (cmd === 'X' && NP)`); goscape's `switch` cannot replicate the fall-through. Each NP-gated arm gets an inner `if !cfg.NodeProduction { break }`. Observable behavior is identical; arm reachability via `case` label differs. Pattern established by NAI-184 teleother (`handlers_game.go:543-545`).

### 6.3 `DEVIATION-NAI-185-D3-VARPOTHER-MESSAGE-TARGET`

TS L242 routes the "busy" message to the **caller**, not the target. Asymmetric and easy to "correct" away. Pinned by dedicated test `TestSetvarother_BusyMessageGoesToCaller`. Not a goscape-vs-TS deviation; flagged so a future reader does not flip the recipient.

### 6.4 `DEVIATION-NAI-185-D4-CARRYFORWARD`

Supersedes `DEVIATION-NAI-184-D2-D3-CARRYFORWARD` at `handlers_game.go:366-377`. Rewritten block enumerates the 10 still-unported cheats with their real blockers:

```
// DEVIATION-NAI-185-D4-CARRYFORWARD — supersedes
// DEVIATION-NAI-184-D2-D3-CARRYFORWARD. 10 TS ClientCheatHandler cheats
// remain unported:
//   Dev block (!NP && >=4): reload, rebuild, speed.
//     Blocked on cache/script reload subsystem + runtime tick-rate
//     mutation (tick.go interval is currently fixed).
//   Admin block (>=3):      locadd, npcadd, openmain.
//     Blocked on dynamic Loc/Npc spawn + interface routing.
//   Super-mod (>=2):        setvis, ban, mute, kick.
//     setvis blocked on Player.SetVisibility setter (trivial).
//     ban/mute/kick: loginBridgeMod.NotifyPlayerBan/Mute exists
//     (handler_reportabuse.go:50, handler_message_private.go:42);
//     blocker is wiring caller-vs-automated args + kick's logout
//     teardown, not the moderation transport itself.
// Each cluster warrants its own follow-up sub-spec.
```

### 6.5 Silent rejects (documented, not a deviation)

TS uses `return false` (silent) for arg-count and name-miss rejects in setvar/setvarother/getvar/getvarother/giveother. Mirrored. **Pin:** unknown-varp-name in setvar/setvarother emits no MessageGame (tests assert outgoing queue empty after dispatch). Convention diverges from `teleother`'s `"X is not logged in."` — that message is for the cross-player lookup miss, not the type lookup miss.

### 6.6 `closeModal → canAccess` ordering (documented)

TS setvar L205-215 and setvarother L238-248 close the modal **before** the canAccess check. On reject-on-busy, the modal stays closed but interaction/mapflag are preserved. Ported in exact order.

## 7. Test plan

~45 focused tests across 7 per-arm files + 2 helper files + 1 dispatch-wiring test. Implementer dispatches per task with the existing TDD cadence.

### 7.1 Shared fixture

`newAdminCheatFixture(t, opts)` (sibling helper to NAI-184's setup):
- `cfg.NodeProduction` (false by default; true for NP-gated tests).
- `cfg.NodeMembers` (true / false variants for givecrap).
- `s.varpTypes` with `ConfigNames` and `Configs`: at least one protect+transmit, one transmit-only, one non-transmit varp.
- `s.objTypes` with `ByName`-resolvable items.
- `s.invTypes.Inv` populated.
- Two registered players via `LookupPlayerByUsername`.
- `staffModLevel = 3` default.

### 7.2 Per-arm tests

**`setvar`** (`handler_setvar_test.go`):
- `len(args)<2` rejects without varp lookup or message.
- Unknown name rejects silently.
- Non-protect varp: SetVarp called; MessageGame `"set <debugname>: to <value>"`.
- Protect varp + CanAccess=true: CloseModal(true), ClearInteraction, UnsetMapFlag, SetVarp all called.
- Protect varp + CanAccess=false: MessageGame `"Please finish what you are doing first."`, no SetVarp, no ClearInteraction/UnsetMapFlag.
- Value clamp: above 0x7fffffff clamps DOWN; below -0x80000000 clamps UP.
- Parse error → defaults to 0.

**`setvarother`** (`handler_setvarother_test.go`):
- NP=false → no effect.
- NP=true, `len(args)<3` rejects.
- NP=true, username not logged in → caller gets `"<arg0> is not logged in."`.
- NP=true, unknown varp → silent reject.
- NP=true, protect varp + target busy → **caller** gets `"<arg0> is busy right now."`. (DEVIATION-NAI-185-D3 pin.)
- NP=true, happy path → `other.SetVarp` invoked, caller gets `"set <argname>: to <value> on <other.username>"`.

**`getvar`** (`handler_getvar_test.go`):
- `len(args)<1` rejects.
- Unknown name rejects silently.
- Happy path → MessageGame `"get <debugname>: <value>"`.

**`getvarother`** (`handler_getvarother_test.go`):
- NP=false → dead.
- NP=true, `len(args)<2` rejects.
- NP=true, username not logged in → caller gets `"<arg0> is not logged in."`.
- NP=true, unknown name → silent reject.
- NP=true, happy path → caller gets `"get <debugname>: <value> on <other.username>"`.

**`giveother`** (`handler_giveother_test.go`):
- NP=false → dead.
- NP=true, `len(args)<2` rejects.
- NP=true, username not logged in → `"<arg0> is not logged in."`.
- NP=true, unknown item → silent reject.
- NP=true, happy path with explicit count → `other.InvAdd(invTypes.Inv, obj.ID, count, false)`.
- NP=true, missing count defaults to 1; count<1 clamps to 1; count>0x7fffffff clamps.

**`givecrap`** (`handler_givecrap_test.go`):
- 28 items added.
- All filter-passing assertion.
- Members-world (NodeMembers=true) allows members items.
- F2P-world (NodeMembers=false) excludes members items.
- Small-pool fixture (1 passing item among 5) under 1s timeout — no infinite loop.

**`broadcast`** (`handler_broadcast_test.go`):
- NP=false → no MessageGame.
- NP=true, empty message → every player receives empty MessageGame.
- NP=true, multi-word args → every player receives `args` verbatim.
- 3-player fixture: each player's outgoing queue has exactly one OpMessageGame.

### 7.3 Helper tests

**`VarpTypeConfigs.ByName`** (`pkg/objtype/varptype_test.go` additions):
- Hit via `ConfigNames`.
- Miss returns nil.
- nil receiver returns nil.
- OOB id in `ConfigNames` (stale-index sentinel) falls through to linear scan.
- Linear-scan fallback when `ConfigNames` is nil/empty but Configs populated.

**`Server.BroadcastMes`** (`modules/world/server_broadcast_test.go`):
- 3 players: all receive identical OpMessageGame payload.
- Nil slot in `s.players` skipped, surrounding players still messaged.
- Empty message string delivered (no defensive reject).

### 7.4 Dispatch wiring test

`TestClientCheat_DispatchesToNAI185Arms` in `handlers_game_test.go`: drives `handleClientCheat` end-to-end for one representative arm of each shape (setvar, setvarother, giveother, givecrap, broadcast). Pins:
- Existing `staffModLevel >= 3` guard.
- Existing `addSessionLog` at staffModLevel >= 2 (cheat names appear in session log).
- `parts[0]` dispatch reaches each arm.

## 8. Files touched

| File | Change | LOC est |
|---|---|---|
| `pkg/objtype/varptype.go` | Add `ByName` method | +20 |
| `pkg/objtype/varptype_test.go` | Add 5 ByName tests | +80 |
| `modules/world/server_broadcast.go` | New file: `BroadcastMes` | +20 |
| `modules/world/server_broadcast_test.go` | New file: 3 tests | +90 |
| `modules/world/handlers_game.go` | 7 new arms; rewrite carryforward comment | +120 |
| `modules/world/handler_setvar_test.go` | New file | +180 |
| `modules/world/handler_setvarother_test.go` | New file | +180 |
| `modules/world/handler_getvar_test.go` | New file | +90 |
| `modules/world/handler_getvarother_test.go` | New file | +110 |
| `modules/world/handler_giveother_test.go` | New file | +140 |
| `modules/world/handler_givecrap_test.go` | New file | +130 |
| `modules/world/handler_broadcast_test.go` | New file | +110 |
| `modules/world/handlers_game_test.go` | Add `newAdminCheatFixture`, dispatch wiring test | +120 |

Approximate total: ~140 LOC production, ~1230 LOC tests.

## 9. Memory-driven controller pre-flight checklist

Before each implementer dispatch, the controller verifies:

- `plan_grep_helper_patterns`: grep `handlers_game.go` for existing helper patterns (parseIntOr, the NodeProduction inline-break shape) before codifying inline boilerplate.
- `plan_sibling_site_guard_audit`: grep all sibling cheat arms for the `staffModLevel >= 3` + NP-gate shape; reproduce verbatim.
- `plan_type_name_grep`: confirm `VarpTypeConfigs` (not `VarPlayerTypeConfigs`), `objtype.LocTypes`-style naming conventions before writing test fixtures.
- `risk_register_premise_grep`: verify `s.playersMu` is the correct lock for `BroadcastMes` by re-reading `tick.go` lock-acquisition sites.
- `controller_preflight`: 30-second grep+Read pass against HEAD before each implementer dispatch.
- `plan_runnable_test_fixtures`: mentally execute (or `go test` dry-run) plan-authored fixtures before dispatch.
- `plan_helper_coverage`: cross-check `newAdminCheatFixture` flag/lifecycle sets against every consuming test.
- `verify_implementer_claims`: 30-second protocol on each commit (fresh `go test ./...` + `git show <SHA> --stat`).
- `close_commit_memory_trailer`: emit `Closes memory:` trailer at the close commit citing any new memory entries.

## 10. Out of scope

- **Dynamic-spawn cohort** (locadd, npcadd, openmain): future sub-spec.
- **Dev block** (reload, rebuild, speed): future sub-spec.
- **Super-mod block** (setvis, ban, mute, kick): future sub-spec.
- **Refactoring `handleClientCheat`**: function will grow to ~250 lines after this sub-spec. Do NOT extract per-arm functions or a dispatch table here. Defer to a separate cleanup sub-spec after the full cheat port closes.
- **Alternative InvType targets**: TS hardcodes `InvType.INV` for giveother/givecrap; goscape mirrors with `s.invTypes.Inv`. No support for arbitrary inv targets.
- **Live VarpType reload**: `ByName` reads load-time `ConfigNames`; no live-reload pathway.
- **RNG injection**: tests use `math/rand/v2` package-level rand per existing convention (input_tracking.go, npc_hunt.go). Post-condition assertions are deterministic.

## 11. Risk register

| Risk | Mitigation |
|---|---|
| `VarpTypeConfigs.ByName` semantics diverge from `ObjTypeConfigs.ByName` at edges | Literal mirror; tests pin nil receiver, stale-index, linear-scan fallback. |
| `s.varpTypes` unset in some test fixture causes nil deref | `ByName` accepts nil receiver; arms reject on nil cfg without panic. |
| `givecrap` infinite loop if all items filter out | Small-pool fixture under 1s timeout; production cache has thousands of passing items. |
| `BroadcastMes` deadlock if a caller already holds `playersMu` | Only invoked from cheat handler path (no lock held). Doc-comment documents the RLock acquisition. |
| Cross-player "busy" routing typo in setvarother | Pinned by `TestSetvarother_BusyMessageGoesToCaller` (DEVIATION-NAI-185-D3). |
| Trailing whitespace in args producing empty trailing token mis-counted | TS `split(' ')` and Go `strings.SplitN` differ on trailing empty tokens; pin `"setvar foo"` (rejects, 1 token) and `"setvar foo 5"` (accepts, 2 tokens). |
