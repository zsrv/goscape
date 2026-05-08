# NAI-123 — Zero-damage residual: ai_queue `last_int` plumbing gap

**Status:** spec — draft 1
**Date:** 2026-05-07
**Predecessor:** NAI-122 close (`9ebd2ed`); residual #1 (zero-damage / non-zero XP).
**Cadence:** investigation_subspec_cadence — Stage 1 short-circuited per `bundle0_short_circuits_stage1_audit` (Bundle 0 produced a binding line-level TS-vs-goscape diff). Stage 2 fix → user-launched smoke.
**Tech stack:** Go 1.26+.

## §0 — One-line summary

`processNpcQueue` builds the dispatched script's state without copying the queued `lastInt` arg, so `[ai_queueN,_] ~proc(last_int);` scripts read 0 and apply 0 damage even when the player rolled a non-zero hit.

## §1 — Symptom and binding evidence

**Smoke (NAI-122 close, 2026-05-07):**
- Tutorial Island player attacks giant rat with bronze dagger.
- All hits show 0-damage hitsplats (blue).
- Combat XP per hit = 75 (vs pre-NAI-122 = 0 XP).
- `%npc_combat_xp_multiplier` reads correctly (NAI-122 V-PARTIAL fix bound).

**XP-vs-damage decoupling proof (content-side):**

`Content/scripts/skill_combat/scripts/player/player_melee.rs2:19-47`:

```
def_int $damage = 0;
if (~player_npc_hit_roll(%damagetype) = true) {
    def_int $maxhit = %com_maxhit;
    ...
    $damage = randominc(min($maxhit, npc_param(max_dealt)));
    def_int $damage_capped = min($damage, npc_stat(hitpoints));
    ~give_combat_experience(%damagestyle, $damage_capped, %npc_combat_xp_multiplier);
    npc_heropoints($damage_capped);
    ...
}
...
npc_queue(2, $damage, 0);
```

`give_combat_experience` runs synchronously inside `player_melee.rs2`, granting XP from the locally-rolled `$damage_capped`. The `npc_queue(2, $damage, 0)` call enqueues `ai_queue2` on the rat, which is the path that handles the actual NPC HP decrement + hitsplat:

`Content/scripts/skill_combat/scripts/npc/npc_combat.rs2:2`:

```
[ai_queue2,_] ~npc_default_damage(last_int);
```

→ `npc_default_damage` proc → `npc_damage(amount, type)` → goscape `handleNpcDamage` (handlers_npc.go:300) → `Npc.Damage(...)` → applyDamage / hitsplat.

So if `last_int` reads 0 in the ai_queue2 dispatch, the NPC takes 0 damage but the player's local $damage was non-zero (XP awarded).

## §2 — Root cause: TS line-level diff

**TS `Npc.ts:538-560`** (processQueue):

```ts
private processQueue() {
    if (!this.isActive) return;
    for (const request of this.queue.all()) {
        if (!this.delayed) request.delay--;
        if (!this.delayed && request.delay <= 0) {
            request.unlink();
            const type: NpcType = NpcType.get(this.type);
            const script = ScriptProvider.getByTrigger(request.queueId, type.id, type.category);
            if (script) {
                const state = ScriptRunner.init(script, this, null, request.args);  // request.args = []
                state.lastInt = request.lastInt;  // <-- THE LINE
                this.executeScript(state);
            }
        }
    }
}
```

**TS `Npc.ts:241-245`** (enqueueScript constructor):

```ts
enqueueScript(queueId: number, delay = 0, arg: number = 0) {
    const request = new NpcQueueRequest(queueId, [], delay);  // empty args[]
    request.lastInt = arg;                                    // arg stored in lastInt
    this.queue.addTail(request);
}
```

**TS `NpcQueueRequest.ts`:** `args: ScriptArgument[]` (positional, always [] at the one enqueue site) AND `lastInt: number = 0` (separate field) — two distinct fields.

**Goscape `pkg/script/queue.go:42-46`:**

```go
type NpcQueueRequest struct {
    Trigger ServerTriggerType
    Delay   int
    IntArg  int
}
```

Single `IntArg` conflated.

**Goscape `modules/world/npc_script.go:483-490`:**

```go
trigger := req.Trigger
intArg := req.IntArg
n.queue = append(n.queue[:i], n.queue[i+1:]...)
if s.scriptProvider == nil { continue }
sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
s.runNpcScript(sf, n, nil, []int{intArg}, nil)  // intArg passed as POSITIONAL via intArgs[0]
```

