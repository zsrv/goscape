# NAI-123 — ai_queue `last_int` plumbing fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the goscape divergence where `processNpcQueue` never copies the queued arg into `state.LastInt`, so `[ai_queueN,_] ~proc(last_int);` content scripts read 0 and apply 0 damage. Mirrors TS `Npc.ts:554-555`.

**Architecture:** Two-task bundle. Task 1 (TDD) lands a failing observable test then the minimal wiring fix in `processNpcQueue` — script.LastInt populated from the queued arg before `resumeOrFinishNpc`. Task 2 is the mechanical TS-fidelity rename of `NpcQueueRequest.IntArg` → `LastInt` (and ripple through interface, impls, mocks, test struct literals). Task 3 is a Sonnet code-reviewer pass.

**Tech Stack:** Go 1.26+. `pkg/script` (engine), `modules/world` (server). All `go` invocations prefixed `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` per global CLAUDE.md.

**Spec:** `docs/superpowers/specs/2026-05-07-nai-123-zero-damage-investigation-design.md` (commit `8feee80`).

---

## Cadence and review constraints

- **Subagent model:** Sonnet implementer per task; Sonnet code-reviewer per `superpowers_code_reviewer_model` (NEVER Opus for reviewers).
- **Controller pre-flight per task:** Re-Read each "Modify" file + re-grep each codified identifier at HEAD before subagent dispatch. Catch stale plan-authored claims early per `controller_preflight`.
- **Controller post-commit verification:** `git show <SHA> --stat` to confirm scope; `git status` to catch worktree-stray writes per `feedback_subagent_wt_path`; fresh `go test ./... && go vet ./... && go build ./...` per `verify_implementer_claims`.
- **Bundle close-gate:** Task 3 reviewer pass over both implementer commits.

---

## File responsibility map

| File | Responsibility |
|---|---|
| `pkg/script/queue.go` | `NpcQueueRequest` struct definition (engine-side; consumed by both `pkg/script` and `modules/world`). |
| `pkg/script/active.go` | `ActiveNpc` interface defining `EnqueueScriptForTrigger`. |
| `pkg/script/handlers_npc.go` | `handleNpcQueue` opcode handler (script-runtime caller of `EnqueueScriptForTrigger`). |
| `pkg/script/handlers_npc_test.go` | `mockNpc` + `mockEnqueueCall` test fixtures + `TestHandleNpcQueueEnqueues`. |
| `modules/world/npc.go` | Concrete `*Npc.EnqueueScriptForTrigger` impl. |
| `modules/world/npc_script.go` | `processNpcQueue` — fires queued ai_queueN scripts on the NPC at delay-zero (the bug site). |
| `modules/world/npc_test.go` | Lifecycle-revert tests holding `NpcQueueRequest` literals. |
| `modules/world/npc_event_queue_test.go` | Event-queue dispatch test holding a `NpcQueueRequest` literal. |
| `modules/world/npc_script_test.go` | `TestNpcEnqueueScriptForTrigger` direct-field assertion + new `TestProcessNpcQueue_SetsStateLastInt`. |

---

## Task 1: TDD wiring fix in `processNpcQueue`

**Subagent:** Sonnet implementer.

**Files:**
- Modify: `modules/world/npc_script.go:469-493` (the `processNpcQueue` function).
- Test (new): `modules/world/npc_script_test.go` — append `TestProcessNpcQueue_SetsStateLastInt`.

**Pre-flight (controller, before dispatch):**
- Re-Read `modules/world/npc_script.go:469-493` at HEAD; confirm the lines `intArg := req.IntArg` and `s.runNpcScript(sf, n, nil, []int{intArg}, nil)` are still present.
- Re-Read `modules/world/npc_script.go:283-321` for `buildNpcScriptState` signature; confirm 5-arg shape `(sf, npc, target, intArgs, stringArgs)` and that `script.Init` is called inside.
- Re-Read `modules/world/npc.go:343-347` for `Npc.SetTimer` — confirm `n.timerInterval = interval`.
- Re-Read `pkg/script/handlers_npc.go:360-369` for `handleNpcSetTimer` — confirm `checkNotNull(interval, "NPC_SETTIMER")` rejects -1 only (so `last_int=42` passes).
- Re-grep `OpLastInt` at `pkg/script/opcode.go` and `OpNpcSetTimer` to confirm the opcode-constant names used in the test bytecode are accurate.

