package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/wordenc/encfilter"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// commonMessagePublicSetup wires a player against a server with
// recording bridges and a known username + session. Mirrors
// commonMessagePrivateSetup in handler_message_private_test.go.
func commonMessagePublicSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	p.session = "uuid-sess-1"
	rec := installRecordingBridges(s)
	return p, rec
}

// packPublicChatPayload returns an opcode-171 MESSAGE_PUBLIC payload:
// [color, effect, word-packed(message)].
func packPublicChatPayload(color, effect byte, message string) []byte {
	out := []byte{color, effect}
	pk := packet.NewPacket(nil)
	wordpack.Pack(pk, message)
	return append(out, pk.Data...)
}

// TestHandleMessagePublic_FiresFriendsBridge pins that a valid
// public-chat utterance triggers FriendsBridge.PublicMessage with the
// expected (username, coord, decoded message) tuple.
// rev-244 re-key: Username is player.username, not session UUID.
// TS World.ts:1620-1628 logPublicChat keys by player.username.
// Coord is the packed coordgrid.PackCoord(level, x, z) value at utterance.
func TestHandleMessagePublic_FiresFriendsBridge(t *testing.T) {
	p, rec := commonMessagePublicSetup(t)
	// Move the player to a known coord so PackCoord output is deterministic.
	p.level, p.x, p.z = 0, 3210, 3210

	payload := packPublicChatPayload(0, 0, "hi")
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}

	if len(rec.publicMsgs) != 1 {
		t.Fatalf("publicMsgs: got %d, want 1", len(rec.publicMsgs))
	}
	got := rec.publicMsgs[0]
	// rev-244: username, not session UUID
	if got.username != "alice" {
		t.Errorf("username: got %q, want alice (rev-244 re-key: keyed by username, not session UUID; TS World.ts:1620-1628)", got.username)
	}
	wantCoord := coordgrid.PackCoord(0, 3210, 3210)
	if got.coord != wantCoord {
		t.Errorf("coord: got %d, want %d", got.coord, wantCoord)
	}
	if got.message != "Hi" { // wordpack.Unpack applies sentence-case to "hi"
		t.Errorf("message: got %q, want %q (sentence-cased)", got.message, "Hi")
	}
}

// TestPublicChatLog_UsernameKeyed pins the rev-244 B5 contract:
// (1) PublicMessage carries Username == p.username (not session UUID),
// (2) a player with empty session STILL logs (225-era session gate is gone;
//
//	TS 244 gates only on logMessage != null, World.ts:677-679),
//
// (3) a player with "headless" session STILL logs (same gate removal).
func TestPublicChatLog_UsernameKeyed(t *testing.T) {
	t.Run("username_carried", func(t *testing.T) {
		p, rec := commonMessagePublicSetup(t)
		p.level, p.x, p.z = 0, 3200, 3200
		payload := packPublicChatPayload(0, 0, "hello")
		if err := handleMessagePublic(p, payload); err != nil {
			t.Fatalf("handleMessagePublic: %v", err)
		}
		if len(rec.publicMsgs) != 1 {
			t.Fatalf("publicMsgs: got %d, want 1", len(rec.publicMsgs))
		}
		if rec.publicMsgs[0].username != "alice" {
			t.Errorf("Username: got %q, want alice (keyed by username, not session UUID)", rec.publicMsgs[0].username)
		}
	})

	t.Run("empty_session_still_logs", func(t *testing.T) {
		// 225-era gate `p.session != ""` is removed. Player with empty session
		// must still emit a PublicMessage row (TS only gates on logMessage != null).
		p, rec := commonMessagePublicSetup(t)
		p.session = "" // was skipped in 225; must fire in 244
		payload := packPublicChatPayload(0, 0, "hi")
		if err := handleMessagePublic(p, payload); err != nil {
			t.Fatalf("handleMessagePublic: %v", err)
		}
		if len(rec.publicMsgs) != 1 {
			t.Errorf("publicMsgs: got %d, want 1 (session gate removed in rev-244, TS World.ts:677-679)", len(rec.publicMsgs))
		}
		// In-world propagation must still fire.
		if p.chatBytes == nil {
			t.Errorf("p.chatBytes: got nil, want non-nil (Chat must fire regardless of session)")
		}
	})

	t.Run("headless_session_still_logs", func(t *testing.T) {
		// 225-era gate `p.session != "headless"` is removed. Player with
		// "headless" session must still emit a PublicMessage row.
		p, rec := commonMessagePublicSetup(t)
		p.session = "headless" // was skipped in 225; must fire in 244
		payload := packPublicChatPayload(0, 0, "hi")
		if err := handleMessagePublic(p, payload); err != nil {
			t.Fatalf("handleMessagePublic: %v", err)
		}
		if len(rec.publicMsgs) != 1 {
			t.Errorf("publicMsgs: got %d, want 1 (headless session gate removed in rev-244)", len(rec.publicMsgs))
		}
	})
}

// TestHandleMessagePublic_AppliesWordEncFilterToChatBytes pins that
// handleMessagePublic unpacks the inbound text, filters it via s.wordenc,
// repacks the filtered text, and that the repacked bytes (not the raw input)
// end up on p.chatBytes. The audit-log call to friendsBridge.PublicMessage
// is asserted to receive the UNFILTERED text (mirrors TS player.logMessage
// at MessagePublicHandler.ts:32, set BEFORE filtering).
func TestHandleMessagePublic_AppliesWordEncFilterToChatBytes(t *testing.T) {
	p, rec := commonMessagePublicSetup(t)

	// Build a *Filter that masks "anal" → "****".
	jf := makeWordencJagWithBad(t, "anal")
	f, err := encfilter.LoadFromJag(jf)
	if err != nil {
		t.Fatalf("LoadFromJag: %v", err)
	}
	p.client.server.wordenc = f

	// Word-pack "anal" so the payload looks like a real client packet.
	bufIn := packet.NewPacket(nil)
	wordpack.Pack(bufIn, "anal")
	packed := bufIn.Bytes()

	// Wire layout: byte 0 = color (0), byte 1 = effect (0), then packed bytes.
	payload := append([]byte{0, 0}, packed...)
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}

	// chatBytes must be the wordpack-packed form of "****", not "anal".
	wantPacked := func() []byte {
		out := packet.NewPacket(nil)
		wordpack.Pack(out, "****")
		return out.Bytes()
	}()
	if !bytes.Equal(p.chatBytes, wantPacked) {
		t.Errorf("p.chatBytes:\n  got  %x\n  want %x", p.chatBytes, wantPacked)
	}

	// PublicMessage audit-log MUST receive the unfiltered text.
	if len(rec.publicMsgs) != 1 {
		t.Fatalf("expected 1 PublicMessage call, got %d", len(rec.publicMsgs))
	}
	if rec.publicMsgs[0].message != "Anal" {
		t.Errorf("audit-log message: got %q, want %q (unfiltered, sentence-cased)", rec.publicMsgs[0].message, "Anal")
	}
}
