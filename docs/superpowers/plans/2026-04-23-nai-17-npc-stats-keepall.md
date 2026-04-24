# NAI-17 Implementation Plan — NPC Stats Array + `NPC_CHANGETYPE_KEEPALL`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the full TS `Npc.levels`/`Npc.baseLevels` 6-stat arrays onto `*Npc`, atomically migrate the S6d `curHP`/`baseHP` scalars into those arrays, refactor `ChangeType` with the TS stats-reset formula (boost/drain preservation), add `ChangeTypeKeepAll` + opcode 2506 (`NPC_CHANGETYPE_KEEPALL`) dispatch, wire `revertType` to branch on the new `resetOnRevert` field, and expand the regen loop to iterate all 6 stats.

**Architecture:** Linear TDD per task — write failing tests, verify failure, implement minimal production code, verify pass, commit. Task 1 is atomic: data-model migration (arrays added + scalars deleted + call-sites rewritten) lands in one commit. Task 2 refactors `ChangeType` into a shared private `changeTypeImpl`. Task 3 exposes the `reset=false` path as `ChangeTypeKeepAll` and wires the opcode. Task 4 branches `revertType`. Task 5 expands regen. Task 6 is the close commit (memory update + full-suite verify).

**Tech Stack:** Go 1.26+. No new packages. Existing `pkg/objtype` (adds `NpcStatCount` constant), `pkg/script` (`ActiveNpc` interface, `handlers_npc.go`, `handlers.go`, `opcode.go`), `modules/world` (`npc.go`, `npc_masks.go`, `npc_script.go`, `npc_source.go`, and their `*_test.go` companions).

**Spec:** `docs/superpowers/specs/2026-04-23-nai-17-npc-stats-keepall-design.md`

**Go command prefix:** All `go` invocations use `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` per the project's `CLAUDE.md`.

**Commit style:** All commits use `--no-gpg-sign` per the user's global `CLAUDE.md`. Every commit message ends with:

```
Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

---

## Task 1 — Data model: `levels`/`baseLevels` arrays + full HP migration

**Files:**
- Modify: `pkg/objtype/npctype.go` (add `NpcStatCount` constant after line 21)
- Modify: `modules/world/npc.go` (struct fields at 101-105, NewNpc at 108-163, delete `initialHP` helper at 165-177, revertType HP reseed at 271-272)
- Modify: `modules/world/npc_source.go` (public HP accessors at 20-21)
- Modify: `modules/world/npc_script.go` (`NpcStat`/`NpcBaseStat` at 35-51; NOT regen yet — regen gets expanded in Task 5)
- Modify: `modules/world/npc_masks.go` (`Damage` at 92-103, `ResetHP` at 140-141, `ResetMasks` doc comment at 106)
- Modify: `modules/world/npc_event_queue_test.go` (rewrite `n.curHP, n.baseHP = X, Y` assignments at lines 174, 193, 207-208, 220-221, 232, 238-239, 250, 256-257)
- Test: `modules/world/npc_test.go` (add 4 new test functions)
- Test: `pkg/script/handlers_npc_test.go` (add 2 out-of-range mock-driven tests)

**Rationale:** Migrate storage in one atomic commit so the tree never has both `curHP` and `levels[HITPOINTS]` as active fields. Ports TS `Npc.ts:50-51` (array decls), `Npc.ts:72` (`resetOnRevert` default), and `Npc.ts:90-94` (ctor seeding loop). Deletes `initialHP` helper (zero consumers post-migration per `dead_api_polish.md`). Regen stays HP-only-through-array-indexing this task; 6-stat expansion is Task 5 so its test surface stays isolated.

### Steps

- [ ] **Step 1.1 — Add `NpcStatCount` constant**

Edit `pkg/objtype/npctype.go`. Insert `NpcStatCount = 6` as the last line in the existing const block containing `NpcStatAttack..NpcStatMagic` (around line 21):

```go
const (
    NpcStatAttack    = 0
    NpcStatDefence   = 1
    NpcStatStrength  = 2
    NpcStatHitpoints = 3
    NpcStatRanged    = 4
    NpcStatMagic     = 5
    NpcStatCount     = 6 // Total number of stat slots; matches TS NpcStat enum.
)
```

- [ ] **Step 1.2 — Write the failing data-model tests**

Append to `modules/world/npc_test.go` (end of file). All 4 tests exercise the new fields BEFORE they exist, so they'll compile-fail and then fail-to-pass once the fields land.

```go
// TestNewNpcSeedsStatsFromType verifies that NewNpc seeds both
// n.levels[] and n.baseLevels[] from typ.Stats for all 6 slots.
// Mirrors TS Npc.ts:90-94 ctor loop.
func TestNewNpcSeedsStatsFromType(t *testing.T) {
    typ := &objtype.NpcType{
        Stats: []uint16{7, 11, 13, 17, 19, 23}, // distinct per slot
    }
    n := NewNpc(1, 42, 100, 100, 0, typ)

    want := []int{7, 11, 13, 17, 19, 23}
    for i := 0; i < objtype.NpcStatCount; i++ {
        if got := n.NpcStat(i); got != want[i] {
            t.Errorf("NpcStat(%d): got %d, want %d", i, got, want[i])
        }
        if got := n.NpcBaseStat(i); got != want[i] {
            t.Errorf("NpcBaseStat(%d): got %d, want %d", i, got, want[i])
        }
    }
    if !n.resetOnRevert {
        t.Errorf("resetOnRevert: got false, want true (default)")
    }
}

// TestNewNpcWithNilStatsStaysZero verifies that a zero-length Stats
// slice leaves both arrays zero-valued (no out-of-bounds panic).
func TestNewNpcWithNilStatsStaysZero(t *testing.T) {
    typ := &objtype.NpcType{Stats: nil}
    n := NewNpc(1, 42, 100, 100, 0, typ)

    for i := 0; i < objtype.NpcStatCount; i++ {
        if got := n.NpcStat(i); got != 0 {
            t.Errorf("NpcStat(%d): got %d, want 0", i, got)
        }
        if got := n.NpcBaseStat(i); got != 0 {
            t.Errorf("NpcBaseStat(%d): got %d, want 0", i, got)
        }
    }
    if got := n.CurHP(); got != 0 {
        t.Errorf("CurHP: got %d, want 0 (nil Stats)", got)
    }
    if got := n.BaseHP(); got != 0 {
        t.Errorf("BaseHP: got %d, want 0 (nil Stats)", got)
    }
}

// TestNpcStatAllSlots verifies NpcStat reads from n.levels for all 6
// slots after direct array writes.
func TestNpcStatAllSlots(t *testing.T) {
    n := newNpcForLifecycleTest(t) // existing fixture
    for i := 0; i < objtype.NpcStatCount; i++ {
        n.levels[i] = 100 + i
    }
    for i := 0; i < objtype.NpcStatCount; i++ {
        if got, want := n.NpcStat(i), 100+i; got != want {
            t.Errorf("NpcStat(%d): got %d, want %d", i, got, want)
        }
    }
}

// TestNpcBaseStatAllSlots verifies NpcBaseStat reads from
// n.baseLevels for all 6 slots after direct array writes.
func TestNpcBaseStatAllSlots(t *testing.T) {
    n := newNpcForLifecycleTest(t)
    for i := 0; i < objtype.NpcStatCount; i++ {
        n.baseLevels[i] = 200 + i
    }
    for i := 0; i < objtype.NpcStatCount; i++ {
        if got, want := n.NpcBaseStat(i), 200+i; got != want {
            t.Errorf("NpcBaseStat(%d): got %d, want %d", i, got, want)
        }
    }
}
```

Append to `pkg/script/handlers_npc_test.go` (end of file, inside the existing package):

```go
// TestNpcStatOutOfRange verifies defensive bounds checking on
// NpcStat's stat parameter (Go-side; NAI-17-D2 deviation).
func TestNpcStatOutOfRange(t *testing.T) {
    npc := newMockNpc()
    for i := 0; i < 6; i++ {
        npc.stats[i] = 99 // populate so in-range would return non-zero
    }
    cases := []struct {
        name string
        id   int
    }{
        {"negative", -1},
        {"at-count", 6},
        {"way-beyond", 100},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            state := runNpcOp(t, npc, nil, OpNpcStat, []int{tc.id})
            if got := state.PopInt(); got != 0 {
                t.Errorf("NpcStat(%d): got %d, want 0 (out of range)", tc.id, got)
            }
        })
    }
}

