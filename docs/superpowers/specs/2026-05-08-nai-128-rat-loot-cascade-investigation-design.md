# NAI-128 — Rat death-loot cascade investigation (item 1 of NAI-127's 4 follow-ups)

**Status:** spec — draft 1
**Date:** 2026-05-08
**Predecessor:** NAI-127 close (`27090aa`); investigates item 2 of NAI-127's smoke matrix (rat death-loot drop FAIL with no new unhandled-opcode WARN).
**Cadence:** investigation+fix sub-spec per `investigation_subspec_cadence` — Stage 1 risk-weighted-short-circuit binding via synthetic Go probe; Stage 2 fix in-scope if ≤~50 LOC, else route to NAI-129; Stage 3 conditional smoke at close.
**Tech stack:** Go 1.26+.

## §0 — One-line summary

Bind the rat-death-loot blocker by walking the full death-loot cascade chain end-to-end through a synthetic Go test that drives `[label,player_melee_attack]` against a giant-rat NPC fixture and asserts each link (NPC_HEROPOINTS credit → ai_queue2 dispatch → npc_default_damage HP decrement → ai_queue3 dispatch → npc_findhero → obj_add ground-obj). Static disasm refutes NAI-127's unhandled-opcode hypothesis; the bug is a semantic gap in handler / dispatch / config.

## §1 — Symptom and binding-hypothesis refutation

**Smoke (NAI-127 close, `27090aa`, 2026-05-08):**
Fresh char + bronze dagger vs Tutorial Island giant rat. Rat dies (NAI-127 PRIMARY item 1 met — NPC_FINDHERO WARN silenced). **Item 2 of the smoke matrix (rat death-loot drop) FAILED with no new unhandled-opcode WARN.**

NAI-127 spec §6 risk-note named the cascade-suspect:

> "If `player_melee_attack` is itself blocked by another unhandled opcode upstream of `npc_heropoints($damage_capped)`, item 2 will FAIL with 'rat dies, no loot' — symptom indistinguishable from NPC_FINDHERO returning 0 in goscape's empty-ledger case."

**Static disasm at HEAD `27090aa`** (per `disasm_reframes_inferred_binding`):

A one-shot `pkg/script` test walked the call graph from `[label,player_melee_attack]` plus every reachable death/retaliate/tutorial-side root (`[ai_queue2,_]`, `[ai_queue3,_]`, `[ai_queue3,newbiegiantrat]`, `[ai_opplayer2,_]`, `[ai_opplayer2,newbiegiantrat]`, `[opnpc2,newbiegiantrat]`, `[apnpc2,newbiegiantrat]`, `[queue,combat_damage_player]`, `[queue,playerhit_n_retaliate]`, `[queue,set_rat_kill]`), following every `OpGosubWithParams` edge transitively via `Provider.GetByID`. Reached **148 scripts** total. Cross-checked the union of opcodes used against the global `pkg/script.handlers` map.

```
=== Reached scripts (148) ===
[ai_opplayer2,_], [ai_opplayer2,newbiegiantrat], [ai_queue2,_],
[ai_queue3,_], [ai_queue3,newbiegiantrat], [apnpc2,newbiegiantrat],
[label,player_melee_attack], [opnpc2,newbiegiantrat],
[proc,chatnpc], [proc,chatnpc_page], ... [proc,npc_default_damage],
[proc,npc_default_death], [proc,npc_death], ... (full list elided)

=== Unhandled opcodes used (0) ===
```

**Result: 0 unhandled opcodes across the full transitive call graph.** NAI-127 §6's cascade hypothesis is refuted by static disasm. The blocker is NOT an unhandled-opcode anywhere in the death-loot dispatch chain. Reframe NAI-128 as a semantic-bug investigation.

The probe `pkg/script/nai128_probe_test.go` was authored at brainstorm-time and removed (data captured in this spec). Stage 1's permanent test, written in `modules/world`, supersedes it.

## §2 — Scope

