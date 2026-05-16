# NAI-209 Binary Script Writer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `compiler/writer/BaseScriptWriter.ts` + `runescript/ServerScriptOpcode.ts` + `runescript/SymbolMapper.ts` + `runescript/writer/BinaryScriptWriter{,Context}.ts` to goscape; extend the pipeline smoke with byte-pinned writer output.

**Architecture:** New leaf package `pkg/pack/compiler/writer/` (opcode IDs + `IdProvider` + `BaseContext` + helpers + `OpcodeWriter` interface + `WriteScript` dispatch). Extends `pkg/pack/compiler/runescript/` with `SymbolMapper` (implements `IdProvider`), `BinaryScriptWriterContext` (raw `[]byte`+offset instruction/switch buffers with random-access placeholder backpatch), `BinaryScriptWriter` (implements `OpcodeWriter`; emits via `BinaryOutput` interface). Adds one-line `IsNameMode(SubjectMode) bool` helper to `trigger`.

**Tech Stack:** Go 1.26+, `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`, `git commit --no-gpg-sign`. TS pin: LostCityRS/RuneScriptTS @ `b8c338801fbb72d294ff9576a58925a8d3f6de47`.

**Authoritative task numbering:** T1, T2, T3, T4, T5, T6, T7, T8, T9. Per `[[plan_code_block_t_number_drift]]`, all in-file doc comments and commit subjects must use this numbering.

**Spec:** `docs/superpowers/specs/2026-05-16-nai-209-binary-script-writer-design.md` (commit `b4792fb`).

---

## File Structure

**Created:**
- `pkg/pack/compiler/writer/opcode.go` — `ServerScriptOpcode` struct + 40 singletons + `All` (T1)
- `pkg/pack/compiler/writer/opcode_test.go` (T1)
- `pkg/pack/compiler/writer/id_provider.go` — `IdProvider` interface (T1)
- `pkg/pack/compiler/writer/base_context.go` — `BaseContext` struct + constructor + insertion-order `LineNumberPCs []int` (T3)
- `pkg/pack/compiler/writer/base_context_test.go` (T3)
- `pkg/pack/compiler/writer/helpers.go` — `GenerateLineNumberTable`, `GenerateJumpTable`, `GetParameterCount`, `GetLocalCount`, `GetVariableId` (T3)
- `pkg/pack/compiler/writer/helpers_test.go` (T3)
- `pkg/pack/compiler/writer/base_writer.go` — `OpcodeWriter` interface + `WriteScript` dispatch + `dispatch` switch (T4)
- `pkg/pack/compiler/writer/base_writer_test.go` (T4)
- `pkg/pack/compiler/runescript/symbol_mapper.go` (T2)
- `pkg/pack/compiler/runescript/symbol_mapper_test.go` (T2)
- `pkg/pack/compiler/runescript/binary_context.go` (T5)
- `pkg/pack/compiler/runescript/binary_context_test.go` (T5)
- `pkg/pack/compiler/runescript/binary_writer.go` (T6, T7)
- `pkg/pack/compiler/runescript/binary_writer_test.go` (T6)
- `pkg/pack/compiler/runescript/binary_writer_lookup_test.go` (T7)
- `pkg/pack/compiler/runescript/nai209_deviation_pins_test.go` (T9)

**Modified:**
- `pkg/pack/compiler/trigger/subjectmode.go` — add `IsNameMode(SubjectMode) bool` (T7)
- `pkg/pack/compiler/codegen/smoke_test.go` — extend `TestPipeline_FullSlice` with writer step + byte-pin (T8)

**Deviation tags** (T9 pins them in `nai209_deviation_pins_test.go`):
- `NAI-209-D-BYTEPACKET-DEFER`
- `NAI-209-D-SYMMAPPER-DIAG-CTOR`
- `NAI-209-D-PUSHLONG-PANIC`
- `NAI-209-D-MAPZONE-COORD-PARSE-PANIC`
- `NAI-209-D-OPCODE-WRITER-INTERFACE`
- `NAI-209-D-BINARYOUTPUT-INTERFACE`
- `NAI-209-D-LINENUMBER-ORDER-SLICE`
- `NAI-209-D-DEBUGPROC-TRIGGER-STRING-CHECK` (string-compare `Trigger.Identifier == "debugproc"` because the DEBUGPROC trigger singleton has not yet been ported)

---

## Task 1: `ServerScriptOpcode` + `IdProvider`

**Files:**
- Create: `pkg/pack/compiler/writer/opcode.go`
- Create: `pkg/pack/compiler/writer/id_provider.go`
- Test: `pkg/pack/compiler/writer/opcode_test.go`

**TS source:** `src/runescript/ServerScriptOpcode.ts` (98 lines). Port the 40 singletons verbatim; numeric IDs and `LargeOperand` flags must match.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/writer/opcode_test.go`:

```go
// pkg/pack/compiler/writer/opcode_test.go
package writer

import "testing"

// TestServerScriptOpcode_IDs pins the numeric ID of every opcode singleton.
// Verbatim from TS src/runescript/ServerScriptOpcode.ts.
func TestServerScriptOpcode_IDs(t *testing.T) {
	cases := []struct {
		name   string
		op     *ServerScriptOpcode
		id     uint16
		large  bool
	}{
		{"PushConstantInt", OpPushConstantInt, 0, true},
		{"PushVarp", OpPushVarp, 1, true},
		{"PopVarp", OpPopVarp, 2, true},
		{"PushConstantString", OpPushConstantString, 3, true},
		{"PushVarn", OpPushVarn, 4, true},
		{"PopVarn", OpPopVarn, 5, true},
		{"Branch", OpBranch, 6, true},
		{"BranchNot", OpBranchNot, 7, true},
		{"BranchEquals", OpBranchEquals, 8, true},
		{"BranchLessThan", OpBranchLessThan, 9, true},
		{"BranchGreaterThan", OpBranchGreaterThan, 10, true},
		{"PushVars", OpPushVars, 11, true},
		{"PopVars", OpPopVars, 12, true},
		{"Return", OpReturn, 21, false},
		{"Gosub", OpGosub, 22, false},
		{"Jump", OpJump, 23, false},
		{"Switch", OpSwitch, 24, true},
		{"PushVarbit", OpPushVarbit, 25, true},
		{"PopVarbit", OpPopVarbit, 27, true},
		{"BranchLessThanOrEquals", OpBranchLessThanOrEquals, 31, true},
		{"BranchGreaterThanOrEquals", OpBranchGreaterThanOrEquals, 32, true},
		{"PushIntLocal", OpPushIntLocal, 33, true},
		{"PopIntLocal", OpPopIntLocal, 34, true},
		{"PushStringLocal", OpPushStringLocal, 35, true},
		{"PopStringLocal", OpPopStringLocal, 36, true},
		{"JoinString", OpJoinString, 37, true},
		{"PopIntDiscard", OpPopIntDiscard, 38, false},
		{"PopStringDiscard", OpPopStringDiscard, 39, false},
		{"GosubWithParams", OpGosubWithParams, 40, true},
		{"JumpWithParams", OpJumpWithParams, 41, true},
		{"DefineArray", OpDefineArray, 44, true},
		{"PushArrayInt", OpPushArrayInt, 45, true},
		{"PopArrayInt", OpPopArrayInt, 46, true},
		{"Add", OpAdd, 4600, false},
		{"Sub", OpSub, 4601, false},
		{"Multiply", OpMultiply, 4602, false},
		{"Divide", OpDivide, 4603, false},
		{"Modulo", OpModulo, 4611, false},
		{"And", OpAnd, 4614, false},
		{"Or", OpOr, 4615, false},
	}
	for _, c := range cases {
		if c.op == nil {
			t.Errorf("%s: singleton is nil", c.name)
			continue
		}
		if c.op.ID != c.id {
			t.Errorf("%s.ID = %d, want %d", c.name, c.op.ID, c.id)
		}
		if c.op.LargeOperand != c.large {
			t.Errorf("%s.LargeOperand = %v, want %v", c.name, c.op.LargeOperand, c.large)
		}
	}
	if len(All) != len(cases) {
		t.Errorf("len(All) = %d, want %d", len(All), len(cases))
	}
}

// TestServerScriptOpcode_AllUniqueIDs pins that no two singletons share an ID.
func TestServerScriptOpcode_AllUniqueIDs(t *testing.T) {
	seen := map[uint16]string{}
	for _, op := range All {
		if prev, ok := seen[op.ID]; ok {
			t.Errorf("duplicate ID %d on opcode (also seen on %s)", op.ID, prev)
		}
		seen[op.ID] = "(?)"
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/writer/...
```

Expected: build failure — package does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/writer/opcode.go`:

```go
// Package writer ports TS src/compiler/writer/ + src/runescript/ServerScriptOpcode.ts.
// It exposes the numeric ServerScriptOpcode IDs, the IdProvider interface
// (numeric mapping for symbols), and the OpcodeWriter dispatch interface that
// concrete writers (binary/text/etc.) implement.
//
// Mirrors TS BaseScriptWriter abstract class via a Go interface + free function
// (NAI-209-D-OPCODE-WRITER-INTERFACE).
package writer

// ServerScriptOpcode is one entry in the binary opcode table. ID is the numeric
// opcode written to the binary stream; LargeOperand selects between a 1-byte
// operand and a 4-byte operand encoding. Mirrors TS ServerScriptOpcode.ts.
type ServerScriptOpcode struct {
	ID           uint16
	LargeOperand bool
}

// Core language ops (0-99). IDs are verbatim from TS ServerScriptOpcode.ts L13-46.
var (
	OpPushConstantInt           = &ServerScriptOpcode{ID: 0, LargeOperand: true}
	OpPushVarp                  = &ServerScriptOpcode{ID: 1, LargeOperand: true}
	OpPopVarp                   = &ServerScriptOpcode{ID: 2, LargeOperand: true}
	OpPushConstantString        = &ServerScriptOpcode{ID: 3, LargeOperand: true}
	OpPushVarn                  = &ServerScriptOpcode{ID: 4, LargeOperand: true}
	OpPopVarn                   = &ServerScriptOpcode{ID: 5, LargeOperand: true}
	OpBranch                    = &ServerScriptOpcode{ID: 6, LargeOperand: true}
	OpBranchNot                 = &ServerScriptOpcode{ID: 7, LargeOperand: true}
	OpBranchEquals              = &ServerScriptOpcode{ID: 8, LargeOperand: true}
	OpBranchLessThan            = &ServerScriptOpcode{ID: 9, LargeOperand: true}
	OpBranchGreaterThan         = &ServerScriptOpcode{ID: 10, LargeOperand: true}
	OpPushVars                  = &ServerScriptOpcode{ID: 11, LargeOperand: true}
	OpPopVars                   = &ServerScriptOpcode{ID: 12, LargeOperand: true}
	OpReturn                    = &ServerScriptOpcode{ID: 21}
	OpGosub                     = &ServerScriptOpcode{ID: 22}
	OpJump                      = &ServerScriptOpcode{ID: 23}
	OpSwitch                    = &ServerScriptOpcode{ID: 24, LargeOperand: true}
	OpPushVarbit                = &ServerScriptOpcode{ID: 25, LargeOperand: true}
	OpPopVarbit                 = &ServerScriptOpcode{ID: 27, LargeOperand: true}
	OpBranchLessThanOrEquals    = &ServerScriptOpcode{ID: 31, LargeOperand: true}
	OpBranchGreaterThanOrEquals = &ServerScriptOpcode{ID: 32, LargeOperand: true}
	OpPushIntLocal              = &ServerScriptOpcode{ID: 33, LargeOperand: true}
	OpPopIntLocal               = &ServerScriptOpcode{ID: 34, LargeOperand: true}
	OpPushStringLocal           = &ServerScriptOpcode{ID: 35, LargeOperand: true}
	OpPopStringLocal            = &ServerScriptOpcode{ID: 36, LargeOperand: true}
	OpJoinString                = &ServerScriptOpcode{ID: 37, LargeOperand: true}
	OpPopIntDiscard             = &ServerScriptOpcode{ID: 38}
	OpPopStringDiscard          = &ServerScriptOpcode{ID: 39}
	OpGosubWithParams           = &ServerScriptOpcode{ID: 40, LargeOperand: true}
	OpJumpWithParams            = &ServerScriptOpcode{ID: 41, LargeOperand: true}
	OpDefineArray               = &ServerScriptOpcode{ID: 44, LargeOperand: true}
	OpPushArrayInt              = &ServerScriptOpcode{ID: 45, LargeOperand: true}
	OpPopArrayInt               = &ServerScriptOpcode{ID: 46, LargeOperand: true}
)

// Number ops (4600-4699). Verbatim from TS L48-54.
var (
	OpAdd      = &ServerScriptOpcode{ID: 4600}
	OpSub      = &ServerScriptOpcode{ID: 4601}
	OpMultiply = &ServerScriptOpcode{ID: 4602}
	OpDivide   = &ServerScriptOpcode{ID: 4603}
	OpModulo   = &ServerScriptOpcode{ID: 4611}
	OpAnd      = &ServerScriptOpcode{ID: 4614}
	OpOr       = &ServerScriptOpcode{ID: 4615}
)

// All enumerates every defined ServerScriptOpcode singleton in the same order
// as TS ServerScriptOpcode.ALL (L56-97). Stable iteration order; safe to
// range over.
var All = []*ServerScriptOpcode{
	OpPushConstantInt, OpPushVarp, OpPopVarp, OpPushConstantString,
	OpPushVarn, OpPopVarn, OpBranch, OpBranchNot, OpBranchEquals,
	OpBranchLessThan, OpBranchGreaterThan, OpPushVars, OpPopVars,
	OpReturn, OpGosub, OpJump, OpSwitch, OpPushVarbit, OpPopVarbit,
	OpBranchLessThanOrEquals, OpBranchGreaterThanOrEquals,
	OpPushIntLocal, OpPopIntLocal, OpPushStringLocal, OpPopStringLocal,
	OpJoinString, OpPopIntDiscard, OpPopStringDiscard,
	OpGosubWithParams, OpJumpWithParams, OpDefineArray,
	OpPushArrayInt, OpPopArrayInt,
	OpAdd, OpSub, OpMultiply, OpDivide, OpModulo, OpAnd, OpOr,
}
```

Create `pkg/pack/compiler/writer/id_provider.go`:

```go
package writer

import "github.com/zsrv/goscape/pkg/pack/compiler/symbol"

// IdProvider maps a compiler-side symbol.Symbol to its runtime numeric ID.
// Concrete impl lives in pkg/pack/compiler/runescript/symbol_mapper.go.
//
// Returns -1 for missing script/command symbols (mirrors TS SymbolMapper
// behavior — TS reports a diagnostic and returns -1, never throws for
// those). For missing basic symbols TS throws; the concrete impl in goscape
// panics for parity.
//
// Mirrors TS src/compiler/writer/BaseScriptWriter.ts L304-313 (IdProvider).
type IdProvider interface {
	Get(s symbol.Symbol) int
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/writer/...
```

Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/writer/opcode.go pkg/pack/compiler/writer/id_provider.go pkg/pack/compiler/writer/opcode_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/writer): NAI-209 T1 — ServerScriptOpcode + IdProvider

Ports TS ServerScriptOpcode.ts and BaseScriptWriter.IdProvider. 40 singletons
with verbatim numeric IDs + LargeOperand flags + All slice.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `SymbolMapper`

**Files:**
- Create: `pkg/pack/compiler/runescript/symbol_mapper.go`
- Test: `pkg/pack/compiler/runescript/symbol_mapper_test.go`

**TS source:** `src/runescript/SymbolMapper.ts` (90 lines). Three private maps; `Get` branches on `ScriptSymbol` and uses `CommandTrigger`-equality to choose between command/script tables.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/symbol_mapper_test.go`:

```go
// pkg/pack/compiler/runescript/symbol_mapper_test.go
package runescript_test

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestSymbolMapper_PutGetCommand pins the command-symbol path: script
// symbol whose Trigger == CommandTrigger looks up via the commands map,
// dot-prefix stripped per TS SymbolMapper.get L60-68.
func TestSymbolMapper_PutGetCommand(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	m.PutCommand(42, "mes")
	cmd := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{
			Trigger: trigger.CommandTrigger,
			Name:    "mes",
		},
	}
	if got := m.Get(cmd); got != 42 {
		t.Errorf("Get(mes) = %d, want 42", got)
	}
	// Dot-prefixed name: TS strips everything up to and including the first dot.
	dot := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{
			Trigger: trigger.CommandTrigger,
			Name:    ".mes",
		},
	}
	if got := m.Get(dot); got != 42 {
		t.Errorf("Get(.mes) = %d, want 42 (dot stripped)", got)
	}
}