- [ ] **Step 1.1: Write the failing observable test**

Append to `modules/world/npc_script_test.go` (after `TestNpcEnqueueScriptForTrigger`):

```go
// TestProcessNpcQueue_SetsStateLastInt pins NAI-123 fix: processNpcQueue
// must copy req.IntArg into state.LastInt before executing the dispatched
// ai_queueN script. Mirrors TS Npc.ts:554-555 — without this line,
// [ai_queueN,_] ~proc(last_int) reads 0 and zero-damages the target.
//
// Observable: register a script at TriggerAiQueue2 whose bytecode pushes
// last_int and feeds it to NPC_SETTIMER. SetTimer writes n.timerInterval
// directly. Enqueue with intArg=42; turn(); assert n.timerInterval == 42.
//
// Failure mode if state.LastInt is unset: OpLastInt pushes 0 → SetTimer(0)
// → n.timerInterval = 0 (got 0, want 42).
func TestProcessNpcQueue_SetsStateLastInt(t *testing.T) {
	s, n := buildNpcForIntegration(t)
	s.scriptProvider = script.NewProvider()

	// Bytecode: OpLastInt; OpNpcSetTimer; OpReturn.
	// OpLastInt pushes state.LastInt; OpNpcSetTimer pops it as the
	// interval and calls n.SetTimer(interval) → n.timerInterval=interval.
	probe := &script.ScriptFile{
		Name:             "nai123_lastint_probe_aiqueue2",
		LookupKey:        uint32(script.TriggerAiQueue2),
		Opcodes:          []script.Opcode{script.OpLastInt, script.OpNpcSetTimer, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(probe)

	if got := s.scriptProvider.GetByTrigger(script.TriggerAiQueue2, n.typeId, n.typ.Category); got != probe {
		t.Fatalf("setup: GetByTrigger(TriggerAiQueue2, ...) = %v, want probe", got)
	}

	n.EnqueueScriptForTrigger(script.TriggerAiQueue2, 1, 42)
	if len(n.queue) != 1 {
		t.Fatalf("setup: queue len = %d, want 1", len(n.queue))
	}

	n.turn(s)

	if len(n.queue) != 0 {
		t.Fatalf("after turn: queue len = %d, want 0 (delay 1→0 fires the queue)", len(n.queue))
	}
	if n.timerInterval != 42 {
		t.Errorf("n.timerInterval: got %d, want 42 (state.LastInt was not propagated to dispatched script — TS Npc.ts:554-555)", n.timerInterval)
	}
}
```

- [ ] **Step 1.2: Run the new test to verify it fails (RED)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessNpcQueue_SetsStateLastInt -v
```

Expected: FAIL with `n.timerInterval: got 0, want 42 (state.LastInt was not propagated...)`.

If it errors with a compile failure (e.g., `OpLastInt undefined`), stop and re-grep `pkg/script/opcode.go` for the opcode constants — do not invent.

- [ ] **Step 1.3: Apply the wiring fix to `processNpcQueue`**

Replace the body of `processNpcQueue` at `modules/world/npc_script.go:469-493` with:

```go
// processNpcQueue walks the NPC's queue, decrementing delays and
// firing ready entries as fresh NPC-anchored script runs. Iterates
// by index so a request appended mid-pass (via a fired script calling
// EnqueueScriptForTrigger again) is visible in the same iteration —
// preserves TS's "speedup quirk" at Npc.ts:538-560.
//
// Delay only decrements when the NPC is not delayed (TS Npc.ts:544-547
// "purposely only decrements the delay when the npc is not delayed").
// Removal happens BEFORE firing so a re-entrant enqueue doesn't
// collide with the index pointer. Matches the player-side pattern at
// modules/world/tick.go:219-242.
//
// NAI-123: req.IntArg is copied into state.LastInt before execution,
// mirroring TS Npc.ts:554-555 (state.lastInt = request.lastInt). The
// queued arg is NOT a positional script arg (TS request.args is always
// [] at the only enqueue site Npc.ts:242); ai_queueN scripts read it
// via the last_int opcode → state.LastInt.
func (s *Server) processNpcQueue(n *Npc) {
	if n.typ == nil {
		return
	}
	i := 0
	for i < len(n.queue) {
		req := &n.queue[i]
		if !n.delayed {
			req.Delay--
		}
		if n.delayed || req.Delay > 0 {
			i++
			continue
		}
		trigger := req.Trigger
		lastIntArg := req.IntArg
		n.queue = append(n.queue[:i], n.queue[i+1:]...)
		if s.scriptProvider == nil {
			continue
		}
		sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
		if sf == nil {
			continue
		}
		state := s.buildNpcScriptState(sf, n, nil, nil, nil)
		state.LastInt = lastIntArg
		s.resumeOrFinishNpc(state, n)
		// Don't advance i — removed current element.
	}
}
```

Three substantive changes vs HEAD:
1. `intArg := req.IntArg` → `lastIntArg := req.IntArg` (rename for clarity; the field rename in Task 2 will make this `req.LastInt`).
2. Replaces `s.runNpcScript(sf, n, nil, []int{intArg}, nil)` with a 3-line build-set-resume sequence so we have a state handle to write `state.LastInt` to. Mirrors TS `Npc.ts:553-556`.
3. Adds explicit `if sf == nil { continue }` guard since we no longer go through `runNpcScript`'s built-in nil-check.

**Do not** modify any other file in this task. The local field-name `req.IntArg` stays unchanged in Task 1 — Task 2 ripples the rename.

- [ ] **Step 1.4: Run the new test, expect GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessNpcQueue_SetsStateLastInt -v
```

