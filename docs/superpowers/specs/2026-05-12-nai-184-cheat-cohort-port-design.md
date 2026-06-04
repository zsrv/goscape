# NAI-184 — Port tractable D3 cheat cohort + correct NAI-183 reboot-trio outer-block

**Status:** Spec
**Date:** 2026-05-12
**TS source:** `LostCityRS/Engine-TS` `src/network/game/client/handler/ClientCheatHandler.ts`
**Tech stack:** Go 1.26+

## 1. Goal

Port 11 of the 26 unported `ClientCheatHandler` cheats tracked by `DEVIATION-NAI-182-D3-OTHER-CHEATS`, and correct a TS-fidelity bug introduced by NAI-183 that placed `reboot`/`slowreboot`/`serverdrop` in the wrong outer block. The two changes share a single structural edit (adding the L189 admin `>=3` outer guard block to `handleClientCheat`), so they are bundled into one sub-spec rather than split.

## 2. Background

### 2.1 NAI-183 misclassification (audit finding)

NAI-183 restructured `handleClientCheat` into three sequential outer-guard blocks mirroring TS L52, L56, L483. The spec pre-flight at NAI-183 §2 enumerated the L56 dev block contents as including `reboot`, `slowreboot`, `serverdrop`, `teleother`, `teleto`, `setvis`, `ban`, `mute`, `kick`. **This enumeration was incorrect.** Re-reading the TS source line-by-line:

- L56 opens the `!Environment.NODE_PRODUCTION && player.staffModLevel >= 4` dev block.
- L187 closes the dev block. Its contents: `debugproc`, `reload`, `rebuild`, `speed`, `fly`, `naive`, `random`.
- L189 opens a new `player.staffModLevel >= 3` admin block.
- L481 closes the admin block. Its contents: `setvar`, `setvarother (&& NP)`, `getvar`, `getvarother (&& NP)`, `give`, `giveother (&& NP)`, `givecrap`, `givemany`, `broadcast (&& NP)`, **`reboot (&& NP)`**, **`slowreboot (&& NP)`**, **`serverdrop`**, `teleother (&& NP)`, `setstat`, `advancestat`, `minme`, `locadd`, `npcadd`, `openmain`, `snapshot`.
- L483 opens the super-mod `>=2` block. Contents: `getcoord`, `tele`, `teleto (&& NP)`, `setvis (&& NP)`, `ban (&& NP)`, `mute (&& NP)`, `kick (&& NP)`.

NAI-183 placed the reboot trio in the dev block. Because the dev block's outer guard is `!NodeProduction` and `reboot`/`slowreboot` carry an inner `&& NodeProduction`, NAI-183 rationalized the misplacement as "TS-faithful dead code" — but in TS-correct placement (admin `>=3` with no outer NP gate, inner `&& NodeProduction`), `reboot`/`slowreboot` fire under `NodeProduction=true` at staff tier 3+, and `serverdrop` fires at staff tier 3+ regardless of NP.

The fix is to relocate the trio to the admin block. Since NAI-184 needs the admin block anyway for `setstat` et al., the corrections are bundled.

### 2.2 NAI-182 D3 count correction

The `DEVIATION-NAI-182-D3-OTHER-CHEATS` enumeration at NAI-182 close listed 25 unported cheats but **omitted `setvar`**. Counting TS L189-481 admin-block arms: 20 distinct arms. NAI-182 ported reboot, slowreboot, serverdrop (3, into the wrong outer block per §2.1 but still ported). Pre-NAI-184 unported in admin block = 20 − 3 = 17. Dev-block arms (excluding debugproc, which is a scripting feature not a cheat): 6 (`reload`, `rebuild`, `speed`, `fly`, `naive`, `random`). All 6 unported. Super-mod-block arms beyond `getcoord`/`tele`: 5 (`teleto`, `setvis`, `ban`, `mute`, `kick`). All 5 unported. **Total unported = 17 + 6 + 5 = 28. Of those, 2 are sessionLog tier-only (none here) and 0 are not cheats (debugproc is the only excluded item). True count = 28.**