// TestSymbolMapper_PutGetScript pins the script-symbol path: non-command
// trigger looks up via the scripts map keyed by "[ident,name]".
func TestSymbolMapper_PutGetScript(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	m.PutScript(7, "[proc,foo]")
	sym := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"},
	}
	if got := m.Get(sym); got != 7 {
		t.Errorf("Get([proc,foo]) = %d, want 7", got)
	}
}

// TestSymbolMapper_PutGetSymbol pins the BasicSymbol path: direct map lookup.
func TestSymbolMapper_PutGetSymbol(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	b := &symbol.BasicSymbol{Name: "weapon1", Type: typ.PrimitiveInt}
	m.PutSymbol(99, b)
	if got := m.Get(b); got != 99 {
		t.Errorf("Get(weapon1) = %d, want 99", got)
	}
}

// TestSymbolMapper_MissingCommand pins the report-and-return-(-1) semantics
// for an unknown command symbol. Verifies a diagnostic was reported.
func TestSymbolMapper_MissingCommand(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	cmd := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{
			Trigger: trigger.CommandTrigger,
			Name:    "ghost",
		},
	}
	if got := m.Get(cmd); got != -1 {
		t.Errorf("Get(ghost) = %d, want -1", got)
	}
	if len(d.List()) != 1 {
		t.Errorf("diagnostics: got %d, want 1", len(d.List()))
	}
}

// TestSymbolMapper_MissingScript pins -1 + diagnostic for unknown script.
func TestSymbolMapper_MissingScript(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	sym := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "ghost"},
	}
	if got := m.Get(sym); got != -1 {
		t.Errorf("Get([proc,ghost]) = %d, want -1", got)
	}
	if len(d.List()) != 1 {
		t.Errorf("diagnostics: got %d, want 1", len(d.List()))
	}
}

// TestSymbolMapper_MissingBasicPanics pins the TS `throw new Error` parity
// for an unmapped non-script symbol.
func TestSymbolMapper_MissingBasicPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Get on unmapped BasicSymbol did not panic")
		}
	}()
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	b := &symbol.BasicSymbol{Name: "x", Type: typ.PrimitiveInt}
	m.Get(b)
}

// TestSymbolMapper_DuplicateSymbolDispatches pins duplicate-PutSymbol behavior:
// reports diagnostic, leaves first mapping intact.
func TestSymbolMapper_DuplicateSymbolDispatches(t *testing.T) {
	d := &diagnostics.Diagnostics{}
	m := runescript.NewSymbolMapper(d)
	b := &symbol.BasicSymbol{Name: "x", Type: typ.PrimitiveInt}
	m.PutSymbol(1, b)
	m.PutSymbol(2, b)
	if got := m.Get(b); got != 1 {
		t.Errorf("Get after duplicate Put = %d, want 1 (first wins)", got)
	}
	if len(d.List()) != 1 {
		t.Errorf("diagnostics on duplicate: got %d, want 1", len(d.List()))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/...
```

Expected: build failure (`NewSymbolMapper undefined`).

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/runescript/symbol_mapper.go`:

```go
// pkg/pack/compiler/runescript/symbol_mapper.go
package runescript

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// SymbolMapper maps compiler symbols to their numeric runtime IDs. Implements
// writer.IdProvider. Mirrors TS src/runescript/SymbolMapper.ts.
//
// Three internal tables:
//   - commands: indexed by stripped command name (".foo" → "foo")
//   - scripts:  indexed by full TS-style key "[trigger,name]"
//   - symbols:  any other symbol.Symbol (Basic/Local/Constant)
//
// NAI-209-D-SYMMAPPER-DIAG-CTOR: TS reads (symbol as any).context?.diagnostics
// on the fly; goscape symbols carry no context field, so diagnostics is
// constructor-injected. Diagnostics may be nil for tests that don't need to
// observe duplicate/missing reports.
type SymbolMapper struct {
	diags    *diagnostics.Diagnostics
	commands map[string]int
	scripts  map[string]int
	symbols  map[symbol.Symbol]int
}

// NewSymbolMapper returns a fresh SymbolMapper. diags may be nil.
func NewSymbolMapper(diags *diagnostics.Diagnostics) *SymbolMapper {
	return &SymbolMapper{
		diags:    diags,
		commands: map[string]int{},
		scripts:  map[string]int{},
		symbols:  map[symbol.Symbol]int{},
	}
}

// Compile-time assertion that SymbolMapper satisfies writer.IdProvider.
var _ writer.IdProvider = (*SymbolMapper)(nil)

// PutSymbol assigns id to s. If s is already mapped, reports a duplicate
// diagnostic (when diags is non-nil) and leaves the first mapping intact.
// Mirrors TS SymbolMapper.putSymbol L32-40.
func (m *SymbolMapper) PutSymbol(id int, s symbol.Symbol) {
	if _, dup := m.symbols[s]; dup {
		m.report(fmt.Sprintf("Duplicate symbol: %s.", s.SymbolName()))
		return
	}
	m.symbols[s] = id
}

// PutCommand maps name → id. Duplicate names are silently ignored
// (TS has no diagnostics-context for the bare-name path: SymbolMapper.ts L42-48).
func (m *SymbolMapper) PutCommand(id int, name string) {
	if _, dup := m.commands[name]; dup {
		return
	}
	m.commands[name] = id
}

// PutScript maps a "[ident,name]"-shaped key → id. Duplicates silently ignored
// (TS L50-56).
func (m *SymbolMapper) PutScript(id int, name string) {
	if _, dup := m.scripts[name]; dup {
		return
	}
	m.scripts[name] = id
}

// Get returns the runtime ID for s. For script symbols, branches on
// Trigger == CommandTrigger to choose between the commands and scripts
// tables. Returns -1 for missing script/command symbols (TS reports and
// returns -1). Panics for missing basic/local symbols (TS throws).
// Mirrors TS SymbolMapper.get L58-89.
func (m *SymbolMapper) Get(s symbol.Symbol) int {
	switch ss := s.(type) {
	case *symbol.ServerScriptSymbol:
		return m.getScript(ss.Trigger, ss.Name, s.SymbolName())
	case *symbol.ClientScriptSymbol:
		return m.getScript(ss.Trigger, ss.Name, s.SymbolName())
	}
	id, ok := m.symbols[s]
	if !ok {
		panic(fmt.Sprintf("SymbolMapper: unable to find id for %q.", s.SymbolName()))
	}
	return id
}

func (m *SymbolMapper) getScript(t *trigger.TriggerType, name, repr string) int {
	if t == trigger.CommandTrigger {
		// Trim everything up to and including the first dot (TS substring).
		key := name
		if i := strings.IndexByte(name, '.'); i >= 0 {
			key = name[i+1:]
		}
		id, ok := m.commands[key]
		if !ok {
			m.report(fmt.Sprintf("Unable to find id for '%s'.", repr))
			return -1
		}
		return id
	}
	key := "[" + t.Identifier + "," + name + "]"
	id, ok := m.scripts[key]
	if !ok {
		m.report(fmt.Sprintf("Unable to find id for '%s'.", repr))
		return -1
	}
	return id
}

func (m *SymbolMapper) report(msg string) {
	if m.diags == nil {
		return
	}
	m.diags.Report(diagnostics.NewDiagnostic(
		lexer.NodeSourceLocation{},
		diagnostics.DiagnosticError,
		diagnostics.MessageGenericInvalidType,
		msg,
	))
}
```

Note: if `diagnostics.NewDiagnostic` / `MessageGenericInvalidType` signatures differ in the live codebase, swap to whatever the existing reporter uses (T2 implementer must grep `pkg/pack/compiler/diagnostics/` for the right constructor — the placeholder in this plan is the shape established at NAI-205+, but adjust to match HEAD). The error-class is incidental; the test only asserts `len(diagnostics) == 1`.

- [ ] **Step 4: Run tests to verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/...
```

Expected: 7 PASS (the 7 mapper tests + pre-existing `ServerPointerChecker` tests untouched).

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/runescript/symbol_mapper.go pkg/pack/compiler/runescript/symbol_mapper_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-209 T2 — SymbolMapper

Implements writer.IdProvider via three internal tables (commands/scripts/symbols),
with dot-prefix stripping for command lookups and full "[ident,name]" keys for
scripts. Diagnostics ctor-injected (NAI-209-D-SYMMAPPER-DIAG-CTOR).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `BaseContext` + helpers

**Files:**
- Create: `pkg/pack/compiler/writer/base_context.go`
- Create: `pkg/pack/compiler/writer/helpers.go`
- Test: `pkg/pack/compiler/writer/base_context_test.go`
- Test: `pkg/pack/compiler/writer/helpers_test.go`

**TS source:** `BaseScriptWriter.ts` L219-282 (helpers) + L285-299 (context).

- [ ] **Step 1: Write the failing tests**

Create `pkg/pack/compiler/writer/helpers_test.go`:

```go
// pkg/pack/compiler/writer/helpers_test.go
package writer_test

import (
	"reflect"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// TestGenerateJumpTable_TwoBlocks pins jump-table layout for a 2-block
// script: block A has 3 instructions, block B has 2. JumpTable[A.Label] = 0,
// JumpTable[B.Label] = 3.
func TestGenerateJumpTable_TwoBlocks(t *testing.T) {
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)

	la := &codegen.Label{Name: "a"}
	lb := &codegen.Label{Name: "b"}
	ba := codegen.NewBlock(la)
	bb := codegen.NewBlock(lb)
	ba.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 1})
	ba.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 2})
	ba.Add(codegen.Instruction{Opcode: codegen.Return})
	bb.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 3})
	bb.Add(codegen.Instruction{Opcode: codegen.Return})
	script.Blocks = []*codegen.Block{ba, bb}

	jt := writer.GenerateJumpTable(script)
	if got := jt[la]; got != 0 {
		t.Errorf("JumpTable[a] = %d, want 0", got)
	}
	if got := jt[lb]; got != 3 {
		t.Errorf("JumpTable[b] = %d, want 3", got)
	}
}

// TestGenerateLineNumberTable_DistinctLines pins the table + the parallel
// insertion-order slice. NAI-209-D-LINENUMBER-ORDER-SLICE: Go maps are
// non-deterministic; consumers iterate via the slice.
func TestGenerateLineNumberTable_DistinctLines(t *testing.T) {
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)

	mk := func(line int) codegen.Instruction {
		return codegen.Instruction{
			Opcode:  codegen.PushConstantInt,
			Operand: 0,
			Source:  lexer.NodeSourceLocation{Line: line},
		}
	}
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	b.Add(mk(1)) // pc 0 → line 1
	b.Add(mk(1)) // pc 1 → same line, skipped
	b.Add(mk(2)) // pc 2 → line 2
	b.Add(mk(2)) // pc 3 → same line, skipped
	b.Add(mk(5)) // pc 4 → line 5 (gap is fine)
	script.Blocks = []*codegen.Block{b}

	tbl, order := writer.GenerateLineNumberTable(script)
	want := map[int]int{0: 1, 2: 2, 4: 5}
	if !reflect.DeepEqual(tbl, want) {
		t.Errorf("LineNumberTable = %v, want %v", tbl, want)
	}
	wantOrder := []int{0, 2, 4}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Errorf("LineNumberPCs = %v, want %v", order, wantOrder)
	}
}

