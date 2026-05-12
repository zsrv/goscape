# NAI-183 — Port TS ClientCheatHandler outer NODE_PRODUCTION + role-level guards

**Status:** Spec
**Date:** 2026-05-12
**TS source:** `LostCityRS/Engine-TS` `src/network/game/client/handler/ClientCheatHandler.ts`
**Tech stack:** Go 1.26+

## 1. Goal

Replace goscape's per-arm `staffModLevel < 2` gates in `modules/world/handlers_game.go` with two TS-faithful outer-guard blocks mirroring `ClientCheatHandler.ts:52,56,483`. Retire `DEVIATION-NAI-182-D2-CHEAT-NODE-PRODUCTION-GATE` (4 references). Port the TS `addSessionLog` tier (line 52-54) using the existing `Player.AddSessionLog` infra.

## 2. Background

NAI-182 B6 added `::reboot`, `::slowreboot`, `::serverdrop` cheats with per-arm `staffModLevel < 2` gates (mirroring the existing `::tele`/`::getcoord` pattern), tagged as `DEVIATION-NAI-182-D2-CHEAT-NODE-PRODUCTION-GATE` because TS gates them under `!Environment.NODE_PRODUCTION && player.staffModLevel >= 4` instead.

Pre-flight against `LostCityRS/Engine-TS` reveals:

- TS has **four distinct gates** in `handleAcknowledged`, not one outer guard:
  - L52: `if (player.staffModLevel >= 2)` → `addSessionLog` (one statement)
  - L56: `if (!Environment.NODE_PRODUCTION && player.staffModLevel >= 4)` → developer block (debugproc, getvar, setvarother, broadcast, **reboot**, **slowreboot**, **serverdrop**, teleother, teleto, setvis, ban, mute, kick)
  - L189: `if (player.staffModLevel >= 3)` → admin block (setvar)
  - L483: `if (player.staffModLevel >= 2)` → super-mod block (**getcoord**, **tele**)
- The dev-block arms `::reboot` and `::slowreboot` carry inner `&& Environment.NODE_PRODUCTION` clauses *inside* the outer `!NODE_PRODUCTION` block. Inside that outer block `NODE_PRODUCTION` is provably false, so these arms are **literal dead code** in TS. Only `::serverdrop` (no inner clause) actually fires. Likely a refactor artifact (variable renamed/inverted, inner clauses never updated).
- `::say` is **not in TS ClientCheatHandler** at all — it's a separate ChatHandler. Goscape's `case "say"` arm has no TS-equivalent gate to mirror.

Goscape state at HEAD `5a79f62`:

- Per-arm `if p.staffModLevel < 2 { return nil }` on: `::getcoord` (handlers_game.go:374), `::tele` (:382), `::reboot` (:434), `::slowreboot` (:445), `::serverdrop` (:459).
- `::say` (handlers_game.go:368-371) has no gate.
- Session-log infra exists: `Player.AddSessionLog(eventType, message string, args ...string)` at `modules/world/player.go:1321`. `LoggerEventTypeModerator = 2` at `modules/world/session_log.go`.
- `s.cfg.NodeProduction` field exists at `modules/world/config.go:43` (yaml `node_production`, default `false`). Existing consumers: `handler_reportabuse.go:59`, `server_varp.go:80`. `newTestServer` builds `&Server{...}` without populating `cfg`, so the field is zero-value (`false`) by default in tests.

## 3. Approach (selected: A — strict TS-faithful, dead code preserved)

Restructure `handleClientCheat` (handlers_game.go:336-467) into three sequential top-level blocks mirroring TS L52, L56, L483, plus an ungated tail switch for `::say`.

### 3.1 Target shape

