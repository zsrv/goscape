# pack-worldmap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `tools/pack/map/Worldmap.ts` (682 LOC) to a goscape `pkg/pack/worldmap` package + a `goscape-cli worldmap` verb. Produces `data/pack/mapview/worldmap.jag`.

**Architecture:** Standalone driver (NOT added to `packall.PackAll`, matching TS). Reads `outDir/server/maps/{m,l,o,n}*` (output of `pkg/pack/maps`) plus CSV/labels/fonts/sprites from Content `srcDir`. Writes a 22-entry jagfile via `pkg/io/jagfile`.

**Tech Stack:** Go 1.26+. Standard lib + existing goscape packages: `pkg/io/{packet,jagfile}`, `pkg/coordgrid`, `pkg/pixpack`, `pkg/objtype` (new `flotype.go`), `pkg/pathfinder/loc`.

**Conventions (always apply):**
- All `go` commands MUST be prefixed: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
- All commits MUST use `git commit --no-gpg-sign`
- Use `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer in commit messages

**Spec:** `docs/superpowers/specs/2026-05-17-pack-worldmap-design.md`

---

## File Structure

**New files:**
- `pkg/objtype/flotype.go` — minimal FloType binary loader (name + count)
- `pkg/objtype/flotype_test.go` — unit tests
- `pkg/pack/worldmap/refcolors.go` — 80-entry floor color literal
- `pkg/pack/worldmap/refcolors_test.go` — length + spot-check
- `pkg/pack/worldmap/csv.go` — `processCsv`, `parseLabels`
- `pkg/pack/worldmap/csv_test.go` — branch coverage for processCsv
- `pkg/pack/worldmap/worldmap.go` — `Pack`, `packWater`, `unpackCoord`, per-map loop
- `pkg/pack/worldmap/worldmap_test.go` — unit tests for packWater/unpackCoord; integration smoke (build-tagged)
- `cmd/goscape-cli/cmd_worldmap.go` — new verb
- `cmd/goscape-cli/cmd_worldmap_test.go` — flag-parse exit-code tests

**Modified files:**
- `cmd/goscape-cli/main.go` — add `worldmap` to `verbs` slice

---

## Task 1: Add FloType binary loader

**Files:**
- Create: `pkg/objtype/flotype.go`
- Create: `pkg/objtype/flotype_test.go`

**Context:** TS `FloType.load('data/pack')` reads `data/pack/server/flo.dat`. `packWorldmap` uses only:
- `FloType.getId(name)` — name → numeric id (returns -1 if missing)
- `FloType.configs.length` — total count

Existing binary loaders (`pkg/objtype/loctype.go`, `pkg/objtype/npctype.go`) follow this shape:
- Open `dir/server/X.dat` via `packet.Load(path, false)`
- Open `dir/client/config` jagfile via `jagfile.LoadJagfile`, read inner `X.dat`
- Skip 2-byte count header on client side, then per-id decode

For worldmap we only need debugname → id mapping. The flo binary opcodes (per TS FloType decoder) we care about: opcode 2 = debugname (GJStrLF), opcode 0 = end-of-record sentinel. All other opcodes can be skipped by reading the right-sized payload.

The minimal-loader approach: instead of full FloType port, parse opcodes generically until 0, but only retain debugname. This keeps the loader small and avoids depending on the full FloType field set.

**TS reference for opcode payload sizes** (Engine-TS `src/cache/config/FloType.ts`):
- `1` → G3 (rgb)
- `2` → GJStrLF (debugname)
- `3` → GJStrLF (texture)
- `5` → G1 (occlude bool-byte)
- `6` → G2 (anim)
- `7` → G3 (hue_overlay)
- `8` → G3 (tint)

Use a generic "skip by code" map keyed by opcode → payload-bytes-or-string-marker.

- [ ] **Step 1: Write the failing test**

Create `pkg/objtype/flotype_test.go`:

```go
package objtype

