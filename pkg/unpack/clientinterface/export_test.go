package clientinterface

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack"
)

// -----------------------------------------------------------------------
// renameModel tests
// -----------------------------------------------------------------------

// TestRenameModelFull_StableNamePassthrough verifies that a model whose name
// does NOT start with "model_" is returned unchanged without touching the pack.
func TestRenameModelFull_StableNamePassthrough(t *testing.T) {
	dir := t.TempDir()
	pf, err := pack.NewPackFile(dir, "model", nil)
	if err != nil {
		t.Fatal(err)
	}
	pf.Register(42, "com_i3")

	calls := 0
	got := renameModelFull(42, pf, dir, nil, nil, func(src, dst string) error {
		calls++
		return nil
	})
	if got != "com_i3" {
		t.Errorf("got %q, want com_i3", got)
	}
	if calls != 0 {
		t.Errorf("rename should not have been called")
	}
}

// TestRenameModelFull_FirstAvailableSlot verifies that model_N is renamed to
// com_i1 when com_i1 is free.
func TestRenameModelFull_FirstAvailableSlot(t *testing.T) {
	dir := t.TempDir()
	pf, _ := pack.NewPackFile(dir, "model", nil)
	pf.Register(10, "model_3024")

	// Create a fake .ob2 file so the rename fires.
	modelsDir := filepath.Join(dir, "models")
	_ = os.MkdirAll(modelsDir, 0o755)
	_ = os.WriteFile(filepath.Join(modelsDir, "model_3024.ob2"), []byte("data"), 0o644)

	existing := pack.ListFilesExt(modelsDir, ".ob2")

	var renames [][2]string
	got := renameModelFull(10, pf, dir, existing, nil, func(src, dst string) error {
		renames = append(renames, [2]string{src, dst})
		return nil
	})

	if got != "com_i1" {
		t.Errorf("got %q, want com_i1", got)
	}
	if len(renames) != 1 {
		t.Fatalf("rename calls: got %d, want 1", len(renames))
	}
	if !strings.HasSuffix(renames[0][0], "model_3024.ob2") {
		t.Errorf("rename src: got %s, want …/model_3024.ob2", renames[0][0])
	}
	if !strings.HasSuffix(renames[0][1], "com_i1.ob2") {
		t.Errorf("rename dst: got %s, want …/com_i1.ob2", renames[0][1])
	}
	if pf.GetByID(10) != "com_i1" {
		t.Errorf("pack entry: got %q, want com_i1", pf.GetByID(10))
	}
}

// TestRenameModelFull_CollisionLoop verifies that when com_i1 and com_i2 are
// already taken, the new model is assigned com_i3.
func TestRenameModelFull_CollisionLoop(t *testing.T) {
	dir := t.TempDir()
	pf, _ := pack.NewPackFile(dir, "model", nil)
	pf.Register(1, "com_i1")
	pf.Register(2, "com_i2")
	pf.Register(10, "model_9999")
	pf.RefreshNames()

	modelsDir := filepath.Join(dir, "models")
	_ = os.MkdirAll(modelsDir, 0o755)
	_ = os.WriteFile(filepath.Join(modelsDir, "model_9999.ob2"), []byte("x"), 0o644)
	existing := pack.ListFilesExt(modelsDir, ".ob2")

	got := renameModelFull(10, pf, dir, existing, nil, func(src, dst string) error {
		return nil
	})
	if got != "com_i3" {
		t.Errorf("got %q, want com_i3", got)
	}
}

// TestRenameModelFull_FileNotFound verifies that when the .ob2 file is absent,
// the error callback fires and the model is still registered.
func TestRenameModelFull_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	pf, _ := pack.NewPackFile(dir, "model", nil)
	pf.Register(7, "model_4242")

	var errors []string
	got := renameModelFull(7, pf, dir, nil, func(format string, args ...any) {
		errors = append(errors, format)
	}, func(src, dst string) error { return nil })

	if got != "com_i1" {
		t.Errorf("got %q, want com_i1", got)
	}
	if len(errors) == 0 {
		t.Error("expected errorf to be called on missing file")
	}
	if pf.GetByID(7) != "com_i1" {
		t.Errorf("pack entry: got %q, want com_i1", pf.GetByID(7))
	}
}

