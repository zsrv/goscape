## NAI-107: handleLcName Name → DebugName → "null" chain

**Date**: 2026-05-05
**Cadence**: combined spec + plan, no formal review (per
`compressed_cadence.md`, ≤15 production-LOC threshold; ~3 production
LOC + ~10 test LOC).
**Predecessor**: NAI-106 (HEAD `608b961` — `FromBase37` zero-reject
narrowing).
**Trigger**: NAI-85 surfaced/deferred follow-up entry
`NAI-85-D-LC_NAME-FIELD-CHOICE` in `nai_followups.md`. Goscape's
`handleLcName` (LC_NAME, opcode 3010) carries a 2-branch
`DebugName ?? "null"` shape with a stale "no separate Name field
server-side" comment; TS `LocConfigOps.ts:12` is a 3-branch
`name ?? debugname ?? 'null'` chain. The `Name` field IS decoded
server-side (`pkg/objtype/loctype.go:72`, code 2 → `GJStrLF()`).
**Tech stack**: Go 1.26+ (per `go_version.md`).
**Successor**: TBD; no new follow-up surfaced at spec-write time.

### 1. Problem

`pkg/script/handlers_config.go:143-161` reads:

```go
// handleLcName (LC_NAME) pops a loc id and pushes its name (or debugname
// fallback; "null" when both are empty).
func handleLcName(s *ScriptState) error {
    if err := requireConfigs(s, "LC_NAME"); err != nil {
        return err
    }
    id := s.PopInt()
    lt := s.Configs.LocType(id)
    if lt == nil {
        return fmt.Errorf("LC_NAME: unknown loc id %d", id)
    }
    // LocType has no separate Name field server-side; fall back to debugname.
    if lt.DebugName != "" {
        s.PushString(lt.DebugName)
    } else {
        s.PushString("null")
    }
    return nil
}
```

TS `Engine-TS/src/engine/script/handlers/LocConfigOps.ts:9-13` reads:

```typescript
[ScriptOpcode.LC_NAME]: state => {
    const locType: LocType = check(state.popInt(), LocTypeValid);

    state.pushString(locType.name ?? locType.debugname ?? 'null');
},
```

The narrow user-visible impact: any RuneScript reading `lc_name(loc)`
for a loc whose `Name` and `DebugName` differ (i.e. real content data,
where `Name` is the human-facing canonical name and `DebugName` is the
slug) currently renders the slug. Post-fix renders the canonical name.

NAI-85's new `handleLocName` (active-loc variant, LOC_NAME 3110) is
already TS-correct from the start (per its TS reference `LocOps.ts:130`,
`Name → 'null'` 2-branch with no debugname fallback). Only the config
reader (LC_NAME, this fix) is currently divergent.

### 2. TS reference

`Engine-TS/src/engine/script/handlers/LocConfigOps.ts:9-13` —
canonical 3-branch chain via JS nullish coalescing.

### 3. Goscape change

`pkg/script/handlers_config.go:143-161` —

- Replace the 6-line if/else block at lines 154-159 with a 9-line
  three-branch chain mirroring TS:

```go
// handleLcName (LC_NAME) pops a loc id and pushes its name, falling
// back to debugname, then "null".
func handleLcName(s *ScriptState) error {
    if err := requireConfigs(s, "LC_NAME"); err != nil {
        return err
    }
    id := s.PopInt()
    lt := s.Configs.LocType(id)
    if lt == nil {
        return fmt.Errorf("LC_NAME: unknown loc id %d", id)
    }
    if lt.Name != "" {
        s.PushString(lt.Name)
    } else if lt.DebugName != "" {
        s.PushString(lt.DebugName)
    } else {
        s.PushString("null")
    }
    return nil
}
```

- Drop the stale comment at line 154
  (`// LocType has no separate Name field server-side; fall back to debugname.`).
- Update the function doc comment at lines 143-144 to reflect the
  3-branch shape (sample text shown above).

### 4. Preserved as-is

- **`TestLcName` (loc 0, DebugName-only fixture)** continues to assert
  `"door"`. Pre-fix it hits the only branch; post-fix it hits the middle
  branch. Same return, same assertion. Doc-comment in the test
  (`// LocType has no Name field server-side; falls back to DebugName.`,
  line 348) is now stale and must be retitled in T2 below.
- **`TestLcNameNullFallback` (loc 1, both empty)** continues to assert
  `"null"`. Pre-fix it hits the else branch; post-fix it hits the third
  branch. Same return, same assertion. No comment change needed.
