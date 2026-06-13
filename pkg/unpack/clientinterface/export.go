// Package clientinterface — export.go implements the naming passes, .if
// emission, and renameModel helper that mirror TS Unpack.ts lines 11–35,
// 290–357, and 359–807.
package clientinterface

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/pack"
)

// stats is the STATS constant array from TS Unpack.ts:57-79.
var stats = [...]string{
	"attack",
	"defence",
	"strength",
	"hitpoints",
	"ranged",
	"prayer",
	"magic",
	"cooking",
	"woodcutting",
	"fletching",
	"fishing",
	"firemaking",
	"crafting",
	"smithing",
	"mining",
	"herblore",
	"agility",
	"thieving",
	"slayer",
	"farming",
	"runecraft",
}

// statName returns the stat name for the given index, or "undefined" when the
// index is out of range. This mirrors JS's STATS[stat] returning undefined (and
// its coercion to the string "undefined") when stat is out of bounds.
func statName(stat int) string {
	if stat >= 0 && stat < len(stats) {
		return stats[stat]
	}
	return "undefined"
}

// renameModelFull is the fs-side wrapper used at run time. It performs the
// filesystem rename (if the file is found) and then updates the pack, returning
// the stable name. Mirrors TS Unpack.ts:11-35 fully.
//
// TS source: tools/unpack/interface/Unpack.ts:11-35.
func renameModelFull(
	id int,
	modelPack *pack.PackFile,
	srcDir string,
	existingFiles []string,
	errorf func(format string, args ...any),
	renameFn func(src, dst string) error,
) string {
	model := modelPack.GetByID(id)
	if !strings.HasPrefix(model, "model_") {
		return model
	}

	// Collision loop — TS Unpack.ts:16-21.
	name := "com_i1"
	i := 2
	for modelPack.GetByName(name) != -1 {
		name = fmt.Sprintf("com_i%d", i)
		i++
	}

	// Filesystem rename — TS Unpack.ts:23-28.
	filePath := ""
	for _, f := range existingFiles {
		if strings.HasSuffix(f, "/"+model+".ob2") {
			filePath = f
			break
		}
	}
	if filePath != "" {
		dest := filepath.Join(srcDir, "models", "com", name+".ob2")
		if err := renameFn(filePath, dest); err != nil && errorf != nil {
			errorf("Model not found on filesystem com %s", model) // TS line 27
		}
	} else {
		if errorf != nil {
			errorf("Model not found on filesystem com %s", model) // TS line 27
		}
	}

	// TS line 30-31: model = name; ModelPack.register(id, model)
	modelPack.Register(id, name)

	return name
}

// ExportOrder writes pack/interface.order, one component ID per line, newline-
// terminated. Mirrors TS IfType.exportOrder (Unpack.ts:290-292).
//
// TS: fs.writeFileSync(..., IfType.order.join('\n') + '\n')
// This means: IDs joined by '\n', then a trailing '\n' appended.
// For an empty order the result is "\n" (join([]) == "" then + '\n').
func ExportOrder(dec *Decoded, destPath string) error {
	ids := make([]string, len(dec.Order))
	for i, id := range dec.Order {
		ids[i] = fmt.Sprintf("%d", id)
	}
	content := strings.Join(ids, "\n") + "\n"
	return writeFileMkdir(destPath, content)
}

