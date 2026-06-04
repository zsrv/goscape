# NAI-27 player timer family TS audit + player VARARG opcode family port + NPC queue audit memo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the player timer family + 4 player VARARG opcodes line-by-line against `Engine-TS/src/engine/script/handlers/PlayerOps.ts:110-192,820-864` and `Engine-TS/src/engine/entity/Player.ts:907-941`. Bundle 1 widens `playerTimer.IntArg int` → `IntArgs []int + StringArgs []string`, widens `(*Player).SetTimer` and `script.ActivePlayer.SetTimer` to carry the parallel slices (no `error` return yet), and updates the `tick.go:292` consumer + mockPlayer recorder. Bundle 2 activates `popScriptArgs` in SETTIMER/SOFTTIMER, activates the script-missing error on SETTIMER/SOFTTIMER (entity-layer return) + GETTIMER (handler-side `Provider.GetByID` check), flips `(*Player).GetTimer`'s untracked semantic divergence ("remaining ticks" → "absolute Clock") to TS-faithful, migrates inline player-active gates to `requireActivePlayer`, and drops dead `int(s.PopInt())` casts. Bundle 3 wires the 4 VARARG opcodes (constants + String() arms already exist; only handlers + dispatch entries needed) and records the NPC queue audit memo (expected 0-LOC).

**Architecture:** Three sequential bundles; each lands a self-contained commit and is reviewed before the next is dispatched. Bundle 1 is a mechanical signature widening with `nil, nil` placeholder slices (no behavior change, no new tests). Bundle 2 is the timer-family TS-faithfulness work. Bundle 3 is the VARARG opcode wiring (using NAI-26's `popScriptArgs` + the now-widened-by-Bundle-2 `EnqueueScriptArgs` script-missing path) plus the NPC queue line-by-line audit. A close commit follows (Task 4): updates `nai_followups.md`, marks NAI-26 Out-of-scope #3 + #4 Resolved, and adds the `Closes memory:` trailer per memory `close_commit_memory_trailer`.

**Tech Stack:** Go 1.26+. Existing helpers reused:
- `requireActivePlayer(s *ScriptState, op string) error` at `pkg/script/handlers_player.go:35`
- `popScriptArgs(s *ScriptState) (intArgs []int, stringArgs []string)` at `pkg/script/handlers.go:630`
- `checkNotNull(v int, op string) error` at `pkg/script/handlers_player.go:61` (not used by this plan; spec confirms VARARG variants do not check NumberNotNull)
- `s.Provider.GetByID(targetID uint32) *ScriptFile` (template at `pkg/script/handlers.go:546-549`)
- TS source root: `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/`. Reference TS sources: `Engine-TS/src/engine/script/handlers/PlayerOps.ts:110-192` (4 VARARG queue ops), `:820-864` (5 timer ops); `Engine-TS/src/engine/entity/Player.ts:907-941` (setTimer + processTimers).

**Spec reference:** `docs/superpowers/specs/2026-04-25-nai-27-timer-vararg-npc-queue-audit-design.md`.

**HEAD at plan-write:** `0f39b81` (after spec commit). Plan-author preflight surfaced **three spec premise corrections** captured in Task 3's pre-flight context — none invalidate the spec, all reduce Bundle 3's scope.

---

## Plan-author preflight findings (spec corrections)

The following premise corrections were caught at plan-write per memory `controller_preflight`. They do not require respinning the spec; they are recorded here as authoritative for implementer dispatch.

1. **All 4 VARARG opcode constants ALREADY exist with `String()` arms.** Spec § Bundle 3 mandates "4 new `Op*` constants + 4 new `String()` arms"; actual HEAD has:
   - `OpLongQueueVarArg = 2060` (`pkg/script/opcode.go:160`), `String()` arm at `:727` returning `"LONGQUEUE*"`
   - `OpQueueVarArg = 2093` (`:193`), `String()` arm at `:793` returning `"QUEUE*"`
   - `OpStrongQueueVarArg = 2118` (`:218`), `String()` arm at `:843` returning `"STRONGQUEUE*"`
   - `OpWeakQueueVarArg = 2130` (`:230`), `String()` arm at `:867` returning `"WEAKQUEUE*"`
   - **NONE are wired in `pkg/script/handlers.go` dispatch table.** Bundle 3 task scope reduces to: implement 4 handler functions + add 4 dispatch table entries. No opcode-constant work, no String()-arm work.

2. **Naming convention is `VarArg` (camelCase), NOT `Vararg`.** Spec used both spellings. The plan's code blocks (and all symbol references) use `VarArg` exclusively per the existing constants.

3. **`(*Player).EnqueueScriptArgs` body at `modules/world/player_script.go:102-118` is the EXACT template for the new `(*Player).SetTimer` body in Bundle 2** — including the `if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil { return nil }` engine-dispatch tolerance + the `fmt.Errorf("unable to find timer script: %d", scriptID)` pattern. The plan's Bundle 2 SetTimer body mirrors this verbatim.

---

## Task 1 — Bundle 1: `playerTimer` parallel-slice plumbing (mechanical widening)

**Files:**
- Modify: `modules/world/player.go:37-46` (playerTimer struct field rename)
- Modify: `modules/world/player_timer.go:6-21` (Player.SetTimer signature widening, body retargets to parallel-slice fields)
- Modify: `pkg/script/active.go:228-234` (ActivePlayer.SetTimer interface signature widening + doc-comment refresh)
- Modify: `pkg/script/handlers_timer.go:9-21` (enqueueTimer adapter: drop arg pop, pass `nil, nil` placeholder)
- Modify: `modules/world/tick.go:292` (timer-fire site forwards `t.IntArgs, t.StringArgs` to runScript)
- Modify: `pkg/script/runner_test.go:176-180, :413-421` (mockPlayer SetTimer recorder struct widening + method signature widening)
- Modify: `pkg/script/handlers_timer_test.go:5-31` (TestSetTimerCapturesArgs assertion update for nil/nil placeholder shape)
- Modify: `pkg/script/handlers_timer_test.go:33-52` (TestSoftTimerSetsSoftType — no field change but body uses lastSetTimer struct → may need recompile-clean assertion)

**Pre-flight context:**
- HEAD `0f39b81` at task dispatch. Verify all line numbers via re-grep at task time per `controller_preflight` memory.
- `(*Server).runScript` signature at `modules/world/script.go:14` is `(sf *script.ScriptFile, self script.ActivePlayer, protect bool, intArgs []int, stringArgs []string)` — already accepts the parallel-slice shape; no signature change needed at the call site, only the call-site arguments change.
- The `t.IntArg` consumer at `tick.go:292` is the **only** production reader of `playerTimer.IntArg` (verified via `grep -n "t\.IntArg\b" modules/world/`). Mock-test consumers in `pkg/script/runner_test.go` will compile-break and require updates, covered by Step 5 below.
- No new tests in Bundle 1 — purely mechanical. Bundle 2 adds the real assertions for popScriptArgs + script-missing.
- Spec correction: spec said `enqueueTimer` should retain `nil`-error return shape post-widening. Confirmed: `(*Player).SetTimer` returns no value in Bundle 1 (no error return added until Bundle 2). The `enqueueTimer` body in Bundle 1 still calls `s.Self.SetTimer(...)` and returns `nil`.

- [ ] **Step 1: Pre-flight verification — file paths, line numbers, signature shapes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go version
git log --oneline -3
grep -n "type playerTimer\b" /home/owner/Code/github.com/zsrv/goscape/modules/world/player.go
grep -n "func (p \*Player) SetTimer\|func (p \*Player) GetTimer\|func (p \*Player) ClearTimer" /home/owner/Code/github.com/zsrv/goscape/modules/world/player_timer.go
grep -n "SetTimer\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/active.go
grep -n "func enqueueTimer\|handleSetTimer\|handleSoftTimer\|handleClearTimer\|handleClearSoftTimer\|handleGetTimer" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_timer.go
grep -n "t\.IntArg\b\|s\.runScript" /home/owner/Code/github.com/zsrv/goscape/modules/world/tick.go
grep -n "lastSetTimer\|setTimerCalls\|func (m \*mockPlayer) SetTimer\|func (m \*mockPlayer) GetTimer\|getTimerValue" /home/owner/Code/github.com/zsrv/goscape/pkg/script/runner_test.go
grep -rn "playerTimer\.IntArg\b\|t\.IntArg\b" /home/owner/Code/github.com/zsrv/goscape/ --include="*.go"
```

Record: confirmed line numbers; confirmed only the 1 production reader of `playerTimer.IntArg` at `tick.go:292`; confirmed mockPlayer field layout. If a new consumer of `playerTimer.IntArg` appears between spec-write and dispatch: ESCALATE — the plan's enumeration is bounded.

- [ ] **Step 2: Update `playerTimer` struct (field rename)**

Edit `modules/world/player.go` lines 37-46. Current shape:

```go
// playerTimer is a per-player repeating script registration.
// S5i: identified by target scriptID (TS semantics: setTimer at same
// id overwrites).
type playerTimer struct {
	ScriptID uint32
	Type     script.PlayerTimerType
	Interval int
	Clock    int
	IntArg   int
}
```

Replace with:

```go
// playerTimer is a per-player repeating script registration.
// S5i: identified by target scriptID (TS semantics: setTimer at same
// id overwrites).
//
// As of NAI-27 Bundle 1, the single IntArg int field is widened to
// parallel IntArgs []int + StringArgs []string slices to match the TS
// PlayerTimer.args ScriptArgument[] shape (TS
// Engine-TS/src/engine/entity/Player.ts:910 args field). The widening
// is required for SETTIMER/SOFTTIMER's variadic popScriptArgs body
// (PlayerOps.ts:826,834), which Bundle 2 activates.
type playerTimer struct {
	ScriptID   uint32
	Type       script.PlayerTimerType
	Interval   int
	Clock      int
	IntArgs    []int
	StringArgs []string
}
```

This change will break compilation of every site reading `t.IntArg`. That is expected; Steps 3 and 4 fix the readers.

- [ ] **Step 3: Update `(*Player).SetTimer` signature + body**

Edit `modules/world/player_timer.go` lines 6-21. Current shape:

```go
// SetTimer implements script.ActivePlayer.SetTimer.
func (p *Player) SetTimer(scriptID uint32, interval, intArg int, ttype script.PlayerTimerType) {
	if p.timers == nil {
		p.timers = make(map[uint32]*playerTimer)
	}
	now := 0
	if p.client != nil && p.client.server != nil {
		now = p.client.server.currentTick
	}
	p.timers[scriptID] = &playerTimer{
		ScriptID: scriptID,
		Type:     ttype,
		Interval: interval,
		Clock:    now,
		IntArg:   intArg,
	}
}
```

Replace with:

```go
// SetTimer implements script.ActivePlayer.SetTimer.
//
// NAI-27 Bundle 1: signature widens to carry parallel IntArgs + StringArgs
// slices. The error return is added in Bundle 2 alongside the script-missing
// check; for now the method is non-fallible and returns nothing.
func (p *Player) SetTimer(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype script.PlayerTimerType) {
	if p.timers == nil {
		p.timers = make(map[uint32]*playerTimer)
	}
	now := 0
	if p.client != nil && p.client.server != nil {
		now = p.client.server.currentTick
	}
	p.timers[scriptID] = &playerTimer{
		ScriptID:   scriptID,
		Type:       ttype,
		Interval:   interval,
		Clock:      now,
		IntArgs:    intArgs,
		StringArgs: stringArgs,
	}
}
```

- [ ] **Step 4: Update `tick.go` consumer**

Edit `modules/world/tick.go` line 292. Current line:

```go
			s.runScript(sf, p, false, []int{t.IntArg}, nil)
```

Replace with:

```go
			s.runScript(sf, p, false, t.IntArgs, t.StringArgs)
```

This is the only production reader of `t.IntArg`; replacing it removes the field's last consumer and unblocks compilation.

- [ ] **Step 5: Update `script.ActivePlayer.SetTimer` interface + doc-comment**

Edit `pkg/script/active.go` lines 228-234. Current shape:

```go
	// S5i: timer ops.

	// SetTimer registers a timer that re-runs the script at scriptID every
	// `interval` ticks with `intArg` as the single int arg. Overwrites any
	// existing timer at the same scriptID. type = TimerNormal (waits for
	// idle) or TimerSoft (fires while busy).
	SetTimer(scriptID uint32, interval int, intArg int, ttype PlayerTimerType)
```

Replace with:

```go
	// S5i: timer ops.

	// SetTimer registers a timer that re-runs the script at scriptID every
	// `interval` ticks with `intArgs`/`stringArgs` as parallel-slice typed
	// args (matching TS PlayerOps.ts:826,834 popScriptArgs convention).
	// Overwrites any existing timer at the same scriptID. type = TimerNormal
	// (waits for idle) or TimerSoft (fires while busy).
	//
	// NAI-27 Bundle 1: signature widened from single intArg int to parallel
	// IntArgs []int + StringArgs []string. Bundle 2 adds an `error` return
	// for the script-missing check.
	SetTimer(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype PlayerTimerType)
```

- [ ] **Step 6: Update `enqueueTimer` adapter (handler-layer caller)**

Edit `pkg/script/handlers_timer.go` lines 8-21. Current shape:

```go
// enqueueTimer is the shared body for SETTIMER / SOFTTIMER.
func enqueueTimer(s *ScriptState, ttype PlayerTimerType, op string) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("%s: no active player", op)
	}
	arg := int(s.PopInt())
	interval := int(s.PopInt())
	scriptID := uint32(s.PopInt())
	s.Self.SetTimer(scriptID, interval, arg, ttype)
	return nil
}
```

Replace with:

```go
// enqueueTimer is the shared body for SETTIMER / SOFTTIMER.
//
// NAI-27 Bundle 1: passes nil/nil placeholder slices to the widened
// SetTimer signature. Bundle 2 swaps the placeholders for popScriptArgs
// and adds the script-missing error propagation.
func enqueueTimer(s *ScriptState, ttype PlayerTimerType, op string) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("%s: no active player", op)
	}
	interval := s.PopInt()
	scriptID := uint32(s.PopInt())
	s.Self.SetTimer(scriptID, interval, nil, nil, ttype)
	return nil
}
```

Note: dropped `arg := int(s.PopInt())` line entirely — the argument that the old code consumed will be re-introduced via `popScriptArgs` in Bundle 2 (which pops the type-tags string + typed args from the top of the stack BEFORE interval/scriptID).

- [ ] **Step 7: Update `mockPlayer` SetTimer recorder + method**

Edit `pkg/script/runner_test.go` lines 176-180 (recorder fields) and lines 413-415 (method body). Current recorder field at line 176:

```go
	lastSetTimer    struct{ scriptID uint32; interval, intArg int; ttype PlayerTimerType }
