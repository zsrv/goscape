# NAI-22 — follow-up bundle (AI_SPAWN producer activation + checkInv huntPlayers filter + appearanceInv ctor-binding cleanup + byte-search test polish)

- **Sub-spec**: NAI-22
- **Date**: 2026-04-25
- **Scope label**: B (logical-grouping follow-up bundle — `modules/world` (production + tests); ~30 LOC production + ~200 LOC tests across 3 bundles; closes 2 tracked deviations (NAI-19-D2 AI_SPAWN re-trigger, NAI-21-D1 appearanceInv ctor-binding); closes 1 open huntPlayers deferral (checkInv); introduces 0 new deviations; net deviation count 16 → 14)
- **Predecessors**: NAI-21 (follow-up bundle) — last on `main` as `01de242`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

Three actionable carry-forwards land in NAI-22:

1. **NAI-19-D2 (AI_SPAWN re-trigger).** Tracked at `npc_registry.go:77` as "AI_SPAWN trigger queue omitted — `script.TriggerAiSpawn` (`pkg/script/trigger.go:171`) declared but no spawn-flow consumer wiring." Pre-flight at HEAD `01de242` confirms the producer is the **only** missing piece: the dispatch infrastructure is fully wired (event queue, type-agnostic processor, tick hook, script lookup, NpcEventSpawn reserved enum). Closing NAI-19-D2 is a textbook `consume_reserved_constant` memory case.

2. **checkInv huntPlayers filter.** Currently deferred at `npc_hunt.go:102` ("checkInv (TS:959-969) — inventory queries"). Pre-flight confirms all required infrastructure is in place: `HuntType.CheckInv`/`CheckObj`/`CheckObjParam`/`CheckInvCondition`/`CheckInvVal` are decoded with TS defaults; `HuntType.CheckHuntCondition` is already used by the sister CheckVars filter at `npc_hunt.go:196`; `Inventory.GetItemCount(id)` exists at `pkg/inventory/inventory.go`; the `*Server` holds `objTypes` and `paramTypes` registries.

3. **NAI-21-D1 (appearanceInv ctor-binding).** Tracked at `appearance.go:25`, `pkg/script/active.go:329`, `pkg/script/handlers_player.go:133` as "TS init binds `appearanceInv` to Worn at ctor; goscape uses `-1` sentinel + reader fallback." Closing this in production is a small post-construction setter call wired into the player-creation flow. The `-1` ctor sentinel is retained as test-only safety — the deviation's *production* concern dissolves.

Pre-flight at HEAD `01de242` against the candidate framings surfaced two scope-shift findings:

- **AI_SPAWN dispatch is a producer wiring task, not a design exercise.** The NAI-19 spec deferred AI_SPAWN with the rationale "event queue vs immediate fire ordering needs design." That design is already settled: `Server.npcEventQueue` exists (`server.go:90`), `processNpcEventQueue` dispatches type-agnostically (`npc_event_queue.go:36-48`), the tick wires it at `tick.go:40` matching TS `World.ts:356`, and the AI_DESPAWN producer at `npc_ai.go:47-58` is the literal template. The "semantically invasive at boot" worry was about *missing dispatch wiring*; with the wiring already in place, TS fidelity (fire on both `firstSpawn=true` and `firstSpawn=false`) is the safe and correct port.

- **(d)'s ctor-signature thread is too invasive.** `newPlayer(c *client)` has 18 callsites (1 production at `client.go:113`, 17 in test files). Threading `invs *objtype.InvTypeConfigs` through the ctor would require updating all 18 callers — well beyond the ~20-30 LOC budget the candidate framing assumed. A post-construction setter `p.setAppearanceInv(invs.Worn)` called from the production wiring touches only the 1 production callsite while leaving test fixtures undisturbed.

The 4 items cluster naturally into three bundles by content type (production behavior change / production behavior change / internal-mechanism + test polish), each landing as one commit with bundle-scoped review. Bundle 1 (Task 1) and Bundle 2 (Task 2) each warrant full two-stage review (independent production-behavior changes); Bundle 3 (Tasks 3 + 4) hits the `compressed_cadence` ~15-LOC threshold for combined spec-implied review.

## Tech stack