// ExportSrc runs the naming passes and writes all .if files, mirroring TS
// IfType.exportSrc (Unpack.ts:294-357).
//
// Parameters:
//   - dec: decoded interface data
//   - interfacePack: registry for interface names (EnsureInterface result)
//   - modelPack: registry for model names (EnsureModel result)
//   - objPack: registry for obj names (EnsureObj result)
//   - seqPack: registry for seq names (EnsureSeq result)
//   - varpPack: registry for varp names (EnsureVarp result)
//   - varbitPack: registry for varbit names (EnsureVarbit result; NEW at 254
//     for the script op 14 push_varbit resolution)
//   - srcDir: content tree root (BUILD_SRC_DIR)
//   - errorf: console.error sink (nil = no-op)
//   - renameFn: filesystem rename hook (nil = real os.Rename)
//
// Returns an error if a fatal naming condition is detected (TS printFatalError).
//
// TS source: tools/unpack/interface/Unpack.ts:294-357.
func ExportSrc(
	dec *Decoded,
	interfacePack *pack.PackFile,
	modelPack *pack.PackFile,
	objPack *pack.PackFile,
	seqPack *pack.PackFile,
	varpPack *pack.PackFile,
	varbitPack *pack.PackFile,
	srcDir string,
	errorf func(format string, args ...any),
	renameFn func(src, dst string) error,
) error {
	if errorf == nil {
		errorf = func(string, ...any) {}
	}
	if renameFn == nil {
		renameFn = defaultRename
	}

	// TS lines 296-313: pass 1 — name roots.
	// ifId is the sequential counter for inter_N names.
	// comCount[rootID] = how many children have been named for that root.
	ifID := 0
	comCount := make(map[int]int)

	for id := range dec.Count {
		com := dec.Components[id]
		if com == nil {
			continue
		}
		if com.ID != com.RootLayer {
			continue
		}
		// TS: const name = InterfacePack.getById(com.id)
		// if (!name || name.startsWith('inter_')) { InterfacePack.register(com.id, `inter_${ifId}`) }
		name := interfacePack.GetByID(com.ID)
		if name == "" || strings.HasPrefix(name, "inter_") {
			interfacePack.Register(com.ID, fmt.Sprintf("inter_%d", ifID))
		}
		ifID++
		comCount[com.ID] = 0
	}

	// TS lines 316-331: pass 2 — name children in Order sequence.
	for _, id := range dec.Order {
		com := dec.Components[id]
		if com == nil || com.ID == com.RootLayer {
			continue
		}

		name := interfacePack.GetByID(com.ID)
		// split on ':' to get optional comName part
		var comName string
		if idx := strings.IndexByte(name, ':'); idx >= 0 {
			comName = name[idx+1:]
		}

		// TS line 325: if (name && typeof comName === 'undefined') { printFatalError(...) }
		// Go: name != "" && comName == "" after split means no ':' — fatal.
		if name != "" && strings.IndexByte(name, ':') < 0 {
			return fmt.Errorf("Issue with component %d must be manually resolved", com.ID)
		}

		// TS line 328: if (!name || (typeof comName !== 'undefined' && comName.startsWith('com_'))) { ... }
		if name == "" || (comName != "" && strings.HasPrefix(comName, "com_")) {
			parentName := interfacePack.GetByID(com.RootLayer)
			interfacePack.Register(com.ID, fmt.Sprintf("%s:com_%d", parentName, comCount[com.RootLayer]))
		}
		comCount[com.RootLayer]++
	}

	// TS line 335: InterfacePack.save()
	if err := interfacePack.Save(); err != nil {
		return fmt.Errorf("save interface pack: %w", err)
	}

	// TS lines 337-340: mkdir scripts/interfaces
	ifDir := filepath.Join(srcDir, "scripts", "interfaces")
	if err := mkdirAll(ifDir); err != nil {
		return fmt.Errorf("mkdir scripts/interfaces: %w", err)
	}

	// TS line 342: existingFiles for .if search
	existingIFFiles := pack.ListFilesExt(filepath.Join(srcDir, "scripts"), ".if")

	// Pre-scan all .ob2 model files once — TS Unpack.ts:12 calls listFilesExt
	// inside renameModel which runs once per model reference. We pre-build the
	// slice and pass it into the export context.
	existingModelFiles := pack.ListFilesExt(filepath.Join(srcDir, "models"), ".ob2")

	// Build a renameModelFn closure with the context bound.
	rmFn := func(id int) string {
		return renameModelFull(id, modelPack, srcDir, existingModelFiles, errorf, renameFn)
	}

	// TS lines 344-356: for each root, export and write.
	for id := range dec.Count {
		com := dec.Components[id]
		if com == nil || com.ID != com.RootLayer {
			continue
		}

		name := interfacePack.GetByID(com.ID)
		src := exportComponent(com, dec.Components, interfacePack, objPack, seqPack, varpPack, varbitPack, rmFn, nil, 0, 0, "")

		// TS lines 353-355: find existing .if or fall back to scripts/interfaces/<name>.if
		destFile := ""
		for _, f := range existingIFFiles {
			if strings.HasSuffix(f, "/"+name+".if") {
				destFile = f
				break
			}
		}
		if destFile == "" {
			destFile = filepath.Join(srcDir, "scripts", "interfaces", name+".if")
		}

		content := strings.Join(src, "\n") + "\n"
		if err := writeFileMkdir(destFile, content); err != nil {
			return fmt.Errorf("write %s: %w", destFile, err)
		}
	}

	return nil
}

