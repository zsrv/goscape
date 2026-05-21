package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// fakeLineValidator is a script.LineValidator test double for
// FindClosestNpc* huntvis tests. Mirrors pkg/script's stubLineValidator
// in shape; defined locally because the script package's stub isn't
// exported.
type fakeLineValidator struct {
	losReturn bool
	lowReturn bool
}

func (f *fakeLineValidator) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return f.losReturn
}

func (f *fakeLineValidator) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return f.lowReturn
}

// recordingFakeLineValidator captures args for arg-shape pin tests.
type recordingFakeLineValidator struct {
	losLevel, losSrcX, losSrcZ, losDestX, losDestZ int
	losReturn                                      bool
	lowLevel, lowSrcX, lowSrcZ, lowDestX, lowDestZ int
	lowReturn                                      bool
}

func (r *recordingFakeLineValidator) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	r.losLevel, r.losSrcX, r.losSrcZ, r.losDestX, r.losDestZ = level, srcX, srcZ, destX, destZ
	return r.losReturn
}

func (r *recordingFakeLineValidator) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	r.lowLevel, r.lowSrcX, r.lowSrcZ, r.lowDestX, r.lowDestZ = level, srcX, srcZ, destX, destZ
	return r.lowReturn
}

// setupLookupServer returns a Server with npcLookup bound and NpcTypes 7
// ("Hans", category 5) and 8 ("Other", category 9) registered. Mirrors
// the fixture patterns at player_npc_test.go:33 and script_test.go:939+.
func setupLookupServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	s.npcLookup = serverNpcLookup{s: s}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: make([]*objtype.NpcType, 100),
	}
	s.npcTypes.Configs[7] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "hans"},
		Name:       "Hans",
		Category:   5,
	}
	s.npcTypes.Configs[8] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 8, DebugName: "other"},
		Name:       "Other",
		Category:   9,
	}
	return s
}

// TestServerNpcLookup_FindClosestByType places 3 NPCs (2 of target type,
// 1 other) and asserts that the closer of the two target-type NPCs is
// returned and the wrong-type NPC is never returned regardless of distance.
func TestServerNpcLookup_FindClosestByType(t *testing.T) {
	s := setupLookupServer(t)
	// near: target type at (50,50,0), far: target type at (60,50,0),
	// wrong: different type at (51,50,0) — closer than far but wrong type.
	near := setupNpc(t, s, 50, 50, 0)
	near.typeId = 7
	far := setupNpc(t, s, 60, 50, 0)
	far.typeId = 7
	wrong := setupNpc(t, s, 51, 50, 0)
	wrong.typeId = 8

	lookup := s.npcLookup
	got := lookup.FindClosestNpcByType(0, 50, 50, 30, 7, 0)
	if got == nil {
		t.Fatal("expected to find an NPC, got nil")
	}
	gotNpc, ok := got.(*Npc)
	if !ok {
		t.Fatalf("got type %T, want *Npc", got)
	}
	if gotNpc != near {
		t.Errorf("expected closest NPC (near), got %v", gotNpc)
	}
}

// TestServerNpcLookup_FindClosestByCategory places 2 NPCs with matching
// category (5) and 1 non-matching (category 9); asserts the closer
// category-match is returned and the non-matching NPC is not.
func TestServerNpcLookup_FindClosestByCategory(t *testing.T) {
	s := setupLookupServer(t)
	// catMatch: type 7 (category 5) at (50,50,0)
	// catFar: type 7 (category 5) at (60,50,0)
	// catMiss: type 8 (category 9) at (51,50,0) — closer than catFar but wrong category.
	catMatch := setupNpc(t, s, 50, 50, 0)
	catMatch.typeId = 7
	catFar := setupNpc(t, s, 60, 50, 0)
	catFar.typeId = 7
	catMiss := setupNpc(t, s, 51, 50, 0)
	catMiss.typeId = 8

	lookup := s.npcLookup
	got := lookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, 0)
	if got == nil {
		t.Fatal("expected to find an NPC with category 5, got nil")
	}
	gotNpc, ok := got.(*Npc)
	if !ok {
		t.Fatalf("got type %T, want *Npc", got)
	}
	if gotNpc != catMatch {
		t.Errorf("expected closer category-match NPC (catMatch), got %v", gotNpc)
	}
}

