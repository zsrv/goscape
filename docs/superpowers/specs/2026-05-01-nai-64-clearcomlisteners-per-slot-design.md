# NAI-64 — Per-slot `clearComListeners` + `invStopListenOnCom` packet collapse

**Status:** spec written 2026-05-01.
**Cadence:** full (≤~70 production LOC + ≤~150 test LOC; one bundle, ~3-4 tasks).
**Predecessors:** NAI-53 (CloseModal full port), NAI-59 (ComponentType config + RootLayer field), NAI-60 (component-registry input-gate cluster cleanup).
**TS source:** `Engine-TS/src/engine/entity/Player.ts`.
**Tech stack:** Go 1.26+.

---

## 1. Closure ledger

### Closes

- **`NAI-53-D-CLEARCOMLISTENERS-PER-SLOT`** (`modules/world/player_script.go:633`).
  TS `(*Player).closeModal` calls `clearComListeners(slotCom)` per slot
  (Player.ts:767, 778, 789), filtering `invListeners` by
  `Component.rootLayer`. Goscape's current `(*Player).encodeOut` instead
  bulk-removes ALL `invListeners` whenever `refreshModalClose` is set
  (`player.go:296-308`). NAI-60 deferred this site explicitly because
  the per-listener filter is structurally distinct from NAI-60's input-gate
  cluster pattern. The ComponentType registry blocker was retired by NAI-59
  (`pkg/objtype/componenttype.go:49` `RootLayer int`).

### Untracked closure (close-commit provenance, no new tag)

- **`(*Player).invStopListenOnCom` packet-write miss.** TS Player.ts:1464-1471
  removes the listener AND writes `UpdateInvStopTransmit(com)` atomically.
  Goscape's `(*Player).invStopListenOnCom` (`player.go:825-827`) only
  removes the map entry; the packet only fires from the bulk
  `(*Player).encodeOut` path on `refreshModalClose`. Practical effect:
  scripts calling the `INV_STOPTRANSMIT` opcode (`pkg/script/handlers_inv.go:441`)
  silently stop server-side updates without notifying the client. The
  client keeps the inventory window registered until the next bulk-clear
  at modal-close, leaving stale data on screen.

  Surfaced during NAI-64 brainstorm; retired in this same spec because
  the per-slot filter pattern naturally re-routes through
  `invStopListenOnCom` and the bulk path is being deleted. Closing both in
  one bundle is more coherent than splitting (the helper trivially writes
  the right packets only when the underlying primitive is already
  TS-faithful). Retirement narrated in close-commit body per
  `audit_arithmetic_correction_in_rollup.md`. No new deviation tag opened
  since none was filed for this miss.

### Opens

- None.

### Out of scope

- **IF_OPENTUT tutorial branch.** TS Player.ts:717-723 calls
  `clearComListeners(this.modalTutorial)`. Goscape has no `modalTutorial`
  modal-close branch; tracked separately under `NAI-59-D-MODALTUTORIAL-NO-PRODUCER`.
- **Other component-registry consumers.** NAI-60 closed the cluster's
  input-gate sites (Op*T, Op*U, InvButton). All remaining open sites are
  conditional / blocked / tracked elsewhere.

---

## 2. TS source reference

### `Player.clearComListeners` — TS Player.ts:728-739

```typescript
clearComListeners(root: number) {
    if (root == -1) {
        return;
    }

    for (let i = 0; i < this.invListeners.length; i++) {
        const { com } = this.invListeners[i];
        if (Component.get(com).rootLayer === root) {
            this.invStopListenOnCom(com);
        }
    }
}
```

### `Player.invStopListenOnCom` — TS Player.ts:1464-1471

```typescript
invStopListenOnCom(com: number) {
    const index = this.invListeners.findIndex(l => l.com === com);
    if (index === -1) {
        return;
    }

    this.invListeners.splice(index, 1);
    this.write(new UpdateInvStopTransmit(com));
}
```