Expected: PASS.

- [ ] **Step 1.5: Run full cross-package suite to verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all green. Watch in particular for the existing `TestNpcTurnFiresQueuedEntryWhenDelayZero`, `TestNpcTurnDoesNotDecrementQueueWhileDelayed`, `TestNpcTurnReentryQueueAppendDuringIteration`, `TestProcessNpcQueueDispatchesAtZeroDelay` (if present) — these all exercise `processNpcQueue` and must remain green.

- [ ] **Step 1.6: Commit**

```bash
git add modules/world/npc_script.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(nai-123): processNpcQueue copies req.IntArg → state.LastInt

ai_queueN content scripts (e.g. [ai_queue2,_] ~npc_default_damage(last_int))
read state.LastInt via the last_int opcode. processNpcQueue previously
passed req.IntArg as a positional script arg via runNpcScript's intArgs
slice; ai_queue scripts have no declared positional args, so the value
landed in unused IntLocals[0] and last_int read 0 → 0-damage hitsplats
on Tutorial Island giant rats despite XP being awarded by the upstream
~give_combat_experience proc.

Mirrors TS Npc.ts:553-556:
  const state = ScriptRunner.init(script, this, null, request.args);
  state.lastInt = request.lastInt;
  this.executeScript(state);

Pinned by TestProcessNpcQueue_SetsStateLastInt — registers a probe
script (OpLastInt; OpNpcSetTimer; OpReturn) at TriggerAiQueue2,
enqueues with intArg=42, asserts n.timerInterval=42 after turn().

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Post-commit (controller verification):**
- `git show HEAD --stat` — confirm scope: 2 files, ~30 LOC test + ~5 LOC prod.
- `git status` — confirm clean tree (no worktree strays per `feedback_subagent_wt_path`).

---

## Task 2: Mechanical TS-fidelity rename `IntArg` → `LastInt`

**Subagent:** Sonnet implementer.

**Files (all modify):**
- `pkg/script/queue.go:42-46`
- `pkg/script/active.go:700-705`
- `pkg/script/handlers_npc.go:332-355`
- `pkg/script/handlers_npc_test.go:191-195` (mockEnqueueCall struct), `:348-354` (impl), `:864-873` (assertions in `TestHandleNpcQueueEnqueues`)
- `modules/world/npc.go:328-338`
- `modules/world/npc_script.go:482` (the `req.IntArg` read landed in Task 1 — flips to `req.LastInt`)
- `modules/world/npc_test.go:481`, `:546`
- `modules/world/npc_event_queue_test.go:60`
- `modules/world/npc_script_test.go:189-190`

**Pre-flight (controller):**
- `rg "IntArg\b" pkg/script/ modules/world/` — confirm only the 11 sites enumerated above (NpcQueueRequest field — singular `IntArg` — distinct from `IntArgs` plural for player-side parallel-slice convention which is OUT OF SCOPE).
- `rg "intArg\b" pkg/script/ modules/world/` — local var / param refs.
- Confirm `IntArgs` (plural, PlayerQueueRequest field for parallel-slice positional args) is **not** in the rename set — different semantic.

- [ ] **Step 2.1: Rename `NpcQueueRequest.IntArg` → `LastInt` in `pkg/script/queue.go`**

Replace `pkg/script/queue.go:35-46`:

```go
// NpcQueueRequest is an NPC-side enqueue entry. Unlike
// PlayerQueueRequest, it has no queue-type distinction — TS's NPC
// queue has no strong/weak/long variants. The Trigger is one of
// TriggerAiQueue1..TriggerAiQueue20 and identifies which script runs
// at fire time (resolved via scriptProvider.GetByTrigger on the
// NPC's type + category).
//
// LastInt is the queued integer arg; processNpcQueue copies it into
// state.LastInt before executing the dispatched script (mirrors TS
// Npc.ts:554-555). The dispatched ai_queueN script reads it via the
// last_int opcode — it is NOT a positional script arg.
//
// NAI-123 DEVIATION-D1: TS NpcQueueRequest has separate args[] +
// lastInt fields. The args[] field is always [] at the one enqueue
// site (TS Npc.ts:242), so goscape collapses to a single LastInt
// field. Retire when a future content surface uses positional
// ai_queue args.
//
// Matches TS NpcQueueRequest at
// Engine-TS/src/engine/entity/NpcQueueRequest.ts:17.
type NpcQueueRequest struct {
	Trigger ServerTriggerType
	Delay   int
	LastInt int
}
```

- [ ] **Step 2.2: Rename interface param in `pkg/script/active.go:705`**

Replace lines 700-705:

```go
	// EnqueueScriptForTrigger appends a queued ai_queueN dispatch to
	// the NPC. Matches TS Npc.enqueueScript at Npc.ts:241-245 — the
	// trigger (TriggerAiQueue1..TriggerAiQueue20) identifies which
	// script runs; lookup happens at fire time via
	// scriptProvider.GetByTrigger keyed on the NPC's type + category.
	// lastIntArg is stored on the queue entry and copied into
	// state.LastInt at fire time (TS Npc.ts:554-555).
	EnqueueScriptForTrigger(trigger ServerTriggerType, delay int, lastIntArg int)