// -----------------------------------------------------------------------
// ExportOrder tests
// -----------------------------------------------------------------------

// TestExportOrder_Format verifies the exact byte content of the output file:
// each ID on its own line, trailing newline.
func TestExportOrder_Format(t *testing.T) {
	dir := t.TempDir()
	dec := &Decoded{
		Order: []int{0, 5, 10, 3},
	}
	path := filepath.Join(dir, "pack", "interface.order")
	if err := ExportOrder(dec, path); err != nil {
		t.Fatalf("ExportOrder: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "0\n5\n10\n3\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

// TestExportOrder_Empty verifies that an empty order produces a single newline
// (strings.Join([], '\n') + '\n' == '\n' in TS).
func TestExportOrder_Empty(t *testing.T) {
	dir := t.TempDir()
	dec := &Decoded{Order: []int{}}
	path := filepath.Join(dir, "pack", "interface.order")
	if err := ExportOrder(dec, path); err != nil {
		t.Fatalf("ExportOrder: %v", err)
	}
	data, _ := os.ReadFile(path)
	// empty order → just a newline (join([]) = "" + '\n' = "\n")
	if string(data) != "\n" {
		t.Errorf("got %q, want newline", string(data))
	}
}

// -----------------------------------------------------------------------
// exportComponent tests (representative per comType)
// -----------------------------------------------------------------------

func makeIfPack(t *testing.T) *pack.PackFile {
	t.Helper()
	dir := t.TempDir()
	pf, _ := pack.NewPackFile(dir, "interface", nil)
	return pf
}

func makeSeqPack(t *testing.T) *pack.PackFile {
	t.Helper()
	dir := t.TempDir()
	pf, _ := pack.NewPackFile(dir, "seq", nil)
	return pf
}

func makeObjPack(t *testing.T) *pack.PackFile {
	t.Helper()
	dir := t.TempDir()
	pf, _ := pack.NewPackFile(dir, "obj", nil)
	return pf
}

func makeVarpPack(t *testing.T) *pack.PackFile {
	t.Helper()
	dir := t.TempDir()
	pf, _ := pack.NewPackFile(dir, "varp", nil)
	return pf
}

func makeVarbitPack(t *testing.T) *pack.PackFile {
	t.Helper()
	dir := t.TempDir()
	pf, _ := pack.NewPackFile(dir, "varbit", nil)
	return pf
}

// noRenameModel is a renameModel stub that returns id as "model_<id>" for tests.
func noRenameModel(id int) string {
	return "test_model"
}

// TestExport_TypeText pins the exact .if lines for a TYPE_TEXT child component.
func TestExport_TypeText(t *testing.T) {
	ifPack := makeIfPack(t)
	// Root: id=0, name="myui"
	ifPack.Register(0, "myui")
	// Child: id=1, name="myui:com_0"
	ifPack.Register(1, "myui:com_0")
	ifPack.RefreshNames()

	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{10},
		ChildY:    []int{20},
	}
	child := &Component{
		ID:           1,
		RootLayer:    0,
		ComType:      TypeText,
		ButtonType:   ButtonOK,
		Width:        100,
		Height:       14,
		Font:         1, // p12
		FontSet:      true,
		Shadowed:     true,
		Text:         "Hello",
		ActiveText:   "World",
		Colour:       0xFF0000,
		ActiveColour: 0x00FF00,
		OverColour:   0x0000FF,
		Option:       "Click",
	}
	components := make([]*Component, 2)
	components[0] = root
	components[1] = child

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")

	joined := strings.Join(lines, "\n")
	checks := []string{
		"[com_0]",
		"type=text",
		"x=10",
		"y=20",
		"buttontype=normal",
		"width=100",
		"height=14",
		"font=p12",
		"shadowed=yes",
		"text=Hello",
		"activetext=World",
		"colour=0xFF0000",
		"activecolour=0x00FF00",
		"overcolour=0x0000FF",
		"option=Click",
	}
	for _, c := range checks {
		if !strings.Contains(joined, c) {
			t.Errorf("missing %q in output:\n%s", c, joined)
		}
	}
}

// TestExport_TypeLayer_Root verifies that a root (id==rootLayer) does NOT emit
// the [name] header block, only the scroll/hide and child recursion.
func TestExport_TypeLayer_Root(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "myui")
	ifPack.RefreshNames()

	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		Scroll:    100,
		Hide:      true,
		ChildID:   []int{},
		ChildX:    []int{},
		ChildY:    []int{},
	}
	components := []*Component{root}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	// Root should NOT emit [myui] header
	if strings.Contains(joined, "[myui]") {
		t.Errorf("root should not emit [myui]: %s", joined)
	}
	if !strings.Contains(joined, "scroll=100") {
		t.Errorf("missing scroll=100: %s", joined)
	}
	if !strings.Contains(joined, "hide=yes") {
		t.Errorf("missing hide=yes: %s", joined)
	}
}

