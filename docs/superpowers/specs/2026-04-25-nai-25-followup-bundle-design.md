# NAI-25 — follow-up bundle (`(*Player).invListenOnCom` TS-faithfulness audit + handlers NumberNotNull audit-completion sweep)

- **Sub-spec**: NAI-25
- **Date**: 2026-04-25
- **Scope label**: B (logical-grouping follow-up bundle — `modules/world` (1 method body + 1 docstring + 1 test helper) + `pkg/script/handlers_*.go` audit sweep across 13 unaudited handler files; ~35-50 LOC production + ~80-150 LOC tests across 2 bundles; resolves From-NAI-24 `Source = -1` API-surface deferral and From-NAI-23 NumberNotNull-sweep tracker; introduces 0 new deviations by default; net deviation count 14 → 14)
- **Predecessors**: NAI-24 (follow-up bundle) — last on `main` as `20fb72a`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

Two adjacent fidelity items land in NAI-25, both surfaced post-NAI-24:

1. **`(*Player).invListenOnCom` TS-faithfulness audit (Bundle 1).** The From-NAI-24 tracker entry at `nai_followups.md:1471-1514` enumerates the post-Bundle-2 state where zero production callers pass `-1` to `InvListenOnCom` and frames the question as "retract the API surface or preserve as defensive future-use." Brainstorm-time investigation against TS source `Engine-TS/src/engine/entity/Player.ts:1441-1462` reframes the question: the `-1` API surface is **not** dead — its **write-side scope-rewrite was never ported**. TS `invListenOnCom` rewrites `source = -1` internally when `invType.scope === SCOPE_SHARED`; goscape's port at `modules/world/player.go:636-646` stores whatever source was passed without scope-checking. The `-1` branch on the read side at `:471-479` is correct — it has no live producer because the producing transformer (the scope rewrite) is missing. Bundle 1 ports the missing rewrite plus two adjacent TS-faithfulness divergences in the same method (early-out on invalid invType, same-type+same-com dedup), producing a line-by-line TS-faithful `(*Player).invListenOnCom`.

2. **NumberNotNull audit-completion sweep across remaining handler files (Bundle 2).** The From-NAI-23 tracker entry at `nai_followups.md:1409-1446` enumerates the unaudited handler files awaiting NumberNotNull audit: `handlers_loc.go`, `handlers_obj.go`, `handlers_db.go`, `handlers_string.go`, `handlers_dialog.go`, `handlers_timer.go`, `handlers_vars.go`, `handlers_array.go`, `handlers_lastinput*`, `handlers_debug.go`, `handlers_server.go`, `handlers_core.go`, plus `handlers_config.go` (NpcConfigOps subset). Brainstorm-time TS-density measurement: the three already-audited TS files (PlayerOps.ts, NpcOps.ts, InvOps.ts) account for 84/86 (97.7%) of all TS-side `NumberNotNull` invocations; remaining TS handler files (LocOps, ObjOps, DbOps, StringOps, ServerOps, CoreOps, DebugOps, NumberOps, StructOps, EnumOps, LocConfigOps, ObjConfigOps) total **0** NumberNotNull. NpcConfigOps.ts has **2**. The high-yield work is exhausted on a per-file basis. **However**, math reveals a hidden cross-file gap: PlayerOps.ts has 56 NumberNotNull; NAI-24 Bundle 1 audited 47 popInt sites in `handlers_player.go`; the 9-site delta lives in PlayerOps.ts opcodes that goscape dispatches from a different file. Group Y goscape files (`handlers_dialog.go`, `handlers_timer.go`, `handlers_lastinput_test.go`) are mapped to PlayerOps.ts via opcode-set membership (P_PAUSEBUTTON, LAST_*, CAM_*, STAFFMODLEVEL, UID for dialog; SETTIMER, SOFTTIMER, CLEARTIMER, CLEARSOFTTIMER, GETTIMER for timer). Bundle 2 sweeps all 13 unaudited goscape handler files, producing a per-file audit table per the NAI-23 Bundle 4 cadence. Files with non-zero net-new wraps get own-commits; confirm-zero files fold into a single rollup commit.

The two items cluster naturally: Bundle 1 closes the From-NAI-24 deferral via TS-faithful porting (not retraction); Bundle 2 closes the From-NAI-23 sweep target list. Bundles touch disjoint files (`modules/world/player.go` + `modules/world/player_inv_test.go` + `modules/world/server_test.go` + `pkg/script/active.go` for Bundle 1; `pkg/script/handlers_*.go` only for Bundle 2) — no inter-bundle dependencies — but **Approach A (sequential dispatch) was selected** so Bundle 2 dispatches only after Bundle 1's close commit lands.