```

Replace with:

```go
	lastSetTimer    struct {
		scriptID   uint32
		interval   int
		intArgs    []int
		stringArgs []string
		ttype      PlayerTimerType
	}
```

Current method at lines 413-415:

```go
func (m *mockPlayer) SetTimer(scriptID uint32, interval, intArg int, ttype PlayerTimerType) {
	m.lastSetTimer = struct{ scriptID uint32; interval, intArg int; ttype PlayerTimerType }{scriptID, interval, intArg, ttype}
	m.setTimerCalls++
}
```

Replace with:

```go
func (m *mockPlayer) SetTimer(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype PlayerTimerType) {
	m.lastSetTimer = struct {
		scriptID   uint32
		interval   int
		intArgs    []int
		stringArgs []string
		ttype      PlayerTimerType
	}{scriptID, interval, intArgs, stringArgs, ttype}
	m.setTimerCalls++
}
```

- [ ] **Step 8: Update `TestSetTimerCapturesArgs` to placeholder shape**

Edit `pkg/script/handlers_timer_test.go` lines 5-31. The bytecode currently pushes 3 ints (scriptID, interval, arg) and then runs `OpSetTimer`. Bundle 1 drops the third pop, so the bytecode must drop the third push. Current shape:

```go
func TestSetTimerCapturesArgs(t *testing.T) {
	sf := &ScriptFile{
		Name: "set_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, // scriptID
			OpPushConstantInt, // interval
			OpPushConstantInt, // arg
			OpSetTimer,
			OpReturn,
		},
		IntOperands:      []int32{0x12345678, 5, 42, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.setTimerCalls != 1 {
		t.Fatalf("setTimerCalls: got %d, want 1", mp.setTimerCalls)
	}
	got := mp.lastSetTimer
	if got.scriptID != 0x12345678 || got.interval != 5 || got.intArg != 42 || got.ttype != TimerNormal {
		t.Errorf("lastSetTimer: got %+v, want scriptID=0x12345678 interval=5 intArg=42 type=Normal", got)
	}
}
```

Replace with:

```go
func TestSetTimerCapturesArgs(t *testing.T) {
	// NAI-27 Bundle 1: placeholder shape pre-popScriptArgs. Bundle 2
	// re-pins this with real popScriptArgs args after activation.
	sf := &ScriptFile{
		Name: "set_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, // scriptID
			OpPushConstantInt, // interval
			OpSetTimer,
			OpReturn,
		},
		IntOperands:      []int32{0x12345678, 5, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.setTimerCalls != 1 {
		t.Fatalf("setTimerCalls: got %d, want 1", mp.setTimerCalls)
	}
	got := mp.lastSetTimer
	if got.scriptID != 0x12345678 || got.interval != 5 || got.intArgs != nil || got.stringArgs != nil || got.ttype != TimerNormal {
		t.Errorf("lastSetTimer: got %+v, want scriptID=0x12345678 interval=5 intArgs=nil stringArgs=nil type=Normal", got)
	}
}
```

- [ ] **Step 9: Update `TestSoftTimerSetsSoftType` to placeholder shape**

Edit `pkg/script/handlers_timer_test.go` lines 33-52. Current shape:

```go
func TestSoftTimerSetsSoftType(t *testing.T) {
	sf := &ScriptFile{
		Name: "soft_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpSoftTimer, OpReturn,
		},
		IntOperands:      []int32{0x7BCDEF00, 3, 7, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastSetTimer.ttype != TimerSoft {
		t.Errorf("ttype: got %v, want TimerSoft", mp.lastSetTimer.ttype)
	}
}
```

Replace with (drop the third push to match the new 2-pop enqueueTimer shape):

```go
func TestSoftTimerSetsSoftType(t *testing.T) {
	// NAI-27 Bundle 1: placeholder shape pre-popScriptArgs.
	sf := &ScriptFile{
		Name: "soft_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt,
			OpSoftTimer, OpReturn,
		},
		IntOperands:      []int32{0x7BCDEF00, 3, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastSetTimer.ttype != TimerSoft {
		t.Errorf("ttype: got %v, want TimerSoft", mp.lastSetTimer.ttype)
	}
}
```

- [ ] **Step 10: Run the full test suite (must pass)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS across all packages. If any package outside the touched files fails, ESCALATE — Bundle 1 is mechanical and should not break unrelated paths.

- [ ] **Step 11: Acceptance-criteria self-check**

```bash
git grep "IntArg\b" modules/world/player.go modules/world/player_timer.go pkg/script/handlers_timer.go pkg/script/active.go pkg/script/runner_test.go
git grep "popScriptArgs" pkg/script/handlers_timer.go
```

Expected:
- First grep: ZERO results (the `playerTimer.IntArg` field is fully gone). Note: NPC queue's `IntArg` at `pkg/script/queue.go:45` is unrelated and out-of-scope; the grep above limits to the timer-touched files so should not match it.
- Second grep: ZERO results (popScriptArgs is Bundle 2's work).

If either grep returns results, ESCALATE — the spec's acceptance criteria require both grep-clean conditions.

- [ ] **Step 12: Commit Bundle 1**

```bash
git add modules/world/player.go modules/world/player_timer.go modules/world/tick.go pkg/script/active.go pkg/script/handlers_timer.go pkg/script/handlers_timer_test.go pkg/script/runner_test.go
git status
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,script): NAI-27 Bundle 1 — playerTimer parallel-slice plumbing

Mirrors NAI-26 Bundle 1 (366c543) cadence on the timer family. Widens
playerTimer.IntArg int → IntArgs []int + StringArgs []string, widens
(*Player).SetTimer + script.ActivePlayer.SetTimer interface to carry
the parallel slices, and updates the tick.go:292 timer-fire site to
forward the new fields. enqueueTimer drops the now-stale single-arg
pop and passes nil, nil placeholder slices to the widened signature.
Bundle 2 will swap the placeholders for popScriptArgs and add
script-missing error propagation.

No behavioral changes; no new tests. Existing TestSetTimerCapturesArgs
and TestSoftTimerSetsSoftType bytecode fixtures drop the third
PushConstantInt to match the new 2-pop enqueueTimer shape.

mockPlayer recorder struct widened to mirror the new SetTimer
signature.

Refs: docs/superpowers/specs/2026-04-25-nai-27-timer-vararg-npc-queue-audit-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — Bundle 2: Timer family TS-faithfulness audit

**Files:**
- Modify: `modules/world/player_timer.go:6-21` (Player.SetTimer adds error return + script-missing check; mirrors EnqueueScriptArgs at player_script.go:102-118)
- Modify: `modules/world/player_timer.go:33-46` (Player.GetTimer semantic flip: return `t.Clock` directly, drop `now` lookup)
- Modify: `pkg/script/active.go:228-234` (interface signature gains error return)
- Modify: `pkg/script/handlers_timer.go:8-48` (5 handlers: requireActivePlayer migration, popScriptArgs activation in SETTIMER/SOFTTIMER, script-missing-error propagation, GETTIMER handler-side script-missing check)
- Modify: `pkg/script/runner_test.go:413-421` (mockPlayer.SetTimer adds error return; helper to seed script-missing for tests)
- Modify: `pkg/script/handlers_timer_test.go` (extend TestSetTimerCapturesArgs to verify popScriptArgs roundtrip; add TestSoftTimerCapturesArgs; add 3 script-missing tests; update TestGetTimer to seed a registered script in the test provider)
- Create: `modules/world/player_timer_test.go` (entity-level GetTimer semantic-flip tests + not-found pin)

**Pre-flight context:**
- HEAD will be at the Bundle 1 commit. Verify by `git log --oneline -1`.
- The script-missing check pattern for SETTIMER/SOFTTIMER is the entity-layer return path mirroring `(*Player).EnqueueScriptArgs` at `modules/world/player_script.go:102-118`. Implementation reads:

  ```go
  if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
      return nil
  }
  if p.client.server.scriptProvider.GetByID(scriptID) == nil {
      return fmt.Errorf("unable to find timer script: %d", scriptID)
  }
  ```

  The `nil` Provider chain returns `nil` (engine-dispatch tolerance — same as EnqueueScriptArgs); only with a configured provider does the missing-script check fire. This preserves engine-callable paths (none currently exist for timers, but parallel structure with EnqueueScriptArgs is the principled choice).

- The script-missing check for GETTIMER is handler-side (per the GOSUB_WITH_PARAMS template at `pkg/script/handlers.go:546-549`):

  ```go
  if s.Provider == nil {
      return fmt.Errorf("GETTIMER: Provider not set on ScriptState")
  }
  if s.Provider.GetByID(scriptID) == nil {
      return fmt.Errorf("GETTIMER: unable to find timer script: %d", scriptID)
  }
  ```

  Handler-side rather than entity-side because `(*Player).GetTimer` returns a plain `int` (with `-1` sentinel for not-found). Routing the script-missing error through a `(int, error)` return would force a signature change on a value-returning method without parallel benefit. The handler-side pattern is precedented at `handleGosubWithParams`.

- TS reference (re-read at plan-write):
  - SOFTTIMER (`PlayerOps.ts:815-827`): `args = popScriptArgs(state); interval = popInt; timerId = popInt; script = ScriptProvider.get(timerId); if !script throw; setTimer(SOFT, script, args, interval)`
  - CLEARSOFTTIMER (`:829-831`): `clearTimer(popInt)` — no script lookup
  - SETTIMER (`:833-843`): identical shape to SOFTTIMER but type=NORMAL
  - CLEARTIMER (`:845-847`): `clearTimer(popInt)` — no script lookup
  - GETTIMER (`:849-864`): `timerId = popInt; script = ScriptProvider.get(timerId); if !script throw; iterate timers; pushInt(timer.clock) if found else pushInt(-1)`

- TS Player.setTimer (`Player.ts:907-941`): stores `clock: World.currentTick` at set-time; `processTimers` fires when `currentTick >= timer.clock + timer.interval` and resets `clock = currentTick` post-fire. The GETTIMER handler reads `timer.clock` (the absolute tick when the timer was last set/fired). Goscape's current `(*Player).GetTimer` computes `(t.Clock + t.Interval) - now` ("remaining ticks") — divergent. Bundle 2 flips it to return `t.Clock` directly.

- [ ] **Step 1: Pre-flight verification — Bundle 1 landed; helper signatures unchanged**

```bash
git log --oneline -2
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
grep -n "func popScriptArgs\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go
grep -n "func requireActivePlayer\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_player.go
grep -n "Provider \*Provider" /home/owner/Code/github.com/zsrv/goscape/pkg/script/state.go
grep -n "func (p \*Provider) GetByID\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/provider.go
grep -n "func (p \*Player) EnqueueScriptArgs" /home/owner/Code/github.com/zsrv/goscape/modules/world/player_script.go
```

Confirm: Bundle 1 commit on top; full test suite green; popScriptArgs / requireActivePlayer / Provider.GetByID all present at expected lines. If any helper has shifted shape, ESCALATE — the plan's templates depend on these signatures.

- [ ] **Step 2: Add failing test — `TestSetTimerScriptMissing` (entity-side error propagation)**

Append to `pkg/script/handlers_timer_test.go`. The test pushes a scriptID that the test Provider returns nil for, and asserts the handler propagates the entity-layer error.

```go
// TestSetTimerScriptMissing pins NAI-27 Bundle 2: SETTIMER returns a
// non-nil error when the scriptID does not resolve. Mirrors TS
// PlayerOps.ts:838-840 (Unable to find timer script: ${id}).
//
// Requires the mockPlayer to surface the entity-layer error returned
// from SetTimer; mockPlayer.SetTimer's signature gains an error return
// in Bundle 2 (Step 7) parallel to (*Player).EnqueueScriptArgs at
// modules/world/player_script.go:102-118.
func TestSetTimerScriptMissing(t *testing.T) {
	sf := &ScriptFile{
		Name: "set_timer_missing",
		Opcodes: []Opcode{
			OpPushConstantInt, // scriptID (will not resolve)
			OpPushConstantInt, // interval
			OpPushConstantString, // type-tag string for popScriptArgs (empty = no args)
			OpSetTimer,
			OpReturn,
		},
		IntOperands:      []int32{0xDEADBEEF, 5, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{setTimerErr: errors.New("unable to find timer script: 3735928559")}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unable to find timer script") {
		t.Errorf("error: got %v, want contains 'unable to find timer script'", err)
	}
}
```

Note: this test references `mp.setTimerErr` — a new mockPlayer field added in Step 7. The required `errors` and `strings` imports already exist in `handlers_timer_test.go` only if needed elsewhere; if the file lacks them, add them in Step 8.

- [ ] **Step 3: Run new test — verify it fails (target: compilation OR runtime failure)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run TestSetTimerScriptMissing -v
```

Expected: COMPILATION ERROR (`mp.setTimerErr` undefined) OR RUNTIME FAIL. Either is acceptable — the test's purpose is unmet until Steps 6-8 land. Continue regardless.

- [ ] **Step 4: Add failing test — `TestSoftTimerCapturesArgs` (popScriptArgs round-trip)**

Append to `pkg/script/handlers_timer_test.go`. The test pushes a popScriptArgs payload (type-tags + typed values) and asserts mockPlayer captures the slices.

popScriptArgs convention (from `pkg/script/handlers.go:607-634` doc-comment): pops the type-tag string FIRST, then pops typed values in tag-reverse order. So for tags="ii" with intArgs=[10, 20] (10 popped first, 20 popped second per tag-reverse iteration), the bytecode pushes 20, 10, "ii" in order; popScriptArgs pops "ii", then pops 10 into the type-tag-position-0 slot, then pops 20 into the type-tag-position-1 slot. The resulting `intArgs` slice is `[10, 20]` (index order matches type-tag order).

Verify the iteration direction by reading `pkg/script/handlers.go:626-634` doc-comment example: "[1, 'two', 3] (rightmost = top of stack at popScriptArgs entry, just below the type-tags string)". So if intArgs=[10, 20], stack order (top-to-bottom) is `tags="ii", 20, 10`. Push order is `10, 20, "ii"`.

```go
// TestSoftTimerCapturesArgs pins NAI-27 Bundle 2: SOFTTIMER pops a
// popScriptArgs payload (type-tag string + typed values) and forwards
// the resulting parallel slices to (*Player).SetTimer. Mirrors TS
// PlayerOps.ts:815-826 (popScriptArgs FIRST, then interval, then timerId).
func TestSoftTimerCapturesArgs(t *testing.T) {
	// Stack layout at OpSoftTimer (top → bottom):
	//   tags="ii"
	//   20 (intArgs[1] — popScriptArgs pops in tag-reverse order)
	//   10 (intArgs[0])
	//   3  (interval)
	//   0x12345678 (scriptID)
	sf := &ScriptFile{
		Name: "soft_timer_args",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID  (bottom of stack at OpSoftTimer)
			OpPushConstantInt,    // interval
			OpPushConstantInt,    // intArgs[0] = 10
			OpPushConstantInt,    // intArgs[1] = 20
			OpPushConstantString, // type-tags "ii"
			OpSoftTimer,
			OpReturn,
		},
		IntOperands:      []int32{0x12345678, 3, 10, 20, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "ii", "", ""},
		InstructionCount: 7,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.setTimerCalls != 1 {
		t.Fatalf("setTimerCalls: got %d, want 1", mp.setTimerCalls)
	}
	got := mp.lastSetTimer
	if got.scriptID != 0x12345678 || got.interval != 3 || got.ttype != TimerSoft {
		t.Errorf("scalars: got scriptID=%#x interval=%d ttype=%v, want 0x12345678 3 Soft", got.scriptID, got.interval, got.ttype)
	}
	if !slices.Equal(got.intArgs, []int{10, 20}) {
		t.Errorf("intArgs: got %v, want [10 20]", got.intArgs)
	}
	if len(got.stringArgs) != 0 {
		t.Errorf("stringArgs: got %v, want empty/nil", got.stringArgs)
	}
}
```

Note: this test imports `slices` (already used by `handlers_test.go` per the brainstorm-time read of `pkg/script/handlers_test.go:5`). If `handlers_timer_test.go` lacks the import, add it in Step 8 alongside the other added imports.

- [ ] **Step 5: Run new tests — verify failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run "TestSoftTimerCapturesArgs|TestSetTimerScriptMissing" -v
```

Expected: COMPILATION FAILURE or RUNTIME FAIL. Both tests depend on the implementation work in Steps 6-8.

- [ ] **Step 6: Update `(*Player).SetTimer` to add error return + script-missing check**

Edit `modules/world/player_timer.go` lines 6-21 (post-Bundle-1 shape). Replace with:

```go
// SetTimer implements script.ActivePlayer.SetTimer.
//
// NAI-27 Bundle 2: adds error return for the script-missing propagation
// pattern mirroring (*Player).EnqueueScriptArgs at player_script.go:102-118.
// When the scriptProvider chain is nil (engine-dispatch path with no
// provider configured), returns nil — preserves the no-op tolerance.
// When the provider returns nil for the scriptID, returns an error
// matching TS PlayerOps.ts:838-840 / :822-824 throw shape.
func (p *Player) SetTimer(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype script.PlayerTimerType) error {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		// Engine-dispatch tolerance — no provider configured, no script to validate.
		// Mirrors EnqueueScriptArgs guard at player_script.go:103-105.
	} else if p.client.server.scriptProvider.GetByID(scriptID) == nil {
		return fmt.Errorf("unable to find timer script: %d", scriptID)
	}
	if p.timers == nil {
		p.timers = make(map[uint32]*playerTimer)
	}
	now := 0
	if p.client != nil && p.client.server != nil {
		now = p.client.server.currentTick
	}
	p.timers[scriptID] = &playerTimer{
		ScriptID:   scriptID,
		Type:       ttype,
		Interval:   interval,
		Clock:      now,
		IntArgs:    intArgs,
		StringArgs: stringArgs,
	}
	return nil
}
```

Add the `fmt` import at the top of the file if not already imported (the file currently only imports `github.com/zsrv/goscape/pkg/script`):

```go
import (
	"fmt"

	"github.com/zsrv/goscape/pkg/script"
)
```

- [ ] **Step 7: Update `(*Player).GetTimer` — semantic flip + helper for entity-test access**

Edit `modules/world/player_timer.go` lines 33-46 (current Bundle-1-survived shape). Replace with:

```go
// GetTimer implements script.ActivePlayer.GetTimer. Returns the
// absolute tick when the timer was last set or fired (TS-faithful per
// PlayerOps.ts:858 → Player.ts:910 timer.clock semantics). Returns -1
// if no timer is registered at scriptID.
//
// NAI-27 Bundle 2: flipped from the prior "(Clock+Interval)-now"
// remaining-ticks computation, which was an untracked semantic
// divergence from TS. The new return matches what TS GETTIMER pushes.
func (p *Player) GetTimer(scriptID uint32) int {
	if p.timers == nil {
		return -1
	}
	t, ok := p.timers[scriptID]
	if !ok {
		return -1
	}
	return t.Clock
}
```

The `now` lookup is gone; the `if p.client != nil && p.client.server != nil` block is removed. The `fmt` import added in Step 6 is unaffected.

- [ ] **Step 8: Update `script.ActivePlayer.SetTimer` interface (add error return)**

Edit `pkg/script/active.go` lines 228-234 (post-Bundle-1 shape). Replace with:

```go
	// S5i: timer ops.

	// SetTimer registers a timer that re-runs the script at scriptID every
	// `interval` ticks with `intArgs`/`stringArgs` as parallel-slice typed
	// args (matching TS PlayerOps.ts:826,834 popScriptArgs convention).
	// Overwrites any existing timer at the same scriptID. type = TimerNormal
	// (waits for idle) or TimerSoft (fires while busy).
	//
	// NAI-27 Bundle 2: returns a non-nil error when the scriptID does not
	// resolve to a registered script (mirrors TS PlayerOps.ts:822-824 +
	// :838-840 throw shape). Engine-dispatch paths with no provider
	// configured are tolerant and return nil unchanged.
	SetTimer(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype PlayerTimerType) error
```

- [ ] **Step 9: Update mockPlayer.SetTimer (add error return + setTimerErr field)**

Edit `pkg/script/runner_test.go` lines 176-182 (recorder fields) and lines 413-415 (method body). Add a `setTimerErr error` field to mockPlayer's struct (alongside other test-pin fields), and update the SetTimer method to return it.

Recorder field block — add `setTimerErr error` near the other timer fields:

```go
	lastSetTimer    struct {
		scriptID   uint32
		interval   int
		intArgs    []int
		stringArgs []string
		ttype      PlayerTimerType
	}
	setTimerCalls   int
	setTimerErr     error // NAI-27 Bundle 2: pre-seed for SetTimer error return
	lastClearTimer  uint32
```

Method body update:

```go
func (m *mockPlayer) SetTimer(scriptID uint32, interval int, intArgs []int, stringArgs []string, ttype PlayerTimerType) error {
	m.lastSetTimer = struct {
		scriptID   uint32
		interval   int
		intArgs    []int
		stringArgs []string
		ttype      PlayerTimerType
	}{scriptID, interval, intArgs, stringArgs, ttype}
	m.setTimerCalls++
	return m.setTimerErr
}
```

- [ ] **Step 10: Activate `popScriptArgs` + script-missing propagation in `enqueueTimer`**

Edit `pkg/script/handlers_timer.go` lines 8-21 (post-Bundle-1 shape). Replace with:

```go
// enqueueTimer is the shared body for SETTIMER / SOFTTIMER.
//
// NAI-27 Bundle 2: activates popScriptArgs (mirrors TS PlayerOps.ts:826,834);
// activates script-missing error propagation via (*Player).SetTimer return
// (mirrors EnqueueScriptArgs pattern at modules/world/player_script.go:102-118
// for the queue family).
func enqueueTimer(s *ScriptState, ttype PlayerTimerType, op string) error {
	if err := requireActivePlayer(s, op); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	interval := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.SetTimer(scriptID, interval, intArgs, stringArgs, ttype)
}
```

This drops the inline `s.Pointers&PtrActivePlayer == 0 || s.Self == nil` gate in favor of `requireActivePlayer`, activates `popScriptArgs` as the FIRST pop (top of stack per TS), and propagates the entity-layer error via `return`.

- [ ] **Step 11: Migrate ClearTimer / ClearSoftTimer / GetTimer handlers**

Edit `pkg/script/handlers_timer.go` lines 23-48 (post-Bundle-1 shape). Replace the three handlers with:

```go
func handleClearTimer(s *ScriptState) error {
	if err := requireActivePlayer(s, "CLEARTIMER"); err != nil {
		return err
	}
	scriptID := uint32(s.PopInt())
	s.Self.ClearTimer(scriptID)
	return nil
}

func handleClearSoftTimer(s *ScriptState) error {
	if err := requireActivePlayer(s, "CLEARSOFTTIMER"); err != nil {
		return err
	}
	scriptID := uint32(s.PopInt())
	s.Self.ClearTimer(scriptID)
	return nil
}

// handleGetTimer (GETTIMER, opcode 2022) pops scriptID, validates it
// resolves to a registered script (TS PlayerOps.ts:852-854), and pushes
// the absolute clock tick (TS PlayerOps.ts:858) of the matching timer
// or -1 if no timer is registered (TS PlayerOps.ts:863).
//
// NAI-27 Bundle 2: handler-side script-missing check (vs entity-side
// for SETTIMER/SOFTTIMER) because (*Player).GetTimer returns int
// (with -1 sentinel) — pattern parallels handleGosubWithParams at
// pkg/script/handlers.go:541-554.
func handleGetTimer(s *ScriptState) error {
	if err := requireActivePlayer(s, "GETTIMER"); err != nil {
		return err
	}
	scriptID := uint32(s.PopInt())
	if s.Provider == nil {
		return fmt.Errorf("GETTIMER: Provider not set on ScriptState")
	}
	if s.Provider.GetByID(scriptID) == nil {
		return fmt.Errorf("GETTIMER: unable to find timer script: %d", scriptID)
	}
	s.PushInt(s.Self.GetTimer(scriptID))
	return nil
}
```

The `errors` import is no longer needed (all error construction now goes through `fmt.Errorf`). Verify by checking imports at the top of the file. After this step, `pkg/script/handlers_timer.go`'s import block should read:

```go
import (
	"fmt"
)
```

If `errors` is still listed, remove it. The pre-NAI-27 file imported both `errors` and `fmt`.

- [ ] **Step 12: Update `TestSetTimerCapturesArgs` to real popScriptArgs shape**

Edit `pkg/script/handlers_timer_test.go` lines 5-31 (post-Bundle-1 shape). Replace with:

```go
func TestSetTimerCapturesArgs(t *testing.T) {
	// NAI-27 Bundle 2: real popScriptArgs payload pinned. Stack layout
	// at OpSetTimer (top → bottom):
	//   tags="ii"
	//   20 (intArgs[1] — popScriptArgs pops in tag-reverse order)
	//   10 (intArgs[0])
	//   5  (interval)
	//   0x12345678 (scriptID)
	sf := &ScriptFile{
		Name: "set_timer",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID
			OpPushConstantInt,    // interval
			OpPushConstantInt,    // intArgs[0] = 10
			OpPushConstantInt,    // intArgs[1] = 20
			OpPushConstantString, // type-tags "ii"
			OpSetTimer,
			OpReturn,
		},
		IntOperands:      []int32{0x12345678, 5, 10, 20, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "ii", "", ""},
		InstructionCount: 7,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.setTimerCalls != 1 {
		t.Fatalf("setTimerCalls: got %d, want 1", mp.setTimerCalls)
	}
	got := mp.lastSetTimer
	if got.scriptID != 0x12345678 || got.interval != 5 || got.ttype != TimerNormal {
		t.Errorf("scalars: got scriptID=%#x interval=%d ttype=%v, want 0x12345678 5 Normal", got.scriptID, got.interval, got.ttype)
	}
	if !slices.Equal(got.intArgs, []int{10, 20}) {
		t.Errorf("intArgs: got %v, want [10 20]", got.intArgs)
	}
	if len(got.stringArgs) != 0 {
		t.Errorf("stringArgs: got %v, want empty/nil", got.stringArgs)
	}
}
```

Note: requires `slices` import — add it to `handlers_timer_test.go` imports (currently only imports `testing`):

```go
import (
	"errors"
	"slices"
	"strings"
	"testing"
)
```

The `errors` and `strings` imports are required by `TestSetTimerScriptMissing` (Step 2).

- [ ] **Step 13: Update `TestSoftTimerSetsSoftType` for popScriptArgs shape**

Edit `pkg/script/handlers_timer_test.go` lines 33-52 (post-Bundle-1 shape). The test now needs to push the popScriptArgs payload (empty type-tag string is valid and produces `nil, nil` per `pkg/script/handlers.go:633-634`). Replace with:

```go
func TestSoftTimerSetsSoftType(t *testing.T) {
	// NAI-27 Bundle 2: empty popScriptArgs payload (tags="") to keep this
	// test focused on the type field; full args coverage in TestSoftTimerCapturesArgs.
	sf := &ScriptFile{
		Name: "soft_timer",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID
			OpPushConstantInt,    // interval
			OpPushConstantString, // type-tags "" (empty → nil/nil from popScriptArgs)
			OpSoftTimer,
			OpReturn,
		},
		IntOperands:      []int32{0x7BCDEF00, 3, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastSetTimer.ttype != TimerSoft {
		t.Errorf("ttype: got %v, want TimerSoft", mp.lastSetTimer.ttype)
	}
}
```

- [ ] **Step 14: Update `TestGetTimer` to seed a registered script in test Provider**

Edit `pkg/script/handlers_timer_test.go` lines 74-92 (current shape, post-Bundle-1 unchanged). The handler now requires `s.Provider.GetByID(scriptID) != nil`; the test must construct a Provider seeded with a script at the queried ID. Replace with:

```go
func TestGetTimer(t *testing.T) {
	// NAI-27 Bundle 2: handler-side script-missing check requires the
	// test provider to have a script registered at the queried ID. The
	// mockPlayer.getTimerValue is now interpreted as "the absolute clock
	// the entity returned" (TS-faithful per (*Player).GetTimer flip).
	const queriedID = uint32(0x22222222)
	registered := &ScriptFile{
		Name:           "registered_timer",
		LookupKey:      queriedID,
		Opcodes:        []Opcode{OpReturn},
		IntOperands:    []int32{0},
		StringOperands: []string{""},
		InstructionCount: 1,
	}
	provider := NewProvider()
	provider.Register(registered)

	sf := &ScriptFile{
		Name: "get_timer",
		Opcodes: []Opcode{
			OpPushConstantInt, OpGetTimer, OpReturn,
		},
		IntOperands:      []int32{int32(queriedID), 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{getTimerValue: 99}
	state := Init(sf, mp, false, nil, nil)
	state.Provider = provider
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("GETTIMER push: got %d, want 99", got)
	}
}
```

Note: this test uses `NewProvider()` and `Provider.Register()`. Plan-author confirmed at preflight: `Provider.Register` is exported (per `nai_followups.md:96` claim that NAI-21 verified this). If `NewProvider`/`Register` signatures have shifted, ESCALATE.

- [ ] **Step 15: Add `TestGetTimerScriptMissing`**

Append to `pkg/script/handlers_timer_test.go`:

```go
// TestGetTimerScriptMissing pins NAI-27 Bundle 2: GETTIMER returns a
// non-nil error when the scriptID does not resolve. Mirrors TS
// PlayerOps.ts:852-854 (Unable to find timer script: ${id}).
func TestGetTimerScriptMissing(t *testing.T) {
	// Empty provider — no scripts registered.
	provider := NewProvider()

	sf := &ScriptFile{
		Name: "get_timer_missing",
		Opcodes: []Opcode{
			OpPushConstantInt, OpGetTimer, OpReturn,
		},
		IntOperands:      []int32{0xCAFEBABE, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{getTimerValue: 99}
	state := Init(sf, mp, false, nil, nil)
	state.Provider = provider
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unable to find timer script") {
		t.Errorf("error: got %v, want contains 'unable to find timer script'", err)
	}
}
```

- [ ] **Step 16: Add `TestSoftTimerScriptMissing` (parallel to TestSetTimerScriptMissing)**

Append to `pkg/script/handlers_timer_test.go`:

```go
// TestSoftTimerScriptMissing pins NAI-27 Bundle 2: SOFTTIMER returns
// the entity-layer script-missing error. Mirrors TS PlayerOps.ts:822-824.
func TestSoftTimerScriptMissing(t *testing.T) {
	sf := &ScriptFile{
		Name: "soft_timer_missing",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID (will not resolve)
			OpPushConstantInt,    // interval
			OpPushConstantString, // type-tag string for popScriptArgs (empty)
			OpSoftTimer,
			OpReturn,
		},
		IntOperands:      []int32{0xCAFEF00D, 5, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{setTimerErr: errors.New("unable to find timer script: 3405705229")}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unable to find timer script") {
		t.Errorf("error: got %v, want contains 'unable to find timer script'", err)
	}
}
```

- [ ] **Step 17: Create `modules/world/player_timer_test.go` for entity-level GetTimer pins**

Create the file with:

```go
package world

import "testing"

// TestPlayerGetTimerReturnsClock pins NAI-27 Bundle 2: GetTimer
// returns the absolute Clock tick (TS-faithful per Player.ts:910 +
// PlayerOps.ts:858), NOT the pre-NAI-27 "(Clock+Interval)-now"
// remaining-ticks computation. The negative pin: passing the test
// after a future regression to the remaining-ticks formula would
// require advance-clock < (Clock+Interval), which the assertion below
// with currentTick=25, Clock=10, Interval=10 would compute as -5
// (not 10).
func TestPlayerGetTimerReturnsClock(t *testing.T) {
	p := &Player{}
	p.timers = map[uint32]*playerTimer{
		42: {
			ScriptID: 42,
			Interval: 10,
			Clock:    10,
		},
	}
	got := p.GetTimer(42)
	if got != 10 {
		t.Errorf("GetTimer: got %d, want 10 (absolute Clock per TS Player.ts:910 / PlayerOps.ts:858; not (Clock+Interval)-now=-5 nor any other arithmetic)", got)
	}
}

// TestPlayerGetTimerNotFoundReturnsMinusOne pins the -1 sentinel for
// unset scriptIDs (TS PlayerOps.ts:863 pushInt(-1) fallthrough).
func TestPlayerGetTimerNotFoundReturnsMinusOne(t *testing.T) {
	p := &Player{}
	got := p.GetTimer(0xDEADBEEF)
	if got != -1 {
		t.Errorf("GetTimer: got %d, want -1 (no timer registered → TS PlayerOps.ts:863)", got)
	}
}

// TestPlayerGetTimerNilTimersMapReturnsMinusOne pins the nil-map fast
// path before the lookup attempt.
func TestPlayerGetTimerNilTimersMapReturnsMinusOne(t *testing.T) {
	p := &Player{}
	if p.timers != nil {
		t.Fatalf("setup: p.timers should be nil")
	}
	got := p.GetTimer(0)
	if got != -1 {
		t.Errorf("GetTimer: got %d, want -1 (nil timers map)", got)
	}
}
```

- [ ] **Step 18: Run the full test suite (must pass)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS across all packages, including the 3 new entity-level tests, the 3 new script-missing tests, the 1 new args-roundtrip test, and the updated TestGetTimer. If any package outside the touched files fails, ESCALATE.

Per memory `verify_implementer_claims`: do NOT trust IDE diagnostics or stale package-scoped caches. Run `go test ./...` (full module) and confirm the output enumerates pkg/script + modules/world + every other package green.

- [ ] **Step 19: Acceptance-criteria self-check**

```bash
git grep "Pointers&PtrActivePlayer" pkg/script/handlers_timer.go
git grep "int(s.PopInt())" pkg/script/handlers_timer.go
git grep -E "Clock \+ .*Interval" modules/world/player_timer.go
ls modules/world/player_timer_test.go
git grep -E "func.*SetTimer.*error\b" pkg/script/active.go modules/world/player_timer.go pkg/script/runner_test.go
git grep "popScriptArgs\b" pkg/script/handlers_timer.go
```

Expected:
- First three greps: ZERO results (gates migrated, dead casts removed, semantic flipped).
- `ls`: file exists.
- 5th grep: matches in all three files (interface, implementation, mock — all return error).
- 6th grep: matches in `enqueueTimer`.

If any expectation fails, ESCALATE.

- [ ] **Step 20: Commit Bundle 2**

```bash
git add modules/world/player_timer.go modules/world/player_timer_test.go pkg/script/active.go pkg/script/handlers_timer.go pkg/script/handlers_timer_test.go pkg/script/runner_test.go
git status
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,script): NAI-27 Bundle 2 — timer family TS-faithfulness audit

Per-handler line-by-line audit of 5 timer handlers vs PlayerOps.ts:820-864
+ entity-level (*Player).GetTimer flip per Player.ts:907-941.

SETTIMER/SOFTTIMER (PlayerOps.ts:815-843):
- Activate popScriptArgs (top-of-stack pop per TS line 826/834).
- Activate script-missing error via (*Player).SetTimer entity-layer
  return — mirrors NAI-26's EnqueueScriptArgs pattern at
  modules/world/player_script.go:102-118. Engine-dispatch tolerance
  preserved (nil provider chain returns nil).
- script.ActivePlayer.SetTimer interface signature gains error return.

GETTIMER (PlayerOps.ts:849-864):
- Handler-side script-missing check via s.Provider.GetByID — pattern
  parallels handleGosubWithParams at handlers.go:541-554. Routed
  handler-side because (*Player).GetTimer returns int with -1 sentinel.
- (*Player).GetTimer flipped from "(Clock+Interval)-now" remaining-ticks
  to absolute t.Clock (TS-faithful per Player.ts:910 + PlayerOps.ts:858).
  This was an untracked semantic divergence — no prior deviation tag.

CLEARTIMER/CLEARSOFTTIMER:
- Migrate inline Pointers&PtrActivePlayer gates to requireActivePlayer.
- Remove dead int(s.PopInt()) casts.

Tests added: TestSoftTimerCapturesArgs (popScriptArgs round-trip),
TestSetTimerScriptMissing + TestSoftTimerScriptMissing +
TestGetTimerScriptMissing (entity/handler-side error propagation),
modules/world/player_timer_test.go with TestPlayerGetTimerReturnsClock
+ TestPlayerGetTimerNotFoundReturnsMinusOne +
TestPlayerGetTimerNilTimersMapReturnsMinusOne (entity-level pin of
the absolute-Clock semantic).

Tests updated: TestSetTimerCapturesArgs re-pins to real popScriptArgs
shape; TestGetTimer seeds a registered script in a NewProvider() to
satisfy the new handler-side script-missing check.

mockPlayer.SetTimer signature gains error return + setTimerErr field
for test pre-seeding.

Refs: docs/superpowers/specs/2026-04-25-nai-27-timer-vararg-npc-queue-audit-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Bundle 3: Player VARARG opcode handlers + NPC queue audit memo

**Files:**
- Create: `pkg/script/handlers_player_vararg.go` (4 new handler bodies)
- Create: `pkg/script/handlers_player_vararg_test.go` (~17 new tests)
- Modify: `pkg/script/handlers.go` (4 new dispatch table entries — adjacent to existing OpStrongQueue/etc rows)
- (No-op) `pkg/script/opcode.go` — constants and String() arms ALREADY EXIST per plan-author preflight finding #1 (lines 160, 193, 218, 230 + corresponding String arms).
- (Audit-memo only) `pkg/script/handlers_npc.go:309-332` — read line-by-line vs TS NpcOps.ts:144-150; expected 0-LOC diff; record outcome in commit body.

**Pre-flight context:**
- HEAD will be at the Bundle 2 commit. Verify by `git log --oneline -1`.
- All 4 VARARG opcode constants exist:
  - `OpLongQueueVarArg = 2060` (`pkg/script/opcode.go:160`)
  - `OpQueueVarArg = 2093` (`:193`)
  - `OpStrongQueueVarArg = 2118` (`:218`)
  - `OpWeakQueueVarArg = 2130` (`:230`)
- All 4 String() arms exist (returning `"LONGQUEUE*"`, `"QUEUE*"`, `"STRONGQUEUE*"`, `"WEAKQUEUE*"`).
- The dispatch table in `pkg/script/handlers.go` does NOT contain entries for any of the 4. Grep confirmed at preflight.
- Existing fixed-arg queue handlers at `pkg/script/handlers.go:670-746` are exact templates for the script-missing-error propagation pattern (entity-layer return via `s.Self.EnqueueScriptArgs`).
- Per memory `vararg_opcode_shapes_dont_share_with_fixed_arg_siblings`: each of the 4 VARARG handlers gets its OWN body. Do NOT factor a shared helper. LONGQUEUEVARARG diverges from the other 3 via extra `logoutAction` popInt + prepended args (TS PlayerOps.ts:191 `[logoutAction, ...args]`).
- TS reference (re-read at plan-write):
  - STRONGQUEUEVARARG (`PlayerOps.ts:110-120`): `args = popScriptArgs; [scriptId, delay] = popInts(2); script = ScriptProvider.get(scriptId); if !script throw; enqueueScript(script, STRONG, delay, args)`
  - WEAKQUEUEVARARG (`:134-144`): identical to STRONG with type=WEAK
  - QUEUEVARARG (`:159-169`): identical to STRONG with type=NORMAL
  - LONGQUEUEVARARG (`:182-192`): `args = popScriptArgs; [scriptId, delay, logoutAction] = popInts(3); script = ScriptProvider.get(scriptId); if !script throw; enqueueScript(script, LONG, delay, [logoutAction, ...args])`

  In goscape pop-order (top→bottom):
  - STRONG/WEAK/QUEUE: `popScriptArgs` (top), `delay = popInt`, `scriptID = popInt`
  - LONG: `popScriptArgs` (top), `logoutAction = popInt`, `delay = popInt`, `scriptID = popInt`
  - LONG args passed to entity: `append([]int{logoutAction}, intArgs...)` (logoutAction prepended)

- NONE of the 4 VARARG variants check NumberNotNull on delay (only the fixed-arg STRONGQUEUE does, per existing `handleStrongQueue` at `handlers.go:714-725`).

- [ ] **Step 1: Pre-flight verification — Bundle 2 landed; opcode constants + dispatch table state**

```bash
git log --oneline -3
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
grep -n -E "OpStrongQueueVarArg|OpWeakQueueVarArg|OpQueueVarArg|OpLongQueueVarArg" /home/owner/Code/github.com/zsrv/goscape/pkg/script/opcode.go
grep -n -E "OpStrongQueueVarArg|OpWeakQueueVarArg|OpQueueVarArg|OpLongQueueVarArg" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go || echo "NOT WIRED — Bundle 3 work confirmed"
grep -n "handleStrongQueue\b\|handleWeakQueue\b\|handleQueue\b\|handleLongQueue\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go
ls /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_player_vararg.go 2>&1 || echo "file does not exist (expected; Bundle 3 creates it)"
```

Confirm: Bundle 2 commit on top; full test suite green; all 4 VARARG constants exist in opcode.go; dispatch table grep returns "NOT WIRED" message; `handlers_player_vararg.go` does not exist.

- [ ] **Step 2: Add failing tests for `handleStrongQueueVarArg` (round-trip + script-missing + null-delay-accept)**

Create `pkg/script/handlers_player_vararg_test.go` with the tests. Start with STRONGQUEUEVARARG; subsequent steps add the other 3 handlers' tests after each handler is implemented (so each handler's TDD cycle is self-contained).

```go
package script

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// TestStrongQueueVarArg_RoundTrip pins NAI-27 Bundle 3:
// STRONGQUEUEVARARG pops popScriptArgs (top), then delay, then scriptID,
// and enqueues a STRONG queue request with the captured args. Mirrors
// TS PlayerOps.ts:110-120 line-by-line.
func TestStrongQueueVarArg_RoundTrip(t *testing.T) {
	// Stack at OpStrongQueueVarArg (top → bottom):
	//   tags="ii"
	//   20 (intArgs[1])
	//   10 (intArgs[0])
	//   3  (delay)
	//   0xABCDEF12 (scriptID)
	sf := &ScriptFile{
		Name: "strong_vararg_rt",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID
			OpPushConstantInt,    // delay
			OpPushConstantInt,    // intArgs[0] = 10
			OpPushConstantInt,    // intArgs[1] = 20
			OpPushConstantString, // tags = "ii"
			OpStrongQueueVarArg,
			OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xABCDEF12)), 3, 10, 20, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "ii", "", ""},
		InstructionCount: 7,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.enqueueCalls != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", mp.enqueueCalls)
	}
	got := mp.lastEnqueue
	if got.scriptID != uint32(0xABCDEF12) || got.delay != 3 || got.qtype != QueueStrong {
		t.Errorf("scalars: got scriptID=%#x delay=%d qtype=%v, want 0xABCDEF12 3 Strong", got.scriptID, got.delay, got.qtype)
	}
	if !slices.Equal(got.intArgs, []int{10, 20}) {
		t.Errorf("intArgs: got %v, want [10 20]", got.intArgs)
	}
	if len(got.stringArgs) != 0 {
		t.Errorf("stringArgs: got %v, want empty/nil", got.stringArgs)
	}
}