// TestExport_TypeModel pins the exact lines for a TYPE_MODEL component.
func TestExport_TypeModel(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	seqPack := makeSeqPack(t)
	seqPack.Register(7, "walk")
	seqPack.RefreshNames()

	child := &Component{
		ID:          1,
		RootLayer:   0,
		ComType:     TypeModel,
		Model:       256,
		ActiveModel: 0,
		Anim:        7,
		ActiveAnim:  -1,
		Zoom:        500,
		Xan:         100,
		Yan:         200,
	}

	rmFn := func(id int) string { return "com_i1" }

	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{50},
		ChildY:    []int{60},
	}
	components := make([]*Component, 2)
	components[0] = root
	components[1] = child

	lines := exportComponent(root, components, ifPack, makeObjPack(t), seqPack, makeVarpPack(t), makeVarbitPack(t), rmFn, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	checks := []string{
		"[com_0]",
		"type=model",
		"x=50",
		"y=60",
		"model=com_i1",
		"anim=walk",
		"zoom=500",
		"xan=100",
		"yan=200",
	}
	for _, c := range checks {
		if !strings.Contains(joined, c) {
			t.Errorf("missing %q:\n%s", c, joined)
		}
	}
	// activemodel should be absent (value 0)
	if strings.Contains(joined, "activemodel=") {
		t.Errorf("unexpected activemodel= in output: %s", joined)
	}
	// activeanim should be absent (value -1)
	if strings.Contains(joined, "activeanim=") {
		t.Errorf("unexpected activeanim= in output: %s", joined)
	}
}

// TestExport_TypeInv pins the TYPE_INV lines including draggable, margin, slot, option.
func TestExport_TypeInv(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	spr0 := "backpack"
	invSlotSprite := make([]*string, 20)
	invSlotSprite[0] = &spr0
	invSlotOffsetX := make([]int, 20)
	invSlotOffsetY := make([]int, 20)
	invSlotOffsetX[0] = 5
	invSlotOffsetY[0] = 10

	iops := make([]*string, 5)
	take := "Take"
	iops[0] = &take

	child := &Component{
		ID:             1,
		RootLayer:      0,
		ComType:        TypeInv,
		Draggable:      true,
		Interactable:   true,
		Usable:         false,
		MarginX:        2,
		MarginY:        3,
		InvSlotSprite:  invSlotSprite,
		InvSlotOffsetX: invSlotOffsetX,
		InvSlotOffsetY: invSlotOffsetY,
		Iops:           iops,
		ActionVerb:     "Use",
		Action:         "with",
		ActionTarget:   3, // obj | npc
	}

	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	checks := []string{
		"[com_0]",
		"type=inv",
		"draggable=yes",
		"interactable=yes",
		"margin=2,3",
		"slot1=backpack:5,10",
		"option1=Take",
		"actionverb=Use",
		"actiontarget=obj,npc",
		"action=with",
	}
	for _, c := range checks {
		if !strings.Contains(joined, c) {
			t.Errorf("missing %q:\n%s", c, joined)
		}
	}
}

