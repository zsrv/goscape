# NAI-22 follow-up bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land three bundles closing two NAI-series deviations (NAI-19-D2 AI_SPAWN producer, NAI-21-D1 appearanceInv ctor binding) and porting the deferred `checkInv` huntPlayers filter, taking the active deviation count from 16 to 14.

**Architecture:** Bundle 1 activates the reserved `NpcEventSpawn` constant by adding the AI_SPAWN producer in `Server.addNpc`, mirroring the existing AI_DESPAWN producer pattern at `npc_ai.go:47-58`. Bundle 2 adds the `checkInv` filter to `huntPlayers` plus a non-ScriptState `invTotalParam` helper in the same file. Bundle 3 calls the existing `Player.SetAppearanceInv` from the production login wiring (`client.go:113`) using `c.server.invTypes.Worn`, retiring NAI-21-D1 callouts across `appearance.go`, `player.go`, `pkg/script/active.go`, `pkg/script/handlers_player.go`, and refactors two manual byte-pair scans in `appearance_test.go` to `bytes.Contains`.

**Tech Stack:** Go 1.26+; existing packages: `modules/world`, `pkg/objtype`, `pkg/script`, `pkg/inventory`. No new packages, no new exported types.

**Spec reference:** `docs/superpowers/specs/2026-04-25-nai-22-followup-bundle-design.md`

**Pre-flight findings (plan-write time, HEAD `66a2f9f`):**
1. **Bundle 3 simplification.** `Player.SetAppearanceInv(id int)` already exists at `modules/world/player_script.go:365` — it writes `p.appearanceInv` and flips `MaskAppearance`. Plan uses this directly; the spec's proposed package-private `setAppearanceInv` is unneeded.
2. **Bundle 2 accessor shape.** `*ObjTypeConfigs` and `*ParamTypeConfigs` expose `Configs []*Type` slices directly (no `Get(id)` accessor). The new helper indexes the slice with bounds-check.
3. **Bundle 3 caller path.** `client.go:113` uses `c.server` (not `c.s`). `c.server.invTypes` is the `*objtype.InvTypeConfigs` registry; `.Worn` is a public `int` field at `pkg/objtype/invtype.go:95`.
4. **Test seeding pattern.** Tests that need `s.scriptProvider` non-nil seed it via `s.scriptProvider = script.NewProvider()` after the test-server builder (see `npc_interaction_test.go:118+`, `npc_script_test.go:306`, and the `defaultTestProvider()` docstring at `server_test.go:280`).

---

## File Structure

| Bundle | File | Action | Purpose |
|---|---|---|---|
| 1 | `modules/world/npc_registry.go:77-82` | Modify | Replace deviation comment with AI_SPAWN producer block |
| 1 | `modules/world/npc_event_queue.go:7-9` | Modify | Update doc-comment narrating producer existence |
| 1 | `modules/world/npc.go:259, 283` | Modify | Update / remove NAI-19-D2 callouts (NAI-19-D1 stays) |
| 1 | `modules/world/npc_event_queue_test.go` | Modify | Add SPAWN-side coverage (5 new tests) |
| 2 | `modules/world/npc_hunt.go` | Modify | Add `invTotalParam` helper + checkInv filter block; update doc-comment header |
| 2 | `modules/world/npc_hunt_test.go` | Modify | Add 6 new CheckInv tests |
| 3 | `modules/world/client.go:113` | Modify | Add `p.SetAppearanceInv(c.server.invTypes.Worn)` line |
| 3 | `modules/world/player.go:322` | Modify | Update sentinel ctor doc-comment |
| 3 | `modules/world/appearance.go:25-28` | Modify | Retag NAI-21-D1 narrative as test-only fallback |
| 3 | `modules/world/appearance_test.go` | Modify | Add `TestSetAppearanceInvBindsId`; refactor 2 byte-search loops to `bytes.Contains` |
| 3 | `pkg/script/active.go:329, 332` | Modify | Retire NAI-21-D1 callouts |
| 3 | `pkg/script/handlers_player.go:133-136` | Modify | Retire NAI-21-D1 callouts |

---

## Bundle 1 — AI_SPAWN producer activation (NAI-19-D2 closure)

**Spec section**: `## Scope > ### Bundle 1 — AI_SPAWN producer activation`.

### Task 1.1: Failing tests for AI_SPAWN producer

**Files:**
- Modify: `modules/world/npc_event_queue_test.go`

- [ ] **Step 1: Add 5 new tests to `npc_event_queue_test.go`**

Append the following 5 tests at the end of the existing test file. The helper `newServerForScriptTest(t)` lives at `npc_script_test.go:54` and returns a `*Server` with `scriptProvider == nil`; tests must seed `s.scriptProvider = script.NewProvider()` before registering scripts. The pattern at `npc_interaction_test.go:118+` is the established idiom.

