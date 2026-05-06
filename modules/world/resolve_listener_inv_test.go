package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
)

// TestResolveListenerInvWorldSource pins Source=-1 → returns the world-
// shared inventory at s.invs[Type]. Branch existed pre-fix and stays
// behaviorally identical post-fix.
func TestResolveListenerInvWorldSource(t *testing.T) {
	s := newTestServer(t)
	s.invs = map[int]*inventory.Inventory{
		42: inventory.New(42, 28, inventory.StackNormal),
	}
	want := s.invs[42]

	got := resolveListenerInv(s, InventoryListener{Type: 42, Source: -1})

	if got != want {
		t.Errorf("resolveListenerInv(world): got %p, want %p", got, want)
	}
}

// TestResolveListenerInvPlayerSourceMatch is the regression pin for
// NAI-114 Stage 5. Pre-fix this test FAILS — the function indexes
// s.players[Source] and Source=98765 trips the >= len(s.players)
// bounds check, returning nil. Post-fix it passes via
// LookupPlayerByUID → target.invs[Type].
func TestResolveListenerInvPlayerSourceMatch(t *testing.T) {
	s := newTestServer(t)

	target := &Player{
		slot:   5,
		uid:    98765,
		active: true,
		invs: map[int]*inventory.Inventory{
			42: inventory.New(42, 28, inventory.StackNormal),
		},
	}
	s.players[target.slot] = target
	s.playerLoop = append(s.playerLoop, target)
	want := target.invs[42]

	got := resolveListenerInv(s, InventoryListener{Type: 42, Source: 98765})

	if got != want {
		t.Errorf("resolveListenerInv(player UID): got %p, want %p", got, want)
	}
}

// TestResolveListenerInvPlayerSourceOffline pins Source=<UID with no
// active player> → returns nil cleanly (no panic, no slot OOB).
func TestResolveListenerInvPlayerSourceOffline(t *testing.T) {
	s := newTestServer(t)
	// No player with uid=999999 wired into s.playerLoop.

	got := resolveListenerInv(s, InventoryListener{Type: 42, Source: 999999})

	if got != nil {
		t.Errorf("resolveListenerInv(no such uid): got %p, want nil", got)
	}
}

// TestResolveListenerInvPlayerSourceNullInv pins target online but
// target.invs[Type] is nil (or unset) → returns nil. Pre-fix this
// passes for the wrong reason (bounds-check trips before the player
// is even consulted); post-fix it exercises the actual null-inv
// branch. Documenting that here so a future reader doesn't take the
// pre-fix pass as evidence the function works.
func TestResolveListenerInvPlayerSourceNullInv(t *testing.T) {
	s := newTestServer(t)

	target := &Player{
		slot:   5,
		uid:    98765,
		active: true,
		// invs left nil — Go map reads on a nil map return the zero
		// value, so target.invs[42] is nil.
	}
	s.players[target.slot] = target
	s.playerLoop = append(s.playerLoop, target)

	got := resolveListenerInv(s, InventoryListener{Type: 42, Source: 98765})

	if got != nil {
		t.Errorf("resolveListenerInv(null inv slot): got %p, want nil", got)
	}
}
