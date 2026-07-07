// Package config implements the config-archive unpackers for the RS2 254 tool chain.
//
// TS source: tools/unpack/config/Common.ts, tools/unpack/config/Unpack.ts (readConfigIdx only).
package config

import (
	"errors"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/render/model"
)

// ConfigIdx mirrors tools/unpack/config/Common.ts:
//
//	export type ConfigIdx = { size: number, pos: number[], len: number[], dat: Packet };
type ConfigIdx struct {
	Size int
	Pos  []int
	Len  []int
	Dat  *packet.Packet
}

// ReadConfigIdx mirrors Unpack.ts readConfigIdx (lines 22-45).
// count = g2 from idx; per-entry len = g2, pos accumulates starting at 2.
// TS calls printFatalError on missing idx/dat; Go returns an error instead.
//
// dat must be the RAW <type>.dat archive blob: its first two bytes are the
// entry count, so entry data starts at offset 2 — that is why pos accumulates
// from 2. Passing an already-sliced (post-header) blob shifts every position
// and trips the per-entry incomplete-read warnings.
//
// The returned ConfigIdx is NOT safe for concurrent use: every unpacker seeks
// the shared Dat packet (Dat.Pos) before reading. Unpack sequentially, or
// build one ConfigIdx per goroutine.
//
// TS source: tools/unpack/config/Unpack.ts:22-45.
func ReadConfigIdx(idx, dat *packet.Packet) (*ConfigIdx, error) {
	if idx == nil || dat == nil {
		return nil, errors.New("missing config data")
	}

	count := int(idx.G2())

	pos := make([]int, count)
	lens := make([]int, count)

	cur := 2
	for i := range count {
		pos[i] = cur
		lens[i] = int(idx.G2())
		cur += lens[i]
	}

	return &ConfigIdx{
		Size: count,
		Pos:  pos,
		Len:  lens,
		Dat:  dat,
	}, nil
}

// Env carries the name-registries and sinks that per-type unpackers consult.
// Fields cover all config types; later tasks extend usage of Models and SrcDir.
// Warnf is the TS printWarning sink; nil = no-op.
// Errorf is the TS console.error sink (used by renameModel on file-not-found); nil = no-op.
type Env struct {
	Flo      *pack.PackFile
	Texture  *pack.PackFile
	Varp     *pack.PackFile
	Varbit   *pack.PackFile // NEW at rev-254 (TS VarbitPack)
	Seq      *pack.PackFile
	Anim     *pack.PackFile
	Obj      *pack.PackFile
	Model    *pack.PackFile
	Npc      *pack.PackFile
	Loc      *pack.PackFile
	SpotAnim *pack.PackFile
	Idk      *pack.PackFile

	Models *model.Store // pkg/render/model; unused by flo/varp/seq
	SrcDir string       // content tree root; unused by flo/varp/seq

	Warnf  func(format string, args ...any) // nil = no-op; TS printWarning
	Errorf func(format string, args ...any) // nil = no-op; TS console.error
}

// warnf calls e.Warnf when non-nil.
func (e *Env) warnf(format string, args ...any) {
	if e.Warnf != nil {
		e.Warnf(format, args...)
	}
}

// errorf calls e.Errorf when non-nil.
func (e *Env) errorf(format string, args ...any) {
	if e.Errorf != nil {
		e.Errorf(format, args...)
	}
}

// getByID returns pf.GetByID(id) when pf is non-nil, else "".
func getByID(pf *pack.PackFile, id int) string {
	if pf == nil {
		return ""
	}
	return pf.GetByID(id)
}

// UnpackFlo mirrors tools/unpack/config/FloConfig.ts:unpackFloConfig.
//
// TS source: tools/unpack/config/FloConfig.ts:6-47.
func (e *Env) UnpackFlo(cfg *ConfigIdx, id int) []string {
	return unpackFlo(cfg, id, e.Flo, e.Texture, e.warnf)
}

// UnpackVarp mirrors tools/unpack/config/VarpConfig.ts:unpackVarpConfig.
//
// TS source: tools/unpack/config/VarpConfig.ts:6-33.
func (e *Env) UnpackVarp(cfg *ConfigIdx, id int) []string {
	return unpackVarp(cfg, id, e.Varp, e.warnf)
}

