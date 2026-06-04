package objtype

import (
	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// AnimFrame is a single animation frame record — delay plus transform data.
// Mirrors Engine-TS/src/cache/graphics/AnimFrame.ts at Engine-TS 9aadcec4.
//
// In 225, frame delays were loaded from server/frame_del.dat as SeqFrame.
// In 244, SeqFrame.ts is deleted; AnimFrame carries the delay directly and is
// loaded from the FileStream cache (archive 2) via AnimFrame.load().
// TS AnimFrame.ts:1-134 (Engine-TS 9aadcec4)
type AnimFrame struct {
	// Delay is the per-frame delay in client ticks.
	// TS AnimFrame.ts:9 (Engine-TS 9aadcec4)
	Delay int

	// Base is the AnimBase id this frame was decoded against.
	// TS AnimFrame.ts:10 (Engine-TS 9aadcec4)
	Base int

	// Length is the number of transform groups.
	// TS AnimFrame.ts:11 (Engine-TS 9aadcec4)
	Length int

	// Groups, X, Y, Z are the per-transform-group data.
	// TS AnimFrame.ts:12-15 (Engine-TS 9aadcec4)
	Groups []int32
	X      []int32
	Y      []int32
	Z      []int32
}

// AnimFrameConfigs holds all parsed AnimFrame records, indexed by frame id.
// TS exposes this as `AnimFrame.instances` static (Engine-TS/.../AnimFrame.ts:6).
type AnimFrameConfigs struct {
	Instances []*AnimFrame
	Order     []int
}

// LoadAnimFrames reads all animation frames from the FileStream cache at dir
// (archive 2). It mirrors TS AnimFrame.load() at Engine-TS/.../AnimFrame.ts:17-24
// (Engine-TS 9aadcec4) which calls OnDemand.cache.count(2) / .read(2, i, true).
//
// Returns an empty registry with nil error when the cache is absent or empty
// (matches TS AnimFrame.load silent-on-missing behaviour).
func LoadAnimFrames(dir string) (*AnimFrameConfigs, error) {
	// TS AnimFrame.load() opens FileStream('data/pack'), which is the cache
	// root — callers pass cfg.CachePath (e.g. "data/pack") exactly as TS does.
	// TS AnimFrame.ts:17-24 (Engine-TS 9aadcec4)
	fs := filestream.New(dir, false, true)
	defer func() { _ = fs.Close() }()

	return parseAnimFrames(fs), nil
}

// parseAnimFrames reads all frames from archive 2 of the given FileStream.
// TS AnimFrame.load() + AnimFrame.unpack() — Engine-TS 9aadcec4
// AnimFrame.ts:17-134
func parseAnimFrames(fs *filestream.FileStream) *AnimFrameConfigs {
	cfg := &AnimFrameConfigs{}

	// TS AnimFrame.ts:18 — count(2) returns number of files in archive 2.
	count := fs.Count(2)
	for i := range count {
		// TS AnimFrame.ts:19-22 — read(2, i, true) decompresses the gzip blob.
		data := fs.Read(2, i, true)
		if data == nil {
			continue
		}
		unpackAnimFrames(cfg, data)
	}

	return cfg
}

// animBase is the minimal AnimBase representation needed for AnimFrame.unpack.
// TS AnimBase.ts (Engine-TS 9aadcec4) — we only need 'types' and the OP_BASE
// constant for transform-group decoding.
type animBase struct {
	types []int32
}

const (
	animBaseOPBase  = 0 // TS AnimBase.OP_BASE  = 0
	animBaseOPScale = 3 // TS AnimBase.OP_SCALE = 3
)

// unpackAnimBase parses an AnimBase record from the data stream and appends it
// to a local slice, returning the index used as baseId. Mirrors
// TS AnimBase.unpack() at Engine-TS/.../AnimBase.ts:17-39 (Engine-TS 9aadcec4).
func unpackAnimBase(bases *[]animBase, dat *packet.Packet) int {
	// TS AnimBase.ts:18 — length = dat.g1()
	length := int(dat.G1())

	types := make([]int32, length)
	labels := make([][]int32, length)

	// TS AnimBase.ts:20-22 — types[i] = dat.g1()
	for i := range length {
		types[i] = int32(dat.G1())
	}

	// TS AnimBase.ts:24-29 — labels[i] = new Int32Array(labelCount); read g1 each
	for i := range length {
		labelCount := int(dat.G1())
		labels[i] = make([]int32, labelCount)
		for j := range labelCount {
			labels[i][j] = int32(dat.G1())
		}
	}

	b := animBase{types: types}
	*bases = append(*bases, b)
	return len(*bases) - 1
}

// unpackAnimFrames parses one FileStream blob (archive 2, file i) and appends
// decoded AnimFrame records into cfg. Mirrors TS AnimFrame.unpack() at
// Engine-TS/.../AnimFrame.ts:26-130 (Engine-TS 9aadcec4).
func unpackAnimFrames(cfg *AnimFrameConfigs, src []byte) {
	// TS AnimFrame.ts:27-28: meta packet positioned at end-8 for section lengths.
	meta := packet.NewPacket(src)
	meta.Pos = len(src) - 8

	offset := 0

	// TS AnimFrame.ts:31-32: head section = meta.g2() + 2 bytes for the count g2.
	head := packet.NewPacket(src)
	head.Pos = offset
	offset += int(meta.G2()) + 2

	// TS AnimFrame.ts:34-35: tran1 section.
	tran1 := packet.NewPacket(src)
	tran1.Pos = offset
	offset += int(meta.G2())

	// TS AnimFrame.ts:37-38: tran2 section.
	tran2 := packet.NewPacket(src)
	tran2.Pos = offset
	offset += int(meta.G2())

	// TS AnimFrame.ts:40-41: del section.
	del := packet.NewPacket(src)
	del.Pos = offset
	offset += int(meta.G2())

	// TS AnimFrame.ts:43-44: baseData section — remaining bytes; AnimBase.unpack appended.
	baseData := packet.NewPacket(src)
	baseData.Pos = offset

	// Each blob carries its own bases slice (matching TS AnimBase.instances which
	// accumulates across calls — we use a local slice indexed by baseId returned
	// from unpackAnimBase, matching the append-and-return-index pattern).
	var bases []animBase
	baseId := unpackAnimBase(&bases, baseData)

	// TS AnimFrame.ts:47: total = head.g2() — number of frames in this blob.
	total := int(head.G2())

	tmpBases := make([]int32, 500)
	tmpX := make([]int32, 500)
	tmpY := make([]int32, 500)
	tmpZ := make([]int32, 500)

	for range total {
		// TS AnimFrame.ts:50-51: id = head.g2(), order.push(id).
		id := int(head.G2())
		cfg.Order = append(cfg.Order, id)

		frame := &AnimFrame{
			// TS AnimFrame.ts:54: delay = del.g1()
			Delay: int(del.G1()),
			// TS AnimFrame.ts:55: base = baseId
			Base: baseId,
		}

		// TS AnimFrame.ts:57: groupCount = head.g1()
		groupCount := int(head.G1())
		lastGroup := -1
		length := 0

		for group := range groupCount {
			// TS AnimFrame.ts:61: flags = tran1.g1()
			flags := int(tran1.G1())
			if flags == 0 {
				continue
			}

			// TS AnimFrame.ts:65-72: insert OP_BASE filler if the current group's
			// type is not OP_BASE and there's an OP_BASE between lastGroup+1..group-1.
			if int(bases[baseId].types[group]) != animBaseOPBase {
				for cur := group - 1; cur > lastGroup; cur-- {
					if int(bases[baseId].types[cur]) == animBaseOPBase {
						tmpBases[length] = int32(cur)
						tmpX[length] = 0
						tmpY[length] = 0
						tmpZ[length] = 0
						length++
						break
					}
				}
			}

			tmpBases[length] = int32(group)

			// TS AnimFrame.ts:76-79: defaultValue = 128 for OP_SCALE, else 0.
			defaultValue := int32(0)
			if int(bases[baseId].types[group]) == animBaseOPScale {
				defaultValue = 128
			}

			// TS AnimFrame.ts:81-85: x
			if flags&0x1 != 0 {
				tmpX[length] = int32(tran2.GSmart())
			} else {
				tmpX[length] = defaultValue
			}

			// TS AnimFrame.ts:87-91: y
			if flags&0x2 != 0 {
				tmpY[length] = int32(tran2.GSmart())
			} else {
				tmpY[length] = defaultValue
			}

			// TS AnimFrame.ts:93-97: z
			if flags&0x4 != 0 {
				tmpZ[length] = int32(tran2.GSmart())
			} else {
				tmpZ[length] = defaultValue
			}

			lastGroup = group
			length++
		}

		// TS AnimFrame.ts:100-109: copy tmp arrays into frame.
		frame.Length = length
		frame.Groups = make([]int32, length)
		frame.X = make([]int32, length)
		frame.Y = make([]int32, length)
		frame.Z = make([]int32, length)
		for j := range length {
			frame.Groups[j] = tmpBases[j]
			frame.X[j] = tmpX[j]
			frame.Y[j] = tmpY[j]
			frame.Z[j] = tmpZ[j]
		}

		// TS AnimFrame.ts:112: AnimFrame.instances[id] = frame
		// Grow slice if necessary.
		for len(cfg.Instances) <= id {
			cfg.Instances = append(cfg.Instances, nil)
		}
		cfg.Instances[id] = frame
	}
}
