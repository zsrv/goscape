package pack

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseParamConfig(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantVal ConfigValue
		wantOK  bool
		wantErr bool
	}{
		{"autodisable yes", "autodisable", "yes", true, true, false},
		{"autodisable no", "autodisable", "no", false, true, false},
		{"autodisable true", "autodisable", "true", true, true, false},
		{"autodisable false", "autodisable", "false", false, true, false},
		{"autodisable invalid", "autodisable", "maybe", nil, true, true},
		{"type int", "type", "int", objtype.ScriptVarTypeInt, true, false},
		{"type loc", "type", "loc", objtype.ScriptVarTypeLoc, true, false},
		{"type string", "type", "string", objtype.ScriptVarTypeString, true, false},
		{"type bogus", "type", "bogus", nil, true, true},
		{"default raw passthrough", "default", "anything", "anything", true, false},
		{"default null", "default", "null", "null", true, false},
		{"unknown key", "unknownkey", "x", nil, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := parseParamConfig(tt.key, tt.value)
			if ok != tt.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tt.wantOK)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("err: got %v, want error=%v", err, tt.wantErr)
			}
			if err == nil && got != tt.wantVal {
				t.Errorf("value: got %#v (%T), want %#v (%T)", got, got, tt.wantVal, tt.wantVal)
			}
		})
	}
}