// TestExport_TypeGraphic pins graphic/activegraphic emission.
func TestExport_TypeGraphic(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:            1,
		RootLayer:     0,
		ComType:       TypeGraphic,
		Graphic:       "coins,1",
		ActiveGraphic: "coins,2",
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "graphic=coins,1") {
		t.Errorf("missing graphic=coins,1: %s", joined)
	}
	if !strings.Contains(joined, "activegraphic=coins,2") {
		t.Errorf("missing activegraphic=coins,2: %s", joined)
	}
}

// TestExport_ScriptOp_PushVar pins the script op 5 (pushvar) emission.
func TestExport_ScriptOp_PushVar(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	varpPack := makeVarpPack(t)
	varpPack.Register(3, "com_mode")
	varpPack.RefreshNames()

	child := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeRect,
		Fill:      true,
		// Script: op5, varp=3 (com_mode)
		Script:           [][]uint16{{5, 3, 0}}, // op5, arg=3, end-sentinel
		ScriptComparator: []uint8{1},            // eq
		ScriptOperand:    []uint16{0},
	}

	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), varpPack, makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "script1op1=pushvar,com_mode") {
		t.Errorf("missing pushvar: %s", joined)
	}
	if !strings.Contains(joined, "script1=eq,0") {
		t.Errorf("missing comparator: %s", joined)
	}
}

// scriptOpFixture builds the standard two-component layer/child fixture with
// the given script array on the child and returns the joined export lines.
func scriptOpFixture(t *testing.T, script [][]uint16, varbitPack *pack.PackFile) string {
	t.Helper()
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:               1,
		RootLayer:        0,
		ComType:          TypeRect,
		Fill:             true,
		Script:           script,
		ScriptComparator: []uint8{1},
		ScriptOperand:    []uint16{0},
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), varbitPack, noRenameModel, nil, 0, 0, "")
	return strings.Join(lines, "\n")
}

// TestExport_ScriptOps14to20 pins the 254 script ops: push_varbit (14, varbit
// name resolution + varbit_<id> fallback), subtract/divide/multiply/coordx/
// coordz (15-19, bare), push_constant (20, literal operand).
//
// TS source: interface/Unpack.ts:530-555 @2e3bcf43.
func TestExport_ScriptOps14to20(t *testing.T) {
	t.Run("push_varbit_named", func(t *testing.T) {
		varbitPack := makeVarbitPack(t)
		varbitPack.Register(5, "quest_bits")
		varbitPack.RefreshNames()
		joined := scriptOpFixture(t, [][]uint16{{14, 5, 0}}, varbitPack)
		if !strings.Contains(joined, "script1op1=push_varbit,quest_bits") {
			t.Errorf("missing push_varbit: %s", joined)
		}
	})

	t.Run("push_varbit_fallback", func(t *testing.T) {
		joined := scriptOpFixture(t, [][]uint16{{14, 261, 0}}, makeVarbitPack(t))
		if !strings.Contains(joined, "script1op1=push_varbit,varbit_261") {
			t.Errorf("missing push_varbit fallback: %s", joined)
		}
	})

	t.Run("arith_and_coord_ops", func(t *testing.T) {
		joined := scriptOpFixture(t, [][]uint16{{15, 16, 17, 18, 19, 0}}, makeVarbitPack(t))
		for _, want := range []string{
			"script1op1=subtract",
			"script1op2=divide",
			"script1op3=multiply",
			"script1op4=coordx",
			"script1op5=coordz",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("missing %s: %s", want, joined)
			}
		}
	})

	t.Run("push_constant", func(t *testing.T) {
		joined := scriptOpFixture(t, [][]uint16{{20, 1234, 0}}, makeVarbitPack(t))
		if !strings.Contains(joined, "script1op1=push_constant,1234") {
			t.Errorf("missing push_constant: %s", joined)
		}
	})
}

// TestExport_ColourHexFormat verifies that colours are emitted as uppercase
// 6-digit hex with 0x prefix (e.g. 0xFF0000, not 0xff0000).
func TestExport_ColourHexFormat(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeText,
		Font:      0,
		FontSet:   true,
		Colour:    0x00C000, // should be 0x00C000
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "colour=0x00C000") {
		t.Errorf("missing uppercase hex colour: %s", joined)
	}
}