// TestGetVariableId_ParamsAndLocals pins the index returned by GetVariableId
// for each LocalVariableSymbol — both parameter int + parameter string +
// declared int local + an int array.
//
// Layout in LocalTable.All:
//   [intParam, strParam, intLocal, intArr]
// Expected indices (post-filter):
//   intParam  -> index 0 among ints (intParam, intLocal): 0
//   strParam  -> index 0 among strings: 0
//   intLocal  -> index 1 among ints: 1
//   intArr    -> index 0 among arrays: 0
func TestGetVariableId_ParamsAndLocals(t *testing.T) {
	intParam := &symbol.LocalVariableSymbol{Name: "$ip", Type: typ.PrimitiveInt}
	strParam := &symbol.LocalVariableSymbol{Name: "$sp", Type: typ.PrimitiveString}
	intLocal := &symbol.LocalVariableSymbol{Name: "$il", Type: typ.PrimitiveInt}
	arr, _ := typ.NewArrayType(typ.PrimitiveInt)
	intArr := &symbol.LocalVariableSymbol{Name: "$ia", Type: arr}

	locals := &codegen.LocalTable{
		Parameters: []*symbol.LocalVariableSymbol{intParam, strParam},
		All:        []*symbol.LocalVariableSymbol{intParam, strParam, intLocal, intArr},
	}

	if got := writer.GetVariableId(locals, intParam); got != 0 {
		t.Errorf("intParam id = %d, want 0", got)
	}
	if got := writer.GetVariableId(locals, strParam); got != 0 {
		t.Errorf("strParam id = %d, want 0", got)
	}
	if got := writer.GetVariableId(locals, intLocal); got != 1 {
		t.Errorf("intLocal id = %d, want 1", got)
	}
	if got := writer.GetVariableId(locals, intArr); got != 0 {
		t.Errorf("intArr id = %d, want 0", got)
	}
}

// TestGetCounts pins GetParameterCount + GetLocalCount.
// GetLocalCount excludes arrays unless the array is a parameter.
func TestGetCounts(t *testing.T) {
	intParam := &symbol.LocalVariableSymbol{Name: "$ip", Type: typ.PrimitiveInt}
	strParam := &symbol.LocalVariableSymbol{Name: "$sp", Type: typ.PrimitiveString}
	intLocal := &symbol.LocalVariableSymbol{Name: "$il", Type: typ.PrimitiveInt}
	arr, _ := typ.NewArrayType(typ.PrimitiveInt)
	intArr := &symbol.LocalVariableSymbol{Name: "$ia", Type: arr}

	locals := &codegen.LocalTable{
		Parameters: []*symbol.LocalVariableSymbol{intParam, strParam},
		All:        []*symbol.LocalVariableSymbol{intParam, strParam, intLocal, intArr},
	}

	if got := writer.GetParameterCount(locals, typ.BaseVarInteger); got != 1 {
		t.Errorf("ParameterCount(Integer) = %d, want 1", got)
	}
	if got := writer.GetParameterCount(locals, typ.BaseVarString); got != 1 {
		t.Errorf("ParameterCount(String) = %d, want 1", got)
	}
	// Two int locals counted (intParam, intLocal); intArr excluded since it
	// is an ArrayType AND it is not a parameter.
	if got := writer.GetLocalCount(locals, typ.BaseVarInteger); got != 2 {
		t.Errorf("LocalCount(Integer) = %d, want 2", got)
	}
}
```

Create `pkg/pack/compiler/writer/base_context_test.go`:

```go
// pkg/pack/compiler/writer/base_context_test.go
package writer_test

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// TestNewBaseContext_PopulatesJumpAndLineTables pins that the ctor invokes
// the static helpers — both tables non-nil and CurIndex zero.
func TestNewBaseContext_PopulatesJumpAndLineTables(t *testing.T) {
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	b.Add(codegen.Instruction{Opcode: codegen.Return})
	script.Blocks = []*codegen.Block{b}

	ctx := writer.NewBaseContext(script)
	if ctx.Script != script {
		t.Errorf("ctx.Script: pointer mismatch")
	}
	if ctx.CurIndex != 0 {
		t.Errorf("CurIndex = %d, want 0", ctx.CurIndex)
	}
	if ctx.JumpTable == nil {
		t.Errorf("JumpTable nil")
	}
	if ctx.LineNumberTable == nil {
		t.Errorf("LineNumberTable nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/writer/...
```

Expected: build failure (helpers, `NewBaseContext` undefined).

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/writer/helpers.go`:

```go
// pkg/pack/compiler/writer/helpers.go
package writer

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// GenerateLineNumberTable walks the script's instruction stream and returns
// (pc → source-line) for every instruction that introduces a *new* source
// line (TS skips runs of same-line instructions). Mirrors TS
// BaseScriptWriter.generateLineNumberTable L219-235.
//
// NAI-209-D-LINENUMBER-ORDER-SLICE: also returns a []int of pcs in insertion
// order, since Go map iteration is randomized but BinaryScriptWriterContext.Finish
// must emit entries in pc-ascending order for byte-identical output.
func GenerateLineNumberTable(script *codegen.RuneScript) (map[int]int, []int) {
	tbl := map[int]int{}
	var order []int
	index := 0
	prevLine := -1
	for _, block := range script.Blocks {
		for _, ins := range block.Instructions {
			line := ins.Source.Line
			if line != 0 && line != prevLine {
				tbl[index] = line
				order = append(order, index)
				prevLine = line
			}
			index++
		}
	}
	return tbl, order
}

// GenerateJumpTable returns label → pc-of-its-first-instruction for every
// Block. Mirrors TS BaseScriptWriter.generateJumpTable L243-251.
func GenerateJumpTable(script *codegen.RuneScript) map[*codegen.Label]int {
	tbl := map[*codegen.Label]int{}
	index := 0
	for _, block := range script.Blocks {
		tbl[block.Label] = index
		index += len(block.Instructions)
	}
	return tbl
}

// GetParameterCount returns the number of parameters in locals whose
// Type.BaseType matches baseType. Mirrors TS getParameterCount L262-264.
func GetParameterCount(locals *codegen.LocalTable, baseType typ.BaseVarType) int {
	n := 0
	for _, p := range locals.Parameters {
		if bt, ok := p.Type.BaseType(); ok && bt == baseType {
			n++
		}
	}
	return n
}

// GetLocalCount returns the number of locals (including parameters) of
// baseType, excluding ArrayType locals UNLESS they are parameters.
// Mirrors TS getLocalCount L269-271.
func GetLocalCount(locals *codegen.LocalTable, baseType typ.BaseVarType) int {
	n := 0
	for _, v := range locals.All {
		bt, ok := v.Type.BaseType()
		if !ok || bt != baseType {
			continue
		}
		if _, isArr := v.Type.(*typ.ArrayType); isArr && !containsLocal(locals.Parameters, v) {
			continue
		}
		n++
	}
	return n
}

// GetVariableId returns the unique runtime ID for local within its locals
// table. The ID is the index among locals of the *same* slot-shape
// (array-vs-scalar; scalar further partitioned by BaseVarType, with
// non-parameter arrays excluded from the scalar pool). Mirrors TS
// getVariableId L276-282.
func GetVariableId(locals *codegen.LocalTable, local *symbol.LocalVariableSymbol) int {
	if _, isArr := local.Type.(*typ.ArrayType); isArr {
		n := 0
		for _, v := range locals.All {
			if v == local {
				return n
			}
			if _, isArr := v.Type.(*typ.ArrayType); isArr {
				n++
			}
		}
		return -1
	}
	bt, _ := local.Type.BaseType()
	n := 0
	for _, v := range locals.All {
		if v == local {
			return n
		}
		vbt, ok := v.Type.BaseType()
		if !ok || vbt != bt {
			continue
		}
		if _, isArr := v.Type.(*typ.ArrayType); isArr && !containsLocal(locals.Parameters, v) {
			continue
		}
		n++
	}
	return -1
}

func containsLocal(xs []*symbol.LocalVariableSymbol, target *symbol.LocalVariableSymbol) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
```

Create `pkg/pack/compiler/writer/base_context.go`:

```go
// pkg/pack/compiler/writer/base_context.go
package writer

import "github.com/zsrv/goscape/pkg/pack/compiler/codegen"

// BaseContext is the shared per-script writer context: tracks the current
// instruction pc plus the precomputed line-number and jump tables.
// Concrete writer contexts (binary/text) embed *BaseContext.
//
// Mirrors TS BaseScriptWriterContext (BaseScriptWriter.ts L289-299).
//
// NAI-209-D-LINENUMBER-ORDER-SLICE: LineNumberPCs holds the insertion-order
// pcs from the LineNumberTable so that Finish() can iterate deterministically.
type BaseContext struct {
	Script          *codegen.RuneScript
	CurIndex        int
	LineNumberTable map[int]int
	LineNumberPCs   []int
	JumpTable       map[*codegen.Label]int
}

// NewBaseContext populates both tables eagerly via GenerateLineNumberTable
// and GenerateJumpTable.
func NewBaseContext(script *codegen.RuneScript) *BaseContext {
	tbl, order := GenerateLineNumberTable(script)
	return &BaseContext{
		Script:          script,
		LineNumberTable: tbl,
		LineNumberPCs:   order,
		JumpTable:       GenerateJumpTable(script),
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/writer/...
```

Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/writer/base_context.go pkg/pack/compiler/writer/helpers.go pkg/pack/compiler/writer/base_context_test.go pkg/pack/compiler/writer/helpers_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/writer): NAI-209 T3 — BaseContext + helpers

BaseContext carries CurIndex, the line-number table + insertion-order pcs
slice (NAI-209-D-LINENUMBER-ORDER-SLICE), and the jump table. Helpers port
TS BaseScriptWriter static methods.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `OpcodeWriter` interface + `WriteScript` dispatch

**Files:**
- Create: `pkg/pack/compiler/writer/base_writer.go`
- Test: `pkg/pack/compiler/writer/base_writer_test.go`

**TS source:** `BaseScriptWriter.ts` L25-148.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/writer/base_writer_test.go`:

```go
// pkg/pack/compiler/writer/base_writer_test.go
package writer_test

import (
	"reflect"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// recorderWriter records the method names called on it in order, so the
// dispatch tests can pin (block-enter, per-instruction call, advance-after)
// ordering.
type recorderWriter struct {
	calls    []string
	curIndex []int
	ctx      *writer.BaseContext
}

func (r *recorderWriter) note(name string) {
	r.calls = append(r.calls, name)
	r.curIndex = append(r.curIndex, r.ctx.CurIndex)
}

func (r *recorderWriter) EnterBlock(b *codegen.Block)                                 { r.note("EnterBlock") }
func (r *recorderWriter) WritePushConstantInt(int32)                                  { r.note("WritePushConstantInt") }
func (r *recorderWriter) WritePushConstantString(string)                              { r.note("WritePushConstantString") }
func (r *recorderWriter) WritePushConstantLong(int64)                                 { r.note("WritePushConstantLong") }
func (r *recorderWriter) WritePushConstantSymbol(symbol.Symbol)                       { r.note("WritePushConstantSymbol") }
func (r *recorderWriter) WritePushLocalVar(*symbol.LocalVariableSymbol)               { r.note("WritePushLocalVar") }
func (r *recorderWriter) WritePopLocalVar(*symbol.LocalVariableSymbol)                { r.note("WritePopLocalVar") }
func (r *recorderWriter) WritePushVar(*symbol.BasicSymbol, bool)                      { r.note("WritePushVar") }
func (r *recorderWriter) WritePopVar(*symbol.BasicSymbol, bool)                       { r.note("WritePopVar") }
func (r *recorderWriter) WriteDefineArray(*symbol.LocalVariableSymbol)                { r.note("WriteDefineArray") }
func (r *recorderWriter) WriteSwitch(*codegen.SwitchTable)                            { r.note("WriteSwitch") }
func (r *recorderWriter) WriteBranch(codegen.Opcode, *codegen.Label)                  { r.note("WriteBranch") }
func (r *recorderWriter) WriteJoinString(int)                                         { r.note("WriteJoinString") }
func (r *recorderWriter) WriteDiscard(typ.BaseVarType)                                { r.note("WriteDiscard") }
func (r *recorderWriter) WriteJump(symbol.Symbol)                                     { r.note("WriteJump") }
func (r *recorderWriter) WriteGosub(symbol.Symbol)                                    { r.note("WriteGosub") }
func (r *recorderWriter) WriteCommand(symbol.Symbol)                                  { r.note("WriteCommand") }
func (r *recorderWriter) WriteReturn()                                                { r.note("WriteReturn") }
func (r *recorderWriter) WriteMath(codegen.Opcode)                                    { r.note("WriteMath") }

// TestWriteScript_DispatchOrder pins the per-instruction dispatch + the
// CurIndex post-increment.
func TestWriteScript_DispatchOrder(t *testing.T) {
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)

	la := &codegen.Label{Name: "a"}
	lb := &codegen.Label{Name: "b"}
	ba := codegen.NewBlock(la)
	bb := codegen.NewBlock(lb)
	ba.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 42})
	ba.Add(codegen.Instruction{Opcode: codegen.Branch, Operand: lb})
	bb.Add(codegen.Instruction{Opcode: codegen.Return})
	script.Blocks = []*codegen.Block{ba, bb}

	ctx := writer.NewBaseContext(script)
	r := &recorderWriter{ctx: ctx}
	writer.WriteScript(r, ctx, script)

	wantCalls := []string{
		"EnterBlock",
		"WritePushConstantInt",
		"WriteBranch",
		"EnterBlock",
		"WriteReturn",
	}
	if !reflect.DeepEqual(r.calls, wantCalls) {
		t.Errorf("calls = %v\nwant %v", r.calls, wantCalls)
	}
	// CurIndex at the time each method ran (Enter sees pre-increment of its
	// block's first instruction):
	//   EnterBlock(a)            CurIndex=0
	//   WritePushConstantInt(42) CurIndex=0
	//   WriteBranch              CurIndex=1
	//   EnterBlock(b)            CurIndex=2
	//   WriteReturn              CurIndex=2
	wantIdx := []int{0, 0, 1, 2, 2}
	if !reflect.DeepEqual(r.curIndex, wantIdx) {
		t.Errorf("curIndex per call = %v\nwant %v", r.curIndex, wantIdx)
	}
	// Post-loop CurIndex equals total instruction count.
	if ctx.CurIndex != 3 {
		t.Errorf("post-loop CurIndex = %d, want 3", ctx.CurIndex)
	}
}