```

- [ ] **Step 2.3: Update `handleNpcQueue` local var + call site in `pkg/script/handlers_npc.go:339-355`**

Replace function body:

```go
func handleNpcQueue(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_QUEUE"); err != nil {
		return err
	}
	delay := s.PopInt()
	if err := checkNotNull(delay, "NPC_QUEUE"); err != nil {
		return err
	}
	lastIntArg := s.PopInt()
	queueID := s.PopInt()
	if queueID < 1 || queueID > 20 {
		return fmt.Errorf("NPC_QUEUE: invalid queueId %d (want 1..20)", queueID)
	}
	trigger := TriggerAiQueue1 + ServerTriggerType(queueID-1)
	s.ActiveNpc.EnqueueScriptForTrigger(trigger, delay, lastIntArg)
	return nil
}
```

(Doc comment at lines 332-338 stays unchanged — already accurately describes the contract.)

- [ ] **Step 2.4: Update concrete impl in `modules/world/npc.go:328-338`**

Replace:

```go
// EnqueueScriptForTrigger appends a queued ai_queueN dispatch.
// Implements script.ActiveNpc.EnqueueScriptForTrigger. Script
// resolution is deferred to fire time via
// scriptProvider.GetByTrigger — matches TS Npc.enqueueScript.
// lastIntArg is stored on the request and copied into state.LastInt
// at fire time (mirrors TS Npc.ts:554-555).
func (n *Npc) EnqueueScriptForTrigger(trigger script.ServerTriggerType, delay, lastIntArg int) {
	n.queue = append(n.queue, script.NpcQueueRequest{
		Trigger: trigger,
		Delay:   delay,
		LastInt: lastIntArg,
	})
}
```

- [ ] **Step 2.5: Update `processNpcQueue` to read renamed field**

In `modules/world/npc_script.go`, the line landed in Task 1:

```go
		lastIntArg := req.IntArg
```

becomes:

```go
		lastIntArg := req.LastInt