// TestFmtHex6_TSFaithful pins fmtHex6 against TS
// `toString(16).toUpperCase().padStart(6, '0')` (Unpack.ts:685 @2e3bcf43):
// NO 24-bit mask — values above 0xFFFFFF emit 7+ digits (the 254 cache
// carries colour=0xAAAAAAA in thormac.if), and negatives emit the JS
// '-'-prefixed absolute hex padded to width 6 including the sign.
func TestFmtHex6_TSFaithful(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0x00C000, "00C000"},
		{0xFFFFFF, "FFFFFF"},
		{0xAAAAAAA, "AAAAAAA"}, // 7 digits, unmasked (the old &0xFFFFFF bug gave AAAAAA)
		{0x1, "000001"},
		{-255, "000-FF"}, // JS: (-255).toString(16) = "-ff" → padStart(6,'0') = "000-ff" → upper
	}
	for _, tc := range cases {
		if got := fmtHex6(tc.in); got != tc.want {
			t.Errorf("fmtHex6(%#x): want %q got %q", tc.in, tc.want, got)
		}
	}
}

// TestExport_ActionTarget_BitfieldAll verifies all five bits of actionTarget.
func TestExport_ActionTarget_BitfieldAll(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:           1,
		RootLayer:    0,
		ComType:      TypeText,
		ButtonType:   ButtonTarget,
		ActionVerb:   "Cast",
		ActionTarget: 0x1F, // all bits: obj|npc|loc|player|heldobj
		Action:       "on",
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "actiontarget=obj,npc,loc,player,heldobj") {
		t.Errorf("wrong actiontarget: %s", joined)
	}
}

// TestExport_ChildLayerRecursion verifies that a sub-layer (non-root TYPE_LAYER
// component with children) emits the layer= field and recurses children with
// the sub-layer's comName as parent.
func TestExport_ChildLayerRecursion(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:sub")   // a named sub-layer
	ifPack.Register(2, "ui:com_1") // child of sub
	ifPack.RefreshNames()

	// Root (id=0) has one child: sub-layer (id=1)
	// Sub-layer (id=1) has one child: com_1 (id=2)
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{5},
		ChildY:    []int{10},
	}
	subLayer := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeLayer,
		Width:     100,
		Height:    50,
		ChildID:   []int{2},
		ChildX:    []int{3},
		ChildY:    []int{7},
	}
	grandchild := &Component{
		ID:        2,
		RootLayer: 0,
		ComType:   TypeRect,
		Fill:      true,
	}
	components := []*Component{root, subLayer, grandchild}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	// sub-layer header
	if !strings.Contains(joined, "[sub]") {
		t.Errorf("missing [sub]: %s", joined)
	}
	// grandchild must have layer=sub
	if !strings.Contains(joined, "layer=sub") {
		t.Errorf("missing layer=sub: %s", joined)
	}
	// grandchild at x=3, y=7 (from subLayer's ChildX/Y)
	if !strings.Contains(joined, "x=3") {
		t.Errorf("missing x=3: %s", joined)
	}
	if !strings.Contains(joined, "y=7") {
		t.Errorf("missing y=7: %s", joined)
	}
}

// -----------------------------------------------------------------------
// ExportSrc naming-pass unit tests
// -----------------------------------------------------------------------

