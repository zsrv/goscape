package objtype

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	io "github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// SeqType is a single seq.dat config record (animation sequence).
// Mirrors Engine-TS/src/cache/config/SeqType.ts at Engine-TS 9aadcec4.
type SeqType struct {
	ConfigType
	FrameCount       int     // TS SeqType.ts:72 (Engine-TS 9aadcec4)
	Frames           []int32 // nil = unset
	IFrames          []int32 // nil = unset; 65535 in source is normalised to -1 (TS L91)
	Delay            []int32 // per-frame delay; 0 → AnimFrame.Delay fallback (TS L105)
	Loops            int     // -1 default
	WalkMerge        []int32 // nil = unset; last entry = 9999999 sentinel (TS L116)
	Stretches        bool
	Priority         int // 5 default
	ReplaceHeldLeft  int // -1 default
	ReplaceHeldRight int // -1 default
	MaxLoops         int // 99 default
	// New in 244:
	PreanimMove       int // -1 default; code 9 g1; TS SeqType.ts:83 (Engine-TS 9aadcec4)
	PostanimMove      int // -1 default; code 10 g1; TS SeqType.ts:84 (Engine-TS 9aadcec4)
	DuplicateBehavior int // 0 default; code 11 g1; TS SeqType.ts:85 (Engine-TS 9aadcec4)

	// precalculated for seqlength
	// TS SeqType.ts:88 (Engine-TS 9aadcec4) — "precalculated for seqlength"
	Duration int

	// animFrames is an unexported back-reference to the AnimFrame registry
	// used by Decode for the delay-fallback. parseSeqTypes sets it
	// before invoking DecodeType.
	animFrames *AnimFrameConfigs
}

// NewSeqType returns a SeqType with TS-faithful defaults.
// TS SeqType.ts:70-88 (Engine-TS 9aadcec4)
func NewSeqType(id int) *SeqType {
	return &SeqType{
		ConfigType:        ConfigType{ID: id},
		Loops:             -1,
		Priority:          5,
		ReplaceHeldLeft:   -1,
		ReplaceHeldRight:  -1,
		MaxLoops:          99,
		PreanimMove:       -1, // TS SeqType.ts:83 — preanim_move: number = -1
		PostanimMove:      -1, // TS SeqType.ts:84 — postanim_move: number = -1
		DuplicateBehavior: 0,  // TS SeqType.ts:85 — duplicatebehavior: number = 0
	}
}

// Decode dispatches on the seq config opcode, matching TS SeqType.decode
// at Engine-TS/src/cache/config/SeqType.ts:90-176 (Engine-TS 9aadcec4).
func (t *SeqType) Decode(code uint8, dat *packet.Packet) error {
	switch code {
	case 1:
		// TS SeqType.ts:91-118 (Engine-TS 9aadcec4)
		t.FrameCount = int(dat.G1())
		t.Frames = make([]int32, t.FrameCount)
		t.IFrames = make([]int32, t.FrameCount)
		t.Delay = make([]int32, t.FrameCount)
		for i := range t.FrameCount {
			t.Frames[i] = int32(dat.G2())

			v := int32(dat.G2())
			if v == 65535 {
				v = -1 // TS L101 normalisation
			}
			t.IFrames[i] = v

			d := int32(dat.G2())
			if d == 0 {
				// TS L105 unconditionally derefs AnimFrame.instances[frames[i]].delay;
				// an OOR frames[i] throws TypeError, aborting the whole config
				// parse. Match TS by dropping the bounds guard so OOR panics here
				// (Go equivalent of TS's throw — both halt parsing on bad data;
				// silent-default would mask data-corruption that TS would catch).
				// cfg-media-1 (2026-05-28 audit). The nil-animFrames guard is
				// preserved as additive robustness (CONFIRMED-EXCEPTION in the
				// ledger) for test fixtures that omit an AnimFrame back-ref;
				// production callers (LoadSeqTypes/parseSeqTypes) always set it.
				if t.animFrames != nil {
					d = int32(t.animFrames.Instances[t.Frames[i]].Delay)
				}
			}
			if d == 0 {
				d = 1 // TS L109
			}
			t.Delay[i] = d
			// Note: duration is NOT accumulated here in 244 — it is computed
			// in postDecode. In 244 SeqType.ts:118 this.duration += this.delay[i]
			// still appears inside decode code 1. Keep it here to match TS exactly.
			// TS SeqType.ts:118 (Engine-TS 9aadcec4)
			t.Duration += int(d)
		}
	case 2:
		t.Loops = int(dat.G2())
	case 3:
		count := int(dat.G1())
		t.WalkMerge = make([]int32, count+1)
		for i := range count {
			t.WalkMerge[i] = int32(dat.G1())
		}
		t.WalkMerge[count] = 9999999 // TS L128
	case 4:
		t.Stretches = true
	case 5:
		t.Priority = int(dat.G1())
	case 6:
		t.ReplaceHeldLeft = int(dat.G2())
	case 7:
		t.ReplaceHeldRight = int(dat.G2())
	case 8:
		t.MaxLoops = int(dat.G1())
	case 9:
		// TS SeqType.ts:137-139 (Engine-TS 9aadcec4) — preanim_move = dat.g1()
		t.PreanimMove = int(dat.G1())
	case 10:
		// TS SeqType.ts:140-142 (Engine-TS 9aadcec4) — postanim_move = dat.g1()
		t.PostanimMove = int(dat.G1())
	case 11:
		// TS SeqType.ts:143-145 (Engine-TS 9aadcec4) — duplicatebehavior = dat.g1()
		t.DuplicateBehavior = int(dat.G1())
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized seq config code %d", code)
	}
	return nil
}