// TestServerNpcLookup_FindAtExactCoord places an NPC at (50,50,0) of type 7
// and verifies hit + four miss sub-cases (off-by-one x, off-by-one z, wrong
// level, wrong type). The "wrong type" sub-case closes spec §5.4 #3 coverage
// gap: a mock in handler tests cannot simulate the type filter world-side.
func TestServerNpcLookup_FindAtExactCoord(t *testing.T) {
	s := setupLookupServer(t)
	exact := setupNpc(t, s, 50, 50, 0)
	exact.typeId = 7

	lookup := s.npcLookup

	// Hit.
	got := lookup.FindNpcAtExactCoord(0, 50, 50, 7)
	if got == nil {
		t.Fatal("exact coord lookup should find the NPC")
	}

	// Compile-time assertion: serverNpcLookup satisfies script.NpcLookup.
	var _ script.NpcLookup = s.npcLookup

	// Miss sub-cases.
	for _, tc := range []struct {
		name    string
		l, x, z int
		typeID  int
	}{
		{"off by one x", 0, 51, 50, 7},
		{"off by one z", 0, 50, 51, 7},
		{"wrong level", 1, 50, 50, 7},
		{"wrong type", 0, 50, 50, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := lookup.FindNpcAtExactCoord(tc.l, tc.x, tc.z, tc.typeID); got != nil {
				t.Errorf("%s: expected nil, got %v", tc.name, got)
			}
		})
	}
}

// --- NAI-33 Task 2: serverNpcLookup.ZoneNpcs tests ----------------------

func TestServerNpcLookup_ZoneNpcs_EmptyZone(t *testing.T) {
	s := setupLookupServer(t)
	got := s.npcLookup.ZoneNpcs(0, 3200, 3300)
	if len(got) != 0 {
		t.Errorf("empty zone: got len=%d, want 0", len(got))
	}
}

func TestServerNpcLookup_ZoneNpcs_SingleNpc(t *testing.T) {
	s := setupLookupServer(t)
	n := setupNpc(t, s, 3200, 3300, 0)
	got := s.npcLookup.ZoneNpcs(0, 3200, 3300)
	if len(got) != 1 {
		t.Fatalf("got len=%d, want 1", len(got))
	}
	if got[0] != script.ActiveNpc(n) {
		t.Errorf("got %v, want %v", got[0], n)
	}
}

func TestServerNpcLookup_ZoneNpcs_OnlyRequestedZone(t *testing.T) {
	s := setupLookupServer(t)
	nIn := setupNpc(t, s, 3200, 3300, 0)
	_ = setupNpc(t, s, 3300, 3400, 0) // different zone (zone-aligned 3296 ≠ 3200)
	got := s.npcLookup.ZoneNpcs(0, 3200, 3300)
	if len(got) != 1 {
		t.Fatalf("got len=%d, want 1 (only the requested zone's NPC)", len(got))
	}
	if got[0] != script.ActiveNpc(nIn) {
		t.Errorf("got %v, want %v", got[0], nIn)
	}
}

func TestServerNpcLookup_ZoneNpcs_OffGridReturnsEmpty(t *testing.T) {
	s := setupLookupServer(t)
	got := s.npcLookup.ZoneNpcs(0, -1000, -1000) // outside any allocated zone
	if len(got) != 0 {
		t.Errorf("off-grid: got len=%d, want 0", len(got))
	}
}

// --- NAI-33 Task 3: huntvisGate + FindClosestNpc* huntvis filtering tests ---

