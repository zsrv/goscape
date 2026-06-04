# LocType/NpcType/ObjType read-side validator wiring (Shape A) — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `checkNpcType` / `checkObjType` at 7 sites across 3 files — 3 in `handlers_npc.go` (NPC_TYPE, NPC_CHANGETYPE, NPC_CHANGETYPE_KEEPALL), 2 in `handlers_obj.go` (OBJ_NAME, OBJ_PARAM), 2 in `handlers_interface.go` (IF_SETNPCHEAD, IF_SETOBJECT). Bring all script-input Npc/Obj type ids reaching these opcodes under the canonical `"%s: no XType with value (%d) found"` wording, matching TS `check(id, NpcTypeValid)` / `check(id, ObjTypeValid)`.

**Architecture:** For sites that currently skip validation entirely (NPC_TYPE / NPC_CHANGETYPE / NPC_CHANGETYPE_KEEPALL / IF_SETNPCHEAD / IF_SETOBJECT), insert `requireConfigs` + `checkXType` BEFORE the action call. For sites that currently use bespoke `"unknown obj id"` wording (OBJ_NAME / OBJ_PARAM), replace the bespoke nil-check block with `checkObjType` + a preserved local-var lookup. Two test assertions in `handlers_obj_test.go` flip from `"unknown obj id"` to `"no ObjType with value"`.

**Tech Stack:** Go 1.26.x; modifies `pkg/script/handlers_npc.go`, `pkg/script/handlers_obj.go`, `pkg/script/handlers_interface.go`, `pkg/script/handlers_obj_test.go`. Existing validators at `handlers_npc.go:88-93` (`checkNpcType`) and `handlers_obj.go:44-49` (`checkObjType`). No new helpers.

**Spec:** `docs/superpowers/specs/2026-05-21-loc-npc-obj-readside-validator-wiring-design.md` (HEAD `39753722`).

---

## Task 1: Wire all 7 sites + flip 2 test assertions + verify gates

**Files:**
- Modify: `pkg/script/handlers_npc.go` (3 handlers: NPC_TYPE, NPC_CHANGETYPE, NPC_CHANGETYPE_KEEPALL)
- Modify: `pkg/script/handlers_obj.go` (2 handlers: OBJ_NAME, OBJ_PARAM)
- Modify: `pkg/script/handlers_interface.go` (2 handlers: IF_SETNPCHEAD, IF_SETOBJECT)
- Modify: `pkg/script/handlers_obj_test.go` (2 assertion flips: `TestObjNameUnknownType`, `TestObjParamUnknownType`)

### Step 1: Pre-impl audit-grep baseline (record exact HEAD counts)

- [ ] **Step 1.1: Record baseline counts**

Run from repo root:

```bash
grep -c "checkNpcType(s, " pkg/script/handlers_npc.go pkg/script/handlers_interface.go
grep -c "checkObjType(s, " pkg/script/handlers_obj.go pkg/script/handlers_interface.go
grep -cE 'unknown obj id|unknown npc id' pkg/script/handlers_npc.go pkg/script/handlers_obj.go pkg/script/handlers_interface.go
grep -nE 'no checkNotNull here \(NAI-23 Bundle 4c\)' pkg/script/handlers_interface.go
```

Expected at HEAD `39753722`:
- `checkNpcType(s, ` → `pkg/script/handlers_npc.go:5`, `pkg/script/handlers_interface.go:0`
- `checkObjType(s, ` → `pkg/script/handlers_obj.go:2`, `pkg/script/handlers_interface.go:0`
- bespoke wordings → `pkg/script/handlers_npc.go:0`, `pkg/script/handlers_obj.go:2`, `pkg/script/handlers_interface.go:0`
- acknowledged-gap comments → 2 hits (`handlers_interface.go:198`, `handlers_interface.go:289`)

If any baseline diverges, STOP and report — HEAD may have drifted. Do NOT proceed.