```go
func handleClientCheat(p *Player, payload []byte) error {
    // ... existing setup: r := packet.New(payload); r.G1(); cheat read; parts split; empty check ...

    // TS L52-54: addSessionLog tier.
    if p.staffModLevel >= 2 {
        p.AddSessionLog(LoggerEventTypeModerator, "Ran cheat", cheat)
    }

    // TS L56: developer block (NODE_PRODUCTION-gated).
    if !p.client.server.cfg.NodeProduction && p.staffModLevel >= 4 {
        switch parts[0] {
        case "reboot":
            // TS-faithful dead code: outer block runs only when NodeProduction=false,
            // so this inner clause never fires. Mirrors TS quirk at
            // ClientCheatHandler.ts:360 (likely refactor artifact).
            if p.client.server.cfg.NodeProduction {
                s := p.client.server
                s.rebootTimer(0)
            }
        case "slowreboot":
            // Same TS dead-code pattern as ::reboot — see comment above.
            if p.client.server.cfg.NodeProduction {
                seconds := parseIntOr(args, 30)
                ticks := int(math.Ceil(float64(seconds) * 1000.0 / 600.0))
                s := p.client.server
                s.rebootTimer(ticks)
            }
        case "serverdrop":
            // No inner clause in TS — actually fires under !NodeProduction.
            if p.client != nil && p.client.conn != nil {
                _ = p.client.conn.Close()
            }
        }
    }

    // TS L483: super-mod block.
    if p.staffModLevel >= 2 {
        switch parts[0] {
        case "getcoord":
            p.MessageGame(coordgrid.FormatString(p.level, p.x, p.z, ","))
        case "tele":
            // ... existing ::tele body unchanged (lines 394-426 of HEAD) ...
        }
    }

    // Ungated arms (no TS counterpart in ClientCheatHandler).
    switch parts[0] {
    case "say":
        if args != "" {
            p.Say([]byte(args))
        }
    }

    return nil
}
```

The DEVIATION-NAI-182-D3-OTHER-CHEATS comment block at handlers_game.go:360-366 is preserved verbatim above the first gate block.

### 3.2 Behavioral matrix (default config: `NodeProduction=false`)

| Cheat        | Min staffModLevel | Fires? | Notes                                            |
|--------------|-------------------|--------|--------------------------------------------------|
| `::say`      | any               | yes    | Ungated; not in TS ClientCheatHandler            |
| `::getcoord` | ≥2                | yes    | Super-mod block (TS L483)                        |
| `::tele`     | ≥2                | yes    | Super-mod block; existing body unchanged          |
| `::reboot`   | ≥4                | **NO (dead)** | Inner `&& NodeProduction` blocks under outer `!NodeProduction` |
| `::slowreboot` | ≥4              | **NO (dead)** | Same                                             |
| `::serverdrop` | ≥4              | yes    | No inner clause in TS                            |

Under `NodeProduction=true`: the entire developer block short-circuits, so all three reboot-cohort cheats go silent. Matches TS exactly.

## 4. Deviations

### 4.1 Retired

- `DEVIATION-NAI-182-D2-CHEAT-NODE-PRODUCTION-GATE` — 4 grep hits at HEAD (handlers_game.go:431, :444, :458; handlers_game_test.go:630). Retire all in the close commit; verify post-commit `rg "DEVIATION-NAI-182-D2-CHEAT-NODE-PRODUCTION-GATE" modules/ pkg/ cmd/` returns zero.

### 4.2 Opened

None. Session-log infra exists, `::say` is genuinely outside TS ClientCheatHandler, and the dead-code preservation is structural fidelity (TS literally has it) rather than a goscape divergence.

## 5. Test plan

Located in `modules/world/handlers_game_test.go`. Existing helpers `teleTestPlayer`, `dispatchTeleCheat`, `drainConn`, `isaacPair` reused without modification.

### 5.1 Modified tests (5)

1. **`TestHandleClientCheat_Reboot_TriggersImmediateBroadcast`** — currently asserts ::reboot fires at `staffModLevel=2`. **Inverted**: with `staffModLevel=4` and `NodeProduction=false`, ::reboot is dead → assert `s.shutdownTick == -1` (unchanged from `newTestServer` initial value) and no UPDATE_REBOOT_TIMER bytes were broadcast. Rename to `TestHandleClientCheat_Reboot_DeadUnderDefaultConfig`.
2. **`TestHandleClientCheat_SlowReboot_NoArgsDefaultsTo30Seconds`** — same inversion. Rename to `TestHandleClientCheat_SlowReboot_NoArgs_DeadUnderDefaultConfig`.
3. **`TestHandleClientCheat_SlowReboot_WithSecondsArg`** — same inversion. Rename to `TestHandleClientCheat_SlowReboot_WithArg_DeadUnderDefaultConfig`.
4. **`TestHandleClientCheat_SlowReboot_NonIntegerArgFallsBackToDefault`** — same inversion. Rename to `TestHandleClientCheat_SlowReboot_NonInteger_DeadUnderDefaultConfig`.
5. **`TestHandleClientCheat_ServerDrop_ClosesConn`** — change setup `p.staffModLevel = 2` → `4`, otherwise assertions unchanged (still fires; conn closes).
6. **`TestHandleClientCheat_RebootCheats_StaffGate`** — gate boundary `<2` → `<4`. Setup `p.staffModLevel = 3` (was `1`). Remove DEVIATION-NAI-182-D2 reference from doc-comment; rewrite to "outer-block staff gate per TS L56".

