# NAI-60 — component-registry cluster cleanup

> **TS-faithfulness gate.** All gate shapes mirror the canonical TS handlers under
> `Engine-TS/src/network/game/client/handler/`. No new deviations are expected.

## §1. Scope

NAI-60 wires the NAI-59-landed `s.lookupComponent` + `Player.IsComponentVisible`
infrastructure into 5 client-message handler families, retiring **10 deviation
sites carrying 8 distinct tags**:

| Tag | Site | Family |
|---|---|---|
| `S6m-D1` | `modules/world/handler_oploc.go:114` | OpLocT |
| `S6m-D2` | `modules/world/handler_oploc.go:195` | OpLocU |
| `S6o-D1` | `modules/world/handler_opnpc.go:97` | OpNpcT |
| `S6o-D2` | `modules/world/handler_opnpc.go:177` | OpNpcU |
| `NAI-50-D1` | `modules/world/handler_opobj.go:95` | OpObjT |
| `NAI-50-D2` | `modules/world/handler_opobj.go:159` | OpObjU |
| `NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED` | `modules/world/handler_op_player.go:74` | OpPlayerT |
| `NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED` | `modules/world/handler_op_player.go:134` | OpPlayerU |
| `NAI-48-D1` | `modules/world/handler_inv_button.go:21` | InvButton |
| `NAI-48-D1` | `modules/world/handler_inv_button.go:74` | InvButtonD |

**Out of scope (deferred to future sub-spec):** NAI-53-D-CLEARCOMLISTENERS-PER-SLOT.
That deviation lives in `modules/world/player_script.go:633` and operates on the
per-listener filter inside `encodeOut` (`modules/world/player.go:291-301`). The
required infrastructure (`s.lookupComponent`) is already available, but the work
is structurally distinct (per-listener filter inside outgoing-tick path vs the
input-gate pattern this sub-spec lands) and warrants its own brainstorm of the
`refreshModalClose` signaling shape.

## §2. Gate templates by cluster

The three gate templates below cover all 10 sites. Code is illustrative; plan-
authored task blocks must show per-site insertion point with surrounding-line
context.

### §2.1 T-variants — spell-on-X (4 handlers)

**Position:** immediately after the `delayed` check, before any entity lookup.
Matches all 4 TS T-handlers (`OpLocTHandler.ts`, `OpNpcTHandler.ts:22-31`,
`OpObjTHandler.ts:20-29`, `OpPlayerTHandler.ts:22-31`).

```go
spellCom := int(r.G2())

com := s.lookupComponent(spellCom)
if com == nil || (com.ActionTarget&objtype.ComActionTarget<X>) == 0 {
    sendUnsetMapFlag(p)
    return nil
}
if !p.IsComponentVisible(com) {
    sendUnsetMapFlag(p)
    return nil
}
```

The `<X>` constant per handler (defined in `pkg/objtype/componenttype.go:38-43`):

| Handler | Bitmask |
|---|---|
| `handleOpLocT` | `ComActionTargetLoc` |
| `handleOpNpcT` | `ComActionTargetNpc` |
| `handleOpObjT` | `ComActionTargetObj` |
| `handleOpPlayerT` | `ComActionTargetPlayer` |

These constants currently have **zero production consumers** (see source comment
at `componenttype.go:36`). NAI-60 is the first consumer; per memory
`consume_reserved_constant`, the new consumer owns the full dispatch path —
plan-author must verify the bit values match TS `Component.ts:321-327` at plan-
write time.

### §2.2 U-variants — item-on-X (4 handlers)

**Gate is uniform across all 4 sites:** `nil || !Usable || !IsComponentVisible`.

**Position is per-TS, not uniform:**

- `handleOpNpcU`, `handleOpPlayerU`: gate fires **immediately after delayed**,
  before any other lookup. Matches `OpNpcUHandler.ts:24-33` and
  `OpPlayerUHandler.ts:24-33`.
- `handleOpObjU`, `handleOpLocU`: gate fires **after coord viewport + entity
  lookup**, **before** inv-listener resolution. Matches
  `OpObjUHandler.ts:39-48`. The TS pattern is: validate the cheap-to-fail
  coord+entity gates first (no entity-state side effects there), then validate
  useCom.

