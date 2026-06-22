# NAI-24 follow-up bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Audit `pkg/script/handlers_player.go` for TS-faithful `NumberNotNull` gates, and silently fix the INV_TRANSMIT source-uid porting bug surfaced by NAI-23 Bundle 4b.

**Architecture:** Two disjoint single-file bundles in `pkg/script`. Bundle 1 applies the NAI-23 Bundle 4 audit cadence (rubric → audit table → wraps → null-pin tests) on `handlers_player.go`. Bundle 2 is a 1-line production fix at `handlers_inv.go:429` plus a doc-comment narration update plus an existing-test assertion flip from `Source: -1` to `Source: <self.uid>`. No inter-bundle dependencies; both committed independently.

**Tech Stack:** Go 1.26+. Existing `checkNotNull` helper at `pkg/script/handlers_player.go:61`. Existing `mockPlayer` test fixture at `pkg/script/runner_test.go:95` (with pre-existing `uidValue int` field at line 189 and `UID() int` method at line 428). TS source root at `$HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/`.

**Spec reference:** `docs/superpowers/specs/2026-04-25-nai-24-followup-bundle-design.md`.

---

## Task 1 — Bundle 1: `handlers_player.go` NumberNotNull audit

**Files:**
- Modify: `pkg/script/handlers_player.go` (per-handler `checkNotNull` wraps; count determined by audit)
- Test: `pkg/script/handlers_player_test.go` (per-handler null-pin tests; one per newly added WRAP)

**Pre-flight context:**
- File enters NAI-24 with **3** pre-existing `checkNotNull` wraps at lines 104, 121, 700. The implementer reads them as templates: `handleAnimProtect` (line 104, op `"P_ANIMPROTECT"`), `handleAllowDesign` (line 121, op `"ALLOWDESIGN"`), `handleMidiJingle` (line 700, op `"MIDI_JINGLE"`). Verified against HEAD `0031ce3` via `grep -n "checkNotNull" pkg/script/handlers_player.go` — the only 5 occurrences are line 58 (doc-comment), line 61 (helper definition), and the 3 wrap call sites at 104/121/700.
- 49 total `s.PopInt()` sites in the file. The audit covers all 49; the 3 pre-existing wraps appear in the audit table as `WRAP (pre-existing)` rows confirming they're TS-faithful.
- Test fixture pattern: existing tests in this file use `mp := &mockPlayer{}` + `sf := &ScriptFile{...}` + `state := Init(sf, mp, false, nil, nil)` + `Execute(state)` (no helper builder; tests inline the ScriptFile construction). `newSingleOp` helper exists at line 49 for trivial single-opcode scripts but most existing tests inline the ScriptFile.
- Null-pin test naming convention (verified across `handlers_inv_test.go` and `handlers_interface_test.go`): `TestHandle<OpName>NullRejected` — the project standard. Use this exact form, not `RejectsNullSentinel` (which the spec drafted but does not match codebase precedent per `plan_grep_helper_patterns` memory).
- Op-name string convention (verified in pre-existing wraps): underscored uppercase, e.g., `"P_ANIMPROTECT"`, `"ALLOWDESIGN"`, `"MIDI_JINGLE"`. The implementer's WRAP additions follow this case-shape — match the corresponding RuneScript opcode mnemonic for each handler.

**Per-pop-site decision rubric** (verbatim from spec § Bundle 1):

1. **TS wraps with `check(state.popInt(), NumberNotNull)`** → **WRAP**. Add `if err := checkNotNull(v, "OP_NAME"); err != nil { return err }` immediately after the `s.PopInt()`.
2. **TS wraps with a typed validator** (StatValid, SeqTypeValid, VarPlayerValid, CategoryTypeValid, EnumTypeValid, LocAngleValid, SpotanimTypeValid, etc.) → **SKIP**. Audit table records `<ValidatorName>` rationale.
3. **TS does not wrap the popped value at all** → **SKIP**. Audit table records `TS does not wrap; preserve tolerance`.
4. **Popped value is semantically signed** (coord delta, search-relative offset, arithmetic operand, queue-arg slot count, varbit-cleared sentinel) → **SKIP**. Audit table records `signed value; -1 sentinel does not apply`.
5. **Ambiguous** → **ESCALATE** to the controller before deciding. Report in summary; do NOT guess.

