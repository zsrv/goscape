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
