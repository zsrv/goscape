# NAI-205 — Type system + symbol table + ScriptRegistration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hand-port the RuneScriptTS compiler-infrastructure layer (diagnostics + type + symbol + trigger packages, ~1300 TS LOC) plus the 457-LOC `ScriptRegistration` first pass. Lift the seven AST fields ScriptRegistration writes (Script.TriggerType/Symbol/Block/ParameterType/ReturnType/SubjectReference + Parameter.Symbol) out of NAI-204-D-AST-NO-TYPE-FIELDS deferral. TypeChecking + the other nine AST fields stay deferred to NAI-206.

**Architecture:** Five new packages under `pkg/pack/compiler/`: `diagnostics`, `type`, `symbol`, `trigger`, `semantics`. The `ast → symbol/trigger/type` would-be-cycle is broken via four marker interfaces in `ast/symbol_refs.go` (`SymbolRef`, `TriggerRef`, `TypeRef`, `SymbolTableRef`), each with one exported zero-arg marker method that concrete types implement structurally — `ast` does not import the consumer packages. ScriptRegistration walks an `*ast.ScriptFile` via Go type-switch (no visitor pattern; continues NAI-204-D-AST-NO-VISITOR), writes seven fields on visited nodes, registers `ServerScriptSymbol`s into a passed-in root `SymbolTable`, and reports diagnostics via a passed-in `*diagnostics.Diagnostics`.

**Tech Stack:** Go 1.26+, stdlib `testing` only. No new external deps. Existing deps: `pkg/pack/compiler/ast` (NAI-204), `pkg/pack/compiler/lexer` (NAI-203, for `NodeSourceLocation`). TS source-of-truth: `/home/owner/Code/github.com/LostCityRS/RuneScriptTS` at HEAD `b8c338801fbb72d294ff9576a58925a8d3f6de47` (same pin as NAI-203/204).

**Spec:** [`docs/superpowers/specs/2026-05-15-nai-205-typesys-symbol-script-registration-design.md`](../specs/2026-05-15-nai-205-typesys-symbol-script-registration-design.md)

**Authoritative task numbering:** T1, T2, T3, T4, T5, T6, T7, T8, T9, T10, T11, T12, T13, T14. Plan-author and controller MUST pass these task numbers verbatim to every implementer dispatch — see [[plan_code_block_t_number_drift]].

---

## Per-task conventions