- [ ] **Step 1: Enumerate popInt sites in `handlers_player.go` and read 5 existing wraps for template**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go version
grep -n "s\.PopInt()" $HOME/Code/github.com/zsrv/goscape/pkg/script/handlers_player.go
grep -nB1 -A4 "checkNotNull" $HOME/Code/github.com/zsrv/goscape/pkg/script/handlers_player.go
```

Record: every `s.PopInt()` line number with its enclosing handler name, and confirm the 5 pre-existing wraps' op-name conventions.

- [ ] **Step 2: For each popInt site, apply the audit rubric against `PlayerOps.ts`**

For every popInt site identified in Step 1:
- Identify the enclosing handler (e.g., `handleSomeOp`).
- Read the matching TS counterpart in `$HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts`. Search by opcode mnemonic (the `case ScriptOpcode.OP_NAME:` lines).
- Apply rubric → record decision (WRAP / SKIP / ESCALATE) + rationale (TS file:line) in the audit table.

Build the per-handler audit table per the spec format:

| Handler | popInt context | TS wraps? | Decision | Rationale (TS file:line) |
|---------|---------------|-----------|----------|-------------------------|
| handleSomeOp | com | NumberNotNull | WRAP | PlayerOps.ts:NNN |
| handleOther | x | not wrapped | SKIP | TS does not wrap (PlayerOps.ts:NNN-NNN) |
| ... | ... | ... | ... | ... |

ESCALATE any unclear case to the controller. Do NOT guess; the rubric's escape hatch exists for exactly this.

- [ ] **Step 3: Write failing null-pin tests for every WRAP candidate**

Append tests to `pkg/script/handlers_player_test.go`. Test shape (template — substitute `<OpName>`, `<OP_NAME>`, `<OpEnumValue>`, and adjust `IntOperands` for the handler's pop order):

```go
// TestHandle<OpName>NullRejected pins <OPCODE_MNEMONIC>: TS wraps <field>
// with NumberNotNull (PlayerOps.ts:NNN).
func TestHandle<OpName>NullRejected(t *testing.T) {
    mp := &mockPlayer{}
    sf := &ScriptFile{
        Name: "<lowercase_opname>_null_<field>",
        Opcodes: []Opcode{
            OpPushConstantInt, // <field> = -1
            Op<OpEnumValue>,
            OpReturn,
        },
        IntOperands: []int32{-1, 0, 0},
    }
    state := Init(sf, mp, false, nil, nil)

    err := Execute(state)
    if err == nil {
        t.Fatalf("Execute: want error for <field>=-1, got nil")
    }
    want := "<OP_NAME>: input number was null(-1)"
    if !strings.Contains(err.Error(), want) {
        t.Errorf("error: got %q, want substring %q", err.Error(), want)
    }
}
```

For multi-pop handlers where multiple ints are wrapped, write one test per wrapped int (table-driven if ≥3 in the same handler). Pin only one int's null at a time; other wrapped ints stay valid (e.g., `0`) so the rejection is attributable to the specific wrap. The push order must match the handler's pop order — the **last `OpPushConstantInt` is on top of the stack and is popped first**. Verify against the handler's own pop sequence at audit time.

For handlers where a side-effect mock-recording field exists on `mockPlayer` (e.g., `lastOpenMain`, `lastInvListenOnCom`), append an "OpenMain should not have been called" assertion mirroring the `handlers_interface_test.go:567-569` pattern. For handlers without such a recording field, omit the side-effect-non-call assertion (the error-return alone is the pin).

- [ ] **Step 4: Run the new null-pin tests to verify FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "NullRejected" ./pkg/script/
```

Expected: **FAIL** — every newly added `TestHandle<OpName>NullRejected` test fails because the production handler does not yet wrap the popped value with `checkNotNull`. The pre-existing 5 `*NullRejected` tests (if any from prior NAI work) and Bundle 4a/4b/4c tests continue to pass.

If a new test passes here, that means the handler is already wrapping (pre-existing WRAP). Re-check Step 2's audit table — that handler should be marked `WRAP (pre-existing)`, not a net new wrap, and its test should be omitted (test already exists or wrap already exists with no test gap to fill).

- [ ] **Step 5: Add `checkNotNull` wraps to every WRAP candidate in `handlers_player.go`**

For each WRAP row in the audit table, insert immediately after the `s.PopInt()` line that produces the value. Wrap shape (verbatim from existing wrap at line 103-106):

```go
v := s.PopInt()
if err := checkNotNull(v, "OP_NAME"); err != nil {
    return err
}
```

