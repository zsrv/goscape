# NAI-190 — `World.reload()` port + `::reload` cheat — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `World.reload()` (`World.ts:206-292`) to `(*Server).Reload(clearInvs bool) error` and wire the `::reload` cheat in the dev-block. Closes 1 of 2 carryforward items.

**Architecture:** Re-invoke all 18 `objtype.LoadXxx` loaders into new local registries, atomically swap onto `Server.*`, reconcile `s.vars`/`s.varsStrings` if `VarSharedType` count changed, reconcile world-shared and per-player invs (when `clearInvs=true`), reload `*Provider` (signature change to surface count), regen CRCs, re-run `cache.PreloadClient(...)`, and re-inject loc/obj types into GameMap (D1). Runs synchronously on the tick goroutine — no locks needed.

**Tech Stack:** Go 1.26+. Touches `pkg/script/provider.go` (signature change), `modules/world/reload.go` (new), `modules/world/reload_test.go` (new), `modules/world/handlers_game.go` (cheat wiring + carryforward rewrite), `modules/world/server.go` (one call-site update for Provider.Load).

**Spec:** `docs/superpowers/specs/2026-05-13-nai-190-world-reload-design.md`
**Predecessors:** NAI-189 (DEBUGPROC dispatch; closed `ClientCheatHandler.ts` except for `reload`/`rebuild`).
**HEAD at plan-write:** `27e503a` (spec commit).

---

## §0 Pre-flight verifications (controller, not implementer)

These were performed at plan-write and codified into the plan below. Re-verify only if the implementer encounters a contradiction.

1. **`Provider.Load` signature** — `pkg/script/provider.go:42` currently returns `error` only. Need to add a count. Single non-test caller: `modules/world/server.go:358`.
2. **`s.players` shape** — `[2048]*Player` array (fixed-size, sparse with nil slots, indices 1..2047). Verified at `modules/world/server.go:56`.
3. **`inventory.FromType` signature** — `(t *objtype.InvType) *Inventory` at `pkg/inventory/inventory.go:40`. Takes a pointer, NOT an id. Plan code threads `inv` (the loop iterator).
4. **`Server.BroadcastMes`** — already exists at `modules/world/server_broadcast.go:8` (NAI-185 T8). Uses `Player.MessageGame`, NOT `WrappedMessageGame`. Holds `s.playersMu.RLock` internally. **Reload reuses it as-is.** No newline-split logic needed (TS broadcast messages here are single-line).
5. **`s.cfg.NodeDebug` / `s.cfg.CachePath`** — both present (`modules/world/config.go:19, 41`). `cfg` is a value field, not a pointer (`modules/world/server.go:53`).
6. **CategoryType loader** — goscape has **no** `LoadCategoryTypes` (confirmed via `pkg/script/handlers_npc.go:105-110` which documents the gap). Step 1 of the pipeline omits the local; tag **DEVIATION-NAI-190-D4-NO-CATEGORYTYPES** at the Reload doc-comment.
7. **`p.invs` mutation pattern** — `tick.go:147` writes `p.invs = map[int]*inventory.Inventory{}` without locking; reload follows the same single-goroutine posture (memory `plan_race_tag_for_cross_goroutine_test`).
8. **`s.gamemap` setters** — `gm.SetLocTypes(locTypes *objtype.LocTypeConfigs)`, `gm.SetObjTypes(objTypes *objtype.ObjTypeConfigs)`, `gm.SetMembers(b bool)` all exist (`modules/world/server.go:233-236`).
9. **`scriptProvider.Load` failure surface** — current Load returns error from file-read failures (`os.ReadFile`), short-file errors, or version-mismatch. Per-script decode errors `continue` and log via slog. TS's `count == -1` ⇒ top-level failure. Goscape mapping: on any returned error, count = -1.
10. **Real cache at `data/pack/`** is fully populated (~19 .dat/.idx files). Integration tests can use relative path `filepath.Join("..", "..", "data", "pack")` from the world package test binary's CWD.

---

## §1 File map

| Path | Action | Responsibility |
|---|---|---|
| `pkg/script/provider.go` | Modify ~L42-106 | Signature change: `Load(cacheDir) error` → `Load(cacheDir) (count int, err error)`. Count = number of successfully-decoded scripts. |
| `pkg/script/provider_test.go` | Modify (existing tests) | Update any test that asserts on `Load(...)` returning only `error`. |
| `modules/world/server.go` | Modify `:358` | Update call site: `count, err := s.scriptProvider.Load(...)`; ignore count at boot (log as info). |
| `modules/world/reload.go` | **Create** | The 11-step `Reload` pipeline + pure helpers (`resizeVarShared`, `reconcileInvs`). |
| `modules/world/reload_test.go` | **Create** | Unit tests for helpers + integration tests for `Reload`. |
| `modules/world/handlers_game.go` | Modify `:585-589` (`reload` case in dev-block fixed-cmd switch) + `:539-547` (carryforward rewrite) | Wire `::reload` cheat; rewrite carryforward block. |

---

## §2 Task plan

Eleven tasks. Each task is RED → GREEN → REVIEW cycles with explicit commit.

### Task 1: `Provider.Load` signature change to `(int, error)`

**Files:**
- Modify: `pkg/script/provider.go:42-106`
- Modify: `pkg/script/provider_test.go` (existing test assertions)
- Modify: `modules/world/server.go:358`

- [ ] **Step 1: Inventory existing call sites**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...   # baseline must pass
grep -rn "scriptProvider\.Load\|\.Load(.*\"server\"\|provider\.Load(" --include="*.go" .
```

Expected: one production call site at `modules/world/server.go:358`. Any test sites in `pkg/script/provider_test.go` must be enumerated. **If the count exceeds 2 (one production + one test cluster), STOP and surface to controller** — memory `parallel_adapter_init_duplication` warns about cross-call propagation. The plan assumes a small blast radius.

- [ ] **Step 2: Write the failing test in `pkg/script/provider_test.go`**

Add (or modify the existing fixture test) to assert the new signature surfaces a count:

```go
func TestProviderLoad_ReturnsCountOfDecodedScripts(t *testing.T) {
    // Use existing test fixture pattern in this file. The test fixture
    // should produce a Provider.Load call that successfully decodes N
    // non-zero-size entries. Choose a tiny synthetic fixture with
    // exactly 3 valid scripts and assert count == 3.
    cacheDir := writeFixtureCache(t, 3) // existing helper or NEW; see fixture pattern in file
    p := NewProvider()
    count, err := p.Load(cacheDir)
    if err != nil {
        t.Fatalf("Load returned err: %v", err)
    }
    if count != 3 {
        t.Errorf("count: got %d, want 3", count)
    }
}