- Go 1.26+
- Existing packages touched:
  - `modules/world/npc_registry.go` (Bundle 1: ~10 LOC added — AI_SPAWN producer block)
  - `modules/world/npc_event_queue.go` (Bundle 1: 1 doc-comment retire — NpcEventSpawn no longer "reserved without producer")
  - `modules/world/npc_hunt.go` (Bundle 2: ~15 LOC added — CheckInv filter; ~30 LOC added — `invTotalParam` helper)
  - `modules/world/client.go` (Bundle 3: 1 LOC added — `p.setAppearanceInv` call)
  - `modules/world/player.go` (Bundle 3: setter method ~5 LOC, ctor doc-comment update)
  - `modules/world/appearance.go` (Bundle 3: sentinel-fallback comment retag from "deviation" to "test-only safety")
- Test files touched:
  - `modules/world/npc_event_queue_test.go` (Bundle 1: extend SPAWN dispatch coverage)
  - `modules/world/npc_registry_test.go` (Bundle 1: new — AI_SPAWN producer tests; or fold into existing addNpc test file if present)
  - `modules/world/npc_hunt_test.go` (Bundle 2: extend with CheckInv tests)
  - `modules/world/appearance_test.go` (Bundle 3: byte-search refactor + setAppearanceInv binding test)
- Doc/script-side files touched:
  - `pkg/script/active.go` (Bundle 3: NAI-21-D1 narrative cleanup; doc-comment update)
  - `pkg/script/handlers_player.go` (Bundle 3: NAI-21-D1 narrative cleanup; doc-comment update)
- No new files in production packages; one optional new test file (`npc_registry_test.go`) if SPAWN producer tests don't fit cleanly elsewhere.

## Scope

### Bundle 1 — AI_SPAWN producer activation (NAI-19-D2 closure)

**Goal**: Activate the reserved `NpcEventSpawn` constant by adding the SPAWN-event producer in `s.addNpc`, mirroring TS `World.ts:1284-1289` and the existing AI_DESPAWN producer pattern in `npc_ai.go:47-58`. Closes deviation NAI-19-D2.

**Source**: NAI-19 deferral at `npc_registry.go:77-82`.

#### Pre-flight verification (per `consume_reserved_constant` memory)

The new consumer owns the full dispatch path. Verified at HEAD `01de242`:

- **Reserved constant**: `NpcEventSpawn = 0` declared at `npc_event_queue.go:13`. Doc-comment at `:7-9` explicitly says "reserved for TS fidelity but has no producer in NAI-5."
- **Event-request struct**: `NpcEventRequest{Type, Script, Npc}` declared at `npc_event_queue.go:22-26`.
- **Server queue field**: `s.npcEventQueue []NpcEventRequest` declared at `server.go:90`.
- **Type-agnostic processor**: `processNpcEventQueue` at `npc_event_queue.go:36-48` dispatches `req.Script` regardless of `req.Type`, gated only on `req.Npc.delayed`. The processor handles SPAWN events identically to DESPAWN events with no code change.
- **Tick wiring**: `s.processNpcEventQueue()` called at `tick.go:40` ("NAI-5: matches TS `World.ts:356`").
- **Script lookup primitive**: `(*script.Provider).GetByTrigger(trigger, typeID, categoryID)` at `pkg/script/provider.go:114`.
- **Trigger constant**: `script.TriggerAiSpawn = 166` at `pkg/script/trigger.go:171`.
- **DESPAWN producer template**: `npc_ai.go:47-58` — the literal pattern to mirror.

#### Code change shape

**Production change** (`modules/world/npc_registry.go`, replacing the deviation comment at lines 77-82):

```go
// AI_SPAWN trigger queue (matches TS World.ts:1284-1289).
// Fires unconditionally — for both firstSpawn=true (server boot) and
// firstSpawn=false (revertType respawn). NPCs without a registered
// AI_SPAWN script never enter the queue (the script != nil guard).
// processNpcEventQueue dispatches at tick.go:40.
if s.scriptProvider != nil && n.typ != nil {
    sf := s.scriptProvider.GetByTrigger(
        script.TriggerAiSpawn, n.typeId, n.typ.Category)
    if sf != nil {
        s.npcEventQueue = append(s.npcEventQueue,
            NpcEventRequest{
                Type:   NpcEventSpawn,
                Script: sf,
                Npc:    n,
            })
    }
}
```

Position inside `addNpc`: AFTER the entity reset / animation play / collision toggle (mirrors TS `World.ts:1281-1289` ordering), BEFORE the `setLifeCycle(duration)` call.

**Production delta**: ~10 LOC added in `addNpc`; deviation comment block at `npc_registry.go:77-82` is replaced.

**Doc-comment retirement** (`modules/world/npc_event_queue.go:7-9`): retire the "reserved for TS fidelity but has no producer in NAI-5" wording — NpcEventSpawn now has a producer. New doc-comment narrates the producer location.

