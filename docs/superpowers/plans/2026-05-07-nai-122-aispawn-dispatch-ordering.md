# NAI-122 — AI_SPAWN dispatch ordering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the structural lag between NPC spawn and `[ai_spawn,_]` dispatch so Tutorial Island giant-rat first-tick attack reads `%npc_combat_xp_multiplier` as its content-defined value rather than `0`.

**Architecture:** Bundle 0 (controller pre-flight, no commits) statically disambiguates Scenario A (engine bug) vs Scenario B (content/data) via a throwaway test against `data/pack/server/` AND verifies the TS dispatch shape directly from `LostCityRS/Engine-TS`. If Scenario A is confirmed and TS uses sync-inline dispatch (the audit-claimed shape), Bundle 1 replaces the queue-append at `npc_registry.go:88-99` with a synchronous `runNpcScript` call. If TS uses a deferred-but-pre-flushed queue, Bundle 1 instead splits AI_SPAWN out of `npcEventQueue` into a dedicated `npcSpawnQueue` drained between `processWorldQueue` and `processInteractions`. If Scenario B is confirmed, NAI-122 closes near-zero-LOC and the symptom routes to a content-pack rebuild item. Smoke is user-launched and binds cascade scope for NAI-121 residuals #1/#2/#3.

**Tech Stack:** Go 1.26+; `pkg/script` provider + `pkg/objtype` config loaders; `modules/world` server.

---

## File Structure

**Modified (engine-fix path (a), default):**
- `modules/world/npc_registry.go:82-99` — replace queue-append AI_SPAWN producer with synchronous `s.runNpcScript(sf, n, nil, nil, nil)`.
- `modules/world/npc_registry_test.go` — add 2 tests (sync-execution pin + queue-not-touched pin).
- `modules/world/npc_event_queue.go:5-16` — update `NpcEventType` doc-comment now that `NpcEventSpawn` has no producer.

**Modified (engine-fix path (c), fallback only):**
- `modules/world/server.go` — new `npcSpawnQueue []NpcEventRequest` field on `*Server`.
- `modules/world/npc_event_queue.go` — new `processNpcSpawnQueue()` method; `processNpcEventQueue` guarded to `Type==NpcEventDespawn` only (or factored).
- `modules/world/tick.go:36-37` — insert `s.processNpcSpawnQueue()` between `processWorldQueue` and `processActiveScripts`.
- `modules/world/npc_registry.go:88-99` — queue target switched from `npcEventQueue` to `npcSpawnQueue`.

**Bundle 0 throwaway (deleted before any commit):**
- `modules/world/aispawn_probe_test.go` — `TestAiSpawnProbe_GiantRatCombatXpMultiplier` prints param presence + value.

**Bundle 0 findings doc (committed at Bundle 0 close):**
- `docs/superpowers/investigations/2026-05-07-nai-122-bundle0-findings.md` — verbatim probe output + verbatim TS excerpt + outcome routing decision.

**Unchanged but verified:**
- `modules/world/npc_registry_test.go::TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne` — must still pass under the new code path.
- `modules/world/npc_event_queue_test.go` (AI_DESPAWN tests) — must still pass.

---

## Bundle 0 — Controller pre-flight (no commits except findings doc)

### Task B0.1: Static disambiguation probe

**Files:**
- Create (throwaway, deleted before any commit): `modules/world/aispawn_probe_test.go`

- [ ] **Step 1: Write the throwaway probe test**

