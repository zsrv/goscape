# NAI-179 — SPLIT_* cache-loader port (MesanimType + FontType)

**Date:** 2026-05-12
**Tech stack:** Go 1.26+
**Cadence:** Standard sub-spec (`runescript_cadence.md`); execution via `subagent-driven-development`.
**Retires:** `NAI-75-D-MESANIM-NOT-PORTED`, `NAI-75-D-FONT-WRAP-NAIVE` (both opened at NAI-75 close).

---

## §1 — Background

NAI-75 (2026-05-03) landed a light-fidelity port of the `SPLIT_*`
opcode family (`SPLIT_INIT`, `SPLIT_GET`, `SPLIT_PAGECOUNT`,
`SPLIT_LINECOUNT`, `SPLIT_GETANIM`) at `pkg/script/handlers_string.go`.
Two TS-fidelity deviations were deliberately deferred because they
each require a new cache-loader:

- **`NAI-75-D-FONT-WRAP-NAIVE`** — `SPLIT_INIT` skips
  `font.split(text, maxWidth)` and falls back to `strings.Split(text, "|")`.
  `fontId` + `maxWidth` are popped-and-discarded. Long lines without `|`
  overflow the dialog component.
- **`NAI-75-D-MESANIM-NOT-PORTED`** — `SPLIT_INIT` recognises and strips
  the `<p,name>` mesanim prefix but does NOT resolve the name to a
  `MesanimType` id; `state.SplitMesanim` stays `-1`. `SPLIT_GETANIM`
  returns `-1` unconditionally. Consequence: chathead anims absent on
  every `~chatnpc*`/`~chatplayer` dialog (static head, no talk-anim).

This sub-spec ports both cache types and retires both tags in one
bundle (user-directed). The two ports are mutually independent.

## §2 — Goals

1. Port `MesanimType` server-side `.dat` config loader (mirrors
   `Engine-TS/src/cache/config/MesanimType.ts`).
2. Port `FontType` client-side Jagfile-backed loader + `Split(text, maxWidth)`
   word-wrap algorithm (mirrors `Engine-TS/src/cache/config/FontType.ts`).
3. Surface both via the `pkg/script.Configs` interface; wire production
   loaders in `modules/world/server.go`.
4. Rewrite `handleSplitInit` to (a) resolve `<p,name>` → `MesanimType` id
   via `Configs.MesanimByName`, (b) call `FontType.Split(text, maxWidth)`
   instead of `strings.Split(text, "|")`.
5. Rewrite `handleSplitGetAnim` to read `MesanimType.Len[lineCount-1]`
   when `splitMesanim >= 0`; otherwise push `-1`.
6. Retire `NAI-75-D-MESANIM-NOT-PORTED` and `NAI-75-D-FONT-WRAP-NAIVE`
   in every doc-comment + test pin (`rg "NAI-75-D-(MESANIM|FONT-WRAP)" pkg/ modules/`).

## §3 — TS-source canonical anchors

- `Engine-TS/src/cache/config/MesanimType.ts:1-71` — `MesanimType` (server-only `.dat`; decode codes 1-4 + 250)
- `Engine-TS/src/cache/config/FontType.ts:1-177` — `FontType` (client `title` Jagfile; decode bitmaps + `drawWidth`; `split(str, maxWidth)`)
- `Engine-TS/src/engine/script/handlers/StringOps.ts:76-122` — `SPLIT_INIT` body, `SPLIT_GETANIM` body
- `Engine-TS/src/engine/script/ScriptValidators.ts:131-132` — `FontTypeValid`, `MesanimValid`

## §4 — Non-goals

- **No** general-purpose font rendering or text-measurement surface
  outside `pkg/fonttype.FontType.Split` + `.StringWidth`. The character
  bitmap data is decoded and discarded after `drawWidth` is computed
  (TS keeps `charMask` resident for rendering; goscape needs only the
  width metric for `Split`).
- **No** `FontTypeValid` / `MesanimValid` cache-validator framework
  parallel-port. Validation collapses inline at handler call site.
- **No** changes to the `<p,name>` parser — the regex-equivalent prefix
  recognition at `handlers_string.go:121-127` already matches TS; only
  the unresolved-name → `-1` branch becomes a real lookup.

## §5 — Architecture

### §5.1 — `pkg/objtype/mesanimtype.go` (server-only)