**Internal-comment retirement at `npc.go:259, 283`**: NAI-19-D2 callouts updated/removed (NAI-19-D1 callouts stay — Zone abstraction is still unported).

#### Test plan (~120 LOC total, 5 tests)

**File**: extend `modules/world/npc_event_queue_test.go` (the existing AI_DESPAWN test home) with SPAWN-side coverage. If file size warrants a split, plan-time may extract into a new `npc_spawn_test.go`.

1. **`TestAddNpcQueuesSpawnEventOnFirstSpawn`** — pin firstSpawn=true producer:
   - Build server + register a SPAWN ScriptFile via `s.scriptProvider.Register(...)`.
   - Call `s.addNpc(n, -1, true)`.
   - Assert: `len(s.npcEventQueue) == 1`, `s.npcEventQueue[0].Type == NpcEventSpawn`, `.Script == registered`, `.Npc == n`.

2. **`TestAddNpcQueuesSpawnEventOnRespawn`** — pin firstSpawn=false producer (revertType heavy path):
   - Same as #1 but call `s.addNpc(n, -1, false)`. Proves the producer is NOT firstSpawn-gated.

3. **`TestAddNpcNoSpawnScriptNoQueue`** — pin the `script == nil` short-circuit:
   - Build server + scriptProvider with NO SPAWN script registered.
   - Call `s.addNpc(n, -1, true)`.
   - Assert: `len(s.npcEventQueue) == 0`.

4. **`TestAddNpcNilScriptProviderNoQueue`** — pin the `s.scriptProvider == nil` defensive guard:
   - Build server with `s.scriptProvider = nil` explicitly.
   - Call `s.addNpc(n, -1, true)`.
   - Assert: no panic, `len(s.npcEventQueue) == 0`.

5. **`TestProcessNpcEventQueueDispatchesSpawn`** — pin end-to-end dispatch through the type-agnostic processor:
   - Build server, register SPAWN ScriptFile with an observable side-effect opcode.
   - Call `s.addNpc(n, -1, true)` then `s.processNpcEventQueue()`.
   - Assert: `len(s.npcEventQueue) == 0` (drained), side-effect fired (proves the script actually ran, not silent dispatch failure — same dual-assertion pattern as NAI-21 Bundle 3).

**TS-fidelity gate**: ✓ Producer matches TS `World.ts:1284-1289` byte-for-byte (typed event, lookup-then-queue, no firstSpawn gate). Dispatch already matches TS `World.ts:664-673` via existing `processNpcEventQueue`.

**Deviation impact**: closes NAI-19-D2 (-1). No new deviations introduced.

#### Open questions deferred to plan-time

- **Test fixture seeding**: whether `s.scriptProvider` is seeded by the existing `newServerForScriptTest(t)` helper (per NAI-21 Bundle 3 pre-flight at NAI-21 spec line 205, `scriptProvider` is **nil** by default). Tests in this bundle must seed `s.scriptProvider = script.NewProvider()` themselves before calling `Register()`.
- **Side-effect opcode for Test 5**: candidates are `OpSetVarN`, `OpSetTimer`, or any opcode that writes a test-readable NPC field. Plan-time selects based on what's already wired in `defaultTestProvider()`'s opcode table.

#### Bundle 1 commit shape

- **One commit**: `feat(world): NAI-22 Bundle 1 — AI_SPAWN producer activation (NAI-19-D2 closed)`
- **Two-stage review**: Stage 1 code review on production change + tests; Stage 2 TS-fidelity whole-impl validates `addNpc` shape against TS `World.ts:1258-1294`, deviation count math (16 → 15), and `consume_reserved_constant` memory's "new consumer owns the full dispatch path" checklist (producer, processor, tick wiring, end-to-end test all present).

### Bundle 2 — checkInv huntPlayers filter

**Goal**: Activate the deferred `checkInv` filter in `huntPlayers`, mirroring TS `Npc.ts:959-969`. Closes the third of three open huntPlayers filter deferrals from `npc_hunt.go:99-102` (checkNotBusy and checkNotTooStrong remain deferred — different missing infrastructure).

**Source**: NAI-8 deferral at `npc_hunt.go:102`, plus `nai_followups.md` huntPlayers section.

#### Pre-flight verification

