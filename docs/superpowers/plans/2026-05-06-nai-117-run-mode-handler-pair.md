# NAI-117 — Run-mode handler pair (P_RUN + RUNENERGY) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port two missing script-opcode handlers (P_RUN=2085, RUNENERGY=2096) line-for-line from TS `PlayerOps.ts` to silence the corresponding `no handler at <site> pc=<n>` errors surfaced at the NAI-116 close smoke.

**Architecture:** Each handler lives in `pkg/script/handlers_player.go`, dispatched from the flat `handlers` map in `pkg/script/handlers.go`. The script-side `ActivePlayer` interface (`pkg/script/active.go`) gets a new mutator (`SetRun`) and a new getter (`RunEnergy`); the world-side `*Player` impl (`modules/world/player_script.go`) and the test-side `mockPlayer` (`pkg/script/runner_test.go`) both satisfy the new methods. A new named constant `VarPlayerRun = 0` reflects the TS hard-coded `VarPlayerType.RUN` id.

**Tech Stack:** Go 1.26+, `pkg/script` (RuneScript bytecode interpreter), `modules/world` (player state).

**Spec:** `docs/superpowers/specs/2026-05-06-nai-117-run-mode-handler-pair-design.md`.

**TS source canonical path:** `LostCityRS/Engine-TS` (per `ts_source_canonical_path` memory).

---

## Task 1: P_RUN (opcode 2085) handler

**TS reference:** `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1204-1209`. Gate: `ProtectedActivePlayer`. Pops int, writes `player.run`, mirrors to `setVar(VarPlayerType.RUN, run)`.

**Files:**
- Create: (none)
- Modify:
  - `pkg/script/active.go` (add `SetRun` to `ActivePlayer` interface; add `VarPlayerRun` constant)
  - `pkg/script/handlers_player.go` (add `handlePRun`)
  - `pkg/script/handlers.go` (wire `OpPRun → handlePRun` in dispatch map)
  - `pkg/script/runner_test.go` (extend `mockPlayer`: `lastSetRun` field, `SetRun` method)
  - `pkg/script/handlers_player_test.go` (add `TestPRunDispatch`, `TestPRunUnprotectedRejected`; extend `TestHandlersRequireActivePlayer` table)
  - `modules/world/player_script.go` (add `func (p *Player) SetRun(v int)`)
- Test: above test additions in `pkg/script/handlers_player_test.go`

---

- [ ] **Step 1.1: Add `lastSetRun` field to `mockPlayer`**

Open `pkg/script/runner_test.go`. Locate the `mockPlayer` struct's BAS-setter group (around line 150-156, where `lastReadyAnim`, `lastTurnAnim`, …, `lastRunAnim` are declared). Add a new field immediately after `lastRunAnim`:

```go
		lastReadyAnim int
		lastTurnAnim  int
		lastWalkAnim  int
		lastWalkAnimB int
		lastWalkAnimL int
		lastWalkAnimR int
		lastRunAnim   int

		// NAI-117 P_RUN: most recent value passed to SetRun(v); -1 sentinel
		// distinguishes "never called" from a legitimate v=0 walk-mode write.
		lastSetRun int
```

- [ ] **Step 1.2: Add `SetRun` method to `mockPlayer`**

Locate the `SetRunAnim` mock method around line 440 (`func (m *mockPlayer) SetRunAnim(seqID int) ...`). Add immediately after:

```go
func (m *mockPlayer) SetRunAnim(seqID int)   { m.lastRunAnim = seqID }

// NAI-117 P_RUN.
func (m *mockPlayer) SetRun(v int) { m.lastSetRun = v }
```

- [ ] **Step 1.3: Add `func (p *Player) SetRun(v int)` to `modules/world/player_script.go`**

(Note: `lastSetRun = -1` sentinel is set inline by each new test that constructs `&mockPlayer{lastSetRun: -1, ...}`, not as a mock-wide default. Existing `&mockPlayer{}` constructions rely on the zero value and are unaffected.)

