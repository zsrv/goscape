package world

import (
	"bytes"
	"sync"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/telemetry"
	"github.com/zsrv/goscape/pkg/wordenc/encfilter"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// captureEmitter records emitted envelopes for assertions. Safe for
// cross-goroutine use (smoke tests capture from gRPC handler goroutines).
type captureEmitter struct {
	mu              sync.Mutex
	worldEnvs       []*eventspb.WorldEnvelope
	playerInputEnvs []*eventspb.PlayerInputEnvelope
}

func (c *captureEmitter) EmitAuth(*eventspb.AuthEnvelope) {}
func (c *captureEmitter) EmitWorld(env *eventspb.WorldEnvelope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.worldEnvs = append(c.worldEnvs, env)
}
func (c *captureEmitter) EmitPlayerInput(env *eventspb.PlayerInputEnvelope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.playerInputEnvs = append(c.playerInputEnvs, env)
}
func (c *captureEmitter) EmitWealth(*eventspb.WealthEnvelope) {}

// publicChats returns the captured WorldEnvelopes carrying a PublicChatEvent.
func (c *captureEmitter) publicChats() []*eventspb.WorldEnvelope {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []*eventspb.WorldEnvelope
	for _, e := range c.worldEnvs {
		if e.GetPublicChat() != nil {
			out = append(out, e)
		}
	}
	return out
}

// commonMessagePublicSetup wires a player against a server and installs
// a capture telemetry emitter. Chat is Kafka-only (spec
// docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md): the
// friends-bridge audit path is retired, so assertions read the emitter.
func commonMessagePublicSetup(t *testing.T) (*Player, *captureEmitter) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	p.session = "uuid-sess-1"
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)
	return p, cap
}

// packPublicChatPayload returns an opcode-171 MESSAGE_PUBLIC payload:
// [color, effect, word-packed(message)].
func packPublicChatPayload(color, effect byte, message string) []byte {
	out := []byte{color, effect}
	pk := packet.NewPacket(nil)
	wordpack.Pack(pk, message)
	return append(out, pk.Data...)
}

// TestHandleMessagePublic_EmitsPublicChatEvent pins that a valid
// public-chat utterance emits exactly one PublicChatEvent with the
// (session_uuid, coord, decoded message) tuple TS used to persist to
// public_chat (World.ts:1567-1574 @2e3bcf43). Chat is Kafka-only —
// documented TS divergence, spec
// docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md.
func TestHandleMessagePublic_EmitsPublicChatEvent(t *testing.T) {
	p, cap := commonMessagePublicSetup(t)
	// Move the player to a known coord so PackCoord output is deterministic.
	p.level, p.x, p.z = 0, 3210, 3210

	payload := packPublicChatPayload(0, 0, "hi")
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}

	envs := cap.publicChats()
	if len(envs) != 1 {
		t.Fatalf("PublicChatEvent envelopes: got %d, want 1", len(envs))
	}
	got := envs[0].GetPublicChat()
	// rev-254 A3: session UUID, not username
	if got.SessionUuid != "uuid-sess-1" {
		t.Errorf("SessionUuid: got %q, want uuid-sess-1 (rev-254 A3 re-key; TS World.ts:1567-1574 @2e3bcf43)", got.SessionUuid)
	}
	wantCoord := int32(coordgrid.PackCoord(0, 3210, 3210))
	if got.Coord != wantCoord {
		t.Errorf("Coord: got %d, want %d", got.Coord, wantCoord)
	}
	if got.Text != "Hi" { // wordpack.Unpack applies sentence-case to "hi"
		t.Errorf("Text: got %q, want %q (sentence-cased)", got.Text, "Hi")
	}
	if envs[0].AccountId != p.accountID {
		t.Errorf("AccountId: got %d, want %d", envs[0].AccountId, p.accountID)
	}
}

