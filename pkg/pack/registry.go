package pack

// Registry holds the *PackFile singletons that PackConfigs builds
// while packing. Client stages (clientinterface, sprites, audio,
// graphics) read from it after PackConfigs returns.
//
// Each EnsureX accessor lazily constructs on first call and memoizes.
// Field names match the TS singleton names (InterfacePack → Interface).
//
// NAI-213-D-REGISTRY-RETURN: TS exposes these as module-level singletons
// (e.g. tools/pack/PackFile.ts:InterfacePack); goscape returns a
// per-PackAll Registry instead. Permanent structural shape change.
type Registry struct {
	SrcDir string

	Interface, Obj, Seq, Loc, Npc, Model, Anim, Base,
	Synth, Texture, Varp, Varn, Vars, Inv, SpotAnim, Idk,
	Flo, Category, Hunt, Param, DbTable, DbRow, MesAnim, Struct,
	Enum, Script *PackFile
}

func (r *Registry) ensure(field **PackFile, packType string) (*PackFile, error) {
	if *field != nil {
		return *field, nil
	}
	pf, err := NewPackFile(r.SrcDir, packType, nil)
	if err != nil {
		return nil, err
	}
	*field = pf
	return pf, nil
}

func (r *Registry) EnsureEnum() (*PackFile, error)      { return r.ensure(&r.Enum, "enum") }
func (r *Registry) EnsureScript() (*PackFile, error)    { return r.ensure(&r.Script, "script") }
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
func (r *Registry) EnsureVarn() (*PackFile, error)      { return r.ensure(&r.Varn, "varn") }
func (r *Registry) EnsureVars() (*PackFile, error)      { return r.ensure(&r.Vars, "vars") }
func (r *Registry) EnsureInv() (*PackFile, error)       { return r.ensure(&r.Inv, "inv") }
func (r *Registry) EnsureSpotAnim() (*PackFile, error)  { return r.ensure(&r.SpotAnim, "spotanim") }
func (r *Registry) EnsureIdk() (*PackFile, error)       { return r.ensure(&r.Idk, "idk") }
func (r *Registry) EnsureFlo() (*PackFile, error)       { return r.ensure(&r.Flo, "flo") }
func (r *Registry) EnsureCategory() (*PackFile, error)  { return r.ensure(&r.Category, "category") }
func (r *Registry) EnsureHunt() (*PackFile, error)      { return r.ensure(&r.Hunt, "hunt") }
func (r *Registry) EnsureParam() (*PackFile, error)     { return r.ensure(&r.Param, "param") }
func (r *Registry) EnsureDbTable() (*PackFile, error)   { return r.ensure(&r.DbTable, "dbtable") }
func (r *Registry) EnsureDbRow() (*PackFile, error)     { return r.ensure(&r.DbRow, "dbrow") }
func (r *Registry) EnsureMesAnim() (*PackFile, error)   { return r.ensure(&r.MesAnim, "mesanim") }
func (r *Registry) EnsureStruct() (*PackFile, error)    { return r.ensure(&r.Struct, "struct") }
