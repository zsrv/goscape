package pack

import "testing"

func TestIsConfigBoolean(t *testing.T) {
	yes := []string{"yes", "no", "true", "false", "1", "0"}
	for _, v := range yes {
		if !IsConfigBoolean(v) {
			t.Errorf("IsConfigBoolean(%q)=false, want true", v)
		}
	}
	no := []string{"", "Yes", "TRUE", "2", "maybe", "y"}
	for _, v := range no {
		if IsConfigBoolean(v) {
			t.Errorf("IsConfigBoolean(%q)=true, want false", v)
		}
	}
}

func TestGetConfigBoolean(t *testing.T) {
	trueCases := []string{"yes", "true", "1"}
	for _, v := range trueCases {
		if !GetConfigBoolean(v) {
			t.Errorf("GetConfigBoolean(%q)=false, want true", v)
		}
	}
	falseCases := []string{"no", "false", "0", "Yes", "TRUE"}
	for _, v := range falseCases {
		if GetConfigBoolean(v) {
			t.Errorf("GetConfigBoolean(%q)=true, want false", v)
		}
	}
}

func TestConfigLine_StructShape(t *testing.T) {
	// Sanity: a ConfigLine can hold any ConfigValue.
	line := ConfigLine{Key: "type", Value: 105}
	if line.Key != "type" {
		t.Fatalf("Key=%q", line.Key)
	}
	if v, ok := line.Value.(int); !ok || v != 105 {
		t.Fatalf("Value=%v", line.Value)
	}
}
