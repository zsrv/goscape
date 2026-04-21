# Sub-spec RuneScript S6i: AdvanceStat Trigger Fire — Design

**Status:** Draft → ready for plan
**Scope:** Port TS `Player`'s AdvanceStat dispatch (Player.ts:1804-1807) — sibling to S6h's ChangeStat fire. Add `Provider.GetByTriggerSpecific(trigger, typeID, categoryID int) *ScriptFile` matching TS `getByTriggerSpecific` (single-tier lookup, no fallback). Add unexported `Player.advanceStat(stat int)` helper. Wire into `Player.AddXP`'s level-up block as a sibling call after `changeStat`. Closes the S6h final-review follow-up #2.
**Out of scope:** Combat-level recompute (TS Player.ts:1810-1813; deferred to combat sub-spec). Other trigger fires that share the level-up block (none others in TS today). Renaming the existing `Player.advanceStat` method (no naming collision — `AddXP` is goscape's name for what TS calls `advanceStat`; the new helper is `Player.advanceStat` lowercase, parallel to S6h's `Player.changeStat`).

---

## Rationale

S6h's final review explicitly flagged AdvanceStat as the natural companion to ChangeStat (recommendation #2). `TriggerAdvanceStat = 160` was already defined in `pkg/script/trigger.go` from the original trigger enum import but has zero call sites. Cache scripts that say "Congratulations, you just advanced an Attack level!" subscribe to `[advancestat,attack]` — without the fire, those messages never appear.

The work also surfaces a small but useful new Provider method. TS distinguishes `getByTrigger` (3-tier fallback) from `getByTriggerSpecific` (single tier, no fallback). For ChangeStat (S6h), TS uses the fallback variant — a global `[changestat,_]` is meaningful for "any stat changed" handlers (combat-level recompute, drained-stat regen). For AdvanceStat, TS uses the specific variant — a global `[advancestat,_]` would fire on every stat advance regardless of which skill, which is almost certainly a bug, not a feature. The split is TS's design call; we mirror it faithfully.

## Architecture

```
pkg/script/
├── provider.go               (modify) — add GetByTriggerSpecific method
└── provider_test.go          (modify) — 6 new tier-isolation tests

modules/world/
├── player_script.go          (modify) — new Player.advanceStat(stat int) helper;
│                                          AddXP calls advanceStat(id) after
│                                          changeStat(id) inside the level-up block;
│                                          AddXP doc-comment refreshed to mention
│                                          both triggers fire
└── player_script_test.go     (modify) — 4 new tests (3 advanceStat-only +
                                          1 integration verifying both fire in TS order)
```

Total **~330 LOC** (production + tests).

## Components

### 1. `Provider.GetByTriggerSpecific` — `pkg/script/provider.go`

Append immediately after the existing `GetByTrigger` method (around line 127):

```go
// GetByTriggerSpecific returns the script for a single tier without the
// 3-level fallback that GetByTrigger does. Caller picks which tier by
// passing -1 for the others:
//   - typeID != -1: returns only the type-specific lookup (no fallback)
//   - else categoryID != -1: returns only the category lookup
//   - else: returns the global lookup
//
// Returns nil if the chosen tier has no registered script. Matches TS
// ScriptProvider.getByTriggerSpecific (ScriptProvider.ts:147-154).
//
// Used by Player.advanceStat to enforce the contract that
// [advancestat,<skill>] scripts must skill-key — a global [advancestat,_]
// script would fire on every stat advance regardless of which skill,
// which is almost certainly a bug not a feature. ChangeStat keeps the
// 3-tier GetByTrigger fallback because "any stat changed" handlers
// (combat-level recompute, regen) are meaningful.
func (p *Provider) GetByTriggerSpecific(trigger ServerTriggerType, typeID, categoryID int) *ScriptFile {
	if typeID != -1 {
		return p.byKey[uint32(trigger)|(0x2<<8)|(uint32(typeID)<<10)]
	}
	if categoryID != -1 {
		return p.byKey[uint32(trigger)|(0x1<<8)|(uint32(categoryID)<<10)]
	}
	return p.byKey[uint32(trigger)]
}
```

Map indexing returns nil for missing keys — same observable behavior as TS's optional-chained `Map.get(...)` returning undefined. No comma-ok check needed.

### 2. `Player.advanceStat` — `modules/world/player_script.go`

Add immediately after the existing `Player.changeStat` helper from S6h:

```go
// advanceStat fires the [advancestat,<skill>] trigger for the given stat
// slot when a cache script is registered for that exact stat. Unlike
// changeStat (which uses the 3-level fallback via GetByTrigger), this
// uses GetByTriggerSpecific — type-specific only, no category or global
// fallback. A global [advancestat,_] script would be wrong here: cache
// scripts that say "Congratulations, you just advanced an Attack level!"
// must be skill-keyed.
//
// Enqueued as QueueNormal so it runs asynchronously through
// processPlayerQueue. Matches TS Player.ts:1804-1807 exactly.
//
// Silent no-op if no specific script is registered (GetByTriggerSpecific
// returns nil → EnqueueScriptFile's nil-check short-circuits). Called
// from AddXP's level-up branch after changeStat.
func (p *Player) advanceStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTriggerSpecific(script.TriggerAdvanceStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, 0, script.QueueNormal)
}
```

