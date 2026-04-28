# Compressed Spec+Plan: processIn lastConnected/lastResponse refresh

**Date:** 2026-04-28  
**Scope:** ~6 LOC production + 2 tests  
**Tech Stack:** Go 1.26, modules/world

---

## Problem

`processIn` (player.go:645) never refreshes `p.lastConnected` or `p.lastResponse` after login.
Both fields are set exactly once in `processLogins` (tick.go:92-93).
After 50 ticks (`timeoutNoConnection` = 30 s) every connected player gets
`requestIdleLogout = true` regardless of activity, then logs out.

**TS reference:**  
- `NetworkPlayer.decodeIn()` (NetworkPlayer.ts:63): `lastConnected = World.currentTick`
  after the `isClientConnected()` guard — every tick the socket is alive.  
- `NetworkPlayer.decodeIn()` (NetworkPlayer.ts:80): `lastResponse = World.currentTick`
  when `bytesRead > 0` — only when the client actually sent bytes.

---

## Fix

### T1 — player.go `processIn` (player.go:645)

After the `c.state != ClientStateGame` early-return guard add the `lastConnected`
assignment. Add `readAny` tracking inside the loop and the `lastResponse` update
after the loop.

**Exact edit — replace the section from `p.userLimit = 0` through end of func:**

```go
	p.lastConnected = currentTick // mirrors TS decodeIn() line 63

	p.userLimit = 0
	p.clientLimit = 0
	p.restrictedLimit = 0
	p.opcalled = false

	c.inMu.Lock()
	defer c.inMu.Unlock()

	readAny := false
	for p.userLimit < userEventLimit &&
		p.clientLimit < clientEventLimit &&
		p.restrictedLimit < restrictedEventLimit {

		opcode, ok, err := p.readPacket()
		if err != nil {
			return
		}
		if !ok {
			break
		}
		readAny = true
		switch gameclient.Ops[opcode].Category {
		case gameclient.CategoryUserEvent:
			p.userLimit++
		case gameclient.CategoryRestrictedEvent:
			p.restrictedLimit++
		default:
			p.clientLimit++
		}
	}
	if readAny {
		p.lastResponse = currentTick // mirrors TS decodeIn() line 80
	}
```

### T2 — server_test.go (two new tests, append near TestProcessLogoutsTimeoutMarksLoggingOut)

```go
// TestProcessInUpdatesLastConnectedWhenGameState: processIn refreshes
// lastConnected every tick when the client is in game state, preventing the
// 30-second idle-logout from firing on active players.
func TestProcessInUpdatesLastConnectedWhenGameState(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	c.server = s
	c.state = ClientStateGame
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	c.encryptor = enc
	p := newPlayer(c)
	c.player = p
	p.lastConnected = 0

	p.processIn(42)

	if p.lastConnected != 42 {
		t.Errorf("lastConnected: got %d, want 42", p.lastConnected)
	}
}

// TestProcessInDoesNotUpdateLastConnectedWhenNotGameState verifies that
// lastConnected is only refreshed once the login handshake is complete.
func TestProcessInDoesNotUpdateLastConnectedWhenNotGameState(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	c.server = s
	// c.state defaults to ClientStateLogin
	p := newPlayer(c)
	c.player = p
	p.lastConnected = 5

	p.processIn(42)

	if p.lastConnected != 5 {
		t.Errorf("lastConnected: got %d, want 5 (unchanged)", p.lastConnected)
	}
}
```

---

## Verification

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestProcessIn|TestProcessLogouts" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

## Deviations

None — direct port of TS behaviour.
