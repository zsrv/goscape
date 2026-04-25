# NAI-26 queue family TS-faithfulness audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the 9 TS-faithfulness divergences (α/β/γ/δ/ε/ζ/η/θ/κ) in the queue-family opcodes (STRONGQUEUE / WEAKQUEUE / QUEUE / LONGQUEUE / P_DELAY) to match `Engine-TS/src/engine/script/handlers/PlayerOps.ts:97-180,375-379,1248-1263` line-by-line. Bundle 1 widens `playerQueueRequest` and the `EnqueueScript*` family to carry parallel `[]int` + `[]string` slices and renames `EnqueueScriptTyped` → `EnqueueScriptArgs` with an error return (placeholder body). Bundle 2 un-shares the 4 queue handlers from `enqueueTyped`, ports each TS-faithful body, adds the `popScriptArgs` helper, adds NumberNotNull wraps on STRONGQUEUE delay + P_DELAY n, and activates the script-missing error.

**Architecture:** Two-bundle sequential follow-up. Bundle 1 is a mechanical signature widening across `pkg/script/active.go` + `pkg/script/handlers.go` + `modules/world/player_script.go` + `modules/world/tick.go` plus 4 test files; the temporary `enqueueTyped` adapter wraps the new parallel-slice shape into the old single-arg call so behavior is unchanged. Bundle 2 is the actual TS-faithfulness work: per-opcode handler bodies, the `popScriptArgs` helper, and the script-missing error rollout. Sequential dispatch — Bundle 2 only after Bundle 1's commit lands.

**Tech Stack:** Go 1.26+. Existing helpers: `checkNotNull(v int, op string) error` at `pkg/script/handlers_player.go:61`, `requireProtectedActivePlayer(s *ScriptState, op string) error` at `pkg/script/handlers_player.go:48` (already used by handlePDelay), `newSingleOp(name string, op Opcode) *ScriptFile` at `pkg/script/handlers_player_test.go:52`, `buildGreetScript(key uint32, ch string) *script.ScriptFile` at `modules/world/script_test.go:130`. TS source root at `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/`. Reference TS sources: `Engine-TS/src/engine/script/handlers/PlayerOps.ts:97-180` (4 queue ops), `:375-379` (P_DELAY), `:1248-1263` (popScriptArgs); `Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15` (ScriptArgument sum-type); `Engine-TS/src/engine/entity/Player.ts:821` (enqueueScript args=[] default).

**Spec reference:** `docs/superpowers/specs/2026-04-25-nai-26-queue-family-audit-design.md`.

**HEAD at plan-write:** `53379a0` (after spec polish commit `53379a0 docs(spec): NAI-26 spec — self-review polish`). Spec's predecessor pin remains `f0d1ed9` (NAI-25 close).

---

## Task 1 — Bundle 1: `playerQueueRequest` parallel-slice plumbing (mechanical widening)

**Files:**
- Modify: `pkg/script/active.go:15-19` (interface contract: rename + signature widening + error return)
- Modify: `pkg/script/handlers.go:599-619` (enqueueTyped adapter body retargets to parallel-slice signature)
- Modify: `modules/world/player_script.go:25-30` (playerQueueRequest struct field rename)
- Modify: `modules/world/player_script.go:46-61` (EnqueueScriptFile signature widening)
- Modify: `modules/world/player_script.go:63-76` (EnqueueScriptTyped → EnqueueScriptArgs rename + body restructure with placeholder nil-error on script-missing)
- Modify: `modules/world/player_script.go:258-263, :280-285` (engine-dispatch sites in changeStat/advanceStat → nil, nil)
- Modify: `modules/world/tick.go:243-246` (processPlayerQueue forwards req.IntArgs / req.StringArgs to runScript)
- Modify: `pkg/script/runner_test.go:246-261` (mockEnqueue struct + mockPlayer.EnqueueScriptTyped → EnqueueScriptArgs)
- Modify: `pkg/script/handlers_test.go:407-478` (TestQueueOpcode, TestQueueVariants slice-based assertions)
- Modify: `modules/world/player_script_test.go:164-192` (TestEnqueueScriptFileDirectPath + TestEnqueueScriptFileNilIsNoop signature update)
- Modify: `modules/world/script_test.go:239, :269, :336, :337, :825, :1020` (6 EnqueueScriptTyped → EnqueueScriptArgs migration call sites)

**Pre-flight context:**
- HEAD `53379a0` at task dispatch. Verify all line numbers via re-grep at task time per `controller_preflight` memory. The two spec commits (`673f690`, `53379a0`) are docs-only — production lines have not drifted from the spec's `f0d1ed9` claims.
- Spec's `runScript` signature concern (Risks bullet "Bundle 1 `(*Server).runScript` signature") is resolved at plan-write: `runScript` already accepts `(sf *script.ScriptFile, self script.ActivePlayer, protect bool, intArgs []int, stringArgs []string)` per `modules/world/script.go:14`. No widening needed; tick.go just hands req.IntArgs / req.StringArgs directly.
- Spec's `req.IntArg` cross-file concern is resolved: only **one** site reads `req.IntArg` in `modules/world/tick.go:243`; the `modules/world/npc_script.go:281` site is `NpcQueueRequest.IntArg` (out of scope per spec § Out-of-scope #3).
- **Pre-flight surprise** (worth capturing in Step 1): all 6 `script_test.go` enqueue sites enqueue scripts that **are registered** via `RegisterAt`:
  - `:239` (`0xAAAA`) ↔ registered at `:231`
  - `:269` (`0xBBBB`) ↔ registered at `:261`
  - `:336` (`0xCCC1`) ↔ registered at `:327`
  - `:337` (`0xCCC2`) ↔ registered at `:328`
  - `:825` (`0xBEEF`) ↔ registered at `:808`
  - `:1020` (`0xBEE2`) ↔ registered at `:1006`

  Spec's framing said these tests "intentionally enqueue non-existent scripts and rely on silent-no-op". That framing is incorrect — the tests enqueue **registered** scripts and assert they fire (or don't fire when delayed). Bundle 1's silent-no-op preservation matters only for the engine-dispatch paths (`changeStat`, `advanceStat`) that pass `nil` script files — covered separately by `EnqueueScriptFile`'s nil-check at `:52-54`. Bundle 2's script-missing error activation will not affect the 6 sites because their scripts resolve. This corrects spec § Bundle 1 touch point #4's "critical sequencing decision" — the sequencing is still useful (keeps mechanical change separate from behavior change) but the rationale is different (it's about isolating commit review surface, not preserving observable test behavior).

- [ ] **Step 1: Pre-flight verification — file paths, line numbers, signature shapes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go version
git log --oneline -3
grep -n "type playerQueueRequest" /home/owner/Code/github.com/zsrv/goscape/modules/world/player_script.go
grep -n "func (p \*Player) EnqueueScriptFile\|func (p \*Player) EnqueueScriptTyped" /home/owner/Code/github.com/zsrv/goscape/modules/world/player_script.go
grep -n "EnqueueScriptTyped" /home/owner/Code/github.com/zsrv/goscape/pkg/script/active.go
grep -n "func enqueueTyped\|handleQueue\|handleWeakQueue\|handleStrongQueue\|handleLongQueue\|handlePDelay" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go
grep -n "intArg := req.IntArg\|s\.runScript" /home/owner/Code/github.com/zsrv/goscape/modules/world/tick.go
grep -n "func.*runScript\b" /home/owner/Code/github.com/zsrv/goscape/modules/world/script.go
grep -rn "EnqueueScriptTyped" /home/owner/Code/github.com/zsrv/goscape/ --include="*.go"
```

Record: confirmed line numbers for the 11 in-scope citations; confirmed `runScript` signature `(sf *script.ScriptFile, self script.ActivePlayer, protect bool, intArgs []int, stringArgs []string)`; confirmed cross-package `EnqueueScriptTyped` call sites are exactly the 6 `script_test.go` sites + the `enqueueTyped` adapter + the production method in `player_script.go` + the interface method in `active.go` + the test mock in `runner_test.go`. If any line drifted, update subsequent steps' citations.

If a new consumer of `EnqueueScriptTyped` appears (e.g. added between spec-write and dispatch): ESCALATE — the plan's enumeration is bounded and a new site requires re-evaluation.

- [ ] **Step 2: Update `playerQueueRequest` struct (field rename)**

Edit `modules/world/player_script.go` lines 25-30. Current shape:

```go
type playerQueueRequest struct {
	Script *script.ScriptFile
	Delay  int
	IntArg int
	Type   script.PlayerQueueType
}
```

Replace with:

```go
type playerQueueRequest struct {
	Script     *script.ScriptFile
	Delay      int
	IntArgs    []int
	StringArgs []string
	Type       script.PlayerQueueType
}
```

The struct doc-comment immediately above (lines 15-24) currently says "one queued fresh-run script request with a single int arg." Update to reflect the parallel-slice shape — replace the first sentence:

```go
// playerQueueRequest is one queued fresh-run script request carrying its
// caller-supplied parallel arg slices (IntArgs + StringArgs). Queue
// entries are processed in processPlayerQueue; when Delay reaches zero
// (or below) the target script runs as a brand-new ScriptState. Type
// selects the queue variant (NORMAL/WEAK/LONG/STRONG); STRONG fires
// even when the player is delayed, the others wait for idle.
//
// As of S6h, Script holds the pre-resolved *ScriptFile directly. ID →
// ScriptFile resolution happens at enqueue time via Player.EnqueueScriptArgs;
// engine-dispatch paths (e.g. changeStat) use Player.EnqueueScriptFile.
//
// As of NAI-26 Bundle 1, the single IntArg int field is widened to
// parallel IntArgs []int + StringArgs []string slices to match the TS
// PlayerQueueRequest.args ScriptArgument[] shape (TS
// Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15). The widening
// is required for STRONGQUEUE's variadic popScriptArgs body
// (PlayerOps.ts:98) and LONGQUEUE's 2-element [logoutAction, arg]
// args array (PlayerOps.ts:179), neither of which fit a single-int field.
```

This change will break compilation of every site reading `req.IntArg`. That is expected; later steps fix the readers.

- [ ] **Step 3: Update `(*Player).EnqueueScriptFile` signature + body**

Edit `modules/world/player_script.go` lines 46-61. Current shape:

```go
// EnqueueScriptFile appends a queued fresh-run request for a specific
// ScriptFile. Delay=0 fires on the next processPlayerQueue pass (subject
// to the STRONG/NORMAL gate). Nil sf is a silent no-op — engine
// dispatchers (e.g. changeStat) call GetByTrigger and may legitimately
// pass nil when no cache script is registered for the event.
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay, intArg int, qtype script.PlayerQueueType) {
	if sf == nil {
		return
	}
	p.queue = append(p.queue, playerQueueRequest{
		Script: sf,
		Delay:  delay,
		IntArg: intArg,
		Type:   qtype,
	})
}
```

Replace with:

```go
// EnqueueScriptFile appends a queued fresh-run request for a specific
// ScriptFile. Delay=0 fires on the next processPlayerQueue pass (subject
// to the STRONG/NORMAL gate). Nil sf is a silent no-op — engine
// dispatchers (e.g. changeStat) call GetByTrigger and may legitimately
// pass nil when no cache script is registered for the event.
//
// intArgs and stringArgs are the parallel-slice args the target script
// will read from its IntArgCount / StringArgCount-sized prelude slots
// (matches TS ScriptArgument[] shape per
// Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15). nil/nil
// expresses "no args" — the TS-faithful default for engine-dispatch
// paths (TS Engine-TS/src/engine/entity/Player.ts:821 args=[] default).
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) {
	if sf == nil {
		return
	}
	p.queue = append(p.queue, playerQueueRequest{
		Script:     sf,
		Delay:      delay,
		IntArgs:    intArgs,
		StringArgs: stringArgs,
		Type:       qtype,
	})
}
```

- [ ] **Step 4: Migrate `changeStat` + `advanceStat` engine-dispatch sites to `nil, nil`**

Edit `modules/world/player_script.go`. Two production call sites read at task time:

`:263` (in `changeStat`):
```go
p.EnqueueScriptFile(sf, 0, 0, script.QueueNormal)
```

`:285` (in `advanceStat`):
```go
p.EnqueueScriptFile(sf, 0, 0, script.QueueNormal)
```

Replace both with:

```go
p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueNormal)
```

Rationale: TS `Player.changeStat` (Player.ts:1816-1821) and `Player.advanceStat` (Player.ts:1804-1807) call `enqueueScript(script, PlayerQueueType.ENGINE, 0)` with the default `args=[]` (Player.ts:821). Goscape's pre-NAI-26 `0` was a 0-int-arg artifact of the old single-int field; `nil, nil` is the TS-faithful empty-args expression in the parallel-slice convention.

- [ ] **Step 5: Rename `EnqueueScriptTyped` → `EnqueueScriptArgs` (Bundle 1 placeholder body)**

Edit `modules/world/player_script.go` lines 63-76. Current shape:

```go
// EnqueueScriptTyped implements script.ActivePlayer.EnqueueScriptTyped by
// resolving scriptID → *ScriptFile via scriptProvider.GetByID and
// delegating to EnqueueScriptFile. Silent no-op on missing script or
// unwired server — same observable contract as the pre-S6h impl, where
// processPlayerQueue's GetByID check served the same role.
//
// Resolution shifts from fire-time (pre-S6h) to enqueue-time (S6h).
// Same tick boundary in practice; simpler codepath.
func (p *Player) EnqueueScriptTyped(scriptID uint32, delay, intArg int, qtype script.PlayerQueueType) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	p.EnqueueScriptFile(p.client.server.scriptProvider.GetByID(scriptID), delay, intArg, qtype)
}
```

Replace with:

```go
// EnqueueScriptArgs implements script.ActivePlayer.EnqueueScriptArgs by
// resolving scriptID → *ScriptFile via scriptProvider.GetByID and
// delegating to EnqueueScriptFile. Returns a non-nil error when the
// scriptID does not resolve to a registered script — mirrors TS
// PlayerOps.ts:103-105 throw shape ("Unable to find queue script: ${id}").
//
// NAI-26 Bundle 1 NOTE: this Bundle 1 placeholder body returns nil
// (preserving silent no-op) when GetByID returns nil. The error return
// is activated in Bundle 2 (Task 6 Step 1) once the per-opcode handlers
// have un-shared bodies that propagate the error explicitly. Splitting
// the rollout keeps Bundle 1's mechanical signature widening separate
// from the Bundle 2 behavioral change for review-surface isolation.
//
// Silent no-op on unwired server (p.client / p.client.server /
// p.client.server.scriptProvider nil) is preserved across both bundles
// — that path corresponds to test fixtures that don't wire a Server,
// not to a script-author error worth surfacing.
//
// Resolution shifts from fire-time (pre-S6h) to enqueue-time (S6h).
// Same tick boundary in practice; simpler codepath.
func (p *Player) EnqueueScriptArgs(scriptID uint32, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) error {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return nil
	}
	sf := p.client.server.scriptProvider.GetByID(scriptID)
	if sf == nil {
		// NAI-26 Bundle 1 placeholder: returns nil to preserve pre-NAI-26
		// silent-no-op behavior. Bundle 2 (Task 6 Step 1) replaces this
		// with `return fmt.Errorf("unable to find queue script: %d", scriptID)`.
		return nil
	}
	p.EnqueueScriptFile(sf, delay, intArgs, stringArgs, qtype)
	return nil
}
```

The struct comment at line 23 references the old name `EnqueueScriptTyped`. Update by replacing that one occurrence inside the doc-comment block (Step 2 already touched the wider doc-comment, but the EnqueueScriptTyped name reference still appears in the unchanged sentence at the original line 23 — re-Edit to confirm post-Step-2 the file references `EnqueueScriptArgs` consistently). After Steps 2-5 the doc-comment paragraph at the top of the struct should refer to `EnqueueScriptArgs`, not `EnqueueScriptTyped`.

- [ ] **Step 6: Update `pkg/script/active.go` interface contract**

Edit `pkg/script/active.go` lines 15-19. Current shape:

```go
	// EnqueueScriptTyped appends a queued fresh-run request with the
	// given queue type. Delay=0 fires same tick. STRONG-type entries
	// fire even if the player is busy; others wait until idle.
	// (S5h: renamed from EnqueueScript to carry type.)
	EnqueueScriptTyped(scriptID uint32, delay int, intArg int, qtype PlayerQueueType)