// TestNpcBaseStatOutOfRange mirrors TestNpcStatOutOfRange for NpcBaseStat.
func TestNpcBaseStatOutOfRange(t *testing.T) {
    npc := newMockNpc()
    cases := []struct {
        name string
        id   int
    }{
        {"negative", -1},
        {"at-count", 6},
        {"way-beyond", 100},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            state := runNpcOp(t, npc, nil, OpNpcBaseStat, []int{tc.id})
            if got := state.PopInt(); got != 0 {
                t.Errorf("NpcBaseStat(%d): got %d, want 0 (out of range)", tc.id, got)
            }
        })
    }
}
```

- [ ] **Step 1.3 — Run tests to verify they fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/... -run 'TestNewNpcSeedsStatsFromType|TestNewNpcWithNilStatsStaysZero|TestNpcStatAllSlots|TestNpcBaseStatAllSlots|TestNpcStatOutOfRange|TestNpcBaseStatOutOfRange' -count=1
```

Expected: FAIL with compile errors (`n.levels` / `n.baseLevels` / `n.resetOnRevert` undefined on `*Npc`). That confirms the tests are targeting the right symbols.

- [ ] **Step 1.4 — Add struct fields to `*Npc`**

Edit `modules/world/npc.go` around lines 101-105. Replace:

```go
    curHP, baseHP                             int
    spotanimID, spotanimHeight, spotanimDelay int
    faceSquareX, faceSquareZ                  int
    changeTypeID                              int
```

With:

```go
    levels        [objtype.NpcStatCount]int // NAI-17: current (boosted) stat values
    baseLevels    [objtype.NpcStatCount]int // NAI-17: base values (regen convergence target)
    resetOnRevert bool                       // NAI-17: TS Npc.ts:72; CHANGETYPE→true, KEEPALL→false
    spotanimID, spotanimHeight, spotanimDelay int
    faceSquareX, faceSquareZ                  int
    changeTypeID                              int
```

(`curHP, baseHP int` line deleted; three new fields inserted in its place.)

- [ ] **Step 1.5 — Update `NewNpc`: remove `initialHP` init entries + add post-literal seeding loop**

Edit `modules/world/npc.go:108-163`. In the struct literal at lines 151-152, DELETE these two entries:

```go
        curHP:           initialHP(typ),
        baseHP:          initialHP(typ),
```

After the `n.targetOp = n.defaultMode()` line (currently line 161), INSERT before `return n`:

```go
    // NAI-17: seed levels[]/baseLevels[] from typ.Stats (mirrors TS Npc.ts:90-94).
    if typ != nil {
        for i := 0; i < objtype.NpcStatCount && i < len(typ.Stats); i++ {
            v := int(typ.Stats[i])
            n.levels[i] = v
            n.baseLevels[i] = v
        }
    }
    n.resetOnRevert = true
```

- [ ] **Step 1.6 — Delete `initialHP` helper**

Edit `modules/world/npc.go`. Delete lines 165-177 (the `initialHP` function and its doc comment — the whole block from `// initialHP returns the max HP...` through `}`). Post-migration this helper has zero consumers.

- [ ] **Step 1.7 — Migrate public HP accessors**

Edit `modules/world/npc_source.go:20-21`. Replace:

```go
func (n *Npc) CurHP() int          { return n.curHP }
func (n *Npc) BaseHP() int         { return n.baseHP }
```

With:

```go
func (n *Npc) CurHP() int  { return n.levels[objtype.NpcStatHitpoints] }
func (n *Npc) BaseHP() int { return n.baseLevels[objtype.NpcStatHitpoints] }
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to the file's imports if not already present.

- [ ] **Step 1.8 — Migrate `NpcStat`/`NpcBaseStat`**

Edit `modules/world/npc_script.go:35-51`. Replace the two existing functions with:

```go
// NpcStat returns the current (boosted) stat level for the given stat id.
// Reads n.levels[stat] — seeded from typ.Stats at NewNpc time and maintained
// by ChangeType / Damage / processNpcRegen.
func (n *Npc) NpcStat(stat int) int {
    if stat < 0 || stat >= objtype.NpcStatCount {
        return 0
    }
    return n.levels[stat]
}

// NpcBaseStat returns the base stat level for the given stat id.
func (n *Npc) NpcBaseStat(stat int) int {
    if stat < 0 || stat >= objtype.NpcStatCount {
        return 0
    }
    return n.baseLevels[stat]
}
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to imports if not already present.

- [ ] **Step 1.9 — Migrate `Damage`**

Edit `modules/world/npc_masks.go:92-103`. Replace the `Damage` body with:

```go
func (n *Npc) Damage(amount, dmgType int) {
    if amount < 0 {
        amount = 0
    }
    cur := n.levels[objtype.NpcStatHitpoints]
    n.damageAmt = min(amount, cur)
    n.damageType = dmgType
    cur -= amount
    if cur < 0 {
        cur = 0
    }
    n.levels[objtype.NpcStatHitpoints] = cur
    n.masks |= rsbuf.NpcMaskDamage
}
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to imports if not already present.

- [ ] **Step 1.10 — Migrate `ResetHP`**

Edit `modules/world/npc_masks.go:138-141` (approximate — locate `func (n *Npc) ResetHP()`). Replace:

```go
func (n *Npc) ResetHP() {
    hp := int(n.typ.Stats[objtype.NpcStatHitpoints])
    n.levels[objtype.NpcStatHitpoints] = hp
    n.baseLevels[objtype.NpcStatHitpoints] = hp
}
```

- [ ] **Step 1.11 — Migrate regen (HP-only through array indexing for now)**

Edit `modules/world/npc_script.go:237-240` (approximate — inside `processNpcRegen`). Replace:

```go
    case n.curHP < n.baseHP:
        n.curHP++
    case n.curHP > n.baseHP:
        n.curHP--
```

With:

```go
    case n.levels[objtype.NpcStatHitpoints] < n.baseLevels[objtype.NpcStatHitpoints]:
        n.levels[objtype.NpcStatHitpoints]++
    case n.levels[objtype.NpcStatHitpoints] > n.baseLevels[objtype.NpcStatHitpoints]:
        n.levels[objtype.NpcStatHitpoints]--
```

This preserves single-slot HP regen semantics temporarily; Task 5 converts it into the 6-stat loop.

- [ ] **Step 1.12 — Migrate `revertType` HP reseed**

Edit `modules/world/npc.go:271-272`. In the existing `revertType` body, replace:

```go
    n.curHP = initialHP(n.typ)
    n.baseHP = initialHP(n.typ)
```

With:

```go
    // NAI-17: reseed HP slot from typ.Stats (temporary single-slot form;
    // Task 4 expands this to a 6-slot loop and adds the resetOnRevert
    // light-path branching).
    if n.typ != nil && len(n.typ.Stats) > objtype.NpcStatHitpoints {
        hp := int(n.typ.Stats[objtype.NpcStatHitpoints])
        n.levels[objtype.NpcStatHitpoints] = hp
        n.baseLevels[objtype.NpcStatHitpoints] = hp
    }
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to the file's imports if not already present.

- [ ] **Step 1.13 — Update `ResetMasks` doc comment**

Edit `modules/world/npc_masks.go:105-107` (approximate). Replace the phrase `curHP, baseHP` in the comment with `levels[NpcStatHitpoints], baseLevels[NpcStatHitpoints]`. Full replacement:

```go
// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (animID, faceSquareX/Z, changeTypeID, and the levels[]/baseLevels[]
// arrays) are retained across ticks — S6d promoted HP to persistent, NAI-17
// extended that to all 6 stats via the array migration.
```

- [ ] **Step 1.14 — Migrate test assignments in `npc_event_queue_test.go`**

Edit `modules/world/npc_event_queue_test.go`. Search-and-replace the 8 sites:

- Every `n.curHP, n.baseHP = X, Y` becomes `n.levels[objtype.NpcStatHitpoints], n.baseLevels[objtype.NpcStatHitpoints] = X, Y`.
- Every `n.curHP` read (e.g., in `if n.curHP != X`) becomes `n.CurHP()`.
- Every `n.baseHP` read becomes `n.BaseHP()`.

The specific line ranges (pre-migration): 174, 181, 193, 207-208, 220-221, 232, 238-239, 250, 256-257. After migration the exact line numbers may drift — find every remaining `curHP` / `baseHP` identifier in that file and convert.

Add `"github.com/zsrv/goscape/pkg/objtype"` to that file's imports if not already present.

