# NAI-21 — follow-up bundle (LoS snapshot LoS-path completion + S7c-D1 closure + NewNpc loop modernize + stale NAI-17-D1 retirement + NAI-3 weak-form strengthening)

- **Sub-spec**: NAI-21
- **Date**: 2026-04-25
- **Scope label**: B (logical-grouping follow-up bundle — `modules/world` (production + tests), `nai_followups.md` doc retirements; ~10 LOC production + ~130 LOC tests + ~60 LOC doc; closes 2 follow-up entries (NAI-17-D1 tracker + NAI-3 weak-form deferral) + 1 tracked deviation (S7c-D1); introduces 1 tracked deviation (NAI-21-D1, internal-mechanism only); net deviation count unchanged at 16)
- **Predecessors**: NAI-20 (follow-up bundle) — last on `main` as `3514264`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

Five tracked NAI-series follow-ups have accumulated since NAI-20 closed, spanning three categories: (i) a TS-fidelity snapshot rollout completion deferred from NAI-20, (ii) a TS-fidelity reader fix tracked as S7c-D1, (iii) a polish-modernization ding from NAI-17, (iv) a stale follow-up tracker (NAI-17-D1) whose underlying gap was closed by NAI-19 but the tracker entry was never retired, and (v) a NAI-3 weak-form test that the original deferral note flagged as needing test-fixture infrastructure.

Pre-flight at HEAD `3514264` against the original deferral notes surfaced two stale-premise findings:

1. **NAI-17-D1's underlying gap is already closed.** The follow-up entry asserts goscape's `revertType` heavy path does an inline reset; in fact `modules/world/npc.go:285-286` already does the structural `s.removeNpc(n, -1) + s.addNpc(n, -1, false)` cycle. NAI-19 closed the inline-reset gap and replaced it with two narrower deviations (NAI-19-D1 zone state, NAI-19-D2 AI_SPAWN re-trigger). The NAI-17-D1 follow-up entry is now doc-rot.
2. **NAI-3 weak-form's "neither fixture exists today" claim is fully stale.** `Provider.Register(*ScriptFile)` at `pkg/script/provider.go:182` is exported with a docstring explicitly saying *"Intended for tests that want to exercise the provider without loading a real cache."* `*script.ScriptFile` and its `LookupKey` field are both exported (`pkg/script/file.go:18`). `buildNpcForIntegration(t)` already wires a `scriptProvider` via `defaultTestProvider()`. The strong-form test can land using existing exported APIs with no new fixture infrastructure.

The 5 items cluster naturally into three logical bundles by content type (production surgical fixes / polish-and-doc / test infra), each landing as one commit with bundle-scoped review.

**Bundle rationale (Order B from brainstorm)**: production-code fixes share a coherent reviewer narrative (snapshot promotion + reader correction); polish-and-doc items share a non-behavioral nature warranting light review; the NAI-3 strengthening is the largest single item and stands alone for full review. Three commits with three reviewer passes (two-stage on Bundles 1+3, light single-stage on Bundle 2) instead of five per-task two-stage cycles.

## Tech stack

- Go 1.26+
- Existing packages touched:
  - `modules/world/npc_interaction.go` (Bundle 1, Task (a): 2 line changes — snapshot reads at `:532` and `:581`)
  - `modules/world/appearance.go` (Bundle 1, Task (d): 6 lines added — sentinel-handling reader)
  - `modules/world/npc.go` (Bundle 2: 2 lines changed at `:164` — modernize loop)
  - `modules/world/npc_interaction_test.go` (Bundle 1, Task (a) tests)
  - `modules/world/appearance_test.go` (Bundle 1, Task (d) tests)
  - `modules/world/npc_script_test.go` (Bundle 3: replace weak-form test with strong form)
- Doc files touched:
  - `nai_followups.md` (Bundle 2: retire NAI-17-D1 entry; Bundle 3: retire NAI-3 weak-form deferral)
- No new files, no new packages, no new exported types.

## Scope

### Bundle 1 — Production surgical fixes (single commit)

#### Task (a) — LoS-path snapshot promotion (~5 LOC + ~30 LOC tests)

**Goal**: Promote the two LoS-path reads in `npc_interaction.go` from live `n.typ.Size` / `t.typ.Size` to snapshot fields `n.size` / `t.size`. Mirrors TS `PathingEntity.width` ctor-snapshot semantics (TS `World.ts:1271, 1302`).

