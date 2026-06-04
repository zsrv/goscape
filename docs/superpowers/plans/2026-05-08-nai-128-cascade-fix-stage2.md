# NAI-128 Stage 2 — Cascade-fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `TestNAI128_RatLootCascade/GroundObjs` GREEN, add `T6 CascadeDispatchTrace` as a permanent diagnostic regression gate, and confirm Java-client smoke shows rat loot drops in Lumbridge.

**Architecture:** Phase A patches `nai128CacheFixture` to mirror production `NewServer` view init (worldVars/configsView/invLookup/npcLookup + vars/varsStrings backing slices). Phase B re-runs T5; if green, fixture parity was the only obstacle and the plan jumps to smoke. If T5 still fails, Phase C adds T6 (recording slog + GetByTrigger probe) which binds to E0/E2a/E2b and routes to a matching production fix.

**Tech Stack:** Go 1.26+, `log/slog`, no new deps. Reuses existing `capturingHandler` from `modules/world/interaction_debug_test.go:64-89`.

**Spec:** `docs/superpowers/specs/2026-05-08-nai-128-cascade-fix-stage2-design.md` (`350a982`).

---

## Phase A — Fixture parity (always runs)

### Task 1: Patch `nai128CacheFixture` to mirror `NewServer` view init

**Why:** `nai128CacheFixture` zero-initializes `s.worldVars`, `s.configsView`, `s.invLookup`, `s.npcLookup`. With `worldVarsView{s: nil}`, `LookupPlayerByUID` short-circuits at `server_varp.go:178-180` regardless of `playerLoop` state — `NPC_FINDHERO` returns 0 silently and both `obj_add` calls are gated out. This explains the T5 failure as fixture noise. Production `NewServer` (`server.go:236-252`) wires these views before tick-loop start; the fixture must do the same before `processNpcQueue` runs.

**Files:**
- Modify: `modules/world/nai128_rat_loot_test.go:18-81`

- [ ] **Step 1: Read current fixture and confirm scope of edit**

Run: `sed -n '18,81p' modules/world/nai128_rat_loot_test.go`
Expected: see existing fixture loading locTypes/gamemap/objTypes/params/npcTypes/varpTypes/scriptProvider but no `worldVars`/`configsView`/`invLookup`/`npcLookup` assignment, no `vars`/`varsStrings` slice malloc, no `varsTypes`/`varnTypes` load.

- [ ] **Step 2: Edit `nai128CacheFixture` — add Vars/Varn loads + slice malloc + view inits**

Replace the block from the `// VarpTypes — for any varp reads inside the cascade.` comment through the line `s.varpTypes = varpTypes` (currently `nai128_rat_loot_test.go:66-71`) with:

```go
	// VarpTypes — for any varp reads inside the cascade.
	varpTypes, err := objtype.LoadVarpTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadVarpTypes: %v", err)
	}
	s.varpTypes = varpTypes

	// VarsTypes / VarnTypes — required to size s.vars and s.varsStrings
	// (mirrors NewServer at server.go:225-237). worldVarsView.VarsInt /
	// SetVarsInt key into these slices; with empty slices any cascade-side
	// varp read returns 0 silently rather than panicking on a nil-server
	// short-circuit.
	varsTypes, err := objtype.LoadVarsTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadVarsTypes: %v", err)
	}
	s.varsTypes = varsTypes
	varnTypes, err := objtype.LoadVarnTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadVarnTypes: %v", err)
	}
	s.varnTypes = varnTypes
	s.vars = make([]int32, len(varsTypes.Configs))
	s.varsStrings = make([]string, len(varsTypes.Configs))

	// Script-side adapter views — wire after cache types are set so the
	// inner-server pointer references a fixture in its final shape. Mirrors
	// NewServer at server.go:238, 250-252. Without these, ScriptState.World
	// is worldVarsView{s: nil} and LookupPlayerByUID short-circuits to nil
	// at server_varp.go:178-180 — silently breaks NPC_FINDHERO even though
	// playerLoop is populated.
	s.worldVars = worldVarsView{s: s}
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
```

- [ ] **Step 3: Run `go vet` to confirm no symbol typos**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...`
Expected: clean exit.

- [ ] **Step 4: Run T5 to capture Phase B outcome (decision point)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v 2>&1 | tail -60`