## Tech stack

- Go 1.26+
- Existing packages touched:
  - `modules/world/player.go` (Bundle 1: `invListenOnCom` body — 3 missing TS branches; doc-comment narration update at `:627-635`)
  - `pkg/script/active.go` (Bundle 1: `ActivePlayer.InvListenOnCom` interface contract docstring at `:277-283` — no signature change)
  - `pkg/script/handlers_*.go` (Bundle 2: per-file `checkNotNull` wraps where audit table identifies WRAP rows)
- Test files touched:
  - `modules/world/player_inv_test.go` (Bundle 1: 4 new tests; existing `TestInvListenOnComReplacesExisting` docstring tightened)
  - `modules/world/server_test.go` (Bundle 1: new helper `newTestPlayerWithInvTypes(t, configs)` for scope-aware tests)
  - `pkg/script/handlers_*_test.go` (Bundle 2: per-WRAP `TestHandle<OpName>NullRejected` test)
- Memory file:
  - `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (NAI-25 close: Bundle 1 marks From-NAI-24 entry at `:1471-1514` Resolved; Bundle 2 marks From-NAI-23 entry at `:1409-1446` Resolved with per-file audit summary)
- No new files in production packages.

## Scope

### Bundle 1 — `(*Player).invListenOnCom` TS-faithfulness audit

**Goal**: Port the three missing TS-faithfulness divergences in `(*Player).invListenOnCom` to match `Engine-TS/src/engine/entity/Player.ts:1441-1462` line-by-line. Add four new tests pinning the new behaviors and a test helper for scope-aware fixtures. Update the `ActivePlayer.InvListenOnCom` interface contract docstring to narrate the rewrite. Resolve the From-NAI-24 tracker entry as "API surface not dead — write-side rewrite was missing; ported."

**Source**: NAI-24 close — From-NAI-24 tracker entry at `nai_followups.md:1471-1514`. Brainstorm reframing surfaced two additional in-scope divergences in the same method.

#### TS source canonical path

`/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts` (per `ts_source_canonical_path` memory). The TS method body at lines 1441-1462 is the line-by-line porting reference.

#### The three divergences

##### (α) Missing `inv === -1` early-out

**TS source** (`Player.ts:1442-1444`):
```ts
if (inv === -1) {
    return;
}
```
**Current goscape state**: missing. Defended upstream by `InvTypeValid` per NAI-23 Bundle 4b (handlers_inv.go validates `invType` via the typed validator before reaching `InvListenOnCom`), so the branch is practically unreachable from the script-side dispatch path. However, the internal-method-defense pattern is TS-faithful at trivial cost.

**Bundle 1 fix** (in `(*Player).invListenOnCom`):
```go
if invType == -1 {
    return
}
```

##### (β) Missing same-type+same-com dedup

**TS source** (`Player.ts:1446-1449`):
```ts
const sameTypeCom = this.invListeners.findIndex(l => l.type === inv && l.com === com);
if (sameTypeCom !== -1) {
    return;
}
```
**Current goscape state**: missing. goscape's map-keyed implementation unconditionally replaces every entry at `com`, **resetting `FirstSeen=true` on every call** — so a redundant `inv_transmit` re-sends the full inv update. TS preserves `FirstSeen=false` after the first dispatch by no-op'ing on same-{type,com}.

**Bundle 1 fix** (map-keyed adaptation):
```go
if existing, ok := p.invListeners[com]; ok && existing.Type == invType {
    return
}
```
The map-keyed lookup adapts the TS array-`findIndex` to goscape's `map[int]InventoryListener` data structure. Same pattern as the existing same-com-different-type splice (which the map naturally enforces via overwrite).

##### (γ) Missing scope-shared rewrite

**TS source** (`Player.ts:1456-1459`):
```ts
const invType = InvType.get(inv);
if (invType.scope === InvType.SCOPE_SHARED) {
    source = -1;
}
```
**Current goscape state**: missing. goscape stores raw `source` regardless of the inv-type's scope. Result: when a script registers an inv-listener on a SCOPE_SHARED inv-type via `inv_transmit`, the dispatch reader at `updateInvs` (`player.go:471-479`) routes to `Server.players[Source].invs[Type]` instead of the shared `Server.invs[Type]`. After NAI-24 Bundle 2's `-1` → `s.Self.UID()` fix, this divergence is the active fidelity bug.

**Bundle 1 fix**:
```go
if p.client != nil && p.client.server != nil && p.client.server.invTypes != nil {
    if cfg := p.client.server.invTypes.Configs[invType]; cfg != nil && cfg.Scope == objtype.InvTypeScopeShared {
        source = -1
    }
}
```

The lookup gracefully degrades when server/invTypes wiring is absent (existing `player_inv_test.go` direct-call tests). The nil-cfg guard handles `Configs[]` indexing where the type ID exceeds the configured range. The pattern mirrors the existing scope-aware lookup in `invLookupView.Get` at `modules/world/server_invs.go:26-32` — we don't extract a helper because the two callers serve different domains (Inv.Get vs listener-registration) and a one-liner inline is clearer than introducing a `(*Server) invScope(typeID int) int` helper for two callers.

#### Final method shape

After all three branches land, the method body (post-Bundle-1) looks like:

```go
func (p *Player) invListenOnCom(invType, com, source int) {
    if invType == -1 {
        return
    }
    if existing, ok := p.invListeners[com]; ok && existing.Type == invType {
        return
    }
    if p.client != nil && p.client.server != nil && p.client.server.invTypes != nil {
        if cfg := p.client.server.invTypes.Configs[invType]; cfg != nil && cfg.Scope == objtype.InvTypeScopeShared {
            source = -1
        }
    }
    if p.invListeners == nil {
        p.invListeners = make(map[int]InventoryListener)
    }
    p.invListeners[com] = InventoryListener{
        Type:      invType,
        Com:       com,
        Source:    source,
        FirstSeen: true,
    }
}
```

Order of branches matches TS exactly (early-out → dedup → splice (implicit via map overwrite) → scope rewrite → push). Lazy-init of the map sits before the final assignment per the existing pattern.

#### Doc-comment update at `modules/world/player.go:627-635`

Narrate all three TS branches, the map-keyed adaptation, and the graceful degradation when server wiring is absent. Replace existing brevity:

```go
// invListenOnCom registers an inventory listener at the given interface
// component ID, matching TS Player.ts:1441-1462 line-by-line.
//
// Behavior:
//   - invType == -1 → no-op (early-out matches TS).
//   - existing listener at com with same Type → no-op (preserves
//     FirstSeen state across redundant inv_transmit calls).
//   - SCOPE_SHARED inv-type → source rewritten to -1 (world-shared
//     dispatch); requires p.client.server.invTypes wired. Graceful
//     no-op when wiring is absent (test-direct-call paths).
//   - Otherwise → store {Type, Com, Source, FirstSeen=true}; the map
//     overwrite naturally implements TS's same-com-different-type
//     splice.
//
// Source = -1 → world-shared inventory (Server.invs[Type]).
// Source >= 0 → another player's slot (Server.players[Source].invs[Type]).
//
// Lazy-initializes the invListeners map on first call.
```

#### Interface contract update at `pkg/script/active.go:277-283`

The `ActivePlayer.InvListenOnCom` interface contract narrates that callers SHOULD pass a real player UID; the implementation rewrites internally for SCOPE_SHARED:

```go
// InvListenOnCom registers an inventory listener at UI component id
// `com` tracking inv type `invType`. Callers pass the player's own
// UID (via ActivePlayer.UID()) or a popped uid for INV_OTHERTRANSMIT
// scenarios; the implementation rewrites source to -1 internally
// when invType has SCOPE_SHARED scope (matches TS Player.ts:1456-1459).
// On the dispatch side, source == -1 routes to the world-shared
// inventory; source >= 0 routes to the player at that server slot.
// Replaces any existing listener at com unless the existing entry
// has the same type (in which case the call is a no-op preserving
// FirstSeen state). Safe when the implementation's listener map is
// still nil — it must lazy-init.
InvListenOnCom(invType, com, source int)
```

No signature change. Doc-only.

#### Touch points

1. **`modules/world/player.go`**:
   - Lines `:627-635`: replace doc-comment per spec above.
   - Lines `:636-646`: replace method body with the three new branches plus the existing logic per the final method shape above.
   - Add `objtype` import if not already present (verify at controller pre-flight; per `controller_preflight` memory).

2. **`modules/world/player_inv_test.go`**:
   - Tighten `TestInvListenOnComReplacesExisting` docstring at `:36-38` to clarify it tests the same-com-DIFFERENT-type splice (which (β) does not affect): replace "matches TS Player.ts:1441-1462 add-or-replace semantics" with "matches TS Player.ts:1457-1460 same-com-different-type splice (β dedup does not apply when types differ)."
   - Add 4 new tests per the test strategy below.

3. **`modules/world/server_test.go`** (or co-located in player_inv_test.go if smaller):
   - Add `newTestPlayerWithInvTypes(t *testing.T, configs []*objtype.InvType) (*Player, net.Conn)` helper that wires a Player to a test Server populated with the given InvType configs. Uses existing `newTestPlayer` and `newTestServer` patterns.

4. **`pkg/script/active.go`**:
   - Lines `:277-283`: replace `ActivePlayer.InvListenOnCom` doc-comment per spec above.

5. **`nai_followups.md:1471-1514`** (NAI-25 close, not Bundle 1 commit):
   - Append a `**Resolved 2026-04-25 (NAI-25 Bundle 1, commit `<hash>`)**` block recording: write-side rewrite porting outcome, the three divergences (α, β, γ) addressed, the new test count, the helper added. Preserve the original tracker body.

#### Tests

Per `plan_test_coverage_crosscheck` memory, four new tests pin the new behaviors:

| Test | Pins | Helper | Setup |
|---|---|---|---|
| `TestInvListenOnComEarlyOutOnInvalidInvType` | (α) | `newTestPlayer` | Call `p.invListenOnCom(-1, 149, 0)`; assert `len(p.invListeners) == 0` and `p.invListeners == nil` |
| `TestInvListenOnComDedupsSameTypeSameCom` | (β) | `newTestPlayer` | Call `p.invListenOnCom(93, 149, -1)`; flip `FirstSeen=false` via read-modify-write; call `p.invListenOnCom(93, 149, -1)` again; assert `FirstSeen` still `false` and `len(p.invListeners) == 1` |
| `TestInvListenOnComRewritesSourceForSharedScope` | (γ) positive | `newTestPlayerWithInvTypes` | Wire `invType=42` with `Scope=InvTypeScopeShared`; call `p.invListenOnCom(42, 149, 99)`; assert stored `Source == -1` (rewritten) |
| `TestInvListenOnComKeepsSourceForNonSharedScope` | (γ) negative | `newTestPlayerWithInvTypes` | Wire `invType=42` with `Scope=InvTypeScopePerm`; call `p.invListenOnCom(42, 149, 99)`; assert stored `Source == 99` (preserved) |

Per `plan_runnable_test_fixtures` memory: the plan author mentally executes (or `go test -run <test-name>` dry-runs) each new test before dispatch.

Per `plan_grep_helper_patterns` memory: before prescribing inline boilerplate, the plan author greps `modules/world/` for existing helper patterns (`newTestPlayer`, `newTestServer`, `discardLogger`, etc.) and reuses them.

Per `plan_helper_coverage` memory: the new `newTestPlayerWithInvTypes` helper is a new addition — its sole consumers are the two new tests in this bundle (γ positive + negative). Future tests can adopt; not a flag-set verification target.

**Existing tests preserved unchanged:**
- `TestInvListenOnComRegistersNewListener` — works because no invTypes wired → γ-branch skipped → source=-1 stays as passed.
- `TestInvListenOnComReplacesExisting` — works because it tests same-com-DIFFERENT-type splice (93 vs 100) which (β) doesn't trigger; docstring tightened only.
- `TestInvListenOnComLazyInitializesMap` — unchanged; lazy-init still happens after the new branches.
- `TestInvStopListenOnCom*` (3 tests) — unchanged.

Per `enumerate_all_sites` memory: at controller pre-flight, re-grep the entire repo for `invListenOnCom\|InvListenOnCom` to confirm no other test pins a behavior that the new branches would change. Pre-flight at spec-write confirms only `modules/world/player_inv_test.go` (existing tests) and `modules/world/modal_close_test.go:14`, `modules/world/inv_update_test.go:90` (struct-literal `InventoryListener` constructions, not invListenOnCom calls) reference the listener path.

#### Deviation impact

0 — three TS-faithful ports; no deviation tags retired or introduced. The From-NAI-24 tracker entry's resolution + commit hash serves as the archaeological record.

#### Commit shape

Single feat commit: `feat(world): NAI-25 Bundle 1 — (*Player).invListenOnCom TS-faithfulness audit`. Body explains the three divergences (with TS:line citations from Player.ts:1441-1462), the brainstorm-time reframing of the From-NAI-24 tracker (API surface not dead — write-side rewrite missing), the test helper addition, the docstring updates, and the tracker resolution direction (close at NAI-25 close commit). Standard `Co-Authored-By: Claude Opus 4.7 (1M context)` trailer.

### Bundle 2 — NumberNotNull audit-completion sweep across remaining handler files

**Goal**: Audit every TS NumberNotNull site in any TS handler file mapped to an unaudited goscape handler file. Each goscape file gets a per-file audit table per the NAI-23 Bundle 4 cadence. Files with non-zero net-new wraps get own-commits; confirm-zero files fold into a single rollup commit. Resolve the From-NAI-23 tracker entry as "all 13 unaudited handler files swept; per-file results documented."

**Source**: NAI-23 close — From-NAI-23 tracker entry `Future NumberNotNull sweep targets (out-of-scope file enumeration)` at `nai_followups.md:1409-1446`.

#### TS source canonical paths

Per `ts_source_canonical_path` memory: only `Engine-TS/src/engine/script/handlers/` is the canonical reference. Per-file mappings:

| goscape file | TS counterpart(s) | TS NumberNotNull count |
|---|---|---|
| `handlers_config.go` | `NpcConfigOps.ts` (2) + `LocConfigOps.ts` (0) + `ObjConfigOps.ts` (0) | 2 |
| `handlers_dialog.go` | `PlayerOps.ts` subset (P_PAUSEBUTTON, P_COUNTDIALOG, LAST_COM, LAST_INT, LAST_ITEM, LAST_SLOT, LAST_USE_ITEM, LAST_USE_SLOT, LAST_TARGET_SLOT, CAM_RESET, STAFF_MOD_LEVEL, UID) | unknown subset of 56 |
| `handlers_timer.go` | `PlayerOps.ts` subset (SETTIMER, SOFTTIMER, CLEARTIMER, CLEARSOFTTIMER, GETTIMER) | unknown subset of 56 |
| `handlers_vars.go` | `CoreOps.ts` subset (PUSH_VARP, POP_VARP, PUSH_VARS, POP_VARS, PUSH_VARN, POP_VARN) | 0 |
| `handlers_array.go` | `CoreOps.ts` subset (DEFINE_ARRAY, PUSH_ARRAY_INT, POP_ARRAY_INT, SWITCH) | 0 |
| `handlers_loc.go` | `LocOps.ts` | 0 |
| `handlers_obj.go` | `ObjOps.ts` | 0 |
| `handlers_db.go` | `DbOps.ts` | 0 |
| `handlers_string.go` | `StringOps.ts` | 0 |
| `handlers_server.go` | `ServerOps.ts` | 0 |
| `handlers_core.go` | `CoreOps.ts` (file-wide minus subsets above) | 0 |
| `handlers_debug.go` | `DebugOps.ts` | 0 |
| `handlers_number.go` | `NumberOps.ts` | 0 |
| `handlers_lastinput_test.go` | `PlayerOps.ts` subset (test-only; production handlers physically live in `handlers_dialog.go`) | covered under handlers_dialog.go |

Brainstorm-time count: 84/86 (97.7%) of all TS-side NumberNotNull invocations across the handlers tree are already absorbed by NAI-23 Bundles 4a/4b/4c and NAI-24 Bundle 1. Bundle 2 covers the remaining 2/86 (NpcConfigOps) plus the PlayerOps.ts cross-file residue surfaced by NAI-24 file-scoped audit.

#### PlayerOps.ts cross-file residue cross-check (Bundle 2 implementer responsibility)

Math: PlayerOps.ts has 56 NumberNotNull; NAI-24 Bundle 1 audited **47 popInt sites** in `handlers_player.go` (per commit `85da016` audit table); the **9-site delta** lives in PlayerOps.ts opcodes that goscape dispatches from a different file (or whose TS counterpart is not a popInt-equivalent in goscape).

The Bundle 2 plan task codifies this enumeration step:
1. Re-grep `PlayerOps.ts` for `NumberNotNull` and enumerate all 56 sites with their opcode names (`grep -n "check(.*NumberNotNull" Engine-TS/src/engine/script/handlers/PlayerOps.ts`).
2. Cross-reference NAI-24 Bundle 1's audit table in commit `85da016` for the 47 wrapped/skipped sites.
3. The delta opcodes (9 sites) get assigned to the correct goscape handler file via opcode-to-handler mapping (`grep -n "func handle<OpName>" pkg/script/handlers_*.go` plus the registration table in `pkg/script/handlers.go` if applicable).
4. Each delta opcode's audit decision lands in the correct Bundle 2 file commit (handlers_dialog.go, handlers_timer.go, or — if a delta opcode maps to a handler that doesn't fit dialog/timer — that file gets the audit row instead).

If after the enumeration zero delta opcodes land in dialog/timer (i.e., all 9 sites are popObj-equivalents or live elsewhere), those files fold into the confirm-zero rollup commit.

#### Per-file audit-pass cadence (per NAI-23 Bundle 4 precedent)

For each goscape file in scope, the implementer subagent applies the established cadence:
1. Enumerate every `s.PopInt()`, `s.PopInts(n)`, `s.PopObj()`, `s.PopString()` site (all pop-with-validator-type operations).
2. Grep TS counterpart for `NumberNotNull`.
3. For each site, decide via the rubric:
   - **WRAP**: TS uses `check(state.popInt(), NumberNotNull)`; goscape lacks the wrap → add `if err := checkNotNull(v, "OP_NAME"); err != nil { return err }`. Add a `TestHandle<OpName>NullRejected` test.
   - **SKIP**: TS uses a typed validator (`InvTypeValid`, `CoordValid`, `PlayerStatValid`, `LocAngleValid`, `SeqTypeValid`, `EnumTypeValid`, etc.) → goscape's existing path already routes through an equivalent typed validator → no production change. Document in audit table.
   - **SKIP (TS not wrapped)**: TS does not wrap the popped value at all → preserve TS tolerance. Document in audit table.
   - **SKIP (signed value)**: the popped value is semantically signed (coord delta, search-relative offset, arithmetic operand) → `-1` is legitimate; preserve.
   - **ESCALATE**: TS uses NumberNotNull but goscape's existing path is structurally different (e.g., goscape pre-validates upstream) → file as a tagged deviation (NAI-25-D1+) with rationale.
4. Record per-handler row in the audit table embedded in the commit message.

This rubric mirrors NAI-23 Bundle 4 and NAI-24 Bundle 1 verbatim — no methodological divergence.

#### Wrap shape and naming

Wrap pattern (mirrors handlers_player.go post-NAI-24):
```go
v := s.PopInt()
if err := checkNotNull(v, "OP_NAME"); err != nil {
    return err
}
```

`OP_NAME` follows the existing convention (lowercase opcode mnemonic; e.g., `"npc_config"`, `"settimer"`, `"last_int"`). The implementer subagent reads the file's existing wrapped handlers (or, for files with zero pre-existing wraps, reads handlers_player.go / handlers_inv.go for templates) and reuses the casing convention.

Test naming: `TestHandle<OpName>NullRejected` (single-int handlers) or table-driven sub-cases when a handler pops multiple wrapped ints. Per NAI-24 Bundle 1 precedent: pin only one int's null at a time; the other wrapped ints stay valid so the rejection is attributable to the specific wrap.

#### Audit table format (canonical artifact)

Embedded in each Bundle 2 commit message (per-file commits and rollup commit). Per-file format mirrors NAI-23 Bundle 4c (commit `8ea45b0`):

| Handler | popInt context | TS wraps? | Decision | Rationale (TS file:line) |
|---------|---------------|-----------|----------|-------------------------|
| handleSomeOp | x | NumberNotNull | WRAP | LocOps.ts:NN |
| handleOther | y | not wrapped | SKIP (TS not wrapped) | LocOps.ts:NN |
| ... | ... | ... | ... | ... |

Skip-reason breakdown summarized at the end (typed-validator skips / signed-sentinel skips / TS-not-wrapped skips counts).

Confirm-zero rollup commit format — one row per file:

```
| goscape file        | TS counterpart   | popInt sites | TS NumberNotNull | Conclusion |
|---------------------|------------------|--------------|------------------|------------|
| handlers_loc.go     | LocOps.ts        | <N>          | 0                | No-op      |
| handlers_obj.go     | ObjOps.ts        | <N>          | 0                | No-op      |
| ...                 | ...              | ...          | ...              | ...        |
```

#### Touch points per file

For each in-scope file:
1. **Production file (`pkg/script/handlers_<name>.go`)**: zero changes if confirm-zero; one or more `checkNotNull` wraps if WRAPs identified.
2. **Test file (`pkg/script/handlers_<name>_test.go`)**: zero changes if confirm-zero; one `TestHandle<OpName>NullRejected` per WRAP.
3. **Audit table**: embedded in the corresponding commit message.

#### Commit organization

**Per-file commits (non-zero yield):**
- `feat(script): NAI-25 Bundle 2 — handlers_config.go NumberNotNull audit` (NpcConfigOps.ts has 2 sites; expected non-zero)
- `feat(script): NAI-25 Bundle 2 — handlers_dialog.go NumberNotNull audit` (only if PlayerOps.ts cross-file residue lands here with WRAPs)
- `feat(script): NAI-25 Bundle 2 — handlers_timer.go NumberNotNull audit` (only if PlayerOps.ts cross-file residue lands here with WRAPs)

**Rollup commit (confirm-zero):**
- `feat(script): NAI-25 Bundle 2 — confirm-zero rollup across <N> handler files`
- Body: per-file audit table (one row per popInt-equivalent site per file) confirming TS-side NumberNotNull is absent; closes the From-NAI-23 sweep tracker.

**Empty-result handling:** if `handlers_dialog.go` or `handlers_timer.go` audit yields zero WRAPs (i.e., the PlayerOps.ts cross-file residue doesn't land in those files), they fold into the rollup commit instead of getting their own commit.

Estimated total Bundle 2 commit count: **3-4 commits** (handlers_config.go own commit + 0-2 PlayerOps.ts-residue own commits + 1 confirm-zero rollup).

#### Tests

Bounded by audit-table WRAP row count per file. Per `plan_test_coverage_crosscheck`: the plan doc lists per-file expected-test-count derived from spec-time TS NumberNotNull counts (handlers_config.go: ≤ 2 tests; handlers_dialog.go / handlers_timer.go: ≤ 9 tests combined depending on residue distribution; rollup: 0 tests).

Per `plan_runnable_test_fixtures` memory: each plan-codified test fixture is mentally executed before dispatch.

Per `plan_grep_helper_patterns` memory: each handlers_*_test.go file's existing test scaffolding is the template; the plan author identifies the file's existing builder by name and confirms its signature at plan-write.

#### Deviation impact

0 by default. ESCALATE rows in the audit table that yield deviation tags get NAI-25-D<n> per the standard deviation-tagging convention. None expected at spec-write.

### Polish commit (between Bundle 2 close and NAI-25 close)

Standard cadence: one polish commit absorbs minor review feedback from both bundles. Per `dead_api_polish` memory, polish commit also catches any helpers shipped with zero consumers (e.g., the new `newTestPlayerWithInvTypes` helper — verify both new tests reference it).

## Out-of-scope (explicitly deferred)

1. **Zone state during respawn (NAI-19-D1 closure track).** Deferred — needs Zone abstraction infrastructure design first; remains the biggest open structural item per the From-NAI-23 / nai_followups archaeology. Out of scope for follow-up bundles.

2. **NAI-11 deferrals**: SMART pathfinding, reach helpers, focus() instant flag — each its own substantial sub-spec. Inherited deferral.

3. **`handlers_lastinput_test.go` separate work item**: the production code physically lives in `handlers_dialog.go`; the test file is a partition for test organization. Bundle 2's `handlers_dialog.go` audit covers the underlying handlers; no separate work needed.

4. **Retract-`-1` API surface (originally framed in From-NAI-24)**: superseded by the brainstorm reframing — Bundle 1's TS-faithful port resolves the question without retraction. The `-1` API surface remains, now with a live producer (the (γ) scope rewrite).

5. **Cross-file PlayerOps.ts deltas that don't land in dialog/timer**: if Bundle 2 implementer enumeration finds delta opcodes in goscape files outside dialog/timer, those handlers' audit rows still belong to Bundle 2 (the original audit-completion charter); they don't spill out to a future sub-spec.

## Risks & mitigations

- **Bundle 1 `objtype` import shape.** Risk: `modules/world/player.go` may not yet import `pkg/objtype`; the (γ) lookup requires `objtype.InvTypeScopeShared`. Mitigation: controller pre-flight verifies import at HEAD; implementer adds the import as part of the production change. Per `controller_preflight` memory.

- **Bundle 1 `p.client.server.invTypes` field path.** Risk: the field name or path differs from spec assumption. Mitigation: pre-flight verified via `invLookupView` at `modules/world/server_invs.go:16` reading `v.s.invTypes`; controller re-confirms at dispatch.

- **Bundle 1 hidden test that pins the (β) FirstSeen-reset behavior.** Risk: a test elsewhere relies on the current behavior of FirstSeen-reset on every call (i.e., calling `invListenOnCom` twice causes a re-emit). Mitigation: `enumerate_all_sites` re-grep at controller pre-flight for `FirstSeen\|firstSeen` across `modules/world/`; pre-flight at spec-write confirms no other test pins this — the only `FirstSeen` references are in `player_inv_test.go` (controlled), `player.go` (production), `inv_update_test.go:90` (struct literal), `modal_close_test.go:14` (struct literal).

- **Bundle 1 cascade from (γ) port.** Risk: porting the (γ) rewrite surfaces a downstream test failure (e.g., a test that expected a per-player slot for an inv-type that's actually SCOPE_SHARED in test fixtures). Mitigation: any cascade gets its own tracker entry; not absorbed into Bundle 1 unless trivial. The fail-fast result is information.

- **Bundle 2 implementer dispatches before controller pre-flight catches a stale TS-side state.** Risk: a NumberNotNull was added to TS upstream since this brainstorm. Mitigation: `controller_preflight` protocol — re-grep TS at dispatch time, not just at spec-write.

- **Bundle 2 PlayerOps.ts cross-file residue distribution surprise.** Risk: the 9-site delta enumeration reveals all sites are popObj-equivalents (not popInt) and don't WRAP-eligible. Mitigation: audit table documents the SKIP rationale; rollup commit absorbs handlers_dialog/timer; no spec change needed.

- **Bundle 2 handlers_config.go ESCALATE.** Risk: the 2 NpcConfigOps NumberNotNull sites are SKIP-eligible (e.g., NpcTypeValid covers them) → zero WRAPs in handlers_config.go. Mitigation: audit table records the SKIP rationale; commit message explains; commit still lands as own-commit because the audit produced a non-trivial decision per site.

- **`controller_preflight` discipline at task dispatch.** Per memory: 30-second grep+Read pass against HEAD before each implementer dispatch to verify file paths, line numbers, signatures, helper init state. Applied per-bundle.

- **`spec_followup_tracker_freshness` discipline.** Per memory: tracker entries silently rot. Spec-write-time re-greps verified the From-NAI-24 entry assertions (zero production callers passing -1; dispatch site at `:471-479`; doc-comment at `:632-633`; interface contract at `:277-283`); the From-NAI-23 entry assertions (per-file enumeration; priority order). All assertions held at HEAD `20fb72a`.

## Review structure

Per `runescript_cadence` memory: two-stage review per bundle (spec compliance → code quality, both via opus). Final whole-impl review after all bundles.

- **Bundle 1**: Stage 1 spec compliance (each of α/β/γ branches lands in correct order; the existing tests still pass; the new tests pin the new behaviors; doc-comment narration matches spec) + Stage 2 code-quality review (test naming, error-message conventions, helper-coverage check on `newTestPlayerWithInvTypes`, doc-comment polish).
- **Bundle 2**: Stage 1 audit-table review (per-pop-site decisions cross-checked against TS; PlayerOps.ts cross-file residue distribution sanity-checked against the 56-47=9 math) + Stage 2 code-quality review (test naming, audit-table consistency across files, rollup-commit table completeness, no shipped-with-zero-consumers helpers).
- **Whole-impl review**: validates that NAI-25 closes the two cited tracker entries (Bundle 1: From-NAI-24 at `nai_followups.md:1471-1514`; Bundle 2: From-NAI-23 at `:1409-1446`) and that the per-bundle commit shapes match the spec.

Polish commits land if final whole-impl review surfaces remediable findings, per NAI-23 / NAI-24 precedent.

## NAI-25 close

The close commit:
- Updates `nai_followups.md`: marks the two tracker entries Resolved (Bundle 1 entry at `:1471-1514` with the brainstorm-reframing context — "API surface not dead — write-side rewrite was missing; ported"; Bundle 2 entry at `:1409-1446` with the per-file audit summary including any dispositioned ESCALATEs).
- Per `close_commit_memory_trailer` memory: includes the standard `Co-Authored-By` trailer; carries `Closes memory: nai_followups.md` if memory edits are part of the close commit.
- Per `post_task_handoff` memory: at NAI-25 close, save non-derivable info to memory AND give the user a paste-ready resume prompt for NAI-26 (with HEAD hash, deviation count, and the most actionable next-NAI candidates).
- Memory entry candidates pre-flagged at brainstorm time (re-evaluate at close):
  - `audit_full_method_against_ts` — when touching a TS-faithfulness boundary, audit the entire method against TS, not just the line that brought you there. Provenance: (α/β/γ) discovery in `(*Player).invListenOnCom`. Likely save.
  - `file_scoped_audits_miss_cross_file_ts` — file-scoped audits silently miss when one TS file's opcodes dispatch from multiple goscape files; re-verify by grepping TS for the validator and confirming every site lives in some goscape audit. Provenance: PlayerOps.ts cross-file residue math (56 NumberNotNull vs 47 audited). Likely save.
  - `prenarrowed_candidates_benefit_from_fresh_density_data` — even pre-narrowed candidate menus deserve fresh data validation at brainstorm time. Provenance: density-data finding that recalibrated candidate (b) from "high yield" to "low yield." Maybe — borderline overlap with `controller_preflight`.
  - `tracker_entry_framing_can_be_incomplete` — From-NAI-N tracker entries can encode a misframed problem; brainstorm should re-derive the problem from primary sources. Provenance: From-NAI-24 "retract vs preserve" reframing into "missing producer." Maybe — overlap with `spec_followup_tracker_freshness`.