- All `go` commands invoke as: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go <args>`.
- All commits use `git commit --no-gpg-sign`.
- Each commit body ends with the trailer block:
  ```
  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  ```
- TDD discipline: write failing test → run to confirm failure → write minimal impl → run to confirm pass → commit.
- Package import paths:
  - `github.com/zsrv/goscape/pkg/pack/compiler/ast` (existing, NAI-204; modified in T1 + T8)
  - `github.com/zsrv/goscape/pkg/pack/compiler/lexer` (existing, NAI-203)
  - `github.com/zsrv/goscape/pkg/pack/compiler/diagnostics` (T7)
  - `github.com/zsrv/goscape/pkg/pack/compiler/type` (T2/T3/T4)
  - `github.com/zsrv/goscape/pkg/pack/compiler/symbol` (T6)
  - `github.com/zsrv/goscape/pkg/pack/compiler/trigger` (T5)
  - `github.com/zsrv/goscape/pkg/pack/compiler/semantics` (T9–T13)
- All file paths are absolute or rooted at `/home/owner/Code/github.com/zsrv/goscape/`.
- **TS reference pin:** `/home/owner/Code/github.com/LostCityRS/RuneScriptTS` at HEAD `b8c338801fbb72d294ff9576a58925a8d3f6de47`.
- **Pre-flight per dispatch** ([[controller_preflight.md]]):
  - Verify nothing already in `pkg/pack/compiler/` collides with the constant/symbol names the task introduces (grep case-insensitively per [[plan_constants_under_different_naming]]).
  - Verify the cited TS line numbers still match TS-HEAD (the pin is fixed; this is a sanity check).
  - Verify the prior task landed at the SHA the dispatch references (controller may consolidate consecutive task completions).

---

## Task summary

| T | Subject | Net new LOC | TS source |
|---|---|---|---|
| T1 | `ast/symbol_refs.go` — four marker interfaces (cyclic-import bridge) | ~30 + tests | n/a (Go idiom) |
| T2 | `type/` part A — BaseVarType, TypeOptions, Type interface, PrimitiveType | ~250 + tests | `BaseVarType.ts`, `TypeOptions.ts`, `Type.ts`, `PrimitiveType.ts` |
| T3 | `type/` part B — TupleType, MetaType, WrappedType, ArrayType, GameVarType | ~400 + tests | `TupleType.ts`, `MetaType.ts`, `wrapped/{Wrapped,Array,GameVar}Type.ts` |
| T4 | `type/` part C — TypeManager | ~200 + tests | `TypeManager.ts` |
| T5 | `trigger/` — SubjectMode, TriggerType, TriggerManager, CommandTrigger | ~250 + tests | `trigger/*.ts` (4 files) |
| T6 | `symbol/` — Symbol/ScriptSymbol/SymbolType/SymbolTable | ~350 + tests | `symbol/*.ts` (4 files) |
| T7 | `diagnostics/` — DiagnosticType/Diagnostic/Diagnostics/Handler/Messages | ~400 + tests | `diagnostics/*.ts` (5 files, minus BaseDiagnosticsHandler) |
| T8 | `ast/scriptfile.go` field additions + narrowed deviation tag + narrowed-tag pin | ~100 | n/a (NAI-205 obligation) |
| T9 | `semantics/` — StrictFeatureLevel + ScriptRegistration constructor/Visit/scoped-table | ~150 + tests | `StrictFeatureLevel.ts`, `ScriptRegistration.ts` L36-94 |
| T10 | `semantics/` — visitScript core (trigger lookup, star check, return type, symbol insert) | ~250 + tests | `ScriptRegistration.ts` L96-180 |
| T11 | `semantics/` — subject validation + tryParseMapZone/Zone + resolveSubjectSymbol | ~300 + tests | `ScriptRegistration.ts` L184-380 |
| T12 | `semantics/` — visitParameter + checkScriptParameters + checkScriptReturns | ~250 + tests | `ScriptRegistration.ts` L385-451 |
| T13 | `semantics/` — end-to-end smoke + 11 deviation-pin tests | ~250 | n/a |
| T14 | Final review + close commit | n/a | n/a |

Total projected: ~3100 LOC across production + tests.

---

## Task 1: `ast/symbol_refs.go` — marker interfaces for cyclic-import bridge

**Why first:** T2–T7 add `AsTypeRef()` / `AsSymbolRef()` / `AsTriggerRef()` / `AsSymbolTableRef()` methods on their concrete types. The interfaces must exist for those methods to have a contract to satisfy. The methods are zero-arg and the interfaces are otherwise empty — landing them upfront is a 30-LOC commit that unblocks every subsequent task.

**Files:**
- Create: `pkg/pack/compiler/ast/symbol_refs.go`
- Create: `pkg/pack/compiler/ast/symbol_refs_test.go`

### Step 1: Write the failing test

```go
// pkg/pack/compiler/ast/symbol_refs_test.go
package ast

import "testing"

// astRefStubSymbol satisfies SymbolRef via its AsSymbolRef method.
// This is the cross-package satisfaction pattern that NAI-205-D-AST-REF-INTERFACES
// relies on: the concrete impl can live outside the ast package because
// the marker method is exported.
type astRefStubSymbol struct{}

func (*astRefStubSymbol) AsSymbolRef() {}

type astRefStubTrigger struct{}

func (*astRefStubTrigger) AsTriggerRef() {}

type astRefStubType struct{}

func (*astRefStubType) AsTypeRef() {}

type astRefStubTable struct{}

func (*astRefStubTable) AsSymbolTableRef() {}

// TestSymbolRef_StructuralSatisfaction pins that a concrete pointer type with
// an exported AsSymbolRef() method satisfies SymbolRef without importing ast
// or sharing a package. This mirrors how *symbol.ServerScriptSymbol etc. will
// flow into ast.Script.Symbol fields after T8.
func TestSymbolRef_StructuralSatisfaction(t *testing.T) {
	var s SymbolRef = (*astRefStubSymbol)(nil)
	_ = s

	var tr TriggerRef = (*astRefStubTrigger)(nil)
	_ = tr

	var ty TypeRef = (*astRefStubType)(nil)
	_ = ty

	var tb SymbolTableRef = (*astRefStubTable)(nil)
	_ = tb
}

// TestSymbolRef_DocCommentTagged pins that symbol_refs.go carries the
// NAI-205-D-AST-REF-INTERFACES deviation tag. Sister NAI-204 pin tests use
// the readAllGoFiles helper in parser/; we duplicate the inline read here
// to avoid a parser → ast import (which would be backwards).
func TestSymbolRef_DocCommentTagged(t *testing.T) {
	b := mustReadFileForTest(t, "symbol_refs.go")
	if !contains(b, "NAI-205-D-AST-REF-INTERFACES") {
		t.Fatal("symbol_refs.go missing deviation tag NAI-205-D-AST-REF-INTERFACES")
	}
}
```

The helpers `mustReadFileForTest` and `contains` are inlined at the bottom of `symbol_refs_test.go`:

```go
import (
	"os"
	"strings"
)

func mustReadFileForTest(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
```

### Step 2: Run test to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/ast/...
```

Expected: build failure ("undefined: SymbolRef", "undefined: TriggerRef", "undefined: TypeRef", "undefined: SymbolTableRef").

### Step 3: Write the minimal implementation

```go
// pkg/pack/compiler/ast/symbol_refs.go

package ast

// NAI-205-D-AST-REF-INTERFACES: TS allows AST nodes to directly typed-reference
// Symbol/Trigger/Type/SymbolTable instances. Go's no-cyclic-import rule would
// force pkg/pack/compiler/ast to import symbol/trigger/type — but those
// packages reference ast.Node (for the AST-field consumer side). Resolution:
// four marker interfaces here, each with one exported zero-arg method.
// Concrete impls in symbol/trigger/type implement the method; structural
// typing satisfies the interface from those packages without importing ast.
// Consumers (semantics, future codegen) type-assert to the concrete type
// at the read site, e.g. `s.Symbol.(*symbol.ServerScriptSymbol)`.

// SymbolRef is satisfied by every concrete symbol type
// (ServerScriptSymbol, ClientScriptSymbol, BasicSymbol, LocalVariableSymbol).
// Stored on ast.Script.Symbol, ast.Script.SubjectReference, ast.Parameter.Symbol.
type SymbolRef interface {
	AsSymbolRef()
}

// TriggerRef is satisfied by *trigger.TriggerType.
// Stored on ast.Script.TriggerType.
type TriggerRef interface {
	AsTriggerRef()
}

// TypeRef is satisfied by every concrete type implementation
// (PrimitiveType, MetaType variants, TupleType, ArrayType, GameVarType variants).
// Stored on ast.Script.ParameterType and ast.Script.ReturnType.
type TypeRef interface {
	AsTypeRef()
}

// SymbolTableRef is satisfied by *symbol.SymbolTable.
// Stored on ast.Script.Block.
type SymbolTableRef interface {
	AsSymbolTableRef()
}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/ast/...
```

Expected: PASS (all existing ast tests still pass too).

### Step 5: Commit

```bash
git add pkg/pack/compiler/ast/symbol_refs.go pkg/pack/compiler/ast/symbol_refs_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/ast): NAI-205 T1 — cyclic-import marker interfaces

Add SymbolRef, TriggerRef, TypeRef, SymbolTableRef as the cyclic-import
bridge between ast and the NAI-205 symbol/trigger/type packages.
Each interface has one exported zero-arg marker method; concrete types
in symbol/trigger/type implement the method to satisfy the interface
structurally without importing ast. Resolves the ast → symbol/trigger
would-be-cycle that field typing on Script/Parameter would otherwise
create.

See deviation tag NAI-205-D-AST-REF-INTERFACES.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `pkg/pack/compiler/type/` part A — BaseVarType, TypeOptions, Type, PrimitiveType

**Spec ref:** §6.2.

**Files:**
- Create: `pkg/pack/compiler/type/basevartype.go`
- Create: `pkg/pack/compiler/type/options.go`
- Create: `pkg/pack/compiler/type/type.go`
- Create: `pkg/pack/compiler/type/primitive.go`
- Create: `pkg/pack/compiler/type/primitive_test.go`

**TS source-of-truth:**
- `src/compiler/type/BaseVarType.ts` (9 LOC, enum)
- `src/compiler/type/TypeOptions.ts` (48 LOC, interface + mutable class)
- `src/compiler/type/Type.ts` (43 LOC, abstract base)
- `src/compiler/type/PrimitiveType.ts` (62 LOC, seven singletons)

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/type/primitive_test.go
package typ

import "testing"

func TestBaseVarType_IntegerValues(t *testing.T) {
	cases := []struct {
		got  BaseVarType
		want int
	}{
		{BaseVarInteger, 0},
		{BaseVarLong, 1},
		{BaseVarString, 2},
	}
	for _, c := range cases {
		if int(c.got) != c.want {
			t.Fatalf("BaseVarType integer value: got %d, want %d", c.got, c.want)
		}
	}
}

func TestTypeOptions_ZeroValueAllPermissive(t *testing.T) {
	// Per spec §6.2 + TS TypeOptions.ts L31-37: MutableOptionsType ctor with no
	// args sets all four flags to true. Goscape mirrors via NewTypeOptions().
	o := NewTypeOptions()
	if !o.AllowSwitch || !o.AllowArray || !o.AllowDeclaration || !o.AllowParameter {
		t.Fatalf("NewTypeOptions defaults not all-true: %+v", o)
	}
}

func TestTypeOptions_BuilderOverride(t *testing.T) {
	o := NewTypeOptions(func(o *TypeOptions) {
		o.AllowSwitch = false
		o.AllowArray = false
	})
	if o.AllowSwitch || o.AllowArray {
		t.Fatalf("builder overrides not applied: %+v", o)
	}
	if !o.AllowDeclaration || !o.AllowParameter {
		t.Fatalf("builder reset unrelated fields: %+v", o)
	}
}

func TestPrimitiveType_INT_FieldShape(t *testing.T) {
	p := PrimitiveInt
	if got, want := p.Representation(), "int"; got != want {
		t.Fatalf("INT representation = %q, want %q", got, want)
	}
	code, ok := p.Code()
	if !ok || code != "i" {
		t.Fatalf("INT code = (%q, %v), want (\"i\", true)", code, ok)
	}
	base, ok := p.BaseType()
	if !ok || base != BaseVarInteger {
		t.Fatalf("INT baseType = (%d, %v), want (%d, true)", base, ok, BaseVarInteger)
	}
	if dv := p.DefaultValue(); dv != 0 {
		t.Fatalf("INT defaultValue = %v, want 0", dv)
	}
	o := p.Options()
	if !o.AllowSwitch || !o.AllowArray || !o.AllowDeclaration || !o.AllowParameter {
		t.Fatalf("INT options = %+v; want all-true", o)
	}
}

func TestPrimitiveType_STRING_NoArrayNoSwitch(t *testing.T) {
	// Per TS PrimitiveType.ts L46-49: STRING disables array+switch in its builder.
	p := PrimitiveString
	if got, want := p.Representation(), "string"; got != want {
		t.Fatalf("STRING representation = %q, want %q", got, want)
	}
	o := p.Options()
	if o.AllowSwitch || o.AllowArray {
		t.Fatalf("STRING options = %+v; want AllowSwitch=false, AllowArray=false", o)
	}
	if !o.AllowDeclaration || !o.AllowParameter {
		t.Fatalf("STRING decl/param options = %+v; want both true", o)
	}
}

func TestPrimitiveType_LONG_NoArrayNoSwitch(t *testing.T) {
	p := PrimitiveLong
	o := p.Options()
	if o.AllowSwitch || o.AllowArray {
		t.Fatalf("LONG options = %+v; want AllowSwitch=false, AllowArray=false", o)
	}
	base, _ := p.BaseType()
	if base != BaseVarLong {
		t.Fatalf("LONG baseType = %d, want %d", base, BaseVarLong)
	}
}

func TestPrimitiveType_AllList(t *testing.T) {
	// Per TS L57: ALL contains [INT, BOOLEAN, COORD, STRING, CHAR, LONG, MAPZONE]
	// in that order. Order matters because PrimitiveAll is used for round-trips.
	want := []string{"int", "boolean", "coord", "string", "char", "long", "mapzone"}
	if got := len(PrimitiveAll); got != len(want) {
		t.Fatalf("PrimitiveAll length = %d, want %d", got, len(want))
	}
	for i, p := range PrimitiveAll {
		if got := p.Representation(); got != want[i] {
			t.Fatalf("PrimitiveAll[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestPrimitiveByRepresentation_HitMiss(t *testing.T) {
	if got := PrimitiveByRepresentation("int"); got != PrimitiveInt {
		t.Fatalf("ByRepresentation(int) = %v, want PrimitiveInt", got)
	}
	if got := PrimitiveByRepresentation("nope"); got != nil {
		t.Fatalf("ByRepresentation(nope) = %v, want nil", got)
	}
}

func TestPrimitiveType_SatisfiesAstTypeRef(t *testing.T) {
	// NAI-205-D-AST-REF-INTERFACES contract: every concrete Type implementation
	// must satisfy ast.TypeRef. Pin by attempting interface assignment.
	var _ astTypeRef = PrimitiveInt
}

// astTypeRef mirrors ast.TypeRef without importing ast (avoid back-edge).
// Tests that try to satisfy this stub catch AsTypeRef() method drift.
type astTypeRef interface {
	AsTypeRef()
}
```

### Step 2: Run tests to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/...
```

Expected: build failure (package `typ` does not exist).

### Step 3: Write the implementation

**Note on package name:** `type` is a Go reserved word so the package's directory is `type/` but its declared name is `typ` (matches similar patterns in stdlib like `go/types`).

```go
// pkg/pack/compiler/type/basevartype.go
package typ

// BaseVarType enumerates the three low-level storage classes for any Type.
// Mirrors TS src/compiler/type/BaseVarType.ts.
type BaseVarType int

const (
	BaseVarInteger BaseVarType = 0
	BaseVarLong    BaseVarType = 1
	BaseVarString  BaseVarType = 2
)
```

```go
// pkg/pack/compiler/type/options.go
package typ

// TypeOptions controls which uses of a Type the compiler accepts.
// Mirrors TS src/compiler/type/TypeOptions.ts L4-37.
//
// NAI-205-D-TYPEOPTIONS-FLAT: TS exports both a readonly interface
// (TypeOptions) and a mutable subclass (MutableOptionsType). Goscape
// collapses to one mutable struct + a builder-fn convention since the
// readonly-vs-mutable distinction has no Go-idiomatic counterpart.
type TypeOptions struct {
	AllowSwitch      bool
	AllowArray       bool
	AllowDeclaration bool
	AllowParameter   bool
}

// NewTypeOptions returns a TypeOptions with all four flags true,
// optionally adjusted by builders called in order.
// Mirrors TS MutableOptionsType ctor + Object.assign(init).
func NewTypeOptions(builders ...func(*TypeOptions)) TypeOptions {
	o := TypeOptions{
		AllowSwitch:      true,
		AllowArray:       true,
		AllowDeclaration: true,
		AllowParameter:   true,
	}
	for _, b := range builders {
		b(&o)
	}
	return o
}
```

```go
// pkg/pack/compiler/type/type.go
package typ

// Type is the goscape port of TS abstract class Type.
//
// All five fields TS declares as `readonly` are exposed as zero-arg accessors:
//   - Representation() string  — always present
//   - Code() (string, bool)    — TS optional; second return is the presence bit
//   - BaseType() (BaseVarType, bool) — TS optional
//   - DefaultValue() any       — TS optional; returns nil when absent
//   - Options() TypeOptions    — always present
//
// Concrete implementations: PrimitiveType, TupleType, MetaType (and the
// MetaType-Wrapped/Script variants), ArrayType, VarPlayerType, VarBitType,
// VarNpcType, VarSharedType.
//
// Every concrete Type must also satisfy ast.TypeRef via an AsTypeRef() method
// (see NAI-205-D-AST-REF-INTERFACES in pkg/pack/compiler/ast/symbol_refs.go).
type Type interface {
	Representation() string
	Code() (string, bool)
	BaseType() (BaseVarType, bool)
	DefaultValue() any
	Options() TypeOptions

	// AsTypeRef satisfies ast.TypeRef. Embedding this method in the Type
	// interface ensures every concrete Type can be assigned to ast.TypeRef
	// without consumers needing to re-assert.
	AsTypeRef()
}
```

```go
// pkg/pack/compiler/type/primitive.go
package typ

// PrimitiveType represents one of the seven main RuneScript primitive types.
// Mirrors TS src/compiler/type/PrimitiveType.ts.
//
// All seven singletons are package-level vars; the constructor is unexported.
type PrimitiveType struct {
	rep      string
	code     string // "" means "no code" — see Code().
	codeOK   bool
	baseType BaseVarType
	dv       any
	options  TypeOptions
}

func newPrimitiveType(name, code string, base BaseVarType, dv any, builders ...func(*TypeOptions)) *PrimitiveType {
	return &PrimitiveType{
		rep:      lowerASCII(name),
		code:     code,
		codeOK:   code != "",
		baseType: base,
		dv:       dv,
		options:  NewTypeOptions(builders...),
	}
}

func (p *PrimitiveType) Representation() string         { return p.rep }
func (p *PrimitiveType) Code() (string, bool)           { return p.code, p.codeOK }
func (p *PrimitiveType) BaseType() (BaseVarType, bool)  { return p.baseType, true }
func (p *PrimitiveType) DefaultValue() any              { return p.dv }
func (p *PrimitiveType) Options() TypeOptions           { return p.options }
func (p *PrimitiveType) AsTypeRef()                     {}

// Singletons. Names + codes + baseType + defaultValue match TS L40-46.
var (
	PrimitiveInt     = newPrimitiveType("INT", "i", BaseVarInteger, 0)
	PrimitiveBoolean = newPrimitiveType("BOOLEAN", "1", BaseVarInteger, 0)
	PrimitiveCoord   = newPrimitiveType("COORD", "c", BaseVarInteger, -1)
	PrimitiveString  = newPrimitiveType("STRING", "s", BaseVarString, "", func(o *TypeOptions) {
		o.AllowArray = false
		o.AllowSwitch = false
	})
	PrimitiveChar    = newPrimitiveType("CHAR", "z", BaseVarInteger, -1)
	PrimitiveLong    = newPrimitiveType("LONG", "Ï", BaseVarLong, -1, func(o *TypeOptions) {
		o.AllowArray = false
		o.AllowSwitch = false
	})
	PrimitiveMapzone = newPrimitiveType("MAPZONE", "0", BaseVarInteger, -1)
)

// PrimitiveAll preserves TS L57 ordering. Used for round-trip / table-driven tests.
var PrimitiveAll = []*PrimitiveType{
	PrimitiveInt, PrimitiveBoolean, PrimitiveCoord, PrimitiveString,
	PrimitiveChar, PrimitiveLong, PrimitiveMapzone,
}

// PrimitiveByRepresentation returns the matching singleton or nil. Mirrors TS L59-61.
func PrimitiveByRepresentation(rep string) *PrimitiveType {
	for _, p := range PrimitiveAll {
		if p.rep == rep {
			return p
		}
	}
	return nil
}

// lowerASCII is the package-local lowercaser for type names. TS uses
// `.toLowerCase()` on type names — all primitive names are ASCII so a
// simple byte-loop suffices and avoids a Unicode dep.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/...
```

Expected: PASS — all primitive/options/baseVarType tests green.

### Step 5: Commit

```bash
git add pkg/pack/compiler/type/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/type): NAI-205 T2 — BaseVarType, TypeOptions, Type, PrimitiveType

Port the four leaf-most TS type-system files:
- BaseVarType (Integer/Long/String enum)
- TypeOptions (the four AllowX flags + builder ctor)
- Type interface (Representation/Code/BaseType/DefaultValue/Options,
  plus the ast.TypeRef marker AsTypeRef())
- PrimitiveType + the seven singletons (INT/BOOLEAN/COORD/STRING/CHAR/
  LONG/MAPZONE) + PrimitiveAll + PrimitiveByRepresentation.

NAI-205-D-TYPEOPTIONS-FLAT: TS readonly-interface + mutable-class duo
collapses to one mutable struct + builder fn.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `pkg/pack/compiler/type/` part B — Tuple/Meta/Wrapped/Array/GameVar

**Spec ref:** §6.2.

**Files:**
- Create: `pkg/pack/compiler/type/tuple.go`
- Create: `pkg/pack/compiler/type/meta.go`
- Create: `pkg/pack/compiler/type/wrapped.go`
- Create: `pkg/pack/compiler/type/array.go`
- Create: `pkg/pack/compiler/type/gamevar.go`
- Create: `pkg/pack/compiler/type/tuple_test.go`
- Create: `pkg/pack/compiler/type/meta_test.go`
- Create: `pkg/pack/compiler/type/array_test.go`
- Create: `pkg/pack/compiler/type/gamevar_test.go`

**TS source-of-truth:**
- `src/compiler/type/TupleType.ts` (75 LOC)
- `src/compiler/type/MetaType.ts` (113 LOC)
- `src/compiler/type/wrapped/{WrappedType,ArrayType,GameVarType}.ts` (16+46+64 LOC)

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/type/tuple_test.go
package typ

import (
	"errors"
	"testing"
)

func TestNewTupleType_RejectsLessThanTwo(t *testing.T) {
	if _, err := NewTupleType(); err == nil {
		t.Fatal("NewTupleType() = nil, want error")
	}
	if _, err := NewTupleType(PrimitiveInt); err == nil {
		t.Fatal("NewTupleType(INT) = nil, want error")
	}
}

func TestNewTupleType_FlattensNested(t *testing.T) {
	inner, err := NewTupleType(PrimitiveInt, PrimitiveString)
	if err != nil {
		t.Fatalf("NewTupleType inner: %v", err)
	}
	tup, err := NewTupleType(inner, PrimitiveBoolean)
	if err != nil {
		t.Fatalf("NewTupleType outer: %v", err)
	}
	// Flattened children: int, string, boolean (no nested tuple)
	got := tup.Children
	want := []Type{PrimitiveInt, PrimitiveString, PrimitiveBoolean}
	if len(got) != len(want) {
		t.Fatalf("children len=%d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("children[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestTupleType_Representation(t *testing.T) {
	tup, _ := NewTupleType(PrimitiveInt, PrimitiveString, PrimitiveBoolean)
	if got, want := tup.Representation(), "int,string,boolean"; got != want {
		t.Fatalf("representation = %q, want %q", got, want)
	}
}

func TestTupleType_FromList(t *testing.T) {
	if got := TupleFromList(nil); got != MetaUnit {
		t.Fatalf("fromList(nil) = %v, want MetaUnit", got)
	}
	if got := TupleFromList([]Type{}); got != MetaUnit {
		t.Fatalf("fromList([]) = %v, want MetaUnit", got)
	}
	if got := TupleFromList([]Type{PrimitiveInt}); got != PrimitiveInt {
		t.Fatalf("fromList([int]) = %v, want PrimitiveInt", got)
	}
	got := TupleFromList([]Type{PrimitiveInt, PrimitiveString})
	tup, ok := got.(*TupleType)
	if !ok {
		t.Fatalf("fromList([int, string]) = %T, want *TupleType", got)
	}
	if len(tup.Children) != 2 {
		t.Fatalf("tuple children len = %d, want 2", len(tup.Children))
	}
}

func TestTupleType_ToList(t *testing.T) {
	if got := TupleToList(nil); len(got) != 0 {
		t.Fatalf("toList(nil) len = %d, want 0", len(got))
	}
	if got := TupleToList(MetaUnit); len(got) != 0 {
		t.Fatalf("toList(MetaUnit) = %v, want []", got)
	}
	if got := TupleToList(MetaNothing); len(got) != 0 {
		t.Fatalf("toList(MetaNothing) = %v, want []", got)
	}
	got := TupleToList(PrimitiveInt)
	if len(got) != 1 || got[0] != PrimitiveInt {
		t.Fatalf("toList(PrimitiveInt) = %v, want [PrimitiveInt]", got)
	}
	tup, _ := NewTupleType(PrimitiveInt, PrimitiveString)
	got = TupleToList(tup)
	if len(got) != 2 || got[0] != PrimitiveInt || got[1] != PrimitiveString {
		t.Fatalf("toList(tuple) = %v, want [int, string]", got)
	}
}

func TestTupleType_SatisfiesAstTypeRef(t *testing.T) {
	tup, _ := NewTupleType(PrimitiveInt, PrimitiveString)
	var _ astTypeRef = tup
}

// Carry-forward: TupleType has slice field Children — verify the
// TupleType-mistakenly-keyed-into-SymbolType invariant by NOT making
// TupleType a comparable struct. (No assertion possible at the type-system
// level; covered by SymbolType tests in T6.)
var _ = errors.New
```

```go
// pkg/pack/compiler/type/meta_test.go
package typ

import "testing"

func TestMetaType_Singletons_DistinctRepresentations(t *testing.T) {
	cases := []struct {
		t    Type
		rep  string
	}{
		{MetaAny, "any"},
		{MetaNothing, "nothing"},
		{MetaError, "error"},
		{MetaUnit, "unit"},
	}
	for _, c := range cases {
		if got := c.t.Representation(); got != c.rep {
			t.Fatalf("%v.Representation() = %q, want %q", c.t, got, c.rep)
		}
	}
}

func TestMetaType_AnyType_Wrapping(t *testing.T) {
	// TS MetaType.Type(MetaType.Any).representation == 'type'
	w := NewMetaWrapping(MetaAny)
	if got := w.Representation(); got != "type" {
		t.Fatalf("wrap(Any).rep = %q, want %q", got, "type")
	}
}

func TestMetaType_TypeWrapping_NonAny(t *testing.T) {
	// TS MetaType.Type(PrimitiveType.INT).representation == 'type<int>'
	w := NewMetaWrapping(PrimitiveInt)
	if got := w.Representation(); got != "type<int>" {
		t.Fatalf("wrap(Int).rep = %q, want %q", got, "type<int>")
	}
}

func TestMetaType_OptionsNoSwitchNoArrayNoDeclNoParam(t *testing.T) {
	// TS MetaType.ts L18-23: all four flags false on every MetaType instance.
	o := MetaAny.Options()
	if o.AllowSwitch || o.AllowArray || o.AllowDeclaration || o.AllowParameter {
		t.Fatalf("MetaAny.options = %+v, want all-false", o)
	}
}

func TestMetaType_BaseTypeInteger(t *testing.T) {
	b, ok := MetaAny.BaseType()
	if !ok || b != BaseVarInteger {
		t.Fatalf("MetaAny.baseType = (%d, %v), want (Integer, true)", b, ok)
	}
}

func TestMetaType_DefaultValueMinusOne(t *testing.T) {
	if got := MetaAny.DefaultValue(); got != -1 {
		t.Fatalf("MetaAny.defaultValue = %v, want -1", got)
	}
}

func TestMetaType_CodeIsAbsent(t *testing.T) {
	// TS L25-27: throws on `code` access. Goscape returns (_, false).
	if _, ok := MetaAny.Code(); ok {
		t.Fatalf("MetaAny.Code() ok=true, want false")
	}
}

func TestMetaType_SatisfiesAstTypeRef(t *testing.T) {
	var _ astTypeRef = MetaAny
	var _ astTypeRef = NewMetaWrapping(PrimitiveInt)
}
```

```go
// pkg/pack/compiler/type/array_test.go
package typ

import "testing"

func TestArrayType_WrapPrimitive(t *testing.T) {
	a, err := NewArrayType(PrimitiveInt)
	if err != nil {
		t.Fatalf("NewArrayType(int): %v", err)
	}
	if got, want := a.Representation(), "intarray"; got != want {
		t.Fatalf("rep = %q, want %q", got, want)
	}
	if a.Inner() != PrimitiveInt {
		t.Fatalf("inner = %v, want PrimitiveInt", a.Inner())
	}
}

func TestArrayType_RejectsNestedArray(t *testing.T) {
	inner, _ := NewArrayType(PrimitiveInt)
	if _, err := NewArrayType(inner); err == nil {
		t.Fatal("NewArrayType(intarray) = nil, want error")
	}
}

func TestArrayType_BaseType(t *testing.T) {
	a, _ := NewArrayType(PrimitiveInt)
	b, ok := a.BaseType()
	if !ok || b != BaseVarInteger {
		t.Fatalf("baseType = (%d, %v), want (Integer, true)", b, ok)
	}
}

func TestArrayType_NoCode_NoDefaultValue(t *testing.T) {
	a, _ := NewArrayType(PrimitiveInt)
	if _, ok := a.Code(); ok {
		t.Fatal("ArrayType.Code() ok=true; want false (TS throws)")
	}
	if a.DefaultValue() != nil {
		t.Fatal("ArrayType.DefaultValue() != nil; want nil (TS throws)")
	}
}

func TestArrayType_OptionsAllowSwitchYesAllowArrayNo(t *testing.T) {
	a, _ := NewArrayType(PrimitiveInt)
	o := a.Options()
	if !o.AllowSwitch || !o.AllowDeclaration || !o.AllowParameter {
		t.Fatalf("ArrayType options: %+v want switch/decl/param all-true", o)
	}
	if o.AllowArray {
		t.Fatalf("ArrayType.options.AllowArray = true; want false (no nested arrays)")
	}
}

func TestArrayType_SatisfiesAstTypeRef(t *testing.T) {
	a, _ := NewArrayType(PrimitiveInt)
	var _ astTypeRef = a
}
```

```go
// pkg/pack/compiler/type/gamevar_test.go
package typ

import "testing"

func TestGameVarType_Representations(t *testing.T) {
	cases := []struct {
		ctor func(Type) Type
		want string
	}{
		{func(t Type) Type { return NewVarPlayerType(t) }, "varp<int>"},
		{func(t Type) Type { return NewVarBitType(t) }, "varbit<int>"},
		{func(t Type) Type { return NewVarNpcType(t) }, "varn<int>"},
		{func(t Type) Type { return NewVarSharedType(t) }, "vars<int>"},
	}
	for _, c := range cases {
		got := c.ctor(PrimitiveInt).Representation()
		if got != c.want {
			t.Fatalf("rep = %q, want %q", got, c.want)
		}
	}
}

func TestGameVarType_OptionsAllFalse(t *testing.T) {
	// Per TS GameVarType.ts L13-19: all four AllowX false.
	v := NewVarPlayerType(PrimitiveInt)
	o := v.Options()
	if o.AllowSwitch || o.AllowArray || o.AllowDeclaration || o.AllowParameter {
		t.Fatalf("varp options = %+v; want all-false", o)
	}
}

func TestGameVarType_BaseAndDefault(t *testing.T) {
	v := NewVarPlayerType(PrimitiveInt)
	if dv := v.DefaultValue(); dv != -1 {
		t.Fatalf("default = %v, want -1", dv)
	}
	b, ok := v.BaseType()
	if !ok || b != BaseVarInteger {
		t.Fatalf("baseType = (%d, %v), want (Integer, true)", b, ok)
	}
}

func TestGameVarType_SatisfiesAstTypeRef(t *testing.T) {
	var _ astTypeRef = NewVarPlayerType(PrimitiveInt)
	var _ astTypeRef = NewVarBitType(PrimitiveInt)
	var _ astTypeRef = NewVarNpcType(PrimitiveInt)
	var _ astTypeRef = NewVarSharedType(PrimitiveInt)
}
```

### Step 2: Run tests to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/...
```

Expected: build failure on TupleType / NewTupleType / MetaAny / NewMetaWrapping / NewArrayType / etc.

### Step 3: Write the implementations

```go
// pkg/pack/compiler/type/wrapped.go
package typ

// WrappedType is implemented by every Type that wraps an inner Type.
// Mirrors TS src/compiler/type/wrapped/WrappedType.ts.
// Both ArrayType and the four GameVarType variants implement WrappedType.
type WrappedType interface {
	Type
	Inner() Type
}
```

```go
// pkg/pack/compiler/type/tuple.go
package typ

import (
	"errors"
	"strings"
)

// TupleType combines multiple Types into one. Mirrors TS TupleType.ts.
// Children is a flattened list (never contains a nested TupleType).
type TupleType struct {
	Children []Type
	rep      string
	options  TypeOptions
}

// NewTupleType returns a *TupleType wrapping the given child Types.
// Nested TupleType arguments are flattened. Errors when fewer than 2
// children remain after flattening (TS throws — TupleType.ts L23).
func NewTupleType(children ...Type) (*TupleType, error) {
	flat := flattenTuples(children)
	if len(flat) < 2 {
		return nil, errors.New("TupleType requires at least 2 children")
	}
	reps := make([]string, len(flat))
	for i, c := range flat {
		reps[i] = c.Representation()
	}
	return &TupleType{
		Children: flat,
		rep:      strings.Join(reps, ","),
		options: NewTypeOptions(func(o *TypeOptions) {
			o.AllowSwitch = false
			o.AllowArray = false
			o.AllowDeclaration = false
			o.AllowParameter = false
		}),
	}, nil
}

func flattenTuples(children []Type) []Type {
	var out []Type
	for _, c := range children {
		if t, ok := c.(*TupleType); ok {
			out = append(out, t.Children...)
		} else {
			out = append(out, c)
		}
	}
	return out
}

func (t *TupleType) Representation() string         { return t.rep }
func (t *TupleType) Code() (string, bool)           { return "", false }
func (t *TupleType) BaseType() (BaseVarType, bool)  { return 0, false }
func (t *TupleType) DefaultValue() any              { return nil }
func (t *TupleType) Options() TypeOptions           { return t.options }
func (t *TupleType) AsTypeRef()                     {}

// TupleFromList collapses a []Type into either MetaUnit, the single
// element, or a TupleType. Mirrors TS TupleType.fromList (L42-49).
func TupleFromList(types []Type) Type {
	if len(types) == 0 {
		return MetaUnit
	}
	if len(types) == 1 {
		return types[0]
	}
	t, err := NewTupleType(types...)
	if err != nil {
		// TS treats this as unreachable since len >= 2; goscape returns
		// MetaError as a safety net so callers don't have to error-check.
		return MetaError
	}
	return t
}

// TupleToList inverts TupleFromList. Mirrors TS TupleType.toList (L57-66).
func TupleToList(t Type) []Type {
	if t == nil || t == MetaUnit || t == MetaNothing {
		return nil
	}
	if tup, ok := t.(*TupleType); ok {
		return tup.Children
	}
	return []Type{t}
}
```

```go
// pkg/pack/compiler/type/meta.go
package typ

// MetaType represents internal compiler types (Any/Nothing/Error/Unit) plus
// the parameterised wrapping types. Mirrors TS MetaType.ts.
//
// NAI-205-D-METATYPE-FLAT: TS nests MetaType.Type and MetaType.Script as
// static class properties extending MetaType. Goscape uses one base struct
// for the four named singletons + two distinct types (metaWrapping,
// metaScript) for the parameterised cases. Each implements Type.

type metaBase struct {
	rep     string
	options TypeOptions
}

func newMetaBase(name string) metaBase {
	return metaBase{
		rep: lowerASCII(name),
		options: NewTypeOptions(func(o *TypeOptions) {
			o.AllowSwitch = false
			o.AllowArray = false
			o.AllowDeclaration = false
			o.AllowParameter = false
		}),
	}
}

// metaPrimitive is the concrete impl for the four named singletons.
type metaPrimitive struct {
	metaBase
}

func (m *metaPrimitive) Representation() string         { return m.rep }
func (m *metaPrimitive) Code() (string, bool)           { return "", false }
func (m *metaPrimitive) BaseType() (BaseVarType, bool)  { return BaseVarInteger, true }
func (m *metaPrimitive) DefaultValue() any              { return -1 }
func (m *metaPrimitive) Options() TypeOptions           { return m.options }
func (m *metaPrimitive) AsTypeRef()                     {}

var (
	MetaAny     Type = &metaPrimitive{newMetaBase("any")}
	MetaNothing Type = &metaPrimitive{newMetaBase("nothing")}
	MetaError   Type = &metaPrimitive{newMetaBase("error")}
	MetaUnit    Type = &metaPrimitive{newMetaBase("unit")}
)

// metaWrapping is the TS MetaType.Type(inner) shape.
type metaWrapping struct {
	metaBase
	inner Type
}

func (m *metaWrapping) Representation() string         { return m.rep }
func (m *metaWrapping) Code() (string, bool)           { return "", false }
func (m *metaWrapping) BaseType() (BaseVarType, bool)  { return BaseVarInteger, true }
func (m *metaWrapping) DefaultValue() any              { return -1 }
func (m *metaWrapping) Options() TypeOptions           { return m.options }
func (m *metaWrapping) AsTypeRef()                     {}
func (m *metaWrapping) Inner() Type                    { return m.inner }

// NewMetaWrapping returns the MetaType.Type(inner) shape.
// When inner == MetaAny, rep = "type"; otherwise rep = "type<inner>".
// Mirrors TS MetaType.ts L80-87.
func NewMetaWrapping(inner Type) Type {
	rep := "type"
	if inner != MetaAny {
		rep = "type<" + inner.Representation() + ">"
	}
	mb := newMetaBase("type")
	mb.rep = rep
	return &metaWrapping{metaBase: mb, inner: inner}
}

// metaScript is the TS MetaType.Script(trigger, params, returns) shape.
// Deferred surface — ScriptRegistration doesn't construct these. We ship
// the shape so TypeChecking (NAI-206) doesn't need a follow-up type to land.
type metaScript struct {
	metaBase
	// trigger field intentionally typed as the ast.TriggerRef marker to avoid
	// cycle on type → trigger. Read-only — set by NewMetaScript and never mutated.
	trigger anyMarkerInterfaceForTrigger
	params  Type
	returns Type
}

// anyMarkerInterfaceForTrigger is the locally-named alias for ast.TriggerRef.
// Goscape's type pkg never imports ast (cyclic). The constructor below takes
// an `any` and stores it; consumers in semantics/codegen retain the original
// concrete pointer via separate parameter passing rather than reading it back
// out of the metaScript. This is acceptable because metaScript exists only
// for type-system *representation* parity — its trigger field is not read
// during NAI-205. NAI-206 may need a different approach.
type anyMarkerInterfaceForTrigger = any

func (m *metaScript) Representation() string         { return m.rep }
func (m *metaScript) Code() (string, bool)           { return "", false }
func (m *metaScript) BaseType() (BaseVarType, bool)  { return BaseVarInteger, true }
func (m *metaScript) DefaultValue() any              { return -1 }
func (m *metaScript) Options() TypeOptions           { return m.options }
func (m *metaScript) AsTypeRef()                     {}

// NewMetaScript constructs the TS MetaType.Script shape. NAI-205 doesn't
// consume it; ports the constructor only for symmetry with MetaType.ts.
// triggerRef is opaque (see anyMarkerInterfaceForTrigger).
func NewMetaScript(triggerRef any, params, returns Type) Type {
	rep := "script(" + params.Representation() + ")->(" + returns.Representation() + ")"
	mb := newMetaBase("script")
	mb.rep = rep
	return &metaScript{metaBase: mb, trigger: triggerRef, params: params, returns: returns}
}
```

```go
// pkg/pack/compiler/type/array.go
package typ

import "errors"

// ArrayType wraps another Type. Mirrors TS wrapped/ArrayType.ts.
type ArrayType struct {
	inner   Type
	options TypeOptions
}

// NewArrayType wraps inner. Errors if inner is itself an ArrayType.
// Mirrors TS L20 throw.
func NewArrayType(inner Type) (*ArrayType, error) {
	if _, nested := inner.(*ArrayType); nested {
		return nil, errors.New("ArrayType cannot wrap another ArrayType")
	}
	return &ArrayType{
		inner: inner,
		options: NewTypeOptions(func(o *TypeOptions) {
			o.AllowArray = false
			o.AllowDeclaration = true
			o.AllowSwitch = true
			o.AllowParameter = true
		}),
	}, nil
}

func (a *ArrayType) Inner() Type                    { return a.inner }
func (a *ArrayType) Representation() string         { return a.inner.Representation() + "array" }
func (a *ArrayType) Code() (string, bool)           { return "", false }
func (a *ArrayType) BaseType() (BaseVarType, bool)  { return BaseVarInteger, true }
func (a *ArrayType) DefaultValue() any              { return nil }
func (a *ArrayType) Options() TypeOptions           { return a.options }
func (a *ArrayType) AsTypeRef()                     {}
```

```go
// pkg/pack/compiler/type/gamevar.go
package typ

// GameVarType-family: four shapes that wrap an inner Type. Mirrors
// TS wrapped/GameVarType.ts.
//
// All four share field shape (inner, rep, options) — defined once via gameVarBase.
type gameVarBase struct {
	inner   Type
	rep     string
	options TypeOptions
}

func newGameVarOptions() TypeOptions {
	return NewTypeOptions(func(o *TypeOptions) {
		o.AllowSwitch = false
		o.AllowArray = false
		o.AllowDeclaration = false
		o.AllowParameter = false
	})
}

type VarPlayerType struct{ gameVarBase }
type VarBitType struct{ gameVarBase }
type VarNpcType struct{ gameVarBase }
type VarSharedType struct{ gameVarBase }

func NewVarPlayerType(inner Type) *VarPlayerType {
	return &VarPlayerType{gameVarBase{inner, "varp<" + inner.Representation() + ">", newGameVarOptions()}}
}

func NewVarBitType(inner Type) *VarBitType {
	return &VarBitType{gameVarBase{inner, "varbit<" + inner.Representation() + ">", newGameVarOptions()}}
}

func NewVarNpcType(inner Type) *VarNpcType {
	return &VarNpcType{gameVarBase{inner, "varn<" + inner.Representation() + ">", newGameVarOptions()}}
}

func NewVarSharedType(inner Type) *VarSharedType {
	return &VarSharedType{gameVarBase{inner, "vars<" + inner.Representation() + ">", newGameVarOptions()}}
}

// Type-interface methods on gameVarBase. Concrete-type method-sets pick these up
// via Go struct embedding. All four sub-types share these implementations.
func (g gameVarBase) Representation() string         { return g.rep }
func (g gameVarBase) Code() (string, bool)           { return "", false }
func (g gameVarBase) BaseType() (BaseVarType, bool)  { return BaseVarInteger, true }
func (g gameVarBase) DefaultValue() any              { return -1 }
func (g gameVarBase) Options() TypeOptions           { return g.options }
func (g gameVarBase) Inner() Type                    { return g.inner }
func (g gameVarBase) AsTypeRef()                     {}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/...
```

Expected: PASS.

### Step 5: Commit

```bash
git add pkg/pack/compiler/type/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/type): NAI-205 T3 — Tuple/Meta/Wrapped/Array/GameVar

Port the wrapped + composite Type concretes:
- TupleType + TupleFromList/TupleToList
- MetaType (Any/Nothing/Error/Unit singletons + MetaWrapping/MetaScript
  parameterised factories)
- WrappedType interface (implemented by ArrayType + four GameVarTypes)
- ArrayType (rejects nested arrays)
- VarPlayerType / VarBitType / VarNpcType / VarSharedType

NAI-205-D-METATYPE-FLAT: TS nested-class shape flattens to two factory fns.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `pkg/pack/compiler/type/` part C — TypeManager

**Spec ref:** §6.2.

**Files:**
- Create: `pkg/pack/compiler/type/manager.go`
- Create: `pkg/pack/compiler/type/manager_test.go`

**TS source-of-truth:** `src/compiler/type/TypeManager.ts` (134 LOC).

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/type/manager_test.go
package typ

import (
	"strings"
	"testing"
)

func TestTypeManager_Register_DoubleErrors(t *testing.T) {
	m := NewTypeManager()
	if err := m.Register("int", PrimitiveInt); err != nil {
		t.Fatalf("first Register int: %v", err)
	}
	if err := m.Register("int", PrimitiveInt); err == nil {
		t.Fatal("double Register int: nil err, want collision error")
	}
}

func TestTypeManager_RegisterByRepresentation(t *testing.T) {
	m := NewTypeManager()
	if err := m.RegisterByRepresentation(PrimitiveInt); err != nil {
		t.Fatalf("RegisterByRepresentation(int): %v", err)
	}
	got, err := m.Find("int", false)
	if err != nil {
		t.Fatalf("Find int: %v", err)
	}
	if got != PrimitiveInt {
		t.Fatalf("Find int = %v, want PrimitiveInt", got)
	}
}

func TestTypeManager_FindOrNil_Miss(t *testing.T) {
	m := NewTypeManager()
	if got := m.FindOrNil("doesnotexist", false); got != nil {
		t.Fatalf("FindOrNil miss = %v, want nil", got)
	}
}

func TestTypeManager_Find_ErrorOnMiss(t *testing.T) {
	m := NewTypeManager()
	if _, err := m.Find("doesnotexist", false); err == nil {
		t.Fatal("Find miss: nil err, want error")
	} else if !strings.Contains(err.Error(), "doesnotexist") {
		t.Fatalf("Find miss err = %v; want to mention 'doesnotexist'", err)
	}
}

func TestTypeManager_AllowArray_WrapsBaseType(t *testing.T) {
	m := NewTypeManager()
	_ = m.RegisterByRepresentation(PrimitiveInt)
	got := m.FindOrNil("intarray", true)
	if got == nil {
		t.Fatal("FindOrNil intarray (allowArray=true) = nil")
	}
	a, ok := got.(*ArrayType)
	if !ok {
		t.Fatalf("intarray result type = %T, want *ArrayType", got)
	}
	if a.Inner() != PrimitiveInt {
		t.Fatalf("intarray.Inner = %v, want PrimitiveInt", a.Inner())
	}
}

func TestTypeManager_AllowArray_RejectsForOptionsAllowArrayFalse(t *testing.T) {
	// STRING disables AllowArray in its TypeOptions (see TS PrimitiveType L46-48).
	m := NewTypeManager()
	_ = m.RegisterByRepresentation(PrimitiveString)
	got := m.FindOrNil("stringarray", true)
	if got != nil {
		t.Fatalf("FindOrNil stringarray = %v, want nil (string.allowArray=false)", got)
	}
}

func TestTypeManager_AllowArray_RejectsForMissingBase(t *testing.T) {
	m := NewTypeManager()
	got := m.FindOrNil("zarray", true)
	if got != nil {
		t.Fatalf("FindOrNil zarray with no base registered = %v, want nil", got)
	}
}

func TestTypeManager_ChangeOptions_MutatesInPlace(t *testing.T) {
	m := NewTypeManager()
	custom := newPrimitiveType("CUSTOM", "x", BaseVarInteger, -1)
	_ = m.RegisterByRepresentation(custom)
	if err := m.ChangeOptions("custom", func(o *TypeOptions) {
		o.AllowSwitch = false
	}); err != nil {
		t.Fatalf("ChangeOptions: %v", err)
	}
	got, _ := m.Find("custom", false)
	if got.Options().AllowSwitch {
		t.Fatalf("after ChangeOptions: AllowSwitch still true")
	}
}

func TestTypeManager_RegisterNew_RoundTrip(t *testing.T) {
	m := NewTypeManager()
	got, err := m.RegisterNew("widget", "w", BaseVarInteger, -1, func(o *TypeOptions) {
		o.AllowArray = false
	})
	if err != nil {
		t.Fatalf("RegisterNew: %v", err)
	}
	if got.Representation() != "widget" {
		t.Fatalf("rep = %q, want %q", got.Representation(), "widget")
	}
	c, ok := got.Code()
	if !ok || c != "w" {
		t.Fatalf("code = (%q, %v), want (\"w\", true)", c, ok)
	}
	if got.Options().AllowArray {
		t.Fatal("widget AllowArray = true, want false")
	}
	resolved, _ := m.Find("widget", false)
	if resolved.Representation() != "widget" {
		t.Fatalf("Find rep mismatch")
	}
}

func TestTypeManager_Check_RegisteredCheckerFires(t *testing.T) {
	m := NewTypeManager()
	m.AddTypeChecker(func(left, right Type) bool {
		return left == PrimitiveInt && right == PrimitiveBoolean
	})
	if !m.Check(PrimitiveInt, PrimitiveBoolean) {
		t.Fatal("Check(int, boolean): false, want true")
	}
	if m.Check(PrimitiveBoolean, PrimitiveInt) {
		t.Fatal("Check(boolean, int): true, want false")
	}
}

func TestTypeManager_Check_EmptyChain(t *testing.T) {
	m := NewTypeManager()
	if m.Check(PrimitiveInt, PrimitiveInt) {
		t.Fatal("Check on empty checker chain: true, want false")
	}
}
```

### Step 2: Run tests to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/...
```

Expected: build failure on `NewTypeManager`, `*TypeManager.Register`, etc.

### Step 3: Write the implementation

```go
// pkg/pack/compiler/type/manager.go
package typ

import (
	"fmt"
	"strings"
)

// TypeChecker is a binary predicate over Types — does `right` flow into `left`?
// Mirrors TS TypeManager.ts L6.
type TypeChecker func(left, right Type) bool

// TypeBuilder mutates options as part of a registerNew/changeOptions call.
type TypeBuilder func(*TypeOptions)

// TypeManager owns the type registry plus a chain of assignability checkers.
// Mirrors TS class TypeManager (TypeManager.ts).
//
// NAI-205-D-TYPE-NO-INTERN: TS uses WeakMap interning + cache lookups for
// equality; goscape relies on singleton pointers for primitives/meta and
// Representation() comparison at the call sites that need it.
type TypeManager struct {
	nameToType map[string]Type
	checkers   []TypeChecker
}

func NewTypeManager() *TypeManager {
	return &TypeManager{
		nameToType: map[string]Type{},
	}
}

// Register inserts a Type under the given name. Errors on duplicate name.
// Mirrors TS register(name, type) overload at TypeManager.ts L23-30.
func (m *TypeManager) Register(name string, t Type) error {
	if _, exists := m.nameToType[name]; exists {
		return fmt.Errorf("type %q is already registered", name)
	}
	m.nameToType[name] = t
	return nil
}

// RegisterByRepresentation registers t under t.Representation().
// Mirrors TS register(type) overload at TypeManager.ts L31-37.
func (m *TypeManager) RegisterByRepresentation(t Type) error {
	return m.Register(t.Representation(), t)
}

// RegisterAll registers every type in the slice via RegisterByRepresentation.
// Mirrors TS registerAll(enumClass) at TypeManager.ts L66-70 — goscape passes
// the slice directly rather than reading a {ALL: readonly Type[]} static.
func (m *TypeManager) RegisterAll(types []Type) error {
	for _, t := range types {
		if err := m.RegisterByRepresentation(t); err != nil {
			return err
		}
	}
	return nil
}

// RegisterNew creates a Type via the createType-equivalent fn-builder shape
// and registers it. Mirrors TS registerNew at TypeManager.ts L42-58.
// Returns the new Type so the caller can keep a reference for set-membership
// checks (e.g. the categoryType cache in ScriptRegistration).
func (m *TypeManager) RegisterNew(name, code string, base BaseVarType, defaultVal any, builders ...TypeBuilder) (Type, error) {
	t := newPrimitiveType(name, code, base, defaultVal, builders...)
	if err := m.RegisterByRepresentation(t); err != nil {
		return nil, err
	}
	return t, nil
}

// ChangeOptions mutates the options of a previously-registered Type via the
// builder fn. Errors if name is unknown. Mirrors TS changeOptions L77-82.
//
// Note: only types stored as *PrimitiveType (created via RegisterNew or
// constructed locally) carry mutable options. Singletons (PrimitiveInt
// etc.) have package-level options. ChangeOptions therefore mutates the
// stored TypeOptions struct directly via pointer.
func (m *TypeManager) ChangeOptions(name string, build TypeBuilder) error {
	t, ok := m.nameToType[name]
	if !ok {
		return fmt.Errorf("type %q not found", name)
	}
	switch concrete := t.(type) {
	case *PrimitiveType:
		build(&concrete.options)
		return nil
	}
	return fmt.Errorf("type %q is not mutable", name)
}

// Find returns the named type or an error. AllowArray strips an "array"
// suffix and wraps base in ArrayType. Mirrors TS find L89-95.
func (m *TypeManager) Find(name string, allowArray bool) (Type, error) {
	if t := m.FindOrNil(name, allowArray); t != nil {
		return t, nil
	}
	return nil, fmt.Errorf("unable to find type %q", name)
}

// FindOrNil returns the named type or nil. Mirrors TS findOrNull L102-110.
func (m *TypeManager) FindOrNil(name string, allowArray bool) Type {
	if allowArray && strings.HasSuffix(name, "array") {
		baseName := name[:len(name)-5]
		base := m.FindOrNil(baseName, false)
		if base == nil || !base.Options().AllowArray {
			return nil
		}
		a, err := NewArrayType(base)
		if err != nil {
			return nil
		}
		return a
	}
	return m.nameToType[name]
}

// AddTypeChecker appends c to the chain. Mirrors TS L118.
func (m *TypeManager) AddTypeChecker(c TypeChecker) {
	m.checkers = append(m.checkers, c)
}

// Check returns true iff any registered checker accepts (left, right).
// Mirrors TS check L124-126.
func (m *TypeManager) Check(left, right Type) bool {
	for _, c := range m.checkers {
		if c(left, right) {
			return true
		}
	}
	return false
}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/...
```

Expected: PASS.

### Step 5: Commit

```bash
git add pkg/pack/compiler/type/manager.go pkg/pack/compiler/type/manager_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/type): NAI-205 T4 — TypeManager (name→Type registry + checker chain)

Port TypeManager: Register / RegisterByRepresentation / RegisterAll /
RegisterNew / ChangeOptions / Find / FindOrNil / AddTypeChecker / Check.
TS thrown errors → Go returned errors; everything else is a literal port.

NAI-205-D-TYPE-NO-INTERN documented on TypeManager.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `pkg/pack/compiler/trigger/` — SubjectMode, TriggerType, TriggerManager, CommandTrigger

**Spec ref:** §6.4.

**Files:**
- Create: `pkg/pack/compiler/trigger/subjectmode.go`
- Create: `pkg/pack/compiler/trigger/triggertype.go`
- Create: `pkg/pack/compiler/trigger/manager.go`
- Create: `pkg/pack/compiler/trigger/command.go`
- Create: `pkg/pack/compiler/trigger/subjectmode_test.go`
- Create: `pkg/pack/compiler/trigger/triggertype_test.go`
- Create: `pkg/pack/compiler/trigger/manager_test.go`

**TS source-of-truth:**
- `src/compiler/trigger/SubjectMode.ts` (30 LOC)
- `src/compiler/trigger/TriggerType.ts` (56 LOC, interface)
- `src/compiler/trigger/TriggerManager.ts` (62 LOC)
- `src/compiler/trigger/CommandTrigger.ts` (13 LOC)

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/trigger/subjectmode_test.go
package trigger

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestSubjectMode_NoneAndName_DistinctSingletons(t *testing.T) {
	if ModeNone == nil || ModeName == nil {
		t.Fatal("ModeNone or ModeName is nil")
	}
	if ModeNone == ModeName {
		t.Fatal("ModeNone == ModeName; want distinct")
	}
}

func TestSubjectMode_NewModeTypeFields(t *testing.T) {
	tm := NewModeType(typ.PrimitiveInt, true, true)
	if tm.Type != typ.PrimitiveInt {
		t.Fatalf("TypeMode.Type = %v, want PrimitiveInt", tm.Type)
	}
	if !tm.Category {
		t.Fatal("TypeMode.Category = false, want true")
	}
	if !tm.Global {
		t.Fatal("TypeMode.Global = false, want true")
	}
}

func TestIsTypeMode_PositiveAndNegative(t *testing.T) {
	tm := NewModeType(typ.PrimitiveInt, false, false)
	got, ok := IsTypeMode(tm)
	if !ok {
		t.Fatal("IsTypeMode(TypeMode) ok=false, want true")
	}
	if got.Type != typ.PrimitiveInt {
		t.Fatalf("IsTypeMode returned wrong TypeMode: %+v", got)
	}

	if _, ok := IsTypeMode(ModeNone); ok {
		t.Fatal("IsTypeMode(ModeNone) ok=true, want false")
	}
	if _, ok := IsTypeMode(ModeName); ok {
		t.Fatal("IsTypeMode(ModeName) ok=true, want false")
	}
}
```

```go
// pkg/pack/compiler/trigger/triggertype_test.go
package trigger

import "testing"

func TestCommandTrigger_FieldShape(t *testing.T) {
	c := CommandTrigger
	if c.Identifier != "command" {
		t.Fatalf("CommandTrigger.Identifier = %q, want \"command\"", c.Identifier)
	}
	if c.ID != -1 {
		t.Fatalf("CommandTrigger.ID = %d, want -1", c.ID)
	}
	if c.SubjectMode != ModeName {
		t.Fatalf("CommandTrigger.SubjectMode != ModeName")
	}
	if !c.AllowParameters {
		t.Fatal("CommandTrigger.AllowParameters = false, want true")
	}
	if !c.AllowReturns {
		t.Fatal("CommandTrigger.AllowReturns = false, want true")
	}
	if c.Parameters != nil {
		t.Fatalf("CommandTrigger.Parameters = %v, want nil", c.Parameters)
	}
	if c.Returns != nil {
		t.Fatalf("CommandTrigger.Returns = %v, want nil", c.Returns)
	}
}

func TestTriggerType_SatisfiesAstTriggerRef(t *testing.T) {
	var _ astTriggerRef = CommandTrigger
}

type astTriggerRef interface {
	AsTriggerRef()
}
```

```go
// pkg/pack/compiler/trigger/manager_test.go
package trigger

import (
	"strings"
	"testing"
)

func newTestTrigger(name string) *TriggerType {
	return &TriggerType{
		ID:              -1,
		Identifier:      name,
		SubjectMode:     ModeNone,
		AllowParameters: false,
		AllowReturns:    false,
	}
}

func TestTriggerManager_RegisterAndFind(t *testing.T) {
	m := NewTriggerManager()
	tg := newTestTrigger("proc")
	if err := m.Register("proc", tg); err != nil {
		t.Fatalf("Register proc: %v", err)
	}
	got, err := m.Find("proc")
	if err != nil {
		t.Fatalf("Find proc: %v", err)
	}
	if got != tg {
		t.Fatalf("Find proc returned different pointer")
	}
}

func TestTriggerManager_RegisterTrigger_UsesIdentifier(t *testing.T) {
	m := NewTriggerManager()
	tg := newTestTrigger("label")
	if err := m.RegisterTrigger(tg); err != nil {
		t.Fatalf("RegisterTrigger: %v", err)
	}
	if got, _ := m.Find("label"); got != tg {
		t.Fatalf("RegisterTrigger did not register under .Identifier")
	}
}

func TestTriggerManager_DoubleRegisterErrors(t *testing.T) {
	m := NewTriggerManager()
	_ = m.Register("proc", newTestTrigger("proc"))
	if err := m.Register("proc", newTestTrigger("proc")); err == nil {
		t.Fatal("double Register: nil err, want collision")
	}
}

func TestTriggerManager_FindOrNil_Miss(t *testing.T) {
	m := NewTriggerManager()
	if got := m.FindOrNil("nope"); got != nil {
		t.Fatalf("FindOrNil miss = %v, want nil", got)
	}
}

func TestTriggerManager_FindErrorMessageContainsName(t *testing.T) {
	m := NewTriggerManager()
	_, err := m.Find("nope")
	if err == nil {
		t.Fatal("Find miss: nil err")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v; want to mention 'nope'", err)
	}
}

func TestTriggerManager_RegisterAll(t *testing.T) {
	m := NewTriggerManager()
	triggers := []*TriggerType{
		newTestTrigger("proc"),
		newTestTrigger("label"),
	}
	if err := m.RegisterAll(triggers); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	if _, err := m.Find("proc"); err != nil {
		t.Fatalf("Find proc after RegisterAll: %v", err)
	}
	if _, err := m.Find("label"); err != nil {
		t.Fatalf("Find label after RegisterAll: %v", err)
	}
}
```

### Step 2: Run tests to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/trigger/...
```

Expected: build failure across all symbols.

### Step 3: Write the implementations

```go
// pkg/pack/compiler/trigger/subjectmode.go
package trigger

import (
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// SubjectMode is the sealed interface representing how a trigger's subject
// is validated. Mirrors TS SubjectMode.ts.
//
// Three concrete impls: ModeNone (global-only), ModeName (any name), and
// TypeMode (subject is a reference to a Type instance). The sealing is
// enforced via the unexported subjectMode() method.
type SubjectMode interface {
	subjectMode()
}

type modeNoneT struct{}
type modeNameT struct{}

func (modeNoneT) subjectMode() {}
func (modeNameT) subjectMode() {}

// ModeNone allows only the global subject `_`. Mirrors TS SubjectMode.None.
var ModeNone SubjectMode = modeNoneT{}

// ModeName allows any string as the subject. Mirrors TS SubjectMode.Name.
var ModeName SubjectMode = modeNameT{}

// TypeMode is a value-typed SubjectMode that carries the resolved Type and
// the category/global feature flags. Mirrors TS SubjectMode.Type(...).
type TypeMode struct {
	Type     typ.Type
	Category bool
	Global   bool
}

func (TypeMode) subjectMode() {}

// NewModeType is the goscape equivalent of TS `SubjectMode.Type(t, category, global)`.
// Returns a value-typed TypeMode (no interning; TS likewise returns a fresh
// class instance per call).
func NewModeType(t typ.Type, category, global bool) TypeMode {
	return TypeMode{Type: t, Category: category, Global: global}
}

// IsTypeMode returns (tm, true) when m is a TypeMode, otherwise (zero, false).
// Replaces TS `'type' in mode` discriminator check.
func IsTypeMode(m SubjectMode) (TypeMode, bool) {
	tm, ok := m.(TypeMode)
	return tm, ok
}
```

```go
// pkg/pack/compiler/trigger/triggertype.go
package trigger

import (
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TriggerType is the goscape port of TS interface TriggerType.
//
// TS makes it an interface implemented by const-literal trigger objects;
// goscape uses a struct since every trigger is a frozen data record.
// Pointer receivers satisfy ast.TriggerRef.
//
// NAI-205-D-TRIGGER-POINTERS-DEFERRED: TS field `pointers: Set<PointerType>`
// pulls in PointerType (codegen package, NAI-207). Goscape keeps the field
// as `any` (unread by ScriptRegistration; set to nil for the test fixtures
// in NAI-205 and for the production CommandTrigger).
type TriggerType struct {
	ID              int
	Identifier      string
	SubjectMode     SubjectMode
	AllowParameters bool
	Parameters      typ.Type // nil = trigger expects no specific param shape
	AllowReturns    bool
	Returns         typ.Type // nil = trigger expects no specific return shape
	Pointers        any
}

// AsTriggerRef satisfies ast.TriggerRef so *TriggerType may be stored in
// ast.Script.TriggerType.
func (*TriggerType) AsTriggerRef() {}
```

```go
// pkg/pack/compiler/trigger/manager.go
package trigger

import "fmt"

// TriggerManager is a name → *TriggerType registry. Mirrors TS TriggerManager.ts.
type TriggerManager struct {
	nameToTrigger map[string]*TriggerType
}

func NewTriggerManager() *TriggerManager {
	return &TriggerManager{nameToTrigger: map[string]*TriggerType{}}
}

// Register inserts t under name. Errors on duplicate name. Mirrors TS L15-19.
func (m *TriggerManager) Register(name string, t *TriggerType) error {
	if _, ok := m.nameToTrigger[name]; ok {
		return fmt.Errorf("trigger %q is already registered", name)
	}
	m.nameToTrigger[name] = t
	return nil
}

// RegisterTrigger registers t under t.Identifier. Mirrors TS L24-26.
func (m *TriggerManager) RegisterTrigger(t *TriggerType) error {
	return m.Register(t.Identifier, t)
}

// RegisterAll registers every trigger via RegisterTrigger.
func (m *TriggerManager) RegisterAll(triggers []*TriggerType) error {
	for _, t := range triggers {
		if err := m.RegisterTrigger(t); err != nil {
			return err
		}
	}
	return nil
}

// Find returns the named trigger or an error. Mirrors TS L40-46.
func (m *TriggerManager) Find(name string) (*TriggerType, error) {
	if t, ok := m.nameToTrigger[name]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("unable to find trigger %q", name)
}

// FindOrNil returns the named trigger or nil. Mirrors TS L53-55.
func (m *TriggerManager) FindOrNil(name string) *TriggerType {
	return m.nameToTrigger[name]
}
```

```go
// pkg/pack/compiler/trigger/command.go
package trigger

// CommandTrigger is the sentinel trigger for `command` scripts.
// Mirrors TS CommandTrigger.ts. ScriptRegistration compares against this
// pointer to gate the `*`-suffix check and other command-only behaviour.
var CommandTrigger = &TriggerType{
	ID:              -1,
	Identifier:      "command",
	SubjectMode:     ModeName,
	AllowParameters: true,
	Parameters:      nil,
	AllowReturns:    true,
	Returns:         nil,
	Pointers:        nil,
}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/trigger/...
```

Expected: PASS.

### Step 5: Commit

```bash
git add pkg/pack/compiler/trigger/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/trigger): NAI-205 T5 — SubjectMode, TriggerType, TriggerManager, CommandTrigger

Port the four trigger-package files:
- SubjectMode (sealed interface; ModeNone, ModeName singletons +
  TypeMode value-type + IsTypeMode discriminator)
- TriggerType (struct; pointer receivers satisfy ast.TriggerRef)
- TriggerManager (Register/RegisterTrigger/RegisterAll/Find/FindOrNil)
- CommandTrigger singleton

NAI-205-D-TRIGGER-POINTERS-DEFERRED documented on TriggerType.Pointers
(any-typed; PointerType lands in NAI-207 codegen).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `pkg/pack/compiler/symbol/` — Symbol/ScriptSymbol/SymbolType/SymbolTable

**Spec ref:** §6.3.

**Files:**
- Create: `pkg/pack/compiler/symbol/symbol.go`
- Create: `pkg/pack/compiler/symbol/script.go`
- Create: `pkg/pack/compiler/symbol/symboltype.go`
- Create: `pkg/pack/compiler/symbol/table.go`
- Create: `pkg/pack/compiler/symbol/symbol_test.go`
- Create: `pkg/pack/compiler/symbol/symboltype_test.go`
- Create: `pkg/pack/compiler/symbol/table_test.go`

**TS source-of-truth:**
- `src/compiler/symbol/Symbol.ts` (39 LOC)
- `src/compiler/symbol/ScriptSymbol.ts` (36 LOC — `pointers(checker)` method DEFERRED per NAI-205-D-SCRIPTSYMBOL-NO-POINTERS)
- `src/compiler/symbol/SymbolType.ts` (45 LOC)
- `src/compiler/symbol/SymbolTable.ts` (96 LOC)

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/symbol/symbol_test.go
package symbol

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestSymbol_LocalVariableShape(t *testing.T) {
	s := &LocalVariableSymbol{Name: "x", Type: typ.PrimitiveInt}
	if s.SymbolName() != "x" {
		t.Fatalf("SymbolName = %q, want \"x\"", s.SymbolName())
	}
}

func TestSymbol_BasicShape(t *testing.T) {
	s := &BasicSymbol{Name: "Goblin Mail", Type: typ.PrimitiveInt, IsProtected: true}
	if s.SymbolName() != "Goblin Mail" {
		t.Fatalf("SymbolName = %q", s.SymbolName())
	}
	if !s.IsProtected {
		t.Fatal("IsProtected = false, want true")
	}
}

func TestSymbol_ConstantShape(t *testing.T) {
	s := &ConstantSymbol{Name: "MAX_LEVEL", Value: "99"}
	if s.SymbolName() != "MAX_LEVEL" || s.Value != "99" {
		t.Fatalf("ConstantSymbol shape: %+v", s)
	}
}

func TestServerScriptSymbol_Shape(t *testing.T) {
	tg := makeTriggerStub("proc")
	s := &ServerScriptSymbol{
		ScriptSymbolFields: ScriptSymbolFields{
			Trigger:    tg,
			Name:       "foo",
			Parameters: typ.MetaUnit,
			Returns:    typ.MetaUnit,
		},
	}
	if s.SymbolName() != "foo" {
		t.Fatalf("SymbolName = %q", s.SymbolName())
	}
	if !s.IsServerScript() {
		t.Fatal("ServerScriptSymbol.IsServerScript() = false")
	}
}

func TestClientScriptSymbol_NotServer(t *testing.T) {
	s := &ClientScriptSymbol{}
	if s.IsServerScript() {
		t.Fatal("ClientScriptSymbol.IsServerScript() = true")
	}
}

func TestAllSymbols_SatisfyAstSymbolRef(t *testing.T) {
	var _ astSymbolRef = (*LocalVariableSymbol)(nil)
	var _ astSymbolRef = (*BasicSymbol)(nil)
	var _ astSymbolRef = (*ConstantSymbol)(nil)
	var _ astSymbolRef = (*ServerScriptSymbol)(nil)
	var _ astSymbolRef = (*ClientScriptSymbol)(nil)
}

type astSymbolRef interface {
	AsSymbolRef()
}
```

```go
// pkg/pack/compiler/symbol/symboltype_test.go
package symbol

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func makeTriggerStub(name string) *trigger.TriggerType {
	return &trigger.TriggerType{ID: -1, Identifier: name, SubjectMode: trigger.ModeNone}
}

func TestSymbolType_ServerScriptKeyByIdentifier(t *testing.T) {
	tg := makeTriggerStub("proc")
	a := SymbolTypeServerScript(tg)
	b := SymbolTypeServerScript(tg)
	if a.Key() != b.Key() {
		t.Fatalf("server-script Key mismatch on same trigger: %q vs %q", a.Key(), b.Key())
	}

	other := makeTriggerStub("label")
	c := SymbolTypeServerScript(other)
	if a.Key() == c.Key() {
		t.Fatalf("server-script Key collision across triggers: %q == %q", a.Key(), c.Key())
	}
}

func TestSymbolType_BasicKeyByRepresentation(t *testing.T) {
	a := SymbolTypeBasic(typ.PrimitiveInt)
	b := SymbolTypeBasic(typ.PrimitiveInt)
	if a.Key() != b.Key() {
		t.Fatalf("basic Key mismatch on same type: %q vs %q", a.Key(), b.Key())
	}

	c := SymbolTypeBasic(typ.PrimitiveString)
	if a.Key() == c.Key() {
		t.Fatalf("basic Key collision across types: %q == %q", a.Key(), c.Key())
	}
}

func TestSymbolType_DistinctKindsKeyDifferent(t *testing.T) {
	tg := makeTriggerStub("proc")
	server := SymbolTypeServerScript(tg).Key()
	client := SymbolTypeClientScript(tg).Key()
	local := SymbolTypeLocalVariable().Key()
	constant := SymbolTypeConstant().Key()
	keys := []string{server, client, local, constant}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] == keys[j] {
				t.Fatalf("kind-collision: keys[%d] == keys[%d] = %q", i, j, keys[i])
			}
		}
	}
}
```

```go
// pkg/pack/compiler/symbol/table_test.go
package symbol

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestSymbolTable_InsertAndFind(t *testing.T) {
	tg := makeTriggerStub("proc")
	st := NewSymbolTable(nil)
	sym := &ServerScriptSymbol{ScriptSymbolFields: ScriptSymbolFields{
		Trigger: tg, Name: "foo", Parameters: typ.MetaUnit, Returns: typ.MetaUnit,
	}}
	if !st.Insert(SymbolTypeServerScript(tg), sym) {
		t.Fatal("first Insert returned false")
	}
	got := st.Find(SymbolTypeServerScript(tg), "foo")
	if got != sym {
		t.Fatalf("Find returned different pointer: %v", got)
	}
}

func TestSymbolTable_Insert_DuplicateReturnsFalse(t *testing.T) {
	tg := makeTriggerStub("proc")
	st := NewSymbolTable(nil)
	first := &ServerScriptSymbol{ScriptSymbolFields: ScriptSymbolFields{
		Trigger: tg, Name: "foo",
	}}
	second := &ServerScriptSymbol{ScriptSymbolFields: ScriptSymbolFields{
		Trigger: tg, Name: "foo",
	}}
	if !st.Insert(SymbolTypeServerScript(tg), first) {
		t.Fatal("first Insert returned false")
	}
	if st.Insert(SymbolTypeServerScript(tg), second) {
		t.Fatal("second Insert returned true; want false (already-defined)")
	}
}

func TestSymbolTable_Find_Miss(t *testing.T) {
	st := NewSymbolTable(nil)
	tg := makeTriggerStub("proc")
	if got := st.Find(SymbolTypeServerScript(tg), "missing"); got != nil {
		t.Fatalf("Find miss = %v, want nil", got)
	}
}

func TestSymbolTable_ChildLookupWalksParent(t *testing.T) {
	tg := makeTriggerStub("proc")
	root := NewSymbolTable(nil)
	root.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "foo"},
	})
	child := root.CreateSubTable()
	got := child.Find(SymbolTypeServerScript(tg), "foo")
	if got == nil {
		t.Fatal("child.Find did not walk to parent")
	}
}

func TestSymbolTable_ParentDoesNotWalkChild(t *testing.T) {
	tg := makeTriggerStub("proc")
	root := NewSymbolTable(nil)
	child := root.CreateSubTable()
	child.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "child_only"},
	})
	if root.Find(SymbolTypeServerScript(tg), "child_only") != nil {
		t.Fatal("root.Find found child-table entry")
	}
}

func TestSymbolTable_ChildInsertBlocksOnParent(t *testing.T) {
	// Per TS L29-36: child Insert checks the parent chain. If parent already
	// has the same (type, name), child Insert returns false.
	tg := makeTriggerStub("proc")
	root := NewSymbolTable(nil)
	root.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "foo"},
	})
	child := root.CreateSubTable()
	if child.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "foo"},
	}) {
		t.Fatal("child.Insert succeeded despite parent having same entry")
	}
}

func TestSymbolTable_BasicNameNormalisation(t *testing.T) {
	// Per TS L18-22: Basic kind normalises name (lowercase + whitespace→_).
	st := NewSymbolTable(nil)
	st.Insert(SymbolTypeBasic(typ.PrimitiveInt), &BasicSymbol{
		Name: "Wooden Bowl", Type: typ.PrimitiveInt,
	})
	got := st.Find(SymbolTypeBasic(typ.PrimitiveInt), "wooden_bowl")
	if got == nil {
		t.Fatal("normalised lookup miss: 'Wooden Bowl' inserted; 'wooden_bowl' should hit")
	}
}

func TestSymbolTable_ServerScriptNameNotNormalised(t *testing.T) {
	tg := makeTriggerStub("proc")
	st := NewSymbolTable(nil)
	st.Insert(SymbolTypeServerScript(tg), &ServerScriptSymbol{
		ScriptSymbolFields: ScriptSymbolFields{Trigger: tg, Name: "PascalCase"},
	})
	if got := st.Find(SymbolTypeServerScript(tg), "pascalcase"); got != nil {
		t.Fatal("server-script name lookup case-insensitive; want case-sensitive")
	}
	if got := st.Find(SymbolTypeServerScript(tg), "PascalCase"); got == nil {
		t.Fatal("server-script exact-case lookup missed")
	}
}

func TestSymbolTable_SatisfiesAstSymbolTableRef(t *testing.T) {
	st := NewSymbolTable(nil)
	var _ astSymbolTableRef = st
}

type astSymbolTableRef interface {
	AsSymbolTableRef()
}
```

### Step 2: Run tests to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/symbol/...
```

Expected: build failure across all symbols.

### Step 3: Write the implementations

```go
// pkg/pack/compiler/symbol/symbol.go
package symbol

import (
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// Symbol is the marker interface for every kind of symbol the compiler
// tracks. Mirrors TS RuneScriptSymbol interface.
type Symbol interface {
	SymbolName() string
	AsSymbolRef() // ast.SymbolRef satisfaction
}

// LocalVariableSymbol represents a script-local variable / parameter.
// Mirrors TS LocalVariableSymbol.
type LocalVariableSymbol struct {
	Name string
	Type typ.Type
}

func (s *LocalVariableSymbol) SymbolName() string { return s.Name }
func (*LocalVariableSymbol) AsSymbolRef()         {}

// BasicSymbol represents a top-level named object (npc / loc / obj / etc.).
// IsProtected gates write access in TypeChecking. Mirrors TS BasicSymbol.
type BasicSymbol struct {
	Name        string
	Type        typ.Type
	IsProtected bool
}

func (s *BasicSymbol) SymbolName() string { return s.Name }
func (*BasicSymbol) AsSymbolRef()         {}

// ConstantSymbol represents a `^FOO = value` constant. Mirrors TS ConstantSymbol.
type ConstantSymbol struct {
	Name  string
	Value string
}

func (s *ConstantSymbol) SymbolName() string { return s.Name }
func (*ConstantSymbol) AsSymbolRef()         {}
```

```go
// pkg/pack/compiler/symbol/script.go
package symbol

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// ScriptSymbolFields is the shared field shape for ServerScriptSymbol +
// ClientScriptSymbol. TS uses subclass; goscape uses struct embedding.
//
// NAI-205-D-SCRIPTSYMBOL-NO-POINTERS: TS adds a `pointers(checker)` method
// that returns a PointerHolder. PointerChecker lives in codegen (NAI-207).
// Goscape omits the method; the field-shape carries forward.
type ScriptSymbolFields struct {
	Trigger    *trigger.TriggerType
	Name       string
	Parameters typ.Type
	Returns    typ.Type
}

// ServerScriptSymbol is a script defined with a server-side trigger (proc,
// label, opheld, etc.). Mirrors TS ServerScriptSymbol.
type ServerScriptSymbol struct {
	ScriptSymbolFields
}

func (s *ServerScriptSymbol) SymbolName() string { return s.Name }
func (*ServerScriptSymbol) AsSymbolRef()         {}
func (*ServerScriptSymbol) IsServerScript() bool { return true }

// ClientScriptSymbol is a script defined with a client-side trigger
// (only `clientscript`). Mirrors TS ClientScriptSymbol.
type ClientScriptSymbol struct {
	ScriptSymbolFields
}

func (s *ClientScriptSymbol) SymbolName() string { return s.Name }
func (*ClientScriptSymbol) AsSymbolRef()         {}
func (*ClientScriptSymbol) IsServerScript() bool { return false }
```

```go
// pkg/pack/compiler/symbol/symboltype.go
package symbol

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// SymbolKind enumerates the five categories of symbol storable in a SymbolTable.
// Mirrors TS SymbolType.ts kinds.
type SymbolKind int

const (
	SymbolKindServerScript SymbolKind = iota
	SymbolKindClientScript
	SymbolKindLocalVariable
	SymbolKindBasic
	SymbolKindConstant
)

// SymbolType is the (kind, optional-trigger-or-type) tuple used as a
// SymbolTable map key. Mirrors TS tagged-union SymbolType<T>.
//
// NAI-205-D-SYMBOLTYPE-STRING-KEY: TS interns via WeakMap+Map (identity
// equality on trigger/type instances). Goscape derives a string key from
// (Kind, Trigger.Identifier or Type.Representation) and uses that as the
// outer map key in SymbolTable. Behaviour-equivalent for the types
// ScriptRegistration actually consumes.
type SymbolType struct {
	Kind      SymbolKind
	Trigger   *trigger.TriggerType
	BasicType typ.Type
}

// Key returns the canonical string identifying this SymbolType, used as
// the outer map key in SymbolTable.
func (s SymbolType) Key() string {
	switch s.Kind {
	case SymbolKindServerScript:
		return "server:" + s.Trigger.Identifier
	case SymbolKindClientScript:
		return "client:" + s.Trigger.Identifier
	case SymbolKindLocalVariable:
		return "local"
	case SymbolKindBasic:
		return "basic:" + s.BasicType.Representation()
	case SymbolKindConstant:
		return "constant"
	}
	return "unknown"
}

// Factory functions matching TS SymbolType.serverScript(...)/etc. Each is a
// thin wrapper; goscape doesn't intern (TS WeakMap interning unnecessary
// since Key() produces the canonical string).
func SymbolTypeServerScript(t *trigger.TriggerType) SymbolType {
	return SymbolType{Kind: SymbolKindServerScript, Trigger: t}
}

func SymbolTypeClientScript(t *trigger.TriggerType) SymbolType {
	return SymbolType{Kind: SymbolKindClientScript, Trigger: t}
}

func SymbolTypeLocalVariable() SymbolType {
	return SymbolType{Kind: SymbolKindLocalVariable}
}

func SymbolTypeBasic(t typ.Type) SymbolType {
	return SymbolType{Kind: SymbolKindBasic, BasicType: t}
}

func SymbolTypeConstant() SymbolType {
	return SymbolType{Kind: SymbolKindConstant}
}
```

```go
// pkg/pack/compiler/symbol/table.go
package symbol

import "strings"

// SymbolTable is the goscape port of TS class SymbolTable.
//
// Outer map is keyed by SymbolType.Key(); inner map is keyed by
// normalise(kind, name). Normalisation lowercases + collapses whitespace
// only for Kind == Basic; all other kinds preserve the original name.
type SymbolTable struct {
	parent  *SymbolTable
	symbols map[string]map[string]Symbol
}

// NewSymbolTable returns a fresh SymbolTable optionally chained to parent.
// Mirrors TS constructor.
func NewSymbolTable(parent *SymbolTable) *SymbolTable {
	return &SymbolTable{parent: parent, symbols: map[string]map[string]Symbol{}}
}

// CreateSubTable returns a SymbolTable whose parent is this table.
// Mirrors TS `createSubTable()` (which TS exposes — verify in plan T6
// review — checked at L66-72).
func (st *SymbolTable) CreateSubTable() *SymbolTable {
	return NewSymbolTable(st)
}

func (st *SymbolTable) normalize(kind SymbolKind, name string) string {
	if kind != SymbolKindBasic {
		return name
	}
	// lowercase + collapse any run of whitespace to a single underscore.
	var b strings.Builder
	b.Grow(len(name))
	prevSpace := false
	for _, r := range name {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			if !prevSpace {
				b.WriteRune('_')
			}
			prevSpace = true
		default:
			prevSpace = false
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Insert returns true iff the symbol was inserted. Returns false if the
// symbol already exists in this table OR any parent table. Mirrors TS
// SymbolTable.insert L28-46.
func (st *SymbolTable) Insert(t SymbolType, s Symbol) bool {
	key := st.normalize(t.Kind, s.SymbolName())
	outerKey := t.Key()

	// Walk the parent chain checking for collisions.
	for cur := st; cur != nil; cur = cur.parent {
		if inner, ok := cur.symbols[outerKey]; ok {
			if _, exists := inner[key]; exists {
				return false
			}
		}
	}

	inner, ok := st.symbols[outerKey]
	if !ok {
		inner = map[string]Symbol{}
		st.symbols[outerKey] = inner
	}
	inner[key] = s
	return true
}

// Find returns the symbol matching (t, name), walking the parent chain.
// Mirrors TS SymbolTable.find L51-58.
func (st *SymbolTable) Find(t SymbolType, name string) Symbol {
	outerKey := t.Key()
	key := st.normalize(t.Kind, name)
	for cur := st; cur != nil; cur = cur.parent {
		if inner, ok := cur.symbols[outerKey]; ok {
			if s, exists := inner[key]; exists {
				return s
			}
		}
	}
	return nil
}

// AsSymbolTableRef satisfies ast.SymbolTableRef.
func (*SymbolTable) AsSymbolTableRef() {}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/symbol/...
```

Expected: PASS.

### Step 5: Commit

```bash
git add pkg/pack/compiler/symbol/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/symbol): NAI-205 T6 — Symbol/ScriptSymbol/SymbolType/SymbolTable

Port the four symbol-package files:
- Symbol marker interface + LocalVariableSymbol, BasicSymbol, ConstantSymbol
- ScriptSymbolFields + ServerScriptSymbol, ClientScriptSymbol (no .pointers(),
  deferred per NAI-205-D-SCRIPTSYMBOL-NO-POINTERS)
- SymbolType tagged-union via string-key derivation
  (NAI-205-D-SYMBOLTYPE-STRING-KEY)
- SymbolTable: Insert/Find with parent-chain walk; Basic-kind lowercase +
  whitespace→_ name normalisation

All concrete symbols + SymbolTable satisfy ast.SymbolRef / .SymbolTableRef.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `pkg/pack/compiler/diagnostics/` — DiagnosticType, Diagnostic, Diagnostics, Handler, Messages, ReportAt

**Spec ref:** §6.1.

**Files:**
- Create: `pkg/pack/compiler/diagnostics/type.go`
- Create: `pkg/pack/compiler/diagnostics/diagnostic.go`
- Create: `pkg/pack/compiler/diagnostics/diagnostics.go`
- Create: `pkg/pack/compiler/diagnostics/handler.go`
- Create: `pkg/pack/compiler/diagnostics/messages.go`
- Create: `pkg/pack/compiler/diagnostics/report_helpers.go`
- Create: `pkg/pack/compiler/diagnostics/diagnostic_test.go`
- Create: `pkg/pack/compiler/diagnostics/diagnostics_test.go`
- Create: `pkg/pack/compiler/diagnostics/report_helpers_test.go`

**TS source-of-truth:**
- `src/compiler/diagnostics/DiagnosticType.ts` (7 LOC, enum)
- `src/compiler/diagnostics/Diagnostic.ts` (43 LOC)
- `src/compiler/diagnostics/Diagnostics.ts` (40 LOC, container)
- `src/compiler/diagnostics/DiagnosticMessage.ts` (106 LOC, constant templates)
- `src/compiler/diagnostics/DiagnosticsHandler.ts` (147 LOC; BaseDiagnosticsHandler deferred to NAI-208)

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/diagnostics/diagnostic_test.go
package diagnostics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

func TestDiagnosticType_IsErrorTypes(t *testing.T) {
	cases := []struct {
		typ  DiagnosticType
		want bool
	}{
		{DiagnosticInfo, false},
		{DiagnosticHint, false},
		{DiagnosticWarning, false},
		{DiagnosticError, true},
		{DiagnosticSyntaxError, true},
	}
	for _, c := range cases {
		d := Diagnostic{Type: c.typ}
		if got := d.IsError(); got != c.want {
			t.Fatalf("IsError(%v) = %v, want %v", c.typ, got, c.want)
		}
	}
}

func TestNewDiagnostic_FieldShape(t *testing.T) {
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 3, Column: 4}
	d := NewDiagnostic(loc, DiagnosticError, MessageScriptTriggerInvalid, "proc")
	if d.Type != DiagnosticError {
		t.Fatalf("Type = %v, want Error", d.Type)
	}
	if d.SourceLocation != loc {
		t.Fatalf("SourceLocation = %+v, want %+v", d.SourceLocation, loc)
	}
	if d.Message != MessageScriptTriggerInvalid {
		t.Fatalf("Message = %q, want template constant", d.Message)
	}
	if len(d.MessageArgs) != 1 || d.MessageArgs[0] != "proc" {
		t.Fatalf("MessageArgs = %v, want [\"proc\"]", d.MessageArgs)
	}
}
```

```go
// pkg/pack/compiler/diagnostics/diagnostics_test.go
package diagnostics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

func TestDiagnostics_ReportAndList(t *testing.T) {
	d := &Diagnostics{}
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 1}
	d.Report(NewDiagnostic(loc, DiagnosticInfo, MessageGenericInvalidType, "x"))
	d.Report(NewDiagnostic(loc, DiagnosticError, MessageGenericInvalidType, "y"))

	got := d.List()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !d.HasErrors() {
		t.Fatal("HasErrors() = false, want true (Error is in the list)")
	}
}

func TestDiagnostics_NoErrors(t *testing.T) {
	d := &Diagnostics{}
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 1}
	d.Report(NewDiagnostic(loc, DiagnosticInfo, MessageGenericInvalidType, "x"))
	d.Report(NewDiagnostic(loc, DiagnosticWarning, MessageGenericInvalidType, "y"))
	if d.HasErrors() {
		t.Fatal("HasErrors() = true, want false (no Error or SyntaxError)")
	}
}
```

```go
// pkg/pack/compiler/diagnostics/report_helpers_test.go
package diagnostics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

func TestReportErrorAt_NodeLocation(t *testing.T) {
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 5, Column: 2}
	node := &ast.Identifier{SrcLoc: loc, Text: "foo"}
	d := &Diagnostics{}
	ReportErrorAt(d, node, MessageScriptTriggerInvalid, "proc")
	list := d.List()
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0].Type != DiagnosticError {
		t.Fatalf("Type = %v, want Error", list[0].Type)
	}
	if list[0].SourceLocation != loc {
		t.Fatalf("SourceLocation = %+v, want %+v", list[0].SourceLocation, loc)
	}
}

func TestReportAt_PreservesType(t *testing.T) {
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 1}
	node := &ast.Token{SrcLoc: loc, Text: "x"}
	d := &Diagnostics{}
	ReportAt(d, node, DiagnosticWarning, MessageGenericInvalidType, "x")
	if list := d.List(); len(list) != 1 || list[0].Type != DiagnosticWarning {
		t.Fatalf("list = %+v", list)
	}
}
```

### Step 2: Run tests to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/diagnostics/...
```

Expected: build failure.

### Step 3: Write the implementations

```go
// pkg/pack/compiler/diagnostics/type.go
package diagnostics

// DiagnosticType discriminates between info/hint/warning/error severities.
// Mirrors TS DiagnosticType.ts (string enum); goscape uses int + String().
type DiagnosticType int

const (
	DiagnosticInfo DiagnosticType = iota
	DiagnosticHint
	DiagnosticWarning
	DiagnosticError
	DiagnosticSyntaxError
)

func (t DiagnosticType) String() string {
	switch t {
	case DiagnosticInfo:
		return "INFO"
	case DiagnosticHint:
		return "HINT"
	case DiagnosticWarning:
		return "WARNING"
	case DiagnosticError:
		return "ERROR"
	case DiagnosticSyntaxError:
		return "SYNTAX_ERROR"
	}
	return "UNKNOWN"
}
```

```go
// pkg/pack/compiler/diagnostics/diagnostic.go
package diagnostics

import "github.com/zsrv/goscape/pkg/pack/compiler/lexer"

// Diagnostic is one entry reported during a compilation step.
// Mirrors TS Diagnostic.ts.
type Diagnostic struct {
	Type           DiagnosticType
	SourceLocation lexer.NodeSourceLocation
	Message        string
	MessageArgs    []any
}

// NewDiagnostic constructs a Diagnostic from a known location.
func NewDiagnostic(loc lexer.NodeSourceLocation, t DiagnosticType, msg string, args ...any) Diagnostic {
	return Diagnostic{
		Type:           t,
		SourceLocation: loc,
		Message:        msg,
		MessageArgs:    args,
	}
}

// IsError reports whether this diagnostic is an Error or SyntaxError.
// Mirrors TS Diagnostic.isError().
func (d Diagnostic) IsError() bool {
	return d.Type == DiagnosticError || d.Type == DiagnosticSyntaxError
}
```

```go
// pkg/pack/compiler/diagnostics/diagnostics.go
package diagnostics

// Diagnostics is the accumulator for one compilation step.
// Mirrors TS Diagnostics class.
type Diagnostics struct {
	entries []Diagnostic
}

// Report appends a diagnostic to the list.
func (d *Diagnostics) Report(diag Diagnostic) {
	d.entries = append(d.entries, diag)
}

// List returns the accumulated diagnostics (read-only; no defensive copy).
// Mirrors TS getter `Diagnostics.diagnostics`.
func (d *Diagnostics) List() []Diagnostic {
	return d.entries
}

// HasErrors reports whether any reported diagnostic is an Error or SyntaxError.
// Mirrors TS Diagnostics.hasErrors L33-38.
func (d *Diagnostics) HasErrors() bool {
	for _, e := range d.entries {
		if e.IsError() {
			return true
		}
	}
	return false
}
```

```go
// pkg/pack/compiler/diagnostics/handler.go
package diagnostics

// Handler hooks into each compilation step's diagnostic stream.
// Mirrors TS DiagnosticsHandler interface.
//
// NAI-205-D-HANDLER-REQUIRED-METHODS: TS optional methods (?:) collapse to
// goscape's interface-with-NopHandler. Every Handler must implement all four;
// NopHandler eats them. BaseDiagnosticsHandler (file-reading + stdout +
// process.exit) is deferred to NAI-208 driver.
type Handler interface {
	HandleParse(*Diagnostics)
	HandleTypeChecking(*Diagnostics)
	HandleCodeGeneration(*Diagnostics)
	HandlePointerChecking(*Diagnostics)
}

// NopHandler implements Handler with all four methods as no-ops. Test
// callers inject this when they don't care about diagnostic dispatch.
type NopHandler struct{}

func (NopHandler) HandleParse(*Diagnostics)            {}
func (NopHandler) HandleTypeChecking(*Diagnostics)     {}
func (NopHandler) HandleCodeGeneration(*Diagnostics)   {}
func (NopHandler) HandlePointerChecking(*Diagnostics)  {}
```

```go
// pkg/pack/compiler/diagnostics/messages.go
package diagnostics

// DiagnosticMessage templates ported verbatim from TS
// src/compiler/diagnostics/DiagnosticMessage.ts. Format strings preserved
// char-for-char (%s placeholders, punctuation, period at end).
//
// Tests assert on (template constant, args slice) — not formatted output —
// to stay deterministic across fmt.Sprintf evolution.

const (
	// Internal compiler errors
	MessageUnsupportedSymbolTypeToType = "Internal compiler error: Unsupported SymbolType -> Type conversion: %s"
	MessageCaseWithoutSwitch           = "Internal compiler error: Case without switch statement as parent."
	MessageReturnOrphan                = "Internal compiler error: Orphaned `return` statement, no parent `script` node found."
	MessageTriggerTypeNotFound         = "Internal compiler error: The trigger '%s' has no declaration."

	// Custom command handler errors
	MessageCustomHandlerNoType   = "Internal compiler error: Custom command handler did not assign return type."
	MessageCustomHandlerNoSymbol = "Internal compiler error: Custom command handler did not assign symbol."

	// Code gen internal compiler errors
	MessageSymbolIsNull         = "Internal compiler error: Symbol has not been defined for the node."
	MessageTypeHasNoBaseType    = "Internal compiler error: Type has no defined base type: %s."
	MessageTypeHasNoDefault     = "Internal compiler error: Return type '%s' has no defined default value."
	MessageInvalidCondition     = "Internal compiler error: %s is not a supported expression type for conditions."
	MessageNullConstant         = "Internal compiler error: %s evaluated to 'null' constant value."
	MessageExpressionNoSubExpr  = "Internal compiler error: No sub expression node."

	// Node type agnostic
	MessageGenericInvalidType      = "'%s' is not a valid type."
	MessageGenericTypeMismatch     = "Type mismatch: '%s' was given but '%s' was expected."
	MessageGenericUnresolvedSymbol = "'%s' could not be resolved to a symbol."
	MessageArithmeticInvalidType   = "Type mismatch: '%s' was given but 'int' or 'long' was expected."

	// Script node specific
	MessageScriptRedeclaration            = "[%s,%s] is already defined."
	MessageScriptLocalRedeclaration       = "'$%s' is already defined."
	MessageScriptTriggerInvalid           = "'%s' is not a valid trigger type."
	MessageScriptCommandOnly              = "Using a '*' is only allowed for commands."
	MessageScriptTriggerNoParameters      = "The trigger type '%s' is not allowed to have parameters defined."
	MessageScriptTriggerExpectedParameters = "The trigger type '%s' is expected to accept (%s)."
	MessageScriptTriggerNoReturns         = "The trigger type '%s' is not allowed to return values."
	MessageScriptTriggerExpectedReturns   = "The trigger type '%s' is expected to return (%s)."
	MessageScriptSubjectOnlyGlobal        = "Trigger '%s' only allows global subjects."
	MessageScriptSubjectNoGlobal          = "Trigger '%s' does not allow global subjects."
	MessageScriptSubjectNoCategory        = "Trigger '%s' does not allow category subjects."
	MessageScriptSubjectNoSpaces          = "Trigger '%s' does not allow spaces in subjects."

	// Switch statement
	MessageSwitchInvalidType        = "'%s' is not allowed within a switch statement."
	MessageSwitchDuplicateDefault   = "Duplicate default label."
	MessageSwitchCaseNotConstant    = "Switch case value is not a constant expression."

	// Assignment
	MessageAssignMultiArray = "Arrays are not allowed in multi-assignment statements."

	// Expression statement
	MessageExpressionStatementNoSideEffect = "Value is discarded."

	// Condition
	MessageConditionInvalidNodeType = "Conditions are only allowed to be binary expressions."
	MessageConditionNotValid        = "Condition is not valid."

	// Binary expr
	MessageBinopInvalidTypes = "Operator '%s' cannot be applied to '%s', '%s'."
	MessageBinopTupleType    = "%s side of binary expressions can only have one type but has '%s'."

	// Call expr
	MessageCommandReferenceUnresolved      = "'%s' cannot be resolved to a command."
	MessageCommandNoArgsExpected           = "'%s' is expected to have no arguments but has '%s'."
	MessageProcReferenceUnresolved         = "'~%s' cannot be resolved to a proc."
	MessageProcNoArgsExpected              = "'~%s' is expected to have no arguments but has '%s'."
	MessageJumpReferenceUnresolved         = "'@%s' cannot be resolved to a label."
	MessageJumpNoArgsExpected              = "'@%s' is expected to have no arguments but has '%s'."
	MessageClientScriptReferenceUnresolved = "'%s' cannot be resolved to a clientscript."
	MessageClientScriptNoArgsExpected      = "'%s' is expected to have no arguments but has '%s'."
	MessageHookTransmitListUnexpected      = "Unexpected hook transmit list."

	// Local
	MessageLocalDeclarationInvalidType  = "'%s' is not allowed to be declared as a type."
	MessageLocalParameterInvalidType    = "'%s' is not allowed to be used as a parameter."
	MessageLocalReferenceUnresolved     = "'$%s' cannot be resolved to a local variable."
	MessageLocalReferenceNotArray       = "Access of indexed value of non-array type variable '$%s'."
	MessageLocalArrayInvalidType        = "'%s' is not allowed to be used as an array."
	MessageLocalArrayReferenceNoIndex   = "'$%s' is a reference to an array variable without specifying the index."

	// Game var
	MessageGameReferenceUnresolved = "'%%%s' cannot be resolved to a game variable."

	// Constant
	MessageConstantReferenceUnresolved = "'^%s' cannot be resolved to a constant."
	MessageConstantCyclicRef           = "Cyclic constant references are not permitted: %s."
	MessageConstantUnknownType         = "Unable to infer type for '^%s'."
	MessageConstantParseError          = "Unable to parse constant value of '%s' into type '%s'."
	MessageConstantNonConstant         = "Constant value of '%s' evaluated to a non-constant expression."

	// Feature flag
	MessageFeatureDisabledTrigger    = "Trigger '%s' is disabled."
	MessageFeatureDisabledCommand    = "Command '%s' is disabled."
	MessageFeatureDisabledType       = "Type '%s' is disabled."
	MessageFeatureDisabledLocal      = "Local variables are disabled."
	MessageFeatureDisabledBoolean    = "Boolean usage is disabled."
	MessageFeatureDisabledOperator   = "Operator '%s' is disabled."
	MessageFeatureDisabledCalc       = "calc(...) usage is disabled."
	MessageLocalDeclarationNotTopLevel = "Local variables may only be declared at the top level of a script."

	// Pointer
	MessagePointerUninitialized = "Attempt to access uninitialized pointer %s."
	MessagePointerCorrupted     = "Attempt to access corrupted pointer %s."
	MessagePointerCorruptedLoc  = "%s corrupted here."
	MessagePointerRequiredLoc   = "%s required here."

	// Mapzone / zone parse — TS uses inline string literals at ScriptRegistration L294/300/304/326/333/339; expose as constants for consistency
	MessageMapzoneSubjectForm        = "Mapzone subject must be of the form: 'level_mx_mz'."
	MessageMapzoneInvalidCoord       = "Invalid mapzone coord."
	MessageMapzoneOnlyLevelZero      = "Mapzone affect all level, just specify '0'."
	MessageZoneSubjectForm           = "Zone subject must be of the form: 'level_mx_mz_lx_lz'."
	MessageZoneInvalidCoord          = "Invalid zone coord."
	MessageZoneLocalCoordMultipleOf8 = "Local zone coord must be a multiple of 8"
)
```

```go
// pkg/pack/compiler/diagnostics/report_helpers.go
package diagnostics

import "github.com/zsrv/goscape/pkg/pack/compiler/ast"

// NAI-205-D-NO-NODE-REPORT-ERROR: TS adds a `reportError` method to every
// Node. Goscape avoids ast → diagnostics import by routing through these
// helpers in the diagnostics package, which accept an ast.Node and pull
// out its NodeSourceLocation via the Source() method.

// ReportAt appends a Diagnostic with the given type, using node.Source() for
// location. Mirrors TS Node.reportError + the type-routing layer above it.
func ReportAt(d *Diagnostics, node ast.Node, t DiagnosticType, msg string, args ...any) {
	d.Report(NewDiagnostic(node.Source(), t, msg, args...))
}

// ReportErrorAt is shorthand for ReportAt with DiagnosticError. Most TS
// `reportError(msg, args...)` call sites map to this.
func ReportErrorAt(d *Diagnostics, node ast.Node, msg string, args ...any) {
	ReportAt(d, node, DiagnosticError, msg, args...)
}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/diagnostics/...
```

Expected: PASS.

### Step 5: Commit

```bash
git add pkg/pack/compiler/diagnostics/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/diagnostics): NAI-205 T7 — Diagnostic + Diagnostics + Handler + Messages + ReportAt

Port the diagnostics-package surface:
- DiagnosticType (Info/Hint/Warning/Error/SyntaxError)
- Diagnostic struct + IsError + NewDiagnostic
- Diagnostics container (Report/List/HasErrors)
- Handler interface + NopHandler (NAI-205-D-HANDLER-REQUIRED-METHODS)
- ~50 MessageXxx template constants (verbatim TS port)
- ReportAt / ReportErrorAt helpers (NAI-205-D-NO-NODE-REPORT-ERROR)

BaseDiagnosticsHandler (file-reading + stdout + process.exit) deferred
to NAI-208 driver.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `pkg/pack/compiler/ast/scriptfile.go` — add seven fields + narrow deviation tag

**Spec ref:** §6.6.

**Files:**
- Modify: `pkg/pack/compiler/ast/scriptfile.go` (add fields + narrow doc-comment)
- Create: `pkg/pack/compiler/ast/narrowed_deviation_pin_test.go`

### Step 1: Write the failing test

```go
// pkg/pack/compiler/ast/narrowed_deviation_pin_test.go
package ast

import (
	"os"
	"strings"
	"testing"
)

// TestPin_NarrowedNAI204DAstNoTypeFields pins that the
// NAI-204-D-AST-NO-TYPE-FIELDS doc-comment in scriptfile.go has been narrowed
// to mention NAI-206 explicitly. Removing the NAI-206 mention regresses the
// "scope of remaining deferral" contract that NAI-205 establishes.
func TestPin_NarrowedNAI204DAstNoTypeFields(t *testing.T) {
	b, err := os.ReadFile("scriptfile.go")
	if err != nil {
		t.Fatalf("read scriptfile.go: %v", err)
	}
	src := string(b)
	if !strings.Contains(src, "NAI-204-D-AST-NO-TYPE-FIELDS") {
		t.Fatal("scriptfile.go missing NAI-204-D-AST-NO-TYPE-FIELDS tag")
	}
	if !strings.Contains(src, "NAI-206") {
		t.Fatal("NAI-204-D-AST-NO-TYPE-FIELDS tag does not mention NAI-206 — the deviation should now reference its retirement slice")
	}
}

func TestScript_NewFieldsExist(t *testing.T) {
	// Compile-only structural check: the seven new fields must be addressable
	// at the Script and Parameter types. Tests will set them via test setters
	// in T9-T13.
	s := &Script{}
	_ = s.TriggerType
	_ = s.Symbol
	_ = s.Block
	_ = s.ParameterType
	_ = s.ReturnType
	_ = s.SubjectReference

	p := &Parameter{}
	_ = p.Symbol
}
```

### Step 2: Run test to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/ast/...
```

Expected: build failure (fields don't exist) + tag-pin failure (no NAI-206 mention).

### Step 3: Modify scriptfile.go

Open `pkg/pack/compiler/ast/scriptfile.go`. Replace the `Script` struct definition (lines 22-36 approx; verify before edit) and the `Parameter` struct definition.

Find:

```go
// Script is a single `[trigger,name] params returns statements*` block.
// Mirrors TS src/parser/ast/Scripts.ts.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Script.symbol, .block, .returnType,
// .triggerType, .subjectReference, .parameterType fields are NAI-205-owned
// and not present here.
type Script struct {
	SrcLoc       lexer.NodeSourceLocation
	Trigger      *Identifier
	Name         *Identifier
	IsStar       bool
	Parameters   []*Parameter // nil if header had no parameter list
	ReturnTokens []*Token     // nil if header had no return-type list
	Statements   []Statement
}
```

Replace with:

```go
// Script is a single `[trigger,name] params returns statements*` block.
// Mirrors TS src/parser/ast/Scripts.ts.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Script.symbol, .block, .returnType,
// .triggerType, .subjectReference, .parameterType landed in NAI-205 (this
// file). The remaining TypeChecking-owned fields (.defaultCase/.type on
// SwitchStatement, .symbol on Declaration*/CallExpression, .reference on
// Identifier/Literal/VariableExpression, .subExpression on
// ConstantVariableExpression/StringLiteral) are NAI-206-owned.
type Script struct {
	SrcLoc       lexer.NodeSourceLocation
	Trigger      *Identifier
	Name         *Identifier
	IsStar       bool
	Parameters   []*Parameter // nil if header had no parameter list
	ReturnTokens []*Token     // nil if header had no return-type list
	Statements   []Statement

	// NAI-205-populated fields (lifted from NAI-204-D-AST-NO-TYPE-FIELDS).
	// Set by pkg/pack/compiler/semantics.ScriptRegistration.

	// TriggerType is the resolved trigger; nil if trigger lookup failed
	// during ScriptRegistration. Concrete type: *trigger.TriggerType.
	TriggerType TriggerRef

	// Symbol is the ServerScriptSymbol inserted into the root SymbolTable
	// for this script; nil if the insert failed (redeclaration).
	// Concrete type: *symbol.ServerScriptSymbol.
	Symbol SymbolRef

	// Block is the per-script local SymbolTable holding parameter symbols.
	// nil before ScriptRegistration runs. Concrete type: *symbol.SymbolTable.
	Block SymbolTableRef

	// ParameterType is the TupleType (or MetaUnit for no params, or single
	// param's type) summarising the parameter list. Concrete type: type.Type.
	ParameterType TypeRef

	// ReturnType mirrors ParameterType for the returns list. Concrete type:
	// type.Type.
	ReturnType TypeRef

	// SubjectReference is the BasicSymbol resolved for type/category subjects;
	// nil for global (`_`) subjects or unresolved references.
	// Concrete type: *symbol.BasicSymbol.
	SubjectReference SymbolRef
}
```

Find:

```go
// Parameter is one `type DOLLAR advancedIdentifier` in a script header.
// Mirrors TS src/parser/ast/Parameter.ts.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Parameter.symbol is NAI-205-owned.
type Parameter struct {
	SrcLoc    lexer.NodeSourceLocation
	TypeToken *Token
	Name      *Identifier
}
```

Replace with:

```go
// Parameter is one `type DOLLAR advancedIdentifier` in a script header.
// Mirrors TS src/parser/ast/Parameter.ts.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Parameter.symbol landed in NAI-205
// (Symbol field below). No further Parameter fields are deferred to NAI-206.
type Parameter struct {
	SrcLoc    lexer.NodeSourceLocation
	TypeToken *Token
	Name      *Identifier

	// Symbol is the LocalVariableSymbol inserted into the script's Block
	// table for this parameter. nil before ScriptRegistration runs.
	// Concrete type: *symbol.LocalVariableSymbol.
	Symbol SymbolRef
}
```

Also update the `Expression` and `Identifier` doc-comments in the same file:

Find:

```go
// Expression marks nodes that produce a value (mirrors TS Expression
// base class). NAI-204-D-AST-NO-TYPE-FIELDS: TS Expression.type and
// Expression.typeHint are not modeled here — NAI-205 adds them.
```

Replace with:

```go
// Expression marks nodes that produce a value (mirrors TS Expression
// base class). NAI-204-D-AST-NO-TYPE-FIELDS: TS Expression.type and
// Expression.typeHint remain absent — NAI-206 (TypeChecking) adds them.
```

And the `Identifier` block (in `expressions.go` or `scriptfile.go` — verify location before edit). Find the existing tag-bearing doc-comment for Identifier and update to mention NAI-206 instead of NAI-205.

If `Identifier` lives in `scriptfile.go`:

Find:

```go
// Identifier is an identifier expression. Mirrors TS
// src/parser/ast/expr/Identifier.ts. Implements Expression — used both
// for bare identifiers and as a sub-node for variable/call names.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Identifier.reference is NAI-205-owned.
type Identifier struct {
```

Replace with:

```go
// Identifier is an identifier expression. Mirrors TS
// src/parser/ast/expr/Identifier.ts. Implements Expression — used both
// for bare identifiers and as a sub-node for variable/call names.
//
// NAI-204-D-AST-NO-TYPE-FIELDS: TS Identifier.reference is NAI-206-owned
// (lifted by TypeChecking; NAI-205 doesn't write to Identifier).
type Identifier struct {
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: PASS for `ast/...` including NAI-204 deviation pins (still find the tag) and the new NAI-205 narrowing pin.

### Step 5: Commit

```bash
git add pkg/pack/compiler/ast/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/ast): NAI-205 T8 — Script/Parameter NAI-205 fields + narrowed deviation

Add the seven AST fields ScriptRegistration writes:
- Script.TriggerType (TriggerRef)
- Script.Symbol (SymbolRef)
- Script.Block (SymbolTableRef)
- Script.ParameterType (TypeRef)
- Script.ReturnType (TypeRef)
- Script.SubjectReference (SymbolRef)
- Parameter.Symbol (SymbolRef)

All fields typed via the marker interfaces from T1; concrete types
plug in via structural satisfaction without import cycles.

Narrow NAI-204-D-AST-NO-TYPE-FIELDS doc-comments on Script,
Parameter, Expression, Identifier to point at NAI-206 as the
retirement slice. Add narrowed-tag pin test to lock in the narrowing.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `pkg/pack/compiler/semantics/` — StrictFeatureLevel + ScriptRegistration constructor/Visit/scoped-table

**Spec ref:** §6.5, §5.5.

**Files:**
- Create: `pkg/pack/compiler/semantics/strict_feature.go`
- Create: `pkg/pack/compiler/semantics/script_registration.go`
- Create: `pkg/pack/compiler/semantics/strict_feature_test.go`
- Create: `pkg/pack/compiler/semantics/registration_skeleton_test.go`

**TS source-of-truth:**
- `src/compiler/StrictFeatureLevel.ts` (small, ~30 LOC inline-checked from imports — port the partial-record shape)
- `src/compiler/semantics/ScriptRegistration.ts` L36-94 (constructor, table stack, createScopedTable, visitScriptFile)

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/semantics/strict_feature_test.go
package semantics

import "testing"

func TestStrictFeatureLevel_ZeroValue_AllEnabled(t *testing.T) {
	// Zero value = nothing disabled. ScriptRegistration treats every
	// `f.DisableX` as false.
	f := StrictFeatureLevel{}
	if f.DisableProcs || f.DisableEnums || f.DisableStructs || f.DisableDBTables || f.DisableBooleans {
		t.Fatalf("zero value has a Disable* set: %+v", f)
	}
}
```

```go
// pkg/pack/compiler/semantics/registration_skeleton_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// newTestFixture returns the four-tuple every ScriptRegistration test wires
// up. Type+trigger managers are minimal (no triggers/types registered);
// callers add what their test needs.
func newTestFixture(t *testing.T) (*typ.TypeManager, *trigger.TriggerManager, *symbol.SymbolTable, *diagnostics.Diagnostics) {
	t.Helper()
	return typ.NewTypeManager(), trigger.NewTriggerManager(), symbol.NewSymbolTable(nil), &diagnostics.Diagnostics{}
}

func TestScriptRegistration_NewVisit_Empty(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	sr.Visit(&ast.ScriptFile{}) // no scripts → no panic, no diagnostics

	if got := d.List(); len(got) != 0 {
		t.Fatalf("diagnostics for empty file: %+v", got)
	}
}

func TestScriptRegistration_ScopedTable_PushPop(t *testing.T) {
	// Each visited script gets a fresh sub-table. After visit, the stack
	// returns to its original depth.
	tm, trm, root, d := newTestFixture(t)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	if got := sr.tableStackDepth(); got != 1 {
		t.Fatalf("initial table-stack depth = %d, want 1 (file-level table)", got)
	}
	// withScopedTable should push, run, and pop back to 1.
	sr.withScopedTable(func() {
		if got := sr.tableStackDepth(); got != 2 {
			t.Fatalf("inside scoped block: depth = %d, want 2", got)
		}
	})
	if got := sr.tableStackDepth(); got != 1 {
		t.Fatalf("after scoped block: depth = %d, want 1", got)
	}
}
```

### Step 2: Run tests to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/semantics/...
```

Expected: build failure on `StrictFeatureLevel`, `NewScriptRegistration`, etc.

### Step 3: Write the skeleton implementation

```go
// pkg/pack/compiler/semantics/strict_feature.go
package semantics

// StrictFeatureLevel toggles feature-disabling at compile time.
// Mirrors TS src/compiler/StrictFeatureLevel.ts.
//
// NAI-205-D-STRICT-INVERTED-POLARITY: TS uses `{ procs?: boolean }`
// where missing-key = enabled (idiomatic in TS); goscape flips polarity
// to `DisableX bool` so the zero value (== TS empty record) corresponds
// to "nothing disabled". If you add fields, name them `DisableX`,
// NEVER `EnableX` — flipping back regresses test fixtures silently.
type StrictFeatureLevel struct {
	DisableProcs    bool // TS features.procs === false
	DisableEnums    bool
	DisableStructs  bool
	DisableDBTables bool
	DisableBooleans bool
}
```

```go
// pkg/pack/compiler/semantics/script_registration.go
package semantics

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// ScriptRegistration is the first-pass semantic walker. It registers each
// script's ServerScriptSymbol into the passed-in root SymbolTable and
// writes seven fields onto each Script + one field onto each Parameter.
// Mirrors TS src/compiler/semantics/ScriptRegistration.ts.
//
// Two-stage compiler pipeline:
//   1. ScriptRegistration (THIS) — register script symbols + parameter symbols.
//   2. TypeChecking (NAI-206) — walk expressions/statements, resolve
//      references, type-check operands, populate the remaining AST fields.
//
// NAI-205-D-NO-VISIT-BLOCK: TS ScriptRegistration calls accept(this) on
// Script.Statements (block walk; no-ops via AstVisitor base class).
// Goscape skips the walk entirely since the no-op visit has no observable
// effect on the seven AST fields ScriptRegistration writes.
type ScriptRegistration struct {
	typeManager    *typ.TypeManager
	triggerManager *trigger.TriggerManager
	rootTable      *symbol.SymbolTable
	diagnostics    *diagnostics.Diagnostics
	features       StrictFeatureLevel

	// Stack of nested SymbolTables; tables[0] is the active table.
	// Mirrors TS `private readonly tables: SymbolTable[]`.
	tables []*symbol.SymbolTable

	// Cached lookup for the `category` type, used for category-subject
	// resolution. nil if the TypeManager has no 'category' type registered.
	categoryType typ.Type
}

// NewScriptRegistration constructs a ScriptRegistration walker.
// Mirrors TS ScriptRegistration constructor L52-62.
func NewScriptRegistration(
	tm *typ.TypeManager,
	trm *trigger.TriggerManager,
	rootTable *symbol.SymbolTable,
	d *diagnostics.Diagnostics,
	features StrictFeatureLevel,
) *ScriptRegistration {
	sr := &ScriptRegistration{
		typeManager:    tm,
		triggerManager: trm,
		rootTable:      rootTable,
		diagnostics:    d,
		features:       features,
		categoryType:   tm.FindOrNil("category", false),
	}
	// Push the file-level table (the TS constructor's `tables.unshift(...)`).
	sr.tables = []*symbol.SymbolTable{rootTable.CreateSubTable()}
	return sr
}

// activeTable returns the SymbolTable at the top of the stack.
// Mirrors TS `private get table()`.
func (sr *ScriptRegistration) activeTable() *symbol.SymbolTable {
	return sr.tables[0]
}

// tableStackDepth is a test-only helper.
func (sr *ScriptRegistration) tableStackDepth() int {
	return len(sr.tables)
}

// withScopedTable runs block with a fresh sub-table at the top of the stack.
// Mirrors TS `createScopedTable(block)` L78-86.
func (sr *ScriptRegistration) withScopedTable(block func()) {
	sub := sr.activeTable().CreateSubTable()
	sr.tables = append([]*symbol.SymbolTable{sub}, sr.tables...)
	defer func() {
		sr.tables = sr.tables[1:]
	}()
	block()
}

// Visit is the public entry. Mirrors TS visitScriptFile L88-94.
func (sr *ScriptRegistration) Visit(file *ast.ScriptFile) {
	for _, script := range file.Scripts {
		sr.withScopedTable(func() {
			sr.visitScript(script)
		})
	}
}

// visitScript is the per-script walker. Skeleton — implementation lands
// in T10/T11/T12.
func (sr *ScriptRegistration) visitScript(script *ast.Script) {
	// T10 fills this in.
	_ = script
}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/semantics/...
```

Expected: PASS.

### Step 5: Commit

```bash
git add pkg/pack/compiler/semantics/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/semantics): NAI-205 T9 — StrictFeatureLevel + ScriptRegistration skeleton

Land the StrictFeatureLevel value type (NAI-205-D-STRICT-INVERTED-POLARITY,
Disable* fields so zero-value == TS empty record) and the ScriptRegistration
skeleton: constructor, Visit entrypoint, withScopedTable helper, table-stack
maintenance. visitScript is empty pending T10.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: `semantics/` — visitScript core (trigger lookup, star check, return type, symbol insert)

**Spec ref:** §6.5 — TS line range L96-180. Drives Script.TriggerType, Script.Symbol, Script.Block, Script.ParameterType, Script.ReturnType.

**Files:**
- Modify: `pkg/pack/compiler/semantics/script_registration.go` (replace visitScript body + add helpers)
- Create: `pkg/pack/compiler/semantics/script_registration_test.go`

**TS reference:**
- `ScriptRegistration.ts` L96-180 (visitScript)

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/semantics/script_registration_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// makeProcTrigger constructs a minimal proc trigger that allows
// parameters + returns and uses ModeName subject mode.
func makeProcTrigger() *trigger.TriggerType {
	return &trigger.TriggerType{
		ID:              0,
		Identifier:      "proc",
		SubjectMode:     trigger.ModeName,
		AllowParameters: true,
		Parameters:      nil,
		AllowReturns:    true,
		Returns:         nil,
	}
}

// makeLabelTrigger constructs a minimal label trigger that allows
// parameters but NOT returns.
func makeLabelTrigger() *trigger.TriggerType {
	return &trigger.TriggerType{
		ID:              1,
		Identifier:      "label",
		SubjectMode:     trigger.ModeName,
		AllowParameters: true,
		Parameters:      nil,
		AllowReturns:    false,
		Returns:         nil,
	}
}

// scriptFor builds an *ast.Script with the named trigger/name and no params/returns.
func scriptFor(trig, name string) *ast.Script {
	loc := lexer.NodeSourceLocation{Name: "<test>", Line: 1}
	return &ast.Script{
		SrcLoc:  loc,
		Trigger: &ast.Identifier{SrcLoc: loc, Text: trig},
		Name:    &ast.Identifier{SrcLoc: loc, Text: name},
	}
}

func TestVisitScript_TriggerInvalid_ReportsError(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("nope", "foo")

	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if s.TriggerType != nil {
		t.Fatalf("TriggerType = %v, want nil (lookup failed)", s.TriggerType)
	}
	if !d.HasErrors() {
		t.Fatal("HasErrors = false, want true (SCRIPT_TRIGGER_INVALID)")
	}
	list := d.List()
	found := false
	for _, e := range list {
		if e.Message == diagnostics.MessageScriptTriggerInvalid {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_INVALID diagnostic; got %+v", list)
	}
}

func TestVisitScript_HappyPath_RegistersSymbol(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	if err := trm.RegisterTrigger(proc); err != nil {
		t.Fatal(err)
	}

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if len(d.List()) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}
	gotTrig, ok := s.TriggerType.(*trigger.TriggerType)
	if !ok || gotTrig != proc {
		t.Fatalf("TriggerType = %v (concrete %T), want proc", s.TriggerType, s.TriggerType)
	}
	if s.Symbol == nil {
		t.Fatal("Symbol nil, want ServerScriptSymbol")
	}
	if s.Block == nil {
		t.Fatal("Block nil, want SymbolTable")
	}
	if s.ParameterType == nil {
		t.Fatal("ParameterType nil")
	}
	if s.ReturnType == nil {
		t.Fatal("ReturnType nil")
	}
	// Root table contains the ServerScriptSymbol under (server:proc, "foo").
	got := root.Find(symbol.SymbolTypeServerScript(proc), "foo")
	if got == nil {
		t.Fatal("root table missing ServerScriptSymbol after register")
	}
}

func TestVisitScript_StarOnNonCommand_ReportsError(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	s.IsStar = true
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptCommandOnly {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_COMMAND_ONLY diagnostic; got %+v", d.List())
	}
}

func TestVisitScript_Redeclaration(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	a := scriptFor("proc", "foo")
	b := scriptFor("proc", "foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{a, b}})

	// First one set Symbol; second one didn't.
	if a.Symbol == nil {
		t.Fatal("first Script.Symbol nil")
	}
	if b.Symbol != nil {
		t.Fatal("second Script.Symbol non-nil; want nil after redeclaration")
	}

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptRedeclaration {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_REDECLARATION diagnostic; got %+v", d.List())
	}
}

func TestVisitScript_ReturnTokens_Resolved(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	_ = tm.RegisterByRepresentation(typ.PrimitiveString)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	loc := lexer.NodeSourceLocation{Name: "<test>", Line: 1}
	s := scriptFor("proc", "foo")
	s.ReturnTokens = []*ast.Token{
		{SrcLoc: loc, Text: "int"},
		{SrcLoc: loc, Text: "string"},
	}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if len(d.List()) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}
	// Two return tokens → TupleType
	if s.ReturnType == nil {
		t.Fatal("ReturnType nil")
	}
	tup, ok := s.ReturnType.(*typ.TupleType)
	if !ok {
		t.Fatalf("ReturnType = %T, want *typ.TupleType", s.ReturnType)
	}
	if len(tup.Children) != 2 || tup.Children[0] != typ.PrimitiveInt || tup.Children[1] != typ.PrimitiveString {
		t.Fatalf("ReturnType children = %v, want [int, string]", tup.Children)
	}
}

func TestVisitScript_NoReturnTokens_AllowReturns_DefaultsUnit(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger() // AllowReturns=true
	_ = trm.RegisterTrigger(proc)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if s.ReturnType != typ.MetaUnit {
		t.Fatalf("ReturnType = %v, want MetaUnit (proc allows returns; no tokens)", s.ReturnType)
	}
}

func TestVisitScript_NoReturnTokens_DisallowReturns_DefaultsNothing(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	label := makeLabelTrigger() // AllowReturns=false
	_ = trm.RegisterTrigger(label)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("label", "foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if s.ReturnType != typ.MetaNothing {
		t.Fatalf("ReturnType = %v, want MetaNothing (label disallows returns)", s.ReturnType)
	}
}

func TestVisitScript_BadReturnType_EmitsInvalidType(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	// TypeManager has nothing registered → return token "int" misses.

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	loc := lexer.NodeSourceLocation{Name: "<test>", Line: 1}
	s := scriptFor("proc", "foo")
	s.ReturnTokens = []*ast.Token{{SrcLoc: loc, Text: "int"}}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageGenericInvalidType {
			found = true
		}
	}
	if !found {
		t.Fatalf("no GENERIC_INVALID_TYPE diagnostic; got %+v", d.List())
	}
}
```

### Step 2: Run tests to confirm failure

Expected: failures across the new tests (visitScript still empty from T9).

### Step 3: Write the implementation

Replace the empty `visitScript` in `script_registration.go` (and add the supporting helpers `isDisabledTrigger`, `isDisabledTypeName`). T11 will add subject-validation helpers; T12 will add visitParameter + checkScriptParameters + checkScriptReturns.

```go
// Add to pkg/pack/compiler/semantics/script_registration.go

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"strings"
)

