# Helm Chart: Custom Login RSA Key Support Design

**Date:** 2026-06-21
**Branches:** all 5 rev branches (rev-225, rev-244, rev-245.2, rev-254, rev-274)
**Scope:** Expose the `world.rsa_private_key_path` option (added in the custom-RSA-key feature) through the `production/helm/goscape` chart as a first-class, existing-Secret-backed option, so operators can run the world server with a custom login RSA key in Kubernetes.

## Context

The goscape binary now accepts `world.rsa_private_key_path` (a PEM RSA private key the **world server** uses to decrypt RuneScape login packets) on all 5 rev branches. The Helm chart renders `config.yaml` via the `goscape.baseConfig` helper and mounts only a `config` ConfigMap (+ a `data` PVC for stateful modes); it has **no file/secret mount mechanism**. This design adds one.

Key facts:
- The RSA key only matters for the **world module**, which runs in `SingleBinary` and `World` deployment modes (not `Management`, which is login+friends gRPC only).
- It's a **private key** → a Kubernetes Secret. Per the chosen approach, operators **pre-create the Secret** (`existingSecret`); the chart never creates a Secret or holds the PEM in Helm values/release.
- `goscape` uses `yaml.UnmarshalStrict`, so the rendered config may only contain keys the binary knows. `world.rsa_private_key_path` is accepted by all 5 branches' binaries.
- The chart files this touches (`values.yaml`, the `world:` section of `goscape.baseConfig`, and `goscape.podTemplate`) are **byte-identical across all 5 branches** (the per-branch helper trim is confined to `friends.profile` / `ondemand.cache_path`, untouched here). So the edit set is uniform; only verification is per-branch.
- `values.schema.json` is non-strict (no `additionalProperties: false`) — no schema change required.

## Design decisions

- **Name:** `goscape.loginRsaKey` (the chart only acts on it in World/SingleBinary modes — the world server). Its doc comment states the world-server/login-packet purpose explicitly.
- **Provisioning:** existing-Secret only. No inline PEM value, no chart-created Secret. Most secure; private keys never enter Helm values or the release.
- **Disabled by default:** empty `existingSecret` ⇒ no config line, no volume, no mount ⇒ built-in default key (zero behavior change on upgrade).
- **Mount path:** `/etc/goscape-login-rsa` — a non-nested sibling of the `/etc/goscape` config mount (avoids nested-volume fragility), read-only.
- **Mode gating:** the feature is inert in `Management` mode (no world server) — neither the config line nor the mount is rendered there.

## Files (per branch)

| File | Change |
|---|---|
| `production/helm/goscape/values.yaml` | Add the `goscape.loginRsaKey` block (`existingSecret: ""`, `key: private.pem`) with `# --` doc comments |
| `production/helm/goscape/templates/_helpers.tpl` | `goscape.baseConfig`: conditional `rsa_private_key_path` line in the `world:` section; `goscape.podTemplate`: conditional read-only secret volume + mount |
| `production/helm/goscape/README.md` | Add the two values rows to the generated values table (hand-edit if `helm-docs` is unavailable) |

No new template file (existing-Secret-only ⇒ the chart creates no Secret). No `values.schema.json` change.

## Component design

### Values (`values.yaml`)

Inserted in the `goscape:` block (e.g. after `extraConfig`):

```yaml
  # -- RSA private key the world server uses to decrypt RuneScape login packets
  #    (World / SingleBinary modes; sets world.rsa_private_key_path). Leave
  #    existingSecret empty to use the built-in default key. The Java client must
  #    carry the matching public key (Client.java LOGIN_RSAN / LOGIN_RSAE).
  #    Generate a keypair with `goscape-cli rsa gen`.
  loginRsaKey:
    # -- Name of an existing Secret holding the PEM-encoded RSA private key (PKCS#1 or PKCS#8). Empty disables the feature.
    existingSecret: ""
    # -- Key within existingSecret holding the PEM.
    key: private.pem
```

### Config line (`goscape.baseConfig`)

