# NAI-152 B1 Implementation Plan — ObjType loader fix + probe retirement

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the static-obj pickup short-circuit by porting TS `ObjType.ts` class-field defaults (`op = [null, null, 'Take', null, null]` / `iop = [null, null, null, null, 'Drop']`) and the F2P / dummyitem post-decode fixups goscape was missing. Retire the B1.5 NodeDebug probe in the same close commit.

**Architecture:** Two production-side changes in `pkg/objtype/objtype.go` (defaults + post-decode fixups) plus a small refactor (extract `applyPostDecodeFixups` helper for testability). One handler-side regression test in `modules/world/handler_opobj_test.go`. Probe retirement strips 8 gateway log sites and 3 probe-only test cases. TDD throughout: red test → run fail → impl → run pass → commit per task.

**Tech Stack:** Go 1.26+; `pkg/objtype` ConfigTypeDecoder pattern (existing); `pkg/io/packet` builder API for cache-fixture construction; `t.Setenv` for `NODE_MEMBERS=false` test isolation.

**Spec:** `docs/superpowers/specs/2026-05-10-nai-152-static-obj-pickup-respawn-design.md` §6.2 (B1 reframed loader fix), §9 (D1 reversed; D1a/b/c added).

**Memory pins applied:**
- `plan_runnable_test_fixtures.md`: every test code block below is mentally executed against the existing helpers.
- `plan_enumerate_struct_literals.md`: production has only ONE `&ObjType{` literal (`NewObjType` itself); test fixtures use struct literals that bypass `NewObjType` and are unaffected by default-init.
- `plan_helper_coverage.md`: `len(.*Op|IOp)` consumers grep'd: `pkg/script/handlers_config.go:388` is `nt.Op` (NpcType, unaffected); `modules/world/handler_opheld.go:80` reads `objType.IOp[op-1]` and benefits transparently from new `IOp[4]=="Drop"` default.
- `close_commit_memory_trailer.md`: T7 close commit carries `Closes memory:` trailer.
- `retire_deviation_grep_all_comments.md`: T7 enumerates probe sites via `rg -n "nai152.opobj.gate"` returning empty.
- `true_to_ts_gate.md`: each TS-source delta has a §9 deviation entry already in the spec.

**Run-everything commands (use throughout):**
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