// isDisabledTypeName mirrors TS L52-62 — feature-flag-based disable of
// boolean / enum / struct / dbtable / dbrow / dbcolumn (+ their array forms).
func (sr *ScriptRegistration) isDisabledTypeName(typeText string) bool {
	text := strings.ToLower(typeText)
	base := text
	if strings.HasSuffix(text, "array") {
		base = text[:len(text)-5]
	}
	if sr.features.DisableBooleans && base == typ.PrimitiveBoolean.Representation() {
		return true
	}
	if sr.features.DisableEnums && base == "enum" {
		return true
	}
	if sr.features.DisableStructs && base == "struct" {
		return true
	}
	if sr.features.DisableDBTables && (base == "dbtable" || base == "dbrow" || base == "dbcolumn") {
		return true
	}
	return false
}

// isDisabledTrigger mirrors TS L64-68.
func (sr *ScriptRegistration) isDisabledTrigger(t *trigger.TriggerType) bool {
	if t == nil {
		return false
	}
	if sr.features.DisableProcs && t.Identifier == "proc" {
		return true
	}
	return false
}

// visitScript replaces the T9 stub. Mirrors TS L96-180 verbatim.
func (sr *ScriptRegistration) visitScript(script *ast.Script) {
	// L98-105: trigger lookup.
	trig := sr.triggerManager.FindOrNil(script.Trigger.Text)
	if trig == nil {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Trigger,
			diagnostics.MessageScriptTriggerInvalid, script.Trigger.Text)
	} else {
		script.TriggerType = trig
		if sr.isDisabledTrigger(trig) {
			diagnostics.ReportErrorAt(sr.diagnostics, script.Trigger,
				diagnostics.MessageFeatureDisabledTrigger, trig.Identifier)
		}
	}

	// L107-117: '*' suffix only valid on command trigger.
	if script.IsStar && trig != trigger.CommandTrigger {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageScriptCommandOnly)
	}

	// L119-120: subject validation. Implementation lives in T11.
	sr.checkScriptSubject(trig, script)

	// L122-125: visit parameters (T12 fills in visitParameter).
	for _, p := range script.Parameters {
		sr.visitParameter(p)
	}

	// L127-128: ParameterType = TupleFromList(params.symbol.type per param)
	paramTypes := make([]typ.Type, 0, len(script.Parameters))
	for _, p := range script.Parameters {
		var pt typ.Type = typ.MetaError
		if p.Symbol != nil {
			if local, ok := p.Symbol.(*symbol.LocalVariableSymbol); ok {
				pt = local.Type
			}
		}
		paramTypes = append(paramTypes, pt)
	}
	script.ParameterType = typ.TupleFromList(paramTypes)

	// L130-131: parameter-vs-trigger compat check (T12).
	sr.checkScriptParameters(trig, script, script.Parameters)

	// L133-153: return type construction.
	if len(script.ReturnTokens) > 0 {
		returns := make([]typ.Type, 0, len(script.ReturnTokens))
		for _, tok := range script.ReturnTokens {
			var ty typ.Type
			if sr.isDisabledTypeName(tok.Text) {
				diagnostics.ReportErrorAt(sr.diagnostics, tok,
					diagnostics.MessageFeatureDisabledType, tok.Text)
				ty = typ.MetaError
			} else {
				ty = sr.typeManager.FindOrNil(tok.Text, false)
				if ty == nil {
					diagnostics.ReportErrorAt(sr.diagnostics, tok,
						diagnostics.MessageGenericInvalidType, tok.Text)
					ty = typ.MetaError
				}
			}
			returns = append(returns, ty)
		}
		script.ReturnType = typ.TupleFromList(returns)
	} else {
		// L154-155: default based on trigger.
		switch {
		case trig == nil:
			script.ReturnType = typ.MetaError
		case trig.AllowReturns:
			script.ReturnType = typ.MetaUnit
		default:
			script.ReturnType = typ.MetaNothing
		}
	}

	// L157: return-vs-trigger compat check (T12).
	sr.checkScriptReturns(trig, script)

	// L159-169: insert ServerScriptSymbol into root table (gated on trigger
	// being present + not disabled).
	if trig != nil && !sr.isDisabledTrigger(trig) {
		ssym := &symbol.ServerScriptSymbol{
			ScriptSymbolFields: symbol.ScriptSymbolFields{
				Trigger:    trig,
				Name:       script.NameString(),
				Parameters: typeRefAsType(script.ParameterType),
				Returns:    typeRefAsType(script.ReturnType),
			},
		}
		inserted := sr.rootTable.Insert(symbol.SymbolTypeServerScript(trig), ssym)
		if !inserted {
			diagnostics.ReportErrorAt(sr.diagnostics, script,
				diagnostics.MessageScriptRedeclaration, trig.Identifier, script.NameString())
		} else {
			script.Symbol = ssym
		}
	}

	// L172: file-level block table assignment.
	script.Block = sr.activeTable()
}