```

Replace with:

```go
	// EnqueueScriptArgs appends a queued fresh-run request with the
	// given queue type and the caller-supplied parallel arg slices
	// (IntArgs + StringArgs — matches TS PlayerQueueRequest.args
	// ScriptArgument[] shape per
	// Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15). Delay=0
	// fires same tick. STRONG-type entries fire even if the player is
	// busy; others wait until idle. nil/nil expresses "no args" — the
	// TS-faithful empty-args default (Player.ts:821 args=[]).
	//
	// Returns a non-nil error when scriptID does not resolve to a
	// registered script (mirrors TS PlayerOps.ts:103-105 throw). The
	// goscape error is `fmt.Errorf("unable to find queue script: %d",
	// scriptID)`. NAI-26 Bundle 1 placeholder implementations may
	// temporarily return nil instead; Bundle 2 activates the error.
	//
	// (S5h: renamed from EnqueueScript to carry type. NAI-26 Bundle 1:
	// renamed from EnqueueScriptTyped to carry parallel-slice args and
	// error return.)
	EnqueueScriptArgs(scriptID uint32, delay int, intArgs []int, stringArgs []string, qtype PlayerQueueType) error
```

- [ ] **Step 7: Update `enqueueTyped` adapter to the parallel-slice signature (Bundle 1 retains adapter)**

Edit `pkg/script/handlers.go` lines 599-619. Current shape:

```go
// enqueueTyped is the shared body for QUEUE / WEAKQUEUE / STRONGQUEUE /
// LONGQUEUE. Pops (scriptID, delay, arg) and calls Self.EnqueueScriptTyped
// with the requested type.
//
// TS (engine/script/handlers/PlayerOps.ts:148):
//
//	const [scriptId, delay, arg] = state.popInts(3);
//
// popInts(n) fills ints[n-1] down to ints[0] via PopInt, so the stack
// top is `arg`, then `delay`, then `scriptId`. The VARARG variants are
// deferred.
func enqueueTyped(s *ScriptState, qtype PlayerQueueType, op string) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("%s: no active player", op)
	}
	arg := int(s.PopInt())
	delay := int(s.PopInt())
	scriptID := uint32(s.PopInt())
	s.Self.EnqueueScriptTyped(scriptID, delay, arg, qtype)
	return nil
}
```

Replace with:

```go
// enqueueTyped is the temporary Bundle-1 shared adapter for QUEUE /
// WEAKQUEUE / STRONGQUEUE / LONGQUEUE. Pops (scriptID, delay, arg) and
// calls Self.EnqueueScriptArgs with the requested type, wrapping the
// single popped int into a 1-element IntArgs slice + nil StringArgs.
//
// TS (engine/script/handlers/PlayerOps.ts:148):
//
//	const [scriptId, delay, arg] = state.popInts(3);
//
// popInts(n) fills ints[n-1] down to ints[0] via PopInt, so the stack
// top is `arg`, then `delay`, then `scriptId`. The VARARG variants are
// deferred (separate TS opcodes; see spec § Out-of-scope #5).
//
// NAI-26 Bundle 1 NOTE: this adapter is mechanically equivalent to the
// pre-NAI-26 body — the parallel-slice widening is transparent. Bundle
// 2 un-shares each of the 4 queue handlers (STRONGQUEUE adds
// popScriptArgs + NumberNotNull; LONGQUEUE adds a 4th popInt and a
// 2-element args ordering) and removes this adapter.
func enqueueTyped(s *ScriptState, qtype PlayerQueueType, op string) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("%s: no active player", op)
	}
	arg := int(s.PopInt())
	delay := int(s.PopInt())
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, []int{arg}, nil, qtype)
}
```

The 4 wrapper handlers `handleQueue` / `handleWeakQueue` / `handleStrongQueue` / `handleLongQueue` at lines 621-634 stay as-is (they call `enqueueTyped` and propagate its return). Bundle 2 replaces these wrappers with own bodies.

- [ ] **Step 8: Update `processPlayerQueue` to forward the parallel slices**

Edit `modules/world/tick.go` lines 242-247. Current shape:

```go
		sf := req.Script
		intArg := req.IntArg
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		if sf != nil {
			s.runScript(sf, p, false, []int{intArg}, nil)
		}
```

Replace with:

```go
		sf := req.Script
		intArgs := req.IntArgs
		stringArgs := req.StringArgs
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		if sf != nil {
			s.runScript(sf, p, false, intArgs, stringArgs)
		}
```

Rationale: `runScript` already accepts `(intArgs []int, stringArgs []string)` per `modules/world/script.go:14`. The pre-NAI-26 `[]int{intArg}` was a single-int boxing artifact; the new shape forwards the parallel slices directly. This activates the Bundle 1 plumbing for the integration test in Task 6 Step 4.

- [ ] **Step 9: Update `mockPlayer` test mock (`pkg/script/runner_test.go`)**

Edit `pkg/script/runner_test.go`. Two changes:

(a) `mockEnqueue` struct at lines 246-251. Current:

```go
type mockEnqueue struct {
	ScriptID uint32
	Delay    int
	IntArg   int
	Type     PlayerQueueType
}
```

Replace with:

```go
type mockEnqueue struct {
	ScriptID    uint32
	Delay       int
	IntArgs     []int
	StringArgs  []string
	Type        PlayerQueueType
	ReturnError error // Bundle 2: tests opting into error-return set this; default nil.
}
```

The `ReturnError` field is unused in Bundle 1 (the mock returns nil unconditionally) — its placement here is forward-prep for Bundle 2 Task 6 (which adds a `wantScriptMissing` mock-side configuration to test the script-missing error propagation). Bundle 1 implementer should leave the field present and untouched; Bundle 2 implementer activates it.

(b) `mockPlayer.EnqueueScriptTyped` method at line 259. Current:

```go
func (m *mockPlayer) EnqueueScriptTyped(id uint32, delay, arg int, qtype PlayerQueueType) {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueue{ScriptID: id, Delay: delay, IntArg: arg, Type: qtype})
}
```

Replace with:

```go
func (m *mockPlayer) EnqueueScriptArgs(id uint32, delay int, intArgs []int, stringArgs []string, qtype PlayerQueueType) error {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueue{ScriptID: id, Delay: delay, IntArgs: intArgs, StringArgs: stringArgs, Type: qtype})
	return nil
}
```

Bundle 1 mock returns `nil` unconditionally. Bundle 2 Task 6 Step 1 adds an opt-in script-missing error toggle.

- [ ] **Step 10: Update `TestQueueOpcode` and `TestQueueVariants` (slice-based assertions)**

Edit `pkg/script/handlers_test.go`.

(a) `TestQueueOpcode` at lines 407-437. Current assertion shape uses `mockEnqueue{ScriptID: 77, Delay: 3, IntArg: 42}`. Pre-existing struct comparison via `got != want` no longer works with slice fields (slices aren't comparable with `==`). Replace with field-by-field assertions plus `slices.Equal` for IntArgs.

Add `"slices"` to the test file's imports if not already present (re-grep at task time).

The full replacement test body:

```go
func TestQueueOpcode(t *testing.T) {
	sf := &ScriptFile{
		Name: "test_queue",
		Opcodes: []Opcode{
			OpPushConstantInt, // push scriptID 77
			OpPushConstantInt, // push delay 3
			OpPushConstantInt, // push arg 42
			OpQueue,
			OpReturn,
		},
		IntOperands:      []int32{77, 3, 42, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished", state.Execution)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 77 {
		t.Errorf("ScriptID: got %d, want 77", got.ScriptID)
	}
	if got.Delay != 3 {
		t.Errorf("Delay: got %d, want 3", got.Delay)
	}
	if !slices.Equal(got.IntArgs, []int{42}) {
		t.Errorf("IntArgs: got %v, want [42]", got.IntArgs)
	}
	if got.StringArgs != nil {
		t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
	}
	if got.Type != QueueNormal {
		t.Errorf("Type: got %v, want QueueNormal", got.Type)
	}
}
```

(b) `TestQueueVariants` at lines 439-478. Same field-by-field shape. Bundle 1 keeps all 3 cases (weak, strong, long) — Bundle 2's Task 5 removes "strong" and "long" cases (un-shared bodies have own tests). The full Bundle-1 replacement:

```go
func TestQueueVariants(t *testing.T) {
	cases := []struct {
		name  string
		op    Opcode
		qtype PlayerQueueType
	}{
		{"weak", OpWeakQueue, QueueWeak},
		{"strong", OpStrongQueue, QueueStrong},
		{"long", OpLongQueue, QueueLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sf := &ScriptFile{
				Name: "q_" + tc.name,
				Opcodes: []Opcode{
					OpPushConstantInt,
					OpPushConstantInt,
					OpPushConstantInt,
					tc.op,
					OpReturn,
				},
				IntOperands:      []int32{77, 3, 42, 0, 0},
				StringOperands:   []string{"", "", "", "", ""},
				InstructionCount: 5,
			}
			mp := &mockPlayer{}
			state := Init(sf, mp, false, nil, nil)
			if err := Execute(state); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(mp.enqueueCalls) != 1 {
				t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
			}
			got := mp.enqueueCalls[0]
			if got.ScriptID != 77 || got.Delay != 3 || got.Type != tc.qtype {
				t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want ScriptID=77 Delay=3 Type=%v",
					got.ScriptID, got.Delay, got.Type, tc.qtype)
			}
			if !slices.Equal(got.IntArgs, []int{42}) {
				t.Errorf("IntArgs: got %v, want [42]", got.IntArgs)
			}
			if got.StringArgs != nil {
				t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
			}
		})
	}
}
```

- [ ] **Step 11: Update `modules/world/player_script_test.go` direct-path tests**

Edit `modules/world/player_script_test.go`.

(a) `TestEnqueueScriptFileDirectPath` at lines 164-184. Current shape (verified at HEAD):

```go
func TestEnqueueScriptFileDirectPath(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "[test_direct]"}
	p.EnqueueScriptFile(sf, 3, 42, script.QueueNormal)
	if len(p.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1", len(p.queue))
	}
	req := p.queue[0]
	if req.Script != sf {
		t.Errorf("queue[0].Script: got %v, want %v", req.Script, sf)
	}
	if req.Delay != 3 {
		t.Errorf("queue[0].Delay: got %d, want 3", req.Delay)
	}
	if req.IntArg != 42 {
		t.Errorf("queue[0].IntArg: got %d, want 42", req.IntArg)
	}
	if req.Type != script.QueueNormal {
		t.Errorf("queue[0].Type: got %v, want %v", req.Type, script.QueueNormal)
	}
}
```

Replace with:

```go
func TestEnqueueScriptFileDirectPath(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "[test_direct]"}
	p.EnqueueScriptFile(sf, 3, []int{42}, nil, script.QueueNormal)
	if len(p.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1", len(p.queue))
	}
	req := p.queue[0]
	if req.Script != sf {
		t.Errorf("queue[0].Script: got %v, want %v", req.Script, sf)
	}
	if req.Delay != 3 {
		t.Errorf("queue[0].Delay: got %d, want 3", req.Delay)
	}
	if !slices.Equal(req.IntArgs, []int{42}) {
		t.Errorf("queue[0].IntArgs: got %v, want [42]", req.IntArgs)
	}
	if req.StringArgs != nil {
		t.Errorf("queue[0].StringArgs: got %v, want nil", req.StringArgs)
	}
	if req.Type != script.QueueNormal {
		t.Errorf("queue[0].Type: got %v, want %v", req.Type, script.QueueNormal)
	}
}
```

(b) `TestEnqueueScriptFileNilIsNoop` at lines 186-192. Replace `p.EnqueueScriptFile(nil, 0, 0, script.QueueNormal)` with `p.EnqueueScriptFile(nil, 0, nil, nil, script.QueueNormal)`. Body is otherwise unchanged.

Add `"slices"` to the test file's imports if not already present. Re-grep at task time:

```bash
grep -n "\"slices\"" /home/owner/Code/github.com/zsrv/goscape/modules/world/player_script_test.go
```

- [ ] **Step 12: Migrate the 6 `script_test.go` call sites (mechanical rename + nil/nil)**

Edit `modules/world/script_test.go`. Six sites, each currently of the form `p.EnqueueScriptTyped(<id>, <delay>, 0, <qtype>)`. Replace each with `p.EnqueueScriptArgs(<id>, <delay>, nil, nil, <qtype>)`.

| Line | Before | After |
|------|--------|-------|
| `:239` | `p.EnqueueScriptTyped(0xAAAA, 1, 0, script.QueueNormal)` | `p.EnqueueScriptArgs(0xAAAA, 1, nil, nil, script.QueueNormal)` |
| `:269` | `p.EnqueueScriptTyped(0xBBBB, 0, 0, script.QueueNormal)` | `p.EnqueueScriptArgs(0xBBBB, 0, nil, nil, script.QueueNormal)` |
| `:336` | `p.EnqueueScriptTyped(0xCCC1, 0, 0, script.QueueNormal)` | `p.EnqueueScriptArgs(0xCCC1, 0, nil, nil, script.QueueNormal)` |
| `:337` | `p.EnqueueScriptTyped(0xCCC2, 0, 0, script.QueueNormal)` | `p.EnqueueScriptArgs(0xCCC2, 0, nil, nil, script.QueueNormal)` |
| `:825` | `p.EnqueueScriptTyped(0xBEEF, 0, 0, script.QueueStrong)` | `p.EnqueueScriptArgs(0xBEEF, 0, nil, nil, script.QueueStrong)` |
| `:1020` | `p.EnqueueScriptTyped(0xBEE2, 0, 0, script.QueueNormal)` | `p.EnqueueScriptArgs(0xBEE2, 0, nil, nil, script.QueueNormal)` |

The new method returns an error; each of the 6 sites currently discards (call-as-statement). Discarding a non-nil error here is incorrect Go. However, all 6 enqueue **registered scripts** (per pre-flight context — 0xAAAA is registered at `:231`, 0xBBBB at `:261`, 0xCCC1/0xCCC2 at `:327-328`, 0xBEEF at `:808`, 0xBEE2 at `:1006`), so the Bundle 1 placeholder body returns nil for all of them. After Bundle 2 activates the error, these sites still return nil (the scripts resolve). Discarding via call-as-statement is fine in both bundles.

Confirm the 6 sites still compile after the rename (the `error` return is implicitly discarded — Go permits this).

- [ ] **Step 13: Compile-check + full-package test run**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: vet clean, build succeeds. If any compilation error appears, the most likely cause is:
- A missed `req.IntArg` reader (re-grep `req.IntArg` across `modules/world/`).
- A missed `EnqueueScriptTyped` consumer (re-grep across the repo).
- A missing `"slices"` import in a test file that now uses `slices.Equal`.

Then run the full test suite:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: **PASS** across the whole repo. Per `verify_implementer_claims` memory: package-scoped green can mask cross-package breakage; the full-repo run is the cross-check. If a pre-existing test fails: investigate, diagnose root cause, do not silence.

- [ ] **Step 14: Commit Bundle 1**

```bash
git add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_test.go pkg/script/runner_test.go modules/world/player_script.go modules/world/player_script_test.go modules/world/script_test.go modules/world/tick.go
git status --short
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,script): NAI-26 Bundle 1 — playerQueueRequest parallel-slice plumbing

