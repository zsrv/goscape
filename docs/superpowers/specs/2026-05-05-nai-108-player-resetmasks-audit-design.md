## NAI-108: Player.ResetMasks ↔ TS resetPathingEntity audit + load-bearing fixes

**Date**: 2026-05-05
**Cadence**: Standard sub-spec (single bundle, ~5 tasks). End-of-bundle review on Sonnet per `superpowers_code_reviewer_model`. Subagent-driven-development per `execution_mode_default`. Compressed cadence threshold (~15 prod LOC) is exceeded by the audit-doc deliverable + (δ) verify-and-pin tests; standard cadence chosen per option (II) of the brainstorm.
**Predecessor**: NAI-107 (HEAD `fa9cb8a` — `handleLcName` Name → DebugName → "null" chain).
**Trigger**: NAI-91 close-day smoke surfaced "after speaking with NPC, player keeps facing NPC after walking away" as an untracked candidate (`nai_followups.md` line 5176). Static audit confirms the load-bearing root cause is a missing trailing-clear in `Player.ResetMasks` mirroring TS `PathingEntity.resetPathingEntity` lines 611-614 (already ported to NPC side at `npc_masks.go:204-207` in the NAI-13/14 era). Pre-existing tracker `NAI-72-N-RESETENTITY-PARTIAL` at `modules/world/tick.go:529-531` enumerates the broader gap and names this spec as the natural retirer.
**Tech stack**: Go 1.26+ (per `go_version.md`).
**Successor**: TBD; spec opens one new follow-up (`NAI-108-D-MOVESPEED-NOT-RESET`) requiring its own audit before fix.

---

### 1. Problem

**Symptom (user-reported, 2026-05-04 NAI-91 smoke)**: After speaking with RuneScape Guide NPC (typeId=945), player character continues to face the NPC even after walking away. Long-standing per user.

**Root cause (static-confirmed)**: `Player.ResetMasks` at `modules/world/player_masks.go:56-74` is missing the trailing per-tick block that TS `PathingEntity.resetPathingEntity` ships at `Engine-TS/src/engine/entity/PathingEntity.ts:611-614`:

```typescript
if (!this.target && this.faceEntity !== -1) {
    this.masks |= this.entitymask;
    this.faceEntity = -1;
}
```

When the player chats with an NPC, `Player.SetInteraction → focus()` sets `p.faceEntity = npcSlot` and OR-emits `MaskFaceEntity`. When the chat ends, `p.target = nil` is set at `interaction.go:130` (or via `ClearInteraction`). Without the trailing-clear, `p.faceEntity` carries the prior NPC slot indefinitely, and the client never receives a "stop facing" update.

The NPC-side equivalent has been correct since NAI-14 (commits `aaa…` era; see `npc_masks.go:184-207` doc-comment). The Player-side gap was never formally tracked.

**Adjacent gap (pre-existing tracker)**: `tick.go:529-531` reads:

```go
// NAI-72 — TS Player.resetEntity(false) at Player.ts:466-467.
// Reset social/report spam-protect flags so the next tick admits
// at most one social/report packet per type per player.
// (Other resetEntity fields — protect, chatColour/Effect/Rights,
// chatMessage, logMessage — belong to other sub-specs; tracked
// as NAI-72-N-RESETENTITY-PARTIAL.)
p.socialProtect = false
p.reportAbuseProtect = false
```

NAI-108 closes `NAI-72-N-RESETENTITY-PARTIAL` by classifying each named field and acting on the live ones.

---

### 2. TS reference

Two TS methods compose the per-tick reset surface for Player. Each tick, `tick.go processCleanup` (Player loop) is the goscape analogue of TS calling `resetEntity(false)` once per player.

**`Engine-TS/src/engine/entity/PathingEntity.ts:577-615` — `resetPathingEntity()`** (called by both Player and Npc `resetEntity`):