**Commit policy:** all commits use `git commit --no-gpg-sign` (per global CLAUDE.md). Use HEREDOC bodies; no `Co-Authored-By` line (per project convention — commits in this repo don't carry it).

---

## File map

- **Modify:** `pkg/objtype/objtype.go`
  - `NewObjType` (currently L282-310): add `Op` and `IOp` default initializers.
  - `Decode` codes 30-34 / 35-39 (currently L213-225): drop dead `if ot.Op == nil { ... }` and `if ot.IOp == nil { ... }` allocations.
  - `parseObjTypes` (currently L67-85): extract post-decode loop body into `applyPostDecodeFixups`; add `DummyItem != 0 → Tradeable=false` gate; replace F2P `nil`-ing with reset-to-defaults; add F2P `Category = -1`.
- **Modify:** `pkg/objtype/objtype_test.go`
  - Add `TestNewObjTypeOpDefaults`, `TestObjTypeDecodeSilentCachePreservesDefaults`, `TestObjTypeDecodeCode32OverridesDefault`, `TestApplyPostDecodeFixupsF2PMembersResetsOpToTakeOnly`, `TestApplyPostDecodeFixupsF2PMembersZeroesCategoryAndDropDefault`, `TestApplyPostDecodeFixupsDummyItemForcesTradeableFalse`.
- **Modify:** `modules/world/handler_opobj.go`
  - Strip 8 `s.log.Info("nai152.opobj.gate", ...)` gateway sites and any probe-only locals (`opVal`, `opLen` capture vars).
- **Modify:** `modules/world/handler_opobj_test.go`
  - Add `TestHandleOpObjReachesInteractionWithDefaultOpType` (handler regression).
  - Remove 3 probe-only tests: `TestHandleOpObj_GatewayLogsOnSlotEmpty` (~L766), `TestHandleOpObj_GatewayLogsOnSuccess` (~L795), `TestHandleOpObj_GatewaySilentWhenNodeDebugFalse` (~L820).
  - Drop `bytes` and `log/slog` imports if no other test in the file uses them after removal.

---

### Task 1: Add `Op`/`IOp` defaults to `NewObjType`

**Files:**
- Modify: `pkg/objtype/objtype.go:282-310` (`NewObjType`)
- Test: `pkg/objtype/objtype_test.go` (append)

**TS reference:** `Engine-TS/src/cache/config/ObjType.ts:151-152`:
```ts
op:  (string | null)[] = [null, null, 'Take', null, null];
iop: (string | null)[] = [null, null, null, null, 'Drop'];
```

- [ ] **Step 1: Write the failing test**

Append to `pkg/objtype/objtype_test.go`:

```go
func TestNewObjTypeOpDefaults(t *testing.T) {
	ot := NewObjType(0)

	if got, want := len(ot.Op), 5; got != want {
		t.Fatalf("len(Op): got %d, want %d", got, want)
	}
	wantOp := []string{"", "", "Take", "", ""}
	for i, w := range wantOp {
		if got := ot.Op[i]; got != w {
			t.Errorf("Op[%d]: got %q, want %q", i, got, w)
		}
	}

	if got, want := len(ot.IOp), 5; got != want {
		t.Fatalf("len(IOp): got %d, want %d", got, want)
	}
	wantIOp := []string{"", "", "", "", "Drop"}
	for i, w := range wantIOp {
		if got := ot.IOp[i]; got != w {
			t.Errorf("IOp[%d]: got %q, want %q", i, got, w)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestNewObjTypeOpDefaults -v
```
Expected: FAIL — `len(Op): got 0, want 5`.

- [ ] **Step 3: Implement defaults**

In `pkg/objtype/objtype.go` `NewObjType`, locate the struct literal returned (currently L283-309). Add `Op` and `IOp` initializers in the literal — place them adjacent to the other slice/array defaults for readability (e.g., right after `RespawnRate: 100,` and before `Params:`).

Final shape of `NewObjType` should be:
```go
func NewObjType(id int) *ObjType {
	return &ObjType{
		ConfigType: ConfigType{
			ID: id,
		},
		Zoom2D:       2000,
		Code10:       -1,
		Cost:         1,
		ManWear:      -1,
		ManWear2:     -1,
		WomanWear:    -1,
		WomanWear2:   -1,
		ManWear3:     -1,
		WomanWear3:   -1,
		ManHead:      -1,
		ManHead2:     -1,
		WomanHead:    -1,
		WomanHead2:   -1,
		CertLink:     -1,
		CertTemplate: -1,

		WearPos:     -1,
		WearPos2:    -1,
		WearPos3:    -1,
		Category:    -1,
		RespawnRate: 100, // defaults to 1 minute
		Op:          []string{"", "", "Take", "", ""},
		IOp:         []string{"", "", "", "", "Drop"},
		Params:      make(ParamMap),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestNewObjTypeOpDefaults -v
```
Expected: PASS.

- [ ] **Step 5: Run full pkg/objtype tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```
Expected: PASS — including the existing `TestObjTypeDecodeOpHiddenCoercedToEmpty`. Note: that test calls `NewObjType(0)` then sets `Op[0]="visible"`, `Op[1]=""` via `Decode` codes 30 and 31; after this change `Op[2]` will be `"Take"` not `""`, but the test only checks `Op[0]` and `Op[1]`, so it stays GREEN.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/objtype.go pkg/objtype/objtype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-152 T1 — NewObjType Op/IOp defaults

Mirrors TS ObjType.ts:151-152 class-field defaults
op=[null,null,'Take',null,null] and iop=[null,null,null,null,'Drop'].
Go's "" sentinel maps to TS's null per existing "hidden"->"" collapse
at objtype.go:218-220.

Pre-fix: any obj whose obj.dat cache leaves codes 30-34 silent had
Op=nil, len(Op)=0; the (TS-faithful) handler gate at handler_opobj.go
short-circuited every pickup attempt. Post-fix: Op[2]="Take" by
default matches TS behavior.

Closes NAI-152-D1.
EOF
)"
```

---

### Task 2: Drop dead `Op`/`IOp` nil-allocations in `Decode` + lock with regression tests

**Files:**
- Modify: `pkg/objtype/objtype.go:213-225` (`Decode` cases 30-34 and 35-39)
- Test: `pkg/objtype/objtype_test.go` (append)

After T1, the `if ot.Op == nil { ot.Op = make([]string, 5) }` and `if ot.IOp == nil { ... }` lines in `Decode` are dead code: `NewObjType` always initializes both slices.

- [ ] **Step 1: Write regression tests for default-preservation and cache-override (still GREEN; locks contract)**

Append to `pkg/objtype/objtype_test.go`:

```go
func TestObjTypeDecodeSilentCachePreservesDefaults(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(0) // terminator only — no codes

	ot := NewObjType(0)
	if err := DecodeType(pkt, ot); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}

	if got := ot.Op[2]; got != "Take" {
		t.Errorf("Op[2] (silent cache): got %q, want \"Take\"", got)
	}
	if got := ot.IOp[4]; got != "Drop" {
		t.Errorf("IOp[4] (silent cache): got %q, want \"Drop\"", got)
	}
}

func TestObjTypeDecodeCode32OverridesDefault(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(32)
	pkt.PJStrLF("Whatever")
	pkt.P1(0)

	ot := NewObjType(0)
	if err := DecodeType(pkt, ot); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}

	if got := ot.Op[2]; got != "Whatever" {
		t.Errorf("Op[2] (code 32 override): got %q, want \"Whatever\"", got)
	}
	// Non-overridden slots still default.
	if got := ot.IOp[4]; got != "Drop" {
		t.Errorf("IOp[4]: got %q, want \"Drop\"", got)
	}
}
```

- [ ] **Step 2: Run new tests, verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run "TestObjTypeDecodeSilentCachePreservesDefaults|TestObjTypeDecodeCode32OverridesDefault" -v
```
Expected: PASS (both). They lock the contract before the dead-code drop.

- [ ] **Step 3: Drop dead nil-allocations in `Decode`**

In `pkg/objtype/objtype.go` `Decode`, locate cases 30-34 and 35-39 (currently L213-225). Replace:

```go
case 30, 31, 32, 33, 34:
	if ot.Op == nil {
		ot.Op = make([]string, 5)
	}
	ot.Op[code-30] = dat.GJStrLF()
	if ot.Op[code-30] == "hidden" {
		ot.Op[code-30] = ""
	}
case 35, 36, 37, 38, 39:
	if ot.IOp == nil {
		ot.IOp = make([]string, 5)
	}
	ot.IOp[code-35] = dat.GJStrLF()
```

with:

```go
case 30, 31, 32, 33, 34:
	ot.Op[code-30] = dat.GJStrLF()
	if ot.Op[code-30] == "hidden" {
		ot.Op[code-30] = ""
	}
case 35, 36, 37, 38, 39:
	ot.IOp[code-35] = dat.GJStrLF()
```

- [ ] **Step 4: Run full pkg/objtype tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```
Expected: PASS — including the new regression tests and the existing `TestObjTypeDecodeOpHiddenCoercedToEmpty`.

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/objtype.go pkg/objtype/objtype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(objtype): NAI-152 T2 — drop dead Op/IOp nil-allocations in Decode

NewObjType (T1) now guarantees Op/IOp are length-5 string slices, so
the per-code nil-check + make() in Decode cases 30-34 / 35-39 is dead.
Drop both branches; keep the "hidden"->"" collapse intact.

Adds two regression tests pinning silent-cache default-preservation
and code-32 override semantics — locks the new contract against future
loader changes.
EOF
)"
```

---

### Task 3: Extract `applyPostDecodeFixups` helper from `parseObjTypes` (refactor; behavior unchanged)

**Files:**
- Modify: `pkg/objtype/objtype.go:67-88` (`parseObjTypes` second loop)

The post-decode loop in `parseObjTypes` currently does the certtemplate flatten + F2P fixup. T4/T5 add tests for that loop's behavior (F2P branch + dummyitem branch); those tests need a directly-callable target. Extract the loop body into a helper. **No behavior change.**

- [ ] **Step 1: Run full pkg/objtype tests baseline**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```
Expected: PASS. Records baseline; the refactor must not change this.

