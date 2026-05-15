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