```go
useCom := int(r.G2())

com := s.lookupComponent(useCom)
if com == nil || !com.Usable {
    sendUnsetMapFlag(p)
    return nil
}
if !p.IsComponentVisible(com) {
    sendUnsetMapFlag(p)
    return nil
}
```

### §2.3 InvButton family (2 handlers)

Mirrors NAI-59 T6 IfButton template. Per-handler the second-stage check
differs — `Iop[op-1]==""` for InvButton, `!Draggable` for InvButtonD. Reject
behavior is **silent drop** (return nil), matching IfButton (TS rejects via
`return false` with no map-flag write).

#### `handleInvButton`

**Position:** after `delayed` check, after payload-length check, before inv-
listener resolution. Matches `InvButtonHandler.ts:14-31`.

```go
// (existing) delayed + payload-length + decode comId
com := s.lookupComponent(comId)
if com == nil {
    return nil
}
if !p.IsComponentVisible(com) {
    return nil
}
if com.Iop == nil || op-1 < 0 || op-1 >= len(com.Iop) || com.Iop[op-1] == "" {
    return nil
}

// (existing) listener / inv / HasAt / lastItem / lastSlot ...

trigger := script.TriggerInvButton1 + script.ServerTriggerType(op-1)
sf := s.scriptProvider.GetByTrigger(trigger, comId, -1)
root := s.lookupComponent(com.RootLayer)
protect := root == nil || !root.Overlay
s.runScript(sf, p, nil, protect, nil, nil)
```

#### `handleInvButtonD`

**Position:** after payload-length + decode, before inv-listener resolution
(BEFORE the `delayed` check, matching the unusual TS ordering at
`InvButtonDHandler.ts:16-44` — TS positions the visual-revert delayed-gate AFTER
the inv-listener gates so a delayed-but-malformed message still drops silently).

```go
// (existing) payload-length + decode comId/slot/targetSlot
com := s.lookupComponent(comId)
if com == nil || !com.Draggable {
    return nil
}
if !p.IsComponentVisible(com) {
    return nil
}

// (existing) listener / inv / slot bounds / Get(slot) / delayed-revert /
// lastSlot / lastTargetSlot ...

sf := s.scriptProvider.GetByTrigger(script.TriggerInvButtonD, comId, -1)
root := s.lookupComponent(com.RootLayer)
protect := root == nil || !root.Overlay
s.runScript(sf, p, nil, protect, nil, nil)
```

## §3. Test strategy

### §3.1 Op*T/U handlers — table-driven helper

Land a new test file `modules/world/handler_component_gate_test.go` housing a
shared helper used by all 8 Op*T/U driver tests.

Helper shape (illustrative — plan-author finalizes the signature):

```go
type compGateCase struct {
    name        string
    handler     func(*Player, []byte) error
    payloadOK   []byte           // valid payload; com bits at the right offset
    rootLayer   int              // RootLayer for the test component
    flagBits    int              // T-variant: ActionTarget bitmask. U-variant: 0.
    isUVariant  bool             // U: gate Usable. T: gate ActionTarget bits.
    setupOk     func(*Player, *Server)
}
```

Each driver test (one per Op*T/U handler) runs the helper through 4 scenarios:

1. **Nil component** — registry empty for the com id → expect UnsetMapFlag, no
   SetInteraction, opcalled false.
2. **Flag fail** — T: `ActionTarget = 0` (wrong bit cleared); U: `Usable=false`.
   → UnsetMapFlag, no SetInteraction.
3. **Not visible** — component registered but `RootLayer` not in any tab/modal
   slot → UnsetMapFlag, no SetInteraction.
4. **Happy path** — all gates pass → SetInteraction called, opcalled true.

The 4 scenarios test the gate; existing handler-specific tests (slot bounds,
listener resolution, etc.) remain in their files and are updated to seed
components per §3.3.

### §3.2 InvButton family — per-handler explicit + protect helper

Per-handler reject tests (inline in `handler_inv_button_test.go`):

