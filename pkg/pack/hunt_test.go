package pack

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// buildHuntParseFn constructs a parseHuntConfigFor closure with the given
// per-domain PackFiles. Pass empty PackFiles for domains not exercised by
// the test.
func buildHuntParseFn(
	categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack *PackFile,
) ParseFn {
	return parseHuntConfigFor(categoryPack, invPack, locPack, npcPack, objPack, paramPack, varnPack, varpPack)
}

// emptyHuntPacks returns 8 empty PackFiles in the order expected by
// parseHuntConfigFor: category, inv, loc, npc, obj, param, varn, varp.
func emptyHuntPacks() (category, inv, loc, npc, obj, param, varn, varp *PackFile) {
	empty := func(t string) *PackFile { return newTestPF(t, map[int]string{}) }
	return empty("category"), empty("inv"), empty("loc"), empty("npc"),
		empty("obj"), empty("param"), empty("varn"), empty("varp")
}

// ---- Parser tests ----

func TestParseHuntConfig_TypeNpc(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, varp := emptyHuntPacks()
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("type", "npc")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.HuntModeNpc {
		t.Fatalf("got %v, want HuntModeNpc=%d", v, objtype.HuntModeNpc)
	}
}

func TestParseHuntConfig_TypeOff(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, varp := emptyHuntPacks()
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("type", "off")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.HuntModeOff {
		t.Fatalf("got %v, want HuntModeOff=0", v)
	}
}

