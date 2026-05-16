// pkg/pack/compiler/type/scriptvar.go
package typ

// ScriptVarType represents a runtime-loaded named type
// (loc/npc/obj/enum/struct/etc.) registered at compiler-setup time. Distinct
// from PrimitiveType because ScriptVarType entries always have
// BaseVarType.Integer + defaultValue -1 + mutable options, and are not
// part of the bootstrap primitive set.
//
// Mirrors TS src/runescript/type/ScriptVarType.ts. Type-wise indistinguishable
// from a PrimitiveType registered with the same parameters.
type ScriptVarType struct {
	rep     string
	code    string
	codeOK  bool
	options TypeOptions
}

func newScriptVarType(name, code string) *ScriptVarType {
	return &ScriptVarType{
		rep:     name,
		code:    code,
		codeOK:  code != "",
		options: NewTypeOptions(),
	}
}

func (s *ScriptVarType) Representation() string        { return s.rep }
func (s *ScriptVarType) Code() (string, bool)          { return s.code, s.codeOK }
func (s *ScriptVarType) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }
func (s *ScriptVarType) DefaultValue() any             { return -1 }
func (s *ScriptVarType) Options() TypeOptions          { return s.options }
func (s *ScriptVarType) AsTypeRef()                    {}

// Singletons. Names + codes match TS ScriptVarType.ts L25-83.
// (CATEGORY is PrimitiveCategory, ported in T5 — not in this slice.)
var (
	ScriptVarSeq               = newScriptVarType("seq", "A")
	ScriptVarLocShape          = newScriptVarType("locshape", "H")
	ScriptVarComponent         = newScriptVarType("component", "I")
	ScriptVarIdKit             = newScriptVarType("idkit", "K")
	ScriptVarMidi              = newScriptVarType("midi", "M")
	ScriptVarNpcMode           = newScriptVarType("npc_mode", "N")
	ScriptVarNamedObj          = newScriptVarType("namedobj", "O")
	ScriptVarSynth             = newScriptVarType("synth", "P")
	ScriptVarArea              = newScriptVarType("area", "R")
	ScriptVarStat              = newScriptVarType("stat", "S")
	ScriptVarNpcStat           = newScriptVarType("npc_stat", "T")
	ScriptVarWriteInv          = newScriptVarType("writeinv", "V")
	ScriptVarMapArea           = newScriptVarType("wma", "`")
	ScriptVarGraphic           = newScriptVarType("graphic", "d")
	ScriptVarFontMetrics       = newScriptVarType("fontmetrics", "f")
	ScriptVarEnum              = newScriptVarType("enum", "g")
	ScriptVarHunt              = newScriptVarType("hunt", "h")
	ScriptVarJingle            = newScriptVarType("jingle", "j")
	ScriptVarLoc               = newScriptVarType("loc", "l")
	ScriptVarModel             = newScriptVarType("model", "m")
	ScriptVarNpc               = newScriptVarType("npc", "n")
	ScriptVarObj               = newScriptVarType("obj", "o")
	ScriptVarPlayerUID         = newScriptVarType("player_uid", "p")
	ScriptVarSpotAnim          = newScriptVarType("spotanim", "t")
	ScriptVarNpcUID            = newScriptVarType("npc_uid", "u")
	ScriptVarInv               = newScriptVarType("inv", "v")
	ScriptVarTexture           = newScriptVarType("texture", "x")
	ScriptVarMapElement        = newScriptVarType("mapelement", "µ")
	ScriptVarHitmark           = newScriptVarType("hitmark", "×")
	ScriptVarStruct            = newScriptVarType("struct", "J")
	ScriptVarDbRow             = newScriptVarType("dbrow", "Ð")
	ScriptVarInterface         = newScriptVarType("interface", "a")
	ScriptVarTopLevelInterface = newScriptVarType("toplevelinterface", "F")
	ScriptVarOverlayInterface  = newScriptVarType("overlayinterface", "L")
	ScriptVarMoveSpeed         = newScriptVarType("movespeed", "Ý")
	ScriptVarEntityOverlay     = newScriptVarType("entityoverlay", "-")
	ScriptVarDbTable           = newScriptVarType("dbtable", "Ø")
	ScriptVarStringVector      = newScriptVarType("stringvector", "¸")
	ScriptVarMesAnim           = newScriptVarType("mesanim", "Á")
	ScriptVarVerifyObject      = newScriptVarType("verifyobj", "®")
)

// ScriptVarTypeAll preserves TS ALL push order (declaration order at L25-83).
var ScriptVarTypeAll = []*ScriptVarType{
	ScriptVarSeq, ScriptVarLocShape, ScriptVarComponent, ScriptVarIdKit,
	ScriptVarMidi, ScriptVarNpcMode, ScriptVarNamedObj, ScriptVarSynth,
	ScriptVarArea, ScriptVarStat, ScriptVarNpcStat, ScriptVarWriteInv,
	ScriptVarMapArea, ScriptVarGraphic, ScriptVarFontMetrics, ScriptVarEnum,
	ScriptVarHunt, ScriptVarJingle, ScriptVarLoc, ScriptVarModel,
	ScriptVarNpc, ScriptVarObj, ScriptVarPlayerUID, ScriptVarSpotAnim,
	ScriptVarNpcUID, ScriptVarInv, ScriptVarTexture,
	ScriptVarMapElement, ScriptVarHitmark, ScriptVarStruct, ScriptVarDbRow,
	ScriptVarInterface, ScriptVarTopLevelInterface, ScriptVarOverlayInterface,
	ScriptVarMoveSpeed, ScriptVarEntityOverlay, ScriptVarDbTable,
	ScriptVarStringVector, ScriptVarMesAnim, ScriptVarVerifyObject,
}