```typescript
protected resetPathingEntity(): void {
    this.moveSpeed = this.defaultMoveSpeed();
    this.walkDir = -1;
    this.runDir = -1;
    this.jump = false;
    this.tele = false;
    this.lastTickX = this.x;
    this.lastTickZ = this.z;
    this.lastLevel = this.level;
    this.stepsTaken = 0;
    this.interacted = false;
    this.apRangeCalled = false;

    this.masks = 0;
    this.exactStartX = -1;
    this.exactStartZ = -1;
    this.exactEndX = -1;
    this.exactEndZ = -1;
    this.exactMoveStart = -1;
    this.exactMoveEnd = -1;
    this.exactMoveFacing = -1;
    this.animId = -1;
    this.animDelay = -1;
    this.sayMessage = null;
    this.hitmarkDamage = -1;
    this.hitmarkType = -1;
    this.spotanimId = -1;
    this.spotanimHeight = -1;
    this.spotanimTime = -1;
    this.faceSquareX = -1;
    this.faceSquareZ = -1;

    if (!this.target && this.faceEntity !== -1) {
        this.masks |= this.entitymask;
        this.faceEntity = -1;
    }
}
```

**`Engine-TS/src/engine/entity/Player.ts:454-464` — `resetEntity(respawn)`**:

```typescript
resetEntity(respawn: boolean) {
    if (respawn) {
        this.unfocus();
    }
    super.resetPathingEntity();
    this.repathed = false;
    this.protect = false;
    this.chatColour = null;
    this.chatEffect = null;
    this.chatRights = null;
    this.chatMessage = null;
    this.logMessage = null;
    this.socialProtect = false;
    this.reportAbuseProtect = false;
}
```

---

### 3. Audit table

Full classification of every TS reset operation against goscape's current per-tick reset surface. Categories:

- **(α)** Already in `Player.ResetMasks` (no action).
- **(β)** Load-bearing missing — port now.
- **(γ)** Goscape intentional persistence — TS resets, goscape preserves; document as labeled divergence.
- **(δ)** Handled elsewhere — not in `ResetMasks`, but has a goscape handler that achieves equivalent semantics; verify and pin if untested.
- **(ε)** Player.resetEntity-specific (non-PathingEntity) — port, retire, or document per case.
- **(ζ)** Respawn-gated — defer to Player respawn/death sub-spec (tracked separately).

