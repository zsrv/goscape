package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestEnqueueQueueEngineRoutesToEngineQueue pins NAI-144 routing: a
// QueueEngine enqueue must land in p.engineQueue, never in p.queue.
func TestEnqueueQueueEngineRoutesToEngineQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[engine,test]", LookupKey: 0xdeadbeef}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	// EnqueueScriptArgs resolves via GetByID (index slot, not LookupKey);
	// Register appended the script at index 0.
	if err := p.EnqueueScriptArgs(0, 0, nil, nil, script.QueueEngine); err != nil {
		t.Fatalf("EnqueueScriptArgs: unexpected error: %v", err)
	}

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (QueueEngine must NOT route to primary queue)", len(p.queue))
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

// TestEnqueueQueueNormalDoesNotRouteToEngineQueue is a regression fence:
// QueueNormal must continue to land in p.queue (not p.engineQueue) after
// the NAI-144 switch is added.
func TestEnqueueQueueNormalDoesNotRouteToEngineQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[normal,test]", LookupKey: 0xc0ffee}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	// EnqueueScriptArgs resolves via GetByID (index slot, not LookupKey);
	// Register appended the script at index 0.
	if err := p.EnqueueScriptArgs(0, 0, nil, nil, script.QueueNormal); err != nil {
		t.Fatalf("EnqueueScriptArgs: unexpected error: %v", err)
	}

	if len(p.queue) != 1 {
		t.Errorf("p.queue len: got %d, want 1 (QueueNormal must route to primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len: got %d, want 0", len(p.engineQueue))
	}
}
