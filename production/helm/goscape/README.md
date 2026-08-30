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
| `SingleBinary` | StatefulSet | ondemand + world + login + friends + hiscore | PVC |
| `Management` | StatefulSet | login + friends + hiscore | PVC |
| `World` | Deployment | ondemand + world (dials Management) | none |

The `account` module (player portal + AccountService gRPC) rides on the two
stateful modes and is off by default — see [Account portal](#account-portal).

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

> **NetworkPolicy is same-namespace.** When `networkPolicy.enabled=true` in Management mode, only goscape pods carrying `app.kubernetes.io/name: goscape` in the **same namespace** may reach the login/friends gRPC ports. Install World releases in the same namespace as the Management release (or adjust the policy). The hiscore HTTP port has its own rule — see `hiscoreGateway` below.

### Account portal

The portal is opt-in, because it is the one module that cannot run on defaults
alone: `publicUrl` has no sensible default and the module refuses to start
without it. Enable it on a `SingleBinary` or `Management` release:

```bash
kubectl create secret generic goscape-account \
  --from-literal=admin-token="$(openssl rand -hex 32)"
helm upgrade --install <release> ./goscape \
  --set goscape.account.enabled=true \
  --set goscape.account.publicUrl=https://portal.example.com \
  --set goscape.account.existingSecret=goscape-account
```

That renders the `account:` config block, adds the portal (8081) and
AccountService (2006) container ports, and publishes the portal on the release's
Service. `accountIngress` is a separate Ingress from the ondemand one, since the
portal is browser-facing on its own hostname — which must match `publicUrl`,
because that value is the base of every emailed verification/reset link and the
OAuth redirect URI.

`existingSecret` names one Secret read for three optional keys: `admin-token`
(guards the admin gRPC surface), `smtp-password`, and `discord-client-secret`.
Only the keys you create are used — an absent key leaves the variable unset,
which expands to the empty string, which is already each field's "feature off"
value. **These values are substituted into `config.yaml` before it is parsed, so
they must be plain scalars**: a value containing `: `, or leading `{`/`[`/`*`/`&`/
`!`/`%`/`@`, will corrupt the file. Random hex or base64url tokens are safe. The
same applies to the PostgreSQL password, which is substituted into the DSN.

Settings without a dedicated key — argon2 cost, session TTLs — go in
`goscape.extraConfig` under `account:`.

`goscape.account.gameLogin=true` additionally points game login at the portal:
it sets `login.auth_mode: account`, dials AccountService over loopback, and
turns `login.auto_register` off, which that mode requires (with the portal in
charge, accounts are created there, not on first game login). It is deliberately
a separate switch: running the portal is not the same decision as changing how
every player authenticates. Existing players' saves are unaffected, but the
credentials they log in with change, so plan that switch with a maintenance
window.

The AccountService gRPC port is intentionally **not** on the Service: `login`
reaches it over loopback inside the same pod in both stateful modes, so a
Service port would widen a bearer-token-guarded admin surface with no in-cluster
consumer. Use `kubectl port-forward` when running `goscape-cli account`.

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
via `<mode>.extraEnv`, using `secretKeyRef` for secrets rather than a literal
value. (The portal's own secrets need none of that; see
[Account portal](#account-portal).)
Because expansion is unconditional, a literal `$` anywhere in `extraConfig`
must be escaped as `$$` per `drone/envsubst` (used to expand the config).

`goscape.node.debug` is the engine's debug mode (one value, rendered into both
`world.node_debug` and `ondemand.node_debug`, which the modules keep separately).
It sends players extra debug information such as missing-trigger messages, and it
is the gate on `/rs2.cgi` serving the Java applet bootstrap for `plugin=1`. It
stays `true` — the engine default this chart has always inherited — so pinning it
changed no behaviour; set it `false` for a public deployment that does not serve
the applet.

`image.digest` pins the image by digest (`repo@sha256:...`); when set, `image.tag` is ignored.

`hiscoreGateway.proxyNamespace` / `hiscoreGateway.proxyPodSelector` scope the
NetworkPolicy's hiscore rule to the Kong proxy pods when
`hiscoreGateway.createGatewayConfig` and `networkPolicy.enabled` are both true,
so in-cluster callers cannot bypass Kong's key-auth and rate limiting.

#### Upgrading from a release before these defaults

- **Volume ownership.** Pods previously ran as root, so everything already on an
  existing PVC — the sqlite database, `players/*.sav` — is owned by uid 0. The
  chart now runs as uid `65532` and relies on `fsGroup: 65532` (with
  `fsGroupChangePolicy: OnRootMismatch`, so the relabel happens once rather than
  on every start) to make that data writable again. This works on CSI drivers
  that honour fsGroup. On NFS, or on any driver whose CSIDriver object sets
  `fsGroupPolicy: None`, the kubelet does **not** apply fsGroup: `chown -R
  65532:65532` the volume contents before upgrading, or the world process will
  fail to write saves.
- **`$` in `extraConfig`.** `--config.expand-env=true` is now always on, so a
  literal `$` in `goscape.extraConfig` — most often inside a password or an
  argon2 PHC string — is consumed by the expansion and the value silently
  changes. Escape every literal `$` as `$$` before upgrading.
- **`world.content_watch`.** It writes a stamp file into `cache_path`, which is
  incompatible with the `readOnlyRootFilesystem: true` container default (and
  with a read-only cache mount). Leave it off in Kubernetes, or mount the cache
  path writable.

## Testing

Run the in-cluster connectivity test against a deployed release (requires a live cluster):

```bash
helm test <release>
```

For a cluster-free render/lint smoke check of all three example values files, use `make helm-test` and `make helm-lint` from the repo root. `make helm-test` also runs `helm-test-account`, which renders the portal-enabled variants and asserts each of the account guard rails fires.