### `Player.closeModal` per-slot calls — TS Player.ts:761-791

Each of three slots (`modalMain`, `modalChat`, `modalSide`) does, in order:
fire `IF_CLOSE` trigger (if registered) → `clearComListeners(slot)` → reset
slot to `-1`.

---

## 3. Architecture

Three production sites change in `modules/world/`. No new files; one new
method on `*Player`.

### 3.1 `(*Player).invStopListenOnCom` — collapse to TS shape

File: `modules/world/player.go` (~line 825).

Mirrors TS Player.ts:1464-1471: missing-key early-return, then delete
+ packet write.

```go
// invStopListenOnCom unregisters the listener at the given component
// ID and notifies the client. No-op if no listener exists there
// (matches Go delete-on-nil-map plus TS L1466-1468 early-return).
//
// Mirrors TS Player.ts:1464-1471. Callers must ensure p.client is
// non-nil; sendUpdateInvStopTransmit dereferences p.client without a
// guard (existing convention shared by every wire-write in this package).
func (p *Player) invStopListenOnCom(com int) {
    if _, ok := p.invListeners[com]; !ok {
        return
    }
    delete(p.invListeners, com)
    sendUpdateInvStopTransmit(p, com)
}
```

The early-return both:
- preserves goscape's existing nil-map / missing-key no-op contract
  (the underlying tests `TestInvStopListenOnComNoopForMissingKey` and
  `TestInvStopListenOnComNoopForNilMap` continue to pass: a nil map is
  also missing-key for any com), AND
- mirrors TS L1466-1468 — only write the packet when there was actually
  a listener to remove (otherwise the client gets a packet for a
  listener it never knew existed).

### 3.2 `(*Player).clearComListeners` — new helper

File: `modules/world/player.go` (immediately after `invStopListenOnCom`).

```go
// clearComListeners removes every inv-listener whose Component.RootLayer
// equals rootCom and writes UpdateInvStopTransmit per removal. No-op
// when rootCom is -1 (slot was unset, mirrors TS L729-731). No-op when
// the player has no Server bound (goscape defensive; TS skips this
// check since TS Components are a global singleton).
//
// Mirrors TS Player.ts:728-739. Closes NAI-53-D-CLEARCOMLISTENERS-PER-SLOT.
func (p *Player) clearComListeners(rootCom int) {
    if rootCom == -1 {
        return
    }
    if p.client == nil || p.client.server == nil {
        return
    }
    s := p.client.server
    for com := range p.invListeners {
        c := s.lookupComponent(com)
        if c == nil {
            // goscape defensive; TS assumes Component.get(com) returns
            // non-nil. Skip unknown components rather than panic.
            continue
        }
        if c.RootLayer == rootCom {
            p.invStopListenOnCom(com)
        }
    }
}
```

**Iteration safety.** Go's specification guarantees that `delete` during
`for k := range m { ... delete(m, k) }` is well-defined: deleted keys are
either yielded once and then dropped, or never yielded. No panic, no
duplicate visits. Calling `invStopListenOnCom` (which calls `delete`)
inside the range loop is safe.

### 3.3 `(*Player).encodeOut` — drop the blanket clear

File: `modules/world/player.go:296-308`.

Before:

```go
if modalChanged {
    if p.refreshModalClose {
        p.writeOut(gameserver.OpIfClose, nil)
        // Stop transmitting every currently-registered inv.
        // Approximation: TS only stops listeners bound to the closing
        // modal's components; we don't yet have a component-to-modal
        // mapping, so clear all. Re-registered on next modal open.
        for _, l := range p.invListeners {
            sendUpdateInvStopTransmit(p, l.Com)
        }
        clear(p.invListeners) // Go 1.21+ map reset; keeps allocated buckets
    }
    p.refreshModalClose = false
    ...
}
```

After:

```go
if modalChanged {
    if p.refreshModalClose {
        p.writeOut(gameserver.OpIfClose, nil)
    }
    p.refreshModalClose = false
    ...
}
```

The for-loop and `clear` are deleted. Listener removal + per-listener
`UpdateInvStopTransmit` packets now come from `CloseModal` →
`clearComListeners` → `invStopListenOnCom`, all fired before `encodeOut`
runs. The `OpIfClose` write itself stays — it is a separate client-facing
modal-close wire event, not a listener event.

### 3.4 `(*Player).CloseModal` — wire `clearComListeners` per slot

File: `modules/world/player_script.go:660-679` (the per-slot dispatch
block).

Current:

```go
if p.client != nil && p.client.server != nil {
    s := p.client.server
    if p.modalMain != -1 {
        p.runIfCloseTrigger(s, p.modalMain)
        p.modalMain = -1
    }
    if p.modalChat != -1 {
        p.runIfCloseTrigger(s, p.modalChat)
        p.modalChat = -1
    }
    if p.modalSide != -1 {
        p.runIfCloseTrigger(s, p.modalSide)
        p.modalSide = -1
    }
} else {
    p.modalMain = -1
    p.modalChat = -1
    p.modalSide = -1
}
```

After (each slot gains a `clearComListeners` call between trigger and reset):

```go
if p.client != nil && p.client.server != nil {
    s := p.client.server
    if p.modalMain != -1 {
        p.runIfCloseTrigger(s, p.modalMain)
        p.clearComListeners(p.modalMain)
        p.modalMain = -1
    }
    if p.modalChat != -1 {
        p.runIfCloseTrigger(s, p.modalChat)
        p.clearComListeners(p.modalChat)
        p.modalChat = -1
    }
    if p.modalSide != -1 {
        p.runIfCloseTrigger(s, p.modalSide)
        p.clearComListeners(p.modalSide)
        p.modalSide = -1
    }
} else {
    p.modalMain = -1
    p.modalChat = -1
    p.modalSide = -1
}
```

Order matches TS Player.ts:761-791 exactly: trigger → clearComListeners →
slot-reset.

The DEVIATION block at `player_script.go:633-637` retires (delete the
comment; replace with TS-citation in the doc-comment header per
`retire_deviation_grep_all_comments.md`).

The no-server `else` branch keeps its bare slot resets — without a Server,
`runIfCloseTrigger` already short-circuits and `clearComListeners` is also
no-op (it reads `s.lookupComponent`).

---

## 4. Data flow

### Before NAI-64

**Script `inv_stoptransmit(149)`:**

```
handleInvStopTransmit
  → s.Self.InvStopListenOnCom(149)
    → (*Player).invStopListenOnCom(149)
      → delete(p.invListeners, 149)
   ⚠ NO PACKET. Client keeps inv 149 listener live; server stops sending
     updates; window goes stale until bulk-clear at next refreshModalClose.
```

**Modal close (`CloseModal(true)` with all three slots set):**

```
CloseModal
  → IF_CLOSE for modalMain, modalChat, modalSide
  → refreshModalClose = true
encodeOut
  → write OpIfClose
  → for every listener: sendUpdateInvStopTransmit (BLANKET)
  → clear(invListeners)
```

### After NAI-64

**Script `inv_stoptransmit(149)`:**

```
handleInvStopTransmit
  → s.Self.InvStopListenOnCom(149)
    → (*Player).invStopListenOnCom(149)
      → delete(p.invListeners, 149)
      → sendUpdateInvStopTransmit(p, 149)  ← packet now wired
```

**Modal close (`CloseModal(true)` with all three slots set):**

```
CloseModal
  → IF_CLOSE for modalMain
  → clearComListeners(modalMain)
       → for each listener with Component.RootLayer == modalMain:
              invStopListenOnCom(com)   // delete + packet
  → IF_CLOSE for modalChat → clearComListeners(modalChat)
  → IF_CLOSE for modalSide → clearComListeners(modalSide)
  → refreshModalClose = true
encodeOut
  → write OpIfClose
  (no blanket clear — listeners already removed per-slot)
```

