package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// newPanicTriggerProvider builds a *script.Provider with a registered
// [login] and [logout] global trigger script so GetByTrigger /
// GetByTriggerSpecific return non-nil and the trigger fires. The real
// script body never runs in these tests: s.runScriptFn is replaced with a
// panicking stub, so only the LookupKey wiring matters here.
func newPanicTriggerProvider() *script.Provider {
	p := script.NewProvider()
	p.Register(&script.ScriptFile{
		Name:      "login",
		LookupKey: script.LookupKeyForGlobal(script.TriggerLogin),
	})
	p.Register(&script.ScriptFile{
		Name:      "logout",
		LookupKey: script.LookupKeyForGlobal(script.TriggerLogout),
	})
	return p
}

// SEC1 M-1 / DEVIATION SEC1-D1: a panicking [login] trigger must not
// escape processLogins. The offending player is force-disconnected
// (recoverPlayer semantics) and the next player in the same batch still
// logs in normally — containment is per player, not per batch.
func TestProcessLogins_LoginTriggerPanicIsContained(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = newPanicTriggerProvider()
	// Panic for the first [login] only: the second player exercises the
	// "everyone else is untouched" half of the claim.
	logins := 0
	s.runScriptFn = func(sf *script.ScriptFile, self script.ActivePlayer, target any, trigger script.ServerTriggerType, protect bool, intArgs []int, stringArgs []string) {
		if trigger == script.TriggerLogin {
			logins++
			if logins == 1 {
				panic("boom: login trigger")
			}
		}
	}
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	c2, _ := newTestClient(t)
	p2 := newPlayer(c2)
	p2.username = "carol"
	s.newPlayers = append(s.newPlayers, p, p2)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped processLogins: %v", r)
		}
	}()
	s.processLogins()

	if !p.requestLogout {
		t.Fatal("panicking player must be flagged for logout")
	}
	if logins != 2 {
		t.Fatalf("[login] fired %d times, want 2 (the second player must still be processed)", logins)
	}
	if p2.requestLogout {
		t.Fatal("bystander player must not be flagged for logout")
	}
	if s.players[p2.slot] != p2 {
		t.Fatal("bystander player must be in the world after the batch")
	}
}

// SEC1 M-1 / DEVIATION SEC1-D1: same for [logout]; the player is still
// removed from the world after the trigger panics.
func TestProcessLogouts_LogoutTriggerPanicIsContained(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = newPanicTriggerProvider()
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "bob"
	if err := s.addPlayer(p); err != nil {
		t.Fatal(err)
	}
	slot := p.slot
	s.runScriptFn = func(sf *script.ScriptFile, self script.ActivePlayer, target any, trigger script.ServerTriggerType, protect bool, intArgs []int, stringArgs []string) {
		if trigger == script.TriggerLogout {
			panic("boom: logout trigger")
		}
	}
	p.requestLogout = true

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped processLogouts: %v", r)
		}
	}()
	s.processLogouts()

	if s.players[slot] != nil {
		t.Fatal("player must be removed even when [logout] panics")
	}
}
