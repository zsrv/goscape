# pack-worldmap port — design

**Date:** 2026-05-17
**TS source:** `Engine-TS/tools/pack/map/Worldmap.ts` (682 LOC)
**Target:** `pkg/pack/worldmap/` + `cmd/goscape-cli/cmd_worldmap.go`

## 1. Goal

Port the TS `packWorldmap()` driver to goscape. Produces `data/pack/mapview/worldmap.jag`, a 22-entry jagfile bundling per-map underlay/overlay/wall/obj/npc/multiway/free-to-play tile encodings plus the floor-color reference table, font metric files, and label/sprite assets. Used by mapview clients; independent of the in-game `PackAll` pipeline (TS does not call it from `PackAll`).

## 2. Non-goals

- **Not added to `packall.PackAll`** — TS parity. `PackAll` does not call `packWorldmap`.
- **Not byte-pinned against a TS reference** — `Engine-TS/data/pack/` ships only `client/` and `server/`; no `mapview/worldmap.jag` exists to diff against. Documented as a known testing gap.
- **Not added to `goscape-cli smoke-pack`** — smoke-pack's purpose is parity with `Engine-TS/data/pack/`; worldmap output has no parity target. Could be added in a later phase if a pinned fixture is captured.
- No async / worker integration with `::rebuild` (out of scope per the rebuild spec).

## 3. Package layout

```
pkg/pack/worldmap/
  worldmap.go          // Pack() entry point, per-map loop, packWater
  csv.go               // processCsv, parseLabels
  refcolors.go         // 80-entry floor color literal (lines 533-613 of TS)
  worldmap_test.go     // integration smoke + table-driven per-map tests
  csv_test.go          // processCsv branches: range/point, comments, alignment warnings
  refcolors_test.go    // len==80 + a couple of sentinel rows
```

Sibling to `pkg/pack/maps`, `pkg/pack/audio`, `pkg/pack/graphics`, etc.

## 4. Public API

```go
// Pack builds data/pack/mapview/worldmap.jag from the server-side
// map outputs already produced by pkg/pack/maps and from
// fonts/sprites/CSVs in srcDir.
//
// srcDir   — Content root (the "BUILD_SRC_DIR" of TS). Reads
//            srcDir/maps/{multiway,free2play,ignore}.csv, labels.txt,
//            srcDir/fonts/*.fm, srcDir/sprites/{mapscene,mapfunction,mapdots}.png.
// outDir   — data/pack root. Reads outDir/server/maps/{m,l,o,n}*,
//            writes outDir/mapview/worldmap.jag.
//
// Returns nil (no error) when outDir/server/maps is missing — TS
// parity with `if (!fs.existsSync(...)) return;` at TS:31-33.
func Pack(srcDir, outDir string) error
```

No other exported symbols.

## 5. Dependency map

All required Go packages already exist in goscape:

| TS dependency | Go equivalent |
|---|---|
| `FloType.load/.getId/.configs` | `pkg/pack/flo.go` (`FloType`, `LoadFloTypes`) |
| `LocType.load/.get/.mapscene/.mapfunction/.active` | `pkg/pack/loc.go` (`LocType`, `LoadLocTypes`) |
| `NpcType.load/.get/.minimap` | `pkg/pack/npc.go` (`NpcType`, `LoadNpcTypes`) |
| `CoordGrid.packCoord` | `pkg/coordgrid.PackCoord` |
| `Jagfile.new/.write/.save` | `pkg/io/jagfile` |
| `Packet.alloc/.load/.g1/.g2/.gsmarts/.p1/.p2/.p4/.pbool/.pjstr` | `pkg/io/packet.Packet` |
| `convertImage` (PixPack) | `pkg/pixpack.ConvertImage` |
| `LocShape.{WALL_STRAIGHT,WALL_L,WALL_DIAGONAL,WALLDECOR_STRAIGHT_NOOFFSET}` | `pkg/pathfinder/loc.{ShapeWallStraight,ShapeWallL,ShapeWallDiagonal,ShapeWallDecorStraightNoOffset}` |
| `printWarning` | `slog.Warn` (package-level logger) |
| `Environment.BUILD_SRC_DIR` | `srcDir` arg |

No new third-party deps.

## 6. Data flow

```
                      ┌──────────────────────────────┐
srcDir (Content) ───► │ load FloType/LocType/NpcType │ (from outDir/server/*.dat)
                      │ load multiway.csv            │
                      │ load free2play.csv           │
                      │ load ignore.csv              │
                      │ list outDir/server/maps/m*   │
                      └──────────────┬───────────────┘
                                     │
                  ┌──────────────────┴──────────────────┐
                  │ per (mx,mz), skip if in ignoremap:  │
                  │   read m{mx}_{mz}, l{mx}_{mz},      │
                  │        o{mx}_{mz}, n{mx}_{mz}       │
                  │   append to underlay/overlay/loc/   │
                  │   obj/npc/multi/free packets        │
                  └──────────────────┬──────────────────┘
                                     │
                                     ▼
                 packWater × 16 hardcoded coords
                                     │
                                     ▼
                 build floorcol from refColors (80 entries)
                                     │
                                     ▼
            convertImage × 4 (mapscene, mapfunction, b12, mapdots)
            load f11/f12/f14/f17/f19/f22/f26/f30 .fm files
                                     │
                                     ▼
                          parse labels.txt
                                     │
                                     ▼
                  Jagfile.write × 22 + Save → worldmap.jag
```