// TestStrongQueueVarArg_ScriptMissing pins NAI-27 Bundle 3:
// STRONGQUEUEVARARG returns the entity-layer script-missing error.
// Mirrors TS PlayerOps.ts:115-117.
func TestStrongQueueVarArg_ScriptMissing(t *testing.T) {
	sf := &ScriptFile{
		Name: "strong_vararg_missing",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID (will not resolve)
			OpPushConstantInt,    // delay
			OpPushConstantString, // tags = "" (empty)
			OpStrongQueueVarArg,
			OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xDEADBEEF)), 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{enqueueErr: errors.New("unable to find queue script: 3735928559")}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unable to find queue script") {
		t.Errorf("error: got %v, want contains 'unable to find queue script'", err)
	}
}

// TestStrongQueueVarArg_AcceptsNullDelay pins NAI-27 Bundle 3 negative
// pin (per memory ts_asymmetry_dual_pin): unlike the fixed-arg
// STRONGQUEUE, STRONGQUEUEVARARG does NOT check NumberNotNull on delay.
// Pushing the null sentinel must enqueue successfully. Escalates if
// upstream TS adds NumberNotNull to STRONGQUEUEVARARG later.
func TestStrongQueueVarArg_AcceptsNullDelay(t *testing.T) {
	const nullDelay = -1 // ScriptStateNullInt sentinel; verify against checkNotNull source if drifted
	sf := &ScriptFile{
		Name: "strong_vararg_null_delay",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID
			OpPushConstantInt,    // delay = null sentinel
			OpPushConstantString, // tags = ""
			OpStrongQueueVarArg,
			OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xCAFE0001)), nullDelay, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v (want nil — STRONGQUEUEVARARG does not check NumberNotNull per TS PlayerOps.ts:110-120)", err)
	}
	if mp.lastEnqueue.delay != nullDelay {
		t.Errorf("delay: got %d, want %d (null sentinel preserved through enqueue)", mp.lastEnqueue.delay, nullDelay)
	}
}
```

Note: tests reference `mp.enqueueCalls`, `mp.lastEnqueue`, `mp.enqueueErr`, and the `QueueStrong` constant. The first three are existing mockPlayer fields per NAI-26 (verify via `grep -n "enqueueCalls\|lastEnqueue\|enqueueErr" pkg/script/runner_test.go`). The `QueueStrong` constant is in `pkg/script/queue.go:9` per the brainstorm-time read. The `nullDelay = -1` sentinel must match `checkNotNull`'s comparison value — verify by reading `pkg/script/handlers_player.go:61` for the `checkNotNull` body before assuming -1.

- [ ] **Step 3: Verify the new tests fail at the dispatch layer**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run "TestStrongQueueVarArg" -v
```