func TestProviderLoad_ReturnsMinusOneOnReadError(t *testing.T) {
    p := NewProvider()
    count, err := p.Load(t.TempDir()) // empty dir → script.dat ReadFile fails
    if err == nil {
        t.Fatal("Load expected to return error on missing dat")
    }
    if count != -1 {
        t.Errorf("count: got %d, want -1 on top-level error", count)
    }
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run "TestProviderLoad_ReturnsCount|TestProviderLoad_ReturnsMinusOne" -v
```

Expected: **FAIL** (signature mismatch — `count, err :=` won't compile against single-return signature). The compile failure IS the RED phase.

- [ ] **Step 4: Implement the signature change**

In `pkg/script/provider.go:42`, change:

```go
func (p *Provider) Load(cacheDir string) error {
```

to:

```go
// Load reads script.dat and script.idx from cacheDir, validates the compiler
// version, decodes every non-empty entry, and populates the lookup tables.
// Returns the count of successfully-decoded scripts (mirrors TS
// ScriptProvider.load return shape), or -1 with a non-nil error on
// top-level file-read or version-mismatch failure. Per-script decode
// failures are logged via slog and counted as a skipped entry (NOT
// reflected in the returned count). NAI-190.
func (p *Provider) Load(cacheDir string) (int, error) {
```

Then update each error-return point in the body. Top-level errors (`os.ReadFile`, short-file checks, version mismatch) return `(-1, err)`. Per-script decode failures inside the loop `continue` without affecting count (they were skipped before; preserve behavior). At the end:

```go
// Count successfully-decoded scripts (those with non-nil entry in
// p.scripts). Mirrors TS ScriptProvider.load which returns the count
// of decoded entries.
count := 0
for _, f := range p.scripts {
    if f != nil {
        count++
    }
}
return count, nil
```

Replace the final `return nil` (L105) with the count-and-return block.

- [ ] **Step 5: Update production call site `modules/world/server.go:358`**

Change:

```go
s.scriptProvider = script.NewProvider()
if err := s.scriptProvider.Load(filepath.Join(cfg.CachePath, "server")); err != nil {
    s.log.Warn("script provider load failed; scripts will not run", "err", err)
    s.scriptProvider = nil
}
```

to:

```go
s.scriptProvider = script.NewProvider()
if count, err := s.scriptProvider.Load(filepath.Join(cfg.CachePath, "server")); err != nil {
    s.log.Warn("script provider load failed; scripts will not run", "err", err)
    s.scriptProvider = nil
} else {
    s.log.Info("script provider loaded", "count", count)
}
```

- [ ] **Step 6: Run tests + vet to verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/... -run "TestProviderLoad|TestAddPlayerAssignsSlot" -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: PASS for the new tests; existing world tests still PASS; no vet warnings.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/provider.go pkg/script/provider_test.go modules/world/server.go
git commit --no-gpg-sign -m "refactor(script): NAI-190 T1 — Provider.Load returns (count int, error)

TS ScriptProvider.load returns the number of successfully-decoded
scripts (or -1 on top-level failure). Plumb the count through the
goscape signature so World.reload's broadcast can report 'Loaded N
scripts.' vs 'There was an issue while reloading scripts.' per TS
World.ts:273-285.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: VarShared resize helper (pure function, TDD)

**Files:**
- Modify: `modules/world/reload.go` (**create**)
- Modify: `modules/world/reload_test.go` (**create**)

- [ ] **Step 1: Create `modules/world/reload.go` with helper signature**

```go
package world

import (
    "github.com/zsrv/goscape/pkg/objtype"
)

// resizeVarShared mirrors TS World.reload's VarSharedType resize block
// at World.ts:246-268. When the new VarSharedType count differs from
// the old, allocates fresh slices of the new size, copies the overlap
// from old, then re-initializes EVERY slot per type (DEVIATION-NAI-190-
// D3-CANDIDATE-VARSHARED-CLOBBER — TS clobbers copied values; mirrored
// verbatim per the true-to-TS gate).
//
// When the counts match, returns the input slices unchanged (TS L246's
// `if` guard).
func resizeVarShared(oldVars []int32, oldStrs []string, newConfigs []*objtype.VarSharedType) (newVars []int32, newStrs []string) {
    if len(oldVars) == len(newConfigs) {
        return oldVars, oldStrs
    }
    newVars = make([]int32, len(newConfigs))
    newStrs = make([]string, len(newConfigs))
    n := min(len(oldVars), len(newVars))
    copy(newVars, oldVars[:n])
    copy(newStrs, oldStrs[:n])
    // TS L259-267: iterates ALL indices unconditionally, clobbering
    // copied non-string slots. Mirrored verbatim.
    for i := 0; i < len(newVars); i++ {
        varsh := newConfigs[i]
        if varsh == nil {
            continue // goscape-defensive; TS VarSharedType.get(id) returns a sentinel
        }
        if varsh.Type == objtype.ScriptVarTypeString {
            continue
        }
        if varsh.Type == objtype.ScriptVarTypeInt {
            newVars[i] = 0
        } else {
            newVars[i] = -1
        }
    }
    return newVars, newStrs
}
```

- [ ] **Step 2: Write failing tests in `modules/world/reload_test.go`**

```go
package world

import (
    "testing"

    "github.com/zsrv/goscape/pkg/objtype"
)

func TestResizeVarShared_CountUnchanged_ReturnsInputs(t *testing.T) {
    oldVars := []int32{10, 20, 30}
    oldStrs := []string{"a", "b", "c"}
    cfgs := []*objtype.VarSharedType{
        {Type: objtype.ScriptVarTypeInt},
        {Type: objtype.ScriptVarTypeInt},
        {Type: objtype.ScriptVarTypeInt},
    }
    newVars, newStrs := resizeVarShared(oldVars, oldStrs, cfgs)
    // No allocation expected — pointer-identity check.
    if &newVars[0] != &oldVars[0] {
        t.Errorf("expected pass-through on count match (no realloc)")
    }
    if newStrs[0] != "a" || newStrs[2] != "c" {
        t.Errorf("strs not preserved on pass-through: %v", newStrs)
    }
}

func TestResizeVarShared_CountGrew_ClobbersAllNonStringSlots(t *testing.T) {
    oldVars := []int32{10, 20, 30}
    oldStrs := []string{"a", "b", "c"}
    cfgs := []*objtype.VarSharedType{
        {Type: objtype.ScriptVarTypeInt},    // i=0: was 10 → clobbered to 0
        {Type: objtype.ScriptVarTypeInt},    // i=1: was 20 → clobbered to 0
        {Type: objtype.ScriptVarTypeInt},    // i=2: was 30 → clobbered to 0
        {Type: objtype.ScriptVarTypeObj},    // i=3: net-new, OBJ default = -1
        {Type: objtype.ScriptVarTypeLoc},    // i=4: net-new, non-INT non-STRING default = -1
    }
    newVars, _ := resizeVarShared(oldVars, oldStrs, cfgs)
    want := []int32{0, 0, 0, -1, -1}
    for i, v := range want {
        if newVars[i] != v {
            t.Errorf("newVars[%d]: got %d, want %d (DEVIATION-NAI-190-D3-CANDIDATE clobber-after-copy)", i, newVars[i], v)
        }
    }
}

func TestResizeVarShared_StringType_KeepsCopiedValue(t *testing.T) {
    oldVars := []int32{0, 0, 0}
    oldStrs := []string{"hello", "world", "foo"}
    cfgs := []*objtype.VarSharedType{
        {Type: objtype.ScriptVarTypeString},
        {Type: objtype.ScriptVarTypeString},
        {Type: objtype.ScriptVarTypeString},
        {Type: objtype.ScriptVarTypeString}, // net-new STRING slot
    }
    _, newStrs := resizeVarShared(oldVars, oldStrs, cfgs)
    // TS L261-263: STRING slots `continue` — copied values survive,
    // net-new slots are zero-value (empty string).
    if newStrs[0] != "hello" || newStrs[1] != "world" || newStrs[2] != "foo" {
        t.Errorf("STRING slots clobbered: %v (expected [hello world foo \"\"])", newStrs)
    }
    if newStrs[3] != "" {
        t.Errorf("net-new STRING slot non-empty: %q", newStrs[3])
    }
}

func TestResizeVarShared_NilConfigSlot_Skipped(t *testing.T) {
    // goscape-defensive: nil slot in newConfigs neither panics nor mutates.
    oldVars := []int32{10}
    oldStrs := []string{"x"}
    cfgs := []*objtype.VarSharedType{
        {Type: objtype.ScriptVarTypeInt},
        nil, // defensive
        {Type: objtype.ScriptVarTypeInt},
    }
    newVars, _ := resizeVarShared(oldVars, oldStrs, cfgs)
    if len(newVars) != 3 {
        t.Fatalf("newVars len: got %d, want 3", len(newVars))
    }
    if newVars[1] != 0 {
        // Index 1 is nil → loop continues without writing. Default zero.
        t.Errorf("nil-config slot should remain zero: got %d", newVars[1])
    }
}
```

- [ ] **Step 3: Run tests to verify RED then GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestResizeVarShared" -v -count=1
```

Expected: After Step 1's helper is in place, all four tests PASS on first run (since this task wrote helper + tests together — the RED phase is implicit by file creation order, but the test code is the spec). Per memory `plan_red_phase_prediction_old_sut`, no OLD SUT exists for a new helper; PASS is the correct end-state.

- [ ] **Step 4: Commit**

```bash
git add modules/world/reload.go modules/world/reload_test.go
git commit --no-gpg-sign -m "feat(world): NAI-190 T2 — resizeVarShared helper

Pure helper that mirrors TS World.reload L246-268 VarShared resize.
Codifies DEVIATION-NAI-190-D3-CANDIDATE-VARSHARED-CLOBBER (TS
unconditionally re-initializes non-string slots after copy; mirrored
verbatim per true-to-TS gate). Four tests pin: count-unchanged
pass-through, count-grew clobber-after-copy, STRING-slot preservation,
nil-config defensive skip.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Inv reconcile helper (pure function, TDD)

**Files:**
- Modify: `modules/world/reload.go`
- Modify: `modules/world/reload_test.go`

- [ ] **Step 1: Add `reconcileInvs` helper to `modules/world/reload.go`**

```go
import (
    "github.com/zsrv/goscape/pkg/inventory"
    "github.com/zsrv/goscape/pkg/objtype"
)

// reconcileInvs mirrors TS World.reload L221-236 (the `if (clearInvs)`
// branch). Empties s.invs, rebuilds SCOPE_SHARED slots, and deletes
// SCOPE_TEMP slots from each player's invs map.
//
// SCOPE_PERM invs are persisted to save files and not reconciled (TS
// L222-235 does not touch SCOPE_PERM — only SHARED and TEMP have arms).
//
// Runs on the tick goroutine; no lock acquisition (memory
// plan_race_tag_for_cross_goroutine_test: production world is
// single-goroutine; tick is sole writer to p.invs).
func reconcileInvs(serverInvs map[int]*inventory.Inventory, players []*Player, invTypes *objtype.InvTypeConfigs) map[int]*inventory.Inventory {
    fresh := make(map[int]*inventory.Inventory)
    if invTypes == nil {
        return fresh
    }
    for id := 0; id < len(invTypes.Configs); id++ {
        inv := invTypes.Configs[id]
        if inv == nil {
            continue // goscape-defensive; TS InvType.get(id) returns a sentinel
        }
        switch inv.Scope {
        case objtype.InvTypeScopeShared:
            fresh[id] = inventory.FromType(inv)
        case objtype.InvTypeScopeTemp:
            for _, p := range players {
                if p == nil || p.invs == nil {
                    continue
                }
                if _, ok := p.invs[id]; ok {
                    delete(p.invs, id)
                }
            }
            // SCOPE_PERM: TS does not reconcile (persisted).
        }
    }
    _ = serverInvs // input is the pre-reconcile map; we discard it (TS L222: this.invs.clear())
    return fresh
}
```

- [ ] **Step 2: Add failing tests to `modules/world/reload_test.go`**

```go
func TestReconcileInvs_Shared_RebuildsFreshFromType(t *testing.T) {
    sentinel := &inventory.Inventory{} // distinguishable from FromType output
    serverInvs := map[int]*inventory.Inventory{42: sentinel}
    invTypes := &objtype.InvTypeConfigs{
        Configs: makeInvConfigs(5, map[int]int{3: objtype.InvTypeScopeShared}),
    }
    fresh := reconcileInvs(serverInvs, nil, invTypes)
    if _, ok := fresh[42]; ok {
        t.Errorf("sentinel at id 42 leaked through clear")
    }
    if fresh[3] == sentinel {
        t.Errorf("SHARED id 3 not replaced with fresh inv (still sentinel)")
    }
    if fresh[3] == nil {
        t.Errorf("SHARED id 3 missing fresh inv")
    }
}

func TestReconcileInvs_Temp_DeletesFromAllPlayers(t *testing.T) {
    sentinel := &inventory.Inventory{}
    p1 := &Player{invs: map[int]*inventory.Inventory{7: sentinel}}
    p2 := &Player{invs: map[int]*inventory.Inventory{7: sentinel}}
    players := []*Player{nil, p1, p2} // index 0 is nil per goscape's slot-1-indexed convention
    invTypes := &objtype.InvTypeConfigs{
        Configs: makeInvConfigs(10, map[int]int{7: objtype.InvTypeScopeTemp}),
    }
    _ = reconcileInvs(nil, players, invTypes)
    if _, ok := p1.invs[7]; ok {
        t.Errorf("p1.invs[7] should be deleted")
    }
    if _, ok := p2.invs[7]; ok {
        t.Errorf("p2.invs[7] should be deleted")
    }
}

func TestReconcileInvs_Perm_LeftUntouched(t *testing.T) {
    sentinel := &inventory.Inventory{}
    p1 := &Player{invs: map[int]*inventory.Inventory{9: sentinel}}
    invTypes := &objtype.InvTypeConfigs{
        Configs: makeInvConfigs(10, map[int]int{9: objtype.InvTypeScopePerm}),
    }
    _ = reconcileInvs(nil, []*Player{p1}, invTypes)
    if p1.invs[9] != sentinel {
        t.Errorf("SCOPE_PERM inv reconciled (should be untouched)")
    }
}

func TestReconcileInvs_NilInvTypes_ReturnsEmptyMap(t *testing.T) {
    fresh := reconcileInvs(nil, nil, nil)
    if fresh == nil {
        t.Fatal("expected empty non-nil map, got nil")
    }
    if len(fresh) != 0 {
        t.Errorf("expected empty map, got %d entries", len(fresh))
    }
}

// makeInvConfigs builds a []*objtype.InvType of size n with default
// InvTypeScopePerm, overriding specific ids per the scopes map.
func makeInvConfigs(n int, scopes map[int]int) []*objtype.InvType {
    configs := make([]*objtype.InvType, n)
    for i := 0; i < n; i++ {
        configs[i] = &objtype.InvType{Scope: objtype.InvTypeScopePerm}
    }
    for id, scope := range scopes {
        configs[id].Scope = scope
    }
    return configs
}
```

- [ ] **Step 3: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestReconcileInvs" -v -count=1
```

Expected: PASS (helper + tests added together).

- [ ] **Step 4: Commit**

```bash
git add modules/world/reload.go modules/world/reload_test.go
git commit --no-gpg-sign -m "feat(world): NAI-190 T3 — reconcileInvs helper

Pure helper mirroring TS World.reload L221-236 clearInvs branch.
SHARED invs rebuild from type; TEMP invs delete from every player's
invs map; PERM invs untouched (TS-parity — persisted to save file).
Four tests pin all three scope arms plus nil-defensive.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: `Reload` happy-path entry point

**Files:**
- Modify: `modules/world/reload.go` — add `Reload` method.
- Modify: `modules/world/reload_test.go` — happy-path integration tests.

- [ ] **Step 1: Write failing happy-path tests**

```go
import (
    "path/filepath"
    "testing"
)

// realCacheDir returns the path to the project-root data/pack directory.
// Tests run from modules/world/ so the cache is two levels up.
func realCacheDir() string {
    return filepath.Join("..", "..", "data", "pack")
}

func TestReload_FreshLoad_PopulatesAllRegistries(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    if err := s.Reload(true); err != nil {
        t.Fatalf("Reload returned err: %v", err)
    }
    if s.paramTypes == nil || len(s.paramTypes.Configs) == 0 {
        t.Errorf("paramTypes empty post-reload")
    }
    if s.objTypes == nil || len(s.objTypes.Configs) == 0 {
        t.Errorf("objTypes empty post-reload")
    }
    if s.locTypes == nil || len(s.locTypes.Configs) == 0 {
        t.Errorf("locTypes empty post-reload")
    }
    if s.npcTypes == nil || len(s.npcTypes.Configs) == 0 {
        t.Errorf("npcTypes empty post-reload")
    }
    if s.invTypes == nil || len(s.invTypes.Configs) == 0 {
        t.Errorf("invTypes empty post-reload")
    }
    // Spot-check the remaining 13 registries similarly.
    if s.varpTypes == nil || s.varsTypes == nil || s.varnTypes == nil {
        t.Errorf("var*Types empty post-reload")
    }
    if s.enumTypes == nil || s.structTypes == nil {
        t.Errorf("enum/struct types empty post-reload")
    }
    if s.seqTypes == nil || s.spotanimTypes == nil || s.idkTypes == nil {
        t.Errorf("seq/spotanim/idk types empty post-reload")
    }
    if s.mesanimTypes == nil || s.dbTableTypes == nil || s.dbRowTypes == nil || s.dbTableIndex == nil {
        t.Errorf("mesanim/dbtable/dbrow/dbtableindex empty post-reload")
    }
    if s.huntTypes == nil || s.componentTypes == nil {
        t.Errorf("hunt/component types empty post-reload")
    }
}

func TestReload_PreservesIdentitySwap(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    if err := s.Reload(true); err != nil {
        t.Fatalf("first Reload: %v", err)
    }
    objBefore := s.objTypes
    locBefore := s.locTypes
    if err := s.Reload(true); err != nil {
        t.Fatalf("second Reload: %v", err)
    }
    if s.objTypes == objBefore {
        t.Errorf("s.objTypes pointer unchanged across reloads (expected fresh instance)")
    }
    if s.locTypes == locBefore {
        t.Errorf("s.locTypes pointer unchanged across reloads (expected fresh instance)")
    }
}

// newTestServerWithCachePath builds a fresh Server using the real
// objtype loaders against cachePath. Mirrors NewServer's loader
// sequence (modules/world/server.go:218-361) minus tick / TCP setup.
// Used only by reload tests that need a fully-populated registry set.
func newTestServerWithCachePath(t *testing.T, cachePath string) *Server {
    t.Helper()
    s := newTestServer(t)
    s.cfg.CachePath = cachePath
    s.cfg.NodeDebug = true
    s.gamemap = nil // reload's GameMap re-injection step is gated on s.gamemap != nil; tested separately in T7
    return s
}
```

- [ ] **Step 2: Verify RED**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestReload_FreshLoad|TestReload_PreservesIdentity" -v -count=1
```

Expected: FAIL — `s.Reload` method does not exist.

- [ ] **Step 3: Implement `Reload` skeleton with all 18 loaders**

Append to `modules/world/reload.go`:

```go
import (
    "fmt"
    "path/filepath"

    "github.com/zsrv/goscape/pkg/cache"
)

// Reload re-loads all type-configs, scripts, CRCs, and preloaded client
// assets from cfg.CachePath. Mirrors TS World.reload at World.ts:206-292.
//
// Callers: (1) handleClientCheat ::reload (always clearInvs=true);
// (2) future friends-server inbound RELAY_RELOAD relay (clearInvs=false;
// TS World.ts:2036 — caller absent at NAI-190; signature preserves the
// parameter for the eventual wire-up).
//
// Runs synchronously on the tick goroutine (memory
// plan_race_tag_for_cross_goroutine_test); no locks acquired. Tick
// spike during cache reload matches TS's blocking-main-thread posture.
//
// DEVIATIONs:
//   - D1-GAMEMAP-RE-INJECT: step 11 re-injects loc/obj types into the
//     GameMap (TS reads package singletons; goscape goes through setters).
//   - D2-HALF-SWAP: post-step-3 mid-pipeline errors leave s.* partially
//     mutated. TS-parity (TS does not roll back). No rollback path.
//   - D3-CANDIDATE-VARSHARED-CLOBBER: see resizeVarShared.
//   - D4-NO-CATEGORYTYPES: goscape has no CategoryType loader (see
//     pkg/script/handlers_npc.go:105-110); TS L216 has no goscape
//     analog. Reload omits this loader.
//
// NAI-190.
func (s *Server) Reload(clearInvs bool) error {
    cachePath := s.cfg.CachePath
    serverDir := filepath.Join(cachePath, "server")

    // ─── Step 1: load pre-inv registries into locals ───
    varpTypes_, err := objtype.LoadVarpTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: varp types: %w", err)
    }
    params_, err := objtype.LoadParams(cachePath)
    if err != nil {
        return fmt.Errorf("reload: params: %w", err)
    }
    objTypes_, err := objtype.LoadObjTypes(cachePath, params_)
    if err != nil {
        return fmt.Errorf("reload: obj types: %w", err)
    }
    locTypes_, err := objtype.LoadLocTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: loc types: %w", err)
    }
    npcTypes_, err := objtype.LoadNPCTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: npc types: %w", err)
    }
    idkTypes_, err := objtype.LoadIdkTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: idk types: %w", err)
    }
    seqFrames_, err := objtype.LoadSeqFrames(cachePath)
    if err != nil {
        return fmt.Errorf("reload: seq frames: %w", err)
    }
    seqTypes_, err := objtype.LoadSeqTypes(cachePath, seqFrames_)
    if err != nil {
        return fmt.Errorf("reload: seq types: %w", err)
    }
    spotanim_, err := objtype.LoadSpotanimTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: spotanim types: %w", err)
    }
    // D4-NO-CATEGORYTYPES: TS L216 has no goscape analog. Skip.
    enumTypes_, err := objtype.LoadEnumTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: enum types: %w", err)
    }
    structTypes_, err := objtype.LoadStructTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: struct types: %w", err)
    }

    // ─── Step 2: load InvType ───
    invTypes_, err := objtype.LoadInvTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: inv types: %w", err)
    }

    // ─── Step 3: atomic swap of pre-inv registries ───
    s.varpTypes = varpTypes_
    s.paramTypes = params_
    s.objTypes = objTypes_
    s.locTypes = locTypes_
    s.npcTypes = npcTypes_
    s.idkTypes = idkTypes_
    s.seqTypes = seqTypes_
    s.spotanimTypes = spotanim_
    s.enumTypes = enumTypes_
    s.structTypes = structTypes_
    s.invTypes = invTypes_

    // ─── Step 4: clearInvs reconcile ───
    if clearInvs {
        s.invs = reconcileInvs(s.invs, s.players[:], s.invTypes)
    }

    // ─── Step 5: load post-inv configs ───
    mesanim_, err := objtype.LoadMesanimTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: mesanim types: %w", err)
    }
    dbTable_, err := objtype.LoadDbTableTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: dbtable types: %w", err)
    }
    dbRow_, err := objtype.LoadDbRowTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: dbrow types: %w", err)
    }
    huntTypes_, err := objtype.LoadHuntTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: hunt types: %w", err)
    }
    varnTypes_, err := objtype.LoadVarnTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: varn types: %w", err)
    }
    varsTypes_, err := objtype.LoadVarsTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: vars types: %w", err)
    }

    // ─── Step 6: swap post-inv registries ───
    s.mesanimTypes = mesanim_
    s.dbTableTypes = dbTable_
    s.dbRowTypes = dbRow_
    s.dbTableIndex = objtype.BuildDbTableIndex(dbTable_, dbRow_)
    s.huntTypes = huntTypes_
    s.varnTypes = varnTypes_
    s.varsTypes = varsTypes_

    // ─── Step 7: VarShared resize ───
    s.vars, s.varsStrings = resizeVarShared(s.vars, s.varsStrings, s.varsTypes.Configs)

    // ─── Step 8: load + swap Component ───
    componentTypes_, err := objtype.LoadComponentTypes(cachePath)
    if err != nil {
        return fmt.Errorf("reload: component types: %w", err)
    }
    s.componentTypes = componentTypes_

    // ─── Step 9: reload scripts + broadcast result ───
    // (Implemented in T5.)

    // ─── Step 10: CRC regen + client preload ───
    // (Implemented in T7.)

    // ─── Step 11: GameMap re-injection (D1) ───
    // (Implemented in T7.)

    return nil
}
```

- [ ] **Step 4: Verify GREEN for happy-path tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestReload_FreshLoad|TestReload_PreservesIdentity" -v -count=1
```

Expected: PASS. (TestReload_FreshLoad asserts only on the registries populated through Step 8; scripts / CRC / GameMap arms come in later tasks.)

- [ ] **Step 5: Run the full world package tests to confirm no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1
```

Expected: PASS for the entire package.

- [ ] **Step 6: Commit**

```bash
git add modules/world/reload.go modules/world/reload_test.go
git commit --no-gpg-sign -m "feat(world): NAI-190 T4 — Reload() entry point (steps 1-8 of pipeline)

Implements TS World.reload L207-270 — load all pre-inv and post-inv
registries, atomic-swap onto Server.*, run clearInvs reconcile via
T3 helper, run VarShared resize via T2 helper, load+swap component
types. Steps 9-11 (scripts broadcast, CRC, GameMap re-inject) land
in T5/T7. Tagged D1/D2/D3-CANDIDATE/D4 in the method doc-comment.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Scripts arm + broadcast

**Files:**
- Modify: `modules/world/reload.go` — step 9.
- Modify: `modules/world/reload_test.go` — broadcast capture tests.

- [ ] **Step 1: Add broadcast capture hook to `Server`**

Add a function-field on Server to allow tests to capture broadcasts without exercising the player-level `MessageGame` path (which requires a wired conn). Modify `modules/world/server.go`:

```go
// In Server struct, near the bridges block (~line 60):

// broadcastMesFunc is the broadcast sink for Server.BroadcastMes-style
// fanouts. Production wiring (nil) routes to BroadcastMes; tests
// override to capture without exercising the player connection layer.
// NAI-190.
broadcastMesFunc func(msg string)
```

In `modules/world/reload.go`, add a helper:

```go
// broadcast routes through the optional capture hook (test-injected)
// or falls back to Server.BroadcastMes (production).
func (s *Server) broadcast(msg string) {
    if s.broadcastMesFunc != nil {
        s.broadcastMesFunc(msg)
        return
    }
    s.BroadcastMes(msg)
}
```

- [ ] **Step 2: Write failing broadcast tests**

```go
func TestReload_ScriptCount_NodeDebug_SuccessBroadcast(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    s.cfg.NodeDebug = true
    var captured []string
    s.broadcastMesFunc = func(msg string) { captured = append(captured, msg) }
    if err := s.Reload(true); err != nil {
        t.Fatalf("Reload: %v", err)
    }
    if len(captured) == 0 {
        t.Fatal("expected broadcast on NodeDebug=true success path")
    }
    last := captured[len(captured)-1]
    if !strings.HasPrefix(last, "Loaded ") || !strings.HasSuffix(last, " scripts.") {
        t.Errorf("broadcast: got %q, want \"Loaded N scripts.\"", last)
    }
}

func TestReload_ScriptCount_NodeDebug_FailureBroadcast(t *testing.T) {
    s := newTestServerWithCachePath(t, t.TempDir()) // empty cache → script.Load fails
    s.cfg.NodeDebug = true
    var captured []string
    s.broadcastMesFunc = func(msg string) { captured = append(captured, msg) }
    // Reload may return error from earlier-step loaders (also missing); we don't care.
    _ = s.Reload(true)
    // Even on top-level Reload error, the scripts-arm test setup is
    // unreachable. Adjust: use a cache w/ partial files — only scripts
    // missing. Build that in newTestServerWithPartialCache below.
}

// To exercise the scripts-failure-only branch, we need a cache where
// every loader except scripts succeeds. Building this requires copying
// the real cache and removing only server/script.{dat,idx}.
func TestReload_ScriptCount_NodeDebug_FailureBroadcast_PartialCache(t *testing.T) {
    cacheDir := copyCacheExcept(t, realCacheDir(), "server/script.dat", "server/script.idx")
    s := newTestServerWithCachePath(t, cacheDir)
    s.cfg.NodeDebug = true
    s.scriptProvider = script.NewProvider()
    var captured []string
    s.broadcastMesFunc = func(msg string) { captured = append(captured, msg) }
    _ = s.Reload(true) // earlier loaders succeed; only scripts.Load fails
    if len(captured) == 0 {
        t.Fatal("expected broadcast on NodeDebug=true script-failure path")
    }
    last := captured[len(captured)-1]
    if last != "There was an issue while reloading scripts." {
        t.Errorf("broadcast: got %q, want failure message", last)
    }
}

func TestReload_NotNodeDebug_DoesNotBroadcast(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    s.cfg.NodeDebug = false
    var captured []string
    s.broadcastMesFunc = func(msg string) { captured = append(captured, msg) }
    if err := s.Reload(true); err != nil {
        t.Fatalf("Reload: %v", err)
    }
    if len(captured) != 0 {
        t.Errorf("NodeDebug=false should not broadcast; got %v", captured)
    }
}

// copyCacheExcept copies all files from src to a t.TempDir, OMITTING
// the listed relative paths. Used to simulate partial-cache failure
// modes without polluting the real cache.
func copyCacheExcept(t *testing.T, src string, omit ...string) string {
    t.Helper()
    dst := t.TempDir()
    omitSet := make(map[string]bool)
    for _, p := range omit {
        omitSet[p] = true
    }
    err := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
        if walkErr != nil {
            return walkErr
        }
        rel, _ := filepath.Rel(src, path)
        if omitSet[rel] {
            return nil
        }
        target := filepath.Join(dst, rel)
        if info.IsDir() {
            return os.MkdirAll(target, 0o755)
        }
        data, err := os.ReadFile(path)
        if err != nil {
            return err
        }
        return os.WriteFile(target, data, 0o644)
    })
    if err != nil {
        t.Fatalf("copyCacheExcept: %v", err)
    }
    return dst
}
```

- [ ] **Step 3: Verify RED**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestReload_ScriptCount|TestReload_NotNodeDebug" -v -count=1
```

Expected: FAIL (Reload() step 9 not implemented; broadcasts not emitted).

- [ ] **Step 4: Implement step 9 in `Reload()`**

Replace the `// ─── Step 9: ───` comment in `modules/world/reload.go` with:

```go
    // ─── Step 9: reload scripts + broadcast result ───
    if s.scriptProvider == nil {
        s.scriptProvider = script.NewProvider()
    }
    count, scriptErr := s.scriptProvider.Load(serverDir)
    if s.cfg.NodeDebug {
        if scriptErr != nil {
            s.broadcast("There was an issue while reloading scripts.")
        } else {
            s.broadcast(fmt.Sprintf("Loaded %d scripts.", count))
        }
    } else {
        if scriptErr != nil {
            s.log.Error("script reload failed", "err", scriptErr)
        } else {
            s.log.Debug("scripts reloaded", "count", count)
        }
    }
```

Add the `"github.com/zsrv/goscape/pkg/script"` import to reload.go.

- [ ] **Step 5: Verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestReload_ScriptCount|TestReload_NotNodeDebug|TestReload_FreshLoad|TestReload_PreservesIdentity" -v -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/reload.go modules/world/reload_test.go modules/world/server.go
git commit --no-gpg-sign -m "feat(world): NAI-190 T5 — Reload step 9 (scripts arm + broadcast)

Implements TS World.reload L272-285. NodeDebug=true routes the
load-result message through broadcast (success: 'Loaded N scripts.';
failure: 'There was an issue while reloading scripts.'); NodeDebug=
false logs via slog instead. Added Server.broadcastMesFunc capture
hook for tests; production callers route through BroadcastMes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: VarShared resize + Inv reconcile integration tests

The helpers are already unit-tested in T2/T3. This task pins the wiring through `Reload()` end-to-end.

**Files:**
- Modify: `modules/world/reload_test.go`.

- [ ] **Step 1: Add integration tests**

```go
func TestReload_ClearInvsTrue_RebuildsSharedInvs(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    sentinel := &inventory.Inventory{}
    s.invs = map[int]*inventory.Inventory{0xDEAD: sentinel}
    if err := s.Reload(true); err != nil {
        t.Fatalf("Reload: %v", err)
    }
    if _, leaked := s.invs[0xDEAD]; leaked {
        t.Errorf("sentinel at id 0xDEAD leaked through clearInvs=true")
    }
    // Find any SCOPE_SHARED id in the real cache and assert it's populated.
    sharedFound := false
    for id, inv := range s.invTypes.Configs {
        if inv == nil || inv.Scope != objtype.InvTypeScopeShared {
            continue
        }
        if s.invs[id] == nil {
            t.Errorf("SHARED inv id %d not populated post-reload", id)
        }
        sharedFound = true
        break
    }
    if !sharedFound {
        t.Skip("no SCOPE_SHARED inv in real cache; cannot pin")
    }
}

func TestReload_ClearInvsFalse_LeavesInvsUntouched(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    sentinel := &inventory.Inventory{}
    s.invs = map[int]*inventory.Inventory{42: sentinel}
    if err := s.Reload(false); err != nil {
        t.Fatalf("Reload: %v", err)
    }
    if s.invs[42] != sentinel {
        t.Errorf("clearInvs=false should leave existing invs untouched")
    }
}

func TestReload_ClearInvsTrue_DeletesTempScopeFromPlayer(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    // Pick any SCOPE_TEMP id from the real cache (run Reload first to populate invTypes).
    if err := s.Reload(false); err != nil {
        t.Fatalf("priming reload: %v", err)
    }
    tempID := -1
    for id, inv := range s.invTypes.Configs {
        if inv != nil && inv.Scope == objtype.InvTypeScopeTemp {
            tempID = id
            break
        }
    }
    if tempID < 0 {
        t.Skip("no SCOPE_TEMP inv in real cache")
    }
    p := &Player{invs: map[int]*inventory.Inventory{tempID: &inventory.Inventory{}}}
    s.players[1] = p
    if err := s.Reload(true); err != nil {
        t.Fatalf("Reload: %v", err)
    }
    if _, ok := p.invs[tempID]; ok {
        t.Errorf("SCOPE_TEMP inv id %d not deleted from player.invs", tempID)
    }
}

func TestReload_VarSharedCountUnchanged_PreservesValues(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    if err := s.Reload(true); err != nil {
        t.Fatalf("priming reload: %v", err)
    }
    if len(s.vars) == 0 {
        t.Skip("no vars in real cache")
    }
    // Find a STRING-typed slot if any (those survive the clobber).
    stringSlot := -1
    for i, v := range s.varsTypes.Configs {
        if v != nil && v.Type == objtype.ScriptVarTypeString {
            stringSlot = i
            break
        }
    }
    if stringSlot < 0 {
        t.Skip("no STRING vars in real cache; covered by unit test instead")
    }
    s.varsStrings[stringSlot] = "marker"
    if err := s.Reload(true); err != nil {
        t.Fatalf("Reload: %v", err)
    }
    if s.varsStrings[stringSlot] != "marker" {
        t.Errorf("STRING var %d was clobbered: %q", stringSlot, s.varsStrings[stringSlot])
    }
}
```

- [ ] **Step 2: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestReload_ClearInvs|TestReload_VarShared" -v -count=1
```

Expected: PASS (helpers already wired via T4). Skips on cache-content gaps are acceptable; covered by unit tests in T2/T3.

- [ ] **Step 3: Commit**

```bash
git add modules/world/reload_test.go
git commit --no-gpg-sign -m "test(world): NAI-190 T6 — Reload var-resize + inv-reconcile integration

Pins the T4 wiring of T2/T3 helpers through Reload() against the real
data/pack cache. SHARED inv rebuild, TEMP inv player-delete,
clearInvs=false untouched, STRING-slot survives count-unchanged
resize. Skip-on-empty-cache for content-dependent variants is
TS-parity-acceptable; unit tests in T2/T3 cover the deterministic shapes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: CRC regen + client preload + GameMap re-injection (steps 10-11)

**Files:**
- Modify: `modules/world/reload.go` — steps 10, 11.
- Modify: `modules/world/reload_test.go` — CRC + GameMap tests.

- [ ] **Step 1: Write failing tests**

```go
import (
    "github.com/zsrv/goscape/pkg/cache"
    "github.com/zsrv/goscape/pkg/gamemap"
)

func TestReload_CRCRegen_OverwritesGlobalCrcBuffer(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    cache.ResetCRCState()
    cache.CrcBuffer32 = 0xDEAD
    if err := s.Reload(true); err != nil {
        t.Fatalf("Reload: %v", err)
    }
    if cache.CrcBuffer32 == 0xDEAD {
        t.Errorf("CrcBuffer32 not regenerated post-reload")
    }
    if len(cache.CrcTable) == 0 {
        t.Errorf("CrcTable empty post-reload")
    }
}

func TestReload_GameMapTypesReInjected(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    // Construct a minimal GameMap (use the real gamemap.New + minimal init).
    gm := gamemap.New(s.log)
    s.gamemap = gm
    if err := s.Reload(true); err != nil {
        t.Fatalf("Reload: %v", err)
    }
    // Expose injected loctypes via a getter for the test (add one if absent).
    if gm.LocTypesForTest() != s.locTypes {
        t.Errorf("GameMap loc types not re-injected post-reload (DEVIATION-NAI-190-D1)")
    }
    if gm.ObjTypesForTest() != s.objTypes {
        t.Errorf("GameMap obj types not re-injected post-reload (DEVIATION-NAI-190-D1)")
    }
}
```

If `LocTypesForTest()` and `ObjTypesForTest()` getters don't exist on `gamemap.GameMap`, add them (one-liner each, returns the stored ref). Per memory `test_export_underscore_test_visibility`, use plain `.go` with `*ForTest` suffix if cross-package visibility is needed.

- [ ] **Step 2: Verify RED**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestReload_CRCRegen|TestReload_GameMap" -v -count=1
```

Expected: FAIL — steps 10-11 not yet implemented.

- [ ] **Step 3: Implement steps 10-11**

Replace `// ─── Step 10: ───` and `// ─── Step 11: ───` in `Reload()` with:

```go
    // ─── Step 10: CRC regen + client preload (TS L288, L291) ───
    cache.MakeCRCs()
    clientDir := filepath.Join(cachePath, "client")
    if err := cache.PreloadClient(clientDir); err != nil {
        // TS preloadClient throws on error; goscape returns. Per
        // DEVIATION-NAI-190-D2-HALF-SWAP, the post-step-3 swap is
        // already committed.
        return fmt.Errorf("reload: preload client: %w", err)
    }

    // ─── Step 11: GameMap re-injection (DEVIATION-NAI-190-D1) ───
    if s.gamemap != nil {
        s.gamemap.SetLocTypes(s.locTypes)
        s.gamemap.SetObjTypes(s.objTypes)
        s.gamemap.SetMembers(s.cfg.NodeMembers)
    }

    return nil
```

- [ ] **Step 4: Add getters in `pkg/gamemap/` if absent**

Verify `LocTypesForTest` / `ObjTypesForTest`:

```bash
grep -n "LocTypesForTest\|ObjTypesForTest" $HOME/Code/github.com/zsrv/goscape/pkg/gamemap/*.go
```

If absent, append to the gamemap getter file (or `gamemap.go` near `SetLocTypes`):

```go
// LocTypesForTest exposes the injected loctypes ref for cross-package
// tests. Production callers use SetLocTypes-injected refs internally.
// NAI-190 T7 (GameMap re-injection pin).
func (m *GameMap) LocTypesForTest() *objtype.LocTypeConfigs { return m.locTypes }
func (m *GameMap) ObjTypesForTest() *objtype.ObjTypeConfigs { return m.objTypes }
```

- [ ] **Step 5: Verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestReload" -v -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS. Full repo test sweep ensures no cross-package regression from the gamemap getter additions.

- [ ] **Step 6: Commit**

```bash
git add modules/world/reload.go modules/world/reload_test.go pkg/gamemap/
git commit --no-gpg-sign -m "feat(world): NAI-190 T7 — Reload steps 10-11 (CRC + preload + GameMap re-inject)

Implements TS World.reload L288-291 (makeCrcs + preloadClient) and
DEVIATION-NAI-190-D1-GAMEMAP-RE-INJECT (push s.locTypes/objTypes/
NodeMembers back into GameMap's internal refs). Added gamemap
*ForTest getters per memory test_export_underscore_test_visibility.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Loader-error path + half-swap skip-with-pin

**Files:**
- Modify: `modules/world/reload_test.go`.

- [ ] **Step 1: Add the no-mutation test**

```go
func TestReload_PreStep3LoaderError_LeavesRegistriesUnmutated(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    if err := s.Reload(true); err != nil {
        t.Fatalf("priming reload: %v", err)
    }
    objBefore := s.objTypes
    locBefore := s.locTypes

    // Point at an empty tempdir so the FIRST loader (LoadVarpTypes) fails.
    s.cfg.CachePath = t.TempDir()
    err := s.Reload(true)
    if err == nil {
        t.Fatal("expected error from missing varp.dat")
    }
    if s.objTypes != objBefore {
        t.Errorf("objTypes mutated despite pre-step-3 error (DEVIATION-NAI-190-D2 contract violated)")
    }
    if s.locTypes != locBefore {
        t.Errorf("locTypes mutated despite pre-step-3 error")
    }
}
```

- [ ] **Step 2: Add the half-swap skip-with-pin**

```go
func TestReload_MidPipelineLoaderError_LeavesHalfSwapped_SkipPin(t *testing.T) {
    s := newTestServerWithCachePath(t, realCacheDir())
    if err := s.Reload(true); err != nil {
        t.Fatalf("priming reload: %v", err)
    }
    objBefore := s.objTypes

    // Construct a partial cache: copy real cache MINUS server/mesanim.dat
    // (step 5 loader). Reload will succeed through step 3 (swap) and fail
    // at step 5. objTypes must be the NEW instance (mutated); locTypes
    // also new. This documents DEVIATION-NAI-190-D2-HALF-SWAP.
    cacheDir := copyCacheExcept(t, realCacheDir(), "server/mesanim.dat", "server/mesanim.idx")
    s.cfg.CachePath = cacheDir
    err := s.Reload(true)
    if err == nil {
        t.Fatal("expected mid-pipeline error")
    }
    // Per memory skip_pin_full_struct_capture: capture verbatim, not inferred.
    t.Logf("DEVIATION-NAI-190-D2-HALF-SWAP captured state post-error:\n"+
        "  s.objTypes pointer changed? %v\n"+
        "  err: %v",
        s.objTypes != objBefore, err)
    if s.objTypes == objBefore {
        t.Errorf("expected post-step-3 swap to have taken effect before step-5 failure")
    }
    t.Skip("DEVIATION-NAI-190-D2-HALF-SWAP: half-swap is the documented contract; this test pins the observed shape but does not enforce it across future refactors.")
}
```

- [ ] **Step 3: Run + verify**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestReload_PreStep3LoaderError|TestReload_MidPipelineLoaderError" -v -count=1
```

Expected: `TestReload_PreStep3` PASS; `TestReload_MidPipeline` SKIP (with `t.Logf` snapshot visible).

- [ ] **Step 4: Commit**

```bash
git add modules/world/reload_test.go
git commit --no-gpg-sign -m "test(world): NAI-190 T8 — Reload loader-error contracts

Pins the pre-step-3 no-mutation contract (positive control) and the
post-step-3 half-swap contract via skip-with-pin (DEVIATION-NAI-190-
D2-HALF-SWAP). Per memory skip_pin_full_struct_capture, the skip
test uses t.Logf to capture observed state verbatim.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Cheat wiring + integration tests

**Files:**
- Modify: `modules/world/handlers_game.go:585-589` (the `switch parts[0]` block in the dev-block).
- Modify: `modules/world/reload_test.go` — cheat-level tests.

- [ ] **Step 1: Write failing cheat-level tests**

```go
func TestHandleClientCheat_Reload_Dispatches(t *testing.T) {
    // Use existing newTestPlayer + wire its server to a real cache.
    p, _ := newTestPlayer(t)
    s := p.client.server
    s.cfg.CachePath = realCacheDir()
    s.cfg.NodeDebug = true
    s.cfg.NodeProduction = false
    p.staffModLevel = 4
    var captured []string
    s.broadcastMesFunc = func(msg string) { captured = append(captured, msg) }

    // Build the CLIENT_CHEAT packet payload: 1 byte unused + GJStrLF("::reload").
    // The dev-block strips the "::" so the dispatch sees "reload".
    payload := buildClientCheatPayload(t, "::reload")
    if err := handleClientCheat(p, payload); err != nil {
        t.Fatalf("handleClientCheat: %v", err)
    }
    if len(captured) == 0 {
        t.Fatal("::reload did not broadcast (cheat not wired or Reload failed)")
    }
    if !strings.HasPrefix(captured[len(captured)-1], "Loaded ") {
        t.Errorf("expected success broadcast; got %q", captured[len(captured)-1])
    }
}

func TestHandleClientCheat_Reload_ErrorPath_LogsAndPrivateMes(t *testing.T) {
    p, conn := newTestPlayer(t)
    s := p.client.server
    s.cfg.CachePath = t.TempDir() // empty → loaders fail
    s.cfg.NodeDebug = true
    s.cfg.NodeProduction = false
    p.staffModLevel = 4

    payload := buildClientCheatPayload(t, "::reload")
    err := handleClientCheat(p, payload)
    if err != nil {
        t.Fatalf("handleClientCheat returned err (should be swallowed): %v", err)
    }
    // Assert the player got a MessageGame("Reload failed: ..."). Read from
    // the conn fixture.
    out := drainConnString(t, conn)
    if !strings.Contains(out, "Reload failed") {
        t.Errorf("expected private 'Reload failed' message; got %q", out)
    }
}

func TestHandleClientCheat_Reload_DefaultsClearInvsTrue(t *testing.T) {
    p, _ := newTestPlayer(t)
    s := p.client.server
    s.cfg.CachePath = realCacheDir()
    s.cfg.NodeProduction = false
    p.staffModLevel = 4
    sentinel := &inventory.Inventory{}
    s.invs = map[int]*inventory.Inventory{0xCAFE: sentinel}

    payload := buildClientCheatPayload(t, "::reload")
    if err := handleClientCheat(p, payload); err != nil {
        t.Fatalf("handleClientCheat: %v", err)
    }
    if _, leaked := s.invs[0xCAFE]; leaked {
        t.Errorf("::reload should default clearInvs=true (sentinel leaked)")
    }
}
```

Helper `buildClientCheatPayload` mirrors the format expected by `handleClientCheat` (1 byte unused + GJStrLF). Look at existing cheat tests in `handler_cheats_supermod_test.go` for the exact payload-build helper — likely already exists as `newCheatPayload` or similar; **plan-author note:** re-grep before writing; reuse if present.

Helper `drainConnString` reads from the test conn and returns the bytes as a string — almost certainly already exists in test helpers.

- [ ] **Step 2: Verify RED**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleClientCheat_Reload" -v -count=1
```

Expected: FAIL — the `"reload"` case does not yet exist in the switch.

- [ ] **Step 3: Wire the cheat**

In `modules/world/handlers_game.go`, locate the dev-block switch (the `case` arms following the debugproc prefix branch around L589). Add a new arm:

```go
        case "reload":
            // TS ClientCheatHandler.ts:149-150 — World.reload() default
            // clearInvs=true. NAI-190.
            if err := p.client.server.Reload(true); err != nil {
                // TS dispatches via try/catch on uncaught throws; goscape
                // surfaces explicitly. DEVIATION-NAI-190-D2-HALF-SWAP
                // documents the half-swap risk on post-step-3 errors.
                p.client.server.log.Error("reload cheat failed", "err", err)
                p.MessageGame("Reload failed: see server log.")
            }
            return nil
```

- [ ] **Step 4: Verify GREEN**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleClientCheat_Reload" -v -count=1
```

Expected: PASS.

- [ ] **Step 5: Run full repo + race**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1
```

Expected: All PASS, no race warnings.

- [ ] **Step 6: Commit**

```bash
git add modules/world/handlers_game.go modules/world/reload_test.go
git commit --no-gpg-sign -m "feat(world): NAI-190 T9 — ::reload cheat wiring

Adds the 'reload' arm to the dev-block fixed-cmd switch. Routes to
(*Server).Reload(true), surfacing errors via slog.Error + private
MessageGame to the issuing player. Three integration tests pin
dispatch, error-path message, and clearInvs=true default. -race
sweep clean.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Carryforward rewrite + DEVIATION tags

**Files:**
- Modify: `modules/world/handlers_game.go:539-547` (carryforward block).

- [ ] **Step 1: Rewrite the carryforward block**

Replace lines 539-547 (the DEVIATION-NAI-189-D1-CARRYFORWARD block) with:

```go
        // DEVIATION-NAI-190-D5-CARRYFORWARD — supersedes
        // DEVIATION-NAI-189-D1-CARRYFORWARD. 1 TS ClientCheatHandler
        // cheat remains unported, blocked on the cache-compiler arc:
        //   rebuild: TS L151-153. Calls World.rebuild() — posts
        //            'world_rebuild' to a DevThread worker that runs
        //            packAll() (TS tools/pack/PackAll.ts; ~8200 LOC).
        //            Blocked on NAI-191..NAI-204 — the staged port of
        //            tools/pack/ (config compilers + RuneScript .rs2
        //            compiler + fsnotify watcher).
        // NAI-190 retired ::reload (TS L149-150). World.reload() is
        // ported as (*Server).Reload(clearInvs bool) error in
        // modules/world/reload.go. Three DEVIATION tags live in the
        // method doc-comment:
        //   D1-GAMEMAP-RE-INJECT — glue-only: TS reads package
        //     singletons, goscape re-injects loc/obj types into the
        //     GameMap struct via SetLocTypes/SetObjTypes.
        //   D2-HALF-SWAP — TS-parity: TS does not roll back on
        //     mid-pipeline errors. Post-step-3 errors leave s.*
        //     partially mutated. Pinned via skip-with-pin in
        //     reload_test.go.
        //   D3-CANDIDATE-VARSHARED-CLOBBER — TS L259-267 clobbers
        //     copied values; mirrored verbatim per true-to-TS gate.
        //   D4-NO-CATEGORYTYPES — goscape has no CategoryType loader;
        //     TS L216 has no analog.
        // NAI-189 retired DEBUGPROC dispatch (TS L59-148). See
        // dispatchDebugproc/marshalDebugprocArgs/parseDebugprocCoord
        // (DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE).
        // NAI-188 retired ::speed (TS L154-167). The tickRate
        // package-level const at modules/world/tick.go:15 was promoted
        // to Server.tickRate.
        // NAI-187 retired the admin spawn/interface cluster (locadd /
        // npcadd / openmain).
```

- [ ] **Step 2: Verify the file compiles + tests still pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1
```

Expected: PASS.

- [ ] **Step 3: Verify deviation-tag grep enumeration**

Per memory `retire_deviation_grep_all_comments`, enumerate all references to the retired tag:

```bash
rg "NAI-189-D1-CARRYFORWARD" --type=go .
```

Expected: zero hits (the carryforward block was the only place this tag lived; the MIRROR-TS-COORD-FRAGILE tag is unrelated and remains).

```bash
rg "NAI-190" --type=go .
```

Expected: all references in the new reload.go, reload_test.go, and the rewritten carryforward block.

- [ ] **Step 4: Commit**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "docs(world): NAI-190 T10 — carryforward rewrite

DEVIATION-NAI-190-D5-CARRYFORWARD supersedes the NAI-189 carryforward.
::reload retirement paragraph added with D1/D2/D3/D4 tag summary; the
remaining unported cheat (::rebuild) is attributed to the NAI-191..
NAI-204 arc (cache-compiler + fsnotify worker port). Tally is now 1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Close commit

**Files:** No code changes; commit-only.

- [ ] **Step 1: Verify the full test sweep**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all PASS; no vet warnings.

- [ ] **Step 2: Verify no orphan TODOs introduced**

```bash
rg "TODO\b|FIXME\b|XXX\b" modules/world/reload.go modules/world/reload_test.go
```

Expected: zero hits.

- [ ] **Step 3: Verify deviation tags codified**

```bash
rg "DEVIATION-NAI-190-D[1-5]" --type=go .
```

Expected: D1, D2, D3-CANDIDATE, D4, D5-CARRYFORWARD all present at their respective sites:
- D1: `modules/world/reload.go` (method doc + step 11 comment)
- D2: `modules/world/reload.go` (method doc) + `modules/world/reload_test.go` (skip-pin)
- D3-CANDIDATE: `modules/world/reload.go` (resizeVarShared doc) + reload_test.go
- D4: `modules/world/reload.go` (method doc + step 1 comment)
- D5-CARRYFORWARD: `modules/world/handlers_game.go`

- [ ] **Step 4: Create the close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-190 — World.reload() port + ::reload cheat

Closes the World.reload() sub-spec (TS World.ts:206-292). Goscape now
provides (*Server).Reload(clearInvs bool) error which re-loads all
~18 type-config registries, atomically swaps onto Server.*, reconciles
VarShared resize and inv state, reloads scripts (via the new
Provider.Load (int, error) signature), regenerates CRCs, re-runs
PreloadClient, and re-injects loc/obj types into the GameMap (D1).
The ::reload cheat in the dev-block fixed-cmd switch routes to
Reload(true) per TS default; error paths surface via slog.Error +
private MessageGame to the issuing player.

After NAI-190, ClientCheatHandler.ts is 100% ported except for
::rebuild (TS L151-153) — blocked on the NAI-191..NAI-204 arc that
ports tools/pack/ (config compilers + RuneScript .rs2 compiler +
fsnotify watcher). Carryforward tally is now 1.

Sub-spec bundle:
- T1: Provider.Load → (count int, err error). Single non-test call
  site updated.
- T2: resizeVarShared pure helper. Mirrors TS L246-268 verbatim
  (DEVIATION-NAI-190-D3-CANDIDATE-VARSHARED-CLOBBER documents the
  copy-then-clobber pattern; mirror per true-to-TS gate).
- T3: reconcileInvs pure helper. SHARED rebuild, TEMP delete, PERM
  untouched (TS-parity).
- T4: Reload() entry point — steps 1-8 (load all pre/post-inv
  registries, atomic swap, clearInvs reconcile, vars resize, component
  load).
- T5: Step 9 (scripts arm + broadcast). Server.broadcastMesFunc
  capture hook added for tests; production routes through
  BroadcastMes.
- T6: Integration tests pinning T2/T3 helpers through Reload() end-
  to-end.
- T7: Steps 10-11 (CRC regen, PreloadClient, GameMap re-injection).
  Gamemap *ForTest getters added per memory
  test_export_underscore_test_visibility.
- T8: Pre-step-3 no-mutation positive control + post-step-3 half-swap
  skip-with-pin (DEVIATION-NAI-190-D2-HALF-SWAP).
- T9: ::reload cheat wiring (dispatch, error-path private mes,
  clearInvs=true default).
- T10: Carryforward rewrite — DEVIATION-NAI-190-D5-CARRYFORWARD
  supersedes NAI-189's; ::reload retirement paragraph added.

Closes memory: plan_race_tag_for_cross_goroutine_test, tracker_carryforward_listings_compound, true_to_ts_gate, skip_pin_full_struct_capture

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If the close commit is non-empty (some final docs touch-up), drop `--allow-empty`.

---

## §3 Self-review

Per writing-plans skill self-review:

**1. Spec coverage:** Each spec §13 acceptance criterion maps to a task:
- (1) `Reload(clearInvs bool) error` in reload.go → T4 (entry point) + T5/T7 (steps 9-11)
- (2) `::reload` cheat wired → T9
- (3) `s.broadcastMes` reusable → T5 (broadcastMesFunc hook layered on existing BroadcastMes)
- (4) Provider.Load signature change → T1
- (5) all tests + skip-pin → T2-T9
- (6) carryforward rewrite + tally=1 → T10
- (7) D1, D2 tags land; D3-CANDIDATE either confirmed or retired → T4 (D1, D2, D4 doc-comments); T2 (D3-CANDIDATE); resolution to retire-or-promote happens in the NAI-190 review pass before close, NOT here. Plan-author note: leaving D3-CANDIDATE in CANDIDATE state through the close commit is acceptable if the reviewer hasn't concluded; document the deferred resolution as a NAI-190 follow-up.
- (8) -race sweep PASS → T9 step 5 + T11 step 1
- (9) Closes memory trailer → T11 step 4

**2. Placeholder scan:** No "TBD" / "TODO" / "Add appropriate error handling" / "Similar to Task N" placeholders. The `plan-author note:` callouts about re-grepping for `newCheatPayload` and `drainConnString` are direct re-verification asks at task-execution time, not deferrals.

**3. Type consistency:**
- `resizeVarShared(oldVars []int32, oldStrs []string, newConfigs []*objtype.VarSharedType) (newVars []int32, newStrs []string)` — same in T2 and T4 (wiring).
- `reconcileInvs(serverInvs map[int]*inventory.Inventory, players []*Player, invTypes *objtype.InvTypeConfigs) map[int]*inventory.Inventory` — same in T3 and T4 (`s.invs = reconcileInvs(s.invs, s.players[:], s.invTypes)`). Note T4 passes `s.players[:]` to convert the `[2048]*Player` array to a slice — verified.
- `(*Provider).Load(cacheDir string) (int, error)` — same in T1, T5.
- `(*Server).Reload(clearInvs bool) error` — same in T4, T9, spec.
- `broadcastMesFunc func(msg string)` — same in T5 (declare + use).

**4. Untested-helpers check:** None — every helper introduced (T1's Load signature, T2's resizeVarShared, T3's reconcileInvs, T5's broadcastMesFunc, T7's *ForTest getters) has at least one test in the same or an immediately-following task.

**5. Memory cross-references:** Plan invokes nine memory entries explicitly: `parallel_adapter_init_duplication` (T1 step 1), `plan_red_phase_prediction_old_sut` (T2 step 3), `plan_race_tag_for_cross_goroutine_test` (T3, T4 doc), `test_export_underscore_test_visibility` (T7), `skip_pin_full_struct_capture` (T8), `retire_deviation_grep_all_comments` (T10), `tracker_carryforward_listings_compound` (T10 close), `true_to_ts_gate` (T2 doc, T4 doc, close), `close_commit_memory_trailer` (T11). All applicable.

No issues found; plan ready for execution.

---

**Plan complete.** Eleven tasks, each with RED → GREEN → COMMIT structure. Spec at `docs/superpowers/specs/2026-05-13-nai-190-world-reload-design.md`; HEAD `27e503a`.

Per memory `execution_mode_default` and `superpowers_clear_between_spec_and_impl`, the resume prompt for the post-`/clear` implementer session is:

> Implement NAI-190 per the plan at `docs/superpowers/plans/2026-05-13-nai-190-world-reload.md` (spec: `docs/superpowers/specs/2026-05-13-nai-190-world-reload-design.md`). Use the `superpowers:subagent-driven-development` skill. Dispatch one subagent per task; two-stage review between tasks; production-quality only.