### Step 2: Wire `handlers_npc.go` (3 sites)

For all 3 sites: insert `requireConfigs(s, op)` BEFORE the existing `requireActiveNpc(s, op)` call (to fail fast on nil-Configs with the standard `"%s: no configs"` wording), then call `checkNpcType` immediately after the relevant id-read. No defensive comment needed — these handlers do not preserve a downstream Configs lookup.

- [ ] **Step 2.1: Wire `handleNpcType` (NPC_TYPE) at `handlers_npc.go:180-187`**

Existing:

```go
// handleNpcType pushes the ActiveNpc's NpcType id.
func handleNpcType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_TYPE"); err != nil {
		return err
	}
	s.PushInt(s.ActiveNpc.NpcType())
	return nil
}
```

Replace with:

```go
// handleNpcType pushes the ActiveNpc's NpcType id. Mirrors TS
// NpcOps.ts:259-261: pushInt(check(activeNpc.type, NpcTypeValid).id).
func handleNpcType(s *ScriptState) error {
	if err := requireConfigs(s, "NPC_TYPE"); err != nil {
		return err
	}
	if err := requireActiveNpc(s, "NPC_TYPE"); err != nil {
		return err
	}
	id := s.ActiveNpc.NpcType()
	if err := checkNpcType(s, id, "NPC_TYPE"); err != nil {
		return err
	}
	s.PushInt(id)
	return nil
}
```

Doc-comment updated to cite TS source (matches `handleNpcCategory`/`handleNpcName` doc-comment style at the same file).

- [ ] **Step 2.2: Wire `handleNpcChangeType` (NPC_CHANGETYPE) at `handlers_npc.go:354-366`**

Existing:

```go
// handleNpcChangeType pops (newType, duration) in TS order (duration
// on top) and morphs the NPC. Matches TS NpcOps.ts:457-462.
// The full body (guard + typeId/uid/mask + stats-reset +
// lifecycleTick fast-path) lives in *Npc.changeTypeImpl.
func handleNpcChangeType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	duration := s.PopInt()
	newType := s.PopInt()
	s.ActiveNpc.ChangeType(newType, duration)
	return nil
}
```

Replace with:

```go
// handleNpcChangeType pops (newType, duration) in TS order (duration
// on top) and morphs the NPC. Matches TS NpcOps.ts:457-462 including
// the check(id, NpcTypeValid) registry-presence gate at :459.
// The full body (guard + typeId/uid/mask + stats-reset +
// lifecycleTick fast-path) lives in *Npc.changeTypeImpl.
func handleNpcChangeType(s *ScriptState) error {
	if err := requireConfigs(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	if err := requireActiveNpc(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	duration := s.PopInt()
	newType := s.PopInt()
	if err := checkNpcType(s, newType, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	s.ActiveNpc.ChangeType(newType, duration)
	return nil
}
```

Doc-comment extended to cite the TS validator gate.

- [ ] **Step 2.3: Wire `handleNpcChangeTypeKeepAll` (NPC_CHANGETYPE_KEEPALL) at `handlers_npc.go:368-379`**

Existing:

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

Replace with:

```go
// handleNpcChangeTypeKeepAll pops (newType, duration) in TS order
// (duration on top) and morphs the NPC preserving all current stats.
// Matches TS NpcOps.ts:465-471 including the check(id, NpcTypeValid)
// registry-presence gate at :467.
func handleNpcChangeTypeKeepAll(s *ScriptState) error {
	if err := requireConfigs(s, "NPC_CHANGETYPE_KEEPALL"); err != nil {
		return err
	}
	if err := requireActiveNpc(s, "NPC_CHANGETYPE_KEEPALL"); err != nil {
		return err
	}
	duration := s.PopInt()
	newType := s.PopInt()
	if err := checkNpcType(s, newType, "NPC_CHANGETYPE_KEEPALL"); err != nil {
		return err
	}
	s.ActiveNpc.ChangeTypeKeepAll(newType, duration)
	return nil
}
```

