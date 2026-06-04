# NAI-114 Stage 4 — OPHELDU reject-gate probe design

**Date:** 2026-05-06
**Predecessor:** [NAI-114 Stage 3 spec](2026-05-06-nai-114-stage3-instrumentation-probe-design.md) (closed; smoke proved trigger-lookup region unreachable).
**Status:** Stage 4 instrumentation sub-spec — binds the early-return gate that rejects tutorial firemaking OPHELDU events upstream of the trigger lookup.

---

## 1. Symptom (post-Stage-3)

Stage 3 instrumentation (commits `a5e78ec` Provider.Names accessor, `6b489b3` boot-time `[opheldu,*]` registry log, `9d19da3` 6 inline DEBUG lines in `handleOpHeldU` L370-L418) shipped and was smoked.

Smoke binding (per `nai_114_stage3_binding.md`):

- Boot logged 225 `[opheldu,*]` scripts; both `[opheldu,tinderbox]` AND `[opheldu,logs]` present. **Path A1 (script not registered) rejected.**
- 3 OPHELDU packets fired during tutorial firemaking (opcode 130, len=12). Pre-existing packet logger at `modules/world/player.go:952` confirms each event reaches `handleOpHeldU` entry.
- Decoded first payload: `obj=2511` (logs variant), `useObj=590` (tinderbox), `comId=useComId=3214` (inv).
- **Zero of the 6 Stage-3 instrumentation lines fired across all 3 events.** No `opheldu trigger probe context`, no `opheldu arm probe`, no `opheldu dispatch`, no `opheldu fallback miss`. No `script execute error` WARN.

The Stage-3 lines all sit between L370 and L418 in `handleOpHeldU`. Their silence binds the handler exit to the early-return region between L272 and L368 — upstream of the trigger lookup at L370. Stage 3's decision matrix did not anticipate this branch ("Path E" upstream-of-trigger blocker), so Stage 4 re-instruments to bind which of the 13 early-return gates fires.

## 2. Hypothesis space

13 candidate early-return gates exist between L272 and L368 in `handleOpHeldU`:

| # | Line | Gate |
|---|------|------|
| 1 | 272-274 | `p.client == nil \|\| p.client.server == nil` |
| 2 | 277-279 | `p.delayed && s.currentTick < p.delayedUntil` |
| 3 | 280-282 | `len(payload) < 12` |
| 4 | 292-294 | `comId != useComId` |
| 5 | 296-298 | `com == nil \|\| !com.Usable` |
| 6 | 300-302 | `!p.IsComponentVisible(com)` |
| 7 | 304-306 | `useCom == nil \|\| !useCom.Usable` |
| 8 | 308-310 | `!p.IsComponentVisible(useCom)` |
| 9 | 312-315 | `comId not in p.invListeners` |
| 10 | 316-319 | `resolveListenerInv(s, listener) == nil` |
| 11 | 320-325 | `!inv.HasAt(slot, obj)` (TS-cleanup before return) |
| 12 | 327-330 | `useComId not in p.invListeners` |
| 13 | 331-334 | `resolveListenerInv(s, useListener) == nil` |
| 14 | 335-340 | `!useInv.HasAt(useSlot, useObj)` (TS-cleanup before return) |
| 15 | 349-351 | `objType for obj is nil` (defensive) |
| 16 | 352-354 | `objType for useObj is nil` (defensive) |
| 17 | 365-368 | members-only gate — `(objType.Members \|\| useObjType.Members) && !s.cfg.NodeMembers` |