Mirror the existing `idktype.go` / `enumtype.go` shape:

```go
type MesanimType struct {
    ConfigType
    Len [4]int // init -1, written by codes 1-4 as int(G2())
}

func NewMesanimType(id int) *MesanimType {
    return &MesanimType{
        ConfigType: ConfigType{ID: id},
        Len:        [4]int{-1, -1, -1, -1},
    }
}

func (t *MesanimType) Decode(code uint8, dat *packet2.Packet) error {
    switch {
    case code >= 1 && code <= 4:
        t.Len[code-1] = int(dat.G2())
    case code == 250:
        t.DebugName = dat.GJStrLF()
    default:
        return fmt.Errorf("unrecognized mesanim config code %d", code)
    }
    return nil
}

type MesanimTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*MesanimType
}

func LoadMesanimTypes(dir string) (*MesanimTypeConfigs, error) {
    // server/mesanim.dat only (TS MesanimType.load:11-17 has no
    // client-jag side, unlike IdkType). Silent-on-missing per TS.
    // Returns empty registry + nil err when file absent.
}
```

### §5.2 — `pkg/fonttype/fonttype.go` (client `title` Jagfile)

New package. TS-faithful port of `FontType.ts:1-177`:

```go
package fonttype

// Package-level char→glyph table, populated at init() per TS static
// initializer block at FontType.ts:7-18.
var CharLookup [256]byte

func init() {
    const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
        "abcdefghijklmnopqrstuvwxyz" +
        "0123456789!\"£$%^&*()-_=+[{]};:'@#~,<.>/?\\| "
    for i := 0; i < 256; i++ {
        if idx := strings.IndexByte(charset, byte(i)); idx >= 0 {
            CharLookup[i] = byte(idx)
        } else {
            CharLookup[i] = 74
        }
    }
}

type FontType struct {
    // drawWidth is the only field used outside the decoder.
    drawWidth [256]byte
    height    int
    // charAdvance kept as decode-time scratch; not exported (matches
    // light-fidelity goal — we don't render text).
}

// Load parses the title Jagfile and returns 4 FontType instances:
// id 0=p11, 1=p12, 2=b12, 3=q8 (matching TS FontType.load:20-27).
// Returns nil + err when title is missing or any font fails to decode.
func Load(dir string) ([]*FontType, error) { ... }

// StringWidth ports TS FontType.stringWidth (FontType.ts:123-138).
// Respects @col@ inline color escapes (4-char skip).
func (f *FontType) StringWidth(s string) int { ... }

// Split ports TS FontType.split (FontType.ts:140-176). Returns a
// slice of lines whose StringWidth is ≤ maxWidth, breaking on
// '|' (forced) or on the last space-boundary that fits.
func (f *FontType) Split(s string, maxWidth int) []string { ... }
```

The bitmap loop (lines 61-114 of TS) decodes per-character pixel
masks; goscape executes the loop for cursor-advance + drawWidth
computation but **does not retain** `charMask`. The two `charAdvance`
fix-ups for left/right empty columns (TS:99-113) remain unchanged.

### §5.3 — `pkg/script.Configs` interface additions

```go
type Configs interface {
    // ... existing ...
    MesanimType(id int) *objtype.MesanimType
    MesanimByName(name string) int          // -1 on miss (matches TS getId)
    FontType(id int) *fonttype.FontType     // nil on out-of-range
}
```

Implementations:
- `serverConfigsView` (`modules/world/server_configs.go`): three new
  methods returning from `s.mesanimTypes` / `s.fontTypes`.
- Test mocks (`mockConfigs` in `pkg/script/handlers_*_test.go`): add
  zero-value-friendly stubs; tests that need real lookups set the
  fields explicitly.

### §5.4 — Production wiring

`modules/world/server.go` `NewServer` adds two new loader calls,
slotted alphabetically next to existing siblings:

```go
mesanimTypes, err := objtype.LoadMesanimTypes(cfg.CachePath)
if err != nil { return nil, fmt.Errorf("load mesanim types: %w", err) }
s.mesanimTypes = mesanimTypes

fontTypes, err := fonttype.Load(cfg.CachePath)
if err != nil { return nil, fmt.Errorf("load font types: %w", err) }
s.fontTypes = fontTypes
```

Both must be loaded before `s.configsView` is taken, but neither is
needed for any other init step.