Expected: tests fail with "no handler for opcode" or similar dispatch-layer error (since `OpStrongQueueVarArg` is not in the dispatch table).

If the tests FAIL with a different error (compile error in the test file, missing field on mockPlayer, missing constant), fix the test file before proceeding. The `enqueueCalls`/`lastEnqueue`/`enqueueErr`/`QueueStrong` references must resolve.

- [ ] **Step 4: Create `pkg/script/handlers_player_vararg.go` with `handleStrongQueueVarArg`**

Create the file:

```go
package script

// handlers_player_vararg.go contains the four VARARG variant queue
// opcode handlers (STRONGQUEUEVARARG / WEAKQUEUEVARARG / QUEUEVARARG /
// LONGQUEUEVARARG). Separated from handlers_player.go (770 LOC at NAI-27
// Bundle 3 dispatch) per design-for-isolation; the file holds only
// VARARG-family handlers.
//
// All four handlers consume popScriptArgs (the top-of-stack type-tag
// string + typed values populated by the new-script bytecode prior to
// the queue opcode) and propagate script-missing errors via the
// (*Player).EnqueueScriptArgs entity-layer return. Per memory
// vararg_opcode_shapes_dont_share_with_fixed_arg_siblings, each
// handler has its own body — no shared helper. LONGQUEUEVARARG
// diverges from the other three by popping an extra logoutAction and
// prepending it to the args slice (TS PlayerOps.ts:191).

// handleStrongQueueVarArg implements STRONGQUEUEVARARG (opcode 2118):
// pop popScriptArgs (top), then delay, then scriptID, and enqueue a
// STRONG queue request. Mirrors TS PlayerOps.ts:110-120.
//
// Unlike fixed-arg STRONGQUEUE (TS PlayerOps.ts:97-108 / handler at
// pkg/script/handlers.go:714-725), STRONGQUEUEVARARG does NOT check
// NumberNotNull on delay — TS PlayerOps.ts:112 destructures both
// scriptId and delay from popInts(2) without a check wrapper.
func handleStrongQueueVarArg(s *ScriptState) error {
	if err := requireActivePlayer(s, "STRONGQUEUEVARARG"); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueStrong)
}
```

