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

> **`extraConfig` keys must match goscape's config schema exactly.** goscape loads its config with strict unmarshalling — any unknown key under `goscape.extraConfig` will cause the pod to fail at startup.

### Custom login RSA key

By default the world server decrypts RuneScape login packets with a built-in RSA key. To use your own (World / SingleBinary modes), pre-create a Secret holding the PEM-encoded private key and reference it via `goscape.loginRsaKey.existingSecret`:

```bash
goscape-cli rsa gen --bits 1024 --out-dir ./keys
kubectl create secret generic goscape-login-rsa --from-file=private.pem=./keys/private.pem
helm upgrade --install <release> ./goscape \
  --set goscape.loginRsaKey.existingSecret=goscape-login-rsa
```

The Secret is mounted read-only at `/etc/goscape-login-rsa` and wired into `world.rsa_private_key_path`. The matching public key must be baked into the Java client (`Client.java` `LOGIN_RSAN` / `LOGIN_RSAE`), or every login fails. Leave `existingSecret` empty to keep the built-in key.

> **NetworkPolicy is same-namespace.** When `networkPolicy.enabled=true` in Management mode, only goscape pods carrying `app.kubernetes.io/name: goscape` in the **same namespace** may reach the login/friends gRPC ports. Install World releases in the same namespace as the Management release (or adjust the policy).

### Security defaults

Pods run as uid `65532` with a read-only root filesystem and all capabilities
dropped; the ServiceAccount token is not mounted (goscape never talks to the
Kubernetes API). The default memory limit is `2Gi` per workload (the world
process fills to its GC ceiling at ~1.1–1.3Gi under load). Liveness is a
`tcpSocket` probe on the primary port (`world-tcp`, or `login-grpc` in
Management) — `/healthz` is used only for readiness, since it can legitimately
return 503 during a slow cold-cache boot.

`--config.expand-env=true` is now always on, so `${VAR}` references inside
`goscape.extraConfig` resolve from the container's environment — set the var
via `<mode>.extraEnv`, using `secretKeyRef` for secrets such as
`account.admin_token` or SMTP/Discord credentials rather than a literal value.
Because expansion is unconditional, a literal `$` anywhere in `extraConfig`
must be escaped as `$$` per `drone/envsubst` (used to expand the config).

`image.digest` pins the image by digest (`repo@sha256:...`); when set, `image.tag` is ignored.

`hiscoreGateway.proxyNamespace` / `hiscoreGateway.proxyPodSelector` scope the
NetworkPolicy's hiscore rule to the Kong proxy pods when
`hiscoreGateway.createGatewayConfig` and `networkPolicy.enabled` are both true,
so in-cluster callers cannot bypass Kong's key-auth and rate limiting.

## Testing

Run the in-cluster connectivity test against a deployed release (requires a live cluster):

```bash
helm test <release>
```

For a cluster-free render/lint smoke check of all three example values files, use `make helm-test` and `make helm-lint` from the repo root.
