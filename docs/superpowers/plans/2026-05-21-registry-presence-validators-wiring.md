# Registry-Presence Validators Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the existing `checkNpcType` / `checkObjType` validators at 25 inline `Configs.X(id) == nil` script-input call sites in `handlers_config.go` (24 sites) + `handlers_obj.go` (1 site). Pure refactor — canonical `"%s: no XType with value (%d) found"` wording, zero behavior change.

**Architecture:** At each site, insert `if err := checkNpcType(s, id, "OP"); err != nil { return err }` (or `checkObjType`) BEFORE the existing `Configs.X(id)` deref. Keep `requireConfigs` early-check. Keep the local `nt` / `ot` var (every NC_/OC_ handler accesses one or more fields). Replace bespoke `"OP: unknown npc/obj id %d"` strings.

**Tech Stack:** Go 1.26.x; existing `pkg/script/` test infrastructure (`mockConfigs`, `runConfigOpExpectErr`).

**Spec:** `docs/superpowers/specs/2026-05-21-registry-presence-validators-wiring-design.md`

---

## File map

**Production files modified (2):**
- `pkg/script/handlers_config.go` — 9 NC_* wires (T1) + 10 OC_* Part A wires (T2) + 5 OC_* Part B wires (T3) = 24 sites
- `pkg/script/handlers_obj.go` — 1 OBJ_FIND wire (T4)

**Test files modified (3):**
- `pkg/script/handlers_config_test.go` — 1 assertion flip in T1 (NC_NAME at :869); 1 assertion flip in T2 (OC_NAME at :884)
- `pkg/script/handlers_obj_test.go` — add `TestCheckObjType` (T2); 1 assertion flip in T4 (TestObjFindUnknownObjId at :816-819)
- (No `handlers_npc_test.go` change — `TestCheckNpcType` already exists at :55.)

**Sites that stay unchanged (NO wiring this slice):**
- `handlers_inv.go` data-integrity guards (6 sites with `"invalid obj/inv id at slot"`)
- `handlers_obj.go` :386-388 (OBJ_NAME), :410-414 (OBJ_PARAM) — `s.ActiveObj.ObjType()` reads
- `handlers_npc.go` :241/:272/:305/:1333 — `s.ActiveNpc.NpcType()` reads
- `handlers_player.go` :1365 — `s.ActiveObj.ObjType()` read
- Corresponding test files retain their `"unknown obj id"` / `"unknown npc id"` wording at the non-candidate test sites.

---

## Task 1: Wire NC_* + NPC_PARAM family (9 sites)

**Files:**
- Modify: `pkg/script/handlers_config.go` lines 287-444 (9 handlers: handleNcName/Param/handleNpcParam/handleNcCategory/Desc/DebugName/Op/Size/VisLevel)
- Modify: `pkg/script/handlers_config_test.go:869` (1 assertion flip for NC_NAME)

- [ ] **Step 1: Update test assertion at `handlers_config_test.go:869` to canonical wording**

Replace:

```go
runConfigOpExpectErr(t, mc, OpNcName, []int{999}, "unknown npc id")
```

With:

```go
runConfigOpExpectErr(t, mc, OpNcName, []int{999}, "no NpcType with value (999) found")
```