Locate the `SetVarp` impl (around line 317-323). Add a new method block immediately after, before the `S5c: position / facing / teleport, stats, and animation.` section header:

```go
// SetVarp implements script.ActivePlayer.SetVarp. Writes the server-
// side value then wire-sends via VARP_SMALL / VARP_LARGE if the varp
// type is transmit=true.
func (p *Player) SetVarp(id int, val int32) {
	if id < 0 || id >= len(p.varps) {
		return
	}
	p.varps[id] = val
	p.writeVarp(id, val)
}

// SetRun implements script.ActivePlayer.SetRun. Writes the run-mode
// toggle (0=walk, 1=run) to the player's run field. Mirrors TS field
// write at PlayerOps.ts:1205. Backs the P_RUN opcode handler. NAI-117.
func (p *Player) SetRun(v int) {
	p.run = v
}

// S5c: position / facing / teleport, stats, and animation.
```

- [ ] **Step 1.4: Add `SetRun` to the `ActivePlayer` interface AND add `VarPlayerRun` constant**

Open `pkg/script/active.go`. Locate the `SetRunAnim` interface entry around line 150-151. Insert a new method below `SetRunAnim` and keep the `// S5f: interface / modal control.` section header intact:

```go
	// SetRunAnim sets the player's run animation.
	SetRunAnim(seqID int)

	// SetRun writes the run-mode toggle (0=walk, 1=run) to the player.
	// Mirrors TS field write `state.activePlayer.run = state.popInt()`
	// at Engine-TS PlayerOps.ts:1205. The varp-mirror side-effect
	// (setVar(VarPlayerType.RUN, run)) remains explicit at the handler
	// call site (handlePRun), per ts_helper_method_bundles memory.
	// NAI-117.
	SetRun(value int)

	// RunEnergy returns the player's current run-energy value as an
	// int (range [0, 10000]). Mirrors TS `state.pushInt(player.runenergy)`
	// at Engine-TS PlayerOps.ts:1177. NAI-117.
	RunEnergy() int

	// S5f: interface / modal control.
```

(`RunEnergy` is added in this step so the single interface-extension build break is paired with both impls in Steps 1.5 + 1.6 immediately following. Task 2 then only adds the handler/dispatch/test for RUNENERGY without further interface churn.)

Add the `VarPlayerRun` constant at the top of `pkg/script/active.go`, immediately after the `package script` line:

```go
package script

// VarPlayerRun is the varp id for the run-mode toggle (`run` varp).
// Mirrors TS VarPlayerType.RUN = 0 at Engine-TS
// cache/config/VarPlayerType.ts:18. Consumed by the P_RUN opcode
// handler to mirror the run field into varp-id 0. NAI-117.
const VarPlayerRun = 0

// ActivePlayer is the minimal surface RuneScript needs from a Player.
```

NOTE: The build will FAIL after this step (mockPlayer + Player satisfy `SetRun` but not yet `RunEnergy`). Steps 1.5 + 1.6 fix the break before Step 1.7's compile check. Sequence: do 1.5 and 1.6 next BEFORE running anything, then continue.

- [ ] **Step 1.5: Add `RunEnergy` method + `runenergyValue` field to `mockPlayer` (paired interface-extension fix)**

In `pkg/script/runner_test.go`, immediately under the `lastSetRun` field added in Step 1.1:

```go
		// NAI-117 P_RUN: most recent value passed to SetRun(v); -1 sentinel
		// distinguishes "never called" from a legitimate v=0 walk-mode write.
		lastSetRun int

		// NAI-117 RUNENERGY: configurable return for RunEnergy(); zero default
		// is fine for tests that don't pin a specific value.
		runenergyValue int
```

Add the method immediately after the `SetRun` method from Step 1.2:

```go
// NAI-117 P_RUN.
func (m *mockPlayer) SetRun(v int) { m.lastSetRun = v }

// NAI-117 RUNENERGY.
func (m *mockPlayer) RunEnergy() int { return m.runenergyValue }
```

- [ ] **Step 1.6: Add `func (p *Player) RunEnergy() int` to `modules/world/player_script.go` (paired interface-extension fix)**