import (
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestFloTypeConfigs_GetId_RoundTrip(t *testing.T) {
	t.Parallel()

	// Build a synthetic flo.dat with 3 entries:
	//   id 0: debugname="water",     opcode 2 then 0
	//   id 1: debugname="muddygrass", opcode 2 then 0
	//   id 2: debugname="grass",     opcode 2 then 0
	dat := packet2.Alloc(1)
	defer dat.Release()
	dat.P2(3) // count
	dat.P1(2)
	dat.PJStrLF("water")
	dat.P1(0)
	dat.P1(2)
	dat.PJStrLF("muddygrass")
	dat.P1(0)
	dat.P1(2)
	dat.PJStrLF("grass")
	dat.P1(0)

	cfg, err := parseFloTypes(dat)
	if err != nil {
		t.Fatalf("parseFloTypes: %v", err)
	}
	if got, want := len(cfg.Configs), 3; got != want {
		t.Errorf("len(Configs) = %d, want %d", got, want)
	}
	if got, want := cfg.GetId("water"), 0; got != want {
		t.Errorf("GetId(water) = %d, want %d", got, want)
	}
	if got, want := cfg.GetId("muddygrass"), 1; got != want {
		t.Errorf("GetId(muddygrass) = %d, want %d", got, want)
	}
	if got, want := cfg.GetId("nope"), -1; got != want {
		t.Errorf("GetId(nope) = %d, want %d", got, want)
	}
}

func TestFloTypeConfigs_SkipsUnknownOpcodes(t *testing.T) {
	t.Parallel()

	// One entry with several non-name opcodes interleaved.
	dat := packet2.Alloc(1)
	defer dat.Release()
	dat.P2(1)
	dat.P1(1)         // opcode 1: rgb (G3)
	dat.P3(0xaabbcc)
	dat.P1(2)         // opcode 2: debugname
	dat.PJStrLF("sandygrass")
	dat.P1(5)         // opcode 5: occlude (G1)
	dat.P1(1)
	dat.P1(6)         // opcode 6: anim (G2)
	dat.P2(0xdead)
	dat.P1(7)         // opcode 7: hue_overlay (G3)
	dat.P3(0x112233)
	dat.P1(3)         // opcode 3: texture (GJStrLF)
	dat.PJStrLF("planks")
	dat.P1(8)         // opcode 8: tint (G3)
	dat.P3(0x445566)
	dat.P1(0)

	cfg, err := parseFloTypes(dat)
	if err != nil {
		t.Fatalf("parseFloTypes: %v", err)
	}
	if got, want := cfg.GetId("sandygrass"), 0; got != want {
		t.Errorf("GetId = %d, want %d", got, want)
	}
}

func TestLoadFloTypes_RealContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}
	dir := "$HOME/Code/github.com/LostCityRS/Engine-TS/data/pack"
	if _, err := filepath.Abs(dir); err != nil {
		t.Skipf("real flo.dat not available: %v", err)
	}
	cfg, err := LoadFloTypes(dir)
	if err != nil {
		t.Skipf("LoadFloTypes(%s): %v", dir, err)
	}
	if len(cfg.Configs) == 0 {
		t.Fatalf("len(Configs) = 0")
	}
	if cfg.GetId("water") < 0 {
		t.Errorf("GetId(water) = -1 (expected >= 0)")
	}
	if cfg.GetId("muddygrass") < 0 {
		t.Errorf("GetId(muddygrass) = -1 (expected >= 0)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestFloType -v
```

Expected: FAIL with "parseFloTypes undefined" / "LoadFloTypes undefined" / "FloTypeConfigs undefined".

- [ ] **Step 3: Write the implementation**

Create `pkg/objtype/flotype.go`:

```go
package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// FloType is a minimal binary view of a floor-type entry. The full
// TS FloType has many more fields; goscape's worldmap packer only
// needs the debugname → id mapping and the total count, so we keep
// this lean.
type FloType struct {
	Id        int
	DebugName string
}

type FloTypeConfigs struct {
	Configs     []*FloType
	ConfigNames map[string]int
}

// GetId returns the numeric id for debugname, or -1 if unknown.
// Mirrors TS FloType.getId.
func (f *FloTypeConfigs) GetId(debugName string) int {
	if id, ok := f.ConfigNames[debugName]; ok {
		return id
	}
	return -1
}

// LoadFloTypes reads dir/server/flo.dat and returns the minimal
// view used by the worldmap packer. Format matches the standard
// goscape ConfigType encoding: u16 count + per-id opcode-tagged
// fields terminated by opcode 0.
func LoadFloTypes(dir string) (*FloTypeConfigs, error) {
	dat, err := packet2.Load(filepath.Join(dir, "server", "flo.dat"), false)
	if err != nil {
		return nil, fmt.Errorf("server/flo.dat: %w", err)
	}
	return parseFloTypes(dat)
}

func parseFloTypes(dat *packet2.Packet) (*FloTypeConfigs, error) {
	count := int(dat.G2())
	configs := make([]*FloType, count)
	names := make(map[string]int, count)
	for id := 0; id < count; id++ {
		ft := &FloType{Id: id}
		for {
			code := dat.G1()
			if code == 0 {
				break
			}
			switch code {
			case 1: // rgb
				_ = dat.G3()
			case 2: // debugname
				ft.DebugName = dat.GJStrLF()
			case 3: // texture
				_ = dat.GJStrLF()
			case 5: // occlude
				_ = dat.G1()
			case 6: // anim
				_ = dat.G2()
			case 7: // hue_overlay
				_ = dat.G3()
			case 8: // tint
				_ = dat.G3()
			default:
				return nil, fmt.Errorf("flo id %d: unknown opcode %d", id, code)
			}
		}
		configs[id] = ft
		if ft.DebugName != "" {
			names[ft.DebugName] = id
		}
	}
	return &FloTypeConfigs{Configs: configs, ConfigNames: names}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestFloType -v
```

Expected: PASS for `TestFloTypeConfigs_GetId_RoundTrip` and `TestFloTypeConfigs_SkipsUnknownOpcodes`. `TestLoadFloTypes_RealContent` may SKIP if the real flo.dat path is unavailable; that's fine.

- [ ] **Step 5: Commit**

```
git add pkg/objtype/flotype.go pkg/objtype/flotype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): add minimal FloType binary loader for worldmap

Reads dir/server/flo.dat and exposes debugname → id (GetId) plus
total count. Skips non-name opcodes generically. Avoids porting
the full TS FloType field set since the worldmap packer only
needs the name lookup.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: refColors literal

**Files:**
- Create: `pkg/pack/worldmap/refcolors.go`
- Create: `pkg/pack/worldmap/refcolors_test.go`

**Context:** TS lines 533-613 declare an 80-entry hardcoded `refColors` array of `[edgeColor, fillColor]` u32 pairs, one per floor type. The packWorldmap loop emits `floorcol` packet as `p2(FloType.configs.length)` followed by `p4(refColors[i][0]); p4(refColors[i][1])` for each loaded FloType id. This means goscape MUST keep the same 80-row array in the same order.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/worldmap/refcolors_test.go`:

```go
package worldmap

import "testing"

func TestRefColors_Length(t *testing.T) {
	t.Parallel()
	if got, want := len(refColors), 80; got != want {
		t.Errorf("len(refColors) = %d, want %d", got, want)
	}
}

func TestRefColors_SpotCheck(t *testing.T) {
	t.Parallel()
	// Row 0: cliff      edge=0x00000038 fill=0x009c8f8e
	if refColors[0][0] != 0x00000038 || refColors[0][1] != 0x009c8f8e {
		t.Errorf("row 0 = (%#x, %#x), want (0x00000038, 0x009c8f8e)",
			refColors[0][0], refColors[0][1])
	}
	// Row 4: woodenfloor edge=0x00000000 fill=0x003b1d0c
	if refColors[4][0] != 0x00000000 || refColors[4][1] != 0x003b1d0c {
		t.Errorf("row 4 = (%#x, %#x), want (0x00000000, 0x003b1d0c)",
			refColors[4][0], refColors[4][1])
	}
	// Row 79 (last, hive): edge=0x00b06826 fill=0x0071673f
	if refColors[79][0] != 0x00b06826 || refColors[79][1] != 0x0071673f {
		t.Errorf("row 79 = (%#x, %#x), want (0x00b06826, 0x0071673f)",
			refColors[79][0], refColors[79][1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run TestRefColors -v
```

Expected: FAIL with "refColors undefined" / "package directory does not exist".

- [ ] **Step 3: Write the implementation**

Create `pkg/pack/worldmap/refcolors.go`:

```go
// Package worldmap ports TS tools/pack/map/Worldmap.ts. Builds
// data/pack/mapview/worldmap.jag from per-map binary outputs and
// CSV/font/sprite assets.
package worldmap

// refColors is the 80-entry hardcoded floor-color palette from TS
// Worldmap.ts:533-613. Each row is [edgeColor, fillColor] as u32.
// Ordering matches FloType id ordering — if Content adds a new
// flo before this is in sync, the packer will panic on out-of-
// range access. Update both Content and this table together.
var refColors = [80][2]uint32{
	{0x00000038, 0x009c8f8e}, // cliff
	{0x00000016, 0x004a4242}, // cliff2
	{0x00000022, 0x004a4242}, // cliff3
	{0x0000002d, 0x00817574}, // cliff4
	{0x00000000, 0x003b1d0c}, // woodenfloor
	{0x00000000, 0x0050648d}, // water
	{0x00000000, 0x00206349}, // gungywater
	{0x0000001e, 0x004a4342}, // greyroof
	{0x01500053, 0x00c2c2ba}, // desertroof
	{0x0000001a, 0x00413b3a}, // road
	{0x0000000b, 0x00191616}, // darkstone
	{0x00000000, 0x00403935}, // pebblefloor
	{0x0000a822, 0x00783633}, // redfloor
	{0x0090ec0c, 0x00513a12}, // mudfloor
	{0x0090ec0c, 0x00120d03}, // mudfloor_bump
	{0x00715411, 0x006f4805}, // mudfloor2
	{0x00715411, 0x003c1d01}, // mudfloor2_bump
	{0x03815422, 0x00061789}, // bluefloor
	{0x00000000, 0x00e36116}, // lava
	{0x00000000, 0x004e4e50}, // marble
	{0x00915419, 0x00583a03}, // sandfloor
	{0x00a09419, 0x004d4320}, // l_brownfloor1
	{0x00a09419, 0x00574730}, // l_brownfloor1_bump
	{0x00000000, 0x0039332d}, // cliff_textured
	{0x00b09435, 0x009b9243}, // sand_cliff
	{0x00c06821, 0x005b5441}, // sand_rock
	{0x00000000, 0x00282211}, // oldbrick
	{0x00000000, 0x00333333}, // brick
	{0x01611c14, 0x003b5e0b}, // grass
	{0x0150004f, 0x00c8c0c0}, // ice_overlay
	{0x00a11012, 0x00734c05}, // upass_floor
	{0x00000000, 0x0037312a}, // stone_texture
	{0x0150004a, 0x00aaafb4}, // ice_overlay_blue
	{0x0000001a, 0x00474040}, // road_bridge
	{0x00000000, 0x003b1d0c}, // woodenfloor_bridge
	{0x0080f013, 0x0062420d}, // mud5_overlay
	{0x00000000, 0x00060505}, // black
	{0x03106027, 0x003e516e}, // lightblue
	{0x00000000, 0x0079a0d7}, // water_fountain
	{0x03808427, 0x004e4a82}, // bluefloor2
	{0x03107420, 0x00364c61}, // waterfallblue
	{0xff21542a, 0x00503000}, // invisible
	{0xff21542a, 0x00503000}, // invisible_occ
	{0x0000001a, 0x00474040}, // road_no_occlude
	{0x00000000, 0x003b1d0c}, // woodenfloor_no_occlude
	{0x00000000, 0x00282211}, // oldbrick_no_occlude
	{0x00000000, 0x00333333}, // brick_no_occlude
	{0x01611c14, 0x0036570a}, // grassland
	{0x01011413, 0x00393c07}, // muddygrass
	{0x00c11c15, 0x00403f07}, // vmuddygrass
	{0x0141181f, 0x00556c0e}, // lightgrass
	{0x0110ac21, 0x0065832a}, // sandygrass
	{0x00d10c0f, 0x00282805}, // swamp
	{0x0250e011, 0x0012513d}, // swamp2
	{0x00000027, 0x00605656}, // lightrock
	{0x00000019, 0x004c4444}, // darkrock
	{0x0000000f, 0x00171414}, // verydarkrock
	{0x0150004f, 0x00c2bbba}, // ice
	{0x01500049, 0x00b6b9bf}, // blueice
	{0x01500049, 0x0098a599}, // greenice
	{0x00c0742b, 0x00797343}, // desert1
	{0x00b0a436, 0x009b9243}, // desert2
	{0x0090ec0c, 0x001b1303}, // mud1
	{0x0090b415, 0x006b5d22}, // mud2
	{0x00a11012, 0x0039280b}, // mud3
	{0x00715411, 0x005c2403}, // mud4
	{0x0080f013, 0x00665716}, // mud5
	{0x00b09435, 0x00b48d4e}, // sand
	{0x0090b415, 0x0052471a}, // mud2_skew
	{0x00a11012, 0x006c4a0e}, // mud3_skew
	{0x00715411, 0x003c2701}, // mud4_skew
	{0x00000001, 0x00060505}, // black_rock
	{0x03106027, 0x00435e79}, // dullblue
	{0xffd06027, 0x008d524f}, // purple_pink
	{0x03106027, 0x0043779b}, // lightblue_underlay
	{0x00b0a82d, 0x00a9974a}, // desert_shadow
	{0x0080782f, 0x00886b4d}, // duel_arena
	{0x0080283c, 0x00b47a4e}, // duelarena
	{0x00b06826, 0x0071673f}, // hive
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run TestRefColors -v
```

Expected: PASS for both `TestRefColors_Length` and `TestRefColors_SpotCheck`.

- [ ] **Step 5: Commit**

```
git add pkg/pack/worldmap/refcolors.go pkg/pack/worldmap/refcolors_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack/worldmap): port refColors floor palette literal

80-entry [edge, fill] u32 table from TS Worldmap.ts:533-613.
Ordering must match FloType id ordering at load time.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: processCsv and parseLabels

**Files:**
- Create: `pkg/pack/worldmap/csv.go`
- Create: `pkg/pack/worldmap/csv_test.go`

**Context:** TS `processCsv` (lines 60-101) handles two row formats:
1. Range: `fromLevel_fromMx_fromMz_fromLx_fromLz,toLevel_toMx_toMz_toLx_toLz` — expands a rectangle of coords (only `fromLevel` is used; toLevel is discarded as `_toLevel`).
2. Single zone: `level_mx_mz_lx_lz` — expands to an 8×8 tile block starting at (lx, lz).

Output is a `Set<number>` of packed coords (`CoordGrid.packCoord(level, x, z)`).

Validations log warnings (do not fail):
- Range alignment: `fromLx % 8 !== 0` OR `fromLz % 8 !== 0` OR `toLx % 8 !== 7` OR `toLz % 8 !== 7` OR `fromMx > toMx` OR `fromMz > toMz` OR (`fromMx <= toMx && fromMz <= toMz && (fromLx > toLx || fromLz > toLz)`)
- Overlap: result already contains the coord being added.

Lines starting with `//` or empty lines are skipped.

TS `parseLabels` reads `labels.txt`, splits on `\r?\n`, keeps only lines starting with `=`, then splits each by `,` into `[text, x, z, type]`. Used to emit `labels.dat`.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/worldmap/csv_test.go`:

```go
package worldmap

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

func TestProcessCsv_SingleZoneExpands8x8(t *testing.T) {
	t.Parallel()
	got := processCsv([]string{"0_50_50_0_0"}, "test", slog.Default())
	if want := 64; len(got) != want {
		t.Fatalf("len = %d, want %d", len(got), want)
	}
	// Spot-check the corners.
	for _, c := range []struct{ x, z int }{
		{50 << 6, 50 << 6},                 // (lx=0, lz=0)
		{(50 << 6) + 7, (50 << 6) + 7},     // (lx=7, lz=7)
	} {
		if _, ok := got[coordgrid.PackCoord(0, c.x, c.z)]; !ok {
			t.Errorf("missing coord (0, %d, %d)", c.x, c.z)
		}
	}
}

func TestProcessCsv_RangeExpandsRectangle(t *testing.T) {
	t.Parallel()
	// Aligned range: (mx=10, mz=10) → (mx=10, mz=10) within one mapsquare.
	got := processCsv([]string{"0_10_10_0_0,0_10_10_7_7"}, "test", slog.Default())
	if want := 64; len(got) != want {
		t.Fatalf("len = %d, want %d", len(got), want)
	}
}

func TestProcessCsv_CommentAndEmpty(t *testing.T) {
	t.Parallel()
	got := processCsv([]string{"// comment", "", "0_0_0_0_0"}, "test", slog.Default())
	if want := 64; len(got) != want {
		t.Fatalf("len = %d, want %d", len(got), want)
	}
}

func TestProcessCsv_AlignmentWarning(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	// fromLx=1 not divisible by 8 → warning.
	_ = processCsv([]string{"0_5_5_1_0,0_5_5_7_7"}, "multiway", lg)
	if !strings.Contains(buf.String(), "not aligned") {
		t.Errorf("expected alignment warning, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "multiway") {
		t.Errorf("expected name in warning, got %q", buf.String())
	}
}

func TestProcessCsv_OverlapWarning(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	_ = processCsv([]string{"0_0_0_0_0", "0_0_0_0_0"}, "ignore", lg)
	if !strings.Contains(buf.String(), "Overlapping") {
		t.Errorf("expected overlap warning, got %q", buf.String())
	}
}

func TestParseLabels_FiltersAndParses(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"// comment",
		"=Lumbridge,3222,3218,0",
		"not_a_label_line",
		"=Falador,2965,3380,1",
		"",
	}, "\n")
	got := parseLabels(src)
	if want := 2; len(got) != want {
		t.Fatalf("len = %d, want %d", len(got), want)
	}
	if got[0].Text != "Lumbridge" || got[0].X != 3222 || got[0].Z != 3218 || got[0].Type != 0 {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Text != "Falador" || got[1].X != 2965 || got[1].Z != 3380 || got[1].Type != 1 {
		t.Errorf("got[1] = %+v", got[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run "TestProcessCsv|TestParseLabels" -v
```

Expected: FAIL with "processCsv undefined" / "parseLabels undefined".

- [ ] **Step 3: Write the implementation**

Create `pkg/pack/worldmap/csv.go`:

```go
package worldmap

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// Label is one parsed entry from labels.txt.
type Label struct {
	Text string
	X    int
	Z    int
	Type int
}

// processCsv expands a multiway / free2play / ignore CSV body into
// a set of packed CoordGrid coords. Lines starting with "//" or
// empty are skipped. Warnings are logged via lg; this function
// never returns an error (TS parity with printWarning).
//
// Two row formats:
//   - "level_mx_mz_lx_lz"                 — one 8×8 tile block
//   - "fromLine,toLine" (5 fields each)   — rectangle expansion
//
// In the range form only fromLevel is used; toLevel is discarded.
func processCsv(lines []string, name string, lg *slog.Logger) map[int]struct{} {
	result := make(map[int]struct{})
	for _, line := range lines {
		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) == 2 {
			fromParts := strings.Split(parts[0], "_")
			toParts := strings.Split(parts[1], "_")
			if len(fromParts) != 5 || len(toParts) != 5 {
				continue
			}
			fromLevel := atoi(fromParts[0])
			fromMx := atoi(fromParts[1])
			fromMz := atoi(fromParts[2])
			fromLx := atoi(fromParts[3])
			fromLz := atoi(fromParts[4])
			toMx := atoi(toParts[1])
			toMz := atoi(toParts[2])
			toLx := atoi(toParts[3])
			toLz := atoi(toParts[4])

			if fromLx%8 != 0 || fromLz%8 != 0 || toLx%8 != 7 || toLz%8 != 7 ||
				fromMx > toMx || fromMz > toMz ||
				(fromMx <= toMx && fromMz <= toMz && (fromLx > toLx || fromLz > toLz)) {
				lg.Warn("map not aligned to a zone", "name", name, "row", line)
			}

			startX := (fromMx << 6) + fromLx
			startZ := (fromMz << 6) + fromLz
			endX := (toMx << 6) + toLx
			endZ := (toMz << 6) + toLz
			for x := startX; x <= endX; x++ {
				for z := startZ; z <= endZ; z++ {
					packed := coordgrid.PackCoord(fromLevel, x, z)
					if _, dup := result[packed]; dup {
						lg.Warn("Overlapping map", "name", name, "row", line)
					}
					result[packed] = struct{}{}
				}
			}
		} else {
			fields := strings.Split(line, "_")
			if len(fields) != 5 {
				continue
			}
			level := atoi(fields[0])
			mx := atoi(fields[1])
			mz := atoi(fields[2])
			lx := atoi(fields[3])
			lz := atoi(fields[4])
			for i := 0; i < 8; i++ {
				for j := 0; j < 8; j++ {
					result[coordgrid.PackCoord(level, (mx<<6)+lx+i, (mz<<6)+lz+j)] = struct{}{}
				}
			}
		}
	}
	return result
}

// parseLabels filters non-"=" lines and parses each remaining row
// as "text,x,z,type" (4 comma-separated fields after stripping
// the leading "=").
func parseLabels(src string) []Label {
	rawLines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	out := make([]Label, 0, len(rawLines))
	for _, line := range rawLines {
		if !strings.HasPrefix(line, "=") {
			continue
		}
		fields := strings.SplitN(line[1:], ",", 4)
		if len(fields) != 4 {
			continue
		}
		out = append(out, Label{
			Text: fields[0],
			X:    atoi(fields[1]),
			Z:    atoi(fields[2]),
			Type: atoi(fields[3]),
		})
	}
	return out
}

// atoi parses base-10 ints. Returns 0 on parse error (TS parseInt
// returns NaN on invalid input; our callers don't distinguish).
func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run "TestProcessCsv|TestParseLabels" -v
```

Expected: PASS for all 6 tests.

- [ ] **Step 5: Commit**

```
git add pkg/pack/worldmap/csv.go pkg/pack/worldmap/csv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack/worldmap): port processCsv and parseLabels helpers

processCsv expands multiway/free2play/ignore CSVs into packed
coord sets; logs alignment + overlap warnings via slog. parseLabels
filters labels.txt to "=" prefixed rows and parses 4-field CSV.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: packWater + unpackCoord

**Files:**
- Create: `pkg/pack/worldmap/worldmap.go` (initial skeleton with only these two helpers + a placeholder `Pack` stub that returns `errors.New("not implemented")`)
- Create: `pkg/pack/worldmap/worldmap_test.go`

**Context:** `packWater` (TS:15-28) emits one map's worth of "ocean" tiles into the underlay+overlay packets. Layout:
- underlay: `p1(mx); p1(mz); for 4096: p1(1 + FloType.getId('muddygrass'))`
- overlay: `p1(mx); p1(mz); for 4096: p1(1 + FloType.getId('water')); p1(0)`

Byte counts: underlay grows by `2 + 4096 = 4098`; overlay grows by `2 + 4096*2 = 8194`.

`unpackCoord` (TS:53-58) extracts level/x/z from a packed-coord int:
- `z = packed & 0x3f`
- `x = (packed >> 6) & 0x3f`
- `level = (packed >> 12) & 0x3`

The (x, z) here are LOCAL tile coords inside a mapsquare (0..63), not world coords — it's used in the loc-file loop to decode `(coord += offset - 1)` deltas where coord is the packed (level, lx, lz) form.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/worldmap/worldmap_test.go`:

```go
package worldmap

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestUnpackCoord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		level, x, z int
	}{
		{0, 0, 0},
		{0, 63, 63},
		{1, 30, 17},
		{3, 63, 63},
	} {
		packed := (tc.level << 12) | (tc.x << 6) | tc.z
		level, x, z := unpackCoord(packed)
		if level != tc.level || x != tc.x || z != tc.z {
			t.Errorf("unpackCoord(%#x) = (%d, %d, %d), want (%d, %d, %d)",
				packed, level, x, z, tc.level, tc.x, tc.z)
		}
	}
}

func TestPackWater_ByteLayout(t *testing.T) {
	t.Parallel()

	// Build a tiny FloTypeConfigs with known ids for muddygrass + water.
	flo := &objtype.FloTypeConfigs{
		ConfigNames: map[string]int{
			"muddygrass": 7,
			"water":      11,
		},
	}

	underlay := packet2.Alloc(1)
	defer underlay.Release()
	overlay := packet2.Alloc(1)
	defer overlay.Release()

	packWater(flo, underlay, overlay, 42, 56)

	if got, want := underlay.Length(), 2+4096; got != want {
		t.Errorf("underlay length = %d, want %d", got, want)
	}
	if got, want := overlay.Length(), 2+4096*2; got != want {
		t.Errorf("overlay length = %d, want %d", got, want)
	}

	// Re-read header bytes.
	underlay.Pos = 0
	if underlay.G1() != 42 || underlay.G1() != 56 {
		t.Errorf("underlay header bytes wrong")
	}
	// Every body byte must be 1 + getId("muddygrass") = 8.
	for i := 0; i < 4096; i++ {
		if got := underlay.G1(); got != 8 {
			t.Fatalf("underlay body byte %d = %d, want 8", i, got)
			break
		}
	}

	overlay.Pos = 0
	if overlay.G1() != 42 || overlay.G1() != 56 {
		t.Errorf("overlay header bytes wrong")
	}
	for i := 0; i < 4096; i++ {
		v := overlay.G1()
		z := overlay.G1()
		if v != 12 || z != 0 {
			t.Fatalf("overlay body pair %d = (%d, %d), want (12, 0)", i, v, z)
			break
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run "TestUnpackCoord|TestPackWater" -v
```

Expected: FAIL with "unpackCoord undefined" / "packWater undefined".

- [ ] **Step 3: Write the implementation**

Create `pkg/pack/worldmap/worldmap.go`:

```go
package worldmap

import (
	"errors"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// Pack is the worldmap packer entry point. See package doc.
//
// This is a stub. Implementation lands in Task 7.
func Pack(srcDir, outDir string) error {
	_ = srcDir
	_ = outDir
	return errors.New("worldmap.Pack: not implemented")
}

// packWater appends one "ocean" map square (mx, mz) to underlay
// and overlay. Mirrors TS Worldmap.ts:15-28.
//
// underlay grows by 2 + 4096 = 4098 bytes.
// overlay  grows by 2 + 4096*2 = 8194 bytes.
func packWater(flo *objtype.FloTypeConfigs, underlay, overlay *packet2.Packet, mx, mz int) {
	muddyId := uint8(1 + flo.GetId("muddygrass"))
	waterId := uint8(1 + flo.GetId("water"))

	underlay.P1(uint8(mx))
	underlay.P1(uint8(mz))
	overlay.P1(uint8(mx))
	overlay.P1(uint8(mz))

	for i := 0; i < 4096; i++ {
		underlay.P1(muddyId)
		overlay.P1(waterId)
		overlay.P1(0)
	}
}

// unpackCoord extracts (level, x, z) from a packed local-coord
// int. x and z are LOCAL mapsquare coords (0..63). Mirrors TS
// Worldmap.ts:53-58.
func unpackCoord(packed int) (level, x, z int) {
	z = packed & 0x3f
	x = (packed >> 6) & 0x3f
	level = (packed >> 12) & 0x3
	return
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run "TestUnpackCoord|TestPackWater" -v
```

Expected: PASS for both.

- [ ] **Step 5: Commit**

```
git add pkg/pack/worldmap/worldmap.go pkg/pack/worldmap/worldmap_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack/worldmap): port packWater + unpackCoord helpers

packWater appends one ocean mapsquare to the underlay+overlay
packets (4098 + 8194 bytes). unpackCoord extracts (level, lx, lz)
from a packed local-coord int per TS Worldmap.ts:53-58.

Pack() is a stub returning "not implemented" until Task 7.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Per-map binary processor

**Files:**
- Modify: `pkg/pack/worldmap/worldmap.go` (add `processMap`)
- Modify: `pkg/pack/worldmap/worldmap_test.go` (add `TestProcessMap_*` tests)

**Context:** Per-map work in TS:114-510 reads four binary files per mapsquare and appends to seven output packets. We refactor this into a `processMap` function that takes context (loaded types, multi/free/ignore sets) plus the seven output packets.

The function:
1. Parses `m{mx}_{mz}` (land file): for each (level, x, z) in 4×64×64, reads a sequence of opcodes terminated by 0 or 1, decoding to `overlayIds`, `overlayShape`, `overlayRotation`, `flags`, `underlayIds`. Opcode 1 has a 1-byte payload.
2. Emits underlay+overlay bytes for the selected level (with the bridge flag adjustment).
3. Parses `l{mx}_{mz}` (loc file): GSmartS-delimited (locId, coord, info) triples. For each loc whose `mapscene != 22`, decodes `info` into `(shape, angle)` and updates `walls/mapscenes/mapfunctions` 3D arrays. Wall codes are a 28-element shape × angle × active matrix (see TS:303-328).
4. Emits loc bytes for the selected level.
5. Parses `o{mx}_{mz}` (obj file) if non-empty: streams (pos, count, [objId, objCount]×count) records, marks any tile that received at least one obj.
6. Emits obj bytes IF the file was non-empty.
7. Parses `n{mx}_{mz}` (npc file) if non-empty: similar to obj, marks tiles whose NpcType has `Minimap=true`.
8. Emits npc bytes IF the file was non-empty.
9. Computes `multi/free` tile masks against the prebuilt sets and emits those if non-empty.

A special-case: `mx==33 && mz>=71 && mz<=73` overrides selected level to 1 (underground pass).

**TS-divergence to be aware of:** Go integer arithmetic is signed; mask/shift in `unpackCoord` uses `0x3f` and `0x3` so values stay in range. The `actualLevel = (bridged ? 1 : 0) + level` calculation can yield `> 3` if level=2|3 and bridged=true — TS array access happily returns undefined (then `-1` check above coerces to "skip"). Go fixed-size arrays would panic. Mirror TS behaviour by bounds-checking before array read.

Walls/shape mapping reference — `pkg/pathfinder/loc.Shape` constants used:
- `ShapeWallStraight` (0) → walls = 1+angle (+4 if active)
- `ShapeWallL` (2) → walls = 9+angle (+4 if active)
- `ShapeWallDecorStraightNoOffset` (4) → walls = 17+angle (+4 if active)
- `ShapeWallDiagonal` (9) → walls = 25+(angle%2) (+2 if active)

This function is large (~150 LOC); split into helper sub-functions if it exceeds 200 lines.

- [ ] **Step 1: Write the failing test**

Append to `pkg/pack/worldmap/worldmap_test.go`:

```go
func TestProcessMap_EmptyLandFile_ProducesHeaderOnlyBytes(t *testing.T) {
	t.Parallel()

	// Synthesise a "land" file where every (level, x, z) immediately
	// hits opcode 0 — all defaults retained.
	land := packet2.Alloc(1)
	defer land.Release()
	for i := 0; i < 4*64*64; i++ {
		land.P1(0)
	}

	// Empty loc/obj/npc.
	loc := packet2.Alloc(1)
	defer loc.Release()
	loc.P1(0) // locIdOffset = 0 → loop exits immediately
	obj := packet2.Alloc(1)
	defer obj.Release()
	npc := packet2.Alloc(1)
	defer npc.Release()

	flo := &objtype.FloTypeConfigs{ConfigNames: map[string]int{"muddygrass": 0, "water": 1}}
	locTypes := &objtype.LocTypeConfigs{Configs: nil}
	npcTypes := &objtype.NPCTypeConfigs{Configs: nil}

	out := newMapPackets()
	defer out.release()
	ctx := mapCtx{
		flo:      flo,
		locTypes: locTypes,
		npcTypes: npcTypes,
		multimap: map[int]struct{}{},
		freemap:  map[int]struct{}{},
	}

	if err := processMap(ctx, out, 50, 50, land, loc, obj, npc); err != nil {
		t.Fatalf("processMap: %v", err)
	}

	// All overlay tiles default to overlayIds=-1 → byte 0 (no overlay).
	// underlay defaults to -1 → byte 0.
	// Headers: 2 bytes mx+mz on each of underlay, overlay, loc.
	// underlay: 2 + 4096 = 4098
	// overlay : 2 + 4096 = 4096 (one byte per tile, the "no overlay" case)
	// loc     : 2 + 4096 = 4098 (one trailing 0 per tile)
	if got, want := out.underlay.Length(), 2+4096; got != want {
		t.Errorf("underlay length = %d, want %d", got, want)
	}
	if got, want := out.overlay.Length(), 2+4096; got != want {
		t.Errorf("overlay length = %d, want %d", got, want)
	}
	if got, want := out.loc.Length(), 2+4096; got != want {
		t.Errorf("loc length = %d, want %d", got, want)
	}
	// obj and npc should be empty (input was empty).
	if got := out.obj.Length(); got != 0 {
		t.Errorf("obj length = %d, want 0 (empty input)", got)
	}
	if got := out.npc.Length(); got != 0 {
		t.Errorf("npc length = %d, want 0 (empty input)", got)
	}
}

func TestProcessMap_UndergroundPassLevelOverride(t *testing.T) {
	t.Parallel()
	// mx=33, mz=72 (in [71,73]) — selected level becomes 1.
	// Encode a land file where level=1, (0,0) has underlay code 82
	// (= underlayId 82-81 = 1) and the rest are 0.
	land := packet2.Alloc(1)
	defer land.Release()
	for level := 0; level < 4; level++ {
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				if level == 1 && x == 0 && z == 0 {
					land.P1(82) // underlay opcode (>81)
					land.P1(0)
				} else {
					land.P1(0)
				}
			}
		}
	}
	loc := packet2.Alloc(1)
	defer loc.Release()
	loc.P1(0)
	obj := packet2.Alloc(1)
	defer obj.Release()
	npc := packet2.Alloc(1)
	defer npc.Release()

	flo := &objtype.FloTypeConfigs{ConfigNames: map[string]int{"muddygrass": 0, "water": 1}}
	out := newMapPackets()
	defer out.release()
	ctx := mapCtx{
		flo:      flo,
		locTypes: &objtype.LocTypeConfigs{},
		npcTypes: &objtype.NPCTypeConfigs{},
		multimap: map[int]struct{}{},
		freemap:  map[int]struct{}{},
	}
	if err := processMap(ctx, out, 33, 72, land, loc, obj, npc); err != nil {
		t.Fatalf("processMap: %v", err)
	}

	// underlay tile (0,0) at level 1 should be byte 1 (= underlayId).
	out.underlay.Pos = 2 // skip header
	if got := out.underlay.G1(); got != 1 {
		t.Errorf("underlay[0][0] = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run TestProcessMap -v
```

Expected: FAIL with "processMap undefined" / "newMapPackets undefined" / "mapCtx undefined".

- [ ] **Step 3: Write the implementation**

Append to `pkg/pack/worldmap/worldmap.go`:

```go
import (
	// add to existing import block:
	pf "github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// mapPackets bundles the seven per-stage output packets that the
// per-map loop appends to.
type mapPackets struct {
	underlay *packet2.Packet
	overlay  *packet2.Packet
	loc      *packet2.Packet
	obj      *packet2.Packet
	npc      *packet2.Packet
	multi    *packet2.Packet
	free     *packet2.Packet
}

func newMapPackets() *mapPackets {
	return &mapPackets{
		underlay: packet2.Alloc(1),
		overlay:  packet2.Alloc(1),
		loc:      packet2.Alloc(1),
		obj:      packet2.Alloc(1),
		npc:      packet2.Alloc(1),
		multi:    packet2.Alloc(1),
		free:     packet2.Alloc(1),
	}
}

func (m *mapPackets) release() {
	m.underlay.Release()
	m.overlay.Release()
	m.loc.Release()
	m.obj.Release()
	m.npc.Release()
	m.multi.Release()
	m.free.Release()
}

// mapCtx is the immutable per-Pack context passed to processMap.
type mapCtx struct {
	flo      *objtype.FloTypeConfigs
	locTypes *objtype.LocTypeConfigs
	npcTypes *objtype.NPCTypeConfigs
	multimap map[int]struct{}
	freemap  map[int]struct{}
}

// processMap appends one (mx, mz) mapsquare's worth of bytes to
// the seven output packets. Mirrors the body of the TS for-loop
// at Worldmap.ts:114-510.
//
// land/loc/obj/npc are the binary mapsquare files. obj and npc
// may be empty (Length()==0); in that case no bytes are emitted
// for those stages.
func processMap(ctx mapCtx, out *mapPackets, mx, mz int, land, loc, obj, npc *packet2.Packet) error {
	level := 0
	if mx == 33 && mz >= 71 && mz <= 73 {
		level = 1 // exception for underground pass
	}

	// --- land file ---
	var (
		flags          [4][64][64]int
		overlayIds     [4][64][64]int
		overlayShape   [4][64][64]int
		overlayRot     [4][64][64]int
		underlayIds    [4][64][64]int
	)
	for l := 0; l < 4; l++ {
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				overlayIds[l][x][z] = -1
				underlayIds[l][x][z] = -1
			}
		}
	}

	for l := 0; l < 4; l++ {
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				for {
					op := int(land.G1())
					if op == 0 {
						break
					}
					if op == 1 {
						_ = land.G1()
						break
					}
					switch {
					case op <= 49:
						overlayIds[l][x][z] = int(land.G1())
						overlayShape[l][x][z] = (op - 2) / 4
						overlayRot[l][x][z] = (op - 2) & 0x3
					case op <= 81:
						flags[l][x][z] = op - 49
					default:
						underlayIds[l][x][z] = op - 81
					}
				}
			}
		}
	}

	out.overlay.P1(uint8(mx))
	out.overlay.P1(uint8(mz))
	out.underlay.P1(uint8(mx))
	out.underlay.P1(uint8(mz))
	for x := 0; x < 64; x++ {
		for z := 0; z < 64; z++ {
			bridged := (flags[1][x][z] & 0x2) == 2
			actualLevel := level
			if bridged {
				actualLevel = 1 + level
			}
			if actualLevel < 0 || actualLevel > 3 {
				out.overlay.P1(0)
				out.underlay.P1(0)
				continue
			}
			if overlayIds[actualLevel][x][z] != -1 {
				out.overlay.P1(uint8(overlayIds[actualLevel][x][z]))
				out.overlay.P1(uint8(overlayRot[actualLevel][x][z] + (overlayShape[actualLevel][x][z] << 2)))
			} else {
				out.overlay.P1(0)
			}
			if underlayIds[actualLevel][x][z] != -1 {
				out.underlay.P1(uint8(underlayIds[actualLevel][x][z]))
			} else {
				out.underlay.P1(0)
			}
		}
	}

	// --- loc file ---
	var (
		walls         [4][64][64]int
		mapscenes     [4][64][64]int
		mapfunctions  [4][64][64]int
	)
	for l := 0; l < 4; l++ {
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				walls[l][x][z] = -1
				mapscenes[l][x][z] = -1
				mapfunctions[l][x][z] = -1
			}
		}
	}

	locId := -1
	locIdOffset := int(loc.GSmartS())
	for locIdOffset != 0 {
		locId += locIdOffset

		coord := 0
		coordOffset := int(loc.GSmartS())
		for coordOffset != 0 {
			coord += coordOffset - 1
			locLevel, x, z := unpackCoord(coord)
			info := int(loc.G1())
			coordOffset = int(loc.GSmartS())

			var bridgedFlag int
			if locLevel == 1 {
				bridgedFlag = flags[locLevel][x][z] & 0x2
			} else {
				bridgedFlag = flags[1][x][z] & 0x2
			}
			actualLevel := locLevel
			if bridgedFlag == 2 {
				actualLevel = locLevel - 1
			}
			if actualLevel < 0 {
				continue
			}

			var locType *objtype.LocType
			if locId >= 0 && locId < len(ctx.locTypes.Configs) {
				locType = ctx.locTypes.Configs[locId]
			}
			if locType == nil {
				continue
			}
			shape := info >> 2
			angle := info & 0x3

			if locType.MapScene == 22 {
				continue
			}

			if walls[actualLevel][x][z] == -1 {
				switch pf.Shape(shape) {
				case pf.ShapeWallStraight:
					w := 1 + angle
					if locType.Active == 1 {
						w += 4
					}
					walls[actualLevel][x][z] = w
				case pf.ShapeWallL:
					w := 9 + angle
					if locType.Active == 1 {
						w += 4
					}
					walls[actualLevel][x][z] = w
				case pf.ShapeWallDecorStraightNoOffset:
					w := 17 + angle
					if locType.Active == 1 {
						w += 4
					}
					walls[actualLevel][x][z] = w
				case pf.ShapeWallDiagonal:
					w := 25 + (angle % 2)
					if locType.Active == 1 {
						w += 2
					}
					walls[actualLevel][x][z] = w
				}
			}
			if locType.MapScene != -1 {
				mapscenes[actualLevel][x][z] = locType.MapScene
			}
			if locType.MapFunction != -1 {
				mapfunctions[actualLevel][x][z] = locType.MapFunction
			}
		}
		locIdOffset = int(loc.GSmartS())
	}

	out.loc.P1(uint8(mx))
	out.loc.P1(uint8(mz))
	for x := 0; x < 64; x++ {
		for z := 0; z < 64; z++ {
			if walls[level][x][z] != -1 {
				out.loc.P1(uint8(walls[level][x][z]))
			}
			if mapscenes[level][x][z] != -1 {
				out.loc.P1(uint8(29 + mapscenes[level][x][z]))
			}
			if mapfunctions[level][x][z] != -1 {
				out.loc.P1(uint8(160 + mapfunctions[level][x][z]))
			}
			out.loc.P1(0)
		}
	}

	// --- obj file ---
	if obj.Length() > 0 {
		var objs [4][64][64]int
		for l := 0; l < 4; l++ {
			for x := 0; x < 64; x++ {
				for z := 0; z < 64; z++ {
					objs[l][x][z] = -1
				}
			}
		}
		for obj.Unused() > 0 {
			pos := int(obj.G2())
			lvl := (pos >> 12) & 0x3
			lx := (pos >> 6) & 0x3f
			lz := pos & 0x3f
			count := int(obj.G1())
			for j := 0; j < count; j++ {
				id := int(obj.G2())
				_ = obj.G1() // count, discarded
				objs[lvl][lx][lz] = id
			}
		}
		out.obj.P1(uint8(mx))
		out.obj.P1(uint8(mz))
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				out.obj.PBool(objs[level][x][z] != -1)
			}
		}
	}

	// --- npc file ---
	if npc.Length() > 0 {
		var npcs [4][64][64]int
		for l := 0; l < 4; l++ {
			for x := 0; x < 64; x++ {
				for z := 0; z < 64; z++ {
					npcs[l][x][z] = -1
				}
			}
		}
		for npc.Unused() > 0 {
			pos := int(npc.G2())
			lvl := (pos >> 12) & 0x3
			lx := (pos >> 6) & 0x3f
			lz := pos & 0x3f
			count := int(npc.G1())
			for j := 0; j < count; j++ {
				id := int(npc.G2())
				if id >= 0 && id < len(ctx.npcTypes.Configs) && ctx.npcTypes.Configs[id] != nil && ctx.npcTypes.Configs[id].Minimap {
					npcs[lvl][lx][lz] = id
				}
			}
		}
		out.npc.P1(uint8(mx))
		out.npc.P1(uint8(mz))
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				out.npc.PBool(npcs[level][x][z] != -1)
			}
		}
	}

	// --- multi / free tile masks ---
	hasMulti := false
	hasFree := false
	var multiTiles [4][64][64]bool
	var freeTiles [4][64][64]bool
	for l := 0; l < 4; l++ {
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				worldX := (mx << 6) + x
				worldZ := (mz << 6) + z
				packed := coordgrid.PackCoord(l, worldX, worldZ)
				if _, ok := ctx.multimap[packed]; ok {
					multiTiles[l][x][z] = true
					hasMulti = true
				}
				if _, ok := ctx.freemap[packed]; ok {
					freeTiles[l][x][z] = true
					hasFree = true
				}
			}
		}
	}
	if hasMulti {
		out.multi.P1(uint8(mx))
		out.multi.P1(uint8(mz))
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				out.multi.PBool(multiTiles[0][x][z])
			}
		}
	}
	if hasFree {
		out.free.P1(uint8(mx))
		out.free.P1(uint8(mz))
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				out.free.PBool(freeTiles[0][x][z])
			}
		}
	}
	return nil
}

