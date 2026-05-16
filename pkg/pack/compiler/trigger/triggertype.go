// pkg/pack/compiler/trigger/triggertype.go
package trigger

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TriggerType is the goscape port of TS interface TriggerType.
//
// TS makes it an interface implemented by const-literal trigger objects;
// goscape uses a struct since every trigger is a frozen data record.
// Pointer receivers satisfy ast.TriggerRef.
//
// (NAI-205-D-TRIGGER-POINTERS-DEFERRED retired by NAI-208: Pointers is now
// *pointer.PointerSet, populated by the trigger-registry caller when the
// trigger implicitly sets the pointer on invocation.)
type TriggerType struct {
	ID              int
	Identifier      string
	SubjectMode     SubjectMode
	AllowParameters bool
	Parameters      typ.Type // nil = trigger expects no specific param shape
	AllowReturns    bool
	Returns         typ.Type // nil = trigger expects no specific return shape
	Pointers        *pointer.PointerSet
}

// AsTriggerRef satisfies ast.TriggerRef so *TriggerType may be stored in
// ast.Script.TriggerType.
func (*TriggerType) AsTriggerRef() {}