Where `OP_NAME` is the underscored-uppercase opcode mnemonic (e.g., `"P_TELEJUMP"`, `"P_OPENCHATBACKDELAY"`, `"P_LOGOUT"` — the implementer chooses each name to match the existing RuneScript opcode it implements).

For multi-pop handlers, each WRAP popInt gets its own wrap immediately after that popInt — **before** the next popInt for the next argument. Pop order is preserved as-is; the wrap insertion does not reorder pops.

Doc-comments updated only where a SKIP rationale is non-obvious for a signed-value site (sparing — add a 1-line `// signed: <reason>` comment only when the SKIP decision would surprise a future reader, e.g., a coord-delta or search-radius site).

- [ ] **Step 6: Run all `pkg/script/` tests to verify PASS + no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/
```

Expected: **PASS**. Every newly added null-pin test passes; every pre-existing test (Stat*, Coord, PTeleJump, BAS*, RunAnim, Stat*, INV_*, IF_*, etc.) continues to pass.

If any pre-existing test fails: investigate — a wrap was inserted in the wrong logical position relative to a side-effect, or an op-name string collides with another handler's existing test fixture. Do NOT silence the failure; restore TS-faithful behavior or ESCALATE to the controller.

- [ ] **Step 7: Run modules/world tests for cross-package regression check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: **PASS** across the whole repo. Per `verify_implementer_claims` memory: package-scoped green can mask cross-package breakage; the full-repo run is the cross-check.

- [ ] **Step 8: Commit Bundle 1**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-24 Bundle 1 — handlers_player.go NumberNotNull audit

Audit pass per NAI-24 spec § Bundle 1: every s.PopInt() in
handlers_player.go is checked against its TS counterpart in
PlayerOps.ts. Sites where TS wraps with check(state.popInt(),
NumberNotNull) gain a goscape checkNotNull wrap; signed-value sites
and TS-unwrapped sites stay raw with recorded rationale.

File entered audit with 3 pre-existing wraps (handleAnimProtect at
line 104, handleAllowDesign at line 121, handleMidiJingle at line
700). N net new wraps across M handlers; K sites SKIPped (rationale
per audit table). N new TestHandle<OpName>NullRejected tests follow
the handlers_interface_test.go shape.

Per-handler audit table:
[paste the audit table from Step 2]

Skip-reason breakdown:
- Typed-validator (StatValid / SeqTypeValid / VarPlayerValid /
  CategoryTypeValid / etc.): K1
- Signed sentinel (coord delta / search-relative offset / arithmetic):
  K2
- TS does not wrap: K3

Closes the From-NAI-23 tracker entry's handlers_player.go priority
row at nai_followups.md:1394-1397. No deviation tags retired or
introduced. Net deviation count unchanged (14).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — Bundle 2: INV_TRANSMIT source-uid remediation

**Files:**
- Modify: `pkg/script/handlers_inv.go:412-431` (1-line production fix at line 429 + doc-comment narration update at lines 412-419)
- Test: `pkg/script/handlers_inv_test.go:383-412` (`TestInvTransmitRegistersListener` assertion flip)

**Pre-flight context:**
- HEAD `1cc0e0f` (after spec commit). The 1-line fix is at `handlers_inv.go:429`. Verify line numbers re-grep at task time per `controller_preflight` memory.
- `mockPlayer` already implements `UID() int` returning `m.uidValue` (`pkg/script/runner_test.go:428`); pre-existing `uidValue int` field at line 189. No fixture additions needed — just set `mp.uidValue` at construction.
- Existing test `TestInvTransmitRegistersListener` at `handlers_inv_test.go:386-412` constructs `mp := &mockPlayer{}` (line 387) — `uidValue` defaults to zero. Bundle 2 changes that to `mp := &mockPlayer{uidValue: 42}` (any deterministic non-zero value is fine; `42` chosen as a clear test-fixture sentinel).
- The existing test `TestInvTransmitNoActivePlayerErrors` at `handlers_inv_test.go:416` is **unaffected** — `requireActivePlayer` fires and returns before `s.Self.UID()` is called. Confirm at task time by re-reading the early-return order in `handleInvTransmit`.
- Cross-package pin search: pre-flight grep at spec-write time confirmed the only test in any package that pins INV_TRANSMIT's `Source: -1` is the one being flipped. Re-run the cross-package grep at task dispatch (per `enumerate_all_sites` memory).

- [ ] **Step 1: Re-verify line numbers and cross-package pin status against HEAD**

```bash
grep -n "InvListenOnCom(invType, com, " $HOME/Code/github.com/zsrv/goscape/pkg/script/handlers_inv.go
grep -rn "Source.*-1\|lastInvListenOnCom" $HOME/Code/github.com/zsrv/goscape/pkg $HOME/Code/github.com/zsrv/goscape/modules
```

Expected: the production call at line 429 still reads `s.Self.InvListenOnCom(invType, com, -1)`; the only test pinning INV_TRANSMIT-specific `Source: -1` is `pkg/script/handlers_inv_test.go:409-411`.

If the pre-flight uncovers a new pin in another file, ESCALATE — the spec did not anticipate it and the bundle scope must reflect the new touchpoint.

- [ ] **Step 2: Flip the existing test assertion (write the failing test)**

Edit `pkg/script/handlers_inv_test.go` `TestInvTransmitRegistersListener` (lines 383-412):

1. Update the doc-comment at line 385 from `InvListenOnCom(invType, com, -1)` to `InvListenOnCom(invType, com, activePlayer.uid)`.
2. Change line 387 from `mp := &mockPlayer{}` to `mp := &mockPlayer{uidValue: 42}`.
3. Change the assertion at line 409 from `got.InvType != 93 || got.Com != 149 || got.Source != -1` to `got.InvType != 93 || got.Com != 149 || got.Source != 42`.
4. Change the error-format string at line 410 from `"want {InvType:93, Com:149, Source:-1}"` to `"want {InvType:93, Com:149, Source:42}"`.

After Step 2, the test reads:

```go
// TestInvTransmitRegistersListener runs a script pushing (com, inv) then
// OpInvTransmit; asserts the mock player recorded
// InvListenOnCom(invType, com, activePlayer.uid). Matches TS InvOps.ts INV_TRANSMIT.
func TestInvTransmitRegistersListener(t *testing.T) {
    mp := &mockPlayer{uidValue: 42}

    sf := &ScriptFile{
        Name: "inv_transmit",
        Opcodes: []Opcode{
            OpPushConstantInt, // com
            OpPushConstantInt, // inv (top)
            OpInvTransmit,
            OpReturn,
        },
        IntOperands:      []int32{149, 93, 0, 0},
        StringOperands:   []string{"", "", "", ""},
        InstructionCount: 4,
    }
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if len(mp.lastInvListenOnCom) != 1 {
        t.Fatalf("expected 1 call to InvListenOnCom, got %d", len(mp.lastInvListenOnCom))
    }
    got := mp.lastInvListenOnCom[0]
    if got.InvType != 93 || got.Com != 149 || got.Source != 42 {
        t.Errorf("InvListenOnCom args: got %+v, want {InvType:93, Com:149, Source:42}", got)
    }
}
```

- [ ] **Step 3: Run the flipped test to verify FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvTransmitRegistersListener" ./pkg/script/ -v
```

