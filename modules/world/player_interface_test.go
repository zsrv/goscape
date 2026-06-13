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

// TestMinimapToggle_WireEncoding pins MINIMAP_TOGGLE (opcode 194, PayloadSize=1):
// MinimapToggle(2) must produce exactly 2 bytes — 1 encrypted opcode + 1 payload
// byte holding the minimapType value. TS MinimapToggleEncoder.ts @dee467c8: buf.p1(type).
func TestMinimapToggle_WireEncoding(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	p.MinimapToggle(2)
	p.client.flushWrite()
	got := <-received

	// 2 bytes: 1 encrypted opcode + 1 payload (P1 minimapType=2).
	if len(got) != 2 {
		t.Fatalf("wire: got %d bytes, want 2", len(got))
	}
	// got[1] = P1(minimapType=2)
	if got[1] != 0x02 {
		t.Errorf("minimapType: got 0x%02X, want 0x02", got[1])
	}
}

// TestIfSetColour_AppliesRgb24to15_OnWire verifies that IfSetColour converts
// the 24-bit colour argument to a 15-bit value before emitting it on the wire.
//
// OpIfSetColour: Opcode=183 (274), PayloadSize=4 (fixed).
// Wire = encrypted_opcode(1) + P2(com)(2) + P2(rgb15)(2) = 5 bytes total.
//
// Input colour 0xFF0000 (red) must map to rgb15=0x7C00.
// Currently FAILS: the writer emits the raw 16-bit low-order bits of 0xFF0000
// (= 0xFF00) instead of 0x7C00, so got[3..4] = [0xFF, 0x00] not [0x7C, 0x00].
func TestIfSetColour_AppliesRgb24to15_OnWire(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	p.IfSetColour(12, 0xFF0000)
	p.client.flushWrite()
	got := <-received

	// 5 bytes: 1 encrypted opcode + 4 payload (P2 com + P2 rgb15).
	if len(got) != 5 {
		t.Fatalf("wire: got %d bytes, want 5", len(got))
	}
	// got[1..2] = P2(com=12)
	if got[1] != 0x00 || got[2] != 0x0c {
		t.Errorf("com: got [0x%02X 0x%02X], want [0x00 0x0C]", got[1], got[2])
	}
	// got[3..4] = P2(rgb15(0xFF0000)) = P2(0x7C00) = [0x7C, 0x00]
	if got[3] != 0x7C || got[4] != 0x00 {
		t.Errorf("colour: got [0x%02X 0x%02X], want [0x7C 0x00] (rgb15 of 0xFF0000)", got[3], got[4])
	}
}
