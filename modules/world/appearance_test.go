package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

func synthesizeTypes(t *testing.T) (*objtype.ObjTypeConfigs, *objtype.InvTypeConfigs) {
	t.Helper()
	objs := &objtype.ObjTypeConfigs{
		Configs:     make([]*objtype.ObjType, 10),
		ConfigNames: map[string]int{},
	}
	// id=1: platebody (wearPos=4, wearPos2=6, wearPos3=-1) - hides arms
	objs.Configs[1] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 1, DebugName: "platebody"},
		WearPos:    4, WearPos2: 6, WearPos3: -1,
	}
	// id=2: full helm (wearPos=8, wearPos2=0, wearPos3=1)
	objs.Configs[2] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 2, DebugName: "full_helm"},
		WearPos:    8, WearPos2: 0, WearPos3: 1,
	}

	invs := &objtype.InvTypeConfigs{
		Configs:     make([]*objtype.InvType, 2),
		ConfigNames: map[string]int{"worn": 0},
		Worn:        0,
	}
	invs.Configs[0] = &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "worn"},
		Size:       14,
	}
	return objs, invs
}

func TestGenerateAppearanceNakedPlayer(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}

	p.generateAppearance(objs, invs, 0)

	if len(p.appearanceBuf) == 0 {
		t.Fatal("appearanceBuf should be non-empty")
	}
	if p.lastAppearance != 0 {
		t.Errorf("lastAppearance: got %d, want 0", p.lastAppearance)
	}
	if p.appearanceBuf[0] != 0 {
		t.Errorf("gender byte: got %d, want 0", p.appearanceBuf[0])
	}
	if p.appearanceBuf[1] != 0 {
		t.Errorf("headicons byte: got %d, want 0", p.appearanceBuf[1])
	}
}

func TestGenerateAppearancePlatebodyEquipped(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	// Equip platebody at slot 4
	p.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}

	p.generateAppearance(objs, invs, 0)

	if len(p.appearanceBuf) == 0 {
		t.Fatal("appearanceBuf should be non-empty")
	}
}

// TestGenerateAppearanceSentinelDefaultReadsWorn pins NAI-21 Task (d):
// when p.appearanceInv == -1 (the default sentinel from newPlayer), the
// reader must fall back to invs.Worn. Production callers always go through
// sendLoginOK → SetAppearanceInv before any tick fires (NAI-22 Bundle 3),
// so this sentinel-fallback path exists as test-only safety for fixtures
// that build a Player via newPlayer(c) directly without login wiring.
func TestGenerateAppearanceSentinelDefaultReadsWorn(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	if p.appearanceInv != -1 {
		t.Fatalf("setup: p.appearanceInv should default to -1, got %d", p.appearanceInv)
	}
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	// Equip a platebody at slot 4 (the platebody synthesized in synthesizeTypes).
	p.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}

	p.generateAppearance(objs, invs, 0)

	// The platebody (id=1) at slot 4 must surface in the appearance buffer.
	// Equipped items use 2-byte form: 0x0200 | (id & 0x1FF). Search the buffer
	// for the encoded byte pair as a non-positional presence check.
	if len(p.appearanceBuf) == 0 {
		t.Fatal("appearanceBuf should be non-empty (sentinel must fall back to Worn)")
	}
	wantSlot4Hi := byte((0x200 | (1 & 0x1FF)) >> 8)
	wantSlot4Lo := byte((0x200 | (1 & 0x1FF)) & 0xFF)
	if !bytes.Contains(p.appearanceBuf, []byte{wantSlot4Hi, wantSlot4Lo}) {
		t.Errorf("appearanceBuf missing platebody encoded bytes (0x%02x 0x%02x); "+
			"sentinel mapping to Worn appears broken", wantSlot4Hi, wantSlot4Lo)
	}
}

