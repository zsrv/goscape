# RESUME: rev-244 port — Bundle 2 (wire protocol + rsbuf)

Self-contained resume prompt. Written 2026-06-04 after Bundle 1 shipped.

## Where you are

This repo is a **multi-revision** Go port of the LostCityRS Engine-TS server:
`main` = codeless cross-revision docs hub (`README.md`, `REFERENCES.md`,
`PORTING-LESSONS.md`); `rev-225` = the complete 225 port; **`rev-244` = the
active 225→244 porting branch** (cut from rev-225). Work on `rev-244` only;
docs-hub updates (e.g. REFERENCES pins) commit on `main`.

The 225→244 work list is the upstream cross-pin diff
`git -C /home/owner/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4`
(the local Engine-TS checkout sits AT the 244 pin `9aadcec4`; clean shared
lineage, so the diff is a real change-for-change work list). Pins:
`git show main:REFERENCES.md` §rev-244.

## Read these first (in order)

1. `git show main:PORTING-LESSONS.md` — porting philosophy, §3 TS→Go gotchas,
   §4 citation conventions, §5 gates.
2. `docs/superpowers/specs/2026-06-03-rev244-port-design.md` (on rev-244) —
   the umbrella design: 7 dependency-ordered bundles B1–B7, definition of
   done, risks.
3. `PORTING.md` §"rev-244 Bundle audit trail" — B1's correspondence table,
   decision rows (deferrals, exceptions, pull-forwards).
4. `docs/superpowers/plans/2026-06-03-rev244-b1-io-cache-primitives.md` —
   the executed B1 plan incl. the Execution addendum (WordEnc gap-close,
   clientinterface writer pull-forward).

## State: B1 SHIPPED (12 commits, `8fcb734e..d82274fb`)

New: `pkg/io/filestream` (dat/idx store), `pkg/io/gziputil` (OS-byte-zeroed
gzip), `pkg/util/pemtoken`. Changed: SeqType/AnimFrame restructure
(`pkg/objtype/animframe.go` replaces seqframe.go; loads FileStream archive 2),
Component (trans byte, g2 childCount, `Operable→Interactable`,
`Iop→InventoryOptions`), NpcType codes 99-102, ObjType (op/iop nil + lazy-init,
244 F2P-members gating), WordEnc (`data/raw/wordenc`, unconditional).
Full suite `go test ./... -count=1` exit 0 on 2026-06-04.

**Format window (until B6 repack):** decoders read 244, `pkg/pack` still
writes 225. Booting from a 225 cache FAILS LOUD (Component positional decode
EOF-panics in world start — by design, documented). Skipping tests:
`TestLoadSeqTypes_FromPack`, `TestNewServer_LoadsWordencFilter`. B6
acceptance criterion: positive end-to-end boot vs a 244 cache.
**B6 must NOT double-apply** the `pkg/pack/clientinterface` writer hunks
(PackShared.ts:267-274,428-431) — already pulled forward in `e4e881d8`.

## Next: Bundle 2 — wire protocol + rsbuf

Surface (from the umbrella spec): `ClientGameProt.ts`/`ServerGameProt.ts`
opcode renumber (~80/78 + 58/61 lines), the interaction-handler family
(OpHeld/OpHeldU/InvButton/OpNpc*/OpObj*/OpLoc*/OpPlayerU — each 20-50 lines
changed), and the `pkg/rsbuf` re-audit against `@2004scape/rsbuf`
`^225.1.7 → ^244.1.0` (pkg/rsbuf is ~6.4k LOC reimplementing the Rust crate;
see `pkg/rsbuf/doc.go` for its closure history).

**PREREQUISITE before freezing any handler shape:** the B5 worker/multiworld
evaluation (umbrella spec §Bundle decomposition note) — investigate whether
244's worker architecture changed the login/friends WIRE (read
`git -C …Engine-TS show 9aadcec4:src/server/login/Messages.ts` + the Worker*
files + `src/appWorker.ts`). If the wire changed, that surface belongs to B2's
scope decisions. Produce the written evaluation deliverable first (it is a B5
deliverable executed early), THEN brainstorm/spec/plan B2 proper.

Process that worked for B1 (repeat it): brainstorm → spec (commit) → plan
(writing-plans, bite-sized TDD tasks, exact TS extraction commands as the
contract) → subagent-driven execution: implementer (sonnet) → TS-parity spec
reviewer → quality reviewer per substantive task; controller-direct
verification for tiny leaf tasks; full-suite gate + PORTING.md
correspondence-audit task at bundle end; final whole-bundle integration
review. The B1 spec reviewer caught a real panic bug and the gate caught a
plan gap — do not skip either.

## Mechanics & gotchas

- Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix.
  Build: `CGO_ENABLED=0 go build -trimpath ./...`. Race: `-race` on touched
  packages (CGO_ENABLED=1). Every commit: `--no-gpg-sign`.
- modules/world full suite takes ~2.5 min — it is not hung.
- Inside the sandbox, `git status` shows phantom `??` dotfiles
  (`.bashrc`, `.gitconfig`, …) — /dev/null device-node masks, NOT real files.
  Never stage them; never `git add -A` (it errors on them). Warn every
  subagent about this.
- Post-TDD stale LSP diagnostics ("undefined: New" etc. from the red phase)
  are normal; verify with a real build/test run instead.
- TS citations: `// TS <File>.ts:<lines>` against the 244 pin; the branch is
  the revision context. Adopt 244 names on renames; chase compile cascades.
- Deviations get PORTING.md rows (closure shapes per its Tracking
  conventions); accepted in-code divergences get `PORTING-EXCEPTION (<id>, …)`
  markers.

## Definition of done for the whole port (umbrella spec)

(a) `git diff rev-225 rev-244` corresponds change-for-change to the TS
cross-pin diff; (b) a 244 client (Client-Java `01f16088` / goscape-client
`rev-244`) logs in and plays; (c) pack byte-parity vs a 244 reference cache
(Engine-TS 244 + Content 244 + RuneScriptKt-26 jar); (d) suite green incl.
`-race`.