```

Also expand the import block at the top of `worldmap.go` to include `coordgrid`, `objtype`, and the pathfinder shape package (the file initially only imports `errors` + `packet2`):

```go
import (
	"errors"

	"github.com/zsrv/goscape/pkg/coordgrid"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	pf "github.com/zsrv/goscape/pkg/pathfinder/loc"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run TestProcessMap -v
```

Expected: PASS for both `TestProcessMap_EmptyLandFile_ProducesHeaderOnlyBytes` and `TestProcessMap_UndergroundPassLevelOverride`. If `Unused()` returns negative for an empty packet, change the loop guard to `obj.Unused() > 0` (already used).

- [ ] **Step 5: Run the full package test suite**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -v
```

Expected: all prior tests still PASS.

- [ ] **Step 6: Commit**

```
git add pkg/pack/worldmap/worldmap.go pkg/pack/worldmap/worldmap_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack/worldmap): port per-map binary processor

processMap reads one (mx,mz) mapsquare's land/loc/obj/npc binaries
and appends to the seven output packets (underlay/overlay/loc/obj/
npc/multi/free). Includes underground-pass level=1 override.
Wall-shape codes match LocShape.{WALL_STRAIGHT,WALL_L,
WALLDECOR_STRAIGHT_NOOFFSET,WALL_DIAGONAL}. obj/npc only emit if
the input file is non-empty.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Pack() entry point

**Files:**
- Modify: `pkg/pack/worldmap/worldmap.go` (replace the `Pack` stub)
- Modify: `pkg/pack/worldmap/worldmap_test.go` (add `TestPack_NoMapsDir_NoOp`)

**Context:** TS `packWorldmap` (top-level body at lines 30-680). Steps:
1. If `outDir/server/maps` does not exist → return nil.
2. Load FloType, LocType, NpcType from `outDir`.
3. Read multiway.csv, free2play.csv, ignore.csv from `srcDir/maps/` → call processCsv.
4. List `outDir/server/maps/m*` files (already sorted by `os.ReadDir`).
5. For each map: parse `(mx, mz)` from filename `m{mx}_{mz}`; skip if in ignoremap; call `processMap`.
6. Append 16 hardcoded `packWater` calls.
7. Build floorcol packet: `p2(len(flo.Configs))` then per-id `p4(refColors[i][0]); p4(refColors[i][1])`.
8. Convert 4 PNG sprites via `pixpack.ConvertImage` (mapscene, mapfunction, b12, mapdots).
9. Load 8 font .fm files via `packet.Load(path, false)`.
10. Parse labels.txt via `parseLabels`, emit labels packet: `p2(count)` then per-label `pjstrLF(text); p2(x); p2(z); p1(type)`.

**TS pjstr default terminator** — TS `Packet.pjstr(str, terminator=10)` defaults to LF (10), so use `PJStrLF`.

11. Build jagfile, write 22 entries in fixed order (TS:657-678), save to `outDir/mapview/worldmap.jag`. `mkdir -p outDir/mapview` first.

**Sort note (NAI-WORLDMAP-D-READDIR-SORTED):** `os.ReadDir` returns sorted entries, which gives a deterministic output ordering. TS `fs.readdirSync` is filesystem-order. Document this in a code comment.

- [ ] **Step 1: Write the failing test**

Append to `pkg/pack/worldmap/worldmap_test.go`:

```go
func TestPack_NoMapsDir_NoOp(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	src := t.TempDir()
	// outDir/server/maps does not exist → TS parity early-return.
	if err := Pack(src, tmp); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	// Nothing should be created.
	if _, err := os.Stat(filepath.Join(tmp, "mapview", "worldmap.jag")); err == nil {
		t.Errorf("worldmap.jag created despite missing server/maps")
	}
}
```

Add the imports at the top of `worldmap_test.go` if not already present:
```go
import (
	"os"
	"path/filepath"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run TestPack_NoMapsDir_NoOp -v
```

Expected: FAIL with the stub's `"worldmap.Pack: not implemented"` error.

- [ ] **Step 3: Replace the Pack stub**

Replace the `Pack` function in `pkg/pack/worldmap/worldmap.go`:

```go
// Pack builds outDir/mapview/worldmap.jag from server-side map
// outputs (outDir/server/maps/{m,l,o,n}*) plus fonts/sprites/CSVs
// in srcDir. Returns nil if outDir/server/maps is missing (TS
// parity with Worldmap.ts:31-33).
func Pack(srcDir, outDir string) error {
	mapsDir := filepath.Join(outDir, "server", "maps")
	if _, err := os.Stat(mapsDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat %s: %w", mapsDir, err)
	}

	lg := slog.Default().With("pack", "worldmap")

	flo, err := objtype.LoadFloTypes(outDir)
	if err != nil {
		return fmt.Errorf("LoadFloTypes: %w", err)
	}
	locTypes, err := objtype.LoadLocTypes(outDir)
	if err != nil {
		return fmt.Errorf("LoadLocTypes: %w", err)
	}
	npcTypes, err := objtype.LoadNPCTypes(outDir)
	if err != nil {
		return fmt.Errorf("LoadNPCTypes: %w", err)
	}

	readCsv := func(name string) ([]string, error) {
		path := filepath.Join(srcDir, "maps", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		return strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n"), nil
	}

	multilines, err := readCsv("multiway.csv")
	if err != nil {
		return err
	}
	multimap := processCsv(multilines, "multiway", lg)

	freeLines, err := readCsv("free2play.csv")
	if err != nil {
		return err
	}
	freemap := processCsv(freeLines, "free", lg)

	ignoreLines, err := readCsv("ignore.csv")
	if err != nil {
		return err
	}
	ignoremap := processCsv(ignoreLines, "ignore", lg)

	ctx := mapCtx{
		flo:      flo,
		locTypes: locTypes,
		npcTypes: npcTypes,
		multimap: multimap,
		freemap:  freemap,
	}
	out := newMapPackets()
	defer out.release()

	// NAI-WORLDMAP-D-READDIR-SORTED: os.ReadDir returns sorted
	// entries. TS fs.readdirSync is filesystem-order. Consequence:
	// per-(mx,mz) entries appear in different positions in the
	// underlay/overlay/loc/obj/npc/multi/free packets than a TS
	// build would produce. No checked-in TS reference exists to
	// pin against, so this is accepted as more deterministic.
	entries, err := os.ReadDir(mapsDir)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", mapsDir, err)
	}
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasPrefix(name, "m") {
			continue
		}
		parts := strings.Split(name[1:], "_")
		if len(parts) != 2 {
			continue
		}
		mx, err1 := strconv.Atoi(parts[0])
		mz, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}
		if _, skip := ignoremap[coordgrid.PackCoord(0, mx<<6, mz<<6)]; skip {
			continue
		}
		land, err := packet2.Load(filepath.Join(mapsDir, name), false)
		if err != nil {
			return fmt.Errorf("load %s: %w", name, err)
		}
		loc, err := packet2.Load(filepath.Join(mapsDir, fmt.Sprintf("l%d_%d", mx, mz)), false)
		if err != nil {
			return fmt.Errorf("load l%d_%d: %w", mx, mz, err)
		}
		obj, err := packet2.Load(filepath.Join(mapsDir, fmt.Sprintf("o%d_%d", mx, mz)), false)
		if err != nil {
			return fmt.Errorf("load o%d_%d: %w", mx, mz, err)
		}
		npc, err := packet2.Load(filepath.Join(mapsDir, fmt.Sprintf("n%d_%d", mx, mz)), false)
		if err != nil {
			return fmt.Errorf("load n%d_%d: %w", mx, mz, err)
		}
		if err := processMap(ctx, out, mx, mz, land, loc, obj, npc); err != nil {
			return fmt.Errorf("processMap %d,%d: %w", mx, mz, err)
		}
		land.Release()
		loc.Release()
		obj.Release()
		npc.Release()
	}

	// Hardcoded water tiles (TS:513-528)
	for _, mxmz := range [][2]int{
		{39, 56}, {40, 56},
		{42, 44}, {42, 45}, {42, 46}, {42, 47}, {42, 48},
		{43, 44}, {44, 44}, {45, 44}, {46, 44}, {47, 44},
		{47, 45}, {47, 46}, {48, 45}, {48, 46},
	} {
		packWater(flo, out.underlay, out.overlay, mxmz[0], mxmz[1])
	}

	// floorcol
	floorcol := packet2.Alloc(1)
	defer floorcol.Release()
	floorcol.P2(uint16(len(flo.Configs)))
	for i := 0; i < len(flo.Configs); i++ {
		floorcol.P4(refColors[i][0])
		floorcol.P4(refColors[i][1])
	}

	// Sprites + fonts
	spriteDir := filepath.Join(srcDir, "sprites")
	fontDir := filepath.Join(srcDir, "fonts")
	index := packet2.Alloc(1)
	defer index.Release()

	convert := func(dir, name string) (*packet2.Packet, error) {
		p, err := pixpack.ConvertImage(index, dir, name)
		if err != nil {
			return nil, fmt.Errorf("convertImage %s/%s: %w", dir, name, err)
		}
		return p, nil
	}

	mapscene, err := convert(spriteDir, "mapscene")
	if err != nil {
		return err
	}
	defer mapscene.Release()
	mapfunction, err := convert(spriteDir, "mapfunction")
	if err != nil {
		return err
	}
	defer mapfunction.Release()
	b12, err := convert(fontDir, "b12")
	if err != nil {
		return err
	}
	defer b12.Release()
	mapdots, err := convert(spriteDir, "mapdots")
	if err != nil {
		return err
	}
	defer mapdots.Release()

	loadFM := func(name string) (*packet2.Packet, error) {
		p, err := packet2.Load(filepath.Join(fontDir, name), false)
		if err != nil {
			return nil, fmt.Errorf("load font %s: %w", name, err)
		}
		return p, nil
	}
	fontNames := []string{"f11.fm", "f12.fm", "f14.fm", "f17.fm", "f19.fm", "f22.fm", "f26.fm", "f30.fm"}
	fonts := make(map[string]*packet2.Packet, len(fontNames))
	for _, n := range fontNames {
		p, err := loadFM(n)
		if err != nil {
			return err
		}
		fonts[n] = p
		defer p.Release()
	}

	// labels
	labelsRaw, err := os.ReadFile(filepath.Join(srcDir, "maps", "labels.txt"))
	if err != nil {
		return fmt.Errorf("read labels.txt: %w", err)
	}
	labels := parseLabels(string(labelsRaw))
	labelsPkt := packet2.Alloc(1)
	defer labelsPkt.Release()
	labelsPkt.P2(uint16(len(labels)))
	for _, lab := range labels {
		labelsPkt.PJStrLF(lab.Text)
		labelsPkt.P2(uint16(lab.X))
		labelsPkt.P2(uint16(lab.Z))
		labelsPkt.P1(uint8(lab.Type))
	}

	// Assemble jagfile (22 entries, TS:657-678 order)
	jag := jagfile.NewEmptyJagfile(false)
	jag.Write("underlay.dat", out.underlay)
	jag.Write("overlay.dat", out.overlay)
	jag.Write("loc.dat", out.loc)
	jag.Write("obj.dat", out.obj)
	jag.Write("npc.dat", out.npc)
	jag.Write("multi.dat", out.multi)
	jag.Write("free.dat", out.free)
	jag.Write("floorcol.dat", floorcol)
	jag.Write("mapscene.dat", mapscene)
	jag.Write("mapfunction.dat", mapfunction)
	jag.Write("b12.dat", b12)
	jag.Write("f11.dat", fonts["f11.fm"])
	jag.Write("f12.dat", fonts["f12.fm"])
	jag.Write("f14.dat", fonts["f14.fm"])
	jag.Write("f17.dat", fonts["f17.fm"])
	jag.Write("f19.dat", fonts["f19.fm"])
	jag.Write("f22.dat", fonts["f22.fm"])
	jag.Write("f26.dat", fonts["f26.fm"])
	jag.Write("f30.dat", fonts["f30.fm"])
	jag.Write("mapdots.dat", mapdots)
	jag.Write("index.dat", index)
	jag.Write("labels.dat", labelsPkt)

	outPath := filepath.Join(outDir, "mapview", "worldmap.jag")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(outPath), err)
	}
	if err := jag.Save(outPath); err != nil {
		return fmt.Errorf("save jagfile %s: %w", outPath, err)
	}
	return nil
}
```

Update the import block at the top of `worldmap.go` to include the new packages:

```go
import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	pf "github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/pixpack"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run TestPack_NoMapsDir_NoOp -v
```

Expected: PASS.

- [ ] **Step 5: Run the full package test suite**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -v
```

