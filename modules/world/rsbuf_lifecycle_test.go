package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// Smoke test for NAI-29 Bundle 4 Task 4.2 — AddPlayer / RemovePlayer
// hooks at login/logout sites. Verifies the round-trip doesn't panic
// and re-add works after remove. Implementation correctness verified
// by reading the diff at (*Server).addPlayer / removePlayer hook sites.
func TestServer_PlayerLifecycleRoundTripSmoke(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	if p.slot < 1 {
		t.Fatalf("addPlayer didn't assign slot (got %d)", p.slot)
	}
	slot := p.slot

	s.removePlayerInternal(p)

	// After removePlayer:
	//  - s.players[slot] should be nil (existing assertion)
	//  - s.rsbuf must not panic on follow-up queries (nil-slot guards in *Buf)
	if s.players.get(slot) != nil {
		t.Errorf("after removePlayer: s.players[%d] = %v, want nil", slot, s.players.get(slot))
	}
	// Smoke: query rsbuf — must not panic.
	_ = s.rsbuf.HasPlayer(int32(slot), 99)
	_ = s.rsbuf.GetNpcObservers(0)

	// Re-add at same slot must succeed.
	p2, _ := newTestPlayer(t)
	if err := s.addPlayer(p2); err != nil {
		t.Fatalf("re-add after removePlayer: %v", err)
	}
}

func TestServer_AddNpcWiresRsbufSlot(t *testing.T) {
	s := newTestServer(t)
	n := newTestNpc(50)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	// Smoke: GetNpcObservers must not panic on the new slot.
	if got := s.rsbuf.GetNpcObservers(int32(n.nid)); got != 0 {
		t.Errorf("GetNpcObservers fresh: got %d, want 0", got)
	}
}

// NAI-150 in-scope-stretch: pins that addNpc(firstSpawn=true) refreshes
// n.uid to match the freshly-allocated n.nid. Pre-fix: production
// spawn site (server.go:312) constructs NewNpc(0, ...) so the
// constructor-computed uid carried slot=0; addNpc allocated a real
// slot ≥1 but never recomputed uid, leaving every spawned NPC with a
// stale slot in its uid. Surfaced by NAI-150 smoke (PROJANIM_NPC
// errored with "invalid npc uid" because npc_uid → slot 0 → s.npcs[0]
// = nil); also silently broke NAI-120's FindNpcByUID.
func TestServer_AddNpc_UidRefreshedAfterSlotAlloc(t *testing.T) {
	s := newTestServer(t)
	// Construct with nid=0 to mirror production server.go:312 NewNpc(0, ...).
	n := newTestNpc(0)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	want := (n.typeId << 16) | n.nid
	if n.uid != want {
		t.Errorf("Npc.uid after addNpc(firstSpawn=true): got %d, want %d (typeId=%d, nid=%d)",
			n.uid, want, n.typeId, n.nid)
	}
	// Roundtrip pin: extracting slot from uid must yield n.nid (the
	// inverse of the staleness bug).
	if got := n.uid & 0xffff; got != n.nid {
		t.Errorf("uid slot extraction: got %d, want %d (n.nid)", got, n.nid)
	}
}

func TestServer_RemoveNpcCleansRsbufSlot(t *testing.T) {
	s := newTestServer(t)
	n := newTestNpc(50)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	s.removeNpc(n, -1)
	// Smoke: post-remove queries must not panic.
	if got := s.rsbuf.GetNpcObservers(int32(n.nid)); got != 0 {
		t.Errorf("GetNpcObservers post-remove: got %d, want 0", got)
	}
}

// TestDeadRespawnNpcPushesActiveFalseToRsbuf is the regression gate for the
// B6 live-smoke bug: "NPC corpse remains on ground indefinitely after death."
//
// Root cause (tick.go processInfo compute loop): the loop skipped dead NPCs
// with `if n.dead { continue }`, never calling ComputeNpc with active=false.
// The rsbuf retained the NPC's last-alive Active=true state. Clients kept
// tracking the corpse indefinitely.
//
// TS reference: World.ts:1066-1096 iterates all NPCs (including dead) and
// calls rsbuf.computeNpc(..., npc.isActive, ...) unconditionally. Dead NPCs
// (isActive=false) therefore receive Active=false each tick, causing
// writeNpcs to remove them from client tracking on the first dead tick.
//
// Fix: remove the n.dead guard from the compute loop so RESPAWN-lifecycle
// dead NPCs receive ComputeNpc(active=false) each tick until respawn.
func TestDeadRespawnNpcPushesActiveFalseToRsbuf(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	// rev-274 processInfo gate (World.ts:979-981): processInfo is skipped when
	// the world is empty. This test drives the NPC compute pass, so it needs a
	// player online to make the world non-empty.
	_ = setupInfoPlayer(t, s, 1, 3094, 3106, 0)

	// Spawn a RESPAWN-lifecycle NPC (default for NewNpc).
	n := newTestNpc(0)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	nid := int32(n.nid)

	// Simulate an "alive" tick: push Active=true into rsbuf, as processInfo
	// would do when the NPC is alive. This mirrors the state before death.
	s.rsbuf.ComputeNpc(
		nid, int32(n.typeId),
		n.x, n.level, n.z,
		n.tele,
		n.jump,
		int8(n.runDir), int8(n.walkDir),
		true, // active
		0, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1,
	)
	if entry := s.rsbuf.NpcForTest(nid); entry == nil || !entry.Active {
		t.Fatal("pre-condition: rsbuf NPC should be Active=true while alive")
	}

	// Mark NPC dead (as npc_del → removeNpc does for RESPAWN-lifecycle NPCs).
	n.dead = true

	// Drive one processInfo call. Before the fix, the dead-NPC guard skips
	// ComputeNpc, leaving Active=true. After the fix, ComputeNpc is called
	// with active=false, setting Active=false in the rsbuf — causing clients
	// to stop tracking the corpse (writeNpcs removes any tracked NPC whose
	// rsbuf entry has Active=false).
	s.processInfo()

	entry := s.rsbuf.NpcForTest(nid)
	if entry == nil {
		t.Fatal("rsbuf NPC slot should not be nil for RESPAWN-lifecycle dead NPC (only DESPAWN nils the slot)")
	}
	if entry.Active {
		t.Errorf("rsbuf NPC Active = true after NPC marked dead; want false. " +
			"Root cause: processInfo skipped ComputeNpc(active=false) for dead NPC " +
			"(tick.go dead-bool guard), leaving corpse visible to clients indefinitely. " +
			"Fix: remove n.dead guard so dead RESPAWN NPCs get ComputeNpc(active=false). " +
			"TS ref: World.ts:1066-1096 iterates ALL npcs with npc.isActive.")
	}
}