Wait — recount with the exhaustive grep: TS admin block contains `setvar`, `setvarother`, `getvar`, `getvarother`, `give`, `giveother`, `givecrap`, `givemany`, `broadcast`, `reboot`, `slowreboot`, `serverdrop`, `teleother`, `setstat`, `advancestat`, `minme`, `locadd`, `npcadd`, `openmain`, `snapshot` = 20 arms. NAI-182 ported reboot/slowreboot/serverdrop = 3. Admin unported = 17.

Dev block (excluding debugproc): `reload`, `rebuild`, `speed`, `fly`, `naive`, `random` = 6.

Super-mod block beyond getcoord/tele: `teleto`, `setvis`, `ban`, `mute`, `kick` = 5.

**True unported cheat count at HEAD `1920a1d` = 17 + 6 + 5 = 28.** NAI-182 D3 listed 25, missing `setvar`, `getvar`, `getvarother` (wait — D3 does include getvar and getvarother; let me recount the D3 list).

NAI-182 D3 list verbatim: "reload, rebuild, speed, fly, naive, random, setvarother, getvar, getvarother, give, giveother, givecrap, givemany, broadcast, teleother, setstat, advancestat, minme, locadd, npcadd, openmain, snapshot, teleto, setvis, ban, mute, kick" = 27 entries. Missing: `setvar`. **Actual unported = 28, D3 listed 27, omitted 1 (`setvar`).** NAI-184 closes 11. Remaining = 28 − 11 = **17 unported after NAI-184**.