// typeRefAsType unwraps an ast.TypeRef that this package wrote (always a
// typ.Type) back into typ.Type. The field is interface-typed only for the
// cyclic-import bridge; the concrete value is always typ.Type.
func typeRefAsType(t ast.TypeRef) typ.Type {
	if t == nil {
		return typ.MetaError
	}
	return t.(typ.Type)
}

// checkScriptSubject is the T11 stub.
func (sr *ScriptRegistration) checkScriptSubject(t *trigger.TriggerType, script *ast.Script) {
	_ = t
	_ = script
}

// visitParameter is the T12 stub.
func (sr *ScriptRegistration) visitParameter(p *ast.Parameter) {
	_ = p
}

// checkScriptParameters is the T12 stub.
func (sr *ScriptRegistration) checkScriptParameters(t *trigger.TriggerType, script *ast.Script, params []*ast.Parameter) {
	_ = t
	_ = script
	_ = params
}

// checkScriptReturns is the T12 stub.
func (sr *ScriptRegistration) checkScriptReturns(t *trigger.TriggerType, script *ast.Script) {
	_ = t
	_ = script
}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/semantics/...
```

Expected: PASS for all T10 tests.

### Step 5: Commit

```bash
git add pkg/pack/compiler/semantics/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/semantics): NAI-205 T10 — ScriptRegistration.visitScript core