`runNpcScript` → `buildNpcScriptState` → `script.Init(sf, nil, false, intArgs, stringArgs)`. `script.Init` (pkg/script/runner.go:30-32) writes `intArgs` into `IntLocals[0]`, never touches `state.LastInt` (which stays at Go zero-value 0).

The ai_queue script declaration `[ai_queue2,_]` has NO positional args, so `IntLocals[0] = $damage` is unused. `last_int` opcode (`handleLastInt` at handlers_dialog.go:40) pushes `state.LastInt = 0`. → `npc_default_damage(0)` → `Npc.Damage(0, type)` → 0 hitsplat.

## §3 — Sibling dispatch sites verified TS-aligned (no fix needed)

| Site | TS line | Goscape line | Shape | Verdict |
|---|---|---|---|---|
| Walktrigger | Npc.ts:354 | npc_interaction.go:304 | `ScriptRunner.init(script, this, null, [walktriggerArg])` | ✓ matches; walktrigger scripts declare `(int $arg)`, positional via IntLocals[0]. |
| consumeHuntTarget QUEUE branch | Npc.ts:901 | npc_hunt.go:343 | `ScriptRunner.init(script, this, null, null)` | ✓ matches; no args at all. |
| processQueue (ai_queue) | Npc.ts:554-555 | npc_script.go:490 | `ScriptRunner.init(...empty); state.lastInt = request.lastInt` | ✗ **the bug** — Stage 2 scope. |

The four other `last_int`-reading content scripts (imp/draynor/salarin/nezikchened — `Content/scripts/.../*.rs2`) all dispatch through `processQueue` and are unblocked by the same fix.

## §4 — Architecture (Approach B selected)

| File | Change | Δ LOC |
|---|---|---|
| `pkg/script/queue.go:42-46` | `NpcQueueRequest.IntArg` → `LastInt`. Doc-update referencing TS `NpcQueueRequest.ts:17`. | rename + comment |
| `pkg/script/active.go:700-705` | `EnqueueScriptForTrigger(trigger, delay int, intArg int)` → `lastIntArg int` param rename. Interface doc-update. | rename + comment |
| `pkg/script/handlers_npc.go:339-355` | `handleNpcQueue` local var `arg` → `lastIntArg`; struct construction at `EnqueueScriptForTrigger`; doc clarifies the popped value sets state.LastInt at fire time, not a positional script arg. | rename + comment |
| `modules/world/npc.go:328-338` | `EnqueueScriptForTrigger` impl — param + struct-literal rename; doc-update. | rename + comment |
| `modules/world/npc_script.go:469-493` | `processNpcQueue` rewires the dispatch: replace `s.runNpcScript(sf, n, nil, []int{intArg}, nil)` with: build state via `state := s.buildNpcScriptState(sf, n, nil, nil, nil)`, then `state.LastInt = req.LastInt`, then `s.resumeOrFinishNpc(state, n)`. Doc with TS Npc.ts:554-555 verbatim reference. | ~+5 / -1 LOC |
| `modules/world/npc_test.go` | Update `NpcQueueRequest{… IntArg: 42}` literals at `:481` and `:546` → `LastInt: 42`. | rename only |
| `modules/world/npc_event_queue_test.go:60` | Update `NpcQueueRequest{Trigger: …, Delay: 5, IntArg: 0}` → `LastInt: 0`. | rename only |
| `modules/world/npc_script_test.go:189-190` | Update `req.IntArg` read + assertion message → `req.LastInt`. | rename only |
| `pkg/script/handlers_npc_test.go` | Verify mock-capture path (existing tests use mockNpc.EnqueueScriptForTrigger capture, not direct struct-literal); update mock field names if any. | rename verification |
| `modules/world/npc_script_test.go` (new test) | `TestProcessNpcQueue_SetsStateLastInt` — register a one-op script `OpLastInt; OpReturn` (or via helper that captures the pushed value); enqueue with `LastInt=42`; assert observable state-side capture = 42. | ~+25 LOC test |

**Total production-code Δ: ~10 LOC + identifier renames (mechanically correlated).**

## §5 — Tracked deviations