### Step 3: Wire `handlers_obj.go` (2 sites) — swap bespoke nil-check for `checkObjType`, preserve local var

For both OBJ_NAME / OBJ_PARAM: delete the bespoke `if ot == nil { return fmt.Errorf("…unknown obj id…") }` block and replace with a `checkObjType` call BEFORE the `s.Configs.ObjType(...)` lookup. The local var `ot` is preserved verbatim for downstream field access. Matches `handleOcName` precedent at `handlers_config.go:450-466`.

- [ ] **Step 3.1: Wire `handleObjName` (OBJ_NAME) at `handlers_obj.go:379-398`**

Existing:

```go
func handleObjName(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_NAME"); err != nil {
		return err
	}
	if err := requireConfigs(s, "OBJ_NAME"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(s.ActiveObj.ObjType())
	if ot == nil {
		return fmt.Errorf("OBJ_NAME: unknown obj id %d", s.ActiveObj.ObjType())
	}
	if ot.Name != "" {
		s.PushString(ot.Name)
	} else if ot.DebugName != "" {
		s.PushString(ot.DebugName)
	} else {
		s.PushString("null")
	}
	return nil
}
```

Replace with:

```go
func handleObjName(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_NAME"); err != nil {
		return err
	}
	if err := requireConfigs(s, "OBJ_NAME"); err != nil {
		return err
	}
	id := s.ActiveObj.ObjType()
	if err := checkObjType(s, id, "OBJ_NAME"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	if ot.Name != "" {
		s.PushString(ot.Name)
	} else if ot.DebugName != "" {
		s.PushString(ot.DebugName)
	} else {
		s.PushString("null")
	}
	return nil
}
```

Note: introduce `id := s.ActiveObj.ObjType()` local var (replacing the inline duplication in the original) to avoid calling `ObjType()` thrice. Doc-comment at `:375-378` does NOT need editing (already cites TS source and the existing `handleOcName` sibling).

- [ ] **Step 3.2: Wire `handleObjParam` (OBJ_PARAM) at `handlers_obj.go:404-417`**

Existing:

```go
func handleObjParam(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_PARAM"); err != nil {
		return err
	}
	if err := requireConfigs(s, "OBJ_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	ot := s.Configs.ObjType(s.ActiveObj.ObjType())
	if ot == nil {
		return fmt.Errorf("OBJ_PARAM: unknown obj id %d", s.ActiveObj.ObjType())
	}
	return paramLookup(s, ot.Params, paramID, "OBJ_PARAM")
}
```

Replace with:

```go
func handleObjParam(s *ScriptState) error {
	if err := requireActiveObj(s, "OBJ_PARAM"); err != nil {
		return err
	}
	if err := requireConfigs(s, "OBJ_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	id := s.ActiveObj.ObjType()
	if err := checkObjType(s, id, "OBJ_PARAM"); err != nil {
		return err
	}
	ot := s.Configs.ObjType(id)
	return paramLookup(s, ot.Params, paramID, "OBJ_PARAM")
}
```

Same `id` local-var pattern as Step 3.1. Doc-comment at `:400-403` unchanged.

### Step 4: Wire `handlers_interface.go` (2 sites) — add `requireConfigs` + `checkXType`, delete acknowledged-gap comment

For both IF_SETNPCHEAD / IF_SETOBJECT: insert `requireConfigs(s, op)` right after the active-player gate, insert `checkXType` after the existing `checkNotNull(com)`, and DELETE the now-stale acknowledged-gap comment ("no checkNotNull here (NAI-23 Bundle 4c)").

- [ ] **Step 4.1: Wire `handleIfSetNpcHead` (IF_SETNPCHEAD) at `handlers_interface.go:186-201`**

Existing:

```go
// handleIfSetNpcHead implements IF_SETNPCHEAD.
// TS PlayerOps.ts:742-749 — popInts(2) → [com, npc], npc on top.
// com wrapped with check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetNpcHead(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETNPCHEAD: no active player")
	}
	npc := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETNPCHEAD"); err != nil {
		return err
	}
	// npc uses NpcTypeValid in TS (not NumberNotNull); no checkNotNull here (NAI-23 Bundle 4c).
	s.Self.IfSetNpcHead(com, npc)
	return nil
}
```

Replace with:

```go
// handleIfSetNpcHead implements IF_SETNPCHEAD.
// TS PlayerOps.ts:742-749 — popInts(2) → [com, npc], npc on top.
// com wrapped with check(com, NumberNotNull); npc wrapped with
// check(npc, NpcTypeValid) (NAI-23 Bundle 4c).
func handleIfSetNpcHead(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETNPCHEAD: no active player")
	}
	if err := requireConfigs(s, "IF_SETNPCHEAD"); err != nil {
		return err
	}
	npc := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETNPCHEAD"); err != nil {
		return err
	}
	if err := checkNpcType(s, npc, "IF_SETNPCHEAD"); err != nil {
		return err
	}
	s.Self.IfSetNpcHead(com, npc)
	return nil
}
```

The inline acknowledged-gap comment is deleted. The doc-comment is updated to reflect that NpcTypeValid is now wired.

- [ ] **Step 4.2: Wire `handleIfSetObject` (IF_SETOBJECT) at `handlers_interface.go:276-295`**

Existing:

```go
// handleIfSetObject implements IF_SETOBJECT.
// TS PlayerOps.ts:663-671 — popInts(3) → [com, obj, scale], scale on top.
// com and scale wrapped with check(_, NumberNotNull); obj uses ObjTypeValid (NAI-23 Bundle 4c).
func handleIfSetObject(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETOBJECT: no active player")
	}
	scale := s.PopInt()
	obj := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETOBJECT"); err != nil {
		return err
	}
	// obj uses ObjTypeValid in TS (not NumberNotNull); no checkNotNull here (NAI-23 Bundle 4c).
	if err := checkNotNull(scale, "IF_SETOBJECT"); err != nil {
		return err
	}
	s.Self.IfSetObject(com, obj, scale)
	return nil
}
```

Replace with:

```go
// handleIfSetObject implements IF_SETOBJECT.
// TS PlayerOps.ts:663-671 — popInts(3) → [com, obj, scale], scale on top.
// com and scale wrapped with check(_, NumberNotNull); obj wrapped with
// check(obj, ObjTypeValid) (NAI-23 Bundle 4c).
func handleIfSetObject(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETOBJECT: no active player")
	}
	if err := requireConfigs(s, "IF_SETOBJECT"); err != nil {
		return err
	}
	scale := s.PopInt()
	obj := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETOBJECT"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "IF_SETOBJECT"); err != nil {
		return err
	}
	if err := checkNotNull(scale, "IF_SETOBJECT"); err != nil {
		return err
	}
	s.Self.IfSetObject(com, obj, scale)
	return nil
}
```

The `checkObjType` insertion goes between `checkNotNull(com)` and `checkNotNull(scale)` — matching the TS argument order from PlayerOps.ts:665-668. The inline acknowledged-gap comment is deleted. Doc-comment updated.

### Step 5: Flip 2 test assertions

- [ ] **Step 5.1: Flip `TestObjNameUnknownType` at `handlers_obj_test.go:1190-1201`**

Existing assertion (lines 1198-1200):

```go
	if !strings.Contains(err.Error(), "unknown obj id") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "unknown obj id")
	}
```

Replace with:

```go
	if !strings.Contains(err.Error(), "no ObjType with value") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "no ObjType with value")
	}
```

The `OBJ_NAME` substring assertion at `:1184-1186` is preserved unchanged.

- [ ] **Step 5.2: Flip `TestObjParamUnknownType` at `handlers_obj_test.go:1330-1343`**

