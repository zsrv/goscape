# NAI-124 — Damage magnitude "far too high" investigation

**Status:** spec — draft 1
**Date:** 2026-05-08
**Predecessor:** NAI-123 close (`5687f4e`); residual #1 carry-forward (NAI-123 §7).
**Cadence:** investigation_subspec_cadence — Stage 1 risk-weighted controller-direct audit → Stage 2 fix → user-launched smoke. Second documented instance of this cadence post-NAI-31.
**Tech stack:** Go 1.26+.

## §0 — One-line summary

Bronze-dagger-vs-3-HP-giant-rat smoke shows damage = 3 every hit, indicating `randominc(min($maxhit, npc_param(max_dealt)))` consistently outputs ≥ 3 and `$damage_capped = min($damage, npc_stat(hitpoints))` clamps to NPC HP. Stage 1 enumerates contributing sign / type / aggregation divergences in the param + varp + arithmetic chain; Stage 2 fixes; smoke binds on a TS-faithful damage distribution.

## §1 — Symptom and binding evidence

**Smoke (NAI-123 close, `5687f4e`, 2026-05-07):**
- Tutorial Island fresh char + bronze dagger vs giant rat (3 HP).
- Every hitsplat shows **3** damage (red).
- XP/hit remains in NAI-122 band (~50-100, confirming V-PARTIAL + ai_queue paths still bind from NAI-122/NAI-123 fixes).

**Why "always 3" implies pre-cap input is large:**

Per `Content/scripts/skill_combat/scripts/player/player_melee.rs2:19-32`:

```
def_int $damage = 0;
if (~player_npc_hit_roll(%damagetype) = true) {
    def_int $maxhit = %com_maxhit;
    ...
    $damage = randominc(min($maxhit, npc_param(max_dealt)));
    def_int $damage_capped = min($damage, npc_stat(hitpoints));
    ~give_combat_experience(%damagestyle, $damage_capped, %npc_combat_xp_multiplier);
    npc_heropoints($damage_capped);
}
...
npc_queue(2, $damage, 0);
```

The `$damage_capped = min($damage, npc_stat(hitpoints))` cap clamps `$damage` to 3 (rat HP). For damage to *consistently* land at 3 across many hits, `randominc(N)` must very rarely return < 3 — i.e. `N` is at least in the tens, more likely hundreds-plus. So `min($maxhit, npc_param(max_dealt))` is huge; one or both of the operands is reading an unintended large value.

**TS-expected band (level 1 strength + +4 strength bonus + accurate style, no prayer):**

Per `Content/scripts/skill_combat/scripts/combat.rs2:7-12`:

```
[proc,combat_maxhit](int $combat_stat)(int)
return(calc(($combat_stat + 320) / 640));

[proc,combat_stat](int $effective_stat, int $bonus)(int)
return(calc($effective_stat * ($bonus + 64)));
```

- `$effective_strength` = `scale(100, 100, 1) + 8 + 0` = **9** (level 1, no prayer, accurate-style strength bonus = 0).
- `$strengthbonus` = **4** (bronze dagger sole contributor; tutorial starting clothes have 0 strength bonus).
- `$melee_strength = combat_stat(9, 4)` = `9 * (4 + 64)` = **612**.
- `%com_maxhit = combat_maxhit(612)` = `(612 + 320) / 640` = `932 / 640` = **1**.

`min(1, max_dealt) → at-most 1` → `randominc(0..1)` → 50/50 0-or-1 distribution. Zero-dominant in long runs since the accuracy gate (`~player_npc_hit_roll`) also intermittently fails.

So the divergence collapses to: **goscape's pre-cap damage input is much higher than 1.**

## §2 — Stage-1 surface inventory (risk-ranked)

Each surface has a verdict to land in the Stage-1 findings doc (DIVERGENT / ALIGNED / N/A) with TS-source line refs.

### S1 — paramLookup default branch sign-extension (highest)

**Location:** `pkg/script/handlers_config.go:51`, `pkg/objtype/paramtype.go:111`, `:183`.

**Hypothesis:** Mirror of the NAI-122 set-branch sign-extension fix, on the *unset-param* default path.