**Wire-behaviour delta.** Listeners whose `RootLayer` is not part of any
closing modal slot survive. Concretely: a sidebar-anchored inv listener
remains live across a chat-only `CloseModal`. Previously the bulk clear
zeroed every listener regardless of which slot was closing.

---

## 5. Edge cases

| Case | Behaviour |
|---|---|
| `clearComListeners(-1)` | early-return (TS L729-731) |
| `p.client == nil` (test path) | `clearComListeners` early-returns, `CloseModal` already takes the no-server branch |
| `p.client.server == nil` | same as above |
| Component lookup miss (`s.lookupComponent(com) == nil`) | goscape-defensive `continue` (skip listener); TS assumes non-nil. Doc-comment label per `defensive_gate_doc_comment_label.md` |
| Empty `invListeners` | `range` over empty map is zero iterations — no-op |
| `invListeners == nil` | same — `range` over nil map yields zero iterations |
| Listener `com == rootCom` (RootLayer points at itself) | included in match (TS does the same: `Component.get(com).rootLayer === root` may be true when `com === root`) |
| `delete` during `range` | well-defined in Go; deleted keys are not re-yielded |

---

## 6. Test plan

### 6.1 New tests

Location: `modules/world/player_inv_test.go` (or split into
`player_clearcomlisteners_test.go` if locality clarity wins).

**`(*Player).invStopListenOnCom` packet emission:**

1. `TestInvStopListenOnComWritesUpdatePacket`
   - Set up `p, conn := newTestPlayer(t)`.
   - `p.invListenOnCom(93, 149, -1)`.
   - `p.invStopListenOnCom(149)`.
   - Drain `conn`; assert exactly one packet with opcode
     `OpUpdateInvStopTransmit` and payload `[0x00, 0x95]` (149 P2 big-endian).
   - Assert `len(p.invListeners) == 0`.

2. `TestInvStopListenOnComMissingKeyWritesNoPacket`
   - `newTestPlayer`; do NOT register any listener.
   - `p.invStopListenOnCom(149)`.
   - Drain `conn`; assert zero packets. Pins TS L1466-1468 early-return.

3. `TestInvStopListenOnComNilMapWritesNoPacket`
   - `newTestPlayer`; `p.invListeners` is nil at construction.
   - `p.invStopListenOnCom(149)`.
   - Assert no panic; `p.invListeners` still nil; drain conn; zero packets.

**`(*Player).clearComListeners` filtering:**

4. `TestClearComListenersFiltersByRootLayer`
   - `newTestPlayer` (provides Server-bound Player); seed registry via
     existing `seedComponentTypes(t, s, map[int]*objtype.ComponentType{
       149: {RootLayer: 100}, 200: {RootLayer: 999}})` helper.
   - Register both listeners (`p.invListenOnCom(...)`).
   - `p.clearComListeners(100)`.
   - Assert listener 149 removed, listener 200 retained, exactly one
     `OpUpdateInvStopTransmit(149)` packet on the wire.

5. `TestClearComListenersRootMinusOneNoOp`
   - Same fixture as #4 but call `p.clearComListeners(-1)`.
   - Assert both listeners retained, zero packets. Pins TS L729-731.

6. `TestClearComListenersUnknownComponentSkipped`
   - Register a listener at `com=9999` with no Component config registered
     (or registered but absent from the registry).
   - `p.clearComListeners(9999)`.
   - Assert listener still present, zero packets. Pins the goscape-
     defensive nil-Component skip.

7. `TestClearComListenersRemovesMultipleSiblings`
   - Register three listeners `{a, b, c}` all with `RootLayer == 50`,
     plus one with `RootLayer == 60`.
   - `p.clearComListeners(50)`.
   - Assert the three matched listeners removed, the unrelated one retained,
     three `OpUpdateInvStopTransmit` packets on the wire (any order). Pins
     iteration safety + multi-removal correctness.

