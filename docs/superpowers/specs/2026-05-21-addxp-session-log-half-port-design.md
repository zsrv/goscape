# AddXP TS:1773-1803 session-log half — port design

**Date:** 2026-05-21
**Predecessor HEAD:** `4e24174c` (AI-queue fencepost tighten close)
**TS source:** `Engine-TS/src/engine/entity/Player.ts:1773-1803` (within `addXp`, in the level-up branch at `if (this.baseLevels[stat] > before)`)
**Closes:** the informal English doc-comment deferral at `modules/world/player_script.go:810-811` ("Does NOT emit session-log / milestone events ... session-log infrastructure not yet ported"). **No formal `NAI-XXX-D-*` tag is retired** — the deferral was English-only.
**Opens:** zero new pins.

## Premise

`AddXP` (`modules/world/player_script.go:812-844`) was previously ported in two halves:
- **Half 1 (done):** XP accumulation, level recompute, changeStat + advanceStat triggers, recomputeCombatLevel.
- **Half 2 (this slice):** four ADVENTURE session-log messages emitted inside the same level-up branch.

The doc comment at line 810-811 currently claims session-log infrastructure is not yet ported. This is **stale** — NAI-74 ported the session_log subsystem and `Player.AddSessionLog(eventType, message, args...)` has been live since `modules/world/player.go:1360`. The slice closes the gap by porting the four emissions.

## What ships

### 1. `pkg/objtype/playerstat.go` — two new exported tables

```go
// PlayerStatFree mirrors TS PlayerStat.ts:55 — true for free-to-play
// skills, false for members-only. Used by AddXP's "you beat f2p!" sentinel
// (total of all-true baseLevels at 99-each = 15 × 99 = 1485).
var PlayerStatFree = [PlayerStatCount]bool{
    true, true, true, true, true, true, true, true, true, false,
    true, true, true, true, true, false, false, false, false, false, true,
}

// PlayerStatNames maps stat index → pre-lowercased skill name. Used by
// AddXP's "Levelled up <skill> from N to M" ADVENTURE session-log entry
// (TS Player.ts:1775). TS stores uppercase names in PlayerStatNameMap and
// calls .toLowerCase() at use-site; goscape pre-lowercases the storage
// since the only consumer (AddXP) needs the lowercase form. PlayerStatMap
// remains the authoritative uppercase mapping for name→index lookups.
var PlayerStatNames = [PlayerStatCount]string{
    "attack", "defence", "strength", "hitpoints", "ranged",
    "prayer", "magic", "cooking", "woodcutting", "fletching",
    "fishing", "firemaking", "crafting", "smithing", "mining",
    "herblore", "agility", "thieving", "stat18", "stat19", "runecraft",
}
```

**Index parity sanity:** `PlayerStatFree` has 15 trues × 99 = **1485** (f2p sentinel). `PlayerStatEnabled` has 19 trues × 99 = **1881** (p2p sentinel). Both match TS exactly.

### 2. `modules/world/player_script.go` — `AddXP` level-up branch extension

Insert a block between `p.changeStat(id)` and `p.advanceStat(id)`. The four emissions all use `LoggerEventTypeAdventure`. The Levelled-up message fires unconditionally on level-up; the milestone-250 message fires only when crossing a 250-boundary; the p2p/f2p messages fire on exact-equality with their respective sentinels.

Diff (relative to current `AddXP` body):

```go
    if afterBase > beforeBase {
        // Level-up: fire [changestat,<skill>] then [advancestat,<skill>]
        // triggers if registered. Matches TS Player.ts:1772, 1804-1807.
        p.changeStat(id)

+       // TS Player.ts:1773-1803 — ADVENTURE session-log entries on level-up.
+       p.AddSessionLog(LoggerEventTypeAdventure,
+           "Levelled up "+objtype.PlayerStatNames[id]+
+               " from "+strconv.Itoa(beforeBase)+
+               " to "+strconv.Itoa(afterBase))
+
+       total, freeTotal := 0, 0
+       for i := range objtype.PlayerStatCount {
+           if !objtype.PlayerStatEnabled[i] {
+               continue
+           }
+           total += int(p.baseLevels[i])
+           if objtype.PlayerStatFree[i] {
+               freeTotal += int(p.baseLevels[i])
+           }
+       }
+       const milestone = 250
+       prevMilestone := (total - (afterBase - beforeBase)) / milestone
+       currMilestone := total / milestone
+       if currMilestone > prevMilestone {
+           p.AddSessionLog(LoggerEventTypeAdventure,
+               "Reached total level "+strconv.Itoa(currMilestone*milestone))
+       }
+       if total == 1881 {
+           p.AddSessionLog(LoggerEventTypeAdventure,
+               "Reached total level 1881 - you beat p2p!")
+       }
+       if freeTotal == 1485 {
+           p.AddSessionLog(LoggerEventTypeAdventure,
+               "Reached total level 1485 - you beat f2p!")
+       }
+
        p.advanceStat(id)
        p.recomputeCombatLevel(true) // TS Player.ts:1810-1813
    }
```

