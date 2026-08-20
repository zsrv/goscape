package hiscore

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestSkills_MatchesEnabledStats(t *testing.T) {
	got := Skills()
	if len(got) != 19 {
		t.Fatalf("Skills(): got %d entries, want 19 enabled stats", len(got))
	}
	for _, s := range got {
		stat := s.Type - 1
		if stat < 0 || stat >= objtype.PlayerStatCount {
			t.Errorf("skill %q: type %d out of stat range", s.Name, s.Type)
			continue
		}
		if !objtype.PlayerStatEnabled[stat] {
			t.Errorf("skill %q: type %d maps to disabled stat %d", s.Name, s.Type, stat)
		}
		if s.Name != objtype.PlayerStatNames[stat] {
			t.Errorf("skill type %d: name %q, want %q", s.Type, s.Name, objtype.PlayerStatNames[stat])
		}
	}
}

func TestSkills_ExcludesOverall(t *testing.T) {
	for _, s := range Skills() {
		if s.Type == 0 || s.Name == SkillOverall {
			t.Fatalf("Skills() must not contain overall, got %+v", s)
		}
	}
}

func TestSkillByName(t *testing.T) {
	tests := []struct {
		name     string
		wantType int
		wantOK   bool
	}{
		{"overall", 0, true},
		{"attack", objtype.PlayerStatAttack + 1, true},
		{"runecraft", objtype.PlayerStatRunecraft + 1, true},
		{"stat18", 0, false}, // disabled reserved slot
		{"stat19", 0, false}, // disabled reserved slot
		{"Attack", 0, false}, // case-sensitive: callers normalize
		{"nonsense", 0, false},
		{"", 0, false},
	}
	for _, tc := range tests {
		got, ok := SkillByName(tc.name)
		if ok != tc.wantOK {
			t.Errorf("SkillByName(%q): ok = %v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if ok && got.Type != tc.wantType {
			t.Errorf("SkillByName(%q): type = %d, want %d", tc.name, got.Type, tc.wantType)
		}
	}
}

func TestTableForType(t *testing.T) {
	if got := TableForType(0); got != "hiscore_large" {
		t.Errorf("TableForType(0) = %q, want hiscore_large", got)
	}
	for _, typ := range []int{1, 5, 21} {
		if got := TableForType(typ); got != "hiscore" {
			t.Errorf("TableForType(%d) = %q, want hiscore", typ, got)
		}
	}
}
