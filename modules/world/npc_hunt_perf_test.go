package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/zone"
)

// PERF-2 pins: the hunt variants run once per hunting NPC per tick
// (huntPlayers via the world-level processNpcHuntPlayers pass, the other
// three via turn()'s huntAll). Pre-fix each call allocated a NearbyZones
// slice plus hunted-append growth — per NPC, per tick. Steady state must
// be allocation-free: candidates land in the Server-owned hunt scratch.

func newHuntPerfFixture(t *testing.T, players, npcs int) (*Server, *Npc) {
	t.Helper()
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	// Spread candidates over a 9x5 tile block around the NPC — all within
	// huntRange, crossing zone boundaries so several zones materialise.
	for i := range players {
		_ = addPlayerToServer(t, s, 1+i, n.x-4+i%9, n.z-2+i/9, n.level)
	}
	for i := range npcs {
		_ = addNpcToServerAt(t, s, 10+i, 1, -1, n.x-4+i%9, n.z-2+i/9, n.level)
	}
	return s, n
}

func TestHuntPlayersSteadyStateZeroAlloc(t *testing.T) {
	s, n := newHuntPerfFixture(t, 8, 0)
	hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}

	if got := len(n.huntPlayers(s, hunt)); got != 8 { // warm the scratch
		t.Fatalf("hunted: got %d, want 8", got)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if got := len(n.huntPlayers(s, hunt)); got != 8 {
			t.Fatalf("hunted: got %d, want 8", got)
		}
	})
	if allocs != 0 {
		t.Errorf("huntPlayers steady-state allocs/op = %v, want 0", allocs)
	}
}

func TestHuntNpcsSteadyStateZeroAlloc(t *testing.T) {
	s, n := newHuntPerfFixture(t, 0, 8)
	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1}

	if got := len(n.huntNpcs(s, hunt)); got != 8 { // warm the scratch
		t.Fatalf("hunted: got %d, want 8", got)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if got := len(n.huntNpcs(s, hunt)); got != 8 {
			t.Fatalf("hunted: got %d, want 8", got)
		}
	})
	if allocs != 0 {
		t.Errorf("huntNpcs steady-state allocs/op = %v, want 0", allocs)
	}
}

// BenchmarkHuntPlayers measures one hunt scan at typical (8) and crowded
// (40) candidate counts. docs/PORTING.md perf-row evidence.
func BenchmarkHuntPlayers(b *testing.B) {
	for _, count := range []int{8, 40} {
		b.Run(map[int]string{8: "players8", 40: "players40"}[count], func(b *testing.B) {
			s := &Server{log: discardLogger(), players: newPlayerList(2048)}
			typ := &objtype.NpcType{
				ID: 0, DebugName: "bench_npc",
				Size:     1,
				Stats:    []uint16{0, 0, 0, 10, 0, 0},
				Category: -1,
			}
			n := NewNpc(1, 0, 3094, 3106, 0, typ)
			n.server = s
			n.huntRange = 10
			s.zoneMap = zone.NewZoneMap()
			for i := range count {
				p := &Player{slot: 1 + i, x: n.x - 4 + i%9, z: n.z - 2 + i/9, level: n.level, active: true}
				s.players.set(p.slot, p)
				p.zoneListElement = s.zoneMap.Get(p.level, p.x, p.z).EnterPlayer(p, nil)
			}
			hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
			b.ReportAllocs()
			for b.Loop() {
				if got := len(n.huntPlayers(s, hunt)); got != count {
					b.Fatalf("hunted: got %d, want %d", got, count)
				}
			}
		})
	}
}
