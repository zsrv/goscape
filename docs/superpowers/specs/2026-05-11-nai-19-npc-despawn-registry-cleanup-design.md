# NAI-19 — NPC DESPAWN-lifecycle registry cleanup

**Status:** Brainstorm complete; ready for plan.

**Scope label:** Medium-cadence (per `runescript_cadence`). Single bundle. ~50-70 LOC production + ~150-200 LOC tests. Single Sonnet implementer + one stage of formal code review.

**Tech stack:** Go 1.26+, project root `/home/owner/Code/github.com/zsrv/goscape`.

## 1. Motivation

NAI-19 was tagged at the original NPC-AI port and deferred since. Inline TODOs remain at:
- `modules/world/npc_registry.go:206` — empty `if n.lifecycle == NpcLifecycleDespawn { … TODO(NAI-19) … }`
- `pkg/script/state.go:107` — doc-comment referencing the gap
- `pkg/script/handlers_npc.go:318` — handler doc-comment referencing the gap

The "pre-existing dead-bool model" keeps DESPAWN'd NPCs in `s.npcs[]` and `s.npcLoop` forever; every consumer guards on `n.dead`. Behavior is correct, but:
- The nid slot (pool of ~8191) is never reclaimed → long-running servers exhaust the allocator.
- `s.npcLoop` grows monotonically → wasted tick iteration over dead entries.
- TS `World.removeNpc` (`World.ts:1312-1315`) does `npcs.remove(nid) + npc.cleanup()` in the DESPAWN arm — goscape's gap is a TS-fidelity divergence.

TS reference (`Engine-TS/src/engine/World.ts:1296-1319`):

```ts
removeNpc(npc: Npc, duration: number): void {
    const zone = this.gameMap.getZone(npc.x, npc.z, npc.level);
    const adjustedDuration = this.scaleByPlayerCount(duration);
    zone.leave(npc);
    npc.isActive = false;
    // ... collision toggles ...
    if (npc.lifecycle === EntityLifeCycle.DESPAWN) {
        rsbuf.removeNpc(npc.nid);
        this.npcs.remove(npc.nid);
        npc.cleanup();
    } else if (npc.lifecycle === EntityLifeCycle.RESPAWN && duration > -1) {
        npc.setLifeCycle(adjustedDuration);
    }
}
```

TS `Npc.cleanup()` (`Engine-TS/src/engine/entity/Npc.ts:187-193`):

```ts
cleanup(): void {
    this.nid = -1;
    this.uid = -1;
    this.activeScript = null;
    this.huntTarget = null;
    this.queue.clear();
}
```

Goscape's `(*Server).removeNpc` (`modules/world/npc_registry.go:178-212` at HEAD) already runs `rsbuf.RemoveNpc`, zone.LeaveNpc, `n.dead=true`, and collision toggles. The DESPAWN arm body is the only missing piece.

## 2. Architecture

Three structural changes:

### 2.1 Slot release + cleanup at `(*Server).removeNpc` DESPAWN branch

`modules/world/npc_registry.go` — replace the empty TODO arm:

```go
if n.lifecycle == NpcLifecycleDespawn {
    // TODO(NAI-19): full registry cleanup (delete from s.npcs[],
    // splice s.npcLoop) remains deferred per pre-existing dead-bool
    // model — see npc_registry.go header history.
}
```

with:

```go
if n.lifecycle == NpcLifecycleDespawn {
    // NAI-19: TS World.ts:1312-1315 — rsbuf.removeNpc already fired
    // above; release the registry slot and run cleanup. The
    // s.npcLoop splice is deferred to compactNpcLoop (end-of-tick)
    // to keep mid-tick iteration safe — see NAI-19-D-DEFERRED-COMPACT-
    // VS-IMMEDIATE-SPLICE in the deviation tracker.
    s.npcs[n.nid] = nil
    n.Cleanup()
}
```

**Order constraint:** `s.npcs[n.nid] = nil` MUST precede `n.Cleanup()` because Cleanup sets `n.nid = -1`.

### 2.2 New `(n *Npc) Cleanup()` method

`modules/world/npc.go` — exported (Capitalized) so test code can pin the post-state:

```go
// Cleanup mirrors TS Npc.cleanup at Engine-TS/src/engine/entity/Npc.ts:187-193.
// Zeros the identity / script / hunt / queue fields after removeNpc
// has released the registry slot. Defensive nullification — TS does
// this so any consumer still holding the *Npc reference sees -1
// sentinels rather than valid-looking state.
func (n *Npc) Cleanup() {
    n.nid = -1
    n.uid = -1
    n.activeScript = nil
    n.huntTarget = nil
    n.queue = nil
}
```

