# goscape Helm Chart Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Loki-style Helm chart for goscape at `production/helm/goscape/` with three role-based example values files, plus bake the packed game cache into the goscape container image.

**Architecture:** A single chart installed once per role, selected by a `deploymentMode` value (`SingleBinary` / `Management` / `World`). Config is rendered into a ConfigMap from `deploymentMode` + `goscape.*` values (always `target: all`, per-mode `enable` flags) with a free-form `goscape.extraConfig` deep-merge escape hatch. A shared `goscape.podTemplate` helper produces the pod spec for all modes; SingleBinary/Management render a StatefulSet with a PVC, World renders a Deployment with the cache baked into the image and dials the Management release over gRPC DNS.

**Tech Stack:** Helm v3 (templates, sprig), Kubernetes (StatefulSet/Deployment/Service/ConfigMap/PVC/Ingress/PDB/NetworkPolicy), Docker multi-stage build, goscape Go binary + `goscape-cli pack`, Make.

## Global Constraints

- Chart path is exactly `production/helm/goscape/` (the Makefile already references it).
- The goscape config always uses `target: all`; role is expressed via per-module `enable` flags. `--target` takes a single value only.
- `deploymentMode` is one of `SingleBinary`, `Management`, `World`.
- All in-pod network listeners bind `0.0.0.0`; exposure is controlled by Services/NetworkPolicy.
- Cache is baked into the image at `/usr/share/goscape/pack`; chart default `goscape.cachePath: /usr/share/goscape/pack`.
- Do NOT create `servicemonitor.yaml` or `hpa.yaml`.
- Replicas are 1 in all modes (singletons); more worlds = more World releases.
- **Workload structure (supersedes the spec's per-mode-subdir layout, per user decision):** the pod spec lives in ONE `goscape.podTemplate` helper shared by all modes; there is ONE `templates/statefulset.yaml` (SingleBinary + Management), ONE `templates/deployment.yaml` (World), and ONE mode-aware `templates/service.yaml`. Do not duplicate the pod spec per mode.
- All `go`/`goscape-cli` invocations are prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`.
- All commits use `git commit --no-gpg-sign` and end the message body with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Ports: ondemand HTTP 8080, world TCP 43594, login gRPC 2004, friends gRPC 2005.
- Tooling present: `helm` v3.19, `docker`. Verify with `helm version --short` before starting.

---

### Task 1: Chart skeleton (lintable empty chart)

**Files:**
- Create: `production/helm/goscape/Chart.yaml`
- Create: `production/helm/goscape/.helmignore`
- Create: `production/helm/goscape/values.schema.json`
- Create: `production/helm/goscape/values.yaml`
- Create: `production/helm/goscape/templates/_helpers.tpl`
- Create: `production/helm/goscape/templates/serviceaccount.yaml`
- Create: `production/helm/goscape/templates/NOTES.txt`

**Interfaces:**
- Produces (for all later tasks): helpers `goscape.name`, `goscape.fullname`, `goscape.chart`, `goscape.labels`, `goscape.selectorLabels`, `goscape.serviceAccountName`, `goscape.image`. Values root keys: `deploymentMode`, `image`, `serviceAccount`, `goscape`, `singleBinary`, `management`, `world`, `persistence`, `service`, `ingress`, `podDisruptionBudget`, `networkPolicy`, `extraManifests`, `commonLabels`, `commonAnnotations`, `nameOverride`, `fullnameOverride`.

- [ ] **Step 1: Write the failing test**

Run this; it must fail because the chart does not exist yet:

```bash
helm lint production/helm/goscape -f production/helm/goscape/values.yaml
```

Expected: FAIL — `Error: ... no chart found` (or path not found).

- [ ] **Step 2: Create `Chart.yaml`**

```yaml
apiVersion: v2
name: goscape
description: Helm chart for goscape — a Go RuneScape server supporting single-binary, central management, and per-world deployment modes.
type: application
version: 0.1.0
appVersion: "0.1.0"
home: https://github.com/zsrv/goscape
sources:
  - https://github.com/zsrv/goscape
maintainers:
  - name: zsrv
```

- [ ] **Step 3: Create `.helmignore`**

```gitignore
.DS_Store
.git/
.gitignore
*.swp
*.bak
*.tmp
*.orig
*~
.project
.idea/
.vscode/
README.md.gotmpl
ci/
```

- [ ] **Step 4: Create `values.schema.json`** (lenient: only constrain `deploymentMode`)

```json
{
  "$schema": "https://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "deploymentMode": {
      "type": "string",
      "enum": ["SingleBinary", "Management", "World"]
    }
  },
  "required": ["deploymentMode"]
}
```

- [ ] **Step 5: Create `values.yaml`** (full default surface)

```yaml
# -- Deployment role for this release: SingleBinary | Management | World
deploymentMode: SingleBinary

# -- Override the chart name portion of resource names
nameOverride: ""
# -- Override the full resource name
fullnameOverride: ""
# -- Labels added to every resource
commonLabels: {}
# -- Annotations added to every resource
commonAnnotations: {}

image:
  # -- Image registry. Leave empty to omit the registry prefix.
  registry: docker.io
  # -- Image repository
  repository: goscape/goscape
  # -- Image tag. Defaults to the chart appVersion when empty.
  tag: ""
  # -- Image pull policy
  pullPolicy: IfNotPresent
  # -- Image pull secrets
  pullSecrets: []

serviceAccount:
  # -- Create a ServiceAccount
  create: true
  # -- Name of the ServiceAccount; generated when empty
  name: ""
  # -- Annotations for the ServiceAccount
  annotations: {}
  # -- Automount the ServiceAccount token
  automountServiceAccountToken: true

# goscape application config (rendered into config.yaml)
goscape:
  # -- Log level: debug | info | warn | error
  logLevel: info
  # -- Log format: text | json
  logFormat: json
  # -- Path to the baked-in packed cache (must match the image)
  cachePath: /usr/share/goscape/pack
  # -- Data dir for stateful modes (login/friends SQLite + player saves)
  dataPath: /var/lib/goscape
  ports:
    # -- ondemand HTTP container port
    ondemandHTTP: 8080
    # -- world TCP container port
    worldTCP: 43594
    # -- login gRPC container port
    loginGRPC: 2004
    # -- friends gRPC container port
    friendsGRPC: 2005
  node:
    # -- World id (World / SingleBinary modes)
    id: 1
    # -- Whether members content is available
    members: true
    # -- Node profile name
    profile: main
  # -- World mode only: address of the Management login gRPC, e.g. goscape-mgmt:2004
  loginServerAddress: ""
  # -- World mode only: address of the Management friends gRPC, e.g. goscape-mgmt:2005
  friendsServerAddress: ""
  # -- Free-form map deep-merged over the generated config.yaml (escape hatch for any flag)
  extraConfig: {}