Added to the `world:` section, immediately after `cache_path` and before the per-mode `login_server_*` block. The gate: `existingSecret` non-empty AND mode ∈ {SingleBinary, World}.

```gotemplate
{{- if and $g.loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World")) }}
  rsa_private_key_path: {{ printf "/etc/goscape-login-rsa/%s" $g.loginRsaKey.key | quote }}
{{- end }}
```

The mount path constant `/etc/goscape-login-rsa` is duplicated in the podTemplate; both reference the same literal (a 2-site constant, acceptable for a chart; not worth a helper given Go-template string-bool gotchas).

### Volume + mount (`goscape.podTemplate`)

The same gate (using `$ctx.Values.goscape` / `$mode`) adds, in `volumeMounts`:

```gotemplate
{{- if and $ctx.Values.goscape.loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World")) }}
        - name: login-rsa
          mountPath: /etc/goscape-login-rsa
          readOnly: true
{{- end }}
```

and in `volumes`:

```gotemplate
{{- if and $ctx.Values.goscape.loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World")) }}
    - name: login-rsa
      secret:
        secretName: {{ $ctx.Values.goscape.loginRsaKey.existingSecret | quote }}
{{- end }}
```

## Data flow

```
values: goscape.loginRsaKey.existingSecret: my-login-rsa   (mode: SingleBinary|World)
   │
   ├─ goscape.baseConfig → world.rsa_private_key_path: /etc/goscape-login-rsa/private.pem
   └─ goscape.podTemplate → volume(secret my-login-rsa) + readOnly mount /etc/goscape-login-rsa
                                   │
   pod start → goscape --config.file → Config.Validate loads /etc/goscape-login-rsa/private.pem
```

Operator workflow: `goscape-cli rsa gen` → `kubectl create secret generic my-login-rsa --from-file=private.pem` → set `goscape.loginRsaKey.existingSecret=my-login-rsa` → bake the matching public key into the client.

## Error handling

| Condition | Behavior |
|---|---|
| `existingSecret` empty (default) | No config line / volume / mount; built-in key used |
| `existingSecret` set, mode = Management | Inert (no world server); nothing rendered |
| `existingSecret` names a missing Secret | Pod fails to mount at runtime (standard k8s behavior) — operator responsibility |
| Secret present but key file missing/invalid | `goscape` boot fails in `Config.Validate` with `world.rsa-private-key-path: …` (existing binary behavior) |

## Testing / verification (per branch)

1. `helm lint` against all three mode values files — passes.
2. **No-regression:** default render (no `loginRsaKey`) → extract `config.yaml` from the ConfigMap → `goscape --config.verify=true` exits 0. (Primary per-branch gate, per the chart-maintenance convention.)
3. **Enabled render:** `helm template … --set goscape.loginRsaKey.existingSecret=test-secret` for SingleBinary and World →
   - the rendered `config.yaml` `world:` section contains `rsa_private_key_path: "/etc/goscape-login-rsa/private.pem"`;
   - the workload (Deployment/StatefulSet) has a `login-rsa` volume sourced from Secret `test-secret` and a `readOnly` mount at `/etc/goscape-login-rsa`.
4. **Management inertness:** `helm template -f management-values.yaml --set goscape.loginRsaKey.existingSecret=test-secret` → no `rsa_private_key_path`, no `login-rsa` volume/mount.

The "key file exists" path is a runtime/k8s concern (mounted Secret), not a `--config.verify` concern, so it is not asserted in chart tests; the binary's own RSA load path is already covered by the RSA feature's Go tests.

## Out of scope

- Inline PEM value / chart-created Secret (rejected: keeps private keys out of Helm values).
- Generic `extraVolumes`/`extraVolumeMounts` (rejected for this turnkey feature; `extraConfig` + `extraManifests` already exist as escape hatches).
- OnDemand `pub_pem` / WebSocket token (a separate, later-revision feature; not on all branches).
- Backporting the goscape binary option (already done on all 5 branches).
