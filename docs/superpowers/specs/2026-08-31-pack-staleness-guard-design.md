# Pack Staleness Guard — Design

**Status:** proposed, not implemented
**Date:** 2026-08-31
**Branch:** would originate on `rev-274`
**Deviation ID prefix:** `PSG-D{n}`

---

## 1. The failure this prevents

The rev-274 engine sync (2026-08-31) moved npc `wanderrange`/`maxrange` from
server opcodes 200/201 to 26/27 and moved `extracheck_var` from 18–20 to 19–21.
A `make pack` afterwards produced a cache that **still contained the old
opcodes for some families**, because the incremental freshness check decided
those outputs did not need rebuilding.

Observed, not hypothesised:

```
$ make pack          # after the sync
$ ls -l --time-style=+%m-%d data/pack/server/{npc,hunt}.dat
08-31  npc.dat        <- rebuilt
06-24  hunt.dat       <- NOT rebuilt, still June bytes
$ find data/pack -type f -not -newermt 2026-08-31 | wc -l
2909
```

`hunt.dat` still encoded `extracheck_var` at opcode 18. The server then either
refuses to boot (`unrecognized npc config code 201`, the loud case) or boots
and serves subtly wrong bytes (the quiet case, which is worse).

The root cause is one sentence: **freshness is keyed on source-file mtimes, so
it cannot see that the packer's encoding changed.** Nothing in `data/src`
changed for hunt configs, so the check was satisfied.

The reference build only came out correct because the sync ran
`npm run clean && npm run build`, forcing a full rebuild.

## 2. What upstream actually does

TS already has a mechanism for exactly this — goscape has **not ported any of
it**.

`tools/pack/FsCache.ts:165` @1d25566c:

```ts
export function didFileSetChange(stampPath: string, files: string[]): boolean {
    const state = files.map(file => `${file}=${fileExists(file) ? stableMtimeMs(fileStats(file).mtimeMs) : 0}`).join('\n');
    if (fileExists(stampPath) && readTextFile(stampPath) === state) {
        return false;
    }
    writeFileIfChanged(stampPath, state);
    return true;
}
```

It records `<packer source path>=<mtime>` into `data/pack/.stamps/*.txt` and
reports a change. The reference cache carries four such files:

```
.stamps/graphics-tools.txt    …/tools/pack/graphics/pack.ts=1781307492809
.stamps/map-tools.txt         …/tools/pack/map/Pack.js=1781307492809
.stamps/midi-tools.txt        …/tools/pack/midi/pack.ts=1781307492809
.stamps/revalidate-tools.txt  …/tools/pack/PackFile.ts=…  tools/pack/Parse.ts=…
```

**Call sites — all five, exhaustively:** `PackFile.ts:204`, `PackFile.ts:327`,
`graphics/pack.ts:15`, `map/Pack.js:244`, `midi/pack.ts:14`.

### 2.1 Upstream has the same hole

The config pipeline is **not** covered. In `PackShared.ts:358-378` @1d25566c,
twenty of twenty-one families are gated purely on content:

```ts
const rebuildHunt = shouldBuildConfigOutput('.hunt', 'data/pack/server/hunt.dat');
const rebuildNpc  = shouldBuildConfigOutput('.npc',  'data/pack/server/npc.dat');
// … 18 more, all the same shape
```

and `shouldBuildConfigOutput` (`PackShared.ts:354`) consults only
`<srcDir>/scripts` plus a content dependency list. Exactly one family checks
the packer sources:

```ts
const rebuildCategory = shouldBuildFile(`${srcDir}/pack/category.pack`, '…/category.dat')
    || shouldBuild('tools/pack/config', '.ts', '…/category.dat');
```

So `category.dat` is protected and `hunt`/`npc`/`seq`/`obj`/`spotanim`/`loc`
are not. **This is an upstream bug, not a goscape porting gap.** Any TS user
who edits `tools/pack/config/HuntConfig.ts` and runs an incremental build gets
the same stale artifact goscape produced.

That matters for scoping: porting `didFileSetChange` faithfully would fix four
stages goscape currently has no protection for at all, and would still leave
the twenty config families exposed on both sides.

## 3. Why goscape cannot copy the mechanism verbatim

Three constraints, each of which kills an obvious approach.

**C1 — no source at runtime.** TS runs from source under `tsx`, so
`import.meta.filename` plus an mtime is a valid identity for "the packer".
goscape ships a compiled binary; a distroless `goscape-cli` image contains no
`.go` files. Mtime-of-packer-source is not available and cannot be made
available without shipping source into the image.

**C2 — the binary is not a stable identity.** `make pack` builds with

```
-ldflags "… -X …/build.BuildDate=2026-08-31T19:59:49Z"
```

so **every build produces a different binary**. Hashing `os.Executable()` would
therefore report "packer changed" on every single invocation, permanently
disabling incrementality rather than guarding it. This is the trap to avoid:
the naive robust-looking option is the one that silently turns a 10 s
incremental pack into a 10 s full pack forever while looking like it works.

**C3 — the CLI is not the only entry point.** `modules/world/rebuild_worker.go`
runs `packall.PackAll` in-process for the `::rebuild` cheat
(`world.content_path`). A live server upgraded to a new binary and then issued
`::rebuild` hits the identical hazard. **A Makefile-only fix is therefore not
sufficient**, and any guard has to live in the library, not the build system.

## 4. Options