// TestDispatch_LineNumberPanics pins that the codegen author can't smuggle
// LineNumber instructions to the writer.
func TestDispatch_LineNumberPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("dispatch did not panic on LineNumber opcode")
		}
	}()
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "foo"}}
	script := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.LineNumber, Operand: 1})
	script.Blocks = []*codegen.Block{b}
	ctx := writer.NewBaseContext(script)
	writer.WriteScript(&recorderWriter{ctx: ctx}, ctx, script)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Expected: build failure (`WriteScript`, `OpcodeWriter` undefined).

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/writer/base_writer.go`:

```go
// pkg/pack/compiler/writer/base_writer.go
package writer

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// OpcodeWriter is the visitor surface for one binary writer. Each method
// corresponds to one TS BaseScriptWriter abstract protected method
// (BaseScriptWriter.ts L150-206). Implementations live in
// pkg/pack/compiler/runescript/binary_writer.go (and any future text writer).
//
// NAI-209-D-OPCODE-WRITER-INTERFACE: TS abstract-class virtual dispatch →
// Go interface + free-function WriteScript.
type OpcodeWriter interface {
	EnterBlock(block *codegen.Block)

	WritePushConstantInt(value int32)
	WritePushConstantString(value string)
	WritePushConstantLong(value int64)
	WritePushConstantSymbol(sym symbol.Symbol)
	WritePushLocalVar(sym *symbol.LocalVariableSymbol)
	WritePopLocalVar(sym *symbol.LocalVariableSymbol)
	WritePushVar(sym *symbol.BasicSymbol, dot bool)
	WritePopVar(sym *symbol.BasicSymbol, dot bool)
	WriteDefineArray(sym *symbol.LocalVariableSymbol)
	WriteSwitch(table *codegen.SwitchTable)
	WriteBranch(opcode codegen.Opcode, label *codegen.Label)
	WriteJoinString(count int)
	WriteDiscard(baseType typ.BaseVarType)
	WriteJump(sym symbol.Symbol)
	WriteGosub(sym symbol.Symbol)
	WriteCommand(sym symbol.Symbol)
	WriteReturn()
	WriteMath(opcode codegen.Opcode)
}

// WriteScript drives a script's block list through w, mirroring TS
// BaseScriptWriter.write (L25-39). CurIndex on ctx is incremented AFTER each
// per-opcode method returns — WriteBranch and WriteSwitch read CurIndex to
// compute relative jumps and depend on this ordering.
func WriteScript(w OpcodeWriter, ctx *BaseContext, script *codegen.RuneScript) {
	for _, block := range script.Blocks {
		w.EnterBlock(block)
		for _, ins := range block.Instructions {
			dispatch(w, ins)
			ctx.CurIndex++
		}
	}
}

// dispatch resolves one instruction to its matching writer method. Mirrors
// TS BaseScriptWriter.writeInstruction (L55-148) one-for-one.
func dispatch(w OpcodeWriter, ins codegen.Instruction) {
	switch ins.Opcode {
	case codegen.PushConstantInt:
		w.WritePushConstantInt(toInt32(ins.Operand))
	case codegen.PushConstantString:
		w.WritePushConstantString(ins.Operand.(string))
	case codegen.PushConstantLong:
		w.WritePushConstantLong(ins.Operand.(int64))
	case codegen.PushConstantSymbol:
		w.WritePushConstantSymbol(ins.Operand.(symbol.Symbol))
	case codegen.PushLocalVar:
		w.WritePushLocalVar(ins.Operand.(*symbol.LocalVariableSymbol))
	case codegen.PopLocalVar:
		w.WritePopLocalVar(ins.Operand.(*symbol.LocalVariableSymbol))
	case codegen.PushVar:
		w.WritePushVar(ins.Operand.(*symbol.BasicSymbol), false)
	case codegen.PushVar2:
		w.WritePushVar(ins.Operand.(*symbol.BasicSymbol), true)
	case codegen.PopVar:
		w.WritePopVar(ins.Operand.(*symbol.BasicSymbol), false)
	case codegen.PopVar2:
		w.WritePopVar(ins.Operand.(*symbol.BasicSymbol), true)
	case codegen.DefineArray:
		w.WriteDefineArray(ins.Operand.(*symbol.LocalVariableSymbol))
	case codegen.Switch:
		w.WriteSwitch(ins.Operand.(*codegen.SwitchTable))
	case codegen.Branch,
		codegen.BranchNot, codegen.BranchEquals,
		codegen.BranchLessThan, codegen.BranchGreaterThan,
		codegen.BranchLessThanOrEquals, codegen.BranchGreaterThanOrEquals,
		codegen.LongBranchNot, codegen.LongBranchEquals,
		codegen.LongBranchLessThan, codegen.LongBranchGreaterThan,
		codegen.LongBranchLessThanOrEquals, codegen.LongBranchGreaterThanOrEquals,
		codegen.ObjBranchNot, codegen.ObjBranchEquals:
		w.WriteBranch(ins.Opcode, ins.Operand.(*codegen.Label))
	case codegen.JoinString:
		w.WriteJoinString(toInt(ins.Operand))
	case codegen.Discard:
		w.WriteDiscard(ins.Operand.(typ.BaseVarType))
	case codegen.Gosub:
		w.WriteGosub(ins.Operand.(symbol.Symbol))
	case codegen.Jump:
		w.WriteJump(ins.Operand.(symbol.Symbol))
	case codegen.Command:
		w.WriteCommand(ins.Operand.(symbol.Symbol))
	case codegen.Return:
		w.WriteReturn()
	case codegen.Add, codegen.Sub, codegen.Multiply, codegen.Divide,
		codegen.Modulo, codegen.Or, codegen.And,
		codegen.LongAdd, codegen.LongSub, codegen.LongMultiply,
		codegen.LongDivide, codegen.LongModulo, codegen.LongOr, codegen.LongAnd:
		w.WriteMath(ins.Opcode)
	case codegen.LineNumber:
		panic("writer: LineNumber opcode should not exist at write time")
	default:
		panic("writer: unknown opcode " + ins.Opcode.Name)
	}
}

// toInt32 accepts the codegen-side untyped int operand for PushConstantInt.
// codegen emits Go int values; cast to int32 (binary writer encodes 4 bytes).
func toInt32(v any) int32 {
	switch x := v.(type) {
	case int:
		return int32(x)
	case int32:
		return x
	case int64:
		return int32(x)
	}
	panic("writer: PushConstantInt operand is not an int-like value")
}

// toInt accepts the codegen-side untyped int operand for JoinString count.
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	}
	panic("writer: JoinString count is not an int-like value")
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/writer/...
```

Expected: PASS (7 tests in the package now).

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/writer/base_writer.go pkg/pack/compiler/writer/base_writer_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/writer): NAI-209 T4 — OpcodeWriter + WriteScript dispatch

Visitor interface + free-function dispatch port (NAI-209-D-OPCODE-WRITER-INTERFACE).
CurIndex increments AFTER per-opcode method returns so WriteBranch/WriteSwitch
see the parity-correct value for `jump - curIndex - 1` arithmetic.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `BinaryScriptWriterContext`

**Files:**
- Create: `pkg/pack/compiler/runescript/binary_context.go`
- Test: `pkg/pack/compiler/runescript/binary_context_test.go`

**TS source:** `src/runescript/writer/BinaryScriptWriterContext.ts` (214 lines).

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/binary_context_test.go`:

```go
// pkg/pack/compiler/runescript/binary_context_test.go
package runescript_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

func newEmptyScript(t *testing.T) *codegen.RuneScript {
	t.Helper()
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName, AllowParameters: true, AllowReturns: true}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{
		Trigger:    procTrig,
		Name:       "foo",
		Parameters: typ.MetaUnit,
		Returns:    typ.MetaUnit,
	}}
	s := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "foo", nil)
	s.Blocks = []*codegen.Block{codegen.NewBlock(&codegen.Label{Name: "e"})}
	return s
}

// TestBinaryContext_InstructionLargeOperand pins that Instruction writes
// opcode (2 BE bytes) + operand (4 BE bytes) for a LargeOperand opcode.
func TestBinaryContext_InstructionLargeOperand(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.Instruction(writer.OpPushConstantInt, 42)

	want := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x2A} // opcode=0, operand=0x2A
	if got := ctx.InstructionBytesForTest(); !bytes.Equal(got, want) {
		t.Errorf("instruction bytes = %x, want %x", got, want)
	}
}

// TestBinaryContext_InstructionSmallOperand pins 2 BE bytes + 1 byte.
func TestBinaryContext_InstructionSmallOperand(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.Instruction(writer.OpReturn, 0)

	want := []byte{0x00, 0x15, 0x00} // opcode=21, operand=0
	if got := ctx.InstructionBytesForTest(); !bytes.Equal(got, want) {
		t.Errorf("instruction bytes = %x, want %x", got, want)
	}
}

// TestBinaryContext_InstructionString pins opcode (2 BE) + string-bytes +
// NUL terminator. TS uses charCodeAt & 0xff per byte.
func TestBinaryContext_InstructionString(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.InstructionString(writer.OpPushConstantString, "hi")

	want := []byte{0x00, 0x03, 'h', 'i', 0x00}
	if got := ctx.InstructionBytesForTest(); !bytes.Equal(got, want) {
		t.Errorf("instruction bytes = %x, want %x", got, want)
	}
}

// TestBinaryContext_InstructionRaw pins opcode + 1-byte operand for raw form.
func TestBinaryContext_InstructionRaw(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.InstructionRaw(0x1234, 0x7F)

	want := []byte{0x12, 0x34, 0x7F}
	if got := ctx.InstructionBytesForTest(); !bytes.Equal(got, want) {
		t.Errorf("instruction bytes = %x, want %x", got, want)
	}
}

// TestBinaryContext_SwitchPlaceholderBackpatch pins the random-access fix-up
// of the 2-byte placeholder at sizePos. Two cases → key-count of 2.
func TestBinaryContext_SwitchPlaceholderBackpatch(t *testing.T) {
	s := newEmptyScript(t)
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	ctx.Switch(0, func() int {
		ctx.SwitchCase(1, 10)
		ctx.SwitchCase(2, 20)
		return 2
	})

	sw := ctx.SwitchBytesForTest()
	// Layout: 2 BE bytes placeholder (now =2), then 2× (4 BE key, 4 BE jump).
	want := []byte{
		0x00, 0x02, // total key count
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x0A,
		0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x14,
	}
	if !bytes.Equal(sw, want) {
		t.Errorf("switch bytes = %x, want %x", sw, want)
	}
}

// TestBinaryContext_FinishHeaderLayout pins the header bytes emitted by
// Finish() for a 1-instruction script (PushConstantInt 42; Return).
//
// Layout (TS BinaryScriptWriterContext.finish L123-179):
//   fullName  null-terminated string
//   sourceName null-terminated string
//   lookupKey       int32 BE
//   debugproc-zero  uint8 (0 because trigger != DEBUGPROC)
//   lineNumberCount uint16 BE
//   instructionBuffer (variable)
//   instructionCount  int32 BE
//   intLocals uint16 BE
//   strLocals uint16 BE
//   intParams uint16 BE
//   strParams uint16 BE
//   switchTableCount  uint8
//   switchBuffer (variable)
//   switchEnd uint16 BE (= switchOffset + 1)
func TestBinaryContext_FinishHeaderLayout(t *testing.T) {
	s := newEmptyScript(t)
	s.Blocks[0].Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 42})
	s.Blocks[0].Add(codegen.Instruction{Opcode: codegen.Return})

	ctx := runescript.NewBinaryScriptWriterContext(s, 0x12345678)
	// Manually emit two instructions to populate ctx.
	ctx.Instruction(writer.OpPushConstantInt, 42)
	ctx.Instruction(writer.OpReturn, 0)

	buf := ctx.Finish()

	var off int
	// fullName: "[proc,foo]\x00"
	expFull := "[proc,foo]"
	if string(buf[off:off+len(expFull)]) != expFull || buf[off+len(expFull)] != 0 {
		t.Fatalf("fullName mismatch at offset 0: %q", buf[:len(expFull)+1])
	}
	off += len(expFull) + 1
	// sourceName: "smoke.rs2\x00"
	expSrc := "smoke.rs2"
	if string(buf[off:off+len(expSrc)]) != expSrc || buf[off+len(expSrc)] != 0 {
		t.Fatalf("sourceName mismatch at offset %d", off)
	}
	off += len(expSrc) + 1
	// lookupKey
	if got := int32(binary.BigEndian.Uint32(buf[off:])); got != 0x12345678 {
		t.Errorf("lookupKey = %#x, want %#x", uint32(got), uint32(0x12345678))
	}
	off += 4
	// debugproc-zero
	if buf[off] != 0 {
		t.Errorf("debugproc-zero = %#x, want 0", buf[off])
	}
	off++
	// lineNumberCount = 0 (no line info on synthesised instructions)
	if got := binary.BigEndian.Uint16(buf[off:]); got != 0 {
		t.Errorf("lineNumberCount = %d, want 0", got)
	}
	off += 2
	// instructionBuffer: opcode 0x0000 + operand 0x0000002A (push 42)
	//                  + opcode 0x0015 + operand 0x00 (return)
	wantIns := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x2A, 0x00, 0x15, 0x00}
	if !bytes.Equal(buf[off:off+len(wantIns)], wantIns) {
		t.Errorf("instructionBuffer = %x, want %x", buf[off:off+len(wantIns)], wantIns)
	}
	off += len(wantIns)
	// instructionCount = 2
	if got := int32(binary.BigEndian.Uint32(buf[off:])); got != 2 {
		t.Errorf("instructionCount = %d, want 2", got)
	}
	off += 4
	// intLocals=0, strLocals=0, intParams=0, strParams=0
	for i, label := range []string{"intLocals", "strLocals", "intParams", "strParams"} {
		if got := binary.BigEndian.Uint16(buf[off:]); got != 0 {
			t.Errorf("%s (idx %d) = %d, want 0", label, i, got)
		}
		off += 2
	}
	// switchTableCount = 0
	if buf[off] != 0 {
		t.Errorf("switchTableCount = %d, want 0", buf[off])
	}
	off++
	// switchEnd = switchOffset+1 = 0+1 = 1
	if got := binary.BigEndian.Uint16(buf[off:]); got != 1 {
		t.Errorf("switchEnd = %d, want 1", got)
	}
}

// TestBinaryContext_FinishDebugproc pins the debugproc-parameter-codes
// path: trigger.Identifier == "debugproc" → emit param-count byte + param
// type-code bytes (signed int8). NAI-209-D-DEBUGPROC-TRIGGER-STRING-CHECK.
func TestBinaryContext_FinishDebugproc(t *testing.T) {
	debugproc := &trigger.TriggerType{ID: 1, Identifier: "debugproc", SubjectMode: trigger.ModeName, AllowParameters: true}
	tup, _ := typ.NewTupleType(typ.PrimitiveInt, typ.PrimitiveString)
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{
		Trigger:    debugproc,
		Name:       "x",
		Parameters: tup,
		Returns:    typ.MetaUnit,
	}}
	s := codegen.NewRuneScript("smoke.rs2", ss, debugproc, "x", nil)
	s.Blocks = []*codegen.Block{codegen.NewBlock(&codegen.Label{Name: "e"})}
	ctx := runescript.NewBinaryScriptWriterContext(s, 0)
	buf := ctx.Finish()

	// fullName "[debugproc,x]\x00" + sourceName "smoke.rs2\x00" + 4 BE lookupKey
	off := len("[debugproc,x]") + 1 + len("smoke.rs2") + 1 + 4
	if buf[off] != 2 {
		t.Errorf("debugproc paramCount = %d, want 2", buf[off])
	}
	off++
	// param[0] is PrimitiveInt: code='i' (0x69); param[1] is PrimitiveString: code='s' (0x73)
	if buf[off] != 'i' {
		t.Errorf("param[0] code = %#x, want 'i'", buf[off])
	}
	if buf[off+1] != 's' {
		t.Errorf("param[1] code = %#x, want 's'", buf[off+1])
	}
}
```

