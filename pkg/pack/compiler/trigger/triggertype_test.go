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
