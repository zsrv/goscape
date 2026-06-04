# NAI-152 B2 — pickup chain (OBJ_TYPE handler + Obj reach-check)

## 1. Scope

Sub-spec for NAI-152 §6.3 Bundle 2, activated by the B1 smoke. B1 (`8c76405`)
closed the handler short-circuit at `handleOpObj`; the resulting mindrune
pickup smoke surfaced two downstream blockers on the pickup chain:

1. **`[label,pickup_obj_table]` script crash** — `pkg/script/handlers.go`
   dispatch raises `no handler for OBJ_TYPE (opcode 3511) at pc=0`. The
   opcode is declared (`pkg/script/opcode.go:319,1020`) but no handler is
   registered. Matches the `protocol_stub_not_completed.md` pattern.
2. **`"I can't reach that!"` chat** — `modules/world/interaction.go:271`
   emits when `inOperableDistance(p, *Obj)` falls through to the Chebyshev
   fallback (NAI-91-D-OPERABLE-CHEB-FALLBACK), which excludes same-tile.
   Picking up an item on the player's own tile always fails the reach check.

Both are pre-existing TS-fidelity gaps unblocked by B1. Single bundle,
three independent T-tasks. Smoke-gated close on the mindrune pickup
Java-client run.

## 2. Tech stack

Go 1.26+. No new deps. Touches `pkg/script` (T1) and `modules/world` (T2,
T3). `pkg/pathfinder/reach.Reached` is the existing port surface used by
the Loc branch of `inOperableDistance` and already imported in both
target files.

## 3. TS source

- **T1 OBJ_TYPE handler:** `Engine-TS/src/engine/script/handlers/ObjOps.ts:132-134`
  ```ts
  [ScriptOpcode.OBJ_TYPE]: state => {
      state.pushInt(check(state.activeObj.type, ObjTypeValid).id);
  },
  ```
- **T2 Player.inOperableDistance Obj branch:** `Engine-TS/src/engine/entity/Player.ts:1099-1111`
  ```ts
  inOperableDistance(target: Entity): boolean {
      if (target.level !== this.level) return false;
      if (target instanceof PathingEntity) { return reachedEntity(...); }
      else if (target instanceof Loc)      { return reachedLoc(...); }
      // instanceof Obj
      return reachedEntity(this.level, this.x, this.z, target.x, target.z,
                           target.width, target.length, this.width)
          || reachedObj(this.level, this.x, this.z, target.x, target.z,
                        target.width, target.length, this.width);
  }
  ```
- **T3 Npc.inOperableDistance Obj branch (base class):** `Engine-TS/src/engine/entity/PathingEntity.ts:378-390`
  ```ts
  inOperableDistance(target: Entity): boolean {
      if (target.level !== this.level) return false;
      if (target instanceof PathingEntity) { return reachedEntity(...); }
      else if (target instanceof Loc)      { return reachedLoc(...); }
      // instanceof Obj
      return reachedObj(this.level, this.x, this.z, target.x, target.z,
                        target.width, target.length, this.width);
  }
  ```
- **Reach helpers** (`Engine-TS/src/engine/GameMap.ts:394-404`):
  - `reachedEntity` → `rsmod.reached(..., 0, -2, 0)`
  - `reachedObj`    → `rsmod.reached(..., 0, -1, 0)`

The goscape port `pkg/pathfinder/reach.Reached` already implements the
TS shape: `locShape=-2` resolves to `rectangleExclusiveStrategy`,
`locShape=-1` to `noStrategy`. The same-tile short-circuit at
`strategy.go:37` is **gated on `strategy != rectangleExclusiveStrategy`**:

- `Reached(..., locShape=-1, ...)` on same-tile 1×1 returns `true` via
  the early-out (noStrategy enters the gate, returns true immediately).
- `Reached(..., locShape=-2, ...)` on same-tile 1×1 enters
  `ReachExclusiveRectangle`. `Collides(...)` returns `true` (1×1 boxes
  on the same tile overlap), so `!collides=false` and the method
  returns `false`. The exclusive-rectangle path is the conservative
  one — it expects `src` to be **outside** the dest box.