- `TestHandleInvButton_NilComponentRejects`
- `TestHandleInvButton_NoIopAtOpRejects`
- `TestHandleInvButton_NotVisibleRejects`
- `TestHandleInvButtonD_NilComponentRejects`
- `TestHandleInvButtonD_NotDraggableRejects`
- `TestHandleInvButtonD_NotVisibleRejects`

Each asserts the handler returned without writing `lastItem`/`lastSlot`/
`lastTargetSlot` and without invoking `runScript`.

Protect tests use a shared helper lifted from NAI-59 T6's
`runIfButtonProtectScript` (`handler_interface_test.go:294-321`). The original
helper stays in place for IfButton tests; the new file gets a generalised
variant:

```go
// runProtectScript registers a script for (trigger,com) that runs P_DELAY
// (requires Protect=true), invokes handlerFn against a Server seeded with
// rootLayer fixture, and reports whether the active script suspended
// (handler computed protect=true → P_DELAY suspended OK) or aborted
// (handler computed protect=false → requireProtectedActivePlayer rejected).
func runProtectScript(
    t *testing.T,
    trigger script.ServerTriggerType,
    comId int,
    rootOverlay bool,
    includeRoot bool,
    handlerFn func(*Player) error,
) bool
```

Three protect cases per Inv handler:

- `TestHandleInvButton_OverlayRootSetsProtectFalse` — Overlay=true → protect=false → script aborts.
- `TestHandleInvButton_NonOverlayRootSetsProtectTrue` — Overlay=false → protect=true → script suspends.
- `TestHandleInvButton_NilRootSetsProtectTrue` — root unregistered → protect=true → script suspends.
- (Same three for `_InvButtonD_*`.)

### §3.3 Existing test sites — compatibility audit

The following existing handler tests run the now-gated handlers without seeding
component types. They will start failing once the gate lands. Plan-author
re-greps these at plan-write time AND each task's controller-preflight pass
re-grep verifies the list before implementer dispatch (per memories
`enumerate_all_sites`, `controller_preflight`).

Initial enumeration at HEAD `57c2212` (plan-author refreshes):

- `modules/world/handler_oploc_test.go` — OpLocT/OpLocU happy-path tests.
- `modules/world/handler_opnpc_test.go` — OpNpcT/OpNpcU happy-path tests.
- `modules/world/handler_opobj_test.go` — OpObjT/OpObjU happy-path tests.
- `modules/world/handler_op_player_test.go` — OpPlayerT/OpPlayerU happy-path tests.
- `modules/world/handler_inv_button_test.go` — InvButton/InvButtonD happy-path tests.

For each affected test the plan code block must:

- Add `seedComponentTypes(...)` with the listener `comId`'s `RootLayer`,
  `ButtonType` (Button or ButtonNone-but-with-flag-set as appropriate),
  `ActionTarget` (T-variant) or `Usable` (U-variant), `Iop` (InvButton),
  `Draggable` (InvButtonD).
- Set `p.tabs[0] = rootLayer` (or seed a modal slot) so `IsComponentVisible`
  passes.

### §3.4 Test helper — `seedComponentTypes` reuse

The existing helper at `handler_interface_test.go:131-148` is `package world`-
scoped and reusable across all test files in the package without relocation.
New test files reference it directly.

## §4. Tasks

### T1 — T-variant gates (4 sites)

Wire the §2.1 gate at the TS-mandated position in:

- `handleOpLocT` (`modules/world/handler_oploc.go`) — flag `ComActionTargetLoc`
- `handleOpNpcT` (`modules/world/handler_opnpc.go`) — flag `ComActionTargetNpc`
- `handleOpObjT` (`modules/world/handler_opobj.go`) — flag `ComActionTargetObj`
- `handleOpPlayerT` (`modules/world/handler_op_player.go`) — flag `ComActionTargetPlayer`

Each site:

- Replace the deviation comment block with new gate-explainer comments
  (NAI-59 T6 precedent: deviation comment fully deleted, gate self-documents).
- Update the existing happy-path test in the corresponding `_test.go` to seed
  components.
- Add the driver test in `handler_component_gate_test.go` (4 scenarios per
  handler, via shared helper).

