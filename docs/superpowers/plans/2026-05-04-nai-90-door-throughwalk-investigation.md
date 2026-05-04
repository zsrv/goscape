# NAI-90: Door throughwalk investigation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land Stage 1 instrumentation (frame T at `handlePTeleport` under `NodeDebug`) so the user's β-scope smoke (RS Guide door + Survival-tutor gate) can bind the throughwalk-gap root cause to one of H1/H2/H3/H4/H5; then memorialize the binding for NAI-91.

**Architecture:** Add `NodeDebug bool` to `pkg/script/ScriptState`; emit one slog frame from `handlePTeleport` when gated; wire production callsites to read `s.cfg.NodeDebug`. No production behavioral change; new tests TDD-pin the frame fields and gate. Smoke handoff between bundles is out-of-band.

**Tech Stack:** Go 1.26+, `log/slog` (project convention; matches `handlePWalk` stub idiom), TDD per `superpowers:test-driven-development`.

**Spec:** `docs/superpowers/specs/2026-05-04-nai-90-door-throughwalk-investigation-design.md`

---

## File Structure

**Modified:**
- `pkg/script/state.go` — add `NodeDebug bool` field on `ScriptState` (after the existing optional-surface fields).
- `pkg/script/handlers_player.go` — extend `handlePTeleport` to emit frame T when gated.
- `pkg/script/handlers_player_test.go` — three new tests for frame T (gated-on, gated-off, field-values).
- `modules/world/script.go` — set `state.NodeDebug = s.cfg.NodeDebug` in `buildPlayerScriptState`.
- `modules/world/interaction_trigger.go` — set `state.NodeDebug = s.cfg.NodeDebug` at each of 6 `script.Init(...)` callsites.
- `modules/world/npc_script.go` — set `state.NodeDebug = s.cfg.NodeDebug` at the one callsite.

**Created:** None for Bundle 1.

**Bundle 2** (post-smoke, memorialize-only):
- Modified: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (append "From NAI-90" entry).
- Modified: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (re-frame or retire `door_throughwalk_gap` line as warranted).

---

## Bundle 1 — Stage 1 instrumentation

### Task 1.1: Add `NodeDebug bool` field to `ScriptState`

**Files:**
- Modify: `pkg/script/state.go` (struct definition starts at line 136)

- [ ] **Step 1: Read the current struct shape**

Read `pkg/script/state.go:136` to confirm the struct still begins with `Script *ScriptFile`. If the line number has drifted, locate `type ScriptState struct {` and proceed.

- [ ] **Step 2: Add the field**

Add the field immediately after `LineValidator LineValidator` (the last "callers set this after Init" optional-surface field, ~line 170). Use this exact block:

```go
	// NodeDebug is the per-state instrumentation gate. Production wiring
	// reads from cfg.NodeDebug at script construction sites; pkg/script
	// itself never reads cfg. Zero-value (false) preserves silence in
	// every existing &ScriptState{} test fixture without modification.
	// NAI-90: gates the handlePTeleport frame T (handlers_player.go).
	NodeDebug bool
```

- [ ] **Step 3: Verify compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean build. No tests run yet.

- [ ] **Step 4: Commit**

```bash
git add pkg/script/state.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-90 T1.1 — add ScriptState.NodeDebug instrumentation gate

Zero-value (false) preserves silence in every existing fixture.
Wiring + frame T land in T1.2 + T1.3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 1.2: TDD frame T at `handlePTeleport`

**Files:**
- Modify: `pkg/script/handlers_player.go` (handlePTeleport at ~line 543)
- Modify: `pkg/script/handlers_player_test.go` (append new tests at end of file)

This is a TDD task. Write the failing tests first, run to confirm RED, then implement the frame.

- [ ] **Step 1: Confirm slog test-handler convention by greping the existing codebase**

Run: `rg -n "slog\.SetDefault|slog.NewRecordHandler|slog.HandlerOptions|slog.New\(slog\." pkg/ modules/ --type go | head -20`
Read whichever existing test (if any) uses slog interception. If none, use the pattern in Step 2 below (a tiny in-test handler that records).

- [ ] **Step 2: Write three failing tests at the end of `pkg/script/handlers_player_test.go`**

Append (after the existing last test):

```go
// -- NAI-90 frame T tests ------------------------------------------------