- **Test fixture `mc.locs[0]` (`door`)** at
  `handlers_config_test.go:117-125` — unchanged. We add a NEW loc id 2
  inside the new test (`TestLcNamePrefersNameOverDebugName`) rather
  than mutating the shared canonical fixture.
- **Dispatch wiring** (`OpLcName` opcode 3010 → `handleLcName` in
  `handlers.go`) — unchanged.
- **Other LC_* readers** (`LC_PARAM`, `LC_CATEGORY`, `LC_DESC`,
  `LC_DEBUGNAME`, `LC_WIDTH`, etc.) — unchanged. Each is already
  TS-faithful per its own per-opcode shape.
- **NAI-85 active-loc `handleLocName` (LOC_NAME 3110)** — unchanged.
  Already TS-correct per `LocOps.ts:130`'s 2-branch `Name → 'null'`
  shape (no DebugName fallback in the active-loc variant).
- **DAMAGE opcode 2015**, **SPLIT_* font-aware wrap**, **Survival
  Expert / Hans pathfinder reach residual**, **`strings.Title`
  deprecation in `ToTitleCase`**, **`FromBase37` `chars[12-l:]` slice
  TODO**, **TS `value < 0n` non-divergence** — all preserved as
  carry-forwards from NAI-104/105/106.

### 5. Deviations introduced

**None.** Full TS-faithful port; this spec brings goscape **closer**
to TS by retiring an unintended divergence.

### 6. Deviations retired

- **`NAI-85-D-LC_NAME-FIELD-CHOICE`** — retired by P1. Re-grep at
  impl time:
  - `rg "if lt.DebugName != \"\"" pkg/script/handlers_config.go` → 0
    matches in `handleLcName` post-fix (the LC_DEBUGNAME handler at
    `handlers_config.go:~225` is a separate function and keeps its own
    branch — it must still match its own TS shape, which IS the
    2-branch `debugname ?? 'null'`).
  - `rg "if lt.Name != \"\"" pkg/script/handlers_config.go` → ≥1
    match (the new top branch of the chain).
  - `rg "no separate Name field server-side" pkg/script/` → 0 matches.
  - Direct empirical pin: T1 asserts
    `LC_NAME(loc-with-Name-and-DebugName) → Name`.

### 7. Implementation plan (subagent-driven, single bundle)

Single subagent dispatch covers all changes; compressed cadence
skips formal review.

**Bundle 1: handleLcName 3-branch chain (single dispatch)**

Tasks for the implementer (TDD per
`superpowers:test-driven-development`):

1. **T1 (TDD, fail-test for Name preference)**: In
   `pkg/script/handlers_config_test.go`, add a new test function
   immediately after `TestLcNameNullFallback`:

   ```go
   func TestLcNamePrefersNameOverDebugName(t *testing.T) {
       mc := newTestConfigs()
       // Seed a loc with both Name and DebugName at id 2; Name wins.
       named := objtype.NewLocType(2)
       named.Name = "Door"
       named.DebugName = "door"
       mc.locs[2] = named
       state := runConfigOp(t, mc, OpLcName, []int{2})
       if got := state.PopString(); got != "Door" {
           t.Errorf("LC_NAME(2 named): got %q, want %q", got, "Door")
       }
   }
   ```

   Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test
   ./pkg/script/...`. Expected RED: the new test fails with
   `LC_NAME(2 named): got "door", want "Door"` because the current
   handler hits the `DebugName != ""` branch first. All other tests
   remain green (existing `TestLcName` and `TestLcNameNullFallback`
   are unaffected — they don't seed `Name`).

2. **T2 (RED→GREEN, install the chain)**: Edit
   `pkg/script/handlers_config.go:143-161`:
   - Replace the 6-line if/else (lines 154-159) with the 9-line
     three-branch chain shown in §3.
   - Drop the stale internal comment at line 154
     (`// LocType has no separate Name field server-side; fall back to debugname.`).
   - Update the function doc comment at lines 143-144 to read
     `// handleLcName (LC_NAME) pops a loc id and pushes its name,
     falling back to debugname, then "null".`

   Also edit `pkg/script/handlers_config_test.go:348` —
   replace the now-stale comment
   `// LocType has no Name field server-side; falls back to DebugName.`
   with `// Loc 0 has DebugName only (no Name); falls back to DebugName.`

   Re-run `go test ./pkg/script/...`. T1 should now pass; the existing
   `TestLcName` and `TestLcNameNullFallback` should remain green.

