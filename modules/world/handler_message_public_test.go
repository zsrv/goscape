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

// packPublicChatPayload returns an opcode-158 MESSAGE_PUBLIC payload:
// [color, effect, word-packed(message)].
func packPublicChatPayload(color, effect byte, message string) []byte {
	out := []byte{color, effect}
	pk := packet.NewPacket(nil)
	wordpack.Pack(pk, message)
	return append(out, pk.Data...)
}

// TestHandleMessagePublic_FiresFriendsBridge pins that a valid
// public-chat utterance triggers FriendsBridge.PublicMessage with the
// expected (sessionUUID, coord, decoded message) tuple. Coord is the
// packed coordgrid.PackCoord(level, x, z) value at utterance.
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
	if got.sessionUUID != "uuid-sess-1" {
		t.Errorf("sessionUUID: got %q, want uuid-sess-1", got.sessionUUID)
	}
	wantCoord := coordgrid.PackCoord(0, 3210, 3210)
	if got.coord != wantCoord {
		t.Errorf("coord: got %d, want %d", got.coord, wantCoord)
	}
	if got.message != "Hi" { // wordpack.Unpack applies sentence-case to "hi"
		t.Errorf("message: got %q, want %q (sentence-cased)", got.message, "Hi")
	}
}

// TestHandleMessagePublic_SkipsWhenSessionHeadless pins that the audit
// hook is skipped when p.session == "headless" (unbridged path; tick
// fallback from slice 7 — audit row would be meaningless).
func TestHandleMessagePublic_SkipsWhenSessionHeadless(t *testing.T) {
	p, rec := commonMessagePublicSetup(t)
	p.session = "headless"

	payload := packPublicChatPayload(0, 0, "hi")
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}
	if len(rec.publicMsgs) != 0 {
		t.Errorf("publicMsgs: got %d, want 0 (skipped due to headless session)", len(rec.publicMsgs))
	}
	// In-world propagation must still fire.
	if p.chatBytes == nil {
		t.Errorf("p.chatBytes: got nil, want non-nil (Chat must fire regardless of session)")
	}
}

// TestHandleMessagePublic_SkipsWhenSessionEmpty pins the defensive skip
// when p.session == "". Slice 7 stamps p.session at newPlayer(c) so
// this should never happen in production, but the guard is defensive
// against test paths and future regressions.
func TestHandleMessagePublic_SkipsWhenSessionEmpty(t *testing.T) {
	p, rec := commonMessagePublicSetup(t)
	p.session = ""

	payload := packPublicChatPayload(0, 0, "hi")
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}
	if len(rec.publicMsgs) != 0 {
		t.Errorf("publicMsgs: got %d, want 0 (skipped due to empty session)", len(rec.publicMsgs))
	}
}

// TestHandleMessagePublic_AppliesWordEncFilterToChatBytes pins that
// handleMessagePublic unpacks the inbound text, filters it via s.wordenc,
// repacks the filtered text, and that the repacked bytes (not the raw input)
// end up on p.chatBytes. The audit-log call to friendsBridge.PublicMessage
// is asserted to receive the UNFILTERED text (mirrors TS player.logMessage
// at MessagePublicHandler.ts:32, set BEFORE filtering).
func TestHandleMessagePublic_AppliesWordEncFilterToChatBytes(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s

	// Wire a recordingBridges so we can read the PublicMessage call.
	rec := &recordingBridges{}
	s.friendsBridge = rec
	p.session = "test-uuid"

	// Build a *Filter that masks "anal" → "****".
	jf := makeWordencJagWithBad(t, "anal")
	f, err := encfilter.LoadFromJag(jf)
	if err != nil {
		t.Fatalf("LoadFromJag: %v", err)
	}
	s.wordenc = f

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
