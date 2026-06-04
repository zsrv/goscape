# NAI-169 — NAI-44 sibling deviation cluster retirement (compressed cadence, docs-only)

**Status:** spec written 2026-05-11. Compressed cadence — single combined spec+plan doc, single docs commit + close. No code-logic change.

**Tech stack:** Go 1.26+ (per `go_version` memory).

**Lineage:** Retires 4 sibling deviations opened during NAI-44 close (commit lineage `c8a92f0..1cc7f44`, 2026-05-01). Three were silently resolved by subsequent sub-specs (NAI-45 T5, NAI-68) or by ambient porting work (Player.processWalktrigger ports); one is misframed at the tracker level. Per `tracker_entry_framing_can_be_incomplete` + `retire_deviation_grep_all_comments`.

## 1. Goal

Retire 4 stale NAI-44 sibling deviation tags. PRIMARY: tracker `nai_followups.md` accurately reflects production state; reader looking up "what NAI-44 carry-forwards are open?" sees zero items. SECONDARY: comment at `modules/world/interaction.go:197-199` no longer cites a TS stun system that doesn't exist.

## 2. TS source of truth

`/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts`:

| Line | Code | Bears on |
|---|---|---|
| 801-803 | `busy() { return this.delayed \|\| this.containsModalInterface(); }` | Defines TS `busy()` — no stun check. |
| 805-812 | `canAccess() { if (World.shutdown) return true; else return !this.protect && !this.busy(); }` | Definitive TS `canAccess()` — no stun/freeze. |
| 1057-1070 | `processWalktrigger()` body | Mirrored by goscape `interaction.go:339-353` (fully ported). |
| 1255-1258 | `if (this.nextTarget) this.target = this.nextTarget;` | nextTarget pop semantics — landed by NAI-68. |

The string `stun` appears nowhere in `Player.ts`'s `canAccess` / `busy` / `processInteraction` paths.

## 3. Goscape state at HEAD `4d91a5e`

### 3.1 NAI-44-D-CANACCESS-NO-STUN-CHECK (misframed)

`modules/world/interaction.go:197-199` carries:
```go
// DEVIATION NAI-44-D-CANACCESS-NO-STUN-CHECK: TS canAccess() also tests
// stun/freeze; goscape has no stun system, so the !p.delayed subset is
// the in-tree approximation.
if p.delayed && s.currentTick < p.delayedUntil {
    return
}
```

Four facts at HEAD:
1. TS `canAccess()` (Player.ts:805-812) tests `!protect && !busy()` — no stun (verified via grep + Read of Player.ts).
2. Goscape has a TS-faithful `(*Player).CanAccess()` port at `player_script.go:390-401` (delayed + modal + protected-script gates).
3. The early-return at L200-202 is an intentional goscape-only tick-math entry-guard pinned by `TestProcessInteraction_CanAccessGate_Delayed_EarlyReturnsBeforePathing` (`interaction_canaccess_gate_test.go:114`) added by NAI-155 (`ac0b56a`). The test comment explicitly frames it as "tick-math entry guard short-circuits the whole function".
4. The TS-aligned CanAccess gates are the three inline checks at `interaction.go:247, 261, 277` (TS L1210/L1232/L1244 mirrors).

The deviation tag's "TS canAccess also tests stun" premise is false. There is no underlying TS-fidelity gap to close.

### 3.2 NAI-44-D-PLAYER-WALKTRIGGER-NOOP (impl shipped)

Tracker (`nai_followups.md:2852`) claims `(p *Player).processWalktrigger()` is "an empty stub for TS-shape parity, closure bundles with NAI-37-D-WALKTRIGGER-NOREADER".

Production at HEAD: `interaction.go:339-353` carries the fully ported method — walktrigger field guard, delayed/protected gate, runScript dispatch (TS Player.ts:1057-1070 mirror). Comment cites "Mirrors TS Player.processWalktrigger at Player.ts:1057-1070" and "The !p.protectedScriptActive() gate mirrors TS L1062 !this.protect". Five live call sites: `handlers_game.go:302`, `interaction.go:249`, `interaction.go:262`, `player_post_decode.go:77`.

The tracker entry is stale — the deviation was silently closed by ambient porting (likely NAI-52 / NAI-111 / NAI-155 walktrigger work).

NAI-37-D-WALKTRIGGER-NOREADER (tracker line 2460) is the NPC-side equivalent and is **not** retired by this sub-spec — it concerns `*Npc.walktrigger` AI-tick consumption which remains unimplemented. Only the Player-side NAI-44-D entry is stale.

### 3.3 NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET (closed by NAI-68)

Tracker (`nai_followups.md:3489, 3541, 3635`) lists "blocked on `p_op*` reshape" as a NAI-65/NAI-66/NAI-67 lineage carry-forward.

Production at HEAD: closed by NAI-68 (commit retiring this deviation; close memory trailer recorded at line 3717 `Closes memory: NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`). Cited in-comment at `interaction.go:294-295` ("NAI-68 closed NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET via this reshape") and `interaction_trigger.go:189` ("NAI-68 closes NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET"). NAI-68 close-memo at line 3674 also confirms `Deviations closed: NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`.