The test references two test-only inspector methods on the context — these are intentional, mirroring the `*ForTest` convention from `[[test_export_underscore_test_visibility]]`. Add them to the production file.

- [ ] **Step 2: Run tests to verify they fail**

Expected: build failure (`NewBinaryScriptWriterContext`, methods undefined).

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/runescript/binary_context.go`:

```go
// pkg/pack/compiler/runescript/binary_context.go
package runescript

import (
	"encoding/binary"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// binaryCtxInitialCapacity matches TS BinaryScriptWriterContext.INITIAL_CAPACITY
// (the average OSRS script size rounded to the next power of two).
const binaryCtxInitialCapacity = 512

// BinaryScriptWriterContext is the concrete writer context for the binary
// on-disk script format. Embeds *writer.BaseContext for CurIndex + line/jump
// tables. Mirrors TS BinaryScriptWriterContext.ts.
type BinaryScriptWriterContext struct {
	*writer.BaseContext

	LookupKey         int32
	instructionBuffer []byte
	switchBuffer      []byte
	instructionCount  int
	instructionOffset int
	switchOffset      int
}

// NewBinaryScriptWriterContext allocates instruction + switch buffers at the
// initial capacity. lookupKey is computed by the caller (binary_writer.go)
// before construction.
func NewBinaryScriptWriterContext(script *codegen.RuneScript, lookupKey int32) *BinaryScriptWriterContext {
	return &BinaryScriptWriterContext{
		BaseContext:       writer.NewBaseContext(script),
		LookupKey:         lookupKey,
		instructionBuffer: make([]byte, binaryCtxInitialCapacity),
		switchBuffer:      make([]byte, binaryCtxInitialCapacity),
	}
}

func (c *BinaryScriptWriterContext) ensureInstruction(extra int) {
	for c.instructionOffset+extra > len(c.instructionBuffer) {
		c.instructionBuffer = append(c.instructionBuffer, make([]byte, len(c.instructionBuffer))...)
	}
}

func (c *BinaryScriptWriterContext) ensureSwitch(extra int) {
	for c.switchOffset+extra > len(c.switchBuffer) {
		c.switchBuffer = append(c.switchBuffer, make([]byte, len(c.switchBuffer))...)
	}
}

// Instruction emits opcode.ID (2 BE bytes) + operand (4 BE bytes if
// opcode.LargeOperand, else 1 byte). Mirrors TS L63-78.
func (c *BinaryScriptWriterContext) Instruction(op *writer.ServerScriptOpcode, operand int32) {
	c.instructionCount++
	size := 4
	if op.LargeOperand {
		size = 6
	}
	c.ensureInstruction(size)
	binary.BigEndian.PutUint16(c.instructionBuffer[c.instructionOffset:], op.ID)
	c.instructionOffset += 2
	if op.LargeOperand {
		binary.BigEndian.PutUint32(c.instructionBuffer[c.instructionOffset:], uint32(operand))
		c.instructionOffset += 4
	} else {
		c.instructionBuffer[c.instructionOffset] = byte(operand & 0xff)
		c.instructionOffset++
	}
}

// InstructionRaw emits a 2-BE-byte opcode + 1-byte operand (no
// LargeOperand-aware sizing). Used by WriteCommand which carries the
// numeric command-id directly. Mirrors TS L80-89.
func (c *BinaryScriptWriterContext) InstructionRaw(opcode, operand int) {
	c.instructionCount++
	c.ensureInstruction(3)
	binary.BigEndian.PutUint16(c.instructionBuffer[c.instructionOffset:], uint16(opcode))
	c.instructionOffset += 2
	c.instructionBuffer[c.instructionOffset] = byte(operand & 0xff)
	c.instructionOffset++
}

// InstructionString emits a 2-BE-byte opcode followed by the operand string,
// each byte being `charCodeAt(i) & 0xff` (TS), terminated by 0x00.
// Mirrors TS L91-101.
func (c *BinaryScriptWriterContext) InstructionString(op *writer.ServerScriptOpcode, operand string) {
	c.instructionCount++
	c.ensureInstruction(2 + len(operand) + 1)
	binary.BigEndian.PutUint16(c.instructionBuffer[c.instructionOffset:], op.ID)
	c.instructionOffset += 2
	for i := 0; i < len(operand); i++ {
		c.instructionBuffer[c.instructionOffset] = operand[i] & 0xff
		c.instructionOffset++
	}
	c.instructionBuffer[c.instructionOffset] = 0
	c.instructionOffset++
}

// Switch emits OpSwitch (with the switch table ID as operand) into the
// instruction stream, then sets up a placeholder for the total key count in
// the switch buffer, invokes block (which writes SwitchCase entries), and
// finally back-patches the placeholder. Mirrors TS L103-112.
func (c *BinaryScriptWriterContext) Switch(id int, block func() int) {
	c.Instruction(writer.OpSwitch, int32(id))
	sizePos := c.switchOffset
	c.ensureSwitch(2)
	c.switchOffset += 2
	total := block()
	binary.BigEndian.PutUint16(c.switchBuffer[sizePos:], uint16(total))
}

// SwitchCase emits one (key int32 BE, jump int32 BE) pair. Mirrors TS L114-121.
func (c *BinaryScriptWriterContext) SwitchCase(key, jump int32) {
	c.ensureSwitch(8)
	binary.BigEndian.PutUint32(c.switchBuffer[c.switchOffset:], uint32(key))
	c.switchOffset += 4
	binary.BigEndian.PutUint32(c.switchBuffer[c.switchOffset:], uint32(jump))
	c.switchOffset += 4
}

// Finish assembles the final binary blob in TS BinaryScriptWriterContext.finish
// header layout (L123-179). Returns a freshly allocated []byte.
//
// NAI-209-D-DEBUGPROC-TRIGGER-STRING-CHECK: the DEBUGPROC trigger singleton
// is not yet ported to goscape; comparison uses `Trigger.Identifier ==
// "debugproc"` for parity.
func (c *BinaryScriptWriterContext) Finish() []byte {
	script := c.Script
	var buf []byte

	buf = appendNULString(buf, script.FullName)
	buf = appendNULString(buf, script.SourceName)
	buf = binary.BigEndian.AppendUint32(buf, uint32(c.LookupKey))

	if script.Trigger != nil && script.Trigger.Identifier == "debugproc" {
		params := paramCodes(script)
		buf = append(buf, byte(len(params)))
		for _, code := range params {
			buf = append(buf, byte(int8(code)))
		}
	} else {
		buf = append(buf, 0)
	}

	buf = binary.BigEndian.AppendUint16(buf, uint16(len(c.LineNumberPCs)))
	for _, pc := range c.LineNumberPCs {
		line := c.LineNumberTable[pc]
		buf = binary.BigEndian.AppendUint32(buf, uint32(int32(pc)))
		buf = binary.BigEndian.AppendUint32(buf, uint32(int32(line)))
	}

	buf = append(buf, c.instructionBuffer[:c.instructionOffset]...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(int32(c.instructionCount)))

	locals := script.Locals
	buf = binary.BigEndian.AppendUint16(buf, uint16(writer.GetLocalCount(locals, typ.BaseVarInteger)))
	buf = binary.BigEndian.AppendUint16(buf, uint16(writer.GetLocalCount(locals, typ.BaseVarString)))
	buf = binary.BigEndian.AppendUint16(buf, uint16(writer.GetParameterCount(locals, typ.BaseVarInteger)))
	buf = binary.BigEndian.AppendUint16(buf, uint16(writer.GetParameterCount(locals, typ.BaseVarString)))

	buf = append(buf, byte(len(script.SwitchTables)))
	buf = append(buf, c.switchBuffer[:c.switchOffset]...)
	buf = binary.BigEndian.AppendUint16(buf, uint16(c.switchOffset+1))

	return buf
}

// paramCodes returns the per-parameter char-code list for a DEBUGPROC's
// Parameters field. Mirrors TS BinaryScriptWriterContext.finish L136-141
// (TupleType.toList + code?.charCodeAt(0) ?? -1).
func paramCodes(script *codegen.RuneScript) []int {
	ss, ok := script.Symbol.(*symbol.ServerScriptSymbol)
	if !ok || ss.Parameters == nil {
		return nil
	}
	params := typ.TupleToList(ss.Parameters)
	out := make([]int, len(params))
	for i, p := range params {
		code, ok := p.Code()
		if !ok || code == "" {
			out[i] = -1
		} else {
			out[i] = int(code[0])
		}
	}
	return out
}

// appendNULString appends each byte of s (low 8 bits, per TS L207-213) plus
// a trailing 0x00.
func appendNULString(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		buf = append(buf, s[i]&0xff)
	}
	return append(buf, 0)
}

// InstructionBytesForTest returns a copy of the instruction buffer's used
// prefix. Test-only inspector; consumers in production code use Finish().
// Naming follows the `*ForTest` convention from
// pkg/pack/compiler/cfg.SetLandBytesForTest (NAI-151).
func (c *BinaryScriptWriterContext) InstructionBytesForTest() []byte {
	out := make([]byte, c.instructionOffset)
	copy(out, c.instructionBuffer)
	return out
}