- [ ] **Step 2: Extract helper**

In `pkg/objtype/objtype.go`, replace the current second loop in `parseObjTypes` (currently L67-85):

```go
	for id := range count {
		config := otc.Configs[id]

		if config.CertTemplate != -1 {
			config.toCertificate(otc)
		}

		if os.Getenv("NODE_MEMBERS") == "false" && config.Members {
			config.Tradeable = false
			config.Op = nil
			config.IOp = nil

			for k, _ := range config.Params {
				if ptc.Configs[k].AutoDisable {
					delete(config.Params, k)
				}
			}
		}
	}

	return otc, nil
}
```

with:

```go
	applyPostDecodeFixups(otc, ptc)

	return otc, nil
}

func applyPostDecodeFixups(otc *ObjTypeConfigs, ptc *ParamTypeConfigs) {
	for id := range otc.Configs {
		config := otc.Configs[id]

		if config.CertTemplate != -1 {
			config.toCertificate(otc)
		}

		if os.Getenv("NODE_MEMBERS") == "false" && config.Members {
			config.Tradeable = false
			config.Op = nil
			config.IOp = nil

			for k, _ := range config.Params {
				if ptc.Configs[k].AutoDisable {
					delete(config.Params, k)
				}
			}
		}
	}
}
```

Note: the new helper iterates `range otc.Configs` instead of `range count` — semantically identical because `count == len(otc.Configs)` by construction at line 42.