- [ ] **Step 5: Wire `OpStrongQueueVarArg` in the dispatch table**

Edit `pkg/script/handlers.go`. Locate the dispatch table (the map literal that contains `OpStrongQueue: handleStrongQueue,` etc — use `grep -n "OpStrongQueue:" pkg/script/handlers.go` to find the exact line). Add the new entry directly below the existing `OpStrongQueue` row:

```go
	OpStrongQueue:        handleStrongQueue,
	OpStrongQueueVarArg:  handleStrongQueueVarArg,
```

- [ ] **Step 6: Run STRONGQUEUEVARARG tests — must pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run "TestStrongQueueVarArg" -v
```

Expected: 3 tests PASS.

- [ ] **Step 7: Add tests + implementation for `handleWeakQueueVarArg`**

Append the 3 WEAK tests to `pkg/script/handlers_player_vararg_test.go`:

```go
// TestWeakQueueVarArg_RoundTrip pins TS PlayerOps.ts:134-144.
func TestWeakQueueVarArg_RoundTrip(t *testing.T) {
	sf := &ScriptFile{
		Name: "weak_vararg_rt",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpPushConstantString, OpWeakQueueVarArg, OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xABCD0002)), 5, 11, 22, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "ii", "", ""},
		InstructionCount: 7,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mp.lastEnqueue
	if got.scriptID != uint32(0xABCD0002) || got.delay != 5 || got.qtype != QueueWeak {
		t.Errorf("scalars: got %#x delay=%d qtype=%v, want 0xABCD0002 5 Weak", got.scriptID, got.delay, got.qtype)
	}
	if !slices.Equal(got.intArgs, []int{11, 22}) {
		t.Errorf("intArgs: got %v, want [11 22]", got.intArgs)
	}
}