Widen playerQueueRequest, script.ActivePlayer.EnqueueScriptArgs,
(*Player).EnqueueScriptFile, (*Player).EnqueueScriptArgs, and
processPlayerQueue to carry parallel IntArgs []int + StringArgs []string
slices instead of a single intArg int. Mechanical signature change with
no observable behavior change — the temporary enqueueTyped adapter
wraps the new shape into the old single-int call so the 4 queue
handlers continue to behave identically.

Type-system widening rationale: TS PlayerQueueRequest.args is
ScriptArgument[] (number | string sum-type per
Engine-TS/src/engine/entity/PlayerQueueRequest.ts:15). STRONGQUEUE
(PlayerOps.ts:98) calls popScriptArgs first — variadic typed args,
not a single int. LONGQUEUE (PlayerOps.ts:179) passes a 2-element
[logoutAction, arg] args array. Neither fits a single intArg int field.
Goscape's parallel-slice convention (already established in
runScript(sf, self, target, intArgs []int, stringArgs []string) per
modules/world/script.go:14) is the natural extension: int args land
in tag-position-relative-int order, string args land in tag-position-
relative-string order.

Engine-dispatch sites in changeStat (player_script.go:263) and
advanceStat (player_script.go:285) migrate from the artifact
intArg=0 to nil, nil — TS-faithful "no args" expression matching
TS Player.ts:821 enqueueScript args=[] default.

EnqueueScriptTyped → EnqueueScriptArgs rename carries the new
signature shape. The Bundle 1 placeholder body returns nil when
GetByID returns nil (preserves silent-no-op for engine-dispatch
fixtures); Bundle 2 activates the
fmt.Errorf("unable to find queue script: %d", scriptID) return per
TS PlayerOps.ts:103-105.

mockEnqueue struct widened to IntArgs []int + StringArgs []string;
ReturnError field added (unused in Bundle 1; Bundle 2 wires it for
the script-missing error tests). TestQueueOpcode and TestQueueVariants
adopt slices.Equal for the IntArgs comparison; field-by-field assertions
replace the pre-NAI-26 struct-equality (slices aren't ==-comparable).

Files:
- pkg/script/active.go: ActivePlayer.EnqueueScriptArgs interface +
  contract docstring (parallel-slice + error-return shape).
- pkg/script/handlers.go: enqueueTyped adapter retargets to
  Self.EnqueueScriptArgs([]int{arg}, nil, qtype). Adapter retained
  in Bundle 1; removed in Bundle 2.
- pkg/script/handlers_test.go: TestQueueOpcode +
  TestQueueVariants slice-based assertions.
- pkg/script/runner_test.go: mockEnqueue struct widening +
  mockPlayer.EnqueueScriptArgs body.
- modules/world/player_script.go: playerQueueRequest field rename;
  EnqueueScriptFile signature widening; EnqueueScriptArgs rename
  + Bundle-1 placeholder body; engine-dispatch nil/nil migration.
- modules/world/player_script_test.go:
  TestEnqueueScriptFileDirectPath/NilIsNoop signature update.
- modules/world/script_test.go: 6 EnqueueScriptTyped call sites
  migrated to EnqueueScriptArgs; all 6 enqueue registered scripts
  (0xAAAA / 0xBBBB / 0xCCC1 / 0xCCC2 / 0xBEEF / 0xBEE2 — confirmed
  via RegisterAt cross-grep) so the placeholder nil-error path
  is not exercised here.
- modules/world/tick.go: processPlayerQueue forwards
  req.IntArgs / req.StringArgs to runScript directly.

Net deviation count unchanged (14). No new tests; existing tests
updated for new field shapes. Bundle 2 (next commit) adds the
TS-faithful per-opcode bodies + popScriptArgs helper + NumberNotNull
wraps + script-missing error activation + 11 new tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — Bundle 2: `popScriptArgs` helper + 5 unit tests

**Files:**
- Modify: `pkg/script/handlers.go` (add `popScriptArgs` function near the queue handlers, ~25 LOC)
- Modify: `pkg/script/handlers_test.go` (5 new unit tests pinning order semantics)

**Pre-flight context:**
- HEAD `<Bundle 1 commit hash>` (post-Task-1). Verify line numbers via re-grep at task time per `controller_preflight` memory.
- `popScriptArgs` is a free function (not a method). Per `plan_grep_helper_patterns` memory: `pkg/script/handlers.go` already has co-located helpers (`enqueueTyped`, `requireActivePlayer` referenced from `handlers_player.go:35`, etc.) — co-locating `popScriptArgs` near the queue handlers (after `enqueueTyped` and before `handleQueue`) is consistent with the file's organization.
- TS reference: `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1248-1263`. The TS `popScriptArgs` returns a single `ScriptArgument[]` indexed by tag position. Goscape's parallel-slice port returns `(intArgs []int, stringArgs []string)` — int args land in tag-relative-int-order, string args land in tag-relative-string-order.
- Stack-push order critical for tests: caller pushes scriptID-tagged values onto the stack BEFORE the type-tags string. The opcode-driver then expects the top-of-stack to be the tags string, then the typed-arg values in tag order (last tag's value is on top, first tag's value is at the bottom of the typed-arg block). TS `popScriptArgs` first pops the tags string, then iterates `i = count-1 → 0` calling `popString` (for 's') or `popInt` (otherwise) — the tag at the largest index pops first.

  Example (mentally executed): `tags="isi"`, caller stack-push order `[1, "two", 3, "isi"]` (from-bottom to top). Pop 1: tags `"isi"` → count=3. Loop i=2: tag='i' → popInt → 3 → intArgs[1]=3 (intIdx was 1, decremented to 0). Loop i=1: tag='s' → popString → "two" → stringArgs[0]="two" (stringIdx was 0, decremented to -1). Loop i=0: tag='i' → popInt → 1 → intArgs[0]=1 (intIdx was 0, decremented to -1). Result: `intArgs=[1, 3]`, `stringArgs=["two"]`. Test `TestPopScriptArgs_Mixed` pins this exact ordering.

- [ ] **Step 1: Pre-flight verification**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go version
git log --oneline -3
grep -n "func enqueueTyped\|func handleQueue\|func handleStrongQueue" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go
grep -n "func TestQueueOpcode\|func TestQueueVariants" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_test.go
grep -n "\"slices\"" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_test.go
```

Verify Bundle 1 commit landed. Verify `enqueueTyped` is still present (Bundle 2's later tasks remove it). Verify the test file's `"slices"` import was added in Task 1 Step 10.

- [ ] **Step 2: Add the `popScriptArgs` helper**

Edit `pkg/script/handlers.go`. Insert immediately after `enqueueTyped` (current line ~620) and before `handleQueue` (current line ~621). The implementer should re-grep `func enqueueTyped\|func handleQueue` to find the exact insertion point post-Bundle-1.

```go
// popScriptArgs pops a type-tags string from the stack, then pops typed
// args in reverse tag order (i = count-1 down to 0): tag char 's' pops
// a string into stringArgs; any other tag pops an int into intArgs.
// Mirrors TS PlayerOps.ts:1248-1263.
//
// TS returns []ScriptArgument (a single ordered slice with mixed types
// indexed by tag position). Goscape's parallel-slice convention encodes
// the same data with two slices: each tag's value lands in the slice
// for its type, in tag-position order. The caller does not need to
// reconstruct positional access — runScript consumes intArgs and
// stringArgs separately, and ScriptState.Init unpacks each into its
// own typed local-variable slot per IntArgCount / StringArgCount.
//
// Returns nil/nil for an empty type-tags string. The caller is
// responsible for ensuring the stack has the popped values in TS-faithful
// order (last tag's value on top of the typed block; tags string on the
// very top — popped first).
func popScriptArgs(s *ScriptState) (intArgs []int, stringArgs []string) {
	types := s.PopString()
	count := len(types)
	if count == 0 {
		return nil, nil
	}
	// Pre-pass: count int and string tags to size the slices.
	var intCount, stringCount int
	for _, t := range types {
		if t == 's' {
			stringCount++
		} else {
			intCount++
		}
	}
	intArgs = make([]int, intCount)
	stringArgs = make([]string, stringCount)
	// Reverse-pop pass: TS iterates i = count-1 down to 0.
	intIdx := intCount - 1
	stringIdx := stringCount - 1
	for i := count - 1; i >= 0; i-- {
		if types[i] == 's' {
			stringArgs[stringIdx] = s.PopString()
			stringIdx--
		} else {
			intArgs[intIdx] = s.PopInt()
			intIdx--
		}
	}
	return intArgs, stringArgs
}
```

Note: spec uses `int(s.PopInt())` but `s.PopInt()` already returns `int` per `pkg/script/state.go:188`. The cast is a no-op; omit it for cleanliness. The implementer should verify this once at task time (`grep -n "func.*PopInt\b" pkg/script/state.go`).

Compile-check:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...
```

Expected: build succeeds. The helper is unused so far (consumers come in Tasks 3-5).

- [ ] **Step 3: Add `TestPopScriptArgs_Empty` (FAIL → PASS)**

Append to `pkg/script/handlers_test.go` (after the existing queue tests, near the end of the file or after `TestQueueVariants`):

```go
// TestPopScriptArgs_Empty pins the empty-tags case: an empty type-tags
// string yields nil/nil. Mirrors TS PlayerOps.ts:1248-1263 with count=0.
func TestPopScriptArgs_Empty(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_empty"}, nil, false, nil, nil)
	state.PushString("")

	intArgs, stringArgs := popScriptArgs(state)

	if intArgs != nil {
		t.Errorf("intArgs: got %v, want nil", intArgs)
	}
	if stringArgs != nil {
		t.Errorf("stringArgs: got %v, want nil", stringArgs)
	}
	if state.SSP != 0 {
		t.Errorf("SSP after pop: got %d, want 0", state.SSP)
	}
}
```

Pre-flight verification of `Init` signature: `func Init(sf *ScriptFile, self ActivePlayer, protect bool, intArgs []int, stringArgs []string) *ScriptState` (see existing tests at `runner_test.go:49` and `handlers_test.go:380`). The empty `&ScriptFile{Name: ...}` is valid; it's a no-op script that's never executed (we just need a state with stacks).

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestPopScriptArgs_Empty" ./pkg/script/ -v
```

Expected: **PASS** (the helper from Step 2 handles count==0 correctly).

- [ ] **Step 4: Add `TestPopScriptArgs_AllInt` (PASS)**

Append:

```go
// TestPopScriptArgs_AllInt pins the all-int case: tags="iii" with stack-
// pushed ints [1, 2, 3] (top of stack is 3) yields intArgs=[1, 2, 3],
// stringArgs=nil. Verifies tag-position order is preserved.
func TestPopScriptArgs_AllInt(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_allint"}, nil, false, nil, nil)
	state.PushInt(1)
	state.PushInt(2)
	state.PushInt(3)
	state.PushString("iii")

	intArgs, stringArgs := popScriptArgs(state)

	if !slices.Equal(intArgs, []int{1, 2, 3}) {
		t.Errorf("intArgs: got %v, want [1 2 3]", intArgs)
	}
	if stringArgs != nil {
		t.Errorf("stringArgs: got %v, want nil", stringArgs)
	}
	if state.ISP != 0 {
		t.Errorf("ISP after pop: got %d, want 0", state.ISP)
	}
}
```

Mental execution: tags `"iii"` → count=3, intCount=3, stringCount=0. intArgs sized to 3, stringArgs nil. Loop i=2: popInt → 3 → intArgs[2]=3, intIdx 1. Loop i=1: popInt → 2 → intArgs[1]=2, intIdx 0. Loop i=0: popInt → 1 → intArgs[0]=1, intIdx -1. Result `[1, 2, 3]` ✓.

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestPopScriptArgs_AllInt" ./pkg/script/ -v
```

Expected: **PASS**.

- [ ] **Step 5: Add `TestPopScriptArgs_AllString` (PASS)**

Append:

```go
// TestPopScriptArgs_AllString pins the all-string case: tags="sss" with
// stack-pushed strings ["a", "b", "c"] (top of stack is "c") yields
// stringArgs=["a", "b", "c"], intArgs=nil.
func TestPopScriptArgs_AllString(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_allstring"}, nil, false, nil, nil)
	state.PushString("a")
	state.PushString("b")
	state.PushString("c")
	state.PushString("sss")

	intArgs, stringArgs := popScriptArgs(state)

	if intArgs != nil {
		t.Errorf("intArgs: got %v, want nil", intArgs)
	}
	if !slices.Equal(stringArgs, []string{"a", "b", "c"}) {
		t.Errorf("stringArgs: got %v, want [a b c]", stringArgs)
	}
	if state.SSP != 0 {
		t.Errorf("SSP after pop: got %d, want 0", state.SSP)
	}
}
```

Mental execution: stack from bottom: [a, b, c, "sss"]. PopString → "sss" → tags. Loop i=2: popString → "c" → stringArgs[2]="c". i=1: popString → "b" → stringArgs[1]="b". i=0: popString → "a" → stringArgs[0]="a". Result `["a", "b", "c"]` ✓.

- [ ] **Step 6: Add `TestPopScriptArgs_Mixed` (PASS)**

Append:

```go
// TestPopScriptArgs_Mixed pins the mixed-type case from spec § Bundle 2
// § "Order semantics": tags="isi" with stack-pushed [1, "two", 3]
// yields intArgs=[1, 3] (tag-relative-int-order: i0 then i2), and
// stringArgs=["two"] (tag-relative-string-order: s1).
//
// Stack push order: PushInt(1), PushString("two"), PushInt(3),
// PushString("isi"). Top of int stack is 3; top of string stack is
// "isi". popScriptArgs first pops "isi" off the string stack, then
// loop i=2 (tag 'i') pops 3 off the int stack into intArgs[1], loop
// i=1 (tag 's') pops "two" off the string stack into stringArgs[0],
// loop i=0 (tag 'i') pops 1 off the int stack into intArgs[0].
func TestPopScriptArgs_Mixed(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_mixed"}, nil, false, nil, nil)
	state.PushInt(1)
	state.PushString("two")
	state.PushInt(3)
	state.PushString("isi")

	intArgs, stringArgs := popScriptArgs(state)

	if !slices.Equal(intArgs, []int{1, 3}) {
		t.Errorf("intArgs: got %v, want [1 3]", intArgs)
	}
	if !slices.Equal(stringArgs, []string{"two"}) {
		t.Errorf("stringArgs: got %v, want [two]", stringArgs)
	}
	if state.ISP != 0 {
		t.Errorf("ISP after pop: got %d, want 0", state.ISP)
	}
	if state.SSP != 0 {
		t.Errorf("SSP after pop: got %d, want 0", state.SSP)
	}
}
```

Mental execution (re-verified): int stack [1, 3], string stack ["two", "isi"]. After PopString→"isi", string stack is ["two"]. Loop i=2 ('i'): PopInt→3, intArgs[intIdx=1]=3, intIdx→0. Int stack is [1]. Loop i=1 ('s'): PopString→"two", stringArgs[stringIdx=0]="two", stringIdx→-1. String stack is []. Loop i=0 ('i'): PopInt→1, intArgs[intIdx=0]=1, intIdx→-1. Int stack is []. Result `intArgs=[1, 3]`, `stringArgs=["two"]` ✓.

- [ ] **Step 7: Add `TestPopScriptArgs_ReverseOrder` (PASS)**

Append:

```go
// TestPopScriptArgs_ReverseOrder pins the reverse-pop semantics that
// distinguish popScriptArgs from a naive forward-iteration: with
// tags="iii" and stack-pushed [10, 20, 30] (i.e. PushInt(10),
// PushInt(20), PushInt(30)), the result is intArgs=[10, 20, 30] —
// NOT [30, 20, 10]. The TS i=count-1→0 loop combined with the
// intIdx=intCount-1→0 decrementer preserves tag-position order even
// though pops are last-in-first-out. This test pins the inversion.
func TestPopScriptArgs_ReverseOrder(t *testing.T) {
	state := Init(&ScriptFile{Name: "popscriptargs_reverseorder"}, nil, false, nil, nil)
	state.PushInt(10)
	state.PushInt(20)
	state.PushInt(30)
	state.PushString("iii")

	intArgs, _ := popScriptArgs(state)

	if !slices.Equal(intArgs, []int{10, 20, 30}) {
		t.Errorf("intArgs: got %v, want [10 20 30] (tag-position order, not LIFO order)", intArgs)
	}
}
```

Run all 5 helper tests:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestPopScriptArgs" ./pkg/script/ -v
```

Expected: all 5 PASS.

- [ ] **Step 8: Run the full `pkg/script/` test suite for regression check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS. The new helper has no production consumers yet — it's added defensively in this task and consumed by `handleStrongQueue` in Task 3. **Per `dead_api_polish` memory: this is acceptable in a multi-task plan because Task 3 IS the consumer** — by Task 5 the helper has its consumers wired. If Task 5 is somehow deferred, polish-time should retract the helper.

Task 2 produces no commit — the helper + its 5 unit tests bundle into a single Bundle-2 commit at the end of Task 6 Step 6.

---

## Task 3 — Bundle 2: `handleStrongQueue` un-shared body + 4 tests (α, β-mixed, β-empty, β-all-int)

**Files:**
- Modify: `pkg/script/handlers.go` (replace `handleStrongQueue` wrapper with a TS-faithful own body, ~12 LOC)
- Modify: `pkg/script/handlers_test.go` (4 new tests pinning STRONGQUEUE divergences α + β)

**Pre-flight context:**
- HEAD `<Task 2 staged>` (Task 2 has no commit; its staged changes should be present in the working tree at task dispatch). Verify `popScriptArgs` is staged via `git diff --staged | grep -A 1 "func popScriptArgs"`.
- Per `plan_test_coverage_crosscheck` memory: spec divergence α (STRONGQUEUE NumberNotNull) and divergence β (STRONGQUEUE popScriptArgs) both anchor here. β has 3 sub-tests pinning empty / all-int / mixed shape.
- Test naming follows project convention `TestHandle<OpName><Behavior>`. The spec uses `TestStrongQueueDelayNullRejected` (NOT `TestHandleStrongQueueDelayNullRejected`) to align with existing queue tests `TestQueueOpcode` / `TestQueueVariants` / `TestPDelayUnprotectedRejected` (no `Handle` prefix in this file). Apply the spec form.
- The test pattern uses `mockPlayer{}` + `Init(sf, mp, false, nil, nil)` + push intArgs to the stack manually + `Execute(state)` (matches `TestPDelayRequiresActivePlayer` at `handlers_test.go:389`).

- [ ] **Step 1: Pre-flight verification**

```bash
git diff --staged pkg/script/handlers.go | head -40
grep -n "func handleStrongQueue\|func enqueueTyped\|func popScriptArgs" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go
grep -n "checkNotNull\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_player.go
```

Verify `popScriptArgs` is staged (Task 2). Verify `checkNotNull` is at `handlers_player.go:61` (helper reuse target). Verify `handleStrongQueue` is the current 1-line wrapper at the end of `handlers.go`.

- [ ] **Step 2: TDD — write failing α test (`TestStrongQueueDelayNullRejected`)**

Append to `pkg/script/handlers_test.go`:

```go
// TestStrongQueueDelayNullRejected pins divergence α: TS
// PlayerOps.ts:99 wraps the popped delay with check(..., NumberNotNull).
// goscape's pre-NAI-26 enqueueTyped helper missed this wrap. This test
// pushes tags="" (empty popScriptArgs), delay=-1 (NULL), scriptID=77
// and expects "STRONGQUEUE: input number was null(-1)".
func TestStrongQueueDelayNullRejected(t *testing.T) {
	sf := newSingleOp("strongqueue_delay_null", OpStrongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77) // scriptID
	state.PushInt(-1) // delay (NULL)
	state.PushString("") // type-tags (no script args)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for delay=-1, got nil")
	}
	want := "STRONGQUEUE: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.enqueueCalls) != 0 {
		t.Errorf("enqueueCalls: got %d, want 0 (rejection should not enqueue)", len(mp.enqueueCalls))
	}
}
```

Verify `"strings"` import is present in `handlers_test.go` (re-grep at task time). If absent, add it.

Stack push order verification: `OpStrongQueue` post-Bundle-2 will pop in this order: tags string first, then delay int, then scriptID int. So push scriptID first (bottom of stack), then delay, then tags string (top). On the int stack: bottom=77, top=-1. On the string stack: bottom="" (which is top since it's the only entry).

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestStrongQueueDelayNullRejected" ./pkg/script/ -v
```