// exportComponent emits the .if lines for com and (if TYPE_LAYER) its
// children, mirroring TS IfType.export (Unpack.ts:359-807).
//
// Parameters:
//   - com: the component to emit
//   - components: full sparse slice (for child lookup)
//   - ifPack, objPack, seqPack, varpPack: name registries
//   - rmFn: renameModel function (id → stable model name)
//   - temp: existing accumulation (for recursion); pass nil for top call
//   - x, y: parent-supplied child coordinates (0,0 for roots)
//   - parent: parent's comName part (empty for top-level, comName for sub-layers)
//
// TS source: tools/unpack/interface/Unpack.ts:359-807.
//
// Cycle guard: TS has the same gap (cyclic child references would also overflow
// the TS call stack). A visited set is threaded internally so Go does not panic;
// on a revisit the child is silently skipped (same observable output as TS would
// produce before the non-cyclic children, just without the stack overflow).
func exportComponent(
	com *Component,
	components []*Component,
	ifPack *pack.PackFile,
	objPack *pack.PackFile,
	seqPack *pack.PackFile,
	varpPack *pack.PackFile,
	varbitPack *pack.PackFile,
	rmFn func(int) string,
	temp []string,
	x, y int,
	parent string,
) []string {
	visited := make(map[int]struct{})
	return exportComponentInner(com, components, ifPack, objPack, seqPack, varpPack, varbitPack, rmFn, temp, x, y, parent, visited)
}