// TestWeakQueueVarArg_ScriptMissing pins TS PlayerOps.ts:139-141.
func TestWeakQueueVarArg_ScriptMissing(t *testing.T) {
	sf := &ScriptFile{
		Name: "weak_vararg_missing",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantString, OpWeakQueueVarArg, OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xDEADBEE2)), 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{enqueueErr: errors.New("unable to find queue script: 3735928546")}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "unable to find queue script") {
		t.Errorf("error: got %v, want contains 'unable to find queue script'", err)
	}
}

// TestWeakQueueVarArg_AcceptsNullDelay pins TS PlayerOps.ts:134-144 (no NumberNotNull).
func TestWeakQueueVarArg_AcceptsNullDelay(t *testing.T) {
	const nullDelay = -1
	sf := &ScriptFile{
		Name: "weak_vararg_null",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantString, OpWeakQueueVarArg, OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xCAFE0002)), nullDelay, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastEnqueue.delay != nullDelay {
		t.Errorf("delay: got %d, want %d", mp.lastEnqueue.delay, nullDelay)
	}
}
```

Append the WEAK handler to `pkg/script/handlers_player_vararg.go`:

```go
// handleWeakQueueVarArg implements WEAKQUEUEVARARG (opcode 2130):
// identical structure to STRONGQUEUEVARARG with QueueWeak. Mirrors TS
// PlayerOps.ts:134-144.
func handleWeakQueueVarArg(s *ScriptState) error {
	if err := requireActivePlayer(s, "WEAKQUEUEVARARG"); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueWeak)
}
```

Add the dispatch table row in `pkg/script/handlers.go` directly below `OpWeakQueue`:

```go
	OpWeakQueue:          handleWeakQueue,
	OpWeakQueueVarArg:    handleWeakQueueVarArg,
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run "TestWeakQueueVarArg" -v` — expected 3 PASS.

- [ ] **Step 8: Add tests + implementation for `handleQueueVarArg`**

Append the 3 QUEUE tests to `pkg/script/handlers_player_vararg_test.go`:

```go
// TestQueueVarArg_RoundTrip pins TS PlayerOps.ts:159-169.
func TestQueueVarArg_RoundTrip(t *testing.T) {
	sf := &ScriptFile{
		Name: "queue_vararg_rt",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpPushConstantString, OpQueueVarArg, OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xABCD0003)), 7, 13, 26, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "ii", "", ""},
		InstructionCount: 7,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mp.lastEnqueue
	if got.scriptID != uint32(0xABCD0003) || got.delay != 7 || got.qtype != QueueNormal {
		t.Errorf("scalars: got %#x delay=%d qtype=%v, want 0xABCD0003 7 Normal", got.scriptID, got.delay, got.qtype)
	}
	if !slices.Equal(got.intArgs, []int{13, 26}) {
		t.Errorf("intArgs: got %v, want [13 26]", got.intArgs)
	}
}

// TestQueueVarArg_ScriptMissing pins TS PlayerOps.ts:163-165.
func TestQueueVarArg_ScriptMissing(t *testing.T) {
	sf := &ScriptFile{
		Name: "queue_vararg_missing",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantString, OpQueueVarArg, OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xDEADBEE3)), 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{enqueueErr: errors.New("unable to find queue script: 3735928547")}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "unable to find queue script") {
		t.Errorf("error: got %v, want contains 'unable to find queue script'", err)
	}
}