**Modal-close integration:**

8. `TestCloseModalClearsOnlyListenersForClosingSlots`
   - Player with `modalMain = 100`, `modalChat = -1`, `modalSide = 200`.
   - `seedComponentTypes` with `{149: {RootLayer: 100}, 250: {RootLayer: 200}, 300: {RootLayer: 999}}`.
   - Register listeners `{149, 250, 300}`.
   - Call `p.CloseModal(true)`.
   - Assert: 149 removed, 250 removed, 300 retained;
     `OpUpdateInvStopTransmit` fired for 149 and 250 only;
     one `OpIfClose` packet from `encodeOut` after a manual encodeOut call
     (or assert `refreshModalClose == true` and skip encodeOut for the unit
     scope).

9. `TestCloseModalNoListenersStillClosesAndWritesIfClose`
   - Player with `modalMain = 100`, no listeners.
   - `p.CloseModal(true)`.
   - Assert no `OpUpdateInvStopTransmit` packets; manual `p.encodeOut()`
     produces exactly one `OpIfClose`.

### 6.2 Existing tests to update

- `TestInvStopListenOnComRemovesListener` (`player_inv_test.go:88`):
  add `drainConn` step + one extra assertion that the
  `OpUpdateInvStopTransmit(149)` packet was written.
- `TestInvStopListenOnComNoopForMissingKey` (`player_inv_test.go:112`):
  add a drain + zero-packet assertion (now covered by new test #2; this
  one becomes a duplicate-coverage tightening — acceptable per
  `ts_asymmetry_dual_pin.md`).
- `TestInvStopListenOnComNoopForNilMap` (`player_inv_test.go:125`):
  same — add zero-packet assertion.
- Any existing `encodeOut`-on-`refreshModalClose` test that asserts
  the blanket-clear behaviour: remove the bulk-clear assertions, retain
  the `OpIfClose` assertion. Plan-author must grep these out before
  dispatch.
- `pkg/script/handlers_inv_test.go` `TestInvStopTransmit*`: mock-based,
  no real packet plumbing. No change required.

### 6.3 Implementation-time grep targets

The implementer must run:

```
rg -n "for _, l := range p.invListeners" modules/world/
rg -n "clear\(p.invListeners\)" modules/world/
rg -n "\binvStopListenOnCom\b" modules/world/ pkg/
rg -n "NAI-53-D-CLEARCOMLISTENERS-PER-SLOT" pkg/ modules/ cmd/
```

The deviation tag must reach zero hits at close. `invStopListenOnCom`
production callers must all be reviewed for nil-conn safety
(`pkg/script/handlers_inv.go:449` is the script-side caller; reaches
`(*Player).InvStopListenOnCom` → unexported `invStopListenOnCom` →
`sendUpdateInvStopTransmit` → `writeOut`. Script execution always
implies a connected client.).

---

## 7. Cadence and bundle plan

Full cadence; one bundle, three implementation tasks plus close.

- **T1**: collapse `(*Player).invStopListenOnCom` to TS shape;
  add `clearComListeners` helper; new tests #1-#7 above; update existing
  `invStopListenOnCom` tests; do NOT yet wire `clearComListeners` into
  `CloseModal` (keeps T1 isolated — bulk path still active, so no
  behavioural drift yet). Per-task two-stage review.

- **T2**: wire `clearComListeners` into `CloseModal`; drop the
  `encodeOut` blanket-clear loop + `clear(invListeners)`; retire the
  DEVIATION block at `player_script.go:633`; new tests #8-#9 above;
  update existing `encodeOut`-on-modal-close tests. Per-task two-stage
  review.