- [ ] **Step 3: Run full pkg/objtype tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```
Expected: PASS — refactor preserves behavior.

- [ ] **Step 4: Commit**

```bash
git add pkg/objtype/objtype.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(objtype): NAI-152 T3 — extract applyPostDecodeFixups helper

Pulls the certtemplate-flatten + F2P-fixup loop body out of
parseObjTypes into a separate function so that T4 and T5 can pin
the F2P / dummyitem branches without spinning up a Jagfile fixture.

No behavior change; covered by existing pkg/objtype tests.
EOF
)"
```

---

### Task 4: F2P branch — `Op`/`IOp` reset to defaults + `Category = -1`

**Files:**
- Modify: `pkg/objtype/objtype.go` (`applyPostDecodeFixups`)
- Test: `pkg/objtype/objtype_test.go` (append)

**TS reference:** `Engine-TS/src/cache/config/ObjType.ts:60-74`:
```ts
if (!Environment.NODE_MEMBERS && config.members) {
    config.tradeable = false;
    config.op = [null, null, 'Take', null, null];
    config.iop = [null, null, null, null, 'Drop'];
    config.category = -1;
    config.params.forEach((_, key): void => { ... });
}
```

Goscape currently sets `config.Op = nil; config.IOp = nil` (which is wrong — strips the "Take"/"Drop" defaults entirely) and omits `Category = -1`.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/objtype/objtype_test.go`:

```go
func TestApplyPostDecodeFixupsF2PMembersResetsOpToTakeOnly(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "false")

	ot := NewObjType(0)
	ot.Members = true
	ot.Op[0] = "Wear" // simulates cache code 30 ("Wear")
	ot.IOp[0] = "Examine"

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	wantOp := []string{"", "", "Take", "", ""}
	for i, w := range wantOp {
		if got := ot.Op[i]; got != w {
			t.Errorf("Op[%d]: got %q, want %q", i, got, w)
		}
	}
	wantIOp := []string{"", "", "", "", "Drop"}
	for i, w := range wantIOp {
		if got := ot.IOp[i]; got != w {
			t.Errorf("IOp[%d]: got %q, want %q", i, got, w)
		}
	}
	if ot.Tradeable != false {
		t.Errorf("Tradeable: got %v, want false", ot.Tradeable)
	}
}

func TestApplyPostDecodeFixupsF2PMembersZeroesCategory(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "false")

	ot := NewObjType(0)
	ot.Members = true
	ot.Category = 42 // simulates cache code 94

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	if ot.Category != -1 {
		t.Errorf("Category: got %d, want -1", ot.Category)
	}
}

func TestApplyPostDecodeFixupsNonF2PMembersUnchanged(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "true")

	ot := NewObjType(0)
	ot.Members = true
	ot.Op[0] = "Wear"
	ot.Category = 42

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	// F2P branch must not fire when NODE_MEMBERS=true.
	if ot.Op[0] != "Wear" {
		t.Errorf("Op[0]: got %q, want \"Wear\" (F2P branch must not fire)", ot.Op[0])
	}
	if ot.Category != 42 {
		t.Errorf("Category: got %d, want 42 (F2P branch must not fire)", ot.Category)
	}
}
```