// TestQueueVarArg_AcceptsNullDelay pins TS PlayerOps.ts:159-169 (no NumberNotNull).
// Note: differs from fixed-arg QUEUE (PlayerOps.ts:148-157), which also lacks
// NumberNotNull — the asymmetry to watch for is QUEUEVARARG vs the fixed-arg
// STRONGQUEUE which DOES check.
func TestQueueVarArg_AcceptsNullDelay(t *testing.T) {
	const nullDelay = -1
	sf := &ScriptFile{
		Name: "queue_vararg_null",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantString, OpQueueVarArg, OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xCAFE0003)), nullDelay, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastEnqueue.delay != nullDelay {
		t.Errorf("delay: got %d, want %d", mp.lastEnqueue.delay, nullDelay)
	}
}
```

Append the QUEUE handler to `pkg/script/handlers_player_vararg.go`:

```go
// handleQueueVarArg implements QUEUEVARARG (opcode 2093): identical
// structure to STRONGQUEUEVARARG with QueueNormal. Mirrors TS
// PlayerOps.ts:159-169. Does NOT check NumberNotNull on delay (TS
// asymmetry — only the fixed-arg STRONGQUEUE checks).
func handleQueueVarArg(s *ScriptState) error {
	if err := requireActivePlayer(s, "QUEUEVARARG"); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.Self.EnqueueScriptArgs(scriptID, delay, intArgs, stringArgs, QueueNormal)
}
```

Add the dispatch table row directly below `OpQueue`:

```go
	OpQueue:              handleQueue,
	OpQueueVarArg:        handleQueueVarArg,
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run "TestQueueVarArg" -v` — expected 3 PASS.

- [ ] **Step 9: Add tests + implementation for `handleLongQueueVarArg` (extra logoutAction handling)**

Append the 4 LONG tests (3 standard + 1 logout-prepend pin) to `pkg/script/handlers_player_vararg_test.go`:

```go
// TestLongQueueVarArg_RoundTrip pins TS PlayerOps.ts:182-192. LONG
// diverges from the other VARARG variants by popping an extra
// logoutAction and prepending it to the args slice (TS line 191
// `[logoutAction, ...args]`).
func TestLongQueueVarArg_RoundTrip(t *testing.T) {
	// Stack at OpLongQueueVarArg (top → bottom):
	//   tags="i"
	//   55 (intArgs[0])
	//   99 (logoutAction)
	//   2  (delay)
	//   0xABCD0004 (scriptID)
	sf := &ScriptFile{
		Name: "long_vararg_rt",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID
			OpPushConstantInt,    // delay
			OpPushConstantInt,    // logoutAction
			OpPushConstantInt,    // intArgs[0]
			OpPushConstantString, // tags = "i"
			OpLongQueueVarArg,
			OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xABCD0004)), 2, 99, 55, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "i", "", ""},
		InstructionCount: 7,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mp.lastEnqueue
	if got.scriptID != uint32(0xABCD0004) || got.delay != 2 || got.qtype != QueueLong {
		t.Errorf("scalars: got %#x delay=%d qtype=%v, want 0xABCD0004 2 Long", got.scriptID, got.delay, got.qtype)
	}
	if !slices.Equal(got.intArgs, []int{99, 55}) {
		t.Errorf("intArgs: got %v, want [99 55] (logoutAction prepended per TS PlayerOps.ts:191)", got.intArgs)
	}
}

// TestLongQueueVarArg_LogoutActionPrepended pins the prepend ordering
// explicitly with multiple intArgs from popScriptArgs to distinguish
// from "wrap as single-element [logoutAction]" or "append" failure modes.
func TestLongQueueVarArg_LogoutActionPrepended(t *testing.T) {
	// popScriptArgs returns intArgs=[1, 2, 3]; logoutAction=99.
	// Expected entity-call intArgs: [99, 1, 2, 3].
	// Stack (top → bottom): tags="iii", 3, 2, 1, 99, 0, 0xCAFE0004
	sf := &ScriptFile{
		Name: "long_vararg_prepend",
		Opcodes: []Opcode{
			OpPushConstantInt,    // scriptID
			OpPushConstantInt,    // delay = 0
			OpPushConstantInt,    // logoutAction = 99
			OpPushConstantInt,    // intArgs[0] = 1
			OpPushConstantInt,    // intArgs[1] = 2
			OpPushConstantInt,    // intArgs[2] = 3
			OpPushConstantString, // tags = "iii"
			OpLongQueueVarArg,
			OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xCAFE0004)), 0, 99, 1, 2, 3, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", "iii", "", ""},
		InstructionCount: 9,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mp.lastEnqueue
	if !slices.Equal(got.intArgs, []int{99, 1, 2, 3}) {
		t.Errorf("intArgs: got %v, want [99 1 2 3] — logoutAction MUST prepend to popScriptArgs intArgs (TS line 191 `[logoutAction, ...args]`); got != [99] (single-wrap failure mode); got != [1 2 3 99] (append failure mode)", got.intArgs)
	}
}

// TestLongQueueVarArg_ScriptMissing pins TS PlayerOps.ts:187-189.
func TestLongQueueVarArg_ScriptMissing(t *testing.T) {
	sf := &ScriptFile{
		Name: "long_vararg_missing",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt, OpPushConstantString, OpLongQueueVarArg, OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xDEADBEE4)), 0, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	mp := &mockPlayer{enqueueErr: errors.New("unable to find queue script: 3735928548")}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "unable to find queue script") {
		t.Errorf("error: got %v, want contains 'unable to find queue script'", err)
	}
}

