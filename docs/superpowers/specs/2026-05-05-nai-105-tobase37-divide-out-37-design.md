## NAI-105: ToBase37 divide-out-37 loop port

**Date**: 2026-05-05
**Cadence**: combined spec + plan, single end-of-impl review (per
`compressed_cadence.md`, ≤15 production-LOC threshold; 3 production
LOC + ~25 test LOC).
**Predecessor**: NAI-104 (HEAD `1c42102` — `ToDisplayName` operation
order fix).
**Trigger**: NAI-104 surfaced/deferred follow-up entry
`NAI-104-D-TOBASE37-DIVIDE-OUT-37` in `nai_followups.md`. NAI-104's
`TestToDisplayName` originally specced 6 cases; dropped
`"alice_smith_jr"` to 5 because its `ToBase37` encoding is divisible
by 37 and `FromBase37` rejects mod-37 values with `"invalid_name"` —
a pre-existing divergence independent of NAI-104's op-order bug.
**Tech stack**: Go 1.26+ (per `go_version.md`).
**Successor**: TBD; one new follow-up surfaced at spec-write time
(see §4 — `FromBase37` zero-rejection guard divergence).

### 1. Problem

`pkg/util/jstring/jstring.go:14-32` `ToBase37` is missing the trailing-
factor-of-37 strip that TS does at `Engine-TS/src/util/JString.ts:21-23`.
After the encode loop, TS divides out trailing factors of 37 so the
stored value is never divisible by 37 (unless 0). Goscape stores the
raw encoded value.

The downstream impact: any input whose `ToBase37` encoding happens to
be divisible by 37 hits goscape's `FromBase37` mod-37 rejection at
`:42-44` and the round-trip returns `"invalid_name"`. Through
`ToDisplayName`, that surfaces as the literal display string
`"Invalid Name"` instead of the correct title-cased name.

**Empirically (verified at spec-write, HEAD `1c42102`):**

The `ToBase37` encoding `l = sum(c_i · 37^(L-1-i)) mod 37` reduces to
`c_{L-1}`, the lookup value of the **last** character. The base37
lookup table maps `'_' → 0`. Therefore `ToBase37(N) % 37 == 0` iff
`N` ends with `'_'` (or `N` is empty / whitespace-only).

Concrete pre-fix shapes:

- `ToDisplayName("alice_")` → `"Invalid Name"` (goscape) vs `"Alice"`
  (TS).
- `ToDisplayName("alice_smith_")` → `"Invalid Name"` (goscape) vs
  `"Alice Smith"` (TS).
- `ToDisplayName("alice_smith_jr")` (input length 14, truncates to
  first 12 chars `"alice_smith_"`) → `"Invalid Name"` (goscape) vs
  `"Alice Smith"` (TS — 12-char truncation is shared by TS, see §2).

**Pre-flight correction surfaced at spec-write:** the
`nai_followups.md` `NAI-104-D-TOBASE37-DIVIDE-OUT-37` entry
prescribes reinstating the dropped `TestToDisplayName` case as
`"alice_smith_jr" → "Alice Smith Jr"`. That expected value is
incorrect — the 12-char encode truncation (shared by both goscape
and TS) drops the trailing `"jr"`, so the post-fix round-trip
produces `"Alice Smith"`, not `"Alice Smith Jr"`. T1 below
reinstates the case with the corrected expected value.

### 2. TS source (canonical)

**`Engine-TS/src/util/JString.ts:1-26`** — `toBase37`:

```typescript
export function toBase37(string: string): bigint {
    string = string.trim();
    let l: bigint = 0n;

    for (let i: number = 0; i < string.length && i < 12; i++) {
        const c: number = string.charCodeAt(i);
        l *= 37n;

        if (c >= 0x41 && c <= 0x5a) {
            // A-Z
            l += BigInt(c + 1 - 0x41);
        } else if (c >= 0x61 && c <= 0x7a) {
            // a-z
            l += BigInt(c + 1 - 0x61);
        } else if (c >= 0x30 && c <= 0x39) {
            // 0-9
            l += BigInt(c + 27 - 0x30);
        }
    }

    while (l % 37n === 0n && l !== 0n) {
        l /= 37n;
    }

    return l;
}
```

The exact 3-line stripper at `:21-23` is the divergence. Both engines
share the `i < 12` truncation at `:5` and the trim at `:2`.

**`Engine-TS/src/util/JString.ts:36-55`** — `fromBase37`:

```typescript
export function fromBase37(value: bigint): string {
    // >= 37 to the 12th power
    if (value < 0n || value >= 6582952005840035281n) {
        return 'invalid_name';
    }

    if (value % 37n === 0n) {
        return 'invalid_name';
    }
    /* ... decode loop ... */
}
```