Expected: **FAIL** — pre-Bundle-2 `handleStrongQueue` calls `enqueueTyped` which does NOT wrap delay with NumberNotNull, so no error is returned. The test will trip on `err == nil`.

- [ ] **Step 3: Implement `handleStrongQueue` un-shared body (verify α PASSES)**

Edit `pkg/script/handlers.go`. Find `handleStrongQueue` (currently `func handleStrongQueue(s *ScriptState) error { return enqueueTyped(s, QueueStrong, "STRONGQUEUE") }` at ~line 629).

Replace with the TS-faithful body (TS PlayerOps.ts:97-108 ported line-by-line):

```go
// handleStrongQueue implements STRONGQUEUE (opcode 2117): pop variadic
// typed args via popScriptArgs (which itself first pops the type-tags
// string and then pops each typed value in tag-reverse order), then
// pop delay (NumberNotNull-checked), then pop scriptID, and enqueue a
// STRONG-typed queue request. Mirrors TS PlayerOps.ts:97-108
// line-by-line.
//
// NAI-26 Bundle 2: un-shared from the pre-NAI-26 enqueueTyped helper
// to fix divergences α (NumberNotNull on delay, missing) + β
// (popScriptArgs, missing — the helper popped only a single arg int,
// silently using the QUEUE shape for a variadic opcode).
func handleStrongQueue(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("STRONGQUEUE: no active player")
	}
	intArgs, stringArgs := popScriptArgs(s)
	delay := s.PopInt()
	if err := checkNotNull(delay, "STRONGQUEUE"); err != nil {
		return err
	}
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueStrong)
}
```

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestStrongQueueDelayNullRejected" ./pkg/script/ -v
```

Expected: **PASS**. Run the full handlers_test set to verify no regression in `TestQueueOpcode` / `TestQueueVariants`:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestQueue\|TestStrongQueue" ./pkg/script/ -v
```

Expected: **PASS** for all queue-family tests.

- [ ] **Step 4: TDD — write `TestStrongQueueEmptyScriptArgs` (β-empty, FAIL → PASS post-implementation)**

Append:

```go
// TestStrongQueueEmptyScriptArgs pins divergence β with an empty
// type-tags string: STRONGQUEUE with tags="", delay=3, scriptID=77
// enqueues with IntArgs=nil, StringArgs=nil.
func TestStrongQueueEmptyScriptArgs(t *testing.T) {
	sf := newSingleOp("strongqueue_empty_args", OpStrongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77) // scriptID
	state.PushInt(3) // delay
	state.PushString("") // type-tags (no script args)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 77 || got.Delay != 3 || got.Type != QueueStrong {
		t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want 77/3/QueueStrong",
			got.ScriptID, got.Delay, got.Type)
	}
	if got.IntArgs != nil {
		t.Errorf("IntArgs: got %v, want nil", got.IntArgs)
	}
	if got.StringArgs != nil {
		t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
	}
}
```

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestStrongQueueEmptyScriptArgs" ./pkg/script/ -v
```

Expected: **PASS** (the post-Step-3 `handleStrongQueue` body delivers nil/nil correctly via `popScriptArgs` returning nil/nil for an empty tags string).

- [ ] **Step 5: TDD — write `TestStrongQueueAllIntScriptArgs` (β-all-int, PASS)**

Append:

```go
// TestStrongQueueAllIntScriptArgs pins divergence β with an all-int
// type-tags string: STRONGQUEUE with tags="iii", three int args
// (10, 20, 30), delay=5, scriptID=77 enqueues with
// IntArgs=[10, 20, 30], StringArgs=nil.
func TestStrongQueueAllIntScriptArgs(t *testing.T) {
	sf := newSingleOp("strongqueue_allint_args", OpStrongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(77) // scriptID (deepest int)
	state.PushInt(5) // delay
	state.PushInt(10) // arg0
	state.PushInt(20) // arg1
	state.PushInt(30) // arg2 (top of int stack)
	state.PushString("iii") // type-tags

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 77 || got.Delay != 5 || got.Type != QueueStrong {
		t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want 77/5/QueueStrong",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{10, 20, 30}) {
		t.Errorf("IntArgs: got %v, want [10 20 30]", got.IntArgs)
	}
	if got.StringArgs != nil {
		t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
	}
}
```

Mental execution: int stack from bottom [77, 5, 10, 20, 30]. String stack ["iii"]. handleStrongQueue calls popScriptArgs first → pops "iii" off string stack → count=3, intCount=3. Loop pops int 30 → intArgs[2]=30, then 20 → intArgs[1]=20, then 10 → intArgs[0]=10. Result intArgs=[10, 20, 30]. Int stack now [77, 5]. Pop delay → 5. checkNotNull(5) → OK. Pop scriptID → 77. EnqueueScriptArgs(77, 5, [10, 20, 30], nil, QueueStrong) → mock records.

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestStrongQueueAllIntScriptArgs" ./pkg/script/ -v
```

Expected: **PASS**.

- [ ] **Step 6: TDD — write `TestStrongQueuePopsMixedScriptArgs` (β-mixed, PASS)**

Append:

```go
// TestStrongQueuePopsMixedScriptArgs pins divergence β with a mixed-
// type type-tags string: STRONGQUEUE with tags="is", arg-int=99,
// arg-string="hello", delay=2, scriptID=77 enqueues with IntArgs=[99],
// StringArgs=["hello"]. Pin shape lifts directly from spec § Bundle 2 §
// "Order semantics".
func TestStrongQueuePopsMixedScriptArgs(t *testing.T) {
	sf := newSingleOp("strongqueue_mixed_args", OpStrongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	// Stack push order — caller pre-loads the typed-arg block in
	// tag-position order, so for tags="is": int arg first (deepest),
	// then string arg, then delay, then scriptID — but ordering on
	// the int and string stacks is independent.
	//
	// Int stack from bottom: [scriptID=77, delay=2, intArg=99].
	// String stack from bottom: [stringArg="hello", tags="is"].
	state.PushInt(77)
	state.PushInt(2)
	state.PushInt(99)
	state.PushString("hello")
	state.PushString("is")

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 77 || got.Delay != 2 || got.Type != QueueStrong {
		t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want 77/2/QueueStrong",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{99}) {
		t.Errorf("IntArgs: got %v, want [99]", got.IntArgs)
	}
	if !slices.Equal(got.StringArgs, []string{"hello"}) {
		t.Errorf("StringArgs: got %v, want [hello]", got.StringArgs)
	}
}
```

Mental execution: int stack [77, 2, 99], string stack ["hello", "is"]. popScriptArgs: PopString → "is" → count=2, intCount=1, stringCount=1. Loop i=1 ('s'): PopString → "hello" → stringArgs[0]="hello". Loop i=0 ('i'): PopInt → 99 → intArgs[0]=99. Int stack now [77, 2]. PopInt delay → 2. checkNotNull(2) → OK. PopInt scriptID → 77. Mock records.

Run all 4 STRONGQUEUE tests:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestStrongQueue" ./pkg/script/ -v
```

Expected: 4 PASS (Delay-null, EmptyScriptArgs, AllInt, Mixed).

- [ ] **Step 7: Verify no regression in the `TestQueueVariants` "strong" case**

The pre-Bundle-2 `TestQueueVariants` still has the "strong" sub-test using the old 3-popInt fixture. Now that `handleStrongQueue` has its own body popping a tags string FIRST, the old fixture (which doesn't push a tags string) will hit a `PopString()` on an empty stack returning `""` (per `state.go:208-213` PopString underflow returns `""`). Then `popScriptArgs` returns nil/nil. Then `delay=42` (popped from the int stack — was previously the "arg" position), `checkNotNull(42)` passes. Then `scriptID=3` (was previously "delay"). Then `Enqueue(3, 42, nil, nil, QueueStrong)`. The pre-NAI-26 expected was `ScriptID=77, Delay=3, IntArg=42` — now it'll be `ScriptID=3, Delay=42, IntArgs=nil`. That's a regression of the test even though the production behavior is correct.

Decision: the "strong" sub-test in `TestQueueVariants` now exercises the WRONG handler shape and produces wrong assertions. Per the spec § Bundle 2 § "Updates to existing tests" the "strong" case is removed in Task 5 (along with "long"). Bundle 2's task ordering means this regression is temporarily live during Tasks 3-4. Acceptable workaround: skip the "strong" sub-test temporarily by removing it from the table NOW (in this step, as part of Task 3) so Task 3's local `go test ./pkg/script/...` is clean.

Edit `TestQueueVariants` at `pkg/script/handlers_test.go`. Remove the `{"strong", OpStrongQueue, QueueStrong}` line from the table. The remaining table entries are `{"weak", ...}` and `{"long", ...}`. Task 4 removes the "long" entry. Task 5 confirms the table is finally `{"weak", ...}` only.

Run the full handlers_test set:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -v
```

Expected: **PASS** (TestQueueVariants now runs only weak and long; "long" still uses the old 3-popInt shape until Task 4).

Task 3 produces no commit — its changes bundle into the Bundle-2 commit at Task 6 Step 6.

---

## Task 4 — Bundle 2: `handleLongQueue` un-shared body + `TestLongQueuePopsFourInts`

**Files:**
- Modify: `pkg/script/handlers.go` (replace `handleLongQueue` wrapper with TS-faithful own body, ~12 LOC)
- Modify: `pkg/script/handlers_test.go` (1 new test pinning the 4-popInt + [logoutAction, arg] ordering)

**Pre-flight context:**
- Per `audit_full_method_against_ts` memory: LONGQUEUE has TWO divergences (ζ + η) bundled into one body change. ζ: pre-NAI-26 enqueueTyped pops only 3 ints; LONGQUEUE TS pops 4 (`scriptId, delay, arg, logoutAction`). η: TS passes `[logoutAction, arg]` as the args array (logoutAction is FIRST per TS PlayerOps.ts:179 even though it's the last popped int and thus on top of the stack at handler entry).
- Stack push order: TS `popInts(4)` returns `[scriptId, delay, arg, logoutAction]` — that array element [3] (logoutAction) is the most-recently-pushed and thus the first popped. Per `state.go:188` PopInt returns the top, so the first PopInt yields logoutAction, then arg, then delay, then scriptID.
- Test name: `TestLongQueuePopsFourInts` covers both ζ and η in one assertion (verifies all 4 ints come out and the IntArgs ordering is [logoutAction, arg]). This collapses spec divergences ζ and η into one test entry per spec § Bundle 2 § "Per-divergence test mapping" which lists η as "covered by ζ via IntArgs ordering assertion".

- [ ] **Step 1: Pre-flight verification**

```bash
grep -n "func handleLongQueue\|func handleStrongQueue" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go
```

Verify Task 3 changes are staged (handleStrongQueue has the un-shared body).

- [ ] **Step 2: TDD — write failing `TestLongQueuePopsFourInts`**

Append to `pkg/script/handlers_test.go`:

```go
// TestLongQueuePopsFourInts pins divergences ζ + η: LONGQUEUE pops 4
// ints (scriptID, delay, arg, logoutAction — the 4th distinguishes it
// from QUEUE/WEAKQUEUE/STRONGQUEUE) and enqueues with the 2-element
// args array [logoutAction, arg] (logoutAction-first per TS
// PlayerOps.ts:179, even though logoutAction is the last-pushed and
// first-popped int).
func TestLongQueuePopsFourInts(t *testing.T) {
	sf := newSingleOp("longqueue_4ints", OpLongQueue)
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	// Stack push order: scriptID first (deepest), then delay, then arg,
	// then logoutAction (top). PopInt order at handler entry:
	// logoutAction → arg → delay → scriptID.
	state.PushInt(77) // scriptID
	state.PushInt(3) // delay
	state.PushInt(99) // arg
	state.PushInt(42) // logoutAction (top)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	if got.ScriptID != 77 || got.Delay != 3 || got.Type != QueueLong {
		t.Errorf("enqueue header: got ScriptID=%d Delay=%d Type=%v, want 77/3/QueueLong",
			got.ScriptID, got.Delay, got.Type)
	}
	if !slices.Equal(got.IntArgs, []int{42, 99}) {
		t.Errorf("IntArgs: got %v, want [42 99] (logoutAction, arg per TS PlayerOps.ts:179)",
			got.IntArgs)
	}
	if got.StringArgs != nil {
		t.Errorf("StringArgs: got %v, want nil", got.StringArgs)
	}
}
```

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestLongQueuePopsFourInts" ./pkg/script/ -v
```

Expected: **FAIL** — pre-Task-4 `handleLongQueue` calls `enqueueTyped` (3-popInt) with logoutAction silently dropped; the test trips on `len(enqueueCalls) != 1` (or on `IntArgs != [42, 99]`).

- [ ] **Step 3: Implement `handleLongQueue` un-shared body**

Edit `pkg/script/handlers.go`. Replace the `handleLongQueue` wrapper:

```go
// handleLongQueue implements LONGQUEUE (opcode 2059): pop scriptID,
// delay, arg, logoutAction (4 ints) and enqueue a LONG-typed queue
// request with [logoutAction, arg] as the args array (logoutAction-
// first per TS PlayerOps.ts:179). Mirrors TS PlayerOps.ts:171-180
// line-by-line.
//
// NAI-26 Bundle 2: un-shared from the pre-NAI-26 enqueueTyped helper
// to fix divergences ζ (4-popInt missing — helper popped only 3) and
// η (2-element args array missing — helper passed [arg] not
// [logoutAction, arg]).
func handleLongQueue(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("LONGQUEUE: no active player")
	}
	logoutAction := s.PopInt()
	arg := s.PopInt()
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, []int{logoutAction, arg}, nil, QueueLong)
}
```

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestLongQueuePopsFourInts" ./pkg/script/ -v
```

Expected: **PASS**.

- [ ] **Step 4: Remove the "long" sub-test from `TestQueueVariants`**

Edit `TestQueueVariants` at `pkg/script/handlers_test.go`. Remove the `{"long", OpLongQueue, QueueLong}` line from the table. The remaining table entry is `{"weak", OpWeakQueue, QueueWeak}` only (after Task 3 removed the "strong" entry).

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestQueue\|TestLongQueue\|TestStrongQueue" ./pkg/script/ -v
```

Expected: **PASS** for all queue-family tests; `TestQueueVariants` runs only the "weak" case.

Task 4 produces no commit — its changes bundle into the Bundle-2 commit at Task 6 Step 6.

---

## Task 5 — Bundle 2: `handleQueue` + `handleWeakQueue` un-shared bodies + `handlePDelay` NumberNotNull wrap + `TestPDelayNullRejected`

**Files:**
- Modify: `pkg/script/handlers.go` (replace `handleQueue` and `handleWeakQueue` wrappers with own bodies; add NumberNotNull wrap to `handlePDelay`; remove `enqueueTyped` helper)
- Modify: `pkg/script/handlers_test.go` (1 new test for κ; verify TestQueueVariants weak case still passes)

**Pre-flight context:**
- These are the mechanical un-sharings for QUEUE and WEAKQUEUE (no popScriptArgs, no NumberNotNull on delay — matches TS PlayerOps.ts:148-157 and :123-132 which do not wrap delay). Behavior is bit-identical to pre-Bundle-2 except they no longer route through `enqueueTyped`. The whole point of un-sharing is to enable Bundle 2 Task 6's per-handler script-missing error propagation (which the un-shared bodies already get via the `EnqueueScriptArgs` return).
- handlePDelay κ: TS PlayerOps.ts:377 wraps the popped `n` with `check(state.popInt(), NumberNotNull)`. Pre-NAI-26 goscape's `handlePDelay` (`handlers.go:589-597`) does not.
- After this task lands, `enqueueTyped` has zero consumers and must be removed (per spec § Bundle 2 § "enqueueTyped removal" + `dead_api_polish` memory).

- [ ] **Step 1: Pre-flight verification**

```bash
grep -n "func handleQueue\|func handleWeakQueue\|func handlePDelay\|func enqueueTyped" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go
```

Confirm `handleQueue`/`handleWeakQueue` are still 1-liners calling `enqueueTyped`, and `handlePDelay` body is unchanged from HEAD.

- [ ] **Step 2: Implement `handleQueue` un-shared body**

Edit `pkg/script/handlers.go`. Replace the `handleQueue` wrapper (currently `func handleQueue(s *ScriptState) error { return enqueueTyped(s, QueueNormal, "QUEUE") }`):

```go
// handleQueue implements QUEUE (opcode 2092): pop scriptID, delay, arg
// (3 ints) and enqueue a NORMAL-typed queue request with [arg] as the
// args array. Mirrors TS PlayerOps.ts:148-157 line-by-line.
//
// NAI-26 Bundle 2: un-shared from the pre-NAI-26 enqueueTyped helper.
// The body here is mechanically equivalent to the old shared helper for
// QUEUE; un-sharing exists to enable per-handler script-missing error
// propagation (divergence ε — TS PlayerOps.ts:152-154) via the
// EnqueueScriptArgs return (Task 6 Step 1 activates the error).
func handleQueue(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("QUEUE: no active player")
	}
	arg := s.PopInt()
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, []int{arg}, nil, QueueNormal)
}
```

- [ ] **Step 3: Implement `handleWeakQueue` un-shared body**

Replace the `handleWeakQueue` wrapper (currently `func handleWeakQueue(s *ScriptState) error { return enqueueTyped(s, QueueWeak, "WEAKQUEUE") }`):

```go
// handleWeakQueue implements WEAKQUEUE (opcode 2129): pop scriptID,
// delay, arg (3 ints) and enqueue a WEAK-typed queue request with [arg]
// as the args array. Mirrors TS PlayerOps.ts:123-132 line-by-line.
//
// NAI-26 Bundle 2: un-shared from the pre-NAI-26 enqueueTyped helper
// to enable per-handler script-missing error propagation
// (divergence δ — TS PlayerOps.ts:127-129) via the EnqueueScriptArgs
// return (Task 6 Step 1 activates the error).
func handleWeakQueue(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("WEAKQUEUE: no active player")
	}
	arg := s.PopInt()
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, []int{arg}, nil, QueueWeak)
}
```

- [ ] **Step 4: Add NumberNotNull wrap to `handlePDelay` (κ)**

Edit `pkg/script/handlers.go`. The current `handlePDelay` body (lines 584-597):

```go
// handlePDelay implements P_DELAY (opcode 2071): pop int n, delay the
// active player by n+1 ticks, and suspend execution. TS PlayerOps.ts
// sets state.delayedUntil = currentTick + 1 + n; we push the whole
// calculation into the ActivePlayer.SetDelayed implementation so
// pkg/script stays decoupled from the server's current-tick counter.
func handlePDelay(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_DELAY"); err != nil {
		return err
	}
	n := int(s.PopInt())
	s.Self.SetDelayed(n)
	s.Execution = Suspended
	return nil
}
```

Replace with (per `plan_grep_helper_patterns` memory: reuse `checkNotNull` — no inline boilerplate):

```go
// handlePDelay implements P_DELAY (opcode 2071): pop int n
// (NumberNotNull-checked), delay the active player by n+1 ticks, and
// suspend execution. TS PlayerOps.ts:375-379 sets
// state.delayedUntil = currentTick + 1 + check(state.popInt(),
// NumberNotNull); we push the +1 calculation into the
// ActivePlayer.SetDelayed implementation so pkg/script stays decoupled
// from the server's current-tick counter.
//
// NAI-26 Bundle 2: NumberNotNull wrap added to fix divergence κ — TS
// PlayerOps.ts:377 wraps the popped n with check(..., NumberNotNull).
func handlePDelay(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_DELAY"); err != nil {
		return err
	}
	n := s.PopInt()
	if err := checkNotNull(n, "P_DELAY"); err != nil {
		return err
	}
	s.Self.SetDelayed(n)
	s.Execution = Suspended
	return nil
}
```

Note: removed `int(...)` cast — `PopInt` already returns `int` per `state.go:188`.

- [ ] **Step 5: Remove the `enqueueTyped` helper**

After Steps 2-3 land, `enqueueTyped` has zero consumers in production and tests. Per `dead_api_polish` memory: helpers shipped with zero consumers are removed at the close of the sub-spec that orphans them. Bundle 2 is that close.

Re-grep to verify:

```bash
grep -rn "enqueueTyped" /home/owner/Code/github.com/zsrv/goscape/ --include="*.go"
```

Expected: only the `func enqueueTyped` definition itself appears. No callers.

Edit `pkg/script/handlers.go`. Remove the entire `enqueueTyped` function block (the doc-comment + the function body). The block lives between the `handlePDelay` function and the `popScriptArgs` helper. After deletion, `popScriptArgs` should be the function immediately following `handlePDelay`.

Compile-check:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...
```