Note: `ParamTypeConfigs.Configs` is `[]*ParamType` (not a map). The F2P branch only indexes `ptc.Configs[k]` for keys present in `config.Params`; the test fixtures use `NewObjType`'s default empty `Params` map, so the inner loop is a no-op and the empty `&ParamTypeConfigs{}` is safe. Verify via `grep -n "ParamTypeConfigs\b" pkg/objtype/paramtype.go`.

- [ ] **Step 2: Run tests to verify F2P-reset and Category tests fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestApplyPostDecodeFixups -v
```
Expected:
- `TestApplyPostDecodeFixupsF2PMembersResetsOpToTakeOnly` FAIL — `Op[2]: got "" want "Take"` (current code nil-s Op).
- `TestApplyPostDecodeFixupsF2PMembersZeroesCategory` FAIL — `Category: got 42, want -1`.
- `TestApplyPostDecodeFixupsNonF2PMembersUnchanged` PASS (regression-pin only).

- [ ] **Step 3: Update F2P branch to TS-faithful reset**

In `pkg/objtype/objtype.go` `applyPostDecodeFixups`, replace:

```go
		if os.Getenv("NODE_MEMBERS") == "false" && config.Members {
			config.Tradeable = false
			config.Op = nil
			config.IOp = nil

			for k, _ := range config.Params {
				if ptc.Configs[k].AutoDisable {
					delete(config.Params, k)
				}
			}
		}
```

with:

```go
		if os.Getenv("NODE_MEMBERS") == "false" && config.Members {
			config.Tradeable = false
			config.Op = []string{"", "", "Take", "", ""}
			config.IOp = []string{"", "", "", "", "Drop"}
			config.Category = -1

			for k, _ := range config.Params {
				if ptc.Configs[k].AutoDisable {
					delete(config.Params, k)
				}
			}
		}
```

- [ ] **Step 4: Run tests, verify all pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestApplyPostDecodeFixups -v
```
Expected: all 3 PASS.

- [ ] **Step 5: Run full pkg/objtype tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/objtype.go pkg/objtype/objtype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-152 T4 — F2P branch resets Op/IOp + Category to TS defaults

Mirrors TS ObjType.ts:60-74. Pre-fix, the F2P/Members branch nil-ed
config.Op and config.IOp (stripping the "Take"/"Drop" defaults
entirely) and omitted config.category = -1 — meaning members objs on
F2P worlds were unpickup-able and retained their content-side category
triggers.

Closes NAI-152-D1a and NAI-152-D1b.
EOF
)"
```

---

### Task 5: `DummyItem != 0` → `Tradeable = false`

**Files:**
- Modify: `pkg/objtype/objtype.go` (`applyPostDecodeFixups`)
- Test: `pkg/objtype/objtype_test.go` (append)

**TS reference:** `Engine-TS/src/cache/config/ObjType.ts:56-58`:
```ts
if (config.dummyitem !== 0) {
    config.tradeable = false;
}
```

Order matters: in TS this branch runs **before** the F2P branch (after certtemplate flatten). Mirror that ordering in goscape.

- [ ] **Step 1: Write the failing test**

Append to `pkg/objtype/objtype_test.go`:

```go
func TestApplyPostDecodeFixupsDummyItemForcesTradeableFalse(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "true") // disable F2P branch

	ot := NewObjType(0)
	ot.DummyItem = 1
	ot.Tradeable = true // simulates cache code 200

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	if ot.Tradeable != false {
		t.Errorf("Tradeable: got %v, want false (DummyItem != 0)", ot.Tradeable)
	}
}