- **DEVIATION-NAI-123-D1:** goscape `NpcQueueRequest` retains a single `LastInt` field rather than TS's split `args: ScriptArgument[]` + `lastInt: number`. Rationale: TS `args` is always `[]` at the only enqueue site (`Npc.ts:242`). Field is dead in TS. Retire condition: a future content-port surfaces an ai_queue dispatch with positional script args.

## §6 — Cadence & verification

- **Stage 1 collapsed** per `bundle0_short_circuits_stage1_audit`. No audit subagent dispatched (ref. `audit_subagent_fabrication`: controller did the TS-source reads directly).
- **Stage 2** dispatched as subagent-driven-development. Single bundle. One Sonnet implementer covering all renames + the `processNpcQueue` rewire + the new test (mechanically correlated; splitting would only add review overhead).
- **Controller pre-flight** per `controller_preflight`: re-grep all `IntArg` references in pkg/script + modules/world before implementer dispatch; re-Read `pkg/script/runner.go:Init`, `pkg/script/state.go:LastInt`, and `modules/world/npc_script.go:processNpcQueue` at HEAD.
- **Post-commit verification** per `verify_implementer_claims`: independent fresh `go test ./...` + `go vet ./...` + `go build ./...`. Watch for stale IDE diagnostics on the interface rename (per the three-protocol catch).
- **Reviewer:** Sonnet code-reviewer subagent over the implementer commit per `superpowers_code_reviewer_model`. Reviewer-fix sub-commit if any landed.
- **Worktree handoff** per `feedback_subagent_wt_path`: post-merge `git status` on main; stash strays.

## §7 — Smoke handoff

User-launched per `smoke_test_server_handoff`. Server binary on host; client = `LostCityRS/Client-Java`.

**Test:** Tutorial Island, fresh char, attack giant rat with bronze dagger, observe ≥10 hits.

**Success — PRIMARY:**
- At least one hit shows a non-zero red hitsplat.
- XP/hit remains in the same band as NAI-122 close (~50-100 XP/hit), confirming the fix didn't regress the V-PARTIAL path.

**Possible adjacent residuals (route per `cascade_theory_smoke_binding` / `smoke_unchanged_means_multiple_blockers`):**
- Damage values feel off-distribution (max-hit formula divergence) → NAI-124 candidate.
- NAI-121 residual #2 (single-attacker contention "Someone else is fighting that") — still queued; may or may not be cleared by NAI-123 if it depended on the same `last_int`-via-queue chain. Independent of NAI-123 fix shape.
- NAI-121 residual #3 (NPC non-retaliation / AI_HUNT or AI_ATTACK) — still queued; separate engine subsystem.

## §8 — Pattern memories applied

- `bundle0_short_circuits_stage1_audit` — Bundle 0 controller-direct TS read produced a binding line-level diff (Npc.ts:554-555 ↔ npc_script.go:490). Stage 1 audit subagent skipped.
- `audit_subagent_fabrication` — controller did the TS read directly; no subagent fabrication risk surface.
- `controller_preflight` — file paths, line numbers, helper signatures, struct layouts, test-fixture sites all verified at HEAD before implementer dispatch.
- `verify_implementer_claims` — fresh test/vet/build cycle post-commit; ignore stale IDE diagnostics on interface rename.
- `superpowers_code_reviewer_model` — Sonnet (not Opus) for code-reviewer subagent.
- `feedback_subagent_wt_path` — post-merge `git status` on main.
- `cascade_theory_smoke_binding` — PRIMARY closes on smoke-bind even if NAI-121 residuals #2/#3 remain.
- `dispatch_correct_reach_blocked` — engine-side dispatch fix; outcome-binding via smoke.
- `compressed_cadence` — DOES NOT apply: total prod Δ ~10 LOC fits the bound, but field-rename ripple touches 8+ files; subagent-driven-development per user default.
- `dead_api_polish` — N/A this sub-spec.
- `close_commit_memory_trailer` — applied on close commit.

## §9 — Cross-references

- TS source verbatim: `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:241-245, 538-560`; `NpcQueueRequest.ts:1-25`; `script/ScriptState.ts:129`.
- Goscape divergence anchor: `modules/world/npc_script.go:469-493`.
- NAI-122 close commit: `9ebd2ed`. NAI-122 close memo: `nai_followups.md:6174-6229`.
- NAI-26 (variadic queue ops parallel-slice convention): does NOT apply here — fixed-arg ai_queue path is single-int.