**In scope.** Item 1 of the 4 NAI-127 smoke follow-ups: rat death-loot drop on Tutorial Island giant rat. End-to-end cascade from `player_melee_attack` to `obj_add(npc_coord, ...)` zone-receiver visibility.

**Out of scope (explicit routes — see §9).**
- Bronze arrow 25-stack inventory expansion → NAI-129 candidate
- Cannot range across fence (line-of-walk / projectile rules) → NAI-130 candidate
- Arrows not consumed on ranged attacks → NAI-131 candidate

Each gets a one-line carry-forward in `nai_followups.md` at NAI-128 close.

## §3 — Death-loot cascade chain (target)

```
player_melee_attack  (player_melee.rs2:31, 47)
  ├─ npc_heropoints($damage_capped)        // NAI-120 + NAI-127 ledger credit
  └─ npc_queue(2, $damage, 0)               // schedules ai_queue2 on rat (T+0)

[ai_queue2,_]  (npc_combat.rs2:2, generic wildcard)
  └─ ~npc_default_damage(last_int)
        └─ npc_damage(^hitmark_damage, $damage)  // HP decrement
        └─ if (npc_stat(hitpoints) = 0)
                npc_queue(3, 0, 0)               // schedules ai_queue3 on rat

[ai_queue3,newbiegiantrat]  (tut_giant_rat.rs2:4, specific match)
  ├─ gosub(npc_death)                            // npc_walk + setmode + arrivedelay + anim + delay + npc_del
  └─ if (npc_findhero = ^true)                   // ledger read
        ├─ obj_add(npc_coord, npc_param(death_drop), 1, ^lootdrop_duration)
        ├─ obj_add(npc_coord, raw_rat_meat, 1, ^lootdrop_duration)
        └─ if (tutorial-state) queue(set_rat_kill, 0, 0)
```

**Engine-tick binding:** `processNpcQueue(n)` (`modules/world/npc_script.go:497`) drains the per-NPC queue once per tick; each entry resolves via `scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)` → 3-tier specific → category → global. Verified at brainstorm: lookup chain works for `[ai_queue2,_]` (global wildcard) and `[ai_queue3,newbiegiantrat]` (specific) at HEAD.

## §4 — Stage 1: synthetic Go probe (binding)

A new `modules/world/nai128_rat_loot_test.go` with a single test `TestNAI128_RatLootCascade` that drives the full chain through real `pkg/script.Execute` + real `processNpcQueue`, without engaging the world tick loop or network layer.

### §4.1 — Probe construction

1. Build a `Server` test fixture using existing test helpers (model after `nai101_fountain_test.go` — locate the cleanest existing world-test scaffold during plan-author).
2. Load real `pkg/script.Provider` from `data/pack/server` (script.dat / script.idx).
3. Load real NPC `objtype.NpcType` config so `npc_param(death_drop)` resolves; load real `objtype.ObjType` so `raw_rat_meat` resolves.
4. Spawn a player at a known coord; spawn a `newbiegiantrat` NPC adjacent.
5. Force the rat HP to 1 (via direct field write or test-only setter — pre-flight in plan-author to confirm shape).
6. Stub `~player_npc_hit_roll` to deterministically return true (script-level shim or seed RNG so `randominc($attack_roll) > randominc($defence_roll)`). Plan-author picks the cleanest mechanism after grepping pkg/script for an RNG-seam.

### §4.2 — Probe execution and assertions

Drive in three phases, asserting at each:

**Phase A — invoke `[label,player_melee_attack]`:**
1. Build a ScriptState with `Self = player`, `ActiveNpc = rat`, run via `script.Execute`.
2. Assert: `rat.heroPoints` ledger contains `(player.UID(), $damage_capped)` post-execution.
3. Assert: `rat.queue` contains one `NpcQueueRequest{Trigger: TriggerAiQueue2, Delay: 0, LastInt: $damage_capped}`.
4. Assert: `player.action_delay` (varp 58) is updated.
5. **Bind candidate A if assertion 2 fails** (NPC_HEROPOINTS not crediting the ledger).