Same-tile pickup therefore relies on the **`reachedObj` (-1) arm** of
the OR-chain. `reachedEntity` (-2) returns false on same-tile 1×1;
`reachedObj` returns true. TS rsmod has the same semantics, so the OR
expression is TS-equivalent.

**TS asymmetry:** Player overrides `PathingEntity.inOperableDistance`
to OR `reachedEntity || reachedObj`; Npc inherits the base method
which only calls `reachedObj`. Goscape mirrors that asymmetry — Player
fix is the OR-chain, Npc fix is `reachedObj` only. Per
`audit_full_method_against_ts.md` + `ts_base_class_read_for_inherited_behavior.md`.

## 4. Existing surface

### 4.1. T1 (OBJ_TYPE handler)

- `pkg/script/opcode.go:319` — `OpObjType Opcode = 3511` declared.
- `pkg/script/opcode.go:1020` — `"OBJ_TYPE"` string mapping declared.
- `pkg/script/handlers.go:120-123` — OBJ family map block; missing
  `OpObjType: handleObjType` entry.
- `pkg/script/handlers_obj.go:11-15` — `requireActiveObj(s, op)` helper
  for the nil-ActiveObj guard.
- `pkg/script/state.go:304-306` — `ScriptState.ActiveObj` field.
- `pkg/script/state.go` `ActiveObj` interface — `ObjType() int` method
  on `*entity.Obj` returns the type id (`pkg/entity/obj.go:66`).

### 4.2. T2 (Player.inOperableDistance Obj branch)

- `modules/world/interaction.go:606-628` — current method. Loc branch
  uses `reach.Reached` with shape/angle/forceapproach. Non-Loc branches
  fall through to `inOperableDistanceCheb` under
  DEVIATION `NAI-91-D-OPERABLE-CHEB-FALLBACK`.
- `modules/world/interaction.go:269-274` — post-step caller; reach-fail
  emits `"I can't reach that!"` and clears interaction.
- `pkg/pathfinder/reach/strategy.go:35-53` — `Reached(...)` signature.
- `pkg/entity/entity.go:7` — `Entity.Width, Length` fields (exported,
  set to 1 at construction via `NewObj`).
- Caller-side reach-fail path (`interaction.go:267-274`) is unchanged.

### 4.3. T3 (Npc.inOperableDistance Obj branch)

- `modules/world/npc_interaction.go:675-696` — current method. Loc
  branch uses `reach.Reached`; non-Loc falls through to Cheb under the
  same NAI-91 deviation tag.
- `modules/world/npc_interaction.go:256` — `tryInteract` call site;
  consumes the bool result identically to the player side.
- Existing tests at `modules/world/npc_interaction_test.go:1738-1860` —
  NAI-91 shape-aware reach tests; T3 extends with Obj-target cases.

## 5. Stage 2 — bundle design

Single bundle, three independent tasks. No inter-task code dependencies.

### 5.1. T1 — handleObjType (pkg/script)

**Production changes (`pkg/script/handlers_obj.go`):**

```go
// handleObjType (OBJ_TYPE, opcode 3511) pushes the active obj's type id.
// Mirrors TS ObjOps.ts:132-134.
//
// TS-fidelity note: TS validates the type id via ObjTypeValid here; in
// goscape the active obj is always registered through Server.AddObj
// after wire-handler ObjType lookup (`handler_opobj.go:62-70`), so the
// id is round-trip-clean. (goscape defensive guard upstream; TS
// re-validates here.)
func handleObjType(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_TYPE"); err != nil {
        return err
    }
    s.PushInt(s.ActiveObj.ObjType())
    return nil
}
```

**Registration (`pkg/script/handlers.go:120-123`):** add
`OpObjType: handleObjType,` to the OBJ family block.

**Pin tests (`pkg/script/handlers_obj_test.go`):**

1. `TestHandleObjType_PushesActiveObjType` — `s := &ScriptState{StackCapacity: 4}`,
   `s.ActiveObj = &entity.Obj{Type: 558}`, `s.Pointers |= PtrActiveObj`.
   Call `handleObjType(s)`. Assert `s.PopInt() == 558`, no error.