### 3. `Player.AddXP` — wire the call

S6h's level-up block currently ends:

```go
	if afterBase > beforeBase {
		// Level-up: fire the [changestat,<skill>] trigger if registered.
		// Matches TS Player.ts:1772.
		p.changeStat(id)
	}
}
```

Replace with:

```go
	if afterBase > beforeBase {
		// Level-up: fire [changestat,<skill>] then [advancestat,<skill>]
		// triggers if registered. Matches TS Player.ts:1772, 1804-1807.
		p.changeStat(id)
		p.advanceStat(id)
	}
}
```

**Order matters semantically:** TS fires `changeStat` first (line 1772) then `advanceStat` (line 1804). Our Go port preserves the order. If both scripts modify shared state (e.g., a varp tracking total levels), the changeStat handler runs first.

Both go into the same async queue with delay=0, so processPlayerQueue fires them in append order on the next tick. Append order = TS order. Test coverage pins this explicitly via the integration test.

### 4. Doc-comment refresh on `Player.AddXP`

S6h's last commit updated the doc to reference only ChangeStat. Refresh to mention both:

```go
// On level-up (baseLevels increases), fires the [changestat,<skill>] trigger
// via changeStat (TS Player.ts:1772) then the [advancestat,<skill>] trigger
// via advanceStat (TS Player.ts:1804-1807). Does NOT recompute combat
// level (future combat sub-spec).
```

## Behavioral contrast: ChangeStat vs AdvanceStat

| Aspect | ChangeStat (S6h) | AdvanceStat (S6i) |
|---|---|---|
| Trigger ID | 165 (`TriggerChangeStat`) | 160 (`TriggerAdvanceStat`) |
| Provider method | `GetByTrigger` (3-tier fallback) | `GetByTriggerSpecific` (single tier) |
| Global `[X,_]` fires? | Yes — falls through | **No — type-specific only** |
| Use case | Any-stat-changed hooks (combat-level recompute, regen) | Per-skill level-up message ("you just advanced an Attack level") |
| Fires on | Every level-up (`afterBase > beforeBase`) | Same condition |
| Order vs the other | First (TS 1772) | Second (TS 1804) |
| Queue type | `QueueNormal` (TS ENGINE-equivalent) | `QueueNormal` (same) |
| Single fire per AddXP | Yes — once per call regardless of level delta | Same |

Both helpers share:
- 3-deep nil guard on `p.client.server.scriptProvider`
- Same async fire-via-queue pattern
- `EnqueueScriptFile` nil-safety on missing scripts
- Single fire per AddXP regardless of multi-level-up

## Edge cases

| # | Case | Handling |
|---|---|---|
| 1 | Level-up with both `[changestat,X]` and `[advancestat,X]` registered | Both fire, in TS order (changeStat first, advanceStat second). Verified by integration test. |
| 2 | Level-up with only `[changestat,X]` | changeStat fires; advanceStat no-ops on nil from `GetByTriggerSpecific`. |
| 3 | Level-up with only `[advancestat,X]` | changeStat no-ops on nil from `GetByTrigger` (no script at any tier); advanceStat fires. |
| 4 | Level-up with only global `[advancestat,_]` registered | **advanceStat does NOT fire.** This is the TS-faithful design point — the specific lookup misses, no fallback. |
| 5 | Level-up with only category `[advancestat,_catN]` registered | Same as #4 — specific tier misses. |
| 6 | No level-up (XP gain below threshold) | Outer `afterBase > beforeBase` guard fails; neither helper called. |
| 7 | Player unwired (nil scriptProvider) | advanceStat early-returns on nil guard. |
| 8 | Multi-level-up in one AddXP call (XP jumps 5+ levels) | Single advanceStat call. Matches TS — one fire per AddXP regardless of level delta. |
| 9 | `GetByTriggerSpecific(trigger, -1, -1)` | Returns the global script. Matches TS fallthrough — useful for ChangeStat-style "any" handlers, but advanceStat passes typeID = stat (always ≥ 0), so this branch is unreachable from advanceStat's call site. |
| 10 | `GetByTriggerSpecific` map miss | Map indexing returns nil for unknown key; same observable behavior as TS's `Map.get(...) ?? undefined`. |

## Testing strategy

### Provider tier-isolation tests — `pkg/script/provider_test.go`

Six new tests pinning the `GetByTriggerSpecific` semantic:

| Test | Asserts |
|---|---|
| `TestGetByTriggerSpecificTypeOnly` | typeID=N + matching script registered → returns it |
| `TestGetByTriggerSpecificCategoryOnly` | typeID=-1, categoryID=N + matching script → returns it |
| `TestGetByTriggerSpecificGlobalOnly` | typeID=-1, categoryID=-1 + global script → returns it |
| `TestGetByTriggerSpecificNoFallback` | only global registered, query with typeID=N → returns nil (no fallback) |
| `TestGetByTriggerSpecificTypeShortCircuitsCategory` | typeID=N AND categoryID=M, only category script registered → returns nil (type tier picked, no fallback) |
| `TestGetByTriggerSpecificMissingReturnsNil` | empty provider → returns nil |

