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
| `rev-254` | this branch's revision |
| `rev-274` | newest revision |
| `rev-245.2`, `rev-244`, `rev-225` | earlier revisions, all maintained |

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

## Development

Requirements: Go 1.27 or newer. No CGO for a normal build; the race detector
needs it.

```bash
make all                       # build goscape and goscape-cli into the repo root
go test ./...                  # the suite; green from a clean checkout
go test -race ./modules/...    # for anything touching concurrency

# Running it needs a config file — there is no default path
CGO_ENABLED=0 go run ./cmd/goscape --config.file examples/bundled/goscape.yaml
```

Tests that exercise the world server's reload pipeline need a packed cache
(`data/pack/`, produced by `goscape-cli pack`) and skip themselves when it is
absent, so a clean checkout is green without one.

## Before you open a pull request

Run whatever your change touches:

| You changed | Run |
|---|---|
| any Go code | `gofmt -l .` (must be empty), `go vet ./...`, `go test ./...` |
| concurrency | `CGO_ENABLED=1 go test -race ./<packages>` |
| `proto/` | `make check-generated-files` — commits the regenerated `.pb.go` |
| `production/helm/` | `make helm-lint` and `make helm-test` |
| `examples/*.yaml` | `go run ./cmd/goscape --config.file <file> --config.verify=true` |

CI runs the Go build, tests, `gofmt`, `go vet`, and race tests on every `rev-*`
branch, plus `buf` lint/format/breaking when protos change. Config decoding is
strict, so a stray key in an example config is a build failure rather than a
warning.

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
