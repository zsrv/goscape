package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestTriggerMapzone_RegisteredScriptEnqueued (T1) — register
// [mapzone,0_50_60]; calling triggerMapzone(50<<6, 60<<6) must enqueue
// exactly one engine-queue entry pointing at the registered script.
func TestTriggerMapzone_RegisteredScriptEnqueued(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[mapzone,0_50_60]", LookupKey: 0x12340001}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.triggerMapzone(50<<6, 60<<6)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (engine triggers must NOT route to primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Script; got != sf {
		t.Errorf("p.engineQueue[0].Script: got %v, want %v", got, sf)
	}
	if got := p.engineQueue[0].Type; got != script.QueueEngine {
		t.Errorf("p.engineQueue[0].Type: got %v, want QueueEngine", got)
	}
}

// TestTriggerMapzone_UnregisteredKeySilent (T2) — no script registered
// for the computed key; trigger must be a silent no-op.
func TestTriggerMapzone_UnregisteredKeySilent(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.triggerMapzone(50<<6, 60<<6)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0", len(p.queue))
	}
	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len: got %d, want 0 (no script registered → silent no-op)", len(p.engineQueue))
	}
}

// TestTriggerZone_KeyShape_Bitmath (T3) — at world coords
// (x=3214, z=3398, level=0), TS bit-math:
//
//	mx = 3214 >> 6      = 50
//	mz = 3398 >> 6      = 53
//	lx = ((3214&0x3f)>>3)<<3 = ((14)>>3)<<3 = 1<<3 = 8
//	lz = ((3398&0x3f)>>3)<<3 = ((6)>>3)<<3  = 0<<3 = 0
//
// Expected key: [zone,0_50_53_8_0].
func TestTriggerZone_KeyShape_Bitmath(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[zone,0_50_53_8_0]", LookupKey: 0x12340002}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.triggerZone(0, 3214, 3398)

	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (key [zone,0_50_53_8_0] must resolve)", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Script; got != sf {
		t.Errorf("p.engineQueue[0].Script: got %v, want %v (key shape mismatch — recompute bit-math)", got, sf)
	}
}

// TestTriggerZoneExit_NoUnderscore_KeyShape (T4) — pins the absence of
// an underscore between `zoneexit` and the level segment. Same bit-math
// as T3; expected key is [zoneexit,0_50_53_8_0].
func TestTriggerZoneExit_NoUnderscore_KeyShape(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[zoneexit,0_50_53_8_0]", LookupKey: 0x12340003}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.triggerZoneExit(0, 3214, 3398)

	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (key [zoneexit,0_50_53_8_0] must resolve)", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Script; got != sf {
		t.Errorf("p.engineQueue[0].Script: got %v, want %v (key shape mismatch — check for stray underscore after `zoneexit`)", got, sf)
	}
}
