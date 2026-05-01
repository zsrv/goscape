package objtype

import "testing"

func TestNewComponentTypeDefaults(t *testing.T) {
	c := NewComponentType(7)
	if c.ID != 7 {
		t.Errorf("ID: got %d, want 7", c.ID)
	}
	if c.RootLayer != -1 {
		t.Errorf("RootLayer: got %d, want -1", c.RootLayer)
	}
	if c.ComType != -1 {
		t.Errorf("ComType: got %d, want -1", c.ComType)
	}
	if c.ButtonType != -1 {
		t.Errorf("ButtonType: got %d, want -1", c.ButtonType)
	}
	if c.OverLayer != -1 {
		t.Errorf("OverLayer: got %d, want -1", c.OverLayer)
	}
	if c.Model != -1 {
		t.Errorf("Model: got %d, want -1", c.Model)
	}
	if c.ActiveModel != -1 {
		t.Errorf("ActiveModel: got %d, want -1", c.ActiveModel)
	}
	if c.Anim != -1 {
		t.Errorf("Anim: got %d, want -1", c.Anim)
	}
	if c.ActiveAnim != -1 {
		t.Errorf("ActiveAnim: got %d, want -1", c.ActiveAnim)
	}
	if c.ActionTarget != -1 {
		t.Errorf("ActionTarget: got %d, want -1", c.ActionTarget)
	}
	if c.ComName != "" {
		t.Errorf("ComName: got %q, want empty", c.ComName)
	}
	if c.Overlay {
		t.Errorf("Overlay: got true, want false")
	}
	if len(c.ScriptComparator) != 0 {
		t.Errorf("ScriptComparator: got %d, want 0", len(c.ScriptComparator))
	}
}