| | Approach | Protects `::rebuild`? | Discipline required | False invalidation |
|---|---|---|---|---|
| **A** | Compiled-in format version per stage group | yes | bump on encoder change | none |
| **B** | Clean-by-default `make pack` | **no** (C3) | none | full pack every time |
| **C** | Source fingerprint injected via ldflags | yes | none | none, but see below |
| **D** | Port `didFileSetChange` verbatim | partial | none | none |

**A — format-version constants.** A `pack.FormatVersion` (or one per stage
group) compiled into the binary and written into the stamp file. A mismatch
forces a rebuild. Simple, no build-system coupling, zero false invalidation.
Its weakness is that it relies on someone bumping it — which is precisely the
discipline that failed during the sync. Mitigated, but not eliminated, by §6.

**B — clean by default.** Make `make pack` wipe `data/pack` first and add an
opt-in `make pack-incremental`. Zero mechanism, zero discipline, costs ~10 s.
Rejected as the *primary* fix because of C3: it does nothing for `::rebuild`
or any other programmatic `PackAll` caller. Worth doing anyway as a cheap
belt-and-braces default.

**C — source fingerprint.** Have the Makefile compute a hash over `pkg/pack/**`
and inject it via `-X`. Deterministic, narrow, and immune to C2 because it
ignores `BuildDate`. Two costs: it couples the guard to the build system, so a
plain `go build`/`go run` gets an empty fingerprint and must then fall back to
"always rebuild" (safe but noisy in dev); and it needs care to hash
working-tree content rather than the git index, or uncommitted edits go
unnoticed.

**D — port `didFileSetChange`.** Faithful, and it closes four stages goscape
has no protection for today. But under C1 there is no packer source to stamp,
so the port would have to substitute a different identity anyway — at which
point it reduces to A or C with extra steps. It does not touch the config
families that actually broke.

## 5. Recommendation

**A as the core guard, plus B as a default, and D's stage coverage folded into
A.**

Concretely:

1. Introduce a stamp file per stage group under `<outDir>/.stamps/`, mirroring
   TS's location and naming so the two trees stay comparable.
2. The stamp content is the stage group's compiled-in format version, not an
   mtime. Reading a stamp whose version differs from the running binary's
   forces that group to rebuild.
3. Cover the four groups TS stamps (graphics, map, midi, the `PackFile`
   revalidate pair) **and** the config pipeline, which TS leaves exposed.
4. Make `make pack` clean by default; add `make pack-incremental` for the fast
   inner loop.

Extending coverage to the twenty config families is a deliberate divergence
from TS, which checks only `category`. It is an improvement rather than drift,
and gets **`PSG-D1`** with the rationale above: TS's own reference build is
only correct because its documented workflow is `npm run clean && npm run
build`, whereas goscape's documented workflow is a bare `make pack`.

Using a compiled-in version instead of a source mtime is **`PSG-D2`**, forced
by C1 — the mechanism is equivalent in effect, and the divergence is in the
identity function only.

## 6. Making the discipline enforceable

Option A's weakness is a human forgetting to bump the constant. The project
already has most of the machinery to catch that, and it fired during the sync:
`TestPackNpcConfigs_Wanderrange`, `TestPackHuntConfigs_OpcodeExtraCheckVar1Through3`
and roughly a dozen siblings all failed the moment the encoder changed. What is
missing is the *link* from "an encoder byte test changed" to "bump the format
version".

Proposal: a single test that hashes the byte output of one small fixture per
stage group and pins it alongside the group's format version. Changing an
encoder changes the hash; the test fails with a message naming the constant to
bump and the golden to regenerate. That converts the discipline from
"remember" into "CI tells you", which is the only kind that survives.

This test is the load-bearing half of Option A. Implementing A without it
reproduces the failure mode one sync later.

## 7. Non-goals

- Reworking the freshness system generally. The mtime-vs-content question for
  *source* files is out of scope; this is only about packer identity.
- Changing what the packer emits. No byte output changes.
- Backporting to `rev-254`/`245.2`/`244`/`225`. The hazard exists there too,
  but per `memory:no_forward_port_deviations` these are goscape-originated
  improvements, not fidelity restorations; ship on `rev-274` first and treat
  the fan-out as a separate user decision.

## 8. Verification

- A test that writes a stamp with version *N*, bumps the compiled version to
  *N+1*, and asserts the stage rebuilds — the direct regression pin for the
  bug in §1.
- The inverse: an unchanged version must **not** force a rebuild, so the guard
  cannot be satisfied by trivially always rebuilding (the
  `memory:test_passes_for_wrong_reason` trap; a guard that always fires passes
  a naive "does it rebuild?" test while destroying incrementality, which is
  exactly the C2 failure).
- A `make pack` run after an artificial encoder change must produce zero
  artifacts older than the run.
- Full pack parity against `Server274-ref` must remain byte-exact — 54/56
  shared artifacts identical, the two exceptions being `script.dat`/`script.idx`
  whose embedded content path differs by invocation.

## 9. Open questions for the user

1. **Granularity.** One global format version (simplest; any packer change
   re-packs everything, ~10 s) versus per-stage-group versions (surgical; more
   constants to maintain). The recommendation assumes per-group, but a single
   global version is defensible given how fast a full pack is.
2. **Whether B alone is enough.** If `::rebuild` is considered a dev-only
   affordance not worth guarding, then making `make pack` clean by default is a
   two-line fix and the rest of this spec is unnecessary. That is a legitimate
   call and would close the observed failure; it just leaves C3 open.