The three lineage carry-forward entries were not housekept when NAI-68 closed. Stale.

### 3.4 NAI-44-D-CONTINUEWALK-UNUSED (closed by NAI-45 T5)

Tracker (`nai_followups.md:2876`) lists "dead-API-polish at next sub-spec close if no consumer".

Production at HEAD: closed by NAI-45 T5 (memo lines 2914, 2922, 2930 confirm: `Scope: ... retire ... NAI-44-D-CONTINUEWALK-UNUSED (tryInteract dead param)`; `NAI-44-D-CONTINUEWALK-UNUSED — tryInteract continueWalk bool removed (T5)`; `T5 close commit carries Closes memory: NAI-45-D3, NAI-44-D-CONTINUEWALK-UNUSED`).

The open-list entry at line 2876 was not housekept when NAI-45 T5 closed. Stale.

## 4. Production change (~6 LOC, comment-only)

Rewrite `modules/world/interaction.go:197-202`:

**Before:**
```go
// DEVIATION NAI-44-D-CANACCESS-NO-STUN-CHECK: TS canAccess() also tests
// stun/freeze; goscape has no stun system, so the !p.delayed subset is
// the in-tree approximation.
if p.delayed && s.currentTick < p.delayedUntil {
    return
}
```

**After:**
```go
// Tick-math entry-guard short-circuit (goscape-only optimization; not a
// TS-fidelity gap — TS canAccess at Player.ts:805-812 is !protect && !busy,
// no stun/freeze). The three canonical CanAccess gates at L247/L261/L277
// (TS L1210/L1232/L1244 mirrors) are the actual TS-faithful layer; this
// pre-empts the whole function (and Frame B emit) when the player is in a
// delay window. Pinned by TestProcessInteraction_CanAccessGate_Delayed_
// EarlyReturnsBeforePathing (NAI-155, commit ac0b56a).
if p.delayed && s.currentTick < p.delayedUntil {
    return
}
```

No production logic change. The DEVIATION tag is removed; the early-return is now annotated as an intentional goscape optimization (not a TS divergence).

## 5. Tracker housekeeping (`nai_followups.md`)

Five strike-through annotations:

| Line | Current state | New state |
|---|---|---|
| 2852 | Open: NAI-44-D-PLAYER-WALKTRIGGER-NOOP | RETIRED 2026-05-11 by NAI-169 — impl ported at `interaction.go:339-353` |
| 2853 | Open: NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET (closure: future `p_op*` reshape) | RETIRED 2026-05-10 by NAI-68 (already noted at line 3674; this is the canonical entry) |
| 2855 | Open: NAI-44-D-CANACCESS-NO-STUN-CHECK (closure: stun system port) | RETIRED 2026-05-11 by NAI-169 — TS canAccess has no stun; no underlying divergence |
| 2876 | Open-list item: NAI-44-D-CONTINUEWALK-UNUSED closure (already retired at line 2922 / 2930) | RETIRED 2026-05-01 by NAI-45 T5 (annotation only) |
| 2877 | Open-list item: NAI-44-D-CANACCESS-NO-STUN-CHECK closure | RETIRED 2026-05-11 by NAI-169 |

Plus 4 lineage carry-forward annotations:
- Line 3489 — annotate IMMEDIATE-POP (RETIRED by NAI-68) and CANACCESS (RETIRED by NAI-169) retirements
- Line 3541 — same
- Line 3635 — annotate IMMEDIATE-POP + CANACCESS retirements (NAI-66 close lineage)
- Line 3731 — annotate CANACCESS retirement (NAI-67 close lineage)

Total: 9 annotations across 5 canonical + 4 lineage references. Annotations use the existing pattern (`~~text~~ RETIRED YYYY-MM-DD by NAI-N` per `close_commit_memory_trailer`).

## 6. Tests intentionally NOT included (with rationale)

| Skipped test | Rationale |
|---|---|
| New test pinning the rewritten comment | Comments aren't behavior. Existing test `TestProcessInteraction_CanAccessGate_Delayed_EarlyReturnsBeforePathing` at `interaction_canaccess_gate_test.go:114` already pins the early-return semantics. Adding another would be redundant. |
| New regression test for IMMEDIATE-POP-VS-NEXTTARGET retirement | NAI-68 already closed it with its own pin. Re-pinning would be churn. |
| New regression test for WALKTRIGGER-NOOP retirement | `processWalktrigger` body has been live for many sub-specs; its callers' tests (NAI-52, NAI-111, NAI-155) exercise it. No new behavior. |

Per `helper_as_oracle_test_anti_pattern`: no helper-as-oracle traps here because there's no new code path to test.

## 7. Deviations expected

None. Pure tracker hygiene + comment reframe. Per `tracker_entry_framing_can_be_incomplete`: tracker assertions can be fact-correct but framing-wrong; the NAI-44-D-CANACCESS-NO-STUN-CHECK entry is fact-correct on "goscape has no stun" but framing-wrong on "TS canAccess tests stun".

## 8. Risk register