2. `TestHandleObjType_NoActiveObj` — `s := &ScriptState{StackCapacity: 4}`,
   `s.ActiveObj = nil`. Call `handleObjType(s)`. Assert returns error
   `"OBJ_TYPE: no active obj"` (matches existing `requireActiveObj`
   wording).

Per `scriptstate_test_fixture_idioms.md` — `StackCapacity` init required.

**LOC:** ~10 production + ~25 test.

### 5.2. T2 — Player.inOperableDistance Obj branch (modules/world)

**Production change (`modules/world/interaction.go:606-628`):**

Replace the current method body. Loc branch unchanged; add Obj branch
between Loc and the PathingEntity fallthrough:

```go
func inOperableDistance(p *Player, target entity) bool {
    tx, tz, tlevel := target.Coords()
    if tlevel != p.level {
        return false
    }
    if loc, ok := target.(*entitypkg.Loc); ok {
        srv := p.client.server
        if srv.gamemap == nil {
            return inOperableDistanceCheb(p.x, p.z, tx, tz) // defensive
        }
        flags := srv.gamemap.Pathfinder.Flags
        var fap int
        if cfg := srv.locTypeOrNil(loc.Type()); cfg != nil {
            fap = cfg.ForceApproach
        }
        return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
            loc.Width, loc.Length, 1, loc.Angle(), loc.Shape(), fap)
    }
    if obj, ok := target.(*entitypkg.Obj); ok {
        // TS Player.ts:1110 — reachedEntity || reachedObj. Same-tile
        // pickup succeeds via reach.Reached's srcX==destX && srcZ==destZ
        // early-out (strategy.go:37). reachedEntity uses locShape=-2,
        // reachedObj uses locShape=-1.
        srv := p.client.server
        if srv.gamemap == nil {
            return inOperableDistanceCheb(p.x, p.z, tx, tz) // defensive
        }
        flags := srv.gamemap.Pathfinder.Flags
        if reach.Reached(flags, p.level, p.x, p.z, tx, tz,
            obj.Width, obj.Length, p.Width(), 0, -2, 0) {
            return true
        }
        return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
            obj.Width, obj.Length, p.Width(), 0, -1, 0)
    }
    return inOperableDistanceCheb(p.x, p.z, tx, tz)
}
```

**Doc-comment update:** strike "and Obj" from the NAI-91 deviation
sentence; deviation now applies only to PathingEntity (Player/Npc)
targets pending entity-shape port.

**Pin tests (`modules/world/interaction_test.go`):**

1. `TestInOperableDistance_Obj_SameTile` — player + obj at (3200, 3200);
   assert `inOperableDistance` returns `true`. Closes the mindrune
   pickup reach-fail.
2. `TestInOperableDistance_Obj_Adjacent` — player (3200, 3200), obj
   (3201, 3200); assert `true` (parity with previous Cheb pass).
3. `TestInOperableDistance_Obj_OutOfReach` — player (3200, 3200), obj
   (3210, 3200); assert `false`.
4. `TestInOperableDistance_Obj_CrossLevel` — player level 0, obj level 1
   same xz; assert `false` (preserves existing top-level guard).

Per `empty_flagmap_degenerate_routefinder.md` — tests must allocate the
collision map via `internal.BuildCollisionMap` (or the existing test
helper that wraps it), not `collision.NewFlagMap()` bare. Plan-author
greps `interaction_test.go` for the existing fixture helper used by the
NAI-91 Loc tests and reuses it.

**LOC:** ~30 production + ~60 test.

### 5.3. T3 — Npc.inOperableDistance Obj branch (modules/world)

**Production change (`modules/world/npc_interaction.go:675-696`):**

Add Obj branch before the Cheb fallthrough. TS base
(`PathingEntity.ts:389`) is `reachedObj` only — no OR-chain:

```go
func (n *Npc) inOperableDistance(target entity) bool {
    tx, tz, tlevel := target.Coords()
    if tlevel != n.level {
        return false
    }
    if loc, ok := target.(*entitypkg.Loc); ok && n.server != nil && n.server.gamemap != nil {
        flags := n.server.gamemap.Pathfinder.Flags
        var fap int
        if cfg := n.server.locTypeOrNil(loc.Type()); cfg != nil {
            fap = cfg.ForceApproach
        }
        srcSize := n.size
        if srcSize <= 0 {
            srcSize = 1
        }
        return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
            loc.Width, loc.Length, srcSize, loc.Angle(), loc.Shape(), fap)
    }
    if obj, ok := target.(*entitypkg.Obj); ok && n.server != nil && n.server.gamemap != nil {
        // TS PathingEntity.ts:389 — reachedObj only (Npc inherits the
        // base method; Player.ts:1110 is the override with the OR-chain).
        // Asymmetry intentional; see audit_full_method_against_ts.md +
        // ts_base_class_read_for_inherited_behavior.md.
        flags := n.server.gamemap.Pathfinder.Flags
        srcSize := n.size
        if srcSize <= 0 {
            srcSize = 1
        }
        return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
            obj.Width, obj.Length, srcSize, 0, -1, 0)
    }
    return inOperableDistanceCheb(n.x, n.z, tx, tz)
}
```

**Doc-comment update:** same trim as T2 — NAI-91 deviation
applies only to PathingEntity targets.

**Pin tests (`modules/world/npc_interaction_test.go`):**

1. `TestNpcInOperableDistance_Obj_SameTile` — npc + obj at (3200, 3200);
   `true`.
2. `TestNpcInOperableDistance_Obj_Adjacent` — adjacent tile; `true`.
3. `TestNpcInOperableDistance_Obj_OutOfReach` — distance>1; `false`.

Larger-than-1×1 NPC variants are scope-deferred (`n.size > 1` NPCs that
target Objs are not a smoke surface today; port if surfaced later).

**LOC:** ~25 production + ~45 test.

## 6. Test strategy summary

| Layer | New tests | LOC est. |
|---|---|---|
| `pkg/script` (T1) | 2 cases | ~25 |
| `modules/world` Player (T2) | 4 cases | ~60 |
| `modules/world` Npc (T3) | 3 cases | ~45 |

