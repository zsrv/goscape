// Package config implements the config-archive unpackers for the RS2 244 tool chain.
//
// TS source: tools/unpack/config/Common.ts, tools/unpack/config/Unpack.ts (readConfigIdx only).
package config

import (
	"errors"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
	"github.com/zsrv/goscape/pkg/unpack/internal/model"
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
type Env struct {
	Flo      *pack.PackFile
	Texture  *pack.PackFile
	Varp     *pack.PackFile
	Seq      *pack.PackFile
	Anim     *pack.PackFile
	Obj      *pack.PackFile
	Model    *pack.PackFile
	Npc      *pack.PackFile
	Loc      *pack.PackFile
	SpotAnim *pack.PackFile
	Idk      *pack.PackFile

	Models *model.Store // pkg/unpack/internal/model; unused by flo/varp/seq
	SrcDir string       // content tree root; unused by flo/varp/seq

	Warnf func(format string, args ...any) // nil = no-op
}

// warnf calls e.Warnf when non-nil.
func (e *Env) warnf(format string, args ...any) {
	if e.Warnf != nil {
		e.Warnf(format, args...)
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

// UnpackSeq mirrors tools/unpack/config/SeqConfig.ts:unpackSeqConfig.
//
// TS source: tools/unpack/config/SeqConfig.ts:6-138.
func (e *Env) UnpackSeq(cfg *ConfigIdx, id int) []string {
	return unpackSeq(cfg, id, e.Seq, e.Anim, e.Obj, e.warnf)
}

