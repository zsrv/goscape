# NAI-128 Stage 1 — Rat death-loot cascade investigation (synthetic probe) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single Go test in `modules/world/nai128_rat_loot_test.go` that drives the rat death-loot cascade end-to-end against the real cache, asserting at each link, so the first failing assertion identifies the Stage-2 fix candidate (NAI-128 §4 of the design doc).

**Architecture:** One file, one top-level `TestNAI128_RatLootCascade`, three subtests phased in cascade order (preconditions → ai_queue2 dispatch → ai_queue3 + obj_adds). Uses real `data/pack/server` artifacts (script.dat, npc.dat, obj.dat, params, varp/varn, gamemap). Bypasses `[label,player_melee_attack]` invocation — that script's hit-roll branches on `randominc()` with no test-injection seam (verified at brainstorm: `pkg/script/handlers_number.go:301` calls `rand.IntN` from `math/rand/v2` global RNG). Instead, the probe directly mutates `rat.heroPoints` and `rat.queue` to simulate post-melee-attack state, then ticks `processNpcQueue` to exercise the death-loot cascade. The credit-side (NPC_HEROPOINTS handler correctness) is already covered by `modules/world/heropoints_test.go` and `pkg/script/handlers_npc_test.go`; this probe targets the death-loot cascade specifically.