// TestExportSrc_NamingPasses verifies the two-pass naming logic:
//   - roots get inter_N names (or keep existing non-inter_ names)
//   - children get parentName:com_N names
func TestExportSrc_NamingPasses(t *testing.T) {
	dir := t.TempDir()
	// Create minimal pack files.
	ifPack, _ := pack.NewPackFile(dir, "interface", nil)
	// Pre-name one root with a real name to verify it is not overwritten.
	ifPack.Register(0, "player_kit")
	ifPack.RefreshNames()

	modelPack, _ := pack.NewPackFile(dir, "model", nil)
	objPack, _ := pack.NewPackFile(dir, "obj", nil)
	seqPack, _ := pack.NewPackFile(dir, "seq", nil)
	varpPack, _ := pack.NewPackFile(dir, "varp", nil)

	// Build a minimal Decoded with two roots and one child each.
	// Root 0 (id=0) has child id=1.
	// Root 5 (id=5) has child id=6.
	comps := make([]*Component, 10)
	comps[0] = &Component{ID: 0, RootLayer: 0, ComType: TypeLayer, ChildID: []int{1}, ChildX: []int{0}, ChildY: []int{0}}
	comps[1] = &Component{ID: 1, RootLayer: 0, ComType: TypeRect}
	comps[5] = &Component{ID: 5, RootLayer: 5, ComType: TypeLayer, ChildID: []int{6}, ChildX: []int{0}, ChildY: []int{0}}
	comps[6] = &Component{ID: 6, RootLayer: 5, ComType: TypeRect}

	dec := &Decoded{
		Count:      7,
		Components: comps,
		Order:      []int{0, 1, 5, 6},
	}

	err := ExportSrc(dec, ifPack, modelPack, objPack, seqPack, varpPack, makeVarbitPack(t), dir, nil, func(src, dst string) error { return nil })
	if err != nil {
		t.Fatalf("ExportSrc: %v", err)
	}

	// Root 0 should keep "player_kit" (already has a non-inter_ name).
	if ifPack.GetByID(0) != "player_kit" {
		t.Errorf("root 0: got %q, want player_kit", ifPack.GetByID(0))
	}
	// Root 5 should get inter_1 (second unnamed root).
	if ifPack.GetByID(5) != "inter_1" {
		t.Errorf("root 5: got %q, want inter_1", ifPack.GetByID(5))
	}
	// Child 1 should be player_kit:com_0.
	if ifPack.GetByID(1) != "player_kit:com_0" {
		t.Errorf("child 1: got %q, want player_kit:com_0", ifPack.GetByID(1))
	}
	// Child 6 should be inter_1:com_0.
	if ifPack.GetByID(6) != "inter_1:com_0" {
		t.Errorf("child 6: got %q, want inter_1:com_0", ifPack.GetByID(6))
	}
}

// TestExportSrc_FatalCondition verifies that a child with a non-':'-containing
// name (i.e. a plain name without colon, not an inter_ prefix) returns an error.
func TestExportSrc_FatalCondition(t *testing.T) {
	dir := t.TempDir()
	ifPack, _ := pack.NewPackFile(dir, "interface", nil)
	// Register a child with a plain name (no colon) — this is the fatal condition.
	ifPack.Register(1, "somename")
	ifPack.RefreshNames()

	modelPack, _ := pack.NewPackFile(dir, "model", nil)
	objPack, _ := pack.NewPackFile(dir, "obj", nil)
	seqPack, _ := pack.NewPackFile(dir, "seq", nil)
	varpPack, _ := pack.NewPackFile(dir, "varp", nil)

	comps := make([]*Component, 2)
	comps[0] = &Component{ID: 0, RootLayer: 0, ComType: TypeLayer, ChildID: []int{1}, ChildX: []int{0}, ChildY: []int{0}}
	comps[1] = &Component{ID: 1, RootLayer: 0, ComType: TypeRect}

	dec := &Decoded{
		Count:      2,
		Components: comps,
		Order:      []int{0, 1},
	}

	err := ExportSrc(dec, ifPack, modelPack, objPack, seqPack, varpPack, makeVarbitPack(t), dir, nil, func(src, dst string) error { return nil })
	if err == nil {
		t.Error("expected fatal error for child with non-colon name, got nil")
	}
}

// -----------------------------------------------------------------------
// Hardening: cycle guard + out-of-range stat names
// -----------------------------------------------------------------------