Add immediately after the `SetRun` impl from Step 1.3:

```go
// SetRun implements script.ActivePlayer.SetRun. Writes the run-mode
// toggle (0=walk, 1=run) to the player's run field. Mirrors TS field
// write at PlayerOps.ts:1205. Backs the P_RUN opcode handler. NAI-117.
func (p *Player) SetRun(v int) {
	p.run = v
}

// RunEnergy implements script.ActivePlayer.RunEnergy. Returns the
// player's current run-energy as an int (range [0, 10000]). Backs the
// RUNENERGY opcode handler. NAI-117.
func (p *Player) RunEnergy() int {
	return p.runenergy
}

// S5c: position / facing / teleport, stats, and animation.
```

- [ ] **Step 1.7: Build the whole tree to confirm the interface extension compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: exit 0, no output. Confirms `*Player` and `*mockPlayer` both satisfy the extended `ActivePlayer` interface.

- [ ] **Step 1.8: Write the failing P_RUN dispatch test (RED)**

In `pkg/script/handlers_player_test.go`, find an appropriate insertion point near the existing dispatch tests for sister opcodes. A clean spot is immediately after the `TestRunAnimAcceptsMinusOne` (around line 762). Add:

```go
// TestPRunDispatch verifies the P_RUN handler (opcode 2085) writes the
// popped int to SetRun and mirrors it to varp id VarPlayerRun. Mirrors
// TS PlayerOps.ts:1204-1209. NAI-117 T1.
func TestPRunDispatch(t *testing.T) {
	for _, v := range []int{0, 1} {
		t.Run(fmt.Sprintf("v=%d", v), func(t *testing.T) {
			mp := &mockPlayer{lastSetRun: -1, varps: map[int]int32{}}
			sf := &ScriptFile{
				Name: "p_run_dispatch",
				Opcodes: []Opcode{
					OpPushConstantInt, OpPRun, OpReturn,
				},
				IntOperands:      []int32{int32(v), 0, 0},
				StringOperands:   []string{"", "", ""},
				InstructionCount: 3,
			}
			state := Init(sf, mp, true, nil, nil) // protect=true (P_RUN gate)
			if err := Execute(state); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if mp.lastSetRun != v {
				t.Errorf("SetRun: got %d, want %d", mp.lastSetRun, v)
			}
			if got := mp.varps[VarPlayerRun]; int(got) != v {
				t.Errorf("varp[VarPlayerRun]: got %d, want %d", got, v)
			}
		})
	}
}
```

- [ ] **Step 1.9: Run the new test to confirm RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPRunDispatch -v`

Expected: FAIL. The Execute call returns an error along the lines of `Execute: no handler at op 2085 pc 1` (the dispatch loop's "unhandled opcode" path). The `t.Fatalf("Execute: %v", err)` line fires.

- [ ] **Step 1.10: Add `handlePRun` to `pkg/script/handlers_player.go`**

Locate `handlePTeleJump` (around line 592-600). Insert `handlePRun` immediately after `handlePWalk` (around line 604-609) and before the `// -- Animation ops -----` divider:

```go
// handlePWalk is a stub. Real implementation requires pathfinder +
// waypoint queue integration; pops the coord, logs, and returns nil.
func handlePWalk(s *ScriptState) error {
	_ = s.PopInt()
	slog.Debug("P_WALK stub invoked; pathfinder integration pending",
		"script", s.Script.Name, "pc", s.PC)
	return nil
}

// handlePRun implements P_RUN (opcode 2085). Pops the run-mode int and
// writes it to the player's run field, then mirrors the value to
// VarPlayerRun. Mirrors TS PlayerOps.ts:1204-1209 line-for-line.
//
// Two-step (field write + varp mirror) is intentional per
// ts_helper_method_bundles memory; TS itself flags the duplication
// with `// todo: better way to sync engine varp` (PlayerOps.ts:1207).
// Gate: ProtectedActivePlayer (TS checkedHandler).
//
// NAI-117 T1.
func handlePRun(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_RUN"); err != nil {
		return err
	}
	v := s.PopInt()
	s.Self.SetRun(v)
	// todo: better way to sync engine varp (mirrored from TS PlayerOps.ts:1207)
	s.Self.SetVarp(VarPlayerRun, int32(v))
	return nil
}