// recordingHandler is a minimal slog handler that captures records for
// assertion. Used by NAI-90 frame T tests; not exported.
type recordingHandler struct {
	records []slog.Record
}

func (h *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

// installRecordingLogger swaps slog.Default for a recording handler at
// INFO level for the duration of the test. Returns the handler so the
// test can read records; restoration is automatic via t.Cleanup.
func installRecordingLogger(t *testing.T) *recordingHandler {
	t.Helper()
	prev := slog.Default()
	h := &recordingHandler{}
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

// packCoord builds the int packed coord used by P_TELEPORT.
// Matches unpackCoord at handlers_player.go:18 ((level<<28)|(x<<14)|z).
func packCoord(level, x, z int) int {
	return (level << 28) | (x << 14) | z
}

func TestPTeleport_FrameT_EmittedWhenNodeDebugTrue(t *testing.T) {
	rec := installRecordingLogger(t)
	mp := &mockPlayer{}
	s := &ScriptState{
		Script:    &ScriptFile{Name: "frame_t_emit"},
		IntStack:  make([]int, StackCapacity),
		Self:      mp,
		Protect:   true,
		NodeDebug: true,
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(packCoord(0, 3098, 3107))

	if err := handlePTeleport(s); err != nil {
		t.Fatalf("handlePTeleport: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("frame T records: got %d, want 1", len(rec.records))
	}
	if rec.records[0].Message != "p_teleport" {
		t.Errorf("frame T message: got %q, want %q", rec.records[0].Message, "p_teleport")
	}
}

func TestPTeleport_FrameT_SuppressedWhenNodeDebugFalse(t *testing.T) {
	rec := installRecordingLogger(t)
	mp := &mockPlayer{}
	s := &ScriptState{
		Script:   &ScriptFile{Name: "frame_t_silent"},
		IntStack: make([]int, StackCapacity),
		Self:     mp,
		Protect:  true,
		// NodeDebug zero-value = false
	}
	s.Pointers |= PtrActivePlayer
	s.PushInt(packCoord(0, 3098, 3107))

	if err := handlePTeleport(s); err != nil {
		t.Fatalf("handlePTeleport: %v", err)
	}

	if len(rec.records) != 0 {
		t.Errorf("frame T records under NodeDebug=false: got %d, want 0", len(rec.records))
	}
}

func TestPTeleport_FrameT_FieldValues(t *testing.T) {
	rec := installRecordingLogger(t)
	mp := &mockPlayer{coordPacked: packCoord(0, 3094, 3107)}
	s := &ScriptState{
		Script:    &ScriptFile{Name: "open_and_close_door"},
		PC:        42,
		IntStack:  make([]int, StackCapacity),
		Self:      mp,
		Protect:   true,
		NodeDebug: true,
	}
	s.Pointers |= PtrActivePlayer
	argCoord := packCoord(0, 3098, 3107)
	s.PushInt(argCoord)

	if err := handlePTeleport(s); err != nil {
		t.Fatalf("handlePTeleport: %v", err)
	}

	if len(rec.records) != 1 {
		t.Fatalf("frame T records: got %d, want 1", len(rec.records))
	}
	got := map[string]any{}
	rec.records[0].Attrs(func(a slog.Attr) bool {
		got[a.Key] = a.Value.Any()
		return true
	})

	want := map[string]any{
		"script_name":    "open_and_close_door",
		"script_pc":      int64(42),
		"self_username":  "",
		"self_coord_pre": int64(packCoord(0, 3094, 3107)),
		"arg_coord":      int64(argCoord),
		"arg_x":          int64(3098),
		"arg_z":          int64(3107),
		"arg_level":      int64(0),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("frame T field %s: got %v, want %v", k, got[k], v)
		}
	}
}
```

Add these imports to the test file's existing import block (do not duplicate already-present imports):

```go
	"context"
	"log/slog"
```

If `mockPlayer` does not yet have a `coordPacked` field or `Username()` returning "" by default, see the next step's mock-fixture sub-task.

- [ ] **Step 3: Confirm `mockPlayer` shape**

Run: `rg -n "type mockPlayer struct|func \(m \*mockPlayer\) (CoordPacked|Username)\(" pkg/script/ | head -10`

Then read the mockPlayer definition block (typically in `pkg/script/handlers_player_test.go`'s top fixture region or `pkg/script/active.go`).

If `mockPlayer.CoordPacked()` does not exist, add a `coordPacked int` field and the method:

```go
func (m *mockPlayer) CoordPacked() int { return m.coordPacked }
```

If `mockPlayer.Username()` does not exist, add a `username string` field and:

```go
func (m *mockPlayer) Username() string { return m.username }
```

If both exist, skip this step.

- [ ] **Step 4: Run the new tests — expect FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPTeleport_FrameT' -v`

Expected: at least 2 of the 3 fail (suppressed-test passes vacuously since no frame is emitted yet; emitted-test and field-values fail because the implementation doesn't emit anything).

- [ ] **Step 5: Implement frame T in `handlePTeleport`**

Read the current handlePTeleport at `pkg/script/handlers_player.go:543`. Replace its body (keeping the existing guard + pop + Self.Teleport call) with:

```go
func handlePTeleport(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_TELEPORT"); err != nil {
		return err
	}
	argCoord := s.PopInt()
	level, x, z := unpackCoord(argCoord)

	if s.NodeDebug {
		var (
			scriptName string
			selfCoord  int
			selfName   string
		)
		if s.Script != nil {
			scriptName = s.Script.Name
		}
		if s.Self != nil {
			selfCoord = s.Self.CoordPacked()
			selfName = s.Self.Username()
		}
		slog.Info("p_teleport",
			"script_name", scriptName,
			"script_pc", s.PC,
			"self_username", selfName,
			"self_coord_pre", selfCoord,
			"arg_coord", argCoord,
			"arg_x", x,
			"arg_z", z,
			"arg_level", level,
		)
	}

	s.Self.Teleport(x, z, level)
	return nil
}
```

Note: `requireProtectedActivePlayer` already proves `s.Self != nil`, so the inner `if s.Self != nil` guard is defensive against a future refactor where the gate could be loosened. Keep it.

- [ ] **Step 6: Run the new tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPTeleport_FrameT' -v`

Expected: all 3 pass.

- [ ] **Step 7: Run the full pkg/script test suite — expect green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`

Expected: PASS. Existing `TestPTeleport*` tests stay green because they construct `&ScriptState{}` with `NodeDebug` zero-value `false`.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-90 T1.2 — frame T at handlePTeleport under NodeDebug

Captures script_name, script_pc, self_username, self_coord_pre,
arg_coord (packed + unpacked level/x/z) when NodeDebug=true.
Tests pin gate-on / gate-off / field-values. Production wiring
in T1.3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 1.3: Wire `NodeDebug` at production `script.Init` callsites

Production sets `state.NodeDebug = s.cfg.NodeDebug` immediately after each `script.Init(...)` call. The `Server` receiver is in scope at every site (8 sites total). Test fixtures that call `script.Init` keep zero-value `false`; only one player-side production-test fixture exists in `script_test.go` and `handler_interface_test.go`, both of which use `script.Init` without a `*Server` (so they leave `NodeDebug=false`, which is correct for fixtures).

**Files:**
- Modify: `modules/world/script.go` (`buildPlayerScriptState` ~line 38)
- Modify: `modules/world/interaction_trigger.go` (6 callsites at ~85, 167, 354, 446, 677, 743)
- Modify: `modules/world/npc_script.go` (1 callsite at ~240)

- [ ] **Step 1: Verify the callsite list against HEAD**

Run: `rg -n "script\.Init\(" modules/world/ | grep -v _test.go`

Expected: 8 lines matching the file:line pairs above. If counts/paths differ, use the actual list — line numbers may have drifted; the grep is the source of truth.

- [ ] **Step 2: `buildPlayerScriptState` (script.go:46)**

Read `modules/world/script.go:38-76`. After the line `state := script.Init(sf, self, protect, intArgs, stringArgs)` (line 46), insert:

```go
	state.NodeDebug = s.cfg.NodeDebug
```

placed before any other `state.X = ...` assignments to keep the gate at the top of the optional-surface block.

- [ ] **Step 3: `interaction_trigger.go` — 6 sites**

For each of the 6 callsites, immediately after `state := script.Init(sf, p, true, nil, nil)` insert:

```go
	state.NodeDebug = s.cfg.NodeDebug
```

Use the existing `state.Pointers |= ...` line below as the second-line anchor (the new line goes between Init and Pointers). Verify the receiver name is `s` (the Server) — if a particular function uses a different receiver letter, use that.

- [ ] **Step 4: `npc_script.go:240`**

Read `modules/world/npc_script.go:235-245`. After `state := script.Init(sf, nil, false, intArgs, stringArgs)` insert:

```go
	state.NodeDebug = s.cfg.NodeDebug
```

- [ ] **Step 5: Build + run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green. No test changed semantics; production now sets the gate from cfg.

- [ ] **Step 6: Re-grep to confirm no missed callsites**

Run: `rg -n "script\.Init\(" modules/world/ | grep -v _test.go | wc -l`

Expected: 8.

Run: `rg -B1 -A2 "script\.Init\(" modules/world/ | grep -v _test.go | grep -A2 "script\.Init" | grep -c "NodeDebug"`

Expected: 8.

If the counts disagree, scan the diff and add `state.NodeDebug = s.cfg.NodeDebug` at any missed site.

- [ ] **Step 7: Commit**

```bash
git add modules/world/script.go modules/world/interaction_trigger.go modules/world/npc_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-90 T1.3 — wire NodeDebug to all production script.Init sites

8 sites total: buildPlayerScriptState + 6 interaction_trigger + 1 npc_script.
Test fixtures keep NodeDebug zero-value false (no fixture sweep needed).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 1.4: Smoke-handoff prep

- [ ] **Step 1: Run the full suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: green.

- [ ] **Step 2: Verify the binary builds**

Run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o /tmp/goscape-nai90 ./cmd/goscape`

Expected: clean build, no errors.

- [ ] **Step 3: Verify default config still has NodeDebug=true**

Run: `rg -n 'BoolVar.*NodeDebug.*true' modules/world/config.go`

Expected: a line like `f.BoolVar(&c.NodeDebug, "world.node-debug", true, ...)`. If the default has changed to false, flag this for the user — the smoke protocol assumes default-on.

- [ ] **Step 4: Pause for smoke handoff**

Stop here. Output a smoke-handoff prompt to the user with this exact content:

```
Stage 1 instrumentation landed (commits T1.1–T1.3). Please run the
β-scope smoke per spec §6:

1. Start server with default config (NodeDebug=true):
   CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run \
     -trimpath ./cmd/goscape --config.file config.yaml

2. Java client login at default Tutorial Island spawn.
3. Walk to RS Guide door (loc_3014, ~3098,3107,0); click once;
   note player position post-tick.
4. If reachable, walk to Survival-tutor gate (loc_3015); click once;
   note position post-tick.
5. Capture goscape.log and attach the click ticks (frame A from
   handleOpLoc + frame T from handlePTeleport).

Paste the log + observation report here. Bundle 2 (memorialize)
follows the smoke binding.
```

Do **not** proceed to Bundle 2 until the user provides smoke evidence.

---

## Bundle 2 — Memorialize

This bundle runs *after* the user attaches the smoke log + observation report. The exact tasks depend on which hypothesis binds (per spec §4 routing rules).

### Task 2.1: Apply routing rules to bind a hypothesis

- [ ] **Step 1: Read smoke evidence**

The user pastes the smoke log + observation report. Identify:

- Frame A records (search log for `"oploc"` or `"interaction frame A"` matching message string used by `interaction_debug.go`; if uncertain about the exact message, run `rg -n "slog\." modules/world/interaction_debug.go` to confirm the message constants).
- Frame T records (search log for `"p_teleport"` message).
- Java-client position observations (door-click pre/post; gate-click pre/post).

- [ ] **Step 2: Apply spec §4 routing**

For each captured frame pair, fill this routing table:

| Smoke target | Frame A `player_coord_post_step` | Frame T `arg_coord` | Java post-click position |
|---|---|---|---|
| RS Guide door (loc_3014) | (fill) | (fill, or "no frame T") | (fill) |
| Survival-tutor gate (loc_3015) | (fill) | (fill) | (fill) |

Match against H1/H2/H3/H4/H5 from spec §4:
- Both frames A show `== loc_coord`, both frame T `arg_coord == loc_coord` for the door → **H1**
- Door A `!= loc_coord`, gate A `!= loc_coord`, only door T `arg_coord == loc_coord` → **H2**
- All frames look correct but Java post-click position wrong → **H3**
- Frame T never fires for door → **H4**
- Door throughwalk works in smoke → **H5**

- [ ] **Step 3: Document the binding**

Note (in scratch, for the next step) the bound hypothesis number and one log excerpt per relevant frame as binding evidence.

---

### Task 2.2: Write `nai_followups.md` "From NAI-90" entry

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

- [ ] **Step 1: Read current `nai_followups.md`**

Read the file. Confirm it follows the existing "From NAI-N" section convention.

- [ ] **Step 2: Append the NAI-90 section**

Add a new section at the bottom, of this shape (substitute the bound hypothesis):

```markdown
## From NAI-90

**Binding:** H{N} — {hypothesis name from spec §4}.

**Evidence (smoke {date}):**
- Frame A door: {one-line excerpt}
- Frame T door: {one-line excerpt or "never fired"}
- Frame A gate: {one-line excerpt}
- Frame T gate: {one-line excerpt}
- Java-client observation: {door post-click coord, gate post-click coord}

**NAI-91 scope:** {per spec §4 binding row, e.g. "full TS PathingEntity.pathToTarget port (closes pathfinder_api_loc_aware)" for H1, or "proc-execution / multi-return arg-stack debug" for H2, etc.}

**Files in NAI-91 scope:** {derived from binding — e.g. modules/world/interaction.go pathToTarget + pkg/pathfinder/routefinder/api.go for H1; pkg/script/runner.go gosub frame for H2}.
```

- [ ] **Step 3: Re-frame or retire `door_throughwalk_gap` in MEMORY.md**

Read `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` for the line:

```
- [Door interaction missing throughwalk player-step](door_throughwalk_gap.md) — TS door _open does change_loc + walk player through; goscape implements only change_loc, leaving player stuck on doorway tile post-revert (NAI-89 smoke surfaced; NAI-90 candidate)
```

Action depends on binding:

- **H5 (over-attribution):** Delete the MEMORY.md line; delete `door_throughwalk_gap.md`. Add a new memory entry capturing whatever the smoke *did* reveal (if non-derivable).
- **H1/H2/H3/H4:** Edit the line to point at the actual bound root cause. Replace with:

```
- [Door interaction missing throughwalk player-step](door_throughwalk_gap.md) — bound to H{N} ({name}) at NAI-90 close; NAI-91 ports {scope}; original NAI-89 framing was content-side, not engine-side
```

Edit `door_throughwalk_gap.md` body to match — replace the engine-side framing with the bound root cause.

---

### Task 2.3: Author NAI-91 paste-ready resume prompt

- [ ] **Step 1: Compose resume prompt**

Compose a prompt of this shape (per `post_task_handoff` memory):

```
NAI-91 brainstorm. {Hypothesis-bound title — e.g. "Full TS PathingEntity.pathToTarget port" for H1}.

Bound by NAI-90 smoke ({date}, commit {SHA of NAI-90 close}). Spec
location: docs/superpowers/specs/2026-05-04-nai-90-door-throughwalk-investigation-design.md
§4 H{N}. nai_followups "From NAI-90" entry has full smoke evidence.

Scope: {per spec §4 binding-row scope ceiling}.

Per `runescript_cadence` memory: brainstorm → spec → plan → subagent-
driven TDD with two-stage review.

Per `session_context_management`: this is a natural fresh-session
boundary; recommend `/clear` before starting NAI-91 brainstorm.
```

- [ ] **Step 2: Output the resume prompt to the user**

Print the composed prompt verbatim, ready for the user to paste into a fresh session.

---

### Task 2.4: Close commit

- [ ] **Step 1: Stage memory files**

The `~/.claude/...` memory directory is outside the repo, so the close commit covers only the repo-side artifacts (the spec + plan are already committed). The close commit is therefore a chore commit summarizing the binding without code changes.

Run: `git status`
Expected: clean working tree (memory edits don't touch the repo).

- [ ] **Step 2: Empty close commit with memory trailer**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-90 — door throughwalk investigation [H{N} binding]

Stage 1 instrumentation (T1.1–T1.3) bound the throughwalk gap to
H{N} ({name}) via β-scope smoke. Memorialized at nai_followups
"From NAI-90"; NAI-91 ships the conditional fix.

Closes memory: door_throughwalk_gap (re-framed to H{N} root cause)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Substitute `{N}` and `{name}` with the bound hypothesis. If H5 bound, the trailer reads `Closes memory: door_throughwalk_gap (retired — over-attribution)` and the body notes that no NAI-91 follows.

- [ ] **Step 3: Verify**

Run: `git log -1 --format="%H %s"`
Expected: the close commit HEAD.

---

## Self-Review

**Spec coverage:**
- Spec §3 cadence: B1 instrumentation = T1.1–T1.4. Smoke handoff = T1.4 step 4. B2 memorialize = T2.1–T2.4. ✓
- Spec §4 hypotheses: T2.1 step 2 routing table; T2.2 step 2 binding section. ✓
- Spec §5.1 frame T fields: T1.2 step 5 implementation + T1.2 step 2 field-values test. ✓
- Spec §5.2 NodeDebug field: T1.1 + T1.3. ✓
- Spec §5.3 logger plumbing (option B, slog.Default()): T1.2 step 5 uses `slog.Info(...)` per default-logger pattern. ✓
- Spec §5.4 three tests: T1.2 step 2. ✓
- Spec §6 smoke protocol: T1.4 step 4 handoff prompt. ✓
- Spec §7 memorialize: T2.1–T2.3. ✓
- Spec §8 no new deviations: no production behavioral change in B1; T1.3 only sets a gate. ✓
- Spec §9 risks R1–R5: R1 (fixture sweep) addressed by zero-value preservation T1.1. R2 (smoke unreachable) handled in T2.1 step 2 routing. R3 (frame T fires for unrelated p_teleport) acceptable per spec. R4 (slog.Default fragility) addressed by `t.Cleanup` in T1.2 helper. R5 (memory over-attribution) handled in T2.2 step 3. ✓

**Placeholder scan:** No "TBD", "TODO", "implement later". Bundle 2 has hypothesis-conditional content (e.g., `{N}`, `{name}`) but each substitution is mechanically derived from the smoke binding — not a deferred-detail placeholder.

**Type consistency:** `NodeDebug bool` field used identically in T1.1 (definition), T1.2 step 2 (test fixture), T1.2 step 5 (handler read), T1.3 (production wiring). `frame T` message string `"p_teleport"` matches across T1.2 step 2 (assertion) and T1.2 step 5 (emission). Field keys `script_name / script_pc / self_username / self_coord_pre / arg_coord / arg_x / arg_z / arg_level` appear identically in test (T1.2 step 2) and impl (T1.2 step 5).

**Identified gap:** `mockPlayer.coordPacked` and `mockPlayer.username` may not exist; T1.2 step 3 grep+conditional-add handles this as a sub-task rather than assuming HEAD shape.