// SwitchBytesForTest returns a copy of the switch buffer's used prefix.
func (c *BinaryScriptWriterContext) SwitchBytesForTest() []byte {
	out := make([]byte, c.switchOffset)
	copy(out, c.switchBuffer)
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/...
```

Expected: PASS (mapper tests + 6 binary-context tests + pre-existing ServerPointerChecker tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/runescript/binary_context.go pkg/pack/compiler/runescript/binary_context_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-209 T5 — BinaryScriptWriterContext

Two raw []byte buffers with explicit offsets for instruction + switch
streams; placeholder back-patch for switch key-count; Finish() assembles
the TS-parity header layout. NAI-209-D-DEBUGPROC-TRIGGER-STRING-CHECK
compares trigger.Identifier == "debugproc" since the trigger singleton is
not yet ported.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `BinaryScriptWriter` per-opcode methods

**Files:**
- Create: `pkg/pack/compiler/runescript/binary_writer.go`
- Test: `pkg/pack/compiler/runescript/binary_writer_test.go`

**TS source:** `BinaryScriptWriter.ts` L91-362 (all `protected override write*`).

Scope note: T6 implements every per-opcode method and the writer's `Write(script)` entry-point. `generateLookupKey` lands in T7 (and gets its own test file) so this task can be reviewed without the lookup-key arithmetic being on the critical path.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/binary_writer_test.go`:

```go
// pkg/pack/compiler/runescript/binary_writer_test.go
package runescript_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// stubIdProvider returns a fixed ID for every symbol.
type stubIdProvider struct{ id int }

func (s stubIdProvider) Get(symbol.Symbol) int { return s.id }

// recOutput captures the data passed to OutputScript so tests can re-decode.
type recOutput struct {
	script *codegen.RuneScript
	data   []byte
}

func (r *recOutput) OutputScript(s *codegen.RuneScript, data []byte) {
	r.script = s
	d := make([]byte, len(data))
	copy(d, data)
	r.data = d
}

func minimalScript(t *testing.T, name string, blocks ...*codegen.Block) *codegen.RuneScript {
	t.Helper()
	procTrig := &trigger.TriggerType{ID: 5, Identifier: "proc", SubjectMode: trigger.ModeName, AllowParameters: true, AllowReturns: true}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{
		Trigger:    procTrig,
		Name:       name,
		Parameters: typ.MetaUnit,
		Returns:    typ.MetaUnit,
	}}
	s := codegen.NewRuneScript("smoke.rs2", ss, procTrig, name, nil)
	if len(blocks) == 0 {
		blocks = []*codegen.Block{codegen.NewBlock(&codegen.Label{Name: "e"})}
	}
	s.Blocks = blocks
	return s
}

// runOne drives a 1-block script with one Instruction through the writer and
// returns the instructionBuffer prefix the writer emitted, *plus* the
// recOutput-captured full buffer for full-stack tests.
func runOne(t *testing.T, idp writer.IdProvider, ins codegen.Instruction) []byte {
	t.Helper()
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(ins)
	s := minimalScript(t, "x", b)
	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(idp, out)
	w.Write(s)
	// The first 2 bytes of the instruction stream within the Finish() blob
	// are the opcode; the test pulls them out by re-decoding the header.
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1
	off += 2 // lineNumberCount
	return out.data[off:]
}

// TestWritePushConstantInt pins the opcode + operand bytes.
func TestWritePushConstantInt(t *testing.T) {
	got := runOne(t, stubIdProvider{}, codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: int(42)})
	// Want: opcode 0x0000, operand 0x0000002A, then more trailing bytes.
	want := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x2A}
	if !bytes.Equal(got[:len(want)], want) {
		t.Errorf("got %x, want %x", got[:len(want)], want)
	}
}

// TestWritePushConstantString pins opcode + bytes + NUL.
func TestWritePushConstantString(t *testing.T) {
	got := runOne(t, stubIdProvider{}, codegen.Instruction{Opcode: codegen.PushConstantString, Operand: "hi"})
	want := []byte{0x00, 0x03, 'h', 'i', 0x00}
	if !bytes.Equal(got[:len(want)], want) {
		t.Errorf("got %x, want %x", got[:len(want)], want)
	}
}

// TestWritePushLocalVar_IntParam pins PushIntLocal (id=33) + variable-id.
func TestWritePushLocalVar_IntParam(t *testing.T) {
	intParam := &symbol.LocalVariableSymbol{Name: "$p", Type: typ.PrimitiveInt}
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.PushLocalVar, Operand: intParam})
	s := minimalScript(t, "x", b)
	s.Locals.Parameters = []*symbol.LocalVariableSymbol{intParam}
	s.Locals.All = []*symbol.LocalVariableSymbol{intParam}

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	want := []byte{0x00, 0x21, 0x00, 0x00, 0x00, 0x00} // op 33, id 0
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWritePushVar_VarpDot pins the dot-bit encoding (+1<<16).
func TestWritePushVar_VarpDot(t *testing.T) {
	v := &symbol.BasicSymbol{Name: "vp", Type: typ.NewVarPlayerType(typ.PrimitiveInt)}
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.PushVar2, Operand: v}) // PushVar2 → dot=true
	s := minimalScript(t, "x", b)

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{id: 7}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	// opcode 1 (PushVarp) + operand (7 + (1<<16)) = 0x00010007
	want := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x07}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWriteDefineArray pins (id<<16) | charCode encoding.
func TestWriteDefineArray(t *testing.T) {
	arr, _ := typ.NewArrayType(typ.PrimitiveInt) // PrimitiveInt code = 'i'
	local := &symbol.LocalVariableSymbol{Name: "$a", Type: arr}
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.DefineArray, Operand: local})
	s := minimalScript(t, "x", b)
	s.Locals.All = []*symbol.LocalVariableSymbol{local}

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	// op DefineArray (44, 0x2C), operand (0<<16)|'i' = 0x69
	want := []byte{0x00, 0x2C, 0x00, 0x00, 0x00, 0x69}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWriteBranch pins the `jumpLocation - curIndex - 1` arithmetic.
// Layout: block A has [PushConstantInt, Branch→B], block B has [Return].
// Branch is at index 1, jumps to block-B (jump=2); operand = 2-1-1 = 0.
func TestWriteBranch(t *testing.T) {
	la := &codegen.Label{Name: "a"}
	lb := &codegen.Label{Name: "b"}
	ba := codegen.NewBlock(la)
	bb := codegen.NewBlock(lb)
	ba.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 0})
	ba.Add(codegen.Instruction{Opcode: codegen.Branch, Operand: lb})
	bb.Add(codegen.Instruction{Opcode: codegen.Return})
	s := minimalScript(t, "x", ba, bb)

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	off += 6 // PushConstantInt 0
	// Branch opcode 0x0006, operand 0x00000000 (forward by zero)
	want := []byte{0x00, 0x06, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWriteCommand pins InstructionRaw with secondary=1 for dot-prefixed names.
func TestWriteCommand(t *testing.T) {
	cmd := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{
		Trigger: trigger.CommandTrigger,
		Name:    ".mes",
	}}
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.Command, Operand: cmd})
	s := minimalScript(t, "x", b)

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{id: 0x1234}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	// opcode 0x1234, operand 0x01 (secondary)
	want := []byte{0x12, 0x34, 0x01}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWritePushConstantLong_Panics pins the TS throw → Go panic
// (NAI-209-D-PUSHLONG-PANIC).
func TestWritePushConstantLong_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("WritePushConstantLong did not panic")
		}
	}()
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.PushConstantLong, Operand: int64(7)})
	s := minimalScript(t, "x", b)

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
}

// TestWriteSwitch_OneCase pins the OpSwitch instruction in the instruction
// stream + the corresponding key/jump pair in the switch buffer.
//
// Block A: [Switch table0]; Block B: [Return]. Table[0].cases = {key 1 → B}.
// jump-to-block-B = pc 1; CurIndex when Switch runs = 0; operand = 1-0-1 = 0.
func TestWriteSwitch_OneCase(t *testing.T) {
	la := &codegen.Label{Name: "a"}
	lb := &codegen.Label{Name: "b"}
	ba := codegen.NewBlock(la)
	bb := codegen.NewBlock(lb)

	// Build script with a switch table referencing block B.
	procTrig := &trigger.TriggerType{ID: 5, Identifier: "proc", SubjectMode: trigger.ModeName}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	s := codegen.NewRuneScript("smoke.rs2", ss, procTrig, "x", nil)
	st := s.GenerateSwitchTable()
	st.AddCase(codegen.SwitchCase{Label: lb, Keys: []any{int(1)}})
	ba.Add(codegen.Instruction{Opcode: codegen.Switch, Operand: st})
	bb.Add(codegen.Instruction{Opcode: codegen.Return})
	s.Blocks = []*codegen.Block{ba, bb}

	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)

	// instructionBuffer offset
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	// Switch opcode (24, 0x18) + operand = table id 0
	wantIns := []byte{0x00, 0x18, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(out.data[off:off+len(wantIns)], wantIns) {
		t.Errorf("instruction stream = %x, want %x", out.data[off:off+len(wantIns)], wantIns)
	}

	// switchBuffer is appended after: instructionBuffer (9 bytes) + instructionCount (4) + locals (8) + switchTableCount (1).
	off += 6 + 3 // Switch + Return → instructionBuffer total 9 bytes
	off += 4     // instructionCount
	off += 8     // four uint16 locals
	off += 1     // switchTableCount
	// switch buffer: total-key-count 2 BE bytes (1) + 4 BE key (1) + 4 BE jump (0)
	wantSw := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(out.data[off:off+len(wantSw)], wantSw) {
		t.Errorf("switch buffer = %x, want %x", out.data[off:off+len(wantSw)], wantSw)
	}
}

// TestWriteMath pins one math arm + the operand=0 convention.
func TestWriteMath(t *testing.T) {
	b := codegen.NewBlock(&codegen.Label{Name: "e"})
	b.Add(codegen.Instruction{Opcode: codegen.Add})
	s := minimalScript(t, "x", b)
	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1 + 4 + 1 + 2
	// Add opcode 4600 = 0x11F8, operand 1 byte = 0x00
	want := []byte{0x11, 0xF8, 0x00}
	if !bytes.Equal(out.data[off:off+len(want)], want) {
		t.Errorf("got %x, want %x", out.data[off:off+len(want)], want)
	}
}

// TestWrite_LookupKeyZero pins that Write() invokes lookup-key generation
// (full computation tested in T7) and stores the result in the blob.
func TestWrite_LookupKeyZero(t *testing.T) {
	s := minimalScript(t, "x")
	s.Blocks[0].Add(codegen.Instruction{Opcode: codegen.Return})
	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1
	// Trigger SubjectMode = ModeName → lookupKey = -1 (TS L65). T7 pins
	// this; here we just assert the header reflects what generateLookupKey
	// computed.
	if got := int32(binary.BigEndian.Uint32(out.data[off:])); got != -1 {
		t.Errorf("lookupKey = %d, want -1 (ModeName)", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Expected: build failure (`NewBinaryScriptWriter`, `BinaryOutput`, etc. undefined).

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/runescript/binary_writer.go`:

```go
// pkg/pack/compiler/runescript/binary_writer.go
package runescript

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// BinaryOutput is the abstract hook for the file-output sink. TS uses an
// abstract `outputScript(script, data)` method on BinaryScriptWriter; goscape
// hoists it to an interface field so concrete sinks (binary file, JagFile,
// etc.) can be injected without subclassing.
//
// NAI-209-D-BINARYOUTPUT-INTERFACE.
type BinaryOutput interface {
	OutputScript(script *codegen.RuneScript, data []byte)
}

// BinaryScriptWriter implements writer.OpcodeWriter and produces a binary
// script blob for each RuneScript handed to Write(). Mirrors TS
// BinaryScriptWriter.ts.
type BinaryScriptWriter struct {
	IdProvider writer.IdProvider
	Output     BinaryOutput

	ctx *BinaryScriptWriterContext // set per Write() call
}

// Compile-time check that BinaryScriptWriter satisfies OpcodeWriter.
var _ writer.OpcodeWriter = (*BinaryScriptWriter)(nil)

// NewBinaryScriptWriter constructs a writer bound to idp + output.
func NewBinaryScriptWriter(idp writer.IdProvider, output BinaryOutput) *BinaryScriptWriter {
	return &BinaryScriptWriter{IdProvider: idp, Output: output}
}

// Write is the public entry-point: compute the lookup key, drive the
// dispatch through writer.WriteScript, then call Finish + emit via Output.
func (b *BinaryScriptWriter) Write(script *codegen.RuneScript) {
	b.ctx = NewBinaryScriptWriterContext(script, b.generateLookupKey(script))
	writer.WriteScript(b, b.ctx.BaseContext, script)
	data := b.ctx.Finish()
	if b.Output != nil {
		b.Output.OutputScript(script, data)
	}
}

// =============================================================================
// OpcodeWriter implementation
// =============================================================================

func (b *BinaryScriptWriter) EnterBlock(*codegen.Block) {} // TS L87-89 NO-OP

func (b *BinaryScriptWriter) WritePushConstantInt(value int32) {
	b.ctx.Instruction(writer.OpPushConstantInt, value)
}

func (b *BinaryScriptWriter) WritePushConstantString(value string) {
	b.ctx.InstructionString(writer.OpPushConstantString, value)
}

func (b *BinaryScriptWriter) WritePushConstantLong(int64) {
	panic("BinaryScriptWriter: PushConstantLong not supported") // NAI-209-D-PUSHLONG-PANIC
}

// WritePushConstantSymbol handles three arms (TS L103-120):
//   - LocalVariableSymbol → variable id
//   - BasicSymbol whose Type is MetaType.Type → take the inner type's char code
//   - any other → IdProvider.Get
func (b *BinaryScriptWriter) WritePushConstantSymbol(sym symbol.Symbol) {
	var id int32
	switch s := sym.(type) {
	case *symbol.LocalVariableSymbol:
		id = int32(writer.GetVariableId(b.ctx.Script.Locals, s))
	case *symbol.BasicSymbol:
		if inner, ok := typ.IsMetaWrapping(s.Type); ok {
			code, hasCode := inner.Code()
			if !hasCode || code == "" {
				panic(fmt.Sprintf("BinaryScriptWriter: MetaType.Type inner %v has no char code", inner))
			}
			id = int32(code[0])
		} else {
			id = int32(b.IdProvider.Get(sym))
		}
	default:
		id = int32(b.IdProvider.Get(sym))
	}
	b.ctx.Instruction(writer.OpPushConstantInt, id)
}

func (b *BinaryScriptWriter) WritePushLocalVar(s *symbol.LocalVariableSymbol) {
	id := int32(writer.GetVariableId(b.ctx.Script.Locals, s))
	op := localVarOpcode(s, true)
	b.ctx.Instruction(op, id)
}

func (b *BinaryScriptWriter) WritePopLocalVar(s *symbol.LocalVariableSymbol) {
	id := int32(writer.GetVariableId(b.ctx.Script.Locals, s))
	op := localVarOpcode(s, false)
	b.ctx.Instruction(op, id)
}

func localVarOpcode(s *symbol.LocalVariableSymbol, push bool) *writer.ServerScriptOpcode {
	if _, isArr := s.Type.(*typ.ArrayType); isArr {
		if push {
			return writer.OpPushArrayInt
		}
		return writer.OpPopArrayInt
	}
	bt, _ := s.Type.BaseType()
	switch bt {
	case typ.BaseVarString:
		if push {
			return writer.OpPushStringLocal
		}
		return writer.OpPopStringLocal
	case typ.BaseVarInteger:
		if push {
			return writer.OpPushIntLocal
		}
		return writer.OpPopIntLocal
	}
	panic(fmt.Sprintf("BinaryScriptWriter: unsupported local variable type %v", s.Type))
}

func (b *BinaryScriptWriter) WritePushVar(s *symbol.BasicSymbol, dot bool) {
	id := b.IdProvider.Get(s)
	op := varOpcode(s, true)
	operand := int32(id)
	if dot {
		operand += 1 << 16
	}
	b.ctx.Instruction(op, operand)
}

func (b *BinaryScriptWriter) WritePopVar(s *symbol.BasicSymbol, dot bool) {
	id := b.IdProvider.Get(s)
	op := varOpcode(s, false)
	operand := int32(id)
	if dot {
		operand += 1 << 16
	}
	b.ctx.Instruction(op, operand)
}

func varOpcode(s *symbol.BasicSymbol, push bool) *writer.ServerScriptOpcode {
	switch s.Type.(type) {
	case *typ.VarPlayerType:
		if push {
			return writer.OpPushVarp
		}
		return writer.OpPopVarp
	case *typ.VarBitType:
		if push {
			return writer.OpPushVarbit
		}
		return writer.OpPopVarbit
	case *typ.VarNpcType:
		if push {
			return writer.OpPushVarn
		}
		return writer.OpPopVarn
	case *typ.VarSharedType:
		if push {
			return writer.OpPushVars
		}
		return writer.OpPopVars
	}
	panic(fmt.Sprintf("BinaryScriptWriter: unsupported variable type %v", s.Type))
}

func (b *BinaryScriptWriter) WriteDefineArray(s *symbol.LocalVariableSymbol) {
	id := writer.GetVariableId(b.ctx.Script.Locals, s)
	arr, ok := s.Type.(*typ.ArrayType)
	if !ok {
		panic(fmt.Sprintf("BinaryScriptWriter: WriteDefineArray on non-ArrayType %v", s.Type))
	}
	code, hasCode := arr.Inner().Code()
	if !hasCode || code == "" {
		panic(fmt.Sprintf("BinaryScriptWriter: ArrayType inner %v has no char code", arr.Inner()))
	}
	operand := int32((id << 16) | int(code[0]))
	b.ctx.Instruction(writer.OpDefineArray, operand)
}

func (b *BinaryScriptWriter) WriteSwitch(table *codegen.SwitchTable) {
	b.ctx.Switch(table.ID, func() int {
		total := 0
		for _, c := range table.Cases() {
			jumpLocation, ok := b.ctx.JumpTable[c.Label]
			if !ok {
				panic(fmt.Sprintf("BinaryScriptWriter: label %q not in jump table", c.Label.Name))
			}
			relativeJump := int32(jumpLocation - b.ctx.CurIndex - 1)
			for _, key := range c.Keys {
				b.ctx.SwitchCase(b.resolveSwitchKey(key), relativeJump)
				total++
			}
		}
		return total
	})
}

// resolveSwitchKey ports TS BinaryScriptWriter.findCaseKeyValue (L239-249).
// Numeric keys flow through; symbol keys are mapped via IdProvider.
func (b *BinaryScriptWriter) resolveSwitchKey(key any) int32 {
	switch v := key.(type) {
	case int:
		return int32(v)
	case int32:
		return v
	case symbol.Symbol:
		return int32(b.IdProvider.Get(v))
	}
	panic(fmt.Sprintf("BinaryScriptWriter: unsupported switch key %T", key))
}

func (b *BinaryScriptWriter) WriteBranch(opcode codegen.Opcode, label *codegen.Label) {
	op, ok := branchOpcode(opcode)
	if !ok {
		panic(fmt.Sprintf("BinaryScriptWriter: unsupported branch opcode %s", opcode.Name))
	}
	jumpLocation, ok := b.ctx.JumpTable[label]
	if !ok {
		panic(fmt.Sprintf("BinaryScriptWriter: label %q not in jump table", label.Name))
	}
	operand := int32(jumpLocation - b.ctx.CurIndex - 1)
	b.ctx.Instruction(op, operand)
}

// branchOpcode maps a codegen.Opcode to the matching ServerScriptOpcode.
// Returns (nil, false) for non-branch inputs. Mirrors TS L251-278.
func branchOpcode(opcode codegen.Opcode) (*writer.ServerScriptOpcode, bool) {
	switch opcode {
	case codegen.Branch:
		return writer.OpBranch, true
	case codegen.BranchNot:
		return writer.OpBranchNot, true
	case codegen.BranchEquals:
		return writer.OpBranchEquals, true
	case codegen.BranchLessThan:
		return writer.OpBranchLessThan, true
	case codegen.BranchGreaterThan:
		return writer.OpBranchGreaterThan, true
	case codegen.BranchLessThanOrEquals:
		return writer.OpBranchLessThanOrEquals, true
	case codegen.BranchGreaterThanOrEquals:
		return writer.OpBranchGreaterThanOrEquals, true
	}
	return nil, false
}

func (b *BinaryScriptWriter) WriteJoinString(count int) {
	b.ctx.Instruction(writer.OpJoinString, int32(count))
}

func (b *BinaryScriptWriter) WriteDiscard(baseType typ.BaseVarType) {
	switch baseType {
	case typ.BaseVarInteger:
		b.ctx.Instruction(writer.OpPopIntDiscard, 0)
	case typ.BaseVarString:
		b.ctx.Instruction(writer.OpPopStringDiscard, 0)
	default:
		panic(fmt.Sprintf("BinaryScriptWriter: unsupported discard base type %v", baseType))
	}
}

func (b *BinaryScriptWriter) WriteGosub(sym symbol.Symbol) {
	id := int32(b.IdProvider.Get(sym))
	b.ctx.Instruction(writer.OpGosubWithParams, id)
}

func (b *BinaryScriptWriter) WriteJump(sym symbol.Symbol) {
	id := int32(b.IdProvider.Get(sym))
	b.ctx.Instruction(writer.OpJumpWithParams, id)
}

// WriteCommand mirrors TS L319-326: emit InstructionRaw with the command's
// IdProvider id as the opcode and 1/0 secondary flag based on the leading dot.
func (b *BinaryScriptWriter) WriteCommand(sym symbol.Symbol) {
	op := b.IdProvider.Get(sym)
	if op == -1 {
		panic(fmt.Sprintf("BinaryScriptWriter: missing opcode id for command %q", sym.SymbolName()))
	}
	secondary := 0
	if strings.HasPrefix(sym.SymbolName(), ".") {
		secondary = 1
	}
	b.ctx.InstructionRaw(op, secondary)
}

func (b *BinaryScriptWriter) WriteReturn() {
	b.ctx.Instruction(writer.OpReturn, 0)
}

func (b *BinaryScriptWriter) WriteMath(opcode codegen.Opcode) {
	op := mathOpcode(opcode)
	if op == nil {
		panic(fmt.Sprintf("BinaryScriptWriter: unsupported math opcode %s", opcode.Name))
	}
	b.ctx.Instruction(op, 0)
}

func mathOpcode(opcode codegen.Opcode) *writer.ServerScriptOpcode {
	switch opcode {
	case codegen.Add:
		return writer.OpAdd
	case codegen.Sub:
		return writer.OpSub
	case codegen.Multiply:
		return writer.OpMultiply
	case codegen.Divide:
		return writer.OpDivide
	case codegen.Modulo:
		return writer.OpModulo
	case codegen.Or:
		return writer.OpOr
	case codegen.And:
		return writer.OpAnd
	}
	return nil
}

// generateLookupKey is a stub here so T6 can land independently; full
// implementation arrives in T7. For now: returns -1 for ModeName (the
// most-common case in tests), 0 otherwise. T7 replaces this body.
func (b *BinaryScriptWriter) generateLookupKey(script *codegen.RuneScript) int32 {
	if _, ok := script.Trigger.SubjectMode.(interface{ subjectMode() }); ok && script.Trigger.SubjectMode == trigger.ModeName {
		return -1
	}
	return 0
}
```

The `generateLookupKey` stub above mirrors only the `SubjectMode.Name → -1` arm; T7 grows it to the full TS implementation. The stub is sufficient for the T6 tests, which only exercise `ModeName` scripts.

- [ ] **Step 4: Run tests to verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/...
```

Expected: PASS (all prior tests + 10 new binary-writer tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/runescript/binary_writer.go pkg/pack/compiler/runescript/binary_writer_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-209 T6 — BinaryScriptWriter per-opcode methods

Ports TS BinaryScriptWriter.ts L91-362 — all 18 per-opcode Write methods plus
the Write() entry-point. NAI-209-D-BINARYOUTPUT-INTERFACE; NAI-209-D-PUSHLONG-PANIC.
generateLookupKey is a stub (ModeName → -1, else 0); T7 grows the full impl.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `generateLookupKey` + `trigger.IsNameMode`

**Files:**
- Modify: `pkg/pack/compiler/trigger/subjectmode.go` (add `IsNameMode`)
- Modify: `pkg/pack/compiler/runescript/binary_writer.go` (replace `generateLookupKey` stub)
- Test: `pkg/pack/compiler/runescript/binary_writer_lookup_test.go`

**TS source:** `BinaryScriptWriter.ts` L58-85.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/binary_writer_lookup_test.go`:

```go
// pkg/pack/compiler/runescript/binary_writer_lookup_test.go
package runescript_test

import (
	"encoding/binary"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// runAndExtractLookupKey runs Write() on a 1-block / 1-instruction script
// and extracts the lookupKey from the Finish() blob. `stubIdProvider` from
// binary_writer_test.go is used directly — both files are package
// runescript_test so they share test fixtures.
func runAndExtractLookupKey(t *testing.T, s *codegen.RuneScript, idp stubIdProvider) int32 {
	t.Helper()
	if len(s.Blocks) == 0 {
		s.Blocks = []*codegen.Block{codegen.NewBlock(&codegen.Label{Name: "e"})}
	}
	s.Blocks[0].Add(codegen.Instruction{Opcode: codegen.Return})
	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(idp, out)
	w.Write(s)
	off := len(s.FullName) + 1 + len(s.SourceName) + 1
	return int32(binary.BigEndian.Uint32(out.data[off:]))
}

// TestLookupKey_NameMode pins SubjectMode.Name → -1 (TS L65).
func TestLookupKey_NameMode(t *testing.T) {
	tr := &trigger.TriggerType{ID: 5, Identifier: "proc", SubjectMode: trigger.ModeName, AllowParameters: true, AllowReturns: true}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", nil)
	if got := runAndExtractLookupKey(t, s, stubIdProvider{}); got != -1 {
		t.Errorf("lookupKey = %d, want -1", got)
	}
}

// TestLookupKey_TypeMode_NonCategory pins category=false subject → key + (2<<8) + (subjectId<<10).
// subject = BasicSymbol "weapon1"; stub IdProvider returns 17.
// Trigger.ID = 5 → key = 5 + (2<<8) + (17<<10) = 5 + 512 + 17408 = 17925.
func TestLookupKey_TypeMode_NonCategory(t *testing.T) {
	tm := trigger.NewModeType(typ.PrimitiveInt, false, false)
	tr := &trigger.TriggerType{ID: 5, Identifier: "opheld1", SubjectMode: tm}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	subject := &symbol.BasicSymbol{Name: "weapon1", Type: typ.PrimitiveInt}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", subject)

	if got := runAndExtractLookupKey(t, s, stubIdProvider{id: 17}); got != 17925 {
		t.Errorf("lookupKey = %d, want 17925", got)
	}
}

// TestLookupKey_TypeMode_Category pins category=true → typeMarker = 1.
// Trigger.ID = 5; subject id 17; key = 5 + (1<<8) + (17<<10) = 5 + 256 + 17408 = 17669.
func TestLookupKey_TypeMode_Category(t *testing.T) {
	tm := trigger.NewModeType(typ.PrimitiveInt, true, false)
	tr := &trigger.TriggerType{ID: 5, Identifier: "opheld1", SubjectMode: tm}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	subject := &symbol.BasicSymbol{Name: "weapons", Type: typ.PrimitiveInt}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", subject)

	if got := runAndExtractLookupKey(t, s, stubIdProvider{id: 17}); got != 17669 {
		t.Errorf("lookupKey = %d, want 17669", got)
	}
}

// TestLookupKey_MapzonePath pins the strconv.Atoi(subject.SymbolName()) path
// for MAPZONE primitives.
func TestLookupKey_MapzonePath(t *testing.T) {
	tm := trigger.NewModeType(typ.PrimitiveMapzone, false, false)
	tr := &trigger.TriggerType{ID: 5, Identifier: "zone_enter", SubjectMode: tm}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	subject := &symbol.BasicSymbol{Name: "12345", Type: typ.PrimitiveMapzone}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", subject)

	// Expected: 5 + (2<<8) + (12345<<10) = 5 + 512 + 12641280 = 12641797.
	if got := runAndExtractLookupKey(t, s, stubIdProvider{}); got != 12641797 {
		t.Errorf("lookupKey = %d, want 12641797", got)
	}
}

// TestLookupKey_MapzoneInvalidPanics pins NAI-209-D-MAPZONE-COORD-PARSE-PANIC.
func TestLookupKey_MapzoneInvalidPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("invalid MAPZONE name did not panic")
		}
	}()
	tm := trigger.NewModeType(typ.PrimitiveMapzone, false, false)
	tr := &trigger.TriggerType{ID: 5, Identifier: "zone_enter", SubjectMode: tm}
	ss := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "x", Parameters: typ.MetaUnit, Returns: typ.MetaUnit}}
	subject := &symbol.BasicSymbol{Name: "not-a-number", Type: typ.PrimitiveMapzone}
	s := codegen.NewRuneScript("smoke.rs2", ss, tr, "x", subject)
	runAndExtractLookupKey(t, s, stubIdProvider{})
}

// TestIsNameMode pins the new trigger helper.
func TestIsNameMode(t *testing.T) {
	if !trigger.IsNameMode(trigger.ModeName) {
		t.Errorf("IsNameMode(ModeName) = false, want true")
	}
	if trigger.IsNameMode(trigger.ModeNone) {
		t.Errorf("IsNameMode(ModeNone) = true, want false")
	}
	if trigger.IsNameMode(trigger.NewModeType(typ.PrimitiveInt, false, false)) {
		t.Errorf("IsNameMode(TypeMode) = true, want false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Expected: `IsNameMode` undefined; lookup-key tests fail because the stub doesn't compute the correct values.

- [ ] **Step 3: Add the `IsNameMode` helper**

Append to `pkg/pack/compiler/trigger/subjectmode.go`:

```go
// IsNameMode returns true when m is the ModeName singleton. Mirrors TS
// `subjectMode === SubjectMode.Name` reference-equality check.
func IsNameMode(m SubjectMode) bool {
	_, ok := m.(modeNameT)
	return ok
}
```

- [ ] **Step 4: Replace `generateLookupKey` with the full implementation**

In `pkg/pack/compiler/runescript/binary_writer.go`, remove the stub and add (also add `strconv` to imports):

```go
// generateLookupKey ports TS BinaryScriptWriter.generateLookupKey (L58-85).
//
// Three arms:
//   - SubjectMode.Name           → -1
//   - SubjectMode.Type + subject → trigger.ID + (typeMarker<<8) + (subjectId<<10)
//   - otherwise (Mode.None)      → trigger.ID
//
// typeMarker = 1 when TypeMode.Category is true, 2 otherwise. subjectId comes
// from strconv.Atoi(subject.SymbolName()) for MAPZONE/COORD primitives, or
// from IdProvider.Get for any other type.
//
// NAI-209-D-MAPZONE-COORD-PARSE-PANIC: invalid Atoi panics (TS would silently
// produce NaN-corrupted output).
func (b *BinaryScriptWriter) generateLookupKey(script *codegen.RuneScript) int32 {
	if trigger.IsNameMode(script.Trigger.SubjectMode) {
		return -1
	}
	key := int32(script.Trigger.ID)
	tm, ok := trigger.IsTypeMode(script.Trigger.SubjectMode)
	if !ok || script.SubjectReference == nil {
		return key
	}
	subject, ok := script.SubjectReference.(symbol.Symbol)
	if !ok {
		panic(fmt.Sprintf("BinaryScriptWriter: SubjectReference %T is not a symbol.Symbol", script.SubjectReference))
	}
	subjectId := b.resolveSubjectId(subject)
	var typeMarker int32 = 2
	if tm.Category {
		typeMarker = 1
	}
	key += (typeMarker << 8) | (subjectId << 10)
	return key
}

func (b *BinaryScriptWriter) resolveSubjectId(subject symbol.Symbol) int32 {
	if bs, ok := subject.(*symbol.BasicSymbol); ok {
		switch bs.Type {
		case typ.PrimitiveMapzone, typ.PrimitiveCoord:
			n, err := strconv.Atoi(subject.SymbolName())
			if err != nil {
				panic(fmt.Sprintf("BinaryScriptWriter: invalid MAPZONE/COORD subject %q: %v",
					subject.SymbolName(), err))
			}
			return int32(n)
		}
	}
	return int32(b.IdProvider.Get(subject))
}
```

Add `"strconv"` to the imports in `binary_writer.go`.

- [ ] **Step 5: Run tests to verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/trigger/... ./pkg/pack/compiler/runescript/...
```

Expected: PASS (existing trigger tests + 6 new lookup tests + all T6 tests still green).

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/trigger/subjectmode.go pkg/pack/compiler/runescript/binary_writer.go pkg/pack/compiler/runescript/binary_writer_lookup_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-209 T7 — generateLookupKey + trigger.IsNameMode

Replaces T6 stub with full lookup-key arithmetic: name-mode → -1, type-mode
→ trigger.ID + (typeMarker<<8) + (subjectId<<10) where typeMarker = 1 for
category subjects and 2 otherwise. MAPZONE/COORD subjects resolve via
strconv.Atoi (NAI-209-D-MAPZONE-COORD-PARSE-PANIC). Adds one-line
trigger.IsNameMode helper paralleling existing IsTypeMode.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Pipeline smoke extension

**Files:**
- Modify: `pkg/pack/compiler/codegen/smoke_test.go`

The existing `TestPipeline_FullSlice` runs parse → register → typecheck → codegen → pointer-check (T8 of NAI-208). T8 of NAI-209 adds the writer step + a byte-pin on the first script's blob.

- [ ] **Step 1: Add new test**

Append to `pkg/pack/compiler/codegen/smoke_test.go`:

```go
// TestPipeline_FullSliceWithWriter extends the codegen+pointer pipeline by
// running BinaryScriptWriter on the two-script source and pins the writer
// output for the `helper` script. Mirrors the existing TestPipeline_FullSlice
// setup but adds a writer hop after PointerChecker.
func TestPipeline_FullSliceWithWriter(t *testing.T) {
	src := `[proc,helper](int $n)(int)
return(calc($n * 2));

[proc,foo](int $x, string $name)(int)
def_int $result = 0;
if ($x > 0) {
  $result = ~helper($x);
} else {
  $result = 0;
}
while ($result < 100) {
  $result = calc($result + 1);
}
return($result);
`

	tm := typ.NewTypeManager()
	for _, p := range typ.PrimitiveAll {
		_ = tm.RegisterByRepresentation(p)
	}
	tm.AddTypeChecker(func(left, right typ.Type) bool { return left == right })

	trm := trigger.NewTriggerManager()
	proc := &trigger.TriggerType{
		ID:              0,
		Identifier:      "proc",
		SubjectMode:     trigger.ModeName,
		AllowParameters: true,
		AllowReturns:    true,
	}
	_ = trm.RegisterTrigger(proc)

	root := symbol.NewSymbolTable(nil)
	d := &diagnostics.Diagnostics{}
	dyn := map[string]semantics.DynamicCommandHandler{}
	command.RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{}, func(name string, h semantics.DynamicCommandHandler) {
		dyn[name] = h
	})

	p := parser.NewScriptFileParser(src, "smoke.rs2")
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("parse failed")
	}
	sr := semantics.NewScriptRegistration(tm, trm, root, d, semantics.StrictFeatureLevel{})
	sr.Visit(sf)
	tc := semantics.NewTypeChecker(tm, trm, root, dyn, d, semantics.StrictFeatureLevel{})
	tc.Visit(sf)
	cg := codegen.NewCodeGenerator(root, dyn, d)
	cg.Visit(sf)
	pc := cfg.NewPointerChecker(d, cg.Scripts(), map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})
	pc.Run()
	if d.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}

	mapper := runescript.NewSymbolMapper(d)
	mapper.PutScript(0, "[proc,helper]")
	mapper.PutScript(1, "[proc,foo]")

	rec := &smokeRec{}
	w := runescript.NewBinaryScriptWriter(mapper, rec)
	for _, s := range cg.Scripts() {
		w.Write(s)
	}

	if len(rec.scripts) != 2 {
		t.Fatalf("writer emitted %d scripts, want 2", len(rec.scripts))
	}

	// Byte-pin: the first 17 bytes of the `helper` blob are deterministic.
	// fullName "[proc,helper]\x00" (14) + sourceName "smoke.rs2\x00" (10) +
	// lookupKey 0xFFFFFFFF (4) + debugproc-zero 0x00 (1) = 29 bytes.
	helperBlob := rec.scripts[0]
	if got := string(helperBlob[:13]); got != "[proc,helper]" {
		t.Errorf("helper.fullName prefix = %q, want %q", got, "[proc,helper]")
	}
	if helperBlob[13] != 0 {
		t.Errorf("helper.fullName terminator missing")
	}
	if got := string(helperBlob[14:23]); got != "smoke.rs2" {
		t.Errorf("helper.sourceName = %q, want %q", got, "smoke.rs2")
	}
	if helperBlob[23] != 0 {
		t.Errorf("helper.sourceName terminator missing")
	}
	// lookupKey = -1 (SubjectMode.Name) at offset 24.
	if got := int32(binary.BigEndian.Uint32(helperBlob[24:28])); got != -1 {
		t.Errorf("helper.lookupKey = %d, want -1", got)
	}
	if helperBlob[28] != 0 {
		t.Errorf("helper debugproc-zero = %d, want 0", helperBlob[28])
	}
}

type smokeRec struct {
	scripts [][]byte
}

func (r *smokeRec) OutputScript(s *codegen.RuneScript, data []byte) {
	d := make([]byte, len(data))
	copy(d, data)
	r.scripts = append(r.scripts, d)
}
```

You will need new imports — add `"encoding/binary"` and `"github.com/zsrv/goscape/pkg/pack/compiler/runescript"` to the file's imports.

- [ ] **Step 2: Run the test**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/codegen/... -run TestPipeline_FullSliceWithWriter -v
```

Expected: PASS.

- [ ] **Step 3: Confirm no regressions**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all packages PASS (NAI-208 smoke still green, plus the new writer test).

- [ ] **Step 4: Commit**

```bash
git add pkg/pack/compiler/codegen/smoke_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/codegen): NAI-209 T8 — wire BinaryScriptWriter into pipeline smoke

