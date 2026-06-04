package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

func setupNpcInfoPlayer(t *testing.T, s *Server, slot, x, z, level int) *Player {
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
	s.players.set(slot, p)
	p.active = true
	if s.rsbuf != nil {
		s.rsbuf.AddPlayer(int32(slot))
	}
	return p
}

func setupNpc(t *testing.T, s *Server, x, z, level int) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		WanderRange: 0, // stationary so coords don't drift
		RespawnRate: 50,
	}
	n := NewNpc(0, 0, x, z, level, typ)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPlayerSeesNearbyNpc(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	p := setupNpcInfoPlayer(t, s, 1, 3094, 3106, 0)
	npc := setupNpc(t, s, 3095, 3106, 0)

	s.processInfo()
	p.updateNpcs()

	if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
		t.Errorf("player should track npc after updateNpcs; HasNpc(%d, %d) returned false", p.slot, npc.nid)
	}
}

func TestNpcSayProducesSayMaskInRenderer(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	npc := setupNpc(t, s, 3095, 3106, 0)
	npc.Say([]byte("hello"))
	s.processInfo()

	highDef := s.renderer.NpcHighDefOf(npc.nid)
	if len(highDef) == 0 {
		t.Fatal("NpcHighDefOf should be non-empty after Say()")
	}
	if highDef[0]&rsbuf.NpcMaskSay == 0 {
		t.Errorf("header byte: NpcMaskSay (0x8) should be set; got %d", highDef[0])
	}
	if !bytes.Contains(highDef, []byte("hello")) {
		t.Errorf("expected 'hello' bytes in payload; got %v", highDef)
	}
}