### 5.2 Added tests (3)

1. **`TestHandleClientCheat_AddsSessionLogAtModLevel2`** — set `p.staffModLevel = 2`; dispatch an unrecognized cheat (e.g. `dispatchTeleCheat(t, p, "foo")`) so no arm body fires and the test isolates the L52 tier. Assert `len(s.sessionLogs) == 1`, `s.sessionLogs[0].EventType == LoggerEventTypeModerator`, `s.sessionLogs[0].Event == "Ran cheat foo"` (per `AddSessionLog` join semantics: `message + " " + strings.Join(args, " ")`; goscape `cheat` is the lowercased input WITHOUT the stripped `::` prefix per handlers_game.go:345-347). Below modLevel 2 (e.g. `staffModLevel = 1`) companion assertion in same test: `len(s.sessionLogs) == 0` after dispatch.
2. **`TestHandleClientCheat_ServerDrop_StaffGate`** — sibling of the renamed reboot gate test for completeness. `p.staffModLevel = 3`; dispatch ::serverdrop; assert conn still open and player still in slot.
3. **`TestHandleClientCheat_NodeProductionTrue_DevBlockShortCircuits`** — set `s.cfg.NodeProduction = true`, `p.staffModLevel = 4`, dispatch ::serverdrop, assert conn NOT closed (proves outer `!NodeProduction` collapses the dev block). Companion assertion: `s.sessionLogs` still gets the modLevel-2 log entry (proves only the dev block short-circuits, not the L52 tier).

### 5.3 Unchanged tests

All `TestHandleClientCheat_Tele_*` and `TestHandleClientCheat_GetCoord_*` tests stay byte-identical — gate predicate is still `>=2`, body unchanged.

## 6. Files touched

- `modules/world/handlers_game.go` — restructure `handleClientCheat` body (lines 367-466 of HEAD).
- `modules/world/handlers_game_test.go` — modify 6 tests, add 3 tests, retire D2 doc-comment reference at line 630.

No new files. No changes to `parseIntOr`, `dispatchTeleCheat`, `teleTestPlayer`, fixture helpers, config schema, or any other module.

## 7. Memory-driven controller pre-flight checklist

Per `controller_preflight` and `plan_sibling_site_guard_audit` memories, before each implementer dispatch:

- [ ] `rg "DEVIATION-NAI-182-D2-CHEAT-NODE-PRODUCTION-GATE" modules/ pkg/ cmd/` returns the same 4 hits at dispatch time.
- [ ] `p.client.server.cfg.NodeProduction` access compiles (sibling guard pattern at `handler_reportabuse.go:59` and `server_varp.go:80` confirms field name + access path).
- [ ] `AddSessionLog` signature `(eventType LoggerEventType, message string, args ...string)` matches the `"Ran cheat", cheat` call site.
- [ ] `newTestServer` zero-value `cfg` confirmed (no helper override needed for `NodeProduction=false` tests).
- [ ] Per `dead_api_polish` memory: confirm no helpers ship with zero consumers post-restructure (the inner `if p.client.server.cfg.NodeProduction { ... }` clauses inside the dev block are deliberately dead, not unused-API).

## 8. Out of scope

- The 25 unported cheats listed in DEVIATION-NAI-182-D3-OTHER-CHEATS stay deferred. NAI-183 only restructures gates + retires D2 + ports addSessionLog tier.
- No new cheats added.
- No `addSessionLog` calls inside individual arm bodies (TS only logs at the L52-54 gate-tier check, not per-arm).
- No changes to `parseIntOr`, `dispatchTeleCheat`, or other test helpers.
- No changes to `::tele`/`::getcoord` bodies — they move under the `>=2` outer block but their internals are byte-identical.
- No port of the TS L189 `>= 3` admin block (no goscape-side admin cheats yet; deferred to NAI-N+1 if/when ported).
