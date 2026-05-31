package world

import "testing"

// h-interface-1: TS Player.openMainModal (Player.ts:1928-1950),
// openChatModal (Player.ts:1952-1972), openSideModal (Player.ts
// :1975-1995), and openMainSideModal (Player.ts:2004-2018) write
// `new IfClose()` eagerly per displaced slot BEFORE refreshModal
// triggers the new-modal open packet. The "close-then-open" wire
// sequence is what makes any client-side IF_CLOSE listener fire
// before the new modal's open packet repaints the screen — without
// it, the client never sees the close signal for the displaced slot.
//
// Goscape's pre-fix OpenMain/OpenChat/OpenSide/OpenMainSide cleared
// the displaced slot's bits but did NOT set refreshModalClose, so
// encodeOut sent only the new IF_OPEN packet — TS-faithful clients
// missed the close-listener wire event.
//
// Post-fix: each method sets p.refreshModalClose = true when
// displacing a slot. encodeOut (player.go:466-477) writes IF_CLOSE
// first, then the new IF_OPEN — matching the TS order.
//
// TS's per-slot IfClose writes (potentially two: one each for the
// two displaced slots) coalesce into goscape's single IF_CLOSE flag,
// because the wire packet is a close-all-modals signal on the client
// — 1 and 2 are functionally identical from the client's perspective.
//
// Self-displacement (e.g. OpenMain with main already open and no
// other slot open) does NOT set the close flag — TS uses |= MAIN
// without a prior-state check, and the new IF_OPEN repaints the slot
// directly.
//
// TUT bit is intentionally ignored by the displacement check — TS
// gates IfClose only on CHAT/MAIN/SIDE, not on TUT (the tutorial
// overlay is independent).
//
// Toggle-revert RED proof: remove the `if displaced {
// p.refreshModalClose = true }` guard in each open method. The
// displacement tests then read p.refreshModalClose=false and fail
// with the cited TS reference.

func TestOpenMain_DisplacesChat_SetsRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 100
	p.refreshModalClose = false

	p.OpenMain(200)

	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (TS Player.openMainModal:1929-1934 writes IfClose when chat was open — h-interface-1)")
	}
	if p.modalMain != 200 {
		t.Errorf("modalMain: got %d, want 200", p.modalMain)
	}
	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1 (cleared by main open)", p.modalChat)
	}
}

func TestOpenMain_DisplacesSide_SetsRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateSide
	p.modalSide = 101
	p.refreshModalClose = false

	p.OpenMain(201)

	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (TS Player.openMainModal:1936-1940 writes IfClose when side was open — h-interface-1)")
	}
	if p.modalMain != 201 {
		t.Errorf("modalMain: got %d, want 201", p.modalMain)
	}
	if p.modalSide != -1 {
		t.Errorf("modalSide: got %d, want -1 (cleared by main open)", p.modalSide)
	}
}

func TestOpenMain_NoDisplacement_DoesNotSetRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateNone
	p.refreshModalClose = false

	p.OpenMain(202)

	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (no slot displaced — TS gates IfClose on prior CHAT/SIDE bit; h-interface-1)")
	}
}

func TestOpenMain_SelfDisplacement_DoesNotSetRefreshModalClose(t *testing.T) {
	// Pre-existing main is replaced by a new main com. TS does not
	// write IfClose for self-displacement (Player.ts:1929-1941 only
	// checks CHAT/SIDE bits).
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain
	p.modalMain = 300
	p.refreshModalClose = false

	p.OpenMain(301)

	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (self-displacement of main is not a close in TS — h-interface-1)")
	}
	if p.modalMain != 301 {
		t.Errorf("modalMain: got %d, want 301 (replaced)", p.modalMain)
	}
}