**Field-name plan-write recheck:** all five fields must exist on `*Npc` at HEAD. Today's pre-flight confirmed `huntTarget` (`npc_hunt.go:63`), `activeScript` (`npc_script.go`), `queue` (`npc_script.go:517`, `npc_registry.go:142`). `nid` and `uid` are universally referenced. Plan-author re-greps `rg -n "n\.nid\b|n\.uid\b" modules/world/npc.go` to confirm field exists on the struct.

**`n.queue = nil` vs `n.queue = n.queue[:0]`:** TS calls `this.queue.clear()` (linked-list clear). Go slice `nil` releases backing array (GC-friendly); `[:0]` retains capacity. For a despawned NPC the slot is reusable but `n` itself is unreachable from `s.npcs` so the backing array is GC'd anyway. Choose `nil` for clarity (matches TS "release storage" intent).

### 2.3 New `(*Server).compactNpcLoop` end-of-tick pass

`modules/world/tick.go` — add helper and wire into `processTick` after all per-NPC processing AND after the NpcInfo write phase:

```go
// compactNpcLoop prunes DESPAWN-lifecycle dead NPCs from s.npcLoop.
// Called once per tick from processTick AFTER NpcInfo writes — the
// client must still see the removal mask for the just-despawned NPC
// (rsbuf.RemoveNpc has already registered it). RESPAWN-lifecycle
// dead NPCs are preserved; they will flip dead=false on their next
// lifecycleTick==0 (see npc_ai.go:31-45).
//
// Mirrors TS's immediate zone-list splice in World.removeNpc (which
// goscape can't do safely mid-iteration); behavior is observably
// identical at tick boundaries. Tracked as NAI-19-D-DEFERRED-COMPACT-
// VS-IMMEDIATE-SPLICE.
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

**Wire-up site:** plan-author identifies the last call in `processTick` after NpcInfo writes complete. Pre-flight: `tick.go:493-654` is the NPC processing block; compactNpcLoop fires after the final per-NPC iteration block. Plan-author reads the full tick to confirm placement.

## 3. Test surface

### 3.1 New tests

**`modules/world/npc_registry_test.go`:**

1. `TestRemoveNpc_DespawnLifecycle_ClearsRegistrySlot` — after `s.removeNpc(n, -1)` with `n.lifecycle = NpcLifecycleDespawn`, capture nid pre-call, assert `s.npcs[capturedNid] == nil`.
2. `TestRemoveNpc_DespawnLifecycle_RunsCleanup` — after removeNpc, assert `n.nid == -1`, `n.uid == -1`, `n.activeScript == nil`, `n.huntTarget == nil`, `n.queue == nil`.
3. `TestRemoveNpc_DespawnLifecycle_SlotReusable` — after removeNpc + `s.compactNpcLoop()`, a fresh `s.addNpc(n2, -1, true)` returns the freed nid (allocator round-robin scans for nil and finds the slot).
4. `TestRemoveNpc_RespawnLifecycle_PreservesRegistry` — with `n.lifecycle = NpcLifecycleRespawn`, assert `s.npcs[n.nid] == n` AND `n.nid` is unchanged AND fields are NOT zeroed (no Cleanup fires).

**`modules/world/tick_test.go` (or `npc_registry_test.go`):**

5. `TestCompactNpcLoop_PrunesDespawnedDead` — pre-seed `s.npcLoop` with three NPCs (one alive, one RESPAWN+dead, one DESPAWN+dead); after `s.compactNpcLoop()`, length is 2, the DESPAWN+dead is absent, the RESPAWN+dead is preserved.
6. `TestCompactNpcLoop_TailNilledForGC` — verifies trailing capacity slots are nilled. Defensive.
7. `TestProcessTick_RunsCompactNpcLoop` — integration: DESPAWN-lifecycle NPC at `lifecycleTick = 0`, drive one tick, assert `s.npcLoop` no longer contains it AND `s.npcs[oldNid] == nil`.
8. `TestRemoveNpcDuringTickIteration_NoPanic` — two NPCs in `s.npcLoop`; the first's `processNpcLifecycle` triggers `removeNpc` on itself; assert no panic during iteration, second NPC processes normally, end-of-tick compact prunes the first.

**`modules/world/npc_test.go`:**

9. `TestNpcCleanup_IsolatedUnit` — direct `(*Npc).Cleanup()` call outside removeNpc; pins the field-zeroing contract.

### 3.2 Existing tests to revisit (plan-write task)

Pre-flight grep identified three sites with `n.lifecycle = NpcLifecycleDespawn` pre-set:
- `modules/world/npc_registry_test.go:80`
- `modules/world/npc_event_queue_test.go:159`
- `modules/world/npc_test.go:150`

Plan-author reads each enclosing test and:
- If it asserts `s.npcs[n.nid] == n` or "still in npcLoop" post-removeNpc → invert.
- If it only asserts `n.dead == true` or unrelated state → leave alone.
- If it relies on `n.nid` staying valid after removeNpc → capture nid before the call.

## 4. Risk register

See brainstorm summary. Reproduced as concise table:

| Risk | Mitigation |
|---|---|
| `s.npcs[slot]` readers don't nil-gate | Plan-write task: `rg -n "s\.npcs\[" modules/ pkg/`; audit each site; today's pre-flight enumerated `handler_opnpc.go:59,144,238`, `npc_script_lookup.go:133`, `server_varp.go:249`. Add nil-gates where missing (current-behavior correct: unspawned slots are already nil). |
| Script `state.ActiveNpc` holds stale pointer post-DESPAWN | No regression: `n.dead=true` is still set; existing dead-bool guards (script resume gate, etc.) still short-circuit. Cleanup just adds nid=-1/uid=-1 sentinels. Defensive test: drive NpcSuspended state across DESPAWN, assert resume gate short-circuits. |
| Field-name confusion: `n.queue` vs `engineQueue` | NPC field is `n.queue` (engineQueue is player-side). Pre-flight confirmed via `npc_registry.go:142` and `npc_script.go:517`. Plan-author re-greps. |
| Mid-tick removal during npcLoop iteration | End-of-tick compact (design); pinned by Test 8. |
| RESPAWN-lifecycle NPCs accidentally pruned | `compactNpcLoop` predicate is `n.dead && lifecycle==Despawn`; RESPAWN+dead preserved. Pinned by Test 5 + Test 4. |
| nid reuse hazard — script captures nid, slot reused mid-session | Same as TS. Scripts capture `*Npc` pointer, not nid alone. Reused nid → different `*Npc`; old script's `state.ActiveNpc` references the old (cleaned, dead) one. No regression. Noted in spec; no mitigation. |
| Compact timing vs NpcInfo encoder | Compact fires AFTER NpcInfo writes — client still sees the removal mask via `rsbuf.RemoveNpc` (already called at top of removeNpc). Plan-author verifies ordering at wire-up. |
| Tests asserting dead-NPC retention | §3.2 above. |
| Double-remove from rsbuf | rsbuf.RemoveNpc fires at top of removeNpc; Cleanup zeros nid AFTER. No double-call. |

## 5. Consumer audit checklist (plan-write tasks)

1. `rg -n "s\.npcs\[" modules/world/ pkg/script/` — every read site has nil-handling.
2. `rg -n "NpcLifecycleDespawn" modules/world/*_test.go` — every test setting this flag is read for retention assumptions.
3. `tick.go` processTick ordering: NpcInfo write → compactNpcLoop (in that order; not reversed). Read the full function.
4. Field names exist on `*Npc`: `nid`, `uid`, `activeScript`, `huntTarget`, `queue`. Re-grep `rg -n "\\bnid\\b|\\buid\\b" modules/world/npc.go | head -10` at struct definition.

## 6. Tracked deviations

**NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE** — introduced.
- **Rationale:** Goscape uses an append-only `s.npcLoop []*Npc` slice, not TS's per-zone linked list. Splicing during mid-tick iteration is unsafe in Go. End-of-tick mark/compact is observably identical at tick boundaries (rsbuf.RemoveNpc has already registered removal in the client write stream by the time compact runs).
- **Closure criterion:** A future sub-spec that migrates `s.npcLoop` to a linked-list / per-zone structure (matching TS NpcList) could retire this deviation. Not anticipated; no follow-up needed.

## 7. Cadence & verification

**Cadence:** Medium. Spec → plan → subagent-driven TDD with single Sonnet implementer + one stage of formal code review.

**Verification before close:**
1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1` — all new tests pass; no existing tests regressed.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...` — clean.
3. Grep verification: `rg -n "TODO\\(NAI-19\\)" modules/ pkg/` returns zero hits in production code (doc-comments at `pkg/script/state.go:107` and `pkg/script/handlers_npc.go:318` updated to remove the "deferred per NAI-19" mention and replace with a "NAI-19 closed; see World.removeNpc" reference, or just dropped if redundant).

## 8. Memory follow-up

- Update `nai_followups.md`: add "From NAI-19 close (2026-05-11)" section noting the close commit and the introduced `NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE` deviation tag.
- No new auto-memory entry needed — the design is well-documented in this spec and the inline comments. If any unexpected lesson surfaces during implementation (e.g., a consumer audit reveals a nil-deref bug), capture it as a `feedback` or `project` memory at close.

## 9. Out of scope

- Player-side equivalent (`s.players[]` slot release on logout). Player logout already runs explicit cleanup in `modules/world/player_logout.go` (verify name at plan-write) — different lifecycle model, different audit.
- Obj-side equivalent (`RemoveObj` registry cleanup). NAI-115-D2 tracks separately.
- Loc-side equivalent. Locs use a different lifecycle (zone-managed, not slot-indexed).
- `s.npcLoop` data-structure migration to TS-equivalent linked list. Deferred indefinitely (see §6 deviation).