// TestFindClosestNpcByType_HuntVisOff_Baseline — regression guard:
// HuntVisOff continues to return the closest type-matched NPC even
// when an always-block validator is wired. Pre-slice behavior preserved.
func TestFindClosestNpcByType_HuntVisOff_Baseline(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{losReturn: false, lowReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisOff))
	if got == nil {
		t.Fatal("HuntVisOff with blocking validator should still emit")
	}
}

// TestFindClosestNpcByType_HuntVisLineOfSight_FiltersBlocked — proves
// LoS-blocked NPCs are skipped; only the LoS-passing NPC at same dist
// is returned. Closes NAI-33-D1 for NPC_FIND via huntvisGate wiring.
func TestFindClosestNpcByType_HuntVisLineOfSight_FiltersBlocked(t *testing.T) {
	s := setupLookupServer(t)
	// Always-block: nothing should pass except via pessimistic-allow,
	// which only triggers on nil validator. With validator present,
	// always-false → all candidates filtered → nil result.
	s.lineValidatorOverride = &fakeLineValidator{losReturn: false}
	npc1 := setupNpc(t, s, 50, 50, 0)
	npc1.typeId = 7
	npc2 := setupNpc(t, s, 51, 50, 0)
	npc2.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 49, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if got != nil {
		t.Errorf("all candidates LoS-blocked: expected nil, got %v", got)
	}

	// Now flip to always-pass and verify the closer one wins.
	s.lineValidatorOverride = &fakeLineValidator{losReturn: true}
	got = s.npcLookup.FindClosestNpcByType(0, 49, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Fatal("LoS-passing: expected emit, got nil")
	}
	if got.(*Npc) != npc1 {
		t.Errorf("LoS-passing: expected closer npc1 (at 50,50 vs lookup at 49,50), got npc2 (at 51,50)")
	}
}

// TestFindClosestNpcByType_HuntVisLineOfWalk_FiltersBlocked — LoW variant.
func TestFindClosestNpcByType_HuntVisLineOfWalk_FiltersBlocked(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{lowReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisLineOfWalk))
	if got != nil {
		t.Errorf("LoW-blocked: expected nil, got %v", got)
	}

	s.lineValidatorOverride = &fakeLineValidator{lowReturn: true}
	got = s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisLineOfWalk))
	if got == nil {
		t.Fatal("LoW-passing: expected emit, got nil")
	}
}

// TestFindClosestNpcByType_NilLineValidator_PessimisticAllow — when
// scriptLineValidator returns nil (no gamemap + no override), huntvis
// filter pessimistically allows. Matches HuntAll-mode iterator
// convention at pkg/script/npc_iterator.go:138-141.
func TestFindClosestNpcByType_NilLineValidator_PessimisticAllow(t *testing.T) {
	s := setupLookupServer(t)
	// s.gamemap == nil (newTestServer doesn't wire one), s.lineValidatorOverride == nil
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Error("nil-validator + LoS huntvis should pessimistically allow")
	}
}

// TestFindClosestNpcByType_HuntVisAfterDistance_ClosestStillWins — 2
// LoS-passing NPCs at different distances; closer wins. Validates
// huntvis filter doesn't disturb the closest-by-euclidean-squared
// selection or the later-match-wins (<=) tie-break.
func TestFindClosestNpcByType_HuntVisAfterDistance_ClosestStillWins(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{losReturn: true}
	near := setupNpc(t, s, 50, 50, 0)
	near.typeId = 7
	far := setupNpc(t, s, 60, 50, 0)
	far.typeId = 7

	got := s.npcLookup.FindClosestNpcByType(0, 50, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Fatal("expected emit")
	}
	if got.(*Npc) != near {
		t.Errorf("expected closer NPC; got far one")
	}
}