Expected: **FAIL** with assertion `got.Source != 42` because the production handler still passes `-1`. Specifically the failure message should resemble `InvListenOnCom args: got {InvType:93 Com:149 Source:-1}, want {InvType:93, Com:149, Source:42}`.

- [ ] **Step 4: Apply the 1-line production fix at `handlers_inv.go:429`**

Change line 429 from:
```go
s.Self.InvListenOnCom(invType, com, -1)
```
to:
```go
s.Self.InvListenOnCom(invType, com, s.Self.UID())
```

- [ ] **Step 5: Update the doc-comment narration at `handlers_inv.go:412-419`**

Replace the existing doc-comment (lines 412-419) with:

```go
// handleInvTransmit implements INV_TRANSMIT. Registers a listener on
// the active player for UI component `com` tracking the active
// player's own inventory of type `invType` (source = activePlayer.UID()).
//
// TS: InvOps.ts INV_TRANSMIT — popInt(inv), popInt(com),
// activePlayer.invListenOnCom(inv, com, activePlayer.uid). com is
// wrapped with check(com, NumberNotNull) in TS; invType uses
// InvTypeValid (not NumberNotNull) — stays raw (NAI-23 Bundle 4b).
// Source porting fix landed in NAI-24 Bundle 2 — origin commit
// 5b67653 (S6u) erroneously hard-coded -1.
```