// -- Animation ops -------------------------------------------------------
```

- [ ] **Step 1.11: Wire `OpPRun → handlePRun` in `pkg/script/handlers.go`**

Locate the player-ops section around line 217-237 (where `OpPTeleport`, `OpPTeleJump`, `OpPWalk` etc. are wired). Insert immediately after `OpPWalk` and before the `S5d: config-read ops` divider:

```go
	// P_WALK stub — real impl needs pathfinder + waypoint integration.
	OpPWalk: handlePWalk,
	// NAI-117 T1: run-mode toggle (gated by ProtectedActivePlayer).
	OpPRun: handlePRun,

	// S5d: config-read ops (enum/struct/loc/npc/obj).
```

- [ ] **Step 1.12: Run the test to confirm GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPRunDispatch -v`
Expected: PASS for both `v=0` and `v=1` subtests.

- [ ] **Step 1.13: Add the unprotected-rejection test**

Sister to `TestPTeleportUnprotectedRejected` (handlers_player_test.go:1131). Insert immediately after `TestPTeleJumpUnprotectedRejected` (around line 1156):

```go
// TestPRunUnprotectedRejected verifies that a script started without
// protection gets the "script not protected" error. Mirrors TS
// checkedHandler(ProtectedActivePlayer, ...) at PlayerOps.ts:1204.
// NAI-117 T1.
func TestPRunUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_run_unprotected", OpPRun)
	state := Init(sf, mp, false, nil, nil) // protect=false
	state.PushInt(1)

	err := Execute(state)
	if err == nil || err.Error() != "P_RUN: script not protected" {
		t.Errorf("expected 'P_RUN: script not protected', got %v", err)
	}
}
```

- [ ] **Step 1.14: Run the unprotected test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPRunUnprotectedRejected -v`
Expected: PASS.

- [ ] **Step 1.15: Add `P_RUN` to the `TestHandlersRequireActivePlayer` table**

Locate the table at handlers_player_test.go:769-806. Insert immediately after the existing `RUNANIM` entry:

```go
		{"RUNANIM", OpRunAnim},
		// NAI-117 T1.
		{"P_RUN", OpPRun},
```

- [ ] **Step 1.16: Run the active-player table test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandlersRequireActivePlayer -v`
Expected: PASS, including the new `P_RUN` subtest (errors when `Self == nil`).

- [ ] **Step 1.17: Run the full test suite for regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS (all packages).

- [ ] **Step 1.18: Commit Task 1**

Stage exactly the files modified in Task 1:

```bash
git add pkg/script/active.go pkg/script/handlers_player.go pkg/script/handlers.go \
        pkg/script/runner_test.go pkg/script/handlers_player_test.go \
        modules/world/player_script.go
git status
```

Confirm only those six files are staged. Then commit:

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-117 T1 — P_RUN handler (opcode 2085)

Ports TS PlayerOps.ts:1204-1209 line-for-line. Pops int, writes to
player.run via Self.SetRun, mirrors to VarPlayerRun (id 0) via
Self.SetVarp. Gate: ProtectedActivePlayer (matches TS
checkedHandler(ProtectedActivePlayer, ...)).

Plumbing additions:
  - VarPlayerRun = 0 named constant in pkg/script/active.go (mirrors
    TS VarPlayerType.RUN at cache/config/VarPlayerType.ts:18).
  - ActivePlayer.SetRun(int) and ActivePlayer.RunEnergy() int methods
    (RunEnergy added in this commit alongside SetRun so the single
    interface-extension build-break is paired with both impls; T2
    adds the matching handler).
  - *Player.SetRun, *Player.RunEnergy in modules/world/player_script.go.
  - mockPlayer.lastSetRun (init -1 sentinel), mockPlayer.runenergyValue,
    SetRun/RunEnergy mock methods.