func TestOpenMain_TutOnly_DoesNotSetRefreshModalClose(t *testing.T) {
	// TUT is independent of CHAT/MAIN/SIDE; TS does not write IfClose
	// when only TUT is open.
	p, _ := newTestPlayer(t)
	p.modalState = modalStateTut
	p.refreshModalClose = false

	p.OpenMain(400)

	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (TUT is not a chat/side slot — TS gates IfClose on CHAT/SIDE only; h-interface-1)")
	}
	if p.modalState&modalStateTut == modalStateNone {
		t.Errorf("modalState: TUT bit cleared (modalState=%#x); TUT must survive a main open — M8 invariant", p.modalState)
	}
}

func TestOpenChat_DisplacesMain_SetsRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain
	p.modalMain = 500
	p.refreshModalClose = false

	p.OpenChat(501)

	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (TS Player.openChatModal:1955-1960 writes IfClose when main was open — h-interface-1)")
	}
	if p.modalChat != 501 {
		t.Errorf("modalChat: got %d, want 501", p.modalChat)
	}
	if p.modalMain != -1 {
		t.Errorf("modalMain: got %d, want -1 (cleared by chat open)", p.modalMain)
	}
}

func TestOpenChat_DisplacesSide_SetsRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateSide
	p.modalSide = 600
	p.refreshModalClose = false

	p.OpenChat(601)

	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (TS Player.openChatModal:1962-1966 writes IfClose when side was open — h-interface-1)")
	}
}

func TestOpenChat_NoDisplacement_DoesNotSetRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateNone
	p.refreshModalClose = false

	p.OpenChat(602)

	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (no slot displaced; h-interface-1)")
	}
}

func TestOpenSide_DisplacesMain_SetsRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain
	p.modalMain = 700
	p.refreshModalClose = false

	p.OpenSide(701)

	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (TS Player.openSideModal:1981-1985 writes IfClose when main was open — h-interface-1)")
	}
}

func TestOpenSide_DisplacesChat_SetsRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 800
	p.refreshModalClose = false

	p.OpenSide(801)

	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (TS Player.openSideModal:1987-1991 writes IfClose when chat was open — h-interface-1)")
	}
}

func TestOpenSide_NoDisplacement_DoesNotSetRefreshModalClose(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalState = modalStateNone
	p.refreshModalClose = false

	p.OpenSide(802)

	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (no slot displaced; h-interface-1)")
	}
}

func TestOpenMainSide_DisplacesChat_SetsRefreshModalClose(t *testing.T) {
	// TS openMainSideModal (Player.ts:2005-2010) writes IfClose only
	// when CHAT was open. MAIN and SIDE are about to be set to new
	// coms — TS uses |= for them with no prior-state IfClose.
	p, _ := newTestPlayer(t)
	p.modalState = modalStateChat
	p.modalChat = 900
	p.refreshModalClose = false

	p.OpenMainSide(901, 902)

	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (TS Player.openMainSideModal:2005-2010 writes IfClose when chat was open — h-interface-1)")
	}
	if p.modalMain != 901 || p.modalSide != 902 {
		t.Errorf("modal slots: main=%d (want 901), side=%d (want 902)", p.modalMain, p.modalSide)
	}
	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1 (cleared by main/side open)", p.modalChat)
	}
}

func TestOpenMainSide_MainSidePriorOnly_DoesNotSetRefreshModalClose(t *testing.T) {
	// Pre-existing main+side replaced silently; TS gates IfClose on
	// CHAT only — replacing main/side themselves is not a close.
	p, _ := newTestPlayer(t)
	p.modalState = modalStateMain | modalStateSide
	p.modalMain = 950
	p.modalSide = 951
	p.refreshModalClose = false

	p.OpenMainSide(960, 961)

	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (TS gates IfClose on prior CHAT bit only; replacing main/side themselves is silent — h-interface-1)")
	}
	if p.modalMain != 960 || p.modalSide != 961 {
		t.Errorf("modal slots: main=%d (want 960), side=%d (want 961)", p.modalMain, p.modalSide)
	}
}
