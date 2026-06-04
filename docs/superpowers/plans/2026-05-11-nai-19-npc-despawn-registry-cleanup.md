# NAI-19 — NPC DESPAWN-lifecycle Registry Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `World.removeNpc` DESPAWN-lifecycle parity (`World.ts:1312-1315`): release the registry slot, run `n.Cleanup()` field-nuking (TS `Npc.cleanup()` at `Npc.ts:187-193`), and prune the despawned NPC from `s.npcLoop` via an end-of-tick `compactNpcLoop` pass.

**Architecture:** Three production touch-points in `modules/world/`:
1. `npc.go` — new exported `(n *Npc) Cleanup()` method.
2. `npc_registry.go` — `(*Server).removeNpc` DESPAWN-arm body (currently an empty TODO).
3. `tick.go` — new `(*Server).compactNpcLoop` helper + wire into `processCleanup`.

Mid-tick iteration safety solved by end-of-tick mark/compact (deferred from TS's immediate per-zone splice). RESPAWN-lifecycle NPCs are preserved in `s.npcLoop` (their `dead=true` flips on next `lifecycleTick==0`). Tracked deviation: `NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE`.

**Tech stack:** Go 1.26+; project root `/home/owner/Code/github.com/zsrv/goscape`. All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. All commits use `git commit --no-gpg-sign`.

**Pre-flight verified at plan-write (commit `5170ba4`):**
- `*Npc` fields: `nid int` (npc.go:28), `uid int` (npc.go:33), `lifecycle int` (npc.go:46), `dead bool` (npc.go:49), `activeScript *script.ScriptState` (npc.go:82), `queue []script.NpcQueueRequest` (npc.go:85), `huntTarget entity` (npc.go:93).
- `*Server` fields: `npcs [8192]*Npc` (server.go:93), `npcLoop []*Npc` (server.go:94), `nextNpcSlot int` (server.go:96).
- `allocNpcSlot` scans `s.npcs` for nil starting at `s.nextNpcSlot`, returns 1..8191 (skips 0); reuses freed slots when found (npc_registry.go:14-32).
- `(*Server).removeNpc` already runs `s.rsbuf.RemoveNpc`, `zone.LeaveNpc`, sets `n.dead=true`, and toggles collision (npc_registry.go:186-212). Only the DESPAWN arm body is missing.
- `processCleanup` is the last `processTick` step before bookkeeping (tick.go:67, body at tick.go:631-670); already does `rsbuf.Cleanup()` after `npcLoop` ResetMasks pass.
- `s.npcs[slot]` consumers all nil-gate: `handler_opnpc.go:59-63`, `npc_script_lookup.go:133-136`, `server_varp.go:249-252`. **No nil-gate fixes needed.**
- Existing `NpcLifecycleDespawn`-using tests do NOT assert `s.npcs[]` / `s.npcLoop` retention: `npc_registry_test.go:72-91`, `npc_event_queue_test.go:155-173`, `npc_test.go:146-157`. **No inversions needed.**
- `newTestServer(t)` at `server_test.go:311` returns a `*Server` with `npcs` zero-initialized (all nil entries) and `npcLoop` nil slice — suitable for direct manipulation.

---

### Task 1: Add `(n *Npc) Cleanup()` method (TDD)

**Files:**
- Modify: `modules/world/npc.go` (append after `ClearActiveScript` at line 302-309)
- Test: `modules/world/npc_test.go` (append at end)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_test.go`:

```go
// TestNpcCleanup pins the (n *Npc) Cleanup() field-zeroing contract.
// Mirrors TS Npc.cleanup at Engine-TS/src/engine/entity/Npc.ts:187-193:
// nid=-1, uid=-1, activeScript=nil, huntTarget=nil, queue cleared.
//
// NAI-19: Cleanup is called from (*Server).removeNpc's DESPAWN-lifecycle
// arm after the registry slot has been nilled. Defensive nullification —
// any caller still holding the *Npc pointer post-DESPAWN reads -1
// sentinels rather than valid-looking state.
func TestNpcCleanup(t *testing.T) {
	n := &Npc{
		nid:           7,
		uid:           (42 << 16) | 7,
		activeScript:  &script.ScriptState{},
		huntTarget:    &Npc{nid: 99},
		queue:         []script.NpcQueueRequest{{}, {}},
	}

	n.Cleanup()

	if n.nid != -1 {
		t.Errorf("nid: got %d, want -1", n.nid)
	}
	if n.uid != -1 {
		t.Errorf("uid: got %d, want -1", n.uid)
	}
	if n.activeScript != nil {
		t.Errorf("activeScript: got %p, want nil", n.activeScript)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil", n.huntTarget)
	}
	if n.queue != nil {
		t.Errorf("queue: got %v, want nil", n.queue)
	}
}
```

Verify imports include `"github.com/zsrv/goscape/pkg/script"` — pre-flight: `grep -n '"github.com/zsrv/goscape/pkg/script"' modules/world/npc_test.go`. If missing, add it.

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcCleanup -count=1
```
Expected: compilation FAIL with `n.Cleanup undefined`.

- [ ] **Step 3: Write minimal implementation**

In `modules/world/npc.go`, after `ClearActiveScript` (around line 309), append:

```go
// Cleanup mirrors TS Npc.cleanup at Engine-TS/src/engine/entity/Npc.ts:187-193.
// Zeros identity / script / hunt / queue fields after (*Server).removeNpc
// has released the registry slot on DESPAWN-lifecycle. Defensive
// nullification: any consumer still holding the *Npc pointer post-DESPAWN
// reads -1 sentinels rather than valid-looking state. NAI-19.
func (n *Npc) Cleanup() {
	n.nid = -1
	n.uid = -1
	n.activeScript = nil
	n.huntTarget = nil
	n.queue = nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcCleanup -count=1
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc.go modules/world/npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 T1 — (*Npc).Cleanup method (TS Npc.cleanup parity)

Adds (n *Npc) Cleanup() mirroring TS Npc.cleanup at Engine-TS/src/engine/
entity/Npc.ts:187-193. Zeros nid, uid, activeScript, huntTarget, queue.
Wired into (*Server).removeNpc DESPAWN-arm in T2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Wire registry slot release + Cleanup in `removeNpc` DESPAWN arm (TDD)

**Files:**
- Modify: `modules/world/npc_registry.go:206-209` (the empty `if n.lifecycle == NpcLifecycleDespawn { TODO }` block)
- Test: `modules/world/npc_registry_test.go` (append at end)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_registry_test.go`. First verify imports include `"github.com/zsrv/goscape/pkg/script"` and `"github.com/zsrv/goscape/pkg/objtype"` (they should — most tests use them). If `slot := n.nid` capture pattern is unclear, the first test demonstrates it.

```go
// TestRemoveNpc_DespawnLifecycle_ClearsRegistrySlot pins the NAI-19
// slot-release: after removeNpc on a DESPAWN-lifecycle NPC, the
// allocated nid slot in s.npcs is nilled so allocNpcSlot can reuse it.
// Mirrors TS World.ts:1314: this.npcs.remove(npc.nid).
func TestRemoveNpc_DespawnLifecycle_ClearsRegistrySlot(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.server = s
	s.npcs[1] = n
	s.npcLoop = append(s.npcLoop, n)
	n.lifecycle = NpcLifecycleDespawn

	slot := n.nid // capture before Cleanup zeros it

	s.removeNpc(n, -1)

	if s.npcs[slot] != nil {
		t.Errorf("s.npcs[%d]: got %p, want nil (slot must be released on DESPAWN)", slot, s.npcs[slot])
	}
}

// TestRemoveNpc_DespawnLifecycle_RunsCleanup pins that the DESPAWN
// arm of removeNpc calls n.Cleanup, zeroing identity / script / hunt /
// queue. Mirrors TS World.ts:1315: npc.cleanup().
func TestRemoveNpc_DespawnLifecycle_RunsCleanup(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.server = s
	s.npcs[1] = n
	s.npcLoop = append(s.npcLoop, n)
	n.lifecycle = NpcLifecycleDespawn
	n.activeScript = &script.ScriptState{}
	n.queue = []script.NpcQueueRequest{{}}

	s.removeNpc(n, -1)

	if n.nid != -1 {
		t.Errorf("nid: got %d, want -1 (Cleanup must zero)", n.nid)
	}
	if n.uid != -1 {
		t.Errorf("uid: got %d, want -1", n.uid)
	}
	if n.activeScript != nil {
		t.Errorf("activeScript: got %p, want nil", n.activeScript)
	}
	if n.queue != nil {
		t.Errorf("queue: got %v, want nil", n.queue)
	}
}

// TestRemoveNpc_DespawnLifecycle_SlotReusable pins that the allocator
// can reuse a slot released by removeNpc. After DESPAWN + compactNpcLoop,
// allocNpcSlot returns the freed nid. Compact is needed because the
// allocator only scans s.npcs (not s.npcLoop), but we run compact here
// to match the production end-of-tick ordering.
func TestRemoveNpc_DespawnLifecycle_SlotReusable(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n1 := NewNpc(0, 7, 100, 100, 0, typ)
	n1.nid = 1
	n1.server = s
	s.npcs[1] = n1
	s.npcLoop = append(s.npcLoop, n1)
	n1.lifecycle = NpcLifecycleDespawn

	s.nextNpcSlot = 1 // force allocator to start at slot 1
	s.removeNpc(n1, -1)
	s.compactNpcLoop()

	reused := s.allocNpcSlot()
	if reused != 1 {
		t.Errorf("allocNpcSlot: got %d, want 1 (freed slot must be reusable)", reused)
	}
}

// TestRemoveNpc_RespawnLifecycle_PreservesRegistry pins that removeNpc
// on a RESPAWN-lifecycle NPC does NOT release the slot or run Cleanup —
// the NPC will respawn in place at lifecycleTick==0 (see npc_ai.go:31-45).
func TestRemoveNpc_RespawnLifecycle_PreservesRegistry(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.uid = (7 << 16) | 1
	n.server = s
	s.npcs[1] = n
	s.npcLoop = append(s.npcLoop, n)
	n.lifecycle = NpcLifecycleRespawn
	n.activeScript = &script.ScriptState{}

	s.removeNpc(n, 50)

	if s.npcs[1] != n {
		t.Errorf("s.npcs[1]: got %p, want %p (RESPAWN must NOT release slot)", s.npcs[1], n)
	}
	if n.nid != 1 {
		t.Errorf("nid: got %d, want 1 (RESPAWN must NOT run Cleanup)", n.nid)
	}
	if n.activeScript == nil {
		t.Error("activeScript: got nil, want preserved (RESPAWN must NOT run Cleanup)")
	}
}
```

Note: `TestRemoveNpc_DespawnLifecycle_SlotReusable` calls `s.compactNpcLoop()` which doesn't exist yet at T2 — that test will compile-FAIL until T3 lands. **Defer it from this commit:** move the SlotReusable test to T3's test block. The other three tests are runnable at T2.

**Revised T2 test block:** include only the three tests `ClearsRegistrySlot`, `RunsCleanup`, `RespawnLifecycle_PreservesRegistry`.

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestRemoveNpc_DespawnLifecycle_ClearsRegistrySlot|TestRemoveNpc_DespawnLifecycle_RunsCleanup|TestRemoveNpc_RespawnLifecycle_PreservesRegistry' -count=1
```
Expected: 2 of 3 FAIL (`ClearsRegistrySlot`: `s.npcs[1] != nil`; `RunsCleanup`: `nid != -1`). `RespawnLifecycle_PreservesRegistry` may pass even pre-change because the RESPAWN arm doesn't touch the registry — that's fine, it pins non-regression.

- [ ] **Step 3: Edit `modules/world/npc_registry.go:206-209`**

Replace the existing block:

```go
	if n.lifecycle == NpcLifecycleDespawn {
		// TODO(NAI-19): full registry cleanup (delete from s.npcs[],
		// splice s.npcLoop) remains deferred per pre-existing dead-bool
		// model — see npc_registry.go header history.
	} else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
```

with:

```go
	if n.lifecycle == NpcLifecycleDespawn {
		// NAI-19: TS World.ts:1312-1315 — rsbuf.removeNpc already fired
		// above; release the registry slot and run Cleanup. The
		// s.npcLoop splice is deferred to compactNpcLoop (end-of-tick)
		// per NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE to keep
		// mid-tick iteration safe. Order matters: nil the slot BEFORE
		// Cleanup, because Cleanup sets n.nid = -1.
		s.npcs[n.nid] = nil
		n.Cleanup()
	} else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
```

Use the Edit tool with the full multi-line `if n.lifecycle == NpcLifecycleDespawn { … } else if` block as `old_string` to ensure uniqueness.

- [ ] **Step 4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestRemoveNpc_DespawnLifecycle_ClearsRegistrySlot|TestRemoveNpc_DespawnLifecycle_RunsCleanup|TestRemoveNpc_RespawnLifecycle_PreservesRegistry' -count=1
```
Expected: PASS.

- [ ] **Step 5: Run the broader test suite for non-regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```
Expected: PASS. The existing `TestRemoveNpcDespawnLifecycleSkipsLifecycleTick` (`npc_registry_test.go:72`) must still pass — it asserts `n.dead == true` and `n.lifecycleTick == 99`, neither of which the T2 change disturbs.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_registry.go modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 T2 — release nid slot + Cleanup on DESPAWN-lifecycle

(*Server).removeNpc DESPAWN-arm now mirrors TS World.ts:1312-1315:
s.npcs[n.nid] = nil (release slot for allocNpcSlot reuse), then
n.Cleanup() (zero identity/script/hunt/queue fields). Order matters:
slot release MUST precede Cleanup because Cleanup sets n.nid = -1.

s.npcLoop splice deferred to T3's compactNpcLoop (end-of-tick) per
NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE.

Three new pins in npc_registry_test.go:
- ClearsRegistrySlot — slot release
- RunsCleanup — field zeroing
- RespawnLifecycle_PreservesRegistry — non-regression of RESPAWN path

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Add `(*Server).compactNpcLoop` helper (TDD)

**Files:**
- Modify: `modules/world/tick.go` (append helper after `processCleanup` body or before — find a clean spot)
- Test: `modules/world/npc_registry_test.go` (append at end)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_registry_test.go`:

```go
// TestCompactNpcLoop_PrunesDespawnedDead pins NAI-19 end-of-tick pruning:
// DESPAWN-lifecycle dead NPCs are removed from s.npcLoop; alive NPCs
// and RESPAWN-lifecycle dead NPCs are preserved. The pruning predicate
// is (n.dead && n.lifecycle == NpcLifecycleDespawn).
func TestCompactNpcLoop_PrunesDespawnedDead(t *testing.T) {
	s := newTestServer(t)
	alive := &Npc{nid: 1, lifecycle: NpcLifecycleRespawn, dead: false}
	respawnDead := &Npc{nid: 2, lifecycle: NpcLifecycleRespawn, dead: true}
	despawnDead := &Npc{nid: 3, lifecycle: NpcLifecycleDespawn, dead: true}
	s.npcLoop = []*Npc{alive, respawnDead, despawnDead}

	s.compactNpcLoop()

	if len(s.npcLoop) != 2 {
		t.Fatalf("len(npcLoop): got %d, want 2", len(s.npcLoop))
	}
	if s.npcLoop[0] != alive {
		t.Errorf("npcLoop[0]: got %p, want %p (alive must be preserved)", s.npcLoop[0], alive)
	}
	if s.npcLoop[1] != respawnDead {
		t.Errorf("npcLoop[1]: got %p, want %p (RESPAWN+dead must be preserved)", s.npcLoop[1], respawnDead)
	}
	for _, n := range s.npcLoop {
		if n == despawnDead {
			t.Errorf("npcLoop still contains DESPAWN+dead %p", despawnDead)
		}
	}
}

// TestCompactNpcLoop_TailNilledForGC pins defensive GC-hint: trailing
// slots in the slice's capacity are nilled to drop pointer retention.
func TestCompactNpcLoop_TailNilledForGC(t *testing.T) {
	s := newTestServer(t)
	alive := &Npc{nid: 1, lifecycle: NpcLifecycleRespawn, dead: false}
	despawnDead := &Npc{nid: 2, lifecycle: NpcLifecycleDespawn, dead: true}
	s.npcLoop = []*Npc{alive, despawnDead}

	s.compactNpcLoop()

	// After compact, len == 1; the underlying capacity-slot [1] must be nil.
	full := s.npcLoop[:cap(s.npcLoop)]
	if full[1] != nil {
		t.Errorf("trailing slot full[1]: got %p, want nil (GC-hint required)", full[1])
	}
}

// TestRemoveNpc_DespawnLifecycle_SlotReusable pins NAI-19 round-trip:
// after removeNpc + compactNpcLoop, the allocator reuses the freed nid.
// Moved here from T2 because compactNpcLoop is defined in T3.
func TestRemoveNpc_DespawnLifecycle_SlotReusable(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n1 := NewNpc(0, 7, 100, 100, 0, typ)
	n1.nid = 1
	n1.server = s
	s.npcs[1] = n1
	s.npcLoop = append(s.npcLoop, n1)
	n1.lifecycle = NpcLifecycleDespawn

	s.nextNpcSlot = 1 // force allocator to start at slot 1
	s.removeNpc(n1, -1)
	s.compactNpcLoop()

	reused := s.allocNpcSlot()
	if reused != 1 {
		t.Errorf("allocNpcSlot: got %d, want 1 (freed slot must be reusable)", reused)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestCompactNpcLoop_PrunesDespawnedDead|TestCompactNpcLoop_TailNilledForGC|TestRemoveNpc_DespawnLifecycle_SlotReusable' -count=1
```
Expected: compilation FAIL — `s.compactNpcLoop undefined`.

- [ ] **Step 3: Write the helper**

In `modules/world/tick.go`, append after `processCleanup` (around line 670):

```go
// compactNpcLoop prunes DESPAWN-lifecycle dead NPCs from s.npcLoop.
// Called once per tick from processCleanup AFTER NpcInfo writes have
// completed — the just-despawned NPC's removal mask is already in the
// client write stream via rsbuf.RemoveNpc (called from removeNpc).
// RESPAWN-lifecycle dead NPCs are preserved; their dead=true flips on
// the next lifecycleTick==0 in npc_ai.go's processNpcLifecycle.
//
// Mirrors TS's per-zone linked-list splice in World.removeNpc, which
// goscape can't do safely mid-iteration (s.npcLoop is an append-only
// slice). End-of-tick mark/compact is observably identical at tick
// boundaries. Tracked deviation: NAI-19-D-DEFERRED-COMPACT-VS-
// IMMEDIATE-SPLICE.
func (s *Server) compactNpcLoop() {
	write := 0
	for _, n := range s.npcLoop {
		if n.dead && n.lifecycle == NpcLifecycleDespawn {
			continue
		}
		s.npcLoop[write] = n
		write++
	}
	for i := write; i < len(s.npcLoop); i++ {
		s.npcLoop[i] = nil // GC hint: drop pointer retention
	}
	s.npcLoop = s.npcLoop[:write]
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestCompactNpcLoop_PrunesDespawnedDead|TestCompactNpcLoop_TailNilledForGC|TestRemoveNpc_DespawnLifecycle_SlotReusable' -count=1
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/tick.go modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 T3 — compactNpcLoop end-of-tick prune helper

Prunes DESPAWN-lifecycle dead NPCs from s.npcLoop. RESPAWN+dead
preserved (will respawn at lifecycleTick==0). GC-hint nils trailing
capacity slots. Helper only — wire-up into processCleanup lands in T4.

Tracked deviation: NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE
(goscape's append-only s.npcLoop can't safely splice mid-iteration;
end-of-tick mark/compact is observably identical at tick boundaries).

Three new pins:
- PrunesDespawnedDead — RESPAWN+dead preserved, alive preserved, DESPAWN+dead pruned
- TailNilledForGC — capacity-slot GC-hint
- SlotReusable — round-trip removeNpc → compactNpcLoop → allocNpcSlot returns freed nid

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Wire `compactNpcLoop` into `processCleanup` (TDD)

**Files:**
- Modify: `modules/world/tick.go:631-670` (`processCleanup` body)
- Test: `modules/world/npc_registry_test.go` (append at end) — or `tick_test.go` if it exists; pre-flight check.

- [ ] **Step 1: Pre-flight — verify tick_test.go existence**

```bash
ls modules/world/tick_test.go 2>/dev/null && echo "exists" || echo "absent"
```

If `tick_test.go` exists, add the integration test there. Otherwise append to `npc_registry_test.go` (where the other compactNpcLoop tests live).

- [ ] **Step 2: Write the failing tests**

Append to the chosen test file:

```go
// TestProcessCleanup_RunsCompactNpcLoop pins the NAI-19 T4 wire-up:
// processCleanup invokes compactNpcLoop, so a DESPAWN-lifecycle dead
// NPC pre-seeded in s.npcLoop is gone after one processCleanup call.
func TestProcessCleanup_RunsCompactNpcLoop(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.server = s
	n.lifecycle = NpcLifecycleDespawn
	n.dead = true
	s.npcLoop = append(s.npcLoop, n)

	s.processCleanup()

	for _, m := range s.npcLoop {
		if m == n {
			t.Errorf("npcLoop still contains DESPAWN+dead NPC %p after processCleanup", n)
		}
	}
}

// TestRemoveNpcDuringTickIteration_NoPanic pins NAI-19 mid-tick safety:
// when one NPC's processing triggers removeNpc on itself while
// s.npcLoop is being iterated, the iteration completes without panic
// and end-of-tick compact (via processCleanup) prunes the dead entry.
//
// Simulates a real tick by iterating s.npcLoop directly (mirrors
// processNpcs at tick.go:577) and calling s.removeNpc inside the loop
// on one of the entries, then running processCleanup.
func TestRemoveNpcDuringTickIteration_NoPanic(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	target := NewNpc(0, 7, 100, 100, 0, typ)
	target.nid = 1
	target.server = s
	target.lifecycle = NpcLifecycleDespawn
	s.npcs[1] = target

	bystander := NewNpc(0, 7, 101, 100, 0, typ)
	bystander.nid = 2
	bystander.server = s
	bystander.lifecycle = NpcLifecycleRespawn
	s.npcs[2] = bystander

	s.npcLoop = []*Npc{target, bystander}

	// Mid-tick iteration: removeNpc on first entry; iteration continues
	// safely because s.npcLoop is not spliced mid-loop.
	for _, n := range s.npcLoop {
		if n == target {
			s.removeNpc(n, -1) // does NOT mutate s.npcLoop
		}
	}

	// End-of-tick compact runs from processCleanup.
	s.processCleanup()

	if len(s.npcLoop) != 1 || s.npcLoop[0] != bystander {
		t.Errorf("npcLoop after compact: got %v, want [%p] (bystander only)", s.npcLoop, bystander)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessCleanup_RunsCompactNpcLoop|TestRemoveNpcDuringTickIteration_NoPanic' -count=1
```
Expected: FAIL — `processCleanup` does not yet call `compactNpcLoop`.

- [ ] **Step 4: Wire into `processCleanup`**

Edit `modules/world/tick.go` `processCleanup` body. Find the section near the end (around line 667):

```go
	if s.rsbuf != nil {
		s.rsbuf.Cleanup()
	}
}
```

and insert the compact call BEFORE the rsbuf.Cleanup (or after — either is correct since they operate on disjoint state; place BEFORE for symmetry with the npc ResetMasks loop):

```go
	// NAI-19: prune DESPAWN-lifecycle dead NPCs from s.npcLoop at
	// end-of-tick. Runs AFTER processInfo's NpcInfo writes (which
	// fire upstream in processTick) so the just-despawned NPC's
	// removal mask is already in the client stream via rsbuf.RemoveNpc.
	s.compactNpcLoop()

	if s.rsbuf != nil {
		s.rsbuf.Cleanup()
	}
}
```

Use the Edit tool with the `if s.rsbuf != nil { … }` block (including the line above it for uniqueness) as `old_string`.

- [ ] **Step 5: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessCleanup_RunsCompactNpcLoop|TestRemoveNpcDuringTickIteration_NoPanic' -count=1
```
Expected: PASS.

- [ ] **Step 6: Run the full modules/world suite for non-regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/tick.go modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-19 T4 — wire compactNpcLoop into processCleanup

processCleanup now calls compactNpcLoop before rsbuf.Cleanup. Pruning
happens AFTER processInfo's NpcInfo writes have flushed via rsbuf, so
clients see the removal mask for the just-despawned NPC. End-of-tick
ordering: per-tick NPC processing → processInfo (NpcInfo write) →
processCleanup (compactNpcLoop → rsbuf.Cleanup).

Two new pins:
- ProcessCleanup_RunsCompactNpcLoop — wire-up
- RemoveNpcDuringTickIteration_NoPanic — mid-tick safety

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Retire NAI-19 doc-comment references in pkg/script

**Files:**
- Modify: `pkg/script/state.go:107` (doc-comment referencing the gap)
- Modify: `pkg/script/handlers_npc.go:318` (handler doc-comment referencing the gap)

- [ ] **Step 1: Read the existing doc-comments**

```bash
sed -n '100,110p' pkg/script/state.go
sed -n '310,325p' pkg/script/handlers_npc.go
```

Pre-flight identified the content. These are doc-comments narrating the gap; with NAI-19 closed, they should be updated, not deleted (the dead-bool model still partially exists — defensive guards remain — but the gap text is stale).

- [ ] **Step 2: Edit `pkg/script/state.go:107`**

Find the comment line:
```
	// DESPAWN-lifecycle (registry cleanup deferred per TODO(NAI-19) at
```
and the surrounding 1-2 lines of context (read the full comment block first via Read).

Replace `registry cleanup deferred per TODO(NAI-19) at` with `registry cleanup landed in NAI-19;` — adjust the surrounding sentence so it reads naturally as a current-state description rather than a deferral note.

**Plan-author judgment:** the implementer reads the full comment, rewrites in-place to remove "deferred" framing while keeping the documentation value. If the entire comment was only meaningful as a deferral note, delete it. Use Read first to see the full sentence structure.

- [ ] **Step 3: Edit `pkg/script/handlers_npc.go:318`**

Same approach: find the doc-comment referencing `TODO(NAI-19)` and reword to indicate the gap is closed. The existing text (per pre-flight) is:
```
// duration passed to World.RemoveNpc is the active NPC type's
// respawnrate; Server.removeNpc scales it by player count and writes
// it to lifecycleTick (RESPAWN-lifecycle) or is a no-op for
// DESPAWN-lifecycle (registry cleanup deferred per dead-bool model —
// see modules/world/npc_registry.go:181 and TODO(NAI-19)).
```

Replace with:
```
// duration passed to World.RemoveNpc is the active NPC type's
// respawnrate; Server.removeNpc scales it by player count and writes
// it to lifecycleTick (RESPAWN-lifecycle) or, on DESPAWN-lifecycle,
// releases the registry slot and runs Cleanup (NAI-19; see
// modules/world/npc_registry.go).
```

- [ ] **Step 4: Verify no remaining `TODO(NAI-19)` in production**

```bash
rg -n 'TODO\(NAI-19\)' modules/ pkg/
```
Expected: zero hits.

- [ ] **Step 5: Run the full suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/... ./pkg/script/...
```
Expected: PASS, clean vet.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/state.go pkg/script/handlers_npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(script): NAI-19 T5 — retire deferred-per-NAI-19 doc-comments

Updates pkg/script/state.go and pkg/script/handlers_npc.go doc-comments
that narrated the NAI-19 registry-cleanup deferral. Reword to current-
state: DESPAWN-lifecycle releases the slot and runs Cleanup (landed in
NAI-19 T2/T3/T4).

Verifies zero remaining `TODO(NAI-19)` in production code.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Final verification + close commit

- [ ] **Step 1: Run the full modules/world + pkg/script suites**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/... -count=1
```
Expected: PASS.

- [ ] **Step 2: Run `go vet` on touched packages**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/... ./pkg/script/...
```
Expected: clean.

- [ ] **Step 3: Grep verification**

```bash
rg -n 'TODO\(NAI-19\)' modules/ pkg/   # zero hits
rg -n 'NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE' modules/   # ≥1 hit (the comment in tick.go and/or npc_registry.go)
```

- [ ] **Step 4: Stage the plan doc + close commit**

```bash
git add docs/superpowers/plans/2026-05-11-nai-19-npc-despawn-registry-cleanup.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-19 — NPC DESPAWN-lifecycle registry cleanup landed

Closes the long-deferred NAI-19 follow-up at npc_registry.go:206.
TS World.removeNpc DESPAWN-lifecycle parity now in place:
  - s.npcs[n.nid] = nil — registry slot released for reuse
  - n.Cleanup() — TS Npc.cleanup parity (nid=-1, uid=-1, activeScript=
    nil, huntTarget=nil, queue=nil)
  - compactNpcLoop — end-of-tick mark/compact prunes DESPAWN+dead from
    s.npcLoop (RESPAWN+dead preserved)

Tracked deviation introduced:
NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE — goscape's append-only
s.npcLoop can't safely splice mid-iteration; end-of-tick mark/compact
is observably identical at tick boundaries.

Implementation: T1 (Cleanup method) → T2 (slot release + Cleanup wire-
up) → T3 (compactNpcLoop helper) → T4 (processCleanup wire-up) → T5
(doc-comment retirement).

Closes memory: nai_followups.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5: Update memory**

After the close commit lands, the controller (not implementer) updates `nai_followups.md` to add a "From NAI-19 (2026-05-11) — registry cleanup closed" section with the close-commit SHA and the introduced deviation tag. **This task is the controller's, not the implementer's.**

---

## Verification matrix (controller checklist before close)

| Check | Command | Expected |
|---|---|---|
| All NAI-19 tests pass | `go test ./modules/world/ -run 'TestNpcCleanup\|TestRemoveNpc_DespawnLifecycle\|TestRemoveNpc_RespawnLifecycle\|TestCompactNpcLoop\|TestProcessCleanup_RunsCompactNpcLoop\|TestRemoveNpcDuringTickIteration' -count=1 -v` | PASS, all 8 new tests |
| Full world suite green | `go test ./modules/world/... -count=1` | PASS |
| Full script suite green | `go test ./pkg/script/... -count=1` | PASS |
| go vet clean | `go vet ./modules/world/... ./pkg/script/...` | clean |
| TODO(NAI-19) retired | `rg -n 'TODO\(NAI-19\)' modules/ pkg/` | 0 hits |
| Deviation tag present | `rg -n 'NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE' modules/` | ≥1 hit |
| Slot reuse round-trip | (covered by TestRemoveNpc_DespawnLifecycle_SlotReusable) | PASS |
| Mid-tick safety | (covered by TestRemoveNpcDuringTickIteration_NoPanic) | PASS |
| Non-regression (existing NpcLifecycleDespawn tests) | `go test ./modules/world/ -run 'TestRemoveNpcDespawnLifecycleSkipsLifecycleTick\|TestNpcChangeTypeBaseTypeDespawnNoFastPath' -count=1` | PASS |

---

## Task summary

| Task | Type | LOC (prod) | LOC (test) | Files touched |
|---|---|---|---|---|
| T1 | TDD: Cleanup method | ~12 | ~30 | npc.go, npc_test.go |
| T2 | TDD: removeNpc DESPAWN-arm | ~7 | ~80 | npc_registry.go, npc_registry_test.go |
| T3 | TDD: compactNpcLoop helper | ~20 | ~70 | tick.go, npc_registry_test.go |
| T4 | TDD: processCleanup wire-up | ~5 | ~50 | tick.go, npc_registry_test.go (or tick_test.go) |
| T5 | Docs cleanup | ~0 (rewrite ~6 lines comment) | 0 | pkg/script/state.go, pkg/script/handlers_npc.go |
| T6 | Close commit | 0 | 0 | docs/plans/ |
| **Total** | | **~44** | **~230** | 5 production files, 2-3 test files |

Cadence: medium. Single Sonnet implementer via subagent-driven-development. Formal code review at end (Sonnet code-quality reviewer per `superpowers_code_reviewer_model` memory).
