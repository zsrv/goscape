# NAI-171 — `NAI-59-D-MODALTUTORIAL-NO-PRODUCER` stale-tag retirement (compressed cadence)

**Status:** spec written 2026-05-11. Compressed cadence — single combined spec+plan doc, single docs commit + close. No code-logic change.

**Tech stack:** Go 1.26+ (per `go_version` memory).

**Lineage:** Retires `NAI-59-D-MODALTUTORIAL-NO-PRODUCER` — opened at NAI-59 close (2026-04-30 cluster). Producer landed via NAI-112 (`dbe7768`, "H6.c TUT_OPEN unconditional re-emit"); tracker was never housekept. Same pattern as NAI-169's NAI-44-D-PLAYER-WALKTRIGGER-NOOP retirement.

## 1. Goal

Retire the stale `NAI-59-D-MODALTUTORIAL-NO-PRODUCER` deviation tag. PRIMARY: tracker `nai_followups.md` accurately reflects production state. SECONDARY: the doc-comment at `modules/world/player_interface.go:137-139` no longer cites a deviation whose underlying gap is closed.

## 2. Goscape state at HEAD `8b310bb`

### 2.1 Producer side (TS `openTutorial` → goscape `OpenTutorial`)

`modules/world/player_script.go:957-976` carries `(*Player).OpenTutorial(com int)`:
- Doc-comment: "OpenTutorial sets the player's tutorial-overlay component and writes TUT_OPEN directly."
- Body writes `p.modalTutorial = com` and emits TUT_OPEN via direct `writeOut`.

`pkg/script/handlers_interface.go:94-104` carries `handleTutOpen`:
- Dispatches script opcode TUT_OPEN.
- Calls `s.Self.OpenTutorial(com)`.

### 2.2 Close side (TS `closeTutorial` → goscape `CloseTutorial`)

`modules/world/player_script.go:995-1006` carries `(*Player).CloseTutorial()`:
- No-op if `p.modalTutorial == -1` (mirrors TS L717).
- Dispatches IF_CLOSE trigger (TS L718-721).
- Resets `p.modalTutorial = -1` (TS L723).
- Writes `TUT_OPEN(-1)` directly (TS L724; NAI-112 Stage 2.2).

`pkg/script/opcode.go:220` declares `OpTutClose = 2120`. `handlers_interface_test.go:1198-1218` pins TUT_CLOSE dispatch through `CloseTutorial`.

### 2.3 Stale comment

`modules/world/player_interface.go:137-139`:
```go
// modalTutorial IS initialized to -1 (see newPlayer); the != -1 guard
// is direct because the field is write-empty until the IF_OPENTUT-
// equivalent opcode lands (DEVIATION NAI-59-D-MODALTUTORIAL-NO-PRODUCER).
```

The IF_OPENTUT-equivalent opcode (TUT_OPEN) HAS landed. `OpenTutorial` writes the field; `IsComponentVisible` reads it at `player_interface.go:158`. The deviation is closed.

## 3. Production change (3 LOC, comment-only)

Replace `modules/world/player_interface.go:137-139`:

**Before:**
```go
// modalTutorial IS initialized to -1 (see newPlayer); the != -1 guard
// is direct because the field is write-empty until the IF_OPENTUT-
// equivalent opcode lands (DEVIATION NAI-59-D-MODALTUTORIAL-NO-PRODUCER).
```

**After:**
```go
// modalTutorial IS initialized to -1 (see newPlayer); the != -1 guard
// is direct. Producer is (*Player).OpenTutorial (player_script.go:971)
// via the TUT_OPEN script-opcode handler (handlers_interface.go:94).
```

No production logic change. The DEVIATION tag is removed; the comment now positively cites the producer.

## 4. Tracker housekeeping (`nai_followups.md`)

Six strike-through / annotation edits:

| Line | Current state | New state |
|---|---|---|
| ~3213-3222 | Open: NAI-59-D-MODALTUTORIAL-NO-PRODUCER (closure: future IF_OPENTUT opcode handler sub-spec) | RETIRED 2026-05-11 by NAI-171 — TUT_OPEN handler landed via NAI-112 (`dbe7768`) at `pkg/script/handlers_interface.go:94`; OpenTutorial at `player_script.go:971` writes `modalTutorial`. |
| ~3242 | Open-list item: IF_OPENTUT opcode handler closes NAI-59-D-MODALTUTORIAL-NO-PRODUCER | RETIRED 2026-05-11 by NAI-171. |
| ~3491 | Lineage carry-forward: NAI-59-D-MODALTUTORIAL-NO-PRODUCER (conditional on tutorial-content driver) | RETIRED 2026-05-11 by NAI-171. |
| ~3543 | Same | Same. |
| ~3635 | Same (NAI-67 carry-forward) | Same. |
| ~3732 | Same (NAI-68 carry-forward) | Same. |