- [ ] **Step 2: Run the assertion to confirm it FAILS (RED)**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run 'TestHandleNcName' -v
```

Expected: FAIL with mismatched error substring (production still emits `"unknown npc id 999"`).

If the test name is different, grep `handlers_config_test.go` for the test that wraps line 869: `awk '/^func Test/{f=$0} NR==869{print f}' pkg/script/handlers_config_test.go`.

- [ ] **Step 3: Wire `checkNpcType` at `handleNcName` (:287)**

Replace lines 291-295:

```go
	id := s.PopInt()
	nt := s.Configs.NpcType(id)
	if nt == nil {
		return fmt.Errorf("NC_NAME: unknown npc id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_NAME"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
```

- [ ] **Step 4: Wire `checkNpcType` at `handleNcParam` (:307)**

Replace lines 311-316:

```go
	paramID := s.PopInt()
	npcID := s.PopInt()
	nt := s.Configs.NpcType(npcID)
	if nt == nil {
		return fmt.Errorf("NC_PARAM: unknown npc id %d", npcID)
	}
```

With:

```go
	paramID := s.PopInt()
	npcID := s.PopInt()
	if err := checkNpcType(s, npcID, "NC_PARAM"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(npcID)
```

- [ ] **Step 5: Wire `checkNpcType` at `handleNpcParam` (:324)**

Replace lines 331-336:

```go
	paramID := s.PopInt()
	npcID := s.ActiveNpc.NpcType()
	nt := s.Configs.NpcType(npcID)
	if nt == nil {
		return fmt.Errorf("NPC_PARAM: unknown npc id %d", npcID)
	}
```

With:

```go
	paramID := s.PopInt()
	npcID := s.ActiveNpc.NpcType()
	if err := checkNpcType(s, npcID, "NPC_PARAM"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(npcID)
```

Note: `npcID` here is from `ActiveNpc.NpcType()` (stored entity state), not script input. Wiring is for wording canonicalization only — same data-source semantics.

- [ ] **Step 6: Wire `checkNpcType` at `handleNcCategory` (:341)**

Replace lines 345-349:

```go
	id := s.PopInt()
	nt := s.Configs.NpcType(id)
	if nt == nil {
		return fmt.Errorf("NC_CATEGORY: unknown npc id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_CATEGORY"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
```

- [ ] **Step 7: Wire `checkNpcType` at `handleNcDesc` (:355)**

Replace lines 359-363:

```go
	id := s.PopInt()
	nt := s.Configs.NpcType(id)
	if nt == nil {
		return fmt.Errorf("NC_DESC: unknown npc id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_DESC"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
```

- [ ] **Step 8: Wire `checkNpcType` at `handleNcDebugName` (:373)**

Replace lines 377-381:

```go
	id := s.PopInt()
	nt := s.Configs.NpcType(id)
	if nt == nil {
		return fmt.Errorf("NC_DEBUGNAME: unknown npc id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_DEBUGNAME"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
```

- [ ] **Step 9: Wire `checkNpcType` at `handleNcOp` (:395)**

Replace lines 403-407:

```go
	npcID := s.PopInt()
	nt := s.Configs.NpcType(npcID)
	if nt == nil {
		return fmt.Errorf("NC_OP: unknown npc id %d", npcID)
	}
```

With:

```go
	npcID := s.PopInt()
	if err := checkNpcType(s, npcID, "NC_OP"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(npcID)
```

- [ ] **Step 10: Wire `checkNpcType` at `handleNcSize` (:418)**

Replace lines 422-426:

```go
	id := s.PopInt()
	nt := s.Configs.NpcType(id)
	if nt == nil {
		return fmt.Errorf("NC_SIZE: unknown npc id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_SIZE"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
```

- [ ] **Step 11: Wire `checkNpcType` at `handleNcVisLevel` (:433)**

Replace lines 437-441:

```go
	id := s.PopInt()
	nt := s.Configs.NpcType(id)
	if nt == nil {
		return fmt.Errorf("NC_VISLEVEL: unknown npc id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkNpcType(s, id, "NC_VISLEVEL"); err != nil {
		return err
	}
	nt := s.Configs.NpcType(id)
```

- [ ] **Step 12: Run the assertion to confirm GREEN**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run 'TestHandleNcName' -v
```

Expected: PASS.

- [ ] **Step 13: Run full `pkg/script/` suite to confirm no regressions**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -count=1
```

Expected: PASS.

- [ ] **Step 14: Run gofmt on edited files**

```bash
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/gofmt -l pkg/script/handlers_config.go pkg/script/handlers_config_test.go
```

Expected: no output (clean).

- [ ] **Step 15: Audit-grep NC_* bespoke wording is gone**

```bash
grep -n '"NC_[A-Z]*: unknown npc id\|"NPC_PARAM: unknown npc id' pkg/script/handlers_config.go
```

Expected: 0 hits.

- [ ] **Step 16: Commit**

```bash
git add pkg/script/handlers_config.go pkg/script/handlers_config_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkNpcType at 9 NC_*/NPC_PARAM sites

Replace inline `s.Configs.NpcType(id) == nil` guards at NC_NAME,
NC_PARAM, NPC_PARAM, NC_CATEGORY, NC_DESC, NC_DEBUGNAME, NC_OP,
NC_SIZE, NC_VISLEVEL in handlers_config.go with the existing
checkNpcType validator. Canonicalize error wording to
"OP: no NpcType with value (id) found" (mirrors TS NpcTypeValid
emission from ScriptInputConfigTypeValidator throw shape).

Slice 4 of the 2026-05-21 4-slice bundle (slice 1 LoW arg-shape
pin, slice 2 doc-comment sweep, slice 3 phantom retired). Pure
refactor — same error semantics, new wording.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Wire OC_* Part A (10 sites) + add TestCheckObjType

**Files:**
- Modify: `pkg/script/handlers_config.go` lines 450-601 (10 OC_* handlers)
- Modify: `pkg/script/handlers_config_test.go:884` (1 assertion flip for OC_NAME)
- Modify: `pkg/script/handlers_obj_test.go` (add `TestCheckObjType` near top, sibling to other test helpers)

- [ ] **Step 1: Update test assertion at `handlers_config_test.go:884` to canonical wording**

Replace:

```go
runConfigOpExpectErr(t, mc, OpOcName, []int{999}, "unknown obj id")
```

With:

```go
runConfigOpExpectErr(t, mc, OpOcName, []int{999}, "no ObjType with value (999) found")
```

- [ ] **Step 2: Add `TestCheckObjType` to `handlers_obj_test.go`**

Read `pkg/script/handlers_npc_test.go:55` for the canonical `TestCheckNpcType` table shape. Add `TestCheckObjType` near the top of `handlers_obj_test.go` (after imports, before the first existing test func). Mirror the 4 sub-cases:

```go
// TestCheckObjType validates the state-aware ObjType validator at
// handlers_obj.go:44. Mirrors TestCheckNpcType.
func TestCheckObjType(t *testing.T) {
	t.Run("valid id returns nil", func(t *testing.T) {
		mc := newTestConfigs(t)
		mc.AddObjType(42, &objtype.ObjType{ID: 42})
		s := &ScriptState{Configs: mc}
		if err := checkObjType(s, 42, "TEST"); err != nil {
			t.Fatalf("checkObjType(42) returned err: %v", err)
		}
	})
	t.Run("unknown id returns canonical error", func(t *testing.T) {
		mc := newTestConfigs(t)
		s := &ScriptState{Configs: mc}
		err := checkObjType(s, 999, "TEST")
		if err == nil {
			t.Fatal("want error, got nil")
		}
		want := "TEST: no ObjType with value (999) found"
		if err.Error() != want {
			t.Errorf("err: got %q, want %q", err.Error(), want)
		}
	})
	t.Run("negative id returns canonical error", func(t *testing.T) {
		mc := newTestConfigs(t)
		s := &ScriptState{Configs: mc}
		err := checkObjType(s, -1, "TEST")
		if err == nil {
			t.Fatal("want error, got nil")
		}
		want := "TEST: no ObjType with value (-1) found"
		if err.Error() != want {
			t.Errorf("err: got %q, want %q", err.Error(), want)
		}
	})
	t.Run("nil Configs returns canonical error", func(t *testing.T) {
		s := &ScriptState{Configs: nil}
		err := checkObjType(s, 42, "TEST")
		if err == nil {
			t.Fatal("want error, got nil")
		}
		want := "TEST: no ObjType with value (42) found"
		if err.Error() != want {
			t.Errorf("err: got %q, want %q", err.Error(), want)
		}
	})
}
```

**If `newTestConfigs(t)` / `mockConfigs.AddObjType` API differs from this sketch:** read `handlers_npc_test.go:55-115` (`TestCheckNpcType` body) and match its setup pattern exactly — that test is the canonical reference. If you can't find an `AddObjType` helper on the mock, look for how `TestCheckNpcType` adds an `NpcType` and mirror it. If no such helper exists, omit the "valid id" sub-case and keep the 3 error sub-cases (validator-only-failure tests still cover the error paths).

- [ ] **Step 3: Run `TestCheckObjType` to confirm it PASSES (validator already exists)**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run TestCheckObjType -v
```

Expected: PASS (validator at `handlers_obj.go:44` already emits the canonical wording).

- [ ] **Step 4: Wire `checkObjType` at `handleOcName` (:450)**

Replace lines 454-458:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_NAME: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_NAME"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 5: Wire `checkObjType` at `handleOcParam` (:470)**

Replace lines 474-479:

```go
	paramID := s.PopInt()
	objID := s.PopInt()
	ot := s.Configs.ObjType(objID)
	if ot == nil {
		return fmt.Errorf("OC_PARAM: unknown obj id %d", objID)
	}
```

With:

```go
	paramID := s.PopInt()
	objID := s.PopInt()
	if err := checkObjType(s, objID, "OC_PARAM"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(objID)
```

- [ ] **Step 6: Wire `checkObjType` at `handleOcCategory` (:484)**

Replace lines 488-492:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_CATEGORY: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_CATEGORY"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 7: Wire `checkObjType` at `handleOcDesc` (:498)**

Replace lines 502-506:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_DESC: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_DESC"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 8: Wire `checkObjType` at `handleOcMembers` (:516)**

Replace lines 520-524:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_MEMBERS: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_MEMBERS"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 9: Wire `checkObjType` at `handleOcWeight` (:534)**

Replace lines 538-542:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_WEIGHT: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_WEIGHT"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 10: Wire `checkObjType` at `handleOcWearPos` (:548)**

Replace lines 552-556:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_WEARPOS: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_WEARPOS"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 11: Wire `checkObjType` at `handleOcWearPos2` (:562)**

Replace lines 566-570:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_WEARPOS2: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_WEARPOS2"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 12: Wire `checkObjType` at `handleOcWearPos3` (:576)**

Replace lines 580-584:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_WEARPOS3: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_WEARPOS3"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 13: Wire `checkObjType` at `handleOcCost` (:590)**

Replace lines 594-598:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_COST: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_COST"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 14: Run T2's affected tests to confirm GREEN**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -count=1
```

Expected: PASS.

- [ ] **Step 15: gofmt + audit-grep**

```bash
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/gofmt -l pkg/script/handlers_config.go pkg/script/handlers_config_test.go pkg/script/handlers_obj_test.go
grep -n 'OC_NAME: unknown obj id\|OC_PARAM: unknown obj id\|OC_CATEGORY: unknown obj id\|OC_DESC: unknown obj id\|OC_MEMBERS: unknown obj id\|OC_WEIGHT: unknown obj id\|OC_WEARPOS: unknown obj id\|OC_WEARPOS2: unknown obj id\|OC_WEARPOS3: unknown obj id\|OC_COST: unknown obj id' pkg/script/handlers_config.go
```

Expected: gofmt empty; grep returns 0 hits.

- [ ] **Step 16: Commit**

```bash
git add pkg/script/handlers_config.go pkg/script/handlers_config_test.go pkg/script/handlers_obj_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkObjType at 10 OC_* Part A sites + TestCheckObjType

Replace inline `s.Configs.ObjType(id) == nil` guards at OC_NAME,
OC_PARAM, OC_CATEGORY, OC_DESC, OC_MEMBERS, OC_WEIGHT, OC_WEARPOS,
OC_WEARPOS2, OC_WEARPOS3, OC_COST in handlers_config.go with the
existing checkObjType validator. Canonicalize error wording to
"OP: no ObjType with value (id) found" (mirrors TS ObjTypeValid
emission).

Add TestCheckObjType to handlers_obj_test.go — sibling-location to
checkObjType per file-pairing convention, mirroring TestCheckNpcType
at handlers_npc_test.go:55.

Slice 4 part 2 of the 2026-05-21 4-slice bundle.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Wire OC_* Part B (5 sites)

**Files:**
- Modify: `pkg/script/handlers_config.go` lines 604-699 (5 OC_* handlers: handleOcTradeable/handleOcDebugName/handleOcCert/handleOcUncert/handleOcStackable)

No test changes — none of the 5 affected handlers have a "unknown obj id" assertion in `handlers_config_test.go` per the brainstorm-time grep (only OC_NAME at :884 was assertable).

- [ ] **Step 1: Wire `checkObjType` at `handleOcTradeable` (:604)**

Replace lines 608-612:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_TRADEABLE: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_TRADEABLE"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 2: Wire `checkObjType` at `handleOcDebugName` (:622)**

Replace lines 626-630:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_DEBUGNAME: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_DEBUGNAME"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 3: Wire `checkObjType` at `handleOcCert` (:644)**

Replace lines 648-652:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_CERT: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_CERT"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 4: Wire `checkObjType` at `handleOcUncert` (:666)**

Replace lines 670-674:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_UNCERT: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_UNCERT"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 5: Wire `checkObjType` at `handleOcStackable` (:684)**

Replace lines 688-692:

```go
	id := s.PopInt()
	ot := s.Configs.ObjType(id)
	if ot == nil {
		return fmt.Errorf("OC_STACKABLE: unknown obj id %d", id)
	}
```

With:

```go
	id := s.PopInt()
	if err := checkObjType(s, id, "OC_STACKABLE"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
```

- [ ] **Step 6: Run pkg/script tests**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -count=1
```

Expected: PASS.

- [ ] **Step 7: gofmt + audit-grep**

```bash
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/gofmt -l pkg/script/handlers_config.go
grep -n 'OC_TRADEABLE: unknown obj id\|OC_DEBUGNAME: unknown obj id\|OC_CERT: unknown obj id\|OC_UNCERT: unknown obj id\|OC_STACKABLE: unknown obj id' pkg/script/handlers_config.go
```

Expected: gofmt empty; grep returns 0 hits.

- [ ] **Step 8: Audit `OC_` family is fully canonicalized**

```bash
grep -n 'OC_[A-Z]*: unknown obj id' pkg/script/handlers_config.go
```

Expected: 0 hits across the whole file.

- [ ] **Step 9: Commit**

```bash
git add pkg/script/handlers_config.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkObjType at 5 OC_* Part B sites

Replace inline `s.Configs.ObjType(id) == nil` guards at OC_TRADEABLE,
OC_DEBUGNAME, OC_CERT, OC_UNCERT, OC_STACKABLE in handlers_config.go
with the existing checkObjType validator. Canonicalize error wording.

Slice 4 part 3 of the 2026-05-21 4-slice bundle. Completes the
OC_* family canonicalization (15 sites in T2 + T3).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire OBJ_FIND + close-slice gates

**Files:**
- Modify: `pkg/script/handlers_obj.go:300-328` (handleObjFind)
- Modify: `pkg/script/handlers_obj_test.go` (TestObjFindUnknownObjId at :808, 3 string occurrences at :816, :818, :819)

- [ ] **Step 1: Update test assertions in `TestObjFindUnknownObjId`**

Read `handlers_obj_test.go:808-822` for the existing test body. Update the 3 occurrences of `"unknown obj id"` to `"no ObjType with value"`:

```go
if err == nil {
    t.Fatal("handleObjFind: want error (no ObjType with value), got nil")
}
if !strings.Contains(err.Error(), "no ObjType with value") {
    t.Errorf("err: got %q, want substring %q", err.Error(), "no ObjType with value")
}
```

- [ ] **Step 2: Run the test to confirm RED**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run TestObjFindUnknownObjId -v
```

Expected: FAIL — production still emits `"OBJ_FIND: unknown obj id 999"`.

- [ ] **Step 3: Wire `checkObjType` at `handleObjFind` (:300)**

Replace lines 307-315 in `handlers_obj.go`:

```go
	objId := s.PopInt()
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "OBJ_FIND")
	if err != nil {
		return err
	}
	if s.Configs.ObjType(objId) == nil {
		return fmt.Errorf("OBJ_FIND: unknown obj id %d", objId)
	}
```

With:

```go
	objId := s.PopInt()
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "OBJ_FIND")
	if err != nil {
		return err
	}
	if err := checkObjType(s, objId, "OBJ_FIND"); err != nil {
		return err
	}
```

Note: no local `ot` var was used here (handler never accesses fields of the looked-up `ObjType` — it just gates the World lookup). Do NOT introduce a new `ot := s.Configs.ObjType(objId)` line; the post-wire shape is just the `checkObjType` call.

- [ ] **Step 4: Run the test to confirm GREEN**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run TestObjFindUnknownObjId -v
```

Expected: PASS.

- [ ] **Step 5: Run full pkg/script suite**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -count=1
```

Expected: PASS.

- [ ] **Step 6: Run race-detector suite (close-slice gate)**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test -race ./...
```

Expected: PASS, ~155s wall clock (modules/world is long pole). 0 failures across 57+ pkgs.

- [ ] **Step 7: Run smoke test (close-slice gate)**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./modules/world/ -run TestPackAll_TwelveStageSmoke -v
```

Expected: PASS.

- [ ] **Step 8: gofmt close-slice gate**

```bash
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/gofmt -l pkg/script/handlers_config.go pkg/script/handlers_obj.go pkg/script/handlers_config_test.go pkg/script/handlers_obj_test.go
```

Expected: no output (clean).

- [ ] **Step 9: Final audit-greps (close-slice gates)**

```bash
# Bespoke wording removed from in-scope sites:
grep -n 'NC_[A-Z]*: unknown npc id\|OC_[A-Z]*: unknown obj id\|NPC_PARAM: unknown npc id\|OBJ_FIND: unknown obj id' pkg/script/handlers_config.go pkg/script/handlers_obj.go

# Canonical wording present:
grep -cn 'checkNpcType\|checkObjType' pkg/script/handlers_config.go pkg/script/handlers_obj.go

# Non-candidate sites still use bespoke wording (intentional):
grep -n '"OBJ_NAME:\|"OBJ_PARAM:' pkg/script/handlers_obj.go
```

Expected:
- First grep: 0 hits.
- Second grep: ≥25 hits in handlers_config.go + 1 in handlers_obj.go (the OBJ_FIND wire) + 1 in handlers_obj.go (checkObjType definition itself at :44). Plus existing checkObjType uses at :82, :250 (OBJ_TAKEITEM wire).
- Third grep: hits at OBJ_NAME and OBJ_PARAM (non-candidate stored-entity-state reads, retain their `"OBJ_NAME: unknown obj id"` wording).

- [ ] **Step 10: Commit**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers_obj_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkObjType at OBJ_FIND (handlers_obj.go:313)

Replace inline `s.Configs.ObjType(objId) == nil` guard at OBJ_FIND
with the existing checkObjType validator. Canonicalize error wording.
No local `ot` var introduced — OBJ_FIND never accesses type fields,
just gates the World.GetObj lookup.

Closes slice 4 of the 2026-05-21 4-slice bundle (25 wires total:
9 NC_* + 15 OC_* + 1 OBJ_FIND).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist (run before subagent dispatch)

- [ ] Spec §5 (site enumeration) covers every step in T1-T4: 9 + 10 + 5 + 1 = 25 wires. ✓
- [ ] Every step shows the exact pre/post code block. ✓
- [ ] All file paths are absolute-relative to repo root, no `.../` or placeholders. ✓
- [ ] `checkNpcType` and `checkObjType` referenced consistently (no `checkNpcType*` / `CheckObjType` casing drift). ✓
- [ ] All commit messages use `--no-gpg-sign` per CLAUDE.md global. ✓
- [ ] All `go` invocations use the `GOROOT=/home/owner/go/go1.26.3 ...` env per CLAUDE.md global. ✓
- [ ] No new `NAI-XXX-D-*` pins opened (pure refactor). ✓
- [ ] T4 race-detector + smoke test gates run at slice close. ✓