func exportComponentInner(
	com *Component,
	components []*Component,
	ifPack *pack.PackFile,
	objPack *pack.PackFile,
	seqPack *pack.PackFile,
	varpPack *pack.PackFile,
	varbitPack *pack.PackFile,
	rmFn func(int) string,
	temp []string,
	x, y int,
	parent string,
	visited map[int]struct{},
) []string {
	if temp == nil {
		temp = []string{}
	}

	// Cycle guard: mark this component as in-progress; if already visited,
	// skip to prevent infinite recursion (same posture as pkg/unpack/config
	// loc.go unknown-opcode bail).
	if _, seen := visited[com.ID]; seen {
		return temp
	}
	visited[com.ID] = struct{}{}

	comName := ifPack.GetByID(com.ID)

	// TS lines 361-438: non-root header block.
	if com.ID != com.RootLayer {
		// TS line 363: temp.push(`[${comName.split(':')[1]}]`)
		comNamePart := comName
		if idx := strings.IndexByte(comName, ':'); idx >= 0 {
			comNamePart = comName[idx+1:]
		}
		temp = append(temp, fmt.Sprintf("[%s]", comNamePart))

		if parent != "" {
			temp = append(temp, fmt.Sprintf("layer=%s", parent))
		}

		// type= based on comType
		switch com.ComType {
		case TypeLayer:
			temp = append(temp, "type=layer")
		case TypeInv:
			temp = append(temp, "type=inv")
		case TypeRect:
			temp = append(temp, "type=rect")
		case TypeText:
			temp = append(temp, "type=text")
		case TypeGraphic:
			temp = append(temp, "type=graphic")
		case TypeModel:
			temp = append(temp, "type=model")
		case TypeInvText:
			temp = append(temp, "type=invtext")
		case 8:
			temp = append(temp, "type=8")
		}

		temp = append(temp, fmt.Sprintf("x=%d", x))
		temp = append(temp, fmt.Sprintf("y=%d", y))

		// buttontype=
		switch com.ButtonType {
		case ButtonOK:
			temp = append(temp, "buttontype=normal")
		case ButtonTarget:
			temp = append(temp, "buttontype=target")
		case ButtonClose:
			temp = append(temp, "buttontype=close")
		case ButtonToggle:
			temp = append(temp, "buttontype=toggle")
		case ButtonSelect:
			temp = append(temp, "buttontype=select")
		case ButtonContinue:
			temp = append(temp, "buttontype=pause")
		}

		if com.ClientCode != 0 {
			temp = append(temp, fmt.Sprintf("clientcode=%d", com.ClientCode))
		}
		if com.Width != 0 {
			temp = append(temp, fmt.Sprintf("width=%d", com.Width))
		}
		if com.Height != 0 {
			temp = append(temp, fmt.Sprintf("height=%d", com.Height))
		}
		if com.Trans != 0 { // TS Unpack.ts:437-439
			temp = append(temp, fmt.Sprintf("trans=%d", com.Trans))
		}

		// TS line 435: if (this.overLayer !== -1)
		if com.OverLayer != -1 {
			ol := ifPack.GetByID(com.OverLayer)
			if idx := strings.IndexByte(ol, ':'); idx >= 0 {
				ol = ol[idx+1:]
			}
			temp = append(temp, fmt.Sprintf("overlayer=%s", ol))
		}
	}

	// TS lines 440-532: script emissions.
	if com.Script != nil {
		for i, sc := range com.Script {
			if sc == nil {
				continue
			}
			opcount := 1

			if len(sc) == 1 {
				// TS line 450: empty script.
				// NOTE: TS uses `script${i + i}op1=` — i+i, NOT i+1 (the
				// non-empty branch at TS:454 uses i+1). Verbatim TS quirk;
				// do not "fix".
				temp = append(temp, fmt.Sprintf("script%dop1=", i+i))
			}

			j := 0
			for j < len(sc)-1 {
				str := fmt.Sprintf("script%dop%d=", i+1, opcount)
				opcount++

				popStack := func() uint16 {
					j++
					if j < len(sc) {
						return sc[j]
					}
					return 0
				}

				op := sc[j]
				switch op {
				case 1:
					stat := int(popStack())
					str += fmt.Sprintf("stat_level,%s", statName(stat))
				case 2:
					stat := int(popStack())
					str += fmt.Sprintf("stat_base_level,%s", statName(stat))
				case 3:
					stat := int(popStack())
					str += fmt.Sprintf("stat_xp,%s", statName(stat))
				case 4:
					inv := int(popStack())
					obj := int(popStack())
					invName := ifPack.GetByID(inv)
					if invName == "" {
						invName = fmt.Sprintf("%d", inv)
					}
					objName := objPack.GetByID(obj)
					if objName == "" {
						objName = fmt.Sprintf("obj_%d", obj)
					}
					str += fmt.Sprintf("inv_count,%s,%s", invName, objName)
				case 5:
					varp := int(popStack())
					varpName := varpPack.GetByID(varp)
					if varpName == "" {
						varpName = fmt.Sprintf("varp_%d", varp)
					}
					str += fmt.Sprintf("pushvar,%s", varpName)
				case 6:
					stat := int(popStack())
					str += fmt.Sprintf("stat_xp_remaining,%s", statName(stat))
				case 7:
					str += "op7"
				case 8:
					str += "op8"
				case 9:
					str += "op9"
				case 10:
					inv := int(popStack())
					obj := int(popStack())
					invName := ifPack.GetByID(inv)
					if invName == "" {
						invName = fmt.Sprintf("%d", inv)
					}
					objName := objPack.GetByID(obj)
					if objName == "" {
						objName = fmt.Sprintf("obj_%d", obj)
					}
					str += fmt.Sprintf("inv_contains,%s,%s", invName, objName)
				case 11:
					str += "runenergy"
				case 12:
					str += "runweight"
				case 13:
					varp := int(popStack())
					bit := int(popStack())
					varpName := varpPack.GetByID(varp)
					if varpName == "" {
						varpName = fmt.Sprintf("varp_%d", varp)
					}
					str += fmt.Sprintf("testbit,%s,%d", varpName, bit)
				case 14:
					// TS Unpack.ts:530-534 @2e3bcf43: push_varbit with the
					// VarbitPack name (varbit_<id> fallback).
					varbit := int(popStack())
					varbitName := varbitPack.GetByID(varbit)
					if varbitName == "" {
						varbitName = fmt.Sprintf("varbit_%d", varbit)
					}
					str += fmt.Sprintf("push_varbit,%s", varbitName)
				case 15:
					str += "subtract" // TS Unpack.ts:535-537
				case 16:
					str += "divide" // TS Unpack.ts:538-540
				case 17:
					str += "multiply" // TS Unpack.ts:541-543
				case 18:
					str += "coordx" // TS Unpack.ts:544-546
				case 19:
					str += "coordz" // TS Unpack.ts:547-549
				case 20:
					// TS Unpack.ts:550-554 @2e3bcf43: push_constant with the
					// literal operand value.
					value := popStack()
					str += fmt.Sprintf("push_constant,%d", value)
				}

				temp = append(temp, str)
				j++
			}
		}
	}

	// TS lines 534-557: script comparators.
	if com.ScriptComparator != nil && com.ScriptOperand != nil {
		for i, cmp := range com.ScriptComparator {
			str := fmt.Sprintf("script%d=", i+1)
			switch cmp {
			case 1:
				str += "eq"
			case 2:
				str += "lt"
			case 3:
				str += "gt"
			case 4:
				str += "neq"
			}
			str += fmt.Sprintf(",%d", com.ScriptOperand[i])
			temp = append(temp, str)
		}
	}

	// TS lines 559-567: TYPE_LAYER fields.
	if com.ComType == TypeLayer {
		if com.Scroll != 0 {
			temp = append(temp, fmt.Sprintf("scroll=%d", com.Scroll))
		}
		if com.Hide {
			temp = append(temp, "hide=yes")
		}
	}

	// TS lines 569-604: TYPE_INV fields.
	if com.ComType == TypeInv {
		if com.Draggable {
			temp = append(temp, "draggable=yes")
		}
		if com.Interactable {
			temp = append(temp, "interactable=yes")
		}
		if com.Usable {
			temp = append(temp, "usable=yes")
		}
		if com.Swappable { // TS Unpack.ts:588-590
			temp = append(temp, "swappable=yes")
		}
		if com.MarginX != 0 || com.MarginY != 0 {
			temp = append(temp, fmt.Sprintf("margin=%d,%d", com.MarginX, com.MarginY))
		}
		if com.InvSlotSprite != nil && com.InvSlotOffsetX != nil && com.InvSlotOffsetY != nil {
			for i := range 20 {
				if com.InvSlotSprite[i] != nil {
					if com.InvSlotOffsetX[i] != 0 || com.InvSlotOffsetY[i] != 0 {
						temp = append(temp, fmt.Sprintf("slot%d=%s:%d,%d", i+1, *com.InvSlotSprite[i], com.InvSlotOffsetX[i], com.InvSlotOffsetY[i]))
					} else {
						temp = append(temp, fmt.Sprintf("slot%d=%s", i+1, *com.InvSlotSprite[i]))
					}
				}
			}
		}
		if com.Iops != nil {
			for i, iop := range com.Iops {
				if iop != nil {
					temp = append(temp, fmt.Sprintf("option%d=%s", i+1, *iop))
				}
			}
		}
	}

	// TS lines 607-611: TYPE_RECT fill.
	if com.ComType == TypeRect {
		if com.Fill {
			temp = append(temp, "fill=yes")
		}
	}

	// TS lines 613-636: TYPE_TEXT font/center/shadowed.
	if com.ComType == TypeText {
		if com.Center {
			temp = append(temp, "center=yes")
		}
		if com.FontSet {
			switch com.Font {
			case 0:
				temp = append(temp, "font=p11_full")
			case 1:
				temp = append(temp, "font=p12_full")
			case 2:
				temp = append(temp, "font=b12_full")
			case 3:
				temp = append(temp, "font=q8_full")
			}
		}
		if com.Shadowed {
			temp = append(temp, "shadowed=yes")
		}
	}

	// TS lines 638-646: TYPE_TEXT text/activetext strings.
	if com.ComType == TypeText {
		if com.Text != "" {
			temp = append(temp, fmt.Sprintf("text=%s", com.Text))
		}
		if com.ActiveText != "" {
			temp = append(temp, fmt.Sprintf("activetext=%s", com.ActiveText))
		}
	}

	// TS lines 648-662: colour for TYPE_RECT | TYPE_TEXT.
	if com.ComType == TypeRect || com.ComType == TypeText {
		if com.Colour != 0 {
			temp = append(temp, fmt.Sprintf("colour=0x%s", fmtHex6(com.Colour)))
		}
	}

	// TS lines 654-662: activeColour / overColour for TYPE_RECT | TYPE_TEXT.
	if com.ComType == TypeRect || com.ComType == TypeText {
		if com.ActiveColour != 0 {
			temp = append(temp, fmt.Sprintf("activecolour=0x%s", fmtHex6(com.ActiveColour)))
		}
		if com.OverColour != 0 {
			temp = append(temp, fmt.Sprintf("overcolour=0x%s", fmtHex6(com.OverColour)))
		}
		if com.ActiveOverColour != 0 { // TS Unpack.ts:673-675
			temp = append(temp, fmt.Sprintf("activeovercolour=0x%s", fmtHex6(com.ActiveOverColour)))
		}
	}

	// TS lines 664-672: TYPE_GRAPHIC.
	if com.ComType == TypeGraphic {
		if com.Graphic != "" {
			temp = append(temp, fmt.Sprintf("graphic=%s", com.Graphic))
		}
		if com.ActiveGraphic != "" {
			temp = append(temp, fmt.Sprintf("activegraphic=%s", com.ActiveGraphic))
		}
	}

	// TS lines 674-702: TYPE_MODEL.
	if com.ComType == TypeModel {
		if com.Model != 0 {
			temp = append(temp, fmt.Sprintf("model=%s", rmFn(com.Model)))
		}
		if com.ActiveModel != 0 {
			temp = append(temp, fmt.Sprintf("activemodel=%s", rmFn(com.ActiveModel)))
		}
		if com.Anim != -1 {
			animName := seqPack.GetByID(com.Anim)
			if animName == "" {
				animName = fmt.Sprintf("seq_%d", com.Anim)
			}
			temp = append(temp, fmt.Sprintf("anim=%s", animName))
		}
		if com.ActiveAnim != -1 {
			animName := seqPack.GetByID(com.ActiveAnim)
			if animName == "" {
				animName = fmt.Sprintf("seq_%d", com.ActiveAnim)
			}
			temp = append(temp, fmt.Sprintf("activeanim=%s", animName))
		}
		if com.Zoom != 0 {
			temp = append(temp, fmt.Sprintf("zoom=%d", com.Zoom))
		}
		if com.Xan != 0 {
			temp = append(temp, fmt.Sprintf("xan=%d", com.Xan))
		}
		if com.Yan != 0 {
			temp = append(temp, fmt.Sprintf("yan=%d", com.Yan))
		}
	}

	// TS lines 704-754: TYPE_INV_TEXT.
	if com.ComType == TypeInvText {
		if com.Center {
			temp = append(temp, "center=yes")
		}
		if com.FontSet {
			switch com.Font {
			case 0:
				temp = append(temp, "font=p11_full")
			case 1:
				temp = append(temp, "font=p12_full")
			case 2:
				temp = append(temp, "font=b12_full")
			case 3:
				temp = append(temp, "font=q8_full")
			}
		}
		if com.Shadowed {
			temp = append(temp, "shadowed=yes")
		}
		if com.Colour != 0 {
			temp = append(temp, fmt.Sprintf("colour=0x%s", fmtHex6(com.Colour)))
		}
		if com.MarginX != 0 || com.MarginY != 0 {
			temp = append(temp, fmt.Sprintf("margin=%d,%d", com.MarginX, com.MarginY))
		}
		if com.InvSlotSprite != nil && com.InvSlotOffsetX != nil && com.InvSlotOffsetY != nil {
			for i := range 20 {
				if com.InvSlotSprite[i] != nil {
					if com.InvSlotOffsetX[i] != 0 || com.InvSlotOffsetY[i] != 0 {
						temp = append(temp, fmt.Sprintf("slot%d=%s:%d,%d", i+1, *com.InvSlotSprite[i], com.InvSlotOffsetX[i], com.InvSlotOffsetY[i]))
					} else {
						temp = append(temp, fmt.Sprintf("slot%d=%s", i+1, *com.InvSlotSprite[i]))
					}
				}
			}
		}
		if com.Iops != nil {
			for i, iop := range com.Iops {
				if iop != nil {
					temp = append(temp, fmt.Sprintf("option%d=%s", i+1, *iop))
				}
			}
		}
	}

	// TS lines 757-786: BUTTON_TARGET or TYPE_INV action fields.
	if com.ButtonType == ButtonTarget || com.ComType == TypeInv {
		if com.ActionVerb != "" {
			temp = append(temp, fmt.Sprintf("actionverb=%s", com.ActionVerb))
		}
		if com.ActionTarget != 0 {
			// ActionTarget is a bitfield; TS Unpack.ts:763-781.
			var targets []string
			if com.ActionTarget&0x1 != 0 {
				targets = append(targets, "obj")
			}
			if com.ActionTarget&0x2 != 0 {
				targets = append(targets, "npc")
			}
			if com.ActionTarget&0x4 != 0 {
				targets = append(targets, "loc")
			}
			if com.ActionTarget&0x8 != 0 {
				targets = append(targets, "player")
			}
			if com.ActionTarget&0x10 != 0 {
				targets = append(targets, "heldobj")
			}
			temp = append(temp, fmt.Sprintf("actiontarget=%s", strings.Join(targets, ",")))
		}
		if com.Action != "" {
			temp = append(temp, fmt.Sprintf("action=%s", com.Action))
		}
	}

	// TS lines 788-792: BUTTON_OK/TOGGLE/SELECT/CONTINUE option.
	if com.ButtonType == ButtonOK || com.ButtonType == ButtonToggle ||
		com.ButtonType == ButtonSelect || com.ButtonType == ButtonContinue {
		if com.Option != "" {
			temp = append(temp, fmt.Sprintf("option=%s", com.Option))
		}
	}

	// TS lines 794-806: TYPE_LAYER children recursion.
	if com.ComType == TypeLayer && com.ChildID != nil && com.ChildX != nil && com.ChildY != nil {
		for i, childID := range com.ChildID {
			// TS line 797-798: insert empty line between children, and before
			// the first child if this is not the root layer.
			if com.ID != com.RootLayer || i > 0 {
				temp = append(temp, "")
			}

			child := components[childID]
			if child == nil {
				continue
			}

			// TS line 801: parentName for the layer
			parentName := ""
			if com.ID != com.RootLayer {
				// comName is already the "parent:child" form; extract child part
				pn := ifPack.GetByID(com.ID)
				if idx := strings.IndexByte(pn, ':'); idx >= 0 {
					parentName = pn[idx+1:]
				} else {
					parentName = pn
				}
			}

			temp = exportComponentInner(child, components, ifPack, objPack, seqPack, varpPack, varbitPack, rmFn, temp, com.ChildX[i], com.ChildY[i], parentName, visited)
		}
	}

	return temp
}

// fmtHex6 formats a colour value mirroring TS
// `toString(16).toUpperCase().padStart(6, '0')` EXACTLY (Unpack.ts:685
// @2e3bcf43): JS does NOT mask to 24 bits, so values above 0xFFFFFF emit
// 7-8 digits unpadded (the 254 cache contains colour=0xAAAAAAA), and a
// negative value emits its '-'-prefixed absolute hex, zero-padded to width
// 6 including the sign (padStart pads the whole string).
//
// The former 24-bit mask was a Go deviation caught by the 254 interface
// parity gate.
func fmtHex6(colour int) string {
	s := strings.ToUpper(strconv.FormatInt(int64(colour), 16))
	for len(s) < 6 {
		s = "0" + s
	}
	return s
}