// TestGenerateAppearanceExplicitWornIdMatchesSentinel pins NAI-21 Task (d):
// explicit p.appearanceInv = invs.Worn produces byte-identical output
// to the sentinel-default path. Proves the new explicit-set codepath
// matches the sentinel-mapped codepath for the common case.
func TestGenerateAppearanceExplicitWornIdMatchesSentinel(t *testing.T) {
	objs, invs := synthesizeTypes(t)

	// Player A: sentinel default (-1).
	pA, _ := newTestPlayer(t)
	pA.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	pA.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}
	pA.generateAppearance(objs, invs, 0)

	// Player B: explicit appearanceInv = invs.Worn.
	pB, _ := newTestPlayer(t)
	pB.appearanceInv = invs.Worn
	pB.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	pB.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}
	pB.generateAppearance(objs, invs, 0)

	if len(pA.appearanceBuf) != len(pB.appearanceBuf) {
		t.Fatalf("buffer length mismatch: sentinel=%d explicit=%d",
			len(pA.appearanceBuf), len(pB.appearanceBuf))
	}
	for i := range pA.appearanceBuf {
		if pA.appearanceBuf[i] != pB.appearanceBuf[i] {
			t.Errorf("byte %d differs: sentinel=0x%02x explicit=0x%02x",
				i, pA.appearanceBuf[i], pB.appearanceBuf[i])
		}
	}
}

// TestGenerateAppearanceCustomInvIdHonored pins NAI-21 Task (d) S7c-D1
// closure: when p.appearanceInv is set to a non-Worn inv id, the reader
// must read FROM that inv (not from invs.Worn). This is the actual S7c-D1
// bug-fix proof — the pre-fix reader at appearance.go:27 read p.invs[invs.Worn]
// regardless of p.appearanceInv, so custom-outfit scripts had no effect.
func TestGenerateAppearanceCustomInvIdHonored(t *testing.T) {
	objs, invs := synthesizeTypes(t)

	// Extend the synthesized invs to add a "custom" inv at id=1 (Worn is id=0).
	// synthesizeTypes pre-allocates Configs with len=2, slots [0]=Worn and [1]=nil.
	// Populate slot [1] in place (don't append, which would push to index 2).
	customInvId := 1
	invs.Configs[customInvId] = &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: customInvId, DebugName: "custom"},
		Size:       14,
	}

	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn:   inventory.FromType(invs.Configs[invs.Worn]), // empty
		customInvId: inventory.FromType(invs.Configs[customInvId]),
	}
	// Worn is empty; custom has a platebody at slot 4.
	p.invs[customInvId].Items[4] = &inventory.Item{Id: 1, Count: 1}
	p.appearanceInv = customInvId

	p.generateAppearance(objs, invs, 0)

	// The platebody must surface in the appearance buffer because the
	// reader read from p.invs[customInvId], NOT from p.invs[invs.Worn].
	wantSlot4Hi := byte((0x200 | (1 & 0x1FF)) >> 8)
	wantSlot4Lo := byte((0x200 | (1 & 0x1FF)) & 0xFF)
	if !bytes.Contains(p.appearanceBuf, []byte{wantSlot4Hi, wantSlot4Lo}) {
		t.Errorf("appearanceBuf missing platebody from custom inv; reader is " +
			"still reading from invs.Worn (S7c-D1 NOT closed)")
	}
}

// TestSetAppearanceInvBindsId pins NAI-22 Bundle 3: SetAppearanceInv
// writes the id to Player.appearanceInv and flips MaskAppearance.
// This is the existing setter (player_script.go:365); the test pins
// its contract independently of integration through client.go login
// wiring (which is harder to unit-test).
func TestSetAppearanceInvBindsId(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.appearanceInv != -1 {
		t.Fatalf("setup: p.appearanceInv should default to -1, got %d", p.appearanceInv)
	}

	p.SetAppearanceInv(42)

	if p.appearanceInv != 42 {
		t.Errorf("p.appearanceInv: got %d, want 42 (setter must bind id)", p.appearanceInv)
	}
	if p.masks&rsbuf.MaskAppearance == 0 {
		t.Errorf("p.masks: MaskAppearance bit unset (setter must flag for regeneration)")
	}
}