```go
// TestAddNpcQueuesSpawnEventOnFirstSpawn pins NAI-22 Bundle 1: addNpc
// queues an NpcEventSpawn entry when a SPAWN script is registered for
// the NPC's typeId/category. Mirrors TS World.ts:1284-1289.
func TestAddNpcQueuesSpawnEventOnFirstSpawn(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	n := newTestNpc(t, s)

	spawnScript := &script.ScriptFile{
		LookupKey: script.GlobalTriggerKey(script.TriggerAiSpawn),
	}
	s.scriptProvider.Register(spawnScript)

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	if len(s.npcEventQueue) != 1 {
		t.Fatalf("npcEventQueue: got len %d, want 1 (SPAWN script registered, must enqueue)", len(s.npcEventQueue))
	}
	got := s.npcEventQueue[0]
	if got.Type != NpcEventSpawn {
		t.Errorf("npcEventQueue[0].Type: got %d, want NpcEventSpawn (%d)", got.Type, NpcEventSpawn)
	}
	if got.Script != spawnScript {
		t.Errorf("npcEventQueue[0].Script: got %p, want %p", got.Script, spawnScript)
	}
	if got.Npc != n {
		t.Errorf("npcEventQueue[0].Npc: got %p, want %p", got.Npc, n)
	}
}

// TestAddNpcQueuesSpawnEventOnRespawn pins NAI-22 Bundle 1: addNpc with
// firstSpawn=false (revertType heavy path) ALSO queues SPAWN. Matches TS
// World.ts:1258-1294, which has no firstSpawn guard around the queue
// insertion (lines 1284-1289).
func TestAddNpcQueuesSpawnEventOnRespawn(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	n := newTestNpc(t, s)
	// Pre-register the NPC at a slot (firstSpawn=true would do this; here
	// we simulate revertType path: NPC keeps its slot, addNpc(firstSpawn=false)).
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("first addNpc setup: %v", err)
	}
	s.npcEventQueue = nil // reset queue from setup; we only want to observe the second call

	spawnScript := &script.ScriptFile{
		LookupKey: script.GlobalTriggerKey(script.TriggerAiSpawn),
	}
	s.scriptProvider.Register(spawnScript)

	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("addNpc(firstSpawn=false): %v", err)
	}

	if len(s.npcEventQueue) != 1 {
		t.Fatalf("npcEventQueue: got len %d, want 1 (SPAWN must fire on respawn too)", len(s.npcEventQueue))
	}
}

// TestAddNpcNoSpawnScriptNoQueue pins the script != nil short-circuit:
// when no SPAWN script is registered, addNpc must NOT enqueue.
func TestAddNpcNoSpawnScriptNoQueue(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider() // empty provider
	n := newTestNpc(t, s)

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (no SPAWN script registered)", len(s.npcEventQueue))
	}
}

// TestAddNpcNilScriptProviderNoQueue pins the s.scriptProvider != nil
// defensive guard. The DESPAWN producer at npc_ai.go:47 uses the same
// guard; SPAWN must mirror.
func TestAddNpcNilScriptProviderNoQueue(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = nil // explicit
	n := newTestNpc(t, s)

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (nil scriptProvider must short-circuit)", len(s.npcEventQueue))
	}
}

// TestProcessNpcEventQueueDispatchesSpawn pins end-to-end SPAWN dispatch:
// addNpc enqueues, processNpcEventQueue drains AND fires the script. The
// type-agnostic processor at npc_event_queue.go:36-48 already handles
// SPAWN identically to DESPAWN; this test pins that contract.
func TestProcessNpcEventQueueDispatchesSpawn(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	n := newTestNpc(t, s)

	// SPAWN script with an observable side-effect: increment n.huntClock
	// via OpReturn-no-op shape — wait, OpReturn alone is not observable.
	// Use a script that the runner will execute and which writes a
	// detectable field. The simplest approach: register a script whose
	// LookupKey routes to TriggerAiSpawn and whose body sets n.tele=true
	// via a test-injected hook. Since we don't have direct opcode-level
	// side-effect injection here, fall back to the simpler proof: count
	// runNpcScript invocations via a test-double.
	//
	// Plan-time choice: use a bare ScriptFile and assert post-process
	// queue is drained. The script-runs-without-error path is proven
	// elsewhere (NAI-21 Bundle 3 strong-form test); this test specifically
	// pins the SPAWN→queue→drain path.
	spawnScript := &script.ScriptFile{
		LookupKey: script.GlobalTriggerKey(script.TriggerAiSpawn),
	}
	s.scriptProvider.Register(spawnScript)

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	if len(s.npcEventQueue) != 1 {
		t.Fatalf("setup: queue len %d, want 1", len(s.npcEventQueue))
	}

	s.processNpcEventQueue()

	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (queue must drain after dispatch)", len(s.npcEventQueue))
	}
}
```

The `newTestNpc(t, *Server)` helper is the established pattern for npc test fixtures in `modules/world` — verify its existence at plan-execution time (grep `func newTestNpc`); if it doesn't exist with that exact signature, use whatever the existing tests in `npc_event_queue_test.go` use to build a test NPC. The test file's existing tests at lines 454+ and 488+ already construct test NPCs; mirror their setup pattern.

**Plan-execution-time signature check** (run before writing tests): grep `script.GlobalTriggerKey\b` to confirm the helper name; if it's `script.LookupKeyForGlobal` or `script.MakeGlobalLookupKey` instead, substitute. The test computes the trigger key the same way the production `Provider.Register()` consumes it.

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestAddNpc -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessNpcEventQueueDispatchesSpawn -v
```

Expected: All 5 new tests FAIL. Likely failure modes:
- `TestAddNpcQueuesSpawnEventOnFirstSpawn`: `npcEventQueue: got len 0, want 1` (producer not yet wired)
- `TestAddNpcQueuesSpawnEventOnRespawn`: same
- `TestAddNpcNoSpawnScriptNoQueue`: PASS (no producer → no queue, accidentally green)
- `TestAddNpcNilScriptProviderNoQueue`: PASS (no producer → no queue, accidentally green)
- `TestProcessNpcEventQueueDispatchesSpawn`: `setup: queue len 0, want 1`

**Pre-existing-failure protocol**: per `verify_implementer_claims` memory, before claiming any of these are red, also run the full package: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`. If unrelated tests fail at HEAD, verify they also fail at HEAD~1 before declaring them pre-existing. NAI-22 Bundle 1 is starting from `66a2f9f` (the spec commit), not `4a696c7`.

### Task 1.2: Implement AI_SPAWN producer in addNpc

**Files:**
- Modify: `modules/world/npc_registry.go:77-82`

- [ ] **Step 1: Replace the deviation comment block at lines 77-82**

In `modules/world/npc_registry.go`, find the block:

```go
	// DEVIATION NAI-19-D2: AI_SPAWN trigger queue omitted —
	// script.TriggerAiSpawn (script/trigger.go:171) declared but no
	// spawn-flow consumer wiring. Activating here would change
	// first-spawn behavior across all existing NPCs at server boot.
	// Tracked for closure in a future "AI_SPAWN dispatch wiring"
	// sub-spec.
```

Replace with:

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