// PostDecode performs post-load fixups after both server and client config
// streams have been decoded. Mirrors TS SeqType.postDecode() at
// Engine-TS/src/cache/config/SeqType.ts:148-175 (Engine-TS 9aadcec4).
func (t *SeqType) PostDecode() {
	// TS SeqType.ts:149-157 — if frameCount === 0, create 1-frame stubs.
	if t.FrameCount == 0 {
		t.FrameCount = 1
		t.Frames = []int32{-1}
		t.IFrames = []int32{-1}
		t.Delay = []int32{-1}
	}

	// TS SeqType.ts:159-166 — preanim_move defaults depend on walkmerge.
	if t.PreanimMove == -1 {
		if t.WalkMerge == nil {
			t.PreanimMove = 0
		} else {
			t.PreanimMove = 2
		}
	}

	// TS SeqType.ts:168-175 — postanim_move defaults depend on walkmerge.
	if t.PostanimMove == -1 {
		if t.WalkMerge == nil {
			t.PostanimMove = 0
		} else {
			t.PostanimMove = 2
		}
	}
}

// SeqTypeConfigs is the parsed registry of all sequence config records.
type SeqTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*SeqType
}

// Count returns len(Configs) as the TS-equivalent SeqType.count static
// (Player.ts L1841 / Npc.ts L452 read this directly). Returns 0 on a
// nil receiver so consumers don't have to nil-guard separately.
func (c *SeqTypeConfigs) Count() int {
	if c == nil {
		return 0
	}
	return len(c.Configs)
}

// LoadSeqTypes parses server/seq.dat + client/config jag → seq.dat into
// a SeqTypeConfigs registry. The animFrames argument is captured by each
// *SeqType for the delay-fallback inside Decode. Returns an empty
// registry with nil error when server/seq.dat is absent (silent-on-missing,
// matching TS SeqType.load at Engine-TS/.../SeqType.ts:10-29 (Engine-TS 9aadcec4)).
func LoadSeqTypes(dir string, animFrames *AnimFrameConfigs) (*SeqTypeConfigs, error) {
	server, err := packet.Load(filepath.Join(dir, "server", "seq.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &SeqTypeConfigs{ConfigNames: map[string]int{}}, nil
		}
		return nil, err
	}

	clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
	if err != nil {
		return nil, err
	}

	return parseSeqTypes(server, clientJag, animFrames)
}

func parseSeqTypes(server *packet.Packet, clientJag *io.Jagfile, animFrames *AnimFrameConfigs) (*SeqTypeConfigs, error) {
	count := int(server.G2())
	configs := make([]*SeqType, count)
	configNames := make(map[string]int, count)

	client, err := clientJag.Read("seq.dat")
	if err != nil {
		return nil, err
	}
	client.Pos = 2 // skip client-side count header (matches idktype.go pattern)

	for id := range count {
		config := NewSeqType(id)
		config.animFrames = animFrames
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		if err := DecodeType(client, config); err != nil {
			return nil, err
		}
		// TS SeqType.ts:37 (Engine-TS 9aadcec4) — config.postDecode()
		config.PostDecode()
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &SeqTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}

// ByName returns the SeqType matching the given debugname, or nil
// if no match exists. Mirrors TS SeqType.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) linear-scan fallback for test fixtures or stale indices.
// Consumed by dispatchDebugproc in modules/world/handlers_game.go (NAI-189).
func (c *SeqTypeConfigs) ByName(name string) *SeqType {
	if c == nil {
		return nil
	}
	if id, ok := c.ConfigNames[name]; ok {
		if id >= 0 && id < len(c.Configs) {
			return c.Configs[id]
		}
	}
	for _, t := range c.Configs {
		if t != nil && t.DebugName == name {
			return t
		}
	}
	return nil
}
