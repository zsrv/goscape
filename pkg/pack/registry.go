package pack

// Registry holds the *PackFile singletons that PackConfigs builds
// while packing. Client stages (clientinterface, sprites, audio,
// graphics) read from it after PackConfigs returns.
//
// Each EnsureX accessor lazily constructs on first call and memoizes.
// Field names match the TS singleton names (InterfacePack → Interface,
// MapPack → Map, AnimSetPack → AnimSet, MidiPack → Midi).
//
// NAI-213-D-REGISTRY-RETURN: TS exposes these as module-level singletons
// (e.g. tools/pack/PackFile.ts:InterfacePack); goscape returns a
// per-PackAll Registry instead. Permanent structural shape change.
//
// TS source: tools/pack/PackFile.ts:191-206 @ 9aadcec4 (rev-244 B6).
type Registry struct {
	SrcDir string

	// SuspendAutoReload, when true, makes every EnsureX accessor
	// construct an EMPTY PackFile (skipping the .pack-file reload) so
	// GetByID returns "" for every id. The unpack tools that consume
	// the shared name registries then emit DEFAULT cross-reference
	// names (loc_<n>, midi_<n>, numeric texture ids, empty model=, ...).
	//
	// This mirrors TS PackFile.suspendAutoReload (PackFileBase.ts:31-33
	// constructor), which tools/pack/PackFile.ts:276 sets true around the
	// shared `export const` registries (@dee467c8, NEW at 274). Those
	// singletons are therefore constructed empty in the unpack process.
	// The PACK pipeline leaves this false (its registries reload).
	//
	// The graphics unpack tools (UnpackModels/UnpackAnims) import
	// PackFile from PackFileBase.js and construct their OWN local packs
	// AFTER module init (when suspendAutoReload is back to false), so
	// those self-load — goscape's graphics unpack keeps this flag false.
	SuspendAutoReload bool

	Interface, Obj, Seq, Loc, Npc, Model, Anim, Base,
	Synth, Texture, Varp, Varbit, Varn, Vars, Inv, SpotAnim, Idk,
	Flo, Category, Hunt, Param, DbTable, DbRow, MesAnim, Struct,
	AnimSet, Map, Midi *PackFile
}

func (r *Registry) ensure(field **PackFile, packType string) (*PackFile, error) {
	if *field != nil {
		return *field, nil
	}
	if r.SuspendAutoReload {
		pf := newEmptyPackFile(r.SrcDir, packType, nil)
		*field = pf
		return pf, nil
	}
	pf, err := NewPackFile(r.SrcDir, packType, nil)
	if err != nil {
		return nil, err
	}
	*field = pf
	return pf, nil
}

func (r *Registry) EnsureInterface() (*PackFile, error) { return r.ensure(&r.Interface, "interface") }
func (r *Registry) EnsureObj() (*PackFile, error)       { return r.ensure(&r.Obj, "obj") }
func (r *Registry) EnsureSeq() (*PackFile, error)       { return r.ensure(&r.Seq, "seq") }
func (r *Registry) EnsureLoc() (*PackFile, error)       { return r.ensure(&r.Loc, "loc") }
func (r *Registry) EnsureNpc() (*PackFile, error)       { return r.ensure(&r.Npc, "npc") }
func (r *Registry) EnsureModel() (*PackFile, error)     { return r.ensure(&r.Model, "model") }
func (r *Registry) EnsureAnim() (*PackFile, error)      { return r.ensure(&r.Anim, "anim") }
func (r *Registry) EnsureBase() (*PackFile, error)      { return r.ensure(&r.Base, "base") }
func (r *Registry) EnsureSynth() (*PackFile, error)     { return r.ensure(&r.Synth, "synth") }
func (r *Registry) EnsureTexture() (*PackFile, error)   { return r.ensure(&r.Texture, "texture") }
func (r *Registry) EnsureVarp() (*PackFile, error)      { return r.ensure(&r.Varp, "varp") }

// EnsureVarbit lazy-constructs the varbit PackFile (NEW at rev-254).
//
// TS: VarbitPack = new PackFile('varbit', validateConfigPack, '.varbit', true)
// TS source: tools/pack/PackFile.ts:220 @ 2e3bcf43.
func (r *Registry) EnsureVarbit() (*PackFile, error)   { return r.ensure(&r.Varbit, "varbit") }
func (r *Registry) EnsureVarn() (*PackFile, error)     { return r.ensure(&r.Varn, "varn") }
func (r *Registry) EnsureVars() (*PackFile, error)     { return r.ensure(&r.Vars, "vars") }
func (r *Registry) EnsureInv() (*PackFile, error)      { return r.ensure(&r.Inv, "inv") }
func (r *Registry) EnsureSpotAnim() (*PackFile, error) { return r.ensure(&r.SpotAnim, "spotanim") }
func (r *Registry) EnsureIdk() (*PackFile, error)      { return r.ensure(&r.Idk, "idk") }
func (r *Registry) EnsureFlo() (*PackFile, error)      { return r.ensure(&r.Flo, "flo") }
func (r *Registry) EnsureCategory() (*PackFile, error) { return r.ensure(&r.Category, "category") }
func (r *Registry) EnsureHunt() (*PackFile, error)     { return r.ensure(&r.Hunt, "hunt") }
func (r *Registry) EnsureParam() (*PackFile, error)    { return r.ensure(&r.Param, "param") }
func (r *Registry) EnsureDbTable() (*PackFile, error)  { return r.ensure(&r.DbTable, "dbtable") }
func (r *Registry) EnsureDbRow() (*PackFile, error)    { return r.ensure(&r.DbRow, "dbrow") }
func (r *Registry) EnsureMesAnim() (*PackFile, error)  { return r.ensure(&r.MesAnim, "mesanim") }
func (r *Registry) EnsureStruct() (*PackFile, error)   { return r.ensure(&r.Struct, "struct") }

// EnsureAnimSet lazy-constructs the animset PackFile.
//
// TS: AnimSetPack = new PackFile('animset', validateFilesPack, [BUILD_SRC_DIR/models], '.anim')
// TS source: tools/pack/PackFile.ts:191 @ 9aadcec4.
func (r *Registry) EnsureAnimSet() (*PackFile, error) { return r.ensure(&r.AnimSet, "animset") }

// EnsureMap lazy-constructs the map PackFile.
//
// TS: MapPack = new PackFile('map', validateFilesPack, [BUILD_SRC_DIR/maps], '.jm2', false)
// TS source: tools/pack/PackFile.ts:205 @ 9aadcec4.
func (r *Registry) EnsureMap() (*PackFile, error) { return r.ensure(&r.Map, "map") }

// EnsureMidi lazy-constructs the midi PackFile.
//
// TS: MidiPack = new PackFile('midi', validateFilesPack, [BUILD_SRC_DIR/jingles, BUILD_SRC_DIR/songs], '.mid')
// TS source: tools/pack/PackFile.ts:206 @ 9aadcec4.
func (r *Registry) EnsureMidi() (*PackFile, error) { return r.ensure(&r.Midi, "midi") }
