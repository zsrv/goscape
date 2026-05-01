package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
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

func TestIsComponentVisible_NilComponentReturnsFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.IsComponentVisible(nil) {
		t.Errorf("IsComponentVisible(nil): got true, want false")
	}
}

func TestIsComponentVisible_MatchesModalMain(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain
	p.modalMain = 200
	com := &objtype.ComponentType{RootLayer: 200, ButtonType: objtype.Button}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (modalMain=200, RootLayer=200)")
	}
}

func TestIsComponentVisible_MainBitOffEvenWithMatchingId(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = 0
	p.modalMain = 200
	com := &objtype.ComponentType{RootLayer: 200}
	if p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got true, want false (modalState=0)")
	}
}

func TestIsComponentVisible_MatchesModalChat(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 300
	com := &objtype.ComponentType{RootLayer: 300}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true")
	}
}

func TestIsComponentVisible_MatchesModalSide(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateSide
	p.modalSide = 400
	com := &objtype.ComponentType{RootLayer: 400}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true")
	}
}

func TestIsComponentVisible_MatchesTab(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.tabs[5] = 42
	com := &objtype.ComponentType{RootLayer: 42}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (tabs[5]=42)")
	}
}

func TestIsComponentVisible_MatchesTabAtIndexZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.tabs[0] = 99
	com := &objtype.ComponentType{RootLayer: 99}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (tabs[0]=99)")
	}
}

func TestIsComponentVisible_MatchesTabAtIndexThirteen(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.tabs[13] = 77
	com := &objtype.ComponentType{RootLayer: 77}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (tabs[13]=77)")
	}
}

func TestIsComponentVisible_TabAllNegOneMisses(t *testing.T) {
	p, _ := newTestPlayer(t)
	com := &objtype.ComponentType{RootLayer: 10}
	if p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got true, want false (all tabs default -1)")
	}
}

func TestIsComponentVisible_MatchesModalTutorial(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalTutorial = 99
	com := &objtype.ComponentType{RootLayer: 99}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (modalTutorial=99)")
	}
}

func TestIsComponentVisible_NoMatchReturnsFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain | modalStateChat | modalStateSide
	p.modalMain = 1
	p.modalChat = 2
	p.modalSide = 3
	p.modalTutorial = 4
	for i := range p.tabs {
		p.tabs[i] = 100 + i
	}
	com := &objtype.ComponentType{RootLayer: 999}
	if p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got true, want false (no slot matches)")
	}
}