Implement visitScript per ScriptRegistration.ts L96-180:
- Trigger lookup + invalid-trigger diagnostic
- Star-suffix-only-for-commands check
- Return type resolution (token-driven or trigger-default)
- ServerScriptSymbol insertion + redeclaration diagnostic
- ParameterType tuple assembly (consumes Parameter.Symbol set by T12)
- Block assignment (file-level scoped table)

Stubs for checkScriptSubject (T11), visitParameter (T12),
checkScriptParameters (T12), checkScriptReturns (T12).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: `semantics/` — subject validation + tryParseMapZone/Zone + resolveSubjectSymbol

**Spec ref:** §6.5 — TS line range L184-380.

**Files:**
- Modify: `pkg/pack/compiler/semantics/script_registration.go` (replace checkScriptSubject stub + add helpers)
- Create: `pkg/pack/compiler/semantics/subject_test.go`

**TS reference:**
- `ScriptRegistration.ts` L184-291 (checkScriptSubject + checkGlobal/Category/TypeScriptSubject)
- `ScriptRegistration.ts` L293-352 (tryParseMapZone + tryParseZone)
- `ScriptRegistration.ts` L357-380 (resolveSubjectSymbol)

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/semantics/subject_test.go
package semantics

import (
	"strconv"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// triggerWithSubjectMode builds a proc-like trigger with the given subject mode.
func triggerWithSubjectMode(ident string, mode trigger.SubjectMode) *trigger.TriggerType {
	return &trigger.TriggerType{
		Identifier:      ident,
		SubjectMode:     mode,
		AllowParameters: true,
		AllowReturns:    true,
	}
}

func TestSubject_GlobalUnderscore_NoSubjectModeReturnsClean(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	t1 := triggerWithSubjectMode("foo", trigger.ModeNone)
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "_")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})
	if d.HasErrors() {
		t.Fatalf("HasErrors for ModeNone+'_': %+v", d.List())
	}
}