The related "(*Player).CloseModal tutorial branch port (TS Player.ts:717-723)" follow-up (mentioned at NAI-59 close-doc line 3220-3222) refers to TS `closeTutorial()`, which is fully ported as `(*Player).CloseTutorial` at `player_script.go:995-1006`. NAI-171 retires that follow-up too — same root cause.

## 5. Tests intentionally NOT included (with rationale)

| Skipped test | Rationale |
|---|---|
| New regression test for OpenTutorial/CloseTutorial behavior | Existing pins: `player_test.go:803` (`TestNAI_112_OpenTutorialUnconditionalReEmit`), `player_test.go:773-...` (TUT_OPEN direct-write), `handlers_interface_test.go:1198-1218` (TUT_CLOSE dispatch). No new behavior to test. |
| Pin asserting comment text | Comments aren't behavior. |
| End-to-end smoke for IsComponentVisible modalTutorial branch | The reader-side code at `player_interface.go:158` is unchanged. Existing IsComponentVisible tests cover it. |

## 6. Deviations expected

None. Pure tracker hygiene + comment update. Per `tracker_entry_framing_can_be_incomplete`: the tracker entry was fact-correct at NAI-59 close but no longer matches production state after NAI-112.

## 7. Risk register

| ID | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | Other doc-comments reference the retired tag | Low | `rg "NAI-59-D-MODALTUTORIAL-NO-PRODUCER" pkg/ modules/` at impl-time confirms zero other references post-T1 (was 1 — the rewritten comment). Per `retire_deviation_grep_all_comments`. |
| R2 | OpenTutorial / CloseTutorial may have subtle TS-divergence I missed | Low | Both are explicitly NAI-112-validated with smoke pins (`TestNAI_112_OpenTutorialUnconditionalReEmit`); CloseTutorial cites TS L716-726 line-by-line. Outside this sub-spec's scope. |
| R3 | The "(*Player).CloseModal tutorial branch port" sub-follow-up at line 3220-3222 might describe a SEPARATE gap I'm conflating | Low | TS `closeTutorial()` is a standalone method at TS L716-726; goscape mirrors it as standalone `(*Player).CloseTutorial`. The NAI-59 note's framing ("(*Player).CloseModal tutorial branch") was imprecise — there is no tutorial branch IN CloseModal in TS, just a separate closeTutorial method. Verified by reading TS Player.ts:706-727. |

## 8. Cadence + commits

Per `compressed_cadence`: single combined spec+plan doc; single docs commit + close.

| Step | Commit | Body |
|---|---|---|
| Spec | `docs(spec): NAI-171 — modalTutorial-producer stale-tag retirement` | This file. |
| T1 | `docs(world): NAI-171 — retire NAI-59-D-MODALTUTORIAL-NO-PRODUCER at player_interface.go:137-139` | The 3-line comment rewrite. No logic change. |
| Close | `chore(close): NAI-171 — NAI-59-D-MODALTUTORIAL-NO-PRODUCER retirement` | Empty marker; carries `Closes memory: NAI-59-D-MODALTUTORIAL-NO-PRODUCER` trailer. |

No TDD pair — no production logic changes. Behavior was pinned by NAI-112's existing tests.

## 9. Verification protocol (per `verification_before_completion`)

**Pre-T1 baseline:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/...` green at HEAD `8b310bb`.

**Post-T1:** same tests green; no behavior change. `git show <T1-SHA>` confirms only the 3-line comment rewrite at `player_interface.go:137-139` (no other touches).

**Final:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green. `rg "NAI-59-D-MODALTUTORIAL-NO-PRODUCER" pkg/ modules/` returns zero hits post-T1.

## 10. Pattern memories applied

- `compressed_cadence` — single combined spec+plan; no separate plan file.
- `runescript_cadence` — preserved spec → docs commit → close phasing.
- `tracker_entry_framing_can_be_incomplete` — tracker entry was fact-correct at NAI-59 close but no longer matches production state after NAI-112.
- `retire_deviation_grep_all_comments` — pre-flight + post-T1 grep enumerates all references; only 1 live ref at HEAD (the canonical comment), zero post-T1.
- `close_commit_memory_trailer` — close commit trailer enumerates the retired tag.
- `audit_full_method_against_ts` — TS Player.ts:716-726 (closeTutorial) read line-by-line; confirms goscape CloseTutorial is faithful.
- `defensive_gate_doc_comment_label` — n/a (no defensive gates added here; existing ones at CloseTutorial already correctly labeled).
- `verify_implementer_claims` — `git show` post-T1 confirms diff matches stated 3-LOC comment-only scope.

## 11. Out of scope

- Any new modalTutorial behavior beyond comment update.
- NAI-112 follow-up smoke audits.
- Other active deviation tags (NAI-91-D-OPERABLE-CHEB-FALLBACK, NAI-98-D-NPC-NO-FOLLOWXY) — each warrants its own investigation per `tracker_entry_framing_can_be_incomplete`.

## 12. Smoke handoff

None. No production behavior change; no client-facing surface affected.
