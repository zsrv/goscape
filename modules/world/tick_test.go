package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestProcessLoginsAllocatesInputTracking pins that newly logged-in
// players have a non-nil InputTracking with a future-scheduled window.
// NAI-73 T6.
func TestProcessLoginsAllocatesInputTracking(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.newPlayers = []*Player{p}
	s.currentTick = 1000

	s.processLogins()

	if p.input == nil {
		t.Fatal("p.input: must be non-nil after processLogins")
	}
	if p.input.player != p {
		t.Error("p.input.player back-pointer must equal p")
	}
	wantMin := 1000 + inputTrackingRate - inputTrackingJitterRange
	wantMax := 1000 + inputTrackingRate + inputTrackingJitterRange
	if p.input.startTrackingAt < wantMin || p.input.startTrackingAt > wantMax {
		t.Errorf("startTrackingAt: got %d, want in [%d, %d]", p.input.startTrackingAt, wantMin, wantMax)
	}
	if got, want := p.input.endTrackingAt, p.input.startTrackingAt+inputTrackingTime; got != want {
		t.Errorf("endTrackingAt: got %d, want %d (startTrackingAt + inputTrackingTime)", got, want)
	}
	if got, want := p.session, "headless"; got != want {
		t.Errorf("p.session default: got %q, want %q", got, want)
	}
}

// seedVarpTypesByType installs a VarpTypeConfigs with the given
// (type, name, protect) entries on s, for tick-init seed-loop tests.
// Distinct from seedVarpTypes(s, transmit) at script_test.go:368, which
// only handles a single transmit-flag varp.
func seedVarpTypesByType(s *Server, entries []struct {
	Type    objtype.ScriptVarType
	Name    string
	Protect bool
}) {
	configs := make([]*objtype.VarPlayerType, len(entries))
	configNames := make(map[string]int, len(entries))
	for i, e := range entries {
		c := objtype.NewVarPlayerType(i)
		c.Type = e.Type
		c.DebugName = e.Name
		c.Protect = e.Protect
		configs[i] = c
		// Match parseVarpTypes: only insert when the name is non-empty.
		if e.Name != "" {
			configNames[e.Name] = i
		}
	}
	s.varpTypes = &objtype.VarpTypeConfigs{ConfigNames: configNames, Configs: configs}
}

func TestInitPlayerVarps_NilVarpTypes_NoOp(t *testing.T) {
	s := newTestServer(t)
	s.varpTypes = nil
	p := &Player{}

	s.initPlayerVarps(p)

	if p.varps != nil {
		t.Errorf("varps: got non-nil, want nil (defensive no-op)")
	}
	if p.varpsString != nil {
		t.Errorf("varpsString: got non-nil, want nil (defensive no-op)")
	}
}

func TestInitPlayerVarps_SeedsByType_IntZero(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypesByType(s, []struct {
		Type    objtype.ScriptVarType
		Name    string
		Protect bool
	}{
		{Type: objtype.ScriptVarTypeInt, Name: "int_var"},
	})
	p := &Player{}

	s.initPlayerVarps(p)

	if p.Varp(0) != 0 {
		t.Errorf("INT varp: got %d, want 0", p.Varp(0))
	}
}

func TestInitPlayerVarps_SeedsByType_NpcUidMinusOne(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypesByType(s, []struct {
		Type    objtype.ScriptVarType
		Name    string
		Protect bool
	}{
		{Type: objtype.ScriptVarTypeNpcUid, Name: "npc_uid_var"},
	})
	p := &Player{}

	s.initPlayerVarps(p)

	if p.Varp(0) != -1 {
		t.Errorf("npc_uid varp: got %d, want -1", p.Varp(0))
	}
}

func TestInitPlayerVarps_SeedsByType_StringEmpty(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypesByType(s, []struct {
		Type    objtype.ScriptVarType
		Name    string
		Protect bool
	}{
		{Type: objtype.ScriptVarTypeString, Name: "string_var"},
	})
	p := &Player{}

	s.initPlayerVarps(p)

	if len(p.varpsString) != 1 {
		t.Fatalf("varpsString length: got %d, want 1", len(p.varpsString))
	}
	if p.varpsString[0] != "" {
		t.Errorf("string varp: got %q, want \"\"", p.varpsString[0])
	}
}

func TestInitPlayerVarps_LengthMatchesRegistry(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypesByType(s, []struct {
		Type    objtype.ScriptVarType
		Name    string
		Protect bool
	}{
		{Type: objtype.ScriptVarTypeInt, Name: "a"},
		{Type: objtype.ScriptVarTypePlayerUid, Name: "b"},
		{Type: objtype.ScriptVarTypeString, Name: "c"},
	})
	p := &Player{}

	s.initPlayerVarps(p)

	if len(p.varps) != 3 || len(p.varpsString) != 3 {
		t.Errorf("lengths: varps=%d varpsString=%d, want both 3", len(p.varps), len(p.varpsString))
	}
	if p.Varp(0) != 0 {
		t.Errorf("varp[0] (INT): got %d, want 0", p.Varp(0))
	}
	if p.Varp(1) != -1 {
		t.Errorf("varp[1] (PlayerUid): got %d, want -1", p.Varp(1))
	}
	if p.varpsString[2] != "" {
		t.Errorf("varpsString[2] (String): got %q, want \"\"", p.varpsString[2])
	}
}