3. **T3 (verification)**: Run `GOPATH=$TMPDIR/go
   GOCACHE=$TMPDIR/go-cache go test ./...` and `GOPATH=$TMPDIR/go
   GOCACHE=$TMPDIR/go-cache go vet ./...`. Both must be clean — no
   downstream regression.

   Re-grep checks (per `verify_implementer_claims.md`, run from a
   fresh shell against HEAD):
   - `rg "no separate Name field server-side" pkg/script/` → 0 matches.
   - `rg "if lt.Name != \"\"" pkg/script/handlers_config.go` → ≥1
     match (in `handleLcName`).
   - `rg "OpLcName" pkg/script/` → unchanged surface (handler dispatch
     entry + tests).

4. **T4 (close commit)**: Single chore(close) commit. Body lists:
   - Retired follow-up: `NAI-85-D-LC_NAME-FIELD-CHOICE`.
   - No new follow-up surfaced.
   - `Closes memory:` trailer per `close_commit_memory_trailer.md` —
     entry is `NAI-85-D-LC_NAME-FIELD-CHOICE` from `nai_followups.md`.

### 8. Risk register

- **R1 — Does any other LC_* or LOC_* handler share this branch
  shape?** [GREEN, pre-flighted at spec-write].
  `handleLcDebugName` is a 2-branch `DebugName ?? "null"` per its TS
  reference `LocConfigOps.ts:35-37`; it must NOT change. `handleLcDesc`
  is a 2-branch `Desc ?? "null"` per `LocConfigOps.ts:31-33`; it must
  NOT change. NAI-85's `handleLocName` (active-loc) is a 2-branch
  `Name ?? "null"` per `LocOps.ts:130`; already correct. Only
  `handleLcName` (this fix) is divergent.

- **R2 — `TestLcName` regression?** [GREEN]. Loc 0 fixture has
  `Name=""` (default zero value of `string`) and `DebugName="door"`.
  Post-fix the chain takes the middle branch and returns `"door"`.
  Same assertion outcome.

- **R3 — `TestLcNameNullFallback` regression?** [GREEN]. Loc 1
  fixture is `objtype.NewLocType(1)` with no field assignments;
  `Name=""` and `DebugName=""`. Post-fix the chain takes the third
  branch and returns `"null"`. Same assertion outcome.

- **R4 — Other tests reading loc 0?** [GREEN].
  `TestLcParam`, `TestLcCategory`, `TestLcDesc`, `TestLcDebugName`,
  `TestLcWidth`, `TestLcLength` (and any other LC_* tests) read fields
  other than `Name` from loc 0 — fixture is untouched, no impact.

- **R5 — Production callers of `LC_NAME` in compiled scripts?**
  [GREEN, behavioral improvement]. Any RuneScript that calls
  `lc_name(loc)` for a loc with both `Name` and `DebugName` set will
  now render the canonical Name (e.g. `"Wooden door"`) instead of the
  slug (`"wooden_door"`). This is the intended TS-faithful behavior;
  not a regression.

- **R6 — Loc fixtures elsewhere in the repo (smoke tests, integration
  fixtures)?** [GREEN]. Test scope is `pkg/script/` only. Real LocType
  data loaded from cache via `LocType.Decode` already populates `Name`
  for any production loc that has it (per code 2 path); the runtime
  behavior shifts toward TS-correct rendering for content data.

### 9. Verification before close

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` clean.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean.
- `rg "no separate Name field server-side" pkg/script/` → 0 matches.
- `rg "if lt.Name != \"\"" pkg/script/handlers_config.go` → 1 match.
- `git show HEAD --stat` matches stated bundle scope: 2 files touched
  (`pkg/script/handlers_config.go`,
  `pkg/script/handlers_config_test.go`); no stray worktree writes
  (per `feedback_subagent_wt_path.md`).

### 10. Notes

- Pattern continuation of NAI-104/105/106 single-file polish chain:
  one TS-asymmetry retirement per sub-spec, compressed cadence,
  combined spec+plan doc, single subagent dispatch, no formal review.
- LC_NAME and LOC_NAME (active-loc) intentionally have different
  TS shapes (3-branch vs 2-branch); preserving that asymmetry is part
  of TS fidelity. Per `ts_asymmetry_dual_pin.md`, both pins matter:
  presence of the chain in LC_NAME, absence of DebugName fallback
  in LOC_NAME. NAI-85's LOC_NAME tests already cover the latter.
- No smoke is gating; T1's literal pin is binding evidence per
  the compressed-cadence pattern.
