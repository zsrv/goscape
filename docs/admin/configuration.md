# Configuration

This page explains how goscape reads its configuration: where values come from
and in what order, how the file is validated, and the few settings that shape the
whole process — which modules run, which database backend is used, and how much
is logged. It does **not** list individual keys. Every option, set to its default
and annotated, lives in the [Config reference](config-reference.md); reach for
that page whenever you need a specific key.

Configuration is supplied in three places: a YAML file (`--config.file`),
environment variables, and command-line flags. Each flag has a matching YAML key;
the flag name uses dots and dashes (`--world.tcp-listen-port`) while the YAML key
uses underscores under a section (`world: { tcp_listen_port: … }`).

## How configuration is resolved

Values are layered from lowest to highest precedence. A later layer overrides an
earlier one for the same setting:

1. **Built-in defaults** — the value baked into the code for every setting.
2. **The config file** — whatever you set in the file named by `--config.file`.
3. **Environment variables** — only when expansion is enabled (see
   [Environment-variable expansion](#environment-variable-expansion) below).
4. **Command-line flags** — for example `--world.tcp-listen-port 43594`.

There is no default config path: **`--config.file` is always required** to start
the server (a bare `go run ./cmd/goscape` with no config file cannot boot). The
`examples/` directory contains two starting points — `examples/bundled/goscape.yaml`,
a minimal "run everything" preset, and `examples/full-config-reference.yaml`,
which lists every key at its default. Copy only the keys you want to override into
your own file.

## Strict decoding

The loader decodes the config file **strictly**: every key in the file must be a
key the server understands. An unknown key — including a typo or a stale key from
an older version — is a **fatal boot error**, not a warning. The server refuses to
start and exits non-zero.

!!! warning "An unknown key aborts startup"
    Given a file with a misspelled key — here `htp_listen_port` instead of
    `http_listen_port`:

    ```yaml
    ondemand:
      enable: true
      htp_listen_port: 8080
    ```

    the server prints the offending line and the type it was decoding into, then
    exits non-zero:

    ```text
    failed to parse config: failed to parse configFile goscape.yaml: yaml: unmarshal errors:
      line 3: field htp_listen_port not found in type ondemand.Config
    ```

    Fix the key name (or remove the line) and try again. Running
    [`--config.verify=true`](#meta-flags-are-value-flags) catches this before you
    ever bind a port.

## Environment-variable expansion

By default the config file is parsed exactly as written. Pass
`--config.expand-env=true` to expand `${VAR}` and `$VAR` references from the
process environment **before** the file is parsed. This lets you keep secrets and
host-specific values out of the file itself. The syntax supports defaults, so a
value can fall back when the variable is unset:

```yaml
database:
  sqlite:
    dsn: ${GOSCAPE_DB:-data/goscape.db}
```

With expansion enabled, `${GOSCAPE_DB}` is substituted from the environment; if it
is unset, the value after `:-` (`data/goscape.db`) is used. Expansion is off unless
you pass the flag, so a literal `$` in a value is only special when you opt in.

## Meta-flags are value flags

Three flags control loading itself rather than any single setting. They are
**value flags**: each requires an explicit `=…`, and the bare form is rejected
before the server starts.

| Flag | Meaning |
|------|---------|
| `--config.file=<path>` | Config file to load (required). |
| `--config.verify=true` | Validate the config and exit; do not start the server. |
| `--config.expand-env=true` | Expand `${VAR}` / `$VAR` in the file from the environment before parsing. |

!!! warning "Write `=true`, not the bare flag"
    `--config.verify` and `--config.expand-env` are boolean-valued, but they are
    registered as value flags, so the `=true` is not optional. A bare
    `--config.verify` is rejected with:

    ```text
    flag needs an argument: -config.verify
    ```

    Always write `--config.verify=true` and `--config.expand-env=true`.

## Choosing which modules run: `target`

The `target` key (or the `--target` flag) selects which modules the process runs:
`ondemand`, `world`, `login`, `friends`, or `all` (the default). Selecting a
target pulls in the modules it depends on automatically. The
[Administrator's Guide overview](index.md#modules-and-the-target-flag) explains
each target and the dependency graph between modules.

`target` interacts with each module's own `enable` key: **a module runs only when
both its `enable` is set to `true` and it is included by the active `target`.** The
bundled preset sets `target: all` and `enable: true` on every module, so the whole
server runs in one process. To split the server across hosts — for example, to run
only the login service on its own machine — set `target: login` there and enable
just that module; see [Deployment scenarios](deployment.md).

## Choosing a database backend

The `login` and `friends` modules share one **central database**. The `database:`
section selects its backend with `database.backend`, which is one of:

=== "`sqlite` (default)"

    ```yaml
    database:
      backend: sqlite
      sqlite:
        dsn: data/goscape.db   # SQLite file path
    ```

    A single local database file. Because both modules must reach the same file,
    the SQLite backend requires them to share a filesystem — that is, to run on the
    same host. This is what the bundled preset uses, and it needs no external
    services. `sqlite.dsn` must be non-empty.

=== "`postgres`"

    ```yaml
    database:
      backend: postgres
      postgres:
        dsn: postgres://user:pass@host:5432/goscape?sslmode=disable
        max_open_conns: 8
    ```

    A network database. Choose `postgres` when you need to run `login` and
    `friends` on **different hosts** against one shared database — the SQLite
    backend cannot span hosts. `postgres.dsn` is required (the server refuses to
    start without it), and `max_open_conns` (the per-service connection-pool size)
    must be at least `1`.

An invalid backend name is rejected at startup — `database.backend` must be one of
`[sqlite, postgres]`. See the overview's [network surfaces](index.md#network-surfaces-and-ports)
for how the shared database sits behind the `login` and `friends` modules, and
[Deployment scenarios](deployment.md) for the multi-host layout that PostgreSQL
enables.

## Logging and per-module log levels

Logging is configured at the top level with `log_level` (minimum severity to log;
one of `trace`, `debug`, `info`, `warn`, `error`, default `info`), `log_format`
(`text` or `json`), and `log_source` (how the source-file attribute is rendered).

Each module can **override** the log level for its own output. Set a `log_level`
inside the module's section; when omitted, the module inherits the top-level
`log_level`. These per-module overrides are **YAML-only — there is no CLI flag**
for them:

```yaml
log_level: info        # global default for every module

world:
  log_level: debug     # only the world module logs at debug
```

!!! note "`ondemand` does not support `trace`"
    The `ondemand` module runs on the shared HTTP server layer, which recognizes
    `debug`, `info`, `warn`, and `error` but not `trace`. The other modules
    (`world`, `login`, `friends`) accept `trace` as well.

## Every other key

This page covers the settings that shape the process as a whole. For every
remaining option — listen addresses and ports, save paths, timeouts, node
identity, and more — each key is documented at its default value in the
[Config reference](config-reference.md).
