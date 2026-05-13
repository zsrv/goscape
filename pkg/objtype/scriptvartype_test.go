package objtype

import "testing"

func TestScriptVarTypeFromName_KnownNames(t *testing.T) {
	cases := []struct {
		name string
		want ScriptVarType
	}{
		{"int", 105},
		{"autoint", 255},
		{"string", 115},
		{"enum", 103},
		{"obj", 111},
		{"loc", 108},
		{"component", 73},
		{"namedobj", 79},
		{"struct", 74},
		{"boolean", 49},
		{"coord", 99},
		{"category", 121},
		{"spotanim", 116},
		{"npc", 110},
		{"inv", 118},
		{"synth", 80},
		{"seq", 65},
		{"stat", 83},
		{"varp", 86},
		{"player_uid", 112},
		{"npc_uid", 78},
		{"interface", 97},
		{"npc_stat", 254},
		{"idkit", 75},
		{"dbrow", 208},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ScriptVarTypeFromName(tc.name)
			if !ok {
				t.Fatalf("ok=false for known name %q", tc.name)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestScriptVarTypeFromName_Unknown(t *testing.T) {
	got, ok := ScriptVarTypeFromName("not_a_real_type")
	if ok {
		t.Fatalf("ok=true for unknown name, got %d", got)
	}
	if got != 0 {
		t.Fatalf("got %d, want 0 (zero value) for unknown name", got)
	}
}

func TestScriptVarTypeFromName_EmptyString(t *testing.T) {
	got, ok := ScriptVarTypeFromName("")
	if ok {
		t.Fatalf("ok=true for empty name, got %d", got)
	}
	if got != 0 {
		t.Fatalf("got %d, want 0 for empty name", got)
	}
}