func TestParseHuntConfig_TypeUnknown(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, varp := emptyHuntPacks()
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	_, ok, err := fn("type", "bogus")
	if err == nil {
		t.Fatal("want error for unknown type")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseHuntConfig_CheckVisLineOfSight(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, varp := emptyHuntPacks()
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("check_vis", "lineofsight")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.HuntVisLineOfSight {
		t.Fatalf("got %v, want HuntVisLineOfSight=%d", v, objtype.HuntVisLineOfSight)
	}
}

func TestParseHuntConfig_RateValid(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, varp := emptyHuntPacks()
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("rate", "5")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 5 {
		t.Fatalf("got %v, want 5", v)
	}
}

func TestParseHuntConfig_RateOutOfRange(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, varp := emptyHuntPacks()
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	_, ok, err := fn("rate", "0")
	if err == nil {
		t.Fatal("want error for rate=0")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseHuntConfig_CheckNotcombatVarp(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, _ := emptyHuntPacks()
	varp := newTestPF("varp", map[int]string{42: "vp1"})
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("check_notcombat", "%vp1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 42 {
		t.Fatalf("got %v, want 42", v)
	}
}

func TestParseHuntConfig_CheckNotcombatMissingPercent(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, varp := emptyHuntPacks()
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	_, ok, err := fn("check_notcombat", "vp1")
	if err == nil {
		t.Fatal("want error for missing % prefix")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseHuntConfig_CheckNotcombatSelfVarn(t *testing.T) {
	cat, inv, loc, npc, obj, param, _, varp := emptyHuntPacks()
	varn := newTestPF("varn", map[int]string{11: "vn1"})
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("check_notcombat_self", "%vn1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 11 {
		t.Fatalf("got %v, want 11", v)
	}
}

func TestParseHuntConfig_FindNewmodeOpobj2Bug(t *testing.T) {
	// NAI-198-D-HUNT-OPOBJ2-TS-BUG: 'opobj2' must map to NPCModeOpObj1 (27),
	// NOT NPCModeOpObj2 (28). This is a faithful port of the TS upstream bug.
	cat, inv, loc, npc, obj, param, varn, varp := emptyHuntPacks()
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("find_newmode", "opobj2")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.NPCModeOpObj1 {
		t.Fatalf("NAI-198-D-HUNT-OPOBJ2-TS-BUG: got %v, want NPCModeOpObj1=%d (not NPCModeOpObj2=%d)",
			v, objtype.NPCModeOpObj1, objtype.NPCModeOpObj2)
	}
}

func TestParseHuntConfig_CheckInvValid(t *testing.T) {
	cat, _, loc, npc, _, param, varn, varp := emptyHuntPacks()
	inv := newTestPF("inv", map[int]string{0: "bank"})
	obj := newTestPF("obj", map[int]string{2: "coins"})
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("check_inv", "bank,coins,>10")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	ci := v.(huntCheckInv)
	if ci.inv != 0 || ci.obj != 2 || ci.condition != ">" || ci.val != 10 {
		t.Fatalf("got %+v, want inv=0 obj=2 condition=> val=10", ci)
	}
}

func TestParseHuntConfig_CheckInvparamValid(t *testing.T) {
	cat, _, loc, npc, obj, _, varn, varp := emptyHuntPacks()
	inv := newTestPF("inv", map[int]string{0: "bank"})
	param := newTestPF("param", map[int]string{5: "strength"})
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("check_invparam", "bank,strength,=3")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	ci := v.(huntCheckInvParam)
	if ci.inv != 0 || ci.param != 5 || ci.condition != "=" || ci.val != 3 {
		t.Fatalf("got %+v, want inv=0 param=5 condition== val=3", ci)
	}
}

func TestParseHuntConfig_ExtracheckVarValid(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, _ := emptyHuntPacks()
	varp := newTestPF("varp", map[int]string{7: "combat_level"})
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("extracheck_var", "%combat_level,>5")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	cv := v.(huntCheckVarParsed)
	if cv.varp != 7 || cv.condition != ">" || cv.val != 5 {
		t.Fatalf("got %+v, want varp=7 condition=> val=5", cv)
	}
}

func TestParseHuntConfig_ExtracheckVarAndOperator(t *testing.T) {
	// '&' (no-common-bits) is whitelisted ONLY for extracheck_var at
	// TS HuntConfig.ts:366 @dee467c8 (['=', '>', '<', '!', '&']).
	cat, inv, loc, npc, obj, param, varn, _ := emptyHuntPacks()
	varp := newTestPF("varp", map[int]string{7: "flags"})
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("extracheck_var", "%flags,&8")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	cv := v.(huntCheckVarParsed)
	if cv.varp != 7 || cv.condition != "&" || cv.val != 8 {
		t.Fatalf("got %+v, want varp=7 condition=& val=8", cv)
	}
}

func TestParseHuntConfig_CheckInvAndOperatorRejected(t *testing.T) {
	// '&' is NOT in the check_inv whitelist at TS HuntConfig.ts:320
	// @dee467c8 (still ['=', '>', '<', '!']) — must be rejected.
	cat, _, loc, npc, _, param, varn, varp := emptyHuntPacks()
	inv := newTestPF("inv", map[int]string{0: "bank"})
	obj := newTestPF("obj", map[int]string{2: "coins"})
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	_, ok, err := fn("check_inv", "bank,coins,&8")
	if err == nil {
		t.Fatal("want error: & is not a valid check_inv operator")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseHuntConfig_CheckInvparamAndOperatorRejected(t *testing.T) {
	// '&' is NOT in the check_invparam whitelist at TS HuntConfig.ts:346
	// @dee467c8 (still ['=', '>', '<', '!']) — must be rejected.
	cat, _, loc, npc, obj, _, varn, varp := emptyHuntPacks()
	inv := newTestPF("inv", map[int]string{0: "bank"})
	param := newTestPF("param", map[int]string{5: "strength"})
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	_, ok, err := fn("check_invparam", "bank,strength,&8")
	if err == nil {
		t.Fatal("want error: & is not a valid check_invparam operator")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseHuntConfig_UnknownKey(t *testing.T) {
	cat, inv, loc, npc, obj, param, varn, varp := emptyHuntPacks()
	fn := buildHuntParseFn(cat, inv, loc, npc, obj, param, varn, varp)
	v, ok, err := fn("not_a_key", "whatever")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok=true for unknown key; want false")
	}
	if v != nil {
		t.Fatalf("v=%v, want nil", v)
	}
}

// ---- Packer helper ----

// buildHuntPF builds a hunt PackFile with one entry at id=0.
func buildHuntPF(name string) *PackFile {
	if name == "" {
		return newTestPF("hunt", map[int]string{0: ""})
	}
	return newTestPF("hunt", map[int]string{0: name})
}

// ---- Packer tests ----

func TestPackHuntConfigs_OpcodeTypeOnly(t *testing.T) {
	// type=npc → emits opcode 1, value=2 (HuntModeNpc).
	pf := buildHuntPF("h1")
	configs := map[string][]ConfigLine{
		"h1": {
			{Key: "type", Value: objtype.HuntModeNpc},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Header p2(1) + opcode 1 + value 2 + debugname 250 + "h1"\n + terminator
	want := []byte{
		0x00, 0x01, // size header
		0x01, 0x02, // opcode 1, HuntModeNpc=2
		0xfa, 'h', '1', 0x0a, // opcode 250 + "h1" LF
		0x00, // terminator
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeTypeOffSkipped(t *testing.T) {
	// type=off → does NOT emit opcode 1 (default-skip).
	pf := buildHuntPF("h_off")
	configs := map[string][]ConfigLine{
		"h_off": {
			{Key: "type", Value: objtype.HuntModeOff},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// No opcode 1; only 250-trailer + terminator.
	want := []byte{
		0x00, 0x01, // size header
		0xfa, 'h', '_', 'o', 'f', 'f', 0x0a, // 250 + "h_off" LF
		0x00, // terminator
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeCheckVis(t *testing.T) {
	// check_vis=lineofsight → emits opcode 2, value=1.
	pf := buildHuntPF("hv")
	configs := map[string][]ConfigLine{
		"hv": {
			{Key: "check_vis", Value: objtype.HuntVisLineOfSight},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01,
		0x02, 0x01, // opcode 2, HuntVisLineOfSight=1
		0xfa, 'h', 'v', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeRate(t *testing.T) {
	// rate=5 → emits opcode 11, p2(5).
	pf := buildHuntPF("hr")
	configs := map[string][]ConfigLine{
		"hr": {
			{Key: "rate", Value: 5},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 11, p2(5) = 0x00 0x05
	want := []byte{
		0x00, 0x01,
		0x0b, 0x00, 0x05, // opcode 11 + p2(5)
		0xfa, 'h', 'r', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeRateOneSkipped(t *testing.T) {
	// rate=1 → does NOT emit opcode 11 (default-skip).
	pf := buildHuntPF("hr1")
	configs := map[string][]ConfigLine{
		"hr1": {
			{Key: "rate", Value: 1},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01,
		0xfa, 'h', 'r', '1', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeNobodynearPauseSkipped(t *testing.T) {
	// nobodynear=pausehunt → does NOT emit opcode 7 (is the default value).
	pf := buildHuntPF("hn")
	configs := map[string][]ConfigLine{
		"hn": {
			{Key: "nobodynear", Value: objtype.HuntNobodyNearPauseHunt},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01,
		0xfa, 'h', 'n', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeNobodynearKeephunting(t *testing.T) {
	// nobodynear=keephunting → emits opcode 7, value=0.
	pf := buildHuntPF("hnk")
	configs := map[string][]ConfigLine{
		"hnk": {
			{Key: "nobodynear", Value: objtype.HuntNobodyNearKeepHunting},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01,
		0x07, 0x00, // opcode 7 + value 0 (HuntNobodyNearKeepHunting)
		0xfa, 'h', 'n', 'k', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeCheckNotcombatVarp(t *testing.T) {
	// check_notcombat with varp at id=42 → emits opcode 8, p2(42).
	pf := buildHuntPF("hnc")
	configs := map[string][]ConfigLine{
		"hnc": {
			// Parser resolves to the varp id directly.
			{Key: "check_notcombat", Value: 42},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 8 + p2(42) = 0x00 0x2a
	want := []byte{
		0x00, 0x01,
		0x08, 0x00, 0x2a, // opcode 8 + p2(42)
		0xfa, 'h', 'n', 'c', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeCheckNotcombatSelfVarn(t *testing.T) {
	// check_notcombat_self with varn at id=11 → emits opcode 9, p2(11).
	pf := buildHuntPF("hncs")
	configs := map[string][]ConfigLine{
		"hncs": {
			{Key: "check_notcombat_self", Value: 11},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 9 + p2(11) = 0x00 0x0b
	want := []byte{
		0x00, 0x01,
		0x09, 0x00, 0x0b, // opcode 9 + p2(11)
		0xfa, 'h', 'n', 'c', 's', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeCheckCategoryWithMatchingType(t *testing.T) {
	// type=npc + check_category → emits opcode 12, p2(3).
	pf := buildHuntPF("hcc")
	configs := map[string][]ConfigLine{
		"hcc": {
			{Key: "type", Value: objtype.HuntModeNpc},
			{Key: "check_category", Value: 3},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 1 + value 2 (type=npc) + opcode 12 + p2(3)
	want := []byte{
		0x00, 0x01,
		0x01, 0x02, // opcode 1, type=npc=2
		0x0c, 0x00, 0x03, // opcode 12 + p2(3)
		0xfa, 'h', 'c', 'c', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeCheckCategoryWithoutTypeErrors(t *testing.T) {
	// check_category without a matching type (no type line) → error.
	pf := buildHuntPF("hcc_err")
	configs := map[string][]ConfigLine{
		"hcc_err": {
			{Key: "check_category", Value: 3},
		},
	}
	_, err := packHuntConfigs(configs, pf, nil)
	if err == nil {
		t.Fatal("want error for check_category without type")
	}
}

func TestPackHuntConfigs_OpcodeCheckCategoryWithPlayerTypeErrors(t *testing.T) {
	// check_category with type=player → error (player is not NPC/OBJ/SCENERY).
	pf := buildHuntPF("hcc_p")
	configs := map[string][]ConfigLine{
		"hcc_p": {
			{Key: "type", Value: objtype.HuntModePlayer},
			{Key: "check_category", Value: 3},
		},
	}
	_, err := packHuntConfigs(configs, pf, nil)
	if err == nil {
		t.Fatal("want error for check_category with type=player")
	}
}

func TestPackHuntConfigs_OpcodeCheckInvWithType(t *testing.T) {
	// type=player + check_inv=bank,coins,>10 → emits opcode 16.
	pf := buildHuntPF("hci")
	configs := map[string][]ConfigLine{
		"hci": {
			{Key: "type", Value: objtype.HuntModePlayer},
			{Key: "check_inv", Value: huntCheckInv{inv: 0, obj: 2, condition: ">", val: 10}},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 1+value 1(player) | opcode 16 | p2(0) p2(2) pjstr(">") p4(10)
	// pjstr(">") = '>' 0x0a
	// p4(10) = 0x00 0x00 0x00 0x0a
	want := []byte{
		0x00, 0x01,
		0x01, 0x01, // opcode 1, type=player=1
		0x10,       // opcode 16
		0x00, 0x00, // p2(inv=0)
		0x00, 0x02, // p2(obj=2)
		'>', 0x0a, // pjstr(">") with LF terminator
		0x00, 0x00, 0x00, 0x0a, // p4(val=10)
		0xfa, 'h', 'c', 'i', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeCheckInvWithoutTypeErrors(t *testing.T) {
	// check_inv without type=player → error.
	pf := buildHuntPF("hci_err")
	configs := map[string][]ConfigLine{
		"hci_err": {
			{Key: "check_inv", Value: huntCheckInv{inv: 0, obj: 2, condition: ">", val: 10}},
		},
	}
	_, err := packHuntConfigs(configs, pf, nil)
	if err == nil {
		t.Fatal("want error for check_inv without type=player")
	}
}

func TestPackHuntConfigs_OpcodeExtraCheckVar1Through3(t *testing.T) {
	// 3 extracheck_var entries → emits opcodes 19, 20, 21. Renumbered from
	// 18-20 by Engine-TS 8139461a, which claims 18 for check_invcat
	// (TS HuntConfig.ts:562 @1d25566c).
	pf := buildHuntPF("hev")
	configs := map[string][]ConfigLine{
		"hev": {
			{Key: "type", Value: objtype.HuntModePlayer},
			{Key: "extracheck_var", Value: huntCheckVarParsed{varp: 7, condition: ">", val: 10}},
			{Key: "extracheck_var", Value: huntCheckVarParsed{varp: 8, condition: "<", val: 5}},
			{Key: "extracheck_var", Value: huntCheckVarParsed{varp: 9, condition: "=", val: 0}},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 1 + 1(player)
	// opcode 19 + p2(7) + pjstr(">") + p4(10)
	// opcode 20 + p2(8) + pjstr("<") + p4(5)
	// opcode 21 + p2(9) + pjstr("=") + p4(0)
	// 250 + "hev"\n + terminator
	want := []byte{
		0x00, 0x01,
		0x01, 0x01, // type=player
		0x13, 0x00, 0x07, '>', 0x0a, 0x00, 0x00, 0x00, 0x0a, // opcode 19 + p2(7) + ">" + p4(10)
		0x14, 0x00, 0x08, '<', 0x0a, 0x00, 0x00, 0x00, 0x05, // opcode 20 + p2(8) + "<" + p4(5)
		0x15, 0x00, 0x09, '=', 0x0a, 0x00, 0x00, 0x00, 0x00, // opcode 21 + p2(9) + "=" + p4(0)
		0xfa, 'h', 'e', 'v', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_OpcodeExtraCheckVarOverflow(t *testing.T) {
	// 4 extracheck_var entries → error (max 3).
	pf := buildHuntPF("hev_err")
	configs := map[string][]ConfigLine{
		"hev_err": {
			{Key: "type", Value: objtype.HuntModePlayer},
			{Key: "extracheck_var", Value: huntCheckVarParsed{varp: 1, condition: ">", val: 0}},
			{Key: "extracheck_var", Value: huntCheckVarParsed{varp: 2, condition: ">", val: 0}},
			{Key: "extracheck_var", Value: huntCheckVarParsed{varp: 3, condition: ">", val: 0}},
			{Key: "extracheck_var", Value: huntCheckVarParsed{varp: 4, condition: ">", val: 0}},
		},
	}
	_, err := packHuntConfigs(configs, pf, nil)
	if err == nil {
		t.Fatal("want error for more than 3 extracheck_var")
	}
}

func TestPackHuntConfigs_OPOBJ2BugPin(t *testing.T) {
	// NAI-198-D-HUNT-OPOBJ2-TS-BUG: find_newmode=opobj2 must emit
	// NPCModeOpObj1=27, NOT NPCModeOpObj2=28.
	// This is a faithful port of the TS upstream bug at HuntConfig.ts:201-202.
	pf := buildHuntPF("hopobj2")
	configs := map[string][]ConfigLine{
		"hopobj2": {
			// Parser sets this to NPCModeOpObj1 per the TS bug.
			// NPCModeNone=0 is the default-skip value; NPCModeOpObj1=27 is non-zero,
			// so opcode 6 is emitted.
			{Key: "find_newmode", Value: objtype.NPCModeOpObj1},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// opcode 6 + uint8(NPCModeOpObj1=27=0x1b)
	want := []byte{
		0x00, 0x01,
		0x06, 0x1b, // opcode 6 + NPCModeOpObj1=27
		0xfa, 'h', 'o', 'p', 'o', 'b', 'j', '2', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
	// Positive-safety check: value emitted is NOT NPCModeOpObj2=28.
	opcodeValIdx := 3 // 2-byte header + opcode byte, then value byte at index 3
	if pd.Dat.Data[opcodeValIdx] == uint8(objtype.NPCModeOpObj2) {
		t.Fatalf("emitted NPCModeOpObj2=28 — the TS bug (opobj2→OPOBJ1=27) was NOT ported faithfully")
	}
}

func TestPackHuntConfigs_DebugnameTrailer(t *testing.T) {
	// Debugname present → emits opcode 250 + PJStr(name).
	pf := buildHuntPF("myname")
	configs := map[string][]ConfigLine{}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Even with no config block, if the pack file has a name, 250-trailer is emitted.
	want := []byte{
		0x00, 0x01,
		0xfa, 'm', 'y', 'n', 'a', 'm', 'e', 0x0a, // 250 + "myname"\n
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_EmptyDebugname_No250Trailer(t *testing.T) {
	// Empty debugname → no 250 byte emitted; only the terminator.
	pf := buildHuntPF("")
	configs := map[string][]ConfigLine{}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	// PackFile with empty name at id=0: Max will be 0+1=1, but name="" so 250 not emitted.
	// Just: header + terminator.
	want := []byte{
		0x00, 0x01, // size header
		0x00, // terminator only
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_CheckNpcWithMatchingType(t *testing.T) {
	// type=npc + check_npc → emits opcode 13, p2(5).
	pf := buildHuntPF("hcn")
	configs := map[string][]ConfigLine{
		"hcn": {
			{Key: "type", Value: objtype.HuntModeNpc},
			{Key: "check_npc", Value: 5},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01,
		0x01, 0x02, // opcode 1, type=npc=2
		0x0d, 0x00, 0x05, // opcode 13 + p2(5)
		0xfa, 'h', 'c', 'n', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackHuntConfigs_CheckNpcWithMutexConflictErrors(t *testing.T) {
	// check_npc + check_category (mutex violation) → error.
	pf := buildHuntPF("hcn_mx")
	configs := map[string][]ConfigLine{
		"hcn_mx": {
			{Key: "type", Value: objtype.HuntModeNpc},
			{Key: "check_category", Value: 3},
			{Key: "check_npc", Value: 5},
		},
	}
	_, err := packHuntConfigs(configs, pf, nil)
	if err == nil {
		t.Fatal("want error for check_npc with mutex conflict (check_category present)")
	}
}

// TestPackHuntConfigs_OpcodeCheckInvCat pins the check_invcat emission added by
// Engine-TS 8139461a (TS HuntConfig.ts:543-556 @1d25566c): opcode 18 carrying
// p2(inv), p2(category), pjstr(condition), p4(val).
func TestPackHuntConfigs_OpcodeCheckInvCat(t *testing.T) {
	pf := buildHuntPF("hic")
	configs := map[string][]ConfigLine{
		"hic": {
			{Key: "type", Value: objtype.HuntModePlayer},
			{Key: "check_invcat", Value: huntCheckInvCat{inv: 7, category: 31, condition: ">", val: 5}},
		},
	}
	pd, err := packHuntConfigs(configs, pf, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x01,
		0x01, 0x01, // type=player
		0x12, 0x00, 0x07, 0x00, 0x1f, '>', 0x0a, 0x00, 0x00, 0x00, 0x05, // 18 + p2(7) + p2(31) + ">" + p4(5)
		0xfa, 'h', 'i', 'c', 0x0a,
		0x00,
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

// TestPackHuntConfigs_CheckInvCatMutex pins that check_invcat is mutually
// exclusive with the other five check_* keys, and that they are mutually
// exclusive with it (TS HuntConfig.ts:477-556 @1d25566c added check_invcat to
// every one of those every() guards).
func TestPackHuntConfigs_CheckInvCatMutex(t *testing.T) {
	for _, other := range []string{"check_category", "check_npc", "check_obj", "check_loc"} {
		t.Run(other, func(t *testing.T) {
			pf := buildHuntPF("hmx")
			configs := map[string][]ConfigLine{
				"hmx": {
					{Key: "type", Value: objtype.HuntModePlayer},
					{Key: other, Value: 3},
					{Key: "check_invcat", Value: huntCheckInvCat{inv: 7, category: 31, condition: ">", val: 5}},
				},
			}
			if _, err := packHuntConfigs(configs, pf, nil); err == nil {
				t.Errorf("check_invcat + %s: got nil error, want mutex rejection", other)
			}
		})
	}

	t.Run("check_inv", func(t *testing.T) {
		pf := buildHuntPF("hmx2")
		configs := map[string][]ConfigLine{
			"hmx2": {
				{Key: "type", Value: objtype.HuntModePlayer},
				{Key: "check_inv", Value: huntCheckInv{inv: 1, obj: 2, condition: "=", val: 1}},
				{Key: "check_invcat", Value: huntCheckInvCat{inv: 7, category: 31, condition: ">", val: 5}},
			},
		}
		if _, err := packHuntConfigs(configs, pf, nil); err == nil {
			t.Error("check_invcat + check_inv: got nil error, want mutex rejection")
		}
	})

	t.Run("check_invparam", func(t *testing.T) {
		pf := buildHuntPF("hmx3")
		configs := map[string][]ConfigLine{
			"hmx3": {
				{Key: "type", Value: objtype.HuntModePlayer},
				{Key: "check_invparam", Value: huntCheckInvParam{inv: 1, param: 2, condition: "=", val: 1}},
				{Key: "check_invcat", Value: huntCheckInvCat{inv: 7, category: 31, condition: ">", val: 5}},
			},
		}
		if _, err := packHuntConfigs(configs, pf, nil); err == nil {
			t.Error("check_invcat + check_invparam: got nil error, want mutex rejection")
		}
	})
}