```

(Single-line change.)

- [ ] **Step 2.6: Update mockEnqueueCall + impl in `pkg/script/handlers_npc_test.go`**

At lines 191-195 (struct definition):

```go
type mockEnqueueCall struct {
	trigger    ServerTriggerType
	delay      int
	lastIntArg int
}
```

At lines 348-354 (mockNpc impl):

```go
func (m *mockNpc) EnqueueScriptForTrigger(trigger ServerTriggerType, delay, lastIntArg int) {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueueCall{
		trigger:    trigger,
		delay:      delay,
		lastIntArg: lastIntArg,
	})
}
```

At lines 871-873 (assertion in `TestHandleNpcQueueEnqueues`):

```go
	if call.lastIntArg != 42 {
		t.Errorf("lastIntArg: got %d, want 42", call.lastIntArg)
	}
```

- [ ] **Step 2.7: Update test struct literals in `modules/world/npc_test.go`**

At line 481:

```go
	n.queue = []script.NpcQueueRequest{{Trigger: 0, Delay: 5, LastInt: 42}}
```

At line 546:

```go
	n.queue = []script.NpcQueueRequest{{Trigger: 0, Delay: 5, LastInt: 42}}
```

(Per `plan_doc_replaceall_timeline`: do NOT use `replace_all` — apply each Edit with surrounding context to avoid timeline-divergent literal collisions in unrelated test fixtures.)

- [ ] **Step 2.8: Update `modules/world/npc_event_queue_test.go:60`**

```go
	n.queue = []script.NpcQueueRequest{{Trigger: script.TriggerAiQueue1, Delay: 5, LastInt: 0}}
```

- [ ] **Step 2.9: Update `modules/world/npc_script_test.go:189-190`**

```go
	if req.LastInt != 42 {
		t.Errorf("LastInt: got %d, want 42", req.LastInt)
	}
```

- [ ] **Step 2.10: Run full cross-package suite + vet + build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all green. Verify in particular `TestProcessNpcQueue_SetsStateLastInt` (still green from Task 1), `TestHandleNpcQueueEnqueues` (mock-side rename), `TestNpcEnqueueScriptForTrigger` (struct-literal-read rename), `TestNpcRevertTypeClearsQueue` (literal rename), `TestRevertTypeHonorsResetOnRevertTrue` (literal rename — note this test's name is at line 525-onward, the literal is at 546).

If a stale `IntArg` site surfaces post-build (Go's strict struct-field naming will surface every site at compile time), fix it inline and re-run before commit.

- [ ] **Step 2.11: Commit**

```bash
git add pkg/script/queue.go pkg/script/active.go pkg/script/handlers_npc.go \
        pkg/script/handlers_npc_test.go \
        modules/world/npc.go modules/world/npc_script.go \
        modules/world/npc_test.go modules/world/npc_event_queue_test.go \
        modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(nai-123): rename NpcQueueRequest.IntArg → LastInt

Mechanical TS-fidelity rename. The field semantically holds the queued
arg that the dispatcher copies into state.LastInt at fire time
(NAI-123 wiring fix); the IntArg name (singular vs plural IntArgs used
for parallel-slice positional convention) was misleading.

Ripples through the ActiveNpc interface signature, mockEnqueueCall,
test struct literals, and the read site in processNpcQueue.
DEVIATION-NAI-123-D1 declared in pkg/script/queue.go: goscape collapses
TS's split args[]+lastInt to a single LastInt because TS args[] is
always [] at the one enqueue site (Npc.ts:242).

No behavior change vs Task 1 commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Post-commit (controller verification):**
- `git show HEAD --stat` — confirm scope: 9 files, rename-only ripples (~20 lines net).
- `git status` — confirm clean.
- `rg "IntArg\b" pkg/script/ modules/world/ | grep -v IntArgs` — should return zero hits other than `IntArgCount` (a distinct ScriptFile field, OUT OF SCOPE).

---

## Task 3: Sonnet code-reviewer pass

**Subagent:** `superpowers:code-reviewer` on **Sonnet** (NEVER Opus per `superpowers_code_reviewer_model`).

**Pre-flight (controller):**
- Confirm `git log --oneline -3` shows Task 1 commit + Task 2 commit at HEAD.

- [ ] **Step 3.1: Dispatch reviewer**

Reviewer prompt:

> Review the two-commit bundle implementing NAI-123 (zero-damage residual fix). Spec at `docs/superpowers/specs/2026-05-07-nai-123-zero-damage-investigation-design.md` (commit `8feee80`). Plan at `docs/superpowers/plans/2026-05-07-nai-123-zero-damage-investigation.md`.
>
> Commit 1 (`HEAD~1`): wiring fix in `processNpcQueue` + new test `TestProcessNpcQueue_SetsStateLastInt`.
> Commit 2 (`HEAD`): mechanical rename `NpcQueueRequest.IntArg` → `LastInt`.
>
> Audit specifically: (a) does Commit 1 correctly mirror TS `Npc.ts:553-556`? (b) does the new test cover the failure mode end-to-end (state.LastInt observable through SetTimer)? (c) does Commit 2 catch every `IntArg` site (cross-package — `pkg/script` interface, `modules/world` impl, test mocks, struct literals)? (d) any divergence from `controller_preflight` line numbers in the plan vs HEAD?
>
> Out of scope: NAI-26 player-side `IntArgs` parallel-slice convention (different semantic, not part of this rename).

- [ ] **Step 3.2: Address findings**

If the reviewer flags a real issue (confidence-filtered), apply the fix inline. Re-run `go test ./... && go vet ./... && go build ./...`. Commit as a reviewer-fix sub-commit:

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(nai-123): reviewer fix — <one-line description>

<reviewer finding text>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If the reviewer flags only stylistic/aesthetic items below confidence threshold, document in the close commit body and skip.

- [ ] **Step 3.3: Final cross-package green check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all green.

---

## Smoke handoff (after Task 3)

Per `smoke_test_server_handoff`, the smoke must be user-launched. Controller emits a handoff message:

> **Smoke handoff — NAI-123.** Build at HEAD and run server with config.yaml. From `LostCityRS/Client-Java`, log in fresh char on Tutorial Island, attack the giant rat with bronze dagger. Observe ≥10 hits.
>
> **PRIMARY criterion:** at least one hit shows a non-zero red hitsplat (was: all blue 0-splats at NAI-122 close).
> **Regression check:** XP/hit stays in the ~50-100 band (not 0).
>
> Possible adjacent residuals (route per `cascade_theory_smoke_binding`): max-hit distribution feel, NAI-121 residual #2 (single-attacker contention), residual #3 (NPC non-retaliation).

---

## Self-review checklist (controller, after plan write)

- [x] **Spec coverage:** §1 symptom → Task 1 test asserts the fix. §2 root cause → Task 1 fix. §3 sibling sites → no change needed (verified TS-aligned in spec; reflected in plan as "out of scope"). §4 architecture table → Task 1 + Task 2 cover all 8 files. §5 deviation → Task 2.1 doc-comment carries DEVIATION-NAI-123-D1. §6 cadence → Task 3 + post-commit verification per task. §7 smoke → handoff message. §8/§9 patterns + cross-refs → reflected throughout.
- [x] **Placeholder scan:** No TBD/TODO. Every code block is the actual content the implementer pastes.
- [x] **Type consistency:** `lastIntArg` used consistently as the param name in interface (`pkg/script/active.go`), impl (`modules/world/npc.go`), mock (`pkg/script/handlers_npc_test.go`), and handler local (`pkg/script/handlers_npc.go`). Field name `LastInt` used consistently in struct (`pkg/script/queue.go`) and all reads (`modules/world/npc_script.go`, `modules/world/npc_script_test.go`). `mockEnqueueCall.lastIntArg` field matches param name.
- [x] **Sibling-site guard audit (`plan_sibling_site_guard_audit`):** Pre-flight grep for `IntArg\b` enumerates all 11 sites; the spec table cross-checked at HEAD before plan-author dispatch.
- [x] **Test runnability (`plan_runnable_test_fixtures`):** New test mentally compiled — `OpLastInt`/`OpNpcSetTimer` confirmed at `pkg/script/opcode.go`; `script.NewProvider`/`Register`/`GetByTrigger` confirmed at HEAD; `n.timerInterval` confirmed at `modules/world/npc.go:85`; `buildNpcForIntegration` confirmed at `modules/world/npc_script_test.go:231`; `n.turn(s)` confirmed as the dispatch trigger; `script.ScriptFile` 5-field literal (Name/LookupKey/Opcodes/IntOperands/StringOperands/InstructionCount) confirmed against the existing `nai21_amplifier_aiqueue1` fixture at line 315.
- [x] **`replace_all` safety (`plan_doc_replaceall_timeline`):** Step 2.7 explicitly forbids `replace_all` for the two npc_test.go literal sites; per-Edit with surrounding context.