Two possible outcomes — record verbatim in commit message:

- **`--- PASS: TestNAI128_RatLootCascade/GroundObjs`** → Phase B succeeded; fixture parity was the only obstacle. Skip Phase C entirely. Proceed to Task 8 (commit) → Task 14 (close + smoke handoff).
- **`--- FAIL: TestNAI128_RatLootCascade/GroundObjs`** → Phase B failed; production bug confirmed. Proceed through Tasks 2-7 in Phase C.

Capture full failure output if T5 still red — paste into commit message body.

- [ ] **Step 5: Commit Phase A**

```bash
git add modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(nai-128): Phase A — patch nai128CacheFixture to mirror NewServer view init

Adds varsTypes/varnTypes loads + s.vars/s.varsStrings slice mallocs +
worldVars/configsView/invLookup/npcLookup view inits. Mirrors production
NewServer at server.go:236-252.

Without these, ScriptState.World is worldVarsView{s: nil} and
LookupPlayerByUID short-circuits to nil at server_varp.go:178-180,
making NPC_FINDHERO return 0 silently regardless of playerLoop state.

Phase B (T5 re-run): [PASTE outcome — PASS or FAIL with first 30 lines
of failure output here].

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase C — T6 probe + fix (conditional on Phase B failure)

**Skip this entire phase if Task 1 Step 4 reported T5 PASS.** Jump to Task 8.

### Task 2: Add `CascadeDispatchTrace` (T6) subtest with recording slog handler

**Why:** With fixture parity in place, T5 still failing means a real production bug. T6 disambiguates between three sub-candidates by capturing the production cascade's `s.log.Warn` output (currently swallowed by `discardLogger`) plus a static `GetByTrigger` probe. Reuses the existing `capturingHandler` at `modules/world/interaction_debug_test.go:64-89` (same package, no import needed).

**Files:**
- Modify: `modules/world/nai128_rat_loot_test.go:85-284` (test body — add recorder setup + new subtest)

- [ ] **Step 1: Add recorder setup before `t.Run("AiQueueCascade", …)` and add `slog`/`context` imports if missing**

First check imports — add `"context"` and `"log/slog"` to the test file's import block if not already present:

Run: `head -12 modules/world/nai128_rat_loot_test.go`
Expected: existing import block visible. Edit the import block to include `"context"` and `"log/slog"` (alongside existing `"os"`, `"path/filepath"`, `"testing"` plus the project imports).

Then locate the line `s.processNpcQueue(rat)` inside `t.Run("AiQueueCascade", …)` (currently `nai128_rat_loot_test.go:186`). Just before the `t.Run("AiQueueCascade", …)` block, insert:

```go
	// T6 setup: replace s.log with a recording handler so the cascade's
	// resumeOrFinishNpc warn-level "npc script execute error" frames are
	// captured (production discardLogger swallows them). Recorder is
	// inspected in T6 below.
	rec := &capturingHandler{}
	s.log = slog.New(rec)
```

- [ ] **Step 2: Add T6 subtest immediately after `AiQueueCascade` and before `GroundObjs`**

Insert between the closing `}` of `t.Run("AiQueueCascade", …)` and the `t.Run("GroundObjs", …)` opening (currently `nai128_rat_loot_test.go:204-206`):

```go
	t.Run("CascadeDispatchTrace", func(t *testing.T) {
		// Static probe: is the rat-specific ai_queue3 script even
		// resolvable via the trigger lookup? processNpcQueue silently
		// `continue`s on nil at npc_script.go:518 — no log, no error.
		// nil → E0 bound (provider lookup gap).
		sf := s.scriptProvider.GetByTrigger(script.TriggerAiQueue3, ratTypeID, ratType.Category)
		if sf == nil {
			t.Errorf("scriptProvider.GetByTrigger(TriggerAiQueue3, %d, %d) = nil; binding candidate E0: provider lookup miss for [ai_queue3,newbiegiantrat]",
				ratTypeID, ratType.Category)
		} else {
			t.Logf("ai_queue3 script resolved: %q", sf.Name)
		}

		// Recorder readout: any warn-level "npc script execute error"
		// frames identify a script.Execute error during cascade. The err
		// attr names the failing opcode.
		records := rec.snapshot()
		var execErrors []slog.Record
		for _, r := range records {
			if r.Level == slog.LevelWarn && r.Message == "npc script execute error" {
				execErrors = append(execErrors, r)
			}
		}
		if len(execErrors) > 0 {
			for i, r := range execErrors {
				var script, errStr string
				r.Attrs(func(a slog.Attr) bool {
					switch a.Key {
					case "script":
						script = a.Value.String()
					case "err":
						errStr = a.Value.String()
					}
					return true
				})
				t.Errorf("warn[%d] script=%q err=%q; binding candidate E2b: cascade script errored",
					i, script, errStr)
			}
		}

		// Diagnostic dump of all captured log frames — useful when the
		// binding is none of E0/E2a/E2b (e.g. an unexpected error path).
		if t.Failed() || testing.Verbose() {
			for i, r := range records {
				t.Logf("log[%d] level=%v msg=%q", i, r.Level, r.Message)
			}
		}

		// Note: E2a (NPC_FINDHERO returning 0 silently) leaves both
		// assertions above passing while T5 still reports 0 objs. The
		// E2a binding is inferred from "T6 PASS + T5 FAIL". After fix,
		// T6's two positive contracts (sf!=nil; execErrors empty) are
		// permanent regression gates.

		_ = context.Background() // keep "context" import live; Handle takes ctx
	})
