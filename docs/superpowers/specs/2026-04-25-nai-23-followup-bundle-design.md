# NAI-23 — follow-up bundle (tracker hygiene + checkNotBusy + checkNotTooStrong + NumberNotNull fidelity sweep)

- **Sub-spec**: NAI-23
- **Date**: 2026-04-25
- **Scope label**: B (logical-grouping follow-up bundle — `modules/world` + `pkg/script` + memory file; ~120 LOC production + ~330 LOC tests across 4 bundles; closes 2 open huntPlayers deferred filters (completes the NAI-8 deferred filter list); closes the NumberNotNull fidelity tracker scoped to 3 handler files; marks 3 stale memory-tracker entries Resolved; introduces 0 new deviations; net deviation count 14 → 14)
- **Predecessors**: NAI-22 (follow-up bundle) — last on `main` as `5ea8760`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

Four actionable items land in NAI-23:

1. **Tracker hygiene (Bundle 1).** Pre-flight against HEAD `5ea8760` discovered three stale entries in `nai_followups.md` whose underlying work has already shipped but were never marked Resolved:
   - **Line 786** (`Stale *Npc.typ snapshot after changetype`) — Resolved by NAI-19 Task 3 commit `8e94b29` ("changeTypeImpl refreshes n.typ snapshot on both paths"). Cross-referenced as resolved at line 1297-1300 of the same file but the primary entry was never updated.
   - **Line 1272** (`Promote n.size snapshot to LoS-path reads`) — Resolved by NAI-21 Bundle 1 commit `ed2f432` ("LoS snapshot LoS-path completion"). The commit message explicitly says "closes NAI-20 deferred follow-up" but the tracker entry stayed open.
   - **Line 244 cross-ref** (`Scope note: npc_changetype duration wiring status — now unassigned`) — primary entry at line 137 marks Resolved by NAI-16 Task 2; cross-ref still says "now unassigned." Stale by inheritance.

   Two of the three were caught by `controller_preflight` discipline at NAI-23 spec-write — exactly the case `spec_followup_tracker_freshness` warns about. Marking them Resolved clears the tracker for the next NAI-N spec author.

2. **checkNotBusy huntPlayers filter (Bundle 2).** Currently deferred at `npc_hunt.go:102` ("checkNotBusy (TS:931-933) — no Player.Busy()"). Pre-flight confirms both predicates TS `player.busy()` checks already exist in goscape: `Player.delayed bool` (`player.go:100`) and `Player.modalState int` (`player.go:175`) with constants `modalStateMain = 0x1` / `modalStateChat = 0x2` (`player.go:31-32`). The "needs Player.Busy() equivalent" framing dramatized what is actually a 1-line aggregator over already-ported state. `HuntType.CheckNotBusy bool` is already decoded by the unmarshaller (`hunttype.go:55, 116`), so live config can already opt-in to the filter — goscape was loading the flag and ignoring it.

3. **checkNotTooStrong huntPlayers filter (Bundle 3).** Currently deferred at `npc_hunt.go:103` ("checkNotTooStrong (TS:939-941) — wilderness + combat-level"). Pre-flight confirms TS `player.isInWilderness()` (`Player.ts:2082-2090`) is a hardcoded coord-range check (no map-zone lookup or metadata), and the supporting fields all exist: `HuntCheckNotTooStrongOutsideWilderness` (`hunttype.go:38`), `HuntType.CheckNotTooStrong int` (`hunttype.go:54`), `Player.combatLevel int` (`player.go:121`), `NpcType.VisLevel int` (`npctype.go:154`). The "needs wilderness detection" framing dramatized what is a 2-rect coord comparison. Together with Bundle 2, this **completes the NAI-8 deferred filter list** — every TS huntPlayers filter is then ported.