**Source**: From-NAI-20 follow-up entry at `nai_followups.md:1229-1281`.

**Code changes** (`modules/world/npc_interaction.go`):

| Line | Before | After |
|---|---|---|
| 532 (`approachEntitySize`, `*Npc` branch) | `size := int(t.typ.Size)` | `size := int(t.size)` |
| 581 (`inApproachDistance`, self side) | `selfSize := int(n.typ.Size)` | `selfSize := int(n.size)` |

Net production delta: **2 lines changed**.

The snapshot fields `n.size` and `n.blockWalk` were introduced in NAI-20 Task 2 (commit `ec0c5e7`) and seeded at `NewNpc` from `typ.Size` / `typ.BlockWalk`. They are intentionally NOT updated by changetype, mirroring TS's ctor-snapshot semantics. NAI-20's scope deliberately limited snapshot consumption to collision-toggle call sites in `addNpc`/`removeNpc` (`npc_registry.go:65-72, 140-147`); NAI-21 picks up the LoS-path remainder.

**Test plan** (`modules/world/npc_interaction_test.go`, ~30 LOC, 2 tests, dual-pin pattern per `ts_asymmetry_dual_pin` memory):

1. **`TestInApproachDistanceUsesSelfSizeSnapshotNotTyp`** (self-side):
   - Construct an NPC with `n.size = 2` (via NewNpc on a size-2 typ).
   - Changetype to a size-1 type so `n.typ.Size = 1` while `n.size` stays 2.
   - Set up a collision scenario where `selfSize=2` produces a different LoS result than `selfSize=1` (e.g., a wall configuration that blocks size-2 self but passes for size-1).
   - Assert `inApproachDistance` returns the result corresponding to `selfSize=2` (snapshot-honoring), not `selfSize=1` (typ-following).

2. **`TestApproachEntitySizeUsesNpcSizeSnapshotNotTyp`** (target-side):
   - Same NPC setup but as the *target*.
   - Call `approachEntitySize(t)` directly.
   - Assert it returns `(2, 2)` (from `t.size`) not `(1, 1)` (from `t.typ.Size`).

**TS-fidelity gate**: ✓ Snapshot-read matches `PathingEntity.width` ctor-snapshot semantics (TS `PathingEntity.ts:402-405`).