- **T3**: cross-test polish + close commit. Run the deviation-tag grep;
  update `nai_followups.md` with the NAI-64 close section + `Closes memory:`
  trailer per `close_commit_memory_trailer.md`.

The two-task split is deliberate: T1 keeps the bulk path live so its
tests are independently meaningful; T2 swaps producers atomically. This
avoids the migration-boundary trap from `latent_bug_at_migration_boundary.md`.

---

## 8. Risks

- **Wire-pattern observability.** A modal close that previously emitted N
  `OpUpdateInvStopTransmit` packets (one per registered listener regardless
  of root) now emits only K packets (one per listener whose RootLayer
  matches a closing slot). Different packet pattern over the wire,
  TS-faithful. Downstream client behaviour unchanged or improved (sidebar
  inv windows correctly survive chat-modal-close).

- **Test-fixture ripple.** Several tests rely on the blanket-clear at
  `encodeOut` to wipe listener state as a side-effect. Each must be
  updated. Controller pre-flight should grep for `invListeners` + test
  files before T2 dispatch.

- **`writeOut` nil-conn dereference.** `sendUpdateInvStopTransmit` calls
  `(*Player).writeOut` which dereferences `p.client.encryptor` without
  guard. Every existing wire-write in this package shares this convention.
  Production callers always have a client; test callers go through
  `newTestPlayer` which provides one. The new code paths preserve this
  contract — `clearComListeners` early-returns on `p.client == nil`,
  and `invStopListenOnCom`'s direct callers (script handler, modal-close
  helper, tests) all guarantee a client.

---

## 9. Memory cross-checks (provenance applied to this spec)

- `runescript_cadence.md` — full cadence; spec → plan → subagent-driven TDD with two-stage review.
- `true_to_ts_gate.md` — every behavioural change cited against TS source.
- `enumerate_all_sites.md` — sites enumerated: 1 deviation tag site + 3 production sites + ≥4 tests; grep targets in §6.3.
- `dead_api_polish.md` — bulk-clear in `encodeOut` is the dead path post-T2; deleted in same task that activates the per-slot path (no orphan window).
- `latent_bug_at_migration_boundary.md` — T1/T2 split deliberately keeps bulk path live during T1 to avoid the dual-path-window trap.
- `retire_deviation_grep_all_comments.md` — T2 must `rg "NAI-53-D-CLEARCOMLISTENERS-PER-SLOT"` to zero across `pkg/ modules/ cmd/`.
- `defensive_gate_doc_comment_label.md` — `clearComListeners` Component-nil and no-server skips are labelled "(goscape defensive; TS skips ...)".
- `audit_arithmetic_correction_in_rollup.md` — close commit narrates the
  untracked `invStopListenOnCom` packet-miss retirement with TS citation
  (Player.ts:1464-1471) and pre-NAI-64 grep evidence.
- `controller_preflight.md` — pre-T1, pre-T2, pre-T3 controller passes
  must verify: `(*Player).writeOut` location and nil-conn dereference;
  `s.lookupComponent` signature; `ComponentType.RootLayer` type; existing
  `invStopListenOnCom` test bodies; the location of any `encodeOut` test
  that asserts the bulk-clear behaviour.
- `close_commit_memory_trailer.md` — close commit carries `Closes memory:`
  trailer with the deviation tag and the untracked-retirement provenance.
- `verify_implementer_claims.md` — at each T-N close, run independent
  `go vet ./...` + `go test ./...` and confirm `git show <SHA> --stat`
  matches the stated scope.

---

## 10. Net deviation tally

Closes: NAI-53-D-CLEARCOMLISTENERS-PER-SLOT (-1).
Opens: 0.
Net delta: **-1** from the running count at NAI-63 close.

The untracked `invStopListenOnCom` packet-miss retirement is not in
the deviation count (no tag was ever filed) but is narrated in the close
commit body for grep-discoverable provenance. The close commit will
re-derive the absolute count from the live tag inventory at close.