// TestPublicChatLog_SessionKeyed pins the rev-254 A3 contract:
// (1) the emitted PublicChatEvent carries the session UUID (TS
// World.ts:1567-1574 @2e3bcf43 `session_uuid: player.session`), NOT the
// username, (2) a player with empty session STILL emits as 'headless'
// (the only gate is logMessage != null, World.ts:629-631 @2e3bcf43; ""
// maps to the TS Player.session ctor default 'headless'), (3) a player
// with "headless" session STILL emits (no session gate).
func TestPublicChatLog_SessionKeyed(t *testing.T) {
	t.Run("session_uuid_carried", func(t *testing.T) {
		p, cap := commonMessagePublicSetup(t)
		p.level, p.x, p.z = 0, 3200, 3200
		payload := packPublicChatPayload(0, 0, "hello")
		if err := handleMessagePublic(p, payload); err != nil {
			t.Fatalf("handleMessagePublic: %v", err)
		}
		envs := cap.publicChats()
		if len(envs) != 1 {
			t.Fatalf("PublicChatEvent envelopes: got %d, want 1", len(envs))
		}
		if got := envs[0].GetPublicChat().SessionUuid; got != "uuid-sess-1" {
			t.Errorf("SessionUuid: got %q, want uuid-sess-1 (keyed by session UUID, not username)", got)
		}
	})

	t.Run("empty_session_emits_as_headless", func(t *testing.T) {
		// No session gate: TS only gates on logMessage != null. An
		// unassigned session ("" in Go) relays as 'headless' (the TS
		// Player.ts:311 ctor default).
		p, cap := commonMessagePublicSetup(t)
		p.session = ""
		payload := packPublicChatPayload(0, 0, "hi")
		if err := handleMessagePublic(p, payload); err != nil {
			t.Fatalf("handleMessagePublic: %v", err)
		}
		envs := cap.publicChats()
		if len(envs) != 1 {
			t.Fatalf("PublicChatEvent envelopes: got %d, want 1 (no session gate, TS World.ts:629-631 @2e3bcf43)", len(envs))
		}
		if got := envs[0].GetPublicChat().SessionUuid; got != "headless" {
			t.Errorf("SessionUuid: got %q, want headless (TS ctor default)", got)
		}
		// In-world propagation must still fire.
		if p.chatBytes == nil {
			t.Errorf("p.chatBytes: got nil, want non-nil (Chat must fire regardless of session)")
		}
	})

	t.Run("headless_session_still_emits", func(t *testing.T) {
		// 'headless' sessions emit too — no session-validity gate.
		p, cap := commonMessagePublicSetup(t)
		p.session = "headless"
		payload := packPublicChatPayload(0, 0, "hi")
		if err := handleMessagePublic(p, payload); err != nil {
			t.Fatalf("handleMessagePublic: %v", err)
		}
		if got := len(cap.publicChats()); got != 1 {
			t.Errorf("PublicChatEvent envelopes: got %d, want 1 (no session gate at 254)", got)
		}
	})
}

// TestHandleMessagePublic_AppliesWordEncFilterToChatBytes pins that
// handleMessagePublic unpacks the inbound text, filters it via s.wordenc,
// repacks the filtered text, and that the repacked bytes (not the raw input)
// end up on p.chatBytes. The emitted PublicChatEvent is asserted to carry
// the UNFILTERED text (mirrors TS player.logMessage at
// MessagePublicHandler.ts:32, set BEFORE filtering).
func TestHandleMessagePublic_AppliesWordEncFilterToChatBytes(t *testing.T) {
	p, cap := commonMessagePublicSetup(t)

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

	// The emitted event MUST carry the unfiltered text (mirrors TS
	// player.logMessage at MessagePublicHandler.ts:32, set BEFORE filtering).
	envs := cap.publicChats()
	if len(envs) != 1 {
		t.Fatalf("expected 1 PublicChatEvent, got %d", len(envs))
	}
	if got := envs[0].GetPublicChat().Text; got != "Anal" {
		t.Errorf("event text: got %q, want %q (unfiltered, sentence-cased)", got, "Anal")
	}
}