// TestFindClosestNpcByType_LineOfSightArgShape pins the LoS arg tuple
// per TS NpcIterator DISTANCE-mode at ScriptIterators.ts:348:
// isLineOfSight(level, lookupX, lookupZ, npc.x, npc.z) — iterator-as-src
// ordering. Guards NAI-166-D-LOW-ARG-SHAPE-SWEEP precedent at the
// FindClosest call site.
func TestFindClosestNpcByType_LineOfSightArgShape(t *testing.T) {
	s := setupLookupServer(t)
	rec := &recordingFakeLineValidator{losReturn: true}
	s.lineValidatorOverride = rec
	npc := setupNpc(t, s, 51, 52, 3)
	npc.typeId = 7

	_ = s.npcLookup.FindClosestNpcByType(3, 50, 50, 30, 7, int(objtype.HuntVisLineOfSight))
	if rec.losLevel != 3 || rec.losSrcX != 50 || rec.losSrcZ != 50 || rec.losDestX != 51 || rec.losDestZ != 52 {
		t.Errorf("LoS arg shape: got (level=%d, src=%d,%d, dst=%d,%d), want (3, 50, 50, 51, 52) — lookup-as-src",
			rec.losLevel, rec.losSrcX, rec.losSrcZ, rec.losDestX, rec.losDestZ)
	}
}

// TestFindClosestNpcByType_LineOfWalkArgShape pins the LoW arg tuple
// per TS NpcIterator DISTANCE-mode at ScriptIterators.ts:351:
// isLineOfWalk(level, lookupX, lookupZ, npc.x, npc.z) — iterator-as-src
// ordering. Guards NAI-166-D-LOW-ARG-SHAPE-SWEEP precedent at the
// FindClosest call site. Mirror of TestFindClosestNpcByType_LineOfSightArgShape.
func TestFindClosestNpcByType_LineOfWalkArgShape(t *testing.T) {
	s := setupLookupServer(t)
	rec := &recordingFakeLineValidator{lowReturn: true}
	s.lineValidatorOverride = rec
	npc := setupNpc(t, s, 51, 52, 3)
	npc.typeId = 7

	_ = s.npcLookup.FindClosestNpcByType(3, 50, 50, 30, 7, int(objtype.HuntVisLineOfWalk))
	if rec.lowLevel != 3 || rec.lowSrcX != 50 || rec.lowSrcZ != 50 || rec.lowDestX != 51 || rec.lowDestZ != 52 {
		t.Errorf("LoW arg shape: got (level=%d, src=%d,%d, dst=%d,%d), want (3, 50, 50, 51, 52) — lookup-as-src",
			rec.lowLevel, rec.lowSrcX, rec.lowSrcZ, rec.lowDestX, rec.lowDestZ)
	}
}

// TestFindClosestNpcByCategory_HuntVisOff_Baseline — regression guard
// for NPC_FINDCAT, mirror of TestFindClosestNpcByType_HuntVisOff_Baseline.
func TestFindClosestNpcByCategory_HuntVisOff_Baseline(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{losReturn: false, lowReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7 // category 5 per setupLookupServer

	got := s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisOff))
	if got == nil {
		t.Fatal("HuntVisOff with blocking validator should still emit")
	}
}

// TestFindClosestNpcByCategory_HuntVisLineOfSight_FiltersBlocked — LoS
// filter wiring on NPC_FINDCAT closes NAI-33-D1 for the category variant.
func TestFindClosestNpcByCategory_HuntVisLineOfSight_FiltersBlocked(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{losReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7 // category 5

	got := s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisLineOfSight))
	if got != nil {
		t.Errorf("LoS-blocked: expected nil, got %v", got)
	}

	s.lineValidatorOverride = &fakeLineValidator{losReturn: true}
	got = s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Error("LoS-passing: expected emit, got nil")
	}
}