func TestApplyPostDecodeFixupsDummyItemZeroPreservesTradeable(t *testing.T) {
	t.Setenv("NODE_MEMBERS", "true")

	ot := NewObjType(0)
	ot.DummyItem = 0
	ot.Tradeable = true

	otc := &ObjTypeConfigs{Configs: []*ObjType{ot}}
	ptc := &ParamTypeConfigs{}

	applyPostDecodeFixups(otc, ptc)

	if ot.Tradeable != true {
		t.Errorf("Tradeable: got %v, want true (DummyItem == 0)", ot.Tradeable)
	}
}
```

- [ ] **Step 2: Run tests to verify the dummyitem test fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run "TestApplyPostDecodeFixupsDummyItem" -v
```
Expected:
- `TestApplyPostDecodeFixupsDummyItemForcesTradeableFalse` FAIL — `Tradeable: got true, want false`.
- `TestApplyPostDecodeFixupsDummyItemZeroPreservesTradeable` PASS.

- [ ] **Step 3: Add the DummyItem branch (before F2P branch)**

In `pkg/objtype/objtype.go` `applyPostDecodeFixups`, insert the new branch after the `toCertificate` call and before the F2P branch. Final shape:

```go
func applyPostDecodeFixups(otc *ObjTypeConfigs, ptc *ParamTypeConfigs) {
	for id := range otc.Configs {
		config := otc.Configs[id]

		if config.CertTemplate != -1 {
			config.toCertificate(otc)
		}

		if config.DummyItem != 0 {
			config.Tradeable = false
		}

		if os.Getenv("NODE_MEMBERS") == "false" && config.Members {
			config.Tradeable = false
			config.Op = []string{"", "", "Take", "", ""}
			config.IOp = []string{"", "", "", "", "Drop"}
			config.Category = -1

			for k, _ := range config.Params {
				if ptc.Configs[k].AutoDisable {
					delete(config.Params, k)
				}
			}
		}
	}
}
```

- [ ] **Step 4: Run tests, verify both pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestApplyPostDecodeFixupsDummyItem -v
```
Expected: both PASS.

- [ ] **Step 5: Run full pkg/objtype tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/objtype.go pkg/objtype/objtype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): NAI-152 T5 — DummyItem != 0 forces Tradeable=false

Mirrors TS ObjType.ts:56-58. Ordered after certtemplate flatten and
before the F2P branch, matching TS source-order semantics.

Closes NAI-152-D1c.
EOF
)"
```

---

### Task 6: Cross-package smoke (`go test ./...`)

**Files:** none (verification step)

After T1-T5 alter `NewObjType` defaults, run the full test suite to surface any consumer that was relying on `Op == nil` / `len(Op) == 0` / `Tradeable=true on dummyitems` semantics.

