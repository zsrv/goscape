package script

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestPointerGroupFind_Content pins spec §7.11: 5 elements in TS order
// (find_player, find_npc, find_loc, find_obj, find_db). Order matters
// because corrupt-slice content is concatenated in this order.
func TestPointerGroupFind_Content(t *testing.T) {
	want := []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db"}
	if !reflect.DeepEqual(PointerGroupFind(), want) {
		t.Fatalf("PointerGroupFind()\n got = %v\nwant = %v", PointerGroupFind(), want)
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
	// the package-internal storage.
	got[0] = "MUTATED"
	if PointerGroupFind()[0] != "find_player" {
		t.Fatalf("PointerGroupFind()[0] = %q after caller mutation; want %q (helper must return a fresh slice)", PointerGroupFind()[0], "find_player")
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

// TestScriptOpcodePointers_LengthParity pins 242 entries at the 274
// pin-advance dee467c8.
// History: 240 at 244 pin 9aadcec4; 245.2 adds IF_SETSCROLLPOS (+1) = 241;
// 254 pin 43e02957 adds SET_PLAYER_OP (+1) = 242 (STAT_TOTAL has NO pointer
// row in TS); 254 pin-advance 2e3bcf43 deletes the BUFFER_FULL,
// NPC_HUNTNEXT and LAST_COORD rows (-3) = 239 (renames BAS_*->*ANIM,
// HINT_PLAYER->HINT_PL, LOWMEMORY->LOWMEM, IF_SETRESUMEBUTTONS->
// IF_ADDRESUMEBUTTON are count-neutral; FINDUID gains corrupt rows only).
// 274 pin-advance dee467c8 adds MINIMAP_TOGGLE, SET_SKILL_LEVEL and
// NPC_DESTINATION rows (+3) = 242 (SETSKINCOLOUR→SETIDKCOLOUR is in-place;
// MAP_LOC has NO pointer row in TS).
// 274 pin-advance 1d25566c adds P_TEMPRUN, P_TRANSMOGRIFY, PLAYERMEMBER,
// STAT_TOTAL, SESSION_LOG and WEALTH_EVENT rows (+6) = 248. FINDHERO gains
// require/require2 and OBJ_FIND gains conditional, both in place.
func TestScriptOpcodePointers_LengthParity(t *testing.T) {
	const wantLen = 248
	if got := len(ScriptOpcodePointers); got != wantLen {
		t.Fatalf("len(ScriptOpcodePointers) = %d, want %d (re-verify against TS ScriptOpcodePointers.ts at pin 1d25566c)", got, wantLen)
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
				Corrupt:     []string{"p_active_player"},
				Corrupt2:    []string{"p_active_player2"},
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

// TestScriptOpcodePointers_KeysAreBoundedOpcodes pins spec §7.9: every
// ScriptOpcodePointers key is in the valid Opcode range — i.e. ≤ the
// max Op* constant defined in pkg/script/opcode.go. Enumerating all
// Op* constants in this test would be brittle; the weaker bound
// (≤ OpTimeSpent, the highest goscape constant at the 254 pin)
// catches typo cases that would assign a wildly-out-of-range value.
//
// If pkg/script/opcode.go adds a new Op* constant with value >
// OpTimeSpent, update this constant.
func TestScriptOpcodePointers_KeysAreBoundedOpcodes(t *testing.T) {
	const maxOp = OpTimeSpent // 10003 at the 254 pin 2e3bcf43
	for op := range ScriptOpcodePointers {
		if op > maxOp {
			t.Errorf("ScriptOpcodePointers[op=%d]: exceeds known max Op* = %d", op, maxOp)
		}
	}
}

// TestPointerGroupFind_AccessorReturnsFreshCopy pins
// NAI-202-D-POINTER-GROUP-FIND-HARDENED: the public PointerGroupFind()
// accessor must return a fresh slice on each call so callers cannot
// mutate package-internal state.
func TestPointerGroupFind_AccessorReturnsFreshCopy(t *testing.T) {
	first := PointerGroupFind()
	want := []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db"}

	if len(first) != len(want) {
		t.Fatalf("len(PointerGroupFind()) = %d, want %d", len(first), len(want))
	}
	for i, name := range want {
		if first[i] != name {
			t.Errorf("PointerGroupFind()[%d] = %q, want %q", i, first[i], name)
		}
	}

	// Mutate the returned slice — must not affect subsequent calls.
	first[0] = "MUTATED"
	second := PointerGroupFind()
	if second[0] != "find_player" {
		t.Errorf("after caller mutation of returned slice, second call returned %q at [0]; want %q (accessor must return fresh copies)", second[0], "find_player")
	}
}

// TestScriptOpcodePointers_CorruptExceptActiveCallSites pins deviation
// NAI-201-D-POINTERS-SPREAD-HELPER. Spec §7.10 asserts BOTH:
//
//	(a) the helper is called exactly 4 times in opcode_pointers.go
//	    (matching TS simple-spread sites at lines 286, 301, 314, 370),
//	(b) the 2 extended-spread entries (NPC_DELAY, NPC_ARRIVEDELAY)
//	    contain the expected 14-element Corrupt slice via literal
//	    expansion.
//
// If a future entry adds another spread site, the count check fails
// and the author updates after re-grepping TS.
func TestScriptOpcodePointers_CorruptExceptActiveCallSites(t *testing.T) {
	// (a) Helper call-site count. The function declaration line is
	// "func corruptExceptActive(" — exclude it via the leading
	// "Corrupt:" prefix.
	src, err := os.ReadFile("opcode_pointers.go")
	if err != nil {
		t.Fatalf("read opcode_pointers.go: %v", err)
	}
	const wantHelperCalls = 4
	got := 0
	for line := range strings.SplitSeq(string(src), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "Corrupt: corruptExceptActive(") || strings.HasPrefix(trim, "Corrupt:corruptExceptActive(") {
			got++
		}
	}
	if got != wantHelperCalls {
		t.Errorf("Corrupt: corruptExceptActive(...) call-site count: got %d, want %d (re-verify TS POINTER_GROUP_FIND simple-spread sites)", got, wantHelperCalls)
	}

	// (b) Extended-spread entries: NPC_DELAY and NPC_ARRIVEDELAY share
	// the same 14-element Corrupt slice shape. Pin the exact contents.
	wantExtendedCorrupt := []string{
		"p_active_player", "p_active_player2",
		"find_player", "find_npc", "find_loc", "find_obj", "find_db",
		"last_com", "last_int", "last_item", "last_slot",
		"last_targetslot", "last_useitem", "last_useslot",
	}
	for _, op := range []Opcode{OpNpcDelay, OpNpcArriveDelay} {
		entry := ScriptOpcodePointers[op]
		if !reflect.DeepEqual(entry.Corrupt, wantExtendedCorrupt) {
			t.Errorf("Op=%d extended-spread Corrupt: got %v, want %v", op, entry.Corrupt, wantExtendedCorrupt)
		}
	}
}

// TestScriptOpcodePointers_254PinAdvanceRows pins the rows changed at the
// 254 pin-advance (TS ScriptOpcodePointers.ts @2e3bcf43): FINDUID gains
// corrupt rows; NPC_FINDHERO / OBJ_ADD / OBJ_TAKEITEM REVERT to their
// pre-254 shapes (undoing the 43e02957-era variants); IF_OPENOVERLAY is
// unchanged. The BUFFER_FULL / NPC_HUNTNEXT / LAST_COORD rows were deleted
// with their ops (their constants no longer exist — absence is enforced by
// TestScriptOpcodePointers_LengthParity + the NoOrphans sweep below).
func TestScriptOpcodePointers_254PinAdvanceRows(t *testing.T) {
	cases := []struct {
		op   Opcode
		want Pointers
		desc string
	}{
		{
			op: OpFindUID,
			want: Pointers{
				Set:         []string{"active_player"},
				Set2:        []string{"active_player2"},
				Corrupt:     []string{"p_active_player"},
				Corrupt2:    []string{"p_active_player2"},
				Conditional: true,
			},
			desc: "FINDUID: corrupt rows added at 2e3bcf43",
		},
		{
			op: OpNpcFindHero,
			want: Pointers{
				Require:     []string{"active_npc"},
				Require2:    []string{"active_npc2"},
				Set:         []string{"active_player"},
				Set2:        []string{"active_player2"},
				Conditional: true,
			},
			desc: "NPC_FINDHERO: reverted to pre-254 shape at 2e3bcf43",
		},
		{
			op: OpObjAdd,
			want: Pointers{
				Require:  []string{"active_player"},
				Require2: []string{"active_player2"},
				Set:      []string{"active_obj"},
				Set2:     []string{"active_obj2"},
			},
			desc: "OBJ_ADD: require2 reverted to active_player2 at 2e3bcf43",
		},
		{
			op: OpObjTakeItem,
			want: Pointers{
				Require:  []string{"active_obj", "active_player"},
				Require2: []string{"active_obj2", "active_player2"},
			},
			desc: "OBJ_TAKEITEM: require2 reverted to active_obj2 at 2e3bcf43",
		},
		{
			op: OpIfOpenOverlay,
			want: Pointers{
				Require:  []string{"active_player"},
				Require2: []string{"active_player2"},
			},
			desc: "IF_OPENOVERLAY: unchanged across the pin-advance",
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

// TestScriptOpcodePointers_NoOrphans verifies that every key in
// ScriptOpcodePointers round-trips through ScriptOpcodeMap — i.e. no
// leftover row exists for a deleted opcode. An orphan key surfaces when an
// opcode is removed from ScriptOpcodeMap but its pointer row is not cleaned up
// (e.g. IF_SETRECOL was deleted in 244).
func TestScriptOpcodePointers_NoOrphans(t *testing.T) {
	// Build the reverse lookup: value → name from ScriptOpcodeMap.
	valueToName := make(map[Opcode]string, len(ScriptOpcodeMap))
	for name, op := range ScriptOpcodeMap {
		valueToName[op] = name
	}
	for op := range ScriptOpcodePointers {
		if _, inMap := valueToName[op]; !inMap {
			// Also accept ops whose String() is in scriptOpcodeMap244Pin —
			// new stubs that have pointer rows but are not yet in map would fail.
			// For now any opcode key must be in ScriptOpcodeMap.
			t.Errorf("ScriptOpcodePointers has orphan key op=%d (%s): not present in ScriptOpcodeMap", op, op.String())
		}
	}
}