- **Config fields decoded**: `HuntType.CheckInv` (default -1), `CheckObj` (default -1), `CheckObjParam` (default -1), `CheckInvCondition` (default ""), `CheckInvVal` (default -1). All at `pkg/objtype/hunttype.go:65-95`.
- **Condition evaluator**: `(*HuntType).CheckHuntCondition(value, condition, checkValue) bool` at `hunttype.go:210`. Already used by sister CheckVars filter at `npc_hunt.go:196`.
- **Inventory count primitive**: `(*Inventory).GetItemCount(id int) int` exists at `pkg/inventory/inventory.go`.
- **Param-aware count primitive**: `handleInvTotalParam` at `pkg/script/handlers_inv.go:224-257` implements the param-resolution logic. Goscape's huntPlayers context cannot reuse this directly because it requires `*ScriptState` for ParamType lookup; we need a parallel non-ScriptState helper.
- **Server registries**: `s.objTypes *ObjTypeConfigs`, `s.paramTypes *ParamTypeConfigs` available on Server.

#### Code change shape

**Helper added** (`modules/world/npc_hunt.go`, new function — keep adjacent to `huntPlayers`):

```go
// invTotalParam mirrors handleInvTotalParam (pkg/script/handlers_inv.go:224)
// for non-ScriptState callers. Sums per-slot ObjType.Params[param] across
// every non-empty slot of inv, falling back to ParamType.DefaultInt for
// missing params. Returns 0 if any required config is nil (defensive —
// huntPlayers cannot abort iteration on a single param-resolution failure).
//
// TS source: Player._invTotalParam at Player.ts:1668-1697 (stack=false branch).
func invTotalParam(inv *inventory.Inventory, param int,
    objs *objtype.ObjTypeConfigs, params *objtype.ParamTypeConfigs) int {
    if inv == nil || objs == nil || params == nil {
        return 0
    }
    pt := params.Get(param)
    if pt == nil {
        return 0
    }
    total := 0
    for _, it := range inv.Items {
        if it == nil || it.Id < 0 {
            continue
        }
        ot := objs.Get(it.Id)
        if ot == nil {
            continue
        }
        if v, ok := ot.Params[uint32(param)]; ok {
            if iv, ok := v.(uint32); ok {
                total += int(iv)
                continue
            }
        }
        total += int(pt.DefaultInt)
    }
    return total
}
```

Note: the exact `ParamTypeConfigs.Get(param)` and `ObjTypeConfigs.Get(id)` accessor names need plan-time verification — `handleInvTotalParam` uses `s.Configs.ParamType(param)` and `s.Configs.ObjType(it.Id)`, but those are `serverConfigsView` methods. The new helper takes the registries directly; if no public `Get` accessor exists, plan-time either adds one or routes through a small `serverConfigsView`-shaped wrapper.

Additional plan-time check: the value-type assertion `v.(uint32)` for `ot.Params[uint32(param)]` mirrors `handleInvTotalParam` at `pkg/script/handlers_inv.go:248`. If goscape's ObjType.Params storage type changes (currently `map[uint32]any` with `uint32` int values), this assertion needs to track. Plan-time greps `ot\.Params\[` consumers to confirm the value-type is stable.

**Filter block added** (`modules/world/npc_hunt.go`, inserted after the existing CheckVars block at line 200, before the `hunted = append(hunted, p)` line):

```go
// checkInv (TS:959-969): if CheckInv is set, compute quantity per
// CheckObj or CheckObjParam branch, then evaluate CheckHuntCondition.
// Defensive: missing inv → quantity=0 (TS throws here, but goscape
// huntPlayers must continue iteration on one bad player; live players
// have all standard invs in practice).
if hunt.CheckInv != -1 {
    quantity := 0
    if pInv := p.invs[hunt.CheckInv]; pInv != nil {
        if hunt.CheckObj != -1 {
            quantity = pInv.GetItemCount(hunt.CheckObj)
        } else if hunt.CheckObjParam != -1 {
            quantity = invTotalParam(pInv, hunt.CheckObjParam,
                s.objTypes, s.paramTypes)
        }
    }
    if !hunt.CheckHuntCondition(quantity,
        hunt.CheckInvCondition, hunt.CheckInvVal) {
        continue
    }
}
```

**Doc-comment retirement** (`modules/world/npc_hunt.go:99-102`): `checkInv` line removed from the "DEFERRED" list, added to the "Filter coverage" list with NAI-22 + TS:959-969 citation.

**Production delta**: ~50 LOC added in `npc_hunt.go` (filter block ~17 LOC + helper ~30 LOC + doc-comment edits).

#### Test plan (~250 LOC total, 6 tests)

**File**: extend `modules/world/npc_hunt_test.go`.

