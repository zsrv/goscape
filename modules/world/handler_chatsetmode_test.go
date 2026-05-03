package world

import "testing"

// TestHandleChatSetModeAssignsAllThreeFields pins ChatSetModeHandler.ts:7-13:
// the 3 wire bytes are written into Player.publicChat / .privateChat /
// .tradeDuel.
func TestHandleChatSetModeAssignsAllThreeFields(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	installRecordingBridges(s)

	// publicChat=2, privateChat=1, tradeDuel=0
	payload := []byte{2, 1, 0}
	if err := handleChatSetMode(p, payload); err != nil {
		t.Fatalf("handleChatSetMode: %v", err)
	}
	if p.publicChat != 2 {
		t.Errorf("publicChat: got %d, want 2", p.publicChat)
	}
	if p.privateChat != 1 {
		t.Errorf("privateChat: got %d, want 1", p.privateChat)
	}
	if p.tradeDuel != 0 {
		t.Errorf("tradeDuel: got %d, want 0", p.tradeDuel)
	}
}

// TestHandleChatSetModeFiresFriendsBridge pins ChatSetModeHandler.ts:11
// — sendPrivateChatModeToFriendsServer is called with player.username and
// the new privateChat value.
func TestHandleChatSetModeFiresFriendsBridge(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)

	if err := handleChatSetMode(p, []byte{0, 1, 2}); err != nil {
		t.Fatalf("handleChatSetMode: %v", err)
	}
	if len(rec.friends) != 1 {
		t.Fatalf("friends bridge: got %d calls, want 1", len(rec.friends))
	}
	got := rec.friends[0]
	if got.method != "SetChatMode" {
		t.Errorf("method: got %q, want SetChatMode", got.method)
	}
	if got.playerUsername != "alice" {
		t.Errorf("playerUsername: got %q, want alice", got.playerUsername)
	}
	if got.privateChatMode != 1 {
		t.Errorf("privateChatMode: got %d, want 1", got.privateChatMode)
	}
}

// TestHandleChatSetModeIgnoresSocialProtect pins that ChatSetMode is NOT
// gated by socialProtect (TS ChatSetModeHandler.ts has no such gate,
// unlike Friend/Ignore/MessagePrivate).
func TestHandleChatSetModeIgnoresSocialProtect(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)
	p.socialProtect = true

	if err := handleChatSetMode(p, []byte{1, 0, 0}); err != nil {
		t.Fatalf("handleChatSetMode: %v", err)
	}
	if p.publicChat != 1 {
		t.Errorf("publicChat: got %d, want 1 (no socialProtect gate)", p.publicChat)
	}
	if len(rec.friends) != 1 {
		t.Errorf("bridge: got %d calls, want 1 (no socialProtect gate)", len(rec.friends))
	}
}

// TestHandleChatSetModeDoesNotSetSocialProtect pins that ChatSetMode does
// NOT set socialProtect = true (TS handler does not).
func TestHandleChatSetModeDoesNotSetSocialProtect(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	installRecordingBridges(s)

	if err := handleChatSetMode(p, []byte{0, 0, 0}); err != nil {
		t.Fatalf("handleChatSetMode: %v", err)
	}
	if p.socialProtect {
		t.Error("socialProtect: must NOT be set by ChatSetMode")
	}
}

// TestHandleChatSetModeNilServerNoOp pins the goscape defensive guard:
// a Player with no server reference returns nil without panic.
func TestHandleChatSetModeNilServerNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = nil

	if err := handleChatSetMode(p, []byte{1, 1, 1}); err != nil {
		t.Errorf("handleChatSetMode with nil server: got err %v, want nil", err)
	}
	if p.publicChat != 0 {
		t.Errorf("publicChat: got %d, want 0 (no-op on nil server)", p.publicChat)
	}
}
