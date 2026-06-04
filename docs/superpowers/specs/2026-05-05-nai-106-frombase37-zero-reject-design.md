## NAI-106: FromBase37 zero-rejection guard narrowing

**Date**: 2026-05-05
**Cadence**: combined spec + plan, single end-of-impl review (per
`compressed_cadence.md`, ≤15 production-LOC threshold; 1 production
LOC + 1 test fixture edit).
**Predecessor**: NAI-105 (HEAD `b6d616f` — `ToBase37` divide-out-37
loop port).
**Trigger**: NAI-105 surfaced/deferred follow-up entry
`NAI-105-D-FROMBASE37-ZERO-REJECT` in `nai_followups.md`. Goscape's
`FromBase37` mod-37 reject carries an extra `v != 0 &&` guard not
present in TS; the narrow user-visible impact is
`ToDisplayName("") → ""` (goscape) vs `"Invalid Name"` (TS).
**Tech stack**: Go 1.26+ (per `go_version.md`).
**Successor**: TBD; no new follow-up surfaced at spec-write time.

### 1. Problem

`pkg/util/jstring/jstring.go:46` reads:

```go
if v != 0 && v%37 == 0 {
    return "invalid_name"
}
```

TS `Engine-TS/src/util/JString.ts:42` reads:

```typescript
if (value % 37n === 0n) {
    return 'invalid_name';
}
```

TS rejects `value = 0` (since `0n % 37n === 0n` is true); goscape
short-circuits via the `v != 0 &&` guard and falls through to the
decode loop, which produces `""` (the empty slice `chars[12:]` since
`l` never increments).

The narrow user-visible impact:
- `FromBase37(0) → ""` (goscape) vs `"invalid_name"` (TS).
- `ToSafeName("") → ""` (goscape) vs `"invalid_name"` (TS).
- `ToDisplayName("") → ""` (goscape) vs `"Invalid Name"` (TS) —
  `strings.ReplaceAll("invalid_name", "_", " ") = "invalid name"` →
  `ToTitleCase("invalid name") = "Invalid Name"`.

Empty-string and whitespace-only `ToBase37` inputs are the only
triggers. Post-NAI-105 `ToBase37` never returns 0 for any non-empty
non-whitespace input, so all real encoded names route around this
guard.

### 2. TS source (canonical)

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

    let len: number = 0;
    const chars: string[] = Array(12);
    while (value !== 0n) {
        const l1: bigint = value;
        value /= 37n;
        chars[11 - len++] = BASE37_LOOKUP[Number(l1 - value * 37n)];
    }

    return chars.slice(12 - len).join('');
}
```

The `value % 37n === 0n` reject at `:42` is unconditional (the
`value < 0n` guard at `:38` is a separate range gate, not relevant
to `value = 0`).

**Note on TS `value < 0n` guard at `:38`**: goscape's `FromBase37`
takes `uint64`, which by type cannot represent negative values. The
`value < 0n` guard is structurally unreachable in goscape and is
**not** a tracked divergence; it is correctly elided by the type
system.

### 3. Solution

#### 3.1 Production change

**(P1)** `pkg/util/jstring/jstring.go:46` — drop the `v != 0 &&`
guard:

```go
// before:
if v != 0 && v%37 == 0 {
    return "invalid_name"
}