| TS field/op | TS line | Goscape disposition | Action |
|---|---|---|---|
| `moveSpeed = defaultMoveSpeed()` | 578 | (δ→follow-up) Player init `moveSpeed: MoveSpeedInstant` (`player.go:454`); set in `player_script.go:402`; read in `afkzone.go:33`, `movement.go:67`. **Not reset each tick.** Real divergence; consumers may rely on persistence. Risk-flag, no fix in NAI-108. | **Open `NAI-108-D-MOVESPEED-NOT-RESET`** in `nai_followups.md`. |
| `walkDir = -1` | 579 | (δ) reset in `movement.go:53,60,64-65` per movement step. Pinned by `movement_test.go:52,66,76-79,87`. | (δ) verify-and-pin: confirm test exists for "no movement → walkDir stays -1 (initial state) post-tick". Add if missing. |
| `runDir = -1` | 580 | (δ) reset in `movement.go:54,61,65,70` per movement step. Pinned by `movement_test.go:79`. | Same as walkDir. |
| `jump = false` | 581 | (α) `ResetMasks:59`. | None. |
| `tele = false` | 582 | (α) `ResetMasks:58`. | None. |
| `lastTickX = x` | 583 | (δ) set in `movement.go:48`. | (δ) verify-and-pin: confirm `lastTickX` matches `p.x` at tick-end when no movement occurred (degenerate equality). |
| `lastTickZ = z` | 584 | (δ) set in `movement.go:49`. | Same. |
| `lastLevel = level` | 585 | (δ) set in `movement.go:50`. | Same. |
| `stepsTaken = 0` | 586 | (δ) reset in `movement.go:46`. Test scaffolding sets it in `rsbuf_per_tick_test.go:171`, `player_reorient_test.go:93,122`. | (δ) verify-and-pin: confirm `stepsTaken=0` post-tick when player took no steps. |
| `interacted = false` | 587 | (δ) reset on `SetInteraction` (`interaction.go:86`) and `ClearInteraction` (`interaction.go:134`); set in `interaction.go:390,400` (post-fire). **NOT reset every tick** — relies on the next interaction-touching path. | (δ) verify-and-pin: trace an idle tick (no interaction set/cleared) and confirm `interacted` from a prior tick does not leak into a later tick's behavior. If it does, add to ResetMasks; if it doesn't (because all consumers re-set before reading), pin with a test. |
| `apRangeCalled = false` | 588 | (δ) reset on `SetInteraction` (`interaction.go:85`), `ClearInteraction` (`interaction.go:133`), and post-fire (`player_interaction_trigger.go:121`). | (δ) verify-and-pin: same flow as `interacted`. |
| `masks = 0` | 590 | (α) `ResetMasks:57`. | None. |
| `exactStartX = -1` | 591 | (α) `ResetMasks:67`. | None. |
| `exactStartZ = -1` | 592 | (α) `ResetMasks:68`. | None. |
| `exactEndX = -1` | 593 | (α) `ResetMasks:69`. | None. |
| `exactEndZ = -1` | 594 | (α) `ResetMasks:70`. | None. |
| `exactMoveStart = -1` | 595 | (α) goscape uses `exactBegin`; `ResetMasks:71`. Naming divergence only. | None. |
| `exactMoveEnd = -1` | 596 | (α) goscape uses `exactFinish`; `ResetMasks:72`. | None. |
| `exactMoveFacing = -1` | 597 | (α) goscape uses `exactDir`; `ResetMasks:73`. | None. |
| `animId = -1` | 598 | (γ) goscape persists. Player init `animID: -1` (`player.go:492`); set by `PlayAnim` consumers; **never reset by `ResetMasks`**. Designated persistent per `player_masks.go:51` comment. | (γ) reinforce inline label in `player_masks.go` ResetMasks doc-comment — cite TS line + reason for divergence (animation persistence across ticks until script changes it). |
| `animDelay = -1` | 599 | (γ) same as animID; `player.go:493`. | (γ) same. |
| `sayMessage = null` | 601 | (α) goscape `sayText`; `ResetMasks:60`. | None. |
| `hitmarkDamage = -1` | 602 | (α) goscape `damageAmt`; `ResetMasks:62`. | None. |
| `hitmarkType = -1` | 603 | (α) goscape `damageType`; `ResetMasks:63`. | None. |
| `spotanimId = -1` | 604 | (α) goscape `spotanimID`; `ResetMasks:64`. | None. |
| `spotanimHeight = -1` | 605 | (α) `ResetMasks:65`. | None. |
| `spotanimTime = -1` | 606 | (α) goscape `spotanimDelay`; `ResetMasks:66`. | None. |
| `faceSquareX = -1` | 608 | (γ) goscape persists. Player init `faceSquareX: -1` (`player.go:511`); set by `FaceCoord` (`player_masks.go:40`); never reset. The persistence-vs-reset question is non-symptomatic because the encoder gates on `MaskFaceCoord` (which IS reset via `ResetMasks:57`'s `p.masks = 0`). | (γ) add explicit inline label in `player_masks.go` doc-comment — TS line + reason. |
| `faceSquareZ = -1` | 609 | (γ) same as faceSquareX. | (γ) same. |
| **Trailing clear: `!target && faceEntity != -1` → `masks \|= entitymask; faceEntity = -1`** | **611-614** | **(β) MISSING — load-bearing.** Causes the user-reported "player keeps facing NPC" symptom. NPC-side analogue at `npc_masks.go:204-207`. | **Port to `Player.ResetMasks` exactly mirroring NPC-side shape.** |
| **Player.resetEntity (Player.ts:454-464)** | | | |
| `if (respawn) this.unfocus()` | 455-457 | (ζ) `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` — blocked on Player respawn/death sub-spec. No consumer in goscape (no respawn path ported yet). | None. Stays deferred. |
| `super.resetPathingEntity()` | 458 | covered above. | (covered) |
| `repathed = false` | 459 | (ε) `Player.repathed` field RETIRED by NAI-98 as TS-vestigial (declared but never read in TS; per `vestigial_field_misread.md`). The TS reset itself is dead code. Goscape correctly omits the field AND its reset. | None. Already-converged divergence — reinforce in spec for clarity, no code change. |
| `protect = false` | 460 | (ε) **Already-converged divergence**: goscape's TS `this.protect` semantics are remapped to `activeScript.Protect` (a script-level field, not a Player field). Documented at `interaction.go:308`, `player_script.go:276,297-300`, `interaction_test.go:772`, `modal_close_test.go:103,148`, `player_test.go:720`. No Player.protect field exists; no reset needed. | None. Reinforce in spec; update `tick.go:529-531` comment to remove `protect` from the deferred list. |
| `chatColour = null` | 461 | (ε) Player field exists (`player.go:197`, init -1 at `player.go:494`). Set in `Chat()` (`player_masks.go:14`). Read in `tick.go:423` for chat encode, **gated on `chatBytes != nil`** (which IS reset via `ResetMasks:61`). Stale color values are functionally inert. **TS-fidelity port adds reset-to-(-1)**; behavior unchanged. | (ε) **Port reset to ResetMasks** for TS fidelity. Pin "chat encoder no-op when chatBytes nil regardless of color" test. |
| `chatEffect = null` | 462 | (ε) same as chatColour. | (ε) same. |
| `chatRights = null` | 463 | (ε) same as chatColour. | (ε) same. |
| `chatMessage = null` | 464 | (ε) **Dead field**: declared at `player.go:196` as `chatMessage []byte`, never read or written anywhere in goscape (verified via `rg "chatMessage"` — only the declaration + comment matches). | (ε) **DELETE** the field per `dead_api_polish.md`. |
| `logMessage = null` | 465 | (ε) **YAGNI**: never declared as a goscape field; no consumer. TS-only. | None. Reinforce in spec; update `tick.go:529-531` comment to remove `logMessage` from the deferred list. |
| `socialProtect = false` | 466 | (α-equivalent) reset in `tick.go:532` (NAI-72). | None. |
| `reportAbuseProtect = false` | 467 | (α-equivalent) reset in `tick.go:533` (NAI-72). | None. |

---

### 4. Goscape changes

**Stage A — Code (load-bearing + (ε) cleanup):**

1. **`modules/world/player_masks.go` `ResetMasks`** — add the trailing-clear block mirroring `npc_masks.go:204-207`:

   ```go
   if p.target == nil && p.faceEntity != -1 {
       p.masks |= p.entitymask
       p.faceEntity = -1
   }
   ```

   Also reset chat metadata for TS fidelity (no observable effect; encoder gates on chatBytes != nil):

   ```go
   p.chatColour = -1
   p.chatEffect  = -1
   p.chatRights  = -1
   ```

   Expand the doc-comment to enumerate intentional persistence divergences with TS-line citations:

   - `animID`/`animDelay` (TS PathingEntity.ts:598-600) — persists across ticks until script changes it; goscape design choice for animation continuity.
   - `faceSquareX`/`faceSquareZ` (TS PathingEntity.ts:608-609) — non-symptomatic persistence; encoder gates on `MaskFaceCoord` which is cleared via `p.masks = 0`.
   - The trailing-clear rationale (one-tick lag deviation already documented for NPC at `npc_masks.go:187-195`; same applies to Player).

2. **`modules/world/player.go`** — delete the dead field at line 196:

   ```go
   chatMessage                        []byte
   ```

   Remove the entire `chatMessage` declaration. The `chatColour, chatEffect, chatRights int` declaration at line 197 stays.

3. **`modules/world/tick.go:526-531`** — update the comment to retire `NAI-72-N-RESETENTITY-PARTIAL` and reflect the post-NAI-108 deferred list:

   ```go
   // NAI-72/108 — TS Player.resetEntity(false) at Player.ts:454-467.
   // Reset social/report spam-protect flags so the next tick admits
   // at most one social/report packet per type per player.
   // (NAI-72-N-RESETENTITY-PARTIAL retired by NAI-108: protect ≡
   // activeScript.Protect [already-converged], chatColour/Effect/Rights
   // [moved to ResetMasks per TS fidelity], chatMessage [dead field
   // deleted], logMessage [TS-only, no goscape consumer]. unfocus()
   // remains deferred per NAI-67-D-PLAYER-UNFOCUS-DEFERRED.)
   ```

**Stage B — Tests (TDD-ordered):**

4. **`modules/world/player_masks_test.go`** — add to NAI-108 section:

   - `TestPlayerResetMasksTrailingClearFires` — set `p.faceEntity = 42`, `p.target = nil`, run `ResetMasks`, assert `p.faceEntity == -1` and `p.masks & rsbuf.MaskFaceEntity != 0`. Mirrors NPC test at `npc_masks_test.go:237-249`.
   - `TestPlayerResetMasksTrailingClearSkippedWhenTargetPresent` — set `p.faceEntity = 42`, `p.target = &Npc{}`, run `ResetMasks`, assert `p.faceEntity == 42` and `p.masks & rsbuf.MaskFaceEntity == 0`. Mirrors NPC test at `npc_masks_test.go:254-267`.
   - `TestPlayerResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne` — set `p.faceEntity = -1`, `p.target = nil`, run `ResetMasks`, assert `p.masks == 0` (no entitymask emit). Mirrors NPC test at `npc_masks_test.go:272-281`.
   - `TestPlayerResetMasksClearsChatMetadata` — set `p.chatColour = 5; p.chatEffect = 3; p.chatRights = 2`, run `ResetMasks`, assert all three are `-1` post-reset.
   - `TestChatEncoderGatesOnChatBytesNotColor` (regression pin for the (ε) chat-reset no-op claim) — exercise the `tick.go:423` chat-encode path with `p.chatBytes = nil` and arbitrary stale color values; assert no chat packet is written.

5. **(δ) verify-and-pin tests** — add per audit table where untested:

   - `TestPlayerLastTickFieldsMatchPositionWhenNoMovement` — set `p.x = 100; p.z = 200; p.level = 0; p.lastTickX = 50` (stale), call the relevant per-tick path, assert `lastTickX/Z/Level == p.x/z/level`. (Skip if existing test covers; TBD at plan-write.)
   - `TestPlayerStepsTakenZeroOnIdleTick` — same shape.
   - `TestPlayerWalkDirRunDirNegativeOneOnIdleTick` — same shape.
   - `TestPlayerInteractedFalseDoesNotLeakAcrossTicks` — set `p.interacted = true` mid-tick (simulating mid-tick fire), run cleanup, assert next-tick `interacted == false` (or document the goscape lifecycle that achieves equivalence).
   - `TestPlayerApRangeCalledFalseDoesNotLeakAcrossTicks` — same shape.

   **Scope discipline**: if the verify step reveals a (δ) item is NOT actually equivalent to TS (e.g. `interacted` leaks), escalate to a tracked deviation rather than scope-creep into Stage A. Implementer reports any such finding; controller decides escalate-vs-port.

---

### 5. Preserved as-is

- **NPC-side `ResetMasks`** at `npc_masks.go:184-207`. Already TS-correct since NAI-13/14. NAI-108 mirrors its trailing-clear shape verbatim.
- **`unfocus()`** — TS PathingEntity.ts:338. Stays deferred per `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` blocked on Player respawn/death sub-spec.
- **Respawn-gated branches in TS Player.resetEntity** — defer alongside `unfocus()`.
- **`activeScript.Protect`** — goscape's convergence of TS `Player.protect`. Stays as the canonical mapping; reinforce in spec.
- **`Player.repathed`** — already retired by NAI-98 as TS-vestigial. Stays deleted.
- **`Player.socialProtect` / `Player.reportAbuseProtect` resets at `tick.go:532-533`** — stays where they are. NAI-72's per-tick semantics are correct; only the comment updates.
- **NPC test surface** at `npc_masks_test.go:230-281`. Player tests mirror the NPC shape but live in `player_masks_test.go`; NPC tests stay untouched.
- **All compiled scripts and content data** — unchanged.

---

### 6. Deviations introduced

- **`NAI-108-D-MOVESPEED-NOT-RESET`** — new tracked deviation. TS resets `moveSpeed` to `defaultMoveSpeed()` each tick at `PathingEntity.ts:578`; goscape persists across ticks. Consumers (`afkzone.go:33`, `movement.go:67`, `player_script.go:402`) may rely on persistence. Requires a separate audit before fix; opens as a follow-up entry, not in NAI-108 scope. **Routing**: future NAI-N+1 sub-spec, sized after consumer audit.

- **One-tick-lag note for trailing-clear** (already documented for NPC at `npc_masks.go:190-195`): goscape's `ResetMasks` runs at tick end (`tick.go processCleanup`), not tick start as TS does. The mask-emit therefore fires on the NEXT tick's info-pass — a one-tick lag vs TS. Accepted deviation; symmetric with the NPC-side handling.

---

### 7. Deviations retired

- **`NAI-72-N-RESETENTITY-PARTIAL`** (`tick.go:529-531` comment) — fully retired by NAI-108. Each named field (`protect`, `chatColour/Effect/Rights`, `chatMessage`, `logMessage`) handled per the audit table.
- **NAI-91 untracked candidate "face-NPC-after-walking-away"** (`nai_followups.md` line 5176) — closed by Stage A item 1 (trailing-clear). Smoke binds at close.

---

### 8. Implementation plan stub

Detailed plan deferred to `superpowers:writing-plans`. Bundle structure preview:

**Bundle 1 — Player.ResetMasks audit + load-bearing port** (single subagent dispatch on Sonnet, end-of-bundle review):

1. **T1 (TDD red)** — add 5 trailing-clear + chat-metadata tests in `player_masks_test.go` (Stage B item 4).
2. **T2 (green)** — add trailing-clear block + chat-metadata reset to `Player.ResetMasks`; expand doc-comment with γ-divergence labels (Stage A item 1).
3. **T3 (dead-field retire)** — delete `chatMessage` field at `player.go:196` (Stage A item 2).
4. **T4 (δ verify-and-pin)** — add (δ) tests per Stage B item 5; controller decides escalate-vs-document for any divergence surfaced.
5. **T5 (comment update + audit doc)** — update `tick.go:526-531` comment (Stage A item 3); confirm spec audit table matches HEAD reality.
6. **T6 (full-repo verification)** — `go test ./...` + `go vet ./...` clean; HEAD-grep checks for retired sentinels (`chatMessage`, `NAI-72-N-RESETENTITY-PARTIAL`).

End-of-bundle review on Sonnet per `superpowers_code_reviewer_model`.

---

### 9. Risk register

- **R1 (medium) — (δ) verify step surfaces real divergences.** If `interacted` or `apRangeCalled` actually leaks across ticks (because no consumer re-sets in idle path), the test fails and the implementer must escalate. Disposition: controller decides per-case. Default: track as new deviation (`NAI-108-D-INTERACTED-LEAK` etc.) and document in spec; do NOT auto-port to ResetMasks. Reasoning: the NPC side has the same handler-elsewhere pattern at `npc_masks.go` and presumably works; surfacing a leak here would be a real bug worthy of its own sub-spec, not a drive-by in NAI-108.

- **R2 (low) — chat-metadata reset has unintended consumer.** Grep at spec-write: only consumer is `tick.go:423`, gated on `p.chatBytes != nil`. Pin the no-op claim with `TestChatEncoderGatesOnChatBytesNotColor` (Stage B item 4). De-risked.

- **R3 (low) — `chatMessage` field deletion breaks a stash/wip.** Pre-flight grep is clean (only `player.go:196` declaration + `tick.go:530` comment reference). Implementer runs `go build ./... && go vet ./...` post-delete; the comment mention at tick.go gets updated in T5 anyway.

- **R4 (medium) — `NAI-108-D-MOVESPEED-NOT-RESET` is load-bearing.** If subsequent smoke shows movespeed-related symptoms (player stuck in walk after running, mid-script speed not releasing), escalate. Currently no smoke pressure. Disposition: open as follow-up entry, route to NAI-N+1 if smoke pressure surfaces.

- **R5 (low) — Trailing-clear breaks `TestResetMasksClearsEphemerals` (`player_masks_test.go:72-98`).** Pre-flight verification at spec-write: the existing test calls `newTestPlayer(t)` which initializes `p.faceEntity = -1` (per `player.go:509`) and never re-assigns `faceEntity`. The trailing-clear is skipped (faceEntity already -1) → test continues to pass post-fix. **However**, the doc-comment at line 91 (`// Persistent (animID, faceEntity, levels[3], baseLevels[3]) should stay.`) becomes misleading post-NAI-108 because faceEntity persistence is now CONDITIONAL on `target != nil`. Disposition: implementer updates the comment in T2 to clarify "faceEntity persists when target is present OR when faceEntity is already -1 (per NAI-108 trailing-clear semantics)". No assertion changes.

- **R6 (low) — `p.target` field assignment in tests.** `p.target` has type `entity` (lowercase interface, defined at `movement_consts.go:45`). Both `*Player` and `*Npc` implement it. NPC-side test at `npc_masks_test.go:255-260` uses pattern `n.target = other` where `other = newTestNpc(2)`. Player-side mirrors: `p.target = newTestPlayer(t)` or `p.target = newTestNpc(1)`. The self-sentinel pattern `p.target = p` (used at `handler_opheld_test.go` for ClearPendingAction sentinel) also satisfies the trailing-clear's `target != nil` skip condition. Implementer picks whichever is cleanest.

- **R7 (low) — Spec table line numbers drift between spec-write and impl-dispatch.** Standard `controller_preflight` protocol catches at plan-write. All cited line numbers verified at HEAD `fa9cb8a`.

---

### 10. Verification before close

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.
- `rg "chatMessage" modules/ pkg/` → 0 matches (field deleted, comment updated).
- `rg "NAI-72-N-RESETENTITY-PARTIAL" modules/ pkg/` → 0 matches (tracker retired in tick.go comment).
- `rg "p\.target == nil && p\.faceEntity" modules/world/player_masks.go` → 1 match (the new trailing-clear).
- `rg "p\.chatColour\s*=\s*-1" modules/world/player_masks.go` → 1 match (the new chat reset, in ResetMasks not Chat).
- `git show HEAD --stat` matches stated bundle scope: 4 files touched (`modules/world/player_masks.go`, `modules/world/player_masks_test.go`, `modules/world/player.go`, `modules/world/tick.go`); no stray worktree writes per `feedback_subagent_wt_path.md`.
- `TestResetMasksClearsEphemerals` (`player_masks_test.go:72-98`) continues green; doc-comment at line 91 updated per R5 disposition.
- **Smoke handoff**: user re-tests "speak with RuneScape Guide → walk away → confirm player face-direction releases (player no longer faces NPC after taking a step)". Smoke binds the (β) trailing-clear fix. Per `cascade_theory_smoke_binding`: if symptom persists, theory was under-attribution; investigate whether `p.target` is being nulled at the expected point in the chat-end flow. If symptom releases, NAI-108 closes with smoke confirmation.

---

### 11. Notes

- **Pattern**: NAI-108 is the first audit-style standard sub-spec since NAI-99 (multi-tile Loc footprint). It deliberately bundles the smoke-driven β fix with the broader audit deliverable per option (II) of the brainstorm. The audit table is the load-bearing scope deliverable; the code changes are deliberately small to keep TDD cycles focused.
- **NAI-67-D-PLAYER-UNFOCUS-DEFERRED stays open**: the orientation-reset-to-south behavior on respawn is genuinely blocked on a Player respawn/death sub-spec that hasn't been started. NAI-108 does not pull it forward.
- **Adjacent audit pattern**: This sub-spec establishes the per-class reset-method audit pattern. If similar smoke pressure surfaces on `Npc.ResetMasks` (e.g. NPC face issues post-target-clear), reuse the audit table format. The NPC-side audit is implicit-passing today (NAI-13/14 already covered the trailing-clear) but the (δ)-handled-elsewhere coverage may be similarly incomplete.
- **Cross-language reference**: TS source canonical path is `LostCityRS/Engine-TS` per `ts_source_canonical_path.md`. All TS line citations in the audit table are at HEAD of that repo as of 2026-05-05.
- **Memory entries to consider at close**:
  - Reinforce `dead_api_polish.md` with the `chatMessage` retire as a new exemplar (declared field, never read or written, surfaced via cross-class audit rather than at consumer add).
  - Possible new entry: "audit-style sub-spec pattern" — when smoke surfaces ONE symptom in a per-tick reset method, audit the WHOLE method against TS in the same spec; bundle load-bearing fix with audit doc; open per-gap follow-ups for genuine divergences.
- **Subagent dispatch posture**: Per `execution_mode_default.md`, dispatch via `superpowers:subagent-driven-development` with the writing-plans output. Per `superpowers_code_reviewer_model`, end-of-bundle review runs on Sonnet (or smaller).