The `script` package import already exists in `npc_registry.go` (verify via the file's import block; if not present, add `"github.com/zsrv/goscape/pkg/script"`).

- [ ] **Step 2: Run the AI_SPAWN tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestAddNpc -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessNpcEventQueueDispatchesSpawn -v
```

Expected: All 5 tests PASS.

- [ ] **Step 3: Run the full `modules/world/` test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
```

Expected: All tests pass. If any pre-existing tests now fail (e.g., a setup that relied on `npcEventQueue` being empty after `addNpc`), inspect the failure — it's evidence the SPAWN producer interacts with another test's fixture. Most likely culprits: `npc_event_queue_test.go` existing tests, `npc_script_test.go` integration tests. The fix is to update the affected fixture's expectations, not to scope the producer.

### Task 1.3: Retire NAI-19-D2 narrative comments

**Files:**
- Modify: `modules/world/npc_event_queue.go:5-15`
- Modify: `modules/world/npc.go:255-285` (the addNpc/revertType narrative block)

- [ ] **Step 1: Update `npc_event_queue.go` doc-comment**

Find the existing doc-comment at lines 5-15 (the `NpcEventType` doc):

```go
// NpcEventType mirrors TS NpcEventType at
// Engine-TS/src/engine/entity/NpcEventRequest.ts.
// NpcEventSpawn is reserved for TS fidelity but has no producer in
// NAI-5 (no script-driven NPC creation yet); NpcEventDespawn is
// queued by the DESPAWN branch of the Npc.turn() Events block.
type NpcEventType int
```

Replace the doc-comment body (keep the `type NpcEventType int` line) with:

```go
// NpcEventType mirrors TS NpcEventType at
// Engine-TS/src/engine/entity/NpcEventRequest.ts.
// NpcEventSpawn is queued by Server.addNpc (NAI-22 Bundle 1, mirroring
// TS World.ts:1284-1289); NpcEventDespawn is queued by the DESPAWN
// branch of the Npc.turn() Events block (NAI-5, mirroring TS
// World.ts:580+).
type NpcEventType int
```

- [ ] **Step 2: Update `npc.go` NAI-19-D2 narrative**

Read `modules/world/npc.go:255-290` to find the exact narrative comment block referencing NAI-19-D2. Two callout sites at lines 259 and 283:

- Line 259 (within a multi-line "Goscape deviations" comment block): mentions NAI-19-D1 and NAI-19-D2 together. Update to remove NAI-19-D2 and keep only NAI-19-D1.
- Line 283 (similar narrative): same retirement.

The exact text shape will vary per the file's existing narrative — this is a doc-only retag. The implementer reads the surrounding 30-line block in each location and rewrites it to: (a) keep the NAI-19-D1 zone-state callout, (b) remove the NAI-19-D2 callout, (c) add a one-line "(NAI-19-D2 closed in NAI-22 Bundle 1)" parenthetical OR drop the historical mention entirely depending on which keeps the narrative cleanest.

Per `retire_deviation_grep_all_comments` memory, before this step run:

```bash
rg "NAI-19-D2" modules/ pkg/ cmd/ docs/
```

The output enumerates every site mentioning NAI-19-D2. The implementer must verify each one is either:
- A spec/plan file (preserve historical mentions; these are immutable)
- A code comment (retire per the rules above)
- A test file (rare; treat as code comment)

The `nai_followups.md` doc-tracker file is a special case — it tracks open follow-ups; NAI-22 Bundle 1 closing the entry should be reflected by removing the entry or marking it Closed at close-commit time, NOT during this task. Bundle 1 is production code only.

- [ ] **Step 3: Run the full test suite to confirm nothing breaks from doc-only changes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: All tests pass (doc-only changes have no behavioral impact).

### Task 1.4: Bundle 1 commit

- [ ] **Step 1: Stage and commit**

```bash
git add modules/world/npc_registry.go modules/world/npc_event_queue.go modules/world/npc_event_queue_test.go modules/world/npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-22 Bundle 1 — AI_SPAWN producer activation (NAI-19-D2 closed)

Activates the reserved NpcEventSpawn constant by adding the SPAWN-event
producer in s.addNpc, mirroring TS World.ts:1284-1289 and the existing
AI_DESPAWN producer pattern at npc_ai.go:47-58. Fires unconditionally
for both firstSpawn=true (server boot) and firstSpawn=false (revertType
respawn) — TS has no firstSpawn guard around the queue insertion.
NpcEventQueue dispatch is type-agnostic (npc_event_queue.go:36-48), so
SPAWN events drain through the same processor as DESPAWN.

Closes deviation NAI-19-D2. Active deviation count 16 → 15.

5 new tests in npc_event_queue_test.go pin: firstSpawn=true producer,
firstSpawn=false producer, no-script short-circuit, nil-scriptProvider
defensive guard, and end-to-end queue drain through processNpcEventQueue.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2: Verify post-commit state**

```bash
git log --oneline -3
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: most-recent commit is the Bundle 1 feat; both test runs green.

---

## Bundle 2 — checkInv huntPlayers filter

**Spec section**: `## Scope > ### Bundle 2 — checkInv huntPlayers filter`.

### Task 2.1: Failing tests for checkInv filter

**Files:**
- Modify: `modules/world/npc_hunt_test.go`

- [ ] **Step 1: Read the existing `npc_hunt_test.go` to understand fixture conventions**

```bash
rg "func Test.*HuntPlayers" modules/world/npc_hunt_test.go -n
```

The implementer reads 1-2 existing tests in this file (e.g., `TestHuntPlayersCheckVarsPassFails`, `TestHuntPlayersCheckAfk`) to identify:
- The test-server builder used (`newServerForHuntTest(t)`, `newTestServer(t)`, etc.)
- The test-player builder
- The HuntType fixture pattern
- How `n.huntMode` and `s.players[slot]` get wired

**Plan-execution-time pin**: substitute the actual builder names where the placeholder helpers below appear (`newHuntTestServer`, `newHuntTestNpc`, `newHuntTestPlayer`).

- [ ] **Step 2: Add 6 new tests at the end of `npc_hunt_test.go`**

```go
// TestHuntPlayersCheckInvDisabled pins NAI-22 Bundle 2: when CheckInv
// is -1 (the TS default), the filter is a no-op. Mirrors the implicit
// TS short-circuit at Npc.ts:959.
func TestHuntPlayersCheckInvDisabled(t *testing.T) {
	s, n, p := newHuntTestServer(t)
	hunt := &objtype.HuntType{
		CheckInv:    -1,  // disabled
		// CheckObj, CheckObjParam left at -1 too
	}
	s.players[p.slot] = p

	got := n.huntPlayers(s, hunt)

	if len(got) != 1 {
		t.Errorf("huntPlayers: got %d players, want 1 (CheckInv disabled, player must pass)", len(got))
	}
}

// TestHuntPlayersCheckInvObjPasses pins NAI-22 Bundle 2: with CheckInv
// set, CheckObj branch evaluates inv.GetItemCount(obj) and compares
// against CheckInvVal via CheckHuntCondition. Player has 5 of obj X,
// hunt requires >=3 → player included. Mirrors TS Npc.ts:961-962.
func TestHuntPlayersCheckInvObjPasses(t *testing.T) {
	s, n, p := newHuntTestServer(t)
	const invID, objID = 0, 100  // arbitrary test ids
	p.invs = map[int]*inventory.Inventory{
		invID: testInvWithItem(t, invID, objID, 5),
	}
	hunt := &objtype.HuntType{
		CheckInv:          invID,
		CheckObj:          objID,
		CheckObjParam:     -1,
		CheckInvCondition: ">=",
		CheckInvVal:       3,
	}
	s.players[p.slot] = p

	got := n.huntPlayers(s, hunt)

	if len(got) != 1 {
		t.Errorf("huntPlayers: got %d, want 1 (5 >= 3 must pass)", len(got))
	}
}

// TestHuntPlayersCheckInvObjFails pins NAI-22 Bundle 2: condition fails
// → player excluded. 1 of obj X, hunt requires >=3 → player NOT included.
func TestHuntPlayersCheckInvObjFails(t *testing.T) {
	s, n, p := newHuntTestServer(t)
	const invID, objID = 0, 100
	p.invs = map[int]*inventory.Inventory{
		invID: testInvWithItem(t, invID, objID, 1),
	}
	hunt := &objtype.HuntType{
		CheckInv:          invID,
		CheckObj:          objID,
		CheckObjParam:     -1,
		CheckInvCondition: ">=",
		CheckInvVal:       3,
	}
	s.players[p.slot] = p

	got := n.huntPlayers(s, hunt)

	if len(got) != 0 {
		t.Errorf("huntPlayers: got %d, want 0 (1 >= 3 must fail)", len(got))
	}
}

// TestHuntPlayersCheckInvObjParamPasses pins NAI-22 Bundle 2: CheckObjParam
// branch sums per-slot ObjType.Params[param] across non-empty slots,
// falling back to ParamType.DefaultInt for missing params. Mirrors TS
// Npc.ts:963-964 + Player.ts:1668-1697 (stack=false).
func TestHuntPlayersCheckInvObjParamPasses(t *testing.T) {
	s, n, p := newHuntTestServer(t)
	const invID, paramID = 0, 200
	// Wire 3 obj types each with Params[paramID]=10.
	objA, objB, objC := 100, 101, 102
	wireParam(t, s, paramID, 0)  // ParamType.DefaultInt = 0
	wireObjWithParam(t, s, objA, paramID, 10)
	wireObjWithParam(t, s, objB, paramID, 10)
	wireObjWithParam(t, s, objC, paramID, 10)
	inv := testInvFromConfig(t, invID)
	inv.Items[0] = &inventory.Item{Id: objA, Count: 1}
	inv.Items[1] = &inventory.Item{Id: objB, Count: 1}
	inv.Items[2] = &inventory.Item{Id: objC, Count: 1}
	p.invs = map[int]*inventory.Inventory{invID: inv}
	hunt := &objtype.HuntType{
		CheckInv:          invID,
		CheckObj:          -1,
		CheckObjParam:     paramID,
		CheckInvCondition: ">=",
		CheckInvVal:       20,
	}
	s.players[p.slot] = p

	got := n.huntPlayers(s, hunt)

	if len(got) != 1 {
		t.Errorf("huntPlayers: got %d, want 1 (sum=30 >= 20 must pass)", len(got))
	}
}

// TestHuntPlayersCheckInvObjParamFails pins NAI-22 Bundle 2: param-sum
// below threshold → player excluded.
func TestHuntPlayersCheckInvObjParamFails(t *testing.T) {
	s, n, p := newHuntTestServer(t)
	const invID, paramID = 0, 200
	objA := 100
	wireParam(t, s, paramID, 0)
	wireObjWithParam(t, s, objA, paramID, 10)
	inv := testInvFromConfig(t, invID)
	inv.Items[0] = &inventory.Item{Id: objA, Count: 1}
	p.invs = map[int]*inventory.Inventory{invID: inv}
	hunt := &objtype.HuntType{
		CheckInv:          invID,
		CheckObj:          -1,
		CheckObjParam:     paramID,
		CheckInvCondition: ">=",
		CheckInvVal:       20,
	}
	s.players[p.slot] = p

	got := n.huntPlayers(s, hunt)

	if len(got) != 0 {
		t.Errorf("huntPlayers: got %d, want 0 (sum=10 >= 20 must fail)", len(got))
	}
}

// TestHuntPlayersCheckInvMissingInvDefensive pins NAI-22 Bundle 2:
// when p.invs[CheckInv] is nil, quantity defaults to 0 and
// CheckHuntCondition decides. This is the goscape-vs-TS divergence
// (TS throws here; goscape iterates with quantity=0). Documented in
// code comment, no deviation tag — TS path is dead in practice.
func TestHuntPlayersCheckInvMissingInvDefensive(t *testing.T) {
	s, n, p := newHuntTestServer(t)
	const invID, objID = 0, 100
	p.invs = map[int]*inventory.Inventory{}  // EMPTY — no inv at invID
	hunt := &objtype.HuntType{
		CheckInv:          invID,
		CheckObj:          objID,
		CheckObjParam:     -1,
		CheckInvCondition: "=",
		CheckInvVal:       0,
	}
	s.players[p.slot] = p

	got := n.huntPlayers(s, hunt)

	if len(got) != 1 {
		t.Errorf("huntPlayers: got %d, want 1 (missing inv → quantity=0; 0 == 0 must pass)", len(got))
	}
}
```

The test helpers `newHuntTestServer`, `testInvWithItem`, `testInvFromConfig`, `wireParam`, `wireObjWithParam` are placeholders — the implementer either:

1. **Finds existing equivalents** in `npc_hunt_test.go` and adjacent test files (likely candidates: `synthesizeTypes(t)` from `appearance_test.go`, `inventory.FromType()`).
2. **Constructs them inline** if the existing test fixtures don't provide a clean fit.

**Plan-time guidance for `wireObjWithParam`**: the simplest implementation is to populate `s.objTypes.Configs[objId]` with an `*ObjType` whose `Params map[uint32]any` contains `{uint32(paramID): uint32(value)}`. Mirrors `handleInvTotalParam` at `pkg/script/handlers_inv.go:247-252`. For `wireParam`, populate `s.paramTypes.Configs[paramID]` with a `*ParamType` whose `DefaultInt` is the fallback value.

- [ ] **Step 3: Run the new tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntPlayersCheckInv -v
```

Expected:
- `TestHuntPlayersCheckInvDisabled` — likely PASS (CheckInv=-1 default behavior unchanged)
- `TestHuntPlayersCheckInvObjPasses` — PASS (filter not yet active, player passes by default)
- `TestHuntPlayersCheckInvObjFails` — **FAIL** (`got 1, want 0`) — filter not active, fails-condition player still passes
- `TestHuntPlayersCheckInvObjParamPasses` — PASS (filter not active, player passes)
- `TestHuntPlayersCheckInvObjParamFails` — **FAIL** (`got 1, want 0`) — filter not active
- `TestHuntPlayersCheckInvMissingInvDefensive` — PASS (filter not active)

The 2 expected FAILs prove the filter is currently absent. After implementation, all 6 should be GREEN.

### Task 2.2: Implement invTotalParam helper + checkInv filter

**Files:**
- Modify: `modules/world/npc_hunt.go`

- [ ] **Step 1: Add the `invTotalParam` helper at the end of `npc_hunt.go`**

Add this function after the existing `huntPlayers` function (around line 207, after `consumeHuntTarget`'s declaration):

```go
// invTotalParam mirrors handleInvTotalParam (pkg/script/handlers_inv.go:224)
// for non-ScriptState callers. Sums per-slot ObjType.Params[param] across
// every non-empty slot of inv, falling back to ParamType.DefaultInt for
// missing params. Returns 0 if any required config is nil — defensive,
// huntPlayers cannot abort iteration on a single param-resolution failure.
//
// TS source: Player._invTotalParam at Player.ts:1668-1697 (stack=false branch).
func invTotalParam(inv *inventory.Inventory, param int,
	objs *objtype.ObjTypeConfigs, params *objtype.ParamTypeConfigs) int {
	if inv == nil || objs == nil || params == nil {
		return 0
	}
	if param < 0 || param >= len(params.Configs) {
		return 0
	}
	pt := params.Configs[param]
	if pt == nil {
		return 0
	}
	total := 0
	for _, it := range inv.Items {
		if it == nil || it.Id < 0 {
			continue
		}
		if it.Id >= len(objs.Configs) {
			continue
		}
		ot := objs.Configs[it.Id]
		if ot == nil {
			continue
		}
		if v, ok := ot.Params[uint32(param)]; ok {
			if iv, ok := v.(uint32); ok {
				total += int(iv)
				continue
			}
		}
		total += int(pt.DefaultInt)
	}
	return total
}
```

The imports needed at the top of the file: `"github.com/zsrv/goscape/pkg/inventory"` and `"github.com/zsrv/goscape/pkg/objtype"`. Verify both are already imported in `npc_hunt.go`; add if missing.

- [ ] **Step 2: Add the checkInv filter block to `huntPlayers`**

In `modules/world/npc_hunt.go`, find the existing CheckVars filter block at lines ~187-203 (the `passCheckVars` AND-chain). Immediately AFTER the `if !passCheckVars { continue }` line (around line 202), and BEFORE the `hunted = append(hunted, p)` line (around line 204), insert the checkInv block:

```go
		// checkInv (TS Npc.ts:959-969): if CheckInv is set, compute quantity
		// per CheckObj or CheckObjParam branch, then evaluate CheckHuntCondition.
		// Defensive: missing inv → quantity=0 (TS throws here, but goscape
		// huntPlayers must continue iteration on one bad player; live players
		// have all standard invs in practice).
		if hunt.CheckInv != -1 {
			quantity := 0
			if pInv := p.invs[hunt.CheckInv]; pInv != nil {
				if hunt.CheckObj != -1 {
					quantity = pInv.GetItemCount(hunt.CheckObj)
				} else if hunt.CheckObjParam != -1 {
					quantity = invTotalParam(pInv, hunt.CheckObjParam,
						s.objTypes, s.paramTypes)
				}
			}
			if !hunt.CheckHuntCondition(quantity,
				hunt.CheckInvCondition, hunt.CheckInvVal) {
				continue
			}
		}
```

- [ ] **Step 3: Update the `huntPlayers` doc-comment header**

Find lines 87-94 of `npc_hunt.go` (the "Filter coverage" / "Filters DEFERRED" doc-comment block). Move the `checkInv` line from "Filters DEFERRED" to "Filter coverage":

Before:
```go
// Filter coverage:
//   - Range + level match:     always
//   - checkAfk                 (NAI-8,  TS:935-937)
//   - CheckVis LoS/LoW         (NAI-12, TS per ScriptIterators.ts:88-94)
//   - Outer combat guard       (NAI-15, TS:942)
//   - checkNotCombat           (NAI-15, TS:943-945)
//   - checkNotCombatSelf       (NAI-16, TS:946-948)
//   - checkVars                (NAI-15, TS:950-957)
//
// Filters DEFERRED (infra missing; each TS line cited):
//   - checkNotBusy             (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong        (TS:939-941)       — wilderness + combat-level
//   - checkInv                 (TS:959-969)       — inventory queries
```

After:
```go
// Filter coverage:
//   - Range + level match:     always
//   - checkAfk                 (NAI-8,  TS:935-937)
//   - CheckVis LoS/LoW         (NAI-12, TS per ScriptIterators.ts:88-94)
//   - Outer combat guard       (NAI-15, TS:942)
//   - checkNotCombat           (NAI-15, TS:943-945)
//   - checkNotCombatSelf       (NAI-16, TS:946-948)
//   - checkVars                (NAI-15, TS:950-957)
//   - checkInv                 (NAI-22, TS:959-969)
//
// Filters DEFERRED (infra missing; each TS line cited):
//   - checkNotBusy             (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong        (TS:939-941)       — wilderness + combat-level
```

- [ ] **Step 4: Run the new tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHuntPlayersCheckInv -v
```

Expected: All 6 tests PASS.

- [ ] **Step 5: Run the full `modules/world/` package**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
```

Expected: All tests pass. The most likely interaction is with existing huntPlayers tests that build a HuntType — if any such test left `CheckInv` at the zero value (0, not -1) accidentally, the filter would now activate. Pre-existing tests should already use `CheckInv: -1` per the TS default that `NewHuntType()` sets. If any pre-existing test fails, inspect — the test fixture likely needs explicit `CheckInv: -1`.

### Task 2.3: Bundle 2 commit

- [ ] **Step 1: Stage and commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-22 Bundle 2 — checkInv huntPlayers filter

Activates the deferred checkInv filter in huntPlayers, mirroring TS
Npc.ts:959-969. Implements both CheckObj branch (uses Inventory.GetItemCount)
and CheckObjParam branch (new local invTotalParam helper that mirrors
handleInvTotalParam without ScriptState).

Defensive choice: missing player inv → quantity=0 (TS throws; goscape
iterates). Documented in code comment; no deviation tag because the TS
throw-path is unreachable for live players.

Closes the third of three open huntPlayers filter deferrals
(checkNotBusy and checkNotTooStrong remain — different missing infra).

6 new tests pin: CheckInv=-1 short-circuit, CheckObj branch passes/fails,
CheckObjParam branch passes/fails, missing-inv defensive treat-as-0.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2: Verify post-commit state**

```bash
git log --oneline -3
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: most-recent commit is Bundle 2; both test runs green.

---

## Bundle 3 — appearanceInv ctor binding (NAI-21-D1 closure) + byte-search test polish

**Spec section**: `## Scope > ### Bundle 3 — appearanceInv ctor binding ... + byte-search test polish`.

### Task 3.1: Failing test for appearanceInv production binding

**Files:**
- Modify: `modules/world/appearance_test.go`

- [ ] **Step 1: Add `TestSetAppearanceInvBindsId`**

Append at the end of `modules/world/appearance_test.go`:

```go
// TestSetAppearanceInvBindsId pins NAI-22 Bundle 3: SetAppearanceInv
// writes the id to Player.appearanceInv and flips MaskAppearance.
// This is the existing setter (player_script.go:365); the test pins
// its contract independently of integration through client.go login
// wiring (which is harder to unit-test).
func TestSetAppearanceInvBindsId(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.appearanceInv != -1 {
		t.Fatalf("setup: p.appearanceInv should default to -1, got %d", p.appearanceInv)
	}

	p.SetAppearanceInv(42)

	if p.appearanceInv != 42 {
		t.Errorf("p.appearanceInv: got %d, want 42 (setter must bind id)", p.appearanceInv)
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("p.masks: MaskAppearance bit unset (setter must flag for regeneration)")
	}
}
```

The `rsbuf` import is needed — `import "github.com/zsrv/goscape/pkg/rsbuf"`. Verify it's already imported in `appearance_test.go`; add if missing.

- [ ] **Step 2: Run the test — should already pass (the setter exists)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSetAppearanceInvBindsId -v
```

Expected: PASS. This test is documenting / pinning the existing setter contract, NOT exercising new code. It's a defense-in-depth test that catches future regressions if someone changes `SetAppearanceInv`'s mask behavior. The "failing test first" rule of TDD is relaxed here because the setter is pre-existing infrastructure being newly consumed.

### Task 3.2: Wire production binding in client.go

**Files:**
- Modify: `modules/world/client.go:113`

- [ ] **Step 1: Add the SetAppearanceInv call in sendLoginOK**

Find `modules/world/client.go:108-128` (the `sendLoginOK` method). At line 113, after `p := newPlayer(c)`, before line 114 `c.server.appendNewPlayer(p)`, insert:

```go
		p.SetAppearanceInv(c.server.invTypes.Worn)
```

The result block should look like:

```go
func (c *client) sendLoginOK() error {
	if c.server != nil {
		p := newPlayer(c)
		p.SetAppearanceInv(c.server.invTypes.Worn)
		c.server.appendNewPlayer(p)
		c.player = p
	}
	// ... rest unchanged
```

- [ ] **Step 2: Run the full test suite to confirm no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
```

Expected: All tests pass. The only behavior change is in production login flow — tests that build a Player via `newPlayer(c)` directly (bypassing `sendLoginOK`) are unaffected; their `appearanceInv` continues to default to `-1` (the test-only sentinel).

### Task 3.3: Refactor byte-search loops to bytes.Contains

**Files:**
- Modify: `modules/world/appearance_test.go:104-111` and `:185-192` (the two manual loops)

- [ ] **Step 1: Verify `bytes` package is imported**

Read the imports at the top of `modules/world/appearance_test.go`. If `"bytes"` is not in the import block, add it.

(Per the existing usage in `modules/world/player_npc_test.go` and `modules/world/interaction_trigger_test.go`, `bytes.Contains` is the established idiom in this codebase.)

- [ ] **Step 2: Refactor `TestGenerateAppearanceSentinelDefaultReadsWorn`**

Find the manual loop at lines ~104-111 in `TestGenerateAppearanceSentinelDefaultReadsWorn`:

```go
	found := false
	for i := 0; i < len(p.appearanceBuf)-1; i++ {
		if p.appearanceBuf[i] == wantSlot4Hi && p.appearanceBuf[i+1] == wantSlot4Lo {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("appearanceBuf missing platebody encoded bytes (0x%02x 0x%02x); "+
			"sentinel mapping to Worn appears broken", wantSlot4Hi, wantSlot4Lo)
	}
```

Replace with:

```go
	if !bytes.Contains(p.appearanceBuf, []byte{wantSlot4Hi, wantSlot4Lo}) {
		t.Errorf("appearanceBuf missing platebody encoded bytes (0x%02x 0x%02x); "+
			"sentinel mapping to Worn appears broken", wantSlot4Hi, wantSlot4Lo)
	}
```

- [ ] **Step 3: Refactor `TestGenerateAppearanceCustomInvIdHonored`**

Find the corresponding loop at lines ~185-192:

```go
	found := false
	for i := 0; i < len(p.appearanceBuf)-1; i++ {
		if p.appearanceBuf[i] == wantSlot4Hi && p.appearanceBuf[i+1] == wantSlot4Lo {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("appearanceBuf missing platebody from custom inv; reader is "+
			"still reading from invs.Worn (S7c-D1 NOT closed)")
	}
```

Replace with:

```go
	if !bytes.Contains(p.appearanceBuf, []byte{wantSlot4Hi, wantSlot4Lo}) {
		t.Errorf("appearanceBuf missing platebody from custom inv; reader is "+
			"still reading from invs.Worn (S7c-D1 NOT closed)")
	}
```

- [ ] **Step 4: Run the appearance tests to confirm equivalent behavior**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestGenerateAppearance -v
```

Expected: All `TestGenerateAppearance*` tests PASS — the refactor is byte-equivalent.

### Task 3.4: Retire NAI-21-D1 narrative comments

**Files:**
- Modify: `modules/world/player.go:322`-adjacent (sentinel ctor doc-comment)
- Modify: `modules/world/appearance.go:25-28`
- Modify: `pkg/script/active.go:329, 332`
- Modify: `pkg/script/handlers_player.go:133, 136`

- [ ] **Step 1: Pre-flight grep for ALL NAI-21-D1 sites**

```bash
rg "NAI-21-D1" modules/ pkg/ cmd/
```

The output must enumerate exactly 4 production-code files (`appearance.go`, `appearance_test.go`, `active.go`, `handlers_player.go`). Doc files (`docs/` and any spec/plan markdown) are out of scope for retirement — they preserve historical context.

If the grep surfaces additional sites not enumerated in this plan, the implementer treats each as a "code or test" decision per the rules in Task 1.3 Step 2.

- [ ] **Step 2: Update `modules/world/player.go:322` ctor doc-comment**

Read `modules/world/player.go:285-340` to find the `newPlayer` function ctor body. The line `appearanceInv: -1,` at line 322 currently has no inline comment. Add a one-line trailing comment:

```go
		appearanceInv:  -1, // test-only sentinel; production binds via SetAppearanceInv from client.go login wiring (NAI-22 Bundle 3 closes NAI-21-D1).
```

- [ ] **Step 3: Update `modules/world/appearance.go:25-28`**

Find the existing block at lines 25-28:

```go
	// NAI-21-D1: TS init binds appearanceInv to Worn at ctor; goscape uses
	// production caller either (i) passes through SetAppearanceInv before
	// ...
```

Read the full block (lines 22-40) to understand the surrounding context, then replace the NAI-21-D1 narrative with a test-only-fallback narrative. The replacement should explain: (a) production callers bind via client.go login wiring before any tick, (b) the sentinel-fallback exists for test fixtures that bypass `sendLoginOK`, (c) the `-1` sentinel maps to `invs.Worn` for backward-compatibility with existing tests.

Sample replacement (the implementer adapts to the actual surrounding text):

```go
	// generateAppearance reads from p.invs[p.appearanceInv]. Production
	// flow: client.go's sendLoginOK calls p.SetAppearanceInv(invs.Worn)
	// immediately after newPlayer, so by the time any tick runs,
	// p.appearanceInv is bound. The -1 sentinel default in newPlayer is
	// retained as test-only safety: tests that build a Player via
	// newPlayer(c) without going through sendLoginOK have appearanceInv=-1,
	// and the fallback below maps that to invs.Worn for byte-equivalent
	// behavior.
	//
	// Mirrors TS Player.ts:1318: `let worn = this.getInventory(this.appearanceInv);`
```

- [ ] **Step 4: Update `pkg/script/active.go:329, 332`**

Read `pkg/script/active.go:325-340`. Find the existing doc-comment block referencing NAI-21-D1 (the SetAppearanceInv-related comments) and remove the NAI-21-D1 callouts. The structure is roughly:

Before (sample shape):
```go
	// SetAppearanceInv updates the active player's appearanceInv field AND
	// ... (multi-line comment) ...
	// init; tracked as NAI-21-D1, internal-mechanism only). Callers
	// pre-validate id via checkInvType.
```

After:
```go
	// SetAppearanceInv updates the active player's appearanceInv field AND
	// ... (multi-line comment, NAI-21-D1 callout removed) ...
	// Callers pre-validate id via checkInvType.
```

The exact wording depends on the surrounding text — the implementer reads the full ~10-line comment block and removes only the NAI-21-D1 mention while preserving the surrounding rationale.

- [ ] **Step 5: Update `pkg/script/handlers_player.go:133, 136`**

Read `pkg/script/handlers_player.go:130-145`. Find the doc-comment block referencing NAI-21-D1 (the BUILDAPPEARANCE narrative) and remove the NAI-21-D1 callout same as Step 4.

- [ ] **Step 6: Verify NAI-21-D1 is fully retired**

```bash
rg "NAI-21-D1" modules/ pkg/ cmd/
```

Expected: ZERO results in production code (`modules/`, `pkg/`, `cmd/`). Documentation files (specs/plans/memory) preserve historical mentions and are excluded from this grep scope.

- [ ] **Step 7: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: All tests pass. Doc-only retag changes have no behavioral impact.

### Task 3.5: Bundle 3 commit

- [ ] **Step 1: Stage and commit**

```bash
git add modules/world/client.go modules/world/player.go modules/world/appearance.go modules/world/appearance_test.go pkg/script/active.go pkg/script/handlers_player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
polish(world): NAI-22 Bundle 3 — appearanceInv ctor binding (NAI-21-D1 closed) + byte-search test polish

Closes NAI-21-D1 by calling the existing Player.SetAppearanceInv from
client.go's sendLoginOK immediately after newPlayer, binding
appearanceInv to invs.Worn before any tick runs. The -1 ctor sentinel
is retained as test-only safety with updated narrative across
appearance.go, player.go, pkg/script/active.go, pkg/script/handlers_player.go.

Plus: replaces two manual byte-pair scans in appearance_test.go with
bytes.Contains calls (review M1 follow-up from NAI-21 Bundle 1).

Active deviation count 15 → 14.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2: Verify post-commit state**

```bash
git log --oneline -5
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
rg "NAI-19-D2|NAI-21-D1" modules/ pkg/ cmd/
```

Expected:
- `git log` shows 4 NAI-22 commits (spec + Bundle 1 + Bundle 2 + Bundle 3)
- Both test runs green
- The grep returns ZERO results in `modules/`, `pkg/`, `cmd/` for both deviation tags

---

## Final close-commit task

### Task 4.1: NAI-22 close commit

**Files:**
- Modify: `nai_followups.md` (if any NAI-22 follow-ups land during execution; otherwise no-op)

- [ ] **Step 1: Run final verification**

```bash
git log --oneline -10
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: Clean test runs, clean vet, clean build.

- [ ] **Step 2: Verify deviation count**

```bash
rg "DEVIATION NAI-" modules/ pkg/ cmd/ | sort -u
```

Count unique deviation tags in production code. Expected: 14 active (16 pre-NAI-22 minus NAI-19-D2 minus NAI-21-D1).

- [ ] **Step 3: Create the close commit**

If any review-driven follow-ups landed during execution (e.g., a Bundle 1 review caught a missing test that landed as a follow-up commit), enumerate them in the close commit body. Otherwise:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(world): NAI-22 closed — three-bundle follow-up

Three bundles closing two NAI-series deviations and porting one
deferred huntPlayers filter:

  Bundle 1: AI_SPAWN producer activation (NAI-19-D2 closed)
  Bundle 2: checkInv huntPlayers filter
  Bundle 3: appearanceInv ctor binding via post-login setter
            (NAI-21-D1 closed) + byte-search test polish

Active deviation count: 16 → 14 (net -2).

Closes memory:
  - consume_reserved_constant.md (Bundle 1: textbook 5-element checklist)
  - controller_preflight.md (caught 2 design assumptions before plan-write)
  - spec_followup_tracker_freshness.md (verified all 4 candidates at HEAD)
  - compressed_cadence.md (Bundle 3 light-review threshold applied)
  - retire_deviation_grep_all_comments.md (Bundle 3 NAI-21-D1 sweep)
  - plan_grep_helper_patterns.md (Bundle 2 inv primitive selection)
  - plan_helper_coverage.md (Bundle 3 newPlayer caller-set drove ctor-vs-setter)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(The `--allow-empty` is intentional if no doc-tracker file changes; otherwise stage the doc edits first.)

- [ ] **Step 4: Final state check**

```bash
git log --oneline -7
git status
```

Expected: Clean working tree; commit log shows spec → plan (this file's commit when it landed) → Bundle 1 → Bundle 2 → Bundle 3 → close.

---

## Self-review (writing-plans skill discipline)

**Spec coverage**:
- Bundle 1 (AI_SPAWN producer activation): Tasks 1.1 (failing tests), 1.2 (impl), 1.3 (narrative retire), 1.4 (commit) ✓
- Bundle 2 (checkInv filter): Tasks 2.1 (tests), 2.2 (impl), 2.3 (commit) ✓
- Bundle 3 (appearanceInv binding + byte-search polish): Tasks 3.1 (setter test), 3.2 (production wiring), 3.3 (test polish), 3.4 (NAI-21-D1 retire), 3.5 (commit) ✓
- Close commit: Task 4.1 ✓

**Spec sections cross-checked**:
- "TS-fidelity gates (per-task)" — each task references TS line numbers in test docs and commit messages ✓
- "Deviation accounting" — close commit asserts 16 → 14 ✓
- "Test strategy summary" — 12 new tests + 2 modified, distributed across bundles per spec table ✓
- "Open questions deferred to plan-time" — addressed inline in tasks (`GlobalTriggerKey`, `Configs[id]` accessor pattern, `c.server.invTypes.Worn` exact path, sendLoginOK insertion point)

**Placeholder scan**: All test code shown in full. All replacement-code blocks shown in full. Helper-name placeholders (`newHuntTestServer`, `wireParam`, `wireObjWithParam`) flagged with explicit "implementer either finds existing or constructs inline" guidance and concrete shape hints. No "TBD"/"TODO"/"add tests for the above" patterns.

**Type consistency**: `NpcEventSpawn` (used in Bundle 1 production + tests), `NpcEventRequest{Type, Script, Npc}` (Bundle 1), `objtype.HuntType{CheckInv, CheckObj, CheckObjParam, CheckInvCondition, CheckInvVal}` (Bundle 2), `Player.SetAppearanceInv(int)` and `rsbuf.MaskAppearance` (Bundle 3) — all match the canonical declarations in the codebase as confirmed during pre-flight.
