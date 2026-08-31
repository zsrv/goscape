# Contributing to goscape

Thanks for looking. This project has one unusual property that shapes almost
every contribution, so it is worth stating up front: **goscape is a port, not a
new game server.** Each `rev-N` branch reproduces the behaviour of Lost City's
[Engine-TS](https://github.com/LostCityRS/Engine-TS) at one wire-protocol
revision. "Better than the original" is usually the wrong direction here.

## Which branch

`main` carries no engine code — only cross-revision documentation and the
docs-site tooling. The server lives on the revision branches:

| Branch | Target |
|---|---|
| `main` | this branch: cross-revision docs and the docs-site tooling |
| `rev-274` | newest revision; default target for engine fixes |
| `rev-254`, `rev-245.2`, `rev-244`, `rev-225` | earlier revisions, all maintained |

Branch from the revision you are fixing. If a bug affects several revisions, fix
it on the newest affected branch first; backports follow, and it is fine to leave
those to a maintainer.

Two rules about moving code between branches:

- **Fixes that restore fidelity backport freely.** If Go diverged from the TS
  reference, every affected revision wants that corrected.
- **Improvements do not forward-port.** A capability a later revision's engine
  gained does not belong on an earlier branch, even when the code would apply
  cleanly. Each branch is a faithful port of *its* revision.

## Fidelity, and how to record a deliberate difference

`PORTING-LESSONS.md` on `main` describes the translation philosophy; read
section 1 before changing game logic. In short: match the reference
implementation's behaviour, including behaviour you think is a bug, and cite the
TS source (`File.ts:line`) in a comment when the reason is not obvious.

When a difference is intentional — a Go-only extension, a divergence with a good
reason, a gap you are deferring — it belongs in `docs/PORTING.md` on that branch,
in the table matching its kind, with the severity legend that file defines. An
undocumented divergence is the thing most likely to get a change sent back.

## Working on this branch

`main` holds the cross-revision documentation (`REFERENCES.md`,
`PORTING-LESSONS.md`, `docs/`) and the tooling that assembles the versioned
site. It has no Go code, so the module resolves per revision —
`go install github.com/zsrv/goscape/cmd/goscape@rev-274`, not `@latest`.

```bash
python -m venv .venv && .venv/bin/pip install -r requirements.txt
.venv/bin/python -m pytest tools/docsgen/tests -q       # tooling unit tests
.venv/bin/python tools/build.py assemble --revision 274 # stage one revision's site
```

`assemble` reads nothing outside this repository. Regenerating the per-revision
overlays (`docsgen`) additionally needs the pinned content trees and client
worktrees; `tools/docsgen/paths.py` explains where it looks for them and how to
point it elsewhere with `GOSCAPE_SRC_ROOT`. Publishing to `gh-pages` with `mike`
stays a maintainer step for that reason.

Documentation describing one revision's behaviour belongs in that revision's
overlay or on the `rev-N` branch itself; only genuinely cross-revision material
belongs here.

## Before you open a pull request

Run the tooling tests, and assemble every revision if you touched the tooling or
the shared pages:

```bash
.venv/bin/python -m pytest tools/docsgen/tests -q
for rev in 225 244 245.2 254 274; do
  .venv/bin/python tools/build.py assemble --revision "$rev"
done
```

CI on `main` runs exactly those two things.

## Commits and pull requests

Commit messages here follow `type(scope): summary` — `feat`, `fix`, `docs`,
`chore`, `refactor`, `test` — and the body explains *why*, including what you
verified. Look at recent history for the tone; a commit that explains the
reasoning behind a subtle port decision is worth more than a tidy diff.

Keep a pull request to one concern. If you find an unrelated problem along the
way, say so in the description rather than fixing it in the same change.

## Security

Do not open a public issue for a vulnerability. [`SECURITY.md`](SECURITY.md) has
the reporting process and the trust model — particularly which listeners are
meant to be internal, which is the most common source of "is this a bug?"
questions.

## Licensing

Contributions are accepted under this repository's [MIT license](LICENSE). By
opening a pull request you agree your work may be distributed under it. If your
change derives from another project's code, say so in the pull request so
[`NOTICE`](NOTICE) can be updated.