### §5.5 — `handleSplitInit` rewrite

```go
func handleSplitInit(s *ScriptState) error {
    fontId := s.PopInt()
    linesPerPage := s.PopInt()
    maxWidth := s.PopInt()
    text := s.PopString()

    s.SplitMesanim = -1
    if strings.HasPrefix(text, "<p,") {
        if end := strings.IndexByte(text, '>'); end != -1 {
            name := text[3:end]
            s.SplitMesanim = s.Configs.MesanimByName(name)
            text = text[end+1:]
        }
    }

    var lines []string
    if font := s.Configs.FontType(fontId); font != nil {
        // TS path: font.split(text, maxWidth) handles both '|' breaks
        // AND per-char-width word-wrap (FontType.ts:140-176).
        lines = font.Split(text, maxWidth)
    } else {
        // Defensive on invalid fontId. TS FontTypeValid throws;
        // goscape falls back to the NAI-75 light-fidelity '|'-only
        // split + slog.Warn. Labelled per
        // defensive_gate_doc_comment_label.md.
        slog.Warn("SPLIT_INIT: invalid fontId; falling back to '|' split",
            "script", s.Script.Name, "fontId", fontId)
        lines = strings.Split(text, "|")
    }

    if linesPerPage < 1 {
        // Defensive: TS would divide-by-zero on splice(0, 0).
        // Preserved from NAI-75.
        s.SplitPages = [][]string{lines}
        return nil
    }
    pages := make([][]string, 0, (len(lines)+linesPerPage-1)/linesPerPage)
    for i := 0; i < len(lines); i += linesPerPage {
        end := i + linesPerPage
        if end > len(lines) {
            end = len(lines)
        }
        pages = append(pages, lines[i:end])
    }
    s.SplitPages = pages
    return nil
}
```

### §5.6 — `handleSplitGetAnim` rewrite

```go
func handleSplitGetAnim(s *ScriptState) error {
    page := s.PopInt()
    if s.SplitMesanim < 0 {
        s.PushInt(-1)
        return nil
    }
    typ := s.Configs.MesanimType(s.SplitMesanim)
    if typ == nil {
        // TS MesanimValid would throw; goscape defensive.
        s.PushInt(-1)
        return nil
    }
    if page < 0 || page >= len(s.SplitPages) {
        s.PushInt(-1)
        return nil
    }
    lineCount := len(s.SplitPages[page])
    idx := lineCount - 1
    if idx < 0 || idx >= len(typ.Len) {
        s.PushInt(-1)
        return nil
    }
    s.PushInt(int(typ.Len[idx]))
    return nil
}
```

## §6 — Dataflow

```
SPLIT_INIT(text, maxWidth, linesPerPage, fontId)
  │
  ├─► Configs.FontType(fontId)  ──nil?──► fallback ('|' split + warn)
  │
  ├─► parse "<p,NAME>" prefix
  │   └─► Configs.MesanimByName(NAME)  ──► state.SplitMesanim (or -1)
  │
  ├─► font.Split(text, maxWidth)  ──► []string lines
  │
  └─► chunk lines into pages of linesPerPage  ──► state.SplitPages

SPLIT_GETANIM(page)
  │
  ├─► state.SplitMesanim < 0  ──► push -1
  │
  ├─► Configs.MesanimType(state.SplitMesanim)  ──nil?──► push -1
  │
  └─► push MesanimType.Len[len(state.SplitPages[page]) - 1]
```

## §7 — Test strategy

Test file → assertions, in implementation order:

**`pkg/objtype/mesanimtype_test.go`** (new)

- `TestMesanimDecode_Code1WritesLen0` — code 1 + G2(42) → `t.Len[0] == 42`.
- `TestMesanimDecode_Code4WritesLen3` — code 4 + G2(7) → `t.Len[3] == 7`.
- `TestMesanimDecode_Code250WritesDebugName` — round-trip `GJStrLF("neutral")`.
- `TestMesanimDecode_UnknownCodeErrors` — code 5 → error containing "unrecognized".
- `TestNewMesanimType_LenInitMinusOne` — all four `Len` entries `-1`.
- `TestLoadMesanimTypes_MissingFileEmptyRegistry` — pass a tmpdir
  containing no `server/mesanim.dat`; expect zero-config registry + nil err.