// TestGenerateAppearanceHighItemIdAdditive pins the bitwise-OR-vs-additive bug
// at appearance.go:77. TS Player.ts:1359 uses `p2(0x200 + equip.id)`; the
// pre-fix Go used `0x200 | (equip.Id & 0x1FF)` which silently masks ids >= 512.
// An item id of 600 must encode to 0x0200 + 600 = 0x0458, NOT 0x0200 | (600 & 0x1FF) = 0x0258.
func TestGenerateAppearanceHighItemIdAdditive(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	// Extend Configs to hold id=600.
	bigger := make([]*objtype.ObjType, 700)
	copy(bigger, objs.Configs)
	objs.Configs = bigger
	// id=600: wearPos=4, no hides (wearPos2/3 = -1).
	objs.Configs[600] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 600, DebugName: "high_id_torso"},
		WearPos:    4, WearPos2: -1, WearPos3: -1,
	}

	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	p.invs[invs.Worn].Items[4] = &inventory.Item{Id: 600, Count: 1}

	p.generateAppearance(objs, invs, 0)

	// Expected additive encoding: 0x200 + 600 = 0x458 → high=0x04, low=0x58.
	wantHi := byte((0x200 + 600) >> 8)
	wantLo := byte((0x200 + 600) & 0xFF)
	if !bytes.Contains(p.appearanceBuf, []byte{wantHi, wantLo}) {
		t.Errorf("appearanceBuf missing additive-encoded high item id 600 "+
			"(want 0x%02x 0x%02x); encoder is masking with 0x1FF instead of adding",
			wantHi, wantLo)
	}
	// Negative assertion: the buggy masked form must NOT appear in slot 4
	// (slot 4 is at offset 2 + 4*size; but mask collision with naked-slot zero
	// bytes can produce false positives — assert by direct slot-4 byte readback).
	// Layout: byte 0 gender, 1 headicons, then slots 0..11 each variable-width.
	// All slots 0..3 are -1 body parts → 1 byte (0) each. Slot 4 starts at offset 6.
	gotHi := p.appearanceBuf[6]
	gotLo := p.appearanceBuf[7]
	if gotHi != wantHi || gotLo != wantLo {
		t.Errorf("slot 4 bytes: got 0x%02x 0x%02x, want 0x%02x 0x%02x",
			gotHi, gotLo, wantHi, wantLo)
	}
}

// TestGenerateAppearanceHighBodyPartIdAdditive pins the bitwise-OR-vs-additive
// bug at appearance.go:85. TS Player.ts:1298 uses `0x100 + part`; the pre-fix
// Go used `0x100 | (part & 0xFF)` which silently masks body-part ids >= 256.
// A body-part id of 300 must encode to 0x0100 + 300 = 0x022C, NOT 0x0100 | (300 & 0xFF) = 0x012C.
func TestGenerateAppearanceHighBodyPartIdAdditive(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	// Set body[2] (slot 4 / torso) to a high idkit id 300.
	p.body[2] = 300

	p.generateAppearance(objs, invs, 0)

	wantHi := byte((0x100 + 300) >> 8)
	wantLo := byte((0x100 + 300) & 0xFF)
	if !bytes.Contains(p.appearanceBuf, []byte{wantHi, wantLo}) {
		t.Errorf("appearanceBuf missing additive-encoded high body-part id 300 "+
			"(want 0x%02x 0x%02x); encoder is masking with 0xFF instead of adding",
			wantHi, wantLo)
	}
	// Slot 4 starts at offset 6 (see test above).
	gotHi := p.appearanceBuf[6]
	gotLo := p.appearanceBuf[7]
	if gotHi != wantHi || gotLo != wantLo {
		t.Errorf("slot 4 bytes: got 0x%02x 0x%02x, want 0x%02x 0x%02x",
			gotHi, gotLo, wantHi, wantLo)
	}
}

func TestGenerateAppearance_SetsLastAppearanceToCurrentTick(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	// Pre-condition: default -1.
	if p.lastAppearance != -1 {
		t.Fatalf("default lastAppearance: got %d, want -1", p.lastAppearance)
	}
	p.generateAppearance(objs, invs, 42)
	if p.lastAppearance != 42 {
		t.Errorf("after generateAppearance(_,_,42): got %d, want 42", p.lastAppearance)
	}
	p.generateAppearance(objs, invs, 100)
	if p.lastAppearance != 100 {
		t.Errorf("after generateAppearance(_,_,100): got %d, want 100", p.lastAppearance)
	}
}
