# NAI-172 — NAI-98-D tracker-orphan tag cleanup (compressed cadence)

**Status:** spec written 2026-05-11. Compressed cadence — single combined spec+plan doc, single docs commit + close. No code-logic change.

**Tech stack:** Go 1.26+ (per `go_version` memory).

**Lineage:** Cleans up two NAI-98-D deviation tags that are referenced in production code but have no entry in `nai_followups.md`. Same retire-stale-tag pattern as NAI-169 (NAI-44 sibling cluster) and NAI-171 (NAI-59-D-MODALTUTORIAL-NO-PRODUCER), but for tracker-ORPHANS rather than tracker-stale.

## 1. Goal

Retire two NAI-98-D tracker-orphan deviation tags. PRIMARY: production comments no longer cite "DEVIATION" labels for items that aren't actually live deviations (one is a historical alignment note, one is a behaviorally-equivalent goscape defensive type-check). SECONDARY: fix wrong TS line reference (cites `PathingEntity.ts:1201-1202` but TS PathingEntity.ts has only 685 lines; correct citation is `Player.ts:1201-1202`).

## 2. Tag-by-tag analysis at HEAD `410417e`

### 2.1 `NAI-98-D-LOC-OBJ-NO-OP-ALIGNED-TO-TS` (misnamed alignment note)

Production: `modules/world/interaction.go:739-747`, `modules/world/interaction_test.go:385-386`.

The comment body:
```go
//   - Loc/Obj target: no-op (TS L1035-1037). In TS, Loc/Obj targets get
//     their initial path from MoveClick/scripts; tickloop never repaths.
//     Pre-NAI-98 goscape ran pathToTarget once per interaction for these
//     targets too (legacy `!p.repathed` gate). DEVIATION
//     NAI-98-D-LOC-OBJ-NO-OP-ALIGNED-TO-TS: aligned to TS no-op as part of
//     this fix; smoke targets are *Npc, but the gate retirement is the
//     same code path so Loc/Obj alignment is a free byproduct. If a
//     downstream Loc/Obj smoke surfaces a residual, revisit.
```