**Phase B — process ai_queue2:**
6. Call `s.processNpcQueue(rat)` once.
7. Assert: rat HP is now 0 (npc_damage decremented; clamped to zero — rat is dead). Note: the value passed to ai_queue2 is the raw `$damage` (player_melee.rs2:47 — `npc_queue(2, $damage, 0)`), NOT `$damage_capped`. With rat HP=1 + any positive damage, HP reaches 0.
8. Assert: `rat.queue` now contains one `NpcQueueRequest{Trigger: TriggerAiQueue3, Delay: 0, LastInt: 0}` (assuming Phase B and Phase C don't collapse — see §4.4 pre-flight).
9. **Bind candidate B if assertion 7 fails** (NPC_DAMAGE handler bug or `[ai_queue2,_]` not dispatching).
10. **Bind candidate C if assertion 8 fails** (npc_default_damage's HP-zero branch not reached, or NPC_QUEUE for queueId=3 fails).

**Phase C — process ai_queue3 + obj_add:**
11. Call `s.processNpcQueue(rat)` once.
12. Assert: ai_queue3 resolved to `[ai_queue3,newbiegiantrat]` (specific) — surface via probe instrumentation hook in plan-author OR re-run with a temporary log.
13. Assert: rat-coord zone has two ground objs registered: one with `objtype = npc_param(death_drop)` and one with `objtype = raw_rat_meat`. Pin via the existing zone-receiver inspection seam in `modules/world` (plan-author identifies the canonical test API; `obj_add` writes to a zone-bound ground-obj receiver).
14. Assert: rat is marked for removal (`npc_del` end-of-`npc_death` proc fired).
15. **Bind candidate D if assertion 12 fails** (specific-trigger dispatch broken — falls back to `[ai_queue3,_]` generic instead).
16. **Bind candidate E if assertion 13 fails** despite ai_queue3 dispatching (split: candidate E1 = `npc_param(death_drop)` returning null/-1 due to config-loader gap; candidate E2 = OBJ_ADD handler not registering the ground-obj; candidate E3 = NPC_FINDHERO returning 0 because the ledger read path is broken even though the ledger was credited in Phase A).

### §4.3 — Risk-weighted short-circuit

Probe asserts in cascade order so the first failure binds the highest-priority candidate. Per `investigation_subspec_cadence`: bind on the first failing assertion; do NOT continue running downstream assertions on a known-failing harness.

### §4.4 — Probe pre-flight checklist for plan-author

Plan-author MUST verify these premises against HEAD before dispatch (per `controller_preflight`, `spec_followup_tracker_freshness`):
- World-test fixture pattern: read `nai101_fountain_test.go` and 1–2 other recent investigation-test files to identify the canonical fixture-construction idiom.
- ObjType / NpcType cache loaders: confirm `data/pack/server` provides registry data for `newbiegiantrat`, `raw_rat_meat`, and `death_drop` param.
- **`processNpcQueue` re-entrancy / phase-collapse risk:** reading `npc_script.go:497-526`, the loop iterates while `i < len(n.queue)`, removing fired entries in-place. ai_queue2's handler enqueues ai_queue3 with Delay=0 mid-loop (via `EnqueueScriptForTrigger` appending to `n.queue`). On the next loop iteration, `req.Delay--` drives 0 → -1, the gate `Delay > 0` is false, and ai_queue3 fires within the SAME `processNpcQueue` call. **This means Phase B and Phase C may collapse into a single call.** Plan-author MUST: (a) verify this behavior empirically on HEAD by inspecting queue length before/after a single call; (b) if collapse confirmed, restructure the probe to one `processNpcQueue` call and assert end-state of all post-Phase-A invariants; (c) if collapse not confirmed (e.g., goscape-side adjustment to add 1 to Delay on enqueue, mirroring TS), keep the two-call structure. Compare goscape's `EnqueueScriptForTrigger` with `Engine-TS/.../Npc.ts` queue semantics to decide whether the collapse is TS-faithful or a divergence — if divergence, file as DEVIATION-NAI-128-Dn or fix in-scope per §5 scope-gate.
- Hit-roll determinism seam: grep `pkg/script` for the RNG used by `randominc`; identify the test-injection point.
- Ledger inspection: `Npc.heroPoints` field exposure for test reads (added in NAI-120 / NAI-127); confirm a public accessor exists OR plan a test-only export.

## §5 — Stage 2: fix (TDD against probe)

Once Stage 1 binds a specific candidate, author Stage 2 fix as TDD against `TestNAI128_RatLootCascade` (turn the binding-failed assertion into the red baseline).

**Scope-gate per NAI-127 close routing convention:**
- Fix ≤~50 LOC (handler semantic fix, missing config field, single-line dispatch wiring) → in-scope under NAI-128.
- Fix >~50 LOC (foundational gap: e.g., NPC config-loader rewrite, ground-obj zone-receiver missing) → spec the binding evidence in NAI-128 close, route the fix to NAI-129 with the bound candidate name + evidence already in hand. Mirrors NAI-114 cascade routing where Stage 5's foundational dependency was carried forward to NAI-115.

## §6 — Stage 3: conditional smoke at close

Decision tree at NAI-128 close (per `cascade_theory_smoke_binding` companion):

| Stage 1 binding layer | Stage 2 outcome | Close requires |
|----|----|----|
| Handler semantic bug (NPC_HEROPOINTS / NPC_FINDHERO / NPC_DAMAGE / OBJ_ADD) | Fix landed; probe green | **Probe-green close** (no smoke). Handler tests + cascade probe are sufficient binding. |
| Config-loader gap (death_drop param, raw_rat_meat ObjType) | Fix landed; probe green | **Smoke requested** at close. Tutorial Island fresh-char + bronze-dagger vs giant rat: rat dies, loot visible on ground. |
| Dispatch-layer (specific vs wildcard fallback in `GetByTrigger` for ai_queue3) | Fix landed; probe green | **Smoke requested** at close (broader content surface implication). |
| Foundational gap routed to NAI-129 | NAI-128 fix not landed | **Probe-binding-only close**. Smoke is NAI-129's burden. |

## §7 — Smoke decision tree (at close)

```
Probe green AND fix in handler layer       → close (no smoke)
Probe green AND fix in config/dispatch     → request smoke
                                              ├─ smoke green → close
                                              └─ smoke fail  → NAI-129 (record symptom shape change)
Probe red AND fix routed to NAI-129         → close on Stage 1 binding evidence + carry-forward
Probe doesn't bind (false-pass risk)       → escalate to Stage 1.5 (instrumentation-smoke per investigation_subspec_cadence)
```

## §8 — Deviations and risks

**R1: Synthetic-probe-vs-engine-tick mismatch.** Probe drives `processNpcQueue` directly and bypasses the full `Server.Tick()` orchestration (player queues, world clock, zone-update batching). If the bug is in tick-ordering or in some interaction between player.Tick and processNpcQueue across ticks, the probe will false-pass. **Mitigation:** mark probe as binding for handler/config-layer bugs only; if all assertions pass but Java-client smoke still shows no loot, escalate to Stage 1.5 instrumentation-smoke (the same instrumentation the probe asserted, rewritten as `slog.Info` lines in the real handler/dispatch sites). Per `smoke_unchanged_means_multiple_blockers`, an unchanged smoke shape after probe-green is a signal to brainstorm a second blocker, not to declare victory.

**R2: Tutorial-state coupling.** `[ai_queue3,newbiegiantrat]` line 9–12 has `if (%tutorial = ^newbie_combat_instructor_during_attacking_melee | ...)` that queues `set_rat_kill`. Probe should NOT depend on this branch firing — it's tutorial-progression, not loot-drop. Pin set_rat_kill firing as an OPTIONAL observation, not a binding. Per `ts_asymmetry_dual_pin`, also assert that the loot drops REGARDLESS of tutorial state (a fresh test fixture with `%tutorial=0` should still drop loot — the loot-drop and tutorial-progression branches are independent within the same `if (npc_findhero=^true)` block).

**R3: ai_queue specific-vs-generic match invisibility.** `processNpcQueue` doesn't log which trigger-tier resolved. Phase C assertion 12 (specific match) is hard to pin without temporary instrumentation. **Mitigation:** plan-author either (a) adds a permanent `lastResolvedTrigger` test-hook field on Npc (small, retire-after-NAI-128 candidate), or (b) verifies via the SIDE-EFFECTS unique to the specific match (the second `obj_add` for `raw_rat_meat`, which is in the newbiegiantrat-specific block but NOT in `[ai_queue3,_]` `[proc,npc_default_death]`). Option (b) is the lower-impact choice; assertion 12 is then implicit from assertion 13's "two ground objs, second = raw_rat_meat".

**R4: NAI-127 ledger persistence.** NPC_HEROPOINTS handler at `pkg/script/handlers_npc.go:1078-1090` calls `s.ActiveNpc.AddHeroPoints(s.Self.UID(), amount)`. The Npc-side AddHeroPoints implementation came from NAI-120 Bundle 2D. Probe Phase A assertion 2 directly tests this; if the handler is fine but the underlying `Npc.heroPoints` field doesn't actually persist (e.g., it's a value-receiver method on a copy), this binds candidate A. NAI-127 added `Player.heroPoints` field but uses player-side credit only for NPC-vs-player damage; for player-vs-NPC the credit is on `Npc.heroPoints` which is the long-standing NAI-120 path. Verified at brainstorm via grep — handler dispatches to `s.ActiveNpc.AddHeroPoints`, not `s.Self.AddHeroPoints`.

**R5: TS-fidelity gate (per `true_to_ts_gate`).** Any divergence found and fixed must be tracked as DEVIATION-NAI-128-Dn with rationale + retirement criterion. None expected unless config-side data is reshaped.

## §9 — Carry-forwards (out-of-scope, route to NAI-129+)

To be added to `nai_followups.md` at NAI-128 close:

```
## From NAI-128 (2026-05-08)

- (NAI-129 candidate) Bronze arrow 25-stack fills 25 inventory slots
  instead of stacking. NAI-127 smoke evidence: Combat Instructor reward
  in Tutorial Island. Likely inv_add stackable-flag handling or ObjType
  stackable-config gap; unclear whether content-side or engine-side.

- (NAI-130 candidate) Cannot range targets across fence in Tutorial
  ranged section. NAI-127 smoke evidence: line-of-walk / projectile
  reach gate. Likely pkg/pathfinder/reach LineValidator branch or
  PathingEntity.canAttack ranged predicate.

- (NAI-131 candidate) Arrows not consumed on ranged attacks. NAI-127
  smoke evidence: bow + arrows equipped, arrows shot, arrow inventory
  count unchanged. Likely missile lifecycle (queue,combat_attack_ranged
  or similar) or inv-decrement on shoot.
```

## §10 — Done criteria

- [ ] Stage 1: `TestNAI128_RatLootCascade` lands. First failing assertion binds Stage 2 candidate A/B/C/D/E with named evidence.
- [ ] Stage 2: fix landed (in-scope ≤50 LOC) OR Stage 1 binding evidence carried to NAI-129 close-doc.
- [ ] Stage 3: smoke decision tree applied per §7; close commit body records the path taken.
- [ ] `nai_followups.md` § "From NAI-128" appended with the 3 ranged-cluster carry-forwards.
- [ ] Memory entries: at least one new entry capturing whatever lesson NAI-128 surfaces (cascade-attribution refutation by static disasm is a meaningful finding regardless of where the fix lands; consider an entry on "static-disasm Stage-0 to refute spec-author cascade hypotheses").
- [ ] Close commit trailer `Closes memory:` per `close_commit_memory_trailer`.