```

(The trailing `_ = context.Background()` keeps the `"context"` import referenced — remove it if some other code in the file already references the package.)

- [ ] **Step 3: Re-run the test to capture T6 binding output**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v 2>&1 | tail -120`

Read the output and **record the binding outcome**:

| Observed | Bound | Next |
|---|---|---|
| `T6 CascadeDispatchTrace` reports `GetByTrigger ... = nil` | **E0** | Task 3 |
| `T6` reports `warn[N] script=... err=...` lines | **E2b** | Task 4 (use err string) |
| `T6` PASSES but `GroundObjs` still FAILS | **E2a** | Task 5 |

Paste full T6 output into the next commit message.

- [ ] **Step 4: Commit T6 probe + binding finding**

```bash
git add modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(nai-128): Phase C — T6 CascadeDispatchTrace probe; binding [E0|E2a|E2b]

Recorder setup before AiQueueCascade injects a slog handler into s.log;
new subtest reads back warn-level "npc script execute error" frames and
runs a static GetByTrigger probe for [ai_queue3,newbiegiantrat].

Observed binding: [PASTE: full T6 output, including resolved script
name OR err strings OR "T6 PASS + T5 FAIL"].

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Fix branch E0 — provider lookup miss

**Skip if T6 binding was E2a or E2b.**

**Why:** `GetByTrigger(TriggerAiQueue3, ratTypeID, ratType.Category)` returned nil despite a registered `[ai_queue3,newbiegiantrat]` script. Either the trigger ID constant in `pkg/script/trigger.go` mismatches the cache-side encoding for AI_QUEUE3, or the type/category lookup keys in `pkg/script/provider.go` miss the wildcard fallthrough.

**Files:**
- Investigate first: `pkg/script/provider.go:114-160`, `pkg/script/trigger.go`, the cached script's encoded trigger ID (read via a one-off `cache/script.dat` decode or by inspecting `provider.byTrigger` map keys at runtime).
- Modify (TBD by investigation): likely `pkg/script/provider.go` lookup-key construction.

- [ ] **Step 1: Inspect provider's trigger map for the rat script**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | grep -A2 "ai_queue3"` to see what (if anything) was resolved.

Add a one-off diagnostic at T6 time (revert before commit): list every script in `provider` whose name contains `"ai_queue3"`:

```go
// TEMPORARY — remove before commit:
for name, sf := range s.scriptProvider.AllByName() { // adjust to actual accessor
    if strings.Contains(name, "ai_queue3") {
        t.Logf("provider has %q -> trigger=%v typeID=%d categoryID=%d",
            name, sf.Trigger, sf.TypeID, sf.CategoryID)
    }
}
```

Note: replace the accessor with whatever exists; if no public iterator, add a temporary one in `pkg/script/provider.go` for diagnosis only.

Run, observe, then **stop and revise this task** with concrete fix code based on the actual lookup-key mismatch found. Do not synthesize a fix from assumption — read the bug.

- [ ] **Step 2: Apply the fix identified in Step 1**