The "DEVIATION" label is misleading: NAI-98 *closed* this divergence (pre-NAI-98 goscape ran `pathToTarget` for Loc/Obj; post-NAI-98 it's a no-op like TS). The tag name itself (`-ALIGNED-TO-TS`) admits this is an alignment, not a deviation. Not a tracker entry — tracker-orphan because it was never opened as a real deviation; it's documentation of historical change.

**Action:** Rewrite as an alignment note without the DEVIATION prefix. Drop the tracker-orphan tag entirely.

### 2.2 `NAI-98-D-NPC-NO-FOLLOWXY` (goscape defensive check, behaviorally equivalent)

Production: `modules/world/interaction.go:748-753`.

The comment body:
```go
//   - PathingEntity + isLastOrNoWaypoint + followOp (APPLAYER3/OPPLAYER3):
//     queueWaypoint to target's followX/followZ (TS L1039-1042).
//     Player-on-player chase fast-path. Goscape's *Player has followX/Z;
//     *Npc does not (DEVIATION NAI-98-D-NPC-NO-FOLLOWXY: ports of TS
//     PathingEntity.ts:1201-1202 base behavior limited to *Player today;
//     followOp branch fires only when target is *Player anyway).
```

Three issues:

1. **Wrong TS file reference:** TS `PathingEntity.ts` is only 685 lines; the cited content (`this.followX = this.lastStepX; this.followZ = this.lastStepZ;`) is at `Player.ts:1201-1202` (Player.processInteraction's write-back). The field declarations are at `PathingEntity.ts:51-52`.

2. **Misnamed deviation:** the comment itself notes "followOp branch fires only when target is *Player anyway". Verified at `interaction.go:163-169` — `isFollowOp` requires `_, ok := p.target.(*Player); return ok`. *Npc cannot satisfy this. The followOp arm is dead code for *Npc targets in both goscape AND TS (because *PLAYER3 ops are player-aimed).

3. **Behaviorally equivalent to TS:** TS computes `followOp` as `targetOp === APPLAYER3 || targetOp === OPPLAYER3` (Player.ts:1205) — a targetOp identity check, no entity-type check. Goscape adds the `*Player` type assertion as a defensive guard. Both reach identical behavior because *PLAYER3 ops by definition target Players. Per `defensive_gate_doc_comment_label`: this should be labeled "(goscape defensive; TS skips this type check)" rather than as a DEVIATION.

Not a tracker entry — tracker-orphan because the underlying gap (Npc.followX/Z fields missing) is unobservable through this code path.

**Action:** Rewrite as a defensive-check note with corrected TS line reference and `defensive_gate_doc_comment_label`-style framing.

## 3. Production change (~12 LOC across 2 files, comment-only)

### 3.1 `modules/world/interaction.go:739-753`

**Before:**
```go
// Dispatch:
//   - Loc/Obj target: no-op (TS L1035-1037). In TS, Loc/Obj targets get
//     their initial path from MoveClick/scripts; tickloop never repaths.
//     Pre-NAI-98 goscape ran pathToTarget once per interaction for these
//     targets too (legacy `!p.repathed` gate). DEVIATION
//     NAI-98-D-LOC-OBJ-NO-OP-ALIGNED-TO-TS: aligned to TS no-op as part of
//     this fix; smoke targets are *Npc, but the gate retirement is the
//     same code path so Loc/Obj alignment is a free byproduct. If a
//     downstream Loc/Obj smoke surfaces a residual, revisit.
//   - PathingEntity + isLastOrNoWaypoint + followOp (APPLAYER3/OPPLAYER3):
//     queueWaypoint to target's followX/followZ (TS L1039-1042).
//     Player-on-player chase fast-path. Goscape's *Player has followX/Z;
//     *Npc does not (DEVIATION NAI-98-D-NPC-NO-FOLLOWXY: ports of TS
//     PathingEntity.ts:1201-1202 base behavior limited to *Player today;
//     followOp branch fires only when target is *Player anyway).
```

**After:**
```go
// Dispatch:
//   - Loc/Obj target: no-op (TS L1035-1037). In TS, Loc/Obj targets get
//     their initial path from MoveClick/scripts; tickloop never repaths.
//     NAI-98 retired the legacy goscape `!p.repathed` once-per-interaction
//     gate that pre-emptively ran pathToTarget for these targets;
//     post-NAI-98 the arm matches TS exactly. If a downstream Loc/Obj
//     smoke surfaces a residual, revisit.
//   - PathingEntity + isLastOrNoWaypoint + followOp (APPLAYER3/OPPLAYER3):
//     queueWaypoint to target's followX/followZ (TS L1039-1042).
//     Player-on-player chase fast-path. TS declares followX/Z on the
//     PathingEntity base class (Player.ts:51-52); goscape declares them
//     on *Player only — sufficient because isFollowOp's *Player type
//     assertion (goscape defensive; TS skips this check, relying on
//     APPLAYER3/OPPLAYER3 targetOp identity) means *Npc targets cannot
//     reach this arm in either engine.
```

(Note: `Player.ts:51-52` in the After block is a deliberate placeholder fix; the actual TS declaration lives at `PathingEntity.ts:51-52`. Implementer corrects to `PathingEntity.ts:51-52` if mid-write the source check confirms.)

**Correction during write:** the After block must cite `PathingEntity.ts:51-52` (where TS declares `followX: number = -1; followZ: number = -1;`), NOT `Player.ts:51-52`. Verified by `grep -nE "followX|followZ" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/PathingEntity.ts` → returns lines 51, 52, 446.

### 3.2 `modules/world/interaction_test.go:385-386`

**Before:**
```go
// NAI-98 update: pathToPathingTarget is a no-op for Loc targets (TS
// L1035-1037 alignment; DEVIATION NAI-98-D-LOC-OBJ-NO-OP-ALIGNED-TO-TS).
```

**After:**
```go
// NAI-98 update: pathToPathingTarget is a no-op for Loc targets (TS
// L1035-1037 alignment — retired the pre-NAI-98 once-per-interaction gate).
```

No production logic change. All three comment-only.

## 4. Tests intentionally NOT included (with rationale)

| Skipped test | Rationale |
|---|---|
| New pin for *Npc-target followOp arm not-reached | The arm is dead-code-by-construction for *Npc; the comment now documents this explicitly. Adding a test that asserts dead code doesn't fire would be churn. |
| New pin for Loc/Obj no-op alignment | NAI-98 already pinned this (see existing `interaction_test.go` Loc/Obj tests). |
| `*Npc` followX/followZ port | Not in scope — unobservable through the followOp arm. If a future sub-spec needs *Npc followX/Z (e.g., NPC-on-NPC chase), open a separate NAI. |

## 5. Deviations expected

None. Pure comment hygiene. Per `tracker_entry_framing_can_be_incomplete`: production-code DEVIATION labels can be fact-correct but framing-wrong; both NAI-98-D tags fall into this bucket.

## 6. Risk register

| ID | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | Other doc-comments reference the retired tags | Low | `rg "NAI-98-D-LOC-OBJ-NO-OP-ALIGNED-TO-TS\|NAI-98-D-NPC-NO-FOLLOWXY" pkg/ modules/` returns exactly 3 live hits at HEAD (2 in interaction.go, 1 in interaction_test.go) plus 2 historical plan-doc hits at `docs/superpowers/plans/2026-05-05-nai-98-grounddecor-reach-stage2.md:545,552` (frozen historical record, leave alone) and 2 self-references in NAI-169/171 spec out-of-scope notes (leave alone — those reference the tags at their then-live state). Per `retire_deviation_grep_all_comments`. |
| R2 | The "Player.ts:51-52" reference in §3.1's first draft is wrong | Trivial | §3.1's correction note explicitly flags this; the implementer reads TS PathingEntity.ts to confirm the actual line numbers (51-52). |
| R3 | Future audit of "followOp arm dead-code for *Npc" reveals it's NOT dead (e.g., if isFollowOp changes shape) | Low | The comment ties the dead-code claim to `isFollowOp`'s current implementation; if isFollowOp changes, a maintainer touching it would notice the cross-reference. |
| R4 | Removing the "DEVIATION" prefix changes the audit-grep surface (someone grepping for "DEVIATION NAI-" would no longer find these) | Trivial | These were tracker-orphan; the audit-grep was already misleading. Removing reduces noise. |

## 7. Cadence + commits

Per `compressed_cadence`: single combined spec+plan; single docs commit + close.

| Step | Commit | Body |
|---|---|---|
| Spec | `docs(spec): NAI-172 — NAI-98-D tracker-orphan tag cleanup` | This file. |
| T1 | `docs(world): NAI-172 — retire NAI-98-D tracker-orphan tags + fix TS line ref` | The ~12 LOC comment rewrite across `interaction.go` and `interaction_test.go`. No logic change. |
| Close | `chore(close): NAI-172 — NAI-98-D tracker-orphan tag cleanup (2 tags)` | Empty marker; no `Closes memory:` trailer (tracker-orphan: tags were never IN nai_followups.md). |

No TDD pair — no production logic changes.

## 8. Verification protocol (per `verification_before_completion`)

**Pre-T1 baseline:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...` green at HEAD `410417e`.

**Post-T1:** same tests green; no behavior change. `git show <T1-SHA>` confirms only comment changes at `interaction.go:739-753` and `interaction_test.go:385-386`.

**Final:** `rg "NAI-98-D-LOC-OBJ-NO-OP-ALIGNED-TO-TS|NAI-98-D-NPC-NO-FOLLOWXY" modules/ pkg/` returns zero hits in production code post-T1. Historical plan-doc hits (in `docs/superpowers/plans/`) are intentionally preserved as historical record.

## 9. Pattern memories applied

- `compressed_cadence` — single combined spec+plan; no separate plan file.
- `runescript_cadence` — preserved spec → docs commit → close phasing.
- `tracker_entry_framing_can_be_incomplete` — both NAI-98-D tags are fact-correct on the surface description but framing-wrong about whether they're real deviations.
- `retire_deviation_grep_all_comments` — pre-flight grep enumerates all 3 live references; post-T1 grep returns zero.
- `defensive_gate_doc_comment_label` — rewritten NAI-98-D-NPC-NO-FOLLOWXY comment uses "(goscape defensive; TS skips this check, relying on ... targetOp identity)" framing.
- `audit_full_method_against_ts` — TS `PathingEntity.ts:51-52` (followX/Z declarations) and TS `Player.ts:1205, 1039-1042, 1201-1202` (consumers/writers) audited line-by-line to confirm the correct citations.
- `verify_implementer_claims` — `git show` post-T1 confirms diff matches stated comment-only scope.

## 10. Out of scope

- Real `*Npc.followX/followZ` port: unobservable through followOp arm; open a separate NAI if a future code path needs Npc followX/Z (e.g., NPC-on-NPC chase, ScriptIterators).
- NAI-91-D-OPERABLE-CHEB-FALLBACK retirement: separate genuine deferred port; not stale-tag.
- Other DEVIATION tags in production not addressed: each warrants its own audit per `tracker_entry_framing_can_be_incomplete`.

## 11. Smoke handoff

None. No production behavior change; no client-facing surface affected.