- [ ] **Step 1: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```
Expected: PASS across all packages.

- [ ] **Step 2: If any failure surfaces**

Most likely candidates (already pre-flighted but verify):
- `modules/world/handler_opheld.go:80` reads `objType.IOp[op-1]` against op=5; with the new `IOp[4]="Drop"` default, callers using `NewObjType(...)` rather than struct literals will now pass the gate. If a test relied on `IOp==nil`, the test was wrong; revisit.
- `modules/world/handler_opobj.go` (current B1.5 probe) reads `objType.Op[op-1]`. Same shape.
- Test fixtures using `&objtype.ObjType{...}` struct literals are unaffected — no defaults applied via struct literals.

If a real failure surfaces that wasn't anticipated, **stop and report**: it likely indicates a dependent consumer the spec didn't audit.

- [ ] **Step 3: No commit needed for verification step.** Proceed to T7 only if green.

---

### Task 7: Probe retirement + handler regression test + close commit

**Files:**
- Modify: `modules/world/handler_opobj.go` (strip 8 gateway sites + probe-only locals)
- Modify: `modules/world/handler_opobj_test.go` (remove 3 probe tests; add 1 regression test; clean imports)

This is the close commit. Strips the B1.5 NodeDebug probe and adds the handler-side regression test that pins the post-T1 behavior: an obj of a `NewObjType(...)` ObjType (no cache override) reaches the interaction.

- [ ] **Step 1: Enumerate probe sites**

```bash
rg -n "nai152.opobj.gate" modules/ pkg/
```
Expected output (11 hits, per spec §6.2 plan-author preflight):
```
modules/world/handler_opobj.go:30: ... "delayed" ...
modules/world/handler_opobj.go:38: ... "short_payload" ...
modules/world/handler_opobj.go:59: ... "viewport" ...
modules/world/handler_opobj.go:68: ... "getobj_nil" ...
modules/world/handler_opobj.go:76: ... "objtype_oob" ...
modules/world/handler_opobj.go:84: ... "objtype_nil" ...
modules/world/handler_opobj.go:96: ... "op_slot_empty" ...
modules/world/handler_opobj.go:103: ... "success" ...
modules/world/handler_opobj_test.go:784: ... contains check
modules/world/handler_opobj_test.go:809: ... contains check
modules/world/handler_opobj_test.go:834: ... contains check
```
Note exact line numbers may have drifted; rely on the rg output, not these.

- [ ] **Step 2: Read the current `handler_opobj.go` to map exact line ranges**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg -n "nai152|s\.cfg\.NodeDebug|opVal|opLen" modules/world/handler_opobj.go
```
This gives the exact code blocks to strip.

- [ ] **Step 3: Strip probe code in `handler_opobj.go`**

For each of the 8 `if s.cfg.NodeDebug { s.log.Info("nai152.opobj.gate", ...) }` blocks, delete the entire `if` block (including braces and any blank line before/after if it leaves a double-blank).

For probe-only locals: the `op_slot_empty` gate captures `opLen` and `opVal` solely for the probe log:
- `opLen := len(objType.Op)` — if used only by the probe log, delete the line.
- `opVal := ""; if op-1 < opLen { opVal = objType.Op[op-1] }` — if used only by the probe log, delete the lines.
- Verify by re-reading `handleOpObj` end-to-end; the production gate uses `len(objType.Op) < op || objType.Op[op-1] == ""` directly without needing local captures.

After stripping, the function should match the pre-B1.5 shape. Spot-check via:
```bash
grep -n "nai152\|NodeDebug" modules/world/handler_opobj.go
```
Expected: no hits.

- [ ] **Step 4: Remove probe-only tests in `handler_opobj_test.go`**

Delete these three test functions in their entirety (use the function-name boundaries; they are contiguous):
- `TestHandleOpObj_GatewayLogsOnSlotEmpty` (~L766-790)
- `TestHandleOpObj_GatewayLogsOnSuccess` (~L795-815)
- `TestHandleOpObj_GatewaySilentWhenNodeDebugFalse` (~L820-837)

After removal, check whether `bytes` and `log/slog` imports are still used elsewhere in the file:
```bash
grep -n "bytes\.\|slog\." modules/world/handler_opobj_test.go
```
If neither is used, remove from the import block at lines 3-16.

- [ ] **Step 5: Add the handler regression test**

Append to `modules/world/handler_opobj_test.go` (after the existing `TestHandleOpObjRejectsEmptyOpSlot` and before the OPOBJT section, or at the end of the file — placement is style-only):

```go
// TestHandleOpObjReachesInteractionWithDefaultOpType pins that an obj of a
// type constructed via NewObjType (no cache overrides) reaches the
// interaction — i.e., the default Op[2]="Take" populated by NewObjType
// passes the op_slot_empty gate. This is the post-NAI-152-B1 regression
// for the static-obj pickup symptom.
func TestHandleOpObjReachesInteractionWithDefaultOpType(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()

	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: make([]*objtype.ObjType, 559),
	}
	// Use NewObjType, NOT a struct literal — exercises the default Op/IOp
	// initializers added in T1.
	ot := objtype.NewObjType(558)
	ot.DebugName = "mindrune"
	s.objTypes.Configs[558] = ot

	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleRespawn, 558, 1)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Objs = append(zn.Objs, obj)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0
	p.originX, p.originZ = 100, 100

	if err := handleOpObj3(p, p2x3ObjPayload(100, 100, 558)); err != nil {
		t.Fatalf("handleOpObj3: %v", err)
	}

	if p.target != obj {
		t.Errorf("target: got %v, want obj (gate must not short-circuit on default Op[2]=\"Take\")", p.target)
	}
	if p.targetOp != 3 {
		t.Errorf("targetOp: got %d, want 3", p.targetOp)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
	if !p.opcalled {
		t.Error("opcalled: want true")
	}
}
```

