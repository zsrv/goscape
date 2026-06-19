# goscape Helm chart

Helm chart for the [goscape](https://github.com/zsrv/goscape) game server.
Installed **once per role**, selected by `deploymentMode`.

## Prerequisites

- Kubernetes + Helm v3.
- A goscape image with the packed game cache baked in at
  `/usr/share/goscape/pack`. Build it with the repo's `cmd/goscape/Dockerfile`
  after placing the source content at `data/src` (see repo `make pack`).

## Deployment modes

| `deploymentMode` | Workload | Modules | State |
|---|---|---|---|
| `SingleBinary` | StatefulSet | ondemand + world + login + friends | PVC |
| `Management` | StatefulSet | login + friends | PVC |
| `World` | Deployment | ondemand + world (dials Management) | none |

## Install

Single binary (everything in one pod):

```bash
helm install goscape ./production/helm/goscape -f production/helm/goscape/single-binary-values.yaml
```

Central management + multiple worlds:

```bash
helm install goscape-mgmt ./production/helm/goscape -f production/helm/goscape/management-values.yaml

helm install goscape-world-1 ./production/helm/goscape -f production/helm/goscape/world-values.yaml \
  --set goscape.node.id=1 \
  --set goscape.loginServerAddress=goscape-mgmt:2004 \
  --set goscape.friendsServerAddress=goscape-mgmt:2005

helm install goscape-world-2 ./production/helm/goscape -f production/helm/goscape/world-values.yaml \
  --set goscape.node.id=2 \
  --set goscape.loginServerAddress=goscape-mgmt:2004 \
  --set goscape.friendsServerAddress=goscape-mgmt:2005
```

The Management release's NOTES prints the exact `loginServerAddress` /
`friendsServerAddress` to use.

## Configuration

Common values live under `goscape.*` (rendered into goscape's `config.yaml`).
Any setting without a dedicated key can be set via `goscape.extraConfig`, which
is deep-merged over the generated config. Workload knobs (resources, scheduling,
security contexts, `extraEnv`/`extraArgs`) live under the per-mode section
(`singleBinary` / `management` / `world`).

## Testing

Run the in-cluster connectivity test against a deployed release (requires a live cluster):

```bash
helm test <release>
```

For a cluster-free render/lint smoke check of all three example values files, use `make helm-test` and `make helm-lint` from the repo root.