Expected: all PASS.

- [ ] **Step 6: Build check**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: no compile errors.

- [ ] **Step 7: Commit**

```
git add pkg/pack/worldmap/worldmap.go pkg/pack/worldmap/worldmap_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack/worldmap): wire Pack() entry point

Orchestrates: load Flo/Loc/Npc types, parse 3 CSVs, iterate
sorted server/maps entries, append 16 hardcoded water tiles,
build floorcol/sprites/fonts/labels packets, save 22-entry
worldmap.jag to outDir/mapview/.

Tags NAI-WORLDMAP-D-READDIR-SORTED: os.ReadDir is sorted, TS
fs.readdirSync is filesystem-order; no TS reference exists to
pin against so deterministic ordering is preferred.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: goscape-cli worldmap verb

**Files:**
- Create: `cmd/goscape-cli/cmd_worldmap.go`
- Create: `cmd/goscape-cli/cmd_worldmap_test.go`
- Modify: `cmd/goscape-cli/main.go`

**Context:** Follow `cmd_pack.go` exactly. The verb signature is `runWorldmap(args []string, stdout, stderr io.Writer) int`. Exit codes: 0 success / `-h`, 1 runtime error, 2 flag parse error.

- [ ] **Step 1: Write the failing test**

Create `cmd/goscape-cli/cmd_worldmap_test.go`:

```go
package main

