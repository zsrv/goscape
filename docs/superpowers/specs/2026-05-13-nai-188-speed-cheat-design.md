# NAI-188 — Dev-block `::speed` cheat (`tickRate` const → `Server` field)

**Date:** 2026-05-13
**Status:** Spec (awaiting plan)
**Tech stack:** Go 1.26+
**TS source:** `LostCityRS/Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts:154-167`
**Predecessors:** NAI-183 (cheat infra), NAI-187 (admin spawn cohort + carryforward rewrite).
**HEAD at spec-write:** `83baea9`

## §1 Goal

Port the dev-tier `::speed <ms>` cheat (TS `ClientCheatHandler.ts:154-167`) into goscape's existing dev block at `modules/world/handlers_game.go:399-430`. Mutates the per-tick sleep interval of the server's tick loop.

Required infra change: `tickRate` is currently a package-level `const` (`modules/world/tick.go:15`). The cheat assigns to `World.tickRate` per cycle, so goscape must promote `tickRate` to a per-`Server` field that the tick loop re-reads each iteration.

Closes the `speed` row of `DEVIATION-NAI-187-D1-CARRYFORWARD` (`modules/world/handlers_game.go:370-381`). After NAI-188, only `reload` and `rebuild` remain unported (both blocked on hot-reload pipelines outside this sub-spec's scope).

## §2 Out of scope

| Concern | TS line | Why deferred |
|---|---|---|
| Shutdown-induced tick speedup (`tickRate = 0` after 1s) | `World.ts:1224` | TS uses 0ms as "as fast as possible" during the post-shutdown drain. goscape's `processShutdown` lives elsewhere (NAI-182) and does not yet model this acceleration. Worth its own audit; not on the `::speed` carryforward. |
| `reload` cheat | `ClientCheatHandler.ts:149-150` | Calls `World.reload()` — full cache hot-reload. Genuine infra gap. |
| `rebuild` cheat | `ClientCheatHandler.ts:151-153` | Calls `World.rebuild()` — script-provider hot-reload. Same infra gap. |
| TS profile printing using `tickRate` | `World.ts:494` | goscape has no `NODE_DEBUG_PROFILE` equivalent; not relevant to `::speed`. |

The carryforward comment in `handlers_game.go:370-381` MUST be rewritten on close to remove the `speed:` row and reframe the cluster as the two remaining infra-blocked cheats.

## §3 Pre-flight audit

Per memory `tracker_entry_framing_can_be_incomplete` and `risk_register_premise_grep`, every premise below was re-verified against HEAD `83baea9`.

### §3.1 TS `tickRate` write sites

Five references (confirmed via grep on `Engine-TS/src/engine/World.ts`):

| Line | Site | Role |
|---|---|---|
| 163 | `tickRate: number = World.TICKRATE` | Field declaration, default 600. |
| 494 | profile printf | Read (debug-only, no goscape equivalent). |
| 502 | `this.nextTick += this.tickRate` | Read (per-cycle advance). |
| 506 | `setTimeout(this.cycle.bind(this), Math.max(0, this.tickRate - ...))` | Read (per-cycle sleep). |
| 1224 | `this.tickRate = 0` | Write (shutdown speedup, deferred — §2). |

The `::speed` write at `ClientCheatHandler.ts:167` is the sixth, and the only one in scope for NAI-188.

### §3.2 goscape `tickRate` references at HEAD `83baea9`

Three sites (`rg "tickRate" modules/ pkg/`):

| File:line | Site | Role |
|---|---|---|
| `modules/world/tick.go:15` | `const tickRate = 600 * time.Millisecond` | Package-level const. |
| `modules/world/tick.go:23` | `s.runTickLoopWithRate(tickRate)` | Production caller. |
| `modules/world/handlers_game.go:379` | Doc-comment | Carryforward note (rewritten on close). |

`runTickLoopWithRate(rate time.Duration)` (tick.go:26) takes the rate as a parameter and uses it as a captured local inside the loop body. One test exercises this parameter:

| File:line | Caller | Rate |
|---|---|---|
| `modules/world/server_test.go:578` | `TestTickLoopIncrementsCurrentTick` | `3 * time.Millisecond` |

No other production or test code reads `tickRate`. The package-level identifier is private and has no `pkg/`-side consumers.

### §3.3 Reusable infra at HEAD

| Component | Location | Use |
|---|---|---|
| `parseIntOr(s string, def int) int` | `handlers_game.go:1011` | TS `tryParseInt` mirror. Already used by `slowreboot`, `setstat`, `advancestat`, `give`. |
| `p.MessageGame(string)` | `player.go` | TS `player.messageGame` mirror. Already used by every existing cheat case. |
| Dev-block switch | `handlers_game.go:399-429` | Outer guard `!NodeProduction && staffModLevel >= 4` already in place from NAI-183. |
| Carryforward doc-comment | `handlers_game.go:370-386` | Will be edited on close to drop `speed:` row. |

No new helpers required at the cheat layer.

## §4 Architecture

### §4.1 Layer 1: `modules/world/tick.go` — promote `tickRate` to a `Server` field

Three coordinated edits:

**(a)** Rename the package-level constant to `defaultTickRate` (kept as the canonical default for `Server` construction, mirroring TS `World.TICKRATE`):

```go
// defaultTickRate is the canonical tick interval. Mirrors TS
// World.TICKRATE (Engine-TS World.ts:120) = 600ms. The ::speed cheat
// (NAI-188) writes Server.tickRate to a different value at runtime.
const defaultTickRate = 600 * time.Millisecond
```

**(b)** Add `tickRate time.Duration` to the `Server` struct (in `modules/world/server.go` next to other tick-loop state) and initialise it in the constructor (`New`, or whichever factory builds `*Server`):

```go
s.tickRate = defaultTickRate
```

**(c)** Rewrite `runTickLoop` / `runTickLoopWithRate` so the loop re-reads `s.tickRate` each iteration:

```go
func (s *Server) runTickLoop() {
    s.runTickLoopWithRate(s.tickRate)
}

func (s *Server) runTickLoopWithRate(rate time.Duration) {
    s.tickRate = rate  // honour test-injected rate
    nextTick := time.Now()
    for {
        // ... existing shutdown / processX dispatch ...

        currentRate := s.tickRate  // re-read each iteration; ::speed mutates mid-loop
        nextTick = nextTick.Add(currentRate)
        delay := currentRate - time.Since(start) - drift
        // ... existing select on s.quit / time.After(delay) ...
    }
}
```

**Naming note:** the inner read is `currentRate := s.tickRate`, not `rate := s.tickRate`, to avoid shadowing the parameter inside the loop body (per memory `plan_var_name_collision`).

**Rationale for the parameter-preserving shape:**

- The existing test `TestTickLoopIncrementsCurrentTick` calls `s.runTickLoopWithRate(3 * time.Millisecond)`. Preserving the parameter (and seeding `s.tickRate` from it) keeps the test surface unchanged.
- The intra-loop re-read makes ::speed mutations take effect on the *next* sleep computation, which matches TS — TS `World.cycle` reads `this.tickRate` on lines 502 and 506 every iteration, and the cheat mutates the same field.
- A separate "writer goroutine vs reader goroutine" race does **not** exist: cheats are dispatched from `processIn` (modules/world/tick.go:96-108, called via `processClientsIn` inside the tick goroutine), so the mutation and the subsequent re-read are both on the tick goroutine. **No mutex required.** (This is the same single-threaded invariant TS relies on.)

**Defensive note on the parameter form:** writing `s.tickRate = rate` at function entry is a deliberate one-way assignment so that the test's 3ms rate is observable via `s.tickRate` from the same point the loop reads it. This is safe because `runTickLoopWithRate` is the only top-of-stack entry into the loop. If a future caller wanted to read `s.tickRate` *before* invoking the loop, the field is already initialised to `defaultTickRate` by the constructor.

### §4.2 Layer 2: `modules/world/handlers_game.go` — dev-block `::speed` case

Add a `case "speed":` to the existing dev-block switch at `handlers_game.go:400`, after `case "naive":` and `case "random":` (TS source order: speed at L154-167 comes *before* fly at L168-175, but goscape's existing dev block already orders cheats by port date — see fly/naive/random at 401-429. New cases append at the end to minimise diff churn).

TS-faithful port:

```go
case "speed":
    // TS ClientCheatHandler.ts:154-167. Args layout: single positional
    // arg parsed as ms; default 20 on parse failure. Validation order:
    //   1. empty args → "Usage: ::speed <ms>" message, no state change.
    //   2. parsed value < 20 → "too low" message, no state change.
    //   3. else → "World speed was changed to {ms}ms" + s.tickRate write.
    if args == "" {
        p.MessageGame("Usage: ::speed <ms>")
        return nil
    }
    // TS uses args.shift() then tryParseInt(arg, 20). args.shift() takes
    // the FIRST whitespace token; goscape's `args` is the post-first-space
    // tail (handlers_game.go:362-368). Slice the first whitespace-delimited
    // token to mirror TS.
    first := args
    if i := strings.IndexAny(args, " \t"); i >= 0 {
        first = args[:i]
    }
    speed := parseIntOr(first, 20)
    if speed < 20 {
        p.MessageGame("::speed input was too low.")
        return nil
    }
    p.MessageGame(fmt.Sprintf("World speed was changed to %dms", speed))
    p.client.server.tickRate = time.Duration(speed) * time.Millisecond
```

**Faithfulness notes:**

- TS `tryParseInt(args.shift(), 20)` returns 20 when the arg is non-numeric (the default). Mirrored exactly via `parseIntOr(first, 20)`.
- TS message strings preserved verbatim, including the trailing period on `"::speed input was too low."` and the lowercase `"Usage:"`.
- Floor of 20ms is enforced strictly; TS uses `<` not `<=`. So `speed == 20` succeeds (corner case test: `speed = 20` → tickRate becomes 20ms).
- TS uses milliseconds as the unit. goscape uses `time.Duration`; conversion happens exactly at the assignment boundary.

### §4.3 Layer 3: carryforward doc-comment

On close (after T4 lands), edit `handlers_game.go:370-386` to remove the `speed:` row and update the tally line ("3 TS … cheats remain" → "2"). NAI-187's framing of `reload` / `rebuild` is preserved verbatim.

## §5 Data flow

```
client cheat packet  →  handleClientCheat (handlers_game.go)
                        ├─ parse "speed 20"  →  parts[0]="speed", args="20"
                        ├─ dev-block guard: !NodeProduction && staffModLevel >= 4
                        └─ case "speed":
                            ├─ args=="" → MessageGame + return
                            ├─ parseIntOr(first, 20) → int
                            ├─ <20 → MessageGame + return
                            └─ MessageGame + s.tickRate = ms * time.Millisecond
                                                ↓
                                       runTickLoopWithRate re-reads
                                       s.tickRate at next iteration
                                                ↓
                                       next sleep uses the new value
```

## §6 Concurrency

- All mutations to `s.tickRate` happen on the tick goroutine (cheat dispatch path runs inside `processClientsIn` → `processIn`).
- All reads of `s.tickRate` happen on the tick goroutine (the `runTickLoopWithRate` body).
- **No locking required.** A `-race`-clean test exercising the cheat mutation will validate this.

If a future code path needs to mutate `tickRate` from a non-tick goroutine (e.g. a future signal handler or an HTTP admin endpoint), it MUST introduce a mutex or post via a channel to the tick goroutine. NAI-188 does not introduce such a path.

## §7 Testing

Six new cheat-handler tests at `modules/world/handlers_game_test.go` (or the existing cheat-test file, whichever holds `TestCheatFly` / `TestCheatNaive` — verified at plan-write time). All follow the existing cheat-test pattern (staffModLevel=4, NodeProduction=false).

| Test | Input | Assertion |
|---|---|---|
| `TestCheatSpeed_EmptyArgs` | `"speed"` | `p.lastMessageGame == "Usage: ::speed <ms>"`; `s.tickRate == defaultTickRate` (unchanged). |
| `TestCheatSpeed_BelowFloor` | `"speed 19"` | `p.lastMessageGame == "::speed input was too low."`; `s.tickRate` unchanged. |
| `TestCheatSpeed_AtFloor` | `"speed 20"` | `p.lastMessageGame == "World speed was changed to 20ms"`; `s.tickRate == 20*time.Millisecond`. |
| `TestCheatSpeed_AboveFloor` | `"speed 100"` | `p.lastMessageGame == "World speed was changed to 100ms"`; `s.tickRate == 100*time.Millisecond`. |
| `TestCheatSpeed_NonNumeric` | `"speed banana"` | `p.lastMessageGame == "World speed was changed to 20ms"`; `s.tickRate == 20*time.Millisecond`. (See §7.1 for TS trace.) |
| `TestCheatSpeed_Negative` | `"speed -5"` | `p.lastMessageGame == "::speed input was too low."`; `s.tickRate` unchanged. |

### §7.1 Non-numeric arg traces to success, not rejection

Re-tracing TS behavior for `::speed banana`:
- `args.shift()` returns `"banana"`.
- `tryParseInt("banana", 20)` returns `20` (the default).
- `20 < 20` is false → fall through to the success branch.
- Result: `MessageGame("World speed was changed to 20ms")` + `World.tickRate = 20`.

The TS "default 20" is *not* a "reject non-numeric" sentinel — it's a silent floor coercion. The `_NonNumeric` test asserts the success path accordingly. `_Negative` then pins the rejection branch via an explicitly-parseable sub-floor value (`-5`), since `parseIntOr("-5", 20) == -5` and `-5 < 20` is true.

### §7.2 Tick-loop honours field mutation (integration-style)

One test extends the existing `TestTickLoopIncrementsCurrentTick` pattern to validate that mutating `s.tickRate` mid-run changes the cadence. Approach: start the loop at 3ms, sleep 30ms (~10 ticks), mutate to 30ms, sleep another 60ms (~2 ticks), assert the post-mutation tick count grew slower than the pre-mutation count. Run with `-race`.

| Test | Pattern | Assertion |
|---|---|---|
| `TestTickLoopHonoursFieldRate` | start at 3ms → sleep 30ms → set s.tickRate=30ms → sleep 60ms → close quit | second-window tick delta < first-window tick delta (with reasonable jitter tolerance) |

### §7.3 Existing test compatibility

`TestTickLoopIncrementsCurrentTick` (`server_test.go:572`) must continue to pass unchanged. The parameter-preserving rewrite of `runTickLoopWithRate` (§4.1) ensures `s.runTickLoopWithRate(3 * time.Millisecond)` still produces a 3ms loop.

## §8 Risk register

| Risk | Evidence | Mitigation |
|---|---|---|
| `s.tickRate` field placement breaks `Server` zero-value tests | grep `modules/world` for tests that construct `&Server{}` directly | Constructor-init pattern: `New(...)` sets `s.tickRate = defaultTickRate`; any direct `&Server{}` literal already produces a zero-value `time.Duration` (= 0), which would deadlock `time.After(0)` in a degenerate way. **Plan-author MUST grep for `&Server{` at plan-write and add an explicit setter call (or migrate to `newTestServer`) at any site that bypasses the constructor.** |
| `runTickLoopWithRate` parameter semantics change silently | TS doesn't have this shape; only goscape test fixtures use it | The `s.tickRate = rate` assignment at function entry is documented inline. Test file uses 3ms; tickRate field updated implicitly. No external callers. |
| Mid-loop mutation race | §6 audit | Single goroutine; `-race` test in §7.2 validates. |
| Non-numeric arg test expectation | §7.1 | Re-traced TS twice; both readings agree on "20 default → success at 20ms". |
| Mutating a `const` in another caller | grep `tickRate` shows only one prod site + one test site | Rename to `defaultTickRate` is exhaustive; no shadowing concern. |

## §9 Deviations from TS

None. NAI-188 is straight-port. The TS `tickRate = 0` shutdown speedup at `World.ts:1224` is deferred (§2), not deviated — it's a separate behavior the carryforward does not require.

## §10 Close criteria

- All §7 tests pass with `-race`.
- `case "speed":` in `modules/world/handlers_game.go` with all five TS branches honoured (empty args / `<20` floor / `>=20` success / message verbatim / `s.tickRate` write).
- `defaultTickRate` constant + `Server.tickRate` field + loop re-read in place.
- `DEVIATION-NAI-187-D1-CARRYFORWARD` doc-comment rewritten to drop `speed:` row; tally updated to "2 TS … cheats remain".
- `git grep "tickRate" modules/ pkg/` shows the new field references; no stale `const tickRate` reference remains.
- Closing commit body includes `Closes memory:` trailer per memory `close_commit_memory_trailer`.

## §11 Plan-author worksheet

Plan should split into tasks roughly:

- **T1** (RED) — write the six `TestCheatSpeed_*` cases + `TestTickLoopHonoursFieldRate` against the unported handler. All must fail (no `case "speed":` exists). Run `-race`.
- **T2** (GREEN) — add `defaultTickRate` const, `Server.tickRate` field, constructor init, `runTickLoopWithRate` re-read shape, and the `case "speed":` body. All §7 tests pass. Existing `TestTickLoopIncrementsCurrentTick` still passes.
- **T3** (DOCS) — rewrite the `DEVIATION-NAI-187-D1-CARRYFORWARD` comment block to drop the `speed:` row and update the tally line.
- **CLOSE** — `chore(close): NAI-188 …` with `Closes memory:` trailer if any new tracker entries are warranted (likely none — this is straight-port with established patterns).

Plan-author must grep `&Server{` in `modules/world/` at plan-write time and enumerate any direct-literal sites that need the tickRate field set (per the §8 mitigation).
