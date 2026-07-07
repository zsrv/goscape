# Operations runbook

This page covers the recurring operational tasks for a running goscape server:
backing up its state, managing the login RSA key, watching its health, restarting
and upgrading it, and packing the game cache. It assumes you already have a server
running — see the [Quick start](quickstart.md) to get there, and the
[Administrator's overview](index.md) for the modules, ports, and data layout the
tasks below refer to.

## Backups

A goscape server keeps its durable state in **two independent places**. A complete
backup must capture both; either one alone is not a full restore.

| State | What it holds | Where it lives | Written by |
|-------|---------------|----------------|------------|
| Central database | Accounts, bcrypt password hashes, staff levels, membership, ban/mute timers, sessions, hiscores, friend and ignore lists, private/public chat logs, login history | SQLite file `data/goscape.db` (default), or a PostgreSQL database | `login` and `friends` |
| Player save files | One binary `.sav` per account per profile — position, appearance, stats and XP, inventory, playtime, and the rest of a character's progress | `data/players/<profile>/<username>.sav` (default) | `login` only |

The database path is set by `database.sqlite.dsn` (default `data/goscape.db`) or, for
the PostgreSQL backend, `database.postgres.dsn`. The save directory is set by
`login.save_path` (default `data/players`). See
[Configuration](configuration.md#choosing-a-database-backend) and the
[Config reference](config-reference.md) for these keys.

!!! warning "Player saves and the database are separate — back up both"
    The `login` module writes player `.sav` files to the local filesystem, while the
    account and social data live in the central database. In a single-host
    deployment both sit under the same `data/` directory. In a
    [multi-host deployment](deployment.md#3-multi-host-central-management-plus-world-hosts)
    with the PostgreSQL backend they are on different machines: the database is on
    the PostgreSQL host, and the saves are on whichever host runs the `login`
    module. Back up both locations.

### Backing up the SQLite database

The SQLite backend runs in **WAL mode** (write-ahead logging), so the on-disk state
is spread across `data/goscape.db` plus its `-wal` and `-shm` sidecar files. A naive
copy of only the `.db` file while the server is running can miss committed data still
in the WAL.

1. **Cold copy (simplest, guaranteed consistent).** Stop the server (see
   [Restarts & upgrades](#restarts-upgrades)), then copy the `.db` file and the whole
   `data/players/` tree. With the process stopped there is nothing to race.
2. **Online copy.** If you cannot stop the server, use SQLite's own backup mechanism
   (for example `sqlite3 data/goscape.db ".backup 'backup.db'"` or `VACUUM INTO`),
   which produces a consistent snapshot across the WAL. Copy `data/players/`
   alongside it.
3. **Filesystem or volume snapshot.** An atomic snapshot of the volume holding both
   `data/goscape.db` (with its WAL files) and `data/players/` captures a consistent
   point-in-time image of everything at once.

### Backing up the PostgreSQL database

With the PostgreSQL backend, back up the database with the tools you already use for
PostgreSQL — a logical dump (`pg_dump`) or your cluster's physical/base-backup
mechanism. Remember that the `.sav` player files are **not** in PostgreSQL: back up
the `login.save_path` directory on the login host separately.

### Restoring

To restore, put the server in a stopped state, replace the database (restore the
SQLite file, or `pg_restore`/`psql` the dump) and the `data/players/` tree from the
same backup point, then start the server. The `database` module brings the schema up
to date at startup, so restoring a database from an older server revision migrates
forward automatically on the next boot.

## Login RSA keys

The world server decrypts the RSA-protected login block using an RSA private key.
Out of the box it uses a **built-in default key compiled into the binary**, and the
matching public key is baked into the stock Java client, so login works with no
setup.

!!! danger "The built-in key is a known, public development key"
    The default key is a **512-bit** key whose private half is published in the
    goscape source (`pkg/io/protocol/rsakey.go`). Anyone can read it and decrypt the
    login handshake, so it offers **no protection** for real credentials. Treat it as
    a development convenience only. For any internet-facing or production deployment,
    generate your own key, wire it into the server, and rebuild the client with the
    matching public key.

### Generating a key

Use the `goscape-cli` tool to generate a keypair:

```bash
goscape-cli rsa gen --bits 1024 --out-dir ./keys
```

This writes two files into `--out-dir` (default the current directory):

- `private.pem` — the RSA private key (PKCS#1), for the server.
- `public.pem` — the RSA public key (PKIX).

It also prints the modulus (**N**), public exponent (**E**), and private exponent
(**D**) in the forms you need to bake the matching public key into the Java client
(its `LOGIN_RSAN` / `LOGIN_RSAE` constants). `--bits` defaults to `1024`.

!!! warning "Keep the modulus within the login wire limit"
    The RSA login block is length-prefixed with a **single byte**, so a modulus
    larger than about 254 bytes (roughly a 2032-bit key) overflows the wire format
    and breaks login. `goscape-cli rsa gen` prints a warning when you exceed this
    limit. A 1024-bit key is the tested default and stays well within it.

### Inspecting a key

`goscape-cli rsa info` prints the N / E / D bake values for a key. Pass a PEM path to
inspect that key, or pass no path to print the **built-in default key**:

```bash
goscape-cli rsa info ./keys/private.pem   # a specific key
goscape-cli rsa info                       # the built-in default key
```

### Wiring the key into the server

Point the world module at the private key with `world.rsa_private_key_path`:

```yaml
world:
  rsa_private_key_path: /etc/goscape/login-rsa/private.pem
```

The file may be PKCS#1 or PKCS#8. When the key path is empty, the server falls back
to the built-in default key. Whichever key the server uses, **the client must carry
the matching public key** — otherwise the server cannot decrypt the login block and
logins fail.

On Kubernetes the private key is supplied as a mounted Secret rather than a file on
disk; the Helm chart mounts it read-only and sets `world.rsa_private_key_path` for
you. See [Deployment scenarios](deployment.md#4-kubernetes-with-the-helm-chart).

## Health & monitoring

### The `/healthz` endpoint

The OnDemand HTTP server (default port **8080**) exposes `GET /healthz`. It is a
**readiness** signal that reflects the game tick loop, not just whether the port is
open:

| Situation | Response |
|-----------|----------|
| OnDemand running without a world (standalone) | `200` — the process is up |
| World wired, before the first game tick, within a 30-second boot-grace window | `200` — "starting" |
| World wired, boot grace elapsed with still no first tick | `503` — "no first tick" (startup wedged) |
| World wired, last tick within the last 10 seconds | `200` — healthy |
| World wired, last tick older than 10 seconds | `503` — "tick stale" (tick loop stalled) |

Because it tracks the tick loop, `/healthz` catches a world that still accepts TCP
connections but has stopped processing — a failure a plain port check cannot see.

!!! note "This is a readiness signal, not liveness"
    A `503` means "do not send this instance traffic right now"; it does not mean
    "restart me". In the Helm chart `/healthz` is wired only as a `readinessProbe`
    (for the SingleBinary and World deployment modes, which always run OnDemand
    alongside the world). A `503` removes the pod from the Service endpoints and it
    self-heals on the next healthy tick — it never triggers a restart. Management-only
    pods (login + friends, with no OnDemand HTTP port) use a coarser TCP readiness
    check on the login port instead. No liveness probe is configured.

### The `/debug/status` endpoint

`GET /debug/status` on the same HTTP server returns a small JSON snapshot for
eyeballing tick health and load:

```json
{
  "world_wired": true,
  "ticking": true,
  "last_tick_age_ms": 42,
  "current_tick": 10381,
  "players_online": 17,
  "tick_ms": 12
}
```

`tick_ms` is the duration of the last tick cycle (the target cadence is ~600 ms), and
`last_tick_age_ms` is how long ago the last tick landed — a rising value is the same
signal `/healthz` turns into a `503`.

### Logs

Logging is configured at the top level and, optionally, per module:

- `log_level` — minimum severity: `trace`, `debug`, `info`, `warn`, or `error`
  (default `info`).
- `log_format` — `text` (default) or `json`. Use `json` when shipping logs to an
  aggregator.
- A per-module `log_level` inside a module's section overrides the global level for
  that module only (YAML-only; there is no CLI flag). The `ondemand` module does not
  support `trace`; the `world`, `login`, and `friends` modules do.

See [Configuration](configuration.md#logging-and-per-module-log-levels) for the full
logging model.

## Restarts & upgrades

### Stopping the server cleanly

Send the process `SIGINT` (Ctrl-C) or `SIGTERM`. A single signal handler at the top
of the process asks every module to stop in reverse dependency order — each module
waits for its dependents to stop first — no module installs its own handler. As the world module stops, its tick loop runs a **final save-all**: every
still-online player is saved and logged out, and shutdown waits (with a bounded
timeout) for those save requests to flush to the login service before exiting. A clean
signal-based stop therefore preserves player progress made since the last autosave.

!!! note "One failure also stops everything"
    The process has no partial, half-running state. If any single module fails, the
    App stops the whole group and the process comes down. See the
    [service lifecycle](index.md#service-lifecycle) in the overview.

### In-game reboot commands

The engine also supports operator-scheduled reboots issued from the in-game admin
console: `::reboot` and `::slowreboot <seconds>`. Both are gated on staff level 3 or
higher **and** production mode.

!!! warning "Reboot commands are inert unless production mode is on"
    `::reboot` and `::slowreboot` only take effect when `world.node_production` is
    `true`. Under the default `world.node_production: false` they do nothing — the
    operator path for stopping the server is the OS signal above.

When production mode is enabled:

- **`::reboot`** schedules the shutdown on the current tick (effectively immediate).
- **`::slowreboot <seconds>`** schedules the shutdown after `ceil(seconds × 1000 / 600)`
  ticks and broadcasts a countdown timer to every connected player. Calling it with no
  argument does nothing.

Once a reboot is scheduled, the shutdown proceeds in stages: during the final ~50
ticks (~30 seconds) new logins are rejected so nobody is admitted only to be evicted;
at the shutdown tick every player is logged out (with a save); any player still stuck
after ~1024 ticks (~10 minutes) is force-removed; and once the world is empty, the
**world module** exits cleanly.

!!! warning "A reboot stops the world module, not the process"
    The world module's graceful exit does not stop anything else. In an `all`
    deployment, the OnDemand, login, and friends modules keep running and the process
    stays up — only a module *failure* stops the whole group, and a scheduled reboot
    is a clean termination, not a failure. `/healthz` eventually starts returning
    `503` as the tick signal goes stale, but nothing restarts or terminates the
    process (the Helm chart configures no liveness probe). Do not rely on `::reboot`
    or `::slowreboot` alone to bring an `all` process down for an upgrade: after the
    countdown has drained the players, send `SIGTERM` to stop the process itself.

### Upgrading the binary

An upgrade is, in the normal case, **replace the binary and restart** against the
same config and cache. Watch for these couplings:

1. **Verify the config first.** Config decoding is strict — an unknown key is a fatal
   boot error — so a config carried forward from an older revision that still names a
   since-removed key will fail to start. Validate it before you cut over:

   ```bash
   goscape --config.file /etc/goscape/goscape.yaml --config.verify=true
   ```

   Nothing binds a port; the command exits `0` if the config is valid. See
   [Verify a config before booting](quickstart.md#verify-a-config-before-booting).

2. **Back up the database before migrating.** The `database` module runs schema
   migrations automatically at startup, before `login` and `friends` come up. The
   migrations are forward-only, so if you ever need to roll back to the previous
   binary you roll back by restoring the pre-upgrade database backup — take one first
   (see [Backups](#backups)).

3. **Repack the cache when the engine expects a newer one.** The cache format is tied
   to the engine revision. When you upgrade across revisions, repack the cache from
   source with the new revision's tooling — see below.

## Cache packing

The **game cache** is the packed data the server hands to clients and reads maps
from. Two modules consume it: OnDemand serves cache archives to connecting clients
over HTTP, and the world server reads map data from `world.cache_path` (default
`./data/pack`).

### Packing the cache

Pack the cache with `make pack`, which drives the `goscape-cli pack` subcommand:

```bash
make pack
```

The Makefile passes three directories, each overridable as a make variable:

| Make variable | `goscape-cli pack` flag | Default | Contents |
|---------------|-------------------------|---------|----------|
| `CACHE_SRC_DIR` | `--src-dir` | `data/src` | Source content to pack |
| `CACHE_OUT_DIR` | `--out-dir` | `data/pack` | Packed output — point `world.cache_path` and OnDemand at this |
| `CACHE_RAW_DIR` | `--raw-dir` | `data/raw` | Engine-owned raw blobs (the `wordenc` Jagfile) |

For example, to pack from a different source tree into a different output directory:

```bash
make pack CACHE_SRC_DIR=/path/to/content CACHE_OUT_DIR=/srv/goscape/pack
```

### When to repack

Repack the cache whenever:

- **The source content changes** — maps, object/NPC/item configs, scripts, models,
  and so on. The packed output is a build product of the source; editing source
  without repacking has no effect on a running server.
- **You upgrade to a server revision whose engine expects a newer cache.** Repack
  from source with that revision's tooling as part of the upgrade.

!!! warning "Pack with the matching `goscape-cli` version"
    The cache layout is coupled to the engine. Pack with the `goscape-cli` built from
    the **same revision** as your `goscape` server binary — the two are built together
    (`make all`). A cache packed by a mismatched tool version can be served or read
    incorrectly.

!!! note "In-process rebuild is a development tool"
    The world module can also repack from `world.content_path` at runtime via the
    `::rebuild` admin command, but that requires `world.content_path` to be set and is
    a development convenience, not the operational packing path. Production
    deployments serve a cache packed with `make pack`.
