package world

import (
	"net"
	"slices"
	"testing"
	"time"

	gamemapPkg "github.com/zsrv/goscape/pkg/gamemap"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/script"
)

// captureBuildAreaWire reads everything currently in the pipe (with a
// 1-second deadline), then walks the byte stream decrypting each
// opcode via the supplied parallel ISAAC decryptor and stepping past
// each known op's PayloadSize. Returns all decoded opcodes; tests
// assert presence/absence of OpSetMultiway by scanning the slice.
//
// Must be called BEFORE the action that causes the write (net.Pipe
// is synchronous — both sides must be ready concurrently). dec must
// already be advanced past any pre-existing bytes that newZoneTestPlayer
// emitted before this goroutine launched. In practice newZoneTestPlayer
// emits no wire packets (rebuildScenery / rebuildZones at fixture-
// construction time write zone state but not raw packets), so dec
// starts at stream offset 0.
//
// updateBuildArea may emit (in order, all optional): cam packets
// (OpCamMoveTo / OpCamLookAt / OpCamShake), zone-update packets from
// rebuildZones (OpUpdateZonePartialFollows / FullFollows /
// PartialEnclosed), and OpSetMultiway. The walker handles each.
// Variable-length packets (PayloadSize == -1 / -2) read their length
// prefix, then advance.
func captureBuildAreaWire(t *testing.T, cc net.Conn, dec *io2.Isaac) <-chan []int {
	t.Helper()
	ch := make(chan []int, 1)
	go func() {
		buf := make([]byte, 16384)
		cc.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := cc.Read(buf)
		buf = buf[:n]

		var opcodes []int
		pos := 0
		for pos < n {
			encByte := int(buf[pos])
			key := int(dec.GetNext() & 0xff)
			opcode := (encByte - key) & 0xff
			pos++
			opcodes = append(opcodes, opcode)

			payloadSize, ok := buildAreaOpPayloadSize(opcode)
			if !ok {
				t.Errorf("captureBuildAreaWire: unknown opcode %d at byte pos %d (n=%d, opcodes-so-far=%v)",
					opcode, pos-1, n, opcodes)
				ch <- opcodes
				return
			}

			switch payloadSize {
			case -1:
				if pos >= n {
					t.Errorf("captureBuildAreaWire: truncated -1 length prefix at pos %d", pos)
					ch <- opcodes
					return
				}
				size := int(buf[pos])
				pos += 1 + size
			case -2:
				if pos+1 >= n {
					t.Errorf("captureBuildAreaWire: truncated -2 length prefix at pos %d", pos)
					ch <- opcodes
					return
				}
				size := (int(buf[pos]) << 8) | int(buf[pos+1])
				pos += 2 + size
			default:
				pos += payloadSize
			}
		}
		ch <- opcodes
	}()
	return ch
}

// buildAreaOpPayloadSize maps every opcode that updateBuildArea may
// emit to its payload size. Anything else returns ok=false → walker
// fails the test with a clear "unknown opcode" message.
func buildAreaOpPayloadSize(opcode int) (int, bool) {
	switch opcode {
	case int(gameserver.OpCamMoveTo.Opcode):
		return gameserver.OpCamMoveTo.PayloadSize, true
	case int(gameserver.OpCamLookAt.Opcode):
		return gameserver.OpCamLookAt.PayloadSize, true
	case int(gameserver.OpCamShake.Opcode):
		return gameserver.OpCamShake.PayloadSize, true
	case int(gameserver.OpSetMultiway.Opcode):
		return gameserver.OpSetMultiway.PayloadSize, true
	case int(gameserver.OpUpdateZonePartialFollows.Opcode):
		return gameserver.OpUpdateZonePartialFollows.PayloadSize, true
	case int(gameserver.OpUpdateZoneFullFollows.Opcode):
		return gameserver.OpUpdateZoneFullFollows.PayloadSize, true
	case int(gameserver.OpUpdateZonePartialEnclosed.Opcode):
		return gameserver.OpUpdateZonePartialEnclosed.PayloadSize, true
	}
	return 0, false
}

// containsOpcode returns true if any element in opcodes equals target.
func containsOpcode(opcodes []int, target int) bool {
	return slices.Contains(opcodes, target)
}

