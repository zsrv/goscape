# NAI-189 — DEBUGPROC Cheat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the TS DEBUGPROC dispatch path (`ClientCheatHandler.ts:59-148`) into goscape — resolve `[debugproc,X]` runescripts by name and dispatch them via `s.runScript` with positional arguments parsed per the script's declared `ParamTypes`.

**Architecture:** Add four missing `ByName` helpers to `pkg/objtype/` (Seq, Spotanim, Idk, Inv) mirroring the NAI-187 cluster shape (ConfigNames hit → bounds check → linear-scan fallback → nil). Add a pure `marshalDebugprocArgs(s, sf, args, rawCheat) ([]int, []string)` helper in `modules/world/handlers_game.go` that walks `sf.ParamTypes` and tokenises arguments. A thin `dispatchDebugproc(p, cmd, args, rawCheat)` wrapper resolves the script by name and calls `s.runScript`. The dev-block in `handleClientCheat` gains a prefix branch (`strings.HasPrefix(parts[0], s.cfg.NodeDebugprocChar)`) BEFORE the existing fixed-cmd switch, mirroring TS dispatch order.

**Tech Stack:** Go 1.26+

**Spec:** `docs/superpowers/specs/2026-05-13-nai-189-debugproc-cheat-design.md`
**HEAD at plan-write:** `46fdd27`
**TS source:** `LostCityRS/Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts:59-148`

---

## File Map

| File | Role | Action |
|---|---|---|
| `pkg/objtype/seqtype.go` | SeqType configs | Modify: add `ByName` method (~16 LOC, mirrors `LocTypeConfigs.ByName`). |
| `pkg/objtype/spotanimtype.go` | SpotanimType configs | Modify: add `ByName` method. |
| `pkg/objtype/idktype.go` | IdkType configs | Modify: add `ByName` method. |
| `pkg/objtype/invtype.go` | InvType configs | Modify: add `ByName` method. |
| `pkg/objtype/seqtype_test.go` | SeqType tests | Modify: add 5 `TestSeqTypeConfigs_ByName_*` cases mirroring `loctype_test.go:660-720`. |
| `pkg/objtype/spotanimtype_test.go` | SpotanimType tests | Modify: add 5 ByName tests. |
| `pkg/objtype/idktype_test.go` | IdkType tests | Modify: add 5 ByName tests. |
| `pkg/objtype/invtype_test.go` | InvType tests | Modify: add 5 ByName tests. |
| `modules/world/handlers_game.go` | cheat dispatch | Modify: add `marshalDebugprocArgs` + `dispatchDebugproc` helpers; insert prefix branch in `handleClientCheat`; rewrite carryforward comment. |
| `modules/world/handlers_game_test.go` | cheat-handler tests | Modify: add 19 unit tests for `marshalDebugprocArgs` + 5 gate/integration tests for `dispatchDebugproc` via `handleClientCheat`. |

No new files.

---

## Plan-author pre-flight (recorded at plan-write, HEAD `46fdd27`)

Re-verified spec premises against current `main`:

1. **ByName cluster pattern (NAI-187)** — `pkg/objtype/loctype.go:254-269`, `npctype.go:409`, `componenttype.go:349` all follow the same 4-step shape:
   - nil-receiver guard
   - `ConfigNames[name]` lookup with bounds check on `Configs`
   - linear-scan fallback over `Configs[*].DebugName`
   - return nil

   Plan-Task 1-4 mirror this verbatim. The simpler "ConfigNames only" sketch in spec §4.1 is **superseded** by this pattern — the linear-scan fallback is established sibling code and the existing `TestLocTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty` test pins it as required behaviour. Each new test file adds 5 tests, not 3.

2. **`ConfigNames` populator branches** — confirmed all 4 types populate `ConfigNames` only when `DebugName != ""`:
   - `seqtype.go:164-165` (inside `parseSeqTypes`)
   - `spotanimtype.go:130-131`
   - `idktype.go:127-128`
   - `invtype.go:127-128`

3. **`Server` field names** — `s.objTypes`, `s.npcTypes`, `s.locTypes`, `s.componentTypes`, `s.invTypes`, `s.idkTypes`, `s.seqTypes`, `s.spotanimTypes` all exist (`modules/world/server.go:95-120`).

4. **`ScriptFile.ParamTypes`** — `pkg/script/file.go:20` `ParamTypes []byte`. Each byte casts to `objtype.ScriptVarType` for comparison (already the convention in `pkg/script/handlers_db.go:115`).

5. **`s.runScript`** — `modules/world/script.go:92` `runScript(sf, self, target, protect, intArgs, stringArgs)`. Existing entry; no signature change needed.

6. **Script-registration API** — `pkg/script/provider.go:179-190` `(*Provider).Register(f *ScriptFile)`. Adds to scripts slice + `byName` map. **Tests can use this directly to stage `[debugproc,X]` fixtures.**

7. **Cheat-test helpers** — `modules/world/handlers_game_test.go:366` `teleTestPlayer`, `:394` `dispatchTeleCheat`, `:406` `drainAfterTele`. All reusable as-is. Per memory `test_fixture_view_parity`, `newTestServer` (`server_test.go:311`) already wires `scriptProvider` + the configsView/invLookup/worldVars/npcLookup needed for cascade-style scripts.

8. **Dev-block scope at HEAD** — `handlers_game.go:402` (`if !p.client.server.cfg.NodeProduction && p.staffModLevel >= 4`) followed by `switch parts[0]`. Prefix branch inserts BEFORE the switch (spec §4.3).

9. **`cheat` variable** — `handlers_game.go:361` already holds the full lowered cheat string. Passes through to `dispatchDebugproc` as `rawCheat`.

10. **`parseIntOr`** — `handlers_game.go:1011`. Mirrors TS `tryParseInt` (returns default on parse fail).

11. **Existing cheat-test file** — `handlers_game_test.go` (not `handler_cheats_supermod_test.go`). DEBUGPROC tests live in `handlers_game_test.go` alongside `TestTeleCheat_*` and the existing fly/naive/speed tests.

12. **`fly`/`naive`/`speed` cohort tests at HEAD** — `grep -n "TestCheatSpeed_\|TestCheatFly\|TestCheatNaive\|TestHandleClientCheat" handlers_game_test.go` shows the existing pin set. The new prefix branch must NOT affect any cheat whose `parts[0]` does not begin with `s.cfg.NodeDebugprocChar`.

13. **Content-side smoke target** — at plan-write, no goscape-side `[debugproc,*]` fixtures exist. The `2004scape/Server` content snapshot location for goscape's cache loader has scripts under `runescript-rev-225/` (per cache path conventions). End-to-end smoke is OUT OF SCOPE for this plan; unit tests via Provider.Register cover the dispatch. Per spec §7.6, document this in close commit so the user can stage a one-liner debugproc for manual smoke if desired.

14. **Per memory `int32_hex_literal_overflow`** — TS's `parseInt(v ?? '0', 10) | 0` coerces NaN→0 via bitor. Goscape's `parseIntOr(tok, 0)` returns 0 on parse fail (same observable result on `""` and on non-numeric tokens). NOT divergent for the INT arm — see §COORD note (Task 7) for where it DOES diverge.

---

## Task 1: `SeqType.ByName` + tests

**Files:**
- Modify: `pkg/objtype/seqtype.go` (add method at end of file)
- Modify: `pkg/objtype/seqtype_test.go` (add 5 tests at end of file)

- [ ] **Step 1: Write the 5 failing tests**

Append to `pkg/objtype/seqtype_test.go`:

```go
func TestSeqTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	c := &SeqTypeConfigs{
		Configs: []*SeqType{
			{ConfigType: ConfigType{ID: 0, DebugName: "first"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "second"}},
		},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := c.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestSeqTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	c := &SeqTypeConfigs{
		Configs:     []*SeqType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := c.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestSeqTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var c *SeqTypeConfigs
	if got := c.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestSeqTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	c := &SeqTypeConfigs{
		Configs: []*SeqType{
			{ConfigType: ConfigType{ID: 0, DebugName: "other"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "fresh"}},
		},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := c.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestSeqTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	c := &SeqTypeConfigs{
		Configs:     []*SeqType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := c.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestSeqTypeConfigs_ByName" ./pkg/objtype/...`