Extends the existing 2-script pipeline test with a writer hop that produces
byte-identical script blobs through a recorder BinaryOutput. Pins the header
prefix (fullName, sourceName, lookupKey, debugproc-zero) on the helper script.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Doc-pass + deviation pins + close commit

**Files:**
- Create: `pkg/pack/compiler/runescript/nai209_deviation_pins_test.go`
- Modify: in-file doc comments that reference NAI-209 deferrals retired by this slice (none expected — NAI-208 didn't defer to NAI-209)

- [ ] **Step 1: Create deviation-pin tests**

Create `pkg/pack/compiler/runescript/nai209_deviation_pins_test.go`:

```go
// pkg/pack/compiler/runescript/nai209_deviation_pins_test.go
package runescript_test

import (
	"os"
	"strings"
	"testing"
)

// nai209DeviationTags is the canonical inventory of NAI-209's deviation tags.
// Each tag must appear in at least one production-source doc comment so a
// future reader can grep from the test to the rationale.
var nai209DeviationTags = []struct {
	tag      string
	rationale string // human-readable hint for failure messages
}{
	{"NAI-209-D-BYTEPACKET-DEFER", "BytePacket deferred to NAI-210"},
	{"NAI-209-D-SYMMAPPER-DIAG-CTOR", "SymbolMapper takes diagnostics in ctor"},
	{"NAI-209-D-PUSHLONG-PANIC", "WritePushConstantLong panics on TS throw parity"},
	{"NAI-209-D-MAPZONE-COORD-PARSE-PANIC", "Atoi failure panics, not silent NaN"},
	{"NAI-209-D-OPCODE-WRITER-INTERFACE", "TS abstract class -> Go interface"},
	{"NAI-209-D-BINARYOUTPUT-INTERFACE", "TS abstract outputScript -> Go interface"},
	{"NAI-209-D-LINENUMBER-ORDER-SLICE", "Map iteration randomized -> parallel slice"},
	{"NAI-209-D-DEBUGPROC-TRIGGER-STRING-CHECK", "DEBUGPROC trigger singleton not yet ported"},
}

// productionFiles lists every NAI-209 production source. The pin test
// reads each file and asserts each tag appears in at least one file's
// content. Mirrors [[pin_test_self_trigger_production_doc]].
var productionFiles = []string{
	"binary_context.go",
	"binary_writer.go",
	"symbol_mapper.go",
	"../writer/base_writer.go",
	"../writer/base_context.go",
	"../writer/helpers.go",
}

func TestNAI209_DeviationTags_PinnedToProductionDocs(t *testing.T) {
	combined := readAll(t, productionFiles)
	for _, c := range nai209DeviationTags {
		if !strings.Contains(combined, c.tag) {
			t.Errorf("deviation tag %s missing from production docs (%s)", c.tag, c.rationale)
		}
	}
}

func readAll(t *testing.T, files []string) string {
	t.Helper()
	var sb strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}
```

- [ ] **Step 2: Run the test and inspect failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/... -run TestNAI209_DeviationTags -v
```

Expected: any missing tag fails this test. For each failure, add the tag to the appropriate production doc comment (find the most semantically relevant location). Re-run until green.

- [ ] **Step 3: Audit adjacent paragraphs for count drift**

Per `[[adjacent_doc_paragraph_count_drift]]`, search for paragraphs in NAI-207/208 docs that enumerate writer-related deferrals and verify they don't now contradict NAI-209's existence:

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache git grep -nE 'NAI-208 will|NAI-209 will|writer pass|writer slice|deferred to (NAI-208|NAI-209)' pkg/pack/compiler/
```

For any "NAI-209 will" comment that this slice fulfills, either delete the stale forward-reference or update it to indicate the work has landed. Do not retroactively add NAI-209 references to NAI-207/208 deviation tags.

- [ ] **Step 4: Run the full suite to confirm green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all packages PASS.

- [ ] **Step 5: Close commit**

```bash
git add pkg/pack/compiler/runescript/nai209_deviation_pins_test.go
# include any doc-comment fixups from Step 3
git commit --no-gpg-sign -m "$(cat <<'EOF'
close(compiler/writer): NAI-209 T9 — deviation pins + close

NAI-209 (compiler slice 6b of 6) ships the binary writer pipeline:
ServerScriptOpcode + SymbolMapper + BaseContext/helpers + OpcodeWriter dispatch
+ BinaryScriptWriterContext + BinaryScriptWriter + generateLookupKey. The
pipeline smoke now byte-pins writer output through a recorder BinaryOutput.

8 deviation tags pinned to production docs by nai209_deviation_pins_test.go.

Closes memory: nai209_close

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final verification

After T9 commits, run:

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/compiler/runescript/... ./pkg/pack/compiler/writer/...
```

Both must be green before declaring NAI-209 closed. Save close memory (`nai209_close`) and resume prompt for NAI-210.