func TestSubject_GlobalUnderscore_TypeModeWithGlobalFalse_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveInt, true, false))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "_")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptSubjectNoGlobal {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_SUBJECT_NO_GLOBAL diagnostic: %+v", d.List())
	}
}

func TestSubject_TypeReference_ResolvesViaBasicLookup(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	// Pre-populate root table with an "obj_bowl" BasicSymbol.
	root.Insert(symbol.SymbolTypeBasic(typ.PrimitiveInt), &symbol.BasicSymbol{
		Name: "obj_bowl",
		Type: typ.PrimitiveInt,
	})
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveInt, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "obj_bowl")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if d.HasErrors() {
		t.Fatalf("HasErrors: %+v", d.List())
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil")
	}
	bs, ok := s.SubjectReference.(*symbol.BasicSymbol)
	if !ok {
		t.Fatalf("SubjectReference = %T, want *symbol.BasicSymbol", s.SubjectReference)
	}
	if bs.Name != "obj_bowl" {
		t.Fatalf("SubjectReference name = %q", bs.Name)
	}
}

func TestSubject_TypeReference_Unresolved_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveInt, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "nope_does_not_exist")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageGenericUnresolvedSymbol {
			found = true
		}
	}
	if !found {
		t.Fatalf("no GENERIC_UNRESOLVED_SYMBOL diagnostic: %+v", d.List())
	}
}