Cross-bundle regression: full `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
with race detector after each task lands. Plan-author preflight
enumerates existing `_test.go` fixtures that assert
`inOperableDistance(..., *Obj)` semantics under the current Cheb path
per `enumerate_all_sites.md` — any test that pinned same-tile=false
gets retired in the same commit per `latent_bug_at_migration_boundary.md`.

Java-client smoke gate (user-launched server per
`smoke_test_server_handoff.md`):

| Smoke step | Pass condition |
|---|---|
| Drop mindrune (id=558) on player tile | item appears at player coord |
| Right-click → Take | no `"no handler for OBJ_TYPE"` server log |
| Same-tile pickup | no `"I can't reach that!"` client chat |
| Inventory check | mindrune in inventory; ground item gone |
| Off-tile table pickup (one tile away) | identical pass shape |

Adjacent surprises route per `smoke_surfaces_adjacent_divergences.md`
— ≤30 LOC stretch-in, larger to NAI-153.

## 7. Cadence

Per `runescript_cadence.md` mid-band, single-bundle, three T-tasks.

- T1 (script handler) — smallest, independent package.
- T2 (Player reach) — smoke-gate carrier.
- T3 (Npc reach parity) — TS-symmetry close on the deviation tag's Obj clause.

**Workflow:** plan → subagent-driven impl per
`execution_mode_default.md` → reviewer subagent on Sonnet per
`superpowers_code_reviewer_model.md` → `/clear` between plan and impl
per `superpowers_clear_between_spec_and_impl.md` → smoke handoff →
close commit with `Closes memory:` trailer per
`close_commit_memory_trailer.md`.

**Total LOC estimate:** ~65 production + ~130 test = ~195 LOC delta.

## 8. TS-fidelity deviations

- **NAI-152-D2 (T1) [NEW]:** OBJ_TYPE handler skips TS's
  `ObjTypeValid` re-check. **Why:** goscape pre-validates at wire
  handler (`handler_opobj.go:62-70`) before constructing the obj; the
  active obj's type id is round-trip-clean by construction. **Risk:**
  none — equivalent to TS post-validation. Doc-comment labels per
  `defensive_gate_doc_comment_label.md`.
- **NAI-91-D-OPERABLE-CHEB-FALLBACK (T2, T3) [RETIRED-PARTIAL]:** Obj
  clause of the deviation retires; PathingEntity (Player/Npc) clause
  persists pending entity-shape port. Both call sites' doc-comments
  trim "and Obj"; the deviation tag itself stays alive for the
  remaining PathingEntity scope. Per `retire_deviation_grep_all_comments.md`,
  plan-author runs `rg "NAI-91-D-OPERABLE-CHEB-FALLBACK" pkg/ modules/`
  to enumerate every comment-site and applies the trim consistently.
- **NAI-152-D2a (T3) [NEW]:** Npc.inOperableDistance Obj branch ports
  `reachedObj` only (no OR-chain), diverging from Player's branch.
  **Why:** TS PathingEntity.ts:389 (base, Npc inherits) is single-call;
  Player.ts:1110 overrides with OR. Asymmetry tracks TS exactly. **Risk:**
  none — symmetric port from per-class TS source.
- **NAI-152-D-X:** open new entry at impl time per `true_to_ts_gate.md`
  for any surfaced divergence before close.

## 9. Risk register

- **R1 (low):** `*entity.Obj.Width == 1 && Length == 1` invariant
  assumed throughout T2/T3. `NewObj` always sets 1×1 (`pkg/entity/obj.go:39`).
  **Mitigation:** plan-author asserts the invariant in T2/T3 doc-comments;
  no production code branches on the values.
- **R2 (low-med):** existing `_test.go` fixtures may have pinned the
  "same-tile rejects" Cheb semantic for Obj targets. **Mitigation:**
  plan-author pre-flight greps both `interaction_test.go` and
  `npc_interaction_test.go` for `*entity.Obj` + `inOperableDistance`
  fixtures and re-pins or retires per
  `enumerate_all_sites.md` + `latent_bug_at_migration_boundary.md`.
- **R3 (low):** B1 closed with mindrune `Op[2]="Take"` validated;
  regression in B1's `applyPostDecodeFixups` would re-block the wire
  handler at `handler_opobj.go:71`. **Mitigation:** controller preflight
  re-greps the gate per `controller_preflight.md` before dispatching
  T2/T3 impl.
- **R4 (low):** Same-tile correctness depends entirely on the
  `reachedObj` (-1) arm — `reachedEntity` (-2) returns `false` on
  1×1 same-tile (Collides=true → !collides=false in
  ReachExclusiveRectangle). If `reach.Reached(..., locShape=-1, ...)`
  short-circuit at `strategy.go:37` regresses, every same-tile pickup
  breaks. **Mitigation:** T2's `TestInOperableDistance_Obj_SameTile`
  pins the end-to-end behavior; the short-circuit is well-covered by
  existing routefinder tests but the new test gives bundle-local
  evidence.
- **R5 (low):** content's `pickup.rs2` is the canonical caller. If
  LostCity registers it under a header goscape's script-loader doesn't
  resolve, T1 won't surface the OBJ_TYPE gap as the binding error.
  **Mitigation:** B1 smoke already confirmed `[label,pickup_obj_table]`
  enters and crashes at OBJ_TYPE — script-loader path is wired. No
  action.

## 10. Acceptance gate

B2 closes only after the Java-client smoke pass shape in §6:
mindrune in inventory, ground item gone, no `"can't reach"` chat, no
`"no handler for OBJ_TYPE"` log. Anything beyond is routed to NAI-152
stretch (≤30 LOC) or NAI-153 (larger).

Closes deviation: NAI-91-D-OPERABLE-CHEB-FALLBACK (Obj clause).
Activates NAI-152 master spec §6.3 B2-β (with the reach-check addition
not predicted pre-B1).