- [ ] **Step 6: Run the flipped test to verify PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvTransmitRegistersListener" ./pkg/script/ -v
```

Expected: **PASS**.

- [ ] **Step 7: Run all pkg/script/ tests + modules/world tests for regression check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: **PASS** across the whole repo. Verify in particular:
- `TestInvTransmitNoActivePlayerErrors` (line 416) still PASS — the early `requireActivePlayer` return fires before `s.Self.UID()` is called.
- `TestInvOtherTransmitRegistersListener` and the INVOTHER_TRANSMIT null-pin tests (lines 469-630) still PASS — that handler already passes the popped uid; Bundle 2 does not touch it.
- `modules/world/player_inv_test.go` tests (TestInvListenOnComRegistersNewListener / Replace / LazyInit) still PASS — those exercise the internal `invListenOnCom` API directly with literal `-1`; Bundle 2 does not touch the API contract.

- [ ] **Step 8: Commit Bundle 2**

```bash
git add pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-24 Bundle 2 — INV_TRANSMIT source uid remediation

Silent porting-bug fix per NAI-24 spec § Bundle 2. handleInvTransmit
at handlers_inv.go:429 was passing source=-1 (origin commit 5b67653
/ S6u); TS InvOps.ts:650 passes state.activePlayer.uid. Equivalence
determination against (*Player).invListenOnCom dispatch
(modules/world/player.go:471-479, :632-633) confirms not equivalent:
-1 reads from Server.invs[Type] (world-shared); p.uid reads from
Server.players[uid].invs[Type] (the player's own slot). For a
typical backpack listen, those resolve to different inventory
objects — INV_TRANSMIT in goscape was reading from the wrong store.

Production change: 1 line. Pass s.Self.UID() instead of hard-coded
-1. Doc-comment narration at lines 412-419 updated to match TS
faithfully (TS narrates source = activePlayer.uid, not -1) and
notes the porting-fix attribution.

Test change: TestInvTransmitRegistersListener at handlers_inv_test.go
:386-412 had its mockPlayer constructed with uidValue: 42 and its
Source assertion flipped from -1 to 42. The internal-API tests at
modules/world/player_inv_test.go are untouched (those exercise
(*Player).invListenOnCom with literal -1 to test the lazy-init /
replace / nil-map paths — testing the listener-API contract, not
the INV_TRANSMIT opcode). TestInvTransmitNoActivePlayerErrors
unchanged — requireActivePlayer fires before UID().

No deviation tag opened (silent fix per Approach 1). Closes the
From-NAI-23 tracker entry "INV_TRANSMIT source-uid divergence" at
nai_followups.md:1356-1386. The now-dead -1 API surface in
(*Player).invListenOnCom is documented in a new From-NAI-24
tracker entry at the close commit.

Net deviation count unchanged (14).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Two-stage review checkpoint (post-Bundle-2)

After both bundles land, dispatch two-stage review per `runescript_cadence` memory:

- **Stage 1 (spec compliance)** — fresh opus subagent compares each bundle's commit against the spec § Bundle N section. Bundle 1: audit table cross-checked against `PlayerOps.ts` decisions (every WRAP row's TS file:line opens to a `check(..., NumberNotNull)`); every newly added test corresponds to a WRAP row. Bundle 2: 1-line fix matches spec; doc-comment matches; assertion flip preserves test intent.
- **Stage 2 (code quality)** — fresh opus subagent reviews for naming consistency, idiomatic Go, test-helper reuse, missed cross-package pins, doc-comment narrative consistency, dead-API leftovers.

Each stage is a single subagent dispatch. Polish commits land **before** the close commit if review surfaces remediable findings (per NAI-23 precedent: `polish(script): NAI-24 close polish` style).

---

## Close commit

Once both bundles + reviews + any polish commits have landed, append the close commit:

1. **Update `nai_followups.md`**:
   - Mark the From-NAI-23 entry at `:1356-1386` Resolved with the Bundle 2 commit hash (preserve original body under the existing separator).
   - Update the "Future NumberNotNull sweep targets" enumeration at `:1388-1418` — mark the `handlers_player.go` row Resolved with the Bundle 1 commit hash; re-order the remaining priorities if any per-file density estimates shifted.
   - Append a new `## From NAI-24 (2026-04-25)` section with the now-dead `-1` API-surface deferral entry per spec § "Out-of-scope" #1.

2. **Stage and commit** (memory file is outside the working tree; no git stage):

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(script): NAI-24 closed — two-bundle follow-up

Closes the From-NAI-23 handlers_player.go NumberNotNull audit row
and the INV_TRANSMIT source-uid divergence.

