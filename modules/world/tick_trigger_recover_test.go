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
// (recoverPlayer semantics); every other player is untouched.
func TestProcessLogins_LoginTriggerPanicIsContained(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = newPanicTriggerProvider() // see Step 1 for the stub to reuse/build
	s.runScriptFn = func(sf *script.ScriptFile, self script.ActivePlayer, target any, trigger script.ServerTriggerType, protect bool, intArgs []int, stringArgs []string) {
		if trigger == script.TriggerLogin {
			panic("boom: login trigger")
		}
	}
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	s.newPlayers = append(s.newPlayers, p)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped processLogins: %v", r)
		}
	}()
	s.processLogins()

	if !p.requestLogout {
		t.Fatal("panicking player must be flagged for logout")
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

	if s.players.get(slot) != nil {
		t.Fatal("player must be removed even when [logout] panics")
	}
}
