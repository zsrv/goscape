# NAI-124 — Stage 1 findings

**Date:** 2026-05-08
**Spec:** `docs/superpowers/specs/2026-05-08-nai-124-damage-magnitude-investigation-design.md` (`f8c3401`)
**Cadence:** controller-direct Stage 1 audit per `bundle0_short_circuits_stage1_audit`. No audit subagent dispatched.

## Per-surface verdicts

### S1 — paramLookup default branch sign-extension

**Verdict:** DIVERGENT (real bug; NOT the bronze-dagger-vs-rat contributor).

**TS reference:**
- `LostCityRS/Engine-TS/src/cache/config/ParamHelper.ts:18-24` — `getIntParam(id, holder, defaultValue)` returns the caller-provided `defaultValue` if the param is not set. All call sites (NpcOps:139, ObjOps:102, LocOps:121, ObjConfigOps:23, LocConfigOps:23, NpcConfigOps, StructOps:17, ObjConfigOps:23, Player.ts:1684) pass `paramType.defaultInt` as the fallback.
- `LostCityRS/Engine-TS/src/cache/config/ParamType.ts:62` — `defaultInt = -1` (JS unset default; signed Number).
- `LostCityRS/Engine-TS/src/cache/config/ParamType.ts:70` — `decode(code 2)` reads `dat.g4s()` (signed 4 bytes).

**Goscape divergence:**
- `pkg/objtype/paramtype.go:111` — `DefaultInt uint32` (unsigned storage).
- `pkg/objtype/paramtype.go:121` — `decode(code 2)` reads `dat.G4()` (unsigned 4 bytes).
- `pkg/objtype/paramtype.go:178-185` — `NewParamType` zero-inits `DefaultInt = 0`; the dangling comment at `:183` admits "this is -1 in js, default 0 here".
- `pkg/script/handlers_config.go:51` — `s.PushInt(int(pt.DefaultInt))` with NO `int32` cast. Set branch at `:43` IS sign-extended (`int(int32(iv))` per NAI-122 `92ca5c4`); default branch is not.

**Two divergent sub-cases:**
1. ParamType with no `code 2` ever set: TS = -1; goscape = 0.
2. ParamType with `code 2` encoding a negative value (wire bytes ≥ `0x80000000`): TS sign-extends to negative int; goscape's `int(uint32(0xFFFFFFFC))` = 4294967292.

**`max_dealt` ParamType wire default:** `default=1000` set explicitly at `Content/scripts/skill_combat/configs/combat.param:151-154` (`type=int; default=1000`). Wire bytes = `0x000003E8`. TS reads 1000; goscape reads 1000. Sign-ext is irrelevant for this positive value.

**`newbiegiantrat` NPC param.max_dealt:** unset at `Content/scripts/tutorial/configs/tutorial.npc:258-283`. Falls through to ParamType default (1000) on both TS and goscape paths.

**`strengthbonus` / sibling combat-bonus ParamTypes:** `default=0` per `Content/scripts/skill_combat/configs/combat.param:51-54` and surrounding (`stabbonus`/`slashbonus`/`crushbonus`/etc all have `default=0`). TS reads 0; goscape reads 0. Sign-ext irrelevant.

**Production effect on bronze-dagger-vs-rat smoke:** `npc_param(max_dealt)` = 1000 on both engines (positive default; sign-ext path not exercised). All combat-bonus default paths return 0 on both engines. **S1 is a real divergence but does NOT contribute to the "always 3" symptom.**

**Sole-contributor analysis:** S1 alone *cannot* produce damage = 3 every hit. Even with maximally-pathological S1 misread, `npc_param(max_dealt)` returns 1000 (the actual encoded default). The `min($maxhit, 1000)` cap at most produces a `randominc(1000)` distribution, but with TS `$maxhit ≈ 1` (level-1-strength expected band), the bronze-dagger-vs-rat smoke would still bind on a low-damage distribution. The "always 3" symptom requires `$maxhit` itself reading huge — not max_dealt.

### S2 — Remaining unsigned-int Params consumer sites

**Verdict:** DIVERGENT (real bug at 2 additional sites; same root cause as S1; NOT contributors to bronze-dagger-vs-rat smoke).