Note: this test uses imports already present in the file (`entitypkg`, `objtype`, `zone`, `io2`); no import changes needed.

- [ ] **Step 6: Run modules/world tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```
Expected: PASS, including the new regression test.

- [ ] **Step 7: Verify probe is fully retired**

```bash
rg -n "nai152.opobj.gate" modules/ pkg/
```
Expected: empty output.

```bash
grep -n "NodeDebug\|nai152" modules/world/handler_opobj.go modules/world/handler_opobj_test.go
```
Expected: empty (no probe references remain in either file).

- [ ] **Step 8: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```
Expected: PASS.

- [ ] **Step 9: Close commit**

```bash
git add modules/world/handler_opobj.go modules/world/handler_opobj_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-152 B1 — retire B1.5 probe + pin handler regression

B1.5 probe (commit 1acb2e1) confirmed the op_slot_empty gate fired
with opLen=0 for mindrune, reframing B1 from handler-gate removal to
the loader fix landed in T1-T5. Probe is retired in this commit:
8 gateway log sites + 3 probe-only test cases + bytes/slog imports
they introduced.

Adds TestHandleOpObjReachesInteractionWithDefaultOpType — pins that
an obj of a NewObjType-constructed type (no cache overrides) reaches
SetInteraction via the default Op[2]="Take", matching TS-faithful
behavior at OpObjHandler.ts:36-50.

Smoke acceptance: user-driven mindrune pickup transitions from
"wholly silent" to either "Nothing interesting happens." chat
message (B2 domain) or successful pickup (skip B2).

Closes memory: nai152_b1_smoke_result.md
EOF
)"
```

---

## Self-review checklist (run before dispatch)

- [ ] **Spec coverage:**
  - §6.2 production change 1 (NewObjType defaults) → T1.
  - §6.2 production change 2 (drop dead Decode allocations) → T2.
  - §6.2 production change 3 (F2P branch reset + Category) → T4 (with helper extracted in T3).
  - §6.2 production change 4 (DummyItem branch) → T5.
  - §6.2 unit tests 1-7 → T1, T2, T4, T5 (test #4 hidden-collapse already exists; tests 5/6 split across T4 functions; test #7 → T5).
  - §6.2 handler regression (test #8) → T7.
  - §6.2 probe retirement → T7.
  - §6.2 smoke acceptance → T7 close-commit message references it; user-driven smoke is post-merge.
- [ ] **Placeholder scan:** no TBD/TODO; every step has runnable code or commands.
- [ ] **Type consistency:** `ObjTypeConfigs`, `ObjType`, `ParamTypeConfigs`, `ParamType` (T4/T5 fixtures); `applyPostDecodeFixups(otc, ptc)` signature consistent across T3-T5.
- [ ] **Run-everything order:** T1-T5 modify only `pkg/objtype/`; T6 is verification; T7 modifies `modules/world/`. Linear dependencies: T2 depends on T1 (defaults guarantee), T4/T5 depend on T3 (extracted helper).

## Smoke handoff (post-merge)

Per `smoke_test_server_handoff.md`: after the close commit, user runs the server with the Java client and attempts mindrune pickup. Predicted observables:

- If pickup completes (item in inventory + cleared from zone): B1 unblocked the full chain; advance to B3 (`Zone.RemoveObj` Respawn semantics).
- If `MessageGame("Nothing interesting happens.")` appears in client chat: B1 unblocked the handler; B2 (script chain) is the next bundle.
- If still wholly silent: a different short-circuit fires; re-introduce a one-shot probe and re-investigate.
