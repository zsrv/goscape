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
// Mirrors Engine-TS/src/cache/config/SeqType.ts.
type SeqType struct {
	ConfigType
	Frames           []int32 // nil = unset
	IFrames          []int32 // nil = unset; 65535 in source is normalised to -1 (TS L91)
	Delay            []int32 // per-frame delay; 0 → SeqFrame.Delay fallback (TS L97)
	Loops            int     // -1 default
	WalkMerge        []int32 // nil = unset; last entry = 9999999 sentinel (TS L116)
	Stretches        bool
	Priority         int // 5 default
	ReplaceHeldLeft  int // -1 default
	ReplaceHeldRight int // -1 default
	MaxLoops         int // 99 default
	Duration         int // sum of post-fallback Delay entries

	// frames is an unexported back-reference to the SeqFrame registry
	// used by Decode for the L97 delay-fallback. parseSeqTypes sets it
	// before invoking DecodeType.
	frames *SeqFrameConfigs
}

// NewSeqType returns a SeqType with TS-faithful defaults.
func NewSeqType(id int) *SeqType {
	return &SeqType{
		ConfigType:       ConfigType{ID: id},
		Loops:            -1,
		Priority:         5,
		ReplaceHeldLeft:  -1,
		ReplaceHeldRight: -1,
		MaxLoops:         99,
	}
}

// Decode dispatches on the seq config opcode, matching TS SeqType.decode
// at Engine-TS/src/cache/config/SeqType.ts:80-131.
func (t *SeqType) Decode(code uint8, dat *packet.Packet) error {
	switch code {
	case 1:
		count := int(dat.G1())
		t.Frames = make([]int32, count)
		t.IFrames = make([]int32, count)
		t.Delay = make([]int32, count)
		for i := range count {
			t.Frames[i] = int32(dat.G2())

			v := int32(dat.G2())
			if v == 65535 {
				v = -1 // TS L91 normalisation
			}
			t.IFrames[i] = v

			d := int32(dat.G2())
			if d == 0 {
				// TS L97 unconditionally derefs SeqFrame.instances[frames[i]].delay;
				// an OOR frames[i] throws TypeError, aborting the whole config
				// parse. Match TS by dropping the bounds guard so OOR panics here
				// (Go equivalent of TS's throw — both halt parsing on bad data;
				// silent-default would mask data-corruption that TS would catch).
				// cfg-media-1 (2026-05-28 audit). The nil-frames AND empty-
				// registry guards are preserved as additive robustness
				// (CONFIRMED-EXCEPTION in the ledger): a missing back-ref OR an
				// empty Instances (e.g. an absent/cross-revision main_file_cache
				// where LoadSeqFrames yields no frames) falls through to the
				// L101 default instead of indexing past the empty slice. A
				// POPULATED registry with an OOR frames[i] still panics (TS
				// throws). Production callers (LoadSeqTypes/parseSeqTypes) set a
				// populated registry.
				if t.frames != nil && len(t.frames.Instances) > 0 {
					d = int32(t.frames.Instances[t.Frames[i]].Delay)
				}
			}
			if d == 0 {
				d = 1 // TS L101
			}
			t.Delay[i] = d
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
		t.WalkMerge[count] = 9999999 // TS L116
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
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized seq config code %d", code)
	}
	return nil
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
// a SeqTypeConfigs registry. The frames argument is captured by each
// *SeqType for the L97 delay-fallback inside Decode. Returns an empty
// registry with nil error when server/seq.dat is absent (silent-on-missing,
// matching TS SeqType.load at Engine-TS/.../SeqType.ts:12-20).
func LoadSeqTypes(dir string, frames *SeqFrameConfigs) (*SeqTypeConfigs, error) {
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

	return parseSeqTypes(server, clientJag, frames)
}

func parseSeqTypes(server *packet.Packet, clientJag *io.Jagfile, frames *SeqFrameConfigs) (*SeqTypeConfigs, error) {
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
		config.frames = frames
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		if err := DecodeType(client, config); err != nil {
			return nil, err
		}
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
