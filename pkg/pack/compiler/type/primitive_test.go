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
	// Per TS L57: ALL contains [INT, BOOLEAN, COORD, STRING, CHAR, LONG, MAPZONE, CATEGORY]
	// in that order. Order matters because PrimitiveAll is used for round-trips.
	want := []string{"int", "boolean", "coord", "string", "char", "long", "mapzone", "category"}
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

// TestPrimitiveCategory_Code pins the TS ScriptVarType.CATEGORY code.
// Mirrors TS src/runescript/type/ScriptVarType.ts L36.
func TestPrimitiveCategory_Code(t *testing.T) {
	code, ok := PrimitiveCategory.Code()
	if !ok || code != "y" {
		t.Errorf("PrimitiveCategory.Code() = (%q, %v), want (\"y\", true)", code, ok)
	}
	if got := PrimitiveCategory.Representation(); got != "category" {
		t.Errorf("PrimitiveCategory.Representation() = %q, want \"category\"", got)
	}
}

// astTypeRef mirrors ast.TypeRef without importing ast (avoid back-edge).
// Tests that try to satisfy this stub catch AsTypeRef() method drift.
type astTypeRef interface {
	AsTypeRef()
}
