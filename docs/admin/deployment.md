# Deployment scenarios

goscape ships as a single binary that can run the whole server or just one part
of it. That flexibility means there is no single "correct" way to deploy it — the
right layout depends on how many players you serve and how you want to scale.
This page walks through five concrete layouts, from a one-process development
server to a Kubernetes fleet, and points at the exact configuration each one
needs.

Two choices shape every layout:

- **Which modules run where.** The `target` key selects a set of modules, and each
  module's own `enable` key gates it; a module runs only when both include it. The
  [Administrator's Guide overview](index.md#modules-and-the-target-flag) explains
  the modules, the [dependency graph](index.md#module-dependency-graph), and the
  [network listeners](index.md#network-surfaces-and-ports) they expose.
- **Which database backend the central database uses.** `login`, `friends`,
  `account` and `hiscore` are the modules that use the central database (`hiscore`
  only reads it, and `account` runs only when you enable it); `sqlite` keeps it in
  a local file, while `postgres` makes it a network service. See
  [Configuration](configuration.md#choosing-a-database-backend) for the
  `database:` section.

Throughout this page the ports `8080` (OnDemand HTTP), `43594` (world TCP), `2004`
(login gRPC), `2005` (friends gRPC), and `8082` (hiscores HTTP) are the values from the overview's
[listener table](index.md#network-surfaces-and-ports); every port and bind address
is configurable through the [Config reference](config-reference.md).

## 1. Single binary with SQLite (development and small deployments)

The simplest layout is one process that runs everything. Set `target: all` and the
process starts every module you have enabled; with the default `sqlite` backend it
keeps its state in a local database file and needs no external services at all. This is what
the bundled example preset does, and it is the layout the [Quick start](quickstart.md)
walks through step by step.

Inside that single process the world module still reaches login and friends over
gRPC, but those calls stay on the loopback interface (`127.0.0.1:2004` and
`127.0.0.1:2005` by default) — no network hop leaves the host.

```mermaid
graph LR
    client([Game client])
    subgraph host["Single host — one goscape process (target: all)"]
        direction LR
        ondemand["ondemand<br/>HTTP :8080"]
        world["world<br/>TCP :43594"]
        login["login<br/>gRPC :2004"]
        friends["friends<br/>gRPC :2005"]
        hiscore["hiscore<br/>HTTP :8082"]
        db[("SQLite file<br/>data/goscape.db")]
        world -->|loopback gRPC| login
        world -->|loopback gRPC| friends
        login -->|reads / writes| db
        friends -->|reads / writes| db
        hiscore -->|reads| db
    end
    client -->|game cache over HTTP| ondemand
    client -->|gameplay over TCP| world
```

*Figure 1: one process runs every enabled module; the login/friends gRPC calls
never leave the host, login and friends write to a single SQLite file, and
hiscore serves reads from it.*

!!! tip "When this layout is enough"
    A single-binary SQLite server is ideal for development, testing, and small
    single-world deployments. Because everything shares one process and one file,
    it has the fewest moving parts to operate. Move to one of the layouts below only
    when you need to scale a part of the server independently or spread it across
    hosts.

## 2. Split targets on one host

You can run each module as its own process on the same machine — for example one
process with `target: login`, one with `target: friends`, one with `target: world`,
and one with `target: ondemand`. Each process enables only the module it should run
(a target pulls its dependencies into the process, but a dependency stays dormant
unless its own `enable` is `true`). This lets you restart, resource-limit, or log
each part independently without splitting across hosts yet.

The processes coordinate through two channels. The world process **dials** the
login and friends processes over gRPC, so its config must point at them:

```yaml
world:
  login_server_enabled: true
  login_server_address: 127.0.0.1:2004     # --world.login-server-address
  friends_server_enabled: true             # default is false — set it to use friends
  friends_server_address: 127.0.0.1:2005   # --world.friends-server-address
```

The login and friends processes **share state** through the central database. Since
they are the only two central-database clients, they are the only processes that
need to reach it.

!!! warning "SQLite serializes writers; PostgreSQL is the safe multi-process choice"
    With the `sqlite` backend the login and friends processes must share a
    filesystem (hence the same host) because both open the same file. goscape opens
    SQLite in write-ahead-logging mode (`PRAGMA journal_mode=WAL`) with a
    five-second busy timeout (`busy_timeout(5000)`) and a single connection per
    process, so two processes writing one file **work**, but SQLite still allows
    only one writer at a time across the whole file: under write contention the
    second process waits up to the busy timeout and can then fail with a "database
    is locked" error. Each process also runs its own schema migrator at boot,
    serialized by the same timeout.

    Switch the backend to `postgres` once you split login and friends into separate
    processes. PostgreSQL is built for concurrent clients, removes the single-writer
    serialization, and is the only option once those two modules move onto different
    hosts (the next scenario). See
    [Configuration](configuration.md#choosing-a-database-backend).

```mermaid
graph LR
    client([Game client])
    subgraph host["Single host — four goscape processes"]
        direction LR
        ondemand["target: ondemand<br/>HTTP :8080"]
        world["target: world<br/>TCP :43594"]
        login["target: login<br/>gRPC :2004"]
        friends["target: friends<br/>gRPC :2005"]
        db[("Central database<br/>(shared SQLite file<br/>or PostgreSQL)")]
        world -->|gRPC :2004| login
        world -->|gRPC :2005| friends
        login -->|writer| db
        friends -->|writer| db
    end
    client -->|HTTP| ondemand
    client -->|TCP| world
```

*Figure 2: four processes on one host; the world process dials login and friends
over gRPC, and only login and friends touch the central database.*

## 3. Multi-host: central management plus world hosts

To scale past a single machine, split the server into a **central management host**
that runs `login` + `friends` against a PostgreSQL database, and **one or more world
hosts** that each run `world` + `ondemand` and dial the management host's gRPC
endpoints. This is the layout PostgreSQL exists for: the SQLite backend cannot span
hosts, so `postgres` is **required** here.

On the management host, choose the PostgreSQL backend and bind the login and friends
listeners to a routable address (the defaults bind `127.0.0.1`, which no other host
can reach):

```yaml
database:
  backend: postgres
  postgres:
    dsn: postgres://user:pass@db-host:5432/goscape?sslmode=disable
login:
  enable: true
  grpc_listen_address: 0.0.0.0   # reachable by the world hosts
friends:
  enable: true
  grpc_listen_address: 0.0.0.0
```

On each world host, disable the local login/friends modules and point the world
module at the management host with these keys:

```yaml
world:
  enable: true
  login_server_enabled: true
  login_server_address: mgmt-host:2004      # --world.login-server-address
  friends_server_enabled: true
  friends_server_address: mgmt-host:2005    # --world.friends-server-address
```

```mermaid
graph LR
    subgraph mgmt["Management host"]
        direction TB
        login["login<br/>gRPC :2004"]
        friends["friends<br/>gRPC :2005"]
        pg[("PostgreSQL")]
        login -->|reads / writes| pg
        friends -->|reads / writes| pg
    end
    subgraph w1["World host 1"]
        w1world["world<br/>TCP :43594"]
        w1od["ondemand<br/>HTTP :8080"]
    end
    subgraph wN["World host N"]
        wNworld["world<br/>TCP :43594"]
        wNod["ondemand<br/>HTTP :8080"]
    end
    w1world -->|login_server_address<br/>gRPC :2004| login
    w1world -->|friends_server_address<br/>gRPC :2005| friends
    wNworld -->|gRPC :2004| login
    wNworld -->|gRPC :2005| friends
```

*Figure 3: N world hosts dial one management host's login and friends gRPC
endpoints; login and friends share one PostgreSQL database.*

!!! note "Match the friends profile to the world profile"
    The friends server serves a single `profile` and rejects a world whose profile
    does not match. Keep the world module's `node_profile` (default `main`) and the
    friends module's `profile` (default `main`) in agreement across every host, or
    the world's connection to the friends server is refused.

## 4. Kubernetes with the Helm chart

The repository ships a Helm chart at `production/helm/goscape` that packages these
layouts for Kubernetes. You install it **once per role**, choosing the role with
`deploymentMode`:

| `deploymentMode` | Workload | Modules | State |
|---|---|---|---|
| `SingleBinary` | StatefulSet | ondemand + world + login + friends | PVC |
| `Management` | StatefulSet | login + friends | PVC |
| `World` | Deployment | ondemand + world (dials Management) | none |

`SingleBinary` maps to scenario 1 and `Management` + `World` map to scenario 3: a
single `Management` release (a StatefulSet with a PersistentVolumeClaim for its
database) is dialed by any number of stateless `World` releases (a Deployment you
can scale horizontally). Install one `World` release per world with a distinct
release name and `goscape.node.id`, pointing each at the management Service:

```bash
helm install goscape-mgmt ./production/helm/goscape \
  -f production/helm/goscape/management-values.yaml

helm install goscape-world-1 ./production/helm/goscape \
  -f production/helm/goscape/world-values.yaml \
  --set goscape.node.id=1 \
  --set goscape.loginServerAddress=goscape-mgmt:2004 \
  --set goscape.friendsServerAddress=goscape-mgmt:2005
```

The chart's rendered values for every revision are captured on the
[Helm values examples](helm-values.md) page. A few chart-specific points matter to
operators:

- **The image must carry the packed cache.** The chart expects the game cache baked
  into the image at `/usr/share/goscape/pack` (the default `goscape.cachePath`), so
  the pods have no cache PVC. Build the image with the repository's
  `cmd/goscape/Dockerfile`, which packs the content placed at `data/src` into the
  image during the build (see [scenario 5](#5-container-images)).
- **`goscape.extraConfig` is an escape hatch, deep-merged over the generated config.**
  Any setting without a dedicated chart value can be set under `goscape.extraConfig`,
  which is merged on top of the config the chart generates.

    !!! warning "`extraConfig` keys must match the config schema exactly"
        goscape decodes its config strictly, so an unknown key under
        `goscape.extraConfig` — a typo or a stale key — makes the pod fail at
        startup rather than warn. See
        [Configuration](configuration.md#strict-decoding) for how strict decoding
        behaves and how to catch a bad key before you deploy.

- **NetworkPolicy is same-namespace.** With `networkPolicy.enabled=true` in
  `Management` mode, only goscape pods carrying the `app.kubernetes.io/name: goscape`
  label **in the same namespace** may reach the login and friends gRPC ports.
  Install `World` releases in the same namespace as the `Management` release, or
  widen the policy yourself.
- **A custom login RSA key is a mounted Secret.** In `World` and `SingleBinary`
  modes you can replace the built-in login-decryption key: pre-create a Secret
  holding the PEM-encoded private key and reference it with
  `goscape.loginRsaKey.existingSecret`. The chart mounts it read-only at
  `/etc/goscape-login-rsa` and wires it into `world.rsa_private_key_path`. The Java
  client must carry the matching public key or every login fails; leave
  `existingSecret` empty to keep the built-in key.

For PostgreSQL, set `goscape.database.backend=postgres` (in `Management` or
`SingleBinary` mode) and supply the password through a referenced Secret rather than
in the values file.

```mermaid
graph LR
    subgraph ns["One Kubernetes namespace"]
        direction LR
        subgraph mgmtset["Management release (StatefulSet + PVC)"]
            mlogin["login<br/>:2004"]
            mfriends["friends<br/>:2005"]
        end
        pg[("PostgreSQL")]
        mlogin --> pg
        mfriends --> pg
        subgraph worldset["World release (Deployment, N replicas)"]
            wworld["world :43594"]
            wod["ondemand :8080"]
        end
        wworld -->|goscape-mgmt:2004| mlogin
        wworld -->|goscape-mgmt:2005| mfriends
    end
```

*Figure 4: the Helm chart's Management-plus-World topology in one namespace; the
stateful Management release is dialed by scalable, stateless World replicas.*

## 5. Container images

The Makefile's `images` target builds two images with `make images`:

- **`goscape`** (from `cmd/goscape/Dockerfile`) — the server image. Its multi-stage
  build compiles both binaries, packs the game cache, and assembles a small
  distroless final image containing the `goscape` server at `/usr/bin/goscape`
  (the entrypoint), the `goscape-cli` tool at `/usr/bin/goscape-cli`, and the
  **packed cache baked in at `/usr/share/goscape/pack`** — the path the Helm chart
  expects. The pack step reads source content from `data/src` in the build context,
  so that content must be present when you build.
- **`goscape-cli`** (from `cmd/goscape-cli/Dockerfile`) — a slim, tooling-only image
  whose entrypoint is the `goscape-cli` offline tool (cache pack/unpack, RuneScript
  compile, RSA key generation, and the other subcommands). Use it when you want the
  tooling without the full server image.

```mermaid
graph LR
    src["Repository checkout<br/>+ content at data/src"]
    subgraph make["make images"]
        d1["cmd/goscape/Dockerfile"]
        d2["cmd/goscape-cli/Dockerfile"]
    end
    src --> d1
    src --> d2
    d1 -->|goscape + goscape-cli<br/>+ packed cache| img1["goscape image<br/>cache at /usr/share/goscape/pack"]
    d2 -->|goscape-cli only| img2["goscape-cli image"]
```

*Figure 5: `make images` builds a server image with the cache baked in and a
separate tooling-only image.*

!!! note "Building the images"
    `make images` builds with `podman` by default (or `docker buildx` under CI,
    which produces multi-architecture `linux/amd64` and `linux/arm64` images). The
    image name prefix (`goscape/`) and tag come from Makefile variables. To pack a
    cache outside of an image build — for a local run or to prepare `data/src` — use
    `make pack`, described in the [Operations runbook](operations.md).
