# NAI-WORDENC-FILTER — `WordEnc.filter` port

**Date:** 2026-05-19
**Predecessor:** [[post-D5 cleanup]] (HEAD `7fed104e`) on top of NAI-182-D5 close `c4df7ce0`
**Retires:** `DEVIATION-NAI-182-D5-NO-WORDENC-FILTER` (sendMessagePrivate currently calls `wordpack.Pack(chat)` without an equivalent of TS `WordEnc.filter`).

## 1. Goal

Port the TS profanity / URL / domain filter at `Engine-TS/src/cache/wordenc/{WordEnc,WordEncBadWords,WordEncFragments,WordEncDomains,WordEncTlds}.ts` (~1000 LOC across 5 classes) to Go. Wire it into the two call sites that emit filtered text: inbound private message delivery and outbound public-chat re-broadcast.

After this slice, the wire bytes for inbound PMs and broadcast public-chat will be byte-identical to TS Engine output on filterable text (e.g. profanity, leetspeak, bare URLs).

## 2. Scope

### In scope

- New Go package `pkg/wordenc/encfilter/`.
- `(*Filter).Filter(s string) string` matching TS `WordEnc.filter` algorithm.
- `encfilter.Load(cachePath string) (*Filter, error)` reading the existing `client/wordenc` jagfile (already built by goscape's pack pipeline — verified by smoke-pack 12/0 baseline at `c4df7ce0`).
- `*Filter` instance held on `*Server` (loaded once at `NewServer`).
- Two call-site changes: `sendMessagePrivate` (filter inbound PM text) and `handleMessagePublic` (unpack → filter → repack before delivery).
- Tests: algorithmic Go-level + TS-derived golden fixtures.

### Out of scope

- Reload-on-rebuild for wordenc data (wordenc cache is effectively static; add later if content edits to `wordenc/*.txt` become routine).
- Public package-level API (no global `Filter()` function — callers go through `s.wordenc`).
- The whitelist (`["cook", "cook's", "cooks", "seeks", "sheet"]`) — hardcoded in the port to match TS, not loaded from cache.
- Filtering anywhere else (chat-cheat outputs, server broadcasts, etc.) — only the two TS-call-site mirrors.

## 3. Architecture

### 3.1 Package layout

```
pkg/wordenc/encfilter/
  encfilter.go      — Filter struct + Load(cachePath) + Filter.Filter(s string) string  (mirrors WordEnc)
  helpers.go        — isAlpha / isSymbol / isNumerical / format / maskChars / replaceUppercases / formatUppercases / etc.
  badwords.go       — internal badWords filter (mirrors WordEncBadWords) — ~400 LOC including getEmulatedBadCharLen
  fragments.go      — internal fragments filter (mirrors WordEncFragments)
  domains.go        — internal domains filter (mirrors WordEncDomains)
  tlds.go           — internal tlds filter (mirrors WordEncTlds)
  encfilter_test.go — algorithmic tests + load tests
  fixtures_test.go  — TS-derived golden input/output pairs (loads JSON)
  testdata/wordenc-fixtures.json
```

`encfilter` is the package name (not `filter`, to avoid collision with the verb-method `Filter`).

### 3.2 Data model

- `Filter` struct holds the 4 decoded sections:
  - `fragments []uint16`
  - `bads [][]rune`, `badCombos [][][2]int` (2D parallel array — TS `Uint16Array[] bads` + `number[][][] badCombinations`)
  - `domains [][]rune`
  - `tlds [][]rune`, `tldTypes []int`
- Whitelist hardcoded: `var whitelist = []string{"cook", "cook's", "cooks", "seeks", "sheet"}`.
- Constants `PERIOD = []rune("dot")`, `AMPERSAT = []rune("(a)")`, `SLASH = []rune("slash")` — package-level since they're TS `WordEnc.PERIOD/AMPERSAT/SLASH` constants. The TS code stores them as `Uint16Array` but they're literal character sequences.

### 3.3 API

```go
package encfilter

// Filter is the world-side instance of the TS WordEnc profanity / URL / domain filter.
// One instance per Server. Construct via Load. Concurrent reads on Filter.Filter are safe;
// no mutation after Load.
type Filter struct {
    // unexported fields
}

// Load reads the jagfile at <cachePath>/client/wordenc and decodes the 4 sections
// (badenc.txt, fragmentsenc.txt, domainenc.txt, tldlist.txt). Returns a fully
// populated *Filter. Returns an error wrapping the underlying I/O or decode failure.
// Mirrors TS WordEnc.load (Engine-TS/src/cache/wordenc/WordEnc.ts:37-44) +
// WordEnc.readAll (:46-71).
func Load(cachePath string) (*Filter, error)

// Filter returns the filtered string. Profanity, leetspeak variants, and detected
// bare URLs / e-mail addresses are masked with '*' characters. Preserves uppercase
// in passthrough text. Mirrors TS WordEnc.filter (Engine-TS/src/cache/wordenc/WordEnc.ts:73-95).
func (f *Filter) Filter(s string) string
```

### 3.4 Runes vs bytes

TS works on JS character arrays (effectively `string[]` where each element is a 1-char string with `charCodeAt`). Go equivalent: `[]rune`.

`Filter.Filter` converts `string → []rune` at entry, operates on the rune slice, returns `string(chars)` at exit. Internal helpers all take `[]rune`. This matches TS exactly for the relevant character set (ASCII + `£`/`€` extended).

Byte-level operations are NOT used internally — the jagfile decoder reads raw bytes off the packet, but converts to runes before they enter the algorithm.

### 3.5 Server integration

```go
// In Server struct (modules/world/server.go around the other type configs):
wordenc *encfilter.Filter

// In NewServer, after the existing LoadXxxTypes block (~server.go:355-390):
s.wordenc, err = encfilter.Load(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load wordenc: %w", err)
}
```

The smoke-pack baseline shows the `Wordenc` packer stage runs and writes `client/wordenc` to the cache path. So `encfilter.Load` reading from `cfg.CachePath/client/wordenc` aligns with the existing convention.

If `client/wordenc` is absent (e.g. test paths that bypass pack), `Load` returns an error — callers must either provide a valid cache or use a test helper that constructs a `*Filter` from in-memory data. We add a `LoadFromJag(jf *jagfile.Jagfile) (*Filter, error)` helper for tests.

### 3.6 Call site: `sendMessagePrivate` (modules/world/friends_emit.go:54)

Current:
```go
buf := packet.NewPacket(nil)
buf.P8(from)
buf.P4(uint32(pmId))
buf.P1(uint8(adjusted))
wordpack.Pack(buf, chat)
```

After:
```go
buf := packet.NewPacket(nil)
buf.P8(from)
buf.P4(uint32(pmId))
buf.P1(uint8(adjusted))
wordpack.Pack(buf, p.client.server.wordenc.Filter(chat))
```

The existing per-encoder byte-pin tests (`friends_emit_test.go`) use simple inputs ("hello world", etc.) that are not filterable. They will need either:
- Updated expected bytes if the test input triggers filtering, OR
- A test seam: tests construct an `*encfilter.Filter` that passes through unchanged. We add `encfilter.Empty() *Filter` returning a Filter with zero rules — `Filter.Filter(s)` then returns `s` unchanged. Tests inject this via `newTestServer`.

The deviation comment at `friends_emit.go:50` is replaced with a positive doc-comment noting WordEnc.filter is applied.

### 3.7 Call site: `handleMessagePublic` (modules/world/handlers_game.go:336)

Current goscape behavior: receives raw word-packed bytes, passes them through to `p.Chat(...)`, then for audit-logging unpacks them via `wordpack.Unpack` to a readable string and sends to friends-bridge.

TS behavior (MessagePublicHandler.ts):
1. `unpack := WordPack.unpack(input)` — readable text
2. `player.logMessage = unpack` — store unfiltered text (audit)
3. `filtered := WordEnc.filter(unpack)` — apply filter
4. `WordPack.pack(filtered)` → `player.chatMessage` — repacked filtered bytes go on wire

After this slice, goscape mirrors TS:
1. Unpack raw bytes → `decoded`
2. Audit log uses `decoded` (unchanged; see DEVIATION below — friends bridge already receives unfiltered text)
3. Apply `s.wordenc.Filter(decoded)` → `filteredText`
4. Repack `wordpack.Pack(filteredText)` → `filteredBytes`
5. Pass `filteredBytes` to `p.Chat(...)` (not the raw input)

The friends-bridge audit logging path stays unfiltered — TS `player.logMessage = unpack` is the unfiltered text (TS sets logMessage BEFORE filtering). This matches goscape's current behavior; no change.

A new test pins that `handleMessagePublic` masks a known bad word in the wire bytes set on `p.chatBytes`, while the audit-log call to `friendsBridge.PublicMessage` still receives unfiltered text.

### 3.8 Test strategy

Two layers:

**Algorithmic tests** (~30 cases, `encfilter_test.go`):
- Direct bad words mask (`Filter("anal") == "****"`).
- Numeric leetspeak via `getEmulatedBadCharLen` (`Filter("4n4l")` matches if `getEmulatedBadCharLen('a','4', _) == 1`).
- Whitelist preserves (`Filter("cooks") == "cooks"`).
- Bare URL detection masks domain+TLD.
- Email `@` detection masks domain.
- Whitespace collapse from `format`.
- Uppercase preserved on non-masked chars (`Filter("Hello") == "Hello"`).
- Numerical-chars detection (`isNumericalChars`).
- Edge cases: empty string, single char, only symbols.

These tests use either `encfilter.Empty()` (no rules) or a small synthetic `*Filter` constructed from in-memory data — not the real jagfile.

**TS-derived golden fixtures** (`fixtures_test.go`):
- One file: `testdata/wordenc-fixtures.json` — `[{"input": "...", "filtered": "..."}]`.
- Generated by a one-shot TS script (committed at `tools/wordenc/gen-fixtures.ts`, not part of build): runs `WordEnc.load("data/pack"); WordEnc.filter(input)` on a curated input set, dumps to JSON.
- Test loads the real `client/wordenc` jagfile from a canonical cache path. If the cache is absent (CI without pack), test skips with `t.Skip("...")`. Matches `loctype_realcache_test.go` pattern.

### 3.9 Call-site test additions

- `friends_emit_test.go` — existing byte-pin tests for `sendMessagePrivate` use `newTestPlayer + newTestServer`; we extend `newTestServer` to inject an `encfilter.Empty()` so existing tests stay passthrough. Add one new test: `TestSendMessagePrivate_AppliesWordEncFilter` injects a real `*Filter` with one bad word, asserts wire bytes show the masked output.
- New test file `handler_message_public_filter_test.go` (or extend the existing `handler_message_public_test.go`): pins that `handleMessagePublic` masks a bad-word input on `p.chatBytes` and leaves `friendsBridge.PublicMessage` argument unchanged.

## 4. Deviations from TS

- **DEVIATION-NAI-WORDENC-FILTER-D-NO-RELOAD** — no `Reload` method on `*Filter`. Add when wordenc data becomes dynamic. Permanent until then.
- **DEVIATION-NAI-WORDENC-FILTER-D-STATEFUL-NOT-STATIC** — TS uses static methods + class-level state on `WordEnc`. Goscape uses an instance struct held on Server. Avoids global mutable state. Permanent design choice.
- **DEVIATION-NAI-WORDENC-FILTER-D-NO-PUBLIC-FILTER-FN** — TS lets any caller invoke `WordEnc.filter` directly. Goscape's `Filter.Filter` is only exposed through `*Server.wordenc`. Other callers route through Server. Permanent design choice.

## 5. Retired tags

- **DEVIATION-NAI-182-D5-NO-WORDENC-FILTER** — retired by this slice (T9 wires `sendMessagePrivate` to filter). The `friends_emit.go:50` doc-comment is removed.

## 6. Task plan summary

For the writing-plans skill to elaborate. T1-T11:

| # | Task | RED test | Files |
|---|------|----------|-------|
| T1 | encfilter package skeleton — `Filter` struct + `Load` jagfile decode of all 4 sections via synthetic in-memory jagfile | section-decode unit tests | `pkg/wordenc/encfilter/encfilter.go`, `helpers.go` |
| T2 | helpers (isAlpha/isSymbol/etc.) | direct unit tests | `helpers.go` |
| T3 | fragments.go + isBadFragment + filter | algorithm tests | `fragments.go` |
| T4 | badwords.go (largest — `getEmulatedBadCharLen` + `filter` + `filterBadCombinations` + `processBadCharacters`) | direct + leetspeak tests | `badwords.go` |
| T5 | domains.go | algorithm tests | `domains.go` |
| T6 | tlds.go | algorithm tests | `tlds.go` |
| T7 | top-level `Filter.Filter` composed; `Empty()` helper for tests; algorithmic E2E suite + TS-derived fixtures | full E2E | `encfilter.go`, `fixtures_test.go`, `testdata/wordenc-fixtures.json` |
| T8 | wire to `Server.wordenc` in `NewServer` | server-init test with cache present | `modules/world/server.go` |
| T9 | wire `sendMessagePrivate` to use Filter; extend `newTestServer` to inject `encfilter.Empty()`; new positive-filter byte-pin test | RED then GREEN | `friends_emit.go`, `friends_emit_test.go` |
| T10 | wire `handleMessagePublic` unpack→filter→repack; preserve unfiltered audit-log path; regression test | RED then GREEN | `handlers_game.go`, new test |
| T11 | retire `DEVIATION-NAI-182-D5-NO-WORDENC-FILTER` in spec doc + memory close | doc-only | `docs/superpowers/specs/2026-05-19-nai-182-d5-social-cluster-design.md` |

Roughly ~700 LOC Go + ~300 LOC tests. Race-clean + smoke-pack 12/0 as the standard gate at every commit.

## 7. Open questions for plan author

- Exact source layout for the one-shot TS fixtures generator (committed in-tree at `tools/wordenc/gen-fixtures.ts` vs out-of-tree). Default: in-tree.
- Whether `Filter.Filter` should normalize NFC/NFD-flavor Unicode (TS doesn't — it operates on raw codepoints). Default: match TS, no normalization.
- Whether to also expose the byte-array surface (`FilterBytes([]byte) []byte`) for callers that have already-encoded bytes. Default: no, only `string → string`.