**Sites enumerated** via `rg "Params\[" pkg/ modules/ --type go | grep -v _test.go`:
- `pkg/script/handlers_config.go:31-43` (paramLookup set branch) — NAI-122 `92ca5c4` fixed via `int(int32(iv))`.
- `pkg/script/handlers_inv.go:247-252` (INV_TOTALPARAM set branch) — NAI-122 fixed.
- `modules/world/npc_hunt.go:289-293` (sumPlayerInvParam set branch) — NAI-122 fixed.

**Default-branch sites mirroring S1's divergence** (UNFIXED, same root cause as S1):
- `pkg/script/handlers_inv.go:256` — `total += int(pt.DefaultInt)` (no int32 cast).
- `modules/world/npc_hunt.go:297` — `total += int(pt.DefaultInt)` (no int32 cast).
- (S1's site `pkg/script/handlers_config.go:51` is the third instance.)

**Already-correct sibling default-branch sites** (use `int32` storage, not `uint32`):
- `pkg/script/handlers_config.go:109` — `s.PushInt(int(et.DefaultInt))` against `enumtype.go:17` `DefaultInt int32`. `int(int32)` sign-extends correctly. ALIGNED.

**Note:** The inconsistency is paramtype-specific: `pkg/objtype/paramtype.go:111` uses `uint32` while sibling `pkg/objtype/enumtype.go:17` and `pkg/objtype/dbtabletype.go:33` use `int32`. Stage 2 should converge paramtype to the sibling pattern.

**Production effect on bronze-dagger-vs-rat smoke:** No. All combat-bonus ParamTypes (`strengthbonus` / `stabattack` / etc.) have `default=0` per `Content/scripts/skill_combat/configs/combat.param`. Both default-branch sites return 0 regardless of the sign-ext bug. Confirmed not on the smoke path.

### S3 — `%com_maxhit` varp store/read sign discipline

**Verdict:** ALIGNED.

**Player.Varp signature:** `func (p *Player) Varp(id int) int32` at `modules/world/player_script.go:307-312`.
**Player.SetVarp signature:** `func (p *Player) SetVarp(id int, val int32)` at `modules/world/player_script.go:317-323`.
**Storage backing:** `p.varps []int32` (signed) — pinned by the slice element type at the read line.

**Round-trip analysis:**
- Write path (`handlers_vars.go:75`): `s.Self.SetVarp(id, int32(s.PopInt()))` — explicit narrowing cast. For values within int32 range (which encompasses all combat-formula outputs given level-1 stats per spec §1 arithmetic), this is bit-exact.
- Read path (`handlers_vars.go:51`): `s.PushInt(int(s.Self.Varp(id)))` — `int(int32)` sign-extends on 64-bit Go (negative int32 becomes negative int).

**TS reference:** TS varps use signed JS Number throughout; goscape's int32-pinned storage is the standard port shape. No divergence.

**Refutation evidence:** Even if `%com_maxhit` were stored as a large positive int32 (e.g. up-to 2^31-1), `int(int32)` preserves it exactly. No zero-extension misread possible at this layer.

### S4 — `Self.Stat()` return-type signedness

**Verdict:** ALIGNED.

**Player.Stat signature:** `func (p *Player) Stat(id int) int` at `modules/world/player_script.go:478-483` returning `int(p.levels[id])`.
**Storage:** `levels [21]uint8` at `modules/world/player.go:181`. Sibling `stats [21]int32` at `:180` holds XP-derived state.

**TS reference:** `Player.ts` uses `levels: number[]` (signed JS Number); the Stat-level read path returns the boosted level directly. Goscape's uint8 storage matches the wire encoding for stat-level updates (max 255 covers the 1-99 valid range plus boost headroom).

**Analysis:** For a fresh tutorial char, `p.levels[strengthSkillID]` = 1; `int(uint8(1))` = 1 exactly. uint8 has no sign-bit at the int-cast layer (range 0-255 is always non-negative). The S4 surface CANNOT produce a non-1 strength read for a level-1 fresh char via this path. ALIGNED.

**Refutation evidence:** any divergence in $effective_stat would require corruption upstream of `levels[id]` (e.g. in `player_combat_stat.rs2` proc using `stat_advance`), which is content-script logic and not part of this surface.

### S5 — TS-vs-goscape arithmetic int32-truncation discipline

**Verdict:** DIVERGENT — **ROOT CAUSE of bronze-dagger-vs-rat smoke.** Not the int32-truncation aspect the spec hypothesised; instead, **handleScale operands are swapped**.

**The bug** at `pkg/script/handlers_number.go:128-136`:

```go
func handleScale(s *ScriptState) error {
    c := s.PopInt()    // top of stack
    b := s.PopInt()
    a := s.PopInt()    // bottom
    if c == 0 {
        return errors.New("SCALE: division by zero")
    }
    s.PushInt(floorDiv(a*b, c))  // BUG: computes a*b/c
    return nil
}
```

**TS reference** at `LostCityRS/Engine-TS/src/engine/script/handlers/NumberOps.ts:124-127`:

```ts
[ScriptOpcode.SCALE]: state => {
    const [a, b, c] = state.popInts(3);    // a=bottom, b=mid, c=top
    state.pushInt((a * c) / b);             // computes a*c/b
},
```

**Variable mapping** is identical: in both engines, `a` is the bottom-of-stack arg, `c` is the top. But the formulas differ:
- TS: `(a * c) / b`.
- Goscape: `(a * b) / c`.

For runescript `scale(value, max, newMax)` (left-to-right push), this means:
- TS: `value * newMax / max` (standard runescript scale semantics).
- Goscape: `value * max / newMax` (operands swapped).

**Smoking-gun trace through bronze-dagger-vs-rat smoke:**

`Content/scripts/skill_combat/scripts/combat.rs2:4-5`:
```
[proc,combat_effective_stat](int $stat_level, int $prayerbonus)(int)
return(scale(max(100, $prayerbonus), 100, $stat_level));
```

For level-1 strength, no prayer (`check_strength_prayer` returns 100): `scale(100, 100, 1)`.
- TS: `(100 * 1) / 100 = 1`. Then `+ 8 + $strength_stylebonus(=0) = 9` → matches spec §1 expected `$effective_strength = 9`.
- Goscape: `(100 * 100) / 1 = 10000`. Then `+ 8 = 10008`.

Cascading downstream:
- `$melee_strength = combat_stat($effective_strength=10008, $strengthbonus=4) = 10008 * (4 + 64) = 680544`.
- `%com_maxhit = combat_maxhit(680544) = (680544 + 320) / 640 = 1063`.
- `min($maxhit=1063, npc_param(max_dealt)=1000) = 1000`.
- `randominc(1000)` produces uniform `0..1000` → P(damage ≥ 3) ≈ 99.7%.
- `min($damage, npc_stat(hitpoints)=3)` clamps to 3.
- **Predicted observation: damage = 3 nearly every hit.** Matches NAI-123 smoke residual exactly.

**Other arithmetic handlers** (audited; no other operand-swap bugs):
- `handleAdd` / `handleSub` (`handlers.go:614-628`): plain `a + b` / `a - b`. TS NumberOps.ts:8-18 same.
- `handleMultiply` (`handlers_number.go:84-90`): `int(int32(lhs) * int32(rhs))`. TS NumberOps.ts:20-24 plain `a * b`. **Goscape int32-truncates, TS does not** — but for level-1 combat operands (max 680544 in goscape's broken state, far below 2^31), no plausible overflow either way.
- `handleDivide` (`handlers_number.go:92-100`): `floorDiv(lhs, rhs)`. TS uses JS double `a / b` — different rounding semantics for negative dividends, but for combat formulas (always positive operands at this call site), they agree.
- `handleAddPercent` (`handlers_number.go:121-126`): `lhs + (lhs*rhs)/100`. TS NumberOps.ts:49-52: `((num * percent) / 100 + num) | 0` (int32-truncated). **Same algebra, different truncation.** No overflow on combat path.
- `handleScale`: BUG (this finding).

**Existing goscape test misframes the bug:** `pkg/script/handlers_number_test.go:46` — `{"scale 3/4 of 200", OpScale, []int{200, 3, 4}, 150}`. The test author intended `scale(200, 3, 4)` → "3/4 of 200 = 150" which means they thought args were `(value, num, denom)` computing `value*num/denom`. With the correct TS semantic `(value, max, newMax)` computing `value*newMax/max`, `scale(200, 3, 4) = 200*4/3 = 266`. The test pins the bug. Stage 2 must update this test case along with the fix.

**Plausibility & alignment with spec hypothesis:** The spec hypothesised int32-truncation divergence; the actual bug is operand-swap in SCALE. S5 was the right surface but the wrong sub-hypothesis. The arithmetic computed `680544` overflows the user-expected ~600 range by 3 orders of magnitude, fully explaining the "always 3" symptom without invoking sign-extension, varp round-trip, or stat misread.

### S6 — Strength-bonus aggregation chain end-to-end

**Verdict:** ALIGNED for the bronze-dagger-vs-rat smoke (S5 root cause renders S6 trace non-load-bearing).

**Tutorial starting worn slots:** any starter clothing in default tutorial flow plus the bronze dagger handed out by combat instructor. Searched `Content/scripts --include=*.obj` for `param=strengthbonus,` — non-zero strengthbonus is set explicitly only on weapons / select armour pieces. Tutorial-starting clothing (default tunic / trousers / etc.) does NOT set `strengthbonus`, so the default branch returns 0 for those slots.

**Per-slot strengthbonus on the smoke path:**
- Bronze dagger (right hand): `param=strengthbonus,3` per `Content/scripts/skill_combat/configs/melee/daggers.obj:30`. Set branch (NAI-122 `92ca5c4` sign-ext fixed): contributes +3.
- All other worn slots: `strengthbonus` not set; default branch (S1) returns 0 (`strengthbonus` ParamType `default=0`).

**Running sum:** `$strengthbonus = 0 + 3 = 3`.

**Combined with S5 root cause:**
- TS-correct trace: `$effective_strength = scale(100, 100, 1) + 8 + 0 = 9`. `$melee_strength = 9 * (3 + 64) = 603`. `%com_maxhit = (603 + 320) / 640 = 1`.
- Goscape-buggy trace: `$effective_strength = 10000 + 8 = 10008`. `$melee_strength = 10008 * 67 = 670536`. `%com_maxhit = 1047`. → randominc(min(1047, 1000)) = randominc(1000). → ~99.7% damage ≥ 3.

**S6 verdict refutation:** the 3-vs-4 bronze-dagger value (spec said +4, actual config says +3) does not change the analysis — the runaway came from $effective_strength, not $strengthbonus. S6 is not a contributor.

## §scope — Stage 2 dispatch shape

**Root cause refutes hypothesis-S1.** Actual root cause is **S5 — SCALE opcode operand-swap** at `pkg/script/handlers_number.go:135`. Goscape computes `floorDiv(a*b, c)` where TS computes `(a*c)/b`; equivalently, goscape's runescript `scale(value, max, newMax)` evaluates as `value*max/newMax` while TS evaluates as `value*newMax/max` (the standard runescript scale semantic).

**Stage 2 fix shape (single bundle, single Sonnet implementer):**

| File | Change | Δ LOC |
|---|---|---|
| `pkg/script/handlers_number.go:128-136` | `floorDiv(a*b, c)` → `floorDiv(a*c, b)`. Rename divide-by-zero check predicate from `c == 0` to `b == 0` (b is now the divisor). Optional: doc-comment cross-reference to TS `NumberOps.ts:124-127`. | +1 / -1 (+ optional comment) |
| `pkg/script/handlers_number_test.go:46` | Existing case `{"scale 3/4 of 200", OpScale, []int{200, 3, 4}, 150}` pins the bug. Update to TS-correct semantic: either rename + fix expected (`scale(200, 4, 3) → 200*3/4 = 150` would now produce correct expected; or update to `{"scale (100,100,1) → 1", OpScale, []int{100,100,1}, 1}` which directly pins the smoke trace). Add 1-2 additional pinning cases including the smoke-failure case `scale(100, 100, 1) = 1` and a divide-by-zero-on-`b`-is-now case. | +5 / -1 |

**S1 + S2 are real divergences but are NOT contributors to this smoke and SHOULD NOT be bundled into Stage 2.** The S1 default-branch sign-ext bug at `handlers_config.go:51` + sibling sites at `handlers_inv.go:256` and `npc_hunt.go:297` are real — but every relevant ParamType on the bronze-dagger-vs-rat path has a non-negative configured default (`max_dealt=1000`, `strengthbonus=0`, …). Routing them into NAI-125 as a follow-up sub-spec (or into a future NAI-N+M cleanup once smoke surfaces a negative-default ParamType in production) preserves smoke-binding clarity for NAI-124. Per `cascade_theory_smoke_binding`, Stage-2 close on S5 fix; if post-fix smoke surfaces an adjacent residual that DOES fall on the S1/S2 path, route to NAI-125 then.

**Stage 2 plan path:** `docs/superpowers/plans/2026-05-08-nai-124-stage2-scale-operand-swap.md` (to be written after `/clear` per `superpowers_clear_between_spec_and_impl`).

## Cross-references

- Spec: `docs/superpowers/specs/2026-05-08-nai-124-damage-magnitude-investigation-design.md` (`f8c3401`).
- Plan: `docs/superpowers/plans/2026-05-08-nai-124-damage-magnitude-stage1.md` (`4926399`).
- NAI-123 close: `b7c16b0` (smoke that surfaced this residual).
- NAI-122 set-branch sign-ext fix: `92ca5c4`.
- NAI-122 close: `2cdeeb9` (flagged DecodeParams uint32-storage as future audit).

**TS sources read:**
- `LostCityRS/Engine-TS/src/cache/config/ParamHelper.ts:9-41` (getIntParam / decodeParams).
- `LostCityRS/Engine-TS/src/cache/config/ParamType.ts:62, 70` (defaultInt = -1; decode g4s).
- `LostCityRS/Engine-TS/src/engine/script/handlers/NumberOps.ts:1-183` (full audit of arithmetic opcodes; SCALE at 124-127, ADD/SUB/MULTIPLY/DIVIDE at 8-30, ADDPERCENT at 49-52).
- `LostCityRS/Engine-TS/src/engine/script/ScriptState.ts:325-331` (popInts ordering).

**Goscape line refs verified at HEAD:**
- `pkg/script/handlers_config.go:43, 51`.
- `pkg/script/handlers_inv.go:247-256`.
- `modules/world/npc_hunt.go:289-297`.
- `pkg/script/handlers_vars.go:42-78`.
- `pkg/script/handlers_player.go:478-483` → `modules/world/player_script.go:478-483`.
- `pkg/script/handlers_number.go:84-150` (full audit; SCALE bug at 128-136).
- `pkg/script/handlers.go:614-628`.
- `pkg/objtype/paramtype.go:108-185` (DefaultInt uint32 / NewParamType zero-init).
- `pkg/objtype/enumtype.go:17` (sibling DefaultInt int32).
- `pkg/objtype/dbtabletype.go:33` (sibling DefaultInts [][]int32).

**Content sources read:**
- `Content/scripts/skill_combat/scripts/combat.rs2:1-15` (combat_effective_stat → scale; combat_maxhit; combat_stat).
- `Content/scripts/skill_combat/scripts/player/player_combat_stat.rs2:1-100` (full combat stat dispatch).
- `Content/scripts/skill_combat/configs/combat.param:1-160` (strengthbonus default=0; max_dealt default=1000).
- `Content/scripts/tutorial/configs/tutorial.npc:258-283` (newbiegiantrat — no max_dealt override).
- `Content/scripts/skill_combat/configs/melee/daggers.obj` (bronze_dagger param=strengthbonus,3).
- `Content/scripts/player/scripts/equip.rs2:195-236` (equip_get_bonuses).

## Provenance

Stage 1 conducted controller-direct per `bundle0_short_circuits_stage1_audit`. No audit subagent dispatched. All TS-source verdicts produced via direct Read of `LostCityRS/Engine-TS/`; no fabrication risk surface (`audit_subagent_fabrication`).

The spec's risk-ranked enumeration (S1 highest, S5 mid-rank) was inverted by the actual root cause: S5 binds; S1/S2 are real but non-contributing. This is a textbook `risk_register_premise_grep` reminder — the spec hypothesised sign-extension because of NAI-122's clear precedent, but the smoke-binding bug was a far simpler operand-swap that no prior memory had primed. Investigation cadence held: per-surface controller probing through ALL six surfaces (rather than short-circuiting on S1) was what surfaced S5.