// TestUpdateBuildArea_FirstTickMapzoneFires_ExitDoesNot (T5) — fresh
// player at (3200, 3200, 0). lastMapZone starts -1 → the first
// updateBuildArea call must enqueue triggerMapzone (entry script)
// but NOT triggerMapzoneExit (exit gated on lastMapZone != -1).
// After the call, lastMapZone is updated to the packed coord.
func TestUpdateBuildArea_FirstTickMapzoneFires_ExitDoesNot(t *testing.T) {
	s := newZoneTestServer(t)
	s.scriptProvider = script.NewProvider()
	enterSf := &script.ScriptFile{Name: "[mapzone,0_50_50]", LookupKey: 0xa1}
	exitSf := &script.ScriptFile{Name: "[mapzoneexit,0_50_50]", LookupKey: 0xa2}
	s.scriptProvider.Register(enterSf)
	s.scriptProvider.Register(exitSf)

	p, _ := newZoneTestPlayer(t, s, 1, 3200, 3200, 0)

	if p.lastMapZone != -1 {
		t.Fatalf("setup: lastMapZone got %d, want -1", p.lastMapZone)
	}

	p.updateBuildArea()

	// Filter engineQueue to entries whose Script Name starts with [mapzone or [mapzoneexit.
	var enterCount, exitCount int
	for _, req := range p.engineQueue {
		if req.Script == enterSf {
			enterCount++
		}
		if req.Script == exitSf {
			exitCount++
		}
	}
	if enterCount != 1 {
		t.Errorf("triggerMapzone fire count: got %d, want 1", enterCount)
	}
	if exitCount != 0 {
		t.Errorf("triggerMapzoneExit fire count: got %d, want 0 (lastMapZone was -1, exit gated)", exitCount)
	}
	if p.lastMapZone == -1 {
		t.Error("lastMapZone must be updated after the transition; still -1")
	}
}

// TestUpdateBuildArea_SetMultiwayEmitOnEntry (T6) — fresh player
// (lastZone=-1) standing on a tile marked multi-combat. The first
// updateBuildArea call decodes lastWasMulti=false (UnpackCoord(-1)
// → defensive Position → IsMulti map-miss) and nowIsMulti=true →
// transition fires; OpSetMultiway with payload [0x01] hits the wire.
func TestUpdateBuildArea_SetMultiwayEmitOnEntry(t *testing.T) {
	s := newZoneTestServer(t)
	// SetMulti requires gamemap to be initialized — newZoneTestServer
	// does not init gamemap by default; do it explicitly here.
	s.gamemap = newTestGamemap(t)

	p, cc := newZoneTestPlayer(t, s, 1, 3200, 3200, 0)
	s.gamemap.SetMulti(p.x, p.z, p.level, true)

	dec := io2.New([4]uint32{1, 2, 3, 4}) // matches slot=1 encryptor seed
	received := captureBuildAreaWire(t, cc, dec)

	p.updateBuildArea()
	p.client.flushWrite()

	opcodes := <-received
	if !containsOpcode(opcodes, int(gameserver.OpSetMultiway.Opcode)) {
		t.Errorf("OpSetMultiway not emitted; opcodes seen=%v (expected opcode 254)", opcodes)
	}
}

// TestUpdateBuildArea_NoSetMultiwayWhenBothFalse (T7) — fresh player
// in a non-multi tile with no SetMulti call. lastWasMulti=false (from
// -1 sentinel) AND nowIsMulti=false → no transition; no OpSetMultiway
// on the wire. Other packets (zone updates from rebuildZones) may
// appear; pin only the absence of opcode 254.
func TestUpdateBuildArea_NoSetMultiwayWhenBothFalse(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = newTestGamemap(t)

	p, cc := newZoneTestPlayer(t, s, 1, 3200, 3200, 0)
	// Deliberately NOT calling SetMulti — lookup returns false.

	dec := io2.New([4]uint32{1, 2, 3, 4})
	received := captureBuildAreaWire(t, cc, dec)

	p.updateBuildArea()
	p.client.flushWrite()

	opcodes := <-received
	if containsOpcode(opcodes, int(gameserver.OpSetMultiway.Opcode)) {
		t.Errorf("OpSetMultiway must NOT be emitted when both sides false; opcodes seen=%v", opcodes)
	}
}

// newTestGamemap constructs a fresh GameMap suitable for SetMulti +
// IsMulti tests. Mirrors the inline init pattern used by
// world_zone_test.go (s.gamemap = gamemap.New(discardLogger())).
func newTestGamemap(t *testing.T) *gamemapPkg.GameMap {
	t.Helper()
	return gamemapPkg.New(discardLogger())
}
