package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// buildSeqTypes returns a SeqTypeConfigs with count entries.
// Each entry has Priority=5 (TS default) and DebugName empty. Tests that
// exercise the priority-comparison arm override per-entry Priority before
// invoking PlayAnim/Animate.
func buildSeqTypes(count int) *objtype.SeqTypeConfigs {
	configs := make([]*objtype.SeqType, count)
	for i := range count {
		configs[i] = objtype.NewSeqType(i)
	}
	return &objtype.SeqTypeConfigs{
		ConfigNames: map[string]int{},
		Configs:     configs,
	}
}

// TestPlayAnim_AnimProtectBlocksWrite — NAI-56. With animProtect set,
// PlayAnim must early-return and leave animID, animDelay, and the
// MaskAnim bit untouched. Mirrors TS Player.ts:1842 where the animProtect
// truthy check short-circuits playAnimation before any write.
func TestPlayAnim_AnimProtectBlocksWrite(t *testing.T) {
	p, _ := newTestPlayer(t)
	initialAnimID := p.animID
	initialAnimDelay := p.animDelay
	initialMasks := p.masks

	p.seqTypes = buildSeqTypes(124)
	p.animProtect = 1
	p.PlayAnim(123, 5)

	if p.animID != initialAnimID {
		t.Errorf("animID: got %d, want %d (unchanged)", p.animID, initialAnimID)
	}
	if p.animDelay != initialAnimDelay {
		t.Errorf("animDelay: got %d, want %d (unchanged)", p.animDelay, initialAnimDelay)
	}
	if p.masks != initialMasks {
		t.Errorf("masks: got %d, want %d (unchanged)", p.masks, initialMasks)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim bit should not be set when animProtect blocks the write")
	}
}

// TestPlayAnim_AnimProtectZeroAllowsWrite — NAI-56. Baseline regression
// guard: with animProtect=0 (default), PlayAnim writes through and sets
// MaskAnim. Pins that the new gate has no effect on the unprotected path.
func TestPlayAnim_AnimProtectZeroAllowsWrite(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.animProtect != 0 {
		t.Fatalf("animProtect: precondition got %d, want 0", p.animProtect)
	}

	p.seqTypes = buildSeqTypes(124)
	p.PlayAnim(123, 5)

	if p.animID != 123 {
		t.Errorf("animID: got %d, want 123", p.animID)
	}
	if p.animDelay != 5 {
		t.Errorf("animDelay: got %d, want 5", p.animDelay)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim bit should be set when animProtect=0")
	}
}

func TestPlayAnim_BoundsRejectAtCount(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.seqTypes = buildSeqTypes(50)
	p.masks = 0
	p.PlayAnim(50, 5)
	if p.animID != -1 {
		t.Errorf("animID: got %d, want -1 (bounds-reject)", p.animID)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim should not be set on bounds-reject")
	}
}

func TestPlayAnim_BoundsRejectAboveCount(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.seqTypes = buildSeqTypes(50)
	p.masks = 0
	p.PlayAnim(99, 5)
	if p.animID != -1 {
		t.Errorf("animID: got %d, want -1", p.animID)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim should not be set")
	}
}

func TestPlayAnim_NilRegistryRejectsAllNonClear(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.seqTypes = nil
	p.masks = 0
	p.PlayAnim(0, 5) // count==0; bounds 0>=0 → reject
	if p.animID != -1 {
		t.Errorf("animID: got %d, want -1 (nil registry → count=0 → bounds-reject)", p.animID)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim should not be set")
	}
}

func TestPlayAnim_NilRegistryAllowsClear(t *testing.T) {
	// Registry never loaded; animID is fresh (-1). Clear with -1 must succeed
	// because the priority arm short-circuits via seqID==-1 before any slice deref.
	p, _ := newTestPlayer(t)
	p.seqTypes = nil
	p.animID = -1 // fresh state
	p.masks = 0
	p.PlayAnim(-1, 0)
	if p.animID != -1 {
		t.Errorf("animID: got %d, want -1", p.animID)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim should be set on clear")
	}
}

func TestPlayAnim_PriorityHigherOverwrites(t *testing.T) {
	p, _ := newTestPlayer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 3
	cfg.Configs[10].Priority = 7
	p.seqTypes = cfg
	p.animID = 5
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 10 {
		t.Errorf("animID: got %d, want 10 (higher priority overwrites)", p.animID)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim should be set on overwrite")
	}
}

func TestPlayAnim_PriorityLowerRejected(t *testing.T) {
	p, _ := newTestPlayer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 7
	cfg.Configs[10].Priority = 3
	p.seqTypes = cfg
	p.animID = 5
	p.animDelay = 99
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 5 {
		t.Errorf("animID: got %d, want 5 (lower priority rejected)", p.animID)
	}
	if p.animDelay != 99 {
		t.Errorf("animDelay: got %d, want 99 (preserved)", p.animDelay)
	}
	if p.masks&rsbuf.MaskAnim != 0 {
		t.Error("MaskAnim should not be set on rejection")
	}
}

