package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// setupInfoPlayer constructs a Player with the full 3a/3b scaffolding bypassing
// the normal login/processLogins pipeline.
func setupInfoPlayer(t *testing.T, s *Server, slot, x, z, level int) *Player {
	t.Helper()
	p, _ := newTestPlayer(t)
	p.client.server = s
	enc, dec := isaacPair([4]uint32{uint32(slot), 2, 3, 4})
	p.client.encryptor = enc
	p.client.decryptor = dec
	p.x, p.z, p.level = x, z, level
	p.originX, p.originZ = x, z
	p.lastTickX, p.lastTickZ, p.lastLevel = x, z, level
	p.slot = slot
	s.players[slot] = p
	s.playerLoop = append(s.playerLoop, p)
	p.active = true
	if s.rsbuf != nil {
		s.rsbuf.AddPlayer(int32(slot))
	}
	return p
}

func TestTwoPlayersSeeEachOther(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	a := setupInfoPlayer(t, s, 1, 3094, 3106, 0)
	_ = setupInfoPlayer(t, s, 2, 3095, 3106, 0)

	s.processInfo()
	a.updatePlayers()

	if !s.rsbuf.HasPlayer(int32(a.slot), 2) {
		t.Errorf("a should track b after updatePlayers; HasPlayer(%d, 2) returned false", a.slot)
	}
}

func TestSayProducesChatMaskInHighDef(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	a := setupInfoPlayer(t, s, 1, 3094, 3106, 0)

	a.Say([]byte("hello"))
	s.processInfo()

	highDef := s.renderer.HighDefOf(1)
	if len(highDef) == 0 {
		t.Fatal("high-def should be non-empty after Say()")
	}
	// Header byte should include MaskSay (0x8). Note: rendering suppresses CHAT
	// (rsbuf.MaskChat=0x40) but MaskSay is the "speech bubble" and IS included.
	if highDef[0]&rsbuf.MaskSay == 0 {
		t.Errorf("high-def header should have MaskSay (0x8): got %d (%#x)", highDef[0], highDef[0])
	}
}