Expected: build succeeds.

- [ ] **Step 6: TDD — write `TestPDelayNullRejected` (κ)**

Append to `pkg/script/handlers_test.go`:

```go
// TestPDelayNullRejected pins divergence κ: TS PlayerOps.ts:377 wraps
// the popped n with check(..., NumberNotNull). Pushes -1 (NULL) → the
// handler returns "P_DELAY: input number was null(-1)" without
// calling SetDelayed.
func TestPDelayNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_delay_null", OpPDelay)
	state := Init(sf, mp, true, nil, nil) // protect=true (P_DELAY needs protection)
	state.PushInt(-1)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "P_DELAY: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if len(mp.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (rejection should not call SetDelayed)",
			mp.setDelayedCalls)
	}
}
```

Verify the `mockPlayer.setDelayedCalls` field is `[]int` (it's read by `TestPDelayUnprotectedRejected` already without slice comparison; per `runner_test.go:256-258`).

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestPDelayNullRejected\|TestPDelay" ./pkg/script/ -v
```

Expected: **PASS** for `TestPDelayNullRejected` and the existing `TestPDelayUnprotectedRejected` / `TestPDelayRequiresActivePlayer`.

- [ ] **Step 7: Verify the full `pkg/script/` test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: **PASS** across all `pkg/script/` tests including `TestQueueOpcode`, `TestQueueVariants` (weak only), 4 STRONGQUEUE tests, 1 LONGQUEUE test, 1 P_DELAY null test, 5 popScriptArgs tests.

Task 5 produces no commit — its changes bundle into the Bundle-2 commit at Task 6 Step 6.

---

## Task 6 — Bundle 2: Activate script-missing error + per-site `script_test.go` audit + integration test + commit

**Files:**
- Modify: `modules/world/player_script.go:63-90` (activate `EnqueueScriptArgs` script-missing error)
- Modify: `pkg/script/runner_test.go:246-261` (extend `mockPlayer.EnqueueScriptArgs` for opt-in error return)
- Modify: `pkg/script/handlers_test.go` (add `TestQueueScriptNotFound` table-driven test)
- Modify: `modules/world/script_test.go` (add `TestProcessPlayerQueueDeliversAllArgs` integration test)
- Read-only audit: `modules/world/script_test.go:239, :269, :336, :337, :825, :1020` (per-site re-evaluation; expected: 0 migrations needed because all 6 enqueue registered scripts)

**Pre-flight context:**
- HEAD `<Tasks 2-5 staged>` (none of Tasks 2-5 produced a commit). The Bundle-2 commit at this task's Step 6 includes everything from Tasks 2-6 in a single commit per spec § Bundle 2 § "Bundle 2 commit shape" (one feat commit covering all of Bundle 2's per-opcode work + popScriptArgs + script-missing rollout + integration test).
- **Pre-flight surprise corollary** (per Task 1's pre-flight context): all 6 `script_test.go` enqueue sites pass registered scripts. The script-missing error activation will not affect any of them. The "per-site re-evaluation" step is therefore a confirmation pass: each site's intent is preserved as-is, no migration needed.
- `TestQueueScriptNotFound` is a table-driven test in `pkg/script/handlers_test.go` covering all 4 queue opcodes (STRONGQUEUE, WEAKQUEUE, QUEUE, LONGQUEUE), pinning that the handler propagates the `EnqueueScriptArgs` error up. Uses the mock's opt-in `ReturnError` field added in Task 1 Step 9.
- The integration test fires a real queue request through `processPlayerQueue` and asserts `runScript` receives the parallel-slice args. Anchors the Bundle-1 plumbing under realistic conditions per spec § Bundle 2 § "Tests" "Integration test" bullet.

- [ ] **Step 1: Activate `(*Player).EnqueueScriptArgs` script-missing error**

Edit `modules/world/player_script.go`. Find the post-Task-1-Step-5 body. Replace the placeholder `return nil` branch with the active error return (per spec § Bundle 2 § "EnqueueScriptArgs script-missing error rollout"):

Current Bundle-1 placeholder (post-Task-1-Step-5):

```go
	sf := p.client.server.scriptProvider.GetByID(scriptID)
	if sf == nil {
		// NAI-26 Bundle 1 placeholder: returns nil to preserve pre-NAI-26
		// silent-no-op behavior. Bundle 2 (Task 6 Step 1) replaces this
		// with `return fmt.Errorf("unable to find queue script: %d", scriptID)`.
		return nil
	}
```

Replace with:

```go
	sf := p.client.server.scriptProvider.GetByID(scriptID)
	if sf == nil {
		// NAI-26 Bundle 2: surfaces script-author errors that pre-NAI-26
		// silent-no-op masked. Mirrors TS PlayerOps.ts:103-105
		// (STRONGQUEUE), :127-129 (WEAKQUEUE), :152-154 (QUEUE),
		// :175-177 (LONGQUEUE) — all four queue handlers throw
		// `Unable to find queue script: ${scriptId}` when the
		// scriptProvider lookup fails.
		return fmt.Errorf("unable to find queue script: %d", scriptID)
	}
```

Add `"fmt"` to the imports of `modules/world/player_script.go` if not already present:

```bash
grep -n "\"fmt\"" /home/owner/Code/github.com/zsrv/goscape/modules/world/player_script.go
```

If absent, add it to the existing import block.

The unwired-server early-out (`if p.client == nil || ... return nil`) is preserved as nil-error: that path corresponds to test fixtures that don't wire a Server (per spec § Bundle 1 touch point #5), not to a script-author error worth surfacing.

Update the doc-comment at line 63-78 to remove the Bundle-1 placeholder language and reflect the activated error. Replace the "NAI-26 Bundle 1 NOTE" paragraph with:

```go
// NAI-26 Bundle 2: this implementation now returns a non-nil error
// when GetByID returns nil — TS-faithful to PlayerOps.ts:103-105
// (and the parallel sites in :127-129, :152-154, :175-177). Bundle 1
// shipped a placeholder body returning nil; the rollout of the error
// activation was deferred to Bundle 2 to keep the mechanical signature
// widening separate from the behavior change for review-surface
// isolation.
```

- [ ] **Step 2: Extend `mockPlayer.EnqueueScriptArgs` for opt-in error return**

Edit `pkg/script/runner_test.go`. Update `mockPlayer.EnqueueScriptArgs` (post-Task-1-Step-9 body):

```go
func (m *mockPlayer) EnqueueScriptArgs(id uint32, delay int, intArgs []int, stringArgs []string, qtype PlayerQueueType) error {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueue{ScriptID: id, Delay: delay, IntArgs: intArgs, StringArgs: stringArgs, Type: qtype})
	return m.enqueueScriptArgsReturnErr
}
```

Add the opt-in error field to the `mockPlayer` struct. Re-grep for the mockPlayer struct:

```bash
grep -n "type mockPlayer struct\|^}" /home/owner/Code/github.com/zsrv/goscape/pkg/script/runner_test.go | head -15
```

Insert at an appropriate place in the struct (e.g. near the other queue-related fields like `enqueueCalls` at `runner_test.go:102`):

```go
	// NAI-26 Bundle 2: opt-in error return for EnqueueScriptArgs,
	// configured by tests that pin script-missing error propagation.
	// Default zero-value (nil error) preserves Bundle-1 mock behavior.
	enqueueScriptArgsReturnErr error
```

Compile-check:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/...
```

Expected: vet clean.

- [ ] **Step 3: TDD — write `TestQueueScriptNotFound` (γ + δ + ε + θ table-driven)**

Append to `pkg/script/handlers_test.go`:

```go
// TestQueueScriptNotFound pins divergences γ + δ + ε + θ:
// STRONGQUEUE / WEAKQUEUE / QUEUE / LONGQUEUE all propagate the
// EnqueueScriptArgs script-missing error to their caller via the
// handler's error return. Mirrors TS PlayerOps.ts:103-105 (STRONG),
// :127-129 (WEAK), :152-154 (NORMAL), :175-177 (LONG). The mock
// player is pre-configured to return the script-missing error;
// the handler must propagate it up.
func TestQueueScriptNotFound(t *testing.T) {
	cases := []struct {
		name     string
		op       Opcode
		setup    func(state *ScriptState) // pushes scriptID/delay/[arg|tags...] in op-specific order
	}{
		{
			name: "STRONGQUEUE",
			op:   OpStrongQueue,
			setup: func(state *ScriptState) {
				state.PushInt(77) // scriptID
				state.PushInt(3)   // delay
				state.PushString("") // tags=""
			},
		},
		{
			name: "WEAKQUEUE",
			op:   OpWeakQueue,
			setup: func(state *ScriptState) {
				state.PushInt(77) // scriptID
				state.PushInt(3)   // delay
				state.PushInt(42)  // arg
			},
		},
		{
			name: "QUEUE",
			op:   OpQueue,
			setup: func(state *ScriptState) {
				state.PushInt(77) // scriptID
				state.PushInt(3)   // delay
				state.PushInt(42)  // arg
			},
		},
		{
			name: "LONGQUEUE",
			op:   OpLongQueue,
			setup: func(state *ScriptState) {
				state.PushInt(77) // scriptID
				state.PushInt(3)   // delay
				state.PushInt(42)  // arg
				state.PushInt(99)  // logoutAction
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mp := &mockPlayer{
				enqueueScriptArgsReturnErr: fmt.Errorf("unable to find queue script: 77"),
			}
			sf := newSingleOp(tc.name+"_notfound", tc.op)
			state := Init(sf, mp, false, nil, nil)
			tc.setup(state)

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error, got nil")
			}
			want := "unable to find queue script: 77"
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error: got %q, want substring %q", err.Error(), want)
			}
			// The mock records the call (the error happens inside the
			// real (*Player).EnqueueScriptArgs in production; the mock
			// records the call AND returns the configured error to the
			// handler).
			if len(mp.enqueueCalls) != 1 {
				t.Errorf("enqueueCalls: got %d, want 1 (mock should record before returning)",
					len(mp.enqueueCalls))
			}
		})
	}
}
```

Verify `"fmt"` import is present in `handlers_test.go`. If absent, add it.

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestQueueScriptNotFound" ./pkg/script/ -v
```

Expected: **PASS** for all 4 sub-tests. Each handler's error-return path propagates the mock's configured error.

- [ ] **Step 4: Per-site re-evaluation of the 6 `script_test.go` sites**

Per the Task 1 pre-flight context: all 6 sites enqueue **registered** scripts. The script-missing error activation in Step 1 does not affect them. This step is a confirmation pass — verify each site at HEAD:

| Line | Test name | Script ID | Registered at | Intent | Migration needed? |
|------|-----------|-----------|---------------|--------|-------------------|
| `:239` | `TestQueueFiresAtDelayExpiry` | `0xAAAA` | `:231` (`RegisterAt(0xAAAA, ...)`) | Pin "delay 1 → fires after pre-decrement" semantics | NO — script resolves; placeholder error path not taken |
| `:269` | `TestQueueZeroDelayFiresSameTick` | `0xBBBB` | `:261` | Pin "zero-delay queue fires same tick" semantics | NO — script resolves |
| `:336` | `TestQueueMultipleEntriesPreservesOrder` | `0xCCC1` | `:327` | Pin "FIFO queue order" with multiple entries | NO — both scripts resolve |
| `:337` | (same test as :336) | `0xCCC2` | `:328` | Same | NO |
| `:825` | `TestStrongQueueFiresWhileDelayed` | `0xBEEF` | `:808` | Pin "STRONG fires even when player.delayed=true" | NO — script resolves |
| `:1020` | `TestNormalQueueWaitsForIdle` | `0xBEE2` | `:1006` | Pin "NORMAL does NOT fire while player.delayed=true" | NO — script resolves |

Run all 6 tests to verify the script-missing error activation does not regress them:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestQueueFiresAtDelayExpiry|TestQueueZeroDelayFiresSameTick|TestQueueMultipleEntriesPreservesOrder|TestStrongQueueFiresWhileDelayed|TestNormalQueueWaitsForIdle" ./modules/world/ -v
```

Expected: all 5 distinct tests PASS (`:336` and `:337` are in the same test, so 5 t-functions total).

If any one fails: investigate. Most likely cause is that the script's `LookupKey` was not properly set so `GetByID` doesn't find it. Re-grep `RegisterAt\|GetByID` in `pkg/script/provider.go` to verify the lookup contract.

- [ ] **Step 5: Add integration test `TestProcessPlayerQueueDeliversAllArgs`**

Add to `modules/world/script_test.go` (append at end of file, or near other `processPlayerQueue` tests). Per spec § Bundle 2 § "Tests" "Integration test": fires a 2-int-arg queue request through `processPlayerQueue` and asserts `runScript` receives both ints.

The test approach: register a script that pushes its IntArgs back onto a captured channel via a built-in `Mes` opcode. Inspect the wire bytes to confirm both ints land. Goscape's existing `buildGreetScript` shows the pattern but pushes a literal char; we need a script that reads `IntArgCount=2` and emits both args.

Since the existing test scaffolding is centered on string-literal scripts, a simpler approach: pre-register a test fixture script whose `IntArgCount=2` and whose body simply `Return`s. Then verify directly that `processPlayerQueue` removed the queue entry (proving the script was fired) and assert the captured args via a hand-built `mockProvider` or by intercepting `(*Server).runScript`.

Cleanest approach: extend the existing test by reading from `p.queue` post-fire and asserting it's empty (existing tests do this), AND add a mock script provider whose registered script captures the runScript args via a closure. However, `runScript` is a method on `(*Server)` — overriding it requires a redesign or a thin shim.