- [ ] **Step 1.15 — Run the failing tests to verify they now pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/... -run 'TestNewNpcSeedsStatsFromType|TestNewNpcWithNilStatsStaysZero|TestNpcStatAllSlots|TestNpcBaseStatAllSlots|TestNpcStatOutOfRange|TestNpcBaseStatOutOfRange' -count=1
```

Expected: PASS.

- [ ] **Step 1.16 — Run the full test suite to verify no regressions**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS across all packages. Existing NAI-16 ChangeType tests, NAI-5 revertType tests, and S6d HP-persistence tests all green.

- [ ] **Step 1.17 — Commit**

```bash
git -C /home/owner/Code/github.com/zsrv/goscape add pkg/objtype/npctype.go modules/world/npc.go modules/world/npc_source.go modules/world/npc_script.go modules/world/npc_masks.go modules/world/npc_test.go modules/world/npc_event_queue_test.go pkg/script/handlers_npc_test.go

git -C /home/owner/Code/github.com/zsrv/goscape commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-17 Task 1 migrate HP scalars to 6-stat levels/baseLevels arrays

Adds objtype.NpcStatCount=6, replaces *Npc.curHP/baseHP with
levels[NpcStatCount]int/baseLevels[NpcStatCount]int + resetOnRevert bool.
NewNpc seeds both arrays from typ.Stats per TS Npc.ts:90-94; initialHP
helper deleted (zero post-migration consumers). All HP call sites
(CurHP/BaseHP accessors, NpcStat/NpcBaseStat getters, Damage, ResetHP,
regen-HP, revertType reseed, test assignments) migrate to array-slot form.

Regen stays HP-only through array indexing this task; 6-stat loop
expansion is Task 5. ChangeType body unchanged this task; refactor into
changeTypeImpl + stats-reset is Task 2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — `ChangeType` refactor + stats-reset formula

**Files:**
- Modify: `pkg/script/active.go` (ActiveNpc `ChangeType` doc comment at lines 335-344 — drop DEFERRED stats-reset language; KEEPALL block deletion waits for Task 3)
- Modify: `modules/world/npc_masks.go` (replace `ChangeType` body at lines 45-58 with delegate; add `changeTypeImpl` + `lookupType` + `resetStatsForType` private helpers; drop DEFERRED comment block at lines 37-44)
- Test: `modules/world/npc_test.go` (add 2 new test functions)

**Rationale:** Ports the TS `Npc.ts:436-443` stats-reset branch behind a shared private `changeTypeImpl`. Public `ChangeType` becomes a one-line delegate with `reset=true`. `ChangeTypeKeepAll` public method is NOT added this task — Task 3 ships it. Isolating the refactor from the interface-surface change keeps Task 2's test surface entirely around the stats-reset formula.

### Steps

- [ ] **Step 2.1 — Write the failing stats-reset tests**

Append to `modules/world/npc_test.go`:

```go
// TestChangeTypeResetsStatsWithBoostPreservation verifies the TS
// Npc.ts:436-443 boost/drain-preserving formula:
//   levels[i] = max(newBase - (baseLevels[i] - levels[i]), 0)
//   baseLevels[i] = newBase
// When the pre-morph NPC has stat boosts/drains, the morph preserves
// the SAME delta against the new type's base.
func TestChangeTypeResetsStatsWithBoostPreservation(t *testing.T) {
    s := newServerForScriptTest(t)
    baseTyp := &objtype.NpcType{Stats: []uint16{10, 10, 10, 10, 10, 10}}
    newTyp := &objtype.NpcType{Stats: []uint16{20, 15, 25, 20, 12, 30}}
    s.npcTypes = &objtype.NpcTypeCache{Configs: []*objtype.NpcType{baseTyp, newTyp}}

    n := NewNpc(1, 0, 100, 100, 0, baseTyp)
    n.server = s
    // Seed deltas: ATK drain=2 (levels=8), DEF boost=2 (levels=12),
    // STR level (no delta), HP drain=5 (levels=5), RNG boost=3 (levels=13),
    // MAG (no delta).
    n.levels[objtype.NpcStatAttack]    = 8
    n.levels[objtype.NpcStatDefence]   = 12
    n.levels[objtype.NpcStatStrength]  = 10
    n.levels[objtype.NpcStatHitpoints] = 5
    n.levels[objtype.NpcStatRanged]    = 13
    n.levels[objtype.NpcStatMagic]     = 10

    n.ChangeType(1, 100) // morph to newTyp

    // Expected: newBase − drain  (drain positive = drained; negative = boosted)
    //   ATK: 20 − 2 = 18
    //   DEF: 15 − (−2) = 17
    //   STR: 25 − 0 = 25
    //   HP:  20 − 5 = 15
    //   RNG: 12 − (−3) = 15
    //   MAG: 30 − 0 = 30
    wantLevels := []int{18, 17, 25, 15, 15, 30}
    wantBase   := []int{20, 15, 25, 20, 12, 30}
    for i := 0; i < objtype.NpcStatCount; i++ {
        if n.levels[i] != wantLevels[i] {
            t.Errorf("levels[%d]: got %d, want %d", i, n.levels[i], wantLevels[i])
        }
        if n.baseLevels[i] != wantBase[i] {
            t.Errorf("baseLevels[%d]: got %d, want %d", i, n.baseLevels[i], wantBase[i])
        }
    }
    if !n.resetOnRevert {
        t.Errorf("resetOnRevert: got false, want true (ChangeType default)")
    }
}

// TestChangeTypeResetsStatsClampedAtZero verifies that an oversize drain
// against a smaller new base clamps to zero via TS's Math.max(..., 0).
func TestChangeTypeResetsStatsClampedAtZero(t *testing.T) {
    s := newServerForScriptTest(t)
    baseTyp := &objtype.NpcType{Stats: []uint16{100, 10, 10, 10, 10, 10}}
    newTyp  := &objtype.NpcType{Stats: []uint16{5, 10, 10, 10, 10, 10}}
    s.npcTypes = &objtype.NpcTypeCache{Configs: []*objtype.NpcType{baseTyp, newTyp}}

    n := NewNpc(1, 0, 100, 100, 0, baseTyp)
    n.server = s
    // ATK drain=90 (base=100, level=10). New base=5. 5 − 90 = −85 → clamp 0.
    n.levels[objtype.NpcStatAttack] = 10

    n.ChangeType(1, 100)

    if got := n.levels[objtype.NpcStatAttack]; got != 0 {
        t.Errorf("levels[ATK]: got %d, want 0 (clamped from -85)", got)
    }
    if got := n.baseLevels[objtype.NpcStatAttack]; got != 5 {
        t.Errorf("baseLevels[ATK]: got %d, want 5", got)
    }
}
```

- [ ] **Step 2.2 — Run tests to verify they fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestChangeTypeResetsStats' -count=1
```

Expected: FAIL — current `ChangeType` body does not reset stats, so `n.levels[]` and `n.baseLevels[]` still carry their pre-morph values.

- [ ] **Step 2.3 — Refactor `ChangeType` into `changeTypeImpl` + add helpers**

Edit `modules/world/npc_masks.go`. Replace the entire block from the `// ChangeType morphs ...` doc comment block (around line 16) through the closing `}` of the existing `ChangeType` (around line 58) with:

```go
// ChangeType morphs the NPC to newType and schedules a revert to
// baseType after `duration` ticks. Resets all 6 stats onto the new
// type's base values using a boost/drain-preserving formula. Mirrors
// TS Npc.changeType at Engine-TS/.../Npc.ts:427-449 with reset=true.
// No-op when duration < 1 OR when the NPC is dead.
func (n *Npc) ChangeType(newType, duration int) {
    n.changeTypeImpl(newType, duration, true)
}

// changeTypeImpl is the shared body behind ChangeType and the
// Task 3 ChangeTypeKeepAll. Mirrors TS Npc.changeType at
// Engine-TS/.../Npc.ts:427-449.
//
// Semantics:
//   - No-op when duration < 1 (TS guard; rejects 0 and negatives) OR
//     when the NPC is dead (TS `!this.isActive`).
//   - On success: writes typeId, changeTypeID, CHANGE_TYPE mask,
//     recomputes uid, writes resetOnRevert=reset.
//   - If reset: runs the TS:436-443 stats-reset loop against the new
//     type's stats (lookupType returns nil when the server/registry
//     is unavailable, in which case the reset silently skips — same
//     tolerance revertType already exhibits).
//   - Fast-path TS:444-445: if newType==baseType && lifecycle==RESPAWN,
//     lifecycleTick=-1 (suppresses Events-block revert). Otherwise
//     lifecycleTick=duration.
func (n *Npc) changeTypeImpl(newType, duration int, reset bool) {
    if duration < 1 || n.dead {
        return
    }
    n.typeId = newType
    n.changeTypeID = newType
    n.masks |= rsbuf.NpcMaskChangeType
    n.uid = (newType << 16) | n.nid
    n.resetOnRevert = reset

    if reset {
        if newTyp := n.lookupType(newType); newTyp != nil {
            n.resetStatsForType(newTyp)
        }
    }

    if newType == n.baseType && n.lifecycle == NpcLifecycleRespawn {
        n.lifecycleTick = -1
    } else {
        n.lifecycleTick = duration
    }
}

// lookupType returns the NpcType config for typeId, or nil if server
// or registry is unavailable or typeId is out of bounds. Mirrors the
// guard shape revertType already uses (npc.go pre-NAI-17 lines 265-268).
func (n *Npc) lookupType(typeId int) *objtype.NpcType {
    if n.server == nil || n.server.npcTypes == nil {
        return nil
    }
    if typeId < 0 || typeId >= len(n.server.npcTypes.Configs) {
        return nil
    }
    return n.server.npcTypes.Configs[typeId]
}

// resetStatsForType applies the TS Npc.ts:436-443 boost/drain-preserving
// stats reset against newTyp's Stats. For each slot i:
//   drain := baseLevels[i] - levels[i]     // positive: drained; negative: boosted
//   levels[i] = max(newBase - drain, 0)
//   baseLevels[i] = newBase
// Iterates over min(NpcStatCount, len(newTyp.Stats)) slots.
func (n *Npc) resetStatsForType(newTyp *objtype.NpcType) {
    for i := 0; i < objtype.NpcStatCount && i < len(newTyp.Stats); i++ {
        newBase := int(newTyp.Stats[i])
        drain := n.baseLevels[i] - n.levels[i]
        v := newBase - drain
        if v < 0 {
            v = 0
        }
        n.levels[i] = v
        n.baseLevels[i] = newBase
    }
}
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to the file's imports if not already present.

- [ ] **Step 2.4 — Update `ActiveNpc.ChangeType` doc comment**

Edit `pkg/script/active.go` around lines 335-344. Keep the KEEPALL/DEFERRED comment block in place for now (Task 3 deletes it). Replace only the `ChangeType` doc comment with:

```go
// ChangeType morphs the NPC to newType and schedules a revert to
// baseType after `duration` ticks. Resets all 6 stats onto the new
// type's base values using a boost/drain-preserving formula. Mirrors
// TS Npc.changeType at Engine-TS/.../Npc.ts:427-449 with reset=true.
// No-op when duration < 1 OR when the NPC is dead.
ChangeType(newType, duration int)
```

(The "DEFERRED: stats-reset branch (TS:436-443)..." phrasing is gone. The "DEFERRED: NPC_CHANGETYPE_KEEPALL..." block at lines 340-343 of the pre-NAI-17 file stays — Task 3 deletes it.)

- [ ] **Step 2.5 — Run the failing tests to verify they now pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestChangeTypeResetsStats' -count=1
```

Expected: PASS.

- [ ] **Step 2.6 — Run the full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS. In particular, existing NAI-16 tests at `npc_test.go:43-136` (`TestNpcChangeType*`) must still pass — they assert typeId/uid/mask/lifecycleTick only; stats-reset firing does not affect their assertions.

- [ ] **Step 2.7 — Commit**

```bash
git -C /home/owner/Code/github.com/zsrv/goscape add pkg/script/active.go modules/world/npc_masks.go modules/world/npc_test.go

git -C /home/owner/Code/github.com/zsrv/goscape commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-17 Task 2 ChangeType stats-reset formula (TS:436-443)

Refactors *Npc.ChangeType into changeTypeImpl(newType, duration, reset)
behind a shared private helper. Ports the TS boost/drain-preserving
stats-reset formula: levels[i] = max(newBase - (baseLevels[i] -
levels[i]), 0); baseLevels[i] = newBase. Adds lookupType + resetStatsForType
helpers. ChangeTypeKeepAll public surface + opcode 2506 dispatch land
in Task 3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — `ChangeTypeKeepAll` + `NPC_CHANGETYPE_KEEPALL` dispatch

**Files:**
- Modify: `pkg/script/active.go` (add `ChangeTypeKeepAll` to `ActiveNpc` interface; delete the `DEFERRED: NPC_CHANGETYPE_KEEPALL` block at lines 340-343)
- Modify: `modules/world/npc_masks.go` (add `ChangeTypeKeepAll` public method delegating to `changeTypeImpl(newType, duration, false)`)
- Modify: `pkg/script/handlers_npc.go` (delete DEFERRED block at lines 176-178; add `handleNpcChangeTypeKeepAll` after `handleNpcChangeType`)
- Modify: `pkg/script/handlers.go` (add `OpNpcChangeTypeKeepAll: handleNpcChangeTypeKeepAll` entry to the dispatch table)
- Modify: `pkg/script/handlers_npc_test.go` (extend `mockNpc` with `ChangeTypeKeepAll` stub; extend the no-active-npc test table at lines 352-353; add `TestHandleNpcChangeTypeKeepAllDispatch` and `TestHandleNpcChangeTypeKeepAllNoActiveNpc`)
- Modify: `pkg/script/handlers_player_test.go` (extend `mockActiveNpc` with `ChangeTypeKeepAll` stub at line 22-23 area)
- Test: `modules/world/npc_test.go` (add 3 new test functions)

**Rationale:** Exposes the `reset=false` code path added in Task 2 as a public interface method and wires it to the reserved opcode 2506. Deletes both DEFERRED comment breadcrumbs (active.go + handlers_npc.go) so the audit grep is clean.

### Steps

- [ ] **Step 3.1 — Write the failing tests (impl + dispatch)**

Append to `modules/world/npc_test.go`:

```go
// TestChangeTypeKeepAllPreservesStats verifies that ChangeTypeKeepAll
// morphs typeId/uid/mask but leaves levels[]/baseLevels[] unchanged
// and writes resetOnRevert=false.
func TestChangeTypeKeepAllPreservesStats(t *testing.T) {
    s := newServerForScriptTest(t)
    baseTyp := &objtype.NpcType{Stats: []uint16{10, 10, 10, 10, 10, 10}}
    newTyp  := &objtype.NpcType{Stats: []uint16{99, 99, 99, 99, 99, 99}}
    s.npcTypes = &objtype.NpcTypeCache{Configs: []*objtype.NpcType{baseTyp, newTyp}}

    n := NewNpc(1, 0, 100, 100, 0, baseTyp)
    n.server = s
    // Seed some deltas.
    n.levels[objtype.NpcStatAttack]    = 5
    n.levels[objtype.NpcStatHitpoints] = 5
    n.levels[objtype.NpcStatDefence]   = 15 // boosted

    n.ChangeTypeKeepAll(1, 100)

    // levels and baseLevels UNCHANGED.
    wantLevels := []int{5, 15, 10, 5, 10, 10}
    wantBase   := []int{10, 10, 10, 10, 10, 10}
    for i := 0; i < objtype.NpcStatCount; i++ {
        if n.levels[i] != wantLevels[i] {
            t.Errorf("levels[%d]: got %d, want %d (KEEPALL preserves)", i, n.levels[i], wantLevels[i])
        }
        if n.baseLevels[i] != wantBase[i] {
            t.Errorf("baseLevels[%d]: got %d, want %d (KEEPALL preserves)", i, n.baseLevels[i], wantBase[i])
        }
    }
    // Morph state applied.
    if n.typeId != 1 {
        t.Errorf("typeId: got %d, want 1", n.typeId)
    }
    if n.uid != (1<<16)|n.nid {
        t.Errorf("uid: got %d, want %d", n.uid, (1<<16)|n.nid)
    }
    if n.masks&rsbuf.NpcMaskChangeType == 0 {
        t.Errorf("mask: CHANGE_TYPE bit not set")
    }
    if n.resetOnRevert {
        t.Errorf("resetOnRevert: got true, want false (KEEPALL)")
    }
    if n.lifecycleTick != 100 {
        t.Errorf("lifecycleTick: got %d, want 100", n.lifecycleTick)
    }
}

// TestChangeTypeKeepAllDurationZeroNoOp verifies duration<1 guard.
func TestChangeTypeKeepAllDurationZeroNoOp(t *testing.T) {
    n := newNpcForLifecycleTest(t)
    n.levels[objtype.NpcStatHitpoints] = 5
    origTypeId := n.typeId
    origResetOnRevert := n.resetOnRevert

    n.ChangeTypeKeepAll(42, 0)

    if n.typeId != origTypeId {
        t.Errorf("typeId: got %d, want %d (duration=0 no-op)", n.typeId, origTypeId)
    }
    if n.resetOnRevert != origResetOnRevert {
        t.Errorf("resetOnRevert: got %v, want %v (duration=0 no-op)",
            n.resetOnRevert, origResetOnRevert)
    }
}