// TestExportComponent_CycleGuard verifies that a two-node A→B→A cycle
// terminates without a stack overflow and still emits the non-cyclic output
// (B's header) that precedes the revisit.
func TestExportComponent_CycleGuard(t *testing.T) {
	ifPack := makeIfPack(t)
	// Root A: id=0, name="ui"
	ifPack.Register(0, "ui")
	// Child B: id=1, name="ui:com_0"
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	// A (root, TypeLayer) → child B (TypeLayer) → child A (cycle back to root)
	compA := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	compB := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{0}, // points back to A — creates A→B→A cycle
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{compA, compB}

	// Must terminate (no stack overflow).
	lines := exportComponent(compA, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	// B's header must be present (non-cyclic output was still emitted).
	if !strings.Contains(joined, "[com_0]") {
		t.Errorf("expected [com_0] in output before cycle skip:\n%s", joined)
	}
}

// TestExport_Trans_Emitted verifies that a non-zero Trans value emits
// "trans=N" positioned after "height=" and before "overlayer=" in the
// non-root header block.
//
// TS source: tools/unpack/interface/Unpack.ts:437-439 — if (this.trans) temp.push(`trans=${this.trans}`)
func TestExport_Trans_Emitted(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeRect,
		Width:     80,
		Height:    20,
		Trans:     128, // non-zero — must emit trans=128
		OverLayer: -1,  // -1 → no overlayer= emitted
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "trans=128") {
		t.Errorf("missing trans=128 in output:\n%s", joined)
	}

	// trans= must appear after height= and before overlayer= (which is absent here)
	heightIdx := strings.Index(joined, "height=")
	transIdx := strings.Index(joined, "trans=")
	if heightIdx < 0 {
		t.Fatalf("missing height= in output:\n%s", joined)
	}
	if transIdx < heightIdx {
		t.Errorf("trans= must appear after height=:\n%s", joined)
	}
}

// TestExport_Trans_Zero_Omitted verifies that Trans==0 emits no trans= line
// (TS mirrors JS truthiness: 0 is falsy).
//
// TS source: tools/unpack/interface/Unpack.ts:437 — if (this.trans)
func TestExport_Trans_Zero_Omitted(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeRect,
		Trans:     0, // zero — must NOT emit trans=
		OverLayer: -1,
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if strings.Contains(joined, "trans=") {
		t.Errorf("unexpected trans= in output when Trans==0:\n%s", joined)
	}
}

// TestExport_Trans_PositionedBeforeOverlayer verifies that trans= appears
// between height= and overlayer= when all three are present.
//
// TS source: tools/unpack/interface/Unpack.ts:433-443 — height, trans, overlayer order
func TestExport_Trans_PositionedBeforeOverlayer(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(0x0105, "ui:overlay") // overLayer = 261
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeRect,
		Height:    30,
		Trans:     64,
		OverLayer: 0x0105, // 261 — triggers overlayer= emission
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	transIdx := strings.Index(joined, "trans=")
	overlayerIdx := strings.Index(joined, "overlayer=")
	heightIdx := strings.Index(joined, "height=")

	if transIdx < 0 {
		t.Fatalf("missing trans= in output:\n%s", joined)
	}
	if overlayerIdx < 0 {
		t.Fatalf("missing overlayer= in output:\n%s", joined)
	}
	if transIdx < heightIdx {
		t.Errorf("trans= must appear after height=:\n%s", joined)
	}
	if transIdx > overlayerIdx {
		t.Errorf("trans= must appear before overlayer=:\n%s", joined)
	}
}

// TestExport_Swappable_Emitted verifies that Swappable=true emits "swappable=yes"
// after "usable=yes" in the TYPE_INV block.
//
// TS source: tools/unpack/interface/Unpack.ts:588-590 — if (this.swappable) temp.push('swappable=yes')
func TestExport_Swappable_Emitted(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeInv,
		Usable:    true,
		Swappable: true,
		OverLayer: -1,
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "swappable=yes") {
		t.Errorf("missing swappable=yes in output:\n%s", joined)
	}
	// swappable= must appear after usable=
	usableIdx := strings.Index(joined, "usable=yes")
	swappableIdx := strings.Index(joined, "swappable=yes")
	if usableIdx < 0 {
		t.Fatalf("missing usable=yes in output:\n%s", joined)
	}
	if swappableIdx < usableIdx {
		t.Errorf("swappable=yes must appear after usable=yes:\n%s", joined)
	}
}