Create `modules/world/aispawn_probe_test.go` with:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// THROWAWAY — NAI-122 Bundle 0 disambiguator. DELETE before commit.
// Prints the giant-rat NpcType.Params lookup for combat_xp_multiplier
// to disambiguate Scenario A (engine ordering bug) vs Scenario B
// (content/data: param absent or zero).
func TestAiSpawnProbe_GiantRatCombatXpMultiplier(t *testing.T) {
	const cacheDir = "../../data/pack"
	npcs, err := objtype.LoadNPCTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadNPCTypes(%q): %v", cacheDir, err)
	}
	params, err := objtype.LoadParams(cacheDir)
	if err != nil {
		t.Fatalf("LoadParams(%q): %v", cacheDir, err)
	}

	paramID, ok := params.ConfigNames["combat_xp_multiplier"]
	if !ok {
		t.Fatalf("ParamType combat_xp_multiplier NOT registered (key absent in ConfigNames). Scenario B confirmed at the param-registry layer.")
	}
	pt := params.Configs[paramID]
	t.Logf("ParamType combat_xp_multiplier: id=%d Type=%d DefaultInt=%d", paramID, pt.Type, pt.DefaultInt)

	ratID, ok := npcs.ConfigNames["giant_rat"]
	if !ok {
		t.Fatalf("NpcType giant_rat NOT registered (key absent in ConfigNames)")
	}
	rat := npcs.Configs[ratID]
	if v, present := rat.Params[uint32(paramID)]; present {
		t.Logf("giant_rat (id=%d) Params[combat_xp_multiplier]: PRESENT value=%v (Go type %T)", ratID, v, v)
	} else {
		t.Logf("giant_rat (id=%d) Params[combat_xp_multiplier]: ABSENT — paramLookup falls back to DefaultInt=%d", ratID, pt.DefaultInt)
	}

	// Sanity dump: a second NPC for comparison.
	for _, name := range []string{"man", "rat", "chicken"} {
		if id, ok := npcs.ConfigNames[name]; ok {
			n := npcs.Configs[id]
			if v, present := n.Params[uint32(paramID)]; present {
				t.Logf("[sanity] %s (id=%d) Params[combat_xp_multiplier]: PRESENT value=%v", name, id, v)
			} else {
				t.Logf("[sanity] %s (id=%d) Params[combat_xp_multiplier]: ABSENT", name, id)
			}
		}
	}
}
```

- [ ] **Step 2: Run the probe**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestAiSpawnProbe_GiantRatCombatXpMultiplier -v ./modules/world/`

Expected: PASS (the test only logs; failures only on missing data files).
Capture all `t.Logf` output verbatim.

- [ ] **Step 3: Interpret outcome**

Three branches:

- **Scenario A confirmed** — `combat_xp_multiplier` param ID present AND `giant_rat.Params` contains it with a non-zero `value` (uint32 or int). Engine has a dispatch ordering bug. Proceed to Task B0.2.
- **Scenario B confirmed (variant 1)** — param key absent from `params.ConfigNames`. The ParamType isn't registered at all. Symptom is content/data; close NAI-122 with no engine change.
- **Scenario B confirmed (variant 2)** — param ID present but `giant_rat.Params` does NOT contain it (so `paramLookup` returns `DefaultInt`), AND `DefaultInt == 0`. AI_SPAWN script writes 0 correctly because the source is 0. Symptom is content/data; close NAI-122 with no engine change.
- **Scenario A still possible (variant 3)** — param ID present, `giant_rat.Params` contains it with a NON-zero value. Engine has the ordering bug. Proceed to Task B0.2.
- **Inconclusive** — any unexpected error (wrong cache path, signature mismatch). Fall back to a temporary production probe behind a debug flag (separate sub-cycle, not pre-decomposed here).

- [ ] **Step 4: Delete the throwaway test**

Run: `rm modules/world/aispawn_probe_test.go`

Verify: `git status` shows no `aispawn_probe_test.go` entry.

- [ ] **Step 5: Do NOT commit yet** — Bundle 0 commits only the findings doc at task B0.3.

### Task B0.2: TS-source verification

**Files:**
- Read only: `LostCityRS/Engine-TS/.../World.ts` (canonical TS source per `ts_source_canonical_path`).

- [ ] **Step 1: Locate `World.addNpc` in TS**

Run from goscape repo root:
```
rg -n "addNpc\s*\(" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/World.ts | head
```
Read the surrounding ~50 lines around the match (`Read` tool, not Bash cat).

- [ ] **Step 2: Locate the AI_SPAWN dispatch site in TS**

Run from goscape repo root:
```
rg -n "AI_SPAWN\|ai_spawn\|ServerTriggerTypes\.AI_SPAWN" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/World.ts | head
```
Read each match in context.

- [ ] **Step 3: Determine TS dispatch shape**

Three TS shapes possible:

- **TS-(a) sync-inline:** `World.addNpc` calls AI_SPAWN dispatch directly inline — e.g. via `executeScript(ScriptRunner.init(...))` or equivalent — within or immediately after the NPC-register block. If found, lock fix shape (a) for Bundle 1.
- **TS-(c) deferred-but-pre-flush:** AI_SPAWN goes through a queue distinct from despawn, and that queue is drained in a tick phase BEFORE combat reads. If found, lock fix shape (c) for Bundle 1.
- **TS-(none) matches goscape current:** TS uses the same single-queue post-interactions ordering goscape has. Then V-PARTIAL has another root cause; close NAI-122 with no engine change and route the symptom forward.

- [ ] **Step 4: Capture verbatim TS excerpts**

Record file path + line range + literal source code for the addNpc neighborhood and the AI_SPAWN dispatch site. These go into the findings doc at B0.3.

### Task B0.3: Commit Bundle 0 findings doc + lock outcome

**Files:**
- Create: `docs/superpowers/investigations/2026-05-07-nai-122-bundle0-findings.md`

- [ ] **Step 1: Write findings doc**