// after:
if v%37 == 0 {
    return "invalid_name"
}
```

1-LOC body change. The doc comment at `:44-45` (`Mirrors TS
JString.ts:42-44 — values divisible by 37 are invalid (NAI-72:
surfaced by social handler invalid_name gate).`) stays — the
post-NAI-106 code is now an exact mirror.

The decode loop at `:50-57` is unaffected; it never executed when
`v = 0` reached it pre-fix (the `for v != 0` guard short-circuited
immediately), so removing the upstream short-circuit doesn't expose
new code paths.

#### 3.2 Test changes

**(T1)** `pkg/util/jstring/jstring_test.go:42` — update the
`TestToDisplayName` empty-string fixture from `{"", ""}` to
`{"", "Invalid Name"}`:

```go
{"", "Invalid Name"},
```

This pins the post-fix downstream propagation through
`ToDisplayName`. No other fixtures change.

**(T2)** Existing `TestFromBase37InvalidNameMod37`,
`TestFromBase37InvalidNameUpperBound`,
`TestFromBase37ValidNameDecodes`, `TestToBase37DividesOutTrailing37`
are **not** modified. None of them feed `v = 0` to `FromBase37`
directly:
- `TestFromBase37InvalidNameMod37` cases: `{37, 74, 1369, 37*12345}`
  — all nonzero, behavior unchanged (still hit the (now-unconditional)
  reject path, return `"invalid_name"`).
- `TestFromBase37InvalidNameUpperBound`: `6582952005840035281` — hits
  the upper-bound guard at `:40-42`, never reaches the mod-37 check.
- `TestFromBase37ValidNameDecodes`: `ToBase37("alice")` → nonzero
  non-mod-37 value, decode round-trips unchanged.
- `TestToBase37DividesOutTrailing37`: pins `ToBase37` only, no
  `FromBase37` calls.

### 4. Out of scope

- **Downstream caller behavior change at production sites**.
  Six production sites consume `FromBase37` (directly or via
  `ToSafeName` / `ToDisplayName`):
  - `modules/world/server.go:591` — `util.ToSafeName(req.Username)`.
    `req.Username` is wire-validated for length 1-20 at `:579-580`
    (caller-side); empty string never reaches here.
  - `modules/world/player.go:434` — `util.ToDisplayName(c.username)`.
    `c.username` populated from login flow; empty would be a malformed
    login already rejected upstream.
  - `modules/world/handler_reportabuse.go:60, 67, 72` — three
    `util.FromBase37(offender)` calls where `offender` is a `uint64`
    read directly off the wire (`pk.G8()`). Pre-NAI-106: an offender
    of `0` is logged/looked-up/notified as `""`; post-NAI-106: as
    `"invalid_name"`. `LookupPlayerByUsername("invalid_name")`
    returns nil exactly as `LookupPlayerByUsername("")` did. Bridge
    notification payloads change from `""` to `"invalid_name"` for
    this degenerate case — TS-faithful, intended.
  - `modules/world/handler_social_list.go:39` — gates on
    `util.FromBase37(username) == "invalid_name"`. Pre-NAI-106 with
    `username=0`: returns `""`, gate is false, social action proceeds
    against the all-zero username. Post-NAI-106: returns
    `"invalid_name"`, gate is true, social action is suppressed.
    This is the **intended** behavioral correction — the gate exists
    precisely to drop invalid usernames.
  None of these sites branch on `== ""` literal; verified by grep at
  spec-write. The behavioral changes are TS-faithful and do not
  require additional spec coordination.
- **`strings.Title` deprecation** in `ToTitleCase`. Per NAI-104 §4
  and NAI-105 §4 — out of scope here too.
- **`FromBase37` `chars[12-l:]` slice TODO comment** at line 59.
  Pre-existing TODO, behavior is correct; not in NAI-106 scope.
- **DAMAGE opcode 2015**, **SPLIT_* font-aware wrap**,
  **Survival Expert / Hans pathfinder reach residual**,
  **NAI-85-D-LC_NAME-FIELD-CHOICE**, **TS `value < 0n` non-
  divergence** — preserved as-is.

### 5. Deviations introduced

**None.** Full TS-faithful port; this spec brings goscape **closer**
to TS by retiring an unintended divergence.

### 6. Deviations retired

- **`NAI-105-D-FROMBASE37-ZERO-REJECT`** — retired by P1. Re-grep at
  impl time:
  - `rg "v != 0 && v%37" pkg/util/jstring/jstring.go` → 0 matches
    post-fix.
  - `rg "v%37 == 0" pkg/util/jstring/jstring.go` → 1 match (the
    now-unconditional reject).
  - Direct empirical pin: T1 asserts the user-visible
    `ToDisplayName("") → "Invalid Name"`.

### 7. Implementation plan (subagent-driven, single bundle)

Single subagent dispatch covers all changes; compressed cadence
skips formal review.

**Bundle 1: FromBase37 zero-reject narrowing (single dispatch)**

Tasks for the implementer (TDD per
`superpowers:test-driven-development`):

1. **T1 (TDD, fail-test for ToDisplayName)**: In
   `pkg/util/jstring/jstring_test.go`, change the
   `TestToDisplayName` fixture at line 42 from `{"", ""}` to
   `{"", "Invalid Name"}`. Run `GOPATH=$TMPDIR/go
   GOCACHE=$TMPDIR/go-cache go test ./pkg/util/jstring/...`.
   Expected RED: that one case fails with
   `ToDisplayName(""): got "", want "Invalid Name"`. All other
   `TestToDisplayName` cases and the other test functions remain
   green.

2. **T2 (RED→GREEN, narrow the guard)**: Edit
   `pkg/util/jstring/jstring.go:46` — change `if v != 0 && v%37 == 0`
   to `if v%37 == 0`. The doc comment at `:44-45` stays. Re-run
   `go test ./pkg/util/jstring/...`. T1 should now pass; all other
   `pkg/util/jstring` tests should remain green.

3. **T3 (verification)**: Run `GOPATH=$TMPDIR/go
   GOCACHE=$TMPDIR/go-cache go test ./...` and `GOPATH=$TMPDIR/go
   GOCACHE=$TMPDIR/go-cache go vet ./...`. Both must be clean — no
   downstream caller regresses. Per controller pre-flight, the six
   production sites that consume `FromBase37` (server.go:591,
   player.go:434, handler_reportabuse.go:60/67/72,
   handler_social_list.go:39) do not branch on `== ""` literal; the
   behavioral changes for the degenerate `v=0` case are TS-faithful
   and intended.

   Re-grep checks:
   - `rg "v != 0 && v%37" pkg/util/jstring/jstring.go` → 0 matches.
   - `rg "v%37 == 0" pkg/util/jstring/jstring.go` → 1 match.
   - `rg "FromBase37" --type go` → external surface unchanged
     (no new or removed call sites; same 6 production callers + 5
     test references).

4. **T4 (close commit)**: Single chore(close) commit. Body lists:
   - Retired follow-up: `NAI-105-D-FROMBASE37-ZERO-REJECT`.
   - No new follow-up surfaced.
   - `Closes memory:` trailer per `close_commit_memory_trailer.md`
     — entry is `NAI-105-D-FROMBASE37-ZERO-REJECT` from
     `nai_followups.md`.

### 8. Risk register

- **R1 — Other `FromBase37(0)` callers whose behavior depends on the
  `""` return?** [GREEN, pre-flighted at spec-write].
  Six production sites enumerated in §4. None branch on `== ""`;
  the three reportabuse `pk.G8()` sites and one social-list
  `pk.G8()` site can in principle receive a `uint64(0)` off the
  wire from a malformed/malicious client, in which case the post-fix
  behavior (gate triggers / payload contains `"invalid_name"`) is
  TS-faithful and improves robustness.

- **R2 — `TestFromBase37InvalidNameMod37` regression?** [GREEN].
  All four cases (`37, 74, 1369, 37*12345`) are nonzero; pre-fix
  they hit the guard via both `v != 0` AND `v%37 == 0`, post-fix
  they hit it via just `v%37 == 0`. Same return, same test outcome.

- **R3 — `TestFromBase37ValidNameDecodes` regression?** [GREEN].
  `ToBase37("alice") = 2494434`, `2494434 % 37 = 5` (not 0). Decode
  path unchanged.

- **R4 — `TestToBase37DividesOutTrailing37` regression?** [GREEN].
  No `FromBase37` calls in this test; `ToBase37` is untouched.

- **R5 — `TestToDisplayName` non-empty cases regression?** [GREEN].
  All other cases produce non-empty `ToBase37` results (verified
  post-NAI-105: divide-out loop guarantees nonzero non-mod-37
  outputs for non-empty inputs). The mod-37 reject is unreachable
  for them; only the `{""}` case changes.

- **R6 — Wire-format / on-the-wire regression?** [GREEN]. The
  change affects only the `v=0` codepath, which corresponds to
  malformed/empty inputs that should not reach the wire under
  normal play. Per §4, the four wire-facing handler sites
  (reportabuse 3×, social_list 1×) gain TS-faithful "invalid"
  treatment for degenerate offender/username values.

### 9. Verification before close

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.
- `rg "v != 0 && v%37" pkg/util/jstring/jstring.go` → 0 matches.
- `rg "v%37 == 0" pkg/util/jstring/jstring.go` → 1 match.
- `rg "FromBase37" --type go | wc -l` matches pre-fix count
  (no surface change).
- `git show HEAD --stat` matches stated bundle scope: 2 files
  touched (`pkg/util/jstring/jstring.go`,
  `pkg/util/jstring/jstring_test.go`); no stray worktree writes
  (per `feedback_subagent_wt_path.md`).

### 10. Notes

- Third consecutive compressed-cadence sub-spec on
  `pkg/util/jstring/` (NAI-104 op-order → NAI-105 divide-out →
  NAI-106 zero-reject narrowing). Each retires one tracked
  follow-up surfaced by its predecessor's pre-flight.
- No new follow-up surfaced at spec-write; the `pkg/util/jstring`
  port reaches TS parity with this change (modulo the carried-
  forward `strings.Title` deprecation and the `chars[12-l:]` TODO
  comment, both unchanged in behavior).
- The TS `value < 0n` guard at `JString.ts:38` is structurally
  inapplicable to goscape's `uint64` parameter — explicitly
  documented in §2 to forestall future "missing guard" tracker
  entries.