1. **`TestHuntPlayersCheckInvDisabled`** — short-circuit pin:
   - Hunt with `CheckInv = -1` (default).
   - Assert: filter no-ops, players hunted as before.

2. **`TestHuntPlayersCheckInvObjPasses`** — CheckObj branch, condition passes:
   - Player has 5 of obj X in inv Y.
   - Hunt with `CheckInv=Y, CheckObj=X, Condition=">=", Val=3`.
   - Assert: player included.

3. **`TestHuntPlayersCheckInvObjFails`** — CheckObj branch, condition fails:
   - Player has 1 of obj X.
   - Hunt with `CheckInv=Y, CheckObj=X, Condition=">=", Val=3`.
   - Assert: player excluded.

4. **`TestHuntPlayersCheckInvObjParamPasses`** — CheckObjParam branch, passing:
   - Player has items in inv Y whose ObjType.Params[P]=10 each (3 such items).
   - Hunt with `CheckInv=Y, CheckObjParam=P, Condition=">=", Val=20`.
   - Assert: player included (sum=30).

5. **`TestHuntPlayersCheckInvObjParamFails`** — CheckObjParam branch, failing:
   - Player has items summing param=15 < required 20.
   - Hunt with same shape, Val=20.
   - Assert: player excluded.

6. **`TestHuntPlayersCheckInvMissingInvDefensive`** — missing-inv pin (`p.invs[hunt.CheckInv] == nil`):
   - Player has no inv at id Y.
   - Hunt with `CheckInv=Y, CheckObj=X, Condition="=", Val=0`.
   - Assert: quantity defaults to 0, condition `0 == 0` passes, player included. Pins the defensive treat-as-0 behavior. **Documents goscape-vs-TS divergence (TS throws here)** with no deviation tag because TS path is dead in practice and goscape's choice is iteration-survival, not behavioral divergence.

**TS-fidelity gate**: ✓ Filter shape matches TS `Npc.ts:959-969` exactly (CheckObj precedence over CheckObjParam, no-quantity-default falls through to CheckHuntCondition with 0). One documented deviation: missing-inv handling (defensive treat-as-0 vs. TS throw); rationale captured in code comment, not deviation tag.

**Deviation impact**: closes 1 huntPlayers filter deferral (no Dn tag was attached). No new deviations introduced.

#### Open questions deferred to plan-time

- **Configs accessor signature**: whether `*ObjTypeConfigs` and `*ParamTypeConfigs` have public `Get(id int)` accessors, or whether the helper must use the existing `serverConfigsView`-shaped wrapper. Plan-time greps `func.*ObjTypeConfigs.*Get` and `func.*ParamTypeConfigs.*Get` to settle the signature.
- **Test inventory fixture shape**: whether existing `newTestPlayer(t)` and inventory helpers in `appearance_test.go` (`p.invs = map[int]*inventory.Inventory{...}`) extend cleanly to the hunt-test fixture, or whether `npc_hunt_test.go` already has its own player-inventory builder.

#### Bundle 2 commit shape