Template:

```markdown
# NAI-122 Bundle 0 — Static disambiguation findings

**Date:** 2026-05-07
**Scope:** Controller pre-flight (no production code change). Disambiguates Scenario A vs B for the V-PARTIAL parked at NAI-120 / NAI-121.

## 1. Static probe output (Task B0.1)

<paste verbatim t.Logf output from the throwaway probe test, including param ID + value + giant_rat lookup result + sanity dumps>

## 2. TS source verification (Task B0.2)

### World.addNpc neighborhood

File: `LostCityRS/Engine-TS/src/engine/World.ts:<line-range>`

```
<paste verbatim TS source>
```

### AI_SPAWN dispatch site

File: `LostCityRS/Engine-TS/src/engine/World.ts:<line-range>`

```
<paste verbatim TS source>
```

## 3. Outcome lock

Selected fix shape:
- [ ] (a) Sync dispatch in `addNpc` — TS-(a) sync-inline.
- [ ] (c) Split-queue + pre-flush — TS-(c) deferred-but-pre-flush.
- [ ] No engine change (Scenario B / TS-(none)) — close NAI-122 near-zero-LOC.

Justification: <one paragraph citing the probe output + TS verbatim>

## 4. Refutations of NAI-121 audit claims

NAI-121 audit cited `World.ts:1284-1289` as TS sync-inline AI_SPAWN dispatch. Verified at HEAD:
- [ ] Line range matches: <yes/no>
- [ ] Shape matches: <yes/no>

If "no" on either: declare DEVIATION-NAI-122-D3 with corrected line citation.
```

- [ ] **Step 2: Commit Bundle 0 findings**

```bash
git add docs/superpowers/investigations/2026-05-07-nai-122-bundle0-findings.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
investigation(nai-122): Bundle 0 — Scenario A vs B disambiguation + TS verification

Static probe output + verbatim TS-source excerpts; locks fix shape for
Bundle 1 (or closes NAI-122 near-zero-LOC if Scenario B).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3: Outcome routing**

- If Scenario B: skip Bundle 1 entirely. Jump to "Close NAI-122 near-zero-LOC" task at end of plan.
- If Scenario A + TS-(a): execute Bundle 1 path (a) (Tasks B1.1–B1.5).
- If Scenario A + TS-(c): execute Bundle 1 path (c) (Tasks B1.1c–B1.5c).
- If TS-(none): close NAI-122 with a no-op commit + symptom routes forward.

---

## Bundle 1 — Path (a): Sync dispatch in addNpc

Materialized only if Bundle 0 locks shape (a). Subagent-driven TDD per task; one fresh Sonnet implementer per task; spec-then-quality reviewer subagent between tasks per `superpowers_code_reviewer_model`.

### Task B1.1: Reentrancy + boot-storm pre-flight audit (controller, no commit)

- [ ] **Step 1: Grep `addNpc` call sites**

Run: `rg -n "s\.addNpc\(|\.addNpc\(" /home/owner/Code/github.com/zsrv/goscape/modules/world/`

Read each match in context. For every site: confirm it is NOT reachable from inside `runNpcScript` (i.e. no opcode handler invoked by AI_SPAWN scripts calls `s.addNpc`).

- [ ] **Step 2: Grep `npc_add` opcode handler**

Run: `rg -n "TriggerNpcAdd|npc_add\b|OpNpcAdd|NpcAddHandler" /home/owner/Code/github.com/zsrv/goscape/pkg/script/ /home/owner/Code/github.com/zsrv/goscape/modules/world/`

If a handler exists and calls `s.addNpc`, escalate: pivot to Bundle 1 path (c) OR add a "during-this-call deferred queue" guard to path (a). Otherwise: clean.

- [ ] **Step 3: Grep AI_SPAWN content scripts for pre-tick hazards**

Run: `rg -n "\[ai_spawn," /home/owner/Code/github.com/LostCityRS/Content/scripts/`

For each match, read the script body. Disqualifying patterns:
- Reads `%world_currenttick` (script behavior depends on tick number).
- Calls `[ai_queue1..ai_queue20]` (these enqueue downstream events; pre-tick enqueue is probably benign but worth noting).
- Calls `world_delay`, `if_close`, `tutorial_*` (player-context-dependent — but AI_SPAWN never has a player context, so these are likely already broken on TS too).

Document findings. If hazards found: declare DEVIATION-NAI-122-D1 with explicit hazard list.

- [ ] **Step 4: Capture findings inline**

Record audit results in the eventual Task B1.5 commit body or as a comment in `docs/superpowers/investigations/2026-05-07-nai-122-bundle0-findings.md` under a "Bundle 1 pre-flight" section. No standalone commit.

### Task B1.2: Write the failing sync-execution test

**Files:**
- Modify: `modules/world/npc_registry_test.go` (append after `TestAddNpc_RespawnAfterChangeType_ReseedsVarns`).

- [ ] **Step 1: Write the failing test**

Append:

```go
// TestAddNpc_FreshSpawn_RunsAiSpawnSynchronously pins NAI-122 fix shape
// (a): the AI_SPAWN script dispatched at addNpc must execute
// synchronously, with side-effects observable on return — NOT deferred
// to processNpcEventQueue. This closes the V-PARTIAL where combat
// reads %npc_combat_xp_multiplier on the same tick the NPC spawns.
func TestAddNpc_FreshSpawn_RunsAiSpawnSynchronously(t *testing.T) {
	s := newTestServer(t)
	// Reset the default test provider to one we control.
	s.scriptProvider = script.NewProvider()

	// Seed a varn the AI_SPAWN script will mutate as the side-effect
	// signal. Use an INT varn so the seed-loop initialises it to 0.
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypeInt, Name: "ai_spawn_marker"},
	})

	// AI_SPAWN script: PUSH_CONSTANT_INT 42; POP_VARN 0; RETURN.
	aiSpawn := &script.ScriptFile{
		Name:      "[ai_spawn,_]",
		LookupKey: script.LookupKeyForGlobal(script.TriggerAiSpawn),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPopVarn,
			script.OpReturn,
		},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(aiSpawn)

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	// Sync-pin: AI_SPAWN side-effect visible on return, BEFORE any
	// processNpcEventQueue call.
	if got := n.NpcVarN(0); got != 42 {
		t.Errorf("ai_spawn_marker after addNpc: got %d, want 42 (AI_SPAWN must run synchronously)", got)
	}
	// Queue-removal pin: nothing landed in the deferred queue.
	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue len after addNpc: got %d, want 0 (sync dispatch must not enqueue)", len(s.npcEventQueue))
	}
}
```

(Plan-author verified: `script.OpPopVarn` (lowercase n) is defined at `pkg/script/opcode.go:38` as `Opcode = 5`. NAI-121-T7 dispatch at `handlers_vars.go:120-132`.)

- [ ] **Step 2: Run the failing test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestAddNpc_FreshSpawn_RunsAiSpawnSynchronously -v ./modules/world/`