// TestChangeTypeKeepAllDeadNoOp verifies dead-NPC guard.
func TestChangeTypeKeepAllDeadNoOp(t *testing.T) {
    n := newNpcForLifecycleTest(t)
    n.dead = true
    origTypeId := n.typeId

    n.ChangeTypeKeepAll(42, 100)

    if n.typeId != origTypeId {
        t.Errorf("typeId: got %d, want %d (dead NPC no-op)", n.typeId, origTypeId)
    }
}
```

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestHandleNpcChangeTypeKeepAllDispatch verifies that opcode 2506
// pops (newType, duration) in TS order and calls ChangeTypeKeepAll.
func TestHandleNpcChangeTypeKeepAllDispatch(t *testing.T) {
    npc := newMockNpc()
    state := runNpcOp(t, npc, nil, OpNpcChangeTypeKeepAll, []int{42, 100}) // id=42, duration=100
    _ = state

    if len(npc.changeTypeKeepAllCalls) != 1 {
        t.Fatalf("changeTypeKeepAllCalls: got %d entries, want 1", len(npc.changeTypeKeepAllCalls))
    }
    got := npc.changeTypeKeepAllCalls[0]
    if got.newType != 42 || got.duration != 100 {
        t.Errorf("call: got {newType=%d, duration=%d}, want {42, 100}", got.newType, got.duration)
    }
    if len(npc.changeTypeCalls) != 0 {
        t.Errorf("changeTypeCalls: got %d, want 0 (KEEPALL should not dispatch through ChangeType)",
            len(npc.changeTypeCalls))
    }
}
```

Also extend the existing no-active-npc test table at `pkg/script/handlers_npc_test.go:352-353`. Find the table literal containing `{"NPC_STAT", OpNpcStat, ...}` / `{"NPC_BASESTAT", OpNpcBaseStat, ...}` entries. Add these two rows (alongside the existing `NPC_CHANGETYPE` row if present, else after `NPC_BASESTAT`):

```go
{"NPC_CHANGETYPE_KEEPALL", OpNpcChangeTypeKeepAll, []int{42, 100}, "NPC_CHANGETYPE_KEEPALL: no active npc"},
```

- [ ] **Step 3.2 — Run tests to verify they fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/... -run 'TestChangeTypeKeepAll|TestHandleNpcChangeTypeKeepAll' -count=1
```

Expected: FAIL with compile errors (`ChangeTypeKeepAll` undefined on `*Npc`; `changeTypeKeepAllCalls` undefined on mockNpc; `handleNpcChangeTypeKeepAll` not in dispatch map).

- [ ] **Step 3.3 — Add `ChangeTypeKeepAll` public method on `*Npc`**

Edit `modules/world/npc_masks.go`. Insert the following AFTER the existing `ChangeType` body (which now delegates to `changeTypeImpl` after Task 2):

```go
// ChangeTypeKeepAll morphs the NPC to newType and schedules a revert
// after `duration` ticks without resetting stats. Dispatched from
// NPC_CHANGETYPE_KEEPALL (opcode 2506). Mirrors TS Npc.changeType at
// Engine-TS/.../Npc.ts:427-449 with reset=false. The revert, when it
// fires, takes the light path (resetOnRevert=false → Task 4 branch).
func (n *Npc) ChangeTypeKeepAll(newType, duration int) {
    n.changeTypeImpl(newType, duration, false)
}
```

- [ ] **Step 3.4 — Extend `ActiveNpc` interface**

Edit `pkg/script/active.go`. Locate the existing `ChangeType(newType, duration int)` method on the interface (around line 344). Immediately below it, insert:

```go

// ChangeTypeKeepAll morphs the NPC to newType and schedules a revert
// after `duration` ticks, preserving all current stat values (no
// reset). The revert, when it fires, takes the light path
// (resetOnRevert=false → typeId + uid + CHANGE_TYPE mask only).
// Mirrors TS Npc.changeType at Engine-TS/.../Npc.ts:427-449 with
// reset=false, dispatched from NPC_CHANGETYPE_KEEPALL (opcode 2506,
// TS NpcOps.ts:465-471). No-op when duration < 1 OR when the NPC is dead.
ChangeTypeKeepAll(newType, duration int)
```

Also DELETE the existing `DEFERRED: the optional reset=false variant...` comment block at lines 340-343 (pre-NAI-17 positioning). The block is now obsolete.

- [ ] **Step 3.5 — Add `handleNpcChangeTypeKeepAll`**

Edit `pkg/script/handlers_npc.go`. Locate `handleNpcChangeType` (currently around line 179). DELETE the preceding `DEFERRED: NPC_CHANGETYPE_KEEPALL...` comment block at lines 176-178. Leave the `handleNpcChangeType` doc comment rewritten (drop the "DEFERRED" reference — stats-reset is now in scope):

```go
// handleNpcChangeType pops (newType, duration) in TS order (duration
// on top) and morphs the NPC. Matches TS NpcOps.ts:457-462.
// The full body (guard + typeId/uid/mask + stats-reset +
// lifecycleTick fast-path) lives in *Npc.changeTypeImpl.
```

Immediately after the closing `}` of `handleNpcChangeType`, insert:

```go

// handleNpcChangeTypeKeepAll pops (newType, duration) in TS order
// (duration on top) and morphs the NPC preserving all current stats.
// Matches TS NpcOps.ts:465-471.
func handleNpcChangeTypeKeepAll(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_CHANGETYPE_KEEPALL"); err != nil {
        return err
    }
    duration := s.PopInt()
    newType := s.PopInt()
    s.ActiveNpc.ChangeTypeKeepAll(newType, duration)
    return nil
}
```

- [ ] **Step 3.6 — Register opcode in dispatch table**

Edit `pkg/script/handlers.go`. Find the line `OpNpcChangeType: handleNpcChangeType,` in the dispatch map. Insert this line directly after (keep map entries grouped by NPC op):

```go
OpNpcChangeTypeKeepAll: handleNpcChangeTypeKeepAll,
```

- [ ] **Step 3.7 — Extend `mockNpc` in `handlers_npc_test.go`**

Edit `pkg/script/handlers_npc_test.go`. Find the existing `mockNpc` struct and method set (around lines 30-80). Add a new `changeTypeKeepAllCalls` field to the struct:

```go
changeTypeKeepAllCalls []struct{ newType, duration int }
```

And add a new method (place near the existing `ChangeType` stub):

```go
func (m *mockNpc) ChangeTypeKeepAll(newType, duration int) {
    m.changeTypeKeepAllCalls = append(m.changeTypeKeepAllCalls, struct{ newType, duration int }{newType, duration})
}
```

- [ ] **Step 3.8 — Extend `mockActiveNpc` in `handlers_player_test.go`**

Edit `pkg/script/handlers_player_test.go` around line 22-23. Locate the `mockActiveNpc` method set. Add:

```go
func (m *mockActiveNpc) ChangeTypeKeepAll(newType, duration int) {}
```

(Stub-only — `handlers_player_test.go` tests don't exercise KEEPALL dispatch.)

- [ ] **Step 3.9 — Run the failing tests to verify they now pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/... -run 'TestChangeTypeKeepAll|TestHandleNpcChangeTypeKeepAll' -count=1
```

Expected: PASS.

- [ ] **Step 3.10 — Run the full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS. All existing tests + new Task 1/2/3 tests all green.

- [ ] **Step 3.11 — Commit**