**Deviation impact**: 0 (closes a soft-tracked TS-divergence-on-paper that was flagged in NAI-20's whole-impl review but never assigned a Dn ID).

#### Task (d) — S7c-D1 closure: appearanceInv reader fix (~6 LOC + ~50 LOC tests)

**Goal**: Read `p.appearanceInv` instead of the global `invs.Worn` constant in `generateAppearance`, matching TS `Player.ts:1318` (`let worn = this.getInventory(this.appearanceInv);`). Closes deviation S7c-D1.

**Source**: From-S7c follow-up entry at `nai_followups.md:1205-1227`.

**Code changes** (`modules/world/appearance.go:25-28`):

Before:
```go
var worn *inventory.Inventory
if p.invs != nil {
    worn = p.invs[invs.Worn]
}
```

After:
```go
// NAI-21-D1: TS init binds appearanceInv to Worn at ctor; goscape uses
// -1 sentinel and maps it here for behavioral parity. Internal mechanism
// only — observationally identical for production callers because every
// production caller either (i) passes through SetAppearanceInv before
// generateAppearance fires, or (ii) is a fresh player whose first read
// must surface worn-inv items.
var worn *inventory.Inventory
if p.invs != nil {
    inventoryId := p.appearanceInv
    if inventoryId < 0 {
        inventoryId = invs.Worn
    }
    worn = p.invs[inventoryId]
}
```

Net production delta: **6 lines added (incl. comment) / 1 line changed**.

**Sentinel-handling rationale**: `newPlayer(c *client)` at `player.go:293` has no `invs` parameter and follows a uniform "sentinel-init then resolve at use site" convention (every other `-1` field at lines 303-345 follows the same pattern). Threading `invs.Worn` into ctor would break that convention and require touching call sites. Sentinel-in-reader preserves production behavior exactly while honoring TS `Player.ts:1318` reader semantics.

**Existing-test impact**: Both existing tests (`TestGenerateAppearanceNakedPlayer`, `TestGenerateAppearancePlatebodyEquipped` at `appearance_test.go:39, 62`) construct a player via `newTestPlayer(t)` which inherits `appearanceInv = -1` (the default at `player.go:322`). With the sentinel-mapping in place, they continue to read `p.invs[invs.Worn]` exactly as before — **no existing-test changes required**.

**Test plan** (`modules/world/appearance_test.go`, ~50 LOC, 3 tests):

1. **`TestGenerateAppearanceSentinelDefaultReadsWorn`** (regression equivalence):
   - Player with `appearanceInv = -1` (the default).
   - Equip a platebody at `p.invs[invs.Worn].Items[4]`.
   - Run `generateAppearance`.
   - Assert the appearance buffer reflects the equipped platebody (proves sentinel-mapping fallback to Worn works).

2. **`TestGenerateAppearanceExplicitWornIdMatchesSentinel`** (new-behavior pin, equivalence):
   - Player with `appearanceInv = invs.Worn` (explicitly set, not via sentinel).
   - Same platebody setup.
   - Assert byte-identical output to Test 1 (proves the explicit-set path matches the sentinel-default path).

3. **`TestGenerateAppearanceCustomInvIdHonored`** (new-behavior pin, divergence):
   - Player with two inventories: `p.invs[invs.Worn]` (empty) and `p.invs[customInvId]` (with platebody at slot 4).
   - Set `p.appearanceInv = customInvId`.
   - Run `generateAppearance`.
   - Assert the output reflects the platebody from the custom inv, NOT the empty Worn inv. **This is the actual S7c-D1 bug fix proof.**

**TS-fidelity gate**: ✓ Reader matches TS `Player.ts:1318`. Sentinel-handling is internal-mechanism deviation tagged as NAI-21-D1.

**Deviation impact**: -1 (closes S7c-D1) +1 (introduces NAI-21-D1) = 0 net.

**No smoke-test required** (per brainstorm decision D1): production callers either pass through SetAppearanceInv before generateAppearance fires, or rely on the sentinel→Worn mapping which preserves byte-identical pre-fix behavior.

#### Bundle 1 commit shape

- **One commit**: `feat(world): NAI-21 Bundle 1 — LoS snapshot LoS-path completion + S7c-D1 closure`
- **Two-stage review** per `runescript_cadence`:
  - Stage 1: per-bundle code review by `superpowers:code-reviewer` — touch-file diffs, sentinel-handling justification, test coverage matches plan.
  - Stage 2: whole-impl TS-fidelity review (separate reviewer pass) — NAI-21-D1 introduction is justified vs. ctor-init alternative; deviation count audit.

### Bundle 2 — Polish & doc cleanup (single commit, light review)

#### Item 1 — Modernize NewNpc stats-seeding loop (~2 LOC)

**Source**: From-NAI-17 follow-up at `nai_followups.md:663-674`.

**Code change** (`modules/world/npc.go:164`):

Before:
```go
for i := 0; i < objtype.NpcStatCount && i < len(typ.Stats); i++ {
    n.NpcStat[i] = typ.Stats[i]
}
```

After:
```go
for i := range min(objtype.NpcStatCount, len(typ.Stats)) {
    n.NpcStat[i] = typ.Stats[i]
}
```

Net delta: **2 lines changed**. Behaviorally identical; pure style consistency with three siblings already modernized (`revertType` heavy-path reseed at `npc.go:288`, `resetStatsForType` at `npc_masks.go:98`, `processNpcRegen` at `npc_script.go:244`).

**Test plan**: No new tests. Existing `NewNpc`-construction tests serve as regression gate (`go test ./modules/world/... -run TestNpc`).

#### Item 2 — Retire stale NAI-17-D1 follow-up tracker (~30 LOC doc)

**Source**: NAI-17-D1 follow-up entry at `nai_followups.md:676-694`.

**Action**: Mark the entry as **Resolved 2026-04-25 (NAI-21 Bundle 2; superseded by NAI-19's structural `removeNpc+addNpc` port; remaining deviation surface tracked as NAI-19-D1 (no zone state, `npc_registry.go:63, 136`) and NAI-19-D2 (no AI_SPAWN re-trigger, `npc_registry.go:77`))**, preserving the original deferral body for historical context (matching the resolution pattern established in NAI-20's resolved entries at `nai_followups.md:31, 698, 973`).

**Grep enumeration** (per `retire_deviation_grep_all_comments` memory):

Implementer must run before editing:

```
grep -rn "NAI-17-D1" /home/owner/Code/github.com/zsrv/goscape/pkg \
                     /home/owner/Code/github.com/zsrv/goscape/modules \
                     /home/owner/Code/github.com/zsrv/goscape/cmd \
                     /home/owner/Code/github.com/zsrv/goscape/docs
```

Pre-flight result (HEAD `3514264`): zero production-code references; only `nai_followups.md` mentions. If the implementer-time grep surfaces additional sites, they must be enumerated and updated as part of this task.

#### Bundle 2 commit shape

- **One commit**: `polish(world): NAI-21 Bundle 2 — modernize NewNpc stats loop + retire NAI-17-D1 tracker`
- **Light review only** (single reviewer pass), justified by: doc-only + cosmetic-only changes, no behavioral surface, well within the `compressed_cadence` ~15-LOC threshold for the bundle as a unit.
- **Reviewer scope**: verify (i) the NewNpc loop is byte-identical in semantics to the pre-modernized form, (ii) the grep enumeration ran and surfaced zero production sites, (iii) the resolved-entry follows the established NAI-20 resolution-pattern formatting.

### Bundle 3 — NAI-3 weak-form NPC queue test strengthening (single commit)

#### Pre-flight finding (changes the design materially)

The original deferral note at `nai_followups.md:88-105` claims "neither fixture exists today" — referring to a `RegisterForTest`-style method on `*script.Provider` and a synthetic test-only opcode. Verified at HEAD `3514264`:

- **`Provider.Register(*ScriptFile)`** (`pkg/script/provider.go:182`) is exported and its docstring explicitly says: *"Intended for tests that want to exercise the provider without loading a real cache."*
- **`*script.ScriptFile`** is exported with a public `LookupKey uint32` field (`pkg/script/file.go:18`). Existing tests already construct `*ScriptFile` literals with explicit `LookupKey` values (`pkg/script/handlers_core_test.go:12, 22, 32`).
- **`buildNpcForIntegration(t)`** (`modules/world/npc_script_test.go:228`) returns a `*Server` from `newServerForScriptTest(t)` (`npc_script_test.go:54`), which is a bare `*Server` with only `log` set — `scriptProvider` is **nil**. The test must seed `s.scriptProvider = script.NewProvider()` itself before calling `Register()`. The pattern is established and documented at `server_test.go:290-291` (the `defaultTestProvider` docstring explicitly says "tests asserting the absence of those fields must seed an empty provider"). Plan-time decides whether to lift the seed into `buildNpcForIntegration` or do it inline in the strong-form test.
- **`processNpcQueue`** (`modules/world/npc_script.go:266-290`) removes queue entries BEFORE firing (line 282) and iterates against live `len(n.queue)` (line 271). The production code already supports the strong-form behavior; comment at lines 257-259 documents this explicitly.

**Conclusion**: No new fixture infrastructure required. Bundle 3 is a test-only addition using already-exported APIs. The brainstorm-time choice of "Option 3 (test-only, package-private, idiomatic Go)" is honored trivially because the test code itself lives in `npc_script_test.go` and consumes existing exported types.

#### Test design (strong form)

**Source**: NAI-3 weak-form deferral at `nai_followups.md:88-115`.

**Goal**: Replace `TestNpcTurnReentryQueueAppendDuringIteration` (`npc_script_test.go:280-297`) with a strong form that proves a script fired mid-iteration of `processNpcQueue` can append a new entry visible to the same pass. Mirrors TS `Npc.ts:538-560` "speedup quirk" semantics.

**Test shape**:

```
1. Build server + NPC via buildNpcForIntegration(t).

2. Construct an "amplifier" *script.ScriptFile:
   - LookupKey = uint32(script.TriggerAiQueue1)      ← global trigger key
   - Opcodes: bytecode invoking OpNpcQueue with operand-encoded args
     (target = TriggerAiQueue2, delay = 0, intArg = 0).
   - One additional observable side-effect op (plan-time pin: e.g., an
     opcode that writes a sentinel to a varN or an NPC field the test
     can read post-turn). Proves the amplifier actually executed
     rather than the queue draining via a silent dispatch failure.

3. (Optional) Construct a "marker" *script.ScriptFile:
   - LookupKey = uint32(script.TriggerAiQueue2)
   - Opcodes: [OpReturn]                              ← bare no-op
   - Strictly OPTIONAL. The queue drains regardless of whether the
     marker is registered, because processNpcQueue (npc_script.go:282)
     removes the entry BEFORE the trigger lookup, and runNpcScript
     (npc_script.go:156) short-circuits on nil ScriptFile. Registering
     the marker provides end-to-end-dispatch confidence (the appended
     entry's script actually executes); without it, the test still
     proves queue mechanics correctly. Plan-time decides whether to
     include the marker for end-to-end coverage or omit it for
     simpler test surface.

4. Seed s.scriptProvider = script.NewProvider() (NOT done by
   buildNpcForIntegration). Then register the amplifier (and
   marker if included) via s.scriptProvider.Register(...).

5. Pre-flight assertion (wiring guard):
   - GetByTrigger(TriggerAiQueue1, n.typeId, n.typ.Category) == amplifier
   This pins amplifier wiring; without it, the side-effect assertion
   could fail for the wrong reason (e.g., wrong LookupKey computation).
   If marker is included, also assert:
   - GetByTrigger(TriggerAiQueue2, n.typeId, n.typ.Category) == marker

6. Action:
   - n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 0, 0)
   - n.turn(s)

7. Assertions:
   - len(n.queue) == 0   (proves both the original AND the
                          amplifier-appended entry drained in the
                          same pass).
   - The amplifier's observable side-effect fired (e.g., the varN
     written sentinel matches expected) — proves the amplifier
     actually ran, distinguishing from a silent dispatch failure.
```

**Failure-mode coverage**:
- A regression where `processNpcQueue` switches to snapshot-len iteration would leave 1 entry in `n.queue` after `turn` — the queue-len assertion catches it.
- A regression where the amplifier silently no-ops (dispatch wired wrong) would still drain queue to 0 — the side-effect assertion catches that case.

**Open question deferred to plan-time**: The exact `OpNpcQueue` operand-encoding shape (push order, intOperands vs stack args) needs verification against `handleNpcQueue` (`pkg/script/handlers_npc.go:295-318`) and the existing test patterns at `pkg/script/handlers_npc_test.go:729+` which already construct synthetic `OpNpcQueue` ScriptFiles. This is implementation detail, not architectural — plan-time pin.

**Open question deferred to plan-time**: The exact opcode for the "side-effect" portion of the amplifier's bytecode. Candidates: an `OpSetVarN`-style write, an `OpSetTimer` write, or an opcode that mutates an NPC field directly readable from the test. Plan-time selects based on what's already wired in `defaultTestProvider()`'s opcode table.

#### Code change inventory

| File | Change | LOC delta |
|---|---|---|
| `modules/world/npc_script_test.go:280-297` | Replace weak-form test with strong form | ~+60 / -18 ≈ +42 net |
| `modules/world/npc_script_test.go` | Optional helper `buildNpcQueueAmplifier(target ServerTrigger) *ScriptFile` if a chain-test extension is added | +10-20 if used |
| `nai_followups.md:88-115` | Mark NAI-3 weak-form deferral as Resolved, preserve historical body | Doc-only |

**No production code changes. No new helpers in `pkg/script/`. No subpackages.**

#### Bundle 3 commit shape

- **One commit**: `test(world): NAI-21 Bundle 3 — strengthen NAI-3 weak-form NPC queue test + retire deferral`
- **Two-stage review** despite test-only nature, justified by: (i) Bundle 3 is the largest single bundle, (ii) the test is the regression gate for a TS-fidelity-load-bearing production behavior (the "speedup quirk"), (iii) the dual side-effect+queue-len assertion shape is non-obvious and reviewer pass should validate it catches both failure modes.

## Test strategy summary

| Bundle | New tests | Modified tests | LOC delta |
|---|---|---|---|
| Bundle 1 | 2 (Task a) + 3 (Task d) = 5 | 0 | ~80 |
| Bundle 2 | 0 | 0 | 0 |
| Bundle 3 | 0 (replacement) | 1 (`TestNpcTurnReentryQueueAppendDuringIteration`) | ~50 |

**Verification protocol** (per `verify_implementer_claims` memory): after each bundle commit, run:

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

at HEAD (post-commit) and confirm green before claiming the bundle complete. Pre-existing failures must be verified at `HEAD~1` per the same memory entry.

## TS-fidelity gates (per-task)

| Task | TS reference | Mechanism | Gate |
|---|---|---|---|
| (a) self-side | `PathingEntity.ts:402-405` | Snapshot-read of `self.width` | ✓ |
| (a) target-side | `PathingEntity.ts` (sister `width` access) | Snapshot-read of target's `width` | ✓ |
| (d) reader | `Player.ts:1318` (`this.getInventory(this.appearanceInv)`) | Reader uses `p.appearanceInv`; sentinel-mapping is internal-mechanism only | ✓ (NAI-21-D1 tracked) |
| Bundle 2 modernize | N/A | Go-style consistency only | N/A |
| Bundle 2 retire | N/A | Doc-only | N/A |
| Bundle 3 strong form | `Npc.ts:538-560` ("speedup quirk") | Production code already supports; test pins it | ✓ |

## Deviation accounting

- **Pre-NAI-21**: 16 tracked deviations.
- **Closes**: S7c-D1 (-1) via Bundle 1 Task (d).
- **Introduces**: NAI-21-D1 (+1, internal-mechanism deviation: appearanceInv sentinel-handling in reader vs TS ctor-init).
- **Post-NAI-21**: 16 tracked deviations (net unchanged).

The NAI-17-D1 retirement is a doc-tracker retirement only (no code-side D-tag was ever added; NAI-19's deviations NAI-19-D1 and NAI-19-D2 supersede the tracker's concern). No deviation count delta from Bundle 2.

## Cadence and review structure

- **Bundles 1 + 3**: Two-stage review per `runescript_cadence` (Stage 1 code review + Stage 2 TS-fidelity whole-impl review).
- **Bundle 2**: Single light reviewer pass, justified by doc-only + cosmetic-only nature within the `compressed_cadence` ~15-LOC threshold for the bundle as a unit.
- **Per `controller_preflight` memory**: 30-second grep+Read pass against HEAD before each implementer dispatch to catch stale plan premises.
- **Per `verify_implementer_claims` memory**: independent fresh `go test ./...` run after each bundle commit; never accept "pre-existing failures" without verification at `HEAD~1`.
- **Per `dispatching-parallel-agents` memory**: Bundle order is sequential (B1 → B2 → B3), not parallel — see brainstorm Order B confirmation. Sequential dispatch matches established post-NAI-12 cadence and avoids cross-bundle false-green / stale-IDE-diagnostic compounding.

## Out of scope

- **Other huntPlayers filters** (checkNotBusy, checkNotTooStrong, checkInv): each blocked on different missing infra (player-state subsystem, wilderness/combat-level computation, inventory-query primitives). Tracked separately at `nai_followups.md` huntPlayers section. Not addressed by NAI-21.
- **NAI-19-D1 (zone state during respawn)**: requires Zone abstraction infrastructure. Not addressed.
- **NAI-19-D2 (AI_SPAWN re-trigger)**: requires trigger-queue plumbing in `addNpc(firstSpawn=false)`. Not addressed.
- **Java-client smoke runs**: per Bundle 1 Task (d) D1 unit-test-only resolution; the byte-equivalence of the sentinel-mapping fallback removes the smoke-test gate that the original S7c-D1 deferral note assumed.
- **Other weak-form retrofits across the NAI series**: Bundle 3 strengthens only `TestNpcTurnReentryQueueAppendDuringIteration`. The fixture pattern it establishes (Provider.Register + LookupKey + amplifier+marker) is reusable for future weak-form retrofits but those are tracked separately and out of scope for NAI-21.

## Memory entries to update on close

Per `close_commit_memory_trailer` memory: NAI-21 close commit will carry a `Closes memory:` trailer enumerating the memory entries this sub-spec validates or invalidates. Candidate entries (final list pinned at close-commit time):

- `controller_preflight.md` — NAI-21's pre-flight caught the NAI-17-D1 stale-premise and the NAI-3 stale-fixture-claim before plan-write.
- `retire_deviation_grep_all_comments.md` — Bundle 2 explicitly invokes the grep enumeration at implementer-dispatch time.
- `ts_asymmetry_dual_pin.md` — Bundle 1 Task (a) tests follow the dual-pin (snapshot-honoring AND typ-following-absence) pattern.
- `dead_api_polish.md` — Bundle 1 Task (d) pattern: catch the helper-without-consumer (the appearanceInv field set by SetAppearanceInv but unread by the reader) at sub-spec close, not via separate cleanup.