Expected: FAIL — `ai_spawn_marker after addNpc: got 0, want 42` AND `npcEventQueue len after addNpc: got 1, want 0`. Both assertions fail under current queue-deferred dispatch.

### Task B1.3: Implement sync dispatch

**Files:**
- Modify: `modules/world/npc_registry.go:82-99`.

- [ ] **Step 1: Replace queue-append with sync dispatch**

Old (lines 82-99):
```go
	// AI_SPAWN trigger queue (matches TS World.ts:1284-1289). Fires
	// unconditionally — for both firstSpawn=true (server boot) and
	// firstSpawn=false (revertType respawn). NPCs without a registered
	// AI_SPAWN script never enter the queue (the script != nil guard).
	// processNpcEventQueue dispatches at tick.go:40. Mirrors the
	// existing AI_DESPAWN producer pattern at npc_ai.go:47-58.
	if s.scriptProvider != nil && n.typ != nil {
		sf := s.scriptProvider.GetByTrigger(
			script.TriggerAiSpawn, n.typeId, n.typ.Category)
		if sf != nil {
			s.npcEventQueue = append(s.npcEventQueue,
				NpcEventRequest{
					Type:   NpcEventSpawn,
					Script: sf,
					Npc:    n,
				})
		}
	}
```

New:
```go
	// AI_SPAWN sync dispatch (NAI-122; matches TS World.ts:<verified-line-range
	// from Bundle 0 findings>). Fires unconditionally for both firstSpawn=true
	// (server boot) and firstSpawn=false (revertType respawn). NPCs without a
	// registered AI_SPAWN script no-op (the script != nil guard).
	//
	// Sync dispatch closes the V-PARTIAL where combat scripts read
	// %npc_combat_xp_multiplier on the same tick the NPC spawned, BEFORE
	// the deferred processNpcEventQueue had populated it (NAI-121 Bundle 2
	// audit; NAI-122 spec §1).
	//
	// DEVIATION-NAI-122-D1: pre-tick synchronous AI_SPAWN at server boot.
	// Scripts run while s.currentTick == 0 and before any processX phase.
	// Audit at Bundle 1 verified no AI_SPAWN content script depends on
	// tick/phase state. Retire if a future content-script needs it.
	if s.scriptProvider != nil && n.typ != nil {
		sf := s.scriptProvider.GetByTrigger(
			script.TriggerAiSpawn, n.typeId, n.typ.Category)
		if sf != nil {
			s.runNpcScript(sf, n, nil, nil, nil)
		}
	}
```

