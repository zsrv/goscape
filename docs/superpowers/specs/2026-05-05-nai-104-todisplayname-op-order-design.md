## NAI-104: ToDisplayName operation order fix

**Date**: 2026-05-05
**Cadence**: combined spec + plan, single end-of-impl review (per
`compressed_cadence.md`, ≤15 production-LOC threshold; 1 production
LOC + ~25 test LOC).
**Predecessor**: NAI-103 (HEAD `6c17228` — DISPLAYNAME opcode 2016 +
`Player.displayName` plumbing).
**Trigger**: NAI-103 opportunistic Tutorial Island smoke (2026-05-05).
Primary signal ✅ — chatplayer_page WARN at pc=24 silenced. Adjacent
divergence surfaced: username `user_two` rendered as `"User two"` in
the chat dialog box; TS would render `"User Two"`.
**Tech stack**: Go 1.26+ (per `go_version.md`).
**Successor**: TBD; no residuals expected.

### 1. Problem

`pkg/util/jstring/jstring.go:66-68` `ToDisplayName` composes its
helpers in the wrong order:

```go
func ToDisplayName(s string) string {
    return strings.ReplaceAll(ToTitleCase(ToSafeName(s)), "_", " ")
}
```

Order: `ToSafeName` → `ToTitleCase` → replace `_` → space. Go's
`strings.Title` does **not** treat `_` as a word boundary (Unicode
word-break rules), so `strings.Title("user_two")` returns
`"User_two"` (only the leading word capitalized). Replacing the `_`
afterward yields `"User two"`.

TS composes them in the opposite order: replace `_` first (creating a
real space-separated string), THEN title-case (the regex `\w\S*`
matches each space-separated token). Result: `"User Two"`.

Empirically confirmed via runtime probe at spec-write:
- goscape `ToDisplayName("user_two")` → `"User two"`
- TS-equivalent ordering → `"User Two"`

The bug is benign for single-word usernames (which round-trip through
both orders identically) and only surfaces when the safe-name
contains an internal underscore (i.e. multi-word display names).

NAI-103's `TestNewPlayer_PopulatesDisplayName` did not catch this
because it uses the helper as its own oracle:
```go
want := util.ToDisplayName("alice_smith")
if p.displayName != want { ... }
```
Round-trip equality holds even with the buggy order; the test pins
**wiring** (newPlayer calls the helper), not output **shape**. This
is the correct shape for a wiring test — the output-shape contract
belongs in `pkg/util/jstring/`, where this spec adds it.

### 2. TS source (canonical)

**`Engine-TS/src/util/JString.ts:65-67`** — composition:

```typescript
export function toDisplayName(name: string): string {
    return toTitleCase(toSafeName(name).replaceAll('_', ' '));
}
```

**`Engine-TS/src/util/JString.ts:57-59`** — toTitleCase:

```typescript
export function toTitleCase(str: string): string {
    return str.replace(/\w\S*/g, (txt: string): string => txt.charAt(0).toUpperCase() + txt.substr(1).toLowerCase());
}
```

**Observations:**
- The regex `\w\S*` matches a word character followed by any
  non-whitespace; for each match, the callback uppercases char[0]
  and lowercases the remainder. Effective behavior: every
  whitespace-separated token gets title-cased.
- `replaceAll('_', ' ')` runs **before** the title-case, so the
  underscore-separated original becomes space-separated, allowing
  the regex to title-case each word.