(Fill in once Step 1 surfaces the actual bug. Likely shape: a lookup-key construction or trigger constant correction in `pkg/script/provider.go` or `pkg/script/trigger.go`.)

- [ ] **Step 3: Re-run TestNAI128_RatLootCascade**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -40`
Expected: all subtests PASS, including `GroundObjs` (2 objs at rat coord).

- [ ] **Step 4: Run full provider tests to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 5: Commit fix**

```bash
git add pkg/script/
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(nai-128): E0 — [DESCRIBE provider lookup fix]

[Body: explain what the lookup-key bug was, citing the actual diff.]

Closes T5 GroundObjs + T6 CascadeDispatchTrace.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Then jump to Task 6 (T6 positive-contract flip).

---

### Task 4: Fix branch E2b — script execution error

**Skip if T6 binding was E0 or E2a.**

**Why:** A warn-level frame named the failing opcode in T6 output. Common shapes:

| err string | Root cause | Fix locus |
|---|---|---|
| `OBJ_ADD: no active player` | `NPC_FINDHERO` set `Self2` instead of `Self` (IntOperands[PC] non-zero unexpectedly) | `pkg/script/handlers_npc.go:1123-1129` |
| `OBJ_ADD: no ObjType with value (N)` | `serverConfigsView.ObjType(N)` returns nil for a registered objId | `modules/world/server_varp.go` (or wherever `serverConfigsView` is defined); `pkg/script/handlers_obj.go:71` |
| `OBJ_ADD: invalid coord` | `checkCoord` rejects the packed `npc_coord` value | `pkg/script/handlers_obj.go:63`, `pkg/coordgrid` packer |
| `OBJ_ADD: invalid count (N)` | `count == -1` slipped past the early `objId == -1 \|\| count == -1` guard | `pkg/script/handlers_obj.go:50-56` |
| `OBJ_ADD: attempted to add dummy item` | raw_rat_meat objType has `DummyItem != 0` (cache decode bug) | `pkg/objtype/objtype.go` |
| `(other)` | inspect handler at the named opcode | per opcode |

**Files:** depend on err string — fill in after Task 2 binding. **Do not write fix code without first matching the err string to one of the rows above.**

- [ ] **Step 1: Identify root cause from err string**

Re-read the T6 commit body (Task 2 Step 4). Match err string to the table above and route to the appropriate file. If err shape is novel, investigate the named handler.

- [ ] **Step 2: Write a unit test in `pkg/script` (or appropriate package) that pins the bug**

Place a focused unit test in the relevant `*_test.go` exercising the failing opcode in isolation. Do NOT wait for the integration test alone — sub-system unit test gives a faster regression gate.

(Fill in concrete test code once root cause is identified.)

- [ ] **Step 3: Verify unit test FAILS at HEAD**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '<TestName>' ./pkg/script/... -v -count=1`
Expected: FAIL with the specific err message.

- [ ] **Step 4: Apply fix**

(Fill in concrete fix once root cause is identified.)

- [ ] **Step 5: Re-run unit test + integration test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '<TestName>' ./pkg/script/... -v -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1
```
Expected: both PASS.

- [ ] **Step 6: Run package-level + cross-package regression sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 7: Commit fix**

```bash
git add pkg/script/ modules/world/
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(nai-128): E2b — [opcode] [DESCRIBE fix]

[Body: cite err string from T6, name the bug, summarize the diff.]

Closes T5 GroundObjs + T6 CascadeDispatchTrace.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Then jump to Task 6.

---

### Task 5: Fix branch E2a — silent NPC_FINDHERO=0

**Skip if T6 binding was E0 or E2b.**

**Why:** T6 PASSES (`sf != nil`, no warn frames) but `GroundObjs` still FAILS — both `obj_add` calls were gated out by `NPC_FINDHERO` returning 0 silently. With Phase A in place, the only paths to 0 are:
1. `state.World == nil` — impossible after Phase A (interface holds non-zero `worldVarsView{s: s}`).
2. `s.ActiveNpc.TopContributor() == 0` — heroPoints ledger cleared between probe Preconditions and ai_queue3 firing.
3. `s.World.LookupPlayerByUID(uid) == nil` — uid composition mismatch between `addPlayer` (`server.go:696`) and `TopContributor`'s recorded uid.

**Files:** depend on which path — investigate first.

- [ ] **Step 1: Add diagnostic logging to T6 to identify the failing path**

Augment T6 with a direct introspection of the cascade-time state. Insert before the recorder readout:

```go
		// E2a diagnosis: recompute the three NPC_FINDHERO gates against
		// post-cascade state. Prints which gate would have failed if
		// NPC_FINDHERO ran here.
		topUID := rat.heroPoints.TopContributor()
		t.Logf("E2a probe: rat.heroPoints.TopContributor() post-cascade = %d (player.UID=%d)", topUID, p.UID())
		if topUID != 0 {
			lookedUp := s.LookupPlayerByUID(topUID)
			t.Logf("E2a probe: s.LookupPlayerByUID(%d) = %v (nil iff lookup miss)", topUID, lookedUp)
		}