// TestExport_Swappable_False_Omitted verifies that Swappable=false emits no
// swappable= line (TS mirrors JS truthiness: false is falsy).
//
// TS source: tools/unpack/interface/Unpack.ts:588 — if (this.swappable)
func TestExport_Swappable_False_Omitted(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeInv,
		Swappable: false,
		OverLayer: -1,
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if strings.Contains(joined, "swappable=") {
		t.Errorf("unexpected swappable= in output when Swappable==false:\n%s", joined)
	}
}

// TestExport_ActiveOverColour_Emitted verifies that a non-zero ActiveOverColour
// emits "activeovercolour=0xXXXXXX" after "overcolour=" in the TYPE_RECT/TYPE_TEXT
// colour block, using uppercase 6-digit hex via fmtHex6.
//
// TS source: tools/unpack/interface/Unpack.ts:673-675 — if (this.activeOverColour) temp.push(...)
func TestExport_ActiveOverColour_Emitted(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:               1,
		RootLayer:        0,
		ComType:          TypeRect,
		OverColour:       0xFF0000,
		ActiveOverColour: 0x00AABB,
		OverLayer:        -1,
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "activeovercolour=0x00AABB") {
		t.Errorf("missing activeovercolour=0x00AABB in output:\n%s", joined)
	}
	// activeovercolour= must appear after overcolour=
	overIdx := strings.Index(joined, "overcolour=")
	activeOverIdx := strings.Index(joined, "activeovercolour=")
	if overIdx < 0 {
		t.Fatalf("missing overcolour= in output:\n%s", joined)
	}
	if activeOverIdx < overIdx {
		t.Errorf("activeovercolour= must appear after overcolour=:\n%s", joined)
	}
}

// TestExport_ActiveOverColour_Zero_Omitted verifies that ActiveOverColour==0
// emits no activeovercolour= line (TS: if (this.activeOverColour) is falsy for 0).
//
// TS source: tools/unpack/interface/Unpack.ts:673 — if (this.activeOverColour)
func TestExport_ActiveOverColour_Zero_Omitted(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	child := &Component{
		ID:               1,
		RootLayer:        0,
		ComType:          TypeRect,
		ActiveOverColour: 0, // zero — must NOT emit
		OverLayer:        -1,
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	if strings.Contains(joined, "activeovercolour=") {
		t.Errorf("unexpected activeovercolour= in output when ActiveOverColour==0:\n%s", joined)
	}
}

// TestExportComponent_StatNameOutOfRange verifies that a script op with a
// stat index ≥ 21 emits "undefined" rather than panicking, faithfully
// mirroring JS's STATS[stat] → undefined stringification.
func TestExportComponent_StatNameOutOfRange(t *testing.T) {
	ifPack := makeIfPack(t)
	ifPack.Register(0, "ui")
	ifPack.Register(1, "ui:com_0")
	ifPack.RefreshNames()

	// Script: op=1 (stat_level), stat=99 (out of range), end sentinel at index 2.
	// The loop runs while j < len(sc)-1 = 2, so j=0: op=sc[0]=1, popStack()→sc[1]=99.
	child := &Component{
		ID:        1,
		RootLayer: 0,
		ComType:   TypeRect,
		Script:    [][]uint16{{1, 99, 0}}, // op1, stat=99, sentinel
	}
	root := &Component{
		ID:        0,
		RootLayer: 0,
		ComType:   TypeLayer,
		ChildID:   []int{1},
		ChildX:    []int{0},
		ChildY:    []int{0},
	}
	components := []*Component{root, child}

	// Must not panic.
	lines := exportComponent(root, components, ifPack, makeObjPack(t), makeSeqPack(t), makeVarpPack(t), makeVarbitPack(t), noRenameModel, nil, 0, 0, "")
	joined := strings.Join(lines, "\n")

	// TS STATS[99] → undefined → string coercion "undefined".
	want := "stat_level,undefined"
	if !strings.Contains(joined, want) {
		t.Errorf("expected %q in output for out-of-range stat:\n%s", want, joined)
	}
}