- **One commit**: `feat(world): NAI-22 Bundle 2 — checkInv huntPlayers filter`
- **Two-stage review**: Stage 1 code review on production change + tests; Stage 2 TS-fidelity whole-impl validates filter shape against TS `Npc.ts:959-969` byte-for-byte and confirms the defensive treat-as-0 deviation is correctly documented (not under-spec'd, not improperly tagged as a tracked deviation).

### Bundle 3 — appearanceInv ctor binding (NAI-21-D1 closure) + byte-search test polish

**Goal**: Close NAI-21-D1's production concern by binding `appearanceInv` to `invs.Worn` immediately after `newPlayer` via a post-construction setter. Retain the `-1` ctor sentinel as test-only safety, retire the deviation tag. Combined with: replace two manual byte-pair scans in `appearance_test.go` with `bytes.Contains` calls.

**Source**: NAI-21 Bundle 1 review M1 follow-up (test polish); NAI-21-D1 deviation closure (post-NAI-21 follow-up).

#### Pre-flight verification

- **Sentinel default**: `appearanceInv: -1` at `modules/world/player.go:322`.
- **Reader fallback**: `appearance.go:32-37` honors `p.appearanceInv == -1` → reads `invs.Worn`.
- **Existing setter**: `pkg/script/active.go:325-332` has `SetAppearanceInv` for the script-side setter that flows through `ScriptState`. Bundle 3's new `(p *Player).setAppearanceInv(int)` is the world-side equivalent (or both routes share an underlying primitive).
- **Production wiring point**: `modules/world/client.go:113` — the single production callsite of `newPlayer`. The `client` has access to `c.s` (the `*Server`), which holds `s.invTypes`. Therefore `s.invTypes.Worn` is reachable from this point.
- **Manual byte-scan loops**: `modules/world/appearance_test.go:104-111` (`TestGenerateAppearanceSentinelDefaultReadsWorn`) and `:185-192` (`TestGenerateAppearanceCustomInvIdHonored`) — both have an identical 4-line byte-pair search.

#### Task 3 (appearanceInv ctor binding closure)

**Production change** (`modules/world/player.go`, new method ~5 LOC):

```go
// setAppearanceInv binds the inv id used by generateAppearance.
// Called from client.go immediately after newPlayer to close the
// NAI-21-D1 production concern (TS init binds appearanceInv at ctor;
// goscape binds it as a deterministic post-ctor wiring step).
//
// The ctor's -1 sentinel default is retained as test-only safety —
// appearance.go's reader fallback handles uninitialized fixtures.
func (p *Player) setAppearanceInv(id int) {
    p.appearanceInv = id
}
```

**Production change** (`modules/world/client.go:113`-area, 1 LOC added immediately after `p := newPlayer(c)`):

```go
p.setAppearanceInv(c.s.invTypes.Worn)
```

The exact accessor — `c.s.invTypes.Worn` (struct field, mirroring the `appearance_test.go:90, 93, 173` test usage `invs.Worn`) — is the canonical guess. Plan-time confirms by grepping `\.Worn\b` consumers in `modules/world/`.

**Ctor doc-comment update** (`modules/world/player.go:322`-adjacent): annotate `appearanceInv: -1` as "test-only sentinel; production binds via setAppearanceInv post-ctor."

**Reader-side comment retag** (`modules/world/appearance.go:25`): replace the "NAI-21-D1: TS init binds at ctor; goscape uses sentinel..." narrative with a "test-only fallback for uninitialized fixtures; production binds via setAppearanceInv" comment. The deviation tag NAI-21-D1 is removed.

**Cross-package narrative cleanup** (`pkg/script/active.go:329`, `pkg/script/handlers_player.go:133`): retire the "as NAI-21-D1, internal-mechanism only" callouts per the `retire_deviation_grep_all_comments` memory. Plan-time runs `rg "NAI-21-D1" pkg/ modules/ cmd/` to enumerate ALL sites, not just production touch points.

**Production delta**: ~6 LOC added (setter + caller); ~10 LOC of doc-comment retags.

#### Task 4 (byte-search test polish)

**Test refactor** (`modules/world/appearance_test.go`, 2 sites):

At each occurrence, replace the 4-line manual loop:

```go
found := false
for i := 0; i < len(p.appearanceBuf)-1; i++ {
    if p.appearanceBuf[i] == wantSlot4Hi && p.appearanceBuf[i+1] == wantSlot4Lo {
        found = true
        break
    }
}
```

with:

```go
found := bytes.Contains(p.appearanceBuf, []byte{wantSlot4Hi, wantSlot4Lo})
```

Add `"bytes"` import if not already present. Failure-message strings unchanged.

**Test delta**: ~-8 LOC across both sites (2 helpers each saving ~4 lines after replacement).

#### Test plan (Bundle 3 combined, ~30 LOC delta)

**Modified tests** (`appearance_test.go`):

- `TestGenerateAppearanceSentinelDefaultReadsWorn`: byte-search loop → `bytes.Contains`. Test name and intent unchanged.
- `TestGenerateAppearanceCustomInvIdHonored`: same refactor.

**New test** (`appearance_test.go`):

- **`TestSetAppearanceInvBindsId`**:
   - Build `p, _ := newTestPlayer(t)`.
   - Confirm `p.appearanceInv == -1` (ctor sentinel).
   - Call `p.setAppearanceInv(42)`.
   - Assert `p.appearanceInv == 42`.

   Trivial pin, but locks the setter's contract independently of the integration test that exercises it through `client.go`.

**Note**: the existing tests `TestGenerateAppearanceSentinelDefaultReadsWorn`, `TestGenerateAppearanceExplicitWornIdMatchesSentinel`, and `TestGenerateAppearanceCustomInvIdHonored` (all from NAI-21 Bundle 1) **stay green** — the reader-side sentinel fallback is preserved unchanged. Bundle 3 adds production binding without disturbing test-time fallback behavior.

**TS-fidelity gate**: ✓ Production now binds `appearanceInv` to Worn before any tick runs. The ctor-literal-vs-post-ctor distinction is internal-mechanism only and not observable from outside the player creation flow. NAI-21-D1's deviation rationale dissolves.

**Deviation impact**: closes NAI-21-D1 (-1). No new deviations introduced.

#### Open questions deferred to plan-time

- **Exact `invTypes.Worn` accessor symbol**: whether `*objtype.InvTypeConfigs` exposes `.Worn` as a struct field (per `appearance_test.go:90-93` usage `invs.Worn`) or via a method. Plan-time greps `invs.Worn` consumers to confirm the symbol shape.
- **`client.go:113`-area exact line**: whether the setter call lands immediately after `p := newPlayer(c)` or after subsequent setup steps (e.g., `c.player = p` assignment). Plan-time reads the surrounding 20 lines and picks the position closest to TS's "ctor-literal" semantic ordering — i.e., before any tick or appearance generation could run.
- **Whether `setAppearanceInv` becomes exported**: currently `pkg/script/active.go:325` has `SetAppearanceInv` (capitalized — accessible from script side). The world-side setter is package-private (lowercase) as proposed. Plan-time confirms the script-side setter and world-side setter remain distinct (they cross package boundaries) or unifies under one entry point.

#### Bundle 3 commit shape

- **One commit**: `polish(world): NAI-22 Bundle 3 — appearanceInv ctor binding (NAI-21-D1 closed) + byte-search test polish`
- **Single light review** per `compressed_cadence` ~15-LOC threshold (Bundle 3's combined production delta is ~6 LOC + ~10 LOC doc-comment retags + ~30 LOC of test edits, well under the standard two-stage threshold). Light review pass validates: NAI-21-D1 callouts removed everywhere (`rg "NAI-21-D1"` returns zero hits), setter is invoked from production wiring, sentinel-fallback-as-test-only narrative is consistent across `appearance.go` / `player.go` / `pkg/script/`.

## Test strategy summary

| Bundle | New tests | Modified tests | LOC delta |
|---|---|---|---|
| Bundle 1 | 5 | 0 | ~120 |
| Bundle 2 | 6 | 0 | ~250 |
| Bundle 3 | 1 (new setter pin) | 2 (byte-search refactor) | ~30 |

**Total tests**: 12 new + 2 modified.

**Verification protocol** (per `verify_implementer_claims` memory): after each bundle commit, run:

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

at HEAD (post-commit) and confirm green before claiming the bundle complete. Pre-existing failures must be verified at `HEAD~1` per the same memory entry.

## TS-fidelity gates (per-task)

| Task | TS reference | Mechanism | Gate |
|---|---|---|---|
| Bundle 1 producer | `World.ts:1284-1289` | SPAWN-event queue insertion | ✓ |
| Bundle 1 dispatch | `World.ts:664-673` | type-agnostic processNpcEventQueue | ✓ (existing) |
| Bundle 1 ordering | `World.ts:356` | tick-time queue processing | ✓ (existing) |
| Bundle 2 filter | `Npc.ts:959-969` | CheckInv branch + CheckHuntCondition | ✓ |
| Bundle 2 invTotal | `Player.ts:1556` | inv-id + obj-id → total count | ✓ |
| Bundle 2 invTotalParam | `Player.ts:1668` | inv-id + param-id → param sum | ✓ |
| Bundle 3 binding | `Player.ts` ctor | `appearanceInv = wornId` | ✓ (post-ctor wiring; ctor-literal-vs-post-ctor is internal-mechanism only) |
| Bundle 3 polish | N/A | Test-only refactor | N/A |

## Deviation accounting

- **Pre-NAI-22**: 16 tracked deviations.
- **Closes**:
  - NAI-19-D2 (-1) via Bundle 1.
  - NAI-21-D1 (-1) via Bundle 3.
- **Introduces**: 0.
- **Post-NAI-22**: **14 tracked deviations** (net -2).

The Bundle 2 missing-inv defensive treat-as-0 is a TS divergence on a dead-in-practice path (TS throws on missing inv; goscape iterates with quantity=0). Captured in code comment with rationale, no deviation tag — the TS throw-path is unreachable for live players, and "iteration-survival under bad data" is itself the divergence reason. Plan-time reviewer may push back; if a tag is required, the math becomes: -2 (closes) + 1 (new) = **15** post-NAI-22.

## Cadence and review structure

- **Bundles 1 + 2**: Two-stage review per `runescript_cadence` (Stage 1 code review + Stage 2 TS-fidelity whole-impl review).
- **Bundle 3**: Single light reviewer pass, justified by ~6 LOC of behavioral production code (the setter + caller) + ~10 LOC of doc-comment retags + test polish — the behavioral surface fits the `compressed_cadence` ≤15-LOC threshold even though combined-line touches are higher.
- **Per `controller_preflight` memory**: 30-second grep+Read pass against HEAD before each implementer dispatch to catch stale plan premises.
- **Per `verify_implementer_claims` memory**: independent fresh `go test ./...` run after each bundle commit; never accept "pre-existing failures" without verification at `HEAD~1`.
- **Per `dispatching-parallel-agents` memory**: Bundle order is sequential (B1 → B2 → B3). Bundle 1 and Bundle 2 each touch independent production files (`npc_registry.go` vs `npc_hunt.go`) so could parallelize in principle, but sequential dispatch matches established post-NAI-12 cadence and avoids cross-bundle false-green / stale-IDE-diagnostic compounding (see `verify_implementer_claims` memory failure modes).
- **Per `consume_reserved_constant` memory**: Bundle 1's reviewer Stage 2 explicitly validates the 5-element dispatch-path checklist (reserved constant, producer, processor, tick wiring, end-to-end dispatch test).
- **Per `retire_deviation_grep_all_comments` memory**: Bundle 3's plan-time and Stage 1 review both run `rg "NAI-21-D1" pkg/ modules/ cmd/` to enumerate all retirement sites. Spec author's pre-flight already identified the four primary sites (`appearance.go:25`, `player.go:322`-adjacent, `pkg/script/active.go:329`, `pkg/script/handlers_player.go:133`); plan-time re-greps to catch any missed callouts in test files or doc comments.

## Out of scope

- **NAI-19-D1 (zone state during respawn)**: requires Zone abstraction infrastructure port. Multi-hundred-LOC sub-spec on its own. Not addressed by NAI-22.
- **Other huntPlayers filters** (`checkNotBusy` blocked on no Player.Busy() infra; `checkNotTooStrong` blocked on wilderness + combat-level computation). Each requires a different infrastructure port. Not addressed by NAI-22.
- **AI_SPAWN script content / cache wiring**: NAI-22 Bundle 1 establishes the dispatch path. Whether any actual `ai_spawn` scripts exist in the cache and what their semantics imply is a content-side concern, not a wiring-side concern. Not addressed.
- **Java-client smoke runs**: Bundle 1's producer activation in production could be verified end-to-end via Java-client smoke (per `smoke_test_server_handoff` memory), but only if a SPAWN script is registered in the test cache. Not part of NAI-22 close — unit tests are sufficient for the wiring proof.
- **CheckObj-and-CheckObjParam-both-set ambiguity**: TS handles via `if/else if` (CheckObj wins). NAI-22 mirrors. Whether HuntType decoder permits both being set simultaneously is a config-validity question, not a runtime question. Not addressed by NAI-22.

## Memory entries to update on close

Per `close_commit_memory_trailer` memory: NAI-22 close commit will carry a `Closes memory:` trailer enumerating the memory entries this sub-spec validates or invalidates. Candidate entries (final list pinned at close-commit time):

- `consume_reserved_constant.md` — Bundle 1 is the textbook case. Validates the memory's "new consumer owns the full dispatch path" guidance and the 5-element checklist (reserved constant, producer, processor, tick wiring, end-to-end test) end-to-end.
- `controller_preflight.md` — NAI-22's pre-flight caught the (a) "design questions resolved" finding and the (d) "ctor-thread is too invasive" finding before plan-write.
- `spec_followup_tracker_freshness.md` — pre-flight verified each candidate's tracker assertion (file paths, line numbers, infrastructure status) at HEAD `01de242`, not just at implementer dispatch.
- `compressed_cadence.md` — Bundle 3 is the canonical example: combined production change + test polish at ≤15 LOC of production delta uses the lighter review cadence.
- `retire_deviation_grep_all_comments.md` — Bundle 3 explicitly invokes the grep enumeration at plan-time and Stage 1 review.
- `plan_grep_helper_patterns.md` — Bundle 2's design-time grep for inventory primitives caught that `Inventory.GetItemCount` already exists, avoiding the inline-vs-Player-method false dichotomy.
- `plan_helper_coverage.md` (sideways application) — Bundle 3's design-time grep of `newPlayer(` callers (18 sites) drove the (i)→(ii) recommendation flip from ctor-thread to post-construction setter.