(Plan-author: implementer fills `<verified-line-range>` from Task B0.3 findings doc.)

- [ ] **Step 2: Run the test from B1.2**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestAddNpc_FreshSpawn_RunsAiSpawnSynchronously -v ./modules/world/`

Expected: PASS.

- [ ] **Step 3: Run NAI-121 PRIMARY pin**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne -v ./modules/world/`

Expected: PASS. (resetEntityForRespawn at line 79 still seeds player_uid varn to -1 BEFORE AI_SPAWN dispatches at line 88; ordering preserved by sync replacement.)

- [ ] **Step 4: Run all `modules/world` tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: PASS. If any AI_DESPAWN test fails, that's a regression — investigate before proceeding.

- [ ] **Step 5: Run cross-package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` and `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` and `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_registry.go modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-122): T1 — sync AI_SPAWN dispatch in addNpc (V-PARTIAL fix)

Replaces queue-append at npc_registry.go:88-99 with synchronous
runNpcScript. Closes the structural lag where combat scripts read
%npc_combat_xp_multiplier on the spawn tick BEFORE the deferred
processNpcEventQueue populates it (NAI-121 Bundle 2 V-PARTIAL audit;
TS World.ts:<line-range from Bundle 0 findings>).

DEVIATION-NAI-122-D1 declared: pre-tick synchronous AI_SPAWN at
server boot. Bundle 1 pre-flight audit (Task B1.1) verified no
AI_SPAWN content script depends on tick/phase state.

Pinned by TestAddNpc_FreshSpawn_RunsAiSpawnSynchronously (sync
side-effect observable on addNpc return + npcEventQueue len 0).
NAI-121 TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne re-verified
green under new code path.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.4: Update NpcEventType doc-comment (NpcEventSpawn now producerless)

**Files:**
- Modify: `modules/world/npc_event_queue.go:5-16`.

- [ ] **Step 1: Read current doc-comment**

Lines 5-16:
```go
// NpcEventType mirrors TS NpcEventType at
// Engine-TS/src/engine/entity/NpcEventRequest.ts.
// NpcEventSpawn is queued by Server.addNpc (NAI-22 Bundle 1, mirroring
// TS World.ts:1284-1289); NpcEventDespawn is queued by the DESPAWN
// branch of the Npc.turn() Events block (NAI-5, mirroring TS
// World.ts:580+).
type NpcEventType int

const (
	NpcEventSpawn   NpcEventType = 0
	NpcEventDespawn NpcEventType = 1
)
```

- [ ] **Step 2: Decision: retain or retire NpcEventSpawn?**

`NpcEventSpawn` no longer has a producer (Task B1.3 removed the only one). Per `dead_api_polish`, retire constants with zero consumers. Grep:

Run: `rg -n "NpcEventSpawn\b" /home/owner/Code/github.com/zsrv/goscape/`

If no production reference remains:
- Delete `NpcEventSpawn = 0` constant.
- Update `NpcEventDespawn = 1` to `NpcEventDespawn = 0` (or leave as 1 for stability — implementer picks; test fixtures may reference the literal value).
- Delete the `NpcEventSpawn`-related sentence from the doc-comment.
- Search test files for `Type: NpcEventSpawn` literals; these should not exist after Task B1.3.

If `NpcEventSpawn` is still referenced in tests (e.g. fixtures that manually enqueued an `NpcEventSpawn`-typed request to test the dispatcher):
- Decide whether those tests still have value. If they tested `processNpcEventQueue` AI_SPAWN dispatch, they're now obsolete (no producer); retire them.
- Update doc-comment to reflect "AI_DESPAWN-only queue" rather than "AI_SPAWN + AI_DESPAWN".

- [ ] **Step 3: Apply chosen change**

If retiring `NpcEventSpawn`:

```go
// NpcEventType mirrors TS NpcEventType at
// Engine-TS/src/engine/entity/NpcEventRequest.ts. As of NAI-122,
// only NpcEventDespawn has a producer — AI_SPAWN dispatches
// synchronously inside Server.addNpc (npc_registry.go).
//
// NpcEventDespawn is queued by the DESPAWN branch of Npc.turn()'s
// Events block (NAI-5, mirroring TS World.ts:580+).
type NpcEventType int

const (
	NpcEventDespawn NpcEventType = 1
)
```

(Implementer note: keep `NpcEventDespawn = 1` to avoid renumbering risk if any wire-encoding path or test fixture references the literal.)

If keeping `NpcEventSpawn` for test fixtures: update doc-comment narration only:

```go
// NpcEventType mirrors TS NpcEventType at
// Engine-TS/src/engine/entity/NpcEventRequest.ts. As of NAI-122,
// AI_SPAWN dispatches synchronously inside Server.addNpc — the
// NpcEventSpawn variant is retained for test fixtures that
// manually enqueue spawn events but has no production producer.
// NpcEventDespawn is queued by the DESPAWN branch of Npc.turn()'s
// Events block (NAI-5, mirroring TS World.ts:580+).
```

- [ ] **Step 4: Run all tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: PASS. If Step 2 retired `NpcEventSpawn` and a test fails compilation, restore the constant and use the doc-comment-only update.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_event_queue.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(nai-122): T2 — retire NpcEventSpawn / update queue doc-comment

Reflects post-T1 state: AI_SPAWN dispatches synchronously in
Server.addNpc; NpcEventSpawn no longer has a producer.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.5: Bundle 1 close gate

- [ ] **Step 1: Cross-package green check**

Run independently in fresh shells:
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: all green at HEAD. Per `verify_implementer_claims`, controller runs each independently — does not trust IDE diagnostics.

- [ ] **Step 2: Sonnet code-reviewer pass**

Per `superpowers_code_reviewer_model`, dispatch a Sonnet reviewer subagent over the Bundle 1 commits (T1 + T2). Review scope:
- TS-fidelity of B1.3 doc-comment vs Bundle 0 findings TS excerpt.
- DEVIATION-NAI-122-D1 wording present + retire condition explicit.
- No regression of NAI-121 PRIMARY pin.
- Bundle 1 pre-flight audit findings (Task B1.1) referenced or inlined.

Expected: conditional ✅ or fixes landed in a follow-up commit before smoke handoff.

- [ ] **Step 3: Commit-content verification**

Per `implementer_commit_content_verify`:
```bash
git log --oneline 1f73294..HEAD
git show <T1-SHA> --stat
git show <T2-SHA> --stat
git status   # must be clean (no stray worktree leakage)
```
Verify each commit's actual diff matches its message. Per `feedback_subagent_wt_path`, also confirm no stray content in the main working tree from any subagent worktree.

---

## Bundle 1 — Path (c): Split queue + pre-flush phase (FALLBACK)

Materialized only if Bundle 0 locks shape (c) OR Task B1.1 surfaces reentrancy / boot-storm hazards that escalate from path (a). Mirrors path (a)'s task structure but with different file inventory.

### Task B1.1c: Same pre-flight audit as B1.1 (no commit)

(Identical to Task B1.1 above — pre-flight grep + read.)

### Task B1.2c: Add npcSpawnQueue field + processNpcSpawnQueue method

**Files:**
- Modify: `modules/world/server.go` — add `npcSpawnQueue []NpcEventRequest` field on `*Server`.
- Modify: `modules/world/npc_event_queue.go` — add `processNpcSpawnQueue()` method.

- [ ] **Step 1: Add the field**

In `modules/world/server.go`, locate the `Server` struct and the existing `npcEventQueue []NpcEventRequest` field (grep `npcEventQueue` to find exact line). Add adjacent:

```go
	// NAI-122 Bundle 1c: AI_SPAWN-only queue, drained between
	// processWorldQueue and processActiveScripts (tick.go) so AI_SPAWN
	// scripts populate npc varns BEFORE processInteractions reads them.
	// Separate from npcEventQueue (DESPAWN-only) per
	// DEVIATION-NAI-122-D2 — TS unified-queue shape diverges; goscape
	// asymmetry retired when TS unifies.
	npcSpawnQueue []NpcEventRequest
```

- [ ] **Step 2: Add the drain method**

In `modules/world/npc_event_queue.go`, append:

```go
// processNpcSpawnQueue drains AI_SPAWN-typed events queued by
// Server.addNpc. Runs in the tick phase between processWorldQueue
// and processActiveScripts so AI_SPAWN scripts populate npc varns
// BEFORE processInteractions reads them on the same tick.
//
// Iteration mirrors processNpcEventQueue's removal-before-fire pattern.
// NAI-122 Bundle 1c.
func (s *Server) processNpcSpawnQueue() {
	i := 0
	for i < len(s.npcSpawnQueue) {
		req := s.npcSpawnQueue[i]
		if req.Npc.delayed {
			i++
			continue
		}
		s.npcSpawnQueue = append(s.npcSpawnQueue[:i], s.npcSpawnQueue[i+1:]...)
		s.runNpcScript(req.Script, req.Npc, nil, nil, nil)
		// don't advance i — removed current entry
	}
}
```

- [ ] **Step 3: Build check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: green (no test changes yet).

- [ ] **Step 4: Commit**

