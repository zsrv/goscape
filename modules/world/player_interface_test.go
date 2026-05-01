package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// newIfSetPlayer wires a player with an encryptor + draining pipe so
// IfSet* calls don't deadlock or panic. Returned player is suitable for
// state-write assertions on IfSet*.
func newIfSetPlayer(t *testing.T) *Player {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	drainConn(t, cc)
	return p
}

func TestNewPlayer_TabsAndModalTutorialDefaults(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.modalTutorial != -1 {
		t.Errorf("modalTutorial: got %d, want -1", p.modalTutorial)
	}
	for i, tab := range p.tabs {
		if tab != -1 {
			t.Errorf("tabs[%d]: got %d, want -1", i, tab)
		}
	}
}

func TestIfSetTab_WritesTabsState(t *testing.T) {
	p := newIfSetPlayer(t)
	p.IfSetTab(100, 3)
	if p.tabs[3] != 100 {
		t.Errorf("tabs[3]: got %d, want 100", p.tabs[3])
	}
}

func TestIfSetTab_OutOfRangeTabSilentlyDropped(t *testing.T) {
	p := newIfSetPlayer(t)
	p.IfSetTab(100, 99)
	for i, tab := range p.tabs {
		if tab != -1 {
			t.Errorf("tabs[%d]: got %d, want -1 (out-of-range tab should not write)", i, tab)
		}
	}
}

func TestIfSetTab_NegativeTabSilentlyDropped(t *testing.T) {
	p := newIfSetPlayer(t)
	p.IfSetTab(100, -1)
	for i, tab := range p.tabs {
		if tab != -1 {
			t.Errorf("tabs[%d]: got %d, want -1", i, tab)
		}
	}
}