// TestLongQueueVarArg_AcceptsNullDelay pins TS PlayerOps.ts:182-192 (no NumberNotNull).
func TestLongQueueVarArg_AcceptsNullDelay(t *testing.T) {
	const nullDelay = -1
	sf := &ScriptFile{
		Name: "long_vararg_null",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt, OpPushConstantString, OpLongQueueVarArg, OpReturn,
		},
		IntOperands:      []int32{int32(uint32(0xCAFE0005)), nullDelay, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastEnqueue.delay != nullDelay {
		t.Errorf("delay: got %d, want %d", mp.lastEnqueue.delay, nullDelay)
	}
}
```

Append the LONG handler to `pkg/script/handlers_player_vararg.go`:

```go
// handleLongQueueVarArg implements LONGQUEUEVARARG (opcode 2060):
// pops popScriptArgs (top), then logoutAction, then delay, then scriptID,
// and enqueues a LONG queue request with the args slice
// `[logoutAction, ...intArgs]` (logoutAction prepended). Mirrors TS
// PlayerOps.ts:182-192 line-by-line.
//
// Per memory vararg_opcode_shapes_dont_share_with_fixed_arg_siblings,
// the extra logoutAction popInt + prepended args slice diverges from
// the other 3 VARARG handlers; this body is intentionally not shared.
func handleLongQueueVarArg(s *ScriptState) error {
	if err := requireActivePlayer(s, "LONGQUEUEVARARG"); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	logoutAction := s.PopInt()
	delay := s.PopInt()
	scriptID := uint32(s.PopInt())
	prepended := append([]int{logoutAction}, intArgs...)
	return s.Self.EnqueueScriptArgs(scriptID, delay, prepended, stringArgs, QueueLong)
}
```

Add the dispatch table row directly below `OpLongQueue`:

```go
	OpLongQueue:          handleLongQueue,
	OpLongQueueVarArg:    handleLongQueueVarArg,
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script -run "TestLongQueueVarArg" -v` — expected 4 PASS.

- [ ] **Step 10: NPC queue audit memo — line-by-line read of `handleNpcQueue` vs TS**

Read `pkg/script/handlers_npc.go:309-332` and `Engine-TS/src/engine/script/handlers/NpcOps.ts:144-150` side-by-side. Note any divergence found in this commit's body.

```bash
sed -n '309,332p' /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_npc.go
sed -n '144,150p' /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts
```

Expected outcome: NO divergence. The handler currently:
- Calls `requireActiveNpc(s, "NPC_QUEUE")` (TS uses `checkedHandler(ActiveNpc, ...)` wrapper)
- Pops `delay` and applies `checkNotNull` (TS line 145: `check(state.popInt(), NumberNotNull)`)
- Pops `arg` (TS line 146)
- Pops `queueID` and applies range check 1..20 (TS line 147 uses `QueueValid` validator — Go's explicit range check is the project's port of QueueValid)
- Computes `trigger = TriggerAiQueue1 + (queueID - 1)` (TS line 149: `ServerTriggerType.AI_QUEUE1 + queueId - 1`)
- Calls `s.ActiveNpc.EnqueueScriptForTrigger(trigger, delay, arg)` (TS line 149: `state.activeNpc.enqueueScript(trigger, delay, arg)`)

If the audit finds NO divergence, record the result in the Bundle 3 commit body (Step 12 below). If a divergence IS found, evaluate scope: small (single-line wrap) → fold into Bundle 3; structural → ESCALATE and record as a new follow-up entry.

- [ ] **Step 11: Run the full test suite (must pass)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS across all packages. The new tests in `handlers_player_vararg_test.go` should all pass (3 STRONG + 3 WEAK + 3 QUEUE + 4 LONG = 13 new tests; with the spec's "~17" estimate, the additional 4 come from the active-gate parameterization which the plan defers to: see Step 12 note).

Per memory `verify_implementer_claims`: confirm `go test ./...` enumerates pkg/script + modules/world + every other package green.

- [ ] **Step 12: Acceptance-criteria self-check**

```bash
git grep -E "OpStrongQueueVarArg|OpWeakQueueVarArg|OpQueueVarArg|OpLongQueueVarArg" pkg/script/
ls pkg/script/handlers_player_vararg.go pkg/script/handlers_player_vararg_test.go
```

Expected:
- First grep: matches in `opcode.go` (4 constants + 4 String() arms = 8 lines), `handlers.go` (4 dispatch entries), `handlers_player_vararg.go` (4 handler functions), `handlers_player_vararg_test.go` (~13 test functions). Each opcode constant should appear at least once in each of the 3 production files.
- `ls`: both new files exist.

Active-gate coverage: this plan dispatches the per-opcode gate via `requireActivePlayer` inside each handler, which is identical to the existing pattern (no parameterized table-test required for this set; the spec's "~17 tests" estimate included a parameterized table-test option that the plan replaced with embedded gate checks per handler — covered implicitly by the round-trip tests when mockPlayer is set vs when mockPlayer is `nil`). If you want explicit per-opcode active-gate coverage, add:

```go
// TestVarArgOpsRequireActivePlayer parameterized over the 4 new opcodes.
func TestVarArgOpsRequireActivePlayer(t *testing.T) {
	for _, op := range []Opcode{OpStrongQueueVarArg, OpWeakQueueVarArg, OpQueueVarArg, OpLongQueueVarArg} {
		t.Run(op.String(), func(t *testing.T) {
			pushCount := 3 // tags + delay + scriptID
			if op == OpLongQueueVarArg {
				pushCount = 4 // + logoutAction
			}
			ops := make([]Opcode, 0, pushCount+2)
			intOps := make([]int32, pushCount+2)
			strOps := make([]string, pushCount+2)
			// scriptID, delay (and logoutAction for LONG): pushed as ints
			for i := 0; i < pushCount-1; i++ {
				ops = append(ops, OpPushConstantInt)
			}
			// tags: pushed as string
			ops = append(ops, OpPushConstantString)
			ops = append(ops, op, OpReturn)
			sf := &ScriptFile{
				Name:             "no_self_" + op.String(),
				Opcodes:          ops,
				IntOperands:      intOps,
				StringOperands:   strOps,
				InstructionCount: uint32(len(ops)),
			}
			state := Init(sf, nil, false, nil, nil)
			if err := Execute(state); err == nil {
				t.Errorf("%v: want error with nil Self", op)
			}
		})
	}
}
```

Append this if you want the explicit gate coverage; otherwise skip and rely on round-trip tests' implicit coverage. Plan-author recommends including it (matches `TestTimerOpsRequireActivePlayer` precedent at handlers_timer_test.go:94).

- [ ] **Step 13: Commit Bundle 3**

```bash
git add pkg/script/handlers.go pkg/script/handlers_player_vararg.go pkg/script/handlers_player_vararg_test.go
git status
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-27 Bundle 3 — player VARARG opcode handlers + NPC queue audit memo

Wires the 4 player VARARG queue opcode handlers
(STRONGQUEUEVARARG / WEAKQUEUEVARARG / QUEUEVARARG / LONGQUEUEVARARG)
line-by-line per TS PlayerOps.ts:110-192. All 4 opcode constants and
String() arms already existed in opcode.go (lines 160, 193, 218, 230);
this commit only adds handler bodies + dispatch table entries.

Per memory vararg_opcode_shapes_dont_share_with_fixed_arg_siblings,
each handler has its own body — no shared helper. LONGQUEUEVARARG
diverges from the other 3 by popping an extra logoutAction and
prepending it to the args slice (`[logoutAction, ...args]` per TS
PlayerOps.ts:191).

NumberNotNull on delay: NONE of the 4 VARARG variants check it (only
the fixed-arg STRONGQUEUE does). The 4 negative-pin
TestX_AcceptsNullDelay tests pin this absence per memory
ts_asymmetry_dual_pin and act as escalation triggers if upstream TS
adds NumberNotNull to a VARARG variant later.

Handlers separated into new file pkg/script/handlers_player_vararg.go
(handlers_player.go is 770 LOC pre-NAI-27) per design-for-isolation.

Tests added in new file pkg/script/handlers_player_vararg_test.go:
- TestStrongQueueVarArg_RoundTrip / _ScriptMissing / _AcceptsNullDelay
- TestWeakQueueVarArg_RoundTrip / _ScriptMissing / _AcceptsNullDelay
- TestQueueVarArg_RoundTrip / _ScriptMissing / _AcceptsNullDelay
- TestLongQueueVarArg_RoundTrip / _LogoutActionPrepended /
  _ScriptMissing / _AcceptsNullDelay
- (optional) TestVarArgOpsRequireActivePlayer parameterized

NPC queue audit memo (NAI-26 Out-of-scope #3 Resolved): line-by-line
read of handleNpcQueue at pkg/script/handlers_npc.go:309-332 vs TS
NpcOps.ts:144-150 found NO divergence. Existing implementation already
matches TS faithfully: requireActiveNpc gate, NumberNotNull on delay
(NAI-20), arg popInt, queueID with 1..20 range check (Go's port of
TS's QueueValid), trigger = TriggerAiQueue1 + (queueID-1). 0-LOC diff.

Refs: docs/superpowers/specs/2026-04-25-nai-27-timer-vararg-npc-queue-audit-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — Polish + close commit

**Files:**
- Modify (if reviewer minors surfaced): targeted polish edits in any of the Bundle 1-3 touched files.
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (close-commit memory updates).

**Pre-flight context:**
- HEAD will be at the Bundle 3 commit. Verify `git log --oneline -4` shows the 3 NAI-27 bundle commits + the spec commit `0f39b81`.
- Polish commit is OPTIONAL — only commit it if reviewer-flagged minors actually exist. If no review minors land, skip directly to the close commit (Step 4).
- Per memory `close_commit_memory_trailer`: the close commit MUST include a `Closes memory:` trailer pointing at the new memory entry path.

- [ ] **Step 1: Run final full-suite test (must pass)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: PASS across all packages, including the race-detector run. If any test fails, ESCALATE — close commit must not land on a regression.

- [ ] **Step 2: (Optional) Polish commit for review minors**

If the per-bundle code review surfaced any minors (stale comments, missed dead-code cleanup, drift in test naming), apply targeted edits and commit:

```bash
git add <touched files>
git status
git commit --no-gpg-sign -m "$(cat <<'EOF'
polish(script): NAI-27 close polish — final review minors

[Description of specific minors absorbed.]

Refs: docs/superpowers/specs/2026-04-25-nai-27-timer-vararg-npc-queue-audit-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If no review minors exist, skip this step.

- [ ] **Step 3: Update `nai_followups.md` with NAI-27 close entry + NAI-26 Out-of-scope resolutions**

Find the NAI-26 entry in `nai_followups.md` and add a "Resolved 2026-04-25 (NAI-27, commits ...)" annotation under both:
- NAI-26 § Out-of-scope #3 (NPC queue audit) — Resolved by Bundle 3 audit memo (0-LOC diff).
- NAI-26 § Out-of-scope #4 (player timer family) — Resolved by Bundles 1+2.

Append a new top-level entry after the existing NAI-26 close note:

```markdown
### NAI-27 close (2026-04-25, commits <bundle1-sha> + <bundle2-sha> + <bundle3-sha>[+ <polish-sha>])

Three-bundle TS-faithfulness sub-spec resolving NAI-26 Out-of-scope #3
(NPC queue audit) + Out-of-scope #4 (player timer family) plus
introducing the 4 player VARARG opcode handlers.

**Resolutions:**

1. **playerTimer parallel-slice widening** (Bundle 1, commit
   `<bundle1-sha>`): playerTimer.IntArg int → IntArgs []int + StringArgs
   []string; Player.SetTimer + script.ActivePlayer.SetTimer interface
   widened. tick.go:292 timer-fire site forwards new fields. Mechanical
   widening with nil/nil placeholder slices. No behavior change.

2. **Timer family TS-faithfulness audit** (Bundle 2, commit
   `<bundle2-sha>`): popScriptArgs activated in SETTIMER/SOFTTIMER (TS
   PlayerOps.ts:826,834). Script-missing error activated on
   SETTIMER/SOFTTIMER (entity-layer return mirroring NAI-26's
   EnqueueScriptArgs pattern) and on GETTIMER (handler-side via
   s.Provider.GetByID, mirroring handleGosubWithParams). (*Player).GetTimer
   semantic flipped from "(Clock+Interval)-now" remaining-ticks to
   absolute t.Clock (TS-faithful per Player.ts:910 + PlayerOps.ts:858).
   Helper migrations to requireActivePlayer + dead int(s.PopInt()) cast
   removal. New entity-level test file modules/world/player_timer_test.go
   pins the absolute-Clock semantic.

3. **Player VARARG opcode family port** (Bundle 3, commit
   `<bundle3-sha>`): 4 new handler functions in new file
   pkg/script/handlers_player_vararg.go; 4 dispatch table entries in
   handlers.go. All 4 opcode constants + String() arms already existed
   in opcode.go (lines 160/193/218/230). LONGQUEUEVARARG diverges from
   the other 3 with extra logoutAction popInt + prepended args slice
   per TS PlayerOps.ts:191. None of the 4 VARARG variants check
   NumberNotNull on delay; 4 negative-pin TestX_AcceptsNullDelay tests
   guard against silent regression. Per memory
   vararg_opcode_shapes_dont_share_with_fixed_arg_siblings, each
   handler has its own body.

4. **NPC queue audit memo** (Bundle 3, same commit): line-by-line read
   of handleNpcQueue (handlers_npc.go:309-332) vs TS NpcOps.ts:144-150
   found NO divergence. Existing implementation already matches TS
   faithfully (NAI-3 + NAI-20 prior work). 0-LOC diff.

**Untracked divergence retired:** GETTIMER's "(Clock+Interval)-now"
semantic (no prior deviation tag).

**Deviations:** 0 new tags introduced. Net deviation count: 14 → 14.

**New disciplines authored:**

- _Optional_ memory entry on the GETTIMER pattern: untracked semantic
  divergences in passthrough opcodes are easy to miss because handler
  tests pass through entity values unchanged; entity-level tests are
  required to pin semantics (parallel to existing audit memories).
```

- [ ] **Step 4: Create the close commit**

```bash
git add docs/superpowers/specs/2026-04-25-nai-27-timer-vararg-npc-queue-audit-design.md  # if any spec touch-ups absorbed
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(world,script): NAI-27 closed — three-bundle follow-up

Three-bundle sub-spec resolving NAI-26 Out-of-scope #3 (NPC queue
audit) + Out-of-scope #4 (player timer family) plus introducing 4
player VARARG opcode handlers.

Bundle 1 (commit <bundle1-sha>): playerTimer parallel-slice plumbing
(mechanical, no behavior change).
Bundle 2 (commit <bundle2-sha>): timer family TS audit — popScriptArgs
+ script-missing on SETTIMER/SOFTTIMER (entity return) + GETTIMER
(handler-side); GetTimer flipped from "(Clock+Interval)-now" to
absolute t.Clock per TS PlayerOps.ts:858; helper migrations.
Bundle 3 (commit <bundle3-sha>): 4 VARARG handlers wired (constants +
String arms pre-existed); LONGQUEUEVARARG diverges per TS line 191
prepend-args; no VARARG variant checks NumberNotNull on delay
(asymmetric to fixed-arg STRONGQUEUE — pinned with 4 negative tests).
NPC queue audit memo: 0-LOC diff (TS-faithful already).

Net deviation count: 14 → 14. No new tags introduced. One untracked
semantic divergence retired (GETTIMER return-value).

Closes memory: ~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md (NAI-27 close entry)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Replace the `<bundleN-sha>` placeholders with the actual commit SHAs from `git log --oneline -5` before invoking the commit. The close commit body must NOT contain literal `<bundle1-sha>` placeholder strings.

- [ ] **Step 5: Final state verification**

```bash
git log --oneline -6
git status
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected:
- 4-5 NAI-27 commits on top of `0f39b81` (3 bundles + optional polish + close).
- Working tree clean.
- All tests PASS.

If anything is off, ESCALATE before declaring NAI-27 closed.

---

## Self-review notes (plan-author)

- **Spec coverage:** Verified each Bundle in the spec maps to a Task above. All 5 timer handlers covered in Bundle 2; all 4 VARARG handlers covered in Bundle 3; NPC queue audit covered in Bundle 3 Step 10.
- **Placeholder scan:** No "TBD"/"TODO"/etc remain in step bodies. The two `<bundleN-sha>` placeholders in Task 4 Steps 3-4 are intentional — they are filled in at execution time after the bundle commits land.
- **Type consistency:** `VarArg` (CamelCase) used for all Go identifiers — opcode constants (`OpStrongQueueVarArg` etc.) and handler function names (`handleStrongQueueVarArg` etc.). The lowercase `vararg` (one word, no separator) appears only in (a) the filename `handlers_player_vararg.go` per Go convention for compound file names, and (b) the TS opcode-table label string returned by `String()` (`"STRONGQUEUE*"` — already in opcode.go, unchanged). Verified via cross-task grep at plan-revise time.
- **Test fixture runnability:** Each bytecode fixture's `IntOperands` and `StringOperands` slice lengths match the `Opcodes` count per memory `plan_runnable_test_fixtures`. Stack-layout comments precede every multi-pop fixture.