// TestFindClosestNpcByCategory_HuntVisLineOfWalk_FiltersBlocked — LoW
// variant for NPC_FINDCAT.
func TestFindClosestNpcByCategory_HuntVisLineOfWalk_FiltersBlocked(t *testing.T) {
	s := setupLookupServer(t)
	s.lineValidatorOverride = &fakeLineValidator{lowReturn: false}
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisLineOfWalk))
	if got != nil {
		t.Errorf("LoW-blocked: expected nil, got %v", got)
	}
}

// TestFindClosestNpcByCategory_NilLineValidator_PessimisticAllow — nil
// validator + LoS huntvis → emit (pessimistic-allow convention).
func TestFindClosestNpcByCategory_NilLineValidator_PessimisticAllow(t *testing.T) {
	s := setupLookupServer(t)
	npc := setupNpc(t, s, 50, 50, 0)
	npc.typeId = 7

	got := s.npcLookup.FindClosestNpcByCategory(0, 50, 50, 30, 5, int(objtype.HuntVisLineOfSight))
	if got == nil {
		t.Error("nil-validator + LoS huntvis should pessimistically allow")
	}
}

// TestFindClosestNpcByCategory_LineOfSightArgShape pins the LoS arg
// tuple per TS NpcIterator DISTANCE-mode at ScriptIterators.ts:348:
// isLineOfSight(level, lookupX, lookupZ, npc.x, npc.z) — iterator-as-src
// ordering. Guards NAI-166-D-LOW-ARG-SHAPE-SWEEP precedent at the
// FindClosest call site. Mirror of TestFindClosestNpcByType_LineOfSightArgShape
// for the Category variant.
func TestFindClosestNpcByCategory_LineOfSightArgShape(t *testing.T) {
	s := setupLookupServer(t)
	rec := &recordingFakeLineValidator{losReturn: true}
	s.lineValidatorOverride = rec
	npc := setupNpc(t, s, 51, 52, 3)
	npc.typeId = 7 // category 5 per setupLookupServer

	_ = s.npcLookup.FindClosestNpcByCategory(3, 50, 50, 30, 5, int(objtype.HuntVisLineOfSight))
	if rec.losLevel != 3 || rec.losSrcX != 50 || rec.losSrcZ != 50 || rec.losDestX != 51 || rec.losDestZ != 52 {
		t.Errorf("LoS arg shape: got (level=%d, src=%d,%d, dst=%d,%d), want (3, 50, 50, 51, 52) — lookup-as-src",
			rec.losLevel, rec.losSrcX, rec.losSrcZ, rec.losDestX, rec.losDestZ)
	}
}

// TestFindClosestNpcByCategory_LineOfWalkArgShape pins the LoW arg
// tuple per TS NpcIterator DISTANCE-mode at ScriptIterators.ts:351:
// isLineOfWalk(level, lookupX, lookupZ, npc.x, npc.z) — iterator-as-src
// ordering. Guards NAI-166-D-LOW-ARG-SHAPE-SWEEP precedent at the
// FindClosest call site. Mirror of TestFindClosestNpcByType_LineOfWalkArgShape
// for the Category variant.
func TestFindClosestNpcByCategory_LineOfWalkArgShape(t *testing.T) {
	s := setupLookupServer(t)
	rec := &recordingFakeLineValidator{lowReturn: true}
	s.lineValidatorOverride = rec
	npc := setupNpc(t, s, 51, 52, 3)
	npc.typeId = 7 // category 5 per setupLookupServer

	_ = s.npcLookup.FindClosestNpcByCategory(3, 50, 50, 30, 5, int(objtype.HuntVisLineOfWalk))
	if rec.lowLevel != 3 || rec.lowSrcX != 50 || rec.lowSrcZ != 50 || rec.lowDestX != 51 || rec.lowDestZ != 52 {
		t.Errorf("LoW arg shape: got (level=%d, src=%d,%d, dst=%d,%d), want (3, 50, 50, 51, 52) — lookup-as-src",
			rec.lowLevel, rec.lowSrcX, rec.lowSrcZ, rec.lowDestX, rec.lowDestZ)
	}
}
