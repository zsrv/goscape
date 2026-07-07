package world

import (
	"testing"

	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// TestNewPlayer_PopulatesUsernameFields pins the wiring from the login flow
// to the Player struct: when a client logs in with a username, newPlayer must
// populate both Player.username (string) and Player.username37 (the base37
// long encoding consumed by the appearance buffer).
//
// Pre-fix, p.username was "" and p.username37 was 0, so the appearance buffer
// wrote 8 zero bytes for the name field. Java client decoded fromBase37(0)
// as "Invalid Name", surfacing in chat sender display. Discovered in NAI-32
// Bundle 3 Stage 5 via 2-client smoke.
// TestNewPlayer_PopulatesDisplayName pins the wiring from the login
// flow to Player.displayName: when a client logs in with a username,
// newPlayer must populate p.displayName with util.ToDisplayName
// (titlecased, underscore-replaced). Used by the DISPLAYNAME script
// opcode (TS Player.ts:417). NAI-103.
func TestNewPlayer_PopulatesDisplayName(t *testing.T) {
	c := &client{username: "alice_smith"}
	p := newPlayer(c)

	want := util.ToDisplayName("alice_smith")
	if p.displayName != want {
		t.Errorf("p.displayName: got %q, want %q", p.displayName, want)
	}
	if p.DisplayName() != want {
		t.Errorf("p.DisplayName(): got %q, want %q", p.DisplayName(), want)
	}
}

func TestNewPlayer_PopulatesUsernameFields(t *testing.T) {
	c := &client{username: "alice"}
	p := newPlayer(c)

	if p.username != "alice" {
		t.Errorf("p.username: got %q, want %q", p.username, "alice")
	}
	want37 := util.ToBase37("alice")
	if p.username37 != want37 {
		t.Errorf("p.username37: got %d, want %d (base37 of %q)", p.username37, want37, "alice")
	}
	if p.username37 == 0 {
		t.Errorf("p.username37: got 0 — pre-fix sentinel; means base37 encoding didn't run")
	}
}

// TestNewPlayer_PopulatesSession pins the slice-7 wiring: newPlayer must
// copy c.sessionUUID onto Player.session so the public-chat telemetry
// record (PublicChatEvent.session_uuid — chat is Kafka-only, spec
// docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md) can key
// on the per-login UUID. The whole-slice review for slice 7 noted that
// only the world-side e2e covered this — this direct unit test fills
// the gap.
func TestNewPlayer_PopulatesSession(t *testing.T) {
	const uuid = "11111111-2222-3333-4444-555555555555"
	c := &client{username: "alice", sessionUUID: uuid}
	p := newPlayer(c)

	if p.session != uuid {
		t.Errorf("p.session: got %q, want %q", p.session, uuid)
	}
}