import (
	"bytes"
	"testing"
)

func TestRunWorldmap_FlagParseError_ReturnsExit2(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if got := runWorldmap([]string{"--unknown-flag"}, &stdout, &stderr); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
}

func TestRunWorldmap_Help_ReturnsExit0(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if got := runWorldmap([]string{"-h"}, &stdout, &stderr); got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
}

func TestRunWorldmap_MissingMapsDir_NoOpReturnsExit0(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	tmp := t.TempDir()
	src := t.TempDir()
	args := []string{
		"--src-dir", src,
		"--out-dir", tmp,
		"--log.level", "error",
	}
	if got := runWorldmap(args, &stdout, &stderr); got != 0 {
		t.Errorf("exit = %d, want 0; stderr=%s", got, stderr.String())
	}
}

func TestDispatch_WorldmapRegistered(t *testing.T) {
	t.Parallel()
	found := false
	for _, v := range verbs {
		if v.name == "worldmap" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("worldmap verb not registered in verbs slice")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run "TestRunWorldmap|TestDispatch_Worldmap" -v
```

Expected: FAIL with "runWorldmap undefined".

- [ ] **Step 3: Write the implementation**

Create `cmd/goscape-cli/cmd_worldmap.go`:

```go
package main

import (
	"errors"
	"flag"
	"io"
	"log/slog"

	"github.com/zsrv/goscape/pkg/pack/worldmap"
	"github.com/zsrv/goscape/pkg/util/log"
)

// runWorldmap implements the `worldmap` verb. Builds the worldmap
// jagfile via pkg/pack/worldmap.Pack.
//
// Exit codes:
//
//	0 — success (or `-h`/`--help` print)
//	1 — logger init failed or worldmap.Pack returned an error
//	2 — flag parse error
func runWorldmap(args []string, stdout, stderr io.Writer) int {
	_ = stdout
	fs := flag.NewFlagSet("worldmap", flag.ContinueOnError)
	fs.SetOutput(stderr)

	srcDir := fs.String("src-dir", "data/src",
		"Source content directory (CSVs, fonts, sprites).")
	outDir := fs.String("out-dir", "data/pack",
		"Output directory (reads server/maps, writes mapview/worldmap.jag).")

	var logLevel slog.Level = slog.LevelInfo
	fs.TextVar(&logLevel, "log.level", logLevel,
		"Log severity (debug|info|warn|error).")
	logFormat := fs.String("log.format", "text",
		"Log format (text|json).")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if err := log.SetDefault(*logFormat, logLevel, stderr); err != nil {
		slog.Error("init logger", "err", err)
		return 1
	}

	if err := worldmap.Pack(*srcDir, *outDir); err != nil {
		slog.Error("worldmap pack failed", "err", err)
		return 1
	}
	return 0
}
```

Note: confirm `log.SetDefault` signature by reading `pkg/util/log/log.go` first. If it differs from this guess, adapt to match (the `cmd_pack.go` should be the canonical reference — copy whatever shape it uses verbatim).

Modify `cmd/goscape-cli/main.go` — find the `verbs` slice (around line 32) and append:

```go
	{name: "worldmap", handler: runWorldmap, help: "Build mapview/worldmap.jag from packed map output and Content assets."},
```

(Match the exact field names used by the other entries — `name`, `handler`, `help` is a guess; copy the shape from existing entries.)

- [ ] **Step 4: Run test to verify it passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run "TestRunWorldmap|TestDispatch_Worldmap" -v
```

Expected: PASS for all 4.

- [ ] **Step 5: Run the full cmd test suite**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -v
```

Expected: all PASS (no regressions in other verb tests).

- [ ] **Step 6: Build the binary**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o $TMPDIR/goscape-cli ./cmd/goscape-cli
$TMPDIR/goscape-cli worldmap -h
```

Expected: help text printed; exit 0.

- [ ] **Step 7: Commit**

```
git add cmd/goscape-cli/cmd_worldmap.go cmd/goscape-cli/cmd_worldmap_test.go cmd/goscape-cli/main.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(goscape-cli): add worldmap verb

Wraps pkg/pack/worldmap.Pack with the standard --src-dir/
--out-dir/--log.* flag block. Follows cmd_pack.go pattern.
Exit codes: 0 success, 1 runtime error, 2 flag parse error.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Integration smoke against real Content

**Files:**
- Modify: `pkg/pack/worldmap/worldmap_test.go` (add build-tagged integration test)

**Context:** End-to-end smoke. Requires real Content + a `data/pack/server/maps` directory pre-built by `packall.PackAll`. Run separately because it depends on absolute paths and takes seconds.

The test is gated by an env var (`GOSCAPE_WORLDMAP_INTEGRATION=1`) — preferred over `//go:build integration` because it avoids needing a separate test-tag invocation.

- [ ] **Step 1: Write the failing test**

Append to `pkg/pack/worldmap/worldmap_test.go`:

```go
func TestPack_RealContent_Integration(t *testing.T) {
	if os.Getenv("GOSCAPE_WORLDMAP_INTEGRATION") != "1" {
		t.Skip("set GOSCAPE_WORLDMAP_INTEGRATION=1 to enable")
	}

	srcDir := os.Getenv("GOSCAPE_CONTENT_DIR")
	if srcDir == "" {
		srcDir = "$HOME/Code/github.com/LostCityRS/content"
	}
	packDir := os.Getenv("GOSCAPE_PACK_DIR")
	if packDir == "" {
		t.Skip("set GOSCAPE_PACK_DIR to a directory containing server/maps/")
	}
	if _, err := os.Stat(filepath.Join(packDir, "server", "maps")); err != nil {
		t.Skipf("%s/server/maps missing: %v", packDir, err)
	}

	// Pack writes into outDir/mapview, so copy/symlink the packed
	// outputs into a tempdir, run there, and inspect the result.
	outDir := t.TempDir()
	if err := os.Symlink(filepath.Join(packDir, "server"), filepath.Join(outDir, "server")); err != nil {
		t.Fatalf("symlink server: %v", err)
	}
	if err := os.Symlink(filepath.Join(packDir, "client"), filepath.Join(outDir, "client")); err != nil {
		t.Fatalf("symlink client: %v", err)
	}

	if err := Pack(srcDir, outDir); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	jagPath := filepath.Join(outDir, "mapview", "worldmap.jag")
	st, err := os.Stat(jagPath)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if st.Size() == 0 {
		t.Fatalf("output jag is empty")
	}

	jag, err := jagfile.LoadJagfile(jagPath)
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}

	// 22 entries in the fixed TS-defined order.
	expectedNames := []string{
		"underlay.dat", "overlay.dat", "loc.dat", "obj.dat", "npc.dat",
		"multi.dat", "free.dat", "floorcol.dat",
		"mapscene.dat", "mapfunction.dat", "b12.dat",
		"f11.dat", "f12.dat", "f14.dat", "f17.dat", "f19.dat",
		"f22.dat", "f26.dat", "f30.dat",
		"mapdots.dat", "index.dat", "labels.dat",
	}
	for _, n := range expectedNames {
		p, err := jag.Read(n)
		if err != nil {
			t.Errorf("missing entry %q: %v", n, err)
			continue
		}
		if p.Length() == 0 {
			// underlay/overlay/loc/floorcol/labels should always be populated.
			switch n {
			case "underlay.dat", "overlay.dat", "loc.dat", "floorcol.dat", "labels.dat":
				t.Errorf("entry %q is empty (expected non-zero)", n)
			}
		}
	}
}
```

Add `"github.com/zsrv/goscape/pkg/io/jagfile"` to the test imports if not already present.

- [ ] **Step 2: Run test (skipped by default)**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/worldmap/ -run TestPack_RealContent_Integration -v
```

Expected: SKIP ("set GOSCAPE_WORLDMAP_INTEGRATION=1").

- [ ] **Step 3: Run with real Content**

First, ensure `pkg/pack/server/maps` exists by running PackAll if needed (build the cli + run `goscape-cli pack` against real Content). Then run:

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o $TMPDIR/goscape-cli ./cmd/goscape-cli
rm -rf $TMPDIR/goscape-out
$TMPDIR/goscape-cli pack \
  --src-dir $HOME/Code/github.com/LostCityRS/content \
  --out-dir $TMPDIR/goscape-out \
  --log.level error

GOSCAPE_WORLDMAP_INTEGRATION=1 \
GOSCAPE_PACK_DIR=$TMPDIR/goscape-out \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  go test ./pkg/pack/worldmap/ -run TestPack_RealContent_Integration -v
```

Expected: PASS. The integration test:
- Loads 22 entries from `worldmap.jag`
- Confirms `underlay.dat`, `overlay.dat`, `loc.dat`, `floorcol.dat`, `labels.dat` are all non-empty

If FAIL: read the error output, inspect intermediate state in `$TMPDIR/goscape-out/`, identify the failing assertion, and fix the production code. Do NOT loosen the test.

- [ ] **Step 4: Try the verb end-to-end**

Run:
```
$TMPDIR/goscape-cli worldmap \
  --src-dir $HOME/Code/github.com/LostCityRS/content \
  --out-dir $TMPDIR/goscape-out \
  --log.level info
ls -la $TMPDIR/goscape-out/mapview/
```

Expected: `worldmap.jag` exists and is non-trivial in size (likely > 100 KB).

- [ ] **Step 5: Commit**

```
git add pkg/pack/worldmap/worldmap_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack/worldmap): add env-gated real-Content integration smoke

Verifies Pack() builds a 22-entry worldmap.jag from a real
pack dir (output of goscape-cli pack). Gated by
GOSCAPE_WORLDMAP_INTEGRATION=1 and GOSCAPE_PACK_DIR=<dir>.
Asserts entry names + non-empty payloads for the always-
populated stages (underlay/overlay/loc/floorcol/labels).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Final sanity sweep

**Files:** none modified.

- [ ] **Step 1: Run all tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run with race detector on the worldmap package**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/worldmap/
```

Expected: PASS.

- [ ] **Step 3: Verify the binary still builds**

```
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o $TMPDIR/goscape ./cmd/goscape
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o $TMPDIR/goscape-cli ./cmd/goscape-cli
```

Expected: both binaries built.

- [ ] **Step 4: Confirm verb in help**

```
$TMPDIR/goscape-cli
```

Expected: usage output lists `worldmap` alongside `pack`, `compile`, `jag`, `smoke-pack`.

- [ ] **Step 5: Final commit (only if any cleanup happened)**

If steps 1-4 surfaced anything that needed fixing, commit those fixes with an appropriate message. Otherwise no commit needed.

---

## Self-Review

**Spec coverage:**
- §3 file layout (4 source + 4 test + cli) → Tasks 1-7 cover all files
- §4 Pack signature → Task 6
- §5 dependency map → Tasks 1 (Flo loader) + 5/6 (consumes Loc/Npc/coordgrid/jagfile/pixpack/pathfinder.loc)
- §6 data flow → Tasks 5+6 implement loop and orchestration
- §7 deviations → NAI-WORLDMAP-D-READDIR-SORTED noted in Task 6
- §8 error handling → Task 6 wraps all error returns; missing-maps early-return tested in Task 6
- §9 testing strategy → Tasks 2-4 unit tests; Task 8 integration smoke
- §10 CLI verb → Task 7
- §11 effort estimate → 8 implementation tasks + 1 sweep matches "~6-8 tasks" estimate

**Placeholder scan:** none found.

**Type consistency:**
- `FloTypeConfigs` (Task 1) used in Tasks 4, 5, 6 → matches
- `mapCtx`, `mapPackets` (Task 5) used in Task 6 → matches
- `processMap` signature (Task 5) called in Task 6 → matches
- `Pack(srcDir, outDir string) error` (stub Task 4, real Task 6) → matches
- `runWorldmap(args, stdout, stderr) int` (Task 7) → matches cmd_pack.go pattern

**Known unknowns to verify during execution:**
- Exact field names in `verbs` slice (Task 7 step 3 says "match the exact field names" — re-read main.go before editing)
- Exact `log.SetDefault` signature (Task 7 step 3 says "confirm by reading pkg/util/log/log.go first")
- TS `Packet.pjstr` default terminator: assumed LF (10), used `PJStrLF`. If TS default differs (some forks use 0/NUL), labels.dat will be byte-different. Worth re-confirming by reading `Engine-TS/src/io/Packet.ts`'s `pjstr` default before Task 6 step 3.

These are intentional verify-on-execute notes, not placeholders — the implementing agent must look up the exact APIs in the existing codebase rather than trust my guess. This is more reliable than codifying potentially-wrong signatures here.

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-05-17-pack-worldmap.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