func TestPlayAnim_PriorityEqualOverwrites(t *testing.T) {
	// 244 changed strict `>` to `>=` (Player.ts:1857 at 9aadcec4). Equal
	// nonzero priority now overwrites. Both Priority=5 (default).
	p, _ := newTestPlayer(t)
	p.seqTypes = buildSeqTypes(20)
	p.animID = 5
	p.animDelay = 99
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 10 {
		t.Errorf("animID: got %d, want 10 (equal priority overwrites under 244 >=)", p.animID)
	}
	if p.animDelay != 3 {
		t.Errorf("animDelay: got %d, want 3", p.animDelay)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim must be set on equal-priority overwrite")
	}
}

func TestPlayAnim_CurrentAnimZeroPriorityOverwrites(t *testing.T) {
	// TS L1846 third disjunct: when current anim's priority is 0, any new
	// anim overwrites regardless of its own priority.
	p, _ := newTestPlayer(t)
	cfg := buildSeqTypes(20)
	cfg.Configs[5].Priority = 0
	cfg.Configs[10].Priority = 5
	p.seqTypes = cfg
	p.animID = 5
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 10 {
		t.Errorf("animID: got %d, want 10 (current zero-priority overwrite)", p.animID)
	}
}

func TestPlayAnim_FreshAnimIDMinusOneAlwaysOverwrites(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.seqTypes = buildSeqTypes(20)
	p.animID = -1 // fresh
	p.masks = 0
	p.PlayAnim(10, 3)
	if p.animID != 10 {
		t.Errorf("animID: got %d, want 10 (fresh animID=-1 short-circuit)", p.animID)
	}
}

// TestSetAnimEqualPriorityOverwrites_Player pins the 244 gate change: when a
// new anim has the SAME nonzero priority as the current anim, the 244 rule
// (>=) overwrites, whereas the 225 rule (>) rejected. TS ref:
// Player.ts:1857 — `SeqType.get(anim).priority >= SeqType.get(this.animId).priority`
// at Engine-TS pin 9aadcec4. The discriminating case is equal nonzero priority.
func TestSetAnimEqualPriorityOverwrites_Player(t *testing.T) {
	p, _ := newTestPlayer(t)
	cfg := buildSeqTypes(20)
	// Both seq 5 and seq 10 have the default Priority=5. Seed with seq 5 active.
	p.seqTypes = cfg
	p.animID = 5
	p.animDelay = 99
	p.masks = 0

	// Play seq 10 at equal priority (5 == 5). 244 gate must overwrite.
	p.PlayAnim(10, 7)

	if p.animID != 10 {
		t.Errorf("animID: got %d, want 10 (equal priority must overwrite under 244 >=)", p.animID)
	}
	if p.animDelay != 7 {
		t.Errorf("animDelay: got %d, want 7", p.animDelay)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("MaskAnim must be set on equal-priority overwrite")
	}
}

// TestResetMasks_ResetsAnimForCrossTickReplay pins that ResetMasks clears
// animID/animDelay back to -1 at tick end, so the same emote/animation can
// play again on a later tick. Without the reset, PlayAnim's priority guard
// compares the re-play against the stale prior animID and rejects an
// equal-priority repeat — so an emote (or bow/bury anim) only fires once per
// session. Mirrors TS PathingEntity.resetPathingEntity (PathingEntity.ts:598-601).
func TestResetMasks_ResetsAnimForCrossTickReplay(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.seqTypes = buildSeqTypes(20)

	// Tick N: play emote 10 (default priority 5). processInfo sends it.
	p.PlayAnim(10, 3)
	if p.animID != 10 || p.masks&rsbuf.MaskAnim == 0 {
		t.Fatalf("first PlayAnim: animID=%d mask-set=%v, want 10/true", p.animID, p.masks&rsbuf.MaskAnim != 0)
	}

	// Tick N end (processCleanup → ResetMasks).
	p.ResetMasks()
	if p.animID != -1 {
		t.Errorf("animID after ResetMasks: got %d, want -1", p.animID)
	}
	if p.animDelay != -1 {
		t.Errorf("animDelay after ResetMasks: got %d, want -1", p.animDelay)
	}

	// Tick N+1: the same emote must replay (re-flag MaskAnim).
	p.PlayAnim(10, 3)
	if p.animID != 10 {
		t.Errorf("replay animID: got %d, want 10", p.animID)
	}
	if p.masks&rsbuf.MaskAnim == 0 {
		t.Error("replay must re-flag MaskAnim (otherwise the emote only plays once per session)")
	}
}