**Evidence already in tree:**
- `pkg/objtype/paramtype.go:111` — `DefaultInt uint32`.
- `pkg/objtype/paramtype.go:183` — `//DefaultInt: -1, // this is -1 in js, default 0 here` (admits the goscape default-default differs from TS's -1).
- `pkg/script/handlers_config.go:51` — `s.PushInt(int(pt.DefaultInt))`. `int(uint32)` zero-extends on 64-bit Go; no `int32` cast.
- NAI-122 close commit `aabdb65` body explicitly flags the DecodeParams uint32-storage boundary as a future audit ("Future audit should consider fixing at the boundary (DecodeParams stores int32 instead of uint32)").

**Probe:**
- Read TS `LostCityRS/Engine-TS/src/engine/script/ParamHelper.ts:getIntParam` default branch.
- Read TS `src/engine/config/ParamType.ts` for `defaultInt` field default.
- Decode `param.dat` for `max_dealt` ParamType — does it set DefaultInt explicitly? If yes, what wire bytes? If `0xFFFFFFFF`, the goscape read returns `4294967295` while TS returns `-1`.
- Cross-check: does *any* other ParamType referenced by player_combat_stat.rs2 / player_melee.rs2 / equip.rs2 also rely on the default branch?

**Why "always 3" rather than "always max_dealt-bounded random":**
- TS unset-default for max_dealt = -1; `min(1, -1) = -1`; `randominc(-1) → 0`; damage = 0.
- Goscape default = 0 (from `DefaultInt uint32` zero-init); `min(1, 0) = 0`; `randominc(0) = 0`; damage = 0.
- But IF `max_dealt`'s ParamType *encodes* a wire-default of `0xFFFFFFFF` (intended as -1), THEN goscape returns `4294967295`; `min(1, 4294967295) = 1`; `randominc(1) → 0/1`; damage = 0/1 (not 3).

S1 alone cannot explain "always 3." It's a real bug to fix, but it's not the sole contributor. Stage 1 must continue past S1 even if S1 binds.

### S2 — Remaining unsigned-int Params consumer sites NAI-122 may have missed

**Location:** any `Params[k].(uint32)` reader plus any sibling `Params` reader in `pkg/`/`modules/`.

**Hypothesis:** NAI-122 fixed three sites (`paramLookup`, `INV_TOTALPARAM`, `sumPlayerInvParam`). A fourth or fifth consumer that reads `Params[k]` and converts via plain `int(iv)` (no `int32` cast) would still buggy. Particularly: param-aggregation sites the NAI-122 grep may have missed.

**Probe:**
- `rg "Params\[" pkg/ modules/` at HEAD; cross-reference each site against NAI-122 `849e2fd` diff.
- For each non-NAI-122-touched site that does an int conversion, classify: still-unsigned (DIVERGENT) or doesn't matter (ALIGNED).

### S3 — `%com_maxhit` varp store/read sign discipline

**Location:** `pkg/script/handlers_vars.go:42-78`, `Player.Varp` / `Player.SetVarp` signatures.

**Hypothesis:** If the varp store path narrows to `int32` (line 75: `s.Self.SetVarp(id, int32(s.PopInt()))`) but the read path reads back as a wider-than-int32 type and `int(...)` zero-extends, %com_maxhit could blow.

**Probe:**
- Read `Player.Varp(id)` return type; if `int32`, `int(int32)` sign-extends correctly (ALIGNED). If `uint32` or wider, divergent.
- Read `Player.SetVarp` parameter type; cross-check round-trip.

### S4 — `Self.Stat(id)` return type

**Location:** `pkg/script/handlers_player.go:250` (`s.PushInt(s.Self.Stat(id))`); `Player.Stat` signature.

**Hypothesis:** Stat-level is encoded in a small unsigned-byte type (uint8 max = 255 valid, but if it's encoded as a signed byte misread, level could blow). If `Stat()` returns `int` directly, fine. If it returns `uint8` or `byte`, a non-int32 cast at the read site doesn't matter (no sign-bit issue for valid range 0-99). DIVERGENT path: the level itself is corrupted upstream.

**Probe:** read `Player.Stat` signature and its storage backing.

### S5 — TS-vs-goscape arithmetic int32-truncation discipline

**Location:** `pkg/script/handlers_number.go:84-150` (Multiply / Divide / Scale / AddPercent), `pkg/script/handlers.go:614-628` (Add / Sub).

**Hypothesis:** TS uses 32-bit Math.imul-style truncation in `OpMultiply` (goscape's `handleMultiply` already does, line 88). `OpAdd` / `OpScale` / `OpAddPercent` are non-truncating in goscape (Go int = int64). For combat-style arithmetic with very large operands, a TS-truncation that goscape misses (or vice-versa) could shift sign or magnitude.

**Probe:** for each opcode, read the TS handler and compare. Flag only if a plausible combat-formula chain exceeds 2^31.

### S6 — Strength-bonus aggregation chain end-to-end

**Location:** `Content/scripts/player/scripts/equip.rs2:195-236` (`equip_get_bonuses`), opcode-by-opcode through goscape handlers.

**Hypothesis:** Bronze dagger strengthbonus = +4 (positive), but other tutorial-starting worn items (helmet/body/shoes) might sum a value that, after all opcode-level conversions, blows up.

**Probe:** trace one full execution path (mock the 6 starting worn slots, run through `oc_param` + `calc` + `add` per slot) and pin the running sum at each step.

### Stage-1 deliverable

`investigation(nai-124): Stage 1 — <root-cause summary>` commit landing:
- `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md` — per-surface verdict (DIVERGENT / ALIGNED / N/A), TS-source line refs, refutation evidence for ALIGNED rows.
- §-final scope statement — "Stage 2 fixes S1 only" / "Stage 2 fixes S1+S3" / "Stage 2 ports missing TS check at <site>" / etc.

Per `bundle0_short_circuits_stage1_audit` and `audit_subagent_fabrication`, Stage 1 is controller-direct (no audit subagent). Per `controller_preflight`, every line ref re-greppable at HEAD before Stage-2 dispatch.

## §3 — Stage-2 fix shape (deferred to Stage-1 outcome)

**Most-likely shape (S1 binds):**

| File | Change | Δ LOC |
|---|---|---|
| `pkg/script/handlers_config.go:51` | `s.PushInt(int(pt.DefaultInt))` → `s.PushInt(int(int32(pt.DefaultInt)))`. Doc-comment cross-references NAI-122 set-branch fix at line 43. | +1 / -1 |
| `pkg/objtype/paramtype.go:178-185` | `NewParamType` initializes `DefaultInt: 0xFFFFFFFF` (uint32 wire encoding of -1) to match TS JS default. Retire the dangling `//DefaultInt: -1, // this is -1 in js, default 0 here` comment. | +1 / -1 |
| `pkg/script/handlers_config_test.go` (new test) | `TestParamLookup_DefaultBranch_SignExtended` — register a ParamType with `DefaultInt = 0xFFFFFFFC` (= -4); pop via paramLookup default; assert popped int = -4. Pre-fix: 4294967292. | +25 LOC |

**Test discipline:** TDD-style. Failing-pre-fix test pins the divergent value AND the expected post-fix value.

**Multi-contributor contingency:** if S2/S3/S4/S5/S6 also bind, group mechanically-correlated sites into one Stage-2 bundle; otherwise split. Per `enumerate_all_sites`, plan re-greps every contributor at plan-write and Stage-2 dispatch.

**Non-leaf root cause contingency:** if Stage 1 surfaces a missing TS check (rather than a sign-extension), Stage 2 ports the check; the architecture table above is replaced by the port shape.

## §4 — Tracked deviations

None inherited. New deviations (if any) land at Stage 2 close per `true_to_ts_gate`.

## §5 — Cadence & verification

- **Stage 1:** controller-direct probe per `bundle0_short_circuits_stage1_audit`; no audit subagent (audit_subagent_fabrication risk avoided). Single Stage-1 commit with findings doc rolled into Stage-2 dispatch.
- **Stage 2:** subagent-driven-development per `execution_mode_default`. Single bundle if root cause is single-surface; otherwise split-by-surface with explicit dispatch ordering.
  - Sonnet implementer per `superpowers_code_reviewer_model`; reviewer also Sonnet.
  - Per `controller_preflight`: re-grep + re-Read every Stage-1 line ref before implementer dispatch.
  - Per `verify_implementer_claims`: fresh `go test ./... && go vet ./... && go build ./...` post-commit; ignore stale IDE diagnostics.
  - Per `feedback_subagent_wt_path`: post-merge `git status` on main; stash strays.
- **Smoke:** user-launched per `smoke_test_server_handoff`.
- **Close commit:** memory trailer per `close_commit_memory_trailer`.

## §6 — Smoke handoff

User-launched per `smoke_test_server_handoff`. Server binary on host; client = `LostCityRS/Client-Java`.

**Test:** Tutorial Island, fresh char, attack giant rat with bronze dagger, observe ≥ 15 hits.

**Success — PRIMARY:**
- Damage distribution shifts from "3 every time" to a heavily-zero-weighted distribution. Specifically: ≥ 5 of 15 hits show **0** (blue hitsplat), and any non-zero hit is **0 or 1** (never ≥ 2 on a level-1 fresh char).
- This pins both: the cap is no longer disabled (max-hit reads ~1, not ~hundreds), and `randominc(maxhit=1)` produces ~50/50 0-vs-1 (the accuracy gate adds zeros on top).
- XP/hit remains in NAI-122/NAI-123 band (~50-100), confirming the V-PARTIAL + ai_queue paths still bind.

**Possible adjacent residuals (route per `cascade_theory_smoke_binding` / `smoke_unchanged_means_multiple_blockers`):**
- If post-fix damage is consistently 1 (no zeros), the accuracy gate (`~player_npc_hit_roll`) is effectively always true → separate sub-spec.
- If damage shows occasional 2s/3s after fix on a fresh char, level-up cascade (stat_advance scaling) is real → NAI-N+1.
- NAI-121 residuals #2/#3 still queued; independent of NAI-124 fix shape.

## §7 — Pattern memories applied

- `investigation_subspec_cadence` — Stage 1 risk-weighted-short-circuit audit → Stage 2 fix → smoke. Second documented instance post-NAI-31.
- `bundle0_short_circuits_stage1_audit` — controller-direct Stage-1 probe.
- `audit_subagent_fabrication` — controller does the TS-source reads.
- `controller_preflight` — re-grep + re-Read all Stage-1 line refs before Stage-2 dispatch.
- `verify_implementer_claims` — fresh test/vet/build cycle post-commit.
- `superpowers_code_reviewer_model` — Sonnet (not Opus) for code-reviewer subagent.
- `feedback_subagent_wt_path` — post-merge `git status` on main.
- `cascade_theory_smoke_binding` — PRIMARY closes on smoke-bind even if adjacent residuals appear.
- `smoke_unchanged_means_multiple_blockers` / `dispatch_correct_reach_blocked` — anticipate multi-contributor; Stage 1 risk-weighted enumeration is the defense.
- `nai_followups.md` — NAI-122 close commit body listed "fix at the DecodeParams boundary" as future audit; this sub-spec is that follow-up.
- `enumerate_all_sites` — plan re-greps every contributor at plan-write and Stage-2 dispatch.
- `close_commit_memory_trailer` — applied on close commit.

## §8 — Cross-references

- TS source: `LostCityRS/Engine-TS/src/engine/config/ParamType.ts` (defaultInt field default); `src/engine/script/ParamHelper.ts:getIntParam` (default branch).
- Goscape divergence anchors: `pkg/objtype/paramtype.go:111`, `:183`; `pkg/script/handlers_config.go:51`.
- NAI-122 close commit: `aabdb65`. NAI-122 sign-ext fix: `849e2fd`. NAI-123 close commit: `5687f4e` (smoke that surfaced this residual).
- Content: `Content/scripts/skill_combat/scripts/player/player_melee.rs2:19-47`; `Content/scripts/skill_combat/scripts/combat.rs2:7-12` (combat_maxhit, combat_stat); `Content/scripts/skill_combat/scripts/player/player_combat_stat.rs2` (~player_combat_stat proc); `Content/scripts/player/scripts/equip.rs2:195-236` (equip_get_bonuses).