- `TestLoadMesanimTypes_RealCache` — load from `data/pack/server/mesanim.dat`
  (test skips when file absent per existing `loctype_realcache_test.go` pattern);
  assert count > 0 and at least one config has a non-empty `DebugName`.

**`pkg/fonttype/fonttype_test.go`** (new)

- `TestCharLookup_AsciiA` — `CharLookup['A'] == 0`, `CharLookup['a'] == 26`, `CharLookup[' '] == 93`.
- `TestCharLookup_Unknown` — `CharLookup[0x01] == 74` (unmapped → fallback slot).
- `TestLoad_FourFonts` — `Load("data/pack")` returns 4 non-nil instances
  (test skips when title file absent).
- `TestFontType_StringWidth_Empty` — empty string → 0.
- `TestFontType_StringWidth_AtColorEscape` — `"@cya@hi"` has same width
  as `"hi"` (4-char escape skip per TS:130-132).
- `TestFontType_Split_EmptyString` — `Split("", 100)` → `[]string{""}` (TS special-case at :141-144).
- `TestFontType_Split_NoBreakNeeded` — short string with no `|` and
  `StringWidth ≤ maxWidth` → 1 line.
- `TestFontType_Split_OnPipe` — `"alpha|beta|gamma"` with huge maxWidth
  → `["alpha", "beta", "gamma"]`.
- `TestFontType_Split_OnSpace_ExceedsMaxWidth` — construct a string with
  spaces and a maxWidth that forces ≥1 line break at the space boundary;
  pin: each output line has `StringWidth ≤ maxWidth` AND splits at a
  whitespace gap (not mid-word).
- `TestFontType_Split_NoSpaceForcesFullLine` — single long word > maxWidth →
  emitted as a single overflowing line (TS:159-170 `splitIndex = str.length` default).

**`pkg/script/handlers_string_test.go`** (update existing)

- Strengthen `TestHandleSplitInitChainReplacesState` (existing): existing
  push-order remains valid because pop arity (3 ints + 1 string) is
  unchanged.
- **New** `TestSplitInit_PrefixResolvesMesanim` — `mockConfigs` registers
  a fake `MesanimType` with `DebugName="neutral"` at id 7; SPLIT_INIT
  on `"<p,neutral>hi"` → `state.SplitMesanim == 7`, `text` post-strip
  matches `["hi"]`.
- **New** `TestSplitInit_PrefixUnknownStaysNegOne` — name not in
  `mockConfigs` map → `SplitMesanim == -1`. (Replaces the old
  `NAI-75-D-MESANIM-NOT-PORTED`-pinned assertion.)
- **New** `TestSplitInit_FontWrap_BreaksOnMaxWidth` — `mockConfigs`
  returns a real `*FontType` loaded from `data/pack`; passes a long
  ASCII sentence and a small `maxWidth`; asserts ≥ 2 output pages-of-1-line
  AND each line's `StringWidth ≤ maxWidth`. Test skips if title file absent.
- **New** `TestSplitInit_InvalidFontFallsBackToPipeSplit` — `mockConfigs.FontType`
  returns `nil`; SPLIT_INIT on `"a|b"` with linesPerPage=1 → 2 pages
  containing `["a"]` and `["b"]` (defensive fallback path).
- **New** `TestSplitGetAnim_ResolvesLen` — register `MesanimType{Len:[-1,5,-1,-1]}`
  at id 3; `SplitMesanim=3`; `SplitPages=[["a","b"]]`; `SPLIT_GETANIM(0)`
  → push 5 (`Len[2-1]`).
- **New** `TestSplitGetAnim_NoMesanimReturnsNegOne` — `SplitMesanim=-1` →
  push -1. (Replaces the existing `NAI-75-D-MESANIM-NOT-PORTED` pin at
  `handlers_string_test.go:332-336`.)
- **New** `TestSplitGetAnim_LenIsMinusOneFallsThrough` — `MesanimType.Len[idx]==-1`
  (config defines no len for that lineCount) → push -1. (Per TS, push
  `len[lineCount-1]` value directly even if it's the sentinel; goscape
  matches.)
- **Update** `TestSplitInitPrefixParse` (existing, line ~144) — drop the
  `SplitMesanim` pin at `-1`; assert it now equals the resolved id.

**`modules/world/server_configs_test.go`** (update if exists; else new)

- `TestServerConfigsView_MesanimType` — assert lookup returns nil for
  out-of-range id, non-nil for a known id (use real cache or seed in test).