```

- [ ] **Step 2: Re-run and read output**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -40`

Match output to one of:

- `topUID = 0` → ledger cleared during cascade. Investigate `Npc.heroPoints` mutation in `~npc_default_damage` or `processNpcQueue`. Fix locus: `pkg/entity/npc.go` or `modules/world/npc_script.go`.
- `topUID != 0` and `LookupPlayerByUID(...) = <nil>` → uid composition mismatch. Inspect `addPlayer`'s `composeUID(p.username37, p.slot)` (`server.go:696`) vs whatever `heroPoints.AddHero` keyed in. Fix locus: probe-side (use the actual composed uid for AddHero) or production (composeUID semantics).
- `topUID != 0` and `LookupPlayerByUID(...) != <nil>` → contradiction; NPC_FINDHERO should have returned 1. Re-investigate `handleNpcFindHero` IntOperand handling at `pkg/script/handlers_npc.go:1123-1129`.

- [ ] **Step 3: Apply fix based on diagnostic outcome**

(Fill in concrete fix once root cause is identified.)

- [ ] **Step 4: Re-run integration test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -40`
Expected: all subtests PASS.

- [ ] **Step 5: Remove the temporary E2a diagnostic logging from T6** (added in Step 1)

The diagnostic was for binding only; T6's permanent contract is the GetByTrigger + warn-frame check.

- [ ] **Step 6: Run package-level regression sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/... ./pkg/entity/... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 7: Commit fix**

```bash
git add pkg/ modules/
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(nai-128): E2a — [DESCRIBE NPC_FINDHERO=0 root cause]

[Body: cite the diagnostic output identifying which gate failed, name
the bug, summarize the diff.]

Closes T5 GroundObjs + T6 CascadeDispatchTrace.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Confirm T6 holds positive contract post-fix

**Why:** After the matching fix branch (Task 3, 4, or 5) lands, T6's two assertions (`sf != nil`; zero warn-error frames) become permanent positive contracts. This task verifies the diagnostic-style code paths still pass cleanly with the fix in place — no skip, no `t.Logf`-only paths, no remaining error frames.

**Files:**
- (No modifications; verification-only task.)

- [ ] **Step 1: Re-run the integration test once more**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -40`

Expected output shape:

```
=== RUN   TestNAI128_RatLootCascade
--- PASS: TestNAI128_RatLootCascade
    --- PASS: TestNAI128_RatLootCascade/FixtureLoaded
    --- PASS: TestNAI128_RatLootCascade/BaselineState
    --- PASS: TestNAI128_RatLootCascade/Preconditions
    --- PASS: TestNAI128_RatLootCascade/AiQueueCascade
    --- PASS: TestNAI128_RatLootCascade/CascadeDispatchTrace
    --- PASS: TestNAI128_RatLootCascade/GroundObjs
```

If `CascadeDispatchTrace` still emits any `t.Errorf`, return to the appropriate fix branch (Task 3/4/5) and re-investigate.

- [ ] **Step 2: Run race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run 'TestNAI128_RatLootCascade$' ./modules/world/ -count=1 2>&1 | tail -20`
Expected: PASS, no race warnings.

(No commit — this is verification only.)

---

### Task 7: Cross-package regression sweep

**Why:** A fix in `pkg/script` or `modules/world` could regress unrelated tests. Sweep before close.

**Files:** none.