```bash
git add modules/world/server.go modules/world/npc_event_queue.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-122): T1c — add npcSpawnQueue field + processNpcSpawnQueue drain

Foundation for path (c) split-queue fix. No producer/consumer wiring
yet — those land in T2c (tick insertion) and T3c (addNpc routing).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.3c: Insert processNpcSpawnQueue in tick.go

**Files:**
- Modify: `modules/world/tick.go:36-37`.

- [ ] **Step 1: Insert call**

Old (lines 35-43):
```go
		s.processClientsIn()
		s.processWorldQueue() // NAI-37: matches TS World.processWorld start-of-cycle ordering
		s.processActiveScripts()
		s.processPlayerTimers()
		s.processPathing()
		s.processInteractions()
		s.processWalkTriggerFallbacks() // NAI-77 T3: TS World.ts:635-641 per-tick re-path + PLAYERSETUP walktrigger
		s.processNpcEventQueue() // NAI-5: matches TS World.ts:356
		s.processNpcs()
```

New:
```go
		s.processClientsIn()
		s.processWorldQueue() // NAI-37: matches TS World.processWorld start-of-cycle ordering
		s.processNpcSpawnQueue() // NAI-122 Bundle 1c: AI_SPAWN drains BEFORE processInteractions reads npc varns
		s.processActiveScripts()
		s.processPlayerTimers()
		s.processPathing()
		s.processInteractions()
		s.processWalkTriggerFallbacks() // NAI-77 T3: TS World.ts:635-641 per-tick re-path + PLAYERSETUP walktrigger
		s.processNpcEventQueue() // NAI-5: matches TS World.ts:356 (AI_DESPAWN-only post-NAI-122)
		s.processNpcs()
```

- [ ] **Step 2: Run all `modules/world` tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: PASS. (Producer for npcSpawnQueue not yet wired; the queue is empty every tick. No regression expected.)

- [ ] **Step 3: Commit**

```bash
git add modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-122): T2c — insert processNpcSpawnQueue in tick.go pre-interactions

Drain happens between processWorldQueue and processActiveScripts so
AI_SPAWN scripts populate npc varns before any same-tick combat read.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.4c: Switch addNpc producer + write the failing test

**Files:**
- Modify: `modules/world/npc_registry.go:82-99`.
- Modify: `modules/world/npc_registry_test.go` — add path-(c) test.

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_registry_test.go`:

```go
// TestAddNpc_FreshSpawn_QueuesAiSpawnInSpawnQueue pins NAI-122 fix shape
// (c): the AI_SPAWN producer at addNpc routes to npcSpawnQueue (drained
// pre-interactions in tick) NOT npcEventQueue (post-interactions).
func TestAddNpc_FreshSpawn_QueuesAiSpawnInSpawnQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	aiSpawn := &script.ScriptFile{
		Name:      "[ai_spawn,_]",
		LookupKey: script.LookupKeyForGlobal(script.TriggerAiSpawn),
		Opcodes: []script.Opcode{
			script.OpReturn,
		},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	s.scriptProvider.Register(aiSpawn)

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	if len(s.npcSpawnQueue) != 1 {
		t.Errorf("npcSpawnQueue len after addNpc: got %d, want 1", len(s.npcSpawnQueue))
	}
	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue len after addNpc: got %d, want 0 (AI_SPAWN must NOT enter despawn-only queue)", len(s.npcEventQueue))
	}
}
```

- [ ] **Step 2: Run failing test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestAddNpc_FreshSpawn_QueuesAiSpawnInSpawnQueue -v ./modules/world/`

Expected: FAIL — `npcSpawnQueue len after addNpc: got 0, want 1` AND `npcEventQueue len after addNpc: got 1, want 0`.

- [ ] **Step 3: Switch the producer**

In `modules/world/npc_registry.go:82-99`, replace `s.npcEventQueue = append(...)` target with `s.npcSpawnQueue = append(...)` and update the comment block to cite NAI-122 Bundle 1c + DEVIATION-NAI-122-D2 (asymmetric SPAWN/DESPAWN queues).

