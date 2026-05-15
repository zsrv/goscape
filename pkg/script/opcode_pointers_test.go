package script

import (
	"reflect"
	"testing"
)

// TestPointerGroupFind_Content pins spec §7.11: 5 elements in TS order
// (find_player, find_npc, find_loc, find_obj, find_db). Order matters
// because corrupt-slice content is concatenated in this order.
func TestPointerGroupFind_Content(t *testing.T) {
	want := []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db"}
	if !reflect.DeepEqual(PointerGroupFind, want) {
		t.Fatalf("PointerGroupFind\n got = %v\nwant = %v", PointerGroupFind, want)
	}
}

// TestCorruptExceptActive_Behavior pins the helper's contract: returns
// PointerGroupFind ++ extras as a fresh slice (caller mutations must
// not alias PointerGroupFind).
func TestCorruptExceptActive_Behavior(t *testing.T) {
	got := corruptExceptActive("last_com", "last_int")
	want := []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db", "last_com", "last_int"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got = %v, want = %v", got, want)
	}

	// Independence: mutating the returned slice must not corrupt
	// PointerGroupFind.
	got[0] = "MUTATED"
	if PointerGroupFind[0] != "find_player" {
		t.Fatalf("PointerGroupFind[0] = %q after caller mutation; want %q (helper must return a fresh slice)", PointerGroupFind[0], "find_player")
	}
}

// TestPointers_ZeroValue pins that a missing entry returns a useful
// zero value (all-nil slices, Conditional=false) — this is what
// callers see on map miss. Matches TS `undefined` semantics via Go map miss.
func TestPointers_ZeroValue(t *testing.T) {
	var p Pointers
	if p.Require != nil || p.Require2 != nil || p.Set != nil || p.Set2 != nil ||
		p.Corrupt != nil || p.Corrupt2 != nil || p.Conditional {
		t.Fatalf("Pointers{} should be all-zero; got %+v", p)
	}
}

// TestScriptOpcodePointers_LengthParity pins spec §7.7: 237 entries
// verified at plan-write via
//
//	grep -c "\[ScriptOpcode\." src/engine/script/ScriptOpcodePointers.ts
func TestScriptOpcodePointers_LengthParity(t *testing.T) {
	const wantLen = 237
	if got := len(ScriptOpcodePointers); got != wantLen {
		t.Fatalf("len(ScriptOpcodePointers) = %d, want %d (re-verify against TS ScriptOpcodePointers.ts)", got, wantLen)
	}
}

// TestScriptOpcodePointers_SpotChecks pins spec §7.8: representative
// entries across the file. Each case verified against the TS line cited.
func TestScriptOpcodePointers_SpotChecks(t *testing.T) {
	cases := []struct {
		op   Opcode
		want Pointers
		desc string
	}{
		{
			op:   OpAllowDesign,
			want: Pointers{Require: []string{"active_player"}},
			desc: "TS:17 — simplest Require-only shape",
		},
		{
			op: OpAnim,
			want: Pointers{
				Require:  []string{"active_player"},
				Require2: []string{"active_player2"},
			},
			desc: "TS:20 — Require + Require2",
		},
		{
			op: OpFindUID,
			want: Pointers{
				Set:         []string{"active_player"},
				Set2:        []string{"active_player2"},
				Conditional: true,
			},
			desc: "TS:103 — Set + Set2 + Conditional=true",
		},
		{
			op:   OpHuntAll,
			want: Pointers{Set: []string{"find_player"}},
			desc: "TS:140 — Set-only shape",
		},
		{
			op: OpHuntNext,
			want: Pointers{
				Require:     []string{"find_player"},
				Require2:    []string{"find_player"},
				Set:         []string{"active_player"},
				Set2:        []string{"active_player2"},
				Conditional: true,
			},
			desc: "TS:143 — full quartet + Conditional",
		},
		{
			op: OpPArriveDelay,
			want: Pointers{
				Require: []string{"p_active_player"},
				Corrupt: corruptExceptActive(
					"last_com", "last_int", "last_item", "last_slot",
					"last_targetslot", "last_useitem", "last_useslot",
				),
			},
			desc: "TS:282 — simple POINTER_GROUP_FIND spread via helper",
		},
		{
			op: OpPPauseButton,
			want: Pointers{
				Require: []string{"p_active_player"},
				Set:     []string{"last_com"},
				Corrupt: corruptExceptActive(
					"last_int", "last_item", "last_slot",
					"last_targetslot", "last_useitem", "last_useslot",
				),
			},
			desc: "TS:365 — Require + Set + simple spread",
		},
		{
			op: OpNpcDelay,
			want: Pointers{
				Require:  []string{"active_npc"},
				Require2: []string{"active_npc2"},
				Corrupt: []string{
					"p_active_player", "p_active_player2",
					"find_player", "find_npc", "find_loc", "find_obj", "find_db",
					"last_com", "last_int", "last_item", "last_slot",
					"last_targetslot", "last_useitem", "last_useslot",
				},
			},
			desc: "TS:567 — extended spread (literal expansion, NOT helper)",
		},
		{
			op: OpNpcArriveDelay,
			want: Pointers{
				Require:  []string{"active_npc"},
				Require2: []string{"active_npc2"},
				Corrupt: []string{
					"p_active_player", "p_active_player2",
					"find_player", "find_npc", "find_loc", "find_obj", "find_db",
					"last_com", "last_int", "last_item", "last_slot",
					"last_targetslot", "last_useitem", "last_useslot",
				},
			},
			desc: "TS:709 — extended spread (literal expansion, NOT helper)",
		},
		{
			op:   OpDbListAll,
			want: Pointers{Set: []string{"find_db"}},
			desc: "TS:976 — late-file entry (Db family)",
		},
		{
			op:   OpDbListAllWithCount,
			want: Pointers{Set: []string{"find_db"}},
			desc: "TS:979 — last entry in TS",
		},
	}
	for _, c := range cases {
		got, present := ScriptOpcodePointers[c.op]
		if !present {
			t.Errorf("%s: ScriptOpcodePointers[op=%d]: missing", c.desc, c.op)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got = %+v\nwant = %+v", c.desc, got, c.want)
		}
	}
}
