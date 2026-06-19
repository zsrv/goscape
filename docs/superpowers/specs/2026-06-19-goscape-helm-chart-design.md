# goscape Helm chart — design

**Date:** 2026-06-19
**Status:** Approved design (pending spec review)
**Author:** brainstorming session

## 1. Goal

Ship a Helm chart for the `goscape` game server, modelled on Grafana Loki's
chart (`production/helm/loki`). Like Loki, it provides a small set of example
values files for distinct deployment shapes, selected by a single
`deploymentMode` value. As part of the same work, bake the packed game cache
into the goscape container image so the chart can run `world`/`ondemand` with
no external cache volume.

The chart is installed **once per role** (Loki's "values file = role"
convention). Each additional world is a separate `helm install` of the World
values with its own release name and `nodeId`.

## 2. Background — goscape topology

`goscape` is one binary with four modules selected at runtime. The CLI's
`--target` flag takes a **single** value (`ondemand`, `world`, `login`,
`friends`, or the composite `all`); `InitModuleServices` is called with one
target. Each module additionally gates itself on its own `enable` flag and
returns an idle no-op service when disabled. Therefore the robust, faithful way
to express any role is **`target: all` + per-module `enable` flags** — exactly
what `config.yaml` and `deploy/bundled/goscape.yaml` already do.

| Module | Protocol / default port | State | Notes |
|---|---|---|---|
| `ondemand` | HTTP `:8080` | none | Serves cache to game clients; optional WS bridge to world |
| `world` | TCP `:43594` | none (reads cache) | Dials `login`+`friends` over gRPC; in-memory game state |
| `login` | gRPC `:2004` | SQLite `login.db` + player saves `players/` | |
| `friends` | gRPC `:2005` | SQLite `friends.db` | |

Inter-module wiring: `world` connects out to login/friends via
`world.login_server_address` / `world.friends_server_address`
(+ `*_server_enabled`). Within a single pod these are `127.0.0.1`; across pods
they are Kubernetes Service DNS names.

The packed cache (`data/pack`: `main_file_cache.dat`, `main_file_cache.idx0..4`,
`maps/`, …) is a **generated artifact** produced by `goscape-cli pack` from
external source content. `data/` is gitignored (only `data/raw/` — engine-owned
raw blobs — is committed).

## 3. Deployment modes

A single chart value, `deploymentMode`, selects the role. It gates which
workload/service templates render and what config is generated. Config always
sets `target: all`; the per-module `enable` flags differ by mode. All network
listeners bind `0.0.0.0` inside the pod; what is exposed is controlled by
Services / NetworkPolicy.

| `deploymentMode` | Workload | `enable` flags | State / PVC | Client-facing ports | Internal ports |
|---|---|---|---|---|---|
| `SingleBinary` | StatefulSet, replicas 1 | ondemand, world, login, friends = true | PVC | ondemand HTTP, world TCP | login/friends on `127.0.0.1` (in-pod) |
| `Management` | StatefulSet, replicas 1 | login, friends = true; ondemand, world = false | PVC | — | login gRPC, friends gRPC (ClusterIP) |
| `World` | Deployment, replicas 1 | ondemand, world = true; login, friends = false | none | ondemand HTTP, world TCP | dials management gRPC via DNS |

Notes:
- Replicas are pinned to 1 in all modes. A goscape world is a singleton with
  in-memory state and a fixed `nodeId`; horizontal scaling of a single world is
  not meaningful. **More worlds = more World releases** (distinct `nodeId`).
  login/friends are SQLite singletons. The chart exposes a `replicas` knob but
  documents that values > 1 are unsupported.
- `World` mode mounts no volume: the cache is baked into the image and login/
  friends state lives on the remote Management release.

## 4. Config generation

A `ConfigMap` renders goscape's `config.yaml`, mounted at
`/etc/goscape/config.yaml`; the container runs
`--config.file=/etc/goscape/config.yaml`.

The template builds a base config from `deploymentMode`:
- sets `target: all` and the per-mode `enable` flags;
- binds all listen addresses to `0.0.0.0`;
- wires ports from `goscape.ports.*`;
- sets stateful paths from `goscape.dataPath` (`login.sqlite_dsn`,
  `login.save_path`, `friends.sqlite_dsn`) and the cache path from
  `goscape.cachePath` (`world.cache_path`, `ondemand.cache_path`);
- in `World` mode, sets `world.login_server_enabled/address` and
  `world.friends_server_enabled/address` from `goscape.loginServerAddress` /
  `goscape.friendsServerAddress`.

