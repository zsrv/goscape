# Helm Chart Custom Login RSA Key — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose `world.rsa_private_key_path` through the `production/helm/goscape` chart as a first-class, existing-Secret-backed option (`goscape.loginRsaKey`) on all 5 rev branches.

**Architecture:** Add a `goscape.loginRsaKey` values block (existingSecret-only). When `existingSecret` is set and the deployment mode is SingleBinary or World, the chart appends `rsa_private_key_path` to the rendered `world:` config and mounts the referenced Secret read-only at `/etc/goscape-login-rsa`. The chart creates no Secret. The edit set is byte-identical across all 5 branches (the files touched don't diverge); only verification is per-branch.

**Tech Stack:** Helm v3 (`helm template`/`helm lint`), Go-template (`_helpers.tpl`), the goscape binary's `--config.verify`.

## Global Constraints

- Go invocations MUST be prefixed: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
- Commits MUST use `git commit --no-gpg-sign`.
- Chart path: `production/helm/goscape` (`CHART` below).
- Naming: the values key is `goscape.loginRsaKey`; the config key it sets is `world.rsa_private_key_path`; the mount path is `/etc/goscape-login-rsa` (non-nested sibling of the `/etc/goscape` config mount); the Secret volume/mount is named `login-rsa`.
- Provisioning is **existingSecret-only**: no inline PEM value, no chart-created Secret.
- Gate (all three insertion points): `existingSecret` non-empty AND deploymentMode ∈ {SingleBinary, World}. Inert in Management.
- Disabled by default (`existingSecret: ""`) ⇒ no config line, no volume, no mount ⇒ zero change to existing renders.
- `goscape` uses `yaml.UnmarshalStrict`; `world.rsa_private_key_path` is accepted by every branch's binary (RSA feature already shipped on all 5).
- Default branch order: rev-274 (current) first as pilot, then rev-254, rev-245.2, rev-244, rev-225.
- The files touched (`values.yaml`, `README.md`, and the `world:` section of `goscape.baseConfig` + `goscape.podTemplate` in `_helpers.tpl`) are byte-identical across all 5 branches. `values.yaml` and `README.md` are fully identical (diff=0) and may be copied verbatim from rev-274 to the other branches; `_helpers.tpl` gets the same 3 exact-string insertions (the other parts of that file differ per branch and must not be clobbered).

---

### Task 1: Implement on rev-274 (pilot) + author reusable scripts

**Files:**
- Modify: `production/helm/goscape/values.yaml`
- Modify: `production/helm/goscape/templates/_helpers.tpl`
- Modify: `production/helm/goscape/README.md`
- Create: `.superpowers/helm-rsa/apply_helm.py` (gitignored scratch; replication script for Tasks 2–5)
- Create: `.superpowers/helm-rsa/verify_helm.sh` (gitignored scratch; per-branch verification)

**Interfaces:**
- Produces: the `goscape.loginRsaKey` values contract and the three `_helpers.tpl` insertions, reused verbatim by Tasks 2–5.

- [ ] **Step 1: Write the verification script** (`.superpowers/helm-rsa/verify_helm.sh`)

```bash
#!/usr/bin/env bash
# Verify the helm login-rsa wiring on the currently-checked-out branch.
# Usage: verify_helm.sh <goscape-binary-path>
set -euo pipefail
GOSCAPE="$1"
CHART="production/helm/goscape"
SEC="test-secret"
WADDR='--set goscape.loginServerAddress=mgmt:2004 --set goscape.friendsServerAddress=mgmt:2005'

echo "== lint (3 modes) =="
helm lint "$CHART" -f "$CHART/single-binary-values.yaml"
helm lint "$CHART" -f "$CHART/management-values.yaml"
# shellcheck disable=SC2086
helm lint "$CHART" -f "$CHART/world-values.yaml" $WADDR

echo "== no-regression: default SingleBinary config validates =="
helm template r "$CHART" -f "$CHART/single-binary-values.yaml" \
  --show-only templates/configmap.yaml \
  | awk 'f{print} /config.yaml: \|/{f=1}' | sed 's/^    //' > "$TMPDIR/cfg.default.yaml"
"$GOSCAPE" --config.file "$TMPDIR/cfg.default.yaml" --config.verify=true
echo "  default verify exit=$?"

echo "== enabled SingleBinary: config line + volume + mount present =="
out=$(helm template r "$CHART" -f "$CHART/single-binary-values.yaml" \
  --set goscape.loginRsaKey.existingSecret="$SEC")
# NOTE: goscape.config round-trips via fromYaml|toYaml, which strips the | quote
# quotes, so the rendered value is unquoted (same as cache_path).
echo "$out" | grep -q 'rsa_private_key_path: /etc/goscape-login-rsa/private.pem' || { echo "FAIL: no config line"; exit 1; }
echo "$out" | grep -q 'secretName: "test-secret"' || { echo "FAIL: no secret volume"; exit 1; }
echo "$out" | grep -q 'mountPath: /etc/goscape-login-rsa' || { echo "FAIL: no mount"; exit 1; }
echo "$out" | grep -q 'readOnly: true' || { echo "FAIL: mount not readOnly"; exit 1; }

echo "== enabled World: config line + volume + mount present =="
# shellcheck disable=SC2086
out=$(helm template r "$CHART" -f "$CHART/world-values.yaml" $WADDR \
  --set goscape.loginRsaKey.existingSecret="$SEC")
echo "$out" | grep -q 'rsa_private_key_path: /etc/goscape-login-rsa/private.pem' || { echo "FAIL: World no config line"; exit 1; }
echo "$out" | grep -q 'mountPath: /etc/goscape-login-rsa' || { echo "FAIL: World no mount"; exit 1; }

echo "== Management inert: nothing rendered even when set =="
out=$(helm template r "$CHART" -f "$CHART/management-values.yaml" \
  --set goscape.loginRsaKey.existingSecret="$SEC")
echo "$out" | grep -q 'rsa_private_key_path' && { echo "FAIL: Management leaked config line"; exit 1; } || true
echo "$out" | grep -q 'login-rsa' && { echo "FAIL: Management leaked volume/mount"; exit 1; } || true

echo "ALL HELM RSA CHECKS PASSED"
```

- [ ] **Step 2: Build the goscape binary and run the verification — expect the enabled checks to FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o "$TMPDIR/goscape-274" ./cmd/goscape
bash .superpowers/helm-rsa/verify_helm.sh "$TMPDIR/goscape-274"
```
Expected: lint passes and the default-config verify exits 0, but the script FAILS at "enabled SingleBinary … no config line" (the feature isn't implemented yet). This confirms the verification is real.

- [ ] **Step 3: Add the `loginRsaKey` block to `values.yaml`**

Insert immediately after the `extraConfig: {}` line (`old_string` → `new_string`):

```yaml
  # -- Free-form map deep-merged over the generated config.yaml (escape hatch for any flag)
  extraConfig: {}
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

- [ ] **Step 4: Add the config line to `goscape.baseConfig`** (`_helpers.tpl`)

Replace the world-section `cache_path` + following conditional anchor:

old:
```
  cache_path: {{ $g.cachePath | quote }}
{{- if eq $mode "SingleBinary" }}
```
new:
```
  cache_path: {{ $g.cachePath | quote }}
{{- if and $g.loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World")) }}
  rsa_private_key_path: {{ printf "/etc/goscape-login-rsa/%s" $g.loginRsaKey.key | quote }}
{{- end }}
{{- if eq $mode "SingleBinary" }}
```

(The combined `cache_path` + `{{- if eq $mode "SingleBinary" }}` anchor is unique to the world section — the ondemand `cache_path` is followed by `login:`.)

- [ ] **Step 5: Add the volume mount to `goscape.podTemplate`** (`_helpers.tpl`)

old:
```
      volumeMounts:
        - name: config
          mountPath: /etc/goscape
```
new:
```
      volumeMounts:
        - name: config
          mountPath: /etc/goscape
        {{- if and $ctx.Values.goscape.loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World")) }}
        - name: login-rsa
          mountPath: /etc/goscape-login-rsa
          readOnly: true
        {{- end }}
```

- [ ] **Step 6: Add the volume to `goscape.podTemplate`** (`_helpers.tpl`)

old:
```
  volumes:
    - name: config
      configMap:
        name: {{ include "goscape.fullname" $ctx }}
```
new:
```
  volumes:
    - name: config
      configMap:
        name: {{ include "goscape.fullname" $ctx }}
    {{- if and $ctx.Values.goscape.loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World")) }}
    - name: login-rsa
      secret:
        secretName: {{ $ctx.Values.goscape.loginRsaKey.existingSecret | quote }}
    {{- end }}
```

- [ ] **Step 7: Document the option in `README.md`**

Insert after the strict-unmarshalling blockquote in the `## Configuration` section:

old:
```
> **`extraConfig` keys must match goscape's config schema exactly.** goscape loads its config with strict unmarshalling — any unknown key under `goscape.extraConfig` will cause the pod to fail at startup.
```
new:
```
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
```

- [ ] **Step 8: Author the replication script** (`.superpowers/helm-rsa/apply_helm.py`)

```python
#!/usr/bin/env python3
"""Replicate the helm login-rsa edits onto the current branch.
values.yaml + README.md are identical across branches -> copied verbatim from
rev-274 (edited+committed in Task 1). _helpers.tpl gets the 3 exact-string
insertions (the rest of that file differs per branch and must not be clobbered).
"""
import subprocess

REPO = subprocess.check_output(["git", "rev-parse", "--show-toplevel"]).decode().strip()
H = f"{REPO}/production/helm/goscape"


def git_show(rev_path, dest):
    data = subprocess.check_output(["git", "show", rev_path])
    with open(dest, "wb") as f:
        f.write(data)


def sub_once(path, old, new, label):
    with open(path) as f:
        s = f.read()
    n = s.count(old)
    if n != 1:
        raise SystemExit(f"ABORT [{label}]: expected 1 occurrence, found {n}")
    with open(path, "w") as f:
        f.write(s.replace(old, new, 1))


# values.yaml + README.md: verbatim from rev-274
git_show("rev-274:production/helm/goscape/values.yaml", f"{H}/values.yaml")
git_show("rev-274:production/helm/goscape/README.md", f"{H}/README.md")

tpl = f"{H}/templates/_helpers.tpl"

sub_once(tpl,
    "  cache_path: {{ $g.cachePath | quote }}\n{{- if eq $mode \"SingleBinary\" }}",
    "  cache_path: {{ $g.cachePath | quote }}\n"
    "{{- if and $g.loginRsaKey.existingSecret (or (eq $mode \"SingleBinary\") (eq $mode \"World\")) }}\n"
    "  rsa_private_key_path: {{ printf \"/etc/goscape-login-rsa/%s\" $g.loginRsaKey.key | quote }}\n"
    "{{- end }}\n"
    "{{- if eq $mode \"SingleBinary\" }}",
    "baseConfig rsa line")

sub_once(tpl,
    "      volumeMounts:\n        - name: config\n          mountPath: /etc/goscape\n",
    "      volumeMounts:\n        - name: config\n          mountPath: /etc/goscape\n"
    "        {{- if and $ctx.Values.goscape.loginRsaKey.existingSecret (or (eq $mode \"SingleBinary\") (eq $mode \"World\")) }}\n"
    "        - name: login-rsa\n"
    "          mountPath: /etc/goscape-login-rsa\n"
    "          readOnly: true\n"
    "        {{- end }}\n",
    "podTemplate volumeMount")

sub_once(tpl,
    "  volumes:\n    - name: config\n      configMap:\n        name: {{ include \"goscape.fullname\" $ctx }}\n",
    "  volumes:\n    - name: config\n      configMap:\n        name: {{ include \"goscape.fullname\" $ctx }}\n"
    "    {{- if and $ctx.Values.goscape.loginRsaKey.existingSecret (or (eq $mode \"SingleBinary\") (eq $mode \"World\")) }}\n"
    "    - name: login-rsa\n"
    "      secret:\n"
    "        secretName: {{ $ctx.Values.goscape.loginRsaKey.existingSecret | quote }}\n"
    "    {{- end }}\n",
    "podTemplate volume")

print("apply_helm.py: OK")
```

- [ ] **Step 9: Run the verification — expect PASS**

```bash
bash .superpowers/helm-rsa/verify_helm.sh "$TMPDIR/goscape-274"
```
Expected: `ALL HELM RSA CHECKS PASSED`.

- [ ] **Step 10: Commit**

```bash
git add production/helm/goscape/values.yaml production/helm/goscape/templates/_helpers.tpl production/helm/goscape/README.md
git commit --no-gpg-sign -m "feat(helm): optional custom login RSA key via goscape.loginRsaKey

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Replicate to rev-254

**Files:** same three chart files, on rev-254.

**Interfaces:**
- Consumes: rev-274's committed `values.yaml`/`README.md` and the `_helpers.tpl` insertions (Task 1), via `.superpowers/helm-rsa/apply_helm.py`.

- [ ] **Step 1: Checkout and apply**

```bash
git checkout rev-254
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache python3 .superpowers/helm-rsa/apply_helm.py
```
Expected: `apply_helm.py: OK` (all three `_helpers.tpl` anchors found exactly once).

- [ ] **Step 2: Build this branch's binary and verify**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o "$TMPDIR/goscape-254" ./cmd/goscape
bash .superpowers/helm-rsa/verify_helm.sh "$TMPDIR/goscape-254"
```
Expected: `ALL HELM RSA CHECKS PASSED`.

- [ ] **Step 3: Commit**

```bash
git add production/helm/goscape/values.yaml production/helm/goscape/templates/_helpers.tpl production/helm/goscape/README.md
git commit --no-gpg-sign -m "feat(helm): optional custom login RSA key via goscape.loginRsaKey

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Replicate to rev-245.2

Identical to Task 2 with `rev-245.2` and binary `$TMPDIR/goscape-2452`.

- [ ] **Step 1: Checkout and apply**

```bash
git checkout rev-245.2
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache python3 .superpowers/helm-rsa/apply_helm.py
```
Expected: `apply_helm.py: OK`.

- [ ] **Step 2: Build and verify**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o "$TMPDIR/goscape-2452" ./cmd/goscape
bash .superpowers/helm-rsa/verify_helm.sh "$TMPDIR/goscape-2452"
```
Expected: `ALL HELM RSA CHECKS PASSED`.

- [ ] **Step 3: Commit**

```bash
git add production/helm/goscape/values.yaml production/helm/goscape/templates/_helpers.tpl production/helm/goscape/README.md
git commit --no-gpg-sign -m "feat(helm): optional custom login RSA key via goscape.loginRsaKey

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Replicate to rev-244

Identical to Task 2 with `rev-244` and binary `$TMPDIR/goscape-244`.

- [ ] **Step 1: Checkout and apply**

```bash
git checkout rev-244
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache python3 .superpowers/helm-rsa/apply_helm.py
```
Expected: `apply_helm.py: OK`.

- [ ] **Step 2: Build and verify**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o "$TMPDIR/goscape-244" ./cmd/goscape
bash .superpowers/helm-rsa/verify_helm.sh "$TMPDIR/goscape-244"
```
Expected: `ALL HELM RSA CHECKS PASSED`.

- [ ] **Step 3: Commit**

```bash
git add production/helm/goscape/values.yaml production/helm/goscape/templates/_helpers.tpl production/helm/goscape/README.md
git commit --no-gpg-sign -m "feat(helm): optional custom login RSA key via goscape.loginRsaKey

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Replicate to rev-225

Identical to Task 2 with `rev-225` and binary `$TMPDIR/goscape-225`. (rev-225's chart files are still byte-identical for the touched parts; the `apply_helm.py` anchor asserts guard this.)

- [ ] **Step 1: Checkout and apply**

```bash
git checkout rev-225
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache python3 .superpowers/helm-rsa/apply_helm.py
```
Expected: `apply_helm.py: OK`.

- [ ] **Step 2: Build and verify**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o "$TMPDIR/goscape-225" ./cmd/goscape
bash .superpowers/helm-rsa/verify_helm.sh "$TMPDIR/goscape-225"
```
Expected: `ALL HELM RSA CHECKS PASSED`.

- [ ] **Step 3: Commit, then return to rev-274**

```bash
git add production/helm/goscape/values.yaml production/helm/goscape/templates/_helpers.tpl production/helm/goscape/README.md
git commit --no-gpg-sign -m "feat(helm): optional custom login RSA key via goscape.loginRsaKey

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
git checkout rev-274
```

---

## Self-Review

**Spec coverage:**
- `goscape.loginRsaKey` values block (existingSecret + key) → Task 1 Step 3 ✓
- Config line gated on existingSecret + mode → Task 1 Step 4 ✓
- Read-only secret volume + mount at `/etc/goscape-login-rsa`, same gate → Task 1 Steps 5–6 ✓
- README documentation → Task 1 Step 7 ✓
- No new template file / no schema change → respected (only 3 files touched) ✓
- All 5 branches → Tasks 1–5 ✓
- Verification: lint (3 modes), no-regression `--config.verify`, enabled SingleBinary+World structural asserts, Management inertness → `verify_helm.sh`, run in every task ✓
- Out-of-scope items (inline PEM, generic volumes, pub_pem) → excluded ✓

**Placeholder scan:** none — every edit shows exact old/new strings; every command has expected output; scripts are complete.

**Type/anchor consistency:** the gate condition `and …loginRsaKey.existingSecret (or (eq $mode "SingleBinary") (eq $mode "World"))`, the mount path `/etc/goscape-login-rsa`, the volume name `login-rsa`, and the config key `rsa_private_key_path` are identical across Steps 4–6, the `apply_helm.py` insertions, and the `verify_helm.sh` assertions.