### T2 — U-variant gates (4 sites)

Wire the §2.2 gate at the TS-mandated position in:

- `handleOpLocU` — gate after entity lookup, before listener resolution
- `handleOpNpcU` — gate after delayed, before any lookup
- `handleOpObjU` — gate after entity lookup, before listener resolution
- `handleOpPlayerU` — gate after delayed, before any lookup

Same per-site pattern as T1: replace deviation comment, update existing happy-
path tests, add driver test.

### T3 — InvButton family (2 sites)

Wire the §2.3 gate + protect computation in `handleInvButton` and
`handleInvButtonD`. Replace deviation comments. Add 6 reject tests + 6 protect
tests (3 per handler) using the lifted `runProtectScript` helper. Update the
existing happy-path tests to seed components and (per InvButton's behavior) to
provide a non-Overlay root so `protect=true` matches existing assertions.

### T4 — Close commit

Retire stale narrative cluster-mention sites that survived the per-task tag
deletions. Initial enumeration (plan-author re-greps and refreshes):

- `modules/world/handler_opnpc.go:97-103` — narrative refs to S6m-D1
- `modules/world/handler_opnpc.go:177-178` — narrative refs to S6m-D2
- `modules/world/handler_op_player.go:74-80` and `:134-137` — narrative refs to S6o-D1/D2
- `modules/world/handler_opobj.go:95-98` and `:159-161` — narrative refs to S6m-*, NAI-48-D1
- `modules/world/handler_oploc_test.go:430` — stale `(S6m-D2/D3)` test docstring

Close commit body includes:

- `Closes:` trailer enumerating all 10 retired deviation sites.
- `Closes memory:` trailer (per `close_commit_memory_trailer`).

## §5. Risks & verification

**No new deviations expected.** Gates fully port the TS validation path. The
`ComActionTarget*` constants land their first production consumers; per
`consume_reserved_constant`, the new consumer owns the full dispatch path and
must verify constant values against TS `Component.ts:321-327` at plan-write.

**Risks.**

- **Per-handler gate position varies** (T-variants: pre-lookup; U-variants for
  Obj/Loc: post-lookup; U-variants for Npc/Player: pre-lookup). Plan code
  blocks must show insertion point with surrounding-line context per site.
- **Existing happy-path tests will break** at the new gate since they don't seed
  components. Plan-author re-greps every affected test file at plan-write time
  and the controller re-greps at each task's pre-flight (per `controller_preflight`,
  `enumerate_all_sites`).
- **Stale narrative rot** in cluster-mention comments needs full sentence-level
  retirement, not just tag-substring patching (per `retire_deviation_grep_all_comments`).
  Several handler doc comments reference cluster siblings via narrative phrases
  (e.g., "bundle with S6m-D1") that go stale after this sub-spec.
- **InvButtonD gate position** is unusual (BEFORE the delayed-revert gate to
  match TS). Plan code block must show the full surrounding pattern so the
  implementer doesn't `replace_all` it into the wrong slot.

**Verification.**

- Standard NAI-N: `go test ./...` package-wide post each task.
- Controller pre-flight grep+Read pass before each implementer dispatch (per
  `controller_preflight` memory).
- Post-commit re-grep of plan-listed sites against new HEAD (per
  `enumerate_all_sites`).

**Smoke test NOT required.** Gates are pure-rejection paths with no positive
output side effects. The user can exercise spell-on-X / drag-item flows in the
Java client post-merge if desired (per `smoke_test_server_handoff` memory: only
positive-side-effect flows mandate the user-launched smoke).

## §6. Tech stack

- Go 1.26+ (per `go_version` memory).
- Engine-TS reference at `/home/owner/Code/github.com/LostCityRS/Engine-TS`
  only (per `ts_source_canonical_path`).

## §7. Cadence

Standard NAI-N: spec → plan → emit resume prompt → user `/clear` → subagent-
driven TDD per task (per `superpowers_clear_between_spec_and_impl`,
`execution_mode_default`). Tasks T1-T4 dispatched serially; each implementer
cycle includes the standard plan-test-coverage and verification gates.