New import: `"strconv"`.

### 3. `modules/world/player_script.go:810-811` — doc-comment refresh

Replace:
```go
// to mirror TS Player.ts:1810-1813. Does NOT emit session-log / milestone
// events (TS Player.ts:1773-1803; session-log infrastructure not yet ported).
```

With:
```go
// to mirror TS Player.ts:1810-1813. On level-up also emits ADVENTURE
// session-log entries (Levelled up + milestone-250 + p2p-1881 + f2p-1485)
// per TS Player.ts:1773-1803.
```

## Tests

### `pkg/objtype/playerstat_test.go` — 2 new tests

1. **`TestPlayerStatFree_MatchesTS`** — pin the exact 21-element array shape (false at indices 9, 15, 16, 17, 18, 19; true elsewhere). Plus sanity assertion: sum of `PlayerStatFree[i]?99:0` across i ∈ [0,21) == 1485.
2. **`TestPlayerStatNames_AllLowercaseAndMatchMap`** — for every (uppercaseName, idx) in `PlayerStatMap`, assert `PlayerStatNames[idx] == strings.ToLower(uppercaseName)`. Pins single source of truth between the two tables.

### `modules/world/player_script_test.go` — 7 new tests

All use `newTestPlayer(t)` (already produces `p.client.server.sessionLogs` buffer) and assert against entries in that slice. Each test resets `p.client.server.sessionLogs = nil` before `AddXP` if needed.

1. **`TestAddXPLevelUpEmitsAdventureLog`** — `stats=800, baseLevels=2, levels=2`; `AddXP(Attack, 1000)` → level 3. Assert exactly one session-log entry with `EventType==LoggerEventTypeAdventure` and `Event=="Levelled up attack from 2 to 3"` (milestone/p2p/f2p NOT triggered at this small total).
2. **`TestAddXPMultiLevelUpEmitsSingleLevelledUpMessage`** — `stats=0, baseLevels=1, levels=1`; `AddXP(Attack, 11540)` (= GetExpByLevel(10)) → level 10. Assert exactly one "Levelled up attack from 1 to 10" entry (NOT 9 entries).
3. **`TestAddXPLevelUpCrossingMilestoneEmitsMilestone`** — fixture: set the 18 other enabled baseLevels (indices 1..17, 20) summing to 247 with Attack at baseLevel=2 (enabled total = 249 before), `AddXP(Attack, GetExpByLevel(3)-GetExpByLevel(2))` → Attack=3, delta=+1, enabled total after = 250. Assert "Reached total level 250" entry present alongside "Levelled up attack from 2 to 3" (2 Adventure entries total; no p2p/f2p).
4. **`TestAddXPLevelUpNoMilestoneWithinBucket`** — defensive: total goes from 251 to 252 (within bucket [250, 500)); assert NO "Reached total level" entry, only the Levelled-up entry. Pins the `currMilestone > prevMilestone` gate.
5. **`TestAddXPLevelUpHitting1881EmitsP2PAndF2P`** — fixture: 18 enabled stats at baseLevel=99 + Attack at baseLevel=98, `AddXP(Attack, GetExpByLevel(99)-GetExpByLevel(98))` → Attack=99, enabled total = 19×99 = 1881, freeTotal = 15×99 = 1485. Assert BOTH "Reached total level 1881 - you beat p2p!" AND "Reached total level 1485 - you beat f2p!" entries present (by construction, total=1881 implies freeTotal=1485). Also assert NO "Reached total level" milestone-250 entry: prevMilestone = 1880/250 = 7, currMilestone = 1881/250 = 7, no crossing.
6. **`TestAddXPLevelUpHitting1485F2POnlyEmitsF2POnly`** — fixture: 14 f2p stats at baseLevel=99 + Attack at baseLevel=98 (f2p) + 4 members-only enabled stats (Fletching=9, Herblore=15, Agility=16, Thieving=17) at baseLevel=1, disabled[18]/[19] don't matter. `AddXP(Attack, GetExpByLevel(99)-GetExpByLevel(98))` → Attack=99, freeTotal = 15×99 = 1485, total = 1485 + 4 = 1489 ≠ 1881. Assert "Reached total level 1485 - you beat f2p!" entry present AND NO p2p entry.
7. **`TestAddXPDisabledStatNotInTotal`** — fixture: 18 enabled non-Attack stats (indices 1..17, 20) summing to 247 + Attack at baseLevel=2, baseLevels[18]=99 and baseLevels[19]=99 (disabled). `AddXP(Attack, GetExpByLevel(3)-GetExpByLevel(2))` → Attack=3, enabled-only total = 250 (delta=1, crosses milestone-1). Correct impl: prevMilestone=249/250=0, currMilestone=250/250=1, "Reached total level 250" entry fires. Incorrect impl (including disabled stats): total=447→448 (both in milestone-1 bucket), no milestone entry. Test asserts "Reached total level 250" entry IS present — would fail if the loop forgot to gate on `PlayerStatEnabled[i]`.

