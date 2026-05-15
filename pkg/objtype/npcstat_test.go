package objtype

import (
	"reflect"
	"testing"
)

// TestNpcStatMap_Parity pins spec §7.1: NpcStatMap mirrors TS
// src/engine/entity/NpcStat.ts:10-17 verbatim. All 6 uppercase stat
// names map to the canonical NpcStat* index values.
func TestNpcStatMap_Parity(t *testing.T) {
	expected := map[string]int{
		"ATTACK":    NpcStatAttack,
		"DEFENCE":   NpcStatDefence,
		"STRENGTH":  NpcStatStrength,
		"HITPOINTS": NpcStatHitpoints,
		"RANGED":    NpcStatRanged,
		"MAGIC":     NpcStatMagic,
	}
	if !reflect.DeepEqual(NpcStatMap, expected) {
		t.Fatalf("NpcStatMap mismatch\n got = %#v\nwant = %#v", NpcStatMap, expected)
	}
}

// TestNpcStat_IndexValues pins the canonical index values match TS
// enum NpcStat (NpcStat.ts:1-8) and the count constant matches.
func TestNpcStat_IndexValues(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"NpcStatAttack", NpcStatAttack, 0},
		{"NpcStatDefence", NpcStatDefence, 1},
		{"NpcStatStrength", NpcStatStrength, 2},
		{"NpcStatHitpoints", NpcStatHitpoints, 3},
		{"NpcStatRanged", NpcStatRanged, 4},
		{"NpcStatMagic", NpcStatMagic, 5},
		{"NpcStatCount", NpcStatCount, 6},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}