| ID | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | A reader of the original comment took the stun framing literally and planned a stun port. | Low | `rg "stun" pkg/ modules/` returns zero hits at HEAD — no production code anticipates a stun field. Pre-flight verified. |
| R2 | The early-return at L200-202 has subtle TS-divergence (skips nextTarget pop / tail mapflag / Frame B for delayed players within window) that the rewritten comment glosses over. | Low | Acknowledged in §3.1; NAI-155 explicitly locked this in via its pin. Mentioning it in the comment without action would lobby for future churn; keeping the comment concise + citing the pin is the cleaner path. If a future smoke surfaces a real bug from this divergence, open a fresh NAI. |
| R3 | NAI-68 close-memo line 3674 already records IMMEDIATE-POP retirement; striking through lineage carry-forwards (3489, 3541, 3635) is redundant. | Trivial | Annotate, don't delete. Lineage carry-forwards document historical state; annotating "RETIRED by NAI-68" preserves history while making current state legible. |
| R4 | Future tracker reader misinterprets the strike-through cluster as one retirement (4-in-1) instead of 3 lineage-stale + 1 fresh. | Trivial | NAI-169 close commit trailer enumerates all 4 retirements separately: `Closes memory: NAI-44-D-CANACCESS-NO-STUN-CHECK, NAI-44-D-PLAYER-WALKTRIGGER-NOOP, NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET (lineage), NAI-44-D-CONTINUEWALK-UNUSED (lineage)`. |

## 9. Cadence + commits

Per `compressed_cadence`: single combined spec+plan doc; single docs commit + close.

| Step | Commit | Body |
|---|---|---|
| Spec | `docs(spec): NAI-169 — NAI-44 sibling deviation cluster retirement` | This file. |
| T1 | `docs(world): NAI-169 — reframe NAI-44-D-CANACCESS-NO-STUN-CHECK comment at interaction.go` | The ~6 LOC comment rewrite. No logic change. |
| Close | `chore(close): NAI-169 — NAI-44 sibling deviation cluster retirement (4 tags)` | Empty commit; carries `Closes memory:` trailer with all 4 tags. |

No TDD pair — no production logic changes. The behavior is pinned by NAI-155's existing test.

## 10. Verification protocol (per `verification_before_completion`)

**Pre-T1 baseline:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'CanAccessGate'` green at HEAD `4d91a5e`.

**Post-T1:** same test green; no behavior change. `git show <T1-SHA>` confirms only the 6-line comment rewrite at `interaction.go:197-202` (no other touches).

**Final:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green. `rg "NAI-44-D-CANACCESS-NO-STUN-CHECK" pkg/ modules/` returns zero hits post-T1 (was 1 — the rewritten comment). `rg "NAI-44-D-PLAYER-WALKTRIGGER-NOOP|NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET|NAI-44-D-CONTINUEWALK-UNUSED" pkg/ modules/` already returns either zero or `closed`-annotated hits (verified pre-flight); no change expected.

## 11. Pattern memories applied

- `compressed_cadence` — single combined spec+plan; no separate plan file.
- `runescript_cadence` — preserved spec → docs commit → close phasing despite compression.
- `tracker_entry_framing_can_be_incomplete` — NAI-44-D-CANACCESS-NO-STUN-CHECK is fact-correct on "no stun" but framing-wrong on "TS canAccess tests stun"; brainstorm re-derived from TS Player.ts primary source.
- `retire_deviation_grep_all_comments` — pre-flight grep enumerated all 4 tags' production touch points (1 live for CANACCESS, 0 for WALKTRIGGER-NOOP, "closed" annotations for IMMEDIATE-POP, 0 for CONTINUEWALK).
- `close_commit_memory_trailer` — close commit trailer enumerates all 4 retired tags.
- `audit_full_method_against_ts` — TS Player.ts:801-812 (busy + canAccess) read line-by-line; confirms no stun/freeze logic.
- `defensive_gate_doc_comment_label` — rewritten comment explicitly labels the early-return as "goscape-only optimization; not a TS-fidelity gap" (parallel to "goscape defensive; TS skips this check" pattern).
- `verify_implementer_claims` — `git show` post-T1 confirms diff matches stated 6-LOC comment-only scope.

## 12. Out of scope

- Stun system port: TS has no stun system in this engine version; not planned.
- NAI-37-D-WALKTRIGGER-NOREADER retirement: NPC-side AI-tick walktrigger consumer is genuinely missing; remains open.
- Investigation of whether the L200-202 early-return's skipped tail processing (nextTarget pop / mapflag / Frame B) hides latent bugs for delayed players: NAI-155 explicitly locked this in; if a smoke surfaces a real symptom, open a separate NAI.
- Any other DEVIATION tag retirements outside the NAI-44 sibling cluster (e.g. NAI-91-D-OPERABLE-CHEB-FALLBACK, NAI-98-D-NPC-NO-FOLLOWXY, NAI-17-D2, NAI-59-D-MODALTUTORIAL-NO-PRODUCER): orthogonal and each warrants its own investigation per `tracker_entry_framing_can_be_incomplete`.

## 13. Smoke handoff

None. No production behavior change; no client-facing surface affected.