Tests:
  - TestPRunDispatch (v=0 and v=1 subtests; pins SetRun call AND varp
    mirror).
  - TestPRunUnprotectedRejected (sister to TestPTeleportUnprotected
    pattern).
  - P_RUN added to TestHandlersRequireActivePlayer table.

Two-step (field write + varp mirror) intentional per
ts_helper_method_bundles memory; TS comment "todo: better way to sync
engine varp" ported verbatim into handlePRun body.

Spec: docs/superpowers/specs/2026-05-06-nai-117-run-mode-handler-pair-design.md
Plan: docs/superpowers/plans/2026-05-06-nai-117-run-mode-handler-pair.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Run: `git status`
Expected: working tree clean.

---

## Task 2: RUNENERGY (opcode 2096) handler

**TS reference:** `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1175-1178`. Gate: `ActivePlayer` (no Protected requirement). Pushes `player.runenergy`.

**Files:**
- Modify:
  - `pkg/script/handlers_player.go` (add `handleRunEnergy`)
  - `pkg/script/handlers.go` (wire `OpRunEnergy → handleRunEnergy`)
  - `pkg/script/handlers_player_test.go` (add `TestRunEnergyDispatch`; extend `TestHandlersRequireActivePlayer` table)

(Interface-method `RunEnergy()` and `mockPlayer.runenergyValue` already added in Task 1 Steps 1.4/1.5/1.6.)

- Test: above test additions in `pkg/script/handlers_player_test.go`

---

- [ ] **Step 2.1: Write the failing RUNENERGY dispatch test (RED)**

Insert in `pkg/script/handlers_player_test.go` immediately after `TestPRunDispatch` (added in Step 1.8):

```go
// TestRunEnergyDispatch verifies the RUNENERGY handler (opcode 2096)
// pushes the active player's runenergy onto the int stack. Mirrors TS
// PlayerOps.ts:1175-1178. NAI-117 T2.
func TestRunEnergyDispatch(t *testing.T) {
	mp := &mockPlayer{runenergyValue: 7250}
	sf := &ScriptFile{
		Name: "runenergy_dispatch",
		Opcodes: []Opcode{
			OpRunEnergy, OpReturn,
		},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 7250 {
		t.Errorf("RUNENERGY: got %d, want 7250", got)
	}
}
```

- [ ] **Step 2.2: Run the new test to confirm RED**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestRunEnergyDispatch -v`
Expected: FAIL. The Execute call returns `no handler at op 2096 pc 0` (or similar unhandled-opcode error); `t.Fatalf("Execute: %v", err)` fires.

- [ ] **Step 2.3: Add `handleRunEnergy` to `pkg/script/handlers_player.go`**

Insert immediately after `handlePRun` (added in Step 1.10), before the `// -- Animation ops -----` divider:

```go
// handlePRun implements P_RUN (opcode 2085). [... preserved from Step 1.10 ...]
func handlePRun(s *ScriptState) error {
	// [... preserved ...]
}

// handleRunEnergy implements RUNENERGY (opcode 2096). Pushes the active
// player's current run-energy as an int (range [0, 10000]). Mirrors TS
// PlayerOps.ts:1175-1178. Gate: ActivePlayer (no Protected requirement).
//
// NAI-117 T2.
func handleRunEnergy(s *ScriptState) error {
	if err := requireActivePlayer(s, "RUNENERGY"); err != nil {
		return err
	}
	s.PushInt(s.Self.RunEnergy())
	return nil
}

// -- Animation ops -------------------------------------------------------
```

- [ ] **Step 2.4: Wire `OpRunEnergy → handleRunEnergy` in `pkg/script/handlers.go`**

Insert in the player-ops section immediately after the `OpPRun` entry (added in Step 1.11):