Bundle 1 (feat): handlers_player.go NumberNotNull audit. N net new
checkNotNull wraps + N null-pin tests across M handlers; K sites
SKIPped per audit table.
Bundle 2 (feat): INV_TRANSMIT source-uid remediation. 1-line silent
porting-bug fix at handlers_inv.go:429; doc-comment narration
updated; existing TestInvTransmitRegistersListener assertion flipped
from Source:-1 to Source:42.

Net deviation count: 14 → 14.

Closes memory: nai_followups.md:1394-1397 (handlers_player.go priority row, Resolved by NAI-24 Bundle 1)
Closes memory: nai_followups.md:1356-1386 (INV_TRANSMIT source-uid divergence, Resolved by NAI-24 Bundle 2)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Plan self-review

**Spec coverage:** Every spec section maps to a task or step:
- Spec § Bundle 1 (audit cadence + rubric + audit table + tests) → Task 1 Steps 1-8.
- Spec § Bundle 2 (1-line fix + doc-comment + assertion flip + tracker resolution) → Task 2 Steps 1-8.
- Spec § "Out-of-scope" #1 (the now-dead `-1` API surface) → Close commit § "Update nai_followups.md" #3.
- Spec § "Out-of-scope" #2-3 → no plan tasks (correctly deferred per spec).
- Spec § "Risks & mitigations" → Bundle 1 ESCALATE rubric (rule 5); Task 2 Step 1 cross-package pin re-grep; Task 1 Step 7 + Task 2 Step 7 full-repo regression checks.
- Spec § "Review structure" → Two-stage review checkpoint section.
- Spec § "NAI-24 close" → Close commit section.

**Placeholder scan:** No forbidden patterns ("TBD", "TODO", "implement later", "fill in details", "appropriate error handling"). Bundle 1 audit table uses `N` / `M` / `K` / `K1` / `K2` / `K3` / `[paste the audit table from Step 2]` placeholders intentionally because the audit IS the work — the implementer fills them with discovered counts at task time (matches NAI-23 Bundle 4 plan precedent). The op-name string templates (`<OpName>`, `<OP_NAME>`, `<OpEnumValue>`) are template variables for the per-WRAP test generation — the implementer substitutes per row.

**Type consistency:** Across both tasks, the production call signature `InvListenOnCom(invType, com, source int)` and its consumers (`s.Self.InvListenOnCom(...)`, `s.Self.UID()`) are referenced consistently. The `mockPlayer.uidValue` field name is consistent across spec and plan. Test naming convention `TestHandle<OpName>NullRejected` is used consistently in Task 1 and matches the codebase precedent at `handlers_interface_test.go` and `handlers_inv_test.go`. The `checkNotNull` helper signature `(v int, op string) error` matches its definition at `pkg/script/handlers_player.go:61`.

**Plan-test-coverage crosscheck** (per `plan_test_coverage_crosscheck` memory):
- Task 1: spec mandates "1 negative-pin test per WRAP" → plan codifies this in Step 3 with the test template + multi-pop sub-case rule + side-effect-non-call optional assertion. Test count is bounded by the audit-table WRAP count; the audit table itself is the per-task expected-test-count that the reviewer cross-checks.
- Task 2: spec mandates "no new tests, flip existing assertion" → plan codifies the exact 4-line edit to `TestInvTransmitRegistersListener` and explicitly preserves `TestInvTransmitNoActivePlayerErrors` plus all internal-API tests in `modules/world/player_inv_test.go`.

**Plan-runnable-test-fixture crosscheck** (per `plan_runnable_test_fixtures` memory):
- Task 1 Step 3 template uses the verified existing pattern (`mp := &mockPlayer{}` + inline `ScriptFile` + `Init` + `Execute`). The push-order rule (last `OpPushConstantInt` is on stack top, popped first) is explicit. The `IntOperands` example `[]int32{-1, 0, 0}` has 3 slots: `-1` (the value pushed via PushConstantInt), then `0, 0` for operand padding (matching `handlers_interface_test.go:555` precedent).
- Task 2 Step 2 codifies the test post-flip in full as a runnable Go block — every line exists in HEAD or is a deterministic edit from a HEAD line.

**Helper-pattern crosscheck** (per `plan_grep_helper_patterns` memory): `checkNotNull` (verified at `handlers_player.go:61`) is the existing helper; plan does not prescribe inline boilerplate. `requireActivePlayer` is referenced indirectly (via the early-return analysis for `TestInvTransmitNoActivePlayerErrors`) and is the existing helper at `handlers_inv.go:421`.

No issues found.