(Per `audit_arithmetic_correction_in_rollup.md`: this arithmetic is being shown in the spec body so the close commit's rollup can be cross-footed against it.)

### 2.3 Cheats ported by NAI-184

Selected for "infra already wired in goscape" tractability:

- **Dev block** (`!NP && >=4`): `fly`, `naive`, `random` (zero new infra)
- **Admin block** (`>=3`): `setstat`, `advancestat`, `minme`, `give`, `givemany`, `snapshot`, `teleother (&& NP)` (one new helper + `PlayerStatMap`)
- **Super-mod block** (`>=2`): `teleto (&& NP)` (zero new infra)

### 2.4 Cheats deferred (carry-forward into D3)

17 cheats remain blocked on missing subsystems; each warrants its own follow-up sub-spec:

| Cheat | Block | Blocking subsystem |
|---|---|---|
| `reload`, `rebuild` | dev | cache reload + script rebuild infrastructure |
| `speed` | dev | runtime tick-rate mutation (currently `const` at `tick.go:15`) |
| `setvar`, `setvarother`, `getvar`, `getvarother` | admin | `VarPlayerType.GetByName` (no name index on varptype) |
| `giveother` | admin | cross-player inventory grant (cross-player + `Player.InvAddFull` once landed) |
| `givecrap` | admin | `ObjType.Count` iteration + members/dummyitem/certtemplate filters + `NodeMembers` flag |
| `broadcast` | admin | `World.broadcastMes` (iterate-all-players helper) |
| `locadd`, `npcadd` | admin | dynamic Loc/NPC spawn at player coord with `LocType.GetByName` / `NpcType.GetByName` lookup |
| `openmain` | admin | `Component.GetByName` + `openMainModal` |
| `setvis`, `ban`, `mute`, `kick` | super-mod | `Player.SetVisibility` plumbing (rsbuf `Visibility` exists but not wired) + login-server moderation callbacks for ban/mute, `loggingOut`+conn-close orchestration for kick |

### 2.5 Goscape state at HEAD `1920a1d`

- `handleClientCheat` at `modules/world/handlers_game.go:336-467`. Three outer-guard blocks: sessionLog L371-373, dev L379-412 (contains reboot/slowreboot dead and serverdrop alive), super-mod L416-466 (contains getcoord and tele).
- `pkg/objtype/playerstat.go` exports `PlayerStat<Name>` constants for the 21 slots, `PlayerStatCount=21`, `GetExpByLevel`, `GetLevelByExp`, `MaxXP`, `MaxSkillXP`. **No** `PlayerStatMap` or `PlayerStatEnabled` yet.
- `Player.SetCurLevel(id, level)` writes ONLY `levels[id]`. **No** `Player.SetStat` (TS `setLevel`-equivalent).
- `Player.AddXP(id, xp)` exists and fires `[changestat,X]` + `[advancestat,X]` triggers on level-up.
- `Server.LookupPlayerByUsername(name) *Player` exists at `server.go:891`.
- `inventory.Inventory.Add(id, count, AddOpts{...})` exists at `pkg/inventory/inventory.go:180`.
- `MoveStrategySmart/Naive/Fly` at `modules/world/movement_consts.go:31-33`. `Player.moveStrategy` field; no method to toggle.
- `Player.afkEventReady` field with `SetAfkEventReady(v bool)` setter (`player_script.go:490`).
- `s.cfg.NodeProduction` at `config.go:43`, zero-value `false` in `newTestServer`.
- `Player.AddSessionLog(eventType, message, args...)` at `player.go:1321`.
- `Player.TeleJump(x, z, level)` at `player_script.go:507`.
- `sendUnsetMapFlag(p)` + `p.waypointIndex = -1` is the canonical TS `unsetMapFlag()` port (see handlers_game.go:447-448).

## 3. Approach (selected: A — fat port + NAI-183 correction)

Restructure `handleClientCheat` into four sequential outer-guard blocks:

```go
func handleClientCheat(p *Player, payload []byte) error {
    // ... existing setup: read input, length check, lowercase, split, empty check ...
    // ... DEVIATION-NAI-182-D3 comment, updated to enumerate the 17 carry-forwards ...

    // TS L52-54 — sessionLog tier (unchanged).
    if p.staffModLevel >= 2 {
        p.AddSessionLog(LoggerEventTypeModerator, "Ran cheat", cheat)
    }

    // TS L56-187 — dev block (!NP && >=4).
    if !p.client.server.cfg.NodeProduction && p.staffModLevel >= 4 {
        switch parts[0] {
        case "fly":          /* see §3.1 */
        case "naive":        /* see §3.1 */
        case "random":       /* see §3.1 */
        }
    }

    // TS L189-481 — admin block (>=3) — NEW outer guard.
    if p.staffModLevel >= 3 {
        switch parts[0] {
        case "setstat":      /* see §3.2 */
        case "advancestat":  /* see §3.2 */
        case "minme":        /* see §3.2 */
        case "give":         /* see §3.3 */
        case "givemany":     /* see §3.3 */
        case "snapshot":     /* see §3.4 */
        case "teleother":    /* see §3.5; inner && NP */
        case "reboot":       /* moved from dev block; inner && NP */
        case "slowreboot":   /* moved from dev block; inner && NP */
        case "serverdrop":   /* moved from dev block; no NP */
        }
    }

    // TS L483- — super-mod block (>=2).
    if p.staffModLevel >= 2 {
        switch parts[0] {
        case "getcoord":     /* unchanged */
        case "tele":         /* unchanged */
        case "teleto":       /* see §3.5; inner && NP */
        }
    }

    // Ungated tail (no TS counterpart).
    switch parts[0] {
    case "say":              /* unchanged */
    }

    return nil
}
```

Each block is an independent `if` (not `else if`). The sessionLog tier runs for every cheat at `>=2`; the gate-block arms fire only if their parts[0] matches.

### 3.1 Dev-block arms (fly / naive / random)

Direct ports of TS L168-186.

```go
case "fly":
    if p.moveStrategy == MoveStrategyFly {
        p.moveStrategy = MoveStrategySmart
    } else {
        p.moveStrategy = MoveStrategyFly
    }
    if p.moveStrategy == MoveStrategyFly {
        p.MessageGame("Changed move strategy: fly")
    } else {
        p.MessageGame("Changed move strategy: smart")
    }

case "naive":
    if p.moveStrategy == MoveStrategyNaive {
        p.moveStrategy = MoveStrategySmart
    } else {
        p.moveStrategy = MoveStrategyNaive
    }
    if p.moveStrategy == MoveStrategyNaive {
        p.MessageGame("Naive move strategy: naive")
    } else {
        p.MessageGame("Naive move strategy: smart")
    }

case "random":
    p.afkEventReady = true
```

### 3.2 Admin-block stat arms (setstat / advancestat / minme)

Depend on new `PlayerStatMap` and new `Player.SetStat`.

```go
case "setstat":
    sub := strings.SplitN(args, " ", 2)
    if args == "" || len(sub) < 2 { return nil }
    stat, ok := objtype.PlayerStatMap[strings.ToUpper(sub[0])]
    if !ok { return nil }
    level := parseIntOr(sub[1], 0)
    p.SetStat(stat, level)

case "advancestat":
    sub := strings.SplitN(args, " ", 2)
    if args == "" { return nil }
    stat, ok := objtype.PlayerStatMap[strings.ToUpper(sub[0])]
    if !ok { return nil }
    levelStr := ""
    if len(sub) > 1 { levelStr = sub[1] }
    level := parseIntOr(levelStr, 0)
    // TS L428-431 — zero out, then AddXP to target. AddXP fires
    // [changestat,X] and [advancestat,X] triggers on level-up
    // (existing Player.AddXP behavior, NAI-144).
    p.stats[stat] = 0
    p.baseLevels[stat] = 1
    p.levels[stat] = 1
    p.AddXP(stat, objtype.GetExpByLevel(level))

case "minme":
    for i := 0; i < objtype.PlayerStatCount; i++ {
        if !objtype.PlayerStatEnabled[i] { continue }
        if i == objtype.PlayerStatHitpoints {
            p.SetStat(i, 10)
        } else {
            p.SetStat(i, 1)
        }
    }
```

### 3.3 Admin-block inventory arms (give / givemany)

Depend on a new thin wrapper that grants items into the player's `inv` inventory. TS calls `player.invAdd(InvType.INV, obj, count, false)` where `false` is the "safe" arg meaning "drop overflow on the floor." Goscape's existing inventory API is `inventory.Inventory.Add(id, count, AddOpts{...})`.

```go
case "give":
    sub := strings.SplitN(args, " ", 2)
    if args == "" { return nil }
    name := sub[0]
    obj := p.client.server.objTypes.ByName(name)   // exact accessor TBD at plan time
    if obj == nil { return nil }
    count := 1
    if len(sub) > 1 {
        count = parseIntOr(sub[1], 1)
        if count < 1 { count = 1 }
        if count > 0x7fffffff { count = 0x7fffffff }
    }
    p.InvAddFull(invID, obj.id, count)   // wrapper name + signature confirmed at plan time

case "givemany":
    sub := strings.SplitN(args, " ", 2)
    if args == "" { return nil }
    obj := p.client.server.objTypes.ByName(sub[0])
    if obj == nil { return nil }
    p.InvAddFull(invID, obj.id, 1000)
```

The wrapper's exact name (`InvAddFull` proposed) and routing (direct `inventory.Add` vs reuse of `pkg/script.performInvAdd`) are plan-time decisions. The contract for the spec is: **TS `player.invAdd(InvType.INV, obj, count, false)` semantics — adds `count` of `obj` to the player's main inventory; on overflow, drops the surplus on the floor.** Plan must verify byte-parity with `performInvAdd` for the simple "fits in inv" case.

### 3.4 Admin-block snapshot arm

```go
case "snapshot":
    path := filepath.Join(os.TempDir(), fmt.Sprintf("heap-%d.pprof", time.Now().Unix()))
    if f, err := os.Create(path); err == nil {
        defer f.Close()
        if err := pprof.WriteHeapProfile(f); err == nil {
            p.client.server.log.Info("heap snapshot written", "path", path)
        }
    }
```

TS uses v8's heap snapshot (JSON-ish). Go's `pprof.WriteHeapProfile` produces a different format but serves the same purpose (admin-tier debug helper). No deviation tag — TS-fidelity here is dispatch behavior, not byte format.

### 3.5 Cross-player tele arms (teleother / teleto)

Both NP-gated via inner `if !cfg.NodeProduction { break }` to keep the case-body's NP intent visible.

```go
case "teleother":
    if !p.client.server.cfg.NodeProduction { break }
    if args == "" { return nil }
    other := p.client.server.LookupPlayerByUsername(args)
    if other == nil {
        p.MessageGame(fmt.Sprintf("%s is not logged in.", args))
        return nil
    }
    other.CloseModal(true)
    if !other.CanAccess() {
        p.MessageGame(fmt.Sprintf("%s is busy right now.", args))
        return nil
    }
    other.ClearInteraction()
    sendUnsetMapFlag(other)
    other.waypointIndex = -1
    other.TeleJump(p.x, p.z, p.level)

case "teleto":
    if !p.client.server.cfg.NodeProduction { break }
    if args == "" { return nil }
    other := p.client.server.LookupPlayerByUsername(args)
    if other == nil {
        p.MessageGame(fmt.Sprintf("%s is not logged in.", args))
        return nil
    }
    p.CloseModal(true)
    if !p.CanAccess() {
        p.MessageGame("Please finish what you are doing first.")
        return nil
    }
    p.ClearInteraction()
    sendUnsetMapFlag(p)
    p.waypointIndex = -1
    p.TeleJump(other.x, other.z, other.level)
```

### 3.6 Reboot trio relocation

Move `reboot`, `slowreboot`, `serverdrop` from the dev block into the admin block. Bodies unchanged from NAI-183. Retire the "TS-faithful dead code" comments — under TS-correct placement, these arms are NOT dead.

```go
// In admin block:
case "reboot":
    // TS L360-364. Production-only via inner && NodeProduction; under
    // default NodeProduction=false, this arm is dead.
    if p.client.server.cfg.NodeProduction {
        p.client.server.rebootTimer(0)
    }

case "slowreboot":
    // TS L365-373. Production-only; default 30s when args missing.
    if p.client.server.cfg.NodeProduction {
        seconds := parseIntOr(args, 30)
        ticks := int(math.Ceil(float64(seconds) * 1000.0 / 600.0))
        p.client.server.rebootTimer(ticks)
    }

case "serverdrop":
    // TS L374-376 player.terminate(). No NP gate — fires at >=3
    // regardless of NodeProduction. Closes the TCP conn without
    // removing the player from s.players; the next reconnect hits
    // this player's slot and runs the onReconnect path.
    if p.client != nil && p.client.conn != nil {
        _ = p.client.conn.Close()
    }
```

## 4. New infra

### 4.1 `pkg/objtype/playerstat.go` additions

Add to existing file:

```go
// PlayerStatMap maps uppercase stat name → stat index. Mirrors TS
// PlayerStatMap (PlayerStat.ts:25-47). Used by ::setstat / ::advancestat
// cheat parsing in modules/world/handlers_game.go.
var PlayerStatMap = map[string]int{
    "ATTACK":      PlayerStatAttack,
    "DEFENCE":     PlayerStatDefence,
    "STRENGTH":    PlayerStatStrength,
    "HITPOINTS":   PlayerStatHitpoints,
    "RANGED":      PlayerStatRanged,
    "PRAYER":      PlayerStatPrayer,
    "MAGIC":       PlayerStatMagic,
    "COOKING":     PlayerStatCooking,
    "WOODCUTTING": PlayerStatWoodcutting,
    "FLETCHING":   PlayerStatFletching,
    "FISHING":     PlayerStatFishing,
    "FIREMAKING":  PlayerStatFiremaking,
    "CRAFTING":    PlayerStatCrafting,
    "SMITHING":    PlayerStatSmithing,
    "MINING":      PlayerStatMining,
    "HERBLORE":    PlayerStatHerblore,
    "AGILITY":     PlayerStatAgility,
    "THIEVING":    PlayerStatThieving,
    "STAT18":      PlayerStat18,
    "STAT19":      PlayerStat19,
    "RUNECRAFT":   PlayerStatRunecraft,
}

// PlayerStatEnabled mirrors TS PlayerStat.ts:53. False entries (STAT18,
// STAT19) are unused 2004-era reserved slots; ::minme skips them.
var PlayerStatEnabled = [PlayerStatCount]bool{
    true, true, true, true, true, true, true, true, true, true,
    true, true, true, true, true, true, true, true, false, false, true,
}
```

### 4.2 `Player.SetStat(stat, level int)` (new method)

Mirrors TS `Player.setLevel` (Player.ts:1823-1834). Writes `baseLevels`, `levels`, and `stats` (XP) for the given slot.

```go
// SetStat clamps level to [1, 99] and writes baseLevels, levels, and
// stats (XP) for the given stat slot. Mirrors TS Player.setLevel
// (Player.ts:1823-1834). Used by ::setstat and ::minme cheats.
//
// DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD: TS calls
// buildAppearance(appearanceInv) on combatLevel change. Combat-level
// recompute + appearance rebuild are deferred to NAI-N+1 (combat
// sub-spec).
func (p *Player) SetStat(stat, level int) {
    if !statBounds(stat) { return }
    if level < 1 { level = 1 }
    if level > 99 { level = 99 }
    p.baseLevels[stat] = uint8(level)
    p.levels[stat] = uint8(level)
    p.stats[stat] = int32(objtype.GetExpByLevel(level))
}
```

### 4.3 `Player.InvAddFull(invID, objID, count int)` (new method)

Thin wrapper for cheat-handler inventory grants. Mirrors TS `player.invAdd(InvType.INV, obj, count, false)` semantics. The implementation route (direct `inventory.Add` vs reuse of `pkg/script.performInvAdd`) is plan-time. Spec contract: overflow drops on the floor.

**Plan must verify**: byte-parity with `performInvAdd` for the simple in-inv case (no overflow); floor-drop behavior on overflow.

### 4.4 No infra changes outside the above

- `MoveStrategy*` already exists.
- `Player.afkEventReady` already has setter.
- `Server.LookupPlayerByUsername` already exists.
- `Player.TeleJump`, `CloseModal`, `CanAccess`, `ClearInteraction`, `sendUnsetMapFlag`, `waypointIndex = -1` all exist (used by `::tele`).
- `runtime/pprof` is stdlib.

## 5. Test plan

All tests in `modules/world/handlers_game_test.go`. Existing helpers reused.

### 5.1 Modified tests (6)

Reboot-trio relocation inverts the `staffModLevel` gate boundary from `<4` to `<3` and (for `serverdrop`) removes the `!NP` outer guard. The literal expectation bytes for default-config (`NP=false`) test cases are mostly unchanged (reboot/slowreboot still dead under `NP=false`); the gate boundary moves down by one staff tier.

1. `TestHandleClientCheat_Reboot_DeadUnderDefaultConfig` — change setup `p.staffModLevel = 4` → `3`. Assertion unchanged (still dead at default config). No rename — the existing name remains accurate (NAI-183 already established the name).
2. `TestHandleClientCheat_SlowReboot_NoArgs_DeadUnderDefaultConfig` — `4` → `3`.
3. `TestHandleClientCheat_SlowReboot_WithArg_DeadUnderDefaultConfig` — `4` → `3`.
4. `TestHandleClientCheat_SlowReboot_NonInteger_DeadUnderDefaultConfig` — `4` → `3`.
5. `TestHandleClientCheat_ServerDrop_ClosesConn` — change setup `p.staffModLevel = 4` → `3`. Assertion unchanged (still fires; conn closes).
6. `TestHandleClientCheat_RebootCheats_StaffGate` — gate boundary `<4` → `<3`. Setup `p.staffModLevel = 2` (was `3`). Rewrite doc-comment to "admin-block staff gate per TS L189".

### 5.2 Deleted tests (1)

7. `TestHandleClientCheat_NodeProductionTrue_DevBlockShortCircuits` — DELETE. Its premise was that `NodeProduction=true` collapses the dev block (NAI-183 placement of serverdrop). Under TS-correct placement, serverdrop is in the admin block with no NP outer guard — `NP=true` does NOT collapse its dispatch. The test's expected behavior no longer matches the new TS-faithful semantics.

### 5.3 Added tests (12)

Each new arm gets one functional test plus one gate-boundary test where applicable.

**Dev block (3 functional):**

8. `TestHandleClientCheat_Fly_TogglesStrategy` — modLevel=4, NP=false; pre-state `moveStrategy=Smart`; dispatch `::fly`; assert `moveStrategy==Fly` + MessageGame includes "Changed move strategy: fly". Re-dispatch; assert back to Smart + message "Changed move strategy: smart".
9. `TestHandleClientCheat_Naive_TogglesStrategy` — symmetric for `Naive`. Message text "Naive move strategy: naive" / "Naive move strategy: smart".
10. `TestHandleClientCheat_Random_SetsAfkEventReady` — clear `afkEventReady`; dispatch `::random`; assert `afkEventReady==true`.

**Admin block (7 functional + 1 gate-boundary = 8 total):**

11. `TestHandleClientCheat_SetStat_SetsBaseCurAndXP` — modLevel=3; dispatch `::setstat attack 50`; assert `baseLevels[PlayerStatAttack]==50`, `levels[PlayerStatAttack]==50`, `stats[PlayerStatAttack]==int32(GetExpByLevel(50))`. Companion (same test): unknown stat name → no mutation.
12. `TestHandleClientCheat_AdvanceStat_ZerosThenAddsXP` — modLevel=3; pre-populate `stats[PlayerStatAttack]=999999`, `levels`/`baseLevels` to non-1 values; dispatch `::advancestat attack 50`. After call: `stats[PlayerStatAttack]==int32(GetExpByLevel(50))`, `baseLevels[PlayerStatAttack]==50`, `levels[PlayerStatAttack]==50` (from AddXP's un-buffed branch since pre-state was levels==baseLevels after the reset). Verify `[changestat,attack]` and `[advancestat,attack]` triggers were enqueued (via test fixture's script provider — pattern from existing AddXP tests).
13. `TestHandleClientCheat_MinMe_AllEnabledStatsSetTo1ExceptHitpoints` — modLevel=3; pre-populate all 21 stats high; dispatch `::minme`; assert: every enabled stat (per `PlayerStatEnabled`) = 1 except HITPOINTS = 10; STAT18/STAT19 unchanged from pre-state.
14. `TestHandleClientCheat_Give_AddsToInv` — modLevel=3; dispatch `::give <known-obj-name> 5`; assert inv slot 0 has that obj with count 5. Companion: unknown name → no inv mutation. Companion: missing count → defaults to 1.
15. `TestHandleClientCheat_GiveMany_Adds1000` — modLevel=3; dispatch `::givemany <known-obj-name>`; assert inv contains 1000 of that obj (one slot if stackable, many slots if not).
16. `TestHandleClientCheat_Snapshot_WritesHeapFile` — modLevel=3; dispatch `::snapshot`; assert a `heap-*.pprof` file exists under `os.TempDir()` and is non-zero-size; remove after.
17. `TestHandleClientCheat_TeleOther_MovesTargetToSource` — NP=true, two players (caller modLevel=3 at coord A, other at coord B); dispatch `::teleother <other-username>`; assert other moved to coord A. Companion: NP=false → no-op (early break, no movement). Companion: unknown username → caller gets "is not logged in." message; no movement.
18. `TestHandleClientCheat_TeleOther_AdminGate` — gate-boundary at `staffModLevel=2`; dispatch `::teleother <name>` under NP=true; assert no movement (admin block doesn't fire).

**Super-mod block (1 functional):**

19. `TestHandleClientCheat_TeleTo_MovesSourceToTarget` — NP=true; dispatch `::teleto <other-username>`; assert caller moved to other's coord. Companion: NP=false → no-op. Companion: unknown username → "is not logged in." message.

### 5.4 Existing-test verification (zero touches)

All `TestHandleClientCheat_Tele_*`, `TestHandleClientCheat_GetCoord_*`, `TestHandleClientCheat_AddsSessionLog*` tests stay byte-identical — gate predicates and bodies unchanged.

## 6. Deviations

### 6.1 Retired

- **NAI-183 "TS-faithful dead code" rationale** — comments at handlers_game.go:382-386 and :392-395. The premise was based on a TS misread per §2.1. After NAI-184, reboot/slowreboot live in the admin block with the inner `&& NodeProduction` as their only NP gate, which is real (not dead). Wipe the dead-code commentary in the close commit.

### 6.2 Opened

- **DEVIATION-NAI-184-D1-SETSTAT-NO-COMBAT-REBUILD** — `Player.SetStat` omits TS L1830-1833 combatLevel recompute + buildAppearance side effect. Annotated on the `SetStat` doc-comment and on the `::setstat` / `::minme` case arms. Retire when combat-level math lands.
- **DEVIATION-NAI-184-D2-D3-CARRYFORWARD** — supersedes DEVIATION-NAI-182-D3-OTHER-CHEATS. Rewrites the comment at handlers_game.go:360-366 to enumerate the 17 remaining unported cheats (per §2.4 table). Retires the old D3 tag in the close commit. The corrected total now includes `setvar` (omitted by NAI-182).

### 6.3 Not opened (decisions documented inline)

- **Inner NP gate as case-body `break`** rather than per-arm predicate — chosen for case-body readability. Functionally identical.
- **Snapshot output format** (Go pprof vs v8 JSON) — formats differ but TS-fidelity is about the dispatch behavior. No deviation tag.
- **Player.SetStat vs Player.SetCurLevel naming** — kept distinct because they have different write semantics (full triple vs current-only). TS uses `setLevel` for both meanings via different call sites; goscape benefits from name-distinct methods.

## 7. Files touched

- `modules/world/handlers_game.go` — restructure `handleClientCheat`; rewrite D3 deviation comment.
- `modules/world/handlers_game_test.go` — modify 6 tests, delete 1, add 12.
- `modules/world/player_script.go` — add `Player.SetStat`, add `Player.InvAddFull` (or wire via existing inv API; resolved at plan).
- `pkg/objtype/playerstat.go` — add `PlayerStatMap`, `PlayerStatEnabled`.
- `pkg/objtype/playerstat_test.go` — add coverage for the new map+slice.

No changes to dskit lifecycle, config schema, or any other module.

## 8. Memory-driven controller pre-flight checklist

Per `controller_preflight`, `plan_sibling_site_guard_audit`, `plan_grep_helper_patterns`, before each implementer dispatch:

- [ ] `rg "DEVIATION-NAI-182-D3" modules/ pkg/ cmd/` returns its current hit set; record the count for the close-commit cross-foot.
- [ ] Confirm `Server.LookupPlayerByUsername` signature at `server.go:891` matches the spec snippet.
- [ ] Confirm `inventory.Inventory.Add(id, count, AddOpts{...})` is the canonical add path and that overflow→drop is the default.
- [ ] Confirm `Player.AddXP(id, xp)` fires `[changestat,X]` and `[advancestat,X]` triggers on level-up (per NAI-144 close).
- [ ] Confirm `Player.SetCurLevel` is NOT being repurposed (`SetStat` is a new method, not a rename).
- [ ] Confirm `s.cfg.NodeProduction` access pattern matches sibling sites (`handler_reportabuse.go:59`, `server_varp.go:80`).
- [ ] Confirm `parseIntOr` (`handlers_game.go` helper) is the right name and signature for parsing `::slowreboot` seconds.
- [ ] Re-grep TS L52-617 at plan-write time to verify the §2.1 outer-block enumeration hasn't drifted (the TS pin at commit `1920a1d`'s submodule head should be stable, but verify).
- [ ] Mental-execute the §3.2 `::advancestat` body against AddXP's three branches (`un-buffed`, `buffed`, `drained`) — pre-state of `levels==baseLevels==1` after reset puts us in the un-buffed branch, so level-up advances both together.

## 9. Out of scope

- The 17 remaining D3 cheats (per §2.4). Each gets its own future sub-spec when its blocking subsystem lands.
- TS `debugproc` handler at L59-148. Not a cheat per se; debugproc dispatch is a runescript scripting feature, tracked separately.
- Combat-level recompute and appearance rebuild on `Player.SetStat` (DEVIATION-NAI-184-D1).
- `giveother` cross-player inventory grant. `Player.InvAddFull` will be self-targeted only in this sub-spec; cross-player grant deferred until the wrapper proves out.
- Changes to existing `::tele`, `::getcoord`, `::say` bodies or to the L52 sessionLog tier.