```bash
git -C /home/owner/Code/github.com/zsrv/goscape add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go modules/world/npc_masks.go modules/world/npc_test.go

git -C /home/owner/Code/github.com/zsrv/goscape commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-17 Task 3 NPC_CHANGETYPE_KEEPALL (opcode 2506) dispatch

Adds *Npc.ChangeTypeKeepAll public method delegating to changeTypeImpl
with reset=false. Registers handleNpcChangeTypeKeepAll in the opcode
dispatch table against the existing OpNpcChangeTypeKeepAll reserved
constant at pkg/script/opcode.go:243. Extends ActiveNpc interface and
both mockNpc + mockActiveNpc test doubles. Deletes the two DEFERRED:
NPC_CHANGETYPE_KEEPALL breadcrumbs at active.go and handlers_npc.go.

revertType branching on resetOnRevert (Task 4) and regen 6-stat
expansion (Task 5) still pending.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4 — `revertType` `resetOnRevert` branching + 6-slot reseed

**Files:**
- Modify: `modules/world/npc.go` (replace entire `revertType` body at lines 261-285; update doc comment)
- Test: `modules/world/npc_test.go` (add 3 new test functions)

**Rationale:** Ports TS `Npc.ts:1082-1091` light-vs-heavy branching on `resetOnRevert`. Light path (KEEPALL consumer) touches only typeId/uid/CHANGE_TYPE mask. Heavy path expands from single-HP-slot reseed (Task 1 intermediate) to the full 6-slot loop matching TS `resetEntity:287-290`. Tail re-arms `resetOnRevert = true` on BOTH branches so a subsequent CHANGETYPE after a KEEPALL cycle behaves as default.

### Steps

- [ ] **Step 4.1 — Write the failing revertType tests**

Append to `modules/world/npc_test.go`:

```go
// TestRevertTypeHonorsResetOnRevertFalse verifies the light path
// (TS Npc.ts:1086-1090): typeId + uid + CHANGE_TYPE mask only;
// stats/queue/waypoints/hunt fields unchanged.
func TestRevertTypeHonorsResetOnRevertFalse(t *testing.T) {
    s := newServerForScriptTest(t)
    baseTyp := &objtype.NpcType{Stats: []uint16{10, 10, 10, 10, 10, 10}}
    s.npcTypes = &objtype.NpcTypeCache{Configs: []*objtype.NpcType{baseTyp}}

    n := NewNpc(1, 0, 100, 100, 0, baseTyp)
    n.server = s
    // Simulate post-KEEPALL state: typeId != baseType, resetOnRevert=false,
    // stats have survived a morph, queue/waypoints/hunt fields populated.
    n.typeId = 99
    n.uid = (99 << 16) | n.nid
    n.resetOnRevert = false
    n.levels[objtype.NpcStatAttack]    = 5  // drained
    n.levels[objtype.NpcStatHitpoints] = 7
    n.baseLevels[objtype.NpcStatAttack]    = 20 // not from baseTyp
    n.baseLevels[objtype.NpcStatHitpoints] = 20
    n.queue = []script.NpcQueueRequest{{Trigger: 0, Delay: 5, IntArg: 42}}
    n.waypointIndex = 3
    n.huntClock = 7
    n.huntRange = 99

    n.revertType()

    // Light path: typeId reverted, uid recomputed, mask raised.
    if n.typeId != n.baseType {
        t.Errorf("typeId: got %d, want %d (baseType)", n.typeId, n.baseType)
    }
    if n.uid != (n.baseType<<16)|n.nid {
        t.Errorf("uid: got %d, want %d", n.uid, (n.baseType<<16)|n.nid)
    }
    if n.masks&rsbuf.NpcMaskChangeType == 0 {
        t.Errorf("mask: CHANGE_TYPE bit not set")
    }
    // Light path: stats/queue/waypoints/hunt UNCHANGED.
    if n.levels[objtype.NpcStatAttack] != 5 {
        t.Errorf("levels[ATK]: got %d, want 5 (light path preserves)", n.levels[objtype.NpcStatAttack])
    }
    if n.baseLevels[objtype.NpcStatAttack] != 20 {
        t.Errorf("baseLevels[ATK]: got %d, want 20 (light path preserves)", n.baseLevels[objtype.NpcStatAttack])
    }
    if len(n.queue) != 1 {
        t.Errorf("queue: got len=%d, want 1 (light path preserves)", len(n.queue))
    }
    if n.waypointIndex != 3 {
        t.Errorf("waypointIndex: got %d, want 3 (light path preserves)", n.waypointIndex)
    }
    if n.huntClock != 7 {
        t.Errorf("huntClock: got %d, want 7 (light path preserves)", n.huntClock)
    }
    if n.huntRange != 99 {
        t.Errorf("huntRange: got %d, want 99 (light path preserves)", n.huntRange)
    }
    // Re-arm tail.
    if !n.resetOnRevert {
        t.Errorf("resetOnRevert: got false, want true (re-armed after revert)")
    }
}

// TestRevertTypeHonorsResetOnRevertTrue verifies the heavy path
// reseeds all 6 stats from n.typ.Stats (expands S6d's HP-only reseed).
func TestRevertTypeHonorsResetOnRevertTrue(t *testing.T) {
    s := newServerForScriptTest(t)
    baseTyp := &objtype.NpcType{
        Stats:     []uint16{7, 11, 13, 17, 19, 23},
        HuntRange: 8,
        HuntMode:  -1,
    }
    morphTyp := &objtype.NpcType{Stats: []uint16{50, 50, 50, 50, 50, 50}}
    s.npcTypes = &objtype.NpcTypeCache{Configs: []*objtype.NpcType{baseTyp, morphTyp}}

    n := NewNpc(1, 0, 100, 100, 0, baseTyp)
    n.server = s
    // Simulate post-CHANGETYPE state: morphed + stats-reset to morphTyp.
    n.typeId = 1
    n.typ = morphTyp
    n.uid = (1 << 16) | n.nid
    n.resetOnRevert = true
    for i := 0; i < objtype.NpcStatCount; i++ {
        n.levels[i] = 50
        n.baseLevels[i] = 50
    }
    n.queue = []script.NpcQueueRequest{{Trigger: 0, Delay: 5, IntArg: 42}}

    n.revertType()

    // Heavy path: stats reseeded to baseTyp, queue cleared.
    want := []int{7, 11, 13, 17, 19, 23}
    for i := 0; i < objtype.NpcStatCount; i++ {
        if n.levels[i] != want[i] {
            t.Errorf("levels[%d]: got %d, want %d (reseed from baseTyp)", i, n.levels[i], want[i])
        }
        if n.baseLevels[i] != want[i] {
            t.Errorf("baseLevels[%d]: got %d, want %d", i, n.baseLevels[i], want[i])
        }
    }
    if n.queue != nil {
        t.Errorf("queue: got %v, want nil (heavy path clears)", n.queue)
    }
    if n.typeId != n.baseType {
        t.Errorf("typeId: got %d, want baseType=%d", n.typeId, n.baseType)
    }
    if !n.resetOnRevert {
        t.Errorf("resetOnRevert: got false, want true (re-armed)")
    }
}

// TestRevertTypeReArmsResetOnRevert is a dedicated assertion of the
// re-arm tail on the light path (the heavy-path test above also
// asserts this, but re-arm regression is worth pinning in a named test).
func TestRevertTypeReArmsResetOnRevert(t *testing.T) {
    n := newNpcForLifecycleTest(t)
    n.resetOnRevert = false
    n.typeId = 42 // != baseType so the typeId write path runs

    n.revertType()

    if !n.resetOnRevert {
        t.Errorf("resetOnRevert: got false, want true (re-armed after revert)")
    }
}
```

- [ ] **Step 4.2 — Run tests to verify they fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestRevertType' -count=1
```

Expected: FAIL — current `revertType` ignores `resetOnRevert` and the heavy-path reseed is still HP-slot-only.

- [ ] **Step 4.3 — Rewrite `revertType`**

Edit `modules/world/npc.go:261-285` (approximate — locate the current `revertType` function and its doc comment block). REPLACE the entire block (doc comment + function body) with:

```go
// revertType restores the NPC to its baseline type. Called from the
// Events block (npc_ai.go:37-40) when lifecycleTick hits 0 on
// RESPAWN+alive, and from the respawn path on revival.
//
// Branches on resetOnRevert (written by changeTypeImpl):
//   - resetOnRevert=false (KEEPALL path): TS Npc.ts:1086-1090 light path.
//     Restore typeId/uid + raise CHANGE_TYPE mask. No stats reset, no
//     queue clear, no waypoint clear, no hunt-field reset. Intended
//     for short-lived morphs that must preserve combat state.
//   - resetOnRevert=true (default, CHANGETYPE path): current heavy-path
//     behavior — inline reset of type/uid/typ, full 6-slot stats reseed
//     (expanded from S6d's HP-only reseed), queue/waypoint clear, tele
//     flag, hunt-field reset.
//
// NAI-17-D1 (tracked deviation): TS's heavy path is World.removeNpc +
// World.addNpc — a despawn+respawn that re-runs the constructor. Go
// does an INLINE reset instead, pre-existing since S6d. See spec §8.
//
// Tail re-arm: sets resetOnRevert = true on BOTH branches so a
// subsequent CHANGETYPE on the same NPC starts from the default. TS
// gets this for free via the ctor rerun; Go must re-arm explicitly.
func (n *Npc) revertType() {
    if !n.resetOnRevert {
        // Light path — TS Npc.ts:1086-1090.
        if n.typeId != n.baseType {
            n.typeId = n.baseType
            n.uid = (n.typeId << 16) | n.nid
        }
        n.masks |= rsbuf.NpcMaskChangeType
        n.resetOnRevert = true
        return
    }

    // Heavy path — inline reset matching TS resetEntity:280-317 semantics
    // (minus the World.removeNpc/addNpc structural call; see NAI-17-D1).
    if n.typeId != n.baseType {
        n.typeId = n.baseType
        n.uid = (n.typeId << 16) | n.nid
        if newTyp := n.lookupType(n.baseType); newTyp != nil {
            n.typ = newTyp
        }
    }
    // Full 6-slot stats reseed (TS resetEntity:287-290).
    if n.typ != nil {
        for i := 0; i < objtype.NpcStatCount && i < len(n.typ.Stats); i++ {
            v := int(n.typ.Stats[i])
            n.levels[i] = v
            n.baseLevels[i] = v
        }
    }
    n.queue = nil
    n.waypointIndex = -1
    n.tele = true
    n.masks |= rsbuf.NpcMaskChangeType
    n.huntClock = 0
    n.huntTarget = nil
    if n.typ != nil {
        n.huntRange = int(n.typ.HuntRange)
        n.huntMode = n.typ.HuntMode
    }
    n.resetOnRevert = true // re-arm default for next morph cycle
}
```