# Workload knobs for SingleBinary mode
singleBinary:
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

# Workload knobs for Management mode
management:
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

# Workload knobs for World mode
world:
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

# Persistent storage (Management + SingleBinary)
persistence:
  enabled: true
  size: 5Gi
  storageClass: ""
  accessModes:
    - ReadWriteOnce
  annotations: {}

service:
  # -- Service type for the primary Service of this release
  type: ClusterIP
  # -- Annotations for the Service
  annotations: {}

# Ingress for the ondemand HTTP endpoint (SingleBinary / World)
ingress:
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

# Arbitrary extra manifests rendered verbatim (list of objects or template strings)
extraManifests: []
```

- [ ] **Step 6: Create `templates/_helpers.tpl`**

```
{{/* Chart name (overridable) */}}
{{- define "goscape.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fully qualified app name */}}
{{- define "goscape.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Chart label */}}
{{- define "goscape.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Common labels */}}
{{- define "goscape.labels" -}}
helm.sh/chart: {{ include "goscape.chart" . }}
{{ include "goscape.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: {{ .Values.deploymentMode | lower }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/* Selector labels */}}
{{- define "goscape.selectorLabels" -}}
app.kubernetes.io/name: {{ include "goscape.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* ServiceAccount name */}}
{{- define "goscape.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "goscape.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/* Fully qualified image reference */}}
{{- define "goscape.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- if .Values.image.registry -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.image.repository $tag -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
{{- end -}}
```

- [ ] **Step 7: Create `templates/serviceaccount.yaml`**

```
{{- if .Values.serviceAccount.create -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "goscape.serviceAccountName" . }}
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
  {{- with .Values.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
automountServiceAccountToken: {{ .Values.serviceAccount.automountServiceAccountToken }}
{{- end }}
```

- [ ] **Step 8: Create `templates/NOTES.txt`**

```
goscape "{{ .Release.Name }}" installed in namespace "{{ .Release.Namespace }}".

Deployment mode: {{ .Values.deploymentMode }}

{{- if eq .Values.deploymentMode "SingleBinary" }}

All four modules run in one StatefulSet. Client-facing Services:
  ondemand HTTP : {{ include "goscape.fullname" . }}:{{ .Values.goscape.ports.ondemandHTTP }}
  world TCP     : {{ include "goscape.fullname" . }}:{{ .Values.goscape.ports.worldTCP }}
{{- else if eq .Values.deploymentMode "Management" }}

login + friends run as a central management StatefulSet. World releases should set:
  goscape.loginServerAddress   = {{ include "goscape.fullname" . }}:{{ .Values.goscape.ports.loginGRPC }}
  goscape.friendsServerAddress = {{ include "goscape.fullname" . }}:{{ .Values.goscape.ports.friendsGRPC }}
{{- else if eq .Values.deploymentMode "World" }}

ondemand + world run as a world Deployment dialing management at:
  login   : {{ .Values.goscape.loginServerAddress }}
  friends : {{ .Values.goscape.friendsServerAddress }}
Client-facing Services:
  ondemand HTTP : {{ include "goscape.fullname" . }}:{{ .Values.goscape.ports.ondemandHTTP }}
  world TCP     : {{ include "goscape.fullname" . }}:{{ .Values.goscape.ports.worldTCP }}
{{- end }}

Run "helm test {{ .Release.Name }}" to smoke-test connectivity.
```

- [ ] **Step 9: Run the lint + render tests to verify they pass**

```bash
helm lint production/helm/goscape -f production/helm/goscape/values.yaml
helm template t production/helm/goscape -f production/helm/goscape/values.yaml --show-only templates/serviceaccount.yaml | grep -q 'kind: ServiceAccount'
```

Expected: lint reports `1 chart(s) linted, 0 chart(s) failed`; grep exits 0.

- [ ] **Step 10: Commit**

```bash
git add production/helm/goscape
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(helm): scaffold goscape chart skeleton

Chart.yaml, helpers, values, schema, ServiceAccount, NOTES.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Config generation (ConfigMap)

**Files:**
- Modify: `production/helm/goscape/templates/_helpers.tpl` (append two defines)
- Create: `production/helm/goscape/templates/configmap.yaml`

**Interfaces:**
- Consumes: helpers and values from Task 1.
- Produces: helper `goscape.config` (renders the full config.yaml as text, base + `extraConfig` deep-merge). Later workload tasks reference `include "goscape.config" .` for the pod `checksum/config` annotation and mount the ConfigMap named `{{ include "goscape.fullname" . }}` at `/etc/goscape`.

- [ ] **Step 1: Write the failing test**

```bash
helm template t production/helm/goscape -f production/helm/goscape/values.yaml --show-only templates/configmap.yaml
```

Expected: FAIL — `Error: could not find template templates/configmap.yaml in chart`.

- [ ] **Step 2: Append the config helpers to `templates/_helpers.tpl`**

```
{{/* goscape.baseConfig — the generated config.yaml before extraConfig merge */}}
{{- define "goscape.baseConfig" -}}
{{- $mode := .Values.deploymentMode -}}
{{- $g := .Values.goscape -}}
target: all
log_level: {{ $g.logLevel | quote }}
log_format: {{ $g.logFormat | quote }}
ondemand:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "World") }}
  http_listen_network: tcp
  http_listen_address: 0.0.0.0
  http_listen_port: {{ $g.ports.ondemandHTTP }}
  cache_path: {{ $g.cachePath | quote }}
login:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "Management") }}
  grpc_listen_address: 0.0.0.0
  grpc_listen_port: {{ $g.ports.loginGRPC }}
  sqlite_dsn: {{ printf "%s/login.db" $g.dataPath | quote }}
  save_path: {{ printf "%s/players" $g.dataPath | quote }}
  node_profile: {{ $g.node.profile | quote }}
friends:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "Management") }}
  grpc_listen_address: 0.0.0.0
  grpc_listen_port: {{ $g.ports.friendsGRPC }}
  sqlite_dsn: {{ printf "%s/friends.db" $g.dataPath | quote }}
  profile: {{ $g.node.profile | quote }}
world:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "World") }}
  tcp_listen_network: tcp
  tcp_listen_address: 0.0.0.0
  tcp_listen_port: {{ $g.ports.worldTCP }}
  node_id: {{ $g.node.id }}
  node_members: {{ $g.node.members }}
  node_profile: {{ $g.node.profile | quote }}
  cache_path: {{ $g.cachePath | quote }}
{{- if eq $mode "SingleBinary" }}
  login_server_enabled: true
  login_server_address: {{ printf "127.0.0.1:%d" (int $g.ports.loginGRPC) | quote }}
  friends_server_enabled: true
  friends_server_address: {{ printf "127.0.0.1:%d" (int $g.ports.friendsGRPC) | quote }}
{{- else if eq $mode "World" }}
  login_server_enabled: true
  login_server_address: {{ required "goscape.loginServerAddress is required when deploymentMode=World" $g.loginServerAddress | quote }}
  friends_server_enabled: true
  friends_server_address: {{ required "goscape.friendsServerAddress is required when deploymentMode=World" $g.friendsServerAddress | quote }}
{{- end }}
{{- end -}}

{{/* goscape.config — baseConfig with extraConfig deep-merged over it */}}
{{- define "goscape.config" -}}
{{- $base := include "goscape.baseConfig" . | fromYaml -}}
{{- $extra := deepCopy (.Values.goscape.extraConfig | default dict) -}}
{{- mergeOverwrite $base $extra | toYaml -}}
{{- end -}}
```

- [ ] **Step 3: Create `templates/configmap.yaml`**

```
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "goscape.fullname" . }}
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
  {{- with .Values.commonAnnotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
data:
  config.yaml: |
    {{- include "goscape.config" . | nindent 4 }}
```

- [ ] **Step 4: Run the SingleBinary config tests to verify they pass**

```bash
helm template t production/helm/goscape -f production/helm/goscape/values.yaml --show-only templates/configmap.yaml | grep -q 'target: all'
helm template t production/helm/goscape -f production/helm/goscape/values.yaml --show-only templates/configmap.yaml | grep -q 'login_server_address: "127.0.0.1:2004"'
```

Expected: both grep exit 0.

- [ ] **Step 5: Verify Management mode disables ondemand/world and World mode requires addresses**

```bash
# Management: login/friends enabled, ondemand listed but disabled
helm template t production/helm/goscape -f production/helm/goscape/values.yaml --set deploymentMode=Management --show-only templates/configmap.yaml | grep -A2 '^    ondemand:' | grep -q 'enable: false'
# World without addresses must FAIL
! helm template t production/helm/goscape -f production/helm/goscape/values.yaml --set deploymentMode=World --show-only templates/configmap.yaml 2>/dev/null
# World with addresses renders them
helm template t production/helm/goscape -f production/helm/goscape/values.yaml --set deploymentMode=World --set goscape.loginServerAddress=mgmt:2004 --set goscape.friendsServerAddress=mgmt:2005 --show-only templates/configmap.yaml | grep -q 'login_server_address: "mgmt:2004"'
```

Expected: all three commands exit 0 (the middle one passes because the failing render is negated).

- [ ] **Step 6: Verify extraConfig deep-merge wins**

```bash
helm template t production/helm/goscape -f production/helm/goscape/values.yaml --set goscape.extraConfig.world.node_xp_rate=5 --show-only templates/configmap.yaml | grep -q 'node_xp_rate: 5'
```

Expected: exit 0.

- [ ] **Step 7: Commit**

```bash
git add production/helm/goscape
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(helm): render goscape config.yaml ConfigMap per deploymentMode

target: all + per-mode enable flags, 0.0.0.0 listeners, extraConfig deep-merge.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Shared pod template, StatefulSet, and Service (SingleBinary + Management)

**Files:**
- Modify: `production/helm/goscape/templates/_helpers.tpl` (append `goscape.podTemplate`)
- Create: `production/helm/goscape/templates/statefulset.yaml`
- Create: `production/helm/goscape/templates/service.yaml`
- Create: `production/helm/goscape/single-binary-values.yaml`
- Create: `production/helm/goscape/management-values.yaml`

**Interfaces:**
- Consumes: helpers, `goscape.config`, values `singleBinary.*` / `management.*` / `world.*`, `persistence.*`, `service.*`, `goscape.ports.*`, `goscape.dataPath`, `image.*`.
- Produces: helper `goscape.podTemplate` (call with `(dict "ctx" $ "workload" $w)`; emits the pod `template:` body — `metadata:` + `spec:` — derived from `deploymentMode`). `statefulset.yaml` renders for SingleBinary or Management (StatefulSet + volumeClaimTemplates). `service.yaml` renders a primary Service plus, for stateful modes, a headless `{{ fullname }}-headless` Service. Port names: `ondemand-http`, `world-tcp`, `login-grpc`, `friends-grpc`. The World Deployment (Task 4) reuses `goscape.podTemplate` and `service.yaml`.

- [ ] **Step 1: Create `single-binary-values.yaml`**

```yaml
---
# Run all four modules (ondemand + world + login + friends) in one StatefulSet.
deploymentMode: SingleBinary

singleBinary:
  replicas: 1
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
  extraEnv:
    - name: GOMEMLIMIT
      value: "900MiB"

persistence:
  enabled: true
  size: 5Gi

service:
  type: ClusterIP
```

- [ ] **Step 2: Create `management-values.yaml`**

```yaml
---
# Central management: login + friends only (no world). Worlds dial this release.
deploymentMode: Management

management:
  replicas: 1
  resources:
    requests:
      cpu: 250m
      memory: 256Mi

persistence:
  enabled: true
  size: 5Gi

service:
  type: ClusterIP
```

- [ ] **Step 3: Write the failing test**

```bash
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --show-only templates/statefulset.yaml
```

Expected: FAIL — `could not find template templates/statefulset.yaml`.

- [ ] **Step 4: Append `goscape.podTemplate` to `templates/_helpers.tpl`**

This helper takes a dict `{ctx, workload}`. `ctx` is the root context (`.`), `workload` is the per-mode values section (`.Values.singleBinary` / `.management` / `.world`). It derives ports/probe/mounts from `ctx.Values.deploymentMode`. Do NOT reassign `$` inside the helper — use `$ctx`.

```
{{/* goscape.podTemplate — the pod template body (metadata + spec) shared by all workloads.
     Call with (dict "ctx" $ "workload" $w). */}}
{{- define "goscape.podTemplate" -}}
{{- $ctx := .ctx -}}
{{- $w := .workload -}}
{{- $mode := $ctx.Values.deploymentMode -}}
metadata:
  annotations:
    checksum/config: {{ include "goscape.config" $ctx | sha256sum }}
    {{- with $w.podAnnotations }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
  labels:
    {{- include "goscape.selectorLabels" $ctx | nindent 4 }}
    {{- with $w.podLabels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  serviceAccountName: {{ include "goscape.serviceAccountName" $ctx }}
  {{- with $ctx.Values.image.pullSecrets }}
  imagePullSecrets:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with $w.podSecurityContext }}
  securityContext:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  containers:
    - name: goscape
      image: {{ include "goscape.image" $ctx }}
      imagePullPolicy: {{ $ctx.Values.image.pullPolicy }}
      args:
        - "--config.file=/etc/goscape/config.yaml"
        {{- with $w.extraArgs }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- with $w.extraEnv }}
      env:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      ports:
        {{- if or (eq $mode "SingleBinary") (eq $mode "World") }}
        - name: ondemand-http
          containerPort: {{ $ctx.Values.goscape.ports.ondemandHTTP }}
        - name: world-tcp
          containerPort: {{ $ctx.Values.goscape.ports.worldTCP }}
        {{- end }}
        {{- if or (eq $mode "SingleBinary") (eq $mode "Management") }}
        - name: login-grpc
          containerPort: {{ $ctx.Values.goscape.ports.loginGRPC }}
        - name: friends-grpc
          containerPort: {{ $ctx.Values.goscape.ports.friendsGRPC }}
        {{- end }}
      readinessProbe:
        tcpSocket:
          port: {{ if eq $mode "Management" }}login-grpc{{ else }}world-tcp{{ end }}
        initialDelaySeconds: 10
        periodSeconds: 10
      {{- with $w.containerSecurityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $w.resources }}
      resources:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      volumeMounts:
        - name: config
          mountPath: /etc/goscape
        {{- if or (eq $mode "SingleBinary") (eq $mode "Management") }}
        - name: data
          mountPath: {{ $ctx.Values.goscape.dataPath }}
        {{- end }}
  {{- with $w.nodeSelector }}
  nodeSelector:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with $w.tolerations }}
  tolerations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with $w.affinity }}
  affinity:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  volumes:
    - name: config
      configMap:
        name: {{ include "goscape.fullname" $ctx }}
{{- end -}}
```

- [ ] **Step 5: Create `templates/statefulset.yaml`** (SingleBinary + Management)

```
{{- if or (eq .Values.deploymentMode "SingleBinary") (eq .Values.deploymentMode "Management") -}}
{{- $w := .Values.singleBinary -}}
{{- if eq .Values.deploymentMode "Management" }}{{- $w = .Values.management -}}{{- end -}}
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ include "goscape.fullname" . }}
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
  {{- with .Values.commonAnnotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  replicas: {{ $w.replicas }}
  serviceName: {{ include "goscape.fullname" . }}-headless
  selector:
    matchLabels:
      {{- include "goscape.selectorLabels" . | nindent 6 }}
  template:
    {{- include "goscape.podTemplate" (dict "ctx" . "workload" $w) | nindent 4 }}
  volumeClaimTemplates:
    - metadata:
        name: data
        {{- with .Values.persistence.annotations }}
        annotations:
          {{- toYaml . | nindent 10 }}
        {{- end }}
      spec:
        accessModes:
          {{- toYaml .Values.persistence.accessModes | nindent 10 }}
        {{- with .Values.persistence.storageClass }}
        storageClassName: {{ . }}
        {{- end }}
        resources:
          requests:
            storage: {{ .Values.persistence.size }}
{{- end }}
```

- [ ] **Step 6: Create `templates/service.yaml`** (all modes; headless for stateful modes)

```
{{- $mode := .Values.deploymentMode -}}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "goscape.fullname" . }}
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
  {{- with .Values.service.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: {{ .Values.service.type }}
  selector:
    {{- include "goscape.selectorLabels" . | nindent 4 }}
  ports:
    {{- if or (eq $mode "SingleBinary") (eq $mode "World") }}
    - name: ondemand-http
      port: {{ .Values.goscape.ports.ondemandHTTP }}
      targetPort: ondemand-http
      protocol: TCP
    - name: world-tcp
      port: {{ .Values.goscape.ports.worldTCP }}
      targetPort: world-tcp
      protocol: TCP
    {{- end }}
    {{- if eq $mode "Management" }}
    - name: login-grpc
      port: {{ .Values.goscape.ports.loginGRPC }}
      targetPort: login-grpc
      protocol: TCP
    - name: friends-grpc
      port: {{ .Values.goscape.ports.friendsGRPC }}
      targetPort: friends-grpc
      protocol: TCP
    {{- end }}
{{- if or (eq $mode "SingleBinary") (eq $mode "Management") }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "goscape.fullname" . }}-headless
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
spec:
  clusterIP: None
  selector:
    {{- include "goscape.selectorLabels" . | nindent 4 }}
  ports:
    {{- if eq $mode "SingleBinary" }}
    - name: world-tcp
      port: {{ .Values.goscape.ports.worldTCP }}
      targetPort: world-tcp
      protocol: TCP
    {{- end }}
    {{- if eq $mode "Management" }}
    - name: login-grpc
      port: {{ .Values.goscape.ports.loginGRPC }}
      targetPort: login-grpc
      protocol: TCP
    - name: friends-grpc
      port: {{ .Values.goscape.ports.friendsGRPC }}
      targetPort: friends-grpc
      protocol: TCP
    {{- end }}
{{- end }}
```

- [ ] **Step 7: Run the SingleBinary + Management tests to verify they pass**

```bash
# SingleBinary: StatefulSet with PVC + client services
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --show-only templates/statefulset.yaml | grep -q 'kind: StatefulSet'
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --show-only templates/statefulset.yaml | grep -q 'storage: 5Gi'
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --show-only templates/service.yaml | grep -q 'name: world-tcp'
# Management: StatefulSet + gRPC services, no world-tcp
helm template t production/helm/goscape -f production/helm/goscape/management-values.yaml --show-only templates/statefulset.yaml | grep -q 'kind: StatefulSet'
helm template t production/helm/goscape -f production/helm/goscape/management-values.yaml --show-only templates/service.yaml | grep -q 'name: login-grpc'
! helm template t production/helm/goscape -f production/helm/goscape/management-values.yaml --show-only templates/service.yaml 2>/dev/null | grep -q 'name: world-tcp'
helm lint production/helm/goscape -f production/helm/goscape/single-binary-values.yaml
helm lint production/helm/goscape -f production/helm/goscape/management-values.yaml
```

Expected: all grep commands exit 0; both lints pass.

- [ ] **Step 8: Commit**

```bash
git add production/helm/goscape
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(helm): shared pod template, StatefulSet, and services

One goscape.podTemplate helper drives SingleBinary + Management StatefulSets
and (Task 4) the World Deployment. Mode-aware service.yaml with headless svc
for stateful modes.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: World Deployment + example values

**Files:**
- Create: `production/helm/goscape/templates/deployment.yaml`
- Create: `production/helm/goscape/world-values.yaml`

**Interfaces:**
- Consumes: helper `goscape.podTemplate` and `service.yaml` from Task 3; values `world.*`, `goscape.loginServerAddress`, `goscape.friendsServerAddress`, `goscape.node.id`.
- Produces: Deployment (no volumes beyond config) rendering only when `deploymentMode=World`; reuses the Task 3 Service (World branch already present in `service.yaml`).

- [ ] **Step 1: Create `world-values.yaml`**

```yaml
---
# A single world server (ondemand + world) that dials the central management release.
# Install once per world with a distinct release name and goscape.node.id.
deploymentMode: World

goscape:
  node:
    id: 1
  # Point these at your Management release's Service (see its NOTES output):
  loginServerAddress: ""    # e.g. goscape-mgmt:2004
  friendsServerAddress: ""  # e.g. goscape-mgmt:2005

world:
  replicas: 1
  resources:
    requests:
      cpu: 500m
      memory: 512Mi

service:
  type: ClusterIP
```

- [ ] **Step 2: Write the failing test**

```bash
helm template t production/helm/goscape -f production/helm/goscape/world-values.yaml --set goscape.loginServerAddress=mgmt:2004 --set goscape.friendsServerAddress=mgmt:2005 --show-only templates/deployment.yaml
```

Expected: FAIL — `could not find template templates/deployment.yaml`.

- [ ] **Step 3: Create `templates/deployment.yaml`**

```
{{- if eq .Values.deploymentMode "World" -}}
{{- $w := .Values.world -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "goscape.fullname" . }}
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
  {{- with .Values.commonAnnotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  replicas: {{ $w.replicas }}
  selector:
    matchLabels:
      {{- include "goscape.selectorLabels" . | nindent 6 }}
  template:
    {{- include "goscape.podTemplate" (dict "ctx" . "workload" $w) | nindent 4 }}
{{- end }}
```

- [ ] **Step 4: Run the World tests to verify they pass**

```bash
helm template t production/helm/goscape -f production/helm/goscape/world-values.yaml --set goscape.loginServerAddress=mgmt:2004 --set goscape.friendsServerAddress=mgmt:2005 --show-only templates/deployment.yaml | grep -q 'kind: Deployment'
# no PVC in world mode
! helm template t production/helm/goscape -f production/helm/goscape/world-values.yaml --set goscape.loginServerAddress=mgmt:2004 --set goscape.friendsServerAddress=mgmt:2005 2>/dev/null | grep -q 'volumeClaimTemplates'
# world service exposes client ports
helm template t production/helm/goscape -f production/helm/goscape/world-values.yaml --set goscape.loginServerAddress=mgmt:2004 --set goscape.friendsServerAddress=mgmt:2005 --show-only templates/service.yaml | grep -q 'name: world-tcp'
# no StatefulSet in world mode
! helm template t production/helm/goscape -f production/helm/goscape/world-values.yaml --set goscape.loginServerAddress=mgmt:2004 --set goscape.friendsServerAddress=mgmt:2005 2>/dev/null | grep -q 'kind: StatefulSet'
helm lint production/helm/goscape -f production/helm/goscape/world-values.yaml --set goscape.loginServerAddress=mgmt:2004 --set goscape.friendsServerAddress=mgmt:2005
```

Expected: positive grep commands exit 0; negated checks exit 0; lint passes.

- [ ] **Step 5: Commit**

```bash
git add production/helm/goscape
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(helm): World Deployment and example values

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Ingress + extra-manifests (gated)

**Files:**
- Create: `production/helm/goscape/templates/ondemand-ingress.yaml`
- Create: `production/helm/goscape/templates/extra-manifests.yaml`

**Interfaces:**
- Consumes: helpers, `ingress.*`, `extraManifests`, `goscape.ports.ondemandHTTP`.
- Produces: optional Ingress routing to the `ondemand-http` Service port; rendering of arbitrary user-supplied manifests.

- [ ] **Step 1: Write the failing test**

```bash
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --set ingress.enabled=true --set 'ingress.hosts[0].host=goscape.example.com' --set 'ingress.hosts[0].paths[0].path=/' --set 'ingress.hosts[0].paths[0].pathType=Prefix' --show-only templates/ondemand-ingress.yaml
```

Expected: FAIL — `could not find template templates/ondemand-ingress.yaml`.

- [ ] **Step 2: Create `templates/ondemand-ingress.yaml`**

```
{{- if .Values.ingress.enabled -}}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "goscape.fullname" . }}-ondemand
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
  {{- with .Values.ingress.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  {{- with .Values.ingress.className }}
  ingressClassName: {{ . }}
  {{- end }}
  {{- with .Values.ingress.tls }}
  tls:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  rules:
    {{- range .Values.ingress.hosts }}
    - host: {{ .host | quote }}
      http:
        paths:
          {{- range .paths }}
          - path: {{ .path }}
            pathType: {{ .pathType }}
            backend:
              service:
                name: {{ include "goscape.fullname" $ }}
                port:
                  number: {{ $.Values.goscape.ports.ondemandHTTP }}
          {{- end }}
    {{- end }}
{{- end }}
```

- [ ] **Step 3: Create `templates/extra-manifests.yaml`**

```
{{- range .Values.extraManifests }}
---
{{- if typeIs "string" . }}
{{ tpl . $ }}
{{- else }}
{{ toYaml . }}
{{- end }}
{{- end }}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --set ingress.enabled=true --set 'ingress.hosts[0].host=goscape.example.com' --set 'ingress.hosts[0].paths[0].path=/' --set 'ingress.hosts[0].paths[0].pathType=Prefix' --show-only templates/ondemand-ingress.yaml | grep -q 'host: "goscape.example.com"'
# ingress absent by default
! helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml 2>/dev/null | grep -q 'kind: Ingress'
# extra-manifests renders a supplied object
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --set 'extraManifests[0].apiVersion=v1' --set 'extraManifests[0].kind=ConfigMap' --set 'extraManifests[0].metadata.name=extra' | grep -q 'name: extra'
helm lint production/helm/goscape -f production/helm/goscape/single-binary-values.yaml
```

Expected: all exit 0; lint passes.

- [ ] **Step 5: Commit**

```bash
git add production/helm/goscape
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(helm): optional ondemand Ingress and extra-manifests hook

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: PodDisruptionBudget + NetworkPolicy (gated)

**Files:**
- Create: `production/helm/goscape/templates/poddisruptionbudget.yaml`
- Create: `production/helm/goscape/templates/networkpolicy.yaml`

**Interfaces:**
- Consumes: helpers, `podDisruptionBudget.*`, `networkPolicy.enabled`, `deploymentMode`, `goscape.ports.*`.
- Produces: optional PDB selecting the release pods; optional NetworkPolicy allowing client-facing ingress (SingleBinary/World) or world-only gRPC ingress (Management).

- [ ] **Step 1: Write the failing test**

```bash
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --set podDisruptionBudget.enabled=true --show-only templates/poddisruptionbudget.yaml
```

Expected: FAIL — `could not find template templates/poddisruptionbudget.yaml`.

- [ ] **Step 2: Create `templates/poddisruptionbudget.yaml`**

```
{{- if .Values.podDisruptionBudget.enabled -}}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "goscape.fullname" . }}
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
spec:
  maxUnavailable: {{ .Values.podDisruptionBudget.maxUnavailable }}
  selector:
    matchLabels:
      {{- include "goscape.selectorLabels" . | nindent 6 }}
{{- end }}
```

- [ ] **Step 3: Create `templates/networkpolicy.yaml`**

```
{{- if .Values.networkPolicy.enabled -}}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "goscape.fullname" . }}
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
spec:
  podSelector:
    matchLabels:
      {{- include "goscape.selectorLabels" . | nindent 6 }}
  policyTypes:
    - Ingress
  ingress:
    {{- if eq .Values.deploymentMode "Management" }}
    # Allow gRPC only from goscape world/single-binary pods in this namespace
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: {{ include "goscape.name" . }}
      ports:
        - port: {{ .Values.goscape.ports.loginGRPC }}
          protocol: TCP
        - port: {{ .Values.goscape.ports.friendsGRPC }}
          protocol: TCP
    {{- else }}
    # Allow client-facing ports from anywhere
    - ports:
        - port: {{ .Values.goscape.ports.ondemandHTTP }}
          protocol: TCP
        - port: {{ .Values.goscape.ports.worldTCP }}
          protocol: TCP
    {{- end }}
{{- end }}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --set podDisruptionBudget.enabled=true --show-only templates/poddisruptionbudget.yaml | grep -q 'kind: PodDisruptionBudget'
helm template t production/helm/goscape -f production/helm/goscape/management-values.yaml --set networkPolicy.enabled=true --show-only templates/networkpolicy.yaml | grep -q 'port: 2004'
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --set networkPolicy.enabled=true --show-only templates/networkpolicy.yaml | grep -q 'port: 43594'
# both absent by default
! helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml 2>/dev/null | grep -qE 'kind: (PodDisruptionBudget|NetworkPolicy)'
helm lint production/helm/goscape -f production/helm/goscape/single-binary-values.yaml
```

Expected: all exit 0; lint passes.

- [ ] **Step 5: Commit**

```bash
git add production/helm/goscape
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(helm): optional PodDisruptionBudget and NetworkPolicy

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: helm test connectivity Pod

**Files:**
- Create: `production/helm/goscape/templates/tests/test-connection.yaml`

**Interfaces:**
- Consumes: helpers, `deploymentMode`, `goscape.ports.*`, `image.pullSecrets`.
- Produces: a `helm.sh/hook: test` Pod that `nc -z` the active mode's ports (and `wget` the ondemand endpoint where present).

- [ ] **Step 1: Write the failing test**

```bash
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --show-only templates/tests/test-connection.yaml
```

Expected: FAIL — `could not find template templates/tests/test-connection.yaml`.

- [ ] **Step 2: Create `templates/tests/test-connection.yaml`**

```
apiVersion: v1
kind: Pod
metadata:
  name: {{ include "goscape.fullname" . }}-test-connection
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
  annotations:
    "helm.sh/hook": test
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  {{- with .Values.image.pullSecrets }}
  imagePullSecrets:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  restartPolicy: Never
  containers:
    - name: test
      image: busybox:1.36
      command:
        - sh
        - -c
        - |
          set -e
          host={{ include "goscape.fullname" . }}
          {{- if eq .Values.deploymentMode "Management" }}
          echo "checking login gRPC"; nc -z -w5 "$host" {{ .Values.goscape.ports.loginGRPC }}
          echo "checking friends gRPC"; nc -z -w5 "$host" {{ .Values.goscape.ports.friendsGRPC }}
          {{- else }}
          echo "checking world TCP"; nc -z -w5 "$host" {{ .Values.goscape.ports.worldTCP }}
          echo "checking ondemand HTTP"; wget -q -T 5 -O- "http://$host:{{ .Values.goscape.ports.ondemandHTTP }}/" >/dev/null
          {{- end }}
          echo OK
```

- [ ] **Step 3: Run the tests to verify they pass**

```bash
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --show-only templates/tests/test-connection.yaml | grep -q 'helm.sh/hook": test'
helm template t production/helm/goscape -f production/helm/goscape/single-binary-values.yaml --show-only templates/tests/test-connection.yaml | grep -q 'checking world TCP'
helm template t production/helm/goscape -f production/helm/goscape/management-values.yaml --show-only templates/tests/test-connection.yaml | grep -q 'checking login gRPC'
helm lint production/helm/goscape -f production/helm/goscape/single-binary-values.yaml
```

Expected: all exit 0; lint passes.

- [ ] **Step 4: Commit**

```bash
git add production/helm/goscape
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(helm): busybox helm test connectivity pod

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Bake the cache into the image (Dockerfile + make pack + .dockerignore)

**Files:**
- Modify: `cmd/goscape/Dockerfile`
- Modify: `Makefile` (add `pack` target + cache vars)
- Modify: `.dockerignore`

**Interfaces:**
- Consumes: `goscape-cli pack` (`--src-dir`, `--raw-dir`, `--out-dir`), built binary at `cmd/goscape-cli/goscape-cli`.
- Produces: image with packed cache at `/usr/share/goscape/pack`; `make pack` local helper.

- [ ] **Step 1: Confirm pack inputs vs outputs (informs .dockerignore)**

Run pack help and confirm only `--src-dir` and `--raw-dir` are inputs; `--out-dir`/`--datapack-dir` are outputs:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ./cmd/goscape-cli pack -h 2>&1 | sed -n '1,40p'
```

Expected: flags `-src-dir` (default `data/src`), `-raw-dir` (default `data/raw`), `-out-dir` (default `data/pack`), `-datapack-dir`. Only `data/src` and `data/raw` are inputs. Do NOT exclude `data/src` or `data/raw` from the build context.

- [ ] **Step 2: Add cache vars + `pack` target to `Makefile`**

Add near the other variable definitions (e.g. after `IMAGE_PREFIX`):

```makefile
# Cache packing (baked into the goscape image)
CACHE_SRC_DIR ?= data/src
CACHE_RAW_DIR ?= data/raw
CACHE_OUT_DIR ?= data/pack
```

Add a `pack` target in the goscape section (after the `cmd/goscape-cli/goscape-cli` rule):

```makefile
.PHONY: pack
pack: goscape-cli ## pack the game cache from $(CACHE_SRC_DIR) into $(CACHE_OUT_DIR)
	CGO_ENABLED=0 ./cmd/goscape-cli/goscape-cli pack \
		--src-dir $(CACHE_SRC_DIR) \
		--raw-dir $(CACHE_RAW_DIR) \
		--out-dir $(CACHE_OUT_DIR)
```

Add `pack` to the relevant `.PHONY` line near the top (the one listing `goscape goscape-cli ...`).

- [ ] **Step 3: Modify `cmd/goscape/Dockerfile` build stage to pack the cache**

After the existing `RUN make clean && make IMAGE_TAG=${IMAGE_TAG} goscape goscape-cli` line, add:

```dockerfile
# Pack the game cache from source content (must be present at data/src in the
# build context) into /pack, to be baked into the final image.
ARG CACHE_SRC_DIR=data/src
ARG CACHE_RAW_DIR=data/raw
RUN ./cmd/goscape-cli/goscape-cli pack \
      --src-dir "${CACHE_SRC_DIR}" \
      --raw-dir "${CACHE_RAW_DIR}" \
      --out-dir /pack
```

- [ ] **Step 4: Modify `cmd/goscape/Dockerfile` final stage to copy the cache**

After the two `COPY --from=build ... /usr/bin/...` lines, add:

```dockerfile
# Baked-in packed cache (matches chart default goscape.cachePath)
ARG CACHE_IMAGE_DIR=/usr/share/goscape/pack
COPY --from=build /pack ${CACHE_IMAGE_DIR}
```

- [ ] **Step 5: Trim `.dockerignore`** (exclude regenerated/runtime artifacts only)

Append to `.dockerignore`:

```gitignore
# Regenerated cache / runtime state (cache is repacked during image build)
data/pack
data/pack.*
data/*.db
data/players
```

- [ ] **Step 6: Verify `make pack` produces a cache** (requires content at `data/src`)

If `data/src` is absent, point it at your local content first (read-only), e.g.:

```bash
[ -e data/src ] || ln -s /home/owner/Code/github.com/LostCityRS/Server274-ref/content data/src
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache make pack CACHE_OUT_DIR=$TMPDIR/packtest
ls $TMPDIR/packtest/main_file_cache.dat $TMPDIR/packtest/main_file_cache.idx0
```

Expected: both files exist (pack succeeded).

- [ ] **Step 7: Verify the image build bakes the cache in**

```bash
docker build -f cmd/goscape/Dockerfile -t goscape/goscape:helmtest .
docker run --rm --entrypoint ls goscape/goscape:helmtest /usr/share/goscape/pack | grep -q main_file_cache.dat
```

Expected: build succeeds; `grep` exits 0. (Note: this build is slow and requires `data/src` content in the context.)

- [ ] **Step 8: Commit**

```bash
git add cmd/goscape/Dockerfile Makefile .dockerignore
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(image): pack and bake the game cache into the goscape image

Adds a build-stage goscape-cli pack step (content from data/src) and bakes
the result into /usr/share/goscape/pack. Adds `make pack`; trims .dockerignore.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Makefile helm targets, chart Makefile, README

**Files:**
- Modify: `Makefile` (repoint `helm-lint`/`helm-test`; remove Go helm-test targets)
- Create: `production/helm/goscape/Makefile`
- Create: `production/helm/goscape/README.md`
- Create: `production/helm/goscape/README.md.gotmpl`

**Interfaces:**
- Consumes: the chart + example values files from Tasks 1–7.
- Produces: working `make helm-lint` and `make helm-test` (cluster-free), chart convenience Makefile, and README docs.

- [ ] **Step 1: Repoint the helm targets in the root `Makefile`**

Replace the existing Helm block:

```makefile
########
# Helm #
########

.PHONY: production/helm/goscape/src/helm-test/helm-test
helm-test: production/helm/goscape/src/helm-test/helm-test ## run helm tests

# Package Helm tests but do not run them.
production/helm/goscape/src/helm-test/helm-test:
	CGO_ENABLED=0 go test $(GO_FLAGS) --tags=helm_test -c -o $@ ./$(@D)

helm-lint: ## run helm linter
	$(MAKE) -BC production/helm/goscape lint

helm-docs: ## generate reference documentation
	$(MAKE) -BC docs sources/setup/install/helm/reference.md
```

with:

```makefile
########
# Helm #
########

HELM_CHART_DIR := production/helm/goscape

helm-lint: ## lint the helm chart against each example values file
	helm lint $(HELM_CHART_DIR) -f $(HELM_CHART_DIR)/single-binary-values.yaml
	helm lint $(HELM_CHART_DIR) -f $(HELM_CHART_DIR)/management-values.yaml
	helm lint $(HELM_CHART_DIR) -f $(HELM_CHART_DIR)/world-values.yaml \
		--set goscape.loginServerAddress=mgmt:2004 \
		--set goscape.friendsServerAddress=mgmt:2005

helm-test: ## render the chart for each example values file (cluster-free smoke)
	helm template goscape-test $(HELM_CHART_DIR) -f $(HELM_CHART_DIR)/single-binary-values.yaml >/dev/null
	helm template goscape-test $(HELM_CHART_DIR) -f $(HELM_CHART_DIR)/management-values.yaml >/dev/null
	helm template goscape-test $(HELM_CHART_DIR) -f $(HELM_CHART_DIR)/world-values.yaml \
		--set goscape.loginServerAddress=mgmt:2004 \
		--set goscape.friendsServerAddress=mgmt:2005 >/dev/null
```

Then update the `.PHONY: helm-test helm-lint` line (remove the now-deleted `helm-docs` from any `.PHONY` if present; leave other PHONY entries intact). Also remove the now-orphaned `helm-test-image` / `helm-test-push` targets and the `#helm-test-image` reference in the `images:` target.

- [ ] **Step 2: Create `production/helm/goscape/Makefile`** (convenience)

```makefile
.DEFAULT_GOAL := lint
.PHONY: lint template install-single-binary install-management install-world uninstall

CHART := .
NAMESPACE ?= goscape

lint:
	helm lint $(CHART) -f single-binary-values.yaml
	helm lint $(CHART) -f management-values.yaml
	helm lint $(CHART) -f world-values.yaml \
		--set goscape.loginServerAddress=mgmt:2004 \
		--set goscape.friendsServerAddress=mgmt:2005

template:
	helm template goscape $(CHART) -f single-binary-values.yaml

install-single-binary:
	helm upgrade --install goscape $(CHART) -f single-binary-values.yaml \
		--create-namespace --namespace $(NAMESPACE)

install-management:
	helm upgrade --install goscape-mgmt $(CHART) -f management-values.yaml \
		--create-namespace --namespace $(NAMESPACE)

# Override RELEASE and the management addresses per world.
RELEASE ?= goscape-world-1
LOGIN_ADDR ?= goscape-mgmt:2004
FRIENDS_ADDR ?= goscape-mgmt:2005
install-world:
	helm upgrade --install $(RELEASE) $(CHART) -f world-values.yaml \
		--namespace $(NAMESPACE) \
		--set goscape.loginServerAddress=$(LOGIN_ADDR) \
		--set goscape.friendsServerAddress=$(FRIENDS_ADDR)

uninstall:
	helm uninstall goscape --namespace $(NAMESPACE) || true
```

- [ ] **Step 3: Create `production/helm/goscape/README.md`**

````markdown
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

```bash
helm test <release>
```
````

- [ ] **Step 4: Create `production/helm/goscape/README.md.gotmpl`** (helm-docs source)

```
# goscape Helm chart

{{ template "chart.description" . }}

{{ template "chart.valuesSection" . }}
```

- [ ] **Step 5: Run the tests to verify they pass**

```bash
make helm-lint
make helm-test
```

Expected: both targets complete with no error (each `helm lint` reports 0 failed; each `helm template` renders without error).

- [ ] **Step 6: Verify the Go helm-test scaffolding is fully removed**

```bash
! grep -rn "src/helm-test" Makefile
test ! -d production/helm/goscape/src
```

Expected: both exit 0 (no references; no `src/` dir).

- [ ] **Step 7: Commit**

```bash
git add Makefile production/helm/goscape/Makefile production/helm/goscape/README.md production/helm/goscape/README.md.gotmpl
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(helm): repoint Makefile helm targets, add chart Makefile + README

Replaces the Go helm-test build with cluster-free render checks; adds chart
convenience Makefile and README/helm-docs source.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**Spec coverage** (each spec section → task):
- §1 Goal / location → Task 1 (skeleton at `production/helm/goscape/`).
- §2 topology (`target: all` + enable) → Task 2 config helper.
- §3 deployment modes → Tasks 3 (SingleBinary + Management), 4 (World).
- §4 config generation + extraConfig merge → Task 2.
- §5 networking/addressing (services, required World addrs, ingress, networkpolicy) → Task 3 (services + headless), 4 (required addresses), 5 (ingress), 6 (networkpolicy).
- §6 persistence (PVC for stateful, none for World) → Task 3 (volumeClaimTemplates), 4 (none, asserted).
- §7 image cache bake (Dockerfile, make pack, .dockerignore) → Task 8.
- §8 helm test (busybox Pod, Makefile repoint, remove Go test) → Tasks 7 + 9.
- §9 file layout → Tasks 1–9 collectively. NOTE: per user decision the per-mode-subdir layout is replaced by a shared `goscape.podTemplate` helper + flat `statefulset.yaml`/`deployment.yaml`/`service.yaml` (see Global Constraints).
- §10 values structure → Task 1 values.yaml.
- §11 verification → per-task render/lint asserts + Task 8 docker checks.
- §12 out-of-scope (no servicemonitor/hpa) → honored (never created).

**Placeholder scan:** No "TBD"/"similar to"/"add error handling" placeholders; every template and command is given in full. The empty `loginServerAddress`/`friendsServerAddress` defaults are intentional (required-at-render in World mode), not placeholders.

**Type/name consistency:** Helper names (`goscape.fullname`, `goscape.config`, `goscape.image`, `goscape.selectorLabels`, `goscape.serviceAccountName`, `goscape.podTemplate`) are defined in Tasks 1–3 and used identically afterward. `goscape.podTemplate` is called with `(dict "ctx" . "workload" $w)` in both `statefulset.yaml` (Task 3) and `deployment.yaml` (Task 4); inside it uses `$ctx`/`$w` (never reassigns `$`). Port value paths (`goscape.ports.ondemandHTTP/worldTCP/loginGRPC/friendsGRPC`), `goscape.dataPath`, `goscape.cachePath`, and per-mode workload sections match between values.yaml (Task 1) and all consumers. Service name `{{ fullname }}` and ConfigMap name `{{ fullname }}` are consistent. Cache path `/usr/share/goscape/pack` matches between values default (Task 1) and Dockerfile (Task 8).