After the divide-out loop is added to `ToBase37`, properly-encoded
inputs cannot reach the `FromBase37` mod-37 reject path; that path
remains as defense-in-depth against malformed direct calls.

### 3. Solution

#### 3.1 Production change

**(P1)** `pkg/util/jstring/jstring.go` — insert the divide-out loop
between the encode loop (closes line 29) and `return l` (line 31):

```go
for l != 0 && l%37 == 0 {
    l /= 37
}
```

3-LOC body, no new imports, no signature change. Mirrors TS
`JString.ts:21-23` exactly (Go's `uint64` is sufficient; max value
post-encode is `37^12 - 1 = 6582952005840035280`, within `uint64`
range; `l != 0` guards the integer-divide trap on the zero case
exactly as TS's `l !== 0n` does).

#### 3.2 Test changes

**(T1)** `pkg/util/jstring/jstring_test.go` — add a new test
function pinning the loop's behavior directly on `ToBase37`:

```go
func TestToBase37DividesOutTrailing37(t *testing.T) {
    // TS JString.ts:21-23 — after encoding, trailing factors of 37 are
    // divided out so the stored value is never divisible by 37 (except 0).
    // The base37 lookup maps '_' → 0; any trailing '_' makes the raw
    // encoding divisible by 37.
    base := ToBase37("alice")
    if base%37 == 0 {
        t.Fatalf("test fixture invariant: ToBase37(%q) = %d unexpectedly divisible by 37", "alice", base)
    }
    cases := []struct {
        in   string
        want uint64 // expected ToBase37 output post-divide-out
    }{
        {"alice", base},
        {"alice_", base},  // single trailing '_' divided out
        {"alice__", base}, // multiple trailing '_' divided out iteratively
    }
    for _, c := range cases {
        if got := ToBase37(c.in); got != c.want {
            t.Errorf("ToBase37(%q): got %d, want %d", c.in, got, c.want)
        }
    }
    // Post-divide-out invariant: nonzero outputs are never divisible by 37.
    for _, in := range []string{"alice_", "alice__", "alice_smith_"} {
        if v := ToBase37(in); v != 0 && v%37 == 0 {
            t.Errorf("ToBase37(%q) = %d post-divide-out invariant violated", in, v)
        }
    }
}
```

**(T2)** `pkg/util/jstring/jstring_test.go` — extend `TestToDisplayName`
(currently 5 cases) with three regression fixtures, and clean up the
deferral comment header (currently `:33-36`):

```go
func TestToDisplayName(t *testing.T) {
    // Trailing-underscore inputs round-trip post-NAI-105 divide-out-37
    // loop in ToBase37 (TS JString.ts:21-23). "alice_smith_jr" is 14
    // chars; the 12-char encode truncation (shared with TS, JString.ts:5)
    // drops the trailing "jr" before the divide-out runs, so the
    // post-fix round-trip is "Alice Smith", not "Alice Smith Jr".
    cases := []struct {
        in   string
        want string
    }{
        {"", ""},
        {"alice", "Alice"},
        {"user_two", "User Two"},
        {"USER_TWO", "User Two"}, // case-insensitive via base37 round-trip
        {"player1", "Player1"},   // digits inside a token
        {"alice_", "Alice"},                  // single trailing '_'
        {"alice_smith_", "Alice Smith"},      // multi-word + trailing '_'
        {"alice_smith_jr", "Alice Smith"},    // 14-char input, truncated at 12
    }
    for _, c := range cases {
        if got := ToDisplayName(c.in); got != c.want {
            t.Errorf("ToDisplayName(%q): got %q, want %q", c.in, got, c.want)
        }
    }
}
```

**(T3)** Existing `TestFromBase37InvalidNameMod37` and
`TestFromBase37ValidNameDecodes` are **not** modified. `FromBase37`
is not touched; its mod-37 guard remains as defense-in-depth (just
unreachable for inputs that flowed through the patched `ToBase37`,
matching TS's invariant at `JString.ts:42-44`).

### 4. Out of scope

- **`FromBase37` zero-rejection guard divergence**.
  `pkg/util/jstring/jstring.go:42` reads `if v != 0 && v%37 == 0`
  (rejects nonzero mod-37 values). TS `JString.ts:42` reads
  `if (value % 37n === 0n)` — TS rejects `value=0` too (returns
  `"invalid_name"`). The narrow user-visible impact is
  `ToDisplayName("") → ""` (goscape) vs `"Invalid Name"` (TS). Not
  in NAI-105 scope; flagged as a new follow-up entry
  `NAI-105-D-FROMBASE37-ZERO-REJECT` at close.
- **`strings.Title` deprecation**. Per NAI-104 §4 — out of scope here
  too.
- **`FromBase37` `chars[12-l:]` slice TODO comment** at line 55. Pre-
  existing TODO, behavior is correct (last `l` chars of the buffer);
  not in NAI-105 scope.
- **DAMAGE opcode 2015**. Original NAI-104 candidate B fallthrough,
  deferred again — preserved for a later sub-spec when its TS
  reference is re-grepped.
- **SPLIT_* font-aware wrap**, **Survival Expert / Hans pathfinder
  reach residual**, **NAI-85-D-LC_NAME-FIELD-CHOICE** — all
  preserved as carry-forwards.

### 5. Deviations introduced

**None.** Full TS-faithful port; this spec brings goscape **closer**
to TS by retiring an unintended divergence.

### 6. Deviations retired

- **`NAI-104-D-TOBASE37-DIVIDE-OUT-37`** — retired by P1. Re-grep at
  impl time:
  - `rg "l % 37" pkg/util/jstring/jstring.go` → 2 matches (the new
    divide-out guard at `ToBase37` and the existing `FromBase37`
    mod-37 reject).
  - Direct empirical pin: T1 asserts `ToBase37("alice_") ==
    ToBase37("alice")`; T2 asserts the user-visible
    `ToDisplayName("alice_smith_jr") → "Alice Smith"`.
- **NAI-104 `TestToDisplayName` deferral comment** at
  `pkg/util/jstring/jstring_test.go:33-36` — replaced by T2's new
  comment header documenting the divide-out + 12-char truncation
  semantics.

### 7. Implementation plan (subagent-driven, single bundle)

Single subagent dispatch covers all changes; compressed cadence
skips formal review.

**Bundle 1: ToBase37 divide-out-37 loop (single dispatch)**

Tasks for the implementer (TDD per
`superpowers:test-driven-development`):

1. **T1 (TDD, fail-test for ToBase37)**: Add
   `TestToBase37DividesOutTrailing37` to
   `pkg/util/jstring/jstring_test.go` exactly as in §3.2 T1. Run
   `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test
   ./pkg/util/jstring/...`. Expected RED:
   `ToBase37("alice_")` and `ToBase37("alice__")` produce values
   different from `ToBase37("alice")` (the trailing-`_` factors of
   37 are still attached pre-fix). The `base%37 == 0` Fatalf guard
   should NOT fire (ToBase37("alice") = 2494434, mod 37 = 5).

2. **T2 (TDD, fail-test for ToDisplayName)**: Replace the existing
   `TestToDisplayName` body (currently
   `pkg/util/jstring/jstring_test.go:32-52`) with the expanded
   version in §3.2 T2. Re-run package tests. Expected RED on the
   three new cases (`"alice_"`, `"alice_smith_"`,
   `"alice_smith_jr"`): all currently produce `"Invalid Name"`
   pre-fix.

3. **T3 (RED→GREEN, divide-out loop)**: Edit
   `pkg/util/jstring/jstring.go` — insert the 3-LOC divide-out
   loop between line 29 (`}` of the encode loop) and line 31
   (`return l`):
   ```go
       for l != 0 && l%37 == 0 {
           l /= 37
       }
   ```
   Re-run `go test ./pkg/util/jstring/...`. All cases in T1 and T2
   should pass.

4. **T4 (verification)**: Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache
   go test ./...`. Expect clean — no other call site regresses.
   Specifically, existing `TestFromBase37InvalidNameMod37`,
   `TestFromBase37InvalidNameUpperBound`,
   `TestFromBase37ValidNameDecodes` should continue passing
   unchanged (the fix doesn't touch `FromBase37`, and `ToBase37`
   on names without trailing `_` is unchanged).

   Re-grep checks:
   - `rg "l % 37|l%37" pkg/util/jstring/jstring.go` → 2 matches
     (the new `ToBase37` guard and the existing `FromBase37`
     reject).
   - `rg "ToBase37" --include='*.go' .` should show the same
     external surface as pre-fix plus T1's new test references.

5. **T5 (close commit)**: Single chore(close) commit. Body lists:
   - Retired follow-up: `NAI-104-D-TOBASE37-DIVIDE-OUT-37`.
   - Pre-flight correction noted: original followup expected
     `"Alice Smith Jr"` was wrong (12-char truncation); spec
     reinstates with `"Alice Smith"`.
   - New follow-up surfaced at spec-write:
     `NAI-105-D-FROMBASE37-ZERO-REJECT` (record in
     `nai_followups.md` at close).
   - `Closes memory:` trailer per
     `close_commit_memory_trailer.md` — entry is
     `NAI-104-D-TOBASE37-DIVIDE-OUT-37` from `nai_followups.md`.

### 8. Risk register

- **R1 — Other `ToBase37` callers whose tests pin output bytes?**
  [GREEN, pre-flighted at spec-write].
  `pkg/util/jstring/jstring.go:14`-only callers: `ToSafeName` (line
  63) and `TestFromBase37ValidNameDecodes` (line 23 of the test
  file, uses `"alice"` which is not divisible by 37 — round-trip
  unchanged). External callers must be confirmed at impl via
  `rg "ToBase37\\(" --include='*.go' .` — any caller that pinned a
  literal numeric output for a trailing-`_` input would break, but
  no such caller is expected (NAI-72 surfaced no such pin).

- **R2 — Other `ToDisplayName` callers whose tests pin output?**
  [GREEN]. Per NAI-104 §8 R2, the surface is decl + newPlayer +
  one helper-as-oracle test. None pin literal trailing-`_`
  outputs.

- **R3 — `FromBase37` mod-37 reject becoming dead code?** [GREEN by
  design]. Post-P1, the `FromBase37` mod-37 reject path is
  unreachable for inputs that flowed through the patched
  `ToBase37` — exactly matching TS's invariant. The path remains
  for direct `FromBase37(v)` calls with malformed `v`; behavior
  is identical to TS. No code removal needed.

- **R4 — Wire-format / on-the-wire regression?** [GREEN]. `ToBase37`
  is used in `Player.username37` plumbing (NAI-103 surface) and in
  `ToSafeName` / `ToDisplayName`. The change makes a previously-
  rejected codepath now succeed (`"invalid_name"` → correct
  title-cased name). For the username flow, the practical effect is
  that a player whose username ends in `_` (or whose 12-char-
  truncated form ends in `_`) now displays correctly instead of
  showing `"Invalid Name"`. For Player.username37, the long
  encoding is also normalized (trailing factors of 37 stripped),
  bringing it in line with TS's stored value. Any persisted
  values from prior runs would be raw-encoded; this is a fresh-
  state codebase (no prod data to migrate), so no compatibility
  concern.

- **R5 — `uint64` overflow on the divide-out loop?** [GREEN]. Max
  pre-divide value is `37^12 - 1 = 6582952005840035280 < 2^63`,
  well within `uint64`. The loop only divides; it cannot grow `l`.

- **R6 — Whitespace-only input causing infinite loop?** [GREEN].
  `ToBase37("   ")` → after `TrimSpace` → `""` → encode loop
  doesn't run → `l = 0` → divide-out loop's `l != 0` guard skips →
  return 0. Identical pre/post behavior. The loop's `l != 0`
  guard mirrors TS's `l !== 0n` exactly.

### 9. Verification before close

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean.
- `rg "l % 37|l%37" pkg/util/jstring/jstring.go` → 2 matches.
- `rg "ToBase37" --include='*.go' .` → external surface unchanged
  apart from the new T1 references.
- `rg "alice_smith_jr" --include='*.go' .` → 1 match in
  `TestToDisplayName` (reinstated), expected value `"Alice Smith"`.
- `git show HEAD --stat` matches stated bundle scope: 2 files
  touched (`pkg/util/jstring/jstring.go`,
  `pkg/util/jstring/jstring_test.go`); no stray worktree writes
  (per `feedback_subagent_wt_path.md`).

### 10. Notes

- This is the second consecutive compressed-cadence sub-spec on
  `pkg/util/jstring/`; both born from the
  `smoke_surfaces_adjacent_divergences` pattern (NAI-103 surfaced
  the `ToDisplayName` op-order bug → NAI-104; NAI-104's deferred
  test case surfaced this `ToBase37` divide-out gap → NAI-105).
- The pre-flight correction (`"Alice Smith Jr"` → `"Alice Smith"`)
  is a fresh data point for `controller_preflight.md`: tracker /
  follow-up entries can carry incorrect expected values when the
  author hasn't traced the full data path (here, the 12-char
  truncation interaction with the divide-out loop). Catching it
  pre-dispatch saved an implementer cycle.
- New follow-up `NAI-105-D-FROMBASE37-ZERO-REJECT` will be
  appended to `nai_followups.md` at close — the goscape
  `v != 0 &&` guard in `FromBase37`'s mod-37 reject is narrower
  than TS's and produces `ToDisplayName("") → ""` vs TS's
  `"Invalid Name"`. Out of NAI-105 scope; standalone polish for a
  later sub-spec.