(17 reject sites total; #1, #3, #4 are sentinels — Stage 3 evidence rules them out, but they're cheap to instrument for completeness.)

User's pre-brainstorm ranked likelihood (top-4): #9 invListener missing > #5/#6 lookupComponent failure > #11 HasAt failure > #2 delayed.

Static-read does NOT bind any of these — RuneScript content (which drives INV_LISTENONCOM, modal state, tutorial-step delays, and inventory population) is loaded from an external content cache, not in this repo. Per `cascade_theory_smoke_binding`, smoke binds; one instrumentation pass that logs every reject site identifies the gate in a single iteration.

## 3. Scope

### In scope

- Add one entry-state DEBUG log in `handleOpHeldU` after the packet-decode block (L290), capturing all decoded fields plus delay state.
- Add 17 inline `s.log.Debug("opheldu reject", "gate", "<name>", …)` lines, one immediately before each `return nil` in L272-L368. Fully additive — no restructuring, no behavior change.
- Revert Stage 3 transient commits as Stage 4's first two implementation commits (one revert per commit, in order).
- All instrumentation lives inline in `modules/world/handler_opheld.go`. No new files.
- One transient implementation commit `chore(debug): NAI-114 Stage 4 — opheldu reject-gate instrumentation`. Reverted at NAI-114 Stage 5 close.

### Out of scope

- **The fix.** Stage 5 (sized to the binding gate) will write+ship it.
- **Reverting the Stage 4 transient probe.** Handled at Stage 5 close.
- **Production-quality logging.** Throwaway probe. No log-level tuning, field-stability guarantees, or test coverage of log-line shape.
- **Stage-3 sites L370-L418.** Reverted by commits 3+4; not re-instrumented (the trigger-lookup region is unreachable until the upstream gate is fixed).
- **`a5e78ec` Provider.Names() accessor.** Stays — long-term API.

## 4. Stage 1 — instrumentation implementation

### 4.1 Commit sequence

1. `docs(spec): NAI-114 Stage 4 — reject-gate probe design` ← this commit.
2. `docs(plan): NAI-114 Stage 4 — reject-gate probe plan` ← from `writing-plans`.
3. `revert: chore(debug): NAI-114 Stage 3 — opheldu trigger-lookup hit-trace` ← `git revert 9d19da3`.
4. `revert: chore(debug): NAI-114 Stage 3 — boot-time opheldu script-registry log` ← `git revert 6b489b3`.
5. `chore(debug): NAI-114 Stage 4 — opheldu reject-gate instrumentation` ← single transient implementation commit.

After commit 4, `handleOpHeldU` returns to its pre-Stage-3 shape. Commit 5's diff is purely additive against that baseline.

### 4.2 Entry log

Insert immediately after the packet-decode block (after L290 `useComId := int(r.G2())`):

```go
s.log.Debug("opheldu entry",
    "tick", s.currentTick,
    "obj", obj, "slot", slot, "comId", comId,
    "useObj", useObj, "useSlot", useSlot, "useComId", useComId,
    "delayed", p.delayed, "delayedUntil", p.delayedUntil)
```

One line per OPHELDU event. Correlates each subsequent `opheldu reject` line to its source packet via `tick`.

### 4.3 Reject-site log table

For each row, insert one `s.log.Debug("opheldu reject", "gate", "<name>", …)` line immediately before the corresponding `return nil`. Plan codifies exact line numbers post-revert (commits 3+4 may shift numbers slightly; plan-author HEAD-verifies after revert lands).

| # | Post-revert line ref | gate= | Extra fields |
|---|---|---|---|
| 1 | L272-274 | `"client_nil"` | `client_nil=p.client==nil`, `server_nil=(p.client!=nil && p.client.server==nil)` |
| 2 | L277-279 | `"delayed"` | `currentTick=s.currentTick`, `delayedUntil=p.delayedUntil` |
| 3 | L280-282 | `"short_payload"` | `payload_len=len(payload)` |
| 4 | L292-294 | `"comId_mismatch"` | `comId`, `useComId` |
| 5 | L296-298 | `"com_nil_or_unusable"` | `com_nil=(com==nil)`, `com_usable=(com!=nil && com.Usable)` |
| 6 | L300-302 | `"com_invisible"` | (no extra) |
| 7 | L304-306 | `"useCom_nil_or_unusable"` | `useCom_nil`, `useCom_usable` |
| 8 | L308-310 | `"useCom_invisible"` | (no extra) |
| 9 | L312-315 | `"invListener_missing"` | `comId`, `listener_count=len(p.invListeners)`, `listener_keys` (sorted, capped at 16) |
| 10 | L316-319 | `"inv_unresolved"` | `listener_type=listener.Type`, `listener_source=listener.Source` |
| 11 | L320-325 | `"inv_hasAt_failed"` | `slot`, `obj` — log placed BEFORE the TS-cleanup (`moveClickRequest=false; ClearPendingAction()`) |
| 12 | L327-330 | `"useInvListener_missing"` | `useComId`, `listener_count=len(p.invListeners)` |
| 13 | L331-334 | `"useInv_unresolved"` | `useListener_type`, `useListener_source` |
| 14 | L335-340 | `"useInv_hasAt_failed"` | `useSlot`, `useObj` — log placed BEFORE the TS-cleanup |
| 15 | L349-351 | `"objType_unregistered"` | `which="obj"`, `id=obj` |
| 16 | L352-354 | `"objType_unregistered"` | `which="useObj"`, `id=useObj` |
| 17 | L365-368 | `"members_only"` | `obj_members=objType.Members`, `useObj_members=useObjType.Members`, `node_members=s.cfg.NodeMembers` |

**Total:** 1 entry log + 17 reject logs = 18 inline DEBUG lines.

### 4.4 `listener_keys` rendering

For sites 9 and 12, derive a bounded sorted-key snapshot:

```go
keys := make([]int, 0, len(p.invListeners))
for k := range p.invListeners {
    keys = append(keys, k)
}
sort.Ints(keys)
if len(keys) > 16 {
    keys = keys[:16]
}
```

Inline this just before the log call. Cap=16 keeps log lines bounded; if the user has hundreds of listeners (unlikely on tutorial), the truncation is acceptable for binding purposes.

### 4.5 Build + test verification

After commit 5:

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` all PASS — additive constraint preserves all `handler_opheld_test.go` assertions.

## 5. Stage 2 — smoke handoff + outcome routing

User re-launches goscape after commit 5 lands, connects via Java client rev-225, walks Tutorial Island fire-making step (tinderbox on logs).

### 5.1 Decision matrix → Stage 5 routing

#### Most-likely group

| Binding gate | Diagnosis | Stage 5 spec framing |
|---|---|---|
| `invListener_missing` (#9) | `p.invListeners[3214]` not set when OPHELDU arrives | Investigate when/whether INV_LISTENONCOM dispatches to register the inventory tab listener. Likely root: tutorial login-script flow gap or missing port. |
| `inv_hasAt_failed` (#11) | listener+inv resolved but server-side slot/obj differs from client's view | Investigate inv-snapshot/listener-update timing; possible NAI-113 inventory-uid plumbing follow-up. |
| `delayed` (#2) | `p.delayed` still true at OPHELDU tick | Investigate which prior tutorial step left `p.delayed=true`; compare TS clearing semantics. |

#### Less-likely group (may need re-brainstorm before fix)

| Binding gate | Diagnosis | Stage 5 framing |
|---|---|---|
| `com_invisible` / `useCom_invisible` (#6/#8) | Component 3214 fails `IsComponentVisible` traversal | Investigate modal-screen ownership / root-layer assignment for component 3214 during tutorial state. |
| `inv_unresolved` (#10) | Listener present but `resolveListenerInv` returns nil | Investigate `Server.invs` / `Player.invs` wiring for the registered listener.Type. |
| `members_only` (#17) | obj 2511 or 590 flagged Members on a free-world config | Likely config bug — Members flag wrong on these objs or `NodeMembers` mis-wired. |
| `objType_unregistered` (#15/#16) | obj 2511 or 590 missing from objType cache | Surprising for stock content; investigate objType cache loader. |

#### Rule-out group (should not fire)

| Binding gate | Pivot |
|---|---|
| `client_nil` (#1), `short_payload` (#3), `comId_mismatch` (#4) | Dispatcher / packet-decode regression. Pivot Stage 5 to investigating `runGamePacket` / `gameHandlers`. |
| `com_nil_or_unusable` / `useCom_nil_or_unusable` (#5/#7) | Component 3214 not registered or marked !Usable. Pivot to interface-config cache investigation. |
| `useInvListener_missing` (#12), `useInv_unresolved` (#13), `useInv_hasAt_failed` (#14) | Should be impossible since `comId==useComId` ⇒ same listener. If they fire, the `comId_mismatch` sentinel was wrong upstream — pivot to packet-decode. |

### 5.2 Stage 4 pass criteria

Stage 4 is diagnostic-only; no user-visible game behavior change is expected. Stage 4 pass =

1. Each OPHELDU packet produces exactly one `opheldu entry` line followed by exactly one `opheldu reject` line in server stdout.
2. The `gate=` value unambiguously selects exactly one row in §5.1.
3. The selected row's diagnosis is internally consistent with the entry-log fields (e.g., `gate=delayed` ⇒ entry log shows `delayed=true` and `currentTick < delayedUntil`).

If multiple gates fire across the 3 OPHELDU packets (different gates per packet), each packet binds independently — Stage 5 may need to address more than one gate.

## 6. Risk register

| # | Risk | Probability | Impact | Mitigation |
|---|---|---|---|---|
| R1 | `s.log` not reachable from `handleOpHeldU` scope | NIL | Compile fail | Already verified — Stage 3's now-reverted inline DEBUGs at L370+ used `s.log` in the same scope. |
| R2 | Log ordering at sites #11/#14 (HasAt failure runs TS-faithful cleanup before return) | LOW | Diagnostic captures post-cleanup state | Plan specifies: log placed BEFORE the cleanup block (`moveClickRequest=false; ClearPendingAction()`). Pre-cleanup state is the diagnostic. |
| R3 | `listener_keys` log line size (high listener count) | LOW | Verbose log line | Cap at 16 sorted entries. |
| R4 | Sentinel sites (#1, #3, #4) firing despite Stage 3 rule-out | LOW | Pivots Stage 5 to dispatcher/decoder | Acceptable — confirmation rather than surprise; rerouted via §5.1 rule-out group. |
| R5 | Multiple gates fire across the 3 packets | MED | Stage 5 may need multi-gate fix | Acceptable — `tick` field on entry log correlates each reject to its packet; Stage 5 brainstorm sizes accordingly. |
| R6 | Post-revert line numbers shift, breaking literal line refs in §4.3 | LOW | Plan-author confusion | Plan-author HEAD-verifies post-revert and re-anchors line refs in plan tasks before implementer dispatch. |
| R7 | Stage 4 commit 5 lands without commits 3+4 (out-of-order revert) | LOW | Mixed Stage-3+Stage-4 instrumentation; harder to read smoke logs | Plan codifies commit sequence; controller pre-flight checks `git log` shape before Stage 5 close. |

## 7. Tech stack & deliverables

- **Go 1.26+** per `go_version`.
- **TS source:** `LostCityRS/Engine-TS` per `ts_source_canonical_path` (Stage 4 doesn't touch TS-faithful code paths).

**Memory updates on Stage 4 close (deferred to Stage 5 close):**

- One memory entry per the binding outcome — pattern depends on the gate. Candidate names depending on outcome:
  - `invListener_missing` → `tutorial_inv_listener_timing.md` or similar.
  - `delayed` → `tutorial_step_delay_carryover.md`.
  - others → as appropriate.
- Update `cascade_theory_smoke_binding.md` with NAI-114 Stage 4 example: probe-only sub-spec with reject-gate enumeration as a cadence template.
- Update `investigation_subspec_cadence.md` to note that Stage-N probe sub-specs can chain (Stage 3 → Stage 4 both probe-only) when the first probe binds an unanticipated upstream branch.

---

## 8. Self-review

1. **Placeholder scan:** none. All gate names, line numbers, and matrix routings are explicit.
2. **Internal consistency:** §2 hypothesis-space rows map 1:1 to §4.3 reject-site log table rows (17 each) and §5.1 decision-matrix groups (17 across the three groups). No contradictions.
3. **Scope check:** focused on a single instrumentation pass; the fix is excluded. Decomposition not needed.
4. **Ambiguity check:** §4.3 specifies pre-revert vs post-revert line numbering and delegates final anchoring to plan-author HEAD-verify (R6). §4.4 codifies `listener_keys` derivation explicitly. §5.1 splits "most-likely" vs "less-likely" vs "rule-out" so a binding outcome unambiguously selects one row.