The "no fallback" test is the load-bearing one — it differentiates from `GetByTrigger` and pins the design intent.

### Player.advanceStat fire tests — `modules/world/player_script_test.go`

Four new tests, mirroring the S6h ChangeStat test pattern:

| Test | Asserts |
|---|---|
| `TestAddXPFiresAdvanceStatOnLevelUp` | Register `[advancestat,attack=0]` at type-specific key; AddXP crosses level; queue grows by 1 with `Script == sf` |
| `TestAddXPDoesNotFireAdvanceStatWithoutLevelUp` | Register the script; AddXP below level-2 threshold; queue length unchanged |
| `TestAddXPAdvanceStatNoFallbackToGlobal` | Register only `[advancestat,_]` global; AddXP with level-up; queue length unchanged AND baseLevels still advanced. Pins TS-faithful no-fallback. |
| `TestAddXPFiresBothChangeAndAdvanceStatOnLevelUp` | Register both type-specific changestat and advancestat scripts; AddXP with level-up; queue grows by 2; entries are in TS order (changeStat first) |

The integration test (#4) is the meaningful new coverage — it verifies S6h and S6i coexist correctly and pins the fire order.

### Cross-spec compatibility

- `TestStatAdvanceViaScript` (script_test.go:511) — adds 50 XP starting from 100; total 150 < 830, no level-up, neither helper fires. Assertion `stats[3] == 150` still holds.
- S6h ChangeStat tests — independent of S6i changes; still pass.
- S6g AddXP scenario tests — level-up math unchanged; pre-S6h tests didn't assert queue contents, post-S6h ones do but aren't affected by the new advanceStat call (they don't register an advancestat script, so the additional helper just no-ops).

**Total test LOC:** ~60 (Provider) + ~110 (Player) = ~170 LOC.

## LOC estimate

| File | LOC |
|---|---|
| `pkg/script/provider.go` (diff) | +20 |
| `pkg/script/provider_test.go` (diff) | +60 |
| `modules/world/player_script.go` (diff) | +25 (helper + AddXP call line + doc refresh) |
| `modules/world/player_script_test.go` (diff) | +110 |
| **Total** | **~215 production + tests** |

(Spec's "330 LOC" estimate was slightly inflated — actual is ~215 because the AddXP doc refresh is shorter than I'd budgeted.)

## Key design calls

- **Match TS exactly: type-specific lookup with no fallback.** AdvanceStat using `GetByTrigger`'s fallback would let a global `[advancestat,_]` fire on every level-up, breaking cache scripts that assume only the skill-keyed handler runs. The "no fallback" test (#3 in Provider, #3 in Player) pins this design intent.
- **`GetByTriggerSpecific` matches TS signature exactly** — three-arg `(trigger, typeID, categoryID)` with first-non-(-1) wins. Symmetric with `GetByTrigger`. Future callers can target any tier directly without coupling to bit-encoding.
- **`Player.advanceStat` unexported.** Package-internal engine dispatch helper, parallel to S6h's `changeStat`. Future boost/drain or AI sub-specs can re-use; the unexported name signals "engine-internal" and lets us rename freely.
- **Order of triggers preserved (changeStat before advanceStat).** TS lines 1772 then 1804. Both go into the same async queue with delay=0; append order = fire order. Integration test pins this.
- **Single helper for the dispatch, not inline in AddXP.** TS does it inline; we extract to a helper for symmetry with `changeStat` and to keep AddXP readable. Net: AddXP gains one line, advanceStat is ~6 lines elsewhere.
- **Doc-comment refresh on AddXP.** S6h's last commit cited only ChangeStat; S6i bumps it to mention both. Same kind of mid-implementation refresh that S6h shipped as a separate `docs(world):` commit.

## Gotchas

- **Map indexing vs comma-ok.** `p.byKey[k]` returns nil for missing keys; we don't need `if sf, ok := p.byKey[k]; ok` because nil is a meaningful "no script" return that `EnqueueScriptFile` already screens.
- **`uint32(typeID)<<10` for negative typeID.** If a caller passed typeID = -2 (against contract), `uint32(-2)` becomes a huge value that won't match any registration. The first-tier check `typeID != -1` short-circuits the canonical -1 sentinel; any other negative value is a contract violation that produces a benign nil return.
- **Tests share `before := len(p.queue)` pattern.** S6h tests already used this; S6i tests follow the same idiom for consistency. Defensive against any fixture that pre-seeds queue entries.
- **The `TestAddXPFiresBothChangeAndAdvanceStatOnLevelUp` integration test is the load-bearing one.** It verifies fire order, queue-length-grows-by-2, and that S6h and S6i don't accidentally cancel each other out. If a future refactor accidentally swaps the call order or de-duplicates the dispatches, this test fails immediately.