- `toSafeName` is base37 round-trip (`fromBase37(toBase37(name))`),
  which lowercases all input letters via the encoding — so the input
  to title-case is always all-lowercase ASCII `[a-z0-9 _]` (or `_`
  surfaces as space after replacement). This means Go's
  `strings.Title` (which doesn't lowercase the rest of each token,
  unlike TS's regex) produces identical output to TS on the
  restricted alphabet — the **only** divergence is the
  composition order.

### 3. Solution

#### 3.1 Production change

**(P1)** `pkg/util/jstring/jstring.go:67` — swap the composition
order to mirror TS exactly:

```go
func ToDisplayName(s string) string {
    return ToTitleCase(strings.ReplaceAll(ToSafeName(s), "_", " "))
}
```

One-line change. No new imports. No signature change.

#### 3.2 Test changes

**(T1)** `pkg/util/jstring/jstring_test.go` — add a table-driven test
`TestToDisplayName` adjacent to the existing `FromBase37` tests
(file already exists; package `util`). Cases pin literal expected
outputs against TS's contract:

```go
func TestToDisplayName(t *testing.T) {
    cases := []struct {
        in   string
        want string
    }{
        {"", ""},
        {"alice", "Alice"},
        {"user_two", "User Two"},
        {"alice_smith_jr", "Alice Smith Jr"},
        {"USER_TWO", "User Two"},  // case-insensitive via base37 round-trip
        {"player1", "Player1"},    // digits inside a token
    }
    for _, c := range cases {
        if got := ToDisplayName(c.in); got != c.want {
            t.Errorf("ToDisplayName(%q): got %q, want %q", c.in, got, c.want)
        }
    }
}
```

The `"user_two"` → `"User Two"` case is the regression fixture for
this sub-spec; the other 5 cases pin the broader output contract
that no existing test covered.

NAI-103's `TestNewPlayer_PopulatesDisplayName` is **not** modified
(per Option B from brainstorming): it remains the wiring pin for
`newPlayer` calling `util.ToDisplayName`. Output-shape regressions
are now owned by T1 in the jstring package, where they belong.

### 4. Out of scope

- **`strings.Title` deprecation**. Deprecated since Go 1.18 in favor
  of `golang.org/x/text/cases.Title(language.English)`. Works
  correctly on the restricted lowercase-ASCII alphabet that
  `toSafeName` produces, so behavior is TS-faithful as-is.
  Modernization is a separate refactor (introduces a new module
  dependency).
- **Long-username truncation behavior**. `ToBase37` truncates to 12
  characters silently; usernames longer than 12 will display as
  `ToDisplayName(firstTwelveChars)`. This matches TS (TS's
  `toBase37` also truncates at 12). Not a divergence, no action.
- **DAMAGE opcode 2015**. The original NAI-104 candidate B
  fallthrough; deferred to NAI-105.
- **SPLIT_* font-aware wrap** at `pkg/script/handlers_string.go:97`.
  Per NAI-103 spec §4; depends on FontType / MesanimType cache
  loaders.
- **Survival Expert / Hans pathfinder reach residual** (NAI-92/94
  carry-forward). Larger investigation, separate sub-spec.

### 5. Deviations introduced

**None.** Full TS-faithful port; this spec brings goscape **closer**
to TS by retiring an unintended divergence.

### 6. Deviations retired

- **NAI-103 smoke residual: `ToDisplayName` operation order
  divergence** — retired by P1. Re-grep at impl time:
  `rg "ReplaceAll" pkg/util/jstring/jstring.go` → 1 match, must be
  inside `ToTitleCase(...)` argument (i.e. inner position), not
  outer. Direct empirical pin: T1's `"user_two"` → `"User Two"`
  case asserts the regression is fixed.

### 7. Implementation plan (subagent-driven, single bundle)

Single subagent dispatch covers all changes; compressed cadence skips
formal review.

**Bundle 1: ToDisplayName op-order fix (single dispatch)**

Tasks for the implementer (TDD per
`superpowers:test-driven-development`):

1. **T1 (TDD, fail-test)**: Add `TestToDisplayName` to
   `pkg/util/jstring/jstring_test.go` with the 6 cases listed in §3.2.
   Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test
   ./pkg/util/jstring/...`. Expected RED: the `"user_two"` case fails
   with `got "User two", want "User Two"`. The other 5 cases pass on
   the buggy code (verify they pass — if any other case unexpectedly
   fails, halt and surface).

2. **T2 (RED→GREEN, op-order swap)**: Edit
   `pkg/util/jstring/jstring.go:67` to:
   ```go
   return ToTitleCase(strings.ReplaceAll(ToSafeName(s), "_", " "))
   ```
   Re-run `go test ./pkg/util/jstring/...`. All 6 cases pass.

3. **T3 (verification)**: Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache
   go test ./...`. Expect clean — no other call site regresses.
   Specifically, `TestNewPlayer_PopulatesDisplayName` in
   `modules/world/login_username_test.go` continues to pass (it uses
   the helper as its own oracle, so it's invariant under this
   change).

   Re-grep checks:
   - `rg "ReplaceAll" pkg/util/jstring/jstring.go` → 1 match, with
     the call now inside `ToTitleCase(...)` (inner position).
   - `rg "ToDisplayName" --include='*.go' .` → 4 matches: decl,
     login_username_test.go (comment + use), player.go (newPlayer),
     jstring_test.go (T1) — same surface as pre-fix plus T1.

4. **T4 (close commit)**: Single chore(close) commit. Body lists
   retired deviation (NAI-103 smoke residual) and `Closes memory:`
   trailer. Per `close_commit_memory_trailer.md`, no NAI-104-specific
   tracker entry exists to retire (one-shot fix, not a tracker-bound
   cascade); trailer reads `(none)` or is omitted.

### 8. Risk register

- **R1 — Other `ToTitleCase` callers?** [GREEN, pre-flighted at
  spec-write]. `rg "ToTitleCase" --include='*.go' .` returns exactly
  2 matches: the decl at `pkg/util/jstring/jstring.go:58` and the
  internal call at `pkg/util/jstring/jstring.go:67`. No external
  callers. The fix moves the `ToTitleCase` call into the outer
  position; no consumer can observe a change in its standalone
  behavior because no consumer calls it standalone.

- **R2 — Other `ToDisplayName` callers whose tests pin output?**
  [GREEN, pre-flighted at spec-write]. `rg "ToDisplayName"
  --include='*.go' .` returns 4 matches:
  `pkg/util/jstring/jstring.go:66` (decl),
  `modules/world/player.go:434` (newPlayer call),
  `modules/world/login_username_test.go:20` (comment),
  `modules/world/login_username_test.go:27` (helper-as-oracle test).
  No third caller; no test asserts a literal output string against
  `ToDisplayName` output anywhere outside this sub-spec's T1.

- **R3 — Wire-format / on-the-wire regression?** [GREEN].
  `displayName` is read by exactly one site: the DISPLAYNAME script
  opcode handler (`pkg/script/handlers_player.go:531-537` post-NAI-103),
  which pushes the string onto the script stack. The change in
  output is the **intended** correction — anywhere the prior buggy
  output was being rendered (only chatplayer_page in the smoke
  matrix so far), the corrected output replaces it. No wire-format
  byte-shape change: it's a string content change.

- **R4 — `strings.Title` Unicode edge cases?** [GREEN].
  `toSafeName`'s base37 round-trip restricts the input alphabet to
  `[a-z0-9 _]` (after replace). Within this alphabet, Go's
  `strings.Title` and TS's regex behavior are equivalent: each
  whitespace-separated token gets char[0] uppercased, with the
  remainder unchanged (already lowercase from base37). No
  multi-byte / locale / non-ASCII concerns possible.

- **R5 — Smoke re-confirmation** [INFO]. Post-impl, user
  opportunistically re-launches Tutorial Island; on a username with
  an internal underscore (e.g. `user_two`), the chatplayer_page
  dialog should render `"User Two"`. Single-word usernames are
  invariant under this change. Not a gate on close; T1's literal
  pins are the binding evidence.

### 9. Verification before close

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean.
- `rg "ReplaceAll" pkg/util/jstring/jstring.go` → 1 match, inside
  the `ToTitleCase(...)` argument (inner position).
- `rg "ToDisplayName" --include='*.go' .` → 4 matches (decl + 3
  call/comment sites; T1 contributes 1 of those 3 via
  `jstring_test.go`).
- `git show HEAD --stat` matches stated bundle scope: 2 files
  touched (`pkg/util/jstring/jstring.go`,
  `pkg/util/jstring/jstring_test.go`); no stray worktree writes
  (per `feedback_subagent_wt_path.md`).

### 10. Notes

This is a textbook compressed-cadence sub-spec born from the
`smoke_surfaces_adjacent_divergences` pattern: NAI-103's primary
signal closed cleanly (chatplayer_page WARN silenced), the smoke
binding surfaced an adjacent untracked divergence (in the very
helper NAI-103 had just plumbed), and the fix is 1 production LOC.
The test-discipline lesson — that helper-as-oracle test patterns
hide helper bugs — is captured as a memory entry at close.