Remove any leftover import of the `n.curHP/initialHP` style now that the HP-slot-only Task 1 intermediate is gone. Ensure `"github.com/zsrv/goscape/pkg/objtype"` import remains.

- [ ] **Step 4.4 — Run the failing tests to verify they now pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestRevertType' -count=1
```

Expected: PASS.

- [ ] **Step 4.5 — Run the full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS. In particular, existing NAI-5 test `TestNpcTurnRespawnAliveMorphReverts` and NAI-16 Task 3 direct revert test stay green — both set `resetOnRevert=true` implicitly via their NPC construction (NewNpc default), so the heavy path runs.

- [ ] **Step 4.6 — Commit**

```bash
git -C /home/owner/Code/github.com/zsrv/goscape add modules/world/npc.go modules/world/npc_test.go

git -C /home/owner/Code/github.com/zsrv/goscape commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-17 Task 4 revertType light/heavy branch on resetOnRevert

Ports TS Npc.ts:1082-1091. Light path (KEEPALL consumer, resetOnRevert=false)
touches only typeId + uid + CHANGE_TYPE mask. Heavy path (CHANGETYPE default,
resetOnRevert=true) expands from S6d's HP-slot-only reseed to the full 6-slot
loop matching TS resetEntity:287-290. Both branches re-arm resetOnRevert=true
so a subsequent CHANGETYPE defaults correctly.

Tracked deviation NAI-17-D1: Go's heavy path is inline reset, not TS's
World.removeNpc+World.addNpc despawn+respawn. Pre-existing since S6d;
now explicitly named in the deviation ledger.

Regen 6-stat expansion lands in Task 5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5 — Regen loop 6-stat expansion

**Files:**
- Modify: `modules/world/npc_script.go` (regen block at lines 237-240 — actual position may drift after Tasks 1-4)
- Test: `modules/world/npc_script_test.go` (add 1 new test with 2 sub-cases)

**Rationale:** Ports TS `Npc.ts:515-523` — iterate all 6 stats, converging `levels[i]` toward `baseLevels[i]`. Behaviorally a no-op at HEAD (no producer writes non-HP `levels[]`), but forecloses the silent divergence the moment a boost/drain opcode lands. Separate task from Task 4 so each has a minimal, targeted test surface.

### Steps

- [ ] **Step 5.1 — Write the failing regen test**