- [ ] **Step 1: Run all script tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 2: Run all world-module tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 3: Run remaining package tests touching entity/zone/etc.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -40`
Expected: all PASS. Address any unrelated failures by re-checking the diff for unintended cross-cuts.

(No commit — verification only.)

---

## Phase D — Smoke gate (always runs)

### Task 8: Hand off to user for Java-client smoke test

**Why:** Per `cascade_theory_smoke_binding` and `smoke_test_server_handoff` memory entries, the user must launch goscape outside the sandbox; Claude cannot reach the Java client from its sandboxed shell. Smoke binds production attribution.

**Files:** none.

- [ ] **Step 1: Confirm config.yaml exists in repo root**

Run: `ls -la config.yaml 2>&1 | head`
Expected: file present.

- [ ] **Step 2: Emit smoke prompt to user**

Output to user (do NOT execute the server command yourself):

```
Phase D smoke handoff — please run from a non-sandboxed shell:

  CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml

Then connect with the Client-Java #225 build. Walk to a Lumbridge rat
(e.g. ~3094, 3106, 0), attack until death, and report whether loot
appears on the ground (bones / raw_rat_meat).

If loot drops: smoke passes; this stage closes. Reply with "smoke OK".
If no loot: smoke fails; cascade attribution stays bound to the fixed
branch. Reply with the symptom and we route the residual to NAI-129+.
```

- [ ] **Step 3: Wait for user smoke result before commit-trailer task**

Block on user response. If `smoke OK`, proceed to Task 9. If smoke fails, surface the residual to the user; do NOT close NAI-128.

(No commit — handoff only.)

---

## Close

### Task 9: Add memory entry for fixture-parity gap pattern

**Why:** Per `post_task_handoff` memory: at every task close, save non-derivable info to memory. The fixture-parity gap surfaced in Stage 2 controller pre-flight is generalizable: any future test exercising script dispatch via `processNpcQueue`/`runScript`/`runNpcScript` against `newTestServer` risks the same trap.

**Files:**
- Create: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/test_fixture_view_parity.md`
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`

- [ ] **Step 1: Write the new memory file**

Create `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/test_fixture_view_parity.md`:

```markdown
---
name: Test fixture must mirror NewServer view inits before exercising script dispatch
description: newTestServer-derived fixtures need worldVars/configsView/invLookup/npcLookup wired before processNpcQueue/runScript/runNpcScript runs; otherwise script-side adapters short-circuit on nil-server guards
type: feedback
---

When a test fixture builds a *Server bypassing NewServer (e.g. newTestServer
+ manual cache loads), it must explicitly initialize the script-side
adapter views that production NewServer wires at server.go:236-252:

```go
s.vars        = make([]int32, len(varsTypes.Configs))
s.varsStrings = make([]string, len(varsTypes.Configs))
s.worldVars   = worldVarsView{s: s}
s.configsView = serverConfigsView{s: s}
s.invLookup   = invLookupView{s: s}
s.npcLookup   = serverNpcLookup{s: s}
```

These views all check `if w.s == nil { return zero-value }` as a defensive
guard (see server_varp.go:178-180 for worldVarsView.LookupPlayerByUID).
The guard short-circuits invisibly when the fixture leaves them
zero-valued. Symptoms: NPC_FINDHERO returns 0, OBJ_ADD checkObjType fails,
script-side varp reads return 0, etc. — all silently, masking real
production bugs.

**Why:** NAI-128 Stage 1 spent a full T5 cycle on a fixture-noise binding
(zero-valued worldVars caused NPC_FINDHERO=0, blocking obj_add) before
controller pre-flight at Stage 2 surfaced the gap. The bug never reached
production because NewServer wires these correctly.