It then **deep-merges a free-form `goscape.extraConfig` map** over the base.
This is the escape hatch for any of goscape's ~50 flags without adding a
dedicated chart knob (mirrors Loki's `loki.structuredConfig`). Rationale: fully
templating every flag is high-maintenance and brittle; merge-over-base keeps the
template small while leaving every setting reachable.

A checksum annotation on the pod template (`sha256sum` of the rendered config)
triggers rollouts on config change.

## 5. Networking & addressing

- **SingleBinary** — one Service exposing ondemand HTTP + world TCP to clients.
  login/friends are reached on `127.0.0.1` in-pod and are not exposed.
- **Management** — a ClusterIP Service exposing login gRPC (`:2004`) + friends
  gRPC (`:2005`), internal only, for World releases to dial.
- **World** — a Service exposing ondemand HTTP + world TCP to clients. World
  pods dial the Management Service via `goscape.loginServerAddress` /
  `goscape.friendsServerAddress` (e.g. `goscape-mgmt-management:2004`). These
  default empty; in `World` mode the template fails with a clear message if they
  are unset, so misconfiguration surfaces at `helm install`, not at runtime.
- `service.type` is configurable per Service (default `ClusterIP`;
  `LoadBalancer`/`NodePort` for client-facing exposure).
- Optional **Ingress** for the ondemand HTTP endpoint (clients download cache
  over HTTP). World TCP cannot use an HTTP Ingress; it is exposed via
  Service type.
- Optional **NetworkPolicy** (default off): allow ingress to client-facing
  ports from anywhere; allow Management gRPC ingress only from pods carrying the
  chart's world selector labels; allow World egress to the Management Service.

## 6. Persistence

`Management` and `SingleBinary` get one PVC via `volumeClaimTemplates` on the
StatefulSet, mounted at `goscape.dataPath` (default `/var/lib/goscape`). Size,
storageClass, and accessModes are configurable (`persistence.*`). `World` mounts
nothing.

## 7. Container image — bake the cache (Dockerfile change)

Modify `cmd/goscape/Dockerfile`:
- Build stage (already does `COPY . .` and builds the binaries): after building,
  run the packer into a scratch output dir:
  ```
  RUN ./cmd/goscape-cli/goscape-cli pack \
        --src-dir ${CACHE_SRC_DIR} --raw-dir ${CACHE_RAW_DIR} --out-dir /pack
  ```
- Final stage: `COPY --from=build /pack /usr/share/goscape/pack`.
- Build args (with defaults): `CACHE_SRC_DIR=data/src`, `CACHE_RAW_DIR=data/raw`,
  `CACHE_OUT_DIR=/pack`, and `CACHE_IMAGE_DIR=/usr/share/goscape/pack`.

Implications (accepted):
- The source content must be present at `data/src` in the build context before
  `docker build`. Content is external to this repo; the operator checks out /
  symlinks it there. The build **fails** if `data/src` is absent (content is now
  a hard build requirement for this image).
- Add a `make pack` helper target that runs `goscape-cli pack` with the same
  defaults, for local cache generation / CI.
- `.dockerignore`: exclude the regenerated/runtime artifacts so the build
  context stays lean and stale local state never leaks in — `data/pack`,
  `data/pack.*`, `data/*.db`, `data/players`, `data/symbols` (regenerated by
  pack). Keep `data/src` and `data/raw` in context. (Conservative; verified
  against pack's actual inputs before finalizing.)
- The cache (~39 MB) is baked into the single image and is carried unused by
  Management pods — acceptable; one image is simpler to manage than a variant.

The chart's `goscape.cachePath` defaults to `/usr/share/goscape/pack` to match.

## 8. Helm test

Swap the Loki-style Go test binary for a lightweight **`helm test` hook Pod**
(annotation `helm.sh/hook: test`) using a stock `busybox` image:
- `nc -z` the relevant ports for the active mode (world TCP, ondemand HTTP,
  login/friends gRPC);
- `wget -q -O- http://<svc>:<port>/` against the ondemand endpoint where
  present.

This needs no custom image and no Go build. Makefile changes:
- Keep `helm-lint`.
- Repoint `helm-test` to a **cluster-free** check: `helm lint` + `helm template`
  rendered against each of the three example values files (catches template
  breakage in CI without a cluster).
- Remove the Go-helm-test targets and references: `helm-test` binary build rule,
  `helm-test-image`, `helm-test-push`, and `production/helm/goscape/src/helm-test/`.
- `helm test <release>` (live cluster) runs the connectivity Pod.

## 9. File layout

```
production/helm/goscape/
  Chart.yaml
  .helmignore
  values.yaml                       # all knobs, documented defaults
  values.schema.json                # light schema: deploymentMode enum, types
  single-binary-values.yaml         # deploymentMode: SingleBinary
  management-values.yaml            # deploymentMode: Management
  world-values.yaml                 # deploymentMode: World (+ nodeId, mgmt addrs)
  README.md
  README.md.gotmpl                  # helm-docs source
  templates/
    _helpers.tpl                    # names, labels, selector labels, image, config base
    NOTES.txt                       # post-install hints per mode
    configmap.yaml                  # renders goscape config.yaml
    serviceaccount.yaml
    single-binary/
      statefulset.yaml
      service.yaml
    management/
      statefulset.yaml
      service.yaml
    world/
      deployment.yaml
      service.yaml
    ondemand-ingress.yaml           # optional, gated by ingress.enabled
    poddisruptionbudget.yaml        # optional, gated
    networkpolicy.yaml              # optional, gated
    extra-manifests.yaml            # render arbitrary user manifests
    tests/
      test-connection.yaml          # helm test hook Pod (busybox)
```

Excluded per request: `servicemonitor.yaml`, `hpa.yaml`. Also excluded vs Loki
Full: the Go `src/helm-test/` (replaced by the Pod test).

## 10. values.yaml — key structure

```yaml
deploymentMode: SingleBinary        # SingleBinary | Management | World

nameOverride: ""
fullnameOverride: ""
commonLabels: {}
commonAnnotations: {}

image:
  registry: docker.io
  repository: goscape/goscape
  tag: ""                           # defaults to .Chart.AppVersion
  pullPolicy: IfNotPresent
  pullSecrets: []

serviceAccount:
  create: true
  name: ""
  annotations: {}
  automountServiceAccountToken: true

goscape:                            # → rendered into config.yaml
  logLevel: info
  logFormat: json
  cachePath: /usr/share/goscape/pack
  dataPath: /var/lib/goscape
  ports:
    ondemandHTTP: 8080
    worldTCP: 43594
    loginGRPC: 2004
    friendsGRPC: 2005
  node:
    id: 1                           # world nodeId (World / SingleBinary)
    members: true
    profile: main
  loginServerAddress: ""            # World mode: e.g. goscape-mgmt-management:2004
  friendsServerAddress: ""          # World mode: e.g. goscape-mgmt-management:2005
  extraConfig: {}                   # free-form, deep-merged over generated config

# Workload knobs — applied to whichever workload the mode renders.
# (Per-mode sections so different roles can differ.)
singleBinary: &workload
  replicas: 1
  resources: {}
  podAnnotations: {}
  podLabels: {}
  nodeSelector: {}
  tolerations: []
  affinity: {}
  podSecurityContext: {}
  containerSecurityContext: {}
  extraEnv: []
  extraArgs: []
management:
  <<: *workload
world:
  <<: *workload

persistence:                        # Management + SingleBinary
  enabled: true
  size: 5Gi
  storageClass: ""
  accessModes: [ReadWriteOnce]
  annotations: {}

service:
  type: ClusterIP
  annotations: {}
  # nodePorts / loadBalancerIP etc. as needed

ingress:                            # ondemand HTTP only
  enabled: false
  className: ""
  annotations: {}
  hosts: []
  tls: []

podDisruptionBudget:
  enabled: false
  maxUnavailable: 1

networkPolicy:
  enabled: false

extraManifests: []
```

(`extraEnv` notably allows `GOMEMLIMIT`, as in Loki's single-binary example.)

## 11. Verification

- `helm lint production/helm/goscape` clean.
- `helm template` against each of `single-binary-values.yaml`,
  `management-values.yaml`, `world-values.yaml` and assert per mode:
  - correct workload kind (StatefulSet vs Deployment) and that only that mode's
    templates render;
  - rendered `config.yaml` has `target: all`, the right `enable` flags, listen
    addresses `0.0.0.0`, correct ports, and (World) the management addresses;
  - Services expose the expected ports;
  - PVC present for Management/SingleBinary, absent for World.
- `make helm-lint` and the repointed `make helm-test` (cluster-free render
  check) pass.
- Dockerfile: `docker build` with content at `data/src` produces an image
  containing `/usr/share/goscape/pack/main_file_cache.dat` (+ idx files). Spot
  check via `docker run --rm <img> ls /usr/share/goscape/pack`.
- Out of scope unless a cluster is available: live `helm install` + `helm test`.

## 12. Out of scope

- Subcharts / bundled databases (login/friends use embedded SQLite).
- `ServiceMonitor`, `HPA` (excluded per request).
- Multi-replica / clustered login/friends (SQLite singleton).
- Building or pinning the external content repo; the chart and image assume
  content is supplied at `data/src` at build time.
```
