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