Append to `modules/world/npc_script_test.go` (or create the file if it doesn't exist yet — locate regen-related tests first):

```go
// TestNpcRegenIteratesAllSixStats verifies that the regen loop
// converges levels[i] toward baseLevels[i] for ALL 6 stats, not
// just HP. Mirrors TS Npc.ts:515-523.
func TestNpcRegenIteratesAllSixStats(t *testing.T) {
    t.Run("drain-converges-up", func(t *testing.T) {
        s := newServerForScriptTest(t)
        typ := &objtype.NpcType{Stats: []uint16{10, 10, 10, 10, 10, 10}, RegenRate: 1}
        n := NewNpc(1, 0, 100, 100, 0, typ)
        n.server = s
        n.regenInterval = 1
        n.regenClock = 1 // ready to fire
        // Seed drains on non-HP slots.
        n.levels[objtype.NpcStatStrength] = 5
        n.levels[objtype.NpcStatMagic]    = 8

        processNpcRegen(s, n)

        if n.levels[objtype.NpcStatStrength] != 6 {
            t.Errorf("levels[STR]: got %d, want 6 (regen increment)", n.levels[objtype.NpcStatStrength])
        }
        if n.levels[objtype.NpcStatMagic] != 9 {
            t.Errorf("levels[MAG]: got %d, want 9 (regen increment)", n.levels[objtype.NpcStatMagic])
        }
    })

    t.Run("boost-converges-down", func(t *testing.T) {
        s := newServerForScriptTest(t)
        typ := &objtype.NpcType{Stats: []uint16{10, 10, 10, 10, 10, 10}, RegenRate: 1}
        n := NewNpc(1, 0, 100, 100, 0, typ)
        n.server = s
        n.regenInterval = 1
        n.regenClock = 1
        // Seed boosts on non-HP slots.
        n.levels[objtype.NpcStatRanged]   = 12
        n.levels[objtype.NpcStatDefence]  = 15

        processNpcRegen(s, n)

        if n.levels[objtype.NpcStatRanged] != 11 {
            t.Errorf("levels[RNG]: got %d, want 11 (regen decrement)", n.levels[objtype.NpcStatRanged])
        }
        if n.levels[objtype.NpcStatDefence] != 14 {
            t.Errorf("levels[DEF]: got %d, want 14 (regen decrement)", n.levels[objtype.NpcStatDefence])
        }
    })
}
```

NOTE: The `processNpcRegen` name and call shape above is an educated guess based on the grep of `npc_script.go:237-240`. The subagent implementing this task MUST first grep `modules/world/npc_script.go` for the actual regen function name and surrounding fixture-invocation shape — there may be an alternate entry point (e.g., called through `n.turn(s)`). If the test has to call `n.turn(s)` to reach regen, adjust the setup accordingly. The seed-and-assert logic stays the same.

- [ ] **Step 5.2 — Run tests to verify they fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestNpcRegenIteratesAllSixStats' -count=1
```

Expected: FAIL — current regen touches only HP, so STR/MAG/RNG/DEF levels don't change.

- [ ] **Step 5.3 — Expand the regen loop**

Edit `modules/world/npc_script.go`. Locate the existing two-case switch (post-Task-1 form):

```go
    case n.levels[objtype.NpcStatHitpoints] < n.baseLevels[objtype.NpcStatHitpoints]:
        n.levels[objtype.NpcStatHitpoints]++
    case n.levels[objtype.NpcStatHitpoints] > n.baseLevels[objtype.NpcStatHitpoints]:
        n.levels[objtype.NpcStatHitpoints]--
```

Replace both cases with a 6-slot loop (mirrors TS Npc.ts:515-523):

```go
    // NAI-17: iterate all 6 stats, converging levels[i] toward baseLevels[i].
    // TS Npc.ts:515-523.
    for i := 0; i < objtype.NpcStatCount; i++ {
        switch {
        case n.levels[i] < n.baseLevels[i]:
            n.levels[i]++
        case n.levels[i] > n.baseLevels[i]:
            n.levels[i]--
        }
    }
```

Pay attention to the enclosing `switch` vs `if` structure — if Task 1 left the cases inside an outer `switch`, the replacement above (inner `for` + nested `switch`) needs the outer `switch` replaced by a plain block (`{ ... }`). If Task 1 left them inside an `if n.regenClock >= n.regenInterval`, the `for` inserts directly in place of the two cases.

- [ ] **Step 5.4 — Run the failing tests to verify they now pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestNpcRegenIteratesAllSixStats' -count=1
```

Expected: PASS.

- [ ] **Step 5.5 — Run the full test suite**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS. Existing HP-regen tests at `npc_event_queue_test.go` stay green — the loop preserves HP-slot convergence behavior identically.

- [ ] **Step 5.6 — Commit**

```bash
git -C /home/owner/Code/github.com/zsrv/goscape add modules/world/npc_script.go modules/world/npc_script_test.go

git -C /home/owner/Code/github.com/zsrv/goscape commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-17 Task 5 regen iterates all 6 stats (TS Npc.ts:515-523)

Expands the regen convergence from HP-slot-only to a 6-slot loop matching
TS behavior. Behaviorally a no-op at HEAD (no producer writes non-HP
levels[] slots), but closes the silent divergence the moment a stat
boost/drain opcode lands.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6 — Close: memory update + full-suite verify

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (mark "From NAI-16: Deferred: NPC stats-array + KEEPALL variant" entry as Resolved with close-commit ref)

**Rationale:** Close the sub-spec cleanly per `runescript_cadence.md`. The memory update names the close-commit hash so future NAI brainstorms can grep "From NAI-16" and see NAI-17 as the closure point. Apply `Closes memory:` trailer per `close_commit_memory_trailer.md`.

### Steps

- [ ] **Step 6.1 — Run the full test suite one more time**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1
```

Expected: PASS. Include `-race` to surface any data races introduced by the struct-field changes (unlikely but a cheap check).

- [ ] **Step 6.2 — Update `nai_followups.md`**

Edit `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`. Locate the "## From NAI-16 (2026-04-23)" section containing the "### Deferred: NPC stats-array + KEEPALL variant" entry.

Add a Resolved preamble BEFORE the existing body (preserve the original body as historical context, matching the pattern used for other Resolved entries in the file):

```markdown
### Deferred: NPC stats-array + KEEPALL variant

**Resolved 2026-04-23 (NAI-17)** in commits for Tasks 1-5 of
`docs/superpowers/plans/2026-04-23-nai-17-npc-stats-keepall.md`.
`*Npc.levels [objtype.NpcStatCount]int` and `*Npc.baseLevels
[objtype.NpcStatCount]int` arrays ship at HEAD, seeded from `typ.Stats`
in `NewNpc` (Task 1). `NpcStat` / `NpcBaseStat` / `CurHP` / `BaseHP`
all read through the arrays; `curHP`/`baseHP` scalars and `initialHP`
helper are deleted. `ChangeType` refactored behind private
`changeTypeImpl(newType, duration, reset bool)`; stats-reset formula
`max(newBase − (baseLevels[i] − levels[i]), 0)` ported from TS
Npc.ts:436-443 (Task 2). `ChangeTypeKeepAll` public method + interface
entry + opcode 2506 handler ship (Task 3). `revertType` branches on
`resetOnRevert`: light path preserves stats/queue/waypoints/hunt per TS
Npc.ts:1086-1090; heavy path reseeds 6 slots per TS resetEntity:287-290
(Task 4). Regen loop expands to iterate all 6 stats per TS Npc.ts:515-523
(Task 5). One tracked deviation: NAI-17-D1 (Go inline revertType vs TS
despawn+respawn — pre-existing since S6d, now explicitly named). See
`docs/superpowers/specs/2026-04-23-nai-17-npc-stats-keepall-design.md`.

---

_Original deferral body (preserved for historical context):_

NAI-16 ported TS `Npc.changeType` minus the stats-reset branch
[...existing body unchanged...]
```

- [ ] **Step 6.3 — Commit the close**

```bash
git -C /home/owner/Code/github.com/zsrv/goscape add -- :!memory

# Memory file lives outside the repo; stage explicitly if in-scope.
# (The nai_followups.md edit above is in ~/.claude/projects/... NOT in
# the goscape working tree. No `git add` needed for it.)

git -C /home/owner/Code/github.com/zsrv/goscape commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(nai): NAI-17 closed — NPC stats-array + NPC_CHANGETYPE_KEEPALL

Tasks 1-5 complete. The "From NAI-16: Deferred: NPC stats-array + KEEPALL
variant" entry in nai_followups.md is marked Resolved.

Shipped:
  - *Npc.levels [6]int + baseLevels [6]int; curHP/baseHP scalars deleted
  - objtype.NpcStatCount constant
  - ChangeType refactor: private changeTypeImpl + lookupType +
    resetStatsForType helpers; TS Npc.ts:436-443 boost/drain-preserving
    stats-reset formula
  - ChangeTypeKeepAll public method + ActiveNpc interface extension
  - NPC_CHANGETYPE_KEEPALL (opcode 2506) handler registered
  - revertType: resetOnRevert branching — light path (TS:1086-1090)
    preserves state; heavy path expands to 6-slot reseed (TS
    resetEntity:287-290)
  - processNpcRegen: 6-stat convergence loop (TS Npc.ts:515-523)

Tracked deviation added:
  - NAI-17-D1: Go inline revertType vs TS World.removeNpc+addNpc despawn+respawn
               (pre-existing since S6d; now explicitly named in the ledger)

Closes memory: From NAI-16 → Deferred: NPC stats-array + KEEPALL variant

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(`--allow-empty` because the memory file isn't in the repo tree; the close commit is informational. If the implementer chooses to touch something in-tree as part of closure — e.g., a trailing DEFERRED comment sweep — drop `--allow-empty`.)

---

## Self-Review (to be run by the plan author, not the subagent)

**Spec coverage check:**

| Spec section | Task(s) |
|--------------|---------|
| §1 file-by-file delta row: NpcStatCount constant | Task 1 Step 1.1 |
| §1: levels/baseLevels/resetOnRevert fields | Task 1 Step 1.4 |
| §1: NewNpc stats-seeding loop | Task 1 Step 1.5 |
| §1: initialHP helper deletion | Task 1 Step 1.6 |
| §1: CurHP/BaseHP accessors | Task 1 Step 1.7 |
| §1: NpcStat/NpcBaseStat getters | Task 1 Step 1.8 |
| §1: Damage body migration | Task 1 Step 1.9 |
| §1: ResetHP body migration | Task 1 Step 1.10 |
| §1: regen HP-only-through-array (temporary) | Task 1 Step 1.11 |
| §1: revertType HP reseed (temporary) | Task 1 Step 1.12 |
| §1: ResetMasks doc comment | Task 1 Step 1.13 |
| §1: npc_event_queue_test.go assignments | Task 1 Step 1.14 |
| §3: ActiveNpc.ChangeType doc comment | Task 2 Step 2.4 |
| §3: ActiveNpc.ChangeTypeKeepAll interface | Task 3 Step 3.4 |
| §3: changeTypeImpl + helpers | Task 2 Step 2.3 |
| §3: ChangeType delegate | Task 2 Step 2.3 |
| §3: ChangeTypeKeepAll public method | Task 3 Step 3.3 |
| §3: handleNpcChangeTypeKeepAll | Task 3 Step 3.5 |
| §3: handlers.go dispatch entry | Task 3 Step 3.6 |
| §3: DEFERRED comment deletions | Task 2 Step 2.4 + Task 3 Steps 3.4, 3.5 |
| §3: mockNpc + mockActiveNpc stubs | Task 3 Steps 3.7, 3.8 |
| §4: revertType rewrite | Task 4 Step 4.3 |
| §5: regen 6-stat loop | Task 5 Step 5.3 |
| §6 (tests): TestNewNpcSeedsStatsFromType | Task 1 Step 1.2 |
| §6: TestNewNpcWithNilStatsStaysZero | Task 1 Step 1.2 |
| §6: TestNpcStatAllSlots/BaseStatAllSlots | Task 1 Step 1.2 |
| §6: TestNpcStatOutOfRange/BaseStatOutOfRange | Task 1 Step 1.2 |
| §6: TestChangeTypeResetsStatsWithBoostPreservation | Task 2 Step 2.1 |
| §6: TestChangeTypeResetsStatsClampedAtZero | Task 2 Step 2.1 |
| §6: TestChangeTypeKeepAllPreservesStats | Task 3 Step 3.1 |
| §6: TestChangeTypeKeepAllDurationZeroNoOp | Task 3 Step 3.1 |
| §6: TestChangeTypeKeepAllDeadNoOp | Task 3 Step 3.1 |
| §6: TestRevertTypeHonorsResetOnRevertFalse | Task 4 Step 4.1 |
| §6: TestRevertTypeHonorsResetOnRevertTrue | Task 4 Step 4.1 |
| §6: TestRevertTypeReArmsResetOnRevert | Task 4 Step 4.1 |
| §6: TestNpcRegenIteratesAllSixStats | Task 5 Step 5.1 |
| §6: TestHandleNpcChangeTypeKeepAllDispatch | Task 3 Step 3.1 |
| §6: TestHandleNpcChangeTypeKeepAllNoActiveNpc | Task 3 Step 3.1 |
| nai_followups.md Resolved update | Task 6 Step 6.2 |

Every spec requirement maps to a concrete task step. No gaps.

**Type-consistency check:**
- `NpcStatCount` used identically in constant decl, array sizes, and bounds checks.
- `changeTypeImpl(newType, duration int, reset bool)` signature consistent across Tasks 2 + 3.
- `lookupType(typeId int)` used identically in changeTypeImpl and revertType.
- `resetStatsForType(newTyp *objtype.NpcType)` only called by changeTypeImpl.
- `changeTypeKeepAllCalls` field name consistent between Step 3.7 (add) and Step 3.1 (test read).

**Placeholder scan:** none. Every code block is complete. Test names, expected test output, and commands are specified.