// UnpackVarbit mirrors tools/unpack/config/VarbitConfig.ts:unpackVarbitConfig
// (NEW at rev-254).
//
// TS source: tools/unpack/config/VarbitConfig.ts:6-38 @2e3bcf43.
func (e *Env) UnpackVarbit(cfg *ConfigIdx, id int) []string {
	return unpackVarbit(cfg, id, e.Varbit, e.Varp, e.warnf)
}

// UnpackSeq mirrors tools/unpack/config/SeqConfig.ts:unpackSeqConfig.
//
// TS source: tools/unpack/config/SeqConfig.ts:6-138.
func (e *Env) UnpackSeq(cfg *ConfigIdx, id int) []string {
	return unpackSeq(cfg, id, e.Seq, e.Anim, e.Obj, e.warnf)
}

// UnpackIdk mirrors tools/unpack/config/IdkConfig.ts:unpackIdkConfig.
// compare/modelRenameOffset feed the 254 model-rename guard.
//
// TS source: tools/unpack/config/IdkConfig.ts:58-159 @2e3bcf43.
func (e *Env) UnpackIdk(cfg *ConfigIdx, id int, compare *ConfigIdx, modelRenameOffset int) []string {
	return unpackIdk(cfg, id, compare, modelRenameOffset, e.Idk, e.Texture, e.Model, e.Models, e.SrcDir, e.warnf, e.errorf)
}

// UnpackNpc mirrors tools/unpack/config/NpcConfig.ts:unpackNpcConfig.
// compare/modelRenameOffset feed the 254 model-rename guard.
//
// TS source: tools/unpack/config/NpcConfig.ts:41-198 @2e3bcf43.
func (e *Env) UnpackNpc(cfg *ConfigIdx, id int, compare *ConfigIdx, modelRenameOffset int) []string {
	return unpackNpc(cfg, id, compare, modelRenameOffset, e.Npc, e.Texture, e.Seq, e.Model, e.Models, e.SrcDir, e.warnf, e.errorf)
}

// UnpackObj mirrors tools/unpack/config/ObjConfig.ts:unpackObjConfig.
// compare/modelRenameOffset feed the 254 model-rename guard.
//
// TS source: tools/unpack/config/ObjConfig.ts:41-308 @2e3bcf43.
func (e *Env) UnpackObj(cfg *ConfigIdx, id int, compare *ConfigIdx, modelRenameOffset int) []string {
	return unpackObj(cfg, id, compare, modelRenameOffset, e.Obj, e.Texture, e.Seq, e.Model, e.Models, e.SrcDir, e.warnf, e.errorf)
}

// UnpackSpotAnim mirrors tools/unpack/config/SpotAnimConfig.ts:unpackSpotAnimConfig.
// compare/modelRenameOffset feed the 254 model-rename guard.
//
// TS source: tools/unpack/config/SpotAnimConfig.ts:41-147 @2e3bcf43.
func (e *Env) UnpackSpotAnim(cfg *ConfigIdx, id int, compare *ConfigIdx, modelRenameOffset int) []string {
	return unpackSpotAnim(cfg, id, compare, modelRenameOffset, e.SpotAnim, e.Texture, e.Seq, e.Model, e.Models, e.SrcDir, e.warnf, e.errorf)
}

// UnpackLoc mirrors tools/unpack/config/LocConfig.ts:unpackLocConfig.
// Returns an error when an unknown opcode is encountered (TS printFatalError).
//
// TS source: tools/unpack/config/LocConfig.ts:157-330.
func (e *Env) UnpackLoc(cfg *ConfigIdx, id int) ([]string, error) {
	return unpackLoc(cfg, id, e.Loc, e.Texture, e.Seq, e.Model, e.Models, e.warnf)
}

// UnpackLocModels mirrors tools/unpack/config/LocConfig.ts:unpackLocModels.
//
// TS source: tools/unpack/config/LocConfig.ts:25-129.
func (e *Env) UnpackLocModels(cfg *ConfigIdx, id int) LocModels {
	return unpackLocModels(cfg, id, e.warnf)
}