**Plan vs spec divergence:** Spec §4.1 step 6 anticipates RNG-seam hit-roll determinism. Brainstorm pre-flight refuted that seam exists. Probe therefore bypasses player_melee_attack invocation (spec §4.2 Phase A's "invoke `[label,player_melee_attack]`") in favor of directly setting up the post-melee state. Cascade coverage is unchanged: Phases B+C still pin the death-loot side end-to-end. Phase A's NPC_HEROPOINTS credit-side correctness is left to existing handler tests + a small inline assertion that the ledger pre-mutation reads back correctly.

**Tech Stack:** Go 1.26+. `pkg/script` (Provider, ScriptState, Opcode), `pkg/objtype` (NPCTypeConfigs, ObjTypeConfigs, params, varp/varn types), `pkg/zone` (zoneMap.Get → Zone.Objs), `pkg/coordgrid`, `pkg/gamemap` (real Lumbridge cache), `modules/world` (newTestServer, newTestPlayer, NewNpc, processNpcQueue).

**Stage 2 is NOT in this plan.** Stage 2 fix tasks are data-dependent on Stage 1's binding result; once T6 emits the binding handoff doc, author a separate `2026-05-XX-nai-128-stage-2-<binding>.md` plan after `/clear`.

---

## File Structure

| File | Responsibility | Status |
|------|----------------|--------|
| `modules/world/nai128_rat_loot_test.go` | Single test file containing `TestNAI128_RatLootCascade` with three phased subtests + the cache-loading fixture helper. | Create |
| `docs/superpowers/handoffs/2026-05-XX-nai-128-stage-1-binding.md` | Stage-1 close note: which assertion failed (or all PASS), what candidate binding (A/B/C/D/E per spec §4.2), pasted test output excerpt, recommended Stage-2 fix shape. | Create at T6 |

No other production files are modified in this Stage-1 plan. If Stage 1 reveals a fix path of ≤~50 LOC, Stage 2 plan authors the production touch points then.

---

## Pre-flight context for the implementer

**Canonical fixture pattern (verified at brainstorm against `modules/world/nai101_fountain_test.go`, `modules/world/server_test.go:311`, `modules/world/npc_test.go:14`):**

- `s := newTestServer(t)` (server_test.go:311) — minimal Server with logger + scriptProvider stub + zoneMap. We REPLACE its `scriptProvider` with a real one and ADD npcTypes, objTypes, params, varpTypes, gamemap.
- `newTestPlayer(t)` (player_test.go:17) — returns `*Player, net.Conn`. Player needs `p.client.server = s` to wire to our server.
- `NewNpc(nid, typeId, x, z, level, typ)` (npc.go:159) — constructs an Npc; `n.uid = (typeId << 16) | nid`. HP lives at `n.levels[objtype.NpcStatHitpoints]` (verified npc.go:141 + npctype.go).
- `n.heroPoints` (npc.go:149, value type `HeroPoints`) — package-internal access from same-package test. Use `n.heroPoints.AddHero(uid, amount)` to credit; `n.heroPoints.TopContributor()` to read.
- `n.EnqueueScriptForTrigger(trigger, delay, lastIntArg)` (npc.go:335) — appends to `n.queue` (the same field `processNpcQueue` reads at npc_script.go:502).
- `s.processNpcQueue(n)` (npc_script.go:497) — drains all entries with `Delay <= 0` in one call. Re-entrant: an enqueue that happens DURING handler execution (via the script runner calling NPC_QUEUE) appends to `n.queue`; the loop's `for i < len(n.queue)` re-reads length each iteration, so a `Delay=0` re-entry fires within the SAME `processNpcQueue` call. **This means ai_queue2 firing → npc_default_damage running NPC_QUEUE(3,0,0) → ai_queue3 firing all happen in ONE `processNpcQueue` call.**
- `s.zoneMap.Get(level, x, z)` returns `*zone.Zone`; `z.Objs []*entity.Obj` is the slice of dynamic objs (verified zone.go:253-270).

**Cache loader sequence** (mirrors server.go:175-260, trimmed to what NAI-128 needs):

```go
cacheDir := filepath.Join("..", "..", "data", "pack")
locTypes, err := objtype.LoadLocTypes(cacheDir)        // gamemap dependency
gm := gamemap.New(discardLogger())
gm.SetLocTypes(locTypes)
gm.Init(cacheDir)                                       // loads m**_** map files
params, err := objtype.LoadParams(cacheDir)             // for ObjTypes + NpcType params
objTypes, err := objtype.LoadObjTypes(cacheDir, params) // raw_rat_meat lookup
npcTypes, err := objtype.LoadNPCTypes(cacheDir)         // newbiegiantrat type
varpTypes, err := objtype.LoadVarpTypes(cacheDir)       // varp resolution
provider := script.NewProvider()
provider.Load(filepath.Join(cacheDir, "server"))        // script.dat / script.idx
```

Skip-if-absent guard at the top of the test (mirroring nai101_fountain_test.go:30) keeps the test CI-portable when the cache is absent.

**Where `newbiegiantrat` lives:** NPC type ID is looked up via `npcTypes.ConfigNames["newbiegiantrat"]`. ObjType ID for `raw_rat_meat` via `objTypes.ConfigNames["raw_rat_meat"]`. Param ID for `death_drop` via `params.ConfigNames["death_drop"]` (verify shape at T1; if `params.ConfigNames` doesn't exist, use `params.Configs[i]` walk).

**Tutorial-state risk:** `[ai_queue3,newbiegiantrat]` line 9-12 has a `if (%tutorial = ...)` branch that fires `queue(set_rat_kill, 0, 0)`. Probe should NOT depend on this — keep `%tutorial = 0` and assert that the loot drops REGARDLESS (per spec §8 R2 dual-pin requirement).

---

### Task 1: Test file scaffolding + cache loader fixture

**Files:**
- Create: `modules/world/nai128_rat_loot_test.go`

- [ ] **Step 1: Create the file with cache-loader fixture and a passing-by-default sanity subtest**

```go
package world

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// nai128CacheFixture loads the real data/pack cache + scriptProvider into a
// test Server. Mirrors the loader sequence in modules/world/server.go:175-260
// trimmed to what NAI-128 Stage 1 needs (locTypes, params, objTypes, npcTypes,
// varpTypes, scriptProvider, gamemap). Returns (server, skipReason); if
// skipReason != "" the test should t.Skipf to keep CI-portable.
func nai128CacheFixture(t *testing.T) (*Server, string) {
	t.Helper()
	cacheDir := filepath.Join("..", "..", "data", "pack")
	for _, p := range []string{
		filepath.Join(cacheDir, "server", "script.dat"),
		filepath.Join(cacheDir, "server", "npc.dat"),
		filepath.Join(cacheDir, "server", "obj.dat"),
	} {
		if _, err := os.Stat(p); err != nil {
			return nil, p + " unavailable: " + err.Error()
		}
	}

	s := newTestServer(t)

	// Locs + gamemap (death-side of cascade routes obj_add through zoneMap;
	// gamemap is required for s.zoneMap.Get to anchor on the rat coord).
	locTypes, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}
	s.locTypes = locTypes
	gm := gamemap.New(discardLogger())
	gm.SetLocTypes(locTypes)
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}
	s.gamemap = gm

	// ObjTypes + params for raw_rat_meat resolution and npc_param(death_drop).
	params, err := objtype.LoadParams(cacheDir)
	if err != nil {
		t.Fatalf("LoadParams: %v", err)
	}
	objTypes, err := objtype.LoadObjTypes(cacheDir, params)
	if err != nil {
		t.Fatalf("LoadObjTypes: %v", err)
	}
	s.paramTypes = params
	s.objTypes = objTypes

	// NpcTypes — defines newbiegiantrat with its death_drop param.
	npcTypes, err := objtype.LoadNPCTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadNPCTypes: %v", err)
	}
	s.npcTypes = npcTypes

	// VarpTypes — for any varp reads inside the cascade.
	varpTypes, err := objtype.LoadVarpTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadVarpTypes: %v", err)
	}
	s.varpTypes = varpTypes

	// Real script provider — replaces the stub from newTestServer.
	provider := script.NewProvider()
	if err := provider.Load(filepath.Join(cacheDir, "server")); err != nil {
		t.Fatalf("provider.Load: %v", err)
	}
	s.scriptProvider = provider

	return s, ""
}

// TestNAI128_RatLootCascade is the Stage-1 binding probe for NAI-128.
// See docs/superpowers/specs/2026-05-08-nai-128-rat-loot-cascade-investigation-design.md.
func TestNAI128_RatLootCascade(t *testing.T) {
	s, skip := nai128CacheFixture(t)
	if skip != "" {
		t.Skipf("cache unavailable: %s", skip)
	}
	t.Run("FixtureLoaded", func(t *testing.T) {
		if s.scriptProvider == nil {
			t.Fatal("scriptProvider nil after fixture load")
		}
		if s.npcTypes == nil || len(s.npcTypes.Configs) == 0 {
			t.Fatal("npcTypes empty after fixture load")
		}
		if s.objTypes == nil || len(s.objTypes.Configs) == 0 {
			t.Fatal("objTypes empty after fixture load")
		}
	})
}
```

- [ ] **Step 2: Run to verify the file compiles and FixtureLoaded passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestNAI128_RatLootCascade -v`

Expected:
- If `data/pack/server/script.dat` etc. exist: `--- PASS: TestNAI128_RatLootCascade/FixtureLoaded`
- If absent: `--- SKIP: TestNAI128_RatLootCascade` with the skip reason

If the test fails to compile, the most likely issue is a field-name mismatch (e.g. `s.varpTypes` doesn't exist on the test Server). Read `modules/world/server.go` field declarations and adjust. DO NOT add new fields to Server — only assign existing ones.

- [ ] **Step 3: Commit**

```bash
git add modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "test(nai-128): T1 — Stage-1 probe scaffolding + cache fixture

Adds nai128CacheFixture helper loading the real data/pack/server
artifacts (script.dat, npc.dat, obj.dat, params, varpTypes, gamemap)
into a test Server. Mirrors modules/world/server.go:175-260 trimmed
to NAI-128 Stage-1 cascade-probe needs. Skip-if-absent guard keeps
CI portable.

FixtureLoaded subtest pins that the cache loaded non-empty."
```

---

### Task 2: Spawn player + giant_rat NPC; pin baseline state

**Files:**
- Modify: `modules/world/nai128_rat_loot_test.go` (add subtest after FixtureLoaded)

- [ ] **Step 1: Add the NpcSpawn + PlayerSpawn subtests**

After the `FixtureLoaded` t.Run block (still inside `TestNAI128_RatLootCascade`), append:

```go
	// Resolve newbiegiantrat type and spawn a rat at a benign coord.
	ratTypeID, ok := s.npcTypes.ConfigNames["newbiegiantrat"]
	if !ok {
		t.Fatalf("npc type 'newbiegiantrat' not in ConfigNames; check NPC_FINDHERO predecessor work")
	}
	ratType := s.npcTypes.Configs[ratTypeID]
	rat := NewNpc(1, ratTypeID, 3094, 3106, 0, ratType)
	rat.server = s

	// Spawn a player adjacent to the rat.
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.x, p.z, p.level = 3094, 3107, 0

	t.Run("BaselineState", func(t *testing.T) {
		if rat.uid == 0 {
			t.Errorf("rat.uid = 0; want non-zero (typeId<<16 | nid)")
		}
		if p.UID() == 0 {
			t.Errorf("player.UID() = 0; want non-zero")
		}
		hp := rat.levels[objtype.NpcStatHitpoints]
		if hp <= 0 {
			t.Errorf("rat HP = %d; want > 0 (seeded from typ.Stats[NpcStatHitpoints])", hp)
		}
		t.Logf("rat: uid=%d typeId=%d HP=%d coord=(%d,%d,%d)",
			rat.uid, rat.typeId, hp, rat.x, rat.z, rat.level)
		t.Logf("player: uid=%d coord=(%d,%d,%d)", p.UID(), p.x, p.z, p.level)
	})
```

- [ ] **Step 2: Run BaselineState; expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestNAI128_RatLootCascade/BaselineState -v`

Expected:
- `--- PASS: TestNAI128_RatLootCascade/BaselineState` with the t.Logf lines visible (-v).
- Rat HP must be > 0. If HP=0, NpcType.Stats seeding is broken — that itself is a finding; record it in the binding handoff doc.

If `npcTypes.ConfigNames["newbiegiantrat"]` lookup fails, the cache may use a different DebugName. Run a one-off probe: `for name := range s.npcTypes.ConfigNames { if strings.Contains(name, "rat") { t.Log(name) } }` to find the right key.

- [ ] **Step 3: Commit**

```bash
git add modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "test(nai-128): T2 — spawn rat + player; pin baseline state

newbiegiantrat NPC at (3094, 3106, 0) + player at (3094, 3107, 0).
BaselineState subtest verifies non-zero UIDs and HP seeding from
typ.Stats[NpcStatHitpoints] (NAI-17 baseline)."
```

---

### Task 3: Pre-populate rat.heroPoints + rat.queue (simulate post-melee-attack state)

**Files:**
- Modify: `modules/world/nai128_rat_loot_test.go` (add subtest after BaselineState)

- [ ] **Step 1: Add the Preconditions subtest**

After the `BaselineState` t.Run block, append:

```go
	// Simulate the state player_melee_attack would leave: rat ledger
	// credited with the player's UID, and ai_queue2 enqueued with the
	// damage value. Per spec §4 plan-vs-spec divergence note, this
	// bypasses the RNG-bound hit-roll branch.
	const damage = 5
	rat.heroPoints.AddHero(p.UID(), damage)
	rat.EnqueueScriptForTrigger(script.TriggerAiQueue2, 0, damage)

	// Force rat HP to 1 so the next damage application kills (one-shot
	// simplifies cascade tracing).
	rat.levels[objtype.NpcStatHitpoints] = 1

	t.Run("Preconditions", func(t *testing.T) {
		top := rat.heroPoints.TopContributor()
		if top != p.UID() {
			t.Errorf("rat.heroPoints.TopContributor() = %d; want %d (player.UID)", top, p.UID())
		}
		if len(rat.queue) != 1 {
			t.Fatalf("rat.queue len = %d; want 1 (ai_queue2 enqueued)", len(rat.queue))
		}
		req := rat.queue[0]
		if req.Trigger != script.TriggerAiQueue2 {
			t.Errorf("rat.queue[0].Trigger = %v; want TriggerAiQueue2 (%d)", req.Trigger, script.TriggerAiQueue2)
		}
		if req.LastInt != damage {
			t.Errorf("rat.queue[0].LastInt = %d; want %d", req.LastInt, damage)
		}
		if req.Delay != 0 {
			t.Errorf("rat.queue[0].Delay = %d; want 0", req.Delay)
		}
		if hp := rat.levels[objtype.NpcStatHitpoints]; hp != 1 {
			t.Errorf("rat HP after force-set = %d; want 1", hp)
		}
	})
```

- [ ] **Step 2: Run Preconditions; expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestNAI128_RatLootCascade/Preconditions -v`

Expected: `--- PASS: TestNAI128_RatLootCascade/Preconditions`.

If `script.TriggerAiQueue2` is not exported as that exact name, grep `pkg/script/trigger.go` for the actual constant and adjust.

- [ ] **Step 3: Commit**

```bash
git add modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "test(nai-128): T3 — pre-populate rat ledger + ai_queue2

Per plan-vs-spec divergence: bypass RNG-bound hit-roll by directly
setting rat.heroPoints.AddHero(p.UID, 5) + EnqueueScriptForTrigger(
TriggerAiQueue2, 0, 5). Force rat HP=1 for one-shot kill.

Preconditions subtest verifies the simulated post-melee-attack
state holds before driving processNpcQueue."
```

---

### Task 4: Drive `processNpcQueue` once; assert HP=0 + (cascade collapsed) ai_queue3 fired

**Files:**
- Modify: `modules/world/nai128_rat_loot_test.go` (add subtest after Preconditions)

- [ ] **Step 1: Add the AiQueueCascade subtest**

After the `Preconditions` t.Run block, append:

```go
	t.Run("AiQueueCascade", func(t *testing.T) {
		// One processNpcQueue call drains all entries with Delay<=0.
		// Per spec §4.4 phase-collapse pre-flight: ai_queue2 firing
		// runs npc_default_damage which calls NPC_QUEUE(3,0,0); the
		// re-entered enqueue lands at end of n.queue with Delay=0 and
		// fires within the SAME loop iteration via the for-len-grows
		// pattern (npc_script.go:497-526). After this call, BOTH
		// ai_queue2 AND ai_queue3 should have run.
		s.processNpcQueue(rat)

		// Cascade link 1: NPC_DAMAGE (called inside ~npc_default_damage)
		// must have decremented HP to 0.
		if hp := rat.levels[objtype.NpcStatHitpoints]; hp != 0 {
			t.Errorf("rat HP after one processNpcQueue = %d; want 0 (binding candidate B/C: NPC_DAMAGE handler bug or [ai_queue2,_] not dispatching)", hp)
		}

		// Cascade link 2: queue must be drained. If ai_queue3 enqueued
		// but didn't fire (phase-collapse hypothesis wrong), the queue
		// will still contain the ai_queue3 entry. If ai_queue2 didn't
		// dispatch at all, we'd have hit the HP assertion above.
		if remaining := len(rat.queue); remaining != 0 {
			t.Errorf("rat.queue len after cascade = %d; want 0 (binding candidate D-or-tick-order: ai_queue3 enqueued but not fired in same call)", remaining)
			for i, req := range rat.queue {
				t.Logf("  rat.queue[%d]: Trigger=%v Delay=%d LastInt=%d", i, req.Trigger, req.Delay, req.LastInt)
			}
		}
	})
```

- [ ] **Step 2: Run AiQueueCascade**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestNAI128_RatLootCascade/AiQueueCascade -v`

**Three possible outcomes — DO NOT proceed to T5 if outcome ≠ PASS:**

1. **PASS** → ai_queue2 dispatched, NPC_DAMAGE worked, ai_queue3 fired. Cascade through this layer is functional. Proceed to T5.
2. **FAIL on rat HP assertion** → binding candidate **B** (NPC_DAMAGE handler bug) or **C** (`[ai_queue2,_]` not dispatching). Stop; capture output; proceed to T6.
3. **FAIL on queue-drained assertion** → binding candidate **D-or-tick-order** (ai_queue3 enqueued but not fired in same call OR `[ai_queue3,newbiegiantrat]` specific lookup fails). Stop; capture output; proceed to T6.

If the test panics (nil pointer, unhandled-opcode runtime error from script.Execute) instead of cleanly failing — that's also a Stage-1 binding (handler dispatch fails before NPC_DAMAGE / NPC_QUEUE complete). Capture the panic stack; proceed to T6.

- [ ] **Step 3: Commit (regardless of PASS/FAIL — Stage-1 evidence belongs in the repo)**

If PASS:
```bash
git add modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "test(nai-128): T4 — ai_queue2/3 cascade probe (PASS at HEAD)

processNpcQueue drains ai_queue2 (npc_default_damage → npc_damage
→ HP 1→0 → npc_queue(3,0,0)) and the phase-collapsed ai_queue3
in one call. Both assertions pass at HEAD; cascade through the
damage+enqueue layer is functional. Proceeds to T5 ground-obj
inspection."
```

If FAIL:
```bash
git add modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "test(nai-128): T4 — ai_queue2/3 cascade probe (FAIL at HEAD)

Stage-1 binding: <candidate B/C/D-or-tick-order per FAIL output>.
<one-line summary of which assertion failed and observed-vs-want>.

Stops Stage 1 here; T6 produces the binding handoff doc; Stage 2
plan authored separately against the binding."
```

---

### Task 5: Assert two ground objs at rat coord (death_drop + raw_rat_meat)

**Files:**
- Modify: `modules/world/nai128_rat_loot_test.go` (add subtest after AiQueueCascade)

**Skip this task if T4 FAILED.**

- [ ] **Step 1: Add the GroundObjs subtest**

After the `AiQueueCascade` t.Run block, append:

```go
	t.Run("GroundObjs", func(t *testing.T) {
		// Look up expected obj IDs.
		ratMeatID, ok := s.objTypes.ConfigNames["raw_rat_meat"]
		if !ok {
			t.Fatalf("obj type 'raw_rat_meat' not in ConfigNames; binding candidate: ObjType cache gap")
		}

		// Resolve the death_drop param ID, then read it off the rat type.
		// NpcType.Params lookup shape: implementer verifies via a one-off
		// printf if the field name differs.
		dropParamID, ok := s.paramTypes.ConfigNames["death_drop"]
		if !ok {
			t.Fatalf("param type 'death_drop' not in ConfigNames; binding candidate: ParamType cache gap")
		}
		dropObjID := ratType.ParamInt(int32(dropParamID), -1)
		if dropObjID < 0 {
			t.Fatalf("ratType.ParamInt(death_drop) = %d; want a valid obj ID (binding candidate D: npc_param(death_drop) returns null/-1)", dropObjID)
		}

		// Read the zone at the rat's coord.
		z := s.zoneMap.Get(rat.level, rat.x, rat.z)
		if z == nil {
			t.Fatal("zoneMap.Get returned nil for rat coord")
		}

		// Assert exactly the two obj_adds from [ai_queue3,newbiegiantrat]:
		// obj_add(npc_coord, npc_param(death_drop), 1, ^lootdrop_duration)
		// obj_add(npc_coord, raw_rat_meat,          1, ^lootdrop_duration)
		// Filter by rat coord (the zone may contain other test obj state).
		var atRat []int
		for _, o := range z.Objs {
			if o.X == rat.x && o.Z == rat.z && o.Level == rat.level {
				atRat = append(atRat, o.Type)
			}
		}
		if len(atRat) != 2 {
			t.Errorf("ground obj count at rat coord = %d; want 2 (binding candidate E: OBJ_ADD not registering OR npc_findhero=false skipping the if-block)", len(atRat))
			t.Logf("  observed types at rat coord: %v", atRat)
			t.Logf("  zone.Objs full: %d entries", len(z.Objs))
			for i, o := range z.Objs {
				t.Logf("    [%d] type=%d count=%d at (%d,%d,%d) lifecycle=%v",
					i, o.Type, o.Count, o.X, o.Z, o.Level, o.Lifecycle)
			}
			return
		}

		// Specific-match dispatch verification (spec §6 R3 mitigation):
		// raw_rat_meat is in the [ai_queue3,newbiegiantrat] specific match
		// but NOT in [ai_queue3,_] / [proc,npc_default_death]. Its presence
		// pins specific-trigger dispatch.
		hasMeat := false
		hasDrop := false
		for _, typ := range atRat {
			if typ == ratMeatID {
				hasMeat = true
			}
			if typ == dropObjID {
				hasDrop = true
			}
		}
		if !hasMeat {
			t.Errorf("raw_rat_meat (id=%d) not among ground objs at rat coord; binding candidate: [ai_queue3,newbiegiantrat] specific-match did not dispatch (fell through to [ai_queue3,_] generic)", ratMeatID)
		}
		if !hasDrop {
			t.Errorf("death_drop (id=%d) not among ground objs at rat coord; binding candidate D: npc_param(death_drop) returned a value but obj_add for it did not register", dropObjID)
		}
	})
```

- [ ] **Step 2: Verify NpcType.ParamInt accessor shape**

Before running, grep to confirm the accessor name. The plan codifies `ratType.ParamInt(int32(dropParamID), -1)` based on convention; the actual signature may be `Param(id) int` or `Params[id]` map. Adjust the call site if needed.

```bash
grep -n -e "ParamInt\|func.*NpcType.*Param" pkg/objtype/npctype.go
```

- [ ] **Step 3: Run GroundObjs**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestNAI128_RatLootCascade/GroundObjs -v`

**Possible outcomes:**

1. **PASS** → cascade fully functional at HEAD. Spec §4.4 R1 risk lands: probe is too synthetic to surface the real bug. Escalate to Stage 1.5 instrumentation-smoke per spec §8 R1.
2. **FAIL on `raw_rat_meat` not in ConfigNames** → ObjType cache gap (binding candidate E1).
3. **FAIL on `death_drop` not in ConfigNames or returning -1** → param/config gap (binding candidate D).
4. **FAIL on `len(atRat) != 2`** with 0 entries → npc_findhero returned false despite ledger pre-populated, OR the if-block didn't execute (binding candidate D-related: NPC_FINDHERO read path broken even though credit-side was fine).
5. **FAIL on `len(atRat) != 2`** with 1 entry → only one obj_add fired. If `raw_rat_meat` missing → specific-match dispatch failed (fell through to `[ai_queue3,_]`'s `[proc,npc_default_death]` which does ONE obj_add). Binding candidate "specific-match dispatch".
6. **FAIL on `len(atRat) != 2`** with 2 entries but missing one ID → the obj_add for that specific objtype is broken.

- [ ] **Step 4: Commit (PASS or FAIL)**

If PASS:
```bash
git add modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "test(nai-128): T5 — ground-obj inspection (PASS at HEAD)

Two ground objs at rat coord post-cascade: death_drop + raw_rat_meat.
Specific-match dispatch ([ai_queue3,newbiegiantrat]) confirmed via
raw_rat_meat presence (per spec §6 R3 dual-pin mitigation).

Stage-1 PROBE PASSES at HEAD — synthetic cascade is functional.
Spec §4.4 R1 lands: real-engine smoke shows loot-drop failure but
synthetic probe doesn't bind. Escalate to Stage 1.5 instrumentation
per spec §8 R1; T6 records this routing decision."
```

If FAIL:
```bash
git add modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "test(nai-128): T5 — ground-obj probe (FAIL at HEAD)

Stage-1 binding: <candidate D / E / specific-match per FAIL output>.
<one-line summary>.

T6 produces handoff doc + Stage 2 plan dispatched against binding."
```

---

### Task 6: Stage-1 binding handoff doc + close commit

**Files:**
- Create: `docs/superpowers/handoffs/2026-05-08-nai-128-stage-1-binding.md`

- [ ] **Step 1: Write the handoff doc capturing whichever Stage-1 outcome surfaced**

Replace the bracketed placeholders with concrete data from T4/T5 runs.

```markdown
# NAI-128 Stage 1 — binding handoff

**Date:** 2026-05-XX
**Stage 1 plan:** docs/superpowers/plans/2026-05-08-nai-128-rat-loot-cascade-investigation.md
**Spec:** docs/superpowers/specs/2026-05-08-nai-128-rat-loot-cascade-investigation-design.md
**Probe:** modules/world/nai128_rat_loot_test.go::TestNAI128_RatLootCascade

## Outcome

[CHOOSE ONE — fill in concrete details:]

**(A) BINDING FOUND.** Failing assertion: `<exact assertion text>`. Test output excerpt:

```
<paste 5-15 lines of go test -v output, including the FAIL line>
```

Bound candidate per spec §4.2: **<A / B / C / D / E / specific-match / D-or-tick-order>**.

Hypothesised root cause: `<one-paragraph diagnosis based on the assertion>`.

Stage 2 fix shape: `<sketch — file paths, function name, line range, expected diff size>`.

**(B) NO BINDING — probe fully PASSES at HEAD.**
Per spec §8 R1, this means the synthetic probe is too narrow to surface the real engine bug. Routing: escalate to Stage 1.5 instrumentation-smoke per spec §8 R1. Stage 1.5 plan authors slog.Info instrumentation at each cascade link in the real handler / dispatch sites, requests Java-client smoke, binds on first-blank-instrumentation-line.

## Resume prompt for next session

```
NAI-128 Stage 2 plan-write. Stage 1 probe at
modules/world/nai128_rat_loot_test.go ran on <date>. Outcome: <A or B>.
[If A:] Bound candidate <X>; expected fix at <file:line> ~<N> LOC.
Author Stage-2 plan via superpowers:writing-plans against this binding;
TDD against the existing probe (turn the failed assertion green).
[If B:] Probe passes; spec §8 R1 escalation to Stage 1.5
instrumentation-smoke required.

HEAD: <SHA after T5 commit>.
```
```

- [ ] **Step 2: Commit the handoff doc**

```bash
git add docs/superpowers/handoffs/2026-05-08-nai-128-stage-1-binding.md
git commit --no-gpg-sign -m "docs(nai-128): Stage-1 binding handoff

<one-line summary of outcome A or B>"
```

- [ ] **Step 3: Verify final state**

Run the full test once more for the close commit's evidence:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world -run TestNAI128_RatLootCascade -v 2>&1 | tail -40
```

Capture the final tail in your handoff to the user.

- [ ] **Step 4: Run full repo test + vet to verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1 | tail -20
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... 2>&1 | tail -20
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -40
```

Expected: build/vet clean; only the NAI-128 test status changes (others unaffected). If any unrelated test newly fails, that is a regression — investigate before declaring Stage 1 done.

---

## Plan self-review

1. **Spec coverage:**
   - Spec §4.1 cache fixture → T1 ✅
   - Spec §4.1 Player+Npc spawn → T2 ✅
   - Spec §4.2 Phase A NPC_HEROPOINTS credit → bypassed per plan-vs-spec divergence note; pre-populated in T3; existing `heropoints_test.go` covers handler unit-test correctness ✅
   - Spec §4.2 Phase A `npc_queue(2,$dmg,0)` → simulated in T3 via `EnqueueScriptForTrigger` ✅
   - Spec §4.2 Phase B HP=0 + ai_queue3 enqueued → T4 ✅
   - Spec §4.2 Phase C ground-obj inspection → T5 ✅
   - Spec §4.2 specific-match dispatch verification (R3 mitigation via raw_rat_meat presence) → T5 ✅
   - Spec §4.4 phase-collapse pre-flight → embedded in T4 design (one processNpcQueue call drains ai_queue2 + ai_queue3) ✅
   - Spec §6 conditional smoke → T6 routing decision per outcome ✅
   - Spec §8 R2 tutorial-state independence → addressed by leaving %tutorial=0 (default; not set in T3) ✅
   - Spec §10 Stage 1 binding handoff → T6 ✅

2. **Placeholder scan:** No "TBD" / "TODO" / "implement later" / vague handling. T5 Step 2 instructs plan-author to verify accessor name via grep — that's a real instruction with the exact grep command, not a placeholder.

3. **Type consistency:**
   - `script.TriggerAiQueue2` referenced consistently in T3 + T4. Verified at brainstorm: `pkg/script/trigger.go:118`.
   - `objtype.NpcStatHitpoints` referenced in T2 + T3 + T4. Verified: `pkg/objtype/npctype.go` defines `NpcStat*` constants.
   - `script.NpcQueueRequest` fields `Trigger`, `Delay`, `LastInt` — verified at brainstorm via npc.go:336 (`script.NpcQueueRequest{Trigger:..., Delay:..., LastInt:...}`).
   - `s.zoneMap.Get(level, x, z)` returning `*zone.Zone` with `.Objs []*entity.Obj` — verified `pkg/zone/zone.go:253-270`.
   - `NpcType.ParamInt` — convention-based; T5 Step 2 explicitly grep-verifies before running.

---

## Execution Handoff

**Plan complete and saved to** `docs/superpowers/plans/2026-05-08-nai-128-rat-loot-cascade-investigation.md`.

Per `superpowers_clear_between_spec_and_impl` memory: this is the boundary. The user should `/clear` before implementing. The next session re-enters Stage 1 with the resume prompt below.

**Resume prompt for the next session (paste verbatim after /clear):**

```
NAI-128 Stage 1 implementation. HEAD is 6f59550 (NAI-128 spec on main).
Plan: docs/superpowers/plans/2026-05-08-nai-128-rat-loot-cascade-investigation.md.
Spec: docs/superpowers/specs/2026-05-08-nai-128-rat-loot-cascade-investigation-design.md.

Dispatch via subagent-driven-development per execution_mode_default.
Use controller_preflight before each task. Apply
plan_runnable_test_fixtures + scriptstate_test_fixture_idioms to
test fixture authoring. Apply verify_implementer_claims at each
task close. The plan's T4 + T5 each have multiple PASS/FAIL outcomes
that route differently — controller adjudicates routing per the
outcome trees in the plan, then dispatches T6 (handoff doc) with
the bound candidate filled in. Stage 2 plan is authored AFTER
Stage 1's T6 commits the handoff; do not bundle.

Memory entries to grep at start: cascade_theory_smoke_binding,
disasm_reframes_inferred_binding, controller_preflight,
verify_implementer_claims, plan_runnable_test_fixtures,
scriptstate_test_fixture_idioms, post_task_handoff,
session_context_management.
```