Existing assertion (lines 1340-1342):

```go
	if !strings.Contains(err.Error(), "unknown obj id") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "unknown obj id")
	}
```

Replace with:

```go
	if !strings.Contains(err.Error(), "no ObjType with value") {
		t.Errorf("err: got %q, want substring %q", err.Error(), "no ObjType with value")
	}
```

The `OBJ_PARAM` substring assertion at `:1325-1327` is preserved unchanged.

### Step 6: Gates

- [ ] **Step 6.1: `gofmt -l` clean**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/gofmt -l pkg/script/handlers_npc.go pkg/script/handlers_obj.go pkg/script/handlers_interface.go pkg/script/handlers_obj_test.go
```

Expected: no output (all 4 files gofmt-clean).

- [ ] **Step 6.2: `go test -race ./...` 0 FAIL**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test -race ./...
```

Expected: all packages PASS. Wall clock ~150-160s (modules/world is the long pole). If any FAIL, halt and diagnose.

- [ ] **Step 6.3: `TestPackAll_TwelveStageSmoke` PASS**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/packall -run TestPackAll_TwelveStageSmoke -count=1
```

Expected: PASS.

- [ ] **Step 6.4: Audit-grep post-impl counts**

```bash
grep -c "checkNpcType(s, " pkg/script/handlers_npc.go pkg/script/handlers_interface.go
grep -c "checkObjType(s, " pkg/script/handlers_obj.go pkg/script/handlers_interface.go
grep -cE 'unknown obj id|unknown npc id' pkg/script/handlers_npc.go pkg/script/handlers_obj.go pkg/script/handlers_interface.go
grep -cn 'no checkNotNull here (NAI-23 Bundle 4c)' pkg/script/handlers_interface.go
```

Expected post-impl:
- `checkNpcType(s, ` → `pkg/script/handlers_npc.go:8` (+3 vs baseline 5), `pkg/script/handlers_interface.go:1` (+1)
- `checkObjType(s, ` → `pkg/script/handlers_obj.go:4` (+2 vs baseline 2), `pkg/script/handlers_interface.go:1` (+1)
- bespoke wordings → all 0 (−2 from handlers_obj.go)
- acknowledged-gap comments → 0 (−2; deleted from both IF_SET* handlers)

If any post-impl count differs from expected, STOP — wiring incomplete or unintended changes occurred. Report each divergence.

- [ ] **Step 6.5: Targeted test PASS**

```bash
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./pkg/script/ -run "TestObjName|TestObjParam|TestNpcType|TestNpcChange|TestIfSetNpcHead|TestIfSetObject" -count=1 -v 2>&1 | tail -40
```

Expected: all matched tests PASS, including the 2 flipped assertions.

### Step 7: Commit

- [ ] **Step 7.1: Stage and commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_obj.go pkg/script/handlers_interface.go pkg/script/handlers_obj_test.go
```

Then commit:

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): wire checkNpcType/checkObjType at 7 read-side sites

Layer the canonical type-registry validators at 7 script-input call
sites outside handlers_inv.go / handlers_config.go, mirroring the
sibling precedents from registry-presence-validators-wiring-close
and handlers-inv-readside-checkinvtype-wiring-close.

handlers_npc.go (3 sites): NPC_TYPE, NPC_CHANGETYPE,
NPC_CHANGETYPE_KEEPALL — none had any prior validator; added
requireConfigs + checkNpcType before the action call.

handlers_obj.go (2 sites): OBJ_NAME, OBJ_PARAM — swap bespoke
"unknown obj id %d" nil-check for checkObjType + preserved `ot`
local var. Mirrors handleOcName precedent at
handlers_config.go:450-466.

handlers_interface.go (2 sites): IF_SETNPCHEAD, IF_SETOBJECT —
add requireConfigs + checkNpcType/checkObjType, delete the
acknowledged-gap "no checkNotNull here (NAI-23 Bundle 4c)" inline
comments at :198 and :289 (the gap they describe is now closed).

TS-faithful per:
- NpcOps.ts:260 (NPC_TYPE), :459 (NPC_CHANGETYPE),
  :467 (NPC_CHANGETYPE_KEEPALL)
- ObjOps.ts:98 (OBJ_PARAM), :107 (OBJ_NAME)
- PlayerOps.ts:746 (IF_SETNPCHEAD), :667 (IF_SETOBJECT)

2 test assertions flipped in handlers_obj_test.go:
- TestObjNameUnknownType: "unknown obj id" → "no ObjType with value"
- TestObjParamUnknownType: "unknown obj id" → "no ObjType with value"

Zero new tests — validator-layer TestCheckNpcType / TestCheckObjType
already cover registry-miss rejection per the registry-presence-
validators-wiring-close precedent.

Audit-grep delta vs HEAD 39753722:
- checkNpcType(s,  in handlers_npc.go      → 5 → 8  (+3)
- checkNpcType(s,  in handlers_interface.go → 0 → 1 (+1)
- checkObjType(s,  in handlers_obj.go      → 2 → 4  (+2)
- checkObjType(s,  in handlers_interface.go → 0 → 1 (+1)
- "unknown obj id" in handlers_obj.go       → 2 → 0 (−2)
- "no checkNotNull here (NAI-23 Bundle 4c)" → 2 → 0 (−2)

Shape B subset (NPC_NAME silent "null" / NPC_CATEGORY silent -1)
deferred to a separate brainstorm-shaped slice per spec §5.1.

Spec: docs/superpowers/specs/2026-05-21-loc-npc-obj-readside-validator-wiring-design.md
Plan: docs/superpowers/plans/2026-05-21-loc-npc-obj-readside-validator-wiring-plan.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7.2: Verify commit**

```bash
git log -1 --stat
git status
```

Expected:
- One new commit at HEAD touching the 4 files above.
- `git status` shows only standing untracked noise + `config.yaml` drift — no other modified files.

---

## Self-Review Notes

- **Spec coverage:** every section of the spec maps to steps above. §4.1 → Steps 2.1-2.3. §4.2 → Steps 3.1-3.2. §4.3 → Steps 4.1-4.2. §5 (out of scope) → no steps. §6.1 (assertion flips) → Steps 5.1-5.2. §6.3 (impl-time audit-grep) → Steps 1.1 + 6.4. §8 (gates) → Steps 6.1-6.5. §9 (cadence) → single task per spec direction.
- **Placeholder scan:** no TBD/TODO; every step has concrete code OR concrete command + expected output.
- **Type/signature consistency:** `checkNpcType(s *ScriptState, id int, op string) error` / `checkObjType(s *ScriptState, id int, op string) error` used uniformly. `id := s.ActiveObj.ObjType()` local var introduced in OBJ_NAME / OBJ_PARAM replaces inline triple-call shape.
- **Naming consistency:** opcode literals (`"NPC_TYPE"`, `"OBJ_NAME"`, `"IF_SETNPCHEAD"`, etc.) match the existing handler doc-comment opcode labels.
- **Doc-comment updates:** NPC_TYPE / NPC_CHANGETYPE / NPC_CHANGETYPE_KEEPALL doc-comments extended to cite the TS validator gate. IF_SETNPCHEAD / IF_SETOBJECT doc-comments updated to reflect that the TS NpcTypeValid / ObjTypeValid gate is now wired. OBJ_NAME / OBJ_PARAM doc-comments unchanged (already cited TS source).
- **Deletion safety:** the bespoke `if ot == nil { return ... "unknown obj id" }` blocks in OBJ_NAME / OBJ_PARAM are safe to delete because `checkObjType` covers the same nil-registry case. The acknowledged-gap inline comments in IF_SETNPCHEAD / IF_SETOBJECT are safe to delete because the gap they describe is closed by this slice.
