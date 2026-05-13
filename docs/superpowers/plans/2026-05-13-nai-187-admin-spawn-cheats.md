# NAI-187 — Admin spawn/interface cheats Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `::locadd`, `::npcadd`, `::openmain` from TS `ClientCheatHandler.ts` into goscape's admin-block cheat switch in `modules/world/handlers_game.go`. Closes the admin-tier portion of `DEVIATION-NAI-186-D2-CARRYFORWARD`.

**Architecture:** Two layers. (1) `pkg/objtype/` — three sibling `ByName` helpers on `LocTypeConfigs`, `NPCTypeConfigs`, `ComponentTypeConfigs` mirroring the established `VarpTypeConfigs.ByName` (`pkg/objtype/varptype.go:120`) / `ObjTypeConfigs.ByName` (`pkg/objtype/objtype.go:76`) pattern over the already-populated `ConfigNames map[string]int` index. (2) `modules/world/handlers_game.go` — three new `case` arms inline in the existing `if p.staffModLevel >= 3 { switch parts[0] { ... } }` block at line 428. All entity construction / registration primitives (`s.AddLoc`, `world.NewNpc` + `s.addNpc`, `p.OpenMain`) already exist.

**Tech Stack:** Go 1.26+. Test runner: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`. Commit signing disabled (`git commit --no-gpg-sign`).

**Spec:** `docs/superpowers/specs/2026-05-13-nai-187-admin-spawn-cheats-design.md`

---

## File Structure

**Created files** (none new — all changes append to existing files):

**Modified files:**
- `pkg/objtype/loctype.go` — append `(*LocTypeConfigs).ByName` method.
- `pkg/objtype/npctype.go` — append `(*NPCTypeConfigs).ByName` method.
- `pkg/objtype/componenttype.go` — append `(*ComponentTypeConfigs).ByName` method.
- `pkg/objtype/loctype_test.go` — append five `TestLocTypeConfigs_ByName_*` tests.
- `pkg/objtype/npctype_test.go` — append five `TestNPCTypeConfigs_ByName_*` tests.
- `pkg/objtype/componenttype_test.go` — append five `TestComponentTypeConfigs_ByName_*` tests.
- `modules/world/handlers_game.go` — add `case "locadd"`, `case "npcadd"`, `case "openmain"` arms inside the `>= 3` switch; add imports `entitypkg "github.com/zsrv/goscape/pkg/entity"` and `"github.com/zsrv/goscape/pkg/pathfinder/loc"`; rewrite the `DEVIATION-NAI-186-D2-CARRYFORWARD` block (CLOSE task).
- `modules/world/handlers_game_test.go` — append per-cheat dispatch tests (T4, T5, T6) and a combined gate test (T7).

---

## Pre-flight expectations for implementers

Per memory `controller_preflight` + `plan_runnable_test_fixtures` + `plan_red_phase_prediction_old_sut`, the controller (subagent-driven-development orchestrator) will re-grep the following premises at HEAD before each implementer dispatch. If any have drifted, halt and update the plan:

- `(*VarpTypeConfigs).ByName` exists at `pkg/objtype/varptype.go:120` with the five-step nil-receiver-guard / index-hit / bounds-check / linear-scan-fallback / nil-return shape.
- `LocTypeConfigs.ConfigNames`, `NPCTypeConfigs.ConfigNames`, `ComponentTypeConfigs.ConfigNames` all exist as `map[string]int` populated at load time (`pkg/objtype/loctype.go:200`, `npctype.go:344`, `componenttype.go:120`).
- `entity.NewLoc(level, x, z, width, length int, lc Lifecycle, typ, shape, angle int) *Loc` exists at `pkg/entity/loc.go:23`.
- `loc.ShapeCentrepieceStraight` exists at `pkg/pathfinder/loc/shape.go:16`.
- `entity.LifecycleDespawn` exists in `pkg/entity/lifecycle.go`.
- `(*Server).AddLoc(*entity.Loc, dur int)` exists at `modules/world/world_zone.go:17`.
- `world.NewNpc(nid, typeId, x, z, level int, typ *objtype.NpcType) *Npc` exists at `modules/world/npc.go:159`. **Panics on `typ == nil`** (reads `typ.RespawnRate` etc.) — the cheat must filter `nt == nil` before calling.
- `(*Server).addNpc(n *Npc, duration int, firstSpawn bool) error` exists at `modules/world/npc_registry.go:48`. Returns `errNpcsFull` when slots are exhausted.
- `NpcLifecycleDespawn = 2` exists at `modules/world/npc.go:15`.
- `(*Player).OpenMain(com int)` exists at `modules/world/player_script.go:943`. Sets `p.modalMain`, clears `p.modalChat` and `p.modalSide`, sets `p.modalState = modalStateMain`, sets `p.refreshModal = true`.
- `teleTestPlayer(t)` returns `(*Player, net.Conn, *Server)` at `modules/world/handlers_game_test.go:364`. The returned server has `gamemap`, `zoneMap`, `rsbuf`, `locObjTracker`, and `players[1]=p` wired; player is at `(3094, 3106, 0)`.
- `dispatchTeleCheat(t, p, "<cmd> <args>")` at `modules/world/handlers_game_test.go:392` builds the wire packet and calls `handleClientCheat(p, payload)`.

Per memory `plan_sibling_site_guard_audit`: the new cheat-side code uses `p.client.server.locTypes` / `npcTypes` / `componentTypes` without nil-guarding because `p.staffModLevel >= 3` is an outer guard that implies a fully-initialised production server. This matches the established pattern in the same admin switch (`give` at line 493: bare `p.client.server.objTypes.ByName(sub[0])` with no nil-guard; `setvar` at line 587: same).

---

## Task 1: `(*LocTypeConfigs).ByName` helper

**Files:**
- Modify: `pkg/objtype/loctype.go` (append at end of file)
- Test: `pkg/objtype/loctype_test.go` (append at end of file)

**Parallel-eligible:** Yes — runs concurrently with T2 and T3 (no shared code paths).

- [ ] **Step 1: Write the five failing tests**

Append to `pkg/objtype/loctype_test.go`:

```go
func TestLocTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	lc := &LocTypeConfigs{
		Configs:     []*LocType{{ConfigType: ConfigType{ID: 0, DebugName: "first"}}, {ConfigType: ConfigType{ID: 1, DebugName: "second"}}},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := lc.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestLocTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	lc := &LocTypeConfigs{
		Configs:     []*LocType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := lc.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestLocTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var lc *LocTypeConfigs
	if got := lc.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestLocTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	// ConfigNames points "fresh" at id=5 but Configs is only length 2.
	// Lookup must NOT panic and must fall through to the linear scan,
	// which finds "fresh" at id=1 by DebugName equality.
	lc := &LocTypeConfigs{
		Configs:     []*LocType{{ConfigType: ConfigType{ID: 0, DebugName: "other"}}, {ConfigType: ConfigType{ID: 1, DebugName: "fresh"}}},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := lc.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestLocTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	// Some test fixtures construct Configs without populating ConfigNames.
	// ByName must still resolve by DebugName.
	lc := &LocTypeConfigs{
		Configs:     []*LocType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := lc.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestLocTypeConfigs_ByName' -v`

Expected: FAIL — `lc.ByName undefined (type *LocTypeConfigs has no field or method ByName)`. This is the red phase per memory `plan_red_phase_prediction_old_sut`: the OLD SUT (no `ByName` method) fails compilation, not test assertions. That is the correct red.

- [ ] **Step 3: Implement `(*LocTypeConfigs).ByName`**

Append to `pkg/objtype/loctype.go`:

```go
// ByName returns the LocType matching the given debugname, or nil
// if no match exists. Mirrors TS LocType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) only if ConfigNames is unpopulated (test fixtures) or stale.
// Consumed by ::locadd in modules/world/handlers_game.go (NAI-187).
func (c *LocTypeConfigs) ByName(name string) *LocType {
	if c == nil {
		return nil
	}
	if id, ok := c.ConfigNames[name]; ok {
		if id >= 0 && id < len(c.Configs) {
			return c.Configs[id]
		}
	}
	for _, t := range c.Configs {
		if t != nil && t.DebugName == name {
			return t
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestLocTypeConfigs_ByName' -v`

Expected: 5 PASS.

- [ ] **Step 5: Run full pkg/objtype test suite to verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/`

Expected: PASS (whole package).

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/loctype.go pkg/objtype/loctype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-187 T1 — LocTypeConfigs.ByName helper

Mirrors VarpTypeConfigs.ByName (pkg/objtype/varptype.go:120) and
ObjTypeConfigs.ByName (pkg/objtype/objtype.go:76). O(1) via the
already-populated ConfigNames index with O(N) linear-scan fallback
for stale-index / test-fixture paths.

Consumed by ::locadd cheat in NAI-187 T4.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `(*NPCTypeConfigs).ByName` helper

**Files:**
- Modify: `pkg/objtype/npctype.go` (append at end of file)
- Test: `pkg/objtype/npctype_test.go` (append at end of file)

**Parallel-eligible:** Yes — runs concurrently with T1 and T3.

- [ ] **Step 1: Write the five failing tests**

Append to `pkg/objtype/npctype_test.go`:

```go
func TestNPCTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	nc := &NPCTypeConfigs{
		Configs:     []*NpcType{{ConfigType: ConfigType{ID: 0, DebugName: "first"}}, {ConfigType: ConfigType{ID: 1, DebugName: "second"}}},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := nc.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestNPCTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	nc := &NPCTypeConfigs{
		Configs:     []*NpcType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := nc.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestNPCTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var nc *NPCTypeConfigs
	if got := nc.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestNPCTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	nc := &NPCTypeConfigs{
		Configs:     []*NpcType{{ConfigType: ConfigType{ID: 0, DebugName: "other"}}, {ConfigType: ConfigType{ID: 1, DebugName: "fresh"}}},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := nc.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestNPCTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	nc := &NPCTypeConfigs{
		Configs:     []*NpcType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := nc.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestNPCTypeConfigs_ByName' -v`

Expected: FAIL — `nc.ByName undefined (type *NPCTypeConfigs has no field or method ByName)`.

- [ ] **Step 3: Implement `(*NPCTypeConfigs).ByName`**

Append to `pkg/objtype/npctype.go`:

```go
// ByName returns the NpcType matching the given debugname, or nil
// if no match exists. Mirrors TS NpcType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) only if ConfigNames is unpopulated (test fixtures) or stale.
// Consumed by ::npcadd in modules/world/handlers_game.go (NAI-187).
func (c *NPCTypeConfigs) ByName(name string) *NpcType {
	if c == nil {
		return nil
	}
	if id, ok := c.ConfigNames[name]; ok {
		if id >= 0 && id < len(c.Configs) {
			return c.Configs[id]
		}
	}
	for _, t := range c.Configs {
		if t != nil && t.DebugName == name {
			return t
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestNPCTypeConfigs_ByName' -v`

Expected: 5 PASS.

- [ ] **Step 5: Run full pkg/objtype test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/npctype.go pkg/objtype/npctype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-187 T2 — NPCTypeConfigs.ByName helper

Mirrors VarpTypeConfigs.ByName and the new LocTypeConfigs.ByName
(NAI-187 T1). Consumed by ::npcadd in NAI-187 T5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `(*ComponentTypeConfigs).ByName` helper

**Files:**
- Modify: `pkg/objtype/componenttype.go` (append at end of file)
- Test: `pkg/objtype/componenttype_test.go` (append at end of file)

**Parallel-eligible:** Yes — runs concurrently with T1 and T2.

- [ ] **Step 1: Write the five failing tests**

Append to `pkg/objtype/componenttype_test.go`:

```go
func TestComponentTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	cc := &ComponentTypeConfigs{
		Configs:     []*ComponentType{{ConfigType: ConfigType{ID: 0, DebugName: "first"}}, {ConfigType: ConfigType{ID: 1, DebugName: "second"}}},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := cc.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestComponentTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	cc := &ComponentTypeConfigs{
		Configs:     []*ComponentType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := cc.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestComponentTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var cc *ComponentTypeConfigs
	if got := cc.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestComponentTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	cc := &ComponentTypeConfigs{
		Configs:     []*ComponentType{{ConfigType: ConfigType{ID: 0, DebugName: "other"}}, {ConfigType: ConfigType{ID: 1, DebugName: "fresh"}}},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := cc.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestComponentTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	cc := &ComponentTypeConfigs{
		Configs:     []*ComponentType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := cc.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestComponentTypeConfigs_ByName' -v`

Expected: FAIL — `cc.ByName undefined (type *ComponentTypeConfigs has no field or method ByName)`.

- [ ] **Step 3: Implement `(*ComponentTypeConfigs).ByName`**

Append to `pkg/objtype/componenttype.go`:

```go
// ByName returns the ComponentType matching the given debugname, or nil
// if no match exists. Mirrors TS Component.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) only if ConfigNames is unpopulated (test fixtures) or stale.
// Consumed by ::openmain in modules/world/handlers_game.go (NAI-187).
func (c *ComponentTypeConfigs) ByName(name string) *ComponentType {
	if c == nil {
		return nil
	}
	if id, ok := c.ConfigNames[name]; ok {
		if id >= 0 && id < len(c.Configs) {
			return c.Configs[id]
		}
	}
	for _, t := range c.Configs {
		if t != nil && t.DebugName == name {
			return t
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run 'TestComponentTypeConfigs_ByName' -v`

Expected: 5 PASS.

- [ ] **Step 5: Run full pkg/objtype test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/componenttype.go pkg/objtype/componenttype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-187 T3 — ComponentTypeConfigs.ByName helper

Mirrors VarpTypeConfigs.ByName and the sibling helpers in NAI-187
T1/T2. Consumed by ::openmain in NAI-187 T6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `::locadd <name>` cheat dispatch

**Files:**
- Modify: `modules/world/handlers_game.go` (add import + new `case` arm in `>= 3` switch)
- Test: `modules/world/handlers_game_test.go` (append three tests)

**Depends on:** T1 (`LocTypeConfigs.ByName`). NOT parallel with T5 / T6 — all three touch the same `handlers_game.go` switch block, so sequential commits avoid edit conflicts.

- [ ] **Step 1: Write the three failing tests**

Append to `modules/world/handlers_game_test.go`:

```go
// TestHandleClientCheat_Locadd_SpawnsLoc pins TS L441-452. Resolves
// LocType by debugname, spawns a CENTREPIECE_STRAIGHT loc with
// angle=WEST=0, duration=500. Emits "Loc Added: <name> (ID: <id>)".
func TestHandleClientCheat_Locadd_SpawnsLoc(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	const locName = "test_dialogue_box"
	const locID = 42

	s.locTypes = &objtype.LocTypeConfigs{
		Configs: []*objtype.LocType{{
			ConfigType: objtype.ConfigType{ID: locID, DebugName: locName},
			Width:      1,
			Length:     1,
		}},
		ConfigNames: map[string]int{locName: 0},
	}

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "locadd "+locName)
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	// Verify the loc was appended to z.Locs (z.AddLoc on a DESPAWN
	// lifecycle appends to z.Locs at pkg/zone/zone.go:164).
	z := s.zoneMap.Get(p.level, p.x, p.z)
	found := false
	for _, l := range z.Locs {
		if l.Type() == locID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Loc type=%d at (%d,%d,%d); zone had %d locs",
			locID, p.x, p.z, p.level, len(z.Locs))
	}

	wantMsg := []byte("Loc Added: " + locName + " (ID: " + fmt.Sprintf("%d", locID) + ")")
	if !bytes.Contains(all, wantMsg) {
		t.Errorf("missing MessageGame %q in emitted bytes (got %d bytes)", string(wantMsg), len(all))
	}
}

// TestHandleClientCheat_Locadd_UnknownName_NoOp pins the TS L448 nil
// guard: an unknown debugname → no spawn, no MessageGame.
func TestHandleClientCheat_Locadd_UnknownName_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 0)}

	dispatchTeleCheat(t, p, "locadd absent_name")

	z := s.zoneMap.Get(p.level, p.x, p.z)
	if len(z.Locs) != 0 {
		t.Errorf("expected zero locs after unknown ::locadd; got %d", len(z.Locs))
	}
}

// TestHandleClientCheat_Locadd_EmptyArgs_NoOp pins TS L443-445 args.length<1.
func TestHandleClientCheat_Locadd_EmptyArgs_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 0)}

	dispatchTeleCheat(t, p, "locadd")

	z := s.zoneMap.Get(p.level, p.x, p.z)
	if len(z.Locs) != 0 {
		t.Errorf("expected zero locs after empty-args ::locadd; got %d", len(z.Locs))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Locadd' -v`

Expected: FAIL on `TestHandleClientCheat_Locadd_SpawnsLoc` — no case arm exists, so `::locadd test_dialogue_box` falls through unhandled; no loc spawned; assertion fails. The other two tests (unknown / empty) pass vacuously because they also expect no spawn — those become real assertions in Step 4 once the case arm exists and could mis-spawn.

**Verify with `-v` output:** the `_SpawnsLoc` test must fail with `expected Loc type=42 at (...)`. If it fails on `expected MessageGame ... in emitted bytes` first, that is also a valid red.

- [ ] **Step 3: Add imports + case arm**

Edit `modules/world/handlers_game.go` imports (line 3-19) to add the two new packages. The current import block:

```go
import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

becomes:

```go
import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

Then add a new `case "locadd"` arm inside the `if p.staffModLevel >= 3 { switch parts[0] { ... } }` block. Place it AFTER `case "minme"` (currently around line 527) and BEFORE `case "teleother"` — keeps TS-line-comments roughly monotonic since `minme` is TS L432, `locadd` is TS L441, and `teleother` is TS L377 (out of order, but `teleother` is grouped with the production-only sub-block). T5 (`npcadd`, TS L453) and T6 (`openmain`, TS L464) will follow immediately after, all three siblings. The arm:

```go
		case "locadd":
			// TS L441-452 — admin spawn. Resolves LocType by debugname,
			// spawns a CENTREPIECE_STRAIGHT loc with angle=WEST=0,
			// duration=500 ticks. Mirrors TS:
			//   World.addLoc(new Loc(player.level, player.x, player.z,
			//                        type.width, type.length,
			//                        EntityLifeCycle.DESPAWN, type.id,
			//                        LocShape.CENTREPIECE_STRAIGHT,
			//                        LocAngle.WEST), 500);
			if args == "" {
				return nil
			}
			name := strings.Fields(args)[0]
			lt := p.client.server.locTypes.ByName(name)
			if lt == nil {
				return nil
			}
			l := entitypkg.NewLoc(
				p.level, p.x, p.z,
				lt.Width, lt.Length,
				entitypkg.LifecycleDespawn,
				lt.ID,
				int(loc.ShapeCentrepieceStraight),
				0, // LocAngle.WEST
			)
			p.client.server.AddLoc(l, 500)
			p.MessageGame(fmt.Sprintf("Loc Added: %s (ID: %d)", name, lt.ID))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Locadd' -v`

Expected: 3 PASS.

- [ ] **Step 5: Run full modules/world test suite to verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-187 T4 — port ::locadd admin cheat

Resolves LocType by debugname (via LocTypeConfigs.ByName from T1),
constructs a CENTREPIECE_STRAIGHT loc at the player's tile with
angle=WEST=0 and duration=500 ticks, routes through s.AddLoc, and
emits "Loc Added: <name> (ID: <id>)". Mirrors TS L441-452.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `::npcadd <name>` cheat dispatch

**Files:**
- Modify: `modules/world/handlers_game.go` (add new `case` arm in `>= 3` switch)
- Test: `modules/world/handlers_game_test.go` (append three tests)

**Depends on:** T2 (`NPCTypeConfigs.ByName`) and T4 (`entitypkg` / `loc` imports already added). Sequential after T4.

- [ ] **Step 1: Write the three failing tests**

Append to `modules/world/handlers_game_test.go`:

```go
// TestHandleClientCheat_Npcadd_SpawnsNpc pins TS L453-463. Resolves
// NpcType by debugname, constructs a DESPAWN npc at the player's
// tile with duration=500; nid allocated inside addNpc. TS has no
// MessageGame.
func TestHandleClientCheat_Npcadd_SpawnsNpc(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	const npcName = "test_chicken"
	const npcID = 41

	// NPC registry needs s.npcTypes populated; teleTestPlayer leaves it nil.
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs:     make([]*objtype.NpcType, 100),
		ConfigNames: map[string]int{npcName: npcID},
	}
	s.npcTypes.Configs[npcID] = &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: npcID, DebugName: npcName},
		Size:         1,
		RespawnRate:  100,
		Timer:        0,
		RegenRate:    0,
		HuntMode:     -1,
		HuntRange:    0,
		BlockWalk:    objtype.BlockWalkNone,
		MoveRestrict: 0,
	}

	startNpcCount := len(s.npcLoop)
	dispatchTeleCheat(t, p, "npcadd "+npcName)

	if len(s.npcLoop) != startNpcCount+1 {
		t.Fatalf("after ::npcadd: npcLoop len = %d, want %d", len(s.npcLoop), startNpcCount+1)
	}
	added := s.npcLoop[len(s.npcLoop)-1]
	if added.typeId != npcID {
		t.Errorf("spawned npc.typeId = %d, want %d", added.typeId, npcID)
	}
	if added.x != p.x || added.z != p.z || added.level != p.level {
		t.Errorf("spawned npc coord = (%d,%d,%d), want (%d,%d,%d)",
			added.x, added.z, added.level, p.x, p.z, p.level)
	}
	if added.lifecycle != NpcLifecycleDespawn {
		t.Errorf("spawned npc.lifecycle = %d, want NpcLifecycleDespawn (%d)",
			added.lifecycle, NpcLifecycleDespawn)
	}
}

// TestHandleClientCheat_Npcadd_UnknownName_NoOp pins the TS L460 nil
// guard: an unknown debugname → no spawn.
func TestHandleClientCheat_Npcadd_UnknownName_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 0)}
	startNpcCount := len(s.npcLoop)

	dispatchTeleCheat(t, p, "npcadd absent_name")

	if len(s.npcLoop) != startNpcCount {
		t.Errorf("unknown ::npcadd should not change npcLoop; len = %d, want %d",
			len(s.npcLoop), startNpcCount)
	}
}

// TestHandleClientCheat_Npcadd_EmptyArgs_NoOp pins TS L455-457
// args.length<1.
func TestHandleClientCheat_Npcadd_EmptyArgs_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 0)}
	startNpcCount := len(s.npcLoop)

	dispatchTeleCheat(t, p, "npcadd")

	if len(s.npcLoop) != startNpcCount {
		t.Errorf("empty-args ::npcadd should not change npcLoop; len = %d, want %d",
			len(s.npcLoop), startNpcCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Npcadd' -v`

Expected: FAIL on `_SpawnsNpc` (no case arm → no spawn → `len(s.npcLoop) != startNpcCount+1`).

- [ ] **Step 3: Add the `case "npcadd"` arm**

Insert into the `if p.staffModLevel >= 3 { switch parts[0] { ... } }` block, AFTER the `case "locadd"` arm added in T4 (alphabetical within the spawn cluster: locadd, npcadd, openmain):

```go
		case "npcadd":
			// TS L453-463 — admin spawn. Resolves NpcType by debugname,
			// constructs a DESPAWN npc at (p.x, p.z, p.level) with
			// duration=500 ticks. nid is allocated inside s.addNpc
			// (firstSpawn=true). TS has no MessageGame on success.
			// Mirrors TS:
			//   World.addNpc(new Npc(player.level, player.x, player.z,
			//                        type.size, type.size,
			//                        EntityLifeCycle.DESPAWN,
			//                        World.getNextNid(), type.id,
			//                        type.moverestrict, type.blockwalk), 500);
			if args == "" {
				return nil
			}
			name := strings.Fields(args)[0]
			nt := p.client.server.npcTypes.ByName(name)
			if nt == nil {
				return nil
			}
			n := NewNpc(0 /* placeholder; allocated inside addNpc */, nt.ID, p.x, p.z, p.level, nt)
			n.lifecycle = NpcLifecycleDespawn
			_ = p.client.server.addNpc(n, 500, true)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Npcadd' -v`

Expected: 3 PASS.

- [ ] **Step 5: Run full modules/world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-187 T5 — port ::npcadd admin cheat

Resolves NpcType by debugname (via NPCTypeConfigs.ByName from T2),
constructs a DESPAWN-lifecycle Npc at the player's tile with
duration=500 ticks, routes through s.addNpc (which allocates the nid).
No MessageGame on success (TS-faithful). Mirrors TS L453-463.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `::openmain <name>` cheat dispatch

**Files:**
- Modify: `modules/world/handlers_game.go` (add new `case` arm in `>= 3` switch)
- Test: `modules/world/handlers_game_test.go` (append three tests)

**Depends on:** T3 (`ComponentTypeConfigs.ByName`). Sequential after T5.

- [ ] **Step 1: Write the three failing tests**

Append to `modules/world/handlers_game_test.go`:

```go
// TestHandleClientCheat_Openmain_OpensMainModal pins TS L464-476.
// Resolves ComponentType by debugname; gate type.rootLayer === type.id
// passes only for root layers; routes through p.OpenMain which sets
// modalMain, clears modalChat/Side, sets modalState=modalStateMain,
// sets refreshModal=true.
func TestHandleClientCheat_Openmain_OpensMainModal(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	const comName = "test_dialogue_root"
	const comID = 100

	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs:     make([]*objtype.ComponentType, 200),
		ConfigNames: map[string]int{comName: comID},
	}
	s.componentTypes.Configs[comID] = &objtype.ComponentType{
		ConfigType: objtype.ConfigType{ID: comID, DebugName: comName},
		RootLayer:  comID, // root: rootLayer == id passes the gate
	}

	// Seed an open chat/side modal so we can verify OpenMain clears them.
	p.modalChat = 999
	p.modalSide = 888
	p.modalState = modalStateChat | modalStateSide
	p.refreshModal = false

	dispatchTeleCheat(t, p, "openmain "+comName)

	if p.modalMain != comID {
		t.Errorf("modalMain = %d, want %d", p.modalMain, comID)
	}
	if p.modalChat != -1 {
		t.Errorf("modalChat = %d, want -1 (cleared by OpenMain)", p.modalChat)
	}
	if p.modalSide != -1 {
		t.Errorf("modalSide = %d, want -1 (cleared by OpenMain)", p.modalSide)
	}
	if p.modalState != modalStateMain {
		t.Errorf("modalState = %d, want modalStateMain (%d)", p.modalState, modalStateMain)
	}
	if !p.refreshModal {
		t.Error("refreshModal = false, want true")
	}
}

// TestHandleClientCheat_Openmain_NonRoot_NoOp pins TS L472 rootLayer
// guard: a component whose rootLayer != id (i.e. a child layer) is
// rejected without opening any modal.
func TestHandleClientCheat_Openmain_NonRoot_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)
	const comName = "test_child_layer"
	const comID = 101

	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs:     make([]*objtype.ComponentType, 200),
		ConfigNames: map[string]int{comName: comID},
	}
	s.componentTypes.Configs[comID] = &objtype.ComponentType{
		ConfigType: objtype.ConfigType{ID: comID, DebugName: comName},
		RootLayer:  50, // child: rootLayer != id fails the gate
	}

	startMain := p.modalMain
	startState := p.modalState

	dispatchTeleCheat(t, p, "openmain "+comName)

	if p.modalMain != startMain {
		t.Errorf("non-root ::openmain mutated modalMain: %d → %d", startMain, p.modalMain)
	}
	if p.modalState != startState {
		t.Errorf("non-root ::openmain mutated modalState: %d → %d", startState, p.modalState)
	}
}

// TestHandleClientCheat_Openmain_UnknownName_NoOp pins TS L472 nil
// guard: an unknown debugname → no modal change.
func TestHandleClientCheat_Openmain_UnknownName_NoOp(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 3
	go io.Copy(io.Discard, cc)

	s.componentTypes = &objtype.ComponentTypeConfigs{Configs: make([]*objtype.ComponentType, 0)}
	startMain := p.modalMain

	dispatchTeleCheat(t, p, "openmain absent_name")

	if p.modalMain != startMain {
		t.Errorf("unknown ::openmain mutated modalMain: %d → %d", startMain, p.modalMain)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Openmain' -v`

Expected: FAIL on `_OpensMainModal` (no case arm → modalMain unchanged).

- [ ] **Step 3: Add the `case "openmain"` arm**

Insert into the `if p.staffModLevel >= 3 { switch parts[0] { ... } }` block, AFTER the `case "npcadd"` arm added in T5:

```go
		case "openmain":
			// TS L464-476 — admin interface routing. Resolves
			// ComponentType by debugname, gates on rootLayer == id
			// (only root layers can be main modals), routes through
			// p.OpenMain (which closes chat + side modals and sets
			// refreshModal per TS Player.openMainModal modal-mutex).
			// TS L476: player.openMainModal(type.id).
			if args == "" {
				return nil
			}
			name := strings.Fields(args)[0]
			ct := p.client.server.componentTypes.ByName(name)
			if ct == nil || ct.RootLayer != ct.ID {
				return nil
			}
			p.OpenMain(ct.ID)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_Openmain' -v`

Expected: 3 PASS.

- [ ] **Step 5: Run full modules/world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-187 T6 — port ::openmain admin cheat

Resolves ComponentType by debugname (via ComponentTypeConfigs.ByName
from T3), gates on rootLayer == id (only root layers can be main
modals), routes through p.OpenMain (modal-mutex: closes chat + side,
sets refreshModal). Mirrors TS L464-476.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Combined `staffModLevel < 3` gate test

**Files:**
- Test: `modules/world/handlers_game_test.go` (append one test)

**Depends on:** T4 + T5 + T6. Sequential.

This task verifies the outer `if p.staffModLevel >= 3` guard rejects all three new cheats. Matches the NAI-185 `TestHandleClientCheat_Give_AdminGate` pattern at `handlers_game_test.go:937`.

- [ ] **Step 1: Write the gate test**

Append to `modules/world/handlers_game_test.go`:

```go
// TestHandleClientCheat_AdminSpawn_StaffGateRejects pins the NAI-187
// admin-tier gate: at p.staffModLevel = 2 (mod tier), none of the
// three NAI-187 cheats fire. Mirrors the NAI-185 _Give_AdminGate
// pattern. Three sub-assertions, one fixture.
func TestHandleClientCheat_AdminSpawn_StaffGateRejects(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 2 // below admin tier
	go io.Copy(io.Discard, cc)

	// Seed all three config tables with valid named entries so the
	// gate is the only thing that can reject. If the gate were absent,
	// each cheat would mutate world state with these fixtures.
	const locName = "gate_test_loc"
	const npcName = "gate_test_npc"
	const comName = "gate_test_com"
	const locID, npcID, comID = 42, 41, 100

	s.locTypes = &objtype.LocTypeConfigs{
		Configs: []*objtype.LocType{{
			ConfigType: objtype.ConfigType{ID: locID, DebugName: locName},
			Width:      1,
			Length:    1,
		}},
		ConfigNames: map[string]int{locName: 0},
	}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: make([]*objtype.NpcType, 100),
		ConfigNames: map[string]int{npcName: npcID},
	}
	s.npcTypes.Configs[npcID] = &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: npcID, DebugName: npcName},
		Size:        1,
		RespawnRate: 100,
		HuntMode:    -1,
		BlockWalk:   objtype.BlockWalkNone,
	}
	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 200),
		ConfigNames: map[string]int{comName: comID},
	}
	s.componentTypes.Configs[comID] = &objtype.ComponentType{
		ConfigType: objtype.ConfigType{ID: comID, DebugName: comName},
		RootLayer:  comID,
	}

	startNpcCount := len(s.npcLoop)
	startModalMain := p.modalMain

	dispatchTeleCheat(t, p, "locadd "+locName)
	dispatchTeleCheat(t, p, "npcadd "+npcName)
	dispatchTeleCheat(t, p, "openmain "+comName)

	z := s.zoneMap.Get(p.level, p.x, p.z)
	if len(z.Locs) != 0 {
		t.Errorf("staff<3 ::locadd should not spawn; zone had %d locs", len(z.Locs))
	}
	if len(s.npcLoop) != startNpcCount {
		t.Errorf("staff<3 ::npcadd should not spawn; npcLoop len = %d, want %d",
			len(s.npcLoop), startNpcCount)
	}
	if p.modalMain != startModalMain {
		t.Errorf("staff<3 ::openmain should not open modal; modalMain = %d, want %d",
			p.modalMain, startModalMain)
	}
}
```

- [ ] **Step 2: Run the test to verify it passes (no implementation needed — gate is the existing `if p.staffModLevel >= 3` at handlers_game.go:427)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleClientCheat_AdminSpawn_StaffGateRejects' -v`

Expected: PASS. (This is a confirmation test, not a TDD red-then-green — the gate already existed before NAI-187. The test pins that the new cases land INSIDE the gate, not outside.)

**Red-phase confidence check** (memory `plan_red_phase_prediction_old_sut`): if any of the new `case` arms were accidentally placed OUTSIDE the `>= 3` block (e.g. in the outer `>=2` switch at line 925), this test would FAIL — that is the regression this test guards against.

- [ ] **Step 3: Run full modules/world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-187 T7 — admin gate test for locadd/npcadd/openmain

Combined staffModLevel<3 gate test mirroring NAI-185
_Give_AdminGate. Pins that none of the three NAI-187 cheats fire
below admin tier — guards against a future accidental relocation of
the case arms outside the >=3 outer guard.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task CLOSE: Rewrite `DEVIATION-NAI-186-D2-CARRYFORWARD` block

**Files:**
- Modify: `modules/world/handlers_game.go` (rewrite the carryforward block at lines 368-377)

**Depends on:** T7.

Per memory `tracker_carryforward_listings_compound` + `close_commit_memory_trailer`: the carryforward block must be re-derived from current TS source state, not copy-edited from prior carryforward text. The CLOSE commit gets a `Closes memory:` trailer for the audit memory referenced in this task.

- [ ] **Step 1: Replace the carryforward block**

Edit `modules/world/handlers_game.go` lines 368-377. Current content:

```go
	// DEVIATION-NAI-186-D2-CARRYFORWARD — supersedes
	// DEVIATION-NAI-185-D4-CARRYFORWARD. 6 TS ClientCheatHandler
	// cheats remain unported:
	//   Dev block (!NP && >=4): reload, rebuild, speed.
	//     Blocked on cache/script reload subsystem + runtime
	//     tick-rate mutation (tick.go interval is currently fixed).
	//   Admin block (>=3):      locadd, npcadd, openmain.
	//     Blocked on dynamic Loc/Npc spawn + interface routing.
	// NAI-186 retired the super-mod cluster (setvis/ban/mute/kick).
	// Each cluster warrants its own follow-up sub-spec.
```

Replace with:

```go
	// DEVIATION-NAI-187-D1-CARRYFORWARD — supersedes
	// DEVIATION-NAI-186-D2-CARRYFORWARD. 3 TS ClientCheatHandler
	// cheats remain unported, all in the dev block (!NP && >=4):
	//   reload:  TS L149-150. Calls World.reload() — full cache
	//            hot-reload pipeline. No goscape equivalent;
	//            substantial new subsystem.
	//   rebuild: TS L151-153. Calls World.rebuild() — script-provider
	//            hot-reload. Same infra gap as reload.
	//   speed:   TS L154-167. Trivial code (~10 LOC) but mutates
	//            World.tickRate, currently a package-level const at
	//            modules/world/tick.go:15. Right size for its own
	//            one-shot follow-up sub-spec.
	// NAI-187 retired the admin spawn/interface cluster (locadd /
	// npcadd / openmain). Per memory tracker_entry_framing_can_be_
	// incomplete: the prior "blocked on dynamic Loc/Npc spawn +
	// interface routing" framing was stale at HEAD — all primitives
	// existed; sole gap was three ByName helpers in pkg/objtype.
```

- [ ] **Step 2: Run full test suite to verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS.

- [ ] **Step 3: Run with race detector for final confidence**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ ./pkg/objtype/`

Expected: PASS.

- [ ] **Step 4: Commit close**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-187 — admin spawn/interface cheats cohort complete

Closes the admin-tier portion of DEVIATION-NAI-186-D2-CARRYFORWARD.
::locadd / ::npcadd / ::openmain now port their TS counterparts
(ClientCheatHandler.ts L441-476) via three new ByName helpers in
pkg/objtype.

Rewrites the carryforward block as DEVIATION-NAI-187-D1-CARRYFORWARD.
Dev-block remainder (reload / rebuild / speed) reframed per-cheat:
reload+rebuild genuinely need a cache/script hot-reload pipeline;
speed is trivial code but touches load-bearing tick-loop infra and
deserves its own one-shot follow-up.

Per memory tracker_entry_framing_can_be_incomplete: the prior
admin-cluster framing was stale — the cited "dynamic Loc/Npc spawn +
interface routing" infra all existed at HEAD; sole gap was the
ByName helpers. Second recurrence of this pattern in this cheat-port
arc (NAI-186 inverted super-mod labels was the first).

Closes memory: tracker_entry_framing_can_be_incomplete
Closes memory: tracker_carryforward_listings_compound

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes

Per `superpowers:writing-plans` self-review checklist, completed inline:

**Spec coverage (vs. `2026-05-13-nai-187-admin-spawn-cheats-design.md`):**

- Spec §1 goal (port 3 cheats + close admin carryforward) → T4/T5/T6/CLOSE ✓
- Spec §2 out-of-scope (reload/rebuild/speed/snapshot) → CLOSE carryforward documents the residual ✓
- Spec §3 pre-flight audit primitives → controller pre-flight section enumerates each ✓
- Spec §4.1 three ByName helpers → T1/T2/T3 ✓
- Spec §4.2 inline placement in admin switch → T4/T5/T6 each show placement ✓
- Spec §5 per-cheat impl (code blocks) → reproduced verbatim in T4/T5/T6 Step 3 ✓
- Spec §6.1 ByName helper tests (5 each) → T1/T2/T3 Step 1 each show 5 tests ✓
- Spec §6.2 cheat dispatch tests (happy / unknown / empty) → T4/T5/T6 Step 1 each show 3 tests ✓
- Spec §6.2 combined gate test → T7 ✓
- Spec §6.3 test fixture audit → controller pre-flight section ✓
- Spec §7 deviations (none expected) → no D-tag in any task ✓
- Spec §9 iteration order (T1-T3 parallel, T4-T6 sequential, T7+CLOSE) → matches plan ✓

**Placeholder scan:** No "TBD" / "TODO" / "Add appropriate X" / "similar to Task N". Every code block is complete. Every test has assertion bodies. ✓

**Type consistency:**
- `LocTypeConfigs` / `NPCTypeConfigs` / `ComponentTypeConfigs` — spelled identically across T1/T2/T3 and the cheat dispatch tasks. ✓
- `ByName` signature `(name string) *<Type>` consistent in all three helpers. ✓
- `entitypkg "github.com/zsrv/goscape/pkg/entity"` — single alias used in T4 import + body; T5/T6 reuse the same alias. ✓
- `loc.ShapeCentrepieceStraight` — package alias `loc` used consistently. ✓
- `objtype.BlockWalkNone` — used in T5 + T7 fixtures consistently.
- `p.client.server.<field>.ByName(name)` — same accessor pattern in T4/T5/T6. ✓
- `NewNpc(0, nt.ID, p.x, p.z, p.level, nt)` signature matches `modules/world/npc.go:159`. ✓

**Fixture runnability** (memory `plan_runnable_test_fixtures`): mentally traced each test fixture against `teleTestPlayer(t)` post-conditions and the called helpers (`AddLoc`, `addNpc`, `OpenMain`). The npcadd fixture sets `HuntMode: -1` and `BlockWalk: BlockWalkNone` to keep the spawned NPC from triggering hunt/collision side effects that could interfere with assertions. The locadd fixture uses `Width: 1, Length: 1` to keep the spawned Loc within a single tile.

**Memory hooks** referenced in this plan:
- `controller_preflight` — pre-flight verification block at top of plan.
- `plan_red_phase_prediction_old_sut` — explicit red-phase notes in T1/T2/T3 Step 2, T7 Step 2.
- `plan_runnable_test_fixtures` — fixtures traced.
- `plan_sibling_site_guard_audit` — `give` / `setvar` sibling pattern audited; no extra nil-guards needed.
- `tracker_entry_framing_can_be_incomplete` — CLOSE commit body references this.
- `tracker_carryforward_listings_compound` — CLOSE rewrites rather than amending.
- `close_commit_memory_trailer` — CLOSE commit has `Closes memory:` trailers.
- `superpowers_code_reviewer_model` — code-reviewer subagents must run on Sonnet, not Opus (subagent-driven-development controller responsibility).
- `superpowers_clear_between_spec_and_impl` — subagent-driven-development should /clear between plan-write and implementer dispatch; the controller emits a resume prompt before exiting plan-write.

---

## Execution Handoff

Per memory `execution_mode_default`: dispatch via **subagent-driven-development** (the default for this project). The controller will:

1. Pre-flight grep the assertions in the "Pre-flight expectations for implementers" section against HEAD before each dispatch.
2. Dispatch T1, T2, T3 in parallel (memory `dispatching-parallel-agents`).
3. Two-stage review after each task (implementer + code-reviewer on Sonnet, memory `superpowers_code_reviewer_model`).
4. After T3 finishes, dispatch T4, then T5, then T6, then T7, then CLOSE sequentially. T4-T6 must be sequential because all three touch the same switch block in `handlers_game.go`.
5. On CLOSE merge, the carryforward block must be re-grep'd to confirm exactly 3 cheats remain unported (`reload`/`rebuild`/`speed`) — memory `tracker_carryforward_listings_compound`.