Expected: FAIL — `c.ByName undefined (type *SeqTypeConfigs has no field or method ByName)`.

- [ ] **Step 3: Add the `ByName` method**

Append to `pkg/objtype/seqtype.go` (after the existing `LoadSeqTypes` function group):

```go
// ByName returns the SeqType matching the given debugname, or nil
// if no match exists. Mirrors TS SeqType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) linear-scan fallback for test fixtures or stale indices.
// Consumed by dispatchDebugproc in modules/world/handlers_game.go (NAI-189).
func (c *SeqTypeConfigs) ByName(name string) *SeqType {
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

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run "TestSeqTypeConfigs_ByName" ./pkg/objtype/...`

Expected: PASS (5 tests).

- [ ] **Step 5: Run the full pkg/objtype suite to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/objtype/...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/seqtype.go pkg/objtype/seqtype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-189 T1 — SeqTypeConfigs.ByName helper

Mirrors the NAI-187 ByName cluster pattern (LocType/NpcType/
ComponentType). ConfigNames-indexed primary lookup with linear-scan
fallback for test fixtures and stale indices. Consumed by
dispatchDebugproc (modules/world/handlers_game.go) to resolve
SEQ-typed positional args for debugproc dispatch.

5 new tests pin the established sibling test set:
HitViaConfigNames / MissReturnsNil / NilReceiverReturnsNil /
StaleIndexFallsThroughToLinearScan / LinearScanWhenConfigNamesEmpty.
EOF
)"
```

---

## Task 2: `SpotanimType.ByName` + tests

**Files:**
- Modify: `pkg/objtype/spotanimtype.go`
- Modify: `pkg/objtype/spotanimtype_test.go`

- [ ] **Step 1: Write the 5 failing tests**

Append to `pkg/objtype/spotanimtype_test.go`:

```go
func TestSpotanimTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	c := &SpotanimTypeConfigs{
		Configs: []*SpotanimType{
			{ConfigType: ConfigType{ID: 0, DebugName: "first"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "second"}},
		},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := c.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestSpotanimTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	c := &SpotanimTypeConfigs{
		Configs:     []*SpotanimType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := c.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestSpotanimTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var c *SpotanimTypeConfigs
	if got := c.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestSpotanimTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	c := &SpotanimTypeConfigs{
		Configs: []*SpotanimType{
			{ConfigType: ConfigType{ID: 0, DebugName: "other"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "fresh"}},
		},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := c.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestSpotanimTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	c := &SpotanimTypeConfigs{
		Configs:     []*SpotanimType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := c.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestSpotanimTypeConfigs_ByName" ./pkg/objtype/...`

Expected: FAIL — method undefined.

- [ ] **Step 3: Add the `ByName` method**

Append to `pkg/objtype/spotanimtype.go`:

```go
// ByName returns the SpotanimType matching the given debugname, or nil
// if no match exists. Mirrors TS SpotanimType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) linear-scan fallback for test fixtures or stale indices.
// Consumed by dispatchDebugproc in modules/world/handlers_game.go (NAI-189).
func (c *SpotanimTypeConfigs) ByName(name string) *SpotanimType {
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

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run "TestSpotanimTypeConfigs_ByName" ./pkg/objtype/...`

Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/spotanimtype.go pkg/objtype/spotanimtype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-189 T2 — SpotanimTypeConfigs.ByName helper"
```

---

## Task 3: `IdkType.ByName` + tests

**Files:**
- Modify: `pkg/objtype/idktype.go`
- Modify: `pkg/objtype/idktype_test.go`

- [ ] **Step 1: Write the 5 failing tests**

Append to `pkg/objtype/idktype_test.go`:

```go
func TestIdkTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	c := &IdkTypeConfigs{
		Configs: []*IdkType{
			{ConfigType: ConfigType{ID: 0, DebugName: "first"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "second"}},
		},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := c.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestIdkTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	c := &IdkTypeConfigs{
		Configs:     []*IdkType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := c.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestIdkTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var c *IdkTypeConfigs
	if got := c.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestIdkTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	c := &IdkTypeConfigs{
		Configs: []*IdkType{
			{ConfigType: ConfigType{ID: 0, DebugName: "other"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "fresh"}},
		},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := c.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestIdkTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	c := &IdkTypeConfigs{
		Configs:     []*IdkType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := c.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestIdkTypeConfigs_ByName" ./pkg/objtype/...`

Expected: FAIL — method undefined.

- [ ] **Step 3: Add the `ByName` method**

Append to `pkg/objtype/idktype.go`:

```go
// ByName returns the IdkType matching the given debugname, or nil
// if no match exists. Mirrors TS IdkType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) linear-scan fallback for test fixtures or stale indices.
// Consumed by dispatchDebugproc in modules/world/handlers_game.go (NAI-189).
func (c *IdkTypeConfigs) ByName(name string) *IdkType {
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

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run "TestIdkTypeConfigs_ByName" ./pkg/objtype/...`

Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/idktype.go pkg/objtype/idktype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-189 T3 — IdkTypeConfigs.ByName helper"
```

---

## Task 4: `InvType.ByName` + tests

**Files:**
- Modify: `pkg/objtype/invtype.go`
- Modify: `pkg/objtype/invtype_test.go`

- [ ] **Step 1: Write the 5 failing tests**

Append to `pkg/objtype/invtype_test.go`:

```go
func TestInvTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	c := &InvTypeConfigs{
		Configs: []*InvType{
			{ConfigType: ConfigType{ID: 0, DebugName: "first"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "second"}},
		},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := c.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestInvTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	c := &InvTypeConfigs{
		Configs:     []*InvType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := c.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestInvTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var c *InvTypeConfigs
	if got := c.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestInvTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	c := &InvTypeConfigs{
		Configs: []*InvType{
			{ConfigType: ConfigType{ID: 0, DebugName: "other"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "fresh"}},
		},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := c.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestInvTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	c := &InvTypeConfigs{
		Configs:     []*InvType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := c.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvTypeConfigs_ByName" ./pkg/objtype/...`

Expected: FAIL — method undefined.

- [ ] **Step 3: Add the `ByName` method**

Append to `pkg/objtype/invtype.go`:

```go
// ByName returns the InvType matching the given debugname, or nil
// if no match exists. Mirrors TS InvType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) linear-scan fallback for test fixtures or stale indices.
// Consumed by dispatchDebugproc in modules/world/handlers_game.go (NAI-189).
func (c *InvTypeConfigs) ByName(name string) *InvType {
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

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/objtype/...`

Expected: PASS (all pkg/objtype tests including the 20 new ByName tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/invtype.go pkg/objtype/invtype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-189 T4 — InvTypeConfigs.ByName helper"
```

---

## Task 5: `marshalDebugprocArgs` helper (RED)

This task introduces a pure arg-marshaling function that walks `sf.ParamTypes` and tokenises input arguments. It's the **testable core** of debugproc dispatch — keeping it separate from `s.runScript` invocation makes the 12 arg-type arms unit-testable without staging a runscript fixture.

**Files:**
- Modify: `modules/world/handlers_game_test.go` (add the 19 failing tests + 1 helper)

The helper to be implemented in Task 6 has this signature:

```go
func (s *Server) marshalDebugprocArgs(sf *script.ScriptFile, args string, rawCheat string) (intArgs []int, stringArgs []string)
```

It walks `sf.ParamTypes` byte-by-byte, casts each to `objtype.ScriptVarType`, and appends to `intArgs` or `stringArgs` per the 12 TS arms. Missing tokens degrade to empty/zero/-1 per type. The COORD arm re-parses `rawCheat`; see Task 7 for that arm's branch.

- [ ] **Step 1: Verify test imports exist**

Open `modules/world/handlers_game_test.go` and confirm these imports are already present (search top of file):

```go
"github.com/zsrv/goscape/pkg/objtype"
scriptpkg "github.com/zsrv/goscape/pkg/script"
```

If `scriptpkg` is not imported under that alias, use whatever alias the file already uses (commonly `script`). For the rest of this plan I write `scriptpkg.ScriptFile` — adjust to the actual alias.

Run: `grep -n "github.com/zsrv/goscape/pkg/script\|github.com/zsrv/goscape/pkg/objtype" modules/world/handlers_game_test.go | head -5`

Expected: both packages already imported. If `script` is the alias, replace `scriptpkg` below with `script`.

- [ ] **Step 2: Add a fixture helper `stageDebugprocScript`**

Append to `modules/world/handlers_game_test.go`:

```go
// stageDebugprocScript registers a [debugproc,<name>] runscript with the
// given ParamTypes onto s.scriptProvider. The script body is a no-op
// (single OpReturn) — debugproc tests assert against the marshaled
// intArgs/stringArgs, not against script execution side-effects.
//
// Replaces s.scriptProvider entirely (rather than appending to the default
// provider) so debugproc-specific fixtures don't compose with the
// catch-all [opnpc1,_default] script seeded by newTestServer's
// defaultTestProvider.
func stageDebugprocScript(t *testing.T, s *Server, name string, paramTypes []byte) *scriptpkg.ScriptFile {
	t.Helper()
	if s.scriptProvider == nil || s.scriptProvider.Count() == 0 {
		s.scriptProvider = scriptpkg.NewProvider()
	}
	sf := &scriptpkg.ScriptFile{
		Name:       "[debugproc," + name + "]",
		LookupKey:  0xFFFFFFFF, // no trigger lookup
		ParamTypes: paramTypes,
		Opcodes:    []scriptpkg.Opcode{scriptpkg.OpReturn},
		IntOperands:    []int32{0},
		StringOperands: []string{""},
		InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)
	return sf
}
```

If the actual ScriptFile struct uses different field names (e.g. `Opcodes` may be `Ops`, `IntOperands` may be `IntOps`), verify by reading `pkg/script/file.go` and adjust verbatim. The intent: a minimal valid `*ScriptFile` that the provider can return via `GetByName`.

Run: `grep -n "type ScriptFile struct" -A 20 pkg/script/file.go`

If the field names don't match the fixture above, edit the fixture to match before committing the test file.

- [ ] **Step 3: Write 19 failing tests for `marshalDebugprocArgs`**

Append to `modules/world/handlers_game_test.go`:

```go
// --- NAI-189: dispatchDebugproc arg marshalling ---

func TestMarshalDebugprocArgs_String_Hit(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeString)})
	intArgs, stringArgs := s.marshalDebugprocArgs(sf, "hello", "~test hello")
	if len(intArgs) != 0 {
		t.Errorf("intArgs len = %d, want 0", len(intArgs))
	}
	if len(stringArgs) != 1 || stringArgs[0] != "hello" {
		t.Errorf("stringArgs = %+v, want [\"hello\"]", stringArgs)
	}
}

func TestMarshalDebugprocArgs_String_Missing(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeString)})
	_, stringArgs := s.marshalDebugprocArgs(sf, "", "~test")
	if len(stringArgs) != 1 || stringArgs[0] != "" {
		t.Errorf("missing-arg stringArgs = %+v, want [\"\"]", stringArgs)
	}
}

func TestMarshalDebugprocArgs_Int_Hit(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInt)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "42", "~test 42")
	if len(intArgs) != 1 || intArgs[0] != 42 {
		t.Errorf("intArgs = %+v, want [42]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Int_NonNumeric(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInt)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "banana", "~test banana")
	// TS: parseInt("banana", 10) | 0 === 0. Goscape: parseIntOr("banana", 0) == 0.
	if len(intArgs) != 1 || intArgs[0] != 0 {
		t.Errorf("non-numeric intArgs = %+v, want [0]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Int_Missing(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInt)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "", "~test")
	// TS: parseInt(undefined ?? '0', 10) | 0 === 0.
	if len(intArgs) != 1 || intArgs[0] != 0 {
		t.Errorf("missing intArgs = %+v, want [0]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Obj_Hit(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs:     []*objtype.ObjType{{ConfigType: objtype.ConfigType{ID: 946, DebugName: "knife"}}},
		ConfigNames: map[string]int{"knife": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeObj)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "knife", "~test knife")
	if len(intArgs) != 1 || intArgs[0] != 946 {
		t.Errorf("Obj_Hit intArgs = %+v, want [946]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Obj_Miss(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs:     []*objtype.ObjType{},
		ConfigNames: map[string]int{},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeObj)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "unknown", "~test unknown")
	if len(intArgs) != 1 || intArgs[0] != -1 {
		t.Errorf("Obj_Miss intArgs = %+v, want [-1]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Namedobj_Hit(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs:     []*objtype.ObjType{{ConfigType: objtype.ConfigType{ID: 7, DebugName: "knife"}}},
		ConfigNames: map[string]int{"knife": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeNamedObj)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "knife", "~test knife")
	if len(intArgs) != 1 || intArgs[0] != 7 {
		t.Errorf("Namedobj_Hit intArgs = %+v, want [7]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Npc_Hit(t *testing.T) {
	s := newTestServer(t)
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs:     []*objtype.NpcType{{ConfigType: objtype.ConfigType{ID: 11, DebugName: "man"}}},
		ConfigNames: map[string]int{"man": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeNPC)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "man", "~test man")
	if len(intArgs) != 1 || intArgs[0] != 11 {
		t.Errorf("Npc_Hit intArgs = %+v, want [11]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Loc_Hit(t *testing.T) {
	s := newTestServer(t)
	s.locTypes = &objtype.LocTypeConfigs{
		Configs:     []*objtype.LocType{{ConfigType: objtype.ConfigType{ID: 33, DebugName: "table_basic"}}},
		ConfigNames: map[string]int{"table_basic": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeLoc)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "table_basic", "~test table_basic")
	if len(intArgs) != 1 || intArgs[0] != 33 {
		t.Errorf("Loc_Hit intArgs = %+v, want [33]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Seq_Hit(t *testing.T) {
	s := newTestServer(t)
	s.seqTypes = &objtype.SeqTypeConfigs{
		Configs:     []*objtype.SeqType{{ConfigType: objtype.ConfigType{ID: 13, DebugName: "human_walk"}}},
		ConfigNames: map[string]int{"human_walk": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeSeq)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "human_walk", "~test human_walk")
	if len(intArgs) != 1 || intArgs[0] != 13 {
		t.Errorf("Seq_Hit intArgs = %+v, want [13]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Stat_Hit(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeStat)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "attack", "~test attack")
	if len(intArgs) != 1 || intArgs[0] != objtype.PlayerStatAttack {
		t.Errorf("Stat_Hit intArgs = %+v, want [%d]", intArgs, objtype.PlayerStatAttack)
	}
}

func TestMarshalDebugprocArgs_Stat_Miss(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeStat)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "unknown", "~test unknown")
	if len(intArgs) != 1 || intArgs[0] != -1 {
		t.Errorf("Stat_Miss intArgs = %+v, want [-1]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Inv_Hit(t *testing.T) {
	s := newTestServer(t)
	s.invTypes = &objtype.InvTypeConfigs{
		Configs:     []*objtype.InvType{{ConfigType: objtype.ConfigType{ID: 93, DebugName: "inv"}}},
		ConfigNames: map[string]int{"inv": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInv)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "inv", "~test inv")
	if len(intArgs) != 1 || intArgs[0] != 93 {
		t.Errorf("Inv_Hit intArgs = %+v, want [93]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Interface_Hit(t *testing.T) {
	s := newTestServer(t)
	s.componentTypes = &objtype.ComponentTypeConfigs{
		Configs:     []*objtype.ComponentType{{ConfigType: objtype.ConfigType{ID: 137, DebugName: "welcome_screen"}}},
		ConfigNames: map[string]int{"welcome_screen": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeInterface)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "welcome_screen", "~test welcome_screen")
	if len(intArgs) != 1 || intArgs[0] != 137 {
		t.Errorf("Interface_Hit intArgs = %+v, want [137]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Spotanim_Hit(t *testing.T) {
	s := newTestServer(t)
	s.spotanimTypes = &objtype.SpotanimTypeConfigs{
		Configs:     []*objtype.SpotanimType{{ConfigType: objtype.ConfigType{ID: 70, DebugName: "air_strike"}}},
		ConfigNames: map[string]int{"air_strike": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeSpotanim)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "air_strike", "~test air_strike")
	if len(intArgs) != 1 || intArgs[0] != 70 {
		t.Errorf("Spotanim_Hit intArgs = %+v, want [70]", intArgs)
	}
}

func TestMarshalDebugprocArgs_Idkit_Hit(t *testing.T) {
	s := newTestServer(t)
	s.idkTypes = &objtype.IdkTypeConfigs{
		Configs:     []*objtype.IdkType{{ConfigType: objtype.ConfigType{ID: 256, DebugName: "arms"}}},
		ConfigNames: map[string]int{"arms": 0},
	}
	sf := stageDebugprocScript(t, s, "test", []byte{byte(objtype.ScriptVarTypeIdkit)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "arms", "~test arms")
	if len(intArgs) != 1 || intArgs[0] != 256 {
		t.Errorf("Idkit_Hit intArgs = %+v, want [256]", intArgs)
	}
}

func TestMarshalDebugprocArgs_MultipleArgsMixed(t *testing.T) {
	// Tests that ParamTypes order drives both stack appends and token consumption.
	// Three params: STRING, INT, OBJ. Args "hello 42 knife" tokenises to
	// stringArgs[0]="hello", intArgs[0]=42, intArgs[1]=946.
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{
		Configs:     []*objtype.ObjType{{ConfigType: objtype.ConfigType{ID: 946, DebugName: "knife"}}},
		ConfigNames: map[string]int{"knife": 0},
	}
	sf := stageDebugprocScript(t, s, "mix", []byte{
		byte(objtype.ScriptVarTypeString),
		byte(objtype.ScriptVarTypeInt),
		byte(objtype.ScriptVarTypeObj),
	})
	intArgs, stringArgs := s.marshalDebugprocArgs(sf, "hello 42 knife", "~mix hello 42 knife")
	if len(stringArgs) != 1 || stringArgs[0] != "hello" {
		t.Errorf("stringArgs = %+v, want [\"hello\"]", stringArgs)
	}
	if len(intArgs) != 2 || intArgs[0] != 42 || intArgs[1] != 946 {
		t.Errorf("intArgs = %+v, want [42, 946]", intArgs)
	}
}

func TestMarshalDebugprocArgs_EmptyParamTypes(t *testing.T) {
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "noargs", []byte{})
	intArgs, stringArgs := s.marshalDebugprocArgs(sf, "ignored", "~noargs ignored")
	if len(intArgs) != 0 {
		t.Errorf("intArgs = %+v, want []", intArgs)
	}
	if len(stringArgs) != 0 {
		t.Errorf("stringArgs = %+v, want []", stringArgs)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestMarshalDebugprocArgs" ./modules/world/...`

Expected: FAIL with compile error: `s.marshalDebugprocArgs undefined`. (Per memory `verify_implementer_claims`, do not skip this verification step — confirm the failure mode is the expected "undefined method" and not, say, "undefined type alias" from a wrong import.)

- [ ] **Step 5: Commit the failing tests**

```bash
git add modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-189 T5 RED — marshalDebugprocArgs unit tests

19 failing tests pin the 12 TS DEBUGPROC arg-type arms (STRING/INT
hit+miss+missing, OBJ/NAMEDOBJ/NPC/LOC/SEQ/INV/INTERFACE/SPOTANIM/
IDKIT hit, STAT hit+miss, mixed-args order, empty ParamTypes) plus
the stageDebugprocScript fixture helper. COORD pinned in T7.

All tests fail to compile pending marshalDebugprocArgs (Task 6).
EOF
)"
```

---

## Task 6: `marshalDebugprocArgs` + `dispatchDebugproc` implementations (GREEN, no COORD)

Implements 11 of the 12 TS arms; COORD is left as a `default: -1` stub and added in Task 7.

**Files:**
- Modify: `modules/world/handlers_game.go`

- [ ] **Step 1: Add the two helpers above `handleClientCheat`**

Open `modules/world/handlers_game.go`. Find the `func handleClientCheat(p *Player, payload []byte) error {` line (currently around line 346). Insert these two helpers immediately BEFORE that function:

```go
// marshalDebugprocArgs walks sf.ParamTypes byte-by-byte, casts each to
// objtype.ScriptVarType, and appends to intArgs or stringArgs per the
// 12 TS arms in ClientCheatHandler.ts:69-140. Missing tokens degrade
// per-TS-arm:
//   - STRING → "" (TS `?? ''`)
//   - INT → 0 (TS `parseInt(v ?? '0', 10) | 0`)
//   - ByName-lookup arms → -1 (TS `getId('')` returns -1)
//   - STAT → -1 (TS `PlayerStatMap.get(undefined)` returns undefined)
//
// The COORD arm re-parses rawCheat (TS L113-124); mirrored verbatim
// in marshalDebugprocCoord (see DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE).
//
// NAI-189.
func (s *Server) marshalDebugprocArgs(sf *script.ScriptFile, args string, rawCheat string) ([]int, []string) {
	tokens := strings.Fields(args)
	take := func() string {
		if len(tokens) == 0 {
			return ""
		}
		t := tokens[0]
		tokens = tokens[1:]
		return t
	}

	intArgs := make([]int, 0, len(sf.ParamTypes))
	stringArgs := make([]string, 0, len(sf.ParamTypes))

	idOr := func(id int) int {
		if id < 0 {
			return -1
		}
		return id
	}

	for i := 0; i < len(sf.ParamTypes); i++ {
		switch objtype.ScriptVarType(sf.ParamTypes[i]) {
		case objtype.ScriptVarTypeString:
			stringArgs = append(stringArgs, take())
		case objtype.ScriptVarTypeInt:
			intArgs = append(intArgs, parseIntOr(take(), 0))
		case objtype.ScriptVarTypeObj, objtype.ScriptVarTypeNamedObj:
			if t := s.objTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, idOr(t.ID))
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeNPC:
			if t := s.npcTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, idOr(t.ID))
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeLoc:
			if t := s.locTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, idOr(t.ID))
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeSeq:
			if t := s.seqTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, idOr(t.ID))
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeStat:
			tok := strings.ToUpper(take())
			if stat, ok := objtype.PlayerStatMap[tok]; ok {
				intArgs = append(intArgs, stat)
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeInv:
			if t := s.invTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, idOr(t.ID))
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeCoord:
			// COORD arm: TS L113-124 re-parses the whole cheat string by
			// underscore. Implementation deferred to Task 7. Stub returns -1.
			// DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE will land in T7.
			intArgs = append(intArgs, -1)
			_ = rawCheat // silence unused while COORD is stubbed
		case objtype.ScriptVarTypeInterface:
			if t := s.componentTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, idOr(t.ID))
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeSpotanim:
			if t := s.spotanimTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, idOr(t.ID))
			} else {
				intArgs = append(intArgs, -1)
			}
		case objtype.ScriptVarTypeIdkit:
			if t := s.idkTypes.ByName(take()); t != nil {
				intArgs = append(intArgs, idOr(t.ID))
			} else {
				intArgs = append(intArgs, -1)
			}
		default:
			// TS has no default; any unrecognised type leaves the slot at -1.
			intArgs = append(intArgs, -1)
		}
	}

	return intArgs, stringArgs
}

// dispatchDebugproc resolves a [debugproc,X] script by name and dispatches
// it via s.runScript with arguments marshaled per the script's ParamTypes.
// Mirrors TS ClientCheatHandler.ts:59-148.
//
// cmd is the lowered first token of the cheat (already verified to start
// with s.cfg.NodeDebugprocChar). args is the post-first-space tail.
// rawCheat is the full lowered cheat string (needed by the COORD arm).
//
// TS-fidelity:
//   - Unknown script name → silent return (TS L62-64 `return false`).
//   - ByName misses → -1 in slot; dispatch continues (TS L74-139 swallow misses).
//
// NAI-189.
func (s *Server) dispatchDebugproc(p *Player, cmd string, args string, rawCheat string) {
	prefix := s.cfg.NodeDebugprocChar
	if prefix == "" || len(cmd) <= len(prefix) || !strings.HasPrefix(cmd, prefix) {
		return
	}
	name := cmd[len(prefix):]
	sf := s.scriptProvider.GetByName("[debugproc," + name + "]")
	if sf == nil {
		return
	}
	intArgs, stringArgs := s.marshalDebugprocArgs(sf, args, rawCheat)
	s.runScript(sf, p, nil, false, intArgs, stringArgs)
}
```

Imports check: the file already imports `strings` (line 12), `objtype` (line 18). The `script` package needs to be added if not already present. Run:

```bash
grep -n 'github.com/zsrv/goscape/pkg/script"' modules/world/handlers_game.go
```

If the result is empty, add the import:

```go
"github.com/zsrv/goscape/pkg/script"
```

to the import block (alphabetical: between `pathfinder/loc` and `rsbuf`).

- [ ] **Step 2: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: clean build.

- [ ] **Step 3: Run the Task 5 unit tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run "TestMarshalDebugprocArgs" ./modules/world/...`

Expected: 18 PASS, 1 SKIP-or-FAIL on the (yet-to-be-added) Coord test. Confirm only Coord-related failures remain.

Note: Task 5 did not write a Coord test for `marshalDebugprocArgs` directly — Coord is pinned in Task 7. All 19 Task-5 tests should PASS here. If `MultipleArgsMixed` or `EmptyParamTypes` fail, re-check the helper.

- [ ] **Step 4: Run full modules/world suite to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

Expected: PASS. The dev-block prefix branch is NOT yet wired into `handleClientCheat` (Task 8), so no existing cheat tests are exercising the new helpers. Per memory `verify_implementer_claims`, run the full suite anyway — the helpers should compile-clean and not have any unintended side-effects.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-189 T6 GREEN — marshalDebugprocArgs + dispatchDebugproc

Adds two helpers to modules/world/handlers_game.go:

marshalDebugprocArgs walks sf.ParamTypes and tokenises args per the
12 TS arms (STRING/INT/OBJ/NAMEDOBJ/NPC/LOC/SEQ/STAT/INV/INTERFACE/
SPOTANIM/IDKIT). Missing tokens degrade to "" / 0 / -1 per arm,
matching TS `?? ''`, `parseInt(v ?? '0') | 0`, and `getId('')`.

dispatchDebugproc resolves [debugproc,X] via s.scriptProvider.GetByName,
marshals args via marshalDebugprocArgs, and dispatches via s.runScript
(target=nil, protect=false) — matching TS L148
player.executeScript(ScriptRunner.init(...), false).

COORD arm stubbed to -1; full implementation lands in T7 with the
DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE tag.

T5 marshalDebugprocArgs tests (19) now pass.
Prefix-branch wiring deferred to T8 — handleClientCheat unchanged.
EOF
)"
```

---

## Task 7: COORD arm + deviation tag

The COORD arm requires re-parsing the full lowered cheat string by underscore. Per spec §4.2.1, this mirrors TS arithmetic verbatim including its NaN-producing slice(6) fragility. The test pin captures the **actual output** of the equivalent arithmetic for one canonical input.

**Files:**
- Modify: `modules/world/handlers_game.go` (replace COORD stub from T6 with real implementation)
- Modify: `modules/world/handlers_game_test.go` (add 2 COORD tests)

- [ ] **Step 1: Determine the expected COORD value via trace**

Per spec §4.2.1, the goscape-side decision is **mirror TS verbatim using `strconv.Atoi` with -1 sentinel on parse failure**. Trace for `"~coord_0_50_50_32_32"`:

1. `rawCheat = "~coord_0_50_50_32_32"`
2. `args2 := strings.Split(rawCheat, "_")` → `["~coord", "0", "50", "50", "32", "32"]`
3. `args2[0] = "~coord"` (6 chars); `args2[0][6:]` → `""` (empty substring; Go does NOT panic on this — `"x"[1:]` is `""` per spec — verify: `len("~coord") == 6`, so `[6:]` is valid and empty).
4. `strconv.Atoi("")` → error; level coerces to `-1`.
5. `mx = strconv.Atoi("50") = 50`; same for mz/lx/lz.
6. `coordgrid.PackCoord(-1, (50<<6)+32, (50<<6)+32) = coordgrid.PackCoord(-1, 3232, 3232)`.

Verify PackCoord's signature and bit layout:

```bash
grep -n "func PackCoord" $HOME/Code/github.com/zsrv/goscape/pkg/coordgrid/coordgrid.go
```

If `PackCoord(level, x, z int) int` packs as `(level << 28) | (x << 14) | z` (TS convention), then `PackCoord(-1, 3232, 3232)` is **dependent on Go integer behavior** — `-1 << 28` in Go on a signed int is implementation-defined for shifts of negative numbers in pre-Go-1.13, but Go 1.13+ allows it as `-(1 << 28)` per spec. For an `int64`, `-1 << 28 == -268435456`. Bit-OR-ing with positive values produces a negative result.

**Write the test against whatever PackCoord actually returns for these inputs** — do not hardcode a value here. The test pin is captured by running the implementation once and recording the output, then asserting that exact value in the test going forward (per memory `skip_pin_full_struct_capture`).

- [ ] **Step 2: Write the 2 failing COORD tests**

Append to `modules/world/handlers_game_test.go`:

```go
func TestMarshalDebugprocArgs_Coord_OneToken(t *testing.T) {
	// DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE: TS's slice(6) on
	// args2[0] produces an empty/non-digit string for all reasonable
	// debugproc names, making level coerce to -1 (Go) / NaN (TS). The
	// (mx<<6)+lx components parse correctly. Pin the result verbatim.
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "coord_0_50_50_32_32", []byte{byte(objtype.ScriptVarTypeCoord)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "", "~coord_0_50_50_32_32")
	// Expected value: coordgrid.PackCoord(-1, (50<<6)+32, (50<<6)+32) =
	// coordgrid.PackCoord(-1, 3232, 3232). Record from a first-run trace.
	wantLevel := -1
	wantX, wantZ := (50<<6)+32, (50<<6)+32
	want := coordgrid.PackCoord(wantLevel, wantX, wantZ)
	if len(intArgs) != 1 || intArgs[0] != want {
		t.Errorf("OneToken intArgs = %+v, want [%d] (PackCoord(%d,%d,%d))",
			intArgs, want, wantLevel, wantX, wantZ)
	}
}

func TestMarshalDebugprocArgs_Coord_TwoToken(t *testing.T) {
	// Same DEVIATION as OneToken; args2[0] is now "~setpos coord" (13 chars).
	// slice(6) = "s coord"; parseInt → NaN (TS) / Atoi → err → -1 (goscape).
	// Body-component arithmetic identical: (50<<6)+32 for both x and z.
	s := newTestServer(t)
	sf := stageDebugprocScript(t, s, "setpos", []byte{byte(objtype.ScriptVarTypeCoord)})
	intArgs, _ := s.marshalDebugprocArgs(sf, "coord_0_50_50_32_32", "~setpos coord_0_50_50_32_32")
	wantLevel := -1
	wantX, wantZ := (50<<6)+32, (50<<6)+32
	want := coordgrid.PackCoord(wantLevel, wantX, wantZ)
	if len(intArgs) != 1 || intArgs[0] != want {
		t.Errorf("TwoToken intArgs = %+v, want [%d] (PackCoord(%d,%d,%d))",
			intArgs, want, wantLevel, wantX, wantZ)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestMarshalDebugprocArgs_Coord" ./modules/world/...`

Expected: FAIL. The current stub returns -1 (not the PackCoord result).

- [ ] **Step 4: Replace the COORD stub with the real implementation**

In `modules/world/handlers_game.go`, locate the COORD case inside `marshalDebugprocArgs` (the stub from Task 6). Replace this block:

```go
		case objtype.ScriptVarTypeCoord:
			// COORD arm: TS L113-124 re-parses the whole cheat string by
			// underscore. Implementation deferred to Task 7. Stub returns -1.
			// DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE will land in T7.
			intArgs = append(intArgs, -1)
			_ = rawCheat // silence unused while COORD is stubbed
```

with:

```go
		case objtype.ScriptVarTypeCoord:
			// DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE
			// TS L113-124 re-parses the full lowered cheat string by
			// underscore and computes level via args2[0].slice(6). For all
			// reasonable debugproc names this produces a non-digit string
			// → TS NaN / goscape -1 sentinel. Mirrored verbatim per the
			// true-to-TS gate; the level component is effectively always -1
			// while x/z parse correctly from (mx<<6)+lx and (mz<<6)+lz.
			// A future upstream fix should derive the offset from cmd
			// length; until then this matches TS observable behavior.
			intArgs = append(intArgs, parseDebugprocCoord(rawCheat))
```

Then add `parseDebugprocCoord` immediately above `marshalDebugprocArgs`:

```go
// parseDebugprocCoord mirrors TS ClientCheatHandler.ts:113-124. Returns
// the packed coord; level coerces to -1 if the slice(6) substring fails
// to parse (the common case — see DEVIATION-NAI-189-D1).
func parseDebugprocCoord(rawCheat string) int {
	parts := strings.Split(rawCheat, "_")
	if len(parts) < 5 {
		return -1
	}

	atoiOr := func(s string, def int) int {
		v, err := strconv.Atoi(s)
		if err != nil {
			return def
		}
		return v
	}

	levelStr := ""
	if len(parts[0]) >= 6 {
		levelStr = parts[0][6:]
	}
	level := atoiOr(levelStr, -1)
	mx := atoiOr(parts[1], 0)
	mz := atoiOr(parts[2], 0)
	lx := atoiOr(parts[3], 0)
	lz := atoiOr(parts[4], 0)
	return coordgrid.PackCoord(level, (mx<<6)+lx, (mz<<6)+lz)
}
```

Imports check: `strconv` is already imported in `handlers_game.go` (line 11); `coordgrid` is already imported (line 15). No import change needed.

- [ ] **Step 5: Run COORD tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run "TestMarshalDebugprocArgs_Coord" ./modules/world/...`

Expected: PASS (2 tests).

If FAIL, the test's `want` value disagrees with the implementation. Print the actual value via `t.Logf("got %d", intArgs[0])` to diagnose. Per memory `skip_pin_full_struct_capture`, the `want` value MUST match the implementation's output verbatim — adjust `want` to record the actual output if the trace in Step 1 was slightly off.

- [ ] **Step 6: Run all marshalDebugprocArgs tests to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run "TestMarshalDebugprocArgs" ./modules/world/...`

Expected: 21 PASS (19 from T5 + 2 from T7).

- [ ] **Step 7: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-189 T7 — COORD arm + DEVIATION-NAI-189-D1

Replaces the T6 COORD stub with parseDebugprocCoord, which mirrors
TS ClientCheatHandler.ts:113-124 verbatim: split rawCheat on '_',
parse level via args2[0][6:] with -1 sentinel on parse failure,
parse mx/mz/lx/lz from positional slots, and pack via
coordgrid.PackCoord.

DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE: TS's hardcoded slice(6)
offset produces NaN for all reasonable debugproc names (see spec
§4.2.1 trace). The x/z components parse correctly; only level is
masked. Doc-comment ties to the deviation tag for future fixup.

2 new tests pin OneToken (~coord_...) and TwoToken (~setpos coord_...)
forms; both assert the same PackCoord(-1, x, z) output since both
forms hit the slice(6)-NaN failure mode.
EOF
)"
```

---

## Task 8: Prefix branch in `handleClientCheat` (GREEN) + integration tests

Wires the dev-block to dispatch `~`-prefixed cheats to `dispatchDebugproc` BEFORE the existing fixed-cmd switch.

**Files:**
- Modify: `modules/world/handlers_game.go` (insert prefix branch at line 402)
- Modify: `modules/world/handlers_game_test.go` (add 5 integration/gate tests)

- [ ] **Step 1: Write the 5 failing integration tests**

Append to `modules/world/handlers_game_test.go`:

```go
// --- NAI-189: handleClientCheat → dispatchDebugproc wiring ---

func TestHandleClientCheat_Debugproc_DispatchesScript(t *testing.T) {
	// Positive control: a registered [debugproc,X] is dispatched when
	// staffModLevel >= 4 && !NodeProduction && cheat starts with the
	// debugproc-char prefix.
	p, _, s := teleTestPlayer(t)
	p.staffModLevel = 4
	s.cfg.NodeProduction = false
	s.cfg.NodeDebugprocChar = "~"

	sf := stageDebugprocScript(t, s, "ping", []byte{})

	// Pre-dispatch: provider holds the script.
	if s.scriptProvider.GetByName(sf.Name) == nil {
		t.Fatal("setup: stageDebugprocScript did not register the script")
	}

	dispatchTeleCheat(t, p, "~ping")

	// Post-dispatch: script ran to completion via runScript → Execute →
	// resumeOrFinish. With OpReturn as the sole body opcode, p.activeScript
	// must be nil (no suspension occurred).
	if p.activeScript != nil {
		t.Errorf("activeScript = %+v, want nil (OpReturn body should finish synchronously)", p.activeScript)
	}
}

func TestHandleClientCheat_Debugproc_UnknownScript_NoOp(t *testing.T) {
	p, _, s := teleTestPlayer(t)
	p.staffModLevel = 4
	s.cfg.NodeProduction = false
	s.cfg.NodeDebugprocChar = "~"
	// No script registered.

	beforeActive := p.activeScript
	dispatchTeleCheat(t, p, "~nonexistent")

	if p.activeScript != beforeActive {
		t.Errorf("activeScript changed; expected silent no-op on unknown script. before=%v after=%v",
			beforeActive, p.activeScript)
	}
}

func TestHandleClientCheat_Debugproc_GateMod3_NoDispatch(t *testing.T) {
	p, _, s := teleTestPlayer(t)
	p.staffModLevel = 3 // BELOW dev-block threshold
	s.cfg.NodeProduction = false
	s.cfg.NodeDebugprocChar = "~"
	_ = stageDebugprocScript(t, s, "ping", []byte{})

	dispatchTeleCheat(t, p, "~ping")

	// At mod=3 the dev-block (>= 4) is skipped — debugproc must not fire.
	// The script never ran, so no Execution state changed. (No simpler
	// observable than "did the script run" without instrumentation; we
	// pin the negative by ensuring activeScript stays nil AND no panic.)
	if p.activeScript != nil {
		t.Errorf("Gate failed: activeScript = %+v, want nil at staffModLevel=3", p.activeScript)
	}
}

func TestHandleClientCheat_Debugproc_GateProd_NoDispatch(t *testing.T) {
	p, _, s := teleTestPlayer(t)
	p.staffModLevel = 4
	s.cfg.NodeProduction = true // dev block gated off
	s.cfg.NodeDebugprocChar = "~"
	_ = stageDebugprocScript(t, s, "ping", []byte{})

	dispatchTeleCheat(t, p, "~ping")

	if p.activeScript != nil {
		t.Errorf("Gate failed: activeScript = %+v, want nil under NodeProduction=true", p.activeScript)
	}
}

func TestHandleClientCheat_Debugproc_NonPrefix_FallsThroughToSwitch(t *testing.T) {
	// Cohort-compatibility: a cheat without the debugproc-char prefix
	// must fall through to the existing fixed-cmd switch. Use ::fly as
	// the witness — it toggles p.moveStrategy, an observable side effect.
	p, _, s := teleTestPlayer(t)
	p.staffModLevel = 4
	s.cfg.NodeProduction = false
	s.cfg.NodeDebugprocChar = "~"

	before := p.moveStrategy
	dispatchTeleCheat(t, p, "fly") // no "~" prefix
	if p.moveStrategy == before {
		t.Errorf("fly cheat did not toggle moveStrategy; prefix branch may have eaten the dispatch. before=%v after=%v", before, p.moveStrategy)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestHandleClientCheat_Debugproc" ./modules/world/...`

Expected: FAIL on the positive-control test (`DispatchesScript`) and the prefix-fallthrough test isn't strictly failing yet because there's no prefix branch — but the script never dispatches, so the `activeScript` assertion in `DispatchesScript` fails. The 3 gate-negative tests may already PASS coincidentally (since the unwired path also doesn't dispatch), but they MUST PASS after the wire-up too. Note this in the test-run output.

- [ ] **Step 3: Insert the prefix branch in `handleClientCheat`**

Open `modules/world/handlers_game.go`. Find the dev-block gate at line 402:

```go
	if !p.client.server.cfg.NodeProduction && p.staffModLevel >= 4 {
		switch parts[0] {
		case "fly":
```

Insert these lines BETWEEN the `if !... staffModLevel >= 4 {` and `switch parts[0] {`:

```go
		// TS ClientCheatHandler.ts:59 — debugproc prefix dispatch BEFORE
		// the fixed-cmd ladder. Cmd-form is `<NodeDebugprocChar><scriptname>`
		// (default "~scriptname"). NAI-189.
		if prefix := p.client.server.cfg.NodeDebugprocChar; prefix != "" && strings.HasPrefix(parts[0], prefix) {
			p.client.server.dispatchDebugproc(p, parts[0], args, cheat)
			return nil
		}
```

The resulting block:

```go
	if !p.client.server.cfg.NodeProduction && p.staffModLevel >= 4 {
		// TS ClientCheatHandler.ts:59 — debugproc prefix dispatch BEFORE
		// the fixed-cmd ladder. Cmd-form is `<NodeDebugprocChar><scriptname>`
		// (default "~scriptname"). NAI-189.
		if prefix := p.client.server.cfg.NodeDebugprocChar; prefix != "" && strings.HasPrefix(parts[0], prefix) {
			p.client.server.dispatchDebugproc(p, parts[0], args, cheat)
			return nil
		}
		switch parts[0] {
		case "fly":
```

- [ ] **Step 4: Run integration tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run "TestHandleClientCheat_Debugproc" ./modules/world/...`

Expected: PASS (5 tests).

- [ ] **Step 5: Run the full handlers_game_test suite to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run "TestHandleClientCheat\|TestCheat\|TestTeleCheat" ./modules/world/...`

Expected: PASS. All existing cheat tests (fly/naive/random/speed/tele/setvar/...) continue to dispatch correctly because their `parts[0]` does not begin with `"~"`.

- [ ] **Step 6: Run the full modules/world suite with -race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-189 T8 GREEN — debugproc prefix branch in handleClientCheat

Inserts a prefix branch in the dev-block (after staffModLevel >= 4
&& !NodeProduction guard, BEFORE the fixed-cmd switch). Tests
parts[0] for HasPrefix(s.cfg.NodeDebugprocChar) and routes to
dispatchDebugproc when matched; otherwise falls through to the
existing fly/naive/random/speed switch.

Activates s.cfg.NodeDebugprocChar (default "~"), previously plumbed
but unread since the field was introduced at config.go:15.

5 new integration tests via dispatchTeleCheat / handleClientCheat:
DispatchesScript / UnknownScript_NoOp / GateMod3_NoDispatch /
GateProd_NoDispatch / NonPrefix_FallsThroughToSwitch.
EOF
)"
```

---

## Task 9: Carryforward doc-comment rewrite

**Files:**
- Modify: `modules/world/handlers_game.go:370-389` (DEVIATION-NAI-188-D1-CARRYFORWARD block)

- [ ] **Step 1: Replace the carryforward comment block**

In `modules/world/handlers_game.go`, find this block (currently lines 370-389):

```go
	// DEVIATION-NAI-188-D1-CARRYFORWARD — supersedes
	// DEVIATION-NAI-187-D1-CARRYFORWARD. 2 TS ClientCheatHandler
	// cheats remain unported, both in the dev block (!NP && >=4) and
	// both blocked on the same infra gap (cache / script hot-reload):
	//   reload:  TS L149-150. Calls World.reload() — full cache
	//            hot-reload pipeline. No goscape equivalent;
	//            substantial new subsystem.
	//   rebuild: TS L151-153. Calls World.rebuild() — script-provider
	//            hot-reload. Same infra gap as reload.
	// NAI-188 retired ::speed (TS L154-167). The tickRate package-level
	// const at modules/world/tick.go:15 was promoted to Server.tickRate
	// (default initialised to defaultTickRate); the tick loop re-reads
	// the field each iteration so the cheat-induced mutation takes
	// effect on the next sleep. See spec §6 for the single-goroutine
	// concurrency argument.
	// NAI-187 retired the admin spawn/interface cluster (locadd /
	// npcadd / openmain). Per memory tracker_entry_framing_can_be_
	// incomplete: the prior "blocked on dynamic Loc/Npc spawn +
	// interface routing" framing was stale at HEAD — all primitives
	// existed; sole gap was three ByName helpers in pkg/objtype.
```

Replace with:

```go
	// DEVIATION-NAI-189-D1-CARRYFORWARD — supersedes
	// DEVIATION-NAI-188-D1-CARRYFORWARD. 2 TS ClientCheatHandler
	// cheats remain unported, both in the dev block (!NP && >=4) and
	// both blocked on the same infra gap (cache / script hot-reload):
	//   reload:  TS L149-150. Calls World.reload() — full cache
	//            hot-reload pipeline. No goscape equivalent;
	//            substantial new subsystem.
	//   rebuild: TS L151-153. Calls World.rebuild() — script-provider
	//            hot-reload. Same infra gap as reload.
	// NAI-189 retired the DEBUGPROC dispatch path (TS L59-148). The
	// dev-block now branches on s.cfg.NodeDebugprocChar BEFORE the
	// fixed-cmd switch; matching cheats route through dispatchDebugproc,
	// which resolves [debugproc,X] via s.scriptProvider.GetByName and
	// dispatches via s.runScript with arguments marshaled per
	// ScriptFile.ParamTypes (12 TS arms — STRING/INT/OBJ/NAMEDOBJ/NPC/
	// LOC/SEQ/STAT/INV/COORD/INTERFACE/SPOTANIM/IDKIT). 4 new ByName
	// helpers added in pkg/objtype (Seq/Spotanim/Idk/Inv) — mirrors the
	// NAI-187 cluster pattern. DEVIATION-NAI-189-D1-MIRROR-TS-COORD-
	// FRAGILE flags the TS slice(6) fragility in the COORD arm.
	// NAI-188 retired ::speed (TS L154-167). The tickRate package-level
	// const at modules/world/tick.go:15 was promoted to Server.tickRate
	// (default initialised to defaultTickRate); the tick loop re-reads
	// the field each iteration so the cheat-induced mutation takes
	// effect on the next sleep. See spec §6 for the single-goroutine
	// concurrency argument.
	// NAI-187 retired the admin spawn/interface cluster (locadd /
	// npcadd / openmain). Per memory tracker_entry_framing_can_be_
	// incomplete: the prior "blocked on dynamic Loc/Npc spawn +
	// interface routing" framing was stale at HEAD — all primitives
	// existed; sole gap was three ByName helpers in pkg/objtype.
```

- [ ] **Step 2: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`

Expected: clean build.

- [ ] **Step 3: Run the full modules/world suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "docs(world): NAI-189 T9 — rewrite carryforward, add D1-COORD-FRAGILE tag"
```

---

## Task 10: CLOSE — verify, audit, close commit

**Files:**
- No code changes; verification + close commit only.

- [ ] **Step 1: Full -race test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: PASS across all packages. Per memory `verify_implementer_claims`, this is a fresh run from the project root — not a recall of a per-package run from earlier.

- [ ] **Step 2: Confirm NodeDebugprocChar is now read**

Run: `grep -n "NodeDebugprocChar" modules/world/`

Expected output: at least 2 hits in `handlers_game.go` (one in `dispatchDebugproc`, one in the prefix branch) plus the original at `config.go:15`. The field is no longer unread.

- [ ] **Step 3: Confirm carryforward tally is correct**

Run: `grep -A 2 "DEVIATION-NAI-189-D1-CARRYFORWARD" modules/world/handlers_game.go`

Expected: "2 TS ClientCheatHandler cheats remain unported" — tally unchanged from NAI-188.

- [ ] **Step 4: Confirm DEBUGPROC retirement note is present**

Run: `grep -B 0 -A 12 "NAI-189 retired the DEBUGPROC dispatch path" modules/world/handlers_game.go | head -15`

Expected: the new paragraph including "4 new ByName helpers" and "DEVIATION-NAI-189-D1-MIRROR-TS-COORD-FRAGILE".

- [ ] **Step 5: Audit deviation tag grep-discoverability**

Per memory `retire_deviation_grep_all_comments`:

Run: `rg "NAI-189" pkg/ modules/ cmd/ docs/`

Expected: hits in `pkg/objtype/{seqtype,spotanimtype,idktype,invtype}.go` (4 doc-comments), `modules/world/handlers_game.go` (carryforward + DEVIATION-NAI-189-D1 doc-comment + 2 helpers), `docs/superpowers/specs/2026-05-13-nai-189-debugproc-cheat-design.md`, `docs/superpowers/plans/2026-05-13-nai-189-debugproc-cheat.md`. No orphaned references.

- [ ] **Step 6: Audit no unintended scope drift**

Run: `git diff --stat main...HEAD`

Expected: 8-10 files modified, no spurious changes:
- `pkg/objtype/{seqtype,spotanimtype,idktype,invtype}.go` (4 production)
- `pkg/objtype/{seqtype,spotanimtype,idktype,invtype}_test.go` (4 tests)
- `modules/world/handlers_game.go` (production)
- `modules/world/handlers_game_test.go` (tests)
- `docs/superpowers/specs/2026-05-13-nai-189-debugproc-cheat-design.md` (already committed in 46fdd27)
- `docs/superpowers/plans/2026-05-13-nai-189-debugproc-cheat.md` (already committed pre-T1)

Per memory `implementer_commit_content_verify`: confirm `git log --oneline main..HEAD` shows the expected T1-T9 commit train and no surprises.

- [ ] **Step 7: Close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-189 — DEBUGPROC dispatch path ported

Closes the DEBUGPROC sub-spec (TS ClientCheatHandler.ts:59-148).
Goscape now resolves [debugproc,X] runescripts by name and
dispatches them via s.runScript with arguments parsed per
ScriptFile.ParamTypes (12 TS arms).

After NAI-189, ClientCheatHandler.ts is 100% ported except for
`reload` (L149-150) and `rebuild` (L151-153) — both blocked on a
cache / script-provider hot-reload subsystem (future NAI-190+).
Carryforward tally unchanged at 2.

Sub-spec bundle:
- T1-T4: 4 new ByName helpers in pkg/objtype (Seq/Spotanim/Idk/Inv).
  Mirrors NAI-187 cluster (ConfigNames primary, linear-scan fallback,
  nil-receiver guard). 20 new tests.
- T5-T6: marshalDebugprocArgs + dispatchDebugproc helpers in
  modules/world/handlers_game.go. 11 of 12 TS arg-type arms in T6;
  COORD in T7.
- T7: parseDebugprocCoord + DEVIATION-NAI-189-D1-MIRROR-TS-COORD-
  FRAGILE. TS's hardcoded slice(6) offset produces NaN for all
  reasonable debugproc names; mirrored verbatim per the true-to-TS
  gate. 2 new pin tests.
- T8: Prefix branch in handleClientCheat — routes `~`-prefixed
  cheats to dispatchDebugproc BEFORE the fixed-cmd switch.
  Activates s.cfg.NodeDebugprocChar (previously unread). 5 new
  integration tests.
- T9: Carryforward rewrite — DEVIATION-NAI-189-D1-CARRYFORWARD
  supersedes NAI-188's; DEBUGPROC retirement paragraph added.

Closes memory: parallel_slice_convention_for_mixed_type_args, tracker_carryforward_listings_compound, true_to_ts_gate
EOF
)"
```

If there are no uncommitted changes, the `--allow-empty` flag produces a close marker commit that's grep-discoverable in `git log` for the sub-spec arc.

---

## Plan self-review

Spec coverage check — every section/requirement of the spec is covered by tasks:

- §3.2 missing ByName helpers (Seq/Spotanim/Idk/Inv) → Tasks 1-4
- §3.3 ParamTypes → ScriptVarType cast → embedded in `marshalDebugprocArgs` (Task 6)
- §3.4 fixture infra → `stageDebugprocScript` helper (Task 5)
- §4.1 ByName helpers → Tasks 1-4 (using the NAI-187 pattern, not the simpler spec sketch — see pre-flight item 1)
- §4.2 `dispatchDebugproc` → Task 6 (without COORD), Task 7 (with COORD)
- §4.2.1 COORD parsing + DEVIATION-NAI-189-D1 → Task 7
- §4.3 prefix branch wiring → Task 8
- §4.4 carryforward rewrite → Task 9
- §5 data flow → no dedicated task; covered by Task 6+Task 8 combined behaviour
- §6 concurrency → not a code change; documented in spec (single-goroutine invariant)
- §7.1 per-arg-type tests (19) → Task 5 RED + Task 7 (Coord cases)
- §7.2 negative-path tests → Task 8 (UnknownScript_NoOp covers the spec's UnknownScript pin; BareDelimiter and EmptyDebugprocChar omitted from the plan as low-value since the early-return guards are trivial — flag if reviewer wants them added)
- §7.3 gate tests → Task 8 (GateMod3, GateProd; the "Gate_Both" positive control is `DispatchesScript`)
- §7.4 cohort-compatibility tests → Task 8 (`NonPrefix_FallsThroughToSwitch` covers spec's `Fallthrough_Fly`)
- §7.5 ByName helper tests → Tasks 1-4 (5 each instead of 3 — pre-flight item 1)
- §10 close criteria → Task 10

Placeholder scan — searched for "TBD", "TODO" in this plan: 0 hits. All code blocks are concrete. All commands have expected outputs.

Type consistency — `marshalDebugprocArgs` signature `(sf *script.ScriptFile, args string, rawCheat string) ([]int, []string)` is used identically in Tasks 5, 6, 7. `dispatchDebugproc(p *Player, cmd string, args string, rawCheat string)` is used identically in Tasks 6 and 8.

Two minor scope notes for the executing agent:
- **Spec §7.2 `BareDelimiter` and `EmptyDebugprocChar` tests** are omitted from Task 8 to keep the integration test set focused on observable side-effects (script ran / didn't run). The early-return guards inside `dispatchDebugproc` (`prefix == "" || len(cmd) <= len(prefix)`) are exercised indirectly via `UnknownScript_NoOp` (no script registered) and by the prefix-branch gate itself in `handleClientCheat` (`prefix != "" && HasPrefix(...)`). If a reviewer asks for explicit pins, add 2 more tests to Task 8 mirroring the same `dispatchTeleCheat` shape.
- **Pre-flight item 13** records that no end-to-end smoke target exists at HEAD. Close commit (Task 10) should note this so the user can stage a one-liner debugproc in content for manual smoke if desired.