func TestSubject_SpaceInSubject_NonTypeMode_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	t1 := triggerWithSubjectMode("foo", trigger.ModeNone)
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "has space")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptSubjectNoSpaces {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_SUBJECT_NO_SPACES diagnostic: %+v", d.List())
	}
}

func TestSubject_MapzoneSubject_PackedAndStoredAsBasicSymbol(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveMapzone)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveMapzone, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "0_50_50")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if d.HasErrors() {
		t.Fatalf("HasErrors for valid mapzone: %+v", d.List())
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil")
	}
	bs := s.SubjectReference.(*symbol.BasicSymbol)
	// Per TS L319 packed = (z & 0x3fff) | ((x & 0x3fff) << 14)
	// where x = mxInt<<6 = 50<<6 = 3200, z = mzInt<<6 = 50<<6 = 3200.
	x := int32(50 << 6)
	z := int32(50 << 6)
	want := (z & 0x3fff) | ((x & 0x3fff) << 14)
	if bs.Name != strconv.Itoa(int(want)) {
		t.Fatalf("SubjectReference name = %q, want %q (packed %d)", bs.Name, strconv.Itoa(int(want)), want)
	}
}

func TestSubject_MapzoneBadLevel_EmitsErrorAndStillSetsSubjectReference(t *testing.T) {
	// Per TS L302-304 + caller pattern at L357-380: when level != 0,
	// reportError is emitted AND the BasicSymbol is constructed with the
	// sentinel value (-1). SubjectReference is set regardless of error.
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveMapzone)
	t1 := triggerWithSubjectMode("foo", trigger.NewModeType(typ.PrimitiveMapzone, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("foo", "1_50_50") // level 1 → error
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if !d.HasErrors() {
		t.Fatal("HasErrors = false; want true (level 1 invalid)")
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil; TS sets it even on error")
	}
}

func TestSubject_CategorySubject_ResolvedViaCategoryType(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_, err := tm.RegisterNew("category", "", typ.BaseVarInteger, -1)
	if err != nil {
		t.Fatal(err)
	}
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	// Pre-populate the category table with "foo".
	cat, _ := tm.Find("category", false)
	root.Insert(symbol.SymbolTypeBasic(cat), &symbol.BasicSymbol{
		Name: "foo",
		Type: cat,
	})
	t1 := triggerWithSubjectMode("trig", trigger.NewModeType(typ.PrimitiveInt, true, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("trig", "_foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if d.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}
	if s.SubjectReference == nil {
		t.Fatal("SubjectReference nil for category subject _foo")
	}
	bs := s.SubjectReference.(*symbol.BasicSymbol)
	if bs.Name != "foo" {
		t.Fatalf("SubjectReference name = %q, want \"foo\"", bs.Name)
	}
}

func TestSubject_CategorySubject_TypeMode_CategoryFalse_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_, _ = tm.RegisterNew("category", "", typ.BaseVarInteger, -1)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	t1 := triggerWithSubjectMode("trig", trigger.NewModeType(typ.PrimitiveInt, false, true))
	_ = trm.RegisterTrigger(t1)
	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("trig", "_foo")
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptSubjectNoCategory {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_SUBJECT_NO_CATEGORY diagnostic: %+v", d.List())
	}
}
```

### Step 2: Run tests to confirm failure

Expected: failures in all subject tests.

### Step 3: Replace stub + add helpers

Replace the `checkScriptSubject` stub in `script_registration.go` and add the supporting helpers below.

```go
// Replace the T10 stub in script_registration.go with the following:

import (
	"strconv"
	"strings"
)

// checkScriptSubject validates that the script's subject (the name field
// after the trigger) is allowed by the trigger's SubjectMode.
// Mirrors TS L184-208.
func (sr *ScriptRegistration) checkScriptSubject(t *trigger.TriggerType, script *ast.Script) {
	if t == nil {
		return
	}
	mode := t.SubjectMode
	if mode == nil {
		return
	}

	subject := script.Name.Text
	if strings.Contains(subject, " ") {
		_, isType := trigger.IsTypeMode(mode)
		if !isType {
			diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
				diagnostics.MessageScriptSubjectNoSpaces, t.Identifier)
			return
		}
	}

	if mode == trigger.ModeName {
		return
	}

	if subject == "_" {
		sr.checkGlobalScriptSubject(t, script)
		return
	}

	if strings.HasPrefix(subject, "_") {
		sr.checkCategoryScriptSubject(t, script, subject[1:])
		return
	}

	sr.checkTypeScriptSubject(t, script, subject)
}

// checkGlobalScriptSubject validates that `_` subjects are allowed for this
// trigger's SubjectMode. Mirrors TS L213-235.
func (sr *ScriptRegistration) checkGlobalScriptSubject(t *trigger.TriggerType, script *ast.Script) {
	mode := t.SubjectMode
	if mode == trigger.ModeNone {
		return
	}
	if tm, ok := trigger.IsTypeMode(mode); ok {
		if !tm.Global {
			diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
				diagnostics.MessageScriptSubjectNoGlobal, t.Identifier)
		}
		return
	}
	// Unexpected: TS throws. Goscape silently no-ops since impossible to reach
	// given the sealed SubjectMode interface.
}

// checkCategoryScriptSubject validates `_FOO`-shaped subjects. Mirrors TS L240-269.
func (sr *ScriptRegistration) checkCategoryScriptSubject(t *trigger.TriggerType, script *ast.Script, subject string) {
	mode := t.SubjectMode
	cat := sr.categoryType
	if cat == nil {
		// TS throws "'category' type not defined." Goscape mirrors as a panic
		// since this is an impossible state when the type registry is correct.
		panic("'category' type not defined")
	}
	if mode == trigger.ModeNone {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageScriptSubjectOnlyGlobal, t.Identifier)
		return
	}
	if tm, ok := trigger.IsTypeMode(mode); ok {
		if !tm.Category {
			diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
				diagnostics.MessageScriptSubjectNoCategory, t.Identifier)
			return
		}
		sr.resolveSubjectSymbol(script, subject, cat)
		return
	}
}

// checkTypeScriptSubject validates type-reference subjects (e.g. "obj_bowl").
// Mirrors TS L274-291.
func (sr *ScriptRegistration) checkTypeScriptSubject(t *trigger.TriggerType, script *ast.Script, subject string) {
	mode := t.SubjectMode
	if mode == trigger.ModeNone {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageScriptSubjectOnlyGlobal, t.Identifier)
		return
	}
	if tm, ok := trigger.IsTypeMode(mode); ok {
		sr.resolveSubjectSymbol(script, subject, tm.Type)
		return
	}
}

// tryParseMapZone parses `level_mx_mz`. Returns the packed int32 (which may
// be -1 on parse failure). Reports diagnostics via script.Name. Mirrors TS
// L293-319.
func (sr *ScriptRegistration) tryParseMapZone(script *ast.Script, coord string) int32 {
	parts := strings.Split(coord, "_")
	if len(parts) != 3 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageMapzoneSubjectForm)
		return -1
	}
	level, errA := strconv.Atoi(parts[0])
	mx, errB := strconv.Atoi(parts[1])
	mz, errC := strconv.Atoi(parts[2])
	if errA != nil || errB != nil || errC != nil {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageMapzoneSubjectForm)
		return -1
	}
	if mx < 0 || mx > 255 || mz < 0 || mz > 255 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageMapzoneInvalidCoord)
	}
	if level != 0 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageMapzoneOnlyLevelZero)
		return -1
	}
	x := int32(mx) << 6
	z := int32(mz) << 6
	return (z & 0x3fff) | ((x & 0x3fff) << 14)
}

// tryParseZone parses `level_mx_mz_lx_lz`. Returns packed int32 (may be -1
// on parse failure). Mirrors TS L321-352.
func (sr *ScriptRegistration) tryParseZone(script *ast.Script, coord string) int32 {
	parts := strings.Split(coord, "_")
	if len(parts) != 5 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageZoneSubjectForm)
		return -1
	}
	level, errA := strconv.Atoi(parts[0])
	mx, errB := strconv.Atoi(parts[1])
	mz, errC := strconv.Atoi(parts[2])
	lx, errD := strconv.Atoi(parts[3])
	lz, errE := strconv.Atoi(parts[4])
	if errA != nil || errB != nil || errC != nil || errD != nil || errE != nil {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageZoneSubjectForm)
		return -1
	}
	if level < 0 || level > 3 || mx < 0 || mx > 255 || mz < 0 || mz > 255 ||
		lx < 0 || lx > 63 || lz < 0 || lz > 63 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageZoneInvalidCoord)
	}
	if lx%8 != 0 || lz%8 != 0 {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageZoneLocalCoordMultipleOf8)
		return -1
	}
	x := (((int32(mx) << 6) | int32(lx)) >> 3) << 3
	z := (((int32(mz) << 6) | int32(lz)) >> 3) << 3
	return (z & 0x3fff) | ((x & 0x3fff) << 14) | ((int32(level) & 0x3) << 28)
}

// resolveSubjectSymbol finds the symbol-table entry for the subject + type.
// Mirrors TS L357-380.
func (sr *ScriptRegistration) resolveSubjectSymbol(script *ast.Script, subject string, t typ.Type) {
	if t == typ.PrimitiveMapzone {
		packed := sr.tryParseMapZone(script, subject)
		script.SubjectReference = &symbol.BasicSymbol{
			Name: strconv.Itoa(int(packed)),
			Type: t,
		}
		return
	}
	if t == typ.PrimitiveCoord {
		packed := sr.tryParseZone(script, subject)
		script.SubjectReference = &symbol.BasicSymbol{
			Name: strconv.Itoa(int(packed)),
			Type: t,
		}
		return
	}

	found := sr.rootTable.Find(symbol.SymbolTypeBasic(t), subject)
	if found == nil {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageGenericUnresolvedSymbol, subject)
		return
	}
	bs, ok := found.(*symbol.BasicSymbol)
	if !ok {
		diagnostics.ReportErrorAt(sr.diagnostics, script.Name,
			diagnostics.MessageGenericUnresolvedSymbol, subject)
		return
	}
	script.SubjectReference = bs
}
```

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/semantics/...
```

Expected: PASS for all T11 + T10 tests.

### Step 5: Commit

