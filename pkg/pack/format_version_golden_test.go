package pack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// packFormatGolden is the SHA-256 of the packers' output over the fixed
// fixtures in TestPackFormatVersionGolden, at FormatVersion 1.
//
// It exists to make FormatVersion enforceable instead of aspirational. A
// format version is only as good as someone remembering to bump it, and that
// is exactly the discipline that failed on rev-274: the config opcodes moved
// in an engine sync, the per-family byte tests correctly failed and were
// updated, and nothing connected that to "the cached outputs are now wrong".
// This closes the loop — CI names the constant instead of relying on memory.
const packFormatGolden = "8a34d8213bb4741c8e6dc31a67e4a0760909bd20713d3472785dc10baa8641ec"

// TestPackFormatVersionGolden hashes the encoded output of one deterministic
// config per family. Any change to a packed byte moves the hash.
//
// IF THIS TEST FAILS AND THE CHANGE IS INTENDED:
//  1. Bump pack.FormatVersion in format_stamp.go (add a history line).
//  2. Replace packFormatGolden above with the hash the failure reports.
//
// Both steps, not just the second. Updating only the golden re-arms the exact
// trap this guards: every existing data/pack tree in the wild would keep its
// stale artifacts, because the recorded format still matches.
//
// Coverage is deliberately narrow — one small config per family, exercising
// the opcode-emission path rather than breadth of content. Breadth is what the
// per-family tests and the byte-parity gate against the reference server are
// for; this test only has to notice that SOMETHING moved.
//
// The families chosen are the ones the rev-274 sync actually broke, minus the
// fields that do not exist at this pin: no check_invcat (server opcode 18
// arrives with the 289 sync) and no reachforward/duplicatebehaviour on seq.
// The point is not which fields are covered but that a moved byte is noticed,
// so any stable set works.
func TestPackFormatVersionGolden(t *testing.T) {
	h := sha256.New()

	add := func(name string, data []byte) {
		fmt.Fprintf(h, "%s:%d:", name, len(data))
		h.Write(data)
	}

	// --- hunt: exercises the extracheck_var opcode window.
	{
		pf := buildHuntPF("g_hunt_var")
		cfgs := map[string][]ConfigLine{
			"g_hunt_var": {
				{Key: "type", Value: objtype.HuntModePlayer},
				{Key: "extracheck_var", Value: huntCheckVarParsed{varp: 7, condition: ">", val: 10}},
			},
		}
		pd, err := packHuntConfigs(cfgs, pf)
		if err != nil {
			t.Fatalf("hunt extracheck: %v", err)
		}
		add("hunt_extracheck", pd.Dat.Data)
	}

	// --- npc: exercises wanderrange/maxrange and the category index.
	{
		npcPack := npcOneSlotPack("g_npc")
		cfgs := map[string][]ConfigLine{
			"g_npc": {
				{Key: "wanderrange", Value: 5},
				{Key: "maxrange", Value: 10},
				{Key: "category", Value: 3},
			},
		}
		server, client, err := packNpcConfigs(cfgs, npcPack)
		if err != nil {
			t.Fatalf("npc: %v", err)
		}
		add("npc_server", server.Dat.Data)
		add("npc_client", client.Dat.Data)
	}

	// --- seq: exercises the animation opcode path.
	{
		cfgs := map[string][]ConfigLine{
			"g_seq": {
				{Key: "loops", Value: 5},
				{Key: "priority", Value: 4},
			},
		}
		server, client := packSeqConfigs(cfgs, seqOneSlotPack("g_seq"))
		add("seq_server", server.Dat.Data)
		add("seq_client", client.Dat.Data)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != packFormatGolden {
		t.Errorf(`packed output changed (FormatVersion currently %d).

  got  %s
  want %s

If this change is intended, do BOTH of these:
  1. bump pack.FormatVersion in format_stamp.go (and add a history line)
  2. set packFormatGolden in this file to the "got" hash above

Updating only the golden leaves every existing data/pack tree carrying stale
artifacts, because their recorded format would still match.`,
			FormatVersion, got, packFormatGolden)
	}
}
