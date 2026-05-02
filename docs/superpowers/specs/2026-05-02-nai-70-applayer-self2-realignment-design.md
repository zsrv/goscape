# NAI-70 — AP-Player / OP-Player Self/Self2 binding realignment to TS

**Status:** Spec written 2026-05-02.
**Predecessor:** NAI-69 (HEAD `54930db`). Net deviation tally entering: 13.
**Closes:** `NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY`.
**Tech stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`.

## 1. Background

NAI-39 introduced goscape's first producer of `state.Self2 +
PtrActivePlayer2`: `buildPlayerScriptState`'s case-`script.ActivePlayer`
arm at `modules/world/script.go:55-59`. The producer dispatch is
correctly shaped — when a script's `target` argument is a `*Player`,
it sets `state.Self2 = target` + `Pointers |= PtrActivePlayer2`.

The two callers wired into this producer — `fireOpTriggerPlayer` and
`fireApTriggerPlayer` (`modules/world/player_interaction_trigger.go:42, 99`)
— pass arguments **reversed** relative to TS:

```go
// Current (NAI-39 era):
srv.runScript(sf, target, p, true, nil, nil)
//                  ^^^^^^  ^   self   target
```

This produces `state.Self = target (clicked player)` and
`state.Self2 = p (clicker)`, the **opposite** of TS:

```ts
// TS Player.ts:1129 (OP) and :1151 (AP):
this.executeScript(ScriptRunner.init(opTrigger, this, target), true);
//                                              ^^^^  ^^^^^^
//                                              self  target
//                                              =clicker  =target
```

Following `ScriptRunner.init` (ScriptRunner.ts:84-87), TS produces
`state._activePlayer = self (clicker)` + `state._activePlayer2 = target`.

### Downstream consequences of the reversal

Every script handler that reads `state.Self` or `state.Self2` sees a
flipped binding for the OPPLAYER/APPLAYER trigger family:

- **`p_aprange` (handlers_player.go:695)** calls `s.Self.SetApRange(n)`.
  Goscape: mutates `target.apRangeCalled` and `target.apRange`. TS:
  mutates clicker's. This is what NAI-69 T3 surfaced — the new
  same-tick retry guard in `tryInteract` reads `clicker.apRangeCalled`,
  stays `false`, and AP-Player retry is structurally inert.
- **`hint_pl` (handlers_player.go:983)** calls
  `s.Self.HintPlayer(s.Self2.Slot())`. Goscape: HINT_ARROW packet lands
  on **target's** wire with **clicker.slot** in the body. TS: lands on
  clicker's wire with target.slot.
- **NAI-62 OP/AP `targetSubject.com` override tests** drain target's
  conn for the `MES` marker; that works only because Self=target routes
  `MessageGame` to target's connection. Post-fix, `MES` writes to
  clicker's conn.

Every Self-side mutation in an OPPLAYER/APPLAYER script — `p_aprange`,
`p_setanim`, `p_damage`, `p_apheal`, `p_say`, `MessageGame`, etc. —
applies to the *wrong player* relative to TS.

### Why this stayed dormant

- No production OPPLAYER/APPLAYER scripts ship in goscape (engine-only
  repo; LostCity content scripts live downstream).
- NAI-39 closure tests pinned the reversed binding directly
  (`TestFireOpTriggerPlayer_BindsSelf2ToClicker`) — locking in the
  divergence as the documented contract.
- The asymmetry only surfaced in NAI-69 T3 because `p_aprange`'s
  `apRangeCalled` mutation became newly observable through the
  `tryInteract` retry guard.

### Risk-register premise note

NAI-69's spec assumed AP-Player would activate the retry path "for
free" via the shared `srv.runScript` plumbing — a same-`*Player`-state
claim that didn't account for the producer-side reversal. Discovering
this at T3 cost a re-dispatch and opened the NAI-70 follow-up.
Reinforces `risk_register_premise_grep.md`: cross-call-chain state
claims need actual grep+Read of `state.Self =` sites.

## 2. Goal

Realign `fireOpTriggerPlayer` and `fireApTriggerPlayer` to TS binding
(Self=clicker, Self2=target) by swapping the `srv.runScript` arg order
at two sites. Activates AP-Player same-tick retry, restores TS-true
behavior across all OPPLAYER/APPLAYER handler dispatch sites.

Closes `NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY`. Opens nothing.
**Net tally: 13 → 12.**

## 3. Out of scope

- **OP/AP-Player handler audit beyond the three sites currently pinned
  by tests (HintPl, ApRange, MES)**. The realignment fixes ALL Self-
  side handlers uniformly; auditing each one in this sub-spec adds
  noise. Tests at the three covered sites prove the binding flip;
  remaining handlers inherit correctness from the producer-binding fix.
- **`interactionFired` field removal** (carried over from NAI-69 §3).
- **NAI-39 producer rework**. The `case script.ActivePlayer:` arm at
  `script.go:55-59` is already TS-correct as written — it sets
  Self2=`target` (its second arg). The fix is at the *call sites*, not
  the producer.
- **Smoke-test exercise.** Per `smoke_test_server_handoff.md`, smokes
  must be user-launched. Test inversions cover the binding flip end-to-
  end at the wire-bytes level (HINT_ARROW conn drain, MES marker conn
  drain). No production OPPLAYER/APPLAYER scripts to smoke against.

## 4. Behavior delta

**Before NAI-70:**

- **`p_aprange` in APPLAYER script** mutates `target.apRange` /
  `target.apRangeCalled`. Same-tick retry guard at
  `interaction.go:336` reads `clicker.apRangeCalled` (stays `false`).
  AP-Player skips retry. Clicker walks one tick wasted before retry
  (across-tick re-fire path), or fails the AP entirely if the script
  intended to halve the approach distance.
- **`hint_pl` in OPPLAYER script** sends HINT_ARROW to target's wire
  with clicker.slot — a player who clicks Bob to OP1 sees no hint
  arrow on their own client; Bob (the target) sees one pointing at the
  clicker. Backwards from TS.
- **`MES` / `MessageGame` in OP/APPLAYER script** appears in target's
  chatbox, not clicker's.

**After NAI-70:**

- `p_aprange` mutates `clicker.apRange` / `clicker.apRangeCalled`.
  `tryInteract`'s NAI-69 T1 guard fires; AP-Player gains same-tick
  retry, matching AP-Loc / AP-Obj.
- `hint_pl` lands on clicker's wire with target.slot — clicker sees
  the hint arrow pointing at their target. Matches TS.
- `MES` lands in clicker's chatbox. Matches TS.

For all three: **no production fallout** because no OPPLAYER/APPLAYER
content scripts run in goscape's CI; the change is observable only via
test inversions and (eventually) downstream content scripts.

## 5. Code map

| File | Change | LOC delta |
|---|---|---|
| `modules/world/player_interaction_trigger.go:76` | `srv.runScript(sf, target, p, ...)` → `srv.runScript(sf, p, target, ...)` | 1 prod |
| `modules/world/player_interaction_trigger.go:132` | Same swap | 1 prod |
| `modules/world/player_interaction_trigger.go:31-41` (`fireOpTriggerPlayer` doc header) | Drop reversed-binding narrative ("Self = target, Self2 = clicker"); restate as TS-true: "Self = `p` (clicker), Self2 = target. Mirrors TS Player.ts:1129 + ScriptRunner.ts:84-87." | doc only |
| `modules/world/player_interaction_trigger.go:84-98` (`fireApTriggerPlayer` doc header) | Drop the entire NAI-69-D narrative paragraph (lines 89-98). Restate Self2 binding as TS-true. Note "Same-tick retry path active per NAI-69 T1 guard at `interaction.go:336`." | doc only |
| `modules/world/player_interaction_trigger.go:122-125` (in-body comment) | Drop "Self=target binding" qualifier; reword as "TS Player.ts:1145-1162 AP save/clear/exec/capture/restore. NAI-68 framework." | doc only |
| `modules/world/script.go:30-34` (`buildPlayerScriptState` doc-comment) | Reword "secondary-binding arm consumed by …" — keep accurate, remove NAI-40-era flavor that hints at producer-direction ambiguity. The arm correctly sets Self2=target; rephrase to make that explicit. | doc only |
| `modules/world/interaction_trigger.go:42` (case-Player dispatch comment) | "case-ActivePlayer arm sets state.Self2 = clicker" → "state.Self2 = target" | doc only |
| `modules/world/script_test.go:1419-1421` (E2E test header narrative) | "Self=target, Self2=clicker → HINT_PL emits to target's outbound" → "Self=clicker, Self2=target → HINT_PL emits to clicker's outbound" | doc only (within test rewrite) |
| `modules/world/player_interaction_trigger_test.go` (6 tests) | Inversions per §7 | ~120 test |
| Deviation-tag retirement: `NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY` at 5 sites (verified §6) | Delete | doc only |

Production behavior change: **2 lines** (the two arg-order swaps).
Everything else is doc/test maintenance.

## 6. Pre-flight grep targets (controller_preflight)

Verified at HEAD `54930db`:

- `rg "NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY" modules/ pkg/`
  → 5 hits:
  - `player_interaction_trigger.go:98` (header doc)
  - `player_interaction_trigger.go:125` (in-body comment)
  - `player_interaction_trigger_test.go:311` (test header)
  - `player_interaction_trigger_test.go:348` (in-test comment)
  - `player_interaction_trigger_test.go:367` (test header)
  All 5 retire in T6.

- `rg "srv\.runScript\(sf, target, p" modules/world/` → 2 hits at
  `player_interaction_trigger.go:76, 132`. T1+T2 swap both.

- `rg "fireOpTriggerPlayer\|fireApTriggerPlayer" modules/world/` →
  call sites enumerated:
  - `interaction_trigger.go:43, 282` (dispatch from `tryFireOpTrigger` /
    `tryFireApTrigger`)
  - 4 test-file invocations (`player_interaction_trigger_test.go:206,
    235, 277, 337`; `interaction_trigger_nai68_test.go:230, 258`)
  No additional production callers.

- `rg "Self=target|Self2=clicker|Self2 = clicker" modules/ pkg/` →
  doc-comment narratives that need rewrite. 8 hits across
  `player_interaction_trigger.go` (3), `player_interaction_trigger_test.go`
  (4), `script_test.go:1421` (1). Enumerated in §5.

- `rg "TestFireOpTriggerPlayer_BindsSelf2ToClicker|TestOpPlayer1_E2E_HintPlOnTarget|TestFireApTriggerPlayer_ApRangeCalled|TestTryInteract_ApPlayer_NoSameTickRetry|TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom|TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom" modules/world/`
  → 6 hits (one per test). Inversions in §7.

- `rg "newPlayerTriggerFixture|makeOpPlayerFixtureWithBothConns" modules/world/`
  → fixture call sites enumerated. The first fixture only gives
  `target` a real conn+encryptor; tests #1 and #4 need clicker upgraded.
  `makeOpPlayerFixtureWithBothConns` (used by NAI-62 tests) already
  gives both players real conns — those tests just switch which conn
  they drain.

- `rg "DEVIATION NAI-69-D-APPLAYER-SELF2-REVERSED" modules/ pkg/` → 0
  hits. The tag exists only as `NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY`
  in narrative comments, not as `DEVIATION` inline tags. (Distinct from
  NAI-68's tag form.)

## 7. Test plan

Single TDD bundle. Tests pin the binding flip at every covered handler
dispatch site (HintPl wire conn + slot body, ApRange Self mutation,
MES Self conn).

### 7.1 Fixture upgrade — `newPlayerTriggerFixture`

`modules/world/player_interaction_trigger_test.go:66-83` currently
gives only `target` a real `net.Conn` + ISAAC encryptor. Tests #1 and
#4 below need clicker drained too. **Upgrade**: add a real conn +
encryptor for clicker. Use a distinct ISAAC seed (e.g.
`[4]uint32{5, 6, 7, 8}`) so wire-byte expectations don't accidentally
match the wrong side.

```go
func newPlayerTriggerFixture(t *testing.T) (s *Server, clicker, target *Player, clickerConn, targetConn net.Conn) {
    // ... existing setup ...
    clicker, clickerConn = newTestPlayer(t)
    clicker.client.server = s
    clicker.client.encryptor = io2.New([4]uint32{5, 6, 7, 8})
    clicker.slot = 1

    target, targetConn = newTestPlayer(t)
    target.client.server = s
    target.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    target.slot = 2
    // ...
}
```

Existing callers that ignore `clickerConn`: pass `_` in their
destructure. T1 will sweep all 4 call sites in
`player_interaction_trigger_test.go` (and 2 in
`interaction_trigger_nai68_test.go`).

### 7.2 Test inversions (T2 — direct producer-binding flip)

#### Test #1 — `TestFireOpTriggerPlayer_BindsSelf2ToClicker` → `BindsSelf2ToTarget`

`player_interaction_trigger_test.go:101`. Currently asserts:
- `targetConn` receives HINT_ARROW
- body slot bytes match `clicker.slot >> 8`, `byte(clicker.slot)`

**Post-fix invert:**
- Drain `clickerConn` (using new fixture upgrade)
- Body slot bytes match `target.slot >> 8`, `byte(target.slot)`
- ISAAC encryptor for parallel byte computation: clicker's seed
  `[4]uint32{5, 6, 7, 8}` (the one matching `clickerConn`)
- Rename test to `TestFireOpTriggerPlayer_BindsSelf2ToTarget`
- Rewrite test header narrative to reflect TS-true binding

#### Test #2 — `TestOpPlayer1_E2E_HintPlOnTarget` → `…OnClicker`

`script_test.go:1434`. Same wire-conn / body-slot inversion. Note:
this fixture (`makeOpPlayerFixture`) is separate from
`newPlayerTriggerFixture`; it builds clicker and target from
`newTestPlayer` and rewires only target's conn at line 1441-1446.
T2 must apply the same fixture pattern to clicker:

```go
freshClicker, clickerConn := newTestPlayer(t)
freshClicker.client.server = s
freshClicker.client.encryptor = io2.New([4]uint32{5, 6, 7, 8})
freshClicker.slot = clicker.slot
s.players[clicker.slot] = freshClicker
clicker = freshClicker
```

Drain `clickerConn`, expect `target.slot` bytes in body.
Rename: `TestOpPlayer1_E2E_HintPlOnClicker`.

#### Test #3 — `TestFireApTriggerPlayer_ApRangeCalled_BindsToTargetNotClicker` → `BindsToClicker`

`player_interaction_trigger_test.go:317`. Currently asserts:
- `clicker.apRangeCalled == false`
- `clicker.apRange == 10` (default)
- `target.apRangeCalled == true`
- `target.apRange == 2`

**Post-fix invert:**
- `clicker.apRangeCalled == true`
- `clicker.apRange == 2` (script-set new range)
- `target.apRangeCalled == false`
- `target.apRange == 10` (default unchanged)
- Rewrite test header narrative
- Rename: `TestFireApTriggerPlayer_ApRangeCalled_BindsToClicker`

#### Test #4 — `TestTryInteract_ApPlayer_NoSameTickRetry_DueToReversedSelf` → `…SameTickRetryActivates`

`player_interaction_trigger_test.go:369`. Currently asserts
`tryInteract` returns `true` (NAI-69 T1 guard inert under reversed
binding).

**Post-fix invert:**
- `tryInteract` returns `false` (NAI-69 T1 guard fires; same-tick
  retry path active)
- `clicker.interactionFired == false` (reset by `tryInteract`'s
  return-false arm)
- `clicker.apRangeCalled == true` (mutated by p_aprange via Self=clicker)
- `target.apRangeCalled == false` (untouched)
- `clicker.target == target` (waypoints+target restored by fire helper
  per NAI-68; tryInteract's return-false doesn't re-clear)
- Rewrite test header — narrative now describes activated retry path
- Rename: `TestTryInteract_ApPlayer_SameTickRetryActivates`

#### Test #5 — `TestFireOpTriggerPlayerOverridesTypeIdFromTargetSubjectCom` (NAI-62, OP)

`player_interaction_trigger_test.go:191`. Currently drains `cc2`
(other / target's conn). Fixture is `makeOpPlayerFixtureWithBothConns`
which already gives both players real conns.

**Post-fix invert:**
- Switch drain from `cc2` → `cc1` (clicker's conn)
- Switch flush call from `other.client.flushWrite()` → `clicker.client.flushWrite()`
- Test description / error messages reference clicker's conn instead of
  target's
- Test name unchanged (the *purpose* — verifying the typeid-override
  lookup — is preserved; only the wire dispatch direction flips)

#### Test #6 — `TestFireApTriggerPlayerOverridesTypeIdFromTargetSubjectCom` (NAI-62, AP)

`player_interaction_trigger_test.go:220`. Same drain-conn switch as #5.
Test name unchanged.

### 7.3 Test reframes (comment-only)

#### `TestFireApTriggerPlayerRestoresTargetAndWaypoints` (`:257`)

Comment narrative says "self=target (the target player), no
p_op_player handler exists, and p_op_npc would act on target's state".
Behavior pin (clicker.target restored, clicker.nextTarget==nil,
clicker.waypoints restored) is producer-binding-agnostic — the noop
script doesn't mutate anything. **Update comment only**: replace
"self=target" framing with "self=clicker (TS-true)".

### 7.4 Tests unaffected (no change required)

- `TestApPlayerTriggerForOp` (:18) — pure trigger lookup, no script run.
- `TestFireOpTriggerPlayer_NoScriptRegistered` (:132) — no-script-found
  path, never reaches `runScript`.
- `TestFireApTriggerPlayer_NoScriptSetsApRangeMinusOne` (:149) — same.
- `TestTryFireOpTrigger_PlayerArm` (:169) — only verifies the dispatch
  arm fires (any wire packet on target). Will fail after the binding
  flip (target conn won't receive a packet anymore). **Reframe** to
  drain clicker's conn instead. (Add to T2 sweep — overlooked in
  initial enumeration; verified by re-grep for `targetConn` in
  player_interaction_trigger_test.go.) See §7.5 below.
- `TestFireOpTriggerPlayerCapturesNextTargetFromScript` (NAI-68 B3,
  `interaction_trigger_nai68_test.go:219`) — pins `clicker.target` and
  `clicker.nextTarget` only; producer-binding-agnostic.
- `TestFireOpTriggerPlayerClearsWaypoints` (NAI-68 B5,
  `interaction_trigger_nai68_test.go:245`) — pins `clicker.waypointIndex`
  only; producer-binding-agnostic.
- `pkg/script/handlers_player_test.go` Self2-guard tests — use direct
  `&ScriptState{}` fixtures, producer-binding-independent.

### 7.5 `TestTryFireOpTrigger_PlayerArm` reframe

Re-grep at `:169-184` shows it drains `targetConn` and asserts
`len(got) > 0`. Post-fix, the HINT_ARROW packet lands on clicker's
conn, not target's. **Add to T2 sweep**: switch drain to `clickerConn`
(via fixture upgrade in §7.1), keep the "len > 0" assertion as-is
(test name and intent — verifying the *Player arm fires — preserved).

### 7.6 New test (T3 — same-tick retry behavior pin)

The Test #4 inversion already pins `tryInteract` returning `false`,
but doesn't pin the *follow-on* tick (the retry attempt itself). Add
one new test:

**`TestApTriggerPlayer_SameTickRetry_FullCycle`** — drives a full
`processInteraction` cycle (or two `tryInteract` calls representing
pre-step + post-step). After the pre-step return-false:
- `clicker.interactionFired == false` (reset for retry)
- A second `tryInteract` call (post-step) re-fires AP because guard is
  reset. Assert script ran twice (use a counter script via
  `OpAddVarp` or a side-effect counter on the fixture).
- Final state pins TS Player.ts:1163-1168 round-trip.

This test is the AP-Player twin of NAI-69's
`TestApTriggerLoc_SameTickRetry_RangeLowered` (which exists in
`interaction_trigger_aprange_test.go` per NAI-69 plan §7.1).

## 8. Implementation tasks (TDD bundle)

### T1 — fixture upgrade (preparatory, no behavior change)

Order:
1. Update `newPlayerTriggerFixture` signature to return
   `clickerConn net.Conn` (4th positional return).
2. Sweep all 4 call sites in `player_interaction_trigger_test.go`
   (lines 132, 149, 169, 220, 257, 317, 369; some use `_` already)
   plus 2 in `interaction_trigger_nai68_test.go` (lines 219, 245). Per
   `enumerate_all_sites.md`.
3. Run `go test ./modules/world/... -count=1` — should remain green;
   pure additive change.
4. Commit: `test(world): NAI-70 T1 — newPlayerTriggerFixture clicker conn upgrade`.

### T2 — production swap + test inversions (red→green)

Order:
1. Write all 6 inverted test bodies (#1-#6 in §7.2) plus the #7.5
   `TestTryFireOpTrigger_PlayerArm` reframe and #7.3 comment fix.
   Tests are RED at this point (current code uses reversed binding).
2. Apply the production swap at `player_interaction_trigger.go:76, 132`.
3. Update the 8 doc-comment narratives per §5 (file-header docs +
   `script.go`, `interaction_trigger.go:42`, `script_test.go:1421`).
4. Run `go test ./modules/world/... -count=1`. All inverted tests pass
   GREEN.
5. Run `go test ./... -race -count=1` — full suite green.
6. Commit: `feat(world): NAI-70 T2 — realign AP/OP-Player runScript binding to TS`.

### T3 — same-tick retry full-cycle pin (red→green or green if T2 covered it)

Order:
1. Write `TestApTriggerPlayer_SameTickRetry_FullCycle` per §7.6.
2. Should pass with no further code changes (T2's binding flip
   activates the retry path; NAI-69 T1's `tryInteract` guard already
   in place).
3. Commit: `test(world): NAI-70 T3 — AP-Player same-tick retry full-cycle pin`.

### T4 — deviation-tag retirement

Order:
1. Delete the 5 `NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY`
   narrative references (already overlapped with T2's doc rewrites for
   3 of 5 sites — the remaining 2 are in test headers that T2 inverted).
2. Verify `rg "NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY" modules/ pkg/` returns 0 hits.
3. Spot-check other deviation tags for similar staleness (per
   `retire_deviation_grep_all_comments.md`); minimal touches only.
4. If no separate doc-only changes remain after T2 absorption, fold
   into the T2 commit body. Otherwise commit:
   `docs(world): NAI-70 T4 — retire NAI-69-D-APPLAYER-SELF2-REVERSED tag`.

### T5 — close commit

Per `close_commit_memory_trailer.md`: include `Closes memory:
NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY` trailer.
Net deviation tally: 13 → 12.

## 9. Risk register

- **Hidden Self-side handler regression** — the realignment changes
  every Self-side handler's effect direction simultaneously. Pre-flight
  grep of `s.Self.` in `pkg/script/handlers_player.go` enumerates the
  handlers that read Self for side-effect dispatch. T2's three pinned
  sites (HintPl, ApRange, MES) cover the wire-conn / state-mutation /
  message-dispatch axes. Other handlers (`p_setanim`, `p_apheal`,
  `p_damage` etc.) inherit correctness from the producer fix; not
  pinned individually because no production tests exercise them via
  OPPLAYER/APPLAYER scripts at HEAD.

  **Verification**: post-T2, run `go test ./pkg/script/... -count=1`
  to ensure no handler-level test regresses. Handlers_player_test.go
  uses direct `&ScriptState{}` fixtures (producer-binding-agnostic) so
  this should be a no-op verification.

- **Fixture conn-uniqueness** — both players need distinct ISAAC seeds
  so HINT_ARROW byte expectations don't accidentally match either side.
  Test #1 and #2 use seed `{5, 6, 7, 8}` for clicker vs `{1, 2, 3, 4}`
  for target. The first encrypted opcode byte differs between seeds,
  preventing accidental cross-conn match.

- **NAI-39 closure pin reframing** — `TestFireOpTriggerPlayer_BindsSelf2ToClicker`
  (the original NAI-39 closure pin) becomes `BindsSelf2ToTarget`. The
  closure of `NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER` stands —
  the producer exists; this sub-spec corrects the *direction* of the
  binding without introducing or removing any producer site.

- **`buildPlayerScriptState` doc comment** at `script.go:30-34` says
  "case script.ActivePlayer is the secondary-binding arm consumed by
  the OPPLAYER<N>/APPLAYER<N> player→player trigger family". The arm
  itself sets Self2=target (its second arg) — TS-correct as written.
  The fix is at the *callers*, not the producer. T2 reword preserves
  the arm's TS-true behavior description.

- **Test inversion fail-on-correct-reason** — Test #4's inversion
  predicts `tryInteract` returns `false`, but the same false return
  could come from an unrelated path. Verify the failure mode is the
  NAI-69 T1 guard firing by ALSO asserting `clicker.apRangeCalled ==
  true` and `clicker.interactionFired == false`. (Triple-pin per
  `test_passes_for_wrong_reason.md`.)

## 10. Acceptance criteria

1. `rg "NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY" modules/ pkg/`
   returns 0 hits at HEAD.
2. `rg "srv\.runScript\(sf, target, p" modules/world/` returns 0 hits.
3. `go test ./... -count=1 -race` passes.
4. The 6 inverted tests + 1 new test all pass; the 1 reframed test
   (`TestTryFireOpTrigger_PlayerArm`) passes after drain-conn switch.
5. No existing test regresses.
6. Doc-comments at `player_interaction_trigger.go` (file-header + in-
   body), `interaction_trigger.go:42`, `script.go:30-34`, and
   `script_test.go:1421` all describe Self=clicker, Self2=target with
   TS-line citations.
7. Net deviation tally in close commit body: **13 → 12**.

## 11. Memory entries reinforced

- `runescript_cadence.md` — full sub-spec → plan → TDD bundle → close.
- `true_to_ts_gate.md` — every behavioral change cited against TS
  Player.ts:1129 + 1151 + ScriptRunner.ts:84-87.
- `controller_preflight.md` — pre-flight grep targets in §6 verified
  against HEAD `54930db`.
- `enumerate_all_sites.md` — all `srv.runScript` callers + all 6+
  fixture call sites enumerated; no implementer-side discovery left.
- `risk_register_premise_grep.md` — NAI-69's missed AP-Player premise
  was the surfacing instance; this spec re-derives every Self/Self2
  claim from grep+Read of `state.Self =` sites.
- `retire_deviation_grep_all_comments.md` — T4 explicitly grep-verifies
  zero residual hits.
- `consume_reserved_constant.md` — NAI-39's reserved producer for
  Self2 + PtrActivePlayer2 is now consumed in TS-correct shape; the
  audit pattern caught the call-site direction divergence one sub-spec
  late.
- `test_passes_for_wrong_reason.md` — Test #4 triple-pin (return value
  + apRangeCalled + interactionFired) prevents accidental green from a
  different code path.
- `close_commit_memory_trailer.md` — close commit carries `Closes
  memory:` trailer.
- `plan_test_coverage_crosscheck.md` — every code change in §5 has a
  matching test in §7.
