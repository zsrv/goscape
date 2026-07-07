# Porting a new revision

This page turns the branch model into an ordered recipe. It operationalizes the
"Future revisions" section of the [References & pins](references-pins.md) lockfile
and the porting workflow in the [Porting lessons](porting-lessons.md) (§2), and
ends with the step that most recipes forget: adding the new revision to **this
documentation site**. Read the [Porting lessons](porting-lessons.md) in full
first — the per-step rules below assume its philosophy (§1) and its TS→Go gotcha
catalogue (§3).

The core identity is simple: **the Go port of revision *N* is the Go port of
revision *N−1* plus the translated Engine-TS delta.** That makes porting a branch
operation, not a rewrite.

## Engine steps

1. **Branch `rev-N` from the nearest prior revision branch.** Cut the new branch
   from the closest existing revision (for the current lineage that is the tip of
   the chain in the [Branch model](branches.md)). The port inherits everything
   already translated; you are only responsible for the delta.

2. **Look up the prior revision's pinned Engine-TS commit** in
   [References & pins](references-pins.md), then **diff the primary reference
   across the gap**:

   ```bash
   git -C Engine-TS diff <prev-commit>..<new-commit>
   ```

   That diff is your **work list** — every changed TS region is a region you must
   translate. (Where the upstream branches share history, this cross-pin diff is
   the real, minimal delta; the pins page notes the merge-base for each gap.)

3. **Translate each TS change into the corresponding Go region**, applying the
   gotcha rules from the [Porting lessons](porting-lessons.md) §3 (identity
   `uid`-vs-`slot` discipline, shared-handler/base-class fixes landing in both Go
   forks, tick-order fidelity, the protected-access invariant, pack determinism,
   and so on). The Go branch diff should correspond change-for-change to the TS
   diff — that correspondence is your audit:

   ```bash
   git diff rev-<N-1> rev-N   # should mirror the Engine-TS diff, region for region
   ```

4. **Bump the Content and compiler pins together with the engine.** The pack
   pipeline is byte-parity-checked against the cache the upstream meta-repo packs,
   so the engine, the Content tree, and the RuneScript compiler move as a set.
   The compiler is revision-dependent (RuneScriptTS for some revisions,
   RuneScriptKt for others — the pins page records which per revision), so
   re-verify the compiler and the resulting cache version bytes for the new pin.

5. **Record the new reference commits** in `REFERENCES.md` under a new `## rev-N`
   section — the Engine-TS, Content, Client-Java, and compiler pins, plus a note
   documenting the lineage (which branch it was cut from, the merge-base, and any
   toolchain swap). Because branch names move, **the commit hash is the pin**;
   record the full hash, not just the branch name.

6. **Keep the branch self-contained.** Do not share code packages across revision
   branches — each revision is an independent faithful translation and must be
   free to diverge with its upstream. Copy-and-adapt rather than extract-and-share.

7. **Carry the deviation tracker forward.** `PORTING.md` rows that still apply
   travel with the new branch; close rows using the closure shapes defined there
   (FIXED / EXCEPTION-DOCUMENTED / NO-DIVERGENCE / NOT-A-GAP), and move closed
   rows to `docs/PORTING-CLOSED.md`. Any accepted divergence keeps its in-code
   `PORTING-EXCEPTION (<row-id>, <short>)` marker.

8. **Pass the verification gate.** Build (`CGO_ENABLED=0 go build -trimpath
   ./...`), `go vet ./...`, `go test ./...`, and `go test -race` on touched
   packages. Confirm byte parity for the boundaries (ISAAC, RSA, opcodes, bzip2,
   CRC, wordenc, collision) and byte-diff the pack output against the
   upstream-packed cache when the pack pipeline changed. Pin the TS-correct
   contract of each fix with a test — see the [Porting lessons](porting-lessons.md)
   §5.

## Documentation step

A revision is not fully ported until it appears on this site. The docs build is
data-driven from one file, so the sequence is short (run it from the docs
worktree root, with the venv on `PATH` — see [Dev environment](dev-setup.md)):

1. **Add the revision to `tools/docsgen/revisions.toml`.** Append the revision to
   the `order` list (and update `latest` if it is now the newest), then add a
   `[revisions.<N>]` table giving its `branch`, `unpack_revision`, `content_dir`,
   and `cache_dir` — the same Content tree and packed cache the engine port was
   built against. (These are the inputs both `docsgen` and `build.py` read.)

2. **Regenerate the overlay pages with docsgen:**

   ```bash
   python -m tools.docsgen --revision <N>     # generates overlays/rev-<N>/…
   python -m tools.docsgen --revision all     # or refresh every rev + comparison table
   ```

   This unpacks the revision's cache/content and writes its item, NPC, location,
   varp, commands, music, and places pages under `overlays/rev-<N>/`, plus the
   navigation fragment `build.py` needs.

3. **Add the mike version and rebuild the site.** `tools/build.py all` assembles
   every revision (injecting the updated `REFERENCES.md` and `PORTING-LESSONS.md`
   as the pins and lessons pages automatically), deploys each as a mike version,
   and sets the newest as the `latest` default:

   ```bash
   python tools/build.py assemble --revision <N>          # stage just the new rev
   zensical build --strict -f .build/rev-<N>/mkdocs.yml   # strict-build it in isolation
   python tools/build.py all                              # deploy every rev as a mike version
   ```

   Because `REFERENCES.md` is injected verbatim into the site at assemble time,
   updating it in step 5 of the engine steps above is also what publishes the new
   revision's pins to the [References & pins](references-pins.md) page — there is
   no separate page to edit.