```go
	// P_WALK stub — real impl needs pathfinder + waypoint integration.
	OpPWalk: handlePWalk,
	// NAI-117 T1: run-mode toggle (gated by ProtectedActivePlayer).
	OpPRun: handlePRun,
	// NAI-117 T2: run-energy reader (gated by ActivePlayer).
	OpRunEnergy: handleRunEnergy,

	// S5d: config-read ops (enum/struct/loc/npc/obj).
```

- [ ] **Step 2.5: Run the test to confirm GREEN**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestRunEnergyDispatch -v`
Expected: PASS.

- [ ] **Step 2.6: Add `RUNENERGY` to the `TestHandlersRequireActivePlayer` table**

Locate the entry added in Step 1.15 and insert RUNENERGY immediately after:

```go
		{"RUNANIM", OpRunAnim},
		// NAI-117 T1.
		{"P_RUN", OpPRun},
		// NAI-117 T2.
		{"RUNENERGY", OpRunEnergy},
```

- [ ] **Step 2.7: Run the active-player table test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandlersRequireActivePlayer -v`
Expected: PASS, including the new `RUNENERGY` subtest.

- [ ] **Step 2.8: Run the full test suite for regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS (all packages).

- [ ] **Step 2.9: Commit Task 2**

Stage:

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git status
```

Confirm only those three files are staged. Commit:

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-117 T2 — RUNENERGY handler (opcode 2096)

Ports TS PlayerOps.ts:1175-1178 line-for-line. Reads
state.activePlayer.runenergy via Self.RunEnergy() (added on the
ActivePlayer interface in T1) and pushes it onto the int stack.
Gate: ActivePlayer (TS checkedHandler(ActivePlayer, ...); no
Protected requirement).

Tests:
  - TestRunEnergyDispatch (pins int-stack top == seeded
    mockPlayer.runenergyValue).
  - RUNENERGY added to TestHandlersRequireActivePlayer table.

Spec: docs/superpowers/specs/2026-05-06-nai-117-run-mode-handler-pair-design.md
Plan: docs/superpowers/plans/2026-05-06-nai-117-run-mode-handler-pair.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Run: `git status`
Expected: working tree clean.

---

## Bundle close (controller-driven, not implementer)

After T1 + T2 land and the controller verifies `git show <T1-SHA> --stat` + `git show <T2-SHA> --stat` against this plan's claimed file lists, the controller hands off the smoke binding to the user per `smoke_test_server_handoff` memory:

**Smoke gates (per spec §5.6):**

1. Tutorial Island Master Chef → exit room → tutorial step that prompts run-mode toggle. Confirm absence of `no handler at [proc,tutorial_step_enable_run] pc=6` in server log.
2. Click controls tab. Confirm absence of `no handler at [if_button,controls:com_5] pc=7` in server log.

**Final-close commit** (controller-authored, post-smoke), with `Closes memory:` trailer per `close_commit_memory_trailer` memory. Carry-forward queue: route any newly-surfaced residuals to NAI-118+ per the `smoke_surfaces_adjacent_divergences` decision tree.

---

## Self-review notes (writing-plans skill)

- **Spec coverage:** §1 (problem), §2 (TS source), §3 (HEAD verification), §4.1-4.5 (design surfaces), §5.1-5.4 (test cases) — every spec requirement maps to at least one step. §5.5 (mockPlayer extensions) covered by Steps 1.1, 1.2, 1.5. §5.6 (smoke binding) covered by the bundle-close section.
- **Placeholder scan:** clean (no TBD, TODO, "implement later", or "similar to" placeholders; every code step shows the actual code).
- **Type consistency:** `SetRun(int)` / `SetRun(value int)` / `SetRun(v int)` — interface uses `value int`, impls use `v int`; Go does not require parameter-name consistency between interface and impl, so this is fine. `RunEnergy() int` consistent across interface, impl, mock. `VarPlayerRun = 0` consistent across constant decl + handler call site + test assertion.
- **Build sequencing:** Steps 1.4 (interface extension) + 1.5 (mockPlayer.RunEnergy) + 1.6 (Player.RunEnergy) are paired so the build is restored before Step 1.7's compile check. Verified by step-by-step ordering above.