## 7. TS parity notes & deviations

Anticipated deviations (each will carry a `NAI-WORLDMAP-D-*` deviation tag if it lands):

1. **`fs.readdirSync` ordering** — TS uses filesystem-order, which on most Linux ext4 deployments is insertion order (effectively non-deterministic across rebuilds). Go's `os.ReadDir` returns lexically sorted entries. **Consequence:** per-map (mx,mz) entries appear in different positions in the underlay/overlay/loc/obj/npc/multi/free packets, so the output `worldmap.jag` would be byte-different from a TS-produced reference even though logically equivalent. This is acceptable because (a) no TS reference is checked in and (b) deterministic ordering is the more defensible behaviour. Will use `os.ReadDir` (already sorted) without an extra `sort.Strings`. Tag: `NAI-WORLDMAP-D-READDIR-SORTED`. If a future byte-pin against TS output is desired, either pre-sort TS-side input or accept the ordering difference.
2. **`printWarning` → `slog.Warn`** — cosmetic, no functional change. No tag.
3. **`Packet.alloc(20_000_000)` pre-sizing** — Go's `bytes.Buffer` grows on demand; pre-sizing is a perf hint. Will use `Packet` default constructor (capacity grows). No tag.
4. **Early-return when `outDir/server/maps` missing** — Direct port (`if !exists return nil`). No tag.

If the wall-shape branch logic at TS:303-328 needs any goscape-side adjustment because `loc.Shape` enum values differ from TS `LocShape` numerical values: this would be a substantive deviation requiring a tag. **Pre-verification**: goscape `ShapeWallStraight=0`, `ShapeWallL=2`, `ShapeWallDecorStraightNoOffset=4`, `ShapeWallDiagonal=9`. These match `@2004scape/rsmod-pathfinder` values. No deviation expected.

## 8. Error handling

- Missing `outDir/server/maps`: return nil (TS parity).
- Missing CSV/labels/font/sprite files: return wrapped error. (TS panics with `ENOENT`; Go returns the error.)
- Malformed CSV row: log warning (parity with TS `printWarning`), continue. Do not error out.
- Per-map binary read errors: wrap with `(mx, mz)` and return; failing one map fails the whole pack (TS parity — TS throws on `Packet.load` failure).
- Jagfile save failure: wrap and return.

## 9. Testing strategy

### Unit tests

- `csv_test.go`:
  - `processCsv` single-tile expansion (one row, 64 packed coords)
  - `processCsv` range expansion (two-tile row, correct rectangle)
  - `//` comment skipping
  - empty-line skipping
  - alignment-violation warning emission (`fromLx % 8 != 0`, `toLx % 8 != 7`, `fromMx > toMx`)
  - overlap warning emission
  - `parseLabels` filters non-`=` prefixed lines, parses 4-field rows
- `refcolors_test.go`: `len(refColors) == 80`, spot-check three rows against TS literal
- `worldmap_test.go`:
  - `packWater` writes exactly `2 + 4096 = 4098` bytes to underlay and `2 + 4096*2 = 8194` bytes to overlay (total 12292 across both packets)
  - `unpackCoord` masks (`level<<12 | x<<6 | z`)

### Integration smoke

- `TestPack_RealContent` (build-tagged `//go:build integration` or env-gated): given real Content + a pre-built `data/pack/server/maps`, runs `Pack(srcDir, outDir)` and asserts:
  - `outDir/mapview/worldmap.jag` exists and is non-empty
  - jagfile parses cleanly via `pkg/io/jagfile`
  - exactly 22 entries
  - entry names contain `{underlay,overlay,loc,obj,npc,multi,free,floorcol,mapscene,mapfunction,b12,f11,f12,f14,f17,f19,f22,f26,f30,mapdots,index,labels}.dat`
  - per-entry payload non-zero for `underlay/overlay/loc/floorcol/labels` (always-populated stages)

### Not in scope

- Byte-pinning against a TS-built `worldmap.jag` reference. Reason: no checked-in reference exists. If the user later generates one (one-time `bun run tools/pack/map/Worldmap.ts` against the same Content), a byte-pin test can be added.

## 10. CLI integration

New verb in `cmd/goscape-cli/cmd_worldmap.go` following `cmd_pack.go` pattern exactly:

```
goscape-cli worldmap [flags]
  --src-dir          Source content directory (default: data/src)
  --out-dir          Output directory (default: data/pack)
  --log.level        Log severity (default: info)
  --log.format       Log format (default: text)
```

Registered in `cmd/goscape-cli/main.go`'s `verbs` slice.

Exit codes match `cmd_pack.go`: 0 success, 1 runtime error, 2 flag parse error.

## 11. Estimated effort

- Port + tests: ~700 LOC Go (rough parity with 682 LOC TS)
- One-day port for a single TDD agent if subagent-driven, or ~half-day controller pass
- Plan will decompose into ~6-8 tasks: csv parsing, refcolors literal, packWater, per-map loop, top-level Pack assembly, jagfile assembly, CLI verb wiring, integration smoke

## 12. Open questions

None at time of writing. Will surface during implementation if the wall-shape logic or `Packet.gsmarts` behaviour diverges.