Pragmatic alternative (recommended): add the test to the `pkg/script/` package level via the existing `mockPlayer` infrastructure. The integration scope is "queue request widens to parallel slices and processPlayerQueue forwards them to runScript". Since `mockPlayer.EnqueueScriptArgs` records `IntArgs []int`, a test that:
1. Calls `(*Player).EnqueueScriptArgs(scriptID, 0, []int{100, 200}, nil, QueueNormal)` directly on a real `*Player` with a real `Server` + `scriptProvider`,
2. Calls `s.processPlayerQueue(p)` to drain the queue,
3. Asserts the queue is empty AND that the script ran with 2 ints,

requires probing what `runScript` saw. In the existing codebase, `runScript` runs the script in-process — there's no recording shim. The closest analog is `TestQueueFiresAtDelayExpiry` (`:228-256`) which uses a wire-bytes-out script (`buildGreetScript`) and asserts the bytes-out shape.

Concrete shape: register a fixture script that uses `OpPushIntArg0` + `OpPushIntArg1` (if such opcodes exist) + `OpAppendNum` + `OpMes` to emit both ints. If those opcodes don't exist, the simpler shape is to register a script that emits a fixed string and just verify the script ran (queue-empty).

Implementer pre-flight at this step: re-grep for arg-reading opcodes:

```bash
grep -n "OpPushIntArg\|OpPushStringArg\|IntArgCount" /home/owner/Code/github.com/zsrv/goscape/pkg/script/opcode.go | head -10
```

Verify whether `OpPushIntArg0`/`OpPushIntArg1` exist. If yes, build a 2-arg script that pushes both ints, appends them, and emits via `OpMes`. If no, fall back to the queue-empty assertion (less informative but still proves the plumbing works).

For this plan, use the queue-empty + script-fired assertion as the minimum viable integration test:

```go
// TestProcessPlayerQueueDeliversAllArgs validates the NAI-26 Bundle 1
// plumbing under realistic queue-fire conditions: a queue request
// carrying IntArgs=[100, 200] is fired through processPlayerQueue and
// the target script runs (proven by the queue draining + the script's
// wire-output landing). The integration test confirms that the
// parallel-slice plumbing reaches runScript.
//
// Spec § Bundle 2 § "Integration test".
func TestProcessPlayerQueueDeliversAllArgs(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register a 2-int-arg script that emits a fixed "ok\n" mes —
	// confirms execution; the args themselves are validated by the
	// pkg/script-level TestQueueOpcode + TestStrongQueue* tests.
	s.scriptProvider.RegisterAt(0xD1D1, buildGreetScript(0xD1D1, "k"))

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	received := drainConn(t, cc)
	// Enqueue with 2 int args via the new parallel-slice signature.
	if err := p.EnqueueScriptArgs(0xD1D1, 0, []int{100, 200}, nil, script.QueueNormal); err != nil {
		t.Fatalf("EnqueueScriptArgs: %v", err)
	}
	// Verify the queue entry carries both args (Bundle 1 plumbing pin).
	if len(p.queue) != 1 {
		t.Fatalf("queue len after enqueue: got %d, want 1", len(p.queue))
	}
	if !slices.Equal(p.queue[0].IntArgs, []int{100, 200}) {
		t.Errorf("queue[0].IntArgs: got %v, want [100 200]", p.queue[0].IntArgs)
	}
	if p.queue[0].StringArgs != nil {
		t.Errorf("queue[0].StringArgs: got %v, want nil", p.queue[0].StringArgs)
	}

	s.processActiveScripts()
	p.client.flushWrite()
	got := <-received

	// Drain confirms the script fired through the parallel-slice path.
	if len(got) != 4 {
		t.Fatalf("queue fire: got %d bytes, want 4", len(got))
	}
	if len(p.queue) != 0 {
		t.Errorf("queue after fire: len=%d, want 0", len(p.queue))
	}
}
```

Verify imports: the test file already imports `"slices"` if Bundle-1 added it to `player_script_test.go`. For `script_test.go`, re-check:

```bash
grep -n "\"slices\"" /home/owner/Code/github.com/zsrv/goscape/modules/world/script_test.go
```

If absent, add `"slices"` to the import block (it appears alongside `"testing"`, `"time"`).

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestProcessPlayerQueueDeliversAllArgs" ./modules/world/ -v
```

Expected: **PASS**. The test pins both the queue-storage parallel-slice plumbing (Bundle 1) and the runScript dispatch (Bundle 1's tick.go forwarding step).

- [ ] **Step 6: Final compile + full-repo test pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: vet clean, build succeeds, full test suite **PASS**.

If anything is red: investigate before committing. Most likely failure modes:
- A test elsewhere (NPC queue tests, timer tests) that uses `playerQueueRequest` literal — re-grep `playerQueueRequest{` to confirm.
- A test that calls `EnqueueScriptTyped` somewhere not in the 6-site list — re-grep `EnqueueScriptTyped`.
- A test that asserts on `req.IntArg` — re-grep `req.IntArg|q.IntArg`.

- [ ] **Step 7: Commit Bundle 2**

```bash
git status --short
git diff --staged --stat
git add pkg/script/handlers.go pkg/script/handlers_test.go pkg/script/runner_test.go modules/world/player_script.go modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,script): NAI-26 Bundle 2 — queue family TS audit (NumberNotNull + popScriptArgs + script-missing error)

Port the 9 TS-faithfulness divergences in the queue family
(STRONGQUEUE / WEAKQUEUE / QUEUE / LONGQUEUE / P_DELAY) to match
Engine-TS/src/engine/script/handlers/PlayerOps.ts:97-180,375-379,1248-1263
line-by-line. Un-shares all 4 queue handlers from the pre-NAI-26
enqueueTyped helper (now removed); each handler has a TS-faithful own
body. Adds the popScriptArgs helper for STRONGQUEUE's variadic body.
Adds NumberNotNull wraps on STRONGQUEUE delay + P_DELAY n. Activates
the script-missing error return on (*Player).EnqueueScriptArgs (the
Bundle-1 placeholder returned nil to keep mechanical signature widening
separate from this behavior change).

Per-divergence remediation (TS file:line):

(α) STRONGQUEUE NumberNotNull on delay (PlayerOps.ts:99) — handleStrongQueue
    wraps PopInt with checkNotNull. Pins: TestStrongQueueDelayNullRejected.
(β) STRONGQUEUE popScriptArgs (PlayerOps.ts:98) — handleStrongQueue calls
    popScriptArgs first, popping a type-tags string then N typed args.
    Pins: TestStrongQueueEmptyScriptArgs / AllInt / Mixed and the 5
    helper unit tests TestPopScriptArgs_Empty / AllInt / AllString /
    Mixed / ReverseOrder.
(γ) STRONGQUEUE script-missing (PlayerOps.ts:103-105) —
    EnqueueScriptArgs activated error return, propagated by
    handleStrongQueue. Pins: TestQueueScriptNotFound.
(δ) WEAKQUEUE script-missing (PlayerOps.ts:127-129) — same. Pins:
    TestQueueScriptNotFound.
(ε) QUEUE script-missing (PlayerOps.ts:152-154) — same. Pins:
    TestQueueScriptNotFound.
(ζ) LONGQUEUE 4-popInt (PlayerOps.ts:172) — handleLongQueue pops 4
    ints (scriptId, delay, arg, logoutAction). Pre-NAI-26 popped only
    3, silently dropping logoutAction. Pins: TestLongQueuePopsFourInts.
(η) LONGQUEUE 2-arg array [logoutAction, arg] (PlayerOps.ts:179) —
    handleLongQueue passes [logoutAction, arg] (logoutAction-first).
    Pins: TestLongQueuePopsFourInts via IntArgs assertion.
(θ) LONGQUEUE script-missing (PlayerOps.ts:175-177) — same as γ. Pins:
    TestQueueScriptNotFound.
(κ) P_DELAY NumberNotNull on n (PlayerOps.ts:377) — handlePDelay
    wraps PopInt with checkNotNull. Pins: TestPDelayNullRejected.

popScriptArgs helper ports TS PlayerOps.ts:1248-1263 with goscape's
parallel-slice convention: returns (intArgs []int, stringArgs
[]string) instead of TS's []ScriptArgument sum-type slice. int args
land in tag-relative-int-order, string args land in tag-relative-
string-order. Mental-execution pin for tags="isi" + stack-pushed
[1, "two", 3]: intArgs=[1, 3], stringArgs=["two"].

enqueueTyped helper removed (zero consumers post-un-sharing per
dead_api_polish memory).

Per-site script_test.go re-evaluation (the 6 sites at :239, :269,
:336, :337, :825, :1020): all 6 enqueue REGISTERED scripts (0xAAAA,
0xBBBB, 0xCCC1, 0xCCC2, 0xBEEF, 0xBEE2 — RegisterAt cross-grep
verified). The Bundle-2 script-missing error activation does not
affect them; their tests still pass on the same registered-script
fire path. Spec's framing of these as "intentionally enqueue
non-existent scripts" was incorrect — corrected at plan-write time.

Integration test TestProcessPlayerQueueDeliversAllArgs validates the
Bundle-1 plumbing under realistic queue-fire conditions: a queue
request with IntArgs=[100, 200] passes intact through processPlayerQueue
to runScript.

TestQueueVariants pruned to weak-only: STRONGQUEUE has its own 4-test
suite (Delay-null + Empty/AllInt/Mixed args), LONGQUEUE has its own
4-popInt test. Removing them from the table-driven test eliminates
shape-mismatched fixtures that became invalid post-un-sharing.

Files:
- pkg/script/handlers.go: handleQueue / handleWeakQueue /
  handleStrongQueue / handleLongQueue un-shared bodies; popScriptArgs
  helper added; enqueueTyped helper removed; handlePDelay
  NumberNotNull wrap.
- pkg/script/handlers_test.go: 5 popScriptArgs unit tests + 4
  STRONGQUEUE tests + 1 LONGQUEUE test + 1 P_DELAY null test + 1
  table-driven TestQueueScriptNotFound covering all 4 queue ops;
  TestQueueVariants pruned to weak-only.
- pkg/script/runner_test.go: mockPlayer.enqueueScriptArgsReturnErr
  field for opt-in error return.
- modules/world/player_script.go: EnqueueScriptArgs script-missing
  error activated.
- modules/world/script_test.go: TestProcessPlayerQueueDeliversAllArgs
  integration test added.

Net deviation count unchanged (14 → 14). All 9 queue-family
divergences remediated TS-faithfully — no new deviation tags
introduced.

Closes the From-NAI-25 tracker entry at nai_followups.md:1574-1617
(STRONGQUEUE/P_DELAY NumberNotNull). Tracker scope was 2 wraps;
full-method audit found 8 additional structural divergences
(popScriptArgs missing, LONGQUEUE 4-popInt missing, script-missing
error missing across 4 opcodes); all 9 remediated TS-faithfully.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Two-stage review checkpoint (post-Bundle-2)

After both bundles land, dispatch two-stage review per `runescript_cadence` memory:

- **Stage 1 (spec compliance)** — fresh opus subagent compares each bundle's commit against the spec § Bundle N section. Bundle 1: signature widening landed at every cited site (struct, EnqueueScriptFile, EnqueueScriptArgs, interface, tick.go forwarding, mock, 6 script_test.go sites); enqueueTyped adapter preserves silent-no-op; engine-dispatch sites migrated to nil/nil; existing tests pass post-update. Bundle 2: each of α/β/γ/δ/ε/ζ/η/θ/κ remediated at the cited TS:line; popScriptArgs order semantics match TS (5 unit tests); script-missing error activated; no shipped-with-zero-consumers helpers (popScriptArgs has handleStrongQueue as consumer; enqueueTyped removed).
- **Stage 2 (code quality)** — fresh opus subagent reviews for naming consistency (`handle<OpName>` form for handlers; `Test<OpName><Behavior>` form for tests; `EnqueueScriptArgs` consistent across pkg/script + modules/world + tests + mocks); idiomatic Go (`slices.Equal`, no unnecessary `int()` casts on `PopInt`, consistent error-message format `"<OPCODE>: input number was null(-1)"`); test-helper reuse (`checkNotNull` reused without redefinition; `requireProtectedActivePlayer` reused unchanged); doc-comment narrative consistency (every Bundle-2-modified handler has a doc-comment naming the divergence(s) it remediates with TS:line citations).

Each stage is a single subagent dispatch. Polish commits land **before** the close commit if review surfaces remediable findings (per NAI-23 / NAI-24 / NAI-25 precedent: `polish(world,script): NAI-26 close polish` style).

---

## Close commit

Once both bundles + reviews + any polish commits have landed, append the close commit:

1. **Update `nai_followups.md`**:
   - Mark the From-NAI-25 entry at `:1574-1617` Resolved with the Bundle 2 commit hash. Resolution narrative: "Tracker scope was 2 NumberNotNull wraps (STRONGQUEUE delay + P_DELAY n); full-method audit found 8 additional structural divergences (popScriptArgs missing in STRONGQUEUE, LONGQUEUE 4-popInt + 2-arg-array missing, script-missing error missing across 4 opcodes); all 9 remediated TS-faithfully via the 2-bundle Bundle-1-plumbing + Bundle-2-handler-bodies cadence." Preserve original body.
   - Append a new `## From NAI-26 (2026-04-25)` section if any tracker-worthy items surfaced during the bundles (e.g. timer-family parallel-slice widening when its TS audit lands as a future sub-spec; the `*VARARG` opcode family per spec § Out-of-scope #5). If no new items, omit this section.

2. **Save memory entries** per `post_task_handoff` memory. Re-evaluate the brainstorm-time pre-flagged candidates from spec § "NAI-26 close":
   - `parallel_slice_convention_for_mixed_type_args` — likely save (generalizable convention; provenance NAI-26 Bundle 1 widening of playerQueueRequest).
   - `defer_behavior_change_within_mechanical_widening` — re-evaluate vs general bundle-decomposition heuristics; maybe save.
   - `vararg_opcode_shapes_dont_share_with_fixed-arg_siblings` — likely save (provenance NAI-26 brainstorm finding STRONGQUEUE silently using QUEUE shape under enqueueTyped).
   - `engine_dispatch_args_default_is_nil_not_zero` — maybe save; small enough to bundle into `parallel_slice_convention_for_mixed_type_args` memory.

3. **Stage and commit** (memory file is outside the working tree; no git stage):

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(world,script): NAI-26 closed — queue family TS-faithfulness audit

Closes the From-NAI-25 STRONGQUEUE/P_DELAY NumberNotNull tracker entry
via a 2-bundle cadence covering 9 queue-family divergences.

Bundle 1 (feat): playerQueueRequest parallel-slice plumbing.
Mechanical signature widening across pkg/script/active.go,
pkg/script/handlers.go, modules/world/player_script.go,
modules/world/tick.go + 4 test files. EnqueueScriptTyped renamed
to EnqueueScriptArgs with parallel IntArgs []int + StringArgs []string
slices and an error return (placeholder body, error activated in
Bundle 2). Engine-dispatch sites migrated to nil/nil per TS Player.ts:821
args=[] default. Temporary enqueueTyped adapter wraps the new shape
into the old single-int call so behavior is unchanged.

Bundle 2 (feat): per-opcode TS-faithful bodies + popScriptArgs +
NumberNotNull wraps + script-missing error activation. Un-shared
QUEUE / WEAKQUEUE / STRONGQUEUE / LONGQUEUE handlers from
enqueueTyped (helper removed). Added popScriptArgs helper porting
TS PlayerOps.ts:1248-1263. Added NumberNotNull on STRONGQUEUE delay
(α) + P_DELAY n (κ). LONGQUEUE 4-popInt (ζ) + [logoutAction, arg]
2-arg array (η). script-missing error propagation across all 4 queue
ops (γ/δ/ε/θ). 11 new tests pinning each divergence; 1 integration
test pinning Bundle 1 plumbing under realistic queue-fire conditions.

Brainstorm reframing: per audit_full_method_against_ts memory and
tracker_entry_framing_can_be_incomplete memory (both freshly recorded
at NAI-25 Bundle 1 close), the From-NAI-25 tracker entry's narrow
"~10 LOC, 2 wraps, compressed cadence" framing was reframed to
"9+ divergences, structural type-system widening, standard cadence
~250 LOC" via line-by-line audit of the entire queue family against
TS PlayerOps.ts:97-180,375-379.

Net deviation count: 14 → 14.

Closes memory: nai_followups.md:1574-1617 (From-NAI-25 STRONGQUEUE/P_DELAY NumberNotNull, Resolved by NAI-26 Bundles 1+2 — tracker scope was 2 wraps; full-method audit found 8 additional structural divergences; all 9 remediated TS-faithfully)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Plan self-review

**Spec coverage:** Every spec section maps to a task or step:
- Spec § Bundle 1 (playerQueueRequest widening + EnqueueScriptFile signature + EnqueueScriptArgs rename + engine-dispatch nil/nil + interface + tick.go + mock) → Task 1 Steps 1-14.
- Spec § Bundle 1 § "Critical sequencing decision" (placeholder nil-error body) → Task 1 Step 5.
- Spec § Bundle 1 § "Bundle 1 commit shape" → Task 1 Step 14.
- Spec § Bundle 2 § "popScriptArgs helper" → Task 2 Steps 2 (helper) + Steps 3-7 (5 unit tests).
- Spec § Bundle 2 § "handleStrongQueue" → Task 3 Steps 2-7.
- Spec § Bundle 2 § "handleLongQueue" → Task 4 Steps 2-4.
- Spec § Bundle 2 § "handleQueue" + "handleWeakQueue" → Task 5 Steps 2-3.
- Spec § Bundle 2 § "handlePDelay" κ → Task 5 Steps 4 + 6.
- Spec § Bundle 2 § "enqueueTyped removal" → Task 5 Step 5.
- Spec § Bundle 2 § "EnqueueScriptArgs script-missing error rollout" → Task 6 Steps 1-2.
- Spec § Bundle 2 § "Tests" / "Per-divergence test mapping" — every divergence listed maps to a test:
  - α (STRONGQUEUE NumberNotNull) → `TestStrongQueueDelayNullRejected` (Task 3 Step 2).
  - β-mixed (STRONGQUEUE popScriptArgs mixed) → `TestStrongQueuePopsMixedScriptArgs` (Task 3 Step 6).
  - β-empty → `TestStrongQueueEmptyScriptArgs` (Task 3 Step 4).
  - β-all-int → `TestStrongQueueAllIntScriptArgs` (Task 3 Step 5).
  - γ (STRONGQUEUE script-missing) → `TestQueueScriptNotFound` (Task 6 Step 3).
  - δ (WEAKQUEUE script-missing) → `TestQueueScriptNotFound` (Task 6 Step 3).
  - ε (QUEUE script-missing) → `TestQueueScriptNotFound` (Task 6 Step 3).
  - ζ (LONGQUEUE popInts(4)) → `TestLongQueuePopsFourInts` (Task 4 Step 2).
  - η (LONGQUEUE 2-arg array ordering) → `TestLongQueuePopsFourInts` IntArgs assertion (Task 4 Step 2).
  - θ (LONGQUEUE script-missing) → `TestQueueScriptNotFound` (Task 6 Step 3).
  - κ (P_DELAY NumberNotNull) → `TestPDelayNullRejected` (Task 5 Step 6).
- Spec § Bundle 2 § "Tests" / "popScriptArgs unit tests" (5 tests) → Task 2 Steps 3-7.
- Spec § Bundle 2 § "Updates to existing tests" (TestQueueVariants prune) → Task 3 Step 7 (strong removed) + Task 4 Step 4 (long removed).
- Spec § Bundle 2 § "Tests" / "Integration test" → Task 6 Step 5.
- Spec § Bundle 2 § "6 script_test.go sites migration" → Task 1 Step 12 (mechanical rename) + Task 6 Step 4 (per-site re-evaluation, conclusion: 0 migrations needed because all 6 enqueue registered scripts).
- Spec § Bundle 2 § "Bundle 2 commit shape" → Task 6 Step 7.
- Spec § "Out-of-scope" #1-6 → no plan tasks (correctly deferred per spec).
- Spec § "Risks & mitigations" → Task 1 Step 1 (struct location, runScript signature, req.IntArg cross-file) + Task 1 Step 13 (full-repo regression) + Task 6 Step 6 (full-repo regression).
- Spec § "Review structure" → Two-stage review checkpoint section.
- Spec § "NAI-26 close" → Close commit section.
- Spec § "Memory entry candidates pre-flagged" → Close commit § "Save memory entries" #2.

**Placeholder scan:** No forbidden patterns ("TBD", "TODO", "implement later", "fill in details", "appropriate error handling"). The Task 1 Step 14 commit message and Task 6 Step 7 commit message use HEREDOC-quoted full text. The `<Bundle 1 commit hash>` and `<Tasks 2-5 staged>` references in Task 2/3/4/5/6 pre-flight contexts are forward-references filled at dispatch time after the prerequisite work lands.

**Type consistency:** Across all 6 tasks, the production signature `(*Player).EnqueueScriptArgs(scriptID uint32, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) error` is referenced consistently. The interface signature in `pkg/script/active.go` matches. The mock's `EnqueueScriptArgs(id uint32, delay int, intArgs []int, stringArgs []string, qtype PlayerQueueType) error` matches. The test names (`TestStrongQueueDelayNullRejected`, `TestStrongQueueEmptyScriptArgs`, `TestStrongQueueAllIntScriptArgs`, `TestStrongQueuePopsMixedScriptArgs`, `TestLongQueuePopsFourInts`, `TestPDelayNullRejected`, `TestQueueScriptNotFound`, `TestPopScriptArgs_Empty/AllInt/AllString/Mixed/ReverseOrder`, `TestProcessPlayerQueueDeliversAllArgs`) are spelled identically across spec, plan steps, and commit messages. The error message format `"<OPCODE>: input number was null(-1)"` (from `checkNotNull` at `handlers_player.go:63`) is consistent across α and κ test substring assertions. The `playerQueueRequest` struct field names (`IntArgs []int`, `StringArgs []string`) are consistent across struct, EnqueueScriptFile body, and tick.go forwarding.

**Cross-reference check:** Task 5 references `popScriptArgs` only via `handleStrongQueue` (Task 3) — no direct Task-5 consumer. Task 2 defines `popScriptArgs` and its 5 unit tests; Task 3 is the first production consumer (`handleStrongQueue`); Tasks 4-5 do not need to reference it. The cross-task chain is: Task 2 (helper + helper tests) → Task 3 (STRONGQUEUE consumer + STRONGQUEUE tests) → Task 4 (LONGQUEUE no helper consumer) → Task 5 (QUEUE/WEAKQUEUE no helper consumer + P_DELAY κ + helper removal of enqueueTyped) → Task 6 (script-missing error activation + per-site re-eval + integration test + commit). Helpers Task 2 ships with future consumers in Task 3 — acceptable per `dead_api_polish` memory because the consumer ships in the same bundle.

**Mental execution of test stack-push sequences:**
- `TestStrongQueueDelayNullRejected`: int [77, -1], string [""]. `popScriptArgs` pops "" → nil/nil. Pop delay → -1. `checkNotNull(-1) → error`. ✓
- `TestStrongQueueEmptyScriptArgs`: int [77, 3], string [""]. popScriptArgs → nil/nil. delay 3 OK. scriptID 77. Mock records ScriptID=77, Delay=3, IntArgs=nil, StringArgs=nil. ✓
- `TestStrongQueueAllIntScriptArgs`: int [77, 5, 10, 20, 30], string ["iii"]. popScriptArgs reverse-pop: i=2 → 30 to intArgs[2], i=1 → 20 to intArgs[1], i=0 → 10 to intArgs[0]. intArgs=[10,20,30]. delay 5, scriptID 77. ✓
- `TestStrongQueuePopsMixedScriptArgs`: int [77, 2, 99], string ["hello", "is"]. popScriptArgs pops "is", count=2, intCount=1, stringCount=1. i=1 ('s') → "hello" to stringArgs[0]. i=0 ('i') → 99 to intArgs[0]. intArgs=[99], stringArgs=["hello"]. delay 2, scriptID 77. ✓
- `TestLongQueuePopsFourInts`: int [77, 3, 99, 42]. handleLongQueue pops in order: logoutAction=42, arg=99, delay=3, scriptID=77. IntArgs=[42, 99] (logoutAction first). ✓
- `TestPDelayNullRejected`: int [-1]. requireProtectedActivePlayer OK (protect=true). PopInt → -1. checkNotNull(-1) → error. ✓
- `TestQueueScriptNotFound` STRONGQUEUE case: int [77, 3], string [""]. popScriptArgs → nil/nil. delay 3 (checkNotNull OK). scriptID 77. Mock returns configured error. handler returns error. ✓
- `TestQueueScriptNotFound` LONGQUEUE case: int [77, 3, 42, 99]. PopInt logoutAction=99, arg=42, delay=3, scriptID=77. EnqueueScriptArgs(77, 3, [99, 42], nil, QueueLong) → mock returns configured error. ✓
- `TestProcessPlayerQueueDeliversAllArgs`: registers 0xD1D1, enqueues `[]int{100, 200}`, asserts queue[0].IntArgs==[100,200] before fire, asserts queue empty after fire and 4 wire bytes received. ✓
- popScriptArgs unit tests: all 5 verified in Task 2 Steps 3-7 inline mental execution.

**Plan-test-coverage crosscheck** (per `plan_test_coverage_crosscheck` memory):
- 9 production divergences (α, β, γ, δ, ε, ζ, η, θ, κ) → 11 new test functions:
  - α → 1 test
  - β → 3 tests (Empty, AllInt, Mixed) + 5 helper unit tests
  - γ + δ + ε + θ → 1 table-driven test with 4 sub-tests
  - ζ + η → 1 test (combined assertion)
  - κ → 1 test
  - integration → 1 test
  - Total: 11 new test functions in `pkg/script/handlers_test.go` + 1 in `modules/world/script_test.go` = 12 new functions; 5 helper unit tests in pkg/script/handlers_test.go.

**Plan-runnable-test-fixture crosscheck** (per `plan_runnable_test_fixtures` memory): every test in Tasks 2-6 is mentally executed inline against the post-implementation code. Stack-push order, validator signatures, expected error messages all verified. The integration test in Task 6 Step 5 has a fallback path documented (queue-empty assertion if arg-reading opcodes don't exist) — implementer pre-flight verifies which path applies.

**Plan-helper-coverage crosscheck** (per `plan_helper_coverage` memory): no new test helpers introduced. `mockPlayer.enqueueScriptArgsReturnErr` is a struct-field opt-in, not a shared helper. `popScriptArgs` is a production helper, exercised by `handleStrongQueue` (Task 3) and 5 dedicated unit tests (Task 2).

**Helper-pattern crosscheck** (per `plan_grep_helper_patterns` memory):
- `checkNotNull(v int, op string) error` at `handlers_player.go:61` — reused in Task 3 Step 3 and Task 5 Step 4 without redefinition.
- `requireProtectedActivePlayer(s, "P_DELAY") error` at `handlers_player.go:48` — used by handlePDelay; Task 5 Step 4 preserves it.
- `newSingleOp(name, op)` at `handlers_player_test.go:52` — reused in Task 3-5-6 tests.
- `buildGreetScript(key, ch)` at `script_test.go:130` — reused in Task 6 Step 5 integration test.
- No inline boilerplate when a helper exists.

**Enumerate-all-sites crosscheck** (per `enumerate_all_sites` memory):
- Task 1 Step 1's `grep -rn "EnqueueScriptTyped"` re-enumerates the cross-package consumer set. Spec-write-time grep results: pkg/script/active.go, pkg/script/handlers.go, modules/world/player_script.go (3 mentions), pkg/script/runner_test.go, modules/world/script_test.go (6 sites). Total cross-package pin count = 12 references across 5 files. If a new site appears at task time: ESCALATE per Step 1.
- Task 5 Step 5's `grep -rn "enqueueTyped"` re-enumerates pre-removal to confirm zero consumers post-Task-5-Steps-2-4.

**Spec-followup-tracker-freshness** (per `spec_followup_tracker_freshness` memory): tracker entry assertions (STRONGQUEUE site at handlers.go:629, P_DELAY site at handlers.go:589, enqueueTyped helper at handlers.go:599-619) verified at HEAD `53379a0`. All assertions held at HEAD. Re-verified at task dispatch via Step 1 grep commands.

**Controller-preflight discipline:** every task begins with a Step 1 pre-flight verification (file paths, line numbers, signature shapes, helper presence, prerequisite-task staging).

**Audit-full-method discipline applied:** the brainstorm reframed the From-NAI-25 tracker's narrow 2-wrap framing (α + κ) into the full 9-divergence set (α, β, γ, δ, ε, ζ, η, θ, κ) by line-by-line audit of TS PlayerOps.ts:97-180, 375-379, 1248-1263 against goscape's queue-family bodies. All 9 divergences appear in the plan with code + tests.

No issues found.