4. **NumberNotNull fidelity sweep, scoped to handlers_npc.go + handlers_inv.go + handlers_interface.go (Bundle 4).** The "From NAI-2" tracker entry at `nai_followups.md:29-54` says "Consider scoping a future fidelity-audit sub-spec that sweeps all opcode handlers for NumberNotNull-equivalent gates." NAI-20 Task 4 closed three NPC handlers as a starter; the broader sweep remains open. Pre-flight audit found 351 raw `popInt` calls across 14 handler files vs only 19 already wrapped with `checkNotNull` — the helper exists at `pkg/script/handlers_player.go:~70` but has been applied piecemeal. Bundle 4 audits the three highest-density files where TS counterparts most consistently wrap with `NumberNotNull`. handlers_config.go and handlers_number.go are explicitly out of scope (config-ID reads have weaker fidelity asymmetry; arithmetic operators don't use the -1 sentinel).

The four items cluster naturally by content type (memory-only / production filter / production filter / handler-sweep audit). Bundle 1 is housekeeping (memory-file edits, no production touch); Bundle 4's audit shape is parallel-friendly (one implementer subagent per handler file). Bundles 2 and 3 each warrant full review (production-behavior changes); Bundle 1 hits compressed-cadence and Bundle 4 is reviewed per-file with a final cross-file ordering sweep.

## Tech stack

- Go 1.26+
- Existing packages touched:
  - `modules/world/player.go` (Bundle 2: `Busy() bool` method ~5 LOC; Bundle 3: `IsInWilderness() bool` method ~12 LOC)
  - `modules/world/npc_hunt.go` (Bundle 2: ~5 LOC checkNotBusy filter + comment update; Bundle 3: ~6 LOC checkNotTooStrong filter + comment update)
  - `pkg/script/handlers_npc.go` (Bundle 4: per-handler `checkNotNull` wraps — count determined by per-file audit)
  - `pkg/script/handlers_inv.go` (Bundle 4: per-handler `checkNotNull` wraps)
  - `pkg/script/handlers_interface.go` (Bundle 4: per-handler `checkNotNull` wraps)
- Test files touched:
  - `modules/world/player_test.go` or `modules/world/player_script_test.go` (Bundle 2: Busy() tests; Bundle 3: IsInWilderness() tests)
  - `modules/world/npc_hunt_test.go` (Bundle 2: checkNotBusy hunt-integration tests; Bundle 3: checkNotTooStrong hunt-integration tests)
  - `pkg/script/handlers_npc_test.go` (Bundle 4: per-handler null-pin tests)
  - `pkg/script/handlers_inv_test.go` (Bundle 4: per-handler null-pin tests)
  - `pkg/script/handlers_interface_test.go` (Bundle 4: per-handler null-pin tests)
- Memory file:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (Bundle 1: 3 entry retags)
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/spec_followup_tracker_freshness.md` (Bundle 1: append a corroborating instance)
- No new files in production packages.

## Scope

### Bundle 1 — Tracker hygiene

**Goal**: Mark three stale entries in `nai_followups.md` Resolved with closure attribution; refresh the `spec_followup_tracker_freshness` memory with NAI-23's catches.

**Source**: NAI-23 spec-write pre-flight (this document).

#### Touch points

1. `nai_followups.md:786` ("Stale `*Npc.typ` snapshot after changetype (newly observable post-NAI-18)"):
   - Prepend the standard `**Resolved 2026-04-24 (NAI-19 Task 3, commit `8e94b29`)**` header followed by a one-paragraph closure description. Preserve the original deferral body under the existing `_Original deferral body (preserved for historical context):_` separator.
   - Closure description: `n.lookupType(newType)` is now lifted outside the `if reset` block in `(*Npc).changeTypeImpl` and the result is unconditionally assigned to `n.typ` (`npc_masks.go:68-69`). Both CHANGETYPE and KEEPALL paths now refresh the snapshot; `n.typ.X` reads after a changetype are no longer stale.

2. `nai_followups.md:1272` ("Promote `n.size` snapshot to LoS-path reads (`inApproachDistance`, `approachEntitySize`)"):
   - Prepend `**Resolved 2026-04-25 (NAI-21 Bundle 1, commit `ed2f432`)**` followed by closure description. Preserve original body.
   - Closure description: `(*Npc).inApproachDistance` reads `n.size` for `selfSize` (`npc_interaction.go:581`); `approachEntitySize` reads `t.size` for the `*Npc` branch (`npc_interaction.go:532`). Two regression tests landed: `TestInApproachDistanceUsesSelfSizeSnapshotNotTyp` and `TestApproachEntitySizeUsesNpcSizeSnapshotNotTyp` (dual-pin per `ts_asymmetry_dual_pin` memory).

3. `nai_followups.md:244` ("Scope note: `npc_changetype` duration wiring status"):
   - Edit the body to replace the stale "now unassigned" sentence with `**Resolved 2026-04-23 (NAI-16 Task 2)**` cross-reference. The "From NAI-5" primary entry at line 137 is already marked Resolved; this entry is a cross-ref that needs the same status.

4. `spec_followup_tracker_freshness.md` (memory):
   - Append a "Triggered by" addendum noting NAI-23 spec-write caught two more stale primary entries (the lines 786 and 1272 cases) on top of the originating NAI-21 Bundle 2 catch. Reinforces the spec-write-time grep-and-Read discipline.
   - Optional: add a one-line note that NAI-N close commits should include tracker updates as a side-action, complementing the `close_commit_memory_trailer` memory's git-trailer discipline.

#### Tests

None — memory-file edits only.

#### Deviation impact

0 — no production code change; no deviation tag retired or introduced.

#### TS-fidelity gate

Not applicable.

#### Compressed-cadence eligibility

Yes (≤15 LOC equivalent across memory file). Combined spec+plan; informal review absorbed into the close commit.

---

### Bundle 2 — checkNotBusy huntPlayers filter + Player.Busy() aggregator

**Goal**: Activate the `checkNotBusy` filter in `(*Npc).huntPlayers` by adding a `Player.Busy() bool` method aggregating the two predicates TS `player.busy()` checks. Closes one of the two remaining NAI-8 deferred filters.

**Source**: `npc_hunt.go:102` deferred TODO; tracker at `nai_followups.md:301, 337-340`.

#### TS reference

- **`Player.busy()`** at `Engine-TS/src/engine/entity/Player.ts:801-803`:
  ```typescript
  busy(): boolean {
      return this.delayed || this.containsModalInterface();
  }
  ```
  with `containsModalInterface()` at `Player.ts:796-799`:
  ```typescript
  containsModalInterface(): boolean {
      return (this.modalState & (ModalState.MAIN | ModalState.CHAT)) !== ModalState.NONE;
  }
  ```

- **Filter call site** at `Engine-TS/src/engine/entity/Npc.ts:931-933`:
  ```typescript
  if (hunt.checkNotBusy && player.busy()) {
      continue;
  }
  ```

#### Pre-flight verification

Verified at HEAD `5ea8760`:
- `Player.delayed bool` exists at `modules/world/player.go:100`.
- `Player.modalState int` exists at `modules/world/player.go:175`.
- `modalStateMain = 0x1`, `modalStateChat = 0x2` constants at `player.go:31-32`.
- `HuntType.CheckNotBusy bool` exists at `pkg/objtype/hunttype.go:55`; unmarshaller sets it at line 116 from a config bit-flag.
- No existing `Player.Busy()` method to collide with (`grep -n "func.*Player.*\bBusy\b"` returns empty).
- Insertion point in `npc_hunt.go`: line 138 (just before `// checkAfk (TS:935-937)`). Filter order: range/level → **checkNotBusy (NEW)** → checkAfk → CheckVis (inlined) → ... — TS-faithful modulo the existing CheckVis-inlining divergence accepted in NAI-12.

#### Code change shape

**Production: `Player.Busy()` method** — add to `modules/world/player.go` (placement: near other simple state predicates; ctor doc-comment NOT required — single method):

```go
// Busy returns true when the player cannot accept new interactions —
// either delayed (suspended by script delay) or has a main/chat modal
// open. Mirrors TS Player.busy() at Engine-TS/.../Player.ts:801-803
// (which composes containsModalInterface at Player.ts:796-799).
func (p *Player) Busy() bool {
    return p.delayed || p.modalState&(modalStateMain|modalStateChat) != 0
}
```

**Production: filter activation** — `modules/world/npc_hunt.go`, insert before line 138:

```go
// checkNotBusy (TS:931-933): skip players whose state cannot accept
// a hunt interaction (delayed or main/chat modal open).
if hunt.CheckNotBusy && p.Busy() {
    continue
}
```

**Doc-comment update** — `npc_hunt.go:88-103` filter-coverage comment block:
- Move the `checkNotBusy` line out of "Filters DEFERRED" into "Filter coverage" with the (NAI-23, TS:931-933) attribution.

#### Tests

In `modules/world/player_test.go` (or `player_script_test.go` — pick whichever already hosts simple-state-predicate tests; if neither does, prefer `player_test.go`):

1. **`TestPlayerBusyNotDelayedNoModal`** — fresh player; assert `p.Busy() == false`.
2. **`TestPlayerBusyDelayedOnly`** — `p.delayed = true`; assert `p.Busy() == true`.
3. **`TestPlayerBusyModalMainOnly`** — `p.modalState = modalStateMain`; assert `p.Busy() == true`.
4. **`TestPlayerBusyModalChatOnly`** — `p.modalState = modalStateChat`; assert `p.Busy() == true`.
5. **`TestPlayerBusyModalSideOnlyNotBusy`** — `p.modalState = modalStateSide` (NOT main, NOT chat); assert `p.Busy() == false`. Pins TS-fidelity for the precise mask: side modals do NOT make the player busy per `Player.ts:796-799`.
6. **`TestPlayerBusyDelayedAndModalCombined`** — both set; assert `p.Busy() == true`.

In `modules/world/npc_hunt_test.go` — extend the existing CheckAfk/CheckInv pattern:

7. **`TestHuntPlayersCheckNotBusyFiltersBusyPlayer`** — two players in range, one busy (`p.delayed = true`); `hunt.CheckNotBusy = true`; assert busy player filtered out, non-busy passes.
8. **`TestHuntPlayersCheckNotBusyDisabled`** — busy player in range; `hunt.CheckNotBusy = false`; assert busy player passes (filter only fires when enabled). Pins the `hunt.CheckNotBusy &&` short-circuit semantics.

#### Deviation impact

0 — TS-faithful port of an already-ported predicate set. No new deviation tags introduced; no existing tags retired (this isn't a tagged deviation, just a tracker-deferred filter).

#### TS-fidelity gate

`Player.Busy()` mirrors TS `player.busy()` exactly: `delayed || (modalState & (MAIN|CHAT)) != 0`. Modal mask intentionally excludes the SIDE bit per TS `containsModalInterface`. Filter ordering preserves TS Npc.ts:931 → 935 sequence with the existing CheckVis-inlining divergence (NAI-12) untouched.

---

### Bundle 3 — checkNotTooStrong huntPlayers filter + Player.IsInWilderness()

**Goal**: Activate the `checkNotTooStrong` filter in `(*Npc).huntPlayers` by adding a `Player.IsInWilderness() bool` method with TS-exact coord bounds. Closes the second of the two remaining NAI-8 deferred filters; **completes the entire NAI-8 deferred filter list**.

**Source**: `npc_hunt.go:103` deferred TODO; tracker at `nai_followups.md:302-303, 342-346`.

#### TS reference

- **`Player.isInWilderness()`** at `Engine-TS/src/engine/entity/Player.ts:2082-2090`:
  ```typescript
  isInWilderness(): boolean {
      if (this.x >= 2944 && this.x < 3392 && this.z >= 3520 && this.z < 6400) {
          return true;
      } else if (this.x >= 2944 && this.x < 3392 && this.z >= 9920 && this.z < 12800) {
          return true;
      } else {
          return false;
      }
  }
  ```

- **Filter call site** at `Engine-TS/src/engine/entity/Npc.ts:939-941`:
  ```typescript
  if (hunt.checkNotTooStrong === HuntCheckNotTooStrong.OUTSIDE_WILDERNESS &&
      !player.isInWilderness() &&
      player.combatLevel > type.vislevel * 2) {
      continue;
  }
  ```

#### Pre-flight verification

Verified at HEAD `5ea8760`:
- `HuntCheckNotTooStrongOutsideWilderness = 1` enum value at `pkg/objtype/hunttype.go:38`.
- `HuntType.CheckNotTooStrong int` field at `hunttype.go:54` (defaults to `HuntCheckNotTooStrongOff`).
- `Player.combatLevel int` field at `modules/world/player.go:121`.
- `NpcType.VisLevel int` field at `pkg/objtype/npctype.go:154`.
- No existing `Player.IsInWilderness()` method or wilderness-related code anywhere in goscape (`grep -rn -i "wilderness\|wildy\|isInWilderness"` returns empty).
- Insertion point in `npc_hunt.go`: line 159 (just before `// Outer combat guard — TS:942`). Filter order: ... → CheckVis (inlined) → **checkNotTooStrong (NEW)** → outer combat guard → ... — TS-faithful modulo the existing CheckVis-inlining divergence.

#### Code change shape

**Production: `Player.IsInWilderness()` method** — add to `modules/world/player.go` (placement: near `Busy()` from Bundle 2, since both are simple state predicates):

```go
// IsInWilderness returns true when the player is inside one of the two
// hardcoded wilderness rectangles. Mirrors TS Player.isInWilderness()
// at Engine-TS/.../Player.ts:2082-2090.
//
// South wilderness: x in [2944, 3392), z in [3520, 6400).
// North wilderness: x in [2944, 3392), z in [9920, 12800).
//
// Bounds are inclusive on the lower edge and exclusive on the upper —
// preserve verbatim: `<=` would shift the boundary by one tile vs TS.
func (p *Player) IsInWilderness() bool {
    if p.x >= 2944 && p.x < 3392 && p.z >= 3520 && p.z < 6400 {
        return true
    }
    if p.x >= 2944 && p.x < 3392 && p.z >= 9920 && p.z < 12800 {
        return true
    }
    return false
}
```

**Production: filter activation** — `modules/world/npc_hunt.go`, insert before line 159:

```go
// checkNotTooStrong (TS:939-941): skip players whose combat level is
// more than 2x the NPC's vislevel when they're OUTSIDE the wilderness
// (the wilderness disables this protection). Filter only applies when
// CheckNotTooStrong is set to OutsideWilderness; Off → filter skipped.
if hunt.CheckNotTooStrong == objtype.HuntCheckNotTooStrongOutsideWilderness &&
    !p.IsInWilderness() &&
    p.combatLevel > n.typ.VisLevel*2 {
    continue
}
```

**Defensive nil-guard:** if `n.typ` can be nil at this call site, the filter must skip-pass on nil. Pre-flight at spec-write found `huntPlayers` already runs in contexts where `n.typ` is reliably non-nil (the function is only entered after `n.typ` is set in `NewNpc`; `processNpcHunt` runs the per-NPC loop after type-binding). No new nil-guard needed; if the implementer subagent's TDD round flags any test that runs `huntPlayers` with `n.typ == nil`, escalate to the controller for a guard decision rather than inlining one.

**Doc-comment update** — `npc_hunt.go:88-103` filter-coverage comment block:
- Move the `checkNotTooStrong` line out of "Filters DEFERRED" into "Filter coverage" with the (NAI-23, TS:939-941) attribution.
- Update the "Filters DEFERRED" block comment to read **"All NAI-8 deferred filters now ported."** (or equivalent) — this bundle closes the deferred-filter list.

#### Tests

In `modules/world/player_test.go` (or wherever Bundle 2's Busy tests landed):

1. **`TestPlayerIsInWildernessSouthRectInside`** — `p.x = 3000, p.z = 5000`; assert `true`.
2. **`TestPlayerIsInWildernessNorthRectInside`** — `p.x = 3000, p.z = 11000`; assert `true`.
3. **`TestPlayerIsInWildernessOutsideAllRects`** — `p.x = 3000, p.z = 3500` (z=3500 below south rect's 3520); assert `false`.
4. **Boundary pins (4 sub-tests, table-driven if cleanest):**
   - `p.x = 2944, p.z = 3520` → `true` (lower-edge inclusive).
   - `p.x = 2943, p.z = 5000` → `false` (just below lower x bound).
   - `p.x = 3392, p.z = 5000` → `false` (upper-edge exclusive).
   - `p.x = 3000, p.z = 6400` → `false` (upper-edge exclusive, south rect).
   - These pin the TS `<` semantics — a future "fix" to `<=` would flip them red.

In `modules/world/npc_hunt_test.go`:

5. **`TestHuntPlayersCheckNotTooStrongFiltersStrongPlayerOutsideWilderness`** — player in range OUTSIDE wilderness, `combatLevel = 100`, `npcType.VisLevel = 30` (player > 2×vislevel = 60); `hunt.CheckNotTooStrong = OutsideWilderness`; assert player filtered out.
6. **`TestHuntPlayersCheckNotTooStrongIgnoresStrongPlayerInWilderness`** — same combatLevel/vislevel ratio, but player is INSIDE wilderness (e.g., x=3000, z=5000); assert player passes (wilderness disables the protection).
7. **`TestHuntPlayersCheckNotTooStrongAllowsWeakPlayer`** — `combatLevel = 50`, `vislevel = 30` (player NOT > 60); player outside wilderness; assert player passes.
8. **`TestHuntPlayersCheckNotTooStrongDisabled`** — strong player outside wilderness; `hunt.CheckNotTooStrong = Off`; assert player passes (filter only fires when enabled).
9. **`TestHuntPlayersCheckNotTooStrongBoundaryComparison`** — combatLevel exactly equal to 2×vislevel (e.g., 60 vs 30); assert player passes (TS uses `>`, not `>=`).

#### Deviation impact

0 — TS-faithful port of a coord-range check + filter activation. No new deviation tags; closes 0 tagged deviations (NAI-8 deferred filter is not tagged). The "complete the NAI-8 deferred filter list" milestone is a tracker-only event.

#### TS-fidelity gate

`IsInWilderness()` mirrors TS bounds verbatim including the `>=` lower / `<` upper asymmetry. Filter ordering preserves TS Npc.ts:939-941 placement (between checkAfk-position and outer-combat-guard at 942). The combatLevel comparison uses `>` not `>=` per TS.

---

### Bundle 4 — NumberNotNull fidelity sweep (handlers_npc.go + handlers_inv.go + handlers_interface.go)

**Goal**: Audit every `popInt` / `PopInt` call in three opcode-handler files and add `checkNotNull` wraps where the TS counterpart applies `NumberNotNull`. Adds a per-handler null-pin test for each newly wrapped handler. Closes the NumberNotNull tracker entry's "audit sub-spec" ask, scoped to the three highest-density files.

**Source**: `nai_followups.md:29-54` (From NAI-2 unassigned fidelity audit). NAI-20 Task 4 closed three NPC handlers as a starter; this bundle continues and bounds the work to three files.

#### Pre-flight verification

Verified at HEAD `5ea8760`:
- `checkNotNull(v int, op string) error` defined at `pkg/script/handlers_player.go:~70`. Sentinel: `-1`. Returns formatted error with the opcode name.
- 351 raw `popInt` / `PopInt` calls across 14 handler files; 19 already wrapped — only 2 of 14 files contain any wraps (handlers_npc.go: 6; handlers_player.go: 5; per the audit). Audit-pass methodology confirmed compatible with parallel implementer dispatch.
- TS counterpart files at `Engine-TS/src/engine/script/handlers/`. Each goscape handler file maps to a same-named TS file (e.g., `handlers_inv.go` ↔ `InvOps.ts`).

#### Per-handler decision rubric

Each handler in scope is audited with the following sequence:

1. **Read the TS counterpart for this opcode.** Locate the matching TS handler in `Engine-TS/src/engine/script/handlers/`. Identify whether the popped value is wrapped with `check(state.popInt(), NumberNotNull)`.
2. **If TS wraps with `NumberNotNull`** → goscape adds `checkNotNull(v, "OP_NAME")`-shaped wrap at the same logical position (immediately after the popInt). Treat as IN-scope for the wrap.
3. **If TS does NOT wrap** → goscape leaves the popInt raw. The implementer subagent records the decision in the per-handler comment as "TS popInt is not NumberNotNull-wrapped — preserved as-is."
4. **If the popped value is semantically signed** (e.g., a coord delta, a search-relative offset, an arithmetic operand) → SKIP regardless of TS, because `-1` is a legitimate value for these. The implementer subagent records "signed value; -1 sentinel does not apply" rationale.
5. **If unclear** → escalate to the controller with the TS line ref + the goscape handler name. Do NOT guess.

This rubric applies uniformly across all three files. The implementer subagent dispatched for each file works through every popInt in that file, applying the rubric per popInt site, and reports a per-handler audit table:

```
| Handler | popInt sites | TS NumberNotNull-wrapped? | Action | Rationale (if skip) |
```

#### Code change shape (per wrapped handler)

**Production wrap pattern.** For each handler that the audit decides to wrap, transform:

```go
v := state.PopInt()
// ... use v ...
```

into:

```go
v := state.PopInt()
if err := checkNotNull(v, "OP_NAME"); err != nil {
    return err
}
// ... use v ...
```

`OP_NAME` matches the existing `checkNotNull` consumer pattern (e.g., `"npc_settimer"` for `handleNpcSetTimer`). The implementer subagent reads existing wrapped handlers as templates (handlers_npc.go has 6, handlers_player.go has 5) and reuses the casing convention.

When a handler pops multiple ints, each gets its own wrap if the TS counterpart wraps each. Multi-wrap handlers must keep the wraps in popInt order.

#### Test pattern (per wrapped handler)

For each handler newly wrapped, add a null-pin test in the corresponding `_test.go` file. Pattern:

```go
func TestHandle<HandlerName>RejectsNullInput(t *testing.T) {
    state := newTestScriptStateWithPushed(-1) // or however the file's test fixtures shape pushed-state
    err := handle<HandlerName>(state)
    if err == nil {
        t.Fatal("handle<HandlerName>: got nil err, want NumberNotNull error")
    }
    if !strings.Contains(err.Error(), "<op_name>") {
        t.Errorf("handle<HandlerName>: err = %q, want contains %q", err.Error(), "<op_name>")
    }
}
```

The exact test fixture builder depends on each file's existing test scaffolding — implementer subagent uses the file's existing test patterns. If no scaffolding exists, prefer table-driven tests grouping all the file's null-pins to keep test-LOC sub-linear in handler count.

When multiple ints are wrapped in one handler, each int gets its own null-pin sub-case (table-driven). Pin only one int's null at a time; the other ints stay valid so the rejection is attributable to the specific wrap.

#### Per-file scope and cadence

Each file is dispatched as one implementer subagent task in the plan, in the order:

1. **handlers_npc.go** (Task 4a): audit pass on the ~18-23 unwrapped candidates. Existing 6 wraps stay. The implementer reuses the wrap conventions visible in this file.
2. **handlers_inv.go** (Task 4b): audit pass on the ~15-18 candidate handlers. No existing wraps in this file — implementer establishes the convention by reading handlers_npc.go's templates.
3. **handlers_interface.go** (Task 4c): audit pass on the ~12-15 candidate handlers. No existing wraps in this file.

Each implementer subagent:
- Reports its per-handler audit table to the controller.
- Lands one commit per file with all the file's wraps + tests bundled (matches the `compressed_cadence` for per-file work; one file = one commit avoids cross-file review boundaries).
- Files are **independent** (no shared state, no cross-file imports) — Tasks 4a/4b/4c can dispatch in parallel per `dispatching-parallel-agents` discipline.

#### Out-of-file handlers

Explicitly out of NAI-23 scope:
- `pkg/script/handlers_config.go` — config-ID reads have weaker fidelity asymmetry; TS InvOps wraps consistently but ConfigOps less so. Defer to a future sweep.
- `pkg/script/handlers_number.go` — arithmetic operators do not use `-1` as a null sentinel. False-positive territory; explicitly skip.
- `pkg/script/handlers_player.go` — already partially wrapped (5 wraps); deferred from this bundle to keep Bundle 4 scope at three audit-pass files. Future sweep can complete.
- All other handler files (~9 remaining) — out of scope; future sweeps each take a file.

The tracker at `nai_followups.md:29-54` is marked `**Partial resolution 2026-04-25 (NAI-23 Bundle 4)**` with the per-file completion list. Future sweeps each cite NAI-23 as a precedent and incrementally close the remaining files.

#### Deviation impact

0 — closing fidelity gaps that are documented in TS but not enforced in goscape. No new tags introduced; the NumberNotNull tracker entry is not a tagged deviation, only a tracker fidelity item.

#### TS-fidelity gate

For each wrapped handler, the `checkNotNull` call applies the same `-1` sentinel rejection that TS's `NumberNotNull` does. Multi-wrap handlers preserve TS's per-popInt wrap-order. Unwrapped handlers (where TS doesn't wrap) explicitly preserve TS's tolerance of negatives (signed-value handlers).

#### Compressed-cadence eligibility

Per-file basis only. Each Task 4a/4b/4c is a per-file audit-pass — too large for compressed-cadence as a whole, but each follows the same fixed shape so the plan codifies the audit pattern once and dispatches three implementers against it.

---

## Out of scope

- **NAI-19-D1 (Zone abstraction track).** The structural deferral at `npc_registry.go:64,149` (`zone.enter`/`zone.leave` omitted) is a multi-spec design effort, not a follow-up bundle. Stays out.
- **Other NAI-11 deferred items.** SMART pathfinding branch in `pathToTarget`, reach helpers (reachedEntity/reachedLoc/reachedObj), focus() instant-flag wire-protocol, `Npc.interacted` field — each is its own substantial sub-spec. Stays out.
- **`pkg/script/handlers_player.go`, `handlers_config.go`, `handlers_number.go`** NumberNotNull sweeps. Out per Bundle 4 scope discussion above.
- **All non-stale tracker entries** in `nai_followups.md`. Bundle 1 only marks the three confirmed-stale entries; other "Resolved" annotations remain as-is.
- **Tagged deviation retirement.** NAI-22 close already swept the active deviation-tag references; no follow-up retire-grep is needed for NAI-23.
- **`huntPlayers` filter ordering refactor** to lift CheckVis to the outer iterator (matching TS architecture). Existing CheckVis-inlining divergence is accepted (NAI-12); not in scope.
- **Smoke-test coverage** for the new filters via Java client. Per `smoke_test_server_handoff` memory, smoke is user-launched and out-of-band. Defer to optional post-close validation if user requests.

## Filter ordering invariant (post-NAI-23)

The full huntPlayers filter chain after NAI-23 lands:

1. range/level (always)
2. **checkNotBusy (NAI-23, TS:931-933)** — NEW
3. checkAfk (NAI-8, TS:935-937)
4. CheckVis LoS/LoW (NAI-12, TS per ScriptIterators.ts; goscape inlines — accepted divergence)
5. **checkNotTooStrong (NAI-23, TS:939-941)** — NEW
6. Outer combat guard (NAI-15, TS:942)
   1. checkNotCombat (NAI-15, TS:943-945)
   2. checkNotCombatSelf (NAI-16, TS:946-948)
7. checkVars (NAI-15, TS:950-957)
8. checkInv (NAI-22, TS:959-969)

This matches TS Npc.ts:921-973 modulo the CheckVis-inlining divergence (NAI-12). Every TS huntPlayers filter is now ported; the "Filters DEFERRED" comment block in `npc_hunt.go` is retired.

## Test strategy

| Bundle | Pattern | Count |
|---|---|---|
| 1 | None (memory-only) | 0 |
| 2 | Player.Busy() unit (6) + huntPlayers integration (2) | 8 tests |
| 3 | Player.IsInWilderness() unit + boundary (~7) + huntPlayers integration (5) | ~12 tests |
| 4 | Per-handler null-pin (1 per wrapped handler; table-driven where ≥3 in same file) | ~45-56 tests |

Per `plan_test_coverage_crosscheck` memory: every wrap landed in Bundle 4 has a null-pin test; the plan lists the per-task expected-test-count for each file so the reviewer can verify implementers didn't drop tests silently.

## True-to-TS gate (per bundle)

- **Bundle 1**: N/A (memory-only).
- **Bundle 2**: `Player.Busy()` is exact composition of `delayed || (modalState & (MAIN|CHAT)) != 0`. Filter call site `hunt.CheckNotBusy && p.Busy()` mirrors TS Npc.ts:931 verbatim. No divergence.
- **Bundle 3**: `IsInWilderness()` bounds are TS-verbatim including `>=`/`<` asymmetry; the filter's `combatLevel > vislevel*2` uses `>` not `>=`. No divergence.
- **Bundle 4**: each wrapped handler matches TS's NumberNotNull placement. Skipped handlers (signed values; TS doesn't wrap) preserve TS's tolerance. Per-handler audit table is the gate evidence.

## Risks

- **Bundle 4 scope blow-up.** Risk: per-file audit reveals more candidates than the ~45-56 estimate, or requires deeper TS reading per handler than the rubric absorbs. Mitigation: rubric explicitly allows the implementer subagent to escalate "unclear" cases instead of guessing. If a file's audit balloons past ~25 handlers, the controller can carve it into a follow-up sub-spec rather than letting Bundle 4 grow.
- **Bundle 3 nil-guard ambiguity.** Risk: `n.typ` could be nil at the filter site in some edge case the spec didn't catch. Mitigation: implementer subagent escalates to controller if a TDD round surfaces a nil-deref test path; do NOT inline a guard without controller decision (per `plan_grep_helper_patterns` memory — "grep helper patterns before prescribing inline boilerplate").
- **Bundle 2 modal-mask drift.** Risk: future modal-state work might add a new bit (e.g., `modalStateOverlay`) and forget to update `Busy()`. Mitigation: `TestPlayerBusyModalSideOnlyNotBusy` explicitly pins that side does NOT contribute; that test would catch a "broaden the mask" mistake immediately. The constants live in the same file as `Busy()`, so reviewers see them together.
- **Bundle 1 memory edits not version-controlled.** Risk: the memory file lives outside the repo; tracker-hygiene work has no git audit trail in the close commit. Mitigation: spec attributes each Resolved-marker with a commit SHA so the audit trail is in the memory file itself, not just git.

## Implementation cadence

Per `runescript_cadence` memory:

1. **Spec** (this document) → user review.
2. **Plan** (next: `superpowers:writing-plans` skill) → per-task instructions, parallel-dispatch markings for Bundle 4's three files.
3. **Subagent-driven TDD** per `subagent-driven-development`:
   - Bundle 1: single-task housekeeping (compressed cadence — no separate plan task; absorbed into close-commit work).
   - Bundle 2: Task 2 (one implementer; full TDD round).
   - Bundle 3: Task 3 (one implementer; full TDD round).
   - Bundle 4: Tasks 4a/4b/4c (three parallel implementers per `dispatching-parallel-agents`; each runs TDD on their file).
4. **Two-stage review** per `runescript_cadence`:
   - Stage 1 (code quality): per-bundle review covering code-shape, idiom, helper reuse.
   - Stage 2 (TS fidelity): per-bundle re-read against TS source; Bundle 4's per-handler audit table is the evidence stream.
5. **Close commit** with `Closes memory:` trailer per `close_commit_memory_trailer` memory.

Bundles 2 and 3 each warrant full Stage-1 + Stage-2 review (production-behavior changes). Bundle 4's review is per-file: each implementer's audit table feeds into a single Stage-2 cross-file ordering sweep. Bundle 1 is pure housekeeping; review is informal.

## Deviation accounting

- **Active deviation count at NAI-22 close (5ea8760)**: 14.
- **Bundle 1**: 0 retired, 0 introduced.
- **Bundle 2**: 0 retired, 0 introduced.
- **Bundle 3**: 0 retired, 0 introduced.
- **Bundle 4**: 0 retired, 0 introduced.
- **Active deviation count at NAI-23 close**: 14.

Net: 14 → 14. The bundle closes tracker-deferred work and tracker entries, not tagged production deviations. The NAI-8 deferred filter list completion is a tracker milestone, not a deviation closure.