**How to apply:** When writing or reviewing a test that exercises
processNpcQueue, runScript, runNpcScript, or any *Server method that
builds a ScriptState via buildNpcScriptState/Init, verify the fixture
copies the seven assignments above (or equivalent for any newer view
adapter). For one-shot fixtures, factor a helper
`fixtureWireScriptViews(s)` in modules/world/test_helpers and call it
after cache loads complete.
```

- [ ] **Step 2: Append index entry to `MEMORY.md`**

Add (sorted into the existing alphabetical/topical structure, e.g. near other testing-fixture entries):

```markdown
- [Test fixture view parity](test_fixture_view_parity.md) — fixtures must mirror NewServer worldVars/configsView/invLookup/npcLookup inits or script-side adapters silently short-circuit (NAI-128 Stage 2 controller pre-flight taught)
```

- [ ] **Step 3: Verify MEMORY.md is still under the 24KB warning threshold**

Run: `wc -c /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`

If now over 24KB, retire the longest stale entry (check for entries marked CLOSED or with smoke-confirmed dates older than 2026-04-15).

(No commit — memory writes don't go to git.)

---

### Task 10: Close commit + PR-ready handoff

**Why:** Per `close_commit_memory_trailer` memory: NAI-N close commits add `Closes memory:` trailer for git-log provenance. This is the final commit of Stage 2.

**Files:** none (close-only).

- [ ] **Step 1: Verify clean working tree**

Run: `git status`
Expected: clean.

- [ ] **Step 2: Verify commit log shape**

Run: `git log --oneline -10`
Expected: see Phase A commit + (if Phase C ran) Phase C T6 commit + fix commit. If Phase B passed and skipped Phase C, only the Phase A commit appears.

- [ ] **Step 3: Add empty close commit (`--allow-empty`) carrying memory provenance**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
docs(nai-128): Stage 2 close — cascade fix landed; smoke confirmed

Phase A: nai128CacheFixture mirrors NewServer view init.
[If Phase C ran:] Phase C: T6 CascadeDispatchTrace probe + fix branch
[E0|E2a|E2b].
Phase D: Java-client smoke confirmed rat loot drops in Lumbridge.

Closes memory: test_fixture_view_parity

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Adjust the `[…]` placeholders to match actual phases run.)

- [ ] **Step 4: Emit resume prompt for next session**

Output the resume prompt for the user's next NAI-N task (per `post_task_handoff`):

```
NAI-128 Stage 2 closed. Phase A fixture parity + [Phase C E0|E2a|E2b fix
if applicable] landed. Java-client smoke confirmed rat loot drops in
Lumbridge.

T5 (TestNAI128_RatLootCascade/GroundObjs) GREEN as close gate.
T6 (CascadeDispatchTrace) added as permanent layered diagnostic.
New memory: test_fixture_view_parity.

Next NAI candidate: [user picks from existing follow-up tracker, or
spec brainstorm for the next investigation/port].
```

(No further commit.)

---

## Self-review

- **Spec coverage:**
  - §1 Goal → close criterion in Task 10.
  - §2 binding recap → Task 2 T6 binds among {E0, E2a, E2b}.
  - §3 fixture-parity gap → Task 1 (Phase A).
  - §4 Phase A → Task 1.
  - §4 Phase B → Task 1 Step 4 (decision point).
  - §4 Phase C → Tasks 2-7.
  - §4 Phase D → Task 8.
  - §5 test strategy (T5 + T6 with positive contracts post-fix) → Tasks 2 + 6.
  - §6 risks → R1 surfaces in Phase B re-run; R2 covered by Phase D smoke; R3 mitigated by reusing existing `capturingHandler`; R4 covered by T5's coord filter; R5 mitigated by `capturingHandler.mu`.
  - §7 out of scope → not in plan.
  - §8 close → Tasks 9-10.
  - §9 memory entries → applied throughout; new entry in Task 9.

- **Placeholder scan:** Tasks 3, 4, 5 contain `[FILL IN]`-style placeholders by design (the fix code depends on the binding observed in Task 2). These are not violations — they are conditional branches whose code is impossible to write before binding. Each placeholder is paired with a concrete investigation step (Step 1) that surfaces the actual bug. **Implementer instruction:** stop and revise the task's later steps with concrete code after Step 1 completes.

- **Type consistency:** `capturingHandler` reused from `interaction_debug_test.go:64-89` — same package, same fields (`mu`, `records`), same `snapshot()` accessor. `script.TriggerAiQueue3` confirmed at `pkg/script/trigger.go:119`. `*script.Provider.GetByTrigger(ServerTriggerType, int, int)` confirmed at `pkg/script/provider.go:114`. `NpcType.Category` confirmed at `pkg/objtype/npctype.go:160`. `worldVarsView{s: s}` and siblings confirmed at `modules/world/server.go:238, 250-252`. `objtype.LoadVarsTypes`/`LoadVarnTypes` signatures confirmed at `pkg/objtype/varstype.go:39` and `varntype.go:39`.