- `TestServerConfigsView_FontType` — assert nil for id 4, non-nil for ids 0-3 (real cache).

## §8 — Plan-runnable test fixtures

Per `plan_runnable_test_fixtures.md`, all bytecode/state fixtures in
this spec mentally compile to a runnable form. Push-order for
`SPLIT_INIT(text, maxWidth, linesPerPage, fontId)` matches the
existing helper `runSplitInitThen` at `handlers_string_test.go:235-260`
(see `plan_runnable_test_fixtures.md` for the historical fixup).

## §9 — Risk register

| # | Premise | Verification at HEAD | Risk if wrong |
|---|---------|----------------------|---------------|
| P1 | `pkg/io/jagfile.Jagfile` can decode `data/pack/client/title` (loads `p11.dat` etc.) | Confirmed: `pkg/cache/crctable.go:53` already references the title file; the jagfile package handles arbitrary jag archives | FontType load fails; sub-spec blocks |
| P2 | `MesanimType.ts` has no client-jag side (server-only loader) | TS `MesanimType.ts:11-17` reads only `${dir}/server/mesanim.dat` — confirmed at brainstorm | Mesanim load shape diverges from siblings |
| P3 | `data/pack/server/mesanim.dat` exists in working copy | Confirmed via `ls` at brainstorm | Real-cache test skips; unit tests still valid |
| P4 | No existing FontType / MesanimType symbols anywhere in `pkg/` or `modules/` | `grep -rn "FontType\|MesanimType" pkg/ modules/` returns only the doc-comment references in `handlers_string*.go` and `state.go` | Symbol collision; build failure |
| P5 | `pkg/script.Configs` interface is the canonical add-method surface for new cache types | All existing types in the same interface (`IdkType`, `EnumType`, etc.) follow this pattern (`pkg/script/configs.go:10-49`) | Wiring divergence; rework |
| P6 | `serverConfigsView` is the sole production implementation of `Configs` | Single hit at `modules/world/server_configs.go:11-181` | Wiring blind spot |
| P7 | TS `FontType.split` correctness pins translate cleanly | TS:140-176 read line-by-line at brainstorm; algorithm is straight while-loop with index update | Subtle wrap-boundary bug |
| P8 | `<p,NAME>` parser at `handlers_string.go:121-127` correctly extracts NAME between offsets 3 and `IndexByte(text, '>')` | Confirmed at brainstorm | Wrong name string → all lookups miss |

All eight premises **GREEN** at brainstorm-time.

## §10 — Deviations

This sub-spec **retires** two existing deviations:
- `NAI-75-D-MESANIM-NOT-PORTED`
- `NAI-75-D-FONT-WRAP-NAIVE`

**Net deviation tally:** N → N-2.

This sub-spec **opens** zero new deviations. Defensive-on-miss branches
in `handleSplitInit` (invalid fontId) and `handleSplitGetAnim` (nil
MesanimType / out-of-range lineCount) are labelled inline per
`defensive_gate_doc_comment_label.md` and are NOT tracked as
NAI-179-D-N tags — they're goscape defensive sugar on TS throws, not
behavioral divergences (script content does not pass invalid ids in
practice; we're matching TS's "abort the script step" outcome by
"silently no-op the SPLIT step").

## §11 — Cadence-specific notes

- **Subagent-driven-development.** Plan will dispatch each task as a
  separate Sonnet implementer.
- **Two-stage review NOT used** for T1/T2 (mechanical config-type ports
  matching the `IdkType` pattern; controller inline-verifies). T3 (FontType)
  gets a Sonnet code-reviewer pass on the `Split` algorithm because TS:140-176
  has subtle index arithmetic. T4 (handler wiring) gets a single inline
  review.
- **Smoke handoff** per `smoke_test_server_handoff.md`: after T4 (the
  handler-rewrite) the user launches the server + Java client and re-runs
  the Tutorial Island chatnpc flow. Pass criteria: (a) long chatnpc
  lines without `|` no longer overflow; (b) chathead anims play during
  `~chatnpc*` (controller cannot verify (b) directly — user observes).

## §12 — Open follow-ups created by this sub-spec

None expected. If FontType decode surfaces an unported branch (e.g.
some title-file variant we don't have), it gets an `NAI-179-D-N` tag
in the close commit, not at spec-write time.