### Existing tests preserved

All 9 existing `TestAddXP*` tests in `player_script_test.go` continue to pass unmodified. The no-level-up tests implicitly cover "no Adventure entries when no level-up" because the new emissions are gated on `afterBase > beforeBase`.

## Non-goals / explicitly out-of-scope

- No formal `NAI-XXX-D-*` tag handling — the deferral was an English doc comment, not a pinned tag. No opening or retiring of any pin.
- No changes to `recomputeCombatLevel`, `changeStat`, `advanceStat`, or any other AddXP machinery — purely additive emissions in the existing level-up branch.
- No changes to LoggerEventType enum, SessionLog struct, or processSessionLogs flush logic — full infra is reused as-is.
- No content-script audit. The four messages exist purely as ADVENTURE-channel records; no script reads them.
- No friends-list / chat propagation. ADVENTURE entries flush through `processSessionLogs` to `loggerBridge.SubmitSessionLogs` and stop there; that's the TS behavior too.
- No PlayerStatNameMap port (Map<int,string>). `PlayerStatNames` array covers the only consumer.

## Implementation order

Mid-ceremony per `[[ai-queue-fencepost-tighten-close]]` precedent: spec (this doc) → in-session execution + single behavior commit → close memory. No plan, no subagent dispatch.

Single commit shape (suggested):
- title: `feat(world): port AddXP session-log half — adventure entries on level-up (TS:1773-1803)`
- body: brief mention of new objtype tables (PlayerStatFree, PlayerStatNames), AddXP block insertion, doc-comment refresh, 9 new tests; reference closing the player_script.go:810-811 informal deferral.

## Pre-commit hygiene

- `git status` immediately before commit; do NOT stage `config.yaml`, `.claude/`, `.bash_profile`, etc. (standing untracked noise).
- `git show --stat HEAD` after commit for verification.
- All commits: `git commit --no-gpg-sign` per global CLAUDE.md.
- Gates: `-race ./...` clean on all touched packages; `TestPackAll_TwelveStageSmoke` PASS.
- Audit-grep post-commit: search for `"session-log infrastructure not yet ported"` and `"TS Player.ts:1773-1803"` → only acceptable remaining hits are this spec doc + close-memory entry.

## Deliberate deviations from TS

1. **Pre-lowercased `PlayerStatNames` storage** (vs TS's uppercase `PlayerStatNameMap` + runtime `.toLowerCase()`). Saves a `strings.ToLower` call per emission. PlayerStatMap (uppercase) remains authoritative for name→index lookups; the lowercase array's only consumer is AddXP. Documented inline.
2. **`for i := range objtype.PlayerStatCount`** modern Go iteration (vs TS's `for (let stat = 0; stat < this.baseLevels.length; stat++)`). House convention per `[[nai184-combat-level-recompute-close]]`. Uses outer `id` as the levelling stat to avoid TS's variable-shadowing trick.
3. **No `?` optional-chain semantics** — `PlayerStatNames[id]` cannot return nil; `id` was already bounds-checked by `statBounds(id)` at the top of AddXP. TS's `PlayerStatNameMap.get(stat)?.toLowerCase()` would return `undefined` for out-of-range (cascading to the string "undefined"); we never reach this case.

None of these change observable behavior for in-bounds inputs. All three follow established goscape porting patterns. No `NAI-XXX-D-*` pin opened for any of them (mirrors `[[combat-sub-spec-framing-doc-cleanup-close]]` — deliberate-deviation-for-correctness/idiom gets inline doc, not formal tag).