```bash
git add pkg/pack/compiler/semantics/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/semantics): NAI-205 T11 — subject validation + mapzone/zone parsing

Implement checkScriptSubject + helpers per ScriptRegistration.ts L184-380:
- checkGlobalScriptSubject / checkCategoryScriptSubject / checkTypeScriptSubject
- tryParseMapZone (level_mx_mz format)
- tryParseZone (level_mx_mz_lx_lz format)
- resolveSubjectSymbol (mapzone/coord packed-int → BasicSymbol;
  other types via root-table lookup)

Diagnostics emitted per TS:
- SCRIPT_SUBJECT_NO_SPACES (non-type-mode subjects with spaces)
- SCRIPT_SUBJECT_NO_GLOBAL (type-mode global=false + '_' subject)
- SCRIPT_SUBJECT_NO_CATEGORY (type-mode category=false + '_X' subject)
- SCRIPT_SUBJECT_ONLY_GLOBAL (ModeNone + non-'_' subject)
- GENERIC_UNRESOLVED_SYMBOL (type-mode subject not in root table)
- Mapzone/zone format + range errors (verbatim TS messages)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: `semantics/` — visitParameter + checkScriptParameters + checkScriptReturns

**Spec ref:** §6.5 — TS line range L385-451.

**Files:**
- Modify: `pkg/pack/compiler/semantics/script_registration.go` (replace three stubs)
- Create: `pkg/pack/compiler/semantics/parameter_test.go`
- Create: `pkg/pack/compiler/semantics/trigger_check_test.go`

### Step 1: Write the failing tests

```go
// pkg/pack/compiler/semantics/parameter_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func paramFor(typeText, name string) *ast.Parameter {
	loc := lexer.NodeSourceLocation{Name: "<test>", Line: 1}
	return &ast.Parameter{
		SrcLoc:    loc,
		TypeToken: &ast.Token{SrcLoc: loc, Text: typeText},
		Name:      &ast.Identifier{SrcLoc: loc, Text: name},
	}
}

func TestVisitParameter_RegistersLocal(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	if d.HasErrors() {
		t.Fatalf("unexpected errors: %+v", d.List())
	}
	if s.Parameters[0].Symbol == nil {
		t.Fatal("Parameter.Symbol nil")
	}
	loc, ok := s.Parameters[0].Symbol.(*symbol.LocalVariableSymbol)
	if !ok {
		t.Fatalf("Symbol = %T, want *LocalVariableSymbol", s.Parameters[0].Symbol)
	}
	if loc.Name != "x" || loc.Type != typ.PrimitiveInt {
		t.Fatalf("LocalVariableSymbol = %+v", loc)
	}
}

func TestVisitParameter_DuplicateName_EmitsLocalRedeclaration(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x"), paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptLocalRedeclaration {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_LOCAL_REDECLARATION: %+v", d.List())
	}
}

func TestVisitParameter_InvalidType_EmitsGenericInvalidType(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	// TypeManager has nothing registered.

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("nope", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageGenericInvalidType {
			found = true
		}
	}
	if !found {
		t.Fatalf("no GENERIC_INVALID_TYPE: %+v", d.List())
	}
}

func TestVisitParameter_FeatureDisabledType_EmitsFeatureDisabledType(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveBoolean)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{DisableBooleans: true})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("boolean", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageFeatureDisabledType {
			found = true
		}
	}
	if !found {
		t.Fatalf("no FEATURE_DISABLED_TYPE: %+v", d.List())
	}
}

func TestVisitParameter_ProcsDisabled_NonCommand_EmitsFeatureDisabledLocal(t *testing.T) {
	// Per TS L420-422: when features.procs===false AND triggerType !== CommandTrigger,
	// any parameter on the script emits FEATURE_DISABLED_LOCAL.
	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{DisableProcs: true})
	s := scriptFor("proc", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageFeatureDisabledLocal {
			found = true
		}
	}
	if !found {
		t.Fatalf("no FEATURE_DISABLED_LOCAL: %+v", d.List())
	}
}
```

```go
// pkg/pack/compiler/semantics/trigger_check_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestCheckParameters_TriggerDisallowsParameters_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	// Build a trigger that disallows parameters.
	t1 := makeProcTrigger()
	t1.AllowParameters = false
	t1.Identifier = "nopars"
	_ = trm.RegisterTrigger(t1)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("nopars", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptTriggerNoParameters {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_NO_PARAMETERS: %+v", d.List())
	}
}

func TestCheckParameters_TriggerExpectsParameters_TypeMismatch_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	// Trigger expects (string) params; script provides (int).
	t1 := makeProcTrigger()
	t1.Identifier = "needsstr"
	t1.Parameters = typ.PrimitiveString // single-type tuple
	_ = trm.RegisterTrigger(t1)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("needsstr", "foo")
	s.Parameters = []*ast.Parameter{paramFor("int", "x")}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptTriggerExpectedParameters {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_EXPECTED_PARAMETERS: %+v", d.List())
	}
}

func TestCheckReturns_TriggerDisallowsReturns_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)

	// label trigger disallows returns.
	t1 := makeLabelTrigger()
	_ = trm.RegisterTrigger(t1)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("label", "foo")
	loc := s.SrcLoc
	s.ReturnTokens = []*ast.Token{{SrcLoc: loc, Text: "int"}}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptTriggerNoReturns {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_NO_RETURNS: %+v", d.List())
	}
}

func TestCheckReturns_TriggerExpectsSpecificReturns_Mismatch_Errors(t *testing.T) {
	tm, trm, root, d := newTestFixture(t)
	_ = tm.RegisterByRepresentation(typ.PrimitiveInt)
	_ = tm.RegisterByRepresentation(typ.PrimitiveString)

	t1 := makeProcTrigger()
	t1.Identifier = "needsint"
	t1.Returns = typ.PrimitiveInt
	_ = trm.RegisterTrigger(t1)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	s := scriptFor("needsint", "foo")
	loc := s.SrcLoc
	s.ReturnTokens = []*ast.Token{{SrcLoc: loc, Text: "string"}}
	sr.Visit(&ast.ScriptFile{Scripts: []*ast.Script{s}})

	found := false
	for _, e := range d.List() {
		if e.Message == diagnostics.MessageScriptTriggerExpectedReturns {
			found = true
		}
	}
	if !found {
		t.Fatalf("no SCRIPT_TRIGGER_EXPECTED_RETURNS: %+v", d.List())
	}
}
```

### Step 2: Run tests to confirm failure

Expected: failures across all T12 tests (stubs from T10 still in place).

### Step 3: Replace the three stubs

Replace the three stubs in `script_registration.go` with the implementation below.

```go
// Replace the T10 stubs in script_registration.go.

// visitParameter mirrors TS L412-451.
func (sr *ScriptRegistration) visitParameter(p *ast.Parameter) {
	name := p.Name.Text
	typeText := p.TypeToken.Text
	var ty typ.Type

	// Find the enclosing Script. TS uses findParentByType; goscape stores
	// the active script on sr.activeScript during visitScript (set in T10's
	// updated visitScript body).
	script := sr.activeScript

	// Helper: returns true iff the enclosing script's resolved trigger is
	// the CommandTrigger singleton. False when script is nil OR trigger
	// lookup failed (TriggerType is nil) OR concrete type isn't CommandTrigger.
	enclosingIsCommand := func() bool {
		if script == nil || script.TriggerType == nil {
			return false
		}
		t, ok := script.TriggerType.(*trigger.TriggerType)
		return ok && t == trigger.CommandTrigger
	}

	if sr.features.DisableProcs && !enclosingIsCommand() {
		// TS L420-421: features.procs===false on non-command triggers => disabled.
		diagnostics.ReportErrorAt(sr.diagnostics, p,
			diagnostics.MessageFeatureDisabledLocal)
		ty = typ.MetaError
	} else if sr.isDisabledTypeName(typeText) {
		diagnostics.ReportErrorAt(sr.diagnostics, p,
			diagnostics.MessageFeatureDisabledType, typeText)
		ty = typ.MetaError
	} else {
		ty = sr.typeManager.FindOrNil(typeText, true)
		if ty != nil && ty != typ.MetaError {
			// TS L426: non-command + non-AllowParameter type → error.
			if !enclosingIsCommand() && !ty.Options().AllowParameter {
				diagnostics.ReportErrorAt(sr.diagnostics, p.TypeToken,
					diagnostics.MessageLocalParameterInvalidType, ty.Representation())
			}
		}
	}

	if ty == nil {
		diagnostics.ReportErrorAt(sr.diagnostics, p,
			diagnostics.MessageGenericInvalidType, typeText)
	}

	var symType typ.Type = ty
	if symType == nil {
		symType = typ.MetaError
	}
	sym := &symbol.LocalVariableSymbol{Name: name, Type: symType}
	inserted := sr.activeTable().Insert(symbol.SymbolTypeLocalVariable(), sym)
	if !inserted {
		diagnostics.ReportErrorAt(sr.diagnostics, p,
			diagnostics.MessageScriptLocalRedeclaration, name)
	}

	p.Symbol = sym
}

// checkScriptParameters mirrors TS L385-396.
func (sr *ScriptRegistration) checkScriptParameters(t *trigger.TriggerType, script *ast.Script, params []*ast.Parameter) {
	if t == nil {
		return
	}
	if !t.AllowParameters && len(params) > 0 {
		diagnostics.ReportErrorAt(sr.diagnostics, params[0],
			diagnostics.MessageScriptTriggerNoParameters, t.Identifier)
		return
	}
	if t.Parameters != nil {
		scriptParams := typeRefAsType(script.ParameterType)
		if scriptParams != t.Parameters {
			diagnostics.ReportErrorAt(sr.diagnostics, script,
				diagnostics.MessageScriptTriggerExpectedParameters,
				script.Trigger.Text, t.Parameters.Representation())
		}
	}
}

// checkScriptReturns mirrors TS L400-410.
func (sr *ScriptRegistration) checkScriptReturns(t *trigger.TriggerType, script *ast.Script) {
	if t == nil {
		return
	}
	scriptReturns := typeRefAsType(script.ReturnType)
	if !t.AllowReturns && scriptReturns != typ.MetaNothing {
		diagnostics.ReportErrorAt(sr.diagnostics, script,
			diagnostics.MessageScriptTriggerNoReturns, t.Identifier)
		return
	}
	if t.Returns != nil && scriptReturns != t.Returns {
		diagnostics.ReportErrorAt(sr.diagnostics, script,
			diagnostics.MessageScriptTriggerExpectedReturns,
			script.Trigger.Text, t.Returns.Representation())
	}
}
```

Add `activeScript` field to `ScriptRegistration` and thread it inside `visitScript`. In `script_registration.go`, locate the existing struct and add the field:

```go
type ScriptRegistration struct {
	// ... existing fields ...
	categoryType typ.Type

	// activeScript is the Script currently being visited. Used by
	// visitParameter to consult the enclosing trigger. nil outside of
	// visitScript.
	activeScript *ast.Script
}
```

In `visitScript`, set it at entry and clear at exit:

```go
func (sr *ScriptRegistration) visitScript(script *ast.Script) {
	sr.activeScript = script
	defer func() { sr.activeScript = nil }()

	// ... rest of T10 body unchanged ...
}
```

Note: `*ast.Parameter` doesn't satisfy `ast.Node` directly unless it has a `Source()` method. Verify in `pkg/pack/compiler/ast/scriptfile.go` that `Parameter.Source()` exists (it should — T6 plan-write confirmation step). If not, the `ReportErrorAt(sr.diagnostics, p, …)` call needs to route through `p.SrcLoc` instead. (Note: per the existing code shown in T8 stub, `(p *Parameter) Source() lexer.NodeSourceLocation { return p.SrcLoc }` already exists — confirmed via T8 review.)

### Step 4: Run tests to confirm pass

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/semantics/...
```

Expected: PASS for all T9-T12 tests.

### Step 5: Commit

```bash
git add pkg/pack/compiler/semantics/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/semantics): NAI-205 T12 — visitParameter + parameter/return trigger checks

Implement the three remaining ScriptRegistration walkers:
- visitParameter (TS L412-451): registers LocalVariableSymbol;
  emits FEATURE_DISABLED_LOCAL / FEATURE_DISABLED_TYPE /
  GENERIC_INVALID_TYPE / LOCAL_PARAMETER_INVALID_TYPE /
  SCRIPT_LOCAL_REDECLARATION as appropriate.
- checkScriptParameters (TS L385-396): SCRIPT_TRIGGER_NO_PARAMETERS
  + SCRIPT_TRIGGER_EXPECTED_PARAMETERS.
- checkScriptReturns (TS L400-410): SCRIPT_TRIGGER_NO_RETURNS
  + SCRIPT_TRIGGER_EXPECTED_RETURNS.

Adds activeScript field to ScriptRegistration so visitParameter can
consult the enclosing script's TriggerType (TS uses findParentByType;
goscape uses an explicit side-channel — see plan T12 step 3 doc).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: `semantics/` — end-to-end smoke + 11 deviation-pin tests

**Spec ref:** §8.5, §7.

**Files:**
- Create: `pkg/pack/compiler/semantics/smoke_test.go`
- Create: `pkg/pack/compiler/semantics/nai205_deviation_pins_test.go`

### Step 1: Write the failing test

```go
// pkg/pack/compiler/semantics/smoke_test.go
package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/parser"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestSmoke_Parser_ScriptRegistration_E2E parses a 2-script source file
// and runs ScriptRegistration. Asserts no diagnostics + root-table state.
func TestSmoke_Parser_ScriptRegistration_E2E(t *testing.T) {
	src := "[proc,foo]\nreturn;\n[label,bar]\nreturn;\n"
	p := parser.NewScriptFileParser(src, "smoke.rs2")
	file := p.ParseScriptFile()
	if file == nil {
		t.Fatal("parser returned nil ScriptFile")
	}
	if got, want := len(file.Scripts), 2; got != want {
		t.Fatalf("parsed Scripts len = %d, want %d", got, want)
	}

	tm, trm, root, d := newTestFixture(t)
	proc := makeProcTrigger()
	label := makeLabelTrigger()
	_ = trm.RegisterTrigger(proc)
	_ = trm.RegisterTrigger(label)

	sr := NewScriptRegistration(tm, trm, root, d, StrictFeatureLevel{})
	sr.Visit(file)

	if d.HasErrors() {
		t.Fatalf("smoke had errors: %+v", d.List())
	}

	for _, s := range file.Scripts {
		if s.Symbol == nil {
			t.Fatalf("script %q Symbol nil", s.NameString())
		}
		if s.Block == nil {
			t.Fatalf("script %q Block nil", s.NameString())
		}
		if s.TriggerType == nil {
			t.Fatalf("script %q TriggerType nil", s.NameString())
		}
	}

	if got := root.Find(symbol.SymbolTypeServerScript(proc), "foo"); got == nil {
		t.Fatal("root table missing proc/foo")
	}
	if got := root.Find(symbol.SymbolTypeServerScript(label), "bar"); got == nil {
		t.Fatal("root table missing label/bar")
	}

	_ = typ.MetaUnit // keep import even if not asserted
}
```

```go
// pkg/pack/compiler/semantics/nai205_deviation_pins_test.go
package semantics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readPackageFiles concatenates every non-_test.go file in dir.
func readPackageFiles(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %q: %v", dir, err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %q: %v", filepath.Join(dir, e.Name()), err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func pin(t *testing.T, dir, tag string) {
	t.Helper()
	src := readPackageFiles(t, dir)
	if !strings.Contains(src, tag) {
		t.Fatalf("%q missing deviation tag %q", dir, tag)
	}
}

func TestPin_NAI205D_NoNodeReportError(t *testing.T) {
	pin(t, "../diagnostics", "NAI-205-D-NO-NODE-REPORT-ERROR")
}

func TestPin_NAI205D_TypeOptionsFlat(t *testing.T) {
	pin(t, "../type", "NAI-205-D-TYPEOPTIONS-FLAT")
}

func TestPin_NAI205D_MetaTypeFlat(t *testing.T) {
	pin(t, "../type", "NAI-205-D-METATYPE-FLAT")
}

func TestPin_NAI205D_TypeNoIntern(t *testing.T) {
	pin(t, "../type", "NAI-205-D-TYPE-NO-INTERN")
}

func TestPin_NAI205D_ScriptSymbolNoPointers(t *testing.T) {
	pin(t, "../symbol", "NAI-205-D-SCRIPTSYMBOL-NO-POINTERS")
}

func TestPin_NAI205D_SymbolTypeStringKey(t *testing.T) {
	pin(t, "../symbol", "NAI-205-D-SYMBOLTYPE-STRING-KEY")
}

func TestPin_NAI205D_TriggerPointersDeferred(t *testing.T) {
	pin(t, "../trigger", "NAI-205-D-TRIGGER-POINTERS-DEFERRED")
}

func TestPin_NAI205D_StrictInvertedPolarity(t *testing.T) {
	pin(t, ".", "NAI-205-D-STRICT-INVERTED-POLARITY")
}

func TestPin_NAI205D_AstRefInterfaces(t *testing.T) {
	pin(t, "../ast", "NAI-205-D-AST-REF-INTERFACES")
}

func TestPin_NAI205D_HandlerRequiredMethods(t *testing.T) {
	pin(t, "../diagnostics", "NAI-205-D-HANDLER-REQUIRED-METHODS")
}

func TestPin_NAI205D_NoVisitBlock(t *testing.T) {
	pin(t, ".", "NAI-205-D-NO-VISIT-BLOCK")
}
```

### Step 2: Run tests to confirm failure

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: smoke passes if T9-T12 are green; some pin tests may fail if any deviation tag was inadvertently omitted. Each failure points at the file/tag combination to add.

### Step 3: Patch up any missing tags

For each failing pin test, locate the production file in the named package and add the tag inside a doc-comment. The tag is just a string — no behaviour change. Each tag's home is defined in spec §7. Common omissions:
- `NAI-205-D-STRICT-INVERTED-POLARITY` — should be in `semantics/strict_feature.go` doc-comment (T9).
- `NAI-205-D-NO-VISIT-BLOCK` — should be in `semantics/script_registration.go` doc-comment on the `ScriptRegistration` struct (T9 / T10).

Re-run after each patch:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/semantics/...
```

### Step 4: Run the full test suite

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/pack/compiler/...
```

Expected: both clean.

### Step 5: Commit

```bash
git add pkg/pack/compiler/semantics/smoke_test.go pkg/pack/compiler/semantics/nai205_deviation_pins_test.go
# Plus any production-file edits to satisfy pin tests.
git add -u pkg/pack/compiler/
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(compiler/semantics): NAI-205 T13 — end-to-end smoke + 11 deviation pins

Add the end-to-end smoke (parser → ScriptRegistration → assert symbol-table
+ AST-field state on a 2-script source) plus pin tests for every NAI-205-D-*
deviation tag. Pin tests enforce the spec §7 inventory across all five new
packages.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Final review + close commit

**Files:** None — review-only task; close commit picks up Closes-memory trailer.

### Step 1: Two-stage review

Per [[runescript_cadence]]:
- Run `code-reviewer` subagent against the NAI-205 commit range (T1..T13).
- Run a second-pass review pinned to TS-source fidelity ([[true_to_ts_gate]], [[plan_grep_helper_patterns]], [[plan_arithmetic_off_by_one_carry_forward]] for tryParseMapZone/Zone math).

Expected output: a list of issues organised by severity. Address each:
- **High:** fix inline; new commit referencing the review.
- **Medium:** decide fix-now vs. follow-up tracker entry. Default to fix-now if scope ≤ 30 LOC.
- **Low:** add to follow-up notes if substantive; otherwise close-commit notes.

### Step 2: Verify the full test suite + vet

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/pack/compiler/...
```

Both clean.

### Step 3: Close commit

Update `MEMORY.md` index for any new memory entries surfaced during NAI-205 (per [[post_task_handoff]]). Common candidates:
- A note on `metaScript.trigger: any` carrying forward (cycle constraint at type → trigger boundary).
- Anything surfaced during review that future plan-authors should grep for.

Commit:

```bash
git add MEMORY.md  # plus any new memory files
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-205 — type/symbol/trigger/diagnostic + ScriptRegistration

Compiler slice 3 of 6. Hand-ports ~1300 LOC of TS compiler infrastructure
plus the 457-LOC ScriptRegistration first pass. Lifts seven AST fields
(Script.TriggerType/Symbol/Block/ParameterType/ReturnType/SubjectReference,
Parameter.Symbol) from NAI-204-D-AST-NO-TYPE-FIELDS deferral; the tag is
narrowed (not retired) to the nine remaining TypeChecking-owned fields,
which NAI-206 will land.

Highlights:
- Cyclic-import resolution via four marker interfaces in ast/symbol_refs.go
  (NAI-205-D-AST-REF-INTERFACES). Concrete types in symbol/trigger/type
  satisfy structurally via exported zero-arg marker methods.
- StrictFeatureLevel polarity inversion (NAI-205-D-STRICT-INVERTED-POLARITY)
  for idiomatic-Go zero-value semantics.
- SymbolType string-key derivation (NAI-205-D-SYMBOLTYPE-STRING-KEY)
  replaces TS WeakMap interning.
- Type system: 7 PrimitiveType singletons + 4 MetaType singletons +
  parameterised MetaWrapping/MetaScript + TupleType + ArrayType + four
  GameVarType shapes + TypeManager with checker chain.
- 11 NAI-205-D-* deviation tags with pin tests across five packages.

Closes memory: (any new entries)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Acceptance criteria

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...` passes.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/pack/compiler/...` is clean.
3. All 11 NAI-205-D-* pins pass; the narrowed-tag pin in `ast/` passes; all four NAI-204 pins still pass.
4. Smoke test (T13) passes — parser + ScriptRegistration on a 2-script source produces no diagnostics + populated symbol table.
5. No new circular import warnings in any tooling.