- [ ] **Step 4: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestAddNpc_FreshSpawn_QueuesAiSpawnInSpawnQueue -v ./modules/world/`

Expected: PASS.

- [ ] **Step 5: Run NAI-121 PRIMARY pin + cross-package**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne -v ./modules/world/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_registry.go modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-122): T3c — addNpc routes AI_SPAWN to npcSpawnQueue

Closes the V-PARTIAL via path (c): AI_SPAWN drains in
processNpcSpawnQueue (tick.go between processWorldQueue and
processActiveScripts) BEFORE processInteractions reads npc varns
on the spawn tick.

DEVIATION-NAI-122-D2 declared: SPAWN/DESPAWN queue asymmetry.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task B1.5c: Bundle 1c close gate

(Identical to Task B1.5 above — cross-package green + Sonnet reviewer + commit-content verify.)

---

## Smoke handoff (user-launched)

Per `smoke_test_server_handoff`, smoke is user-launched.

- [ ] **Step 1: Build the binary**

```
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o /go/bin/goscape ./cmd/goscape
```

(Plan-author note: emit a resume prompt asking the user to launch their server with the post-Bundle-1 binary.)

- [ ] **Step 2: User runs the smoke flow**

Tutorial Island (or whichever map suits per `java_client_coord_chat_suppression`): log in → walk to giant rat → attack.

- [ ] **Step 3: Bind on smoke result**

- ✅ **PRIMARY met:** first hit on giant rat deals NON-ZERO damage. `%npc_combat_xp_multiplier` reads correctly. Proceed to Step 4.
- ❌ **PRIMARY not met:** if hits still deal 0 damage despite the dispatch fix, residual #1 was NOT cascade — V-PARTIAL is fixed but damage formula has a separate root cause. Materialize Bundle 2 (probe damage formula) OR route to NAI-123 per `smoke_surfaces_adjacent_divergences` 30-LOC threshold.

- [ ] **Step 4: Cascade binding for residuals #2/#3**

- ✅ Residual #2 ("Someone else is fighting that") cascade-resolved? Bind: in-scope-stretch (≤30 LOC follow-up commit) or NAI-123.
- ✅ Residual #3 (NPC non-retaliation) cascade-resolved? Almost certainly NO (different engine subsystem). Route to NAI-123.

---

## Bundle 2 — Conditional, materialized only on smoke failure

Templated; not pre-decomposed. If Step 3 of smoke shows residual #1 (zero damage) persists despite `%npc_combat_xp_multiplier` reading correctly:

1. Probe inside damage-formula handler chain — grep `pkg/script/handlers_combat*.go` (or equivalent). Add a test that exercises the damage formula with a known multiplier and asserts the output.
2. If probe shows the formula is correct but a different gate intercepts → escalate to NAI-123.
3. If probe shows the formula multiplies wrong → ≤30 LOC fix in-scope-stretch; otherwise route to NAI-123.

Smoke binds re-cycles with the same handoff pattern.

---

## Close NAI-122 (final commit, after smoke binds)

- [ ] **Step 1: Update memory entries**

Per `post_task_handoff` and `nai_followups`:
- Add a NAI-122 close section to `nai_followups.md` mirroring NAI-121's structure: scope, spec/plan/findings paths, cadence, commits with SHAs, deviations, smoke result, pattern memories applied, cross-references, carry-forward routing.
- If new deviations declared: add their retire conditions to the carry-forward queue.
- If smoke surfaced new residuals: enumerate them.

- [ ] **Step 2: Final close commit**

Per `close_commit_memory_trailer`:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-122 — V-PARTIAL fix + smoke binding

PRIMARY met: %npc_combat_xp_multiplier reads correctly on Tutorial
Island giant-rat first-tick attack. <Cascade summary: residual
#N closed in-scope-stretch / routed to NAI-123>.

Closes memory: <list of memory entries directly applied>.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Plan-author: implementer fills cascade summary + Closes memory list at smoke close.)

- [ ] **Step 3: Emit resume prompt for next NAI**

Per `post_task_handoff`, give the user a paste-ready resume prompt for NAI-123 that enumerates remaining residuals + carry-forward queue.

---

## Plan-coverage crosscheck (controller, pre-dispatch)

Per `plan_test_coverage_crosscheck` + `controller_preflight`, before dispatching the first implementer for Bundle 1:

- [ ] Diff spec §6 test list against the test code blocks in B1.2 / B1.4c: every spec test has a plan task that materializes it.
- [ ] Re-grep HEAD for premises:
  - `npc_registry.go:82-99` — confirm line numbers still match (lines may have shifted post-NAI-121).
  - `tick.go:35-43` — confirm `processWorldQueue` / `processActiveScripts` line numbers.
  - `npc_event_queue.go:5-16` — confirm `NpcEventType` doc-comment line range.
  - `npc_registry_test.go::TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne` exists at line ~462.
  - `script.OpPopVarn` constant (lowercase n; verified `pkg/script/opcode.go:38`).
  - `seedVarnTypes` helper signature — match parameters from `npc_registry_test.go:330-348`.
  - `LookupKeyForGlobal` helper exists in `pkg/script/lookup_key.go`.
- [ ] Grep `s\.addNpc\(` — confirm Task B1.1's reentrancy audit assumptions (no path inside `runNpcScript`).
